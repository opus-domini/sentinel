package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestSessionLaunchers(t *testing.T) {
	t.Parallel()

	t.Run("create list update delete", func(t *testing.T) {
		t.Parallel()

		s := newTestStore(t)
		ctx := context.Background()

		created, err := s.CreateSessionLauncher(ctx, SessionLauncherWrite{
			Name: "api",
			Cwd:  "/srv/api",
			Icon: "server",
			User: "deploy",
		})
		if err != nil {
			t.Fatalf("CreateSessionLauncher() error = %v", err)
		}
		if created.ID == "" {
			t.Fatal("created launcher id is empty")
		}
		if created.Name != "api" || created.Cwd != "/srv/api" || created.User != "deploy" {
			t.Fatalf("created launcher = %#v", created)
		}

		updated, err := s.UpdateSessionLauncher(ctx, created.ID, SessionLauncherWrite{
			Name: "web",
			Cwd:  "/srv/web",
			Icon: "globe",
		})
		if err != nil {
			t.Fatalf("UpdateSessionLauncher() error = %v", err)
		}
		if updated.ID != created.ID || updated.Name != "web" || updated.User != "" {
			t.Fatalf("updated launcher = %#v", updated)
		}

		launchers, err := s.ListSessionLaunchers(ctx)
		if err != nil {
			t.Fatalf("ListSessionLaunchers() error = %v", err)
		}
		if len(launchers) != 1 || launchers[0].ID != created.ID {
			t.Fatalf("launchers = %#v, want one updated launcher", launchers)
		}

		if err := s.DeleteSessionLauncher(ctx, created.ID); err != nil {
			t.Fatalf("DeleteSessionLauncher() error = %v", err)
		}
		if _, err := s.GetSessionLauncher(ctx, created.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("GetSessionLauncher() error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("mark used", func(t *testing.T) {
		t.Parallel()

		s := newTestStore(t)
		ctx := context.Background()
		created, err := s.CreateSessionLauncher(ctx, SessionLauncherWrite{
			Name: "bot",
			Cwd:  "/srv/bot",
			Icon: "bot",
		})
		if err != nil {
			t.Fatalf("CreateSessionLauncher() error = %v", err)
		}

		if err := s.MarkSessionLauncherUsed(ctx, created.ID); err != nil {
			t.Fatalf("MarkSessionLauncherUsed() error = %v", err)
		}
		updated, err := s.GetSessionLauncher(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetSessionLauncher() error = %v", err)
		}
		if updated.UseCount != 1 || updated.LastUsedAt.IsZero() {
			t.Fatalf("used launcher = %#v, want use count and timestamp", updated)
		}
	})

	t.Run("reorder", func(t *testing.T) {
		t.Parallel()

		s := newTestStore(t)
		ctx := context.Background()
		first, err := s.CreateSessionLauncher(ctx, SessionLauncherWrite{
			Name: "api",
			Cwd:  "/srv/api",
			Icon: "server",
		})
		if err != nil {
			t.Fatalf("CreateSessionLauncher(first) error = %v", err)
		}
		second, err := s.CreateSessionLauncher(ctx, SessionLauncherWrite{
			Name: "web",
			Cwd:  "/srv/web",
			Icon: "globe",
		})
		if err != nil {
			t.Fatalf("CreateSessionLauncher(second) error = %v", err)
		}

		if err := s.ReorderSessionLaunchers(ctx, []string{second.ID, first.ID}); err != nil {
			t.Fatalf("ReorderSessionLaunchers() error = %v", err)
		}
		launchers, err := s.ListSessionLaunchers(ctx)
		if err != nil {
			t.Fatalf("ListSessionLaunchers() error = %v", err)
		}
		if launchers[0].ID != second.ID || launchers[1].ID != first.ID {
			t.Fatalf("launchers = %#v, want reordered list", launchers)
		}
	})
}
