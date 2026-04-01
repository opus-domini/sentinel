package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

func TestTmuxLauncherHandlers(t *testing.T) {
	t.Parallel()

	t.Run("create list update delete launcher", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)

		createW := httptest.NewRecorder()
		createR := httptest.NewRequest(http.MethodPost, "/api/tmux/launchers", strings.NewReader(`{"name":"Codex","icon":"code","command":"codex","cwdMode":"active-pane","windowName":"codex"}`))
		h.createTmuxLauncher(createW, createR)

		if createW.Code != http.StatusCreated {
			t.Fatalf("create status = %d, want 201", createW.Code)
		}

		createBody := jsonBody(t, createW)
		data, _ := createBody["data"].(map[string]any)
		launcher, _ := data["launcher"].(map[string]any)
		launcherID, _ := launcher["id"].(string)
		if launcherID == "" {
			t.Fatal("launcher id is empty")
		}

		listW := httptest.NewRecorder()
		listR := httptest.NewRequest(http.MethodGet, "/api/tmux/launchers", nil)
		h.listTmuxLaunchers(listW, listR)

		if listW.Code != http.StatusOK {
			t.Fatalf("list status = %d, want 200", listW.Code)
		}
		listBody := jsonBody(t, listW)
		listData, _ := listBody["data"].(map[string]any)
		launchers, _ := listData["launchers"].([]any)
		if len(launchers) != 1 {
			t.Fatalf("got %d launchers, want 1", len(launchers))
		}

		updateW := httptest.NewRecorder()
		updateR := httptest.NewRequest(http.MethodPatch, "/api/tmux/launchers/"+launcherID, strings.NewReader(`{"name":"Claude Code","icon":"bot","command":"claude","cwdMode":"fixed","cwdValue":"/srv/api","windowName":"claude"}`))
		updateR.SetPathValue("launcher", launcherID)
		h.updateTmuxLauncher(updateW, updateR)

		if updateW.Code != http.StatusOK {
			t.Fatalf("update status = %d, want 200", updateW.Code)
		}

		deleteW := httptest.NewRecorder()
		deleteR := httptest.NewRequest(http.MethodDelete, "/api/tmux/launchers/"+launcherID, nil)
		deleteR.SetPathValue("launcher", launcherID)
		h.deleteTmuxLauncher(deleteW, deleteR)

		if deleteW.Code != http.StatusNoContent {
			t.Fatalf("delete status = %d, want 204", deleteW.Code)
		}
	})

	t.Run("create validates icon and cwd", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/launchers", strings.NewReader(`{"name":"Bad","icon":"bad icon","command":"codex","cwdMode":"fixed","cwdValue":"relative"}`))
		h.createTmuxLauncher(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("create accepts blank command", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/launchers", strings.NewReader(`{"name":"Runner","icon":"terminal","command":"","cwdMode":"session","windowName":"runner"}`))
		h.createTmuxLauncher(w, r)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201", w.Code)
		}

		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		launcher, _ := data["launcher"].(map[string]any)
		if command, _ := launcher["command"].(string); command != "" {
			t.Fatalf("command = %q, want empty", command)
		}
	})

	t.Run("create returns invalid request for missing fixed cwd", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/launchers", strings.NewReader(`{"name":"Runner","icon":"terminal","command":"runner","cwdMode":"fixed","cwdValue":"","windowName":"runner"}`))
		h.createTmuxLauncher(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}

		body := jsonBody(t, w)
		errBody, _ := body["error"].(map[string]any)
		if message, _ := errBody["message"].(string); message != "tmux launcher fixed cwd is required" {
			t.Fatalf("error message = %q, want fixed cwd validation", message)
		}
	})

	t.Run("reorder launchers", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, &mockTmux{}, nil)
		first, err := st.CreateTmuxLauncher(context.Background(), store.TmuxLauncherWrite{
			Name:       "One",
			Icon:       "terminal",
			Command:    "one",
			CwdMode:    store.TmuxLauncherCwdModeSession,
			WindowName: "one",
		})
		if err != nil {
			t.Fatalf("CreateTmuxLauncher(first) error = %v", err)
		}
		second, err := st.CreateTmuxLauncher(context.Background(), store.TmuxLauncherWrite{
			Name:       "Two",
			Icon:       "terminal",
			Command:    "two",
			CwdMode:    store.TmuxLauncherCwdModeSession,
			WindowName: "two",
		})
		if err != nil {
			t.Fatalf("CreateTmuxLauncher(second) error = %v", err)
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/launchers/order", strings.NewReader(`{"ids":["`+second.ID+`","`+first.ID+`"]}`))
		h.reorderTmuxLaunchers(w, r)

		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", w.Code)
		}

		launchers, err := st.ListTmuxLaunchers(context.Background())
		if err != nil {
			t.Fatalf("ListTmuxLaunchers() error = %v", err)
		}
		if launchers[0].ID != second.ID || launchers[1].ID != first.ID {
			t.Fatalf("unexpected launcher order: %#v", launchers)
		}
	})

	t.Run("launch launcher creates window and sends command", func(t *testing.T) {
		t.Parallel()

		const sessionName = "dev"

		var (
			gotSession string
			gotName    string
			gotCWD     string
			gotPaneID  string
			gotKeys    string
			gotEnter   bool
		)
		tm := &mockTmux{
			listPanesFn: func(_ context.Context, session string) ([]tmux.Pane, error) {
				return []tmux.Pane{{
					Session:     session,
					WindowIndex: 0,
					PaneIndex:   0,
					PaneID:      "%7",
					Active:      true,
					CurrentPath: "/srv/api",
				}}, nil
			},
			newWindowWithOptionsFn: func(_ context.Context, session, name, cwd string) (tmux.NewWindowResult, error) {
				gotSession = session
				gotName = name
				gotCWD = cwd
				return tmux.NewWindowResult{Index: 2, PaneID: "%22"}, nil
			},
			sendKeysFn: func(_ context.Context, paneID, keys string, enter bool) error {
				gotPaneID = paneID
				gotKeys = keys
				gotEnter = enter
				return nil
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
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/"+sessionName+"/launchers/"+launcher.ID+"/launch", nil)
		r.SetPathValue("session", sessionName)
		r.SetPathValue("launcher", launcher.ID)
		h.launchTmuxLauncher(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if gotSession != sessionName || gotName != "codex" || gotCWD != "/srv/api" {
			t.Fatalf("launch args = (%q, %q, %q), want (dev, codex, /srv/api)", gotSession, gotName, gotCWD)
		}
		if gotPaneID != "%22" || gotKeys != "codex" || !gotEnter {
			t.Fatalf("send keys = (%q, %q, %v), want (%%22, codex, true)", gotPaneID, gotKeys, gotEnter)
		}

		stored, err := st.GetTmuxLauncher(context.Background(), launcher.ID)
		if err != nil {
			t.Fatalf("GetTmuxLauncher() error = %v", err)
		}
		if stored.LastUsedAt.IsZero() {
			t.Fatal("LastUsedAt is zero, want non-zero")
		}
	})

	t.Run("launch launcher with blank command skips send keys", func(t *testing.T) {
		t.Parallel()

		var sendKeysCalls int
		tm := &mockTmux{
			newWindowWithOptionsFn: func(_ context.Context, session, name, cwd string) (tmux.NewWindowResult, error) {
				return tmux.NewWindowResult{ID: "@12", Index: 2, PaneID: "%22"}, nil
			},
			sendKeysFn: func(_ context.Context, paneID, keys string, enter bool) error {
				sendKeysCalls++
				return nil
			},
		}
		h, st := newTestHandler(t, tm, nil)

		launcher, err := st.CreateTmuxLauncher(context.Background(), store.TmuxLauncherWrite{
			Name:       "Runner",
			Icon:       "terminal",
			Command:    "",
			CwdMode:    store.TmuxLauncherCwdModeSession,
			WindowName: "runner",
		})
		if err != nil {
			t.Fatalf("CreateTmuxLauncher() error = %v", err)
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/dev/launchers/"+launcher.ID+"/launch", nil)
		r.SetPathValue("session", "dev")
		r.SetPathValue("launcher", launcher.ID)
		h.launchTmuxLauncher(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if sendKeysCalls != 0 {
			t.Fatalf("SendKeys() calls = %d, want 0", sendKeysCalls)
		}
	})
}
