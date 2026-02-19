package runbook

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/timeline"
)

// Repo defines the store operations consumed by the runbook runner.
type Repo interface {
	UpdateOpsRunbookRun(ctx context.Context, update store.OpsRunbookRunUpdate) (store.OpsRunbookRun, error)
	GetOpsRunbook(ctx context.Context, id string) (store.OpsRunbook, error)
	GetOpsRunbookRun(ctx context.Context, id string) (store.OpsRunbookRun, error)
	InsertTimelineEvent(ctx context.Context, event timeline.EventWrite) (timeline.Event, error)
}

// EmitFunc publishes a real-time event to connected clients.
type EmitFunc func(eventType string, payload map[string]any)

// RunParams configures a single runbook execution.
type RunParams struct {
	// Job is the run record created before calling Run.
	Job store.OpsRunbookRun

	// Source identifies the caller for timeline events ("runbook", "scheduler").
	Source string

	// StepTimeout is the per-step execution timeout.
	StepTimeout time.Duration

	// RunTimeout is the maximum wall-clock duration for the entire run.
	// Defaults to 5 minutes if zero.
	RunTimeout time.Duration

	// ExtraMetadata is merged into timeline event metadata on completion.
	ExtraMetadata map[string]string

	// OnFinish is called after the run is persisted with the final status.
	OnFinish func(ctx context.Context, status string)
}

const (
	runnerStatusRunning   = "running"
	runnerStatusSucceeded = "succeeded"
	runnerStatusFailed    = "failed"
)

const defaultRunTimeout = 5 * time.Minute

// Run executes a runbook run to completion. It marks the run as running,
// fetches steps, executes them with progress updates, and records the
// final result including a timeline event.
//
// The provided ctx controls cancellation — when the caller cancels (e.g.
// on server shutdown), in-flight execution is aborted. A run-level timeout
// (RunParams.RunTimeout, default 5 min) is composed on top of ctx.
func Run(ctx context.Context, repo Repo, emit EmitFunc, params RunParams) {
	runTimeout := params.RunTimeout
	if runTimeout <= 0 {
		runTimeout = defaultRunTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	job := params.Job
	now := time.Now().UTC()

	// Mark as running (best-effort).
	runningJob, err := repo.UpdateOpsRunbookRun(ctx, store.OpsRunbookRunUpdate{
		RunID:          job.ID,
		Status:         runnerStatusRunning,
		CompletedSteps: 0,
		CurrentStep:    job.CurrentStep,
		StartedAt:      now.Format(time.RFC3339),
	})
	if err != nil {
		slog.Warn("runbook runner: failed to mark run as running", "err", err)
	}
	emit("ops.job.updated", map[string]any{
		"globalRev": now.UnixMilli(),
		"job":       runningJob,
	})

	// Fetch runbook steps.
	rb, err := repo.GetOpsRunbook(ctx, job.RunbookID)
	if err != nil {
		finishRun(ctx, repo, emit, params, 0, "", err.Error(), "[]")
		return
	}
	steps := make([]Step, len(rb.Steps))
	for i, s := range rb.Steps {
		steps[i] = Step{
			Type:        s.Type,
			Title:       s.Title,
			Command:     s.Command,
			Check:       s.Check,
			Description: s.Description,
		}
	}

	stepTimeout := params.StepTimeout
	if stepTimeout <= 0 {
		stepTimeout = 30 * time.Second
	}
	executor := NewExecutor(nil, stepTimeout)
	var accumulated []store.OpsRunbookStepResult
	progress := func(completed int, stepTitle string, result StepResult) {
		sr := store.OpsRunbookStepResult{
			StepIndex:  result.StepIndex,
			Title:      result.Title,
			Type:       result.Type,
			Output:     result.Output,
			Error:      result.Error,
			DurationMs: result.Duration.Milliseconds(),
		}
		accumulated = append(accumulated, sr)
		stepResultsJSON, marshalErr := json.Marshal(accumulated)
		if marshalErr != nil {
			slog.Warn("runbook runner: failed to marshal step results", "err", marshalErr)
		}
		updated, updateErr := repo.UpdateOpsRunbookRun(ctx, store.OpsRunbookRunUpdate{
			RunID:          job.ID,
			Status:         runnerStatusRunning,
			CompletedSteps: completed,
			CurrentStep:    stepTitle,
			StepResults:    string(stepResultsJSON),
			StartedAt:      now.Format(time.RFC3339),
		})
		if updateErr != nil {
			slog.Warn("runbook runner: failed to update run progress", "err", updateErr)
		}
		emit("ops.job.updated", map[string]any{
			"globalRev": time.Now().UTC().UnixMilli(),
			"job":       updated,
		})
	}

	results, execErr := executor.Execute(ctx, steps, progress)

	errMsg := ""
	if execErr != nil {
		errMsg = execErr.Error()
	}
	lastStep := ""
	if len(results) > 0 {
		lastStep = results[len(results)-1].Title
	}
	stepResultsJSON, marshalErr := json.Marshal(accumulated)
	if marshalErr != nil {
		slog.Warn("runbook runner: failed to marshal final step results", "err", marshalErr)
	}

	finishRun(ctx, repo, emit, params, len(results), lastStep, errMsg, string(stepResultsJSON))
}

func finishRun(ctx context.Context, repo Repo, emit EmitFunc, params RunParams, completed int, lastStep, errMsg, stepResultsJSON string) {
	status := runnerStatusSucceeded
	if errMsg != "" {
		status = runnerStatusFailed
	}

	finished := time.Now().UTC()
	if _, err := repo.UpdateOpsRunbookRun(ctx, store.OpsRunbookRunUpdate{
		RunID:          params.Job.ID,
		Status:         status,
		CompletedSteps: completed,
		CurrentStep:    lastStep,
		Error:          errMsg,
		StepResults:    stepResultsJSON,
		FinishedAt:     finished.Format(time.RFC3339),
	}); err != nil {
		slog.Warn("runbook runner: failed to update finished run", "err", err)
	}

	globalRev := finished.UnixMilli()
	updatedJob, getErr := repo.GetOpsRunbookRun(ctx, params.Job.ID)
	if getErr != nil {
		slog.Warn("runbook runner: failed to get finished run", "err", getErr)
	}
	emit("ops.job.updated", map[string]any{
		"globalRev": globalRev,
		"job":       updatedJob,
	})

	severity := "info"
	if status == runnerStatusFailed {
		severity = "error"
	}

	metadata := make(map[string]string)
	metadata["jobId"] = params.Job.ID
	metadata["status"] = status
	for k, v := range params.ExtraMetadata {
		metadata[k] = v
	}
	metaJSON, metaErr := json.Marshal(metadata)
	if metaErr != nil {
		slog.Warn("runbook runner: failed to marshal timeline metadata", "err", metaErr)
	}

	te, teErr := repo.InsertTimelineEvent(ctx, timeline.EventWrite{
		Source:    params.Source,
		EventType: "runbook." + status,
		Severity:  severity,
		Resource:  params.Job.ID,
		Message:   fmt.Sprintf("Runbook run %s", status),
		Details:   errMsg,
		Metadata:  string(metaJSON),
		CreatedAt: finished,
	})
	if teErr != nil {
		slog.Warn("runbook runner: failed to insert timeline event", "err", teErr)
	}
	if te.ID > 0 {
		emit("ops.timeline.updated", map[string]any{
			"globalRev": globalRev,
			"event":     te,
		})
	}

	if params.OnFinish != nil {
		params.OnFinish(ctx, status)
	}
}
