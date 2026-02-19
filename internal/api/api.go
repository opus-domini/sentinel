package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/guardrails"
	"github.com/opus-domini/sentinel/internal/recovery"
	"github.com/opus-domini/sentinel/internal/security"
	opsplane "github.com/opus-domini/sentinel/internal/services"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
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
	Logs(ctx context.Context, name string, lines int) (string, error)
	Metrics(ctx context.Context) opsplane.HostMetrics
	DiscoverServices(ctx context.Context) ([]opsplane.AvailableService, error)
	BrowseServices(ctx context.Context) ([]opsplane.BrowsedService, error)
	ActByUnit(ctx context.Context, unit, scope, manager, action string) error
	InspectByUnit(ctx context.Context, unit, scope, manager string) (opsplane.ServiceInspect, error)
	LogsByUnit(ctx context.Context, unit, scope, manager string, lines int) (string, error)
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
	configPath string
}

const (
	defaultDirectorySuggestLimit = 12
	maxDirectorySuggestLimit     = 64
	defaultMetaVersion           = "dev"
	stateFailed                  = "failed"
	scheduleTypeCron             = "cron"
	scheduleTypeOnce             = "once"
)

func marshalMetadata(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		slog.Warn("failed to marshal metadata", "error", err)
		return "{}"
	}
	return string(b)
}

func Register(
	mux *http.ServeMux,
	guard *security.Guard,
	st *store.Store,
	ops opsControlPlane,
	recoverySvc recoveryService,
	eventsHub *events.Hub,
	version string,
	configPath string,
) {
	h := &Handler{
		guard:      guard,
		tmux:       tmux.Service{},
		recovery:   recoverySvc,
		ops:        ops,
		events:     eventsHub,
		store:      st,
		guardrails: guardrails.New(st),
		version:    strings.TrimSpace(version),
		configPath: configPath,
	}
	h.registerMetaRoutes(mux)
	h.registerTmuxRoutes(mux)
	h.registerServicesRoutes(mux)
	h.registerRunbooksRoutes(mux)
	h.registerAlertsRoutes(mux)
	h.registerTimelineRoutes(mux)
	h.registerMetricsRoutes(mux)
	h.registerGuardrailsRoutes(mux)
	h.registerSettingsRoutes(mux)
	h.registerRecoveryRoutes(mux)
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

type setAuthTokenRequest struct {
	Token string `json:"token"`
}

func (h *Handler) setAuthToken(w http.ResponseWriter, r *http.Request) {
	if !h.guard.TokenRequired() {
		writeData(w, http.StatusOK, map[string]any{"authenticated": true})
		return
	}

	var req setAuthTokenRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "token is required", nil)
		return
	}
	if !h.guard.TokenMatches(token) {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid token", nil)
		return
	}

	h.guard.SetAuthCookie(w, r)
	writeData(w, http.StatusOK, map[string]any{"authenticated": true})
}

func (h *Handler) clearAuthToken(w http.ResponseWriter, r *http.Request) {
	h.guard.ClearAuthCookie(w, r)
	writeData(w, http.StatusOK, map[string]any{"authenticated": false})
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

func (h *Handler) wrapOrigin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h.guard.CheckOrigin(r); err != nil {
			writeError(w, http.StatusForbidden, "ORIGIN_DENIED", "request origin is not allowed", nil)
			return
		}
		next(w, r)
	}
}

func (h *Handler) wrap(next http.HandlerFunc) http.HandlerFunc {
	return h.wrapOrigin(func(w http.ResponseWriter, r *http.Request) {
		if err := h.guard.RequireAuth(r); err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid token", nil)
			return
		}
		next(w, r)
	})
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

func decodeJSON(r *http.Request, dst any) error {
	defer func() { _ = r.Body.Close() }()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return errors.New("invalid json body")
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
		return errors.New("invalid json body")
	}
	if strings.TrimSpace(string(body)) == "" {
		return nil
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return errors.New("invalid json body")
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
