//go:build darwin || linux

package term

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/opus-domini/sentinel/internal/userswitch"
)

// PTY represents PTY data.
type PTY struct {
	master    *os.File
	cmd       *exec.Cmd
	closeOnce sync.Once
}

// UserSwitchMethod controls how multi-user tmux attach commands are launched.
// Set from main.go after config.Load().
var UserSwitchMethod = defaultUserSwitchMethod // set once at startup from config

// StartTmuxAttach starts tmux attach.
func StartTmuxAttach(ctx context.Context, session string, cols, rows int) (*PTY, error) {
	cmd := exec.CommandContext(ctx, "tmux", tmuxAttachArgs(session)...)
	return startCommand(cmd, cols, rows)
}

// StartTmuxAttachAsUser wraps the tmux attach command using the configured
// user switch method. When user is empty, it falls back to StartTmuxAttach.
func StartTmuxAttachAsUser(ctx context.Context, session, user string, cols, rows int) (*PTY, error) {
	if user == "" {
		return StartTmuxAttach(ctx, session, cols, rows)
	}
	name, args, err := buildTmuxAttachCommandAsUser(session, user)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, name, args...)
	return startCommand(cmd, cols, rows)
}

func buildTmuxAttachCommandAsUser(session, user string) (string, []string, error) {
	return userswitch.BuildTmuxCommand(UserSwitchMethod, user, tmuxAttachArgs(session), true)
}

// StartShell starts shell.
func StartShell(ctx context.Context, requestedShell string, cols, rows int) (*PTY, error) {
	shellPath, err := resolveShell(requestedShell)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, shellPath)
	return startCommand(cmd, cols, rows)
}

func resolveShell(requestedShell string) (string, error) {
	candidates := make([]string, 0, 8)
	if shell := strings.TrimSpace(requestedShell); shell != "" {
		candidates = append(candidates, shell)
	}
	if shell := strings.TrimSpace(os.Getenv("SHELL")); shell != "" {
		candidates = append(candidates, shell)
	}
	candidates = append(candidates,
		"/bin/zsh",
		"/usr/bin/zsh",
		"/bin/bash",
		"/usr/bin/bash",
		"/bin/sh",
		"/usr/bin/sh",
	)

	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate)
		if err == nil {
			return path, nil
		}
	}

	return "", errors.New("no interactive shell found on host")
}

func startCommand(cmd *exec.Cmd, cols, rows int) (*PTY, error) {
	master, slave, err := openPTY()
	if err != nil {
		return nil, err
	}

	if cols > 0 && rows > 0 {
		if err := setWinsize(master.Fd(), cols, rows); err != nil {
			_ = master.Close()
			_ = slave.Close()
			return nil, err
		}
	}

	cmd.Env = ensureEnv(os.Environ())
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0,
	}

	if err := cmd.Start(); err != nil {
		_ = master.Close()
		_ = slave.Close()
		return nil, err
	}
	_ = slave.Close()

	return &PTY{
		master: master,
		cmd:    cmd,
	}, nil
}

func (p *PTY) Read(dst []byte) (int, error) {
	return p.master.Read(dst)
}

func (p *PTY) Write(src []byte) (int, error) {
	return p.master.Write(src)
}

// Wait handles wait.
func (p *PTY) Wait() error {
	if p.cmd == nil {
		return nil
	}
	return p.cmd.Wait()
}

// Resize handles resize.
func (p *PTY) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return errors.New("invalid terminal dimensions")
	}
	return setWinsize(p.master.Fd(), cols, rows)
}

// Close closes value.
func (p *PTY) Close() error {
	var outErr error
	p.closeOnce.Do(func() {
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
		outErr = p.master.Close()
	})
	return outErr
}

// ensureEnv adds essential variables when they are missing or unusable from the
// environment. This is necessary when Sentinel runs as a systemd service or
// test harness where the inherited environment is intentionally minimal.
func ensureEnv(env []string) []string {
	env = ensureEnvValue(env, "TERM", "xterm-256color", usableTerminalType)
	env = ensureEnvValue(env, "LANG", "C.UTF-8", nonEmptyEnvValue)
	return env
}

func ensureEnvValue(env []string, key, fallback string, usable func(string) bool) []string {
	prefix := key + "="
	for index, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			continue
		}
		if usable(strings.TrimPrefix(entry, prefix)) {
			return env
		}
		env[index] = prefix + fallback
		return env
	}
	return append(env, prefix+fallback)
}

func usableTerminalType(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && !strings.EqualFold(trimmed, "dumb")
}

func nonEmptyEnvValue(value string) bool {
	return strings.TrimSpace(value) != ""
}

func hasEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}

var _ io.ReadWriteCloser = (*PTY)(nil)
