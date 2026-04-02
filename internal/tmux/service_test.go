package tmux

import (
	"context"
	"strings"
	"testing"
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
	// Not parallel: mutates package-level run variable.

	original := run
	t.Cleanup(func() { run = original })

	called := false
	run = func(_ context.Context, args ...string) (string, error) {
		called = true
		return "", nil
	}

	// runAsUser with a non-empty user should NOT call the package-level run.
	// It will fail because sudo is not available, but that's expected.
	_, err := runAsUser(context.Background(), "testuser", "list-sessions")
	if called {
		t.Fatal("package-level run should NOT be called when user is set")
	}
	// We expect an error because sudo won't work in test env.
	if err == nil {
		t.Fatal("expected error from sudo, got nil")
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

	run = func(_ context.Context, args ...string) (string, error) {
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
