package info

import "time"

// Info describes the running Restora instance.
type Info struct {
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	StartedAt time.Time `json:"startedAt"`
}
