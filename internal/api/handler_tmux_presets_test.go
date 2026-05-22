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

	t.Run("create list update delete preset", testSessionPresetCreateListUpdateDelete)
	t.Run("create preset validates icon and cwd", testSessionPresetCreateValidatesIconAndCWD)
	t.Run("launch creates session and records launch", testSessionPresetLaunchCreatesSessionAndRecordsLaunch)
	t.Run("launch existing session opens pinned session", testSessionPresetLaunchExistingSession)
	t.Run("create session accepts optional icon", testSessionPresetCreateSessionAcceptsOptionalIcon)
}

func testSessionPresetCreateListUpdateDelete(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, &mockTmux{})

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
}

func testSessionPresetCreateValidatesIconAndCWD(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, &mockTmux{})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/tmux/session-presets", strings.NewReader(`{"name":"api","cwd":"relative","icon":"bad icon"}`))
	h.createSessionPreset(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func testSessionPresetLaunchCreatesSessionAndRecordsLaunch(t *testing.T) {
	t.Parallel()

	const botName = "bot"

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
	h, st := newTestHandler(t, tm)

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
}

func testSessionPresetLaunchExistingSession(t *testing.T) {
	t.Parallel()

	const (
		apiSession       = "api"
		apiSessionLaunch = "/api/tmux/session-presets/api/launch"
	)

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
	h, st := newTestHandler(t, tm)

	if _, err := st.CreateSessionPreset(context.Background(), store.SessionPresetWrite{
		Name: apiSession,
		Cwd:  "/srv/api",
		Icon: "server",
	}); err != nil {
		t.Fatalf("CreateSessionPreset() error = %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, apiSessionLaunch, nil)
	r.SetPathValue("preset", apiSession)
	h.launchSessionPreset(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	if got, _ := data["name"].(string); got != apiSession {
		t.Fatalf("name = %q, want api", got)
	}
	if created, _ := data["created"].(bool); created {
		t.Fatal("created = true, want false")
	}
	if len(attempts) != 1 || attempts[0] != apiSession {
		t.Fatalf("create attempts = %#v, want [api]", attempts)
	}
	presets, err := st.ListSessionPresets(context.Background())
	if err != nil {
		t.Fatalf("ListSessionPresets() error = %v", err)
	}
	if len(presets) != 1 || presets[0].Name != apiSession {
		t.Fatalf("presets = %#v, want api preset unchanged", presets)
	}
}

func testSessionPresetCreateSessionAcceptsOptionalIcon(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, &mockTmux{})
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
}
