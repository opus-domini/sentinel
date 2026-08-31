package store

import (
	"context"
	"testing"
	"time"
)

func TestWriteWatchtowerSessionProjection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.April, 23, 10, 0, 0, 0, time.UTC)

	t.Run("writes rows and purges what tmux no longer reports", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)

		first := WatchtowerSessionProjection{
			Session: WatchtowerSessionWrite{
				SessionName: "dev", Windows: 2, Panes: 2, ActivityAt: now, Rev: 1, UpdatedAt: now,
			},
			Windows: []WatchtowerWindowWrite{
				{SessionName: "dev", TmuxWindowID: "@0", WindowIndex: 0, Name: "main", Active: true, Rev: 1, UpdatedAt: now},
				{SessionName: "dev", TmuxWindowID: "@1", WindowIndex: 1, Name: "logs", Rev: 1, UpdatedAt: now},
			},
			Panes: []WatchtowerPaneWrite{
				{PaneID: "%0", SessionName: "dev", WindowIndex: 0, Revision: 1, UpdatedAt: now},
				{PaneID: "%1", SessionName: "dev", WindowIndex: 1, Revision: 1, UpdatedAt: now},
			},
			ActivePaneIDs:       []string{"%0", "%1"},
			ActiveWindowIndices: []int{0, 1},
		}
		if err := s.WriteWatchtowerSessionProjection(ctx, first); err != nil {
			t.Fatalf("WriteWatchtowerSessionProjection(first): %v", err)
		}

		// Window 1 and pane %1 are gone on the next tick.
		second := WatchtowerSessionProjection{
			Session: WatchtowerSessionWrite{
				SessionName: "dev", Windows: 1, Panes: 1, ActivityAt: now, Rev: 2, UpdatedAt: now,
			},
			Windows: []WatchtowerWindowWrite{
				{SessionName: "dev", TmuxWindowID: "@0", WindowIndex: 0, Name: "main", Active: true, Rev: 2, UpdatedAt: now},
			},
			Panes: []WatchtowerPaneWrite{
				{PaneID: "%0", SessionName: "dev", WindowIndex: 0, Revision: 2, UpdatedAt: now},
			},
			ActivePaneIDs:       []string{"%0"},
			ActiveWindowIndices: []int{0},
		}
		if err := s.WriteWatchtowerSessionProjection(ctx, second); err != nil {
			t.Fatalf("WriteWatchtowerSessionProjection(second): %v", err)
		}

		panes, err := s.ListWatchtowerPanes(ctx, "dev")
		if err != nil {
			t.Fatalf("ListWatchtowerPanes: %v", err)
		}
		if len(panes) != 1 || panes[0].PaneID != "%0" || panes[0].Revision != 2 {
			t.Fatalf("panes = %+v, want only %%0 at revision 2", panes)
		}
		windows, err := s.ListWatchtowerWindows(ctx, "dev")
		if err != nil {
			t.Fatalf("ListWatchtowerWindows: %v", err)
		}
		if len(windows) != 1 || windows[0].WindowIndex != 0 {
			t.Fatalf("windows = %+v, want only index 0", windows)
		}
		session, err := s.GetWatchtowerSession(ctx, "dev")
		if err != nil {
			t.Fatalf("GetWatchtowerSession: %v", err)
		}
		if session.Rev != 2 || session.Panes != 1 {
			t.Fatalf("session = %+v, want rev 2 and 1 pane", session)
		}
	})

	t.Run("a bad row rolls the whole tick back", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)

		if err := s.WriteWatchtowerSessionProjection(ctx, WatchtowerSessionProjection{
			Session: WatchtowerSessionWrite{SessionName: "dev", Panes: 1, ActivityAt: now, Rev: 1, UpdatedAt: now},
			Panes: []WatchtowerPaneWrite{
				{PaneID: "%0", SessionName: "dev", Revision: 1, UpdatedAt: now},
				{PaneID: "", SessionName: "dev", Revision: 1, UpdatedAt: now},
			},
			ActivePaneIDs: []string{"%0"},
		}); err == nil {
			t.Fatal("expected an error for a pane row with no id")
		}

		panes, err := s.ListWatchtowerPanes(ctx, "dev")
		if err != nil {
			t.Fatalf("ListWatchtowerPanes: %v", err)
		}
		if len(panes) != 0 {
			t.Fatalf("panes = %+v, want none: a failed tick must not leave partial state", panes)
		}
		if _, err := s.GetWatchtowerSession(ctx, "dev"); err == nil {
			t.Fatal("session row was committed despite the failed tick")
		}
	})

	t.Run("empty session name is rejected", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)

		if err := s.WriteWatchtowerSessionProjection(ctx, WatchtowerSessionProjection{
			Session: WatchtowerSessionWrite{SessionName: "  "},
		}); err == nil {
			t.Fatal("expected an error for an empty session name")
		}
	})
}
