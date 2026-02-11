package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"sentinel/internal/security"
	"sentinel/internal/store"
	"sentinel/internal/terminals"
	"sentinel/internal/tmux"
	"sentinel/internal/validate"
)

type tmuxService interface {
	ListSessions(ctx context.Context) ([]tmux.Session, error)
	ListActivePaneCommands(ctx context.Context) (map[string]tmux.PaneSnapshot, error)
	CapturePane(ctx context.Context, session string) (string, error)
	CreateSession(ctx context.Context, name, cwd string) error
	RenameSession(ctx context.Context, session, newName string) error
	RenameWindow(ctx context.Context, session string, index int, name string) error
	RenamePane(ctx context.Context, paneID, title string) error
	KillSession(ctx context.Context, session string) error
	ListWindows(ctx context.Context, session string) ([]tmux.Window, error)
	ListPanes(ctx context.Context, session string) ([]tmux.Pane, error)
	SelectWindow(ctx context.Context, session string, index int) error
	SelectPane(ctx context.Context, paneID string) error
	NewWindow(ctx context.Context, session string) error
	KillWindow(ctx context.Context, session string, index int) error
	KillPane(ctx context.Context, paneID string) error
	SplitPane(ctx context.Context, paneID, direction string) error
}

type systemTerminals interface {
	ListSystem(ctx context.Context) ([]terminals.SystemTerminal, error)
	ListProcesses(ctx context.Context, tty string) ([]terminals.TerminalProcess, error)
}

type Handler struct {
	guard     *security.Guard
	tmux      tmuxService
	sysTerms  systemTerminals
	terminals *terminals.Registry
	store     *store.Store
}

func Register(mux *http.ServeMux, guard *security.Guard, terminalRegistry *terminals.Registry, st *store.Store) {
	h := &Handler{guard: guard, tmux: tmux.Service{}, sysTerms: terminals.SystemService{}, terminals: terminalRegistry, store: st}
	mux.HandleFunc("GET /api/meta", h.wrap(h.meta))
	mux.HandleFunc("GET /api/tmux/sessions", h.wrap(h.listSessions))
	mux.HandleFunc("POST /api/tmux/sessions", h.wrap(h.createSession))
	mux.HandleFunc("PATCH /api/tmux/sessions/{session}", h.wrap(h.renameSession))
	mux.HandleFunc("POST /api/tmux/sessions/{session}/rename-window", h.wrap(h.renameWindow))
	mux.HandleFunc("POST /api/tmux/sessions/{session}/rename-pane", h.wrap(h.renamePane))
	mux.HandleFunc("DELETE /api/tmux/sessions/{session}", h.wrap(h.deleteSession))
	mux.HandleFunc("POST /api/tmux/sessions/{session}/select-window", h.wrap(h.selectWindow))
	mux.HandleFunc("POST /api/tmux/sessions/{session}/select-pane", h.wrap(h.selectPane))
	mux.HandleFunc("POST /api/tmux/sessions/{session}/new-window", h.wrap(h.newWindow))
	mux.HandleFunc("POST /api/tmux/sessions/{session}/kill-window", h.wrap(h.killWindow))
	mux.HandleFunc("POST /api/tmux/sessions/{session}/kill-pane", h.wrap(h.killPane))
	mux.HandleFunc("POST /api/tmux/sessions/{session}/split-pane", h.wrap(h.splitPane))
	mux.HandleFunc("PATCH /api/tmux/sessions/{session}/icon", h.wrap(h.setSessionIcon))
	mux.HandleFunc("GET /api/tmux/sessions/{session}/windows", h.wrap(h.listWindows))
	mux.HandleFunc("GET /api/tmux/sessions/{session}/panes", h.wrap(h.listPanes))
	mux.HandleFunc("GET /api/terminals", h.wrap(h.listTerminals))
	mux.HandleFunc("GET /api/terminals/system/{tty...}", h.wrap(h.getSystemTerminal))
	mux.HandleFunc("DELETE /api/terminals/{terminal}", h.wrap(h.closeTerminal))
}

func (h *Handler) meta(w http.ResponseWriter, _ *http.Request) {
	writeData(w, http.StatusOK, map[string]any{
		"tokenRequired": h.guard.TokenRequired(),
	})
}

func (h *Handler) wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h.guard.CheckOrigin(r); err != nil {
			writeError(w, http.StatusForbidden, "ORIGIN_DENIED", "request origin is not allowed", nil)
			return
		}
		if err := h.guard.RequireBearer(r); err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid token", nil)
			return
		}
		next(w, r)
	}
}

func (h *Handler) listSessions(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	sessions, err := h.tmux.ListSessions(ctx)
	if err != nil {
		writeTmuxError(w, err)
		return
	}

	snapshots, err := h.tmux.ListActivePaneCommands(ctx)
	if err != nil {
		slog.Warn("list-pane-commands failed", "err", err)
		snapshots = map[string]tmux.PaneSnapshot{}
	}

	stored, err := h.store.GetAll(ctx)
	if err != nil {
		slog.Warn("store.GetAll failed", "err", err)
		stored = map[string]store.SessionMeta{}
	}

	type enrichedSession struct {
		Name        string `json:"name"`
		Windows     int    `json:"windows"`
		Panes       int    `json:"panes"`
		Attached    int    `json:"attached"`
		CreatedAt   string `json:"createdAt"`
		ActivityAt  string `json:"activityAt"`
		Command     string `json:"command"`
		Hash        string `json:"hash"`
		LastContent string `json:"lastContent"`
		Icon        string `json:"icon"`
	}

	activeNames := make([]string, 0, len(sessions))
	result := make([]enrichedSession, 0, len(sessions))

	for _, s := range sessions {
		activeNames = append(activeNames, s.Name)

		snap := snapshots[s.Name]
		meta := stored[s.Name]

		hash := meta.Hash
		if hash == "" {
			hash = tmux.SessionHash(s.Name, s.CreatedAt.Unix())
		}

		content := meta.LastContent
		if captured, capErr := h.tmux.CapturePane(ctx, s.Name); capErr == nil && captured != "" {
			content = captured
		}

		if err := h.store.UpsertSession(ctx, s.Name, hash, content); err != nil {
			slog.Warn("store.UpsertSession failed", "session", s.Name, "err", err)
		}

		panes := snap.Panes
		if panes == 0 {
			panes = s.Windows
		}

		result = append(result, enrichedSession{
			Name:        s.Name,
			Windows:     s.Windows,
			Panes:       panes,
			Attached:    s.Attached,
			CreatedAt:   s.CreatedAt.Format(time.RFC3339),
			ActivityAt:  s.ActivityAt.Format(time.RFC3339),
			Command:     snap.Command,
			Hash:        hash,
			LastContent: content,
			Icon:        meta.Icon,
		})
	}

	if err := h.store.Purge(ctx, activeNames); err != nil {
		slog.Warn("store.Purge failed", "err", err)
	}

	writeData(w, http.StatusOK, map[string]any{"sessions": result})
}

func (h *Handler) createSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Cwd  string `json:"cwd"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Cwd = strings.TrimSpace(req.Cwd)
	if !validate.SessionName(req.Name) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "name must match ^[A-Za-z0-9._-]{1,64}$", nil)
		return
	}
	if req.Cwd != "" && !filepath.IsAbs(req.Cwd) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "cwd must be an absolute path", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.tmux.CreateSession(ctx, req.Name, req.Cwd); err != nil {
		writeTmuxError(w, err)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"name": req.Name})
}

func (h *Handler) renameSession(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid source session name", nil)
		return
	}

	var req struct {
		NewName string `json:"newName"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.NewName = strings.TrimSpace(req.NewName)
	if !validate.SessionName(req.NewName) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "newName must match ^[A-Za-z0-9._-]{1,64}$", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.tmux.RenameSession(ctx, session, req.NewName); err != nil {
		writeTmuxError(w, err)
		return
	}
	if err := h.store.Rename(ctx, session, req.NewName); err != nil {
		slog.Warn("store.Rename failed", "from", session, "to", req.NewName, "err", err)
	}
	writeData(w, http.StatusOK, map[string]any{"name": req.NewName})
}

func (h *Handler) setSessionIcon(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	var req struct {
		Icon string `json:"icon"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.Icon = strings.TrimSpace(req.Icon)
	if !validate.IconKey(req.Icon) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "icon must match ^[a-z0-9-]{1,32}$", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.store.SetIcon(ctx, session, req.Icon); err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to set icon", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) deleteSession(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.tmux.KillSession(ctx, session); err != nil {
		writeTmuxError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listWindows(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	windows, err := h.tmux.ListWindows(ctx, session)
	if err != nil {
		writeTmuxError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"windows": windows})
}

func (h *Handler) listPanes(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	panes, err := h.tmux.ListPanes(ctx, session)
	if err != nil {
		writeTmuxError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"panes": panes})
}

func (h *Handler) selectWindow(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	var req struct {
		Index int `json:"index"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if req.Index < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "index must be >= 0", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.tmux.SelectWindow(ctx, session, req.Index); err != nil {
		writeTmuxError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) selectPane(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	var req struct {
		PaneID string `json:"paneId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.PaneID = strings.TrimSpace(req.PaneID)
	if !strings.HasPrefix(req.PaneID, "%") {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "paneId must start with %", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.tmux.SelectPane(ctx, req.PaneID); err != nil {
		writeTmuxError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) renameWindow(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	var req struct {
		Index int    `json:"index"`
		Name  string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Index < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "index must be >= 0", nil)
		return
	}
	if !validate.SessionName(req.Name) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "name must match ^[A-Za-z0-9._-]{1,64}$", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.tmux.RenameWindow(ctx, session, req.Index, req.Name); err != nil {
		writeTmuxError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) renamePane(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	var req struct {
		PaneID string `json:"paneId"`
		Title  string `json:"title"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.PaneID = strings.TrimSpace(req.PaneID)
	req.Title = strings.TrimSpace(req.Title)
	if !strings.HasPrefix(req.PaneID, "%") {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "paneId must start with %", nil)
		return
	}
	if !validate.SessionName(req.Title) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "title must match ^[A-Za-z0-9._-]{1,64}$", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.tmux.RenamePane(ctx, req.PaneID, req.Title); err != nil {
		writeTmuxError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) newWindow(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.tmux.NewWindow(ctx, session); err != nil {
		writeTmuxError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) killWindow(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	var req struct {
		Index int `json:"index"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if req.Index < 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "index must be >= 0", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.tmux.KillWindow(ctx, session, req.Index); err != nil {
		writeTmuxError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) killPane(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	var req struct {
		PaneID string `json:"paneId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.PaneID = strings.TrimSpace(req.PaneID)
	if !strings.HasPrefix(req.PaneID, "%") {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "paneId must start with %", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.tmux.KillPane(ctx, req.PaneID); err != nil {
		writeTmuxError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) splitPane(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	var req struct {
		PaneID    string `json:"paneId"`
		Direction string `json:"direction"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.PaneID = strings.TrimSpace(req.PaneID)
	req.Direction = strings.TrimSpace(strings.ToLower(req.Direction))
	if !strings.HasPrefix(req.PaneID, "%") {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "paneId must start with %", nil)
		return
	}
	if req.Direction != "vertical" && req.Direction != "horizontal" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "direction must be vertical or horizontal", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.tmux.SplitPane(ctx, req.PaneID, req.Direction); err != nil {
		writeTmuxError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listTerminals(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	systemTerminals, err := h.sysTerms.ListSystem(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "TERMINALS_UNAVAILABLE", "failed to list system terminals", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"terminals": systemTerminals})
}

func (h *Handler) getSystemTerminal(w http.ResponseWriter, r *http.Request) {
	tty := strings.TrimSpace(r.PathValue("tty"))
	if tty == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "tty is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	processes, err := h.sysTerms.ListProcesses(ctx, tty)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"tty": tty, "processes": processes})
}

func (h *Handler) closeTerminal(w http.ResponseWriter, r *http.Request) {
	terminalID := strings.TrimSpace(r.PathValue("terminal"))
	if terminalID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid terminal id", nil)
		return
	}
	if h.terminals == nil || !h.terminals.Close(terminalID, "closed by terminal manager") {
		writeError(w, http.StatusNotFound, "TERMINAL_NOT_FOUND", "terminal not found", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeJSON(r *http.Request, dst any) error {
	defer func() { _ = r.Body.Close() }()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid json body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("invalid json body: multiple json values")
	}
	return nil
}

func writeTmuxError(w http.ResponseWriter, err error) {
	switch {
	case tmux.IsKind(err, tmux.ErrKindNotFound):
		writeError(w, http.StatusServiceUnavailable, string(tmux.ErrKindNotFound), "tmux binary not found", nil)
	case tmux.IsKind(err, tmux.ErrKindSessionNotFound):
		writeError(w, http.StatusNotFound, string(tmux.ErrKindSessionNotFound), "tmux session not found", nil)
	case tmux.IsKind(err, tmux.ErrKindSessionExists):
		writeError(w, http.StatusConflict, string(tmux.ErrKindSessionExists), "tmux session already exists", nil)
	case tmux.IsKind(err, tmux.ErrKindServerNotRunning):
		writeError(w, http.StatusServiceUnavailable, string(tmux.ErrKindServerNotRunning), "tmux server not running", nil)
	default:
		writeError(w, http.StatusInternalServerError, string(tmux.ErrKindCommandFailed), "tmux command failed", nil)
	}
}

func writeData(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, map[string]any{"data": data})
}

func writeError(w http.ResponseWriter, status int, code, message string, details any) {
	errObj := map[string]any{
		"code":    code,
		"message": message,
	}
	if details != nil {
		errObj["details"] = details
	}
	writeJSON(w, status, map[string]any{"error": errObj})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	if err := enc.Encode(payload); err != nil {
		slog.Error("json encode error", "err", err)
	}
}
