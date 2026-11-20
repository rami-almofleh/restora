package snapshot

import (
	"context"
	"errors"

	"k8s.io/client-go/dynamic"
	"restora.local/restora/internal/crstore"
)

// ErrNotFound is returned when a Snapshot does not exist.
var ErrNotFound = errors.New("snapshot not found")

// Store reads and writes Snapshot resources in one namespace.
type Store struct {
	*crstore.Store[Snapshot, *Snapshot]
}

// NewStore creates a Store for the namespace Restora runs in.
func NewStore(client dynamic.Interface, namespace string) *Store {
	return &Store{crstore.New[Snapshot](client, GVR, namespace, Kind, ErrNotFound)}
}

// List returns the Snapshots of one application, in no particular order.
func (s *Store) List(ctx context.Context, application string) ([]Snapshot, error) {
	return s.Store.List(ctx, LabelApplication+"="+application)
}

// All returns the Snapshots of every application, in no particular order.
func (s *Store) All(ctx context.Context) ([]Snapshot, error) {
	return s.Store.List(ctx, "")
}

// ByPolicy returns the Snapshots a policy created, in no particular order.
func (s *Store) ByPolicy(ctx context.Context, policyID string) ([]Snapshot, error) {
	return s.Store.List(ctx, LabelPolicy+"="+policyID)
}
