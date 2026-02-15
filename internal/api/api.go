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
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/guardrails"
	opsplane "github.com/opus-domini/sentinel/internal/ops"
	"github.com/opus-domini/sentinel/internal/recovery"
	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/store"
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

type recoveryService interface {
	Overview(ctx context.Context) (recovery.Overview, error)
	ListKilledSessions(ctx context.Context) ([]store.RecoverySession, error)
	ListSnapshots(ctx context.Context, sessionName string, limit int) ([]store.RecoverySnapshot, error)
	GetSnapshot(ctx context.Context, id int64) (recovery.SnapshotView, error)
	RestoreSnapshotAsync(ctx context.Context, snapshotID int64, options recovery.RestoreOptions) (store.RecoveryJob, error)
	GetJob(ctx context.Context, id string) (store.RecoveryJob, error)
	ArchiveSession(ctx context.Context, name string) error
}

type opsControlPlane interface {
	Overview(ctx context.Context) (opsplane.Overview, error)
	ListServices(ctx context.Context) ([]opsplane.ServiceStatus, error)
	Act(ctx context.Context, name, action string) (opsplane.ServiceStatus, error)
	Inspect(ctx context.Context, name string) (opsplane.ServiceInspect, error)
}

type Handler struct {
	guard      *security.Guard
	tmux       tmuxService
	recovery   recoveryService
	ops        opsControlPlane
	events     *events.Hub
	store      *store.Store
	guardrails *guardrails.Service
	version    string
}

const (
	defaultDirectorySuggestLimit = 12
	maxDirectorySuggestLimit     = 64
	defaultMetaVersion           = "dev"
)

func Register(
	mux *http.ServeMux,
	guard *security.Guard,
	st *store.Store,
	recoverySvc recoveryService,
	eventsHub *events.Hub,
	version string,
) {
	h := &Handler{
		guard:      guard,
		tmux:       tmux.Service{},
		recovery:   recoverySvc,
		ops:        opsplane.NewManager(time.Now()),
		events:     eventsHub,
		store:      st,
		guardrails: guardrails.New(st),
		version:    strings.TrimSpace(version),
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
	mux.HandleFunc("GET /api/tmux/timeline", h.wrap(h.timelineSearch))
	mux.HandleFunc("GET /api/ops/overview", h.wrap(h.opsOverview))
	mux.HandleFunc("GET /api/ops/services", h.wrap(h.opsServices))
	mux.HandleFunc("GET /api/ops/services/{service}/status", h.wrap(h.opsServiceStatus))
	mux.HandleFunc("POST /api/ops/services/{service}/action", h.wrap(h.opsServiceAction))
	mux.HandleFunc("GET /api/ops/alerts", h.wrap(h.opsAlerts))
	mux.HandleFunc("POST /api/ops/alerts/{alert}/ack", h.wrap(h.ackOpsAlert))
	mux.HandleFunc("GET /api/ops/timeline", h.wrap(h.opsTimeline))
	mux.HandleFunc("GET /api/ops/runbooks", h.wrap(h.opsRunbooks))
	mux.HandleFunc("POST /api/ops/runbooks/{runbook}/run", h.wrap(h.runOpsRunbook))
	mux.HandleFunc("GET /api/ops/jobs/{job}", h.wrap(h.opsJob))
	mux.HandleFunc("GET /api/ops/storage/stats", h.wrap(h.storageStats))
	mux.HandleFunc("POST /api/ops/storage/flush", h.wrap(h.flushStorage))
	mux.HandleFunc("GET /api/ops/guardrails/rules", h.wrap(h.listGuardrailRules))
	mux.HandleFunc("PATCH /api/ops/guardrails/rules/{rule}", h.wrap(h.updateGuardrailRule))
	mux.HandleFunc("GET /api/ops/guardrails/audit", h.wrap(h.listGuardrailAudit))
	mux.HandleFunc("POST /api/ops/guardrails/evaluate", h.wrap(h.evaluateGuardrail))
	mux.HandleFunc("POST /api/tmux/sessions/{session}/seen", h.wrap(h.markSessionSeen))
	mux.HandleFunc("PUT /api/tmux/presence", h.wrap(h.setTmuxPresence))
	mux.HandleFunc("GET /api/recovery/overview", h.wrap(h.recoveryOverview))
	mux.HandleFunc("GET /api/recovery/sessions", h.wrap(h.listRecoverySessions))
	mux.HandleFunc("POST /api/recovery/sessions/{session}/archive", h.wrap(h.archiveRecoverySession))
	mux.HandleFunc("GET /api/recovery/sessions/{session}/snapshots", h.wrap(h.listRecoverySnapshots))
	mux.HandleFunc("GET /api/recovery/snapshots/{snapshot}", h.wrap(h.getRecoverySnapshot))
	mux.HandleFunc("POST /api/recovery/snapshots/{snapshot}/restore", h.wrap(h.restoreRecoverySnapshot))
	mux.HandleFunc("GET /api/recovery/jobs/{job}", h.wrap(h.getRecoveryJob))
}

func (h *Handler) emit(eventType string, payload map[string]any) {
	if h == nil || h.events == nil {
		return
	}
	h.events.Publish(events.NewEvent(eventType, payload))
}

func (h *Handler) meta(w http.ResponseWriter, _ *http.Request) {
	defaultCwd := defaultSessionCWD()
	version := strings.TrimSpace(h.version)
	if version == "" {
		version = defaultMetaVersion
	}
	writeData(w, http.StatusOK, map[string]any{
		"tokenRequired": h.guard.TokenRequired(),
		"defaultCwd":    defaultCwd,
		"version":       version,
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

	stored := h.loadSessionMetaMap(ctx)
	if sessions, ok := h.listSessionsFromProjection(ctx, stored); ok {
		writeData(w, http.StatusOK, map[string]any{"sessions": sessions})
		return
	}

	sessions, err := h.listSessionsFromTmux(ctx, stored)
	if err != nil {
		writeTmuxError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (h *Handler) loadSessionMetaMap(ctx context.Context) map[string]store.SessionMeta {
	if h.store == nil {
		return map[string]store.SessionMeta{}
	}
	meta, err := h.store.GetAll(ctx)
	if err != nil {
		slog.Warn("store.GetAll failed", "err", err)
		return map[string]store.SessionMeta{}
	}
	return meta
}

func (h *Handler) listSessionsFromProjection(ctx context.Context, stored map[string]store.SessionMeta) ([]enrichedSession, bool) {
	if h.store == nil {
		return nil, false
	}
	projected, err := h.store.ListWatchtowerSessions(ctx)
	if err != nil {
		slog.Warn("store.ListWatchtowerSessions failed", "err", err)
		return nil, false
	}
	if len(projected) == 0 {
		return nil, false
	}

	activeNames := make([]string, 0, len(projected))
	result := make([]enrichedSession, 0, len(projected))
	for _, row := range projected {
		activeNames = append(activeNames, row.SessionName)
		result = append(result, h.projectedSessionToEnriched(ctx, row, stored[row.SessionName]))
	}
	h.purgeStoredSessionsBestEffort(ctx, activeNames)
	return result, true
}

func (h *Handler) projectedSessionToEnriched(ctx context.Context, row store.WatchtowerSession, meta store.SessionMeta) enrichedSession {
	hash := strings.TrimSpace(meta.Hash)
	if hash == "" {
		hash = tmux.SessionHash(row.SessionName, row.ActivityAt.Unix())
	}
	lastContent := strings.TrimSpace(row.LastPreview)
	if lastContent == "" {
		lastContent = strings.TrimSpace(meta.LastContent)
	}
	h.upsertSessionMetaBestEffort(ctx, row.SessionName, hash, lastContent)
	return enrichedSession{
		Name:          row.SessionName,
		Windows:       row.Windows,
		Panes:         row.Panes,
		Attached:      row.Attached,
		CreatedAt:     projectedCreatedAt(row).Format(time.RFC3339),
		ActivityAt:    row.ActivityAt.Format(time.RFC3339),
		Command:       "",
		Hash:          hash,
		LastContent:   lastContent,
		Icon:          meta.Icon,
		UnreadWindows: row.UnreadWindows,
		UnreadPanes:   row.UnreadPanes,
		Rev:           row.Rev,
	}
}

func projectedCreatedAt(row store.WatchtowerSession) time.Time {
	createdAt := row.ActivityAt
	if row.LastPreviewAt.Before(createdAt) {
		return row.LastPreviewAt
	}
	return createdAt
}

func (h *Handler) listSessionsFromTmux(ctx context.Context, stored map[string]store.SessionMeta) ([]enrichedSession, error) {
	// Warmup fallback: projections may still be empty right after startup.
	sessions, err := h.tmux.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	snapshots := h.loadActivePaneSnapshots(ctx)

	activeNames := make([]string, 0, len(sessions))
	result := make([]enrichedSession, 0, len(sessions))
	for _, sess := range sessions {
		activeNames = append(activeNames, sess.Name)
		result = append(result, h.tmuxSessionToEnriched(ctx, sess, snapshots[sess.Name], stored[sess.Name]))
	}
	h.purgeStoredSessionsBestEffort(ctx, activeNames)
	return result, nil
}

func (h *Handler) loadActivePaneSnapshots(ctx context.Context) map[string]tmux.PaneSnapshot {
	snapshots, err := h.tmux.ListActivePaneCommands(ctx)
	if err != nil {
		slog.Warn("list-pane-commands failed", "err", err)
		return map[string]tmux.PaneSnapshot{}
	}
	return snapshots
}

func (h *Handler) tmuxSessionToEnriched(ctx context.Context, sess tmux.Session, snap tmux.PaneSnapshot, meta store.SessionMeta) enrichedSession {
	hash := strings.TrimSpace(meta.Hash)
	if hash == "" {
		hash = tmux.SessionHash(sess.Name, sess.CreatedAt.Unix())
	}
	lastContent := h.resolveSessionLastContent(ctx, sess.Name, meta.LastContent)
	h.upsertSessionMetaBestEffort(ctx, sess.Name, hash, lastContent)

	return enrichedSession{
		Name:          sess.Name,
		Windows:       sess.Windows,
		Panes:         h.resolveSessionPaneCount(ctx, sess.Name, snap.Panes, sess.Windows),
		Attached:      sess.Attached,
		CreatedAt:     sess.CreatedAt.Format(time.RFC3339),
		ActivityAt:    sess.ActivityAt.Format(time.RFC3339),
		Command:       snap.Command,
		Hash:          hash,
		LastContent:   lastContent,
		Icon:          meta.Icon,
		UnreadWindows: 0,
		UnreadPanes:   0,
		Rev:           0,
	}
}

func (h *Handler) resolveSessionLastContent(ctx context.Context, sessionName, fallback string) string {
	lastContent := strings.TrimSpace(fallback)
	captured, err := h.tmux.CapturePane(ctx, sessionName)
	if err != nil {
		return lastContent
	}
	trimmed := strings.TrimSpace(captured)
	if trimmed == "" {
		return lastContent
	}
	return trimmed
}

func (h *Handler) resolveSessionPaneCount(ctx context.Context, sessionName string, projectedPanes, windowFallback int) int {
	if projectedPanes > 0 {
		return projectedPanes
	}
	paneList, err := h.tmux.ListPanes(ctx, sessionName)
	if err != nil {
		return windowFallback
	}
	return len(paneList)
}

func (h *Handler) upsertSessionMetaBestEffort(ctx context.Context, sessionName, hash, lastContent string) {
	if h.store == nil {
		return
	}
	if err := h.store.UpsertSession(ctx, sessionName, hash, lastContent); err != nil {
		slog.Warn("store.UpsertSession failed", "session", sessionName, "err", err)
	}
}

func (h *Handler) purgeStoredSessionsBestEffort(ctx context.Context, activeNames []string) {
	if h.store == nil {
		return
	}
	if err := h.store.Purge(ctx, activeNames); err != nil {
		slog.Warn("store.Purge failed", "err", err)
	}
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

	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "session.create",
		SessionName: req.Name,
		WindowIndex: -1,
	}); !ok {
		return
	}

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

	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "session.kill",
		SessionName: session,
		WindowIndex: -1,
	}); !ok {
		return
	}

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

	req, err := decodeMarkSeenRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if err := validateMarkSeenRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	acked, err := h.applyMarkSeen(ctx, session, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to mark seen", nil)
		return
	}

	globalRev := readWatchtowerGlobalRev(ctx, h.store)
	sessionPatches, inspectorPatches := h.collectSeenPatches(ctx, session)
	if acked {
		h.emitMarkSeenEvents(session, req.Scope, globalRev, sessionPatches, inspectorPatches)
	}
	writeData(w, http.StatusOK, buildSeenResponsePayload(session, req.Scope, acked, globalRev, sessionPatches, inspectorPatches))
}

type markSeenRequest struct {
	Scope       string `json:"scope"`
	WindowIndex int    `json:"windowIndex"`
	PaneID      string `json:"paneId"`
}

func decodeMarkSeenRequest(r *http.Request) (markSeenRequest, error) {
	var req markSeenRequest
	if err := decodeJSON(r, &req); err != nil {
		return markSeenRequest{}, err
	}
	req.Scope = strings.TrimSpace(strings.ToLower(req.Scope))
	req.PaneID = strings.TrimSpace(req.PaneID)
	return req, nil
}

func validateMarkSeenRequest(req markSeenRequest) error {
	if req.Scope == "" {
		return errors.New("scope is required")
	}
	switch req.Scope {
	case "pane":
		if !strings.HasPrefix(req.PaneID, "%") {
			return errors.New("paneId must start with %")
		}
	case "window":
		if req.WindowIndex < 0 {
			return errors.New("windowIndex must be >= 0")
		}
	case "session":
	default:
		return errors.New("scope must be pane, window, or session")
	}
	return nil
}

func (h *Handler) applyMarkSeen(ctx context.Context, session string, req markSeenRequest) (bool, error) {
	switch req.Scope {
	case "pane":
		return h.store.MarkWatchtowerPaneSeen(ctx, session, req.PaneID)
	case "window":
		return h.store.MarkWatchtowerWindowSeen(ctx, session, req.WindowIndex)
	default:
		return h.store.MarkWatchtowerSessionSeen(ctx, session)
	}
}

func readWatchtowerGlobalRev(ctx context.Context, st *store.Store) int64 {
	if st == nil {
		return 0
	}
	raw, err := st.GetWatchtowerRuntimeValue(ctx, "global_rev")
	if err != nil {
		return 0
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func (h *Handler) collectSeenPatches(ctx context.Context, session string) ([]map[string]any, []map[string]any) {
	sessionPatches := make([]map[string]any, 0, 1)
	inspectorPatches := make([]map[string]any, 0, 1)
	if patch, err := h.store.GetWatchtowerSessionActivityPatch(ctx, session); err == nil {
		sessionPatches = append(sessionPatches, patch)
	}
	if patch, err := h.store.GetWatchtowerInspectorPatch(ctx, session); err == nil {
		inspectorPatches = append(inspectorPatches, patch)
	}
	return sessionPatches, inspectorPatches
}

func (h *Handler) emitMarkSeenEvents(session, scope string, globalRev int64, sessionPatches, inspectorPatches []map[string]any) {
	h.emit(events.TypeTmuxInspector, map[string]any{
		"session": session,
		"action":  "seen",
		"scope":   scope,
	})
	payload := map[string]any{
		"session":   session,
		"action":    "seen",
		"scope":     scope,
		"globalRev": globalRev,
	}
	if len(sessionPatches) > 0 {
		payload["sessionPatches"] = sessionPatches
	}
	if len(inspectorPatches) > 0 {
		payload["inspectorPatches"] = inspectorPatches
	}
	h.emit(events.TypeTmuxSessions, payload)
}

func buildSeenResponsePayload(session, scope string, acked bool, globalRev int64, sessionPatches, inspectorPatches []map[string]any) map[string]any {
	response := map[string]any{
		"session":   session,
		"scope":     scope,
		"acked":     acked,
		"globalRev": globalRev,
	}
	if len(sessionPatches) > 0 {
		response["sessionPatches"] = sessionPatches
	}
	if len(inspectorPatches) > 0 {
		response["inspectorPatches"] = inspectorPatches
	}
	return response
}

func (h *Handler) activityDelta(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}

	since, limit, err := parseActivityDeltaParams(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
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

	globalRev := readWatchtowerGlobalRev(ctx, h.store)
	sessionNames := extractChangedSessionNames(changes)
	sessionPatches, inspectorPatches := h.collectSessionsPatches(ctx, sessionNames)
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

func parseActivityDeltaParams(r *http.Request) (int64, int, error) {
	since := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			return 0, 0, errors.New("since must be >= 0")
		}
		since = parsed
	}

	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return 0, 0, errors.New("limit must be > 0")
		}
		if parsed > 1000 {
			parsed = 1000
		}
		limit = parsed
	}
	return since, limit, nil
}

func extractChangedSessionNames(changes []store.WatchtowerJournal) []string {
	sessionSet := make(map[string]struct{}, len(changes))
	for _, change := range changes {
		session := strings.TrimSpace(change.Session)
		if session == "" {
			continue
		}
		sessionSet[session] = struct{}{}
	}
	names := make([]string, 0, len(sessionSet))
	for session := range sessionSet {
		names = append(names, session)
	}
	return names
}

func (h *Handler) collectSessionsPatches(ctx context.Context, sessions []string) ([]map[string]any, []map[string]any) {
	sessionPatches := make([]map[string]any, 0, len(sessions))
	inspectorPatches := make([]map[string]any, 0, len(sessions))
	for _, session := range sessions {
		if patch, err := h.store.GetWatchtowerSessionActivityPatch(ctx, session); err == nil {
			sessionPatches = append(sessionPatches, patch)
		}
		if patch, err := h.store.GetWatchtowerInspectorPatch(ctx, session); err == nil {
			inspectorPatches = append(inspectorPatches, patch)
		}
	}
	return sessionPatches, inspectorPatches
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

func (h *Handler) timelineSearch(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}

	query, err := parseTimelineSearchQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	result, err := h.store.SearchWatchtowerTimelineEvents(ctx, query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to query timeline", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"events":  result.Events,
		"hasMore": result.HasMore,
	})
}

func (h *Handler) opsOverview(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	overview, err := h.ops.Overview(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "OPS_UNAVAILABLE", "failed to load ops overview", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"overview": overview,
	})
}

func (h *Handler) opsServices(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	services, err := h.ops.ListServices(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "OPS_UNAVAILABLE", "failed to load ops services", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"services": services,
	})
}

func (h *Handler) opsServiceAction(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}

	serviceName := strings.TrimSpace(r.PathValue("service"))
	if serviceName == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "service is required", nil)
		return
	}

	var req struct {
		Action string `json:"action"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	if !slices.Contains([]string{
		opsplane.ActionStart,
		opsplane.ActionStop,
		opsplane.ActionRestart,
	}, req.Action) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "action must be start, stop, or restart", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	serviceStatus, err := h.ops.Act(ctx, serviceName, req.Action)
	if err != nil {
		switch {
		case errors.Is(err, opsplane.ErrServiceNotFound):
			writeError(w, http.StatusNotFound, "OPS_SERVICE_NOT_FOUND", "service not found", nil)
		case errors.Is(err, opsplane.ErrInvalidAction):
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid action", nil)
		default:
			writeError(w, http.StatusBadRequest, "OPS_ACTION_FAILED", err.Error(), nil)
		}
		return
	}

	services, err := h.ops.ListServices(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "OPS_UNAVAILABLE", "failed to refresh ops services", nil)
		return
	}
	overview, err := h.ops.Overview(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "OPS_UNAVAILABLE", "failed to refresh ops overview", nil)
		return
	}

	now := time.Now().UTC()
	timelineEvent, timelineRecorded, alerts, err := h.recordOpsServiceAction(ctx, serviceStatus, req.Action, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to persist ops action", nil)
		return
	}

	globalRev := now.UnixMilli()
	h.emit(events.TypeOpsServices, map[string]any{
		"globalRev": globalRev,
		"service":   serviceStatus.Name,
		"action":    req.Action,
		"services":  services,
	})
	h.emit(events.TypeOpsOverview, map[string]any{
		"globalRev": globalRev,
		"overview":  overview,
	})
	if timelineRecorded {
		h.emit(events.TypeOpsTimeline, map[string]any{
			"globalRev": globalRev,
			"event":     timelineEvent,
		})
	}
	if len(alerts) > 0 {
		h.emit(events.TypeOpsAlerts, map[string]any{
			"globalRev": globalRev,
			"alerts":    alerts,
		})
	}

	response := map[string]any{
		"service":   serviceStatus,
		"services":  services,
		"overview":  overview,
		"alerts":    alerts,
		"globalRev": globalRev,
	}
	if timelineRecorded {
		response["timelineEvent"] = timelineEvent
	}
	writeData(w, http.StatusOK, response)
}

func (h *Handler) opsServiceStatus(w http.ResponseWriter, r *http.Request) {
	if h.ops == nil {
		writeError(w, http.StatusServiceUnavailable, "OPS_UNAVAILABLE", "ops control plane unavailable", nil)
		return
	}

	serviceName := strings.TrimSpace(r.PathValue("service"))
	if serviceName == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "service is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status, err := h.ops.Inspect(ctx, serviceName)
	if err != nil {
		if errors.Is(err, opsplane.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "OPS_SERVICE_NOT_FOUND", "service not found", nil)
			return
		}
		writeError(w, http.StatusBadRequest, "OPS_ACTION_FAILED", err.Error(), nil)
		return
	}

	writeData(w, http.StatusOK, map[string]any{
		"status": status,
	})
}

func (h *Handler) recordOpsServiceAction(ctx context.Context, serviceStatus opsplane.ServiceStatus, action string, at time.Time) (store.OpsTimelineEvent, bool, []store.OpsAlert, error) {
	if h.store == nil {
		return store.OpsTimelineEvent{}, false, nil, nil
	}
	normalizedAction := strings.ToLower(strings.TrimSpace(action))
	state := strings.ToLower(strings.TrimSpace(serviceStatus.ActiveState))
	severity := "info"
	switch {
	case state == "failed":
		severity = "error"
	case normalizedAction == opsplane.ActionStop:
		severity = "warn"
	}

	event, err := h.store.InsertOpsTimelineEvent(ctx, store.OpsTimelineEventWrite{
		Source:    "service",
		EventType: "service.action",
		Severity:  severity,
		Resource:  serviceStatus.Name,
		Message:   fmt.Sprintf("%s %s", serviceStatus.DisplayName, normalizedAction),
		Details:   fmt.Sprintf("unit=%s manager=%s scope=%s state=%s", serviceStatus.Unit, serviceStatus.Manager, serviceStatus.Scope, serviceStatus.ActiveState),
		Metadata:  fmt.Sprintf(`{"action":"%s","service":"%s","manager":"%s","scope":"%s","state":"%s"}`, normalizedAction, serviceStatus.Name, serviceStatus.Manager, serviceStatus.Scope, serviceStatus.ActiveState),
		CreatedAt: at,
	})
	if err != nil {
		return store.OpsTimelineEvent{}, false, nil, err
	}

	alerts := make([]store.OpsAlert, 0, 1)
	if state == "failed" {
		alert, err := h.store.UpsertOpsAlert(ctx, store.OpsAlertWrite{
			DedupeKey: fmt.Sprintf("service:%s:failed", serviceStatus.Name),
			Source:    "service",
			Resource:  serviceStatus.Name,
			Title:     fmt.Sprintf("%s entered failed state", serviceStatus.DisplayName),
			Message:   fmt.Sprintf("%s is failed after %s", serviceStatus.DisplayName, normalizedAction),
			Severity:  "error",
			Metadata:  fmt.Sprintf(`{"action":"%s","service":"%s","unit":"%s"}`, normalizedAction, serviceStatus.Name, serviceStatus.Unit),
			CreatedAt: at,
		})
		if err != nil {
			return store.OpsTimelineEvent{}, false, nil, err
		}
		alerts = append(alerts, alert)
	}

	return event, true, alerts, nil
}

func (h *Handler) opsAlerts(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	limit, err := parseTimelineLimitParam(strings.TrimSpace(r.URL.Query().Get("limit")), 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	alerts, err := h.store.ListOpsAlerts(ctx, limit, status)
	if err != nil {
		if errors.Is(err, store.ErrInvalidOpsFilter) {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load alerts", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"alerts": alerts,
	})
}

func (h *Handler) ackOpsAlert(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	alertRaw := strings.TrimSpace(r.PathValue("alert"))
	alertID, err := strconv.ParseInt(alertRaw, 10, 64)
	if err != nil || alertID <= 0 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "alert must be a positive integer", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()

	now := time.Now().UTC()
	alert, err := h.store.AckOpsAlert(ctx, alertID, now)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "OPS_ALERT_NOT_FOUND", "alert not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to ack alert", nil)
		return
	}

	timelineEvent, timelineRecorded, timelineErr := h.recordOpsAlertAck(ctx, alert, now)
	if timelineErr != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to write alert timeline event", nil)
		return
	}

	globalRev := now.UnixMilli()
	h.emit(events.TypeOpsAlerts, map[string]any{
		"globalRev": globalRev,
		"alert":     alert,
		"action":    "ack",
	})
	if timelineRecorded {
		h.emit(events.TypeOpsTimeline, map[string]any{
			"globalRev": globalRev,
			"event":     timelineEvent,
		})
	}

	writeData(w, http.StatusOK, map[string]any{
		"alert":         alert,
		"timelineEvent": timelineEvent,
		"globalRev":     globalRev,
	})
}

func (h *Handler) recordOpsAlertAck(ctx context.Context, alert store.OpsAlert, at time.Time) (store.OpsTimelineEvent, bool, error) {
	if h.store == nil {
		return store.OpsTimelineEvent{}, false, nil
	}
	event, err := h.store.InsertOpsTimelineEvent(ctx, store.OpsTimelineEventWrite{
		Source:    "alert",
		EventType: "alert.acked",
		Severity:  "info",
		Resource:  alert.Resource,
		Message:   fmt.Sprintf("Alert acknowledged: %s", alert.Title),
		Details:   alert.Message,
		Metadata:  fmt.Sprintf(`{"alertId":%d,"dedupeKey":"%s"}`, alert.ID, alert.DedupeKey),
		CreatedAt: at,
	})
	if err != nil {
		return store.OpsTimelineEvent{}, false, err
	}
	return event, true, nil
}

func (h *Handler) opsTimeline(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	limit, err := parseTimelineLimitParam(strings.TrimSpace(r.URL.Query().Get("limit")), 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	query := store.OpsTimelineQuery{
		Query:    strings.TrimSpace(r.URL.Query().Get("q")),
		Severity: strings.TrimSpace(r.URL.Query().Get("severity")),
		Source:   strings.TrimSpace(r.URL.Query().Get("source")),
		Limit:    limit,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	result, err := h.store.SearchOpsTimelineEvents(ctx, query)
	if err != nil {
		if errors.Is(err, store.ErrInvalidOpsFilter) {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to query ops timeline", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"events":  result.Events,
		"hasMore": result.HasMore,
	})
}

func (h *Handler) opsRunbooks(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	runbooks, err := h.store.ListOpsRunbooks(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load runbooks", nil)
		return
	}
	jobs, err := h.store.ListOpsRunbookRuns(ctx, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load runbook jobs", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"runbooks": runbooks,
		"jobs":     jobs,
	})
}

func (h *Handler) runOpsRunbook(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	runbookID := strings.TrimSpace(r.PathValue("runbook"))
	if runbookID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "runbook is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

	now := time.Now().UTC()
	job, err := h.store.StartOpsRunbook(ctx, runbookID, now)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "OPS_RUNBOOK_NOT_FOUND", "runbook not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to run runbook", nil)
		return
	}
	timelineEvent, timelineErr := h.store.InsertOpsTimelineEvent(ctx, store.OpsTimelineEventWrite{
		Source:    "runbook",
		EventType: "runbook.executed",
		Severity:  "info",
		Resource:  job.RunbookID,
		Message:   fmt.Sprintf("Runbook executed: %s", job.RunbookName),
		Details:   fmt.Sprintf("job=%s status=%s steps=%d/%d", job.ID, job.Status, job.CompletedSteps, job.TotalSteps),
		Metadata:  fmt.Sprintf(`{"jobId":"%s","runbookId":"%s","status":"%s"}`, job.ID, job.RunbookID, job.Status),
		CreatedAt: now,
	})
	if timelineErr != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to persist runbook timeline", nil)
		return
	}

	globalRev := now.UnixMilli()
	h.emit(events.TypeOpsJob, map[string]any{
		"globalRev": globalRev,
		"job":       job,
	})
	h.emit(events.TypeOpsTimeline, map[string]any{
		"globalRev": globalRev,
		"event":     timelineEvent,
	})

	writeData(w, http.StatusAccepted, map[string]any{
		"job":           job,
		"timelineEvent": timelineEvent,
		"globalRev":     globalRev,
	})
}

func (h *Handler) opsJob(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	jobID := strings.TrimSpace(r.PathValue("job"))
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "job id is required", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	job, err := h.store.GetOpsRunbookRun(ctx, jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "OPS_JOB_NOT_FOUND", "job not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load job", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"job": job,
	})
}

func parseTimelineSearchQuery(r *http.Request) (store.WatchtowerTimelineQuery, error) {
	query := store.WatchtowerTimelineQuery{
		Session:   strings.TrimSpace(r.URL.Query().Get("session")),
		PaneID:    strings.TrimSpace(r.URL.Query().Get("paneId")),
		Query:     strings.TrimSpace(r.URL.Query().Get("q")),
		Severity:  strings.TrimSpace(r.URL.Query().Get("severity")),
		EventType: strings.TrimSpace(r.URL.Query().Get("eventType")),
		Limit:     100,
	}
	if err := validateTimelineScope(query.Session, query.PaneID); err != nil {
		return store.WatchtowerTimelineQuery{}, err
	}
	windowIdx, err := parseTimelineWindowIndexParam(strings.TrimSpace(r.URL.Query().Get("windowIndex")))
	if err != nil {
		return store.WatchtowerTimelineQuery{}, err
	}
	since, err := parseTimelineRFC3339Param(strings.TrimSpace(r.URL.Query().Get("since")), "since")
	if err != nil {
		return store.WatchtowerTimelineQuery{}, err
	}
	until, err := parseTimelineRFC3339Param(strings.TrimSpace(r.URL.Query().Get("until")), "until")
	if err != nil {
		return store.WatchtowerTimelineQuery{}, err
	}
	limit, err := parseTimelineLimitParam(strings.TrimSpace(r.URL.Query().Get("limit")), query.Limit)
	if err != nil {
		return store.WatchtowerTimelineQuery{}, err
	}
	query.WindowIdx = windowIdx
	query.Since = since
	query.Until = until
	query.Limit = limit
	return query, nil
}

func validateTimelineScope(session, paneID string) error {
	if session != "" && !validate.SessionName(session) {
		return errors.New("invalid session name")
	}
	if paneID != "" && !strings.HasPrefix(paneID, "%") {
		return errors.New("paneId must start with %")
	}
	return nil
}

func parseTimelineWindowIndexParam(raw string) (*int, error) {
	if raw == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 0 {
		return nil, errors.New("windowIndex must be >= 0")
	}
	return &parsed, nil
}

func parseTimelineRFC3339Param(raw, field string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be RFC3339", field)
	}
	return parsed.UTC(), nil
}

func parseTimelineLimitParam(raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return 0, errors.New("limit must be > 0")
	}
	if parsed > 500 {
		parsed = 500
	}
	return parsed, nil
}

func (h *Handler) storageStats(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stats, err := h.store.GetStorageStats(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to load storage stats", nil)
		return
	}
	writeData(w, http.StatusOK, stats)
}

func (h *Handler) flushStorage(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "store is unavailable", nil)
		return
	}
	var req struct {
		Resource string `json:"resource"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}

	resource := store.NormalizeStorageResource(req.Resource)
	if resource == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "resource is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	results, err := h.store.FlushStorageResource(ctx, resource)
	if err != nil {
		if errors.Is(err, store.ErrInvalidStorageResource) {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid resource", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to flush storage resource", nil)
		return
	}

	writeData(w, http.StatusOK, map[string]any{
		"results":   results,
		"flushedAt": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *Handler) listGuardrailRules(w http.ResponseWriter, r *http.Request) {
	if h.guardrails == nil {
		writeData(w, http.StatusOK, map[string]any{"rules": []store.GuardrailRule{}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	rules, err := h.guardrails.ListRules(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to list guardrail rules", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"rules": rules})
}

func (h *Handler) updateGuardrailRule(w http.ResponseWriter, r *http.Request) {
	if h.guardrails == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "guardrails are unavailable", nil)
		return
	}
	ruleID := strings.TrimSpace(r.PathValue("rule"))
	if ruleID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "rule is required", nil)
		return
	}

	var req struct {
		Name     string `json:"name"`
		Scope    string `json:"scope"`
		Pattern  string `json:"pattern"`
		Mode     string `json:"mode"`
		Severity string `json:"severity"`
		Message  string `json:"message"`
		Enabled  *bool  `json:"enabled"`
		Priority int    `json:"priority"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if strings.TrimSpace(req.Pattern) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "pattern is required", nil)
		return
	}
	if req.Enabled == nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "enabled is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.guardrails.UpsertRule(ctx, store.GuardrailRuleWrite{
		ID:       ruleID,
		Name:     req.Name,
		Scope:    req.Scope,
		Pattern:  req.Pattern,
		Mode:     req.Mode,
		Severity: req.Severity,
		Message:  req.Message,
		Enabled:  *req.Enabled,
		Priority: req.Priority,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to update guardrail rule", nil)
		return
	}
	rules, err := h.guardrails.ListRules(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to list guardrail rules", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"rules": rules})
}

func (h *Handler) listGuardrailAudit(w http.ResponseWriter, r *http.Request) {
	if h.guardrails == nil {
		writeData(w, http.StatusOK, map[string]any{"audit": []store.GuardrailAudit{}})
		return
	}

	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "limit must be > 0", nil)
			return
		}
		if parsed > 500 {
			parsed = 500
		}
		limit = parsed
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	auditRows, err := h.guardrails.ListAudit(ctx, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to list guardrail audit", nil)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"audit": auditRows})
}

func (h *Handler) evaluateGuardrail(w http.ResponseWriter, r *http.Request) {
	if h.guardrails == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "guardrails are unavailable", nil)
		return
	}

	var req struct {
		Action      string `json:"action"`
		Command     string `json:"command"`
		SessionName string `json:"sessionName"`
		WindowIndex int    `json:"windowIndex"`
		PaneID      string `json:"paneId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if req.PaneID != "" && !strings.HasPrefix(strings.TrimSpace(req.PaneID), "%") {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "paneId must start with %", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	decision, err := h.guardrails.Evaluate(ctx, guardrails.Input{
		Action:      req.Action,
		Command:     req.Command,
		SessionName: req.SessionName,
		WindowIndex: req.WindowIndex,
		PaneID:      req.PaneID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORE_ERROR", "failed to evaluate guardrail policy", nil)
		return
	}
	if err := h.guardrails.RecordAudit(ctx, guardrails.Input{
		Action:      req.Action,
		Command:     req.Command,
		SessionName: req.SessionName,
		WindowIndex: req.WindowIndex,
		PaneID:      req.PaneID,
	}, decision, false, "manual evaluate"); err != nil {
		slog.Warn("guardrail evaluate audit write failed", "err", err)
	}
	writeData(w, http.StatusOK, map[string]any{"decision": decision})
}

func (h *Handler) enforceGuardrail(
	w http.ResponseWriter,
	r *http.Request,
	input guardrails.Input,
) bool {
	if h == nil || h.guardrails == nil {
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	decision, err := h.guardrails.Evaluate(ctx, input)
	if err != nil {
		slog.Warn("guardrail evaluate failed, allowing request", "action", input.Action, "err", err)
		return true
	}

	confirmed := hasGuardrailConfirm(r)
	auditOverride := false
	auditReason := ""
	switch decision.Mode {
	case store.GuardrailModeBlock:
		if h.events != nil {
			h.events.Publish(events.NewEvent(events.TypeTmuxGuardrail, map[string]any{
				"action":   strings.TrimSpace(input.Action),
				"session":  strings.TrimSpace(input.SessionName),
				"paneId":   strings.TrimSpace(input.PaneID),
				"decision": decision,
			}))
		}
		if err := h.guardrails.RecordAudit(ctx, input, decision, false, "blocked"); err != nil {
			slog.Warn("guardrail audit write failed", "err", err)
		}
		writeError(w, http.StatusConflict, "GUARDRAIL_BLOCKED", decision.Message, map[string]any{
			"decision": decision,
		})
		return false
	case store.GuardrailModeConfirm:
		if !confirmed {
			if h.events != nil {
				h.events.Publish(events.NewEvent(events.TypeTmuxGuardrail, map[string]any{
					"action":   strings.TrimSpace(input.Action),
					"session":  strings.TrimSpace(input.SessionName),
					"paneId":   strings.TrimSpace(input.PaneID),
					"decision": decision,
				}))
			}
			if err := h.guardrails.RecordAudit(ctx, input, decision, false, "confirm-required"); err != nil {
				slog.Warn("guardrail audit write failed", "err", err)
			}
			writeError(w, http.StatusPreconditionRequired, "GUARDRAIL_CONFIRM_REQUIRED", decision.Message, map[string]any{
				"decision": decision,
			})
			return false
		}
		auditOverride = true
		auditReason = "confirmed"
	default:
		if decision.Mode == store.GuardrailModeWarn {
			auditReason = "warn"
		}
	}
	if len(decision.MatchedRules) > 0 {
		if err := h.guardrails.RecordAudit(ctx, input, decision, auditOverride, auditReason); err != nil {
			slog.Warn("guardrail audit write failed", "err", err)
		}
	}
	return true
}

func hasGuardrailConfirm(r *http.Request) bool {
	if r == nil {
		return false
	}
	candidates := []string{
		r.Header.Get("X-Sentinel-Guardrail-Confirm"),
		r.URL.Query().Get("confirm"),
	}
	for _, raw := range candidates {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "1", "true", "yes", "confirm", "confirmed":
			return true
		}
	}
	return false
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

func defaultWindowName(sequence int) string {
	if sequence < 1 {
		sequence = 1
	}
	return fmt.Sprintf("win-%d", sequence)
}

func parseNamedSequence(name, prefix string) (int, bool) {
	trimmed := strings.TrimSpace(name)
	if !strings.HasPrefix(trimmed, prefix) {
		return 0, false
	}
	raw := strings.TrimPrefix(trimmed, prefix)
	if raw == "" {
		return 0, false
	}
	seq, err := strconv.Atoi(raw)
	if err != nil || seq < 1 {
		return 0, false
	}
	return seq, true
}

func nextWindowNameSequence(windows []tmux.Window) int {
	next := 1
	for _, window := range windows {
		seq, ok := parseNamedSequence(window.Name, "win-")
		if !ok {
			continue
		}
		if candidate := seq + 1; candidate > next {
			next = candidate
		}
	}
	return next
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

	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "window.create",
		SessionName: session,
		WindowIndex: -1,
	}); !ok {
		return
	}

	createdWindow, err := h.tmux.NewWindow(ctx, session)
	if err != nil {
		writeTmuxError(w, err)
		return
	}

	windowNameSequence := createdWindow.Index + 1
	if windowNameSequence < 1 {
		windowNameSequence = 1
	}
	if windows, listErr := h.tmux.ListWindows(ctx, session); listErr != nil {
		slog.Warn("failed to resolve window count for default name", "session", session, "index", createdWindow.Index, "err", listErr)
	} else if next := nextWindowNameSequence(windows); next > windowNameSequence {
		windowNameSequence = next
	}
	if h.store != nil {
		allocatedSequence, allocErr := h.store.AllocateNextWindowSequence(ctx, session, windowNameSequence)
		if allocErr != nil {
			slog.Warn("failed to allocate default window sequence", "session", session, "min", windowNameSequence, "err", allocErr)
		} else {
			windowNameSequence = allocatedSequence
		}
	}
	windowName := defaultWindowName(windowNameSequence)
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

	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "window.kill",
		SessionName: session,
		WindowIndex: req.Index,
	}); !ok {
		return
	}

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

	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "pane.kill",
		SessionName: session,
		WindowIndex: -1,
		PaneID:      req.PaneID,
	}); !ok {
		return
	}

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

	if ok := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "pane.split",
		SessionName: session,
		WindowIndex: -1,
		PaneID:      req.PaneID,
	}); !ok {
		return
	}

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
