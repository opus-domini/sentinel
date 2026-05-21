package tmux

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestCapturePaneVia(t *testing.T) {
	t.Parallel()

	t.Run("returns last non-empty line", func(t *testing.T) {
		t.Parallel()

		var gotArgs []string
		runFn := func(_ context.Context, args ...string) (string, error) {
			gotArgs = slices.Clone(args)
			return "first\nlast line\n\n  \n", nil
		}
		out, err := capturePane(context.Background(), runFn, "dev")
		if err != nil {
			t.Fatalf("capturePane() error = %v", err)
		}
		if out != "last line" {
			t.Fatalf("output = %q, want %q", out, "last line")
		}
		want := []string{"capture-pane", "-t", "dev:", "-p", "-S", "-3"}
		if !slices.Equal(gotArgs, want) {
			t.Fatalf("args = %#v, want %#v", gotArgs, want)
		}
	})

	t.Run("runner error yields empty string and no error", func(t *testing.T) {
		t.Parallel()

		runFn := func(_ context.Context, _ ...string) (string, error) {
			return "", errors.New("boom")
		}
		out, err := capturePane(context.Background(), runFn, "dev")
		if err != nil || out != "" {
			t.Fatalf("capturePane() = %q, %v, want \"\", nil", out, err)
		}
	})

	t.Run("all blank lines yield empty string", func(t *testing.T) {
		t.Parallel()

		runFn := func(_ context.Context, _ ...string) (string, error) {
			return "\n   \n\t\n", nil
		}
		out, err := capturePane(context.Background(), runFn, "dev")
		if err != nil || out != "" {
			t.Fatalf("capturePane() = %q, %v, want \"\", nil", out, err)
		}
	})
}

func TestSelectWindowVia(t *testing.T) {
	t.Parallel()

	var gotArgs []string
	runFn := func(_ context.Context, args ...string) (string, error) {
		gotArgs = slices.Clone(args)
		return "", nil
	}
	if err := selectWindowVia(context.Background(), runFn, "dev", 2); err != nil {
		t.Fatalf("selectWindowVia() error = %v", err)
	}
	want := []string{"select-window", "-t", "dev:2"}
	if !slices.Equal(gotArgs, want) {
		t.Fatalf("args = %#v, want %#v", gotArgs, want)
	}

	wantErr := errors.New("fail")
	errRun := func(_ context.Context, _ ...string) (string, error) { return "", wantErr }
	if err := selectWindowVia(context.Background(), errRun, "dev", 1); !errors.Is(err, wantErr) {
		t.Fatalf("selectWindowVia() error = %v, want %v", err, wantErr)
	}
}

func TestSelectLayoutVia(t *testing.T) {
	t.Parallel()

	var gotArgs []string
	runFn := func(_ context.Context, args ...string) (string, error) {
		gotArgs = slices.Clone(args)
		return "", nil
	}
	if err := selectLayoutVia(context.Background(), runFn, "dev", 3, "tiled"); err != nil {
		t.Fatalf("selectLayoutVia() error = %v", err)
	}
	want := []string{"select-layout", "-t", "dev:3", "tiled"}
	if !slices.Equal(gotArgs, want) {
		t.Fatalf("args = %#v, want %#v", gotArgs, want)
	}
}

func TestKillWindowVia(t *testing.T) {
	t.Parallel()

	var gotArgs []string
	runFn := func(_ context.Context, args ...string) (string, error) {
		gotArgs = slices.Clone(args)
		return "", nil
	}
	if err := killWindowVia(context.Background(), runFn, "dev", 4); err != nil {
		t.Fatalf("killWindowVia() error = %v", err)
	}
	want := []string{"kill-window", "-t", "dev:4"}
	if !slices.Equal(gotArgs, want) {
		t.Fatalf("args = %#v, want %#v", gotArgs, want)
	}

	wantErr := errors.New("fail")
	errRun := func(_ context.Context, _ ...string) (string, error) { return "", wantErr }
	if err := killWindowVia(context.Background(), errRun, "dev", 1); !errors.Is(err, wantErr) {
		t.Fatalf("killWindowVia() error = %v, want %v", err, wantErr)
	}
}

func TestNewWindowAtVia(t *testing.T) {
	t.Parallel()

	t.Run("with name and cwd", func(t *testing.T) {
		t.Parallel()

		var gotArgs []string
		runFn := func(_ context.Context, args ...string) (string, error) {
			gotArgs = slices.Clone(args)
			return "", nil
		}
		if err := newWindowAtVia(context.Background(), runFn, "dev", 2, "logs", "/tmp"); err != nil {
			t.Fatalf("newWindowAtVia() error = %v", err)
		}
		for _, want := range []string{"-d", "-t", "dev:2", "-n", "logs", "-c", "/tmp"} {
			if !slices.Contains(gotArgs, want) {
				t.Fatalf("args = %#v, missing %q", gotArgs, want)
			}
		}
	})

	t.Run("without name or cwd", func(t *testing.T) {
		t.Parallel()

		var gotArgs []string
		runFn := func(_ context.Context, args ...string) (string, error) {
			gotArgs = slices.Clone(args)
			return "", nil
		}
		if err := newWindowAtVia(context.Background(), runFn, "dev", 0, "  ", " "); err != nil {
			t.Fatalf("newWindowAtVia() error = %v", err)
		}
		if slices.Contains(gotArgs, "-n") || slices.Contains(gotArgs, "-c") {
			t.Fatalf("args = %#v, should omit -n and -c", gotArgs)
		}
	})
}

func TestSplitPaneInVia(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		direction string
		wantFlag  string
	}{
		{name: "vertical", direction: dirVertical, wantFlag: "-h"},
		{name: "horizontal", direction: dirHorizontal, wantFlag: "-v"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotArgs []string
			runFn := func(_ context.Context, args ...string) (string, error) {
				gotArgs = slices.Clone(args)
				return "  %7\n", nil
			}
			paneID, err := splitPaneInVia(context.Background(), runFn, "%1", tt.direction, "/work")
			if err != nil {
				t.Fatalf("splitPaneInVia() error = %v", err)
			}
			if paneID != "%7" {
				t.Fatalf("paneID = %q, want %%7", paneID)
			}
			if !slices.Contains(gotArgs, tt.wantFlag) {
				t.Fatalf("args = %#v, want flag %s", gotArgs, tt.wantFlag)
			}
			if !slices.Contains(gotArgs, "-c") {
				t.Fatalf("args = %#v, want -c for cwd", gotArgs)
			}
		})
	}

	t.Run("invalid direction", func(t *testing.T) {
		t.Parallel()

		runFn := func(_ context.Context, _ ...string) (string, error) { return "", nil }
		if _, err := splitPaneInVia(context.Background(), runFn, "%1", "diagonal", ""); !IsKind(err, ErrKindInvalidIdentifier) {
			t.Fatalf("splitPaneInVia() error = %v, want ErrKindInvalidIdentifier", err)
		}
	})

	t.Run("runner error", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("fail")
		runFn := func(_ context.Context, _ ...string) (string, error) { return "", wantErr }
		if _, err := splitPaneInVia(context.Background(), runFn, "%1", dirVertical, ""); !errors.Is(err, wantErr) {
			t.Fatalf("splitPaneInVia() error = %v, want %v", err, wantErr)
		}
	})
}

func TestNewWindowWithOptionsVia(t *testing.T) {
	t.Parallel()

	t.Run("resolves next index and explicit cwd", func(t *testing.T) {
		t.Parallel()

		var newWindowArgs []string
		runFn := func(_ context.Context, args ...string) (string, error) {
			switch args[0] {
			case "list-windows":
				return "0\n1\n2\n", nil
			case cmdNewWindow:
				newWindowArgs = slices.Clone(args)
				return "@9\t3\t%4\n", nil
			default:
				return "", nil
			}
		}
		res, err := newWindowWithOptionsVia(context.Background(), runFn, "dev", "logs", "/tmp")
		if err != nil {
			t.Fatalf("newWindowWithOptionsVia() error = %v", err)
		}
		if res.ID != "@9" || res.Index != 3 || res.PaneID != "%4" {
			t.Fatalf("result = %+v, want @9/3/%%4", res)
		}
		for _, want := range []string{"-t", "dev:3", "-n", "logs", "-c", "/tmp"} {
			if !slices.Contains(newWindowArgs, want) {
				t.Fatalf("new-window args = %#v, missing %q", newWindowArgs, want)
			}
		}
	})

	t.Run("resolves cwd via display-message when cwd empty", func(t *testing.T) {
		t.Parallel()

		var newWindowArgs []string
		runFn := func(_ context.Context, args ...string) (string, error) {
			switch args[0] {
			case "list-windows":
				return "0\n", nil
			case "display-message":
				return "/resolved/path\n", nil
			case cmdNewWindow:
				newWindowArgs = slices.Clone(args)
				return "@2\t1\t%2", nil
			default:
				return "", nil
			}
		}
		res, err := newWindowWithOptionsVia(context.Background(), runFn, "dev", "", "")
		if err != nil {
			t.Fatalf("newWindowWithOptionsVia() error = %v", err)
		}
		if res.ID != "@2" {
			t.Fatalf("result = %+v, want @2", res)
		}
		idx := slices.Index(newWindowArgs, "-c")
		if idx < 0 || idx+1 >= len(newWindowArgs) || newWindowArgs[idx+1] != "/resolved/path" {
			t.Fatalf("new-window args = %#v, want -c /resolved/path", newWindowArgs)
		}
		if slices.Contains(newWindowArgs, "-n") {
			t.Fatalf("new-window args = %#v, should omit -n", newWindowArgs)
		}
	})

	t.Run("falls back to session target when list-windows fails", func(t *testing.T) {
		t.Parallel()

		var newWindowArgs []string
		runFn := func(_ context.Context, args ...string) (string, error) {
			switch args[0] {
			case "list-windows":
				return "", errors.New("no server")
			case cmdNewWindow:
				newWindowArgs = slices.Clone(args)
				return "@1\t0\t%1", nil
			default:
				return "", nil
			}
		}
		if _, err := newWindowWithOptionsVia(context.Background(), runFn, "dev", "x", "/tmp"); err != nil {
			t.Fatalf("newWindowWithOptionsVia() error = %v", err)
		}
		idx := slices.Index(newWindowArgs, "-t")
		if idx < 0 || newWindowArgs[idx+1] != "dev:" {
			t.Fatalf("new-window args = %#v, want -t dev:", newWindowArgs)
		}
	})

	t.Run("propagates new-window error", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("create failed")
		runFn := func(_ context.Context, args ...string) (string, error) {
			if args[0] == cmdNewWindow {
				return "", wantErr
			}
			return "0\n", nil
		}
		if _, err := newWindowWithOptionsVia(context.Background(), runFn, "dev", "x", "/tmp"); !errors.Is(err, wantErr) {
			t.Fatalf("newWindowWithOptionsVia() error = %v, want %v", err, wantErr)
		}
	})
}

func TestReorderWindowsVia(t *testing.T) {
	t.Parallel()

	noopRun := func(_ context.Context, _ ...string) (string, error) { return "", nil }

	t.Run("validation errors", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name    string
			session string
			ids     []string
		}{
			{name: "empty session", session: "  ", ids: []string{"@1"}},
			{name: "empty id list", session: "dev", ids: nil},
			{name: "blank id", session: "dev", ids: []string{"@1", "  "}},
			{name: "duplicate id", session: "dev", ids: []string{"@1", "@1"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				err := reorderWindowsVia(context.Background(), noopRun, tc.session, tc.ids)
				if !IsKind(err, ErrKindInvalidIdentifier) {
					t.Fatalf("error = %v, want ErrKindInvalidIdentifier", err)
				}
			})
		}
	})

	t.Run("propagates list-windows error", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("list failed")
		runFn := func(_ context.Context, _ ...string) (string, error) { return "", wantErr }
		if err := reorderWindowsVia(context.Background(), runFn, "dev", []string{"@1"}); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
	})

	t.Run("length mismatch", func(t *testing.T) {
		t.Parallel()

		runFn := func(_ context.Context, _ ...string) (string, error) {
			return "dev\t@1\t0\tw0\t1\t1\tlay\ndev\t@2\t1\tw1\t0\t1\tlay", nil
		}
		err := reorderWindowsVia(context.Background(), runFn, "dev", []string{"@1"})
		if !IsKind(err, ErrKindInvalidIdentifier) {
			t.Fatalf("error = %v, want ErrKindInvalidIdentifier", err)
		}
	})

	t.Run("unknown window id", func(t *testing.T) {
		t.Parallel()

		runFn := func(_ context.Context, _ ...string) (string, error) {
			return "dev\t@1\t0\tw0\t1\t1\tlay\ndev\t@2\t1\tw1\t0\t1\tlay", nil
		}
		err := reorderWindowsVia(context.Background(), runFn, "dev", []string{"@1", "@9"})
		if !IsKind(err, ErrKindInvalidIdentifier) {
			t.Fatalf("error = %v, want ErrKindInvalidIdentifier", err)
		}
	})

	t.Run("swaps windows into requested order", func(t *testing.T) {
		t.Parallel()

		var swaps [][]string
		runFn := func(_ context.Context, args ...string) (string, error) {
			if args[0] == "list-windows" {
				return "dev\t@1\t0\tw0\t1\t1\tlay\ndev\t@2\t1\tw1\t0\t1\tlay", nil
			}
			swaps = append(swaps, slices.Clone(args))
			return "", nil
		}
		if err := reorderWindowsVia(context.Background(), runFn, "dev", []string{"@2", "@1"}); err != nil {
			t.Fatalf("reorderWindowsVia() error = %v", err)
		}
		if len(swaps) != 1 {
			t.Fatalf("swaps = %#v, want exactly one swap", swaps)
		}
		if swaps[0][0] != "swap-window" {
			t.Fatalf("swap call = %#v, want swap-window", swaps[0])
		}
	})

	t.Run("already ordered performs no swaps", func(t *testing.T) {
		t.Parallel()

		var swaps int
		runFn := func(_ context.Context, args ...string) (string, error) {
			if args[0] == "list-windows" {
				return "dev\t@1\t0\tw0\t1\t1\tlay\ndev\t@2\t1\tw1\t0\t1\tlay", nil
			}
			swaps++
			return "", nil
		}
		if err := reorderWindowsVia(context.Background(), runFn, "dev", []string{"@1", "@2"}); err != nil {
			t.Fatalf("reorderWindowsVia() error = %v", err)
		}
		if swaps != 0 {
			t.Fatalf("swaps = %d, want 0", swaps)
		}
	})

	t.Run("blank live window id", func(t *testing.T) {
		t.Parallel()

		runFn := func(_ context.Context, _ ...string) (string, error) {
			return "dev\t\t0\tw0\t1\t1\tlay\ndev\t@2\t1\tw1\t0\t1\tlay", nil
		}
		err := reorderWindowsVia(context.Background(), runFn, "dev", []string{"@1", "@2"})
		if !IsKind(err, ErrKindInvalidIdentifier) {
			t.Fatalf("error = %v, want ErrKindInvalidIdentifier", err)
		}
	})

	t.Run("propagates swap-window error", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("swap failed")
		runFn := func(_ context.Context, args ...string) (string, error) {
			if args[0] == "list-windows" {
				return "dev\t@1\t0\tw0\t1\t1\tlay\ndev\t@2\t1\tw1\t0\t1\tlay", nil
			}
			return "", wantErr
		}
		if err := reorderWindowsVia(context.Background(), runFn, "dev", []string{"@2", "@1"}); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
	})
}
