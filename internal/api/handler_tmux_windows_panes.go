package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/guardrails"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
	"github.com/opus-domini/sentinel/internal/validate"
)

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
