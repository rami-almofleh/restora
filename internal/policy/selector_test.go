package policy

import (
	"errors"
	"slices"
	"testing"
)

func TestSelectorValidate(t *testing.T) {
	tests := []struct {
		name    string
		in      Selector
		wantErr bool
	}{
		{"all", Selector{Mode: SelectAll}, false},
		{"applications", Selector{Mode: SelectApplications, Applications: []string{"demo", "shop"}}, false},
		{"labels", Selector{Mode: SelectLabels, MatchLabels: map[string]string{"team": "a"}}, false},
		{"label with prefix", Selector{Mode: SelectLabels, MatchLabels: map[string]string{"example.com/team": "a"}}, false},
		{"no applications", Selector{Mode: SelectApplications}, true},
		{"bad namespace name", Selector{Mode: SelectApplications, Applications: []string{"Not Valid"}}, true},
		{"no labels", Selector{Mode: SelectLabels}, true},
		{"bad label name", Selector{Mode: SelectLabels, MatchLabels: map[string]string{"bad key!": "a"}}, true},
		{"bad label value", Selector{Mode: SelectLabels, MatchLabels: map[string]string{"team": "no spaces allowed"}}, true},
		{"unknown mode", Selector{Mode: "some"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.in.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, want error %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidSelector) {
				t.Errorf("error %v must wrap ErrInvalidSelector", err)
			}
		})
	}
}

func TestSelectorSelect(t *testing.T) {
	apps := testApps().refs
	tests := []struct {
		name        string
		in          Selector
		wantFound   []string
		wantMissing []string
	}{
		{"all", Selector{Mode: SelectAll}, []string{"demo", "shop", "wiki"}, []string{}},
		{"named, one missing, duplicate ignored", Selector{Mode: SelectApplications, Applications: []string{"wiki", "gone", "demo", "demo"}},
			[]string{"demo", "wiki"}, []string{"gone"}},
		{"one label", Selector{Mode: SelectLabels, MatchLabels: map[string]string{"team": "a"}}, []string{"demo", "wiki"}, []string{}},
		{"all labels must match", Selector{Mode: SelectLabels, MatchLabels: map[string]string{"team": "a", "tier": "prod"}}, []string{"demo"}, []string{}},
		{"no label match", Selector{Mode: SelectLabels, MatchLabels: map[string]string{"team": "z"}}, []string{}, []string{}},
		{"empty label selector takes nothing", Selector{Mode: SelectLabels}, []string{}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, missing := tt.in.Select(apps)
			if !slices.Equal(found, tt.wantFound) || !slices.Equal(missing, tt.wantMissing) {
				t.Errorf("Select = %v, %v; want %v, %v", found, missing, tt.wantFound, tt.wantMissing)
			}
		})
	}
}
