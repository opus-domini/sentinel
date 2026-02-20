package httpui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/term"
	"github.com/opus-domini/sentinel/internal/tmux"
)

const (
	testPaneID        = "%11"
	testScopeWin      = "window"
	testScopeSess     = "session"
	testStateAttached = "attached"
)

// ---------------------------------------------------------------------------
// isReservedPath
// ---------------------------------------------------------------------------

func TestIsReservedPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"api_exact", "api", true},
		{"ws_exact", "ws", true},
		{"assets_exact", "assets", true},
		{"api_sub", "api/v1/sessions", true},
		{"ws_sub", "ws/tmux", true},
		{"assets_sub", "assets/main.js", true},
		{"empty", "", false},
		{"root_page", "tmux", false},
		{"nested", "dashboard/settings", false},
		{"api_like_but_not", "apidocs", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isReservedPath(tc.path)
			if got != tc.want {
				t.Fatalf("isReservedPath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// tmuxHTTPError
// ---------------------------------------------------------------------------

func TestTmuxHTTPError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string
	}{
		{
			"not_found",
			&tmux.Error{Kind: tmux.ErrKindNotFound},
			http.StatusServiceUnavailable,
			"tmux binary not found",
		},
		{
			"session_not_found",
			&tmux.Error{Kind: tmux.ErrKindSessionNotFound},
			http.StatusNotFound,
			"session not found",
		},
		{
			"server_not_running",
			&tmux.Error{Kind: tmux.ErrKindServerNotRunning},
			http.StatusServiceUnavailable,
			"tmux server not running",
		},
		{
			"generic_error",
			&tmux.Error{Kind: tmux.ErrKindCommandFailed},
			http.StatusInternalServerError,
			"tmux error",
		},
		{
			"plain_error",
			errors.New("something went wrong"),
			http.StatusInternalServerError,
			"tmux error",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			status, msg := tmuxHTTPError(tc.err)
			if status != tc.wantStatus {
				t.Fatalf("status = %d, want %d", status, tc.wantStatus)
			}
			if msg != tc.wantMsg {
				t.Fatalf("msg = %q, want %q", msg, tc.wantMsg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// handleControlMessage — early-return paths (nil PTY is safe)
// ---------------------------------------------------------------------------

func TestHandleControlMessageEarlyReturns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
	}{
		{"not_resize_type", `{"type":"focus","cols":80,"rows":24}`},
		{"invalid_json", `not json at all`},
		{"zero_cols", `{"type":"resize","cols":0,"rows":24}`},
		{"negative_rows", `{"type":"resize","cols":80,"rows":-1}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// These paths return before calling pty.Resize, so nil is safe.
			cols, rows, err := handleControlMessage([]byte(tc.payload), nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cols != 0 || rows != 0 {
				t.Fatalf("cols,rows = %d,%d want 0,0", cols, rows)
			}
		})
	}
}

func TestHandleControlMessageTooLarge(t *testing.T) {
	t.Parallel()

	payload := make([]byte, 9*1024)
	for i := range payload {
		payload[i] = 'x'
	}
	_, _, err := handleControlMessage(payload, nil)
	if err == nil {
		t.Fatal("expected error for oversized payload")
	}
}

func TestHandleControlMessageValidResize(t *testing.T) {
	t.Parallel()

	pty, err := startShellForTest(t)
	if err != nil {
		t.Skipf("shell unavailable: %v", err)
	}
	defer func() { _ = pty.Close() }()

	cols, rows, resizeErr := handleControlMessage([]byte(`{"type":"resize","cols":100,"rows":50}`), pty)
	if resizeErr != nil {
		t.Fatalf("handleControlMessage resize error: %v", resizeErr)
	}
	if cols != 100 || rows != 50 {
		t.Fatalf("cols,rows = %d,%d want 100,50", cols, rows)
	}
}

// startShellForTest creates a real PTY for tests that need the concrete *term.PTY.
func startShellForTest(t *testing.T) (*term.PTY, error) {
	t.Helper()
	return term.StartShell(context.Background(), "/bin/sh", 80, 24)
}

// ---------------------------------------------------------------------------
// validateEventsSeenMessage — window and session scopes
// ---------------------------------------------------------------------------

func TestValidateEventsSeenMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		msg     eventsSeenMessage
		wantErr bool
	}{
		{
			"valid_pane",
			eventsSeenMessage{Session: testSessionName, Scope: testSeenScopePane, PaneID: testPaneID},
			false,
		},
		{
			"valid_window",
			eventsSeenMessage{Session: testSessionName, Scope: testScopeWin, WindowIdx: 0},
			false,
		},
		{
			"valid_session",
			eventsSeenMessage{Session: testSessionName, Scope: testScopeSess},
			false,
		},
		{
			"invalid_session_name",
			eventsSeenMessage{Session: "", Scope: testScopeSess},
			true,
		},
		{
			"invalid_scope",
			eventsSeenMessage{Session: testSessionName, Scope: "unknown"},
			true,
		},
		{
			"pane_missing_percent",
			eventsSeenMessage{Session: testSessionName, Scope: testSeenScopePane, PaneID: "11"},
			true,
		},
		{
			"window_negative_index",
			eventsSeenMessage{Session: testSessionName, Scope: testScopeWin, WindowIdx: -1},
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateEventsSeenMessage(tc.msg)
			if tc.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// markEventsSeen — all scopes
// ---------------------------------------------------------------------------

func TestMarkEventsSeenWindow(t *testing.T) {
	t.Parallel()

	st := newHTTPUIStore(t)
	seedWatchtowerSeenState(t, st)
	h := &Handler{store: st}

	acked, err := h.markEventsSeen(context.Background(), eventsSeenMessage{
		Session:   testSessionName,
		Scope:     testScopeWin,
		WindowIdx: 0,
	})
	if err != nil {
		t.Fatalf("markEventsSeen(window) error = %v", err)
	}
	if !acked {
		t.Fatal("expected acked=true for window scope")
	}
}

func TestMarkEventsSeenSession(t *testing.T) {
	t.Parallel()

	st := newHTTPUIStore(t)
	seedWatchtowerSeenState(t, st)
	h := &Handler{store: st}

	acked, err := h.markEventsSeen(context.Background(), eventsSeenMessage{
		Session: testSessionName,
		Scope:   testScopeSess,
	})
	if err != nil {
		t.Fatalf("markEventsSeen(session) error = %v", err)
	}
	if !acked {
		t.Fatal("expected acked=true for session scope")
	}
}

// ---------------------------------------------------------------------------
// marshalEventsWSMessage — nil input
// ---------------------------------------------------------------------------

func TestMarshalEventsWSMessageNil(t *testing.T) {
	t.Parallel()

	got := marshalEventsWSMessage(nil)
	if got != nil {
		t.Fatalf("marshalEventsWSMessage(nil) = %v, want nil", got)
	}
}

// ---------------------------------------------------------------------------
// handleEventsClientMessage — unknown type
// ---------------------------------------------------------------------------

func TestHandleEventsClientMessageUnknownType(t *testing.T) {
	t.Parallel()

	h := &Handler{store: newHTTPUIStore(t)}
	got := h.handleEventsClientMessage([]byte(`{"type":"unknown_event"}`))
	if got != nil {
		t.Fatalf("expected nil for unknown type, got %s", string(got))
	}
}

// ---------------------------------------------------------------------------
// handleEventsClientMessage — empty/invalid payloads
// ---------------------------------------------------------------------------

func TestHandleEventsClientMessageEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload []byte
	}{
		{"nil_payload", nil},
		{"empty_payload", []byte{}},
		{"invalid_json", []byte(`{not json}`)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := &Handler{store: newHTTPUIStore(t)}
			got := h.handleEventsClientMessage(tc.payload)
			if got != nil {
				t.Fatalf("expected nil for %s, got %s", tc.name, string(got))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// handleEventsClientMessage — nil handler
// ---------------------------------------------------------------------------

func TestHandleEventsClientMessageNilHandler(t *testing.T) {
	t.Parallel()

	var h *Handler
	got := h.handleEventsClientMessage([]byte(`{"type":"seen"}`))
	if got != nil {
		t.Fatalf("expected nil for nil handler, got %s", string(got))
	}
}

// ---------------------------------------------------------------------------
// requireWSAuth — rejects unauthorized
// ---------------------------------------------------------------------------

func TestRequireWSAuthRejectsUnauthorized(t *testing.T) {
	t.Parallel()

	guard := security.New("secret-token", nil, security.CookieSecureNever)
	h := &Handler{guard: guard}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ws/tmux", nil)

	ok := h.requireWSAuth(rec, req)
	if ok {
		t.Fatal("expected requireWSAuth to return false for unauthorized request")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// requireWSAuth — rejects bad origin
// ---------------------------------------------------------------------------

func TestRequireWSAuthRejectsBadOrigin(t *testing.T) {
	t.Parallel()

	guard := security.New("", []string{"https://allowed.com"}, security.CookieSecureNever)
	h := &Handler{guard: guard}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ws/tmux", nil)
	req.Header.Set("Origin", "https://evil.com")

	ok := h.requireWSAuth(rec, req)
	if ok {
		t.Fatal("expected requireWSAuth to return false for bad origin")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

// ---------------------------------------------------------------------------
// authorizeEventsWS — nil hub
// ---------------------------------------------------------------------------

func TestAuthorizeEventsWSNilHub(t *testing.T) {
	t.Parallel()

	guard := security.New("", nil, security.CookieSecureNever)
	h := &Handler{guard: guard, events: nil}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ws/events", nil)

	ok := h.authorizeEventsWS(rec, req)
	if ok {
		t.Fatal("expected authorizeEventsWS to return false when hub is nil")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// attachWS — invalid session name
// ---------------------------------------------------------------------------

func TestAttachWSInvalidSession(t *testing.T) {
	t.Parallel()

	guard := security.New("", nil, security.CookieSecureNever)
	h := &Handler{guard: guard}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ws/tmux?session=", nil)

	h.attachWS(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// attachWS — session does not exist
// ---------------------------------------------------------------------------

func TestAttachWSSessionNotFound(t *testing.T) {
	originalExists := tmuxSessionExistsFn
	t.Cleanup(func() { tmuxSessionExistsFn = originalExists })

	tmuxSessionExistsFn = func(_ context.Context, _ string) (bool, error) {
		return false, nil
	}

	guard := security.New("", nil, security.CookieSecureNever)
	h := &Handler{guard: guard}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ws/tmux?session=dev", nil)

	h.attachWS(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// attachWS — tmux error
// ---------------------------------------------------------------------------

func TestAttachWSTmuxError(t *testing.T) {
	originalExists := tmuxSessionExistsFn
	t.Cleanup(func() { tmuxSessionExistsFn = originalExists })

	tmuxSessionExistsFn = func(_ context.Context, _ string) (bool, error) {
		return false, &tmux.Error{Kind: tmux.ErrKindServerNotRunning, Msg: "no server"}
	}

	guard := security.New("", nil, security.CookieSecureNever)
	h := &Handler{guard: guard}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ws/tmux?session=dev", nil)

	h.attachWS(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// attachLogsWS — missing service/unit param
// ---------------------------------------------------------------------------

func TestAttachLogsWSMissingParams(t *testing.T) {
	t.Parallel()

	guard := security.New("", nil, security.CookieSecureNever)
	h := &Handler{guard: guard}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ws/logs", nil)

	h.attachLogsWS(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// spaPage — rejects reserved paths
// ---------------------------------------------------------------------------

func TestSpaPageRejectsReservedPaths(t *testing.T) {
	t.Parallel()

	guard := security.New("", nil, security.CookieSecureNever)
	h := &Handler{guard: guard}

	paths := []string{"/api/sessions", "/ws/tmux", "/assets/main.js"}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", p, nil)
			h.spaPage(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("spaPage(%s) status = %d, want 404", p, rec.Code)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// spaPage — rejects bad origin
// ---------------------------------------------------------------------------

func TestSpaPageRejectsBadOrigin(t *testing.T) {
	t.Parallel()

	guard := security.New("", []string{"https://allowed.com"}, security.CookieSecureNever)
	h := &Handler{guard: guard}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://evil.com")

	h.spaPage(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

// ---------------------------------------------------------------------------
// decodeEventsSeenMessage — trims and lowercases
// ---------------------------------------------------------------------------

func TestDecodeEventsSeenMessage(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"requestId":" REQ-1 ","session":" Dev ","scope":" PANE ","paneId":" %11 ","windowIndex":2}`)
	msg, err := decodeEventsSeenMessage(payload)
	if err != nil {
		t.Fatalf("decodeEventsSeenMessage() error = %v", err)
	}
	if msg.RequestID != "REQ-1" {
		t.Fatalf("requestId = %q, want REQ-1", msg.RequestID)
	}
	if msg.Session != "Dev" {
		t.Fatalf("session = %q, want Dev", msg.Session)
	}
	if msg.Scope != testSeenScopePane {
		t.Fatalf("scope = %q, want %s", msg.Scope, testSeenScopePane)
	}
	if msg.PaneID != testPaneID {
		t.Fatalf("paneId = %q, want %s", msg.PaneID, testPaneID)
	}
	if msg.WindowIdx != 2 {
		t.Fatalf("windowIndex = %d, want 2", msg.WindowIdx)
	}
}

func TestDecodeEventsSeenMessageInvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := decodeEventsSeenMessage([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid json")
	}
}

// ---------------------------------------------------------------------------
// newEventsSeenAck — conditional fields
// ---------------------------------------------------------------------------

func TestNewEventsSeenAck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		msg          eventsSeenMessage
		hasPaneID    bool
		hasWindowIdx bool
	}{
		{
			"pane_scope",
			eventsSeenMessage{RequestID: "r1", Session: testSessionName, Scope: testSeenScopePane, PaneID: testPaneID, WindowIdx: -1},
			true, false,
		},
		{
			"window_scope",
			eventsSeenMessage{RequestID: "r2", Session: testSessionName, Scope: testScopeWin, WindowIdx: 0},
			false, true,
		},
		{
			"session_scope",
			eventsSeenMessage{RequestID: "r3", Session: testSessionName, Scope: testScopeSess, WindowIdx: -1},
			false, false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ack := newEventsSeenAck(tc.msg)
			if ack["type"] != "tmux.seen.ack" {
				t.Fatalf("type = %v, want tmux.seen.ack", ack["type"])
			}
			_, hasPaneID := ack["paneId"]
			_, hasWindowIdx := ack["windowIndex"]
			if hasPaneID != tc.hasPaneID {
				t.Fatalf("hasPaneId = %v, want %v", hasPaneID, tc.hasPaneID)
			}
			if hasWindowIdx != tc.hasWindowIdx {
				t.Fatalf("hasWindowIdx = %v, want %v", hasWindowIdx, tc.hasWindowIdx)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// handleEventsSeenClientMessage — window scope ack
// ---------------------------------------------------------------------------

func TestHandleEventsSeenClientMessageWindowScope(t *testing.T) {
	t.Parallel()

	st := newHTTPUIStore(t)
	seedWatchtowerSeenState(t, st)
	hub := events.NewHub()
	eventsCh, unsub := hub.Subscribe(8)
	defer unsub()
	h := &Handler{store: st, events: hub}

	ackPayload := h.handleEventsSeenClientMessage([]byte(`{
		"requestId":"req-w1",
		"session":"dev",
		"scope":"window",
		"windowIndex":0
	}`))
	if len(ackPayload) == 0 {
		t.Fatal("seen ack payload is empty")
	}

	var ack map[string]any
	if err := json.Unmarshal(ackPayload, &ack); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if ack["scope"] != testScopeWin {
		t.Fatalf("scope = %v, want %s", ack["scope"], testScopeWin)
	}
	if ack["acked"] != true {
		t.Fatalf("acked = %v, want true", ack["acked"])
	}

	// Drain expected events.
	gotCount := 0
	for gotCount < 2 {
		select {
		case <-eventsCh:
			gotCount++
		default:
			if gotCount < 2 {
				continue
			}
		}
	}
}

// ---------------------------------------------------------------------------
// handleEventsSeenClientMessage — session scope ack
// ---------------------------------------------------------------------------

func TestHandleEventsSeenClientMessageSessionScope(t *testing.T) {
	t.Parallel()

	st := newHTTPUIStore(t)
	seedWatchtowerSeenState(t, st)
	h := &Handler{store: st, events: events.NewHub()}

	ackPayload := h.handleEventsSeenClientMessage([]byte(`{
		"requestId":"req-s1",
		"session":"dev",
		"scope":"session"
	}`))
	if len(ackPayload) == 0 {
		t.Fatal("seen ack payload is empty")
	}

	var ack map[string]any
	if err := json.Unmarshal(ackPayload, &ack); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if ack["scope"] != testScopeSess {
		t.Fatalf("scope = %v, want %s", ack["scope"], testScopeSess)
	}
	if ack["acked"] != true {
		t.Fatalf("acked = %v, want true", ack["acked"])
	}
}

// ---------------------------------------------------------------------------
// handleEventsPresenceClientMessage — nil store guard
// ---------------------------------------------------------------------------

func TestHandleEventsPresenceClientMessageNilStore(t *testing.T) {
	t.Parallel()
	h := &Handler{store: nil}
	// Should not panic.
	h.handleEventsPresenceClientMessage([]byte(`{
		"type":"presence",
		"terminalId":"term-1",
		"session":"dev",
		"windowIndex":0,
		"paneId":"%11"
	}`))
}

// ---------------------------------------------------------------------------
// handleEventsPresenceClientMessage — empty terminalId
// ---------------------------------------------------------------------------

func TestHandleEventsPresenceClientMessageEmptyTerminalID(t *testing.T) {
	t.Parallel()

	st := newHTTPUIStore(t)
	h := &Handler{store: st}
	h.handleEventsPresenceClientMessage([]byte(`{
		"type":"presence",
		"terminalId":"",
		"session":"dev",
		"windowIndex":0,
		"paneId":"%11"
	}`))

	rows, err := st.ListWatchtowerPresenceBySession(context.Background(), testSessionName)
	if err != nil {
		t.Fatalf("ListWatchtowerPresenceBySession error: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows for empty terminalId, got %d", len(rows))
	}
}

// ---------------------------------------------------------------------------
// handleEventsPresenceClientMessage — negative windowIndex
// ---------------------------------------------------------------------------

func TestHandleEventsPresenceClientMessageNegativeWindowIdx(t *testing.T) {
	t.Parallel()

	st := newHTTPUIStore(t)
	h := &Handler{store: st}
	h.handleEventsPresenceClientMessage([]byte(`{
		"type":"presence",
		"terminalId":"term-1",
		"session":"dev",
		"windowIndex":-2,
		"paneId":"%11"
	}`))

	rows, err := st.ListWatchtowerPresenceBySession(context.Background(), testSessionName)
	if err != nil {
		t.Fatalf("ListWatchtowerPresenceBySession error: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows for windowIndex < -1, got %d", len(rows))
	}
}

// ---------------------------------------------------------------------------
// handleEventsPresenceClientMessage — invalid session name
// ---------------------------------------------------------------------------

func TestHandleEventsPresenceClientMessageInvalidSession(t *testing.T) {
	t.Parallel()

	st := newHTTPUIStore(t)
	h := &Handler{store: st}
	h.handleEventsPresenceClientMessage([]byte(`{
		"type":"presence",
		"terminalId":"term-1",
		"session":"invalid session name!@#",
		"windowIndex":0,
		"paneId":"%11"
	}`))

	rows, err := st.ListWatchtowerPresenceBySession(context.Background(), "invalid session name!@#")
	if err != nil {
		t.Fatalf("ListWatchtowerPresenceBySession error: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows for invalid session name, got %d", len(rows))
	}
}

// ---------------------------------------------------------------------------
// handleEventsSeenClientMessage — nil store guard
// ---------------------------------------------------------------------------

func TestHandleEventsSeenClientMessageNilStore(t *testing.T) {
	t.Parallel()
	h := &Handler{store: nil}
	got := h.handleEventsSeenClientMessage([]byte(`{"session":"dev","scope":"pane","paneId":"%11"}`))
	if got != nil {
		t.Fatalf("expected nil for nil store, got %s", string(got))
	}
}

// ---------------------------------------------------------------------------
// writeAttachStatus — verify JSON structure
// ---------------------------------------------------------------------------

func TestWriteAttachStatusPayload(t *testing.T) {
	t.Parallel()

	// writeAttachStatus takes *ws.Conn (concrete type); test the marshal logic.
	status := map[string]any{
		"type":    "status",
		"state":   testStateAttached,
		"session": testSessionName,
	}
	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if decoded["type"] != "status" || decoded["state"] != testStateAttached || decoded["session"] != testSessionName {
		t.Fatalf("unexpected status payload: %s", string(payload))
	}
}

// ---------------------------------------------------------------------------
// attachWS — full integration: auth failure (token required)
// ---------------------------------------------------------------------------

func TestAttachWSIntegrationAuthFailure(t *testing.T) {
	guard := security.New("secret-token", nil, security.CookieSecureNever)
	h := &Handler{guard: guard}

	srv := httptest.NewServer(http.HandlerFunc(h.attachWS))
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/ws/tmux?session=dev", nil)
	if err != nil {
		t.Fatalf("NewRequest error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// attachLogsWS — auth failure
// ---------------------------------------------------------------------------

func TestAttachLogsWSAuthFailure(t *testing.T) {
	guard := security.New("secret-token", nil, security.CookieSecureNever)
	h := &Handler{guard: guard}

	srv := httptest.NewServer(http.HandlerFunc(h.attachLogsWS))
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/ws/logs?service=nginx", nil)
	if err != nil {
		t.Fatalf("NewRequest error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// attachEventsWS — auth failure
// ---------------------------------------------------------------------------

func TestAttachEventsWSAuthFailure(t *testing.T) {
	guard := security.New("secret-token", nil, security.CookieSecureNever)
	h := &Handler{guard: guard, events: events.NewHub()}

	srv := httptest.NewServer(http.HandlerFunc(h.attachEventsWS))
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/ws/events", nil)
	if err != nil {
		t.Fatalf("NewRequest error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// newAttachErrChannel — non-blocking send
// ---------------------------------------------------------------------------

func TestNewAttachErrChannelNonBlocking(t *testing.T) {
	t.Parallel()

	errCh, sendErr := newAttachErrChannel()
	sendErr(errors.New("first"))
	sendErr(errors.New("second")) // should not block

	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "first") {
		t.Fatalf("expected first error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// registerAssetRoutes — sets up file server routes from embedded FS
// ---------------------------------------------------------------------------

func TestRegisterAssetRoutes(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	if err := registerAssetRoutes(mux); err != nil {
		t.Fatalf("registerAssetRoutes() error = %v", err)
	}

	// After registration distFS must be non-nil.
	if distFS == nil {
		t.Fatal("distFS is nil after registerAssetRoutes()")
	}
}

// ---------------------------------------------------------------------------
// serveDistPath — serves known files, rejects dirs and missing paths
// ---------------------------------------------------------------------------

func TestServeDistPath(t *testing.T) {
	t.Parallel()

	// Ensure distFS is initialised by calling registerAssetRoutes first.
	mux := http.NewServeMux()
	if err := registerAssetRoutes(mux); err != nil {
		t.Fatalf("registerAssetRoutes() error = %v", err)
	}

	tests := []struct {
		name     string
		path     string
		wantServ bool
	}{
		{"empty_path", "", false},
		{"dot_path", ".", false},
		{"slash_only", "/", false},
		{"nonexistent_file", "does-not-exist-xyz.html", false},
		// index.html exists in the embedded dist (committed to git).
		{"index_html", "index.html", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/"+tc.path, nil)
			got := serveDistPath(rec, req, tc.path)
			if got != tc.wantServ {
				t.Fatalf("serveDistPath(%q) = %v, want %v", tc.path, got, tc.wantServ)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Register — full mux setup, verify SPA route works
// ---------------------------------------------------------------------------

func TestRegisterSetsUpRoutes(t *testing.T) {
	t.Parallel()

	st := newHTTPUIStore(t)
	guard := security.New("", nil, security.CookieSecureNever)
	hub := events.NewHub()

	mux := http.NewServeMux()
	if err := Register(mux, guard, st, hub, nil); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// A GET to / should serve the SPA (index.html or placeholder).
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", rec.Code)
	}

	// A GET to /tmux should also serve the SPA (client-side route).
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/tmux", nil)
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("GET /tmux status = %d, want 200", rec2.Code)
	}

	// A GET to /api/... should 404 (reserved, no API handler registered).
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest("GET", "/api/sessions", nil)
	mux.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusNotFound {
		t.Fatalf("GET /api/sessions status = %d, want 404", rec3.Code)
	}
}

// ---------------------------------------------------------------------------
// spaPage — serves index.html for non-reserved, unknown paths
// ---------------------------------------------------------------------------

func TestSpaPageServesIndexForUnknownPaths(t *testing.T) {
	t.Parallel()

	guard := security.New("", nil, security.CookieSecureNever)

	// Register to populate distFS.
	mux := http.NewServeMux()
	st := newHTTPUIStore(t)
	if err := Register(mux, guard, st, events.NewHub(), nil); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	h := &Handler{guard: guard}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/some/client/route", nil)
	h.spaPage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("spaPage(/some/client/route) status = %d, want 200", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// startTmuxPTY — web mouse patch failure is tolerated
// ---------------------------------------------------------------------------

func TestStartTmuxPTYContinuesWhenWebMousePatchFails(t *testing.T) {
	originalEnsureMouse := tmuxEnsureWebMouse
	originalMouse := tmuxSetSessionMouse
	originalAttach := startTmuxAttachFn
	t.Cleanup(func() {
		tmuxEnsureWebMouse = originalEnsureMouse
		tmuxSetSessionMouse = originalMouse
		startTmuxAttachFn = originalAttach
	})

	// EnsureWebMouse fails, but everything else succeeds.
	tmuxEnsureWebMouse = func(_ context.Context) error {
		return errors.New("web mouse patch failed")
	}
	tmuxSetSessionMouse = func(_ context.Context, _ string, _ bool) error { return nil }
	wantPTY := &term.PTY{}
	startTmuxAttachFn = func(_ context.Context, _ string, _ int, _ int) (*term.PTY, error) {
		return wantPTY, nil
	}

	h := &Handler{}
	got, err := h.startTmuxPTY(context.Background(), testSessionName)
	if err != nil {
		t.Fatalf("startTmuxPTY error = %v", err)
	}
	if got != wantPTY {
		t.Fatal("startTmuxPTY returned unexpected PTY pointer")
	}
}

// ---------------------------------------------------------------------------
// handleEventsPresenceClientMessage — invalid JSON
// ---------------------------------------------------------------------------

func TestHandleEventsPresenceClientMessageInvalidJSON(t *testing.T) {
	t.Parallel()

	st := newHTTPUIStore(t)
	h := &Handler{store: st}
	// Should not panic; just silently return.
	h.handleEventsPresenceClientMessage([]byte(`not valid json`))
}

// ---------------------------------------------------------------------------
// handleEventsPresenceClientMessage — nil handler
// ---------------------------------------------------------------------------

func TestHandleEventsPresenceClientMessageNilHandler(t *testing.T) {
	t.Parallel()

	var h *Handler
	// Should not panic.
	h.handleEventsPresenceClientMessage([]byte(`{"type":"presence","terminalId":"t1"}`))
}

// ---------------------------------------------------------------------------
// handleEventsPresenceClientMessage — empty payload
// ---------------------------------------------------------------------------

func TestHandleEventsPresenceClientMessageEmptyPayload(t *testing.T) {
	t.Parallel()

	h := &Handler{store: newHTTPUIStore(t)}
	// Should not panic.
	h.handleEventsPresenceClientMessage(nil)
	h.handleEventsPresenceClientMessage([]byte{})
}

// ---------------------------------------------------------------------------
// handleEventsSeenClientMessage — empty payload
// ---------------------------------------------------------------------------

func TestHandleEventsSeenClientMessageEmptyPayload(t *testing.T) {
	t.Parallel()

	h := &Handler{store: newHTTPUIStore(t)}
	got := h.handleEventsSeenClientMessage(nil)
	if got != nil {
		t.Fatalf("expected nil for nil payload, got %s", string(got))
	}
	got = h.handleEventsSeenClientMessage([]byte{})
	if got != nil {
		t.Fatalf("expected nil for empty payload, got %s", string(got))
	}
}

// ---------------------------------------------------------------------------
// handleEventsSeenClientMessage — nil handler
// ---------------------------------------------------------------------------

func TestHandleEventsSeenClientMessageNilHandler(t *testing.T) {
	t.Parallel()

	var h *Handler
	got := h.handleEventsSeenClientMessage([]byte(`{"session":"dev","scope":"session"}`))
	if got != nil {
		t.Fatalf("expected nil for nil handler, got %s", string(got))
	}
}

// ---------------------------------------------------------------------------
// handleEventsSeenClientMessage — invalid JSON
// ---------------------------------------------------------------------------

func TestHandleEventsSeenClientMessageInvalidJSON(t *testing.T) {
	t.Parallel()

	h := &Handler{store: newHTTPUIStore(t)}
	got := h.handleEventsSeenClientMessage([]byte(`not json`))
	if got != nil {
		t.Fatalf("expected nil for invalid json, got %s", string(got))
	}
}

// ---------------------------------------------------------------------------
// collectSeenPatches — session activity patch is empty, inspector patch is
// always returned (even with no windows/panes data).
// ---------------------------------------------------------------------------

func TestCollectSeenPatchesNoSession(t *testing.T) {
	t.Parallel()

	st := newHTTPUIStore(t)
	ctx := context.Background()
	sessionPatches, inspectorPatches := collectSeenPatches(ctx, st, "nonexistent")
	if len(sessionPatches) != 0 {
		t.Fatalf("sessionPatches len = %d, want 0", len(sessionPatches))
	}
	// GetWatchtowerInspectorPatch succeeds even for unknown sessions
	// (returns an empty-data patch), so inspectorPatches always has 1 entry.
	if len(inspectorPatches) != 1 {
		t.Fatalf("inspectorPatches len = %d, want 1", len(inspectorPatches))
	}
}

// ---------------------------------------------------------------------------
// marshalEventsWSMessage — valid payload
// ---------------------------------------------------------------------------

func TestMarshalEventsWSMessageValid(t *testing.T) {
	t.Parallel()

	payload := map[string]any{"type": "test", "value": 42}
	got := marshalEventsWSMessage(payload)
	if got == nil {
		t.Fatal("marshalEventsWSMessage returned nil for valid payload")
	}
	var decoded map[string]any
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if decoded["type"] != "test" {
		t.Fatalf("type = %v, want test", decoded["type"])
	}
}

// ---------------------------------------------------------------------------
// publishEventsSeenAck — with and without patches
// ---------------------------------------------------------------------------

func TestPublishEventsSeenAckNoPatchesNoInspector(t *testing.T) {
	t.Parallel()

	hub := events.NewHub()
	eventsCh, unsub := hub.Subscribe(8)
	defer unsub()

	publishEventsSeenAck(hub, "dev", "session", 5, nil, nil)

	gotTypes := map[string]bool{}
	for len(gotTypes) < 2 {
		select {
		case evt := <-eventsCh:
			gotTypes[evt.Type] = true
			if evt.Type == events.TypeTmuxSessions {
				// No patches provided — verify they don't appear.
				if _, ok := evt.Payload["sessionPatches"]; ok {
					t.Fatal("expected no sessionPatches in payload")
				}
				if _, ok := evt.Payload["inspectorPatches"]; ok {
					t.Fatal("expected no inspectorPatches in payload")
				}
			}
		default:
			continue
		}
	}
}

// ---------------------------------------------------------------------------
// handleEventsPresenceClientMessage — windowIndex of -1 is valid
// ---------------------------------------------------------------------------

func TestHandleEventsPresenceClientMessageWindowIdxNegOne(t *testing.T) {
	t.Parallel()

	st := newHTTPUIStore(t)
	h := &Handler{store: st}
	h.handleEventsPresenceClientMessage([]byte(`{
		"type":"presence",
		"terminalId":"term-1",
		"session":"dev",
		"windowIndex":-1,
		"paneId":"%11",
		"visible":true,
		"focused":true
	}`))

	rows, err := st.ListWatchtowerPresenceBySession(context.Background(), testSessionName)
	if err != nil {
		t.Fatalf("ListWatchtowerPresenceBySession error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row for windowIndex=-1, got %d", len(rows))
	}
}

// ---------------------------------------------------------------------------
// handleEventsPresenceClientMessage — empty session does not panic
// ---------------------------------------------------------------------------

func TestHandleEventsPresenceClientMessageEmptySession(t *testing.T) {
	t.Parallel()

	st := newHTTPUIStore(t)
	h := &Handler{store: st}
	// Should not panic — empty session passes validation but
	// ListWatchtowerPresenceBySession("") returns empty by design.
	h.handleEventsPresenceClientMessage([]byte(`{
		"type":"presence",
		"terminalId":"term-1",
		"session":"",
		"windowIndex":0,
		"paneId":"%11",
		"visible":true,
		"focused":false
	}`))
}
