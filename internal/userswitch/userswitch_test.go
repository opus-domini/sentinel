package userswitch

import (
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

func TestNormalizeMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		method   string
		fallback string
		want     string
	}{
		{name: "sudo", method: " sudo ", fallback: MethodSystemdRun, want: MethodSudo},
		{name: "systemd alias", method: "systemd", fallback: MethodSudo, want: MethodSystemdRun},
		{name: "systemd-run", method: "SYSTEMD-RUN", fallback: MethodSudo, want: MethodSystemdRun},
		{name: "invalid uses fallback", method: "bad", fallback: MethodSystemdRun, want: MethodSystemdRun},
		{name: "empty fallback defaults to sudo", method: "bad", fallback: "", want: MethodSudo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := NormalizeMethod(tt.method, tt.fallback); got != tt.want {
				t.Fatalf("NormalizeMethod(%q, %q) = %q, want %q", tt.method, tt.fallback, got, tt.want)
			}
		})
	}
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
