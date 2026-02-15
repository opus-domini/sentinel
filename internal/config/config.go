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

type Config struct {
	ListenAddr     string
	Token          string
	AllowedOrigins []string
	DataDir        string
	LogLevel       string
	Watchtower     WatchtowerConfig
	Recovery       RecoveryConfig
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

# Bearer token for API and WebSocket authentication.
# When set, all requests must include this token.
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
# recovery_enabled = true
# recovery_snapshot_interval = "5s"
# recovery_max_snapshots = 300
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
	}

	// Resolve DataDir first (needed for config file path).
	if v := strings.TrimSpace(os.Getenv("SENTINEL_DATA_DIR")); v != "" {
		cfg.DataDir = v
	} else if home, err := resolveHomeDir(); err == nil {
		cfg.DataDir = filepath.Join(home, ".sentinel")
	} else {
		// Last-resort fallback for restricted service environments.
		cfg.DataDir = filepath.Join(osTempDir(), "sentinel")
	}

	// Create default config file if it does not exist.
	configPath := filepath.Join(cfg.DataDir, "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		writeDefaultConfig(configPath)
	}

	// Load config file (values act as defaults).
	file := loadFile(configPath)

	// Token: env > file
	if v := strings.TrimSpace(os.Getenv("SENTINEL_TOKEN")); v != "" {
		cfg.Token = v
	} else if v := file["token"]; v != "" {
		cfg.Token = v
	}

	// Listen: env > file > default
	if v := strings.TrimSpace(os.Getenv("SENTINEL_LISTEN")); v != "" {
		cfg.ListenAddr = v
	} else if v := file["listen"]; v != "" {
		cfg.ListenAddr = v
	}

	// Allowed origins: env > file
	if raw := strings.TrimSpace(os.Getenv("SENTINEL_ALLOWED_ORIGINS")); raw != "" {
		cfg.AllowedOrigins = splitCSV(raw)
	} else if v := file["allowed_origins"]; v != "" {
		cfg.AllowedOrigins = splitCSV(v)
	}

	// Log level: env > file > default (info)
	cfg.LogLevel = "info"
	if v := strings.TrimSpace(os.Getenv("SENTINEL_LOG_LEVEL")); v != "" {
		cfg.LogLevel = strings.ToLower(v)
	} else if v := file["log_level"]; v != "" {
		cfg.LogLevel = strings.ToLower(v)
	}

	// Watchtower enabled.
	if raw := strings.TrimSpace(os.Getenv("SENTINEL_WATCHTOWER_ENABLED")); raw != "" {
		if parsed, ok := parseBool(raw); ok {
			cfg.Watchtower.Enabled = parsed
		}
	} else if raw := file["watchtower_enabled"]; raw != "" {
		if parsed, ok := parseBool(raw); ok {
			cfg.Watchtower.Enabled = parsed
		}
	}

	// Watchtower tick interval.
	if raw := strings.TrimSpace(os.Getenv("SENTINEL_WATCHTOWER_TICK_INTERVAL")); raw != "" {
		if parsed, ok := parseDuration(raw); ok {
			cfg.Watchtower.TickInterval = parsed
		}
	} else if raw := file["watchtower_tick_interval"]; raw != "" {
		if parsed, ok := parseDuration(raw); ok {
			cfg.Watchtower.TickInterval = parsed
		}
	}

	// Watchtower capture lines.
	if raw := strings.TrimSpace(os.Getenv("SENTINEL_WATCHTOWER_CAPTURE_LINES")); raw != "" {
		if parsed, ok := parsePositiveInt(raw); ok {
			cfg.Watchtower.CaptureLines = parsed
		}
	} else if raw := file["watchtower_capture_lines"]; raw != "" {
		if parsed, ok := parsePositiveInt(raw); ok {
			cfg.Watchtower.CaptureLines = parsed
		}
	}

	// Watchtower capture timeout.
	if raw := strings.TrimSpace(os.Getenv("SENTINEL_WATCHTOWER_CAPTURE_TIMEOUT")); raw != "" {
		if parsed, ok := parseDuration(raw); ok {
			cfg.Watchtower.CaptureTimeout = parsed
		}
	} else if raw := file["watchtower_capture_timeout"]; raw != "" {
		if parsed, ok := parseDuration(raw); ok {
			cfg.Watchtower.CaptureTimeout = parsed
		}
	}

	// Watchtower journal max rows.
	if raw := strings.TrimSpace(os.Getenv("SENTINEL_WATCHTOWER_JOURNAL_ROWS")); raw != "" {
		if parsed, ok := parsePositiveInt(raw); ok {
			cfg.Watchtower.JournalRows = parsed
		}
	} else if raw := file["watchtower_journal_rows"]; raw != "" {
		if parsed, ok := parsePositiveInt(raw); ok {
			cfg.Watchtower.JournalRows = parsed
		}
	}

	// Recovery enabled.
	if raw := strings.TrimSpace(os.Getenv("SENTINEL_RECOVERY_ENABLED")); raw != "" {
		if parsed, ok := parseBool(raw); ok {
			cfg.Recovery.Enabled = parsed
		}
	} else if raw := file["recovery_enabled"]; raw != "" {
		if parsed, ok := parseBool(raw); ok {
			cfg.Recovery.Enabled = parsed
		}
	}

	// Recovery snapshot interval.
	if raw := strings.TrimSpace(os.Getenv("SENTINEL_RECOVERY_SNAPSHOT_INTERVAL")); raw != "" {
		if parsed, ok := parseDuration(raw); ok {
			cfg.Recovery.SnapshotInterval = parsed
		}
	} else if raw := file["recovery_snapshot_interval"]; raw != "" {
		if parsed, ok := parseDuration(raw); ok {
			cfg.Recovery.SnapshotInterval = parsed
		}
	}

	// Recovery capture lines.
	if raw := strings.TrimSpace(os.Getenv("SENTINEL_RECOVERY_CAPTURE_LINES")); raw != "" {
		if parsed, ok := parsePositiveInt(raw); ok {
			cfg.Recovery.CaptureLines = parsed
		}
	} else if raw := file["recovery_capture_lines"]; raw != "" {
		if parsed, ok := parsePositiveInt(raw); ok {
			cfg.Recovery.CaptureLines = parsed
		}
	}

	// Recovery max snapshots.
	if raw := strings.TrimSpace(os.Getenv("SENTINEL_RECOVERY_MAX_SNAPSHOTS")); raw != "" {
		if parsed, ok := parsePositiveInt(raw); ok {
			cfg.Recovery.MaxSnapshots = parsed
		}
	} else if raw := file["recovery_max_snapshots"]; raw != "" {
		if parsed, ok := parsePositiveInt(raw); ok {
			cfg.Recovery.MaxSnapshots = parsed
		}
	}

	return cfg
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
