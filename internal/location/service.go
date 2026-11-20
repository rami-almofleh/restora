package location

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"restora.local/restora/internal/crstore"
	"restora.local/restora/internal/snapshot"
)

// ErrNameTaken is returned when another location already has the name.
var ErrNameTaken = errors.New("location name already in use")

// ErrTargetTaken is returned when another location already writes to the same bucket folder or share.
var ErrTargetTaken = errors.New("storage target already used by another location")

// ErrInUse is returned when something still needs the location, for example a policy that copies backups to it.
var ErrInUse = errors.New("location is in use")

// DeleteGuard returns an error wrapping ErrInUse when the location must not be deleted right now.
type DeleteGuard func(ctx context.Context, id string) error

// ExportCounter says how many stored backups each location holds. offsite.RecordService implements it.
type ExportCounter interface {
	CountByLocation(ctx context.Context) (map[string]int, error)
}

// Credentials stores keys. SecretWriter implements it.
type Credentials interface {
	Put(ctx context.Context, owner *LocationProfile, accessKey, secretKey string) error
}

// Service is the API side of locations.
type Service struct {
	store        *Store
	credentials  Credentials
	deleteGuards []DeleteGuard
	exports      ExportCounter
	now          func() time.Time
}

// NewService creates a Service.
func NewService(store *Store, credentials Credentials) *Service {
	return &Service{store: store, credentials: credentials, now: time.Now}
}

// AddDeleteGuard registers a check that runs before a location is deleted.
func (s *Service) AddDeleteGuard(g DeleteGuard) {
	s.deleteGuards = append(s.deleteGuards, g)
}

// CountExports makes locations report how many stored backups they hold.
func (s *Service) CountExports(c ExportCounter) {
	s.exports = c
}

// withExports fills in the number of stored backups.
func (s *Service) withExports(ctx context.Context, views ...*View) error {
	if s.exports == nil {
		return nil
	}
	counts, err := s.exports.CountByLocation(ctx)
	if err != nil {
		return err
	}
	for _, v := range views {
		v.Exports = counts[v.ID]
	}
	return nil
}

// Create saves a new location, stores its keys as a secret and asks the controller to test the connection.
// It returns ErrInvalid or ErrNameTaken.
func (s *Service) Create(ctx context.Context, in Input) (View, error) {
	spec, err := in.normalize()
	if err != nil {
		return View{}, err
	}
	if in.Type == TypeS3 && in.S3.AccessKey == "" {
		return View{}, invalid("accessKey", "give the access key and the secret key")
	}
	if err := s.ensureNameFree(ctx, spec.Name, ""); err != nil {
		return View{}, err
	}
	if err := s.ensureTargetFree(ctx, spec, ""); err != nil {
		return View{}, err
	}

	id := crstore.NewName("l", s.now())
	if spec.S3 != nil {
		spec.S3.CredentialsSecret = SecretName(id)
	}
	requested := metav1.NewTime(s.now())
	spec.TestRequestedAt = &requested

	created, err := s.store.Create(ctx, &LocationProfile{
		ObjectMeta: metav1.ObjectMeta{Name: id, Labels: map[string]string{snapshot.LabelManagedBy: snapshot.ManagedByValue}},
		Spec:       spec,
	})
	if err != nil {
		return View{}, err
	}
	if in.Type == TypeS3 {
		if err := s.credentials.Put(ctx, created, in.S3.AccessKey, in.S3.SecretKey); err != nil {
			// A location without keys would only ever fail its test.
			_ = s.store.Delete(ctx, id)
			return View{}, err
		}
	}
	return NewView(created), nil
}

// Update replaces the settings of a location and asks for a new test. The type cannot change. Leaving both keys
// out keeps the stored ones.
func (s *Service) Update(ctx context.Context, id string, in Input) (View, error) {
	spec, err := in.normalize()
	if err != nil {
		return View{}, err
	}
	current, err := s.store.Get(ctx, id)
	if err != nil {
		return View{}, err
	}
	if current.Spec.Type != spec.Type {
		return View{}, invalid("type", "the type of a location cannot be changed; create a new one")
	}
	if err := s.ensureNameFree(ctx, spec.Name, id); err != nil {
		return View{}, err
	}
	if err := s.ensureTargetFree(ctx, spec, id); err != nil {
		return View{}, err
	}

	if spec.S3 != nil {
		spec.S3.CredentialsSecret = current.Spec.S3.CredentialsSecret
		if spec.S3.CredentialsSecret == "" {
			spec.S3.CredentialsSecret = SecretName(id)
			if in.S3.AccessKey == "" {
				return View{}, invalid("accessKey", "this location has no keys yet; give the access key and the secret key")
			}
		}
		// The secret goes first: the controller tests as soon as the profile changes and must see the new keys.
		if in.S3.AccessKey != "" {
			if err := s.credentials.Put(ctx, current, in.S3.AccessKey, in.S3.SecretKey); err != nil {
				return View{}, err
			}
		}
	}
	requested := metav1.NewTime(s.now())
	spec.TestRequestedAt = &requested
	current.Spec = spec
	updated, err := s.store.Update(ctx, current)
	if err != nil {
		return View{}, err
	}
	return NewView(updated), nil
}

// RequestTest asks the controller to test the connection again.
func (s *Service) RequestTest(ctx context.Context, id string) (View, error) {
	current, err := s.store.Get(ctx, id)
	if err != nil {
		return View{}, err
	}
	requested := metav1.NewTime(s.now())
	current.Spec.TestRequestedAt = &requested
	updated, err := s.store.Update(ctx, current)
	if err != nil {
		return View{}, err
	}
	return NewView(updated), nil
}

// Delete removes a location. Its secret goes with it, because the secret belongs to the profile.
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := s.store.Get(ctx, id); err != nil {
		return err
	}
	for _, guard := range s.deleteGuards {
		if err := guard(ctx, id); err != nil {
			return err
		}
	}
	return s.store.Delete(ctx, id)
}

// Get returns one location.
func (s *Service) Get(ctx context.Context, id string) (View, error) {
	l, err := s.store.Get(ctx, id)
	if err != nil {
		return View{}, err
	}
	view := NewView(l)
	return view, s.withExports(ctx, &view)
}

// List returns the locations, newest first. limit <= 0 uses the default page size;
// cursor is the id of the last item of the previous page.
func (s *Service) List(ctx context.Context, limit int, cursor string) (Page, error) {
	all, err := s.store.List(ctx, "")
	if err != nil {
		return Page{}, err
	}
	items, next := crstore.NewestFirst(all, func(l LocationProfile) string { return l.Name }, limit, cursor)
	page := Page{Items: make([]View, 0, len(items)), Total: len(all), NextCursor: next}
	for i := range items {
		page.Items = append(page.Items, NewView(&items[i]))
	}
	views := make([]*View, len(page.Items))
	for i := range page.Items {
		views[i] = &page.Items[i]
	}
	return page, s.withExports(ctx, views...)
}

func (s *Service) ensureNameFree(ctx context.Context, name, ownID string) error {
	all, err := s.store.List(ctx, "")
	if err != nil {
		return err
	}
	for i := range all {
		if all[i].Name != ownID && strings.EqualFold(all[i].Spec.Name, name) {
			return fmt.Errorf("%w: %s", ErrNameTaken, name)
		}
	}
	return nil
}

// ensureTargetFree refuses a location that writes to the same place as another one.
func (s *Service) ensureTargetFree(ctx context.Context, spec Spec, ownID string) error {
	all, err := s.store.List(ctx, "")
	if err != nil {
		return err
	}
	key := spec.TargetKey()
	for i := range all {
		if all[i].Name != ownID && all[i].Spec.TargetKey() == key {
			return fmt.Errorf("%w: %s", ErrTargetTaken, all[i].Spec.Name)
		}
	}
	return nil
}
