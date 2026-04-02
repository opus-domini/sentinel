package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	CookieSecureAuto   = "auto"
	CookieSecureAlways = "always"
	CookieSecureNever  = "never"

	DefaultLogLevel = "info"
)

type AlertThresholds struct {
	CPUPercent  float64
	MemPercent  float64
	DiskPercent float64
}

type MultiUserConfig struct {
	AllowedUsers     []string
	AllowRootTarget  bool
	UserSwitchMethod string // "sudo" (default) or "direct"
}

type Config struct {
	ListenAddr             string
	Token                  string
	AllowedOrigins         []string
	CookieSecure           string
	AllowInsecureCookie    bool
	DataDir                string
	LogLevel               string
	Timezone               string
	Locale                 string
	RunbookMaxConcurrent   int
	MultiUser              MultiUserConfig
	SystemUsers            []string // populated at startup from OS, not from config file
	Watchtower             WatchtowerConfig
	AlertThresholds        AlertThresholds
	AlertWebhookURL        string
	AlertWebhookEvents     []string
	HealthReportWebhookURL string
	HealthReportSchedule   string
}

type WatchtowerConfig struct {
	Enabled        bool
	TickInterval   time.Duration
	CaptureLines   int
	CaptureTimeout time.Duration
	JournalRows    int
}

var (
	osUserHomeDir = os.UserHomeDir
	osCurrentUser = user.Current
	osGeteuid     = os.Geteuid
	osTempDir     = os.TempDir
)

const defaultConfigContent = `# Sentinel configuration
# All values shown are defaults. Uncomment and edit to customize.
# Environment variables (SENTINEL_*) always take precedence over file values.

[server]
# Address and port the server listens on.
# Environment variable: SENTINEL_LISTEN
# listen = "127.0.0.1:4040"

# Authentication token used by the lock dialog to issue an HttpOnly cookie.
# When set, all API and WebSocket requests require a valid auth cookie.
# Environment variable: SENTINEL_TOKEN
# token = ""

# Comma-separated list of allowed CORS origins.
# Environment variable: SENTINEL_ALLOWED_ORIGINS
# allowed_origins = ""

# Cookie Secure flag: "auto", "always", or "never".
# Environment variable: SENTINEL_COOKIE_SECURE
# cookie_secure = "auto"

# Allow insecure (non-HTTPS) cookies.
# Environment variable: SENTINEL_ALLOW_INSECURE_COOKIE
# allow_insecure_cookie = false

# Log level: debug, info, warn, error.
# Environment variable: SENTINEL_LOG_LEVEL
# log_level = "info"

# IANA timezone for all displayed timestamps.
# Environment variable: SENTINEL_TIMEZONE
# timezone = "America/Sao_Paulo"

# BCP 47 locale for date/number formatting (e.g. "pt-BR", "en-US").
# When empty, the browser's default locale is used.
# Environment variable: SENTINEL_LOCALE
# locale = "pt-BR"

[alerts]
# CPU usage threshold percentage for alerts.
# Environment variable: SENTINEL_ALERT_CPU_PERCENT
# cpu_percent = 90.0

# Memory usage threshold percentage for alerts.
# Environment variable: SENTINEL_ALERT_MEM_PERCENT
# mem_percent = 90.0

# Disk usage threshold percentage for alerts.
# Environment variable: SENTINEL_ALERT_DISK_PERCENT
# disk_percent = 95.0

# Webhook URL for alert notifications. When set, HTTP POST requests
# are sent for alert lifecycle events.
# Environment variable: SENTINEL_ALERT_WEBHOOK_URL
# webhook_url = ""

# Comma-separated list of events that trigger webhooks.
# Supported: "alert.created", "alert.resolved", "alert.acked"
# Default (when URL is set): "alert.created,alert.resolved"
# Environment variable: SENTINEL_ALERT_WEBHOOK_EVENTS
# webhook_events = "alert.created,alert.resolved"

[health_report]
# Webhook URL for scheduled health report delivery.
# Environment variable: SENTINEL_HEALTH_REPORT_WEBHOOK_URL
# webhook_url = ""

# Cron schedule for health report generation (5-field cron or @descriptor).
# Example: "0 8 * * *" for daily at 8am.
# Environment variable: SENTINEL_HEALTH_REPORT_SCHEDULE
# schedule = ""

[watchtower]
# Enable the watchtower subsystem (background activity projection + unread journal).
# Environment variable: SENTINEL_WATCHTOWER_ENABLED
# enabled = true

# How often watchtower polls for activity.
# Environment variable: SENTINEL_WATCHTOWER_TICK_INTERVAL
# tick_interval = "1s"

# Number of lines to capture from each pane.
# Environment variable: SENTINEL_WATCHTOWER_CAPTURE_LINES
# capture_lines = 80

# Timeout for capturing pane content.
# Environment variable: SENTINEL_WATCHTOWER_CAPTURE_TIMEOUT
# capture_timeout = "150ms"

# Maximum number of rows in the unread journal.
# Environment variable: SENTINEL_WATCHTOWER_JOURNAL_ROWS
# journal_rows = 5000

[runbooks]
# Maximum number of concurrent manual runbook executions.
# Environment variable: SENTINEL_RUNBOOK_MAX_CONCURRENT
# max_concurrent = 5

[multi_user]
# Multi-user tmux session support is always active. Sessions can be
# created as a different OS user via sudo.

# List of OS users allowed as session targets. When empty, any user is allowed.
# Environment variable: SENTINEL_ALLOWED_USERS (comma-separated)
# allowed_users = []

# Allow targeting the root user for sessions.
# Environment variable: SENTINEL_ALLOW_ROOT_TARGET
# allow_root_target = false

# Method for switching users: "sudo" (default) or "direct".
# Environment variable: SENTINEL_USER_SWITCH_METHOD
# user_switch_method = "sudo"
`

func Load() Config {
	cfg := Config{
		ListenAddr:           "127.0.0.1:4040",
		RunbookMaxConcurrent: 5,
		Watchtower: WatchtowerConfig{
			Enabled:        true,
			TickInterval:   1 * time.Second,
			CaptureLines:   80,
			CaptureTimeout: 150 * time.Millisecond,
			JournalRows:    5000,
		},
		AlertThresholds: AlertThresholds{
			CPUPercent:  90.0,
			MemPercent:  90.0,
			DiskPercent: 95.0,
		},
	}

	cfg.Timezone = time.Now().Location().String()
	cfg.DataDir = resolveDataDir()
	configPath := filepath.Join(cfg.DataDir, "config.toml")
	ensureDefaultConfig(configPath)

	file := loadFile(configPath)
	applyCoreConfig(&cfg, file)
	applyWatchtowerConfig(&cfg, file)
	applyAlertThresholdsConfig(&cfg, file)
	applyMultiUserConfig(&cfg, file)

	return cfg
}

func resolveDataDir() string {
	if v := strings.TrimSpace(os.Getenv("SENTINEL_DATA_DIR")); v != "" {
		return v
	}
	if home, err := resolveHomeDir(); err == nil {
		return filepath.Join(home, ".sentinel")
	}
	// Last-resort fallback for restricted service environments.
	return filepath.Join(osTempDir(), "sentinel")
}

func ensureDefaultConfig(configPath string) {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		writeDefaultConfig(configPath)
	}
}

func applyCoreConfig(cfg *Config, file map[string]string) {
	if cfg == nil {
		return
	}

	cfg.Token = readRawEnvOrFile("SENTINEL_TOKEN", "token", file)
	if listen := readRawEnvOrFile("SENTINEL_LISTEN", "listen", file); listen != "" {
		cfg.ListenAddr = listen
	}
	if origins := readRawEnvOrFile("SENTINEL_ALLOWED_ORIGINS", "allowed_origins", file); origins != "" {
		cfg.AllowedOrigins = splitCSV(origins)
	}
	cfg.CookieSecure = CookieSecureAuto
	if cs := readRawEnvOrFile("SENTINEL_COOKIE_SECURE", "cookie_secure", file); cs != "" {
		switch strings.ToLower(cs) {
		case CookieSecureAuto, CookieSecureAlways, CookieSecureNever:
			cfg.CookieSecure = strings.ToLower(cs)
		default:
			cfg.CookieSecure = CookieSecureAuto
		}
	}

	cfg.AllowInsecureCookie = readBoolEnvOrFile(
		"SENTINEL_ALLOW_INSECURE_COOKIE",
		"allow_insecure_cookie",
		file,
		false,
	)

	cfg.LogLevel = DefaultLogLevel
	if level := readRawEnvOrFile("SENTINEL_LOG_LEVEL", "log_level", file); level != "" {
		cfg.LogLevel = strings.ToLower(level)
	}

	if tz := readRawEnvOrFile("SENTINEL_TIMEZONE", "timezone", file); tz != "" {
		if _, err := time.LoadLocation(tz); err == nil {
			cfg.Timezone = tz
		}
	}

	if locale := readRawEnvOrFile("SENTINEL_LOCALE", "locale", file); locale != "" {
		cfg.Locale = locale
	}

	cfg.RunbookMaxConcurrent = readPositiveIntEnvOrFile(
		"SENTINEL_RUNBOOK_MAX_CONCURRENT",
		"runbook_max_concurrent",
		file,
		cfg.RunbookMaxConcurrent,
	)

	cfg.AlertWebhookURL = readRawEnvOrFile("SENTINEL_ALERT_WEBHOOK_URL", "alert_webhook_url", file)
	if evts := readRawEnvOrFile("SENTINEL_ALERT_WEBHOOK_EVENTS", "alert_webhook_events", file); evts != "" {
		cfg.AlertWebhookEvents = splitCSV(evts)
	}

	cfg.HealthReportWebhookURL = readRawEnvOrFile("SENTINEL_HEALTH_REPORT_WEBHOOK_URL", "health_report_webhook_url", file)
	cfg.HealthReportSchedule = readRawEnvOrFile("SENTINEL_HEALTH_REPORT_SCHEDULE", "health_report_schedule", file)
}

func applyWatchtowerConfig(cfg *Config, file map[string]string) {
	if cfg == nil {
		return
	}

	cfg.Watchtower.Enabled = readBoolEnvOrFile(
		"SENTINEL_WATCHTOWER_ENABLED",
		"watchtower_enabled",
		file,
		cfg.Watchtower.Enabled,
	)
	cfg.Watchtower.TickInterval = readDurationEnvOrFile(
		"SENTINEL_WATCHTOWER_TICK_INTERVAL",
		"watchtower_tick_interval",
		file,
		cfg.Watchtower.TickInterval,
	)
	cfg.Watchtower.CaptureLines = readPositiveIntEnvOrFile(
		"SENTINEL_WATCHTOWER_CAPTURE_LINES",
		"watchtower_capture_lines",
		file,
		cfg.Watchtower.CaptureLines,
	)
	cfg.Watchtower.CaptureTimeout = readDurationEnvOrFile(
		"SENTINEL_WATCHTOWER_CAPTURE_TIMEOUT",
		"watchtower_capture_timeout",
		file,
		cfg.Watchtower.CaptureTimeout,
	)
	cfg.Watchtower.JournalRows = readPositiveIntEnvOrFile(
		"SENTINEL_WATCHTOWER_JOURNAL_ROWS",
		"watchtower_journal_rows",
		file,
		cfg.Watchtower.JournalRows,
	)
}

func applyAlertThresholdsConfig(cfg *Config, file map[string]string) {
	if cfg == nil {
		return
	}
	cfg.AlertThresholds.CPUPercent = readPositiveFloatEnvOrFile(
		"SENTINEL_ALERT_CPU_PERCENT",
		"alert_cpu_percent",
		file,
		cfg.AlertThresholds.CPUPercent,
	)
	cfg.AlertThresholds.MemPercent = readPositiveFloatEnvOrFile(
		"SENTINEL_ALERT_MEM_PERCENT",
		"alert_mem_percent",
		file,
		cfg.AlertThresholds.MemPercent,
	)
	cfg.AlertThresholds.DiskPercent = readPositiveFloatEnvOrFile(
		"SENTINEL_ALERT_DISK_PERCENT",
		"alert_disk_percent",
		file,
		cfg.AlertThresholds.DiskPercent,
	)
}

func applyMultiUserConfig(cfg *Config, file map[string]string) {
	if cfg == nil {
		return
	}

	if users := readRawEnvOrFile("SENTINEL_ALLOWED_USERS", "allowed_users", file); users != "" {
		cfg.MultiUser.AllowedUsers = splitCSV(users)
	}

	cfg.MultiUser.AllowRootTarget = readBoolEnvOrFile(
		"SENTINEL_ALLOW_ROOT_TARGET",
		"allow_root_target",
		file,
		false,
	)

	cfg.MultiUser.UserSwitchMethod = "sudo"
	if method := readRawEnvOrFile("SENTINEL_USER_SWITCH_METHOD", "user_switch_method", file); method != "" {
		switch strings.ToLower(method) {
		case "sudo", "direct":
			cfg.MultiUser.UserSwitchMethod = strings.ToLower(method)
		}
	}
}

// ValidateMultiUser checks multi-user config consistency and logs warnings
// for recoverable issues. It modifies the config in place when corrections
// are needed (e.g. removing root from allowlist).
func ValidateMultiUser(cfg *Config) {
	if cfg == nil {
		return
	}

	// Remove root from allowlist if allow_root_target is false.
	if !cfg.MultiUser.AllowRootTarget {
		filtered := cfg.MultiUser.AllowedUsers[:0]
		for _, u := range cfg.MultiUser.AllowedUsers {
			if u == "root" {
				slog.Warn("removing root from allowed_users because allow_root_target is false")
				continue
			}
			filtered = append(filtered, u)
		}
		cfg.MultiUser.AllowedUsers = filtered
	}

	// Cross-reference allowed_users against system users.
	if len(cfg.SystemUsers) > 0 {
		systemSet := make(map[string]struct{}, len(cfg.SystemUsers))
		for _, u := range cfg.SystemUsers {
			systemSet[u] = struct{}{}
		}
		for _, u := range cfg.MultiUser.AllowedUsers {
			if _, ok := systemSet[u]; !ok {
				slog.Warn("allowed_users entry not found in system users", "user", u)
			}
		}
	}
}

func readRawEnvOrFile(envKey, fileKey string, file map[string]string) string {
	if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
		return v
	}
	if file == nil {
		return ""
	}
	return strings.TrimSpace(file[fileKey])
}

func readBoolEnvOrFile(envKey, fileKey string, file map[string]string, fallback bool) bool {
	raw := readRawEnvOrFile(envKey, fileKey, file)
	if raw == "" {
		return fallback
	}
	if parsed, ok := parseBool(raw); ok {
		return parsed
	}
	return fallback
}

func readDurationEnvOrFile(envKey, fileKey string, file map[string]string, fallback time.Duration) time.Duration {
	raw := readRawEnvOrFile(envKey, fileKey, file)
	if raw == "" {
		return fallback
	}
	if parsed, ok := parseDuration(raw); ok {
		return parsed
	}
	return fallback
}

func readPositiveIntEnvOrFile(envKey, fileKey string, file map[string]string, fallback int) int {
	raw := readRawEnvOrFile(envKey, fileKey, file)
	if raw == "" {
		return fallback
	}
	if parsed, ok := parsePositiveInt(raw); ok {
		return parsed
	}
	return fallback
}

func readPositiveFloatEnvOrFile(envKey, fileKey string, file map[string]string, fallback float64) float64 {
	raw := readRawEnvOrFile(envKey, fileKey, file)
	if raw == "" {
		return fallback
	}
	if parsed, ok := parsePositiveFloat(raw); ok {
		return parsed
	}
	return fallback
}

func parsePositiveFloat(raw string) (float64, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}

// tomlServer maps the [server] section of the config file.
type tomlServer struct {
	Listen              *string `toml:"listen"`
	Token               *string `toml:"token"`
	AllowedOrigins      *string `toml:"allowed_origins"`
	CookieSecure        *string `toml:"cookie_secure"`
	AllowInsecureCookie *bool   `toml:"allow_insecure_cookie"`
	LogLevel            *string `toml:"log_level"`
	Timezone            *string `toml:"timezone"`
	Locale              *string `toml:"locale"`
}

// tomlAlerts maps the [alerts] section of the config file.
type tomlAlerts struct {
	CPUPercent  *float64 `toml:"cpu_percent"`
	MemPercent  *float64 `toml:"mem_percent"`
	DiskPercent *float64 `toml:"disk_percent"`
	WebhookURL  *string  `toml:"webhook_url"`
	WebhookEvts *string  `toml:"webhook_events"`
}

// tomlWatchtower maps the [watchtower] section of the config file.
type tomlWatchtower struct {
	Enabled        *bool   `toml:"enabled"`
	TickInterval   *string `toml:"tick_interval"`
	CaptureLines   *int64  `toml:"capture_lines"`
	CaptureTimeout *string `toml:"capture_timeout"`
	JournalRows    *int64  `toml:"journal_rows"`
}

// tomlHealthReport maps the [health_report] section of the config file.
type tomlHealthReport struct {
	WebhookURL *string `toml:"webhook_url"`
	Schedule   *string `toml:"schedule"`
}

// tomlRunbooks maps the [runbooks] section of the config file.
type tomlRunbooks struct {
	MaxConcurrent *int64 `toml:"max_concurrent"`
}

// tomlMultiUser maps the [multi_user] section of the config file.
type tomlMultiUser struct {
	AllowedUsers     []string `toml:"allowed_users"`
	AllowRootTarget  *bool    `toml:"allow_root_target"`
	UserSwitchMethod *string  `toml:"user_switch_method"`
}

// tomlConfig is the top-level structure for the TOML config file.
// It supports both sectioned format (preferred) and flat legacy keys.
type tomlConfig struct {
	Server       tomlServer       `toml:"server"`
	Alerts       tomlAlerts       `toml:"alerts"`
	HealthReport tomlHealthReport `toml:"health_report"`
	Watchtower   tomlWatchtower   `toml:"watchtower"`
	Runbooks     tomlRunbooks     `toml:"runbooks"`
	MultiUser    tomlMultiUser    `toml:"multi_user"`

	// Legacy flat keys (for backward compatibility with pre-section configs).
	Listen                   *string  `toml:"listen"`
	Token                    *string  `toml:"token"`
	AllowedOrigins           *string  `toml:"allowed_origins"`
	CookieSecure             *string  `toml:"cookie_secure"`
	AllowInsecureCookie      *bool    `toml:"allow_insecure_cookie"`
	LogLevel                 *string  `toml:"log_level"`
	Timezone                 *string  `toml:"timezone"`
	Locale                   *string  `toml:"locale"`
	RunbookMaxConcurrent     *int64   `toml:"runbook_max_concurrent"`
	WatchtowerEnabled        *bool    `toml:"watchtower_enabled"`
	WatchtowerTickInterval   *string  `toml:"watchtower_tick_interval"`
	WatchtowerCaptureLines   *int64   `toml:"watchtower_capture_lines"`
	WatchtowerCaptureTimeout *string  `toml:"watchtower_capture_timeout"`
	WatchtowerJournalRows    *int64   `toml:"watchtower_journal_rows"`
	AlertCPUPercent          *float64 `toml:"alert_cpu_percent"`
	AlertMemPercent          *float64 `toml:"alert_mem_percent"`
	AlertDiskPercent         *float64 `toml:"alert_disk_percent"`
	AlertWebhookURL          *string  `toml:"alert_webhook_url"`
	AlertWebhookEvents       *string  `toml:"alert_webhook_events"`
	HealthReportWebhookURL   *string  `toml:"health_report_webhook_url"`
	HealthReportSchedule     *string  `toml:"health_report_schedule"`
}

// flatten converts the parsed TOML config into a flat key-value map.
// Sectioned keys take precedence over legacy flat keys.
func (tc *tomlConfig) flatten() map[string]string {
	m := make(map[string]string)

	// Helper to set a string value if the pointer is non-nil.
	setStr := func(key string, v *string) {
		if v != nil {
			m[key] = *v
		}
	}
	setBool := func(key string, v *bool) {
		if v != nil {
			m[key] = strconv.FormatBool(*v)
		}
	}
	setInt := func(key string, v *int64) {
		if v != nil {
			m[key] = strconv.FormatInt(*v, 10)
		}
	}
	setFloat := func(key string, v *float64) {
		if v != nil {
			m[key] = strconv.FormatFloat(*v, 'f', -1, 64)
		}
	}

	// Apply legacy flat keys first.
	setStr("listen", tc.Listen)
	setStr("token", tc.Token)
	setStr("allowed_origins", tc.AllowedOrigins)
	setStr("cookie_secure", tc.CookieSecure)
	setBool("allow_insecure_cookie", tc.AllowInsecureCookie)
	setStr("log_level", tc.LogLevel)
	setStr("timezone", tc.Timezone)
	setStr("locale", tc.Locale)
	setInt("runbook_max_concurrent", tc.RunbookMaxConcurrent)
	setBool("watchtower_enabled", tc.WatchtowerEnabled)
	setStr("watchtower_tick_interval", tc.WatchtowerTickInterval)
	setInt("watchtower_capture_lines", tc.WatchtowerCaptureLines)
	setStr("watchtower_capture_timeout", tc.WatchtowerCaptureTimeout)
	setInt("watchtower_journal_rows", tc.WatchtowerJournalRows)
	setFloat("alert_cpu_percent", tc.AlertCPUPercent)
	setFloat("alert_mem_percent", tc.AlertMemPercent)
	setFloat("alert_disk_percent", tc.AlertDiskPercent)
	setStr("alert_webhook_url", tc.AlertWebhookURL)
	setStr("alert_webhook_events", tc.AlertWebhookEvents)
	setStr("health_report_webhook_url", tc.HealthReportWebhookURL)
	setStr("health_report_schedule", tc.HealthReportSchedule)

	// Sectioned keys override legacy flat keys.
	setStr("listen", tc.Server.Listen)
	setStr("token", tc.Server.Token)
	setStr("allowed_origins", tc.Server.AllowedOrigins)
	setStr("cookie_secure", tc.Server.CookieSecure)
	setBool("allow_insecure_cookie", tc.Server.AllowInsecureCookie)
	setStr("log_level", tc.Server.LogLevel)
	setStr("timezone", tc.Server.Timezone)
	setStr("locale", tc.Server.Locale)

	setFloat("alert_cpu_percent", tc.Alerts.CPUPercent)
	setFloat("alert_mem_percent", tc.Alerts.MemPercent)
	setFloat("alert_disk_percent", tc.Alerts.DiskPercent)
	setStr("alert_webhook_url", tc.Alerts.WebhookURL)
	setStr("alert_webhook_events", tc.Alerts.WebhookEvts)

	setStr("health_report_webhook_url", tc.HealthReport.WebhookURL)
	setStr("health_report_schedule", tc.HealthReport.Schedule)

	setBool("watchtower_enabled", tc.Watchtower.Enabled)
	setStr("watchtower_tick_interval", tc.Watchtower.TickInterval)
	setInt("watchtower_capture_lines", tc.Watchtower.CaptureLines)
	setStr("watchtower_capture_timeout", tc.Watchtower.CaptureTimeout)
	setInt("watchtower_journal_rows", tc.Watchtower.JournalRows)

	setInt("runbook_max_concurrent", tc.Runbooks.MaxConcurrent)

	if len(tc.MultiUser.AllowedUsers) > 0 {
		joined := strings.Join(tc.MultiUser.AllowedUsers, ",")
		m["allowed_users"] = joined
	}
	setBool("allow_root_target", tc.MultiUser.AllowRootTarget)
	setStr("user_switch_method", tc.MultiUser.UserSwitchMethod)

	return m
}

// loadFile parses a TOML config file and returns a flat key-value map.
// Returns an empty map if the file does not exist or cannot be parsed.
func loadFile(path string) map[string]string {
	var tc tomlConfig
	if _, err := toml.DecodeFile(path, &tc); err != nil {
		return make(map[string]string)
	}
	return tc.flatten()
}

// decodeTOML parses TOML content from a string and returns a flat key-value map.
// Used for testing; returns an error if parsing fails.
func decodeTOML(content string) (map[string]string, error) {
	var tc tomlConfig
	if _, err := toml.Decode(content, &tc); err != nil {
		return nil, fmt.Errorf("decode toml: %w", err)
	}
	return tc.flatten(), nil
}

// writeDefaultConfig creates the config file with commented-out defaults.
// Best-effort: errors are silently ignored.
func writeDefaultConfig(path string) {
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, []byte(defaultConfigContent), 0o600) //nolint:gosec // fixed content, not user input
}

func splitCSV(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func parseBool(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func parseDuration(raw string) (time.Duration, bool) {
	v, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

func parsePositiveInt(raw string) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}

func resolveHomeDir() (string, error) {
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return home, nil
	}
	if home, err := osUserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return strings.TrimSpace(home), nil
	}
	if current, err := osCurrentUser(); err == nil && current != nil {
		if home := strings.TrimSpace(current.HomeDir); home != "" {
			return home, nil
		}
	}
	if osGeteuid() == 0 {
		// System services may run without HOME set.
		if runtime.GOOS == "darwin" {
			return "/var/root", nil
		}
		return "/root", nil
	}
	return "", errors.New("home directory not found")
}
