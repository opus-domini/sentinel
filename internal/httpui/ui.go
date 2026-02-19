package httpui

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/term"
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

// OpsLogStreamer provides streaming log access for managed services.
type OpsLogStreamer interface {
	StreamLogs(ctx context.Context, name string) (io.ReadCloser, error)
	StreamLogsByUnit(ctx context.Context, unit, scope, manager string) (io.ReadCloser, error)
}

type Handler struct {
	guard  *security.Guard
	events *events.Hub
	store  *store.Store
	ops    OpsLogStreamer
}

func Register(mux *http.ServeMux, guard *security.Guard, st *store.Store, eventsHub *events.Hub, ops OpsLogStreamer) error {
	h := &Handler{guard: guard, events: eventsHub, store: st, ops: ops}
	if err := registerAssetRoutes(mux); err != nil {
		return err
	}
	mux.HandleFunc("GET /ws/tmux", h.attachWS)
	mux.HandleFunc("GET /ws/events", h.attachEventsWS)
	mux.HandleFunc("GET /ws/logs", h.attachLogsWS)
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
	if err := h.guard.RequireAuth(r); err != nil {
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

func (h *Handler) attachEventsWS(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeEventsWS(w, r) {
		return
	}

	wsConn, _, err := ws.UpgradeWithSubprotocols(w, r, nil, []string{"sentinel.v1"})
	if err != nil {
		return
	}
	defer func() { _ = wsConn.Close() }()

	eventsCh, unsubscribe := h.events.Subscribe(64)
	defer unsubscribe()

	writeEventsReadyPayload(wsConn)
	readErrCh := startEventsWSReader(wsConn, h.handleEventsClientMessage)
	runEventsWSLoop(wsConn, eventsCh, readErrCh)
}

func (h *Handler) attachLogsWS(w http.ResponseWriter, r *http.Request) {
	if err := h.guard.CheckOrigin(r); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := h.guard.RequireAuth(r); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	service := strings.TrimSpace(r.URL.Query().Get("service"))
	unit := strings.TrimSpace(r.URL.Query().Get("unit"))
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	manager := strings.TrimSpace(r.URL.Query().Get("manager"))

	if service == "" && unit == "" {
		http.Error(w, "service or unit required", http.StatusBadRequest)
		return
	}

	wsConn, _, err := ws.UpgradeWithSubprotocols(w, r, nil, []string{"sentinel.v1"})
	if err != nil {
		return
	}
	defer func() { _ = wsConn.Close() }()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var stream io.ReadCloser
	if service != "" {
		stream, err = h.ops.StreamLogs(ctx, service)
	} else {
		stream, err = h.ops.StreamLogsByUnit(ctx, unit, scope, manager)
	}
	if err != nil {
		errMsg, _ := json.Marshal(map[string]string{"type": "error", "message": err.Error()})
		_ = wsConn.WriteText(errMsg)
		_ = wsConn.WriteClose(ws.CloseInternal, "stream start failed")
		return
	}
	defer func() { _ = stream.Close() }()

	statusMsg, _ := json.Marshal(map[string]string{"type": "status", "state": "streaming"})
	_ = wsConn.WriteText(statusMsg)

	errCh, sendErr := newAttachErrChannel()

	go func() {
		scanner := bufio.NewScanner(stream)
		scanner.Buffer(make([]byte, 64*1024), 64*1024)
		for scanner.Scan() {
			line := scanner.Text()
			msg, _ := json.Marshal(map[string]string{"type": "log", "line": line})
			if writeErr := wsConn.WriteText(msg); writeErr != nil {
				sendErr(writeErr)
				return
			}
		}
		if scanErr := scanner.Err(); scanErr != nil {
			sendErr(scanErr)
		} else {
			sendErr(io.EOF)
		}
	}()

	go func() {
		for {
			_, _, readErr := wsConn.ReadMessage()
			if readErr != nil {
				sendErr(readErr)
				return
			}
		}
	}()

	pingTicker := time.NewTicker(20 * time.Second)
	defer pingTicker.Stop()
	go runPingLoop(ctx, wsConn, pingTicker.C, sendErr)

	finalErr := <-errCh
	if finalErr != nil && !errors.Is(finalErr, io.EOF) && !errors.Is(finalErr, ws.ErrClosed) {
		slog.Warn("log stream error", "service", service, "unit", unit, "err", finalErr)
		_ = wsConn.WriteClose(ws.CloseInternal, "stream error")
		return
	}
	_ = wsConn.WriteClose(ws.CloseNormal, "done")
}

func (h *Handler) authorizeEventsWS(w http.ResponseWriter, r *http.Request) bool {
	if err := h.guard.CheckOrigin(r); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	if err := h.guard.RequireAuth(r); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	if h.events == nil {
		http.Error(w, "events unavailable", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func writeEventsReadyPayload(wsConn *ws.Conn) {
	readyPayload, _ := json.Marshal(events.NewEvent(events.TypeReady, map[string]any{
		"message": "subscribed",
	}))
	_ = wsConn.WriteText(readyPayload)
}

func startEventsWSReader(wsConn *ws.Conn, handleMessage func([]byte) []byte) <-chan error {
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
			if responsePayload := handleMessage(payload); len(responsePayload) > 0 {
				if err := wsConn.WriteText(responsePayload); err != nil {
					sendReadErr(err)
					return
				}
			}
		}
	}()
	return readErrCh
}

func runEventsWSLoop(wsConn *ws.Conn, eventsCh <-chan events.Event, readErrCh <-chan error) {
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
		ExpiresAt:   now.Add(events.PresenceExpiry),
	}); err != nil {
		slog.Warn("events ws presence write failed", "terminal", terminalID, "err", err)
	}
}

func (h *Handler) handleEventsSeenClientMessage(payload []byte) []byte {
	if h == nil || h.store == nil || len(payload) == 0 {
		return nil
	}

	msg, err := decodeEventsSeenMessage(payload)
	if err != nil {
		return nil
	}

	ack := newEventsSeenAck(msg)
	if err := validateEventsSeenMessage(msg); err != nil {
		ack["error"] = err.Error()
		return marshalEventsWSMessage(ack)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	acked, err := h.markEventsSeen(ctx, msg)
	if err != nil {
		ack["error"] = "failed to mark seen"
		slog.Warn("events ws seen write failed", "session", msg.Session, "scope", msg.Scope, "err", err)
		return marshalEventsWSMessage(ack)
	}

	globalRev, _ := h.store.WatchtowerGlobalRevision(ctx)
	sessionPatches, inspectorPatches := collectSeenPatches(ctx, h.store, msg.Session)
	ack["acked"] = acked
	ack["globalRev"] = globalRev
	if len(sessionPatches) > 0 {
		ack["sessionPatches"] = sessionPatches
	}
	if len(inspectorPatches) > 0 {
		ack["inspectorPatches"] = inspectorPatches
	}

	if acked && h.events != nil {
		publishEventsSeenAck(h.events, msg.Session, msg.Scope, globalRev, sessionPatches, inspectorPatches)
	}

	return marshalEventsWSMessage(ack)
}

type eventsSeenMessage struct {
	RequestID string `json:"requestId"`
	Session   string `json:"session"`
	Scope     string `json:"scope"`
	WindowIdx int    `json:"windowIndex"`
	PaneID    string `json:"paneId"`
}

func decodeEventsSeenMessage(payload []byte) (eventsSeenMessage, error) {
	var msg eventsSeenMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return eventsSeenMessage{}, err
	}
	msg.RequestID = strings.TrimSpace(msg.RequestID)
	msg.Session = strings.TrimSpace(msg.Session)
	msg.Scope = strings.ToLower(strings.TrimSpace(msg.Scope))
	msg.PaneID = strings.TrimSpace(msg.PaneID)
	return msg, nil
}

func newEventsSeenAck(msg eventsSeenMessage) map[string]any {
	ack := map[string]any{
		"type":      "tmux.seen.ack",
		"requestId": msg.RequestID,
		"session":   msg.Session,
		"scope":     msg.Scope,
		"acked":     false,
		"globalRev": int64(0),
	}
	if msg.PaneID != "" {
		ack["paneId"] = msg.PaneID
	}
	if msg.WindowIdx >= 0 {
		ack["windowIndex"] = msg.WindowIdx
	}
	return ack
}

func validateEventsSeenMessage(msg eventsSeenMessage) error {
	if !validate.SessionName(msg.Session) {
		return errors.New("invalid session name")
	}
	switch msg.Scope {
	case "pane":
		if !strings.HasPrefix(msg.PaneID, "%") {
			return errors.New("paneId must start with %")
		}
	case "window":
		if msg.WindowIdx < 0 {
			return errors.New("windowIndex must be >= 0")
		}
	case "session":
	default:
		return errors.New("scope must be pane, window, or session")
	}
	return nil
}

func (h *Handler) markEventsSeen(ctx context.Context, msg eventsSeenMessage) (bool, error) {
	switch msg.Scope {
	case "pane":
		return h.store.MarkWatchtowerPaneSeen(ctx, msg.Session, msg.PaneID)
	case "window":
		return h.store.MarkWatchtowerWindowSeen(ctx, msg.Session, msg.WindowIdx)
	default:
		return h.store.MarkWatchtowerSessionSeen(ctx, msg.Session)
	}
}

func collectSeenPatches(ctx context.Context, st *store.Store, sessionName string) ([]map[string]any, []map[string]any) {
	sessionPatches := make([]map[string]any, 0, 1)
	inspectorPatches := make([]map[string]any, 0, 1)
	if patch, err := st.GetWatchtowerSessionActivityPatch(ctx, sessionName); err == nil {
		sessionPatches = append(sessionPatches, patch)
	}
	if patch, err := st.GetWatchtowerInspectorPatch(ctx, sessionName); err == nil {
		inspectorPatches = append(inspectorPatches, patch)
	}
	return sessionPatches, inspectorPatches
}

func publishEventsSeenAck(hub *events.Hub, sessionName, scope string, globalRev int64, sessionPatches, inspectorPatches []map[string]any) {
	hub.Publish(events.NewEvent(events.TypeTmuxInspector, map[string]any{
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
	if len(inspectorPatches) > 0 {
		sessionsPayload["inspectorPatches"] = inspectorPatches
	}
	hub.Publish(events.NewEvent(events.TypeTmuxSessions, sessionsPayload))
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
	label     string
	startPTY  func(ctx context.Context) (*term.PTY, error)
	statusMsg map[string]any
}

type pingWriter interface {
	WritePing(payload []byte) error
}

func (h *Handler) attachPTY(wsConn *ws.Conn, opts attachPTYOptions) {
	attachCtx, cancelAttach := context.WithCancel(context.Background())
	defer cancelAttach()

	pty, ok := startAttachPTY(wsConn, opts, attachCtx)
	if !ok {
		return
	}
	defer func() { _ = pty.Close() }()

	shutdown := attachShutdown(cancelAttach, pty, wsConn, opts.label)
	defer shutdown("connection closed")

	if !writeAttachStatus(wsConn, opts.statusMsg, opts.label) {
		return
	}

	errCh, sendErr := newAttachErrChannel()
	startPTYReadLoop(pty, wsConn, sendErr)
	startWSReadLoop(wsConn, pty, sendErr)
	startPTYWaitLoop(pty, sendErr)

	// Keepalive pings
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()
	go runPingLoop(attachCtx, wsConn, pingTicker.C, sendErr)

	err := <-errCh
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, ws.ErrClosed) {
		slog.Warn("ws error", "label", opts.label, "err", err)
		_ = wsConn.WriteClose(ws.CloseInternal, "connection error")
		return
	}
	_ = wsConn.WriteClose(ws.CloseNormal, "done")
}

func startAttachPTY(wsConn *ws.Conn, opts attachPTYOptions, attachCtx context.Context) (*term.PTY, bool) {
	pty, err := opts.startPTY(attachCtx)
	if err != nil {
		slog.Error("pty start failed", "label", opts.label, "err", err)
		_ = wsConn.WriteText([]byte(`{"type":"error","code":"PTY_FAILED","message":"unable to start terminal"}`))
		_ = wsConn.WriteClose(ws.CloseInternal, "pty start failed")
		return nil, false
	}
	return pty, true
}

func attachShutdown(cancelAttach context.CancelFunc, pty *term.PTY, wsConn *ws.Conn, label string) func(string) {
	var shutdownOnce sync.Once
	return func(reason string) {
		shutdownOnce.Do(func() {
			cancelAttach()
			_ = pty.Close()
			_ = wsConn.Close()
			slog.Info("terminal closed", "label", label, "reason", reason)
		})
	}
}

func writeAttachStatus(wsConn *ws.Conn, status map[string]any, label string) bool {
	statusPayload, err := json.Marshal(status)
	if err != nil {
		slog.Error("marshal status failed", "label", label, "err", err)
		_ = wsConn.WriteClose(ws.CloseInternal, "internal error")
		return false
	}
	_ = wsConn.WriteText(statusPayload)
	return true
}

func newAttachErrChannel() (chan error, func(error)) {
	errCh := make(chan error, 1)
	sendErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}
	return errCh, sendErr
}

func startPTYReadLoop(pty *term.PTY, wsConn *ws.Conn, sendErr func(error)) {
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
}

func startWSReadLoop(wsConn *ws.Conn, pty *term.PTY, sendErr func(error)) {
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
				if _, _, ctrlErr := handleControlMessage(payload, pty); ctrlErr != nil {
					sendErr(ctrlErr)
					return
				}
			}
		}
	}()
}

func startPTYWaitLoop(pty *term.PTY, sendErr func(error)) {
	go func() {
		waitErr := pty.Wait()
		if waitErr != nil {
			sendErr(waitErr)
			return
		}
		sendErr(io.EOF)
	}()
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
