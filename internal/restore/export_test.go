package restore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"restora.local/restora/internal/offsite"
	"restora.local/restora/internal/snapshot"
)

// fakeOffsite stands in for the data movers. Every step needs two passes: one to start, one to see it finished.
type fakeOffsite struct {
	w            *world
	prepareCalls int
	volumeCalls  []int
	prepareErr   error
	volumeErr    error
	cleanups     int
}

func (f *fakeOffsite) Prepare(_ context.Context, requestName string, _ *offsite.Record) (bool, error) {
	f.prepareCalls++
	if f.prepareErr != nil {
		return false, f.prepareErr
	}
	if f.prepareCalls < 2 {
		return false, nil
	}
	// Fetching the saved resources puts the file of the backup into the staging directory.
	src := filepath.Join(f.w.exportDir, "s-1", "resources.json.gz")
	dst := filepath.Join(f.w.exportDir, offsite.StageName(requestName))
	data, err := os.ReadFile(src)
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return false, err
	}
	return true, os.WriteFile(filepath.Join(dst, "resources.json.gz"), data, 0o600)
}

func (f *fakeOffsite) Manifest(string) (offsite.Manifest, error) {
	var m offsite.Manifest
	m.Namespace.Labels = map[string]string{"team": "a"}
	m.Namespace.Annotations = map[string]string{"note": "restored"}
	return m, nil
}

func (f *fakeOffsite) RestoreVolume(_ context.Context, _ string, _ *offsite.Record, index int) (bool, error) {
	if f.volumeErr != nil {
		return false, f.volumeErr
	}
	f.volumeCalls = append(f.volumeCalls, index)
	return len(f.volumeCalls)%2 == 0, nil
}

func (f *fakeOffsite) Cleanup(context.Context, string, string) error {
	f.cleanups++
	return nil
}

// exportWorld is the demo application after a disaster: namespace, volume and workloads are gone, only the export is
// left in the storage location.
type exportWorld struct {
	*world
	off     *fakeOffsite
	records *offsite.RecordStore
}

func newExportWorld(t *testing.T) *exportWorld {
	t.Helper()
	w := newWorld(t)
	records := offsite.NewRecordStore(w.client, testNamespace)
	if _, err := records.Create(context.Background(), &offsite.Record{
		ObjectMeta: metav1.ObjectMeta{Name: "x-1"},
		Spec: offsite.RecordSpec{
			Application: "demo", Snapshot: "s-1", Location: "l-1", LocationName: "MinIO", ResourcesSnapshot: "res-1",
			Volumes: []offsite.RecordVolume{{Claim: "data", KopiaSnapshot: "vol-1", SizeBytes: 100}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	off := &fakeOffsite{w: w}
	w.rec.Records, w.rec.Offsite = records, off

	// The disaster: everything of the application is gone, including its namespace and its local backup.
	for _, kind := range []string{"Deployment", "ConfigMap", "Service", "Secret", "PersistentVolumeClaim"} {
		gvr, _ := snapshot.TargetGVR(kind)
		list, _ := w.client.Resource(gvr).Namespace("demo").List(context.Background(), metav1.ListOptions{})
		for _, item := range list.Items {
			w.deleteResource(gvr, item.GetName())
		}
	}
	if err := w.client.Resource(gvrNamespace).Delete(context.Background(), "demo", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := w.rec.Snapshots.Delete(context.Background(), "s-1"); err != nil {
		t.Fatal(err)
	}
	return &exportWorld{world: w, off: off, records: records}
}

func (w *exportWorld) request() {
	w.t.Helper()
	if _, err := w.rec.Store.Create(context.Background(), &RestoreRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "r-1"},
		Spec:       Spec{Export: "x-1", Application: "demo"},
	}); err != nil {
		w.t.Fatalf("create restore: %v", err)
	}
}

// run reconciles until the request ends, and starts the restored deployment the way the cluster would.
func (w *exportWorld) run() *RestoreRequest {
	w.t.Helper()
	for i := 0; i < 40; i++ {
		w.reconcile()
		if w.exists(deploymentGVR(), "db") {
			w.markReady("db", 1)
		}
		req := w.get()
		if req.Status.Phase == snapshot.PhaseSucceeded || req.Status.Phase == snapshot.PhaseFailed {
			return req
		}
	}
	w.t.Fatalf("restore did not finish: %+v", w.get().Status)
	return nil
}

func TestRestoreFromExportRebuildsAWholeDeletedApplication(t *testing.T) {
	w := newExportWorld(t)
	w.request()

	req := w.run()

	if req.Status.Phase != snapshot.PhaseSucceeded {
		t.Fatalf("status = %+v", req.Status)
	}
	ns, err := w.client.Resource(gvrNamespace).Get(context.Background(), "demo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("the namespace must be created again: %v", err)
	}
	if ns.GetLabels()["team"] != "a" || ns.GetAnnotations()["note"] != "restored" {
		t.Errorf("namespace labels %v, annotations %v; they must come back as they were", ns.GetLabels(), ns.GetAnnotations())
	}

	claim := w.resource(gvrClaim, "data")
	if claim.GetAnnotations()[AnnotationRestoredFrom] != "export:x-1" {
		t.Errorf("claim annotations = %v", claim.GetAnnotations())
	}
	if _, found, _ := unstructured.NestedMap(claim.Object, "spec", "dataSource"); found {
		t.Error("the claim must start empty: a data mover fills it, there is no VolumeSnapshot")
	}
	if len(w.off.volumeCalls) < 2 || w.off.volumeCalls[0] != 0 {
		t.Errorf("volume calls = %v", w.off.volumeCalls)
	}
	if !req.Status.Volumes[0].Restored {
		t.Errorf("volumes = %+v", req.Status.Volumes)
	}

	for _, name := range []string{"settings", "db"} {
		kind := map[string]string{"settings": "ConfigMap", "db": "Deployment"}[name]
		gvr, _ := snapshot.TargetGVR(kind)
		if !w.exists(gvr, name) {
			t.Errorf("%s %s was not restored", kind, name)
		}
	}
	if req.Status.Resources == nil || req.Status.Resources.Created < 3 {
		t.Errorf("resources = %+v", req.Status.Resources)
	}
	if w.off.cleanups == 0 {
		t.Error("the data movers and fetched files must be cleaned up when the restore is done")
	}
}

func TestRestoreFromExportWaitsForTheFetchBeforeTouchingAnything(t *testing.T) {
	w := newExportWorld(t)
	w.request()

	w.reconcile() // starts, and the first Prepare says: still fetching

	if _, err := w.client.Resource(gvrNamespace).Get(context.Background(), "demo", metav1.GetOptions{}); err == nil {
		t.Error("the namespace must not be created before the saved resources are here")
	}
	if req := w.get(); req.Status.Phase != snapshot.PhaseRunning || stepPhase(req, StepCheck) == snapshot.PhaseSucceeded {
		t.Errorf("status = %+v", req.Status)
	}
}

func TestRestoreFromExportFailsWithTheReasonOfTheDataMover(t *testing.T) {
	tests := []struct {
		name     string
		prepare  error
		volume   error
		wantCode string
		wantStep string
	}{
		{"the key is missing", &snapshot.Failure{Code: offsite.CodeKeyMissing, Message: "no key", Hint: "restore it"}, nil, offsite.CodeKeyMissing, StepCheck},
		{"the volume cannot be read", nil, &snapshot.Failure{Code: offsite.CodeWrongPassword, Message: "invalid repository password", Hint: "x"}, offsite.CodeWrongPassword, StepVolumes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newExportWorld(t)
			w.off.prepareErr, w.off.volumeErr = tt.prepare, tt.volume
			w.request()

			req := w.run()
			if req.Status.Phase != snapshot.PhaseFailed || req.Status.ErrorCode != tt.wantCode || req.Status.Hint == "" || stepPhase(req, tt.wantStep) != snapshot.PhaseFailed {
				t.Errorf("status = %+v", req.Status)
			}
			if w.off.cleanups == 0 {
				t.Error("a failed restore must clean up the data movers too")
			}
		})
	}
}

func TestRestoreFromExportOfAnUnknownExport(t *testing.T) {
	w := newExportWorld(t)
	if err := w.records.Delete(context.Background(), "x-1"); err != nil {
		t.Fatal(err)
	}
	w.request()

	req := w.run()
	if req.Status.ErrorCode != "export_not_found" {
		t.Errorf("status = %+v", req.Status)
	}
}

func TestRestoreFromExportKeepsAnExistingNamespace(t *testing.T) {
	w := newExportWorld(t)
	existing := object("v1", "Namespace", "", "demo", nil)
	existing.SetLabels(map[string]string{"mine": "yes"})
	if _, err := w.client.Resource(gvrNamespace).Create(context.Background(), existing, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	w.request()

	req := w.run()
	if req.Status.Phase != snapshot.PhaseSucceeded {
		t.Fatalf("status = %+v", req.Status)
	}
	ns, _ := w.client.Resource(gvrNamespace).Get(context.Background(), "demo", metav1.GetOptions{})
	if ns.GetLabels()["mine"] != "yes" || ns.GetLabels()["team"] != "" {
		t.Errorf("an existing namespace must stay as it is: %v", ns.GetLabels())
	}
}

func TestRestoreFromExportReplacesAnOldClaim(t *testing.T) {
	w := newExportWorld(t)
	// The application was set up again by hand, with an empty volume of the same name.
	if _, err := w.client.Resource(gvrNamespace).Create(context.Background(), object("v1", "Namespace", "", "demo", nil), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.client.Resource(gvrClaim).Namespace("demo").Create(context.Background(),
		object("v1", "PersistentVolumeClaim", "demo", "data", map[string]any{"spec": map[string]any{"volumeName": "pv-new"}}), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	w.request()

	req := w.run()
	if req.Status.Phase != snapshot.PhaseSucceeded {
		t.Fatalf("status = %+v", req.Status)
	}
	claim := w.resource(gvrClaim, "data")
	if claim.GetAnnotations()[AnnotationRestoredFrom] != "export:x-1" {
		t.Errorf("the old claim must be replaced by one that the restore filled: %v", claim.GetAnnotations())
	}
}

func TestRestoreFromExportHasALongerTimeLimit(t *testing.T) {
	w := newExportWorld(t)
	w.rec.Timeout = 1
	w.request()
	w.reconcile()

	req := w.get()
	if got := w.rec.timeoutFor(req); got != DefaultExportTimeout {
		t.Errorf("timeout = %v, want %v for a restore from a storage location", got, DefaultExportTimeout)
	}
	if got := w.rec.timeoutFor(&RestoreRequest{Spec: Spec{Snapshot: "s-1"}}); got != 1 {
		t.Errorf("timeout = %v, a local restore keeps its own limit", got)
	}
}

func TestCreateFromExport(t *testing.T) {
	w := newExportWorld(t)
	svc := NewService(w.rec.Store, w.rec.Snapshots).WithExports(w.records)
	svc.now = func() time.Time { return testNow }

	view, err := svc.CreateFromExport(context.Background(), "x-1")
	if err != nil {
		t.Fatal(err)
	}
	if view.Application != "demo" || view.Export == nil || view.Export.ID != "x-1" || view.Export.LocationName != "MinIO" || view.Snapshot != "" {
		t.Errorf("view = %+v", view)
	}
	req, _ := svc.store.Get(context.Background(), view.ID)
	if req.Labels[LabelExport] != "x-1" || req.Labels[LabelApplication] != "demo" {
		t.Errorf("labels = %v", req.Labels)
	}

	if _, err := svc.CreateFromExport(context.Background(), "x-1"); !errors.Is(err, ErrInProgress) {
		t.Errorf("second restore while one runs: error = %v", err)
	}
	if _, err := svc.CreateFromExport(context.Background(), "x-nope"); !errors.Is(err, offsite.ErrNotFound) {
		t.Errorf("unknown export: error = %v", err)
	}
}

func TestCreateFromExportRefusesWhileABackupRuns(t *testing.T) {
	w := newExportWorld(t)
	svc := NewService(w.rec.Store, w.rec.Snapshots).WithExports(w.records)
	if _, err := w.rec.Snapshots.Create(context.Background(), &snapshot.Snapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "s-2", Labels: map[string]string{snapshot.LabelApplication: "demo"}},
		Spec:       snapshot.Spec{Application: "demo"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateFromExport(context.Background(), "x-1"); !errors.Is(err, ErrSnapshotRunning) {
		t.Errorf("error = %v, want ErrSnapshotRunning", err)
	}
}

func TestHandlerRestoreFromExport(t *testing.T) {
	w := newExportWorld(t)
	svc := NewService(w.rec.Store, w.rec.Snapshots).WithExports(w.records)
	mux := http.NewServeMux()
	Register(mux, svc, discardLog())

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/exports/x-1/restores", nil))
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"export":{"id":"x-1"`) {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/exports/x-1/restores", nil))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "restore_in_progress") {
		t.Errorf("second: status = %d, body %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/exports/x-nope/restores", nil))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "export_not_found") {
		t.Errorf("unknown: status = %d, body %s", rec.Code, rec.Body)
	}
}

func deploymentGVR() schema.GroupVersionResource {
	gvr, _ := snapshot.TargetGVR("Deployment")
	return gvr
}
