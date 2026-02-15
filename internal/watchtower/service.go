package watchtower

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

const (
	defaultTickInterval  = time.Second
	defaultCaptureLines  = 80
	defaultCaptureTimout = 150 * time.Millisecond
	defaultJournalRows   = 5000
	defaultTimelineRows  = 20000

	runtimeGlobalRevKey          = "global_rev"
	runtimeCollectTotalKey       = "collect_total"
	runtimeCollectErrorsTotalKey = "collect_errors_total"
	runtimeLastCollectAtKey      = "last_collect_at"
	runtimeLastCollectMSKey      = "last_collect_duration_ms"
	runtimeLastCollectSessKey    = "last_collect_sessions"
	runtimeLastCollectChangedKey = "last_collect_changed_sessions"
	runtimeLastCollectErrorKey   = "last_collect_error"
)

type tmuxClient interface {
	ListSessions(ctx context.Context) ([]tmux.Session, error)
	ListWindows(ctx context.Context, session string) ([]tmux.Window, error)
	ListPanes(ctx context.Context, session string) ([]tmux.Pane, error)
	CapturePaneLines(ctx context.Context, target string, lines int) (string, error)
}

type CollectFunc func(ctx context.Context) error

type Options struct {
	TickInterval   time.Duration
	CaptureLines   int
	CaptureTimeout time.Duration
	JournalRows    int
	TimelineRows   int
	Collect        CollectFunc
	Publish        func(eventType string, payload map[string]any)
}

type Service struct {
	store   *store.Store
	tmux    tmuxClient
	options Options

	startOnce sync.Once
	stopOnce  sync.Once

	stopFn context.CancelFunc
	doneCh chan struct{}
}

type windowAggregate struct {
	unreadPanes int
	latestAt    time.Time
}

func New(st *store.Store, tm tmuxClient, options Options) *Service {
	if options.TickInterval <= 0 {
		options.TickInterval = defaultTickInterval
	}
	if options.CaptureLines <= 0 {
		options.CaptureLines = defaultCaptureLines
	}
	if options.CaptureTimeout <= 0 {
		options.CaptureTimeout = defaultCaptureTimout
	}
	if options.JournalRows <= 0 {
		options.JournalRows = defaultJournalRows
	}
	if options.TimelineRows <= 0 {
		options.TimelineRows = defaultTimelineRows
	}
	return &Service{
		store:   st,
		tmux:    tm,
		options: options,
	}
}

func (s *Service) Start(parent context.Context) {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(parent)
		s.stopFn = cancel
		s.doneCh = make(chan struct{})

		go func() {
			defer close(s.doneCh)
			if err := s.collect(ctx); err != nil {
				slog.Warn("watchtower initial collect failed", "err", err)
			}

			ticker := time.NewTicker(s.options.TickInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := s.collect(ctx); err != nil {
						slog.Warn("watchtower collect failed", "err", err)
					}
				}
			}
		}()
	})
}

func (s *Service) Stop(ctx context.Context) {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.stopFn != nil {
			s.stopFn()
		}
		if s.doneCh == nil {
			return
		}
		select {
		case <-s.doneCh:
		case <-ctx.Done():
		}
	})
}

func (s *Service) collect(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if s.options.Collect != nil {
		return s.options.Collect(ctx)
	}
	return s.collectOnce(ctx)
}

func (s *Service) collectOnce(ctx context.Context) (err error) {
	if s == nil || s.store == nil || s.tmux == nil {
		return nil
	}
	startedAt := time.Now().UTC()
	sessionsCount := 0
	changedCount := 0
	defer func() {
		s.recordCollectMetrics(ctx, startedAt, sessionsCount, changedCount, err)
	}()

	if _, err := s.store.PruneWatchtowerPresence(ctx, time.Now().UTC()); err != nil {
		slog.Warn("watchtower prune presence failed", "err", err)
	}

	sessions, err := s.tmux.ListSessions(ctx)
	if err != nil {
		if tmux.IsKind(err, tmux.ErrKindServerNotRunning) || tmux.IsKind(err, tmux.ErrKindNotFound) {
			return nil
		}
		return err
	}
	sessionsCount = len(sessions)

	active := make([]string, 0, len(sessions))
	changedSessions := make([]string, 0, len(sessions))
	timelineChangedSessions := make(map[string]struct{}, len(sessions))
	for _, sess := range sessions {
		keep, changed, timelineChanged, collectErr := s.collectSession(ctx, sess)
		if collectErr != nil {
			slog.Warn("watchtower collect session failed", "session", sess.Name, "err", collectErr)
		}
		if keep {
			active = append(active, sess.Name)
		}
		if keep && changed {
			changedSessions = append(changedSessions, sess.Name)
		}
		if keep && timelineChanged {
			timelineChangedSessions[sess.Name] = struct{}{}
		}
	}

	if err := s.store.PurgeWatchtowerSessions(ctx, active); err != nil {
		return err
	}
	changedCount = len(changedSessions)

	globalRev := int64(0)
	if len(changedSessions) > 0 {
		currentRev, err := s.currentGlobalRev(ctx)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, sessionName := range changedSessions {
			currentRev++
			if _, err := s.store.InsertWatchtowerJournal(ctx, store.WatchtowerJournalWrite{
				GlobalRev:  currentRev,
				EntityType: "session",
				Session:    sessionName,
				WindowIdx:  -1,
				ChangeKind: "activity",
				ChangedAt:  now,
			}); err != nil {
				return err
			}
		}
		if err := s.store.SetWatchtowerRuntimeValue(ctx, runtimeGlobalRevKey, strconv.FormatInt(currentRev, 10)); err != nil {
			return err
		}
		globalRev = currentRev
	}

	if _, err := s.store.PruneWatchtowerJournalRows(ctx, s.options.JournalRows); err != nil {
		slog.Warn("watchtower prune journal failed", "err", err)
	}
	if _, err := s.store.PruneWatchtowerTimelineRows(ctx, s.options.TimelineRows); err != nil {
		slog.Warn("watchtower prune timeline failed", "err", err)
	}

	if len(changedSessions) > 0 && s.options.Publish != nil {
		sessionPatches := s.buildSessionActivityPatches(ctx, changedSessions)
		inspectorPatches := s.buildInspectorActivityPatches(ctx, changedSessions)
		s.options.Publish(events.TypeTmuxSessions, map[string]any{
			"action":           "activity",
			"sessions":         changedSessions,
			"globalRev":        globalRev,
			"sessionPatches":   sessionPatches,
			"inspectorPatches": inspectorPatches,
		})
		s.options.Publish(events.TypeTmuxActivity, map[string]any{
			"globalRev":        globalRev,
			"sessions":         changedSessions,
			"sessionPatches":   sessionPatches,
			"inspectorPatches": inspectorPatches,
		})
	}
	if len(timelineChangedSessions) > 0 && s.options.Publish != nil {
		sessionsPayload := make([]string, 0, len(timelineChangedSessions))
		for sessionName := range timelineChangedSessions {
			trimmed := strings.TrimSpace(sessionName)
			if trimmed == "" {
				continue
			}
			sessionsPayload = append(sessionsPayload, trimmed)
		}
		if len(sessionsPayload) > 0 {
			sort.Strings(sessionsPayload)
			s.options.Publish(events.TypeTmuxTimeline, map[string]any{
				"sessions": sessionsPayload,
			})
		}
	}
	return nil
}

func (s *Service) buildSessionActivityPatches(ctx context.Context, sessionNames []string) []map[string]any {
	if s == nil || s.store == nil || len(sessionNames) == 0 {
		return nil
	}

	patches := make([]map[string]any, 0, len(sessionNames))
	for _, name := range sessionNames {
		sessionName := strings.TrimSpace(name)
		if sessionName == "" {
			continue
		}
		row, err := s.store.GetWatchtowerSession(ctx, sessionName)
		if err != nil {
			slog.Warn("watchtower session patch build failed", "session", sessionName, "err", err)
			continue
		}
		patches = append(patches, store.BuildWatchtowerSessionActivityPatch(row))
	}
	return patches
}

func (s *Service) buildInspectorActivityPatches(ctx context.Context, sessionNames []string) []map[string]any {
	if s == nil || s.store == nil || len(sessionNames) == 0 {
		return nil
	}

	patches := make([]map[string]any, 0, len(sessionNames))
	for _, name := range sessionNames {
		sessionName := strings.TrimSpace(name)
		if sessionName == "" {
			continue
		}
		windows, winErr := s.store.ListWatchtowerWindows(ctx, sessionName)
		if winErr != nil {
			slog.Warn("watchtower inspector patch windows build failed", "session", sessionName, "err", winErr)
			continue
		}
		panes, paneErr := s.store.ListWatchtowerPanes(ctx, sessionName)
		if paneErr != nil {
			slog.Warn("watchtower inspector patch panes build failed", "session", sessionName, "err", paneErr)
			continue
		}
		patches = append(patches, store.BuildWatchtowerInspectorPatch(sessionName, windows, panes))
	}
	return patches
}

func (s *Service) collectSession(ctx context.Context, sess tmux.Session) (bool, bool, bool, error) {
	name := strings.TrimSpace(sess.Name)
	if name == "" {
		return false, false, false, nil
	}

	windows, err := s.tmux.ListWindows(ctx, name)
	if err != nil {
		if tmux.IsKind(err, tmux.ErrKindSessionNotFound) {
			return false, false, false, nil
		}
		return true, false, false, err
	}
	panes, err := s.tmux.ListPanes(ctx, name)
	if err != nil {
		if tmux.IsKind(err, tmux.ErrKindSessionNotFound) {
			return false, false, false, nil
		}
		return true, false, false, err
	}

	now := time.Now().UTC()

	existingSession, sessionErr := s.store.GetWatchtowerSession(ctx, name)
	hasExistingSession := sessionErr == nil
	if sessionErr != nil && !errors.Is(sessionErr, sql.ErrNoRows) {
		return true, false, false, sessionErr
	}

	existingPanes, err := s.store.ListWatchtowerPanes(ctx, name)
	if err != nil {
		return true, false, false, err
	}
	existingPaneByID := make(map[string]store.WatchtowerPane, len(existingPanes))
	for _, item := range existingPanes {
		existingPaneByID[item.PaneID] = item
	}

	existingWindows, err := s.store.ListWatchtowerWindows(ctx, name)
	if err != nil {
		return true, false, false, err
	}
	existingWindowByIndex := make(map[int]store.WatchtowerWindow, len(existingWindows))
	for _, item := range existingWindows {
		existingWindowByIndex[item.WindowIndex] = item
	}
	windowNameByIndex := make(map[int]string, len(windows))
	for _, window := range windows {
		name := strings.TrimSpace(window.Name)
		if name == "" {
			continue
		}
		windowNameByIndex[window.Index] = name
	}

	presenceRows, err := s.store.ListWatchtowerPresenceBySession(ctx, name)
	if err != nil {
		return true, false, false, err
	}
	focusedPanes := make(map[string]bool, len(presenceRows))
	for _, entry := range presenceRows {
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
		focusedPanes[paneID] = true
	}

	runtimeRows, err := s.store.ListWatchtowerPaneRuntimeBySession(ctx, name)
	if err != nil {
		return true, false, false, err
	}
	runtimeByPaneID := make(map[string]store.WatchtowerPaneRuntime, len(runtimeRows))
	for _, row := range runtimeRows {
		paneID := strings.TrimSpace(row.PaneID)
		if paneID == "" {
			continue
		}
		runtimeByPaneID[paneID] = row
	}

	windowAgg := make(map[int]*windowAggregate)
	paneIDs := make([]string, 0, len(panes))
	bestPreview := strings.TrimSpace(existingSession.LastPreview)
	bestPreviewAt := existingSession.LastPreviewAt
	bestPreviewPaneID := strings.TrimSpace(existingSession.LastPreviewPaneID)
	anyPaneChanged := false
	timelineChanged := false

	appendTimeline := func(event store.WatchtowerTimelineEventWrite) error {
		if _, insertErr := s.store.InsertWatchtowerTimelineEvent(ctx, event); insertErr != nil {
			return insertErr
		}
		timelineChanged = true
		return nil
	}

	for _, pane := range panes {
		paneIDs = append(paneIDs, pane.PaneID)
		command := normalizeRuntimeCommand(pane.CurrentCommand, pane.StartCommand)
		paneTitle := strings.TrimSpace(pane.Title)
		windowName := strings.TrimSpace(windowNameByIndex[pane.WindowIndex])
		if windowName == "" {
			if previousWindow, ok := existingWindowByIndex[pane.WindowIndex]; ok {
				windowName = strings.TrimSpace(previousWindow.Name)
			}
		}
		baseTimelineMetadata := map[string]any{
			"paneIndex": pane.PaneIndex,
			"title":     paneTitle,
		}
		if paneTitle != "" {
			baseTimelineMetadata["paneTitle"] = paneTitle
		}
		if windowName != "" {
			baseTimelineMetadata["windowName"] = windowName
		}

		prev, hadPrev := existingPaneByID[pane.PaneID]
		tailPreview := ""
		tailHash := ""
		tailCapturedAt := time.Time{}

		capCtx, cancel := context.WithTimeout(ctx, s.options.CaptureTimeout)
		captured, capErr := s.tmux.CapturePaneLines(capCtx, pane.PaneID, s.options.CaptureLines)
		cancel()
		if capErr == nil {
			tailPreview = normalizePaneTail(captured)
			tailHash = hashPaneTail(tailPreview)
			tailCapturedAt = now
		} else if hadPrev {
			tailPreview = prev.TailPreview
			tailHash = prev.TailHash
			tailCapturedAt = prev.TailCapturedAt
		}

		revision := int64(0)
		seenRevision := int64(0)
		changedAt := time.Time{}
		if hadPrev {
			revision = prev.Revision
			seenRevision = prev.SeenRevision
			changedAt = prev.ChangedAt
		}

		paneChanged := false
		if !hadPrev {
			if tailHash != "" {
				revision = 1
				changedAt = now
				paneChanged = true
			}
		} else if tailHash != prev.TailHash || tailPreview != prev.TailPreview {
			revision++
			changedAt = now
			paneChanged = true
		}
		if paneChanged {
			anyPaneChanged = true
			if focusedPanes[pane.PaneID] {
				seenRevision = revision
			}
		}
		if paneChanged && strings.TrimSpace(tailPreview) != "" {
			marker, severity, matched := detectTimelineMarker(tailPreview)
			if matched {
				summary := timelineLastLine(tailPreview)
				if summary == "" {
					summary = "output marker detected"
				}
				outputMetadata := make(map[string]any, len(baseTimelineMetadata)+1)
				for key, value := range baseTimelineMetadata {
					outputMetadata[key] = value
				}
				outputMetadata["revision"] = revision
				if err := appendTimeline(store.WatchtowerTimelineEventWrite{
					Session:   name,
					WindowIdx: pane.WindowIndex,
					PaneID:    pane.PaneID,
					EventType: "output.marker",
					Severity:  severity,
					Command:   command,
					Cwd:       pane.CurrentPath,
					Summary:   summary,
					Details:   tailPreview,
					Marker:    marker,
					Metadata:  timelineMetadataJSON(outputMetadata),
					CreatedAt: now,
				}); err != nil {
					return true, false, false, err
				}
			}
		}

		runtime, hadRuntime := runtimeByPaneID[pane.PaneID]
		prevCommand := strings.TrimSpace(runtime.CurrentCommand)
		startedAt := runtime.StartedAt
		if startedAt.IsZero() {
			startedAt = now
		}
		nextStartedAt := startedAt

		if !hadRuntime {
			nextStartedAt = now
			if command != "" && !isShellLikeCommand(command) {
				if err := appendTimeline(store.WatchtowerTimelineEventWrite{
					Session:   name,
					WindowIdx: pane.WindowIndex,
					PaneID:    pane.PaneID,
					EventType: "command.started",
					Severity:  "info",
					Command:   command,
					Cwd:       pane.CurrentPath,
					Summary:   "command started: " + command,
					Metadata:  timelineMetadataJSON(baseTimelineMetadata),
					CreatedAt: now,
				}); err != nil {
					return true, false, false, err
				}
			}
		} else if command != prevCommand {
			if prevCommand != "" && !isShellLikeCommand(prevCommand) {
				durationMS := now.Sub(startedAt).Milliseconds()
				if durationMS < 0 {
					durationMS = 0
				}
				if err := appendTimeline(store.WatchtowerTimelineEventWrite{
					Session:    name,
					WindowIdx:  pane.WindowIndex,
					PaneID:     pane.PaneID,
					EventType:  "command.finished",
					Severity:   "info",
					Command:    prevCommand,
					Cwd:        pane.CurrentPath,
					DurationMS: durationMS,
					Summary:    "command finished: " + prevCommand,
					Metadata:   timelineMetadataJSON(baseTimelineMetadata),
					CreatedAt:  now,
				}); err != nil {
					return true, false, false, err
				}
			}
			nextStartedAt = now
			if command != "" && !isShellLikeCommand(command) {
				if err := appendTimeline(store.WatchtowerTimelineEventWrite{
					Session:   name,
					WindowIdx: pane.WindowIndex,
					PaneID:    pane.PaneID,
					EventType: "command.started",
					Severity:  "info",
					Command:   command,
					Cwd:       pane.CurrentPath,
					Summary:   "command started: " + command,
					Metadata:  timelineMetadataJSON(baseTimelineMetadata),
					CreatedAt: now,
				}); err != nil {
					return true, false, false, err
				}
			}
		}
		if err := s.store.UpsertWatchtowerPaneRuntime(ctx, store.WatchtowerPaneRuntimeWrite{
			PaneID:         pane.PaneID,
			SessionName:    name,
			WindowIdx:      pane.WindowIndex,
			CurrentCommand: command,
			StartedAt:      nextStartedAt,
			UpdatedAt:      now,
		}); err != nil {
			return true, false, false, err
		}

		agg := windowAgg[pane.WindowIndex]
		if agg == nil {
			agg = &windowAggregate{}
			windowAgg[pane.WindowIndex] = agg
		}
		if revision > seenRevision {
			agg.unreadPanes++
		}
		if !changedAt.IsZero() && changedAt.After(agg.latestAt) {
			agg.latestAt = changedAt
		}
		if !changedAt.IsZero() && changedAt.After(bestPreviewAt) && strings.TrimSpace(tailPreview) != "" {
			bestPreview = tailPreview
			bestPreviewAt = changedAt
			bestPreviewPaneID = pane.PaneID
		}

		if err := s.store.UpsertWatchtowerPane(ctx, store.WatchtowerPaneWrite{
			PaneID:         pane.PaneID,
			SessionName:    name,
			WindowIndex:    pane.WindowIndex,
			PaneIndex:      pane.PaneIndex,
			Title:          pane.Title,
			Active:         pane.Active,
			TTY:            pane.TTY,
			CurrentPath:    pane.CurrentPath,
			StartCommand:   pane.StartCommand,
			CurrentCommand: pane.CurrentCommand,
			TailHash:       tailHash,
			TailPreview:    tailPreview,
			TailCapturedAt: tailCapturedAt,
			Revision:       revision,
			SeenRevision:   seenRevision,
			ChangedAt:      changedAt,
			UpdatedAt:      now,
		}); err != nil {
			return true, false, false, err
		}
	}

	if err := s.store.PurgeWatchtowerPanes(ctx, name, paneIDs); err != nil {
		return true, false, false, err
	}
	if err := s.store.PurgeWatchtowerPaneRuntime(ctx, name, paneIDs); err != nil {
		return true, false, false, err
	}

	windowIndices := make([]int, 0, len(windows))
	unreadWindows := 0
	unreadPanes := 0
	anyWindowChanged := false

	for _, win := range windows {
		windowIndices = append(windowIndices, win.Index)

		agg := windowAgg[win.Index]
		unread := 0
		windowActivityAt := time.Time{}
		if agg != nil {
			unread = agg.unreadPanes
			windowActivityAt = agg.latestAt
		}
		if unread > 0 {
			unreadWindows++
		}
		unreadPanes += unread

		prev, hadPrev := existingWindowByIndex[win.Index]
		if windowActivityAt.IsZero() && hadPrev {
			windowActivityAt = prev.WindowActivityAt
		}
		if windowActivityAt.IsZero() {
			windowActivityAt = sess.ActivityAt.UTC()
		}

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
			!prev.WindowActivityAt.Equal(windowActivityAt)
		if windowChanged {
			windowRev++
			anyWindowChanged = true
		}
		if windowRev == 0 {
			windowRev = 1
		}

		if err := s.store.UpsertWatchtowerWindow(ctx, store.WatchtowerWindowWrite{
			SessionName:      name,
			WindowIndex:      win.Index,
			Name:             win.Name,
			Active:           win.Active,
			Layout:           win.Layout,
			WindowActivityAt: windowActivityAt,
			UnreadPanes:      unread,
			HasUnread:        hasUnread,
			Rev:              windowRev,
			UpdatedAt:        now,
		}); err != nil {
			return true, false, false, err
		}
	}

	if err := s.store.PurgeWatchtowerWindows(ctx, name, windowIndices); err != nil {
		return true, false, false, err
	}

	sessionRev := int64(0)
	if hasExistingSession {
		sessionRev = existingSession.Rev
	}
	sessionChanged := !hasExistingSession ||
		existingSession.Attached != sess.Attached ||
		existingSession.Windows != sess.Windows ||
		existingSession.Panes != len(panes) ||
		existingSession.UnreadWindows != unreadWindows ||
		existingSession.UnreadPanes != unreadPanes ||
		existingSession.LastPreview != bestPreview ||
		existingSession.LastPreviewPaneID != bestPreviewPaneID ||
		!existingSession.LastPreviewAt.Equal(bestPreviewAt) ||
		!existingSession.ActivityAt.Equal(sess.ActivityAt.UTC()) ||
		anyPaneChanged ||
		anyWindowChanged
	if sessionChanged {
		sessionRev++
	}
	if sessionRev == 0 {
		sessionRev = 1
	}

	if err := s.store.UpsertWatchtowerSession(ctx, store.WatchtowerSessionWrite{
		SessionName:       name,
		Attached:          sess.Attached,
		Windows:           sess.Windows,
		Panes:             len(panes),
		ActivityAt:        sess.ActivityAt.UTC(),
		LastPreview:       bestPreview,
		LastPreviewAt:     bestPreviewAt,
		LastPreviewPaneID: bestPreviewPaneID,
		UnreadWindows:     unreadWindows,
		UnreadPanes:       unreadPanes,
		Rev:               sessionRev,
		UpdatedAt:         now,
	}); err != nil {
		return true, false, false, err
	}

	return true, sessionChanged, timelineChanged, nil
}

func (s *Service) currentGlobalRev(ctx context.Context) (int64, error) {
	raw, err := s.store.GetWatchtowerRuntimeValue(ctx, runtimeGlobalRevKey)
	if err != nil {
		return 0, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func (s *Service) recordCollectMetrics(ctx context.Context, startedAt time.Time, sessionsCount, changedCount int, collectErr error) {
	if s == nil || s.store == nil {
		return
	}

	durationMS := time.Since(startedAt).Milliseconds()
	s.setRuntimeValueBestEffort(ctx, runtimeLastCollectAtKey, startedAt.Format(time.RFC3339))
	s.setRuntimeValueBestEffort(ctx, runtimeLastCollectMSKey, strconv.FormatInt(durationMS, 10))
	s.setRuntimeValueBestEffort(ctx, runtimeLastCollectSessKey, strconv.Itoa(sessionsCount))
	s.setRuntimeValueBestEffort(ctx, runtimeLastCollectChangedKey, strconv.Itoa(changedCount))
	if collectErr != nil {
		s.setRuntimeValueBestEffort(ctx, runtimeLastCollectErrorKey, collectErr.Error())
	} else {
		s.setRuntimeValueBestEffort(ctx, runtimeLastCollectErrorKey, "")
	}

	s.bumpRuntimeCounterBestEffort(ctx, runtimeCollectTotalKey)
	if collectErr != nil {
		s.bumpRuntimeCounterBestEffort(ctx, runtimeCollectErrorsTotalKey)
	}
}

func (s *Service) setRuntimeValueBestEffort(ctx context.Context, key, value string) {
	if err := s.store.SetWatchtowerRuntimeValue(ctx, key, value); err != nil {
		slog.Warn("watchtower runtime metric write failed", "key", key, "err", err)
	}
}

func (s *Service) bumpRuntimeCounterBestEffort(ctx context.Context, key string) {
	raw, err := s.store.GetWatchtowerRuntimeValue(ctx, key)
	if err != nil {
		slog.Warn("watchtower runtime metric read failed", "key", key, "err", err)
		return
	}
	current := int64(0)
	raw = strings.TrimSpace(raw)
	if raw != "" {
		if parsed, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil {
			current = parsed
		}
	}
	if err := s.store.SetWatchtowerRuntimeValue(ctx, key, strconv.FormatInt(current+1, 10)); err != nil {
		slog.Warn("watchtower runtime metric increment failed", "key", key, "err", err)
	}
}
