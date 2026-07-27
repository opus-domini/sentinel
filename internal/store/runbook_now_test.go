package store

import (
	"context"
	"testing"
	"time"
)

func TestListOpsRunbookNowQueriesAreDeterministicAndUnbounded(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	at := time.Date(2026, 7, 27, 13, 0, 0, 0, time.UTC)
	runbook := createNowTestRunbook(t, s, "deterministic")

	active := make([]OpsRunbookRun, 0, 24)
	for i := range 24 {
		status := OpsRunbookStatusQueued
		if i%2 == 0 {
			status = OpsRunbookStatusWaitingApproval
		}
		active = append(active, createNowTestRun(t, s, runbook.ID, at, status))
	}

	got, err := s.ListOpsRunbookActiveRuns(ctx)
	if err != nil {
		t.Fatalf("ListOpsRunbookActiveRuns() error = %v", err)
	}
	if len(got) != len(active) {
		t.Fatalf("len(runs) = %d, want %d", len(got), len(active))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].CreatedAt < got[i].CreatedAt ||
			(got[i-1].CreatedAt == got[i].CreatedAt && got[i-1].ID < got[i].ID) {
			t.Fatalf("runs are not ordered by created_at DESC, id DESC at %d: %q before %q", i, got[i-1].ID, got[i].ID)
		}
	}
}

func createNowTestRunbook(t *testing.T, s *Store, name string) OpsRunbook {
	t.Helper()

	runbook, err := s.InsertOpsRunbook(context.Background(), OpsRunbookWrite{
		Name:    name,
		Enabled: true,
		Steps: []OpsRunbookStep{{
			Type:    "command",
			Title:   "check",
			Command: "true",
		}},
	})
	if err != nil {
		t.Fatalf("InsertOpsRunbook(%q) error = %v", name, err)
	}
	return runbook
}

func createNowTestRun(t *testing.T, s *Store, runbookID string, at time.Time, status string) OpsRunbookRun {
	t.Helper()

	runbook, err := s.GetOpsRunbook(context.Background(), runbookID)
	if err != nil {
		t.Fatalf("GetOpsRunbook() error = %v", err)
	}
	run, err := s.CreateOpsRunbookRun(context.Background(), OpsRunbookRunWrite{
		Definition: runbook,
		Source:     OpsRunbookRunSourceRunbooks,
		At:         at,
	})
	if err != nil {
		t.Fatalf("CreateOpsRunbookRun() error = %v", err)
	}
	if status == OpsRunbookStatusQueued {
		return run
	}

	update := OpsRunbookRunUpdate{
		RunID:       run.ID,
		Status:      status,
		StepResults: "[]",
	}
	if status == OpsRunbookStatusRunning || status == OpsRunbookStatusWaitingApproval {
		update.StartedAt = at.Format(time.RFC3339)
	} else {
		update.FinishedAt = at.Format(time.RFC3339)
	}
	updated, err := s.UpdateOpsRunbookRun(context.Background(), update)
	if err != nil {
		t.Fatalf("UpdateOpsRunbookRun(%q) error = %v", status, err)
	}
	return updated
}
