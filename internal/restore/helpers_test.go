package restore

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"restora.local/restora/internal/offsite"
	"restora.local/restora/internal/snapshot"
)

const testNamespace = "restora-system"

var testNow = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newFakeClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	kinds := snapshot.ListKinds()
	for gvr, kind := range map[schema.GroupVersionResource]string{
		snapshot.GVR:      "SnapshotList",
		GVR:               "RestoreRequestList",
		offsite.GVR:       "ExportList",
		gvrNamespace:      "NamespaceList",
		gvrPod:            "PodList",
		gvrReplicaSet:     "ReplicaSetList",
		gvrVolumeSnapshot: "VolumeSnapshotList",
	} {
		kinds[gvr] = kind
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, objects...)
}

func object(apiVersion, kind, namespace, name string, fields map[string]any) *unstructured.Unstructured {
	meta := map[string]any{"name": name}
	if namespace != "" {
		meta["namespace"] = namespace
	}
	obj := map[string]any{"apiVersion": apiVersion, "kind": kind, "metadata": meta}
	for k, v := range fields {
		obj[k] = v
	}
	return &unstructured.Unstructured{Object: obj}
}

func podUsing(name, claim, ownerKind, ownerName string) *unstructured.Unstructured {
	pod := object("v1", "Pod", "demo", name, map[string]any{
		"spec": map[string]any{"volumes": []any{
			map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": claim}},
		}},
		"status": map[string]any{"phase": "Running"},
	})
	if ownerKind != "" {
		yes := true
		pod.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "apps/v1", Kind: ownerKind, Name: ownerName, UID: "u", Controller: &yes}})
	}
	return pod
}

func replicaSetOf(name, deployment string) *unstructured.Unstructured {
	rs := object("apps/v1", "ReplicaSet", "demo", name, nil)
	yes := true
	rs.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: deployment, UID: "u", Controller: &yes}})
	return rs
}

func deployment(name string, replicas int64) *unstructured.Unstructured {
	return object("apps/v1", "Deployment", "demo", name, map[string]any{
		"spec": map[string]any{"replicas": replicas},
	})
}

// world is a cluster with an application, one backup of it and everything a restore needs.
type world struct {
	t         *testing.T
	client    *dynamicfake.FakeDynamicClient
	rec       *Reconciler
	exportDir string
}

// newWorld builds namespace "demo" as it was at backup time (claim data, deployment db with 1 replica,
// config map, service, secret), exports it as snapshot s-1 and stores the matching VolumeSnapshot.
func newWorld(t *testing.T) *world {
	t.Helper()
	claim := object("v1", "PersistentVolumeClaim", "demo", "data", map[string]any{
		"spec": map[string]any{
			"storageClassName": "csi",
			"volumeName":       "pv-1",
			"resources":        map[string]any{"requests": map[string]any{"storage": "1Gi"}},
		},
		"status": map[string]any{"phase": "Bound"},
	})
	claim.SetAnnotations(map[string]string{"pv.kubernetes.io/bind-completed": "yes", "keep": "me"})
	claim.SetFinalizers([]string{"kubernetes.io/pvc-protection"})

	client := newFakeClient(
		object("v1", "Namespace", "", "demo", nil),
		claim,
		deployment("db", 1),
		object("v1", "ConfigMap", "demo", "settings", map[string]any{"data": map[string]any{"k": "backup"}}),
		object("v1", "Service", "demo", "db", map[string]any{"spec": map[string]any{"clusterIP": "10.0.0.1", "ports": []any{}}}),
		object("v1", "Secret", "demo", "db", map[string]any{"type": "Opaque", "data": map[string]any{"password": "c2VjcmV0"}}),
		object("snapshot.storage.k8s.io/v1", "VolumeSnapshot", "demo", "s-1-0", map[string]any{
			"status": map[string]any{"readyToUse": true},
		}),
	)

	exportDir := t.TempDir()
	exports := snapshot.NewExporter(client, exportDir)
	if _, err := exports.Export(context.Background(), "s-1", "demo"); err != nil {
		t.Fatalf("export: %v", err)
	}

	snaps := snapshot.NewStore(client, testNamespace)
	snap, err := snaps.Create(context.Background(), &snapshot.Snapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "s-1"},
		Spec:       snapshot.Spec{Application: "demo"},
	})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	snap.Status = snapshot.Status{
		Phase: snapshot.PhaseSucceeded,
		Volumes: []snapshot.VolumeStatus{
			{Claim: "data", VolumeSnapshot: "s-1-0", ClassName: "csi", Ready: true, SizeBytes: 2 << 30},
		},
	}
	if _, err := snaps.UpdateStatus(context.Background(), snap); err != nil {
		t.Fatalf("update snapshot: %v", err)
	}

	return &world{t: t, client: client, exportDir: exportDir, rec: &Reconciler{
		Client:    client,
		Store:     NewStore(client, testNamespace),
		Snapshots: snaps,
		Exports:   exports,
		Log:       discardLog(),
		Now:       func() time.Time { return testNow },
	}}
}

func (w *world) request(snapshotName string) string {
	w.t.Helper()
	req, err := w.rec.Store.Create(context.Background(), &RestoreRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "r-1"},
		Spec:       Spec{Snapshot: snapshotName, Application: "demo"},
	})
	if err != nil {
		w.t.Fatalf("create restore: %v", err)
	}
	return req.Name
}

func (w *world) reconcile() time.Duration {
	w.t.Helper()
	after, err := w.rec.Reconcile(context.Background(), "r-1")
	if err != nil {
		w.t.Fatalf("Reconcile: %v", err)
	}
	return after
}

func (w *world) get() *RestoreRequest {
	w.t.Helper()
	req, err := w.rec.Store.Get(context.Background(), "r-1")
	if err != nil {
		w.t.Fatalf("get restore: %v", err)
	}
	return req
}

func (w *world) resource(gvr schema.GroupVersionResource, name string) *unstructured.Unstructured {
	w.t.Helper()
	obj, err := w.client.Resource(gvr).Namespace("demo").Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		w.t.Fatalf("get %s %s: %v", gvr.Resource, name, err)
	}
	return obj
}

func (w *world) exists(gvr schema.GroupVersionResource, name string) bool {
	_, err := w.client.Resource(gvr).Namespace("demo").Get(context.Background(), name, metav1.GetOptions{})
	return err == nil
}

func (w *world) put(obj *unstructured.Unstructured) {
	w.t.Helper()
	gvr, ok := snapshot.TargetGVR(obj.GetKind())
	if obj.GetKind() == "Pod" {
		gvr, ok = gvrPod, true
	}
	if obj.GetKind() == "ReplicaSet" {
		gvr, ok = gvrReplicaSet, true
	}
	if !ok {
		w.t.Fatalf("no resource for kind %s", obj.GetKind())
	}
	if _, err := w.client.Resource(gvr).Namespace("demo").Create(context.Background(), obj, metav1.CreateOptions{}); err != nil {
		w.t.Fatalf("create %s %s: %v", obj.GetKind(), obj.GetName(), err)
	}
}

func (w *world) deleteResource(gvr schema.GroupVersionResource, name string) {
	w.t.Helper()
	if err := w.client.Resource(gvr).Namespace("demo").Delete(context.Background(), name, metav1.DeleteOptions{}); err != nil {
		w.t.Fatalf("delete %s %s: %v", gvr.Resource, name, err)
	}
}

func (w *world) replicas(kind, name string) int64 {
	w.t.Helper()
	gvr, _ := snapshot.TargetGVR(kind)
	n, _, _ := unstructured.NestedInt64(w.resource(gvr, name).Object, "spec", "replicas")
	return n
}

func (w *world) markReady(name string, ready int64) {
	w.t.Helper()
	gvr, _ := snapshot.TargetGVR("Deployment")
	obj := w.resource(gvr, name)
	_ = unstructured.SetNestedField(obj.Object, ready, "status", "readyReplicas")
	if _, err := w.client.Resource(gvr).Namespace("demo").Update(context.Background(), obj, metav1.UpdateOptions{}); err != nil {
		w.t.Fatalf("update deployment: %v", err)
	}
}
