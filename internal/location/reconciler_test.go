package location

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"restora.local/restora/internal/snapshot"
)

type fakeProber struct {
	calls   int
	outcome *Outcome
}

func (f *fakeProber) Probe(context.Context, *LocationProfile) *Outcome {
	f.calls++
	return f.outcome
}

func okOutcome() *Outcome {
	return &Outcome{Checks: []snapshot.StepStatus{{ID: CheckConnect, Phase: snapshot.PhaseSucceeded}}}
}

func newTestReconciler(store *Store, s3, nfs Prober, now time.Time) *Reconciler {
	return &Reconciler{Store: store, S3: s3, NFS: nfs, Log: discardLog(), Now: func() time.Time { return now }}
}

func TestReconcilerTestsAfterCreateAndRecordsTheResult(t *testing.T) {
	w := newWorld()
	created := mustCreate(t, w.svc, s3Input("MinIO"))
	s3 := &fakeProber{outcome: okOutcome()}
	r := newTestReconciler(w.store, s3, &fakeProber{}, testNow.Add(2*time.Second))

	if _, err := r.Reconcile(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}

	view, _ := w.svc.Get(context.Background(), created.ID)
	if s3.calls != 1 || view.Test.State != StateSucceeded || view.Test.At == nil || view.Test.DurationMs != 0 {
		t.Errorf("calls = %d, test = %+v", s3.calls, view.Test)
	}
	if len(view.Test.Checks) != 1 {
		t.Errorf("checks = %+v", view.Test.Checks)
	}

	// A second look finds the result up to date and does not test again.
	if _, err := r.Reconcile(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	if s3.calls != 1 {
		t.Errorf("calls = %d, want the result to be kept until somebody asks again", s3.calls)
	}
}

func TestReconcilerRecordsAFailure(t *testing.T) {
	w := newWorld()
	created := mustCreate(t, w.svc, nfsInput("Share"))
	failed := newOutcome(CheckMount, CheckWrite).fail(CheckMount, CodeNFSMountFailed, "connection timed out", "Check the server.")
	nfs := &fakeProber{outcome: failed}

	if _, err := newTestReconciler(w.store, &fakeProber{}, nfs, testNow).Reconcile(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}

	view, _ := w.svc.Get(context.Background(), created.ID)
	if view.Test.State != StateFailed || view.Test.ErrorCode != CodeNFSMountFailed || view.Test.Message != "connection timed out" || view.Test.Hint == "" {
		t.Errorf("test = %+v", view.Test)
	}
}

func TestReconcilerTestsAgainWhenAsked(t *testing.T) {
	w := newWorld()
	created := mustCreate(t, w.svc, nfsInput("Share"))
	nfs := &fakeProber{outcome: okOutcome()}
	r := newTestReconciler(w.store, &fakeProber{}, nfs, testNow)
	if _, err := r.Reconcile(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}

	w.svc.now = func() time.Time { return testNow.Add(time.Minute) }
	if _, err := w.svc.RequestTest(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	if view, _ := w.svc.Get(context.Background(), created.ID); view.Test.State != StatePending {
		t.Fatalf("state = %q, want pending until the controller has answered", view.Test.State)
	}
	if _, err := r.Reconcile(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}

	if nfs.calls != 2 {
		t.Errorf("calls = %d, want 2", nfs.calls)
	}
	if view, _ := w.svc.Get(context.Background(), created.ID); view.Test.State != StateSucceeded {
		t.Errorf("state = %q", view.Test.State)
	}
}

func TestReconcilerTestsAProfileWrittenByHand(t *testing.T) {
	store := newStore()
	if _, err := store.Create(context.Background(), &LocationProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "l-hand", Namespace: testNamespace},
		Spec:       Spec{Name: "Von Hand", Type: TypeNFS, NFS: &NFSSpec{Server: "nfs", Path: "/x"}},
	}); err != nil {
		t.Fatal(err)
	}
	nfs := &fakeProber{outcome: okOutcome()}

	if _, err := newTestReconciler(store, &fakeProber{}, nfs, testNow).Reconcile(context.Background(), "l-hand"); err != nil {
		t.Fatal(err)
	}
	if nfs.calls != 1 {
		t.Errorf("calls = %d; a profile without testRequestedAt is tested once", nfs.calls)
	}
}

func TestReconcilerIgnoresDeletedAndUnknown(t *testing.T) {
	r := newTestReconciler(newStore(), &fakeProber{}, &fakeProber{}, testNow)
	if wait, err := r.Reconcile(context.Background(), "l-gone"); err != nil || wait != 0 {
		t.Errorf("Reconcile = %v, %v", wait, err)
	}
}
