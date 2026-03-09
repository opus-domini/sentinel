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

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/runbook"
	"github.com/opus-domini/sentinel/internal/store"
)

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
		"runbooks":  runbooks,
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
	runbookID := strings.TrimSpace(r.PathValue("runbook"))
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
		"globalRev": globalRev,
		"job":       job,
	})
	h.emit(events.TypeOpsActivity, map[string]any{
		"globalRev": globalRev,
		"event":     timelineEvent,
	})

	writeData(w, http.StatusAccepted, map[string]any{
		"job":           job,
		"timelineEvent": timelineEvent,
		"globalRev":     globalRev,
	})
}

func (h *Handler) executeRunbookAsync(ctx context.Context, job store.OpsRunbookRun, params map[string]string) {
	runbook.Run(ctx, h.repo, h.emitEvent, runbook.RunParams{
		Job:           job,
		Source:        "runbook",
		StepTimeout:   30 * time.Second,
		Parameters:    params,
		ExtraMetadata: map[string]string{"runbookId": job.RunbookID},
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
	jobID := strings.TrimSpace(r.PathValue("job"))
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
		"job": job,
	})
}

func (h *Handler) deleteOpsJob(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	jobID := strings.TrimSpace(r.PathValue("job"))
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
	writeData(w, http.StatusOK, map[string]any{"deleted": true})
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
	writeData(w, http.StatusCreated, map[string]any{
		"runbook": rb,
	})
}

func (h *Handler) updateOpsRunbook(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	runbookID := strings.TrimSpace(r.PathValue("runbook"))
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
	writeData(w, http.StatusOK, map[string]any{
		"runbook": rb,
	})
}

func (h *Handler) deleteOpsRunbook(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	runbookID := strings.TrimSpace(r.PathValue("runbook"))
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
		"removed": runbookID,
	})
}

var validStepTypes = map[string]bool{
	"command": true,
	"check":   true,
	"manual":  true,
}

func validateRunbookSteps(steps []store.OpsRunbookStep) error {
	for i, step := range steps {
		if !validStepTypes[step.Type] {
			return fmt.Errorf("step %d: type must be command, check, or manual", i)
		}
		if strings.TrimSpace(step.Title) == "" {
			return fmt.Errorf("step %d: title is required", i)
		}
	}
	return nil
}

func validateWebhookURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("webhook URL is invalid")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook URL must use http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("webhook URL must include a host")
	}
	return nil
}
