package policy

import "time"

// View is a policy as the REST API returns it.
type View struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Selector  Selector  `json:"selector"`
	Schedule  Schedule  `json:"schedule"`
	Retention Retention `json:"retention"`
	// ExportTo lists the storage locations the backups are copied to.
	ExportTo []string `json:"exportTo"`
	// Cron is the expression the schedule runs, also for the presets.
	Cron   string `json:"cron"`
	Paused bool   `json:"paused"`
	// Applications are the applications the selector takes right now, Missing the named ones that do not exist.
	Applications []string `json:"applications"`
	Missing      []string `json:"missing"`
	// NextRunAt is unset while the policy is paused or invalid.
	NextRunAt *time.Time `json:"nextRunAt,omitempty"`
	LastRun   *LastRun   `json:"lastRun,omitempty"`
	// LastCleanup is set once retention has deleted backups.
	LastCleanup *LastCleanup `json:"lastCleanup,omitempty"`
	ErrorCode   string       `json:"errorCode,omitempty"`
	Message     string       `json:"message,omitempty"`
	Hint        string       `json:"hint,omitempty"`
	CreatedAt   time.Time    `json:"createdAt"`
}

// LastRun is the outcome of the latest scheduled run.
type LastRun struct {
	At        time.Time `json:"at"`
	Snapshots []string  `json:"snapshots"`
	Skipped   []Skipped `json:"skipped"`
}

// LastCleanup is what retention did last.
type LastCleanup struct {
	At      time.Time `json:"at"`
	Deleted int       `json:"deleted"`
}

// Page is one slice of the policy list, newest first.
type Page struct {
	Items      []View `json:"items"`
	Total      int    `json:"total"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// Preview shows what a policy would do, before it is saved.
type Preview struct {
	Cron         string      `json:"cron"`
	NextRuns     []time.Time `json:"nextRuns"`
	Applications []string    `json:"applications"`
	Missing      []string    `json:"missing"`
}

// newView converts the custom resource. The schedule may be invalid when the resource was written by hand;
// then Cron is empty and the status explains.
func newView(p *Policy, applications, missing []string) View {
	v := View{
		ID:           p.Name,
		Name:         p.Spec.Name,
		Selector:     p.Spec.Selector,
		Schedule:     p.Spec.Schedule,
		Retention:    p.Spec.Retention,
		ExportTo:     append([]string{}, p.Spec.ExportTo...),
		Paused:       p.Spec.Paused,
		Applications: applications,
		Missing:      missing,
		ErrorCode:    p.Status.ErrorCode,
		Message:      p.Status.Message,
		Hint:         p.Status.Hint,
		CreatedAt:    p.CreationTimestamp.UTC(),
	}
	if expr, err := p.Spec.Schedule.Expression(); err == nil {
		v.Cron = expr
	}
	if p.Status.NextRunAt != nil && !p.Spec.Paused {
		t := p.Status.NextRunAt.UTC()
		v.NextRunAt = &t
	}
	if last := p.Status.LastRun; last != nil {
		v.LastRun = &LastRun{
			At:        last.At.UTC(),
			Snapshots: append([]string{}, last.Snapshots...),
			Skipped:   append([]Skipped{}, last.Skipped...),
		}
	}
	if c := p.Status.LastCleanup; c != nil {
		v.LastCleanup = &LastCleanup{At: c.At.UTC(), Deleted: c.Deleted}
	}
	return v
}
