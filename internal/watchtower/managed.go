package watchtower

import (
	"context"
	"strings"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

func managedWindowIndexMap(rows []store.ManagedTmuxWindow) map[int]store.ManagedTmuxWindow {
	byIndex := make(map[int]store.ManagedTmuxWindow, len(rows))
	for _, row := range rows {
		if row.LastWindowIndex < 0 {
			continue
		}
		byIndex[row.LastWindowIndex] = row
	}
	return byIndex
}

func managedWindowRuntimeMap(rows []store.ManagedTmuxWindow) map[string]store.ManagedTmuxWindow {
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

func (s *Service) reconcileManagedTmuxWindows(ctx context.Context, sessionName string, liveWindows []tmux.Window) ([]store.ManagedTmuxWindow, error) {
	rows, err := s.store.ListManagedTmuxWindowsBySession(ctx, sessionName)
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
			if err := s.store.UpdateManagedTmuxWindowRuntime(ctx, row.ID, runtimeID, liveWindow.Index); err == nil {
				row.LastWindowIndex = liveWindow.Index
			}
		}
		filtered = append(filtered, row)
	}

	if err := s.store.DeleteManagedTmuxWindowsMissingRuntime(ctx, sessionName, liveIDs); err != nil {
		return nil, err
	}
	return filtered, nil
}

func windowNamesByIndex(windows []tmux.Window, managedByRuntime map[string]store.ManagedTmuxWindow) map[int]string {
	byIndex := make(map[int]string, len(windows))
	for _, window := range windows {
		name := strings.TrimSpace(window.Name)
		if managed, ok := managedByRuntime[strings.TrimSpace(window.ID)]; ok {
			if managedName := strings.TrimSpace(managed.WindowName); managedName != "" {
				name = managedName
			}
		}
		if name == "" {
			continue
		}
		byIndex[window.Index] = name
	}
	return byIndex
}
