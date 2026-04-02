package watchtower

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/tmux"
)

const testDevSession = "dev"

func TestQualifyPaneID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		user   string
		paneID string
		want   string
	}{
		{"", "%1", "%1"},
		{"", "%42", "%42"},
		{"alice", "%1", "alice:%1"},
		{"bob", "%99", "bob:%99"},
	}
	for _, tt := range tests {
		t.Run(tt.user+"/"+tt.paneID, func(t *testing.T) {
			t.Parallel()
			got := qualifyPaneID(tt.user, tt.paneID)
			if got != tt.want {
				t.Fatalf("qualifyPaneID(%q, %q) = %q, want %q", tt.user, tt.paneID, got, tt.want)
			}
		})
	}
}

func TestResolveUsersNilProvider(t *testing.T) {
	t.Parallel()

	svc := New(nil, fakeTmux{}, Options{})
	got := svc.resolveUsers(context.Background())
	if got != nil {
		t.Fatalf("resolveUsers with nil provider = %v, want nil", got)
	}
}

func TestResolveUsersCaches(t *testing.T) {
	t.Parallel()

	calls := 0
	svc := New(nil, fakeTmux{}, Options{
		UserProvider: func(context.Context) []string {
			calls++
			return []string{"alice"}
		},
	})

	ctx := context.Background()
	first := svc.resolveUsers(ctx)
	if len(first) != 1 || first[0] != "alice" {
		t.Fatalf("first resolveUsers = %v, want [alice]", first)
	}
	if calls != 1 {
		t.Fatalf("calls after first = %d, want 1", calls)
	}

	second := svc.resolveUsers(ctx)
	if len(second) != 1 || second[0] != "alice" {
		t.Fatalf("second resolveUsers = %v, want [alice]", second)
	}
	if calls != 1 {
		t.Fatalf("calls after second = %d, want 1 (cached)", calls)
	}
}

func TestCollectMultiUserSessions(t *testing.T) {
	t.Parallel()

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	now := time.Now().UTC().Truncate(time.Second)

	defaultTmux := fakeTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return []tmux.Session{{
				Name:       testDevSession,
				Windows:    1,
				Attached:   1,
				CreatedAt:  now,
				ActivityAt: now,
			}}, nil
		},
		listWindowsFn: func(_ context.Context, session string) ([]tmux.Window, error) {
			return []tmux.Window{{
				Session: session,
				Index:   0,
				Name:    "main",
				Active:  true,
				Panes:   1,
				Layout:  "layout",
			}}, nil
		},
		listPanesFn: func(_ context.Context, session string) ([]tmux.Pane, error) {
			return []tmux.Pane{{
				Session:        session,
				WindowIndex:    0,
				PaneIndex:      0,
				PaneID:         "%1",
				Title:          "shell",
				Active:         true,
				TTY:            "/dev/pts/1",
				CurrentPath:    "/tmp",
				StartCommand:   shellCommand,
				CurrentCommand: shellCommand,
			}}, nil
		},
		capturePaneLinesFn: func(context.Context, string, int) (string, error) {
			return "default output", nil
		},
	}

	svc := New(st, defaultTmux, Options{
		CaptureLines: 80,
		UserProvider: func(context.Context) []string {
			return nil
		},
	})

	if err := svc.collect(context.Background()); err != nil {
		t.Fatalf("collect: %v", err)
	}

	session, err := st.GetWatchtowerSession(context.Background(), testDevSession)
	if err != nil {
		t.Fatalf("GetWatchtowerSession(%s): %v", testDevSession, err)
	}
	if session.LastPreview != "default output" {
		t.Fatalf("session.LastPreview = %q, want %q", session.LastPreview, "default output")
	}
}

func TestCollectMultiUserPaneIDNamespacing(t *testing.T) {
	t.Parallel()

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	now := time.Now().UTC().Truncate(time.Second)

	// Simulate a multi-user session directly by calling collectSession with
	// a taggedSession that has user set.
	userTmux := fakeTmux{
		listWindowsFn: func(_ context.Context, _ string) ([]tmux.Window, error) {
			return []tmux.Window{{
				Session: "remote",
				Index:   0,
				Name:    "editor",
				Active:  true,
				Panes:   1,
				Layout:  "layout",
			}}, nil
		},
		listPanesFn: func(_ context.Context, _ string) ([]tmux.Pane, error) {
			return []tmux.Pane{{
				Session:        "remote",
				WindowIndex:    0,
				PaneIndex:      0,
				PaneID:         "%1",
				Title:          "vim",
				Active:         true,
				TTY:            "/dev/pts/2",
				CurrentPath:    "/home/alice",
				StartCommand:   shellCommand,
				CurrentCommand: "vim",
			}}, nil
		},
		capturePaneLinesFn: func(context.Context, string, int) (string, error) {
			return "alice output", nil
		},
	}

	svc := New(st, fakeTmux{}, Options{CaptureLines: 80})

	ts := taggedSession{
		Session: tmux.Session{
			Name:       "remote",
			Windows:    1,
			Attached:   0,
			CreatedAt:  now,
			ActivityAt: now,
		},
		client: userTmux,
		user:   "alice",
	}

	keep, _, _, _, err := svc.collectSession(context.Background(), ts)
	if err != nil {
		t.Fatalf("collectSession: %v", err)
	}
	if !keep {
		t.Fatal("collectSession returned keep=false")
	}

	// Verify pane was stored with qualified ID.
	panes, err := st.ListWatchtowerPanes(context.Background(), "remote")
	if err != nil {
		t.Fatalf("ListWatchtowerPanes(remote): %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("panes len = %d, want 1", len(panes))
	}
	wantPaneID := "alice:%1"
	if panes[0].PaneID != wantPaneID {
		t.Fatalf("pane ID = %q, want %q", panes[0].PaneID, wantPaneID)
	}
	if panes[0].TailPreview != "alice output" {
		t.Fatalf("pane preview = %q, want %q", panes[0].TailPreview, "alice output")
	}

	// Verify session preview pane ID is qualified.
	session, err := st.GetWatchtowerSession(context.Background(), "remote")
	if err != nil {
		t.Fatalf("GetWatchtowerSession(remote): %v", err)
	}
	if session.LastPreviewPaneID != wantPaneID {
		t.Fatalf("session.LastPreviewPaneID = %q, want %q", session.LastPreviewPaneID, wantPaneID)
	}
}

func TestCollectMultiUserCapturePaneUsesRawID(t *testing.T) {
	t.Parallel()

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	now := time.Now().UTC().Truncate(time.Second)

	// Track which pane ID was passed to CapturePaneLines.
	var capturedTarget string
	userTmux := fakeTmux{
		listWindowsFn: func(_ context.Context, _ string) ([]tmux.Window, error) {
			return []tmux.Window{{
				Session: "remote",
				Index:   0,
				Name:    "main",
				Active:  true,
				Panes:   1,
				Layout:  "layout",
			}}, nil
		},
		listPanesFn: func(_ context.Context, _ string) ([]tmux.Pane, error) {
			return []tmux.Pane{{
				Session:        "remote",
				WindowIndex:    0,
				PaneIndex:      0,
				PaneID:         "%5",
				Title:          "shell",
				Active:         true,
				TTY:            "/dev/pts/3",
				CurrentPath:    "/home/bob",
				StartCommand:   shellCommand,
				CurrentCommand: shellCommand,
			}}, nil
		},
		capturePaneLinesFn: func(_ context.Context, target string, _ int) (string, error) {
			capturedTarget = target
			return helloWorldPreview, nil
		},
	}

	svc := New(st, fakeTmux{}, Options{CaptureLines: 80})

	ts := taggedSession{
		Session: tmux.Session{
			Name:       "remote",
			Windows:    1,
			CreatedAt:  now,
			ActivityAt: now,
		},
		client: userTmux,
		user:   "bob",
	}

	_, _, _, _, err := svc.collectSession(context.Background(), ts)
	if err != nil {
		t.Fatalf("collectSession: %v", err)
	}

	// The raw pane ID (without user prefix) should be passed to tmux.
	if capturedTarget != "%5" {
		t.Fatalf("CapturePaneLines target = %q, want %%5 (raw, not qualified)", capturedTarget)
	}

	// But the store should have the qualified ID.
	panes, err := st.ListWatchtowerPanes(context.Background(), "remote")
	if err != nil {
		t.Fatalf("ListWatchtowerPanes(remote): %v", err)
	}
	if len(panes) != 1 || panes[0].PaneID != "bob:%5" {
		t.Fatalf("stored pane ID = %q, want %q", panes[0].PaneID, "bob:%5")
	}
}

func TestCollectDefaultUserPaneIDNotQualified(t *testing.T) {
	t.Parallel()

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	now := time.Now().UTC().Truncate(time.Second)
	fake := fakeTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return []tmux.Session{{
				Name:       testDevSession,
				Windows:    1,
				Attached:   1,
				CreatedAt:  now,
				ActivityAt: now,
			}}, nil
		},
		listWindowsFn: func(context.Context, string) ([]tmux.Window, error) {
			return []tmux.Window{{
				Session: testDevSession,
				Index:   0,
				Name:    "main",
				Active:  true,
				Panes:   1,
				Layout:  "layout",
			}}, nil
		},
		listPanesFn: func(context.Context, string) ([]tmux.Pane, error) {
			return []tmux.Pane{{
				Session:        testDevSession,
				WindowIndex:    0,
				PaneIndex:      0,
				PaneID:         "%1",
				Title:          "shell",
				Active:         true,
				TTY:            "/dev/pts/1",
				CurrentPath:    "/tmp",
				StartCommand:   shellCommand,
				CurrentCommand: shellCommand,
			}}, nil
		},
		capturePaneLinesFn: func(context.Context, string, int) (string, error) {
			return helloWorldPreview, nil
		},
	}

	svc := New(st, fake, Options{CaptureLines: 80})
	if err := svc.collect(context.Background()); err != nil {
		t.Fatalf("collect: %v", err)
	}

	// Default user pane IDs should NOT be qualified.
	panes, err := st.ListWatchtowerPanes(context.Background(), testDevSession)
	if err != nil {
		t.Fatalf("ListWatchtowerPanes(%s): %v", testDevSession, err)
	}
	if len(panes) != 1 || panes[0].PaneID != "%1" {
		t.Fatalf("default user pane ID = %q, want %%1", panes[0].PaneID)
	}
}

func TestListCollectSessionsServerNotRunning(t *testing.T) {
	t.Parallel()

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	// Default server not running, no multi-user providers.
	fake := fakeTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return nil, &tmux.Error{Kind: tmux.ErrKindServerNotRunning, Msg: "no server running"}
		},
	}

	svc := New(st, fake, Options{})
	tagged, proceed, err := svc.listCollectSessions(context.Background())
	if err != nil {
		t.Fatalf("listCollectSessions: %v", err)
	}
	if proceed {
		t.Fatal("expected proceed=false when no sessions from any source")
	}
	if len(tagged) != 0 {
		t.Fatalf("tagged len = %d, want 0", len(tagged))
	}
}

func TestCollectMultiUserBothSources(t *testing.T) {
	t.Parallel()

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	now := time.Now().UTC().Truncate(time.Second)

	defaultTmux := fakeTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return []tmux.Session{{
				Name:       testDevSession,
				Windows:    1,
				Attached:   1,
				CreatedAt:  now,
				ActivityAt: now,
			}}, nil
		},
		listWindowsFn: func(_ context.Context, session string) ([]tmux.Window, error) {
			return []tmux.Window{{
				Session: session,
				Index:   0,
				Name:    "main",
				Active:  true,
				Panes:   1,
				Layout:  "layout",
			}}, nil
		},
		listPanesFn: func(_ context.Context, session string) ([]tmux.Pane, error) {
			return []tmux.Pane{{
				Session:        session,
				WindowIndex:    0,
				PaneIndex:      0,
				PaneID:         "%1",
				Title:          "shell",
				Active:         true,
				TTY:            "/dev/pts/1",
				CurrentPath:    "/tmp",
				StartCommand:   shellCommand,
				CurrentCommand: shellCommand,
			}}, nil
		},
		capturePaneLinesFn: func(context.Context, string, int) (string, error) {
			return helloWorldPreview + " " + strconv.FormatInt(time.Now().UnixNano(), 10), nil
		},
	}

	svc := New(st, defaultTmux, Options{
		CaptureLines: 80,
		UserProvider: func(context.Context) []string {
			return nil
		},
	})

	if err := svc.collect(context.Background()); err != nil {
		t.Fatalf("collect: %v", err)
	}

	session, err := st.GetWatchtowerSession(context.Background(), testDevSession)
	if err != nil {
		t.Fatalf("GetWatchtowerSession(%s): %v", testDevSession, err)
	}
	if session.SessionName != testDevSession {
		t.Fatalf("session name = %q, want %s", session.SessionName, testDevSession)
	}
}
