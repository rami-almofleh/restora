package activity

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandler(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, testService(), slog.New(slog.NewTextHandler(io.Discard, nil)))

	tests := []struct {
		name      string
		query     string
		wantCode  int
		wantErr   string
		wantItems int
	}{
		{"all", "", http.StatusOK, "", 5},
		{"filtered", "?type=backup&status=failed&q=shop", http.StatusOK, "", 1},
		{"limit", "?limit=2", http.StatusOK, "", 2},
		{"unknown type", "?type=policy", http.StatusBadRequest, "invalid_filter", 0},
		{"unknown status", "?status=done", http.StatusBadRequest, "invalid_filter", 0},
		{"bad limit", "?limit=0", http.StatusBadRequest, "invalid_limit", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/activities"+tt.query, nil))
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.wantCode, rec.Body)
			}
			if tt.wantErr != "" {
				var body struct {
					Error struct{ Code, Message, Hint string }
				}
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || body.Error.Code != tt.wantErr || body.Error.Hint == "" {
					t.Errorf("error = %+v (%v), want code %q with hint", body.Error, err, tt.wantErr)
				}
				return
			}
			var page Page
			if err := json.NewDecoder(rec.Body).Decode(&page); err != nil || len(page.Items) != tt.wantItems {
				t.Errorf("items = %d (%v), want %d", len(page.Items), err, tt.wantItems)
			}
		})
	}
}

func TestHandlerClusterError(t *testing.T) {
	mux := http.NewServeMux()
	svc := NewService(fakeBackups{err: io.ErrUnexpectedEOF}, fakeRestores{})
	Register(mux, svc, slog.New(slog.NewTextHandler(io.Discard, nil)))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/activities", nil))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}
