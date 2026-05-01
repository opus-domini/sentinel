package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/guardrails"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/validate"
)

func (h *Handler) listSessionLaunchers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	launchers, err := h.repo.ListSessionLaunchers(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to list session launchers", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"launchers": launchers})
}

func (h *Handler) createSessionLauncher(w http.ResponseWriter, r *http.Request) {
	row, err := decodeSessionLauncherWrite(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if err := h.guard.ValidateTargetUser(row.User); err != nil {
		writeError(w, http.StatusForbidden, "USER_NOT_ALLOWED", err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	launcher, err := h.repo.CreateSessionLauncher(ctx, row)
	if err != nil {
		if isUniqueConstraintError(err) {
			writeError(w, http.StatusConflict, "SESSION_LAUNCHER_EXISTS", "session launcher already exists", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to create session launcher", nil)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"launcher": launcher})
}

func (h *Handler) updateSessionLauncher(w http.ResponseWriter, r *http.Request) {
	launcherID := strings.TrimSpace(r.PathValue("launcher"))
	if launcherID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "session launcher id is required", nil)
		return
	}

	row, err := decodeSessionLauncherWrite(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if err := h.guard.ValidateTargetUser(row.User); err != nil {
		writeError(w, http.StatusForbidden, "USER_NOT_ALLOWED", err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	launcher, err := h.repo.UpdateSessionLauncher(ctx, launcherID, row)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeError(w, http.StatusNotFound, "SESSION_LAUNCHER_NOT_FOUND", "session launcher not found", nil)
		case isUniqueConstraintError(err):
			writeError(w, http.StatusConflict, "SESSION_LAUNCHER_EXISTS", "session launcher already exists", nil)
		default:
			writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to update session launcher", nil)
		}
		return
	}
	writeData(w, http.StatusOK, map[string]any{"launcher": launcher})
}

func (h *Handler) deleteSessionLauncher(w http.ResponseWriter, r *http.Request) {
	launcherID := strings.TrimSpace(r.PathValue("launcher"))
	if launcherID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "session launcher id is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	err := h.repo.DeleteSessionLauncher(ctx, launcherID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "SESSION_LAUNCHER_NOT_FOUND", "session launcher not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to delete session launcher", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) reorderSessionLaunchers(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if len(req.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "ids is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.repo.ReorderSessionLaunchers(ctx, req.IDs); err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to reorder session launchers", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) launchSessionLauncher(w http.ResponseWriter, r *http.Request) {
	launcherID := strings.TrimSpace(r.PathValue("launcher"))
	if launcherID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "session launcher id is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	launcher, err := h.repo.GetSessionLauncher(ctx, launcherID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "SESSION_LAUNCHER_NOT_FOUND", "session launcher not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load session launcher", nil)
		return
	}
	if err := h.guard.ValidateTargetUser(launcher.User); err != nil {
		writeError(w, http.StatusForbidden, "USER_NOT_ALLOWED", err.Error(), nil)
		return
	}

	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "session.create",
		SessionName: launcher.Name,
		WindowIndex: -1,
	}); !ok {
		return
	}

	tmuxSvc := h.tmuxForUser(launcher.User)
	sessionName, err := createSessionWithAvailableName(ctx, tmuxSvc, launcher.Name, launcher.Cwd)
	if err != nil {
		writeTmuxError(w, err)
		return
	}
	h.registerSessionUser(sessionName, launcher.User)
	if launcher.User != "" {
		slog.Warn("multi-user session created",
			"action", "session.launcher.launch",
			"target_user", launcher.User,
			"session", sessionName,
			"launcher", launcher.ID,
			"source_ip", r.RemoteAddr,
		)
	}

	h.persistSessionLaunchMetadataBestEffort(ctx, sessionName, launcher.Cwd, launcher.Icon)
	if err := h.repo.MarkSessionLauncherUsed(ctx, launcher.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to record session launcher usage", nil)
		return
	}
	if err := h.repo.MoveSessionToFront(ctx, sessionName); err != nil {
		slog.Warn("failed to move session to front", "session", sessionName, "err", err)
	}
	h.emit(events.TypeTmuxSessions, map[string]any{
		"session":  sessionName,
		"launcher": launcher.ID,
		"action":   "create",
	})
	writeData(w, http.StatusOK, map[string]any{
		"launcherId": launcher.ID,
		"name":       sessionName,
		"created":    true,
	})
}

func decodeSessionLauncherWrite(r *http.Request) (store.SessionLauncherWrite, error) {
	var req struct {
		Name string `json:"name"`
		Cwd  string `json:"cwd"`
		Icon string `json:"icon"`
		User string `json:"user"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return store.SessionLauncherWrite{}, err
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Cwd = strings.TrimSpace(req.Cwd)
	req.Icon = strings.TrimSpace(req.Icon)
	req.User = strings.TrimSpace(req.User)
	if req.Cwd == "" {
		req.Cwd = defaultSessionCWD()
	}
	if req.Icon == "" {
		req.Icon = defaultSessionPresetIcon
	}
	if !validate.SessionName(req.Name) {
		return store.SessionLauncherWrite{}, errors.New("name must match ^[A-Za-z0-9._-]{1,64}$")
	}
	if req.Cwd == "" {
		return store.SessionLauncherWrite{}, errors.New("cwd is required")
	}
	if !filepath.IsAbs(req.Cwd) {
		return store.SessionLauncherWrite{}, errors.New("cwd must be an absolute path")
	}
	if !validate.IconKey(req.Icon) {
		return store.SessionLauncherWrite{}, errors.New("icon must match ^[a-z0-9-]{1,32}$")
	}

	return store.SessionLauncherWrite{
		Name: req.Name,
		Cwd:  req.Cwd,
		Icon: req.Icon,
		User: req.User,
	}, nil
}
