package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestMux() (*http.ServeMux, *Service) {
	svc, _ := newTestService()
	mux := http.NewServeMux()
	Register(mux, svc, discardLog())
	return mux, svc
}

func TestHandlerCreate(t *testing.T) {
	mux, _ := newTestMux()

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/applications/demo/snapshots", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body %s", rec.Code, rec.Body)
	}
	raw := rec.Body.String()
	var view View
	if err := json.Unmarshal([]byte(raw), &view); err != nil || view.Application != "demo" || view.Phase != PhasePending {
		t.Fatalf("view = %+v, err = %v", view, err)
	}
	if !strings.Contains(raw, `"steps":[]`) || !strings.Contains(raw, `"volumes":[]`) {
		t.Errorf("empty lists must be [] not null: %s", raw)
	}

	tests := []struct {
		name     string
		path     string
		wantCode int
		wantErr  string
	}{
		{"second while running", "/api/v1/applications/demo/snapshots", http.StatusConflict, "snapshot_in_progress"},
		{"unknown application", "/api/v1/applications/nope/snapshots", http.StatusNotFound, "application_not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tt.path, nil))
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
}

func TestHandlerList(t *testing.T) {
	mux, svc := newTestMux()
	if _, err := svc.Create(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/applications/demo/snapshots", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var page Page
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil || len(page.Items) != 1 || page.Total != 1 {
		t.Errorf("page = %+v, err = %v", page, err)
	}

	for _, url := range []string{"?limit=abc", "?limit=0", "?limit=-1"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/applications/demo/snapshots"+url, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", url, rec.Code)
		}
	}
}

func TestHandlerCreateBlocked(t *testing.T) {
	mux, svc := newTestMux()
	svc.AddGuard(func(context.Context, string) error { return ErrBlocked })

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/applications/demo/snapshots", nil))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "restore_in_progress") {
		t.Errorf("status = %d, body %s, want 409 restore_in_progress", rec.Code, rec.Body)
	}
}

func TestHandlerDelete(t *testing.T) {
	svc, store := newTestService()
	mux := http.NewServeMux()
	Register(mux, svc, discardLog())
	ctx := context.Background()

	running, _ := svc.Create(ctx, "demo")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/snapshots/"+running.ID, nil))
	assertError(t, rec, http.StatusConflict, "snapshot_running")

	finishSnapshot(t, store, running.ID, PhaseSucceeded)
	svc.AddDeleteGuard(func(context.Context, *Snapshot) error { return fmt.Errorf("%w: restore", ErrInUse) })
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/snapshots/"+running.ID, nil))
	assertError(t, rec, http.StatusConflict, "snapshot_in_use")

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/snapshots/s-nope", nil))
	assertError(t, rec, http.StatusNotFound, "snapshot_not_found")

	svc.deleteGuards = nil
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/snapshots/"+running.ID, nil))
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Errorf("status = %d, body %q; want 204 and no body", rec.Code, rec.Body)
	}
}

func assertError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body %s", rec.Code, status, rec.Body)
	}
	var body struct {
		Error struct{ Code, Message, Hint string }
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || body.Error.Code != code {
		t.Errorf("error = %+v (%v), want code %q", body.Error, err, code)
	}
	if body.Error.Message == "" || body.Error.Hint == "" {
		t.Errorf("error needs message and hint: %+v", body.Error)
	}
}
