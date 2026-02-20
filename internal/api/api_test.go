package api

import (
	"context"
	"database/sql"
	"encoding/base64"
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

	"github.com/opus-domini/sentinel/internal/activity"
	"github.com/opus-domini/sentinel/internal/alerts"
	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/guardrails"
	"github.com/opus-domini/sentinel/internal/recovery"
	"github.com/opus-domini/sentinel/internal/security"
	opsplane "github.com/opus-domini/sentinel/internal/services"
	"github.com/opus-domini/sentinel/internal/store"
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

type mockSysTerms struct{}

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

type mockOpsControlPlane struct {
	overviewFn      func(ctx context.Context) (opsplane.Overview, error)
	listServicesFn  func(ctx context.Context) ([]opsplane.ServiceStatus, error)
	actFn           func(ctx context.Context, name, action string) (opsplane.ServiceStatus, error)
	inspectFn       func(ctx context.Context, name string) (opsplane.ServiceInspect, error)
	logsFn          func(ctx context.Context, name string, lines int) (string, error)
	metricsFn       func(ctx context.Context) opsplane.HostMetrics
	discoverFn      func(ctx context.Context) ([]opsplane.AvailableService, error)
	browseFn        func(ctx context.Context) ([]opsplane.BrowsedService, error)
	actByUnitFn     func(ctx context.Context, unit, scope, manager, action string) error
	inspectByUnitFn func(ctx context.Context, unit, scope, manager string) (opsplane.ServiceInspect, error)
	logsByUnitFn    func(ctx context.Context, unit, scope, manager string, lines int) (string, error)
}

func (m *mockOpsControlPlane) Overview(ctx context.Context) (opsplane.Overview, error) {
	if m.overviewFn != nil {
		return m.overviewFn(ctx)
	}
	return opsplane.Overview{}, nil
}

func (m *mockOpsControlPlane) ListServices(ctx context.Context) ([]opsplane.ServiceStatus, error) {
	if m.listServicesFn != nil {
		return m.listServicesFn(ctx)
	}
	return []opsplane.ServiceStatus{}, nil
}

func (m *mockOpsControlPlane) Act(ctx context.Context, name, action string) (opsplane.ServiceStatus, error) {
	if m.actFn != nil {
		return m.actFn(ctx, name, action)
	}
	return opsplane.ServiceStatus{}, nil
}

func (m *mockOpsControlPlane) Inspect(ctx context.Context, name string) (opsplane.ServiceInspect, error) {
	if m.inspectFn != nil {
		return m.inspectFn(ctx, name)
	}
	return opsplane.ServiceInspect{}, nil
}

func (m *mockOpsControlPlane) Logs(ctx context.Context, name string, lines int) (string, error) {
	if m.logsFn != nil {
		return m.logsFn(ctx, name, lines)
	}
	return "", nil
}

func (m *mockOpsControlPlane) Metrics(ctx context.Context) opsplane.HostMetrics {
	if m.metricsFn != nil {
		return m.metricsFn(ctx)
	}
	return opsplane.HostMetrics{}
}

func (m *mockOpsControlPlane) DiscoverServices(ctx context.Context) ([]opsplane.AvailableService, error) {
	if m.discoverFn != nil {
		return m.discoverFn(ctx)
	}
	return []opsplane.AvailableService{}, nil
}

func (m *mockOpsControlPlane) BrowseServices(ctx context.Context) ([]opsplane.BrowsedService, error) {
	if m.browseFn != nil {
		return m.browseFn(ctx)
	}
	return []opsplane.BrowsedService{}, nil
}

func (m *mockOpsControlPlane) ActByUnit(ctx context.Context, unit, scope, manager, action string) error {
	if m.actByUnitFn != nil {
		return m.actByUnitFn(ctx, unit, scope, manager, action)
	}
	return nil
}

func (m *mockOpsControlPlane) InspectByUnit(ctx context.Context, unit, scope, manager string) (opsplane.ServiceInspect, error) {
	if m.inspectByUnitFn != nil {
		return m.inspectByUnitFn(ctx, unit, scope, manager)
	}
	return opsplane.ServiceInspect{}, nil
}

func (m *mockOpsControlPlane) LogsByUnit(ctx context.Context, unit, scope, manager string, lines int) (string, error) {
	if m.logsByUnitFn != nil {
		return m.logsByUnitFn(ctx, unit, scope, manager, lines)
	}
	return "", nil
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

func newTestHandler(t *testing.T, tm *mockTmux, sys *mockSysTerms) (*Handler, *store.Store) {
	t.Helper()
	guard := security.New("", nil, security.CookieSecureAuto)
	st := newTestStore(t)
	if tm == nil {
		tm = &mockTmux{}
	}
	_ = sys
	runCtx, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)
	return &Handler{
		guard:     guard,
		tmux:      tm,
		ops:       &mockOpsControlPlane{},
		repo:      st,
		orch:      &opsOrchestrator{repo: st},
		runCtx:    runCtx,
		runCancel: runCancel,
	}, st
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
		cookie     string
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
			name:       "token required wrong cookie token",
			token:      "secret",
			cookie:     "wrong",
			host:       "localhost:4040",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "token required correct cookie token",
			token:      "secret",
			cookie:     "secret",
			host:       "localhost:4040",
			wantStatus: http.StatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			guard := security.New(tt.token, tt.origins, security.CookieSecureAuto)
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
			if tt.cookie != "" {
				r.AddCookie(&http.Cookie{
					Name:  security.AuthCookieName,
					Value: base64.RawURLEncoding.EncodeToString([]byte(tt.cookie)),
				})
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
		version      string
		wantRequired bool
		wantVersion  string
	}{
		{
			name:         "token configured",
			token:        "secret",
			version:      "1.2.3",
			wantRequired: true,
			wantVersion:  "1.2.3",
		},
		{
			name:         "no token",
			token:        "",
			version:      "",
			wantRequired: false,
			wantVersion:  defaultMetaVersion,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			guard := security.New(tt.token, nil, security.CookieSecureAuto)
			h := &Handler{guard: guard, version: tt.version}

			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/api/meta", nil)
			h.meta(w, r)

			var body struct {
				Data struct {
					TokenRequired bool   `json:"tokenRequired"`
					DefaultCwd    string `json:"defaultCwd"`
					Version       string `json:"version"`
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
			if body.Data.Version != tt.wantVersion {
				t.Errorf("version = %q, want %q", body.Data.Version, tt.wantVersion)
			}
		})
	}
}

func TestSetAuthTokenHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		configToken string
		body        string
		wantStatus  int
		wantCookie  bool
		wantCode    string
	}{
		{
			name:        "token not required",
			configToken: "",
			body:        `{"token":"anything"}`,
			wantStatus:  http.StatusOK,
			wantCookie:  false,
		},
		{
			name:        "invalid json",
			configToken: "secret",
			body:        `{not-json}`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    invalidRequestCode,
		},
		{
			name:        "empty token",
			configToken: "secret",
			body:        `{"token":"   "}`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    invalidRequestCode,
		},
		{
			name:        "wrong token",
			configToken: "secret",
			body:        `{"token":"wrong"}`,
			wantStatus:  http.StatusUnauthorized,
			wantCode:    "UNAUTHORIZED",
		},
		{
			name:        "valid token",
			configToken: "secret",
			body:        `{"token":"secret"}`,
			wantStatus:  http.StatusOK,
			wantCookie:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := &Handler{guard: security.New(tt.configToken, nil, security.CookieSecureAuto)}
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPut, "/api/auth/token", strings.NewReader(tt.body))
			r.Header.Set("Content-Type", "application/json")

			h.setAuthToken(w, r)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantCode != "" {
				if code := errCode(jsonBody(t, w)); code != tt.wantCode {
					t.Fatalf("code = %q, want %q", code, tt.wantCode)
				}
			}

			cookies := w.Result().Cookies()
			hasAuthCookie := false
			for _, cookie := range cookies {
				if cookie.Name == security.AuthCookieName {
					hasAuthCookie = true
					break
				}
			}
			if hasAuthCookie != tt.wantCookie {
				t.Fatalf("auth cookie present = %v, want %v", hasAuthCookie, tt.wantCookie)
			}
		})
	}
}

func TestClearAuthTokenHandler(t *testing.T) {
	t.Parallel()

	h := &Handler{guard: security.New("secret", nil, security.CookieSecureAuto)}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/api/auth/token", nil)
	h.clearAuthToken(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies len = %d, want 1", len(cookies))
	}
	if cookies[0].Name != security.AuthCookieName {
		t.Fatalf("cookie name = %q, want %q", cookies[0].Name, security.AuthCookieName)
	}
	if cookies[0].MaxAge >= 0 {
		t.Fatalf("cookie MaxAge = %d, want negative", cookies[0].MaxAge)
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

	h, _ := newTestHandler(t, &mockTmux{}, nil)
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
		h, _ := newTestHandler(t, tm, nil)

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
		h, _ := newTestHandler(t, tm, nil)

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
		h, _ := newTestHandler(t, tm, nil)

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
		h, _ := newTestHandler(t, tm, nil)

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
	h, st := newTestHandler(t, tm, nil)

	if err := st.UpsertSession(ctx, sessionName, "h-fixed", "legacy"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	if err := st.SetIcon(ctx, sessionName, "bolt"); err != nil {
		t.Fatalf("SetIcon: %v", err)
	}
	if err := st.UpsertWatchtowerSession(ctx, store.WatchtowerSessionWrite{
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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
		h, _ := newTestHandler(t, tm, nil)
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
		h, _ := newTestHandler(t, tm, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions", strings.NewReader(`{"name":"bad name!"}`))
		h.createSession(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("relative cwd", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions", strings.NewReader(`{"name":"s1","cwd":"relative/path"}`))
		h.createSession(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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
		h, _ := newTestHandler(t, tm, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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
		h, _ := newTestHandler(t, tm, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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
		h, _ := newTestHandler(t, tm, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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
		h, _ := newTestHandler(t, tm, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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
		h, _ := newTestHandler(t, tm, nil)
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

		h, st := newTestHandler(t, &mockTmux{}, nil)
		// Seed a session so the store has a row to update.
		if err := st.UpsertSession(context.Background(), "dev", "h1", "c1"); err != nil {
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
		meta, err := st.GetAll(context.Background())
		if err != nil {
			t.Fatalf("GetAll error = %v", err)
		}
		if meta["dev"].Icon != "bot" {
			t.Errorf("icon = %q, want bot", meta["dev"].Icon)
		}
	})

	t.Run("invalid session name", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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
		h, _ := newTestHandler(t, tm, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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
		h, _ := newTestHandler(t, tm, nil)
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
	h, st := newTestHandler(t, tm, nil)

	if err := st.UpsertWatchtowerSession(ctx, store.WatchtowerSessionWrite{
		SessionName: "dev",
		ActivityAt:  now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerSession: %v", err)
	}
	if err := st.UpsertWatchtowerWindow(ctx, store.WatchtowerWindowWrite{
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
	if err := st.UpsertWatchtowerWindow(ctx, store.WatchtowerWindowWrite{
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
		if err := st.UpsertWatchtowerPane(ctx, pane); err != nil {
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
		h, _ := newTestHandler(t, tm, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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
		h, _ := newTestHandler(t, tm, nil)
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
	h, st := newTestHandler(t, tm, nil)

	if err := st.UpsertWatchtowerSession(ctx, store.WatchtowerSessionWrite{
		SessionName: "dev",
		ActivityAt:  now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerSession: %v", err)
	}
	if err := st.UpsertWatchtowerPane(ctx, store.WatchtowerPaneWrite{
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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
		h, _ := newTestHandler(t, tm, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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
		h, _ := newTestHandler(t, tm, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, tm, nil)
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

		h, _ := newTestHandler(t, tm, nil)

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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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
		h, _ := newTestHandler(t, tm, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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
		h, _ := newTestHandler(t, tm, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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
		h, _ := newTestHandler(t, tm, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, tm, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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

		h, _ := newTestHandler(t, &mockTmux{}, nil)
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
		h, _ := newTestHandler(t, tm, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/sessions/dev/split-pane", strings.NewReader(`{"paneId":"%0","direction":"vertical"}`))
		r.SetPathValue("session", "dev")
		h.splitPane(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", w.Code)
		}
	})
}

func TestRecoveryOverviewHandler(t *testing.T) {
	t.Parallel()

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/recovery/overview", nil)
		h.recoveryOverview(w, r)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
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

	h, _ := newTestHandler(t, nil, nil)
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

func TestActivityDeltaHandlerOverflow(t *testing.T) {
	t.Parallel()
	h, _ := seededActivityDeltaHandler(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/tmux/activity/delta?since=0&limit=2", nil)
	h.activityDelta(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	data, _ := jsonBody(t, w)["data"].(map[string]any)
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
}

func TestActivityDeltaHandlerWithoutOverflow(t *testing.T) {
	t.Parallel()
	const sessionName = "dev"
	h, _ := seededActivityDeltaHandler(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/tmux/activity/delta?since=2&limit=5", nil)
	h.activityDelta(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	data, _ := jsonBody(t, w)["data"].(map[string]any)
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
	assertActivityDeltaPatchPayloads(t, data, sessionName)
}

func TestActivityDeltaHandlerInvalidSince(t *testing.T) {
	t.Parallel()
	h, _ := seededActivityDeltaHandler(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/tmux/activity/delta?since=-1", nil)
	h.activityDelta(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if code := errCode(jsonBody(t, w)); code != invalidRequestCode {
		t.Fatalf("code = %q, want %s", code, invalidRequestCode)
	}
}

func seededActivityDeltaHandler(t *testing.T) (*Handler, *store.Store) {
	t.Helper()
	const sessionName = "dev"
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	h, st := newTestHandler(t, nil, nil)
	if err := st.SetWatchtowerRuntimeValue(ctx, "global_rev", "3"); err != nil {
		t.Fatalf("SetWatchtowerRuntimeValue(global_rev): %v", err)
	}
	if err := st.UpsertWatchtowerSession(ctx, store.WatchtowerSessionWrite{
		SessionName:   sessionName,
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
	if err := st.UpsertWatchtowerWindow(ctx, store.WatchtowerWindowWrite{
		SessionName:      sessionName,
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
	if err := st.UpsertWatchtowerPane(ctx, store.WatchtowerPaneWrite{
		PaneID:         "%1",
		SessionName:    sessionName,
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
	seedActivityDeltaJournal(t, st, ctx, now, sessionName)
	return h, st
}

func seedActivityDeltaJournal(t *testing.T, st *store.Store, ctx context.Context, now time.Time, sessionName string) {
	t.Helper()
	for rev := 1; rev <= 3; rev++ {
		if _, err := st.InsertWatchtowerJournal(ctx, store.WatchtowerJournalWrite{
			GlobalRev:  int64(rev),
			EntityType: "session",
			Session:    sessionName,
			WindowIdx:  -1,
			ChangeKind: "activity",
			ChangedAt:  now.Add(time.Duration(rev) * time.Second),
		}); err != nil {
			t.Fatalf("InsertWatchtowerJournal(%d): %v", rev, err)
		}
	}
}

func assertActivityDeltaPatchPayloads(t *testing.T, data map[string]any, sessionName string) {
	t.Helper()
	sessionPatches, ok := data["sessionPatches"].([]any)
	if !ok || len(sessionPatches) != 1 {
		t.Fatalf("sessionPatches = %T(%v), want len=1", data["sessionPatches"], data["sessionPatches"])
	}
	sessionPatch, _ := sessionPatches[0].(map[string]any)
	if sessionPatch["name"] != sessionName {
		t.Fatalf("session patch name = %v, want %s", sessionPatch["name"], sessionName)
	}
	inspectorPatches, ok := data["inspectorPatches"].([]any)
	if !ok || len(inspectorPatches) != 1 {
		t.Fatalf("inspectorPatches = %T(%v), want len=1", data["inspectorPatches"], data["inspectorPatches"])
	}
	inspectorPatch, _ := inspectorPatches[0].(map[string]any)
	if inspectorPatch["session"] != sessionName {
		t.Fatalf("inspector patch session = %v, want %s", inspectorPatch["session"], sessionName)
	}
}

func TestActivityStatsHandler(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h, st := newTestHandler(t, nil, nil)
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
		if err := st.SetWatchtowerRuntimeValue(ctx, key, value); err != nil {
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

func TestTimelineSearchHandler(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)

	rows := []store.WatchtowerTimelineEventWrite{
		{
			Session:   "dev",
			WindowIdx: 0,
			PaneID:    "%1",
			EventType: "command.started",
			Severity:  "info",
			Command:   "go test ./...",
			Cwd:       "/repo",
			Summary:   "command started",
			CreatedAt: base.Add(-2 * time.Minute),
		},
		{
			Session:   "dev",
			WindowIdx: 0,
			PaneID:    "%1",
			EventType: "output.marker",
			Severity:  "error",
			Marker:    "panic",
			Summary:   "panic marker",
			Details:   "panic: boom",
			CreatedAt: base.Add(-1 * time.Minute),
		},
		{
			Session:   "ops",
			WindowIdx: 2,
			PaneID:    "%9",
			EventType: "command.finished",
			Severity:  "warn",
			Command:   "deploy",
			Summary:   "deploy finished",
			CreatedAt: base,
		},
	}
	for i, row := range rows {
		if _, err := st.InsertWatchtowerTimelineEvent(ctx, row); err != nil {
			t.Fatalf("InsertWatchtowerTimelineEvent[%d]: %v", i, err)
		}
	}

	t.Run("returns filtered timeline", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			"GET",
			"/api/tmux/timeline?session=dev&q=panic&severity=error&limit=5",
			nil,
		)
		h.timelineSearch(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		eventsRaw, _ := data["events"].([]any)
		if len(eventsRaw) != 1 {
			t.Fatalf("len(events) = %d, want 1", len(eventsRaw))
		}
		event, _ := eventsRaw[0].(map[string]any)
		if event["eventType"] != "output.marker" {
			t.Fatalf("eventType = %v, want output.marker", event["eventType"])
		}
		if data["hasMore"] != false {
			t.Fatalf("hasMore = %v, want false", data["hasMore"])
		}
	})

	t.Run("supports pagination hint", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/tmux/timeline?limit=1", nil)
		h.timelineSearch(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		if data["hasMore"] != true {
			t.Fatalf("hasMore = %v, want true", data["hasMore"])
		}
	})

	t.Run("invalid session", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/tmux/timeline?session=bad%20name", nil)
		h.timelineSearch(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
		if code := errCode(jsonBody(t, w)); code != invalidRequestCode {
			t.Fatalf("code = %q, want %s", code, invalidRequestCode)
		}
	})

	t.Run("invalid since", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/tmux/timeline?since=not-time", nil)
		h.timelineSearch(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
		if code := errCode(jsonBody(t, w)); code != invalidRequestCode {
			t.Fatalf("code = %q, want %s", code, invalidRequestCode)
		}
	})
}

func TestOpsOverviewAndServicesHandlers(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.ops = &mockOpsControlPlane{
		overviewFn: func(context.Context) (opsplane.Overview, error) {
			return opsplane.Overview{
				Host: opsplane.HostOverview{
					Hostname: "devbox",
					OS:       "linux",
				},
				UpdatedAt: "2026-02-15T12:00:00Z",
			}, nil
		},
		listServicesFn: func(context.Context) ([]opsplane.ServiceStatus, error) {
			return []opsplane.ServiceStatus{
				{
					Name:         opsplane.ServiceNameSentinel,
					DisplayName:  "Sentinel service",
					Manager:      "systemd",
					Scope:        "user",
					Unit:         "sentinel",
					Exists:       true,
					EnabledState: "enabled",
					ActiveState:  "active",
					UpdatedAt:    "2026-02-15T12:00:00Z",
				},
			}, nil
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/overview", nil)
	h.opsOverview(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("opsOverview status = %d, want 200", w.Code)
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	overview, _ := data["overview"].(map[string]any)
	host, _ := overview["host"].(map[string]any)
	if host["hostname"] != "devbox" {
		t.Fatalf("host.hostname = %v, want devbox", host["hostname"])
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/api/ops/services", nil)
	h.opsServices(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("opsServices status = %d, want 200", w.Code)
	}
	body = jsonBody(t, w)
	data, _ = body["data"].(map[string]any)
	services, _ := data["services"].([]any)
	if len(services) != 1 {
		t.Fatalf("services len = %d, want 1", len(services))
	}
}

func TestOpsServiceActionHandler(t *testing.T) {
	t.Parallel()

	eventHub := events.NewHub()
	eventsCh, unsubscribe := eventHub.Subscribe(8)
	defer unsubscribe()

	h, _ := newTestHandler(t, nil, nil)
	h.events = eventHub
	h.ops = &mockOpsControlPlane{
		actFn: func(_ context.Context, name, action string) (opsplane.ServiceStatus, error) {
			if name != opsplane.ServiceNameSentinel {
				t.Fatalf("service name = %q, want %q", name, opsplane.ServiceNameSentinel)
			}
			if action != opsplane.ActionRestart {
				t.Fatalf("action = %q, want %q", action, opsplane.ActionRestart)
			}
			return opsplane.ServiceStatus{
				Name:         opsplane.ServiceNameSentinel,
				DisplayName:  "Sentinel service",
				Manager:      "systemd",
				Scope:        "user",
				Unit:         "sentinel",
				Exists:       true,
				EnabledState: "enabled",
				ActiveState:  "active",
				UpdatedAt:    "2026-02-15T12:00:00Z",
			}, nil
		},
		listServicesFn: func(context.Context) ([]opsplane.ServiceStatus, error) {
			return []opsplane.ServiceStatus{
				{
					Name:         opsplane.ServiceNameSentinel,
					DisplayName:  "Sentinel service",
					Manager:      "systemd",
					Scope:        "user",
					Unit:         "sentinel",
					Exists:       true,
					EnabledState: "enabled",
					ActiveState:  "active",
					UpdatedAt:    "2026-02-15T12:00:00Z",
				},
			}, nil
		},
		overviewFn: func(context.Context) (opsplane.Overview, error) {
			return opsplane.Overview{
				UpdatedAt: "2026-02-15T12:00:00Z",
			}, nil
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(
		"POST",
		"/api/ops/services/sentinel/action",
		strings.NewReader(`{"action":"restart"}`),
	)
	r.SetPathValue("service", "sentinel")
	h.opsServiceAction(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("opsServiceAction status = %d, want 200", w.Code)
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	if _, ok := data["globalRev"].(float64); !ok {
		t.Fatalf("globalRev = %T(%v), want float64", data["globalRev"], data["globalRev"])
	}

	gotTypes := map[string]bool{}
	timeout := time.After(500 * time.Millisecond)
	for len(gotTypes) < 2 {
		select {
		case evt := <-eventsCh:
			gotTypes[evt.Type] = true
		case <-timeout:
			t.Fatalf("did not receive ops events, got=%v", gotTypes)
		}
	}
	if !gotTypes[events.TypeOpsServices] || !gotTypes[events.TypeOpsOverview] {
		t.Fatalf("unexpected event types: %v", gotTypes)
	}
}

func TestOpsServiceStatusHandler(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.ops = &mockOpsControlPlane{
		inspectFn: func(_ context.Context, name string) (opsplane.ServiceInspect, error) {
			if name != opsplane.ServiceNameUpdater {
				t.Fatalf("service name = %q, want %q", name, opsplane.ServiceNameUpdater)
			}
			return opsplane.ServiceInspect{
				Service: opsplane.ServiceStatus{
					Name:         opsplane.ServiceNameUpdater,
					DisplayName:  "Autoupdate timer",
					Manager:      "systemd",
					Scope:        "user",
					Unit:         "sentinel-updater.timer",
					Exists:       true,
					EnabledState: "enabled",
					ActiveState:  "active",
					UpdatedAt:    "2026-02-15T12:00:00Z",
				},
				Summary: "load=loaded active=active sub=waiting",
				Properties: map[string]string{
					"LoadState":   "loaded",
					"ActiveState": "active",
					"SubState":    "waiting",
				},
				Output:    "Id=sentinel-updater.timer",
				CheckedAt: "2026-02-15T12:00:01Z",
			}, nil
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/services/sentinel-updater/status", nil)
	r.SetPathValue("service", "sentinel-updater")
	h.opsServiceStatus(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	status, _ := data["status"].(map[string]any)
	service, _ := status["service"].(map[string]any)
	if service["name"] != opsplane.ServiceNameUpdater {
		t.Fatalf("service.name = %v, want %s", service["name"], opsplane.ServiceNameUpdater)
	}
	if status["summary"] != "load=loaded active=active sub=waiting" {
		t.Fatalf("summary = %v, want expected summary", status["summary"])
	}
}

func TestOpsServiceStatusHandlerNotFound(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.ops = &mockOpsControlPlane{
		inspectFn: func(context.Context, string) (opsplane.ServiceInspect, error) {
			return opsplane.ServiceInspect{}, opsplane.ErrServiceNotFound
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/services/missing/status", nil)
	r.SetPathValue("service", "missing")
	h.opsServiceStatus(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	body := jsonBody(t, w)
	if got := errCode(body); got != "OPS_SERVICE_NOT_FOUND" {
		t.Fatalf("error code = %q, want OPS_SERVICE_NOT_FOUND", got)
	}
}

func TestOpsServiceActionHandlerInvalidInput(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.ops = &mockOpsControlPlane{}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(
		"POST",
		"/api/ops/services/sentinel/action",
		strings.NewReader(`{"action":"invalid"}`),
	)
	r.SetPathValue("service", "sentinel")
	h.opsServiceAction(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if code := errCode(jsonBody(t, w)); code != invalidRequestCode {
		t.Fatalf("code = %q, want %s", code, invalidRequestCode)
	}
}

func TestOpsServiceActionHandlerNotFound(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.ops = &mockOpsControlPlane{
		actFn: func(context.Context, string, string) (opsplane.ServiceStatus, error) {
			return opsplane.ServiceStatus{}, opsplane.ErrServiceNotFound
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(
		"POST",
		"/api/ops/services/missing/action",
		strings.NewReader(`{"action":"restart"}`),
	)
	r.SetPathValue("service", "missing")
	h.opsServiceAction(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	body := jsonBody(t, w)
	if got := errCode(body); got != "OPS_SERVICE_NOT_FOUND" {
		t.Fatalf("error code = %q, want OPS_SERVICE_NOT_FOUND", got)
	}
}

func TestOpsAlertsAndActivityHandlers(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	event, err := st.InsertActivityEvent(ctx, activity.EventWrite{
		Source:    "service",
		EventType: "service.action",
		Severity:  "warn",
		Resource:  "sentinel",
		Message:   "service restarted",
		Details:   "test",
		Metadata:  `{"action":"restart"}`,
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("InsertActivityEvent: %v", err)
	}
	alert, err := st.UpsertAlert(ctx, alerts.AlertWrite{
		DedupeKey: "service:sentinel:failed",
		Source:    "service",
		Resource:  "sentinel",
		Title:     "Sentinel failed",
		Message:   "service entered failed state",
		Severity:  "error",
		Metadata:  `{"state":"failed"}`,
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("UpsertAlert: %v", err)
	}

	t.Run("list alerts", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/alerts?status=open&limit=5", nil)
		h.opsAlerts(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		alerts, _ := data["alerts"].([]any)
		if len(alerts) != 1 {
			t.Fatalf("len(alerts) = %d, want 1", len(alerts))
		}
	})

	t.Run("list activity", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/activity?q=restarted&severity=warn&source=service", nil)
		h.opsActivity(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		eventsRaw, _ := data["events"].([]any)
		if len(eventsRaw) != 1 {
			t.Fatalf("len(events) = %d, want 1", len(eventsRaw))
		}
		item, _ := eventsRaw[0].(map[string]any)
		if int64(item["id"].(float64)) != event.ID {
			t.Fatalf("event id = %v, want %d", item["id"], event.ID)
		}
	})

	t.Run("list activity invalid severity", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/activity?severity=invalid", nil)
		h.opsActivity(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
		if code := errCode(jsonBody(t, w)); code != invalidRequestCode {
			t.Fatalf("code = %q, want %s", code, invalidRequestCode)
		}
	})

	t.Run("ack alert publishes events", func(t *testing.T) {
		hub := events.NewHub()
		eventsCh, unsubscribe := hub.Subscribe(8)
		defer unsubscribe()
		h.events = hub

		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", fmt.Sprintf("/api/ops/alerts/%d/ack", alert.ID), nil)
		r.SetPathValue("alert", fmt.Sprintf("%d", alert.ID))
		h.ackOpsAlert(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}

		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		alertBody, _ := data["alert"].(map[string]any)
		if alertBody["status"] != "acked" {
			t.Fatalf("alert.status = %v, want acked", alertBody["status"])
		}

		gotTypes := map[string]bool{}
		timeout := time.After(500 * time.Millisecond)
		for len(gotTypes) < 2 {
			select {
			case evt := <-eventsCh:
				gotTypes[evt.Type] = true
			case <-timeout:
				t.Fatalf("did not receive expected ack events, got=%v", gotTypes)
			}
		}
		if !gotTypes[events.TypeOpsAlerts] || !gotTypes[events.TypeOpsActivity] {
			t.Fatalf("unexpected ack event types: %v", gotTypes)
		}
	})
}

func TestOpsRunbooksAndJobsHandlers(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	ctx := context.Background()

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/runbooks", nil)
	h.opsRunbooks(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("opsRunbooks status = %d, want 200", w.Code)
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	runbooksRaw, _ := data["runbooks"].([]any)
	if len(runbooksRaw) == 0 {
		t.Fatalf("expected seeded runbooks")
	}
	firstRunbook, _ := runbooksRaw[0].(map[string]any)
	runbookID, _ := firstRunbook["id"].(string)
	if runbookID == "" {
		t.Fatalf("runbook id should not be empty")
	}

	eventHub := events.NewHub()
	eventsCh, unsubscribe := eventHub.Subscribe(8)
	defer unsubscribe()
	h.events = eventHub

	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/api/ops/runbooks/"+runbookID+"/run", nil)
	r.SetPathValue("runbook", runbookID)
	h.runOpsRunbook(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("runOpsRunbook status = %d, want 202", w.Code)
	}
	body = jsonBody(t, w)
	data, _ = body["data"].(map[string]any)
	jobRaw, _ := data["job"].(map[string]any)
	jobID, _ := jobRaw["id"].(string)
	if jobID == "" {
		t.Fatalf("job id should not be empty")
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/api/ops/jobs/"+jobID, nil)
	r.SetPathValue("job", jobID)
	h.opsJob(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("opsJob status = %d, want 200", w.Code)
	}

	gotTypes := map[string]bool{}
	timeout := time.After(500 * time.Millisecond)
	for len(gotTypes) < 2 {
		select {
		case evt := <-eventsCh:
			gotTypes[evt.Type] = true
		case <-timeout:
			t.Fatalf("did not receive expected runbook events, got=%v", gotTypes)
		}
	}
	if !gotTypes[events.TypeOpsJob] || !gotTypes[events.TypeOpsActivity] {
		t.Fatalf("unexpected runbook event types: %v", gotTypes)
	}

	loaded, err := st.GetOpsRunbookRun(ctx, jobID)
	if err != nil {
		t.Fatalf("GetOpsRunbookRun(%s): %v", jobID, err)
	}
	if loaded.RunbookID != runbookID {
		t.Fatalf("job runbook id = %q, want %q", loaded.RunbookID, runbookID)
	}
}

func TestStorageStatsAndFlushHandlers(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)
	seedStorageHandlersData(t, st, ctx, base)

	getResources := func(t *testing.T) map[string]map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/storage/stats", nil)
		h.storageStats(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("storageStats status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		rawResources, _ := data["resources"].([]any)
		resources := make(map[string]map[string]any, len(rawResources))
		for _, raw := range rawResources {
			item, _ := raw.(map[string]any)
			name, _ := item["resource"].(string)
			resources[name] = item
		}
		return resources
	}

	resources := getResources(t)
	for _, key := range []string{
		store.StorageResourceTimeline,
		store.StorageResourceActivityLog,
		store.StorageResourceGuardrailLog,
		store.StorageResourceRecoveryLog,
	} {
		item := resources[key]
		if item == nil {
			t.Fatalf("missing resource %q in stats", key)
		}
		rows, _ := item["rows"].(float64)
		if rows < 1 {
			t.Fatalf("resource %q rows = %v, want >= 1", key, item["rows"])
		}
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(
		"POST",
		"/api/ops/storage/flush",
		strings.NewReader(`{"resource":"timeline"}`),
	)
	h.flushStorage(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("flushStorage status = %d, want 200", w.Code)
	}
	flushBody := jsonBody(t, w)
	flushData, _ := flushBody["data"].(map[string]any)
	flushResults, _ := flushData["results"].([]any)
	if len(flushResults) != 1 {
		t.Fatalf("flush results len = %d, want 1", len(flushResults))
	}
	firstResult, _ := flushResults[0].(map[string]any)
	if firstResult["resource"] != store.StorageResourceTimeline {
		t.Fatalf("flushed resource = %v, want timeline", firstResult["resource"])
	}
	removedRows, _ := firstResult["removedRows"].(float64)
	if removedRows < 1 {
		t.Fatalf("removedRows = %v, want >= 1", firstResult["removedRows"])
	}

	resources = getResources(t)
	timelineRows, _ := resources[store.StorageResourceTimeline]["rows"].(float64)
	if timelineRows != 0 {
		t.Fatalf("timeline rows after flush = %v, want 0", timelineRows)
	}
	journalRows, _ := resources[store.StorageResourceActivityLog]["rows"].(float64)
	if journalRows < 1 {
		t.Fatalf("journal rows after timeline flush = %v, want >= 1", journalRows)
	}
}

func seedStorageHandlersData(t *testing.T, st *store.Store, ctx context.Context, base time.Time) {
	t.Helper()
	if _, err := st.InsertWatchtowerTimelineEvent(ctx, store.WatchtowerTimelineEventWrite{
		Session:   "dev",
		WindowIdx: 0,
		PaneID:    "%1",
		EventType: "output.marker",
		Severity:  "warn",
		Summary:   "warning marker",
		Details:   "deprecated warning",
		CreatedAt: base,
	}); err != nil {
		t.Fatalf("InsertWatchtowerTimelineEvent: %v", err)
	}
	if _, err := st.InsertWatchtowerJournal(ctx, store.WatchtowerJournalWrite{
		GlobalRev:  1,
		EntityType: "pane",
		Session:    "dev",
		WindowIdx:  0,
		PaneID:     "%1",
		ChangeKind: "updated",
		ChangedAt:  base,
	}); err != nil {
		t.Fatalf("InsertWatchtowerJournal: %v", err)
	}
	if _, err := st.InsertGuardrailAudit(ctx, store.GuardrailAuditWrite{
		RuleID:      "rule.test",
		Decision:    "warn",
		Action:      "session.kill",
		Command:     "tmux kill-session -t dev",
		SessionName: "dev",
		WindowIndex: 0,
		PaneID:      "%1",
		Reason:      "test",
		MetadataRaw: `{"source":"test"}`,
		CreatedAt:   base,
	}); err != nil {
		t.Fatalf("InsertGuardrailAudit: %v", err)
	}
	snapshot, _, err := st.UpsertRecoverySnapshot(ctx, store.RecoverySnapshotWrite{
		SessionName:  "dev",
		BootID:       "boot-1",
		StateHash:    "hash-1",
		CapturedAt:   base,
		ActiveWindow: 0,
		ActivePaneID: "%1",
		Windows:      1,
		Panes:        1,
		PayloadJSON:  `{"windows":[],"panes":[]}`,
	})
	if err != nil {
		t.Fatalf("UpsertRecoverySnapshot: %v", err)
	}
	if err := st.CreateRecoveryJob(ctx, store.RecoveryJob{
		ID:             "job-1",
		SessionName:    "dev",
		TargetSession:  "dev-restored",
		SnapshotID:     snapshot.ID,
		Mode:           "safe",
		ConflictPolicy: "rename",
		Status:         store.RecoveryJobQueued,
		CreatedAt:      base,
	}); err != nil {
		t.Fatalf("CreateRecoveryJob: %v", err)
	}
}

func TestFlushStorageRejectsInvalidResource(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(
		"POST",
		"/api/ops/storage/flush",
		strings.NewReader(`{"resource":"unknown"}`),
	)

	h.flushStorage(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	body := jsonBody(t, w)
	if got := errCode(body); got != invalidRequestCode {
		t.Fatalf("error code = %q, want %q", got, invalidRequestCode)
	}
}

func TestDeleteSessionGuardrailConfirmRequired(t *testing.T) {
	t.Parallel()

	var killCalls int
	h, st := newTestHandler(t, &mockTmux{
		killSessionFn: func(_ context.Context, session string) error {
			killCalls++
			if session != "dev" {
				t.Fatalf("kill session = %q, want dev", session)
			}
			return nil
		},
	}, nil)
	h.guardrails = guardrails.New(st)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/api/tmux/sessions/dev", nil)
	r.SetPathValue("session", "dev")
	h.deleteSession(w, r)
	if w.Code != http.StatusPreconditionRequired {
		t.Fatalf("status = %d, want 428", w.Code)
	}
	if code := errCode(jsonBody(t, w)); code != "GUARDRAIL_CONFIRM_REQUIRED" {
		t.Fatalf("code = %q, want GUARDRAIL_CONFIRM_REQUIRED", code)
	}
	if killCalls != 0 {
		t.Fatalf("killCalls = %d, want 0", killCalls)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest("DELETE", "/api/tmux/sessions/dev", nil)
	r.SetPathValue("session", "dev")
	r.Header.Set("X-Sentinel-Guardrail-Confirm", "true")
	h.deleteSession(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if killCalls != 1 {
		t.Fatalf("killCalls = %d, want 1", killCalls)
	}
}

func TestGuardrailEndpoints(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	h.guardrails = guardrails.New(st)

	t.Run("list rules", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/guardrails/rules", nil)
		h.listGuardrailRules(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		rules, _ := data["rules"].([]any)
		if len(rules) < 2 {
			t.Fatalf("rules len = %d, want >= 2", len(rules))
		}
	})

	t.Run("evaluate action", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/ops/guardrails/evaluate", strings.NewReader(`{
			"action":"pane.kill",
			"sessionName":"dev",
			"paneId":"%1",
			"windowIndex":0
		}`))
		h.evaluateGuardrail(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		decision, _ := data["decision"].(map[string]any)
		if decision["mode"] != "warn" {
			t.Fatalf("decision.mode = %v, want warn", decision["mode"])
		}
	})

	t.Run("update rule", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest("PATCH", "/api/ops/guardrails/rules/action.session.kill.confirm", strings.NewReader(`{
			"name":"Confirm session kill",
			"scope":"action",
			"pattern":"^session\\.kill$",
			"mode":"confirm",
			"severity":"warn",
			"message":"confirm required",
			"enabled":true,
			"priority":8
		}`))
		r.SetPathValue("rule", "action.session.kill.confirm")
		h.updateGuardrailRule(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})
}

func TestMarkSessionSeenHandler(t *testing.T) {
	t.Parallel()
	const sessionName = "dev"
	h, st, eventsCh := seededMarkSessionSeenHandler(t, sessionName)
	ctx := context.Background()

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
	data, _ := jsonBody(t, w)["data"].(map[string]any)
	assertMarkSeenResponsePayload(t, data, sessionName)
	assertMarkSeenStoreState(t, st, ctx, sessionName)
	sessionsEvent := expectMarkSeenEvents(t, eventsCh)
	assertMarkSeenEventPayload(t, sessionsEvent, sessionName)
}

func seededMarkSessionSeenHandler(t *testing.T, sessionName string) (*Handler, *store.Store, <-chan events.Event) {
	t.Helper()
	h, st := newTestHandler(t, nil, nil)
	hub := events.NewHub()
	eventsCh, unsubscribe := hub.Subscribe(8)
	t.Cleanup(unsubscribe)
	h.events = hub
	now := time.Now().UTC().Truncate(time.Second)
	ctx := context.Background()
	if err := st.UpsertWatchtowerSession(ctx, store.WatchtowerSessionWrite{
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
	if err := st.UpsertWatchtowerWindow(ctx, store.WatchtowerWindowWrite{
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
	if err := st.UpsertWatchtowerPane(ctx, store.WatchtowerPaneWrite{
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
	return h, st, eventsCh
}

func assertMarkSeenResponsePayload(t *testing.T, data map[string]any, sessionName string) {
	t.Helper()
	if data["acked"] != true {
		t.Fatalf("acked = %v, want true", data["acked"])
	}
	rawPatches, ok := data["sessionPatches"].([]any)
	if !ok || len(rawPatches) != 1 {
		t.Fatalf("sessionPatches = %T(%v), want len=1", data["sessionPatches"], data["sessionPatches"])
	}
	patch, _ := rawPatches[0].(map[string]any)
	if patch["name"] != sessionName || patch["unreadPanes"] != float64(0) {
		t.Fatalf("unexpected session patch: %+v", patch)
	}
	rawInspector, ok := data["inspectorPatches"].([]any)
	if !ok || len(rawInspector) != 1 {
		t.Fatalf("inspectorPatches = %T(%v), want len=1", data["inspectorPatches"], data["inspectorPatches"])
	}
	inspector, _ := rawInspector[0].(map[string]any)
	if inspector["session"] != sessionName {
		t.Fatalf("inspector session = %v, want %s", inspector["session"], sessionName)
	}
	if windows, ok := inspector["windows"].([]any); !ok || len(windows) != 1 {
		t.Fatalf("inspector windows = %T(%v), want len=1", inspector["windows"], inspector["windows"])
	}
	if panesRaw, ok := inspector["panes"].([]any); !ok || len(panesRaw) != 1 {
		t.Fatalf("inspector panes = %T(%v), want len=1", inspector["panes"], inspector["panes"])
	}
}

func assertMarkSeenStoreState(t *testing.T, st *store.Store, ctx context.Context, sessionName string) {
	t.Helper()
	panes, err := st.ListWatchtowerPanes(ctx, sessionName)
	if err != nil {
		t.Fatalf("ListWatchtowerPanes(%s): %v", sessionName, err)
	}
	if len(panes) != 1 || panes[0].SeenRevision != panes[0].Revision {
		t.Fatalf("unexpected pane seen state: %+v", panes)
	}
	session, err := st.GetWatchtowerSession(ctx, sessionName)
	if err != nil {
		t.Fatalf("GetWatchtowerSession(%s): %v", sessionName, err)
	}
	if session.UnreadPanes != 0 || session.UnreadWindows != 0 {
		t.Fatalf("unexpected unread counters after seen: %+v", session)
	}
}

func expectMarkSeenEvents(t *testing.T, eventsCh <-chan events.Event) events.Event {
	t.Helper()
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
	return sessionsEvent
}

func assertMarkSeenEventPayload(t *testing.T, sessionsEvent events.Event, sessionName string) {
	t.Helper()
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

	h, _ := newTestHandler(t, nil, nil)
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

		h, st := newTestHandler(t, nil, nil)
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

		presence, err := st.ListWatchtowerPresenceBySession(context.Background(), "dev")
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

		h, _ := newTestHandler(t, nil, nil)
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

// ---------------------------------------------------------------------------
// Custom service registration / unregistration tests
// ---------------------------------------------------------------------------

func TestRegisterAndUnregisterOpsService(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)

	// Register a custom service.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/ops/services", strings.NewReader(`{
		"name":"myapp",
		"displayName":"My App",
		"manager":"systemd",
		"unit":"myapp.service",
		"scope":"user"
	}`))
	h.registerOpsService(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	services, _ := data["services"].([]any)
	// The mock ListServices returns empty, so services array may be empty;
	// verify the store persisted the record.
	_ = services
	custom, err := st.ListCustomServices(r.Context())
	if err != nil {
		t.Fatalf("ListCustomServices: %v", err)
	}
	if len(custom) != 1 || custom[0].Name != "myapp" {
		t.Fatalf("custom services = %+v, want 1 entry named myapp", custom)
	}

	// Duplicate registration should conflict.
	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/api/ops/services", strings.NewReader(`{
		"name":"myapp",
		"displayName":"My App",
		"manager":"systemd",
		"unit":"myapp.service",
		"scope":"user"
	}`))
	h.registerOpsService(w, r)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate register status = %d, want 409", w.Code)
	}

	// Unregister the service.
	w = httptest.NewRecorder()
	r = httptest.NewRequest("DELETE", "/api/ops/services/myapp", nil)
	r.SetPathValue("service", "myapp")
	h.unregisterOpsService(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("unregister status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	body = jsonBody(t, w)
	data, _ = body["data"].(map[string]any)
	if data["removed"] != "myapp" {
		t.Fatalf("removed = %v, want myapp", data["removed"])
	}

	// Unregister again should 404.
	w = httptest.NewRecorder()
	r = httptest.NewRequest("DELETE", "/api/ops/services/myapp", nil)
	r.SetPathValue("service", "myapp")
	h.unregisterOpsService(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("double-unregister status = %d, want 404", w.Code)
	}
}

func TestRegisterOpsServiceValidation(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)

	// Missing name.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/ops/services", strings.NewReader(`{
		"unit":"myapp.service"
	}`))
	h.registerOpsService(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}

	// Missing unit.
	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/api/ops/services", strings.NewReader(`{
		"name":"myapp"
	}`))
	h.registerOpsService(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Service logs handler tests
// ---------------------------------------------------------------------------

func TestOpsServiceLogsHandler(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.ops = &mockOpsControlPlane{
		logsFn: func(_ context.Context, name string, lines int) (string, error) {
			if name != "sentinel" {
				t.Fatalf("service = %q, want sentinel", name)
			}
			if lines != 50 {
				t.Fatalf("lines = %d, want 50", lines)
			}
			return "line1\nline2\nline3", nil
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/services/sentinel/logs?lines=50", nil)
	r.SetPathValue("service", "sentinel")
	h.opsServiceLogs(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	if data["output"] != "line1\nline2\nline3" {
		t.Fatalf("output = %v, want log lines", data["output"])
	}
}

func TestOpsServiceLogsNotFound(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.ops = &mockOpsControlPlane{
		logsFn: func(context.Context, string, int) (string, error) {
			return "", opsplane.ErrServiceNotFound
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/services/missing/logs", nil)
	r.SetPathValue("service", "missing")
	h.opsServiceLogs(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Metrics handler tests
// ---------------------------------------------------------------------------

func TestOpsMetricsHandler(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.ops = &mockOpsControlPlane{
		metricsFn: func(context.Context) opsplane.HostMetrics {
			return opsplane.HostMetrics{
				CPUPercent:    42.5,
				MemPercent:    65.2,
				DiskPercent:   78.0,
				LoadAvg1:      1.5,
				NumGoroutines: 120,
			}
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/metrics", nil)
	h.opsMetrics(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	metrics, _ := data["metrics"].(map[string]any)
	if metrics["cpuPercent"] != 42.5 {
		t.Fatalf("cpuPercent = %v, want 42.5", metrics["cpuPercent"])
	}
}

// ---------------------------------------------------------------------------
// Browse + unit-based handler tests
// ---------------------------------------------------------------------------

const testNginxUnit = "nginx.service"

func TestBrowseOpsServicesHandler(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.ops = &mockOpsControlPlane{
		browseFn: func(context.Context) ([]opsplane.BrowsedService, error) {
			return []opsplane.BrowsedService{
				{Unit: testNginxUnit, ActiveState: "active", Manager: "systemd", Scope: "system", Tracked: false},
				{Unit: "sentinel", ActiveState: "active", Manager: "systemd", Scope: "user", Tracked: true, TrackedName: "sentinel"},
			}, nil
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/services/browse", nil)
	h.browseOpsServices(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	services, _ := data["services"].([]any)
	if len(services) != 2 {
		t.Fatalf("len(services) = %d, want 2", len(services))
	}
	first, _ := services[0].(map[string]any)
	if first["tracked"] != false {
		t.Fatalf("first service tracked = %v, want false", first["tracked"])
	}
	second, _ := services[1].(map[string]any)
	if second["tracked"] != true {
		t.Fatalf("second service tracked = %v, want true", second["tracked"])
	}
	if second["trackedName"] != "sentinel" {
		t.Fatalf("trackedName = %v, want sentinel", second["trackedName"])
	}
}

func TestOpsUnitActionHandler(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.ops = &mockOpsControlPlane{
		actByUnitFn: func(_ context.Context, unit, scope, manager, action string) error {
			if unit != testNginxUnit {
				t.Fatalf("unit = %q, want %s", unit, testNginxUnit)
			}
			if action != "restart" {
				t.Fatalf("action = %q, want restart", action)
			}
			return nil
		},
		overviewFn: func(context.Context) (opsplane.Overview, error) {
			return opsplane.Overview{Host: opsplane.HostOverview{Hostname: "test"}}, nil
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/ops/services/unit/action",
		strings.NewReader(`{"unit":"`+testNginxUnit+`","scope":"system","manager":"systemd","action":"restart"}`))
	r.Header.Set("Content-Type", "application/json")
	h.opsUnitAction(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
}

func TestOpsUnitActionHandlerMissingUnit(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.ops = &mockOpsControlPlane{}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/ops/services/unit/action",
		strings.NewReader(`{"scope":"system","manager":"systemd","action":"restart"}`))
	r.Header.Set("Content-Type", "application/json")
	h.opsUnitAction(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
}

func TestOpsUnitStatusHandler(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.ops = &mockOpsControlPlane{
		inspectByUnitFn: func(_ context.Context, unit, scope, manager string) (opsplane.ServiceInspect, error) {
			if unit != testNginxUnit {
				t.Fatalf("unit = %q, want %s", unit, testNginxUnit)
			}
			return opsplane.ServiceInspect{
				Service: opsplane.ServiceStatus{Unit: unit, Scope: scope, Manager: manager},
				Summary: "load=loaded active=active sub=running",
			}, nil
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/services/unit/status?unit="+testNginxUnit+"&scope=system&manager=systemd", nil)
	h.opsUnitStatus(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	status, _ := data["status"].(map[string]any)
	if status["summary"] != "load=loaded active=active sub=running" {
		t.Fatalf("summary = %v, want load=loaded...", status["summary"])
	}
}

func TestOpsUnitLogsHandler(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.ops = &mockOpsControlPlane{
		logsByUnitFn: func(_ context.Context, unit, scope, manager string, lines int) (string, error) {
			if unit != testNginxUnit {
				t.Fatalf("unit = %q, want %s", unit, testNginxUnit)
			}
			if lines != 50 {
				t.Fatalf("lines = %d, want 50", lines)
			}
			return "log line 1\nlog line 2", nil
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/services/unit/logs?unit="+testNginxUnit+"&scope=system&manager=systemd&lines=50", nil)
	h.opsUnitLogs(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	if data["output"] != "log line 1\nlog line 2" {
		t.Fatalf("output = %v", data["output"])
	}
}

// ---------------------------------------------------------------------------
// Config handler tests
// ---------------------------------------------------------------------------

func TestOpsConfigHandler(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte("[server]\nport = 4040\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	h.configPath = configPath

	// GET config.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/config", nil)
	h.opsConfig(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("GET config status = %d, want 200", w.Code)
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	if data["content"] != "[server]\nport = 4040\n" {
		t.Fatalf("content = %q, want config content", data["content"])
	}

	// PATCH config.
	w = httptest.NewRecorder()
	r = httptest.NewRequest("PATCH", "/api/ops/config", strings.NewReader(`{"content":"[server]\nport = 5050\n"}`))
	h.patchOpsConfig(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH config status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	// Verify written content.
	got, err := os.ReadFile(configPath) //nolint:gosec // test file with known temp path
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(got) != "[server]\nport = 5050\n" {
		t.Fatalf("config file content = %q, want updated content", string(got))
	}
}

func TestOpsConfigNoPath(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	// configPath is empty by default.

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/config", nil)
	h.opsConfig(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestOpsPatchConfigValidation(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.configPath = filepath.Join(t.TempDir(), "config.toml")

	// Empty content should fail.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PATCH", "/api/ops/config", strings.NewReader(`{"content":""}`))
	h.patchOpsConfig(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Runbook CRUD handler tests
// ---------------------------------------------------------------------------

func TestCreateUpdateDeleteOpsRunbook(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	ctx := context.Background()
	_ = ctx

	// Create runbook.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/ops/runbooks", strings.NewReader(`{
		"name":"deploy",
		"description":"Deploy the app",
		"steps":[{"type":"command","title":"Build","command":"make build"}],
		"enabled":true
	}`))
	h.createOpsRunbook(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	rb, _ := data["runbook"].(map[string]any)
	rbID, _ := rb["id"].(string)
	if rbID == "" {
		t.Fatal("runbook id should not be empty")
	}
	if rb["name"] != "deploy" {
		t.Fatalf("name = %v, want deploy", rb["name"])
	}

	// Update runbook.
	w = httptest.NewRecorder()
	r = httptest.NewRequest("PUT", "/api/ops/runbooks/"+rbID, strings.NewReader(`{
		"name":"deploy-v2",
		"description":"Deploy v2",
		"steps":[{"type":"command","title":"Build","command":"make build-all"}],
		"enabled":true
	}`))
	r.SetPathValue("runbook", rbID)
	h.updateOpsRunbook(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	body = jsonBody(t, w)
	data, _ = body["data"].(map[string]any)
	rb, _ = data["runbook"].(map[string]any)
	if rb["name"] != "deploy-v2" {
		t.Fatalf("updated name = %v, want deploy-v2", rb["name"])
	}

	// Delete runbook.
	w = httptest.NewRecorder()
	r = httptest.NewRequest("DELETE", "/api/ops/runbooks/"+rbID, nil)
	r.SetPathValue("runbook", rbID)
	h.deleteOpsRunbook(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	// Delete again should 404.
	w = httptest.NewRecorder()
	r = httptest.NewRequest("DELETE", "/api/ops/runbooks/"+rbID, nil)
	r.SetPathValue("runbook", rbID)
	h.deleteOpsRunbook(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("double-delete status = %d, want 404", w.Code)
	}
}

func TestCreateOpsRunbookValidation(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)

	// Missing name.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/ops/runbooks", strings.NewReader(`{
		"description":"no name"
	}`))
	h.createOpsRunbook(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestUpdateOpsRunbookNotFound(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/ops/runbooks/nonexistent", strings.NewReader(`{
		"name":"test"
	}`))
	r.SetPathValue("runbook", "nonexistent")
	h.updateOpsRunbook(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestTriggerScheduleFinalisesState(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	h.events = events.NewHub()
	ctx := context.Background()

	// Create a runbook with a simple echo step.
	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		Name: "trigger-test-rb",
		Steps: []store.OpsRunbookStep{
			{Type: "command", Title: "echo", Command: "echo ok"},
		},
	})
	if err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}

	// Create a cron schedule with a valid next_run_at.
	future := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
	sched, err := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
		RunbookID:    rb.ID,
		Name:         "trigger-test-sched",
		ScheduleType: "cron",
		CronExpr:     "0 * * * *",
		Timezone:     "UTC",
		Enabled:      true,
		NextRunAt:    future,
	})
	if err != nil {
		t.Fatalf("InsertOpsSchedule: %v", err)
	}

	// Trigger the schedule.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/ops/schedules/"+sched.ID+"/trigger", nil)
	r.SetPathValue("schedule", sched.ID)
	h.triggerSchedule(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("triggerSchedule status = %d, want 202; body=%s", w.Code, w.Body.String())
	}

	// Wait for async run to complete.
	h.wg.Wait()

	// Reload schedule and verify state is terminal.
	schedules, err := st.ListOpsSchedules(ctx)
	if err != nil {
		t.Fatalf("ListOpsSchedules: %v", err)
	}
	var got *store.OpsSchedule
	for i := range schedules {
		if schedules[i].ID == sched.ID {
			got = &schedules[i]
			break
		}
	}
	if got == nil {
		t.Fatal("schedule not found after trigger")
	}
	if got.LastRunStatus == "running" {
		t.Fatalf("schedule stuck in running; expected terminal status")
	}
	if got.LastRunStatus != "succeeded" && got.LastRunStatus != "failed" {
		t.Fatalf("schedule last_run_status = %q, want succeeded or failed", got.LastRunStatus)
	}
	if !got.Enabled {
		t.Fatal("cron schedule should remain enabled after manual trigger")
	}
	// Manual trigger recomputes next_run_at via cron.Next(now) — it must
	// differ from the original future value and point to the next hour boundary.
	if got.NextRunAt == future {
		t.Fatal("cron schedule next_run_at should be recomputed, not preserved")
	}
	recomputed, err := time.Parse(time.RFC3339, got.NextRunAt)
	if err != nil {
		t.Fatalf("next_run_at is not valid RFC3339: %q", got.NextRunAt)
	}
	if !recomputed.After(time.Now().UTC()) {
		t.Fatalf("recomputed next_run_at %s should be in the future", got.NextRunAt)
	}
}

func TestTriggerOnceScheduleDisabledAfterManualRun(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	h.events = events.NewHub()
	ctx := context.Background()

	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		Name: "once-trigger-rb",
		Steps: []store.OpsRunbookStep{
			{Type: "command", Title: "echo", Command: "echo ok"},
		},
	})
	if err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}

	future := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
	sched, err := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
		RunbookID:    rb.ID,
		Name:         "once-trigger-sched",
		ScheduleType: "once",
		RunAt:        future,
		Enabled:      false,
		NextRunAt:    future,
	})
	if err != nil {
		t.Fatalf("InsertOpsSchedule: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/ops/schedules/"+sched.ID+"/trigger", nil)
	r.SetPathValue("schedule", sched.ID)
	h.triggerSchedule(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("triggerSchedule status = %d, want 202; body=%s", w.Code, w.Body.String())
	}

	h.wg.Wait()

	schedules, err := st.ListOpsSchedules(ctx)
	if err != nil {
		t.Fatalf("ListOpsSchedules: %v", err)
	}
	var got *store.OpsSchedule
	for i := range schedules {
		if schedules[i].ID == sched.ID {
			got = &schedules[i]
			break
		}
	}
	if got == nil {
		t.Fatal("schedule not found after trigger")
	}
	if got.LastRunStatus == "running" {
		t.Fatalf("once schedule stuck in running; expected terminal status")
	}
}

func TestDeleteRunbookCascadesSchedules(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	h.events = events.NewHub()
	ctx := context.Background()

	// Create runbook.
	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		Name:  "cascade-rb",
		Steps: []store.OpsRunbookStep{{Type: "command", Title: "echo", Command: "echo ok"}},
	})
	if err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}

	// Create two schedules for this runbook.
	future := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
	for _, name := range []string{"sched-a", "sched-b"} {
		_, err := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
			RunbookID:    rb.ID,
			Name:         name,
			ScheduleType: "cron",
			CronExpr:     "0 * * * *",
			Timezone:     "UTC",
			Enabled:      true,
			NextRunAt:    future,
		})
		if err != nil {
			t.Fatalf("InsertOpsSchedule(%s): %v", name, err)
		}
	}

	// Delete the runbook.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/api/ops/runbooks/"+rb.ID, nil)
	r.SetPathValue("runbook", rb.ID)
	h.deleteOpsRunbook(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	// Verify schedules were removed.
	schedules, err := st.ListOpsSchedules(ctx)
	if err != nil {
		t.Fatalf("ListOpsSchedules: %v", err)
	}
	for _, s := range schedules {
		if s.RunbookID == rb.ID {
			t.Fatalf("orphan schedule %s still exists after runbook delete", s.ID)
		}
	}
}

func TestCreateRunbookRejectsInvalidStepType(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)

	tests := []struct {
		name string
		body string
	}{
		{"invalid type", `{"name":"bad","steps":[{"type":"shell","title":"Build","command":"make"}]}`},
		{"empty type", `{"name":"bad","steps":[{"type":"","title":"Build","command":"make"}]}`},
		{"missing title", `{"name":"bad","steps":[{"type":"command","title":"","command":"make"}]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/api/ops/runbooks", strings.NewReader(tc.body))
			h.createOpsRunbook(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestTriggerCronScheduleRecomputesNextRunAt(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	h.events = events.NewHub()
	ctx := context.Background()

	rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
		Name:  "recompute-rb",
		Steps: []store.OpsRunbookStep{{Type: "command", Title: "echo", Command: "echo ok"}},
	})
	if err != nil {
		t.Fatalf("InsertOpsRunbook: %v", err)
	}

	// Create a cron schedule with a stale (past) next_run_at.
	stale := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	sched, err := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
		RunbookID:    rb.ID,
		Name:         "recompute-sched",
		ScheduleType: "cron",
		CronExpr:     "0 * * * *",
		Timezone:     "UTC",
		Enabled:      true,
		NextRunAt:    stale,
	})
	if err != nil {
		t.Fatalf("InsertOpsSchedule: %v", err)
	}

	// Trigger manually.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/ops/schedules/"+sched.ID+"/trigger", nil)
	r.SetPathValue("schedule", sched.ID)
	h.triggerSchedule(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("triggerSchedule status = %d, want 202; body=%s", w.Code, w.Body.String())
	}

	h.wg.Wait()

	// Verify next_run_at was recomputed to the future (not the stale value).
	schedules, err := st.ListOpsSchedules(ctx)
	if err != nil {
		t.Fatalf("ListOpsSchedules: %v", err)
	}
	var got *store.OpsSchedule
	for i := range schedules {
		if schedules[i].ID == sched.ID {
			got = &schedules[i]
			break
		}
	}
	if got == nil {
		t.Fatal("schedule not found")
	}
	if got.NextRunAt == stale {
		t.Fatal("next_run_at was not recomputed; still has stale value")
	}
	nextRun, err := time.Parse(time.RFC3339, got.NextRunAt)
	if err != nil {
		t.Fatalf("invalid next_run_at: %v", err)
	}
	if !nextRun.After(time.Now().UTC()) {
		t.Fatalf("next_run_at = %s, expected future timestamp", got.NextRunAt)
	}
	if !got.Enabled {
		t.Fatal("cron schedule should remain enabled after manual trigger")
	}
}

func TestGuardrailFailClosedOnEvaluateError(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	h.guardrails = guardrails.New(st)

	// Close the store to force Evaluate to fail.
	_ = st.Close()

	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/api/tmux/sessions/dev", nil)
	r.SetPathValue("session", "dev")

	allowed := h.enforceGuardrail(w, r, guardrails.Input{
		Action:      "kill-session",
		SessionName: "dev",
	})
	if allowed {
		t.Fatal("expected guardrail to block when Evaluate fails, but it allowed")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestCreateGuardrailRule(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	h.guardrails = guardrails.New(st)

	// Create a rule with explicit ID.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/ops/guardrails/rules", strings.NewReader(`{
		"id": "test.create.rule",
		"name": "Test Rule",
		"scope": "action",
		"pattern": "^session\\.create$",
		"mode": "block",
		"severity": "error",
		"message": "Blocked session creation",
		"enabled": true,
		"priority": 5
	}`))
	r.Header.Set("Content-Type", "application/json")
	h.createGuardrailRule(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	body := jsonBody(t, w)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data in response")
	}
	rules, ok := data["rules"].([]any)
	if !ok {
		t.Fatal("expected rules array in response")
	}
	found := false
	for _, rule := range rules {
		rMap, _ := rule.(map[string]any)
		if rMap["id"] == "test.create.rule" {
			found = true
			if rMap["name"] != "Test Rule" {
				t.Errorf("name = %v, want Test Rule", rMap["name"])
			}
		}
	}
	if !found {
		t.Fatal("created rule not found in response list")
	}
}

func TestCreateGuardrailRuleAutoID(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	h.guardrails = guardrails.New(st)

	// Create a rule without ID — should auto-generate.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/ops/guardrails/rules", strings.NewReader(`{
		"pattern": "^pane\\.split$",
		"enabled": true
	}`))
	r.Header.Set("Content-Type", "application/json")
	h.createGuardrailRule(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
}

func TestCreateGuardrailRuleValidation(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	h.guardrails = guardrails.New(st)

	tests := []struct {
		name string
		body string
	}{
		{name: "missing pattern", body: `{"enabled": true}`},
		{name: "missing enabled", body: `{"pattern": "^test$"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/api/ops/guardrails/rules", strings.NewReader(tt.body))
			r.Header.Set("Content-Type", "application/json")
			h.createGuardrailRule(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestDeleteGuardrailRule(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	h.guardrails = guardrails.New(st)
	ctx := context.Background()

	// Create a rule first.
	err := st.UpsertGuardrailRule(ctx, store.GuardrailRuleWrite{
		ID:      "test.delete.rule",
		Name:    "Delete Me",
		Pattern: "^test$",
		Mode:    "warn",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("UpsertGuardrailRule: %v", err)
	}

	// Delete it.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/api/ops/guardrails/rules/test.delete.rule", nil)
	r.SetPathValue("rule", "test.delete.rule")
	h.deleteGuardrailRule(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := jsonBody(t, w)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data in response")
	}
	if data["removed"] != "test.delete.rule" {
		t.Errorf("removed = %v, want test.delete.rule", data["removed"])
	}

	// Verify it's gone.
	rules, err := st.ListGuardrailRules(ctx)
	if err != nil {
		t.Fatalf("ListGuardrailRules: %v", err)
	}
	for _, rule := range rules {
		if rule.ID == "test.delete.rule" {
			t.Fatal("rule should have been deleted but still exists")
		}
	}
}

func TestDeleteGuardrailRuleNotFound(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil, nil)
	h.guardrails = guardrails.New(st)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/api/ops/guardrails/rules/nonexistent", nil)
	r.SetPathValue("rule", "nonexistent")
	h.deleteGuardrailRule(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Schedule handler tests
// ---------------------------------------------------------------------------

func TestListSchedulesHandler(t *testing.T) {
	t.Parallel()

	t.Run("empty list", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/schedules", nil)
		h.listSchedules(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		schedules, _ := data["schedules"].([]any)
		if len(schedules) != 0 {
			t.Fatalf("schedules len = %d, want 0", len(schedules))
		}
	})

	t.Run("with schedules", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, nil, nil)
		ctx := context.Background()
		rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
			Name:  "list-sched-rb",
			Steps: []store.OpsRunbookStep{{Type: "command", Title: "echo", Command: "echo ok"}},
		})
		if err != nil {
			t.Fatalf("InsertOpsRunbook: %v", err)
		}
		future := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
		if _, err := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
			RunbookID: rb.ID, Name: "sched-1", ScheduleType: "cron",
			CronExpr: "0 * * * *", Timezone: "UTC", Enabled: true, NextRunAt: future,
		}); err != nil {
			t.Fatalf("InsertOpsSchedule: %v", err)
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/schedules", nil)
		h.listSchedules(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		schedules, _ := data["schedules"].([]any)
		if len(schedules) != 1 {
			t.Fatalf("schedules len = %d, want 1", len(schedules))
		}
	})

	t.Run("nil repo", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.repo = nil
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/schedules", nil)
		h.listSchedules(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})
}

func TestCreateScheduleHandler(t *testing.T) {
	t.Parallel()

	t.Run("cron schedule", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, nil, nil)
		h.events = events.NewHub()
		ctx := context.Background()
		rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
			Name:  "create-sched-rb",
			Steps: []store.OpsRunbookStep{{Type: "command", Title: "echo", Command: "echo ok"}},
		})
		if err != nil {
			t.Fatalf("InsertOpsRunbook: %v", err)
		}

		body := fmt.Sprintf(`{"runbookId":"%s","name":"my-cron","scheduleType":"cron","cronExpr":"0 * * * *","timezone":"UTC","enabled":true}`, rb.ID)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/ops/schedules", strings.NewReader(body))
		h.createSchedule(w, r)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
		}
		resp := jsonBody(t, w)
		data, _ := resp["data"].(map[string]any)
		sched, _ := data["schedule"].(map[string]any)
		if sched["name"] != "my-cron" {
			t.Fatalf("name = %v, want my-cron", sched["name"])
		}
	})

	t.Run("once schedule", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, nil, nil)
		h.events = events.NewHub()
		ctx := context.Background()
		rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
			Name:  "create-once-rb",
			Steps: []store.OpsRunbookStep{{Type: "command", Title: "echo", Command: "echo ok"}},
		})
		if err != nil {
			t.Fatalf("InsertOpsRunbook: %v", err)
		}

		future := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
		body := fmt.Sprintf(`{"runbookId":"%s","name":"my-once","scheduleType":"once","runAt":"%s","enabled":true}`, rb.ID, future)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/ops/schedules", strings.NewReader(body))
		h.createSchedule(w, r)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("validation errors", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		tests := []struct {
			name string
			body string
		}{
			{"missing runbookId", `{"name":"x","scheduleType":"cron","cronExpr":"0 * * * *"}`},
			{"missing name", `{"runbookId":"x","scheduleType":"cron","cronExpr":"0 * * * *"}`},
			{"invalid scheduleType", `{"runbookId":"x","name":"x","scheduleType":"bad"}`},
			{"invalid json", `{not-json}`},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				w := httptest.NewRecorder()
				r := httptest.NewRequest("POST", "/api/ops/schedules", strings.NewReader(tt.body))
				h.createSchedule(w, r)

				if w.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
				}
			})
		}
	})

	t.Run("nil repo", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.repo = nil
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/ops/schedules", strings.NewReader(`{"runbookId":"x","name":"x","scheduleType":"cron","cronExpr":"0 * * * *"}`))
		h.createSchedule(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})
}

func TestUpdateScheduleHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, nil, nil)
		h.events = events.NewHub()
		ctx := context.Background()
		rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
			Name:  "update-sched-rb",
			Steps: []store.OpsRunbookStep{{Type: "command", Title: "echo", Command: "echo ok"}},
		})
		if err != nil {
			t.Fatalf("InsertOpsRunbook: %v", err)
		}
		future := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
		sched, err := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
			RunbookID: rb.ID, Name: "orig", ScheduleType: "cron",
			CronExpr: "0 * * * *", Timezone: "UTC", Enabled: true, NextRunAt: future,
		})
		if err != nil {
			t.Fatalf("InsertOpsSchedule: %v", err)
		}

		body := fmt.Sprintf(`{"runbookId":"%s","name":"updated","scheduleType":"cron","cronExpr":"30 * * * *","timezone":"UTC","enabled":true}`, rb.ID)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/api/ops/schedules/"+sched.ID, strings.NewReader(body))
		r.SetPathValue("schedule", sched.ID)
		h.updateSchedule(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		resp := jsonBody(t, w)
		data, _ := resp["data"].(map[string]any)
		updated, _ := data["schedule"].(map[string]any)
		if updated["name"] != "updated" {
			t.Fatalf("name = %v, want updated", updated["name"])
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, nil, nil)
		ctx := context.Background()
		rb, _ := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
			Name:  "upd-nf-rb",
			Steps: []store.OpsRunbookStep{{Type: "command", Title: "echo", Command: "echo ok"}},
		})
		body := fmt.Sprintf(`{"runbookId":"%s","name":"x","scheduleType":"cron","cronExpr":"0 * * * *","timezone":"UTC","enabled":true}`, rb.ID)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/api/ops/schedules/nonexistent", strings.NewReader(body))
		r.SetPathValue("schedule", "nonexistent")
		h.updateSchedule(w, r)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing schedule id", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/api/ops/schedules/", strings.NewReader(`{"runbookId":"x","name":"x","scheduleType":"cron","cronExpr":"0 * * * *"}`))
		r.SetPathValue("schedule", "")
		h.updateSchedule(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("nil repo", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.repo = nil
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/api/ops/schedules/abc", strings.NewReader(`{}`))
		r.SetPathValue("schedule", "abc")
		h.updateSchedule(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})
}

func TestDeleteScheduleHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, nil, nil)
		h.events = events.NewHub()
		ctx := context.Background()
		rb, _ := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
			Name:  "del-sched-rb",
			Steps: []store.OpsRunbookStep{{Type: "command", Title: "echo", Command: "echo ok"}},
		})
		future := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
		sched, _ := st.InsertOpsSchedule(ctx, store.OpsScheduleWrite{
			RunbookID: rb.ID, Name: "to-delete", ScheduleType: "cron",
			CronExpr: "0 * * * *", Timezone: "UTC", Enabled: true, NextRunAt: future,
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/api/ops/schedules/"+sched.ID, nil)
		r.SetPathValue("schedule", sched.ID)
		h.deleteSchedule(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		resp := jsonBody(t, w)
		data, _ := resp["data"].(map[string]any)
		if data["removed"] != sched.ID {
			t.Fatalf("removed = %v, want %s", data["removed"], sched.ID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/api/ops/schedules/nonexistent", nil)
		r.SetPathValue("schedule", "nonexistent")
		h.deleteSchedule(w, r)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("missing id", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/api/ops/schedules/", nil)
		r.SetPathValue("schedule", "")
		h.deleteSchedule(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("nil repo", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.repo = nil
		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/api/ops/schedules/abc", nil)
		r.SetPathValue("schedule", "abc")
		h.deleteSchedule(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})
}

func TestValidateScheduleRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("runbook not found", func(t *testing.T) {
		t.Parallel()

		st := newTestStore(t)
		_, err := validateScheduleRequest(ctx, st, "nonexistent", "cron", "0 * * * *", "UTC", "")
		if err == nil || !strings.Contains(err.Error(), "runbook not found") {
			t.Fatalf("err = %v, want runbook not found", err)
		}
	})

	t.Run("invalid cron", func(t *testing.T) {
		t.Parallel()

		st := newTestStore(t)
		rb, _ := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
			Name:  "val-cron-rb",
			Steps: []store.OpsRunbookStep{{Type: "command", Title: "echo", Command: "echo ok"}},
		})
		_, err := validateScheduleRequest(ctx, st, rb.ID, "cron", "bad-cron", "UTC", "")
		if err == nil || !strings.Contains(err.Error(), "invalid cron") {
			t.Fatalf("err = %v, want invalid cron", err)
		}
	})

	t.Run("invalid timezone", func(t *testing.T) {
		t.Parallel()

		st := newTestStore(t)
		rb, _ := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
			Name:  "val-tz-rb",
			Steps: []store.OpsRunbookStep{{Type: "command", Title: "echo", Command: "echo ok"}},
		})
		_, err := validateScheduleRequest(ctx, st, rb.ID, "cron", "0 * * * *", "Invalid/Zone", "")
		if err == nil || !strings.Contains(err.Error(), "invalid timezone") {
			t.Fatalf("err = %v, want invalid timezone", err)
		}
	})

	t.Run("once missing runAt", func(t *testing.T) {
		t.Parallel()

		st := newTestStore(t)
		rb, _ := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
			Name:  "val-once-rb",
			Steps: []store.OpsRunbookStep{{Type: "command", Title: "echo", Command: "echo ok"}},
		})
		_, err := validateScheduleRequest(ctx, st, rb.ID, "once", "", "", "not-rfc3339")
		if err == nil || !strings.Contains(err.Error(), "runAt must be a valid RFC3339") {
			t.Fatalf("err = %v, want runAt must be RFC3339", err)
		}
	})

	t.Run("once past runAt", func(t *testing.T) {
		t.Parallel()

		st := newTestStore(t)
		rb, _ := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
			Name:  "val-past-rb",
			Steps: []store.OpsRunbookStep{{Type: "command", Title: "echo", Command: "echo ok"}},
		})
		past := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
		_, err := validateScheduleRequest(ctx, st, rb.ID, "once", "", "", past)
		if err == nil || !strings.Contains(err.Error(), "runAt must be in the future") {
			t.Fatalf("err = %v, want runAt must be in the future", err)
		}
	})

	t.Run("valid cron returns nextRunAt", func(t *testing.T) {
		t.Parallel()

		st := newTestStore(t)
		rb, _ := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
			Name:  "val-ok-rb",
			Steps: []store.OpsRunbookStep{{Type: "command", Title: "echo", Command: "echo ok"}},
		})
		next, err := validateScheduleRequest(ctx, st, rb.ID, "cron", "0 * * * *", "UTC", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		parsed, err := time.Parse(time.RFC3339, next)
		if err != nil {
			t.Fatalf("nextRunAt is not RFC3339: %q", next)
		}
		if !parsed.After(time.Now().UTC()) {
			t.Fatalf("nextRunAt should be in the future: %s", next)
		}
	})

	t.Run("valid once returns nextRunAt", func(t *testing.T) {
		t.Parallel()

		st := newTestStore(t)
		rb, _ := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
			Name:  "val-once-ok-rb",
			Steps: []store.OpsRunbookStep{{Type: "command", Title: "echo", Command: "echo ok"}},
		})
		future := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
		next, err := validateScheduleRequest(ctx, st, rb.ID, "once", "", "", future)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if next == "" {
			t.Fatal("nextRunAt should not be empty")
		}
	})
}

// ---------------------------------------------------------------------------
// Recovery handler tests
// ---------------------------------------------------------------------------

func TestListRecoverySessionsHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		sessions := []store.RecoverySession{
			{Name: "dev", State: "killed"},
		}
		h, _ := newTestHandler(t, nil, nil)
		h.recovery = &mockRecovery{
			listKilledSessionsFn: func(_ context.Context) ([]store.RecoverySession, error) {
				return sessions, nil
			},
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/recovery/sessions", nil)
		h.listRecoverySessions(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		got, _ := data["sessions"].([]any)
		if len(got) != 1 {
			t.Fatalf("sessions len = %d, want 1", len(got))
		}
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.recovery = &mockRecovery{
			listKilledSessionsFn: func(_ context.Context) ([]store.RecoverySession, error) {
				return nil, fmt.Errorf("db error")
			},
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/recovery/sessions", nil)
		h.listRecoverySessions(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})

	t.Run("recovery disabled", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.recovery = nil

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/recovery/sessions", nil)
		h.listRecoverySessions(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})
}

func TestArchiveRecoverySessionHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.events = events.NewHub()
		h.recovery = &mockRecovery{}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/recovery/sessions/dev/archive", nil)
		r.SetPathValue("session", "dev")
		h.archiveRecoverySession(w, r)

		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid session name", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.recovery = &mockRecovery{}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/recovery/sessions/inv@lid/archive", nil)
		r.SetPathValue("session", "inv@lid")
		h.archiveRecoverySession(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("archive error", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.recovery = &mockRecovery{
			archiveSessionFn: func(_ context.Context, _ string) error {
				return fmt.Errorf("archive fail")
			},
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/recovery/sessions/dev/archive", nil)
		r.SetPathValue("session", "dev")
		h.archiveRecoverySession(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})

	t.Run("recovery disabled", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.recovery = nil

		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/recovery/sessions/dev/archive", nil)
		r.SetPathValue("session", "dev")
		h.archiveRecoverySession(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})
}

func TestListRecoverySnapshotsHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.recovery = &mockRecovery{
			listSnapshotsFn: func(_ context.Context, _ string, _ int) ([]store.RecoverySnapshot, error) {
				return []store.RecoverySnapshot{{ID: 1, SessionName: "dev"}}, nil
			},
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/recovery/sessions/dev/snapshots", nil)
		r.SetPathValue("session", "dev")
		h.listRecoverySnapshots(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		snaps, _ := data["snapshots"].([]any)
		if len(snaps) != 1 {
			t.Fatalf("snapshots len = %d, want 1", len(snaps))
		}
	})

	t.Run("invalid session", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.recovery = &mockRecovery{}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/recovery/sessions/inv@lid/snapshots", nil)
		r.SetPathValue("session", "inv@lid")
		h.listRecoverySnapshots(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("custom limit", func(t *testing.T) {
		t.Parallel()

		var gotLimit int
		h, _ := newTestHandler(t, nil, nil)
		h.recovery = &mockRecovery{
			listSnapshotsFn: func(_ context.Context, _ string, limit int) ([]store.RecoverySnapshot, error) {
				gotLimit = limit
				return nil, nil
			},
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/recovery/sessions/dev/snapshots?limit=5", nil)
		r.SetPathValue("session", "dev")
		h.listRecoverySnapshots(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if gotLimit != 5 {
			t.Fatalf("limit = %d, want 5", gotLimit)
		}
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.recovery = &mockRecovery{
			listSnapshotsFn: func(_ context.Context, _ string, _ int) ([]store.RecoverySnapshot, error) {
				return nil, fmt.Errorf("db error")
			},
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/recovery/sessions/dev/snapshots", nil)
		r.SetPathValue("session", "dev")
		h.listRecoverySnapshots(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})
}

func TestGetRecoverySnapshotHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.recovery = &mockRecovery{
			getSnapshotFn: func(_ context.Context, id int64) (recovery.SnapshotView, error) {
				return recovery.SnapshotView{
					Meta: store.RecoverySnapshot{ID: id, SessionName: "dev"},
				}, nil
			},
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/recovery/snapshots/42", nil)
		r.SetPathValue("snapshot", "42")
		h.getRecoverySnapshot(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})

	t.Run("invalid id", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.recovery = &mockRecovery{}

		tests := []struct {
			name string
			id   string
		}{
			{"not a number", "abc"},
			{"zero", "0"},
			{"negative", "-5"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				w := httptest.NewRecorder()
				r := httptest.NewRequest("GET", "/api/recovery/snapshots/"+tt.id, nil)
				r.SetPathValue("snapshot", tt.id)
				h.getRecoverySnapshot(w, r)

				if w.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400", w.Code)
				}
			})
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.recovery = &mockRecovery{
			getSnapshotFn: func(_ context.Context, _ int64) (recovery.SnapshotView, error) {
				return recovery.SnapshotView{}, fmt.Errorf("not found: %w", sql.ErrNoRows)
			},
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/recovery/snapshots/999", nil)
		r.SetPathValue("snapshot", "999")
		h.getRecoverySnapshot(w, r)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("internal error", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.recovery = &mockRecovery{
			getSnapshotFn: func(_ context.Context, _ int64) (recovery.SnapshotView, error) {
				return recovery.SnapshotView{}, fmt.Errorf("disk error")
			},
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/recovery/snapshots/42", nil)
		r.SetPathValue("snapshot", "42")
		h.getRecoverySnapshot(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})
}

func TestGetRecoveryJobHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.recovery = &mockRecovery{
			getJobFn: func(_ context.Context, id string) (store.RecoveryJob, error) {
				return store.RecoveryJob{ID: id, SessionName: "dev"}, nil
			},
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/recovery/jobs/job-1", nil)
		r.SetPathValue("job", "job-1")
		h.getRecoveryJob(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})

	t.Run("empty id", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.recovery = &mockRecovery{}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/recovery/jobs/", nil)
		r.SetPathValue("job", "")
		h.getRecoveryJob(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.recovery = &mockRecovery{
			getJobFn: func(_ context.Context, _ string) (store.RecoveryJob, error) {
				return store.RecoveryJob{}, fmt.Errorf("not found: %w", sql.ErrNoRows)
			},
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/recovery/jobs/missing", nil)
		r.SetPathValue("job", "missing")
		h.getRecoveryJob(w, r)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("internal error", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.recovery = &mockRecovery{
			getJobFn: func(_ context.Context, _ string) (store.RecoveryJob, error) {
				return store.RecoveryJob{}, fmt.Errorf("disk error")
			},
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/recovery/jobs/job-1", nil)
		r.SetPathValue("job", "job-1")
		h.getRecoveryJob(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})

	t.Run("recovery disabled", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.recovery = nil

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/recovery/jobs/job-1", nil)
		r.SetPathValue("job", "job-1")
		h.getRecoveryJob(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Ops handler tests – deleteOpsAlert, discoverOpsServices
// ---------------------------------------------------------------------------

func TestDeleteOpsAlertHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, nil, nil)
		h.events = events.NewHub()
		ctx := context.Background()

		alert, err := st.UpsertAlert(ctx, alerts.AlertWrite{
			DedupeKey: "test:del", Source: "test", Resource: "svc",
			Title: "test alert", Message: "msg", Severity: "error",
			CreatedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("UpsertAlert: %v", err)
		}
		// Resolve the alert first (deleteAlert requires resolved status).
		if _, err := st.ResolveAlert(ctx, "test:del", time.Now().UTC()); err != nil {
			t.Fatalf("ResolveAlert: %v", err)
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", fmt.Sprintf("/api/ops/alerts/%d", alert.ID), nil)
		r.SetPathValue("alert", fmt.Sprintf("%d", alert.ID))
		h.deleteOpsAlert(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		deleted, _ := data["deleted"].(float64)
		if int64(deleted) != alert.ID {
			t.Fatalf("deleted = %v, want %d", deleted, alert.ID)
		}
	})

	t.Run("invalid id", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		tests := []struct {
			name string
			id   string
		}{
			{"not a number", "abc"},
			{"zero", "0"},
			{"negative", "-1"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				w := httptest.NewRecorder()
				r := httptest.NewRequest("DELETE", "/api/ops/alerts/"+tt.id, nil)
				r.SetPathValue("alert", tt.id)
				h.deleteOpsAlert(w, r)

				if w.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400", w.Code)
				}
			})
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/api/ops/alerts/99999", nil)
		r.SetPathValue("alert", "99999")
		h.deleteOpsAlert(w, r)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})
}

func TestDiscoverOpsServicesHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.ops = &mockOpsControlPlane{
			discoverFn: func(_ context.Context) ([]opsplane.AvailableService, error) {
				return []opsplane.AvailableService{{Unit: "test.service"}}, nil
			},
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/services/discover", nil)
		h.discoverOpsServices(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		services, _ := data["services"].([]any)
		if len(services) != 1 {
			t.Fatalf("services len = %d, want 1", len(services))
		}
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.ops = &mockOpsControlPlane{
			discoverFn: func(_ context.Context) ([]opsplane.AvailableService, error) {
				return nil, fmt.Errorf("discover fail")
			},
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/services/discover", nil)
		h.discoverOpsServices(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})

	t.Run("nil ops", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.ops = nil

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/services/discover", nil)
		h.discoverOpsServices(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Runbook handler tests – deleteOpsJob, opsJob error paths
// ---------------------------------------------------------------------------

func TestDeleteOpsJobHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, nil, nil)
		ctx := context.Background()

		rb, _ := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
			Name:  "del-job-rb",
			Steps: []store.OpsRunbookStep{{Type: "command", Title: "echo", Command: "echo ok"}},
		})
		job, err := st.CreateOpsRunbookRun(ctx, rb.ID, time.Now().UTC())
		if err != nil {
			t.Fatalf("CreateOpsRunbookRun: %v", err)
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/api/ops/jobs/"+job.ID, nil)
		r.SetPathValue("job", job.ID)
		h.deleteOpsJob(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		if data["deleted"] != true {
			t.Fatalf("deleted = %v, want true", data["deleted"])
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/api/ops/jobs/nonexistent", nil)
		r.SetPathValue("job", "nonexistent")
		h.deleteOpsJob(w, r)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("empty id", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/api/ops/jobs/", nil)
		r.SetPathValue("job", "")
		h.deleteOpsJob(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("nil repo", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.repo = nil

		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/api/ops/jobs/abc", nil)
		r.SetPathValue("job", "abc")
		h.deleteOpsJob(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})
}

func TestOpsJobHandlerNotFound(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/jobs/nonexistent", nil)
	r.SetPathValue("job", "nonexistent")
	h.opsJob(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestOpsJobHandlerEmptyID(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/jobs/", nil)
	r.SetPathValue("job", "")
	h.opsJob(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestOpsJobHandlerNilRepo(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.repo = nil

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/jobs/abc", nil)
	r.SetPathValue("job", "abc")
	h.opsJob(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Guardrail handler tests – listGuardrailAudit, updateGuardrailRule
// ---------------------------------------------------------------------------

func TestListGuardrailAuditHandler(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/guardrails/audit", nil)
		h.listGuardrailAudit(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		audit, _ := data["audit"].([]any)
		if audit == nil {
			t.Fatal("audit should be present")
		}
	})

	t.Run("with custom limit", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/guardrails/audit?limit=10", nil)
		h.listGuardrailAudit(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})

	t.Run("invalid limit", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/guardrails/audit?limit=bad", nil)
		h.listGuardrailAudit(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("zero limit", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/guardrails/audit?limit=0", nil)
		h.listGuardrailAudit(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("nil guardrails returns empty", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.guardrails = nil

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/guardrails/audit", nil)
		h.listGuardrailAudit(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})
}

func TestUpdateGuardrailRuleHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)
		ctx := context.Background()

		// Seed a rule via the store.
		if err := st.UpsertGuardrailRule(ctx, store.GuardrailRuleWrite{
			ID: "rule-upd", Name: "test", Pattern: "rm -rf", Mode: "block",
			Severity: "error", Enabled: true,
		}); err != nil {
			t.Fatalf("seed rule: %v", err)
		}

		body := `{"name":"updated","pattern":"rm -rf /","mode":"warn","severity":"warn","enabled":true,"priority":5}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/api/ops/guardrails/rules/rule-upd", strings.NewReader(body))
		r.SetPathValue("rule", "rule-upd")
		h.updateGuardrailRule(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("missing pattern", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)

		body := `{"name":"x","pattern":"","mode":"block","severity":"error","enabled":true}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/api/ops/guardrails/rules/rule-1", strings.NewReader(body))
		r.SetPathValue("rule", "rule-1")
		h.updateGuardrailRule(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("missing enabled", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)

		body := `{"name":"x","pattern":"rm","mode":"block","severity":"error"}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/api/ops/guardrails/rules/rule-1", strings.NewReader(body))
		r.SetPathValue("rule", "rule-1")
		h.updateGuardrailRule(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("missing rule id", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)

		body := `{"name":"x","pattern":"rm","mode":"block","severity":"error","enabled":true}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/api/ops/guardrails/rules/", strings.NewReader(body))
		r.SetPathValue("rule", "")
		h.updateGuardrailRule(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("nil guardrails", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.guardrails = nil

		body := `{"name":"x","pattern":"rm","mode":"block","severity":"error","enabled":true}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/api/ops/guardrails/rules/rule-1", strings.NewReader(body))
		r.SetPathValue("rule", "rule-1")
		h.updateGuardrailRule(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Ops handler tests – opsAlerts filters, opsUnitStatus, opsUnitLogs errors
// ---------------------------------------------------------------------------

func TestOpsAlertsHandlerFilters(t *testing.T) {
	t.Parallel()

	t.Run("invalid limit", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/alerts?limit=bad", nil)
		h.opsAlerts(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("nil repo", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.repo = nil
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/alerts", nil)
		h.opsAlerts(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})

	t.Run("valid status filter", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/alerts?status=open", nil)
		h.opsAlerts(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid status filter", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/alerts?status=bogus", nil)
		h.opsAlerts(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}

func TestOpsUnitStatusHandlerValidation(t *testing.T) {
	t.Parallel()

	t.Run("missing unit", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/units/status?manager=systemd", nil)
		h.opsUnitStatus(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid manager", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/units/status?unit=test.service&manager=bad", nil)
		h.opsUnitStatus(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid scope", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/units/status?unit=test.service&manager=systemd&scope=bad", nil)
		h.opsUnitStatus(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("nil ops", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.ops = nil
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/units/status?unit=test.service&manager=systemd", nil)
		h.opsUnitStatus(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})
}

func TestOpsUnitLogsHandlerValidation(t *testing.T) {
	t.Parallel()

	t.Run("missing unit", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/units/logs?manager=systemd", nil)
		h.opsUnitLogs(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid manager", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/units/logs?unit=test.service&manager=bad", nil)
		h.opsUnitLogs(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid scope", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/units/logs?unit=test.service&manager=systemd&scope=bad", nil)
		h.opsUnitLogs(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("success with lines param", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.ops = &mockOpsControlPlane{
			logsByUnitFn: func(_ context.Context, _, _, _ string, lines int) (string, error) {
				return fmt.Sprintf("lines=%d", lines), nil
			},
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/units/logs?unit=test.service&manager=systemd&scope=user&lines=50", nil)
		h.opsUnitLogs(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("error from service", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.ops = &mockOpsControlPlane{
			logsByUnitFn: func(_ context.Context, _, _, _ string, _ int) (string, error) {
				return "", fmt.Errorf("logs fail")
			},
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/units/logs?unit=test.service&manager=systemd&scope=user", nil)
		h.opsUnitLogs(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})

	t.Run("nil ops", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.ops = nil
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/units/logs?unit=test.service&manager=systemd", nil)
		h.opsUnitLogs(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Presence handler tests – setTmuxPresence validation
// ---------------------------------------------------------------------------

func TestSetTmuxPresenceHandlerValidation(t *testing.T) {
	t.Parallel()

	t.Run("missing terminalId", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		body := `{"session":"dev","windowIndex":0,"paneId":"%1","visible":true,"focused":true}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/presence", strings.NewReader(body))
		h.setTmuxPresence(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid session name", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		body := `{"terminalId":"t1","session":"inv@lid","windowIndex":0,"paneId":"%1","visible":true,"focused":true}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/presence", strings.NewReader(body))
		h.setTmuxPresence(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid windowIndex", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		body := `{"terminalId":"t1","session":"dev","windowIndex":-2,"paneId":"%1","visible":true,"focused":true}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/presence", strings.NewReader(body))
		h.setTmuxPresence(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid paneId", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		body := `{"terminalId":"t1","session":"dev","windowIndex":0,"paneId":"nopercent","visible":true,"focused":true}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/presence", strings.NewReader(body))
		h.setTmuxPresence(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("nil repo", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.repo = nil
		body := `{"terminalId":"t1","session":"dev","windowIndex":0,"paneId":"%1","visible":true,"focused":true}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/tmux/presence", strings.NewReader(body))
		h.setTmuxPresence(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Ops handler tests – opsOverview, opsServices nil-ops and error paths
// ---------------------------------------------------------------------------

func TestOpsOverviewHandlerErrors(t *testing.T) {
	t.Parallel()

	t.Run("nil ops", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.ops = nil
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/overview", nil)
		h.opsOverview(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})

	t.Run("overview error", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.ops = &mockOpsControlPlane{
			overviewFn: func(_ context.Context) (opsplane.Overview, error) {
				return opsplane.Overview{}, fmt.Errorf("fail")
			},
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/overview", nil)
		h.opsOverview(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})
}

func TestOpsServicesHandlerErrors(t *testing.T) {
	t.Parallel()

	t.Run("nil ops", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.ops = nil
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/services", nil)
		h.opsServices(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})

	t.Run("list error", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.ops = &mockOpsControlPlane{
			listServicesFn: func(_ context.Context) ([]opsplane.ServiceStatus, error) {
				return nil, fmt.Errorf("fail")
			},
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/ops/services", nil)
		h.opsServices(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Ops metrics nil-ops test
// ---------------------------------------------------------------------------

func TestOpsMetricsNilOps(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.ops = nil
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/metrics", nil)
	h.opsMetrics(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Evaluate guardrail handler test
// ---------------------------------------------------------------------------

func TestEvaluateGuardrailHandler(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)

		body := `{"action":"rm -rf /","sessionName":"dev","windowIndex":0,"paneId":"%1"}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/ops/guardrails/evaluate", strings.NewReader(body))
		h.evaluateGuardrail(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid paneId", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)

		body := `{"action":"test","paneId":"nopercent"}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/ops/guardrails/evaluate", strings.NewReader(body))
		h.evaluateGuardrail(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("nil guardrails", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, nil, nil)
		h.guardrails = nil

		body := `{"action":"test"}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/ops/guardrails/evaluate", strings.NewReader(body))
		h.evaluateGuardrail(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Ops service action handler – invalid action error from service
// ---------------------------------------------------------------------------

func TestOpsServiceActionHandlerInvalidActionFromService(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.ops = &mockOpsControlPlane{
		actFn: func(_ context.Context, _, _ string) (opsplane.ServiceStatus, error) {
			return opsplane.ServiceStatus{}, opsplane.ErrInvalidAction
		},
	}

	body := `{"action":"restart"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/ops/services/test-svc/action", strings.NewReader(body))
	r.SetPathValue("service", "test-svc")
	h.opsServiceAction(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Ops service logs – custom lines parameter
// ---------------------------------------------------------------------------

func TestOpsServiceLogsCustomLines(t *testing.T) {
	t.Parallel()

	var gotLines int
	h, _ := newTestHandler(t, nil, nil)
	h.ops = &mockOpsControlPlane{
		logsFn: func(_ context.Context, _ string, lines int) (string, error) {
			gotLines = lines
			return "log output", nil
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/services/test-svc/logs?lines=50", nil)
	r.SetPathValue("service", "test-svc")
	h.opsServiceLogs(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotLines != 50 {
		t.Fatalf("lines = %d, want 50", gotLines)
	}
}

// ---------------------------------------------------------------------------
// Ack ops alert – not found error
// ---------------------------------------------------------------------------

func TestAckOpsAlertNotFound(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/ops/alerts/99999/ack", nil)
	r.SetPathValue("alert", "99999")
	h.ackOpsAlert(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Browse ops services – nil result normalization
// ---------------------------------------------------------------------------

func TestBrowseOpsServicesNilResult(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.ops = &mockOpsControlPlane{
		browseFn: func(_ context.Context) ([]opsplane.BrowsedService, error) {
			return nil, nil
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/services/browse", nil)
	h.browseOpsServices(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	services, _ := data["services"].([]any)
	if services == nil {
		t.Fatal("services should be non-nil empty array")
	}
}

// ---------------------------------------------------------------------------
// Ops unit action – validation, ErrInvalidAction
// ---------------------------------------------------------------------------

func TestOpsUnitActionHandlerValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{"invalid action", `{"unit":"test.service","manager":"systemd","action":"bad"}`},
		{"invalid manager", `{"unit":"test.service","manager":"bad","action":"start"}`},
		{"invalid scope", `{"unit":"test.service","manager":"systemd","scope":"bad","action":"start"}`},
		{"invalid json", `{not-json}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler(t, nil, nil)
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/api/ops/units/action", strings.NewReader(tt.body))
			h.opsUnitAction(w, r)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestOpsUnitActionHandlerInvalidActionFromService(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.ops = &mockOpsControlPlane{
		actByUnitFn: func(_ context.Context, _, _, _, _ string) error {
			return opsplane.ErrInvalidAction
		},
		overviewFn: func(_ context.Context) (opsplane.Overview, error) {
			return opsplane.Overview{}, nil
		},
	}

	body := `{"unit":"test.service","manager":"systemd","action":"start"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/ops/units/action", strings.NewReader(body))
	h.opsUnitAction(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Storage handler – opsConfig and patchOpsConfig edge cases
// ---------------------------------------------------------------------------

func TestOpsConfigHandlerReadError(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.configPath = "/nonexistent/path/config.toml"

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/config", nil)
	h.opsConfig(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestPatchOpsConfigHandlerEmptyContent(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.configPath = filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(h.configPath, []byte("# orig"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	body := `{"content":""}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PATCH", "/api/ops/config", strings.NewReader(body))
	h.patchOpsConfig(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Storage flush – nil repo
// ---------------------------------------------------------------------------

func TestStorageStatsNilRepo(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.repo = nil

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/storage/stats", nil)
	h.storageStats(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestFlushStorageNilRepo(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.repo = nil

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/ops/storage/flush", strings.NewReader(`{"resource":"timeline"}`))
	h.flushStorage(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

// ---------------------------------------------------------------------------
// decodeOptionalJSON edge cases
// ---------------------------------------------------------------------------

func TestDecodeOptionalJSON(t *testing.T) {
	t.Parallel()

	t.Run("empty body", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest("POST", "/", strings.NewReader(""))
		var dst struct{ Name string }
		err := decodeOptionalJSON(r, &dst)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("whitespace body", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest("POST", "/", strings.NewReader("   "))
		var dst struct{ Name string }
		err := decodeOptionalJSON(r, &dst)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("valid json", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"test"}`))
		var dst struct {
			Name string `json:"name"`
		}
		err := decodeOptionalJSON(r, &dst)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dst.Name != "test" {
			t.Fatalf("name = %q, want test", dst.Name)
		}
	})

	t.Run("multiple json values", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"a"}{"name":"b"}`))
		var dst struct {
			Name string `json:"name"`
		}
		err := decodeOptionalJSON(r, &dst)
		if err == nil {
			t.Fatal("expected error for multiple json values")
		}
	})

	t.Run("unknown fields", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"a","extra":true}`))
		var dst struct {
			Name string `json:"name"`
		}
		err := decodeOptionalJSON(r, &dst)
		if err == nil {
			t.Fatal("expected error for unknown fields")
		}
	})
}

// ---------------------------------------------------------------------------
// parseActivityLimitParam edge cases
// ---------------------------------------------------------------------------

func TestParseActivityLimitParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		fallback int
		want     int
		wantErr  bool
	}{
		{"empty uses fallback", "", 50, 50, false},
		{"valid number", "10", 50, 10, false},
		{"over 500 clamped", "999", 50, 500, false},
		{"negative rejected", "-1", 50, 0, true},
		{"zero rejected", "0", 50, 0, true},
		{"not a number", "abc", 50, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseActivityLimitParam(tt.raw, tt.fallback)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got = %d, want %d", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Activity handler – opsActivity nil repo / invalid filter
// ---------------------------------------------------------------------------

func TestOpsActivityNilRepo(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.repo = nil

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/activity", nil)
	h.opsActivity(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestOpsActivityInvalidLimit(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/activity?limit=bad", nil)
	h.opsActivity(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Runbooks handler – opsRunbooks nil repo
// ---------------------------------------------------------------------------

func TestOpsRunbooksNilRepo(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.repo = nil

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/ops/runbooks", nil)
	h.opsRunbooks(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

// ---------------------------------------------------------------------------
// runOpsRunbook – nil repo, empty runbook id
// ---------------------------------------------------------------------------

func TestRunOpsRunbookNilRepo(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	h.repo = nil

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/ops/runbooks/abc/run", nil)
	r.SetPathValue("runbook", "abc")
	h.runOpsRunbook(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestRunOpsRunbookEmptyID(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/ops/runbooks//run", nil)
	r.SetPathValue("runbook", "")
	h.runOpsRunbook(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestRunOpsRunbookNotFound(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/ops/runbooks/nonexistent/run", nil)
	r.SetPathValue("runbook", "nonexistent")
	h.runOpsRunbook(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}
