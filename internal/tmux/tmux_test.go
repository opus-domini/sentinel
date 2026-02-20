package tmux

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
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
		{"no_server_running", baseErr, "no server running on /tmp/tmux-1000/default", ErrKindServerNotRunning},
		{"error_connecting_nosuchfile", baseErr, "error connecting to /tmp/tmux-1000/default (No such file or directory)", ErrKindServerNotRunning},
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

func TestParseSessionListOutput(t *testing.T) {
	t.Parallel()

	withActivity := "app\t3\t1\t1700000000\t1700000300"
	sessions := parseSessionListOutput(withActivity)
	if len(sessions) != 1 {
		t.Fatalf("len(parseSessionListOutput) = %d, want 1", len(sessions))
	}
	if sessions[0].Name != "app" || sessions[0].Windows != 3 || sessions[0].Attached != 1 {
		t.Fatalf("unexpected parsed session: %+v", sessions[0])
	}
	if sessions[0].CreatedAt.Unix() != 1700000000 {
		t.Fatalf("CreatedAt = %d, want 1700000000", sessions[0].CreatedAt.Unix())
	}
	if sessions[0].ActivityAt.Unix() != 1700000300 {
		t.Fatalf("ActivityAt = %d, want 1700000300", sessions[0].ActivityAt.Unix())
	}

	withoutActivity := "legacy\t2\t0\t1700000500"
	sessions = parseSessionListOutput(withoutActivity)
	if len(sessions) != 1 {
		t.Fatalf("len(parseSessionListOutput legacy) = %d, want 1", len(sessions))
	}
	if sessions[0].ActivityAt.Unix() != 1700000500 {
		t.Fatalf("legacy ActivityAt = %d, want fallback created epoch", sessions[0].ActivityAt.Unix())
	}
}

func TestShouldRetryListSessionsWithoutActivity(t *testing.T) {
	t.Parallel()

	if !shouldRetryListSessionsWithoutActivity(errors.New("unknown format: session_activity")) {
		t.Fatal("expected retry for unknown session_activity format")
	}
	if !shouldRetryListSessionsWithoutActivity(errors.New("bad format #{session_activity}")) {
		t.Fatal("expected retry for bad session_activity format")
	}
	if shouldRetryListSessionsWithoutActivity(errors.New("some other error")) {
		t.Fatal("did not expect retry for generic error")
	}
	if shouldRetryListSessionsWithoutActivity(nil) {
		t.Fatal("did not expect retry for nil error")
	}
}

func TestPatchMouseDown3PaneBinding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		line      string
		want      string
		changed   bool
		wantToken string
	}{
		{ //nolint:gosec // G101 false positive: tmux config test data, not credentials
			name:      "adds_O_and_M_flags",
			line:      `bind-key -T root MouseDown3Pane if-shell -F "#{mouse_any_flag}" { send-keys -M } { display-menu -T "Pane" -t = -x M -y M "Kill" X { kill-pane } }`,
			changed:   true,
			wantToken: "display-menu -O -M -T",
		},
		{ //nolint:gosec // G101 false positive: tmux config test data, not credentials
			name:      "already_patched",
			line:      `bind-key -T root MouseDown3Pane if-shell -F "#{mouse_any_flag}" { send-keys -M } { display-menu -O -M -T "Pane" -t = -x M -y M "Kill" X { kill-pane } }`,
			changed:   false,
			wantToken: "display-menu -O -M -T",
		},
		{
			name:      "unrelated_binding",
			line:      `bind-key -T root MouseDown1Pane select-pane -t = \; send-keys -M`,
			changed:   false,
			wantToken: "MouseDown1Pane",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, changed := patchMouseDown3PaneBinding(tt.line)
			if changed != tt.changed {
				t.Fatalf("changed = %v, want %v", changed, tt.changed)
			}
			if !strings.Contains(got, tt.wantToken) {
				t.Fatalf("patched line missing token %q: %q", tt.wantToken, got)
			}
		})
	}
}

func TestPatchDoubleClick1PaneBinding(t *testing.T) {
	t.Parallel()

	line := `bind-key -T root DoubleClick1Pane select-pane -t = \; if-shell -F "#{||:#{pane_in_mode},#{mouse_any_flag}}" { send-keys -M } { copy-mode -H ; send-keys -X select-word ; run-shell -d 0.3 ; send-keys -X copy-pipe-and-cancel }`
	got, changed := patchDoubleClick1PaneBinding(line)
	if !changed {
		t.Fatal("expected patchDoubleClick1PaneBinding to change default binding")
	}
	if strings.Contains(got, "copy-pipe-and-cancel") {
		t.Fatalf("expected copy-pipe-and-cancel to be removed: %q", got)
	}
	if !strings.Contains(got, "{ send-keys -M }") {
		t.Fatalf("expected fallback block to be send-keys -M: %q", got)
	}
}

func TestPatchTripleClick1PaneBinding(t *testing.T) {
	t.Parallel()

	line := `bind-key -T root TripleClick1Pane select-pane -t = \; if-shell -F "#{||:#{pane_in_mode},#{mouse_any_flag}}" { send-keys -M } { copy-mode -H ; send-keys -X select-line ; run-shell -d 0.3 ; send-keys -X copy-pipe-and-cancel }`
	got, changed := patchTripleClick1PaneBinding(line)
	if !changed {
		t.Fatal("expected patchTripleClick1PaneBinding to change default binding")
	}
	if strings.Contains(got, "copy-pipe-and-cancel") {
		t.Fatalf("expected copy-pipe-and-cancel to be removed: %q", got)
	}
	if !strings.Contains(got, "{ send-keys -M }") {
		t.Fatalf("expected fallback block to be send-keys -M: %q", got)
	}
}

func TestPatchCopyModeDragEndBinding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		line    string
		want    string
		changed bool
	}{
		{
			name:    "patches_copy_pipe_and_cancel",
			line:    `bind-key -T copy-mode-vi MouseDragEnd1Pane send-keys -X copy-pipe-and-cancel`,
			want:    `bind-key -T copy-mode-vi MouseDragEnd1Pane send-keys -X copy-pipe-no-clear`,
			changed: true,
		},
		{
			name:    "patches_copy_selection_and_cancel",
			line:    `bind-key -T copy-mode MouseDragEnd1Pane send-keys -X copy-selection-and-cancel`,
			want:    `bind-key -T copy-mode MouseDragEnd1Pane send-keys -X copy-selection-no-clear`,
			changed: true,
		},
		{
			name:    "already_patched_pipe",
			line:    `bind-key -T copy-mode-vi MouseDragEnd1Pane send-keys -X copy-pipe-no-clear`,
			want:    `bind-key -T copy-mode-vi MouseDragEnd1Pane send-keys -X copy-pipe-no-clear`,
			changed: false,
		},
		{
			name:    "already_patched_selection",
			line:    `bind-key -T copy-mode MouseDragEnd1Pane send-keys -X copy-selection-no-clear`,
			want:    `bind-key -T copy-mode MouseDragEnd1Pane send-keys -X copy-selection-no-clear`,
			changed: false,
		},
		{
			name:    "patches_copy_pipe_with_command_arg",
			line:    `bind-key -T copy-mode-vi MouseDragEnd1Pane send-keys -X copy-pipe-and-cancel "pbcopy"`,
			want:    `bind-key -T copy-mode-vi MouseDragEnd1Pane send-keys -X copy-pipe-no-clear "pbcopy"`,
			changed: true,
		},
		{
			name:    "patches_copy_selection_in_compound_binding",
			line:    `bind-key -T copy-mode MouseDragEnd1Pane select-pane \; send-keys -X copy-selection-and-cancel`,
			want:    `bind-key -T copy-mode MouseDragEnd1Pane select-pane \; send-keys -X copy-selection-no-clear`,
			changed: true,
		},
		{
			name:    "no_cancel_suffix_unchanged",
			line:    `bind-key -T copy-mode MouseDragEnd1Pane send-keys -X cancel`,
			want:    `bind-key -T copy-mode MouseDragEnd1Pane send-keys -X cancel`,
			changed: false,
		},
		{
			name:    "unrelated_binding",
			line:    `bind-key -T copy-mode MouseDrag1Pane select-pane \; send-keys -X begin-selection`,
			want:    `bind-key -T copy-mode MouseDrag1Pane select-pane \; send-keys -X begin-selection`,
			changed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, changed := patchCopyModeDragEndBinding(tt.line)
			if changed != tt.changed {
				t.Fatalf("changed = %v, want %v", changed, tt.changed)
			}
			if got != tt.want {
				t.Fatalf("got = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseNewWindowOutput(t *testing.T) {
	t.Parallel()

	t.Run("valid output", func(t *testing.T) {
		t.Parallel()
		got, err := parseNewWindowOutput("3\t%19\n")
		if err != nil {
			t.Fatalf("parseNewWindowOutput error = %v", err)
		}
		if got.Index != 3 || got.PaneID != "%19" {
			t.Fatalf("got = %+v, want index=3 paneID=%%19", got)
		}
	})

	t.Run("invalid output", func(t *testing.T) {
		t.Parallel()
		if _, err := parseNewWindowOutput("bad-output"); err == nil {
			t.Fatal("expected parseNewWindowOutput to fail")
		}
	})
}

func TestParseSplitPaneOutput(t *testing.T) {
	t.Parallel()

	t.Run("valid output", func(t *testing.T) {
		t.Parallel()
		got, err := parseSplitPaneOutput("%7\n")
		if err != nil {
			t.Fatalf("parseSplitPaneOutput error = %v", err)
		}
		if got != "%7" {
			t.Fatalf("got = %q, want %q", got, "%7")
		}
	})

	t.Run("invalid output", func(t *testing.T) {
		t.Parallel()
		if _, err := parseSplitPaneOutput("7"); err == nil {
			t.Fatal("expected parseSplitPaneOutput to fail")
		}
	})
}

func TestNextWindowIndexFromListOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		out    string
		want   int
		wantOK bool
	}{
		{
			name:   "sequential indexes",
			out:    "0\n1\n2\n",
			want:   3,
			wantOK: true,
		},
		{
			name:   "with gaps uses rightmost",
			out:    "0\n2\n",
			want:   3,
			wantOK: true,
		},
		{
			name:   "ignores invalid rows",
			out:    "0\nfoo\n5\n",
			want:   6,
			wantOK: true,
		},
		{
			name:   "no valid indexes",
			out:    "\nfoo\n-1\n",
			want:   0,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := nextWindowIndexFromListOutput(tt.out)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("nextWindowIndexFromListOutput(%q) = %d, want %d", tt.out, got, tt.want)
			}
		})
	}
}
