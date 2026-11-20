package offsite

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"restora.local/restora/internal/snapshot"
)

func newRecords(t *testing.T) *RecordStore {
	t.Helper()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{GVR: "ExportList"})
	store := NewRecordStore(client, testNamespace)
	add := func(snap, app, loc string, at time.Time, size int64) {
		if _, err := store.Create(context.Background(), &Record{
			ObjectMeta: metav1.ObjectMeta{Name: RecordName(snap, loc)},
			Spec: RecordSpec{
				Application: app, Snapshot: snap, Location: loc, LocationName: "Storage " + loc, CreatedAt: metav1.NewTime(at),
				ResourcesSnapshot: "res", Resources: snapshot.ResourceStats{Count: 4},
				Volumes: []RecordVolume{{Claim: "data", KopiaSnapshot: "vol", SizeBytes: size}}, AddedBytes: 10,
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	add("s-20261120-100000-aaaa", "demo", "l-20261120-090000-1111", testNow.Add(-3*time.Hour), 100)
	add("s-20261120-110000-bbbb", "demo", "l-20261120-090000-1111", testNow.Add(-2*time.Hour), 120)
	add("s-20261120-110000-bbbb", "demo", "l-20261120-090000-2222", testNow.Add(-2*time.Hour), 120)
	add("s-20261120-120000-cccc", "shop", "l-20261120-090000-1111", testNow.Add(-time.Hour), 7)
	return store
}

func TestRecordNameStartsWithTheTimeOfTheBackup(t *testing.T) {
	if got := RecordName("s-20261120-110000-bbbb", "l-20261120-090000-2222"); got != "x-20261120-110000-bbbb-2222" {
		t.Errorf("RecordName = %q", got)
	}
}

func TestRecordListFiltersAndSortsNewestFirst(t *testing.T) {
	svc := NewRecordService(newRecords(t))
	ctx := context.Background()

	all, err := svc.List(ctx, "", "", 0, "")
	if err != nil || all.Total != 4 || len(all.Items) != 4 || all.Items[0].Application != "shop" {
		t.Fatalf("all = %+v, %v", all, err)
	}
	demo, _ := svc.List(ctx, "demo", "", 0, "")
	if demo.Total != 3 {
		t.Errorf("demo = %d", demo.Total)
	}
	one, _ := svc.List(ctx, "demo", "l-20261120-090000-2222", 0, "")
	if one.Total != 1 || one.Items[0].Location.Name != "Storage l-20261120-090000-2222" {
		t.Errorf("one = %+v", one)
	}
	first, _ := svc.List(ctx, "", "", 3, "")
	second, _ := svc.List(ctx, "", "", 3, first.NextCursor)
	if len(first.Items) != 3 || len(second.Items) != 1 || second.NextCursor != "" {
		t.Errorf("paging: %d then %d", len(first.Items), len(second.Items))
	}
}

func TestRecordViewHidesRepositoryIds(t *testing.T) {
	svc := NewRecordService(newRecords(t))
	page, _ := svc.List(context.Background(), "shop", "", 0, "")
	raw, _ := json.Marshal(page.Items[0])
	if strings.Contains(string(raw), "kopia") || strings.Contains(string(raw), `"res"`) || strings.Contains(string(raw), `"vol"`) {
		t.Errorf("the view must not carry repository ids: %s", raw)
	}
	if v := page.Items[0]; v.SizeBytes != 7 || v.Resources != 4 || v.AddedBytes != 10 || len(v.Volumes) != 1 {
		t.Errorf("view = %+v", v)
	}
}

func TestExportedApplicationsAndCounts(t *testing.T) {
	svc := NewRecordService(newRecords(t))
	apps, err := svc.Applications(context.Background())
	if err != nil || len(apps) != 2 || apps[0].Application != "demo" || apps[0].Exports != 3 || apps[1].Application != "shop" {
		t.Fatalf("applications = %+v, %v", apps, err)
	}
	if !apps[0].LatestAt.Equal(testNow.Add(-2 * time.Hour)) {
		t.Errorf("latest = %v", apps[0].LatestAt)
	}
	counts, _ := svc.CountByLocation(context.Background())
	if counts["l-20261120-090000-1111"] != 3 || counts["l-20261120-090000-2222"] != 1 {
		t.Errorf("counts = %v", counts)
	}
}

func TestRecordHandler(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRecords(mux, NewRecordService(newRecords(t)), discardLog())
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	if rec := get("/api/v1/exports?application=demo"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"total":3`) {
		t.Errorf("list: %d %s", rec.Code, rec.Body)
	}
	if rec := get("/api/v1/exports/x-20261120-120000-cccc-1111"); rec.Code != http.StatusOK {
		t.Errorf("get: %d %s", rec.Code, rec.Body)
	}
	if rec := get("/api/v1/exports/x-nope"); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "export_not_found") {
		t.Errorf("unknown: %d %s", rec.Code, rec.Body)
	}
	if rec := get("/api/v1/exports?limit=0"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad limit: %d", rec.Code)
	}
	if rec := get("/api/v1/export-applications"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"application":"shop"`) {
		t.Errorf("applications: %d %s", rec.Code, rec.Body)
	}
}
