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

const (
	helloWorldPreview = "hello world"
	shellCommand      = "zsh"
)

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
				StartCommand:   shellCommand,
				CurrentCommand: shellCommand,
			}}, nil
		},
		capturePaneLinesFn: func(context.Context, string, int) (string, error) {
			return "\n\n" + helloWorldPreview + "\n", nil
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
	if session.LastPreview != helloWorldPreview {
		t.Fatalf("session.LastPreview = %q, want %q", session.LastPreview, helloWorldPreview)
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
	const sessionName = "dev"

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	now := time.Now().UTC().Truncate(time.Second)
	fake := fakeTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return []tmux.Session{{
				Name:       sessionName,
				Windows:    1,
				Attached:   1,
				CreatedAt:  now,
				ActivityAt: now,
			}}, nil
		},
		listWindowsFn: func(context.Context, string) ([]tmux.Window, error) {
			return []tmux.Window{{
				Session: sessionName,
				Index:   0,
				Name:    "main",
				Active:  true,
				Panes:   1,
				Layout:  "layout",
			}}, nil
		},
		listPanesFn: func(context.Context, string) ([]tmux.Pane, error) {
			return []tmux.Pane{{
				Session:        sessionName,
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

	asserter := newJournalPublishAsserter(t, sessionName)
	svc := New(st, fake, Options{
		Publish: asserter.Handle,
	})

	if err := svc.collect(context.Background()); err != nil {
		t.Fatalf("collect #1: %v", err)
	}
	asserter.AssertCounts(t, 1, 1, "first collect")

	if err := svc.collect(context.Background()); err != nil {
		t.Fatalf("collect #2: %v", err)
	}
	asserter.AssertCounts(t, 1, 1, "second collect")
}

type journalPublishAsserter struct {
	t           *testing.T
	sessionName string
	publish     atomic.Int32
	activity    atomic.Int32
}

func newJournalPublishAsserter(t *testing.T, sessionName string) *journalPublishAsserter {
	t.Helper()
	return &journalPublishAsserter{t: t, sessionName: sessionName}
}

func (a *journalPublishAsserter) Handle(eventType string, payload map[string]any) {
	a.t.Helper()
	a.assertSessionAndInspectorPatches(payload)
	switch eventType {
	case "tmux.sessions.updated":
		if payload["action"] != "activity" {
			a.t.Fatalf("unexpected action payload: %+v", payload)
		}
		if _, ok := payload["globalRev"]; !ok {
			a.t.Fatalf("missing globalRev payload: %+v", payload)
		}
		a.publish.Add(1)
	case "tmux.activity.updated":
		if _, ok := payload["globalRev"]; !ok {
			a.t.Fatalf("missing globalRev payload: %+v", payload)
		}
		a.activity.Add(1)
	default:
		a.t.Fatalf("unexpected event type: %s", eventType)
	}
}

func (a *journalPublishAsserter) assertSessionAndInspectorPatches(payload map[string]any) {
	a.t.Helper()
	patches := mustSessionPatches(a.t, payload)
	if len(patches) != 1 {
		a.t.Fatalf("sessionPatches len = %d, want 1", len(patches))
	}
	patch := patches[0]
	if patch["name"] != a.sessionName {
		a.t.Fatalf("session patch name = %v, want %s", patch["name"], a.sessionName)
	}
	if patch["lastContent"] != helloWorldPreview || patch["unreadPanes"] != 1 {
		a.t.Fatalf("unexpected session patch payload: %+v", patch)
	}
	inspector := mustInspectorPatch(a.t, payload, a.sessionName)
	rawPanes, ok := inspector["panes"].([]map[string]any)
	if !ok || len(rawPanes) != 1 || rawPanes[0]["paneId"] != "%1" {
		a.t.Fatalf("unexpected inspector panes payload: %T(%v)", inspector["panes"], inspector["panes"])
	}
}

func mustSessionPatches(t *testing.T, payload map[string]any) []map[string]any {
	t.Helper()
	rawPatches, ok := payload["sessionPatches"]
	if !ok {
		t.Fatalf("missing sessionPatches payload: %+v", payload)
	}
	patches, ok := rawPatches.([]map[string]any)
	if !ok {
		t.Fatalf("sessionPatches type = %T, want []map[string]any", rawPatches)
	}
	return patches
}

func mustInspectorPatch(t *testing.T, payload map[string]any, sessionName string) map[string]any {
	t.Helper()
	rawInspector, ok := payload["inspectorPatches"]
	if !ok {
		t.Fatalf("missing inspectorPatches payload: %+v", payload)
	}
	inspectorPatches, ok := rawInspector.([]map[string]any)
	if !ok || len(inspectorPatches) != 1 {
		t.Fatalf("inspectorPatches invalid payload: %T(%v)", rawInspector, rawInspector)
	}
	inspector := inspectorPatches[0]
	if inspector["session"] != sessionName {
		t.Fatalf("inspector patch session = %v, want %s", inspector["session"], sessionName)
	}
	rawWindows, ok := inspector["windows"].([]map[string]any)
	if !ok || len(rawWindows) != 1 {
		t.Fatalf("inspector windows = %T(%v), want len=1", inspector["windows"], inspector["windows"])
	}
	return inspector
}

func (a *journalPublishAsserter) AssertCounts(t *testing.T, wantPublish, wantActivity int32, stage string) {
	t.Helper()
	if got := a.publish.Load(); got != wantPublish {
		t.Fatalf("publish count after %s = %d, want %d", stage, got, wantPublish)
	}
	if got := a.activity.Load(); got != wantActivity {
		t.Fatalf("journal update count after %s = %d, want %d", stage, got, wantActivity)
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
				StartCommand:   shellCommand,
				CurrentCommand: shellCommand,
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
				StartCommand:   shellCommand,
				CurrentCommand: shellCommand,
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

func TestCollectPreservesPreviousTailOnCaptureError(t *testing.T) {
	t.Parallel()

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	previous := "previous output"
	if err := st.UpsertWatchtowerSession(ctx, store.WatchtowerSessionWrite{
		SessionName:       "dev",
		Attached:          1,
		Windows:           1,
		Panes:             1,
		ActivityAt:        now,
		LastPreview:       previous,
		LastPreviewAt:     now,
		LastPreviewPaneID: "%1",
		Rev:               7,
		UpdatedAt:         now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerSession: %v", err)
	}
	if err := st.UpsertWatchtowerWindow(ctx, store.WatchtowerWindowWrite{
		SessionName:      "dev",
		TmuxWindowID:     "@1",
		WindowIndex:      0,
		Name:             "main",
		Active:           true,
		Layout:           "layout",
		WindowActivityAt: now,
		Rev:              3,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerWindow: %v", err)
	}
	if err := st.UpsertWatchtowerPane(ctx, store.WatchtowerPaneWrite{
		PaneID:         "%1",
		SessionName:    "dev",
		WindowIndex:    0,
		PaneIndex:      0,
		Title:          "shell",
		Active:         true,
		CurrentCommand: shellCommand,
		TailHash:       hashPaneTail(previous),
		TailPreview:    previous,
		TailCapturedAt: now,
		Revision:       5,
		SeenRevision:   5,
		ChangedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerPane: %v", err)
	}

	fake := fakeTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return []tmux.Session{{Name: "dev", Windows: 1, Attached: 1, ActivityAt: now}}, nil
		},
		listWindowsFn: func(context.Context, string) ([]tmux.Window, error) {
			return []tmux.Window{{ID: "@1", Session: "dev", Index: 0, Name: "main", Active: true, Panes: 1, Layout: "layout"}}, nil
		},
		listPanesFn: func(context.Context, string) ([]tmux.Pane, error) {
			return []tmux.Pane{{Session: "dev", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", Title: "shell", Active: true, CurrentCommand: shellCommand}}, nil
		},
		capturePaneLinesFn: func(context.Context, string, int) (string, error) {
			return "", &tmux.Error{Kind: tmux.ErrKindCommandFailed, Msg: "capture failed"}
		},
	}

	svc := New(st, fake, Options{CaptureLines: 80})
	if err := svc.collect(ctx); err != nil {
		t.Fatalf("collect: %v", err)
	}

	panes, err := st.ListWatchtowerPanes(ctx, "dev")
	if err != nil {
		t.Fatalf("ListWatchtowerPanes(dev): %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("panes len = %d, want 1", len(panes))
	}
	if panes[0].Revision != 5 || panes[0].SeenRevision != 5 || panes[0].TailPreview != previous {
		t.Fatalf("pane should preserve previous tail on capture error: %+v", panes[0])
	}
	session, err := st.GetWatchtowerSession(ctx, "dev")
	if err != nil {
		t.Fatalf("GetWatchtowerSession(dev): %v", err)
	}
	if session.Rev != 7 || session.LastPreview != previous {
		t.Fatalf("session should remain unchanged on capture error: %+v", session)
	}
}

func TestPersistActivityJournalAdvancesGlobalRevision(t *testing.T) {
	t.Parallel()

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()
	ctx := context.Background()
	svc := New(st, fakeTmux{}, Options{})

	rev, err := svc.persistActivityJournal(ctx, nil)
	if err != nil {
		t.Fatalf("persistActivityJournal(empty): %v", err)
	}
	if rev != 0 {
		t.Fatalf("empty revision = %d, want 0", rev)
	}

	if err := st.SetWatchtowerRuntimeValue(ctx, runtimeGlobalRevKey, "4"); err != nil {
		t.Fatalf("SetWatchtowerRuntimeValue: %v", err)
	}
	rev, err = svc.persistActivityJournal(ctx, []string{"dev", "prod"})
	if err != nil {
		t.Fatalf("persistActivityJournal: %v", err)
	}
	if rev != 6 {
		t.Fatalf("revision = %d, want 6", rev)
	}
	runtimeRev, err := st.GetWatchtowerRuntimeValue(ctx, runtimeGlobalRevKey)
	if err != nil {
		t.Fatalf("GetWatchtowerRuntimeValue(%s): %v", runtimeGlobalRevKey, err)
	}
	if runtimeRev != "6" {
		t.Fatalf("runtime revision = %q, want 6", runtimeRev)
	}

	rows, err := st.ListWatchtowerJournalSince(ctx, 4, 10)
	if err != nil {
		t.Fatalf("ListWatchtowerJournalSince: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("journal len = %d, want 2", len(rows))
	}
	for i, row := range rows {
		wantRev := int64(5 + i)
		wantSession := []string{"dev", "prod"}[i]
		if row.GlobalRev != wantRev || row.Session != wantSession || row.EntityType != "session" || row.ChangeKind != "activity" || row.WindowIdx != -1 {
			t.Fatalf("journal row %d = %+v", i, row)
		}
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

func TestCollectPublishesInspectorEventOnActiveWindowChange(t *testing.T) {
	t.Parallel()

	st := newWatchtowerTestStore(t)
	defer func() { _ = st.Close() }()

	now := time.Now().UTC().Truncate(time.Second)

	// Window 0 starts as active; after first collect, window 1 becomes active.
	var collectCount atomic.Int32
	fake := fakeTmux{
		listSessionsFn: func(context.Context) ([]tmux.Session, error) {
			return []tmux.Session{{
				Name:       "dev",
				Windows:    2,
				Attached:   1,
				CreatedAt:  now,
				ActivityAt: now,
			}}, nil
		},
		listWindowsFn: func(context.Context, string) ([]tmux.Window, error) {
			secondCollect := collectCount.Load() >= 1
			return []tmux.Window{
				{Session: "dev", Index: 0, Name: "main", Active: !secondCollect, Panes: 1, Layout: "layout"},
				{Session: "dev", Index: 1, Name: "alt", Active: secondCollect, Panes: 1, Layout: "layout"},
			}, nil
		},
		listPanesFn: func(context.Context, string) ([]tmux.Pane, error) {
			secondCollect := collectCount.Load() >= 1
			return []tmux.Pane{
				{Session: "dev", WindowIndex: 0, PaneIndex: 0, PaneID: "%1", Active: !secondCollect},
				{Session: "dev", WindowIndex: 1, PaneIndex: 0, PaneID: "%2", Active: secondCollect},
			}, nil
		},
		capturePaneLinesFn: func(context.Context, string, int) (string, error) {
			return "output", nil
		},
	}

	var inspectorEvents atomic.Int32
	var inspectorSession, inspectorAction atomic.Value
	svc := New(st, fake, Options{
		Publish: func(eventType string, payload map[string]any) {
			if eventType == "tmux.inspector.updated" {
				inspectorEvents.Add(1)
				inspectorSession.Store(payload["session"])
				inspectorAction.Store(payload["action"])
			}
		},
	})

	// First collect: establishes baseline. No active window change yet.
	if err := svc.collect(context.Background()); err != nil {
		t.Fatalf("collect #1: %v", err)
	}
	if got := inspectorEvents.Load(); got != 0 {
		t.Fatalf("inspector events after first collect = %d, want 0", got)
	}

	// Second collect: active window switches from 0 → 1.
	collectCount.Store(1)
	if err := svc.collect(context.Background()); err != nil {
		t.Fatalf("collect #2: %v", err)
	}
	if got := inspectorEvents.Load(); got != 1 {
		t.Fatalf("inspector events after second collect = %d, want 1", got)
	}
	if got := inspectorSession.Load(); got != "dev" {
		t.Fatalf("inspector event session = %v, want dev", got)
	}
	if got := inspectorAction.Load(); got != "active-window-changed" {
		t.Fatalf("inspector event action = %v, want active-window-changed", got)
	}

	// Third collect: no further active window change.
	if err := svc.collect(context.Background()); err != nil {
		t.Fatalf("collect #3: %v", err)
	}
	if got := inspectorEvents.Load(); got != 1 {
		t.Fatalf("inspector events after third collect = %d, want 1 (no new emission)", got)
	}
}

func TestCollectDoesNotPublishInspectorEventWhenActiveWindowUnchanged(t *testing.T) {
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
			return "output", nil
		},
	}

	var inspectorEvents atomic.Int32
	svc := New(st, fake, Options{
		Publish: func(eventType string, _ map[string]any) {
			if eventType == "tmux.inspector.updated" {
				inspectorEvents.Add(1)
			}
		},
	})

	for i := range 3 {
		if err := svc.collect(context.Background()); err != nil {
			t.Fatalf("collect #%d: %v", i+1, err)
		}
	}
	if got := inspectorEvents.Load(); got != 0 {
		t.Fatalf("inspector events = %d, want 0 (active window never changed)", got)
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
