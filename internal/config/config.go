package config

import (
	"bufio"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	CookieSecureAuto   = "auto"
	CookieSecureAlways = "always"
	CookieSecureNever  = "never"
)

type AlertThresholds struct {
	CPUPercent  float64
	MemPercent  float64
	DiskPercent float64
}

type Config struct {
	ListenAddr          string
	Token               string
	AllowedOrigins      []string
	CookieSecure        string
	AllowInsecureCookie bool
	DataDir             string
	LogLevel            string
	Timezone            string
	Locale              string
	Watchtower          WatchtowerConfig
	Recovery            RecoveryConfig
	AlertThresholds     AlertThresholds
}

type WatchtowerConfig struct {
	Enabled        bool
	TickInterval   time.Duration
	CaptureLines   int
	CaptureTimeout time.Duration
	JournalRows    int
}

type RecoveryConfig struct {
	Enabled          bool
	SnapshotInterval time.Duration
	CaptureLines     int
	MaxSnapshots     int
	BootRestore      string // "off", "safe", "confirm", "full"
}

var (
	osUserHomeDir = os.UserHomeDir
	osCurrentUser = user.Current
	osGeteuid     = os.Geteuid
	osTempDir     = os.TempDir
)

const defaultConfigContent = `# Sentinel configuration
# All values shown are defaults. Uncomment and edit to customize.

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

# Log level: debug, info, warn, error.
# Environment variable: SENTINEL_LOG_LEVEL
# log_level = "info"

# Watchtower subsystem (background activity projection + unread journal).
# Environment variables:
# - SENTINEL_WATCHTOWER_ENABLED
# - SENTINEL_WATCHTOWER_TICK_INTERVAL
# - SENTINEL_WATCHTOWER_CAPTURE_LINES
# - SENTINEL_WATCHTOWER_CAPTURE_TIMEOUT
# - SENTINEL_WATCHTOWER_JOURNAL_ROWS
# watchtower_enabled = true
# watchtower_tick_interval = "1s"
# watchtower_capture_lines = 80
# watchtower_capture_timeout = "150ms"
# watchtower_journal_rows = 5000

# Recovery subsystem (tmux session journal + restore engine).
# Environment variables:
# - SENTINEL_RECOVERY_ENABLED
# - SENTINEL_RECOVERY_SNAPSHOT_INTERVAL
# - SENTINEL_RECOVERY_MAX_SNAPSHOTS
# - SENTINEL_RECOVERY_BOOT_RESTORE
# recovery_enabled = true
# recovery_snapshot_interval = "5s"
# recovery_max_snapshots = 300
# recovery_boot_restore = "off"  # off | safe | confirm | full

# IANA timezone for all displayed timestamps.
# Environment variable: SENTINEL_TIMEZONE
# timezone = "America/Sao_Paulo"

# BCP 47 locale for date/number formatting (e.g. "pt-BR", "en-US").
# When empty, the browser's default locale is used.
# Environment variable: SENTINEL_LOCALE
# locale = "pt-BR"
`

func Load() Config {
	cfg := Config{
		ListenAddr: "127.0.0.1:4040",
		Watchtower: WatchtowerConfig{
			Enabled:        true,
			TickInterval:   1 * time.Second,
			CaptureLines:   80,
			CaptureTimeout: 150 * time.Millisecond,
			JournalRows:    5000,
		},
		Recovery: RecoveryConfig{
			Enabled:          true,
			SnapshotInterval: 5 * time.Second,
			CaptureLines:     80,
			MaxSnapshots:     300,
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
	applyRecoveryConfig(&cfg, file)
	applyAlertThresholdsConfig(&cfg, file)

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

	cfg.LogLevel = "info"
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

func applyRecoveryConfig(cfg *Config, file map[string]string) {
	if cfg == nil {
		return
	}

	cfg.Recovery.Enabled = readBoolEnvOrFile(
		"SENTINEL_RECOVERY_ENABLED",
		"recovery_enabled",
		file,
		cfg.Recovery.Enabled,
	)
	cfg.Recovery.SnapshotInterval = readDurationEnvOrFile(
		"SENTINEL_RECOVERY_SNAPSHOT_INTERVAL",
		"recovery_snapshot_interval",
		file,
		cfg.Recovery.SnapshotInterval,
	)
	cfg.Recovery.CaptureLines = readPositiveIntEnvOrFile(
		"SENTINEL_RECOVERY_CAPTURE_LINES",
		"recovery_capture_lines",
		file,
		cfg.Recovery.CaptureLines,
	)
	cfg.Recovery.MaxSnapshots = readPositiveIntEnvOrFile(
		"SENTINEL_RECOVERY_MAX_SNAPSHOTS",
		"recovery_max_snapshots",
		file,
		cfg.Recovery.MaxSnapshots,
	)
	if raw := readRawEnvOrFile("SENTINEL_RECOVERY_BOOT_RESTORE", "recovery_boot_restore", file); raw != "" {
		switch raw {
		case "off", "safe", "confirm", "full":
			cfg.Recovery.BootRestore = raw
		}
	}
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

// loadFile reads a simple key = value config file.
// Lines starting with # are comments. Quotes around values are stripped.
// Returns an empty map if the file does not exist.
func loadFile(path string) map[string]string {
	m := make(map[string]string)
	f, err := os.Open(path) //nolint:gosec // path is derived from DataDir, not user input
	if err != nil {
		return m
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == '[' {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k := strings.TrimSpace(key)
		v := strings.TrimSpace(val)
		// Strip surrounding quotes.
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		m[k] = v
	}
	return m
}

// writeDefaultConfig creates the config file with commented-out defaults.
// Best-effort: errors are silently ignored.
func writeDefaultConfig(path string) {
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, []byte(defaultConfigContent), 0o600) //nolint:gosec // fixed content, not user input
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
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
