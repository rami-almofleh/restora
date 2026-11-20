package activity

import (
	"context"
	"strings"
	"time"

	"restora.local/restora/internal/crstore"
	"restora.local/restora/internal/restore"
	"restora.local/restora/internal/snapshot"
)

// Backups reads all snapshots. snapshot.Store implements it.
type Backups interface {
	All(ctx context.Context) ([]snapshot.Snapshot, error)
}

// Restores reads all restore requests. restore.Store implements it.
type Restores interface {
	All(ctx context.Context) ([]restore.RestoreRequest, error)
}

// Service builds the timeline from the backups and restores in the cluster.
type Service struct {
	backups  Backups
	restores Restores
}

// NewService creates a Service.
func NewService(backups Backups, restores Restores) *Service {
	return &Service{backups: backups, restores: restores}
}

// List returns the entries that match the filter, newest first. limit <= 0 uses the default page size;
// cursor is the nextCursor of the previous page.
func (s *Service) List(ctx context.Context, filter Filter, limit int, cursor string) (Page, error) {
	items, err := s.collect(ctx)
	if err != nil {
		return Page{}, err
	}

	query := strings.ToLower(filter.Query)
	var counts Counts
	matching := make([]Item, 0, len(items))
	for _, item := range items {
		if (filter.Type != "" && item.Type != filter.Type) ||
			(query != "" && !strings.Contains(strings.ToLower(item.Application), query)) {
			continue
		}
		counts.add(item)
		if filter.Status == "" || statusOf(item) == filter.Status {
			matching = append(matching, item)
		}
	}

	total := len(matching)
	page, next := crstore.NewestFirst(matching, sortKey, limit, cursor)
	if page == nil {
		page = []Item{}
	}
	return Page{Items: page, Total: total, Counts: counts, NextCursor: next}, nil
}

func (s *Service) collect(ctx context.Context) ([]Item, error) {
	backups, err := s.backups.All(ctx)
	if err != nil {
		return nil, err
	}
	restores, err := s.restores.All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(backups)+len(restores))
	createdAt := make(map[string]time.Time, len(backups))
	for i := range backups {
		if backups[i].Deleting() {
			continue
		}
		view := snapshot.NewView(&backups[i])
		createdAt[view.ID] = view.CreatedAt
		items = append(items, fromBackup(view))
	}
	for i := range restores {
		item := fromRestore(restore.NewView(&restores[i]))
		if t, ok := createdAt[item.Restore.Snapshot]; ok && item.Restore.Snapshot != "" {
			item.Restore.SnapshotCreatedAt = &t
		}
		items = append(items, item)
	}
	return items, nil
}

// sortKey orders entries of both kinds by creation time: resource names start with a prefix and then the
// creation time (see crstore.NewName). The full name is appended so that the key is unique.
func sortKey(item Item) string {
	_, stamp, _ := strings.Cut(item.ID, "-")
	return stamp + "/" + item.ID
}

func statusOf(item Item) Status {
	switch item.Phase {
	case snapshot.PhaseSucceeded:
		return StatusSucceeded
	case snapshot.PhaseFailed:
		return StatusFailed
	default:
		return StatusActive
	}
}

func (c *Counts) add(item Item) {
	c.Total++
	switch statusOf(item) {
	case StatusSucceeded:
		c.Succeeded++
	case StatusFailed:
		c.Failed++
	default:
		c.Active++
	}
}

func fromBackup(v snapshot.View) Item {
	details := &BackupDetails{Volumes: len(v.Volumes)}
	for _, vol := range v.Volumes {
		details.Bytes += vol.SizeBytes
	}
	if v.Resources != nil {
		details.Resources = v.Resources.Count
	}
	return Item{
		Type: TypeBackup, ID: v.ID, Application: v.Application, Phase: v.Phase,
		Message: v.Message, ErrorCode: v.ErrorCode, Hint: v.Hint,
		CreatedAt: v.CreatedAt, FinishedAt: v.FinishedAt, Steps: v.Steps, Policy: v.Policy, Exports: v.Exports, Backup: details,
	}
}

func fromRestore(v restore.View) Item {
	details := &RestoreDetails{Snapshot: v.Snapshot, Warnings: v.Warnings}
	if v.Export != nil {
		details.ExportLocation, details.SnapshotCreatedAt = v.Export.LocationName, v.Export.CreatedAt
	}
	if v.Resources != nil {
		details.Created = v.Resources.Created
		details.Updated = v.Resources.Updated
	}
	return Item{
		Type: TypeRestore, ID: v.ID, Application: v.Application, Phase: v.Phase,
		Message: v.Message, ErrorCode: v.ErrorCode, Hint: v.Hint,
		CreatedAt: v.CreatedAt, FinishedAt: v.FinishedAt, Steps: v.Steps, Exports: []snapshot.ExportStatus{}, Restore: details,
	}
}
