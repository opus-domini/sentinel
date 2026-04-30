package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
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
