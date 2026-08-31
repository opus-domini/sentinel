//go:build linux

package term

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	// fakeTmuxDir names the directory holding the fake tmux executable. The
	// helper process recognises its own invocation by that directory name.
	fakeTmuxDir = "sentinel-test-fake-tmux"
	// fakeTmuxArgvFile is written by the fake tmux next to its own executable.
	fakeTmuxArgvFile = "argv"
)

// TestStartShellLifecycle drives an isolated helper PTY end to end: it exercises
// StartShell, startCommand, openPTY, setWinsize, ioctl, Read, Write, Resize,
// Wait and Close.
func TestStartShellLifecycle(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pty, err := StartShell(ctx, isolatedPTYShell(t), 80, 24)
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

func TestPTYHelperProcess(_ *testing.T) {
	if len(os.Args) == 0 || !strings.HasPrefix(filepath.Base(os.Args[len(os.Args)-1]), "sentinel-test-pty-helper") {
		return
	}
	waitForHelperExit()
}

// TestTmuxHelperProcess impersonates tmux when it is executed through the fake
// installed by installFakeTmux: it records the argv it received and then holds
// the PTY open until stdin asks it to exit.
func TestTmuxHelperProcess(_ *testing.T) {
	dir, args, ok := fakeTmuxInvocation()
	if !ok {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, fakeTmuxArgvFile), []byte(strings.Join(args, "\n")), 0o600)
	waitForHelperExit()
}

// waitForHelperExit blocks a helper process until its PTY master asks it to
// exit, mirroring an interactive program reading from a terminal.
func waitForHelperExit() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		if strings.HasPrefix(strings.TrimSpace(scanner.Text()), "exit") {
			return
		}
	}
}

// fakeTmuxInvocation reports the directory of the fake tmux executable and the
// argv it received. A shebang invocation is "<test binary> <-test.run flag>
// <script path> <script args...>", so the script path is always os.Args[2].
func fakeTmuxInvocation() (dir string, args []string, ok bool) {
	const scriptIndex = 2
	if len(os.Args) <= scriptIndex {
		return "", nil, false
	}
	dir = filepath.Dir(os.Args[scriptIndex])
	if filepath.Base(dir) != fakeTmuxDir {
		return "", nil, false
	}
	return dir, os.Args[scriptIndex+1:], true
}

func isolatedPTYShell(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sentinel-test-pty-helper")
	writeHelperExecutable(t, path, "TestPTYHelperProcess")
	return path
}

// installFakeTmux puts an executable named tmux on the test PATH so the attach
// helpers resolve to a recorder instead of the absent host binary. It returns
// the directory the recorded argv is written to.
func installFakeTmux(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), fakeTmuxDir)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("create fake tmux directory: %v", err)
	}
	writeHelperExecutable(t, filepath.Join(dir, "tmux"), "TestTmuxHelperProcess")
	t.Setenv("PATH", dir)
	return dir
}

// writeHelperExecutable writes an executable that re-enters this test binary
// running only the named helper test.
func writeHelperExecutable(t *testing.T, path, helperTest string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable(): %v", err)
	}
	script := "#!" + executable + " -test.run=^" + helperTest + "$\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write %s executable: %v", helperTest, err)
	}
}

// TestStartTmuxAttach asserts the argv StartTmuxAttach hands to tmux and that
// the resulting PTY completes a round trip.
func TestStartTmuxAttach(t *testing.T) {
	// Not parallel: installFakeTmux replaces the process PATH.

	dir := installFakeTmux(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pty, err := StartTmuxAttach(ctx, "dev", 80, 24)
	if err != nil {
		t.Fatalf("StartTmuxAttach() error = %v", err)
	}
	assertAttachArgv(t, dir, pty, tmuxAttachArgs("dev"))
}

// TestStartTmuxAttachAsUserEmptyUser covers the empty-user branch, which must
// delegate to StartTmuxAttach instead of wrapping the command.
func TestStartTmuxAttachAsUserEmptyUser(t *testing.T) {
	// Not parallel: installFakeTmux replaces the process PATH.

	dir := installFakeTmux(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pty, err := StartTmuxAttachAsUser(ctx, "dev", "", 80, 24)
	if err != nil {
		t.Fatalf("StartTmuxAttachAsUser() error = %v", err)
	}
	assertAttachArgv(t, dir, pty, tmuxAttachArgs("dev"))
}

// assertAttachArgv finishes the PTY round trip started by an attach helper and
// compares the argv the fake tmux recorded against want.
func assertAttachArgv(t *testing.T, dir string, pty *PTY, want []string) {
	t.Helper()
	t.Cleanup(func() { _ = pty.Close() })

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		_, _ = io.Copy(io.Discard, pty)
	}()

	if _, err := pty.Write([]byte("exit\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := pty.Wait(); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}

	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("output drain did not finish after the attach helper exited")
	}

	recorded, err := os.ReadFile(filepath.Join(dir, fakeTmuxArgvFile))
	if err != nil {
		t.Fatalf("read recorded tmux argv: %v", err)
	}
	got := strings.Split(string(recorded), "\n")
	if !slices.Equal(got, want) {
		t.Fatalf("tmux argv = %v, want %v", got, want)
	}
}
