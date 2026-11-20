package policy

import (
	"errors"

	"k8s.io/client-go/dynamic"
	"restora.local/restora/internal/crstore"
)

// ErrNotFound is returned when a Policy does not exist.
var ErrNotFound = errors.New("policy not found")

// Store reads and writes Policy resources in one namespace.
type Store struct {
	*crstore.Store[Policy, *Policy]
}

// NewStore creates a Store for the namespace Restora runs in.
func NewStore(client dynamic.Interface, namespace string) *Store {
	return &Store{crstore.New[Policy](client, GVR, namespace, Kind, ErrNotFound)}
}
