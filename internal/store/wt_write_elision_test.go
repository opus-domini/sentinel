package store

import (
	"context"
	"testing"
	"time"
)

var elisionBase = time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

func basePaneWrite() WatchtowerPaneWrite {
	return WatchtowerPaneWrite{
		PaneID:         "%1",
		SessionName:    "dev",
		WindowIndex:    0,
		PaneIndex:      0,
		Title:          "shell",
		Active:         true,
		TTY:            "/dev/pts/1",
		CurrentPath:    "/home/dev",
		StartCommand:   "zsh",
		CurrentCommand: "zsh",
		TailHash:       "hash-1",
		TailPreview:    "line one",
		TailCapturedAt: elisionBase,
		Revision:       3,
		SeenRevision:   1,
		ChangedAt:      elisionBase,
		UpdatedAt:      elisionBase,
	}
}

func baseWindowWrite() WatchtowerWindowWrite {
	return WatchtowerWindowWrite{
		SessionName:      "dev",
		TmuxWindowID:     "@1",
		WindowIndex:      0,
		Name:             "main",
		Active:           true,
		Layout:           "layout-a",
		WindowActivityAt: elisionBase,
		UnreadPanes:      1,
		HasUnread:        true,
		Rev:              2,
		UpdatedAt:        elisionBase,
	}
}

func baseSessionWrite() WatchtowerSessionWrite {
	return WatchtowerSessionWrite{
		SessionName:       "dev",
		Attached:          1,
		Windows:           2,
		Panes:             3,
		ActivityAt:        elisionBase,
		LastPreview:       "preview",
		LastPreviewAt:     elisionBase,
		LastPreviewPaneID: "%1",
		UnreadWindows:     1,
		UnreadPanes:       2,
		Rev:               5,
		UpdatedAt:         elisionBase,
	}
}

func rawColumn(t *testing.T, s *Store, query string, args ...any) string {
	t.Helper()
	var value string
	if err := s.db.QueryRowContext(context.Background(), query, args...).Scan(&value); err != nil {
		t.Fatalf("scan %q: %v", query, err)
	}
	return value
}

func paneUpdatedAt(t *testing.T, s *Store) string {
	t.Helper()
	return rawColumn(t, s, "SELECT updated_at FROM wt_panes WHERE pane_id = ?", "%1")
}

func windowUpdatedAt(t *testing.T, s *Store) string {
	t.Helper()
	return rawColumn(t, s, "SELECT updated_at FROM wt_windows WHERE session_name = ? AND window_index = 0", "dev")
}

func sessionUpdatedAt(t *testing.T, s *Store) string {
	t.Helper()
	return rawColumn(t, s, "SELECT updated_at FROM wt_sessions WHERE session_name = ?", "dev")
}

// WAL without synchronous=NORMAL fsyncs on every commit, and every commit on
// this store queues behind the single connection every reader also uses.
func TestStorePragmasPairWalWithNormalSynchronous(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	if got := rawColumn(t, s, "PRAGMA journal_mode"); got != "wal" {
		t.Fatalf("journal_mode = %q, want wal", got)
	}
	// 1 is NORMAL.
	if got := rawColumn(t, s, "PRAGMA synchronous"); got != "1" {
		t.Fatalf("synchronous = %q, want 1 (NORMAL)", got)
	}
}

// The collector re-upserts every pane, window and session once per tick. An
// upsert that carries no observable change must not touch the row, otherwise an
// idle host pays a WAL page write per projection row per second.
func TestWatchtowerProjectionUpsertsSkipUnchangedRows(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	if err := s.UpsertWatchtowerPane(ctx, basePaneWrite()); err != nil {
		t.Fatalf("seed pane: %v", err)
	}
	if err := s.UpsertWatchtowerWindow(ctx, baseWindowWrite()); err != nil {
		t.Fatalf("seed window: %v", err)
	}
	if err := s.UpsertWatchtowerSession(ctx, baseSessionWrite()); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	paneBefore := paneUpdatedAt(t, s)
	windowBefore := windowUpdatedAt(t, s)
	sessionBefore := sessionUpdatedAt(t, s)

	// A later tick: same projection, fresh wall clock. This is the shape the
	// collector produces on an idle host.
	later := elisionBase.Add(time.Minute)

	pane := basePaneWrite()
	pane.UpdatedAt = later
	pane.TailCapturedAt = later
	// A stale seen_revision must stay clamped and must not count as a change.
	pane.SeenRevision = 0
	if err := s.UpsertWatchtowerPane(ctx, pane); err != nil {
		t.Fatalf("re-upsert pane: %v", err)
	}

	window := baseWindowWrite()
	window.UpdatedAt = later
	if err := s.UpsertWatchtowerWindow(ctx, window); err != nil {
		t.Fatalf("re-upsert window: %v", err)
	}

	session := baseSessionWrite()
	session.UpdatedAt = later
	if err := s.UpsertWatchtowerSession(ctx, session); err != nil {
		t.Fatalf("re-upsert session: %v", err)
	}

	if got := paneUpdatedAt(t, s); got != paneBefore {
		t.Fatalf("pane row rewritten by unchanged upsert: updated_at = %q, want %q", got, paneBefore)
	}
	if got := windowUpdatedAt(t, s); got != windowBefore {
		t.Fatalf("window row rewritten by unchanged upsert: updated_at = %q, want %q", got, windowBefore)
	}
	if got := sessionUpdatedAt(t, s); got != sessionBefore {
		t.Fatalf("session row rewritten by unchanged upsert: updated_at = %q, want %q", got, sessionBefore)
	}

	panes, err := s.ListWatchtowerPanes(ctx, "dev")
	if err != nil {
		t.Fatalf("ListWatchtowerPanes: %v", err)
	}
	if len(panes) != 1 || panes[0].SeenRevision != 1 {
		t.Fatalf("seen_revision clamp broken: %+v", panes)
	}
}

// Every column the guard compares must still round-trip: a guard that is too
// wide would silently drop real collector updates.
func TestWatchtowerPaneUpsertPersistsEveryGuardedField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(w *WatchtowerPaneWrite)
		want   func(row WatchtowerPane) bool
	}{
		{"session name", func(w *WatchtowerPaneWrite) { w.SessionName = "ops" }, nil},
		{"window index", func(w *WatchtowerPaneWrite) { w.WindowIndex = 4 }, func(r WatchtowerPane) bool { return r.WindowIndex == 4 }},
		{"pane index", func(w *WatchtowerPaneWrite) { w.PaneIndex = 7 }, func(r WatchtowerPane) bool { return r.PaneIndex == 7 }},
		{"title", func(w *WatchtowerPaneWrite) { w.Title = "vim" }, func(r WatchtowerPane) bool { return r.Title == "vim" }},
		{"active", func(w *WatchtowerPaneWrite) { w.Active = false }, func(r WatchtowerPane) bool { return !r.Active }},
		{"tty", func(w *WatchtowerPaneWrite) { w.TTY = "/dev/pts/9" }, func(r WatchtowerPane) bool { return r.TTY == "/dev/pts/9" }},
		{"current path", func(w *WatchtowerPaneWrite) { w.CurrentPath = "/tmp" }, func(r WatchtowerPane) bool { return r.CurrentPath == "/tmp" }},
		{"start command", func(w *WatchtowerPaneWrite) { w.StartCommand = "bash" }, func(r WatchtowerPane) bool { return r.StartCommand == "bash" }},
		{"current command", func(w *WatchtowerPaneWrite) { w.CurrentCommand = "htop" }, func(r WatchtowerPane) bool { return r.CurrentCommand == "htop" }},
		{"tail hash", func(w *WatchtowerPaneWrite) { w.TailHash = "hash-2" }, func(r WatchtowerPane) bool { return r.TailHash == "hash-2" }},
		{"tail preview", func(w *WatchtowerPaneWrite) { w.TailPreview = "line two" }, func(r WatchtowerPane) bool { return r.TailPreview == "line two" }},
		{"revision", func(w *WatchtowerPaneWrite) { w.Revision = 9 }, func(r WatchtowerPane) bool { return r.Revision == 9 }},
		{"seen revision", func(w *WatchtowerPaneWrite) { w.SeenRevision = 3 }, func(r WatchtowerPane) bool { return r.SeenRevision == 3 }},
		{
			"changed at",
			func(w *WatchtowerPaneWrite) { w.ChangedAt = elisionBase.Add(time.Hour) },
			func(r WatchtowerPane) bool { return r.ChangedAt.Equal(elisionBase.Add(time.Hour)) },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := newTestStore(t)
			defer func() { _ = s.Close() }()
			ctx := context.Background()

			if err := s.UpsertWatchtowerPane(ctx, basePaneWrite()); err != nil {
				t.Fatalf("seed pane: %v", err)
			}
			before := paneUpdatedAt(t, s)

			write := basePaneWrite()
			write.UpdatedAt = elisionBase.Add(time.Minute)
			tc.mutate(&write)
			if err := s.UpsertWatchtowerPane(ctx, write); err != nil {
				t.Fatalf("upsert pane: %v", err)
			}

			if got := paneUpdatedAt(t, s); got == before {
				t.Fatalf("changing %s did not rewrite the row", tc.name)
			}
			if tc.want == nil {
				return
			}
			panes, err := s.ListWatchtowerPanes(ctx, write.SessionName)
			if err != nil {
				t.Fatalf("ListWatchtowerPanes: %v", err)
			}
			if len(panes) != 1 {
				t.Fatalf("panes = %d, want 1", len(panes))
			}
			if !tc.want(panes[0]) {
				t.Fatalf("changing %s did not persist: %+v", tc.name, panes[0])
			}
		})
	}
}

func TestWatchtowerWindowUpsertPersistsEveryGuardedField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(w *WatchtowerWindowWrite)
		want   func(row WatchtowerWindow) bool
	}{
		{"tmux window id", func(w *WatchtowerWindowWrite) { w.TmuxWindowID = "@9" }, func(r WatchtowerWindow) bool { return r.TmuxWindowID == "@9" }},
		{"name", func(w *WatchtowerWindowWrite) { w.Name = "logs" }, func(r WatchtowerWindow) bool { return r.Name == "logs" }},
		{"active", func(w *WatchtowerWindowWrite) { w.Active = false }, func(r WatchtowerWindow) bool { return !r.Active }},
		{"layout", func(w *WatchtowerWindowWrite) { w.Layout = "layout-b" }, func(r WatchtowerWindow) bool { return r.Layout == "layout-b" }},
		{
			"window activity at",
			func(w *WatchtowerWindowWrite) { w.WindowActivityAt = elisionBase.Add(time.Hour) },
			func(r WatchtowerWindow) bool { return r.WindowActivityAt.Equal(elisionBase.Add(time.Hour)) },
		},
		{"unread panes", func(w *WatchtowerWindowWrite) { w.UnreadPanes = 4 }, func(r WatchtowerWindow) bool { return r.UnreadPanes == 4 }},
		{"has unread", func(w *WatchtowerWindowWrite) { w.HasUnread = false }, func(r WatchtowerWindow) bool { return !r.HasUnread }},
		{"rev", func(w *WatchtowerWindowWrite) { w.Rev = 11 }, func(r WatchtowerWindow) bool { return r.Rev == 11 }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := newTestStore(t)
			defer func() { _ = s.Close() }()
			ctx := context.Background()

			if err := s.UpsertWatchtowerWindow(ctx, baseWindowWrite()); err != nil {
				t.Fatalf("seed window: %v", err)
			}
			before := windowUpdatedAt(t, s)

			write := baseWindowWrite()
			write.UpdatedAt = elisionBase.Add(time.Minute)
			tc.mutate(&write)
			if err := s.UpsertWatchtowerWindow(ctx, write); err != nil {
				t.Fatalf("upsert window: %v", err)
			}

			if got := windowUpdatedAt(t, s); got == before {
				t.Fatalf("changing %s did not rewrite the row", tc.name)
			}
			windows, err := s.ListWatchtowerWindows(ctx, "dev")
			if err != nil {
				t.Fatalf("ListWatchtowerWindows: %v", err)
			}
			if len(windows) != 1 {
				t.Fatalf("windows = %d, want 1", len(windows))
			}
			if !tc.want(windows[0]) {
				t.Fatalf("changing %s did not persist: %+v", tc.name, windows[0])
			}
		})
	}
}

func TestWatchtowerSessionUpsertPersistsEveryGuardedField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(w *WatchtowerSessionWrite)
		want   func(row WatchtowerSession) bool
	}{
		{"attached", func(w *WatchtowerSessionWrite) { w.Attached = 0 }, func(r WatchtowerSession) bool { return r.Attached == 0 }},
		{"windows", func(w *WatchtowerSessionWrite) { w.Windows = 6 }, func(r WatchtowerSession) bool { return r.Windows == 6 }},
		{"panes", func(w *WatchtowerSessionWrite) { w.Panes = 8 }, func(r WatchtowerSession) bool { return r.Panes == 8 }},
		{
			"activity at",
			func(w *WatchtowerSessionWrite) { w.ActivityAt = elisionBase.Add(time.Hour) },
			func(r WatchtowerSession) bool { return r.ActivityAt.Equal(elisionBase.Add(time.Hour)) },
		},
		{"last preview", func(w *WatchtowerSessionWrite) { w.LastPreview = "other" }, func(r WatchtowerSession) bool { return r.LastPreview == "other" }},
		{
			"last preview at",
			func(w *WatchtowerSessionWrite) { w.LastPreviewAt = elisionBase.Add(time.Hour) },
			func(r WatchtowerSession) bool { return r.LastPreviewAt.Equal(elisionBase.Add(time.Hour)) },
		},
		{"last preview pane id", func(w *WatchtowerSessionWrite) { w.LastPreviewPaneID = "%9" }, func(r WatchtowerSession) bool { return r.LastPreviewPaneID == "%9" }},
		{"unread windows", func(w *WatchtowerSessionWrite) { w.UnreadWindows = 4 }, func(r WatchtowerSession) bool { return r.UnreadWindows == 4 }},
		{"unread panes", func(w *WatchtowerSessionWrite) { w.UnreadPanes = 5 }, func(r WatchtowerSession) bool { return r.UnreadPanes == 5 }},
		{"rev", func(w *WatchtowerSessionWrite) { w.Rev = 12 }, func(r WatchtowerSession) bool { return r.Rev == 12 }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := newTestStore(t)
			defer func() { _ = s.Close() }()
			ctx := context.Background()

			if err := s.UpsertWatchtowerSession(ctx, baseSessionWrite()); err != nil {
				t.Fatalf("seed session: %v", err)
			}
			before := sessionUpdatedAt(t, s)

			write := baseSessionWrite()
			write.UpdatedAt = elisionBase.Add(time.Minute)
			tc.mutate(&write)
			if err := s.UpsertWatchtowerSession(ctx, write); err != nil {
				t.Fatalf("upsert session: %v", err)
			}

			if got := sessionUpdatedAt(t, s); got == before {
				t.Fatalf("changing %s did not rewrite the row", tc.name)
			}
			row, err := s.GetWatchtowerSession(ctx, "dev")
			if err != nil {
				t.Fatalf("GetWatchtowerSession: %v", err)
			}
			if !tc.want(row) {
				t.Fatalf("changing %s did not persist: %+v", tc.name, row)
			}
		})
	}
}

// TestWatchtowerPaneRevisionNeverGoesBackwards pins the clamp that keeps a pane
// able to report unread. The collector reads existing panes filtered by session
// name while the upsert conflicts on pane_id alone, so after a session rename a
// pane arrives looking brand new, with revision 1. Without the clamp that
// overwrote a high revision while seen_revision kept its own clamped value,
// leaving revision permanently below seen_revision — and unread is
// revision > seen_revision, so the pane could never light up again.
func TestWatchtowerPaneRevisionNeverGoesBackwards(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	at := time.Date(2026, 8, 31, 4, 0, 0, 0, time.UTC)
	established := WatchtowerPaneWrite{
		PaneID: "%1", SessionName: "old", WindowIndex: 0, PaneIndex: 0,
		Title: "shell", TailHash: "aaa", Revision: 500, SeenRevision: 500,
		TailCapturedAt: at, ChangedAt: at, UpdatedAt: at,
	}
	if err := s.UpsertWatchtowerPane(ctx, established); err != nil {
		t.Fatalf("UpsertWatchtowerPane() error = %v", err)
	}

	// The same pane as the collector sees it after a rename: no prior row was
	// found for the new session name, so it starts counting from one again.
	renamed := established
	renamed.SessionName = "new"
	renamed.Revision = 1
	renamed.SeenRevision = 0
	renamed.TailHash = "bbb"
	if err := s.UpsertWatchtowerPane(ctx, renamed); err != nil {
		t.Fatalf("UpsertWatchtowerPane(renamed) error = %v", err)
	}

	var revision, seen int64
	var session string
	if err := s.db.QueryRowContext(ctx,
		"SELECT session_name, revision, seen_revision FROM wt_panes WHERE pane_id = ?", "%1",
	).Scan(&session, &revision, &seen); err != nil {
		t.Fatalf("QueryRow() error = %v", err)
	}
	if session != "new" {
		t.Fatalf("session_name = %q, want the renamed value", session)
	}
	if revision < seen {
		t.Fatalf("revision = %d, seen_revision = %d: revision fell below seen, so the pane can never report unread again", revision, seen)
	}
	if revision != 500 {
		t.Fatalf("revision = %d, want the established 500 to be kept", revision)
	}
}
