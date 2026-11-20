package activity

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"restora.local/restora/internal/restore"
	"restora.local/restora/internal/snapshot"
)

type fakeBackups struct {
	items []snapshot.Snapshot
	err   error
}

func (f fakeBackups) All(context.Context) ([]snapshot.Snapshot, error) { return f.items, f.err }

type fakeRestores struct {
	items []restore.RestoreRequest
	err   error
}

func (f fakeRestores) All(context.Context) ([]restore.RestoreRequest, error) { return f.items, f.err }

var created = time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)

func backup(name, app string, phase snapshot.Phase) snapshot.Snapshot {
	return snapshot.Snapshot{
		ObjectMeta: metav1.ObjectMeta{Name: name, CreationTimestamp: metav1.NewTime(created)},
		Spec:       snapshot.Spec{Application: app},
		Status:     snapshot.Status{Phase: phase},
	}
}

func restoreRequest(name, app, snap string, phase snapshot.Phase) restore.RestoreRequest {
	return restore.RestoreRequest{
		ObjectMeta: metav1.ObjectMeta{Name: name, CreationTimestamp: metav1.NewTime(created)},
		Spec:       restore.Spec{Application: app, Snapshot: snap},
		Status:     restore.Status{Phase: phase},
	}
}

func testService() *Service {
	return NewService(
		fakeBackups{items: []snapshot.Snapshot{
			backup("s-20261119-100000-aaaa", "demo", snapshot.PhaseSucceeded),
			backup("s-20261119-100200-bbbb", "shop", snapshot.PhaseFailed),
			backup("s-20261119-100400-cccc", "demo", snapshot.PhaseRunning),
		}},
		fakeRestores{items: []restore.RestoreRequest{
			restoreRequest("r-20261119-100100-dddd", "demo", "s-20261119-100000-aaaa", snapshot.PhaseSucceeded),
			restoreRequest("r-20261119-100300-eeee", "shop", "s-x", snapshot.PhaseFailed),
		}},
	)
}

func ids(page Page) []string {
	out := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		out = append(out, item.ID)
	}
	return out
}

func TestListMergesBackupsAndRestoresNewestFirst(t *testing.T) {
	page, err := testService().List(context.Background(), Filter{}, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"s-20261119-100400-cccc", "r-20261119-100300-eeee", "s-20261119-100200-bbbb",
		"r-20261119-100100-dddd", "s-20261119-100000-aaaa",
	}
	if got := ids(page); !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	if page.Total != 5 {
		t.Errorf("total = %d, want 5", page.Total)
	}
}

func TestListFilters(t *testing.T) {
	tests := []struct {
		name        string
		filter      Filter
		want        []string
		wantCounts  Counts
		wantTotalAs int
	}{
		{"backups only", Filter{Type: TypeBackup},
			[]string{"s-20261119-100400-cccc", "s-20261119-100200-bbbb", "s-20261119-100000-aaaa"},
			Counts{Total: 3, Active: 1, Succeeded: 1, Failed: 1}, 3},
		{"restores only", Filter{Type: TypeRestore},
			[]string{"r-20261119-100300-eeee", "r-20261119-100100-dddd"},
			Counts{Total: 2, Succeeded: 1, Failed: 1}, 2},
		{"failed", Filter{Status: StatusFailed},
			[]string{"r-20261119-100300-eeee", "s-20261119-100200-bbbb"},
			Counts{Total: 5, Active: 1, Succeeded: 2, Failed: 2}, 2},
		{"active", Filter{Status: StatusActive},
			[]string{"s-20261119-100400-cccc"},
			Counts{Total: 5, Active: 1, Succeeded: 2, Failed: 2}, 1},
		{"application search ignores case", Filter{Query: "SHO"},
			[]string{"r-20261119-100300-eeee", "s-20261119-100200-bbbb"},
			Counts{Total: 2, Failed: 2}, 2},
		{"search and status", Filter{Query: "demo", Status: StatusSucceeded},
			[]string{"r-20261119-100100-dddd", "s-20261119-100000-aaaa"},
			Counts{Total: 3, Active: 1, Succeeded: 2}, 2},
		{"nothing matches", Filter{Query: "zzz"}, []string{}, Counts{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := testService().List(context.Background(), tt.filter, 0, "")
			if err != nil {
				t.Fatal(err)
			}
			if got := ids(page); !slices.Equal(got, tt.want) {
				t.Errorf("items = %v, want %v", got, tt.want)
			}
			if page.Counts != tt.wantCounts {
				t.Errorf("counts = %+v, want %+v", page.Counts, tt.wantCounts)
			}
			if page.Total != tt.wantTotalAs {
				t.Errorf("total = %d, want %d", page.Total, tt.wantTotalAs)
			}
			if page.Items == nil {
				t.Error("items must be an empty list, not nil")
			}
		})
	}
}

func TestListPaging(t *testing.T) {
	svc := testService()
	first, err := svc.List(context.Background(), Filter{}, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %v, cursor %q", ids(first), first.NextCursor)
	}
	second, err := svc.List(context.Background(), Filter{}, 2, first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	third, err := svc.List(context.Background(), Filter{}, 2, second.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	got := slices.Concat(ids(first), ids(second), ids(third))
	if len(got) != 5 || third.NextCursor != "" {
		t.Fatalf("pages = %v, last cursor %q", got, third.NextCursor)
	}
	seen := map[string]bool{}
	for _, id := range got {
		if seen[id] {
			t.Errorf("%s appears on two pages", id)
		}
		seen[id] = true
	}
}

func TestListDetails(t *testing.T) {
	b := backup("s-20261119-100000-aaaa", "demo", snapshot.PhaseSucceeded)
	b.Status.Volumes = []snapshot.VolumeStatus{{Claim: "data", SizeBytes: 1000}, {Claim: "logs", SizeBytes: 24}}
	b.Status.Resources = &snapshot.ResourceStats{Count: 7, Bytes: 5}
	r := restoreRequest("r-20261119-100100-dddd", "demo", "s-20261119-100000-aaaa", snapshot.PhaseSucceeded)
	r.Status.Resources = &restore.ResourceResult{Created: 2, Updated: 5}
	r.Status.Warnings = []string{"Role/x: forbidden"}

	page, err := NewService(fakeBackups{items: []snapshot.Snapshot{b}}, fakeRestores{items: []restore.RestoreRequest{r}}).
		List(context.Background(), Filter{}, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	restoreItem, backupItem := page.Items[0], page.Items[1]
	if d := backupItem.Backup; d == nil || d.Volumes != 2 || d.Resources != 7 || d.Bytes != 1024 || backupItem.Restore != nil {
		t.Errorf("backup details = %+v", d)
	}
	if d := restoreItem.Restore; d == nil || d.SnapshotCreatedAt == nil || !d.SnapshotCreatedAt.Equal(created) || d.Snapshot != "s-20261119-100000-aaaa" || d.Created != 2 || d.Updated != 5 || len(d.Warnings) != 1 || restoreItem.Backup != nil {
		t.Errorf("restore details = %+v", d)
	}
}

func TestListShowsPolicyOfScheduledBackup(t *testing.T) {
	b := backup("s-20261119-100000-aaaa", "demo", snapshot.PhaseSucceeded)
	b.Spec.Policy = &snapshot.PolicyRef{ID: "p-1", Name: "Nightly"}
	page, err := NewService(fakeBackups{items: []snapshot.Snapshot{b}}, fakeRestores{}).List(context.Background(), Filter{}, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if p := page.Items[0].Policy; p == nil || p.Name != "Nightly" {
		t.Errorf("policy = %+v", p)
	}
}

func TestListRestoreOfDeletedBackup(t *testing.T) {
	r := restoreRequest("r-20261119-100100-dddd", "demo", "s-gone", snapshot.PhaseSucceeded)
	page, err := NewService(fakeBackups{}, fakeRestores{items: []restore.RestoreRequest{r}}).List(context.Background(), Filter{}, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if d := page.Items[0].Restore; d == nil || d.SnapshotCreatedAt != nil {
		t.Errorf("restore details = %+v, want no backup time", d)
	}
}

func TestListPendingWithoutStatus(t *testing.T) {
	svc := NewService(fakeBackups{items: []snapshot.Snapshot{backup("s-20261119-100000-aaaa", "demo", "")}}, fakeRestores{})
	page, err := svc.List(context.Background(), Filter{Status: StatusActive}, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Phase != snapshot.PhasePending {
		t.Errorf("items = %+v, want one pending entry", page.Items)
	}
}

func TestListSourceErrors(t *testing.T) {
	boom := errors.New("boom")
	if _, err := NewService(fakeBackups{err: boom}, fakeRestores{}).List(context.Background(), Filter{}, 0, ""); !errors.Is(err, boom) {
		t.Errorf("backup error = %v", err)
	}
	if _, err := NewService(fakeBackups{}, fakeRestores{err: boom}).List(context.Background(), Filter{}, 0, ""); !errors.Is(err, boom) {
		t.Errorf("restore error = %v", err)
	}
}
