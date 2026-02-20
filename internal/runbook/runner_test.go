package runbook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/activity"
	"github.com/opus-domini/sentinel/internal/store"
)

// mockRepo implements Repo for testing the runner.
type mockRepo struct {
	mu sync.Mutex

	runbook   store.OpsRunbook
	runbookOK bool

	updatedRuns []store.OpsRunbookRunUpdate
	gotRunIDs   []string
	insertedTL  []activity.EventWrite

	// getRunbookErr, if set, is returned by GetOpsRunbook.
	getRunbookErr error
}

func (m *mockRepo) UpdateOpsRunbookRun(_ context.Context, update store.OpsRunbookRunUpdate) (store.OpsRunbookRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updatedRuns = append(m.updatedRuns, update)
	return store.OpsRunbookRun{ID: update.RunID, Status: update.Status}, nil
}

func (m *mockRepo) GetOpsRunbook(_ context.Context, id string) (store.OpsRunbook, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getRunbookErr != nil {
		return store.OpsRunbook{}, m.getRunbookErr
	}
	if m.runbookOK {
		return m.runbook, nil
	}
	return store.OpsRunbook{ID: id}, nil
}

func (m *mockRepo) GetOpsRunbookRun(_ context.Context, id string) (store.OpsRunbookRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gotRunIDs = append(m.gotRunIDs, id)
	return store.OpsRunbookRun{ID: id}, nil
}

func (m *mockRepo) InsertActivityEvent(_ context.Context, event activity.EventWrite) (activity.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.insertedTL = append(m.insertedTL, event)
	return activity.Event{ID: 1, Source: event.Source}, nil
}

func (m *mockRepo) lastUpdate() store.OpsRunbookRunUpdate {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.updatedRuns) == 0 {
		return store.OpsRunbookRunUpdate{}
	}
	return m.updatedRuns[len(m.updatedRuns)-1]
}

func (m *mockRepo) timelineCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.insertedTL)
}

func TestRunPersistsTerminalStateOnCancelledContext(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{
		runbookOK: true,
		runbook: store.OpsRunbook{
			ID:   "rb-1",
			Name: "test-runbook",
			Steps: []store.OpsRunbookStep{
				{Type: "command", Title: "slow step", Command: "sleep 60"},
			},
		},
	}

	var emittedEvents []string
	emit := func(eventType string, _ map[string]any) {
		emittedEvents = append(emittedEvents, eventType)
	}

	// Create a context that we cancel immediately to simulate shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before Run starts — simulates shutdown.

	Run(ctx, repo, emit, RunParams{
		Job:         store.OpsRunbookRun{ID: "run-1", RunbookID: "rb-1"},
		Source:      "test",
		StepTimeout: 1 * time.Second,
		RunTimeout:  1 * time.Second,
	})

	// The final update must have a terminal status and finished_at,
	// even though the context was cancelled before execution started.
	last := repo.lastUpdate()
	if last.Status != runnerStatusFailed {
		t.Errorf("last update status = %q, want %q", last.Status, runnerStatusFailed)
	}
	if last.FinishedAt == "" {
		t.Error("last update FinishedAt is empty, want non-empty timestamp")
	}

	// Timeline event must have been inserted despite cancelled context.
	if repo.timelineCount() == 0 {
		t.Error("no timeline events inserted, want at least 1")
	}
}

func TestRunPersistsTerminalStateOnTimeout(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{
		runbookOK: true,
		runbook: store.OpsRunbook{
			ID:   "rb-2",
			Name: "timeout-runbook",
			Steps: []store.OpsRunbookStep{
				{Type: "command", Title: "slow step", Command: "sleep 60"},
			},
		},
	}

	emit := func(_ string, _ map[string]any) {}

	// Use a very short RunTimeout so it expires mid-execution.
	Run(context.Background(), repo, emit, RunParams{
		Job:         store.OpsRunbookRun{ID: "run-2", RunbookID: "rb-2"},
		Source:      "test",
		StepTimeout: 5 * time.Second,
		RunTimeout:  100 * time.Millisecond,
	})

	last := repo.lastUpdate()
	if last.Status != runnerStatusFailed {
		t.Errorf("last update status = %q, want %q", last.Status, runnerStatusFailed)
	}
	if last.FinishedAt == "" {
		t.Error("last update FinishedAt is empty, want non-empty timestamp")
	}
	if repo.timelineCount() == 0 {
		t.Error("no timeline events inserted, want at least 1")
	}
}

func TestRunOnFinishReceivesFinalizeContext(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{
		runbookOK: true,
		runbook: store.OpsRunbook{
			ID:   "rb-3",
			Name: "callback-runbook",
			Steps: []store.OpsRunbookStep{
				{Type: "command", Title: "quick", Command: "echo ok"},
			},
		},
	}

	emit := func(_ string, _ map[string]any) {}

	// Cancel before Run starts.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var onFinishCtxErr error
	var onFinishStatus string

	Run(ctx, repo, emit, RunParams{
		Job:         store.OpsRunbookRun{ID: "run-3", RunbookID: "rb-3"},
		Source:      "test",
		StepTimeout: 1 * time.Second,
		RunTimeout:  1 * time.Second,
		OnFinish: func(ctx context.Context, status string) {
			onFinishCtxErr = ctx.Err()
			onFinishStatus = status
		},
	})

	// OnFinish must have been called with a non-cancelled context.
	if onFinishCtxErr != nil {
		t.Errorf("OnFinish ctx.Err() = %v, want nil (finalize context should not be cancelled)", onFinishCtxErr)
	}
	if onFinishStatus != runnerStatusFailed {
		t.Errorf("OnFinish status = %q, want %q", onFinishStatus, runnerStatusFailed)
	}
}

func TestBuildWebhookPayload(t *testing.T) {
	t.Parallel()

	params := RunParams{
		Job: store.OpsRunbookRun{
			ID:         "run-42",
			RunbookID:  "rb-7",
			RunbookName: "Deploy Service",
		},
		Source: "scheduler",
	}
	job := store.OpsRunbookRun{
		ID:             "run-42",
		Status:         "succeeded",
		TotalSteps:     3,
		CompletedSteps: 3,
		StartedAt:      "2026-02-20T22:00:00Z",
		FinishedAt:     "2026-02-20T22:01:00Z",
		StepResults: []store.OpsRunbookStepResult{
			{StepIndex: 0, Title: "Build", Type: "command", Output: "ok", DurationMs: 120},
			{StepIndex: 1, Title: "Test", Type: "command", Output: "passed", DurationMs: 340},
			{StepIndex: 2, Title: "Verify", Type: "check", DurationMs: 50},
		},
	}

	payload := buildWebhookPayload(params, job)

	if payload.Event != "runbook.completed" {
		t.Fatalf("event = %q, want runbook.completed", payload.Event)
	}
	if payload.SentAt == "" {
		t.Fatal("sentAt should not be empty")
	}
	if payload.Runbook.ID != "rb-7" {
		t.Fatalf("runbook.id = %q, want rb-7", payload.Runbook.ID)
	}
	if payload.Runbook.Name != "Deploy Service" {
		t.Fatalf("runbook.name = %q, want Deploy Service", payload.Runbook.Name)
	}
	if payload.Job.ID != "run-42" {
		t.Fatalf("job.id = %q, want run-42", payload.Job.ID)
	}
	if payload.Job.Status != "succeeded" {
		t.Fatalf("job.status = %q, want succeeded", payload.Job.Status)
	}
	if payload.Job.Source != "scheduler" {
		t.Fatalf("job.source = %q, want scheduler", payload.Job.Source)
	}
	if payload.Job.TotalSteps != 3 {
		t.Fatalf("job.totalSteps = %d, want 3", payload.Job.TotalSteps)
	}
	if payload.Job.CompletedSteps != 3 {
		t.Fatalf("job.completedSteps = %d, want 3", payload.Job.CompletedSteps)
	}
	if payload.Job.StartedAt != "2026-02-20T22:00:00Z" {
		t.Fatalf("job.startedAt = %q, want 2026-02-20T22:00:00Z", payload.Job.StartedAt)
	}
	if payload.Job.FinishedAt != "2026-02-20T22:01:00Z" {
		t.Fatalf("job.finishedAt = %q, want 2026-02-20T22:01:00Z", payload.Job.FinishedAt)
	}

	// Step details.
	if len(payload.Job.Steps) != 3 {
		t.Fatalf("len(steps) = %d, want 3", len(payload.Job.Steps))
	}
	if payload.Job.Steps[0].Title != "Build" {
		t.Fatalf("steps[0].title = %q, want Build", payload.Job.Steps[0].Title)
	}
	if payload.Job.Steps[0].Output != "ok" {
		t.Fatalf("steps[0].output = %q, want ok", payload.Job.Steps[0].Output)
	}
	if payload.Job.Steps[0].DurationMs != 120 {
		t.Fatalf("steps[0].durationMs = %d, want 120", payload.Job.Steps[0].DurationMs)
	}
	if payload.Job.Steps[1].Type != "command" {
		t.Fatalf("steps[1].type = %q, want command", payload.Job.Steps[1].Type)
	}
	if payload.Job.Steps[2].Title != "Verify" {
		t.Fatalf("steps[2].title = %q, want Verify", payload.Job.Steps[2].Title)
	}
}

func TestBuildWebhookPayloadFailedRun(t *testing.T) {
	t.Parallel()

	params := RunParams{
		Job: store.OpsRunbookRun{
			ID:          "run-99",
			RunbookID:   "rb-1",
			RunbookName: "Health Check",
		},
		Source: "runbook",
	}
	job := store.OpsRunbookRun{
		ID:             "run-99",
		Status:         "failed",
		TotalSteps:     2,
		CompletedSteps: 1,
		Error:          "step 2 timed out",
		StartedAt:      "2026-02-20T22:00:00Z",
		FinishedAt:     "2026-02-20T22:00:35Z",
		StepResults: []store.OpsRunbookStepResult{
			{StepIndex: 0, Title: "Setup", Type: "command", Output: "done", DurationMs: 200},
			{StepIndex: 1, Title: "Deploy", Type: "command", Error: "timed out", DurationMs: 30000},
		},
	}

	payload := buildWebhookPayload(params, job)

	if payload.Job.Status != "failed" {
		t.Fatalf("job.status = %q, want failed", payload.Job.Status)
	}
	if payload.Job.Error != "step 2 timed out" {
		t.Fatalf("job.error = %q, want 'step 2 timed out'", payload.Job.Error)
	}
	if payload.Job.CompletedSteps != 1 {
		t.Fatalf("job.completedSteps = %d, want 1", payload.Job.CompletedSteps)
	}
	if len(payload.Job.Steps) != 2 {
		t.Fatalf("len(steps) = %d, want 2", len(payload.Job.Steps))
	}
	if payload.Job.Steps[1].Error != "timed out" {
		t.Fatalf("steps[1].error = %q, want 'timed out'", payload.Job.Steps[1].Error)
	}
}

func TestBuildWebhookPayloadOmitsEmptyFields(t *testing.T) {
	t.Parallel()

	params := RunParams{
		Job: store.OpsRunbookRun{
			ID:          "run-0",
			RunbookID:   "rb-0",
			RunbookName: "Minimal",
		},
		Source: "runbook",
	}
	job := store.OpsRunbookRun{
		ID:             "run-0",
		Status:         "succeeded",
		TotalSteps:     1,
		CompletedSteps: 1,
	}

	payload := buildWebhookPayload(params, job)

	// Marshal to JSON and verify omitempty fields are absent.
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	raw := string(data)
	if strings.Contains(raw, `"error"`) {
		t.Fatalf("JSON should omit empty error field: %s", raw)
	}
	if strings.Contains(raw, `"startedAt"`) {
		t.Fatalf("JSON should omit empty startedAt field: %s", raw)
	}
	if strings.Contains(raw, `"finishedAt"`) {
		t.Fatalf("JSON should omit empty finishedAt field: %s", raw)
	}
}

func TestFireWebhookDeliversPayload(t *testing.T) {
	t.Parallel()

	var received webhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	payload := webhookPayload{
		Event:  "runbook.completed",
		SentAt: "2026-02-20T22:00:00Z",
		Runbook: webhookRunbook{
			ID:   "rb-1",
			Name: "Test",
		},
		Job: webhookJob{
			ID:             "run-1",
			Status:         "succeeded",
			TotalSteps:     1,
			CompletedSteps: 1,
		},
	}

	fireWebhook(context.Background(), server.URL, payload)

	if received.Event != "runbook.completed" {
		t.Fatalf("received event = %q, want runbook.completed", received.Event)
	}
	if received.Runbook.ID != "rb-1" {
		t.Fatalf("received runbook.id = %q, want rb-1", received.Runbook.ID)
	}
	if received.Job.Status != "succeeded" {
		t.Fatalf("received job.status = %q, want succeeded", received.Job.Status)
	}
}

func TestFireWebhookHandlesServerError(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		attempts++
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// Should not panic; logs a warning instead.
	fireWebhook(context.Background(), server.URL, map[string]string{"test": "true"})

	mu.Lock()
	got := attempts
	mu.Unlock()
	if got < 1 {
		t.Fatalf("attempts = %d, want at least 1", got)
	}
}

func TestFireWebhookHandlesInvalidURL(t *testing.T) {
	t.Parallel()

	// Use a short-lived context so we don't wait for full retry timeouts.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Should not panic on an unreachable URL.
	fireWebhook(ctx, "http://192.0.2.1:1/webhook", map[string]string{"test": "true"})
}
