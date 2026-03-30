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

func TestSessionPresetHandlers(t *testing.T) {
	t.Parallel()

	const botName = "bot"

	t.Run("create list update delete preset", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)

		createW := httptest.NewRecorder()
		createR := httptest.NewRequest(http.MethodPost, "/api/tmux/session-presets", strings.NewReader(`{"name":"api","cwd":"/srv/api","icon":"server"}`))
		h.createSessionPreset(createW, createR)

		if createW.Code != http.StatusCreated {
			t.Fatalf("create status = %d, want 201", createW.Code)
		}

		listW := httptest.NewRecorder()
		listR := httptest.NewRequest(http.MethodGet, "/api/tmux/session-presets", nil)
		h.listSessionPresets(listW, listR)

		if listW.Code != http.StatusOK {
			t.Fatalf("list status = %d, want 200", listW.Code)
		}
		body := jsonBody(t, listW)
		data, _ := body["data"].(map[string]any)
		presets, _ := data["presets"].([]any)
		if len(presets) != 1 {
			t.Fatalf("got %d presets, want 1", len(presets))
		}

		updateW := httptest.NewRecorder()
		updateR := httptest.NewRequest(http.MethodPatch, "/api/tmux/session-presets/api", strings.NewReader(`{"name":"web","cwd":"/srv/web","icon":"globe"}`))
		updateR.SetPathValue("preset", "api")
		h.updateSessionPreset(updateW, updateR)

		if updateW.Code != http.StatusOK {
			t.Fatalf("update status = %d, want 200", updateW.Code)
		}

		deleteW := httptest.NewRecorder()
		deleteR := httptest.NewRequest(http.MethodDelete, "/api/tmux/session-presets/web", nil)
		deleteR.SetPathValue("preset", "web")
		h.deleteSessionPreset(deleteW, deleteR)

		if deleteW.Code != http.StatusNoContent {
			t.Fatalf("delete status = %d, want 204", deleteW.Code)
		}
	})

	t.Run("create preset validates icon and cwd", func(t *testing.T) {
		t.Parallel()

		h, _ := newTestHandler(t, &mockTmux{}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/session-presets", strings.NewReader(`{"name":"api","cwd":"relative","icon":"bad icon"}`))
		h.createSessionPreset(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("launch creates session and records launch", func(t *testing.T) {
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

		if _, err := st.CreateSessionPreset(context.Background(), store.SessionPresetWrite{
			Name: botName,
			Cwd:  "/srv/bot",
			Icon: "bot",
		}); err != nil {
			t.Fatalf("CreateSessionPreset() error = %v", err)
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/session-presets/bot/launch", nil)
		r.SetPathValue("preset", botName)
		h.launchSessionPreset(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if gotName != botName {
			t.Fatalf("gotName = %q, want %s", gotName, botName)
		}
		if gotCwd != "/srv/bot" {
			t.Fatalf("gotCwd = %q, want /srv/bot", gotCwd)
		}

		meta, err := st.GetAll(context.Background())
		if err != nil {
			t.Fatalf("GetAll() error = %v", err)
		}
		if meta[botName].Icon != "bot" {
			t.Fatalf("meta[%s].Icon = %q, want bot", botName, meta[botName].Icon)
		}

		presets, err := st.ListSessionPresets(context.Background())
		if err != nil {
			t.Fatalf("ListSessionPresets() error = %v", err)
		}
		if len(presets) != 1 {
			t.Fatalf("got %d presets, want 1", len(presets))
		}
		if presets[0].LaunchCount != 1 {
			t.Fatalf("LaunchCount = %d, want 1", presets[0].LaunchCount)
		}
	})

	t.Run("launch existing session returns created false", func(t *testing.T) {
		t.Parallel()

		tm := &mockTmux{
			createSessionFn: func(_ context.Context, _, _ string) error {
				return &tmux.Error{Kind: tmux.ErrKindSessionExists}
			},
		}
		h, st := newTestHandler(t, tm, nil)

		if _, err := st.CreateSessionPreset(context.Background(), store.SessionPresetWrite{
			Name: "api",
			Cwd:  "/srv/api",
			Icon: "server",
		}); err != nil {
			t.Fatalf("CreateSessionPreset() error = %v", err)
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/session-presets/api/launch", nil)
		r.SetPathValue("preset", "api")
		h.launchSessionPreset(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := jsonBody(t, w)
		data, _ := body["data"].(map[string]any)
		if created, _ := data["created"].(bool); created {
			t.Fatal("created = true, want false")
		}
	})

	t.Run("create session accepts optional icon", func(t *testing.T) {
		t.Parallel()

		h, st := newTestHandler(t, &mockTmux{}, nil)
		if err := st.UpsertSession(context.Background(), "older", "hash-older", "ready"); err != nil {
			t.Fatalf("UpsertSession(older) error = %v", err)
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", strings.NewReader(`{"name":"dev","cwd":"/tmp","icon":"code"}`))
		h.createSession(w, r)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201", w.Code)
		}

		meta, err := st.GetAll(context.Background())
		if err != nil {
			t.Fatalf("GetAll() error = %v", err)
		}
		if meta["dev"].Icon != "code" {
			t.Fatalf("meta[dev].Icon = %q, want code", meta["dev"].Icon)
		}
		if meta["dev"].SortOrder >= meta["older"].SortOrder {
			t.Fatalf("dev sort_order = %d, older sort_order = %d, want dev before older", meta["dev"].SortOrder, meta["older"].SortOrder)
		}
	})
}
