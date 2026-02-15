package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	userUnitName              = "sentinel.service"
	systemUnitPath            = "/etc/systemd/system/sentinel.service"
	userAutoUpdateServiceName = "sentinel-updater.service"
	userAutoUpdateTimerName   = "sentinel-updater.timer"
	systemAutoUpdateService   = "/etc/systemd/system/sentinel-updater.service"
	systemAutoUpdateTimer     = "/etc/systemd/system/sentinel-updater.timer"
	systemdSupportedOS        = "linux"
	managerScopeAuto          = "auto"
	managerScopeUser          = "user"
	managerScopeSystem        = "system"
	managerScopeLaunchd       = "launchd"
	systemdStateUnknown       = "unknown"
)

type InstallUserOptions struct {
	ExecPath string
	Enable   bool
	Start    bool
}

type UninstallUserOptions struct {
	Disable    bool
	Stop       bool
	RemoveUnit bool
}

type InstallUserAutoUpdateOptions struct {
	ExecPath        string
	Enable          bool
	Start           bool
	ServiceUnit     string
	SystemdScope    string // user, system
	OnCalendar      string
	RandomizedDelay time.Duration
}

type UninstallUserAutoUpdateOptions struct {
	Disable    bool
	Stop       bool
	RemoveUnit bool
	Scope      string
}

type UserServiceStatus struct {
	ServicePath        string
	UnitFileExists     bool
	SystemctlAvailable bool
	EnabledState       string
	ActiveState        string
}

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

func InstallUser(opts InstallUserOptions) error {
	if runtime.GOOS == launchdSupportedOS {
		return installUserLaunchd(opts)
	}
	if err := ensureServicePlatformSupported(); err != nil {
		return err
	}
	if runtime.GOOS == systemdSupportedOS && os.Geteuid() == 0 {
		return installSystemServiceLinux(opts)
	}
	if err := ensureSystemdUserSupported(); err != nil {
		return err
	}

	execPath, err := resolveExecPath(opts.ExecPath)
	if err != nil {
		return err
	}

	servicePath, err := UserServicePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(servicePath), 0o750); err != nil {
		return fmt.Errorf("create systemd user directory: %w", err)
	}

	unit := renderUserUnit(execPath)
	if err := os.WriteFile(servicePath, []byte(unit), 0o600); err != nil {
		return fmt.Errorf("write user service: %w", err)
	}

	if err := runSystemctlUser("daemon-reload"); err != nil {
		return err
	}
	if opts.Enable && opts.Start {
		return runSystemctlUser("enable", "--now", "sentinel")
	}
	if opts.Enable {
		return runSystemctlUser("enable", "sentinel")
	}
	if opts.Start {
		return runSystemctlUser("start", "sentinel")
	}
	return nil
}

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

	execPath, err := resolveExecPath(opts.ExecPath)
	if err != nil {
		return err
	}
	serviceUnit := strings.TrimSpace(opts.ServiceUnit)
	if serviceUnit == "" {
		serviceUnit = "sentinel"
	}
	if strings.ContainsAny(serviceUnit, "\n\r\t ") {
		return errors.New("invalid service unit name")
	}
	onCalendar := strings.TrimSpace(opts.OnCalendar)
	if onCalendar == "" {
		onCalendar = "daily"
	}
	randomizedDelay := opts.RandomizedDelay
	if randomizedDelay <= 0 {
		randomizedDelay = time.Hour
	}

	if scope == managerScopeSystem {
		return installSystemAutoUpdateLinux(execPath, serviceUnit, scope, onCalendar, randomizedDelay, opts.Enable, opts.Start)
	}
	if err := ensureSystemdUserSupported(); err != nil {
		return err
	}

	servicePath, err := UserAutoUpdateServicePathForScope(scope)
	if err != nil {
		return err
	}
	timerPath, err := UserAutoUpdateTimerPathForScope(scope)
	if err != nil {
		return err
	}

	baseDir := filepath.Dir(servicePath)
	if err := os.MkdirAll(baseDir, 0o750); err != nil {
		return fmt.Errorf("create systemd user directory: %w", err)
	}

	serviceUnitText := renderUserAutoUpdateUnit(execPath, serviceUnit, scope)
	if err := os.WriteFile(servicePath, []byte(serviceUnitText), 0o600); err != nil {
		return fmt.Errorf("write updater service: %w", err)
	}
	timerUnitText := renderUserAutoUpdateTimer(onCalendar, randomizedDelay)
	if err := os.WriteFile(timerPath, []byte(timerUnitText), 0o600); err != nil {
		return fmt.Errorf("write updater timer: %w", err)
	}

	if err := runSystemctlUser("daemon-reload"); err != nil {
		return withSystemdUserBusHint(err)
	}
	switch {
	case opts.Enable && opts.Start:
		return withSystemdUserBusHint(runSystemctlUser("enable", "--now", "sentinel-updater.timer"))
	case opts.Enable:
		return withSystemdUserBusHint(runSystemctlUser("enable", "sentinel-updater.timer"))
	case opts.Start:
		return withSystemdUserBusHint(runSystemctlUser("start", "sentinel-updater.timer"))
	default:
		return nil
	}
}

func UninstallUser(opts UninstallUserOptions) error {
	if runtime.GOOS == launchdSupportedOS {
		return uninstallUserLaunchd(opts)
	}
	if err := ensureServicePlatformSupported(); err != nil {
		return err
	}
	if runtime.GOOS == systemdSupportedOS && os.Geteuid() == 0 {
		return uninstallSystemServiceLinux(opts)
	}
	if err := ensureSystemdUserSupported(); err != nil {
		return err
	}

	switch {
	case opts.Disable && opts.Stop:
		_ = runSystemctlUser("disable", "--now", "sentinel")
	case opts.Disable:
		_ = runSystemctlUser("disable", "sentinel")
	case opts.Stop:
		_ = runSystemctlUser("stop", "sentinel")
	}

	if opts.RemoveUnit {
		servicePath, err := UserServicePath()
		if err != nil {
			return err
		}
		if err := os.Remove(servicePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove user service: %w", err)
		}
	}

	return runSystemctlUser("daemon-reload")
}

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

	switch {
	case opts.Disable && opts.Stop:
		_ = runSystemctlUser("disable", "--now", "sentinel-updater.timer")
	case opts.Disable:
		_ = runSystemctlUser("disable", "sentinel-updater.timer")
	case opts.Stop:
		_ = runSystemctlUser("stop", "sentinel-updater.timer")
	}

	if opts.RemoveUnit {
		servicePath, err := UserAutoUpdateServicePath()
		if err != nil {
			return err
		}
		timerPath, err := UserAutoUpdateTimerPath()
		if err != nil {
			return err
		}
		if err := os.Remove(servicePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove updater service: %w", err)
		}
		if err := os.Remove(timerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove updater timer: %w", err)
		}
	}

	if err := runSystemctlUser("daemon-reload"); err != nil {
		return withSystemdUserBusHint(err)
	}
	return nil
}

func UserStatus() (UserServiceStatus, error) {
	if runtime.GOOS == launchdSupportedOS {
		return userStatusLaunchd()
	}
	if err := ensureServicePlatformSupported(); err != nil {
		return UserServiceStatus{}, err
	}
	if runtime.GOOS == systemdSupportedOS && os.Geteuid() == 0 {
		return userStatusSystemLinux()
	}
	servicePath, err := UserServicePath()
	if err != nil {
		return UserServiceStatus{}, err
	}

	st := UserServiceStatus{
		ServicePath: servicePath,
	}
	if info, statErr := os.Stat(servicePath); statErr == nil && !info.IsDir() {
		st.UnitFileExists = true
	}

	if runtime.GOOS != systemdSupportedOS {
		return st, nil
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return st, nil
	}

	st.SystemctlAvailable = true
	st.EnabledState = readSystemctlState("is-enabled", "sentinel")
	st.ActiveState = readSystemctlState("is-active", "sentinel")
	return st, nil
}

func UserAutoUpdateStatus() (UserAutoUpdateServiceStatus, error) {
	return UserAutoUpdateStatusForScope("")
}

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

func UserServicePath() (string, error) {
	if runtime.GOOS == launchdSupportedOS {
		return userServicePathLaunchd()
	}
	if err := ensureServicePlatformSupported(); err != nil {
		return "", err
	}
	if runtime.GOOS == systemdSupportedOS && os.Geteuid() == 0 {
		return systemUnitPath, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "systemd", "user", userUnitName), nil
}

func UserAutoUpdateServicePath() (string, error) {
	return UserAutoUpdateServicePathForScope("")
}

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
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "systemd", "user", userAutoUpdateServiceName), nil
}

func UserAutoUpdateTimerPath() (string, error) {
	return UserAutoUpdateTimerPathForScope("")
}

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
	home, err := os.UserHomeDir()
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
	case "", managerScopeAuto:
		if os.Geteuid() == 0 {
			return managerScopeSystem, nil
		}
		return managerScopeUser, nil
	case managerScopeUser:
		return managerScopeUser, nil
	case managerScopeSystem:
		return managerScopeSystem, nil
	default:
		return "", fmt.Errorf("invalid systemd scope: %s", raw)
	}
}

func installSystemServiceLinux(opts InstallUserOptions) error {
	execPath, err := resolveExecPath(opts.ExecPath)
	if err != nil {
		return err
	}
	if os.Geteuid() != 0 {
		return errors.New("system service install requires root privileges")
	}

	if err := os.MkdirAll(filepath.Dir(systemUnitPath), 0o750); err != nil {
		return fmt.Errorf("create systemd system directory: %w", err)
	}
	unit := renderUserUnit(execPath)
	if err := os.WriteFile(systemUnitPath, []byte(unit), 0o600); err != nil {
		return fmt.Errorf("write system service: %w", err)
	}

	if err := runSystemctlSystem("daemon-reload"); err != nil {
		return err
	}
	switch {
	case opts.Enable && opts.Start:
		return runSystemctlSystem("enable", "--now", "sentinel")
	case opts.Enable:
		return runSystemctlSystem("enable", "sentinel")
	case opts.Start:
		return runSystemctlSystem("start", "sentinel")
	default:
		return nil
	}
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
	}
	return runSystemctlSystem("daemon-reload")
}

func userStatusSystemLinux() (UserServiceStatus, error) {
	st := UserServiceStatus{
		ServicePath: systemUnitPath,
	}
	if info, statErr := os.Stat(systemUnitPath); statErr == nil && !info.IsDir() {
		st.UnitFileExists = true
	}

	if _, err := exec.LookPath("systemctl"); err != nil {
		return st, nil
	}
	st.SystemctlAvailable = true
	st.EnabledState = readSystemctlSystemState("is-enabled", "sentinel")
	st.ActiveState = readSystemctlSystemState("is-active", "sentinel")
	return st, nil
}

func installSystemAutoUpdateLinux(execPath, serviceUnit, scope, onCalendar string, randomizedDelay time.Duration, enable, start bool) error {
	if os.Geteuid() != 0 {
		return errors.New("scope=system requires root privileges")
	}
	servicePath := systemAutoUpdateService
	timerPath := systemAutoUpdateTimer

	if err := os.MkdirAll(filepath.Dir(servicePath), 0o750); err != nil {
		return fmt.Errorf("create systemd system directory: %w", err)
	}

	serviceUnitText := renderUserAutoUpdateUnit(execPath, serviceUnit, scope)
	if err := os.WriteFile(servicePath, []byte(serviceUnitText), 0o600); err != nil {
		return fmt.Errorf("write updater system service: %w", err)
	}
	timerUnitText := renderUserAutoUpdateTimer(onCalendar, randomizedDelay)
	if err := os.WriteFile(timerPath, []byte(timerUnitText), 0o600); err != nil {
		return fmt.Errorf("write updater system timer: %w", err)
	}

	if err := runSystemctlSystem("daemon-reload"); err != nil {
		return err
	}
	switch {
	case enable && start:
		return runSystemctlSystem("enable", "--now", "sentinel-updater.timer")
	case enable:
		return runSystemctlSystem("enable", "sentinel-updater.timer")
	case start:
		return runSystemctlSystem("start", "sentinel-updater.timer")
	default:
		return nil
	}
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
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
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
	cmd := exec.Command("systemctl", args...)
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
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
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
	cmd := exec.Command("systemctl", args...)
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

func renderUserUnit(execPath string) string {
	return fmt.Sprintf(`[Unit]
Description=Sentinel - terminal workspace
Documentation=https://github.com/opus-domini/sentinel
StartLimitIntervalSec=60
StartLimitBurst=4

[Service]
Type=simple
ExecStart=%s
Restart=on-failure
RestartSec=2
KillMode=process
Environment=SENTINEL_LOG_LEVEL=info
Environment=HOME=%%h
Environment=TERM=xterm-256color
Environment=LANG=C.UTF-8
SystemCallArchitectures=native
NoNewPrivileges=true

[Install]
WantedBy=default.target
`, escapeSystemdExec(execPath))
}

func renderUserAutoUpdateUnit(execPath, serviceUnit, scope string) string {
	return fmt.Sprintf(`[Unit]
Description=Sentinel automatic updater
Documentation=https://github.com/opus-domini/sentinel
Wants=network-online.target
After=network-online.target

[Service]
Type=oneshot
ExecStart=%s update apply -restart=true -service=%s -systemd-scope=%s
Environment=SENTINEL_LOG_LEVEL=info
NoNewPrivileges=true

[Install]
WantedBy=default.target
`, escapeSystemdExec(execPath), serviceUnit, scope)
}

func renderUserAutoUpdateTimer(onCalendar string, randomizedDelay time.Duration) string {
	return fmt.Sprintf(`[Unit]
Description=Run Sentinel updater daily

[Timer]
OnCalendar=%s
RandomizedDelaySec=%s
Persistent=true
Unit=sentinel-updater.service

[Install]
WantedBy=timers.target
`, onCalendar, randomizedDelay.String())
}

func escapeSystemdExec(path string) string {
	path = strings.ReplaceAll(path, "\\", "\\\\")
	return strings.ReplaceAll(path, " ", "\\x20")
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
