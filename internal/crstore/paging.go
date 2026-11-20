package crstore

import (
	"crypto/rand"
	"fmt"
	"sort"
	"time"
)

const (
	// DefaultLimit and MaxLimit bound the page size of every list endpoint.
	DefaultLimit = 50
	MaxLimit     = 200
)

// ClampLimit turns a requested page size into a usable one.
func ClampLimit(limit int) int {
	if limit <= 0 {
		return DefaultLimit
	}
	return min(limit, MaxLimit)
}

// NewName returns a short, sortable, unique name such as s-20261119-101530-k3x9.
// Names start with the creation time, so name order is time order.
func NewName(prefix string, now time.Time) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix) // crypto/rand does not fail on supported platforms
	for i, b := range suffix {
		suffix[i] = alphabet[int(b)%len(alphabet)]
	}
	return fmt.Sprintf("%s-%s-%s", prefix, now.UTC().Format("20060102-150405"), suffix)
}

// NewestFirst sorts items by name, newest first, and returns one page of them. cursor is the name of the last item
// of the previous page. next is the cursor for the following page, or empty when there is none.
func NewestFirst[T any](items []T, name func(T) string, limit int, cursor string) (page []T, next string) {
	sort.Slice(items, func(i, j int) bool { return name(items[i]) > name(items[j]) })

	start := 0
	if cursor != "" {
		start = sort.Search(len(items), func(i int) bool { return name(items[i]) < cursor })
	}
	end := min(start+ClampLimit(limit), len(items))

	page = items[start:end]
	if end < len(items) {
		next = name(items[end-1])
	}
	return page, next
}
