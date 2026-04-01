package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/tmux"
	"github.com/opus-domini/sentinel/internal/validate"
)

func (h *Handler) reorderSessions(w http.ResponseWriter, r *http.Request) {
	names, err := decodeSessionOrderNames(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.repo.ReorderSessions(ctx, names); err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to reorder sessions", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) reorderSessionPresets(w http.ResponseWriter, r *http.Request) {
	names, err := decodeSessionOrderNames(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.repo.ReorderSessionPresets(ctx, names); err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to reorder session presets", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) reorderWindows(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	windowIDs, err := decodeWindowOrderIDs(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	liveWindows, err := h.tmux.ListWindows(ctx, session)
	if err != nil {
		writeTmuxError(w, err)
		return
	}
	if !windowOrderMatchesLive(liveWindows, windowIDs) {
		writeError(w, http.StatusConflict, "WINDOW_ORDER_STALE", "window order is stale; refresh and retry", nil)
		return
	}

	activeWindowID := activeWindowRuntimeID(liveWindows)
	if err := h.tmux.ReorderWindows(ctx, session, windowIDs); err != nil {
		switch {
		case tmux.IsKind(err, tmux.ErrKindInvalidIdentifier) && isWindowOrderStaleError(err):
			writeError(w, http.StatusConflict, "WINDOW_ORDER_STALE", "window order is stale; refresh and retry", nil)
			return
		case tmux.IsKind(err, tmux.ErrKindInvalidIdentifier):
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
			return
		default:
			writeTmuxError(w, err)
			return
		}
	}

	liveWindows, err = h.tmux.ListWindows(ctx, session)
	if err != nil {
		writeTmuxError(w, err)
		return
	}
	if err := h.syncManagedTmuxWindowOrder(ctx, session, liveWindows); err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to sync managed tmux windows", nil)
		return
	}
	restoreActiveWindowBestEffort(ctx, h.tmux, session, activeWindowID, liveWindows)

	h.emit(events.TypeTmuxInspector, map[string]any{
		"session": session,
		"action":  "reorder-windows",
	})
	w.WriteHeader(http.StatusNoContent)
}

func decodeSessionOrderNames(r *http.Request) ([]string, error) {
	var req struct {
		Names []string `json:"names"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return nil, err
	}
	if len(req.Names) == 0 {
		return nil, errors.New("names are required")
	}
	seen := make(map[string]struct{}, len(req.Names))
	for _, name := range req.Names {
		if !validate.SessionName(name) {
			return nil, errors.New("names must match ^[A-Za-z0-9._-]{1,64}$")
		}
		if _, ok := seen[name]; ok {
			return nil, errors.New("names must be unique")
		}
		seen[name] = struct{}{}
	}
	return req.Names, nil
}

func decodeWindowOrderIDs(r *http.Request) ([]string, error) {
	var req struct {
		WindowIDs []string `json:"windowIds"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return nil, err
	}
	if len(req.WindowIDs) == 0 {
		return nil, errors.New("windowIds are required")
	}
	seen := make(map[string]struct{}, len(req.WindowIDs))
	windowIDs := make([]string, 0, len(req.WindowIDs))
	for _, rawID := range req.WindowIDs {
		windowID := strings.TrimSpace(rawID)
		if windowID == "" {
			return nil, errors.New("windowIds must not be empty")
		}
		if _, ok := seen[windowID]; ok {
			return nil, errors.New("windowIds must be unique")
		}
		seen[windowID] = struct{}{}
		windowIDs = append(windowIDs, windowID)
	}
	return windowIDs, nil
}

func windowOrderMatchesLive(liveWindows []tmux.Window, orderedWindowIDs []string) bool {
	if len(liveWindows) != len(orderedWindowIDs) {
		return false
	}
	liveSet := make(map[string]struct{}, len(liveWindows))
	for _, window := range liveWindows {
		windowID := strings.TrimSpace(window.ID)
		if windowID == "" {
			return false
		}
		liveSet[windowID] = struct{}{}
	}
	for _, windowID := range orderedWindowIDs {
		if _, ok := liveSet[windowID]; !ok {
			return false
		}
	}
	return true
}

func activeWindowRuntimeID(liveWindows []tmux.Window) string {
	for _, window := range liveWindows {
		if window.Active {
			return strings.TrimSpace(window.ID)
		}
	}
	return ""
}

func isWindowOrderStaleError(err error) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "does not match live windows")
}

func restoreActiveWindowBestEffort(
	ctx context.Context,
	service tmuxService,
	session, activeWindowID string,
	liveWindows []tmux.Window,
) {
	activeWindowID = strings.TrimSpace(activeWindowID)
	if activeWindowID == "" {
		return
	}
	for _, window := range liveWindows {
		if strings.TrimSpace(window.ID) != activeWindowID {
			continue
		}
		if window.Active {
			return
		}
		if err := service.SelectWindow(ctx, session, window.Index); err != nil {
			slog.Warn("failed to restore active tmux window after reorder", "session", session, "windowId", activeWindowID, "index", window.Index, "err", err)
		}
		return
	}
}
