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
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	runbooks, err := h.store.ListOpsRunbooks(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load runbooks", nil)
		return
	}
	jobs, err := h.store.ListOpsRunbookRuns(ctx, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load runbook jobs", nil)
		return
	}
	schedules, err := h.store.ListOpsSchedules(ctx)
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
	if h.store == nil {
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
	job, err := h.store.CreateOpsRunbookRun(ctx, runbookID, now)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "OPS_RUNBOOK_NOT_FOUND", "runbook not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to run runbook", nil)
		return
	}
	timelineEvent, timelineErr := h.store.InsertOpsTimelineEvent(ctx, store.OpsTimelineEventWrite{
		Source:    "runbook",
		EventType: "runbook.started",
		Severity:  "info",
		Resource:  job.RunbookID,
		Message:   fmt.Sprintf("Runbook started: %s", job.RunbookName),
		Details:   fmt.Sprintf("job=%s steps=%d", job.ID, job.TotalSteps),
		Metadata:  marshalMetadata(map[string]string{"jobId": job.ID, "runbookId": job.RunbookID, "status": job.Status}),
		CreatedAt: now,
	})
	if timelineErr != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to persist runbook timeline", nil)
		return
	}

	// Launch async execution.
	go h.executeRunbookAsync(job)

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

func (h *Handler) executeRunbookAsync(job store.OpsRunbookRun) {
	runbook.Run(h.store, h.emitEvent, runbook.RunParams{
		Job:           job,
		Source:        "runbook",
		StepTimeout:   30 * time.Second,
		ExtraMetadata: map[string]string{"runbookId": job.RunbookID},
	})
}

func (h *Handler) emitEvent(eventType string, payload map[string]any) {
	h.emit(eventType, payload)
}

func (h *Handler) opsJob(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
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

	job, err := h.store.GetOpsRunbookRun(ctx, jobID)
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
	if h.store == nil {
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

	if err := h.store.DeleteOpsRunbookRun(ctx, jobID); err != nil {
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
	if h.store == nil {
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

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	rb, err := h.store.InsertOpsRunbook(ctx, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to create runbook", nil)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{
		"runbook": rb,
	})
}

func (h *Handler) updateOpsRunbook(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
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

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	rb, err := h.store.UpdateOpsRunbook(ctx, req)
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
	if h.store == nil {
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

	if err := h.store.DeleteOpsRunbook(ctx, runbookID); err != nil {
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
