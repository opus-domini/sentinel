package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/user"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/daemon"
	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/userswitch"
)

type settingsEnvelope struct {
	Data settingsResponse `json:"data"`
}

func TestSettingsGetIsTypedRevisionedAndSecretSafe(t *testing.T) {
	content := `[server]
token = "shared-secret"
locale = "pt-BR"

[health_report]
webhook_url = "https://hooks.example.test/private-secret"
schedule = "0 * * * *"
`
	service, runtime := newTestSettings(t, &content)
	h, _ := newTestHandler(t, nil)
	h.version = "test-version"
	h.configService = service
	h.settings = runtime
	h.deployments = func() ([]daemon.Deployment, error) {
		return []daemon.Deployment{{
			Scope:      daemon.ScopeUser,
			ConfigPath: service.Path(),
		}}, nil
	}

	rec := httptest.NewRecorder()
	h.getSettings(rec, httptest.NewRequest(http.MethodGet, "/api/ops/settings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body settingsEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if got := rec.Header().Get("ETag"); got != quoteETag(body.Data.Revision) {
		t.Fatalf("ETag = %q, revision = %q", got, body.Data.Revision)
	}
	if body.Data.Metadata.Version != "test-version" {
		t.Fatalf("version = %q", body.Data.Metadata.Version)
	}
	if body.Data.Deployment.Scope != daemon.ScopeUser ||
		body.Data.Deployment.RuntimeMode != "service" {
		t.Fatalf("deployment = %+v", body.Data.Deployment)
	}
	if body.Data.Experience.Locale.Source != config.FieldSourceFile ||
		body.Data.Experience.Timezone.Source != config.FieldSourceDefault {
		t.Fatalf("experience sources = %+v", body.Data.Experience)
	}
	if body.Data.Experience.Timezone.ApplyMode != config.ApplyModePartial ||
		body.Data.Integrations.MCP.Enabled.ApplyMode != config.ApplyModeLive {
		t.Fatalf("apply modes = timezone:%q mcp:%q",
			body.Data.Experience.Timezone.ApplyMode,
			body.Data.Integrations.MCP.Enabled.ApplyMode,
		)
	}
	if !body.Data.Integrations.MCP.Token.Configured ||
		!body.Data.Integrations.MCP.RuntimeTokenConfigured {
		t.Fatal("configured shared token was not reported")
	}
	if !body.Data.Integrations.HealthReport.WebhookURL.Configured ||
		body.Data.Integrations.HealthReport.NextActivation == "" {
		t.Fatalf("health report = %+v", body.Data.Integrations.HealthReport)
	}
	if strings.Contains(rec.Body.String(), "shared-secret") ||
		strings.Contains(rec.Body.String(), "private-secret") ||
		strings.Contains(rec.Body.String(), "webhook_url") ||
		strings.Contains(rec.Body.String(), "[server]") {
		t.Fatalf("settings response leaked a secret or raw TOML: %s", rec.Body.String())
	}
}

func TestSettingsPatchRequiresCurrentRevisionAndPreservesConflictWinner(t *testing.T) {
	content := "[server]\ntimezone = \"UTC\"\nlocale = \"en-US\"\n"
	service, runtime := newTestSettings(t, &content)
	h, _ := newTestHandler(t, nil)
	h.configService = service
	h.settings = runtime

	missing := httptest.NewRecorder()
	h.patchSettings(
		missing,
		httptest.NewRequest(http.MethodPatch, "/api/ops/settings", strings.NewReader(`{"experience":{"locale":"pt-BR"}}`)),
	)
	if missing.Code != http.StatusPreconditionRequired ||
		!strings.Contains(missing.Body.String(), "REVISION_REQUIRED") {
		t.Fatalf("missing If-Match = %d %s", missing.Code, missing.Body.String())
	}

	etag := getSettingsETag(t, h)
	first := patchSettingsRequestWithETag(
		t,
		h,
		etag,
		`{"experience":{"timezone":"America/Sao_Paulo","locale":"pt-BR"}}`,
	)
	if first.Code != http.StatusOK {
		t.Fatalf("first patch = %d %s", first.Code, first.Body.String())
	}
	if runtime.Timezone() != "America/Sao_Paulo" || runtime.Locale() != "pt-BR" {
		t.Fatalf("live settings = timezone:%q locale:%q", runtime.Timezone(), runtime.Locale())
	}
	var firstBody settingsEnvelope
	if err := json.Unmarshal(first.Body.Bytes(), &firstBody); err != nil {
		t.Fatal(err)
	}
	if !firstBody.Data.Restart.Required ||
		!slicesContain(firstBody.Data.Restart.ChangedKeys, config.FieldServerTimezone) ||
		firstBody.Data.Experience.Locale.RestartPending {
		t.Fatalf("restart state = %+v", firstBody.Data.Restart)
	}
	winner, err := os.ReadFile(service.Path())
	if err != nil {
		t.Fatal(err)
	}

	stale := patchSettingsRequestWithETag(
		t,
		h,
		etag,
		`{"experience":{"locale":"en-GB"}}`,
	)
	if stale.Code != http.StatusPreconditionFailed ||
		!strings.Contains(stale.Body.String(), "CONFIG_CONFLICT") {
		t.Fatalf("stale patch = %d %s", stale.Code, stale.Body.String())
	}
	afterConflict, err := os.ReadFile(service.Path())
	if err != nil {
		t.Fatal(err)
	}
	if string(afterConflict) != string(winner) {
		t.Fatal("stale patch changed the winning config")
	}
}

func TestSettingsPatchRejectsEnvironmentOwnershipAndInvalidFields(t *testing.T) {
	t.Setenv("SENTINEL_SERVER_LOCALE", "pt-BR")
	content := "[server]\nlocale = \"en-US\"\n"
	service, runtime := newTestSettings(t, &content)
	h, _ := newTestHandler(t, nil)
	h.configService = service
	h.settings = runtime
	original, err := os.ReadFile(service.Path())
	if err != nil {
		t.Fatal(err)
	}

	owned := patchSettingsRequestWithETag(
		t,
		h,
		getSettingsETag(t, h),
		`{"experience":{"locale":"en-GB"}}`,
	)
	if owned.Code != http.StatusConflict ||
		!strings.Contains(owned.Body.String(), "ENVIRONMENT_OWNED") ||
		!strings.Contains(owned.Body.String(), config.FieldServerLocale) {
		t.Fatalf("environment-owned patch = %d %s", owned.Code, owned.Body.String())
	}
	afterOwned, err := os.ReadFile(service.Path())
	if err != nil {
		t.Fatal(err)
	}
	if string(afterOwned) != string(original) {
		t.Fatal("environment-owned patch changed config")
	}

	invalid := patchSettingsRequestWithETag(
		t,
		h,
		getSettingsETag(t, h),
		`{"experience":{"timezone":"Not/AZone"}}`,
	)
	if invalid.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(invalid.Body.String(), "CONFIG_INVALID") ||
		!strings.Contains(invalid.Body.String(), "server.timezone") {
		t.Fatalf("invalid patch = %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestSettingsOperationsExposeConstraintsProvenanceAndRestartLifecycle(t *testing.T) {
	t.Setenv("SENTINEL_LOG_LEVEL", "debug")
	content := `[watchtower]
enabled = false
tick_interval = "2s"
capture_lines = 120
capture_timeout = "250ms"
journal_rows = 8000

[runbooks]
max_concurrent = 7

[log]
level = "warn"
`
	service, runtime := newTestSettings(t, &content)
	h, _ := newTestHandler(t, nil)
	h.configService = service
	h.settings = runtime

	rec := httptest.NewRecorder()
	h.getSettings(rec, httptest.NewRequest(http.MethodGet, "/api/ops/settings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET settings = %d %s", rec.Code, rec.Body.String())
	}
	var body settingsEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	operations := body.Data.Operations
	if operations.Watchtower.Enabled.EffectiveValue ||
		operations.Watchtower.TickInterval.EffectiveValue != "2s" ||
		operations.Watchtower.CaptureLines.EffectiveValue != 120 ||
		operations.Watchtower.CaptureTimeout.EffectiveValue != "250ms" ||
		operations.Watchtower.JournalRows.EffectiveValue != 8000 ||
		operations.Runbooks.MaxConcurrent.EffectiveValue != 7 {
		t.Fatalf("operations = %+v", operations)
	}
	if operations.Watchtower.TickInterval.Validation.Min != "100ms" ||
		operations.Watchtower.TickInterval.Validation.Max != "1m" ||
		operations.Watchtower.CaptureLines.Validation.Min != 1 ||
		operations.Watchtower.CaptureLines.Validation.Max != 2000 ||
		operations.Runbooks.MaxConcurrent.Validation.Min != 1 ||
		operations.Runbooks.MaxConcurrent.Validation.Max != 64 {
		t.Fatalf("operation constraints = %+v", operations)
	}
	if operations.Log.Level.Source != config.FieldSourceEnvironment ||
		operations.Log.Level.Editable ||
		operations.Log.Level.EffectiveValue != "debug" {
		t.Fatalf("environment-owned log level = %+v", operations.Log.Level)
	}
	if operations.Watchtower.TickInterval.ApplyMode != config.ApplyModeRestart ||
		operations.Runbooks.MaxConcurrent.ApplyMode != config.ApplyModeRestart {
		t.Fatalf("operation apply modes = %+v", operations)
	}
}

func TestSettingsOperationsPatchPersistsTypedDraftAndRejectsRanges(t *testing.T) {
	service, runtime := newTestSettings(t, nil)
	h, _ := newTestHandler(t, nil)
	h.configService = service
	h.settings = runtime

	rec := patchSettingsRequestWithETag(
		t,
		h,
		getSettingsETag(t, h),
		`{"operations":{"watchtower":{"enabled":false,"tickInterval":"3s","captureLines":140,"captureTimeout":"300ms","journalRows":9000},"runbooks":{"maxConcurrent":8},"log":{"level":"warn"}}}`,
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("operations patch = %d %s", rec.Code, rec.Body.String())
	}
	var body settingsEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	wantChanged := []string{
		config.FieldLogLevel,
		config.FieldRunbooksMaxConcurrent,
		config.FieldWatchtowerCaptureLines,
		config.FieldWatchtowerCaptureTimeout,
		config.FieldWatchtowerEnabled,
		config.FieldWatchtowerJournalRows,
		config.FieldWatchtowerTickInterval,
	}
	for _, key := range wantChanged {
		if !slicesContain(body.Data.Restart.ChangedKeys, key) {
			t.Fatalf("restart changed keys = %v, missing %q", body.Data.Restart.ChangedKeys, key)
		}
	}
	state, err := service.Read()
	if err != nil {
		t.Fatal(err)
	}
	if state.Persisted.Watchtower.Enabled ||
		state.Persisted.Watchtower.TickInterval != 3*time.Second ||
		state.Persisted.Watchtower.CaptureLines != 140 ||
		state.Persisted.Watchtower.CaptureTimeout != 300*time.Millisecond ||
		state.Persisted.Watchtower.JournalRows != 9000 ||
		state.Persisted.Runbooks.MaxConcurrent != 8 ||
		state.Persisted.Log.Level != "warn" {
		t.Fatalf("persisted operations = %+v", state.Persisted)
	}

	for _, test := range []struct {
		name string
		body string
		key  string
	}{
		{
			name: "tick interval below minimum",
			body: `{"operations":{"watchtower":{"tickInterval":"99ms"}}}`,
			key:  config.FieldWatchtowerTickInterval,
		},
		{
			name: "capture lines above maximum",
			body: `{"operations":{"watchtower":{"captureLines":2001}}}`,
			key:  config.FieldWatchtowerCaptureLines,
		},
		{
			name: "capture timeout malformed",
			body: `{"operations":{"watchtower":{"captureTimeout":"soon"}}}`,
			key:  config.FieldWatchtowerCaptureTimeout,
		},
		{
			name: "journal rows below minimum",
			body: `{"operations":{"watchtower":{"journalRows":99}}}`,
			key:  config.FieldWatchtowerJournalRows,
		},
		{
			name: "runbook concurrency above maximum",
			body: `{"operations":{"runbooks":{"maxConcurrent":65}}}`,
			key:  config.FieldRunbooksMaxConcurrent,
		},
		{
			name: "unsupported log level",
			body: `{"operations":{"log":{"level":"verbose"}}}`,
			key:  config.FieldLogLevel,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			before, readErr := os.ReadFile(service.Path())
			if readErr != nil {
				t.Fatal(readErr)
			}
			invalid := patchSettingsRequestWithETag(t, h, getSettingsETag(t, h), test.body)
			if invalid.Code != http.StatusUnprocessableEntity ||
				!strings.Contains(invalid.Body.String(), "CONFIG_INVALID") ||
				!strings.Contains(invalid.Body.String(), test.key) {
				t.Fatalf("invalid operations patch = %d %s", invalid.Code, invalid.Body.String())
			}
			after, readErr := os.ReadFile(service.Path())
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(after) != string(before) {
				t.Fatal("invalid operations patch changed config")
			}
		})
	}
}

func TestSettingsMCPRequiresTokenAndUsesRuntimeCapability(t *testing.T) {
	t.Run("missing token", func(t *testing.T) {
		service, runtime := newTestSettings(t, nil)
		h, _ := newTestHandler(t, nil)
		h.configService = service
		h.settings = runtime

		rec := patchSettingsRequestWithETag(
			t,
			h,
			getSettingsETag(t, h),
			`{"integrations":{"mcp":{"enabled":true}}}`,
		)
		if rec.Code != http.StatusUnprocessableEntity ||
			!strings.Contains(rec.Body.String(), "mcp.enabled requires server.token") {
			t.Fatalf("MCP without token = %d %s", rec.Code, rec.Body.String())
		}
		if _, err := os.Stat(service.Path()); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("rejected first save created config: %v", err)
		}
	})

	t.Run("configured token", func(t *testing.T) {
		content := "[server]\ntoken = \"shared-secret\"\n"
		service, runtime := newTestSettings(t, &content)
		h, _ := newTestHandler(t, nil)
		h.configService = service
		h.settings = runtime

		rec := patchSettingsRequestWithETag(
			t,
			h,
			getSettingsETag(t, h),
			`{"integrations":{"mcp":{"enabled":true}}}`,
		)
		if rec.Code != http.StatusOK {
			t.Fatalf("MCP enable = %d %s", rec.Code, rec.Body.String())
		}
		if !runtime.Enabled() {
			t.Fatal("MCP live state remained disabled")
		}
		if strings.Contains(rec.Body.String(), "shared-secret") {
			t.Fatal("MCP response leaked token")
		}
	})

	t.Run("new token waits for restart", func(t *testing.T) {
		service, runtime := newTestSettings(t, nil)
		h, _ := newTestHandler(t, nil)
		h.configService = service
		h.settings = runtime

		rec := patchSettingsRequestWithETag(
			t,
			h,
			getSettingsETag(t, h),
			`{"integrations":{"mcp":{"enabled":true,"token":{"action":"replace","value":"new-private-token"}}}}`,
		)
		if rec.Code != http.StatusOK {
			t.Fatalf("MCP token replace = %d %s", rec.Code, rec.Body.String())
		}
		var body settingsEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		mcp := body.Data.Integrations.MCP
		if runtime.Enabled() ||
			mcp.RuntimeTokenConfigured ||
			!mcp.Token.Configured ||
			!mcp.Token.RestartPending ||
			mcp.Enabled.ApplyMode != config.ApplyModeRestart ||
			!mcp.Enabled.RestartPending {
			t.Fatalf("MCP deferred state = %+v runtime=%t", mcp, runtime.Enabled())
		}
		if strings.Contains(rec.Body.String(), "new-private-token") {
			t.Fatal("MCP response leaked replacement token")
		}
	})
}

func TestSettingsIntegrationSecretLifecycleIsWriteOnly(t *testing.T) {
	service, runtime := newTestSettings(t, nil)
	h, _ := newTestHandler(t, nil)
	h.configService = service
	h.settings = runtime

	const (
		tokenSecret   = "mcp-write-only-secret"
		webhookSecret = "webhook-write-only-secret"
		webhookURL    = "https://hooks.example.test/private?token=" + webhookSecret
	)
	replace := patchSettingsRequestWithETag(
		t,
		h,
		getSettingsETag(t, h),
		`{"integrations":{"mcp":{"enabled":true,"token":{"action":"replace","value":"`+tokenSecret+`"}},"healthReport":{"schedule":"*/15 * * * *","webhookUrl":{"action":"replace","value":"`+webhookURL+`"}}}}`,
	)
	if replace.Code != http.StatusOK {
		t.Fatalf("replace integrations = %d %s", replace.Code, replace.Body.String())
	}
	var replaced settingsEnvelope
	if err := json.Unmarshal(replace.Body.Bytes(), &replaced); err != nil {
		t.Fatal(err)
	}
	if !replaced.Data.Integrations.MCP.Token.Configured ||
		!replaced.Data.Integrations.HealthReport.WebhookURL.Configured ||
		replaced.Data.Integrations.HealthReport.NextActivation == "" {
		t.Fatalf("replace state = %+v", replaced.Data.Integrations)
	}
	assertSettingsBodySecretSafe(t, replace.Body.String(), tokenSecret, webhookSecret, webhookURL)

	get := httptest.NewRecorder()
	h.getSettings(get, httptest.NewRequest(http.MethodGet, "/api/ops/settings", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("GET after replace = %d %s", get.Code, get.Body.String())
	}
	assertSettingsBodySecretSafe(t, get.Body.String(), tokenSecret, webhookSecret, webhookURL)

	persisted, err := os.ReadFile(service.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(persisted), tokenSecret) || !strings.Contains(string(persisted), webhookSecret) {
		t.Fatal("replacement secrets were not persisted")
	}

	clearResponse := patchSettingsRequestWithETag(
		t,
		h,
		quoteETag(replaced.Data.Revision),
		`{"integrations":{"mcp":{"enabled":false,"token":{"action":"clear"}},"healthReport":{"schedule":"","webhookUrl":{"action":"clear"}}}}`,
	)
	if clearResponse.Code != http.StatusOK {
		t.Fatalf("clear integrations = %d %s", clearResponse.Code, clearResponse.Body.String())
	}
	var cleared settingsEnvelope
	if err := json.Unmarshal(clearResponse.Body.Bytes(), &cleared); err != nil {
		t.Fatal(err)
	}
	if cleared.Data.Integrations.MCP.Token.Configured ||
		cleared.Data.Integrations.HealthReport.WebhookURL.Configured ||
		cleared.Data.Integrations.HealthReport.NextActivation != "" {
		t.Fatalf("clear state = %+v", cleared.Data.Integrations)
	}
	assertSettingsBodySecretSafe(t, clearResponse.Body.String(), tokenSecret, webhookSecret, webhookURL)

	keep := patchSettingsRequestWithETag(
		t,
		h,
		quoteETag(cleared.Data.Revision),
		`{"integrations":{"mcp":{"token":{"action":"keep"}},"healthReport":{"webhookUrl":{"action":"keep"}}}}`,
	)
	if keep.Code != http.StatusOK {
		t.Fatalf("keep integrations = %d %s", keep.Code, keep.Body.String())
	}
	var kept settingsEnvelope
	if err := json.Unmarshal(keep.Body.Bytes(), &kept); err != nil {
		t.Fatal(err)
	}
	if kept.Data.Revision != cleared.Data.Revision {
		t.Fatalf("keep revision = %q, want %q", kept.Data.Revision, cleared.Data.Revision)
	}
}

func TestSettingsIntegrationValidationDoesNotWriteOrEchoSecrets(t *testing.T) {
	content := "[health_report]\nschedule = \"0 * * * *\"\n"
	service, runtime := newTestSettings(t, &content)
	h, _ := newTestHandler(t, nil)
	h.configService = service
	h.settings = runtime

	tests := []struct {
		name       string
		body       string
		statusCode int
		want       string
		forbidden  string
	}{
		{
			name:       "unknown action",
			body:       `{"integrations":{"mcp":{"token":{"action":"reveal","value":"must-not-echo"}}}}`,
			statusCode: http.StatusBadRequest,
			want:       "action must be keep, replace, or clear",
			forbidden:  "must-not-echo",
		},
		{
			name:       "invalid cron",
			body:       `{"integrations":{"healthReport":{"schedule":"not a cron"}}}`,
			statusCode: http.StatusUnprocessableEntity,
			want:       "health_report.schedule",
		},
		{
			name:       "invalid webhook",
			body:       `{"integrations":{"healthReport":{"webhookUrl":{"action":"replace","value":"https://user:private@example.test/hook#secret-fragment"}}}}`,
			statusCode: http.StatusUnprocessableEntity,
			want:       "health_report.webhook_url",
			forbidden:  "secret-fragment",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before, err := os.ReadFile(service.Path())
			if err != nil {
				t.Fatal(err)
			}
			revision := getSettingsETag(t, h)
			rec := patchSettingsRequestWithETag(t, h, revision, test.body)
			if rec.Code != test.statusCode || !strings.Contains(rec.Body.String(), test.want) {
				t.Fatalf("invalid integration = %d %s", rec.Code, rec.Body.String())
			}
			if test.forbidden != "" && strings.Contains(rec.Body.String(), test.forbidden) {
				t.Fatalf("validation response echoed secret: %s", rec.Body.String())
			}
			after, err := os.ReadFile(service.Path())
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) || getSettingsETag(t, h) != revision {
				t.Fatal("invalid integration changed file or revision")
			}
		})
	}
}

func TestSettingsIntegrationSecretsOwnedByEnvironmentAreReadOnly(t *testing.T) {
	t.Setenv("SENTINEL_SERVER_TOKEN", "environment-token-secret")
	t.Setenv(
		"SENTINEL_HEALTH_REPORT_WEBHOOK_URL",
		"https://hooks.example.test/environment-webhook-secret",
	)
	service, runtime := newTestSettings(t, nil)
	h, _ := newTestHandler(t, nil)
	h.configService = service
	h.settings = runtime

	rec := httptest.NewRecorder()
	h.getSettings(rec, httptest.NewRequest(http.MethodGet, "/api/ops/settings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body.String())
	}
	var body settingsEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Integrations.MCP.Token.Source != config.FieldSourceEnvironment ||
		body.Data.Integrations.MCP.Token.Editable ||
		body.Data.Integrations.HealthReport.WebhookURL.Source != config.FieldSourceEnvironment ||
		body.Data.Integrations.HealthReport.WebhookURL.Editable {
		t.Fatalf("environment secret metadata = %+v", body.Data.Integrations)
	}
	assertSettingsBodySecretSafe(
		t,
		rec.Body.String(),
		"environment-token-secret",
		"environment-webhook-secret",
	)

	for _, test := range []struct {
		field string
		body  string
	}{
		{
			field: config.FieldServerToken,
			body:  `{"integrations":{"mcp":{"token":{"action":"replace","value":"tampered-token"}}}}`,
		},
		{
			field: config.FieldHealthReportWebhookURL,
			body:  `{"integrations":{"healthReport":{"webhookUrl":{"action":"clear"}}}}`,
		},
	} {
		rec := patchSettingsRequestWithETag(t, h, getSettingsETag(t, h), test.body)
		if rec.Code != http.StatusConflict ||
			!strings.Contains(rec.Body.String(), "ENVIRONMENT_OWNED") ||
			!strings.Contains(rec.Body.String(), test.field) {
			t.Fatalf("environment-owned %s = %d %s", test.field, rec.Code, rec.Body.String())
		}
		assertSettingsBodySecretSafe(t, rec.Body.String(), "tampered-token")
	}
}

func TestSettingsAccountsExposeReadOnlyInventoryAndCapabilities(t *testing.T) {
	content := `[multi_user]
allowed_users = ["deploy", "root"]
allow_root_target = true
user_switch_method = "sudo"
`
	service, runtime := newTestSettings(t, &content)
	h, _ := newTestHandler(t, nil)
	h.configService = service
	h.settings = runtime
	h.guard = security.NewWithMultiUser("", nil, security.CookieSecureAuto, security.MultiUserConfig{
		AllowedUsers:    []string{"deploy", "root"},
		AllowRootTarget: true,
		SystemUsers:     []string{"root", "hugo", "deploy", "deploy"},
	})
	h.switchCapabilities = func() []userswitch.Capability {
		return []userswitch.Capability{
			{Method: userswitch.MethodSudo, Available: true, Detail: "sudo ready"},
			{Method: userswitch.MethodSystemdRun, Available: false, Detail: "systemd-run missing"},
		}
	}
	originalCurrentUser := osCurrentUser
	originalGeteuid := osGeteuid
	osCurrentUser = func() (*user.User, error) {
		return &user.User{Username: "hugo"}, nil
	}
	osGeteuid = func() int { return 1000 }
	t.Cleanup(func() {
		osCurrentUser = originalCurrentUser
		osGeteuid = originalGeteuid
	})

	rec := httptest.NewRecorder()
	h.getSettings(rec, httptest.NewRequest(http.MethodGet, "/api/ops/settings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET accounts = %d %s", rec.Code, rec.Body.String())
	}
	var body settingsEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	accounts := body.Data.Accounts
	if accounts.ProcessUser != "hugo" || accounts.ProcessIsRoot || !accounts.InventoryAvailable {
		t.Fatalf("process identity = %+v", accounts)
	}
	if got := settingsAccountNames(accounts.Users); !slices.Equal(got, []string{"deploy", "hugo", "root"}) {
		t.Fatalf("system users = %v", got)
	}
	if !accounts.Users[0].Allowed || accounts.Users[1].Allowed || !accounts.Users[2].Allowed {
		t.Fatalf("account authorization = %+v", accounts.Users)
	}
	if !slices.Equal(accounts.AllowedUsers.EffectiveValue, []string{"deploy", "root"}) ||
		accounts.AllowedUsers.Validation.AllowCustom ||
		accounts.AllowRootTarget.ApplyMode != config.ApplyModeRestart ||
		accounts.UserSwitchMethod.EffectiveValue != userswitch.MethodSudo {
		t.Fatalf("account settings = %+v", accounts)
	}
	if len(accounts.MethodCapabilities) != 2 ||
		!accounts.MethodCapabilities[0].Available ||
		accounts.MethodCapabilities[1].Available ||
		accounts.PrivilegeGuidance == "" {
		t.Fatalf("capabilities = %+v guidance=%q", accounts.MethodCapabilities, accounts.PrivilegeGuidance)
	}
}

func TestSettingsAccountsPatchIsStrictAndKeepsRootPolicyConsistent(t *testing.T) {
	content := `[multi_user]
allowed_users = ["deploy"]
allow_root_target = false
user_switch_method = "systemd-run"
`
	service, runtime := newTestSettings(t, &content)
	h, _ := newTestHandler(t, nil)
	h.configService = service
	h.settings = runtime
	h.guard = security.NewWithMultiUser("", nil, security.CookieSecureAuto, security.MultiUserConfig{
		AllowedUsers: []string{"deploy"},
		SystemUsers:  []string{"root", "hugo", "deploy"},
	})

	enableRoot := patchSettingsRequestWithETag(
		t,
		h,
		getSettingsETag(t, h),
		`{"accounts":{"allowedUsers":["deploy"],"allowRootTarget":true,"userSwitchMethod":"sudo"}}`,
	)
	if enableRoot.Code != http.StatusOK {
		t.Fatalf("enable root = %d %s", enableRoot.Code, enableRoot.Body.String())
	}
	var enabled settingsEnvelope
	if err := json.Unmarshal(enableRoot.Body.Bytes(), &enabled); err != nil {
		t.Fatal(err)
	}
	if !enabled.Data.Accounts.AllowRootTarget.EffectiveValue ||
		!slices.Equal(enabled.Data.Accounts.AllowedUsers.EffectiveValue, []string{"deploy", "root"}) ||
		enabled.Data.Accounts.UserSwitchMethod.EffectiveValue != userswitch.MethodSudo {
		t.Fatalf("enabled account settings = %+v", enabled.Data.Accounts)
	}

	disableRoot := patchSettingsRequestWithETag(
		t,
		h,
		quoteETag(enabled.Data.Revision),
		`{"accounts":{"allowRootTarget":false}}`,
	)
	if disableRoot.Code != http.StatusOK {
		t.Fatalf("disable root = %d %s", disableRoot.Code, disableRoot.Body.String())
	}
	var disabled settingsEnvelope
	if err := json.Unmarshal(disableRoot.Body.Bytes(), &disabled); err != nil {
		t.Fatal(err)
	}
	if disabled.Data.Accounts.AllowRootTarget.EffectiveValue ||
		slices.Contains(disabled.Data.Accounts.AllowedUsers.EffectiveValue, "root") {
		t.Fatalf("root remained authorized = %+v", disabled.Data.Accounts)
	}

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "unknown user",
			body: `{"accounts":{"allowedUsers":["ghost"]}}`,
			want: "unknown account ghost",
		},
		{
			name: "duplicate user",
			body: `{"accounts":{"allowedUsers":["deploy","deploy"]}}`,
			want: "duplicate account deploy",
		},
		{
			name: "root without gate",
			body: `{"accounts":{"allowedUsers":["root"]}}`,
			want: "cannot include root",
		},
		{
			name: "invalid method",
			body: `{"accounts":{"userSwitchMethod":"systemd"}}`,
			want: "must be",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before, err := os.ReadFile(service.Path())
			if err != nil {
				t.Fatal(err)
			}
			rec := patchSettingsRequestWithETag(t, h, getSettingsETag(t, h), test.body)
			if rec.Code != http.StatusUnprocessableEntity ||
				!strings.Contains(rec.Body.String(), test.want) {
				t.Fatalf("strict account patch = %d %s", rec.Code, rec.Body.String())
			}
			after, err := os.ReadFile(service.Path())
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatal("rejected account patch changed config")
			}
		})
	}
}

func TestSettingsAccountsEnvironmentOwnershipIsReadOnly(t *testing.T) {
	t.Setenv("SENTINEL_ALLOWED_USERS", "deploy")
	t.Setenv("SENTINEL_USER_SWITCH_METHOD", "sudo")
	service, runtime := newTestSettings(t, nil)
	h, _ := newTestHandler(t, nil)
	h.configService = service
	h.settings = runtime
	h.guard = security.NewWithMultiUser("", nil, security.CookieSecureAuto, security.MultiUserConfig{
		SystemUsers: []string{"root", "hugo", "deploy"},
	})

	rec := httptest.NewRecorder()
	h.getSettings(rec, httptest.NewRequest(http.MethodGet, "/api/ops/settings", nil))
	var body settingsEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Accounts.AllowedUsers.Editable ||
		body.Data.Accounts.AllowedUsers.Source != config.FieldSourceEnvironment ||
		body.Data.Accounts.UserSwitchMethod.Editable ||
		body.Data.Accounts.UserSwitchMethod.Source != config.FieldSourceEnvironment {
		t.Fatalf("environment account settings = %+v", body.Data.Accounts)
	}

	for _, requestBody := range []string{
		`{"accounts":{"allowedUsers":["hugo"]}}`,
		`{"accounts":{"userSwitchMethod":"systemd-run"}}`,
	} {
		rejected := patchSettingsRequestWithETag(t, h, getSettingsETag(t, h), requestBody)
		if rejected.Code != http.StatusConflict ||
			!strings.Contains(rejected.Body.String(), "ENVIRONMENT_OWNED") {
			t.Fatalf("environment-owned account patch = %d %s", rejected.Code, rejected.Body.String())
		}
	}
}

func TestSettingsPatchMapsLockAndRollsBackLiveFailure(t *testing.T) {
	content := "[server]\nlocale = \"en-US\"\n"
	service, runtime := newTestSettings(t, &content)
	h, _ := newTestHandler(t, nil)
	h.configService = service
	h.settings = runtime

	lockFile, err := os.OpenFile(service.Path()+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lockFile.Close() }()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	locked := patchSettingsRequestWithETag(
		t,
		h,
		getSettingsETag(t, h),
		`{"experience":{"locale":"pt-BR"}}`,
	)
	if locked.Code != http.StatusLocked ||
		!strings.Contains(locked.Body.String(), "CONFIG_LOCKED") {
		t.Fatalf("locked patch = %d %s", locked.Code, locked.Body.String())
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}

	original, err := os.ReadFile(service.Path())
	if err != nil {
		t.Fatal(err)
	}
	runtime.mu.Lock()
	runtime.failNext = true
	runtime.mu.Unlock()
	failed := patchSettingsRequestWithETag(
		t,
		h,
		getSettingsETag(t, h),
		`{"experience":{"locale":"pt-BR"}}`,
	)
	if failed.Code != http.StatusInternalServerError ||
		!strings.Contains(failed.Body.String(), "CONFIG_WRITE_FAILED") {
		t.Fatalf("live failure = %d %s", failed.Code, failed.Body.String())
	}
	rolledBack, err := os.ReadFile(service.Path())
	if err != nil {
		t.Fatal(err)
	}
	if string(rolledBack) != string(original) || runtime.Locale() != "en-US" {
		t.Fatalf("rollback failed: locale=%q config=%s", runtime.Locale(), rolledBack)
	}
}

func TestSettingsDeploymentRequiresExactConfigPathMatch(t *testing.T) {
	service, runtime := newTestSettings(t, nil)
	h, _ := newTestHandler(t, nil)
	h.configService = service
	h.settings = runtime
	h.deployments = func() ([]daemon.Deployment, error) {
		return []daemon.Deployment{
			{Scope: daemon.ScopeSystem, ConfigPath: service.Path() + ".other"},
			{Scope: daemon.ScopeUser, ConfigPath: service.Path()},
		}, nil
	}
	state, err := service.Read()
	if err != nil {
		t.Fatal(err)
	}
	response := h.settingsResponse(state)
	if response.Deployment.Scope != daemon.ScopeUser ||
		response.Restart.Command != "sentinel service restart --scope user" {
		t.Fatalf("matched deployment = %+v restart=%+v", response.Deployment, response.Restart)
	}

	h.deployments = func() ([]daemon.Deployment, error) {
		return []daemon.Deployment{{Scope: daemon.ScopeSystem, ConfigPath: service.Path() + ".other"}}, nil
	}
	response = h.settingsResponse(state)
	if response.Deployment.Scope != "standalone" ||
		response.Restart.Command != "" ||
		!strings.Contains(response.Restart.Instruction, "external supervisor") {
		t.Fatalf("standalone deployment = %+v restart=%+v", response.Deployment, response.Restart)
	}
}

func getSettingsETag(t *testing.T, h *Handler) string {
	t.Helper()
	rec := httptest.NewRecorder()
	h.getSettings(rec, httptest.NewRequest(http.MethodGet, "/api/ops/settings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET settings = %d %s", rec.Code, rec.Body.String())
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("GET settings returned no ETag")
	}
	return etag
}

func patchSettingsRequestWithETag(
	t *testing.T,
	h *Handler,
	etag string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/ops/settings", strings.NewReader(body))
	req.Header.Set("If-Match", etag)
	req.Header.Set("Content-Type", "application/json")
	h.patchSettings(rec, req)
	return rec
}

func assertSettingsBodySecretSafe(t *testing.T, body string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if secret != "" && strings.Contains(body, secret) {
			t.Fatalf("settings response exposed secret %q: %s", secret, body)
		}
	}
}

func slicesContain(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func settingsAccountNames(accounts []settingsSystemUser) []string {
	names := make([]string, 0, len(accounts))
	for _, account := range accounts {
		names = append(names, account.Name)
	}
	return names
}
