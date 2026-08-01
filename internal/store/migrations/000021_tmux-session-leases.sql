CREATE TABLE IF NOT EXISTS tmux_session_leases (
    lease_id        TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL,
    session_name    TEXT NOT NULL,
    user            TEXT NOT NULL DEFAULT '',
    source          TEXT NOT NULL CHECK (source = 'mcp'),
    state           TEXT NOT NULL CHECK (state IN ('active', 'grace', 'cleanup_blocked')),
    created_at      TEXT NOT NULL,
    last_renewed_at TEXT NOT NULL,
    expires_at      TEXT NOT NULL,
    grace_until     TEXT NOT NULL DEFAULT '',
    updated_at      TEXT NOT NULL,
    UNIQUE (user, session_id),
    UNIQUE (user, session_name)
);

CREATE INDEX IF NOT EXISTS idx_tmux_session_leases_deadlines
    ON tmux_session_leases (state, expires_at, grace_until);
