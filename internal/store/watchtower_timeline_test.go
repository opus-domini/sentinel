package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestWatchtowerTimelineInsertSearchAndFilters(t *testing.T) {
	t.Parallel()
	const (
		sessionDev    = "dev"
		sessionOps    = "ops"
		severityError = "error"
	)

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)

	insert := func(row WatchtowerTimelineEventWrite) {
		t.Helper()
		if _, err := s.InsertWatchtowerTimelineEvent(ctx, row); err != nil {
			t.Fatalf("InsertWatchtowerTimelineEvent(%s): %v", row.EventType, err)
		}
	}

	insert(WatchtowerTimelineEventWrite{
		Session:    sessionDev,
		WindowIdx:  0,
		PaneID:     "%1",
		EventType:  "command.started",
		Severity:   "info",
		Command:    "go test ./...",
		Cwd:        "/repo",
		Summary:    "started go test",
		Metadata:   json.RawMessage(`{"source":"watchtower"}`),
		CreatedAt:  base.Add(-2 * time.Minute),
		DurationMS: 10,
	})
	insert(WatchtowerTimelineEventWrite{
		Session:    sessionDev,
		WindowIdx:  0,
		PaneID:     "%1",
		EventType:  "output.marker",
		Severity:   "error",
		Marker:     "panic",
		Summary:    "panic detected",
		Details:    "panic: runtime error",
		CreatedAt:  base.Add(-1 * time.Minute),
		DurationMS: -10,
	})
	insert(WatchtowerTimelineEventWrite{
		Session:   sessionOps,
		WindowIdx: 2,
		PaneID:    "%9",
		EventType: "command.finished",
		Severity:  "warn",
		Command:   "deploy",
		Summary:   "deploy took too long",
		CreatedAt: base,
	})

	allRows, err := s.SearchWatchtowerTimelineEvents(ctx, WatchtowerTimelineQuery{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchWatchtowerTimelineEvents(all): %v", err)
	}
	if len(allRows.Events) != 3 {
		t.Fatalf("len(allRows.Events) = %d, want 3", len(allRows.Events))
	}
	if allRows.Events[0].Session != sessionOps {
		t.Fatalf("latest event session = %q, want %s", allRows.Events[0].Session, sessionOps)
	}
	if allRows.Events[1].Severity != severityError {
		t.Fatalf("severity normalization failed, got %q want %s", allRows.Events[1].Severity, severityError)
	}
	if allRows.Events[1].DurationMS != 0 {
		t.Fatalf("negative duration should clamp to 0, got %d", allRows.Events[1].DurationMS)
	}

	devRows, err := s.SearchWatchtowerTimelineEvents(ctx, WatchtowerTimelineQuery{
		Session: sessionDev,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("SearchWatchtowerTimelineEvents(dev): %v", err)
	}
	if len(devRows.Events) != 2 {
		t.Fatalf("len(devRows.Events) = %d, want 2", len(devRows.Events))
	}

	panicRows, err := s.SearchWatchtowerTimelineEvents(ctx, WatchtowerTimelineQuery{
		Query: "panic",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchWatchtowerTimelineEvents(query=panic): %v", err)
	}
	if len(panicRows.Events) != 1 {
		t.Fatalf("len(panicRows.Events) = %d, want 1", len(panicRows.Events))
	}
	if panicRows.Events[0].Marker != "panic" {
		t.Fatalf("marker = %q, want panic", panicRows.Events[0].Marker)
	}

	limitedRows, err := s.SearchWatchtowerTimelineEvents(ctx, WatchtowerTimelineQuery{
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("SearchWatchtowerTimelineEvents(limit=1): %v", err)
	}
	if len(limitedRows.Events) != 1 {
		t.Fatalf("len(limitedRows.Events) = %d, want 1", len(limitedRows.Events))
	}
	if !limitedRows.HasMore {
		t.Fatalf("HasMore = false, want true")
	}
}

func TestWatchtowerTimelinePruneRows(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)

	for i := 0; i < 6; i++ {
		if _, err := s.InsertWatchtowerTimelineEvent(ctx, WatchtowerTimelineEventWrite{
			Session:   "dev",
			WindowIdx: 0,
			PaneID:    "%1",
			EventType: "output.marker",
			Summary:   "row",
			CreatedAt: base.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("InsertWatchtowerTimelineEvent #%d: %v", i, err)
		}
	}

	removed, err := s.PruneWatchtowerTimelineRows(ctx, 2)
	if err != nil {
		t.Fatalf("PruneWatchtowerTimelineRows: %v", err)
	}
	if removed != 4 {
		t.Fatalf("removed = %d, want 4", removed)
	}

	rows, err := s.SearchWatchtowerTimelineEvents(ctx, WatchtowerTimelineQuery{
		Session: "dev",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("SearchWatchtowerTimelineEvents after prune: %v", err)
	}
	if len(rows.Events) != 2 {
		t.Fatalf("len(rows.Events) = %d, want 2", len(rows.Events))
	}
}

func TestWatchtowerPaneRuntimeAccessors(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.UpsertWatchtowerPaneRuntime(ctx, WatchtowerPaneRuntimeWrite{
		PaneID:         "%1",
		SessionName:    "dev",
		WindowIdx:      0,
		CurrentCommand: "go test",
		StartedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerPaneRuntime(%%1): %v", err)
	}
	if err := s.UpsertWatchtowerPaneRuntime(ctx, WatchtowerPaneRuntimeWrite{
		PaneID:         "%2",
		SessionName:    "dev",
		WindowIdx:      1,
		CurrentCommand: "htop",
		StartedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerPaneRuntime(%%2): %v", err)
	}
	if err := s.UpsertWatchtowerPaneRuntime(ctx, WatchtowerPaneRuntimeWrite{
		PaneID:         "%1",
		SessionName:    "dev",
		WindowIdx:      0,
		CurrentCommand: "npm run test",
		StartedAt:      now.Add(2 * time.Second),
		UpdatedAt:      now.Add(2 * time.Second),
	}); err != nil {
		t.Fatalf("UpsertWatchtowerPaneRuntime update(%%1): %v", err)
	}

	rows, err := s.ListWatchtowerPaneRuntimeBySession(ctx, "dev")
	if err != nil {
		t.Fatalf("ListWatchtowerPaneRuntimeBySession(dev): %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if rows[0].PaneID != "%1" || rows[0].CurrentCommand != "npm run test" {
		t.Fatalf("unexpected first pane runtime row: %+v", rows[0])
	}

	if err := s.PurgeWatchtowerPaneRuntime(ctx, "dev", []string{"%2"}); err != nil {
		t.Fatalf("PurgeWatchtowerPaneRuntime keep %%2: %v", err)
	}
	rows, err = s.ListWatchtowerPaneRuntimeBySession(ctx, "dev")
	if err != nil {
		t.Fatalf("ListWatchtowerPaneRuntimeBySession(dev) after keep: %v", err)
	}
	if len(rows) != 1 || rows[0].PaneID != "%2" {
		t.Fatalf("unexpected rows after keep purge: %+v", rows)
	}

	if err := s.PurgeWatchtowerPaneRuntime(ctx, "dev", nil); err != nil {
		t.Fatalf("PurgeWatchtowerPaneRuntime clear: %v", err)
	}
	rows, err = s.ListWatchtowerPaneRuntimeBySession(ctx, "dev")
	if err != nil {
		t.Fatalf("ListWatchtowerPaneRuntimeBySession(dev) after clear: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("len(rows) after clear = %d, want 0", len(rows))
	}
}
