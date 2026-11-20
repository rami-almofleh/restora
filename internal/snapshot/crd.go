// Package snapshot creates and tracks backups of applications. The API creates Snapshot resources,
// the controller carries them out.
package snapshot

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// Group and Version identify the Snapshot custom resource.
	Group   = "restora.local"
	Version = "v1alpha1"
	Kind    = "Snapshot"

	// Finalizer makes sure the volume snapshots and exported files are removed together with the Snapshot.
	Finalizer = "restora.local/snapshot-cleanup"

	// Labels set on everything Restora creates for a snapshot.
	LabelManagedBy   = "app.kubernetes.io/managed-by"
	LabelApplication = "restora.local/application"
	LabelSnapshot    = "restora.local/snapshot"
	LabelPolicy      = "restora.local/policy"
	ManagedByValue   = "restora"
)

// GVR addresses the Snapshot custom resource.
var GVR = schema.GroupVersionResource{Group: Group, Version: Version, Resource: "snapshots"}

// Deleting reports whether the Snapshot is being removed: the controller is still cleaning up behind it.
// Such a snapshot is gone as far as people are concerned.
func (s *Snapshot) Deleting() bool {
	return s.DeletionTimestamp != nil
}

// Phase is the state of a Snapshot. The values are shared by all Restora resources.
type Phase string

const (
	PhasePending   Phase = "Pending"
	PhaseRunning   Phase = "Running"
	PhaseSucceeded Phase = "Succeeded"
	PhaseFailed    Phase = "Failed"
)

// Step ids, shown in the interface in this order.
const (
	StepExportResources = "exportResources"
	StepSnapshotVolumes = "snapshotVolumes"
	// StepExportToLocation copies the backup to the storage locations of the policy. It only exists for backups that have any.
	StepExportToLocation = "exportToLocation"
)

// Snapshot is the custom resource.
type Snapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              Spec   `json:"spec"`
	Status            Status `json:"status,omitempty"`
}

// Spec says what to back up.
type Spec struct {
	// Application is the namespace to back up.
	Application string `json:"application"`
	// VolumeSnapshotClassName overrides the automatic choice by CSI driver.
	VolumeSnapshotClassName string `json:"volumeSnapshotClassName,omitempty"`
	// Policy is set when a policy started the snapshot instead of a person.
	Policy *PolicyRef `json:"policy,omitempty"`
	// ExportTo lists the storage locations (LocationProfile names) the backup is copied to after it was taken.
	ExportTo []string `json:"exportTo,omitempty"`
	// ExportRetries asks the controller to try a failed copy again. The API only records the wish, the controller
	// resets the copy.
	ExportRetries []ExportRetry `json:"exportRetries,omitempty"`
}

// ExportRetry is the wish to copy a backup to one storage location again.
type ExportRetry struct {
	Location    string      `json:"location"`
	RequestedAt metav1.Time `json:"requestedAt"`
}

// PolicyRef names the policy that started a snapshot. The name is the one at that time, so history stays readable
// after a rename.
type PolicyRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Status reports progress and result.
type Status struct {
	Phase Phase `json:"phase,omitempty"`
	// Message is a plain sentence about the current state or the failure.
	Message string `json:"message,omitempty"`
	// ErrorCode is a stable identifier of the failure, for example snapshot_class_missing.
	ErrorCode  string         `json:"errorCode,omitempty"`
	Hint       string         `json:"hint,omitempty"`
	StartedAt  *metav1.Time   `json:"startedAt,omitempty"`
	FinishedAt *metav1.Time   `json:"finishedAt,omitempty"`
	Steps      []StepStatus   `json:"steps,omitempty"`
	Volumes    []VolumeStatus `json:"volumes,omitempty"`
	Resources  *ResourceStats `json:"resources,omitempty"`
	// Exports is the progress and result of the copy to each storage location. A failed export does not fail the
	// snapshot: the local backup is complete and usable.
	Exports    []ExportStatus     `json:"exports,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ExportStatus is the copy of one backup to one storage location.
type ExportStatus struct {
	// Location is the name of the LocationProfile. LocationName is its title when the copy started.
	Location     string `json:"location"`
	LocationName string `json:"locationName,omitempty"`
	Phase        Phase  `json:"phase"`
	// ErrorCode is a stable identifier of the failure, for example repository_password_invalid.
	ErrorCode  string       `json:"errorCode,omitempty"`
	Message    string       `json:"message,omitempty"`
	Hint       string       `json:"hint,omitempty"`
	StartedAt  *metav1.Time `json:"startedAt,omitempty"`
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`
	// ResourcesSnapshot is the id of the stored resources in the repository, empty until they are stored.
	ResourcesSnapshot string `json:"resourcesSnapshot,omitempty"`
	// AddedBytes is how much the repository grew because of this copy. Data that was stored before does not count.
	AddedBytes int64                `json:"addedBytes,omitempty"`
	Volumes    []ExportVolumeStatus `json:"volumes,omitempty"`
}

// ExportVolumeStatus is the copy of one volume.
type ExportVolumeStatus struct {
	Claim string `json:"claim"`
	Phase Phase  `json:"phase"`
	// Pod is the data mover pod while one exists, StartedAt when it was created.
	Pod       string       `json:"pod,omitempty"`
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
	// KopiaSnapshot is the id of the volume in the repository.
	KopiaSnapshot string `json:"kopiaSnapshot,omitempty"`
	SizeBytes     int64  `json:"sizeBytes,omitempty"`
	FileCount     int64  `json:"fileCount,omitempty"`
	AddedBytes    int64  `json:"addedBytes,omitempty"`
	Error         string `json:"error,omitempty"`
}

// Finished reports whether the copy ended, in success or failure.
func (e ExportStatus) Finished() bool {
	return e.Phase == PhaseSucceeded || e.Phase == PhaseFailed
}

// StepStatus is the progress of one step.
type StepStatus struct {
	ID    string `json:"id"`
	Phase Phase  `json:"phase"`
}

// VolumeStatus is the progress of one volume (PersistentVolumeClaim).
type VolumeStatus struct {
	Claim          string `json:"claim"`
	VolumeSnapshot string `json:"volumeSnapshot"`
	ClassName      string `json:"className"`
	Ready          bool   `json:"ready"`
	SizeBytes      int64  `json:"sizeBytes,omitempty"`
	// Error is the last error reported for this volume while it was not ready.
	Error string `json:"error,omitempty"`
}

// ResourceStats describes the exported Kubernetes resources.
type ResourceStats struct {
	Count int   `json:"count"`
	Bytes int64 `json:"bytes"`
}
