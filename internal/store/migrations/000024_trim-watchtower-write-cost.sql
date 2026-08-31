-- ============================================================
-- watchtower: drop two indexes and the journal's AUTOINCREMENT
-- ============================================================
-- Both changes remove writes the collector paid on every change without
-- buying a read.
--
-- idx_wt_windows_session_activity indexed window_activity_at, which appears in
-- no WHERE and no ORDER BY: ListWatchtowerWindows is served by
-- sqlite_autoindex_wt_windows_1 with or without it.
--
-- idx_wt_panes_unread was chosen by exactly one query, the mark-session-seen
-- update. Verified with EXPLAIN QUERY PLAN against a copy of a live database:
-- after the drop that query is served by idx_wt_panes_session_window through
-- the same session_name prefix, with the same SEARCH plan.

DROP INDEX IF EXISTS idx_wt_windows_session_activity;
DROP INDEX IF EXISTS idx_wt_panes_unread;

-- AUTOINCREMENT makes SQLite maintain the sqlite_sequence table on every
-- insert -- an extra page dirtied per journal row -- to guarantee ids are never
-- reused. Nothing reads wt_journal.id: the client paginates on global_rev, and
-- INTEGER PRIMARY KEY still gives a unique monotonic rowid. Recreating the
-- table drops the ring's current contents, which is harmless: the client
-- already does a full refresh whenever the delta reports an overflow.

DROP TABLE wt_journal;

CREATE TABLE wt_journal (
    id           INTEGER PRIMARY KEY,
    global_rev   INTEGER NOT NULL,
    entity_type  TEXT NOT NULL,
    session_name TEXT NOT NULL DEFAULT '',
    window_index INTEGER NOT NULL DEFAULT -1,
    pane_id      TEXT NOT NULL DEFAULT '',
    change_kind  TEXT NOT NULL DEFAULT '',
    changed_at   TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_wt_journal_global_rev
    ON wt_journal (global_rev ASC);
