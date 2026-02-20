package runbook

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/opus-domini/sentinel/internal/activity"
	"github.com/opus-domini/sentinel/internal/alerts"
	"github.com/opus-domini/sentinel/internal/store"
)

// Repo defines the store operations consumed by the runbook runner.
type Repo interface {
	UpdateOpsRunbookRun(ctx context.Context, update store.OpsRunbookRunUpdate) (store.OpsRunbookRun, error)
	GetOpsRunbook(ctx context.Context, id string) (store.OpsRunbook, error)
	GetOpsRunbookRun(ctx context.Context, id string) (store.OpsRunbookRun, error)
	InsertActivityEvent(ctx context.Context, event activity.EventWrite) (activity.Event, error)
}

// EmitFunc publishes a real-time event to connected clients.
type EmitFunc func(eventType string, payload map[string]any)

// AlertRepo is an optional interface for raising/resolving alerts on run completion.
type AlertRepo interface {
	UpsertAlert(ctx context.Context, write alerts.AlertWrite) (alerts.Alert, error)
	ResolveAlert(ctx context.Context, dedupeKey string, at time.Time) (alerts.Alert, error)
}

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

	// AlertRepo is an optional alert repository. When non-nil, failed runs
	// raise alerts and successful runs resolve them.
	AlertRepo AlertRepo
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
		finCtx, finCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second) //nolint:govet // finCancel is deferred
		defer finCancel()
		finishRun(finCtx, repo, emit, params, 0, "", err.Error(), "[]")
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

	// beforeStep writes a preliminary step result to the DB before execution.
	// If the server dies mid-step, this entry already exists with the correct
	// step title so FailOrphanedRuns does not need to reconstruct it.
	beforeStep := func(stepIndex int, step Step) {
		accumulated = append(accumulated, store.OpsRunbookStepResult{
			StepIndex: stepIndex,
			Title:     step.Title,
			Type:      step.Type,
		})
		stepResultsJSON, marshalErr := json.Marshal(accumulated)
		if marshalErr != nil {
			slog.Warn("runbook runner: failed to marshal step results", "err", marshalErr)
		}
		updated, updateErr := repo.UpdateOpsRunbookRun(ctx, store.OpsRunbookRunUpdate{
			RunID:          job.ID,
			Status:         runnerStatusRunning,
			CompletedSteps: stepIndex,
			CurrentStep:    step.Title,
			StepResults:    string(stepResultsJSON),
			StartedAt:      now.Format(time.RFC3339),
		})
		if updateErr != nil {
			slog.Warn("runbook runner: failed to update run before step", "err", updateErr)
		}
		emit("ops.job.updated", map[string]any{
			"globalRev": time.Now().UTC().UnixMilli(),
			"job":       updated,
		})
	}

	// progress updates the last step result entry with actual output/error/duration.
	progress := func(completed int, stepTitle string, result StepResult) {
		last := len(accumulated) - 1
		accumulated[last] = store.OpsRunbookStepResult{
			StepIndex:  result.StepIndex,
			Title:      result.Title,
			Type:       result.Type,
			Output:     result.Output,
			Error:      result.Error,
			DurationMs: result.Duration.Milliseconds(),
		}
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

	results, execErr := executor.Execute(ctx, steps, beforeStep, progress)

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

	// Use a context detached from cancellation for terminal writes so that
	// finishRun succeeds even when the execution context has been cancelled
	// (timeout, server shutdown). context.WithoutCancel preserves Values
	// (trace IDs) while shedding the done channel.
	finCtx, finCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finCancel()
	finishRun(finCtx, repo, emit, params, len(results), lastStep, errMsg, string(stepResultsJSON))
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

	te, teErr := repo.InsertActivityEvent(ctx, activity.EventWrite{
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
		emit("ops.activity.updated", map[string]any{
			"globalRev": globalRev,
			"event":     te,
		})
	}

	if params.AlertRepo != nil {
		dedupeKey := fmt.Sprintf("runbook:%s:failed", params.Job.RunbookID)
		switch status {
		case runnerStatusFailed:
			if _, alertErr := params.AlertRepo.UpsertAlert(ctx, alerts.AlertWrite{
				DedupeKey: dedupeKey,
				Source:    "runbook",
				Resource:  params.Job.RunbookName,
				Title:     fmt.Sprintf("Runbook %s failed", params.Job.RunbookName),
				Message:   errMsg,
				Severity:  "error",
				CreatedAt: finished,
			}); alertErr != nil {
				slog.Warn("runbook runner: failed to upsert alert", "err", alertErr)
			}
		case runnerStatusSucceeded:
			if _, alertErr := params.AlertRepo.ResolveAlert(ctx, dedupeKey, finished); alertErr != nil {
				// sql.ErrNoRows is expected when no prior alert exists.
				if !errors.Is(alertErr, sql.ErrNoRows) {
					slog.Warn("runbook runner: failed to resolve alert", "err", alertErr)
				}
			}
		}
	}

	if params.OnFinish != nil {
		params.OnFinish(ctx, status)
	}
}
