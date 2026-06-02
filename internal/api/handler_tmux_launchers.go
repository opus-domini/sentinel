package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/guardrails"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
	"github.com/opus-domini/sentinel/internal/userswitch"
	"github.com/opus-domini/sentinel/internal/validate"
)

func (h *Handler) listTmuxLaunchers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	launchers, err := h.repo.ListTmuxLaunchers(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to list tmux launchers", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"launchers": launchers})
}

func (h *Handler) createTmuxLauncher(w http.ResponseWriter, r *http.Request) {
	row, err := decodeTmuxLauncherWrite(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if err := h.validateTmuxLauncherUser(row); err != nil {
		writeError(w, http.StatusForbidden, "USER_NOT_ALLOWED", err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	launcher, err := h.repo.CreateTmuxLauncher(ctx, row)
	if err != nil {
		if isUniqueConstraintError(err) {
			writeError(w, http.StatusConflict, "TMUX_LAUNCHER_EXISTS", "tmux launcher already exists", nil)
			return
		}
		if isTmuxLauncherValidationError(err) {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to create tmux launcher", nil)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{keyLauncher: launcher})
}

func (h *Handler) updateTmuxLauncher(w http.ResponseWriter, r *http.Request) {
	launcherID := strings.TrimSpace(r.PathValue(keyLauncher))
	if launcherID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "tmux launcher id is required", nil)
		return
	}

	row, err := decodeTmuxLauncherWrite(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if err := h.validateTmuxLauncherUser(row); err != nil {
		writeError(w, http.StatusForbidden, "USER_NOT_ALLOWED", err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	launcher, err := h.repo.UpdateTmuxLauncher(ctx, launcherID, row)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeError(w, http.StatusNotFound, "TMUX_LAUNCHER_NOT_FOUND", "tmux launcher not found", nil)
		case isUniqueConstraintError(err):
			writeError(w, http.StatusConflict, "TMUX_LAUNCHER_EXISTS", "tmux launcher already exists", nil)
		case isTmuxLauncherValidationError(err):
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		default:
			writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to update tmux launcher", nil)
		}
		return
	}
	writeData(w, http.StatusOK, map[string]any{keyLauncher: launcher})
}

func (h *Handler) deleteTmuxLauncher(w http.ResponseWriter, r *http.Request) {
	launcherID := strings.TrimSpace(r.PathValue(keyLauncher))
	if launcherID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "tmux launcher id is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	err := h.repo.DeleteTmuxLauncher(ctx, launcherID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "TMUX_LAUNCHER_NOT_FOUND", "tmux launcher not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to delete tmux launcher", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) reorderTmuxLaunchers(w http.ResponseWriter, r *http.Request) {
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

	if err := h.repo.ReorderTmuxLaunchers(ctx, req.IDs); err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to reorder tmux launchers", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) launchTmuxLauncher(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue(keySession))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}
	launcherID := strings.TrimSpace(r.PathValue(keyLauncher))
	if launcherID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "tmux launcher id is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	launcher, err := h.repo.GetTmuxLauncher(ctx, launcherID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "TMUX_LAUNCHER_NOT_FOUND", "tmux launcher not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load tmux launcher", nil)
		return
	}

	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "window.create",
		SessionName: session,
		WindowIndex: -1,
	}); !ok {
		return
	}

	cwd, err := h.resolveTmuxLauncherCwd(ctx, session, launcher)
	if err != nil {
		writeTmuxError(w, err)
		return
	}

	windowName := strings.TrimSpace(launcher.WindowName)
	if windowName == "" {
		windowName = launcher.Name
	}

	svc := h.tmuxForSession(ctx, session)
	createdWindow, err := svc.NewWindowWithOptions(ctx, session, windowName, cwd)
	if err != nil {
		writeTmuxError(w, err)
		return
	}
	launcherCommand, err := h.tmuxLauncherCommand(session, launcher)
	if err != nil {
		writeError(w, http.StatusForbidden, "USER_NOT_ALLOWED", err.Error(), nil)
		return
	}
	if strings.TrimSpace(launcherCommand) != "" {
		if err := svc.SendKeys(ctx, createdWindow.PaneID, launcherCommand, true); err != nil {
			writeTmuxError(w, err)
			return
		}
	}
	if err := h.repo.MarkTmuxLauncherUsed(ctx, launcher.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to record tmux launcher usage", nil)
		return
	}
	managedWindow, err := h.repo.CreateManagedTmuxWindow(ctx, store.ManagedTmuxWindowWrite{
		SessionName:     session,
		LauncherID:      launcher.ID,
		LauncherName:    launcher.Name,
		Icon:            launcher.Icon,
		Command:         launcher.Command,
		CwdMode:         launcher.CwdMode,
		CwdValue:        launcher.CwdValue,
		ResolvedCwd:     cwd,
		WindowName:      windowName,
		TmuxWindowID:    createdWindow.ID,
		LastWindowIndex: createdWindow.Index,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to persist managed tmux window", nil)
		return
	}

	h.emit(events.TypeTmuxInspector, map[string]any{
		keySession: session,
		keyAction:  "launcher-window",
		keyIndex:   createdWindow.Index,
		keyPaneID:  createdWindow.PaneID,
		keyName:    windowName,
	})
	h.emit(events.TypeTmuxSessions, map[string]any{keySession: session, keyAction: actionWindowCount})
	writeData(w, http.StatusOK, map[string]any{
		"launcherId":      launcher.ID,
		"windowIndex":     createdWindow.Index,
		keyPaneID:         createdWindow.PaneID,
		"windowName":      windowName,
		"managedWindowId": managedWindow.ID,
	})
}

func (h *Handler) validateTmuxLauncherUser(row store.TmuxLauncherWrite) error {
	if strings.TrimSpace(row.UserMode) != store.TmuxLauncherUserModeFixed {
		return nil
	}
	return h.guard.ValidateTargetUser(row.UserValue)
}

func (h *Handler) tmuxLauncherCommand(session string, launcher store.TmuxLauncher) (string, error) {
	command := strings.TrimSpace(launcher.Command)
	if strings.TrimSpace(launcher.UserMode) != store.TmuxLauncherUserModeFixed {
		return command, nil
	}

	targetUser := strings.TrimSpace(launcher.UserValue)
	if err := h.guard.ValidateTargetUser(targetUser); err != nil {
		return "", err
	}
	if targetUser == strings.TrimSpace(h.SessionUser(session)) {
		return command, nil
	}
	method := strings.TrimSpace(h.userSwitchMethod)
	if method == "" {
		method = tmux.UserSwitchMethod
	}
	return userswitch.BuildShellCommand(method, targetUser, command)
}

func (h *Handler) resolveTmuxLauncherCwd(ctx context.Context, session string, launcher store.TmuxLauncher) (string, error) {
	switch launcher.CwdMode {
	case store.TmuxLauncherCwdModeSession:
		return "", nil
	case store.TmuxLauncherCwdModeFixed:
		return launcher.CwdValue, nil
	case store.TmuxLauncherCwdModeActivePane:
		panes, err := h.tmuxForSession(ctx, session).ListPanes(ctx, session)
		if err != nil {
			return "", err
		}
		for _, pane := range panes {
			if pane.Active && strings.TrimSpace(pane.CurrentPath) != "" {
				return pane.CurrentPath, nil
			}
		}
		for _, pane := range panes {
			if strings.TrimSpace(pane.CurrentPath) != "" {
				return pane.CurrentPath, nil
			}
		}
		return "", nil
	default:
		return "", errors.New("invalid tmux launcher cwd mode")
	}
}

func isTmuxLauncherValidationError(err error) bool {
	if err == nil {
		return false
	}
	switch err.Error() {
	case "tmux launcher name is required",
		"tmux launcher icon is required",
		"tmux launcher fixed cwd is required",
		"tmux launcher fixed user is required",
		"invalid tmux launcher cwd mode",
		"invalid tmux launcher user mode":
		return true
	default:
		return false
	}
}

func decodeTmuxLauncherWrite(r *http.Request) (store.TmuxLauncherWrite, error) {
	var req struct {
		Name       string `json:"name"`
		Icon       string `json:"icon"`
		Command    string `json:"command"`
		CwdMode    string `json:"cwdMode"`
		CwdValue   string `json:"cwdValue"`
		WindowName string `json:"windowName"`
		UserMode   string `json:"userMode"`
		UserValue  string `json:"userValue"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return store.TmuxLauncherWrite{}, err
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Icon = strings.TrimSpace(req.Icon)
	req.Command = strings.TrimSpace(req.Command)
	req.CwdMode = strings.TrimSpace(req.CwdMode)
	req.CwdValue = strings.TrimSpace(req.CwdValue)
	req.WindowName = strings.TrimSpace(req.WindowName)
	req.UserMode = strings.TrimSpace(req.UserMode)
	req.UserValue = strings.TrimSpace(req.UserValue)

	if !validate.WindowName(req.Name) {
		return store.TmuxLauncherWrite{}, errors.New("tmux launcher name must match ^[A-Za-z0-9._][A-Za-z0-9._\\- ]{0,63}$")
	}
	if !validate.IconKey(req.Icon) {
		return store.TmuxLauncherWrite{}, errors.New("icon must match ^[a-z0-9-]{1,32}$")
	}
	if req.WindowName != "" && !validate.WindowName(req.WindowName) {
		return store.TmuxLauncherWrite{}, errors.New("windowName must match ^[A-Za-z0-9._][A-Za-z0-9._\\- ]{0,63}$")
	}
	if req.CwdMode == store.TmuxLauncherCwdModeFixed && req.CwdValue != "" && !filepath.IsAbs(req.CwdValue) {
		return store.TmuxLauncherWrite{}, errors.New("cwdValue must be an absolute path")
	}

	return store.TmuxLauncherWrite{
		Name:       req.Name,
		Icon:       req.Icon,
		Command:    req.Command,
		CwdMode:    req.CwdMode,
		CwdValue:   req.CwdValue,
		WindowName: req.WindowName,
		UserMode:   req.UserMode,
		UserValue:  req.UserValue,
	}, nil
}
