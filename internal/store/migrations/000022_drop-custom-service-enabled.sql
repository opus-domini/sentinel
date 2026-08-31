-- ============================================================
-- ops: drop the never-written custom service "enabled" column
-- ============================================================
-- No code path ever wrote anything but 1 into this column, so the
-- "WHERE enabled = 1" filter in ListCustomServices was a permanent no-op that
-- read as real soft-disable state. Registering and deregistering a custom
-- service is an INSERT/DELETE pair; a soft-disable, if ever wanted, should be
-- added deliberately rather than preserved as a stub.

ALTER TABLE ops_custom_services DROP COLUMN enabled;
