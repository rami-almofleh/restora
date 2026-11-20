package location

import (
	"errors"

	"k8s.io/client-go/dynamic"
	"restora.local/restora/internal/crstore"
)

// ErrNotFound is returned when a LocationProfile does not exist.
var ErrNotFound = errors.New("location not found")

// Store reads and writes LocationProfile resources in one namespace.
type Store struct {
	*crstore.Store[LocationProfile, *LocationProfile]
}

// NewStore creates a Store for the namespace Restora runs in.
func NewStore(client dynamic.Interface, namespace string) *Store {
	return &Store{crstore.New[LocationProfile](client, GVR, namespace, Kind, ErrNotFound)}
}
