package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/opus-domini/sentinel/internal/events"
)

func TestMarkerPatternHandlers(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil)

	listEmptyW := httptest.NewRecorder()
	h.listMarkerPatterns(listEmptyW, httptest.NewRequest(http.MethodGet, "/api/ops/markers", nil))
	if listEmptyW.Code != http.StatusOK {
		t.Fatalf("list empty status = %d, want 200; body=%s", listEmptyW.Code, listEmptyW.Body.String())
	}

	upsertW := httptest.NewRecorder()
	upsertR := httptest.NewRequest(http.MethodPut, "/api/ops/markers/build-failed", strings.NewReader(`{
		"pattern":"build failed",
		"severity":"error",
		"label":"Build failed",
		"enabled":true,
		"priority":10
	}`))
	upsertR.SetPathValue("pattern", "build-failed")
	h.upsertMarkerPattern(upsertW, upsertR)
	if upsertW.Code != http.StatusOK {
		t.Fatalf("upsert status = %d, want 200; body=%s", upsertW.Code, upsertW.Body.String())
	}

	listW := httptest.NewRecorder()
	h.listMarkerPatterns(listW, httptest.NewRequest(http.MethodGet, "/api/ops/markers", nil))
	body := jsonBody(t, listW)
	data, _ := body["data"].(map[string]any)
	patterns, _ := data["patterns"].([]any)
	found := false
	for _, item := range patterns {
		pattern, _ := item.(map[string]any)
		if pattern["id"] == "build-failed" && pattern["pattern"] == "build failed" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("custom pattern not found; body=%s", listW.Body.String())
	}

	deleteW := httptest.NewRecorder()
	deleteR := httptest.NewRequest(http.MethodDelete, "/api/ops/markers/build-failed", nil)
	deleteR.SetPathValue("pattern", "build-failed")
	h.deleteMarkerPattern(deleteW, deleteR)
	if deleteW.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200; body=%s", deleteW.Code, deleteW.Body.String())
	}
}

func TestMarkerPatternHandlersValidateInput(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/ops/markers/bad", strings.NewReader(`{"pattern":"","enabled":true}`))
	r.SetPathValue("pattern", "bad")
	h.upsertMarkerPattern(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing pattern status = %d, want 400", w.Code)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPut, "/api/ops/markers/bad", strings.NewReader(`{"pattern":"error"}`))
	r.SetPathValue("pattern", "bad")
	h.upsertMarkerPattern(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing enabled status = %d, want 400", w.Code)
	}
}

func TestSettingsHandlersPersistConfig(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil)
	h.events = events.NewHub()
	configPath := t.TempDir() + "/config.toml"
	if err := os.WriteFile(configPath, []byte("[server]\n# timezone = \"UTC\"\nlocale = \"en-US\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	h.configPath = configPath

	timezoneW := httptest.NewRecorder()
	timezoneR := httptest.NewRequest(http.MethodPatch, "/api/ops/settings/timezone", strings.NewReader(`{"timezone":"America/Sao_Paulo"}`))
	h.patchTimezone(timezoneW, timezoneR)
	if timezoneW.Code != http.StatusOK {
		t.Fatalf("patchTimezone status = %d, want 200; body=%s", timezoneW.Code, timezoneW.Body.String())
	}
	if h.timezone != "America/Sao_Paulo" {
		t.Fatalf("handler timezone = %q, want America/Sao_Paulo", h.timezone)
	}

	localeW := httptest.NewRecorder()
	localeR := httptest.NewRequest(http.MethodPatch, "/api/ops/settings/locale", strings.NewReader(`{"locale":"pt-BR"}`))
	h.patchLocale(localeW, localeR)
	if localeW.Code != http.StatusOK {
		t.Fatalf("patchLocale status = %d, want 200; body=%s", localeW.Code, localeW.Body.String())
	}
	if h.locale != "pt-BR" {
		t.Fatalf("handler locale = %q, want pt-BR", h.locale)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	got := string(content)
	for _, want := range []string{`timezone = "America/Sao_Paulo"`, `locale = "pt-BR"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("config missing %q:\n%s", want, got)
		}
	}
}

func TestSettingsHandlersSerializeConcurrentWrites(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil)
	h.events = events.NewHub()
	configPath := t.TempDir() + "/config.toml"
	if err := os.WriteFile(configPath, []byte("[server]\nlocale = \"en-US\"\n\n[alerts]\nwebhook_url = \"\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	h.configPath = configPath

	const rounds = 25
	fire := func(wg *sync.WaitGroup, path, body string, fn func(http.ResponseWriter, *http.Request)) {
		defer wg.Done()
		for range rounds {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPatch, path, strings.NewReader(body))
			fn(rec, req)
		}
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go fire(&wg, "/api/ops/settings/timezone", `{"timezone":"America/Sao_Paulo"}`, h.patchTimezone)
	go fire(&wg, "/api/ops/settings/locale", `{"locale":"pt-BR"}`, h.patchLocale)
	go fire(&wg, "/api/ops/settings/webhook", `{"url":"https://example.com/hook","events":["alert.created"]}`, h.patchWebhookSettings)
	wg.Wait()

	// Without configMu serialization the interleaved read-modify-write cycles
	// produce a torn config.toml. It must still parse and reflect every writer.
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]any
	if err := toml.Unmarshal(content, &cfg); err != nil {
		t.Fatalf("config corrupted by concurrent writes: %v\n%s", err, content)
	}
	for _, want := range []string{`timezone = "America/Sao_Paulo"`, `locale = "pt-BR"`, `webhook_url = "https://example.com/hook"`} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("config missing %q after concurrent writes:\n%s", want, content)
		}
	}
}

func TestWebhookSettingsHandlers(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil)
	configPath := t.TempDir() + "/config.toml"
	if err := os.WriteFile(configPath, []byte("[alerts]\nwebhook_url = \"\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	h.configPath = configPath

	patchW := httptest.NewRecorder()
	patchR := httptest.NewRequest(http.MethodPatch, "/api/ops/settings/webhook", strings.NewReader(`{
		"url":"https://example.com/hook",
		"events":["alert.acked","alert.created","alert.acked"]
	}`))
	h.patchWebhookSettings(patchW, patchR)
	if patchW.Code != http.StatusOK {
		t.Fatalf("patchWebhookSettings status = %d, want 200; body=%s", patchW.Code, patchW.Body.String())
	}

	getW := httptest.NewRecorder()
	h.getWebhookSettings(getW, httptest.NewRequest(http.MethodGet, "/api/ops/settings/webhook", nil))
	if getW.Code != http.StatusOK {
		t.Fatalf("getWebhookSettings status = %d, want 200; body=%s", getW.Code, getW.Body.String())
	}
	body := jsonBody(t, getW)
	data, _ := body["data"].(map[string]any)
	if data["url"] != "https://example.com/hook" {
		t.Fatalf("webhook url = %#v, want configured URL", data["url"])
	}
	events, _ := data["events"].([]any)
	if len(events) != 2 {
		t.Fatalf("events len = %d, want deduped 2; body=%s", len(events), getW.Body.String())
	}
}

func TestUpsertConfigKeyQuotesValues(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/config.toml"
	if err := os.WriteFile(path, []byte("[server]\n# locale = \"en-US\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := upsertConfigString(path, "server", "locale", `pt-"BR"\test`); err != nil {
		t.Fatalf("upsertConfigString: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(content), `locale = "pt-\"BR\"\\test"`) {
		t.Fatalf("config did not quote value safely:\n%s", string(content))
	}
}

func TestPatchTimezoneRejectsInvalidTimezone(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPatch, "/api/ops/settings/timezone", strings.NewReader(`{"timezone":"Not/AZone"}`))
	h.patchTimezone(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("patchTimezone invalid status = %d, want 400", w.Code)
	}
}

func TestGetWebhookSettingsWithNilNotifier(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil)
	w := httptest.NewRecorder()
	h.getWebhookSettings(w, httptest.NewRequest(http.MethodGet, "/api/ops/settings/webhook", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("getWebhookSettings status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	if data["url"] != "" {
		t.Fatalf("nil notifier url = %#v, want empty", data["url"])
	}
}

func TestTestWebhookDeliversPayload(t *testing.T) {
	t.Parallel()

	var received atomic.Bool
	var receivedBody atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody.Store(string(body))
		received.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	h, _ := newTestHandler(t, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ops/settings/webhook/test", strings.NewReader(`{"url":"`+server.URL+`"}`))
	h.testWebhook(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("testWebhook status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !received.Load() {
		t.Fatal("test webhook server did not receive payload")
	}
	body, _ := receivedBody.Load().(string)
	if !strings.Contains(body, "alert.test") {
		t.Fatalf("test webhook body = %s, want alert.test", body)
	}
}

func TestMarkerPatternGeneratedID(t *testing.T) {
	t.Parallel()

	h, st := newTestHandler(t, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/ops/markers", strings.NewReader(`{"pattern":"panic","enabled":true}`))
	h.upsertMarkerPattern(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("upsert generated id status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	patterns, err := st.ListMarkerPatterns(context.Background())
	if err != nil {
		t.Fatalf("ListMarkerPatterns: %v", err)
	}
	found := false
	for _, pattern := range patterns {
		if !strings.HasPrefix(pattern.ID, "builtin.") && pattern.Pattern == "panic" && pattern.ID != "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("patterns = %#v, want one generated custom id", patterns)
	}
}

func TestFrequentDirectories(t *testing.T) {
	t.Parallel()

	const apiDir = "/srv/api"

	h, st := newTestHandler(t, nil)
	ctx := context.Background()
	for _, dir := range []string{apiDir, "/srv/web", apiDir} {
		if err := st.RecordSessionDirectory(ctx, dir); err != nil {
			t.Fatalf("RecordSessionDirectory(%s): %v", dir, err)
		}
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/tmux/frequent-directories?limit=1", nil)
	h.frequentDirectories(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("frequentDirectories status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := jsonBody(t, w)
	data, _ := body["data"].(map[string]any)
	dirs, _ := data["dirs"].([]any)
	if len(dirs) != 1 || dirs[0] != apiDir {
		t.Fatalf("dirs = %#v, want [/srv/api]", dirs)
	}
}
