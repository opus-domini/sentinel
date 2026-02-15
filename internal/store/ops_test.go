package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestOpsTimelineInsertAndSearch(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	_, err := s.InsertOpsTimelineEvent(ctx, OpsTimelineEventWrite{
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
		t.Fatalf("InsertOpsTimelineEvent: %v", err)
	}

	result, err := s.SearchOpsTimelineEvents(ctx, OpsTimelineQuery{
		Query:    "restart",
		Severity: "warn",
		Source:   "service",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("SearchOpsTimelineEvents: %v", err)
	}
	if len(result.Events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(result.Events))
	}
	event := result.Events[0]
	if event.EventType != "service.action" || event.Resource != "sentinel" {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestOpsAlertUpsertAndAck(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	first, err := s.UpsertOpsAlert(ctx, OpsAlertWrite{
		DedupeKey: "service:sentinel:failed",
		Source:    "service",
		Resource:  "sentinel",
		Title:     "Sentinel failed",
		Message:   "service state changed to failed",
		Severity:  "error",
		Metadata:  `{"state":"failed"}`,
		CreatedAt: base,
	})
	if err != nil {
		t.Fatalf("UpsertOpsAlert(first): %v", err)
	}

	second, err := s.UpsertOpsAlert(ctx, OpsAlertWrite{
		DedupeKey: "service:sentinel:failed",
		Source:    "service",
		Resource:  "sentinel",
		Title:     "Sentinel failed",
		Message:   "service state changed to failed again",
		Severity:  "error",
		Metadata:  `{"state":"failed"}`,
		CreatedAt: base.Add(30 * time.Second),
	})
	if err != nil {
		t.Fatalf("UpsertOpsAlert(second): %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("alert id changed on dedupe: first=%d second=%d", first.ID, second.ID)
	}
	if second.Occurrences != 2 {
		t.Fatalf("occurrences = %d, want 2", second.Occurrences)
	}

	alerts, err := s.ListOpsAlerts(ctx, 10, opsAlertStatusOpen)
	if err != nil {
		t.Fatalf("ListOpsAlerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("len(alerts) = %d, want 1", len(alerts))
	}

	acked, err := s.AckOpsAlert(ctx, first.ID, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("AckOpsAlert: %v", err)
	}
	if acked.Status != opsAlertStatusAcked {
		t.Fatalf("status = %q, want %q", acked.Status, opsAlertStatusAcked)
	}
	if acked.AckedAt == "" {
		t.Fatalf("ackedAt should not be empty")
	}

	_, err = s.AckOpsAlert(ctx, 99999, base)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ack missing alert error = %v, want sql.ErrNoRows", err)
	}
}

func TestOpsRunbooksAndRuns(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	runbooks, err := s.ListOpsRunbooks(ctx)
	if err != nil {
		t.Fatalf("ListOpsRunbooks: %v", err)
	}
	if len(runbooks) == 0 {
		t.Fatalf("expected seeded runbooks")
	}

	run, err := s.StartOpsRunbook(ctx, runbooks[0].ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("StartOpsRunbook: %v", err)
	}
	if run.Status != opsRunbookStatusSucceeded {
		t.Fatalf("status = %q, want %q", run.Status, opsRunbookStatusSucceeded)
	}
	if run.TotalSteps < 1 {
		t.Fatalf("total steps = %d, want >= 1", run.TotalSteps)
	}
	if run.CompletedSteps != run.TotalSteps {
		t.Fatalf("completed=%d total=%d, want equal", run.CompletedSteps, run.TotalSteps)
	}

	loaded, err := s.GetOpsRunbookRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetOpsRunbookRun: %v", err)
	}
	if loaded.ID != run.ID {
		t.Fatalf("run id = %q, want %q", loaded.ID, run.ID)
	}

	history, err := s.ListOpsRunbookRuns(ctx, 10)
	if err != nil {
		t.Fatalf("ListOpsRunbookRuns: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("len(history) = %d, want 1", len(history))
	}
}
