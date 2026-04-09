package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/opus-domini/sentinel/internal/activity"
	"github.com/opus-domini/sentinel/internal/api"
	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/httpui"
	"github.com/opus-domini/sentinel/internal/notify"
	"github.com/opus-domini/sentinel/internal/report"
	"github.com/opus-domini/sentinel/internal/scheduler"
	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/services"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
	"github.com/opus-domini/sentinel/internal/watchtower"
)

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

func serve() int {
	cfg := config.Load()
	initLogger(cfg.LogLevel)

	if err := security.ValidateRemoteExposure(cfg.ListenAddr, cfg.Token); err != nil {
		slog.Error("security: token is required for remote listen address", "listen", cfg.ListenAddr)
		return 1
	}
	if security.ExposesBeyondLoopback(cfg.ListenAddr) && !security.HasAllowedOrigins(cfg.AllowedOrigins) {
		slog.Warn("consider setting allowed_origins to restrict cross-origin access", "listen", cfg.ListenAddr)
	}

	cfg.SystemUsers = config.ReadSystemUsers()
	if len(cfg.SystemUsers) > 0 {
		slog.Info("system users loaded", "count", len(cfg.SystemUsers))
	} else {
		slog.Warn("could not read system users; multi-user switching disabled")
	}

	config.ValidateMultiUser(&cfg)
	tmux.SystemUsers = cfg.SystemUsers
	cookiePolicy := security.ParseCookieSecurePolicy(cfg.CookieSecure)
	guard := security.NewWithMultiUser(cfg.Token, cfg.AllowedOrigins, cookiePolicy, security.MultiUserConfig{
		AllowedUsers:    cfg.MultiUser.AllowedUsers,
		AllowRootTarget: cfg.MultiUser.AllowRootTarget,
		SystemUsers:     cfg.SystemUsers,
	})

	if security.ExposesBeyondLoopback(cfg.ListenAddr) && cfg.Token != "" && cookiePolicy == security.CookieSecureNever {
		if cfg.AllowInsecureCookie {
			slog.Warn("cookie_secure=never with remote exposure and token auth; bypassed via SENTINEL_ALLOW_INSECURE_COOKIE")
		} else {
			slog.Error("cookie_secure=never is not allowed with remote exposure and token auth; set cookie_secure=auto or cookie_secure=always, or set SENTINEL_ALLOW_INSECURE_COOKIE=true to bypass")
			return 1
		}
	}
	eventHub := events.NewHub()

	st, err := store.New(filepath.Join(cfg.DataDir, "sentinel.db"))
	if err != nil {
		slog.Error("store init failed", "err", err)
		return 1
	}
	defer func() { _ = st.Close() }()

	if n, err := st.FailOrphanedRuns(context.Background()); err != nil {
		slog.Warn("failed to reconcile orphaned runbook runs", "err", err)
	} else if n > 0 {
		slog.Info("reconciled orphaned runbook runs", "count", n)
	}

	restorePinnedCtx, cancelRestorePinned := context.WithTimeout(context.Background(), 15*time.Second)
	restoredPinned, err := restorePinnedSessions(restorePinnedCtx, st, func(user string) pinnedSessionStarter {
		return tmux.Service{User: strings.TrimSpace(user)}
	})
	cancelRestorePinned()
	if err != nil {
		slog.Warn("failed to restore pinned sessions", "err", err)
	} else if restoredPinned > 0 {
		slog.Info("restored pinned sessions", "count", restoredPinned)
	}

	opsManager := services.NewManager(time.Now(), st)

	mux := http.NewServeMux()
	configPath := filepath.Join(cfg.DataDir, "config.toml")
	apiHandler := api.Register(mux, guard, st, opsManager, eventHub, currentVersion(), configPath, cfg.Timezone, cfg.Locale, cfg.RunbookMaxConcurrent)

	if err := httpui.Register(mux, guard, st, eventHub, opsManager, apiHandler.SessionUser); err != nil {
		slog.Error("frontend init failed", "err", err)
		return 1
	}

	watchtowerService := watchtower.New(st, tmux.Service{}, watchtower.Options{
		TickInterval:   cfg.Watchtower.TickInterval,
		CaptureLines:   cfg.Watchtower.CaptureLines,
		CaptureTimeout: cfg.Watchtower.CaptureTimeout,
		JournalRows:    cfg.Watchtower.JournalRows,
		Publish: func(eventType string, payload map[string]any) {
			eventHub.Publish(events.NewEvent(eventType, payload))
		},
		UserProvider: func(ctx context.Context) []string {
			userMap, err := st.ListSessionUsers(ctx)
			if err != nil {
				return nil
			}
			seen := make(map[string]struct{})
			for _, u := range userMap {
				if u != "" {
					seen[u] = struct{}{}
				}
			}
			users := make([]string, 0, len(seen))
			for u := range seen {
				users = append(users, u)
			}
			return users
		},
	})
	if cfg.Watchtower.Enabled {
		watchtowerService.Start(context.Background())
	}

	alertNotifier := notify.New(cfg.AlertWebhookURL, cfg.AlertWebhookEvents)
	if alertNotifier != nil {
		slog.Info("alert webhook enabled", "url", cfg.AlertWebhookURL)
	}

	healthChecker := services.NewHealthChecker(opsManager, st, func(eventType string, payload map[string]any) {
		eventHub.Publish(events.NewEvent(eventType, payload))
	}, 0, services.AlertThresholds{
		CPUPercent:  cfg.AlertThresholds.CPUPercent,
		MemPercent:  cfg.AlertThresholds.MemPercent,
		DiskPercent: cfg.AlertThresholds.DiskPercent,
	})
	healthChecker.SetActivityRepo(st)
	healthChecker.SetNotifier(alertNotifier)
	healthChecker.Start(context.Background())

	schedulerService := scheduler.New(st, st, scheduler.Options{
		TickInterval: 5 * time.Second,
		EventHub:     eventHub,
		AlertRepo:    st,
	})
	schedulerService.Start(context.Background())

	// Health report generator (optional: requires webhook URL + schedule).
	var reportGen *report.Generator
	if cfg.HealthReportWebhookURL != "" {
		reportNotifier := notify.New(cfg.HealthReportWebhookURL, nil)
		reportGen = report.New(st, opsManager, reportNotifier)
		if cfg.HealthReportSchedule != "" {
			if err := reportGen.StartSchedule(context.Background(), cfg.HealthReportSchedule, cfg.Timezone); err != nil {
				slog.Warn("health report schedule failed to start", "error", err)
			} else {
				slog.Info("health report enabled", "url", cfg.HealthReportWebhookURL, "schedule", cfg.HealthReportSchedule)
			}
		}
	}

	metricsCtx, stopMetrics := context.WithCancel(context.Background())
	metricsDone := startMetricsTicker(metricsCtx, opsManager, eventHub)

	alertsCtx, stopAlerts := context.WithCancel(context.Background())
	alertsDone := startAlertsTicker(alertsCtx, st, eventHub)

	activityCtx, stopActivity := context.WithCancel(context.Background())
	activityDone := startActivityTicker(activityCtx, st, eventHub)

	pruneCtx, stopPrune := context.WithCancel(context.Background())
	pruneDone := startOpsPruneTicker(pruneCtx, st)

	apiHandler.SetNotifier(alertNotifier)

	exitCode := run(cfg, mux)

	// Shutdown in LIFO order: API handler first (drains in-flight requests),
	// then tickers (wait for doneCh so no queries race with st.Close),
	// then services, then store.
	apiShutdownCtx, cancelAPI := context.WithTimeout(context.Background(), 5*time.Second)
	apiHandler.Shutdown(apiShutdownCtx)
	cancelAPI()

	stopPrune()
	stopActivity()
	stopAlerts()
	stopMetrics()
	<-pruneDone
	<-activityDone
	<-alertsDone
	<-metricsDone

	stopReportCtx, cancelReport := context.WithTimeout(context.Background(), 2*time.Second)
	reportGen.Stop(stopReportCtx)
	cancelReport()

	stopSchedulerCtx, cancelScheduler := context.WithTimeout(context.Background(), 2*time.Second)
	schedulerService.Stop(stopSchedulerCtx)
	cancelScheduler()

	stopHealthCtx, cancelHealth := context.WithTimeout(context.Background(), 2*time.Second)
	healthChecker.Stop(stopHealthCtx)
	cancelHealth()

	if cfg.Watchtower.Enabled {
		stopWatchtowerCtx, cancelWatchtower := context.WithTimeout(context.Background(), 2*time.Second)
		watchtowerService.Stop(stopWatchtowerCtx)
		cancelWatchtower()
	}
	return exitCode
}

type commandContext struct {
	stdout io.Writer
	stderr io.Writer
}

func run(cfg config.Config, mux *http.ServeMux) int {
	server := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      requestLog(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-shutdownCh
		slog.Info("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			slog.Error("shutdown error", "err", err)
		}
	}()

	slog.Info("sentinel starting", "version", currentVersion(), "listen", cfg.ListenAddr, "data_dir", cfg.DataDir)
	slog.Info("security", "token_required", cfg.Token != "", "allowed_origins", len(cfg.AllowedOrigins))

	if cfg.Watchtower.Enabled {
		slog.Info("watchtower enabled", "tick", cfg.Watchtower.TickInterval, "capture_lines", cfg.Watchtower.CaptureLines)
	} else {
		slog.Info("watchtower disabled")
	}

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server error", "err", err)
		return 1
	}
	slog.Info("sentinel stopped")
	return 0
}

type pinnedSessionStore interface {
	ListSessionPresets(ctx context.Context) ([]store.SessionPreset, error)
	RecordSessionDirectory(ctx context.Context, path string) error
	SetIcon(ctx context.Context, name, icon string) error
	MarkSessionPresetLaunched(ctx context.Context, name string) error
	ListManagedTmuxWindowsBySession(ctx context.Context, sessionName string) ([]store.ManagedTmuxWindow, error)
	UpdateManagedTmuxWindowRuntime(ctx context.Context, id, tmuxWindowID string, lastWindowIndex int) error
}

type pinnedSessionStarter interface {
	CreateSession(ctx context.Context, name, cwd string) error
	ListWindows(ctx context.Context, session string) ([]tmux.Window, error)
	ListPanes(ctx context.Context, session string) ([]tmux.Pane, error)
	RenameWindow(ctx context.Context, session string, index int, name string) error
	NewWindowWithOptions(ctx context.Context, session, name, cwd string) (tmux.NewWindowResult, error)
	SendKeys(ctx context.Context, paneID, keys string, enter bool) error
}

type pinnedSessionStarterFactory func(user string) pinnedSessionStarter

func restorePinnedSessions(ctx context.Context, repo pinnedSessionStore, starterForUser pinnedSessionStarterFactory) (int, error) {
	presets, err := repo.ListSessionPresets(ctx)
	if err != nil {
		return 0, err
	}

	restored := 0
	for _, preset := range presets {
		tm := starterForUser(strings.TrimSpace(preset.User))
		created := true
		err := tm.CreateSession(ctx, preset.Name, preset.Cwd)
		if err != nil && !tmux.IsKind(err, tmux.ErrKindSessionExists) {
			slog.Warn("failed to restore pinned session", "session", preset.Name, "cwd", preset.Cwd, "err", err)
			continue
		}
		if tmux.IsKind(err, tmux.ErrKindSessionExists) {
			created = false
		}

		restored++
		if err := repo.RecordSessionDirectory(ctx, preset.Cwd); err != nil {
			slog.Warn("failed to record pinned session directory", "session", preset.Name, "cwd", preset.Cwd, "err", err)
		}
		if err := repo.SetIcon(ctx, preset.Name, preset.Icon); err != nil {
			slog.Warn("failed to restore pinned session icon", "session", preset.Name, "icon", preset.Icon, "err", err)
		}
		if err := repo.MarkSessionPresetLaunched(ctx, preset.Name); err != nil {
			slog.Warn("failed to mark pinned session launched", "session", preset.Name, "err", err)
		}
		if created {
			if err := restoreManagedTmuxWindowsForSession(ctx, repo, tm, preset); err != nil {
				slog.Warn("failed to restore managed tmux windows", "session", preset.Name, "err", err)
			}
		}
	}

	return restored, nil
}

func restoreManagedTmuxWindowsForSession(ctx context.Context, repo pinnedSessionStore, tm pinnedSessionStarter, preset store.SessionPreset) error {
	managedWindows, err := repo.ListManagedTmuxWindowsBySession(ctx, preset.Name)
	if err != nil || len(managedWindows) == 0 {
		return err
	}

	liveWindows, err := tm.ListWindows(ctx, preset.Name)
	if err != nil {
		return err
	}
	livePanes, err := tm.ListPanes(ctx, preset.Name)
	if err != nil {
		return err
	}
	if len(liveWindows) == 0 {
		return nil
	}

	firstWindow := liveWindows[0]
	for _, window := range liveWindows[1:] {
		if window.Index < firstWindow.Index {
			firstWindow = window
		}
	}
	firstPane, ok := firstPaneForWindow(livePanes, firstWindow.Index)
	if !ok {
		return nil
	}

	var firstErr error
	if err := restoreManagedTmuxWindowInExistingSlot(ctx, repo, tm, preset, managedWindows[0], firstWindow, firstPane); err != nil && firstErr == nil {
		firstErr = err
	}
	for _, managedWindow := range managedWindows[1:] {
		if err := restoreManagedTmuxWindow(ctx, repo, tm, preset, managedWindow); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func restoreManagedTmuxWindowInExistingSlot(ctx context.Context, repo pinnedSessionStore, tm pinnedSessionStarter, preset store.SessionPreset, managedWindow store.ManagedTmuxWindow, liveWindow tmux.Window, livePane tmux.Pane) error {
	if err := tm.RenameWindow(ctx, preset.Name, liveWindow.Index, managedWindow.WindowName); err != nil {
		return err
	}
	if err := repo.UpdateManagedTmuxWindowRuntime(ctx, managedWindow.ID, liveWindow.ID, liveWindow.Index); err != nil {
		return err
	}
	if strings.TrimSpace(managedWindow.Command) == "" {
		return nil
	}
	return tm.SendKeys(ctx, livePane.PaneID, managedWindow.Command, true)
}

func restoreManagedTmuxWindow(ctx context.Context, repo pinnedSessionStore, tm pinnedSessionStarter, preset store.SessionPreset, managedWindow store.ManagedTmuxWindow) error {
	createdWindow, err := tm.NewWindowWithOptions(
		ctx,
		preset.Name,
		managedWindow.WindowName,
		resolveManagedTmuxWindowCwd(managedWindow, preset.Cwd),
	)
	if err != nil {
		return err
	}
	if err := repo.UpdateManagedTmuxWindowRuntime(ctx, managedWindow.ID, createdWindow.ID, createdWindow.Index); err != nil {
		return err
	}
	if strings.TrimSpace(managedWindow.Command) == "" {
		return nil
	}
	return tm.SendKeys(ctx, createdWindow.PaneID, managedWindow.Command, true)
}

func resolveManagedTmuxWindowCwd(managedWindow store.ManagedTmuxWindow, sessionCwd string) string {
	if resolved := strings.TrimSpace(managedWindow.ResolvedCwd); resolved != "" {
		return resolved
	}
	if managedWindow.CwdMode == store.TmuxLauncherCwdModeFixed {
		return strings.TrimSpace(managedWindow.CwdValue)
	}
	return strings.TrimSpace(sessionCwd)
}

func firstPaneForWindow(panes []tmux.Pane, windowIndex int) (tmux.Pane, bool) {
	for _, pane := range panes {
		if pane.WindowIndex == windowIndex && pane.Active {
			return pane, true
		}
	}
	for _, pane := range panes {
		if pane.WindowIndex == windowIndex {
			return pane, true
		}
	}
	return tmux.Pane{}, false
}

func startMetricsTicker(ctx context.Context, mgr *services.Manager, hub *events.Hub) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				collectCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
				m := mgr.Metrics(collectCtx)
				cancel()
				hub.Publish(events.NewEvent(events.TypeOpsMetrics, map[string]any{
					"metrics": m,
				}))
			}
		}
	}()
	return done
}

func startAlertsTicker(ctx context.Context, st *store.Store, hub *events.Hub) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		var lastRev int64
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				collectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				rev, err := st.GetOpsAlertRevision(collectCtx)
				if err != nil {
					cancel()
					slog.Warn("alerts tick: revision check failed", "err", err)
					continue
				}
				if rev == lastRev {
					cancel()
					continue
				}
				alerts, err := st.ListAlerts(collectCtx, 100, "")
				cancel()
				if err != nil {
					slog.Warn("alerts tick failed", "err", err)
					continue
				}
				lastRev = rev
				hub.Publish(events.NewEvent(events.TypeOpsAlerts, map[string]any{
					"alerts": alerts,
				}))
			}
		}
	}()
	return done
}

func startActivityTicker(ctx context.Context, st *store.Store, hub *events.Hub) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		var lastRev int64
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				collectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				rev, err := st.GetOpsActivityRevision(collectCtx)
				if err != nil {
					cancel()
					slog.Warn("activity tick: revision check failed", "err", err)
					continue
				}
				if rev == lastRev {
					cancel()
					continue
				}
				result, err := st.SearchActivityEvents(collectCtx, activity.Query{
					Limit: 200,
				})
				cancel()
				if err != nil {
					slog.Warn("activity tick failed", "err", err)
					continue
				}
				lastRev = rev
				hub.Publish(events.NewEvent(events.TypeOpsActivity, map[string]any{
					"events": result.Events,
				}))
			}
		}
	}()
	return done
}

func startOpsPruneTicker(ctx context.Context, st *store.Store) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pruneCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				if n, err := st.PruneOpsActivityRows(pruneCtx, 10000); err != nil {
					slog.Warn("ops activity prune failed", "err", err)
				} else if n > 0 {
					slog.Info("ops activity pruned", "removed", n)
				}
				cancel()
			}
		}
	}()
	return done
}

// requestIDKey is the context key for the request ID.
type requestIDKey struct{}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := generateRequestID()
		ctx := context.WithValue(r.Context(), requestIDKey{}, rid)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-ID", rid)

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		rec.ServeHTTP(next, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration", time.Since(start).Truncate(time.Millisecond), "request_id", rid)
	})
}

// statusRecorder wraps http.ResponseWriter to capture the status code.
// Unwrap returns the underlying ResponseWriter so net/http can discover
// http.Hijacker (needed for WebSocket upgrade) via interface assertion.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.wroteHeader = true
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap exposes the underlying ResponseWriter so http.ResponseController
// and net/http can discover interfaces like http.Hijacker.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// Hijack implements http.Hijacker so that direct type assertions
// (e.g. w.(http.Hijacker)) work through the wrapper. This is required
// for WebSocket upgrade in internal/ws which uses a direct assertion.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := r.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}

func (r *statusRecorder) ServeHTTP(next http.Handler, req *http.Request) {
	next.ServeHTTP(r, req)
}

func generateRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}

func initLogger(level string) {
	var lv slog.Level
	switch level {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lv})))
}
