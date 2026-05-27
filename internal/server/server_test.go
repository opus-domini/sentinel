package server

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/services"
	"github.com/opus-domini/sentinel/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "server-test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestRequestLogSetsRequestIDAndCapturesStatus(t *testing.T) {
	t.Parallel()

	var sawRequestID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRequestID, _ = r.Context().Value(requestIDKey{}).(string)
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("hi"))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	requestLog(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTeapot)
	}
	id := rec.Header().Get("X-Request-ID")
	if len(id) != 32 {
		t.Fatalf("X-Request-ID = %q, want 32 hex chars", id)
	}
	if sawRequestID != id {
		t.Fatalf("context request id = %q, want %q", sawRequestID, id)
	}
}

func TestGenerateRequestIDUniqueAndHex(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{})
	for range 100 {
		id := generateRequestID()
		if len(id) != 32 {
			t.Fatalf("id = %q, want 32 hex chars", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestStatusRecorderWriteImplicitOK(t *testing.T) {
	t.Parallel()

	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	if _, err := rec.Write([]byte("body")); err != nil {
		t.Fatalf("Write error = %v", err)
	}
	if rec.status != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.status)
	}
	if !rec.wroteHeader {
		t.Fatal("wroteHeader = false, want true after Write")
	}
}

func TestStatusRecorderWriteHeaderOnce(t *testing.T) {
	t.Parallel()

	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	rec.WriteHeader(http.StatusNotFound)
	rec.WriteHeader(http.StatusInternalServerError)
	if rec.status != http.StatusNotFound {
		t.Fatalf("status = %d, want first write 404", rec.status)
	}
}

func TestStatusRecorderUnwrap(t *testing.T) {
	t.Parallel()

	inner := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: inner}
	if rec.Unwrap() != inner {
		t.Fatal("Unwrap did not return the underlying ResponseWriter")
	}
}

type hijackableWriter struct {
	http.ResponseWriter
	hijacked bool
}

func (h *hijackableWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	client, _ := net.Pipe()
	return client, nil, nil
}

func TestStatusRecorderHijackSuccess(t *testing.T) {
	t.Parallel()

	hj := &hijackableWriter{ResponseWriter: httptest.NewRecorder()}
	rec := &statusRecorder{ResponseWriter: hj}
	conn, _, err := rec.Hijack()
	if err != nil {
		t.Fatalf("Hijack error = %v", err)
	}
	if conn != nil {
		_ = conn.Close()
	}
	if !hj.hijacked {
		t.Fatal("underlying Hijack was not called")
	}
}

func TestStatusRecorderHijackUnsupported(t *testing.T) {
	t.Parallel()

	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder()}
	if _, _, err := rec.Hijack(); err == nil {
		t.Fatal("Hijack error = nil, want error for non-hijacker writer")
	}
}

func TestInitLogger(t *testing.T) {
	for _, level := range []string{"debug", "warn", "error", "info", "unknown"} {
		closeLogger, err := initLogger(level, "")
		if err != nil {
			t.Fatalf("initLogger(%q) error = %v", level, err)
		}
		closeLogger()
	}
}

func TestStartMetricsTickerStopsOnCancel(t *testing.T) {
	t.Parallel()

	hub := events.NewHub()
	mgr := services.NewManager(time.Now(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := startMetricsTicker(ctx, mgr, hub)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("metrics ticker did not stop after cancel")
	}
}

func TestStartStoreTickersStopOnCancel(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	hub := events.NewHub()

	tickers := map[string]func(context.Context) <-chan struct{}{
		"alerts":   func(c context.Context) <-chan struct{} { return startAlertsTicker(c, st, hub) },
		"activity": func(c context.Context) <-chan struct{} { return startActivityTicker(c, st, hub) },
		"prune":    func(c context.Context) <-chan struct{} { return startOpsPruneTicker(c, st) },
	}
	for name, start := range tickers {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			done := start(ctx)
			cancel()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatalf("%s ticker did not stop after cancel", name)
			}
		})
	}
}

func TestLoopTickerRunsTickThenStops(t *testing.T) {
	t.Parallel()

	ticks := make(chan struct{}, 8)
	ctx, cancel := context.WithCancel(context.Background())
	done := loopTicker(ctx, 5*time.Millisecond, func() {
		select {
		case ticks <- struct{}{}:
		default:
		}
	})
	select {
	case <-ticks:
	case <-time.After(2 * time.Second):
		t.Fatal("tick function was never invoked")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("loopTicker did not stop after cancel")
	}
}

func TestPublishMetrics(t *testing.T) {
	t.Parallel()

	hub := events.NewHub()
	ch, unsub := hub.Subscribe(4)
	defer unsub()
	publishMetrics(context.Background(), services.NewManager(time.Now(), nil), hub)
	select {
	case ev := <-ch:
		if ev.Type != events.TypeOpsMetrics {
			t.Fatalf("event type = %q, want %q", ev.Type, events.TypeOpsMetrics)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no metrics event published")
	}
}

func TestPublishAlertsIfChanged(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	hub := events.NewHub()
	ch, unsub := hub.Subscribe(4)
	defer unsub()
	ctx := context.Background()

	// A lastRev that cannot match the real revision forces a publish.
	rev := publishAlertsIfChanged(ctx, st, hub, -1)
	select {
	case ev := <-ch:
		if ev.Type != events.TypeOpsAlerts {
			t.Fatalf("event type = %q, want %q", ev.Type, events.TypeOpsAlerts)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no alerts event published on revision change")
	}

	// Passing the now-current revision must not publish again.
	if got := publishAlertsIfChanged(ctx, st, hub, rev); got != rev {
		t.Fatalf("unchanged revision returned %d, want %d", got, rev)
	}
	select {
	case ev := <-ch:
		t.Fatalf("unexpected event published for unchanged revision: %q", ev.Type)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestPublishActivityIfChanged(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	hub := events.NewHub()
	ch, unsub := hub.Subscribe(4)
	defer unsub()
	ctx := context.Background()

	rev := publishActivityIfChanged(ctx, st, hub, -1)
	select {
	case ev := <-ch:
		if ev.Type != events.TypeOpsActivity {
			t.Fatalf("event type = %q, want %q", ev.Type, events.TypeOpsActivity)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no activity event published on revision change")
	}

	if got := publishActivityIfChanged(ctx, st, hub, rev); got != rev {
		t.Fatalf("unchanged revision returned %d, want %d", got, rev)
	}
}

func TestPruneOpsActivity(t *testing.T) {
	t.Parallel()

	pruneOpsActivity(context.Background(), newTestStore(t))
}

func TestTickHandlersWithClosedStore(t *testing.T) {
	t.Parallel()

	st, err := store.New(filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	_ = st.Close()
	hub := events.NewHub()
	ctx := context.Background()

	if got := publishAlertsIfChanged(ctx, st, hub, 7); got != 7 {
		t.Fatalf("closed-store alerts returned %d, want lastRev 7", got)
	}
	if got := publishActivityIfChanged(ctx, st, hub, 9); got != 9 {
		t.Fatalf("closed-store activity returned %d, want lastRev 9", got)
	}
	pruneOpsActivity(ctx, st)
}

func TestRunFailsOnInvalidListenAddr(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Server.Host = "localhost"
	cfg.Server.Port = 999999
	if code := run("test-version", cfg, http.NewServeMux()); code != 1 {
		t.Fatalf("run() = %d, want 1 for an invalid listen address", code)
	}
}

// TestServeBootsAndShutsDown drives the full Serve bootstrap: config, store,
// service registration, background workers and the LIFO shutdown. An invalid
// listen address makes the embedded HTTP server fail fast so Serve returns
// without needing a shutdown signal.
func TestServeBootsAndShutsDown(t *testing.T) {
	t.Setenv("SENTINEL_SERVER_HOST", "localhost")
	t.Setenv("SENTINEL_SERVER_PORT", "999999")
	t.Setenv("SENTINEL_DATA_DIR", t.TempDir())

	if code := Serve("test-version"); code != 1 {
		t.Fatalf("Serve() = %d, want 1 for an invalid listen address", code)
	}
}

func TestServeFailsOnInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)

	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte("[log]\nlevel = \"verbose\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if code := Serve("test-version"); code != 1 {
		t.Fatalf("Serve() = %d, want 1 for invalid config", code)
	}
}
