package policy

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"restora.local/restora/internal/location"
)

func withExport(in Input, locations ...string) Input {
	in.ExportTo = locations
	return in
}

func TestPolicyStoresItsExportTargets(t *testing.T) {
	svc := newTestService()
	created := mustCreate(t, svc, withExport(everyMinute("A", "demo"), "l-1", " l-2 ", "l-1", ""))
	if !slices.Equal(created.ExportTo, []string{"l-1", "l-2"}) {
		t.Errorf("exportTo = %v, want duplicates and blanks removed", created.ExportTo)
	}

	updated, err := svc.Update(context.Background(), created.ID, withExport(everyMinute("A", "demo"), "l-2"))
	if err != nil || !slices.Equal(updated.ExportTo, []string{"l-2"}) {
		t.Errorf("updated = %v, %v", updated.ExportTo, err)
	}
	cleared, err := svc.Update(context.Background(), created.ID, everyMinute("A", "demo"))
	if err != nil || len(cleared.ExportTo) != 0 || cleared.ExportTo == nil {
		t.Errorf("cleared = %#v, %v; want an empty list", cleared.ExportTo, err)
	}
}

func TestPolicyRefusesUnknownOrTooManyExportTargets(t *testing.T) {
	svc := newTestService()
	if _, err := svc.Create(context.Background(), withExport(everyMinute("A", "demo"), "l-1", "l-gone")); !errors.Is(err, ErrInvalidExport) {
		t.Errorf("unknown location: error = %v", err)
	}
	many := []string{"l-1", "l-2", "l-3", "l-4", "l-5", "l-6"}
	if _, err := svc.Create(context.Background(), withExport(everyMinute("B", "demo"), many...)); !errors.Is(err, ErrInvalidExport) {
		t.Errorf("too many: error = %v", err)
	}
}

func TestRunNowStartsTheBackupsWithTheExportTargets(t *testing.T) {
	svc := newTestService()
	backups := &fakeBackups{}
	svc.EnableRunNow(backups, discardLog())
	created := mustCreate(t, svc, withExport(everyMinute("A", "demo", "shop", "ghost"), "l-1"))

	result, err := svc.RunNow(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(backups.started, []string{"demo", "shop"}) || !slices.Equal(backups.exportTo, []string{"l-1"}) {
		t.Errorf("started = %v, exportTo = %v", backups.started, backups.exportTo)
	}
	if len(result.Snapshots) != 2 || len(result.Skipped) != 1 || result.Skipped[0].Reason != ReasonApplicationMissing {
		t.Errorf("result = %+v", result)
	}
	if _, err := svc.RunNow(context.Background(), "p-nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown policy: error = %v", err)
	}
}

func TestRunNowNeedsToBeEnabled(t *testing.T) {
	svc := newTestService()
	created := mustCreate(t, svc, everyMinute("A", "demo"))
	if _, err := svc.RunNow(context.Background(), created.ID); err == nil {
		t.Error("without EnableRunNow there is nothing to start backups with")
	}
}

func TestScheduledRunsCarryTheExportTargets(t *testing.T) {
	svc := newTestService()
	p := storedPolicy(t, svc, withExport(everyMinute("A", "demo"), "l-2"))
	backups := &fakeBackups{}
	r := newTestReconciler(svc.store, testApps(), backups, testNow)
	reconcile(t, r, p.Name)
	r.Now = func() time.Time { return testNow.Add(2 * time.Minute) }
	reconcile(t, r, p.Name)

	if !slices.Equal(backups.exportTo, []string{"l-2"}) {
		t.Errorf("exportTo = %v", backups.exportTo)
	}
}

func TestLocationGuard(t *testing.T) {
	svc := newTestService()
	mustCreate(t, svc, withExport(everyMinute("Nightly", "demo"), "l-1"))

	err := svc.LocationGuard(context.Background(), "l-1")
	if !errors.Is(err, location.ErrInUse) || !strings.Contains(err.Error(), "Nightly") {
		t.Errorf("error = %v, want ErrInUse naming the policy", err)
	}
	if err := svc.LocationGuard(context.Background(), "l-2"); err != nil {
		t.Errorf("a location no policy uses: %v", err)
	}
}

func TestHandlerRunAndInvalidExport(t *testing.T) {
	svc := newTestService()
	svc.EnableRunNow(&fakeBackups{}, discardLog())
	mux := http.NewServeMux()
	Register(mux, svc, discardLog())

	rec := do(mux, http.MethodPost, "/api/v1/policies", `{"name":"x","selector":{"mode":"all"},"schedule":{"type":"hourly"},"exportTo":["l-gone"]}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_export") {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}

	rec = do(mux, http.MethodPost, "/api/v1/policies", `{"name":"x","selector":{"mode":"all"},"schedule":{"type":"hourly"},"exportTo":["l-1"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, body %s", rec.Code, rec.Body)
	}
	id := strings.Split(strings.Split(rec.Body.String(), `"id":"`)[1], `"`)[0]
	rec = do(mux, http.MethodPost, "/api/v1/policies/"+id+"/run", "")
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"snapshots":["s-demo"`) {
		t.Errorf("run: status = %d, body %s", rec.Code, rec.Body)
	}
	if rec = do(mux, http.MethodPost, "/api/v1/policies/p-nope/run", ""); rec.Code != http.StatusNotFound {
		t.Errorf("run unknown: status = %d", rec.Code)
	}
}
