package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/runbook"
	"github.com/opus-domini/sentinel/internal/store"
)

func (h *Handler) opsRunbooks(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil || h.runbooks == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	runbooks, err := h.runbooks.List(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load runbooks", nil)
		return
	}
	jobs, err := h.runbooks.ListRuns(ctx, 20)
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
	if h.repo == nil || h.runbooks == nil {
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

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	job, err := h.runbooks.Start(ctx, runbookID, reqParams, "runbook")
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeError(w, http.StatusNotFound, "OPS_RUNBOOK_NOT_FOUND", "runbook not found", nil)
		case errors.Is(err, runbook.ErrTooManyExecutions):
			writeError(w, http.StatusTooManyRequests, "TOO_MANY_REQUESTS", err.Error(), nil)
		case errors.Is(err, runbook.ErrInvalidParameters):
			writeError(w, http.StatusBadRequest, "INVALID_PARAMETERS", err.Error(), nil)
		default:
			writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to run runbook", nil)
		}
		return
	}

	writeData(w, http.StatusAccepted, map[string]any{
		keyJob:       job,
		keyGlobalRev: time.Now().UTC().UnixMilli(),
	})
}

func (h *Handler) emitEvent(eventType string, payload map[string]any) {
	h.emit(eventType, payload)
}

func (h *Handler) opsJob(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil || h.runbooks == nil {
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

	job, err := h.runbooks.GetRun(ctx, jobID)
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
	if h.repo == nil || h.runbooks == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	var req store.OpsRunbookWrite
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	rb, warnings, err := h.runbooks.Create(ctx, req)
	if err != nil {
		if errors.Is(err, runbook.ErrInvalidDefinition) {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to create runbook", nil)
		return
	}

	result := map[string]any{
		keyRunbook: rb,
	}
	if len(warnings) > 0 {
		result["shellWarnings"] = warnings
	}
	writeData(w, http.StatusCreated, result)
}

func (h *Handler) updateOpsRunbook(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil || h.runbooks == nil {
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
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	rb, warnings, err := h.runbooks.Update(ctx, req)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeError(w, http.StatusNotFound, "OPS_RUNBOOK_NOT_FOUND", "runbook not found", nil)
		case errors.Is(err, runbook.ErrInvalidDefinition):
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		default:
			writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to update runbook", nil)
		}
		return
	}

	result := map[string]any{
		keyRunbook: rb,
	}
	if len(warnings) > 0 {
		result["shellWarnings"] = warnings
	}
	writeData(w, http.StatusOK, result)
}

func (h *Handler) deleteOpsRunbook(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil || h.runbooks == nil {
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

	deleted, err := h.runbooks.Delete(ctx, runbookID, "")
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeError(w, http.StatusNotFound, "OPS_RUNBOOK_NOT_FOUND", "runbook not found", nil)
		case errors.Is(err, store.ErrOpsRunbookActive):
			writeError(w, http.StatusConflict, "RUNBOOK_ACTIVE", err.Error(), nil)
		default:
			writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to delete runbook", nil)
		}
		return
	}

	writeData(w, http.StatusOK, map[string]any{
		keyRemoved:         deleted.ID,
		"deletedSchedules": deleted.DeletedSchedules,
	})
}

func (h *Handler) approveOpsRunbookRun(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil || h.runbooks == nil {
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

	job, err := h.runbooks.Approve(ctx, runID, "runbook")
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeError(w, http.StatusNotFound, "OPS_JOB_NOT_FOUND", "run not found", nil)
		case errors.Is(err, runbook.ErrInvalidRunState):
			writeError(w, http.StatusConflict, "INVALID_STATE", err.Error(), nil)
		case errors.Is(err, runbook.ErrTooManyExecutions):
			writeError(w, http.StatusTooManyRequests, "TOO_MANY_REQUESTS", err.Error(), nil)
		default:
			writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to resume run", nil)
		}
		return
	}

	writeData(w, http.StatusAccepted, map[string]any{
		keyJob:       job,
		keyGlobalRev: time.Now().UTC().UnixMilli(),
	})
}

func (h *Handler) rejectOpsRunbookRun(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil || h.runbooks == nil {
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

	job, err := h.runbooks.Reject(ctx, runID)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeError(w, http.StatusNotFound, "OPS_JOB_NOT_FOUND", "run not found", nil)
		case errors.Is(err, runbook.ErrInvalidRunState):
			writeError(w, http.StatusConflict, "INVALID_STATE", err.Error(), nil)
		default:
			writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to reject run", nil)
		}
		return
	}

	writeData(w, http.StatusOK, map[string]any{
		keyJob:       job,
		keyGlobalRev: time.Now().UTC().UnixMilli(),
	})
}
