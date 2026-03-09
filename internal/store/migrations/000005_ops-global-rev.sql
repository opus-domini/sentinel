-- 000005_ops-global-rev.sql: Revision tracking for delta-based tickers.
-- A lightweight table that tracks the current revision for ops tables.
-- Tickers compare their last-seen revision against the current value
-- to skip broadcasts when no data has changed.

CREATE TABLE IF NOT EXISTS ops_revisions (
    table_name TEXT PRIMARY KEY,
    rev        INTEGER NOT NULL DEFAULT 0
);

INSERT OR IGNORE INTO ops_revisions(table_name, rev) VALUES ('ops_alerts', 0);
INSERT OR IGNORE INTO ops_revisions(table_name, rev) VALUES ('ops_timeline_events', 0);
