package policy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"restora.local/restora/internal/snapshot"
)

func newTestMux() *http.ServeMux {
	mux := http.NewServeMux()
	Register(mux, newTestService(), discardLog())
	return mux
}

func do(mux *http.ServeMux, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	mux.ServeHTTP(rec, req)
	return rec
}

const validBody = `{"name":"Nightly","selector":{"mode":"applications","applications":["demo"]},"schedule":{"type":"daily","minute":0,"hour":2,"weekday":0}}`

func TestHandlerLifecycle(t *testing.T) {
	mux := newTestMux()

	rec := do(mux, http.MethodPost, "/api/v1/policies", validBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d; body %s", rec.Code, rec.Body)
	}
	raw := rec.Body.String()
	var created View
	if err := json.Unmarshal([]byte(raw), &created); err != nil || created.Cron != "0 2 * * *" {
		t.Fatalf("created = %+v, %v", created, err)
	}
	if !strings.Contains(raw, `"missing":[]`) {
		t.Errorf("empty lists must be [] not null: %s", raw)
	}
	id := created.ID

	rec = do(mux, http.MethodGet, "/api/v1/policies", "")
	var page Page
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("list = %s", rec.Body)
	}

	rec = do(mux, http.MethodPost, "/api/v1/policies/"+id+"/pause", "")
	var paused View
	if err := json.Unmarshal(rec.Body.Bytes(), &paused); err != nil || !paused.Paused {
		t.Fatalf("pause = %d %s", rec.Code, rec.Body)
	}
	rec = do(mux, http.MethodPost, "/api/v1/policies/"+id+"/resume", "")
	var resumed View
	if err := json.Unmarshal(rec.Body.Bytes(), &resumed); err != nil || resumed.Paused {
		t.Fatalf("resume = %d %s", rec.Code, rec.Body)
	}

	rec = do(mux, http.MethodPut, "/api/v1/policies/"+id, strings.Replace(validBody, "Nightly", "Renamed", 1))
	var updated View
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil || updated.Name != "Renamed" {
		t.Fatalf("update = %d %s", rec.Code, rec.Body)
	}

	if rec = do(mux, http.MethodDelete, "/api/v1/policies/"+id, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d", rec.Code)
	}
	if rec = do(mux, http.MethodGet, "/api/v1/policies/"+id, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: status = %d", rec.Code)
	}
}

func TestHandlerErrors(t *testing.T) {
	mux := newTestMux()
	do(mux, http.MethodPost, "/api/v1/policies", validBody)

	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		wantCode int
		wantErr  string
	}{
		{"duplicate name", "POST", "/api/v1/policies", validBody, http.StatusConflict, "policy_name_taken"},
		{"empty name", "POST", "/api/v1/policies", strings.Replace(validBody, "Nightly", " ", 1), http.StatusBadRequest, "invalid_name"},
		{"bad cron", "POST", "/api/v1/policies", `{"name":"x","selector":{"mode":"all"},"schedule":{"type":"custom","cron":"nope"}}`, http.StatusBadRequest, "invalid_schedule"},
		{"bad selector", "POST", "/api/v1/policies", `{"name":"x","selector":{"mode":"labels"},"schedule":{"type":"hourly"}}`, http.StatusBadRequest, "invalid_selector"},
		{"not json", "POST", "/api/v1/policies", `{`, http.StatusBadRequest, "invalid_body"},
		{"unknown field", "POST", "/api/v1/policies", `{"name":"x","admin":true}`, http.StatusBadRequest, "invalid_body"},
		{"unknown policy", "GET", "/api/v1/policies/p-nope", "", http.StatusNotFound, "policy_not_found"},
		{"pause unknown", "POST", "/api/v1/policies/p-nope/pause", "", http.StatusNotFound, "policy_not_found"},
		{"delete unknown", "DELETE", "/api/v1/policies/p-nope", "", http.StatusNotFound, "policy_not_found"},
		{"bad limit", "GET", "/api/v1/policies?limit=0", "", http.StatusBadRequest, "invalid_limit"},
		{"preview bad cron", "POST", "/api/v1/policies/preview", `{"selector":{"mode":"all"},"schedule":{"type":"custom","cron":"nope"}}`, http.StatusBadRequest, "invalid_schedule"},
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
		})
	}
}

func TestHandlerPreview(t *testing.T) {
	mux := newTestMux()
	rec := do(mux, http.MethodPost, "/api/v1/policies/preview",
		`{"selector":{"mode":"labels","matchLabels":{"team":"a"}},"schedule":{"type":"hourly","minute":10}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body)
	}
	var preview Preview
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil || preview.Cron != "10 * * * *" ||
		len(preview.NextRuns) != PreviewRuns || len(preview.Applications) != 2 {
		t.Errorf("preview = %+v, %v", preview, err)
	}
}

func TestHandlerClusterError(t *testing.T) {
	mux := http.NewServeMux()
	svc := NewService(newStore(), fakeApps{err: errBoom}, &fakeSnapshots{}, fakeLocations{})
	Register(mux, svc, discardLog())
	if rec := do(mux, http.MethodGet, "/api/v1/policies", ""); rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}

func TestHandlerRetentionPreview(t *testing.T) {
	svc := newTestService()
	svc.snapshots = &fakeSnapshots{items: []snapshot.Snapshot{
		ok("demo", testNow), ok("demo", testNow.Add(-time.Minute)), ok("demo", testNow.Add(-2*time.Minute)),
	}}
	mux := http.NewServeMux()
	Register(mux, svc, discardLog())

	created := do(mux, http.MethodPost, "/api/v1/policies", validBody)
	var view View
	if err := json.Unmarshal(created.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	body := `{"name":"x","selector":{"mode":"all"},"schedule":{"type":"daily","hour":2},"retention":{"last":1}}`

	rec := do(mux, http.MethodPost, "/api/v1/policies/"+view.ID+"/retention-preview", body)
	var plan Plan
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil || rec.Code != http.StatusOK || len(plan.Keep) != 1 || len(plan.Delete) != 2 {
		t.Fatalf("status %d, plan %+v, %v; body %s", rec.Code, plan, err, rec.Body)
	}

	rec = do(mux, http.MethodPost, "/api/v1/policies/p-nope/retention-preview", body)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown policy: status = %d", rec.Code)
	}
	rec = do(mux, http.MethodPost, "/api/v1/policies/"+view.ID+"/retention-preview", strings.Replace(body, `"last":1`, `"last":-4`, 1))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_retention") {
		t.Errorf("invalid retention: status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestHandlerStoresRetention(t *testing.T) {
	mux := newTestMux()
	body := `{"name":"R","selector":{"mode":"all"},"schedule":{"type":"daily","hour":2},"retention":{"last":2,"daily":7}}`
	rec := do(mux, http.MethodPost, "/api/v1/policies", body)
	var view View
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil || rec.Code != http.StatusCreated || view.Retention.Last != 2 || view.Retention.Daily != 7 {
		t.Fatalf("status %d, view %+v, %v", rec.Code, view.Retention, err)
	}

	tooMany := strings.Replace(strings.Replace(body, `"R"`, `"R2"`, 1), `"daily":7`, `"daily":5000`, 1)
	rec = do(mux, http.MethodPost, "/api/v1/policies", tooMany)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_retention") {
		t.Errorf("too large: status = %d, body %s", rec.Code, rec.Body)
	}
}
