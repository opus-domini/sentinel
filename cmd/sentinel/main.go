package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/opus-domini/sentinel/internal/api"
	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/httpui"
	"github.com/opus-domini/sentinel/internal/ops"
	"github.com/opus-domini/sentinel/internal/recovery"
	"github.com/opus-domini/sentinel/internal/scheduler"
	"github.com/opus-domini/sentinel/internal/security"
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

	if err := security.ValidateRemoteExposure(cfg.ListenAddr, cfg.Token, cfg.AllowedOrigins); err != nil {
		if errors.Is(err, security.ErrRemoteToken) {
			slog.Error("security baseline check failed",
				"listen", cfg.ListenAddr,
				"token_required", cfg.Token != "",
				"allowed_origins", len(cfg.AllowedOrigins),
				"err", err,
			)
			return 1
		}
		slog.Warn("security baseline warning",
			"listen", cfg.ListenAddr,
			"token_required", cfg.Token != "",
			"allowed_origins", len(cfg.AllowedOrigins),
			"err", err,
		)
	}

	guard := security.New(cfg.Token, cfg.AllowedOrigins)
	eventHub := events.NewHub()

	st, err := store.New(filepath.Join(cfg.DataDir, "sentinel.db"))
	if err != nil {
		slog.Error("store init failed", "err", err)
		return 1
	}

	mux := http.NewServeMux()
	if err := httpui.Register(mux, guard, st, eventHub, ops.NewManager(time.Now(), st)); err != nil {
		slog.Error("frontend init failed", "err", err)
		return 1
	}

	var recoveryService *recovery.Service
	watchtowerService := watchtower.New(st, tmux.Service{}, watchtower.Options{
		TickInterval:   cfg.Watchtower.TickInterval,
		CaptureLines:   cfg.Watchtower.CaptureLines,
		CaptureTimeout: cfg.Watchtower.CaptureTimeout,
		JournalRows:    cfg.Watchtower.JournalRows,
		Publish: func(eventType string, payload map[string]any) {
			eventHub.Publish(events.NewEvent(eventType, payload))
		},
		OpsTimeline: func(ctx context.Context, source, eventType, severity, resource, message, details string) {
			if _, err := st.InsertOpsTimelineEvent(ctx, store.OpsTimelineEventWrite{
				Source:    source,
				EventType: eventType,
				Severity:  severity,
				Resource:  resource,
				Message:   message,
				Details:   details,
				CreatedAt: time.Now().UTC(),
			}); err != nil {
				slog.Warn("ops timeline write failed", "source", source, "err", err)
			}
		},
	})
	if cfg.Watchtower.Enabled {
		watchtowerService.Start(context.Background())
	}
	if cfg.Recovery.Enabled {
		recoveryService = recovery.New(st, tmux.Service{}, recovery.Options{
			SnapshotInterval:    cfg.Recovery.SnapshotInterval,
			CaptureLines:        cfg.Recovery.CaptureLines,
			MaxSnapshotsPerSess: cfg.Recovery.MaxSnapshots,
			EventHub:            eventHub,
		})
		recoveryService.Start(context.Background())
	}

	opsManager := ops.NewManager(time.Now(), st)
	healthChecker := ops.NewHealthChecker(opsManager, st, func(eventType string, payload map[string]any) {
		eventHub.Publish(events.NewEvent(eventType, payload))
	}, 0)
	healthChecker.Start(context.Background())

	schedulerService := scheduler.New(st, scheduler.Options{
		TickInterval: 5 * time.Second,
		EventHub:     eventHub,
	})
	schedulerService.Start(context.Background())

	metricsCtx, stopMetrics := context.WithCancel(context.Background())
	go startMetricsTicker(metricsCtx, opsManager, eventHub)

	alertsCtx, stopAlerts := context.WithCancel(context.Background())
	go startAlertsTicker(alertsCtx, st, eventHub)

	timelineCtx, stopTimeline := context.WithCancel(context.Background())
	go startTimelineTicker(timelineCtx, st, eventHub)

	configPath := filepath.Join(cfg.DataDir, "config.toml")
	api.Register(mux, guard, st, recoveryService, eventHub, currentVersion(), configPath)

	exitCode := run(cfg, mux)
	stopTimeline()
	stopAlerts()
	stopMetrics()
	stopSchedulerCtx, cancelScheduler := context.WithTimeout(context.Background(), 2*time.Second)
	schedulerService.Stop(stopSchedulerCtx)
	cancelScheduler()
	healthChecker.Stop()
	if cfg.Watchtower.Enabled {
		stopWatchtowerCtx, cancelWatchtower := context.WithTimeout(context.Background(), 2*time.Second)
		watchtowerService.Stop(stopWatchtowerCtx)
		cancelWatchtower()
	}
	if recoveryService != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		recoveryService.Stop(stopCtx)
		cancel()
	}
	_ = st.Close()
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

	slog.Info("sentinel started",
		"listen", cfg.ListenAddr,
		"data_dir", cfg.DataDir,
		"token_required", cfg.Token != "",
		"log_level", cfg.LogLevel,
		"watchtower_enabled", cfg.Watchtower.Enabled,
		"watchtower_tick", cfg.Watchtower.TickInterval.String(),
		"watchtower_capture_lines", cfg.Watchtower.CaptureLines,
		"watchtower_capture_timeout", cfg.Watchtower.CaptureTimeout.String(),
		"watchtower_journal_rows", cfg.Watchtower.JournalRows,
		"recovery_enabled", cfg.Recovery.Enabled,
		"recovery_interval", cfg.Recovery.SnapshotInterval.String(),
	)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server error", "err", err)
		return 1
	}
	slog.Info("sentinel stopped")
	return 0
}

func startMetricsTicker(ctx context.Context, mgr *ops.Manager, hub *events.Hub) {
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
}

func startAlertsTicker(ctx context.Context, st *store.Store, hub *events.Hub) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			alerts, err := st.ListOpsAlerts(collectCtx, 100, "")
			cancel()
			if err != nil {
				continue
			}
			hub.Publish(events.NewEvent(events.TypeOpsAlerts, map[string]any{
				"alerts": alerts,
			}))
		}
	}
}

func startTimelineTicker(ctx context.Context, st *store.Store, hub *events.Hub) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			result, err := st.SearchOpsTimelineEvents(collectCtx, store.OpsTimelineQuery{
				Limit: 200,
			})
			cancel()
			if err != nil {
				continue
			}
			hub.Publish(events.NewEvent(events.TypeOpsTimeline, map[string]any{
				"events": result.Events,
			}))
		}
	}
}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Debug("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start).Truncate(time.Millisecond))
	})
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
