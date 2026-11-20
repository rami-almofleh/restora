package policy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"restora.local/restora/internal/snapshot"
)

// maxWait bounds how long the controller sleeps between looks at a policy, so a clock jump cannot delay a run
// for long.
const maxWait = time.Minute

// Backups starts snapshots. snapshot.Service implements it.
type Backups interface {
	Schedule(ctx context.Context, application string, by snapshot.PolicyRef, exportTo []string) (snapshot.View, error)
}

// Deleter removes backups. snapshot.Service implements it and refuses while a backup runs or a restore needs it.
type Deleter interface {
	Delete(ctx context.Context, id string) error
}

// Reconciler starts the snapshots of the policies when they are due and removes the ones retention no longer keeps.
type Reconciler struct {
	Store     *Store
	Apps      Applications
	Backups   Backups
	Snapshots Snapshots
	Deleter   Deleter
	Log       *slog.Logger
	Now       func() time.Time
}

// Reconcile looks at one policy, starts its run when it is due and applies its retention. It returns how long to
// wait until the next look; paused and invalid policies need no timer.
//
// A run that was missed, for example while the controller was down, is not made up for: the policy runs once now
// and the next run follows the schedule from here.
func (r *Reconciler) Reconcile(ctx context.Context, name string) (time.Duration, error) {
	p, err := r.Store.Get(ctx, name)
	if errors.Is(err, ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	now := r.Now()

	compiled, err := p.Spec.Schedule.Compile()
	if err != nil {
		return 0, r.reject(ctx, p, ErrorInvalidSchedule, err, "Edit the policy and fix the schedule.")
	}
	if err := p.Spec.Selector.Validate(); err != nil {
		return 0, r.reject(ctx, p, ErrorInvalidSelector, err, "Edit the policy and fix the selection.")
	}
	if err := p.Spec.Retention.Validate(); err != nil {
		return 0, r.reject(ctx, p, ErrorInvalidRetention, err, "Edit the policy and fix the retention.")
	}

	if p.Spec.Paused {
		return 0, r.plan(ctx, p, nil, nil)
	}

	delay, err := r.schedule(ctx, p, compiled, now)
	if err != nil {
		return 0, err
	}
	if err := r.enforceRetention(ctx, p, compiled, now); err != nil {
		return 0, err
	}
	return delay, nil
}

// schedule starts the run when it is due and returns how long to wait for the next one.
func (r *Reconciler) schedule(ctx context.Context, p *Policy, compiled Compiled, now time.Time) (time.Duration, error) {
	// A new policy, a resumed one and one whose schedule changed all start counting from now.
	if p.Status.NextRunAt == nil || p.Status.ObservedGeneration != p.Generation {
		next := compiled.Next(now)
		if err := r.plan(ctx, p, &next, nil); err != nil {
			return 0, err
		}
		return wait(next, now), nil
	}

	due := p.Status.NextRunAt.Time
	if now.Before(due) {
		return wait(due, now), nil
	}

	result, err := r.run(ctx, p, now)
	if err != nil {
		return 0, err
	}
	next := compiled.Next(now)
	if err := r.plan(ctx, p, &next, result); err != nil {
		return 0, err
	}
	return wait(next, now), nil
}

// enforceRetention deletes the backups of the policy that its retention no longer keeps. A backup that cannot be
// deleted right now, for example because a restore reads from it, stays for the next pass.
func (r *Reconciler) enforceRetention(ctx context.Context, p *Policy, compiled Compiled, now time.Time) error {
	if !p.Spec.Retention.Enabled() {
		return nil
	}
	snaps, err := r.Snapshots.ByPolicy(ctx, p.Name)
	if err != nil {
		return fmt.Errorf("list backups of policy %s: %w", p.Name, err)
	}
	plan := p.Spec.Retention.Plan(snaps, now, compiled.Location())

	deleted := 0
	var failed []error
	for _, d := range plan.Delete {
		err := r.Deleter.Delete(ctx, d.ID)
		switch {
		case err == nil:
			deleted++
		case errors.Is(err, snapshot.ErrNotFound), errors.Is(err, snapshot.ErrInUse), errors.Is(err, snapshot.ErrRunning):
			r.Log.Info("retention leaves a backup for now", "policy", p.Name, "snapshot", d.ID, "reason", err.Error())
		default:
			failed = append(failed, fmt.Errorf("delete %s: %w", d.ID, err))
		}
	}
	if deleted > 0 {
		r.Log.Info("retention deleted backups", "policy", p.Name, "deleted", deleted)
		cleanup := &Cleanup{At: metav1.NewTime(now), Deleted: deleted}
		if err := r.update(ctx, p, func(s *Status) { s.LastCleanup = cleanup }); err != nil {
			failed = append(failed, err)
		}
	}
	return errors.Join(failed...)
}

// run starts a snapshot for every selected application and records what happened to each.
func (r *Reconciler) run(ctx context.Context, p *Policy, now time.Time) (*RunResult, error) {
	result, err := StartSnapshots(ctx, r.Log, r.Apps, r.Backups, p, now)
	if err != nil {
		return nil, err
	}
	r.Log.Info("policy ran", "policy", p.Name, "started", len(result.Snapshots), "skipped", len(result.Skipped))
	return result, nil
}

// StartSnapshots starts a snapshot for every application a policy selects, copying it to the storage locations of
// the policy. Applications that cannot be backed up right now are skipped, with the reason.
func StartSnapshots(ctx context.Context, log *slog.Logger, apps Applications, backups Backups, p *Policy, now time.Time) (*RunResult, error) {
	refs, err := apps.Refs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	found, missing := p.Spec.Selector.Select(refs)

	result := &RunResult{At: metav1.NewTime(now), Snapshots: []string{}, Skipped: []Skipped{}}
	for _, name := range missing {
		result.Skipped = append(result.Skipped, Skipped{Application: name, Reason: ReasonApplicationMissing})
	}
	by := snapshot.PolicyRef{ID: p.Name, Name: p.Spec.Name}
	for _, app := range found {
		view, err := backups.Schedule(ctx, app, by, p.Spec.ExportTo)
		switch {
		case err == nil:
			result.Snapshots = append(result.Snapshots, view.ID)
		case errors.Is(err, snapshot.ErrInProgress):
			result.Skipped = append(result.Skipped, Skipped{Application: app, Reason: ReasonSnapshotInProgress})
		case errors.Is(err, snapshot.ErrBlocked):
			result.Skipped = append(result.Skipped, Skipped{Application: app, Reason: ReasonRestoreInProgress})
		default:
			log.Error("start scheduled snapshot", "policy", p.Name, "application", app, "error", err)
			result.Skipped = append(result.Skipped, Skipped{Application: app, Reason: ReasonRequestFailed})
		}
	}
	return result, nil
}

// plan stores the next run, or none for a nil next, and clears an earlier error. A non-nil lastRun replaces the
// recorded one.
func (r *Reconciler) plan(ctx context.Context, p *Policy, next *time.Time, lastRun *RunResult) error {
	return r.update(ctx, p, func(s *Status) {
		s.ErrorCode, s.Message, s.Hint = "", "", ""
		s.NextRunAt = nil
		if next != nil {
			t := metav1.NewTime(*next)
			s.NextRunAt = &t
		}
		if lastRun != nil {
			s.LastRun = lastRun
		}
	})
}

// reject records why the policy cannot run. It does not return an error: retrying would not help until a person edits it.
func (r *Reconciler) reject(ctx context.Context, p *Policy, code string, cause error, hint string) error {
	return r.update(ctx, p, func(s *Status) {
		s.NextRunAt = nil
		s.ErrorCode, s.Message, s.Hint = code, cause.Error(), hint
	})
}

// update applies a change to the status and writes it when something changed.
func (r *Reconciler) update(ctx context.Context, p *Policy, change func(*Status)) error {
	status := p.Status // a shallow copy is enough: change replaces pointers, it never edits what they point to
	change(&status)
	status.ObservedGeneration = p.Generation
	if equality.Semantic.DeepEqual(p.Status, status) {
		return nil
	}
	p.Status = status
	updated, err := r.Store.UpdateStatus(ctx, p)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	// Keep the new resource version, so that a second write in the same pass does not conflict.
	*p = *updated
	return nil
}

func wait(until, now time.Time) time.Duration {
	d := until.Sub(now)
	if d < time.Second {
		d = time.Second
	}
	return min(d, maxWait)
}
