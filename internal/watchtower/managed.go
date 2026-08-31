package watchtower

import (
	"context"
	"strings"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

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

func (s *Service) reconcileManagedTmuxWindows(ctx context.Context, sessionName string, liveWindows []tmux.Window) error {
	rows, err := s.store.ListManagedTmuxWindowsBySession(ctx, sessionName)
	if err != nil || len(rows) == 0 {
		return err
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

	for _, row := range rows {
		runtimeID := strings.TrimSpace(row.TmuxWindowID)
		if runtimeID == "" {
			continue
		}
		liveWindow, ok := liveByID[runtimeID]
		if !ok {
			continue
		}
		if row.LastWindowIndex != liveWindow.Index {
			// Best-effort: a failed index refresh is retried on the next tick.
			_ = s.store.UpdateManagedTmuxWindowRuntime(ctx, row.ID, runtimeID, liveWindow.Index)
		}
	}

	return s.store.DeleteManagedTmuxWindowsMissingRuntime(ctx, sessionName, liveIDs)
}
