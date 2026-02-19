package config

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	content := `# Sentinel config
listen = "0.0.0.0:4040"
token = 'my-secret'
allowed_origins = "http://localhost:3000, http://192.168.1.10:4040"

# Section headers are ignored
[section]
unknown_key = ignored
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	m := loadFile(path)

	tests := []struct {
		key, want string
	}{
		{"listen", "0.0.0.0:4040"},
		{"token", "my-secret"},
		{"allowed_origins", "http://localhost:3000, http://192.168.1.10:4040"},
		{"unknown_key", "ignored"},
	}
	for _, tt := range tests {
		if got := m[tt.key]; got != tt.want {
			t.Errorf("loadFile[%q] = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestLoadFileMissing(t *testing.T) {
	m := loadFile("/nonexistent/path/config.toml")
	if len(m) != 0 {
		t.Errorf("expected empty map for missing file, got %v", m)
	}
}

func TestLoadUsesConfigFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := `listen = "0.0.0.0:9090"
token = "file-token"
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// Clear env vars to ensure file values are used.
	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")

	cfg := Load()

	if cfg.ListenAddr != "0.0.0.0:9090" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, "0.0.0.0:9090")
	}
	if cfg.Token != "file-token" {
		t.Errorf("Token = %q, want %q", cfg.Token, "file-token")
	}
}

func TestLoadCreatesDefaultConfig(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_LOG_LEVEL", "")

	cfg := Load()

	// Default config file should be created.
	configPath := filepath.Join(dir, "config.toml")
	data, err := os.ReadFile(configPath) //nolint:gosec // test file, path is from t.TempDir()
	if err != nil {
		t.Fatalf("expected config file to be created: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "# listen") {
		t.Error("expected config file to contain '# listen'")
	}
	if !strings.Contains(content, "# token") {
		t.Error("expected config file to contain '# token'")
	}

	// All defaults should still apply (file is all comments).
	if cfg.ListenAddr != "127.0.0.1:4040" {
		t.Errorf("ListenAddr = %q, want default", cfg.ListenAddr)
	}
	if cfg.Token != "" {
		t.Errorf("Token = %q, want empty", cfg.Token)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
}

func TestLoadDoesNotOverwriteExistingConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	original := `listen = "0.0.0.0:8080"
`
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_LOG_LEVEL", "")

	cfg := Load()

	// Existing config should not be overwritten.
	data, err := os.ReadFile(configPath) //nolint:gosec // test file, path is from t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Errorf("config file was overwritten: got %q", string(data))
	}
	if cfg.ListenAddr != "0.0.0.0:8080" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, "0.0.0.0:8080")
	}
}

func TestLoadEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := `listen = "0.0.0.0:9090"
token = "file-token"
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_LISTEN", "127.0.0.1:5050")
	t.Setenv("SENTINEL_TOKEN", "env-token")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")

	cfg := Load()

	if cfg.ListenAddr != "127.0.0.1:5050" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, "127.0.0.1:5050")
	}
	if cfg.Token != "env-token" {
		t.Errorf("Token = %q, want %q", cfg.Token, "env-token")
	}
}

func TestLoadFallsBackToCurrentUserHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", "")
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("HOME", "")

	originalHomeFn := osUserHomeDir
	originalCurrentFn := osCurrentUser
	t.Cleanup(func() {
		osUserHomeDir = originalHomeFn
		osCurrentUser = originalCurrentFn
	})

	osUserHomeDir = func() (string, error) {
		return "", errors.New("home unavailable")
	}
	osCurrentUser = func() (*user.User, error) {
		return &user.User{HomeDir: dir}, nil
	}

	cfg := Load()
	want := filepath.Join(dir, ".sentinel")
	if cfg.DataDir != want {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, want)
	}
}

func TestLoadFallsBackToTempDirWhenHomeUnavailable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", "")
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("HOME", "")

	originalHomeFn := osUserHomeDir
	originalCurrentFn := osCurrentUser
	originalGeteuidFn := osGeteuid
	originalTempDirFn := osTempDir
	t.Cleanup(func() {
		osUserHomeDir = originalHomeFn
		osCurrentUser = originalCurrentFn
		osGeteuid = originalGeteuidFn
		osTempDir = originalTempDirFn
	})

	osUserHomeDir = func() (string, error) {
		return "", errors.New("home unavailable")
	}
	osCurrentUser = func() (*user.User, error) {
		return nil, errors.New("user unavailable")
	}
	osGeteuid = func() int {
		return 1000
	}
	osTempDir = func() string {
		return dir
	}

	cfg := Load()
	want := filepath.Join(dir, "sentinel")
	if cfg.DataDir != want {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, want)
	}
}

func TestSplitCSV(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty string", "", nil},
		{"single value", "foo", []string{"foo"}},
		{"multiple values", "a, b, c", []string{"a", "b", "c"}},
		{"whitespace", " a , b ", []string{"a", "b"}},
		{"empty segments", "a,,b,,", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := splitCSV(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("splitCSV(%q) = %v (len %d), want %v (len %d)", tt.input, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitCSV(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCookieSecureDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_COOKIE_SECURE", "")
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_LOG_LEVEL", "")

	cfg := Load()
	if cfg.CookieSecure != CookieSecureAuto {
		t.Fatalf("CookieSecure = %q, want auto", cfg.CookieSecure)
	}
}

func TestCookieSecureEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_COOKIE_SECURE", "always")
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_LOG_LEVEL", "")

	cfg := Load()
	if cfg.CookieSecure != "always" {
		t.Fatalf("CookieSecure = %q, want always", cfg.CookieSecure)
	}
}

func TestCookieSecureInvalidFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_COOKIE_SECURE", "bogus")
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_LOG_LEVEL", "")

	cfg := Load()
	if cfg.CookieSecure != CookieSecureAuto {
		t.Fatalf("CookieSecure = %q, want auto (fallback)", cfg.CookieSecure)
	}
}

func TestCookieSecureFromFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := `cookie_secure = "never"
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_COOKIE_SECURE", "")
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_LOG_LEVEL", "")

	cfg := Load()
	if cfg.CookieSecure != "never" {
		t.Fatalf("CookieSecure = %q, want never from file", cfg.CookieSecure)
	}
}

func TestAllowInsecureCookieDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_ALLOW_INSECURE_COOKIE", "")
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_LOG_LEVEL", "")
	t.Setenv("SENTINEL_COOKIE_SECURE", "")

	cfg := Load()
	if cfg.AllowInsecureCookie {
		t.Fatal("AllowInsecureCookie = true, want false by default")
	}
}

func TestAllowInsecureCookieEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_ALLOW_INSECURE_COOKIE", "true")
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_LOG_LEVEL", "")
	t.Setenv("SENTINEL_COOKIE_SECURE", "")

	cfg := Load()
	if !cfg.AllowInsecureCookie {
		t.Fatal("AllowInsecureCookie = false, want true from env")
	}
}

func TestLoadWatchtowerConfigFromEnvAndFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := `watchtower_enabled = false
watchtower_tick_interval = "2s"
watchtower_capture_lines = 120
watchtower_capture_timeout = "250ms"
watchtower_journal_rows = 7000
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_WATCHTOWER_ENABLED", "")
	t.Setenv("SENTINEL_WATCHTOWER_TICK_INTERVAL", "")
	t.Setenv("SENTINEL_WATCHTOWER_CAPTURE_LINES", "")
	t.Setenv("SENTINEL_WATCHTOWER_CAPTURE_TIMEOUT", "")
	t.Setenv("SENTINEL_WATCHTOWER_JOURNAL_ROWS", "")

	cfg := Load()
	if cfg.Watchtower.Enabled {
		t.Fatalf("watchtower enabled = true, want false from config file")
	}
	if cfg.Watchtower.TickInterval != 2*time.Second {
		t.Fatalf("watchtower tick interval = %s, want 2s", cfg.Watchtower.TickInterval)
	}
	if cfg.Watchtower.CaptureLines != 120 {
		t.Fatalf("watchtower capture lines = %d, want 120", cfg.Watchtower.CaptureLines)
	}
	if cfg.Watchtower.CaptureTimeout != 250*time.Millisecond {
		t.Fatalf("watchtower capture timeout = %s, want 250ms", cfg.Watchtower.CaptureTimeout)
	}
	if cfg.Watchtower.JournalRows != 7000 {
		t.Fatalf("watchtower journal rows = %d, want 7000", cfg.Watchtower.JournalRows)
	}

	t.Setenv("SENTINEL_WATCHTOWER_ENABLED", "true")
	t.Setenv("SENTINEL_WATCHTOWER_TICK_INTERVAL", "3s")
	t.Setenv("SENTINEL_WATCHTOWER_CAPTURE_LINES", "160")
	t.Setenv("SENTINEL_WATCHTOWER_CAPTURE_TIMEOUT", "300ms")
	t.Setenv("SENTINEL_WATCHTOWER_JOURNAL_ROWS", "9000")

	cfg = Load()
	if !cfg.Watchtower.Enabled {
		t.Fatalf("watchtower enabled = false, want true from env")
	}
	if cfg.Watchtower.TickInterval != 3*time.Second {
		t.Fatalf("watchtower tick interval = %s, want 3s", cfg.Watchtower.TickInterval)
	}
	if cfg.Watchtower.CaptureLines != 160 {
		t.Fatalf("watchtower capture lines = %d, want 160", cfg.Watchtower.CaptureLines)
	}
	if cfg.Watchtower.CaptureTimeout != 300*time.Millisecond {
		t.Fatalf("watchtower capture timeout = %s, want 300ms", cfg.Watchtower.CaptureTimeout)
	}
	if cfg.Watchtower.JournalRows != 9000 {
		t.Fatalf("watchtower journal rows = %d, want 9000", cfg.Watchtower.JournalRows)
	}
}
