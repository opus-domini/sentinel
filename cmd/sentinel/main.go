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
	"github.com/opus-domini/sentinel/internal/recovery"
	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/terminals"
	"github.com/opus-domini/sentinel/internal/tmux"
)

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

func serve() int {
	cfg := config.Load()
	initLogger(cfg.LogLevel)

	guard := security.New(cfg.Token, cfg.AllowedOrigins)
	terminalRegistry := terminals.NewRegistry()
	eventHub := events.NewHub()

	mux := http.NewServeMux()
	if err := httpui.Register(mux, guard, terminalRegistry, eventHub); err != nil {
		slog.Error("frontend init failed", "err", err)
		return 1
	}

	st, err := store.New(filepath.Join(cfg.DataDir, "sentinel.db"))
	if err != nil {
		slog.Error("store init failed", "err", err)
		return 1
	}

	var recoveryService *recovery.Service
	if cfg.Recovery.Enabled {
		recoveryService = recovery.New(st, tmux.Service{}, recovery.Options{
			SnapshotInterval:    cfg.Recovery.SnapshotInterval,
			CaptureLines:        cfg.Recovery.CaptureLines,
			MaxSnapshotsPerSess: cfg.Recovery.MaxSnapshots,
			EventHub:            eventHub,
		})
		recoveryService.Start(context.Background())
	}

	api.Register(mux, guard, terminalRegistry, st, recoveryService, eventHub)

	exitCode := run(cfg, mux)
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
