package api

import (
	"encoding/json"
	"errors"
	"net"
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

func TestSettingsRestartAcceptsCurrentManagedDeploymentOnce(t *testing.T) {
	service, runtime := newTestSettings(t, nil)
	h, _ := newTestHandler(t, nil)
	h.configService = service
	h.settings = runtime
	h.deployments = func() ([]daemon.Deployment, error) {
		return []daemon.Deployment{{
			Scope:      daemon.ScopeUser,
			ConfigPath: service.Path(),
		}}, nil
	}
	restartCall := make(chan [2]string, 1)
	h.settingsControl = func(action, scope string) error {
		restartCall <- [2]string{action, scope}
		return nil
	}
	h.settingsRestartIn = time.Millisecond

	saved := patchSettingsRequestWithETag(
		t,
		h,
		getSettingsETag(t, h),
		`{"operations":{"runbooks":{"maxConcurrent":9}}}`,
	)
	if saved.Code != http.StatusOK {
		t.Fatalf("save pending settings = %d %s", saved.Code, saved.Body.String())
	}
	var savedBody settingsEnvelope
	if err := json.Unmarshal(saved.Body.Bytes(), &savedBody); err != nil {
		t.Fatal(err)
	}
	if !savedBody.Data.Restart.Required || !savedBody.Data.Restart.Available {
		t.Fatalf("restart = %+v", savedBody.Data.Restart)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/ops/settings/restart", nil)
	request.Header.Set("If-Match", saved.Header().Get("ETag"))
	accepted := httptest.NewRecorder()
	h.restartSettings(accepted, request)
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("restart = %d %s", accepted.Code, accepted.Body.String())
	}
	if !strings.Contains(accepted.Body.String(), `"status":"accepted"`) ||
		!strings.Contains(accepted.Body.String(), config.FieldRunbooksMaxConcurrent) {
		t.Fatalf("accepted restart body = %s", accepted.Body.String())
	}

	duplicate := httptest.NewRecorder()
	h.restartSettings(duplicate, request)
	if duplicate.Code != http.StatusConflict ||
		!strings.Contains(duplicate.Body.String(), "RESTART_ALREADY_SCHEDULED") {
		t.Fatalf("duplicate restart = %d %s", duplicate.Code, duplicate.Body.String())
	}

	select {
	case call := <-restartCall:
		if call != [2]string{"restart", daemon.ScopeUser} {
			t.Fatalf("restart call = %v", call)
		}
	case <-time.After(time.Second):
		t.Fatal("managed service restart was not scheduled")
	}
}

func TestSettingsRestartRejectsUnsafeOrStaleRequests(t *testing.T) {
	service, runtime := newTestSettings(t, nil)
	h, _ := newTestHandler(t, nil)
	h.configService = service
	h.settings = runtime
	h.settingsControl = func(string, string) error {
		t.Fatal("service control must not run")
		return nil
	}

	missingRevision := httptest.NewRecorder()
	h.restartSettings(
		missingRevision,
		httptest.NewRequest(http.MethodPost, "/api/ops/settings/restart", nil),
	)
	if missingRevision.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing revision = %d %s", missingRevision.Code, missingRevision.Body.String())
	}

	notRequiredRequest := httptest.NewRequest(http.MethodPost, "/api/ops/settings/restart", nil)
	notRequiredRequest.Header.Set("If-Match", getSettingsETag(t, h))
	notRequired := httptest.NewRecorder()
	h.restartSettings(notRequired, notRequiredRequest)
	if notRequired.Code != http.StatusConflict ||
		!strings.Contains(notRequired.Body.String(), "RESTART_NOT_REQUIRED") {
		t.Fatalf("not required = %d %s", notRequired.Code, notRequired.Body.String())
	}

	saved := patchSettingsRequestWithETag(
		t,
		h,
		getSettingsETag(t, h),
		`{"operations":{"runbooks":{"maxConcurrent":9}}}`,
	)
	if saved.Code != http.StatusOK {
		t.Fatalf("save pending settings = %d %s", saved.Code, saved.Body.String())
	}

	staleRequest := httptest.NewRequest(http.MethodPost, "/api/ops/settings/restart", nil)
	staleRequest.Header.Set("If-Match", quoteETag(strings.Repeat("0", 64)))
	stale := httptest.NewRecorder()
	h.restartSettings(stale, staleRequest)
	if stale.Code != http.StatusPreconditionFailed ||
		!strings.Contains(stale.Body.String(), "CONFIG_CONFLICT") {
		t.Fatalf("stale = %d %s", stale.Code, stale.Body.String())
	}

	standaloneRequest := httptest.NewRequest(http.MethodPost, "/api/ops/settings/restart", nil)
	standaloneRequest.Header.Set("If-Match", saved.Header().Get("ETag"))
	standalone := httptest.NewRecorder()
	h.restartSettings(standalone, standaloneRequest)
	if standalone.Code != http.StatusConflict ||
		!strings.Contains(standalone.Body.String(), "RESTART_UNAVAILABLE") {
		t.Fatalf("standalone = %d %s", standalone.Code, standalone.Body.String())
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

	t.Run("token mutation belongs to access", func(t *testing.T) {
		service, runtime := newTestSettings(t, nil)
		h, _ := newTestHandler(t, nil)
		h.configService = service
		h.settings = runtime

		rec := patchSettingsRequestWithETag(
			t,
			h,
			getSettingsETag(t, h),
			`{"integrations":{"mcp":{"token":{"action":"replace","value":"must-not-echo"}}}}`,
		)
		if rec.Code != http.StatusBadRequest ||
			!strings.Contains(rec.Body.String(), "INVALID_REQUEST") {
			t.Fatalf("integration token mutation = %d %s", rec.Code, rec.Body.String())
		}
		assertSettingsBodySecretSafe(t, rec.Body.String(), "must-not-echo")
	})
}

func TestSettingsHealthReportSecretLifecycleIsWriteOnly(t *testing.T) {
	service, runtime := newTestSettings(t, nil)
	h, _ := newTestHandler(t, nil)
	h.configService = service
	h.settings = runtime

	const webhookSecret = "webhook-write-only-secret"
	const webhookURL = "https://hooks.example.test/private?token=" + webhookSecret
	replace := patchSettingsRequestWithETag(
		t,
		h,
		getSettingsETag(t, h),
		`{"integrations":{"healthReport":{"schedule":"*/15 * * * *","webhookUrl":{"action":"replace","value":"`+webhookURL+`"}}}}`,
	)
	if replace.Code != http.StatusOK {
		t.Fatalf("replace integrations = %d %s", replace.Code, replace.Body.String())
	}
	var replaced settingsEnvelope
	if err := json.Unmarshal(replace.Body.Bytes(), &replaced); err != nil {
		t.Fatal(err)
	}
	if !replaced.Data.Integrations.HealthReport.WebhookURL.Configured ||
		replaced.Data.Integrations.HealthReport.NextActivation == "" {
		t.Fatalf("replace state = %+v", replaced.Data.Integrations)
	}
	assertSettingsBodySecretSafe(t, replace.Body.String(), webhookSecret, webhookURL)

	get := httptest.NewRecorder()
	h.getSettings(get, httptest.NewRequest(http.MethodGet, "/api/ops/settings", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("GET after replace = %d %s", get.Code, get.Body.String())
	}
	assertSettingsBodySecretSafe(t, get.Body.String(), webhookSecret, webhookURL)

	persisted, err := os.ReadFile(service.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(persisted), webhookSecret) {
		t.Fatal("replacement webhook was not persisted")
	}

	clearResponse := patchSettingsRequestWithETag(
		t,
		h,
		quoteETag(replaced.Data.Revision),
		`{"integrations":{"healthReport":{"schedule":"","webhookUrl":{"action":"clear"}}}}`,
	)
	if clearResponse.Code != http.StatusOK {
		t.Fatalf("clear integrations = %d %s", clearResponse.Code, clearResponse.Body.String())
	}
	var cleared settingsEnvelope
	if err := json.Unmarshal(clearResponse.Body.Bytes(), &cleared); err != nil {
		t.Fatal(err)
	}
	if cleared.Data.Integrations.HealthReport.WebhookURL.Configured ||
		cleared.Data.Integrations.HealthReport.NextActivation != "" {
		t.Fatalf("clear state = %+v", cleared.Data.Integrations)
	}
	assertSettingsBodySecretSafe(t, clearResponse.Body.String(), webhookSecret, webhookURL)

	keep := patchSettingsRequestWithETag(
		t,
		h,
		quoteETag(cleared.Data.Revision),
		`{"integrations":{"healthReport":{"webhookUrl":{"action":"keep"}}}}`,
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
			name:       "unknown webhook action",
			body:       `{"integrations":{"healthReport":{"webhookUrl":{"action":"reveal","value":"must-not-echo"}}}}`,
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

func TestSettingsSecretsOwnedByEnvironmentAreReadOnly(t *testing.T) {
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
			body:  `{"access":{"reconnectOrigin":"http://127.0.0.1:4040","token":{"action":"replace","value":"tampered-token"}}}`,
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

func TestSettingsAccessResponseIsTypedSecretSafeAndActionable(t *testing.T) {
	content := `[server]
host = "127.0.0.1"
port = 4545
token = "access-private-token"
allowed_origins = ["https://sentinel.example.test"]
trusted_proxies = ["127.0.0.1", "10.0.0.0/8"]
cookie_secure = "always"
allow_insecure_cookie = false
`
	service, runtime := newTestSettings(t, &content)
	h, _ := newTestHandler(t, nil)
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
		t.Fatalf("GET access = %d %s", rec.Code, rec.Body.String())
	}
	var body settingsEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	access := body.Data.Access
	if access.Listener.Host.EffectiveValue != "127.0.0.1" ||
		access.Listener.Port.EffectiveValue != 4545 ||
		access.Listener.Classification != "loopback" ||
		access.Listener.Address != "127.0.0.1:4545" {
		t.Fatalf("listener = %+v", access.Listener)
	}
	if !access.Authentication.Token.Configured ||
		!access.Authentication.RuntimeTokenConfigured ||
		access.Cookies.Secure.EffectiveValue != config.CookieSecureAlways ||
		!slices.Equal(
			access.Origins.Allowed.EffectiveValue,
			[]string{"https://sentinel.example.test"},
		) ||
		!slices.Equal(
			access.Proxies.Trusted.EffectiveValue,
			[]string{"127.0.0.1", "10.0.0.0/8"},
		) {
		t.Fatalf("access = %+v", access)
	}
	if access.Recovery.ConfigPath != service.Path() ||
		access.Recovery.BackupPath != service.BackupPath() ||
		!strings.Contains(access.Recovery.RestoreCommand, "cp --") ||
		!strings.Contains(access.Recovery.ValidateCommand, "config validate --effective") ||
		access.Recovery.RestartCommand != "sentinel service restart --scope user" {
		t.Fatalf("recovery = %+v", access.Recovery)
	}
	assertSettingsBodySecretSafe(t, rec.Body.String(), "access-private-token")
}

func TestSettingsAccessRequiresCompatibleReconnectOriginBeforeWrite(t *testing.T) {
	content := "[server]\nhost = \"127.0.0.1\"\nport = 4040\n"
	service, runtime := newTestSettings(t, &content)
	h, _ := newTestHandler(t, nil)
	h.configService = service
	h.settings = runtime
	original, err := os.ReadFile(service.Path())
	if err != nil {
		t.Fatal(err)
	}
	revision := getSettingsETag(t, h)

	for _, test := range []struct {
		name   string
		body   string
		origin string
		want   string
	}{
		{
			name: "missing",
			body: `{"access":{"port":5050}}`,
			want: "access.reconnectOrigin is required",
		},
		{
			name: "non canonical",
			body: `{"access":{"reconnectOrigin":"http://127.0.0.1:5050/","port":5050}}`,
			want: "canonical form",
		},
		{
			name:   "incompatible specific listener",
			body:   `{"access":{"reconnectOrigin":"http://localhost:5050","port":5050}}`,
			origin: "http://localhost:4040",
			want:   `http://127.0.0.1:5050`,
		},
		{
			name:   "wildcard preserves browser hostname",
			body:   `{"access":{"reconnectOrigin":"http://different.example:5050","host":"0.0.0.0","port":5050,"token":{"action":"replace","value":"secret"},"allowedOrigins":["http://sentinel.example:5050"]}}`,
			origin: "http://sentinel.example:4040",
			want:   `http://sentinel.example:5050`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			rec := patchSettingsAccessRequestWithETag(t, h, revision, test.origin, test.body)
			if rec.Code != http.StatusUnprocessableEntity ||
				!strings.Contains(rec.Body.String(), test.want) {
				t.Fatalf("reconnect validation = %d %s", rec.Code, rec.Body.String())
			}
			after, readErr := os.ReadFile(service.Path())
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(after) != string(original) || getSettingsETag(t, h) != revision {
				t.Fatal("reconnect rejection changed file or revision")
			}
			assertSettingsBodySecretSafe(t, rec.Body.String(), "secret")
		})
	}
}

func TestSettingsAccessBindPreflightRejectsExternalConflictBeforeWrite(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	_, rawPort, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	content := "[server]\nhost = \"127.0.0.1\"\nport = 4040\n"
	service, runtime := newTestSettings(t, &content)
	h, _ := newTestHandler(t, nil)
	h.configService = service
	h.settings = runtime
	h.settingsBindCheck = preflightSettingsBind
	original, err := os.ReadFile(service.Path())
	if err != nil {
		t.Fatal(err)
	}
	revision := getSettingsETag(t, h)
	body := `{"access":{"reconnectOrigin":"http://127.0.0.1:` + rawPort + `","port":` + rawPort + `}}`
	rec := patchSettingsAccessRequestWithETag(t, h, revision, "", body)
	if rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), "could not bind candidate address") {
		t.Fatalf("occupied bind = %d %s", rec.Code, rec.Body.String())
	}
	after, err := os.ReadFile(service.Path())
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) || getSettingsETag(t, h) != revision {
		t.Fatal("occupied bind changed file or revision")
	}
}

func TestSettingsAccessValidRemoteRotationIsWriteOnlyAndRestartBased(t *testing.T) {
	content := "[server]\nhost = \"127.0.0.1\"\nport = 4040\n"
	service, runtime := newTestSettings(t, &content)
	h, _ := newTestHandler(t, nil)
	h.configService = service
	h.settings = runtime
	h.settingsBindCheck = func(current, candidate string) error {
		if current != "127.0.0.1:4040" || candidate != "0.0.0.0:5050" {
			t.Fatalf("bind preflight = current:%q candidate:%q", current, candidate)
		}
		return nil
	}

	const replacement = "rotated-access-private-token"
	rec := patchSettingsAccessRequestWithETag(
		t,
		h,
		getSettingsETag(t, h),
		"http://sentinel.example:4040",
		`{"access":{"reconnectOrigin":"http://sentinel.example:5050","host":"0.0.0.0","port":5050,"token":{"action":"replace","value":"`+replacement+`"},"allowedOrigins":["http://sentinel.example:5050"],"trustedProxies":["127.0.0.1","10.0.0.0/8"],"cookieSecure":"auto","allowInsecureCookie":false}}`,
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("remote access patch = %d %s", rec.Code, rec.Body.String())
	}
	var body settingsEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Access.Listener.Classification != "wildcard" ||
		body.Data.Access.Listener.Address != "0.0.0.0:5050" ||
		!body.Data.Access.Authentication.Token.Configured ||
		body.Data.Access.Authentication.RuntimeTokenConfigured ||
		!body.Data.Restart.Required {
		t.Fatalf("remote access response = %+v restart=%+v", body.Data.Access, body.Data.Restart)
	}
	for _, key := range []string{
		config.FieldServerHost,
		config.FieldServerPort,
		config.FieldServerToken,
		config.FieldServerAllowedOrigins,
		config.FieldServerTrustedProxies,
	} {
		if !slices.Contains(body.Data.Restart.ChangedKeys, key) {
			t.Fatalf("restart keys = %v, missing %q", body.Data.Restart.ChangedKeys, key)
		}
	}
	assertSettingsBodySecretSafe(t, rec.Body.String(), replacement)
	state, err := service.Read()
	if err != nil {
		t.Fatal(err)
	}
	if state.Persisted.Server.Token != replacement ||
		state.Persisted.Address() != "0.0.0.0:5050" {
		t.Fatalf("persisted access = %+v", state.Persisted.Server)
	}
}

func TestSettingsAccessRejectsUnsafeCandidateAndEnvironmentOwnership(t *testing.T) {
	t.Run("remote without token and origin", func(t *testing.T) {
		content := "[server]\nhost = \"127.0.0.1\"\nport = 4040\n"
		service, runtime := newTestSettings(t, &content)
		h, _ := newTestHandler(t, nil)
		h.configService = service
		h.settings = runtime
		revision := getSettingsETag(t, h)
		original, err := os.ReadFile(service.Path())
		if err != nil {
			t.Fatal(err)
		}
		rec := patchSettingsAccessRequestWithETag(
			t,
			h,
			revision,
			"http://sentinel.example:4040",
			`{"access":{"reconnectOrigin":"http://sentinel.example:4040","host":"0.0.0.0"}}`,
		)
		if rec.Code != http.StatusUnprocessableEntity ||
			!strings.Contains(rec.Body.String(), "token is required") {
			t.Fatalf("unsafe remote = %d %s", rec.Code, rec.Body.String())
		}
		after, err := os.ReadFile(service.Path())
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != string(original) || getSettingsETag(t, h) != revision {
			t.Fatal("unsafe remote candidate changed config")
		}
	})

	t.Run("clear token while remote", func(t *testing.T) {
		content := `[server]
host = "0.0.0.0"
port = 4040
token = "current-private-token"
allowed_origins = ["http://sentinel.example:4040"]
`
		service, runtime := newTestSettings(t, &content)
		h, _ := newTestHandler(t, nil)
		h.configService = service
		h.settings = runtime
		rec := patchSettingsAccessRequestWithETag(
			t,
			h,
			getSettingsETag(t, h),
			"http://sentinel.example:4040",
			`{"access":{"reconnectOrigin":"http://sentinel.example:4040","token":{"action":"clear"}}}`,
		)
		if rec.Code != http.StatusUnprocessableEntity ||
			!strings.Contains(rec.Body.String(), "token is required") {
			t.Fatalf("remote token clear = %d %s", rec.Code, rec.Body.String())
		}
		assertSettingsBodySecretSafe(t, rec.Body.String(), "current-private-token")
	})

	t.Run("secure cookie over http", func(t *testing.T) {
		content := "[server]\nhost = \"127.0.0.1\"\nport = 4040\ntoken = \"current-private-token\"\n"
		service, runtime := newTestSettings(t, &content)
		h, _ := newTestHandler(t, nil)
		h.configService = service
		h.settings = runtime
		rec := patchSettingsAccessRequestWithETag(
			t,
			h,
			getSettingsETag(t, h),
			"http://127.0.0.1:4040",
			`{"access":{"reconnectOrigin":"http://127.0.0.1:4040","cookieSecure":"always"}}`,
		)
		if rec.Code != http.StatusUnprocessableEntity ||
			!strings.Contains(rec.Body.String(), "requires an HTTPS reconnect origin") {
			t.Fatalf("http secure cookie = %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("environment owned", func(t *testing.T) {
		t.Setenv("SENTINEL_SERVER_PORT", "5050")
		service, runtime := newTestSettings(t, nil)
		h, _ := newTestHandler(t, nil)
		h.configService = service
		h.settings = runtime
		rec := patchSettingsAccessRequestWithETag(
			t,
			h,
			getSettingsETag(t, h),
			"",
			`{"access":{"reconnectOrigin":"http://127.0.0.1:6060","port":6060}}`,
		)
		if rec.Code != http.StatusConflict ||
			!strings.Contains(rec.Body.String(), "ENVIRONMENT_OWNED") ||
			!strings.Contains(rec.Body.String(), config.FieldServerPort) {
			t.Fatalf("environment access = %d %s", rec.Code, rec.Body.String())
		}
	})
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

func patchSettingsAccessRequestWithETag(
	t *testing.T,
	h *Handler,
	etag string,
	origin string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/ops/settings", strings.NewReader(body))
	req.Header.Set("If-Match", etag)
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
		req.Host = strings.TrimPrefix(origin, "http://")
		req.Host = strings.TrimPrefix(req.Host, "https://")
	}
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
