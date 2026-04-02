package api

import (
	"context"
	"strings"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

type managedWindowPresentation struct {
	displayName     string
	displayIcon     string
	managed         bool
	managedWindowID string
	launcherID      string
}

func presentationForLiveWindow(
	window tmux.Window,
	managedByRuntime map[string]store.ManagedTmuxWindow,
) managedWindowPresentation {
	return presentationForManagedWindow(window.Name, managedByRuntime[strings.TrimSpace(window.ID)])
}

func presentationForProjectedWindow(
	name string,
	tmuxWindowID string,
	managedByRuntime map[string]store.ManagedTmuxWindow,
) managedWindowPresentation {
	return presentationForManagedWindow(name, managedByRuntime[strings.TrimSpace(tmuxWindowID)])
}

func presentationForManagedWindow(
	fallbackName string,
	managed store.ManagedTmuxWindow,
) managedWindowPresentation {
	displayName := strings.TrimSpace(fallbackName)
	if name := strings.TrimSpace(managed.WindowName); name != "" {
		displayName = name
	}
	if displayName == "" {
		displayName = strings.TrimSpace(fallbackName)
	}
	return managedWindowPresentation{
		displayName:     displayName,
		displayIcon:     strings.TrimSpace(managed.Icon),
		managed:         strings.TrimSpace(managed.ID) != "",
		managedWindowID: strings.TrimSpace(managed.ID),
		launcherID:      strings.TrimSpace(managed.LauncherID),
	}
}

func managedWindowsByRuntime(rows []store.ManagedTmuxWindow) map[string]store.ManagedTmuxWindow {
	byRuntime := make(map[string]store.ManagedTmuxWindow, len(rows))
	for _, row := range rows {
		runtimeID := strings.TrimSpace(row.TmuxWindowID)
		if runtimeID == "" {
			continue
		}
		byRuntime[runtimeID] = row
	}
	return byRuntime
}

func (h *Handler) listManagedTmuxWindows(ctx context.Context, session string) ([]store.ManagedTmuxWindow, error) {
	if h.repo == nil {
		return nil, nil
	}
	rows, err := h.repo.ListManagedTmuxWindowsBySession(ctx, session)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (h *Handler) reconcileManagedTmuxWindows(ctx context.Context, session string, liveWindows []tmux.Window) ([]store.ManagedTmuxWindow, error) {
	rows, err := h.listManagedTmuxWindows(ctx, session)
	if err != nil || len(rows) == 0 {
		return rows, err
	}

	liveByID := make(map[string]tmux.Window, len(liveWindows))
	liveIDs := make([]string, 0, len(liveWindows))
	for _, window := range liveWindows {
		windowID := strings.TrimSpace(window.ID)
		if windowID == "" {
			continue
		}
		liveByID[windowID] = window
		liveIDs = append(liveIDs, windowID)
	}

	filtered := make([]store.ManagedTmuxWindow, 0, len(rows))
	for _, row := range rows {
		runtimeID := strings.TrimSpace(row.TmuxWindowID)
		if runtimeID == "" {
			filtered = append(filtered, row)
			continue
		}
		liveWindow, ok := liveByID[runtimeID]
		if !ok {
			continue
		}
		if row.LastWindowIndex != liveWindow.Index {
			if err := h.repo.UpdateManagedTmuxWindowRuntime(ctx, row.ID, runtimeID, liveWindow.Index); err == nil {
				row.LastWindowIndex = liveWindow.Index
			}
		}
		filtered = append(filtered, row)
	}

	if err := h.repo.DeleteManagedTmuxWindowsMissingRuntime(ctx, session, liveIDs); err != nil {
		return nil, err
	}
	return filtered, nil
}

func (h *Handler) managedTmuxWindowForIndex(ctx context.Context, session string, index int) (store.ManagedTmuxWindow, bool, error) {
	liveWindows, err := h.tmux.ListWindows(ctx, session)
	if err != nil {
		return store.ManagedTmuxWindow{}, false, err
	}
	managedRows, err := h.reconcileManagedTmuxWindows(ctx, session, liveWindows)
	if err != nil {
		return store.ManagedTmuxWindow{}, false, err
	}
	managedByRuntime := managedWindowsByRuntime(managedRows)
	for _, window := range liveWindows {
		if window.Index != index {
			continue
		}
		row, ok := managedByRuntime[strings.TrimSpace(window.ID)]
		return row, ok, nil
	}
	return store.ManagedTmuxWindow{}, false, nil
}

func (h *Handler) syncManagedTmuxWindowOrder(ctx context.Context, session string, liveWindows []tmux.Window) error {
	rows, err := h.listManagedTmuxWindows(ctx, session)
	if err != nil || len(rows) == 0 {
		return err
	}

	managedByRuntime := managedWindowsByRuntime(rows)
	for order, window := range liveWindows {
		runtimeID := strings.TrimSpace(window.ID)
		if runtimeID == "" {
			continue
		}
		row, ok := managedByRuntime[runtimeID]
		if !ok {
			continue
		}
		if row.LastWindowIndex != window.Index {
			if err := h.repo.UpdateManagedTmuxWindowRuntime(ctx, row.ID, runtimeID, window.Index); err != nil {
				return err
			}
		}
		nextSortOrder := order + 1
		if row.SortOrder != nextSortOrder {
			if err := h.repo.UpdateManagedTmuxWindowSortOrder(ctx, row.ID, nextSortOrder); err != nil {
				return err
			}
		}
	}
	return nil
}
