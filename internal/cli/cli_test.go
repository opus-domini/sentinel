package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/daemon"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/updater"
)

const (
	testSentinelPath    = "/tmp/sentinel"
	testCurrentVersion1 = "1.0.0"
	testScopeUser       = "user"
	testScopeSystem     = "system"
)

func TestRunWithoutArgsPrintsHelp(t *testing.T) {
	origDaemon := daemonFn
	t.Cleanup(func() { daemonFn = origDaemon })

	called := false
	daemonFn = func() int {
		called = true
		return 0
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run(nil, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if called {
		t.Fatal("daemonFn must not run when sentinel is invoked without args")
	}
	if !strings.Contains(out.String(), "USAGE") {
		t.Fatalf("stdout missing root help: %s", out.String())
	}
}

func TestRunCLIConfigInitCreatesConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"config", "init"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	configPath := filepath.Join(dir, "config.toml")
	if !strings.Contains(out.String(), "wrote "+configPath) {
		t.Fatalf("unexpected stdout: %s", out.String())
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), "# Sentinel configuration") {
		t.Fatalf("config missing default content:\n%s", string(data))
	}
}

func TestRunCLIConfigInitRequiresForceForExistingConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte("stale = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"config", "init"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "use --force to overwrite") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}

	out.Reset()
	errOut.Reset()
	code = Run([]string{"config", "init", "--force"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "overwrote "+configPath) {
		t.Fatalf("unexpected stdout: %s", out.String())
	}
}

func TestRunCLIConfigPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"config", "path"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if got, want := strings.TrimSpace(out.String()), filepath.Join(dir, "config.toml"); got != want {
		t.Fatalf("config path output = %q, want %q", got, want)
	}
}

func TestRunCLIConfigFlagOverridesPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_CONFIG", "")
	configPath := filepath.Join(dir, "custom.toml")

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"--config", configPath, "config", "path"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if got := strings.TrimSpace(out.String()); got != configPath {
		t.Fatalf("config path output = %q, want %q", got, configPath)
	}
}

func TestRunCLIConfigValidate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte("[log]\nlevel = \"info\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"config", "validate"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "ok: "+configPath+" - config valid") {
		t.Fatalf("unexpected stdout: %s", out.String())
	}

	if err := os.WriteFile(configPath, []byte("[log]\nlevel = \"verbose\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	code = Run([]string{"config", "validate"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "log.level") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunCLIConfigShowPrintsEffectiveConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_SERVER_HOST", "")
	t.Setenv("SENTINEL_SERVER_PORT", "")
	t.Setenv("SENTINEL_SERVER_ALLOWED_ORIGINS", "")
	t.Setenv("SENTINEL_LOG_LEVEL", "")
	configPath := filepath.Join(dir, "config.toml")
	content := `version = 1

[server]
host = "127.0.0.1"
port = 5050
token = "super-secret"
allowed_origins = ["http://localhost:3000", "http://127.0.0.1:3000"]

[log]
level = "debug"

[watchtower]
enabled = false
tick_interval = "2s"
capture_timeout = "250ms"

[health_report]
webhook_url = "https://discord.com/api/webhooks/123/secret"
schedule = "0 9 * * *"
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"config", "show"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("config show output is not JSON: %v\n%s", err, out.String())
	}
	server, ok := got["server"].(map[string]any)
	if !ok {
		t.Fatalf("server = %#v", got["server"])
	}
	if server["host"] != "127.0.0.1" || server["port"] != float64(5050) {
		t.Fatalf("server = %#v", server)
	}
	logCfg, ok := got["log"].(map[string]any)
	if !ok || logCfg["level"] != "debug" {
		t.Fatalf("log = %#v", got["log"])
	}
	if server["token"] != "******" {
		t.Fatalf("token = %v, want redacted", server["token"])
	}
	origins, ok := server["allowed_origins"].([]any)
	if !ok || len(origins) != 2 || origins[0] != "http://localhost:3000" || origins[1] != "http://127.0.0.1:3000" {
		t.Fatalf("allowed_origins = %#v", server["allowed_origins"])
	}
	watchtower, ok := got["watchtower"].(map[string]any)
	if !ok {
		t.Fatalf("watchtower = %#v", got["watchtower"])
	}
	if watchtower["enabled"] != false {
		t.Fatalf("watchtower.enabled = %v", watchtower["enabled"])
	}
	if watchtower["tick_interval"] != "2s" {
		t.Fatalf("watchtower.tick_interval = %v", watchtower["tick_interval"])
	}
	if watchtower["capture_timeout"] != "250ms" {
		t.Fatalf("watchtower.capture_timeout = %v", watchtower["capture_timeout"])
	}
	// Webhook URLs embed secrets in their path and must be
	// redacted like the token (config show is pasted into logs/support/screens).
	healthCfg, ok := got["health_report"].(map[string]any)
	if !ok || healthCfg["webhook_url"] != "******" {
		t.Fatalf("health_report.webhook_url = %v, want redacted", got["health_report"])
	}
	if healthCfg["schedule"] != "0 9 * * *" {
		t.Fatalf("health_report.schedule = %v, want preserved", healthCfg["schedule"])
	}
}

func TestRunCLIConfigShowValidatesExistingConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("SENTINEL_CONFIG", "")
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte("[log]\nlevel = \"verbose\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"config", "show"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stdout: %s)", code, out.String())
	}
	if !strings.Contains(errOut.String(), "log.level") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunCLIConfigEditInitializesRunsEditorAndValidates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	editorPath := filepath.Join(t.TempDir(), "editor.sh")
	if err := os.WriteFile(editorPath, []byte("#!/bin/sh\nprintf '\\n# edited\\n' >> \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", editorPath)
	t.Setenv("VISUAL", "")

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"config", "edit"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	configPath := filepath.Join(dir, "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), "# edited") {
		t.Fatalf("editor did not update config:\n%s", string(data))
	}
	if !strings.Contains(out.String(), "config valid") {
		t.Fatalf("unexpected stdout: %s", out.String())
	}
}

func TestRunCLIConfigEditXDGOpenFallbackSkipsValidation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")

	origLookup := lookupExec
	origExec := execCommand
	t.Cleanup(func() {
		lookupExec = origLookup
		execCommand = origExec
	})
	lookupExec = func(name string) (string, error) {
		if name == "xdg-open" {
			return "/usr/bin/xdg-open", nil
		}
		return "", os.ErrNotExist
	}
	var gotName string
	var gotArgs []string
	execCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return exec.CommandContext(ctx, "/bin/true")
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"config", "edit"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	configPath := filepath.Join(dir, "config.toml")
	if gotName != "xdg-open" || len(gotArgs) != 1 || gotArgs[0] != configPath {
		t.Fatalf("editor command = %s %v, want xdg-open [%s]", gotName, gotArgs, configPath)
	}
	if !strings.Contains(out.String(), "Run `sentinel config validate` after saving.") {
		t.Fatalf("unexpected stdout: %s", out.String())
	}
}

func TestRunCLIConfigEditRequiresEditor(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")

	origLookup := lookupExec
	t.Cleanup(func() { lookupExec = origLookup })
	lookupExec = func(string) (string, error) { return "", os.ErrNotExist }

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"config", "edit"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stdout: %s)", code, out.String())
	}
	if !strings.Contains(errOut.String(), "$EDITOR") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunCLIDBInitCreatesConfigAndDatabase(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"db", "init"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	configPath := filepath.Join(dir, "config.toml")
	dbPath := filepath.Join(dir, "sentinel.db")
	for _, path := range []string{configPath, dbPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
	}
	for _, fragment := range []string{
		"config: " + configPath,
		"database: " + dbPath,
		"status: ok",
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("stdout missing %q: %s", fragment, out.String())
		}
	}
}

func TestRunCLIDBStatus(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"db", "status"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	for _, fragment := range []string{
		"database: " + filepath.Join(dir, "sentinel.db"),
		"total size:",
		store.StorageResourceActivityLog + ":",
		store.StorageResourceOpsJobs + ":",
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("stdout missing %q: %s", fragment, out.String())
		}
	}
}

func TestRunCLIDBResetRequiresYes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"db", "reset"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stdout: %s)", code, out.String())
	}
	if !strings.Contains(errOut.String(), "without --yes") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunCLIDBResetFlushesResource(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"db", "reset", "--yes", "--resource", store.StorageResourceOpsJobs}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	for _, fragment := range []string{
		"database: " + filepath.Join(dir, "sentinel.db"),
		"mode: flush",
		"resource: " + store.StorageResourceOpsJobs,
		store.StorageResourceOpsJobs + ": 0 rows removed",
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("stdout missing %q: %s", fragment, out.String())
		}
	}
}

func TestRunCLIDBResetForceWipesAndRecreatesDatabase(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)
	dbPath := filepath.Join(dir, "sentinel.db")

	st, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	if err := st.UpsertSession(context.Background(), "keep", "hash", "content"); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	if err := os.WriteFile(dbPath+"-journal", []byte("stale"), 0o600); err != nil {
		t.Fatalf("write journal sidecar: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"db", "reset", "--yes", "--force"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	for _, fragment := range []string{
		"database: " + dbPath,
		"mode: force",
		"status: recreated",
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("stdout missing %q: %s", fragment, out.String())
		}
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("stat recreated database: %v", err)
	}
	if _, err := os.Stat(dbPath + "-journal"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("journal sidecar exists after force reset: %v", err)
	}

	st, err = store.New(dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer func() { _ = st.Close() }()
	sessions, err := st.GetAll(context.Background())
	if err != nil {
		t.Fatalf("get sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions after force reset = %v, want empty", sessions)
	}
}

func TestRunCLIDBResetRejectsInvalidResource(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"db", "reset", "--yes", "--resource", "bogus"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stdout: %s)", code, out.String())
	}
	if !strings.Contains(errOut.String(), "invalid storage resource") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunCLIDBResetForceRejectsResource(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENTINEL_DATA_DIR", dir)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"db", "reset", "--yes", "--force", "--resource", store.StorageResourceOpsJobs}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stdout: %s)", code, out.String())
	}
	if !strings.Contains(errOut.String(), "cannot combine --force with --resource") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunCLIServiceInstallParsesFlags(t *testing.T) {
	origInstall := installUserSvcFn
	t.Cleanup(func() { installUserSvcFn = origInstall })

	var got daemon.InstallUserOptions
	installUserSvcFn = func(opts daemon.InstallUserOptions) error {
		got = opts
		return nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"service", "install", "--exec", testSentinelPath, "--enable=false", "--start=false"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if got.ExecPath != testSentinelPath {
		t.Fatalf("ExecPath = %q, want %s", got.ExecPath, testSentinelPath)
	}
	if got.Enable {
		t.Fatal("Enable = true, want false")
	}
	if got.Start {
		t.Fatal("Start = true, want false")
	}
}

func TestRunCLIServiceStatus(t *testing.T) {
	origStatus := serviceStatusFn
	t.Cleanup(func() { serviceStatusFn = origStatus })

	serviceStatusFn = func() ([]daemon.ScopedServiceStatus, error) {
		return []daemon.ScopedServiceStatus{{
			Scope: "user",
			UserServiceStatus: daemon.UserServiceStatus{
				ServicePath:        "/tmp/sentinel.service",
				UnitFileExists:     true,
				SystemctlAvailable: true,
				EnabledState:       "enabled",
				ActiveState:        "active",
			},
		}}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"service", "status"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	text := out.String()
	managerLabel := runtimeServiceManagerLabel()
	for _, fragment := range []string{
		"user unit file: /tmp/sentinel.service",
		"user unit exists: true",
		managerLabel + " available: true",
		"user unit enabled: enabled",
		"user unit active: active",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, text)
		}
	}
}

func TestRunCLIServiceStatusSystemUnitLabel(t *testing.T) {
	origStatus := serviceStatusFn
	t.Cleanup(func() { serviceStatusFn = origStatus })

	serviceStatusFn = func() ([]daemon.ScopedServiceStatus, error) {
		return []daemon.ScopedServiceStatus{{
			Scope: "system",
			UserServiceStatus: daemon.UserServiceStatus{
				ServicePath:        "/etc/systemd/system/sentinel.service",
				UnitFileExists:     false,
				SystemctlAvailable: true,
				EnabledState:       "not-found",
				ActiveState:        "inactive",
			},
		}}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"service", "status"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	text := out.String()
	managerLabel := runtimeServiceManagerLabel()
	for _, fragment := range []string{
		"system unit file: /etc/systemd/system/sentinel.service",
		"system unit exists: false",
		managerLabel + " available: true",
		"system unit enabled: not-found",
		"system unit active: inactive",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, text)
		}
	}
}

func TestRunCLIDoctor(t *testing.T) {
	origLoad := loadConfigFn
	origStatus := serviceStatusFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		serviceStatusFn = origStatus
	})

	loadConfigFn = testLoadConfig("/tmp/.sentinel", "token")
	serviceStatusFn = func() ([]daemon.ScopedServiceStatus, error) {
		return []daemon.ScopedServiceStatus{{
			Scope: "user",
			UserServiceStatus: daemon.UserServiceStatus{
				ServicePath:        "/tmp/sentinel.service",
				UnitFileExists:     true,
				EnabledState:       "enabled",
				ActiveState:        "active",
				SystemctlAvailable: true,
			},
		}}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"doctor"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	text := out.String()
	for _, fragment := range []string{
		"Sentinel doctor report",
		"listen: 127.0.0.1:4040",
		"data dir: /tmp/.sentinel",
		"token required: true",
		"user unit file: /tmp/sentinel.service",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, text)
		}
	}
}

func TestRunCLIDoctorSystemUnitLabel(t *testing.T) {
	origLoad := loadConfigFn
	origStatus := serviceStatusFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		serviceStatusFn = origStatus
	})

	loadConfigFn = testLoadConfig("/tmp/.sentinel", "")
	serviceStatusFn = func() ([]daemon.ScopedServiceStatus, error) {
		return []daemon.ScopedServiceStatus{{
			Scope: "system",
			UserServiceStatus: daemon.UserServiceStatus{
				ServicePath:        "/etc/systemd/system/sentinel.service",
				UnitFileExists:     false,
				EnabledState:       "not-found",
				ActiveState:        "inactive",
				SystemctlAvailable: true,
			},
		}}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"doctor"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	text := out.String()
	for _, fragment := range []string{
		"system unit file: /etc/systemd/system/sentinel.service",
		"system unit exists: false",
		"system unit enabled: not-found",
		"system unit active: inactive",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, text)
		}
	}
}

func TestRunCLIUnknownCommand(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"unknown"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunCLIServiceInstallFailure(t *testing.T) {
	origInstall := installUserSvcFn
	t.Cleanup(func() { installUserSvcFn = origInstall })

	installUserSvcFn = func(_ daemon.InstallUserOptions) error {
		return errors.New("install failed")
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"service", "install"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "service install failed") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunCLIHelpFlag(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"-h"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "USAGE") {
		t.Fatalf("unexpected help output: %s", out.String())
	}
}

func TestRunCLIServiceAutoUpdateInstallParsesFlags(t *testing.T) {
	origInstall := installUserAutoUpdateFn
	t.Cleanup(func() { installUserAutoUpdateFn = origInstall })

	var got daemon.InstallUserAutoUpdateOptions
	installUserAutoUpdateFn = func(opts daemon.InstallUserAutoUpdateOptions) error {
		got = opts
		return nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{
		"service", "autoupdate", "install",
		"--exec", testSentinelPath,
		"--enable=false",
		"--start=false",
		"--service", "sentinel-custom",
		"--scope", testScopeSystem,
		"--on-calendar", "hourly",
		"--randomized-delay", "30m",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if got.ExecPath != testSentinelPath {
		t.Fatalf("ExecPath = %q, want %s", got.ExecPath, testSentinelPath)
	}
	if got.Enable {
		t.Fatal("Enable = true, want false")
	}
	if got.Start {
		t.Fatal("Start = true, want false")
	}
	if got.ServiceUnit != "sentinel-custom" {
		t.Fatalf("ServiceUnit = %q, want sentinel-custom", got.ServiceUnit)
	}
	if got.SystemdScope != testScopeSystem {
		t.Fatalf("SystemdScope = %q, want %s", got.SystemdScope, testScopeSystem)
	}
	if got.OnCalendar != "hourly" {
		t.Fatalf("OnCalendar = %q, want hourly", got.OnCalendar)
	}
	if got.RandomizedDelay != 30*time.Minute {
		t.Fatalf("RandomizedDelay = %s, want 30m", got.RandomizedDelay)
	}
}

func TestRunCLIServiceAutoUpdateStatus(t *testing.T) {
	origStatus := userAutoUpdateStatusFn
	t.Cleanup(func() { userAutoUpdateStatusFn = origStatus })

	userAutoUpdateStatusFn = func(scope string) (daemon.UserAutoUpdateServiceStatus, error) {
		if scope != testScopeUser {
			t.Fatalf("scope = %q, want %s", scope, testScopeUser)
		}
		return daemon.UserAutoUpdateServiceStatus{
			ServicePath:        "/tmp/sentinel-updater.service",
			TimerPath:          "/tmp/sentinel-updater.timer",
			ServiceUnitExists:  true,
			TimerUnitExists:    true,
			SystemctlAvailable: true,
			TimerEnabledState:  "enabled",
			TimerActiveState:   "active",
			LastRunState:       "inactive",
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"service", "autoupdate", "status", "--scope", testScopeUser}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	text := out.String()
	for _, fragment := range []string{
		"service file: /tmp/sentinel-updater.service",
		"timer file: /tmp/sentinel-updater.timer",
		"timer enabled: enabled",
		"timer active: active",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, text)
		}
	}
}

func TestRunCLIServiceAutoUpdateStatusScopeFlag(t *testing.T) {
	origStatus := userAutoUpdateStatusFn
	t.Cleanup(func() { userAutoUpdateStatusFn = origStatus })

	userAutoUpdateStatusFn = func(scope string) (daemon.UserAutoUpdateServiceStatus, error) {
		if scope != testScopeSystem {
			t.Fatalf("scope = %q, want %s", scope, testScopeSystem)
		}
		return daemon.UserAutoUpdateServiceStatus{}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"service", "autoupdate", "status", "--scope", testScopeSystem}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
}

func TestRunCLIServiceAutoUpdateUninstallScopeFlag(t *testing.T) {
	origUninstall := uninstallUserAutoUpdateFn
	t.Cleanup(func() { uninstallUserAutoUpdateFn = origUninstall })

	var got daemon.UninstallUserAutoUpdateOptions
	uninstallUserAutoUpdateFn = func(opts daemon.UninstallUserAutoUpdateOptions) error {
		got = opts
		return nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"service", "autoupdate", "uninstall", "--scope", testScopeSystem}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if got.Scope != testScopeSystem {
		t.Fatalf("Scope = %q, want %s", got.Scope, testScopeSystem)
	}
}

func TestRunCLIUpdateCheck(t *testing.T) {
	origLoad := loadConfigFn
	origVersion := currentVersionFn
	origCheck := updateCheckFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		currentVersionFn = origVersion
		updateCheckFn = origCheck
	})

	loadConfigFn = testLoadConfig(testSentinelPath, "")
	currentVersionFn = func() string { return testCurrentVersion1 }

	var got updater.CheckOptions
	updateCheckFn = func(_ context.Context, opts updater.CheckOptions) (updater.CheckResult, error) {
		got = opts
		return updater.CheckResult{
			CurrentVersion: "1.0.0",
			LatestVersion:  "1.1.0",
			UpToDate:       false,
			ReleaseURL:     "https://github.com/opus-domini/sentinel/releases/tag/v1.1.0",
			AssetName:      "sentinel-1.1.0-linux-amd64.tar.gz",
			ExpectedSHA256: strings.Repeat("a", 64),
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"update", "check", "--repo", "opus-domini/sentinel", "--api", "http://example", "--os", "linux", "--arch", "amd64"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if got.CurrentVersion != testCurrentVersion1 {
		t.Fatalf("CurrentVersion = %q, want %s", got.CurrentVersion, testCurrentVersion1)
	}
	if got.DataDir != testSentinelPath {
		t.Fatalf("DataDir = %q, want %s", got.DataDir, testSentinelPath)
	}
	if got.Repo != "opus-domini/sentinel" {
		t.Fatalf("Repo = %q, want opus-domini/sentinel", got.Repo)
	}
	if !strings.Contains(out.String(), "latest version: 1.1.0") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestRunCLIUpdateApplyParsesFlags(t *testing.T) {
	origLoad := loadConfigFn
	origVersion := currentVersionFn
	origApply := updateApplyFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		currentVersionFn = origVersion
		updateApplyFn = origApply
	})

	loadConfigFn = testLoadConfig(testSentinelPath, "")
	currentVersionFn = func() string { return testCurrentVersion1 }

	var got updater.ApplyOptions
	updateApplyFn = func(_ context.Context, opts updater.ApplyOptions) (updater.ApplyResult, error) {
		got = opts
		return updater.ApplyResult{
			Applied:        true,
			CurrentVersion: testCurrentVersion1,
			LatestVersion:  "1.1.0",
			BinaryPath:     testSentinelPath,
			BackupPath:     testSentinelPath + ".bak",
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{
		"update", "apply",
		"--repo", "opus-domini/sentinel",
		"--api", "http://example",
		"--os", "linux",
		"--arch", "amd64",
		"--exec", testSentinelPath,
		"--allow-downgrade=true",
		"--allow-unverified=true",
		"--restart=false",
		"--service", "sentinel",
		"--scope", testScopeSystem,
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if got.DataDir != testSentinelPath {
		t.Fatalf("DataDir = %q, want %s", got.DataDir, testSentinelPath)
	}
	if got.ExecPath != testSentinelPath {
		t.Fatalf("ExecPath = %q, want %s", got.ExecPath, testSentinelPath)
	}
	if !got.AllowDowngrade {
		t.Fatal("AllowDowngrade = false, want true")
	}
	if !got.AllowUnverified {
		t.Fatal("AllowUnverified = false, want true")
	}
	if !got.SkipRestart {
		t.Fatal("SkipRestart = false, want true")
	}
	if got.SystemdScope != testScopeSystem {
		t.Fatalf("SystemdScope = %q, want %s", got.SystemdScope, testScopeSystem)
	}
	if !strings.Contains(out.String(), "updated from: 1.0.0") || !strings.Contains(out.String(), "updated to: 1.1.0") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestRunCLIUpdateApplyRestartsByDefault(t *testing.T) {
	origLoad := loadConfigFn
	origVersion := currentVersionFn
	origApply := updateApplyFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		currentVersionFn = origVersion
		updateApplyFn = origApply
	})

	loadConfigFn = testLoadConfig(testSentinelPath, "")
	currentVersionFn = func() string { return testCurrentVersion1 }

	var got updater.ApplyOptions
	updateApplyFn = func(_ context.Context, opts updater.ApplyOptions) (updater.ApplyResult, error) {
		got = opts
		return updater.ApplyResult{
			Applied:        false,
			CurrentVersion: testCurrentVersion1,
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"update", "apply"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if got.SkipRestart {
		t.Fatal("SkipRestart = true, want false by default")
	}
	if got.SystemdScope != "auto" {
		t.Fatalf("SystemdScope = %q, want auto by default", got.SystemdScope)
	}
}

func TestRunCLIUpdateApplyRejectsInvalidScope(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"update", "apply", "--scope", "invalid"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "unsupported update apply scope") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunCLIUpdateStatus(t *testing.T) {
	origLoad := loadConfigFn
	origStatus := updateStatusFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		updateStatusFn = origStatus
	})

	loadConfigFn = testLoadConfig(testSentinelPath, "")
	updateStatusFn = func(dataDir string) (updater.State, error) {
		if dataDir != testSentinelPath {
			t.Fatalf("dataDir = %q, want %s", dataDir, testSentinelPath)
		}
		return updater.State{
			LastCheckedAt:  time.Date(2026, time.February, 15, 12, 0, 0, 0, time.UTC),
			LastAppliedAt:  time.Date(2026, time.February, 15, 12, 30, 0, 0, time.UTC),
			CurrentVersion: "1.1.0",
			LatestVersion:  "1.1.0",
			UpToDate:       true,
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"update", "status"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	text := out.String()
	for _, fragment := range []string{
		"current version: 1.1.0",
		"latest version: 1.1.0",
		"up to date: true",
		"last checked: 2026-02-15T12:00:00Z",
		"last applied: 2026-02-15T12:30:00Z",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, text)
		}
	}
}

func TestRunCLIVersionFlag(t *testing.T) {
	origVersion := currentVersionFn
	t.Cleanup(func() { currentVersionFn = origVersion })
	currentVersionFn = func() string { return "v1.2.3" }

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run([]string{"--version"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(out.String()) != "sentinel version v1.2.3" {
		t.Fatalf("unexpected version output: %q", out.String())
	}
}
