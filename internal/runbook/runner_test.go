package runbook

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/timeline"
)

// mockRepo implements Repo for testing the runner.
type mockRepo struct {
	mu sync.Mutex

	runbook   store.OpsRunbook
	runbookOK bool

	updatedRuns []store.OpsRunbookRunUpdate
	gotRunIDs   []string
	insertedTL  []timeline.EventWrite

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

func (m *mockRepo) InsertTimelineEvent(_ context.Context, event timeline.EventWrite) (timeline.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.insertedTL = append(m.insertedTL, event)
	return timeline.Event{ID: 1, Source: event.Source}, nil
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
