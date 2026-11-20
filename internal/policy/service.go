package policy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"restora.local/restora/internal/crstore"
	"restora.local/restora/internal/discovery"
	"restora.local/restora/internal/location"
	"restora.local/restora/internal/snapshot"
)

// PreviewRuns is how many upcoming runs a preview shows.
const PreviewRuns = 5

// MaxNameLength bounds the title of a policy.
const MaxNameLength = 64

// ErrNameTaken is returned when another policy already has the name.
var ErrNameTaken = errors.New("policy name already in use")

// ErrInvalidName is returned for an empty or too long name.
var ErrInvalidName = errors.New("invalid policy name")

// Applications lists the applications of the cluster. discovery.Service implements it.
type Applications interface {
	Refs(ctx context.Context) ([]discovery.Ref, error)
}

// LocationLookup finds storage locations. location.Store implements it.
type LocationLookup interface {
	Get(ctx context.Context, name string) (*location.LocationProfile, error)
}

// MaxExportTargets bounds how many storage locations a policy copies to.
const MaxExportTargets = 5

// ErrInvalidExport is returned when a policy names a storage location that does not exist, or too many.
var ErrInvalidExport = errors.New("invalid export")

// Input is what a person enters when they create or change a policy.
type Input struct {
	Name     string   `json:"name"`
	Selector Selector `json:"selector"`
	Schedule Schedule `json:"schedule"`
	// Retention is optional: leaving it out keeps every backup.
	Retention Retention `json:"retention"`
	// ExportTo is optional: the storage locations every backup is copied to.
	ExportTo []string `json:"exportTo"`
}

// Snapshots lists the backups of a policy. snapshot.Store implements it.
type Snapshots interface {
	ByPolicy(ctx context.Context, policyID string) ([]snapshot.Snapshot, error)
}

// Service is the API side of policies.
type Service struct {
	store     *Store
	apps      Applications
	snapshots Snapshots
	locations LocationLookup
	backups   Backups
	log       *slog.Logger
	now       func() time.Time
}

// NewService creates a Service.
func NewService(store *Store, apps Applications, snapshots Snapshots, locations LocationLookup) *Service {
	return &Service{store: store, apps: apps, snapshots: snapshots, locations: locations, log: slog.Default(), now: time.Now}
}

// EnableRunNow makes "run now" available: backups started that way follow the same rules as the scheduled ones.
func (s *Service) EnableRunNow(backups Backups, log *slog.Logger) {
	s.backups, s.log = backups, log
}

// validate checks the input without looking at the cluster and returns the compiled schedule.
func validate(in *Input) (Compiled, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > MaxNameLength {
		return Compiled{}, fmt.Errorf("%w: use 1 to %d characters", ErrInvalidName, MaxNameLength)
	}
	if err := in.Selector.Validate(); err != nil {
		return Compiled{}, err
	}
	if err := in.Retention.Validate(); err != nil {
		return Compiled{}, err
	}
	return in.Schedule.Compile()
}

// Preview shows the next runs and the applications an input would take, without saving anything.
func (s *Service) Preview(ctx context.Context, in Input) (Preview, error) {
	compiled, err := in.Schedule.Compile()
	if err != nil {
		return Preview{}, err
	}
	if err := in.Selector.Validate(); err != nil {
		return Preview{}, err
	}
	refs, err := s.apps.Refs(ctx)
	if err != nil {
		return Preview{}, err
	}
	found, missing := in.Selector.Select(refs)
	return Preview{
		Cron:         compiled.Expression,
		NextRuns:     compiled.NextRuns(s.now(), PreviewRuns),
		Applications: found,
		Missing:      missing,
	}, nil
}

// RetentionPreview shows which backups of an existing policy retention would delete if the input were saved.
// The input's name and selector do not matter here, only its retention and the time zone of its schedule.
func (s *Service) RetentionPreview(ctx context.Context, id string, in Input) (Plan, error) {
	if err := in.Retention.Validate(); err != nil {
		return Plan{}, err
	}
	compiled, err := in.Schedule.Compile()
	if err != nil {
		return Plan{}, err
	}
	if _, err := s.store.Get(ctx, id); err != nil {
		return Plan{}, err
	}
	snaps, err := s.snapshots.ByPolicy(ctx, id)
	if err != nil {
		return Plan{}, err
	}
	return in.Retention.Plan(snaps, s.now(), compiled.Location()), nil
}

// Create saves a new policy. It returns ErrInvalidName, ErrInvalidSelector, ErrInvalidSchedule,
// ErrInvalidRetention or ErrNameTaken.
func (s *Service) Create(ctx context.Context, in Input) (View, error) {
	if _, err := validate(&in); err != nil {
		return View{}, err
	}
	if err := s.checkExport(ctx, &in); err != nil {
		return View{}, err
	}
	if err := s.ensureNameFree(ctx, in.Name, ""); err != nil {
		return View{}, err
	}
	p := &Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:   crstore.NewName("p", s.now()),
			Labels: map[string]string{snapshot.LabelManagedBy: snapshot.ManagedByValue},
		},
		Spec: Spec{Name: in.Name, Selector: in.Selector, Schedule: in.Schedule, Retention: in.Retention, ExportTo: in.ExportTo},
	}
	created, err := s.store.Create(ctx, p)
	if err != nil {
		return View{}, err
	}
	return s.view(ctx, created)
}

// Update replaces name, selector, schedule and retention of a policy. Whether it is paused does not change.
func (s *Service) Update(ctx context.Context, id string, in Input) (View, error) {
	if _, err := validate(&in); err != nil {
		return View{}, err
	}
	if err := s.checkExport(ctx, &in); err != nil {
		return View{}, err
	}
	p, err := s.store.Get(ctx, id)
	if err != nil {
		return View{}, err
	}
	if err := s.ensureNameFree(ctx, in.Name, id); err != nil {
		return View{}, err
	}
	p.Spec.Name, p.Spec.Selector, p.Spec.Schedule, p.Spec.Retention, p.Spec.ExportTo = in.Name, in.Selector, in.Schedule, in.Retention, in.ExportTo
	updated, err := s.store.Update(ctx, p)
	if err != nil {
		return View{}, err
	}
	return s.view(ctx, updated)
}

// SetPaused pauses or resumes a policy.
func (s *Service) SetPaused(ctx context.Context, id string, paused bool) (View, error) {
	p, err := s.store.Get(ctx, id)
	if err != nil {
		return View{}, err
	}
	if p.Spec.Paused == paused {
		return s.view(ctx, p)
	}
	p.Spec.Paused = paused
	updated, err := s.store.Update(ctx, p)
	if err != nil {
		return View{}, err
	}
	return s.view(ctx, updated)
}

// Delete removes a policy. Snapshots it created stay.
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := s.store.Get(ctx, id); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

// Get returns one policy.
func (s *Service) Get(ctx context.Context, id string) (View, error) {
	p, err := s.store.Get(ctx, id)
	if err != nil {
		return View{}, err
	}
	return s.view(ctx, p)
}

// List returns the policies, newest first. limit <= 0 uses the default page size;
// cursor is the id of the last item of the previous page.
func (s *Service) List(ctx context.Context, limit int, cursor string) (Page, error) {
	all, err := s.store.List(ctx, "")
	if err != nil {
		return Page{}, err
	}
	refs, err := s.apps.Refs(ctx)
	if err != nil {
		return Page{}, err
	}
	items, next := crstore.NewestFirst(all, func(p Policy) string { return p.Name }, limit, cursor)
	page := Page{Items: make([]View, 0, len(items)), Total: len(all), NextCursor: next}
	for i := range items {
		found, missing := items[i].Spec.Selector.Select(refs)
		page.Items = append(page.Items, newView(&items[i], found, missing))
	}
	return page, nil
}

func (s *Service) view(ctx context.Context, p *Policy) (View, error) {
	refs, err := s.apps.Refs(ctx)
	if err != nil {
		return View{}, err
	}
	found, missing := p.Spec.Selector.Select(refs)
	return newView(p, found, missing), nil
}

// checkExport removes duplicates from the storage locations and makes sure each of them exists.
func (s *Service) checkExport(ctx context.Context, in *Input) error {
	seen := map[string]bool{}
	var targets []string
	for _, id := range in.ExportTo {
		if id = strings.TrimSpace(id); id != "" && !seen[id] {
			seen[id] = true
			targets = append(targets, id)
		}
	}
	if len(targets) > MaxExportTargets {
		return fmt.Errorf("%w: at most %d storage locations", ErrInvalidExport, MaxExportTargets)
	}
	for _, id := range targets {
		if _, err := s.locations.Get(ctx, id); errors.Is(err, location.ErrNotFound) {
			return fmt.Errorf("%w: the storage location %s does not exist", ErrInvalidExport, id)
		} else if err != nil {
			return err
		}
	}
	in.ExportTo = targets
	return nil
}

// RunNow starts the backups of a policy now, outside its schedule, and says what was started. It does not change
// when the policy runs next. RunNow needs EnableRunNow.
func (s *Service) RunNow(ctx context.Context, id string) (*RunResult, error) {
	p, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.backups == nil {
		return nil, errors.New("running a policy now is not enabled")
	}
	return StartSnapshots(ctx, s.log, s.apps, s.backups, p, s.now())
}

// LocationGuard keeps a storage location from being deleted while a policy copies to it. Register it with
// location.Service.AddDeleteGuard.
func (s *Service) LocationGuard(ctx context.Context, locationID string) error {
	all, err := s.store.List(ctx, "")
	if err != nil {
		return err
	}
	var names []string
	for i := range all {
		if slices.Contains(all[i].Spec.ExportTo, locationID) {
			names = append(names, all[i].Spec.Name)
		}
	}
	if len(names) > 0 {
		return fmt.Errorf("%w: used by %s", location.ErrInUse, strings.Join(names, ", "))
	}
	return nil
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
