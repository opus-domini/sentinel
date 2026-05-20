package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/guardrails"
	"github.com/opus-domini/sentinel/internal/tmux"
	"github.com/opus-domini/sentinel/internal/validate"
)

const maxSessionNameVariants = 99

func (h *Handler) createSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Cwd         string `json:"cwd"`
		Icon        string `json:"icon"`
		User        string `json:"user"`
		OperationID string `json:"operationId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Cwd = strings.TrimSpace(req.Cwd)
	req.Icon = strings.TrimSpace(req.Icon)
	req.User = strings.TrimSpace(req.User)
	req.OperationID = strings.TrimSpace(req.OperationID)
	if req.Cwd == "" {
		req.Cwd = defaultSessionCWD()
	}
	if !validate.SessionName(req.Name) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "name must match ^[A-Za-z0-9._-]{1,64}$", nil)
		return
	}
	if req.Cwd != "" && !filepath.IsAbs(req.Cwd) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "cwd must be an absolute path", nil)
		return
	}
	if req.Icon != "" && !validate.IconKey(req.Icon) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "icon must match ^[a-z0-9-]{1,32}$", nil)
		return
	}
	if err := h.guard.ValidateTargetUser(req.User); err != nil {
		writeError(w, http.StatusForbidden, "USER_NOT_ALLOWED", err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      actionSessionCreate,
		SessionName: req.Name,
		WindowIndex: -1,
	}); !ok {
		return
	}

	tmuxSvc := h.tmuxForUser(req.User)
	finalName, err := createSessionWithAvailableName(ctx, tmuxSvc, req.Name, req.Cwd)
	if err != nil {
		writeTmuxError(w, err)
		return
	}
	h.registerSessionUser(finalName, req.User)
	if req.User != "" {
		slog.Warn("multi-user session created",
			keyAction, actionSessionCreate,
			"target_user", req.User,
			keySession, finalName,
			"source_ip", r.RemoteAddr,
		)
	}
	h.persistSessionLaunchMetadataBestEffort(ctx, finalName, req.Cwd, req.Icon)
	if err := h.repo.MoveSessionToFront(ctx, finalName); err != nil {
		slog.Warn("failed to move session to front", keySession, finalName, "err", err)
	}
	payload := map[string]any{
		keySession: finalName,
		keyAction:  "create",
	}
	setOperationID(payload, req.OperationID)
	h.emit(events.TypeTmuxSessions, payload)
	writeData(w, http.StatusCreated, map[string]any{keyName: finalName})
}

func createSessionWithAvailableName(ctx context.Context, tmuxSvc tmuxService, seed, cwd string) (string, error) {
	for i := 0; i <= maxSessionNameVariants; i++ {
		candidate := sessionNameVariant(seed, i)
		if !validate.SessionName(candidate) {
			continue
		}
		if err := tmuxSvc.CreateSession(ctx, candidate, cwd); err == nil {
			return candidate, nil
		} else if !tmux.IsKind(err, tmux.ErrKindSessionExists) {
			return "", err
		}
	}
	return "", &tmux.Error{Kind: tmux.ErrKindSessionExists, Msg: "all name variants already exist"}
}

func sessionNameVariant(seed string, sequence int) string {
	seed = strings.TrimSpace(seed)
	if sequence <= 0 {
		return seed
	}
	suffix := fmt.Sprintf("-%d", sequence)
	if len(seed)+len(suffix) <= 64 {
		return seed + suffix
	}
	maxSeedLen := 64 - len(suffix)
	if maxSeedLen < 1 {
		return suffix[1:]
	}
	return seed[:maxSeedLen] + suffix
}

// tmuxForUser returns the tmux service to use for the given user.
// When user is empty, it returns the handler's default tmux service.
// When user is set, it returns a new tmux.Service that wraps commands
// with the configured user switching method.
func (h *Handler) tmuxForUser(user string) tmuxService {
	user = strings.TrimSpace(user)
	if user == "" {
		return h.tmux
	}
	return tmux.Service{User: user}
}

// tmuxForSession returns the tmux service for a session by looking up
// which OS user owns it. If the session was created as a different user,
// commands are wrapped with the configured user switching method.
// When the session is not in the registry, it probes known multi-user
// tmux servers as a fallback (the registry can be lost on restart).
func (h *Handler) tmuxForSession(session string) tmuxService {
	if user, ok := h.sessionUsers.Load(session); ok {
		if u, _ := user.(string); u != "" {
			return tmux.Service{User: u}
		}
	}

	// Fallback: probe known users' tmux servers for this session.
	for _, user := range h.knownSessionUsers() {
		svc := tmux.Service{User: user}
		if svc.HasSession(context.Background(), session) {
			h.registerSessionUser(session, user)
			return svc
		}
	}

	return h.tmux
}

// SessionUser returns the OS user that owns the given session, or "".
func (h *Handler) SessionUser(session string) string {
	if user, ok := h.sessionUsers.Load(session); ok {
		if u, _ := user.(string); u != "" {
			return u
		}
	}
	return ""
}

// registerSessionUser records which OS user owns a tmux session,
// both in memory and in persistent storage.
func (h *Handler) registerSessionUser(session, user string) {
	user = strings.TrimSpace(user)
	if user == "" {
		return
	}
	h.sessionUsers.Store(session, user)
	if h.repo != nil {
		_ = h.repo.SetSessionUser(context.Background(), session, user)
	}
}

// knownSessionUsers returns the set of unique non-empty usernames
// from the session user registry.
func (h *Handler) knownSessionUsers() []string {
	seen := make(map[string]struct{})
	h.sessionUsers.Range(func(_, value any) bool {
		if u, _ := value.(string); u != "" {
			seen[u] = struct{}{}
		}
		return true
	})
	users := make([]string, 0, len(seen))
	for u := range seen {
		users = append(users, u)
	}
	return users
}

// populateSessionUsersFromPresets loads session user mappings from
// persistent storage and pinned session presets so that multi-user sessions
// are known after restart.
func (h *Handler) populateSessionUsersFromPresets(ctx context.Context) {
	if h.repo == nil {
		return
	}
	// Load from dedicated session_users table first.
	if userMap, err := h.repo.ListSessionUsers(ctx); err == nil {
		for session, user := range userMap {
			h.sessionUsers.Store(session, user)
		}
	}
	// Also load pinned session presets, which may have user overrides.
	presets, err := h.repo.ListSessionPresets(ctx)
	if err != nil {
		slog.Warn("failed to load pinned session presets for user registry", "err", err)
		return
	}
	for _, preset := range presets {
		if preset.User != "" {
			h.sessionUsers.Store(preset.Name, preset.User)
		}
	}
}

func (h *Handler) renameSession(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue(keySession))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid source session name", nil)
		return
	}

	var req struct {
		NewName string `json:"newName"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.NewName = strings.TrimSpace(req.NewName)
	if !validate.SessionName(req.NewName) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "newName must match ^[A-Za-z0-9._-]{1,64}$", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	svc := h.tmuxForSession(session)
	if err := svc.RenameSession(ctx, session, req.NewName); err != nil {
		writeTmuxError(w, err)
		return
	}
	// Migrate the user registry entry to the new session name.
	if user := h.SessionUser(session); user != "" {
		h.sessionUsers.Delete(session)
		h.registerSessionUser(req.NewName, user)
		if h.repo != nil {
			_ = h.repo.RenameSessionUser(context.Background(), session, req.NewName)
		}
	}
	if err := h.repo.Rename(ctx, session, req.NewName); err != nil {
		slog.Warn("store.Rename failed", "from", session, "to", req.NewName, "err", err)
	}
	h.renameSessionPresetBestEffort(ctx, session, req.NewName)
	h.emit(events.TypeTmuxSessions, map[string]any{
		keySession: session,
		"newName":  req.NewName,
		keyAction:  "rename",
	})
	writeData(w, http.StatusOK, map[string]any{keyName: req.NewName})
}

func (h *Handler) setSessionIcon(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue(keySession))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	var req struct {
		Icon string `json:"icon"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.Icon = strings.TrimSpace(req.Icon)
	if !validate.IconKey(req.Icon) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "icon must match ^[a-z0-9-]{1,32}$", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.repo.SetIcon(ctx, session, req.Icon); err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to set icon", nil)
		return
	}
	h.emit(events.TypeTmuxSessions, map[string]any{
		keySession: session,
		keyAction:  "icon",
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) deleteSession(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue(keySession))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "session.kill",
		SessionName: session,
		WindowIndex: -1,
	}); !ok {
		return
	}

	if err := h.tmuxForSession(session).KillSession(ctx, session); err != nil &&
		!tmux.IsKind(err, tmux.ErrKindSessionNotFound) &&
		!tmux.IsKind(err, tmux.ErrKindServerNotRunning) {
		writeTmuxError(w, err)
		return
	}
	h.sessionUsers.Delete(session)
	if h.repo != nil {
		_ = h.repo.DeleteSessionUser(context.Background(), session)
		_ = h.repo.DeleteSessionPreset(context.Background(), session)
	}
	h.emit(events.TypeTmuxSessions, map[string]any{keySession: session, keyAction: "delete"})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) frequentDirectories(w http.ResponseWriter, r *http.Request) {
	limit := 5
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			if parsed > 20 {
				parsed = 20
			}
			limit = parsed
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	dirs, err := h.repo.ListFrequentDirectories(ctx, limit)
	if err != nil {
		slog.Warn("failed to list frequent directories", "err", err)
		writeData(w, http.StatusOK, map[string]any{keyDirs: []string{}})
		return
	}
	writeData(w, http.StatusOK, map[string]any{keyDirs: dirs})
}
