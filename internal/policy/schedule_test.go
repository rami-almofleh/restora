package policy

import (
	"errors"
	"testing"
	"time"
)

func TestScheduleExpression(t *testing.T) {
	tests := []struct {
		name    string
		in      Schedule
		want    string
		wantErr bool
	}{
		{"hourly", Schedule{Type: ScheduleHourly, Minute: 15}, "15 * * * *", false},
		{"daily", Schedule{Type: ScheduleDaily, Hour: 2, Minute: 30}, "30 2 * * *", false},
		{"weekly", Schedule{Type: ScheduleWeekly, Weekday: 0, Hour: 3}, "0 3 * * 0", false},
		{"custom is trimmed", Schedule{Type: ScheduleCustom, Cron: "  */5 * * * * "}, "*/5 * * * *", false},
		{"minute too high", Schedule{Type: ScheduleHourly, Minute: 60}, "", true},
		{"hour too high", Schedule{Type: ScheduleDaily, Hour: 24}, "", true},
		{"weekday too high", Schedule{Type: ScheduleWeekly, Weekday: 7}, "", true},
		{"custom with six fields", Schedule{Type: ScheduleCustom, Cron: "0 * * * * *"}, "", true},
		{"custom empty", Schedule{Type: ScheduleCustom}, "", true},
		{"unknown type", Schedule{Type: "monthly"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.Expression()
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("Expression() = %q, %v; want %q, error %v", got, err, tt.want, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidSchedule) {
				t.Errorf("error %v must wrap ErrInvalidSchedule", err)
			}
		})
	}
}

func TestCompileRejects(t *testing.T) {
	tests := []struct {
		name string
		in   Schedule
	}{
		{"bad field", Schedule{Type: ScheduleCustom, Cron: "61 * * * *"}},
		{"descriptor", Schedule{Type: ScheduleCustom, Cron: "@every 1h"}},
		{"unknown zone", Schedule{Type: ScheduleDaily, Timezone: "Mars/Base"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.in.Compile(); !errors.Is(err, ErrInvalidSchedule) {
				t.Errorf("Compile() error = %v, want ErrInvalidSchedule", err)
			}
		})
	}
}

func TestNextRuns(t *testing.T) {
	tests := []struct {
		name string
		in   Schedule
		want []time.Time
	}{
		{"every minute", Schedule{Type: ScheduleCustom, Cron: "* * * * *"}, []time.Time{
			time.Date(2026, 11, 19, 10, 16, 0, 0, time.UTC), time.Date(2026, 11, 19, 10, 17, 0, 0, time.UTC),
		}},
		{"hourly at :05", Schedule{Type: ScheduleHourly, Minute: 5}, []time.Time{
			time.Date(2026, 11, 19, 11, 5, 0, 0, time.UTC), time.Date(2026, 11, 19, 12, 5, 0, 0, time.UTC),
		}},
		{"daily 02:00 in Berlin is 01:00 UTC in winter", Schedule{Type: ScheduleDaily, Hour: 2, Timezone: "Europe/Berlin"}, []time.Time{
			time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC), time.Date(2026, 11, 21, 1, 0, 0, 0, time.UTC),
		}},
		{"weekly on Sunday", Schedule{Type: ScheduleWeekly, Weekday: 0, Hour: 3}, []time.Time{
			time.Date(2026, 11, 22, 3, 0, 0, 0, time.UTC), time.Date(2026, 11, 29, 3, 0, 0, 0, time.UTC),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiled, err := tt.in.Compile()
			if err != nil {
				t.Fatal(err)
			}
			got := compiled.NextRuns(testNow, len(tt.want))
			for i := range tt.want {
				if i >= len(got) || !got[i].Equal(tt.want[i]) {
					t.Fatalf("NextRuns = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
