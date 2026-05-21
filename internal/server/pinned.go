package server

import (
	"context"
	"log/slog"
	"strings"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

type pinnedSessionStore interface {
	ListSessionPresets(ctx context.Context) ([]store.SessionPreset, error)
	RecordSessionDirectory(ctx context.Context, path string) error
	SetIcon(ctx context.Context, name, icon string) error
	MarkSessionPresetLaunched(ctx context.Context, name string) error
	ListManagedTmuxWindowsBySession(ctx context.Context, sessionName string) ([]store.ManagedTmuxWindow, error)
	UpdateManagedTmuxWindowRuntime(ctx context.Context, id, tmuxWindowID string, lastWindowIndex int) error
}

type pinnedSessionStarter interface {
	CreateSession(ctx context.Context, name, cwd string) error
	ListWindows(ctx context.Context, session string) ([]tmux.Window, error)
	ListPanes(ctx context.Context, session string) ([]tmux.Pane, error)
	RenameWindow(ctx context.Context, session string, index int, name string) error
	NewWindowWithOptions(ctx context.Context, session, name, cwd string) (tmux.NewWindowResult, error)
	SendKeys(ctx context.Context, paneID, keys string, enter bool) error
}

type pinnedSessionStarterFactory func(user string) pinnedSessionStarter

func restorePinnedSessions(ctx context.Context, repo pinnedSessionStore, starterForUser pinnedSessionStarterFactory) (int, error) {
	presets, err := repo.ListSessionPresets(ctx)
	if err != nil {
		return 0, err
	}

	restored := 0
	for _, preset := range presets {
		tm := starterForUser(strings.TrimSpace(preset.User))
		created := true
		err := tm.CreateSession(ctx, preset.Name, preset.Cwd)
		if err != nil && !tmux.IsKind(err, tmux.ErrKindSessionExists) {
			slog.Warn("failed to restore pinned session", "session", preset.Name, "cwd", preset.Cwd, "err", err)
			continue
		}
		if tmux.IsKind(err, tmux.ErrKindSessionExists) {
			created = false
		}

		restored++
		if err := repo.RecordSessionDirectory(ctx, preset.Cwd); err != nil {
			slog.Warn("failed to record pinned session directory", "session", preset.Name, "cwd", preset.Cwd, "err", err)
		}
		if err := repo.SetIcon(ctx, preset.Name, preset.Icon); err != nil {
			slog.Warn("failed to restore pinned session icon", "session", preset.Name, "icon", preset.Icon, "err", err)
		}
		if err := repo.MarkSessionPresetLaunched(ctx, preset.Name); err != nil {
			slog.Warn("failed to mark pinned session launched", "session", preset.Name, "err", err)
		}
		if created {
			if err := restoreManagedTmuxWindowsForSession(ctx, repo, tm, preset); err != nil {
				slog.Warn("failed to restore managed tmux windows", "session", preset.Name, "err", err)
			}
		}
	}

	return restored, nil
}

func restoreManagedTmuxWindowsForSession(ctx context.Context, repo pinnedSessionStore, tm pinnedSessionStarter, preset store.SessionPreset) error {
	managedWindows, err := repo.ListManagedTmuxWindowsBySession(ctx, preset.Name)
	if err != nil || len(managedWindows) == 0 {
		return err
	}

	liveWindows, err := tm.ListWindows(ctx, preset.Name)
	if err != nil {
		return err
	}
	livePanes, err := tm.ListPanes(ctx, preset.Name)
	if err != nil {
		return err
	}
	if len(liveWindows) == 0 {
		return nil
	}

	firstWindow := liveWindows[0]
	for _, window := range liveWindows[1:] {
		if window.Index < firstWindow.Index {
			firstWindow = window
		}
	}
	firstPane, ok := firstPaneForWindow(livePanes, firstWindow.Index)
	if !ok {
		return nil
	}

	var firstErr error
	if err := restoreManagedTmuxWindowInExistingSlot(ctx, repo, tm, preset, managedWindows[0], firstWindow, firstPane); err != nil && firstErr == nil {
		firstErr = err
	}
	for _, managedWindow := range managedWindows[1:] {
		if err := restoreManagedTmuxWindow(ctx, repo, tm, preset, managedWindow); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func restoreManagedTmuxWindowInExistingSlot(ctx context.Context, repo pinnedSessionStore, tm pinnedSessionStarter, preset store.SessionPreset, managedWindow store.ManagedTmuxWindow, liveWindow tmux.Window, livePane tmux.Pane) error {
	if err := tm.RenameWindow(ctx, preset.Name, liveWindow.Index, managedWindow.WindowName); err != nil {
		return err
	}
	if err := repo.UpdateManagedTmuxWindowRuntime(ctx, managedWindow.ID, liveWindow.ID, liveWindow.Index); err != nil {
		return err
	}
	if strings.TrimSpace(managedWindow.Command) == "" {
		return nil
	}
	return tm.SendKeys(ctx, livePane.PaneID, managedWindow.Command, true)
}

func restoreManagedTmuxWindow(ctx context.Context, repo pinnedSessionStore, tm pinnedSessionStarter, preset store.SessionPreset, managedWindow store.ManagedTmuxWindow) error {
	createdWindow, err := tm.NewWindowWithOptions(
		ctx,
		preset.Name,
		managedWindow.WindowName,
		resolveManagedTmuxWindowCwd(managedWindow, preset.Cwd),
	)
	if err != nil {
		return err
	}
	if err := repo.UpdateManagedTmuxWindowRuntime(ctx, managedWindow.ID, createdWindow.ID, createdWindow.Index); err != nil {
		return err
	}
	if strings.TrimSpace(managedWindow.Command) == "" {
		return nil
	}
	return tm.SendKeys(ctx, createdWindow.PaneID, managedWindow.Command, true)
}

func resolveManagedTmuxWindowCwd(managedWindow store.ManagedTmuxWindow, sessionCwd string) string {
	if resolved := strings.TrimSpace(managedWindow.ResolvedCwd); resolved != "" {
		return resolved
	}
	if managedWindow.CwdMode == store.TmuxLauncherCwdModeFixed {
		return strings.TrimSpace(managedWindow.CwdValue)
	}
	return strings.TrimSpace(sessionCwd)
}

func firstPaneForWindow(panes []tmux.Pane, windowIndex int) (tmux.Pane, bool) {
	for _, pane := range panes {
		if pane.WindowIndex == windowIndex && pane.Active {
			return pane, true
		}
	}
	for _, pane := range panes {
		if pane.WindowIndex == windowIndex {
			return pane, true
		}
	}
	return tmux.Pane{}, false
}
