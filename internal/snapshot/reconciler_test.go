package snapshot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func createSnapshot(t *testing.T, r *Reconciler, name string, spec Spec) {
	t.Helper()
	snap := &Snapshot{ObjectMeta: metav1.ObjectMeta{Name: name}, Spec: spec}
	if _, err := r.Store.Create(context.Background(), snap); err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
}

func reconcileOnce(t *testing.T, r *Reconciler, name string) time.Duration {
	t.Helper()
	after, err := r.Reconcile(context.Background(), name)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return after
}

func markVolumeSnapshotReady(t *testing.T, r *Reconciler, application, name string) {
	t.Helper()
	client := r.Volumes.client.Resource(gvrVolumeSnapshot).Namespace(application)
	obj, err := client.Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get volume snapshot: %v", err)
	}
	_ = unstructured.SetNestedField(obj.Object, true, "status", "readyToUse")
	_ = unstructured.SetNestedField(obj.Object, "1Gi", "status", "restoreSize")
	if _, err := client.UpdateStatus(context.Background(), obj, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update volume snapshot: %v", err)
	}
}

func TestReconcileSucceeds(t *testing.T) {
	r := newReconciler(t, newFakeClient(demoCluster()...))
	createSnapshot(t, r, "s-1", Spec{Application: "demo"})

	if after := reconcileOnce(t, r, "s-1"); after != pollInterval {
		t.Errorf("requeue after start = %v, want %v", after, pollInterval)
	}
	snap, _ := r.Store.Get(context.Background(), "s-1")
	if snap.Status.Phase != PhaseRunning || len(snap.Status.Volumes) != 1 {
		t.Fatalf("after start: phase %q, volumes %v", snap.Status.Phase, snap.Status.Volumes)
	}
	if snap.Status.Volumes[0].ClassName != "hostpath-class" || snap.Status.Volumes[0].Claim != "data" {
		t.Errorf("volume = %+v", snap.Status.Volumes[0])
	}
	if !contains(snap.Finalizers, Finalizer) {
		t.Errorf("finalizer missing: %v", snap.Finalizers)
	}
	if _, err := os.Stat(filepath.Join(r.Exporter.dir, "s-1", bundleFile)); err != nil {
		t.Errorf("export file missing: %v", err)
	}

	// Still not ready: keep polling.
	if after := reconcileOnce(t, r, "s-1"); after != pollInterval {
		t.Errorf("requeue while waiting = %v, want %v", after, pollInterval)
	}

	markVolumeSnapshotReady(t, r, "demo", "s-1-0")
	if after := reconcileOnce(t, r, "s-1"); after != 0 {
		t.Errorf("requeue after success = %v, want 0", after)
	}
	snap, _ = r.Store.Get(context.Background(), "s-1")
	if snap.Status.Phase != PhaseSucceeded || snap.Status.FinishedAt == nil {
		t.Fatalf("phase = %q, finishedAt = %v", snap.Status.Phase, snap.Status.FinishedAt)
	}
	if !snap.Status.Volumes[0].Ready || snap.Status.Volumes[0].SizeBytes != 1<<30 {
		t.Errorf("volume = %+v", snap.Status.Volumes[0])
	}
	for _, step := range snap.Status.Steps {
		if step.Phase != PhaseSucceeded {
			t.Errorf("step %s = %s, want Succeeded", step.ID, step.Phase)
		}
	}
	if len(snap.Status.Conditions) != 1 || snap.Status.Conditions[0].Status != metav1.ConditionTrue {
		t.Errorf("conditions = %v", snap.Status.Conditions)
	}

	// A finished snapshot is never touched again.
	if after := reconcileOnce(t, r, "s-1"); after != 0 {
		t.Errorf("requeue for finished snapshot = %v", after)
	}
}

func TestReconcileWithoutVolumes(t *testing.T) {
	r := newReconciler(t, newFakeClient(object("v1", "Namespace", "", "demo", nil)))
	createSnapshot(t, r, "s-1", Spec{Application: "demo"})

	reconcileOnce(t, r, "s-1")

	snap, _ := r.Store.Get(context.Background(), "s-1")
	if snap.Status.Phase != PhaseSucceeded {
		t.Errorf("phase = %q, want Succeeded: an application without volumes only has resources to save", snap.Status.Phase)
	}
}

func TestReconcileFailures(t *testing.T) {
	withoutClass := func() []runtimeObject {
		objs := demoCluster()
		return objs[:3] // drop the VolumeSnapshotClass
	}
	tests := []struct {
		name     string
		objects  []runtimeObject
		spec     Spec
		wantCode string
	}{
		{"no snapshot class", withoutClass(), Spec{Application: "demo"}, "snapshot_class_missing"},
		{"unknown class override", demoCluster(), Spec{Application: "demo", VolumeSnapshotClassName: "nope"}, "snapshot_class_missing"},
		{"namespace missing", demoCluster(), Spec{Application: "gone"}, "application_missing"},
		{
			"volume not bound",
			append(demoCluster()[:1:1], object("v1", "PersistentVolumeClaim", "demo", "data", map[string]any{
				"status": map[string]any{"phase": "Pending"},
			})),
			Spec{Application: "demo"},
			"volume_not_bound",
		},
		{
			"volume without csi",
			[]runtimeObject{
				object("v1", "Namespace", "", "demo", nil),
				object("v1", "PersistentVolumeClaim", "demo", "data", map[string]any{
					"spec": map[string]any{"volumeName": "pv-1"}, "status": map[string]any{"phase": "Bound"},
				}),
				object("v1", "PersistentVolume", "", "pv-1", map[string]any{"spec": map[string]any{"hostPath": map[string]any{"path": "/x"}}}),
			},
			Spec{Application: "demo"},
			"volume_not_snapshottable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newReconciler(t, newFakeClient(tt.objects...))
			createSnapshot(t, r, "s-1", tt.spec)

			if after := reconcileOnce(t, r, "s-1"); after != 0 {
				t.Errorf("requeue = %v, want 0 for a failed snapshot", after)
			}
			snap, _ := r.Store.Get(context.Background(), "s-1")
			if snap.Status.Phase != PhaseFailed || snap.Status.ErrorCode != tt.wantCode {
				t.Fatalf("phase = %q, code = %q, want Failed with %q", snap.Status.Phase, snap.Status.ErrorCode, tt.wantCode)
			}
			if snap.Status.Message == "" || snap.Status.Hint == "" {
				t.Errorf("message %q and hint %q must both explain the failure", snap.Status.Message, snap.Status.Hint)
			}
			if _, err := os.Stat(filepath.Join(r.Exporter.dir, "s-1")); !os.IsNotExist(err) {
				t.Errorf("export of a failed snapshot must be removed, stat error = %v", err)
			}
		})
	}
}

func TestReconcileVolumeErrorFailsAfterGrace(t *testing.T) {
	r := newReconciler(t, newFakeClient(demoCluster()...))
	createSnapshot(t, r, "s-1", Spec{Application: "demo"})
	reconcileOnce(t, r, "s-1")

	client := r.Volumes.client.Resource(gvrVolumeSnapshot).Namespace("demo")
	obj, _ := client.Get(context.Background(), "s-1-0", metav1.GetOptions{})
	_ = unstructured.SetNestedField(obj.Object, "driver is out of space", "status", "error", "message")
	if _, err := client.UpdateStatus(context.Background(), obj, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	reconcileOnce(t, r, "s-1")
	snap, _ := r.Store.Get(context.Background(), "s-1")
	if snap.Status.Phase != PhaseRunning {
		t.Fatalf("phase = %q, want Running: errors inside the grace period are retried by the driver", snap.Status.Phase)
	}

	r.Now = func() time.Time { return testNow.Add(errorGrace + time.Second) }
	reconcileOnce(t, r, "s-1")
	snap, _ = r.Store.Get(context.Background(), "s-1")
	if snap.Status.Phase != PhaseFailed || snap.Status.ErrorCode != "snapshot_failed" {
		t.Errorf("phase = %q, code = %q, want Failed with snapshot_failed", snap.Status.Phase, snap.Status.ErrorCode)
	}
}

func TestReconcileTimeout(t *testing.T) {
	r := newReconciler(t, newFakeClient(demoCluster()...))
	createSnapshot(t, r, "s-1", Spec{Application: "demo"})
	reconcileOnce(t, r, "s-1")

	r.Now = func() time.Time { return testNow.Add(DefaultTimeout + time.Second) }
	reconcileOnce(t, r, "s-1")

	snap, _ := r.Store.Get(context.Background(), "s-1")
	if snap.Status.Phase != PhaseFailed || snap.Status.ErrorCode != "timeout" {
		t.Errorf("phase = %q, code = %q, want Failed with timeout", snap.Status.Phase, snap.Status.ErrorCode)
	}
}

func TestReconcileRepeatsInterruptedStart(t *testing.T) {
	r := newReconciler(t, newFakeClient(demoCluster()...))
	createSnapshot(t, r, "s-1", Spec{Application: "demo"})

	// The controller stopped right after it wrote "Running", before it recorded any volume.
	snap, _ := r.Store.Get(context.Background(), "s-1")
	started := metav1.NewTime(testNow)
	snap.Status = Status{Phase: PhaseRunning, StartedAt: &started}
	if _, err := r.Store.UpdateStatus(context.Background(), snap); err != nil {
		t.Fatal(err)
	}

	reconcileOnce(t, r, "s-1")

	snap, _ = r.Store.Get(context.Background(), "s-1")
	if snap.Status.Phase != PhaseRunning || len(snap.Status.Volumes) != 1 {
		t.Errorf("phase = %q, volumes = %v, want Running with the volume recorded (not a false success)", snap.Status.Phase, snap.Status.Volumes)
	}
}

func TestFinalizeCleansUp(t *testing.T) {
	r := newReconciler(t, newFakeClient(demoCluster()...))
	createSnapshot(t, r, "s-1", Spec{Application: "demo"})
	reconcileOnce(t, r, "s-1")

	// The fake client does not run deletion logic, so mark the object as deleting the way the API server does.
	snap, _ := r.Store.Get(context.Background(), "s-1")
	now := metav1.NewTime(testNow)
	snap.DeletionTimestamp = &now
	if _, err := r.Store.Update(context.Background(), snap); err != nil {
		t.Fatal(err)
	}

	reconcileOnce(t, r, "s-1")

	list, err := r.Volumes.client.Resource(gvrVolumeSnapshot).Namespace("demo").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 0 {
		t.Errorf("volume snapshots left: %d", len(list.Items))
	}
	if _, err := os.Stat(filepath.Join(r.Exporter.dir, "s-1")); !os.IsNotExist(err) {
		t.Errorf("export left behind, stat error = %v", err)
	}
	snap, _ = r.Store.Get(context.Background(), "s-1")
	if contains(snap.Finalizers, Finalizer) {
		t.Errorf("finalizer still set: %v", snap.Finalizers)
	}
}

func TestReconcileUnknownSnapshotIsIgnored(t *testing.T) {
	r := newReconciler(t, newFakeClient())
	if after, err := r.Reconcile(context.Background(), "gone"); err != nil || after != 0 {
		t.Errorf("Reconcile = (%v, %v), want (0, nil)", after, err)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// fakeDriver stands in for the copy to storage locations. Each pass finishes one location.
type fakeDriver struct {
	advanced int
	cleaned  int
	fail     map[string]bool
	err      error
}

func (f *fakeDriver) Advance(_ context.Context, snap *Snapshot) (bool, error) {
	f.advanced++
	if f.err != nil {
		return false, f.err
	}
	for i := range snap.Status.Exports {
		es := &snap.Status.Exports[i]
		if es.Finished() {
			continue
		}
		finished := metav1.NewTime(testNow)
		es.Phase, es.FinishedAt = PhaseSucceeded, &finished
		if f.fail[es.Location] {
			es.Phase, es.ErrorCode = PhaseFailed, "access_denied"
		}
		return false, nil
	}
	return true, nil
}

func (f *fakeDriver) Cleanup(context.Context, *Snapshot) error {
	f.cleaned++
	return nil
}

func snapshotWithExport(t *testing.T, locations ...string) (*Reconciler, *fakeDriver, string) {
	t.Helper()
	r := newReconciler(t, newFakeClient(demoCluster()...))
	driver := &fakeDriver{fail: map[string]bool{}}
	r.Export = driver
	createSnapshot(t, r, "s-1", Spec{Application: "demo", ExportTo: locations})
	return r, driver, "s-1"
}

// reconcileUntilPhase runs the reconciler, marking the volume snapshot ready, until the snapshot reaches the phase.
func reconcileUntilPhase(t *testing.T, r *Reconciler, name string, want Phase) *Snapshot {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 40; i++ {
		if _, err := r.Reconcile(ctx, name); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if snap, _ := r.Store.Get(ctx, name); len(snap.Status.Volumes) > 0 && !snap.Status.Volumes[0].Ready {
			markVolumeSnapshotReady(t, r, "demo", name+"-0")
		}
		snap, err := r.Store.Get(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		if snap.Status.Phase == want {
			return snap
		}
	}
	snap, _ := r.Store.Get(ctx, name)
	t.Fatalf("snapshot did not reach %s: %+v", want, snap.Status)
	return nil
}

func stepPhases(snap *Snapshot) map[string]Phase {
	steps := map[string]Phase{}
	for _, s := range snap.Status.Steps {
		steps[s.ID] = s.Phase
	}
	return steps
}

func TestSnapshotCopiesToStorageLocationsBeforeItIsComplete(t *testing.T) {
	r, driver, name := snapshotWithExport(t, "l-1", "l-2")

	snap := reconcileUntilPhase(t, r, name, PhaseSucceeded)

	if driver.advanced < 3 {
		t.Errorf("advanced = %d, want one pass per location and one to see it is done", driver.advanced)
	}
	if len(snap.Status.Exports) != 2 || snap.Status.Exports[0].Phase != PhaseSucceeded || snap.Status.Exports[1].Phase != PhaseSucceeded {
		t.Errorf("exports = %+v", snap.Status.Exports)
	}
	if steps := stepPhases(snap); steps[StepSnapshotVolumes] != PhaseSucceeded || steps[StepExportToLocation] != PhaseSucceeded {
		t.Errorf("steps = %v", steps)
	}
}

func TestSnapshotWaitsForTheCopyBeforeItSucceeds(t *testing.T) {
	r, driver, name := snapshotWithExport(t, "l-1")
	driver.fail = nil

	// Run until the volume is ready and the copy has begun, then look before it ends.
	for i := 0; i < 10; i++ {
		reconcileOnce(t, r, name)
		if snap, _ := r.Store.Get(context.Background(), name); len(snap.Status.Volumes) > 0 && !snap.Status.Volumes[0].Ready {
			markVolumeSnapshotReady(t, r, "demo", name+"-0")
		}
	}
	snap, _ := r.Store.Get(context.Background(), name)
	if snap.Status.Phase != PhaseRunning && snap.Status.Phase != PhaseSucceeded {
		t.Fatalf("phase = %s", snap.Status.Phase)
	}
	if snap.Status.Phase == PhaseSucceeded && driver.advanced == 0 {
		t.Error("the snapshot succeeded without ever copying")
	}
}

func TestSnapshotStaysUsableWhenACopyFails(t *testing.T) {
	r, driver, name := snapshotWithExport(t, "l-1", "l-2")
	driver.fail["l-2"] = true

	snap := reconcileUntilPhase(t, r, name, PhaseSucceeded)

	if snap.Status.Exports[0].Phase != PhaseSucceeded || snap.Status.Exports[1].Phase != PhaseFailed {
		t.Errorf("exports = %+v", snap.Status.Exports)
	}
	if !strings.Contains(snap.Status.Message, "failed") {
		t.Errorf("message = %q, want it to mention the failed copy", snap.Status.Message)
	}
	if steps := stepPhases(snap); steps[StepExportToLocation] != PhaseFailed || len(snap.Status.Volumes) == 0 {
		t.Errorf("steps = %v, volumes = %d; the local volume snapshots must stay", steps, len(snap.Status.Volumes))
	}
}

func TestSnapshotWithoutADriverFailsOnlyTheCopy(t *testing.T) {
	r, _, name := snapshotWithExport(t, "l-1")
	r.Export = nil

	snap := reconcileUntilPhase(t, r, name, PhaseSucceeded)
	if snap.Status.Exports[0].Phase != PhaseFailed || snap.Status.Exports[0].ErrorCode != "export_unavailable" {
		t.Errorf("exports = %+v", snap.Status.Exports)
	}
}

func TestSnapshotRetriesWhenTheCopyHasAnError(t *testing.T) {
	r, driver, name := snapshotWithExport(t, "l-1")
	driver.err = errors.New("cluster unreachable")

	var failed bool
	for i := 0; i < 12 && !failed; i++ {
		if _, err := r.Reconcile(context.Background(), name); err != nil {
			failed = true
		}
		if snap, _ := r.Store.Get(context.Background(), name); len(snap.Status.Volumes) > 0 && !snap.Status.Volumes[0].Ready {
			markVolumeSnapshotReady(t, r, "demo", name+"-0")
		}
	}
	if !failed {
		t.Error("an error of the driver must be returned so that the pass is retried")
	}
}

func TestDeletingASnapshotAlsoCleansUpTheCopies(t *testing.T) {
	r, driver, name := snapshotWithExport(t, "l-1")
	reconcileOnce(t, r, name)
	snap, err := r.Store.Get(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.cleanup(context.Background(), snap); err != nil {
		t.Fatal(err)
	}
	if driver.cleaned != 1 {
		t.Errorf("cleaned = %d", driver.cleaned)
	}
}

func TestAFailedCopyCanBeRepeated(t *testing.T) {
	r, driver, name := snapshotWithExport(t, "l-1", "l-2")
	driver.fail["l-2"] = true
	reconcileUntilPhase(t, r, name, PhaseSucceeded)
	ctx := context.Background()

	snap, _ := r.Store.Get(ctx, name)
	snap.Spec.ExportRetries = []ExportRetry{{Location: "l-2", RequestedAt: metav1.NewTime(testNow.Add(time.Minute))}}
	if _, err := r.Store.Update(ctx, snap); err != nil {
		t.Fatal(err)
	}
	driver.fail["l-2"] = false
	r.Now = func() time.Time { return testNow.Add(2 * time.Minute) }

	if after := reconcileOnce(t, r, name); after != exportPollInterval {
		t.Errorf("requeue after restart = %v, want %v", after, exportPollInterval)
	}
	snap, _ = r.Store.Get(ctx, name)
	if snap.Status.Phase != PhaseRunning || snap.Status.Exports[0].Phase != PhaseSucceeded || snap.Status.Exports[1].Phase != PhasePending {
		t.Fatalf("after the request: phase %s, exports %+v; only the failed copy starts again", snap.Status.Phase, snap.Status.Exports)
	}

	snap = reconcileUntilPhase(t, r, name, PhaseSucceeded)
	if snap.Status.Exports[1].Phase != PhaseSucceeded || stepPhases(snap)[StepExportToLocation] != PhaseSucceeded {
		t.Errorf("exports = %+v, steps = %v", snap.Status.Exports, stepPhases(snap))
	}
	advanced := driver.advanced
	if after := reconcileOnce(t, r, name); after != 0 || driver.advanced != advanced {
		t.Error("a request must only count once")
	}
}

func TestACopyThatWorkedIsNotRepeated(t *testing.T) {
	r, driver, name := snapshotWithExport(t, "l-1")
	reconcileUntilPhase(t, r, name, PhaseSucceeded)
	ctx := context.Background()
	snap, _ := r.Store.Get(ctx, name)
	snap.Spec.ExportRetries = []ExportRetry{{Location: "l-1", RequestedAt: metav1.NewTime(testNow.Add(time.Minute))}}
	_, _ = r.Store.Update(ctx, snap)
	advanced := driver.advanced

	reconcileOnce(t, r, name)
	if snap, _ = r.Store.Get(ctx, name); snap.Status.Phase != PhaseSucceeded || driver.advanced != advanced {
		t.Errorf("phase = %s: a copy that succeeded must stay as it is", snap.Status.Phase)
	}
}
