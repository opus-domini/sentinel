package watchtower

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

const keySessions = "sessions"

const (
	defaultTickInterval   = time.Second
	defaultCaptureLines   = 80
	defaultCaptureTimeout = 150 * time.Millisecond
	defaultJournalRows    = 300
	// journalPruneInterval amortizes the retention DELETE: it removes a whole
	// batch in one statement, so running it every tick scanned the retention
	// index for nothing on the ticks that inserted nothing.
	journalPruneInterval = 50

	runtimeGlobalRevKey = "global_rev"
)

type tmuxClient interface {
	ListSessions(ctx context.Context) ([]tmux.Session, error)
	ListWindows(ctx context.Context, session string) ([]tmux.Window, error)
	ListPanes(ctx context.Context, session string) ([]tmux.Pane, error)
	CapturePaneLines(ctx context.Context, target string, lines int) (string, error)
}

// projectionRepo covers projection writes and purges.
type projectionRepo interface {
	WriteWatchtowerSessionProjection(ctx context.Context, projection store.WatchtowerSessionProjection) error
	PurgeWatchtowerSessions(ctx context.Context, activeSessions []string) error
}

// paneRepo covers pane state reads and presence lookups.
type paneRepo interface {
	ListWatchtowerPanes(ctx context.Context, sessionName string) ([]store.WatchtowerPane, error)
	GetWatchtowerSession(ctx context.Context, sessionName string) (store.WatchtowerSession, error)
	ListWatchtowerWindows(ctx context.Context, sessionName string) ([]store.WatchtowerWindow, error)
	ListWatchtowerPresenceBySession(ctx context.Context, sessionName string) ([]store.WatchtowerPresence, error)
	ListManagedTmuxWindowsBySession(ctx context.Context, sessionName string) ([]store.ManagedTmuxWindow, error)
	UpdateManagedTmuxWindowRuntime(ctx context.Context, id, tmuxWindowID string, lastWindowIndex int) error
	DeleteManagedTmuxWindowsMissingRuntime(ctx context.Context, sessionName string, liveWindowIDs []string) error
}

// journalRepo covers journal insert and prune operations.
type journalRepo interface {
	InsertWatchtowerJournal(ctx context.Context, row store.WatchtowerJournalWrite) (int64, error)
	PruneWatchtowerJournalRows(ctx context.Context, maxRows int) (int64, error)
	PruneWatchtowerPresence(ctx context.Context, now time.Time) (int64, error)
}

// runtimeRepo covers key-value runtime state.
type runtimeRepo interface {
	GetWatchtowerRuntimeValue(ctx context.Context, key string) (string, error)
	SetWatchtowerRuntimeValue(ctx context.Context, key, value string) error
}

// watchtowerStore is the composite data-access interface used by Service.
type watchtowerStore interface {
	projectionRepo
	paneRepo
	journalRepo
	runtimeRepo
}

// Compile-time check: *store.Store satisfies watchtowerStore.
var _ watchtowerStore = (*store.Store)(nil)

// CollectFunc represents collect func data.
type CollectFunc func(ctx context.Context) error

// Options represents options data.
type Options struct {
	TickInterval   time.Duration
	CaptureLines   int
	CaptureTimeout time.Duration
	JournalRows    int
	Collect        CollectFunc
	Publish        func(eventType string, payload map[string]any)

	// UserProvider returns the list of OS users with active multi-user sessions.
	// Called periodically to discover which additional tmux servers to scan.
	// Returns nil or empty when no multi-user sessions exist.
	UserProvider func(ctx context.Context) []string
}

// Service represents service data.
type Service struct {
	store   watchtowerStore
	tmux    tmuxClient
	options Options

	startOnce sync.Once
	stopOnce  sync.Once

	stopFn context.CancelFunc
	doneCh chan struct{}

	// userCache holds the last resolved multi-user list with a TTL.
	userCache     []string
	userCacheTime time.Time

	// journalWrites counts journal rows inserted since the last prune. Only the
	// collector loop touches it, so it needs no lock.
	journalWrites int
}

type windowAggregate struct {
	unreadPanes int
	latestAt    time.Time
}

type taggedSession struct {
	tmux.Session
	client tmuxClient
	user   string // "" for default
}

func (s *Service) resolveUsers(ctx context.Context) []string {
	if s.options.UserProvider == nil {
		return nil
	}
	if time.Since(s.userCacheTime) < 10*time.Second {
		return s.userCache
	}
	users := s.options.UserProvider(ctx)
	s.userCache = users
	s.userCacheTime = time.Now()
	return users
}

func qualifyPaneID(user, paneID string) string {
	if user == "" {
		return paneID
	}
	return user + ":" + paneID
}

// New creates a new service value.
func New(st watchtowerStore, tm tmuxClient, options Options) *Service {
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
	return &Service{
		store:   st,
		tmux:    tm,
		options: options,
	}
}

// Start starts value.
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

// Stop stops value.
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

func (s *Service) collectOnce(ctx context.Context) error {
	if s == nil || s.store == nil || s.tmux == nil {
		return nil
	}
	s.prunePresenceBestEffort(ctx)

	tagged, proceed, err := s.listCollectSessions(ctx)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}

	summary := s.collectSessionsProjection(ctx, tagged)
	if err := s.store.PurgeWatchtowerSessions(ctx, summary.activeSessions); err != nil {
		return err
	}

	globalRev, err := s.persistActivityJournal(ctx, summary.changedSessions)
	if err != nil {
		return err
	}

	// Only prune after rows were actually inserted, and only every
	// journalPruneInterval insertions: the DELETE removes a whole batch in one
	// statement, so running it on every tick scanned the retention index for
	// nothing on the ticks that wrote nothing.
	if len(summary.changedSessions) > 0 {
		s.journalWrites += len(summary.changedSessions)
		if s.journalWrites >= journalPruneInterval {
			s.journalWrites = 0
			s.pruneRetentionBestEffort(ctx)
		}
	}
	s.publishCollectEvents(ctx, summary, globalRev)
	return nil
}

type collectSummary struct {
	activeSessions              []string
	changedSessions             []string
	activeWindowChangedSessions []string
}

func (s *Service) prunePresenceBestEffort(ctx context.Context) {
	if _, err := s.store.PruneWatchtowerPresence(ctx, time.Now().UTC()); err != nil {
		slog.Warn("watchtower prune presence failed", "err", err)
	}
}

func (s *Service) listCollectSessions(ctx context.Context) ([]taggedSession, bool, error) {
	var tagged []taggedSession
	anySourceReachable := false

	sessions, err := s.tmux.ListSessions(ctx)
	if err != nil {
		if !tmux.IsKind(err, tmux.ErrKindServerNotRunning) && !tmux.IsKind(err, tmux.ErrKindNotFound) {
			return nil, false, err
		}
		// Default server not running; still try multi-user below.
	} else {
		anySourceReachable = true
		for _, sess := range sessions {
			tagged = append(tagged, taggedSession{Session: sess, client: s.tmux})
		}
	}

	for _, user := range s.resolveUsers(ctx) {
		userClient := tmux.Service{User: user}
		userSessions, userErr := userClient.ListSessions(ctx)
		if userErr != nil {
			if tmux.IsKind(userErr, tmux.ErrKindServerNotRunning) || tmux.IsKind(userErr, tmux.ErrKindNotFound) {
				slog.Debug("watchtower: user tmux server not running", "user", user)
				continue
			}
			slog.Warn("watchtower: list sessions for user failed", "user", user, "err", userErr)
			continue
		}
		anySourceReachable = true
		for _, sess := range userSessions {
			tagged = append(tagged, taggedSession{Session: sess, client: userClient, user: user})
		}
	}

	if !anySourceReachable {
		return nil, false, nil
	}
	return tagged, true, nil
}

func (s *Service) collectSessionsProjection(ctx context.Context, sessions []taggedSession) collectSummary {
	summary := collectSummary{
		activeSessions:  make([]string, 0, len(sessions)),
		changedSessions: make([]string, 0, len(sessions)),
	}
	for _, ts := range sessions {
		keep, changed, activeWindowSwitched, collectErr := s.collectSession(ctx, ts)
		if collectErr != nil {
			slog.Warn("watchtower collect session failed", "session", ts.Name, "user", ts.user, "err", collectErr)
		}
		if !keep {
			continue
		}
		summary.activeSessions = append(summary.activeSessions, ts.Name)
		if changed {
			summary.changedSessions = append(summary.changedSessions, ts.Name)
		}
		if activeWindowSwitched {
			summary.activeWindowChangedSessions = append(summary.activeWindowChangedSessions, ts.Name)
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
			keySessions:        summary.changedSessions,
			"globalRev":        globalRev,
			"sessionPatches":   sessionPatches,
			"inspectorPatches": inspectorPatches,
		})
		s.options.Publish(events.TypeTmuxActivity, map[string]any{
			"globalRev":        globalRev,
			keySessions:        summary.changedSessions,
			"sessionPatches":   sessionPatches,
			"inspectorPatches": inspectorPatches,
		})
	}

	// Notify the frontend that the active window changed externally so it
	// can schedule a refreshInspector and reconcile any stale overrides.
	for _, sessionName := range summary.activeWindowChangedSessions {
		s.options.Publish(events.TypeTmuxInspector, map[string]any{
			"session": sessionName,
			"action":  "active-window-changed",
		})
	}
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
		managed, managedErr := s.store.ListManagedTmuxWindowsBySession(ctx, sessionName)
		if managedErr != nil {
			slog.Warn("watchtower inspector patch managed windows build failed", "session", sessionName, "err", managedErr)
			managed = nil
		}
		patches = append(patches, store.BuildWatchtowerInspectorPatchWithManaged(sessionName, windows, panes, managedWindowRuntimeMap(managed)))
	}
	return patches
}

// collectSession returns (keep, changed, activeWindowSwitched, err).
func (s *Service) collectSession(ctx context.Context, ts taggedSession) (bool, bool, bool, error) {
	state, keep, err := s.prepareCollectSessionState(ctx, ts)
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
	return true, sessionChanged, state.activeWindowSwitched, nil
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
