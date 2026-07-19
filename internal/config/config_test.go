package config

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDefaultUsesSentinelDataRoot(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("SENTINEL_DATA_DIR", "")
	t.Setenv("SENTINEL_CONFIG", "")

	cfg := Default()

	if got, want := cfg.Address(), "127.0.0.1:4040"; got != want {
		t.Fatalf("Address() = %q, want %q", got, want)
	}
	if got, want := cfg.Storage.Path, filepath.Join(dir, ".sentinel", "sentinel.db"); got != want {
		t.Fatalf("Storage.Path = %q, want %q", got, want)
	}
	if got, want := cfg.Log.Path, filepath.Join(dir, ".sentinel", "logs", "sentinel.log"); got != want {
		t.Fatalf("Log.Path = %q, want %q", got, want)
	}
}

func TestDefaultForDataDirDoesNotDependOnCallerHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dataDir := filepath.Join(t.TempDir(), "system-data")

	cfg := DefaultForDataDir(dataDir)
	if got, want := cfg.Storage.Path, filepath.Join(dataDir, "sentinel.db"); got != want {
		t.Fatalf("Storage.Path = %q, want %q", got, want)
	}
	if got, want := cfg.Log.Path, filepath.Join(dataDir, "logs", "sentinel.log"); got != want {
		t.Fatalf("Log.Path = %q, want %q", got, want)
	}
}

func TestDefaultForDeploymentUsesSeparateLogPath(t *testing.T) {
	t.Parallel()

	dataDir := filepath.Join(t.TempDir(), "lib")
	logPath := filepath.Join(t.TempDir(), "log", "sentinel.log")
	cfg := DefaultForDeployment(dataDir, logPath)
	if got := cfg.Storage.Path; got != filepath.Join(dataDir, "sentinel.db") {
		t.Fatalf("storage path = %q", got)
	}
	if got := cfg.Log.Path; got != logPath {
		t.Fatalf("log path = %q, want %q", got, logPath)
	}
}

func TestDefaultUsesManagedLogDefaultWithoutOverridingConfig(t *testing.T) {
	dataDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "sentinel.log")
	t.Setenv("SENTINEL_DATA_DIR", dataDir)
	t.Setenv(ManagedDefaultLogPathEnv, logPath)
	t.Setenv("SENTINEL_LOG_PATH", "")

	if got := Default().Log.Path; got != logPath {
		t.Fatalf("managed default log = %q, want %q", got, logPath)
	}
}

func TestLoadPathForDataDirRootsMissingConfigDefaultsInDeployment(t *testing.T) {
	clearConfigEnv(t)
	root := t.TempDir()
	configPath := filepath.Join(root, "etc", "config.toml")
	dataDir := filepath.Join(root, "var", "lib", "sentinel")

	cfg, gotPath, err := LoadPathForDataDir(configPath, dataDir)
	if err != nil {
		t.Fatalf("LoadPathForDataDir() error = %v", err)
	}
	if gotPath != configPath {
		t.Fatalf("path = %q, want %q", gotPath, configPath)
	}
	if got, want := cfg.Storage.Path, filepath.Join(dataDir, "sentinel.db"); got != want {
		t.Fatalf("Storage.Path = %q, want %q", got, want)
	}
}

func TestPathUsesExplicitConfigEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.toml")
	t.Setenv("SENTINEL_CONFIG", path)

	if got := Path(); got != path {
		t.Fatalf("Path() = %q, want %q", got, path)
	}
}

func TestLoadMissingConfigUsesDefaultsWithoutCreatingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_CONFIG", "")
	clearConfigEnv(t)

	cfg, path, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := path, filepath.Join(dir, "config.toml"); got != want {
		t.Fatalf("Load() path = %q, want %q", got, want)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load() created config file: %v", err)
	}
	if cfg.Address() != "127.0.0.1:4040" {
		t.Fatalf("Address() = %q", cfg.Address())
	}
}

func TestInitCreatesDefaultConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_CONFIG", "")

	path, err := Init(false)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	content := string(data)
	for _, fragment := range []string{
		"version = 1",
		"[server]",
		`host = "127.0.0.1"`,
		"port = 4040",
		"[storage]",
		`path = "` + filepath.Join(dir, "sentinel.db") + `"`,
		"[log]",
		`path = "` + filepath.Join(dir, "logs", "sentinel.log") + `"`,
		"[mcp]",
		"enabled = false",
	} {
		if !strings.Contains(content, fragment) {
			t.Fatalf("default config missing %q:\n%s", fragment, content)
		}
	}
}

func TestInitPathCreatesDeploymentConfig(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "etc", "sentinel", "config.toml")
	dataDir := filepath.Join(root, "var", "lib", "sentinel")

	gotPath, err := InitPath(configPath, dataDir, false)
	if err != nil {
		t.Fatalf("InitPath() error = %v", err)
	}
	if gotPath != configPath {
		t.Fatalf("path = %q, want %q", gotPath, configPath)
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`path = "` + filepath.Join(dataDir, "sentinel.db") + `"`,
		`path = "` + filepath.Join(dataDir, "logs", "sentinel.log") + `"`,
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("config missing %q:\n%s", want, content)
		}
	}
}

func TestInitRejectsExistingConfigWithoutForce(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	path := filepath.Join(dir, "config.toml")
	original := "version = 1\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Init(false)
	if !errors.Is(err, ErrConfigExists) {
		t.Fatalf("Init(false) error = %v, want ErrConfigExists", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("config was overwritten:\n%s", string(got))
	}
}

func TestInitForceOverwritesExistingConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("stale = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	gotPath, err := Init(true)
	if err != nil {
		t.Fatalf("Init(true) error = %v", err)
	}
	if gotPath != path {
		t.Fatalf("Init(true) path = %q, want %q", gotPath, path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "stale = true") {
		t.Fatalf("force init did not overwrite stale config:\n%s", string(got))
	}
}

func TestLoadSectionedConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	dbPath := filepath.Join(dir, "data", "sentinel.db")
	logPath := filepath.Join(dir, "logs", "sentinel.log")
	content := `version = 1

[server]
host = "0.0.0.0"
port = 8080
token = "my-token"
allowed_origins = ["http://localhost:3000"]
cookie_secure = "always"
allow_insecure_cookie = true
timezone = "UTC"
locale = "en-US"

[storage]
path = "` + dbPath + `"

[log]
level = "debug"
path = "` + logPath + `"

[health_report]
webhook_url = "https://example.com/report"
schedule = "@daily"

[watchtower]
enabled = false
tick_interval = "5s"
capture_lines = 200
capture_timeout = "500ms"
journal_rows = 10000

[runbooks]
max_concurrent = 8

[mcp]
enabled = true

[multi_user]
allowed_users = ["deploy"]
allow_root_target = true
user_switch_method = "sudo"
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SENTINEL_CONFIG", configPath)
	clearConfigEnv(t)

	cfg, resolved, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if resolved != configPath {
		t.Fatalf("resolved path = %q, want %q", resolved, configPath)
	}
	if cfg.Address() != "0.0.0.0:8080" {
		t.Fatalf("Address() = %q", cfg.Address())
	}
	if cfg.Server.Token != "my-token" {
		t.Fatalf("Server.Token = %q", cfg.Server.Token)
	}
	if len(cfg.Server.AllowedOrigins) != 1 || cfg.Server.AllowedOrigins[0] != "http://localhost:3000" {
		t.Fatalf("AllowedOrigins = %v", cfg.Server.AllowedOrigins)
	}
	if cfg.Storage.Path != dbPath || cfg.Log.Path != logPath {
		t.Fatalf("paths = storage:%q log:%q", cfg.Storage.Path, cfg.Log.Path)
	}
	if cfg.Log.Level != "debug" {
		t.Fatalf("Log.Level = %q", cfg.Log.Level)
	}
	if cfg.Watchtower.TickInterval != 5*time.Second || cfg.Watchtower.CaptureTimeout != 500*time.Millisecond {
		t.Fatalf("Watchtower = %+v", cfg.Watchtower)
	}
	if cfg.Runbooks.MaxConcurrent != 8 {
		t.Fatalf("Runbooks.MaxConcurrent = %d", cfg.Runbooks.MaxConcurrent)
	}
	if !cfg.MCP.Enabled {
		t.Fatal("MCP.Enabled = false, want true")
	}
	if cfg.MultiUser.UserSwitchMethod != "sudo" {
		t.Fatalf("UserSwitchMethod = %q", cfg.MultiUser.UserSwitchMethod)
	}
}

func TestLoadEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte(`version = 1
[server]
host = "0.0.0.0"
port = 8080
token = "file-token"
[storage]
path = "`+filepath.Join(dir, "file.db")+`"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SENTINEL_CONFIG", configPath)
	t.Setenv("SENTINEL_SERVER_HOST", "127.0.0.1")
	t.Setenv("SENTINEL_SERVER_PORT", "5050")
	t.Setenv("SENTINEL_SERVER_TOKEN", "env-token")
	t.Setenv("SENTINEL_STORAGE_PATH", filepath.Join(dir, "env.db"))
	t.Setenv("SENTINEL_MCP_ENABLED", "true")

	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := cfg.Address(), "127.0.0.1:5050"; got != want {
		t.Fatalf("Address() = %q, want %q", got, want)
	}
	if cfg.Server.Token != "env-token" {
		t.Fatalf("Server.Token = %q", cfg.Server.Token)
	}
	if cfg.Storage.Path != filepath.Join(dir, "env.db") {
		t.Fatalf("Storage.Path = %q", cfg.Storage.Path)
	}
	if !cfg.MCP.Enabled {
		t.Fatal("MCP.Enabled = false, want env override true")
	}
}

func TestApplyEnvOverridesAllRuntimeSections(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SENTINEL_SERVER_ALLOWED_ORIGINS", "https://one.example, https://two.example")
	t.Setenv("SENTINEL_SERVER_TRUSTED_PROXIES", "127.0.0.1/32,10.0.0.0/8")
	t.Setenv("SENTINEL_SERVER_COOKIE_SECURE", "always")
	t.Setenv("SENTINEL_SERVER_ALLOW_INSECURE_COOKIE", "true")
	t.Setenv("SENTINEL_SERVER_TIMEZONE", "America/Sao_Paulo")
	t.Setenv("SENTINEL_SERVER_LOCALE", "pt-BR")
	t.Setenv("SENTINEL_LOG_LEVEL", "debug")
	t.Setenv("SENTINEL_LOG_PATH", "/tmp/sentinel-test.log")
	t.Setenv("SENTINEL_HEALTH_REPORT_WEBHOOK_URL", "https://hooks.example/sentinel")
	t.Setenv("SENTINEL_HEALTH_REPORT_SCHEDULE", "0 * * * *")
	t.Setenv("SENTINEL_WATCHTOWER_ENABLED", "true")
	t.Setenv("SENTINEL_WATCHTOWER_TICK_INTERVAL", "3s")
	t.Setenv("SENTINEL_WATCHTOWER_CAPTURE_LINES", "120")
	t.Setenv("SENTINEL_WATCHTOWER_CAPTURE_TIMEOUT", "750ms")
	t.Setenv("SENTINEL_WATCHTOWER_JOURNAL_ROWS", "240")
	t.Setenv("SENTINEL_RUNBOOK_MAX_CONCURRENT", "7")
	t.Setenv("SENTINEL_ALLOWED_USERS", "alice, bob")
	t.Setenv("SENTINEL_ALLOW_ROOT_TARGET", "true")
	t.Setenv("SENTINEL_USER_SWITCH_METHOD", "sudo")

	cfg := Default()
	applyEnv(&cfg)

	if got, want := cfg.Server.AllowedOrigins, []string{"https://one.example", "https://two.example"}; !slices.Equal(got, want) {
		t.Fatalf("AllowedOrigins = %v, want %v", got, want)
	}
	if got, want := cfg.Server.TrustedProxies, []string{"127.0.0.1/32", "10.0.0.0/8"}; !slices.Equal(got, want) {
		t.Fatalf("TrustedProxies = %v, want %v", got, want)
	}
	if cfg.Server.CookieSecure != "always" || !cfg.Server.AllowInsecureCookie {
		t.Fatalf("cookie settings = secure:%q allow_insecure:%t", cfg.Server.CookieSecure, cfg.Server.AllowInsecureCookie)
	}
	if cfg.Server.Timezone != "America/Sao_Paulo" || cfg.Server.Locale != "pt-BR" {
		t.Fatalf("server locale settings = timezone:%q locale:%q", cfg.Server.Timezone, cfg.Server.Locale)
	}
	if cfg.Log.Level != "debug" || cfg.Log.Path != "/tmp/sentinel-test.log" {
		t.Fatalf("log settings = %+v", cfg.Log)
	}
	if cfg.HealthReport.WebhookURL != "https://hooks.example/sentinel" || cfg.HealthReport.Schedule != "0 * * * *" {
		t.Fatalf("health report settings = %+v", cfg.HealthReport)
	}
	if !cfg.Watchtower.Enabled || cfg.Watchtower.TickInterval != 3*time.Second || cfg.Watchtower.CaptureLines != 120 || cfg.Watchtower.CaptureTimeout != 750*time.Millisecond || cfg.Watchtower.JournalRows != 240 {
		t.Fatalf("watchtower settings = %+v", cfg.Watchtower)
	}
	if cfg.Runbooks.MaxConcurrent != 7 {
		t.Fatalf("Runbooks.MaxConcurrent = %d, want 7", cfg.Runbooks.MaxConcurrent)
	}
	if got, want := cfg.MultiUser.AllowedUsers, []string{"alice", "bob"}; !slices.Equal(got, want) {
		t.Fatalf("AllowedUsers = %v, want %v", got, want)
	}
	if !cfg.MultiUser.AllowRootTarget || cfg.MultiUser.UserSwitchMethod != "sudo" {
		t.Fatalf("multi-user settings = %+v", cfg.MultiUser)
	}
}

func TestLoadRejectsEnabledMCPWithoutSharedToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[mcp]\nenabled = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	clearConfigEnv(t)

	_, _, err := LoadPath(path)
	if err == nil || !strings.Contains(err.Error(), "mcp.enabled requires server.token") {
		t.Fatalf("LoadPath() error = %v", err)
	}
}

func TestServerPortEnvInvalidPreservesConfig(t *testing.T) {
	for _, value := range []string{"-1", "999999", "0"} {
		t.Run(value, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte("[server]\nport = 5050\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("SENTINEL_SERVER_PORT", value)
			cfg, _, err := LoadPath(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Server.Port != 5050 {
				t.Fatalf("port = %d, want 5050", cfg.Server.Port)
			}
		})
	}
}

func TestServerPortEnvInvalidPreservesDefault(t *testing.T) {
	for _, value := range []string{"", "0"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("SENTINEL_SERVER_PORT", value)
			cfg, _, err := LoadPath(filepath.Join(t.TempDir(), "missing.toml"))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Server.Port != defaultPort {
				t.Fatalf("port = %d, want %d", cfg.Server.Port, defaultPort)
			}
		})
	}
}

func TestCSVEnvReplacesLists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[server]\nallowed_origins = [\"http://old\"]\n[multi_user]\nallowed_users = [\"old\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SENTINEL_SERVER_ALLOWED_ORIGINS", "https://a.example, https://b.example")
	t.Setenv("SENTINEL_SERVER_TRUSTED_PROXIES", "127.0.0.1")
	t.Setenv("SENTINEL_ALLOWED_USERS", "alice,bob")
	cfg, _, err := LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cfg.Server.AllowedOrigins, ","); got != "https://a.example,https://b.example" {
		t.Fatalf("origins = %v", cfg.Server.AllowedOrigins)
	}
	if got := strings.Join(cfg.MultiUser.AllowedUsers, ","); got != "alice,bob" {
		t.Fatalf("users = %v", cfg.MultiUser.AllowedUsers)
	}
}

func TestValidateFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name: "valid",
			content: `version = 1
[server]
host = "127.0.0.1"
port = 4040
[log]
level = "debug"
[health_report]
schedule = "@daily"
`,
		},
		{name: "legacy listen rejected", content: "[server]\nlisten = \"127.0.0.1:4040\"\n", wantErr: "unknown key: server.listen"},
		{name: "invalid port", content: "[server]\nport = 999999\n", wantErr: "server.port"},
		{name: "invalid log level", content: "[log]\nlevel = \"verbose\"\n", wantErr: "log.level"},
		{name: "invalid schedule", content: "[health_report]\nschedule = \"not cron\"\n", wantErr: "health_report.schedule"},
		{name: "origin with path", content: "[server]\nallowed_origins = [\"https://example.com/path\"]\n", wantErr: "must not contain credentials, a path"},
		{name: "invalid trusted proxy", content: "[server]\ntrusted_proxies = [\"localhost\"]\n", wantErr: "must be an IP address or CIDR"},
		{name: "https origin supports implicit loopback proxy", content: "[server]\nallowed_origins = [\"https://example.com\"]\n"},
		{name: "https origin with trusted proxy", content: "[server]\nallowed_origins = [\"https://example.com\"]\ntrusted_proxies = [\"127.0.0.1\"]\n"},
		{name: "unknown key", content: "[server]\nwat = true\n", wantErr: "unknown key: server.wat"},
		{name: "bad toml", content: "[server\n", wantErr: "decode config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			err := ValidateFile(path)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateFile() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateFile() error = %v, want fragment %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateFileMissing(t *testing.T) {
	err := ValidateFile(filepath.Join(t.TempDir(), "config.toml"))
	if err == nil || !strings.Contains(err.Error(), "config file not found") {
		t.Fatalf("ValidateFile() error = %v, want missing file error", err)
	}
}

func TestEnsureCreatesConfigAndDirs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_CONFIG", "")
	clearConfigEnv(t)

	cfg, path, err := Ensure()
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config not created: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(cfg.Storage.Path)); err != nil {
		t.Fatalf("storage dir not created: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(cfg.Log.Path)); err != nil {
		t.Fatalf("log dir not created: %v", err)
	}
}

func TestExpandPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("SENTINEL_TMP_NAME", "nested")

	got, err := ExpandPath("~/$SENTINEL_TMP_NAME/file.txt")
	if err != nil {
		t.Fatalf("ExpandPath() error = %v", err)
	}
	want := filepath.Join(dir, "nested", "file.txt")
	if got != want {
		t.Fatalf("ExpandPath() = %q, want %q", got, want)
	}
}

func TestLoadFallsBackToCurrentUserHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", "")
	t.Setenv("SENTINEL_CONFIG", "")
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

	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := filepath.Join(dir, ".sentinel")
	if cfg.DataDir() != want {
		t.Fatalf("DataDir() = %q, want %q", cfg.DataDir(), want)
	}
}

func TestLoadFallsBackToTempDirWhenHomeUnavailable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", "")
	t.Setenv("SENTINEL_CONFIG", "")
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

	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := filepath.Join(dir, "sentinel")
	if cfg.DataDir() != want {
		t.Fatalf("DataDir() = %q, want %q", cfg.DataDir(), want)
	}
}

func TestParseHelpers(t *testing.T) {
	if v, ok := parseBool("yes"); !ok || !v {
		t.Fatalf("parseBool yes = %t, %t", v, ok)
	}
	if _, ok := parseBool("maybe"); ok {
		t.Fatal("parseBool accepted invalid value")
	}
	if v, ok := parseDuration("150ms"); !ok || v != 150*time.Millisecond {
		t.Fatalf("parseDuration = %s, %t", v, ok)
	}
	if _, ok := parseDuration("0s"); ok {
		t.Fatal("parseDuration accepted zero")
	}
	if v, ok := parsePositiveInt("42"); !ok || v != 42 {
		t.Fatalf("parsePositiveInt = %d, %t", v, ok)
	}
	if _, ok := parsePositiveInt("-1"); ok {
		t.Fatal("parsePositiveInt accepted negative")
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"SENTINEL_SERVER_HOST",
		"SENTINEL_SERVER_PORT",
		"SENTINEL_SERVER_TOKEN",
		"SENTINEL_SERVER_ALLOWED_ORIGINS",
		"SENTINEL_SERVER_TRUSTED_PROXIES",
		"SENTINEL_SERVER_COOKIE_SECURE",
		"SENTINEL_SERVER_ALLOW_INSECURE_COOKIE",
		"SENTINEL_SERVER_TIMEZONE",
		"SENTINEL_SERVER_LOCALE",
		"SENTINEL_STORAGE_PATH",
		"SENTINEL_LOG_LEVEL",
		"SENTINEL_LOG_PATH",
		ManagedDefaultLogPathEnv,
		"SENTINEL_HEALTH_REPORT_WEBHOOK_URL",
		"SENTINEL_HEALTH_REPORT_SCHEDULE",
		"SENTINEL_WATCHTOWER_ENABLED",
		"SENTINEL_WATCHTOWER_TICK_INTERVAL",
		"SENTINEL_WATCHTOWER_CAPTURE_LINES",
		"SENTINEL_WATCHTOWER_CAPTURE_TIMEOUT",
		"SENTINEL_WATCHTOWER_JOURNAL_ROWS",
		"SENTINEL_RUNBOOK_MAX_CONCURRENT",
		"SENTINEL_MCP_ENABLED",
		"SENTINEL_ALLOWED_USERS",
		"SENTINEL_ALLOW_ROOT_TARGET",
		"SENTINEL_USER_SWITCH_METHOD",
	} {
		t.Setenv(key, "")
	}
}
