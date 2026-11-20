package policy

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"restora.local/restora/internal/discovery"
	"restora.local/restora/internal/location"
	"restora.local/restora/internal/snapshot"
)

const testNamespace = "restora-system"

// testNow is a Thursday.
var testNow = time.Date(2026, 11, 19, 10, 15, 30, 0, time.UTC)

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newStore(objects ...runtime.Object) *Store {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{GVR: "PolicyList"}, objects...)
	return NewStore(client, testNamespace)
}

type fakeApps struct {
	refs []discovery.Ref
	err  error
}

func (f fakeApps) Refs(context.Context) ([]discovery.Ref, error) { return f.refs, f.err }

func testApps() fakeApps {
	return fakeApps{refs: []discovery.Ref{
		{Name: "demo", Labels: map[string]string{"team": "a", "tier": "prod"}},
		{Name: "shop", Labels: map[string]string{"team": "b", "tier": "prod"}},
		{Name: "wiki", Labels: map[string]string{"team": "a"}},
	}}
}

// fakeLocations knows the storage locations l-1 and l-2.
type fakeLocations struct{}

func (fakeLocations) Get(_ context.Context, name string) (*location.LocationProfile, error) {
	if name == "l-1" || name == "l-2" {
		return &location.LocationProfile{}, nil
	}
	return nil, location.ErrNotFound
}

func newTestService() *Service {
	svc := NewService(newStore(), testApps(), &fakeSnapshots{}, fakeLocations{})
	svc.now = func() time.Time { return testNow }
	return svc
}

// fakeBackups records the snapshots a run asks for and can refuse chosen applications.
type fakeBackups struct {
	mu       sync.Mutex
	started  []string
	exportTo []string
	refuse   map[string]error
}

func (f *fakeBackups) Schedule(_ context.Context, application string, by snapshot.PolicyRef, exportTo []string) (snapshot.View, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.refuse[application]; err != nil {
		return snapshot.View{}, err
	}
	f.started = append(f.started, application)
	f.exportTo = append([]string{}, exportTo...)
	return snapshot.View{ID: "s-" + application, Application: application, Policy: &by}, nil
}

var errBoom = errors.New("boom")

func newTestReconciler(store *Store, apps Applications, backups Backups, now time.Time) *Reconciler {
	return &Reconciler{Store: store, Apps: apps, Backups: backups, Log: discardLog(), Now: func() time.Time { return now }}
}

func mustCreate(t *testing.T, svc *Service, in Input) View {
	t.Helper()
	v, err := svc.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return v
}

func everyMinute(name string, apps ...string) Input {
	return Input{
		Name:     name,
		Selector: Selector{Mode: SelectApplications, Applications: apps},
		Schedule: Schedule{Type: ScheduleCustom, Cron: "* * * * *"},
	}
}

// fakeSnapshots returns the same backups for every policy.
type fakeSnapshots struct {
	items []snapshot.Snapshot
	err   error
}

func (f *fakeSnapshots) ByPolicy(context.Context, string) ([]snapshot.Snapshot, error) {
	return f.items, f.err
}

// fakeDeleter records the deletions and can refuse chosen backups.
type fakeDeleter struct {
	deleted []string
	refuse  map[string]error
}

func (f *fakeDeleter) Delete(_ context.Context, id string) error {
	if err := f.refuse[id]; err != nil {
		return err
	}
	f.deleted = append(f.deleted, id)
	return nil
}
