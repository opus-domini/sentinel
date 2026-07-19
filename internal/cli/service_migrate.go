package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/daemon"
)

type deploymentMigration struct {
	deployment daemon.Deployment
	legacy     daemon.ScopeLayout
	canonical  daemon.ScopeLayout
	configRaw  []byte
}

type migrationRollback struct {
	configPath     string
	previousConfig []byte
	configExisted  bool
	createdData    string
	createdLogs    string
	createdBackups []string
}

var planDeploymentMigrationFn = planDeploymentMigration

func newServiceMigrateCmd(app *App) *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate a legacy deployment to canonical system paths",
		Long: "Move a legacy managed deployment to the canonical config, data and log paths.\n\n" +
			"The service is stopped while SQLite and runtime state are copied. If active and\n" +
			"legacy configs differ, migration stops without choosing one silently.",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServiceMigrate(app, scope)
		},
	}
	cmd.Flags().StringVar(&scope, "scope", optionAuto, "target deployment: auto|user|system")
	return cmd
}

func runServiceMigrate(app *App, scopeRaw string) error {
	deployment, err := resolveDeploymentFn(scopeRaw)
	if err != nil {
		return failf("service migrate failed: %w", err)
	}
	if err := requireScopeAccessFn(deployment.Scope); err != nil {
		return failf("service migrate failed: %w", err)
	}
	migration, err := planDeploymentMigrationFn(deployment)
	if err != nil {
		return failf("service migrate failed: %w", err)
	}
	canonical, err := daemon.HasCanonicalPaths(deployment)
	if err != nil {
		return failf("service migrate failed: %w", err)
	}
	if canonical {
		writeln(app.Stdout, "deployment already uses canonical paths")
		printMigrationLayout(app, migration.canonical)
		return nil
	}

	enabled, active, err := migrationServiceState(deployment.Scope)
	if err != nil {
		return failf("service migrate failed: %w", err)
	}
	autoUpdateActive, err := pauseAutoUpdateFn(deployment.Scope)
	if err != nil {
		return failf("service migrate failed: pause autoupdate: %w", err)
	}
	if active {
		if err := controlScopedServiceFn("stop", deployment.Scope); err != nil {
			restoreErr := restoreMigrationRuntime(deployment.Scope, active, autoUpdateActive)
			if restoreErr != nil {
				return failf("service migrate failed: stop %s service: %w; restore failed: %w", deployment.Scope, err, restoreErr)
			}
			return failf("service migrate failed: stop %s service: %w", deployment.Scope, err)
		}
	}

	rollback, err := stageDeploymentMigration(migration)
	if err != nil {
		if restoreErr := restoreMigrationRuntime(deployment.Scope, active, autoUpdateActive); restoreErr != nil {
			return failf("service migrate failed: %w; restore failed: %w", err, restoreErr)
		}
		return failf("service migrate failed: %w", err)
	}
	rollbackAndRestart := func(cause error) error {
		rollbackErr := rollback.rollback()
		restoreErr := restoreMigrationRuntime(deployment.Scope, active, autoUpdateActive)
		if rollbackErr != nil {
			cause = fmt.Errorf("%w; rollback failed: %w", cause, rollbackErr)
		}
		if restoreErr != nil {
			return fmt.Errorf("%w; restore failed: %w", cause, restoreErr)
		}
		return cause
	}

	cfg, _, err := config.LoadPathForDeployment(
		migration.canonical.ConfigPath,
		migration.canonical.DataDir,
		migration.canonical.LogPath,
	)
	if err != nil {
		return failf("service migrate failed: %w", rollbackAndRestart(fmt.Errorf("validate migrated config: %w", err)))
	}
	if pathInside(cfg.Storage.Path, migration.legacy.DataDir) || pathInside(cfg.Log.Path, migration.legacy.DataDir) {
		return failf("service migrate failed: %w", rollbackAndRestart(fmt.Errorf(
			"config still references the legacy data directory %s; move custom storage/log paths before migrating",
			migration.legacy.DataDir,
		)))
	}
	if err := config.EnsureDirs(cfg); err != nil {
		return failf("service migrate failed: %w", rollbackAndRestart(fmt.Errorf("create canonical runtime directories: %w", err)))
	}
	if err := installUserSvcFn(daemon.InstallUserOptions{
		ExecPath:   deployment.BinaryPath,
		Scope:      deployment.Scope,
		ConfigPath: migration.canonical.ConfigPath,
		DataDir:    migration.canonical.DataDir,
		Enable:     enabled,
		Start:      active,
	}); err != nil {
		return failf("service migrate failed: %w", rollbackAndRestart(fmt.Errorf("install canonical service definition: %w", err)))
	}

	if err := removeLegacyDeploymentData(migration); err != nil {
		if resumeErr := resumeAutoUpdateFn(deployment.Scope, autoUpdateActive); resumeErr != nil {
			return failf(
				"service migrated and restarted, but legacy cleanup failed: %w; restoring autoupdate also failed: %w; canonical data is active at %s",
				err,
				resumeErr,
				migration.canonical.DataDir,
			)
		}
		return failf(
			"service migrated and restarted, but legacy cleanup failed: %w; canonical data is active at %s",
			err,
			migration.canonical.DataDir,
		)
	}
	if err := resumeAutoUpdateFn(deployment.Scope, autoUpdateActive); err != nil {
		return failf("service migrated successfully, but restoring autoupdate failed: %w", err)
	}
	printNotice(app.Stdout, "deployment migrated successfully")
	printMigrationLayout(app, migration.canonical)
	return nil
}

func printMigrationLayout(app *App, layout daemon.ScopeLayout) {
	printRows(app.Stdout, []outputRow{
		{Key: "config", Value: layout.ConfigPath},
		{Key: "data dir", Value: layout.DataDir},
		{Key: "log", Value: layout.LogPath},
	})
}

func planDeploymentMigration(deployment daemon.Deployment) (deploymentMigration, error) {
	canonical, err := daemon.LayoutForScope(deployment.Scope)
	if err != nil {
		return deploymentMigration{}, err
	}
	legacy, err := daemon.LegacyLayoutForScope(deployment.Scope)
	if err != nil {
		return deploymentMigration{}, err
	}
	canonicalPaths, err := daemon.HasCanonicalPaths(deployment)
	if err != nil {
		return deploymentMigration{}, err
	}
	if canonicalPaths {
		return deploymentMigration{deployment: deployment, legacy: legacy, canonical: canonical}, nil
	}
	return planDeploymentMigrationWithLayouts(deployment, legacy, canonical)
}

func planDeploymentMigrationWithLayouts(
	deployment daemon.Deployment,
	legacy daemon.ScopeLayout,
	canonical daemon.ScopeLayout,
) (deploymentMigration, error) {
	if deployment.Scope != daemon.ScopeSystem {
		return deploymentMigration{}, errors.New("only historical system deployments require filesystem migration")
	}
	if !samePath(deployment.ConfigPath, legacy.ConfigPath) && !samePath(deployment.ConfigPath, canonical.ConfigPath) {
		return deploymentMigration{}, fmt.Errorf("unsupported system config path %s", deployment.ConfigPath)
	}
	if !samePath(deployment.DataDir, legacy.DataDir) && !samePath(deployment.DataDir, canonical.DataDir) {
		return deploymentMigration{}, fmt.Errorf("unsupported system data directory %s", deployment.DataDir)
	}

	activeRaw, err := os.ReadFile(deployment.ConfigPath)
	if err != nil {
		return deploymentMigration{}, fmt.Errorf("read active config %s: %w", deployment.ConfigPath, err)
	}
	for _, candidate := range []string{legacy.ConfigPath, canonical.ConfigPath} {
		if samePath(candidate, deployment.ConfigPath) {
			continue
		}
		raw, readErr := os.ReadFile(candidate) //nolint:gosec // fixed legacy/canonical path.
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return deploymentMigration{}, fmt.Errorf("read alternate config %s: %w", candidate, readErr)
		}
		if !bytes.Equal(activeRaw, raw) {
			return deploymentMigration{}, fmt.Errorf(
				"active config %s differs from %s; reconcile the files explicitly and rerun migration",
				deployment.ConfigPath,
				candidate,
			)
		}
	}

	rebased, err := rebaseManagedConfig(activeRaw, legacy, canonical)
	if err != nil {
		return deploymentMigration{}, err
	}
	if err := requireEmptyDestination(canonical.DataDir, deployment.DataDir); err != nil {
		return deploymentMigration{}, err
	}
	if err := requireEmptyDestination(filepath.Dir(canonical.LogPath), filepath.Dir(legacy.LogPath)); err != nil {
		return deploymentMigration{}, err
	}
	return deploymentMigration{
		deployment: deployment,
		legacy:     legacy,
		canonical:  canonical,
		configRaw:  rebased,
	}, nil
}

func migrationServiceState(scope string) (enabled, active bool, err error) {
	report, err := serviceStatusFn()
	if err != nil {
		return false, false, err
	}
	for _, status := range report {
		if status.Scope != scope {
			continue
		}
		if !status.SystemctlAvailable {
			return false, false, errors.New("service manager status is unavailable")
		}
		enabled := status.EnabledState == stateEnabled || status.EnabledState == "enabled-runtime" ||
			status.EnabledState == "linked" || status.EnabledState == "linked-runtime" || status.EnabledState == "alias"
		return enabled, status.ActiveState == stateActive || status.ActiveState == stateRunning, nil
	}
	return false, false, fmt.Errorf("no status found for %s deployment", scope)
}

func stageDeploymentMigration(migration deploymentMigration) (migrationRollback, error) {
	rollback := migrationRollback{configPath: migration.canonical.ConfigPath}
	if raw, err := os.ReadFile(migration.canonical.ConfigPath); err == nil {
		rollback.configExisted = true
		rollback.previousConfig = raw
	} else if !errors.Is(err, os.ErrNotExist) {
		return rollback, fmt.Errorf("read canonical config: %w", err)
	}
	if err := writeFileAtomic(migration.canonical.ConfigPath, migration.configRaw, 0o600); err != nil {
		return rollback, fmt.Errorf("write canonical config: %w", err)
	}

	if !samePath(migration.deployment.DataDir, migration.canonical.DataDir) {
		if err := copyTreeAtomic(migration.deployment.DataDir, migration.canonical.DataDir, skipLegacyDataEntry); err != nil {
			_ = rollback.rollback()
			return rollback, fmt.Errorf("copy runtime data: %w", err)
		}
		rollback.createdData = migration.canonical.DataDir
	}
	legacyLogs := filepath.Dir(migration.legacy.LogPath)
	canonicalLogs := filepath.Dir(migration.canonical.LogPath)
	if !samePath(legacyLogs, canonicalLogs) {
		if _, err := os.Stat(legacyLogs); err == nil {
			if err := copyTreeAtomic(legacyLogs, canonicalLogs, nil); err != nil {
				_ = rollback.rollback()
				return rollback, fmt.Errorf("copy logs: %w", err)
			}
			rollback.createdLogs = canonicalLogs
		} else if !errors.Is(err, os.ErrNotExist) {
			_ = rollback.rollback()
			return rollback, fmt.Errorf("stat legacy logs: %w", err)
		}
	}
	backups, err := copyLegacyConfigBackups(migration.legacy.DataDir, migration.canonical.ConfigPath)
	rollback.createdBackups = backups
	if err != nil {
		_ = rollback.rollback()
		return rollback, err
	}
	return rollback, nil
}

func (rollback migrationRollback) rollback() error {
	var errs []error
	for _, path := range rollback.createdBackups {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	for _, path := range []string{rollback.createdLogs, rollback.createdData} {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			errs = append(errs, err)
		}
	}
	if rollback.configExisted {
		if err := writeFileAtomic(rollback.configPath, rollback.previousConfig, 0o600); err != nil {
			errs = append(errs, err)
		}
	} else if err := os.Remove(rollback.configPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func restoreMigrationRuntime(scope string, serviceActive, autoUpdateActive bool) error {
	var errs []error
	if serviceActive {
		if err := controlScopedServiceFn("start", scope); err != nil {
			errs = append(errs, fmt.Errorf("restart service: %w", err))
		}
	}
	if err := resumeAutoUpdateFn(scope, autoUpdateActive); err != nil {
		errs = append(errs, fmt.Errorf("restore autoupdate: %w", err))
	}
	return errors.Join(errs...)
}

func removeLegacyDeploymentData(migration deploymentMigration) error {
	if samePath(migration.legacy.DataDir, migration.canonical.DataDir) {
		return nil
	}
	if !samePath(migration.deployment.DataDir, migration.legacy.DataDir) {
		return fmt.Errorf("refusing to remove unrecognized data directory %s", migration.deployment.DataDir)
	}
	return os.RemoveAll(migration.legacy.DataDir)
}

func rebaseManagedConfig(raw []byte, legacy, canonical daemon.ScopeLayout) ([]byte, error) {
	lines := strings.SplitAfter(string(raw), "\n")
	section := ""
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]")
			continue
		}
		var oldPath, newPath string
		switch section {
		case "storage":
			oldPath = filepath.Join(legacy.DataDir, "sentinel.db")
			newPath = filepath.Join(canonical.DataDir, "sentinel.db")
		case "log":
			oldPath = legacy.LogPath
			newPath = canonical.LogPath
		default:
			continue
		}
		rewritten, changed, err := rewriteTOMLPathLine(line, oldPath, newPath)
		if err != nil {
			return nil, err
		}
		if changed {
			lines[index] = rewritten
		}
	}
	return []byte(strings.Join(lines, "")), nil
}

func rewriteTOMLPathLine(line, oldPath, newPath string) (string, bool, error) {
	newline := ""
	content := line
	if strings.HasSuffix(content, "\n") {
		newline = "\n"
		content = strings.TrimSuffix(content, "\n")
	}
	assignment := strings.Index(content, "=")
	if assignment < 0 || strings.TrimSpace(content[:assignment]) != "path" {
		return line, false, nil
	}
	rawValue := strings.TrimSpace(content[assignment+1:])
	value, suffix, err := parseTOMLStringValue(rawValue)
	if err != nil {
		return "", false, fmt.Errorf("parse managed path assignment %q: %w", strings.TrimSpace(content), err)
	}
	if filepath.Clean(value) != filepath.Clean(oldPath) {
		return line, false, nil
	}
	prefix := content[:assignment+1]
	return prefix + " " + strconv.Quote(newPath) + suffix + newline, true, nil
}

func parseTOMLStringValue(raw string) (value, suffix string, err error) {
	if raw == "" {
		return "", "", errors.New("path value is empty")
	}
	if raw[0] == '\'' {
		end := strings.IndexByte(raw[1:], '\'')
		if end < 0 {
			return "", "", errors.New("unterminated literal string")
		}
		end++
		return raw[1:end], raw[end+1:], nil
	}
	if raw[0] != '"' {
		return "", "", errors.New("path value must be a TOML string")
	}
	escaped := false
	for index := 1; index < len(raw); index++ {
		switch {
		case escaped:
			escaped = false
		case raw[index] == '\\':
			escaped = true
		case raw[index] == '"':
			value, err := strconv.Unquote(raw[:index+1])
			if err != nil {
				return "", "", err
			}
			return value, raw[index+1:], nil
		}
	}
	return "", "", errors.New("unterminated basic string")
}

func requireEmptyDestination(destination, source string) error {
	if samePath(destination, source) {
		return nil
	}
	entries, err := os.ReadDir(destination)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect migration destination %s: %w", destination, err)
	}
	if len(entries) != 0 {
		return fmt.Errorf("migration destination %s is not empty; reconcile it before migrating", destination)
	}
	return nil
}

func copyTreeAtomic(source, destination string, skip func(string, fs.DirEntry) bool) error {
	if samePath(source, destination) {
		return nil
	}
	if err := requireEmptyDestination(destination, source); err != nil {
		return err
	}
	if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove empty destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp(filepath.Dir(destination), ".sentinel-migrate-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporary) }()

	if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
		return os.Rename(temporary, destination)
	} else if err != nil {
		return err
	}
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil || relative == "." {
			return err
		}
		if skip != nil && skip(relative, entry) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to migrate symlink %s", path)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		target := filepath.Join(temporary, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to migrate non-regular file %s", path)
		}
		return copyRegularFile(path, target, info.Mode().Perm())
	})
	if err != nil {
		return err
	}
	return os.Rename(temporary, destination)
}

func skipLegacyDataEntry(relative string, _ fs.DirEntry) bool {
	first := strings.Split(relative, string(filepath.Separator))[0]
	return first == "logs" || first == "config.toml" || strings.HasPrefix(first, "config.toml.")
}

func copyLegacyConfigBackups(sourceDir, canonicalConfigPath string) ([]string, error) {
	entries, err := os.ReadDir(sourceDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read legacy config backups: %w", err)
	}
	var created []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "config.toml.") {
			continue
		}
		source := filepath.Join(sourceDir, entry.Name())
		suffix := strings.TrimPrefix(entry.Name(), "config.toml")
		target := canonicalConfigPath + suffix
		if _, err := os.Stat(target); err == nil {
			return created, fmt.Errorf("config backup destination already exists: %s", target)
		} else if !errors.Is(err, os.ErrNotExist) {
			return created, err
		}
		info, err := entry.Info()
		if err != nil {
			return created, err
		}
		if !info.Mode().IsRegular() {
			return created, fmt.Errorf("refusing to migrate non-regular config backup %s", source)
		}
		if err := copyRegularFile(source, target, info.Mode().Perm()); err != nil {
			return created, err
		}
		created = append(created, target)
	}
	return created, nil
}

func copyRegularFile(source, target string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	in, err := os.Open(source) //nolint:gosec // validated migration source.
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode) //nolint:gosec // explicit managed target.
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(target)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(target)
		return err
	}
	return nil
}

func writeFileAtomic(path string, raw []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".sentinel-config-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer func() { _ = os.Remove(temporary) }()
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func pathInside(path, directory string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	directory = filepath.Clean(strings.TrimSpace(directory))
	if path == "." || directory == "." {
		return false
	}
	relative, err := filepath.Rel(directory, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func samePath(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	return left != "" && right != "" && filepath.Clean(left) == filepath.Clean(right)
}
