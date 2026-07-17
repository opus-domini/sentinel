package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/opus-domini/sentinel/internal/events"
)

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

func TestMCPSettingsPersistAndChangeLiveState(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil)
	state := &fakeMCPSettings{tokenConfigured: true}
	h.mcpSettings = state
	configPath := t.TempDir() + "/config.toml"
	if err := os.WriteFile(configPath, []byte("[server]\ntoken = \"shared-token\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	h.configPath = configPath

	patchW := httptest.NewRecorder()
	patchR := httptest.NewRequest(http.MethodPatch, "/api/ops/settings/mcp", strings.NewReader(`{"enabled":true}`))
	h.patchMCPSettings(patchW, patchR)
	if patchW.Code != http.StatusOK {
		t.Fatalf("patchMCPSettings status = %d, want 200; body=%s", patchW.Code, patchW.Body.String())
	}
	if !state.enabled {
		t.Fatal("MCP live state remained disabled")
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var persisted struct {
		MCP struct {
			Enabled bool `toml:"enabled"`
		} `toml:"mcp"`
	}
	if err := toml.Unmarshal(content, &persisted); err != nil {
		t.Fatalf("parse persisted config: %v", err)
	}
	if !persisted.MCP.Enabled {
		t.Fatalf("persisted MCP setting is disabled:\n%s", content)
	}

	getW := httptest.NewRecorder()
	h.getMCPSettings(getW, httptest.NewRequest(http.MethodGet, "/api/ops/settings/mcp", nil))
	if getW.Code != http.StatusOK || !strings.Contains(getW.Body.String(), `"endpoint":"/mcp"`) {
		t.Fatalf("getMCPSettings response = %d %s", getW.Code, getW.Body.String())
	}
}

func TestMCPSettingsRejectEnableWithoutSharedToken(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil)
	h.mcpSettings = &fakeMCPSettings{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPatch, "/api/ops/settings/mcp", strings.NewReader(`{"enabled":true}`))
	h.patchMCPSettings(w, r)

	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "MCP_TOKEN_REQUIRED") {
		t.Fatalf("patchMCPSettings response = %d %s", w.Code, w.Body.String())
	}
}

type fakeMCPSettings struct {
	enabled         bool
	tokenConfigured bool
}

func (s *fakeMCPSettings) Enabled() bool { return s.enabled }

func (s *fakeMCPSettings) TokenConfigured() bool { return s.tokenConfigured }

func (s *fakeMCPSettings) SetEnabled(enabled bool) error {
	s.enabled = enabled
	return nil
}

func TestSettingsHandlersSerializeConcurrentWrites(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, nil)
	h.events = events.NewHub()
	configPath := t.TempDir() + "/config.toml"
	if err := os.WriteFile(configPath, []byte("[server]\nlocale = \"en-US\"\n"), 0o600); err != nil {
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
	wg.Add(2)
	go fire(&wg, "/api/ops/settings/timezone", `{"timezone":"America/Sao_Paulo"}`, h.patchTimezone)
	go fire(&wg, "/api/ops/settings/locale", `{"locale":"pt-BR"}`, h.patchLocale)
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
	for _, want := range []string{`timezone = "America/Sao_Paulo"`, `locale = "pt-BR"`} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("config missing %q after concurrent writes:\n%s", want, content)
		}
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
