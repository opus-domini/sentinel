package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/opus-domini/sentinel/internal/daemon"
)

func TestPreflightInstallDestination(t *testing.T) {
	t.Parallel()

	destination := filepath.Join(t.TempDir(), "nested", "sentinel")
	if err := preflightInstallDestination(destination); err != nil {
		t.Fatalf("preflightInstallDestination() error = %v", err)
	}
	if _, err := os.Stat(filepath.Dir(destination)); err != nil {
		t.Fatalf("destination directory was not created: %v", err)
	}

	blockingFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := preflightInstallDestination(filepath.Join(blockingFile, "sentinel"))
	if err == nil || !strings.Contains(err.Error(), "create binary directory") {
		t.Fatalf("error = %v, want directory creation failure", err)
	}
}

func TestRemoveSentinelBinaryAt(t *testing.T) {
	t.Parallel()

	if _, err := removeSentinelBinaryAt("  "); err == nil {
		t.Fatal("removeSentinelBinaryAt() accepted an empty path")
	}

	path := filepath.Join(t.TempDir(), "sentinel")
	if err := os.WriteFile(path, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	removed, err := removeSentinelBinaryAt("  " + path + "  ")
	if err != nil {
		t.Fatalf("removeSentinelBinaryAt() error = %v", err)
	}
	if removed != path {
		t.Fatalf("removed path = %q, want %q", removed, path)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("binary still exists: %v", err)
	}
	if _, err := removeSentinelBinaryAt(path); !os.IsNotExist(err) {
		t.Fatalf("second removal error = %v, want not-exist", err)
	}
}

func TestRemoveShellCompletions(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("avoids touching the system completion path as root")
	}
	home := t.TempDir()
	configHome := filepath.Join(home, "xdg")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	paths := []string{
		filepath.Join(home, ".local", "share", "bash-completion", "completions", "sentinel"),
		filepath.Join(home, ".local", "share", "zsh", "site-functions", "_sentinel"),
		filepath.Join(configHome, "fish", "completions", "sentinel.fish"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("completion"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	removed := removeShellCompletions()
	for _, path := range paths {
		if !slices.Contains(removed, path) {
			t.Errorf("removed paths missing %q: %v", path, removed)
		}
	}
}

func TestMigrateLegacyConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "legacy", "config.toml")
	target := filepath.Join(dir, "canonical", "config.toml")
	if err := os.MkdirAll(filepath.Dir(source), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("version = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyConfig(source, target); err != nil {
		t.Fatalf("migrateLegacyConfig() error = %v", err)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "version = 1\n" {
		t.Fatalf("target contents = %q", raw)
	}

	if err := os.WriteFile(source, []byte("changed = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyConfig(source, target); err != nil {
		t.Fatalf("existing target should be preserved: %v", err)
	}
	raw, err = os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "version = 1\n" {
		t.Fatalf("existing target was overwritten: %q", raw)
	}

	if err := migrateLegacyConfig(filepath.Join(dir, "missing.toml"), filepath.Join(dir, "unused", "config.toml")); err != nil {
		t.Fatalf("missing legacy config should be ignored: %v", err)
	}
}

func TestMigrateLegacyConfigFailures(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(source, []byte("version = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	blockingFile := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyConfig(source, filepath.Join(blockingFile, "config.toml")); err == nil || !strings.Contains(err.Error(), "stat canonical system config") {
		t.Fatalf("blocked target error = %v", err)
	}
	if err := migrateLegacyConfig(dir, filepath.Join(dir, "target", "config.toml")); err == nil || !strings.Contains(err.Error(), "read legacy system config") {
		t.Fatalf("directory source error = %v", err)
	}
}

func TestPrepareServiceConfigExplicitPath(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config", "sentinel.toml")
	t.Setenv("SENTINEL_CONFIG", configPath)
	t.Setenv("SENTINEL_DATA_DIR", "")

	resolved, dataDir, err := prepareServiceConfig("user", true)
	if err != nil {
		t.Fatalf("prepareServiceConfig() error = %v", err)
	}
	if resolved != configPath {
		t.Fatalf("config path = %q, want %q", resolved, configPath)
	}
	if dataDir == "" {
		t.Fatal("data directory is empty")
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config was not created: %v", err)
	}
}

func TestServiceConfigHelperErrors(t *testing.T) {
	if _, _, err := prepareServiceConfig("bogus", false); err == nil {
		t.Fatal("prepareServiceConfig() accepted an invalid scope")
	}

	origDeployments := installedDeploymentsFn
	t.Cleanup(func() { installedDeploymentsFn = origDeployments })
	wantErr := errors.New("discovery failed")
	installedDeploymentsFn = func() ([]daemon.Deployment, error) { return nil, wantErr }
	if _, _, err := prepareServiceConfig(daemon.ScopeUser, false); !errors.Is(err, wantErr) {
		t.Fatalf("prepareServiceConfig() error = %v, want %v", err, wantErr)
	}
	if _, err := validateServiceInstallBinary(daemon.ScopeUser, "/tmp/sentinel"); !errors.Is(err, wantErr) {
		t.Fatalf("validateServiceInstallBinary() error = %v, want %v", err, wantErr)
	}
}

func TestValidateServiceInstallBinaryKeepsManagedPath(t *testing.T) {
	origDeployments := installedDeploymentsFn
	t.Cleanup(func() { installedDeploymentsFn = origDeployments })
	installedDeploymentsFn = func() ([]daemon.Deployment, error) {
		return []daemon.Deployment{{
			Scope:      daemon.ScopeUser,
			BinaryPath: "/opt/sentinel/bin/sentinel",
		}}, nil
	}

	got, err := validateServiceInstallBinary(daemon.ScopeUser, "/opt/sentinel/bin/../bin/sentinel")
	if err != nil {
		t.Fatalf("validateServiceInstallBinary() error = %v", err)
	}
	if got != "/opt/sentinel/bin/sentinel" {
		t.Fatalf("binary path = %q", got)
	}
	if sameBinaryPath("", got) {
		t.Fatal("sameBinaryPath() matched an empty path")
	}
}

func TestPrepareServiceConfigWithoutCreatingFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "missing.toml")
	t.Setenv("SENTINEL_CONFIG", configPath)
	t.Setenv("SENTINEL_DATA_DIR", "")

	resolved, _, err := prepareServiceConfig(daemon.ScopeUser, false)
	if err != nil {
		t.Fatalf("prepareServiceConfig() error = %v", err)
	}
	if resolved != configPath {
		t.Fatalf("config path = %q, want %q", resolved, configPath)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("config file was unexpectedly created: %v", err)
	}
}

func TestRunServiceInstallCheck(t *testing.T) {
	stubUserServiceInstallContext(t)
	dir := t.TempDir()
	executable := filepath.Join(dir, "sentinel")
	if err := os.WriteFile(executable, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SENTINEL_CONFIG", filepath.Join(dir, "config.toml"))

	origInstall := installUserSvcFn
	t.Cleanup(func() { installUserSvcFn = origInstall })
	installUserSvcFn = func(_ daemon.InstallUserOptions) error {
		return errors.New("install must not run during --check")
	}

	var out, errOut bytes.Buffer
	code := Run([]string{"service", "install", "--check", "--exec", executable}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "service install check passed") {
		t.Fatalf("stdout = %s", out.String())
	}
}

func TestPreflightBinaryWrite(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sentinel")
	if err := os.WriteFile(path, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := preflightBinaryWrite(path); err != nil {
		t.Fatalf("preflightBinaryWrite() error = %v", err)
	}
	if err := preflightBinaryWrite(""); err == nil {
		t.Fatal("preflightBinaryWrite() accepted an empty path")
	}
	if err := preflightBinaryWrite(filepath.Dir(path)); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("directory error = %v", err)
	}
	if err := preflightBinaryWrite(filepath.Join(t.TempDir(), "missing")); err == nil || !strings.Contains(err.Error(), "access deployment binary") {
		t.Fatalf("missing binary error = %v", err)
	}
}
