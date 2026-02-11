package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"sentinel/internal/api"
	"sentinel/internal/config"
	"sentinel/internal/httpui"
	"sentinel/internal/security"
	"sentinel/internal/store"
	"sentinel/internal/terminals"
)

func main() {
	cfg := config.Load()
	initLogger(cfg.LogLevel)

	guard := security.New(cfg.Token, cfg.AllowedOrigins)
	terminalRegistry := terminals.NewRegistry()

	mux := http.NewServeMux()
	if err := httpui.Register(mux, guard, terminalRegistry); err != nil {
		slog.Error("frontend init failed", "err", err)
		os.Exit(1)
	}

	st, err := store.New(filepath.Join(cfg.DataDir, "sentinel.db"))
	if err != nil {
		slog.Error("store init failed", "err", err)
		os.Exit(1)
	}

	api.Register(mux, guard, terminalRegistry, st)

	exitCode := run(cfg, mux)
	_ = st.Close()
	os.Exit(exitCode)
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
