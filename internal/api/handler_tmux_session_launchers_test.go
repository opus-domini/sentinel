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

const (
	apiSession    = "api"
	apiSessionCwd = "/srv/api"
)

func TestSessionLauncherHandlers(t *testing.T) {
	t.Parallel()

	t.Run("create list update delete launcher", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)

		createW := httptest.NewRecorder()
		createR := httptest.NewRequest(http.MethodPost, "/api/tmux/session-launchers", strings.NewReader(`{"name":"api","cwd":"/srv/api","icon":"server"}`))
		h.createSessionLauncher(createW, createR)

		if createW.Code != http.StatusCreated {
			t.Fatalf("create status = %d, want 201", createW.Code)
		}
		createBody := jsonBody(t, createW)
		createData, _ := createBody["data"].(map[string]any)
		created, _ := createData["launcher"].(map[string]any)
		launcherID, _ := created["id"].(string)
		if launcherID == "" {
			t.Fatal("launcher id is empty")
		}

		listW := httptest.NewRecorder()
		listR := httptest.NewRequest(http.MethodGet, "/api/tmux/session-launchers", nil)
		h.listSessionLaunchers(listW, listR)

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
		updateR := httptest.NewRequest(http.MethodPatch, "/api/tmux/session-launchers/"+launcherID, strings.NewReader(`{"name":"web","cwd":"/srv/web","icon":"globe"}`))
		updateR.SetPathValue("launcher", launcherID)
		h.updateSessionLauncher(updateW, updateR)

		if updateW.Code != http.StatusOK {
			t.Fatalf("update status = %d, want 200", updateW.Code)
		}

		deleteW := httptest.NewRecorder()
		deleteR := httptest.NewRequest(http.MethodDelete, "/api/tmux/session-launchers/"+launcherID, nil)
		deleteR.SetPathValue("launcher", launcherID)
		h.deleteSessionLauncher(deleteW, deleteR)

		if deleteW.Code != http.StatusNoContent {
			t.Fatalf("delete status = %d, want 204", deleteW.Code)
		}
	})

	t.Run("reorder launchers", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, &mockTmux{}, nil)
		ctx := context.Background()
		first, err := st.CreateSessionLauncher(ctx, store.SessionLauncherWrite{
			Name: apiSession,
			Cwd:  apiSessionCwd,
			Icon: "server",
		})
		if err != nil {
			t.Fatalf("CreateSessionLauncher(first) error = %v", err)
		}
		second, err := st.CreateSessionLauncher(ctx, store.SessionLauncherWrite{
			Name: "web",
			Cwd:  "/srv/web",
			Icon: "globe",
		})
		if err != nil {
			t.Fatalf("CreateSessionLauncher(second) error = %v", err)
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/api/tmux/session-launchers/order", strings.NewReader(`{"ids":["`+second.ID+`","`+first.ID+`"]}`))
		h.reorderSessionLaunchers(w, r)

		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", w.Code)
		}

		launchers, err := st.ListSessionLaunchers(ctx)
		if err != nil {
			t.Fatalf("ListSessionLaunchers() error = %v", err)
		}
		if launchers[0].ID != second.ID || launchers[1].ID != first.ID {
			t.Fatalf("unexpected launcher order: %#v", launchers)
		}
	})

	t.Run("launch creates session and records usage", func(t *testing.T) {
		t.Parallel()

		var (
			gotName string
			gotCwd  string
		)
		tm := &mockTmux{
			createSessionFn: func(_ context.Context, name, cwd string) error {
				gotName = name
				gotCwd = cwd
				return nil
			},
		}
		h, st := newTestHandler(t, tm, nil)
		launcher, err := st.CreateSessionLauncher(context.Background(), store.SessionLauncherWrite{
			Name: apiSession,
			Cwd:  apiSessionCwd,
			Icon: "server",
		})
		if err != nil {
			t.Fatalf("CreateSessionLauncher() error = %v", err)
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/session-launchers/"+launcher.ID+"/launch", nil)
		r.SetPathValue("launcher", launcher.ID)
		h.launchSessionLauncher(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if gotName != apiSession || gotCwd != apiSessionCwd {
			t.Fatalf("launch args = (%q, %q), want (api, /srv/api)", gotName, gotCwd)
		}

		stored, err := st.GetSessionLauncher(context.Background(), launcher.ID)
		if err != nil {
			t.Fatalf("GetSessionLauncher() error = %v", err)
		}
		if stored.UseCount != 1 || stored.LastUsedAt.IsZero() {
			t.Fatalf("used launcher = %#v, want use count and timestamp", stored)
		}
	})

	t.Run("launch existing session creates numbered session", func(t *testing.T) {
		t.Parallel()

		var attempts []string
		tm := &mockTmux{
			createSessionFn: func(_ context.Context, name, _ string) error {
				attempts = append(attempts, name)
				if name == apiSession {
					return &tmux.Error{Kind: tmux.ErrKindSessionExists}
				}
				return nil
			},
		}
		h, st := newTestHandler(t, tm, nil)
		launcher, err := st.CreateSessionLauncher(context.Background(), store.SessionLauncherWrite{
			Name: apiSession,
			Cwd:  apiSessionCwd,
			Icon: "server",
		})
		if err != nil {
			t.Fatalf("CreateSessionLauncher() error = %v", err)
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/session-launchers/"+launcher.ID+"/launch", nil)
		r.SetPathValue("launcher", launcher.ID)
		h.launchSessionLauncher(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		if got, _ := data["name"].(string); got != "api-1" {
			t.Fatalf("name = %q, want api-1", got)
		}
		if len(attempts) != 2 || attempts[0] != apiSession || attempts[1] != "api-1" {
			t.Fatalf("create attempts = %#v, want [api api-1]", attempts)
		}
	})

	t.Run("killing launched session keeps launcher", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, &mockTmux{}, nil)
		ctx := context.Background()
		launcher, err := st.CreateSessionLauncher(ctx, store.SessionLauncherWrite{
			Name: apiSession,
			Cwd:  apiSessionCwd,
			Icon: "server",
		})
		if err != nil {
			t.Fatalf("CreateSessionLauncher() error = %v", err)
		}

		launchW := httptest.NewRecorder()
		launchR := httptest.NewRequest(http.MethodPost, "/api/tmux/session-launchers/"+launcher.ID+"/launch", nil)
		launchR.SetPathValue("launcher", launcher.ID)
		h.launchSessionLauncher(launchW, launchR)
		if launchW.Code != http.StatusOK {
			t.Fatalf("launch status = %d, want 200", launchW.Code)
		}

		deleteW := httptest.NewRecorder()
		deleteR := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/api", nil)
		deleteR.SetPathValue("session", apiSession)
		h.deleteSession(deleteW, deleteR)
		if deleteW.Code != http.StatusNoContent {
			t.Fatalf("delete status = %d, want 204", deleteW.Code)
		}

		if _, err := st.GetSessionLauncher(ctx, launcher.ID); err != nil {
			t.Fatalf("GetSessionLauncher() error = %v, want launcher preserved", err)
		}
	})
}
