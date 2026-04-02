ALTER TABLE session_presets ADD COLUMN user TEXT NOT NULL DEFAULT '';
ALTER TABLE tmux_launchers ADD COLUMN user_mode TEXT NOT NULL DEFAULT 'session';
ALTER TABLE tmux_launchers ADD COLUMN user_value TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS session_users (
    session_name TEXT PRIMARY KEY,
    user         TEXT NOT NULL,
    updated_at   TEXT NOT NULL DEFAULT (datetime('now'))
);
