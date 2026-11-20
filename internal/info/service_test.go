package info

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestServiceGet(t *testing.T) {
	zone := time.FixedZone("test", 2*60*60)
	tests := []struct {
		name      string
		version   string
		startedAt time.Time
		wantUTC   time.Time
	}{
		{
			name:      "stores start time in UTC",
			version:   "1.2.3",
			startedAt: time.Date(2026, 11, 17, 12, 0, 0, 0, zone),
			wantUTC:   time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC),
		},
		{
			name:      "keeps dev version",
			version:   "dev",
			startedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			wantUTC:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := New(tt.version, tt.startedAt).Get()
			if got.Name != "Restora" {
				t.Errorf("Name = %q, want %q", got.Name, "Restora")
			}
			if got.Version != tt.version {
				t.Errorf("Version = %q, want %q", got.Version, tt.version)
			}
			if !got.StartedAt.Equal(tt.wantUTC) || got.StartedAt.Location() != time.UTC {
				t.Errorf("StartedAt = %v, want %v in UTC", got.StartedAt, tt.wantUTC)
			}
		})
	}
}

func TestHandler(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, New("1.2.3", time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/info", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got Info
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Version != "1.2.3" {
		t.Errorf("Version = %q, want %q", got.Version, "1.2.3")
	}
}
