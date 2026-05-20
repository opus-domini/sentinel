package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/activity"
	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/runbook"
	"github.com/opus-domini/sentinel/internal/store"
)

func (h *Handler) suggestRunbooksForMarker(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}

	marker := strings.TrimSpace(r.URL.Query().Get("marker"))
	session := strings.TrimSpace(r.URL.Query().Get(keySession))
	if marker == "" && session == "" {
		writeData(w, http.StatusOK, map[string]any{
			keyRunbooks: []store.OpsRunbook{},
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	runbooks, err := h.repo.SuggestRunbooksForMarker(ctx, marker, session)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to suggest runbooks", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		keyRunbooks: runbooks,
	})
}

func (h *Handler) opsRunbooks(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	runbooks, err := h.repo.ListOpsRunbooks(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load runbooks", nil)
		return
	}
	jobs, err := h.repo.ListOpsRunbookRuns(ctx, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load runbook jobs", nil)
		return
	}
	schedules, err := h.repo.ListOpsSchedules(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load schedules", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		keyRunbooks: runbooks,
		"jobs":      jobs,
		"schedules": schedules,
	})
}

type runOpsRunbookRequest struct {
	Parameters map[string]string `json:"parameters"`
}

func (h *Handler) runOpsRunbook(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	runbookID := strings.TrimSpace(r.PathValue(keyRunbook))
	if runbookID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "runbook is required", nil)
		return
	}

	// Parse optional parameters from request body.
	var reqParams map[string]string
	if r.Body != nil && r.ContentLength != 0 {
		var req runOpsRunbookRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
			return
		}
		reqParams = req.Parameters
	}

	// Acquire the runbook concurrency semaphore (non-blocking).
	select {
	case h.runSem <- struct{}{}:
		// Acquired — release happens in the goroutine below.
	default:
		writeError(w, http.StatusTooManyRequests, "TOO_MANY_REQUESTS",
			"too many concurrent runbook executions", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

	// Fetch the runbook to validate parameters.
	rb, err := h.repo.GetOpsRunbook(ctx, runbookID)
	if err != nil {
		<-h.runSem // release on early return
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "OPS_RUNBOOK_NOT_FOUND", "runbook not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load runbook", nil)
		return
	}

	// Resolve and validate parameters.
	resolved := runbook.ResolveParams(rb.Parameters, reqParams)
	if err := runbook.ValidateParams(rb.Parameters, resolved); err != nil {
		<-h.runSem // release on early return
		writeError(w, http.StatusBadRequest, "INVALID_PARAMETERS", err.Error(), nil)
		return
	}

	now := time.Now().UTC()
	job, err := h.repo.CreateOpsRunbookRunWithParams(ctx, runbookID, now, resolved)
	if err != nil {
		<-h.runSem // release on early return
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "OPS_RUNBOOK_NOT_FOUND", "runbook not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to run runbook", nil)
		return
	}

	timelineEvent, timelineErr := h.orch.RecordRunbookStarted(ctx, job, now)
	if timelineErr != nil {
		<-h.runSem // release on early return
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to persist runbook timeline", nil)
		return
	}

	// Launch async execution.
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		defer func() { <-h.runSem }()
		h.executeRunbookAsync(h.runCtx, job, resolved)
	}()

	globalRev := now.UnixMilli()
	h.emit(events.TypeOpsJob, map[string]any{
		keyGlobalRev: globalRev,
		keyJob:       job,
	})
	h.emit(events.TypeOpsActivity, map[string]any{
		keyGlobalRev: globalRev,
		keyEvent:     timelineEvent,
	})

	writeData(w, http.StatusAccepted, map[string]any{
		keyJob:          job,
		"timelineEvent": timelineEvent,
		keyGlobalRev:    globalRev,
	})
}

func (h *Handler) executeRunbookAsync(ctx context.Context, job store.OpsRunbookRun, params map[string]string) {
	runbook.Run(ctx, h.repo, h.emitEvent, runbook.RunParams{
		Job:           job,
		Source:        activity.SourceRunbook,
		StepTimeout:   30 * time.Second,
		Parameters:    params,
		ExtraMetadata: map[string]string{keyRunbookID: job.RunbookID},
		AlertRepo:     h.repo,
	})
}

func (h *Handler) emitEvent(eventType string, payload map[string]any) {
	h.emit(eventType, payload)
}

func (h *Handler) opsJob(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	jobID := strings.TrimSpace(r.PathValue(keyJob))
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "job id is required", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	job, err := h.repo.GetOpsRunbookRun(ctx, jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "OPS_JOB_NOT_FOUND", "job not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load job", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		keyJob: job,
	})
}

func (h *Handler) deleteOpsJob(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	jobID := strings.TrimSpace(r.PathValue(keyJob))
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "job id is required", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.repo.DeleteOpsRunbookRun(ctx, jobID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "OPS_JOB_NOT_FOUND", "job not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to delete job", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{keyDeleted: true})
}

func (h *Handler) createOpsRunbook(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	var req store.OpsRunbookWrite
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "runbook name is required", nil)
		return
	}
	if err := validateRunbookSteps(req.Steps); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if err := validateWebhookURL(req.WebhookURL); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	rb, err := h.repo.InsertOpsRunbook(ctx, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to create runbook", nil)
		return
	}

	result := map[string]any{
		keyRunbook: rb,
	}
	if warnings := validateShellSyntax(req.Steps); len(warnings) > 0 {
		result["shellWarnings"] = warnings
	}
	writeData(w, http.StatusCreated, result)
}

func (h *Handler) updateOpsRunbook(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	runbookID := strings.TrimSpace(r.PathValue(keyRunbook))
	if runbookID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "runbook id is required", nil)
		return
	}
	var req store.OpsRunbookWrite
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.ID = runbookID
	if err := validateRunbookSteps(req.Steps); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if err := validateWebhookURL(req.WebhookURL); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	rb, err := h.repo.UpdateOpsRunbook(ctx, req)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "OPS_RUNBOOK_NOT_FOUND", "runbook not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to update runbook", nil)
		return
	}

	result := map[string]any{
		keyRunbook: rb,
	}
	if warnings := validateShellSyntax(req.Steps); len(warnings) > 0 {
		result["shellWarnings"] = warnings
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) deleteOpsRunbook(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	runbookID := strings.TrimSpace(r.PathValue(keyRunbook))
	if runbookID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "runbook id is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	// Remove associated schedules before deleting the runbook to avoid
	// orphan schedules that would cause the scheduler to loop on errors.
	if err := h.repo.DeleteSchedulesByRunbook(ctx, runbookID); err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to remove associated schedules", nil)
		return
	}

	if err := h.repo.DeleteOpsRunbook(ctx, runbookID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "OPS_RUNBOOK_NOT_FOUND", "runbook not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to delete runbook", nil)
		return
	}

	writeData(w, http.StatusOK, map[string]any{
		keyRemoved: runbookID,
	})
}

var validStepTypes = map[string]bool{
	keyRun:           true,
	keyScript:        true,
	stepTypeApproval: true,
}

func validateRunbookSteps(steps []store.OpsRunbookStep) error {
	for i, step := range steps {
		if !validStepTypes[step.Type] {
			return fmt.Errorf("step %d: type must be run, script, or approval", i)
		}
		if strings.TrimSpace(step.Title) == "" {
			return fmt.Errorf("step %d: title is required", i)
		}
	}
	return nil
}

// validateShellSyntax runs shell syntax validation on all run/script steps
// and returns warnings (not blocking errors).
func validateShellSyntax(steps []store.OpsRunbookStep) []runbook.ShellWarning {
	var inputs []runbook.ShellCheckInput
	for i, s := range steps {
		switch s.Type {
		case keyRun:
			if s.Command != "" {
				inputs = append(inputs, runbook.ShellCheckInput{Step: i, Type: keyRun, Source: s.Command})
			}
		case keyScript:
			if s.Script != "" {
				inputs = append(inputs, runbook.ShellCheckInput{Step: i, Type: keyScript, Source: s.Script})
			}
		}
	}
	return runbook.ValidateShellSyntaxFromStrings(inputs)
}

func validateWebhookURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("webhook URL is invalid")
	}
	if u.Scheme != "http" && u.Scheme != "https" { //nolint:goconst // inline URL scheme check
		return fmt.Errorf("webhook URL must use http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("webhook URL must include a host")
	}
	return nil
}

func (h *Handler) approveOpsRunbookRun(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	runID := strings.TrimSpace(r.PathValue("runId"))
	if runID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "run id is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

	job, err := h.repo.GetOpsRunbookRun(ctx, runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "OPS_JOB_NOT_FOUND", "run not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load run", nil)
		return
	}

	if job.Status != store.OpsRunbookStatusWaitingApproval {
		writeError(w, http.StatusConflict, "INVALID_STATE", fmt.Sprintf("run status is %q, not waiting_approval", job.Status), nil)
		return
	}

	// Find the approval step index: it's the last approval step result.
	approvalStepIndex := -1
	for _, sr := range job.StepResults {
		if sr.Type == stepTypeApproval {
			approvalStepIndex = sr.StepIndex
		}
	}
	if approvalStepIndex < 0 {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "could not find approval step in results", nil)
		return
	}

	// Acquire the runbook concurrency semaphore (non-blocking).
	select {
	case h.runSem <- struct{}{}:
	default:
		writeError(w, http.StatusTooManyRequests, "TOO_MANY_REQUESTS",
			"too many concurrent runbook executions", nil)
		return
	}

	now := time.Now().UTC()
	runningJob, err := h.repo.UpdateOpsRunbookRun(ctx, store.OpsRunbookRunUpdate{
		RunID:          job.ID,
		Status:         stateRunning,
		CompletedSteps: approvalStepIndex + 1,
		CurrentStep:    job.CurrentStep,
		StartedAt:      now.Format(time.RFC3339),
	})
	if err != nil {
		<-h.runSem
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to resume run", nil)
		return
	}
	globalRev := now.UnixMilli()
	h.emit(events.TypeOpsJob, map[string]any{
		keyGlobalRev: globalRev,
		keyJob:       runningJob,
	})

	// Resolve parameters from the run record.
	resolved := job.ParametersUsed

	// Launch async execution from the step after approval.
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		defer func() { <-h.runSem }()
		runbook.ResumeRun(h.runCtx, h.repo, h.emitEvent, runbook.RunParams{
			Job:           runningJob,
			Source:        activity.SourceRunbook,
			StepTimeout:   30 * time.Second,
			Parameters:    resolved,
			ExtraMetadata: map[string]string{keyRunbookID: job.RunbookID},
			AlertRepo:     h.repo,
		}, approvalStepIndex)
	}()

	writeData(w, http.StatusAccepted, map[string]any{
		keyJob:       runningJob,
		keyGlobalRev: globalRev,
	})
}

func (h *Handler) rejectOpsRunbookRun(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	runID := strings.TrimSpace(r.PathValue("runId"))
	if runID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "run id is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	job, err := h.repo.GetOpsRunbookRun(ctx, runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "OPS_JOB_NOT_FOUND", "run not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load run", nil)
		return
	}

	if job.Status != store.OpsRunbookStatusWaitingApproval {
		writeError(w, http.StatusConflict, "INVALID_STATE", fmt.Sprintf("run status is %q, not waiting_approval", job.Status), nil)
		return
	}

	now := time.Now().UTC()
	updated, err := h.repo.UpdateOpsRunbookRun(ctx, store.OpsRunbookRunUpdate{
		RunID:          runID,
		Status:         stateFailed,
		CompletedSteps: job.CompletedSteps,
		CurrentStep:    job.CurrentStep,
		Error:          "approval rejected",
		FinishedAt:     now.Format(time.RFC3339),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to update run", nil)
		return
	}

	globalRev := now.UnixMilli()
	h.emit(events.TypeOpsJob, map[string]any{
		keyGlobalRev: globalRev,
		keyJob:       updated,
	})

	writeData(w, http.StatusOK, map[string]any{
		keyJob:       updated,
		keyGlobalRev: globalRev,
	})
}
