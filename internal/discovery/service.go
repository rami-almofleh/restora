package discovery

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrNotFound is returned when an application does not exist.
var ErrNotFound = errors.New("application not found")

const (
	// DefaultLimit and MaxLimit bound the page size of the application list.
	DefaultLimit = 50
	MaxLimit     = 200
)

// systemNamespaces are never offered as applications.
var systemNamespaces = map[string]bool{
	"kube-system":        true,
	"kube-public":        true,
	"kube-node-lease":    true,
	"restora-system":     true,
	"local-path-storage": true,
}

// Service finds applications in the cluster.
type Service struct {
	src Source
}

// New creates a Service reading from src.
func New(src Source) *Service {
	return &Service{src: src}
}

// List returns one page of applications, sorted by name.
// limit <= 0 uses DefaultLimit; cursor is the name of the last item of the previous page.
func (s *Service) List(ctx context.Context, f Filter, limit int, cursor string) (Page, error) {
	apps, err := s.discover(ctx)
	if err != nil {
		return Page{}, err
	}

	var matched []Summary
	for _, name := range sortedNames(apps) {
		if sum := apps[name].summary(); f.matches(sum) {
			matched = append(matched, sum)
		}
	}

	limit = clampLimit(limit)
	start := sort.Search(len(matched), func(i int) bool { return matched[i].Name > cursor })
	end := min(start+limit, len(matched))

	page := Page{Items: matched[start:end], Total: len(matched)}
	if page.Items == nil {
		page.Items = []Summary{}
	}
	if end < len(matched) {
		page.NextCursor = matched[end-1].Name
	}
	return page, nil
}

// Ref names an application together with the labels of its namespace. Policies select applications by them.
type Ref struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
}

// Refs returns every application with its namespace labels, sorted by name.
func (s *Service) Refs(ctx context.Context) ([]Ref, error) {
	apps, err := s.discover(ctx)
	if err != nil {
		return nil, err
	}
	refs := make([]Ref, 0, len(apps))
	for _, name := range sortedNames(apps) {
		refs = append(refs, Ref{Name: name, Labels: apps[name].labels})
	}
	return refs, nil
}

// Get returns the details of one application.
func (s *Service) Get(ctx context.Context, name string) (Detail, error) {
	apps, err := s.discover(ctx)
	if err != nil {
		return Detail{}, err
	}
	app, ok := apps[name]
	if !ok {
		return Detail{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return app.detail(), nil
}

func (s *Service) discover(ctx context.Context) (map[string]*app, error) {
	namespaces, err := s.src.Namespaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}
	workloads, err := s.src.Workloads(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workloads: %w", err)
	}
	volumes, err := s.src.Volumes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list volumes: %w", err)
	}
	configMaps, err := s.src.ConfigMaps(ctx)
	if err != nil {
		return nil, fmt.Errorf("list config maps: %w", err)
	}
	secrets, err := s.src.Secrets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}

	apps := make(map[string]*app)
	for _, ns := range namespaces {
		if !systemNamespaces[ns.Name] {
			apps[ns.Name] = &app{name: ns.Name, labels: ns.Labels, createdAt: ns.CreatedAt}
		}
	}
	for _, w := range workloads {
		if a, ok := apps[w.Namespace]; ok {
			a.workloads = append(a.workloads, w.Workload)
		}
	}
	for _, v := range volumes {
		if a, ok := apps[v.Namespace]; ok {
			a.volumes = append(a.volumes, v.Volume)
		}
	}
	for _, c := range configMaps {
		if a, ok := apps[c.Namespace]; ok {
			a.configMaps = append(a.configMaps, c.Name)
		}
	}
	for _, c := range secrets {
		if a, ok := apps[c.Namespace]; ok {
			a.secrets = append(a.secrets, c.Name)
		}
	}

	// A namespace without workloads and volumes holds nothing to protect.
	for name, a := range apps {
		if len(a.workloads) == 0 && len(a.volumes) == 0 {
			delete(apps, name)
		}
	}
	return apps, nil
}

func (f Filter) matches(s Summary) bool {
	switch f.Volumes {
	case "with":
		if s.Volumes == 0 {
			return false
		}
	case "without":
		if s.Volumes > 0 {
			return false
		}
	}
	q := strings.ToLower(strings.TrimSpace(f.Query))
	if q == "" {
		return true
	}
	if strings.Contains(strings.ToLower(s.Name), q) {
		return true
	}
	for _, e := range s.Engines {
		if strings.Contains(strings.ToLower(e), q) {
			return true
		}
	}
	return false
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return DefaultLimit
	}
	return min(limit, MaxLimit)
}

func sortedNames(apps map[string]*app) []string {
	names := make([]string, 0, len(apps))
	for name := range apps {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
