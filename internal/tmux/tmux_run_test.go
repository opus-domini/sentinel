package tmux

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
)

const (
	cmdListWindows    = "list-windows"
	cmdDisplayMessage = "display-message"
)

// local is the default-user tmux service. With an empty User every command
// routes through the injectable package-level run variable (and every command
// that can start the server through createSessionRun), so these tests exercise
// the exact path production takes on the host account.
var local = Service{}

// setRun swaps the package-level run function for the duration of the
// test and restores the original in t.Cleanup.
// Tests that call setRun must NOT use t.Parallel() at the top level
// to avoid data races on the package variable. Sub-tests may be parallel
// because they share the same fake within a single parent scope.
func setRun(t *testing.T, fn func(ctx context.Context, args ...string) (string, error)) {
	t.Helper()
	orig := run
	run = fn
	t.Cleanup(func() { run = orig })
}

func setCreateSessionRun(t *testing.T, fn func(ctx context.Context, args ...string) (string, error)) {
	t.Helper()
	original := createSessionRun
	createSessionRun = fn
	t.Cleanup(func() { createSessionRun = original })
}

func errServerNotRunning() *Error {
	return &Error{Kind: ErrKindServerNotRunning, Msg: "no server running"}
}

func errSessionNotFound() *Error {
	return &Error{Kind: ErrKindSessionNotFound, Msg: "can't find session: test"}
}

func errCommandFailed(msg string) *Error {
	return &Error{Kind: ErrKindCommandFailed, Msg: msg}
}

// --- ListSessions ---

func TestListSessions(t *testing.T) {
	ctx := context.Background()

	t.Run("happy_path_with_activity", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "$1\x1fdev\x1f2\x1f1\x1f1700000000\x1f1700000300\n$2\x1fweb\x1f1\x1f0\x1f1700000100\x1f1700000400\n", nil
		})

		sessions, err := local.ListSessions(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sessions) != 2 {
			t.Fatalf("got %d sessions, want 2", len(sessions))
		}
		if sessions[0].Name != "dev" || sessions[0].Windows != 2 || sessions[0].Attached != 1 {
			t.Errorf("sessions[0] = %+v", sessions[0])
		}
		if sessions[1].Name != "web" || sessions[1].Windows != 1 || sessions[1].Attached != 0 {
			t.Errorf("sessions[1] = %+v", sessions[1])
		}
	})

	t.Run("server_not_running_returns_empty", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "", errServerNotRunning()
		})

		sessions, err := local.ListSessions(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sessions) != 0 {
			t.Fatalf("got %d sessions, want 0", len(sessions))
		}
	})

	t.Run("retry_without_activity_on_format_error", func(t *testing.T) {
		calls := 0
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			calls++
			if calls == 1 {
				return "", errCommandFailed("unknown format: session_activity")
			}
			return "$3\x1flegacy\x1f1\x1f0\x1f1700000500\n", nil
		})

		sessions, err := local.ListSessions(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sessions) != 1 || sessions[0].Name != "legacy" {
			t.Errorf("sessions = %+v", sessions)
		}
		if calls != 2 {
			t.Errorf("expected 2 run calls (retry), got %d", calls)
		}
	})

	t.Run("retry_server_not_running_on_second_call", func(t *testing.T) {
		calls := 0
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			calls++
			if calls == 1 {
				return "", errCommandFailed("unknown format: session_activity")
			}
			return "", errServerNotRunning()
		})

		sessions, err := local.ListSessions(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sessions) != 0 {
			t.Fatalf("got %d sessions, want 0", len(sessions))
		}
	})

	t.Run("retry_error_on_second_call", func(t *testing.T) {
		calls := 0
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			calls++
			if calls == 1 {
				return "", errCommandFailed("bad format #{session_activity}")
			}
			return "", errCommandFailed("some other failure")
		})

		_, err := local.ListSessions(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsKind(err, ErrKindCommandFailed) {
			t.Errorf("expected ErrKindCommandFailed, got %v", err)
		}
	})

	t.Run("non_retryable_error", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "", errCommandFailed("permission denied")
		})

		_, err := local.ListSessions(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsKind(err, ErrKindCommandFailed) {
			t.Errorf("expected ErrKindCommandFailed, got %v", err)
		}
	})
}

func TestGetSession(t *testing.T) {
	ctx := context.Background()

	t.Run("exact_target", func(t *testing.T) {
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			want := []string{"list-sessions", "-F", listSessionsFormatWithActivity}
			if !slices.Equal(args, want) {
				t.Fatalf("args = %#v, want %#v", args, want)
			}
			return "$8\x1fdev-server\x1f1\x1f0\x1f1700000000\x1f1700000100\n$9\x1fdev\x1f2\x1f0\x1f1700000000\x1f1700000100\n", nil
		})
		session, err := local.GetSession(ctx, "dev")
		if err != nil {
			t.Fatalf("GetSession() error = %v", err)
		}
		if session.ID != "$9" || session.Name != "dev" {
			t.Fatalf("GetSession() = %#v", session)
		}
	})

	t.Run("empty_name", func(t *testing.T) {
		if _, err := local.GetSession(ctx, " "); !IsKind(err, ErrKindInvalidIdentifier) {
			t.Fatalf("GetSession() error = %v", err)
		}
	})

	t.Run("empty_result", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) { return "", nil })
		if _, err := local.GetSession(ctx, "missing"); !IsKind(err, ErrKindSessionNotFound) {
			t.Fatalf("GetSession() error = %v", err)
		}
	})
}

func TestCreateSessionWithID(t *testing.T) {
	ctx := context.Background()

	t.Run("returns_stable_identity", func(t *testing.T) {
		// Injected through createSessionRun, not run: this command can start
		// the tmux server, so it must take the systemd-run isolated path.
		setCreateSessionRun(t, func(_ context.Context, args ...string) (string, error) {
			want := []string{"new-session", "-d", "-P", "-F", createSessionFormat, "-s", "agent", "-c", "/srv/app"}
			if !slices.Equal(args, want) {
				t.Fatalf("args = %#v, want %#v", args, want)
			}
			return "$17\x1fagent\n", nil
		})
		session, err := local.CreateSessionWithID(ctx, "agent", "/srv/app")
		if err != nil {
			t.Fatalf("CreateSessionWithID() error = %v", err)
		}
		if session.ID != "$17" || session.Name != "agent" {
			t.Fatalf("CreateSessionWithID() = %#v", session)
		}
	})

	t.Run("rejects_invalid_identity", func(t *testing.T) {
		setCreateSessionRun(t, func(_ context.Context, _ ...string) (string, error) { return "agent", nil })
		if _, err := local.CreateSessionWithID(ctx, "agent", ""); !IsKind(err, ErrKindCommandFailed) {
			t.Fatalf("CreateSessionWithID() error = %v", err)
		}
	})
}

// --- ListActivePaneCommands ---

func TestListActivePaneCommands(t *testing.T) {
	ctx := context.Background()

	t.Run("happy_path", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "dev\x1f1\x1f1\x1fclaude --resume\x1fclaude\ndev\x1f0\x1f0\x1fbash\x1fbash\nweb\x1f1\x1f1\x1fnpx vite\x1fvite\n", nil
		})

		result, err := local.ListActivePaneCommands(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("got %d entries, want 2", len(result))
		}
		if result["dev"].Command != "claude" {
			t.Errorf("dev command = %q, want %q", result["dev"].Command, "claude")
		}
		if result["dev"].Panes != 2 {
			t.Errorf("dev panes = %d, want 2", result["dev"].Panes)
		}
		if result["web"].Command != "vite" {
			t.Errorf("web command = %q, want %q", result["web"].Command, "vite")
		}
	})

	t.Run("empty_output", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "  \n", nil
		})

		result, err := local.ListActivePaneCommands(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Fatalf("got %d entries, want 0", len(result))
		}
	})

	t.Run("server_not_running", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "", errServerNotRunning()
		})

		result, err := local.ListActivePaneCommands(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Fatalf("got %d entries, want 0", len(result))
		}
	})

	t.Run("malformed_lines_skipped", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "bad\x1fline\ndev\x1f1\x1f1\x1fclaude\x1fclaude\n", nil
		})

		result, err := local.ListActivePaneCommands(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("got %d entries, want 1", len(result))
		}
	})

	t.Run("non_active_pane_not_used_for_command", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			// window_active=0, pane_active=0 — not the active pane
			return "app\x1f0\x1f0\x1fbash\x1fbash\n", nil
		})

		result, err := local.ListActivePaneCommands(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result["app"].Command != "" {
			t.Errorf("expected empty command for non-active pane, got %q", result["app"].Command)
		}
		if result["app"].Panes != 1 {
			t.Errorf("pane count = %d, want 1", result["app"].Panes)
		}
	})

	t.Run("error_propagation", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "", errCommandFailed("timeout")
		})

		_, err := local.ListActivePaneCommands(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("fallback_to_current_command", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			// pane_start_command is empty, falls back to pane_current_command
			return "app\x1f1\x1f1\x1f\x1fnode\n", nil
		})

		result, err := local.ListActivePaneCommands(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result["app"].Command != "node" {
			t.Errorf("command = %q, want %q", result["app"].Command, "node")
		}
	})
}

// --- CapturePane ---

func TestCapturePane(t *testing.T) {
	ctx := context.Background()

	t.Run("returns_last_non_empty_line", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "first line\nsecond line\n  \n\n", nil
		})

		got, err := local.CapturePane(ctx, "dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "second line" {
			t.Errorf("got %q, want %q", got, "second line")
		}
	})

	t.Run("all_blank_returns_empty", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "  \n  \n\n", nil
		})

		got, err := local.CapturePane(ctx, "dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("run_error_returns_empty_no_error", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "", errCommandFailed("pane not found")
		})

		got, err := local.CapturePane(ctx, "dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

// --- ListWindows ---

func TestListWindows(t *testing.T) {
	ctx := context.Background()

	t.Run("parses_multi_window_output", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "dev\x1f@1\x1f0\x1fcode\x1f1\x1f2\x1f83ed,204x51,0,0{102x51,0,0,0,101x51,103,0,1}\ndev\x1f@2\x1f1\x1fshell\x1f0\x1f1\x1f8502,204x51,0,0,2\n", nil
		})

		windows, err := local.ListWindows(ctx, "dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(windows) != 2 {
			t.Fatalf("got %d windows, want 2", len(windows))
		}
		if windows[0].Session != "dev" || windows[0].ID != "@1" || windows[0].Index != 0 || windows[0].Name != "code" || !windows[0].Active || windows[0].Panes != 2 {
			t.Errorf("windows[0] = %+v", windows[0])
		}
		if windows[1].Session != "dev" || windows[1].ID != "@2" || windows[1].Index != 1 || windows[1].Name != "shell" || windows[1].Active || windows[1].Panes != 1 {
			t.Errorf("windows[1] = %+v", windows[1])
		}
		if windows[0].Layout == "" {
			t.Error("expected non-empty layout for windows[0]")
		}
	})

	t.Run("empty_output", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "  \n", nil
		})

		windows, err := local.ListWindows(ctx, "dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(windows) != 0 {
			t.Fatalf("got %d windows, want 0", len(windows))
		}
	})

	t.Run("error_propagation", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "", errSessionNotFound()
		})

		_, err := local.ListWindows(ctx, "dev")
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsKind(err, ErrKindSessionNotFound) {
			t.Errorf("expected ErrKindSessionNotFound, got %v", err)
		}
	})

	t.Run("short_lines_skipped", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "bad\x1fshort\ndev\x1f@1\x1f0\x1fcode\x1f1\x1f1\x1f8502,204x51,0,0,2\n", nil
		})

		windows, err := local.ListWindows(ctx, "dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(windows) != 1 {
			t.Fatalf("got %d windows, want 1", len(windows))
		}
	})

	t.Run("no_layout_column", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "dev\x1f@1\x1f0\x1fcode\x1f1\x1f1\n", nil
		})

		windows, err := local.ListWindows(ctx, "dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(windows) != 1 {
			t.Fatalf("got %d windows, want 1", len(windows))
		}
		if windows[0].Layout != "" {
			t.Errorf("expected empty layout, got %q", windows[0].Layout)
		}
	})
}

func TestReorderWindows(t *testing.T) {
	ctx := context.Background()

	t.Run("reorders by stable tmux window ids", func(t *testing.T) {
		var commands [][]string
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			commands = append(commands, append([]string{}, args...))
			switch args[0] {
			case cmdListWindows:
				return "dev\x1f@1\x1f0\x1fclaude\x1f1\x1f1\x1flayout-a\ndev\x1f@2\x1f1\x1fshell\x1f0\x1f1\x1flayout-b\ndev\x1f@3\x1f2\x1frunner\x1f0\x1f1\x1flayout-c\n", nil
			case "swap-window":
				return "", nil
			default:
				t.Fatalf("unexpected tmux command: %v", args)
				return "", nil
			}
		})

		err := local.ReorderWindows(ctx, "dev", []string{"@2", "@3", "@1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := [][]string{
			{
				"list-windows",
				"-t",
				"dev",
				"-F",
				listWindowsFormat,
			},
			{"swap-window", "-d", "-s", "@1", "-t", "@2"},
			{"swap-window", "-d", "-s", "@1", "-t", "@3"},
		}
		if len(commands) != len(want) {
			t.Fatalf("commands = %#v, want %#v", commands, want)
		}
		for index := range want {
			if fmt.Sprint(commands[index]) != fmt.Sprint(want[index]) {
				t.Fatalf("command[%d] = %#v, want %#v", index, commands[index], want[index])
			}
		}
	})

	t.Run("rejects stale window order", func(t *testing.T) {
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			if args[0] != cmdListWindows {
				t.Fatalf("unexpected tmux command: %v", args)
			}
			return "dev\x1f@1\x1f0\x1fclaude\x1f1\x1f1\x1flayout-a\ndev\x1f@2\x1f1\x1fshell\x1f0\x1f1\x1flayout-b\n", nil
		})

		err := local.ReorderWindows(ctx, "dev", []string{"@1", "@3"})
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsKind(err, ErrKindInvalidIdentifier) {
			t.Fatalf("expected ErrKindInvalidIdentifier, got %v", err)
		}
		if !strings.Contains(err.Error(), "does not match live windows") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})
}

// --- ListPanes ---

func TestListPanes(t *testing.T) {
	ctx := context.Background()

	t.Run("parses_and_filters_by_session", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			lines := []string{
				"dev\x1f0\x1f0\x1f%0\x1ftitle0\x1f1\x1f/dev/pts/0\x1f/home\x1fbash\x1fbash\x1f0\x1f0\x1f102\x1f51",
				"dev\x1f0\x1f1\x1f%1\x1ftitle1\x1f0\x1f/dev/pts/1\x1f/home\x1fbash\x1fvim\x1f103\x1f0\x1f101\x1f51",
				"other\x1f0\x1f0\x1f%2\x1ftitle2\x1f1\x1f/dev/pts/2\x1f/tmp\x1fsh\x1fsh\x1f0\x1f0\x1f204\x1f51",
			}
			return strings.Join(lines, "\n") + "\n", nil
		})

		panes, err := local.ListPanes(ctx, "dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(panes) != 2 {
			t.Fatalf("got %d panes, want 2 (filtered)", len(panes))
		}
		if panes[0].PaneID != "%0" || panes[0].Title != "title0" || !panes[0].Active {
			t.Errorf("panes[0] = %+v", panes[0])
		}
		if panes[1].PaneID != "%1" || panes[1].WindowIndex != 0 || panes[1].PaneIndex != 1 {
			t.Errorf("panes[1] = %+v", panes[1])
		}
		if panes[0].Width != 102 || panes[0].Height != 51 {
			t.Errorf("panes[0] dimensions = %dx%d, want 102x51", panes[0].Width, panes[0].Height)
		}
		if panes[1].Left != 103 || panes[1].CurrentCommand != "vim" {
			t.Errorf("panes[1] left=%d cmd=%q", panes[1].Left, panes[1].CurrentCommand)
		}
	})

	t.Run("empty_output", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return " \n", nil
		})

		panes, err := local.ListPanes(ctx, "dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(panes) != 0 {
			t.Fatalf("got %d panes, want 0", len(panes))
		}
	})

	t.Run("error_propagation", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "", errSessionNotFound()
		})

		_, err := local.ListPanes(ctx, "dev")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("short_lines_skipped", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "dev\x1f0\x1f0\n", nil
		})

		panes, err := local.ListPanes(ctx, "dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(panes) != 0 {
			t.Fatalf("got %d panes, want 0", len(panes))
		}
	})

	t.Run("minimal_fields", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			// Only 7 fields (the minimum), no optional fields
			return "dev\x1f0\x1f0\x1f%5\x1fmypane\x1f1\x1f/dev/pts/3\n", nil
		})

		panes, err := local.ListPanes(ctx, "dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(panes) != 1 {
			t.Fatalf("got %d panes, want 1", len(panes))
		}
		if panes[0].PaneID != "%5" || panes[0].TTY != "/dev/pts/3" {
			t.Errorf("panes[0] = %+v", panes[0])
		}
		// Optional fields should be zero values
		if panes[0].CurrentPath != "" || panes[0].Width != 0 {
			t.Errorf("expected zero optional fields: path=%q width=%d", panes[0].CurrentPath, panes[0].Width)
		}
	})
}

// --- SessionExists ---

func TestSessionExists(t *testing.T) {
	ctx := context.Background()

	t.Run("exists", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "", nil
		})

		got, err := local.SessionExists(ctx, "dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got {
			t.Error("expected true")
		}
	})

	t.Run("session_not_found", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "", errSessionNotFound()
		})

		got, err := local.SessionExists(ctx, "dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Error("expected false")
		}
	})

	t.Run("server_not_running", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "", errServerNotRunning()
		})

		got, err := local.SessionExists(ctx, "dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Error("expected false")
		}
	})

	t.Run("other_error", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "", errCommandFailed("permission denied")
		})

		_, err := local.SessionExists(ctx, "dev")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

// --- CapturePaneLines ---

func TestCapturePaneLines(t *testing.T) {
	ctx := context.Background()

	t.Run("empty_target_returns_error", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			t.Fatal("run should not be called")
			return "", nil
		})

		_, err := local.CapturePaneLines(ctx, "  ", 10)
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsKind(err, ErrKindInvalidIdentifier) {
			t.Errorf("expected ErrKindInvalidIdentifier, got %v", err)
		}
	})

	t.Run("happy_path", func(t *testing.T) {
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			// Verify -S flag has correct line count
			for i, arg := range args {
				if arg == "-S" && i+1 < len(args) && args[i+1] != "-50" {
					t.Errorf("expected -S -50, got -S %s", args[i+1])
				}
			}
			return "line 1\nline 2\n", nil
		})

		got, err := local.CapturePaneLines(ctx, "%0", 50)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "line 1\nline 2\n" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("zero_lines_uses_default", func(t *testing.T) {
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			for i, arg := range args {
				if arg == "-S" && i+1 < len(args) && args[i+1] != "-80" {
					t.Errorf("expected default -S -80, got -S %s", args[i+1])
				}
			}
			return "output\n", nil
		})

		_, err := local.CapturePaneLines(ctx, "%0", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("error_propagation", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "", errSessionNotFound()
		})

		_, err := local.CapturePaneLines(ctx, "%0", 10)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

// --- SplitPane ---

func TestSplitPane(t *testing.T) {
	ctx := context.Background()

	t.Run("vertical_split", func(t *testing.T) {
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "-h") {
				t.Errorf("expected -h flag for vertical split, got args: %v", args)
			}
			return "%42\n", nil
		})

		got, err := local.SplitPane(ctx, "%0", "vertical")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "%42" {
			t.Errorf("got %q, want %%42", got)
		}
	})

	t.Run("horizontal_split", func(t *testing.T) {
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "-v") {
				t.Errorf("expected -v flag for horizontal split, got args: %v", args)
			}
			return "%43\n", nil
		})

		got, err := local.SplitPane(ctx, "%0", "horizontal")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "%43" {
			t.Errorf("got %q, want %%43", got)
		}
	})

	t.Run("invalid_direction", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			t.Fatal("run should not be called for invalid direction")
			return "", nil
		})

		_, err := local.SplitPane(ctx, "%0", "diagonal")
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsKind(err, ErrKindInvalidIdentifier) {
			t.Errorf("expected ErrKindInvalidIdentifier, got %v", err)
		}
	})

	t.Run("run_error", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "", errCommandFailed("no space")
		})

		_, err := local.SplitPane(ctx, "%0", "vertical")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("parse_error", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "bad-output", nil // not starting with %
		})

		_, err := local.SplitPane(ctx, "%0", "vertical")
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsKind(err, ErrKindCommandFailed) {
			t.Errorf("expected ErrKindCommandFailed, got %v", err)
		}
	})
}

// --- SendKeys ---

func TestSendKeys(t *testing.T) {
	ctx := context.Background()

	t.Run("keys_and_enter", func(t *testing.T) {
		calls := 0
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			calls++
			switch calls {
			case 1:
				// First call sends keys with -l
				if args[0] != "send-keys" {
					t.Errorf("call 1: expected send-keys, got %s", args[0])
				}
				joined := strings.Join(args, " ")
				if !strings.Contains(joined, "-l") {
					t.Errorf("call 1: expected -l flag, got: %v", args)
				}
			case 2:
				// Second call sends C-m for enter
				last := args[len(args)-1]
				if last != "C-m" {
					t.Errorf("call 2: expected C-m, got %s", last)
				}
			}
			return "", nil
		})

		err := local.SendKeys(ctx, "%0", "ls", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 2 {
			t.Errorf("expected 2 run calls, got %d", calls)
		}
	})

	t.Run("enter_only", func(t *testing.T) {
		calls := 0
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			calls++
			last := args[len(args)-1]
			if last != "C-m" {
				t.Errorf("expected C-m, got %s", last)
			}
			return "", nil
		})

		err := local.SendKeys(ctx, "%0", "", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 1 {
			t.Errorf("expected 1 run call, got %d", calls)
		}
	})

	t.Run("keys_only_no_enter", func(t *testing.T) {
		calls := 0
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			calls++
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "-l") {
				t.Errorf("expected -l flag, got: %v", args)
			}
			return "", nil
		})

		err := local.SendKeys(ctx, "%0", "text", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 1 {
			t.Errorf("expected 1 run call, got %d", calls)
		}
	})

	t.Run("keys_error_stops_execution", func(t *testing.T) {
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "", errCommandFailed("pane gone")
		})

		err := local.SendKeys(ctx, "%0", "text", true)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("enter_error", func(t *testing.T) {
		calls := 0
		setRun(t, func(_ context.Context, _ ...string) (string, error) {
			calls++
			if calls == 1 {
				return "", nil
			}
			return "", errCommandFailed("pane gone")
		})

		err := local.SendKeys(ctx, "%0", "text", true)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

// --- CreateSession ---

func TestCreateSession(t *testing.T) {
	ctx := context.Background()

	t.Run("with_cwd", func(t *testing.T) {
		setCreateSessionRun(t, func(_ context.Context, args ...string) (string, error) {
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "-c /home/user") {
				t.Errorf("expected -c /home/user, got: %v", args)
			}
			if !strings.Contains(joined, "-s myapp") {
				t.Errorf("expected -s myapp, got: %v", args)
			}
			return "", nil
		})

		err := local.CreateSession(ctx, "myapp", "/home/user")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("without_cwd", func(t *testing.T) {
		setCreateSessionRun(t, func(_ context.Context, args ...string) (string, error) {
			joined := strings.Join(args, " ")
			if strings.Contains(joined, "-c") {
				t.Errorf("expected no -c flag, got: %v", args)
			}
			return "", nil
		})

		err := local.CreateSession(ctx, "myapp", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("error_propagation", func(t *testing.T) {
		setCreateSessionRun(t, func(_ context.Context, _ ...string) (string, error) {
			return "", &Error{Kind: ErrKindSessionExists, Msg: "already exists"}
		})

		err := local.CreateSession(ctx, "myapp", "")
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsKind(err, ErrKindSessionExists) {
			t.Errorf("expected ErrKindSessionExists, got %v", err)
		}
	})
}

func TestBuildCreateSessionCommand(t *testing.T) {
	tmuxArgs := []string{"new-session", "-d", "-s", "work", "-c", "/srv/work"}

	tests := []struct {
		name     string
		goos     string
		euid     int
		wantName string
		wantArgs []string
	}{
		{
			name:     "linux user scope",
			goos:     "linux",
			euid:     1000,
			wantName: "systemd-run",
			wantArgs: append(
				[]string{"--user", "--scope", "--collect", "--quiet", "--", "tmux"},
				tmuxArgs...,
			),
		},
		{
			name:     "linux system scope",
			goos:     "linux",
			euid:     0,
			wantName: "systemd-run",
			wantArgs: append(
				[]string{"--scope", "--collect", "--quiet", "--", "tmux"},
				tmuxArgs...,
			),
		},
		{
			name:     "non linux",
			goos:     "darwin",
			euid:     501,
			wantName: "tmux",
			wantArgs: tmuxArgs,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			name, args := buildCreateSessionCommand(test.goos, test.euid, tmuxArgs)
			if name != test.wantName || !slices.Equal(args, test.wantArgs) {
				t.Fatalf("buildCreateSessionCommand() = %q %#v, want %q %#v", name, args, test.wantName, test.wantArgs)
			}
		})
	}
}

// --- SetSessionMouse ---

func TestSetSessionMouse(t *testing.T) {
	ctx := context.Background()

	t.Run("enabled", func(t *testing.T) {
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			last := args[len(args)-1]
			if last != "on" {
				t.Errorf("expected 'on', got %q", last)
			}
			return "", nil
		})

		err := local.SetSessionMouse(ctx, "dev", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			last := args[len(args)-1]
			if last != "off" {
				t.Errorf("expected 'off', got %q", last)
			}
			return "", nil
		})

		err := local.SetSessionMouse(ctx, "dev", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// --- SetSessionStatus ---

func TestSetSessionStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("enabled", func(t *testing.T) {
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			last := args[len(args)-1]
			if last != "on" {
				t.Errorf("expected 'on', got %q", last)
			}
			return "", nil
		})

		err := local.SetSessionStatus(ctx, "dev", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			last := args[len(args)-1]
			if last != "off" {
				t.Errorf("expected 'off', got %q", last)
			}
			return "", nil
		})

		err := local.SetSessionStatus(ctx, "dev", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// --- NewWindow ---

func TestNewWindow(t *testing.T) {
	ctx := context.Background()

	t.Run("happy_path_with_next_index", func(t *testing.T) {
		testNewWindowHappyPathWithNextIndex(ctx, t)
	})
	t.Run("list_windows_fails_falls_back", func(t *testing.T) {
		testNewWindowListWindowsFailsFallsBack(ctx, t)
	})
	t.Run("new_window_error", func(t *testing.T) {
		testNewWindowError(ctx, t)
	})
	t.Run("new_window_parse_error", func(t *testing.T) {
		testNewWindowParseError(ctx, t)
	})
	t.Run("display_message_fails_omits_cwd", func(t *testing.T) {
		testNewWindowDisplayMessageFailsOmitsCWD(ctx, t)
	})
	t.Run("with_name_and_explicit_cwd", func(t *testing.T) {
		testNewWindowWithNameAndExplicitCWD(ctx, t)
	})
}

func testNewWindowHappyPathWithNextIndex(ctx context.Context, t *testing.T) {
	t.Helper()

	setRun(t, func(_ context.Context, args ...string) (string, error) {
		switch args[0] {
		case cmdListWindows:
			return "0\n1\n2\n", nil
		case cmdDisplayMessage:
			return "/home/dev/project\n", nil
		case cmdNewWindow:
			assertNewWindowArg(t, args, "-t", "dev:3")
			assertNewWindowArg(t, args, "-c", "/home/dev/project")
			return "@3\x1f3\x1f%15\n", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", args[0])
		}
	})

	result, err := local.NewWindow(ctx, "dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "@3" || result.Index != 3 || result.PaneID != "%15" {
		t.Errorf("result = %+v, want ID=@3 Index=3 PaneID=%%15", result)
	}
}

func testNewWindowListWindowsFailsFallsBack(ctx context.Context, t *testing.T) {
	t.Helper()

	setRun(t, func(_ context.Context, args ...string) (string, error) {
		switch args[0] {
		case cmdListWindows:
			return "", errCommandFailed("session gone")
		case cmdDisplayMessage:
			return "/home/dev/project\n", nil
		case cmdNewWindow:
			assertNewWindowArg(t, args, "-t", "dev:")
			return "@20\x1f0\x1f%20\n", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", args[0])
		}
	})

	result, err := local.NewWindow(ctx, "dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "@20" || result.Index != 0 || result.PaneID != "%20" {
		t.Errorf("result = %+v", result)
	}
}

func testNewWindowError(ctx context.Context, t *testing.T) {
	t.Helper()

	setRun(t, func(_ context.Context, args ...string) (string, error) {
		switch args[0] {
		case cmdListWindows:
			return "0\n", nil
		case cmdDisplayMessage:
			return "/tmp\n", nil
		case cmdNewWindow:
			return "", errCommandFailed("server dead")
		default:
			return "", nil
		}
	})

	_, err := local.NewWindow(ctx, "dev")
	if err == nil {
		t.Fatal("expected error")
	}
}

func testNewWindowParseError(ctx context.Context, t *testing.T) {
	t.Helper()

	setRun(t, func(_ context.Context, args ...string) (string, error) {
		switch args[0] {
		case cmdListWindows:
			return "0\n", nil
		case cmdDisplayMessage:
			return "/tmp\n", nil
		case cmdNewWindow:
			return "garbage", nil
		default:
			return "", nil
		}
	})

	_, err := local.NewWindow(ctx, "dev")
	if err == nil {
		t.Fatal("expected error")
	}
}

func testNewWindowDisplayMessageFailsOmitsCWD(ctx context.Context, t *testing.T) {
	t.Helper()

	setRun(t, func(_ context.Context, args ...string) (string, error) {
		switch args[0] {
		case cmdListWindows:
			return "0\n", nil
		case cmdDisplayMessage:
			return "", errCommandFailed("no session")
		case cmdNewWindow:
			assertNewWindowArgMissing(t, args, "-c")
			return "@10\x1f1\x1f%10\n", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", args[0])
		}
	})

	result, err := local.NewWindow(ctx, "dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "@10" || result.Index != 1 || result.PaneID != "%10" {
		t.Errorf("result = %+v, want ID=@10 Index=1 PaneID=%%10", result)
	}
}

func testNewWindowWithNameAndExplicitCWD(ctx context.Context, t *testing.T) {
	t.Helper()

	setRun(t, func(_ context.Context, args ...string) (string, error) {
		switch args[0] {
		case cmdListWindows:
			return "0\n1\n", nil
		case cmdNewWindow:
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "-n codex") {
				t.Errorf("expected -n codex, got: %v", args)
			}
			if !strings.Contains(joined, "-c /srv/api") {
				t.Errorf("expected -c /srv/api, got: %v", args)
			}
			return "@22\x1f2\x1f%22\n", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", args[0])
		}
	})

	result, err := local.NewWindowWithOptions(ctx, "dev", "codex", "/srv/api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "@22" || result.Index != 2 || result.PaneID != "%22" {
		t.Errorf("result = %+v, want ID=@22 Index=2 PaneID=%%22", result)
	}
}

func assertNewWindowArg(t *testing.T, args []string, flag string, want string) {
	t.Helper()

	for i, arg := range args {
		if arg == flag && i+1 < len(args) && args[i+1] != want {
			t.Errorf("expected %s %s, got %s", flag, want, args[i+1])
		}
	}
}

func assertNewWindowArgMissing(t *testing.T, args []string, flag string) {
	t.Helper()

	for _, arg := range args {
		if arg == flag {
			t.Errorf("should not pass %s", flag)
		}
	}
}

// --- Simple wrappers (RenameSession, KillSession, SelectWindow, etc.) ---

func TestSimpleWrappers(t *testing.T) {
	ctx := context.Background()

	t.Run("RenameSession", func(t *testing.T) {
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			if args[0] != "rename-session" {
				t.Errorf("expected rename-session, got %s", args[0])
			}
			return "", nil
		})
		if err := local.RenameSession(ctx, "old", "new"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("RenameWindow", func(t *testing.T) {
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			if args[0] != "rename-window" {
				t.Errorf("expected rename-window, got %s", args[0])
			}
			// target should be "dev:1"
			for i, a := range args {
				if a == "-t" && args[i+1] != "dev:1" {
					t.Errorf("expected target dev:1, got %s", args[i+1])
				}
			}
			return "", nil
		})
		if err := local.RenameWindow(ctx, "dev", 1, "newname"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("RenamePane", func(t *testing.T) {
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			if args[0] != "select-pane" {
				t.Errorf("expected select-pane, got %s", args[0])
			}
			return "", nil
		})
		if err := local.RenamePane(ctx, "%5", "mytitle"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("KillSession", func(t *testing.T) {
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			if args[0] != "kill-session" {
				t.Errorf("expected kill-session, got %s", args[0])
			}
			return "", nil
		})
		if err := local.KillSession(ctx, "dev"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("KillSessionByID", func(t *testing.T) {
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			if !slices.Equal(args, []string{"kill-session", "-t", "$42"}) {
				t.Fatalf("args = %#v", args)
			}
			return "", nil
		})
		if err := local.KillSessionByID(ctx, "$42"); err != nil {
			t.Fatalf("KillSessionByID() error = %v", err)
		}
	})

	t.Run("KillSessionByIDRejectsName", func(t *testing.T) {
		if err := local.KillSessionByID(ctx, "dev"); !IsKind(err, ErrKindInvalidIdentifier) {
			t.Fatalf("KillSessionByID() error = %v", err)
		}
	})

	t.Run("KillWindow", func(t *testing.T) {
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			if args[0] != "kill-window" {
				t.Errorf("expected kill-window, got %s", args[0])
			}
			return "", nil
		})
		if err := local.KillWindow(ctx, "dev", 2); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("KillPane", func(t *testing.T) {
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			if args[0] != "kill-pane" {
				t.Errorf("expected kill-pane, got %s", args[0])
			}
			return "", nil
		})
		if err := local.KillPane(ctx, "%3"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("SelectWindow", func(t *testing.T) {
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			if args[0] != "select-window" {
				t.Errorf("expected select-window, got %s", args[0])
			}
			return "", nil
		})
		if err := local.SelectWindow(ctx, "dev", 1); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("SelectPane", func(t *testing.T) {
		setRun(t, func(_ context.Context, args ...string) (string, error) {
			if args[0] != "select-pane" {
				t.Errorf("expected select-pane, got %s", args[0])
			}
			return "", nil
		})
		if err := local.SelectPane(ctx, "%7"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// --- valueAt ---

func TestValueAt(t *testing.T) {
	t.Parallel()

	parts := []string{"a", "b", "c"}

	tests := []struct {
		name string
		idx  int
		want string
	}{
		{"first", 0, "a"},
		{"middle", 1, "b"},
		{"last", 2, "c"},
		{"out_of_bounds", 5, ""},
		{"negative", -1, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := valueAt(parts, tt.idx)
			if got != tt.want {
				t.Errorf("valueAt(%v, %d) = %q, want %q", parts, tt.idx, got, tt.want)
			}
		})
	}
}

// --- parseNewWindowOutput additional cases ---

func TestParseNewWindowOutputExtended(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
		errKind ErrorKind
	}{
		{
			name:    "negative_index",
			input:   "-1\x1f%5\n",
			wantErr: true,
			errKind: ErrKindCommandFailed,
		},
		{
			name:    "non_numeric_index",
			input:   "abc\x1f%5\n",
			wantErr: true,
			errKind: ErrKindCommandFailed,
		},
		{
			name:    "missing_percent_prefix",
			input:   "3\x1f5\n",
			wantErr: true,
			errKind: ErrKindCommandFailed,
		},
		{
			name:    "too_many_fields",
			input:   "3\x1f%5\x1fextra\n",
			wantErr: true,
			errKind: ErrKindCommandFailed,
		},
		{
			name:    "empty_string",
			input:   "",
			wantErr: true,
			errKind: ErrKindCommandFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseNewWindowOutput(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if !IsKind(err, tt.errKind) {
					t.Errorf("expected kind %q, got %v", tt.errKind, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// --- parseSessionListOutput additional cases ---

func TestParseSessionListOutputExtended(t *testing.T) {
	t.Parallel()

	t.Run("empty_input", func(t *testing.T) {
		t.Parallel()
		sessions := parseSessionListOutput("")
		if len(sessions) != 0 {
			t.Fatalf("got %d sessions, want 0", len(sessions))
		}
	})

	t.Run("whitespace_only", func(t *testing.T) {
		t.Parallel()
		sessions := parseSessionListOutput("  \n  ")
		if len(sessions) != 0 {
			t.Fatalf("got %d sessions, want 0", len(sessions))
		}
	})

	t.Run("multiple_sessions", func(t *testing.T) {
		t.Parallel()
		input := "$1\x1fa\x1f1\x1f0\x1f1700000000\x1f1700000100\n$2\x1fb\x1f2\x1f1\x1f1700000200\x1f1700000300\n$3\x1fc\x1f3\x1f0\x1f1700000400\x1f1700000500\n"
		sessions := parseSessionListOutput(input)
		if len(sessions) != 3 {
			t.Fatalf("got %d sessions, want 3", len(sessions))
		}
		if sessions[2].Name != "c" || sessions[2].Windows != 3 {
			t.Errorf("sessions[2] = %+v", sessions[2])
		}
	})

	t.Run("short_line_skipped", func(t *testing.T) {
		t.Parallel()
		input := "too\x1fshort\n"
		sessions := parseSessionListOutput(input)
		if len(sessions) != 0 {
			t.Fatalf("got %d sessions, want 0", len(sessions))
		}
	})
}

// --- isServerNotRunningMessage ---

func TestIsServerNotRunningMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  string
		want bool
	}{
		{"failed_to_connect", "failed to connect to server", true},
		{"cant_connect", "can't connect to server", true},
		{"no_server_running", "no server running on /tmp/tmux-1000/default", true},
		{"error_connecting_nosuchfile", "error connecting to /tmp/tmux-1000/default (no such file or directory)", true},
		{"unrelated", "permission denied", false},
		{"error_connecting_without_nosuchfile", "error connecting to /tmp/tmux-1000/default (connection refused)", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isServerNotRunningMessage(tt.msg)
			if got != tt.want {
				t.Errorf("isServerNotRunningMessage(%q) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}
