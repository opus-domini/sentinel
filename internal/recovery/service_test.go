package recovery

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

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
				Name:       "dev",
				Attached:   1,
				CreatedAt:  time.Now().UTC(),
				ActivityAt: time.Now().UTC(),
			},
		},
		windows: map[string][]tmux.Window{
			"dev": {
				{Session: "dev", Index: 0, Name: "editor", Active: true, Panes: 1, Layout: "abcd,120x40,0,0,0"},
			},
		},
		panes: map[string][]tmux.Pane{
			"dev": {
				{
					Session:        "dev",
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
	svc.bootID = func(context.Context) string { return "boot-a" }

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
	if sessions[0].Name != "dev" {
		t.Fatalf("session name = %q, want dev", sessions[0].Name)
	}
	if sessions[0].LatestSnapshotID <= 0 {
		t.Fatalf("latest snapshot id = %d, want > 0", sessions[0].LatestSnapshotID)
	}

	snapshots, err := st.ListRecoverySnapshots(context.Background(), "dev", 10)
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

	bootID := "boot-a"
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
