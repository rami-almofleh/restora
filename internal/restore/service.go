package restore

import (
	"context"
	"errors"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"restora.local/restora/internal/crstore"
	"restora.local/restora/internal/offsite"
	"restora.local/restora/internal/snapshot"
)

var (
	// ErrSnapshotNotReady is returned when the snapshot is still running or failed.
	ErrSnapshotNotReady = errors.New("snapshot is not ready")
	// ErrInProgress is returned when the application already has a restore running.
	ErrInProgress = errors.New("restore already in progress")
	// ErrSnapshotRunning is returned when a backup of the application is still being created.
	ErrSnapshotRunning = errors.New("snapshot in progress")
)

// Snapshots reads snapshots. snapshot.Store implements it.
type Snapshots interface {
	Get(ctx context.Context, name string) (*snapshot.Snapshot, error)
	List(ctx context.Context, application string) ([]snapshot.Snapshot, error)
}

// Exports finds stored backups. offsite.RecordStore implements it.
type Exports interface {
	Get(ctx context.Context, name string) (*offsite.Record, error)
}

// Service is the API side of restores: it requests them and reports their state.
type Service struct {
	store     *Store
	snapshots Snapshots
	exports   Exports
	now       func() time.Time
}

// NewService creates a Service.
func NewService(store *Store, snapshots Snapshots) *Service {
	return &Service{store: store, snapshots: snapshots, now: time.Now}
}

// WithExports lets the service restore from storage locations.
func (s *Service) WithExports(exports Exports) *Service {
	s.exports = exports
	return s
}

// CreateFromExport requests a restore of a backup that is stored in a storage location. The application may be gone.
// It returns offsite.ErrNotFound, ErrInProgress or ErrSnapshotRunning when it must refuse.
func (s *Service) CreateFromExport(ctx context.Context, exportID string) (View, error) {
	record, err := s.exports.Get(ctx, exportID)
	if err != nil {
		return View{}, err
	}
	application := record.Spec.Application
	if err := s.ensureFree(ctx, application); err != nil {
		return View{}, err
	}
	created := record.Spec.CreatedAt
	req := &RestoreRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: crstore.NewName("r", s.now()),
			Labels: map[string]string{
				snapshot.LabelManagedBy: snapshot.ManagedByValue,
				LabelApplication:        application,
				LabelExport:             exportID,
			},
		},
		Spec: Spec{Export: exportID, Application: application, ExportLocationName: record.Spec.LocationName, ExportCreatedAt: &created},
	}
	createdReq, err := s.store.Create(ctx, req)
	if err != nil {
		return View{}, err
	}
	return NewView(createdReq), nil
}

// ensureFree refuses a restore while a restore or a backup of the application is running.
func (s *Service) ensureFree(ctx context.Context, application string) error {
	active, err := s.Active(ctx, application)
	if err != nil {
		return err
	}
	if active {
		return fmt.Errorf("%w: %s", ErrInProgress, application)
	}
	others, err := s.snapshots.List(ctx, application)
	if err != nil {
		return err
	}
	for i := range others {
		if snapshot.NewView(&others[i]).Active() {
			return fmt.Errorf("%w: %s", ErrSnapshotRunning, others[i].Name)
		}
	}
	return nil
}

// Create requests a restore of a snapshot into its own application. The controller carries it out.
// It returns snapshot.ErrNotFound, ErrSnapshotNotReady, ErrInProgress or ErrSnapshotRunning when it must refuse.
func (s *Service) Create(ctx context.Context, snapshotID string) (View, error) {
	snap, err := s.snapshots.Get(ctx, snapshotID)
	if err != nil {
		return View{}, err
	}
	if snap.Deleting() {
		return View{}, fmt.Errorf("%w: %s is being deleted", snapshot.ErrNotFound, snapshotID)
	}
	if snap.Status.Phase != snapshot.PhaseSucceeded {
		return View{}, fmt.Errorf("%w: %s is %s", ErrSnapshotNotReady, snapshotID, snap.Status.Phase)
	}
	application := snap.Spec.Application

	if err := s.ensureFree(ctx, application); err != nil {
		return View{}, err
	}

	req := &RestoreRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: crstore.NewName("r", s.now()),
			Labels: map[string]string{
				snapshot.LabelManagedBy: snapshot.ManagedByValue,
				LabelApplication:        application,
				LabelSnapshot:           snapshotID,
			},
		},
		Spec: Spec{Snapshot: snapshotID, Application: application},
	}
	created, err := s.store.Create(ctx, req)
	if err != nil {
		return View{}, err
	}
	return NewView(created), nil
}

// Active reports whether a restore of the application is running. It also serves as the guard that keeps
// new backups from starting while data is being replaced.
func (s *Service) Active(ctx context.Context, application string) (bool, error) {
	existing, err := s.store.List(ctx, application)
	if err != nil {
		return false, err
	}
	for i := range existing {
		if NewView(&existing[i]).Active() {
			return true, nil
		}
	}
	return false, nil
}

// Guard keeps new backups from starting while a restore replaces the data. Register it with snapshot.Service.AddGuard.
func (s *Service) Guard(ctx context.Context, application string) error {
	active, err := s.Active(ctx, application)
	if err != nil {
		return err
	}
	if active {
		return fmt.Errorf("%w: restore of %s is running", snapshot.ErrBlocked, application)
	}
	return nil
}

// SnapshotGuard keeps a backup from being deleted while a restore reads from it. Register it with
// snapshot.Service.AddDeleteGuard.
func (s *Service) SnapshotGuard(ctx context.Context, snap *snapshot.Snapshot) error {
	existing, err := s.store.List(ctx, snap.Spec.Application)
	if err != nil {
		return err
	}
	for i := range existing {
		if existing[i].Spec.Snapshot == snap.Name && NewView(&existing[i]).Active() {
			return fmt.Errorf("%w: restore %s reads from %s", snapshot.ErrInUse, existing[i].Name, snap.Name)
		}
	}
	return nil
}

// List returns the restores of an application, newest first. limit <= 0 uses the default page size;
// cursor is the id of the last item of the previous page.
func (s *Service) List(ctx context.Context, application string, limit int, cursor string) (Page, error) {
	reqs, err := s.store.List(ctx, application)
	if err != nil {
		return Page{}, err
	}
	total := len(reqs)
	items, next := crstore.NewestFirst(reqs, func(r RestoreRequest) string { return r.Name }, limit, cursor)

	page := Page{Items: make([]View, 0, len(items)), Total: total, NextCursor: next}
	for i := range items {
		page.Items = append(page.Items, NewView(&items[i]))
	}
	return page, nil
}
