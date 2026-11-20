package snapshot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// pollInterval is how often a running snapshot is checked.
	pollInterval = 2 * time.Second
	// exportPollInterval is how often a running copy is checked. Copies take much longer than snapshots.
	exportPollInterval = 3 * time.Second
	// errorGrace is how long a VolumeSnapshot may report an error before the backup fails.
	// CSI drivers retry, and the first attempts often fail for harmless reasons.
	errorGrace = 60 * time.Second
	// DefaultTimeout is how long a snapshot may take before it is marked as failed.
	DefaultTimeout = 10 * time.Minute

	readyCondition = "Ready"
)

// ExportDriver copies finished backups to storage locations. offsite.Exporter implements it.
type ExportDriver interface {
	// Advance moves the copies of a snapshot forward by one step and records the progress in snap.Status.Exports.
	// done is true when every copy has ended, in success or failure.
	Advance(ctx context.Context, snap *Snapshot) (done bool, err error)
	// Cleanup removes what copies left in the cluster: data mover pods, temporary volumes and secrets.
	Cleanup(ctx context.Context, snap *Snapshot) error
}

// Reconciler carries out Snapshot resources.
type Reconciler struct {
	Store    *Store
	Volumes  *Volumes
	Exporter *Exporter
	// Export copies backups to storage locations. Without it, backups that ask for a copy only fail the copy.
	Export  ExportDriver
	Log     *slog.Logger
	Timeout time.Duration
	Now     func() time.Time
}

// Reconcile moves one Snapshot forward. Finished snapshots are left alone.
func (r *Reconciler) Reconcile(ctx context.Context, name string) (time.Duration, error) {
	snap, err := r.Store.Get(ctx, name)
	if errors.Is(err, ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	if snap.DeletionTimestamp != nil {
		return 0, r.finalize(ctx, snap)
	}
	if !slices.Contains(snap.Finalizers, Finalizer) {
		snap.Finalizers = append(snap.Finalizers, Finalizer)
		if snap, err = r.Store.Update(ctx, snap); err != nil {
			return 0, err
		}
	}

	switch snap.Status.Phase {
	case PhaseSucceeded:
		return r.retryExports(ctx, snap)
	case PhaseFailed:
		return 0, nil
	case PhaseRunning:
		if len(snap.Status.Volumes) == 0 {
			// The controller stopped before it recorded the volumes. start is safe to repeat.
			return r.start(ctx, snap)
		}
		return r.progress(ctx, snap)
	default:
		return r.start(ctx, snap)
	}
}

// start exports the resources and creates the VolumeSnapshots.
func (r *Reconciler) start(ctx context.Context, snap *Snapshot) (time.Duration, error) {
	now := metav1.NewTime(r.Now())
	snap.Status = Status{
		Phase:     PhaseRunning,
		Message:   "The backup is running.",
		StartedAt: &now,
		Steps: []StepStatus{
			{ID: StepExportResources, Phase: PhaseRunning},
			{ID: StepSnapshotVolumes, Phase: PhasePending},
		},
	}
	if len(snap.Spec.ExportTo) > 0 {
		snap.Status.Steps = append(snap.Status.Steps, StepStatus{ID: StepExportToLocation, Phase: PhasePending})
	}
	snap, err := r.Store.UpdateStatus(ctx, snap)
	if err != nil {
		return 0, err
	}
	log := r.Log.With("snapshot", snap.Name, "application", snap.Spec.Application)

	stats, err := r.Exporter.Export(ctx, snap.Name, snap.Spec.Application)
	if err != nil {
		return 0, r.fail(ctx, snap, StepExportResources, &Failure{
			Code:    "export_failed",
			Message: err.Error(),
			Hint:    "Check that the Restora controller may read the resources of the application and that its data volume has free space.",
		})
	}
	snap.Status.Resources = &stats
	setStep(&snap.Status, StepExportResources, PhaseSucceeded)
	setStep(&snap.Status, StepSnapshotVolumes, PhaseRunning)
	log.Info("resources exported", "resources", stats.Count, "bytes", stats.Bytes)

	planned, err := r.Volumes.Plan(ctx, snap.Spec.Application, snap.Spec.VolumeSnapshotClassName)
	if err != nil {
		return 0, r.failWith(ctx, snap, StepSnapshotVolumes, err)
	}
	volumes, err := r.Volumes.Create(ctx, snap.Name, snap.Spec.Application, planned)
	if err != nil {
		return 0, r.failWith(ctx, snap, StepSnapshotVolumes, err)
	}
	snap.Status.Volumes = volumes
	log.Info("volume snapshots requested", "volumes", len(volumes))

	if len(volumes) == 0 {
		return 0, r.succeed(ctx, snap)
	}
	if _, err := r.Store.UpdateStatus(ctx, snap); err != nil {
		return 0, err
	}
	return pollInterval, nil
}

// progress checks the VolumeSnapshots of a running snapshot, and once they are ready, the copies to storage locations.
func (r *Reconciler) progress(ctx context.Context, snap *Snapshot) (time.Duration, error) {
	if stepDone(&snap.Status, StepSnapshotVolumes) {
		return r.exportProgress(ctx, snap)
	}
	started := r.Now()
	if snap.Status.StartedAt != nil {
		started = snap.Status.StartedAt.Time
	}
	elapsed := r.Now().Sub(started)
	if elapsed > r.timeout() {
		return 0, r.fail(ctx, snap, StepSnapshotVolumes, &Failure{
			Code:    "timeout",
			Message: fmt.Sprintf("The backup did not finish within %s.", r.timeout()),
			Hint:    "Check the storage driver and the snapshot controller in your cluster, then start a new backup.",
		})
	}

	volumes, err := r.Volumes.Refresh(ctx, snap.Spec.Application, snap.Status.Volumes)
	if err != nil {
		return 0, r.failWith(ctx, snap, StepSnapshotVolumes, err)
	}
	snap.Status.Volumes = volumes

	allReady := true
	for _, v := range volumes {
		if v.Ready {
			continue
		}
		allReady = false
		if v.Error != "" && elapsed > errorGrace {
			return 0, r.fail(ctx, snap, StepSnapshotVolumes, &Failure{
				Code:    "snapshot_failed",
				Message: fmt.Sprintf("Volume %s: %s", v.Claim, v.Error),
				Hint:    "The storage driver reported this error. Check its logs and the events of the VolumeSnapshot, then start a new backup.",
			})
		}
	}
	if allReady {
		if len(snap.Spec.ExportTo) > 0 {
			return r.startExport(ctx, snap)
		}
		return 0, r.succeed(ctx, snap)
	}
	if _, err := r.Store.UpdateStatus(ctx, snap); err != nil {
		return 0, err
	}
	return pollInterval, nil
}

// startExport ends the local part of the backup and begins the copies to the storage locations. The local backup is
// complete at this point, so the time limit for creating it no longer applies.
func (r *Reconciler) startExport(ctx context.Context, snap *Snapshot) (time.Duration, error) {
	setStep(&snap.Status, StepSnapshotVolumes, PhaseSucceeded)
	setStep(&snap.Status, StepExportToLocation, PhaseRunning)
	snap.Status.Message = "The backup is taken. It is now copied to the storage locations."
	if len(snap.Status.Exports) == 0 {
		for _, location := range snap.Spec.ExportTo {
			snap.Status.Exports = append(snap.Status.Exports, ExportStatus{Location: location, Phase: PhasePending})
		}
	}
	if _, err := r.Store.UpdateStatus(ctx, snap); err != nil {
		return 0, err
	}
	return pollInterval, nil
}

// exportProgress moves the copies forward. When all of them ended the snapshot is complete, even if a copy failed:
// the local backup works, and the failed copy is reported on its own.
func (r *Reconciler) exportProgress(ctx context.Context, snap *Snapshot) (time.Duration, error) {
	if r.Export == nil {
		for i := range snap.Status.Exports {
			if !snap.Status.Exports[i].Finished() {
				failExport(&snap.Status.Exports[i], r.Now(), "export_unavailable",
					"This version of Restora cannot copy backups to storage locations.", "Update the Restora controller.")
			}
		}
	}
	if r.Export != nil {
		done, err := r.Export.Advance(ctx, snap)
		if err != nil {
			return 0, err
		}
		if !done {
			if _, err := r.Store.UpdateStatus(ctx, snap); err != nil {
				return 0, err
			}
			return exportPollInterval, nil
		}
	}

	failed := false
	for _, e := range snap.Status.Exports {
		failed = failed || e.Phase == PhaseFailed
	}
	if failed {
		setStep(&snap.Status, StepExportToLocation, PhaseFailed)
	} else {
		setStep(&snap.Status, StepExportToLocation, PhaseSucceeded)
	}
	return 0, r.succeed(ctx, snap)
}

// retryExports starts the copies again that a person asked to repeat. A request counts once: it is newer than the
// end of the failed copy, and a copy that runs again ends later than the request.
func (r *Reconciler) retryExports(ctx context.Context, snap *Snapshot) (time.Duration, error) {
	if r.Export == nil {
		return 0, nil
	}
	restarted := false
	for _, retry := range snap.Spec.ExportRetries {
		for i := range snap.Status.Exports {
			e := &snap.Status.Exports[i]
			if e.Location != retry.Location || e.Phase != PhaseFailed || e.FinishedAt == nil || !retry.RequestedAt.After(e.FinishedAt.Time) {
				continue
			}
			*e = ExportStatus{Location: e.Location, LocationName: e.LocationName, Phase: PhasePending}
			restarted = true
		}
	}
	if !restarted {
		return 0, nil
	}
	snap.Status.Phase = PhaseRunning
	snap.Status.Message = "The backup is copied to the storage locations again."
	snap.Status.FinishedAt = nil
	setStep(&snap.Status, StepExportToLocation, PhaseRunning)
	setReady(snap, metav1.ConditionFalse, "Running")
	if _, err := r.Store.UpdateStatus(ctx, snap); err != nil {
		return 0, err
	}
	return exportPollInterval, nil
}

func failExport(e *ExportStatus, now time.Time, code, message, hint string) {
	finished := metav1.NewTime(now)
	e.Phase, e.ErrorCode, e.Message, e.Hint, e.FinishedAt = PhaseFailed, code, message, hint, &finished
}

func stepDone(st *Status, id string) bool {
	for _, s := range st.Steps {
		if s.ID == id {
			return s.Phase == PhaseSucceeded
		}
	}
	return false
}

func (r *Reconciler) succeed(ctx context.Context, snap *Snapshot) error {
	finished := metav1.NewTime(r.Now())
	snap.Status.Phase = PhaseSucceeded
	snap.Status.Message = "The backup is complete."
	for _, e := range snap.Status.Exports {
		if e.Phase == PhaseFailed {
			snap.Status.Message = "The backup is complete, but copying it to a storage location failed."
			break
		}
	}
	snap.Status.ErrorCode, snap.Status.Hint = "", ""
	snap.Status.FinishedAt = &finished
	setStep(&snap.Status, StepSnapshotVolumes, PhaseSucceeded)
	setReady(snap, metav1.ConditionTrue, "Succeeded")
	if _, err := r.Store.UpdateStatus(ctx, snap); err != nil {
		return err
	}
	r.Log.Info("snapshot succeeded", "snapshot", snap.Name, "application", snap.Spec.Application)
	return nil
}

// failWith turns a Failure into a failed snapshot. Any other error is returned for a retry.
func (r *Reconciler) failWith(ctx context.Context, snap *Snapshot, step string, err error) error {
	var failure *Failure
	if errors.As(err, &failure) {
		return r.fail(ctx, snap, step, failure)
	}
	return err
}

// fail marks the snapshot as failed and removes what it created, so a failed backup leaves nothing behind.
func (r *Reconciler) fail(ctx context.Context, snap *Snapshot, step string, f *Failure) error {
	finished := metav1.NewTime(r.Now())
	snap.Status.Phase = PhaseFailed
	snap.Status.Message = f.Message
	snap.Status.ErrorCode = f.Code
	snap.Status.Hint = f.Hint
	snap.Status.FinishedAt = &finished
	setStep(&snap.Status, step, PhaseFailed)
	setReady(snap, metav1.ConditionFalse, "Failed")

	if err := r.cleanup(ctx, snap); err != nil {
		r.Log.Warn("clean up after failed snapshot", "snapshot", snap.Name, "error", err)
	}
	if _, err := r.Store.UpdateStatus(ctx, snap); err != nil {
		return err
	}
	r.Log.Warn("snapshot failed", "snapshot", snap.Name, "application", snap.Spec.Application, "code", f.Code, "message", f.Message)
	return nil
}

// finalize removes everything a deleted Snapshot owns, then releases it.
func (r *Reconciler) finalize(ctx context.Context, snap *Snapshot) error {
	if !slices.Contains(snap.Finalizers, Finalizer) {
		return nil
	}
	if err := r.cleanup(ctx, snap); err != nil {
		return err
	}
	snap.Finalizers = slices.DeleteFunc(snap.Finalizers, func(f string) bool { return f == Finalizer })
	_, err := r.Store.Update(ctx, snap)
	return err
}

func (r *Reconciler) cleanup(ctx context.Context, snap *Snapshot) error {
	errs := []error{
		r.Volumes.Delete(ctx, snap.Name, snap.Spec.Application),
		r.Exporter.Delete(snap.Name),
	}
	if r.Export != nil {
		errs = append(errs, r.Export.Cleanup(ctx, snap))
	}
	return errors.Join(errs...)
}

func (r *Reconciler) timeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}
	return DefaultTimeout
}

func setStep(st *Status, id string, phase Phase) {
	for i := range st.Steps {
		if st.Steps[i].ID == id {
			st.Steps[i].Phase = phase
			return
		}
	}
	st.Steps = append(st.Steps, StepStatus{ID: id, Phase: phase})
}

func setReady(snap *Snapshot, status metav1.ConditionStatus, reason string) {
	meta.SetStatusCondition(&snap.Status.Conditions, metav1.Condition{
		Type:               readyCondition,
		Status:             status,
		Reason:             reason,
		Message:            snap.Status.Message,
		ObservedGeneration: snap.Generation,
	})
}
