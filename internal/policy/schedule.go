package policy

import (
	"errors"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata" // so that time zones work in the minimal container image

	"github.com/robfig/cron/v3"
)

// Schedule types.
const (
	ScheduleHourly = "hourly"
	ScheduleDaily  = "daily"
	ScheduleWeekly = "weekly"
	ScheduleCustom = "custom"
)

// DefaultTimezone is used when a schedule names none.
const DefaultTimezone = "UTC"

// ErrInvalidSchedule is returned for a schedule that cannot be run.
var ErrInvalidSchedule = errors.New("invalid schedule")

// Schedule says when a policy runs. The presets (hourly, daily, weekly) are turned into a cron expression;
// custom takes the expression as is.
type Schedule struct {
	// Type is hourly, daily, weekly or custom.
	Type string `json:"type"`
	// Minute is used by hourly, daily and weekly (0-59), Hour by daily and weekly (0-23),
	// Weekday by weekly (0 = Sunday ... 6 = Saturday).
	Minute  int `json:"minute"`
	Hour    int `json:"hour"`
	Weekday int `json:"weekday"`
	// Cron is the five-field expression of a custom schedule.
	Cron string `json:"cron,omitempty"`
	// Timezone is an IANA name such as Europe/Berlin. Empty means UTC.
	Timezone string `json:"timezone,omitempty"`
}

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// Expression returns the cron expression of the schedule.
func (s Schedule) Expression() (string, error) {
	switch s.Type {
	case ScheduleHourly:
		if err := inRange("minute", s.Minute, 0, 59); err != nil {
			return "", err
		}
		return fmt.Sprintf("%d * * * *", s.Minute), nil
	case ScheduleDaily:
		if err := errors.Join(inRange("minute", s.Minute, 0, 59), inRange("hour", s.Hour, 0, 23)); err != nil {
			return "", err
		}
		return fmt.Sprintf("%d %d * * *", s.Minute, s.Hour), nil
	case ScheduleWeekly:
		if err := errors.Join(inRange("minute", s.Minute, 0, 59), inRange("hour", s.Hour, 0, 23), inRange("weekday", s.Weekday, 0, 6)); err != nil {
			return "", err
		}
		return fmt.Sprintf("%d %d * * %d", s.Minute, s.Hour, s.Weekday), nil
	case ScheduleCustom:
		expr := strings.TrimSpace(s.Cron)
		if len(strings.Fields(expr)) != 5 {
			return "", fmt.Errorf("%w: the cron expression needs five fields (minute hour day month weekday)", ErrInvalidSchedule)
		}
		return expr, nil
	default:
		return "", fmt.Errorf("%w: type must be hourly, daily, weekly or custom", ErrInvalidSchedule)
	}
}

func inRange(name string, v, lo, hi int) error {
	if v < lo || v > hi {
		return fmt.Errorf("%w: %s must be between %d and %d", ErrInvalidSchedule, name, lo, hi)
	}
	return nil
}

// Compiled is a schedule that is ready to compute run times.
type Compiled struct {
	// Expression is the cron expression that is run.
	Expression string
	schedule   cron.Schedule
	location   *time.Location
}

// Compile validates the schedule.
func (s Schedule) Compile() (Compiled, error) {
	expr, err := s.Expression()
	if err != nil {
		return Compiled{}, err
	}
	parsed, err := cronParser.Parse(expr)
	if err != nil {
		return Compiled{}, fmt.Errorf("%w: %s", ErrInvalidSchedule, err)
	}
	name := s.Timezone
	if name == "" {
		name = DefaultTimezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return Compiled{}, fmt.Errorf("%w: unknown time zone %q", ErrInvalidSchedule, name)
	}
	return Compiled{Expression: expr, schedule: parsed, location: loc}, nil
}

// Location is the time zone of the schedule.
func (c Compiled) Location() *time.Location {
	return c.location
}

// Next returns the first run after t.
func (c Compiled) Next(t time.Time) time.Time {
	return c.schedule.Next(t.In(c.location)).UTC()
}

// NextRuns returns the following n runs after t.
func (c Compiled) NextRuns(t time.Time, n int) []time.Time {
	runs := make([]time.Time, 0, n)
	for range n {
		t = c.Next(t)
		if t.IsZero() {
			break
		}
		runs = append(runs, t)
	}
	return runs
}
