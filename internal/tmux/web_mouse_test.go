package tmux

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestHasSession(t *testing.T) {
	// Not parallel: mutates the package-level run variable.

	original := run
	t.Cleanup(func() { run = original })

	t.Run("exists", func(t *testing.T) {
		run = func(_ context.Context, _ ...string) (string, error) { return "", nil }
		if !HasSession(context.Background(), "dev") {
			t.Fatal("HasSession() = false, want true when run succeeds")
		}
	})

	t.Run("missing", func(t *testing.T) {
		run = func(_ context.Context, _ ...string) (string, error) {
			return "", errors.New("no such session")
		}
		if HasSession(context.Background(), "ghost") {
			t.Fatal("HasSession() = true, want false when run fails")
		}
	})
}

func TestPatchBinding(t *testing.T) {
	// Not parallel: mutates the package-level run variable.

	original := run
	t.Cleanup(func() { run = original })

	noChange := func(line string) (string, bool) { return line, false }
	rewrite := func(_ string) (string, bool) { return "bind-key -T root patched", true }

	t.Run("list-keys error is ignored", func(t *testing.T) {
		run = func(_ context.Context, _ ...string) (string, error) {
			return "", errors.New("unknown key")
		}
		if err := patchBinding(context.Background(), "root", "Key", rewrite); err != nil {
			t.Fatalf("patchBinding() error = %v, want nil", err)
		}
	})

	t.Run("empty binding is a no-op", func(t *testing.T) {
		run = func(_ context.Context, _ ...string) (string, error) { return "   \n", nil }
		if err := patchBinding(context.Background(), "root", "Key", rewrite); err != nil {
			t.Fatalf("patchBinding() error = %v, want nil", err)
		}
	})

	t.Run("patch reports no change", func(t *testing.T) {
		run = func(_ context.Context, _ ...string) (string, error) { return "bind-key -T root Key x", nil }
		if err := patchBinding(context.Background(), "root", "Key", noChange); err != nil {
			t.Fatalf("patchBinding() error = %v, want nil", err)
		}
	})

	t.Run("applies patch via source-file", func(t *testing.T) {
		var sourced bool
		run = func(_ context.Context, args ...string) (string, error) {
			switch args[0] {
			case "list-keys":
				return "bind-key -T root Key original", nil
			case "source-file":
				sourced = true
				return "", nil
			default:
				return "", nil
			}
		}
		if err := patchBinding(context.Background(), "root", "Key", rewrite); err != nil {
			t.Fatalf("patchBinding() error = %v", err)
		}
		if !sourced {
			t.Fatal("patchBinding did not invoke source-file")
		}
	})

	t.Run("propagates source-file error", func(t *testing.T) {
		wantErr := errors.New("source failed")
		run = func(_ context.Context, args ...string) (string, error) {
			if args[0] == "source-file" {
				return "", wantErr
			}
			return "bind-key -T root Key original", nil
		}
		if err := patchBinding(context.Background(), "root", "Key", rewrite); !errors.Is(err, wantErr) {
			t.Fatalf("patchBinding() error = %v, want %v", err, wantErr)
		}
	})
}

func TestEnsureWebMouseBindings(t *testing.T) {
	// Not parallel: mutates the package-level run variable.

	original := run
	t.Cleanup(func() { run = original })

	t.Run("no bindings to patch", func(t *testing.T) {
		run = func(_ context.Context, _ ...string) (string, error) { return "", nil }
		if err := EnsureWebMouseBindings(context.Background()); err != nil {
			t.Fatalf("EnsureWebMouseBindings() error = %v, want nil", err)
		}
	})

	t.Run("patches MouseDown3Pane menu binding", func(t *testing.T) {
		var sourced int
		run = func(_ context.Context, args ...string) (string, error) {
			switch {
			case args[0] == "list-keys" && contains(args, "MouseDown3Pane"):
				return "bind-key -T root MouseDown3Pane display-menu -t = ", nil
			case args[0] == "source-file":
				sourced++
				return "", nil
			default:
				return "", nil
			}
		}
		if err := EnsureWebMouseBindings(context.Background()); err != nil {
			t.Fatalf("EnsureWebMouseBindings() error = %v", err)
		}
		if sourced == 0 {
			t.Fatal("expected at least one binding to be patched via source-file")
		}
	})
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if strings.Contains(a, want) {
			return true
		}
	}
	return false
}
