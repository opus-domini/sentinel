package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestSessionPresets(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	const (
		apiName = "api"
		webName = "web"
	)

	t.Run("create and list", func(t *testing.T) {
		t.Parallel()

		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		created, err := s.CreateSessionPreset(ctx, SessionPresetWrite{
			Name: apiName,
			Cwd:  "/srv/api",
			Icon: "server",
		})
		if err != nil {
			t.Fatalf("CreateSessionPreset() error = %v", err)
		}
		if created.Name != apiName {
			t.Fatalf("created.Name = %q, want %s", created.Name, apiName)
		}
		if created.Cwd != "/srv/api" {
			t.Fatalf("created.Cwd = %q, want /srv/api", created.Cwd)
		}
		if created.Icon != "server" {
			t.Fatalf("created.Icon = %q, want server", created.Icon)
		}

		presets, err := s.ListSessionPresets(ctx)
		if err != nil {
			t.Fatalf("ListSessionPresets() error = %v", err)
		}
		if len(presets) != 1 {
			t.Fatalf("got %d presets, want 1", len(presets))
		}
		if presets[0].Name != apiName {
			t.Fatalf("presets[0].Name = %q, want %s", presets[0].Name, apiName)
		}
	})

	t.Run("update can rename preset", func(t *testing.T) {
		t.Parallel()

		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		if _, err := s.CreateSessionPreset(ctx, SessionPresetWrite{
			Name: apiName,
			Cwd:  "/srv/api",
			Icon: "server",
		}); err != nil {
			t.Fatalf("CreateSessionPreset() error = %v", err)
		}

		updated, err := s.UpdateSessionPreset(ctx, apiName, SessionPresetWrite{
			Name: webName,
			Cwd:  "/srv/web",
			Icon: "globe",
		})
		if err != nil {
			t.Fatalf("UpdateSessionPreset() error = %v", err)
		}
		if updated.Name != webName {
			t.Fatalf("updated.Name = %q, want %s", updated.Name, webName)
		}
		if updated.Cwd != "/srv/web" {
			t.Fatalf("updated.Cwd = %q, want /srv/web", updated.Cwd)
		}
		if updated.Icon != "globe" {
			t.Fatalf("updated.Icon = %q, want globe", updated.Icon)
		}

		presets, err := s.ListSessionPresets(ctx)
		if err != nil {
			t.Fatalf("ListSessionPresets() error = %v", err)
		}
		if len(presets) != 1 {
			t.Fatalf("got %d presets, want 1", len(presets))
		}
		if presets[0].Name != webName {
			t.Fatalf("presets[0].Name = %q, want %s", presets[0].Name, webName)
		}
	})

	t.Run("delete removes preset", func(t *testing.T) {
		t.Parallel()

		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		if _, err := s.CreateSessionPreset(ctx, SessionPresetWrite{
			Name: "logs",
			Cwd:  "/var/log",
			Icon: "terminal",
		}); err != nil {
			t.Fatalf("CreateSessionPreset() error = %v", err)
		}

		if err := s.DeleteSessionPreset(ctx, "logs"); err != nil {
			t.Fatalf("DeleteSessionPreset() error = %v", err)
		}
		if err := s.DeleteSessionPreset(ctx, "logs"); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("DeleteSessionPreset() second error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("mark launched increments counters", func(t *testing.T) {
		t.Parallel()

		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		if _, err := s.CreateSessionPreset(ctx, SessionPresetWrite{
			Name: "bot",
			Cwd:  "/srv/bot",
			Icon: "bot",
		}); err != nil {
			t.Fatalf("CreateSessionPreset() error = %v", err)
		}

		if err := s.MarkSessionPresetLaunched(ctx, "bot"); err != nil {
			t.Fatalf("MarkSessionPresetLaunched() error = %v", err)
		}

		presets, err := s.ListSessionPresets(ctx)
		if err != nil {
			t.Fatalf("ListSessionPresets() error = %v", err)
		}
		if len(presets) != 1 {
			t.Fatalf("got %d presets, want 1", len(presets))
		}
		if presets[0].LaunchCount != 1 {
			t.Fatalf("presets[0].LaunchCount = %d, want 1", presets[0].LaunchCount)
		}
		if presets[0].LastLaunchedAt.IsZero() {
			t.Fatal("presets[0].LastLaunchedAt is zero, want non-zero")
		}
	})
}
