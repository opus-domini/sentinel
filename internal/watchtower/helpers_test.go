package watchtower

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

func TestNormalizeRuntimeCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current string
		start   string
		want    string
	}{
		{
			name:    "prefers current command",
			current: "  npm run dev  ",
			start:   "bash",
			want:    "npm run dev",
		},
		{
			name:    "falls back to start command",
			current: "   ",
			start:   "  zsh  ",
			want:    "zsh",
		},
		{
			name:    "treats dash as empty",
			current: "-",
			start:   "bash",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := normalizeRuntimeCommand(tt.current, tt.start); got != tt.want {
				t.Fatalf("normalizeRuntimeCommand(%q, %q) = %q, want %q", tt.current, tt.start, got, tt.want)
			}
		})
	}
}

func TestIsShellLikeCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		command string
		want    bool
	}{
		{command: "", want: true},
		{command: " bash ", want: true},
		{command: "ZSH", want: true},
		{command: "tmux", want: true},
		{command: "python", want: false},
		{command: "npm run dev", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(strings.TrimSpace(tt.command), func(t *testing.T) {
			t.Parallel()

			if got := isShellLikeCommand(tt.command); got != tt.want {
				t.Fatalf("isShellLikeCommand(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}

func TestTimelineLastLine(t *testing.T) {
	t.Parallel()

	if got := timelineLastLine(" \nline one\n  final line  \n"); got != "final line" {
		t.Fatalf("timelineLastLine returned %q, want %q", got, "final line")
	}

	longLine := strings.Repeat("x", 300)
	got := timelineLastLine("first\n" + longLine)
	if len(got) != 240 {
		t.Fatalf("timelineLastLine length = %d, want 240", len(got))
	}
	if got != longLine[:240] {
		t.Fatalf("timelineLastLine truncated value mismatch")
	}
}

func TestTimelineMetadataJSON(t *testing.T) {
	t.Parallel()

	if got := string(timelineMetadataJSON(nil)); got != "{}" {
		t.Fatalf("timelineMetadataJSON(nil) = %q, want {}", got)
	}

	payload := timelineMetadataJSON(map[string]any{
		"pane":     "%1",
		"exitCode": 1,
	})
	if !json.Valid(payload) {
		t.Fatalf("timelineMetadataJSON returned invalid JSON: %s", string(payload))
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded["pane"] != "%1" || decoded["exitCode"] != float64(1) {
		t.Fatalf("decoded metadata = %#v", decoded)
	}

	fallback := timelineMetadataJSON(map[string]any{
		"bad": make(chan int),
	})
	if string(fallback) != "{}" {
		t.Fatalf("timelineMetadataJSON fallback = %q, want {}", string(fallback))
	}
}

func TestManagedWindowRuntimeMap(t *testing.T) {
	t.Parallel()

	rows := []store.ManagedTmuxWindow{
		{ID: "skip-blank", TmuxWindowID: "   "},
		{ID: "managed-1", TmuxWindowID: " @1 ", WindowName: "Codex"},
	}

	got := managedWindowRuntimeMap(rows)
	if len(got) != 1 {
		t.Fatalf("managedWindowRuntimeMap len = %d, want 1", len(got))
	}
	if got["@1"].ID != "managed-1" {
		t.Fatalf("managedWindowRuntimeMap[@1] = %#v", got["@1"])
	}
}

func TestWindowNamesByIndex(t *testing.T) {
	t.Parallel()

	windows := []tmux.Window{
		{ID: "@1", Index: 0, Name: "shell"},
		{ID: "@2", Index: 1, Name: "runner"},
		{ID: "@3", Index: 2, Name: "   "},
	}
	managed := map[string]store.ManagedTmuxWindow{
		"@1": {WindowName: "Codex"},
		"@2": {WindowName: "   "},
	}

	got := windowNamesByIndex(windows, managed)
	want := map[int]string{
		0: "Codex",
		1: "runner",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("windowNamesByIndex = %#v, want %#v", got, want)
	}
}

func TestSortedNonEmptySessionNames(t *testing.T) {
	t.Parallel()

	got := sortedNonEmptySessionNames(map[string]struct{}{
		" beta ": {},
		"":       {},
		"alpha":  {},
		"   ":    {},
	})
	want := []string{"alpha", "beta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sortedNonEmptySessionNames = %#v, want %#v", got, want)
	}
}

func TestCurrentGlobalRev(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	svc := New(st, fakeTmux{}, Options{})

	rev, err := svc.currentGlobalRev(ctx)
	if err != nil {
		t.Fatalf("currentGlobalRev(empty): %v", err)
	}
	if rev != 0 {
		t.Fatalf("currentGlobalRev(empty) = %d, want 0", rev)
	}

	if err := st.SetWatchtowerRuntimeValue(ctx, runtimeGlobalRevKey, " 41 "); err != nil {
		t.Fatalf("SetWatchtowerRuntimeValue(valid): %v", err)
	}
	rev, err = svc.currentGlobalRev(ctx)
	if err != nil {
		t.Fatalf("currentGlobalRev(valid): %v", err)
	}
	if rev != 41 {
		t.Fatalf("currentGlobalRev(valid) = %d, want 41", rev)
	}

	if err := st.SetWatchtowerRuntimeValue(ctx, runtimeGlobalRevKey, "nope"); err != nil {
		t.Fatalf("SetWatchtowerRuntimeValue(invalid): %v", err)
	}
	if _, err := svc.currentGlobalRev(ctx); err == nil {
		t.Fatal("currentGlobalRev(invalid) expected parse error")
	}
}

func TestBuildSessionActivityPatches(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	now := time.Date(2026, time.April, 23, 10, 0, 0, 0, time.UTC)
	if err := st.UpsertWatchtowerSession(ctx, store.WatchtowerSessionWrite{
		SessionName:   testDevSession,
		Attached:      1,
		Windows:       2,
		Panes:         3,
		ActivityAt:    now,
		LastPreview:   "deploy failed",
		UnreadWindows: 1,
		UnreadPanes:   2,
		Rev:           7,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerSession: %v", err)
	}

	svc := New(st, fakeTmux{}, Options{})
	patches := svc.buildSessionActivityPatches(ctx, []string{" ", "missing", " " + testDevSession + " "})
	if len(patches) != 1 {
		t.Fatalf("buildSessionActivityPatches len = %d, want 1", len(patches))
	}

	patch := patches[0]
	if patch["name"] != testDevSession {
		t.Fatalf("patch name = %#v, want %s", patch["name"], testDevSession)
	}
	if patch["lastContent"] != "deploy failed" {
		t.Fatalf("patch lastContent = %#v, want deploy failed", patch["lastContent"])
	}
	if patch["activityAt"] != now.Format(time.RFC3339) {
		t.Fatalf("patch activityAt = %#v, want %q", patch["activityAt"], now.Format(time.RFC3339))
	}
	if patch["rev"] != int64(7) {
		t.Fatalf("patch rev = %#v, want 7", patch["rev"])
	}
}

func TestBuildInspectorActivityPatches(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	now := time.Date(2026, time.April, 23, 11, 0, 0, 0, time.UTC)
	if err := st.UpsertWatchtowerWindow(ctx, store.WatchtowerWindowWrite{
		SessionName:      testDevSession,
		TmuxWindowID:     "@1",
		WindowIndex:      0,
		Name:             "main",
		Active:           true,
		Layout:           "layout",
		WindowActivityAt: now,
		UnreadPanes:      1,
		HasUnread:        true,
		Rev:              5,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerWindow: %v", err)
	}
	if err := st.UpsertWatchtowerPane(ctx, store.WatchtowerPaneWrite{
		PaneID:         "%1",
		SessionName:    testDevSession,
		WindowIndex:    0,
		PaneIndex:      0,
		Title:          "shell",
		Active:         true,
		TTY:            "/dev/pts/1",
		CurrentPath:    "/repo",
		StartCommand:   "zsh",
		CurrentCommand: "vim",
		TailPreview:    "build output",
		Revision:       4,
		SeenRevision:   2,
		ChangedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerPane: %v", err)
	}
	managed, err := st.CreateManagedTmuxWindow(ctx, store.ManagedTmuxWindowWrite{
		SessionName:     testDevSession,
		LauncherID:      "launcher-codex",
		LauncherName:    "Codex",
		Icon:            "bot",
		Command:         "codex",
		CwdMode:         "fixed",
		CwdValue:        "/repo",
		ResolvedCwd:     "/repo",
		WindowName:      "Codex",
		TmuxWindowID:    "@1",
		LastWindowIndex: 0,
	})
	if err != nil {
		t.Fatalf("CreateManagedTmuxWindow: %v", err)
	}

	svc := New(st, fakeTmux{}, Options{})
	patches := svc.buildInspectorActivityPatches(ctx, []string{" ", testDevSession, "missing"})
	if len(patches) != 2 {
		t.Fatalf("buildInspectorActivityPatches len = %d, want 2", len(patches))
	}

	patch := patches[0]
	if patch["session"] != testDevSession {
		t.Fatalf("patch session = %#v, want %s", patch["session"], testDevSession)
	}

	windows, ok := patch["windows"].([]map[string]any)
	if !ok || len(windows) != 1 {
		t.Fatalf("windows patch = %#v", patch["windows"])
	}
	window := windows[0]
	if window["displayName"] != "Codex" {
		t.Fatalf("window displayName = %#v, want Codex", window["displayName"])
	}
	if window["displayIcon"] != "bot" {
		t.Fatalf("window displayIcon = %#v, want bot", window["displayIcon"])
	}
	if window["managed"] != true {
		t.Fatalf("window managed = %#v, want true", window["managed"])
	}
	if window["managedWindowId"] != managed.ID {
		t.Fatalf("window managedWindowId = %#v, want %q", window["managedWindowId"], managed.ID)
	}
	if window["launcherId"] != "launcher-codex" {
		t.Fatalf("window launcherId = %#v, want launcher-codex", window["launcherId"])
	}

	panes, ok := patch["panes"].([]map[string]any)
	if !ok || len(panes) != 1 {
		t.Fatalf("panes patch = %#v", patch["panes"])
	}
	pane := panes[0]
	if pane["paneId"] != "%1" {
		t.Fatalf("pane paneId = %#v, want %%1", pane["paneId"])
	}
	if pane["hasUnread"] != true {
		t.Fatalf("pane hasUnread = %#v, want true", pane["hasUnread"])
	}
	if pane["currentCommand"] != "vim" {
		t.Fatalf("pane currentCommand = %#v, want vim", pane["currentCommand"])
	}

	missing := patches[1]
	if missing["session"] != "missing" {
		t.Fatalf("missing patch session = %#v, want missing", missing["session"])
	}
	if windows, ok := missing["windows"].([]map[string]any); !ok || len(windows) != 0 {
		t.Fatalf("missing patch windows = %#v, want empty slice", missing["windows"])
	}
	if panes, ok := missing["panes"].([]map[string]any); !ok || len(panes) != 0 {
		t.Fatalf("missing patch panes = %#v, want empty slice", missing["panes"])
	}
}

func TestReconcileManagedTmuxWindows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	blankRuntime, err := st.CreateManagedTmuxWindow(ctx, store.ManagedTmuxWindowWrite{
		SessionName:     testDevSession,
		LauncherID:      "launcher-blank",
		LauncherName:    "Blank",
		Icon:            "terminal",
		Command:         "bash",
		CwdMode:         "fixed",
		CwdValue:        "/tmp",
		ResolvedCwd:     "/tmp",
		WindowName:      "Blank Runtime",
		LastWindowIndex: -1,
	})
	if err != nil {
		t.Fatalf("CreateManagedTmuxWindow(blankRuntime): %v", err)
	}
	updatedRuntime, err := st.CreateManagedTmuxWindow(ctx, store.ManagedTmuxWindowWrite{
		SessionName:     testDevSession,
		LauncherID:      "launcher-codex",
		LauncherName:    "Codex",
		Icon:            "bot",
		Command:         "codex",
		CwdMode:         "fixed",
		CwdValue:        "/repo",
		ResolvedCwd:     "/repo",
		WindowName:      "Codex",
		TmuxWindowID:    "@1",
		LastWindowIndex: 4,
	})
	if err != nil {
		t.Fatalf("CreateManagedTmuxWindow(updatedRuntime): %v", err)
	}
	staleRuntime, err := st.CreateManagedTmuxWindow(ctx, store.ManagedTmuxWindowWrite{
		SessionName:     testDevSession,
		LauncherID:      "launcher-stale",
		LauncherName:    "Stale",
		Icon:            "ghost",
		Command:         "sleep 1",
		CwdMode:         "fixed",
		CwdValue:        "/srv",
		ResolvedCwd:     "/srv",
		WindowName:      "Stale",
		TmuxWindowID:    "@stale",
		LastWindowIndex: 9,
	})
	if err != nil {
		t.Fatalf("CreateManagedTmuxWindow(staleRuntime): %v", err)
	}

	svc := New(st, fakeTmux{}, Options{})
	filtered, err := svc.reconcileManagedTmuxWindows(ctx, testDevSession, []tmux.Window{
		{ID: "@1", Index: 2, Name: "main"},
	})
	if err != nil {
		t.Fatalf("reconcileManagedTmuxWindows: %v", err)
	}

	if len(filtered) != 2 {
		t.Fatalf("filtered len = %d, want 2", len(filtered))
	}
	filteredByID := make(map[string]store.ManagedTmuxWindow, len(filtered))
	for _, row := range filtered {
		filteredByID[row.ID] = row
	}
	if _, ok := filteredByID[blankRuntime.ID]; !ok {
		t.Fatalf("blank runtime row was removed: %#v", filtered)
	}
	if got := filteredByID[updatedRuntime.ID].LastWindowIndex; got != 2 {
		t.Fatalf("updated runtime LastWindowIndex = %d, want 2", got)
	}
	if _, ok := filteredByID[staleRuntime.ID]; ok {
		t.Fatalf("stale runtime row still present: %#v", filteredByID[staleRuntime.ID])
	}

	rows, err := st.ListManagedTmuxWindowsBySession(ctx, testDevSession)
	if err != nil {
		t.Fatalf("ListManagedTmuxWindowsBySession: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("stored managed windows len = %d, want 2", len(rows))
	}
}

func TestLoadFocusedPanes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	now := time.Date(2026, time.April, 23, 12, 0, 0, 0, time.UTC)
	for _, row := range []store.WatchtowerPresenceWrite{
		{
			TerminalID:  "term-focused",
			SessionName: testDevSession,
			WindowIndex: 0,
			PaneID:      "%1",
			Visible:     true,
			Focused:     true,
			UpdatedAt:   now,
			ExpiresAt:   now.Add(time.Minute),
		},
		{
			TerminalID:  "term-hidden",
			SessionName: testDevSession,
			WindowIndex: 0,
			PaneID:      "%2",
			Visible:     false,
			Focused:     true,
			UpdatedAt:   now,
			ExpiresAt:   now.Add(time.Minute),
		},
		{
			TerminalID:  "term-unfocused",
			SessionName: testDevSession,
			WindowIndex: 0,
			PaneID:      "%3",
			Visible:     true,
			Focused:     false,
			UpdatedAt:   now,
			ExpiresAt:   now.Add(time.Minute),
		},
		{
			TerminalID:  "term-expired",
			SessionName: testDevSession,
			WindowIndex: 0,
			PaneID:      "%4",
			Visible:     true,
			Focused:     true,
			UpdatedAt:   now,
			ExpiresAt:   now.Add(-time.Minute),
		},
		{
			TerminalID:  "term-blank-pane",
			SessionName: testDevSession,
			WindowIndex: 0,
			PaneID:      "   ",
			Visible:     true,
			Focused:     true,
			UpdatedAt:   now,
			ExpiresAt:   now.Add(time.Minute),
		},
	} {
		if err := st.UpsertWatchtowerPresence(ctx, row); err != nil {
			t.Fatalf("UpsertWatchtowerPresence(%s): %v", row.TerminalID, err)
		}
	}

	svc := New(st, fakeTmux{}, Options{})
	focused, err := svc.loadFocusedPanes(ctx, testDevSession, now)
	if err != nil {
		t.Fatalf("loadFocusedPanes: %v", err)
	}

	want := map[string]bool{"%1": true}
	if !reflect.DeepEqual(focused, want) {
		t.Fatalf("loadFocusedPanes = %#v, want %#v", focused, want)
	}
}

func TestListCollectSessions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	t.Run("returns default sessions when tmux is reachable", func(t *testing.T) {
		t.Parallel()

		svc := New(st, fakeTmux{
			listSessionsFn: func(context.Context) ([]tmux.Session, error) {
				return []tmux.Session{{Name: testDevSession}}, nil
			},
		}, Options{})

		tagged, proceed, err := svc.listCollectSessions(ctx)
		if err != nil {
			t.Fatalf("listCollectSessions: %v", err)
		}
		if !proceed {
			t.Fatal("proceed = false, want true")
		}
		if len(tagged) != 1 || tagged[0].Name != testDevSession || tagged[0].user != "" {
			t.Fatalf("tagged = %#v", tagged)
		}
	})

	t.Run("returns unexpected tmux errors", func(t *testing.T) {
		t.Parallel()

		svc := New(st, fakeTmux{
			listSessionsFn: func(context.Context) ([]tmux.Session, error) {
				return nil, errors.New("boom")
			},
		}, Options{})

		if _, _, err := svc.listCollectSessions(ctx); err == nil {
			t.Fatal("listCollectSessions expected error")
		}
	})
}
