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
	for _, want := range []string{"--pty", "--send-sighup"} {
		if !slices.Contains(args, want) {
			t.Fatalf("interactive args missing %q: %#v", want, args)
		}
	}
	if slices.Contains(args, "--pipe") {
		t.Fatalf("interactive args should not contain --pipe: %#v", args)
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
