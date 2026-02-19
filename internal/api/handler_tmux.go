package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/guardrails"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
	"github.com/opus-domini/sentinel/internal/validate"
)

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

	activeNames := make([]string, 0, len(projected))
	result := make([]enrichedSession, 0, len(projected))
	for _, row := range projected {
		activeNames = append(activeNames, row.SessionName)
		result = append(result, h.projectedSessionToEnriched(ctx, row, stored[row.SessionName]))
	}
	h.purgeStoredSessionsBestEffort(ctx, activeNames)
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

	activeNames := make([]string, 0, len(sessions))
	result := make([]enrichedSession, 0, len(sessions))
	for _, sess := range sessions {
		activeNames = append(activeNames, sess.Name)
		result = append(result, h.tmuxSessionToEnriched(ctx, sess, snapshots[sess.Name], stored[sess.Name]))
	}
	h.purgeStoredSessionsBestEffort(ctx, activeNames)
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
		UnreadWindows: 0,
		UnreadPanes:   0,
		Rev:           0,
	}
}

func (h *Handler) resolveSessionLastContent(ctx context.Context, sessionName, fallback string) string {
	lastContent := strings.TrimSpace(fallback)
	captured, err := h.tmux.CapturePane(ctx, sessionName)
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
	paneList, err := h.tmux.ListPanes(ctx, sessionName)
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
		Name string `json:"name"`
		Cwd  string `json:"cwd"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Cwd = strings.TrimSpace(req.Cwd)
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

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "session.create",
		SessionName: req.Name,
		WindowIndex: -1,
	}); !ok {
		return
	}

	if err := h.tmux.CreateSession(ctx, req.Name, req.Cwd); err != nil {
		writeTmuxError(w, err)
		return
	}
	h.emit(events.TypeTmuxSessions, map[string]any{"session": req.Name, "action": "create"})
	writeData(w, http.StatusCreated, map[string]any{"name": req.Name})
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

	if err := h.tmux.RenameSession(ctx, session, req.NewName); err != nil {
		writeTmuxError(w, err)
		return
	}
	if err := h.repo.Rename(ctx, session, req.NewName); err != nil {
		slog.Warn("store.Rename failed", "from", session, "to", req.NewName, "err", err)
	}
	if err := h.repo.RenameRecoverySession(ctx, session, req.NewName); err != nil {
		slog.Warn("store.RenameRecoverySession failed", "from", session, "to", req.NewName, "err", err)
	}
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

	if err := h.tmux.KillSession(ctx, session); err != nil {
		writeTmuxError(w, err)
		return
	}
	if h.recovery != nil {
		if err := h.recovery.ArchiveSession(ctx, session); err != nil {
			slog.Warn("recovery archive on kill failed", "session", session, "err", err)
		}
	}
	h.emit(events.TypeTmuxSessions, map[string]any{"session": session, "action": "delete"})
	h.emit(events.TypeRecoveryOverview, map[string]any{"session": session, "action": "archive"})
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

	if h.repo != nil {
		projected, err := h.repo.ListWatchtowerWindows(ctx, session)
		if err != nil {
			slog.Warn("store.ListWatchtowerWindows failed", "session", session, "err", err)
		} else {
			_, sessionErr := h.repo.GetWatchtowerSession(ctx, session)
			hasSession := sessionErr == nil
			if len(projected) > 0 || hasSession {
				panes, panesErr := h.repo.ListWatchtowerPanes(ctx, session)
				if panesErr != nil {
					writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to list windows", nil)
					return
				}

				paneCounts := make(map[int]int, len(projected))
				for _, pane := range panes {
					paneCounts[pane.WindowIndex]++
				}

				resp := make([]enrichedWindow, 0, len(projected))
				for _, row := range projected {
					resp = append(resp, enrichedWindow{
						Session:     row.SessionName,
						Index:       row.WindowIndex,
						Name:        row.Name,
						Active:      row.Active,
						Panes:       paneCounts[row.WindowIndex],
						Layout:      row.Layout,
						UnreadPanes: row.UnreadPanes,
						HasUnread:   row.HasUnread,
						Rev:         row.Rev,
						ActivityAt:  row.WindowActivityAt.Format(time.RFC3339),
					})
				}
				writeData(w, http.StatusOK, map[string]any{"windows": resp})
				return
			}
		}
	}

	windows, err := h.tmux.ListWindows(ctx, session)
	if err != nil {
		writeTmuxError(w, err)
		return
	}
	resp := make([]enrichedWindow, 0, len(windows))
	for _, row := range windows {
		resp = append(resp, enrichedWindow{
			Session:     row.Session,
			Index:       row.Index,
			Name:        row.Name,
			Active:      row.Active,
			Panes:       row.Panes,
			Layout:      row.Layout,
			UnreadPanes: 0,
			HasUnread:   false,
			Rev:         0,
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

	if h.repo != nil {
		projected, err := h.repo.ListWatchtowerPanes(ctx, session)
		if err != nil {
			slog.Warn("store.ListWatchtowerPanes failed", "session", session, "err", err)
		} else {
			_, sessionErr := h.repo.GetWatchtowerSession(ctx, session)
			hasSession := sessionErr == nil
			if len(projected) > 0 || hasSession {
				resp := make([]enrichedPane, 0, len(projected))
				for _, row := range projected {
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
				writeData(w, http.StatusOK, map[string]any{"panes": resp})
				return
			}
		}
	}

	panes, err := h.tmux.ListPanes(ctx, session)
	if err != nil {
		writeTmuxError(w, err)
		return
	}
	resp := make([]enrichedPane, 0, len(panes))
	for _, row := range panes {
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
			Revision:       0,
			SeenRevision:   0,
			HasUnread:      false,
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

	if err := h.tmux.SelectWindow(ctx, session, req.Index); err != nil {
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

	if err := h.tmux.SelectPane(ctx, req.PaneID); err != nil {
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

	if err := h.tmux.RenameWindow(ctx, session, req.Index, req.Name); err != nil {
		writeTmuxError(w, err)
		return
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

	if err := h.tmux.RenamePane(ctx, req.PaneID, req.Title); err != nil {
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

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "window.create",
		SessionName: session,
		WindowIndex: -1,
	}); !ok {
		return
	}

	createdWindow, err := h.tmux.NewWindow(ctx, session)
	if err != nil {
		writeTmuxError(w, err)
		return
	}

	windowNameSequence := createdWindow.Index + 1
	if windowNameSequence < 1 {
		windowNameSequence = 1
	}
	if windows, listErr := h.tmux.ListWindows(ctx, session); listErr != nil {
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
	if err := h.tmux.RenameWindow(ctx, session, createdWindow.Index, windowName); err != nil {
		slog.Warn("failed to apply default window name", "session", session, "index", createdWindow.Index, "name", windowName, "err", err)
	}
	if createdWindow.PaneID != "" {
		paneTitle := defaultPaneTitle(createdWindow.PaneID)
		if err := h.tmux.RenamePane(ctx, createdWindow.PaneID, paneTitle); err != nil {
			slog.Warn("failed to apply default pane title", "session", session, "paneId", createdWindow.PaneID, "title", paneTitle, "err", err)
		}
	}
	h.emit(events.TypeTmuxInspector, map[string]any{
		"session": session,
		"action":  "new-window",
		"index":   createdWindow.Index,
		"paneId":  createdWindow.PaneID,
	})
	h.emit(events.TypeTmuxSessions, map[string]any{"session": session, "action": "window-count"})
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

	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "window.kill",
		SessionName: session,
		WindowIndex: req.Index,
	}); !ok {
		return
	}

	if err := h.tmux.KillWindow(ctx, session, req.Index); err != nil {
		writeTmuxError(w, err)
		return
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

	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "pane.kill",
		SessionName: session,
		WindowIndex: -1,
		PaneID:      req.PaneID,
	}); !ok {
		return
	}

	if err := h.tmux.KillPane(ctx, req.PaneID); err != nil {
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
		PaneID    string `json:"paneId"`
		Direction string `json:"direction"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.PaneID = strings.TrimSpace(req.PaneID)
	req.Direction = strings.TrimSpace(strings.ToLower(req.Direction))
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

	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "pane.split",
		SessionName: session,
		WindowIndex: -1,
		PaneID:      req.PaneID,
	}); !ok {
		return
	}

	createdPaneID, err := h.tmux.SplitPane(ctx, req.PaneID, req.Direction)
	if err != nil {
		writeTmuxError(w, err)
		return
	}
	if createdPaneID != "" {
		paneTitle := defaultPaneTitle(createdPaneID)
		if err := h.tmux.RenamePane(ctx, createdPaneID, paneTitle); err != nil {
			slog.Warn("failed to apply default pane title", "session", session, "paneId", createdPaneID, "title", paneTitle, "err", err)
		}
	}
	h.emit(events.TypeTmuxInspector, map[string]any{
		"session":   session,
		"action":    "split-pane",
		"paneId":    req.PaneID,
		"createdId": createdPaneID,
		"direction": req.Direction,
	})
	h.emit(events.TypeTmuxSessions, map[string]any{"session": session, "action": "pane-count"})
	w.WriteHeader(http.StatusNoContent)
}
