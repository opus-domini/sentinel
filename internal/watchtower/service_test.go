package watchtower

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

type fakeTmux struct {
	listSessionsFn     func(context.Context) ([]tmux.Session, error)
	listWindowsFn      func(context.Context, string) ([]tmux.Window, error)
	listPanesFn        func(context.Context, string) ([]tmux.Pane, error)
	capturePaneLinesFn func(context.Context, string, int) (string, error)
}

func (f fakeTmux) ListSessions(ctx context.Context) ([]tmux.Session, error) {
	if f.listSessionsFn != nil {
		return f.listSessionsFn(ctx)
	}
	return []tmux.Session{}, nil
}

func (f fakeTmux) ListWindows(ctx context.Context, session string) ([]tmux.Window, error) {
	if f.listWindowsFn != nil {
		return f.listWindowsFn(ctx, session)
	}
	return []tmux.Window{}, nil
}

func (f fakeTmux) ListPanes(ctx context.Context, session string) ([]tmux.Pane, error) {
	if f.listPanesFn != nil {
		return f.listPanesFn(ctx, session)
	}
	return []tmux.Pane{}, nil
}

func (f fakeTmux) CapturePaneLines(ctx context.Context, target string, lines int) (string, error) {
	if f.capturePaneLinesFn != nil {
		return f.capturePaneLinesFn(ctx, target, lines)
	}
	return "", nil
}

func TestServiceStartStop(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	svc := New(nil, fakeTmux{}, Options{
		TickInterval: 10 * time.Millisecond,
		Collect: func(context.Context) error {
			calls.Add(1)
			return nil
		},
	})

	svc.Start(context.Background())
	time.Sleep(35 * time.Millisecond)

	if calls.Load() == 0 {
		t.Fatal("collect was not called")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	svc.Stop(stopCtx)
}

func TestServiceStartStopIdempotent(t *testing.T) {
	t.Parallel()

	svc := New(nil, fakeTmux{}, Options{
		TickInterval: 5 * time.Millisecond,
	})

	svc.Start(context.Background())
	svc.Start(context.Background())

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	svc.Stop(stopCtx)
	svc.Stop(stopCtx)
}

func TestCollectWritesProjections(t *testing.T) {
	t.Parallel()

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	now := time.Now().UTC().Truncate(time.Second)
	fake := fakeTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return []tmux.Session{{
				Name:       "dev",
				Windows:    1,
				Attached:   1,
				CreatedAt:  now,
				ActivityAt: now,
			}}, nil
		},
		listWindowsFn: func(context.Context, string) ([]tmux.Window, error) {
			return []tmux.Window{{
				Session: "dev",
				Index:   0,
				Name:    "main",
				Active:  true,
				Panes:   1,
				Layout:  "layout",
			}}, nil
		},
		listPanesFn: func(context.Context, string) ([]tmux.Pane, error) {
			return []tmux.Pane{{
				Session:        "dev",
				WindowIndex:    0,
				PaneIndex:      0,
				PaneID:         "%1",
				Title:          "shell",
				Active:         true,
				TTY:            "/dev/pts/1",
				CurrentPath:    "/tmp",
				StartCommand:   "zsh",
				CurrentCommand: "zsh",
			}}, nil
		},
		capturePaneLinesFn: func(context.Context, string, int) (string, error) {
			return "\n\nhello world\n", nil
		},
	}

	svc := New(st, fake, Options{CaptureLines: 80})
	if err := svc.collect(context.Background()); err != nil {
		t.Fatalf("collect: %v", err)
	}

	session, err := st.GetWatchtowerSession(context.Background(), "dev")
	if err != nil {
		t.Fatalf("GetWatchtowerSession(dev): %v", err)
	}
	if session.UnreadPanes != 1 || session.UnreadWindows != 1 {
		t.Fatalf("unexpected unread counters: %+v", session)
	}
	if session.LastPreview != "hello world" {
		t.Fatalf("session.LastPreview = %q, want %q", session.LastPreview, "hello world")
	}

	windows, err := st.ListWatchtowerWindows(context.Background(), "dev")
	if err != nil {
		t.Fatalf("ListWatchtowerWindows(dev): %v", err)
	}
	if len(windows) != 1 || !windows[0].HasUnread || windows[0].UnreadPanes != 1 {
		t.Fatalf("unexpected windows projection: %+v", windows)
	}

	panes, err := st.ListWatchtowerPanes(context.Background(), "dev")
	if err != nil {
		t.Fatalf("ListWatchtowerPanes(dev): %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("panes len = %d, want 1", len(panes))
	}
	if panes[0].Revision != 1 || panes[0].SeenRevision != 0 {
		t.Fatalf("unexpected pane revisions: %+v", panes[0])
	}
}

func TestCollectPublishesSessionsEventOnActivity(t *testing.T) {
	t.Parallel()

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	now := time.Now().UTC().Truncate(time.Second)
	fake := fakeTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return []tmux.Session{{
				Name:       "dev",
				Windows:    1,
				Attached:   1,
				CreatedAt:  now,
				ActivityAt: now,
			}}, nil
		},
		listWindowsFn: func(context.Context, string) ([]tmux.Window, error) {
			return []tmux.Window{{
				Session: "dev",
				Index:   0,
				Name:    "main",
				Active:  true,
				Panes:   1,
				Layout:  "layout",
			}}, nil
		},
		listPanesFn: func(context.Context, string) ([]tmux.Pane, error) {
			return []tmux.Pane{{
				Session:        "dev",
				WindowIndex:    0,
				PaneIndex:      0,
				PaneID:         "%1",
				Title:          "shell",
				Active:         true,
				TTY:            "/dev/pts/1",
				CurrentPath:    "/tmp",
				StartCommand:   "zsh",
				CurrentCommand: "zsh",
			}}, nil
		},
		capturePaneLinesFn: func(context.Context, string, int) (string, error) {
			return "hello world", nil
		},
	}

	var publishCount atomic.Int32
	var activityEventCount atomic.Int32
	svc := New(st, fake, Options{
		Publish: func(eventType string, payload map[string]any) {
			switch eventType {
			case "tmux.sessions.updated":
				if payload["action"] != "activity" {
					t.Fatalf("unexpected action payload: %+v", payload)
				}
				publishCount.Add(1)
			case "tmux.activity.updated":
				if _, ok := payload["globalRev"]; !ok {
					t.Fatalf("missing globalRev payload: %+v", payload)
				}
				activityEventCount.Add(1)
			default:
				t.Fatalf("unexpected event type: %s", eventType)
			}
		},
	})

	if err := svc.collect(context.Background()); err != nil {
		t.Fatalf("collect #1: %v", err)
	}
	if got := publishCount.Load(); got != 1 {
		t.Fatalf("publish count after first collect = %d, want 1", got)
	}
	if got := activityEventCount.Load(); got != 1 {
		t.Fatalf("activity event count after first collect = %d, want 1", got)
	}

	if err := svc.collect(context.Background()); err != nil {
		t.Fatalf("collect #2: %v", err)
	}
	if got := publishCount.Load(); got != 1 {
		t.Fatalf("publish count after second collect = %d, want 1", got)
	}
	if got := activityEventCount.Load(); got != 1 {
		t.Fatalf("activity event count after second collect = %d, want 1", got)
	}
}

func TestCollectUpdatesGlobalRevAndJournal(t *testing.T) {
	t.Parallel()

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	now := time.Now().UTC().Truncate(time.Second)
	var captureCount atomic.Int32
	fake := fakeTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return []tmux.Session{{
				Name:       "dev",
				Windows:    1,
				Attached:   1,
				CreatedAt:  now,
				ActivityAt: now,
			}}, nil
		},
		listWindowsFn: func(context.Context, string) ([]tmux.Window, error) {
			return []tmux.Window{{
				Session: "dev",
				Index:   0,
				Name:    "main",
				Active:  true,
				Panes:   1,
				Layout:  "layout",
			}}, nil
		},
		listPanesFn: func(context.Context, string) ([]tmux.Pane, error) {
			return []tmux.Pane{{
				Session:        "dev",
				WindowIndex:    0,
				PaneIndex:      0,
				PaneID:         "%1",
				Title:          "shell",
				Active:         true,
				TTY:            "/dev/pts/1",
				CurrentPath:    "/tmp",
				StartCommand:   "zsh",
				CurrentCommand: "zsh",
			}}, nil
		},
		capturePaneLinesFn: func(context.Context, string, int) (string, error) {
			if captureCount.Add(1) == 1 {
				return "line one", nil
			}
			return "line two", nil
		},
	}

	svc := New(st, fake, Options{})
	if err := svc.collect(context.Background()); err != nil {
		t.Fatalf("collect #1: %v", err)
	}
	if err := svc.collect(context.Background()); err != nil {
		t.Fatalf("collect #2: %v", err)
	}

	rawRev, err := st.GetWatchtowerRuntimeValue(context.Background(), "global_rev")
	if err != nil {
		t.Fatalf("GetWatchtowerRuntimeValue(global_rev): %v", err)
	}
	if rawRev != "2" {
		t.Fatalf("global_rev = %q, want 2", rawRev)
	}

	entries, err := st.ListWatchtowerJournalSince(context.Background(), 0, 10)
	if err != nil {
		t.Fatalf("ListWatchtowerJournalSince: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("journal entries len = %d, want 2", len(entries))
	}
	if entries[0].GlobalRev != 1 || entries[1].GlobalRev != 2 {
		t.Fatalf("unexpected journal revisions: %+v", entries)
	}
}

func TestCollectRecordsRuntimeMetricsSuccess(t *testing.T) {
	t.Parallel()

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	now := time.Now().UTC().Truncate(time.Second)
	fake := fakeTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return []tmux.Session{{
				Name:       "dev",
				Windows:    1,
				Attached:   1,
				CreatedAt:  now,
				ActivityAt: now,
			}}, nil
		},
		listWindowsFn: func(context.Context, string) ([]tmux.Window, error) {
			return []tmux.Window{{Session: "dev", Index: 0, Name: "main", Active: true, Panes: 1, Layout: "layout"}}, nil
		},
		listPanesFn: func(context.Context, string) ([]tmux.Pane, error) {
			return []tmux.Pane{{Session: "dev", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", Active: true}}, nil
		},
		capturePaneLinesFn: func(context.Context, string, int) (string, error) {
			return "line", nil
		},
	}

	svc := New(st, fake, Options{})
	if err := svc.collect(context.Background()); err != nil {
		t.Fatalf("collect: %v", err)
	}

	for key, want := range map[string]string{
		runtimeCollectTotalKey:       "1",
		runtimeCollectErrorsTotalKey: "",
		runtimeLastCollectSessKey:    "1",
		runtimeLastCollectChangedKey: "1",
		runtimeLastCollectErrorKey:   "",
	} {
		got, err := st.GetWatchtowerRuntimeValue(context.Background(), key)
		if err != nil {
			t.Fatalf("GetWatchtowerRuntimeValue(%s): %v", key, err)
		}
		if want == "" {
			if key == runtimeCollectErrorsTotalKey {
				if got != "" && got != "0" {
					t.Fatalf("%s = %q, want empty or 0", key, got)
				}
			}
			continue
		}
		if got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestCollectRecordsRuntimeMetricsError(t *testing.T) {
	t.Parallel()

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	fake := fakeTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return nil, &tmux.Error{Kind: tmux.ErrKindCommandFailed, Msg: "boom"}
		},
	}

	svc := New(st, fake, Options{})
	err := svc.collect(context.Background())
	if err == nil {
		t.Fatalf("collect err = nil, want error")
	}

	total, err := st.GetWatchtowerRuntimeValue(context.Background(), runtimeCollectTotalKey)
	if err != nil {
		t.Fatalf("GetWatchtowerRuntimeValue(%s): %v", runtimeCollectTotalKey, err)
	}
	if total != "1" {
		t.Fatalf("%s = %q, want 1", runtimeCollectTotalKey, total)
	}
	errorsTotal, err := st.GetWatchtowerRuntimeValue(context.Background(), runtimeCollectErrorsTotalKey)
	if err != nil {
		t.Fatalf("GetWatchtowerRuntimeValue(%s): %v", runtimeCollectErrorsTotalKey, err)
	}
	if errorsTotal != "1" {
		t.Fatalf("%s = %q, want 1", runtimeCollectErrorsTotalKey, errorsTotal)
	}
	lastErr, err := st.GetWatchtowerRuntimeValue(context.Background(), runtimeLastCollectErrorKey)
	if err != nil {
		t.Fatalf("GetWatchtowerRuntimeValue(%s): %v", runtimeLastCollectErrorKey, err)
	}
	if strings.TrimSpace(lastErr) == "" {
		t.Fatalf("%s is empty, want collect error message", runtimeLastCollectErrorKey)
	}
}

func TestCollectIncrementsRevisionOnOutputChange(t *testing.T) {
	t.Parallel()

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	now := time.Now().UTC().Truncate(time.Second)
	var captureCount atomic.Int32
	fake := fakeTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return []tmux.Session{{
				Name:       "dev",
				Windows:    1,
				Attached:   1,
				CreatedAt:  now,
				ActivityAt: now,
			}}, nil
		},
		listWindowsFn: func(context.Context, string) ([]tmux.Window, error) {
			return []tmux.Window{{
				Session: "dev",
				Index:   0,
				Name:    "main",
				Active:  true,
				Panes:   1,
				Layout:  "layout",
			}}, nil
		},
		listPanesFn: func(context.Context, string) ([]tmux.Pane, error) {
			return []tmux.Pane{{
				Session:        "dev",
				WindowIndex:    0,
				PaneIndex:      0,
				PaneID:         "%1",
				Title:          "shell",
				Active:         true,
				TTY:            "/dev/pts/1",
				CurrentPath:    "/tmp",
				StartCommand:   "zsh",
				CurrentCommand: "zsh",
			}}, nil
		},
		capturePaneLinesFn: func(context.Context, string, int) (string, error) {
			if captureCount.Add(1) == 1 {
				return "first", nil
			}
			return "second", nil
		},
	}

	svc := New(st, fake, Options{CaptureLines: 80})
	if err := svc.collect(context.Background()); err != nil {
		t.Fatalf("collect #1: %v", err)
	}
	if err := svc.collect(context.Background()); err != nil {
		t.Fatalf("collect #2: %v", err)
	}

	panes, err := st.ListWatchtowerPanes(context.Background(), "dev")
	if err != nil {
		t.Fatalf("ListWatchtowerPanes(dev): %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("panes len = %d, want 1", len(panes))
	}
	if panes[0].Revision != 2 {
		t.Fatalf("pane revision = %d, want 2", panes[0].Revision)
	}
	if panes[0].TailPreview != "second" {
		t.Fatalf("pane tail preview = %q, want second", panes[0].TailPreview)
	}
}

func TestCollectPurgesRemovedSessions(t *testing.T) {
	t.Parallel()

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	now := time.Now().UTC().Truncate(time.Second)
	var withSession atomic.Bool
	withSession.Store(true)

	fake := fakeTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			if !withSession.Load() {
				return []tmux.Session{}, nil
			}
			return []tmux.Session{{
				Name:       "dev",
				Windows:    1,
				Attached:   1,
				CreatedAt:  now,
				ActivityAt: now,
			}}, nil
		},
		listWindowsFn: func(context.Context, string) ([]tmux.Window, error) {
			return []tmux.Window{{Session: "dev", Index: 0, Name: "main", Active: true, Panes: 1, Layout: "layout"}}, nil
		},
		listPanesFn: func(context.Context, string) ([]tmux.Pane, error) {
			return []tmux.Pane{{Session: "dev", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", Active: true}}, nil
		},
		capturePaneLinesFn: func(context.Context, string, int) (string, error) {
			return "hello", nil
		},
	}

	svc := New(st, fake, Options{})
	if err := svc.collect(context.Background()); err != nil {
		t.Fatalf("collect #1: %v", err)
	}

	withSession.Store(false)
	if err := svc.collect(context.Background()); err != nil {
		t.Fatalf("collect #2: %v", err)
	}

	sessions, err := st.ListWatchtowerSessions(context.Background())
	if err != nil {
		t.Fatalf("ListWatchtowerSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions len after purge = %d, want 0", len(sessions))
	}
}

func TestCollectHandlesTmuxServerDown(t *testing.T) {
	t.Parallel()

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	now := time.Now().UTC().Truncate(time.Second)
	if err := st.UpsertWatchtowerSession(context.Background(), store.WatchtowerSessionWrite{
		SessionName:   "dev",
		Attached:      1,
		Windows:       1,
		Panes:         1,
		ActivityAt:    now,
		LastPreview:   "existing",
		LastPreviewAt: now,
		Rev:           1,
	}); err != nil {
		t.Fatalf("seed UpsertWatchtowerSession: %v", err)
	}

	fake := fakeTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return nil, &tmux.Error{Kind: tmux.ErrKindServerNotRunning, Msg: "server down"}
		},
	}
	svc := New(st, fake, Options{})
	if err := svc.collect(context.Background()); err != nil {
		t.Fatalf("collect should tolerate server down: %v", err)
	}

	session, err := st.GetWatchtowerSession(context.Background(), "dev")
	if err != nil {
		t.Fatalf("GetWatchtowerSession(dev): %v", err)
	}
	if session.LastPreview != "existing" {
		t.Fatalf("session preview changed unexpectedly: %+v", session)
	}
}

func newWatchtowerTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "watchtower-test.db")
	st, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New(%s): %v", dbPath, err)
	}
	return st
}
