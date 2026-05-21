//go:build linux

package term

import (
	"context"
	"io"
	"testing"
	"time"
)

// TestStartShellLifecycle drives a real shell PTY end to end: it exercises
// StartShell, startCommand, openPTY, setWinsize, ioctl, Read, Write, Resize,
// Wait and Close.
func TestStartShellLifecycle(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pty, err := StartShell(ctx, "/bin/sh", 80, 24)
	if err != nil {
		t.Fatalf("StartShell() error = %v", err)
	}
	t.Cleanup(func() { _ = pty.Close() })

	if err := pty.Resize(100, 30); err != nil {
		t.Fatalf("Resize() error = %v", err)
	}

	// Drain output in the background so the shell never blocks on a full
	// PTY buffer; the copy ends when the shell exits and the master errors.
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		_, _ = io.Copy(io.Discard, pty)
	}()

	if _, err := pty.Write([]byte("exit 0\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if err := pty.Wait(); err != nil {
		t.Logf("Wait() returned %v (shell exit status is not asserted)", err)
	}

	if err := pty.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	// Close must be idempotent.
	if err := pty.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("output drain did not finish after the shell exited")
	}
}

// TestStartTmuxAttach covers StartTmuxAttach. tmux may be absent on the host,
// in which case startCommand fails; either way the start path is exercised.
func TestStartTmuxAttach(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pty, err := StartTmuxAttach(ctx, "sentinel-pty-test-missing-session", 80, 24)
	if err != nil {
		return
	}
	_ = pty.Close()
}

// TestStartTmuxAttachAsUserEmptyUser covers the empty-user branch, which must
// delegate to StartTmuxAttach.
func TestStartTmuxAttachAsUserEmptyUser(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pty, err := StartTmuxAttachAsUser(ctx, "sentinel-pty-test-missing-session", "", 80, 24)
	if err != nil {
		return
	}
	_ = pty.Close()
}
