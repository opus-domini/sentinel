package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/recovery"
	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/terminals"
	"github.com/opus-domini/sentinel/internal/tmux"
)

// ---------------------------------------------------------------------------
// Mock types
// ---------------------------------------------------------------------------

type mockTmux struct {
	listSessionsFn           func(ctx context.Context) ([]tmux.Session, error)
	listActivePaneCommandsFn func(ctx context.Context) (map[string]tmux.PaneSnapshot, error)
	capturePaneFn            func(ctx context.Context, session string) (string, error)
	createSessionFn          func(ctx context.Context, name, cwd string) error
	renameSessionFn          func(ctx context.Context, session, newName string) error
	renameWindowFn           func(ctx context.Context, session string, index int, name string) error
	renamePaneFn             func(ctx context.Context, paneID, title string) error
	killSessionFn            func(ctx context.Context, session string) error
	listWindowsFn            func(ctx context.Context, session string) ([]tmux.Window, error)
	listPanesFn              func(ctx context.Context, session string) ([]tmux.Pane, error)
	selectWindowFn           func(ctx context.Context, session string, index int) error
	selectPaneFn             func(ctx context.Context, paneID string) error
	newWindowFn              func(ctx context.Context, session string) (tmux.NewWindowResult, error)
	killWindowFn             func(ctx context.Context, session string, index int) error
	killPaneFn               func(ctx context.Context, paneID string) error
	splitPaneFn              func(ctx context.Context, paneID, direction string) (string, error)
}

func (m *mockTmux) ListSessions(ctx context.Context) ([]tmux.Session, error) {
	if m.listSessionsFn != nil {
		return m.listSessionsFn(ctx)
	}
	return nil, nil
}

func (m *mockTmux) ListActivePaneCommands(ctx context.Context) (map[string]tmux.PaneSnapshot, error) {
	if m.listActivePaneCommandsFn != nil {
		return m.listActivePaneCommandsFn(ctx)
	}
	return map[string]tmux.PaneSnapshot{}, nil
}

func (m *mockTmux) CapturePane(ctx context.Context, session string) (string, error) {
	if m.capturePaneFn != nil {
		return m.capturePaneFn(ctx, session)
	}
	return "", nil
}

func (m *mockTmux) CreateSession(ctx context.Context, name, cwd string) error {
	if m.createSessionFn != nil {
		return m.createSessionFn(ctx, name, cwd)
	}
	return nil
}

func (m *mockTmux) RenameSession(ctx context.Context, session, newName string) error {
	if m.renameSessionFn != nil {
		return m.renameSessionFn(ctx, session, newName)
	}
	return nil
}

func (m *mockTmux) RenameWindow(ctx context.Context, session string, index int, name string) error {
	if m.renameWindowFn != nil {
		return m.renameWindowFn(ctx, session, index, name)
	}
	return nil
}

func (m *mockTmux) RenamePane(ctx context.Context, paneID, title string) error {
	if m.renamePaneFn != nil {
		return m.renamePaneFn(ctx, paneID, title)
	}
	return nil
}

func (m *mockTmux) KillSession(ctx context.Context, session string) error {
	if m.killSessionFn != nil {
		return m.killSessionFn(ctx, session)
	}
	return nil
}

func (m *mockTmux) ListWindows(ctx context.Context, session string) ([]tmux.Window, error) {
	if m.listWindowsFn != nil {
		return m.listWindowsFn(ctx, session)
	}
	return nil, nil
}

func (m *mockTmux) ListPanes(ctx context.Context, session string) ([]tmux.Pane, error) {
	if m.listPanesFn != nil {
		return m.listPanesFn(ctx, session)
	}
	return nil, nil
}

func (m *mockTmux) SelectWindow(ctx context.Context, session string, index int) error {
	if m.selectWindowFn != nil {
		return m.selectWindowFn(ctx, session, index)
	}
	return nil
}

func (m *mockTmux) SelectPane(ctx context.Context, paneID string) error {
	if m.selectPaneFn != nil {
		return m.selectPaneFn(ctx, paneID)
	}
	return nil
}

func (m *mockTmux) NewWindow(ctx context.Context, session string) (tmux.NewWindowResult, error) {
	if m.newWindowFn != nil {
		return m.newWindowFn(ctx, session)
	}
	return tmux.NewWindowResult{Index: 0, PaneID: "%0"}, nil
}

func (m *mockTmux) KillWindow(ctx context.Context, session string, index int) error {
	if m.killWindowFn != nil {
		return m.killWindowFn(ctx, session, index)
	}
	return nil
}

func (m *mockTmux) KillPane(ctx context.Context, paneID string) error {
	if m.killPaneFn != nil {
		return m.killPaneFn(ctx, paneID)
	}
	return nil
}

func (m *mockTmux) SplitPane(ctx context.Context, paneID, direction string) (string, error) {
	if m.splitPaneFn != nil {
		return m.splitPaneFn(ctx, paneID, direction)
	}
	return "%0", nil
}

type mockSysTerms struct {
	listSystemFn    func(ctx context.Context) ([]terminals.SystemTerminal, error)
	listProcessesFn func(ctx context.Context, tty string) ([]terminals.TerminalProcess, error)
}

func (m *mockSysTerms) ListSystem(ctx context.Context) ([]terminals.SystemTerminal, error) {
	if m.listSystemFn != nil {
		return m.listSystemFn(ctx)
	}
	return nil, nil
}

type mockRecovery struct {
	overviewFn             func(ctx context.Context) (recovery.Overview, error)
	listKilledSessionsFn   func(ctx context.Context) ([]store.RecoverySession, error)
	listSnapshotsFn        func(ctx context.Context, sessionName string, limit int) ([]store.RecoverySnapshot, error)
	getSnapshotFn          func(ctx context.Context, id int64) (recovery.SnapshotView, error)
	restoreSnapshotAsyncFn func(ctx context.Context, snapshotID int64, options recovery.RestoreOptions) (store.RecoveryJob, error)
	getJobFn               func(ctx context.Context, id string) (store.RecoveryJob, error)
	archiveSessionFn       func(ctx context.Context, name string) error
}

func (m *mockRecovery) Overview(ctx context.Context) (recovery.Overview, error) {
	if m.overviewFn != nil {
		return m.overviewFn(ctx)
	}
	return recovery.Overview{}, nil
}

func (m *mockRecovery) ListKilledSessions(ctx context.Context) ([]store.RecoverySession, error) {
	if m.listKilledSessionsFn != nil {
		return m.listKilledSessionsFn(ctx)
	}
	return nil, nil
}

func (m *mockRecovery) ListSnapshots(ctx context.Context, sessionName string, limit int) ([]store.RecoverySnapshot, error) {
	if m.listSnapshotsFn != nil {
		return m.listSnapshotsFn(ctx, sessionName, limit)
	}
	return nil, nil
}

func (m *mockRecovery) GetSnapshot(ctx context.Context, id int64) (recovery.SnapshotView, error) {
	if m.getSnapshotFn != nil {
		return m.getSnapshotFn(ctx, id)
	}
	return recovery.SnapshotView{}, nil
}

func (m *mockRecovery) RestoreSnapshotAsync(ctx context.Context, snapshotID int64, options recovery.RestoreOptions) (store.RecoveryJob, error) {
	if m.restoreSnapshotAsyncFn != nil {
		return m.restoreSnapshotAsyncFn(ctx, snapshotID, options)
	}
	return store.RecoveryJob{}, nil
}

func (m *mockRecovery) GetJob(ctx context.Context, id string) (store.RecoveryJob, error) {
	if m.getJobFn != nil {
		return m.getJobFn(ctx, id)
	}
	return store.RecoveryJob{}, nil
}

func (m *mockRecovery) ArchiveSession(ctx context.Context, name string) error {
	if m.archiveSessionFn != nil {
		return m.archiveSessionFn(ctx, name)
	}
	return nil
}

func (m *mockSysTerms) ListProcesses(ctx context.Context, tty string) ([]terminals.TerminalProcess, error) {
	if m.listProcessesFn != nil {
		return m.listProcessesFn(ctx, tty)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "sentinel.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func newTestHandler(t *testing.T, tm *mockTmux, sys *mockSysTerms) *Handler {
	t.Helper()
	guard := security.New("", nil)
	st := newTestStore(t)
	if tm == nil {
		tm = &mockTmux{}
	}
	if sys == nil {
		sys = &mockSysTerms{}
	}
	return &Handler{guard: guard, tmux: tm, sysTerms: sys, terminals: terminals.NewRegistry(), store: st}
}

// jsonBody is a helper to decode a JSON response body.
func jsonBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json decode error: %v\nbody: %s", err, w.Body.String())
	}
	return body
}

func errCode(body map[string]any) string {
	e, ok := body["error"].(map[string]any)
	if !ok {
		return ""
	}
	c, _ := e["code"].(string)
	return c
}

const invalidRequestCode = "INVALID_REQUEST"

// ---------------------------------------------------------------------------
// Existing unit tests (unchanged)
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		payload    any
		wantStatus int
	}{
		{
			name:       "ok with map",
			status:     http.StatusOK,
			payload:    map[string]string{"key": "value"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "created with struct",
			status:     http.StatusCreated,
			payload:    struct{ Name string }{"test"},
			wantStatus: http.StatusCreated,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			writeJSON(w, tt.status, tt.payload)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if ct := w.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			// Body must be valid JSON.
			var parsed any
			if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
				t.Errorf("body is not valid JSON: %v", err)
			}
		})
	}
}

func TestWriteData(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	writeData(w, http.StatusOK, map[string]string{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var body struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json decode error: %v", err)
	}
	if body.Data["key"] != "value" {
		t.Errorf("data.key = %q, want %q", body.Data["key"], "value")
	}
}

func TestWriteError(t *testing.T) {
	t.Parallel()

	t.Run("without details", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		writeError(w, http.StatusBadRequest, "BAD", "something wrong", nil)

		var body struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
				Details any    `json:"details"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("json decode error: %v", err)
		}
		if body.Error.Code != "BAD" {
			t.Errorf("code = %q, want BAD", body.Error.Code)
		}
		if body.Error.Message != "something wrong" {
			t.Errorf("message = %q, want %q", body.Error.Message, "something wrong")
		}
		if body.Error.Details != nil {
			t.Errorf("details = %v, want nil", body.Error.Details)
		}
	})

	t.Run("with details", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		writeError(w, http.StatusBadRequest, "BAD", "wrong", map[string]string{"field": "name"})

		var body struct {
			Error struct {
				Code    string         `json:"code"`
				Message string         `json:"message"`
				Details map[string]any `json:"details"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("json decode error: %v", err)
		}
		if body.Error.Details["field"] != "name" {
			t.Errorf("details.field = %v, want name", body.Error.Details["field"])
		}
	})
}

func TestWriteTmuxError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "not found",
			err:        &tmux.Error{Kind: tmux.ErrKindNotFound},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   string(tmux.ErrKindNotFound),
		},
		{
			name:       "session not found",
			err:        &tmux.Error{Kind: tmux.ErrKindSessionNotFound},
			wantStatus: http.StatusNotFound,
			wantCode:   string(tmux.ErrKindSessionNotFound),
		},
		{
			name:       "session exists",
			err:        &tmux.Error{Kind: tmux.ErrKindSessionExists},
			wantStatus: http.StatusConflict,
			wantCode:   string(tmux.ErrKindSessionExists),
		},
		{
			name:       "server not running",
			err:        &tmux.Error{Kind: tmux.ErrKindServerNotRunning},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   string(tmux.ErrKindServerNotRunning),
		},
		{
			name:       "command failed default",
			err:        &tmux.Error{Kind: tmux.ErrKindCommandFailed},
			wantStatus: http.StatusInternalServerError,
			wantCode:   string(tmux.ErrKindCommandFailed),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			writeTmuxError(w, tt.err)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("json decode error: %v", err)
			}
			if body.Error.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", body.Error.Code, tt.wantCode)
			}
		})
	}
}

func TestDecodeJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "valid JSON",
			body: `{"name": "test"}`,
		},
		{
			name:    "invalid JSON",
			body:    `{not json}`,
			wantErr: "invalid json body",
		},
		{
			name:    "unknown fields",
			body:    `{"name": "test", "extra": true}`,
			wantErr: "invalid json body",
		},
		{
			name:    "multiple JSON values",
			body:    `{"name": "a"}{"name": "b"}`,
			wantErr: "multiple json values",
		},
		{
			name:    "body exceeds 1MB",
			body:    `{"name": "` + strings.Repeat("x", 1<<20) + `"}`,
			wantErr: "invalid json body",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest("POST", "/", strings.NewReader(tt.body))
			var dst struct {
				Name string `json:"name"`
			}
			err := decodeJSON(r, &dst)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if dst.Name != "test" {
					t.Errorf("decoded name = %q, want %q", dst.Name, "test")
				}
			}
		})
	}
}

func TestWrapMiddleware(t *testing.T) {
	t.Parallel()

	okHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	tests := []struct {
		name       string
		token      string
		origins    []string
		origin     string
		auth       string
		host       string
		wantStatus int
	}{
		{
			name:       "no token no origin",
			host:       "localhost:4040",
			wantStatus: http.StatusOK,
		},
		{
			name:       "bad origin",
			origin:     "http://evil.example.com",
			host:       "localhost:4040",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "token required wrong token",
			token:      "secret",
			auth:       "Bearer wrong",
			host:       "localhost:4040",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "token required correct token",
			token:      "secret",
			auth:       "Bearer secret",
			host:       "localhost:4040",
			wantStatus: http.StatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			guard := security.New(tt.token, tt.origins)
			h := &Handler{guard: guard}

			wrapped := h.wrap(okHandler)

			host := tt.host
			if host == "" {
				host = "localhost:4040"
			}
			r := httptest.NewRequest("GET", "http://"+host+"/", nil)
			r.Host = host
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			if tt.auth != "" {
				r.Header.Set("Authorization", tt.auth)
			}

			w := httptest.NewRecorder()
			wrapped(w, r)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestMetaHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		token        string
		wantRequired bool
	}{
		{
			name:         "token configured",
			token:        "secret",
			wantRequired: true,
		},
		{
			name:         "no token",
			token:        "",
			wantRequired: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			guard := security.New(tt.token, nil)
			h := &Handler{guard: guard}

			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/api/meta", nil)
			h.meta(w, r)

			var body struct {
				Data struct {
					TokenRequired bool   `json:"tokenRequired"`
					DefaultCwd    string `json:"defaultCwd"`
				} `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("json decode error: %v", err)
			}
			if body.Data.TokenRequired != tt.wantRequired {
				t.Errorf("tokenRequired = %v, want %v", body.Data.TokenRequired, tt.wantRequired)
			}
			if body.Data.DefaultCwd != defaultSessionCWD() {
				t.Errorf("defaultCwd = %q, want %q", body.Data.DefaultCwd, defaultSessionCWD())
			}
		})
	}
}

func TestNormalizeDirectoryPrefix(t *testing.T) {
	t.Parallel()

	home := "/home/tester"
	tests := []struct {
		name      string
		rawPrefix string
		want      string
	}{
		{name: "empty uses home", rawPrefix: "", want: home},
		{name: "absolute", rawPrefix: "/tmp", want: "/tmp"},
		{name: "tilde root", rawPrefix: "~", want: home},
		{name: "tilde path", rawPrefix: "~/work", want: "/home/tester/work"},
		{name: "tilde user unsupported", rawPrefix: "~root/work", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeDirectoryPrefix(tt.rawPrefix, home)
			if got != tt.want {
				t.Errorf("normalizeDirectoryPrefix(%q) = %q, want %q", tt.rawPrefix, got, tt.want)
			}
		})
	}
}

func TestSplitDirectoryLookup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		prefix     string
		wantBase   string
		wantPrefix string
		wantOK     bool
	}{
		{name: "root", prefix: "/", wantBase: "/", wantPrefix: "", wantOK: true},
		{name: "trailing slash", prefix: "/tmp/", wantBase: "/tmp", wantPrefix: "", wantOK: true},
		{name: "partial segment", prefix: "/tmp/ab", wantBase: "/tmp", wantPrefix: "ab", wantOK: true},
		{name: "relative invalid", prefix: "tmp/ab", wantBase: "", wantPrefix: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			base, match, ok := splitDirectoryLookup(tt.prefix)
			if ok != tt.wantOK {
				t.Fatalf("splitDirectoryLookup(%q) ok = %v, want %v", tt.prefix, ok, tt.wantOK)
			}
			if base != tt.wantBase || match != tt.wantPrefix {
				t.Errorf("splitDirectoryLookup(%q) = (%q, %q), want (%q, %q)", tt.prefix, base, match, tt.wantBase, tt.wantPrefix)
			}
		})
	}
}

func TestListDirectoriesHandler(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "alpha"), 0o750); err != nil {
		t.Fatalf("mkdir alpha: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(base, "alphabet"), 0o750); err != nil {
		t.Fatalf("mkdir alphabet: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(base, "beta"), 0o750); err != nil {
		t.Fatalf("mkdir beta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "alpha.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	h := newTestHandler(t, &mockTmux{}, nil)
	req := httptest.NewRequest("GET", "/api/fs/dirs?prefix="+url.QueryEscape(filepath.Join(base, "alp"))+"&limit=10", nil)
	w := httptest.NewRecorder()

	h.listDirectories(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var body struct {
		Data struct {
			Dirs []string `json:"dirs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json decode error: %v", err)
	}

	want := []string{
		filepath.Join(base, "alpha"),
		filepath.Join(base, "alphabet"),
	}
	if len(body.Data.Dirs) != len(want) {
		t.Fatalf("dirs len = %d, want %d (%v)", len(body.Data.Dirs), len(want), body.Data.Dirs)
	}
	for i := range want {
		if body.Data.Dirs[i] != want[i] {
			t.Errorf("dirs[%d] = %q, want %q", i, body.Data.Dirs[i], want[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Handler tests with mocks
// ---------------------------------------------------------------------------

func TestListSessionsHandler(t *testing.T) {
	t.Parallel()

	t.Run("success with enrichment", func(t *testing.T) {
		t.Parallel()

		const sessionName = "dev"
		now := time.Now().UTC().Truncate(time.Second)
		tm := &mockTmux{
			listSessionsFn: func(_ context.Context) ([]tmux.Session, error) {
				return []tmux.Session{
					{Name: sessionName, Windows: 2, Attached: 1, CreatedAt: now, ActivityAt: now},
				}, nil
			},
			listActivePaneCommandsFn: func(_ context.Context) (map[string]tmux.PaneSnapshot, error) {
				return map[string]tmux.PaneSnapshot{
					sessionName: {Command: "vim", Panes: 3},
				}, nil
			},
			capturePaneFn: func(_ context.Context, _ string) (string, error) {
				return "$ echo hello", nil
			},
		}
		h := newTestHandler(t, tm, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/tmux/sessions", nil)
		h.listSessions(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}

		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		sessions, _ := data["sessions"].([]any)
		if len(sessions) != 1 {
			t.Fatalf("sessions count = %d, want 1", len(sessions))
		}
		s := sessions[0].(map[string]any)
		if s["name"] != sessionName {
			t.Errorf("name = %v, want %s", s["name"], sessionName)
		}
		if s["command"] != "vim" {
			t.Errorf("command = %v, want vim", s["command"])
		}
		// Panes should come from snapshot (3), not session Windows (2).
		if int(s["panes"].(float64)) != 3 {
			t.Errorf("panes = %v, want 3", s["panes"])
		}
		if s["lastContent"] != "$ echo hello" {
			t.Errorf("lastContent = %v, want '$ echo hello'", s["lastContent"])
		}
		if s["hash"] == nil || s["hash"] == "" {
			t.Error("hash should not be empty")
		}
	})

	t.Run("fallback to list panes when snapshot is missing", func(t *testing.T) {
		t.Parallel()

		const sessionName = "dev"
		now := time.Now().UTC().Truncate(time.Second)
		tm := &mockTmux{
			listSessionsFn: func(_ context.Context) ([]tmux.Session, error) {
				return []tmux.Session{
					{Name: sessionName, Windows: 2, Attached: 1, CreatedAt: now, ActivityAt: now},
				}, nil
			},
			listActivePaneCommandsFn: func(_ context.Context) (map[string]tmux.PaneSnapshot, error) {
				return map[string]tmux.PaneSnapshot{}, nil
			},
			listPanesFn: func(_ context.Context, session string) ([]tmux.Pane, error) {
				if session != sessionName {
					t.Fatalf("unexpected session %q", session)
				}
				return []tmux.Pane{
					{PaneID: "%1"},
					{PaneID: "%2"},
					{PaneID: "%3"},
					{PaneID: "%4"},
				}, nil
			},
		}
		h := newTestHandler(t, tm, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/tmux/sessions", nil)
		h.listSessions(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}

		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		sessions, _ := data["sessions"].([]any)
		if len(sessions) != 1 {
			t.Fatalf("sessions count = %d, want 1", len(sessions))
		}
		s := sessions[0].(map[string]any)
		if int(s["panes"].(float64)) != 4 {
			t.Errorf("panes = %v, want 4", s["panes"])
		}
	})

	t.Run("tmux error", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			listSessionsFn: func(_ context.Context) ([]tmux.Session, error) {
				return nil, &tmux.Error{Kind: tmux.ErrKindNotFound}
			},
		}
		h := newTestHandler(t, tm, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/tmux/sessions", nil)
		h.listSessions(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", w.Code)
		}
	})

	t.Run("graceful fallback on pane commands error", func(t *testing.T) {
		t.Parallel()

		now := time.Now().UTC().Truncate(time.Second)
		tm := &mockTmux{
			listSessionsFn: func(_ context.Context) ([]tmux.Session, error) {
				return []tmux.Session{
					{Name: "s1", Windows: 1, CreatedAt: now, ActivityAt: now},
				}, nil
			},
			listActivePaneCommandsFn: func(_ context.Context) (map[string]tmux.PaneSnapshot, error) {
				return nil, &tmux.Error{Kind: tmux.ErrKindCommandFailed}
			},
		}
		h := newTestHandler(t, tm, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/tmux/sessions", nil)
		h.listSessions(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})
}

func TestListSessionsHandlerProjectedFromWatchtower(t *testing.T) {
	t.Parallel()

	const sessionName = "dev"
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	tm := &mockTmux{
		listSessionsFn: func(_ context.Context) ([]tmux.Session, error) {
			return nil, &tmux.Error{Kind: tmux.ErrKindCommandFailed}
		},
	}
	h := newTestHandler(t, tm, nil)

	if err := h.store.UpsertSession(ctx, sessionName, "h-fixed", "legacy"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	if err := h.store.SetIcon(ctx, sessionName, "bolt"); err != nil {
		t.Fatalf("SetIcon: %v", err)
	}
	if err := h.store.UpsertWatchtowerSession(ctx, store.WatchtowerSessionWrite{
		SessionName:       sessionName,
		Attached:          1,
		Windows:           2,
		Panes:             3,
		ActivityAt:        now,
		LastPreview:       "tail from watchtower",
		LastPreviewAt:     now,
		LastPreviewPaneID: "%5",
		UnreadWindows:     1,
		UnreadPanes:       2,
		Rev:               7,
		UpdatedAt:         now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerSession: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/tmux/sessions", nil)
	h.listSessions(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	sessions, _ := data["sessions"].([]any)
	if len(sessions) != 1 {
		t.Fatalf("sessions count = %d, want 1", len(sessions))
	}
	item := sessions[0].(map[string]any)
	if item["name"] != sessionName {
		t.Fatalf("name = %v, want %s", item["name"], sessionName)
	}
	if item["lastContent"] != "tail from watchtower" {
		t.Fatalf("lastContent = %v, want tail from watchtower", item["lastContent"])
	}
	if item["hash"] != "h-fixed" {
		t.Fatalf("hash = %v, want h-fixed", item["hash"])
	}
	if item["icon"] != "bolt" {
		t.Fatalf("icon = %v, want bolt", item["icon"])
	}
	if int(item["unreadWindows"].(float64)) != 1 {
		t.Fatalf("unreadWindows = %v, want 1", item["unreadWindows"])
	}
	if int(item["unreadPanes"].(float64)) != 2 {
		t.Fatalf("unreadPanes = %v, want 2", item["unreadPanes"])
	}
	if int64(item["rev"].(float64)) != 7 {
		t.Fatalf("rev = %v, want 7", item["rev"])
	}
}

func TestCreateSessionHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions", strings.NewReader(`{"name":"test-session"}`))
		h.createSession(w, r)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		if data["name"] != "test-session" {
			t.Errorf("name = %v, want test-session", data["name"])
		}
	})

	t.Run("success with cwd", func(t *testing.T) {
		t.Parallel()

		var gotCwd string
		tm := &mockTmux{
			createSessionFn: func(_ context.Context, _, cwd string) error {
				gotCwd = cwd
				return nil
			},
		}
		h := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions", strings.NewReader(`{"name":"s1","cwd":"/tmp"}`))
		h.createSession(w, r)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201", w.Code)
		}
		if gotCwd != "/tmp" {
			t.Errorf("cwd = %q, want /tmp", gotCwd)
		}
	})

	t.Run("success defaults cwd to home when empty", func(t *testing.T) {
		t.Parallel()

		var gotCwd string
		tm := &mockTmux{
			createSessionFn: func(_ context.Context, _, cwd string) error {
				gotCwd = cwd
				return nil
			},
		}
		h := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions", strings.NewReader(`{"name":"s1","cwd":""}`))
		h.createSession(w, r)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201", w.Code)
		}
		if gotCwd != defaultSessionCWD() {
			t.Errorf("cwd = %q, want %q", gotCwd, defaultSessionCWD())
		}
	})

	t.Run("invalid name", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions", strings.NewReader(`{"name":"bad name!"}`))
		h.createSession(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("relative cwd", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions", strings.NewReader(`{"name":"s1","cwd":"relative/path"}`))
		h.createSession(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions", strings.NewReader(`{bad}`))
		h.createSession(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("session exists error", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			createSessionFn: func(_ context.Context, _, _ string) error {
				return &tmux.Error{Kind: tmux.ErrKindSessionExists}
			},
		}
		h := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions", strings.NewReader(`{"name":"existing"}`))
		h.createSession(w, r)

		if w.Code != http.StatusConflict {
			t.Errorf("status = %d, want 409", w.Code)
		}
	})
}

func TestDeleteSessionHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/api/tmux/sessions/dev", nil)
		r.SetPathValue("session", "dev")
		h.deleteSession(w, r)

		if w.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204", w.Code)
		}
	})

	t.Run("invalid name", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/api/tmux/sessions/bad%20name", nil)
		r.SetPathValue("session", "bad name")
		h.deleteSession(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("session not found", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			killSessionFn: func(_ context.Context, _ string) error {
				return &tmux.Error{Kind: tmux.ErrKindSessionNotFound}
			},
		}
		h := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/api/tmux/sessions/ghost", nil)
		r.SetPathValue("session", "ghost")
		h.deleteSession(w, r)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})
}

func TestRenameSessionHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PATCH", "/api/tmux/sessions/old", strings.NewReader(`{"newName":"new"}`))
		r.SetPathValue("session", "old")
		h.renameSession(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		if data["name"] != "new" {
			t.Errorf("name = %v, want new", data["name"])
		}
	})

	t.Run("invalid source session", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PATCH", "/api/tmux/sessions/bad%20name", strings.NewReader(`{"newName":"new"}`))
		r.SetPathValue("session", "bad name")
		h.renameSession(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid new name", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PATCH", "/api/tmux/sessions/old", strings.NewReader(`{"newName":"bad name!"}`))
		r.SetPathValue("session", "old")
		h.renameSession(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PATCH", "/api/tmux/sessions/old", strings.NewReader(`{bad}`))
		r.SetPathValue("session", "old")
		h.renameSession(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("tmux error", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			renameSessionFn: func(_ context.Context, _, _ string) error {
				return &tmux.Error{Kind: tmux.ErrKindSessionNotFound}
			},
		}
		h := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PATCH", "/api/tmux/sessions/old", strings.NewReader(`{"newName":"new"}`))
		r.SetPathValue("session", "old")
		h.renameSession(w, r)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})
}

func TestRenameWindowHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/rename-window", strings.NewReader(`{"index":1,"name":"editor"}`))
		r.SetPathValue("session", "dev")
		h.renameWindow(w, r)

		if w.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204", w.Code)
		}
	})

	t.Run("invalid session name", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/bad%20name/rename-window", strings.NewReader(`{"index":0,"name":"main"}`))
		r.SetPathValue("session", "bad name")
		h.renameWindow(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("negative index", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/rename-window", strings.NewReader(`{"index":-1,"name":"main"}`))
		r.SetPathValue("session", "dev")
		h.renameWindow(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid name", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/rename-window", strings.NewReader(`{"index":0,"name":"  "}`))
		r.SetPathValue("session", "dev")
		h.renameWindow(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/rename-window", strings.NewReader(`{bad}`))
		r.SetPathValue("session", "dev")
		h.renameWindow(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("tmux error", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			renameWindowFn: func(_ context.Context, _ string, _ int, _ string) error {
				return &tmux.Error{Kind: tmux.ErrKindSessionNotFound}
			},
		}
		h := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/rename-window", strings.NewReader(`{"index":0,"name":"main"}`))
		r.SetPathValue("session", "dev")
		h.renameWindow(w, r)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})
}

func TestRenamePaneHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/rename-pane", strings.NewReader(`{"paneId":"%5","title":"logs"}`))
		r.SetPathValue("session", "dev")
		h.renamePane(w, r)

		if w.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204", w.Code)
		}
	})

	t.Run("invalid session name", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/bad%20name/rename-pane", strings.NewReader(`{"paneId":"%0","title":"main"}`))
		r.SetPathValue("session", "bad name")
		h.renamePane(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("paneId without percent", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/rename-pane", strings.NewReader(`{"paneId":"5","title":"main"}`))
		r.SetPathValue("session", "dev")
		h.renamePane(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid title", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/rename-pane", strings.NewReader(`{"paneId":"%0","title":"  "}`))
		r.SetPathValue("session", "dev")
		h.renamePane(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/rename-pane", strings.NewReader(`{bad}`))
		r.SetPathValue("session", "dev")
		h.renamePane(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("tmux error", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			renamePaneFn: func(_ context.Context, _, _ string) error {
				return &tmux.Error{Kind: tmux.ErrKindCommandFailed}
			},
		}
		h := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/rename-pane", strings.NewReader(`{"paneId":"%0","title":"main"}`))
		r.SetPathValue("session", "dev")
		h.renamePane(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", w.Code)
		}
	})
}

func TestSetSessionIconHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		// Seed a session so the store has a row to update.
		if err := h.store.UpsertSession(context.Background(), "dev", "h1", "c1"); err != nil {
			t.Fatalf("UpsertSession error = %v", err)
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PATCH", "/api/tmux/sessions/dev/icon", strings.NewReader(`{"icon":"bot"}`))
		r.SetPathValue("session", "dev")
		h.setSessionIcon(w, r)

		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", w.Code)
		}

		// Verify icon was persisted.
		meta, err := h.store.GetAll(context.Background())
		if err != nil {
			t.Fatalf("GetAll error = %v", err)
		}
		if meta["dev"].Icon != "bot" {
			t.Errorf("icon = %q, want bot", meta["dev"].Icon)
		}
	})

	t.Run("invalid session name", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PATCH", "/api/tmux/sessions/bad%20name/icon", strings.NewReader(`{"icon":"bot"}`))
		r.SetPathValue("session", "bad name")
		h.setSessionIcon(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid icon key", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PATCH", "/api/tmux/sessions/dev/icon", strings.NewReader(`{"icon":"Bad Icon!"}`))
		r.SetPathValue("session", "dev")
		h.setSessionIcon(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("empty icon key", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PATCH", "/api/tmux/sessions/dev/icon", strings.NewReader(`{"icon":""}`))
		r.SetPathValue("session", "dev")
		h.setSessionIcon(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PATCH", "/api/tmux/sessions/dev/icon", strings.NewReader(`{bad}`))
		r.SetPathValue("session", "dev")
		h.setSessionIcon(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})
}

func TestListWindowsHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			listWindowsFn: func(_ context.Context, _ string) ([]tmux.Window, error) {
				return []tmux.Window{
					{Session: "dev", Index: 0, Name: "main", Active: true, Panes: 1},
					{Session: "dev", Index: 1, Name: "logs", Active: false, Panes: 2},
				}, nil
			},
		}
		h := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/tmux/sessions/dev/windows", nil)
		r.SetPathValue("session", "dev")
		h.listWindows(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		windows, _ := data["windows"].([]any)
		if len(windows) != 2 {
			t.Errorf("windows count = %d, want 2", len(windows))
		}
	})

	t.Run("invalid session name", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/tmux/sessions/bad%20name/windows", nil)
		r.SetPathValue("session", "bad name")
		h.listWindows(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("tmux error", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			listWindowsFn: func(_ context.Context, _ string) ([]tmux.Window, error) {
				return nil, &tmux.Error{Kind: tmux.ErrKindSessionNotFound}
			},
		}
		h := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/tmux/sessions/ghost/windows", nil)
		r.SetPathValue("session", "ghost")
		h.listWindows(w, r)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})
}

func TestListWindowsHandlerProjectedFromWatchtower(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	tm := &mockTmux{
		listWindowsFn: func(_ context.Context, _ string) ([]tmux.Window, error) {
			return nil, &tmux.Error{Kind: tmux.ErrKindCommandFailed}
		},
	}
	h := newTestHandler(t, tm, nil)

	if err := h.store.UpsertWatchtowerSession(ctx, store.WatchtowerSessionWrite{
		SessionName: "dev",
		ActivityAt:  now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerSession: %v", err)
	}
	if err := h.store.UpsertWatchtowerWindow(ctx, store.WatchtowerWindowWrite{
		SessionName:      "dev",
		WindowIndex:      0,
		Name:             "main",
		Active:           true,
		Layout:           "layout-0",
		WindowActivityAt: now,
		UnreadPanes:      1,
		HasUnread:        true,
		Rev:              3,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerWindow(0): %v", err)
	}
	if err := h.store.UpsertWatchtowerWindow(ctx, store.WatchtowerWindowWrite{
		SessionName:      "dev",
		WindowIndex:      1,
		Name:             "logs",
		Active:           false,
		Layout:           "layout-1",
		WindowActivityAt: now,
		UnreadPanes:      0,
		HasUnread:        false,
		Rev:              2,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerWindow(1): %v", err)
	}
	for _, pane := range []store.WatchtowerPaneWrite{
		{
			PaneID:         "%1",
			SessionName:    "dev",
			WindowIndex:    0,
			PaneIndex:      0,
			Revision:       2,
			SeenRevision:   1,
			TailCapturedAt: now,
			ChangedAt:      now,
			UpdatedAt:      now,
		},
		{
			PaneID:         "%2",
			SessionName:    "dev",
			WindowIndex:    1,
			PaneIndex:      0,
			Revision:       1,
			SeenRevision:   1,
			TailCapturedAt: now,
			ChangedAt:      now,
			UpdatedAt:      now,
		},
	} {
		if err := h.store.UpsertWatchtowerPane(ctx, pane); err != nil {
			t.Fatalf("UpsertWatchtowerPane(%s): %v", pane.PaneID, err)
		}
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/tmux/sessions/dev/windows", nil)
	r.SetPathValue("session", "dev")
	h.listWindows(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	windows, _ := data["windows"].([]any)
	if len(windows) != 2 {
		t.Fatalf("windows len = %d, want 2", len(windows))
	}

	first := windows[0].(map[string]any)
	if first["name"] != "main" {
		t.Fatalf("first window name = %v, want main", first["name"])
	}
	if int(first["panes"].(float64)) != 1 {
		t.Fatalf("first window panes = %v, want 1", first["panes"])
	}
	if first["hasUnread"] != true {
		t.Fatalf("first window hasUnread = %v, want true", first["hasUnread"])
	}
	if int(first["unreadPanes"].(float64)) != 1 {
		t.Fatalf("first window unreadPanes = %v, want 1", first["unreadPanes"])
	}
	if int64(first["rev"].(float64)) != 3 {
		t.Fatalf("first window rev = %v, want 3", first["rev"])
	}
}

func TestListPanesHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			listPanesFn: func(_ context.Context, _ string) ([]tmux.Pane, error) {
				return []tmux.Pane{
					{Session: "dev", WindowIndex: 0, PaneIndex: 0, PaneID: "%0", Active: true},
				}, nil
			},
		}
		h := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/tmux/sessions/dev/panes", nil)
		r.SetPathValue("session", "dev")
		h.listPanes(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})

	t.Run("invalid session name", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/tmux/sessions/bad%20name/panes", nil)
		r.SetPathValue("session", "bad name")
		h.listPanes(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("tmux error", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			listPanesFn: func(_ context.Context, _ string) ([]tmux.Pane, error) {
				return nil, &tmux.Error{Kind: tmux.ErrKindCommandFailed}
			},
		}
		h := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/tmux/sessions/dev/panes", nil)
		r.SetPathValue("session", "dev")
		h.listPanes(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", w.Code)
		}
	})
}

func TestListPanesHandlerProjectedFromWatchtower(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	tm := &mockTmux{
		listPanesFn: func(_ context.Context, _ string) ([]tmux.Pane, error) {
			return nil, &tmux.Error{Kind: tmux.ErrKindCommandFailed}
		},
	}
	h := newTestHandler(t, tm, nil)

	if err := h.store.UpsertWatchtowerSession(ctx, store.WatchtowerSessionWrite{
		SessionName: "dev",
		ActivityAt:  now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerSession: %v", err)
	}
	if err := h.store.UpsertWatchtowerPane(ctx, store.WatchtowerPaneWrite{
		PaneID:         "%8",
		SessionName:    "dev",
		WindowIndex:    0,
		PaneIndex:      1,
		Title:          "shell",
		Active:         true,
		TTY:            "pts/1",
		CurrentPath:    "/tmp",
		StartCommand:   "bash",
		CurrentCommand: "vim",
		TailPreview:    "line",
		TailCapturedAt: now,
		Revision:       5,
		SeenRevision:   3,
		ChangedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerPane: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/tmux/sessions/dev/panes", nil)
	r.SetPathValue("session", "dev")
	h.listPanes(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	panes, _ := data["panes"].([]any)
	if len(panes) != 1 {
		t.Fatalf("panes len = %d, want 1", len(panes))
	}
	item := panes[0].(map[string]any)
	if item["paneId"] != "%8" {
		t.Fatalf("paneId = %v, want %%8", item["paneId"])
	}
	if item["tailPreview"] != "line" {
		t.Fatalf("tailPreview = %v, want line", item["tailPreview"])
	}
	if int64(item["revision"].(float64)) != 5 {
		t.Fatalf("revision = %v, want 5", item["revision"])
	}
	if int64(item["seenRevision"].(float64)) != 3 {
		t.Fatalf("seenRevision = %v, want 3", item["seenRevision"])
	}
	if item["hasUnread"] != true {
		t.Fatalf("hasUnread = %v, want true", item["hasUnread"])
	}
}

func TestSelectWindowHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/select-window", strings.NewReader(`{"index":2}`))
		r.SetPathValue("session", "dev")
		h.selectWindow(w, r)

		if w.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204", w.Code)
		}
	})

	t.Run("invalid session name", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/bad%20name/select-window", strings.NewReader(`{"index":0}`))
		r.SetPathValue("session", "bad name")
		h.selectWindow(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("negative index", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/select-window", strings.NewReader(`{"index":-1}`))
		r.SetPathValue("session", "dev")
		h.selectWindow(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/select-window", strings.NewReader(`{bad}`))
		r.SetPathValue("session", "dev")
		h.selectWindow(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("tmux error", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			selectWindowFn: func(_ context.Context, _ string, _ int) error {
				return &tmux.Error{Kind: tmux.ErrKindSessionNotFound}
			},
		}
		h := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/ghost/select-window", strings.NewReader(`{"index":0}`))
		r.SetPathValue("session", "ghost")
		h.selectWindow(w, r)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})
}

func TestSelectPaneHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/select-pane", strings.NewReader(`{"paneId":"%5"}`))
		r.SetPathValue("session", "dev")
		h.selectPane(w, r)

		if w.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204", w.Code)
		}
	})

	t.Run("invalid session name", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/bad%20name/select-pane", strings.NewReader(`{"paneId":"%0"}`))
		r.SetPathValue("session", "bad name")
		h.selectPane(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("paneId without percent", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/select-pane", strings.NewReader(`{"paneId":"5"}`))
		r.SetPathValue("session", "dev")
		h.selectPane(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
		body := jsonBody(t, w)
		if errCode(body) != invalidRequestCode {
			t.Errorf("code = %q, want %s", errCode(body), invalidRequestCode)
		}
	})

	t.Run("tmux error", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			selectPaneFn: func(_ context.Context, _ string) error {
				return &tmux.Error{Kind: tmux.ErrKindCommandFailed}
			},
		}
		h := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/select-pane", strings.NewReader(`{"paneId":"%99"}`))
		r.SetPathValue("session", "dev")
		h.selectPane(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", w.Code)
		}
	})
}

func TestNewWindowHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/new-window", nil)
		r.SetPathValue("session", "dev")
		h.newWindow(w, r)

		if w.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204", w.Code)
		}
	})

	t.Run("applies default names for new window and first pane", func(t *testing.T) {
		t.Parallel()

		renamedWindow := ""
		renamedPane := ""
		tm := &mockTmux{
			newWindowFn: func(_ context.Context, _ string) (tmux.NewWindowResult, error) {
				return tmux.NewWindowResult{Index: 1, PaneID: "%42"}, nil
			},
			listWindowsFn: func(_ context.Context, _ string) ([]tmux.Window, error) {
				return []tmux.Window{
					{Index: 0, Name: "win-1"},
					{Index: 1, Name: "win-3"},
					{Index: 2, Name: "zsh"},
				}, nil
			},
			renameWindowFn: func(_ context.Context, _ string, index int, name string) error {
				renamedWindow = fmt.Sprintf("%d:%s", index, name)
				return nil
			},
			renamePaneFn: func(_ context.Context, paneID, title string) error {
				renamedPane = fmt.Sprintf("%s:%s", paneID, title)
				return nil
			},
		}

		h := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/new-window", nil)
		r.SetPathValue("session", "dev")
		h.newWindow(w, r)

		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", w.Code)
		}
		if renamedWindow != "1:win-4" {
			t.Fatalf("renamed window = %q, want %q", renamedWindow, "1:win-4")
		}
		if renamedPane != "%42:pan-42" {
			t.Fatalf("renamed pane = %q, want %q", renamedPane, "%42:pan-42")
		}
	})

	t.Run("allocates monotonic default window names when tmux reuses index", func(t *testing.T) {
		t.Parallel()

		renamedWindows := make([]string, 0, 2)
		callCount := 0
		tm := &mockTmux{
			newWindowFn: func(_ context.Context, _ string) (tmux.NewWindowResult, error) {
				callCount++
				return tmux.NewWindowResult{Index: 0, PaneID: "%10"}, nil
			},
			listWindowsFn: func(_ context.Context, _ string) ([]tmux.Window, error) {
				return nil, fmt.Errorf("temporary list error")
			},
			renameWindowFn: func(_ context.Context, _ string, index int, name string) error {
				renamedWindows = append(renamedWindows, fmt.Sprintf("%d:%s", index, name))
				return nil
			},
		}

		h := newTestHandler(t, tm, nil)

		for i := 0; i < 2; i++ {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/new-window", nil)
			r.SetPathValue("session", "dev")
			h.newWindow(w, r)
			if w.Code != http.StatusNoContent {
				t.Fatalf("call %d status = %d, want 204", i+1, w.Code)
			}
		}

		if callCount != 2 {
			t.Fatalf("newWindow calls = %d, want 2", callCount)
		}
		if len(renamedWindows) != 2 {
			t.Fatalf("renamed windows = %d, want 2", len(renamedWindows))
		}
		if renamedWindows[0] != "0:win-1" || renamedWindows[1] != "0:win-2" {
			t.Fatalf("renamed windows = %v, want [0:win-1 0:win-2]", renamedWindows)
		}
	})

	t.Run("invalid session name", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/bad%20name/new-window", nil)
		r.SetPathValue("session", "bad name")
		h.newWindow(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("tmux error", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			newWindowFn: func(_ context.Context, _ string) (tmux.NewWindowResult, error) {
				return tmux.NewWindowResult{}, &tmux.Error{Kind: tmux.ErrKindServerNotRunning}
			},
		}
		h := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/new-window", nil)
		r.SetPathValue("session", "dev")
		h.newWindow(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", w.Code)
		}
	})
}

func TestKillWindowHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/kill-window", strings.NewReader(`{"index":1}`))
		r.SetPathValue("session", "dev")
		h.killWindow(w, r)

		if w.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204", w.Code)
		}
	})

	t.Run("invalid session name", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/bad%20name/kill-window", strings.NewReader(`{"index":0}`))
		r.SetPathValue("session", "bad name")
		h.killWindow(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("negative index", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/kill-window", strings.NewReader(`{"index":-1}`))
		r.SetPathValue("session", "dev")
		h.killWindow(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/kill-window", strings.NewReader(`{bad}`))
		r.SetPathValue("session", "dev")
		h.killWindow(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("tmux error", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			killWindowFn: func(_ context.Context, _ string, _ int) error {
				return &tmux.Error{Kind: tmux.ErrKindSessionNotFound}
			},
		}
		h := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/ghost/kill-window", strings.NewReader(`{"index":0}`))
		r.SetPathValue("session", "ghost")
		h.killWindow(w, r)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})
}

func TestKillPaneHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/kill-pane", strings.NewReader(`{"paneId":"%3"}`))
		r.SetPathValue("session", "dev")
		h.killPane(w, r)

		if w.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204", w.Code)
		}
	})

	t.Run("invalid session name", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/bad%20name/kill-pane", strings.NewReader(`{"paneId":"%0"}`))
		r.SetPathValue("session", "bad name")
		h.killPane(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("paneId without percent", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/kill-pane", strings.NewReader(`{"paneId":"3"}`))
		r.SetPathValue("session", "dev")
		h.killPane(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("tmux error", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			killPaneFn: func(_ context.Context, _ string) error {
				return &tmux.Error{Kind: tmux.ErrKindCommandFailed}
			},
		}
		h := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/kill-pane", strings.NewReader(`{"paneId":"%99"}`))
		r.SetPathValue("session", "dev")
		h.killPane(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", w.Code)
		}
	})
}

func TestSplitPaneHandler(t *testing.T) {
	t.Parallel()

	t.Run("success vertical", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/split-pane", strings.NewReader(`{"paneId":"%0","direction":"vertical"}`))
		r.SetPathValue("session", "dev")
		h.splitPane(w, r)

		if w.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204", w.Code)
		}
	})

	t.Run("success horizontal", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/split-pane", strings.NewReader(`{"paneId":"%0","direction":"horizontal"}`))
		r.SetPathValue("session", "dev")
		h.splitPane(w, r)

		if w.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204", w.Code)
		}
	})

	t.Run("applies default name for created pane", func(t *testing.T) {
		t.Parallel()

		renamedPane := ""
		tm := &mockTmux{
			splitPaneFn: func(_ context.Context, _, _ string) (string, error) {
				return "%77", nil
			},
			renamePaneFn: func(_ context.Context, paneID, title string) error {
				renamedPane = fmt.Sprintf("%s:%s", paneID, title)
				return nil
			},
		}

		h := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/split-pane", strings.NewReader(`{"paneId":"%0","direction":"vertical"}`))
		r.SetPathValue("session", "dev")
		h.splitPane(w, r)

		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", w.Code)
		}
		if renamedPane != "%77:pan-77" {
			t.Fatalf("renamed pane = %q, want %q", renamedPane, "%77:pan-77")
		}
	})

	t.Run("invalid session name", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/bad%20name/split-pane", strings.NewReader(`{"paneId":"%0","direction":"vertical"}`))
		r.SetPathValue("session", "bad name")
		h.splitPane(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("paneId without percent", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/split-pane", strings.NewReader(`{"paneId":"0","direction":"vertical"}`))
		r.SetPathValue("session", "dev")
		h.splitPane(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid direction", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/split-pane", strings.NewReader(`{"paneId":"%0","direction":"diagonal"}`))
		r.SetPathValue("session", "dev")
		h.splitPane(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("tmux error", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			splitPaneFn: func(_ context.Context, _, _ string) (string, error) {
				return "", &tmux.Error{Kind: tmux.ErrKindCommandFailed}
			},
		}
		h := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/split-pane", strings.NewReader(`{"paneId":"%0","direction":"vertical"}`))
		r.SetPathValue("session", "dev")
		h.splitPane(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", w.Code)
		}
	})
}

func TestListTerminalsHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		sys := &mockSysTerms{
			listSystemFn: func(_ context.Context) ([]terminals.SystemTerminal, error) {
				return []terminals.SystemTerminal{
					{ID: "pts/0", TTY: "pts/0", User: "user", ProcessCount: 2, LeaderPID: 1234, Command: "bash"},
				}, nil
			},
		}
		h := newTestHandler(t, nil, sys)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/terminals", nil)
		h.listTerminals(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		terms, _ := data["terminals"].([]any)
		if len(terms) != 1 {
			t.Errorf("terminals count = %d, want 1", len(terms))
		}
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		sys := &mockSysTerms{
			listSystemFn: func(_ context.Context) ([]terminals.SystemTerminal, error) {
				return nil, fmt.Errorf("ps failed")
			},
		}
		h := newTestHandler(t, nil, sys)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/terminals", nil)
		h.listTerminals(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", w.Code)
		}
	})
}

func TestGetSystemTerminalHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		sys := &mockSysTerms{
			listProcessesFn: func(_ context.Context, tty string) ([]terminals.TerminalProcess, error) {
				return []terminals.TerminalProcess{
					{PID: 100, PPID: 1, User: "user", Command: "bash", Args: "bash"},
				}, nil
			},
		}
		h := newTestHandler(t, nil, sys)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/terminals/system/pts/0", nil)
		r.SetPathValue("tty", "pts/0")
		h.getSystemTerminal(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		if data["tty"] != "pts/0" {
			t.Errorf("tty = %v, want pts/0", data["tty"])
		}
	})

	t.Run("empty tty", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/terminals/system/", nil)
		r.SetPathValue("tty", "")
		h.getSystemTerminal(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("process list error", func(t *testing.T) {
		t.Parallel()

		sys := &mockSysTerms{
			listProcessesFn: func(_ context.Context, _ string) ([]terminals.TerminalProcess, error) {
				return nil, fmt.Errorf("invalid tty: bad")
			},
		}
		h := newTestHandler(t, nil, sys)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/terminals/system/bad", nil)
		r.SetPathValue("tty", "bad")
		h.getSystemTerminal(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})
}

func TestCloseTerminalHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, nil, nil)
		// Register a terminal so we can close it.
		id, _ := h.terminals.Register("dev", "127.0.0.1", 80, 24, func(_ string) {})
		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/api/terminals/"+id, nil)
		r.SetPathValue("terminal", id)
		h.closeTerminal(w, r)

		if w.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204", w.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/api/terminals/nonexistent", nil)
		r.SetPathValue("terminal", "nonexistent")
		h.closeTerminal(w, r)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})

	t.Run("empty id", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/api/terminals/", nil)
		r.SetPathValue("terminal", "")
		h.closeTerminal(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})
}

func TestRecoveryOverviewHandler(t *testing.T) {
	t.Parallel()

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/recovery/overview", nil)
		h.recoveryOverview(w, r)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t, nil, nil)
		h.recovery = &mockRecovery{
			overviewFn: func(_ context.Context) (recovery.Overview, error) {
				return recovery.Overview{
					BootID: "boot-a",
					KilledSessions: []store.RecoverySession{
						{Name: "dev", State: store.RecoveryStateKilled},
					},
				}, nil
			},
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/recovery/overview", nil)
		h.recoveryOverview(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		overview, _ := data["overview"].(map[string]any)
		if overview["bootId"] != "boot-a" {
			t.Errorf("bootId = %v, want boot-a", overview["bootId"])
		}
	})
}

func TestRestoreRecoverySnapshotHandler(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t, nil, nil)
	h.recovery = &mockRecovery{
		restoreSnapshotAsyncFn: func(_ context.Context, snapshotID int64, options recovery.RestoreOptions) (store.RecoveryJob, error) {
			if snapshotID != 12 {
				t.Fatalf("snapshotID = %d, want 12", snapshotID)
			}
			if options.TargetSession != "dev-restored" {
				t.Fatalf("target session = %q, want dev-restored", options.TargetSession)
			}
			return store.RecoveryJob{
				ID:            "job-1",
				SessionName:   "dev",
				TargetSession: "dev-restored",
				SnapshotID:    12,
				Status:        store.RecoveryJobQueued,
				CreatedAt:     time.Now().UTC(),
			}, nil
		},
	}

	body := `{"mode":"confirm","conflictPolicy":"rename","targetSession":"dev-restored"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/recovery/snapshots/12/restore", strings.NewReader(body))
	r.SetPathValue("snapshot", "12")
	h.restoreRecoverySnapshot(w, r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	payload := jsonBody(t, w)
	data, _ := payload["data"].(map[string]any)
	job, _ := data["job"].(map[string]any)
	if job["id"] != "job-1" {
		t.Errorf("job.id = %v, want job-1", job["id"])
	}
}

func TestActivityDeltaHandler(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	h := newTestHandler(t, nil, nil)

	if err := h.store.SetWatchtowerRuntimeValue(ctx, "global_rev", "3"); err != nil {
		t.Fatalf("SetWatchtowerRuntimeValue(global_rev): %v", err)
	}
	if err := h.store.UpsertWatchtowerSession(ctx, store.WatchtowerSessionWrite{
		SessionName:   "dev",
		Attached:      1,
		Windows:       1,
		Panes:         1,
		ActivityAt:    now,
		LastPreview:   "delta line",
		UnreadWindows: 1,
		UnreadPanes:   1,
		Rev:           3,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerSession(dev): %v", err)
	}
	if err := h.store.UpsertWatchtowerWindow(ctx, store.WatchtowerWindowWrite{
		SessionName:      "dev",
		WindowIndex:      0,
		Name:             "main",
		Active:           true,
		Layout:           "layout",
		WindowActivityAt: now,
		UnreadPanes:      1,
		HasUnread:        true,
		Rev:              2,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerWindow(dev): %v", err)
	}
	if err := h.store.UpsertWatchtowerPane(ctx, store.WatchtowerPaneWrite{
		PaneID:         "%1",
		SessionName:    "dev",
		WindowIndex:    0,
		PaneIndex:      0,
		Title:          "shell",
		Active:         true,
		TTY:            "/dev/pts/1",
		TailHash:       "h1",
		TailPreview:    "delta line",
		TailCapturedAt: now,
		Revision:       4,
		SeenRevision:   2,
		ChangedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerPane(dev): %v", err)
	}
	for rev := 1; rev <= 3; rev++ {
		if _, err := h.store.InsertWatchtowerJournal(ctx, store.WatchtowerJournalWrite{
			GlobalRev:  int64(rev),
			EntityType: "session",
			Session:    "dev",
			WindowIdx:  -1,
			ChangeKind: "activity",
			ChangedAt:  now.Add(time.Duration(rev) * time.Second),
		}); err != nil {
			t.Fatalf("InsertWatchtowerJournal(%d): %v", rev, err)
		}
	}

	t.Run("overflow", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/tmux/activity/delta?since=0&limit=2", nil)
		h.activityDelta(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		if data["overflow"] != true {
			t.Fatalf("overflow = %v, want true", data["overflow"])
		}
		if int64(data["globalRev"].(float64)) != 3 {
			t.Fatalf("globalRev = %v, want 3", data["globalRev"])
		}
		changes, _ := data["changes"].([]any)
		if len(changes) != 2 {
			t.Fatalf("changes len = %d, want 2", len(changes))
		}
	})

	t.Run("success without overflow", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/tmux/activity/delta?since=2&limit=5", nil)
		h.activityDelta(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		if data["overflow"] != false {
			t.Fatalf("overflow = %v, want false", data["overflow"])
		}
		changes, _ := data["changes"].([]any)
		if len(changes) != 1 {
			t.Fatalf("changes len = %d, want 1", len(changes))
		}
		change, _ := changes[0].(map[string]any)
		if int64(change["globalRev"].(float64)) != 3 {
			t.Fatalf("change.globalRev = %v, want 3", change["globalRev"])
		}
		sessionPatches, ok := data["sessionPatches"].([]any)
		if !ok || len(sessionPatches) != 1 {
			t.Fatalf("sessionPatches = %T(%v), want len=1", data["sessionPatches"], data["sessionPatches"])
		}
		sessionPatch, _ := sessionPatches[0].(map[string]any)
		if sessionPatch["name"] != "dev" {
			t.Fatalf("session patch name = %v, want dev", sessionPatch["name"])
		}
		inspectorPatches, ok := data["inspectorPatches"].([]any)
		if !ok || len(inspectorPatches) != 1 {
			t.Fatalf("inspectorPatches = %T(%v), want len=1", data["inspectorPatches"], data["inspectorPatches"])
		}
		inspectorPatch, _ := inspectorPatches[0].(map[string]any)
		if inspectorPatch["session"] != "dev" {
			t.Fatalf("inspector patch session = %v, want dev", inspectorPatch["session"])
		}
	})

	t.Run("invalid since", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/tmux/activity/delta?since=-1", nil)
		h.activityDelta(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
		if code := errCode(jsonBody(t, w)); code != invalidRequestCode {
			t.Fatalf("code = %q, want %s", code, invalidRequestCode)
		}
	})
}

func TestActivityStatsHandler(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h := newTestHandler(t, nil, nil)
	for key, value := range map[string]string{
		"global_rev":                    "11",
		"collect_total":                 "25",
		"collect_errors_total":          "2",
		"last_collect_at":               "2026-02-13T20:00:00Z",
		"last_collect_duration_ms":      "57",
		"last_collect_sessions":         "4",
		"last_collect_changed_sessions": "3",
		"last_collect_error":            "",
	} {
		if err := h.store.SetWatchtowerRuntimeValue(ctx, key, value); err != nil {
			t.Fatalf("SetWatchtowerRuntimeValue(%s): %v", key, err)
		}
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/tmux/activity/stats", nil)
	h.activityStats(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	if int64(data["globalRev"].(float64)) != 11 {
		t.Fatalf("globalRev = %v, want 11", data["globalRev"])
	}
	if int64(data["collectTotal"].(float64)) != 25 {
		t.Fatalf("collectTotal = %v, want 25", data["collectTotal"])
	}
	if int64(data["collectErrorsTotal"].(float64)) != 2 {
		t.Fatalf("collectErrorsTotal = %v, want 2", data["collectErrorsTotal"])
	}
	if int64(data["lastCollectDurationMs"].(float64)) != 57 {
		t.Fatalf("lastCollectDurationMs = %v, want 57", data["lastCollectDurationMs"])
	}
	if int64(data["lastCollectSessions"].(float64)) != 4 {
		t.Fatalf("lastCollectSessions = %v, want 4", data["lastCollectSessions"])
	}
	if int64(data["lastCollectChanged"].(float64)) != 3 {
		t.Fatalf("lastCollectChanged = %v, want 3", data["lastCollectChanged"])
	}
	if data["lastCollectAt"] != "2026-02-13T20:00:00Z" {
		t.Fatalf("lastCollectAt = %v, want 2026-02-13T20:00:00Z", data["lastCollectAt"])
	}
}

func TestMarkSessionSeenHandler(t *testing.T) {
	t.Parallel()

	const sessionName = "dev"

	h := newTestHandler(t, nil, nil)
	hub := events.NewHub()
	eventsCh, unsubscribe := hub.Subscribe(8)
	t.Cleanup(unsubscribe)
	h.events = hub
	now := time.Now().UTC().Truncate(time.Second)
	ctx := context.Background()

	if err := h.store.UpsertWatchtowerSession(ctx, store.WatchtowerSessionWrite{
		SessionName:   sessionName,
		Attached:      1,
		Windows:       1,
		Panes:         1,
		ActivityAt:    now,
		LastPreview:   "log line",
		LastPreviewAt: now,
		UnreadWindows: 1,
		UnreadPanes:   1,
		Rev:           1,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerSession: %v", err)
	}
	if err := h.store.UpsertWatchtowerWindow(ctx, store.WatchtowerWindowWrite{
		SessionName:      sessionName,
		WindowIndex:      0,
		Name:             "main",
		Active:           true,
		Layout:           "layout",
		WindowActivityAt: now,
		UnreadPanes:      1,
		HasUnread:        true,
		Rev:              1,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerWindow: %v", err)
	}
	if err := h.store.UpsertWatchtowerPane(ctx, store.WatchtowerPaneWrite{
		PaneID:         "%1",
		SessionName:    sessionName,
		WindowIndex:    0,
		PaneIndex:      0,
		Title:          "shell",
		Active:         true,
		TailHash:       "h1",
		TailPreview:    "line",
		TailCapturedAt: now,
		Revision:       3,
		SeenRevision:   1,
		ChangedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerPane: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(
		"POST",
		fmt.Sprintf("/api/tmux/sessions/%s/seen", sessionName),
		strings.NewReader(`{"scope":"pane","paneId":"%1"}`),
	)
	r.SetPathValue("session", sessionName)
	h.markSessionSeen(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	if data["acked"] != true {
		t.Fatalf("acked = %v, want true", data["acked"])
	}
	rawPatches, ok := data["sessionPatches"].([]any)
	if !ok || len(rawPatches) != 1 {
		t.Fatalf("sessionPatches = %T(%v), want len=1", data["sessionPatches"], data["sessionPatches"])
	}
	patch, ok := rawPatches[0].(map[string]any)
	if !ok {
		t.Fatalf("session patch type = %T, want map[string]any", rawPatches[0])
	}
	if patch["name"] != sessionName {
		t.Fatalf("session patch name = %v, want dev", patch["name"])
	}
	if patch["unreadPanes"] != float64(0) {
		t.Fatalf("session patch unreadPanes = %v, want 0", patch["unreadPanes"])
	}
	rawInspector, ok := data["inspectorPatches"].([]any)
	if !ok || len(rawInspector) != 1 {
		t.Fatalf("inspectorPatches = %T(%v), want len=1", data["inspectorPatches"], data["inspectorPatches"])
	}
	inspector, ok := rawInspector[0].(map[string]any)
	if !ok {
		t.Fatalf("inspector patch type = %T, want map[string]any", rawInspector[0])
	}
	if inspector["session"] != sessionName {
		t.Fatalf("inspector session = %v, want dev", inspector["session"])
	}
	if windows, ok := inspector["windows"].([]any); !ok || len(windows) != 1 {
		t.Fatalf("inspector windows = %T(%v), want len=1", inspector["windows"], inspector["windows"])
	}
	if panesRaw, ok := inspector["panes"].([]any); !ok || len(panesRaw) != 1 {
		t.Fatalf("inspector panes = %T(%v), want len=1", inspector["panes"], inspector["panes"])
	}

	panes, err := h.store.ListWatchtowerPanes(ctx, sessionName)
	if err != nil {
		t.Fatalf("ListWatchtowerPanes(%s): %v", sessionName, err)
	}
	if len(panes) != 1 {
		t.Fatalf("panes len = %d, want 1", len(panes))
	}
	if panes[0].SeenRevision != panes[0].Revision {
		t.Fatalf("seenRevision = %d, revision = %d, want equal", panes[0].SeenRevision, panes[0].Revision)
	}

	session, err := h.store.GetWatchtowerSession(ctx, sessionName)
	if err != nil {
		t.Fatalf("GetWatchtowerSession(%s): %v", sessionName, err)
	}
	if session.UnreadPanes != 0 || session.UnreadWindows != 0 {
		t.Fatalf("unexpected unread counters after seen: %+v", session)
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
	if sessionsEvent.Payload["action"] != "seen" {
		t.Fatalf("sessions event action = %v, want seen", sessionsEvent.Payload["action"])
	}
	eventRawPatches, ok := sessionsEvent.Payload["sessionPatches"].([]map[string]any)
	if !ok || len(eventRawPatches) != 1 {
		t.Fatalf("sessions event sessionPatches = %T(%v), want len=1", sessionsEvent.Payload["sessionPatches"], sessionsEvent.Payload["sessionPatches"])
	}
	if eventRawPatches[0]["name"] != sessionName || eventRawPatches[0]["unreadPanes"] != 0 {
		t.Fatalf("unexpected sessions event patch: %+v", eventRawPatches[0])
	}
	eventInspector, ok := sessionsEvent.Payload["inspectorPatches"].([]map[string]any)
	if !ok || len(eventInspector) != 1 {
		t.Fatalf("sessions event inspectorPatches = %T(%v), want len=1", sessionsEvent.Payload["inspectorPatches"], sessionsEvent.Payload["inspectorPatches"])
	}
	if eventInspector[0]["session"] != sessionName {
		t.Fatalf("unexpected sessions event inspector patch: %+v", eventInspector[0])
	}
}

func TestMarkSessionSeenHandlerInvalidScope(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t, nil, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/seen", strings.NewReader(`{"scope":"bad"}`))
	r.SetPathValue("session", "dev")
	h.markSessionSeen(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if code := errCode(jsonBody(t, w)); code != invalidRequestCode {
		t.Fatalf("code = %q, want %s", code, invalidRequestCode)
	}
}

func TestSetTmuxPresenceHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/api/tmux/presence", strings.NewReader(`{
		  "terminalId":"term-1",
		  "session":"dev",
		  "windowIndex":1,
		  "paneId":"%11",
		  "visible":true,
		  "focused":true
		}`))
		h.setTmuxPresence(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}

		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		if data["accepted"] != true {
			t.Fatalf("accepted = %v, want true", data["accepted"])
		}

		presence, err := h.store.ListWatchtowerPresenceBySession(context.Background(), "dev")
		if err != nil {
			t.Fatalf("ListWatchtowerPresenceBySession(dev): %v", err)
		}
		if len(presence) != 1 {
			t.Fatalf("presence len = %d, want 1", len(presence))
		}
		if presence[0].TerminalID != "term-1" || !presence[0].Visible || !presence[0].Focused {
			t.Fatalf("unexpected presence row: %+v", presence[0])
		}
	})

	t.Run("invalid pane id", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/api/tmux/presence", strings.NewReader(`{
		  "terminalId":"term-1",
		  "session":"dev",
		  "windowIndex":1,
		  "paneId":"11",
		  "visible":true,
		  "focused":false
		}`))
		h.setTmuxPresence(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
		if code := errCode(jsonBody(t, w)); code != invalidRequestCode {
			t.Fatalf("code = %q, want %s", code, invalidRequestCode)
		}
	})
}
