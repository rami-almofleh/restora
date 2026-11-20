// Package restore brings an application back from a snapshot. The API creates RestoreRequest resources,
// the controller carries them out.
package restore

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"restora.local/restora/internal/snapshot"
)

const (
	// Kind is the custom resource kind.
	Kind = "RestoreRequest"

	// LabelApplication and LabelSnapshot are set on every request, so lists can be filtered.
	LabelApplication = snapshot.LabelApplication
	LabelSnapshot    = snapshot.LabelSnapshot
	// LabelExport is set on requests that restore from a storage location.
	LabelExport = "restora.local/export"

	// AnnotationRestoredFrom marks a volume claim that Restora created from a VolumeSnapshot.
	// It makes the volume step safe to repeat.
	AnnotationRestoredFrom = "restora.local/restored-from"
)

// GVR addresses the RestoreRequest custom resource.
var GVR = schema.GroupVersionResource{Group: snapshot.Group, Version: snapshot.Version, Resource: "restorerequests"}

// Step ids in the order they run, shown in the interface.
const (
	StepCheck     = "checkSnapshot"
	StepStop      = "stopWorkloads"
	StepVolumes   = "restoreVolumes"
	StepResources = "restoreResources"
	StepReady     = "waitReady"
)

var stepOrder = []string{StepCheck, StepStop, StepVolumes, StepResources, StepReady}

// RestoreRequest is the custom resource.
type RestoreRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              Spec   `json:"spec"`
	Status            Status `json:"status,omitempty"`
}

// Spec says which backup to restore: a local snapshot, or one that is stored in a storage location.
type Spec struct {
	// Snapshot is the name of the Snapshot resource. It is empty when the restore comes from a storage location.
	Snapshot string `json:"snapshot,omitempty"`
	// Export is the name of the Export resource, for a restore from a storage location. Then the application may
	// be gone, with its namespace and its snapshots.
	Export string `json:"export,omitempty"`
	// ExportLocationName and ExportCreatedAt describe the export for the history, even after it is gone.
	ExportLocationName string       `json:"exportLocationName,omitempty"`
	ExportCreatedAt    *metav1.Time `json:"exportCreatedAt,omitempty"`
	// Application is the namespace to restore into: the one the snapshot was taken from.
	Application string `json:"application"`
}

// Status reports progress and result.
type Status struct {
	Phase     snapshot.Phase `json:"phase,omitempty"`
	Message   string         `json:"message,omitempty"`
	ErrorCode string         `json:"errorCode,omitempty"`
	Hint      string         `json:"hint,omitempty"`

	StartedAt  *metav1.Time          `json:"startedAt,omitempty"`
	FinishedAt *metav1.Time          `json:"finishedAt,omitempty"`
	Steps      []snapshot.StepStatus `json:"steps,omitempty"`
	// Stopped lists the workloads that were scaled to zero, with the replica count to return to.
	Stopped   []StoppedWorkload `json:"stopped,omitempty"`
	Volumes   []RestoredVolume  `json:"volumes,omitempty"`
	Resources *ResourceResult   `json:"resources,omitempty"`
	// Warnings lists resources that were left as they are, with the reason.
	Warnings   []string           `json:"warnings,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// StoppedWorkload is a Deployment or StatefulSet that was scaled down for the restore.
type StoppedWorkload struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Replicas int32  `json:"replicas"`
}

// RestoredVolume is the progress of one volume claim.
type RestoredVolume struct {
	Claim string `json:"claim"`
	// VolumeSnapshot is the snapshot the claim is filled from. It is empty for a restore from a storage location.
	VolumeSnapshot string `json:"volumeSnapshot,omitempty"`
	Restored       bool   `json:"restored"`
}

// ResourceResult counts what happened to the saved resources.
type ResourceResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}
