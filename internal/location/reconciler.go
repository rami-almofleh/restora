package location

import (
	"context"
	"errors"
	"log/slog"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Prober tests one location. S3Prober and NFSProber implement it.
type Prober interface {
	Probe(ctx context.Context, l *LocationProfile) *Outcome
}

// Reconciler tests the connection of locations when it is asked to, and once when a location appears.
type Reconciler struct {
	Store *Store
	S3    Prober
	NFS   Prober
	Log   *slog.Logger
	Now   func() time.Time
}

// needsTest reports whether the latest result is older than the latest request.
func needsTest(l *LocationProfile) bool {
	last := l.Status.LastTest
	if last == nil {
		return true
	}
	asked := l.Spec.TestRequestedAt
	return asked != nil && (last.RequestedAt == nil || !last.RequestedAt.Time.Equal(asked.Time))
}

// Reconcile tests a location when a test is due. The result is written to the status.
func (r *Reconciler) Reconcile(ctx context.Context, name string) (time.Duration, error) {
	l, err := r.Store.Get(ctx, name)
	if errors.Is(err, ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if l.DeletionTimestamp != nil || !needsTest(l) {
		return 0, nil
	}

	var prober Prober
	switch l.Spec.Type {
	case TypeS3:
		prober = r.S3
	case TypeNFS:
		prober = r.NFS
	default:
		return 0, nil
	}

	started := r.Now()
	outcome := prober.Probe(ctx, l)
	finished := r.Now()
	r.Log.Info("location tested", "location", l.Name, "type", l.Spec.Type, "ok", outcome.OK(), "code", outcome.Code)

	l.Status.LastTest = &TestResult{
		RequestedAt: l.Spec.TestRequestedAt,
		At:          metav1.NewTime(finished),
		OK:          outcome.OK(),
		DurationMs:  finished.Sub(started).Milliseconds(),
		Checks:      outcome.Checks,
		ErrorCode:   outcome.Code,
		Message:     outcome.Message,
		Hint:        outcome.Hint,
	}
	if _, err := r.Store.UpdateStatus(ctx, l); err != nil && !apierrors.IsNotFound(err) {
		return 0, err
	}
	return 0, nil
}
