package policy

import (
	"fmt"
	"slices"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"restora.local/restora/internal/snapshot"
)

func at(day, hour, minute int) time.Time {
	return time.Date(2026, 11, day, hour, minute, 0, 0, time.UTC)
}

func snap(app string, created time.Time, phase snapshot.Phase) snapshot.Snapshot {
	return snapshot.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:              fmt.Sprintf("s-%s-%s", app, created.Format("20060102-150405")),
			CreationTimestamp: metav1.NewTime(created),
		},
		Spec:   snapshot.Spec{Application: app},
		Status: snapshot.Status{Phase: phase},
	}
}

func ok(app string, created time.Time) snapshot.Snapshot {
	return snap(app, created, snapshot.PhaseSucceeded)
}

func ids(plan Plan) (keep, del []string) {
	for _, k := range plan.Keep {
		keep = append(keep, k.ID)
	}
	for _, d := range plan.Delete {
		del = append(del, d.ID)
	}
	return keep, del
}

func name(app string, t time.Time) string { return snap(app, t, "").Name }

func TestRetentionValidate(t *testing.T) {
	tests := []struct {
		name    string
		in      Retention
		wantErr bool
	}{
		{"nothing set", Retention{}, false},
		{"some", Retention{Last: 3, Daily: 7}, false},
		{"maximum", Retention{Yearly: MaxKeep}, false},
		{"negative", Retention{Daily: -1}, true},
		{"too many", Retention{Hourly: MaxKeep + 1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() = %v, want error %v", err, tt.wantErr)
			}
		})
	}
}

func TestRetentionWithoutRulesKeepsEverything(t *testing.T) {
	plan := Retention{}.Plan([]snapshot.Snapshot{ok("demo", at(19, 10, 0)), snap("demo", at(1, 10, 0), snapshot.PhaseFailed)}, at(19, 12, 0), time.UTC)
	if len(plan.Keep) != 0 || len(plan.Delete) != 0 || len(plan.Busy) != 0 {
		t.Errorf("plan = %+v, want nothing", plan)
	}
	if plan.Keep == nil || plan.Delete == nil || plan.Busy == nil {
		t.Error("lists must be empty, not nil, so they encode as []")
	}
}

func TestRetentionLastKeepsTheNewest(t *testing.T) {
	var snaps []snapshot.Snapshot
	for minute := 0; minute < 4; minute++ {
		snaps = append(snaps, ok("demo", at(19, 10, minute)))
	}
	plan := Retention{Last: 2}.Plan(snaps, at(19, 11, 0), time.UTC)

	keep, del := ids(plan)
	if !slices.Equal(keep, []string{name("demo", at(19, 10, 3)), name("demo", at(19, 10, 2))}) {
		t.Errorf("keep = %v", keep)
	}
	if !slices.Equal(del, []string{name("demo", at(19, 10, 1)), name("demo", at(19, 10, 0))}) {
		t.Errorf("delete = %v", del)
	}
	for _, d := range plan.Delete {
		if d.Reason != DeleteNotKept {
			t.Errorf("reason = %q", d.Reason)
		}
	}
}

func TestRetentionBucketRules(t *testing.T) {
	// Two backups per hour for three hours, on two days.
	snaps := []snapshot.Snapshot{
		ok("demo", at(18, 22, 0)), ok("demo", at(18, 22, 30)),
		ok("demo", at(19, 8, 0)), ok("demo", at(19, 8, 30)),
		ok("demo", at(19, 9, 0)), ok("demo", at(19, 9, 30)),
	}
	now := at(19, 10, 0)

	tests := []struct {
		name string
		in   Retention
		want []time.Time
	}{
		{"hourly 2 keeps the newest of the last two hours", Retention{Hourly: 2}, []time.Time{at(19, 9, 30), at(19, 8, 30)}},
		{"daily 1 keeps the newest of the last day", Retention{Daily: 1}, []time.Time{at(19, 9, 30)}},
		{"daily 2 adds the newest of the day before", Retention{Daily: 2}, []time.Time{at(19, 9, 30), at(18, 22, 30)}},
		{"weekly 1", Retention{Weekly: 1}, []time.Time{at(19, 9, 30)}},
		{"monthly and yearly 1", Retention{Monthly: 1, Yearly: 1}, []time.Time{at(19, 9, 30)}},
		{"rules add up", Retention{Last: 1, Daily: 2}, []time.Time{at(19, 9, 30), at(18, 22, 30)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keep, _ := ids(tt.in.Plan(snaps, now, time.UTC))
			var want []string
			for _, w := range tt.want {
				want = append(want, name("demo", w))
			}
			if !slices.Equal(keep, want) {
				t.Errorf("keep = %v, want %v", keep, want)
			}
		})
	}
}

func TestRetentionCountsBucketsWithBackupsNotCalendarPeriods(t *testing.T) {
	// Nothing was backed up between the 10th and the 19th. "3 days" still reaches back to the 10th.
	snaps := []snapshot.Snapshot{ok("demo", at(19, 9, 0)), ok("demo", at(10, 9, 0)), ok("demo", at(9, 9, 0)), ok("demo", at(8, 9, 0))}
	keep, del := ids(Retention{Daily: 3}.Plan(snaps, at(19, 10, 0), time.UTC))

	if len(keep) != 3 || len(del) != 1 || del[0] != name("demo", at(8, 9, 0)) {
		t.Errorf("keep = %v, delete = %v", keep, del)
	}
}

func TestRetentionWeeksStartOnMonday(t *testing.T) {
	// Sunday the 15th and Monday the 16th are in different ISO weeks.
	snaps := []snapshot.Snapshot{ok("demo", at(16, 9, 0)), ok("demo", at(15, 9, 0))}
	keep, _ := ids(Retention{Weekly: 2}.Plan(snaps, at(19, 10, 0), time.UTC))
	if len(keep) != 2 {
		t.Errorf("keep = %v, want both", keep)
	}
}

func TestRetentionCountsDaysInTheGivenZone(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	// 23:30 UTC on the 18th is 00:30 on the 19th in Berlin, the same day as 08:00 UTC on the 19th.
	snaps := []snapshot.Snapshot{ok("demo", at(19, 8, 0)), ok("demo", at(18, 23, 30))}

	_, delUTC := ids(Retention{Daily: 2}.Plan(snaps, at(19, 10, 0), time.UTC))
	_, delBerlin := ids(Retention{Daily: 2}.Plan(snaps, at(19, 10, 0), berlin))
	if len(delUTC) != 0 {
		t.Errorf("UTC: delete = %v, want none (two different days)", delUTC)
	}
	if !slices.Equal(delBerlin, []string{name("demo", at(18, 23, 30))}) {
		t.Errorf("Berlin: delete = %v, want the older one (same day)", delBerlin)
	}
}

func TestRetentionKeepsApplicationsApart(t *testing.T) {
	snaps := []snapshot.Snapshot{
		ok("demo", at(19, 9, 0)), ok("demo", at(19, 8, 0)),
		ok("shop", at(19, 9, 0)), ok("shop", at(19, 8, 0)),
	}
	plan := Retention{Last: 1}.Plan(snaps, at(19, 10, 0), time.UTC)

	keep, del := ids(plan)
	if len(keep) != 2 || len(del) != 2 {
		t.Fatalf("keep = %v, delete = %v; each application keeps its own newest", keep, del)
	}
}

func TestRetentionFailedBackupsExpire(t *testing.T) {
	now := at(19, 10, 0)
	old := snap("demo", now.Add(-FailedKeep-time.Hour), snapshot.PhaseFailed)
	recent := snap("demo", now.Add(-time.Hour), snapshot.PhaseFailed)

	plan := Retention{Last: 5}.Plan([]snapshot.Snapshot{old, recent, ok("demo", now.Add(-time.Minute))}, now, time.UTC)

	_, del := ids(plan)
	if !slices.Equal(del, []string{old.Name}) || plan.Delete[0].Reason != DeleteFailedExpire {
		t.Errorf("delete = %+v, want only the old failed backup", plan.Delete)
	}
	keep, _ := ids(plan)
	if slices.Contains(keep, recent.Name) {
		t.Error("a failed backup is no restore point and must not be listed as kept")
	}
}

func TestRetentionLeavesAnApplicationAloneWhileABackupRuns(t *testing.T) {
	snaps := []snapshot.Snapshot{
		ok("demo", at(19, 9, 0)), ok("demo", at(19, 8, 0)), snap("demo", at(19, 9, 59), snapshot.PhaseRunning),
		ok("shop", at(19, 9, 0)), ok("shop", at(19, 8, 0)),
	}
	plan := Retention{Last: 1}.Plan(snaps, at(19, 10, 0), time.UTC)

	if !slices.Equal(plan.Busy, []string{"demo"}) {
		t.Errorf("busy = %v", plan.Busy)
	}
	for _, d := range plan.Delete {
		if d.Application == "demo" {
			t.Errorf("demo must not lose anything while its backup runs: %+v", d)
		}
	}
	if len(plan.Delete) != 1 || plan.Delete[0].Application != "shop" {
		t.Errorf("delete = %+v, want the older shop backup", plan.Delete)
	}
}

func TestRetentionIgnoresBackupsThatAreBeingDeleted(t *testing.T) {
	gone := ok("demo", at(19, 9, 0))
	now := metav1.NewTime(at(19, 9, 5))
	gone.DeletionTimestamp = &now

	plan := Retention{Last: 1}.Plan([]snapshot.Snapshot{gone, ok("demo", at(19, 8, 0))}, at(19, 10, 0), time.UTC)
	keep, del := ids(plan)
	if len(keep) != 1 || keep[0] != name("demo", at(19, 8, 0)) || len(del) != 0 {
		t.Errorf("keep = %v, delete = %v", keep, del)
	}
}

func TestRetentionReasonsListEveryRuleThatKeeps(t *testing.T) {
	plan := Retention{Last: 1, Daily: 1, Monthly: 1}.Plan([]snapshot.Snapshot{ok("demo", at(19, 9, 0))}, at(19, 10, 0), time.UTC)
	if len(plan.Keep) != 1 || !slices.Equal(plan.Keep[0].Reasons, []string{KeepLast, KeepDaily, KeepMonthly}) {
		t.Errorf("keep = %+v", plan.Keep)
	}
}
