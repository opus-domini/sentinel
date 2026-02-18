package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/recovery"
	"github.com/opus-domini/sentinel/internal/validate"
)

func (h *Handler) requireRecovery(w http.ResponseWriter) bool {
	if h.recovery != nil {
		return true
	}
	writeError(w, http.StatusServiceUnavailable, "RECOVERY_DISABLED", "recovery subsystem is disabled", nil)
	return false
}

func (h *Handler) recoveryOverview(w http.ResponseWriter, r *http.Request) {
	if !h.requireRecovery(w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	overview, err := h.recovery.Overview(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RECOVERY_ERROR", "failed to load recovery overview", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"overview": overview,
	})
}

func (h *Handler) listRecoverySessions(w http.ResponseWriter, r *http.Request) {
	if !h.requireRecovery(w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	sessions, err := h.recovery.ListKilledSessions(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RECOVERY_ERROR", "failed to list recovery sessions", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"sessions": sessions,
	})
}

func (h *Handler) archiveRecoverySession(w http.ResponseWriter, r *http.Request) {
	if !h.requireRecovery(w) {
		return
	}
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.recovery.ArchiveSession(ctx, session); err != nil {
		writeError(w, http.StatusInternalServerError, "RECOVERY_ERROR", "failed to archive recovery session", nil)
		return
	}
	h.emit(events.TypeRecoveryOverview, map[string]any{
		"session": session,
		"action":  "archive",
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listRecoverySnapshots(w http.ResponseWriter, r *http.Request) {
	if !h.requireRecovery(w) {
		return
	}
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	snapshots, err := h.recovery.ListSnapshots(ctx, session, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RECOVERY_ERROR", "failed to list snapshots", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"snapshots": snapshots,
	})
}

func (h *Handler) getRecoverySnapshot(w http.ResponseWriter, r *http.Request) {
	if !h.requireRecovery(w) {
		return
	}
	rawID := strings.TrimSpace(r.PathValue("snapshot"))
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "snapshot must be a positive integer", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	view, err := h.recovery.GetSnapshot(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "SNAPSHOT_NOT_FOUND", "snapshot not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "RECOVERY_ERROR", "failed to load snapshot", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"snapshot": view,
	})
}

func (h *Handler) restoreRecoverySnapshot(w http.ResponseWriter, r *http.Request) {
	if !h.requireRecovery(w) {
		return
	}
	rawID := strings.TrimSpace(r.PathValue("snapshot"))
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "snapshot must be a positive integer", nil)
		return
	}

	var req struct {
		Mode           recovery.ReplayMode     `json:"mode"`
		ConflictPolicy recovery.ConflictPolicy `json:"conflictPolicy"`
		TargetSession  string                  `json:"targetSession"`
	}
	if err := decodeOptionalJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.TargetSession = strings.TrimSpace(req.TargetSession)
	if req.TargetSession != "" && !validate.SessionName(req.TargetSession) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "targetSession must match ^[A-Za-z0-9._-]{1,64}$", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	job, err := h.recovery.RestoreSnapshotAsync(ctx, id, recovery.RestoreOptions{
		Mode:           req.Mode,
		ConflictPolicy: req.ConflictPolicy,
		TargetSession:  req.TargetSession,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "SNAPSHOT_NOT_FOUND", "snapshot not found", nil)
			return
		}
		slog.Warn("recovery restore failed", "snapshot", id, "err", err)
		writeError(w, http.StatusInternalServerError, "RECOVERY_RESTORE_FAILED", "failed to restore snapshot", nil)
		return
	}
	h.emit(events.TypeRecoveryJob, map[string]any{
		"jobId":  job.ID,
		"status": string(job.Status),
	})
	h.emit(events.TypeRecoveryOverview, map[string]any{
		"session": job.SessionName,
		"action":  "restore-started",
	})
	writeData(w, http.StatusAccepted, map[string]any{
		"job": job,
	})
}

func (h *Handler) getRecoveryJob(w http.ResponseWriter, r *http.Request) {
	if !h.requireRecovery(w) {
		return
	}
	jobID := strings.TrimSpace(r.PathValue("job"))
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "job id is required", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	job, err := h.recovery.GetJob(ctx, jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "recovery job not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "RECOVERY_ERROR", "failed to load recovery job", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"job": job,
	})
}
