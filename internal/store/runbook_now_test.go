package store

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestListOpsRunbookLatestTerminalRunsUsesLatestTerminalPerRunbook(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)

	failureThenSuccess := createNowTestRunbook(t, s, "failure-then-success")
	failedFirst := createNowTestRun(t, s, failureThenSuccess.ID, base, OpsRunbookStatusFailed)
	successLatest := createNowTestRun(t, s, failureThenSuccess.ID, base.Add(time.Minute), OpsRunbookStatusSucceeded)

	failureThenActive := createNowTestRunbook(t, s, "failure-then-active")
	failureLatest := createNowTestRun(t, s, failureThenActive.ID, base.Add(2*time.Minute), OpsRunbookStatusFailed)
	activeLatest := createNowTestRun(t, s, failureThenActive.ID, base.Add(3*time.Minute), OpsRunbookStatusRunning)

	onlyFailure := createNowTestRunbook(t, s, "only-failure")
	onlyFailureRun := createNowTestRun(t, s, onlyFailure.ID, base.Add(4*time.Minute), OpsRunbookStatusFailed)

	got, err := s.ListOpsRunbookLatestTerminalRuns(ctx)
	if err != nil {
		t.Fatalf("ListOpsRunbookLatestTerminalRuns() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(runs) = %d, want 3", len(got))
	}

	gotIDs := []string{got[0].ID, got[1].ID, got[2].ID}
	wantIDs := []string{onlyFailureRun.ID, failureLatest.ID, successLatest.ID}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("run ids = %v, want %v", gotIDs, wantIDs)
	}
	if slices.Contains(gotIDs, failedFirst.ID) {
		t.Fatalf("superseded failure %q was returned", failedFirst.ID)
	}
	if slices.Contains(gotIDs, activeLatest.ID) {
		t.Fatalf("active run %q was returned as terminal", activeLatest.ID)
	}
}

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
