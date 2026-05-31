package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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

// TestCompletionBashUsesV2 verifies the bash completion is generated with the
// V2 generator (which emits the __start_sentinel entrypoint), not the legacy V1.
func TestCompletionBashUsesV2(t *testing.T) {
	t.Parallel()

	var out, errOut bytes.Buffer
	code := Run([]string{"completion", "bash"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "__start_sentinel") {
		t.Fatalf("bash completion is not V2 (missing __start_sentinel entrypoint)")
	}
}

// TestCompletionInstallZshNote verifies installing zsh completion prints both
// the install confirmation and the fpath guidance note.
func TestCompletionInstallZshNote(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SENTINEL_CONFIG", filepath.Join(home, "missing.yaml"))

	var out, errOut bytes.Buffer
	code := Run([]string{"completion", "install", "--shell", "zsh"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	output := out.String()
	if !strings.Contains(output, "zsh completion installed to") {
		t.Fatalf("missing install confirmation, got: %q", output)
	}
	fpathDir := filepath.Join(home, ".local", "share", "zsh", "site-functions")
	if !strings.Contains(output, "zsh note: ensure "+fpathDir+" is in your fpath.") {
		t.Fatalf("missing zsh fpath note for %q, got: %q", fpathDir, output)
	}
}

// TestShellName covers the basename/dash-stripping normalization and the
// allowlist of supported shells.
func TestShellName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"bare bash", "bash", shellBash},
		{"absolute path", "/usr/bin/zsh", shellZsh},
		{"login dash prefix", "-bash", shellBash},
		{"surrounding space", "  /bin/fish\n", shellFish},
		{"unsupported", "/bin/sh", ""},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := shellName(tc.value); got != tc.want {
				t.Fatalf("shellName(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// TestDetectShell verifies $SHELL drives detection and that an unrecognized
// value falls back to bash.
func TestDetectShell(t *testing.T) {
	t.Run("from SHELL", func(t *testing.T) {
		t.Setenv("SHELL", "/usr/bin/zsh")
		if got := detectShell(); got != shellZsh {
			t.Fatalf("detectShell() = %q, want %q", got, shellZsh)
		}
	})

	t.Run("fallback to bash", func(t *testing.T) {
		t.Setenv("SHELL", "/bin/unknownsh")
		if got := detectShell(); got != shellBash {
			t.Fatalf("detectShell() = %q, want %q", got, shellBash)
		}
	})
}

// TestCompletionShells covers normalization, the auto/all aliases and the
// unsupported-shell error path.
func TestCompletionShells(t *testing.T) {
	t.Run("explicit", func(t *testing.T) {
		t.Parallel()

		got, err := completionShells("  ZSH ")
		if err != nil {
			t.Fatalf("completionShells: %v", err)
		}
		if len(got) != 1 || got[0] != shellZsh {
			t.Fatalf("completionShells(ZSH) = %v, want [zsh]", got)
		}
	})

	t.Run("all", func(t *testing.T) {
		t.Parallel()

		got, err := completionShells(shellAll)
		if err != nil {
			t.Fatalf("completionShells: %v", err)
		}
		want := []string{shellBash, shellZsh, shellFish}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("completionShells(all) = %v, want %v", got, want)
		}
	})

	t.Run("auto resolves to detected shell", func(t *testing.T) {
		t.Setenv("SHELL", "/bin/bash")
		got, err := completionShells(optionAuto)
		if err != nil {
			t.Fatalf("completionShells: %v", err)
		}
		if len(got) != 1 || got[0] != shellBash {
			t.Fatalf("completionShells(auto) = %v, want [bash]", got)
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		t.Parallel()

		if _, err := completionShells("powershell"); err == nil {
			t.Fatalf("completionShells(powershell) = nil error, want failure")
		}
	})
}

// TestCompletionPath verifies the per-shell install destinations, including the
// XDG_CONFIG_HOME override for fish.
func TestCompletionPath(t *testing.T) {
	t.Run("bash and zsh under HOME", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		bash, err := completionPath(shellBash)
		if err != nil {
			t.Fatalf("completionPath(bash): %v", err)
		}
		if want := filepath.Join(home, ".local", "share", "bash-completion", "completions", "sentinel"); bash != want {
			t.Fatalf("completionPath(bash) = %q, want %q", bash, want)
		}

		zsh, err := completionPath(shellZsh)
		if err != nil {
			t.Fatalf("completionPath(zsh): %v", err)
		}
		if want := filepath.Join(home, ".local", "share", "zsh", "site-functions", "_sentinel"); zsh != want {
			t.Fatalf("completionPath(zsh) = %q, want %q", zsh, want)
		}
	})

	t.Run("fish honors XDG_CONFIG_HOME", func(t *testing.T) {
		home := t.TempDir()
		configHome := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", configHome)

		fish, err := completionPath(shellFish)
		if err != nil {
			t.Fatalf("completionPath(fish): %v", err)
		}
		if want := filepath.Join(configHome, "fish", "completions", "sentinel.fish"); fish != want {
			t.Fatalf("completionPath(fish) = %q, want %q", fish, want)
		}
	})
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
