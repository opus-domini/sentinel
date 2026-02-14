package httpui

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/term"
	"github.com/opus-domini/sentinel/internal/ws"
)

type fakePingConn struct {
	writes atomic.Int32
	err    error
}

const testSessionName = "dev"

func (f *fakePingConn) WritePing(_ []byte) error {
	f.writes.Add(1)
	return f.err
}

func TestRunPingLoopStopsOnContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time, 2)
	conn := &fakePingConn{}
	errCh := make(chan error, 1)
	done := make(chan struct{})

	go func() {
		runPingLoop(ctx, conn, ticks, func(err error) {
			select {
			case errCh <- err:
			default:
			}
		})
		close(done)
	}()

	ticks <- time.Now()

	deadline := time.Now().Add(500 * time.Millisecond)
	for conn.writes.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if conn.writes.Load() != 1 {
		t.Fatalf("writes = %d, want 1", conn.writes.Load())
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runPingLoop did not stop after context cancellation")
	}

	select {
	case err := <-errCh:
		t.Fatalf("unexpected error reported: %v", err)
	default:
	}
}

func TestRunPingLoopReportsPingError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ticks := make(chan time.Time, 1)
	pingErr := errors.New("ping failed")
	conn := &fakePingConn{err: pingErr}
	gotErr := make(chan error, 1)
	done := make(chan struct{})

	go func() {
		runPingLoop(ctx, conn, ticks, func(err error) {
			select {
			case gotErr <- err:
			default:
			}
		})
		close(done)
	}()

	ticks <- time.Now()

	select {
	case err := <-gotErr:
		if !errors.Is(err, pingErr) {
			t.Fatalf("reported error = %v, want %v", err, pingErr)
		}
	case <-time.After(time.Second):
		t.Fatal("expected ping error to be reported")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runPingLoop did not stop after ping error")
	}
}

func TestStartTmuxPTYEnablesMouseBeforeAttach(t *testing.T) {
	originalEnsureMouse := tmuxEnsureWebMouse
	originalMouse := tmuxSetSessionMouse
	originalAttach := startTmuxAttachFn
	t.Cleanup(func() {
		tmuxEnsureWebMouse = originalEnsureMouse
		tmuxSetSessionMouse = originalMouse
		startTmuxAttachFn = originalAttach
	})
	tmuxEnsureWebMouse = func(_ context.Context) error { return nil }

	callOrder := make([]string, 0, 2)
	tmuxSetSessionMouse = func(_ context.Context, session string, enabled bool) error {
		if session != testSessionName {
			t.Fatalf("session = %q, want %q", session, testSessionName)
		}
		if !enabled {
			t.Fatal("enabled = false, want true")
		}
		callOrder = append(callOrder, "mouse")
		return nil
	}

	wantPTY := &term.PTY{}
	startTmuxAttachFn = func(_ context.Context, session string, cols, rows int) (*term.PTY, error) {
		if session != testSessionName {
			t.Fatalf("session = %q, want %q", session, testSessionName)
		}
		if cols != defaultTermCols || rows != defaultTermRows {
			t.Fatalf("terminal size = %dx%d, want %dx%d", cols, rows, defaultTermCols, defaultTermRows)
		}
		callOrder = append(callOrder, "attach")
		return wantPTY, nil
	}

	h := &Handler{}
	got, err := h.startTmuxPTY(context.Background(), testSessionName)
	if err != nil {
		t.Fatalf("startTmuxPTY error = %v", err)
	}
	if got != wantPTY {
		t.Fatalf("startTmuxPTY returned unexpected PTY pointer")
	}
	if !slices.Equal(callOrder, []string{"mouse", "attach"}) {
		t.Fatalf("call order = %v, want [mouse attach]", callOrder)
	}
}

func TestStartTmuxPTYContinuesWhenMouseEnableFails(t *testing.T) {
	originalEnsureMouse := tmuxEnsureWebMouse
	originalMouse := tmuxSetSessionMouse
	originalAttach := startTmuxAttachFn
	t.Cleanup(func() {
		tmuxEnsureWebMouse = originalEnsureMouse
		tmuxSetSessionMouse = originalMouse
		startTmuxAttachFn = originalAttach
	})
	tmuxEnsureWebMouse = func(_ context.Context) error { return nil }

	callOrder := make([]string, 0, 2)
	tmuxSetSessionMouse = func(_ context.Context, _ string, _ bool) error {
		callOrder = append(callOrder, "mouse")
		return errors.New("set-option failed")
	}

	wantPTY := &term.PTY{}
	startTmuxAttachFn = func(_ context.Context, _ string, _ int, _ int) (*term.PTY, error) {
		callOrder = append(callOrder, "attach")
		return wantPTY, nil
	}

	h := &Handler{}
	got, err := h.startTmuxPTY(context.Background(), testSessionName)
	if err != nil {
		t.Fatalf("startTmuxPTY error = %v", err)
	}
	if got != wantPTY {
		t.Fatalf("startTmuxPTY returned unexpected PTY pointer")
	}
	if !slices.Equal(callOrder, []string{"mouse", "attach"}) {
		t.Fatalf("call order = %v, want [mouse attach]", callOrder)
	}
}

func TestAttachWSIntegrationEnablesMouseBeforeAttach(t *testing.T) {
	originalExists := tmuxSessionExistsFn
	originalEnsureMouse := tmuxEnsureWebMouse
	originalMouse := tmuxSetSessionMouse
	originalAttach := startTmuxAttachFn
	t.Cleanup(func() {
		tmuxSessionExistsFn = originalExists
		tmuxEnsureWebMouse = originalEnsureMouse
		tmuxSetSessionMouse = originalMouse
		startTmuxAttachFn = originalAttach
	})
	tmuxEnsureWebMouse = func(_ context.Context) error { return nil }

	tmuxSessionExistsFn = func(_ context.Context, session string) (bool, error) {
		if session != testSessionName {
			t.Fatalf("session = %q, want %q", session, testSessionName)
		}
		return true, nil
	}

	var mu sync.Mutex
	callOrder := make([]string, 0, 2)
	tmuxSetSessionMouse = func(_ context.Context, session string, enabled bool) error {
		if session != testSessionName {
			t.Fatalf("session = %q, want %q", session, testSessionName)
		}
		if !enabled {
			t.Fatal("enabled = false, want true")
		}
		mu.Lock()
		callOrder = append(callOrder, "mouse")
		mu.Unlock()
		return nil
	}
	startTmuxAttachFn = func(ctx context.Context, session string, cols, rows int) (*term.PTY, error) {
		if session != testSessionName {
			t.Fatalf("session = %q, want %q", session, testSessionName)
		}
		if cols != defaultTermCols || rows != defaultTermRows {
			t.Fatalf("terminal size = %dx%d, want %dx%d", cols, rows, defaultTermCols, defaultTermRows)
		}
		mu.Lock()
		callOrder = append(callOrder, "attach")
		mu.Unlock()
		return term.StartShell(ctx, "/bin/sh", cols, rows)
	}

	h := &Handler{guard: security.New("", nil)}
	srv := httptest.NewServer(http.HandlerFunc(h.attachWS))
	defer srv.Close()

	conn := dialWebSocketPath(t, srv.URL, "/ws/tmux?session="+testSessionName)
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	opcode, payload, err := readServerFrame(conn)
	if err != nil {
		t.Fatalf("readServerFrame error = %v", err)
	}
	if opcode != ws.OpText {
		t.Fatalf("opcode = %d, want %d (text)", opcode, ws.OpText)
	}
	var status map[string]any
	if err := json.Unmarshal(payload, &status); err != nil {
		t.Fatalf("status payload is not JSON: %v", err)
	}
	if status["type"] != "status" || status["state"] != "attached" || status["session"] != testSessionName {
		t.Fatalf("unexpected status payload: %s", string(payload))
	}

	mu.Lock()
	gotOrder := append([]string{}, callOrder...)
	mu.Unlock()
	if !slices.Equal(gotOrder, []string{"mouse", "attach"}) {
		t.Fatalf("call order = %v, want [mouse attach]", gotOrder)
	}
}

func TestAttachWSIntegrationContinuesWhenMouseEnableFails(t *testing.T) {
	originalExists := tmuxSessionExistsFn
	originalEnsureMouse := tmuxEnsureWebMouse
	originalMouse := tmuxSetSessionMouse
	originalAttach := startTmuxAttachFn
	t.Cleanup(func() {
		tmuxSessionExistsFn = originalExists
		tmuxEnsureWebMouse = originalEnsureMouse
		tmuxSetSessionMouse = originalMouse
		startTmuxAttachFn = originalAttach
	})
	tmuxEnsureWebMouse = func(_ context.Context) error { return nil }

	tmuxSessionExistsFn = func(_ context.Context, _ string) (bool, error) {
		return true, nil
	}

	var mu sync.Mutex
	callOrder := make([]string, 0, 2)
	tmuxSetSessionMouse = func(_ context.Context, _ string, _ bool) error {
		mu.Lock()
		callOrder = append(callOrder, "mouse")
		mu.Unlock()
		return errors.New("set-option failed")
	}
	startTmuxAttachFn = func(ctx context.Context, _ string, cols, rows int) (*term.PTY, error) {
		mu.Lock()
		callOrder = append(callOrder, "attach")
		mu.Unlock()
		return term.StartShell(ctx, "/bin/sh", cols, rows)
	}

	h := &Handler{guard: security.New("", nil)}
	srv := httptest.NewServer(http.HandlerFunc(h.attachWS))
	defer srv.Close()

	conn := dialWebSocketPath(t, srv.URL, "/ws/tmux?session="+testSessionName)
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	opcode, payload, err := readServerFrame(conn)
	if err != nil {
		t.Fatalf("readServerFrame error = %v", err)
	}
	if opcode != ws.OpText {
		t.Fatalf("opcode = %d, want %d (text)", opcode, ws.OpText)
	}
	var status map[string]any
	if err := json.Unmarshal(payload, &status); err != nil {
		t.Fatalf("status payload is not JSON: %v", err)
	}
	if status["state"] != "attached" {
		t.Fatalf("unexpected status payload: %s", string(payload))
	}

	mu.Lock()
	gotOrder := append([]string{}, callOrder...)
	mu.Unlock()
	if !slices.Equal(gotOrder, []string{"mouse", "attach"}) {
		t.Fatalf("call order = %v, want [mouse attach]", gotOrder)
	}
}

func TestHandleEventsClientMessagePresence(t *testing.T) {
	t.Parallel()

	st := newHTTPUIStore(t)
	h := &Handler{store: st}
	h.handleEventsClientMessage([]byte(`{
		"type":"presence",
		"terminalId":"term-1",
		"session":"dev",
		"windowIndex":1,
		"paneId":"%11",
		"visible":true,
		"focused":true
	}`))

	rows, err := st.ListWatchtowerPresenceBySession(context.Background(), "dev")
	if err != nil {
		t.Fatalf("ListWatchtowerPresenceBySession(dev): %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.TerminalID != "term-1" || row.WindowIndex != 1 || row.PaneID != "%11" {
		t.Fatalf("unexpected presence row: %+v", row)
	}
	if !row.Visible || !row.Focused {
		t.Fatalf("presence flags should be true: %+v", row)
	}
}

func TestHandleEventsClientMessageIgnoresInvalidPresence(t *testing.T) {
	t.Parallel()

	st := newHTTPUIStore(t)
	h := &Handler{store: st}
	h.handleEventsClientMessage([]byte(`{
		"type":"presence",
		"terminalId":"term-1",
		"session":"dev",
		"windowIndex":0,
		"paneId":"11",
		"visible":true,
		"focused":false
	}`))

	rows, err := st.ListWatchtowerPresenceBySession(context.Background(), "dev")
	if err != nil {
		t.Fatalf("ListWatchtowerPresenceBySession(dev): %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows len = %d, want 0", len(rows))
	}
}

func TestHandleEventsClientMessageSeenAck(t *testing.T) {
	t.Parallel()

	st := newHTTPUIStore(t)
	seedWatchtowerSeenState(t, st)
	hub := events.NewHub()
	eventsCh, unsubscribe := hub.Subscribe(8)
	t.Cleanup(unsubscribe)
	h := &Handler{store: st, events: hub}

	ackPayload := h.handleEventsClientMessage([]byte(`{
		"type":"seen",
		"requestId":"req-1",
		"session":"dev",
		"scope":"pane",
		"paneId":"%11"
	}`))
	if len(ackPayload) == 0 {
		t.Fatal("seen ack payload is empty")
	}

	var ack struct {
		Type      string `json:"type"`
		RequestID string `json:"requestId"`
		Session   string `json:"session"`
		Scope     string `json:"scope"`
		PaneID    string `json:"paneId"`
		Acked     bool   `json:"acked"`
		GlobalRev int64  `json:"globalRev"`
		Error     string `json:"error"`
		Patches   []struct {
			Name        string `json:"name"`
			UnreadPanes int    `json:"unreadPanes"`
		} `json:"sessionPatches"`
		InspectorPatches []struct {
			Session string `json:"session"`
			Windows []struct {
				Index int `json:"index"`
			} `json:"windows"`
			Panes []struct {
				PaneID string `json:"paneId"`
			} `json:"panes"`
		} `json:"inspectorPatches"`
	}
	if err := json.Unmarshal(ackPayload, &ack); err != nil {
		t.Fatalf("seen ack json: %v", err)
	}
	if ack.Type != "tmux.seen.ack" || ack.RequestID != "req-1" {
		t.Fatalf("unexpected seen ack identity: %+v", ack)
	}
	if ack.Session != testSessionName || ack.Scope != "pane" || ack.PaneID != "%11" {
		t.Fatalf("unexpected seen ack payload: %+v", ack)
	}
	if !ack.Acked || ack.Error != "" {
		t.Fatalf("expected acked seen ack without error: %+v", ack)
	}
	if len(ack.Patches) != 1 {
		t.Fatalf("seen ack sessionPatches len = %d, want 1", len(ack.Patches))
	}
	if ack.Patches[0].Name != testSessionName || ack.Patches[0].UnreadPanes != 0 {
		t.Fatalf("unexpected seen ack patch: %+v", ack.Patches[0])
	}
	if len(ack.InspectorPatches) != 1 {
		t.Fatalf("seen ack inspectorPatches len = %d, want 1", len(ack.InspectorPatches))
	}
	if ack.InspectorPatches[0].Session != testSessionName {
		t.Fatalf("unexpected seen ack inspector session: %+v", ack.InspectorPatches[0])
	}
	if len(ack.InspectorPatches[0].Windows) != 1 || len(ack.InspectorPatches[0].Panes) != 1 {
		t.Fatalf("unexpected seen ack inspector payload: %+v", ack.InspectorPatches[0])
	}

	panes, err := st.ListWatchtowerPanes(context.Background(), testSessionName)
	if err != nil {
		t.Fatalf("ListWatchtowerPanes(%s): %v", testSessionName, err)
	}
	if len(panes) != 1 {
		t.Fatalf("panes len = %d, want 1", len(panes))
	}
	if panes[0].SeenRevision != panes[0].Revision {
		t.Fatalf("pane seen revision not updated: %+v", panes[0])
	}

	gotTypes := map[string]bool{}
	var sessionsEvent events.Event
	timeout := time.After(500 * time.Millisecond)
	for len(gotTypes) < 2 {
		select {
		case evt := <-eventsCh:
			gotTypes[evt.Type] = true
			if evt.Type == events.TypeTmuxSessions {
				sessionsEvent = evt
			}
		case <-timeout:
			t.Fatalf("did not receive expected seen events, got=%v", gotTypes)
		}
	}
	if !gotTypes[events.TypeTmuxInspector] || !gotTypes[events.TypeTmuxSessions] {
		t.Fatalf("unexpected seen event types: %v", gotTypes)
	}
	rawPatches, ok := sessionsEvent.Payload["sessionPatches"].([]map[string]any)
	if !ok || len(rawPatches) != 1 {
		t.Fatalf("sessions event patches = %T(%v), want len=1", sessionsEvent.Payload["sessionPatches"], sessionsEvent.Payload["sessionPatches"])
	}
	if rawPatches[0]["name"] != testSessionName || rawPatches[0]["unreadPanes"] != 0 {
		t.Fatalf("unexpected sessions event patch: %+v", rawPatches[0])
	}
	rawInspector, ok := sessionsEvent.Payload["inspectorPatches"].([]map[string]any)
	if !ok || len(rawInspector) != 1 {
		t.Fatalf("sessions event inspectorPatches = %T(%v), want len=1", sessionsEvent.Payload["inspectorPatches"], sessionsEvent.Payload["inspectorPatches"])
	}
	if rawInspector[0]["session"] != testSessionName {
		t.Fatalf("unexpected sessions event inspector patch: %+v", rawInspector[0])
	}
}

func TestHandleEventsClientMessageSeenAckValidationError(t *testing.T) {
	t.Parallel()

	st := newHTTPUIStore(t)
	seedWatchtowerSeenState(t, st)
	hub := events.NewHub()
	eventsCh, unsubscribe := hub.Subscribe(2)
	t.Cleanup(unsubscribe)
	h := &Handler{store: st, events: hub}

	ackPayload := h.handleEventsClientMessage([]byte(`{
		"type":"seen",
		"requestId":"req-2",
		"session":"dev",
		"scope":"pane",
		"paneId":"11"
	}`))
	if len(ackPayload) == 0 {
		t.Fatal("seen ack payload is empty")
	}

	var ack struct {
		RequestID string `json:"requestId"`
		Acked     bool   `json:"acked"`
		Error     string `json:"error"`
	}
	if err := json.Unmarshal(ackPayload, &ack); err != nil {
		t.Fatalf("seen ack json: %v", err)
	}
	if ack.RequestID != "req-2" {
		t.Fatalf("requestId = %q, want req-2", ack.RequestID)
	}
	if ack.Acked {
		t.Fatalf("acked = true, want false for invalid pane id")
	}
	if strings.TrimSpace(ack.Error) == "" {
		t.Fatalf("error should be populated on invalid seen payload: %+v", ack)
	}

	panes, err := st.ListWatchtowerPanes(context.Background(), "dev")
	if err != nil {
		t.Fatalf("ListWatchtowerPanes(dev): %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("panes len = %d, want 1", len(panes))
	}
	if panes[0].SeenRevision != 1 {
		t.Fatalf("seen revision changed unexpectedly: %+v", panes[0])
	}

	select {
	case evt := <-eventsCh:
		t.Fatalf("unexpected seen event published: %+v", evt)
	default:
	}
}

func dialWebSocketPath(t *testing.T, serverURL, path string) net.Conn {
	t.Helper()

	addr := strings.TrimPrefix(serverURL, "http://")
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("net.Dial(%s) error = %v", addr, err)
	}

	key := base64.StdEncoding.EncodeToString([]byte("test-websocket-key!"))
	req := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Connection: Upgrade\r\n" +
		"Upgrade: websocket\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Protocol: sentinel.v1\r\n" +
		"\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		_ = conn.Close()
		t.Fatalf("write upgrade request error = %v", err)
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		t.Fatalf("read status line error = %v", err)
	}
	if !strings.Contains(statusLine, "101") {
		_ = conn.Close()
		t.Fatalf("expected 101, got: %s", statusLine)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			_ = conn.Close()
			t.Fatalf("read headers error = %v", err)
		}
		if strings.TrimSpace(line) == "" {
			break
		}
	}

	return &bufferedConn{Conn: conn, reader: reader}
}

func newHTTPUIStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "sentinel.db"))
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func seedWatchtowerSeenState(t *testing.T, st *store.Store) {
	t.Helper()
	now := time.Now().UTC()
	if err := st.UpsertWatchtowerSession(context.Background(), store.WatchtowerSessionWrite{
		SessionName:       "dev",
		Attached:          1,
		Windows:           1,
		Panes:             1,
		ActivityAt:        now,
		LastPreview:       "line",
		LastPreviewAt:     now,
		LastPreviewPaneID: "%11",
		UnreadWindows:     1,
		UnreadPanes:       1,
		Rev:               1,
		UpdatedAt:         now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerSession(dev): %v", err)
	}
	if err := st.UpsertWatchtowerWindow(context.Background(), store.WatchtowerWindowWrite{
		SessionName:      "dev",
		WindowIndex:      0,
		Name:             "main",
		Active:           true,
		Layout:           "",
		WindowActivityAt: now,
		UnreadPanes:      1,
		HasUnread:        true,
		Rev:              1,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerWindow(dev,0): %v", err)
	}
	if err := st.UpsertWatchtowerPane(context.Background(), store.WatchtowerPaneWrite{
		PaneID:         "%11",
		SessionName:    "dev",
		WindowIndex:    0,
		PaneIndex:      0,
		Title:          "shell",
		Active:         true,
		TTY:            "/dev/pts/11",
		CurrentPath:    "/tmp",
		StartCommand:   "zsh",
		CurrentCommand: "zsh",
		TailHash:       "hash",
		TailPreview:    "line",
		TailCapturedAt: now,
		Revision:       2,
		SeenRevision:   1,
		ChangedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerPane(dev,%%11): %v", err)
	}
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (bc *bufferedConn) Read(p []byte) (int, error) {
	return bc.reader.Read(p)
}

func readServerFrame(r io.Reader) (byte, []byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	opcode := header[0] & 0x0F
	lengthByte := header[1] & 0x7F

	var payloadLen uint64
	switch {
	case lengthByte < 126:
		payloadLen = uint64(lengthByte)
	case lengthByte == 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return 0, nil, err
		}
		payloadLen = uint64(binary.BigEndian.Uint16(ext[:]))
	case lengthByte == 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return 0, nil, err
		}
		payloadLen = binary.BigEndian.Uint64(ext[:])
	}

	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return 0, nil, err
		}
	}
	return opcode, payload, nil
}
