CREATE TABLE IF NOT EXISTS session_directories (
    path       TEXT PRIMARY KEY,
    use_count  INTEGER NOT NULL DEFAULT 1,
    last_used  TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_session_directories_frequency
    ON session_directories (use_count DESC, last_used DESC);
