package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	StorageResourceTimeline      = "timeline"
	StorageResourceActivityLog   = "activity-journal"
	StorageResourceGuardrailLog  = "guardrail-audit"
	StorageResourceRecoveryLog   = "recovery-history"
	StorageResourceAll           = "all"
	storageResourceTimelineLabel = "Timeline events"
	storageResourceActivityLabel = "Activity journal"
	storageResourceGuardrailLbl  = "Guardrail audit"
	storageResourceRecoveryLbl   = "Recovery history"
)

var ErrInvalidStorageResource = errors.New("invalid storage resource")

type StorageResourceStat struct {
	Resource    string `json:"resource"`
	Label       string `json:"label"`
	Rows        int64  `json:"rows"`
	ApproxBytes int64  `json:"approxBytes"`
}

type StorageStats struct {
	DatabaseBytes int64                 `json:"databaseBytes"`
	WALBytes      int64                 `json:"walBytes"`
	SHMBytes      int64                 `json:"shmBytes"`
	TotalBytes    int64                 `json:"totalBytes"`
	Resources     []StorageResourceStat `json:"resources"`
	CollectedAt   time.Time             `json:"collectedAt"`
}

type StorageFlushResult struct {
	Resource    string `json:"resource"`
	RemovedRows int64  `json:"removedRows"`
}

func NormalizeStorageResource(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func IsStorageResource(raw string) bool {
	switch NormalizeStorageResource(raw) {
	case StorageResourceTimeline,
		StorageResourceActivityLog,
		StorageResourceGuardrailLog,
		StorageResourceRecoveryLog,
		StorageResourceAll:
		return true
	default:
		return false
	}
}

func (s *Store) GetStorageStats(ctx context.Context) (StorageStats, error) {
	stats := StorageStats{
		Resources:   make([]StorageResourceStat, 0, 4),
		CollectedAt: time.Now().UTC(),
	}

	dbBytes, err := fileSizeBestEffort(s.dbPath)
	if err != nil {
		return StorageStats{}, err
	}
	walBytes, err := fileSizeBestEffort(s.dbPath + "-wal")
	if err != nil {
		return StorageStats{}, err
	}
	shmBytes, err := fileSizeBestEffort(s.dbPath + "-shm")
	if err != nil {
		return StorageStats{}, err
	}
	stats.DatabaseBytes = dbBytes
	stats.WALBytes = walBytes
	stats.SHMBytes = shmBytes
	stats.TotalBytes = dbBytes + walBytes + shmBytes

	for _, resource := range []string{
		StorageResourceTimeline,
		StorageResourceActivityLog,
		StorageResourceGuardrailLog,
		StorageResourceRecoveryLog,
	} {
		item, err := s.resourceStorageStats(ctx, resource)
		if err != nil {
			return StorageStats{}, err
		}
		stats.Resources = append(stats.Resources, item)
	}

	return stats, nil
}

func (s *Store) FlushStorageResource(ctx context.Context, resource string) ([]StorageFlushResult, error) {
	resource = NormalizeStorageResource(resource)
	if resource == StorageResourceAll {
		results := make([]StorageFlushResult, 0, 4)
		for _, key := range []string{
			StorageResourceTimeline,
			StorageResourceActivityLog,
			StorageResourceGuardrailLog,
			StorageResourceRecoveryLog,
		} {
			item, err := s.flushStorageResourceSingle(ctx, key)
			if err != nil {
				return nil, err
			}
			results = append(results, item)
		}
		_ = s.walCheckpoint(ctx)
		return results, nil
	}

	item, err := s.flushStorageResourceSingle(ctx, resource)
	if err != nil {
		return nil, err
	}
	_ = s.walCheckpoint(ctx)
	return []StorageFlushResult{item}, nil
}

func (s *Store) flushStorageResourceSingle(ctx context.Context, resource string) (StorageFlushResult, error) {
	switch resource {
	case StorageResourceTimeline:
		removed, err := deleteRows(ctx, s.db, "DELETE FROM wt_timeline_events")
		if err != nil {
			return StorageFlushResult{}, err
		}
		return StorageFlushResult{Resource: resource, RemovedRows: removed}, nil
	case StorageResourceActivityLog:
		removed, err := deleteRows(ctx, s.db, "DELETE FROM wt_journal")
		if err != nil {
			return StorageFlushResult{}, err
		}
		return StorageFlushResult{Resource: resource, RemovedRows: removed}, nil
	case StorageResourceGuardrailLog:
		removed, err := deleteRows(ctx, s.db, "DELETE FROM guardrail_audit")
		if err != nil {
			return StorageFlushResult{}, err
		}
		return StorageFlushResult{Resource: resource, RemovedRows: removed}, nil
	case StorageResourceRecoveryLog:
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return StorageFlushResult{}, err
		}
		defer func() { _ = tx.Rollback() }()

		snapshotsRemoved, err := deleteRowsTx(ctx, tx, "DELETE FROM recovery_snapshots")
		if err != nil {
			return StorageFlushResult{}, err
		}
		jobsRemoved, err := deleteRowsTx(ctx, tx, "DELETE FROM recovery_jobs")
		if err != nil {
			return StorageFlushResult{}, err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE recovery_sessions
			    SET latest_snapshot_id = 0,
			        snapshot_hash = '',
			        snapshot_at = '',
			        windows = 0,
			        panes = 0,
			        updated_at = datetime('now')`); err != nil {
			return StorageFlushResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return StorageFlushResult{}, err
		}
		return StorageFlushResult{
			Resource:    resource,
			RemovedRows: snapshotsRemoved + jobsRemoved,
		}, nil
	default:
		return StorageFlushResult{}, ErrInvalidStorageResource
	}
}

func (s *Store) resourceStorageStats(ctx context.Context, resource string) (StorageResourceStat, error) {
	switch resource {
	case StorageResourceTimeline:
		rows, approxBytes, err := queryRowsAndBytes(ctx, s.db, `SELECT
			COUNT(*),
			COALESCE(SUM(
				length(session_name) + length(pane_id) + length(event_type) +
				length(severity) + length(command) + length(cwd) + length(summary) +
				length(details) + length(marker) + length(metadata_json) + length(created_at)
			), 0)
		FROM wt_timeline_events`)
		if err != nil {
			return StorageResourceStat{}, err
		}
		return StorageResourceStat{
			Resource:    resource,
			Label:       storageResourceTimelineLabel,
			Rows:        rows,
			ApproxBytes: approxBytes,
		}, nil
	case StorageResourceActivityLog:
		rows, approxBytes, err := queryRowsAndBytes(ctx, s.db, `SELECT
			COUNT(*),
			COALESCE(SUM(
				length(entity_type) + length(session_name) + length(pane_id) +
				length(change_kind) + length(changed_at)
			), 0)
		FROM wt_journal`)
		if err != nil {
			return StorageResourceStat{}, err
		}
		return StorageResourceStat{
			Resource:    resource,
			Label:       storageResourceActivityLabel,
			Rows:        rows,
			ApproxBytes: approxBytes,
		}, nil
	case StorageResourceGuardrailLog:
		rows, approxBytes, err := queryRowsAndBytes(ctx, s.db, `SELECT
			COUNT(*),
			COALESCE(SUM(
				length(rule_id) + length(decision) + length(action) + length(command) +
				length(session_name) + length(pane_id) + length(reason) +
				length(metadata) + length(created_at)
			), 0)
		FROM guardrail_audit`)
		if err != nil {
			return StorageResourceStat{}, err
		}
		return StorageResourceStat{
			Resource:    resource,
			Label:       storageResourceGuardrailLbl,
			Rows:        rows,
			ApproxBytes: approxBytes,
		}, nil
	case StorageResourceRecoveryLog:
		snapRows, snapBytes, err := queryRowsAndBytes(ctx, s.db, `SELECT
			COUNT(*),
			COALESCE(SUM(
				length(session_name) + length(boot_id) + length(state_hash) +
				length(captured_at) + length(active_pane_id) + length(payload_json)
			), 0)
		FROM recovery_snapshots`)
		if err != nil {
			return StorageResourceStat{}, err
		}
		jobRows, jobBytes, err := queryRowsAndBytes(ctx, s.db, `SELECT
			COUNT(*),
			COALESCE(SUM(
				length(id) + length(session_name) + length(target_session) + length(mode) +
				length(conflict_policy) + length(status) + length(current_step) +
				length(error) + length(created_at) + length(started_at) + length(finished_at)
			), 0)
		FROM recovery_jobs`)
		if err != nil {
			return StorageResourceStat{}, err
		}
		return StorageResourceStat{
			Resource:    resource,
			Label:       storageResourceRecoveryLbl,
			Rows:        snapRows + jobRows,
			ApproxBytes: snapBytes + jobBytes,
		}, nil
	default:
		return StorageResourceStat{}, ErrInvalidStorageResource
	}
}

func queryRowsAndBytes(ctx context.Context, db *sql.DB, query string) (int64, int64, error) {
	var rows, approxBytes int64
	if err := db.QueryRowContext(ctx, query).Scan(&rows, &approxBytes); err != nil {
		return 0, 0, err
	}
	if rows < 0 {
		rows = 0
	}
	if approxBytes < 0 {
		approxBytes = 0
	}
	return rows, approxBytes, nil
}

func deleteRows(ctx context.Context, db *sql.DB, query string, args ...any) (int64, error) {
	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func deleteRowsTx(ctx context.Context, tx *sql.Tx, query string, args ...any) (int64, error) {
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) walCheckpoint(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return err
	}
	return nil
}

func fileSizeBestEffort(path string) (int64, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return 0, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("stat %s: %w", path, err)
	}
	return info.Size(), nil
}
