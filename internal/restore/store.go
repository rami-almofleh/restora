package restore

import (
	"context"
	"errors"

	"k8s.io/client-go/dynamic"
	"restora.local/restora/internal/crstore"
)

// ErrNotFound is returned when a RestoreRequest does not exist.
var ErrNotFound = errors.New("restore not found")

// Store reads and writes RestoreRequest resources in one namespace.
type Store struct {
	*crstore.Store[RestoreRequest, *RestoreRequest]
}

// NewStore creates a Store for the namespace Restora runs in.
func NewStore(client dynamic.Interface, namespace string) *Store {
	return &Store{crstore.New[RestoreRequest](client, GVR, namespace, Kind, ErrNotFound)}
}

// List returns the requests of one application, in no particular order.
func (s *Store) List(ctx context.Context, application string) ([]RestoreRequest, error) {
	return s.Store.List(ctx, LabelApplication+"="+application)
}

// All returns the RestoreRequests of every application, in no particular order.
func (s *Store) All(ctx context.Context) ([]RestoreRequest, error) {
	return s.Store.List(ctx, "")
}
