package main

import (
	"context"
	"errors"
	"testing"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

type fakePinnedStore struct {
	presets        []store.SessionPreset
	managed        map[string][]store.ManagedTmuxWindow
	listErr        error
	recordedDirs   []string
	icons          map[string]string
	markedLaunched []string
	runtimeUpdates []struct {
		id           string
		tmuxWindowID string
		lastIndex    int
	}
}

func (f *fakePinnedStore) ListSessionPresets(context.Context) ([]store.SessionPreset, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.presets, nil
}

func (f *fakePinnedStore) RecordSessionDirectory(_ context.Context, path string) error {
	f.recordedDirs = append(f.recordedDirs, path)
	return nil
}

func (f *fakePinnedStore) SetIcon(_ context.Context, name, icon string) error {
	if f.icons == nil {
		f.icons = make(map[string]string)
	}
	f.icons[name] = icon
	return nil
}

func (f *fakePinnedStore) MarkSessionPresetLaunched(_ context.Context, name string) error {
	f.markedLaunched = append(f.markedLaunched, name)
	return nil
}

func (f *fakePinnedStore) ListManagedTmuxWindowsBySession(_ context.Context, sessionName string) ([]store.ManagedTmuxWindow, error) {
	return append([]store.ManagedTmuxWindow(nil), f.managed[sessionName]...), nil
}

func (f *fakePinnedStore) UpdateManagedTmuxWindowRuntime(_ context.Context, id, tmuxWindowID string, lastWindowIndex int) error {
	f.runtimeUpdates = append(f.runtimeUpdates, struct {
		id           string
		tmuxWindowID string
		lastIndex    int
	}{id: id, tmuxWindowID: tmuxWindowID, lastIndex: lastWindowIndex})
	return nil
}

type fakePinnedTmux struct {
	user      string
	errByName map[string]error
	calls     []struct {
		name string
		cwd  string
	}
	windowsBySession map[string][]tmux.Window
	panesBySession   map[string][]tmux.Pane
	renamedWindows   []string
	sentKeys         []struct {
		paneID string
		keys   string
		enter  bool
	}
	newWindows []struct {
		session string
		name    string
		cwd     string
	}
}

type fakePinnedTmuxFactory struct {
	byUser map[string]*fakePinnedTmux
}

func (f *fakePinnedTmuxFactory) starter(user string) pinnedSessionStarter {
	if f.byUser == nil {
		f.byUser = make(map[string]*fakePinnedTmux)
	}
	tm, ok := f.byUser[user]
	if !ok {
		tm = &fakePinnedTmux{user: user}
		f.byUser[user] = tm
	}
	return tm
}

func (f *fakePinnedTmux) CreateSession(_ context.Context, name, cwd string) error {
	f.calls = append(f.calls, struct {
		name string
		cwd  string
	}{name: name, cwd: cwd})
	return f.errByName[name]
}

func (f *fakePinnedTmux) ListWindows(_ context.Context, session string) ([]tmux.Window, error) {
	return append([]tmux.Window(nil), f.windowsBySession[session]...), nil
}

func (f *fakePinnedTmux) ListPanes(_ context.Context, session string) ([]tmux.Pane, error) {
	return append([]tmux.Pane(nil), f.panesBySession[session]...), nil
}

func (f *fakePinnedTmux) RenameWindow(_ context.Context, session string, index int, name string) error {
	f.renamedWindows = append(f.renamedWindows, name)
	return nil
}

func (f *fakePinnedTmux) NewWindowWithOptions(_ context.Context, session, name, cwd string) (tmux.NewWindowResult, error) {
	f.newWindows = append(f.newWindows, struct {
		session string
		name    string
		cwd     string
	}{session: session, name: name, cwd: cwd})
	return tmux.NewWindowResult{ID: "@restored", Index: 1, PaneID: "%restored"}, nil
}

func (f *fakePinnedTmux) SendKeys(_ context.Context, paneID, keys string, enter bool) error {
	f.sentKeys = append(f.sentKeys, struct {
		paneID string
		keys   string
		enter  bool
	}{paneID: paneID, keys: keys, enter: enter})
	return nil
}

func TestRestorePinnedSessions(t *testing.T) {
	t.Run("restores and marks pinned sessions", func(t *testing.T) {
		repo := &fakePinnedStore{
			presets: []store.SessionPreset{
				{Name: "api", Cwd: "/srv/api", Icon: "server"},
				{Name: "web", Cwd: "/srv/web", Icon: "globe"},
			},
		}
		tm := &fakePinnedTmux{
			errByName: map[string]error{
				"web": &tmux.Error{Kind: tmux.ErrKindSessionExists},
			},
		}

		restored, err := restorePinnedSessions(context.Background(), repo, func(string) pinnedSessionStarter { return tm })
		if err != nil {
			t.Fatalf("restorePinnedSessions() error = %v", err)
		}
		if restored != 2 {
			t.Fatalf("restored = %d, want 2", restored)
		}
		if len(tm.calls) != 2 {
			t.Fatalf("create calls = %d, want 2", len(tm.calls))
		}
		if got := repo.icons["api"]; got != "server" {
			t.Fatalf("api icon = %q, want server", got)
		}
		if got := repo.icons["web"]; got != "globe" {
			t.Fatalf("web icon = %q, want globe", got)
		}
		if len(repo.recordedDirs) != 2 {
			t.Fatalf("recorded dirs = %d, want 2", len(repo.recordedDirs))
		}
		if len(repo.markedLaunched) != 2 {
			t.Fatalf("marked launched = %d, want 2", len(repo.markedLaunched))
		}
	})

	t.Run("continues after individual restore failure", func(t *testing.T) {
		repo := &fakePinnedStore{
			presets: []store.SessionPreset{
				{Name: "broken", Cwd: "/srv/broken", Icon: "server"},
				{Name: "api", Cwd: "/srv/api", Icon: "server"},
			},
		}
		tm := &fakePinnedTmux{
			errByName: map[string]error{
				"broken": errors.New("tmux failed"),
			},
		}

		restored, err := restorePinnedSessions(context.Background(), repo, func(string) pinnedSessionStarter { return tm })
		if err != nil {
			t.Fatalf("restorePinnedSessions() error = %v", err)
		}
		if restored != 1 {
			t.Fatalf("restored = %d, want 1", restored)
		}
		if len(repo.markedLaunched) != 1 || repo.markedLaunched[0] != "api" {
			t.Fatalf("marked launched = %v, want [api]", repo.markedLaunched)
		}
	})

	t.Run("restores managed tmux windows for newly created pinned session", func(t *testing.T) {
		repo := &fakePinnedStore{
			presets: []store.SessionPreset{
				{Name: "api", Cwd: "/srv/api", Icon: "server"},
			},
			managed: map[string][]store.ManagedTmuxWindow{
				"api": {
					{
						ID:              "mw-1",
						SessionName:     "api",
						WindowName:      "claude",
						Command:         "claude",
						ResolvedCwd:     "/srv/api",
						LastWindowIndex: 0,
					},
					{
						ID:              "mw-2",
						SessionName:     "api",
						WindowName:      "codex",
						Command:         "codex",
						ResolvedCwd:     "/srv/api",
						LastWindowIndex: 1,
					},
				},
			},
		}
		tm := &fakePinnedTmux{
			windowsBySession: map[string][]tmux.Window{
				"api": {{Session: "api", ID: "@0", Index: 0, Name: "0", Active: true, Panes: 1}},
			},
			panesBySession: map[string][]tmux.Pane{
				"api": {{Session: "api", WindowIndex: 0, PaneIndex: 0, PaneID: "%0", Active: true}},
			},
		}

		restored, err := restorePinnedSessions(context.Background(), repo, func(string) pinnedSessionStarter { return tm })
		if err != nil {
			t.Fatalf("restorePinnedSessions() error = %v", err)
		}
		if restored != 1 {
			t.Fatalf("restored = %d, want 1", restored)
		}
		if len(tm.renamedWindows) != 1 || tm.renamedWindows[0] != "claude" {
			t.Fatalf("renamed windows = %v, want [claude]", tm.renamedWindows)
		}
		if len(tm.newWindows) != 1 || tm.newWindows[0].name != "codex" {
			t.Fatalf("new windows = %+v, want codex restore", tm.newWindows)
		}
		if len(tm.sentKeys) != 2 {
			t.Fatalf("sent keys = %d, want 2", len(tm.sentKeys))
		}
		if len(repo.runtimeUpdates) != 2 {
			t.Fatalf("runtime updates = %d, want 2", len(repo.runtimeUpdates))
		}
	})

	t.Run("restores pinned sessions with their configured user", func(t *testing.T) {
		repo := &fakePinnedStore{
			presets: []store.SessionPreset{
				{Name: "api", Cwd: "/srv/api", Icon: "server", User: "postgres"},
				{Name: "web", Cwd: "/srv/web", Icon: "globe"},
			},
		}
		factory := &fakePinnedTmuxFactory{}

		restored, err := restorePinnedSessions(context.Background(), repo, factory.starter)
		if err != nil {
			t.Fatalf("restorePinnedSessions() error = %v", err)
		}
		if restored != 2 {
			t.Fatalf("restored = %d, want 2", restored)
		}

		postgresTm := factory.byUser["postgres"]
		if postgresTm == nil || len(postgresTm.calls) != 1 || postgresTm.calls[0].name != "api" {
			t.Fatalf("postgres restore calls = %+v, want api", postgresTm)
		}

		defaultTm := factory.byUser[""]
		if defaultTm == nil || len(defaultTm.calls) != 1 || defaultTm.calls[0].name != "web" {
			t.Fatalf("default restore calls = %+v, want web", defaultTm)
		}
	})

	t.Run("returns list error", func(t *testing.T) {
		wantErr := errors.New("list failed")
		repo := &fakePinnedStore{listErr: wantErr}

		_, err := restorePinnedSessions(context.Background(), repo, func(string) pinnedSessionStarter { return &fakePinnedTmux{} })
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
	})
}
