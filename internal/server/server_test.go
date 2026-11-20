package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoints(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHealth(mux)

	tests := []struct {
		name string
		path string
	}{
		{name: "liveness", path: "/healthz"},
		{name: "readiness", path: "/readyz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			var body healthBody
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Status != "ok" {
				t.Fatalf("status field = %q, want %q", body.Status, "ok")
			}
		})
	}
}

func TestNotFoundUsesErrorFormat(t *testing.T) {
	rec := httptest.NewRecorder()
	NotFound(rec, httptest.NewRequest(http.MethodGet, "/api/v1/nothing", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	var body ErrorBody
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "not_found" || body.Error.Message == "" || body.Error.Hint == "" {
		t.Fatalf("unexpected error body: %+v", body)
	}
}

func TestSecureHeaders(t *testing.T) {
	h := SecureHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	for _, key := range []string{"Content-Security-Policy", "X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy"} {
		if rec.Header().Get(key) == "" {
			t.Errorf("header %s is missing", key)
		}
	}
}

func TestParseLimit(t *testing.T) {
	tests := []struct {
		query     string
		wantLimit int
		wantOK    bool
	}{
		{"", 0, true},
		{"?limit=25", 25, true},
		{"?limit=0", 0, false},
		{"?limit=-3", 0, false},
		{"?limit=many", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			rec := httptest.NewRecorder()
			limit, ok := ParseLimit(rec, httptest.NewRequest(http.MethodGet, "/x"+tt.query, nil))
			if limit != tt.wantLimit || ok != tt.wantOK {
				t.Errorf("ParseLimit = %d, %v; want %d, %v", limit, ok, tt.wantLimit, tt.wantOK)
			}
			if !ok && rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
		})
	}
}
