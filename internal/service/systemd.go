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
	userAutoUpdateServiceName = "sentinel-updater.service"
	userAutoUpdateTimerName   = "sentinel-updater.timer"
	systemdSupportedOS        = "linux"
	managerScopeUser          = "user"
	managerScopeSystem        = "system"
	managerScopeLaunchd       = "launchd"
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
	if err := ensureSystemdUserSupported(); err != nil {
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
	scope := strings.ToLower(strings.TrimSpace(opts.SystemdScope))
	switch scope {
	case "", managerScopeUser:
		scope = managerScopeUser
	case managerScopeSystem:
		// supported mostly for privileged/root timer setups.
	default:
		return fmt.Errorf("invalid systemd scope: %s", scope)
	}
	onCalendar := strings.TrimSpace(opts.OnCalendar)
	if onCalendar == "" {
		onCalendar = "daily"
	}
	randomizedDelay := opts.RandomizedDelay
	if randomizedDelay <= 0 {
		randomizedDelay = time.Hour
	}

	servicePath, err := UserAutoUpdateServicePath()
	if err != nil {
		return err
	}
	timerPath, err := UserAutoUpdateTimerPath()
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
		return err
	}
	switch {
	case opts.Enable && opts.Start:
		return runSystemctlUser("enable", "--now", "sentinel-updater.timer")
	case opts.Enable:
		return runSystemctlUser("enable", "sentinel-updater.timer")
	case opts.Start:
		return runSystemctlUser("start", "sentinel-updater.timer")
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

	return runSystemctlUser("daemon-reload")
}

func UserStatus() (UserServiceStatus, error) {
	if runtime.GOOS == launchdSupportedOS {
		return userStatusLaunchd()
	}
	if err := ensureServicePlatformSupported(); err != nil {
		return UserServiceStatus{}, err
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
	if runtime.GOOS == launchdSupportedOS {
		return userAutoUpdateStatusLaunchd()
	}
	if err := ensureServicePlatformSupported(); err != nil {
		return UserAutoUpdateServiceStatus{}, err
	}
	servicePath, err := UserAutoUpdateServicePath()
	if err != nil {
		return UserAutoUpdateServiceStatus{}, err
	}
	timerPath, err := UserAutoUpdateTimerPath()
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
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "systemd", "user", userUnitName), nil
}

func UserAutoUpdateServicePath() (string, error) {
	if runtime.GOOS == launchdSupportedOS {
		return userAutoUpdatePathLaunchd()
	}
	if err := ensureServicePlatformSupported(); err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "systemd", "user", userAutoUpdateServiceName), nil
}

func UserAutoUpdateTimerPath() (string, error) {
	if runtime.GOOS == launchdSupportedOS {
		// launchd runs timer semantics inside a single plist.
		return userAutoUpdatePathLaunchd()
	}
	if err := ensureServicePlatformSupported(); err != nil {
		return "", err
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

func readSystemctlState(args ...string) string {
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	out, err := cmd.CombinedOutput()
	state := strings.TrimSpace(string(out))
	if err != nil {
		normalized := strings.ToLower(state)
		switch {
		case strings.Contains(normalized, "failed to connect"):
			return "unavailable"
		case strings.Contains(normalized, "could not be found"):
			return "not-found"
		case state == "":
			return "unknown"
		case strings.Contains(state, "\n"):
			return "unknown"
		default:
			return state
		}
	}
	if state == "" {
		return "-"
	}
	return state
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
