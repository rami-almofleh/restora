package discovery

import (
	"context"
	"time"
)

// Source reads the raw objects of a cluster. The Kubernetes implementation lives in kube.go;
// tests use an in-memory one.
type Source interface {
	Namespaces(ctx context.Context) ([]NamespaceObj, error)
	Workloads(ctx context.Context) ([]WorkloadObj, error)
	Volumes(ctx context.Context) ([]VolumeObj, error)
	ConfigMaps(ctx context.Context) ([]NamedObj, error)
	Secrets(ctx context.Context) ([]NamedObj, error)
}

// NamespaceObj is a namespace.
type NamespaceObj struct {
	Name      string
	Labels    map[string]string
	CreatedAt time.Time
}

// WorkloadObj is a Deployment or StatefulSet together with its namespace.
type WorkloadObj struct {
	Namespace string
	Workload
}

// VolumeObj is a PersistentVolumeClaim together with its namespace.
type VolumeObj struct {
	Namespace string
	Volume
}

// NamedObj is an object of which only the name is needed.
type NamedObj struct {
	Namespace string
	Name      string
}
