//go:build darwin

package term

import (
	"bytes"
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
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open /dev/ptmx: %w", err)
	}
	defer func() {
		if outErr != nil {
			_ = master.Close()
		}
	}()

	if err := ptyGrant(master); err != nil {
		return nil, nil, err
	}
	if err := ptyUnlock(master); err != nil {
		return nil, nil, err
	}

	slaveName, err := ptyName(master)
	if err != nil {
		return nil, nil, err
	}

	slave, err = os.OpenFile(slaveName, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open slave pty %s: %w", slaveName, err)
	}
	return master, slave, nil
}

func ptyName(master *os.File) (string, error) {
	// Parameter length is encoded in the low 13 bits of the top word.
	const iocParmMask = 0x1FFF
	paramLen := (syscall.TIOCPTYGNAME >> 16) & iocParmMask
	if paramLen <= 0 {
		paramLen = 128
	}
	out := make([]byte, paramLen)

	if err := ioctl(master.Fd(), "TIOCPTYGNAME", uintptr(syscall.TIOCPTYGNAME), uintptr(unsafe.Pointer(&out[0]))); err != nil {
		return "", err
	}

	end := bytes.IndexByte(out, 0x00)
	if end < 0 {
		end = len(out)
	}
	name := string(out[:end])
	if strings.TrimSpace(name) == "" {
		return "", errors.New("empty pty slave path")
	}
	return name, nil
}

func ptyGrant(master *os.File) error {
	return ioctl(master.Fd(), "TIOCPTYGRANT", uintptr(syscall.TIOCPTYGRANT), 0)
}

func ptyUnlock(master *os.File) error {
	return ioctl(master.Fd(), "TIOCPTYUNLK", uintptr(syscall.TIOCPTYUNLK), 0)
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
	return ioctl(fd, "TIOCSWINSZ", uintptr(syscall.TIOCSWINSZ), uintptr(unsafe.Pointer(&ws)))
}

func ioctl(fd uintptr, name string, cmd uintptr, ptr uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, cmd, ptr)
	if errno != 0 {
		return fmt.Errorf("%s ioctl failed: %w", name, errno)
	}
	return nil
}

// ensureEnv adds essential variables when they are missing from the
// environment. This is necessary when Sentinel runs as a service where the
// inherited environment is intentionally minimal.
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
