CREATE TABLE IF NOT EXISTS session_presets (
    name             TEXT PRIMARY KEY,
    cwd              TEXT NOT NULL DEFAULT '',
    icon             TEXT NOT NULL DEFAULT 'terminal',
    created_at       TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at       TEXT NOT NULL DEFAULT (datetime('now')),
    last_launched_at TEXT NOT NULL DEFAULT '',
    launch_count     INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_session_presets_name
    ON session_presets (name ASC);
