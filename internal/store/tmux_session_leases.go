package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

const (
	// TmuxSessionLeaseSourceMCP identifies leases created through MCP.
	TmuxSessionLeaseSourceMCP = "mcp"
	// TmuxSessionLeaseActive identifies a renewable lease before expiry.
	TmuxSessionLeaseActive = "active"
	// TmuxSessionLeaseGrace identifies an expired lease in its safety grace.
	TmuxSessionLeaseGrace = "grace"
	// TmuxSessionLeaseCleanupBlocked identifies a lease protected by an attached client.
	TmuxSessionLeaseCleanupBlocked = "cleanup_blocked"
)

// TmuxSessionLease is the persisted lifecycle owner for one ephemeral tmux runtime.
type TmuxSessionLease struct {
	LeaseID       string
	SessionID     string
	SessionName   string
	User          string
	Source        string
	State         string
	CreatedAt     time.Time
	LastRenewedAt time.Time
	ExpiresAt     time.Time
	GraceUntil    time.Time
	UpdatedAt     time.Time
}

// CreateTmuxSessionLease persists one active ephemeral session lease.
func (s *Store) CreateTmuxSessionLease(ctx context.Context, lease TmuxSessionLease) error {
	lease, err := normalizeTmuxSessionLease(lease)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO tmux_session_leases (
			lease_id, session_id, session_name, user, source, state,
			created_at, last_renewed_at, expires_at, grace_until, updated_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		lease.LeaseID,
		lease.SessionID,
		lease.SessionName,
		lease.User,
		lease.Source,
		lease.State,
		formatStoreValueTime(lease.CreatedAt),
		formatStoreValueTime(lease.LastRenewedAt),
		formatStoreValueTime(lease.ExpiresAt),
		formatStoreValueTime(lease.GraceUntil),
		formatStoreValueTime(lease.UpdatedAt),
	)
	return err
}

// GetTmuxSessionLease returns one lifecycle lease by opaque ID.
func (s *Store) GetTmuxSessionLease(ctx context.Context, leaseID string) (TmuxSessionLease, error) {
	leaseID = strings.TrimSpace(leaseID)
	if leaseID == "" {
		return TmuxSessionLease{}, errors.New("tmux session lease ID is required")
	}
	return scanTmuxSessionLease(s.db.QueryRowContext(ctx,
		`SELECT lease_id, session_id, session_name, user, source, state,
		        created_at, last_renewed_at, expires_at, grace_until, updated_at
		   FROM tmux_session_leases
		  WHERE lease_id = ?`,
		leaseID,
	))
}

// ListTmuxSessionLeases returns every active lifecycle lease.
func (s *Store) ListTmuxSessionLeases(ctx context.Context) ([]TmuxSessionLease, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT lease_id, session_id, session_name, user, source, state,
		        created_at, last_renewed_at, expires_at, grace_until, updated_at
		   FROM tmux_session_leases
		  ORDER BY created_at ASC, lease_id ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	leases := make([]TmuxSessionLease, 0, 8)
	for rows.Next() {
		lease, err := scanTmuxSessionLease(rows)
		if err != nil {
			return nil, err
		}
		leases = append(leases, lease)
	}
	return leases, rows.Err()
}

// UpdateTmuxSessionLeaseState persists a lifecycle state transition.
func (s *Store) UpdateTmuxSessionLeaseState(
	ctx context.Context,
	leaseID, state string,
	expiresAt, graceUntil, updatedAt time.Time,
) error {
	leaseID = strings.TrimSpace(leaseID)
	state = strings.TrimSpace(state)
	if leaseID == "" {
		return errors.New("tmux session lease ID is required")
	}
	if !validTmuxSessionLeaseState(state) {
		return errors.New("invalid tmux session lease state")
	}
	if expiresAt.IsZero() || updatedAt.IsZero() {
		return errors.New("tmux session lease deadlines are required")
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE tmux_session_leases
		    SET state = ?, expires_at = ?, grace_until = ?, updated_at = ?
		  WHERE lease_id = ?`,
		state,
		formatStoreValueTime(expiresAt),
		formatStoreValueTime(graceUntil),
		formatStoreValueTime(updatedAt),
		leaseID,
	)
	return requireAffectedRow(result, err)
}

// TouchTmuxSessionLease renews one lifecycle lease after successful MCP activity.
func (s *Store) TouchTmuxSessionLease(
	ctx context.Context,
	leaseID string,
	lastRenewedAt, expiresAt, updatedAt time.Time,
) error {
	leaseID = strings.TrimSpace(leaseID)
	if leaseID == "" {
		return errors.New("tmux session lease ID is required")
	}
	if lastRenewedAt.IsZero() || expiresAt.IsZero() || updatedAt.IsZero() {
		return errors.New("tmux session lease renewal times are required")
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE tmux_session_leases
		    SET state = ?, last_renewed_at = ?, expires_at = ?,
		        grace_until = '', updated_at = ?
		  WHERE lease_id = ?`,
		TmuxSessionLeaseActive,
		formatStoreValueTime(lastRenewedAt),
		formatStoreValueTime(expiresAt),
		formatStoreValueTime(updatedAt),
		leaseID,
	)
	return requireAffectedRow(result, err)
}

// RenameTmuxSessionLease records the current runtime name for one stable ID.
func (s *Store) RenameTmuxSessionLease(ctx context.Context, leaseID, sessionName string, updatedAt time.Time) error {
	leaseID = strings.TrimSpace(leaseID)
	sessionName = strings.TrimSpace(sessionName)
	if leaseID == "" || sessionName == "" {
		return errors.New("tmux session lease ID and session name are required")
	}
	if updatedAt.IsZero() {
		return errors.New("tmux session lease updated time is required")
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE tmux_session_leases
		    SET session_name = ?, updated_at = ?
		  WHERE lease_id = ?`,
		sessionName,
		formatStoreValueTime(updatedAt),
		leaseID,
	)
	return requireAffectedRow(result, err)
}

// DeleteTmuxSessionLease removes terminal lifecycle state.
func (s *Store) DeleteTmuxSessionLease(ctx context.Context, leaseID string) error {
	leaseID = strings.TrimSpace(leaseID)
	if leaseID == "" {
		return errors.New("tmux session lease ID is required")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM tmux_session_leases WHERE lease_id = ?`, leaseID)
	return requireAffectedRow(result, err)
}

type tmuxSessionLeaseScanner interface {
	Scan(dest ...any) error
}

func scanTmuxSessionLease(scanner tmuxSessionLeaseScanner) (TmuxSessionLease, error) {
	var (
		lease                                                          TmuxSessionLease
		createdAtRaw, renewedAtRaw, expiresAtRaw, graceRaw, updatedRaw string
	)
	err := scanner.Scan(
		&lease.LeaseID,
		&lease.SessionID,
		&lease.SessionName,
		&lease.User,
		&lease.Source,
		&lease.State,
		&createdAtRaw,
		&renewedAtRaw,
		&expiresAtRaw,
		&graceRaw,
		&updatedRaw,
	)
	if err != nil {
		return TmuxSessionLease{}, err
	}
	lease.CreatedAt = parseStoreTime(createdAtRaw)
	lease.LastRenewedAt = parseStoreTime(renewedAtRaw)
	lease.ExpiresAt = parseStoreTime(expiresAtRaw)
	lease.GraceUntil = parseStoreTime(graceRaw)
	lease.UpdatedAt = parseStoreTime(updatedRaw)
	return lease, nil
}

func normalizeTmuxSessionLease(lease TmuxSessionLease) (TmuxSessionLease, error) {
	lease.LeaseID = strings.TrimSpace(lease.LeaseID)
	lease.SessionID = strings.TrimSpace(lease.SessionID)
	lease.SessionName = strings.TrimSpace(lease.SessionName)
	lease.User = strings.TrimSpace(lease.User)
	lease.Source = strings.TrimSpace(lease.Source)
	lease.State = strings.TrimSpace(lease.State)
	if lease.LeaseID == "" || lease.SessionID == "" || lease.SessionName == "" {
		return TmuxSessionLease{}, errors.New("tmux session lease identity is required")
	}
	if lease.Source != TmuxSessionLeaseSourceMCP {
		return TmuxSessionLease{}, errors.New("tmux session lease source must be mcp")
	}
	if !validTmuxSessionLeaseState(lease.State) {
		return TmuxSessionLease{}, errors.New("invalid tmux session lease state")
	}
	if lease.CreatedAt.IsZero() || lease.LastRenewedAt.IsZero() || lease.ExpiresAt.IsZero() || lease.UpdatedAt.IsZero() {
		return TmuxSessionLease{}, errors.New("tmux session lease timestamps are required")
	}
	return lease, nil
}

func validTmuxSessionLeaseState(state string) bool {
	switch state {
	case TmuxSessionLeaseActive, TmuxSessionLeaseGrace, TmuxSessionLeaseCleanupBlocked:
		return true
	default:
		return false
	}
}

func requireAffectedRow(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
