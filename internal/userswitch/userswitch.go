package userswitch

import (
	"errors"
	"strings"
)

const (
	MethodSudo       = "sudo"
	MethodSystemdRun = "systemd-run"
)

// execSudo and execTmux are the binaries used to launch commands as another user.
const (
	execSudo = "sudo"
	execTmux = "tmux"
)

func DefaultMethod(goos string) string {
	if goos == "linux" {
		return MethodSystemdRun
	}
	return MethodSudo
}

func NormalizeMethod(method, fallback string) string {
	if fallback == "" {
		fallback = MethodSudo
	}
	switch strings.ToLower(strings.TrimSpace(method)) {
	case MethodSudo:
		return MethodSudo
	case MethodSystemdRun, "systemd":
		return MethodSystemdRun
	default:
		return fallback
	}
}

func BuildTmuxCommand(method, user string, tmuxArgs []string, interactive bool) (string, []string, error) {
	user = strings.TrimSpace(user)
	args := append([]string{}, tmuxArgs...)
	if user == "" {
		return execTmux, args, nil
	}

	switch NormalizeMethod(method, MethodSudo) {
	case MethodSudo:
		return execSudo, append([]string{"-n", "-u", user, execTmux}, args...), nil
	case MethodSystemdRun:
		return execSudo, append(systemdRunPrefix(user, interactive), append([]string{execTmux}, args...)...), nil
	default:
		return "", nil, errors.New("invalid user switch method")
	}
}

func BuildShellCommand(method, user, command string) (string, error) {
	user = strings.TrimSpace(user)
	if user == "" {
		return strings.TrimSpace(command), nil
	}

	var args []string
	switch NormalizeMethod(method, MethodSudo) {
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
