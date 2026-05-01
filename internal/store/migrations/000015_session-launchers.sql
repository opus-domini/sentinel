CREATE TABLE IF NOT EXISTS session_launchers (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL COLLATE NOCASE UNIQUE,
    cwd          TEXT NOT NULL DEFAULT '',
    icon         TEXT NOT NULL DEFAULT 'terminal',
    user         TEXT NOT NULL DEFAULT '',
    sort_order   INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
    last_used_at TEXT NOT NULL DEFAULT '',
    use_count    INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_session_launchers_sort_order
    ON session_launchers (sort_order ASC, name ASC);
