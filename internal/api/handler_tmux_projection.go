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
	"github.com/opus-domini/sentinel/internal/tmuxlifecycle"
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
		slog.Warn("store.ListWatchtowerWindows failed", keySession, session, "err", err)
		return nil, nil, false
	}
	panes, err := h.repo.ListWatchtowerPanes(ctx, session)
	if err != nil {
		slog.Warn("store.ListWatchtowerPanes failed", keySession, session, "err", err)
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
		slog.Warn("store.ListWatchtowerPanes failed", keySession, session, "err", err)
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
	panes, err := h.tmuxForSession(ctx, session).ListPanes(ctx, session)
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

	snapshot := h.loadEnrichedSessions(ctx)
	if len(snapshot.Sessions) > 0 || snapshot.Status == nowSourceCurrent || snapshot.Status == nowSourceStale {
		writeData(w, http.StatusOK, map[string]any{"sessions": snapshot.Sessions})
		return
	}
	if snapshot.Err != nil {
		writeTmuxError(w, snapshot.Err)
		return
	}
	writeError(w, http.StatusInternalServerError, string(tmux.ErrKindCommandFailed), "tmux command failed", nil)
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

type enrichedSessionsSnapshot struct {
	Sessions   []enrichedSession
	Status     string
	Message    string
	ObservedAt time.Time
	Err        error
}

// loadEnrichedSessions is the single session projection used by both the Tmux
// owner page and Now. It preserves Watchtower unread state, overlays live
// runtime sessions, and reports whether the result is fresh or only a usable
// persisted projection.
func (h *Handler) loadEnrichedSessions(ctx context.Context) enrichedSessionsSnapshot {
	stored := h.loadSessionMetaMap(ctx)
	projected := []store.WatchtowerSession{}
	if h.repo != nil {
		rows, err := h.repo.ListWatchtowerSessions(ctx)
		if err != nil {
			slog.Warn("store.ListWatchtowerSessions failed", "err", err)
		} else {
			projected = rows
		}
	}

	seen := make(map[string]struct{}, len(projected))
	activeNames := make([]string, 0, len(projected))
	result := make([]enrichedSession, 0, len(projected))
	for _, row := range projected {
		seen[row.SessionName] = struct{}{}
		activeNames = append(activeNames, row.SessionName)
		result = append(result, h.projectedSessionToEnriched(ctx, row, stored[row.SessionName]))
	}

	liveSessionIDs := make(map[string]string)
	runtimeAvailable := make(map[string]bool)
	liveSessions, runtimeErr := h.tmux.ListSessions(ctx)
	if runtimeErr == nil {
		runtimeAvailable[""] = true
		snapshots := h.loadActivePaneSnapshots(ctx)
		for _, sess := range liveSessions {
			liveSessionIDs[lifecycleSessionKey("", sess.Name)] = sess.ID
			if _, exists := seen[sess.Name]; exists {
				continue
			}
			seen[sess.Name] = struct{}{}
			activeNames = append(activeNames, sess.Name)
			result = append(result, h.tmuxSessionToEnriched(ctx, sess, snapshots[sess.Name], stored[sess.Name]))
		}
	}

	result, activeNames = h.appendKnownUserSessions(
		ctx,
		result,
		activeNames,
		seen,
		stored,
		liveSessionIDs,
		runtimeAvailable,
	)
	h.overlaySessionLifecycles(result, liveSessionIDs, runtimeAvailable)

	if runtimeErr == nil || len(projected) > 0 {
		h.purgeStoredSessionsBestEffort(ctx, activeNames)
	}
	sortSessionsByStoredOrder(result)

	snapshot := enrichedSessionsSnapshot{
		Sessions:   result,
		Status:     nowSourceCurrent,
		ObservedAt: time.Now().UTC(),
	}
	switch {
	case runtimeErr == nil:
		return snapshot
	case tmux.IsKind(runtimeErr, tmux.ErrKindNotFound):
		snapshot.Status = nowSourceNotConfigured
		snapshot.Message = "tmux_not_installed"
	case tmux.IsKind(runtimeErr, tmux.ErrKindServerNotRunning) && len(projected) == 0:
		// An installed tmux binary with no server is a fresh, empty runtime.
		snapshot.Status = nowSourceCurrent
	case len(projected) > 0:
		snapshot.Status = nowSourceStale
		snapshot.Message = "tmux_projection_stale"
		snapshot.ObservedAt = time.Time{}
		for _, row := range projected {
			if row.UpdatedAt.After(snapshot.ObservedAt) {
				snapshot.ObservedAt = row.UpdatedAt.UTC()
			}
		}
		if snapshot.ObservedAt.IsZero() {
			snapshot.ObservedAt = time.Now().UTC()
		}
	default:
		snapshot.Status = nowSourceUnavailable
		snapshot.Message = "tmux_unavailable"
	}
	snapshot.Err = runtimeErr
	return snapshot
}

func (h *Handler) appendKnownUserSessions(
	ctx context.Context,
	result []enrichedSession,
	activeNames []string,
	seen map[string]struct{},
	stored map[string]store.SessionMeta,
	liveSessionIDs map[string]string,
	runtimeAvailable map[string]bool,
) ([]enrichedSession, []string) {
	for _, user := range h.knownSessionUsers() {
		svc := tmux.Service{User: user}
		userSessions, listErr := svc.ListSessions(ctx)
		if listErr != nil {
			slog.Warn("multi-user session list failed", "user", user, "err", listErr)
			continue
		}
		runtimeAvailable[user] = true
		userSnapshots, _ := svc.ListActivePaneCommands(ctx)
		for _, sess := range userSessions {
			liveSessionIDs[lifecycleSessionKey(user, sess.Name)] = sess.ID
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
	return result, activeNames
}

func (h *Handler) overlaySessionLifecycles(
	sessions []enrichedSession,
	liveSessionIDs map[string]string,
	runtimeAvailable map[string]bool,
) {
	if h.lifecycle == nil {
		return
	}

	byTarget := make(map[string]tmuxlifecycle.Snapshot)
	byName := make(map[string]tmuxlifecycle.Snapshot)
	for _, snapshot := range h.lifecycle.Snapshot() {
		byTarget[lifecycleTargetKey(snapshot.User, snapshot.SessionID)] = snapshot
		byName[lifecycleSessionKey(snapshot.User, snapshot.SessionName)] = snapshot
	}

	for index := range sessions {
		user := strings.TrimSpace(sessions[index].User)
		nameKey := lifecycleSessionKey(user, sessions[index].Name)
		var (
			snapshot tmuxlifecycle.Snapshot
			found    bool
		)
		if runtimeAvailable[user] {
			sessionID := liveSessionIDs[nameKey]
			if sessionID != "" {
				snapshot, found = byTarget[lifecycleTargetKey(user, sessionID)]
			}
		} else {
			snapshot, found = byName[nameKey]
		}
		if found {
			sessions[index].Lifecycle = lifecycleProjection(snapshot)
		}
	}
}

func lifecycleProjection(snapshot tmuxlifecycle.Snapshot) *enrichedSessionLifecycle {
	projection := &enrichedSessionLifecycle{
		Mode:         "ephemeral",
		Source:       snapshot.Source,
		CleanupState: snapshot.State,
		ExpiresAt:    snapshot.ExpiresAt.UTC().Format(time.RFC3339),
	}
	if !snapshot.GraceUntil.IsZero() {
		projection.GraceUntil = snapshot.GraceUntil.UTC().Format(time.RFC3339)
	}
	return projection
}

func lifecycleSessionKey(user, sessionName string) string {
	return strings.TrimSpace(user) + "\x00" + strings.TrimSpace(sessionName)
}

func lifecycleTargetKey(user, sessionID string) string {
	return strings.TrimSpace(user) + "\x00" + strings.TrimSpace(sessionID)
}

func (h *Handler) projectedSessionToEnriched(ctx context.Context, row store.WatchtowerSession, meta store.SessionMeta) enrichedSession {
	hash := strings.TrimSpace(meta.Hash)
	lastContent := strings.TrimSpace(row.LastPreview)
	if lastContent == "" {
		lastContent = strings.TrimSpace(meta.LastContent)
	}
	if hash == "" {
		hash = tmux.SessionHash(row.SessionName, row.ActivityAt.Unix())
		h.initSessionMetaBestEffort(ctx, row.SessionName, hash, lastContent)
	}
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
	lastContent := h.resolveSessionLastContent(ctx, sess.Name, meta.LastContent)
	if hash == "" {
		hash = tmux.SessionHash(sess.Name, sess.CreatedAt.Unix())
		h.initSessionMetaBestEffort(ctx, sess.Name, hash, lastContent)
	}

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
	captured, err := h.tmuxForSession(ctx, sessionName).CapturePane(ctx, sessionName)
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
	paneList, err := h.tmuxForSession(ctx, sessionName).ListPanes(ctx, sessionName)
	if err != nil {
		return windowFallback
	}
	return len(paneList)
}

// initSessionMetaBestEffort persists the session hash the first time a session
// is seen. Listing sessions is otherwise a read: re-upserting every row on every
// GET /api/tmux/sessions and /api/now cost one write transaction per session per
// request on the single SQLite connection, and rewrote values that were already
// identical. last_preview belongs to the watchtower, so it is only seeded here.
func (h *Handler) initSessionMetaBestEffort(ctx context.Context, sessionName, hash, lastContent string) {
	if h.repo == nil {
		return
	}
	if err := h.repo.UpsertSession(ctx, sessionName, hash, lastContent); err != nil {
		slog.Warn("store.UpsertSession failed", keySession, sessionName, "err", err)
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
