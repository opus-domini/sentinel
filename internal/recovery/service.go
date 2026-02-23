package recovery

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/opus-domini/sentinel/internal/alerts"
	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

const (
	runtimeBootIDKey       = "recovery.last_boot_id"
	runtimeCollectAtKey    = "recovery.last_collect_at"
	runtimeBootChangeKey   = "recovery.last_boot_change_at"
	runtimeLiveSessionsKey = "recovery.last_live_sessions"
	defaultSnapshotPeriod  = 5 * time.Second
	defaultMaxSnapshots    = 300
)

// recoveryStore sub-interfaces — each has at most 5 methods.

type runtimeKV interface {
	GetRuntimeValue(ctx context.Context, key string) (string, error)
	SetRuntimeValue(ctx context.Context, key, value string) error
}

type snapshotRepo interface {
	UpsertRecoverySnapshot(ctx context.Context, snap store.RecoverySnapshotWrite) (store.RecoverySnapshot, bool, error)
	TrimRecoverySnapshots(ctx context.Context, maxPerSession int) error
	GetRecoverySnapshot(ctx context.Context, id int64) (store.RecoverySnapshot, error)
	ListRecoverySnapshots(ctx context.Context, sessionName string, limit int) ([]store.RecoverySnapshot, error)
}

type sessionRepo interface {
	ListRecoverySessions(ctx context.Context, states []store.RecoverySessionState) ([]store.RecoverySession, error)
	MarkRecoverySessionsKilled(ctx context.Context, names []string, bootID string, killedAt time.Time) error
	MarkRecoverySessionRestoring(ctx context.Context, name string) error
	MarkRecoverySessionRestored(ctx context.Context, name string, restoredAt time.Time) error
	MarkRecoverySessionArchived(ctx context.Context, name string, archivedAt time.Time) error
}

type restoreRepo interface {
	MarkRecoverySessionRestoreFailed(ctx context.Context, name, errMsg string) error
	UpdateRecoveryJobProgress(ctx context.Context, id string, completedSteps, totalSteps int, currentStep string) error
	UpdateRecoveryJobTarget(ctx context.Context, id, target string) error
}

type jobRepo interface {
	CreateRecoveryJob(ctx context.Context, job store.RecoveryJob) error
	GetRecoveryJob(ctx context.Context, id string) (store.RecoveryJob, error)
	ListRecoveryJobs(ctx context.Context, statuses []store.RecoveryJobStatus, limit int) ([]store.RecoveryJob, error)
	SetRecoveryJobRunning(ctx context.Context, id string, startedAt time.Time) error
	FinishRecoveryJob(ctx context.Context, id string, status store.RecoveryJobStatus, errMsg string, finishedAt time.Time) error
}

type watchtowerReader interface {
	GetWatchtowerSession(ctx context.Context, sessionName string) (store.WatchtowerSession, error)
	ListWatchtowerWindows(ctx context.Context, sessionName string) ([]store.WatchtowerWindow, error)
	ListWatchtowerPanes(ctx context.Context, sessionName string) ([]store.WatchtowerPane, error)
}

type recoveryStore interface {
	runtimeKV
	snapshotRepo
	sessionRepo
	restoreRepo
	jobRepo
	watchtowerReader
}

var _ recoveryStore = (*store.Store)(nil)

type tmuxClient interface {
	ListSessions(ctx context.Context) ([]tmux.Session, error)
	ListWindows(ctx context.Context, session string) ([]tmux.Window, error)
	ListPanes(ctx context.Context, session string) ([]tmux.Pane, error)
	SessionExists(ctx context.Context, session string) (bool, error)
	CreateSession(ctx context.Context, name, cwd string) error
	RenameWindow(ctx context.Context, session string, index int, name string) error
	NewWindowAt(ctx context.Context, session string, index int, name, cwd string) error
	SplitPaneIn(ctx context.Context, paneID, direction, cwd string) (string, error)
	SelectLayout(ctx context.Context, session string, index int, layout string) error
	SelectWindow(ctx context.Context, session string, index int) error
	SelectPane(ctx context.Context, paneID string) error
	RenamePane(ctx context.Context, paneID, title string) error
	SendKeys(ctx context.Context, paneID, keys string, enter bool) error
	KillSession(ctx context.Context, session string) error
}

// AlertRepo is an optional interface for raising/resolving recovery alerts.
type AlertRepo interface {
	UpsertAlert(ctx context.Context, write alerts.AlertWrite) (alerts.Alert, error)
	ResolveAlert(ctx context.Context, dedupeKey string, at time.Time) (alerts.Alert, error)
}

type Options struct {
	SnapshotInterval    time.Duration
	MaxSnapshotsPerSess int
	EventHub            *events.Hub
	AlertRepo           AlertRepo
	BootRestore         string // "off", "safe", "confirm", "full"; empty = "off"
}

type Overview struct {
	BootID         string                  `json:"bootId"`
	LastBootID     string                  `json:"lastBootId"`
	LastCollectAt  time.Time               `json:"lastCollectAt"`
	LastBootChange time.Time               `json:"lastBootChange"`
	KilledSessions []store.RecoverySession `json:"killedSessions"`
	RunningJobs    []store.RecoveryJob     `json:"runningJobs"`
}

type SnapshotView struct {
	Meta    store.RecoverySnapshot `json:"meta"`
	Payload SessionSnapshot        `json:"payload"`
}

type Service struct {
	store   recoveryStore
	tmux    tmuxClient
	options Options
	bootID  func(context.Context) string
	events  *events.Hub

	startOnce sync.Once
	stopOnce  sync.Once
	stopFn    context.CancelFunc
	doneCh    chan struct{}

	// runCtx is the parent context for restore-job goroutines.
	runCtx    context.Context
	runCancel context.CancelFunc
	wg        sync.WaitGroup

	collectMu sync.Mutex
}

func New(st recoveryStore, tm tmuxClient, options Options) *Service {
	if options.SnapshotInterval <= 0 {
		options.SnapshotInterval = defaultSnapshotPeriod
	}
	if options.MaxSnapshotsPerSess <= 0 {
		options.MaxSnapshotsPerSess = defaultMaxSnapshots
	}
	runCtx, runCancel := context.WithCancel(context.Background())
	return &Service{
		store:     st,
		tmux:      tm,
		options:   options,
		bootID:    currentBootID,
		events:    options.EventHub,
		runCtx:    runCtx,
		runCancel: runCancel,
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
		// Cancel the fallback runCtx from New() before replacing with
		// one derived from parent, so server shutdown propagates.
		s.runCancel()
		s.runCtx, s.runCancel = context.WithCancel(parent)

		go func() {
			defer close(s.doneCh)
			if err := s.Collect(ctx); err != nil {
				slog.Warn("recovery initial collect failed", "err", err)
			}
			ticker := time.NewTicker(s.options.SnapshotInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := s.Collect(ctx); err != nil {
						slog.Warn("recovery periodic collect failed", "err", err)
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
		if s.runCancel != nil {
			s.runCancel()
		}
		if s.doneCh == nil {
			return
		}
		// Wait for collect loop.
		select {
		case <-s.doneCh:
		case <-ctx.Done():
		}
		// Wait for in-flight restore goroutines.
		done := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
		}
	})
}

func (s *Service) Collect(ctx context.Context) error {
	if s == nil || s.store == nil || s.tmux == nil {
		return nil
	}

	s.collectMu.Lock()
	defer s.collectMu.Unlock()

	now := time.Now().UTC()
	bootID := s.bootID(ctx)
	bootChanged, err := s.bootStateChanged(ctx, bootID)
	if err != nil {
		return err
	}

	liveSessions, err := s.listLiveSessions(ctx)
	if err != nil {
		return err
	}

	liveNames, liveSessionList, changedCount := s.captureLiveSessions(ctx, liveSessions, bootID, now)
	liveSetChanged, err := s.updateLiveSessionsRuntime(ctx, liveSessionList)
	if err != nil {
		return err
	}

	killedCount, err := s.markKilledSessionsAfterBootChange(ctx, bootChanged, liveNames, bootID, now)
	if err != nil {
		return err
	}

	if err := s.storeCollectRuntime(ctx, bootID, now); err != nil {
		return err
	}
	if err := s.store.TrimRecoverySnapshots(ctx, s.options.MaxSnapshotsPerSess); err != nil {
		slog.Warn("recovery snapshot trim failed", "err", err)
	}

	s.publishCollectEvents(liveSetChanged, len(liveSessionList), changedCount, killedCount, bootChanged)
	return nil
}

func (s *Service) bootStateChanged(ctx context.Context, bootID string) (bool, error) {
	lastBootID, err := s.store.GetRuntimeValue(ctx, runtimeBootIDKey)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(lastBootID) != "" && strings.TrimSpace(bootID) != "" && bootID != lastBootID, nil
}

func (s *Service) listLiveSessions(ctx context.Context) ([]tmux.Session, error) {
	liveSessions, err := s.tmux.ListSessions(ctx)
	if err == nil {
		return liveSessions, nil
	}
	// If tmux server is not running, treat as no active sessions.
	if tmux.IsKind(err, tmux.ErrKindServerNotRunning) {
		return []tmux.Session{}, nil
	}
	return nil, err
}

func (s *Service) captureLiveSessions(ctx context.Context, sessions []tmux.Session, bootID string, now time.Time) (map[string]bool, []string, int) {
	liveNames := make(map[string]bool, len(sessions))
	liveSessionList := make([]string, 0, len(sessions))
	changedCount := 0
	for _, session := range sessions {
		if strings.TrimSpace(session.Name) == "" {
			continue
		}
		liveNames[session.Name] = true
		liveSessionList = append(liveSessionList, session.Name)
		changed, err := s.captureSession(ctx, session, bootID, now)
		if err != nil {
			slog.Warn("recovery capture session failed", "session", session.Name, "err", err)
			continue
		}
		if changed {
			changedCount++
		}
	}
	sort.Strings(liveSessionList)
	return liveNames, liveSessionList, changedCount
}

func (s *Service) updateLiveSessionsRuntime(ctx context.Context, liveSessionList []string) (bool, error) {
	liveJoined := strings.Join(liveSessionList, ",")
	lastLiveJoined, err := s.store.GetRuntimeValue(ctx, runtimeLiveSessionsKey)
	if err != nil {
		return false, err
	}
	if err := s.store.SetRuntimeValue(ctx, runtimeLiveSessionsKey, liveJoined); err != nil {
		return false, err
	}
	return strings.TrimSpace(lastLiveJoined) != strings.TrimSpace(liveJoined), nil
}

func (s *Service) markKilledSessionsAfterBootChange(ctx context.Context, bootChanged bool, liveNames map[string]bool, bootID string, now time.Time) (int, error) {
	if !bootChanged {
		return 0, nil
	}
	tracked, err := s.store.ListRecoverySessions(ctx, []store.RecoverySessionState{
		store.RecoveryStateRunning,
		store.RecoveryStateRestoring,
		store.RecoveryStateRestored,
	})
	if err != nil {
		return 0, err
	}

	killed := make([]string, 0, len(tracked))
	for _, item := range tracked {
		if !liveNames[item.Name] {
			killed = append(killed, item.Name)
		}
	}
	if len(killed) == 0 {
		return 0, nil
	}
	if err := s.store.MarkRecoverySessionsKilled(ctx, killed, bootID, now); err != nil {
		return 0, err
	}
	for _, name := range killed {
		s.raiseAlert(ctx, alerts.AlertWrite{
			DedupeKey: fmt.Sprintf("recovery:session:%s:killed", name),
			Source:    "recovery",
			Resource:  name,
			Title:     fmt.Sprintf("Session %s killed after reboot", name),
			Message:   fmt.Sprintf("Session %s was lost after boot change", name),
			Severity:  "warn",
			CreatedAt: now,
		})
	}
	_ = s.store.SetRuntimeValue(ctx, runtimeBootChangeKey, now.Format(time.RFC3339))
	slog.Info("recovery marked sessions as killed after boot change", "count", len(killed))

	if mode := s.options.BootRestore; mode != "" && mode != "off" {
		for _, name := range killed {
			s.autoRestoreSession(ctx, name, ReplayMode(mode))
		}
	}
	return len(killed), nil
}

func (s *Service) autoRestoreSession(ctx context.Context, sessionName string, mode ReplayMode) {
	snapshots, err := s.store.ListRecoverySnapshots(ctx, sessionName, 1)
	if err != nil || len(snapshots) == 0 {
		return
	}
	_, err = s.RestoreSnapshotAsync(ctx, snapshots[0].ID, RestoreOptions{
		Mode:           mode,
		ConflictPolicy: ConflictRename,
		TriggeredBy:    "boot",
	})
	if err != nil {
		slog.Warn("recovery auto-restore failed", "session", sessionName, "err", err)
	} else {
		slog.Info("recovery auto-restore started", "session", sessionName, "mode", mode)
	}
}

func (s *Service) storeCollectRuntime(ctx context.Context, bootID string, now time.Time) error {
	if err := s.store.SetRuntimeValue(ctx, runtimeBootIDKey, bootID); err != nil {
		return err
	}
	return s.store.SetRuntimeValue(ctx, runtimeCollectAtKey, now.Format(time.RFC3339))
}

func (s *Service) publishCollectEvents(liveSetChanged bool, liveCount, changedCount, killedCount int, bootChanged bool) {
	if liveSetChanged {
		s.publish(events.TypeTmuxSessions, map[string]any{
			"changedSessions": changedCount,
			"liveCount":       liveCount,
			"action":          "live-set",
		})
	}
	if changedCount > 0 || killedCount > 0 || bootChanged {
		s.publish(events.TypeRecoveryOverview, map[string]any{
			"changedSessions": changedCount,
			"killed":          killedCount,
			"bootChanged":     bootChanged,
		})
	}
}

func (s *Service) captureSession(ctx context.Context, sess tmux.Session, bootID string, capturedAt time.Time) (bool, error) {
	snap, ok, err := s.snapshotFromWatchtowerProjection(ctx, sess, bootID, capturedAt)
	if err != nil {
		return false, err
	}
	if !ok {
		snap, err = s.snapshotFromTmuxRuntime(ctx, sess, bootID, capturedAt)
		if err != nil {
			return false, err
		}
	}
	return s.persistSnapshot(ctx, snap, capturedAt)
}

func (s *Service) snapshotFromWatchtowerProjection(ctx context.Context, sess tmux.Session, bootID string, capturedAt time.Time) (SessionSnapshot, bool, error) {
	_, err := s.store.GetWatchtowerSession(ctx, sess.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionSnapshot{}, false, nil
	}
	if err != nil {
		return SessionSnapshot{}, false, err
	}

	windows, err := s.store.ListWatchtowerWindows(ctx, sess.Name)
	if err != nil {
		return SessionSnapshot{}, false, err
	}
	panes, err := s.store.ListWatchtowerPanes(ctx, sess.Name)
	if err != nil {
		return SessionSnapshot{}, false, err
	}
	if len(windows) == 0 && len(panes) == 0 {
		return SessionSnapshot{}, false, nil
	}

	sort.Slice(windows, func(i, j int) bool { return windows[i].WindowIndex < windows[j].WindowIndex })
	sort.Slice(panes, func(i, j int) bool {
		if panes[i].WindowIndex == panes[j].WindowIndex {
			return panes[i].PaneIndex < panes[j].PaneIndex
		}
		return panes[i].WindowIndex < panes[j].WindowIndex
	})

	paneCountsByWindow := make(map[int]int, len(windows))
	for _, pane := range panes {
		paneCountsByWindow[pane.WindowIndex]++
	}

	snap := SessionSnapshot{
		SessionName:  sess.Name,
		CapturedAt:   capturedAt.UTC(),
		BootID:       bootID,
		Attached:     sess.Attached,
		ActiveWindow: -1,
		ActivePaneID: "",
		Windows:      make([]WindowSnapshot, 0, len(windows)),
		Panes:        make([]PaneSnapshot, 0, len(panes)),
	}
	for _, window := range windows {
		if window.Active {
			snap.ActiveWindow = window.WindowIndex
		}
		snap.Windows = append(snap.Windows, WindowSnapshot{
			Index:  window.WindowIndex,
			Name:   window.Name,
			Active: window.Active,
			Panes:  paneCountsByWindow[window.WindowIndex],
			Layout: window.Layout,
		})
	}
	for _, pane := range panes {
		if pane.Active {
			snap.ActivePaneID = pane.PaneID
			if snap.ActiveWindow < 0 {
				snap.ActiveWindow = pane.WindowIndex
			}
		}
		snap.Panes = append(snap.Panes, PaneSnapshot{
			WindowIndex:    pane.WindowIndex,
			PaneIndex:      pane.PaneIndex,
			Title:          pane.Title,
			Active:         pane.Active,
			CurrentPath:    pane.CurrentPath,
			StartCommand:   pane.StartCommand,
			CurrentCommand: pane.CurrentCommand,
			LastContent:    pane.TailPreview,
		})
	}
	return snap, true, nil
}

func (s *Service) snapshotFromTmuxRuntime(ctx context.Context, sess tmux.Session, bootID string, capturedAt time.Time) (SessionSnapshot, error) {
	windows, err := s.tmux.ListWindows(ctx, sess.Name)
	if err != nil {
		return SessionSnapshot{}, err
	}
	panes, err := s.tmux.ListPanes(ctx, sess.Name)
	if err != nil {
		return SessionSnapshot{}, err
	}

	sort.Slice(windows, func(i, j int) bool { return windows[i].Index < windows[j].Index })
	sort.Slice(panes, func(i, j int) bool {
		if panes[i].WindowIndex == panes[j].WindowIndex {
			return panes[i].PaneIndex < panes[j].PaneIndex
		}
		return panes[i].WindowIndex < panes[j].WindowIndex
	})

	snap := SessionSnapshot{
		SessionName:  sess.Name,
		CapturedAt:   capturedAt.UTC(),
		BootID:       bootID,
		Attached:     sess.Attached,
		ActiveWindow: -1,
		ActivePaneID: "",
		Windows:      make([]WindowSnapshot, 0, len(windows)),
		Panes:        make([]PaneSnapshot, 0, len(panes)),
	}

	for _, window := range windows {
		if window.Active {
			snap.ActiveWindow = window.Index
		}
		snap.Windows = append(snap.Windows, WindowSnapshot{
			Index:  window.Index,
			Name:   window.Name,
			Active: window.Active,
			Panes:  window.Panes,
			Layout: window.Layout,
		})
	}
	for _, pane := range panes {
		if pane.Active {
			snap.ActivePaneID = pane.PaneID
		}
		snap.Panes = append(snap.Panes, PaneSnapshot{
			WindowIndex:    pane.WindowIndex,
			PaneIndex:      pane.PaneIndex,
			Title:          pane.Title,
			Active:         pane.Active,
			CurrentPath:    pane.CurrentPath,
			StartCommand:   pane.StartCommand,
			CurrentCommand: pane.CurrentCommand,
			LastContent:    "",
		})
	}
	return snap, nil
}

func (s *Service) persistSnapshot(ctx context.Context, snap SessionSnapshot, capturedAt time.Time) (bool, error) {
	payloadBytes, err := json.Marshal(snap)
	if err != nil {
		return false, err
	}

	hash, err := snapshotHash(snap)
	if err != nil {
		return false, err
	}
	_, changed, err := s.store.UpsertRecoverySnapshot(ctx, store.RecoverySnapshotWrite{
		SessionName:  snap.SessionName,
		BootID:       snap.BootID,
		StateHash:    hash,
		CapturedAt:   capturedAt,
		ActiveWindow: snap.ActiveWindow,
		ActivePaneID: snap.ActivePaneID,
		Windows:      len(snap.Windows),
		Panes:        len(snap.Panes),
		PayloadJSON:  string(payloadBytes),
	})
	return changed, err
}

func (s *Service) Overview(ctx context.Context) (Overview, error) {
	lastBootID, err := s.store.GetRuntimeValue(ctx, runtimeBootIDKey)
	if err != nil {
		return Overview{}, err
	}
	lastCollectRaw, err := s.store.GetRuntimeValue(ctx, runtimeCollectAtKey)
	if err != nil {
		return Overview{}, err
	}
	lastBootChangeRaw, err := s.store.GetRuntimeValue(ctx, runtimeBootChangeKey)
	if err != nil {
		return Overview{}, err
	}

	killed, err := s.store.ListRecoverySessions(ctx, []store.RecoverySessionState{
		store.RecoveryStateKilled,
		store.RecoveryStateRestoring,
		store.RecoveryStateRestored,
	})
	if err != nil {
		return Overview{}, err
	}
	jobs, err := s.store.ListRecoveryJobs(ctx, []store.RecoveryJobStatus{
		store.RecoveryJobQueued,
		store.RecoveryJobRunning,
		store.RecoveryJobFailed,
	}, 30)
	if err != nil {
		return Overview{}, err
	}

	return Overview{
		BootID:         s.bootID(ctx),
		LastBootID:     lastBootID,
		LastCollectAt:  parseOverviewTime(lastCollectRaw),
		LastBootChange: parseOverviewTime(lastBootChangeRaw),
		KilledSessions: killed,
		RunningJobs:    jobs,
	}, nil
}

func (s *Service) ListKilledSessions(ctx context.Context) ([]store.RecoverySession, error) {
	return s.store.ListRecoverySessions(ctx, []store.RecoverySessionState{
		store.RecoveryStateKilled,
		store.RecoveryStateRestoring,
		store.RecoveryStateRestored,
	})
}

func (s *Service) GetSnapshot(ctx context.Context, id int64) (SnapshotView, error) {
	meta, err := s.store.GetRecoverySnapshot(ctx, id)
	if err != nil {
		return SnapshotView{}, err
	}
	var payload SessionSnapshot
	if err := json.Unmarshal([]byte(meta.PayloadJSON), &payload); err != nil {
		return SnapshotView{}, err
	}
	return SnapshotView{
		Meta:    meta,
		Payload: payload,
	}, nil
}

func (s *Service) ListSnapshots(ctx context.Context, sessionName string, limit int) ([]store.RecoverySnapshot, error) {
	return s.store.ListRecoverySnapshots(ctx, sessionName, limit)
}

func (s *Service) GetJob(ctx context.Context, id string) (store.RecoveryJob, error) {
	return s.store.GetRecoveryJob(ctx, id)
}

func (s *Service) ArchiveSession(ctx context.Context, name string) error {
	if err := s.store.MarkRecoverySessionArchived(ctx, name, time.Now().UTC()); err != nil {
		return err
	}
	s.resolveAlert(ctx, fmt.Sprintf("recovery:session:%s:killed", name), time.Now().UTC())
	return nil
}

func (s *Service) RestoreSnapshotAsync(ctx context.Context, snapshotID int64, options RestoreOptions) (store.RecoveryJob, error) {
	options = options.normalize()
	view, err := s.GetSnapshot(ctx, snapshotID)
	if err != nil {
		return store.RecoveryJob{}, err
	}

	target := strings.TrimSpace(options.TargetSession)
	if target == "" {
		target = view.Payload.SessionName
	}
	jobID := randomJobID()
	totalSteps := estimateTotalSteps(view.Payload)
	triggeredBy := strings.TrimSpace(options.TriggeredBy)
	if triggeredBy == "" {
		triggeredBy = "manual"
	}
	job := store.RecoveryJob{
		ID:             jobID,
		SessionName:    view.Payload.SessionName,
		TargetSession:  target,
		SnapshotID:     snapshotID,
		Mode:           string(options.Mode),
		ConflictPolicy: string(options.ConflictPolicy),
		Status:         store.RecoveryJobQueued,
		TotalSteps:     totalSteps,
		CompletedSteps: 0,
		CurrentStep:    "",
		TriggeredBy:    triggeredBy,
		CreatedAt:      time.Now().UTC(),
	}
	if err := s.store.CreateRecoveryJob(ctx, job); err != nil {
		return store.RecoveryJob{}, err
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runRestoreJob(s.runCtx, jobID, view.Payload, options, target)
	}()

	return job, nil
}

func (s *Service) runRestoreJob(parent context.Context, jobID string, snap SessionSnapshot, options RestoreOptions, requestedTarget string) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()

	updateFailure := func(message string) {
		// Use a context detached from cancellation so terminal writes
		// succeed even after timeout or server shutdown.
		finCtx, finCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer finCancel()
		_ = s.store.FinishRecoveryJob(finCtx, jobID, store.RecoveryJobFailed, message, time.Now().UTC())
		_ = s.store.MarkRecoverySessionRestoreFailed(finCtx, snap.SessionName, message)
		s.raiseAlert(finCtx, alerts.AlertWrite{
			DedupeKey: fmt.Sprintf("recovery:job:%s:failed", jobID),
			Source:    "recovery",
			Resource:  snap.SessionName,
			Title:     fmt.Sprintf("Restore job %s failed", jobID),
			Message:   message,
			Severity:  "error",
			CreatedAt: time.Now().UTC(),
		})
		s.publishJobTransition(jobID, store.RecoveryJobFailed,
			snap.SessionName, store.RecoveryStateKilled, map[string]any{"message": message})
	}

	if err := s.store.SetRecoveryJobRunning(ctx, jobID, time.Now().UTC()); err != nil {
		slog.Warn("recovery set job running failed", "job", jobID, "err", err)
		updateFailure(fmt.Sprintf("failed to set job running: %v", err))
		return
	}
	if err := s.store.MarkRecoverySessionRestoring(ctx, snap.SessionName); err != nil {
		slog.Warn("recovery mark restoring failed", "session", snap.SessionName, "err", err)
	}
	s.publishJobTransition(jobID, store.RecoveryJobRunning,
		snap.SessionName, store.RecoveryStateRestoring, nil)

	totalSteps := estimateTotalSteps(snap)
	doneSteps := 0
	step := func(label string) {
		doneSteps++
		_ = s.store.UpdateRecoveryJobProgress(ctx, jobID, doneSteps, totalSteps, label)
		s.publish(events.TypeRecoveryJob, map[string]any{
			"jobId":     jobID,
			"status":    string(store.RecoveryJobRunning),
			"completed": doneSteps,
			"total":     totalSteps,
			"step":      label,
		})
	}

	target, err := s.resolveRestoreTarget(ctx, requestedTarget, options.ConflictPolicy)
	if err != nil {
		updateFailure(err.Error())
		return
	}
	if target != requestedTarget {
		_ = s.store.UpdateRecoveryJobTarget(ctx, jobID, target)
	}
	step("target resolved")

	if err := s.restoreSession(ctx, snap, options.Mode, target, step); err != nil {
		updateFailure(err.Error())
		return
	}

	// Use a context detached from cancellation for terminal writes.
	finCtx, finCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finCancel()
	if err := s.store.MarkRecoverySessionRestored(finCtx, snap.SessionName, time.Now().UTC()); err != nil {
		slog.Warn("recovery mark restored failed", "session", snap.SessionName, "err", err)
	}
	if err := s.store.FinishRecoveryJob(finCtx, jobID, store.RecoveryJobSucceeded, "", time.Now().UTC()); err != nil {
		slog.Warn("recovery finish job failed", "job", jobID, "err", err)
	}
	s.resolveAlert(finCtx, fmt.Sprintf("recovery:session:%s:killed", snap.SessionName), time.Now().UTC())
	s.publishJobTransition(jobID, store.RecoveryJobSucceeded,
		snap.SessionName, store.RecoveryStateRestored, map[string]any{"target": target})
	s.publish(events.TypeTmuxSessions, map[string]any{
		"source": "recovery.restore",
		"target": target,
	})

	// Trigger a fresh snapshot after restore to keep journal consistent.
	if err := s.Collect(finCtx); err != nil {
		slog.Warn("recovery collect after restore failed", "job", jobID, "err", err)
	}
}

func (s *Service) resolveRestoreTarget(ctx context.Context, requested string, policy ConflictPolicy) (string, error) {
	target := strings.TrimSpace(requested)
	if target == "" {
		return "", errors.New("target session is required")
	}
	exists, err := s.tmux.SessionExists(ctx, target)
	if err != nil {
		return "", err
	}
	if !exists {
		return target, nil
	}

	switch policy {
	case ConflictReplace:
		if err := s.tmux.KillSession(ctx, target); err != nil {
			return "", err
		}
		return target, nil
	case ConflictSkip:
		return "", fmt.Errorf("session %q already exists", target)
	case ConflictRename:
		for i := 1; i <= 99; i++ {
			candidate := fmt.Sprintf("%s-restored-%02d", target, i)
			ok, err := s.tmux.SessionExists(ctx, candidate)
			if err != nil {
				return "", err
			}
			if !ok {
				return candidate, nil
			}
		}
		return "", fmt.Errorf("unable to allocate unique restore target for %q", target)
	default:
		return "", fmt.Errorf("invalid conflict policy: %s", policy)
	}
}

func (s *Service) restoreSession(ctx context.Context, snap SessionSnapshot, mode ReplayMode, target string, step func(label string)) error {
	windows := append([]WindowSnapshot{}, snap.Windows...)
	panes := append([]PaneSnapshot{}, snap.Panes...)
	sort.Slice(windows, func(i, j int) bool { return windows[i].Index < windows[j].Index })
	sort.Slice(panes, func(i, j int) bool {
		if panes[i].WindowIndex == panes[j].WindowIndex {
			return panes[i].PaneIndex < panes[j].PaneIndex
		}
		return panes[i].WindowIndex < panes[j].WindowIndex
	})
	if len(windows) == 0 {
		return errors.New("snapshot has no windows")
	}

	firstWindow := windows[0]
	firstCWD := firstPaneCWDForWindow(panes, firstWindow.Index)
	if err := s.tmux.CreateSession(ctx, target, firstCWD); err != nil {
		return err
	}
	if firstWindow.Name != "" {
		if err := s.tmux.RenameWindow(ctx, target, 0, firstWindow.Name); err != nil {
			return err
		}
	}
	step("session created")

	for idx, window := range windows {
		if idx > 0 {
			if err := s.tmux.NewWindowAt(ctx, target, window.Index, window.Name, firstPaneCWDForWindow(panes, window.Index)); err != nil {
				return err
			}
		}
		step(fmt.Sprintf("window %d ready", window.Index))

		windowPanes := panesForWindow(panes, window.Index)
		if len(windowPanes) == 0 {
			continue
		}

		tree := parseLayout(strings.TrimSpace(window.Layout))
		if tree != nil && tree.leafCount() == len(windowPanes) {
			// Layout-aware: create panes matching the layout tree.
			firstPaneID, err := s.firstPaneIDForWindow(ctx, target, window.Index)
			if err != nil {
				return err
			}
			counter := 0
			cwdFn := func(leafIdx int) string {
				if leafIdx < len(windowPanes) {
					return strings.TrimSpace(windowPanes[leafIdx].CurrentPath)
				}
				return ""
			}
			result, err := buildPanes(ctx, s.tmux, tree, firstPaneID, cwdFn, &counter)
			if err != nil {
				return err
			}
			if strings.TrimSpace(window.Layout) != "" {
				if err := s.tmux.SelectLayout(ctx, target, window.Index, window.Layout); err != nil {
					slog.Debug("recovery select-layout failed", "session", target, "window", window.Index, "err", err)
				}
			}
			if err := s.restoreWindowPanesWithIDs(ctx, result.paneIDs, windowPanes, mode); err != nil {
				return err
			}
		} else {
			// Fallback: sequential splits for snapshots without valid layout.
			if err := s.ensurePaneCount(ctx, target, window.Index, len(windowPanes), windowPanes); err != nil {
				return err
			}
			if strings.TrimSpace(window.Layout) != "" {
				if err := s.tmux.SelectLayout(ctx, target, window.Index, window.Layout); err != nil {
					slog.Debug("recovery select-layout failed", "session", target, "window", window.Index, "err", err)
				}
			}
			if err := s.restoreWindowPanes(ctx, target, window.Index, windowPanes, mode); err != nil {
				return err
			}
		}
		step(fmt.Sprintf("window %d restored", window.Index))
	}

	if snap.ActiveWindow >= 0 {
		_ = s.tmux.SelectWindow(ctx, target, snap.ActiveWindow)
	}
	return nil
}

func (s *Service) ensurePaneCount(ctx context.Context, session string, windowIndex, wanted int, snapPanes []PaneSnapshot) error {
	if wanted <= 1 {
		return nil
	}
	for {
		allPanes, err := s.tmux.ListPanes(ctx, session)
		if err != nil {
			return err
		}
		current := filterPanesByWindow(allPanes, windowIndex)
		if len(current) >= wanted {
			return nil
		}
		sort.Slice(current, func(i, j int) bool { return current[i].PaneIndex < current[j].PaneIndex })
		targetPane := current[len(current)-1]
		nextIdx := len(current)
		nextCWD := paneCWDByIndex(snapPanes, nextIdx)
		direction := "horizontal"
		if nextIdx%2 == 1 {
			direction = "vertical"
		}
		if _, err := s.tmux.SplitPaneIn(ctx, targetPane.PaneID, direction, nextCWD); err != nil {
			return err
		}
	}
}

func (s *Service) restoreWindowPanes(ctx context.Context, session string, windowIndex int, snapPanes []PaneSnapshot, mode ReplayMode) error {
	allPanes, err := s.tmux.ListPanes(ctx, session)
	if err != nil {
		return err
	}
	live := filterPanesByWindow(allPanes, windowIndex)
	sort.Slice(live, func(i, j int) bool { return live[i].PaneIndex < live[j].PaneIndex })
	sort.Slice(snapPanes, func(i, j int) bool { return snapPanes[i].PaneIndex < snapPanes[j].PaneIndex })

	count := len(snapPanes)
	if len(live) < count {
		count = len(live)
	}
	for i := 0; i < count; i++ {
		sp := snapPanes[i]
		lp := live[i]
		if title := strings.TrimSpace(sp.Title); title != "" {
			_ = s.tmux.RenamePane(ctx, lp.PaneID, title)
		}
		if mode == ReplayModeConfirm || mode == ReplayModeFull {
			if cwd := strings.TrimSpace(sp.CurrentPath); cwd != "" {
				_ = s.tmux.SendKeys(ctx, lp.PaneID, "cd "+shellQuoteSingle(cwd), true)
			}
		}
		if mode == ReplayModeFull {
			cmd := strings.TrimSpace(sp.StartCommand)
			if cmd == "" {
				cmd = strings.TrimSpace(sp.CurrentCommand)
			}
			if cmd != "" {
				_ = s.tmux.SendKeys(ctx, lp.PaneID, cmd, true)
			}
		}
		if sp.Active {
			_ = s.tmux.SelectPane(ctx, lp.PaneID)
		}
	}
	return nil
}

func (s *Service) firstPaneIDForWindow(ctx context.Context, session string, windowIndex int) (string, error) {
	allPanes, err := s.tmux.ListPanes(ctx, session)
	if err != nil {
		return "", err
	}
	wPanes := filterPanesByWindow(allPanes, windowIndex)
	if len(wPanes) == 0 {
		return "", fmt.Errorf("no panes found for window %d in session %q", windowIndex, session)
	}
	sort.Slice(wPanes, func(i, j int) bool { return wPanes[i].PaneIndex < wPanes[j].PaneIndex })
	return wPanes[0].PaneID, nil
}

// restoreWindowPanesWithIDs applies snapshot state to panes identified by
// explicit IDs (from layout-aware buildPanes), without needing to re-query
// the pane list from tmux.
func (s *Service) restoreWindowPanesWithIDs(ctx context.Context, paneIDs []string, snapPanes []PaneSnapshot, mode ReplayMode) error {
	sort.Slice(snapPanes, func(i, j int) bool { return snapPanes[i].PaneIndex < snapPanes[j].PaneIndex })
	count := min(len(paneIDs), len(snapPanes))
	for i := 0; i < count; i++ {
		sp := snapPanes[i]
		paneID := paneIDs[i]
		if title := strings.TrimSpace(sp.Title); title != "" {
			_ = s.tmux.RenamePane(ctx, paneID, title)
		}
		if mode == ReplayModeConfirm || mode == ReplayModeFull {
			if cwd := strings.TrimSpace(sp.CurrentPath); cwd != "" {
				_ = s.tmux.SendKeys(ctx, paneID, "cd "+shellQuoteSingle(cwd), true)
			}
		}
		if mode == ReplayModeFull {
			cmd := strings.TrimSpace(sp.StartCommand)
			if cmd == "" {
				cmd = strings.TrimSpace(sp.CurrentCommand)
			}
			if cmd != "" {
				_ = s.tmux.SendKeys(ctx, paneID, cmd, true)
			}
		}
		if sp.Active {
			_ = s.tmux.SelectPane(ctx, paneID)
		}
	}
	return nil
}

func panesForWindow(panes []PaneSnapshot, windowIndex int) []PaneSnapshot {
	out := make([]PaneSnapshot, 0, 8)
	for _, pane := range panes {
		if pane.WindowIndex == windowIndex {
			out = append(out, pane)
		}
	}
	return out
}

func filterPanesByWindow(all []tmux.Pane, windowIndex int) []tmux.Pane {
	out := make([]tmux.Pane, 0, 8)
	for _, pane := range all {
		if pane.WindowIndex == windowIndex {
			out = append(out, pane)
		}
	}
	return out
}

func paneCWDByIndex(panes []PaneSnapshot, paneIndex int) string {
	for _, pane := range panes {
		if pane.PaneIndex == paneIndex {
			return strings.TrimSpace(pane.CurrentPath)
		}
	}
	return ""
}

func firstPaneCWDForWindow(panes []PaneSnapshot, windowIndex int) string {
	windowPanes := panesForWindow(panes, windowIndex)
	if len(windowPanes) == 0 {
		return ""
	}
	sort.Slice(windowPanes, func(i, j int) bool { return windowPanes[i].PaneIndex < windowPanes[j].PaneIndex })
	return strings.TrimSpace(windowPanes[0].CurrentPath)
}

func estimateTotalSteps(snap SessionSnapshot) int {
	total := 1 // session create
	total += len(snap.Windows) * 2
	total += len(snap.Panes)
	if total < 1 {
		return 1
	}
	return total
}

func parseOverviewTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		return ts.UTC()
	}
	return time.Time{}
}

func randomJobID() string {
	var raw [10]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("job-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}

type windowHashEntry struct {
	Index  int
	Name   string
	Panes  int
	Layout string
}

type paneHashEntry struct {
	WindowIndex    int
	PaneIndex      int
	Title          string
	CurrentPath    string
	StartCommand   string
	CurrentCommand string
}

type snapshotHashPayload struct {
	SessionName  string
	Attached     int
	ActiveWindow int
	ActivePaneID string
	Windows      []windowHashEntry
	Panes        []paneHashEntry
}

func snapshotHash(snap SessionSnapshot) (string, error) {
	windows := make([]windowHashEntry, 0, len(snap.Windows))
	for _, item := range snap.Windows {
		windows = append(windows, windowHashEntry{
			Index:  item.Index,
			Name:   item.Name,
			Panes:  item.Panes,
			Layout: item.Layout,
		})
	}
	panes := make([]paneHashEntry, 0, len(snap.Panes))
	for _, item := range snap.Panes {
		panes = append(panes, paneHashEntry{
			WindowIndex:    item.WindowIndex,
			PaneIndex:      item.PaneIndex,
			Title:          item.Title,
			CurrentPath:    item.CurrentPath,
			StartCommand:   item.StartCommand,
			CurrentCommand: item.CurrentCommand,
		})
	}

	sort.Slice(windows, func(i, j int) bool { return windows[i].Index < windows[j].Index })
	sort.Slice(panes, func(i, j int) bool {
		if panes[i].WindowIndex == panes[j].WindowIndex {
			return panes[i].PaneIndex < panes[j].PaneIndex
		}
		return panes[i].WindowIndex < panes[j].WindowIndex
	})

	blob, err := json.Marshal(snapshotHashPayload{
		SessionName:  snap.SessionName,
		Attached:     snap.Attached,
		ActiveWindow: snap.ActiveWindow,
		ActivePaneID: snap.ActivePaneID,
		Windows:      windows,
		Panes:        panes,
	})
	if err != nil {
		return "", fmt.Errorf("marshal snapshot for hashing: %w", err)
	}
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:8]), nil
}

func (s *Service) publishJobTransition(jobID string, jobStatus store.RecoveryJobStatus,
	sessionName string, sessionState store.RecoverySessionState, extra map[string]any) {
	jobPayload := map[string]any{
		"jobId":  jobID,
		"status": string(jobStatus),
	}
	maps.Copy(jobPayload, extra)
	s.publish(events.TypeRecoveryJob, jobPayload)
	s.publish(events.TypeRecoveryOverview, map[string]any{
		"session": sessionName,
		"status":  string(sessionState),
	})
}

func (s *Service) publish(eventType string, payload map[string]any) {
	if s == nil || s.events == nil {
		return
	}
	s.events.Publish(events.NewEvent(eventType, payload))
}

func (s *Service) raiseAlert(ctx context.Context, write alerts.AlertWrite) {
	if s == nil || s.options.AlertRepo == nil {
		return
	}
	if _, err := s.options.AlertRepo.UpsertAlert(ctx, write); err != nil {
		slog.Warn("recovery: upsert alert failed", "dedupeKey", write.DedupeKey, "error", err)
	}
}

func (s *Service) resolveAlert(ctx context.Context, dedupeKey string, at time.Time) {
	if s == nil || s.options.AlertRepo == nil {
		return
	}
	if _, err := s.options.AlertRepo.ResolveAlert(ctx, dedupeKey, at); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Warn("recovery: resolve alert failed", "dedupeKey", dedupeKey, "error", err)
		}
	}
}
