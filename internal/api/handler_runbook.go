package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
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

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

	now := time.Now().UTC()
	job, err := h.repo.CreateOpsRunbookRun(ctx, runbookID, now)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "OPS_RUNBOOK_NOT_FOUND", "runbook not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to run runbook", nil)
		return
	}

	timelineEvent, timelineErr := h.orch.RecordRunbookStarted(ctx, job, now)
	if timelineErr != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to persist runbook timeline", nil)
		return
	}

	// Launch async execution.
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		h.executeRunbookAsync(h.runCtx, job)
	}()

	globalRev := now.UnixMilli()
	h.emit(events.TypeOpsJob, map[string]any{
		"globalRev": globalRev,
		"job":       job,
	})
	h.emit(events.TypeOpsTimeline, map[string]any{
		"globalRev": globalRev,
		"event":     timelineEvent,
	})

	writeData(w, http.StatusAccepted, map[string]any{
		"job":           job,
		"timelineEvent": timelineEvent,
		"globalRev":     globalRev,
	})
}

func (h *Handler) executeRunbookAsync(ctx context.Context, job store.OpsRunbookRun) {
	runbook.Run(ctx, h.repo, h.emitEvent, runbook.RunParams{
		Job:           job,
		Source:        "runbook",
		StepTimeout:   30 * time.Second,
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
