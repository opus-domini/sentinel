package tmuxlifecycle

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

type resolution int

const (
	resolutionSame resolution = iota
	resolutionMissing
	resolutionMismatch
)

func (m *Manager) run() {
	ticker := time.NewTicker(m.opts.SweepInterval)
	defer func() {
		ticker.Stop()
		close(m.done)
	}()
	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), m.opts.SweepInterval)
			if err := m.Sweep(ctx); err != nil {
				slog.Warn("tmux lifecycle sweep failed", "err", err)
			}
			cancel()
		}
	}
}

// Reconcile restores persisted leases without performing immediate cleanup. It
// rebuilds byLease/byTarget from scratch, discarding the inFlight and
// cleanupClaimed guards of every entry, so it is a boot step owned by Start and
// a seam for reconciliation tests — never a mutator to re-enter on a running
// manager, which would leave in-flight operations unprotected against cleanup.
func (m *Manager) Reconcile(ctx context.Context) error {
	leases, err := m.store.ListTmuxSessionLeases(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.byLease = make(map[string]*leaseEntry, len(leases))
	m.byTarget = make(map[string]*leaseEntry, len(leases))
	for i := range leases {
		entry := &leaseEntry{lease: leases[i]}
		m.addEntryLocked(entry)
	}
	m.mu.Unlock()

	for i := range leases {
		m.mu.Lock()
		entry := m.byLease[leases[i].LeaseID]
		if entry == nil {
			m.mu.Unlock()
			continue
		}
		lease := entry.lease
		gen := entry.gen
		m.mu.Unlock()
		now := m.now()
		deadlineFuture := lease.State == store.TmuxSessionLeaseActive && lease.ExpiresAt.After(now)
		graceFuture := lease.State == store.TmuxSessionLeaseGrace && lease.GraceUntil.After(now)
		if !deadlineFuture && !graceFuture {
			lease.State = store.TmuxSessionLeaseGrace
			lease.GraceUntil = now.Add(m.opts.GracePeriod)
			lease.UpdatedAt = now
			if err := m.store.UpdateTmuxSessionLeaseState(
				ctx, lease.LeaseID, lease.State, lease.ExpiresAt, lease.GraceUntil, lease.UpdatedAt,
			); err != nil {
				slog.Warn("tmux lifecycle recovery grace persistence deferred", "lease_id", lease.LeaseID, "err", err)
			}
			m.mu.Lock()
			lease = m.applyLeaseLocked(entry, lease, gen)
			m.mu.Unlock()
			m.publish(lease)
		}
		status, err := m.resolveRuntime(ctx, entry, lease)
		if err != nil {
			slog.Warn("tmux lifecycle boot reconciliation deferred", "lease_id", lease.LeaseID, "err", err)
			continue
		}
		switch status {
		case resolutionSame:
		case resolutionMissing:
			if err := m.forgetResolved(ctx, entry, true); err != nil {
				slog.Warn("tmux lifecycle stale lease cleanup deferred", "lease_id", lease.LeaseID, "err", err)
			}
			continue
		case resolutionMismatch:
			if err := m.forgetResolved(ctx, entry, false); err != nil {
				slog.Warn("tmux lifecycle mismatched lease cleanup deferred", "lease_id", lease.LeaseID, "err", err)
			}
			continue
		}
	}
	return nil
}

// Sweep advances expired leases and performs safe, exact-ID cleanup.
func (m *Manager) Sweep(ctx context.Context) error {
	m.mu.Lock()
	entries := make([]*leaseEntry, 0, len(m.byLease))
	for _, entry := range m.byLease {
		entries = append(entries, entry)
	}
	m.mu.Unlock()
	var result error
	for _, entry := range entries {
		if err := m.sweepEntry(ctx, entry); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}

func (m *Manager) sweepEntry(ctx context.Context, entry *leaseEntry) error {
	m.mu.Lock()
	if m.byLease[entry.lease.LeaseID] != entry || entry.cleanupClaimed {
		m.mu.Unlock()
		return nil
	}
	lease := entry.lease
	dirtyTouch := entry.dirtyTouch
	m.mu.Unlock()
	if dirtyTouch {
		if err := m.persistTouch(ctx, entry, lease); err != nil {
			return err
		}
	}

	status, err := m.resolveRuntime(ctx, entry, lease)
	if err != nil {
		return err
	}
	if status == resolutionMissing {
		return m.forgetResolved(ctx, entry, true)
	}
	if status == resolutionMismatch {
		return m.forgetResolved(ctx, entry, false)
	}

	now := m.now()
	m.mu.Lock()
	if m.byLease[entry.lease.LeaseID] != entry || entry.cleanupClaimed {
		m.mu.Unlock()
		return nil
	}
	lease = entry.lease
	gen := entry.gen
	if lease.State == store.TmuxSessionLeaseActive {
		if lease.ExpiresAt.After(now) {
			m.mu.Unlock()
			return nil
		}
		lease.State = store.TmuxSessionLeaseGrace
		lease.GraceUntil = now.Add(m.opts.GracePeriod)
		lease.UpdatedAt = now
		m.mu.Unlock()
		if err := m.store.UpdateTmuxSessionLeaseState(
			ctx, lease.LeaseID, lease.State, lease.ExpiresAt, lease.GraceUntil, lease.UpdatedAt,
		); err != nil {
			return err
		}
		m.mu.Lock()
		applied := m.applyLeaseLocked(entry, lease, gen)
		m.mu.Unlock()
		m.publish(applied)
		return nil
	}
	if lease.State == store.TmuxSessionLeaseGrace && lease.GraceUntil.After(now) {
		m.mu.Unlock()
		return nil
	}
	if entry.inFlight > 0 {
		m.mu.Unlock()
		return nil
	}
	entry.cleanupClaimed = true
	m.mu.Unlock()

	m.opts.DetachSession(lease.User, lease.SessionName)
	status, current, err := m.resolveRuntimeForCleanup(ctx, lease)
	if err != nil {
		m.releaseClaim(entry)
		return err
	}
	if status == resolutionMissing {
		return m.forgetClaimed(ctx, entry, lease, true)
	}
	if status == resolutionMismatch {
		return m.forgetClaimed(ctx, entry, lease, false)
	}
	if current.Attached > 0 {
		lease.State = store.TmuxSessionLeaseCleanupBlocked
		lease.UpdatedAt = now
		if lease.GraceUntil.IsZero() {
			lease.GraceUntil = now
		}
		if err := m.store.UpdateTmuxSessionLeaseState(
			ctx, lease.LeaseID, lease.State, lease.ExpiresAt, lease.GraceUntil, lease.UpdatedAt,
		); err != nil {
			m.releaseClaim(entry)
			return err
		}
		// The cleanup claim excludes every other mutator, so only the entry
		// still being registered has to be re-checked here.
		m.mu.Lock()
		if m.byLease[lease.LeaseID] == entry {
			entry.lease = lease
			entry.gen++
			entry.cleanupClaimed = false
		}
		m.mu.Unlock()
		m.publish(lease)
		return nil
	}
	runtime := m.opts.RuntimeForUser(lease.User)
	if err := runtime.KillSessionByID(ctx, lease.SessionID); err != nil {
		m.releaseClaim(entry)
		return err
	}
	return m.forgetClaimed(ctx, entry, lease, true)
}

func (m *Manager) resolveRuntime(ctx context.Context, entry *leaseEntry, lease Lease) (resolution, error) {
	status, current, err := m.resolveRuntimeForCleanup(ctx, lease)
	if err != nil || status != resolutionSame {
		return status, err
	}
	if current.Name != lease.SessionName {
		now := m.now()
		if err := m.store.RenameTmuxSessionLease(ctx, lease.LeaseID, current.Name, now); err != nil {
			return resolutionSame, err
		}
		m.mu.Lock()
		var updated Lease
		if m.byLease[lease.LeaseID] == entry && !entry.cleanupClaimed {
			entry.lease.SessionName = current.Name
			entry.lease.UpdatedAt = now
			entry.gen++
			updated = entry.lease
		}
		m.mu.Unlock()
		if updated.LeaseID != "" {
			m.publish(updated)
		}
	}
	return resolutionSame, nil
}

func (m *Manager) resolveRuntimeForCleanup(
	ctx context.Context,
	lease Lease,
) (resolution, tmux.Session, error) {
	runtime := m.opts.RuntimeForUser(lease.User)
	current, err := runtime.GetSession(ctx, lease.SessionName)
	if err == nil && current.ID == lease.SessionID {
		return resolutionSame, current, nil
	}
	// A dead tmux server is proof of absence, exactly like a missing session:
	// api and watchtower already read the kind that way. Deferral is reserved
	// for genuinely ambiguous failures (command failed, permission denied),
	// otherwise every ephemeral lease survives a kill-server or a reboot
	// forever and the sweep warns once a minute for the life of the process.
	if err != nil && !isAbsenceKind(err) {
		return resolutionMissing, tmux.Session{}, err
	}
	sessions, listErr := runtime.ListSessions(ctx)
	if listErr != nil {
		return resolutionMissing, tmux.Session{}, listErr
	}
	for _, session := range sessions {
		if session.ID == lease.SessionID {
			return resolutionSame, session, nil
		}
	}
	if err == nil {
		return resolutionMismatch, current, nil
	}
	return resolutionMissing, tmux.Session{}, nil
}

// isAbsenceKind reports whether a tmux lookup error proves the session is gone
// rather than leaving its fate unknown.
func isAbsenceKind(err error) bool {
	return tmux.IsKind(err, tmux.ErrKindSessionNotFound) ||
		tmux.IsKind(err, tmux.ErrKindServerNotRunning)
}

func (m *Manager) forgetResolved(ctx context.Context, entry *leaseEntry, cleanupRuntime bool) error {
	m.mu.Lock()
	if m.byLease[entry.lease.LeaseID] != entry || entry.inFlight > 0 || entry.cleanupClaimed {
		m.mu.Unlock()
		return nil
	}
	entry.cleanupClaimed = true
	lease := entry.lease
	m.mu.Unlock()
	return m.forgetClaimed(ctx, entry, lease, cleanupRuntime)
}

func (m *Manager) forgetClaimed(ctx context.Context, entry *leaseEntry, lease Lease, cleanupRuntime bool) error {
	if cleanupRuntime {
		if err := m.store.DeleteTmuxSessionRuntimeState(ctx, lease.SessionName); err != nil {
			m.releaseClaim(entry)
			return err
		}
	}
	if err := m.store.DeleteTmuxSessionLease(ctx, lease.LeaseID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		m.releaseClaim(entry)
		return err
	}
	m.mu.Lock()
	m.removeEntryLocked(entry)
	m.mu.Unlock()
	m.publish(lease)
	return nil
}
