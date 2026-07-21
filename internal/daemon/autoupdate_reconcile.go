package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

var launchdStartIntervalRE = regexp.MustCompile(`(?s)<key>StartInterval</key>\s*<integer>([0-9]+)</integer>`)

// ReconcileAutoUpdate refreshes an existing managed updater after the main
// service is installed. It preserves the timer schedule and activation state.
func ReconcileAutoUpdate(opts InstallUserAutoUpdateOptions) (bool, error) {
	return reconcileAutoUpdateForEUID(opts, os.Geteuid())
}

func reconcileAutoUpdateForEUID(opts InstallUserAutoUpdateOptions, euid int) (bool, error) {
	scope, err := normalizeExplicitScope(opts.SystemdScope)
	if err != nil {
		return false, err
	}
	if err := requireScopeAccess(scope, euid); err != nil {
		return false, err
	}
	status, err := UserAutoUpdateStatusForScope(scope)
	if err != nil {
		return false, err
	}
	if !status.ServiceUnitExists && !status.TimerUnitExists {
		return false, nil
	}

	opts.SystemdScope = scope
	if runtime.GOOS == launchdSupportedOS {
		return true, reconcileLaunchdAutoUpdate(opts, status)
	}
	return true, reconcileSystemdAutoUpdate(opts, status)
}

func reconcileSystemdAutoUpdate(opts InstallUserAutoUpdateOptions, status UserAutoUpdateServiceStatus) error {
	cfg, err := resolveInstallUserAutoUpdateConfig(opts)
	if err != nil {
		return err
	}
	if err := validateExecutable(cfg.execPath); err != nil {
		return err
	}

	servicePath, err := UserAutoUpdateServicePathForScope(cfg.scope)
	if err != nil {
		return err
	}
	timerPath, err := UserAutoUpdateTimerPathForScope(cfg.scope)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(servicePath), 0o750); err != nil {
		return fmt.Errorf("create systemd updater directory: %w", err)
	}

	mode := systemdUnitFileMode(cfg.scope)
	serviceReplacement, err := replaceManagedFile(
		servicePath,
		[]byte(renderUserAutoUpdateUnit(cfg.execPath, cfg.configPath, cfg.dataDir, cfg.serviceUnit, cfg.scope)),
		mode,
	)
	if err != nil {
		return fmt.Errorf("refresh updater service: %w", err)
	}

	timerContent := []byte(renderUserAutoUpdateTimer(cfg.onCalendar, cfg.randomizedDelay))
	if status.TimerUnitExists {
		// #nosec G304 -- timerPath is the fixed managed path for the validated scope.
		timerContent, err = os.ReadFile(timerPath)
		if err != nil {
			return rollbackManagedFiles(fmt.Errorf("read existing updater timer: %w", err), serviceReplacement)
		}
	}
	timerReplacement, err := replaceManagedFile(timerPath, timerContent, mode)
	if err != nil {
		return rollbackManagedFiles(fmt.Errorf("refresh updater timer: %w", err), serviceReplacement)
	}

	runFn := runSystemctlUser
	if cfg.scope == managerScopeSystem {
		runFn = runSystemctlSystem
	}
	if err := runFn("daemon-reload"); err != nil {
		return rollbackAutoUpdateReconcile(err, runFn, status, serviceReplacement, timerReplacement)
	}
	if err := runFn("reset-failed", userAutoUpdateServiceName); err != nil {
		return rollbackAutoUpdateReconcile(err, runFn, status, serviceReplacement, timerReplacement)
	}
	return nil
}

func rollbackAutoUpdateReconcile(
	cause error,
	runFn func(args ...string) error,
	status UserAutoUpdateServiceStatus,
	replacements ...*managedFileReplacement,
) error {
	rollbackErr := rollbackManagedFiles(cause, replacements...)
	if reloadErr := runFn("daemon-reload"); reloadErr != nil {
		rollbackErr = errors.Join(rollbackErr, fmt.Errorf("reload restored updater definitions: %w", reloadErr))
	}
	if status.TimerActiveState == serviceStateActive || status.TimerActiveState == "running" {
		if startErr := runFn("start", userAutoUpdateTimerName); startErr != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restart restored updater timer: %w", startErr))
		}
	}
	return rollbackErr
}

func reconcileLaunchdAutoUpdate(opts InstallUserAutoUpdateOptions, status UserAutoUpdateServiceStatus) error {
	content, err := os.ReadFile(status.ServicePath)
	if err != nil {
		return fmt.Errorf("read existing launchd updater: %w", err)
	}
	match := launchdStartIntervalRE.FindSubmatch(content)
	if len(match) != 2 {
		return errors.New("existing launchd updater has no valid StartInterval")
	}
	interval, err := strconv.Atoi(string(match[1]))
	if err != nil || interval <= 0 {
		return errors.New("existing launchd updater has an invalid StartInterval")
	}
	opts.OnCalendar = strconv.Itoa(interval)

	cfg, err := resolveLaunchdAutoUpdateInstallConfig(opts)
	if err != nil {
		return err
	}
	replacement, err := replaceLaunchdAutoUpdatePlist(cfg)
	if err != nil {
		return err
	}
	if strings.EqualFold(status.TimerEnabledState, "loaded") {
		_ = launchdBootout(cfg.scope, launchdAutoUpdateLabel)
		if err := launchdBootstrap(cfg.scope, cfg.updaterPath, launchdAutoUpdateLabel); err != nil {
			return rollbackLaunchdInstall(err, replacement, cfg.scope, launchdAutoUpdateLabel)
		}
	}
	return nil
}
