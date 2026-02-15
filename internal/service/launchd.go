package service

import (
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
)

func installUserLaunchd(opts InstallUserOptions) error {
	if err := ensureLaunchdUserSupported(); err != nil {
		return err
	}

	execPath, err := resolveExecPath(opts.ExecPath)
	if err != nil {
		return err
	}

	servicePath, err := userServicePathLaunchd()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(servicePath), 0o750); err != nil {
		return fmt.Errorf("create launchd user directory: %w", err)
	}

	stdoutPath, stderrPath, err := launchdLogPaths("sentinel")
	if err != nil {
		return err
	}
	plist := renderLaunchdUserServicePlist(execPath, stdoutPath, stderrPath)
	if err := os.WriteFile(servicePath, []byte(plist), 0o600); err != nil {
		return fmt.Errorf("write launchd service plist: %w", err)
	}

	if opts.Enable || opts.Start {
		_ = launchdBootout(launchdServiceLabel)
		if err := launchdBootstrap(servicePath, launchdServiceLabel); err != nil {
			return err
		}
	}
	if opts.Start {
		if err := launchdKickstart(launchdServiceLabel); err != nil {
			return err
		}
	}
	return nil
}

func installUserAutoUpdateLaunchd(opts InstallUserAutoUpdateOptions) error {
	if err := ensureLaunchdUserSupported(); err != nil {
		return err
	}

	execPath, err := resolveExecPath(opts.ExecPath)
	if err != nil {
		return err
	}

	serviceLabel, err := launchdLabelFromServiceUnit(opts.ServiceUnit)
	if err != nil {
		return err
	}
	interval, err := launchdStartInterval(opts.OnCalendar)
	if err != nil {
		return err
	}
	scope := strings.ToLower(strings.TrimSpace(opts.SystemdScope))
	switch scope {
	case "", managerScopeLaunchd, managerScopeUser:
		// Accept "user" for CLI parity with Linux defaults.
	case managerScopeSystem:
		return errors.New("launchd autoupdate does not support system scope")
	default:
		return fmt.Errorf("invalid launchd scope: %s", opts.SystemdScope)
	}

	updaterPath, err := userAutoUpdatePathLaunchd()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(updaterPath), 0o750); err != nil {
		return fmt.Errorf("create launchd user directory: %w", err)
	}

	stdoutPath, stderrPath, err := launchdLogPaths("sentinel-updater")
	if err != nil {
		return err
	}
	// launchd does not provide a direct RandomizedDelaySec equivalent.
	_ = opts.RandomizedDelay

	plist := renderLaunchdUserAutoUpdatePlist(execPath, serviceLabel, interval, stdoutPath, stderrPath)
	if err := os.WriteFile(updaterPath, []byte(plist), 0o600); err != nil {
		return fmt.Errorf("write launchd autoupdate plist: %w", err)
	}

	if opts.Enable || opts.Start {
		_ = launchdBootout(launchdAutoUpdateLabel)
		if err := launchdBootstrap(updaterPath, launchdAutoUpdateLabel); err != nil {
			return err
		}
	}
	if opts.Start {
		if err := launchdKickstart(launchdAutoUpdateLabel); err != nil {
			return err
		}
	}
	return nil
}

func uninstallUserLaunchd(opts UninstallUserOptions) error {
	if err := ensureLaunchdUserSupported(); err != nil {
		return err
	}

	if opts.Disable || opts.Stop {
		_ = launchdBootout(launchdServiceLabel)
	}

	if opts.RemoveUnit {
		servicePath, err := userServicePathLaunchd()
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
	if err := ensureLaunchdUserSupported(); err != nil {
		return err
	}

	if opts.Disable || opts.Stop {
		_ = launchdBootout(launchdAutoUpdateLabel)
	}

	if opts.RemoveUnit {
		updaterPath, err := userAutoUpdatePathLaunchd()
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
	servicePath, err := userServicePathLaunchd()
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
	loaded, active, _ := readLaunchdJobState(launchdServiceLabel)
	if loaded {
		st.EnabledState = "loaded"
		st.ActiveState = active
	} else {
		st.EnabledState = "not-loaded"
		st.ActiveState = launchdStateInactive
	}
	return st, nil
}

func userAutoUpdateStatusLaunchd() (UserAutoUpdateServiceStatus, error) {
	updaterPath, err := userAutoUpdatePathLaunchd()
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
	loaded, active, lastRun := readLaunchdJobState(launchdAutoUpdateLabel)
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
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdServicePlistName), nil
}

func userAutoUpdatePathLaunchd() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdUpdaterPlistName), nil
}

func launchdLogPaths(baseName string) (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("resolve home dir: %w", err)
	}
	logDir := filepath.Join(home, ".sentinel", "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return "", "", fmt.Errorf("create sentinel log directory: %w", err)
	}
	return filepath.Join(logDir, baseName+".out.log"), filepath.Join(logDir, baseName+".err.log"), nil
}

func ensureLaunchdUserSupported() error {
	if runtime.GOOS != launchdSupportedOS {
		return errors.New("launchd user service commands are supported on macOS only")
	}
	if _, err := exec.LookPath("launchctl"); err != nil {
		return errors.New("launchctl was not found in PATH")
	}
	return nil
}

func launchdBootstrap(plistPath, label string) error {
	if err := runLaunchctl("bootstrap", launchdDomainTarget(), plistPath); err != nil {
		loaded, _, _ := readLaunchdJobState(label)
		if loaded {
			return nil
		}
		return err
	}
	return nil
}

func launchdBootout(label string) error {
	if err := runLaunchctl("bootout", launchdJobTarget(label)); err != nil {
		loaded, _, _ := readLaunchdJobState(label)
		if !loaded {
			return nil
		}
		return err
	}
	return nil
}

func launchdKickstart(label string) error {
	return runLaunchctl("kickstart", "-k", launchdJobTarget(label))
}

func launchdDomainTarget() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func launchdJobTarget(label string) string {
	return launchdDomainTarget() + "/" + label
}

func runLaunchctl(args ...string) error {
	cmd := exec.Command("launchctl", args...)
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
	cmd := exec.Command("launchctl", args...)
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

func readLaunchdJobState(label string) (loaded bool, active string, lastRun string) {
	out, err := runLaunchctlOutput("print", launchdJobTarget(label))
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

func renderLaunchdUserAutoUpdatePlist(execPath, serviceLabel string, intervalSeconds int, stdoutPath, stderrPath string) string {
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
		<string>-systemd-scope=launchd</string>
	</array>
	<key>StartInterval</key>
	<integer>%d</integer>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, xmlEscape(launchdAutoUpdateLabel), xmlEscape(execPath), xmlEscape(serviceLabel), intervalSeconds, xmlEscape(stdoutPath), xmlEscape(stderrPath))
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
