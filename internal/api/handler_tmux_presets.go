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
	"github.com/opus-domini/sentinel/internal/tmux"
	"github.com/opus-domini/sentinel/internal/validate"
)

const defaultSessionPresetIcon = "terminal"

func (h *Handler) listSessionPresets(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	presets, err := h.repo.ListSessionPresets(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to list session presets", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"presets": presets})
}

func (h *Handler) createSessionPreset(w http.ResponseWriter, r *http.Request) {
	row, err := decodeSessionPresetWrite(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	preset, err := h.repo.CreateSessionPreset(ctx, row)
	if err != nil {
		if isUniqueConstraintError(err) {
			writeError(w, http.StatusConflict, "SESSION_PRESET_EXISTS", "session preset already exists", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to create session preset", nil)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"preset": preset})
}

func (h *Handler) updateSessionPreset(w http.ResponseWriter, r *http.Request) {
	presetName := strings.TrimSpace(r.PathValue("preset"))
	if !validate.SessionName(presetName) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session preset name", nil)
		return
	}

	row, err := decodeSessionPresetWrite(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	preset, err := h.repo.UpdateSessionPreset(ctx, presetName, row)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeError(w, http.StatusNotFound, "SESSION_PRESET_NOT_FOUND", "session preset not found", nil)
		case isUniqueConstraintError(err):
			writeError(w, http.StatusConflict, "SESSION_PRESET_EXISTS", "session preset already exists", nil)
		default:
			writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to update session preset", nil)
		}
		return
	}
	writeData(w, http.StatusOK, map[string]any{"preset": preset})
}

func (h *Handler) deleteSessionPreset(w http.ResponseWriter, r *http.Request) {
	presetName := strings.TrimSpace(r.PathValue("preset"))
	if !validate.SessionName(presetName) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session preset name", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	err := h.repo.DeleteSessionPreset(ctx, presetName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "SESSION_PRESET_NOT_FOUND", "session preset not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to delete session preset", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) launchSessionPreset(w http.ResponseWriter, r *http.Request) {
	presetName := strings.TrimSpace(r.PathValue("preset"))
	if !validate.SessionName(presetName) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session preset name", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	preset, err := h.findSessionPreset(ctx, presetName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "SESSION_PRESET_NOT_FOUND", "session preset not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load session preset", nil)
		return
	}

	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "session.create",
		SessionName: preset.Name,
		WindowIndex: -1,
	}); !ok {
		return
	}

	created := true
	if err := h.tmux.CreateSession(ctx, preset.Name, preset.Cwd); err != nil {
		if tmux.IsKind(err, tmux.ErrKindSessionExists) {
			created = false
		} else {
			writeTmuxError(w, err)
			return
		}
	}

	h.persistSessionLaunchMetadataBestEffort(ctx, preset.Name, preset.Cwd, preset.Icon)
	if err := h.repo.MarkSessionPresetLaunched(ctx, preset.Name); err != nil {
		slog.Warn("failed to mark session preset launched", "preset", preset.Name, "err", err)
	}
	h.emit(events.TypeTmuxSessions, map[string]any{
		"session": preset.Name,
		"action":  "launch",
	})
	writeData(w, http.StatusOK, map[string]any{
		"name":    preset.Name,
		"created": created,
	})
}

func (h *Handler) findSessionPreset(ctx context.Context, name string) (store.SessionPreset, error) {
	presets, err := h.repo.ListSessionPresets(ctx)
	if err != nil {
		return store.SessionPreset{}, err
	}
	for _, preset := range presets {
		if preset.Name == name {
			return preset, nil
		}
	}
	return store.SessionPreset{}, sql.ErrNoRows
}

func (h *Handler) renameSessionPresetBestEffort(ctx context.Context, oldName, newName string) {
	if h.repo == nil || strings.TrimSpace(oldName) == strings.TrimSpace(newName) {
		return
	}

	preset, err := h.findSessionPreset(ctx, oldName)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Warn("failed to load session preset during rename", "preset", oldName, "err", err)
		}
		return
	}

	if _, err := h.repo.UpdateSessionPreset(ctx, oldName, store.SessionPresetWrite{
		Name: newName,
		Cwd:  preset.Cwd,
		Icon: preset.Icon,
	}); err != nil {
		slog.Warn("failed to rename session preset", "from", oldName, "to", newName, "err", err)
	}
}

func (h *Handler) persistSessionLaunchMetadataBestEffort(ctx context.Context, sessionName, cwd, icon string) {
	if h.repo == nil {
		return
	}
	if cwd != "" {
		if err := h.repo.RecordSessionDirectory(ctx, cwd); err != nil {
			slog.Warn("failed to record session directory", "cwd", cwd, "err", err)
		}
	}
	if icon != "" {
		if err := h.repo.SetIcon(ctx, sessionName, icon); err != nil {
			slog.Warn("failed to persist session icon", "session", sessionName, "icon", icon, "err", err)
		}
	}
}

func decodeSessionPresetWrite(r *http.Request) (store.SessionPresetWrite, error) {
	var req struct {
		Name string `json:"name"`
		Cwd  string `json:"cwd"`
		Icon string `json:"icon"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return store.SessionPresetWrite{}, err
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Cwd = strings.TrimSpace(req.Cwd)
	if req.Cwd == "" {
		req.Cwd = defaultSessionCWD()
	}
	req.Icon = strings.TrimSpace(req.Icon)
	if req.Icon == "" {
		req.Icon = defaultSessionPresetIcon
	}

	switch {
	case !validate.SessionName(req.Name):
		return store.SessionPresetWrite{}, errors.New("name must match ^[A-Za-z0-9._-]{1,64}$")
	case req.Cwd == "":
		return store.SessionPresetWrite{}, errors.New("cwd is required")
	case !filepath.IsAbs(req.Cwd):
		return store.SessionPresetWrite{}, errors.New("cwd must be an absolute path")
	case !validate.IconKey(req.Icon):
		return store.SessionPresetWrite{}, errors.New("icon must match ^[a-z0-9-]{1,32}$")
	}

	return store.SessionPresetWrite{
		Name: req.Name,
		Cwd:  req.Cwd,
		Icon: req.Icon,
	}, nil
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed")
}
