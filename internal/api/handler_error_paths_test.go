package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/guardrails"
	"github.com/opus-domini/sentinel/internal/notify"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

// ---------------------------------------------------------------------------
// tmux launcher error paths
// ---------------------------------------------------------------------------

func TestTmuxLauncherErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("create rejects malformed JSON", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/launchers", strings.NewReader(`{`))
		h.createTmuxLauncher(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("update rejects empty id", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/launchers/", strings.NewReader(`{}`))
		h.updateTmuxLauncher(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("update rejects malformed JSON", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/launchers/abc", strings.NewReader(`not json`))
		r.SetPathValue("launcher", "abc")
		h.updateTmuxLauncher(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("update returns 404 for unknown launcher", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/launchers/missing", strings.NewReader(`{"name":"Codex","icon":"code","command":"codex","cwdMode":"session"}`))
		r.SetPathValue("launcher", "missing")
		h.updateTmuxLauncher(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("delete rejects empty id", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/tmux/launchers/", nil)
		h.deleteTmuxLauncher(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("delete returns 404 for unknown launcher", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/tmux/launchers/missing", nil)
		r.SetPathValue("launcher", "missing")
		h.deleteTmuxLauncher(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("reorder rejects malformed JSON", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/launchers/order", strings.NewReader(`{`))
		h.reorderTmuxLaunchers(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("reorder rejects empty ids", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/launchers/order", strings.NewReader(`{"ids":[]}`))
		h.reorderTmuxLaunchers(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("launch rejects invalid session name", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/bad/launchers/x/launch", nil)
		r.SetPathValue("session", "bad name")
		r.SetPathValue("launcher", "x")
		h.launchTmuxLauncher(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("launch rejects empty launcher id", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/dev/launchers//launch", nil)
		r.SetPathValue("session", "dev")
		h.launchTmuxLauncher(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("launch returns 404 for unknown launcher", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/dev/launchers/missing/launch", nil)
		r.SetPathValue("session", "dev")
		r.SetPathValue("launcher", "missing")
		h.launchTmuxLauncher(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("launch surfaces tmux new-window failure", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			newWindowWithOptionsFn: func(context.Context, string, string, string) (tmux.NewWindowResult, error) {
				return tmux.NewWindowResult{}, &tmux.Error{Kind: tmux.ErrKindCommandFailed}
			},
		}
		h, st := newTestHandler(t, tm, nil)
		launcher, err := st.CreateTmuxLauncher(context.Background(), store.TmuxLauncherWrite{
			Name:       "Codex",
			Icon:       "code",
			Command:    "codex",
			CwdMode:    store.TmuxLauncherCwdModeSession,
			WindowName: "codex",
		})
		if err != nil {
			t.Fatalf("CreateTmuxLauncher() error = %v", err)
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/dev/launchers/"+launcher.ID+"/launch", nil)
		r.SetPathValue("session", "dev")
		r.SetPathValue("launcher", launcher.ID)
		h.launchTmuxLauncher(w, r)
		if w.Code != http.StatusInternalServerError && w.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want tmux error status", w.Code)
		}
	})

	t.Run("launch surfaces active-pane cwd lookup failure", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			listPanesFn: func(context.Context, string) ([]tmux.Pane, error) {
				return nil, &tmux.Error{Kind: tmux.ErrKindCommandFailed}
			},
		}
		h, st := newTestHandler(t, tm, nil)
		launcher, err := st.CreateTmuxLauncher(context.Background(), store.TmuxLauncherWrite{
			Name:       "Codex",
			Icon:       "code",
			Command:    "codex",
			CwdMode:    store.TmuxLauncherCwdModeActivePane,
			WindowName: "codex",
		})
		if err != nil {
			t.Fatalf("CreateTmuxLauncher() error = %v", err)
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/dev/launchers/"+launcher.ID+"/launch", nil)
		r.SetPathValue("session", "dev")
		r.SetPathValue("launcher", launcher.ID)
		h.launchTmuxLauncher(w, r)
		if w.Code == http.StatusOK {
			t.Fatalf("status = %d, want error", w.Code)
		}
	})
}

func TestResolveTmuxLauncherCwd(t *testing.T) {
	t.Parallel()

	t.Run("session mode returns empty", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		cwd, err := h.resolveTmuxLauncherCwd(context.Background(), "dev", store.TmuxLauncher{
			CwdMode: store.TmuxLauncherCwdModeSession,
		})
		if err != nil || cwd != "" {
			t.Fatalf("got (%q, %v), want (\"\", nil)", cwd, err)
		}
	})

	t.Run("fixed mode returns configured value", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		cwd, err := h.resolveTmuxLauncherCwd(context.Background(), "dev", store.TmuxLauncher{
			CwdMode:  store.TmuxLauncherCwdModeFixed,
			CwdValue: "/srv/api",
		})
		if err != nil || cwd != "/srv/api" {
			t.Fatalf("got (%q, %v), want (/srv/api, nil)", cwd, err)
		}
	})

	t.Run("invalid mode returns error", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		if _, err := h.resolveTmuxLauncherCwd(context.Background(), "dev", store.TmuxLauncher{
			CwdMode: "bogus",
		}); err == nil {
			t.Fatal("expected error for invalid cwd mode")
		}
	})

	t.Run("active-pane falls back to non-active pane path", func(t *testing.T) {
		t.Parallel()
		tm := &mockTmux{
			listPanesFn: func(context.Context, string) ([]tmux.Pane, error) {
				return []tmux.Pane{
					{Active: true, CurrentPath: ""},
					{Active: false, CurrentPath: "/srv/fallback"},
				}, nil
			},
		}
		h, _ := newTestHandler(t, tm, nil)
		cwd, err := h.resolveTmuxLauncherCwd(context.Background(), "dev", store.TmuxLauncher{
			CwdMode: store.TmuxLauncherCwdModeActivePane,
		})
		if err != nil || cwd != "/srv/fallback" {
			t.Fatalf("got (%q, %v), want (/srv/fallback, nil)", cwd, err)
		}
	})
}

// ---------------------------------------------------------------------------
// session preset error paths
// ---------------------------------------------------------------------------

func TestSessionPresetErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("create rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/session-presets", strings.NewReader(`{`))
		h.createSessionPreset(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("create rejects duplicate preset", func(t *testing.T) {
		t.Parallel()
		h, st := newTestHandler(t, &mockTmux{}, nil)
		if _, err := st.CreateSessionPreset(context.Background(), store.SessionPresetWrite{
			Name: "api", Cwd: "/srv/api", Icon: "server",
		}); err != nil {
			t.Fatalf("CreateSessionPreset() error = %v", err)
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/session-presets", strings.NewReader(`{"name":"api","cwd":"/srv/api","icon":"server"}`))
		h.createSessionPreset(w, r)
		if w.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("update rejects invalid name", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/session-presets/bad%20name", strings.NewReader(`{}`))
		r.SetPathValue("preset", "bad name")
		h.updateSessionPreset(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("update rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/session-presets/api", strings.NewReader(`bad`))
		r.SetPathValue("preset", "api")
		h.updateSessionPreset(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("update returns 404 for unknown preset", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/session-presets/missing", strings.NewReader(`{"name":"missing","cwd":"/srv/x","icon":"server"}`))
		r.SetPathValue("preset", "missing")
		h.updateSessionPreset(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("delete rejects invalid name", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/tmux/session-presets/bad", nil)
		r.SetPathValue("preset", "bad name")
		h.deleteSessionPreset(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("delete returns 404 for unknown preset", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/tmux/session-presets/missing", nil)
		r.SetPathValue("preset", "missing")
		h.deleteSessionPreset(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("launch rejects invalid name", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/session-presets/bad/launch", nil)
		r.SetPathValue("preset", "bad name")
		h.launchSessionPreset(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("launch returns 404 for unknown preset", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/session-presets/missing/launch", nil)
		r.SetPathValue("preset", "missing")
		h.launchSessionPreset(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("launch surfaces tmux create failure", func(t *testing.T) {
		t.Parallel()
		tm := &mockTmux{
			createSessionFn: func(context.Context, string, string) error {
				return &tmux.Error{Kind: tmux.ErrKindCommandFailed}
			},
		}
		h, st := newTestHandler(t, tm, nil)
		if _, err := st.CreateSessionPreset(context.Background(), store.SessionPresetWrite{
			Name: "api", Cwd: "/srv/api", Icon: "server",
		}); err != nil {
			t.Fatalf("CreateSessionPreset() error = %v", err)
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/session-presets/api/launch", nil)
		r.SetPathValue("preset", "api")
		h.launchSessionPreset(w, r)
		if w.Code == http.StatusOK {
			t.Fatalf("status = %d, want error", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// session launcher error paths
// ---------------------------------------------------------------------------

func TestSessionLauncherErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("create rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/session-launchers", strings.NewReader(`{`))
		h.createSessionLauncher(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("create rejects invalid name", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/session-launchers", strings.NewReader(`{"name":"bad name","cwd":"/srv/api","icon":"server"}`))
		h.createSessionLauncher(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("create rejects duplicate launcher", func(t *testing.T) {
		t.Parallel()
		h, st := newTestHandler(t, &mockTmux{}, nil)
		if _, err := st.CreateSessionLauncher(context.Background(), store.SessionLauncherWrite{
			Name: "api", Cwd: "/srv/api", Icon: "server",
		}); err != nil {
			t.Fatalf("CreateSessionLauncher() error = %v", err)
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/session-launchers", strings.NewReader(`{"name":"api","cwd":"/srv/api","icon":"server"}`))
		h.createSessionLauncher(w, r)
		if w.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("update rejects empty id", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/session-launchers/", strings.NewReader(`{}`))
		h.updateSessionLauncher(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("update rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/session-launchers/abc", strings.NewReader(`bad`))
		r.SetPathValue("launcher", "abc")
		h.updateSessionLauncher(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("update returns 404 for unknown launcher", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/session-launchers/missing", strings.NewReader(`{"name":"x","cwd":"/srv/x","icon":"server"}`))
		r.SetPathValue("launcher", "missing")
		h.updateSessionLauncher(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("delete rejects empty id", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/tmux/session-launchers/", nil)
		h.deleteSessionLauncher(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("delete returns 404 for unknown launcher", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/tmux/session-launchers/missing", nil)
		r.SetPathValue("launcher", "missing")
		h.deleteSessionLauncher(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("reorder rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/session-launchers/order", strings.NewReader(`{`))
		h.reorderSessionLaunchers(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("reorder rejects empty ids", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/session-launchers/order", strings.NewReader(`{"ids":[]}`))
		h.reorderSessionLaunchers(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("launch rejects empty id", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/session-launchers//launch", nil)
		h.launchSessionLauncher(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("launch returns 404 for unknown launcher", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, &mockTmux{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/session-launchers/missing/launch", nil)
		r.SetPathValue("launcher", "missing")
		h.launchSessionLauncher(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("launch surfaces tmux create failure", func(t *testing.T) {
		t.Parallel()
		tm := &mockTmux{
			createSessionFn: func(context.Context, string, string) error {
				return &tmux.Error{Kind: tmux.ErrKindCommandFailed}
			},
		}
		h, st := newTestHandler(t, tm, nil)
		launcher, err := st.CreateSessionLauncher(context.Background(), store.SessionLauncherWrite{
			Name: "api", Cwd: "/srv/api", Icon: "server",
		})
		if err != nil {
			t.Fatalf("CreateSessionLauncher() error = %v", err)
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/session-launchers/"+launcher.ID+"/launch", nil)
		r.SetPathValue("launcher", launcher.ID)
		h.launchSessionLauncher(w, r)
		if w.Code == http.StatusOK {
			t.Fatalf("status = %d, want error", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// runbook error paths
// ---------------------------------------------------------------------------

func TestRunbookErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("update rejects empty id", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/ops/runbooks/", strings.NewReader(`{}`))
		h.updateOpsRunbook(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("update rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/ops/runbooks/abc", strings.NewReader(`bad`))
		r.SetPathValue("runbook", "abc")
		h.updateOpsRunbook(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("update rejects invalid step type", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/ops/runbooks/abc", strings.NewReader(`{"name":"rb","steps":[{"type":"bogus","title":"x"}]}`))
		r.SetPathValue("runbook", "abc")
		h.updateOpsRunbook(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("update rejects invalid webhook URL", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/ops/runbooks/abc", strings.NewReader(`{"name":"rb","webhookUrl":"ftp://bad","steps":[]}`))
		r.SetPathValue("runbook", "abc")
		h.updateOpsRunbook(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("delete rejects empty id", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/ops/runbooks/", nil)
		h.deleteOpsRunbook(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("delete returns 404 for unknown runbook", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/ops/runbooks/missing", nil)
		r.SetPathValue("runbook", "missing")
		h.deleteOpsRunbook(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("reject rejects empty run id", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/jobs//reject", nil)
		h.rejectOpsRunbookRun(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("reject returns 404 for unknown run", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/jobs/missing/reject", nil)
		r.SetPathValue("runId", "missing")
		h.rejectOpsRunbookRun(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("reject rejects run not waiting for approval", func(t *testing.T) {
		t.Parallel()
		h, st := newTestHandler(t, nil, nil)
		ctx := context.Background()
		rb, err := st.InsertOpsRunbook(ctx, store.OpsRunbookWrite{
			Name:  "reject-state",
			Steps: []store.OpsRunbookStep{{Type: "run", Title: "x", Command: "true"}},
		})
		if err != nil {
			t.Fatalf("InsertOpsRunbook: %v", err)
		}
		run, err := st.CreateOpsRunbookRun(ctx, rb.ID, time.Now().UTC())
		if err != nil {
			t.Fatalf("CreateOpsRunbookRun: %v", err)
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/jobs/"+run.ID+"/reject", nil)
		r.SetPathValue("runId", run.ID)
		h.rejectOpsRunbookRun(w, r)
		if w.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("approve rejects empty run id", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/jobs//approve", nil)
		h.approveOpsRunbookRun(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("approve returns 404 for unknown run", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/jobs/missing/approve", nil)
		r.SetPathValue("runId", "missing")
		h.approveOpsRunbookRun(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// guardrails error paths
// ---------------------------------------------------------------------------

func TestGuardrailErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("update rejects empty rule id", func(t *testing.T) {
		t.Parallel()
		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/ops/guardrails/", strings.NewReader(`{}`))
		h.updateGuardrailRule(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("update rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/ops/guardrails/rule-1", strings.NewReader(`bad`))
		r.SetPathValue("rule", "rule-1")
		h.updateGuardrailRule(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("update rejects missing pattern", func(t *testing.T) {
		t.Parallel()
		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/ops/guardrails/rule-1", strings.NewReader(`{"enabled":true}`))
		r.SetPathValue("rule", "rule-1")
		h.updateGuardrailRule(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("update rejects missing enabled flag", func(t *testing.T) {
		t.Parallel()
		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/ops/guardrails/rule-1", strings.NewReader(`{"pattern":"rm -rf"}`))
		r.SetPathValue("rule", "rule-1")
		h.updateGuardrailRule(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("create rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/guardrails", strings.NewReader(`bad`))
		h.createGuardrailRule(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("create rejects missing enabled flag", func(t *testing.T) {
		t.Parallel()
		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/guardrails", strings.NewReader(`{"pattern":"rm -rf"}`))
		h.createGuardrailRule(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("delete rejects empty rule id", func(t *testing.T) {
		t.Parallel()
		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/ops/guardrails/", nil)
		h.deleteGuardrailRule(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("evaluate rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/guardrails/evaluate", strings.NewReader(`bad`))
		h.evaluateGuardrail(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("evaluate rejects bad pane id", func(t *testing.T) {
		t.Parallel()
		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/guardrails/evaluate", strings.NewReader(`{"action":"pane.kill","paneId":"7"}`))
		h.evaluateGuardrail(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("list audit rejects invalid limit", func(t *testing.T) {
		t.Parallel()
		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/ops/guardrails/audit?limit=-1", nil)
		h.listGuardrailAudit(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("list audit caps oversized limit", func(t *testing.T) {
		t.Parallel()
		h, st := newTestHandler(t, nil, nil)
		h.guardrails = guardrails.New(st)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/ops/guardrails/audit?limit=99999", nil)
		h.listGuardrailAudit(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// schedule error paths
// ---------------------------------------------------------------------------

func TestScheduleErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("create rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/schedules", strings.NewReader(`bad`))
		h.createSchedule(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("create rejects missing runbook id", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/schedules", strings.NewReader(`{"name":"x","scheduleType":"cron"}`))
		h.createSchedule(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("create rejects missing name", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/schedules", strings.NewReader(`{"runbookId":"rb","scheduleType":"cron"}`))
		h.createSchedule(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("create rejects invalid schedule type", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/schedules", strings.NewReader(`{"runbookId":"rb","name":"x","scheduleType":"weekly"}`))
		h.createSchedule(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("create rejects unknown runbook", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/schedules", strings.NewReader(`{"runbookId":"missing","name":"x","scheduleType":"cron","cronExpr":"* * * * *"}`))
		h.createSchedule(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("create rejects invalid cron expression", func(t *testing.T) {
		t.Parallel()
		h, st := newTestHandler(t, nil, nil)
		rb, err := st.InsertOpsRunbook(context.Background(), store.OpsRunbookWrite{Name: "rb"})
		if err != nil {
			t.Fatalf("InsertOpsRunbook: %v", err)
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/schedules", strings.NewReader(`{"runbookId":"`+rb.ID+`","name":"x","scheduleType":"cron","cronExpr":"not a cron"}`))
		h.createSchedule(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("update rejects empty id", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/ops/schedules/", strings.NewReader(`{}`))
		h.updateSchedule(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("update rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/ops/schedules/abc", strings.NewReader(`bad`))
		r.SetPathValue("schedule", "abc")
		h.updateSchedule(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("update rejects invalid schedule type", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/ops/schedules/abc", strings.NewReader(`{"runbookId":"rb","name":"x","scheduleType":"weekly"}`))
		r.SetPathValue("schedule", "abc")
		h.updateSchedule(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("update returns 404 for unknown schedule", func(t *testing.T) {
		t.Parallel()
		h, st := newTestHandler(t, nil, nil)
		rb, err := st.InsertOpsRunbook(context.Background(), store.OpsRunbookWrite{Name: "rb"})
		if err != nil {
			t.Fatalf("InsertOpsRunbook: %v", err)
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/ops/schedules/missing", strings.NewReader(`{"runbookId":"`+rb.ID+`","name":"x","scheduleType":"cron","cronExpr":"* * * * *"}`))
		r.SetPathValue("schedule", "missing")
		h.updateSchedule(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("delete rejects empty id", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/ops/schedules/", nil)
		h.deleteSchedule(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("delete returns 404 for unknown schedule", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/ops/schedules/missing", nil)
		r.SetPathValue("schedule", "missing")
		h.deleteSchedule(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("trigger rejects empty id", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/schedules//trigger", nil)
		h.triggerSchedule(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("trigger returns 404 for unknown schedule", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/schedules/missing/trigger", nil)
		r.SetPathValue("schedule", "missing")
		h.triggerSchedule(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// markers / storage / webhook error paths
// ---------------------------------------------------------------------------

func TestMarkerAndStorageErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("delete marker rejects empty id", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/ops/markers/", nil)
		h.deleteMarkerPattern(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("delete marker returns 404 for unknown pattern", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/ops/markers/missing", nil)
		r.SetPathValue("pattern", "missing")
		h.deleteMarkerPattern(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("upsert marker rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/ops/markers/x", strings.NewReader(`bad`))
		r.SetPathValue("pattern", "x")
		h.upsertMarkerPattern(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("test webhook rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/settings/webhook/test", strings.NewReader(`bad`))
		h.testWebhook(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("test webhook rejects empty url", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/settings/webhook/test", strings.NewReader(`{"url":""}`))
		h.testWebhook(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("test webhook rejects bad url scheme", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/settings/webhook/test", strings.NewReader(`{"url":"ftp://example.com"}`))
		h.testWebhook(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("test webhook reports delivery failure", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		// Unroutable address -> Send fails.
		r := httptest.NewRequest(http.MethodPost, "/api/ops/settings/webhook/test", strings.NewReader(`{"url":"http://127.0.0.1:0/hook"}`))
		h.testWebhook(w, r)
		if w.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502; body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("patch webhook rejects bad url", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/ops/settings/webhook", strings.NewReader(`{"url":"ftp://example.com"}`))
		h.patchWebhookSettings(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("patch webhook rejects unsupported event", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/ops/settings/webhook", strings.NewReader(`{"url":"https://example.com","events":["bogus.event"]}`))
		h.patchWebhookSettings(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("patch timezone rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/ops/settings/timezone", strings.NewReader(`bad`))
		h.patchTimezone(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("patch timezone rejects empty timezone", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/ops/settings/timezone", strings.NewReader(`{"timezone":""}`))
		h.patchTimezone(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("patch locale rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/ops/settings/locale", strings.NewReader(`bad`))
		h.patchLocale(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("patch ops config rejects empty content", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		h.configPath = t.TempDir() + "/config.toml"
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/ops/config", strings.NewReader(`{"content":""}`))
		h.patchOpsConfig(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("patch ops config rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		h.configPath = t.TempDir() + "/config.toml"
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/ops/config", strings.NewReader(`bad`))
		h.patchOpsConfig(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("flush storage rejects empty resource", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/ops/storage/flush", strings.NewReader(`{"resource":""}`))
		h.flushStorage(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// presence / timeline helper coverage
// ---------------------------------------------------------------------------

func TestParseTimelineWindowIndexParam(t *testing.T) {
	t.Parallel()

	t.Run("empty returns nil", func(t *testing.T) {
		t.Parallel()
		got, err := parseTimelineWindowIndexParam("")
		if err != nil || got != nil {
			t.Fatalf("got (%v, %v), want (nil, nil)", got, err)
		}
	})

	t.Run("valid returns pointer", func(t *testing.T) {
		t.Parallel()
		got, err := parseTimelineWindowIndexParam("3")
		if err != nil || got == nil || *got != 3 {
			t.Fatalf("got (%v, %v), want (*3, nil)", got, err)
		}
	})

	t.Run("negative returns error", func(t *testing.T) {
		t.Parallel()
		if _, err := parseTimelineWindowIndexParam("-2"); err == nil {
			t.Fatal("expected error for negative window index")
		}
	})

	t.Run("non-numeric returns error", func(t *testing.T) {
		t.Parallel()
		if _, err := parseTimelineWindowIndexParam("abc"); err == nil {
			t.Fatal("expected error for non-numeric window index")
		}
	})
}

func TestTimelineSearchRejectsBadParams(t *testing.T) {
	t.Parallel()

	t.Run("invalid window index", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/ops/timeline/search?windowIndex=-1", nil)
		h.timelineSearch(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid since timestamp", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/ops/timeline/search?since=not-a-time", nil)
		h.timelineSearch(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid session scope", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/ops/timeline/search?session=bad%20name", nil)
		h.timelineSearch(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid pane scope", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t, nil, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/ops/timeline/search?paneId=7", nil)
		h.timelineSearch(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// projection helpers
// ---------------------------------------------------------------------------

func TestSameProjectedWindowSet(t *testing.T) {
	t.Parallel()

	t.Run("empty live returns false", func(t *testing.T) {
		t.Parallel()
		if sameProjectedWindowSet(nil, nil) {
			t.Fatal("empty sets should not be considered equal")
		}
	})

	t.Run("length mismatch returns false", func(t *testing.T) {
		t.Parallel()
		live := []tmux.Window{{Index: 0}}
		if sameProjectedWindowSet(live, nil) {
			t.Fatal("length mismatch should not match")
		}
	})

	t.Run("index mismatch returns false", func(t *testing.T) {
		t.Parallel()
		live := []tmux.Window{{Index: 0}, {Index: 1}}
		projected := []store.WatchtowerWindow{{WindowIndex: 0}, {WindowIndex: 5}}
		if sameProjectedWindowSet(live, projected) {
			t.Fatal("differing indices should not match")
		}
	})

	t.Run("matching set returns true", func(t *testing.T) {
		t.Parallel()
		live := []tmux.Window{{Index: 0}, {Index: 1}}
		projected := []store.WatchtowerWindow{{WindowIndex: 1}, {WindowIndex: 0}}
		if !sameProjectedWindowSet(live, projected) {
			t.Fatal("matching index set should be equal")
		}
	})
}

func TestManagedTmuxWindowForIndex(t *testing.T) {
	t.Parallel()

	t.Run("surfaces list-windows failure", func(t *testing.T) {
		t.Parallel()
		tm := &mockTmux{
			listWindowsFn: func(context.Context, string) ([]tmux.Window, error) {
				return nil, &tmux.Error{Kind: tmux.ErrKindCommandFailed}
			},
		}
		h, _ := newTestHandler(t, tm, nil)
		if _, _, err := h.managedTmuxWindowForIndex(context.Background(), "dev", 0); err == nil {
			t.Fatal("expected error from list windows failure")
		}
	})

	t.Run("returns false when index not found", func(t *testing.T) {
		t.Parallel()
		tm := &mockTmux{
			listWindowsFn: func(context.Context, string) ([]tmux.Window, error) {
				return []tmux.Window{{Index: 0, ID: "@1"}}, nil
			},
		}
		h, _ := newTestHandler(t, tm, nil)
		_, ok, err := h.managedTmuxWindowForIndex(context.Background(), "dev", 9)
		if err != nil || ok {
			t.Fatalf("got (ok=%v, err=%v), want (false, nil)", ok, err)
		}
	})

	t.Run("returns unmanaged window when no managed row", func(t *testing.T) {
		t.Parallel()
		tm := &mockTmux{
			listWindowsFn: func(context.Context, string) ([]tmux.Window, error) {
				return []tmux.Window{{Index: 0, ID: "@1"}}, nil
			},
		}
		h, _ := newTestHandler(t, tm, nil)
		_, ok, err := h.managedTmuxWindowForIndex(context.Background(), "dev", 0)
		if err != nil || ok {
			t.Fatalf("got (ok=%v, err=%v), want (false, nil)", ok, err)
		}
	})
}

func TestReconcileManagedTmuxWindowsNoRows(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, &mockTmux{}, nil)
	rows, err := h.reconcileManagedTmuxWindows(context.Background(), "dev", nil)
	if err != nil || len(rows) != 0 {
		t.Fatalf("got (%d rows, %v), want (0, nil)", len(rows), err)
	}
}

// ---------------------------------------------------------------------------
// api.go helpers
// ---------------------------------------------------------------------------

func TestSetNotifier(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil, nil)
	n := notify.New("https://example.com/hook", []string{"alert.created"})
	h.SetNotifier(n)
	if h.notifier != n {
		t.Fatal("SetNotifier did not store notifier")
	}

	// Nil receiver must not panic.
	var nilHandler *Handler
	nilHandler.SetNotifier(n)
}

func TestMarshalMetadata(t *testing.T) {
	t.Parallel()

	t.Run("valid value marshals", func(t *testing.T) {
		t.Parallel()
		got := marshalMetadata(map[string]string{"k": "v"})
		if got != `{"k":"v"}` {
			t.Fatalf("got %q, want JSON object", got)
		}
	})

	t.Run("unmarshalable value returns empty object", func(t *testing.T) {
		t.Parallel()
		got := marshalMetadata(make(chan int))
		if got != "{}" {
			t.Fatalf("got %q, want {}", got)
		}
	})
}
