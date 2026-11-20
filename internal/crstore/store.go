// Package crstore holds what every Restora custom resource store needs: typed access through the dynamic client,
// readable names and newest-first paging.
package crstore

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// Store reads and writes one kind of custom resource in one namespace. T is the resource struct,
// PT its pointer (which must expose the object metadata).
type Store[T any, PT interface {
	*T
	metav1.Object
}] struct {
	resource   dynamic.ResourceInterface
	apiVersion string
	kind       string
	notFound   error
}

// New creates a Store. notFound is the error (wrapped) that Get returns for a missing object.
func New[T any, PT interface {
	*T
	metav1.Object
}](client dynamic.Interface, gvr schema.GroupVersionResource, namespace, kind string, notFound error) *Store[T, PT] {
	return &Store[T, PT]{
		resource:   client.Resource(gvr).Namespace(namespace),
		apiVersion: gvr.GroupVersion().String(),
		kind:       kind,
		notFound:   notFound,
	}
}

// Create stores a new object.
func (s *Store[T, PT]) Create(ctx context.Context, obj PT) (PT, error) {
	u, err := s.toUnstructured(obj)
	if err != nil {
		return nil, err
	}
	created, err := s.resource.Create(ctx, u, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create %s %s: %w", s.kind, obj.GetName(), err)
	}
	return s.fromUnstructured(created)
}

// Get returns one object, or an error wrapping the not-found error given to New.
func (s *Store[T, PT]) Get(ctx context.Context, name string) (PT, error) {
	u, err := s.resource.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("%w: %s", s.notFound, name)
	}
	if err != nil {
		return nil, fmt.Errorf("get %s %s: %w", s.kind, name, err)
	}
	return s.fromUnstructured(u)
}

// Delete removes one object. A missing object is not an error: the goal, that it is gone, is reached.
func (s *Store[T, PT]) Delete(ctx context.Context, name string) error {
	err := s.resource.Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete %s %s: %w", s.kind, name, err)
	}
	return nil
}

// List returns the objects that match a label selector (empty selects all), in no particular order.
func (s *Store[T, PT]) List(ctx context.Context, labelSelector string) ([]T, error) {
	list, err := s.resource.List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("list %ss: %w", s.kind, err)
	}
	out := make([]T, 0, len(list.Items))
	for i := range list.Items {
		obj, err := s.fromUnstructured(&list.Items[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *obj)
	}
	return out, nil
}

// Update writes metadata and spec. Status is not changed.
func (s *Store[T, PT]) Update(ctx context.Context, obj PT) (PT, error) {
	u, err := s.toUnstructured(obj)
	if err != nil {
		return nil, err
	}
	updated, err := s.resource.Update(ctx, u, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("update %s %s: %w", s.kind, obj.GetName(), err)
	}
	return s.fromUnstructured(updated)
}

// UpdateStatus writes the status only.
func (s *Store[T, PT]) UpdateStatus(ctx context.Context, obj PT) (PT, error) {
	u, err := s.toUnstructured(obj)
	if err != nil {
		return nil, err
	}
	updated, err := s.resource.UpdateStatus(ctx, u, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("update status of %s %s: %w", s.kind, obj.GetName(), err)
	}
	return s.fromUnstructured(updated)
}

func (s *Store[T, PT]) toUnstructured(obj PT) (*unstructured.Unstructured, error) {
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("convert %s %s: %w", s.kind, obj.GetName(), err)
	}
	u := &unstructured.Unstructured{Object: m}
	u.SetAPIVersion(s.apiVersion)
	u.SetKind(s.kind)
	return u, nil
}

func (s *Store[T, PT]) fromUnstructured(u *unstructured.Unstructured) (PT, error) {
	var obj T
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &obj); err != nil {
		return nil, fmt.Errorf("convert %s %s: %w", s.kind, u.GetName(), err)
	}
	return &obj, nil
}
