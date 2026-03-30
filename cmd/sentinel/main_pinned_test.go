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
	listErr        error
	recordedDirs   []string
	icons          map[string]string
	markedLaunched []string
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

type fakePinnedTmux struct {
	errByName map[string]error
	calls     []struct {
		name string
		cwd  string
	}
}

func (f *fakePinnedTmux) CreateSession(_ context.Context, name, cwd string) error {
	f.calls = append(f.calls, struct {
		name string
		cwd  string
	}{name: name, cwd: cwd})
	return f.errByName[name]
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

		restored, err := restorePinnedSessions(context.Background(), repo, tm)
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

		restored, err := restorePinnedSessions(context.Background(), repo, tm)
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

	t.Run("returns list error", func(t *testing.T) {
		wantErr := errors.New("list failed")
		repo := &fakePinnedStore{listErr: wantErr}

		_, err := restorePinnedSessions(context.Background(), repo, &fakePinnedTmux{})
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
	})
}
