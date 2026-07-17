package tmux

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/opus-domini/sentinel/internal/userswitch"
)

func TestServiceDelegatesToPackageLevelRunWhenNoUser(t *testing.T) {
	// Not parallel: mutates package-level run variable.

	original := run
	t.Cleanup(func() { run = original })

	var gotArgs []string
	run = func(_ context.Context, args ...string) (string, error) {
		gotArgs = args
		return "ok\n", nil
	}

	svc := Service{}
	_, err := svc.run(context.Background(), "list-sessions", "-F", "test")
	if err != nil {
		t.Fatalf("run error = %v", err)
	}
	if len(gotArgs) != 3 || gotArgs[0] != "list-sessions" {
		t.Errorf("gotArgs = %v, want [list-sessions -F test]", gotArgs)
	}
}

func TestRunAsUserWrapsWithSudo(t *testing.T) {
	// Not parallel: mutates package-level execCommandContext, UserSwitchMethod and SystemUsers.

	originalUsers := SystemUsers
	t.Cleanup(func() { SystemUsers = originalUsers })
	SystemUsers = []string{"testuser"}

	originalMethod := UserSwitchMethod
	t.Cleanup(func() { UserSwitchMethod = originalMethod })
	UserSwitchMethod = userswitch.MethodSudo

	installExecCommandRecorder(t)

	out, err := runAsUser(context.Background(), "testuser", "list-sessions")
	if err != nil {
		t.Fatalf("runAsUser error = %v", err)
	}
	got := strings.Split(strings.TrimSpace(out), "\n")
	want := []string{"sudo", "-n", "-u", "testuser", "tmux", "list-sessions"}
	if !slices.Equal(got, want) {
		t.Fatalf("command = %#v, want %#v", got, want)
	}
}

func TestRunAsUserWrapsWithSystemdRun(t *testing.T) {
	// Not parallel: mutates package-level execCommandContext, UserSwitchMethod and SystemUsers.

	originalUsers := SystemUsers
	t.Cleanup(func() { SystemUsers = originalUsers })
	SystemUsers = []string{"testuser"}

	originalMethod := UserSwitchMethod
	t.Cleanup(func() { UserSwitchMethod = originalMethod })
	UserSwitchMethod = userswitch.MethodSystemdRun

	installExecCommandRecorder(t)

	out, err := runAsUser(context.Background(), "testuser", "list-sessions")
	if err != nil {
		t.Fatalf("runAsUser error = %v", err)
	}
	got := strings.Split(strings.TrimSpace(out), "\n")
	want := []string{
		"sudo",
		"-n",
		"systemd-run",
		"--user",
		"--machine=testuser@.host",
		"--collect",
		"--quiet",
		"--service-type=exec",
		"--expand-environment=no",
		"--property=KillMode=process",
		"--wait",
		"--pipe",
		"tmux",
		"list-sessions",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("command = %#v, want %#v", got, want)
	}
}

func TestRunAsUserEmptyDelegatesToRun(t *testing.T) {
	// Not parallel: mutates package-level run variable.

	original := run
	t.Cleanup(func() { run = original })

	var gotArgs []string
	run = func(_ context.Context, args ...string) (string, error) {
		gotArgs = args
		return "ok\n", nil
	}

	out, err := runAsUser(context.Background(), "", "has-session", "-t", "test")
	if err != nil {
		t.Fatalf("run error = %v", err)
	}
	if strings.TrimSpace(out) != "ok" {
		t.Errorf("output = %q, want ok", out)
	}
	if len(gotArgs) != 3 || gotArgs[0] != "has-session" {
		t.Errorf("gotArgs = %v, want [has-session -t test]", gotArgs)
	}
}

func TestBuildControlCommandUsesNativeTmuxControlMode(t *testing.T) {
	name, args, err := BuildControlCommand("", "agent-work")
	if err != nil {
		t.Fatalf("BuildControlCommand() error = %v", err)
	}
	want := []string{"-C", "attach-session", "-f", "active-pane,ignore-size", "-t", "agent-work"}
	if name != "tmux" || !slices.Equal(args, want) {
		t.Fatalf("BuildControlCommand() = %q %#v, want tmux %#v", name, args, want)
	}
}

func TestBuildControlCommandRejectsEmptySession(t *testing.T) {
	_, _, err := BuildControlCommand("", " ")
	if !IsKind(err, ErrKindInvalidIdentifier) {
		t.Fatalf("BuildControlCommand() error = %v, want invalid identifier", err)
	}
}

func TestServiceCreateSessionWithUser(t *testing.T) {
	// Not parallel: mutates package-level run variable.

	original := run
	t.Cleanup(func() { run = original })

	var gotArgs []string
	run = func(_ context.Context, args ...string) (string, error) {
		gotArgs = args
		return "", nil
	}

	// Empty user => goes through package-level run.
	svc := Service{User: ""}
	if err := svc.CreateSession(context.Background(), "test", "/tmp"); err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	if len(gotArgs) < 1 || gotArgs[0] != "new-session" {
		t.Errorf("gotArgs = %v, want [new-session ...]", gotArgs)
	}
}

func TestServiceSessionExistsWithNoUser(t *testing.T) {
	// Not parallel: mutates package-level run variable.

	original := run
	t.Cleanup(func() { run = original })

	run = func(_ context.Context, _ ...string) (string, error) {
		return "", nil
	}

	svc := Service{}
	exists, err := svc.SessionExists(context.Background(), "test")
	if err != nil {
		t.Fatalf("SessionExists error = %v", err)
	}
	if !exists {
		t.Error("SessionExists = false, want true")
	}
}

func TestVerifySystemUser(t *testing.T) {
	// Not parallel: mutates package-level SystemUsers.

	originalUsers := SystemUsers
	t.Cleanup(func() { SystemUsers = originalUsers })
	SystemUsers = []string{"root", "hugo", "deploy"}

	tests := []struct {
		name    string
		user    string
		wantErr bool
	}{
		{name: "valid root", user: "root", wantErr: false},
		{name: "valid hugo", user: "hugo", wantErr: false},
		{name: "valid deploy", user: "deploy", wantErr: false},
		{name: "empty", user: "", wantErr: true},
		{name: "shell injection semicolon", user: "test;whoami", wantErr: true},
		{name: "shell injection space", user: "test whoami", wantErr: true},
		{name: "shell injection backtick", user: "test`id`", wantErr: true},
		{name: "path traversal", user: "../etc/passwd", wantErr: true},
		{name: "starts with dash", user: "-evil", wantErr: true},
		{name: "too long", user: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", wantErr: true},
		{name: "uppercase rejected", user: "Root", wantErr: true},
		{name: "nonexistent user", user: "zzz_nonexistent_user_zzz", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifySystemUser(tt.user)
			if (err != nil) != tt.wantErr {
				t.Errorf("verifySystemUser(%q) error = %v, wantErr %v", tt.user, err, tt.wantErr)
			}
		})
	}
}

func TestVerifySystemUserEmptyList(t *testing.T) {
	// Not parallel: mutates package-level SystemUsers.

	originalUsers := SystemUsers
	t.Cleanup(func() { SystemUsers = originalUsers })
	SystemUsers = nil

	err := verifySystemUser("hugo")
	if err == nil {
		t.Fatal("expected error when SystemUsers is empty, got nil")
	}
}

func installExecCommandRecorder(t *testing.T) {
	t.Helper()

	original := execCommandContext
	t.Cleanup(func() { execCommandContext = original })
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		helperArgs := []string{"-test.run=TestExecCommandRecorder", "--", name}
		helperArgs = append(helperArgs, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], helperArgs...)
		cmd.Env = append(os.Environ(), "SENTINEL_EXEC_COMMAND_RECORDER=1")
		return cmd
	}
}

func TestExecCommandRecorder(_ *testing.T) {
	if os.Getenv("SENTINEL_EXEC_COMMAND_RECORDER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			for _, item := range os.Args[i+1:] {
				_, _ = os.Stdout.WriteString(item + "\n")
			}
			os.Exit(0)
		}
	}
	os.Exit(2)
}
