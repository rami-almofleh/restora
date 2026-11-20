package restore

import (
	"time"

	"restora.local/restora/internal/snapshot"
)

// View is a restore as the REST API returns it.
type View struct {
	ID          string `json:"id"`
	Application string `json:"application"`
	Snapshot    string `json:"snapshot,omitempty"`
	// Export is set when the restore comes from a storage location.
	Export     *ExportSource         `json:"export,omitempty"`
	Phase      snapshot.Phase        `json:"phase"`
	Message    string                `json:"message,omitempty"`
	ErrorCode  string                `json:"errorCode,omitempty"`
	Hint       string                `json:"hint,omitempty"`
	CreatedAt  time.Time             `json:"createdAt"`
	FinishedAt *time.Time            `json:"finishedAt,omitempty"`
	Steps      []snapshot.StepStatus `json:"steps"`
	Resources  *ResourceResult       `json:"resources,omitempty"`
	Warnings   []string              `json:"warnings"`
}

// ExportSource says which stored backup a restore came from.
type ExportSource struct {
	ID           string     `json:"id"`
	LocationName string     `json:"locationName,omitempty"`
	CreatedAt    *time.Time `json:"createdAt,omitempty"`
}

// Page is one slice of the restore list, newest first.
type Page struct {
	Items      []View `json:"items"`
	Total      int    `json:"total"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// NewView converts the custom resource for the API. A request the controller has not touched yet is Pending.
func NewView(r *RestoreRequest) View {
	phase := r.Status.Phase
	if phase == "" {
		phase = snapshot.PhasePending
	}
	v := View{
		ID:          r.Name,
		Application: r.Spec.Application,
		Snapshot:    r.Spec.Snapshot,
		Phase:       phase,
		Message:     r.Status.Message,
		ErrorCode:   r.Status.ErrorCode,
		Hint:        r.Status.Hint,
		CreatedAt:   r.CreationTimestamp.UTC(),
		Steps:       append([]snapshot.StepStatus{}, r.Status.Steps...),
		Resources:   r.Status.Resources,
		Warnings:    append([]string{}, r.Status.Warnings...),
	}
	if r.Spec.Export != "" {
		src := &ExportSource{ID: r.Spec.Export, LocationName: r.Spec.ExportLocationName}
		if r.Spec.ExportCreatedAt != nil {
			t := r.Spec.ExportCreatedAt.UTC()
			src.CreatedAt = &t
		}
		v.Export = src
	}
	if r.Status.FinishedAt != nil {
		t := r.Status.FinishedAt.UTC()
		v.FinishedAt = &t
	}
	return v
}

// Active reports whether the restore is still running.
func (v View) Active() bool {
	return v.Phase == snapshot.PhasePending || v.Phase == snapshot.PhaseRunning
}
