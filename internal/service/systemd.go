package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	userUnitName = "sentinel.service"
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

type UserServiceStatus struct {
	ServicePath        string
	UnitFileExists     bool
	SystemctlAvailable bool
	EnabledState       string
	ActiveState        string
}

func InstallUser(opts InstallUserOptions) error {
	if err := ensureSystemdUserSupported(); err != nil {
		return err
	}

	execPath := strings.TrimSpace(opts.ExecPath)
	if execPath == "" {
		path, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolve executable path: %w", err)
		}
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			execPath = resolved
		} else {
			execPath = path
		}
	}
	if strings.Contains(execPath, "\n") || strings.Contains(execPath, "\r") {
		return errors.New("invalid executable path")
	}

	servicePath, err := UserServicePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(servicePath), 0o755); err != nil {
		return fmt.Errorf("create systemd user directory: %w", err)
	}

	unit := renderUserUnit(execPath)
	if err := os.WriteFile(servicePath, []byte(unit), 0o644); err != nil {
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

func UninstallUser(opts UninstallUserOptions) error {
	if err := ensureSystemdUserSupported(); err != nil {
		return err
	}

	if opts.Disable && opts.Stop {
		_ = runSystemctlUser("disable", "--now", "sentinel")
	} else if opts.Disable {
		_ = runSystemctlUser("disable", "sentinel")
	} else if opts.Stop {
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

func UserStatus() (UserServiceStatus, error) {
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

	if runtime.GOOS != "linux" {
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

func UserServicePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "systemd", "user", userUnitName), nil
}

func ensureSystemdUserSupported() error {
	if runtime.GOOS != "linux" {
		return errors.New("systemd user service commands are supported on Linux only")
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return errors.New("systemctl was not found in PATH")
	}
	return nil
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

func escapeSystemdExec(path string) string {
	path = strings.ReplaceAll(path, "\\", "\\\\")
	return strings.ReplaceAll(path, " ", "\\x20")
}
