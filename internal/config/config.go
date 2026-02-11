package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	ListenAddr     string
	Token          string
	AllowedOrigins []string
	DataDir        string
	LogLevel       string
}

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
`

func Load() Config {
	cfg := Config{
		ListenAddr: "127.0.0.1:4040",
	}

	// Resolve DataDir first (needed for config file path).
	if v := strings.TrimSpace(os.Getenv("SENTINEL_DATA_DIR")); v != "" {
		cfg.DataDir = v
	} else if home, err := os.UserHomeDir(); err == nil {
		cfg.DataDir = filepath.Join(home, ".sentinel")
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
