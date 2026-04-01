package store

import (
	"context"
	"testing"
)

func TestManagedTmuxWindows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("create list update runtime rename and delete", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)

		window, err := s.CreateManagedTmuxWindow(ctx, ManagedTmuxWindowWrite{
			SessionName:     "dev",
			LauncherID:      "launcher-claude",
			LauncherName:    "Claude Code",
			Icon:            "bot",
			Command:         "claude",
			CwdMode:         TmuxLauncherCwdModeActivePane,
			WindowName:      "claude",
			TmuxWindowID:    "@12",
			LastWindowIndex: 2,
		})
		if err != nil {
			t.Fatalf("CreateManagedTmuxWindow() error = %v", err)
		}

		windows, err := s.ListManagedTmuxWindowsBySession(ctx, "dev")
		if err != nil {
			t.Fatalf("ListManagedTmuxWindowsBySession() error = %v", err)
		}
		if len(windows) != 1 {
			t.Fatalf("got %d windows, want 1", len(windows))
		}
		if windows[0].TmuxWindowID != "@12" || windows[0].WindowName != "claude" {
			t.Fatalf("windows[0] = %+v", windows[0])
		}

		if err := s.UpdateManagedTmuxWindowRuntime(ctx, window.ID, "@19", 4); err != nil {
			t.Fatalf("UpdateManagedTmuxWindowRuntime() error = %v", err)
		}
		if err := s.UpdateManagedTmuxWindowName(ctx, window.ID, "claude-main"); err != nil {
			t.Fatalf("UpdateManagedTmuxWindowName() error = %v", err)
		}
		if err := s.UpdateManagedTmuxWindowSortOrder(ctx, window.ID, 3); err != nil {
			t.Fatalf("UpdateManagedTmuxWindowSortOrder() error = %v", err)
		}

		windows, err = s.ListManagedTmuxWindowsBySession(ctx, "dev")
		if err != nil {
			t.Fatalf("ListManagedTmuxWindowsBySession() error = %v", err)
		}
		if windows[0].TmuxWindowID != "@19" || windows[0].LastWindowIndex != 4 {
			t.Fatalf("updated runtime = %+v", windows[0])
		}
		if windows[0].WindowName != "claude-main" {
			t.Fatalf("window name = %q, want claude-main", windows[0].WindowName)
		}
		if windows[0].SortOrder != 3 {
			t.Fatalf("sort order = %d, want 3", windows[0].SortOrder)
		}

		if err := s.DeleteManagedTmuxWindow(ctx, window.ID); err != nil {
			t.Fatalf("DeleteManagedTmuxWindow() error = %v", err)
		}

		windows, err = s.ListManagedTmuxWindowsBySession(ctx, "dev")
		if err != nil {
			t.Fatalf("ListManagedTmuxWindowsBySession() after delete error = %v", err)
		}
		if len(windows) != 0 {
			t.Fatalf("got %d windows after delete, want 0", len(windows))
		}
	})

	t.Run("create allows blank command", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)

		window, err := s.CreateManagedTmuxWindow(ctx, ManagedTmuxWindowWrite{
			SessionName:     "dev",
			LauncherID:      "launcher-runner",
			LauncherName:    "Runner",
			Icon:            "terminal",
			Command:         "",
			CwdMode:         TmuxLauncherCwdModeSession,
			WindowName:      "runner",
			TmuxWindowID:    "@4",
			LastWindowIndex: 1,
		})
		if err != nil {
			t.Fatalf("CreateManagedTmuxWindow() error = %v", err)
		}
		if window.Command != "" {
			t.Fatalf("window command = %q, want empty", window.Command)
		}
	})

	t.Run("delete missing runtime removes stale bindings only", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)

		first, err := s.CreateManagedTmuxWindow(ctx, ManagedTmuxWindowWrite{
			SessionName:     "dev",
			LauncherID:      "launcher-a",
			LauncherName:    "A",
			Icon:            "terminal",
			Command:         "a",
			CwdMode:         TmuxLauncherCwdModeSession,
			WindowName:      "a",
			TmuxWindowID:    "@1",
			LastWindowIndex: 0,
		})
		if err != nil {
			t.Fatalf("CreateManagedTmuxWindow(first) error = %v", err)
		}
		_, err = s.CreateManagedTmuxWindow(ctx, ManagedTmuxWindowWrite{
			SessionName:     "dev",
			LauncherID:      "launcher-b",
			LauncherName:    "B",
			Icon:            "terminal",
			Command:         "b",
			CwdMode:         TmuxLauncherCwdModeSession,
			WindowName:      "b",
			TmuxWindowID:    "@2",
			LastWindowIndex: 1,
		})
		if err != nil {
			t.Fatalf("CreateManagedTmuxWindow(second) error = %v", err)
		}
		_, err = s.CreateManagedTmuxWindow(ctx, ManagedTmuxWindowWrite{
			SessionName:     "dev",
			LauncherID:      "launcher-c",
			LauncherName:    "C",
			Icon:            "terminal",
			Command:         "c",
			CwdMode:         TmuxLauncherCwdModeSession,
			WindowName:      "c",
			LastWindowIndex: -1,
		})
		if err != nil {
			t.Fatalf("CreateManagedTmuxWindow(third) error = %v", err)
		}

		if err := s.DeleteManagedTmuxWindowsMissingRuntime(ctx, "dev", []string{"@2"}); err != nil {
			t.Fatalf("DeleteManagedTmuxWindowsMissingRuntime() error = %v", err)
		}

		windows, err := s.ListManagedTmuxWindowsBySession(ctx, "dev")
		if err != nil {
			t.Fatalf("ListManagedTmuxWindowsBySession() error = %v", err)
		}
		if len(windows) != 2 {
			t.Fatalf("got %d windows, want 2", len(windows))
		}
		if windows[0].ID == first.ID {
			t.Fatalf("stale runtime binding was not removed: %+v", windows)
		}
		if windows[1].TmuxWindowID != "" {
			t.Fatalf("unbound window should remain unbound, got %+v", windows[1])
		}
	})
}
