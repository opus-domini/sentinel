package store

import (
	"context"
	"testing"
)

func TestSessionPresetUserField(t *testing.T) {
	t.Parallel()

	const targetUser = "postgres"
	st := newTestStore(t)
	ctx := context.Background()

	// Create preset with user field.
	preset, err := st.CreateSessionPreset(ctx, SessionPresetWrite{
		Name: "db-work",
		Cwd:  "/var/lib/postgres",
		Icon: "database",
		User: targetUser,
	})
	if err != nil {
		t.Fatalf("CreateSessionPreset: %v", err)
	}
	if preset.User != targetUser {
		t.Errorf("User = %q, want %s", preset.User, targetUser)
	}

	// List and verify.
	presets, err := st.ListSessionPresets(ctx)
	if err != nil {
		t.Fatalf("ListSessionPresets: %v", err)
	}
	if len(presets) != 1 {
		t.Fatalf("len(presets) = %d, want 1", len(presets))
	}
	if presets[0].User != targetUser {
		t.Errorf("listed preset User = %q, want %s", presets[0].User, targetUser)
	}

	// Update user.
	updated, err := st.UpdateSessionPreset(ctx, "db-work", SessionPresetWrite{
		Name: "db-work",
		Cwd:  "/var/lib/postgres",
		Icon: "database",
		User: "deploy",
	})
	if err != nil {
		t.Fatalf("UpdateSessionPreset: %v", err)
	}
	if updated.User != "deploy" {
		t.Errorf("updated User = %q, want deploy", updated.User)
	}
}

func TestSessionPresetEmptyUser(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	ctx := context.Background()

	preset, err := st.CreateSessionPreset(ctx, SessionPresetWrite{
		Name: "default",
		Cwd:  "/home/user",
		Icon: "terminal",
	})
	if err != nil {
		t.Fatalf("CreateSessionPreset: %v", err)
	}
	if preset.User != "" {
		t.Errorf("User = %q, want empty", preset.User)
	}
}

func TestTmuxLauncherUserModeFields(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	ctx := context.Background()

	// Create launcher with user mode.
	launcher, err := st.CreateTmuxLauncher(ctx, TmuxLauncherWrite{
		Name:      "pg-shell",
		Icon:      "database",
		Command:   "psql",
		CwdMode:   TmuxLauncherCwdModeSession,
		UserMode:  TmuxLauncherUserModeFixed,
		UserValue: "postgres",
	})
	if err != nil {
		t.Fatalf("CreateTmuxLauncher: %v", err)
	}
	if launcher.UserMode != TmuxLauncherUserModeFixed {
		t.Errorf("UserMode = %q, want %q", launcher.UserMode, TmuxLauncherUserModeFixed)
	}
	if launcher.UserValue != "postgres" {
		t.Errorf("UserValue = %q, want postgres", launcher.UserValue)
	}

	// List and verify.
	launchers, err := st.ListTmuxLaunchers(ctx)
	if err != nil {
		t.Fatalf("ListTmuxLaunchers: %v", err)
	}
	if len(launchers) != 1 {
		t.Fatalf("len(launchers) = %d, want 1", len(launchers))
	}
	if launchers[0].UserMode != TmuxLauncherUserModeFixed {
		t.Errorf("listed launcher UserMode = %q, want %q", launchers[0].UserMode, TmuxLauncherUserModeFixed)
	}
	if launchers[0].UserValue != "postgres" {
		t.Errorf("listed launcher UserValue = %q, want postgres", launchers[0].UserValue)
	}

	// Update to session mode.
	updated, err := st.UpdateTmuxLauncher(ctx, launcher.ID, TmuxLauncherWrite{
		Name:     "pg-shell",
		Icon:     "database",
		Command:  "psql",
		CwdMode:  TmuxLauncherCwdModeSession,
		UserMode: TmuxLauncherUserModeSession,
	})
	if err != nil {
		t.Fatalf("UpdateTmuxLauncher: %v", err)
	}
	if updated.UserMode != TmuxLauncherUserModeSession {
		t.Errorf("updated UserMode = %q, want %q", updated.UserMode, TmuxLauncherUserModeSession)
	}
	if updated.UserValue != "" {
		t.Errorf("updated UserValue = %q, want empty", updated.UserValue)
	}
}

func TestTmuxLauncherDefaultUserMode(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	ctx := context.Background()

	// Omitting user mode should default to "session".
	launcher, err := st.CreateTmuxLauncher(ctx, TmuxLauncherWrite{
		Name:    "shell",
		Icon:    "terminal",
		CwdMode: TmuxLauncherCwdModeSession,
	})
	if err != nil {
		t.Fatalf("CreateTmuxLauncher: %v", err)
	}
	if launcher.UserMode != TmuxLauncherUserModeSession {
		t.Errorf("UserMode = %q, want %q", launcher.UserMode, TmuxLauncherUserModeSession)
	}
}

func TestTmuxLauncherInvalidUserMode(t *testing.T) {
	t.Parallel()

	st := newTestStore(t)
	ctx := context.Background()

	_, err := st.CreateTmuxLauncher(ctx, TmuxLauncherWrite{
		Name:     "bad",
		Icon:     "terminal",
		CwdMode:  TmuxLauncherCwdModeSession,
		UserMode: "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid user mode")
	}
}
