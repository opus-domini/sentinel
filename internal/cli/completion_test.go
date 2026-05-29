package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestCompletionCommand verifies cobra's generated completion command emits a
// non-empty script for each supported shell.
func TestCompletionCommand(t *testing.T) {
	t.Parallel()

	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			t.Parallel()

			var out, errOut bytes.Buffer
			code := Run([]string{"completion", shell}, &out, &errOut)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
			}
			if out.Len() == 0 {
				t.Fatalf("completion %s produced no script", shell)
			}
		})
	}
}

func TestCompletionInstallAll(t *testing.T) {
	home := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("SENTINEL_CONFIG", filepath.Join(home, "missing.yaml"))

	var out, errOut bytes.Buffer
	code := Run([]string{"completion", "install", "--shell", "all"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}

	paths := []string{
		filepath.Join(home, ".local", "share", "bash-completion", "completions", "sentinel"),
		filepath.Join(home, ".local", "share", "zsh", "site-functions", "_sentinel"),
		filepath.Join(configHome, "fish", "completions", "sentinel.fish"),
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("completion script not installed: %v", err)
			}
			if info.Size() == 0 {
				t.Fatalf("completion script is empty")
			}
		})
	}
}
