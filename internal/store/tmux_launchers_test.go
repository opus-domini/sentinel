package store

import (
	"context"
	"database/sql"
	"testing"
)

func TestTmuxLaunchers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("create list update delete launcher", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)

		launcher, err := s.CreateTmuxLauncher(ctx, TmuxLauncherWrite{
			Name:       "Codex",
			Icon:       "code",
			Command:    "codex",
			CwdMode:    TmuxLauncherCwdModeActivePane,
			WindowName: "codex",
		})
		if err != nil {
			t.Fatalf("CreateTmuxLauncher() error = %v", err)
		}
		if launcher.ID == "" {
			t.Fatal("CreateTmuxLauncher() returned empty id")
		}

		launchers, err := s.ListTmuxLaunchers(ctx)
		if err != nil {
			t.Fatalf("ListTmuxLaunchers() error = %v", err)
		}
		if len(launchers) != 1 {
			t.Fatalf("got %d launchers, want 1", len(launchers))
		}
		if launchers[0].Name != "Codex" {
			t.Fatalf("launcher name = %q, want Codex", launchers[0].Name)
		}

		updated, err := s.UpdateTmuxLauncher(ctx, launcher.ID, TmuxLauncherWrite{
			Name:       "Claude Code",
			Icon:       "bot",
			Command:    "claude",
			CwdMode:    TmuxLauncherCwdModeFixed,
			CwdValue:   "/srv/app",
			WindowName: "claude",
		})
		if err != nil {
			t.Fatalf("UpdateTmuxLauncher() error = %v", err)
		}
		if updated.Name != "Claude Code" {
			t.Fatalf("updated name = %q, want Claude Code", updated.Name)
		}
		if updated.CwdValue != "/srv/app" {
			t.Fatalf("updated cwdValue = %q, want /srv/app", updated.CwdValue)
		}

		if err := s.DeleteTmuxLauncher(ctx, launcher.ID); err != nil {
			t.Fatalf("DeleteTmuxLauncher() error = %v", err)
		}

		launchers, err = s.ListTmuxLaunchers(ctx)
		if err != nil {
			t.Fatalf("ListTmuxLaunchers() after delete error = %v", err)
		}
		if len(launchers) != 0 {
			t.Fatalf("got %d launchers after delete, want 0", len(launchers))
		}
	})

	t.Run("create validates fixed cwd", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)

		_, err := s.CreateTmuxLauncher(ctx, TmuxLauncherWrite{
			Name:    "Broken",
			Icon:    "terminal",
			Command: "echo ok",
			CwdMode: TmuxLauncherCwdModeFixed,
		})
		if err == nil {
			t.Fatal("CreateTmuxLauncher() error = nil, want error")
		}
	})

	t.Run("create allows blank command", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)

		launcher, err := s.CreateTmuxLauncher(ctx, TmuxLauncherWrite{
			Name:       "Runner",
			Icon:       "terminal",
			Command:    "",
			CwdMode:    TmuxLauncherCwdModeSession,
			WindowName: "runner",
		})
		if err != nil {
			t.Fatalf("CreateTmuxLauncher() error = %v", err)
		}
		if launcher.Command != "" {
			t.Fatalf("launcher command = %q, want empty", launcher.Command)
		}
	})

	t.Run("get launcher not found", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)

		_, err := s.GetTmuxLauncher(ctx, "missing")
		if err == nil {
			t.Fatal("GetTmuxLauncher() error = nil, want error")
		}
		if err != sql.ErrNoRows {
			t.Fatalf("GetTmuxLauncher() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("mark used updates last used timestamp", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)

		launcher, err := s.CreateTmuxLauncher(ctx, TmuxLauncherWrite{
			Name:       "Recent",
			Icon:       "terminal",
			Command:    "echo ok",
			CwdMode:    TmuxLauncherCwdModeSession,
			WindowName: "recent",
		})
		if err != nil {
			t.Fatalf("CreateTmuxLauncher() error = %v", err)
		}

		if err := s.MarkTmuxLauncherUsed(ctx, launcher.ID); err != nil {
			t.Fatalf("MarkTmuxLauncherUsed() error = %v", err)
		}

		stored, err := s.GetTmuxLauncher(ctx, launcher.ID)
		if err != nil {
			t.Fatalf("GetTmuxLauncher() error = %v", err)
		}
		if stored.LastUsedAt.IsZero() {
			t.Fatal("LastUsedAt is zero, want non-zero")
		}
	})

	t.Run("reorder updates sort order", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)

		first, err := s.CreateTmuxLauncher(ctx, TmuxLauncherWrite{
			Name:       "One",
			Icon:       "terminal",
			Command:    "one",
			CwdMode:    TmuxLauncherCwdModeSession,
			WindowName: "one",
		})
		if err != nil {
			t.Fatalf("CreateTmuxLauncher(first) error = %v", err)
		}
		second, err := s.CreateTmuxLauncher(ctx, TmuxLauncherWrite{
			Name:       "Two",
			Icon:       "terminal",
			Command:    "two",
			CwdMode:    TmuxLauncherCwdModeSession,
			WindowName: "two",
		})
		if err != nil {
			t.Fatalf("CreateTmuxLauncher(second) error = %v", err)
		}

		if err := s.ReorderTmuxLaunchers(ctx, []string{second.ID, first.ID}); err != nil {
			t.Fatalf("ReorderTmuxLaunchers() error = %v", err)
		}

		launchers, err := s.ListTmuxLaunchers(ctx)
		if err != nil {
			t.Fatalf("ListTmuxLaunchers() error = %v", err)
		}
		if len(launchers) != 2 {
			t.Fatalf("got %d launchers, want 2", len(launchers))
		}
		if launchers[0].ID != second.ID || launchers[1].ID != first.ID {
			t.Fatalf("unexpected launcher order: %#v", launchers)
		}
	})
}
