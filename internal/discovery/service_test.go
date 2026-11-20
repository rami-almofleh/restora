package discovery

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

type fakeSource struct {
	namespaces []NamespaceObj
	workloads  []WorkloadObj
	volumes    []VolumeObj
	configMaps []NamedObj
	secrets    []NamedObj
	err        error
}

func (f *fakeSource) Namespaces(context.Context) ([]NamespaceObj, error) { return f.namespaces, f.err }
func (f *fakeSource) Workloads(context.Context) ([]WorkloadObj, error)   { return f.workloads, nil }
func (f *fakeSource) Volumes(context.Context) ([]VolumeObj, error)       { return f.volumes, nil }
func (f *fakeSource) ConfigMaps(context.Context) ([]NamedObj, error)     { return f.configMaps, nil }
func (f *fakeSource) Secrets(context.Context) ([]NamedObj, error)        { return f.secrets, nil }

func ns(names ...string) []NamespaceObj {
	out := make([]NamespaceObj, 0, len(names))
	for _, n := range names {
		out = append(out, NamespaceObj{Name: n, CreatedAt: time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)})
	}
	return out
}

func deploy(namespace, name string, ready, desired int32, images ...string) WorkloadObj {
	return WorkloadObj{Namespace: namespace, Workload: Workload{Kind: "Deployment", Name: name, Ready: ready, Desired: desired, Images: images}}
}

func demoSource() *fakeSource {
	return &fakeSource{
		namespaces: ns("demo", "shop", "empty", "kube-system", "restora-system"),
		workloads: []WorkloadObj{
			deploy("demo", "demo-db", 1, 1, "docker.io/library/postgres:16-alpine"),
			deploy("shop", "web", 1, 2, "nginx:1.27"),
			deploy("shop", "cache", 0, 0, "redis:7"),
			deploy("kube-system", "coredns", 2, 2, "coredns:1.11"),
		},
		volumes: []VolumeObj{
			{Namespace: "demo", Volume: Volume{Name: "demo-db-data", StorageClass: "csi-hostpath-sc", CapacityBytes: 1 << 30, Phase: "Bound"}},
		},
		configMaps: []NamedObj{{Namespace: "demo", Name: "settings"}},
		secrets:    []NamedObj{{Namespace: "demo", Name: "demo-db"}},
	}
}

func TestListFindsApplications(t *testing.T) {
	page, err := New(demoSource()).List(context.Background(), Filter{}, 0, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var names []string
	for _, s := range page.Items {
		names = append(names, s.Name)
	}
	// "empty" holds nothing, system namespaces are hidden.
	if want := []string{"demo", "shop"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}

	demo := page.Items[0]
	if demo.Volumes != 1 || demo.Workloads != 1 || demo.ConfigMaps != 1 || demo.Secrets != 1 {
		t.Errorf("demo counts = %+v", demo)
	}
	if want := []string{"PostgreSQL"}; !reflect.DeepEqual(demo.Engines, want) {
		t.Errorf("demo engines = %v, want %v", demo.Engines, want)
	}
}

func TestHealth(t *testing.T) {
	tests := []struct {
		name      string
		workloads []Workload
		want      Health
	}{
		{"all ready", []Workload{{Ready: 2, Desired: 2}, {Ready: 1, Desired: 1}}, HealthRunning},
		{"one missing", []Workload{{Ready: 1, Desired: 2}}, HealthDegraded},
		{"scaled to zero", []Workload{{Ready: 0, Desired: 0}}, HealthStopped},
		{"no workloads", nil, HealthStopped},
		{"one down one stopped", []Workload{{Ready: 0, Desired: 1}, {Ready: 0, Desired: 0}}, HealthDegraded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (&app{workloads: tt.workloads}).health(); got != tt.want {
				t.Errorf("health = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFilter(t *testing.T) {
	tests := []struct {
		name   string
		filter Filter
		want   []string
	}{
		{"no filter", Filter{}, []string{"demo", "shop"}},
		{"name query", Filter{Query: "DEM"}, []string{"demo"}},
		{"engine query", Filter{Query: "postgres"}, []string{"demo"}},
		{"query without match", Filter{Query: "zzz"}, nil},
		{"with volumes", Filter{Volumes: "with"}, []string{"demo"}},
		{"without volumes", Filter{Volumes: "without"}, []string{"shop"}},
		{"combined", Filter{Query: "shop", Volumes: "with"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := New(demoSource()).List(context.Background(), tt.filter, 0, "")
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			var got []string
			for _, s := range page.Items {
				got = append(got, s.Name)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("names = %v, want %v", got, tt.want)
			}
			if page.Total != len(tt.want) {
				t.Errorf("Total = %d, want %d", page.Total, len(tt.want))
			}
		})
	}
}

func TestPagination(t *testing.T) {
	src := &fakeSource{}
	for i := range 5 {
		name := fmt.Sprintf("app-%d", i)
		src.namespaces = append(src.namespaces, ns(name)...)
		src.workloads = append(src.workloads, deploy(name, "web", 1, 1, "nginx"))
	}
	svc := New(src)

	var got []string
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		page, err := svc.List(context.Background(), Filter{}, 2, cursor)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if page.Total != 5 {
			t.Fatalf("Total = %d, want 5", page.Total)
		}
		for _, s := range page.Items {
			got = append(got, s.Name)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	want := []string{"app-0", "app-1", "app-2", "app-3", "app-4"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("names = %v, want %v", got, want)
	}
}

func TestGet(t *testing.T) {
	svc := New(demoSource())

	detail, err := svc.Get(context.Background(), "demo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(detail.WorkloadItems) != 1 || detail.WorkloadItems[0].Engine != "PostgreSQL" {
		t.Errorf("workloads = %+v", detail.WorkloadItems)
	}
	if len(detail.VolumeItems) != 1 || detail.VolumeItems[0].CapacityBytes != 1<<30 {
		t.Errorf("volumes = %+v", detail.VolumeItems)
	}
	if !reflect.DeepEqual(detail.SecretNames, []string{"demo-db"}) {
		t.Errorf("secrets = %v", detail.SecretNames)
	}

	for _, name := range []string{"missing", "kube-system", "empty"} {
		if _, err := svc.Get(context.Background(), name); !errors.Is(err, ErrNotFound) {
			t.Errorf("Get(%q) error = %v, want ErrNotFound", name, err)
		}
	}
}

func TestSourceErrorIsWrapped(t *testing.T) {
	boom := errors.New("boom")
	_, err := New(&fakeSource{err: boom}).List(context.Background(), Filter{}, 0, "")
	if !errors.Is(err, boom) {
		t.Errorf("error = %v, want it to wrap %v", err, boom)
	}
}

func TestDetectEngine(t *testing.T) {
	tests := []struct {
		image string
		want  string
	}{
		{"postgres:16-alpine", "PostgreSQL"},
		{"docker.io/library/postgres:16", "PostgreSQL"},
		{"registry.example.com:5000/team/mariadb@sha256:abc", "MariaDB"},
		{"bitnami/redis:7", "Redis"},
		{"nginx:1.27", ""},
		{"myorg/postgres-exporter:1", "PostgreSQL"},
	}
	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			if got := detectEngine([]string{tt.image}); got != tt.want {
				t.Errorf("detectEngine = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRefsListApplicationsWithNamespaceLabels(t *testing.T) {
	src := demoSource()
	src.namespaces[0].Labels = map[string]string{"team": "a"}

	refs, err := New(src).Refs(context.Background())
	if err != nil {
		t.Fatalf("Refs: %v", err)
	}

	want := []Ref{{Name: "demo", Labels: map[string]string{"team": "a"}}, {Name: "shop"}}
	if !reflect.DeepEqual(refs, want) {
		t.Errorf("Refs = %+v, want %+v (system and empty namespaces are not applications)", refs, want)
	}
}
