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

func TestLoadFileWithSectionedTOML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	content := `[server]
listen = "0.0.0.0:4040"
token = "my-secret"
allowed_origins = "http://localhost:3000, http://192.168.1.10:4040"
log_level = "debug"

[alerts]
cpu_percent = 85.0

[watchtower]
enabled = false
tick_interval = "2s"
capture_lines = 100

[runbooks]
max_concurrent = 10
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
		{"log_level", "debug"},
		{"alert_cpu_percent", "85"},
		{"watchtower_enabled", "false"},
		{"watchtower_tick_interval", "2s"},
		{"watchtower_capture_lines", "100"},
		{"runbook_max_concurrent", "10"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()
			if got := m[tt.key]; got != tt.want {
				t.Errorf("loadFile[%q] = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestLoadFileWithFlatLegacyTOML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	content := `listen = "0.0.0.0:4040"
token = 'my-secret'
allowed_origins = "http://localhost:3000, http://192.168.1.10:4040"
watchtower_enabled = false
watchtower_tick_interval = "2s"
watchtower_capture_lines = 100
runbook_max_concurrent = 10
alert_cpu_percent = 85.0
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
		{"watchtower_enabled", "false"},
		{"watchtower_tick_interval", "2s"},
		{"watchtower_capture_lines", "100"},
		{"runbook_max_concurrent", "10"},
		{"alert_cpu_percent", "85"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()
			if got := m[tt.key]; got != tt.want {
				t.Errorf("loadFile[%q] = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestLoadFileSectionedOverridesFlat(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Flat key first, then sectioned key — sectioned should win.
	content := `listen = "flat-addr"

[server]
listen = "sectioned-addr"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	m := loadFile(path)
	if got := m["listen"]; got != "sectioned-addr" {
		t.Errorf("loadFile[listen] = %q, want %q (sectioned should override flat)", got, "sectioned-addr")
	}
}

func TestLoadFileMissing(t *testing.T) {
	t.Parallel()

	m := loadFile("/nonexistent/path/config.toml")
	if len(m) != 0 {
		t.Errorf("expected empty map for missing file, got %v", m)
	}
}

func TestLoadFileInvalidTOML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	content := `this is not valid toml =[[[`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	m := loadFile(path)
	if len(m) != 0 {
		t.Errorf("expected empty map for invalid TOML, got %v", m)
	}
}

func TestDecodeTOML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantKey string
		wantVal string
		wantErr bool
	}{
		{
			name:    "server listen",
			input:   "[server]\nlisten = \"0.0.0.0:9090\"",
			wantKey: "listen",
			wantVal: "0.0.0.0:9090",
		},
		{
			name:    "flat legacy token",
			input:   `token = "abc123"`,
			wantKey: "token",
			wantVal: "abc123",
		},
		{
			name:    "invalid toml",
			input:   `[[[broken`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, err := decodeTOML(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := m[tt.wantKey]; got != tt.wantVal {
				t.Errorf("decodeTOML[%q] = %q, want %q", tt.wantKey, got, tt.wantVal)
			}
		})
	}
}

func TestLoadUsesConfigFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := `[server]
listen = "0.0.0.0:9090"
token = "file-token"
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

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

func TestLoadUsesLegacyFlatConfigFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := `listen = "0.0.0.0:9090"
token = "file-token"
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

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

	configPath := filepath.Join(dir, "config.toml")
	data, err := os.ReadFile(configPath) //nolint:gosec // test file, path is from t.TempDir()
	if err != nil {
		t.Fatalf("expected config file to be created: %v", err)
	}
	content := string(data)

	// Check that TOML sections are present.
	for _, section := range []string{"[server]", "[alerts]", "[watchtower]", "[runbooks]"} {
		if !strings.Contains(content, section) {
			t.Errorf("expected config file to contain %q", section)
		}
	}
	// Check that commented-out keys are present.
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
	original := `[server]
listen = "0.0.0.0:8080"
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
	content := `[server]
listen = "0.0.0.0:9090"
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
	content := `[server]
cookie_secure = "never"
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

func TestLoadWatchtowerConfigFromSectionedFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := `[watchtower]
enabled = false
tick_interval = "2s"
capture_lines = 120
capture_timeout = "250ms"
journal_rows = 7000
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
}

func TestLoadWatchtowerConfigFromLegacyFlatFile(t *testing.T) {
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
}

func TestLoadWatchtowerEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := `[watchtower]
enabled = false
tick_interval = "2s"
capture_lines = 120
capture_timeout = "250ms"
journal_rows = 7000
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_WATCHTOWER_ENABLED", "true")
	t.Setenv("SENTINEL_WATCHTOWER_TICK_INTERVAL", "3s")
	t.Setenv("SENTINEL_WATCHTOWER_CAPTURE_LINES", "160")
	t.Setenv("SENTINEL_WATCHTOWER_CAPTURE_TIMEOUT", "300ms")
	t.Setenv("SENTINEL_WATCHTOWER_JOURNAL_ROWS", "9000")

	cfg := Load()
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

func TestLoadAlertThresholdsFromSectionedFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := `[alerts]
cpu_percent = 75.0
mem_percent = 80.0
disk_percent = 85.0
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_ALERT_CPU_PERCENT", "")
	t.Setenv("SENTINEL_ALERT_MEM_PERCENT", "")
	t.Setenv("SENTINEL_ALERT_DISK_PERCENT", "")

	cfg := Load()
	if cfg.AlertThresholds.CPUPercent != 75.0 {
		t.Fatalf("CPUPercent = %f, want 75.0", cfg.AlertThresholds.CPUPercent)
	}
	if cfg.AlertThresholds.MemPercent != 80.0 {
		t.Fatalf("MemPercent = %f, want 80.0", cfg.AlertThresholds.MemPercent)
	}
	if cfg.AlertThresholds.DiskPercent != 85.0 {
		t.Fatalf("DiskPercent = %f, want 85.0", cfg.AlertThresholds.DiskPercent)
	}
}

func TestRunbookMaxConcurrentDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_RUNBOOK_MAX_CONCURRENT", "")

	cfg := Load()
	if cfg.RunbookMaxConcurrent != 5 {
		t.Fatalf("RunbookMaxConcurrent = %d, want 5", cfg.RunbookMaxConcurrent)
	}
}

func TestRunbookMaxConcurrentEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_RUNBOOK_MAX_CONCURRENT", "10")

	cfg := Load()
	if cfg.RunbookMaxConcurrent != 10 {
		t.Fatalf("RunbookMaxConcurrent = %d, want 10", cfg.RunbookMaxConcurrent)
	}
}

func TestRunbookMaxConcurrentFromSectionedFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := `[runbooks]
max_concurrent = 3
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_RUNBOOK_MAX_CONCURRENT", "")

	cfg := Load()
	if cfg.RunbookMaxConcurrent != 3 {
		t.Fatalf("RunbookMaxConcurrent = %d, want 3", cfg.RunbookMaxConcurrent)
	}
}

func TestRunbookMaxConcurrentFromLegacyFlatFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := `runbook_max_concurrent = 3
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_RUNBOOK_MAX_CONCURRENT", "")

	cfg := Load()
	if cfg.RunbookMaxConcurrent != 3 {
		t.Fatalf("RunbookMaxConcurrent = %d, want 3", cfg.RunbookMaxConcurrent)
	}
}

func TestRunbookMaxConcurrentInvalidFallsBack(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_RUNBOOK_MAX_CONCURRENT", "not-a-number")

	cfg := Load()
	if cfg.RunbookMaxConcurrent != 5 {
		t.Fatalf("RunbookMaxConcurrent = %d, want 5 (default fallback)", cfg.RunbookMaxConcurrent)
	}
}

func TestLoadDefaultValuesWithEmptyConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	// Write an empty file — all defaults should apply.
	if err := os.WriteFile(configPath, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_LOG_LEVEL", "")
	t.Setenv("SENTINEL_RUNBOOK_MAX_CONCURRENT", "")
	t.Setenv("SENTINEL_WATCHTOWER_ENABLED", "")
	t.Setenv("SENTINEL_WATCHTOWER_TICK_INTERVAL", "")
	t.Setenv("SENTINEL_WATCHTOWER_CAPTURE_LINES", "")
	t.Setenv("SENTINEL_WATCHTOWER_CAPTURE_TIMEOUT", "")
	t.Setenv("SENTINEL_WATCHTOWER_JOURNAL_ROWS", "")
	t.Setenv("SENTINEL_ALERT_CPU_PERCENT", "")
	t.Setenv("SENTINEL_ALERT_MEM_PERCENT", "")
	t.Setenv("SENTINEL_ALERT_DISK_PERCENT", "")
	t.Setenv("SENTINEL_COOKIE_SECURE", "")
	t.Setenv("SENTINEL_ALLOW_INSECURE_COOKIE", "")

	cfg := Load()

	if cfg.ListenAddr != "127.0.0.1:4040" {
		t.Errorf("ListenAddr = %q, want default", cfg.ListenAddr)
	}
	if cfg.Token != "" {
		t.Errorf("Token = %q, want empty", cfg.Token)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.CookieSecure != CookieSecureAuto {
		t.Errorf("CookieSecure = %q, want auto", cfg.CookieSecure)
	}
	if cfg.AllowInsecureCookie {
		t.Error("AllowInsecureCookie = true, want false")
	}
	if cfg.RunbookMaxConcurrent != 5 {
		t.Errorf("RunbookMaxConcurrent = %d, want 5", cfg.RunbookMaxConcurrent)
	}
	if !cfg.Watchtower.Enabled {
		t.Error("Watchtower.Enabled = false, want true")
	}
	if cfg.Watchtower.TickInterval != 1*time.Second {
		t.Errorf("Watchtower.TickInterval = %s, want 1s", cfg.Watchtower.TickInterval)
	}
	if cfg.Watchtower.CaptureLines != 80 {
		t.Errorf("Watchtower.CaptureLines = %d, want 80", cfg.Watchtower.CaptureLines)
	}
	if cfg.Watchtower.CaptureTimeout != 150*time.Millisecond {
		t.Errorf("Watchtower.CaptureTimeout = %s, want 150ms", cfg.Watchtower.CaptureTimeout)
	}
	if cfg.Watchtower.JournalRows != 5000 {
		t.Errorf("Watchtower.JournalRows = %d, want 5000", cfg.Watchtower.JournalRows)
	}
	if cfg.AlertThresholds.CPUPercent != 90.0 {
		t.Errorf("AlertThresholds.CPUPercent = %f, want 90.0", cfg.AlertThresholds.CPUPercent)
	}
	if cfg.AlertThresholds.MemPercent != 90.0 {
		t.Errorf("AlertThresholds.MemPercent = %f, want 90.0", cfg.AlertThresholds.MemPercent)
	}
	if cfg.AlertThresholds.DiskPercent != 95.0 {
		t.Errorf("AlertThresholds.DiskPercent = %f, want 95.0", cfg.AlertThresholds.DiskPercent)
	}
}

func TestLoadFullSectionedConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := `[server]
listen = "0.0.0.0:8080"
token = "my-token"
allowed_origins = "http://localhost:3000"
cookie_secure = "always"
allow_insecure_cookie = true
log_level = "debug"
timezone = "UTC"
locale = "en-US"

[alerts]
cpu_percent = 75.0
mem_percent = 80.0
disk_percent = 85.0

[watchtower]
enabled = false
tick_interval = "5s"
capture_lines = 200
capture_timeout = "500ms"
journal_rows = 10000

[runbooks]
max_concurrent = 8
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_LISTEN", "")
	t.Setenv("SENTINEL_TOKEN", "")
	t.Setenv("SENTINEL_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_LOG_LEVEL", "")
	t.Setenv("SENTINEL_COOKIE_SECURE", "")
	t.Setenv("SENTINEL_ALLOW_INSECURE_COOKIE", "")
	t.Setenv("SENTINEL_TIMEZONE", "")
	t.Setenv("SENTINEL_LOCALE", "")
	t.Setenv("SENTINEL_RUNBOOK_MAX_CONCURRENT", "")
	t.Setenv("SENTINEL_WATCHTOWER_ENABLED", "")
	t.Setenv("SENTINEL_WATCHTOWER_TICK_INTERVAL", "")
	t.Setenv("SENTINEL_WATCHTOWER_CAPTURE_LINES", "")
	t.Setenv("SENTINEL_WATCHTOWER_CAPTURE_TIMEOUT", "")
	t.Setenv("SENTINEL_WATCHTOWER_JOURNAL_ROWS", "")
	t.Setenv("SENTINEL_ALERT_CPU_PERCENT", "")
	t.Setenv("SENTINEL_ALERT_MEM_PERCENT", "")
	t.Setenv("SENTINEL_ALERT_DISK_PERCENT", "")

	cfg := Load()

	if cfg.ListenAddr != "0.0.0.0:8080" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, "0.0.0.0:8080")
	}
	if cfg.Token != "my-token" {
		t.Errorf("Token = %q, want %q", cfg.Token, "my-token")
	}
	if len(cfg.AllowedOrigins) != 1 || cfg.AllowedOrigins[0] != "http://localhost:3000" {
		t.Errorf("AllowedOrigins = %v, want [http://localhost:3000]", cfg.AllowedOrigins)
	}
	if cfg.CookieSecure != "always" {
		t.Errorf("CookieSecure = %q, want always", cfg.CookieSecure)
	}
	if !cfg.AllowInsecureCookie {
		t.Error("AllowInsecureCookie = false, want true")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want UTC", cfg.Timezone)
	}
	if cfg.Locale != "en-US" {
		t.Errorf("Locale = %q, want en-US", cfg.Locale)
	}
	if cfg.RunbookMaxConcurrent != 8 {
		t.Errorf("RunbookMaxConcurrent = %d, want 8", cfg.RunbookMaxConcurrent)
	}
	if cfg.Watchtower.Enabled {
		t.Error("Watchtower.Enabled = true, want false")
	}
	if cfg.Watchtower.TickInterval != 5*time.Second {
		t.Errorf("Watchtower.TickInterval = %s, want 5s", cfg.Watchtower.TickInterval)
	}
	if cfg.Watchtower.CaptureLines != 200 {
		t.Errorf("Watchtower.CaptureLines = %d, want 200", cfg.Watchtower.CaptureLines)
	}
	if cfg.Watchtower.CaptureTimeout != 500*time.Millisecond {
		t.Errorf("Watchtower.CaptureTimeout = %s, want 500ms", cfg.Watchtower.CaptureTimeout)
	}
	if cfg.Watchtower.JournalRows != 10000 {
		t.Errorf("Watchtower.JournalRows = %d, want 10000", cfg.Watchtower.JournalRows)
	}
	if cfg.AlertThresholds.CPUPercent != 75.0 {
		t.Errorf("AlertThresholds.CPUPercent = %f, want 75.0", cfg.AlertThresholds.CPUPercent)
	}
	if cfg.AlertThresholds.MemPercent != 80.0 {
		t.Errorf("AlertThresholds.MemPercent = %f, want 80.0", cfg.AlertThresholds.MemPercent)
	}
	if cfg.AlertThresholds.DiskPercent != 85.0 {
		t.Errorf("AlertThresholds.DiskPercent = %f, want 85.0", cfg.AlertThresholds.DiskPercent)
	}
}
