package watchtower

import (
	"context"
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
	defaultTickInterval   = time.Second
	defaultCaptureLines   = 80
	defaultCaptureTimeout = 150 * time.Millisecond
	defaultJournalRows    = 5000
	defaultTimelineRows   = 20000

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

// OpsTimelineFunc is called to record significant watchtower events in the ops timeline.
type OpsTimelineFunc func(ctx context.Context, source, eventType, severity, resource, message, details string)

type Options struct {
	TickInterval   time.Duration
	CaptureLines   int
	CaptureTimeout time.Duration
	JournalRows    int
	TimelineRows   int
	Collect        CollectFunc
	Publish        func(eventType string, payload map[string]any)
	OpsTimeline    OpsTimelineFunc
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
		options.CaptureTimeout = defaultCaptureTimeout
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

	s.prunePresenceBestEffort(ctx)

	sessions, proceed, err := s.listCollectSessions(ctx)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}
	sessionsCount = len(sessions)

	summary := s.collectSessionsProjection(ctx, sessions)
	if err := s.store.PurgeWatchtowerSessions(ctx, summary.activeSessions); err != nil {
		return err
	}
	changedCount = len(summary.changedSessions)

	globalRev, err := s.persistActivityJournal(ctx, summary.changedSessions)
	if err != nil {
		return err
	}

	s.pruneRetentionBestEffort(ctx)
	s.publishCollectEvents(ctx, summary, globalRev)
	return nil
}

type collectSummary struct {
	activeSessions          []string
	changedSessions         []string
	timelineChangedSessions map[string]struct{}
}

func (s *Service) prunePresenceBestEffort(ctx context.Context) {
	if _, err := s.store.PruneWatchtowerPresence(ctx, time.Now().UTC()); err != nil {
		slog.Warn("watchtower prune presence failed", "err", err)
	}
}

func (s *Service) listCollectSessions(ctx context.Context) ([]tmux.Session, bool, error) {
	sessions, err := s.tmux.ListSessions(ctx)
	if err != nil {
		if tmux.IsKind(err, tmux.ErrKindServerNotRunning) || tmux.IsKind(err, tmux.ErrKindNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return sessions, true, nil
}

func (s *Service) collectSessionsProjection(ctx context.Context, sessions []tmux.Session) collectSummary {
	summary := collectSummary{
		activeSessions:          make([]string, 0, len(sessions)),
		changedSessions:         make([]string, 0, len(sessions)),
		timelineChangedSessions: make(map[string]struct{}, len(sessions)),
	}
	for _, sess := range sessions {
		keep, changed, timelineChanged, collectErr := s.collectSession(ctx, sess)
		if collectErr != nil {
			slog.Warn("watchtower collect session failed", "session", sess.Name, "err", collectErr)
		}
		if !keep {
			continue
		}
		summary.activeSessions = append(summary.activeSessions, sess.Name)
		if changed {
			summary.changedSessions = append(summary.changedSessions, sess.Name)
		}
		if timelineChanged {
			summary.timelineChangedSessions[sess.Name] = struct{}{}
		}
	}
	return summary
}

func (s *Service) persistActivityJournal(ctx context.Context, changedSessions []string) (int64, error) {
	if len(changedSessions) == 0 {
		return 0, nil
	}

	currentRev, err := s.currentGlobalRev(ctx)
	if err != nil {
		return 0, err
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
			return 0, err
		}
	}
	if err := s.store.SetWatchtowerRuntimeValue(ctx, runtimeGlobalRevKey, strconv.FormatInt(currentRev, 10)); err != nil {
		return 0, err
	}
	return currentRev, nil
}

func (s *Service) pruneRetentionBestEffort(ctx context.Context) {
	if _, err := s.store.PruneWatchtowerJournalRows(ctx, s.options.JournalRows); err != nil {
		slog.Warn("watchtower prune journal failed", "err", err)
	}
	if _, err := s.store.PruneWatchtowerTimelineRows(ctx, s.options.TimelineRows); err != nil {
		slog.Warn("watchtower prune timeline failed", "err", err)
	}
}

func (s *Service) publishCollectEvents(ctx context.Context, summary collectSummary, globalRev int64) {
	if s.options.Publish == nil {
		return
	}

	if len(summary.changedSessions) > 0 {
		sessionPatches := s.buildSessionActivityPatches(ctx, summary.changedSessions)
		inspectorPatches := s.buildInspectorActivityPatches(ctx, summary.changedSessions)
		s.options.Publish(events.TypeTmuxSessions, map[string]any{
			"action":           "activity",
			"sessions":         summary.changedSessions,
			"globalRev":        globalRev,
			"sessionPatches":   sessionPatches,
			"inspectorPatches": inspectorPatches,
		})
		s.options.Publish(events.TypeTmuxActivity, map[string]any{
			"globalRev":        globalRev,
			"sessions":         summary.changedSessions,
			"sessionPatches":   sessionPatches,
			"inspectorPatches": inspectorPatches,
		})
	}

	// Emit ops timeline events for timeline-significant changes.
	if s.options.OpsTimeline != nil && len(summary.timelineChangedSessions) > 0 {
		for sessionName := range summary.timelineChangedSessions {
			s.options.OpsTimeline(ctx, "watchtower", "session.activity", "info",
				sessionName, "Terminal activity detected in "+sessionName, "")
		}
	}

	sessionsPayload := sortedNonEmptySessionNames(summary.timelineChangedSessions)
	if len(sessionsPayload) == 0 {
		return
	}
	s.options.Publish(events.TypeTmuxTimeline, map[string]any{
		"sessions": sessionsPayload,
	})
}

func sortedNonEmptySessionNames(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for sessionName := range values {
		trimmed := strings.TrimSpace(sessionName)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
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
	state, keep, err := s.prepareCollectSessionState(ctx, sess)
	if err != nil {
		return keep, false, false, err
	}
	if !keep {
		return false, false, false, nil
	}
	sessionChanged, err := state.collect()
	if err != nil {
		return true, false, false, err
	}
	return true, sessionChanged, state.timelineChanged, nil
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
