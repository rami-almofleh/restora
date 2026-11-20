package snapshot

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"restora.local/restora/internal/discovery"
)

func newTestService() (*Service, *Store) {
	store := NewStore(newFakeClient(), testNamespace)
	svc := NewService(store, fakeApps{known: map[string]bool{"demo": true, "shop": true}})
	svc.now = func() time.Time { return testNow }
	return svc, store
}

func TestCreate(t *testing.T) {
	svc, store := newTestService()
	ctx := context.Background()

	view, err := svc.Create(ctx, "demo")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !regexp.MustCompile(`^s-20261119-101530-[a-z0-9]{4}$`).MatchString(view.ID) {
		t.Errorf("ID = %q, want s-<time>-<4 chars>", view.ID)
	}
	if view.Application != "demo" || view.Phase != PhasePending {
		t.Errorf("view = %+v, want application demo and phase Pending", view)
	}

	stored, err := store.Get(ctx, view.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Labels[LabelApplication] != "demo" || stored.Labels[LabelManagedBy] != ManagedByValue {
		t.Errorf("labels = %v", stored.Labels)
	}
}

func TestCreateErrors(t *testing.T) {
	svc, store := newTestService()
	ctx := context.Background()

	if _, err := svc.Create(ctx, "missing"); !errors.Is(err, discovery.ErrNotFound) {
		t.Errorf("unknown application: error = %v, want discovery.ErrNotFound", err)
	}

	first, err := svc.Create(ctx, "demo")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Create(ctx, "demo"); !errors.Is(err, ErrInProgress) {
		t.Errorf("second snapshot while pending: error = %v, want ErrInProgress", err)
	}
	if _, err := svc.Create(ctx, "shop"); err != nil {
		t.Errorf("another application must not be blocked: %v", err)
	}

	for _, phase := range []Phase{PhaseSucceeded, PhaseFailed} {
		snap, _ := store.Get(ctx, first.ID)
		snap.Status.Phase = phase
		if _, err := store.UpdateStatus(ctx, snap); err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		svc.now = func() time.Time { return testNow.Add(time.Hour) } // new name, so no clash
		next, err := svc.Create(ctx, "demo")
		if err != nil {
			t.Fatalf("Create after %s: %v", phase, err)
		}
		snap, _ = store.Get(ctx, next.ID)
		snap.Status.Phase = PhaseSucceeded
		_, _ = store.UpdateStatus(ctx, snap)
		first = next
	}
}

func TestListNewestFirstWithPaging(t *testing.T) {
	svc, store := newTestService()
	ctx := context.Background()

	var ids []string
	for i := range 5 {
		svc.now = func() time.Time { return testNow.Add(time.Duration(i) * time.Minute) }
		view, err := svc.Create(ctx, "demo")
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
		ids = append(ids, view.ID)
		snap, _ := store.Get(ctx, view.ID)
		snap.Status.Phase = PhaseSucceeded
		_, _ = store.UpdateStatus(ctx, snap)
	}
	if _, err := svc.Create(ctx, "shop"); err != nil {
		t.Fatal(err)
	}

	var got []string
	cursor := ""
	for range 10 {
		page, err := svc.List(ctx, "demo", 2, cursor)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if page.Total != 5 {
			t.Fatalf("Total = %d, want 5 (snapshots of other applications must not count)", page.Total)
		}
		for _, v := range page.Items {
			got = append(got, v.ID)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}

	var want []string
	for i := len(ids) - 1; i >= 0; i-- {
		want = append(want, ids[i])
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("order = %v, want newest first %v", got, want)
	}
}

func TestListEmptyIsNotNull(t *testing.T) {
	svc, _ := newTestService()
	page, err := svc.List(context.Background(), "demo", 0, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if page.Items == nil || len(page.Items) != 0 {
		t.Errorf("Items = %#v, want empty non-nil slice so JSON shows []", page.Items)
	}
}

func TestCreateRunsGuards(t *testing.T) {
	svc, _ := newTestService()
	svc.AddGuard(func(_ context.Context, application string) error {
		if application == "demo" {
			return fmt.Errorf("%w: restore running", ErrBlocked)
		}
		return nil
	})

	if _, err := svc.Create(context.Background(), "demo"); !errors.Is(err, ErrBlocked) {
		t.Errorf("guarded application: error = %v, want ErrBlocked", err)
	}
	if _, err := svc.Create(context.Background(), "shop"); err != nil {
		t.Errorf("other application: %v", err)
	}
}

func TestScheduleMarksThePolicyAndSkipsTheApplicationLookup(t *testing.T) {
	store := NewStore(newFakeClient(), testNamespace)
	svc := NewService(store, nil)
	svc.now = func() time.Time { return testNow }
	ctx := context.Background()

	view, err := svc.Schedule(ctx, "demo", PolicyRef{ID: "p-1", Name: "Nightly"}, []string{"l-1"})
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if view.Policy == nil || view.Policy.ID != "p-1" || view.Policy.Name != "Nightly" {
		t.Errorf("view.Policy = %+v", view.Policy)
	}
	stored, err := store.Get(ctx, view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Labels[LabelPolicy] != "p-1" || stored.Labels[LabelApplication] != "demo" {
		t.Errorf("labels = %v", stored.Labels)
	}
	if len(stored.Spec.ExportTo) != 1 || stored.Spec.ExportTo[0] != "l-1" {
		t.Errorf("exportTo = %v, want the locations of the policy", stored.Spec.ExportTo)
	}

	if _, err := svc.Schedule(ctx, "demo", PolicyRef{ID: "p-1", Name: "Nightly"}, []string{"l-1"}); !errors.Is(err, ErrInProgress) {
		t.Errorf("second scheduled run while the first is active: error = %v, want ErrInProgress", err)
	}
}

func TestScheduleHonoursGuards(t *testing.T) {
	svc := NewService(NewStore(newFakeClient(), testNamespace), nil)
	svc.AddGuard(func(context.Context, string) error { return fmt.Errorf("%w: restore", ErrBlocked) })

	if _, err := svc.Schedule(context.Background(), "demo", PolicyRef{ID: "p-1", Name: "x"}, nil); !errors.Is(err, ErrBlocked) {
		t.Errorf("error = %v, want ErrBlocked", err)
	}
}

func finishSnapshot(t *testing.T, store *Store, id string, phase Phase) {
	t.Helper()
	snap, err := store.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	snap.Status.Phase = phase
	if _, err := store.UpdateStatus(context.Background(), snap); err != nil {
		t.Fatal(err)
	}
}

func TestDelete(t *testing.T) {
	svc, store := newTestService()
	ctx := context.Background()
	view, err := svc.Create(ctx, "demo")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(ctx, view.ID); !errors.Is(err, ErrRunning) {
		t.Fatalf("deleting a running snapshot: error = %v, want ErrRunning", err)
	}

	finishSnapshot(t, store, view.ID, PhaseSucceeded)
	if err := svc.Delete(ctx, view.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, view.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("snapshot still there after delete: %v", err)
	}
	if err := svc.Delete(ctx, view.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting twice: error = %v, want ErrNotFound", err)
	}
}

func TestDeleteOfFailedSnapshotWorks(t *testing.T) {
	svc, store := newTestService()
	ctx := context.Background()
	view, _ := svc.Create(ctx, "demo")
	finishSnapshot(t, store, view.ID, PhaseFailed)

	if err := svc.Delete(ctx, view.ID); err != nil {
		t.Errorf("Delete of a failed snapshot: %v", err)
	}
}

func TestDeleteHonoursGuards(t *testing.T) {
	svc, store := newTestService()
	ctx := context.Background()
	view, _ := svc.Create(ctx, "demo")
	finishSnapshot(t, store, view.ID, PhaseSucceeded)

	var seen string
	svc.AddDeleteGuard(func(_ context.Context, snap *Snapshot) error {
		seen = snap.Name
		return fmt.Errorf("%w: a restore reads from it", ErrInUse)
	})
	if err := svc.Delete(ctx, view.ID); !errors.Is(err, ErrInUse) {
		t.Fatalf("error = %v, want ErrInUse", err)
	}
	if seen != view.ID {
		t.Errorf("guard saw %q, want %q", seen, view.ID)
	}
	if _, err := store.Get(ctx, view.ID); err != nil {
		t.Errorf("a refused delete must keep the snapshot: %v", err)
	}
}

func TestSnapshotBeingDeletedIsHiddenAndDeleteIsIdempotent(t *testing.T) {
	svc, store := newTestService()
	ctx := context.Background()
	keep, _ := svc.Create(ctx, "demo")
	finishSnapshot(t, store, keep.ID, PhaseSucceeded)

	now := metav1.NewTime(testNow)
	gone, err := store.Create(ctx, &Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "s-gone", DeletionTimestamp: &now, Labels: map[string]string{LabelApplication: "demo"},
		},
		Spec: Spec{Application: "demo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !gone.Deleting() {
		t.Skip("the fake client drops the deletion timestamp")
	}

	page, err := svc.List(ctx, "demo", 0, "")
	if err != nil || len(page.Items) != 1 || page.Total != 1 || page.Items[0].ID != keep.ID {
		t.Errorf("list = %+v, %v; want only the snapshot that is not being deleted", page, err)
	}
	if err := svc.Delete(ctx, "s-gone"); err != nil {
		t.Errorf("deleting what is being deleted: %v", err)
	}
}

func TestRetryExportOnlyForFailedCopies(t *testing.T) {
	svc, store := newTestService()
	ctx := context.Background()
	snap := &Snapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "s-1"},
		Spec:       Spec{Application: "demo", ExportTo: []string{"l-1", "l-2"}},
		Status: Status{Phase: PhaseSucceeded, Exports: []ExportStatus{
			{Location: "l-1", Phase: PhaseSucceeded}, {Location: "l-2", Phase: PhaseFailed},
		}},
	}
	created, err := store.Create(ctx, snap)
	if err != nil {
		t.Fatal(err)
	}
	created.Status = snap.Status
	if _, err := store.UpdateStatus(ctx, created); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.RetryExport(ctx, "s-1", "l-1"); !errors.Is(err, ErrNotRetryable) {
		t.Errorf("a copy that worked: err = %v, want ErrNotRetryable", err)
	}
	if _, err := svc.RetryExport(ctx, "s-1", "l-9"); !errors.Is(err, ErrNotRetryable) {
		t.Errorf("an unknown location: err = %v, want ErrNotRetryable", err)
	}
	if _, err := svc.RetryExport(ctx, "s-x", "l-2"); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown backup: err = %v, want ErrNotFound", err)
	}
	if _, err := svc.RetryExport(ctx, "s-1", "l-2"); err != nil {
		t.Fatalf("RetryExport: %v", err)
	}
	if _, err := svc.RetryExport(ctx, "s-1", "l-2"); err != nil {
		t.Fatalf("asking twice: %v", err)
	}
	got, _ := store.Get(ctx, "s-1")
	if len(got.Spec.ExportRetries) != 1 || got.Spec.ExportRetries[0].Location != "l-2" {
		t.Errorf("retries = %+v, want exactly one for l-2", got.Spec.ExportRetries)
	}
}
