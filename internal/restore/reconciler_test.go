package restore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clienttesting "k8s.io/client-go/testing"
	"restora.local/restora/internal/snapshot"
)

func ctx() context.Context { return context.Background() }

var (
	gvrDeployment, _ = snapshot.TargetGVR("Deployment")
	gvrConfigMap, _  = snapshot.TargetGVR("ConfigMap")
	gvrService, _    = snapshot.TargetGVR("Service")
	gvrRole, _       = snapshot.TargetGVR("Role")
)

func stepPhases(req *RestoreRequest) map[string]snapshot.Phase {
	out := map[string]snapshot.Phase{}
	for _, s := range req.Status.Steps {
		out[s.ID] = s.Phase
	}
	return out
}

// drift changes the application after the backup: config edited, deployment scaled up, service address changed,
// a new workload that uses the same volume, and a pod for each deployment.
func (w *world) drift() {
	w.t.Helper()
	cm := w.resource(gvrConfigMap, "settings")
	_ = unstructured.SetNestedField(cm.Object, "changed", "data", "k")
	w.updateResource(gvrConfigMap, cm)

	db := w.resource(gvrDeployment, "db")
	_ = unstructured.SetNestedField(db.Object, int64(3), "spec", "replicas")
	w.updateResource(gvrDeployment, db)

	svc := w.resource(gvrService, "db")
	_ = unstructured.SetNestedField(svc.Object, "10.0.0.9", "spec", "clusterIP")
	w.updateResource(gvrService, svc)

	w.put(deployment("late", 2))
	w.put(replicaSetOf("rs-db", "db"))
	w.put(replicaSetOf("rs-late", "late"))
	w.put(podUsing("db-1", "data", "ReplicaSet", "rs-db"))
	w.put(podUsing("late-1", "data", "ReplicaSet", "rs-late"))
}

func (w *world) updateResource(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) {
	w.t.Helper()
	if _, err := w.client.Resource(gvr).Namespace("demo").Update(ctx(), obj, metav1.UpdateOptions{}); err != nil {
		w.t.Fatalf("update %s: %v", obj.GetName(), err)
	}
}

func TestRestoreEndToEnd(t *testing.T) {
	w := newWorld(t)
	w.drift()
	w.request("s-1")

	// Pass 1: workloads are scaled down, but their pods are still there.
	if after := w.reconcile(); after != pollInterval {
		t.Fatalf("requeue = %v, want %v while pods are still running", after, pollInterval)
	}
	req := w.get()
	if len(req.Status.Stopped) != 2 {
		t.Fatalf("stopped = %+v, want db and late", req.Status.Stopped)
	}
	if w.replicas("Deployment", "db") != 0 || w.replicas("Deployment", "late") != 0 {
		t.Fatal("deployments must be scaled to zero before volumes are touched")
	}
	if !w.exists(gvrClaim, "data") {
		t.Fatal("the volume claim must survive until the pods are gone")
	}

	// Pass 2: pods are gone, the old claim is deleted. It is not recreated in the same pass,
	// because a real cluster needs time to remove it.
	w.deleteResource(gvrPod, "db-1")
	w.deleteResource(gvrPod, "late-1")
	w.reconcile()
	if w.exists(gvrClaim, "data") {
		t.Fatal("the old claim must be deleted, and only recreated in a later pass")
	}

	// Pass 3 and on: resources applied, workloads wait for readiness.
	w.reconcile()
	claim := w.resource(gvrClaim, "data")
	if claim.GetAnnotations()[AnnotationRestoredFrom] != "s-1-0" {
		t.Fatalf("claim annotations = %v, want restored-from s-1-0", claim.GetAnnotations())
	}
	source, _, _ := unstructured.NestedStringMap(claim.Object, "spec", "dataSource")
	if source["kind"] != "VolumeSnapshot" || source["name"] != "s-1-0" || source["apiGroup"] != "snapshot.storage.k8s.io" {
		t.Errorf("dataSource = %v", source)
	}
	if _, found, _ := unstructured.NestedString(claim.Object, "spec", "volumeName"); found {
		t.Error("volumeName must be cleared so the claim is provisioned from the snapshot")
	}
	if claim.GetAnnotations()["keep"] != "me" || claim.GetAnnotations()["pv.kubernetes.io/bind-completed"] != "" {
		t.Errorf("annotations = %v: runtime annotations go, others stay", claim.GetAnnotations())
	}

	cm := w.resource(gvrConfigMap, "settings")
	if got, _, _ := unstructured.NestedString(cm.Object, "data", "k"); got != "backup" {
		t.Errorf("config map k = %q, want the backed-up value", got)
	}
	svc := w.resource(gvrService, "db")
	if got, _, _ := unstructured.NestedString(svc.Object, "spec", "clusterIP"); got != "10.0.0.9" {
		t.Errorf("clusterIP = %q, want the live address kept (it cannot change)", got)
	}
	if w.replicas("Deployment", "db") != 1 {
		t.Errorf("db replicas = %d, want 1 from the backup", w.replicas("Deployment", "db"))
	}
	if w.replicas("Deployment", "late") != 2 {
		t.Errorf("late replicas = %d, want 2: not part of the backup, so only started again", w.replicas("Deployment", "late"))
	}

	// Not ready yet: the restore keeps waiting.
	if got := w.get().Status.Phase; got != snapshot.PhaseRunning {
		t.Fatalf("phase = %q, want Running until the workloads are ready", got)
	}
	w.markReady("db", 1)
	w.markReady("late", 2)
	if after := w.reconcile(); after != 0 {
		t.Errorf("requeue after success = %v", after)
	}

	req = w.get()
	if req.Status.Phase != snapshot.PhaseSucceeded || req.Status.FinishedAt == nil {
		t.Fatalf("phase = %q, finishedAt = %v", req.Status.Phase, req.Status.FinishedAt)
	}
	for id, phase := range stepPhases(req) {
		if phase != snapshot.PhaseSucceeded {
			t.Errorf("step %s = %s, want Succeeded", id, phase)
		}
	}
	if r := req.Status.Resources; r == nil || r.Updated == 0 {
		t.Errorf("resources = %+v, want updated resources counted", r)
	}
	if !w.exists(gvrDeployment, "late") {
		t.Error("resources that exist only now must not be deleted")
	}
}

func TestRestoreRecreatesDeletedResources(t *testing.T) {
	w := newWorld(t)
	w.deleteResource(gvrDeployment, "db")
	w.deleteResource(gvrConfigMap, "settings")
	w.deleteResource(gvrClaim, "data")
	w.request("s-1")

	w.reconcile()

	if !w.exists(gvrDeployment, "db") || !w.exists(gvrConfigMap, "settings") || !w.exists(gvrClaim, "data") {
		t.Fatal("deployment, config map and claim must be created again")
	}
	if r := w.get().Status.Resources; r == nil || r.Created < 2 {
		t.Errorf("resources = %+v, want created >= 2", r)
	}
}

func TestRestoreFailures(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(w *world)
		snapshot string
		wantCode string
	}{
		{"snapshot deleted", func(w *world) {}, "gone", "snapshot_not_found"},
		{"snapshot not finished", func(w *world) {
			snap, _ := w.rec.Snapshots.Get(ctx(), "s-1")
			snap.Status.Phase = snapshot.PhaseRunning
			_, _ = w.rec.Snapshots.UpdateStatus(ctx(), snap)
		}, "s-1", "snapshot_not_ready"},
		{"volume snapshot deleted", func(w *world) {
			w.deleteResource(gvrVolumeSnapshot, "s-1-0")
		}, "s-1", "volume_snapshot_missing"},
		{"export lost", func(w *world) {
			_ = os.RemoveAll(filepath.Join(w.exportDir, "s-1"))
		}, "s-1", "export_missing"},
		{"namespace gone", func(w *world) {
			_ = w.client.Resource(gvrNamespace).Delete(ctx(), "demo", metav1.DeleteOptions{})
		}, "s-1", "application_missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			tt.prepare(w)
			w.request(tt.snapshot)

			if after := w.reconcile(); after != 0 {
				t.Errorf("requeue = %v, want 0 for a failed restore", after)
			}
			req := w.get()
			if req.Status.Phase != snapshot.PhaseFailed || req.Status.ErrorCode != tt.wantCode {
				t.Fatalf("phase = %q, code = %q, want Failed with %q", req.Status.Phase, req.Status.ErrorCode, tt.wantCode)
			}
			if req.Status.Message == "" || req.Status.Hint == "" {
				t.Errorf("message %q and hint %q must both explain the failure", req.Status.Message, req.Status.Hint)
			}
			if stepPhases(req)[StepCheck] != snapshot.PhaseFailed {
				t.Errorf("steps = %v, want the check step marked as failed", stepPhases(req))
			}
		})
	}
}

func TestRestoreNothingIsTouchedWhenCheckFails(t *testing.T) {
	w := newWorld(t)
	w.deleteResource(gvrVolumeSnapshot, "s-1-0")
	w.drift()
	w.request("s-1")

	w.reconcile()

	if !w.exists(gvrClaim, "data") || w.replicas("Deployment", "db") != 3 {
		t.Error("a restore that fails its check must leave the application untouched")
	}
}

func TestRestoreRefusesUnmanagedPodAndStartsWorkloadsAgain(t *testing.T) {
	w := newWorld(t)
	w.drift()
	w.request("s-1")
	w.reconcile() // scales db (3) and late (2) to zero

	// A bare pod appears that uses the volume. Nobody would restart it, so the restore must stop.
	w.put(podUsing("manual", "data", "", ""))
	w.reconcile()

	req := w.get()
	if req.Status.Phase != snapshot.PhaseFailed || req.Status.ErrorCode != "volume_in_use" {
		t.Fatalf("phase = %q, code = %q, want Failed with volume_in_use", req.Status.Phase, req.Status.ErrorCode)
	}
	if w.replicas("Deployment", "db") != 3 || w.replicas("Deployment", "late") != 2 {
		t.Errorf("db = %d, late = %d, want the original 3 and 2 back: nothing was destroyed yet",
			w.replicas("Deployment", "db"), w.replicas("Deployment", "late"))
	}
	if !w.exists(gvrClaim, "data") {
		t.Error("the volume claim must still be there")
	}
}

func TestRestoreTimeout(t *testing.T) {
	t.Run("pods never stop", func(t *testing.T) {
		w := newWorld(t)
		w.drift()
		w.request("s-1")
		w.reconcile()

		w.rec.Now = func() time.Time { return testNow.Add(DefaultTimeout + time.Second) }
		w.reconcile()

		req := w.get()
		if req.Status.Phase != snapshot.PhaseFailed || req.Status.ErrorCode != "timeout" {
			t.Fatalf("phase = %q, code = %q, want Failed with timeout", req.Status.Phase, req.Status.ErrorCode)
		}
		if w.replicas("Deployment", "db") != 3 {
			t.Error("a timeout before volumes are touched must start the workloads again")
		}
	})

	t.Run("application never becomes ready", func(t *testing.T) {
		w := newWorld(t)
		w.request("s-1")
		w.reconcile() // deletes the old claim
		w.reconcile() // creates the new one and waits for readiness

		w.rec.Now = func() time.Time { return testNow.Add(DefaultTimeout + time.Second) }
		w.reconcile()

		req := w.get()
		if req.Status.Phase != snapshot.PhaseFailed || req.Status.ErrorCode != "workloads_not_ready" {
			t.Fatalf("phase = %q, code = %q, want Failed with workloads_not_ready", req.Status.Phase, req.Status.ErrorCode)
		}
	})
}

func TestRestoreIsRepeatable(t *testing.T) {
	w := newWorld(t)
	w.request("s-1")
	w.reconcile() // deletes the old claim
	w.reconcile() // creates the new claim and goes on

	// Simulate a controller that stopped before it wrote down that the volume was restored.
	claim := w.resource(gvrClaim, "data")
	claim.SetAnnotations(map[string]string{AnnotationRestoredFrom: "s-1-0", "marker": "created-once"})
	w.updateResource(gvrClaim, claim)
	req := w.get()
	req.Status.Volumes[0].Restored = false
	setStep(req, StepVolumes, snapshot.PhaseRunning)
	if _, err := w.rec.Store.UpdateStatus(ctx(), req); err != nil {
		t.Fatal(err)
	}

	w.reconcile()

	if got := w.resource(gvrClaim, "data").GetAnnotations()["marker"]; got != "created-once" {
		t.Error("the claim was deleted and created again: a repeated pass must recognise its own work")
	}
	if !w.get().Status.Volumes[0].Restored {
		t.Error("the volume must be recorded as restored")
	}
}

func TestFinishedRequestsAreIgnored(t *testing.T) {
	w := newWorld(t)
	w.request("s-1")
	w.reconcile()
	w.reconcile()
	w.markReady("db", 1)
	w.reconcile()
	if w.get().Status.Phase != snapshot.PhaseSucceeded {
		t.Fatal("setup: restore must have succeeded")
	}

	w.deleteResource(gvrConfigMap, "settings")
	if after := w.reconcile(); after != 0 {
		t.Errorf("requeue = %v", after)
	}
	if w.exists(gvrConfigMap, "settings") {
		t.Error("a finished restore must never run again")
	}

	if after, err := w.rec.Reconcile(ctx(), "gone"); err != nil || after != 0 {
		t.Errorf("unknown request: (%v, %v), want (0, nil)", after, err)
	}
}

func TestClaimFromSnapshot(t *testing.T) {
	saved := map[string]any{
		"apiVersion": "v1",
		"kind":       "PersistentVolumeClaim",
		"metadata": map[string]any{
			"name":        "data",
			"annotations": map[string]any{"volume.kubernetes.io/selected-node": "n1", "team": "db"},
			"finalizers":  []any{"kubernetes.io/pvc-protection"},
		},
		"spec": map[string]any{
			"volumeName":       "pv-9",
			"storageClassName": "csi",
			"dataSourceRef":    map[string]any{"kind": "Old"},
			"resources":        map[string]any{"requests": map[string]any{"storage": "1Gi"}},
		},
	}

	t.Run("grows to the snapshot size", func(t *testing.T) {
		claim := claimFromSnapshot(saved, "demo", "vs-1", 3<<30)
		if got, _, _ := unstructured.NestedString(claim.Object, "spec", "resources", "requests", "storage"); got != "3Gi" {
			t.Errorf("storage = %q, want 3Gi (a claim smaller than its snapshot cannot be provisioned)", got)
		}
	})
	t.Run("keeps a larger request", func(t *testing.T) {
		claim := claimFromSnapshot(saved, "demo", "vs-1", 1<<20)
		if got, _, _ := unstructured.NestedString(claim.Object, "spec", "resources", "requests", "storage"); got != "1Gi" {
			t.Errorf("storage = %q, want 1Gi", got)
		}
	})
	t.Run("clears bindings and sets the source", func(t *testing.T) {
		claim := claimFromSnapshot(saved, "demo", "vs-1", 0)
		if len(claim.GetFinalizers()) != 0 {
			t.Errorf("finalizers = %v", claim.GetFinalizers())
		}
		if _, found, _ := unstructured.NestedFieldNoCopy(claim.Object, "spec", "dataSourceRef"); found {
			t.Error("dataSourceRef must be cleared")
		}
		if claim.GetNamespace() != "demo" || claim.GetLabels()[snapshot.LabelManagedBy] != snapshot.ManagedByValue {
			t.Errorf("namespace %q, labels %v", claim.GetNamespace(), claim.GetLabels())
		}
		if a := claim.GetAnnotations(); a["team"] != "db" || a["volume.kubernetes.io/selected-node"] != "" {
			t.Errorf("annotations = %v", a)
		}
	})
	t.Run("does not change the saved copy", func(t *testing.T) {
		_ = claimFromSnapshot(saved, "demo", "vs-1", 3<<30)
		if got := saved["spec"].(map[string]any)["volumeName"]; got != "pv-9" {
			t.Errorf("saved spec was modified: volumeName = %v", got)
		}
	})
}

func TestRestoreReportsForbiddenRolesAsWarnings(t *testing.T) {
	w := newWorld(t)
	w.put(object("rbac.authorization.k8s.io/v1", "Role", "demo", "reader", map[string]any{"rules": []any{}}))
	// The role is part of this backup, so export again, then make writing roles fail like a cluster without "escalate".
	if _, err := w.rec.Exports.Export(ctx(), "s-1", "demo"); err != nil {
		t.Fatal(err)
	}
	w.deleteResource(gvrRole, "reader")
	w.client.PrependReactor("create", "roles", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "rbac.authorization.k8s.io", Resource: "roles"}, "reader", errors.New("escalation"))
	})
	w.request("s-1")

	w.reconcile()
	w.reconcile()

	req := w.get()
	if req.Status.Phase == snapshot.PhaseFailed {
		t.Fatalf("a role that cannot be written must not fail the restore: %s", req.Status.Message)
	}
	if len(req.Status.Warnings) != 1 || !strings.Contains(req.Status.Warnings[0], "Role/reader") {
		t.Errorf("warnings = %v, want one naming Role/reader", req.Status.Warnings)
	}
	if req.Status.Resources == nil || req.Status.Resources.Skipped != 1 {
		t.Errorf("resources = %+v, want 1 skipped", req.Status.Resources)
	}
}
