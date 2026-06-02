package watchtower

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

type collectSessionState struct {
	service      *Service
	ctx          context.Context
	tmuxOverride tmuxClient
	user         string // "" for default

	sess tmux.Session
	name string
	now  time.Time

	windows []tmux.Window
	panes   []tmux.Pane

	hasExistingSession bool
	existingSession    store.WatchtowerSession
	existingPaneByID   map[string]store.WatchtowerPane
	existingWindowByID map[int]store.WatchtowerWindow
	focusedPanes       map[string]bool
	windowNameByIndex  map[int]string

	windowAgg map[int]*windowAggregate
	paneIDs   []string

	windowIndices []int
	unreadWindows int
	unreadPanes   int

	bestPreview       string
	bestPreviewAt     time.Time
	bestPreviewPaneID string

	anyPaneChanged       bool
	anyWindowChanged     bool
	activeWindowSwitched bool
}

type paneTailSnapshot struct {
	preview    string
	hash       string
	capturedAt time.Time
}

type paneRevisionSnapshot struct {
	revision     int64
	seenRevision int64
	changedAt    time.Time
	changed      bool
}

func (c *collectSessionState) resolveTmuxClient() tmuxClient {
	if c.tmuxOverride != nil {
		return c.tmuxOverride
	}
	return c.service.tmux
}

func (s *Service) prepareCollectSessionState(ctx context.Context, ts taggedSession) (*collectSessionState, bool, error) {
	sess := ts.Session
	name := strings.TrimSpace(sess.Name)
	if name == "" {
		return nil, false, nil
	}

	client := ts.client
	if client == nil {
		client = s.tmux
	}

	windows, err := client.ListWindows(ctx, name)
	if err != nil {
		if tmux.IsKind(err, tmux.ErrKindSessionNotFound) {
			return nil, false, nil
		}
		return nil, true, err
	}
	panes, err := client.ListPanes(ctx, name)
	if err != nil {
		if tmux.IsKind(err, tmux.ErrKindSessionNotFound) {
			return nil, false, nil
		}
		return nil, true, err
	}

	now := time.Now().UTC()
	existingSession, hasExistingSession, err := s.loadExistingSession(ctx, name)
	if err != nil {
		return nil, true, err
	}

	existingPaneByID, err := s.loadExistingPaneByID(ctx, name)
	if err != nil {
		return nil, true, err
	}
	existingWindowByID, err := s.loadExistingWindowByIndex(ctx, name)
	if err != nil {
		return nil, true, err
	}
	focusedPanes, err := s.loadFocusedPanes(ctx, name, now)
	if err != nil {
		return nil, true, err
	}
	managedWindows, err := s.reconcileManagedTmuxWindows(ctx, name, windows)
	if err != nil {
		return nil, true, err
	}
	managedByRuntime := managedWindowRuntimeMap(managedWindows)

	state := &collectSessionState{
		service:      s,
		ctx:          ctx,
		tmuxOverride: client,
		user:         ts.user,
		sess:         sess,
		name:         name,
		now:          now,

		windows: windows,
		panes:   panes,

		hasExistingSession: hasExistingSession,
		existingSession:    existingSession,
		existingPaneByID:   existingPaneByID,
		existingWindowByID: existingWindowByID,
		focusedPanes:       focusedPanes,
		windowNameByIndex:  windowNamesByIndex(windows, managedByRuntime),

		windowAgg: make(map[int]*windowAggregate),
		paneIDs:   make([]string, 0, len(panes)),

		windowIndices: make([]int, 0, len(windows)),

		bestPreview:       strings.TrimSpace(existingSession.LastPreview),
		bestPreviewAt:     existingSession.LastPreviewAt,
		bestPreviewPaneID: strings.TrimSpace(existingSession.LastPreviewPaneID),
	}
	return state, true, nil
}

func (s *Service) loadExistingSession(ctx context.Context, sessionName string) (store.WatchtowerSession, bool, error) {
	row, err := s.store.GetWatchtowerSession(ctx, sessionName)
	if err == nil {
		return row, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return store.WatchtowerSession{}, false, nil
	}
	return store.WatchtowerSession{}, false, err
}

func (s *Service) loadExistingPaneByID(ctx context.Context, sessionName string) (map[string]store.WatchtowerPane, error) {
	rows, err := s.store.ListWatchtowerPanes(ctx, sessionName)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]store.WatchtowerPane, len(rows))
	for _, row := range rows {
		byID[row.PaneID] = row
	}
	return byID, nil
}

func (s *Service) loadExistingWindowByIndex(ctx context.Context, sessionName string) (map[int]store.WatchtowerWindow, error) {
	rows, err := s.store.ListWatchtowerWindows(ctx, sessionName)
	if err != nil {
		return nil, err
	}
	byIndex := make(map[int]store.WatchtowerWindow, len(rows))
	for _, row := range rows {
		byIndex[row.WindowIndex] = row
	}
	return byIndex, nil
}

func (s *Service) loadFocusedPanes(ctx context.Context, sessionName string, now time.Time) (map[string]bool, error) {
	rows, err := s.store.ListWatchtowerPresenceBySession(ctx, sessionName)
	if err != nil {
		return nil, err
	}
	focused := make(map[string]bool, len(rows))
	for _, entry := range rows {
		if !entry.Visible || !entry.Focused {
			continue
		}
		if !entry.ExpiresAt.IsZero() && entry.ExpiresAt.Before(now) {
			continue
		}
		paneID := strings.TrimSpace(entry.PaneID)
		if paneID == "" {
			continue
		}
		focused[paneID] = true
	}
	return focused, nil
}

func (c *collectSessionState) collect() (bool, error) {
	if err := c.collectPanes(); err != nil {
		return false, err
	}
	if err := c.purgePanes(); err != nil {
		return false, err
	}
	if err := c.collectWindows(); err != nil {
		return false, err
	}
	if err := c.purgeWindows(); err != nil {
		return false, err
	}
	return c.persistSession()
}

func (c *collectSessionState) collectPanes() error {
	for _, pane := range c.panes {
		if err := c.collectPane(pane); err != nil {
			return err
		}
	}
	return nil
}

func (c *collectSessionState) collectPane(pane tmux.Pane) error {
	rawPaneID := pane.PaneID
	qualifiedID := qualifyPaneID(c.user, rawPaneID)

	c.paneIDs = append(c.paneIDs, qualifiedID)

	prev, hadPrev := c.existingPaneByID[qualifiedID]
	tail := c.capturePaneTail(rawPaneID, prev, hadPrev)
	revision := c.computePaneRevision(qualifiedID, prev, hadPrev, tail)

	// Use qualified pane ID for store writes, raw for tmux calls.
	qualifiedPane := pane
	qualifiedPane.PaneID = qualifiedID

	c.updateWindowAggregate(pane.WindowIndex, revision)
	c.updateBestPreview(qualifiedID, tail.preview, revision.changedAt)

	return c.service.store.UpsertWatchtowerPane(c.ctx, store.WatchtowerPaneWrite{
		PaneID:         qualifiedID,
		SessionName:    c.name,
		WindowIndex:    pane.WindowIndex,
		PaneIndex:      pane.PaneIndex,
		Title:          pane.Title,
		Active:         pane.Active,
		TTY:            pane.TTY,
		CurrentPath:    pane.CurrentPath,
		StartCommand:   pane.StartCommand,
		CurrentCommand: pane.CurrentCommand,
		TailHash:       tail.hash,
		TailPreview:    tail.preview,
		TailCapturedAt: tail.capturedAt,
		Revision:       revision.revision,
		SeenRevision:   revision.seenRevision,
		ChangedAt:      revision.changedAt,
		UpdatedAt:      c.now,
	})
}

func (c *collectSessionState) capturePaneTail(paneID string, prev store.WatchtowerPane, hadPrev bool) paneTailSnapshot {
	tail := paneTailSnapshot{}

	capCtx, cancel := context.WithTimeout(c.ctx, c.service.options.CaptureTimeout)
	captured, capErr := c.resolveTmuxClient().CapturePaneLines(capCtx, paneID, c.service.options.CaptureLines)
	cancel()

	if capErr == nil {
		tail.preview = normalizePaneTail(captured)
		tail.hash = hashPaneTail(tail.preview)
		tail.capturedAt = c.now
		return tail
	}
	if hadPrev {
		tail.preview = prev.TailPreview
		tail.hash = prev.TailHash
		tail.capturedAt = prev.TailCapturedAt
	}
	return tail
}

func (c *collectSessionState) computePaneRevision(paneID string, prev store.WatchtowerPane, hadPrev bool, tail paneTailSnapshot) paneRevisionSnapshot {
	revision := paneRevisionSnapshot{}
	if hadPrev {
		revision.revision = prev.Revision
		revision.seenRevision = prev.SeenRevision
		revision.changedAt = prev.ChangedAt
	}

	if !hadPrev {
		if tail.hash != "" {
			revision.revision = 1
			revision.changedAt = c.now
			revision.changed = true
		}
	} else if tail.hash != prev.TailHash || tail.preview != prev.TailPreview {
		revision.revision++
		revision.changedAt = c.now
		revision.changed = true
	}

	if revision.changed {
		c.anyPaneChanged = true
		if c.focusedPanes[paneID] {
			revision.seenRevision = revision.revision
		}
	}
	return revision
}

func (c *collectSessionState) updateWindowAggregate(windowIndex int, revision paneRevisionSnapshot) {
	agg := c.windowAgg[windowIndex]
	if agg == nil {
		agg = &windowAggregate{}
		c.windowAgg[windowIndex] = agg
	}
	if revision.revision > revision.seenRevision {
		agg.unreadPanes++
	}
	if !revision.changedAt.IsZero() && revision.changedAt.After(agg.latestAt) {
		agg.latestAt = revision.changedAt
	}
}

func (c *collectSessionState) updateBestPreview(paneID, tailPreview string, changedAt time.Time) {
	if changedAt.IsZero() || !changedAt.After(c.bestPreviewAt) {
		return
	}
	if strings.TrimSpace(tailPreview) == "" {
		return
	}
	c.bestPreview = tailPreview
	c.bestPreviewAt = changedAt
	c.bestPreviewPaneID = paneID
}

func (c *collectSessionState) purgePanes() error {
	return c.service.store.PurgeWatchtowerPanes(c.ctx, c.name, c.paneIDs)
}

func (c *collectSessionState) collectWindows() error {
	for _, win := range c.windows {
		c.windowIndices = append(c.windowIndices, win.Index)

		unread, activityAt, hadPrev, prev := c.windowProjection(win.Index)
		if unread > 0 {
			c.unreadWindows++
		}
		c.unreadPanes += unread

		hasUnread := unread > 0
		windowRev := int64(0)
		if hadPrev {
			windowRev = prev.Rev
		}
		windowChanged := !hadPrev ||
			prev.Name != win.Name ||
			prev.Active != win.Active ||
			prev.Layout != win.Layout ||
			prev.UnreadPanes != unread ||
			prev.HasUnread != hasUnread ||
			!prev.WindowActivityAt.Equal(activityAt)
		if windowChanged {
			windowRev++
			c.anyWindowChanged = true
		}
		if hadPrev && prev.Active != win.Active {
			c.activeWindowSwitched = true
		}
		if windowRev == 0 {
			windowRev = 1
		}

		if err := c.service.store.UpsertWatchtowerWindow(c.ctx, store.WatchtowerWindowWrite{
			SessionName:      c.name,
			TmuxWindowID:     win.ID,
			WindowIndex:      win.Index,
			Name:             win.Name,
			Active:           win.Active,
			Layout:           win.Layout,
			WindowActivityAt: activityAt,
			UnreadPanes:      unread,
			HasUnread:        hasUnread,
			Rev:              windowRev,
			UpdatedAt:        c.now,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (c *collectSessionState) windowProjection(windowIndex int) (int, time.Time, bool, store.WatchtowerWindow) {
	agg := c.windowAgg[windowIndex]
	unread := 0
	activityAt := time.Time{}
	if agg != nil {
		unread = agg.unreadPanes
		activityAt = agg.latestAt
	}

	prev, hadPrev := c.existingWindowByID[windowIndex]
	if activityAt.IsZero() && hadPrev {
		activityAt = prev.WindowActivityAt
	}
	if activityAt.IsZero() {
		activityAt = c.sess.ActivityAt.UTC()
	}
	return unread, activityAt, hadPrev, prev
}

func (c *collectSessionState) purgeWindows() error {
	return c.service.store.PurgeWatchtowerWindows(c.ctx, c.name, c.windowIndices)
}

func (c *collectSessionState) persistSession() (bool, error) {
	sessionRev := int64(0)
	if c.hasExistingSession {
		sessionRev = c.existingSession.Rev
	}
	sessionChanged := c.sessionProjectionChanged()
	if sessionChanged {
		sessionRev++
	}
	if sessionRev == 0 {
		sessionRev = 1
	}

	if err := c.service.store.UpsertWatchtowerSession(c.ctx, store.WatchtowerSessionWrite{
		SessionName:       c.name,
		Attached:          c.sess.Attached,
		Windows:           c.sess.Windows,
		Panes:             len(c.panes),
		ActivityAt:        c.sess.ActivityAt.UTC(),
		LastPreview:       c.bestPreview,
		LastPreviewAt:     c.bestPreviewAt,
		LastPreviewPaneID: c.bestPreviewPaneID,
		UnreadWindows:     c.unreadWindows,
		UnreadPanes:       c.unreadPanes,
		Rev:               sessionRev,
		UpdatedAt:         c.now,
	}); err != nil {
		return false, err
	}

	return sessionChanged, nil
}

func (c *collectSessionState) sessionProjectionChanged() bool {
	checks := []bool{
		!c.hasExistingSession,
		c.existingSession.Attached != c.sess.Attached,
		c.existingSession.Windows != c.sess.Windows,
		c.existingSession.Panes != len(c.panes),
		c.existingSession.UnreadWindows != c.unreadWindows,
		c.existingSession.UnreadPanes != c.unreadPanes,
		c.existingSession.LastPreview != c.bestPreview,
		c.existingSession.LastPreviewPaneID != c.bestPreviewPaneID,
		!c.existingSession.LastPreviewAt.Equal(c.bestPreviewAt),
		!c.existingSession.ActivityAt.Equal(c.sess.ActivityAt.UTC()),
		c.anyPaneChanged,
		c.anyWindowChanged,
	}
	return anyTrue(checks)
}

func anyTrue(values []bool) bool {
	for _, value := range values {
		if value {
			return true
		}
	}
	return false
}
