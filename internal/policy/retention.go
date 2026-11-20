package policy

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"restora.local/restora/internal/snapshot"
)

// MaxKeep bounds every number of a retention rule.
const MaxKeep = 1000

// FailedKeep is how long a failed backup of a policy stays in the list before retention removes it. Failed backups
// are no restore points, but people want to see them for a while to find out what went wrong.
const FailedKeep = 7 * 24 * time.Hour

// ErrInvalidRetention is returned for a retention rule that cannot be used.
var ErrInvalidRetention = errors.New("invalid retention")

// Reasons why a backup is kept or deleted. They are stable codes the interface translates.
const (
	KeepLast    = "last"
	KeepHourly  = "hourly"
	KeepDaily   = "daily"
	KeepWeekly  = "weekly"
	KeepMonthly = "monthly"
	KeepYearly  = "yearly"

	DeleteNotKept      = "not_kept"
	DeleteFailedExpire = "failed_expired"
)

// Retention says which backups of a policy are kept. All numbers zero means "keep everything".
//
// Each number keeps the newest backup of each of the most recent hours, days, weeks, months or years that have a
// backup (the grandfather-father-son scheme), and Last keeps the newest backups no matter when they were made.
// A backup is kept when any rule keeps it. The rules apply to each application of the policy on its own.
type Retention struct {
	Last    int `json:"last"`
	Hourly  int `json:"hourly"`
	Daily   int `json:"daily"`
	Weekly  int `json:"weekly"`
	Monthly int `json:"monthly"`
	Yearly  int `json:"yearly"`
}

// Enabled reports whether any rule is set. Without one nothing is ever deleted.
func (r Retention) Enabled() bool {
	return r.Last+r.Hourly+r.Daily+r.Weekly+r.Monthly+r.Yearly > 0
}

// Validate checks the numbers.
func (r Retention) Validate() error {
	for name, n := range map[string]int{
		"last": r.Last, "hourly": r.Hourly, "daily": r.Daily, "weekly": r.Weekly, "monthly": r.Monthly, "yearly": r.Yearly,
	} {
		if n < 0 || n > MaxKeep {
			return fmt.Errorf("%w: %s must be between 0 and %d", ErrInvalidRetention, name, MaxKeep)
		}
	}
	return nil
}

// Kept is a backup the plan keeps, with the rules that keep it.
type Kept struct {
	ID          string    `json:"id"`
	Application string    `json:"application"`
	CreatedAt   time.Time `json:"createdAt"`
	Reasons     []string  `json:"reasons"`
}

// Doomed is a backup the plan deletes.
type Doomed struct {
	ID          string    `json:"id"`
	Application string    `json:"application"`
	CreatedAt   time.Time `json:"createdAt"`
	Reason      string    `json:"reason"`
}

// Plan is what retention keeps and what it deletes.
type Plan struct {
	Keep   []Kept   `json:"keep"`
	Delete []Doomed `json:"delete"`
	// Busy lists applications with a backup in progress. Nothing of theirs is planned, because the new backup
	// changes what has to be kept.
	Busy []string `json:"busy"`
}

type rule struct {
	reason string
	limit  int
	// bucket maps a time to the period it falls into.
	bucket func(time.Time) string
}

func (r Retention) rules(loc *time.Location) []rule {
	local := func(t time.Time) time.Time { return t.In(loc) }
	return []rule{
		{KeepHourly, r.Hourly, func(t time.Time) string { return t.UTC().Truncate(time.Hour).Format(time.RFC3339) }},
		{KeepDaily, r.Daily, func(t time.Time) string { return local(t).Format("2006-01-02") }},
		{KeepWeekly, r.Weekly, func(t time.Time) string {
			year, week := local(t).ISOWeek()
			return fmt.Sprintf("%d-W%02d", year, week)
		}},
		{KeepMonthly, r.Monthly, func(t time.Time) string { return local(t).Format("2006-01") }},
		{KeepYearly, r.Yearly, func(t time.Time) string { return local(t).Format("2006") }},
	}
}

// Plan decides which of the given backups to keep. Backups that are being deleted are ignored. Days, weeks, months
// and years are counted in loc, hours in UTC. A retention without rules plans nothing.
func (r Retention) Plan(snaps []snapshot.Snapshot, now time.Time, loc *time.Location) Plan {
	plan := Plan{Keep: []Kept{}, Delete: []Doomed{}, Busy: []string{}}
	if !r.Enabled() {
		return plan
	}

	byApp := map[string][]snapshot.Snapshot{}
	for _, s := range snaps {
		if !s.Deleting() {
			byApp[s.Spec.Application] = append(byApp[s.Spec.Application], s)
		}
	}
	apps := make([]string, 0, len(byApp))
	for app := range byApp {
		apps = append(apps, app)
	}
	sort.Strings(apps)

	for _, app := range apps {
		kept, doomed, busy := r.planApplication(app, byApp[app], now, loc)
		if busy {
			plan.Busy = append(plan.Busy, app)
			continue
		}
		plan.Keep = append(plan.Keep, kept...)
		plan.Delete = append(plan.Delete, doomed...)
	}
	sortNewestFirst(plan.Keep, func(k Kept) (time.Time, string) { return k.CreatedAt, k.ID })
	sortNewestFirst(plan.Delete, func(d Doomed) (time.Time, string) { return d.CreatedAt, d.ID })
	return plan
}

func (r Retention) planApplication(app string, snaps []snapshot.Snapshot, now time.Time, loc *time.Location) (kept []Kept, doomed []Doomed, busy bool) {
	var good []snapshot.Snapshot
	for _, s := range snaps {
		switch snapshot.NewView(&s).Phase {
		case snapshot.PhasePending, snapshot.PhaseRunning:
			return nil, nil, true
		case snapshot.PhaseSucceeded:
			good = append(good, s)
		case snapshot.PhaseFailed:
			if now.Sub(s.CreationTimestamp.Time) > FailedKeep {
				doomed = append(doomed, Doomed{ID: s.Name, Application: app, CreatedAt: s.CreationTimestamp.UTC(), Reason: DeleteFailedExpire})
			}
		}
	}
	sortNewestFirst(good, func(s snapshot.Snapshot) (time.Time, string) { return s.CreationTimestamp.Time, s.Name })

	reasons := make(map[string][]string, len(good))
	for i := 0; i < r.Last && i < len(good); i++ {
		reasons[good[i].Name] = append(reasons[good[i].Name], KeepLast)
	}
	for _, rl := range r.rules(loc) {
		seen := map[string]bool{}
		for _, s := range good {
			key := rl.bucket(s.CreationTimestamp.Time)
			if seen[key] {
				continue
			}
			if len(seen) >= rl.limit {
				break
			}
			seen[key] = true
			reasons[s.Name] = append(reasons[s.Name], rl.reason)
		}
	}

	for _, s := range good {
		if rs := reasons[s.Name]; len(rs) > 0 {
			kept = append(kept, Kept{ID: s.Name, Application: app, CreatedAt: s.CreationTimestamp.UTC(), Reasons: rs})
		} else {
			doomed = append(doomed, Doomed{ID: s.Name, Application: app, CreatedAt: s.CreationTimestamp.UTC(), Reason: DeleteNotKept})
		}
	}
	return kept, doomed, false
}

// sortNewestFirst orders by time, newest first, and by id to be stable for equal times.
func sortNewestFirst[T any](items []T, key func(T) (time.Time, string)) {
	sort.Slice(items, func(i, j int) bool {
		ti, ni := key(items[i])
		tj, nj := key(items[j])
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return ni > nj
	})
}
