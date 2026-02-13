package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"
)

func TestWatchtowerSchemaCreated(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	wantTables := []string{
		"wt_journal",
		"wt_panes",
		"wt_presence",
		"wt_runtime",
		"wt_sessions",
		"wt_windows",
	}

	gotTables := make([]string, 0, len(wantTables))
	rows, err := s.db.QueryContext(ctx,
		`SELECT name
		   FROM sqlite_master
		  WHERE type = 'table'
		    AND name LIKE 'wt_%'
		  ORDER BY name ASC`,
	)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		gotTables = append(gotTables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tables: %v", err)
	}

	if fmt.Sprint(gotTables) != fmt.Sprint(wantTables) {
		t.Fatalf("watchtower tables = %v, want %v", gotTables, wantTables)
	}
}

func TestWatchtowerSchemaIdempotentAndBackfill(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	if err := s.UpsertSession(ctx, "dev", "h1", "last line"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	if err := s.initWatchtowerSchema(); err != nil {
		t.Fatalf("initWatchtowerSchema second call: %v", err)
	}
	if err := s.initWatchtowerSchema(); err != nil {
		t.Fatalf("initWatchtowerSchema third call: %v", err)
	}

	row, err := s.GetWatchtowerSession(ctx, "dev")
	if err != nil {
		t.Fatalf("GetWatchtowerSession(dev): %v", err)
	}
	if row.LastPreview != "last line" {
		t.Fatalf("LastPreview = %q, want %q", row.LastPreview, "last line")
	}
}

func TestWatchtowerSessionAccessors(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.UpsertWatchtowerSession(ctx, WatchtowerSessionWrite{
		SessionName:       "dev",
		Attached:          1,
		Windows:           2,
		Panes:             4,
		ActivityAt:        now,
		LastPreview:       "go test ./...",
		LastPreviewAt:     now,
		LastPreviewPaneID: "%3",
		UnreadWindows:     1,
		UnreadPanes:       2,
		Rev:               12,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerSession: %v", err)
	}

	row, err := s.GetWatchtowerSession(ctx, "dev")
	if err != nil {
		t.Fatalf("GetWatchtowerSession(dev): %v", err)
	}

	if row.Attached != 1 || row.Windows != 2 || row.Panes != 4 {
		t.Fatalf("unexpected session counts: %+v", row)
	}
	if row.LastPreview != "go test ./..." || row.LastPreviewPaneID != "%3" {
		t.Fatalf("unexpected preview fields: %+v", row)
	}
	if row.UnreadWindows != 1 || row.UnreadPanes != 2 || row.Rev != 12 {
		t.Fatalf("unexpected unread/rev: %+v", row)
	}

	list, err := s.ListWatchtowerSessions(ctx)
	if err != nil {
		t.Fatalf("ListWatchtowerSessions: %v", err)
	}
	if len(list) != 1 || list[0].SessionName != "dev" {
		t.Fatalf("ListWatchtowerSessions = %+v, want 1 row for dev", list)
	}
}

func TestWatchtowerWindowAndPaneAccessors(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for _, w := range []WatchtowerWindowWrite{
		{
			SessionName:      "dev",
			WindowIndex:      1,
			Name:             "logs",
			Active:           false,
			Layout:           "abcd",
			WindowActivityAt: now,
			UnreadPanes:      1,
			HasUnread:        true,
			Rev:              5,
		},
		{
			SessionName:      "dev",
			WindowIndex:      0,
			Name:             "editor",
			Active:           true,
			Layout:           "efgh",
			WindowActivityAt: now,
			UnreadPanes:      0,
			HasUnread:        false,
			Rev:              4,
		},
	} {
		if err := s.UpsertWatchtowerWindow(ctx, w); err != nil {
			t.Fatalf("UpsertWatchtowerWindow(%d): %v", w.WindowIndex, err)
		}
	}

	windows, err := s.ListWatchtowerWindows(ctx, "dev")
	if err != nil {
		t.Fatalf("ListWatchtowerWindows: %v", err)
	}
	if len(windows) != 2 {
		t.Fatalf("windows len = %d, want 2", len(windows))
	}
	if windows[0].WindowIndex != 0 || windows[1].WindowIndex != 1 {
		t.Fatalf("windows not sorted by index: %+v", windows)
	}
	if !windows[1].HasUnread || windows[1].UnreadPanes != 1 {
		t.Fatalf("unexpected unread window state: %+v", windows[1])
	}

	for _, p := range []WatchtowerPaneWrite{
		{
			PaneID:         "%11",
			SessionName:    "dev",
			WindowIndex:    1,
			PaneIndex:      0,
			Title:          "tail",
			Active:         false,
			TTY:            "/dev/pts/2",
			CurrentPath:    "/tmp",
			StartCommand:   "tail",
			CurrentCommand: "tail -f app.log",
			TailHash:       "aa",
			TailPreview:    "line 1",
			TailCapturedAt: now,
			Revision:       3,
			SeenRevision:   1,
			ChangedAt:      now,
		},
		{
			PaneID:         "%12",
			SessionName:    "dev",
			WindowIndex:    0,
			PaneIndex:      0,
			Title:          "shell",
			Active:         true,
			TTY:            "/dev/pts/3",
			CurrentPath:    "/home/hugo",
			StartCommand:   "zsh",
			CurrentCommand: "zsh",
			TailHash:       "bb",
			TailPreview:    "$",
			TailCapturedAt: now,
			Revision:       8,
			SeenRevision:   8,
			ChangedAt:      now,
		},
	} {
		if err := s.UpsertWatchtowerPane(ctx, p); err != nil {
			t.Fatalf("UpsertWatchtowerPane(%s): %v", p.PaneID, err)
		}
	}

	panes, err := s.ListWatchtowerPanes(ctx, "dev")
	if err != nil {
		t.Fatalf("ListWatchtowerPanes: %v", err)
	}
	if len(panes) != 2 {
		t.Fatalf("panes len = %d, want 2", len(panes))
	}

	gotPaneIDs := []string{panes[0].PaneID, panes[1].PaneID}
	sort.Strings(gotPaneIDs)
	if fmt.Sprint(gotPaneIDs) != fmt.Sprint([]string{"%11", "%12"}) {
		t.Fatalf("pane ids = %v, want [%s %s]", gotPaneIDs, "%11", "%12")
	}
}

func TestWatchtowerPresenceAccessors(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.UpsertWatchtowerPresence(ctx, WatchtowerPresenceWrite{
		TerminalID:  "term-1",
		SessionName: "dev",
		WindowIndex: 1,
		PaneID:      "%11",
		Visible:     true,
		Focused:     true,
		UpdatedAt:   now,
		ExpiresAt:   now.Add(15 * time.Second),
	}); err != nil {
		t.Fatalf("UpsertWatchtowerPresence(term-1): %v", err)
	}
	if err := s.UpsertWatchtowerPresence(ctx, WatchtowerPresenceWrite{
		TerminalID:  "term-old",
		SessionName: "dev",
		WindowIndex: 0,
		PaneID:      "%10",
		Visible:     false,
		Focused:     false,
		UpdatedAt:   now.Add(-30 * time.Second),
		ExpiresAt:   now.Add(-1 * time.Second),
	}); err != nil {
		t.Fatalf("UpsertWatchtowerPresence(term-old): %v", err)
	}

	removed, err := s.PruneWatchtowerPresence(ctx, now)
	if err != nil {
		t.Fatalf("PruneWatchtowerPresence: %v", err)
	}
	if removed != 1 {
		t.Fatalf("PruneWatchtowerPresence removed = %d, want 1", removed)
	}

	presences, err := s.ListWatchtowerPresence(ctx)
	if err != nil {
		t.Fatalf("ListWatchtowerPresence: %v", err)
	}
	if len(presences) != 1 || presences[0].TerminalID != "term-1" {
		t.Fatalf("presence = %+v, want only term-1", presences)
	}
	if !presences[0].Visible || !presences[0].Focused {
		t.Fatalf("unexpected presence flags: %+v", presences[0])
	}

	bySession, err := s.ListWatchtowerPresenceBySession(ctx, "dev")
	if err != nil {
		t.Fatalf("ListWatchtowerPresenceBySession(dev): %v", err)
	}
	if len(bySession) != 1 || bySession[0].TerminalID != "term-1" {
		t.Fatalf("presence by session = %+v, want only term-1", bySession)
	}
}

func TestWatchtowerJournalAndRuntimeAccessors(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// runtime
	missing, err := s.GetWatchtowerRuntimeValue(ctx, "missing")
	if err != nil {
		t.Fatalf("GetWatchtowerRuntimeValue(missing): %v", err)
	}
	if missing != "" {
		t.Fatalf("missing runtime value = %q, want empty", missing)
	}
	if err := s.SetWatchtowerRuntimeValue(ctx, "global_rev", "41"); err != nil {
		t.Fatalf("SetWatchtowerRuntimeValue: %v", err)
	}
	got, err := s.GetWatchtowerRuntimeValue(ctx, "global_rev")
	if err != nil {
		t.Fatalf("GetWatchtowerRuntimeValue(global_rev): %v", err)
	}
	if got != "41" {
		t.Fatalf("runtime global_rev = %q, want 41", got)
	}

	// journal
	if _, err := s.InsertWatchtowerJournal(ctx, WatchtowerJournalWrite{
		GlobalRev:  1,
		EntityType: "pane",
		Session:    "dev",
		WindowIdx:  1,
		PaneID:     "%11",
		ChangeKind: "tail-changed",
		ChangedAt:  now,
	}); err != nil {
		t.Fatalf("InsertWatchtowerJournal #1: %v", err)
	}
	if _, err := s.InsertWatchtowerJournal(ctx, WatchtowerJournalWrite{
		GlobalRev:  2,
		EntityType: "window",
		Session:    "dev",
		WindowIdx:  1,
		ChangeKind: "unread-updated",
		ChangedAt:  now.Add(time.Second),
	}); err != nil {
		t.Fatalf("InsertWatchtowerJournal #2: %v", err)
	}

	entries, err := s.ListWatchtowerJournalSince(ctx, 1, 10)
	if err != nil {
		t.Fatalf("ListWatchtowerJournalSince: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("journal entries len = %d, want 1", len(entries))
	}
	if entries[0].GlobalRev != 2 || entries[0].EntityType != "window" {
		t.Fatalf("unexpected journal entry: %+v", entries[0])
	}

	for i := 3; i <= 8; i++ {
		if _, err := s.InsertWatchtowerJournal(ctx, WatchtowerJournalWrite{
			GlobalRev:  int64(i),
			EntityType: "session",
			Session:    "dev",
			WindowIdx:  -1,
			ChangeKind: "activity",
			ChangedAt:  now.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("InsertWatchtowerJournal #%d: %v", i, err)
		}
	}

	removed, err := s.PruneWatchtowerJournalRows(ctx, 3)
	if err != nil {
		t.Fatalf("PruneWatchtowerJournalRows(3): %v", err)
	}
	if removed != 5 {
		t.Fatalf("PruneWatchtowerJournalRows removed = %d, want 5", removed)
	}

	remaining, err := s.ListWatchtowerJournalSince(ctx, 0, 10)
	if err != nil {
		t.Fatalf("ListWatchtowerJournalSince after prune: %v", err)
	}
	if len(remaining) != 3 {
		t.Fatalf("remaining journal entries len = %d, want 3", len(remaining))
	}
	if remaining[0].GlobalRev != 6 || remaining[2].GlobalRev != 8 {
		t.Fatalf("unexpected remaining journal range: %+v", remaining)
	}
}

func TestWatchtowerPurgeHelpers(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	seedSession := func(name string) {
		t.Helper()
		if err := s.UpsertWatchtowerSession(ctx, WatchtowerSessionWrite{
			SessionName:   name,
			Attached:      1,
			Windows:       1,
			Panes:         1,
			ActivityAt:    now,
			LastPreview:   name + "-preview",
			LastPreviewAt: now,
			Rev:           1,
		}); err != nil {
			t.Fatalf("UpsertWatchtowerSession(%s): %v", name, err)
		}
		if err := s.UpsertWatchtowerWindow(ctx, WatchtowerWindowWrite{
			SessionName:      name,
			WindowIndex:      0,
			Name:             "w0",
			Active:           true,
			Layout:           "layout",
			WindowActivityAt: now,
			Rev:              1,
		}); err != nil {
			t.Fatalf("UpsertWatchtowerWindow(%s): %v", name, err)
		}
		if err := s.UpsertWatchtowerPane(ctx, WatchtowerPaneWrite{
			PaneID:         "%" + name,
			SessionName:    name,
			WindowIndex:    0,
			PaneIndex:      0,
			Title:          "pane",
			Active:         true,
			TTY:            "/dev/null",
			TailHash:       "h",
			TailPreview:    "p",
			TailCapturedAt: now,
			Revision:       1,
			SeenRevision:   0,
			ChangedAt:      now,
		}); err != nil {
			t.Fatalf("UpsertWatchtowerPane(%s): %v", name, err)
		}
	}

	seedSession("a")
	seedSession("b")
	seedSession("c")

	if err := s.PurgeWatchtowerSessions(ctx, []string{"a", "c"}); err != nil {
		t.Fatalf("PurgeWatchtowerSessions([a,c]): %v", err)
	}

	sessions, err := s.ListWatchtowerSessions(ctx)
	if err != nil {
		t.Fatalf("ListWatchtowerSessions after purge: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions len after purge = %d, want 2", len(sessions))
	}
	if sessions[0].SessionName != "a" || sessions[1].SessionName != "c" {
		t.Fatalf("sessions = %+v, want [a c]", sessions)
	}
	if _, err := s.GetWatchtowerSession(ctx, "b"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetWatchtowerSession(b) err = %v, want sql.ErrNoRows", err)
	}

	if err := s.PurgeWatchtowerWindows(ctx, "a", []int{}); err != nil {
		t.Fatalf("PurgeWatchtowerWindows(a, []): %v", err)
	}
	windows, err := s.ListWatchtowerWindows(ctx, "a")
	if err != nil {
		t.Fatalf("ListWatchtowerWindows(a): %v", err)
	}
	if len(windows) != 0 {
		t.Fatalf("windows len for a after purge = %d, want 0", len(windows))
	}

	if err := s.PurgeWatchtowerPanes(ctx, "c", []string{}); err != nil {
		t.Fatalf("PurgeWatchtowerPanes(c, []): %v", err)
	}
	panes, err := s.ListWatchtowerPanes(ctx, "c")
	if err != nil {
		t.Fatalf("ListWatchtowerPanes(c): %v", err)
	}
	if len(panes) != 0 {
		t.Fatalf("panes len for c after purge = %d, want 0", len(panes))
	}
}

func TestWatchtowerSeenScopes(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.UpsertWatchtowerSession(ctx, WatchtowerSessionWrite{
		SessionName:   "dev",
		Attached:      1,
		Windows:       2,
		Panes:         3,
		ActivityAt:    now,
		LastPreview:   "seed",
		LastPreviewAt: now,
		Rev:           1,
	}); err != nil {
		t.Fatalf("UpsertWatchtowerSession: %v", err)
	}

	for _, w := range []WatchtowerWindowWrite{
		{SessionName: "dev", WindowIndex: 0, Name: "w0", Active: true, Layout: "l0", WindowActivityAt: now, Rev: 1},
		{SessionName: "dev", WindowIndex: 1, Name: "w1", Active: false, Layout: "l1", WindowActivityAt: now, Rev: 1},
	} {
		if err := s.UpsertWatchtowerWindow(ctx, w); err != nil {
			t.Fatalf("UpsertWatchtowerWindow(%d): %v", w.WindowIndex, err)
		}
	}

	for _, p := range []WatchtowerPaneWrite{
		{PaneID: "%1", SessionName: "dev", WindowIndex: 0, PaneIndex: 0, Active: true, Revision: 3, SeenRevision: 1, ChangedAt: now},
		{PaneID: "%2", SessionName: "dev", WindowIndex: 0, PaneIndex: 1, Active: false, Revision: 2, SeenRevision: 2, ChangedAt: now},
		{PaneID: "%3", SessionName: "dev", WindowIndex: 1, PaneIndex: 0, Active: false, Revision: 7, SeenRevision: 0, ChangedAt: now},
	} {
		if err := s.UpsertWatchtowerPane(ctx, p); err != nil {
			t.Fatalf("UpsertWatchtowerPane(%s): %v", p.PaneID, err)
		}
	}

	changed, err := s.MarkWatchtowerPaneSeen(ctx, "dev", "%1")
	if err != nil {
		t.Fatalf("MarkWatchtowerPaneSeen: %v", err)
	}
	if !changed {
		t.Fatalf("MarkWatchtowerPaneSeen changed = false, want true")
	}
	panes, err := s.ListWatchtowerPanes(ctx, "dev")
	if err != nil {
		t.Fatalf("ListWatchtowerPanes(dev): %v", err)
	}
	paneByID := map[string]WatchtowerPane{}
	for _, p := range panes {
		paneByID[p.PaneID] = p
	}
	if paneByID["%1"].SeenRevision != paneByID["%1"].Revision {
		t.Fatalf("pane %%1 seen/rev mismatch after pane seen: %+v", paneByID["%1"])
	}

	changed, err = s.MarkWatchtowerWindowSeen(ctx, "dev", 1)
	if err != nil {
		t.Fatalf("MarkWatchtowerWindowSeen: %v", err)
	}
	if !changed {
		t.Fatalf("MarkWatchtowerWindowSeen changed = false, want true")
	}
	panes, err = s.ListWatchtowerPanes(ctx, "dev")
	if err != nil {
		t.Fatalf("ListWatchtowerPanes(dev) #2: %v", err)
	}
	paneByID = map[string]WatchtowerPane{}
	for _, p := range panes {
		paneByID[p.PaneID] = p
	}
	if paneByID["%3"].SeenRevision != paneByID["%3"].Revision {
		t.Fatalf("pane %%3 seen/rev mismatch after window seen: %+v", paneByID["%3"])
	}

	changed, err = s.MarkWatchtowerSessionSeen(ctx, "dev")
	if err != nil {
		t.Fatalf("MarkWatchtowerSessionSeen: %v", err)
	}
	if changed {
		t.Fatalf("MarkWatchtowerSessionSeen changed = true, want false (already seen)")
	}

	session, err := s.GetWatchtowerSession(ctx, "dev")
	if err != nil {
		t.Fatalf("GetWatchtowerSession(dev): %v", err)
	}
	if session.UnreadPanes != 0 || session.UnreadWindows != 0 {
		t.Fatalf("session unread counters should be zero: %+v", session)
	}

	windows, err := s.ListWatchtowerWindows(ctx, "dev")
	if err != nil {
		t.Fatalf("ListWatchtowerWindows(dev): %v", err)
	}
	for _, w := range windows {
		if w.HasUnread || w.UnreadPanes != 0 {
			t.Fatalf("window should be read after seen ops: %+v", w)
		}
	}
}
