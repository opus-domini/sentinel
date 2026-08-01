// Package server boots the Sentinel HTTP server: configuration, the SQLite
// store, the event hub, background tickers and the REST/WebSocket handlers.
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/opus-domini/sentinel/internal/api"
	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/mcpserver"
	"github.com/opus-domini/sentinel/internal/notify"
	"github.com/opus-domini/sentinel/internal/report"
	"github.com/opus-domini/sentinel/internal/scheduler"
	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/services"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/term"
	"github.com/opus-domini/sentinel/internal/tmux"
	"github.com/opus-domini/sentinel/internal/tmuxlifecycle"
	"github.com/opus-domini/sentinel/internal/ui"
	"github.com/opus-domini/sentinel/internal/watchtower"
)

// Serve starts the Sentinel HTTP server and blocks until shutdown. It returns
// the process exit code. The Serve/run split keeps os.Exit out of any function
// holding a defer (the exitAfterDefer lint issue): run returns the exit code.
func Serve(version string) int {
	liveSettings := newSettingsRuntime()
	configService, err := config.NewService(config.Path(), config.Default(), liveSettings)
	if err != nil {
		closeLogger, _ := initLogger(config.DefaultLogLevel, "")
		defer closeLogger()
		slog.Error("config service init failed", "err", err)
		return 1
	}
	configState, err := configService.Read()
	if err != nil {
		closeLogger, _ := initLogger(config.DefaultLogLevel, "")
		defer closeLogger()
		slog.Error("config load failed", "err", err)
		return 1
	}
	cfg := configState.Effective
	liveSettings.initialize(cfg)
	closeLogger, err := initLogger(cfg.Log.Level, cfg.Log.Path)
	if err != nil {
		closeFallback, _ := initLogger(config.DefaultLogLevel, "")
		defer closeFallback()
		slog.Error("logger init failed", "err", err)
		return 1
	}
	defer closeLogger()

	cfg.SystemUsers = config.ReadSystemUsers()
	if len(cfg.SystemUsers) > 0 {
		slog.Info("system users loaded", "count", len(cfg.SystemUsers))
	} else {
		slog.Warn("could not read system users; multi-user switching disabled")
	}

	config.ValidateMultiUser(&cfg)
	tmux.SystemUsers = cfg.SystemUsers
	tmux.UserSwitchMethod = cfg.MultiUser.UserSwitchMethod
	term.UserSwitchMethod = cfg.MultiUser.UserSwitchMethod
	slog.Info("multi-user switching configured", "method", cfg.MultiUser.UserSwitchMethod)
	cookiePolicy := security.ParseCookieSecurePolicy(cfg.Server.CookieSecure)
	guard := security.NewWithOptions(cfg.Server.Token, cfg.Server.AllowedOrigins, cookiePolicy, security.MultiUserConfig{
		AllowedUsers:    cfg.MultiUser.AllowedUsers,
		AllowRootTarget: cfg.MultiUser.AllowRootTarget,
		SystemUsers:     cfg.SystemUsers,
	}, cfg.Server.TrustedProxies)

	eventHub := events.NewHub()

	st, err := store.New(cfg.Storage.Path)
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
	attachments := mcpserver.NewAttachmentManager()
	lifecycleManager := tmuxlifecycle.New(st, tmuxlifecycle.Options{
		RuntimeForUser: func(user string) tmuxlifecycle.Runtime {
			return tmux.Service{User: strings.TrimSpace(user)}
		},
		DetachSession: attachments.DetachSession,
		Publish: func(eventType string, payload map[string]any) {
			eventHub.Publish(events.NewEvent(eventType, payload))
		},
	})

	mux := http.NewServeMux()
	apiHandler := api.Register(mux, guard, st, opsManager, eventHub, version, configService, liveSettings, cfg.Runbooks.MaxConcurrent)
	mcpServer := mcpserver.New(liveSettings, guard, mcpserver.Options{
		Version:             version,
		Attachments:         attachments,
		SessionUser:         apiHandler.SessionUser,
		KnownSessionUsers:   apiHandler.KnownSessionUsers,
		RegisterSessionUser: apiHandler.RegisterSessionUser,
		Runbooks:            apiHandler.RunbookManager(),
	})
	mux.Handle("POST /mcp", mcpServer)
	mux.Handle("GET /mcp", mcpServer)
	mux.Handle("DELETE /mcp", mcpServer)

	if err := ui.Register(mux, guard, st, eventHub, opsManager, apiHandler.SessionUser); err != nil {
		slog.Error("frontend init failed", "err", err)
		return 1
	}
	if err := lifecycleManager.Start(context.Background()); err != nil {
		slog.Error("tmux lifecycle manager failed to start", "err", err)
		mcpServer.Shutdown(context.Background())
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

	schedulerService := scheduler.New(st, st, scheduler.Options{
		TickInterval: 5 * time.Second,
		EventHub:     eventHub,
	})
	schedulerService.Start(context.Background())

	// Health report generator (optional: requires webhook URL + schedule).
	var reportGen *report.Generator
	if cfg.HealthReport.WebhookURL != "" {
		reportNotifier := notify.New(cfg.HealthReport.WebhookURL)
		reportGen = report.New(opsManager, reportNotifier)
		if cfg.HealthReport.Schedule != "" {
			if err := reportGen.StartSchedule(context.Background(), cfg.HealthReport.Schedule, cfg.Server.Timezone); err != nil {
				slog.Warn("health report schedule failed to start", "error", err)
			} else {
				slog.Info("health report enabled", "webhook_configured", true, "schedule", cfg.HealthReport.Schedule)
			}
		}
	}

	opsTickersCtx, stopOpsTickers := context.WithCancel(context.Background())
	metricsDone := startMetricsTicker(opsTickersCtx, opsManager, eventHub)
	servicesDone := startServicesWatcher(opsTickersCtx, opsManager, eventHub)

	exitCode := run(version, cfg, mux)

	// Shutdown in LIFO order: API handler first (drains in-flight requests),
	// then tickers (wait for doneCh so no queries race with st.Close),
	// then services, then store.
	apiShutdownCtx, cancelAPI := context.WithTimeout(context.Background(), 5*time.Second)
	apiHandler.Shutdown(apiShutdownCtx)
	cancelAPI()
	lifecycleShutdownCtx, cancelLifecycle := context.WithTimeout(context.Background(), 5*time.Second)
	if err := lifecycleManager.Stop(lifecycleShutdownCtx); err != nil {
		slog.Warn("tmux lifecycle manager failed to stop", "err", err)
	}
	cancelLifecycle()
	mcpShutdownCtx, cancelMCP := context.WithTimeout(context.Background(), 3*time.Second)
	mcpServer.Shutdown(mcpShutdownCtx)
	cancelMCP()

	stopOpsTickers()
	<-metricsDone
	<-servicesDone

	stopReportCtx, cancelReport := context.WithTimeout(context.Background(), 2*time.Second)
	reportGen.Stop(stopReportCtx)
	cancelReport()

	stopSchedulerCtx, cancelScheduler := context.WithTimeout(context.Background(), 2*time.Second)
	schedulerService.Stop(stopSchedulerCtx)
	cancelScheduler()

	if cfg.Watchtower.Enabled {
		stopWatchtowerCtx, cancelWatchtower := context.WithTimeout(context.Background(), 2*time.Second)
		watchtowerService.Stop(stopWatchtowerCtx)
		cancelWatchtower()
	}
	return exitCode
}

func run(version string, cfg config.Config, mux *http.ServeMux) int {
	server := &http.Server{
		Addr:         cfg.Address(),
		Handler:      requestLog(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	// Disable HTTP/1.1 keep-alive so each response closes the TCP
	// connection immediately.  This prevents idle connections from
	// occupying Chrome's per-site socket pool (max 6, shared across
	// all ports on the same domain) and starving WebSocket upgrades.
	// Hijacked WebSocket connections are unaffected — they bypass the
	// HTTP server's connection lifecycle entirely.
	server.SetKeepAlivesEnabled(false)

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

	slog.Info("sentinel starting", "version", version, "listen", cfg.Address(), "data_dir", cfg.DataDir(), "log", cfg.Log.Path)
	slog.Info("security", "token_required", cfg.Server.Token != "", "allowed_origins", len(cfg.Server.AllowedOrigins))

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

func initLogger(level, path string) (func(), error) {
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
	writer := io.Writer(os.Stderr)
	closeFn := func() {}
	if strings.TrimSpace(path) != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return closeFn, fmt.Errorf("create log dir: %w", err)
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // configured daemon log path.
		if err != nil {
			return closeFn, fmt.Errorf("open log file: %w", err)
		}
		writer = io.MultiWriter(os.Stderr, file)
		closeFn = func() { _ = file.Close() }
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(writer, &slog.HandlerOptions{Level: lv})))
	return closeFn, nil
}
