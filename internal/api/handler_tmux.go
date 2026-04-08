package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/guardrails"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
	"github.com/opus-domini/sentinel/internal/validate"
)

func sameProjectedWindowSet(live []tmux.Window, projected []store.WatchtowerWindow) bool {
	if len(live) != len(projected) {
		return false
	}
	if len(live) == 0 {
		return false
	}
	projectedByIndex := make(map[int]struct{}, len(projected))
	for _, row := range projected {
		projectedByIndex[row.WindowIndex] = struct{}{}
	}
	for _, row := range live {
		if _, ok := projectedByIndex[row.Index]; !ok {
			return false
		}
	}
	return true
}

func sameProjectedPaneSet(live []tmux.Pane, projected []store.WatchtowerPane) bool {
	if len(live) != len(projected) {
		return false
	}
	if len(live) == 0 {
		return false
	}
	projectedByID := make(map[string]struct{}, len(projected))
	for _, row := range projected {
		projectedByID[strings.TrimSpace(row.PaneID)] = struct{}{}
	}
	for _, row := range live {
		if _, ok := projectedByID[strings.TrimSpace(row.PaneID)]; !ok {
			return false
		}
	}
	return true
}

func setOperationID(payload map[string]any, operationID string) {
	trimmed := strings.TrimSpace(operationID)
	if trimmed == "" {
		return
	}
	payload["operationId"] = trimmed
}

func projectedWindowsToEnriched(
	windows []store.WatchtowerWindow,
	panes []store.WatchtowerPane,
	managedByRuntime map[string]store.ManagedTmuxWindow,
) []enrichedWindow {
	paneCounts := make(map[int]int, len(windows))
	for _, pane := range panes {
		paneCounts[pane.WindowIndex]++
	}

	resp := make([]enrichedWindow, 0, len(windows))
	for _, row := range windows {
		presentation := presentationForProjectedWindow(row.Name, row.TmuxWindowID, managedByRuntime)
		resp = append(resp, enrichedWindow{
			Session:         row.SessionName,
			Index:           row.WindowIndex,
			Name:            row.Name,
			DisplayName:     presentation.displayName,
			DisplayIcon:     presentation.displayIcon,
			TmuxWindowID:    strings.TrimSpace(row.TmuxWindowID),
			Managed:         presentation.managed,
			ManagedWindowID: presentation.managedWindowID,
			LauncherID:      presentation.launcherID,
			Active:          row.Active,
			Panes:           paneCounts[row.WindowIndex],
			Layout:          row.Layout,
			UnreadPanes:     row.UnreadPanes,
			HasUnread:       row.HasUnread,
			Rev:             row.Rev,
			ActivityAt:      row.WindowActivityAt.Format(time.RFC3339),
		})
	}
	return resp
}

func projectedPanesToEnriched(panes []store.WatchtowerPane) []enrichedPane {
	resp := make([]enrichedPane, 0, len(panes))
	for _, row := range panes {
		resp = append(resp, enrichedPane{
			Session:        row.SessionName,
			WindowIndex:    row.WindowIndex,
			PaneIndex:      row.PaneIndex,
			PaneID:         row.PaneID,
			Title:          row.Title,
			Active:         row.Active,
			TTY:            row.TTY,
			CurrentPath:    row.CurrentPath,
			StartCommand:   row.StartCommand,
			CurrentCommand: row.CurrentCommand,
			TailPreview:    row.TailPreview,
			Revision:       row.Revision,
			SeenRevision:   row.SeenRevision,
			HasUnread:      row.Revision > row.SeenRevision,
			ChangedAt:      row.ChangedAt.Format(time.RFC3339),
		})
	}
	return resp
}

func (h *Handler) listProjectedWindows(ctx context.Context, session string) ([]store.WatchtowerWindow, []store.WatchtowerPane, bool) {
	if h.repo == nil {
		return nil, nil, false
	}

	windows, err := h.repo.ListWatchtowerWindows(ctx, session)
	if err != nil {
		slog.Warn("store.ListWatchtowerWindows failed", "session", session, "err", err)
		return nil, nil, false
	}
	panes, err := h.repo.ListWatchtowerPanes(ctx, session)
	if err != nil {
		slog.Warn("store.ListWatchtowerPanes failed", "session", session, "err", err)
		return nil, nil, false
	}
	if len(windows) == 0 {
		return nil, nil, false
	}
	return windows, panes, true
}

func (h *Handler) listProjectedPanes(ctx context.Context, session string) ([]store.WatchtowerPane, bool) {
	if h.repo == nil {
		return nil, false
	}

	panes, err := h.repo.ListWatchtowerPanes(ctx, session)
	if err != nil {
		slog.Warn("store.ListWatchtowerPanes failed", "session", session, "err", err)
		return nil, false
	}
	if len(panes) == 0 {
		return nil, false
	}
	return panes, true
}

func paneBelongsToSession(panes []tmux.Pane, paneID string) bool {
	id := strings.TrimSpace(paneID)
	if id == "" {
		return false
	}
	for _, pane := range panes {
		if strings.TrimSpace(pane.PaneID) == id {
			return true
		}
	}
	return false
}

func (h *Handler) ensureSessionPane(ctx context.Context, session, paneID string) error {
	panes, err := h.tmuxForSession(session).ListPanes(ctx, session)
	if err != nil {
		return err
	}
	if !paneBelongsToSession(panes, paneID) {
		return errors.New("pane does not belong to session")
	}
	return nil
}

func (h *Handler) listSessions(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stored := h.loadSessionMetaMap(ctx)
	if sessions, ok := h.listSessionsFromProjection(ctx, stored); ok {
		writeData(w, http.StatusOK, map[string]any{"sessions": sessions})
		return
	}

	sessions, err := h.listSessionsFromTmux(ctx, stored)
	if err != nil {
		writeTmuxError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (h *Handler) loadSessionMetaMap(ctx context.Context) map[string]store.SessionMeta {
	if h.repo == nil {
		return map[string]store.SessionMeta{}
	}
	meta, err := h.repo.GetAll(ctx)
	if err != nil {
		slog.Warn("store.GetAll failed", "err", err)
		return map[string]store.SessionMeta{}
	}
	return meta
}

func (h *Handler) listSessionsFromProjection(ctx context.Context, stored map[string]store.SessionMeta) ([]enrichedSession, bool) {
	if h.repo == nil {
		return nil, false
	}
	projected, err := h.repo.ListWatchtowerSessions(ctx)
	if err != nil {
		slog.Warn("store.ListWatchtowerSessions failed", "err", err)
		return nil, false
	}
	if len(projected) == 0 {
		return nil, false
	}

	seen := make(map[string]struct{}, len(projected))
	activeNames := make([]string, 0, len(projected))
	result := make([]enrichedSession, 0, len(projected))
	for _, row := range projected {
		seen[row.SessionName] = struct{}{}
		activeNames = append(activeNames, row.SessionName)
		result = append(result, h.projectedSessionToEnriched(ctx, row, stored[row.SessionName]))
	}

	// Append sessions from multi-user tmux servers not covered by
	// the watchtower projection (which only monitors the default user).
	for _, user := range h.knownSessionUsers() {
		svc := tmux.Service{User: user}
		userSessions, listErr := svc.ListSessions(ctx)
		if listErr != nil {
			slog.Warn("multi-user session list failed", "user", user, "err", listErr)
			continue
		}
		userSnapshots, _ := svc.ListActivePaneCommands(ctx)
		for _, sess := range userSessions {
			if _, exists := seen[sess.Name]; exists {
				continue
			}
			seen[sess.Name] = struct{}{}
			activeNames = append(activeNames, sess.Name)
			h.registerSessionUser(sess.Name, user)
			enriched := h.tmuxSessionToEnriched(ctx, sess, userSnapshots[sess.Name], stored[sess.Name])
			enriched.User = user
			result = append(result, enriched)
		}
	}

	h.purgeStoredSessionsBestEffort(ctx, activeNames)
	sortSessionsByStoredOrder(result)
	return result, true
}

func (h *Handler) projectedSessionToEnriched(ctx context.Context, row store.WatchtowerSession, meta store.SessionMeta) enrichedSession {
	hash := strings.TrimSpace(meta.Hash)
	if hash == "" {
		hash = tmux.SessionHash(row.SessionName, row.ActivityAt.Unix())
	}
	lastContent := strings.TrimSpace(row.LastPreview)
	if lastContent == "" {
		lastContent = strings.TrimSpace(meta.LastContent)
	}
	h.upsertSessionMetaBestEffort(ctx, row.SessionName, hash, lastContent)
	return enrichedSession{
		Name:          row.SessionName,
		Windows:       row.Windows,
		Panes:         row.Panes,
		Attached:      row.Attached,
		CreatedAt:     projectedCreatedAt(row).Format(time.RFC3339),
		ActivityAt:    row.ActivityAt.Format(time.RFC3339),
		Command:       "",
		Hash:          hash,
		LastContent:   lastContent,
		Icon:          meta.Icon,
		User:          h.SessionUser(row.SessionName),
		SortOrder:     meta.SortOrder,
		UnreadWindows: row.UnreadWindows,
		UnreadPanes:   row.UnreadPanes,
		Rev:           row.Rev,
	}
}

func projectedCreatedAt(row store.WatchtowerSession) time.Time {
	createdAt := row.ActivityAt
	if row.LastPreviewAt.Before(createdAt) {
		return row.LastPreviewAt
	}
	return createdAt
}

func (h *Handler) listSessionsFromTmux(ctx context.Context, stored map[string]store.SessionMeta) ([]enrichedSession, error) {
	// Warmup fallback: projections may still be empty right after startup.
	sessions, err := h.tmux.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	snapshots := h.loadActivePaneSnapshots(ctx)

	// Collect sessions from the default user's tmux server.
	seen := make(map[string]struct{}, len(sessions))
	activeNames := make([]string, 0, len(sessions))
	result := make([]enrichedSession, 0, len(sessions))
	for _, sess := range sessions {
		seen[sess.Name] = struct{}{}
		activeNames = append(activeNames, sess.Name)
		enriched := h.tmuxSessionToEnriched(ctx, sess, snapshots[sess.Name], stored[sess.Name])
		result = append(result, enriched)
	}

	// Also query each known multi-user tmux server.
	for _, user := range h.knownSessionUsers() {
		svc := tmux.Service{User: user}
		userSessions, listErr := svc.ListSessions(ctx)
		if listErr != nil {
			slog.Warn("multi-user session list failed", "user", user, "err", listErr)
			continue
		}
		userSnapshots, _ := svc.ListActivePaneCommands(ctx)
		for _, sess := range userSessions {
			if _, exists := seen[sess.Name]; exists {
				continue
			}
			seen[sess.Name] = struct{}{}
			activeNames = append(activeNames, sess.Name)
			h.registerSessionUser(sess.Name, user)
			enriched := h.tmuxSessionToEnriched(ctx, sess, userSnapshots[sess.Name], stored[sess.Name])
			enriched.User = user
			result = append(result, enriched)
		}
	}

	h.purgeStoredSessionsBestEffort(ctx, activeNames)
	sortSessionsByStoredOrder(result)
	return result, nil
}

func (h *Handler) loadActivePaneSnapshots(ctx context.Context) map[string]tmux.PaneSnapshot {
	snapshots, err := h.tmux.ListActivePaneCommands(ctx)
	if err != nil {
		slog.Warn("list-pane-commands failed", "err", err)
		return map[string]tmux.PaneSnapshot{}
	}
	return snapshots
}

func (h *Handler) tmuxSessionToEnriched(ctx context.Context, sess tmux.Session, snap tmux.PaneSnapshot, meta store.SessionMeta) enrichedSession {
	hash := strings.TrimSpace(meta.Hash)
	if hash == "" {
		hash = tmux.SessionHash(sess.Name, sess.CreatedAt.Unix())
	}
	lastContent := h.resolveSessionLastContent(ctx, sess.Name, meta.LastContent)
	h.upsertSessionMetaBestEffort(ctx, sess.Name, hash, lastContent)

	return enrichedSession{
		Name:          sess.Name,
		Windows:       sess.Windows,
		Panes:         h.resolveSessionPaneCount(ctx, sess.Name, snap.Panes, sess.Windows),
		Attached:      sess.Attached,
		CreatedAt:     sess.CreatedAt.Format(time.RFC3339),
		ActivityAt:    sess.ActivityAt.Format(time.RFC3339),
		Command:       snap.Command,
		Hash:          hash,
		LastContent:   lastContent,
		Icon:          meta.Icon,
		User:          h.SessionUser(sess.Name),
		SortOrder:     meta.SortOrder,
		UnreadWindows: 0,
		UnreadPanes:   0,
		Rev:           0,
	}
}

func sortSessionsByStoredOrder(sessions []enrichedSession) {
	sort.SliceStable(sessions, func(left, right int) bool {
		leftOrder := sessions[left].SortOrder
		rightOrder := sessions[right].SortOrder
		switch {
		case leftOrder == rightOrder:
			return strings.ToLower(sessions[left].Name) < strings.ToLower(sessions[right].Name)
		case leftOrder == 0:
			return false
		case rightOrder == 0:
			return true
		default:
			return leftOrder < rightOrder
		}
	})
}

func (h *Handler) resolveSessionLastContent(ctx context.Context, sessionName, fallback string) string {
	lastContent := strings.TrimSpace(fallback)
	captured, err := h.tmuxForSession(sessionName).CapturePane(ctx, sessionName)
	if err != nil {
		return lastContent
	}
	trimmed := strings.TrimSpace(captured)
	if trimmed == "" {
		return lastContent
	}
	return trimmed
}

func (h *Handler) resolveSessionPaneCount(ctx context.Context, sessionName string, projectedPanes, windowFallback int) int {
	if projectedPanes > 0 {
		return projectedPanes
	}
	paneList, err := h.tmuxForSession(sessionName).ListPanes(ctx, sessionName)
	if err != nil {
		return windowFallback
	}
	return len(paneList)
}

func (h *Handler) upsertSessionMetaBestEffort(ctx context.Context, sessionName, hash, lastContent string) {
	if h.repo == nil {
		return
	}
	if err := h.repo.UpsertSession(ctx, sessionName, hash, lastContent); err != nil {
		slog.Warn("store.UpsertSession failed", "session", sessionName, "err", err)
	}
}

func (h *Handler) purgeStoredSessionsBestEffort(ctx context.Context, activeNames []string) {
	if h.repo == nil {
		return
	}
	if err := h.repo.Purge(ctx, activeNames); err != nil {
		slog.Warn("store.Purge failed", "err", err)
	}
}

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
		Action:      "session.create",
		SessionName: req.Name,
		WindowIndex: -1,
	}); !ok {
		return
	}

	tmuxSvc := h.tmuxForUser(req.User)

	// Try the requested name first, then append -1, -2, etc. on collision.
	finalName := req.Name
	if err := tmuxSvc.CreateSession(ctx, finalName, req.Cwd); err != nil {
		if !tmux.IsKind(err, tmux.ErrKindSessionExists) {
			writeTmuxError(w, err)
			return
		}
		created := false
		for i := 1; i <= 9; i++ {
			candidate := fmt.Sprintf("%s-%d", req.Name, i)
			if err := tmuxSvc.CreateSession(ctx, candidate, req.Cwd); err == nil {
				finalName = candidate
				created = true
				break
			} else if !tmux.IsKind(err, tmux.ErrKindSessionExists) {
				writeTmuxError(w, err)
				return
			}
		}
		if !created {
			writeTmuxError(w, &tmux.Error{Kind: tmux.ErrKindSessionExists, Msg: "all name variants already exist"})
			return
		}
	}
	h.registerSessionUser(finalName, req.User)
	if req.User != "" {
		slog.Warn("multi-user session created",
			"action", "session.create",
			"target_user", req.User,
			"session", finalName,
			"source_ip", r.RemoteAddr,
		)
	}
	h.persistSessionLaunchMetadataBestEffort(ctx, finalName, req.Cwd, req.Icon)
	if err := h.repo.MoveSessionToFront(ctx, finalName); err != nil {
		slog.Warn("failed to move session to front", "session", finalName, "err", err)
	}
	payload := map[string]any{
		"session": finalName,
		"action":  "create",
	}
	setOperationID(payload, req.OperationID)
	h.emit(events.TypeTmuxSessions, payload)
	writeData(w, http.StatusCreated, map[string]any{"name": finalName})
}

// tmuxForUser returns the tmux service to use for the given user.
// When user is empty, it returns the handler's default tmux service.
// When user is set, it returns a new tmux.Service that wraps commands
// with sudo -n -u <user>.
func (h *Handler) tmuxForUser(user string) tmuxService {
	user = strings.TrimSpace(user)
	if user == "" {
		return h.tmux
	}
	return tmux.Service{User: user}
}

// tmuxForSession returns the tmux service for a session by looking up
// which OS user owns it. If the session was created as a different user,
// commands are wrapped with sudo -n -u <user>.
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
// persistent storage and session presets so that multi-user sessions
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
	// Also load from session presets (which may have user overrides).
	presets, err := h.repo.ListSessionPresets(ctx)
	if err != nil {
		slog.Warn("failed to load session presets for user registry", "err", err)
		return
	}
	for _, preset := range presets {
		if preset.User != "" {
			h.sessionUsers.Store(preset.Name, preset.User)
		}
	}
}

func (h *Handler) renameSession(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
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
		"session": session,
		"newName": req.NewName,
		"action":  "rename",
	})
	writeData(w, http.StatusOK, map[string]any{"name": req.NewName})
}

func (h *Handler) setSessionIcon(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
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
		"session": session,
		"action":  "icon",
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) deleteSession(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
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

	if err := h.tmuxForSession(session).KillSession(ctx, session); err != nil {
		writeTmuxError(w, err)
		return
	}
	h.sessionUsers.Delete(session)
	if h.repo != nil {
		_ = h.repo.DeleteSessionUser(context.Background(), session)
		_ = h.repo.DeleteSessionPreset(context.Background(), session)
	}
	h.emit(events.TypeTmuxSessions, map[string]any{"session": session, "action": "delete"})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listWindows(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	svc := h.tmuxForSession(session)
	windows, err := svc.ListWindows(ctx, session)
	if err != nil {
		projectedWindows, projectedPanes, ok := h.listProjectedWindows(ctx, session)
		if ok {
			managedRows, managedErr := h.listManagedTmuxWindows(ctx, session)
			if managedErr != nil {
				slog.Warn("store.ListManagedTmuxWindowsBySession failed", "session", session, "err", managedErr)
			}
			writeData(w, http.StatusOK, map[string]any{
				"windows": projectedWindowsToEnriched(projectedWindows, projectedPanes, managedWindowsByRuntime(managedRows)),
			})
			return
		}
		writeTmuxError(w, err)
		return
	}

	managedRows, managedErr := h.reconcileManagedTmuxWindows(ctx, session, windows)
	if managedErr != nil {
		slog.Warn("failed to reconcile managed tmux windows", "session", session, "err", managedErr)
		managedRows = nil
	}
	managedByRuntime := managedWindowsByRuntime(managedRows)

	projectedWindows, _, canOverlay := h.listProjectedWindows(ctx, session)
	projectedByIndex := make(map[int]store.WatchtowerWindow)
	if canOverlay && sameProjectedWindowSet(windows, projectedWindows) {
		projectedByIndex = make(map[int]store.WatchtowerWindow, len(projectedWindows))
		for _, row := range projectedWindows {
			projectedByIndex[row.WindowIndex] = row
		}
	}

	resp := make([]enrichedWindow, 0, len(windows))
	for _, row := range windows {
		presentation := presentationForLiveWindow(row, managedByRuntime)
		projected, hasProjected := projectedByIndex[row.Index]
		activityAt := ""
		unreadPanes := 0
		hasUnread := false
		var rev int64
		if hasProjected {
			activityAt = projected.WindowActivityAt.Format(time.RFC3339)
			unreadPanes = projected.UnreadPanes
			hasUnread = projected.HasUnread
			rev = projected.Rev
		}
		resp = append(resp, enrichedWindow{
			Session:         row.Session,
			Index:           row.Index,
			Name:            row.Name,
			DisplayName:     presentation.displayName,
			DisplayIcon:     presentation.displayIcon,
			TmuxWindowID:    strings.TrimSpace(row.ID),
			Managed:         presentation.managed,
			ManagedWindowID: presentation.managedWindowID,
			LauncherID:      presentation.launcherID,
			Active:          row.Active,
			Panes:           row.Panes,
			Layout:          row.Layout,
			UnreadPanes:     unreadPanes,
			HasUnread:       hasUnread,
			Rev:             rev,
			ActivityAt:      activityAt,
		})
	}
	writeData(w, http.StatusOK, map[string]any{"windows": resp})
}

func (h *Handler) listPanes(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	panes, err := h.tmuxForSession(session).ListPanes(ctx, session)
	if err != nil {
		projectedPanes, ok := h.listProjectedPanes(ctx, session)
		if ok {
			writeData(w, http.StatusOK, map[string]any{
				"panes": projectedPanesToEnriched(projectedPanes),
			})
			return
		}
		writeTmuxError(w, err)
		return
	}

	projectedPanes, canOverlay := h.listProjectedPanes(ctx, session)
	projectedByID := make(map[string]store.WatchtowerPane)
	if canOverlay && sameProjectedPaneSet(panes, projectedPanes) {
		projectedByID = make(map[string]store.WatchtowerPane, len(projectedPanes))
		for _, row := range projectedPanes {
			projectedByID[strings.TrimSpace(row.PaneID)] = row
		}
	}

	resp := make([]enrichedPane, 0, len(panes))
	for _, row := range panes {
		projected, hasProjected := projectedByID[strings.TrimSpace(row.PaneID)]
		tailPreview := ""
		var revision int64
		var seenRevision int64
		hasUnread := false
		changedAt := ""
		if hasProjected {
			tailPreview = projected.TailPreview
			revision = projected.Revision
			seenRevision = projected.SeenRevision
			hasUnread = projected.Revision > projected.SeenRevision
			changedAt = projected.ChangedAt.Format(time.RFC3339)
		}
		resp = append(resp, enrichedPane{
			Session:        row.Session,
			WindowIndex:    row.WindowIndex,
			PaneIndex:      row.PaneIndex,
			PaneID:         row.PaneID,
			Title:          row.Title,
			Active:         row.Active,
			TTY:            row.TTY,
			CurrentPath:    row.CurrentPath,
			StartCommand:   row.StartCommand,
			CurrentCommand: row.CurrentCommand,
			TailPreview:    tailPreview,
			Revision:       revision,
			SeenRevision:   seenRevision,
			HasUnread:      hasUnread,
			ChangedAt:      changedAt,
		})
	}
	writeData(w, http.StatusOK, map[string]any{"panes": resp})
}

func (h *Handler) markSessionSeen(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}
	if h.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}

	req, err := decodeMarkSeenRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if err := validateMarkSeenRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	acked, err := h.applyMarkSeen(ctx, session, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to mark seen", nil)
		return
	}

	globalRev := readWatchtowerGlobalRev(ctx, h.repo)
	sessionPatches, inspectorPatches := h.collectSeenPatches(ctx, session)
	if acked {
		h.emitMarkSeenEvents(session, req.Scope, globalRev, sessionPatches, inspectorPatches)
	}
	writeData(w, http.StatusOK, buildSeenResponsePayload(session, req.Scope, acked, globalRev, sessionPatches, inspectorPatches))
}

type markSeenRequest struct {
	Scope       string `json:"scope"`
	WindowIndex int    `json:"windowIndex"`
	PaneID      string `json:"paneId"`
}

func decodeMarkSeenRequest(r *http.Request) (markSeenRequest, error) {
	var req markSeenRequest
	if err := decodeJSON(r, &req); err != nil {
		return markSeenRequest{}, err
	}
	req.Scope = strings.TrimSpace(strings.ToLower(req.Scope))
	req.PaneID = strings.TrimSpace(req.PaneID)
	return req, nil
}

func validateMarkSeenRequest(req markSeenRequest) error {
	if req.Scope == "" {
		return errors.New("scope is required")
	}
	switch req.Scope {
	case "pane":
		if !strings.HasPrefix(req.PaneID, "%") {
			return errors.New("paneId must start with %")
		}
	case "window":
		if req.WindowIndex < 0 {
			return errors.New("windowIndex must be >= 0")
		}
	case "session":
	default:
		return errors.New("scope must be pane, window, or session")
	}
	return nil
}

func (h *Handler) applyMarkSeen(ctx context.Context, session string, req markSeenRequest) (bool, error) {
	switch req.Scope {
	case "pane":
		return h.repo.MarkWatchtowerPaneSeen(ctx, session, req.PaneID)
	case "window":
		return h.repo.MarkWatchtowerWindowSeen(ctx, session, req.WindowIndex)
	default:
		return h.repo.MarkWatchtowerSessionSeen(ctx, session)
	}
}

type runtimeValueReader interface {
	GetWatchtowerRuntimeValue(ctx context.Context, key string) (string, error)
}

func readWatchtowerGlobalRev(ctx context.Context, r runtimeValueReader) int64 {
	if r == nil {
		return 0
	}
	raw, err := r.GetWatchtowerRuntimeValue(ctx, "global_rev")
	if err != nil {
		return 0
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func (h *Handler) collectSeenPatches(ctx context.Context, session string) ([]map[string]any, []map[string]any) {
	sessionPatches := make([]map[string]any, 0, 1)
	inspectorPatches := make([]map[string]any, 0, 1)
	if patch, err := h.repo.GetWatchtowerSessionActivityPatch(ctx, session); err == nil {
		sessionPatches = append(sessionPatches, patch)
	}
	if patch, err := h.repo.GetWatchtowerInspectorPatch(ctx, session); err == nil {
		inspectorPatches = append(inspectorPatches, patch)
	}
	return sessionPatches, inspectorPatches
}

func (h *Handler) emitMarkSeenEvents(session, scope string, globalRev int64, sessionPatches, inspectorPatches []map[string]any) {
	h.emit(events.TypeTmuxInspector, map[string]any{
		"session": session,
		"action":  "seen",
		"scope":   scope,
	})
	payload := map[string]any{
		"session":   session,
		"action":    "seen",
		"scope":     scope,
		"globalRev": globalRev,
	}
	if len(sessionPatches) > 0 {
		payload["sessionPatches"] = sessionPatches
	}
	if len(inspectorPatches) > 0 {
		payload["inspectorPatches"] = inspectorPatches
	}
	h.emit(events.TypeTmuxSessions, payload)
}

func buildSeenResponsePayload(session, scope string, acked bool, globalRev int64, sessionPatches, inspectorPatches []map[string]any) map[string]any {
	response := map[string]any{
		"session":   session,
		"scope":     scope,
		"acked":     acked,
		"globalRev": globalRev,
	}
	if len(sessionPatches) > 0 {
		response["sessionPatches"] = sessionPatches
	}
	if len(inspectorPatches) > 0 {
		response["inspectorPatches"] = inspectorPatches
	}
	return response
}

func (h *Handler) selectWindow(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	var req struct {
		Index int `json:"index"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if req.Index < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "index must be >= 0", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.tmuxForSession(session).SelectWindow(ctx, session, req.Index); err != nil {
		writeTmuxError(w, err)
		return
	}
	h.emit(events.TypeTmuxInspector, map[string]any{
		"session": session,
		"action":  "select-window",
		"index":   req.Index,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) selectPane(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	var req struct {
		PaneID string `json:"paneId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.PaneID = strings.TrimSpace(req.PaneID)
	if !strings.HasPrefix(req.PaneID, "%") {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "paneId must start with %", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.ensureSessionPane(ctx, session, req.PaneID); err != nil {
		if tmux.IsKind(err, tmux.ErrKindSessionNotFound) {
			writeTmuxError(w, err)
			return
		}
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "paneId does not belong to session", nil)
		return
	}
	if err := h.tmuxForSession(session).SelectPane(ctx, req.PaneID); err != nil {
		writeTmuxError(w, err)
		return
	}
	h.emit(events.TypeTmuxInspector, map[string]any{
		"session": session,
		"action":  "select-pane",
		"paneId":  req.PaneID,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) renameWindow(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	var req struct {
		Index int    `json:"index"`
		Name  string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Index < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "index must be >= 0", nil)
		return
	}
	if !validate.WindowName(req.Name) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "window name must be 1-64 characters (letters, digits, dots, hyphens, underscores, spaces)", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.tmuxForSession(session).RenameWindow(ctx, session, req.Index, req.Name); err != nil {
		writeTmuxError(w, err)
		return
	}
	if managedWindow, ok, err := h.managedTmuxWindowForIndex(ctx, session, req.Index); err != nil {
		slog.Warn("failed to load managed tmux window after rename", "session", session, "index", req.Index, "err", err)
	} else if ok {
		if err := h.repo.UpdateManagedTmuxWindowName(ctx, managedWindow.ID, req.Name); err != nil {
			slog.Warn("failed to persist managed tmux window name", "session", session, "index", req.Index, "managedWindowId", managedWindow.ID, "err", err)
		}
	}
	h.emit(events.TypeTmuxInspector, map[string]any{
		"session": session,
		"action":  "rename-window",
		"index":   req.Index,
	})
	h.emit(events.TypeTmuxSessions, map[string]any{"session": session, "action": "window-meta"})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) renamePane(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	var req struct {
		PaneID string `json:"paneId"`
		Title  string `json:"title"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.PaneID = strings.TrimSpace(req.PaneID)
	req.Title = strings.TrimSpace(req.Title)
	if !strings.HasPrefix(req.PaneID, "%") {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "paneId must start with %", nil)
		return
	}
	if !validate.PaneTitle(req.Title) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "pane title must be 1-128 characters", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.ensureSessionPane(ctx, session, req.PaneID); err != nil {
		if tmux.IsKind(err, tmux.ErrKindSessionNotFound) {
			writeTmuxError(w, err)
			return
		}
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "paneId does not belong to session", nil)
		return
	}
	if err := h.tmuxForSession(session).RenamePane(ctx, req.PaneID, req.Title); err != nil {
		writeTmuxError(w, err)
		return
	}
	h.emit(events.TypeTmuxInspector, map[string]any{
		"session": session,
		"action":  "rename-pane",
		"paneId":  req.PaneID,
	})
	w.WriteHeader(http.StatusNoContent)
}

func defaultWindowName(sequence int) string {
	if sequence < 1 {
		sequence = 1
	}
	return fmt.Sprintf("win-%d", sequence)
}

func parseNamedSequence(name, prefix string) (int, bool) {
	trimmed := strings.TrimSpace(name)
	if !strings.HasPrefix(trimmed, prefix) {
		return 0, false
	}
	raw := strings.TrimPrefix(trimmed, prefix)
	if raw == "" {
		return 0, false
	}
	seq, err := strconv.Atoi(raw)
	if err != nil || seq < 1 {
		return 0, false
	}
	return seq, true
}

func nextWindowNameSequence(windows []tmux.Window) int {
	next := 1
	for _, window := range windows {
		seq, ok := parseNamedSequence(window.Name, "win-")
		if !ok {
			continue
		}
		if candidate := seq + 1; candidate > next {
			next = candidate
		}
	}
	return next
}

func defaultPaneTitle(paneID string) string {
	suffix := strings.TrimPrefix(strings.TrimSpace(paneID), "%")
	if suffix == "" {
		return "pan-0"
	}
	return "pan-" + suffix
}

func (h *Handler) newWindow(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	var req struct {
		OperationID string `json:"operationId"`
	}
	if err := decodeOptionalJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.OperationID = strings.TrimSpace(req.OperationID)

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "window.create",
		SessionName: session,
		WindowIndex: -1,
	}); !ok {
		return
	}

	svc := h.tmuxForSession(session)
	createdWindow, err := svc.NewWindow(ctx, session)
	if err != nil {
		writeTmuxError(w, err)
		return
	}

	windowNameSequence := createdWindow.Index + 1
	if windowNameSequence < 1 {
		windowNameSequence = 1
	}
	if windows, listErr := svc.ListWindows(ctx, session); listErr != nil {
		slog.Warn("failed to resolve window count for default name", "session", session, "index", createdWindow.Index, "err", listErr)
	} else if next := nextWindowNameSequence(windows); next > windowNameSequence {
		windowNameSequence = next
	}
	if h.repo != nil {
		allocatedSequence, allocErr := h.repo.AllocateNextWindowSequence(ctx, session, windowNameSequence)
		if allocErr != nil {
			slog.Warn("failed to allocate default window sequence", "session", session, "min", windowNameSequence, "err", allocErr)
		} else {
			windowNameSequence = allocatedSequence
		}
	}
	windowName := defaultWindowName(windowNameSequence)
	if err := svc.RenameWindow(ctx, session, createdWindow.Index, windowName); err != nil {
		slog.Warn("failed to apply default window name", "session", session, "index", createdWindow.Index, "name", windowName, "err", err)
	}
	if createdWindow.PaneID != "" {
		paneTitle := defaultPaneTitle(createdWindow.PaneID)
		if err := svc.RenamePane(ctx, createdWindow.PaneID, paneTitle); err != nil {
			slog.Warn("failed to apply default pane title", "session", session, "paneId", createdWindow.PaneID, "title", paneTitle, "err", err)
		}
	}
	inspectorPayload := map[string]any{
		"session": session,
		"action":  "new-window",
		"index":   createdWindow.Index,
		"paneId":  createdWindow.PaneID,
	}
	setOperationID(inspectorPayload, req.OperationID)
	h.emit(events.TypeTmuxInspector, inspectorPayload)
	sessionsPayload := map[string]any{
		"session": session,
		"action":  "window-count",
	}
	setOperationID(sessionsPayload, req.OperationID)
	h.emit(events.TypeTmuxSessions, sessionsPayload)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) killWindow(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	var req struct {
		Index int `json:"index"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if req.Index < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "index must be >= 0", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	managedWindow, hasManagedWindow, managedErr := h.managedTmuxWindowForIndex(ctx, session, req.Index)
	if managedErr != nil {
		slog.Warn("failed to resolve managed tmux window before delete", "session", session, "index", req.Index, "err", managedErr)
	}

	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "window.kill",
		SessionName: session,
		WindowIndex: req.Index,
	}); !ok {
		return
	}

	if err := h.tmuxForSession(session).KillWindow(ctx, session, req.Index); err != nil {
		writeTmuxError(w, err)
		return
	}
	if hasManagedWindow {
		if err := h.repo.DeleteManagedTmuxWindow(ctx, managedWindow.ID); err != nil {
			slog.Warn("failed to delete managed tmux window", "session", session, "index", req.Index, "managedWindowId", managedWindow.ID, "err", err)
		}
	}
	h.emit(events.TypeTmuxInspector, map[string]any{
		"session": session,
		"action":  "kill-window",
		"index":   req.Index,
	})
	h.emit(events.TypeTmuxSessions, map[string]any{"session": session, "action": "window-count"})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) killPane(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	var req struct {
		PaneID string `json:"paneId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.PaneID = strings.TrimSpace(req.PaneID)
	if !strings.HasPrefix(req.PaneID, "%") {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "paneId must start with %", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.ensureSessionPane(ctx, session, req.PaneID); err != nil {
		if tmux.IsKind(err, tmux.ErrKindSessionNotFound) {
			writeTmuxError(w, err)
			return
		}
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "paneId does not belong to session", nil)
		return
	}
	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "pane.kill",
		SessionName: session,
		WindowIndex: -1,
		PaneID:      req.PaneID,
	}); !ok {
		return
	}

	if err := h.tmuxForSession(session).KillPane(ctx, req.PaneID); err != nil {
		writeTmuxError(w, err)
		return
	}
	h.emit(events.TypeTmuxInspector, map[string]any{
		"session": session,
		"action":  "kill-pane",
		"paneId":  req.PaneID,
	})
	h.emit(events.TypeTmuxSessions, map[string]any{"session": session, "action": "pane-count"})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) splitPane(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	var req struct {
		PaneID      string `json:"paneId"`
		Direction   string `json:"direction"`
		OperationID string `json:"operationId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.PaneID = strings.TrimSpace(req.PaneID)
	req.Direction = strings.TrimSpace(strings.ToLower(req.Direction))
	req.OperationID = strings.TrimSpace(req.OperationID)
	if !strings.HasPrefix(req.PaneID, "%") {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "paneId must start with %", nil)
		return
	}
	if req.Direction != "vertical" && req.Direction != "horizontal" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "direction must be vertical or horizontal", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.ensureSessionPane(ctx, session, req.PaneID); err != nil {
		if tmux.IsKind(err, tmux.ErrKindSessionNotFound) {
			writeTmuxError(w, err)
			return
		}
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "paneId does not belong to session", nil)
		return
	}
	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "pane.split",
		SessionName: session,
		WindowIndex: -1,
		PaneID:      req.PaneID,
	}); !ok {
		return
	}

	svc := h.tmuxForSession(session)
	createdPaneID, err := svc.SplitPane(ctx, req.PaneID, req.Direction)
	if err != nil {
		writeTmuxError(w, err)
		return
	}
	if createdPaneID != "" {
		paneTitle := defaultPaneTitle(createdPaneID)
		if err := svc.RenamePane(ctx, createdPaneID, paneTitle); err != nil {
			slog.Warn("failed to apply default pane title", "session", session, "paneId", createdPaneID, "title", paneTitle, "err", err)
		}
	}
	inspectorPayload := map[string]any{
		"session":   session,
		"action":    "split-pane",
		"paneId":    req.PaneID,
		"createdId": createdPaneID,
		"direction": req.Direction,
	}
	setOperationID(inspectorPayload, req.OperationID)
	h.emit(events.TypeTmuxInspector, inspectorPayload)
	sessionsPayload := map[string]any{
		"session": session,
		"action":  "pane-count",
	}
	setOperationID(sessionsPayload, req.OperationID)
	h.emit(events.TypeTmuxSessions, sessionsPayload)
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
		writeData(w, http.StatusOK, map[string]any{"dirs": []string{}})
		return
	}
	writeData(w, http.StatusOK, map[string]any{"dirs": dirs})
}
