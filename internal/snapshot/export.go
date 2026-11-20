package snapshot

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// exportTarget is one kind of namespaced resource that belongs to an application.
type exportTarget struct {
	gvr  schema.GroupVersionResource
	kind string
}

// exportTargets lists what is saved besides the volumes. Pods, ReplicaSets and everything else that a
// controller owns are left out: they come back when their owner is restored.
var exportTargets = []exportTarget{
	{schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, "ConfigMap"},
	{schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, "Secret"},
	{schema.GroupVersionResource{Version: "v1", Resource: "serviceaccounts"}, "ServiceAccount"},
	{schema.GroupVersionResource{Version: "v1", Resource: "services"}, "Service"},
	{schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}, "PersistentVolumeClaim"},
	{schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, "Deployment"},
	{schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, "StatefulSet"},
	{schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, "DaemonSet"},
	{schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}, "CronJob"},
	{schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}, "Job"},
	{schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}, "Ingress"},
	{schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}, "NetworkPolicy"},
	{schema.GroupVersionResource{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}, "HorizontalPodAutoscaler"},
	{schema.GroupVersionResource{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"}, "PodDisruptionBudget"},
	{schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"}, "Role"},
	{schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}, "RoleBinding"},
}

const bundleFile = "resources.json.gz"

// Bundle is the file format of an export.
type Bundle struct {
	Application string           `json:"application"`
	ExportedAt  time.Time        `json:"exportedAt"`
	Items       []map[string]any `json:"items"`
}

// Exporter saves the Kubernetes resources of an application to files below dir.
// The files hold Secrets in clear text until export encryption exists, so they are readable by the owner only.
type Exporter struct {
	client dynamic.Interface
	dir    string
	now    func() time.Time
}

// NewExporter creates an Exporter writing below dir.
func NewExporter(client dynamic.Interface, dir string) *Exporter {
	return &Exporter{client: client, dir: dir, now: time.Now}
}

// Export saves all resources of the application as one compressed file per snapshot.
func (e *Exporter) Export(ctx context.Context, snapshotName, application string) (ResourceStats, error) {
	b := Bundle{Application: application, ExportedAt: e.now().UTC(), Items: []map[string]any{}}
	for _, target := range exportTargets {
		list, err := e.client.Resource(target.gvr).Namespace(application).List(ctx, metav1.ListOptions{})
		if apierrors.IsNotFound(err) {
			continue // this kind does not exist in the cluster
		}
		if err != nil {
			return ResourceStats{}, fmt.Errorf("list %s in %s: %w", target.gvr.Resource, application, err)
		}
		for i := range list.Items {
			if item := cleanForExport(&list.Items[i], target.kind); item != nil {
				b.Items = append(b.Items, item)
			}
		}
	}

	size, err := e.write(snapshotName, b)
	if err != nil {
		return ResourceStats{}, err
	}
	return ResourceStats{Count: len(b.Items), Bytes: size}, nil
}

// ListKinds maps every exported resource to its list kind. Fake clients need it to serve list calls.
func ListKinds() map[schema.GroupVersionResource]string {
	kinds := make(map[schema.GroupVersionResource]string, len(exportTargets))
	for _, t := range exportTargets {
		kinds[t.gvr] = t.kind + "List"
	}
	return kinds
}

// Path returns the directory that holds the saved resources of a snapshot or of a staged restore.
func (e *Exporter) Path(name string) string {
	return filepath.Join(e.dir, name)
}

// Dir returns the directory the Exporter writes below.
func (e *Exporter) Dir() string {
	return e.dir
}

// Load reads the export of a snapshot.
func (e *Exporter) Load(snapshotName string) (Bundle, error) {
	f, err := os.Open(filepath.Join(e.dir, snapshotName, bundleFile))
	if err != nil {
		return Bundle{}, fmt.Errorf("open export of %s: %w", snapshotName, err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return Bundle{}, fmt.Errorf("read export of %s: %w", snapshotName, err)
	}
	defer gz.Close()
	var b Bundle
	if err := json.NewDecoder(gz).Decode(&b); err != nil {
		return Bundle{}, fmt.Errorf("decode export of %s: %w", snapshotName, err)
	}
	for i := range b.Items {
		b.Items[i] = normalizeNumbers(b.Items[i]).(map[string]any)
	}
	return b, nil
}

// normalizeNumbers turns whole numbers that JSON decoding made float64 back into int64, which is what the
// Kubernetes client libraries expect (for example for spec.replicas).
func normalizeNumbers(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, item := range t {
			t[k] = normalizeNumbers(item)
		}
	case []any:
		for i, item := range t {
			t[i] = normalizeNumbers(item)
		}
	case float64:
		if t == math.Trunc(t) && math.Abs(t) < 1<<53 {
			return int64(t)
		}
	}
	return v
}

// TargetGVR returns the resource that holds objects of an exported kind.
func TargetGVR(kind string) (schema.GroupVersionResource, bool) {
	for _, t := range exportTargets {
		if t.kind == kind {
			return t.gvr, true
		}
	}
	return schema.GroupVersionResource{}, false
}

// Delete removes the file of a snapshot. A missing file is not an error.
func (e *Exporter) Delete(snapshotName string) error {
	if err := os.RemoveAll(filepath.Join(e.dir, snapshotName)); err != nil {
		return fmt.Errorf("delete export of %s: %w", snapshotName, err)
	}
	return nil
}

// write stores the bundle atomically: a half-written file never carries the final name.
func (e *Exporter) write(snapshotName string, b Bundle) (int64, error) {
	dir := filepath.Join(e.dir, snapshotName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return 0, fmt.Errorf("create export directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, bundleFile+".*")
	if err != nil {
		return 0, fmt.Errorf("create export file: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename

	gz := gzip.NewWriter(tmp)
	if err := json.NewEncoder(gz).Encode(b); err != nil {
		tmp.Close()
		return 0, fmt.Errorf("write export: %w", err)
	}
	if err := gz.Close(); err != nil {
		tmp.Close()
		return 0, fmt.Errorf("write export: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return 0, fmt.Errorf("write export: %w", err)
	}
	final := filepath.Join(dir, bundleFile)
	if err := os.Rename(tmp.Name(), final); err != nil {
		return 0, fmt.Errorf("finish export: %w", err)
	}
	info, err := os.Stat(final)
	if err != nil {
		return 0, fmt.Errorf("stat export: %w", err)
	}
	return info.Size(), nil
}

// cleanForExport removes everything the cluster fills in at runtime so the object can be applied again.
// It returns nil for objects that must not be exported.
func cleanForExport(obj *unstructured.Unstructured, kind string) map[string]any {
	if len(obj.GetOwnerReferences()) > 0 {
		return nil
	}
	if kind == "Secret" {
		if t, _, _ := unstructured.NestedString(obj.Object, "type"); t == "kubernetes.io/service-account-token" {
			return nil
		}
	}
	if kind == "ConfigMap" && obj.GetName() == "kube-root-ca.crt" {
		return nil
	}

	out := obj.DeepCopy()
	out.SetKind(kind)
	out.SetUID("")
	out.SetResourceVersion("")
	out.SetGeneration(0)
	out.SetCreationTimestamp(metav1.Time{})
	out.SetManagedFields(nil)
	out.SetSelfLink("")
	unstructured.RemoveNestedField(out.Object, "status")
	return out.Object
}
