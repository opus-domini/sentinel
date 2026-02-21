-- 000001_init.sql: Complete Sentinel schema as of v0.4.5.
-- All statements use IF NOT EXISTS / OR IGNORE so the migration is safe
-- for both fresh databases and existing databases upgrading to the migrator.

-- ============================================================
-- sessions
-- ============================================================

CREATE TABLE IF NOT EXISTS sessions (
    name            TEXT PRIMARY KEY,
    hash            TEXT NOT NULL,
    last_content    TEXT DEFAULT '',
    icon            TEXT DEFAULT '',
    next_window_seq INTEGER NOT NULL DEFAULT 1,
    updated_at      TEXT DEFAULT (datetime('now'))
);

-- ============================================================
-- recovery
-- ============================================================

CREATE TABLE IF NOT EXISTS recovery_sessions (
    name                TEXT PRIMARY KEY,
    state               TEXT NOT NULL DEFAULT 'running',
    latest_snapshot_id  INTEGER NOT NULL DEFAULT 0,
    snapshot_hash       TEXT NOT NULL DEFAULT '',
    snapshot_at         TEXT NOT NULL DEFAULT '',
    last_boot_id        TEXT NOT NULL DEFAULT '',
    last_seen_at        TEXT NOT NULL DEFAULT '',
    killed_at           TEXT NOT NULL DEFAULT '',
    restored_at         TEXT NOT NULL DEFAULT '',
    archived_at         TEXT NOT NULL DEFAULT '',
    restore_error       TEXT NOT NULL DEFAULT '',
    windows             INTEGER NOT NULL DEFAULT 0,
    panes               INTEGER NOT NULL DEFAULT 0,
    updated_at          TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS recovery_snapshots (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    session_name  TEXT NOT NULL,
    boot_id       TEXT NOT NULL,
    state_hash    TEXT NOT NULL,
    captured_at   TEXT NOT NULL,
    active_window INTEGER NOT NULL DEFAULT 0,
    active_pane_id TEXT NOT NULL DEFAULT '',
    windows       INTEGER NOT NULL DEFAULT 0,
    panes         INTEGER NOT NULL DEFAULT 0,
    payload_json  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_recovery_snapshots_session_captured_at
    ON recovery_snapshots (session_name, captured_at DESC);

CREATE TABLE IF NOT EXISTS recovery_jobs (
    id               TEXT PRIMARY KEY,
    session_name     TEXT NOT NULL,
    target_session   TEXT NOT NULL DEFAULT '',
    snapshot_id      INTEGER NOT NULL,
    mode             TEXT NOT NULL,
    conflict_policy  TEXT NOT NULL,
    status           TEXT NOT NULL,
    total_steps      INTEGER NOT NULL DEFAULT 0,
    completed_steps  INTEGER NOT NULL DEFAULT 0,
    current_step     TEXT NOT NULL DEFAULT '',
    error            TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL,
    started_at       TEXT NOT NULL DEFAULT '',
    finished_at      TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_recovery_jobs_created_at
    ON recovery_jobs (created_at DESC);

CREATE TABLE IF NOT EXISTS runtime_kv (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- watchtower
-- ============================================================

CREATE TABLE IF NOT EXISTS wt_sessions (
    session_name         TEXT PRIMARY KEY,
    attached             INTEGER NOT NULL DEFAULT 0,
    windows              INTEGER NOT NULL DEFAULT 0,
    panes                INTEGER NOT NULL DEFAULT 0,
    activity_at          TEXT NOT NULL DEFAULT '',
    last_preview         TEXT NOT NULL DEFAULT '',
    last_preview_at      TEXT NOT NULL DEFAULT '',
    last_preview_pane_id TEXT NOT NULL DEFAULT '',
    unread_windows       INTEGER NOT NULL DEFAULT 0,
    unread_panes         INTEGER NOT NULL DEFAULT 0,
    rev                  INTEGER NOT NULL DEFAULT 0,
    updated_at           TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS wt_windows (
    session_name       TEXT NOT NULL,
    window_index       INTEGER NOT NULL,
    name               TEXT NOT NULL DEFAULT '',
    active             INTEGER NOT NULL DEFAULT 0,
    layout             TEXT NOT NULL DEFAULT '',
    window_activity_at TEXT NOT NULL DEFAULT '',
    unread_panes       INTEGER NOT NULL DEFAULT 0,
    has_unread         INTEGER NOT NULL DEFAULT 0,
    rev                INTEGER NOT NULL DEFAULT 0,
    updated_at         TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (session_name, window_index)
);

CREATE INDEX IF NOT EXISTS idx_wt_windows_session_activity
    ON wt_windows (session_name, window_activity_at DESC);

CREATE TABLE IF NOT EXISTS wt_panes (
    pane_id          TEXT PRIMARY KEY,
    session_name     TEXT NOT NULL,
    window_index     INTEGER NOT NULL,
    pane_index       INTEGER NOT NULL,
    title            TEXT NOT NULL DEFAULT '',
    active           INTEGER NOT NULL DEFAULT 0,
    tty              TEXT NOT NULL DEFAULT '',
    current_path     TEXT NOT NULL DEFAULT '',
    start_command    TEXT NOT NULL DEFAULT '',
    current_command  TEXT NOT NULL DEFAULT '',
    tail_hash        TEXT NOT NULL DEFAULT '',
    tail_preview     TEXT NOT NULL DEFAULT '',
    tail_captured_at TEXT NOT NULL DEFAULT '',
    revision         INTEGER NOT NULL DEFAULT 0,
    seen_revision    INTEGER NOT NULL DEFAULT 0,
    changed_at       TEXT NOT NULL DEFAULT '',
    updated_at       TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_wt_panes_session_window
    ON wt_panes (session_name, window_index, pane_index);

CREATE INDEX IF NOT EXISTS idx_wt_panes_unread
    ON wt_panes (session_name, revision, seen_revision);

CREATE TABLE IF NOT EXISTS wt_presence (
    terminal_id   TEXT PRIMARY KEY,
    session_name  TEXT NOT NULL DEFAULT '',
    window_index  INTEGER NOT NULL DEFAULT -1,
    pane_id       TEXT NOT NULL DEFAULT '',
    visible       INTEGER NOT NULL DEFAULT 0,
    focused       INTEGER NOT NULL DEFAULT 0,
    updated_at    TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at    TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_wt_presence_expires_at
    ON wt_presence (expires_at);

CREATE TABLE IF NOT EXISTS wt_journal (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    global_rev   INTEGER NOT NULL,
    entity_type  TEXT NOT NULL,
    session_name TEXT NOT NULL DEFAULT '',
    window_index INTEGER NOT NULL DEFAULT -1,
    pane_id      TEXT NOT NULL DEFAULT '',
    change_kind  TEXT NOT NULL DEFAULT '',
    changed_at   TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_wt_journal_global_rev
    ON wt_journal (global_rev ASC);

CREATE TABLE IF NOT EXISTS wt_runtime (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS wt_timeline_events (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    session_name  TEXT NOT NULL DEFAULT '',
    window_index  INTEGER NOT NULL DEFAULT -1,
    pane_id       TEXT NOT NULL DEFAULT '',
    event_type    TEXT NOT NULL DEFAULT '',
    severity      TEXT NOT NULL DEFAULT '',
    command       TEXT NOT NULL DEFAULT '',
    cwd           TEXT NOT NULL DEFAULT '',
    duration_ms   INTEGER NOT NULL DEFAULT 0,
    summary       TEXT NOT NULL DEFAULT '',
    details       TEXT NOT NULL DEFAULT '',
    marker        TEXT NOT NULL DEFAULT '',
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at    TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_wt_timeline_created
    ON wt_timeline_events (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_wt_timeline_session
    ON wt_timeline_events (session_name, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_wt_timeline_scope
    ON wt_timeline_events (session_name, window_index, pane_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_wt_timeline_event_type
    ON wt_timeline_events (event_type, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS wt_pane_runtime (
    pane_id          TEXT PRIMARY KEY,
    session_name     TEXT NOT NULL DEFAULT '',
    window_index     INTEGER NOT NULL DEFAULT -1,
    current_command  TEXT NOT NULL DEFAULT '',
    started_at       TEXT NOT NULL DEFAULT '',
    updated_at       TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_wt_pane_runtime_session
    ON wt_pane_runtime (session_name, pane_id);

-- ============================================================
-- guardrails
-- ============================================================

CREATE TABLE IF NOT EXISTS guardrail_rules (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL DEFAULT '',
    scope      TEXT NOT NULL DEFAULT 'action',
    pattern    TEXT NOT NULL DEFAULT '',
    mode       TEXT NOT NULL DEFAULT 'warn',
    severity   TEXT NOT NULL DEFAULT 'warn',
    message    TEXT NOT NULL DEFAULT '',
    enabled    INTEGER NOT NULL DEFAULT 1,
    priority   INTEGER NOT NULL DEFAULT 100,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS guardrail_audit (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    rule_id      TEXT NOT NULL DEFAULT '',
    decision     TEXT NOT NULL DEFAULT '',
    action       TEXT NOT NULL DEFAULT '',
    command      TEXT NOT NULL DEFAULT '',
    session_name TEXT NOT NULL DEFAULT '',
    window_index INTEGER NOT NULL DEFAULT -1,
    pane_id      TEXT NOT NULL DEFAULT '',
    override     INTEGER NOT NULL DEFAULT 0,
    reason       TEXT NOT NULL DEFAULT '',
    metadata     TEXT NOT NULL DEFAULT '{}',
    created_at   TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_guardrail_rules_priority
    ON guardrail_rules (priority ASC, id ASC);

CREATE INDEX IF NOT EXISTS idx_guardrail_audit_created
    ON guardrail_audit (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_guardrail_audit_action
    ON guardrail_audit (action, created_at DESC, id DESC);

-- ============================================================
-- ops: activity timeline
-- ============================================================

CREATE TABLE IF NOT EXISTS ops_timeline_events (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    source       TEXT NOT NULL,
    event_type   TEXT NOT NULL,
    severity     TEXT NOT NULL,
    resource     TEXT NOT NULL,
    message      TEXT NOT NULL,
    details      TEXT NOT NULL DEFAULT '',
    metadata     TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_ops_timeline_created
    ON ops_timeline_events (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_ops_timeline_severity
    ON ops_timeline_events (severity, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_ops_timeline_source
    ON ops_timeline_events (source, created_at DESC, id DESC);

-- ============================================================
-- ops: alerts
-- ============================================================

CREATE TABLE IF NOT EXISTS ops_alerts (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    dedupe_key    TEXT NOT NULL UNIQUE,
    source        TEXT NOT NULL,
    resource      TEXT NOT NULL,
    title         TEXT NOT NULL,
    message       TEXT NOT NULL,
    severity      TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'open',
    occurrences   INTEGER NOT NULL DEFAULT 1,
    metadata      TEXT NOT NULL DEFAULT '',
    first_seen_at TEXT NOT NULL,
    last_seen_at  TEXT NOT NULL,
    acked_at      TEXT DEFAULT '',
    resolved_at   TEXT DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_ops_alerts_status
    ON ops_alerts (status, last_seen_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_ops_alerts_last_seen
    ON ops_alerts (last_seen_at DESC, id DESC);

-- ============================================================
-- ops: runbooks
-- ============================================================

CREATE TABLE IF NOT EXISTS ops_runbooks (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    steps_json  TEXT NOT NULL DEFAULT '[]',
    enabled     INTEGER NOT NULL DEFAULT 1,
    webhook_url TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS ops_runbook_runs (
    id              TEXT PRIMARY KEY,
    runbook_id      TEXT NOT NULL,
    runbook_name    TEXT NOT NULL,
    status          TEXT NOT NULL,
    total_steps     INTEGER NOT NULL DEFAULT 0,
    completed_steps INTEGER NOT NULL DEFAULT 0,
    current_step    TEXT NOT NULL DEFAULT '',
    error           TEXT NOT NULL DEFAULT '',
    step_results    TEXT NOT NULL DEFAULT '[]',
    created_at      TEXT NOT NULL,
    started_at      TEXT NOT NULL DEFAULT '',
    finished_at     TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_ops_runbook_runs_created
    ON ops_runbook_runs (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_ops_runbook_runs_status
    ON ops_runbook_runs (status, created_at DESC, id DESC);

-- ============================================================
-- ops: custom services
-- ============================================================

CREATE TABLE IF NOT EXISTS ops_custom_services (
    name         TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    manager      TEXT NOT NULL DEFAULT 'systemd',
    unit         TEXT NOT NULL,
    scope        TEXT NOT NULL DEFAULT 'user',
    enabled      INTEGER NOT NULL DEFAULT 1,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- ops: scheduler
-- ============================================================

CREATE TABLE IF NOT EXISTS ops_schedules (
    id              TEXT PRIMARY KEY,
    runbook_id      TEXT NOT NULL,
    name            TEXT NOT NULL DEFAULT '',
    schedule_type   TEXT NOT NULL,
    cron_expr       TEXT NOT NULL DEFAULT '',
    timezone        TEXT NOT NULL DEFAULT 'UTC',
    run_at          TEXT NOT NULL DEFAULT '',
    enabled         INTEGER NOT NULL DEFAULT 1,
    last_run_at     TEXT NOT NULL DEFAULT '',
    last_run_status TEXT NOT NULL DEFAULT '',
    next_run_at     TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_ops_schedules_next_run
    ON ops_schedules (enabled, next_run_at ASC);

CREATE INDEX IF NOT EXISTS idx_ops_schedules_runbook
    ON ops_schedules (runbook_id);

-- ============================================================
-- seed data
-- ============================================================

-- Watchtower global revision counter.
INSERT OR IGNORE INTO wt_runtime(key, value) VALUES ('global_rev', '0');

-- Copy any pre-existing sessions into watchtower (no-op on fresh DB).
INSERT OR IGNORE INTO wt_sessions(session_name, last_preview, updated_at)
SELECT name, COALESCE(last_content, ''), datetime('now')
FROM sessions;

-- Default guardrail rules.
INSERT OR IGNORE INTO guardrail_rules(
    id, name, scope, pattern, mode, severity, message, enabled, priority
) VALUES (
    'action.session.kill.confirm',
    'Confirm session kill',
    'action',
    '^session\.kill$',
    'confirm',
    'warn',
    'Session termination requires explicit confirmation.',
    1,
    10
);

INSERT OR IGNORE INTO guardrail_rules(
    id, name, scope, pattern, mode, severity, message, enabled, priority
) VALUES (
    'action.pane.kill.warn',
    'Warn on pane kill',
    'action',
    '^pane\.kill$',
    'warn',
    'warn',
    'Pane termination logged for audit.',
    1,
    20
);

-- Normalize any legacy command-scope rules to action scope.
UPDATE guardrail_rules SET scope = 'action' WHERE scope = 'command';

-- Default runbooks.
INSERT OR IGNORE INTO ops_runbooks(
    id, name, description, steps_json, enabled, created_at, updated_at
) VALUES (
    'ops.service.recover',
    'Service Recovery',
    'Validate and recover the Sentinel service runtime.',
    '[{"type":"command","title":"Inspect service status","command":"sentinel service status"},{"type":"command","title":"Restart service","command":"sentinel service install --start=true"},{"type":"check","title":"Confirm healthy status","check":"service should be active"}]',
    1,
    datetime('now'),
    datetime('now')
);

INSERT OR IGNORE INTO ops_runbooks(
    id, name, description, steps_json, enabled, created_at, updated_at
) VALUES (
    'ops.autoupdate.verify',
    'Autoupdate Verification',
    'Check updater configuration and latest release state.',
    '[{"type":"command","title":"Check updater timer","command":"sentinel service autoupdate status"},{"type":"command","title":"Check release status","command":"sentinel update check"},{"type":"manual","title":"Review output","description":"Review versions and update policy before apply."}]',
    1,
    datetime('now'),
    datetime('now')
);

INSERT OR IGNORE INTO ops_runbooks(
    id, name, description, steps_json, enabled, created_at, updated_at
) VALUES (
    'ops.update.apply',
    'Apply Update',
    'Check for updates, download and install the latest version, and restart the service.',
    '[{"type":"command","title":"Check for updates","command":"sentinel update check"},{"type":"command","title":"Apply update and restart","command":"sentinel update apply --restart"}]',
    1,
    datetime('now'),
    datetime('now')
);
