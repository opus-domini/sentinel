-- ============================================================
-- watchtower: delete the runtime keys nothing writes any more
-- ============================================================
-- The collector's per-tick telemetry was removed along with the endpoint that
-- read it, so these six keys are frozen at whatever the last write left. They
-- are not merely stale: collect_total accumulated across restarts and
-- last_collect_at survived one, so they describe a collection no running
-- process performed. Delete them rather than leave rows that read as live.
--
-- global_rev stays: the activity delta protocol depends on it.

DELETE FROM wt_runtime
 WHERE key IN (
   'collect_total',
   'collect_errors_total',
   'last_collect_at',
   'last_collect_duration_ms',
   'last_collect_sessions',
   'last_collect_changed_sessions',
   'last_collect_error'
 );
