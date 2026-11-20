package restore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"restora.local/restora/internal/offsite"
	"restora.local/restora/internal/snapshot"
)

const (
	// pollInterval is how often a waiting step is checked again.
	pollInterval = 2 * time.Second
	// DefaultTimeout is how long a whole restore may take before it is marked as failed.
	DefaultTimeout = 15 * time.Minute

	readyCondition = "Ready"
)

// OffsiteRestorer brings backups back from storage locations. offsite.Restorer implements it.
type OffsiteRestorer interface {
	// Prepare fetches the saved resources of an export. It returns false while that is still going on.
	Prepare(ctx context.Context, requestName string, rec *offsite.Record) (bool, error)
	// Manifest reads what Prepare fetched about the application.
	Manifest(requestName string) (offsite.Manifest, error)
	// RestoreVolume fills the (empty) claim of one volume of the export. It returns false while that is still going on.
	RestoreVolume(ctx context.Context, requestName string, rec *offsite.Record, index int) (bool, error)
	// Cleanup removes the data mover pods and the fetched files of a restore.
	Cleanup(ctx context.Context, requestName, application string) error
}

// Reconciler carries out RestoreRequest resources.
type Reconciler struct {
	Client    dynamic.Interface
	Store     *Store
	Snapshots *snapshot.Store
	Exports   *snapshot.Exporter
	// Records and Offsite are needed to restore from storage locations.
	Records *offsite.RecordStore
	Offsite OffsiteRestorer
	Log     *slog.Logger
	Timeout time.Duration
	// ExportTimeout is how long a restore from a storage location may take. It is longer, because data comes
	// over the network.
	ExportTimeout time.Duration
	Now           func() time.Time
}

// stepFunc does the work of one step. It returns done=false to be called again after a short wait.
// Every step must be safe to repeat: the controller can stop at any point.
type stepFunc func(ctx context.Context, req *RestoreRequest, env *environment) (done bool, err error)

// environment holds what the steps read from the snapshot. It is loaded once per pass.
type environment struct {
	snap *snapshot.Snapshot
	// record is set instead of snap when the restore comes from a storage location.
	record *offsite.Record
	bundle snapshot.Bundle
	loaded bool
}

// Reconcile moves one RestoreRequest forward. Finished requests are left alone.
func (r *Reconciler) Reconcile(ctx context.Context, name string) (time.Duration, error) {
	req, err := r.Store.Get(ctx, name)
	if errors.Is(err, ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	switch req.Status.Phase {
	case snapshot.PhaseSucceeded, snapshot.PhaseFailed:
		return 0, nil
	case snapshot.PhaseRunning:
	default:
		started := metav1.NewTime(r.Now())
		req.Status = Status{Phase: snapshot.PhaseRunning, Message: "The restore is running.", StartedAt: &started}
		for _, id := range stepOrder {
			req.Status.Steps = append(req.Status.Steps, snapshot.StepStatus{ID: id, Phase: snapshot.PhasePending})
		}
		if err := r.save(ctx, req); err != nil {
			return 0, err
		}
		r.Log.Info("restore started", "restore", req.Name, "application", req.Spec.Application, "snapshot", req.Spec.Snapshot)
	}

	if req.Status.StartedAt != nil && r.Now().Sub(req.Status.StartedAt.Time) > r.timeoutFor(req) {
		return 0, r.fail(ctx, req, r.currentStep(req), r.timeoutFailure(req))
	}
	return r.advance(ctx, req)
}

// advance runs the steps in order, starting with the first one that is not finished.
func (r *Reconciler) advance(ctx context.Context, req *RestoreRequest) (time.Duration, error) {
	steps := map[string]stepFunc{
		StepCheck:     r.checkSnapshot,
		StepStop:      r.stopWorkloads,
		StepVolumes:   r.restoreVolumes,
		StepResources: r.restoreResources,
		StepReady:     r.waitReady,
	}
	env := &environment{}
	for _, id := range stepOrder {
		if stepPhase(req, id) == snapshot.PhaseSucceeded {
			continue
		}
		setStep(req, id, snapshot.PhaseRunning)
		done, err := steps[id](ctx, req, env)
		if err != nil {
			return 0, r.failWith(ctx, req, id, err)
		}
		if !done {
			if err := r.save(ctx, req); err != nil {
				return 0, err
			}
			return pollInterval, nil
		}
		setStep(req, id, snapshot.PhaseSucceeded)
		if err := r.save(ctx, req); err != nil {
			return 0, err
		}
		r.Log.Info("restore step done", "restore", req.Name, "step", id)
	}
	return 0, r.succeed(ctx, req)
}

func (r *Reconciler) succeed(ctx context.Context, req *RestoreRequest) error {
	finished := metav1.NewTime(r.Now())
	req.Status.Phase = snapshot.PhaseSucceeded
	req.Status.Message = "The application was restored."
	req.Status.ErrorCode, req.Status.Hint = "", ""
	req.Status.FinishedAt = &finished
	setReady(req, metav1.ConditionTrue, "Succeeded")
	if err := r.save(ctx, req); err != nil {
		return err
	}
	r.cleanupExport(ctx, req)
	r.Log.Info("restore succeeded", "restore", req.Name, "application", req.Spec.Application)
	return nil
}

// failWith turns a snapshot.Failure into a failed restore. Any other error is returned for a retry.
func (r *Reconciler) failWith(ctx context.Context, req *RestoreRequest, step string, err error) error {
	var failure *snapshot.Failure
	if errors.As(err, &failure) {
		return r.fail(ctx, req, step, failure)
	}
	return err
}

// fail marks the restore as failed. Before any data was touched the workloads are started again, so a restore
// that never began leaves the application as it was.
func (r *Reconciler) fail(ctx context.Context, req *RestoreRequest, step string, f *snapshot.Failure) error {
	if step == StepCheck || step == StepStop {
		if err := r.scaleBack(ctx, req, nil); err != nil {
			r.Log.Warn("start workloads again after failed restore", "restore", req.Name, "error", err)
		}
	}
	finished := metav1.NewTime(r.Now())
	req.Status.Phase = snapshot.PhaseFailed
	req.Status.Message = f.Message
	req.Status.ErrorCode = f.Code
	req.Status.Hint = f.Hint
	req.Status.FinishedAt = &finished
	if step != "" {
		setStep(req, step, snapshot.PhaseFailed)
	}
	setReady(req, metav1.ConditionFalse, "Failed")
	if err := r.save(ctx, req); err != nil {
		return err
	}
	r.cleanupExport(ctx, req)
	r.Log.Warn("restore failed", "restore", req.Name, "application", req.Spec.Application, "code", f.Code, "message", f.Message)
	return nil
}

// cleanupExport removes what a restore from a storage location left in the cluster: data mover pods and fetched files.
func (r *Reconciler) cleanupExport(ctx context.Context, req *RestoreRequest) {
	if req.Spec.Export == "" || r.Offsite == nil {
		return
	}
	if err := r.Offsite.Cleanup(ctx, req.Name, req.Spec.Application); err != nil {
		r.Log.Warn("clean up after restore from export", "restore", req.Name, "error", err)
	}
}

func (r *Reconciler) timeoutFailure(req *RestoreRequest) *snapshot.Failure {
	limit := r.timeoutFor(req)
	if r.currentStep(req) == StepReady {
		return &snapshot.Failure{
			Code:    "workloads_not_ready",
			Message: "The data was restored, but the application did not start in time.",
			Hint:    "Look at the pods and events of the application to see why it does not start. The backup itself is unchanged.",
		}
	}
	return &snapshot.Failure{
		Code:    "timeout",
		Message: fmt.Sprintf("The restore did not finish within %s.", limit),
		Hint:    "Check that the pods of the application stop and that the storage driver can create volumes from snapshots, then start a new restore. The backup is unchanged.",
	}
}

func (r *Reconciler) save(ctx context.Context, req *RestoreRequest) error {
	updated, err := r.Store.UpdateStatus(ctx, req)
	if err != nil {
		return err
	}
	*req = *updated
	return nil
}

// DefaultExportTimeout is how long a restore from a storage location may take.
const DefaultExportTimeout = 6 * time.Hour

func (r *Reconciler) timeoutFor(req *RestoreRequest) time.Duration {
	if req.Spec.Export == "" {
		return r.timeout()
	}
	if r.ExportTimeout > 0 {
		return r.ExportTimeout
	}
	return DefaultExportTimeout
}

func (r *Reconciler) timeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}
	return DefaultTimeout
}

// currentStep returns the first step that has not finished.
func (r *Reconciler) currentStep(req *RestoreRequest) string {
	for _, id := range stepOrder {
		if stepPhase(req, id) != snapshot.PhaseSucceeded {
			return id
		}
	}
	return ""
}

func stepPhase(req *RestoreRequest, id string) snapshot.Phase {
	for _, s := range req.Status.Steps {
		if s.ID == id {
			return s.Phase
		}
	}
	return snapshot.PhasePending
}

func setStep(req *RestoreRequest, id string, phase snapshot.Phase) {
	for i := range req.Status.Steps {
		if req.Status.Steps[i].ID == id {
			req.Status.Steps[i].Phase = phase
			return
		}
	}
	req.Status.Steps = append(req.Status.Steps, snapshot.StepStatus{ID: id, Phase: phase})
}

func setReady(req *RestoreRequest, status metav1.ConditionStatus, reason string) {
	meta.SetStatusCondition(&req.Status.Conditions, metav1.Condition{
		Type:               readyCondition,
		Status:             status,
		Reason:             reason,
		Message:            req.Status.Message,
		ObservedGeneration: req.Generation,
	})
}

// load reads the snapshot and its export once per pass.
func (e *environment) load(ctx context.Context, r *Reconciler, req *RestoreRequest) error {
	if e.loaded {
		return nil
	}
	if req.Spec.Export != "" {
		return e.loadExport(ctx, r, req)
	}
	snap, err := r.Snapshots.Get(ctx, req.Spec.Snapshot)
	if errors.Is(err, snapshot.ErrNotFound) {
		return &snapshot.Failure{
			Code:    "snapshot_not_found",
			Message: fmt.Sprintf("Backup %s does not exist any more.", req.Spec.Snapshot),
			Hint:    "Someone deleted the backup. Pick another one.",
		}
	}
	if err != nil {
		return err
	}
	bundle, err := r.Exports.Load(req.Spec.Snapshot)
	if err != nil {
		return &snapshot.Failure{
			Code:    "export_missing",
			Message: fmt.Sprintf("The saved resources of backup %s cannot be read: %v", req.Spec.Snapshot, err),
			Hint:    "The Restora data volume lost the files of this backup. The backup cannot be restored.",
		}
	}
	e.snap, e.bundle, e.loaded = snap, bundle, true
	return nil
}

// loadExport reads the export record and the saved resources that were fetched from the repository.
func (e *environment) loadExport(ctx context.Context, r *Reconciler, req *RestoreRequest) error {
	record, err := r.Records.Get(ctx, req.Spec.Export)
	if errors.Is(err, offsite.ErrNotFound) {
		return &snapshot.Failure{
			Code:    "export_not_found",
			Message: fmt.Sprintf("The stored backup %s does not exist any more.", req.Spec.Export),
			Hint:    "Pick another backup.",
		}
	}
	if err != nil {
		return err
	}
	bundle, err := r.Exports.Load(offsite.StageName(req.Name))
	if err != nil {
		return &snapshot.Failure{
			Code:    "export_missing",
			Message: fmt.Sprintf("The saved resources of the stored backup cannot be read: %v", err),
			Hint:    "The Restora data volume lost the fetched files. Start the restore again.",
		}
	}
	e.record, e.bundle, e.loaded = record, bundle, true
	return nil
}
