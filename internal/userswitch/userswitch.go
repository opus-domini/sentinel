// Package userswitch builds commands for executing as another user.
package userswitch

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const (
	// MethodSudo switches users through sudo.
	MethodSudo = "sudo"
	// MethodSystemdRun switches users through systemd-run.
	MethodSystemdRun = "systemd-run"
)

// Capability reports whether a user switch method has the required local
// executables. It does not verify sudo policy or execute any command.
type Capability struct {
	Method    string
	Available bool
	Detail    string
}

// execSudo and execTmux are the binaries used to launch commands as another user.
const (
	execSudo       = "sudo"
	execSystemdRun = "systemd-run"
	execTmux       = "tmux"
)

// Capabilities detects the local executables required by each supported method.
func Capabilities() []Capability {
	return capabilitiesWithLookPath(exec.LookPath)
}

func capabilitiesWithLookPath(lookPath func(string) (string, error)) []Capability {
	_, sudoErr := lookPath(execSudo)
	_, systemdRunErr := lookPath(execSystemdRun)
	sudoAvailable := sudoErr == nil
	systemdRunAvailable := sudoAvailable && systemdRunErr == nil

	sudoDetail := "sudo is installed; passwordless policy must still be configured by the operator"
	if !sudoAvailable {
		sudoDetail = "sudo was not found in PATH"
	}
	systemdRunDetail := "sudo and systemd-run are installed; passwordless policy must still be configured by the operator"
	switch {
	case !sudoAvailable:
		systemdRunDetail = "sudo was not found in PATH"
	case systemdRunErr != nil:
		systemdRunDetail = "systemd-run was not found in PATH"
	}

	return []Capability{
		{Method: MethodSudo, Available: sudoAvailable, Detail: sudoDetail},
		{Method: MethodSystemdRun, Available: systemdRunAvailable, Detail: systemdRunDetail},
	}
}

// DefaultMethod returns the default method.
func DefaultMethod(goos string) string {
	if goos == "linux" {
		return MethodSystemdRun
	}
	return MethodSudo
}

// ParseMethod validates and canonicalizes a configured switch method.
func ParseMethod(method string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case MethodSudo:
		return MethodSudo, nil
	case MethodSystemdRun:
		return MethodSystemdRun, nil
	default:
		return "", fmt.Errorf("must be %q or %q", MethodSudo, MethodSystemdRun)
	}
}

// BuildTmuxCommand builds tmux command.
func BuildTmuxCommand(method, user string, tmuxArgs []string, interactive bool) (string, []string, error) {
	user = strings.TrimSpace(user)
	args := append([]string{}, tmuxArgs...)
	if user == "" {
		return execTmux, args, nil
	}

	canonicalMethod, err := ParseMethod(method)
	if err != nil {
		return "", nil, fmt.Errorf("invalid user switch method: %w", err)
	}
	switch canonicalMethod {
	case MethodSudo:
		return execSudo, append([]string{"-n", "-u", user, execTmux}, args...), nil
	case MethodSystemdRun:
		return execSudo, append(systemdRunPrefix(user, interactive), append([]string{execTmux}, args...)...), nil
	default:
		return "", nil, errors.New("invalid user switch method")
	}
}

// BuildShellCommand builds shell command.
func BuildShellCommand(method, user, command string) (string, error) {
	user = strings.TrimSpace(user)
	if user == "" {
		return strings.TrimSpace(command), nil
	}

	var args []string
	canonicalMethod, err := ParseMethod(method)
	if err != nil {
		return "", fmt.Errorf("invalid user switch method: %w", err)
	}
	switch canonicalMethod {
	case MethodSudo:
		if strings.TrimSpace(command) == "" {
			args = []string{execSudo, "-n", "-i", "-u", user}
		} else {
			args = []string{execSudo, "-n", "-u", user, "/bin/sh", "-lc", command}
		}
	case MethodSystemdRun:
		args = append([]string{execSudo}, systemdRunPrefix(user, true)...)
		args = append(args, "--same-dir")
		if strings.TrimSpace(command) == "" {
			args = append(args, "--shell")
		} else {
			args = append(args, "/bin/sh", "-lc", command)
		}
	default:
		return "", errors.New("invalid user switch method")
	}

	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " "), nil
}

func systemdRunPrefix(user string, interactive bool) []string {
	args := []string{
		"-n",
		"systemd-run",
		"--user",
		"--machine=" + user + "@.host",
		"--collect",
		"--quiet",
		"--service-type=exec",
		"--expand-environment=no",
	}
	if interactive {
		return append(args, "--wait", "--pipe")
	}
	// tmux new-session -d starts a server process after the client exits.
	// Keep systemd from tearing that child process down with the transient unit.
	return append(args, "--property=KillMode=process", "--wait", "--pipe")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
