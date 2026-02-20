package recovery

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

const testBootID = "boot-a"

type fakeTmux struct {
	sessions []tmux.Session
	windows  map[string][]tmux.Window
	panes    map[string][]tmux.Pane
}

func (f *fakeTmux) ListSessions(_ context.Context) ([]tmux.Session, error) {
	return append([]tmux.Session{}, f.sessions...), nil
}

func (f *fakeTmux) ListWindows(_ context.Context, session string) ([]tmux.Window, error) {
	return append([]tmux.Window{}, f.windows[session]...), nil
}

func (f *fakeTmux) ListPanes(_ context.Context, session string) ([]tmux.Pane, error) {
	return append([]tmux.Pane{}, f.panes[session]...), nil
}

func (f *fakeTmux) CapturePaneLines(_ context.Context, _ string, _ int) (string, error) {
	return "echo ready\n", nil
}

func (f *fakeTmux) SessionExists(_ context.Context, session string) (bool, error) {
	for _, item := range f.sessions {
		if item.Name == session {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeTmux) CreateSession(_ context.Context, name, _ string) error {
	f.sessions = append(f.sessions, tmux.Session{Name: name, Attached: 1, CreatedAt: time.Now().UTC(), ActivityAt: time.Now().UTC()})
	if f.windows == nil {
		f.windows = make(map[string][]tmux.Window)
	}
	if f.panes == nil {
		f.panes = make(map[string][]tmux.Pane)
	}
	f.windows[name] = []tmux.Window{{Session: name, Index: 0, Name: "main", Active: true, Panes: 1}}
	f.panes[name] = []tmux.Pane{{Session: name, WindowIndex: 0, PaneIndex: 0, PaneID: "%1", Active: true}}
	return nil
}

func (f *fakeTmux) RenameWindow(_ context.Context, _ string, _ int, _ string) error { return nil }
func (f *fakeTmux) NewWindowAt(_ context.Context, _ string, _ int, _ string, _ string) error {
	return nil
}
func (f *fakeTmux) SplitPaneIn(_ context.Context, _ string, _ string, _ string) error { return nil }
func (f *fakeTmux) SelectLayout(_ context.Context, _ string, _ int, _ string) error   { return nil }
func (f *fakeTmux) SelectWindow(_ context.Context, _ string, _ int) error             { return nil }
func (f *fakeTmux) SelectPane(_ context.Context, _ string) error                      { return nil }
func (f *fakeTmux) RenamePane(_ context.Context, _ string, _ string) error            { return nil }
func (f *fakeTmux) SendKeys(_ context.Context, _ string, _ string, _ bool) error      { return nil }
func (f *fakeTmux) KillSession(_ context.Context, session string) error {
	out := make([]tmux.Session, 0, len(f.sessions))
	for _, item := range f.sessions {
		if item.Name != session {
			out = append(out, item)
		}
	}
	f.sessions = out
	delete(f.windows, session)
	delete(f.panes, session)
	return nil
}

func newRecoveryStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "sentinel.db"))
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestCollectPersistsRecoverySnapshot(t *testing.T) {
	t.Parallel()

	st := newRecoveryStore(t)
	fake := &fakeTmux{
		sessions: []tmux.Session{
			{
				Name:       testSessionDev,
				Attached:   1,
				CreatedAt:  time.Now().UTC(),
				ActivityAt: time.Now().UTC(),
			},
		},
		windows: map[string][]tmux.Window{
			testSessionDev: {
				{Session: testSessionDev, Index: 0, Name: "editor", Active: true, Panes: 1, Layout: "abcd,120x40,0,0,0"},
			},
		},
		panes: map[string][]tmux.Pane{
			testSessionDev: {
				{
					Session:        testSessionDev,
					WindowIndex:    0,
					PaneIndex:      0,
					PaneID:         "%1",
					Title:          "editor",
					Active:         true,
					CurrentPath:    "/tmp/dev",
					StartCommand:   "nvim",
					CurrentCommand: "nvim",
				},
			},
		},
	}

	svc := New(st, fake, Options{})
	svc.bootID = func(context.Context) string { return testBootID }

	if err := svc.Collect(context.Background()); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	sessions, err := st.ListRecoverySessions(context.Background(), []store.RecoverySessionState{store.RecoveryStateRunning})
	if err != nil {
		t.Fatalf("ListRecoverySessions() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("running sessions = %d, want 1", len(sessions))
	}
	if sessions[0].Name != testSessionDev {
		t.Fatalf("session name = %q, want dev", sessions[0].Name)
	}
	if sessions[0].LatestSnapshotID <= 0 {
		t.Fatalf("latest snapshot id = %d, want > 0", sessions[0].LatestSnapshotID)
	}

	snapshots, err := st.ListRecoverySnapshots(context.Background(), testSessionDev, 10)
	if err != nil {
		t.Fatalf("ListRecoverySnapshots() error = %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snapshots))
	}
}

func TestCollectMarksKilledSessionsAfterBootChange(t *testing.T) {
	t.Parallel()

	st := newRecoveryStore(t)
	fake := &fakeTmux{
		sessions: []tmux.Session{
			{
				Name:       "work",
				Attached:   1,
				CreatedAt:  time.Now().UTC(),
				ActivityAt: time.Now().UTC(),
			},
		},
		windows: map[string][]tmux.Window{
			"work": {
				{Session: "work", Index: 0, Name: "main", Active: true, Panes: 1, Layout: "abcd,120x40,0,0,0"},
			},
		},
		panes: map[string][]tmux.Pane{
			"work": {
				{
					Session:     "work",
					WindowIndex: 0,
					PaneIndex:   0,
					PaneID:      "%2",
					Active:      true,
					CurrentPath: "/tmp/work",
				},
			},
		},
	}

	bootID := testBootID
	svc := New(st, fake, Options{})
	svc.bootID = func(context.Context) string { return bootID }

	if err := svc.Collect(context.Background()); err != nil {
		t.Fatalf("first Collect() error = %v", err)
	}

	// Simulate reboot + tmux server reset: no active session from previous boot.
	bootID = "boot-b"
	fake.sessions = nil
	fake.windows = map[string][]tmux.Window{}
	fake.panes = map[string][]tmux.Pane{}

	if err := svc.Collect(context.Background()); err != nil {
		t.Fatalf("second Collect() error = %v", err)
	}

	killed, err := st.ListRecoverySessions(context.Background(), []store.RecoverySessionState{store.RecoveryStateKilled})
	if err != nil {
		t.Fatalf("ListRecoverySessions(killed) error = %v", err)
	}
	if len(killed) != 1 {
		t.Fatalf("killed sessions = %d, want 1", len(killed))
	}
	if killed[0].Name != "work" {
		t.Fatalf("killed session name = %q, want work", killed[0].Name)
	}
}

func TestCollectBuildsSnapshotFromWatchtowerProjection(t *testing.T) {
	t.Parallel()

	st := newRecoveryStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	ctx := context.Background()
	seedProjectionSnapshotState(t, st, ctx, now)

	fake := &fakeTmux{
		sessions: []tmux.Session{
			{
				Name:       testSessionDev,
				Attached:   2,
				CreatedAt:  now,
				ActivityAt: now,
			},
		},
		windows: map[string][]tmux.Window{
			testSessionDev: {},
		},
		panes: map[string][]tmux.Pane{
			testSessionDev: {},
		},
	}

	svc := New(st, fake, Options{})
	svc.bootID = func(context.Context) string { return "boot-proj" }

	if err := svc.Collect(ctx); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	snapshots, err := st.ListRecoverySnapshots(ctx, testSessionDev, 10)
	if err != nil {
		t.Fatalf("ListRecoverySnapshots() error = %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("snapshots len = %d, want 1", len(snapshots))
	}

	view, err := svc.GetSnapshot(ctx, snapshots[0].ID)
	if err != nil {
		t.Fatalf("GetSnapshot() error = %v", err)
	}
	assertProjectionSnapshotView(t, view)
}

func seedProjectionSnapshotState(t *testing.T, st *store.Store, ctx context.Context, now time.Time) {
	t.Helper()
	if err := st.UpsertWatchtowerSession(ctx, store.WatchtowerSessionWrite{
		SessionName:       testSessionDev,
		Attached:          1,
		Windows:           2,
		Panes:             2,
		ActivityAt:        now,
		LastPreview:       "projection preview",
		LastPreviewAt:     now,
		LastPreviewPaneID: "%1",
		Rev:               4,
		UpdatedAt:         now,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerSession: %v", err)
	}
	for _, win := range []store.WatchtowerWindowWrite{
		{SessionName: testSessionDev, WindowIndex: 0, Name: "editor", Active: true, Layout: "layout-a", WindowActivityAt: now, Rev: 2, UpdatedAt: now},
		{SessionName: testSessionDev, WindowIndex: 1, Name: "logs", Active: false, Layout: "layout-b", WindowActivityAt: now, Rev: 2, UpdatedAt: now},
	} {
		if err := st.UpsertWatchtowerWindow(ctx, win); err != nil {
			t.Fatalf("UpsertWatchtowerWindow(%d): %v", win.WindowIndex, err)
		}
	}
	for _, pane := range []store.WatchtowerPaneWrite{
		{
			PaneID:         "%1",
			SessionName:    testSessionDev,
			WindowIndex:    0,
			PaneIndex:      0,
			Title:          "editor-pane",
			Active:         true,
			CurrentPath:    "/tmp/dev",
			StartCommand:   "nvim",
			CurrentCommand: "nvim",
			TailPreview:    "line from projection 1",
			TailCapturedAt: now,
			Revision:       2,
			SeenRevision:   1,
			ChangedAt:      now,
			UpdatedAt:      now,
		},
		{
			PaneID:         "%2",
			SessionName:    testSessionDev,
			WindowIndex:    1,
			PaneIndex:      0,
			Title:          "logs-pane",
			Active:         false,
			CurrentPath:    "/var/log",
			StartCommand:   "tail",
			CurrentCommand: "tail -f app.log",
			TailPreview:    "line from projection 2",
			TailCapturedAt: now,
			Revision:       3,
			SeenRevision:   1,
			ChangedAt:      now,
			UpdatedAt:      now,
		},
	} {
		if err := st.UpsertWatchtowerPane(ctx, pane); err != nil {
			t.Fatalf("UpsertWatchtowerPane(%s): %v", pane.PaneID, err)
		}
	}
}

func TestRestoreSafeModeSkipsCd(t *testing.T) {
	t.Parallel()

	st := newRecoveryStore(t)
	ctx := context.Background()

	var sentKeys []string
	fake := &fakeTmux{
		sessions: []tmux.Session{
			{Name: testSessionDev, Attached: 1, CreatedAt: time.Now().UTC(), ActivityAt: time.Now().UTC()},
		},
		windows: map[string][]tmux.Window{
			testSessionDev: {{Session: testSessionDev, Index: 0, Name: "editor", Active: true, Panes: 1, Layout: "abcd,120x40,0,0,0"}},
		},
		panes: map[string][]tmux.Pane{
			testSessionDev: {{Session: testSessionDev, WindowIndex: 0, PaneIndex: 0, PaneID: "%1", Active: true, CurrentPath: "/tmp/dev", StartCommand: "nvim", CurrentCommand: "nvim"}},
		},
	}

	svc := New(st, fake, Options{})
	svc.bootID = func(context.Context) string { return testBootID }

	if err := svc.Collect(ctx); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	snapshots, err := st.ListRecoverySnapshots(ctx, testSessionDev, 1)
	if err != nil || len(snapshots) == 0 {
		t.Fatalf("ListRecoverySnapshots() = %v, err = %v", snapshots, err)
	}

	// Replace the fake with one that tracks SendKeys calls.
	trackFake := &sendKeysTracker{fakeTmux: *fake}
	svc.tmux = trackFake

	// Restore with safe mode.
	job, err := svc.RestoreSnapshotAsync(ctx, snapshots[0].ID, RestoreOptions{
		Mode:           ReplayModeSafe,
		ConflictPolicy: ConflictRename,
	})
	if err != nil {
		t.Fatalf("RestoreSnapshotAsync() error = %v", err)
	}

	// Wait for the job to finish.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		j, err := st.GetRecoveryJob(ctx, job.ID)
		if err != nil {
			t.Fatalf("GetRecoveryJob() error = %v", err)
		}
		if j.Status == store.RecoveryJobSucceeded || j.Status == store.RecoveryJobFailed {
			if j.Status == store.RecoveryJobFailed {
				t.Fatalf("restore job failed: %s", j.Error)
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	sentKeys = trackFake.keys()
	for _, key := range sentKeys {
		if key == "cd " || len(key) > 3 && key[:3] == "cd " {
			t.Fatalf("safe mode sent cd command: %q", key)
		}
		if key == "nvim" {
			t.Fatalf("safe mode sent start command: %q", key)
		}
	}
}

type sendKeysTracker struct {
	fakeTmux
	mu       sync.Mutex
	sentKeys []string
}

func (s *sendKeysTracker) SendKeys(_ context.Context, _ string, keys string, _ bool) error {
	s.mu.Lock()
	s.sentKeys = append(s.sentKeys, keys)
	s.mu.Unlock()
	return nil
}

func (s *sendKeysTracker) keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.sentKeys...)
}

func assertProjectionSnapshotView(t *testing.T, view SnapshotView) {
	t.Helper()
	if view.Payload.Attached != 2 {
		t.Fatalf("payload.Attached = %d, want 2", view.Payload.Attached)
	}
	if len(view.Payload.Windows) != 2 || len(view.Payload.Panes) != 2 {
		t.Fatalf("payload sizes = windows:%d panes:%d, want windows:2 panes:2", len(view.Payload.Windows), len(view.Payload.Panes))
	}
	if view.Payload.ActiveWindow != 0 || view.Payload.ActivePaneID != "%1" {
		t.Fatalf("active selection = (%d,%s), want (0,%%1)", view.Payload.ActiveWindow, view.Payload.ActivePaneID)
	}
	if view.Payload.Panes[0].LastContent != "line from projection 1" {
		t.Fatalf("pane[0].LastContent = %q, want line from projection 1", view.Payload.Panes[0].LastContent)
	}
	if view.Payload.Panes[1].LastContent != "line from projection 2" {
		t.Fatalf("pane[1].LastContent = %q, want line from projection 2", view.Payload.Panes[1].LastContent)
	}
}
