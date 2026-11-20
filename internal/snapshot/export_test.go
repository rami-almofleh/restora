package snapshot

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func readBundle(t *testing.T, dir, snapshotName string) Bundle {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, snapshotName, bundleFile))
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	var b Bundle
	if err := json.NewDecoder(gz).Decode(&b); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	return b
}

func TestExport(t *testing.T) {
	owned := object("v1", "ConfigMap", "demo", "owned", nil)
	owned.SetOwnerReferences([]metaOwner{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "rs", UID: "u"}})

	deployment := object("apps/v1", "Deployment", "demo", "web", map[string]any{
		"spec":   map[string]any{"replicas": int64(2)},
		"status": map[string]any{"readyReplicas": int64(2)},
	})
	deployment.SetUID("abc")
	deployment.SetResourceVersion("42")
	deployment.SetGeneration(7)

	client := newFakeClient(
		deployment,
		object("v1", "ConfigMap", "demo", "settings", map[string]any{"data": map[string]any{"k": "v"}}),
		object("v1", "ConfigMap", "demo", "kube-root-ca.crt", nil),
		owned,
		object("v1", "Secret", "demo", "db", map[string]any{"type": "Opaque", "data": map[string]any{"password": "c2VjcmV0"}}),
		object("v1", "Secret", "demo", "default-token", map[string]any{"type": "kubernetes.io/service-account-token"}),
		object("v1", "ConfigMap", "other", "elsewhere", nil),
	)
	dir := t.TempDir()
	stats, err := NewExporter(client, dir).Export(context.Background(), "s-1", "demo")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	b := readBundle(t, dir, "s-1")
	names := map[string]bool{}
	for _, item := range b.Items {
		meta := item["metadata"].(map[string]any)
		names[item["kind"].(string)+"/"+meta["name"].(string)] = true
	}
	want := []string{"Deployment/web", "ConfigMap/settings", "Secret/db"}
	if len(names) != len(want) {
		t.Errorf("exported %v, want exactly %v", names, want)
	}
	for _, w := range want {
		if !names[w] {
			t.Errorf("%s missing from export %v", w, names)
		}
	}
	if stats.Count != 3 || stats.Bytes <= 0 {
		t.Errorf("stats = %+v, want 3 resources and a positive size", stats)
	}
	if b.Application != "demo" {
		t.Errorf("application = %q", b.Application)
	}

	for _, item := range b.Items {
		if item["kind"] != "Deployment" {
			continue
		}
		meta := item["metadata"].(map[string]any)
		for _, field := range []string{"uid", "resourceVersion", "generation", "creationTimestamp", "managedFields"} {
			if v, ok := meta[field]; ok && v != "" && v != nil && v != float64(0) && v != int64(0) {
				t.Errorf("runtime field %s = %v must be removed", field, v)
			}
		}
		if _, ok := item["status"]; ok {
			t.Error("status must be removed")
		}
		if item["spec"].(map[string]any)["replicas"] != float64(2) {
			t.Errorf("spec lost: %v", item["spec"])
		}
	}
}

func TestExportFileIsPrivate(t *testing.T) {
	dir := t.TempDir()
	if _, err := NewExporter(newFakeClient(), dir).Export(context.Background(), "s-1", "demo"); err != nil {
		t.Fatalf("Export: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "s-1", bundleFile))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("export file mode = %v, want no access for group and others (it holds Secrets)", perm)
	}
}

func TestExportDelete(t *testing.T) {
	dir := t.TempDir()
	e := NewExporter(newFakeClient(), dir)
	if _, err := e.Export(context.Background(), "s-1", "demo"); err != nil {
		t.Fatal(err)
	}
	if err := e.Delete("s-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := e.Delete("s-1"); err != nil {
		t.Errorf("deleting twice must not fail: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "s-1")); !os.IsNotExist(err) {
		t.Errorf("directory still exists: %v", err)
	}
}

func TestLoadKeepsWholeNumbersAsIntegers(t *testing.T) {
	deployment := object("apps/v1", "Deployment", "demo", "web", map[string]any{
		"spec": map[string]any{"replicas": int64(2), "ratio": 0.5, "ports": []any{int64(80)}},
	})
	dir := t.TempDir()
	e := NewExporter(newFakeClient(deployment), dir)
	if _, err := e.Export(context.Background(), "s-1", "demo"); err != nil {
		t.Fatalf("Export: %v", err)
	}

	b, err := e.Load("s-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	spec := b.Items[0]["spec"].(map[string]any)
	if got, ok := spec["replicas"].(int64); !ok || got != 2 {
		t.Errorf("replicas = %#v, want int64(2): the client libraries cannot read float64 here", spec["replicas"])
	}
	if got := spec["ratio"]; got != 0.5 {
		t.Errorf("ratio = %#v, want the fraction kept as float64", got)
	}
	if got := spec["ports"].([]any)[0]; got != int64(80) {
		t.Errorf("ports[0] = %#v, want int64(80)", got)
	}
}

func TestLoadMissingExport(t *testing.T) {
	if _, err := NewExporter(newFakeClient(), t.TempDir()).Load("nope"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want it to wrap os.ErrNotExist", err)
	}
}
