package config

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// FieldSource identifies the layer that owns an effective config value.
type FieldSource string

const (
	// FieldSourceDefault marks a field inherited from deployment defaults.
	FieldSourceDefault FieldSource = "default"
	// FieldSourceFile marks a field explicitly defined in config.toml.
	FieldSourceFile FieldSource = "file"
	// FieldSourceEnvironment marks a field controlled by a SENTINEL_* override.
	FieldSourceEnvironment FieldSource = "environment"
)

// ApplyMode describes how a setting reaches the running process.
type ApplyMode string

const (
	// ApplyModeLive marks a field fully applied by the running process.
	ApplyModeLive ApplyMode = "live"
	// ApplyModePartial marks a field with both live and restart-only consumers.
	ApplyModePartial ApplyMode = "partial"
	// ApplyModeRestart marks a field applied only after process restart.
	ApplyModeRestart ApplyMode = "restart"
)

// Canonical config field keys shared by the state service and typed API.
const (
	FieldVersion                   = "version"
	FieldServerHost                = "server.host"
	FieldServerPort                = "server.port"
	FieldServerToken               = "server.token"
	FieldServerAllowedOrigins      = "server.allowed_origins"
	FieldServerTrustedProxies      = "server.trusted_proxies"
	FieldServerCookieSecure        = "server.cookie_secure"
	FieldServerAllowInsecureCookie = "server.allow_insecure_cookie"
	FieldServerTimezone            = "server.timezone"
	FieldServerLocale              = "server.locale"
	FieldStoragePath               = "storage.path"
	FieldLogLevel                  = "log.level"
	FieldLogPath                   = "log.path"
	FieldHealthReportWebhookURL    = "health_report.webhook_url"
	FieldHealthReportSchedule      = "health_report.schedule"
	FieldWatchtowerEnabled         = "watchtower.enabled"
	FieldWatchtowerTickInterval    = "watchtower.tick_interval"
	FieldWatchtowerCaptureLines    = "watchtower.capture_lines"
	FieldWatchtowerCaptureTimeout  = "watchtower.capture_timeout"
	FieldWatchtowerJournalRows     = "watchtower.journal_rows"
	FieldMCPEnabled                = "mcp.enabled"
	FieldRunbooksMaxConcurrent     = "runbooks.max_concurrent"
	FieldMultiUserAllowedUsers     = "multi_user.allowed_users"
	FieldMultiUserAllowRootTarget  = "multi_user.allow_root_target"
	FieldMultiUserUserSwitchMethod = "multi_user.user_switch_method"
)

var configFieldKeys = []string{
	FieldVersion,
	FieldServerHost,
	FieldServerPort,
	FieldServerToken,
	FieldServerAllowedOrigins,
	FieldServerTrustedProxies,
	FieldServerCookieSecure,
	FieldServerAllowInsecureCookie,
	FieldServerTimezone,
	FieldServerLocale,
	FieldStoragePath,
	FieldLogLevel,
	FieldLogPath,
	FieldHealthReportWebhookURL,
	FieldHealthReportSchedule,
	FieldWatchtowerEnabled,
	FieldWatchtowerTickInterval,
	FieldWatchtowerCaptureLines,
	FieldWatchtowerCaptureTimeout,
	FieldWatchtowerJournalRows,
	FieldMCPEnabled,
	FieldRunbooksMaxConcurrent,
	FieldMultiUserAllowedUsers,
	FieldMultiUserAllowRootTarget,
	FieldMultiUserUserSwitchMethod,
}

// FieldState describes ownership and lifecycle without exposing a value.
type FieldState struct {
	Key            string
	Source         FieldSource
	Defined        bool
	Editable       bool
	Sensitive      bool
	Configured     bool
	ApplyMode      ApplyMode
	RestartPending bool
}

// State is one coherent snapshot of config persistence and runtime precedence.
type State struct {
	Path       string
	Exists     bool
	Revision   string
	Default    Config
	Persisted  Config
	Effective  Config
	Fields     map[string]FieldState
	fileFields map[string]bool
	envFields  map[string]bool
}

// FieldKeys returns the stable persisted field inventory.
func FieldKeys() []string {
	return slices.Clone(configFieldKeys)
}

// Field returns metadata for a canonical key.
func (s State) Field(key string) (FieldState, bool) {
	field, ok := s.Fields[key]
	return field, ok
}

func configRevision(exists bool, raw []byte) string {
	hash := sha256.New()
	if exists {
		_, _ = hash.Write([]byte{1})
	} else {
		_, _ = hash.Write([]byte{0})
	}
	_, _ = hash.Write(raw)
	return hex.EncodeToString(hash.Sum(nil))
}

func fileFieldSet(meta toml.MetaData) map[string]bool {
	fields := make(map[string]bool, len(configFieldKeys))
	for _, key := range configFieldKeys {
		fields[key] = meta.IsDefined(strings.Split(key, ".")...)
	}
	return fields
}

func buildFieldStates(state State, baseline Config) map[string]FieldState {
	fields := make(map[string]FieldState, len(configFieldKeys))
	for _, key := range configFieldKeys {
		source := FieldSourceDefault
		if state.fileFields[key] {
			source = FieldSourceFile
		}
		if state.envFields[key] {
			source = FieldSourceEnvironment
		}
		mode := applyModeForField(key, baseline, state.Effective)
		fields[key] = FieldState{
			Key:            key,
			Source:         source,
			Defined:        state.fileFields[key],
			Editable:       source != FieldSourceEnvironment && fieldIsWritable(key),
			Sensitive:      fieldIsSensitive(key),
			Configured:     fieldIsConfigured(key, state.Effective),
			ApplyMode:      mode,
			RestartPending: mode != ApplyModeLive && !reflect.DeepEqual(configFieldValue(baseline, key), configFieldValue(state.Effective, key)),
		}
	}
	return fields
}

func fieldIsWritable(key string) bool {
	switch key {
	case FieldVersion, FieldStoragePath, FieldLogPath:
		return false
	default:
		return true
	}
}

func fieldIsSensitive(key string) bool {
	return key == FieldServerToken || key == FieldHealthReportWebhookURL
}

func fieldIsConfigured(key string, cfg Config) bool {
	switch key {
	case FieldServerToken:
		return strings.TrimSpace(cfg.Server.Token) != ""
	case FieldHealthReportWebhookURL:
		return strings.TrimSpace(cfg.HealthReport.WebhookURL) != ""
	default:
		return true
	}
}

func applyModeForField(key string, baseline, effective Config) ApplyMode {
	switch key {
	case FieldServerLocale:
		return ApplyModeLive
	case FieldServerTimezone:
		return ApplyModePartial
	case FieldMCPEnabled:
		if !effective.MCP.Enabled || strings.TrimSpace(baseline.Server.Token) != "" {
			return ApplyModeLive
		}
		return ApplyModeRestart
	default:
		return ApplyModeRestart
	}
}

func diffConfigKeys(before, after Config) []string {
	keys := make([]string, 0)
	for _, key := range configFieldKeys {
		if !reflect.DeepEqual(configFieldValue(before, key), configFieldValue(after, key)) {
			keys = append(keys, key)
		}
	}
	return keys
}

func configFieldValue(cfg Config, key string) any {
	switch key {
	case FieldVersion:
		return cfg.Version
	case FieldServerHost:
		return cfg.Server.Host
	case FieldServerPort:
		return cfg.Server.Port
	case FieldServerToken:
		return cfg.Server.Token
	case FieldServerAllowedOrigins:
		return cfg.Server.AllowedOrigins
	case FieldServerTrustedProxies:
		return cfg.Server.TrustedProxies
	case FieldServerCookieSecure:
		return cfg.Server.CookieSecure
	case FieldServerAllowInsecureCookie:
		return cfg.Server.AllowInsecureCookie
	case FieldServerTimezone:
		return cfg.Server.Timezone
	case FieldServerLocale:
		return cfg.Server.Locale
	case FieldStoragePath:
		return cfg.Storage.Path
	case FieldLogLevel:
		return cfg.Log.Level
	case FieldLogPath:
		return cfg.Log.Path
	case FieldHealthReportWebhookURL:
		return cfg.HealthReport.WebhookURL
	case FieldHealthReportSchedule:
		return cfg.HealthReport.Schedule
	case FieldWatchtowerEnabled:
		return cfg.Watchtower.Enabled
	case FieldWatchtowerTickInterval:
		return cfg.Watchtower.TickInterval
	case FieldWatchtowerCaptureLines:
		return cfg.Watchtower.CaptureLines
	case FieldWatchtowerCaptureTimeout:
		return cfg.Watchtower.CaptureTimeout
	case FieldWatchtowerJournalRows:
		return cfg.Watchtower.JournalRows
	case FieldMCPEnabled:
		return cfg.MCP.Enabled
	case FieldRunbooksMaxConcurrent:
		return cfg.Runbooks.MaxConcurrent
	case FieldMultiUserAllowedUsers:
		return cfg.MultiUser.AllowedUsers
	case FieldMultiUserAllowRootTarget:
		return cfg.MultiUser.AllowRootTarget
	case FieldMultiUserUserSwitchMethod:
		return cfg.MultiUser.UserSwitchMethod
	default:
		return nil
	}
}
