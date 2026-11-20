package snapshot

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
	"restora.local/restora/internal/discovery"
)

const testNamespace = "restora-system"

var testNow = time.Date(2026, 11, 19, 10, 15, 30, 0, time.UTC)

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// newFakeClient registers every resource the code lists, so the fake client does not panic on a list call.
func newFakeClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	kinds := map[schema.GroupVersionResource]string{
		GVR:                    "SnapshotList",
		gvrNamespace:           "NamespaceList",
		gvrClaim:               "PersistentVolumeClaimList",
		gvrVolume:              "PersistentVolumeList",
		gvrVolumeSnapshot:      "VolumeSnapshotList",
		gvrVolumeSnapshotClass: "VolumeSnapshotClassList",
	}
	for _, t := range exportTargets {
		kinds[t.gvr] = t.kind + "List"
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

// demoCluster is a namespace "demo" with one bound CSI volume and a matching snapshot class.
func demoCluster(extra ...runtime.Object) []runtime.Object {
	objs := []runtime.Object{
		object("v1", "Namespace", "", "demo", nil),
		object("v1", "PersistentVolumeClaim", "demo", "data", map[string]any{
			"spec":   map[string]any{"volumeName": "pv-1"},
			"status": map[string]any{"phase": "Bound"},
		}),
		object("v1", "PersistentVolume", "", "pv-1", map[string]any{
			"spec": map[string]any{"csi": map[string]any{"driver": "hostpath.csi.k8s.io"}},
		}),
		object("snapshot.storage.k8s.io/v1", "VolumeSnapshotClass", "", "hostpath-class", map[string]any{
			"driver": "hostpath.csi.k8s.io",
		}),
	}
	return append(objs, extra...)
}

type fakeApps struct{ known map[string]bool }

func (f fakeApps) Get(_ context.Context, name string) (discovery.Detail, error) {
	if !f.known[name] {
		return discovery.Detail{}, discovery.ErrNotFound
	}
	return discovery.Detail{Summary: discovery.Summary{Name: name}}, nil
}

func newReconciler(t *testing.T, client *dynamicfake.FakeDynamicClient) *Reconciler {
	t.Helper()
	return &Reconciler{
		Store:    NewStore(client, testNamespace),
		Volumes:  NewVolumes(client),
		Exporter: NewExporter(client, t.TempDir()),
		Log:      discardLog(),
		Now:      func() time.Time { return testNow },
	}
}

type (
	runtimeObject = runtime.Object
	metaOwner     = metav1.OwnerReference
)
