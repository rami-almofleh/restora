package discovery

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestMux() *http.ServeMux {
	mux := http.NewServeMux()
	Register(mux, New(demoSource()), slog.New(slog.NewTextHandler(io.Discard, nil)))
	return mux
}

func TestHandlerList(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantCode  int
		wantItems int
		wantError string
	}{
		{"all", "/api/v1/applications", http.StatusOK, 2, ""},
		{"search", "/api/v1/applications?q=demo", http.StatusOK, 1, ""},
		{"filter", "/api/v1/applications?volumes=without", http.StatusOK, 1, ""},
		{"bad filter", "/api/v1/applications?volumes=maybe", http.StatusBadRequest, 0, "invalid_filter"},
		{"bad limit", "/api/v1/applications?limit=abc", http.StatusBadRequest, 0, "invalid_limit"},
		{"zero limit", "/api/v1/applications?limit=0", http.StatusBadRequest, 0, "invalid_limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newTestMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.url, nil))
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if tt.wantError != "" {
				var body struct{ Error struct{ Code string } }
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || body.Error.Code != tt.wantError {
					t.Errorf("error code = %q (%v), want %q", body.Error.Code, err, tt.wantError)
				}
				return
			}
			var page Page
			if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(page.Items) != tt.wantItems {
				t.Errorf("items = %d, want %d", len(page.Items), tt.wantItems)
			}
		})
	}
}

func TestHandlerGet(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/applications/demo", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	newTestMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/applications/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
