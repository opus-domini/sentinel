package userswitch

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestDefaultMethod(t *testing.T) {
	t.Parallel()

	if got := DefaultMethod("linux"); got != MethodSystemdRun {
		t.Fatalf("DefaultMethod(linux) = %q, want %q", got, MethodSystemdRun)
	}
	if got := DefaultMethod("darwin"); got != MethodSudo {
		t.Fatalf("DefaultMethod(darwin) = %q, want %q", got, MethodSudo)
	}
}

func TestParseMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		method  string
		want    string
		wantErr bool
	}{
		{name: "sudo", method: " sudo ", want: MethodSudo},
		{name: "systemd-run", method: "SYSTEMD-RUN", want: MethodSystemdRun},
		{name: "systemd alias rejected", method: "systemd", wantErr: true},
		{name: "invalid rejected", method: "bad", wantErr: true},
		{name: "empty rejected", method: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseMethod(tt.method)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseMethod(%q) = %q, want error", tt.method, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMethod(%q) error = %v", tt.method, err)
			}
			if got != tt.want {
				t.Fatalf("ParseMethod(%q) = %q, want %q", tt.method, got, tt.want)
			}
		})
	}
}

func TestCapabilities(t *testing.T) {
	t.Run("all executables available", func(t *testing.T) {
		got := capabilitiesWithLookPath(func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		})
		want := []Capability{
			{
				Method:    MethodSudo,
				Available: true,
				Detail:    "sudo is installed; passwordless policy must still be configured by the operator",
			},
			{
				Method:    MethodSystemdRun,
				Available: true,
				Detail:    "sudo and systemd-run are installed; passwordless policy must still be configured by the operator",
			},
		}
		if !slices.Equal(got, want) {
			t.Fatalf("capabilities = %#v, want %#v", got, want)
		}
	})

	t.Run("systemd-run missing", func(t *testing.T) {
		got := capabilitiesWithLookPath(func(name string) (string, error) {
			if name == execSystemdRun {
				return "", errors.New("not found")
			}
			return "/usr/bin/" + name, nil
		})
		if !got[0].Available || got[1].Available ||
			got[1].Detail != "systemd-run was not found in PATH" {
			t.Fatalf("capabilities = %#v", got)
		}
	})

	t.Run("sudo missing blocks both methods", func(t *testing.T) {
		got := capabilitiesWithLookPath(func(name string) (string, error) {
			if name == execSudo {
				return "", errors.New("not found")
			}
			return "/usr/bin/" + name, nil
		})
		if got[0].Available || got[1].Available ||
			got[0].Detail != "sudo was not found in PATH" ||
			got[1].Detail != "sudo was not found in PATH" {
			t.Fatalf("capabilities = %#v", got)
		}
	})
}

func TestBuildTmuxCommandDefaultUser(t *testing.T) {
	t.Parallel()

	name, args, err := BuildTmuxCommand(MethodSystemdRun, "", []string{"list-sessions"}, false)
	if err != nil {
		t.Fatalf("BuildTmuxCommand() error = %v", err)
	}
	if name != "tmux" {
		t.Fatalf("command name = %q, want tmux", name)
	}
	if !slices.Equal(args, []string{"list-sessions"}) {
		t.Fatalf("args = %#v, want list-sessions", args)
	}
}

func TestBuildTmuxCommandSystemdRun(t *testing.T) {
	t.Parallel()

	name, args, err := BuildTmuxCommand(MethodSystemdRun, "deploy", []string{"new-session", "-d", "-s", "api"}, false)
	if err != nil {
		t.Fatalf("BuildTmuxCommand() error = %v", err)
	}
	if name != MethodSudo {
		t.Fatalf("command name = %q, want %s", name, MethodSudo)
	}
	want := []string{
		"-n",
		"systemd-run",
		"--user",
		"--machine=deploy@.host",
		"--collect",
		"--quiet",
		"--service-type=exec",
		"--expand-environment=no",
		"--property=KillMode=process",
		"--wait",
		"--pipe",
		"tmux",
		"new-session",
		"-d",
		"-s",
		"api",
	}
	if !slices.Equal(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestBuildTmuxCommandInteractiveSystemdRun(t *testing.T) {
	t.Parallel()

	name, args, err := BuildTmuxCommand(MethodSystemdRun, "deploy", []string{"attach", "-t", "api"}, true)
	if err != nil {
		t.Fatalf("BuildTmuxCommand() error = %v", err)
	}
	if name != MethodSudo {
		t.Fatalf("command name = %q, want %s", name, MethodSudo)
	}
	for _, want := range []string{"--wait", "--pipe"} {
		if !slices.Contains(args, want) {
			t.Fatalf("interactive args missing %q: %#v", want, args)
		}
	}
	for _, notWant := range []string{"--pty", "--send-sighup", "--property=KillMode=process"} {
		if slices.Contains(args, notWant) {
			t.Fatalf("interactive args should not contain %q: %#v", notWant, args)
		}
	}
}

func TestBuildTmuxCommandSudo(t *testing.T) {
	t.Parallel()

	name, args, err := BuildTmuxCommand(MethodSudo, "deploy", []string{"list-sessions"}, false)
	if err != nil {
		t.Fatalf("BuildTmuxCommand() error = %v", err)
	}
	if name != MethodSudo {
		t.Fatalf("command name = %q, want %s", name, MethodSudo)
	}
	want := []string{"-n", "-u", "deploy", "tmux", "list-sessions"}
	if !slices.Equal(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestBuildShellCommandSudoLogin(t *testing.T) {
	t.Parallel()

	got, err := BuildShellCommand(MethodSudo, "deploy", "")
	if err != nil {
		t.Fatalf("BuildShellCommand() error = %v", err)
	}
	want := "'sudo' '-n' '-i' '-u' 'deploy'"
	if got != want {
		t.Fatalf("BuildShellCommand() = %q, want %q", got, want)
	}
}

func TestBuildShellCommandSystemdShell(t *testing.T) {
	t.Parallel()

	got, err := BuildShellCommand(MethodSystemdRun, "deploy", "  ")
	if err != nil {
		t.Fatalf("BuildShellCommand() error = %v", err)
	}
	for _, want := range []string{"'systemd-run'", "'--machine=deploy@.host'", "'--shell'"} {
		if !strings.Contains(got, want) {
			t.Fatalf("command missing %q: %s", want, got)
		}
	}
}

func TestBuildShellCommandQuotesUserCommand(t *testing.T) {
	t.Parallel()

	got, err := BuildShellCommand(MethodSystemdRun, "deploy", "echo '$HOME'")
	if err != nil {
		t.Fatalf("BuildShellCommand() error = %v", err)
	}
	if !strings.Contains(got, "'--machine=deploy@.host'") {
		t.Fatalf("command missing machine target: %s", got)
	}
	if !strings.Contains(got, "'--same-dir'") {
		t.Fatalf("command missing --same-dir: %s", got)
	}
	if !strings.Contains(got, "'echo '\\''$HOME'\\'''") {
		t.Fatalf("command did not quote shell payload safely: %s", got)
	}
}
