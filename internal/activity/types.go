// Package activity lists everything Restora did or is doing, across all applications: backups and restores
// in one timeline with filters.
package activity

import (
	"time"

	"restora.local/restora/internal/snapshot"
)

// Type is what kind of action an entry describes.
type Type string

const (
	TypeBackup  Type = "backup"
	TypeRestore Type = "restore"
)

// Status groups the phases for filtering: active covers Pending and Running.
type Status string

const (
	StatusActive    Status = "active"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

// Filter narrows the list. Empty fields select everything.
type Filter struct {
	Type   Type
	Status Status
	// Query matches the application name, case-insensitive.
	Query string
}

// Item is one backup or restore as an entry of the timeline.
type Item struct {
	Type        Type                  `json:"type"`
	ID          string                `json:"id"`
	Application string                `json:"application"`
	Phase       snapshot.Phase        `json:"phase"`
	Message     string                `json:"message,omitempty"`
	ErrorCode   string                `json:"errorCode,omitempty"`
	Hint        string                `json:"hint,omitempty"`
	CreatedAt   time.Time             `json:"createdAt"`
	FinishedAt  *time.Time            `json:"finishedAt,omitempty"`
	Steps       []snapshot.StepStatus `json:"steps"`
	// Policy is set for backups that a policy started.
	Policy *snapshot.PolicyRef `json:"policy,omitempty"`
	// Exports is the copy of a backup to each storage location.
	Exports []snapshot.ExportStatus `json:"exports"`
	// Backup is set for backups, Restore for restores.
	Backup  *BackupDetails  `json:"backup,omitempty"`
	Restore *RestoreDetails `json:"restore,omitempty"`
}

// BackupDetails is what a finished backup contains.
type BackupDetails struct {
	Volumes   int   `json:"volumes"`
	Resources int   `json:"resources"`
	Bytes     int64 `json:"bytes"`
}

// RestoreDetails is where a restore came from and what it did.
type RestoreDetails struct {
	Snapshot string `json:"snapshot"`
	// SnapshotCreatedAt is when the backup was made. It is unset when the backup no longer exists.
	SnapshotCreatedAt *time.Time `json:"snapshotCreatedAt,omitempty"`
	// ExportLocation is the title of the storage location the restore came from. It is set for restores from a
	// storage location, which have no local Snapshot.
	ExportLocation string   `json:"exportLocation,omitempty"`
	Created        int      `json:"created"`
	Updated        int      `json:"updated"`
	Warnings       []string `json:"warnings"`
}

// Counts says how many entries are in each status. It ignores the status filter, so the numbers stay
// the same while the user switches between statuses.
type Counts struct {
	Total     int `json:"total"`
	Active    int `json:"active"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

// Page is one slice of the timeline, newest first.
type Page struct {
	Items []Item `json:"items"`
	// Total is the number of entries that match the whole filter.
	Total      int    `json:"total"`
	Counts     Counts `json:"counts"`
	NextCursor string `json:"nextCursor,omitempty"`
}
