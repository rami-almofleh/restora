// Package policy runs backups on a schedule. The API creates Policy resources, the controller starts a snapshot
// for every selected application when the schedule says so.
package policy

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"restora.local/restora/internal/snapshot"
)

const (
	// Kind identifies the Policy custom resource. Group and version are shared with the other Restora resources.
	Kind = "Policy"
)

// GVR addresses the Policy custom resource.
var GVR = schema.GroupVersionResource{Group: snapshot.Group, Version: snapshot.Version, Resource: "policies"}

// Policy is the custom resource.
type Policy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              Spec   `json:"spec"`
	Status            Status `json:"status,omitempty"`
}

// Spec says what to back up and when.
type Spec struct {
	// Name is the title shown to people. The resource name is a generated id.
	Name     string   `json:"name"`
	Selector Selector `json:"selector"`
	Schedule Schedule `json:"schedule"`
	// ExportTo lists the storage locations (LocationProfile names) every backup of the policy is copied to.
	// Without any, the backups stay in the cluster.
	ExportTo []string `json:"exportTo,omitempty"`
	// Retention says which backups of the policy are kept. All zero keeps everything.
	Retention Retention `json:"retention"`
	// Paused stops new runs. Resuming does not catch up on missed runs.
	Paused bool `json:"paused,omitempty"`
}

// Status reports what the controller did.
type Status struct {
	// ObservedGeneration is the spec generation the schedule was last computed for.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// NextRunAt is unset while the policy is paused or invalid.
	NextRunAt *metav1.Time `json:"nextRunAt,omitempty"`
	LastRun   *RunResult   `json:"lastRun,omitempty"`
	// LastCleanup is the latest time retention deleted backups.
	LastCleanup *Cleanup `json:"lastCleanup,omitempty"`
	// ErrorCode and Hint are set when the policy cannot run, for example invalid_schedule.
	ErrorCode string `json:"errorCode,omitempty"`
	Message   string `json:"message,omitempty"`
	Hint      string `json:"hint,omitempty"`
}

// RunResult is the outcome of the latest scheduled run.
type RunResult struct {
	At metav1.Time `json:"at"`
	// Snapshots lists the snapshots that were started.
	Snapshots []string  `json:"snapshots"`
	Skipped   []Skipped `json:"skipped"`
}

// Cleanup is what retention did last.
type Cleanup struct {
	At      metav1.Time `json:"at"`
	Deleted int         `json:"deleted"`
}

// Skipped is an application a run left out, with a stable reason code.
type Skipped struct {
	Application string `json:"application"`
	// Reason is snapshot_in_progress, restore_in_progress, application_missing or request_failed.
	Reason string `json:"reason"`
}

// Reasons a run skips an application.
const (
	ReasonSnapshotInProgress = "snapshot_in_progress"
	ReasonRestoreInProgress  = "restore_in_progress"
	ReasonApplicationMissing = "application_missing"
	ReasonRequestFailed      = "request_failed"
)

// Error codes in Status.ErrorCode.
const (
	ErrorInvalidSchedule  = "invalid_schedule"
	ErrorInvalidSelector  = "invalid_selector"
	ErrorInvalidRetention = "invalid_retention"
)
