package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	launchdSupportedOS      = "darwin"
	launchdServiceLabel     = "io.opusdomini.sentinel"
	launchdAutoUpdateLabel  = "io.opusdomini.sentinel.updater"
	launchdServicePlistName = launchdServiceLabel + ".plist"
	launchdUpdaterPlistName = launchdAutoUpdateLabel + ".plist"
	launchdStateInactive    = "inactive"

	launchdSystemServicePath = "/Library/LaunchDaemons/" + launchdServicePlistName
	launchdSystemUpdaterPath = "/Library/LaunchDaemons/" + launchdUpdaterPlistName
	launchdSystemLogDir      = "/var/log/sentinel"
)

type launchdAutoUpdateInstallConfig struct {
	scope        string
	execPath     string
	serviceLabel string
	interval     int
	updaterPath  string
	stdoutPath   string
	stderrPath   string
}

func installUserLaunchd(opts InstallUserOptions) error {
	if err := ensureLaunchdSupported(); err != nil {
		return err
	}

	scope, err := normalizeLaunchdScope(managerScopeAuto)
	if err != nil {
		return err
	}
	if err := ensureLaunchdScopePrivileges(scope); err != nil {
		return err
	}

	execPath, err := resolveExecPath(opts.ExecPath)
	if err != nil {
		return err
	}

	servicePath, err := userServicePathLaunchdForScope(scope)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(servicePath), 0o750); err != nil {
		return fmt.Errorf("create launchd directory: %w", err)
	}

	stdoutPath, stderrPath, err := launchdLogPathsForScope("sentinel", scope)
	if err != nil {
		return err
	}
	plist := renderLaunchdUserServicePlist(execPath, stdoutPath, stderrPath)
	if err := os.WriteFile(servicePath, []byte(plist), launchdUnitFileMode(scope)); err != nil {
		return fmt.Errorf("write launchd service plist: %w", err)
	}

	if opts.Enable || opts.Start {
		_ = launchdBootout(scope, launchdServiceLabel)
		if err := launchdBootstrap(scope, servicePath, launchdServiceLabel); err != nil {
			return err
		}
	}
	if opts.Start {
		if err := launchdKickstart(scope, launchdServiceLabel); err != nil {
			return err
		}
	}
	return nil
}

func installUserAutoUpdateLaunchd(opts InstallUserAutoUpdateOptions) error {
	if err := ensureLaunchdSupported(); err != nil {
		return err
	}

	cfg, err := resolveLaunchdAutoUpdateInstallConfig(opts)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.updaterPath), 0o750); err != nil {
		return fmt.Errorf("create launchd directory: %w", err)
	}
	if err := writeLaunchdAutoUpdatePlist(cfg); err != nil {
		return err
	}

	if opts.Enable || opts.Start {
		_ = launchdBootout(cfg.scope, launchdAutoUpdateLabel)
		if err := launchdBootstrap(cfg.scope, cfg.updaterPath, launchdAutoUpdateLabel); err != nil {
			return err
		}
	}
	if opts.Start {
		if err := launchdKickstart(cfg.scope, launchdAutoUpdateLabel); err != nil {
			return err
		}
	}
	return nil
}

func resolveLaunchdAutoUpdateInstallConfig(opts InstallUserAutoUpdateOptions) (launchdAutoUpdateInstallConfig, error) {
	scope, err := normalizeLaunchdScope(opts.SystemdScope)
	if err != nil {
		return launchdAutoUpdateInstallConfig{}, err
	}
	if err := ensureLaunchdScopePrivileges(scope); err != nil {
		return launchdAutoUpdateInstallConfig{}, err
	}

	execPath, err := resolveExecPath(opts.ExecPath)
	if err != nil {
		return launchdAutoUpdateInstallConfig{}, err
	}
	serviceLabel, err := launchdLabelFromServiceUnit(opts.ServiceUnit)
	if err != nil {
		return launchdAutoUpdateInstallConfig{}, err
	}
	interval, err := launchdStartInterval(opts.OnCalendar)
	if err != nil {
		return launchdAutoUpdateInstallConfig{}, err
	}
	updaterPath, err := userAutoUpdatePathLaunchdForScope(scope)
	if err != nil {
		return launchdAutoUpdateInstallConfig{}, err
	}
	stdoutPath, stderrPath, err := launchdLogPathsForScope("sentinel-updater", scope)
	if err != nil {
		return launchdAutoUpdateInstallConfig{}, err
	}

	// launchd does not provide a direct RandomizedDelaySec equivalent.
	_ = opts.RandomizedDelay

	return launchdAutoUpdateInstallConfig{
		scope:        scope,
		execPath:     execPath,
		serviceLabel: serviceLabel,
		interval:     interval,
		updaterPath:  updaterPath,
		stdoutPath:   stdoutPath,
		stderrPath:   stderrPath,
	}, nil
}

func writeLaunchdAutoUpdatePlist(cfg launchdAutoUpdateInstallConfig) error {
	plist := renderLaunchdUserAutoUpdatePlist(
		cfg.execPath,
		cfg.serviceLabel,
		cfg.scope,
		cfg.interval,
		cfg.stdoutPath,
		cfg.stderrPath,
	)
	if err := os.WriteFile(cfg.updaterPath, []byte(plist), launchdUnitFileMode(cfg.scope)); err != nil {
		return fmt.Errorf("write launchd autoupdate plist: %w", err)
	}
	return nil
}

func uninstallUserLaunchd(opts UninstallUserOptions) error {
	if err := ensureLaunchdSupported(); err != nil {
		return err
	}

	scope, err := normalizeLaunchdScope(managerScopeAuto)
	if err != nil {
		return err
	}
	if err := ensureLaunchdScopePrivileges(scope); err != nil {
		return err
	}

	if opts.Disable || opts.Stop {
		_ = launchdBootout(scope, launchdServiceLabel)
	}

	if opts.RemoveUnit {
		servicePath, err := userServicePathLaunchdForScope(scope)
		if err != nil {
			return err
		}
		if err := os.Remove(servicePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove launchd service plist: %w", err)
		}
	}
	return nil
}

func uninstallUserAutoUpdateLaunchd(opts UninstallUserAutoUpdateOptions) error {
	if err := ensureLaunchdSupported(); err != nil {
		return err
	}

	scope, err := normalizeLaunchdScope(opts.Scope)
	if err != nil {
		return err
	}
	if err := ensureLaunchdScopePrivileges(scope); err != nil {
		return err
	}

	if opts.Disable || opts.Stop {
		_ = launchdBootout(scope, launchdAutoUpdateLabel)
	}

	if opts.RemoveUnit {
		updaterPath, err := userAutoUpdatePathLaunchdForScope(scope)
		if err != nil {
			return err
		}
		if err := os.Remove(updaterPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove launchd autoupdate plist: %w", err)
		}
	}
	return nil
}

func userStatusLaunchd() (UserServiceStatus, error) {
	return userStatusLaunchdForScope("")
}

func userStatusLaunchdForScope(scopeRaw string) (UserServiceStatus, error) {
	scope, err := normalizeLaunchdScope(scopeRaw)
	if err != nil {
		return UserServiceStatus{}, err
	}

	servicePath, err := userServicePathLaunchdForScope(scope)
	if err != nil {
		return UserServiceStatus{}, err
	}

	st := UserServiceStatus{
		ServicePath: servicePath,
	}
	if info, statErr := os.Stat(servicePath); statErr == nil && !info.IsDir() {
		st.UnitFileExists = true
	}
	if runtime.GOOS != launchdSupportedOS {
		return st, nil
	}
	if _, err := exec.LookPath("launchctl"); err != nil {
		return st, nil
	}

	st.SystemctlAvailable = true
	loaded, active, _ := readLaunchdJobState(scope, launchdServiceLabel)
	if loaded {
		st.EnabledState = "loaded"
		st.ActiveState = active
	} else {
		st.EnabledState = "not-loaded"
		st.ActiveState = launchdStateInactive
	}
	return st, nil
}

func userAutoUpdateStatusLaunchdForScope(scopeRaw string) (UserAutoUpdateServiceStatus, error) {
	scope, err := normalizeLaunchdScope(scopeRaw)
	if err != nil {
		return UserAutoUpdateServiceStatus{}, err
	}

	updaterPath, err := userAutoUpdatePathLaunchdForScope(scope)
	if err != nil {
		return UserAutoUpdateServiceStatus{}, err
	}

	st := UserAutoUpdateServiceStatus{
		ServicePath: updaterPath,
		TimerPath:   updaterPath,
	}
	if info, statErr := os.Stat(updaterPath); statErr == nil && !info.IsDir() {
		st.ServiceUnitExists = true
		st.TimerUnitExists = true
	}
	if runtime.GOOS != launchdSupportedOS {
		return st, nil
	}
	if _, err := exec.LookPath("launchctl"); err != nil {
		return st, nil
	}

	st.SystemctlAvailable = true
	loaded, active, lastRun := readLaunchdJobState(scope, launchdAutoUpdateLabel)
	if loaded {
		st.TimerEnabledState = "loaded"
		st.TimerActiveState = active
	} else {
		st.TimerEnabledState = "not-loaded"
		st.TimerActiveState = launchdStateInactive
	}
	st.LastRunState = lastRun
	return st, nil
}

func userServicePathLaunchd() (string, error) {
	return userServicePathLaunchdForScope("")
}

func userServicePathLaunchdForScope(scopeRaw string) (string, error) {
	scope, err := normalizeLaunchdScope(scopeRaw)
	if err != nil {
		return "", err
	}
	if scope == managerScopeSystem {
		return launchdSystemServicePath, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdServicePlistName), nil
}

func userAutoUpdatePathLaunchdForScope(scopeRaw string) (string, error) {
	scope, err := normalizeLaunchdScope(scopeRaw)
	if err != nil {
		return "", err
	}
	if scope == managerScopeSystem {
		return launchdSystemUpdaterPath, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdUpdaterPlistName), nil
}

func launchdLogPathsForScope(baseName, scopeRaw string) (string, string, error) {
	scope, err := normalizeLaunchdScope(scopeRaw)
	if err != nil {
		return "", "", err
	}

	var logDir string
	if scope == managerScopeSystem {
		logDir = launchdSystemLogDir
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", fmt.Errorf("resolve home dir: %w", err)
		}
		logDir = filepath.Join(home, ".sentinel", "logs")
	}
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return "", "", fmt.Errorf("create sentinel log directory: %w", err)
	}
	return filepath.Join(logDir, baseName+".out.log"), filepath.Join(logDir, baseName+".err.log"), nil
}

func ensureLaunchdSupported() error {
	if runtime.GOOS != launchdSupportedOS {
		return errors.New("launchd service commands are supported on macOS only")
	}
	if _, err := exec.LookPath("launchctl"); err != nil {
		return errors.New("launchctl was not found in PATH")
	}
	return nil
}

func ensureLaunchdScopePrivileges(scope string) error {
	if scope == managerScopeSystem && os.Geteuid() != 0 {
		return errors.New("scope=system requires root privileges")
	}
	return nil
}

func normalizeLaunchdScope(raw string) (string, error) {
	scope := strings.ToLower(strings.TrimSpace(raw))
	switch scope {
	case "", managerScopeAuto, managerScopeLaunchd:
		if os.Geteuid() == 0 {
			return managerScopeSystem, nil
		}
		return managerScopeUser, nil
	case managerScopeUser:
		return managerScopeUser, nil
	case managerScopeSystem:
		return managerScopeSystem, nil
	default:
		return "", fmt.Errorf("invalid launchd scope: %s", raw)
	}
}

func launchdUnitFileMode(scope string) os.FileMode {
	if scope == managerScopeSystem {
		return 0o644
	}
	return 0o600
}

func launchdBootstrap(scope, plistPath, label string) error {
	if err := runLaunchctl("bootstrap", launchdDomainTarget(scope), plistPath); err != nil {
		loaded, _, _ := readLaunchdJobState(scope, label)
		if loaded {
			return nil
		}
		return err
	}
	return nil
}

func launchdBootout(scope, label string) error {
	if err := runLaunchctl("bootout", launchdJobTarget(scope, label)); err != nil {
		loaded, _, _ := readLaunchdJobState(scope, label)
		if !loaded {
			return nil
		}
		return err
	}
	return nil
}

func launchdKickstart(scope, label string) error {
	return runLaunchctl("kickstart", "-k", launchdJobTarget(scope, label))
}

func launchdDomainTarget(scope string) string {
	if scope == managerScopeSystem {
		return managerScopeSystem
	}
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func launchdJobTarget(scope, label string) string {
	return launchdDomainTarget(scope) + "/" + label
}

func runLaunchctl(args ...string) error {
	cmd := exec.CommandContext(context.Background(), "launchctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("launchctl %s failed: %w", strings.Join(args, " "), err)
		}
		return fmt.Errorf("launchctl %s failed: %s", strings.Join(args, " "), msg)
	}
	return nil
}

func runLaunchctlOutput(args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "launchctl", args...)
	out, err := cmd.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		if msg == "" {
			return "", fmt.Errorf("launchctl %s failed: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("launchctl %s failed: %s", strings.Join(args, " "), msg)
	}
	return msg, nil
}

func readLaunchdJobState(scope, label string) (loaded bool, active string, lastRun string) {
	out, err := runLaunchctlOutput("print", launchdJobTarget(scope, label))
	if err != nil {
		return false, launchdStateInactive, "-"
	}

	active = launchdStateInactive
	if strings.Contains(strings.ToLower(out), "state = running") {
		active = "active"
	}
	lastRun = parseLaunchdLastRun(out)
	return true, active, lastRun
}

func parseLaunchdLastRun(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		lower := strings.ToLower(strings.TrimSpace(line))
		if !strings.Contains(lower, "last exit") {
			continue
		}
		if idx := strings.Index(line, "="); idx >= 0 {
			value := strings.TrimSpace(line[idx+1:])
			if value != "" {
				return value
			}
		}
	}
	return "-"
}

func launchdLabelFromServiceUnit(raw string) (string, error) {
	label := strings.TrimSpace(raw)
	if label == "" || label == "sentinel" {
		return launchdServiceLabel, nil
	}
	if strings.ContainsAny(label, "\n\r\t ") {
		return "", errors.New("invalid service unit name")
	}
	return label, nil
}

func launchdStartInterval(raw string) (int, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "daily":
		return 24 * 60 * 60, nil
	case "hourly":
		return 60 * 60, nil
	case "weekly":
		return 7 * 24 * 60 * 60, nil
	}

	if duration, err := time.ParseDuration(value); err == nil {
		seconds := int(duration.Seconds())
		if seconds > 0 {
			return seconds, nil
		}
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return seconds, nil
	}
	return 0, fmt.Errorf("invalid on-calendar value for launchd: %s", raw)
}

func renderLaunchdUserServicePlist(execPath, stdoutPath, stderrPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>serve</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
	<key>EnvironmentVariables</key>
	<dict>
		<key>SENTINEL_LOG_LEVEL</key>
		<string>info</string>
		<key>TERM</key>
		<string>xterm-256color</string>
	</dict>
</dict>
</plist>
`, xmlEscape(launchdServiceLabel), xmlEscape(execPath), xmlEscape(stdoutPath), xmlEscape(stderrPath))
}

func renderLaunchdUserAutoUpdatePlist(
	execPath,
	serviceLabel,
	restartScope string,
	intervalSeconds int,
	stdoutPath,
	stderrPath string,
) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>update</string>
		<string>apply</string>
		<string>-restart=true</string>
		<string>-service=%s</string>
		<string>-systemd-scope=%s</string>
	</array>
	<key>StartInterval</key>
	<integer>%d</integer>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, xmlEscape(launchdAutoUpdateLabel), xmlEscape(execPath), xmlEscape(serviceLabel), xmlEscape(restartScope), intervalSeconds, xmlEscape(stdoutPath), xmlEscape(stderrPath))
}

func xmlEscape(raw string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(raw)
}
