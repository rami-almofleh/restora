package info

import "time"

const productName = "Restora"

// Service reports basic facts about the running instance.
type Service struct {
	version   string
	startedAt time.Time
}

// New creates a Service. startedAt is stored in UTC.
func New(version string, startedAt time.Time) *Service {
	return &Service{version: version, startedAt: startedAt.UTC()}
}

// Get returns the current instance information.
func (s *Service) Get() Info {
	return Info{Name: productName, Version: s.version, StartedAt: s.startedAt}
}
