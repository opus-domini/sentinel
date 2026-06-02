package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStorageStatsAndFlush(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)
	seedStorageStatsData(ctx, t, s, base)

	stats, err := s.GetStorageStats(ctx)
	if err != nil {
		t.Fatalf("GetStorageStats: %v", err)
	}
	if len(stats.Resources) != 2 {
		t.Fatalf("len(resources) = %d, want 2", len(stats.Resources))
	}

	rowsByResource := make(map[string]int64, len(stats.Resources))
	for _, item := range stats.Resources {
		rowsByResource[item.Resource] = item.Rows
	}
	for _, resource := range []string{
		StorageResourceActivityLog,
		StorageResourceOpsJobs,
	} {
		if rowsByResource[resource] < 1 {
			t.Fatalf("resource %q rows = %d, want >= 1", resource, rowsByResource[resource])
		}
	}

	results, err := s.FlushStorageResource(ctx, StorageResourceAll)
	if err != nil {
		t.Fatalf("FlushStorageResource(all): %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	after, err := s.GetStorageStats(ctx)
	if err != nil {
		t.Fatalf("GetStorageStats after flush: %v", err)
	}
	for _, item := range after.Resources {
		if item.Rows != 0 {
			t.Fatalf("resource %q rows after flush = %d, want 0", item.Resource, item.Rows)
		}
	}
}

func seedStorageStatsData(ctx context.Context, t *testing.T, s *Store, base time.Time) {
	t.Helper()
	if _, err := s.InsertWatchtowerJournal(ctx, WatchtowerJournalWrite{
		GlobalRev:  1,
		EntityType: "pane",
		Session:    "dev",
		WindowIdx:  0,
		PaneID:     "%1",
		ChangeKind: "updated",
		ChangedAt:  base,
	}); err != nil {
		t.Fatalf("InsertWatchtowerJournal: %v", err)
	}
	runbooks, err := s.ListOpsRunbooks(ctx)
	if err != nil {
		t.Fatalf("ListOpsRunbooks: %v", err)
	}
	if len(runbooks) == 0 {
		t.Fatalf("expected at least one seeded runbook")
	}
	if _, err := s.StartOpsRunbook(ctx, runbooks[0].ID, base); err != nil {
		t.Fatalf("StartOpsRunbook: %v", err)
	}
}

func TestFlushStorageRejectsInvalidResource(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	_, err := s.FlushStorageResource(context.Background(), "unknown")
	if !errors.Is(err, ErrInvalidStorageResource) {
		t.Fatalf("error = %v, want ErrInvalidStorageResource", err)
	}
}
