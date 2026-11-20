package snapshot

import "time"

// View is the snapshot as the REST API returns it.
type View struct {
	ID          string         `json:"id"`
	Application string         `json:"application"`
	Phase       Phase          `json:"phase"`
	Message     string         `json:"message,omitempty"`
	ErrorCode   string         `json:"errorCode,omitempty"`
	Hint        string         `json:"hint,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	FinishedAt  *time.Time     `json:"finishedAt,omitempty"`
	Steps       []StepStatus   `json:"steps"`
	Volumes     []VolumeStatus `json:"volumes"`
	Resources   *ResourceStats `json:"resources,omitempty"`
	// Policy is set when a policy started the snapshot.
	Policy *PolicyRef `json:"policy,omitempty"`
	// Exports is the copy to each storage location, empty for backups that stay local.
	Exports []ExportStatus `json:"exports"`
}

// Page is one slice of the snapshot list, newest first.
type Page struct {
	Items      []View `json:"items"`
	Total      int    `json:"total"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// NewView converts the custom resource for the API. A resource the controller has not touched yet is Pending.
func NewView(s *Snapshot) View {
	phase := s.Status.Phase
	if phase == "" {
		phase = PhasePending
	}
	v := View{
		ID:          s.Name,
		Application: s.Spec.Application,
		Phase:       phase,
		Message:     s.Status.Message,
		ErrorCode:   s.Status.ErrorCode,
		Hint:        s.Status.Hint,
		CreatedAt:   s.CreationTimestamp.UTC(),
		Steps:       append([]StepStatus{}, s.Status.Steps...),
		Volumes:     append([]VolumeStatus{}, s.Status.Volumes...),
		Resources:   s.Status.Resources,
		Policy:      s.Spec.Policy,
		Exports:     append([]ExportStatus{}, s.Status.Exports...),
	}
	if s.Status.FinishedAt != nil {
		t := s.Status.FinishedAt.UTC()
		v.FinishedAt = &t
	}
	return v
}

// Active reports whether the snapshot is still being created.
func (v View) Active() bool {
	return v.Phase == PhasePending || v.Phase == PhaseRunning
}
