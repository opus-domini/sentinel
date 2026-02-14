package httpui

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/term"
	"github.com/opus-domini/sentinel/internal/terminals"
	"github.com/opus-domini/sentinel/internal/tmux"
	"github.com/opus-domini/sentinel/internal/validate"
	"github.com/opus-domini/sentinel/internal/ws"
)

const (
	defaultTermCols = 120
	defaultTermRows = 40
)

var (
	tmuxSessionExistsFn = tmux.SessionExists
	tmuxEnsureWebMouse  = tmux.EnsureWebMouseBindings
	tmuxSetSessionMouse = tmux.SetSessionMouse
	startTmuxAttachFn   = term.StartTmuxAttach
)

type Handler struct {
	guard     *security.Guard
	events    *events.Hub
	terminals *terminals.Registry
	store     *store.Store
}

func Register(mux *http.ServeMux, guard *security.Guard, terminalRegistry *terminals.Registry, st *store.Store, eventsHub *events.Hub) error {
	h := &Handler{guard: guard, events: eventsHub, terminals: terminalRegistry, store: st}
	if err := registerAssetRoutes(mux); err != nil {
		return err
	}
	mux.HandleFunc("GET /ws/tmux", h.attachWS)
	mux.HandleFunc("GET /ws/terminals", h.attachTerminalWS)
	mux.HandleFunc("GET /ws/events", h.attachEventsWS)
	mux.HandleFunc("GET /{path...}", h.spaPage)
	return nil
}

func (h *Handler) spaPage(w http.ResponseWriter, r *http.Request) {
	if err := h.guard.CheckOrigin(r); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	urlPath := strings.TrimPrefix(r.URL.Path, "/")
	if isReservedPath(urlPath) {
		http.NotFound(w, r)
		return
	}

	if urlPath != "" && serveDistPath(w, r, urlPath) {
		return
	}
	if serveDistPath(w, r, "index.html") {
		return
	}
	if !serveDistPath(w, r, "index.placeholder.html") {
		http.Error(w, "frontend bundle missing", http.StatusInternalServerError)
	}
}

func (h *Handler) attachWS(w http.ResponseWriter, r *http.Request) {
	if err := h.guard.CheckOrigin(r); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := h.guard.RequireWSToken(r); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	session := strings.TrimSpace(r.URL.Query().Get("session"))
	if !validate.SessionName(session) {
		http.Error(w, "invalid session", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	exists, err := tmuxSessionExistsFn(ctx, session)
	cancel()
	if err != nil {
		status, message := tmuxHTTPError(err)
		http.Error(w, message, status)
		return
	}
	if !exists {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	wsConn, _, err := ws.UpgradeWithSubprotocols(w, r, nil, []string{"sentinel.v1"})
	if err != nil {
		return
	}
	defer func() { _ = wsConn.Close() }()

	h.attachPTY(wsConn, attachPTYOptions{
		label: session,
		startPTY: func(ctx context.Context) (*term.PTY, error) {
			return h.startTmuxPTY(ctx, session)
		},
		statusMsg: map[string]any{
			"type":    "status",
			"state":   "attached",
			"session": session,
		},
		registerFn: func(shutdown func(string)) (string, func()) {
			if h.terminals == nil {
				return "", func() {}
			}
			return h.terminals.Register(
				session,
				strings.TrimSpace(r.RemoteAddr),
				defaultTermCols,
				defaultTermRows,
				shutdown,
			)
		},
	})
}

func (h *Handler) startTmuxPTY(ctx context.Context, session string) (*term.PTY, error) {
	// Best-effort: patch default tmux mouse bindings for web terminals.
	if err := tmuxEnsureWebMouse(ctx); err != nil {
		slog.Warn("tmux web mouse patch failed", "session", session, "err", err)
	}

	// Best-effort: keep wheel events as tmux mouse scroll instead of
	// application ArrowUp/ArrowDown in alternate buffer contexts.
	if err := tmuxSetSessionMouse(ctx, session, true); err != nil {
		slog.Warn("tmux mouse enable failed", "session", session, "err", err)
	}
	return startTmuxAttachFn(ctx, session, defaultTermCols, defaultTermRows)
}

func (h *Handler) attachTerminalWS(w http.ResponseWriter, r *http.Request) {
	if err := h.guard.CheckOrigin(r); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := h.guard.RequireWSToken(r); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	terminalName := strings.TrimSpace(r.URL.Query().Get("terminal"))
	if !validate.SessionName(terminalName) {
		http.Error(w, "invalid terminal", http.StatusBadRequest)
		return
	}

	wsConn, _, err := ws.UpgradeWithSubprotocols(w, r, nil, []string{"sentinel.v1"})
	if err != nil {
		return
	}
	defer func() { _ = wsConn.Close() }()

	h.attachPTY(wsConn, attachPTYOptions{
		label: terminalName,
		startPTY: func(ctx context.Context) (*term.PTY, error) {
			return term.StartShell(ctx, "", defaultTermCols, defaultTermRows)
		},
		statusMsg: map[string]any{
			"type":     "status",
			"state":    "attached",
			"terminal": terminalName,
		},
		registerFn: func(shutdown func(string)) (string, func()) {
			if h.terminals == nil {
				return "", func() {}
			}
			return h.terminals.Register(
				terminalName,
				strings.TrimSpace(r.RemoteAddr),
				defaultTermCols,
				defaultTermRows,
				shutdown,
			)
		},
	})
}

func (h *Handler) attachEventsWS(w http.ResponseWriter, r *http.Request) {
	if err := h.guard.CheckOrigin(r); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := h.guard.RequireWSToken(r); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if h.events == nil {
		http.Error(w, "events unavailable", http.StatusServiceUnavailable)
		return
	}

	wsConn, _, err := ws.UpgradeWithSubprotocols(w, r, nil, []string{"sentinel.v1"})
	if err != nil {
		return
	}
	defer func() { _ = wsConn.Close() }()

	eventsCh, unsubscribe := h.events.Subscribe(64)
	defer unsubscribe()

	readyPayload, _ := json.Marshal(events.NewEvent(events.TypeReady, map[string]any{
		"message": "subscribed",
	}))
	_ = wsConn.WriteText(readyPayload)

	readErrCh := make(chan error, 1)
	sendReadErr := func(err error) {
		select {
		case readErrCh <- err:
		default:
		}
	}
	go func() {
		for {
			opcode, payload, readErr := wsConn.ReadMessage()
			if readErr != nil {
				sendReadErr(readErr)
				return
			}
			if opcode != ws.OpText {
				continue
			}
			if responsePayload := h.handleEventsClientMessage(payload); len(responsePayload) > 0 {
				if err := wsConn.WriteText(responsePayload); err != nil {
					sendReadErr(err)
					return
				}
			}
		}
	}()

	pingTicker := time.NewTicker(20 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case evt, ok := <-eventsCh:
			if !ok {
				return
			}
			payload, marshalErr := json.Marshal(evt)
			if marshalErr != nil {
				continue
			}
			if err := wsConn.WriteText(payload); err != nil {
				return
			}
		case <-pingTicker.C:
			if err := wsConn.WritePing([]byte("k")); err != nil {
				return
			}
		case <-readErrCh:
			return
		}
	}
}

func (h *Handler) handleEventsClientMessage(payload []byte) []byte {
	if h == nil || len(payload) == 0 {
		return nil
	}

	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(envelope.Type)) {
	case "presence":
		h.handleEventsPresenceClientMessage(payload)
		return nil
	case "seen":
		return h.handleEventsSeenClientMessage(payload)
	default:
		return nil
	}
}

func (h *Handler) handleEventsPresenceClientMessage(payload []byte) {
	if h == nil || h.store == nil || len(payload) == 0 {
		return
	}

	var msg struct {
		Type       string `json:"type"`
		TerminalID string `json:"terminalId"`
		Session    string `json:"session"`
		WindowIdx  int    `json:"windowIndex"`
		PaneID     string `json:"paneId"`
		Visible    bool   `json:"visible"`
		Focused    bool   `json:"focused"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil {
		return
	}

	terminalID := strings.TrimSpace(msg.TerminalID)
	sessionName := strings.TrimSpace(msg.Session)
	paneID := strings.TrimSpace(msg.PaneID)
	if terminalID == "" {
		return
	}
	if sessionName != "" && !validate.SessionName(sessionName) {
		return
	}
	if msg.WindowIdx < -1 {
		return
	}
	if paneID != "" && !strings.HasPrefix(paneID, "%") {
		return
	}

	now := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := h.store.UpsertWatchtowerPresence(ctx, store.WatchtowerPresenceWrite{
		TerminalID:  terminalID,
		SessionName: sessionName,
		WindowIndex: msg.WindowIdx,
		PaneID:      paneID,
		Visible:     msg.Visible,
		Focused:     msg.Focused,
		UpdatedAt:   now,
		ExpiresAt:   now.Add(30 * time.Second),
	}); err != nil {
		slog.Warn("events ws presence write failed", "terminal", terminalID, "err", err)
	}
}

func (h *Handler) handleEventsSeenClientMessage(payload []byte) []byte {
	if h == nil || h.store == nil || len(payload) == 0 {
		return nil
	}

	var msg struct {
		RequestID string `json:"requestId"`
		Session   string `json:"session"`
		Scope     string `json:"scope"`
		WindowIdx int    `json:"windowIndex"`
		PaneID    string `json:"paneId"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil {
		return nil
	}

	requestID := strings.TrimSpace(msg.RequestID)
	sessionName := strings.TrimSpace(msg.Session)
	scope := strings.ToLower(strings.TrimSpace(msg.Scope))
	paneID := strings.TrimSpace(msg.PaneID)

	ack := map[string]any{
		"type":      "tmux.seen.ack",
		"requestId": requestID,
		"session":   sessionName,
		"scope":     scope,
		"acked":     false,
		"globalRev": int64(0),
	}
	if paneID != "" {
		ack["paneId"] = paneID
	}
	if msg.WindowIdx >= 0 {
		ack["windowIndex"] = msg.WindowIdx
	}

	if !validate.SessionName(sessionName) {
		ack["error"] = "invalid session name"
		return marshalEventsWSMessage(ack)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	acked := false
	var err error
	switch scope {
	case "pane":
		if !strings.HasPrefix(paneID, "%") {
			ack["error"] = "paneId must start with %"
			return marshalEventsWSMessage(ack)
		}
		acked, err = h.store.MarkWatchtowerPaneSeen(ctx, sessionName, paneID)
	case "window":
		if msg.WindowIdx < 0 {
			ack["error"] = "windowIndex must be >= 0"
			return marshalEventsWSMessage(ack)
		}
		acked, err = h.store.MarkWatchtowerWindowSeen(ctx, sessionName, msg.WindowIdx)
	case "session":
		acked, err = h.store.MarkWatchtowerSessionSeen(ctx, sessionName)
	default:
		ack["error"] = "scope must be pane, window, or session"
		return marshalEventsWSMessage(ack)
	}
	if err != nil {
		ack["error"] = "failed to mark seen"
		slog.Warn("events ws seen write failed", "session", sessionName, "scope", scope, "err", err)
		return marshalEventsWSMessage(ack)
	}

	globalRev := int64(0)
	if raw, getErr := h.store.GetWatchtowerRuntimeValue(ctx, "global_rev"); getErr == nil {
		if parsed, parseErr := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); parseErr == nil {
			globalRev = parsed
		}
	}

	var sessionPatches []map[string]any
	if patch, patchErr := h.store.GetWatchtowerSessionActivityPatch(ctx, sessionName); patchErr == nil {
		sessionPatches = append(sessionPatches, patch)
	}

	ack["acked"] = acked
	ack["globalRev"] = globalRev
	if len(sessionPatches) > 0 {
		ack["sessionPatches"] = sessionPatches
	}

	if acked && h.events != nil {
		h.events.Publish(events.NewEvent(events.TypeTmuxInspector, map[string]any{
			"session": sessionName,
			"action":  "seen",
			"scope":   scope,
		}))
		sessionsPayload := map[string]any{
			"session":   sessionName,
			"action":    "seen",
			"scope":     scope,
			"globalRev": globalRev,
		}
		if len(sessionPatches) > 0 {
			sessionsPayload["sessionPatches"] = sessionPatches
		}
		h.events.Publish(events.NewEvent(events.TypeTmuxSessions, sessionsPayload))
	}

	return marshalEventsWSMessage(ack)
}

func marshalEventsWSMessage(payload map[string]any) []byte {
	if payload == nil {
		return nil
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return buf
}

type attachPTYOptions struct {
	label      string
	startPTY   func(ctx context.Context) (*term.PTY, error)
	statusMsg  map[string]any
	registerFn func(shutdown func(string)) (id string, unregister func())
}

type pingWriter interface {
	WritePing(payload []byte) error
}

func (h *Handler) attachPTY(wsConn *ws.Conn, opts attachPTYOptions) {
	attachCtx, cancelAttach := context.WithCancel(context.Background())
	defer cancelAttach()

	pty, err := opts.startPTY(attachCtx)
	if err != nil {
		slog.Error("pty start failed", "label", opts.label, "err", err)
		_ = wsConn.WriteText([]byte(`{"type":"error","code":"PTY_FAILED","message":"unable to start terminal"}`))
		_ = wsConn.WriteClose(ws.CloseInternal, "pty start failed")
		return
	}
	defer func() { _ = pty.Close() }()

	var shutdownOnce sync.Once
	shutdown := func(reason string) {
		shutdownOnce.Do(func() {
			cancelAttach()
			_ = pty.Close()
			_ = wsConn.Close()
			slog.Info("terminal closed", "label", opts.label, "reason", reason)
		})
	}
	defer shutdown("connection closed")

	terminalID := ""
	unregisterTerminal := func() {}
	if opts.registerFn != nil {
		terminalID, unregisterTerminal = opts.registerFn(func(reason string) {
			shutdown(reason)
		})
		opts.statusMsg["terminalId"] = terminalID
	}
	defer unregisterTerminal()

	statusPayload, err := json.Marshal(opts.statusMsg)
	if err != nil {
		slog.Error("marshal status failed", "label", opts.label, "err", err)
		_ = wsConn.WriteClose(ws.CloseInternal, "internal error")
		return
	}
	_ = wsConn.WriteText(statusPayload)

	errCh := make(chan error, 1)
	sendErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}

	// PTY → WebSocket
	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := pty.Read(buf)
			if n > 0 {
				if werr := wsConn.WriteBinary(buf[:n]); werr != nil {
					sendErr(werr)
					return
				}
			}
			if readErr != nil {
				if !errors.Is(readErr, io.EOF) {
					sendErr(readErr)
				}
				return
			}
		}
	}()

	// WebSocket → PTY
	go func() {
		for {
			opcode, payload, readErr := wsConn.ReadMessage()
			if readErr != nil {
				sendErr(readErr)
				return
			}
			switch opcode {
			case ws.OpBinary:
				if _, writeErr := pty.Write(payload); writeErr != nil {
					sendErr(writeErr)
					return
				}
			case ws.OpText:
				cols, rows, ctrlErr := handleControlMessage(payload, pty)
				if ctrlErr != nil {
					sendErr(ctrlErr)
					return
				}
				if terminalID != "" && cols > 0 && rows > 0 && h.terminals != nil {
					h.terminals.UpdateSize(terminalID, cols, rows)
				}
			}
		}
	}()

	// Wait for PTY exit
	go func() {
		waitErr := pty.Wait()
		if waitErr != nil {
			sendErr(waitErr)
			return
		}
		sendErr(io.EOF)
	}()

	// Keepalive pings
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()
	go runPingLoop(attachCtx, wsConn, pingTicker.C, sendErr)

	err = <-errCh
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, ws.ErrClosed) {
		slog.Warn("ws error", "label", opts.label, "err", err)
		_ = wsConn.WriteClose(ws.CloseInternal, "connection error")
		return
	}
	_ = wsConn.WriteClose(ws.CloseNormal, "done")
}

func runPingLoop(ctx context.Context, conn pingWriter, ticks <-chan time.Time, sendErr func(error)) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			if pingErr := conn.WritePing([]byte("k")); pingErr != nil {
				sendErr(pingErr)
				return
			}
		}
	}
}

func handleControlMessage(payload []byte, pty *term.PTY) (int, int, error) {
	if len(payload) > 8*1024 {
		return 0, 0, errors.New("control payload too large")
	}

	var msg struct {
		Type string `json:"type"`
		Cols int    `json:"cols"`
		Rows int    `json:"rows"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil {
		return 0, 0, nil
	}
	if msg.Type != "resize" {
		return 0, 0, nil
	}
	if msg.Cols <= 0 || msg.Rows <= 0 {
		return 0, 0, nil
	}
	if err := pty.Resize(msg.Cols, msg.Rows); err != nil {
		return 0, 0, err
	}
	return msg.Cols, msg.Rows, nil
}

func tmuxHTTPError(err error) (int, string) {
	switch {
	case tmux.IsKind(err, tmux.ErrKindNotFound):
		return http.StatusServiceUnavailable, "tmux binary not found"
	case tmux.IsKind(err, tmux.ErrKindSessionNotFound):
		return http.StatusNotFound, "session not found"
	case tmux.IsKind(err, tmux.ErrKindServerNotRunning):
		return http.StatusServiceUnavailable, "tmux server not running"
	default:
		return http.StatusInternalServerError, "tmux error"
	}
}
