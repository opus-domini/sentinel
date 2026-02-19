package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/activity"
)

func TestOpsActivityInsertAndSearch(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	_, err := s.InsertActivityEvent(ctx, activity.EventWrite{
		Source:    "service",
		EventType: "service.action",
		Severity:  "warn",
		Resource:  "sentinel",
		Message:   "restart executed",
		Details:   "systemctl --user restart sentinel",
		Metadata:  `{"action":"restart"}`,
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("InsertActivityEvent: %v", err)
	}

	result, err := s.SearchActivityEvents(ctx, activity.Query{
		Query:    "restart",
		Severity: "warn",
		Source:   "service",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("SearchActivityEvents: %v", err)
	}
	if len(result.Events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(result.Events))
	}
	event := result.Events[0]
	if event.EventType != "service.action" || event.Resource != "sentinel" {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestInsertOpsActivityEventDefaults(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	// Insert with all empty/default fields.
	event, err := s.InsertActivityEvent(ctx, activity.EventWrite{
		Message: "bare event",
	})
	if err != nil {
		t.Fatalf("InsertActivityEvent: %v", err)
	}
	if event.Source != "ops" {
		t.Fatalf("source = %q, want ops (default)", event.Source)
	}
	if event.EventType != "ops.event" {
		t.Fatalf("eventType = %q, want ops.event (default)", event.EventType)
	}
	if event.Severity != activity.SeverityInfo {
		t.Fatalf("severity = %q, want %q (default)", event.Severity, activity.SeverityInfo)
	}
	if event.CreatedAt == "" {
		t.Fatalf("createdAt should be set by default")
	}
}

func TestSearchOpsActivityEventsFilters(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	// Seed diverse events.
	events := []activity.EventWrite{
		{Source: "service", EventType: "restart", Severity: "warn", Resource: "nginx", Message: "nginx restarted", CreatedAt: base},
		{Source: "service", EventType: "start", Severity: "info", Resource: "redis", Message: "redis started", CreatedAt: base.Add(time.Second)},
		{Source: "deploy", EventType: "deploy", Severity: "error", Resource: "app", Message: "deploy failed", CreatedAt: base.Add(2 * time.Second)},
	}
	for _, e := range events {
		if _, err := s.InsertActivityEvent(ctx, e); err != nil {
			t.Fatalf("InsertActivityEvent(%s): %v", e.Resource, err)
		}
	}

	t.Run("filter by severity only", func(t *testing.T) {
		result, err := s.SearchActivityEvents(ctx, activity.Query{Severity: "error"})
		if err != nil {
			t.Fatalf("SearchActivityEvents: %v", err)
		}
		if len(result.Events) != 1 || result.Events[0].Resource != "app" {
			t.Fatalf("expected 1 error event (app), got %d: %+v", len(result.Events), result.Events)
		}
	})

	t.Run("filter by source only", func(t *testing.T) {
		result, err := s.SearchActivityEvents(ctx, activity.Query{Source: "service"})
		if err != nil {
			t.Fatalf("SearchActivityEvents: %v", err)
		}
		if len(result.Events) != 2 {
			t.Fatalf("expected 2 service events, got %d", len(result.Events))
		}
	})

	t.Run("filter by query text", func(t *testing.T) {
		result, err := s.SearchActivityEvents(ctx, activity.Query{Query: "redis"})
		if err != nil {
			t.Fatalf("SearchActivityEvents: %v", err)
		}
		if len(result.Events) != 1 || result.Events[0].Resource != "redis" {
			t.Fatalf("expected 1 redis event, got %d", len(result.Events))
		}
	})

	t.Run("empty query returns all", func(t *testing.T) {
		result, err := s.SearchActivityEvents(ctx, activity.Query{})
		if err != nil {
			t.Fatalf("SearchActivityEvents: %v", err)
		}
		if len(result.Events) != 3 {
			t.Fatalf("expected 3 events, got %d", len(result.Events))
		}
	})

	t.Run("severity 'all' returns all", func(t *testing.T) {
		result, err := s.SearchActivityEvents(ctx, activity.Query{Severity: "all"})
		if err != nil {
			t.Fatalf("SearchActivityEvents: %v", err)
		}
		if len(result.Events) != 3 {
			t.Fatalf("expected 3 events, got %d", len(result.Events))
		}
	})

	t.Run("invalid severity returns error", func(t *testing.T) {
		_, err := s.SearchActivityEvents(ctx, activity.Query{Severity: "critical"})
		if err == nil {
			t.Fatalf("expected error for invalid severity")
		}
		if !errors.Is(err, activity.ErrInvalidFilter) {
			t.Fatalf("error = %v, want activity.ErrInvalidFilter", err)
		}
	})

	t.Run("HasMore when limit exceeded", func(t *testing.T) {
		result, err := s.SearchActivityEvents(ctx, activity.Query{Limit: 2})
		if err != nil {
			t.Fatalf("SearchActivityEvents: %v", err)
		}
		if !result.HasMore {
			t.Fatalf("hasMore = false, want true")
		}
		if len(result.Events) != 2 {
			t.Fatalf("len(events) = %d, want 2 (limited)", len(result.Events))
		}
	})

	t.Run("negative limit defaults to 100", func(t *testing.T) {
		result, err := s.SearchActivityEvents(ctx, activity.Query{Limit: -5})
		if err != nil {
			t.Fatalf("SearchActivityEvents: %v", err)
		}
		// Should return all 3 events (well under default 100 limit).
		if len(result.Events) != 3 {
			t.Fatalf("len(events) = %d, want 3", len(result.Events))
		}
	})

	t.Run("severity aliases normalized", func(t *testing.T) {
		// "warning" should be treated as "warn".
		result, err := s.SearchActivityEvents(ctx, activity.Query{Severity: "warning"})
		if err != nil {
			t.Fatalf("SearchActivityEvents: %v", err)
		}
		if len(result.Events) != 1 || result.Events[0].Severity != activity.SeverityWarn {
			t.Fatalf("expected 1 warn event, got %d", len(result.Events))
		}

		// "err" should be treated as "error".
		result, err = s.SearchActivityEvents(ctx, activity.Query{Severity: "err"})
		if err != nil {
			t.Fatalf("SearchActivityEvents(err): %v", err)
		}
		if len(result.Events) != 1 || result.Events[0].Severity != activity.SeverityError {
			t.Fatalf("expected 1 error event, got %d", len(result.Events))
		}
	})

	t.Run("results ordered by created_at DESC", func(t *testing.T) {
		result, err := s.SearchActivityEvents(ctx, activity.Query{})
		if err != nil {
			t.Fatalf("SearchActivityEvents: %v", err)
		}
		if len(result.Events) < 2 {
			t.Fatalf("need at least 2 events for ordering check")
		}
		// First event should be the most recent.
		if result.Events[0].Resource != "app" {
			t.Fatalf("first event = %q, want app (most recent)", result.Events[0].Resource)
		}
	})
}

func TestPruneOpsActivityRows(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	// Insert 15 events with distinct timestamps.
	for i := range 15 {
		if _, err := s.InsertActivityEvent(ctx, activity.EventWrite{
			Source:    "test",
			EventType: "ops.event",
			Severity:  "info",
			Resource:  "res",
			Message:   "event " + time.Duration(i).String(),
			CreatedAt: base.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("InsertActivityEvent(%d): %v", i, err)
		}
	}

	// Verify all 15 exist.
	all, err := s.SearchActivityEvents(ctx, activity.Query{Limit: 100})
	if err != nil {
		t.Fatalf("SearchActivityEvents: %v", err)
	}
	if len(all.Events) != 15 {
		t.Fatalf("pre-prune count = %d, want 15", len(all.Events))
	}

	// Prune to keep only 10.
	removed, err := s.PruneOpsActivityRows(ctx, 10)
	if err != nil {
		t.Fatalf("PruneOpsActivityRows: %v", err)
	}
	if removed != 5 {
		t.Fatalf("removed = %d, want 5", removed)
	}

	// Verify 10 remain and they are the newest.
	remaining, err := s.SearchActivityEvents(ctx, activity.Query{Limit: 100})
	if err != nil {
		t.Fatalf("SearchActivityEvents after prune: %v", err)
	}
	if len(remaining.Events) != 10 {
		t.Fatalf("post-prune count = %d, want 10", len(remaining.Events))
	}
	// The newest event should be the last one inserted (base + 14s).
	newest := remaining.Events[0]
	wantNewest := base.Add(14 * time.Second).Format(time.RFC3339)
	if newest.CreatedAt != wantNewest {
		t.Fatalf("newest event createdAt = %q, want %q", newest.CreatedAt, wantNewest)
	}

	t.Run("zero maxRows is no-op", func(t *testing.T) {
		t.Parallel()
		s2 := newTestStore(t)
		n, err := s2.PruneOpsActivityRows(context.Background(), 0)
		if err != nil {
			t.Fatalf("PruneOpsActivityRows(0): %v", err)
		}
		if n != 0 {
			t.Fatalf("removed = %d, want 0", n)
		}
	})

	t.Run("negative maxRows is no-op", func(t *testing.T) {
		t.Parallel()
		s2 := newTestStore(t)
		n, err := s2.PruneOpsActivityRows(context.Background(), -5)
		if err != nil {
			t.Fatalf("PruneOpsActivityRows(-5): %v", err)
		}
		if n != 0 {
			t.Fatalf("removed = %d, want 0", n)
		}
	})
}
