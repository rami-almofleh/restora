package policy

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"restora.local/restora/internal/snapshot"
)

// storedPolicy creates a policy through the service and returns it as the controller sees it.
func storedPolicy(t *testing.T, svc *Service, in Input) *Policy {
	t.Helper()
	v := mustCreate(t, svc, in)
	p, err := svc.store.Get(context.Background(), v.ID)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func reconcile(t *testing.T, r *Reconciler, id string) time.Duration {
	t.Helper()
	d, err := r.Reconcile(context.Background(), id)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return d
}

func TestReconcilerRunsWhenDue(t *testing.T) {
	svc := newTestService()
	p := storedPolicy(t, svc, everyMinute("A", "demo", "shop"))
	backups := &fakeBackups{}
	r := newTestReconciler(svc.store, testApps(), backups, testNow)

	// First look: the clock starts, nothing runs yet.
	if wait := reconcile(t, r, p.Name); wait < time.Second || wait > time.Minute {
		t.Errorf("wait = %v", wait)
	}
	got, _ := svc.store.Get(context.Background(), p.Name)
	wantNext := time.Date(2026, 11, 19, 10, 16, 0, 0, time.UTC)
	if got.Status.NextRunAt == nil || !got.Status.NextRunAt.Time.Equal(wantNext) || len(backups.started) != 0 {
		t.Fatalf("after first look: next = %v, started = %v", got.Status.NextRunAt, backups.started)
	}

	// Before the time: nothing happens.
	r.Now = func() time.Time { return testNow.Add(20 * time.Second) }
	reconcile(t, r, p.Name)
	if len(backups.started) != 0 {
		t.Fatalf("ran too early: %v", backups.started)
	}

	// When due: one snapshot per application, and the next run follows.
	r.Now = func() time.Time { return wantNext.Add(2 * time.Second) }
	reconcile(t, r, p.Name)
	if !slices.Equal(backups.started, []string{"demo", "shop"}) {
		t.Fatalf("started = %v", backups.started)
	}
	got, _ = svc.store.Get(context.Background(), p.Name)
	if got.Status.LastRun == nil || len(got.Status.LastRun.Snapshots) != 2 || len(got.Status.LastRun.Skipped) != 0 {
		t.Errorf("last run = %+v", got.Status.LastRun)
	}
	if next := got.Status.NextRunAt.Time; !next.Equal(wantNext.Add(time.Minute)) {
		t.Errorf("next = %v, want %v", next, wantNext.Add(time.Minute))
	}

	// Looking again at once does not run twice.
	reconcile(t, r, p.Name)
	if len(backups.started) != 2 {
		t.Errorf("ran twice: %v", backups.started)
	}
}

func TestReconcilerDoesNotMakeUpForMissedRuns(t *testing.T) {
	svc := newTestService()
	p := storedPolicy(t, svc, everyMinute("A", "demo"))
	backups := &fakeBackups{}
	r := newTestReconciler(svc.store, testApps(), backups, testNow)
	reconcile(t, r, p.Name)

	// The controller was down for an hour.
	r.Now = func() time.Time { return testNow.Add(time.Hour) }
	reconcile(t, r, p.Name)

	if len(backups.started) != 1 {
		t.Errorf("started = %v, want exactly one run", backups.started)
	}
	got, _ := svc.store.Get(context.Background(), p.Name)
	if next := got.Status.NextRunAt.Time; !next.After(testNow.Add(time.Hour)) {
		t.Errorf("next = %v must lie after the late run", next)
	}
}

func TestReconcilerRecordsSkippedApplications(t *testing.T) {
	svc := newTestService()
	p := storedPolicy(t, svc, everyMinute("A", "demo", "shop", "wiki", "gone"))
	backups := &fakeBackups{refuse: map[string]error{
		"demo": fmt.Errorf("%w: s-1", snapshot.ErrInProgress),
		"shop": fmt.Errorf("%w: restore", snapshot.ErrBlocked),
		"wiki": errBoom,
	}}
	r := newTestReconciler(svc.store, testApps(), backups, testNow)
	reconcile(t, r, p.Name)
	r.Now = func() time.Time { return testNow.Add(2 * time.Minute) }
	reconcile(t, r, p.Name)

	got, _ := svc.store.Get(context.Background(), p.Name)
	want := map[string]string{
		"gone": ReasonApplicationMissing, "demo": ReasonSnapshotInProgress,
		"shop": ReasonRestoreInProgress, "wiki": ReasonRequestFailed,
	}
	if got.Status.LastRun == nil || len(got.Status.LastRun.Snapshots) != 0 || len(got.Status.LastRun.Skipped) != len(want) {
		t.Fatalf("last run = %+v", got.Status.LastRun)
	}
	for _, s := range got.Status.LastRun.Skipped {
		if want[s.Application] != s.Reason {
			t.Errorf("%s skipped for %q, want %q", s.Application, s.Reason, want[s.Application])
		}
	}
}

func TestReconcilerPassesPolicyToSnapshot(t *testing.T) {
	svc := newTestService()
	p := storedPolicy(t, svc, everyMinute("Nightly", "demo"))
	var got snapshot.PolicyRef
	r := newTestReconciler(svc.store, testApps(), backupsFunc(func(by snapshot.PolicyRef) { got = by }), testNow)
	reconcile(t, r, p.Name)
	r.Now = func() time.Time { return testNow.Add(2 * time.Minute) }
	reconcile(t, r, p.Name)

	if got.ID != p.Name || got.Name != "Nightly" {
		t.Errorf("policy ref = %+v", got)
	}
}

type backupsFunc func(by snapshot.PolicyRef)

func (f backupsFunc) Schedule(_ context.Context, application string, by snapshot.PolicyRef, _ []string) (snapshot.View, error) {
	f(by)
	return snapshot.View{ID: "s-" + application}, nil
}

func TestReconcilerPausedPolicyNeverRuns(t *testing.T) {
	svc := newTestService()
	p := storedPolicy(t, svc, everyMinute("A", "demo"))
	backups := &fakeBackups{}
	r := newTestReconciler(svc.store, testApps(), backups, testNow)
	reconcile(t, r, p.Name)

	if _, err := svc.SetPaused(context.Background(), p.Name, true); err != nil {
		t.Fatal(err)
	}
	r.Now = func() time.Time { return testNow.Add(10 * time.Minute) }
	if wait := reconcile(t, r, p.Name); wait != 0 {
		t.Errorf("wait = %v, a paused policy needs no timer", wait)
	}
	got, _ := svc.store.Get(context.Background(), p.Name)
	if len(backups.started) != 0 || got.Status.NextRunAt != nil {
		t.Errorf("started = %v, next = %v", backups.started, got.Status.NextRunAt)
	}

	// Resuming starts the clock from now, without catching up.
	if _, err := svc.SetPaused(context.Background(), p.Name, false); err != nil {
		t.Fatal(err)
	}
	reconcile(t, r, p.Name)
	got, _ = svc.store.Get(context.Background(), p.Name)
	if len(backups.started) != 0 || got.Status.NextRunAt == nil || !got.Status.NextRunAt.Time.After(testNow.Add(10*time.Minute)) {
		t.Errorf("after resume: started = %v, next = %v", backups.started, got.Status.NextRunAt)
	}
}

func TestReconcilerRestartsClockWhenScheduleChanges(t *testing.T) {
	svc := newTestService()
	p := storedPolicy(t, svc, Input{
		Name: "A", Selector: Selector{Mode: SelectAll}, Schedule: Schedule{Type: ScheduleDaily, Hour: 2},
	})
	r := newTestReconciler(svc.store, testApps(), &fakeBackups{}, testNow)
	reconcile(t, r, p.Name)

	// The real API server raises metadata.generation on every spec change; the fake one does not.
	changed, _ := svc.store.Get(context.Background(), p.Name)
	changed.Spec.Schedule = Schedule{Type: ScheduleHourly, Minute: 5}
	changed.Generation++
	if _, err := svc.store.Update(context.Background(), changed); err != nil {
		t.Fatal(err)
	}
	reconcile(t, r, p.Name)

	got, _ := svc.store.Get(context.Background(), p.Name)
	if want := time.Date(2026, 11, 19, 11, 5, 0, 0, time.UTC); !got.Status.NextRunAt.Time.Equal(want) {
		t.Errorf("next = %v, want %v", got.Status.NextRunAt.Time, want)
	}
}

func TestReconcilerRejectsInvalidPolicy(t *testing.T) {
	tests := []struct {
		name     string
		spec     Spec
		wantCode string
	}{
		{"schedule", Spec{Name: "A", Selector: Selector{Mode: SelectAll}, Schedule: Schedule{Type: ScheduleCustom, Cron: "x"}}, ErrorInvalidSchedule},
		{"selector", Spec{Name: "A", Selector: Selector{Mode: SelectLabels}, Schedule: Schedule{Type: ScheduleHourly}}, ErrorInvalidSelector},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Written by hand with kubectl, so the API validation never saw it.
			bad := &Policy{ObjectMeta: metav1.ObjectMeta{Name: "p-hand", Namespace: testNamespace}, Spec: tt.spec}
			store := newStore()
			if _, err := store.Create(context.Background(), bad); err != nil {
				t.Fatal(err)
			}
			backups := &fakeBackups{}
			r := newTestReconciler(store, testApps(), backups, testNow)

			if wait := reconcile(t, r, "p-hand"); wait != 0 {
				t.Errorf("wait = %v, an invalid policy needs no timer", wait)
			}
			got, _ := store.Get(context.Background(), "p-hand")
			if got.Status.ErrorCode != tt.wantCode || got.Status.Message == "" || got.Status.Hint == "" || got.Status.NextRunAt != nil || len(backups.started) != 0 {
				t.Errorf("status = %+v", got.Status)
			}
		})
	}
}

func TestReconcilerIgnoresDeletedPolicy(t *testing.T) {
	r := newTestReconciler(newStore(), testApps(), &fakeBackups{}, testNow)
	if wait, err := r.Reconcile(context.Background(), "p-gone"); err != nil || wait != 0 {
		t.Errorf("Reconcile = %v, %v; want 0, nil", wait, err)
	}
}

func TestReconcilerRetriesWhenClusterFails(t *testing.T) {
	svc := newTestService()
	p := storedPolicy(t, svc, everyMinute("A", "demo"))
	r := newTestReconciler(svc.store, fakeApps{err: errBoom}, &fakeBackups{}, testNow)
	reconcile(t, r, p.Name)

	r.Now = func() time.Time { return testNow.Add(2 * time.Minute) }
	if _, err := r.Reconcile(context.Background(), p.Name); !errors.Is(err, errBoom) {
		t.Errorf("error = %v, want the cluster error so the run is retried", err)
	}
}
