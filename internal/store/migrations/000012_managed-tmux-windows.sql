CREATE TABLE IF NOT EXISTS managed_tmux_windows (
    id                TEXT PRIMARY KEY,
    session_name      TEXT NOT NULL,
    launcher_id       TEXT NOT NULL DEFAULT '',
    launcher_name     TEXT NOT NULL DEFAULT '',
    icon              TEXT NOT NULL DEFAULT 'terminal',
    command           TEXT NOT NULL DEFAULT '',
    cwd_mode          TEXT NOT NULL DEFAULT 'session',
    cwd_value         TEXT NOT NULL DEFAULT '',
    resolved_cwd      TEXT NOT NULL DEFAULT '',
    window_name       TEXT NOT NULL DEFAULT '',
    tmux_window_id    TEXT NOT NULL DEFAULT '',
    last_window_index INTEGER NOT NULL DEFAULT -1,
    sort_order        INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_managed_tmux_windows_session_sort
    ON managed_tmux_windows (session_name, sort_order ASC, created_at ASC, id ASC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_managed_tmux_windows_runtime
    ON managed_tmux_windows (session_name, tmux_window_id)
    WHERE tmux_window_id != '';
