//go:build linux

package term

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

type PTY struct {
	master    *os.File
	cmd       *exec.Cmd
	closeOnce sync.Once
}

func StartTmuxAttach(ctx context.Context, session string, cols, rows int) (*PTY, error) {
	cmd := exec.CommandContext(ctx, "tmux", "attach", "-t", session)
	return startCommand(ctx, cmd, cols, rows)
}

func StartShell(ctx context.Context, requestedShell string, cols, rows int) (*PTY, error) {
	shellPath, err := resolveShell(requestedShell)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, shellPath)
	return startCommand(ctx, cmd, cols, rows)
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

func startCommand(ctx context.Context, cmd *exec.Cmd, cols, rows int) (*PTY, error) {
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

func (p *PTY) Wait() error {
	if p.cmd == nil {
		return nil
	}
	return p.cmd.Wait()
}

func (p *PTY) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return errors.New("invalid terminal dimensions")
	}
	return setWinsize(p.master.Fd(), cols, rows)
}

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

func openPTY() (master *os.File, slave *os.File, outErr error) {
	masterFD, err := syscall.Open("/dev/ptmx", syscall.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open /dev/ptmx: %w", err)
	}

	closeMaster := true
	defer func() {
		if closeMaster {
			_ = syscall.Close(masterFD)
		}
	}()

	var unlock int32
	if err := ioctl(masterFD, syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); err != nil {
		return nil, nil, fmt.Errorf("unlock ptmx: %w", err)
	}

	var ptyNum uint32
	if err := ioctl(masterFD, syscall.TIOCGPTN, uintptr(unsafe.Pointer(&ptyNum))); err != nil {
		return nil, nil, fmt.Errorf("read pty number: %w", err)
	}

	slaveName := fmt.Sprintf("/dev/pts/%d", ptyNum)
	slaveFD, err := syscall.Open(slaveName, syscall.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open slave pty %s: %w", slaveName, err)
	}

	closeMaster = false
	master = os.NewFile(uintptr(masterFD), "/dev/ptmx")
	slave = os.NewFile(uintptr(slaveFD), slaveName)
	return master, slave, nil
}

func setWinsize(fd uintptr, cols, rows int) error {
	ws := struct {
		Rows uint16
		Cols uint16
		X    uint16
		Y    uint16
	}{
		Rows: uint16(rows),
		Cols: uint16(cols),
	}
	return ioctl(int(fd), syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&ws)))
}

func ioctl(fd int, cmd uintptr, ptr uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), cmd, ptr)
	if errno != 0 {
		return errno
	}
	return nil
}

// ensureEnv adds essential variables when they are missing from the
// environment.  This is necessary when Sentinel runs as a systemd service
// where the inherited environment is intentionally minimal.
func ensureEnv(env []string) []string {
	defaults := [][2]string{
		{"TERM", "xterm-256color"},
		{"LANG", "C.UTF-8"},
	}
	for _, kv := range defaults {
		if !hasEnvKey(env, kv[0]) {
			env = append(env, kv[0]+"="+kv[1])
		}
	}
	return env
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
