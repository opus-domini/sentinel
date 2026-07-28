package runbook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	fastshot "github.com/opus-domini/fast-shot"

	"github.com/opus-domini/sentinel/internal/store"
)

// Repo defines the store operations consumed by the runbook runner.
type Repo interface {
	UpdateOpsRunbookRun(ctx context.Context, update store.OpsRunbookRunUpdate) (store.OpsRunbookRun, error)
	GetOpsRunbookRun(ctx context.Context, id string) (store.OpsRunbookRun, error)
}

// EmitFunc publishes a real-time event to connected clients.
type EmitFunc func(eventType string, payload map[string]any)

// RunParams configures a single runbook execution.
type RunParams struct {
	// Job is the run record created before calling Run.
	Job store.OpsRunbookRun

	// StepTimeout is the per-step execution timeout.
	StepTimeout time.Duration

	// RunTimeout is the maximum wall-clock duration for the entire run.
	// Defaults to 5 minutes if zero.
	RunTimeout time.Duration

	// CommandRunner executes run and script steps. Production callers leave it
	// nil to use the real command runner; tests provide an isolated fake.
	CommandRunner CommandRunner

	// OnFinish is called after the run is persisted with the final status.
	OnFinish func(ctx context.Context, status string)
}

const (
	keyGlobalRev = "globalRev"
	keyJob       = "job"
)

const (
	runnerStatusRunning         = "running"
	runnerStatusSucceeded       = "succeeded"
	runnerStatusFailed          = "failed"
	runnerStatusWaitingApproval = "waiting_approval"
)

const defaultRunTimeout = 5 * time.Minute

// Run executes a runbook run to completion. It marks the run as running,
// fetches steps, executes them with progress updates, and records the
// final result.
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
	definition, err := executionDefinition(job)
	if err != nil {
		finCtx, finCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer finCancel()
		finishRun(finCtx, repo, emit, params, 0, "", err.Error(), "[]", "")
		return
	}

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
		keyGlobalRev: now.UnixMilli(),
		keyJob:       runningJob,
	})

	steps := executionSteps(definition.Steps)

	stepTimeout := params.StepTimeout
	if stepTimeout <= 0 {
		stepTimeout = 30 * time.Second
	}
	executor := NewExecutor(params.CommandRunner, stepTimeout, job.ParametersUsed)
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
			keyGlobalRev: time.Now().UTC().UnixMilli(),
			keyJob:       updated,
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
			keyGlobalRev: time.Now().UTC().UnixMilli(),
			keyJob:       updated,
		})
	}

	execResult := executor.ExecuteFrom(ctx, steps, 0, beforeStep, progress)
	results := execResult.Results

	// Handle approval pause: update run to waiting_approval and return early.
	if execResult.NeedsApproval {
		stepResultsJSON, marshalErr := json.Marshal(accumulated)
		if marshalErr != nil {
			slog.Warn("runbook runner: failed to marshal step results for approval", "err", marshalErr)
		}
		lastStep := ""
		if len(results) > 0 {
			lastStep = results[len(results)-1].Title
		}
		if _, err := repo.UpdateOpsRunbookRun(ctx, store.OpsRunbookRunUpdate{
			RunID:          job.ID,
			Status:         runnerStatusWaitingApproval,
			CompletedSteps: len(results),
			CurrentStep:    lastStep,
			StepResults:    string(stepResultsJSON),
			StartedAt:      now.Format(time.RFC3339),
		}); err != nil {
			slog.Warn("runbook runner: failed to update run for approval", "err", err)
		}
		updatedJob, getErr := repo.GetOpsRunbookRun(ctx, job.ID)
		if getErr != nil {
			slog.Warn("runbook runner: failed to get run after approval pause", "err", getErr)
		}
		emit("ops.job.updated", map[string]any{
			keyGlobalRev: time.Now().UTC().UnixMilli(),
			keyJob:       updatedJob,
		})
		return
	}

	errMsg := ""
	if execErr := execResult.Err(); execErr != nil {
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
	finishRun(finCtx, repo, emit, params, len(results), lastStep, errMsg, string(stepResultsJSON), definition.WebhookURL)
}

func finishRun(ctx context.Context, repo Repo, emit EmitFunc, params RunParams, completed int, lastStep, errMsg, stepResultsJSON, webhookURL string) {
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
		keyGlobalRev: globalRev,
		keyJob:       updatedJob,
	})

	if webhookURL != "" {
		fireWebhook(ctx, webhookURL, buildWebhookPayload(params, updatedJob))
	}

	if params.OnFinish != nil {
		params.OnFinish(ctx, status)
	}
}

type webhookPayload struct {
	Event   string         `json:"event"`
	SentAt  string         `json:"sentAt"`
	Runbook webhookRunbook `json:"runbook"`
	Job     webhookJob     `json:"job"`
}

type webhookRunbook struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type webhookJob struct {
	ID             string        `json:"id"`
	Status         string        `json:"status"`
	Source         string        `json:"source"`
	TargetKind     string        `json:"targetKind,omitempty"`
	TargetName     string        `json:"targetName,omitempty"`
	TotalSteps     int           `json:"totalSteps"`
	CompletedSteps int           `json:"completedSteps"`
	Error          string        `json:"error,omitempty"`
	StartedAt      string        `json:"startedAt,omitempty"`
	FinishedAt     string        `json:"finishedAt,omitempty"`
	Steps          []webhookStep `json:"steps,omitempty"`
}

type webhookStep struct {
	Index      int    `json:"index"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"durationMs"`
}

func buildWebhookPayload(params RunParams, job store.OpsRunbookRun) webhookPayload {
	steps := make([]webhookStep, len(job.StepResults))
	for i, sr := range job.StepResults {
		steps[i] = webhookStep{
			Index:      sr.StepIndex,
			Title:      sr.Title,
			Type:       sr.Type,
			Output:     sr.Output,
			Error:      sr.Error,
			DurationMs: sr.DurationMs,
		}
	}

	runbookID := params.Job.RunbookID
	runbookName := params.Job.RunbookName
	if params.Job.Definition != nil {
		runbookID = params.Job.Definition.RunbookID
		runbookName = params.Job.Definition.Name
	}
	return webhookPayload{
		Event:  "runbook.completed",
		SentAt: time.Now().UTC().Format(time.RFC3339),
		Runbook: webhookRunbook{
			ID:   runbookID,
			Name: runbookName,
		},
		Job: webhookJob{
			ID:             job.ID,
			Status:         job.Status,
			Source:         job.Source,
			TargetKind:     job.TargetKind,
			TargetName:     job.TargetName,
			TotalSteps:     job.TotalSteps,
			CompletedSteps: job.CompletedSteps,
			Error:          job.Error,
			StartedAt:      job.StartedAt,
			FinishedAt:     job.FinishedAt,
			Steps:          steps,
		},
	}
}

func fireWebhook(ctx context.Context, webhookURL string, payload any) {
	client := fastshot.NewClient(webhookURL).
		Config().SetTimeout(10 * time.Second).
		Build()

	resp, err := client.POST("").
		Body().AsJSON(payload).
		Context().Set(ctx).
		Retry().SetExponentialBackoffWithJitter(1*time.Second, 3, 2.0).
		Retry().WithMaxDelay(5 * time.Second).
		Retry().WithRetryCondition(func(r *fastshot.Response) bool {
		return r.Status().Is5xxServerError()
	}).
		Send()
	if err != nil {
		slog.Warn("webhook delivery failed", "error", err)
		return
	}
	defer resp.Body().Close()
	if resp.Status().IsError() {
		slog.Warn("webhook delivery rejected", "status", resp.Status().Code())
		return
	}
	slog.Info("webhook delivered", "status", resp.Status().Code())
}

// ResumeRun continues a paused runbook run from the immutable receipt.
func ResumeRun(ctx context.Context, repo Repo, emit EmitFunc, params RunParams, resumeFromStep int) {
	runTimeout := params.RunTimeout
	if runTimeout <= 0 {
		runTimeout = defaultRunTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	job := params.Job
	now := time.Now().UTC()
	definition, err := executionDefinition(job)
	if err != nil {
		finCtx, finCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer finCancel()
		finishRun(finCtx, repo, emit, params, resumeFromStep+1, "", err.Error(), "", "")
		return
	}

	// Mark as running again.
	runningJob, err := repo.UpdateOpsRunbookRun(ctx, store.OpsRunbookRunUpdate{
		RunID:          job.ID,
		Status:         runnerStatusRunning,
		CompletedSteps: resumeFromStep + 1,
		CurrentStep:    job.CurrentStep,
		StartedAt:      now.Format(time.RFC3339),
	})
	if err != nil {
		slog.Warn("runbook runner: failed to mark resumed run as running", "err", err)
	}
	emit("ops.job.updated", map[string]any{
		keyGlobalRev: now.UnixMilli(),
		keyJob:       runningJob,
	})

	steps := executionSteps(definition.Steps)

	stepTimeout := params.StepTimeout
	if stepTimeout <= 0 {
		stepTimeout = 30 * time.Second
	}
	executor := NewExecutor(params.CommandRunner, stepTimeout, job.ParametersUsed)

	// Recover previous step results from the run record. If this read fails,
	// continuing would start from an empty set and overwrite the pre-approval
	// results, so fail the run instead — passing an empty step-results payload
	// keeps the existing ones (the store preserves step_results on "").
	existingRun, err := repo.GetOpsRunbookRun(ctx, job.ID)
	if err != nil {
		finCtx, finCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer finCancel()
		finishRun(finCtx, repo, emit, params, resumeFromStep+1, "", fmt.Sprintf("resume failed: %v", err), "", "")
		return
	}
	accumulated := make([]store.OpsRunbookStepResult, len(existingRun.StepResults))
	copy(accumulated, existingRun.StepResults)

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
			keyGlobalRev: time.Now().UTC().UnixMilli(),
			keyJob:       updated,
		})
	}

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
		// completed here is relative to the resumed steps; adjust for total.
		totalCompleted := resumeFromStep + 1 + completed
		updated, updateErr := repo.UpdateOpsRunbookRun(ctx, store.OpsRunbookRunUpdate{
			RunID:          job.ID,
			Status:         runnerStatusRunning,
			CompletedSteps: totalCompleted,
			CurrentStep:    stepTitle,
			StepResults:    string(stepResultsJSON),
			StartedAt:      now.Format(time.RFC3339),
		})
		if updateErr != nil {
			slog.Warn("runbook runner: failed to update run progress", "err", updateErr)
		}
		emit("ops.job.updated", map[string]any{
			keyGlobalRev: time.Now().UTC().UnixMilli(),
			keyJob:       updated,
		})
	}

	// Resume from the step after the approval step.
	execResult := executor.ExecuteFrom(ctx, steps, resumeFromStep+1, beforeStep, progress)
	results := execResult.Results

	// Handle another approval pause.
	if execResult.NeedsApproval {
		stepResultsJSON, marshalErr := json.Marshal(accumulated)
		if marshalErr != nil {
			slog.Warn("runbook runner: failed to marshal step results for approval", "err", marshalErr)
		}
		lastStep := ""
		if len(results) > 0 {
			lastStep = results[len(results)-1].Title
		}
		if _, err := repo.UpdateOpsRunbookRun(ctx, store.OpsRunbookRunUpdate{
			RunID:          job.ID,
			Status:         runnerStatusWaitingApproval,
			CompletedSteps: resumeFromStep + 1 + len(results),
			CurrentStep:    lastStep,
			StepResults:    string(stepResultsJSON),
			StartedAt:      now.Format(time.RFC3339),
		}); err != nil {
			slog.Warn("runbook runner: failed to update run for approval", "err", err)
		}
		updatedJob, getErr := repo.GetOpsRunbookRun(ctx, job.ID)
		if getErr != nil {
			slog.Warn("runbook runner: failed to get run after approval pause", "err", getErr)
		}
		emit("ops.job.updated", map[string]any{
			keyGlobalRev: time.Now().UTC().UnixMilli(),
			keyJob:       updatedJob,
		})
		return
	}

	errMsg := ""
	if execErr := execResult.Err(); execErr != nil {
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

	finCtx, finCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finCancel()
	finishRun(finCtx, repo, emit, params, resumeFromStep+1+len(results), lastStep, errMsg, string(stepResultsJSON), definition.WebhookURL)
}

func executionDefinition(job store.OpsRunbookRun) (*store.OpsRunbookExecutionSnapshot, error) {
	if job.Definition == nil {
		return nil, errors.New("execution receipt is missing")
	}
	if job.Definition.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported execution receipt schema version %d", job.Definition.SchemaVersion)
	}
	if strings.TrimSpace(job.Definition.RunbookID) == "" || job.Definition.RunbookID != job.RunbookID {
		return nil, errors.New("execution receipt does not match the runbook")
	}
	return job.Definition, nil
}

func executionSteps(source []store.OpsRunbookStep) []Step {
	steps := make([]Step, len(source))
	for i, step := range source {
		steps[i] = Step{
			Type:            step.Type,
			Title:           step.Title,
			Command:         step.Command,
			Script:          step.Script,
			Description:     step.Description,
			ContinueOnError: step.ContinueOnError,
			Timeout:         step.Timeout,
			Retries:         step.Retries,
			RetryDelay:      step.RetryDelay,
		}
	}
	return steps
}
