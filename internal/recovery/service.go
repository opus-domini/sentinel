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
	"sort"
	"strings"
	"sync"
	"time"

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
	defaultCaptureLines    = 80
	defaultMaxSnapshots    = 300
)

type tmuxClient interface {
	ListSessions(ctx context.Context) ([]tmux.Session, error)
	ListWindows(ctx context.Context, session string) ([]tmux.Window, error)
	ListPanes(ctx context.Context, session string) ([]tmux.Pane, error)
	SessionExists(ctx context.Context, session string) (bool, error)
	CreateSession(ctx context.Context, name, cwd string) error
	RenameWindow(ctx context.Context, session string, index int, name string) error
	NewWindowAt(ctx context.Context, session string, index int, name, cwd string) error
	SplitPaneIn(ctx context.Context, paneID, direction, cwd string) error
	SelectLayout(ctx context.Context, session string, index int, layout string) error
	SelectWindow(ctx context.Context, session string, index int) error
	SelectPane(ctx context.Context, paneID string) error
	RenamePane(ctx context.Context, paneID, title string) error
	SendKeys(ctx context.Context, paneID, keys string, enter bool) error
	KillSession(ctx context.Context, session string) error
}

type Options struct {
	SnapshotInterval    time.Duration
	CaptureLines        int
	MaxSnapshotsPerSess int
	EventHub            *events.Hub
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
	store   *store.Store
	tmux    tmuxClient
	options Options
	bootID  func(context.Context) string
	events  *events.Hub

	startOnce sync.Once
	stopOnce  sync.Once
	stopFn    context.CancelFunc
	doneCh    chan struct{}

	collectMu sync.Mutex
}

func New(st *store.Store, tm tmuxClient, options Options) *Service {
	if options.SnapshotInterval <= 0 {
		options.SnapshotInterval = defaultSnapshotPeriod
	}
	if options.CaptureLines <= 0 {
		options.CaptureLines = defaultCaptureLines
	}
	if options.MaxSnapshotsPerSess <= 0 {
		options.MaxSnapshotsPerSess = defaultMaxSnapshots
	}
	return &Service{
		store:   st,
		tmux:    tm,
		options: options,
		bootID:  currentBootID,
		events:  options.EventHub,
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
		if s.doneCh == nil {
			return
		}
		select {
		case <-s.doneCh:
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
	lastBootID, err := s.store.GetRuntimeValue(ctx, runtimeBootIDKey)
	if err != nil {
		return err
	}
	bootChanged := strings.TrimSpace(lastBootID) != "" && strings.TrimSpace(bootID) != "" && bootID != lastBootID

	liveSessions, err := s.tmux.ListSessions(ctx)
	if err != nil {
		// If tmux server is not running, treat as no active sessions.
		if !tmux.IsKind(err, tmux.ErrKindServerNotRunning) {
			return err
		}
		liveSessions = []tmux.Session{}
	}

	liveNames := make(map[string]bool, len(liveSessions))
	liveSessionList := make([]string, 0, len(liveSessions))
	changedCount := 0
	for _, session := range liveSessions {
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
	liveJoined := strings.Join(liveSessionList, ",")
	lastLiveJoined, err := s.store.GetRuntimeValue(ctx, runtimeLiveSessionsKey)
	if err != nil {
		return err
	}
	if err := s.store.SetRuntimeValue(ctx, runtimeLiveSessionsKey, liveJoined); err != nil {
		return err
	}
	liveSetChanged := strings.TrimSpace(lastLiveJoined) != strings.TrimSpace(liveJoined)

	killedCount := 0
	if bootChanged {
		tracked, err := s.store.ListRecoverySessions(ctx, []store.RecoverySessionState{
			store.RecoveryStateRunning,
			store.RecoveryStateRestoring,
			store.RecoveryStateRestored,
		})
		if err != nil {
			return err
		}

		var killed []string
		for _, item := range tracked {
			if !liveNames[item.Name] {
				killed = append(killed, item.Name)
			}
		}
		if len(killed) > 0 {
			killedCount = len(killed)
			if err := s.store.MarkRecoverySessionsKilled(ctx, killed, bootID, now); err != nil {
				return err
			}
			_ = s.store.SetRuntimeValue(ctx, runtimeBootChangeKey, now.Format(time.RFC3339))
			slog.Info("recovery marked sessions as killed after boot change", "count", len(killed))
		}
	}

	if err := s.store.SetRuntimeValue(ctx, runtimeBootIDKey, bootID); err != nil {
		return err
	}
	if err := s.store.SetRuntimeValue(ctx, runtimeCollectAtKey, now.Format(time.RFC3339)); err != nil {
		return err
	}
	if err := s.store.TrimRecoverySnapshots(ctx, s.options.MaxSnapshotsPerSess); err != nil {
		slog.Warn("recovery snapshot trim failed", "err", err)
	}

	if liveSetChanged {
		s.publish(events.TypeTmuxSessions, map[string]any{
			"changedSessions": changedCount,
			"liveCount":       len(liveSessionList),
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
	return nil
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

	hash := snapshotHash(snap)
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
		store.RecoveryJobPartial,
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
	return s.store.MarkRecoverySessionArchived(ctx, name, time.Now().UTC())
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
		CreatedAt:      time.Now().UTC(),
	}
	if err := s.store.CreateRecoveryJob(ctx, job); err != nil {
		return store.RecoveryJob{}, err
	}

	go s.runRestoreJob(jobID, view.Payload, options, target)

	return job, nil
}

func (s *Service) runRestoreJob(jobID string, snap SessionSnapshot, options RestoreOptions, requestedTarget string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	updateFailure := func(message string) {
		_ = s.store.FinishRecoveryJob(ctx, jobID, store.RecoveryJobFailed, message, time.Now().UTC())
		_ = s.store.MarkRecoverySessionRestoreFailed(ctx, snap.SessionName, message)
		s.publish(events.TypeRecoveryJob, map[string]any{
			"jobId":   jobID,
			"status":  string(store.RecoveryJobFailed),
			"message": message,
		})
		s.publish(events.TypeRecoveryOverview, map[string]any{
			"session": snap.SessionName,
			"status":  string(store.RecoveryStateKilled),
		})
	}

	if err := s.store.SetRecoveryJobRunning(ctx, jobID, time.Now().UTC()); err != nil {
		slog.Warn("recovery set job running failed", "job", jobID, "err", err)
		return
	}
	if err := s.store.MarkRecoverySessionRestoring(ctx, snap.SessionName); err != nil {
		slog.Warn("recovery mark restoring failed", "session", snap.SessionName, "err", err)
	}
	s.publish(events.TypeRecoveryJob, map[string]any{
		"jobId":  jobID,
		"status": string(store.RecoveryJobRunning),
	})
	s.publish(events.TypeRecoveryOverview, map[string]any{
		"session": snap.SessionName,
		"status":  string(store.RecoveryStateRestoring),
	})

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

	if err := s.store.MarkRecoverySessionRestored(ctx, snap.SessionName, time.Now().UTC()); err != nil {
		slog.Warn("recovery mark restored failed", "session", snap.SessionName, "err", err)
	}
	if err := s.store.FinishRecoveryJob(ctx, jobID, store.RecoveryJobSucceeded, "", time.Now().UTC()); err != nil {
		slog.Warn("recovery finish job failed", "job", jobID, "err", err)
	}
	s.publish(events.TypeRecoveryJob, map[string]any{
		"jobId":  jobID,
		"status": string(store.RecoveryJobSucceeded),
		"target": target,
	})
	s.publish(events.TypeRecoveryOverview, map[string]any{
		"session": snap.SessionName,
		"status":  string(store.RecoveryStateRestored),
	})
	s.publish(events.TypeTmuxSessions, map[string]any{
		"source": "recovery.restore",
		"target": target,
	})

	// Trigger a fresh snapshot after restore to keep journal consistent.
	if err := s.Collect(ctx); err != nil {
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
		if err := s.tmux.SplitPaneIn(ctx, targetPane.PaneID, direction, nextCWD); err != nil {
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
		if cwd := strings.TrimSpace(sp.CurrentPath); cwd != "" {
			_ = s.tmux.SendKeys(ctx, lp.PaneID, "cd "+shellQuoteSingle(cwd), true)
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

func snapshotHash(snap SessionSnapshot) string {
	type windowHash struct {
		Index  int
		Name   string
		Panes  int
		Layout string
	}
	type paneHash struct {
		WindowIndex    int
		PaneIndex      int
		Title          string
		CurrentPath    string
		StartCommand   string
		CurrentCommand string
	}

	windows := make([]windowHash, 0, len(snap.Windows))
	for _, item := range snap.Windows {
		windows = append(windows, windowHash{
			Index:  item.Index,
			Name:   item.Name,
			Panes:  item.Panes,
			Layout: item.Layout,
		})
	}
	panes := make([]paneHash, 0, len(snap.Panes))
	for _, item := range snap.Panes {
		panes = append(panes, paneHash{
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

	flat := struct {
		SessionName  string
		Attached     int
		ActiveWindow int
		ActivePaneID string
		Windows      []windowHash
		Panes        []paneHash
	}{
		SessionName:  snap.SessionName,
		Attached:     snap.Attached,
		ActiveWindow: snap.ActiveWindow,
		ActivePaneID: snap.ActivePaneID,
		Windows:      windows,
		Panes:        panes,
	}

	blob, _ := json.Marshal(flat)
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:8])
}

func (s *Service) publish(eventType string, payload map[string]any) {
	if s == nil || s.events == nil {
		return
	}
	s.events.Publish(events.NewEvent(eventType, payload))
}
