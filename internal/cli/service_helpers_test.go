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
	home := t.TempDir()
	configHome := filepath.Join(home, "xdg")
	systemPath := filepath.Join(home, "system", "sentinel")
	paths := []string{
		filepath.Join(home, ".local", "share", "bash-completion", "completions", "sentinel"),
		filepath.Join(home, ".local", "share", "zsh", "site-functions", "_sentinel"),
		filepath.Join(configHome, "fish", "completions", "sentinel.fish"),
		systemPath,
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("completion"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	removed := removeShellCompletionsFrom(home, configHome, systemPath)
	for _, path := range paths {
		if !slices.Contains(removed, path) {
			t.Errorf("removed paths missing %q: %v", path, removed)
		}
	}
}

func TestRemoveShellCompletionsUsesDefaultConfigHome(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".config", "fish", "completions", "sentinel.fish")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("completion"), 0o600); err != nil {
		t.Fatal(err)
	}
	removed := removeShellCompletionsFrom(home, "", "")
	if !slices.Contains(removed, path) {
		t.Fatalf("removed paths missing %q: %v", path, removed)
	}
}

func TestRebaseManagedConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	legacy := daemon.ScopeLayout{
		DataDir: filepath.Join(dir, "legacy"),
		LogPath: filepath.Join(dir, "legacy", "logs", "sentinel.log"),
	}
	canonical := daemon.ScopeLayout{
		DataDir: filepath.Join(dir, "data"),
		LogPath: filepath.Join(dir, "log", "sentinel.log"),
	}
	raw := []byte("version = 1\n\n[storage]\n  path = \"" + filepath.Join(legacy.DataDir, "sentinel.db") + "\"\n\n[log]\n  path = \"" + legacy.LogPath + "\"\n")
	rebased, err := rebaseManagedConfig(raw, legacy, canonical)
	if err != nil {
		t.Fatalf("rebaseManagedConfig() error = %v", err)
	}
	for _, expected := range []string{
		filepath.Join(canonical.DataDir, "sentinel.db"),
		canonical.LogPath,
	} {
		if !strings.Contains(string(rebased), expected) {
			t.Fatalf("rebased config missing %q: %s", expected, rebased)
		}
	}
}

func TestRewriteTOMLPathLinePreservesLiteralAndInlineComment(t *testing.T) {
	t.Parallel()

	line := "  path = '/root/.sentinel/sentinel.db' # keep this\n"
	rewritten, changed, err := rewriteTOMLPathLine(line, "/root/.sentinel/sentinel.db", "/var/lib/sentinel/sentinel.db")
	if err != nil || !changed {
		t.Fatalf("rewriteTOMLPathLine() = %q, %t, %v", rewritten, changed, err)
	}
	if !strings.Contains(rewritten, `"/var/lib/sentinel/sentinel.db" # keep this`) {
		t.Fatalf("rewritten line = %q", rewritten)
	}
}

func TestCopyTreeAtomicSeparatesManagedFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "legacy")
	destination := filepath.Join(dir, "canonical")
	for path, contents := range map[string]string{
		"config.toml":        "config",
		"config.toml.bk":     "backup",
		"logs/sentinel.log":  "log",
		"sentinel.db":        "db",
		"updater/state.json": "state",
	} {
		fullPath := filepath.Join(source, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := copyTreeAtomic(source, destination, skipLegacyDataEntry); err != nil {
		t.Fatalf("copyTreeAtomic() error = %v", err)
	}
	for _, path := range []string{"sentinel.db", "updater/state.json"} {
		if _, err := os.Stat(filepath.Join(destination, path)); err != nil {
			t.Fatalf("migrated data %s: %v", path, err)
		}
	}
	for _, path := range []string{"config.toml", "config.toml.bk", "logs/sentinel.log"} {
		if _, err := os.Stat(filepath.Join(destination, path)); !os.IsNotExist(err) {
			t.Fatalf("separated path %s was copied into data dir", path)
		}
	}
}

func TestPlanDeploymentMigrationRejectsDivergentConfigs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	legacy := daemon.ScopeLayout{
		ConfigPath: filepath.Join(dir, "legacy", "config.toml"),
		DataDir:    filepath.Join(dir, "legacy"),
		LogPath:    filepath.Join(dir, "legacy", "logs", "sentinel.log"),
	}
	canonical := daemon.ScopeLayout{
		ConfigPath: filepath.Join(dir, "etc", "config.toml"),
		DataDir:    filepath.Join(dir, "lib"),
		LogPath:    filepath.Join(dir, "log", "sentinel.log"),
	}
	for path, contents := range map[string]string{
		legacy.ConfigPath:    "version = 1\n# desired edit\n",
		canonical.ConfigPath: "version = 1\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, err := planDeploymentMigrationWithLayouts(daemon.Deployment{
		Scope:      daemon.ScopeSystem,
		ConfigPath: canonical.ConfigPath,
		DataDir:    legacy.DataDir,
	}, legacy, canonical)
	if err == nil || !strings.Contains(err.Error(), "differs") || !strings.Contains(err.Error(), "reconcile") {
		t.Fatalf("migration conflict error = %v", err)
	}
}

func TestStageDeploymentMigrationSeparatesAndRollsBack(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	legacy := daemon.ScopeLayout{
		ConfigPath: filepath.Join(dir, "legacy", "config.toml"),
		DataDir:    filepath.Join(dir, "legacy"),
		LogPath:    filepath.Join(dir, "legacy", "logs", "sentinel.log"),
	}
	canonical := daemon.ScopeLayout{
		ConfigPath: filepath.Join(dir, "etc", "config.toml"),
		DataDir:    filepath.Join(dir, "lib"),
		LogPath:    filepath.Join(dir, "log", "sentinel.log"),
	}
	configRaw := []byte("version = 1\n\n[storage]\n  path = \"" + filepath.Join(legacy.DataDir, "sentinel.db") + "\"\n\n[log]\n  path = \"" + legacy.LogPath + "\"\n")
	for path, contents := range map[string][]byte{
		legacy.ConfigPath: configRaw,
		filepath.Join(legacy.DataDir, "config.toml.bk"): []byte("backup"),
		filepath.Join(legacy.DataDir, "sentinel.db"):    []byte("database"),
		legacy.LogPath: []byte("logs"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := planDeploymentMigrationWithLayouts(daemon.Deployment{
		Scope:      daemon.ScopeSystem,
		ConfigPath: legacy.ConfigPath,
		DataDir:    legacy.DataDir,
	}, legacy, canonical)
	if err != nil {
		t.Fatalf("planDeploymentMigrationWithLayouts() error = %v", err)
	}
	rollback, err := stageDeploymentMigration(plan)
	if err != nil {
		t.Fatalf("stageDeploymentMigration() error = %v", err)
	}
	for _, path := range []string{
		canonical.ConfigPath,
		filepath.Join(filepath.Dir(canonical.ConfigPath), "config.toml.bk"),
		filepath.Join(canonical.DataDir, "sentinel.db"),
		canonical.LogPath,
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("staged path %s: %v", path, err)
		}
	}
	rebased, err := os.ReadFile(canonical.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rebased), legacy.DataDir) || !strings.Contains(string(rebased), canonical.LogPath) {
		t.Fatalf("canonical config was not rebased: %s", rebased)
	}
	if err := rollback.rollback(); err != nil {
		t.Fatalf("rollback() error = %v", err)
	}
	for _, path := range []string{canonical.ConfigPath, canonical.DataDir, filepath.Dir(canonical.LogPath)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("rollback left %s", path)
		}
	}
	if _, err := os.Stat(filepath.Join(legacy.DataDir, "sentinel.db")); err != nil {
		t.Fatalf("rollback damaged legacy source: %v", err)
	}
}

func TestRunServiceMigrateStopsCopiesInstallsAndRestores(t *testing.T) {
	dir := t.TempDir()
	legacy := daemon.ScopeLayout{
		ConfigPath: filepath.Join(dir, "legacy", "config.toml"),
		DataDir:    filepath.Join(dir, "legacy"),
		LogPath:    filepath.Join(dir, "legacy", "logs", "sentinel.log"),
	}
	canonical := daemon.ScopeLayout{
		ConfigPath: filepath.Join(dir, "etc", "config.toml"),
		DataDir:    filepath.Join(dir, "lib"),
		LogPath:    filepath.Join(dir, "log", "sentinel.log"),
	}
	for path, contents := range map[string]string{
		legacy.ConfigPath: "version = 1\n",
		filepath.Join(legacy.DataDir, "sentinel.db"): "database",
		legacy.LogPath: "logs",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	deployment := daemon.Deployment{
		Scope:      daemon.ScopeSystem,
		BinaryPath: "/usr/local/bin/sentinel",
		ConfigPath: "/etc/sentinel/config.toml",
		DataDir:    "/root/.sentinel",
	}
	migration := deploymentMigration{
		deployment: daemon.Deployment{
			Scope:      daemon.ScopeSystem,
			BinaryPath: deployment.BinaryPath,
			ConfigPath: legacy.ConfigPath,
			DataDir:    legacy.DataDir,
		},
		legacy:    legacy,
		canonical: canonical,
		configRaw: []byte("version = 1\n"),
	}

	origResolve := resolveDeploymentFn
	origAccess := requireScopeAccessFn
	origPlan := planDeploymentMigrationFn
	origStatus := serviceStatusFn
	origPause := pauseAutoUpdateFn
	origResume := resumeAutoUpdateFn
	origControl := controlScopedServiceFn
	origInstall := installUserSvcFn
	t.Cleanup(func() {
		resolveDeploymentFn = origResolve
		requireScopeAccessFn = origAccess
		planDeploymentMigrationFn = origPlan
		serviceStatusFn = origStatus
		pauseAutoUpdateFn = origPause
		resumeAutoUpdateFn = origResume
		controlScopedServiceFn = origControl
		installUserSvcFn = origInstall
	})
	resolveDeploymentFn = func(string) (daemon.Deployment, error) { return deployment, nil }
	requireScopeAccessFn = func(string) error { return nil }
	planDeploymentMigrationFn = func(daemon.Deployment) (deploymentMigration, error) { return migration, nil }
	serviceStatusFn = func() ([]daemon.ScopedServiceStatus, error) {
		return []daemon.ScopedServiceStatus{{
			Deployment: deployment,
			UserServiceStatus: daemon.UserServiceStatus{
				SystemctlAvailable: true,
				EnabledState:       stateEnabled,
				ActiveState:        stateActive,
			},
		}}, nil
	}
	pauseCalls := 0
	resumeCalls := 0
	pauseAutoUpdateFn = func(string) (bool, error) {
		pauseCalls++
		return true, nil
	}
	resumeAutoUpdateFn = func(_ string, active bool) error {
		if !active {
			t.Fatal("resume did not preserve active autoupdate state")
		}
		resumeCalls++
		return nil
	}
	var controls []string
	controlScopedServiceFn = func(action, _ string) error {
		controls = append(controls, action)
		return nil
	}
	var installed daemon.InstallUserOptions
	installUserSvcFn = func(opts daemon.InstallUserOptions) error {
		installed = opts
		return nil
	}

	var out, errOut bytes.Buffer
	err := runServiceMigrate(&App{Stdout: &out, Stderr: &errOut}, daemon.ScopeSystem)
	if err != nil {
		t.Fatalf("runServiceMigrate() error = %v", err)
	}
	if pauseCalls != 1 || resumeCalls != 1 || !slices.Equal(controls, []string{"stop"}) {
		t.Fatalf("pause=%d resume=%d controls=%v", pauseCalls, resumeCalls, controls)
	}
	if installed.ConfigPath != canonical.ConfigPath || installed.DataDir != canonical.DataDir || !installed.Enable || !installed.Start {
		t.Fatalf("install options = %+v", installed)
	}
	if _, err := os.Stat(legacy.DataDir); !os.IsNotExist(err) {
		t.Fatalf("legacy data still exists: %v", err)
	}
	for _, path := range []string{canonical.ConfigPath, filepath.Join(canonical.DataDir, "sentinel.db"), canonical.LogPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("canonical path %s: %v", path, err)
		}
	}
	if !strings.Contains(out.String(), "deployment migrated successfully") {
		t.Fatalf("stdout = %s", out.String())
	}
}

func TestRunServiceMigrateCanonicalNoOp(t *testing.T) {
	deployment := daemon.Deployment{
		Scope:      daemon.ScopeSystem,
		ConfigPath: "/etc/sentinel/config.toml",
		DataDir:    "/var/lib/sentinel",
	}
	origResolve := resolveDeploymentFn
	origAccess := requireScopeAccessFn
	origPlan := planDeploymentMigrationFn
	origPause := pauseAutoUpdateFn
	t.Cleanup(func() {
		resolveDeploymentFn = origResolve
		requireScopeAccessFn = origAccess
		planDeploymentMigrationFn = origPlan
		pauseAutoUpdateFn = origPause
	})
	resolveDeploymentFn = func(string) (daemon.Deployment, error) { return deployment, nil }
	requireScopeAccessFn = func(string) error { return nil }
	planDeploymentMigrationFn = func(daemon.Deployment) (deploymentMigration, error) {
		layout, err := daemon.LayoutForScope(daemon.ScopeSystem)
		return deploymentMigration{deployment: deployment, canonical: layout}, err
	}
	pauseAutoUpdateFn = func(string) (bool, error) {
		t.Fatal("canonical migration paused autoupdate")
		return false, nil
	}
	var out, errOut bytes.Buffer
	if err := runServiceMigrate(&App{Stdout: &out, Stderr: &errOut}, optionAuto); err != nil {
		t.Fatalf("runServiceMigrate() error = %v", err)
	}
	if !strings.Contains(out.String(), "already uses canonical paths") || !strings.Contains(out.String(), "/var/log/sentinel/sentinel.log") {
		t.Fatalf("stdout = %s", out.String())
	}
	planned, err := planDeploymentMigration(deployment)
	if err != nil || planned.canonical.ConfigPath != deployment.ConfigPath {
		t.Fatalf("planDeploymentMigration() = %+v, %v", planned, err)
	}
}

func TestRunServiceMigrateRestartsSourceWhenStagingFails(t *testing.T) {
	dir := t.TempDir()
	blockingFile := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	deployment := daemon.Deployment{
		Scope:      daemon.ScopeSystem,
		BinaryPath: "/usr/local/bin/sentinel",
		ConfigPath: "/etc/sentinel/config.toml",
		DataDir:    "/root/.sentinel",
	}
	migration := deploymentMigration{
		deployment: daemon.Deployment{Scope: daemon.ScopeSystem, DataDir: filepath.Join(dir, "legacy")},
		legacy:     daemon.ScopeLayout{DataDir: filepath.Join(dir, "legacy")},
		canonical: daemon.ScopeLayout{
			ConfigPath: filepath.Join(blockingFile, "config.toml"),
			DataDir:    filepath.Join(dir, "lib"),
			LogPath:    filepath.Join(dir, "log", "sentinel.log"),
		},
		configRaw: []byte("version = 1\n"),
	}
	origResolve := resolveDeploymentFn
	origAccess := requireScopeAccessFn
	origPlan := planDeploymentMigrationFn
	origStatus := serviceStatusFn
	origPause := pauseAutoUpdateFn
	origResume := resumeAutoUpdateFn
	origControl := controlScopedServiceFn
	t.Cleanup(func() {
		resolveDeploymentFn = origResolve
		requireScopeAccessFn = origAccess
		planDeploymentMigrationFn = origPlan
		serviceStatusFn = origStatus
		pauseAutoUpdateFn = origPause
		resumeAutoUpdateFn = origResume
		controlScopedServiceFn = origControl
	})
	resolveDeploymentFn = func(string) (daemon.Deployment, error) { return deployment, nil }
	requireScopeAccessFn = func(string) error { return nil }
	planDeploymentMigrationFn = func(daemon.Deployment) (deploymentMigration, error) { return migration, nil }
	serviceStatusFn = func() ([]daemon.ScopedServiceStatus, error) {
		return []daemon.ScopedServiceStatus{{
			Deployment: deployment,
			UserServiceStatus: daemon.UserServiceStatus{
				SystemctlAvailable: true,
				ActiveState:        stateActive,
			},
		}}, nil
	}
	pauseAutoUpdateFn = func(string) (bool, error) { return false, nil }
	resumeAutoUpdateFn = func(string, bool) error { return nil }
	var controls []string
	controlScopedServiceFn = func(action, _ string) error {
		controls = append(controls, action)
		return nil
	}
	var out, errOut bytes.Buffer
	err := runServiceMigrate(&App{Stdout: &out, Stderr: &errOut}, daemon.ScopeSystem)
	if err == nil || !strings.Contains(err.Error(), "canonical config") {
		t.Fatalf("runServiceMigrate() error = %v", err)
	}
	if !slices.Equal(controls, []string{"stop", "start"}) {
		t.Fatalf("service controls = %v", controls)
	}
}

func TestMigrationHelperFailuresAndSafety(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if _, _, err := rewriteTOMLPathLine("path = not-quoted\n", "/old", "/new"); err == nil {
		t.Fatal("rewriteTOMLPathLine() accepted malformed TOML string")
	}
	nonempty := filepath.Join(dir, "nonempty")
	if err := os.MkdirAll(nonempty, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nonempty, "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := requireEmptyDestination(nonempty, filepath.Join(dir, "source")); err == nil {
		t.Fatal("requireEmptyDestination() accepted nonempty directory")
	}

	source := filepath.Join(dir, "source")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(source, "link")); err != nil {
		t.Fatal(err)
	}
	if err := copyTreeAtomic(source, filepath.Join(dir, "target"), nil); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("copyTreeAtomic() symlink error = %v", err)
	}
	missingTarget := filepath.Join(dir, "missing-target")
	if err := copyTreeAtomic(filepath.Join(dir, "missing-source"), missingTarget, nil); err != nil {
		t.Fatalf("copyTreeAtomic(missing source) error = %v", err)
	}
	if info, err := os.Stat(missingTarget); err != nil || !info.IsDir() {
		t.Fatalf("missing source destination = %v, %v", info, err)
	}

	legacy := daemon.ScopeLayout{DataDir: filepath.Join(dir, "legacy")}
	canonical := daemon.ScopeLayout{DataDir: filepath.Join(dir, "canonical")}
	migration := deploymentMigration{
		deployment: daemon.Deployment{DataDir: filepath.Join(dir, "unexpected")},
		legacy:     legacy,
		canonical:  canonical,
	}
	if err := removeLegacyDeploymentData(migration); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("removeLegacyDeploymentData() error = %v", err)
	}
	if !pathInside(filepath.Join(legacy.DataDir, "sentinel.db"), legacy.DataDir) || pathInside(canonical.DataDir, legacy.DataDir) {
		t.Fatal("pathInside() did not bound the legacy directory")
	}
}

func TestMigrationRollbackRestoresExistingConfigAndArtifacts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "etc", "config.toml")
	dataDir := filepath.Join(dir, "lib")
	logDir := filepath.Join(dir, "log")
	backupPath := filepath.Join(dir, "etc", "config.toml.bk")
	for path, contents := range map[string]string{
		configPath:                            "new",
		filepath.Join(dataDir, "sentinel.db"): "db",
		filepath.Join(logDir, "sentinel.log"): "log",
		backupPath:                            "backup",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rollback := migrationRollback{
		configPath:     configPath,
		previousConfig: []byte("old"),
		configExisted:  true,
		createdData:    dataDir,
		createdLogs:    logDir,
		createdBackups: []string{backupPath},
	}
	if err := rollback.rollback(); err != nil {
		t.Fatalf("rollback() error = %v", err)
	}
	raw, err := os.ReadFile(configPath)
	if err != nil || string(raw) != "old" {
		t.Fatalf("restored config = %q, %v", raw, err)
	}
	for _, path := range []string{dataDir, logDir, backupPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("rollback left %s", path)
		}
	}
}

func TestCopyLegacyConfigBackupsRejectsExistingTarget(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "legacy")
	target := filepath.Join(dir, "etc")
	for _, path := range []string{
		filepath.Join(source, "config.toml.bk"),
		filepath.Join(target, "config.toml.bk"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("config"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := copyLegacyConfigBackups(source, filepath.Join(target, "config.toml")); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("copyLegacyConfigBackups() error = %v", err)
	}
}

func TestMigrationServiceStateDiagnostics(t *testing.T) {
	origStatus := serviceStatusFn
	t.Cleanup(func() { serviceStatusFn = origStatus })

	wantErr := errors.New("status failed")
	serviceStatusFn = func() ([]daemon.ScopedServiceStatus, error) { return nil, wantErr }
	if _, _, err := migrationServiceState(daemon.ScopeSystem); !errors.Is(err, wantErr) {
		t.Fatalf("status error = %v", err)
	}
	serviceStatusFn = func() ([]daemon.ScopedServiceStatus, error) {
		return []daemon.ScopedServiceStatus{{
			Deployment:        daemon.Deployment{Scope: daemon.ScopeSystem},
			UserServiceStatus: daemon.UserServiceStatus{},
		}}, nil
	}
	if _, _, err := migrationServiceState(daemon.ScopeSystem); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("manager unavailable error = %v", err)
	}
	serviceStatusFn = func() ([]daemon.ScopedServiceStatus, error) { return nil, nil }
	if _, _, err := migrationServiceState(daemon.ScopeSystem); err == nil || !strings.Contains(err.Error(), "no status") {
		t.Fatalf("missing status error = %v", err)
	}
}

func TestPlanDeploymentMigrationValidationErrors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	legacy := daemon.ScopeLayout{
		ConfigPath: filepath.Join(dir, "legacy", "config.toml"),
		DataDir:    filepath.Join(dir, "legacy"),
		LogPath:    filepath.Join(dir, "legacy", "logs", "sentinel.log"),
	}
	canonical := daemon.ScopeLayout{
		ConfigPath: filepath.Join(dir, "etc", "config.toml"),
		DataDir:    filepath.Join(dir, "lib"),
		LogPath:    filepath.Join(dir, "log", "sentinel.log"),
	}
	if _, err := planDeploymentMigrationWithLayouts(daemon.Deployment{Scope: daemon.ScopeUser}, legacy, canonical); err == nil {
		t.Fatal("user deployment was accepted for historical migration")
	}
	if _, err := planDeploymentMigrationWithLayouts(daemon.Deployment{
		Scope:      daemon.ScopeSystem,
		ConfigPath: filepath.Join(dir, "custom.toml"),
		DataDir:    legacy.DataDir,
	}, legacy, canonical); err == nil || !strings.Contains(err.Error(), "unsupported system config") {
		t.Fatalf("custom config error = %v", err)
	}
	if _, err := planDeploymentMigrationWithLayouts(daemon.Deployment{
		Scope:      daemon.ScopeSystem,
		ConfigPath: legacy.ConfigPath,
		DataDir:    filepath.Join(dir, "custom-data"),
	}, legacy, canonical); err == nil || !strings.Contains(err.Error(), "unsupported system data") {
		t.Fatalf("custom data error = %v", err)
	}
	if _, err := planDeploymentMigrationWithLayouts(daemon.Deployment{
		Scope:      daemon.ScopeSystem,
		ConfigPath: legacy.ConfigPath,
		DataDir:    legacy.DataDir,
	}, legacy, canonical); err == nil || !strings.Contains(err.Error(), "read active config") {
		t.Fatalf("missing config error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(legacy.ConfigPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy.ConfigPath, []byte("version = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(canonical.DataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonical.DataDir, "occupied"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := planDeploymentMigrationWithLayouts(daemon.Deployment{
		Scope:      daemon.ScopeSystem,
		ConfigPath: legacy.ConfigPath,
		DataDir:    legacy.DataDir,
	}, legacy, canonical); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("occupied destination error = %v", err)
	}
}

func TestPrepareServiceConfigRejectsNoncanonicalExplicitPath(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config", "sentinel.toml")
	t.Setenv("SENTINEL_CONFIG", configPath)
	t.Setenv("SENTINEL_DATA_DIR", "")

	if _, _, err := prepareServiceConfig("user", true); err == nil || !strings.Contains(err.Error(), "managed user deployments use") {
		t.Fatalf("prepareServiceConfig() error = %v", err)
	}
}

func TestResolveConfigTargetUsesInstalledSystemDeployment(t *testing.T) {
	t.Setenv("SENTINEL_CONFIG", "")
	t.Setenv("SENTINEL_DATA_DIR", "")
	origResolve := resolveDeploymentFn
	t.Cleanup(func() { resolveDeploymentFn = origResolve })
	resolveDeploymentFn = func(string) (daemon.Deployment, error) {
		return daemon.Deployment{
			Scope:      daemon.ScopeSystem,
			ConfigPath: "/etc/sentinel/config.toml",
			DataDir:    "/var/lib/sentinel",
		}, nil
	}
	target, err := resolveConfigTarget(optionAuto)
	if err != nil {
		t.Fatalf("resolveConfigTarget() error = %v", err)
	}
	if target.path != "/etc/sentinel/config.toml" || target.dataDir != "/var/lib/sentinel" || target.logPath != "/var/log/sentinel/sentinel.log" {
		t.Fatalf("target = %+v", target)
	}
}

func TestResolveConfigTargetUsesEffectiveHybridConfig(t *testing.T) {
	t.Setenv("SENTINEL_CONFIG", "")
	t.Setenv("SENTINEL_DATA_DIR", "")
	origResolve := resolveDeploymentFn
	t.Cleanup(func() { resolveDeploymentFn = origResolve })
	resolveDeploymentFn = func(string) (daemon.Deployment, error) {
		return daemon.Deployment{
			Scope:      daemon.ScopeSystem,
			ConfigPath: "/etc/sentinel/config.toml",
			DataDir:    "/root/.sentinel",
		}, nil
	}
	target, err := resolveConfigTarget(optionAuto)
	if err != nil {
		t.Fatalf("resolveConfigTarget() error = %v", err)
	}
	if target.path != "/etc/sentinel/config.toml" || target.dataDir != "/root/.sentinel" || target.logPath != "/root/.sentinel/logs/sentinel.log" {
		t.Fatalf("target = %+v", target)
	}
}

func TestResolveConfigTargetWithoutManagedDeployment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SENTINEL_CONFIG", "")
	t.Setenv("SENTINEL_DATA_DIR", "")
	origResolve := resolveDeploymentFn
	origAccess := requireScopeAccessFn
	t.Cleanup(func() {
		resolveDeploymentFn = origResolve
		requireScopeAccessFn = origAccess
	})
	resolveDeploymentFn = func(string) (daemon.Deployment, error) {
		return daemon.Deployment{}, daemon.ErrNoServiceInstalled
	}
	requireScopeAccessFn = func(string) error { return nil }

	automatic, err := resolveConfigTarget(optionAuto)
	if err != nil {
		t.Fatalf("resolveConfigTarget(auto) error = %v", err)
	}
	wantUserPath := filepath.Join(home, ".sentinel", "config.toml")
	if automatic.path != wantUserPath {
		t.Fatalf("automatic path = %q, want %q", automatic.path, wantUserPath)
	}
	user, err := resolveConfigTarget(optionUser)
	if err != nil || user.path != wantUserPath {
		t.Fatalf("resolveConfigTarget(user) = %+v, %v", user, err)
	}
	if _, err := resolveConfigTarget("bogus"); err == nil || !strings.Contains(err.Error(), "invalid scope") {
		t.Fatalf("invalid scope error = %v", err)
	}

	wantErr := errors.New("scope denied")
	requireScopeAccessFn = func(string) error { return wantErr }
	if _, err := resolveConfigTarget(optionSystem); !errors.Is(err, wantErr) {
		t.Fatalf("scope access error = %v", err)
	}
	resolveDeploymentFn = func(string) (daemon.Deployment, error) { return daemon.Deployment{}, wantErr }
	if _, err := resolveConfigTarget(optionAuto); !errors.Is(err, wantErr) {
		t.Fatalf("deployment error = %v", err)
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
	home := t.TempDir()
	configPath := filepath.Join(home, ".sentinel", "config.toml")
	t.Setenv("HOME", home)
	t.Setenv("SENTINEL_CONFIG", "")
	t.Setenv("SENTINEL_DATA_DIR", "")
	origDeployments := installedDeploymentsFn
	t.Cleanup(func() { installedDeploymentsFn = origDeployments })
	installedDeploymentsFn = func() ([]daemon.Deployment, error) { return nil, nil }

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

func TestPrepareServiceConfigRequiresMigrationForInstalledLegacyDeployment(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	t.Setenv("SENTINEL_CONFIG", "")
	if err := os.WriteFile(configPath, []byte("version = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	origDeployments := installedDeploymentsFn
	t.Cleanup(func() { installedDeploymentsFn = origDeployments })
	installedDeploymentsFn = func() ([]daemon.Deployment, error) {
		return []daemon.Deployment{{
			Scope:      daemon.ScopeSystem,
			ConfigPath: configPath,
			DataDir:    dir,
		}}, nil
	}

	_, _, err := prepareServiceConfig(daemon.ScopeSystem, false)
	if err == nil || !strings.Contains(err.Error(), "service migrate --scope system") {
		t.Fatalf("prepareServiceConfig() error = %v", err)
	}
}

func TestValidateServiceInstallBinaryRecognizesSameFile(t *testing.T) {
	dir := t.TempDir()
	managedPath := filepath.Join(dir, "sentinel")
	requestedPath := filepath.Join(dir, "sentinel-link")
	if err := os.WriteFile(managedPath, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(managedPath, requestedPath); err != nil {
		t.Fatal(err)
	}

	origDeployments := installedDeploymentsFn
	t.Cleanup(func() { installedDeploymentsFn = origDeployments })
	installedDeploymentsFn = func() ([]daemon.Deployment, error) {
		return []daemon.Deployment{{Scope: daemon.ScopeUser, BinaryPath: managedPath}}, nil
	}
	got, err := validateServiceInstallBinary(daemon.ScopeUser, requestedPath)
	if err != nil || got != managedPath {
		t.Fatalf("binary = %q, error = %v", got, err)
	}
}

func TestRunServiceInstallCheck(t *testing.T) {
	stubUserServiceInstallContext(t)
	dir := t.TempDir()
	executable := filepath.Join(dir, "sentinel")
	if err := os.WriteFile(executable, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SENTINEL_CONFIG", "")

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
