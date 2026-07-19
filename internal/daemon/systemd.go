package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/config"
)

// serviceUnitNameRE bounds an installed unit name to systemd-safe characters.
// In particular it forbids '%', which systemd treats as a specifier, and any
// path/whitespace characters that could break the generated unit file.
var serviceUnitNameRE = regexp.MustCompile(`^[A-Za-z0-9:_.@-]{1,128}$`)

const (
	userUnitName              = "sentinel.service"
	systemUnitPath            = "/etc/systemd/system/sentinel.service"
	userAutoUpdateServiceName = "sentinel-updater.service"
	userAutoUpdateTimerName   = "sentinel-updater.timer"
	systemAutoUpdateService   = "/etc/systemd/system/sentinel-updater.service"
	systemAutoUpdateTimer     = "/etc/systemd/system/sentinel-updater.timer"
	needrestartConfDir        = "/etc/needrestart/conf.d"
	needrestartConfPath       = "/etc/needrestart/conf.d/sentinel.conf"
	systemdSupportedOS        = "linux"
	managerScopeAuto          = "auto"
	managerScopeUser          = "user"
	managerScopeSystem        = "system"
	systemdStateUnknown       = "unknown"
	serviceStateActive        = "active"
	defaultOnCalendar         = "daily"
)

// InstallUserOptions represents install user options data.
type InstallUserOptions struct {
	ExecPath   string
	Scope      string
	ConfigPath string
	DataDir    string
	Enable     bool
	Start      bool
}

// UninstallUserOptions represents uninstall user options data.
type UninstallUserOptions struct {
	Scope      string
	Disable    bool
	Stop       bool
	RemoveUnit bool
}

// LogsOptions controls how the managed service log stream is rendered.
type LogsOptions struct {
	Scope  string
	Follow bool
	Lines  int
	Stdout io.Writer
	Stderr io.Writer
}

const defaultLogLines = 50

// InstallUserAutoUpdateOptions represents install user auto update options data.
type InstallUserAutoUpdateOptions struct {
	ExecPath        string
	ConfigPath      string
	DataDir         string
	Enable          bool
	Start           bool
	ServiceUnit     string
	SystemdScope    string // user, system
	OnCalendar      string
	RandomizedDelay time.Duration
}

// UninstallUserAutoUpdateOptions represents uninstall user auto update options data.
type UninstallUserAutoUpdateOptions struct {
	Disable    bool
	Stop       bool
	RemoveUnit bool
	Scope      string
}

// UserServiceStatus represents user service status data.
type UserServiceStatus struct {
	ServicePath        string
	UnitFileExists     bool
	SystemctlAvailable bool
	EnabledState       string
	ActiveState        string
}

// UserAutoUpdateServiceStatus represents user auto update service status data.
type UserAutoUpdateServiceStatus struct {
	ServicePath        string
	TimerPath          string
	ServiceUnitExists  bool
	TimerUnitExists    bool
	SystemctlAvailable bool
	TimerEnabledState  string
	TimerActiveState   string
	LastRunState       string
}

type installUserAutoUpdateConfig struct {
	scope           string
	execPath        string
	configPath      string
	dataDir         string
	serviceUnit     string
	onCalendar      string
	randomizedDelay time.Duration
}

// InstallUser handles install user.
func InstallUser(opts InstallUserOptions) error {
	if runtime.GOOS == launchdSupportedOS {
		return installUserLaunchd(opts)
	}
	if err := ensureServicePlatformSupported(); err != nil {
		return err
	}
	scope, err := normalizeExplicitScope(opts.Scope)
	if err != nil {
		return err
	}
	if err := RequireScopeAccess(scope); err != nil {
		return err
	}
	opts.Scope = scope
	if runtime.GOOS == systemdSupportedOS && scope == managerScopeSystem {
		return installSystemServiceLinux(opts)
	}
	if err := ensureSystemdUserSupported(); err != nil {
		return err
	}

	execPath, err := resolveExecPath(opts.ExecPath)
	if err != nil {
		return err
	}
	if err := validateExecutable(execPath); err != nil {
		return err
	}

	servicePath, err := userScopeUnitPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(servicePath), 0o750); err != nil {
		return fmt.Errorf("create systemd user directory: %w", err)
	}
	layout, err := LayoutForScope(scope)
	if err != nil {
		return err
	}
	unit := renderUserUnit(execPath, opts.ConfigPath, opts.DataDir, layout.LogPath)
	wasActive := isSystemctlUserActive("sentinel")
	replacement, err := replaceManagedFile(servicePath, []byte(unit), systemdUnitFileMode(scope))
	if err != nil {
		return fmt.Errorf("write user service: %w", err)
	}

	if err := runSystemctlUser("daemon-reload"); err != nil {
		return rollbackSystemdInstall(err, runSystemctlUser, "sentinel", wasActive, replacement)
	}
	if err := applySystemdUnitState(opts.Enable, opts.Start, isSystemctlUserActive, runSystemctlUser); err != nil {
		return rollbackSystemdInstall(err, runSystemctlUser, "sentinel", wasActive, replacement)
	}
	return nil
}

// InstallUserAutoUpdate handles install user auto update.
func InstallUserAutoUpdate(opts InstallUserAutoUpdateOptions) error {
	if runtime.GOOS == launchdSupportedOS {
		return installUserAutoUpdateLaunchd(opts)
	}
	if err := ensureServicePlatformSupported(); err != nil {
		return err
	}
	if runtime.GOOS != systemdSupportedOS {
		return errors.New("auto-update service commands are supported on Linux and macOS only")
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return errors.New("systemctl was not found in PATH")
	}
	scope, err := normalizeLinuxAutoUpdateScope(opts.SystemdScope)
	if err != nil {
		return err
	}
	if scope == managerScopeSystem && os.Geteuid() != 0 {
		return errors.New("scope=system requires root privileges")
	}

	cfg, err := resolveInstallUserAutoUpdateConfig(opts)
	if err != nil {
		return err
	}
	if err := validateExecutable(cfg.execPath); err != nil {
		return err
	}

	if cfg.scope == managerScopeSystem {
		return installSystemAutoUpdateLinux(
			cfg.execPath,
			cfg.configPath,
			cfg.dataDir,
			cfg.serviceUnit,
			cfg.onCalendar,
			cfg.randomizedDelay,
			opts.Enable,
			opts.Start,
		)
	}
	if err := ensureSystemdUserSupported(); err != nil {
		return err
	}
	return installUserAutoUpdateLinuxUser(cfg, opts)
}

func resolveInstallUserAutoUpdateConfig(opts InstallUserAutoUpdateOptions) (installUserAutoUpdateConfig, error) {
	scope, err := normalizeLinuxAutoUpdateScope(opts.SystemdScope)
	if err != nil {
		return installUserAutoUpdateConfig{}, err
	}
	execPath, err := resolveExecPath(opts.ExecPath)
	if err != nil {
		return installUserAutoUpdateConfig{}, err
	}

	serviceUnit := strings.TrimSpace(opts.ServiceUnit)
	if serviceUnit == "" {
		serviceUnit = "sentinel"
	}
	if !serviceUnitNameRE.MatchString(serviceUnit) {
		return installUserAutoUpdateConfig{}, errors.New("invalid service unit name")
	}

	onCalendar := strings.TrimSpace(opts.OnCalendar)
	if onCalendar == "" {
		onCalendar = defaultOnCalendar
	}
	randomizedDelay := opts.RandomizedDelay
	if randomizedDelay <= 0 {
		randomizedDelay = time.Hour
	}

	return installUserAutoUpdateConfig{
		scope:           scope,
		execPath:        execPath,
		configPath:      strings.TrimSpace(opts.ConfigPath),
		dataDir:         strings.TrimSpace(opts.DataDir),
		serviceUnit:     serviceUnit,
		onCalendar:      onCalendar,
		randomizedDelay: randomizedDelay,
	}, nil
}

func installUserAutoUpdateLinuxUser(cfg installUserAutoUpdateConfig, opts InstallUserAutoUpdateOptions) error {
	servicePath, err := UserAutoUpdateServicePathForScope(cfg.scope)
	if err != nil {
		return err
	}
	timerPath, err := UserAutoUpdateTimerPathForScope(cfg.scope)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(servicePath), 0o750); err != nil {
		return fmt.Errorf("create systemd user directory: %w", err)
	}

	serviceUnitText := renderUserAutoUpdateUnit(cfg.execPath, cfg.configPath, cfg.dataDir, cfg.serviceUnit, cfg.scope)
	wasActive := isSystemctlUserActive("sentinel-updater.timer")
	serviceReplacement, err := replaceManagedFile(servicePath, []byte(serviceUnitText), systemdUnitFileMode(cfg.scope))
	if err != nil {
		return fmt.Errorf("write updater service: %w", err)
	}
	timerUnitText := renderUserAutoUpdateTimer(cfg.onCalendar, cfg.randomizedDelay)
	timerReplacement, err := replaceManagedFile(timerPath, []byte(timerUnitText), systemdUnitFileMode(cfg.scope))
	if err != nil {
		return rollbackManagedFiles(fmt.Errorf("write updater timer: %w", err), serviceReplacement)
	}

	if err := runSystemctlUser("daemon-reload"); err != nil {
		return rollbackSystemdInstall(withSystemdUserBusHint(err), runSystemctlUser, "sentinel-updater.timer", wasActive, serviceReplacement, timerReplacement)
	}
	if err := applyUserAutoUpdateTimerState(opts.Enable, opts.Start); err != nil {
		return rollbackSystemdInstall(err, runSystemctlUser, "sentinel-updater.timer", wasActive, serviceReplacement, timerReplacement)
	}
	return nil
}

func applyUserAutoUpdateTimerState(enable, start bool) error {
	switch {
	case enable && start:
		return withSystemdUserBusHint(runSystemctlUser("enable", "--now", "sentinel-updater.timer"))
	case enable:
		return withSystemdUserBusHint(runSystemctlUser("enable", "sentinel-updater.timer"))
	case start:
		return withSystemdUserBusHint(runSystemctlUser("start", "sentinel-updater.timer"))
	default:
		return nil
	}
}

// UninstallUser handles uninstall user.
func UninstallUser(opts UninstallUserOptions) error {
	if runtime.GOOS == launchdSupportedOS {
		return uninstallUserLaunchd(opts)
	}
	if err := ensureServicePlatformSupported(); err != nil {
		return err
	}

	deployment, err := ResolveDeployment(opts.Scope)
	if err != nil {
		return err
	}
	scope := deployment.Scope
	if err := requireScopePrivilege(scope); err != nil {
		return err
	}
	if scope == managerScopeSystem {
		return uninstallSystemServiceLinux(opts)
	}

	if err := ensureSystemdUserSupported(); err != nil {
		return err
	}
	servicePath, err := userScopeUnitPath()
	if err != nil {
		return err
	}
	return uninstallUserSystemd(opts, servicePath, runSystemctlUser)
}

func uninstallUserSystemd(opts UninstallUserOptions, servicePath string, runFn func(args ...string) error) error {
	switch {
	case opts.Disable && opts.Stop:
		_ = runFn("disable", "--now", "sentinel")
	case opts.Disable:
		_ = runFn("disable", "sentinel")
	case opts.Stop:
		_ = runFn("stop", "sentinel")
	}

	if opts.RemoveUnit {
		if err := os.Remove(servicePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove user service: %w", err)
		}
	}

	return runFn("daemon-reload")
}

// UninstallUserAutoUpdate handles uninstall user auto update.
func UninstallUserAutoUpdate(opts UninstallUserAutoUpdateOptions) error {
	if runtime.GOOS == launchdSupportedOS {
		return uninstallUserAutoUpdateLaunchd(opts)
	}
	if err := ensureServicePlatformSupported(); err != nil {
		return err
	}
	scope, err := normalizeLinuxAutoUpdateScope(opts.Scope)
	if err != nil {
		return err
	}

	if scope == managerScopeSystem {
		return uninstallSystemAutoUpdateLinux(opts)
	}

	if err := ensureSystemdUserSupported(); err != nil {
		return err
	}

	return uninstallUserAutoUpdateSystemd(opts, runSystemctlUser)
}

func uninstallUserAutoUpdateSystemd(opts UninstallUserAutoUpdateOptions, runFn func(args ...string) error) error {
	stopUserAutoUpdateTimer(opts.Disable, opts.Stop, runFn)
	if opts.RemoveUnit {
		if err := removeUserAutoUpdateUnits(opts.Scope); err != nil {
			return err
		}
	}

	if err := runFn("daemon-reload"); err != nil {
		return withSystemdUserBusHint(err)
	}
	return nil
}

func stopUserAutoUpdateTimer(disable, stop bool, runFn func(args ...string) error) {
	switch {
	case disable && stop:
		_ = runFn("disable", "--now", "sentinel-updater.timer")
	case disable:
		_ = runFn("disable", "sentinel-updater.timer")
	case stop:
		_ = runFn("stop", "sentinel-updater.timer")
	}
}

func removeUserAutoUpdateUnits(scope string) error {
	servicePath, err := UserAutoUpdateServicePathForScope(scope)
	if err != nil {
		return err
	}
	timerPath, err := UserAutoUpdateTimerPathForScope(scope)
	if err != nil {
		return err
	}
	if err := os.Remove(servicePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove updater service: %w", err)
	}
	if err := os.Remove(timerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove updater timer: %w", err)
	}
	return nil
}

// userStatusUserLinux reads the status of the user-scope systemd unit.
func userStatusUserLinux() (UserServiceStatus, error) {
	servicePath, err := userScopeUnitPath()
	if err != nil {
		return UserServiceStatus{}, err
	}

	st := UserServiceStatus{ServicePath: servicePath}
	if fileExists(servicePath) {
		st.UnitFileExists = true
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return st, nil
	}

	st.SystemctlAvailable = true
	st.EnabledState = readSystemctlState("is-enabled", "sentinel")
	st.ActiveState = readSystemctlState("is-active", "sentinel")
	return st, nil
}

// UserLogs streams the managed service log to opts.Stdout/opts.Stderr. On Linux
// it shells out to journalctl for the sentinel unit; on macOS it tails the
// launchd plist log files.
func UserLogs(opts LogsOptions) error {
	if runtime.GOOS == launchdSupportedOS {
		return userLogsLaunchd(opts)
	}
	if runtime.GOOS != systemdSupportedOS {
		return errors.New("service log commands are supported on Linux and macOS only")
	}
	if _, err := exec.LookPath("journalctl"); err != nil {
		return errors.New("journalctl was not found in PATH")
	}

	deployment, err := ResolveDeployment(opts.Scope)
	if err != nil {
		return err
	}
	scope := deployment.Scope
	args := journalctlLogArgs(scope == managerScopeSystem, opts.Follow, opts.Lines)
	cmd := exec.CommandContext(context.Background(), "journalctl", args...)
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	return runLogCommand(cmd)
}

// journalctlLogArgs builds the journalctl arguments for the sentinel unit. A
// non-zero or negative line count falls back to the default. The output is
// paged off (--no-pager) unless following, which streams live lines instead.
func journalctlLogArgs(system, follow bool, lines int) []string {
	if lines <= 0 {
		lines = defaultLogLines
	}
	args := make([]string, 0, 7)
	if !system {
		args = append(args, "--user")
	}
	args = append(args, "-u", userUnitName, "-n", strconv.Itoa(lines))
	if follow {
		args = append(args, "-f")
	} else {
		args = append(args, "--no-pager")
	}
	return args
}

// runLogCommand runs a log-streaming command. A non-zero exit (an empty unit,
// or SIGINT while following) is not a failure — the stream itself is the
// result, and genuine errors already reached the connected stderr.
func runLogCommand(cmd *exec.Cmd) error {
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil
		}
		return fmt.Errorf("run %s: %w", filepath.Base(cmd.Path), err)
	}
	return nil
}

// UserAutoUpdateStatusForScope handles user auto update status for scope.
func UserAutoUpdateStatusForScope(scopeRaw string) (UserAutoUpdateServiceStatus, error) {
	if runtime.GOOS == launchdSupportedOS {
		return userAutoUpdateStatusLaunchdForScope(scopeRaw)
	}
	if err := ensureServicePlatformSupported(); err != nil {
		return UserAutoUpdateServiceStatus{}, err
	}

	scope, err := normalizeLinuxAutoUpdateScope(scopeRaw)
	if err != nil {
		return UserAutoUpdateServiceStatus{}, err
	}

	servicePath, err := UserAutoUpdateServicePathForScope(scope)
	if err != nil {
		return UserAutoUpdateServiceStatus{}, err
	}
	timerPath, err := UserAutoUpdateTimerPathForScope(scope)
	if err != nil {
		return UserAutoUpdateServiceStatus{}, err
	}

	st := UserAutoUpdateServiceStatus{
		ServicePath: servicePath,
		TimerPath:   timerPath,
	}
	if info, statErr := os.Stat(servicePath); statErr == nil && !info.IsDir() {
		st.ServiceUnitExists = true
	}
	if info, statErr := os.Stat(timerPath); statErr == nil && !info.IsDir() {
		st.TimerUnitExists = true
	}

	if runtime.GOOS != systemdSupportedOS {
		return st, nil
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return st, nil
	}

	st.SystemctlAvailable = true
	if scope == managerScopeSystem {
		st.TimerEnabledState = readSystemctlSystemState("is-enabled", "sentinel-updater.timer")
		st.TimerActiveState = readSystemctlSystemState("is-active", "sentinel-updater.timer")
		st.LastRunState = readSystemctlSystemState("is-active", "sentinel-updater.service")
		return st, nil
	}

	st.TimerEnabledState = readSystemctlState("is-enabled", "sentinel-updater.timer")
	st.TimerActiveState = readSystemctlState("is-active", "sentinel-updater.timer")
	st.LastRunState = readSystemctlState("is-active", "sentinel-updater.service")
	return st, nil
}

// UserServicePathForScope returns the managed service path without deriving the
// scope from process privileges.
func UserServicePathForScope(scope string) (string, error) {
	return servicePathForScope(scope)
}

// UserAutoUpdateServicePathForScope handles user auto update service path for scope.
func UserAutoUpdateServicePathForScope(scopeRaw string) (string, error) {
	if runtime.GOOS == launchdSupportedOS {
		return userAutoUpdatePathLaunchdForScope(scopeRaw)
	}
	if err := ensureServicePlatformSupported(); err != nil {
		return "", err
	}
	scope, err := normalizeLinuxAutoUpdateScope(scopeRaw)
	if err != nil {
		return "", err
	}
	if scope == managerScopeSystem {
		return systemAutoUpdateService, nil
	}
	home, err := userScopeHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "systemd", "user", userAutoUpdateServiceName), nil
}

// UserAutoUpdateTimerPathForScope handles user auto update timer path for scope.
func UserAutoUpdateTimerPathForScope(scopeRaw string) (string, error) {
	if runtime.GOOS == launchdSupportedOS {
		// launchd runs timer semantics inside a single plist.
		return userAutoUpdatePathLaunchdForScope(scopeRaw)
	}
	if err := ensureServicePlatformSupported(); err != nil {
		return "", err
	}
	scope, err := normalizeLinuxAutoUpdateScope(scopeRaw)
	if err != nil {
		return "", err
	}
	if scope == managerScopeSystem {
		return systemAutoUpdateTimer, nil
	}
	home, err := userScopeHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "systemd", "user", userAutoUpdateTimerName), nil
}

func ensureSystemdUserSupported() error {
	if runtime.GOOS != systemdSupportedOS {
		return errors.New("systemd user service commands are supported on Linux only")
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return errors.New("systemctl was not found in PATH")
	}
	return nil
}

func ensureServicePlatformSupported() error {
	if runtime.GOOS == systemdSupportedOS || runtime.GOOS == launchdSupportedOS {
		return nil
	}
	return errors.New("service commands are supported on Linux and macOS only")
}

func normalizeLinuxAutoUpdateScope(raw string) (string, error) {
	scope := strings.ToLower(strings.TrimSpace(raw))
	switch scope {
	case managerScopeUser:
		return managerScopeUser, nil
	case managerScopeSystem:
		return managerScopeSystem, nil
	default:
		return "", fmt.Errorf("invalid systemd scope %q: pass user or system explicitly", raw)
	}
}

func systemdUnitFileMode(scope string) os.FileMode {
	if scope == managerScopeSystem {
		return 0o644
	}
	return 0o600
}

func installSystemServiceLinux(opts InstallUserOptions) error {
	if os.Geteuid() != 0 {
		return errors.New("system service install requires root privileges")
	}
	execPath, err := resolveExecPath(opts.ExecPath)
	if err != nil {
		return err
	}
	if err := validateExecutable(execPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(systemUnitPath), 0o750); err != nil {
		return fmt.Errorf("create systemd system directory: %w", err)
	}
	layout, err := LayoutForScope(managerScopeSystem)
	if err != nil {
		return err
	}
	unit := renderUserUnit(execPath, opts.ConfigPath, opts.DataDir, layout.LogPath)
	wasActive := isSystemctlSystemActive("sentinel")
	replacement, err := replaceManagedFile(systemUnitPath, []byte(unit), systemdUnitFileMode(managerScopeSystem))
	if err != nil {
		return fmt.Errorf("write system service: %w", err)
	}

	installNeedrestartOverride(needrestartConfDir, needrestartConfPath)

	if err := runSystemctlSystem("daemon-reload"); err != nil {
		return rollbackSystemdInstall(err, runSystemctlSystem, "sentinel", wasActive, replacement)
	}
	if err := applySystemdUnitState(opts.Enable, opts.Start, isSystemctlSystemActive, runSystemctlSystem); err != nil {
		return rollbackSystemdInstall(err, runSystemctlSystem, "sentinel", wasActive, replacement)
	}
	return nil
}

func uninstallSystemServiceLinux(opts UninstallUserOptions) error {
	if os.Geteuid() != 0 {
		return errors.New("system service uninstall requires root privileges")
	}
	switch {
	case opts.Disable && opts.Stop:
		_ = runSystemctlSystem("disable", "--now", "sentinel")
	case opts.Disable:
		_ = runSystemctlSystem("disable", "sentinel")
	case opts.Stop:
		_ = runSystemctlSystem("stop", "sentinel")
	}

	if opts.RemoveUnit {
		if err := os.Remove(systemUnitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove system service: %w", err)
		}
		removeNeedrestartOverride(needrestartConfPath)
	}
	return runSystemctlSystem("daemon-reload")
}

func userStatusSystemLinux() UserServiceStatus {
	st := UserServiceStatus{
		ServicePath: systemUnitPath,
	}
	if info, statErr := os.Stat(systemUnitPath); statErr == nil && !info.IsDir() {
		st.UnitFileExists = true
	}

	if _, err := exec.LookPath("systemctl"); err != nil {
		return st
	}
	st.SystemctlAvailable = true
	st.EnabledState = readSystemctlSystemState("is-enabled", "sentinel")
	st.ActiveState = readSystemctlSystemState("is-active", "sentinel")
	return st
}

func installSystemAutoUpdateLinux(execPath, configPath, dataDir, serviceUnit, onCalendar string, randomizedDelay time.Duration, enable, start bool) error {
	if os.Geteuid() != 0 {
		return errors.New("scope=system requires root privileges")
	}
	servicePath := systemAutoUpdateService
	timerPath := systemAutoUpdateTimer

	if err := os.MkdirAll(filepath.Dir(servicePath), 0o750); err != nil {
		return fmt.Errorf("create systemd system directory: %w", err)
	}

	serviceUnitText := renderUserAutoUpdateUnit(execPath, configPath, dataDir, serviceUnit, managerScopeSystem)
	wasActive := isSystemctlSystemActive("sentinel-updater.timer")
	serviceReplacement, err := replaceManagedFile(servicePath, []byte(serviceUnitText), systemdUnitFileMode(managerScopeSystem))
	if err != nil {
		return fmt.Errorf("write updater system service: %w", err)
	}
	timerUnitText := renderUserAutoUpdateTimer(onCalendar, randomizedDelay)
	timerReplacement, err := replaceManagedFile(timerPath, []byte(timerUnitText), systemdUnitFileMode(managerScopeSystem))
	if err != nil {
		return rollbackManagedFiles(fmt.Errorf("write updater system timer: %w", err), serviceReplacement)
	}

	if err := runSystemctlSystem("daemon-reload"); err != nil {
		return rollbackSystemdInstall(err, runSystemctlSystem, "sentinel-updater.timer", wasActive, serviceReplacement, timerReplacement)
	}
	var stateErr error
	switch {
	case enable && start:
		stateErr = runSystemctlSystem("enable", "--now", "sentinel-updater.timer")
	case enable:
		stateErr = runSystemctlSystem("enable", "sentinel-updater.timer")
	case start:
		stateErr = runSystemctlSystem("start", "sentinel-updater.timer")
	}
	if stateErr != nil {
		return rollbackSystemdInstall(stateErr, runSystemctlSystem, "sentinel-updater.timer", wasActive, serviceReplacement, timerReplacement)
	}
	return nil
}

func rollbackSystemdInstall(
	cause error,
	runFn func(args ...string) error,
	unit string,
	wasActive bool,
	replacements ...*managedFileReplacement,
) error {
	errs := []error{cause}
	wasInstalled := len(replacements) > 0 && replacements[0] != nil && replacements[0].existed
	if !wasInstalled {
		if disableErr := runFn("disable", "--now", unit); disableErr != nil {
			errs = append(errs, fmt.Errorf("disable partially installed systemd unit: %w", disableErr))
		}
	}
	rollbackErr := rollbackManagedFiles(errors.Join(errs...), replacements...)
	if reloadErr := runFn("daemon-reload"); reloadErr != nil {
		return errors.Join(rollbackErr, fmt.Errorf("reload restored systemd definitions: %w", reloadErr))
	}
	if wasInstalled && wasActive {
		if restartErr := runFn("restart", unit); restartErr != nil {
			return errors.Join(rollbackErr, fmt.Errorf("restart restored systemd unit: %w", restartErr))
		}
	}
	return rollbackErr
}

func uninstallSystemAutoUpdateLinux(opts UninstallUserAutoUpdateOptions) error {
	if os.Geteuid() != 0 {
		return errors.New("scope=system requires root privileges")
	}

	switch {
	case opts.Disable && opts.Stop:
		_ = runSystemctlSystem("disable", "--now", "sentinel-updater.timer")
	case opts.Disable:
		_ = runSystemctlSystem("disable", "sentinel-updater.timer")
	case opts.Stop:
		_ = runSystemctlSystem("stop", "sentinel-updater.timer")
	}

	if opts.RemoveUnit {
		if err := os.Remove(systemAutoUpdateService); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove updater system service: %w", err)
		}
		if err := os.Remove(systemAutoUpdateTimer); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove updater system timer: %w", err)
		}
	}
	return runSystemctlSystem("daemon-reload")
}

func runSystemctlUser(args ...string) error {
	cmd := exec.CommandContext(context.Background(), "systemctl", append([]string{"--user"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("systemctl --user %s failed: %w", strings.Join(args, " "), err)
		}
		return fmt.Errorf("systemctl --user %s failed: %s", strings.Join(args, " "), msg)
	}
	return nil
}

func runSystemctlSystem(args ...string) error {
	cmd := exec.CommandContext(context.Background(), "systemctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("systemctl %s failed: %w", strings.Join(args, " "), err)
		}
		return fmt.Errorf("systemctl %s failed: %s", strings.Join(args, " "), msg)
	}
	return nil
}

func isSystemctlUserActive(unit string) bool {
	cmd := exec.CommandContext(context.Background(), "systemctl", "--user", "is-active", "--quiet", unit)
	return cmd.Run() == nil
}

func isSystemctlSystemActive(unit string) bool {
	cmd := exec.CommandContext(context.Background(), "systemctl", "is-active", "--quiet", unit)
	return cmd.Run() == nil
}

func applySystemdUnitState(
	enable bool,
	start bool,
	isActiveFn func(unit string) bool,
	runFn func(args ...string) error,
) error {
	const unit = "sentinel"

	if enable {
		if err := runFn("enable", unit); err != nil {
			return err
		}
	}
	if !start {
		return nil
	}

	action := actionStart
	if isActiveFn != nil && isActiveFn(unit) {
		action = actionRestart
	}
	return runFn(action, unit)
}

func withSystemdUserBusHint(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "failed to connect to user scope bus") ||
		strings.Contains(msg, "dbus_session_bus_address") ||
		strings.Contains(msg, "xdg_runtime_dir not defined") {
		return fmt.Errorf("%w; if running as root use -scope system, or run as the target user with an active user session", err)
	}
	return err
}

func readSystemctlState(args ...string) string {
	cmd := exec.CommandContext(context.Background(), "systemctl", append([]string{"--user"}, args...)...)
	out, err := cmd.CombinedOutput()
	state := strings.TrimSpace(string(out))
	if err != nil {
		return normalizeSystemctlErrorState(state)
	}
	if state == "" {
		return "-"
	}
	return state
}

func readSystemctlSystemState(args ...string) string {
	cmd := exec.CommandContext(context.Background(), "systemctl", args...)
	out, err := cmd.CombinedOutput()
	state := strings.TrimSpace(string(out))
	if err != nil {
		return normalizeSystemctlErrorState(state)
	}
	if state == "" {
		return "-"
	}
	return state
}

func normalizeSystemctlErrorState(state string) string {
	normalized := strings.ToLower(strings.TrimSpace(state))
	switch {
	case strings.Contains(normalized, "failed to connect"):
		return "unavailable"
	case strings.Contains(normalized, "could not be found"),
		strings.Contains(normalized, "no such file or directory"),
		strings.Contains(normalized, "failed to get unit file state"):
		return "not-found"
	case normalized == "":
		return systemdStateUnknown
	case strings.Contains(normalized, "\n"):
		return systemdStateUnknown
	default:
		return strings.TrimSpace(state)
	}
}

func renderUserUnit(execPath, configPath, dataDir, logPath string) string {
	configArg := ""
	if strings.TrimSpace(configPath) != "" {
		configArg = " --config=" + escapeSystemdExec(configPath)
	}
	return fmt.Sprintf(`[Unit]
Description=Sentinel - terminal workspace
Documentation=https://github.com/opus-domini/sentinel
StartLimitIntervalSec=60
StartLimitBurst=4

[Service]
Type=simple
ExecStart=%s%s daemon
Restart=on-failure
RestartSec=2
KillMode=process
Environment=SENTINEL_LOG_LEVEL=info
Environment=HOME=%%h
Environment="SENTINEL_DATA_DIR=%s"
Environment="%s=%s"
Environment=TERM=xterm-256color
Environment=LANG=C.UTF-8
# Note: NoNewPrivileges and SystemCallArchitectures are intentionally omitted —
# sudo requires new-privilege capability for multi-user sessions.

[Install]
WantedBy=default.target
`, escapeSystemdExec(execPath), configArg, escapeSystemdEnvironment(dataDir), config.ManagedDefaultLogPathEnv, escapeSystemdEnvironment(logPath))
}

func renderUserAutoUpdateUnit(execPath, configPath, dataDir, _ string, scope string) string {
	configArg := ""
	if strings.TrimSpace(configPath) != "" {
		configArg = " --config=" + escapeSystemdExec(configPath)
	}
	return fmt.Sprintf(`[Unit]
Description=Sentinel automatic updater
Documentation=https://github.com/opus-domini/sentinel
Wants=network-online.target
After=network-online.target

[Service]
Type=oneshot
ExecStart=%s%s update apply --scope=%s
Environment=SENTINEL_LOG_LEVEL=info
Environment="SENTINEL_DATA_DIR=%s"

[Install]
WantedBy=default.target
`, escapeSystemdExec(execPath), configArg, scope, escapeSystemdEnvironment(dataDir))
}

func renderUserAutoUpdateTimer(onCalendar string, randomizedDelay time.Duration) string {
	return fmt.Sprintf(`[Unit]
Description=Run Sentinel updater daily

[Timer]
OnCalendar=%s
RandomizedDelaySec=%d
Persistent=true
Unit=sentinel-updater.service

[Install]
WantedBy=timers.target
`, onCalendar, int64(randomizedDelay.Seconds()))
}

func escapeSystemdExec(path string) string {
	path = strings.ReplaceAll(path, "\\", "\\\\")
	return strings.ReplaceAll(path, " ", "\\x20")
}

func escapeSystemdEnvironment(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

// installNeedrestartOverride writes a config snippet that prevents needrestart
// from automatically restarting sentinel during unattended package upgrades.
// Sentinel is a static Go binary — its child processes (tmux, bash) naturally
// pick up new libraries when recreated, so an automatic service restart is
// both unnecessary and harmful (rapid restarts trigger systemd start-limit).
func installNeedrestartOverride(confDir, confPath string) {
	if info, err := os.Stat(confDir); err != nil || !info.IsDir() {
		return
	}
	const content = "# Sentinel: static Go binary, does not need restart after library upgrades.\n" +
		"$nrconf{override_rc}{qr(^sentinel)} = 0;\n"
	if err := os.WriteFile(confPath, []byte(content), 0o644); err == nil { //nolint:gosec // needrestart convention requires a world-readable system config
		_ = os.Chmod(confPath, 0o644) //nolint:gosec // enforce the documented mode independently of the process umask
	}
}

func removeNeedrestartOverride(confPath string) {
	_ = os.Remove(confPath)
}

func resolveExecPath(raw string) (string, error) {
	execPath := strings.TrimSpace(raw)
	if execPath == "" {
		path, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("resolve executable path: %w", err)
		}
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			execPath = resolved
		} else {
			execPath = path
		}
	}
	if strings.Contains(execPath, "\n") || strings.Contains(execPath, "\r") {
		return "", errors.New("invalid executable path")
	}
	return execPath, nil
}

func validateExecutable(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("executable path must be absolute: %s", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect executable %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("executable path is not a regular file: %s", path)
	}
	if info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("executable path is not executable: %s", path)
	}
	return nil
}
