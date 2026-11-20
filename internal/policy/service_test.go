package policy

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceCreateAndGet(t *testing.T) {
	svc := newTestService()
	created := mustCreate(t, svc, everyMinute("  Every minute ", "demo", "gone"))

	if created.Name != "Every minute" || created.Cron != "* * * * *" || created.Paused {
		t.Errorf("created = %+v", created)
	}
	if len(created.Applications) != 1 || created.Applications[0] != "demo" || len(created.Missing) != 1 || created.Missing[0] != "gone" {
		t.Errorf("applications = %v, missing = %v", created.Applications, created.Missing)
	}

	got, err := svc.Get(context.Background(), created.ID)
	if err != nil || got.ID != created.ID {
		t.Fatalf("Get = %+v, %v", got, err)
	}
}

func TestServiceCreateRejects(t *testing.T) {
	svc := newTestService()
	mustCreate(t, svc, everyMinute("Nightly", "demo"))

	tests := []struct {
		name string
		in   Input
		want error
	}{
		{"same name, other case", everyMinute("nightly", "shop"), ErrNameTaken},
		{"empty name", everyMinute("  ", "shop"), ErrInvalidName},
		{"name too long", everyMinute(string(make([]rune, MaxNameLength+1)), "shop"), ErrInvalidName},
		{"bad schedule", Input{Name: "x", Selector: Selector{Mode: SelectAll}, Schedule: Schedule{Type: ScheduleCustom, Cron: "nope"}}, ErrInvalidSchedule},
		{"bad selector", Input{Name: "x", Selector: Selector{Mode: SelectApplications}, Schedule: Schedule{Type: ScheduleHourly}}, ErrInvalidSelector},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := svc.Create(context.Background(), tt.in); !errors.Is(err, tt.want) {
				t.Errorf("Create error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestServiceUpdate(t *testing.T) {
	svc := newTestService()
	a := mustCreate(t, svc, everyMinute("A", "demo"))
	mustCreate(t, svc, everyMinute("B", "shop"))

	updated, err := svc.Update(context.Background(), a.ID, Input{
		Name:     "A renamed",
		Selector: Selector{Mode: SelectAll},
		Schedule: Schedule{Type: ScheduleDaily, Hour: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "A renamed" || updated.Cron != "0 2 * * *" || len(updated.Applications) != 3 {
		t.Errorf("updated = %+v", updated)
	}

	if _, err := svc.Update(context.Background(), a.ID, everyMinute("b", "demo")); !errors.Is(err, ErrNameTaken) {
		t.Errorf("rename to a taken name: error = %v, want ErrNameTaken", err)
	}
	if _, err := svc.Update(context.Background(), a.ID, everyMinute("A renamed", "demo")); err != nil {
		t.Errorf("keeping the own name must work: %v", err)
	}
	if _, err := svc.Update(context.Background(), "p-missing", everyMinute("C", "demo")); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: error = %v, want ErrNotFound", err)
	}
}

func TestServicePauseResumeDelete(t *testing.T) {
	svc := newTestService()
	p := mustCreate(t, svc, everyMinute("A", "demo"))
	ctx := context.Background()

	paused, err := svc.SetPaused(ctx, p.ID, true)
	if err != nil || !paused.Paused {
		t.Fatalf("pause = %+v, %v", paused, err)
	}
	again, err := svc.SetPaused(ctx, p.ID, true)
	if err != nil || !again.Paused {
		t.Fatalf("pausing twice must be fine: %+v, %v", again, err)
	}
	resumed, err := svc.SetPaused(ctx, p.ID, false)
	if err != nil || resumed.Paused {
		t.Fatalf("resume = %+v, %v", resumed, err)
	}

	if err := svc.Delete(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after delete: error = %v, want ErrNotFound", err)
	}
	if err := svc.Delete(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: error = %v, want ErrNotFound", err)
	}
}

func TestServiceListNewestFirstWithPaging(t *testing.T) {
	svc := newTestService()
	clock := testNow
	svc.now = func() time.Time { clock = clock.Add(time.Second); return clock }
	a := mustCreate(t, svc, everyMinute("A", "demo"))
	b := mustCreate(t, svc, everyMinute("B", "demo"))
	c := mustCreate(t, svc, everyMinute("C", "demo"))

	first, err := svc.List(context.Background(), 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 3 || len(first.Items) != 2 || first.Items[0].ID != c.ID || first.Items[1].ID != b.ID || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	second, err := svc.List(context.Background(), 2, first.NextCursor)
	if err != nil || len(second.Items) != 1 || second.Items[0].ID != a.ID || second.NextCursor != "" {
		t.Fatalf("second page = %+v, %v", second, err)
	}
}

func TestServicePreview(t *testing.T) {
	svc := newTestService()
	preview, err := svc.Preview(context.Background(), Input{
		Selector: Selector{Mode: SelectLabels, MatchLabels: map[string]string{"team": "a"}},
		Schedule: Schedule{Type: ScheduleHourly, Minute: 30},
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Cron != "30 * * * *" || len(preview.NextRuns) != PreviewRuns || len(preview.Applications) != 2 {
		t.Errorf("preview = %+v", preview)
	}
	if !preview.NextRuns[0].After(testNow) {
		t.Errorf("first run %v must be after now", preview.NextRuns[0])
	}

	if _, err := svc.Preview(context.Background(), Input{Selector: Selector{Mode: SelectAll}, Schedule: Schedule{Type: "x"}}); !errors.Is(err, ErrInvalidSchedule) {
		t.Errorf("bad schedule: error = %v", err)
	}
	if _, err := svc.Preview(context.Background(), Input{Selector: Selector{Mode: "x"}, Schedule: Schedule{Type: ScheduleHourly}}); !errors.Is(err, ErrInvalidSelector) {
		t.Errorf("bad selector: error = %v", err)
	}
}
