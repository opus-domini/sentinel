package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/store"
)

func TestRejectOpsRunbookRun(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	h.events = events.NewHub()
	run := createWaitingApprovalRun(t, st)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ops/jobs/"+run.ID+"/reject", nil)
	r.SetPathValue("runId", run.ID)
	h.rejectOpsRunbookRun(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("reject status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	updated, err := st.GetOpsRunbookRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetOpsRunbookRun: %v", err)
	}
	if updated.Status != "failed" {
		t.Fatalf("run status = %q, want failed", updated.Status)
	}
	if updated.Error != "approval rejected" {
		t.Fatalf("run error = %q, want approval rejected", updated.Error)
	}
	if updated.FinishedAt == "" {
		t.Fatal("FinishedAt is empty")
	}
	if len(updated.StepResults) != 1 {
		t.Fatalf("step results = %d, want approval evidence preserved", len(updated.StepResults))
	}
	if updated.StepResults[0].Type != "approval" {
		t.Fatalf("step result type = %q, want approval", updated.StepResults[0].Type)
	}
}

func TestApproveOpsRunbookRunResumesRun(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	h.events = events.NewHub()
	run := createWaitingApprovalRun(t, st)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ops/jobs/"+run.ID+"/approve", nil)
	r.SetPathValue("runId", run.ID)
	h.approveOpsRunbookRun(w, r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("approve status = %d, want 202; body=%s", w.Code, w.Body.String())
	}
	h.wg.Wait()

	updated, err := st.GetOpsRunbookRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetOpsRunbookRun: %v", err)
	}
	if updated.Status != "succeeded" {
		t.Fatalf("run status = %q, want succeeded", updated.Status)
	}
	if updated.CompletedSteps != 1 {
		t.Fatalf("completed steps = %d, want 1", updated.CompletedSteps)
	}
	if len(updated.StepResults) != 1 {
		t.Fatalf("step results = %d, want approval evidence preserved", len(updated.StepResults))
	}
	if updated.StepResults[0].Type != "approval" {
		t.Fatalf("step result type = %q, want approval", updated.StepResults[0].Type)
	}
}

func TestApproveOpsRunbookRunResumesFollowingSteps(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	h.events = events.NewHub()
	run := createWaitingApprovalRunWithSteps(t, st, []store.OpsRunbookStep{
		{Type: "approval", Title: "Approve"},
		{Type: "run", Title: "After approval", Command: "printf after"},
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ops/jobs/"+run.ID+"/approve", nil)
	r.SetPathValue("runId", run.ID)
	h.approveOpsRunbookRun(w, r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("approve status = %d, want 202; body=%s", w.Code, w.Body.String())
	}
	h.wg.Wait()

	updated, err := st.GetOpsRunbookRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetOpsRunbookRun: %v", err)
	}
	if updated.Status != "succeeded" {
		t.Fatalf("run status = %q, want succeeded", updated.Status)
	}
	if updated.CompletedSteps != 2 {
		t.Fatalf("completed steps = %d, want 2", updated.CompletedSteps)
	}
	if len(updated.StepResults) != 2 {
		t.Fatalf("step results = %d, want approval and resumed step", len(updated.StepResults))
	}
	if updated.StepResults[1].Type != "run" {
		t.Fatalf("step result type = %q, want run", updated.StepResults[1].Type)
	}
	if updated.StepResults[1].Output != "after" {
		t.Fatalf("step result output = %q, want after", updated.StepResults[1].Output)
	}
}

func TestApproveOpsRunbookRunRejectsInvalidState(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	run := createWaitingApprovalRun(t, st)
	if _, err := st.UpdateOpsRunbookRun(context.Background(), store.OpsRunbookRunUpdate{
		RunID:          run.ID,
		Status:         "succeeded",
		CompletedSteps: 1,
		CurrentStep:    "Approve",
		StepResults:    "[]",
		FinishedAt:     time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("UpdateOpsRunbookRun: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ops/jobs/"+run.ID+"/approve", nil)
	r.SetPathValue("runId", run.ID)
	h.approveOpsRunbookRun(w, r)

	if w.Code != http.StatusConflict {
		t.Fatalf("approve invalid state status = %d, want 409; body=%s", w.Code, w.Body.String())
	}
}

func createWaitingApprovalRun(t *testing.T, st *store.Store) store.OpsRunbookRun {
	t.Helper()

	return createWaitingApprovalRunWithSteps(t, st, []store.OpsRunbookStep{{Type: "approval", Title: "Approve"}})
}

func createWaitingApprovalRunWithSteps(
	t *testing.T,
	st *store.Store,
	steps []store.OpsRunbookStep,
) store.OpsRunbookRun {
	t.Helper()

	ctx := context.Background()
	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		Name:  "approval-test",
		Steps: steps,
	})
	if err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}
	run, err := st.CreateOpsRunbookRun(ctx, rb.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("CreateOpsRunbookRun: %v", err)
	}
	results, err := json.Marshal([]store.OpsRunbookStepResult{{
		StepIndex: 0,
		Title:     "Approve",
		Type:      "approval",
	}})
	if err != nil {
		t.Fatalf("marshal step results: %v", err)
	}
	updated, err := st.UpdateOpsRunbookRun(ctx, store.OpsRunbookRunUpdate{
		RunID:          run.ID,
		Status:         store.OpsRunbookStatusWaitingApproval,
		CompletedSteps: 1,
		CurrentStep:    "Approve",
		StepResults:    string(results),
		StartedAt:      time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("UpdateOpsRunbookRun(waiting): %v", err)
	}
	return updated
}
