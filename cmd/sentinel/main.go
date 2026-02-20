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
	"syscall"
	"time"

	"github.com/opus-domini/sentinel/internal/activity"
	"github.com/opus-domini/sentinel/internal/api"
	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/httpui"
	"github.com/opus-domini/sentinel/internal/recovery"
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

	cookiePolicy := security.ParseCookieSecurePolicy(cfg.CookieSecure)
	guard := security.New(cfg.Token, cfg.AllowedOrigins, cookiePolicy)

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

	opsManager := services.NewManager(time.Now(), st)

	mux := http.NewServeMux()
	if err := httpui.Register(mux, guard, st, eventHub, opsManager); err != nil {
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
	})
	if cfg.Watchtower.Enabled {
		watchtowerService.Start(context.Background())
	}
	if cfg.Recovery.Enabled {
		recoveryService = recovery.New(st, tmux.Service{}, recovery.Options{
			SnapshotInterval:    cfg.Recovery.SnapshotInterval,
			MaxSnapshotsPerSess: cfg.Recovery.MaxSnapshots,
			EventHub:            eventHub,
			AlertRepo:           st,
		})
		recoveryService.Start(context.Background())
	}

	healthChecker := services.NewHealthChecker(opsManager, st, func(eventType string, payload map[string]any) {
		eventHub.Publish(events.NewEvent(eventType, payload))
	}, 0, services.AlertThresholds{
		CPUPercent:  cfg.AlertThresholds.CPUPercent,
		MemPercent:  cfg.AlertThresholds.MemPercent,
		DiskPercent: cfg.AlertThresholds.DiskPercent,
	})
	healthChecker.Start(context.Background())

	schedulerService := scheduler.New(st, st, scheduler.Options{
		TickInterval: 5 * time.Second,
		EventHub:     eventHub,
		AlertRepo:    st,
	})
	schedulerService.Start(context.Background())

	metricsCtx, stopMetrics := context.WithCancel(context.Background())
	metricsDone := startMetricsTicker(metricsCtx, opsManager, eventHub)

	alertsCtx, stopAlerts := context.WithCancel(context.Background())
	alertsDone := startAlertsTicker(alertsCtx, st, eventHub)

	activityCtx, stopActivity := context.WithCancel(context.Background())
	activityDone := startActivityTicker(activityCtx, st, eventHub)

	pruneCtx, stopPrune := context.WithCancel(context.Background())
	pruneDone := startOpsPruneTicker(pruneCtx, st)

	configPath := filepath.Join(cfg.DataDir, "config.toml")
	apiHandler := api.Register(mux, guard, st, opsManager, recoveryService, eventHub, currentVersion(), configPath)

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
	if recoveryService != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		recoveryService.Stop(stopCtx)
		cancel()
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

	if cfg.Recovery.Enabled {
		slog.Info("recovery enabled", "interval", cfg.Recovery.SnapshotInterval)
	} else {
		slog.Info("recovery disabled")
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server error", "err", err)
		return 1
	}
	slog.Info("sentinel stopped")
	return 0
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
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				collectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				alerts, err := st.ListAlerts(collectCtx, 100, "")
				cancel()
				if err != nil {
					slog.Warn("alerts tick failed", "err", err)
					continue
				}
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
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				collectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				result, err := st.SearchActivityEvents(collectCtx, activity.Query{
					Limit: 200,
				})
				cancel()
				if err != nil {
					slog.Warn("activity tick failed", "err", err)
					continue
				}
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
