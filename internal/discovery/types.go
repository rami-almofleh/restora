package discovery

import "time"

// Health is the overall state of an application.
type Health string

const (
	// HealthRunning means every workload has all desired replicas ready.
	HealthRunning Health = "running"
	// HealthDegraded means at least one workload is missing ready replicas.
	HealthDegraded Health = "degraded"
	// HealthStopped means every workload is scaled to zero or there are no workloads.
	HealthStopped Health = "stopped"
)

// Summary is one row in the application list. An application is a namespace.
type Summary struct {
	Name       string    `json:"name"`
	Health     Health    `json:"health"`
	Engines    []string  `json:"engines"`
	Workloads  int       `json:"workloads"`
	Volumes    int       `json:"volumes"`
	ConfigMaps int       `json:"configMaps"`
	Secrets    int       `json:"secrets"`
	CreatedAt  time.Time `json:"createdAt"`
}

// Detail is a Summary plus everything found in the namespace.
// Secrets and config maps are listed by name only, never by content.
type Detail struct {
	Summary
	WorkloadItems  []Workload `json:"workloadItems"`
	VolumeItems    []Volume   `json:"volumeItems"`
	ConfigMapNames []string   `json:"configMapNames"`
	SecretNames    []string   `json:"secretNames"`
}

// Workload is a Deployment or StatefulSet.
type Workload struct {
	Kind    string   `json:"kind"`
	Name    string   `json:"name"`
	Ready   int32    `json:"ready"`
	Desired int32    `json:"desired"`
	Images  []string `json:"images"`
	Engine  string   `json:"engine,omitempty"`
}

// Volume is a PersistentVolumeClaim.
type Volume struct {
	Name          string `json:"name"`
	StorageClass  string `json:"storageClass,omitempty"`
	CapacityBytes int64  `json:"capacityBytes"`
	Phase         string `json:"phase"`
}

// Filter narrows the application list.
type Filter struct {
	// Query matches application names and detected engines, case-insensitive.
	Query string
	// Volumes is "", "with" or "without".
	Volumes string
}

// Page is one slice of the application list.
type Page struct {
	Items      []Summary `json:"items"`
	Total      int       `json:"total"`
	NextCursor string    `json:"nextCursor,omitempty"`
}
