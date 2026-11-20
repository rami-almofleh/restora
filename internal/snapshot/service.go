package snapshot

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"restora.local/restora/internal/crstore"
	"restora.local/restora/internal/discovery"
)

// ErrInProgress is returned when an application already has a snapshot being created.
var ErrInProgress = errors.New("snapshot already in progress")

// ErrBlocked is returned when something else, such as a restore, keeps the application from being backed up right now.
var ErrBlocked = errors.New("application is busy")

// ErrRunning is returned when a snapshot that is still being created is to be deleted.
var ErrRunning = errors.New("snapshot is still running")

// ErrNotRetryable is returned when a copy cannot be repeated: the backup or the copy does not exist, or the copy did not fail.
var ErrNotRetryable = errors.New("copy cannot be repeated")

// ErrInUse is returned when something else, such as a restore, still needs the snapshot.
var ErrInUse = errors.New("snapshot is in use")

// DeleteGuard returns an error wrapping ErrInUse when the snapshot must not be deleted right now.
type DeleteGuard func(ctx context.Context, snap *Snapshot) error

// Guard returns an error wrapping ErrBlocked when a new snapshot of the application must not start.
type Guard func(ctx context.Context, application string) error

// Applications looks up applications. discovery.Service implements it.
type Applications interface {
	Get(ctx context.Context, name string) (discovery.Detail, error)
}

// Service is the API side of snapshots: it requests them and reports their state.
type Service struct {
	store  *Store
	apps   Applications
	guards []Guard
	// deleteGuards run before a snapshot is deleted.
	deleteGuards []DeleteGuard
	now          func() time.Time
}

// NewService creates a Service. apps may be nil when only Schedule is used.
func NewService(store *Store, apps Applications) *Service {
	return &Service{store: store, apps: apps, now: time.Now}
}

// AddGuard registers a check that runs before every new snapshot.
func (s *Service) AddGuard(g Guard) {
	s.guards = append(s.guards, g)
}

// AddDeleteGuard registers a check that runs before every deletion.
func (s *Service) AddDeleteGuard(g DeleteGuard) {
	s.deleteGuards = append(s.deleteGuards, g)
}

// Create requests a snapshot of an application. The controller carries it out.
// It returns discovery.ErrNotFound for an unknown application and ErrInProgress while another snapshot runs.
func (s *Service) Create(ctx context.Context, application string) (View, error) {
	if _, err := s.apps.Get(ctx, application); err != nil {
		return View{}, err
	}
	return s.request(ctx, application, nil, nil)
}

// Schedule requests a snapshot on behalf of a policy and, when exportTo is not empty, a copy of it to those storage
// locations. The policy has already checked that the application exists, so Schedule does not look it up again;
// the refusals are the same as for Create.
func (s *Service) Schedule(ctx context.Context, application string, by PolicyRef, exportTo []string) (View, error) {
	return s.request(ctx, application, &by, exportTo)
}

func (s *Service) request(ctx context.Context, application string, by *PolicyRef, exportTo []string) (View, error) {
	existing, err := s.store.List(ctx, application)
	if err != nil {
		return View{}, err
	}
	for i := range existing {
		if NewView(&existing[i]).Active() {
			return View{}, fmt.Errorf("%w: %s", ErrInProgress, existing[i].Name)
		}
	}

	for _, guard := range s.guards {
		if err := guard(ctx, application); err != nil {
			return View{}, err
		}
	}

	labels := map[string]string{
		LabelManagedBy:   ManagedByValue,
		LabelApplication: application,
	}
	if by != nil {
		labels[LabelPolicy] = by.ID
	}
	snap := &Snapshot{
		ObjectMeta: metav1.ObjectMeta{Name: crstore.NewName("s", s.now()), Labels: labels},
		Spec:       Spec{Application: application, Policy: by, ExportTo: exportTo},
	}
	created, err := s.store.Create(ctx, snap)
	if err != nil {
		return View{}, err
	}
	return NewView(created), nil
}

// Delete removes a snapshot. The controller deletes the volume snapshots and the saved files behind it.
// It returns ErrNotFound, ErrRunning while the snapshot is still being created, or ErrInUse when a guard refuses.
// Deleting a snapshot that is already being removed succeeds.
func (s *Service) Delete(ctx context.Context, id string) error {
	snap, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if snap.Deleting() {
		return nil
	}
	if NewView(snap).Active() {
		return fmt.Errorf("%w: %s", ErrRunning, id)
	}
	for _, guard := range s.deleteGuards {
		if err := guard(ctx, snap); err != nil {
			return err
		}
	}
	return s.store.Delete(ctx, id)
}

// RetryExport asks the controller to copy a finished backup to a storage location again, after the copy failed.
// It returns ErrNotFound or ErrNotRetryable.
func (s *Service) RetryExport(ctx context.Context, id, location string) (View, error) {
	snap, err := s.store.Get(ctx, id)
	if err != nil {
		return View{}, err
	}
	if snap.Deleting() {
		return View{}, ErrNotFound
	}
	failed := false
	for _, e := range snap.Status.Exports {
		failed = failed || (e.Location == location && e.Phase == PhaseFailed)
	}
	if snap.Status.Phase != PhaseSucceeded || !failed {
		return View{}, fmt.Errorf("%w: %s", ErrNotRetryable, location)
	}
	retry := ExportRetry{Location: location, RequestedAt: metav1.NewTime(s.now())}
	replaced := false
	for i := range snap.Spec.ExportRetries {
		if snap.Spec.ExportRetries[i].Location == location {
			snap.Spec.ExportRetries[i], replaced = retry, true
		}
	}
	if !replaced {
		snap.Spec.ExportRetries = append(snap.Spec.ExportRetries, retry)
	}
	updated, err := s.store.Update(ctx, snap)
	if err != nil {
		return View{}, err
	}
	return NewView(updated), nil
}

// List returns the snapshots of an application, newest first. Snapshots stay listed after the
// application itself is gone. limit <= 0 uses the default page size; cursor is the id of the last item of the previous page.
func (s *Service) List(ctx context.Context, application string, limit int, cursor string) (Page, error) {
	all, err := s.store.List(ctx, application)
	if err != nil {
		return Page{}, err
	}
	snaps := slices.DeleteFunc(all, func(s Snapshot) bool { return s.Deleting() })
	total := len(snaps)
	items, next := crstore.NewestFirst(snaps, func(s Snapshot) string { return s.Name }, limit, cursor)

	page := Page{Items: make([]View, 0, len(items)), Total: total, NextCursor: next}
	for i := range items {
		page.Items = append(page.Items, NewView(&items[i]))
	}
	return page, nil
}
