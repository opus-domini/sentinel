package tmux

import (
	"errors"
	"fmt"
	"os/exec"
	"testing"
)

func TestInferCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"simple_command", "claude --resume", "claude"},
		{"npx_runner", "npx codex --full-auto", "codex"},
		{"env_var_prefix", "NODE_ENV=prod claude", "claude"},
		{"absolute_path", "/usr/local/bin/claude", "claude"},
		{"single_quoted", "'quoted command'", "quoted"},
		{"double_quoted", `"double quoted"`, "double"},
		{"env_runner_extension", "env KEY=val node server.js", "node"},
		{"sudo_python_extension", "sudo python3 script.py", "python3"},
		{"bunx_ts_extension", "bunx tsx app.ts", "tsx"},
		{"flags_only", "--flag-only", ""},
		{"env_vars_only", "KEY=val", ""},
		{"whitespace", "  spaces  ", "spaces"},
		{"yarn_dlx", "yarn dlx create-app", "dlx"},
		{"env_node_mjs", "/usr/bin/env node index.mjs", "node"},
		{"pnpm_runner", "pnpm vitest run", "vitest"},
		{"rb_extension", "ruby app.rb", "ruby"},
		{"cjs_extension", "node loader.cjs", "node"},
		{"pl_extension", "exec script.pl", "script"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := inferCommand(tt.input)
			if got != tt.want {
				t.Errorf("inferCommand(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSessionHash(t *testing.T) {
	t.Parallel()

	t.Run("golden_value", func(t *testing.T) {
		t.Parallel()
		got := SessionHash("test", 1700000000)
		want := "47bb9a80cd8e"
		if got != want {
			t.Errorf("SessionHash(%q, %d) = %q, want %q", "test", 1700000000, got, want)
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		t.Parallel()
		a := SessionHash("myapp", 1000)
		b := SessionHash("myapp", 1000)
		if a != b {
			t.Errorf("SessionHash not idempotent: %q != %q", a, b)
		}
	})

	t.Run("different_epoch", func(t *testing.T) {
		t.Parallel()
		a := SessionHash("myapp", 1000)
		b := SessionHash("myapp", 2000)
		if a == b {
			t.Error("SessionHash should differ for different epochs")
		}
	})

	t.Run("different_name", func(t *testing.T) {
		t.Parallel()
		a := SessionHash("foo", 1000)
		b := SessionHash("bar", 1000)
		if a == b {
			t.Error("SessionHash should differ for different names")
		}
	})
}

func TestErrorString(t *testing.T) {
	t.Parallel()

	t.Run("with_msg", func(t *testing.T) {
		t.Parallel()
		e := &Error{Kind: ErrKindCommandFailed, Msg: "custom message", Err: fmt.Errorf("wrapped")}
		if got := e.Error(); got != "custom message" {
			t.Errorf("Error() = %q, want %q", got, "custom message")
		}
	})

	t.Run("msg_empty_with_err", func(t *testing.T) {
		t.Parallel()
		inner := fmt.Errorf("inner error")
		e := &Error{Kind: ErrKindCommandFailed, Err: inner}
		if got := e.Error(); got != "inner error" {
			t.Errorf("Error() = %q, want %q", got, "inner error")
		}
	})

	t.Run("both_empty", func(t *testing.T) {
		t.Parallel()
		e := &Error{Kind: ErrKindNotFound}
		if got := e.Error(); got != string(ErrKindNotFound) {
			t.Errorf("Error() = %q, want %q", got, string(ErrKindNotFound))
		}
	})
}

func TestErrorUnwrap(t *testing.T) {
	t.Parallel()

	t.Run("with_err", func(t *testing.T) {
		t.Parallel()
		inner := fmt.Errorf("wrapped")
		e := &Error{Kind: ErrKindCommandFailed, Err: inner}
		if got := e.Unwrap(); got != inner {
			t.Errorf("Unwrap() = %v, want %v", got, inner)
		}
	})

	t.Run("without_err", func(t *testing.T) {
		t.Parallel()
		e := &Error{Kind: ErrKindCommandFailed}
		if got := e.Unwrap(); got != nil {
			t.Errorf("Unwrap() = %v, want nil", got)
		}
	})
}

func TestIsKind(t *testing.T) {
	t.Parallel()

	t.Run("match", func(t *testing.T) {
		t.Parallel()
		err := &Error{Kind: ErrKindSessionNotFound}
		if !IsKind(err, ErrKindSessionNotFound) {
			t.Error("IsKind should return true for matching kind")
		}
	})

	t.Run("no_match", func(t *testing.T) {
		t.Parallel()
		err := &Error{Kind: ErrKindSessionNotFound}
		if IsKind(err, ErrKindSessionExists) {
			t.Error("IsKind should return false for non-matching kind")
		}
	})

	t.Run("wrong_type", func(t *testing.T) {
		t.Parallel()
		err := fmt.Errorf("plain error")
		if IsKind(err, ErrKindSessionNotFound) {
			t.Error("IsKind should return false for non-tmux error")
		}
	})

	t.Run("wrapped", func(t *testing.T) {
		t.Parallel()
		inner := &Error{Kind: ErrKindSessionExists}
		wrapped := fmt.Errorf("wrapped: %w", inner)
		if !IsKind(wrapped, ErrKindSessionExists) {
			t.Error("IsKind should find kind through wrapped errors")
		}
	})

	t.Run("nil_error", func(t *testing.T) {
		t.Parallel()
		if IsKind(nil, ErrKindNotFound) {
			t.Error("IsKind should return false for nil error")
		}
	})
}

func TestClassifyError(t *testing.T) {
	t.Parallel()

	baseErr := fmt.Errorf("exit status 1")
	args := []string{"list-sessions"}

	tests := []struct {
		name   string
		err    error
		stderr string
		want   ErrorKind
	}{
		{"not_found", exec.ErrNotFound, "", ErrKindNotFound},
		{"cant_find_session", baseErr, "can't find session: foo", ErrKindSessionNotFound},
		{"no_such_session", baseErr, "no such session: bar", ErrKindSessionNotFound},
		{"duplicate_session", baseErr, "duplicate session: baz", ErrKindSessionExists},
		{"already_exists", baseErr, "session already exists: qux", ErrKindSessionExists},
		{"server_not_running", baseErr, "failed to connect to server", ErrKindServerNotRunning},
		{"default", baseErr, "some other error", ErrKindCommandFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := classifyError(tt.err, tt.stderr, args)
			var terr *Error
			if !errors.As(result, &terr) {
				t.Fatalf("classifyError did not return *Error, got %T", result)
			}
			if terr.Kind != tt.want {
				t.Errorf("classifyError kind = %q, want %q", terr.Kind, tt.want)
			}
			if terr.Err != tt.err {
				t.Errorf("classifyError wrapped err = %v, want %v", terr.Err, tt.err)
			}
		})
	}
}
