package policy

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"restora.local/restora/internal/snapshot"
)

// retentionWorld is a policy that keeps the newest two backups, with five backups on record.
type retentionWorld struct {
	svc      *Service
	policy   *Policy
	snaps    *fakeSnapshots
	deleter  *fakeDeleter
	reconcil *Reconciler
	now      time.Time
}

func newRetentionWorld(t *testing.T, retention Retention) *retentionWorld {
	t.Helper()
	svc := newTestService()
	in := everyMinute("Keep", "demo")
	in.Retention = retention
	p := storedPolicy(t, svc, in)

	w := &retentionWorld{svc: svc, policy: p, snaps: &fakeSnapshots{}, deleter: &fakeDeleter{}, now: testNow.Add(time.Hour)}
	for minute := 0; minute < 5; minute++ {
		w.snaps.items = append(w.snaps.items, ok("demo", testNow.Add(time.Duration(minute)*time.Minute)))
	}
	w.reconcil = &Reconciler{
		Store: svc.store, Apps: testApps(), Backups: &fakeBackups{}, Snapshots: w.snaps, Deleter: w.deleter,
		Log: discardLog(), Now: func() time.Time { return w.now },
	}
	// The first look starts the clock. Retention already applies at that look, so start counting afterwards.
	reconcile(t, w.reconcil, p.Name)
	w.deleter.deleted = nil
	return w
}

func (w *retentionWorld) policyNow(t *testing.T) *Policy {
	t.Helper()
	p, err := w.svc.store.Get(context.Background(), w.policy.Name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReconcilerDeletesWhatRetentionDoesNotKeep(t *testing.T) {
	w := newRetentionWorld(t, Retention{Last: 2})
	// A look a little later, before the next run is due.
	w.now = testNow.Add(time.Hour + 5*time.Second)
	reconcile(t, w.reconcil, w.policy.Name)

	want := []string{name("demo", testNow.Add(2*time.Minute)), name("demo", testNow.Add(time.Minute)), name("demo", testNow)}
	if !slices.Equal(w.deleter.deleted, want) {
		t.Errorf("deleted = %v, want %v", w.deleter.deleted, want)
	}
	cleanup := w.policyNow(t).Status.LastCleanup
	if cleanup == nil || cleanup.Deleted != 3 {
		t.Errorf("last cleanup = %+v, want 3 deleted", cleanup)
	}
}

func TestReconcilerKeepsEverythingWithoutRetention(t *testing.T) {
	w := newRetentionWorld(t, Retention{})
	w.now = testNow.Add(time.Hour + 5*time.Second)
	reconcile(t, w.reconcil, w.policy.Name)

	if len(w.deleter.deleted) != 0 || w.policyNow(t).Status.LastCleanup != nil {
		t.Errorf("deleted = %v, cleanup = %+v; without rules nothing may go", w.deleter.deleted, w.policyNow(t).Status.LastCleanup)
	}
}

func TestReconcilerLeavesBackupsThatCannotBeDeletedYet(t *testing.T) {
	w := newRetentionWorld(t, Retention{Last: 2})
	busy := name("demo", testNow.Add(time.Minute))
	w.deleter.refuse = map[string]error{busy: fmt.Errorf("%w: restore", snapshot.ErrInUse)}
	w.now = testNow.Add(time.Hour + 5*time.Second)

	reconcile(t, w.reconcil, w.policy.Name) // no error: the backup is tried again next time

	if slices.Contains(w.deleter.deleted, busy) || len(w.deleter.deleted) != 2 {
		t.Errorf("deleted = %v, want the two others without %s", w.deleter.deleted, busy)
	}
	if cleanup := w.policyNow(t).Status.LastCleanup; cleanup == nil || cleanup.Deleted != 2 {
		t.Errorf("last cleanup = %+v", cleanup)
	}
}

func TestReconcilerRetriesAfterARealDeletionError(t *testing.T) {
	w := newRetentionWorld(t, Retention{Last: 2})
	w.deleter.refuse = map[string]error{name("demo", testNow): errBoom}
	w.now = testNow.Add(time.Hour + 5*time.Second)

	if _, err := w.reconcil.Reconcile(context.Background(), w.policy.Name); err == nil {
		t.Error("an unexpected deletion error must be returned so that the pass is retried")
	}
}

func TestReconcilerPausedPolicyDeletesNothing(t *testing.T) {
	w := newRetentionWorld(t, Retention{Last: 2})
	if _, err := w.svc.SetPaused(context.Background(), w.policy.Name, true); err != nil {
		t.Fatal(err)
	}
	w.now = testNow.Add(time.Hour + 5*time.Second)
	reconcile(t, w.reconcil, w.policy.Name)

	if len(w.deleter.deleted) != 0 {
		t.Errorf("deleted = %v, a paused policy must not delete", w.deleter.deleted)
	}
}

func TestReconcilerRejectsInvalidRetention(t *testing.T) {
	svc := newTestService()
	bad := storedPolicy(t, svc, everyMinute("Bad", "demo"))
	bad.Spec.Retention = Retention{Daily: -5}
	if _, err := svc.store.Update(context.Background(), bad); err != nil {
		t.Fatal(err)
	}
	r := newTestReconciler(svc.store, testApps(), &fakeBackups{}, testNow)
	reconcile(t, r, bad.Name)

	got, _ := svc.store.Get(context.Background(), bad.Name)
	if got.Status.ErrorCode != ErrorInvalidRetention || got.Status.Hint == "" {
		t.Errorf("status = %+v", got.Status)
	}
}

func TestServiceRetentionPreview(t *testing.T) {
	svc := newTestService()
	p := mustCreate(t, svc, everyMinute("A", "demo"))
	snaps := &fakeSnapshots{}
	for minute := 0; minute < 4; minute++ {
		snaps.items = append(snaps.items, ok("demo", testNow.Add(-time.Duration(minute)*time.Minute)))
	}
	svc.snapshots = snaps

	in := everyMinute("A", "demo")
	in.Retention = Retention{Last: 1}
	plan, err := svc.RetentionPreview(context.Background(), p.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Keep) != 1 || len(plan.Delete) != 3 {
		t.Errorf("plan = %+v, want 1 kept and 3 deleted", plan)
	}

	in.Retention = Retention{}
	plan, err = svc.RetentionPreview(context.Background(), p.ID, in)
	if err != nil || len(plan.Delete) != 0 {
		t.Errorf("without rules: plan = %+v, %v", plan, err)
	}
}

func TestServiceRetentionPreviewRefuses(t *testing.T) {
	svc := newTestService()
	p := mustCreate(t, svc, everyMinute("A", "demo"))

	in := everyMinute("A", "demo")
	in.Retention = Retention{Daily: -1}
	if _, err := svc.RetentionPreview(context.Background(), p.ID, in); err == nil {
		t.Error("an invalid retention must be refused")
	}
	in.Retention = Retention{Last: 1}
	if _, err := svc.RetentionPreview(context.Background(), "p-nope", in); err == nil {
		t.Error("an unknown policy must be refused")
	}
}

func TestServiceStoresRetention(t *testing.T) {
	svc := newTestService()
	in := everyMinute("A", "demo")
	in.Retention = Retention{Last: 2, Daily: 7}
	created := mustCreate(t, svc, in)
	if created.Retention != in.Retention {
		t.Errorf("created.Retention = %+v", created.Retention)
	}

	in.Retention = Retention{Monthly: 3}
	updated, err := svc.Update(context.Background(), created.ID, in)
	if err != nil || updated.Retention != in.Retention {
		t.Errorf("updated = %+v, %v", updated.Retention, err)
	}

	in.Retention = Retention{Yearly: MaxKeep + 1}
	if _, err := svc.Update(context.Background(), created.ID, in); err == nil {
		t.Error("an invalid retention must be refused")
	}
}
