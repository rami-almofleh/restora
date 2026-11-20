package location

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestMux() (*http.ServeMux, *world) {
	w := newWorld()
	mux := http.NewServeMux()
	Register(mux, w.svc, discardLog())
	return mux, w
}

func do(mux *http.ServeMux, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

const s3Body = `{"name":"MinIO","type":"s3","s3":{"endpoint":"http://minio.minio:9000","bucket":"restora","pathStyle":true,"skipTLSVerify":false,"accessKey":"AKIATEST","secretKey":"very-secret-value"}}`

func TestHandlerLifecycle(t *testing.T) {
	mux, _ := newTestMux()

	rec := do(mux, http.MethodPost, "/api/v1/locations", s3Body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d; body %s", rec.Code, rec.Body)
	}
	raw := rec.Body.String()
	for _, secret := range []string{"AKIATEST", "very-secret-value"} {
		if strings.Contains(raw, secret) {
			t.Errorf("the answer contains %q: %s", secret, raw)
		}
	}
	if !strings.Contains(raw, `"credentialsSet":true`) || !strings.Contains(raw, `"checks":[]`) {
		t.Errorf("answer = %s", raw)
	}
	var created View
	if err := json.Unmarshal([]byte(raw), &created); err != nil {
		t.Fatal(err)
	}

	rec = do(mux, http.MethodGet, "/api/v1/locations", "")
	var page Page
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || page.Total != 1 || strings.Contains(rec.Body.String(), "very-secret-value") {
		t.Fatalf("list = %s, %v", rec.Body, err)
	}

	rec = do(mux, http.MethodGet, "/api/v1/locations/"+created.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get: status = %d", rec.Code)
	}

	rec = do(mux, http.MethodPut, "/api/v1/locations/"+created.ID, strings.Replace(strings.Replace(s3Body, "MinIO", "Renamed", 1), `,"accessKey":"AKIATEST","secretKey":"very-secret-value"`, "", 1))
	var updated View
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil || rec.Code != http.StatusOK || updated.Name != "Renamed" {
		t.Fatalf("update: status %d, %s", rec.Code, rec.Body)
	}

	rec = do(mux, http.MethodPost, "/api/v1/locations/"+created.ID+"/test", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("test: status = %d; body %s", rec.Code, rec.Body)
	}

	if rec = do(mux, http.MethodDelete, "/api/v1/locations/"+created.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d", rec.Code)
	}
	if rec = do(mux, http.MethodGet, "/api/v1/locations/"+created.ID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: status = %d", rec.Code)
	}
}

func TestHandlerErrors(t *testing.T) {
	mux, _ := newTestMux()
	do(mux, http.MethodPost, "/api/v1/locations", s3Body)

	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		wantCode int
		wantErr  string
	}{
		{"duplicate name", "POST", "/api/v1/locations", s3Body, http.StatusConflict, "location_name_taken"},
		{"invalid bucket", "POST", "/api/v1/locations", strings.Replace(s3Body, `"restora"`, `"X"`, 1), http.StatusBadRequest, "invalid_location"},
		{"not json", "POST", "/api/v1/locations", `{`, http.StatusBadRequest, "invalid_body"},
		{"unknown field", "POST", "/api/v1/locations", `{"name":"x","admin":true}`, http.StatusBadRequest, "invalid_body"},
		{"unknown location", "GET", "/api/v1/locations/l-nope", "", http.StatusNotFound, "location_not_found"},
		{"test unknown", "POST", "/api/v1/locations/l-nope/test", "", http.StatusNotFound, "location_not_found"},
		{"delete unknown", "DELETE", "/api/v1/locations/l-nope", "", http.StatusNotFound, "location_not_found"},
		{"bad limit", "GET", "/api/v1/locations?limit=0", "", http.StatusBadRequest, "invalid_limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(mux, tt.method, tt.path, tt.body)
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.wantCode, rec.Body)
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
			if strings.Contains(body.Error.Message, "very-secret-value") {
				t.Error("an error must never repeat the keys")
			}
		})
	}
}

func TestHandlerRefusesToDeleteALocationThatIsInUse(t *testing.T) {
	mux, w := newTestMux()
	rec := do(mux, http.MethodPost, "/api/v1/locations", s3Body)
	var created View
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	w.svc.AddDeleteGuard(func(context.Context, string) error { return fmt.Errorf("%w: policy", ErrInUse) })

	rec = do(mux, http.MethodDelete, "/api/v1/locations/"+created.ID, "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "location_in_use") {
		t.Errorf("status = %d, body %s", rec.Code, rec.Body)
	}
}
