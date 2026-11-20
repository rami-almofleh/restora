package offsite

import (
	"context"
	"sort"
	"time"

	"restora.local/restora/internal/crstore"
	"restora.local/restora/internal/snapshot"
)

// RecordView is an Export as the REST API returns it.
type RecordView struct {
	ID          string `json:"id"`
	Application string `json:"application"`
	Snapshot    string `json:"snapshot"`
	Location    struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"location"`
	CreatedAt time.Time           `json:"createdAt"`
	Policy    *snapshot.PolicyRef `json:"policy,omitempty"`
	Volumes   []RecordVolumeView  `json:"volumes"`
	Resources int                 `json:"resources"`
	// SizeBytes is the size of the volumes. AddedBytes is how much the repository grew by this copy.
	SizeBytes  int64 `json:"sizeBytes"`
	AddedBytes int64 `json:"addedBytes"`
}

// RecordVolumeView is a volume of an Export. Repository ids stay inside Restora.
type RecordVolumeView struct {
	Claim     string `json:"claim"`
	SizeBytes int64  `json:"sizeBytes"`
}

// RecordPage is one slice of the export list, newest first.
type RecordPage struct {
	Items      []RecordView `json:"items"`
	Total      int          `json:"total"`
	NextCursor string       `json:"nextCursor,omitempty"`
}

// ApplicationSummary says that an application has exports, and since when.
type ApplicationSummary struct {
	Application string    `json:"application"`
	Exports     int       `json:"exports"`
	LatestAt    time.Time `json:"latestAt"`
}

// NewRecordView converts the custom resource for the API.
func NewRecordView(r *Record) RecordView {
	v := RecordView{
		ID: r.Name, Application: r.Spec.Application, Snapshot: r.Spec.Snapshot,
		CreatedAt: r.Spec.CreatedAt.UTC(), Policy: r.Spec.Policy, Volumes: make([]RecordVolumeView, 0, len(r.Spec.Volumes)),
		Resources: r.Spec.Resources.Count, SizeBytes: r.Spec.SizeBytes(), AddedBytes: r.Spec.AddedBytes,
	}
	v.Location.ID, v.Location.Name = r.Spec.Location, r.Spec.LocationName
	for _, vol := range r.Spec.Volumes {
		v.Volumes = append(v.Volumes, RecordVolumeView{Claim: vol.Claim, SizeBytes: vol.SizeBytes})
	}
	return v
}

// RecordService is the API side of exports: it lists what is stored in the storage locations.
type RecordService struct {
	store *RecordStore
}

// NewRecordService creates a RecordService.
func NewRecordService(store *RecordStore) *RecordService {
	return &RecordService{store: store}
}

// List returns the exports, newest first, optionally of one application or one location.
// limit <= 0 uses the default page size; cursor is the id of the last item of the previous page.
func (s *RecordService) List(ctx context.Context, application, location string, limit int, cursor string) (RecordPage, error) {
	all, err := s.store.List(ctx, "")
	if err != nil {
		return RecordPage{}, err
	}
	matching := all[:0:0]
	for _, r := range all {
		if (application == "" || r.Spec.Application == application) && (location == "" || r.Spec.Location == location) {
			matching = append(matching, r)
		}
	}
	items, next := crstore.NewestFirst(matching, func(r Record) string { return r.Name }, limit, cursor)
	page := RecordPage{Items: make([]RecordView, 0, len(items)), Total: len(matching), NextCursor: next}
	for i := range items {
		page.Items = append(page.Items, NewRecordView(&items[i]))
	}
	return page, nil
}

// Get returns one export.
func (s *RecordService) Get(ctx context.Context, id string) (RecordView, error) {
	r, err := s.store.Get(ctx, id)
	if err != nil {
		return RecordView{}, err
	}
	return NewRecordView(r), nil
}

// Applications returns the applications that have exports, sorted by name.
func (s *RecordService) Applications(ctx context.Context) ([]ApplicationSummary, error) {
	all, err := s.store.List(ctx, "")
	if err != nil {
		return nil, err
	}
	byName := map[string]*ApplicationSummary{}
	for _, r := range all {
		sum := byName[r.Spec.Application]
		if sum == nil {
			sum = &ApplicationSummary{Application: r.Spec.Application}
			byName[r.Spec.Application] = sum
		}
		sum.Exports++
		if t := r.Spec.CreatedAt.UTC(); t.After(sum.LatestAt) {
			sum.LatestAt = t
		}
	}
	out := make([]ApplicationSummary, 0, len(byName))
	for _, sum := range byName {
		out = append(out, *sum)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Application < out[j].Application })
	return out, nil
}

// CountByLocation returns how many exports each location holds.
func (s *RecordService) CountByLocation(ctx context.Context) (map[string]int, error) {
	all, err := s.store.List(ctx, "")
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, r := range all {
		counts[r.Spec.Location]++
	}
	return counts, nil
}
