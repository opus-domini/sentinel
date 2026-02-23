ALTER TABLE recovery_jobs ADD COLUMN triggered_by    TEXT NOT NULL DEFAULT 'manual';
ALTER TABLE recovery_jobs ADD COLUMN degraded        INTEGER NOT NULL DEFAULT 0;
ALTER TABLE recovery_jobs ADD COLUMN degraded_reason TEXT NOT NULL DEFAULT '';
