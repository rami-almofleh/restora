package location

import (
	"time"

	"restora.local/restora/internal/snapshot"
)

// Test states of a location.
const (
	StatePending   = "pending"
	StateSucceeded = "succeeded"
	StateFailed    = "failed"
)

// View is a location as the REST API returns it. It never contains keys.
type View struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Type Type     `json:"type"`
	S3   *S3View  `json:"s3,omitempty"`
	NFS  *NFSSpec `json:"nfs,omitempty"`
	Test TestView `json:"test"`
	// Exports is how many stored backups the location holds.
	Exports int `json:"exports"`
	// CreatedAt is when the location was created.
	CreatedAt time.Time `json:"createdAt"`
}

// S3View is the S3 part of a View: everything but the keys, plus whether keys are stored.
type S3View struct {
	Endpoint       string `json:"endpoint"`
	Region         string `json:"region,omitempty"`
	Bucket         string `json:"bucket"`
	Prefix         string `json:"prefix,omitempty"`
	PathStyle      bool   `json:"pathStyle"`
	SkipTLSVerify  bool   `json:"skipTLSVerify"`
	CredentialsSet bool   `json:"credentialsSet"`
}

// TestView is the state of the latest connection test.
type TestView struct {
	// State is pending while a test has been asked for and has not finished, then succeeded or failed.
	State      string                `json:"state"`
	At         *time.Time            `json:"at,omitempty"`
	DurationMs int64                 `json:"durationMs,omitempty"`
	Checks     []snapshot.StepStatus `json:"checks"`
	ErrorCode  string                `json:"errorCode,omitempty"`
	Message    string                `json:"message,omitempty"`
	Hint       string                `json:"hint,omitempty"`
}

// Page is one slice of the location list, newest first.
type Page struct {
	Items      []View `json:"items"`
	Total      int    `json:"total"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// NewView converts the custom resource for the API.
func NewView(l *LocationProfile) View {
	v := View{
		ID:        l.Name,
		Name:      l.Spec.Name,
		Type:      l.Spec.Type,
		NFS:       l.Spec.NFS,
		Test:      newTestView(l),
		CreatedAt: l.CreationTimestamp.UTC(),
	}
	if s := l.Spec.S3; s != nil {
		v.S3 = &S3View{
			Endpoint: s.Endpoint, Region: s.Region, Bucket: s.Bucket, Prefix: s.Prefix,
			PathStyle: s.PathStyle, SkipTLSVerify: s.SkipTLSVerify, CredentialsSet: s.CredentialsSecret != "",
		}
	}
	return v
}

func newTestView(l *LocationProfile) TestView {
	last := l.Status.LastTest
	asked := l.Spec.TestRequestedAt
	answered := last != nil && (asked == nil || (last.RequestedAt != nil && last.RequestedAt.Time.Equal(asked.Time)))
	if !answered {
		return TestView{State: StatePending, Checks: []snapshot.StepStatus{}}
	}
	at := last.At.UTC()
	state := StateFailed
	if last.OK {
		state = StateSucceeded
	}
	return TestView{
		State: state, At: &at, DurationMs: last.DurationMs,
		Checks:    append([]snapshot.StepStatus{}, last.Checks...),
		ErrorCode: last.ErrorCode, Message: last.Message, Hint: last.Hint,
	}
}
