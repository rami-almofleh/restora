package crstore

import (
	"reflect"
	"regexp"
	"testing"
	"time"
)

func TestClampLimit(t *testing.T) {
	tests := []struct{ in, want int }{{0, DefaultLimit}, {-3, DefaultLimit}, {7, 7}, {MaxLimit, MaxLimit}, {MaxLimit + 1, MaxLimit}}
	for _, tt := range tests {
		if got := ClampLimit(tt.in); got != tt.want {
			t.Errorf("ClampLimit(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestNewName(t *testing.T) {
	now := time.Date(2026, 11, 19, 10, 15, 30, 0, time.FixedZone("x", 3600))
	got := NewName("s", now)
	if !regexp.MustCompile(`^s-20261119-091530-[a-z0-9]{4}$`).MatchString(got) {
		t.Errorf("NewName = %q, want s-<UTC time>-<4 chars>", got)
	}
	if NewName("s", now) == NewName("s", now) {
		t.Error("two names in the same second must differ")
	}
}

func TestNewestFirst(t *testing.T) {
	id := func(s string) string { return s }
	items := []string{"a", "c", "b", "e", "d"}

	var got []string
	cursor := ""
	for range 10 {
		page, next := NewestFirst(append([]string{}, items...), id, 2, cursor)
		got = append(got, page...)
		if next == "" {
			break
		}
		cursor = next
	}
	if want := []string{"e", "d", "c", "b", "a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}

	if page, next := NewestFirst([]string{}, id, 5, ""); len(page) != 0 || next != "" {
		t.Errorf("empty input: page %v, next %q", page, next)
	}
	if page, _ := NewestFirst([]string{"a", "b"}, id, 5, "a"); len(page) != 0 {
		t.Errorf("cursor at the end: page %v, want empty", page)
	}
}
