package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSPAHandler(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":      {Data: []byte("<html>app</html>")},
		"assets/app-1.js": {Data: []byte("console.log(1)")},
		"theme-init.js":   {Data: []byte("// theme")},
	}
	h := newSPAHandler(fsys)

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
		wantCache  string
	}{
		{name: "root serves index", path: "/", wantStatus: http.StatusOK, wantBody: "app", wantCache: "no-cache"},
		{name: "hashed asset is cached long", path: "/assets/app-1.js", wantStatus: http.StatusOK, wantBody: "console.log", wantCache: "public, max-age=31536000, immutable"},
		{name: "plain file is not cached", path: "/theme-init.js", wantStatus: http.StatusOK, wantBody: "// theme", wantCache: "no-cache"},
		{name: "client route falls back to index", path: "/applications", wantStatus: http.StatusOK, wantBody: "app", wantCache: "no-cache"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.wantBody)
			}
			if got := rec.Header().Get("Cache-Control"); got != tt.wantCache {
				t.Errorf("Cache-Control = %q, want %q", got, tt.wantCache)
			}
		})
	}
}

func TestSPAHandlerWithoutBuild(t *testing.T) {
	h := newSPAHandler(fstest.MapFS{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}
