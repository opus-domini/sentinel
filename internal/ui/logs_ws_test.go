package ui

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/ws"
)

// fakeLogStreamer drives /ws/logs from an in-process pipe so the route's own
// lifecycle — start error, status frame, scanner goroutine, final close
// classification — is exercised without a real journalctl.
type fakeLogStreamer struct {
	stream    io.ReadCloser
	err       error
	byUnit    bool
	gotName   string
	gotUnit   string
	gotScope  string
	gotManage string
}

func (f *fakeLogStreamer) StreamLogs(_ context.Context, name string) (io.ReadCloser, error) {
	f.gotName = name
	if f.err != nil {
		return nil, f.err
	}
	return f.stream, nil
}

func (f *fakeLogStreamer) StreamLogsByUnit(_ context.Context, unit, scope, manager string) (io.ReadCloser, error) {
	f.byUnit = true
	f.gotUnit, f.gotScope, f.gotManage = unit, scope, manager
	if f.err != nil {
		return nil, f.err
	}
	return f.stream, nil
}

func newLogsWSServer(t *testing.T, ops OpsLogStreamer) *httptest.Server {
	t.Helper()
	h := &Handler{guard: security.New("", nil, security.CookieSecureNever), ops: ops}
	srv := httptest.NewServer(http.HandlerFunc(h.attachLogsWS))
	t.Cleanup(srv.Close)
	return srv
}

func readLogsWSJSON(t *testing.T, conn net.Conn) map[string]any {
	t.Helper()
	opcode, payload, err := readServerFrame(conn)
	if err != nil {
		t.Fatalf("readServerFrame error = %v", err)
	}
	if opcode != ws.OpText {
		t.Fatalf("opcode = %d, want %d (text)", opcode, ws.OpText)
	}
	var msg map[string]any
	if err := json.Unmarshal(payload, &msg); err != nil {
		t.Fatalf("payload is not JSON (%s): %v", string(payload), err)
	}
	return msg
}

// A clean end of stream is a normal close, and every scanned line reaches the
// client as its own log frame.
func TestAttachLogsWSStreamsLinesAndClosesNormallyOnEOF(t *testing.T) {
	reader, writer := io.Pipe()
	streamer := &fakeLogStreamer{stream: reader}
	srv := newLogsWSServer(t, streamer)

	conn := dialWebSocketPath(t, srv.URL, "/ws/logs?service=sentinel")
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	if status := readLogsWSJSON(t, conn); status["type"] != "status" || status["state"] != "streaming" {
		t.Fatalf("first frame = %#v, want streaming status", status)
	}

	go func() {
		_, _ = io.WriteString(writer, "first line\nsecond line\n")
		_ = writer.Close()
	}()

	for _, want := range []string{"first line", "second line"} {
		msg := readLogsWSJSON(t, conn)
		if msg["type"] != "log" || msg["line"] != want {
			t.Fatalf("frame = %#v, want log line %q", msg, want)
		}
	}

	opcode, payload, err := readServerFrame(conn)
	if err != nil {
		t.Fatalf("readServerFrame(close) error = %v", err)
	}
	if opcode != wsOpClose {
		t.Fatalf("opcode = %d, want %d (close)", opcode, wsOpClose)
	}
	if code := closeCodeOf(payload); code != ws.CloseNormal {
		t.Fatalf("close code = %d, want %d (normal)", code, ws.CloseNormal)
	}
	if streamer.gotName != "sentinel" {
		t.Fatalf("StreamLogs name = %q, want sentinel", streamer.gotName)
	}
}

// A stream that fails mid-flight must close as an internal error, not as done.
func TestAttachLogsWSClosesInternalOnStreamError(t *testing.T) {
	reader, writer := io.Pipe()
	srv := newLogsWSServer(t, &fakeLogStreamer{stream: reader})

	conn := dialWebSocketPath(t, srv.URL, "/ws/logs?unit=sentinel.service&scope=user&manager=systemd")
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	if status := readLogsWSJSON(t, conn); status["state"] != "streaming" {
		t.Fatalf("first frame = %#v, want streaming status", status)
	}

	go func() {
		_ = writer.CloseWithError(errors.New("journal read failed"))
	}()

	opcode, payload, err := readServerFrame(conn)
	if err != nil {
		t.Fatalf("readServerFrame(close) error = %v", err)
	}
	if opcode != wsOpClose {
		t.Fatalf("opcode = %d, want %d (close)", opcode, wsOpClose)
	}
	if code := closeCodeOf(payload); code != ws.CloseInternal {
		t.Fatalf("close code = %d, want %d (internal)", code, ws.CloseInternal)
	}
}

// When the stream cannot start, the client gets an error frame before the close
// so the SPA can show why, rather than a bare disconnect.
func TestAttachLogsWSWritesErrorFrameWhenStreamStartFails(t *testing.T) {
	streamer := &fakeLogStreamer{err: errors.New("unit not found")}
	srv := newLogsWSServer(t, streamer)

	conn := dialWebSocketPath(t, srv.URL, "/ws/logs?unit=ghost.service&scope=system&manager=systemd")
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	msg := readLogsWSJSON(t, conn)
	if msg["type"] != "error" || msg["message"] != "unit not found" {
		t.Fatalf("frame = %#v, want the stream start error", msg)
	}

	opcode, payload, err := readServerFrame(conn)
	if err != nil {
		t.Fatalf("readServerFrame(close) error = %v", err)
	}
	if opcode != wsOpClose {
		t.Fatalf("opcode = %d, want %d (close)", opcode, wsOpClose)
	}
	if code := closeCodeOf(payload); code != ws.CloseInternal {
		t.Fatalf("close code = %d, want %d (internal)", code, ws.CloseInternal)
	}
	if !streamer.byUnit {
		t.Fatal("unit query did not route to StreamLogsByUnit")
	}
	if streamer.gotUnit != "ghost.service" || streamer.gotScope != "system" || streamer.gotManage != "systemd" {
		t.Fatalf("unit target = (%q, %q, %q), want (ghost.service, system, systemd)",
			streamer.gotUnit, streamer.gotScope, streamer.gotManage)
	}
}

// wsOpClose is the RFC 6455 close opcode; internal/ws keeps its own copy
// unexported.
const wsOpClose byte = 0x8

func closeCodeOf(payload []byte) int {
	if len(payload) < 2 {
		return 0
	}
	return int(payload[0])<<8 | int(payload[1])
}
