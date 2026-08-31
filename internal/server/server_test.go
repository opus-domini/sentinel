package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/services"
)

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
	ctx, cancel := context.WithCancel(context.Background())
	done := startMetricsTicker(ctx, staticMetricsProvider{}, hub)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("metrics ticker did not stop after cancel")
	}
}

func TestStartStoreTickersStopOnCancel(t *testing.T) {
	t.Parallel()

	tickers := map[string]func(context.Context) <-chan struct{}{
		"metrics": func(c context.Context) <-chan struct{} {
			return startMetricsTicker(c, staticMetricsProvider{}, events.NewHub())
		},
		"services": func(c context.Context) <-chan struct{} {
			return startServicesWatcher(c, staticServicesProvider{}, events.NewHub())
		},
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
	publishMetrics(context.Background(), staticMetricsProvider{}, hub, &postureEventState{})
	select {
	case ev := <-ch:
		if ev.Type != events.TypeOpsMetrics {
			t.Fatalf("event type = %q, want %q", ev.Type, events.TypeOpsMetrics)
		}
		posture, ok := ev.Payload["posture"].(services.MetricPosture)
		if !ok {
			t.Fatalf("event posture = %T, want services.MetricPosture", ev.Payload["posture"])
		}
		if posture.State != services.MetricPostureStateNormal ||
			posture.Severity != services.MetricPostureSeverityOK {
			t.Fatalf("event posture = %+v, want normal/ok", posture)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no metrics event published")
	}
}

func TestPublishMetricsSeparatesRawSamplesFromSemanticPosture(t *testing.T) {
	t.Parallel()

	hub := events.NewHub()
	ch, unsub := hub.Subscribe(8)
	defer unsub()

	provider := &sequenceMetricsProvider{
		snapshots: []services.MetricsSnapshot{
			metricsSnapshotWithCPU(81, services.MetricPostureSeverityWarning),
			metricsSnapshotWithCPU(84, services.MetricPostureSeverityWarning),
			metricsSnapshotWithCPU(92, services.MetricPostureSeverityCritical),
		},
	}
	state := &postureEventState{}
	for range 3 {
		publishMetrics(context.Background(), provider, hub, state)
	}

	var metricsEvents, postureEvents int
	for range 5 {
		select {
		case event := <-ch:
			switch event.Type {
			case events.TypeOpsMetrics:
				metricsEvents++
			case events.TypeOpsPosture:
				postureEvents++
			default:
				t.Fatalf("unexpected event type %q", event.Type)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for metrics events")
		}
	}
	if metricsEvents != 3 || postureEvents != 2 {
		t.Fatalf(
			"event counts metrics=%d posture=%d, want metrics=3 posture=2",
			metricsEvents,
			postureEvents,
		)
	}
}

func TestMetricPostureSignatureIgnoresValuesTimestampsAndSignalOrder(t *testing.T) {
	t.Parallel()

	first := services.MetricPosture{
		State:      services.MetricPostureStatePressure,
		Severity:   services.MetricPostureSeverityCritical,
		ObservedAt: "2026-07-27T12:00:00Z",
		Signals: []services.MetricPostureSignal{
			{Name: "memory", Severity: services.MetricPostureSeverityCritical, Value: 95, Since: "2026-07-27T11:59:00Z"},
			{Name: "cpu", Severity: services.MetricPostureSeverityWarning, Value: 81, Since: "2026-07-27T11:58:00Z"},
		},
	}
	second := services.MetricPosture{
		State:      services.MetricPostureStatePressure,
		Severity:   services.MetricPostureSeverityCritical,
		ObservedAt: "2026-07-27T12:00:02Z",
		Signals: []services.MetricPostureSignal{
			{Name: "cpu", Severity: services.MetricPostureSeverityWarning, Value: 88, Since: "2026-07-27T11:58:00Z"},
			{Name: "memory", Severity: services.MetricPostureSeverityCritical, Value: 99, Since: "2026-07-27T11:59:00Z"},
		},
	}
	if got, want := metricPostureSignature(second), metricPostureSignature(first); got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
}

func TestMetricPostureSignatureTracksSensorSubjectChanges(t *testing.T) {
	t.Parallel()

	first := services.MetricPosture{
		State:    services.MetricPostureStatePressure,
		Severity: services.MetricPostureSeverityCritical,
		Signals: []services.MetricPostureSignal{
			{Name: "temperature", Subject: "CPU package", Severity: services.MetricPostureSeverityCritical},
		},
	}
	second := first
	second.Signals = []services.MetricPostureSignal{
		{Name: "temperature", Subject: "NVMe composite", Severity: services.MetricPostureSeverityCritical},
	}
	if metricPostureSignature(first) == metricPostureSignature(second) {
		t.Fatal("sensor subject change did not change posture signature")
	}
}

func TestServicesWatcherPublishesOnlyCanonicalStateChanges(t *testing.T) {
	t.Parallel()

	hub := events.NewHub()
	ch, unsubscribe := hub.Subscribe(4)
	defer unsubscribe()
	state := &servicesEventState{}
	provider := &sequenceServicesProvider{
		snapshots: [][]services.ServiceStatus{
			{
				{Name: "sentinel", ActiveState: "active", EnabledState: "enabled"},
				{Name: "redis", ActiveState: "active", EnabledState: "enabled"},
			},
			{
				{Name: "sentinel", ActiveState: "failed", EnabledState: "enabled"},
				{Name: "redis", ActiveState: "active", EnabledState: "enabled"},
			},
			{
				{Name: "redis", ActiveState: "active", EnabledState: "enabled"},
				{Name: "sentinel", ActiveState: "failed", EnabledState: "enabled"},
			},
		},
	}

	for range 3 {
		if err := publishServicesIfChanged(context.Background(), provider, hub, state); err != nil {
			t.Fatal(err)
		}
	}

	select {
	case event := <-ch:
		if event.Type != events.TypeOpsServices {
			t.Fatalf("event type = %q", event.Type)
		}
		observed, ok := event.Payload["services"].([]services.ServiceStatus)
		if !ok || len(observed) != 2 || observed[0].ActiveState != "failed" {
			t.Fatalf("event services = %#v", event.Payload["services"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("service change event was not published")
	}
	select {
	case event := <-ch:
		t.Fatalf("unchanged canonical state published %q", event.Type)
	default:
	}
}

func TestTickHandlersWithClosedStore(t *testing.T) {
	t.Parallel()

	hub := events.NewHub()
	ctx := context.Background()

	publishMetrics(ctx, staticMetricsProvider{}, hub, &postureEventState{})
}

type staticMetricsProvider struct{}

func (staticMetricsProvider) MetricsSnapshot(context.Context) services.MetricsSnapshot {
	return services.MetricsSnapshot{
		Metrics: services.HostMetrics{
			CPUPercent:       20,
			CPUPressureAvg10: -1,
			MemPressureAvg10: -1,
			IOPressureAvg10:  -1,
			CollectedAt:      "2026-07-24T12:00:00Z",
		},
		Posture: services.MetricPosture{
			State:      services.MetricPostureStateNormal,
			Severity:   services.MetricPostureSeverityOK,
			Signals:    []services.MetricPostureSignal{},
			ObservedAt: "2026-07-24T12:00:00Z",
		},
	}
}

type staticServicesProvider struct{}

func (staticServicesProvider) ListServices(context.Context) ([]services.ServiceStatus, error) {
	return []services.ServiceStatus{}, nil
}

type sequenceServicesProvider struct {
	snapshots [][]services.ServiceStatus
	index     int
}

func (p *sequenceServicesProvider) ListServices(context.Context) ([]services.ServiceStatus, error) {
	snapshot := p.snapshots[p.index]
	p.index++
	return snapshot, nil
}

type sequenceMetricsProvider struct {
	snapshots []services.MetricsSnapshot
	index     int
}

func (p *sequenceMetricsProvider) MetricsSnapshot(context.Context) services.MetricsSnapshot {
	snapshot := p.snapshots[p.index]
	p.index++
	return snapshot
}

func metricsSnapshotWithCPU(value float64, severity string) services.MetricsSnapshot {
	return services.MetricsSnapshot{
		Metrics: services.HostMetrics{CPUPercent: value},
		Posture: services.MetricPosture{
			State:      services.MetricPostureStatePressure,
			Severity:   severity,
			ObservedAt: "2026-07-24T12:00:00Z",
			Signals: []services.MetricPostureSignal{
				{
					Name:     "cpu",
					Severity: severity,
					Value:    value,
					Since:    "2026-07-24T11:59:00Z",
				},
			},
		},
	}
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
	// Reserve an ephemeral port and point Serve() at it so the listen step is
	// guaranteed to fail with "address already in use", exercising the
	// error-return path deterministically. The previous fixture used an
	// out-of-range port (999999), but config sanitizes that back to the
	// default port — so the server actually booted and Serve() blocked forever
	// whenever the default port happened to be free (e.g. on CI), only passing
	// locally because a real instance already held the default port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer func() { _ = ln.Close() }()

	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}

	t.Setenv("SENTINEL_SERVER_HOST", host)
	t.Setenv("SENTINEL_SERVER_PORT", port)
	t.Setenv("SENTINEL_DATA_DIR", t.TempDir())
	// This test exercises the server lifecycle, not Watchtower collection.
	// Keep Watchtower disabled so its startup goroutine cannot reach tmux
	// before the deliberately occupied listener makes Serve return.
	t.Setenv("SENTINEL_WATCHTOWER_ENABLED", "false")

	if code := Serve("test-version"); code != 1 {
		t.Fatalf("Serve() = %d, want 1 when the listen address is already in use", code)
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

// freeLoopbackConfig returns a config pointing at a loopback port that was
// free at the moment of the call.
func freeLoopbackConfig(t *testing.T) config.Config {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("release port: %v", err)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	portNum, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	cfg := config.Default()
	cfg.Server.Host = host
	cfg.Server.Port = portNum
	return cfg
}

// waitForListener blocks until addr answers an HTTP request or the deadline
// expires, so a test only drives the server once it is really accepting.
func waitForListener(t *testing.T, addr string) {
	t.Helper()

	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://" + addr + "/ready")
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server at %s never started listening", addr)
}

func readyMux(t *testing.T) *http.ServeMux {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

// TestRunServerWaitsForInFlightRequest is the regression guard for the drain
// window: http.Server.Shutdown closes the listeners first, so ListenAndServe
// returns ErrServerClosed while handlers are still running. If runServer
// returns at that point, Serve's teardown (store close, handler stop) races
// live handlers and the in-flight request is truncated.
func TestRunServerWaitsForInFlightRequest(t *testing.T) {
	t.Parallel()

	cfg := freeLoopbackConfig(t)
	entered := make(chan struct{})
	var handlerFinished atomic.Bool

	mux := readyMux(t)
	mux.HandleFunc("GET /slow", func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		time.Sleep(300 * time.Millisecond)
		handlerFinished.Store(true)
		_, _ = w.Write([]byte("drained"))
	})

	shutdownCh := make(chan os.Signal, 1)
	exit := make(chan int, 1)
	go func() { exit <- runServer("test-version", cfg, mux, shutdownCh) }()

	waitForListener(t, cfg.Address())

	body := make(chan string, 1)
	go func() {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get("http://" + cfg.Address() + "/slow")
		if err != nil {
			body <- "request error: " + err.Error()
			return
		}
		defer func() { _ = resp.Body.Close() }()
		payload, err := io.ReadAll(resp.Body)
		if err != nil {
			body <- "read error: " + err.Error()
			return
		}
		body <- string(payload)
	}()

	<-entered
	shutdownCh <- syscall.SIGTERM

	select {
	case code := <-exit:
		if code != 0 {
			t.Fatalf("runServer() = %d, want 0 after a shutdown signal", code)
		}
		if !handlerFinished.Load() {
			t.Fatal("runServer returned while a request was still in flight; the graceful drain is not awaited")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runServer did not return after the shutdown signal")
	}

	select {
	case got := <-body:
		if got != "drained" {
			t.Fatalf("in-flight response = %q, want %q", got, "drained")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight request never completed")
	}
}

func TestRunServerReturnsPromptlyWhenIdle(t *testing.T) {
	t.Parallel()

	cfg := freeLoopbackConfig(t)
	shutdownCh := make(chan os.Signal, 1)
	exit := make(chan int, 1)
	go func() { exit <- runServer("test-version", cfg, readyMux(t), shutdownCh) }()

	waitForListener(t, cfg.Address())
	shutdownCh <- syscall.SIGINT

	select {
	case code := <-exit:
		if code != 0 {
			t.Fatalf("runServer() = %d, want 0 after a shutdown signal", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("idle runServer did not return promptly after the shutdown signal")
	}
}

// TestRunServerShutdownDoesNotWaitForHijackedConnections pins the documented
// contract that bounds the drain: Shutdown neither closes nor waits for
// hijacked connections (the /ws/* routes), so an open WebSocket must not keep
// runServer blocked for the whole 10s budget.
func TestRunServerShutdownDoesNotWaitForHijackedConnections(t *testing.T) {
	t.Parallel()

	cfg := freeLoopbackConfig(t)
	hijacked := make(chan net.Conn, 1)

	mux := readyMux(t)
	mux.HandleFunc("GET /hijack", func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("Hijack error = %v", err)
			return
		}
		hijacked <- conn
	})

	shutdownCh := make(chan os.Signal, 1)
	exit := make(chan int, 1)
	go func() { exit <- runServer("test-version", cfg, mux, shutdownCh) }()

	waitForListener(t, cfg.Address())

	client, err := net.Dial("tcp", cfg.Address())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = client.Close() }()
	if _, err := fmt.Fprintf(client, "GET /hijack HTTP/1.1\r\nHost: %s\r\n\r\n", cfg.Address()); err != nil {
		t.Fatalf("write request: %v", err)
	}

	var serverConn net.Conn
	select {
	case serverConn = <-hijacked:
		defer func() { _ = serverConn.Close() }()
	case <-time.After(5 * time.Second):
		t.Fatal("handler never hijacked the connection")
	}

	shutdownCh <- syscall.SIGTERM

	select {
	case code := <-exit:
		if code != 0 {
			t.Fatalf("runServer() = %d, want 0 after a shutdown signal", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runServer waited on a hijacked connection instead of cutting it")
	}
}
