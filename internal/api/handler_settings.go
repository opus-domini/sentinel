package api

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/daemon"
)

const settingsDeploymentStandalone = "standalone"

var errMCPTokenRequired = errors.New("server.token must be available at process startup before MCP can be enabled")

type deploymentDetector func() ([]daemon.Deployment, error)

var installedDeployments deploymentDetector = daemon.InstalledDeployments

type settingsResponse struct {
	Revision     string               `json:"revision"`
	Metadata     settingsMetadata     `json:"metadata"`
	Deployment   settingsDeployment   `json:"deployment"`
	Restart      settingsRestart      `json:"restart"`
	Experience   settingsExperience   `json:"experience"`
	Integrations settingsIntegrations `json:"integrations"`
	Diagnostics  settingsDiagnostics  `json:"diagnostics"`
}

type settingsMetadata struct {
	Version string `json:"version"`
}

type settingsDeployment struct {
	Scope       string `json:"scope"`
	RuntimeMode string `json:"runtimeMode"`
	ConfigPath  string `json:"configPath"`
}

type settingsRestart struct {
	Required    bool     `json:"required"`
	ChangedKeys []string `json:"changedKeys"`
	Command     string   `json:"command,omitempty"`
	BackupPath  string   `json:"backupPath"`
	Instruction string   `json:"instruction"`
}

type settingsExperience struct {
	Timezone stringSetting `json:"timezone"`
	Locale   stringSetting `json:"locale"`
}

type settingsIntegrations struct {
	MCP settingsMCP `json:"mcp"`
}

type settingsMCP struct {
	Enabled         boolSetting `json:"enabled"`
	TokenConfigured bool        `json:"tokenConfigured"`
	Endpoint        string      `json:"endpoint"`
}

type settingsDiagnostics struct {
	ConfigExists         bool     `json:"configExists"`
	EnvironmentOwnedKeys []string `json:"environmentOwnedKeys"`
	ReadOnlyKeys         []string `json:"readOnlyKeys"`
	DeploymentDetection  string   `json:"deploymentDetection"`
}

type settingOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type stringValidation struct {
	Required    bool            `json:"required"`
	Format      string          `json:"format,omitempty"`
	AllowCustom bool            `json:"allowCustom"`
	Options     []settingOption `json:"options"`
}

type boolValidation struct {
	Required bool `json:"required"`
}

type stringSetting struct {
	PersistedValue *string            `json:"persistedValue,omitempty"`
	EffectiveValue string             `json:"effectiveValue"`
	DefaultValue   string             `json:"defaultValue"`
	Source         config.FieldSource `json:"source"`
	Editable       bool               `json:"editable"`
	ApplyMode      config.ApplyMode   `json:"applyMode"`
	RestartPending bool               `json:"restartPending"`
	Validation     stringValidation   `json:"validation"`
}

type boolSetting struct {
	PersistedValue *bool              `json:"persistedValue,omitempty"`
	EffectiveValue bool               `json:"effectiveValue"`
	DefaultValue   bool               `json:"defaultValue"`
	Source         config.FieldSource `json:"source"`
	Editable       bool               `json:"editable"`
	ApplyMode      config.ApplyMode   `json:"applyMode"`
	RestartPending bool               `json:"restartPending"`
	Validation     boolValidation     `json:"validation"`
}

type patchSettingsRequest struct {
	Experience   *patchExperience   `json:"experience,omitempty"`
	Integrations *patchIntegrations `json:"integrations,omitempty"`
}

type patchExperience struct {
	Timezone *string `json:"timezone,omitempty"`
	Locale   *string `json:"locale,omitempty"`
}

type patchIntegrations struct {
	MCP *patchMCP `json:"mcp,omitempty"`
}

type patchMCP struct {
	Enabled *bool `json:"enabled,omitempty"`
}

func (h *Handler) getSettings(w http.ResponseWriter, _ *http.Request) {
	if h.configService == nil || h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "SETTINGS_UNAVAILABLE", "settings are unavailable", nil)
		return
	}
	state, err := h.configService.Read()
	if err != nil {
		writeSettingsError(w, err)
		return
	}
	response := h.settingsResponse(state)
	w.Header().Set("ETag", quoteETag(state.Revision))
	writeData(w, http.StatusOK, response)
}

func (h *Handler) patchSettings(w http.ResponseWriter, r *http.Request) {
	if h.configService == nil || h.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "SETTINGS_UNAVAILABLE", "settings are unavailable", nil)
		return
	}
	rawRevision := strings.TrimSpace(r.Header.Get("If-Match"))
	if rawRevision == "" {
		writeError(w, http.StatusPreconditionRequired, "REVISION_REQUIRED", "If-Match is required", nil)
		return
	}
	revision, err := parseETag(rawRevision)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REVISION", "If-Match must contain the current Settings ETag", nil)
		return
	}

	var req patchSettingsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	keys, mutate, err := h.settingsMutation(req)
	if err != nil {
		writeSettingsError(w, err)
		return
	}
	state, err := h.configService.Update(r.Context(), revision, keys, mutate)
	if err != nil {
		writeSettingsError(w, err)
		return
	}
	response := h.settingsResponse(state)
	w.Header().Set("ETag", quoteETag(state.Revision))
	writeData(w, http.StatusOK, response)
}

func (h *Handler) settingsMutation(req patchSettingsRequest) ([]string, func(*config.Config) error, error) {
	var keys []string
	if req.Experience != nil {
		if req.Experience.Timezone != nil {
			keys = append(keys, config.FieldServerTimezone)
		}
		if req.Experience.Locale != nil {
			keys = append(keys, config.FieldServerLocale)
		}
	}
	if req.Integrations != nil && req.Integrations.MCP != nil && req.Integrations.MCP.Enabled != nil {
		keys = append(keys, config.FieldMCPEnabled)
	}
	if len(keys) == 0 {
		return nil, nil, errors.New("at least one supported settings field is required")
	}

	return keys, func(candidate *config.Config) error {
		if req.Experience != nil {
			if req.Experience.Timezone != nil {
				timezone := strings.TrimSpace(*req.Experience.Timezone)
				if timezone == "" {
					return config.ValidationError{Path: h.configService.Path(), Issues: []string{"server.timezone is required"}}
				}
				candidate.Server.Timezone = timezone
			}
			if req.Experience.Locale != nil {
				candidate.Server.Locale = strings.TrimSpace(*req.Experience.Locale)
			}
		}
		if req.Integrations != nil && req.Integrations.MCP != nil && req.Integrations.MCP.Enabled != nil {
			enabled := *req.Integrations.MCP.Enabled
			if enabled && !h.settings.TokenConfigured() {
				return errMCPTokenRequired
			}
			candidate.MCP.Enabled = enabled
		}
		return nil
	}, nil
}

func (h *Handler) settingsResponse(state config.State) settingsResponse {
	deployment, detection := h.detectSettingsDeployment(state.Path)
	changedKeys := make([]string, 0)
	environmentKeys := make([]string, 0)
	readOnlyKeys := make([]string, 0)
	for _, key := range config.FieldKeys() {
		field, ok := state.Field(key)
		if !ok {
			continue
		}
		if field.RestartPending {
			changedKeys = append(changedKeys, key)
		}
		if field.Source == config.FieldSourceEnvironment {
			environmentKeys = append(environmentKeys, key)
		}
		if !field.Editable && field.Source != config.FieldSourceEnvironment {
			readOnlyKeys = append(readOnlyKeys, key)
		}
	}

	timezoneField, _ := state.Field(config.FieldServerTimezone)
	localeField, _ := state.Field(config.FieldServerLocale)
	mcpField, _ := state.Field(config.FieldMCPEnabled)
	return settingsResponse{
		Revision:   state.Revision,
		Metadata:   settingsMetadata{Version: h.settingsVersion()},
		Deployment: deployment,
		Restart: settingsRestartForDeployment(
			len(changedKeys) > 0,
			changedKeys,
			h.configService.BackupPath(),
			deployment,
		),
		Experience: settingsExperience{
			Timezone: buildStringSetting(
				timezoneField,
				state.Persisted.Server.Timezone,
				state.Effective.Server.Timezone,
				state.Default.Server.Timezone,
				stringValidation{
					Required:    true,
					Format:      "iana-timezone",
					AllowCustom: true,
					Options:     timezoneOptions(),
				},
			),
			Locale: buildStringSetting(
				localeField,
				state.Persisted.Server.Locale,
				state.Effective.Server.Locale,
				state.Default.Server.Locale,
				stringValidation{
					Required:    false,
					Format:      "bcp-47",
					AllowCustom: false,
					Options:     localeOptions(),
				},
			),
		},
		Integrations: settingsIntegrations{
			MCP: settingsMCP{
				Enabled: buildBoolSetting(
					mcpField,
					state.Persisted.MCP.Enabled,
					state.Effective.MCP.Enabled,
					state.Default.MCP.Enabled,
				),
				TokenConfigured: h.settings.TokenConfigured(),
				Endpoint:        "/mcp",
			},
		},
		Diagnostics: settingsDiagnostics{
			ConfigExists:         state.Exists,
			EnvironmentOwnedKeys: environmentKeys,
			ReadOnlyKeys:         readOnlyKeys,
			DeploymentDetection:  detection,
		},
	}
}

func (h *Handler) detectSettingsDeployment(configPath string) (settingsDeployment, string) {
	standalone := settingsDeployment{
		Scope:       settingsDeploymentStandalone,
		RuntimeMode: settingsDeploymentStandalone,
		ConfigPath:  configPath,
	}
	if h.deployments == nil {
		return standalone, "unavailable"
	}
	deployments, err := h.deployments()
	if err != nil {
		return standalone, "unavailable"
	}
	cleanConfigPath := filepath.Clean(configPath)
	for _, deployment := range deployments {
		if filepath.Clean(deployment.ConfigPath) != cleanConfigPath {
			continue
		}
		if deployment.Scope != daemon.ScopeUser && deployment.Scope != daemon.ScopeSystem {
			continue
		}
		return settingsDeployment{
			Scope:       deployment.Scope,
			RuntimeMode: "service",
			ConfigPath:  configPath,
		}, "matched"
	}
	return standalone, settingsDeploymentStandalone
}

func settingsRestartForDeployment(
	required bool,
	changedKeys []string,
	backupPath string,
	deployment settingsDeployment,
) settingsRestart {
	restart := settingsRestart{
		Required:    required,
		ChangedKeys: slices.Clone(changedKeys),
		BackupPath:  backupPath,
	}
	switch deployment.Scope {
	case daemon.ScopeUser:
		restart.Command = "sentinel service restart --scope user"
		restart.Instruction = "Run the command after reviewing the saved configuration."
	case daemon.ScopeSystem:
		restart.Command = "sudo sentinel service restart --scope system"
		restart.Instruction = "Run the privileged command after reviewing the saved configuration."
	default:
		restart.Instruction = "Restart Sentinel with the external supervisor that owns this process."
	}
	return restart
}

func buildStringSetting(
	field config.FieldState,
	persisted string,
	effective string,
	defaultValue string,
	validation stringValidation,
) stringSetting {
	setting := stringSetting{
		EffectiveValue: effective,
		DefaultValue:   defaultValue,
		Source:         field.Source,
		Editable:       field.Editable,
		ApplyMode:      field.ApplyMode,
		RestartPending: field.RestartPending,
		Validation:     validation,
	}
	if field.Defined {
		value := persisted
		setting.PersistedValue = &value
	}
	return setting
}

func buildBoolSetting(field config.FieldState, persisted, effective, defaultValue bool) boolSetting {
	setting := boolSetting{
		EffectiveValue: effective,
		DefaultValue:   defaultValue,
		Source:         field.Source,
		Editable:       field.Editable,
		ApplyMode:      field.ApplyMode,
		RestartPending: field.RestartPending,
		Validation:     boolValidation{Required: true},
	}
	if field.Defined {
		value := persisted
		setting.PersistedValue = &value
	}
	return setting
}

func (h *Handler) settingsVersion() string {
	version := strings.TrimSpace(h.version)
	if version == "" {
		return defaultMetaVersion
	}
	return version
}

func quoteETag(revision string) string {
	return `"` + revision + `"`
}

func parseETag(value string) (string, error) {
	if strings.HasPrefix(value, "W/") || strings.Contains(value, ",") {
		return "", errors.New("weak or multiple ETags are not supported")
	}
	if len(value) != 66 || value[0] != '"' || value[len(value)-1] != '"' {
		return "", errors.New("ETag must be a quoted SHA-256 revision")
	}
	revision := value[1 : len(value)-1]
	decoded, err := hex.DecodeString(revision)
	if err != nil || len(decoded) != 32 {
		return "", errors.New("ETag must be a quoted SHA-256 revision")
	}
	return revision, nil
}

func writeSettingsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, config.ErrRevisionConflict):
		writeError(w, http.StatusPreconditionFailed, "CONFIG_CONFLICT", "settings changed since they were loaded", nil)
	case errors.Is(err, config.ErrEnvironmentOwned):
		writeError(
			w,
			http.StatusConflict,
			"ENVIRONMENT_OWNED",
			"setting is controlled by an environment variable",
			struct {
				Field string `json:"field"`
			}{Field: config.EnvironmentOwnedField(err)},
		)
	case errors.Is(err, config.ErrConfigLocked):
		writeError(w, http.StatusLocked, "CONFIG_LOCKED", "settings are being updated by another process", nil)
	case errors.Is(err, errMCPTokenRequired):
		writeError(w, http.StatusConflict, "MCP_TOKEN_REQUIRED", "configure server.token and restart Sentinel before enabling MCP", nil)
	default:
		if issues := config.ValidationIssues(err); len(issues) > 0 {
			writeError(
				w,
				http.StatusUnprocessableEntity,
				"CONFIG_INVALID",
				"one or more settings are invalid",
				struct {
					Issues []string `json:"issues"`
				}{Issues: issues},
			)
			return
		}
		if err != nil && err.Error() == "at least one supported settings field is required" {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "CONFIG_WRITE_FAILED", "failed to update settings", nil)
	}
}

func timezoneOptions() []settingOption {
	values := []string{
		"Local",
		"UTC",
		"America/New_York",
		"America/Chicago",
		"America/Denver",
		"America/Los_Angeles",
		"America/Sao_Paulo",
		"America/Argentina/Buenos_Aires",
		"America/Mexico_City",
		"America/Toronto",
		"America/Vancouver",
		"Europe/London",
		"Europe/Paris",
		"Europe/Berlin",
		"Europe/Madrid",
		"Europe/Rome",
		"Europe/Amsterdam",
		"Europe/Moscow",
		"Europe/Istanbul",
		"Asia/Tokyo",
		"Asia/Shanghai",
		"Asia/Kolkata",
		"Asia/Singapore",
		"Asia/Seoul",
		"Asia/Dubai",
		"Australia/Sydney",
		"Australia/Melbourne",
		"Pacific/Auckland",
	}
	options := make([]settingOption, 0, len(values))
	for _, value := range values {
		options = append(options, settingOption{Value: value, Label: value})
	}
	return options
}

func localeOptions() []settingOption {
	options := make([]settingOption, 0, len(config.SupportedLocales()))
	for _, value := range config.SupportedLocales() {
		options = append(options, settingOption{Value: value, Label: localeLabel(value)})
	}
	return options
}

func localeLabel(value string) string {
	switch value {
	case "":
		return "Browser default"
	case "en-US":
		return "English (US)"
	case "en-GB":
		return "English (UK)"
	case "pt-BR":
		return "Português (Brasil)"
	case "pt-PT":
		return "Português (Portugal)"
	case "es-ES":
		return "Español (España)"
	case "es-MX":
		return "Español (México)"
	case "fr-FR":
		return "Français"
	case "de-DE":
		return "Deutsch"
	case "it-IT":
		return "Italiano"
	case "nl-NL":
		return "Nederlands"
	case "ja-JP":
		return "日本語"
	case "zh-CN":
		return "中文 (简体)"
	case "ko-KR":
		return "한국어"
	case "ru-RU":
		return "Русский"
	case "tr-TR":
		return "Türkçe"
	case "ar-SA":
		return "العربية"
	case "hi-IN":
		return "हिन्दी"
	default:
		return fmt.Sprintf("Locale %s", value)
	}
}
