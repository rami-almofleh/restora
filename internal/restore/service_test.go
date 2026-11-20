package restore

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"restora.local/restora/internal/snapshot"
)

func newTestService(t *testing.T) (*Service, *snapshot.Store) {
	t.Helper()
	client := newFakeClient()
	snaps := snapshot.NewStore(client, testNamespace)
	svc := NewService(NewStore(client, testNamespace), snaps)
	svc.now = func() time.Time { return testNow }
	return svc, snaps
}

func addSnapshot(t *testing.T, snaps *snapshot.Store, name, application string, phase snapshot.Phase) {
	t.Helper()
	created, err := snaps.Create(context.Background(), &snapshot.Snapshot{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{snapshot.LabelApplication: application}},
		Spec:       snapshot.Spec{Application: application},
	})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	created.Status.Phase = phase
	if _, err := snaps.UpdateStatus(context.Background(), created); err != nil {
		t.Fatalf("update snapshot: %v", err)
	}
}

func TestCreate(t *testing.T) {
	svc, snaps := newTestService(t)
	addSnapshot(t, snaps, "s-1", "demo", snapshot.PhaseSucceeded)

	view, err := svc.Create(context.Background(), "s-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !regexp.MustCompile(`^r-20261119-120000-[a-z0-9]{4}$`).MatchString(view.ID) {
		t.Errorf("ID = %q", view.ID)
	}
	if view.Application != "demo" || view.Snapshot != "s-1" || view.Phase != snapshot.PhasePending {
		t.Errorf("view = %+v", view)
	}
}

func TestCreateRefuses(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown snapshot", func(t *testing.T) {
		svc, _ := newTestService(t)
		if _, err := svc.Create(ctx, "nope"); !errors.Is(err, snapshot.ErrNotFound) {
			t.Errorf("error = %v, want snapshot.ErrNotFound", err)
		}
	})
	t.Run("snapshot not finished", func(t *testing.T) {
		for _, phase := range []snapshot.Phase{snapshot.PhaseRunning, snapshot.PhaseFailed, ""} {
			svc, snaps := newTestService(t)
			addSnapshot(t, snaps, "s-1", "demo", phase)
			if _, err := svc.Create(ctx, "s-1"); !errors.Is(err, ErrSnapshotNotReady) {
				t.Errorf("phase %q: error = %v, want ErrSnapshotNotReady", phase, err)
			}
		}
	})
	t.Run("restore already running", func(t *testing.T) {
		svc, snaps := newTestService(t)
		addSnapshot(t, snaps, "s-1", "demo", snapshot.PhaseSucceeded)
		if _, err := svc.Create(ctx, "s-1"); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Create(ctx, "s-1"); !errors.Is(err, ErrInProgress) {
			t.Errorf("error = %v, want ErrInProgress", err)
		}
	})
	t.Run("backup running", func(t *testing.T) {
		svc, snaps := newTestService(t)
		addSnapshot(t, snaps, "s-1", "demo", snapshot.PhaseSucceeded)
		addSnapshot(t, snaps, "s-2", "demo", snapshot.PhaseRunning)
		if _, err := svc.Create(ctx, "s-1"); !errors.Is(err, ErrSnapshotRunning) {
			t.Errorf("error = %v, want ErrSnapshotRunning", err)
		}
	})
	t.Run("backup of another application does not matter", func(t *testing.T) {
		svc, snaps := newTestService(t)
		addSnapshot(t, snaps, "s-1", "demo", snapshot.PhaseSucceeded)
		addSnapshot(t, snaps, "s-2", "shop", snapshot.PhaseRunning)
		if _, err := svc.Create(ctx, "s-1"); err != nil {
			t.Errorf("error = %v, want none", err)
		}
	})
}

func TestGuardBlocksBackupsWhileRestoring(t *testing.T) {
	ctx := context.Background()
	svc, snaps := newTestService(t)
	addSnapshot(t, snaps, "s-1", "demo", snapshot.PhaseSucceeded)

	if err := svc.Guard(ctx, "demo"); err != nil {
		t.Fatalf("Guard without restore: %v", err)
	}
	if _, err := svc.Create(ctx, "s-1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Guard(ctx, "demo"); !errors.Is(err, snapshot.ErrBlocked) {
		t.Errorf("Guard during restore = %v, want snapshot.ErrBlocked", err)
	}
	if err := svc.Guard(ctx, "shop"); err != nil {
		t.Errorf("Guard for another application = %v, want nil", err)
	}
}

func TestListNewestFirstWithPaging(t *testing.T) {
	ctx := context.Background()
	svc, snaps := newTestService(t)
	addSnapshot(t, snaps, "s-1", "demo", snapshot.PhaseSucceeded)

	var ids []string
	for i := range 3 {
		svc.now = func() time.Time { return testNow.Add(time.Duration(i) * time.Minute) }
		view, err := svc.Create(ctx, "s-1")
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
		ids = append(ids, view.ID)
		req, _ := svc.store.Get(ctx, view.ID)
		req.Status.Phase = snapshot.PhaseSucceeded
		_, _ = svc.store.UpdateStatus(ctx, req)
	}

	first, err := svc.List(ctx, "demo", 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 3 || len(first.Items) != 2 || first.Items[0].ID != ids[2] || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	second, _ := svc.List(ctx, "demo", 2, first.NextCursor)
	if len(second.Items) != 1 || second.Items[0].ID != ids[0] || second.NextCursor != "" {
		t.Errorf("second page = %+v", second)
	}
}

func TestHandler(t *testing.T) {
	svc, snaps := newTestService(t)
	addSnapshot(t, snaps, "s-1", "demo", snapshot.PhaseSucceeded)
	addSnapshot(t, snaps, "s-run", "other", snapshot.PhaseRunning)
	mux := http.NewServeMux()
	Register(mux, svc, discardLog())

	post := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		return rec
	}

	rec := post("/api/v1/snapshots/s-1/restores")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body %s", rec.Code, rec.Body)
	}
	raw := rec.Body.String()
	if !strings.Contains(raw, `"steps":[]`) || !strings.Contains(raw, `"warnings":[]`) {
		t.Errorf("empty lists must be [] not null: %s", raw)
	}

	tests := []struct {
		name     string
		path     string
		wantCode int
		wantErr  string
	}{
		{"second restore", "/api/v1/snapshots/s-1/restores", http.StatusConflict, "restore_in_progress"},
		{"unknown snapshot", "/api/v1/snapshots/nope/restores", http.StatusNotFound, "snapshot_not_found"},
		{"unfinished snapshot", "/api/v1/snapshots/s-run/restores", http.StatusConflict, "snapshot_not_ready"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := post(tt.path)
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			var body struct {
				Error struct{ Code, Message, Hint string }
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || body.Error.Code != tt.wantErr {
				t.Errorf("error = %+v (%v), want code %q", body.Error, err, tt.wantErr)
			}
			if body.Error.Message == "" || body.Error.Hint == "" {
				t.Errorf("error needs message and hint: %+v", body.Error)
			}
		})
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/applications/demo/restores", nil))
	var page Page
	if rec.Code != http.StatusOK || json.NewDecoder(rec.Body).Decode(&page) != nil || len(page.Items) != 1 {
		t.Errorf("list: status %d, page %+v", rec.Code, page)
	}
	for _, q := range []string{"?limit=abc", "?limit=0"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/applications/demo/restores"+q, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", q, rec.Code)
		}
	}
}

func TestSnapshotGuardRefusesWhileARestoreReadsFromIt(t *testing.T) {
	svc, snaps := newTestService(t)
	ctx := context.Background()
	addSnapshot(t, snaps, "s-1", "demo", snapshot.PhaseSucceeded)
	addSnapshot(t, snaps, "s-2", "demo", snapshot.PhaseSucceeded)
	snap1, _ := snaps.Get(ctx, "s-1")
	snap2, _ := snaps.Get(ctx, "s-2")

	if err := svc.SnapshotGuard(ctx, snap1); err != nil {
		t.Fatalf("no restore yet: %v", err)
	}
	view, err := svc.Create(ctx, "s-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SnapshotGuard(ctx, snap1); !errors.Is(err, snapshot.ErrInUse) {
		t.Errorf("snapshot of the running restore: error = %v, want ErrInUse", err)
	}
	if err := svc.SnapshotGuard(ctx, snap2); err != nil {
		t.Errorf("another snapshot of the same application must stay deletable: %v", err)
	}

	req, _ := svc.store.Get(ctx, view.ID)
	req.Status.Phase = snapshot.PhaseSucceeded
	if _, err := svc.store.UpdateStatus(ctx, req); err != nil {
		t.Fatal(err)
	}
	if err := svc.SnapshotGuard(ctx, snap1); err != nil {
		t.Errorf("after the restore finished: %v", err)
	}
}

func TestCreateRefusesASnapshotThatIsBeingDeleted(t *testing.T) {
	svc, snaps := newTestService(t)
	ctx := context.Background()
	now := metav1.NewTime(testNow)
	gone, err := snaps.Create(ctx, &snapshot.Snapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "s-gone", DeletionTimestamp: &now, Labels: map[string]string{snapshot.LabelApplication: "demo"}},
		Spec:       snapshot.Spec{Application: "demo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !gone.Deleting() {
		t.Skip("the fake client drops the deletion timestamp")
	}
	gone.Status.Phase = snapshot.PhaseSucceeded
	if _, err := snaps.UpdateStatus(ctx, gone); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Create(ctx, "s-gone"); !errors.Is(err, snapshot.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}
