package tmux

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestParseActivePaneCommandsOutput(t *testing.T) {
	t.Parallel()

	got := parseActivePaneCommandsOutput("dev\t1\t1\tnpx vite\tvite\ndev\t0\t0\tbash\tbash\nbad\tline\n")
	if got["dev"].Panes != 2 {
		t.Fatalf("dev panes = %d, want 2", got["dev"].Panes)
	}
	if got["dev"].Command != "vite" {
		t.Fatalf("dev command = %q, want vite", got["dev"].Command)
	}
}

func TestParseWindowAndPaneListOutput(t *testing.T) {
	t.Parallel()

	windows := parseWindowListOutput("dev\t@1\t0\tmain\t1\t2\tlayout\nshort\n")
	if len(windows) != 1 {
		t.Fatalf("windows len = %d, want 1", len(windows))
	}
	if windows[0].ID != "@1" || windows[0].Index != 0 || !windows[0].Active || windows[0].Layout != "layout" {
		t.Fatalf("window = %+v, want parsed @1 window", windows[0])
	}

	panes := parsePaneListOutput("dev\t0\t1\t%2\tlogs\t1\t/dev/pts/2\t/tmp\tbash\tvim\t10\t20\t80\t24\nother\t0\t0\t%9\tx\t0\t/dev/null\n", "dev")
	if len(panes) != 1 {
		t.Fatalf("panes len = %d, want 1", len(panes))
	}
	if panes[0].PaneID != "%2" || panes[0].CurrentPath != "/tmp" || panes[0].Left != 10 || panes[0].Height != 24 {
		t.Fatalf("pane = %+v, want parsed pane", panes[0])
	}
}

func TestSendKeysVia(t *testing.T) {
	t.Parallel()

	var calls [][]string
	runFn := func(_ context.Context, args ...string) (string, error) {
		calls = append(calls, slices.Clone(args))
		return "", nil
	}
	if err := sendKeysVia(context.Background(), runFn, "%1", "echo ok", true); err != nil {
		t.Fatalf("sendKeysVia() error = %v", err)
	}
	want := [][]string{
		{"send-keys", "-t", "%1", "-l", "echo ok"},
		{"send-keys", "-t", "%1", "C-m"},
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
	for i := range want {
		if !slices.Equal(calls[i], want[i]) {
			t.Fatalf("call %d = %#v, want %#v", i, calls[i], want[i])
		}
	}
}

func TestSendTextAndKeyViaPreserveExplicitActions(t *testing.T) {
	t.Parallel()

	var calls [][]string
	runFn := func(_ context.Context, args ...string) (string, error) {
		calls = append(calls, slices.Clone(args))
		return "", nil
	}
	if err := sendTextVia(context.Background(), runFn, "%7", "  -n value  "); err != nil {
		t.Fatalf("sendTextVia() error = %v", err)
	}
	if err := sendKeyVia(context.Background(), runFn, "%7", "Enter"); err != nil {
		t.Fatalf("sendKeyVia() error = %v", err)
	}
	want := [][]string{
		{"send-keys", "-t", "%7", "-l", "--", "  -n value  "},
		{"send-keys", "-t", "%7", "--", "Enter"},
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
	for index := range want {
		if !slices.Equal(calls[index], want[index]) {
			t.Fatalf("call %d = %#v, want %#v", index, calls[index], want[index])
		}
	}
}

func TestCapturePaneScreenVia(t *testing.T) {
	t.Parallel()

	var got []string
	runFn := func(_ context.Context, args ...string) (string, error) {
		got = slices.Clone(args)
		return "prompt$ ", nil
	}
	screen, err := capturePaneScreenVia(context.Background(), runFn, "%3")
	if err != nil {
		t.Fatalf("capturePaneScreenVia() error = %v", err)
	}
	if screen != "prompt$ " || !slices.Equal(got, []string{"capture-pane", "-p", "-t", "%3"}) {
		t.Fatalf("screen = %q, args = %#v", screen, got)
	}
}

func TestSplitPaneViaBuildsDirectionFlags(t *testing.T) {
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
				return "%2\n", nil
			}
			paneID, err := splitPaneVia(context.Background(), runFn, "%1", tt.direction)
			if err != nil {
				t.Fatalf("splitPaneVia() error = %v", err)
			}
			if paneID != "%2" {
				t.Fatalf("paneID = %q, want %%2", paneID)
			}
			if !slices.Contains(gotArgs, tt.wantFlag) {
				t.Fatalf("args = %#v, want flag %s", gotArgs, tt.wantFlag)
			}
		})
	}
}

func TestCapturePaneLinesViaValidationAndCommand(t *testing.T) {
	t.Parallel()

	if _, err := capturePaneLinesVia(context.Background(), nil, "", 10); !IsKind(err, ErrKindInvalidIdentifier) {
		t.Fatalf("empty target error = %v, want ErrKindInvalidIdentifier", err)
	}

	var gotArgs []string
	runFn := func(_ context.Context, args ...string) (string, error) {
		gotArgs = slices.Clone(args)
		return "tail", nil
	}
	out, err := capturePaneLinesVia(context.Background(), runFn, "%1", 0)
	if err != nil {
		t.Fatalf("capturePaneLinesVia() error = %v", err)
	}
	if out != "tail" {
		t.Fatalf("output = %q, want tail", out)
	}
	want := []string{"capture-pane", "-t", "%1", "-p", "-S", "-80"}
	if !slices.Equal(gotArgs, want) {
		t.Fatalf("args = %#v, want %#v", gotArgs, want)
	}
}

func TestSetSessionOptionVia(t *testing.T) {
	t.Parallel()

	var calls [][]string
	runFn := func(_ context.Context, args ...string) (string, error) {
		calls = append(calls, slices.Clone(args))
		return "", nil
	}
	if err := setSessionOptionVia(context.Background(), runFn, "dev", "mouse", true); err != nil {
		t.Fatalf("setSessionOptionVia(on) error = %v", err)
	}
	if err := setSessionOptionVia(context.Background(), runFn, "dev", "status", false); err != nil {
		t.Fatalf("setSessionOptionVia(off) error = %v", err)
	}
	want := [][]string{
		{"set-option", "-t", "dev", "mouse", "on"},
		{"set-option", "-t", "dev", "status", "off"},
	}
	for i := range want {
		if !slices.Equal(calls[i], want[i]) {
			t.Fatalf("call %d = %#v, want %#v", i, calls[i], want[i])
		}
	}
}

func TestViaHelpersPropagateRunnerError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("tmux failed")
	runFn := func(_ context.Context, _ ...string) (string, error) {
		return "", wantErr
	}
	if err := renameWindowVia(context.Background(), runFn, "dev", 1, "logs"); !errors.Is(err, wantErr) {
		t.Fatalf("renameWindowVia error = %v, want %v", err, wantErr)
	}
	if _, err := listWindowsVia(context.Background(), runFn, "dev"); !errors.Is(err, wantErr) {
		t.Fatalf("listWindowsVia error = %v, want %v", err, wantErr)
	}
	if _, err := listPanesVia(context.Background(), runFn, "dev"); !errors.Is(err, wantErr) {
		t.Fatalf("listPanesVia error = %v, want %v", err, wantErr)
	}
}
