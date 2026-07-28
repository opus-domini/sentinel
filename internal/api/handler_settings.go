package api

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/daemon"
	"github.com/opus-domini/sentinel/internal/humanize"
	"github.com/opus-domini/sentinel/internal/userswitch"
	"github.com/opus-domini/sentinel/internal/validate"
)

const settingsDeploymentStandalone = "standalone"
const settingsRootAccount = "root"
const defaultSettingsRestartDelay = 750 * time.Millisecond

const (
	secretActionKeep    = "keep"
	secretActionReplace = "replace"
	secretActionClear   = "clear"
)

var errInvalidSettingsRequest = errors.New("invalid settings request")

type deploymentDetector func() ([]daemon.Deployment, error)

var installedDeployments deploymentDetector = daemon.InstalledDeployments
var controlManagedService = daemon.Control

type settingsResponse struct {
	Revision     string               `json:"revision"`
	Metadata     settingsMetadata     `json:"metadata"`
	Deployment   settingsDeployment   `json:"deployment"`
	Restart      settingsRestart      `json:"restart"`
	Experience   settingsExperience   `json:"experience"`
	Operations   settingsOperations   `json:"operations"`
	Integrations settingsIntegrations `json:"integrations"`
	Accounts     settingsAccounts     `json:"accounts"`
	Access       settingsAccess       `json:"access"`
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
	Available   bool     `json:"available"`
	ChangedKeys []string `json:"changedKeys"`
	Command     string   `json:"command,omitempty"`
	BackupPath  string   `json:"backupPath"`
	Instruction string   `json:"instruction"`
}

type settingsRestartAccepted struct {
	Status      string   `json:"status"`
	Scope       string   `json:"scope"`
	ChangedKeys []string `json:"changedKeys"`
}

type settingsExperience struct {
	Timezone stringSetting `json:"timezone"`
	Locale   stringSetting `json:"locale"`
}

type settingsOperations struct {
	Watchtower settingsWatchtower `json:"watchtower"`
	Runbooks   settingsRunbooks   `json:"runbooks"`
	Log        settingsLog        `json:"log"`
}

type settingsWatchtower struct {
	Enabled        boolSetting    `json:"enabled"`
	TickInterval   stringSetting  `json:"tickInterval"`
	CaptureLines   integerSetting `json:"captureLines"`
	CaptureTimeout stringSetting  `json:"captureTimeout"`
	JournalRows    integerSetting `json:"journalRows"`
}

type settingsRunbooks struct {
	MaxConcurrent integerSetting `json:"maxConcurrent"`
}

type settingsLog struct {
	Level stringSetting `json:"level"`
}

type settingsIntegrations struct {
	MCP          settingsMCP          `json:"mcp"`
	HealthReport settingsHealthReport `json:"healthReport"`
}

type settingsMCP struct {
	Enabled                boolSetting      `json:"enabled"`
	Token                  sensitiveSetting `json:"token"`
	RuntimeTokenConfigured bool             `json:"runtimeTokenConfigured"`
	Endpoint               string           `json:"endpoint"`
}

type settingsHealthReport struct {
	Schedule       stringSetting    `json:"schedule"`
	WebhookURL     sensitiveSetting `json:"webhookUrl"`
	NextActivation string           `json:"nextActivation,omitempty"`
}

type settingsAccounts struct {
	ProcessUser        string                     `json:"processUser"`
	ProcessIsRoot      bool                       `json:"processIsRoot"`
	InventoryAvailable bool                       `json:"inventoryAvailable"`
	Users              []settingsSystemUser       `json:"users"`
	AllowedUsers       stringListSetting          `json:"allowedUsers"`
	AllowRootTarget    boolSetting                `json:"allowRootTarget"`
	UserSwitchMethod   stringSetting              `json:"userSwitchMethod"`
	MethodCapabilities []settingsMethodCapability `json:"methodCapabilities"`
	PrivilegeGuidance  string                     `json:"privilegeGuidance"`
}

type settingsSystemUser struct {
	Name        string `json:"name"`
	ProcessUser bool   `json:"processUser"`
	Root        bool   `json:"root"`
	Allowed     bool   `json:"allowed"`
}

type settingsMethodCapability struct {
	Value     string `json:"value"`
	Label     string `json:"label"`
	Available bool   `json:"available"`
	Detail    string `json:"detail"`
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
	Min         string          `json:"min,omitempty"`
	Max         string          `json:"max,omitempty"`
}

type boolValidation struct {
	Required bool `json:"required"`
}

type integerValidation struct {
	Required bool `json:"required"`
	Min      int  `json:"min"`
	Max      int  `json:"max"`
	Step     int  `json:"step"`
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

type stringListSetting struct {
	PersistedValue *[]string            `json:"persistedValue,omitempty"`
	EffectiveValue []string             `json:"effectiveValue"`
	DefaultValue   []string             `json:"defaultValue"`
	Source         config.FieldSource   `json:"source"`
	Editable       bool                 `json:"editable"`
	ApplyMode      config.ApplyMode     `json:"applyMode"`
	RestartPending bool                 `json:"restartPending"`
	Validation     stringListValidation `json:"validation"`
}

type stringListValidation struct {
	Required    bool            `json:"required"`
	AllowCustom bool            `json:"allowCustom"`
	Options     []settingOption `json:"options"`
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

type integerSetting struct {
	PersistedValue *int               `json:"persistedValue,omitempty"`
	EffectiveValue int                `json:"effectiveValue"`
	DefaultValue   int                `json:"defaultValue"`
	Source         config.FieldSource `json:"source"`
	Editable       bool               `json:"editable"`
	ApplyMode      config.ApplyMode   `json:"applyMode"`
	RestartPending bool               `json:"restartPending"`
	Validation     integerValidation  `json:"validation"`
}

type sensitiveValidation struct {
	Required bool   `json:"required"`
	Format   string `json:"format,omitempty"`
}

type sensitiveSetting struct {
	Configured     bool                `json:"configured"`
	Source         config.FieldSource  `json:"source"`
	Editable       bool                `json:"editable"`
	ApplyMode      config.ApplyMode    `json:"applyMode"`
	RestartPending bool                `json:"restartPending"`
	Validation     sensitiveValidation `json:"validation"`
}

type patchSettingsRequest struct {
	Experience   *patchExperience   `json:"experience,omitempty"`
	Operations   *patchOperations   `json:"operations,omitempty"`
	Integrations *patchIntegrations `json:"integrations,omitempty"`
	Accounts     *patchAccounts     `json:"accounts,omitempty"`
	Access       *patchAccess       `json:"access,omitempty"`
}

type patchExperience struct {
	Timezone *string `json:"timezone,omitempty"`
	Locale   *string `json:"locale,omitempty"`
}

type patchOperations struct {
	Watchtower *patchWatchtower `json:"watchtower,omitempty"`
	Runbooks   *patchRunbooks   `json:"runbooks,omitempty"`
	Log        *patchLog        `json:"log,omitempty"`
}

type patchWatchtower struct {
	Enabled        *bool   `json:"enabled,omitempty"`
	TickInterval   *string `json:"tickInterval,omitempty"`
	CaptureLines   *int    `json:"captureLines,omitempty"`
	CaptureTimeout *string `json:"captureTimeout,omitempty"`
	JournalRows    *int    `json:"journalRows,omitempty"`
}

type patchRunbooks struct {
	MaxConcurrent *int `json:"maxConcurrent,omitempty"`
}

type patchLog struct {
	Level *string `json:"level,omitempty"`
}

type patchIntegrations struct {
	MCP          *patchMCP          `json:"mcp,omitempty"`
	HealthReport *patchHealthReport `json:"healthReport,omitempty"`
}

type patchMCP struct {
	Enabled *bool `json:"enabled,omitempty"`
}

type patchHealthReport struct {
	Schedule   *string              `json:"schedule,omitempty"`
	WebhookURL *patchSecretMutation `json:"webhookUrl,omitempty"`
}

type patchSecretMutation struct {
	Action string `json:"action"`
	Value  string `json:"value,omitempty"`
}

type patchAccounts struct {
	AllowedUsers     *[]string `json:"allowedUsers,omitempty"`
	AllowRootTarget  *bool     `json:"allowRootTarget,omitempty"`
	UserSwitchMethod *string   `json:"userSwitchMethod,omitempty"`
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
	current, err := h.configService.Read()
	if err != nil {
		writeSettingsError(w, err)
		return
	}
	if err := h.validateAccessRequest(req.Access); err != nil {
		writeSettingsError(w, err)
		return
	}
	keys, mutate, err := h.settingsMutation(req, current)
	if err != nil {
		writeSettingsError(w, err)
		return
	}
	state, err := h.configService.UpdateWithPreflight(
		r.Context(),
		revision,
		keys,
		mutate,
		h.accessCandidatePreflight(r, req.Access),
	)
	if err != nil {
		writeSettingsError(w, err)
		return
	}
	response := h.settingsResponse(state)
	w.Header().Set("ETag", quoteETag(state.Revision))
	writeData(w, http.StatusOK, response)
}

func (h *Handler) restartSettings(w http.ResponseWriter, r *http.Request) {
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
	state, err := h.configService.Read()
	if err != nil {
		writeSettingsError(w, err)
		return
	}
	if state.Revision != revision {
		writeError(w, http.StatusPreconditionFailed, "CONFIG_CONFLICT", "settings changed since they were loaded", nil)
		return
	}
	response := h.settingsResponse(state)
	if !response.Restart.Required {
		writeError(w, http.StatusConflict, "RESTART_NOT_REQUIRED", "no saved settings are waiting for restart", nil)
		return
	}
	if !response.Restart.Available || h.settingsControl == nil {
		writeError(
			w,
			http.StatusConflict,
			"RESTART_UNAVAILABLE",
			"Sentinel cannot restart this deployment from the SPA",
			nil,
		)
		return
	}
	if !h.restartScheduled.CompareAndSwap(false, true) {
		writeError(
			w,
			http.StatusConflict,
			"RESTART_ALREADY_SCHEDULED",
			"Sentinel restart is already scheduled",
			nil,
		)
		return
	}

	accepted := settingsRestartAccepted{
		Status:      "accepted",
		Scope:       response.Deployment.Scope,
		ChangedKeys: slices.Clone(response.Restart.ChangedKeys),
	}
	writeData(w, http.StatusAccepted, accepted)
	h.scheduleSettingsRestart(response.Deployment.Scope)
}

func (h *Handler) scheduleSettingsRestart(scope string) {
	delay := h.settingsRestartIn
	if delay <= 0 {
		delay = defaultSettingsRestartDelay
	}
	time.AfterFunc(delay, func() {
		if err := h.settingsControl("restart", scope); err != nil {
			h.restartScheduled.Store(false)
			slog.Error("settings service restart failed", "scope", scope, "err", err)
		}
	})
}

func (h *Handler) settingsMutation(
	req patchSettingsRequest,
	current config.State,
) ([]string, func(*config.Config) error, error) {
	if err := validateIntegrationSecretMutations(req.Integrations); err != nil {
		return nil, nil, err
	}
	if err := validateAccessSecretMutation(req.Access); err != nil {
		return nil, nil, err
	}
	keys := settingsChangedKeys(req, current)
	if len(keys) == 0 && !hasExplicitSecretKeep(req.Integrations, req.Access) {
		return nil, nil, errors.New("at least one supported settings field is required")
	}

	return keys, func(candidate *config.Config) error {
		if err := h.applyExperienceSettings(candidate, req.Experience); err != nil {
			return err
		}
		if err := h.applyOperationsSettings(candidate, req.Operations); err != nil {
			return err
		}
		if err := h.applyIntegrationSettings(candidate, req.Integrations); err != nil {
			return err
		}
		if err := h.applyAccountSettings(candidate, req.Accounts, current); err != nil {
			return err
		}
		return h.applyAccessSettings(candidate, req.Access)
	}, nil
}

func settingsChangedKeys(req patchSettingsRequest, current config.State) []string {
	keys := experienceSettingsKeys(req.Experience)
	keys = append(keys, operationsSettingsKeys(req.Operations)...)
	keys = append(keys, integrationSettingsKeys(req.Integrations)...)
	keys = append(keys, accountSettingsKeys(req.Accounts, current)...)
	return append(keys, accessSettingsKeys(req.Access)...)
}

func experienceSettingsKeys(req *patchExperience) []string {
	if req == nil {
		return nil
	}
	var keys []string
	if req.Timezone != nil {
		keys = append(keys, config.FieldServerTimezone)
	}
	if req.Locale != nil {
		keys = append(keys, config.FieldServerLocale)
	}
	return keys
}

func operationsSettingsKeys(req *patchOperations) []string {
	if req == nil {
		return nil
	}
	var keys []string
	if req.Watchtower != nil {
		if req.Watchtower.Enabled != nil {
			keys = append(keys, config.FieldWatchtowerEnabled)
		}
		if req.Watchtower.TickInterval != nil {
			keys = append(keys, config.FieldWatchtowerTickInterval)
		}
		if req.Watchtower.CaptureLines != nil {
			keys = append(keys, config.FieldWatchtowerCaptureLines)
		}
		if req.Watchtower.CaptureTimeout != nil {
			keys = append(keys, config.FieldWatchtowerCaptureTimeout)
		}
		if req.Watchtower.JournalRows != nil {
			keys = append(keys, config.FieldWatchtowerJournalRows)
		}
	}
	if req.Runbooks != nil && req.Runbooks.MaxConcurrent != nil {
		keys = append(keys, config.FieldRunbooksMaxConcurrent)
	}
	if req.Log != nil && req.Log.Level != nil {
		keys = append(keys, config.FieldLogLevel)
	}
	return keys
}

func integrationSettingsKeys(req *patchIntegrations) []string {
	if req == nil {
		return nil
	}
	var keys []string
	if req.MCP != nil {
		if req.MCP.Enabled != nil {
			keys = append(keys, config.FieldMCPEnabled)
		}
	}
	if req.HealthReport != nil {
		if req.HealthReport.Schedule != nil {
			keys = append(keys, config.FieldHealthReportSchedule)
		}
		if secretMutationChangesValue(req.HealthReport.WebhookURL) {
			keys = append(keys, config.FieldHealthReportWebhookURL)
		}
	}
	return keys
}

func accountSettingsKeys(req *patchAccounts, current config.State) []string {
	if req == nil {
		return nil
	}
	var keys []string
	allowedField, _ := current.Field(config.FieldMultiUserAllowedUsers)
	if req.AllowedUsers != nil ||
		req.AllowRootTarget != nil && allowedField.Editable && len(current.Effective.MultiUser.AllowedUsers) > 0 {
		keys = append(keys, config.FieldMultiUserAllowedUsers)
	}
	if req.AllowRootTarget != nil {
		keys = append(keys, config.FieldMultiUserAllowRootTarget)
	}
	if req.UserSwitchMethod != nil {
		keys = append(keys, config.FieldMultiUserUserSwitchMethod)
	}
	return keys
}

func (h *Handler) applyExperienceSettings(candidate *config.Config, req *patchExperience) error {
	if req == nil {
		return nil
	}
	if req.Timezone != nil {
		timezone := strings.TrimSpace(*req.Timezone)
		if timezone == "" {
			return config.ValidationError{Path: h.configService.Path(), Issues: []string{"server.timezone is required"}}
		}
		candidate.Server.Timezone = timezone
	}
	if req.Locale != nil {
		candidate.Server.Locale = strings.TrimSpace(*req.Locale)
	}
	return nil
}

func (h *Handler) applyOperationsSettings(candidate *config.Config, req *patchOperations) error {
	if req == nil {
		return nil
	}
	if err := h.applyWatchtowerSettings(candidate, req.Watchtower); err != nil {
		return err
	}
	if req.Runbooks != nil && req.Runbooks.MaxConcurrent != nil {
		candidate.Runbooks.MaxConcurrent = *req.Runbooks.MaxConcurrent
	}
	if req.Log != nil && req.Log.Level != nil {
		candidate.Log.Level = strings.TrimSpace(*req.Log.Level)
	}
	return nil
}

func (h *Handler) applyWatchtowerSettings(candidate *config.Config, req *patchWatchtower) error {
	if req == nil {
		return nil
	}
	if req.Enabled != nil {
		candidate.Watchtower.Enabled = *req.Enabled
	}
	if req.TickInterval != nil {
		value, err := parseSettingsDuration(config.FieldWatchtowerTickInterval, *req.TickInterval, h.configService.Path())
		if err != nil {
			return err
		}
		candidate.Watchtower.TickInterval = value
	}
	if req.CaptureLines != nil {
		candidate.Watchtower.CaptureLines = *req.CaptureLines
	}
	if req.CaptureTimeout != nil {
		value, err := parseSettingsDuration(config.FieldWatchtowerCaptureTimeout, *req.CaptureTimeout, h.configService.Path())
		if err != nil {
			return err
		}
		candidate.Watchtower.CaptureTimeout = value
	}
	if req.JournalRows != nil {
		candidate.Watchtower.JournalRows = *req.JournalRows
	}
	return nil
}

func (h *Handler) applyIntegrationSettings(candidate *config.Config, req *patchIntegrations) error {
	if req == nil {
		return nil
	}
	if req.MCP != nil {
		if req.MCP.Enabled != nil {
			candidate.MCP.Enabled = *req.MCP.Enabled
		}
	}
	if req.HealthReport != nil {
		if req.HealthReport.Schedule != nil {
			candidate.HealthReport.Schedule = strings.TrimSpace(*req.HealthReport.Schedule)
		}
		applySecretMutation(&candidate.HealthReport.WebhookURL, req.HealthReport.WebhookURL)
	}
	return nil
}

func (h *Handler) applyAccountSettings(
	candidate *config.Config,
	req *patchAccounts,
	current config.State,
) error {
	if req == nil {
		return nil
	}

	systemUsers := h.systemUsers()
	finalRootAllowed := current.Effective.MultiUser.AllowRootTarget
	if req.AllowRootTarget != nil {
		finalRootAllowed = *req.AllowRootTarget
		if finalRootAllowed && !slices.Contains(systemUsers, settingsRootAccount) {
			return h.accountValidationError("multi_user.allow_root_target cannot be enabled because root was not detected")
		}
		candidate.MultiUser.AllowRootTarget = finalRootAllowed
	}

	allowedField, _ := current.Field(config.FieldMultiUserAllowedUsers)
	switch {
	case req.AllowedUsers != nil:
		allowed, err := h.validateAllowedUsers(*req.AllowedUsers, systemUsers)
		if err != nil {
			return err
		}
		if slices.Contains(allowed, settingsRootAccount) && !finalRootAllowed {
			if req.AllowRootTarget == nil || *req.AllowRootTarget {
				return h.accountValidationError("multi_user.allowed_users cannot include root while multi_user.allow_root_target is false")
			}
			allowed = removeString(allowed, settingsRootAccount)
		}
		if req.AllowRootTarget != nil && *req.AllowRootTarget && len(allowed) > 0 &&
			!slices.Contains(allowed, settingsRootAccount) {
			allowed = append(allowed, settingsRootAccount)
			slices.Sort(allowed)
		}
		candidate.MultiUser.AllowedUsers = allowed
	case req.AllowRootTarget != nil && allowedField.Editable &&
		len(current.Effective.MultiUser.AllowedUsers) > 0:
		allowed := slices.Clone(candidate.MultiUser.AllowedUsers)
		if *req.AllowRootTarget {
			if !slices.Contains(allowed, settingsRootAccount) {
				allowed = append(allowed, settingsRootAccount)
				slices.Sort(allowed)
			}
		} else {
			allowed = removeString(allowed, settingsRootAccount)
		}
		candidate.MultiUser.AllowedUsers = allowed
	}

	if req.UserSwitchMethod != nil {
		method, err := userswitch.ParseMethod(*req.UserSwitchMethod)
		if err != nil {
			return h.accountValidationError("multi_user.user_switch_method " + err.Error())
		}
		candidate.MultiUser.UserSwitchMethod = method
	}
	return nil
}

func (h *Handler) validateAllowedUsers(values, systemUsers []string) ([]string, error) {
	if len(values) > 0 && len(systemUsers) == 0 {
		return nil, h.accountValidationError("multi_user.allowed_users cannot be changed because the system user inventory is unavailable")
	}
	seen := make(map[string]struct{}, len(values))
	allowed := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" || value != raw {
			return nil, h.accountValidationError("multi_user.allowed_users entries must be non-empty exact account names")
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, h.accountValidationError("multi_user.allowed_users contains duplicate account " + value)
		}
		if !slices.Contains(systemUsers, value) {
			return nil, h.accountValidationError("multi_user.allowed_users contains unknown account " + value)
		}
		seen[value] = struct{}{}
		allowed = append(allowed, value)
	}
	slices.Sort(allowed)
	return allowed, nil
}

func (h *Handler) accountValidationError(issue string) error {
	return config.ValidationError{Path: h.configService.Path(), Issues: []string{issue}}
}

func removeString(values []string, target string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if value != target {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func validateIntegrationSecretMutations(req *patchIntegrations) error {
	if req == nil {
		return nil
	}
	if req.HealthReport != nil {
		if err := validateSecretMutation("integrations.healthReport.webhookUrl", req.HealthReport.WebhookURL); err != nil {
			return err
		}
	}
	return nil
}

func validateSecretMutation(field string, mutation *patchSecretMutation) error {
	if mutation == nil {
		return nil
	}
	switch mutation.Action {
	case secretActionKeep, secretActionClear:
		if mutation.Value != "" {
			return fmt.Errorf("%w: %s does not accept a value for action %q", errInvalidSettingsRequest, field, mutation.Action)
		}
	case secretActionReplace:
		if strings.TrimSpace(mutation.Value) == "" {
			return fmt.Errorf("%w: %s requires a non-empty replacement", errInvalidSettingsRequest, field)
		}
	default:
		return fmt.Errorf("%w: %s action must be keep, replace, or clear", errInvalidSettingsRequest, field)
	}
	return nil
}

func secretMutationChangesValue(mutation *patchSecretMutation) bool {
	return mutation != nil && (mutation.Action == secretActionReplace || mutation.Action == secretActionClear)
}

func hasExplicitSecretKeep(integrations *patchIntegrations, access *patchAccess) bool {
	integrationKeep := integrations != nil &&
		integrations.HealthReport != nil &&
		integrations.HealthReport.WebhookURL != nil &&
		integrations.HealthReport.WebhookURL.Action == secretActionKeep
	accessKeep := access != nil &&
		access.Token != nil &&
		access.Token.Action == secretActionKeep
	return integrationKeep || accessKeep
}

func applySecretMutation(target *string, mutation *patchSecretMutation) {
	if mutation == nil {
		return
	}
	switch mutation.Action {
	case secretActionReplace:
		*target = strings.TrimSpace(mutation.Value)
	case secretActionClear:
		*target = ""
	}
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
	watchtowerEnabledField, _ := state.Field(config.FieldWatchtowerEnabled)
	watchtowerTickField, _ := state.Field(config.FieldWatchtowerTickInterval)
	watchtowerLinesField, _ := state.Field(config.FieldWatchtowerCaptureLines)
	watchtowerTimeoutField, _ := state.Field(config.FieldWatchtowerCaptureTimeout)
	watchtowerJournalField, _ := state.Field(config.FieldWatchtowerJournalRows)
	runbooksConcurrentField, _ := state.Field(config.FieldRunbooksMaxConcurrent)
	logLevelField, _ := state.Field(config.FieldLogLevel)
	serverTokenField, _ := state.Field(config.FieldServerToken)
	mcpField, _ := state.Field(config.FieldMCPEnabled)
	healthReportScheduleField, _ := state.Field(config.FieldHealthReportSchedule)
	healthReportWebhookField, _ := state.Field(config.FieldHealthReportWebhookURL)
	allowedUsersField, _ := state.Field(config.FieldMultiUserAllowedUsers)
	allowRootField, _ := state.Field(config.FieldMultiUserAllowRootTarget)
	switchMethodField, _ := state.Field(config.FieldMultiUserUserSwitchMethod)
	serverHostField, _ := state.Field(config.FieldServerHost)
	serverPortField, _ := state.Field(config.FieldServerPort)
	allowedOriginsField, _ := state.Field(config.FieldServerAllowedOrigins)
	trustedProxiesField, _ := state.Field(config.FieldServerTrustedProxies)
	cookieSecureField, _ := state.Field(config.FieldServerCookieSecure)
	allowInsecureCookieField, _ := state.Field(config.FieldServerAllowInsecureCookie)
	systemUsers := h.systemUsers()
	processUser, processIsRoot := settingsProcessIdentity()
	deploymentRestart := settingsRestartForDeployment(
		len(changedKeys) > 0,
		changedKeys,
		h.configService.BackupPath(),
		deployment,
	)
	return settingsResponse{
		Revision:   state.Revision,
		Metadata:   settingsMetadata{Version: h.settingsVersion()},
		Deployment: deployment,
		Restart:    deploymentRestart,
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
		Operations: settingsOperations{
			Watchtower: settingsWatchtower{
				Enabled: buildBoolSetting(
					watchtowerEnabledField,
					state.Persisted.Watchtower.Enabled,
					state.Effective.Watchtower.Enabled,
					state.Default.Watchtower.Enabled,
				),
				TickInterval: buildStringSetting(
					watchtowerTickField,
					humanize.Duration(state.Persisted.Watchtower.TickInterval),
					humanize.Duration(state.Effective.Watchtower.TickInterval),
					humanize.Duration(state.Default.Watchtower.TickInterval),
					stringValidation{
						Required:    true,
						Format:      "duration",
						AllowCustom: true,
						Options:     []settingOption{},
						Min:         humanize.Duration(config.MinWatchtowerTickInterval),
						Max:         humanize.Duration(config.MaxWatchtowerTickInterval),
					},
				),
				CaptureLines: buildIntegerSetting(
					watchtowerLinesField,
					state.Persisted.Watchtower.CaptureLines,
					state.Effective.Watchtower.CaptureLines,
					state.Default.Watchtower.CaptureLines,
					config.MinWatchtowerCaptureLines,
					config.MaxWatchtowerCaptureLines,
				),
				CaptureTimeout: buildStringSetting(
					watchtowerTimeoutField,
					humanize.Duration(state.Persisted.Watchtower.CaptureTimeout),
					humanize.Duration(state.Effective.Watchtower.CaptureTimeout),
					humanize.Duration(state.Default.Watchtower.CaptureTimeout),
					stringValidation{
						Required:    true,
						Format:      "duration",
						AllowCustom: true,
						Options:     []settingOption{},
						Min:         humanize.Duration(config.MinWatchtowerCaptureTimeout),
						Max:         humanize.Duration(config.MaxWatchtowerCaptureTimeout),
					},
				),
				JournalRows: buildIntegerSetting(
					watchtowerJournalField,
					state.Persisted.Watchtower.JournalRows,
					state.Effective.Watchtower.JournalRows,
					state.Default.Watchtower.JournalRows,
					config.MinWatchtowerJournalRows,
					config.MaxWatchtowerJournalRows,
				),
			},
			Runbooks: settingsRunbooks{
				MaxConcurrent: buildIntegerSetting(
					runbooksConcurrentField,
					state.Persisted.Runbooks.MaxConcurrent,
					state.Effective.Runbooks.MaxConcurrent,
					state.Default.Runbooks.MaxConcurrent,
					config.MinRunbooksMaxConcurrent,
					config.MaxRunbooksMaxConcurrent,
				),
			},
			Log: settingsLog{
				Level: buildStringSetting(
					logLevelField,
					state.Persisted.Log.Level,
					state.Effective.Log.Level,
					state.Default.Log.Level,
					stringValidation{
						Required:    true,
						AllowCustom: false,
						Options:     logLevelOptions(),
					},
				),
			},
		},
		Integrations: settingsIntegrations{
			MCP: settingsMCP{
				Enabled: buildBoolSetting(
					mcpField,
					state.Persisted.MCP.Enabled,
					state.Effective.MCP.Enabled,
					state.Default.MCP.Enabled,
				),
				Token:                  buildSensitiveSetting(serverTokenField, ""),
				RuntimeTokenConfigured: h.settings.TokenConfigured(),
				Endpoint:               "/mcp",
			},
			HealthReport: settingsHealthReport{
				Schedule: buildStringSetting(
					healthReportScheduleField,
					state.Persisted.HealthReport.Schedule,
					state.Effective.HealthReport.Schedule,
					state.Default.HealthReport.Schedule,
					stringValidation{
						Required:    false,
						Format:      "cron",
						AllowCustom: true,
						Options:     []settingOption{},
					},
				),
				WebhookURL: buildSensitiveSetting(healthReportWebhookField, "url"),
				NextActivation: nextHealthReportActivation(
					state.Effective.HealthReport.Schedule,
					state.Effective.Server.Timezone,
					healthReportWebhookField.Configured,
				),
			},
		},
		Accounts: settingsAccounts{
			ProcessUser:        processUser,
			ProcessIsRoot:      processIsRoot,
			InventoryAvailable: len(systemUsers) > 0,
			Users: settingsSystemUsers(
				systemUsers,
				processUser,
				state.Effective.MultiUser.AllowedUsers,
				state.Effective.MultiUser.AllowRootTarget,
			),
			AllowedUsers: buildStringListSetting(
				allowedUsersField,
				state.Persisted.MultiUser.AllowedUsers,
				state.Effective.MultiUser.AllowedUsers,
				state.Default.MultiUser.AllowedUsers,
				stringListValidation{
					Required:    false,
					AllowCustom: false,
					Options:     accountOptions(systemUsers),
				},
			),
			AllowRootTarget: buildBoolSetting(
				allowRootField,
				state.Persisted.MultiUser.AllowRootTarget,
				state.Effective.MultiUser.AllowRootTarget,
				state.Default.MultiUser.AllowRootTarget,
			),
			UserSwitchMethod: buildStringSetting(
				switchMethodField,
				state.Persisted.MultiUser.UserSwitchMethod,
				state.Effective.MultiUser.UserSwitchMethod,
				state.Default.MultiUser.UserSwitchMethod,
				stringValidation{
					Required:    true,
					AllowCustom: false,
					Options: []settingOption{
						{Value: userswitch.MethodSudo, Label: "sudo"},
						{Value: userswitch.MethodSystemdRun, Label: "systemd-run"},
					},
				},
			),
			MethodCapabilities: h.settingsMethodCapabilities(),
			PrivilegeGuidance:  "Sentinel detects executables but cannot grant sudo permissions. Configure passwordless policy for the process user and selected targets outside Sentinel.",
		},
		Access: settingsAccess{
			Listener: settingsListener{
				Host: buildStringSetting(
					serverHostField,
					state.Persisted.Server.Host,
					state.Effective.Server.Host,
					state.Default.Server.Host,
					stringValidation{
						Required:    true,
						Format:      "listen-host",
						AllowCustom: true,
						Options:     []settingOption{},
					},
				),
				Port: buildIntegerSetting(
					serverPortField,
					state.Persisted.Server.Port,
					state.Effective.Server.Port,
					state.Default.Server.Port,
					1,
					65535,
				),
				Classification: settingsListenerClassification(state.Effective.Server.Host),
				Address:        state.Effective.Address(),
			},
			Authentication: settingsAuthentication{
				Token:                  buildSensitiveSetting(serverTokenField, ""),
				RuntimeTokenConfigured: h.settings.TokenConfigured(),
			},
			Origins: settingsOrigins{
				Allowed: buildStringListSetting(
					allowedOriginsField,
					state.Persisted.Server.AllowedOrigins,
					state.Effective.Server.AllowedOrigins,
					state.Default.Server.AllowedOrigins,
					stringListValidation{
						Required:    false,
						AllowCustom: true,
						Options:     []settingOption{},
					},
				),
			},
			Proxies: settingsProxies{
				Trusted: buildStringListSetting(
					trustedProxiesField,
					state.Persisted.Server.TrustedProxies,
					state.Effective.Server.TrustedProxies,
					state.Default.Server.TrustedProxies,
					stringListValidation{
						Required:    false,
						AllowCustom: true,
						Options:     []settingOption{},
					},
				),
			},
			Cookies: settingsCookies{
				Secure: buildStringSetting(
					cookieSecureField,
					state.Persisted.Server.CookieSecure,
					state.Effective.Server.CookieSecure,
					state.Default.Server.CookieSecure,
					stringValidation{
						Required:    true,
						AllowCustom: false,
						Options: []settingOption{
							{Value: config.CookieSecureAuto, Label: "Auto"},
							{Value: config.CookieSecureAlways, Label: "Always secure"},
							{Value: config.CookieSecureNever, Label: "Never secure"},
						},
					},
				),
				AllowInsecure: buildBoolSetting(
					allowInsecureCookieField,
					state.Persisted.Server.AllowInsecureCookie,
					state.Effective.Server.AllowInsecureCookie,
					state.Default.Server.AllowInsecureCookie,
				),
			},
			Recovery: settingsRecoveryForDeployment(
				h.configService.Path(),
				h.configService.BackupPath(),
				deployment,
				deploymentRestart,
			),
		},
		Diagnostics: settingsDiagnostics{
			ConfigExists:         state.Exists,
			EnvironmentOwnedKeys: environmentKeys,
			ReadOnlyKeys:         readOnlyKeys,
			DeploymentDetection:  detection,
		},
	}
}

func (h *Handler) systemUsers() []string {
	if h == nil || h.guard == nil {
		return []string{}
	}
	users := slices.Clone(h.guard.SystemUsers())
	slices.Sort(users)
	return slices.Compact(users)
}

func settingsProcessIdentity() (string, bool) {
	isRoot := osGeteuid() == 0
	current, err := osCurrentUser()
	if err != nil || current == nil {
		if isRoot {
			return settingsRootAccount, true
		}
		return "", false
	}
	name := strings.TrimSpace(current.Username)
	return name, isRoot
}

func settingsSystemUsers(
	systemUsers []string,
	processUser string,
	explicitAllowed []string,
	allowRoot bool,
) []settingsSystemUser {
	users := make([]settingsSystemUser, 0, len(systemUsers))
	for _, name := range systemUsers {
		allowed := len(explicitAllowed) == 0 || slices.Contains(explicitAllowed, name)
		if name == settingsRootAccount && !allowRoot {
			allowed = false
		}
		users = append(users, settingsSystemUser{
			Name:        name,
			ProcessUser: name == processUser,
			Root:        name == settingsRootAccount,
			Allowed:     allowed,
		})
	}
	return users
}

func accountOptions(systemUsers []string) []settingOption {
	options := make([]settingOption, 0, len(systemUsers))
	for _, name := range systemUsers {
		options = append(options, settingOption{Value: name, Label: name})
	}
	return options
}

func (h *Handler) settingsMethodCapabilities() []settingsMethodCapability {
	provider := userswitch.Capabilities
	if h != nil && h.switchCapabilities != nil {
		provider = h.switchCapabilities
	}
	capabilities := provider()
	result := make([]settingsMethodCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		result = append(result, settingsMethodCapability{
			Value:     capability.Method,
			Label:     capability.Method,
			Available: capability.Available,
			Detail:    capability.Detail,
		})
	}
	return result
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
		Available:   settingsDeploymentRestartAvailable(deployment),
		ChangedKeys: slices.Clone(changedKeys),
		BackupPath:  backupPath,
	}
	switch deployment.Scope {
	case daemon.ScopeUser:
		restart.Command = "sentinel service restart --scope user"
		restart.Instruction = "Restart Sentinel here after reviewing the saved configuration."
	case daemon.ScopeSystem:
		restart.Command = "sudo sentinel service restart --scope system"
		restart.Instruction = "Restart Sentinel here after reviewing the saved configuration."
	default:
		restart.Instruction = "Restart Sentinel with the external supervisor that owns this process."
	}
	return restart
}

func settingsDeploymentRestartAvailable(deployment settingsDeployment) bool {
	if deployment.RuntimeMode != "service" {
		return false
	}
	if deployment.Scope != daemon.ScopeUser && deployment.Scope != daemon.ScopeSystem {
		return false
	}
	return daemon.RequireScopeAccess(deployment.Scope) == nil
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

func buildStringListSetting(
	field config.FieldState,
	persisted []string,
	effective []string,
	defaultValue []string,
	validation stringListValidation,
) stringListSetting {
	setting := stringListSetting{
		EffectiveValue: slices.Clone(effective),
		DefaultValue:   slices.Clone(defaultValue),
		Source:         field.Source,
		Editable:       field.Editable,
		ApplyMode:      field.ApplyMode,
		RestartPending: field.RestartPending,
		Validation:     validation,
	}
	if field.Defined {
		value := slices.Clone(persisted)
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

func buildIntegerSetting(
	field config.FieldState,
	persisted int,
	effective int,
	defaultValue int,
	minValue int,
	maxValue int,
) integerSetting {
	setting := integerSetting{
		EffectiveValue: effective,
		DefaultValue:   defaultValue,
		Source:         field.Source,
		Editable:       field.Editable,
		ApplyMode:      field.ApplyMode,
		RestartPending: field.RestartPending,
		Validation: integerValidation{
			Required: true,
			Min:      minValue,
			Max:      maxValue,
			Step:     1,
		},
	}
	if field.Defined {
		value := persisted
		setting.PersistedValue = &value
	}
	return setting
}

func buildSensitiveSetting(field config.FieldState, format string) sensitiveSetting {
	return sensitiveSetting{
		Configured:     field.Configured,
		Source:         field.Source,
		Editable:       field.Editable,
		ApplyMode:      field.ApplyMode,
		RestartPending: field.RestartPending,
		Validation: sensitiveValidation{
			Required: false,
			Format:   format,
		},
	}
}

func parseSettingsDuration(key, raw, path string) (time.Duration, error) {
	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return 0, config.ValidationError{
			Path:   path,
			Issues: []string{key + " must be a positive duration such as 150ms, 1s, or 1m"},
		}
	}
	return value, nil
}

func nextHealthReportActivation(schedule, timezone string, webhookConfigured bool) string {
	schedule = strings.TrimSpace(schedule)
	if schedule == "" || !webhookConfigured {
		return ""
	}
	parsed, err := validate.ParseCron(schedule)
	if err != nil {
		return ""
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		location = time.UTC
	}
	return parsed.Next(time.Now().In(location)).Format(time.RFC3339)
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
	case errors.Is(err, errInvalidSettingsRequest):
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
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

func logLevelOptions() []settingOption {
	return []settingOption{
		{Value: "debug", Label: "Debug"},
		{Value: "info", Label: "Info"},
		{Value: "warn", Label: "Warn"},
		{Value: "error", Label: "Error"},
	}
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
