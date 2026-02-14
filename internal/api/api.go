package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/recovery"
	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/terminals"
	"github.com/opus-domini/sentinel/internal/tmux"
	"github.com/opus-domini/sentinel/internal/validate"
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
	NewWindow(ctx context.Context, session string) (tmux.NewWindowResult, error)
	KillWindow(ctx context.Context, session string, index int) error
	KillPane(ctx context.Context, paneID string) error
	SplitPane(ctx context.Context, paneID, direction string) (string, error)
}

type systemTerminals interface {
	ListSystem(ctx context.Context) ([]terminals.SystemTerminal, error)
	ListProcesses(ctx context.Context, tty string) ([]terminals.TerminalProcess, error)
}

type recoveryService interface {
	Overview(ctx context.Context) (recovery.Overview, error)
	ListKilledSessions(ctx context.Context) ([]store.RecoverySession, error)
	ListSnapshots(ctx context.Context, sessionName string, limit int) ([]store.RecoverySnapshot, error)
	GetSnapshot(ctx context.Context, id int64) (recovery.SnapshotView, error)
	RestoreSnapshotAsync(ctx context.Context, snapshotID int64, options recovery.RestoreOptions) (store.RecoveryJob, error)
	GetJob(ctx context.Context, id string) (store.RecoveryJob, error)
	ArchiveSession(ctx context.Context, name string) error
}

type Handler struct {
	guard     *security.Guard
	tmux      tmuxService
	sysTerms  systemTerminals
	recovery  recoveryService
	events    *events.Hub
	terminals *terminals.Registry
	store     *store.Store
}

const (
	defaultDirectorySuggestLimit = 12
	maxDirectorySuggestLimit     = 64
)

func Register(mux *http.ServeMux, guard *security.Guard, terminalRegistry *terminals.Registry, st *store.Store, recoverySvc recoveryService, eventsHub *events.Hub) {
	h := &Handler{
		guard:     guard,
		tmux:      tmux.Service{},
		sysTerms:  terminals.SystemService{},
		recovery:  recoverySvc,
		events:    eventsHub,
		terminals: terminalRegistry,
		store:     st,
	}
	mux.HandleFunc("GET /api/meta", h.wrap(h.meta))
	mux.HandleFunc("GET /api/fs/dirs", h.wrap(h.listDirectories))
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
	mux.HandleFunc("GET /api/tmux/activity/delta", h.wrap(h.activityDelta))
	mux.HandleFunc("GET /api/tmux/activity/stats", h.wrap(h.activityStats))
	mux.HandleFunc("POST /api/tmux/sessions/{session}/seen", h.wrap(h.markSessionSeen))
	mux.HandleFunc("PUT /api/tmux/presence", h.wrap(h.setTmuxPresence))
	mux.HandleFunc("GET /api/recovery/overview", h.wrap(h.recoveryOverview))
	mux.HandleFunc("GET /api/recovery/sessions", h.wrap(h.listRecoverySessions))
	mux.HandleFunc("POST /api/recovery/sessions/{session}/archive", h.wrap(h.archiveRecoverySession))
	mux.HandleFunc("GET /api/recovery/sessions/{session}/snapshots", h.wrap(h.listRecoverySnapshots))
	mux.HandleFunc("GET /api/recovery/snapshots/{snapshot}", h.wrap(h.getRecoverySnapshot))
	mux.HandleFunc("POST /api/recovery/snapshots/{snapshot}/restore", h.wrap(h.restoreRecoverySnapshot))
	mux.HandleFunc("GET /api/recovery/jobs/{job}", h.wrap(h.getRecoveryJob))
	mux.HandleFunc("GET /api/terminals", h.wrap(h.listTerminals))
	mux.HandleFunc("GET /api/terminals/system/{tty...}", h.wrap(h.getSystemTerminal))
	mux.HandleFunc("DELETE /api/terminals/{terminal}", h.wrap(h.closeTerminal))
}

func (h *Handler) emit(eventType string, payload map[string]any) {
	if h == nil || h.events == nil {
		return
	}
	h.events.Publish(events.NewEvent(eventType, payload))
}

func (h *Handler) meta(w http.ResponseWriter, _ *http.Request) {
	defaultCwd := defaultSessionCWD()
	writeData(w, http.StatusOK, map[string]any{
		"tokenRequired": h.guard.TokenRequired(),
		"defaultCwd":    defaultCwd,
	})
}

func (h *Handler) listDirectories(w http.ResponseWriter, r *http.Request) {
	limit := defaultDirectorySuggestLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			switch {
			case parsed <= 0:
				limit = defaultDirectorySuggestLimit
			case parsed > maxDirectorySuggestLimit:
				limit = maxDirectorySuggestLimit
			default:
				limit = parsed
			}
		}
	}

	prefix := strings.TrimSpace(r.URL.Query().Get("prefix"))
	dirs := listDirectorySuggestions(prefix, defaultSessionCWD(), limit)
	writeData(w, http.StatusOK, map[string]any{
		"dirs": dirs,
	})
}

func defaultSessionCWD() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	home = strings.TrimSpace(home)
	if home == "" || !filepath.IsAbs(home) {
		return ""
	}
	return home
}

func listDirectorySuggestions(rawPrefix, home string, limit int) []string {
	if limit <= 0 {
		limit = defaultDirectorySuggestLimit
	}
	if limit > maxDirectorySuggestLimit {
		limit = maxDirectorySuggestLimit
	}

	expanded := normalizeDirectoryPrefix(rawPrefix, home)
	if expanded == "" {
		return []string{}
	}

	baseDir, matchPrefix, ok := splitDirectoryLookup(expanded)
	if !ok {
		return []string{}
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		// Non-fatal for autocomplete: path may not exist or be inaccessible.
		return []string{}
	}

	matchPrefix = strings.ToLower(matchPrefix)
	suggestions := make([]string, 0, limit)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if matchPrefix != "" && !strings.HasPrefix(strings.ToLower(name), matchPrefix) {
			continue
		}
		suggestions = append(suggestions, filepath.Join(baseDir, name))
	}

	sort.Slice(suggestions, func(i, j int) bool {
		left := strings.ToLower(filepath.Base(suggestions[i]))
		right := strings.ToLower(filepath.Base(suggestions[j]))
		if left == right {
			return suggestions[i] < suggestions[j]
		}
		return left < right
	})

	if len(suggestions) > limit {
		suggestions = suggestions[:limit]
	}
	return suggestions
}

func normalizeDirectoryPrefix(rawPrefix, home string) string {
	rawPrefix = strings.TrimSpace(rawPrefix)
	home = strings.TrimSpace(home)

	if rawPrefix == "" {
		rawPrefix = home
	}
	if rawPrefix == "" {
		return ""
	}

	switch {
	case rawPrefix == "~":
		rawPrefix = home
	case strings.HasPrefix(rawPrefix, "~/"):
		rawPrefix = filepath.Join(home, strings.TrimPrefix(rawPrefix, "~/"))
	case strings.HasPrefix(rawPrefix, "~"):
		// "~user" expansion is intentionally unsupported.
		return ""
	}

	if strings.TrimSpace(rawPrefix) == "" {
		return ""
	}
	return rawPrefix
}

func splitDirectoryLookup(prefix string) (baseDir string, matchPrefix string, ok bool) {
	hadTrailingSlash := strings.HasSuffix(prefix, string(os.PathSeparator))
	cleaned := filepath.Clean(prefix)
	if !filepath.IsAbs(cleaned) {
		return "", "", false
	}

	if hadTrailingSlash || cleaned == string(os.PathSeparator) {
		return cleaned, "", true
	}
	return filepath.Dir(cleaned), filepath.Base(cleaned), true
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

type enrichedSession struct {
	Name          string `json:"name"`
	Windows       int    `json:"windows"`
	Panes         int    `json:"panes"`
	Attached      int    `json:"attached"`
	CreatedAt     string `json:"createdAt"`
	ActivityAt    string `json:"activityAt"`
	Command       string `json:"command"`
	Hash          string `json:"hash"`
	LastContent   string `json:"lastContent"`
	Icon          string `json:"icon"`
	UnreadWindows int    `json:"unreadWindows"`
	UnreadPanes   int    `json:"unreadPanes"`
	Rev           int64  `json:"rev"`
}

type enrichedWindow struct {
	Session     string `json:"session"`
	Index       int    `json:"index"`
	Name        string `json:"name"`
	Active      bool   `json:"active"`
	Panes       int    `json:"panes"`
	Layout      string `json:"layout,omitempty"`
	UnreadPanes int    `json:"unreadPanes"`
	HasUnread   bool   `json:"hasUnread"`
	Rev         int64  `json:"rev"`
	ActivityAt  string `json:"activityAt,omitempty"`
}

type enrichedPane struct {
	Session        string `json:"session"`
	WindowIndex    int    `json:"windowIndex"`
	PaneIndex      int    `json:"paneIndex"`
	PaneID         string `json:"paneId"`
	Title          string `json:"title"`
	Active         bool   `json:"active"`
	TTY            string `json:"tty"`
	CurrentPath    string `json:"currentPath,omitempty"`
	StartCommand   string `json:"startCommand,omitempty"`
	CurrentCommand string `json:"currentCommand,omitempty"`
	TailPreview    string `json:"tailPreview,omitempty"`
	Revision       int64  `json:"revision"`
	SeenRevision   int64  `json:"seenRevision"`
	HasUnread      bool   `json:"hasUnread"`
	ChangedAt      string `json:"changedAt,omitempty"`
}

func (h *Handler) listSessions(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stored := map[string]store.SessionMeta{}
	if h.store != nil {
		if meta, err := h.store.GetAll(ctx); err != nil {
			slog.Warn("store.GetAll failed", "err", err)
		} else {
			stored = meta
		}
	}

	if h.store != nil {
		projected, err := h.store.ListWatchtowerSessions(ctx)
		if err != nil {
			slog.Warn("store.ListWatchtowerSessions failed", "err", err)
		} else if len(projected) > 0 {
			activeNames := make([]string, 0, len(projected))
			result := make([]enrichedSession, 0, len(projected))

			for _, row := range projected {
				activeNames = append(activeNames, row.SessionName)

				meta := stored[row.SessionName]
				hash := strings.TrimSpace(meta.Hash)
				if hash == "" {
					hash = tmux.SessionHash(row.SessionName, row.ActivityAt.Unix())
				}
				lastContent := strings.TrimSpace(row.LastPreview)
				if lastContent == "" {
					lastContent = strings.TrimSpace(meta.LastContent)
				}
				if err := h.store.UpsertSession(ctx, row.SessionName, hash, lastContent); err != nil {
					slog.Warn("store.UpsertSession failed", "session", row.SessionName, "err", err)
				}

				createdAt := row.ActivityAt
				if row.LastPreviewAt.Before(createdAt) {
					createdAt = row.LastPreviewAt
				}

				result = append(result, enrichedSession{
					Name:          row.SessionName,
					Windows:       row.Windows,
					Panes:         row.Panes,
					Attached:      row.Attached,
					CreatedAt:     createdAt.Format(time.RFC3339),
					ActivityAt:    row.ActivityAt.Format(time.RFC3339),
					Command:       "",
					Hash:          hash,
					LastContent:   lastContent,
					Icon:          meta.Icon,
					UnreadWindows: row.UnreadWindows,
					UnreadPanes:   row.UnreadPanes,
					Rev:           row.Rev,
				})
			}
			if err := h.store.Purge(ctx, activeNames); err != nil {
				slog.Warn("store.Purge failed", "err", err)
			}
			writeData(w, http.StatusOK, map[string]any{"sessions": result})
			return
		}
	}

	// Warmup fallback: projections may still be empty right after startup.
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

	activeNames := make([]string, 0, len(sessions))
	result := make([]enrichedSession, 0, len(sessions))
	for _, s := range sessions {
		activeNames = append(activeNames, s.Name)
		snap := snapshots[s.Name]
		meta := stored[s.Name]

		hash := strings.TrimSpace(meta.Hash)
		if hash == "" {
			hash = tmux.SessionHash(s.Name, s.CreatedAt.Unix())
		}
		lastContent := strings.TrimSpace(meta.LastContent)
		if captured, capErr := h.tmux.CapturePane(ctx, s.Name); capErr == nil {
			if trimmed := strings.TrimSpace(captured); trimmed != "" {
				lastContent = trimmed
			}
		}
		if h.store != nil {
			if err := h.store.UpsertSession(ctx, s.Name, hash, lastContent); err != nil {
				slog.Warn("store.UpsertSession failed", "session", s.Name, "err", err)
			}
		}

		panes := snap.Panes
		if panes == 0 {
			if paneList, paneErr := h.tmux.ListPanes(ctx, s.Name); paneErr == nil {
				panes = len(paneList)
			} else {
				panes = s.Windows
			}
		}

		result = append(result, enrichedSession{
			Name:          s.Name,
			Windows:       s.Windows,
			Panes:         panes,
			Attached:      s.Attached,
			CreatedAt:     s.CreatedAt.Format(time.RFC3339),
			ActivityAt:    s.ActivityAt.Format(time.RFC3339),
			Command:       snap.Command,
			Hash:          hash,
			LastContent:   lastContent,
			Icon:          meta.Icon,
			UnreadWindows: 0,
			UnreadPanes:   0,
			Rev:           0,
		})
	}
	if h.store != nil {
		if err := h.store.Purge(ctx, activeNames); err != nil {
			slog.Warn("store.Purge failed", "err", err)
		}
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
	if req.Cwd == "" {
		req.Cwd = defaultSessionCWD()
	}
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
	h.emit(events.TypeTmuxSessions, map[string]any{"session": req.Name, "action": "create"})
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
	if err := h.store.RenameRecoverySession(ctx, session, req.NewName); err != nil {
		slog.Warn("store.RenameRecoverySession failed", "from", session, "to", req.NewName, "err", err)
	}
	h.emit(events.TypeTmuxSessions, map[string]any{
		"session": session,
		"newName": req.NewName,
		"action":  "rename",
	})
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
	h.emit(events.TypeTmuxSessions, map[string]any{
		"session": session,
		"action":  "icon",
	})
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
	if h.recovery != nil {
		if err := h.recovery.ArchiveSession(ctx, session); err != nil {
			slog.Warn("recovery archive on kill failed", "session", session, "err", err)
		}
	}
	h.emit(events.TypeTmuxSessions, map[string]any{"session": session, "action": "delete"})
	h.emit(events.TypeRecoveryOverview, map[string]any{"session": session, "action": "archive"})
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

	if h.store != nil {
		projected, err := h.store.ListWatchtowerWindows(ctx, session)
		if err != nil {
			slog.Warn("store.ListWatchtowerWindows failed", "session", session, "err", err)
		} else {
			_, sessionErr := h.store.GetWatchtowerSession(ctx, session)
			hasSession := sessionErr == nil
			if len(projected) > 0 || hasSession {
				panes, panesErr := h.store.ListWatchtowerPanes(ctx, session)
				if panesErr != nil {
					writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to list windows", nil)
					return
				}

				paneCounts := make(map[int]int, len(projected))
				for _, pane := range panes {
					paneCounts[pane.WindowIndex]++
				}

				resp := make([]enrichedWindow, 0, len(projected))
				for _, row := range projected {
					resp = append(resp, enrichedWindow{
						Session:     row.SessionName,
						Index:       row.WindowIndex,
						Name:        row.Name,
						Active:      row.Active,
						Panes:       paneCounts[row.WindowIndex],
						Layout:      row.Layout,
						UnreadPanes: row.UnreadPanes,
						HasUnread:   row.HasUnread,
						Rev:         row.Rev,
						ActivityAt:  row.WindowActivityAt.Format(time.RFC3339),
					})
				}
				writeData(w, http.StatusOK, map[string]any{"windows": resp})
				return
			}
		}
	}

	windows, err := h.tmux.ListWindows(ctx, session)
	if err != nil {
		writeTmuxError(w, err)
		return
	}
	resp := make([]enrichedWindow, 0, len(windows))
	for _, row := range windows {
		resp = append(resp, enrichedWindow{
			Session:     row.Session,
			Index:       row.Index,
			Name:        row.Name,
			Active:      row.Active,
			Panes:       row.Panes,
			Layout:      row.Layout,
			UnreadPanes: 0,
			HasUnread:   false,
			Rev:         0,
		})
	}
	writeData(w, http.StatusOK, map[string]any{"windows": resp})
}

func (h *Handler) listPanes(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if h.store != nil {
		projected, err := h.store.ListWatchtowerPanes(ctx, session)
		if err != nil {
			slog.Warn("store.ListWatchtowerPanes failed", "session", session, "err", err)
		} else {
			_, sessionErr := h.store.GetWatchtowerSession(ctx, session)
			hasSession := sessionErr == nil
			if len(projected) > 0 || hasSession {
				resp := make([]enrichedPane, 0, len(projected))
				for _, row := range projected {
					resp = append(resp, enrichedPane{
						Session:        row.SessionName,
						WindowIndex:    row.WindowIndex,
						PaneIndex:      row.PaneIndex,
						PaneID:         row.PaneID,
						Title:          row.Title,
						Active:         row.Active,
						TTY:            row.TTY,
						CurrentPath:    row.CurrentPath,
						StartCommand:   row.StartCommand,
						CurrentCommand: row.CurrentCommand,
						TailPreview:    row.TailPreview,
						Revision:       row.Revision,
						SeenRevision:   row.SeenRevision,
						HasUnread:      row.Revision > row.SeenRevision,
						ChangedAt:      row.ChangedAt.Format(time.RFC3339),
					})
				}
				writeData(w, http.StatusOK, map[string]any{"panes": resp})
				return
			}
		}
	}

	panes, err := h.tmux.ListPanes(ctx, session)
	if err != nil {
		writeTmuxError(w, err)
		return
	}
	resp := make([]enrichedPane, 0, len(panes))
	for _, row := range panes {
		resp = append(resp, enrichedPane{
			Session:        row.Session,
			WindowIndex:    row.WindowIndex,
			PaneIndex:      row.PaneIndex,
			PaneID:         row.PaneID,
			Title:          row.Title,
			Active:         row.Active,
			TTY:            row.TTY,
			CurrentPath:    row.CurrentPath,
			StartCommand:   row.StartCommand,
			CurrentCommand: row.CurrentCommand,
			Revision:       0,
			SeenRevision:   0,
			HasUnread:      false,
		})
	}
	writeData(w, http.StatusOK, map[string]any{"panes": resp})
}

func (h *Handler) markSessionSeen(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}

	var req struct {
		Scope       string `json:"scope"`
		WindowIndex int    `json:"windowIndex"`
		PaneID      string `json:"paneId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.Scope = strings.TrimSpace(strings.ToLower(req.Scope))
	req.PaneID = strings.TrimSpace(req.PaneID)

	if req.Scope == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "scope is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	acked := false
	var err error
	switch req.Scope {
	case "pane":
		if !strings.HasPrefix(req.PaneID, "%") {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "paneId must start with %", nil)
			return
		}
		acked, err = h.store.MarkWatchtowerPaneSeen(ctx, session, req.PaneID)
	case "window":
		if req.WindowIndex < 0 {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "windowIndex must be >= 0", nil)
			return
		}
		acked, err = h.store.MarkWatchtowerWindowSeen(ctx, session, req.WindowIndex)
	case "session":
		acked, err = h.store.MarkWatchtowerSessionSeen(ctx, session)
	default:
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "scope must be pane, window, or session", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to mark seen", nil)
		return
	}

	globalRev := int64(0)
	if raw, getErr := h.store.GetWatchtowerRuntimeValue(ctx, "global_rev"); getErr == nil {
		if parsed, parseErr := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); parseErr == nil {
			globalRev = parsed
		}
	}

	var sessionPatches []map[string]any
	if patch, patchErr := h.store.GetWatchtowerSessionActivityPatch(ctx, session); patchErr == nil {
		sessionPatches = append(sessionPatches, patch)
	}
	var inspectorPatches []map[string]any
	if patch, patchErr := h.store.GetWatchtowerInspectorPatch(ctx, session); patchErr == nil {
		inspectorPatches = append(inspectorPatches, patch)
	}

	if acked {
		h.emit(events.TypeTmuxInspector, map[string]any{
			"session": session,
			"action":  "seen",
			"scope":   req.Scope,
		})
		sessionsPayload := map[string]any{
			"session":   session,
			"action":    "seen",
			"scope":     req.Scope,
			"globalRev": globalRev,
		}
		if len(sessionPatches) > 0 {
			sessionsPayload["sessionPatches"] = sessionPatches
		}
		if len(inspectorPatches) > 0 {
			sessionsPayload["inspectorPatches"] = inspectorPatches
		}
		h.emit(events.TypeTmuxSessions, sessionsPayload)
	}

	response := map[string]any{
		"session":   session,
		"scope":     req.Scope,
		"acked":     acked,
		"globalRev": globalRev,
	}
	if len(sessionPatches) > 0 {
		response["sessionPatches"] = sessionPatches
	}
	if len(inspectorPatches) > 0 {
		response["inspectorPatches"] = inspectorPatches
	}
	writeData(w, http.StatusOK, response)
}

func (h *Handler) activityDelta(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}

	since := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "since must be >= 0", nil)
			return
		}
		since = parsed
	}

	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "limit must be > 0", nil)
			return
		}
		if parsed > 1000 {
			parsed = 1000
		}
		limit = parsed
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	changes, err := h.store.ListWatchtowerJournalSince(ctx, since, limit+1)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to read activity delta", nil)
		return
	}
	overflow := false
	if len(changes) > limit {
		overflow = true
		changes = changes[:limit]
	}

	globalRev := int64(0)
	if raw, getErr := h.store.GetWatchtowerRuntimeValue(ctx, "global_rev"); getErr == nil {
		if parsed, parseErr := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); parseErr == nil {
			globalRev = parsed
		}
	}

	sessionNames := make(map[string]struct{}, len(changes))
	for _, change := range changes {
		session := strings.TrimSpace(change.Session)
		if session == "" {
			continue
		}
		sessionNames[session] = struct{}{}
	}

	sessionPatches := make([]map[string]any, 0, len(sessionNames))
	inspectorPatches := make([]map[string]any, 0, len(sessionNames))
	for sessionName := range sessionNames {
		if patch, patchErr := h.store.GetWatchtowerSessionActivityPatch(ctx, sessionName); patchErr == nil {
			sessionPatches = append(sessionPatches, patch)
		}
		if patch, patchErr := h.store.GetWatchtowerInspectorPatch(ctx, sessionName); patchErr == nil {
			inspectorPatches = append(inspectorPatches, patch)
		}
	}

	response := map[string]any{
		"since":     since,
		"limit":     limit,
		"globalRev": globalRev,
		"overflow":  overflow,
		"changes":   changes,
	}
	if len(sessionPatches) > 0 {
		response["sessionPatches"] = sessionPatches
	}
	if len(inspectorPatches) > 0 {
		response["inspectorPatches"] = inspectorPatches
	}
	writeData(w, http.StatusOK, response)
}

func (h *Handler) activityStats(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	keys := []string{
		"global_rev",
		"collect_total",
		"collect_errors_total",
		"last_collect_at",
		"last_collect_duration_ms",
		"last_collect_sessions",
		"last_collect_changed_sessions",
		"last_collect_error",
	}

	runtime := make(map[string]string, len(keys))
	for _, key := range keys {
		value, err := h.store.GetWatchtowerRuntimeValue(ctx, key)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to read activity stats", nil)
			return
		}
		runtime[key] = strings.TrimSpace(value)
	}

	parseInt := func(key string) int64 {
		raw := strings.TrimSpace(runtime[key])
		if raw == "" {
			return 0
		}
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0
		}
		return parsed
	}

	writeData(w, http.StatusOK, map[string]any{
		"globalRev":             parseInt("global_rev"),
		"collectTotal":          parseInt("collect_total"),
		"collectErrorsTotal":    parseInt("collect_errors_total"),
		"lastCollectAt":         runtime["last_collect_at"],
		"lastCollectDurationMs": parseInt("last_collect_duration_ms"),
		"lastCollectSessions":   parseInt("last_collect_sessions"),
		"lastCollectChanged":    parseInt("last_collect_changed_sessions"),
		"lastCollectError":      runtime["last_collect_error"],
		"runtime":               runtime,
	})
}

func (h *Handler) setTmuxPresence(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}

	var req struct {
		TerminalID  string `json:"terminalId"`
		SessionName string `json:"session"`
		WindowIndex int    `json:"windowIndex"`
		PaneID      string `json:"paneId"`
		Visible     bool   `json:"visible"`
		Focused     bool   `json:"focused"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	req.TerminalID = strings.TrimSpace(req.TerminalID)
	req.SessionName = strings.TrimSpace(req.SessionName)
	req.PaneID = strings.TrimSpace(req.PaneID)

	if req.TerminalID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "terminalId is required", nil)
		return
	}
	if req.SessionName != "" && !validate.SessionName(req.SessionName) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}
	if req.WindowIndex < -1 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "windowIndex must be >= -1", nil)
		return
	}
	if req.PaneID != "" && !strings.HasPrefix(req.PaneID, "%") {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "paneId must start with %", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	now := time.Now().UTC()
	expiresAt := now.Add(30 * time.Second)
	if err := h.store.UpsertWatchtowerPresence(ctx, store.WatchtowerPresenceWrite{
		TerminalID:  req.TerminalID,
		SessionName: req.SessionName,
		WindowIndex: req.WindowIndex,
		PaneID:      req.PaneID,
		Visible:     req.Visible,
		Focused:     req.Focused,
		UpdatedAt:   now,
		ExpiresAt:   expiresAt,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to set presence", nil)
		return
	}

	writeData(w, http.StatusOK, map[string]any{
		"accepted":  true,
		"expiresAt": expiresAt.Format(time.RFC3339),
	})
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
	h.emit(events.TypeTmuxInspector, map[string]any{
		"session": session,
		"action":  "select-window",
		"index":   req.Index,
	})
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
	h.emit(events.TypeTmuxInspector, map[string]any{
		"session": session,
		"action":  "select-pane",
		"paneId":  req.PaneID,
	})
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
	h.emit(events.TypeTmuxInspector, map[string]any{
		"session": session,
		"action":  "rename-window",
		"index":   req.Index,
	})
	h.emit(events.TypeTmuxSessions, map[string]any{"session": session, "action": "window-meta"})
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
	h.emit(events.TypeTmuxInspector, map[string]any{
		"session": session,
		"action":  "rename-pane",
		"paneId":  req.PaneID,
	})
	w.WriteHeader(http.StatusNoContent)
}

func defaultWindowName(index int) string {
	return fmt.Sprintf("win-%d", index)
}

func defaultPaneTitle(paneID string) string {
	suffix := strings.TrimPrefix(strings.TrimSpace(paneID), "%")
	if suffix == "" {
		return "pan-0"
	}
	return "pan-" + suffix
}

func (h *Handler) newWindow(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	createdWindow, err := h.tmux.NewWindow(ctx, session)
	if err != nil {
		writeTmuxError(w, err)
		return
	}

	windowName := defaultWindowName(createdWindow.Index)
	if err := h.tmux.RenameWindow(ctx, session, createdWindow.Index, windowName); err != nil {
		slog.Warn("failed to apply default window name", "session", session, "index", createdWindow.Index, "name", windowName, "err", err)
	}
	if createdWindow.PaneID != "" {
		paneTitle := defaultPaneTitle(createdWindow.PaneID)
		if err := h.tmux.RenamePane(ctx, createdWindow.PaneID, paneTitle); err != nil {
			slog.Warn("failed to apply default pane title", "session", session, "paneId", createdWindow.PaneID, "title", paneTitle, "err", err)
		}
	}
	h.emit(events.TypeTmuxInspector, map[string]any{
		"session": session,
		"action":  "new-window",
		"index":   createdWindow.Index,
		"paneId":  createdWindow.PaneID,
	})
	h.emit(events.TypeTmuxSessions, map[string]any{"session": session, "action": "window-count"})
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
	h.emit(events.TypeTmuxInspector, map[string]any{
		"session": session,
		"action":  "kill-window",
		"index":   req.Index,
	})
	h.emit(events.TypeTmuxSessions, map[string]any{"session": session, "action": "window-count"})
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
	h.emit(events.TypeTmuxInspector, map[string]any{
		"session": session,
		"action":  "kill-pane",
		"paneId":  req.PaneID,
	})
	h.emit(events.TypeTmuxSessions, map[string]any{"session": session, "action": "pane-count"})
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

	createdPaneID, err := h.tmux.SplitPane(ctx, req.PaneID, req.Direction)
	if err != nil {
		writeTmuxError(w, err)
		return
	}
	if createdPaneID != "" {
		paneTitle := defaultPaneTitle(createdPaneID)
		if err := h.tmux.RenamePane(ctx, createdPaneID, paneTitle); err != nil {
			slog.Warn("failed to apply default pane title", "session", session, "paneId", createdPaneID, "title", paneTitle, "err", err)
		}
	}
	h.emit(events.TypeTmuxInspector, map[string]any{
		"session":   session,
		"action":    "split-pane",
		"paneId":    req.PaneID,
		"createdId": createdPaneID,
		"direction": req.Direction,
	})
	h.emit(events.TypeTmuxSessions, map[string]any{"session": session, "action": "pane-count"})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) requireRecovery(w http.ResponseWriter) bool {
	if h.recovery != nil {
		return true
	}
	writeError(w, http.StatusServiceUnavailable, "RECOVERY_DISABLED", "recovery subsystem is disabled", nil)
	return false
}

func (h *Handler) recoveryOverview(w http.ResponseWriter, r *http.Request) {
	if !h.requireRecovery(w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	overview, err := h.recovery.Overview(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RECOVERY_ERROR", "failed to load recovery overview", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"overview": overview,
	})
}

func (h *Handler) listRecoverySessions(w http.ResponseWriter, r *http.Request) {
	if !h.requireRecovery(w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	sessions, err := h.recovery.ListKilledSessions(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RECOVERY_ERROR", "failed to list recovery sessions", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"sessions": sessions,
	})
}

func (h *Handler) archiveRecoverySession(w http.ResponseWriter, r *http.Request) {
	if !h.requireRecovery(w) {
		return
	}
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.recovery.ArchiveSession(ctx, session); err != nil {
		writeError(w, http.StatusInternalServerError, "RECOVERY_ERROR", "failed to archive recovery session", nil)
		return
	}
	h.emit(events.TypeRecoveryOverview, map[string]any{
		"session": session,
		"action":  "archive",
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listRecoverySnapshots(w http.ResponseWriter, r *http.Request) {
	if !h.requireRecovery(w) {
		return
	}
	session := strings.TrimSpace(r.PathValue("session"))
	if !validate.SessionName(session) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid session name", nil)
		return
	}
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	snapshots, err := h.recovery.ListSnapshots(ctx, session, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RECOVERY_ERROR", "failed to list snapshots", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"snapshots": snapshots,
	})
}

func (h *Handler) getRecoverySnapshot(w http.ResponseWriter, r *http.Request) {
	if !h.requireRecovery(w) {
		return
	}
	rawID := strings.TrimSpace(r.PathValue("snapshot"))
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "snapshot must be a positive integer", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	view, err := h.recovery.GetSnapshot(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "SNAPSHOT_NOT_FOUND", "snapshot not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "RECOVERY_ERROR", "failed to load snapshot", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"snapshot": view,
	})
}

func (h *Handler) restoreRecoverySnapshot(w http.ResponseWriter, r *http.Request) {
	if !h.requireRecovery(w) {
		return
	}
	rawID := strings.TrimSpace(r.PathValue("snapshot"))
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "snapshot must be a positive integer", nil)
		return
	}

	var req struct {
		Mode           recovery.ReplayMode     `json:"mode"`
		ConflictPolicy recovery.ConflictPolicy `json:"conflictPolicy"`
		TargetSession  string                  `json:"targetSession"`
	}
	if err := decodeOptionalJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	req.TargetSession = strings.TrimSpace(req.TargetSession)
	if req.TargetSession != "" && !validate.SessionName(req.TargetSession) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "targetSession must match ^[A-Za-z0-9._-]{1,64}$", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	job, err := h.recovery.RestoreSnapshotAsync(ctx, id, recovery.RestoreOptions{
		Mode:           req.Mode,
		ConflictPolicy: req.ConflictPolicy,
		TargetSession:  req.TargetSession,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "SNAPSHOT_NOT_FOUND", "snapshot not found", nil)
			return
		}
		writeError(w, http.StatusBadRequest, "RECOVERY_RESTORE_FAILED", err.Error(), nil)
		return
	}
	h.emit(events.TypeRecoveryJob, map[string]any{
		"jobId":  job.ID,
		"status": string(job.Status),
	})
	h.emit(events.TypeRecoveryOverview, map[string]any{
		"session": job.SessionName,
		"action":  "restore-started",
	})
	writeData(w, http.StatusAccepted, map[string]any{
		"job": job,
	})
}

func (h *Handler) getRecoveryJob(w http.ResponseWriter, r *http.Request) {
	if !h.requireRecovery(w) {
		return
	}
	jobID := strings.TrimSpace(r.PathValue("job"))
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "job id is required", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	job, err := h.recovery.GetJob(ctx, jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "recovery job not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "RECOVERY_ERROR", "failed to load recovery job", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"job": job,
	})
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

func decodeOptionalJSON(r *http.Request, dst any) error {
	defer func() { _ = r.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("invalid json body: %w", err)
	}
	if strings.TrimSpace(string(body)) == "" {
		return nil
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid json body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("multiple json values are not allowed")
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
