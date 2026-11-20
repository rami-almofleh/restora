package policy

import (
	"errors"
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/util/validation"
	"restora.local/restora/internal/discovery"
)

// Selector modes.
const (
	// SelectApplications names the applications.
	SelectApplications = "applications"
	// SelectLabels takes every application whose namespace has all the labels.
	SelectLabels = "labels"
	// SelectAll takes every application.
	SelectAll = "all"
)

// MaxApplications bounds a named selection.
const MaxApplications = 200

// ErrInvalidSelector is returned for a selection that cannot be used.
var ErrInvalidSelector = errors.New("invalid selector")

// Selector says which applications a policy protects.
type Selector struct {
	// Mode is applications, labels or all.
	Mode         string            `json:"mode"`
	Applications []string          `json:"applications,omitempty"`
	MatchLabels  map[string]string `json:"matchLabels,omitempty"`
}

// Validate checks the selector without looking at the cluster.
func (s Selector) Validate() error {
	switch s.Mode {
	case SelectAll:
		return nil
	case SelectApplications:
		if len(s.Applications) == 0 {
			return fmt.Errorf("%w: choose at least one application", ErrInvalidSelector)
		}
		if len(s.Applications) > MaxApplications {
			return fmt.Errorf("%w: at most %d applications", ErrInvalidSelector, MaxApplications)
		}
		for _, name := range s.Applications {
			if errs := validation.IsDNS1123Label(name); len(errs) > 0 {
				return fmt.Errorf("%w: %q is not a namespace name", ErrInvalidSelector, name)
			}
		}
		return nil
	case SelectLabels:
		if len(s.MatchLabels) == 0 {
			return fmt.Errorf("%w: give at least one label", ErrInvalidSelector)
		}
		for k, v := range s.MatchLabels {
			if errs := validation.IsQualifiedName(k); len(errs) > 0 {
				return fmt.Errorf("%w: %q is not a valid label name", ErrInvalidSelector, k)
			}
			if errs := validation.IsValidLabelValue(v); len(errs) > 0 {
				return fmt.Errorf("%w: %q is not a valid label value", ErrInvalidSelector, v)
			}
		}
		return nil
	default:
		return fmt.Errorf("%w: mode must be applications, labels or all", ErrInvalidSelector)
	}
}

// Select returns the applications of the cluster that the selector takes, sorted by name. missing lists named
// applications that do not exist (only for the applications mode).
func (s Selector) Select(apps []discovery.Ref) (found, missing []string) {
	found, missing = []string{}, []string{}
	switch s.Mode {
	case SelectAll:
		for _, a := range apps {
			found = append(found, a.Name)
		}
	case SelectApplications:
		existing := make(map[string]bool, len(apps))
		for _, a := range apps {
			existing[a.Name] = true
		}
		seen := map[string]bool{}
		for _, name := range s.Applications {
			if seen[name] {
				continue
			}
			seen[name] = true
			if existing[name] {
				found = append(found, name)
			} else {
				missing = append(missing, name)
			}
		}
	case SelectLabels:
		for _, a := range apps {
			if hasAll(a.Labels, s.MatchLabels) {
				found = append(found, a.Name)
			}
		}
	}
	sort.Strings(found)
	sort.Strings(missing)
	return found, missing
}

func hasAll(have, want map[string]string) bool {
	for k, v := range want {
		if got, ok := have[k]; !ok || got != v {
			return false
		}
	}
	return len(want) > 0
}
