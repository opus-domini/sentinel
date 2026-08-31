package tmuxlifecycle

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/opus-domini/sentinel/internal/events"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

type leaseEntry struct {
	lease          Lease
	inFlight       int
	cleanupClaimed bool
	dirtyTouch     bool
	// gen advances on every change to lease so a write back decided before an
	// unlocked persistence gap can detect that it lost the race and drop its
	// stale copy instead of clobbering the winner.
	gen uint64
}

// Manager owns all active ephemeral tmux session leases in this process.
type Manager struct {
	store LeaseStore
	opts  Options

	mu       sync.Mutex
	byLease  map[string]*leaseEntry
	byTarget map[string]*leaseEntry
	started  bool
	stop     chan struct{}
	done     chan struct{}
}

// New constructs a stopped lifecycle manager.
func New(leaseStore LeaseStore, opts Options) *Manager {
	if opts.Clock == nil {
		opts.Clock = systemClock{}
	}
	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = DefaultIdleTimeout
	}
	if opts.GracePeriod <= 0 {
		opts.GracePeriod = DefaultGracePeriod
	}
	if opts.SweepInterval <= 0 {
		opts.SweepInterval = DefaultSweepInterval
	}
	if opts.DetachSession == nil {
		opts.DetachSession = func(string, string) {}
	}
	if opts.Publish == nil {
		opts.Publish = func(string, map[string]any) {}
	}
	return &Manager{
		store:    leaseStore,
		opts:     opts,
		byLease:  make(map[string]*leaseEntry),
		byTarget: make(map[string]*leaseEntry),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Start reconciles persisted leases before starting the background sweeper.
func (m *Manager) Start(ctx context.Context) error {
	if m == nil || m.store == nil || m.opts.RuntimeForUser == nil {
		return errors.New("tmux lifecycle manager dependencies are unavailable")
	}
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return errors.New("tmux lifecycle manager already started")
	}
	m.started = true
	m.mu.Unlock()
	if err := m.Reconcile(ctx); err != nil {
		m.mu.Lock()
		m.started = false
		m.mu.Unlock()
		return err
	}
	go m.run()
	return nil
}

// Stop cancels the sweeper and waits for it to exit.
func (m *Manager) Stop(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	started := m.started
	m.mu.Unlock()
	if !started {
		return nil
	}
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}
	select {
	case <-m.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Create creates an ephemeral session and compensates every pre-response failure.
func (m *Manager) Create(
	ctx context.Context,
	user, name, cwd string,
	prepare func(context.Context, tmux.Session) error,
) (Snapshot, error) {
	user = strings.TrimSpace(user)
	name = strings.TrimSpace(name)
	if name == "" {
		return Snapshot{}, errors.New("tmux session name is required")
	}
	runtime := m.opts.RuntimeForUser(user)
	created, err := runtime.CreateSessionWithID(ctx, name, cwd)
	if err != nil {
		return Snapshot{}, err
	}
	if created.ID == "" {
		return Snapshot{}, errors.New("tmux create returned no stable session ID")
	}
	current, err := runtime.GetSession(ctx, name)
	if err != nil || current.ID != created.ID {
		abortErr := m.abortCreated(ctx, runtime, created, "")
		return Snapshot{}, errors.Join(identityError(err), abortErr)
	}

	now := m.now()
	leaseID, err := newLeaseID()
	if err != nil {
		abortErr := m.abortCreated(ctx, runtime, created, "")
		return Snapshot{}, errors.Join(err, abortErr)
	}
	lease := Lease{
		LeaseID:       leaseID,
		SessionID:     created.ID,
		SessionName:   current.Name,
		User:          user,
		Source:        store.TmuxSessionLeaseSourceMCP,
		State:         store.TmuxSessionLeaseActive,
		CreatedAt:     now,
		LastRenewedAt: now,
		ExpiresAt:     now.Add(m.opts.IdleTimeout),
		UpdatedAt:     now,
	}
	if err := m.store.CreateTmuxSessionLease(ctx, lease); err != nil {
		abortErr := m.abortCreated(ctx, runtime, created, "")
		return Snapshot{}, errors.Join(err, abortErr)
	}
	entry := &leaseEntry{lease: lease}
	m.mu.Lock()
	m.addEntryLocked(entry)
	m.mu.Unlock()

	if prepare != nil {
		if err := prepare(ctx, current); err != nil {
			abortErr := m.abort(ctx, leaseID)
			return Snapshot{}, errors.Join(err, abortErr)
		}
	}
	verified, err := runtime.GetSession(ctx, current.Name)
	if err != nil || verified.ID != created.ID {
		abortErr := m.abort(ctx, leaseID)
		return Snapshot{}, errors.Join(identityError(err), abortErr)
	}
	m.publish(lease)
	return snapshot(lease), nil
}

// abort compensates a failed ephemeral create without killing a reused name.
func (m *Manager) abort(ctx context.Context, leaseID string) error {
	entry, err := m.claim(leaseID, "")
	if err != nil {
		return err
	}
	runtime := m.opts.RuntimeForUser(entry.lease.User)
	err = m.abortCreated(ctx, runtime, tmux.Session{
		ID: entry.lease.SessionID, Name: entry.lease.SessionName,
	}, entry.lease.LeaseID)
	if err != nil {
		m.releaseClaim(entry)
	}
	return err
}

// BeginUse protects a targeted MCP operation from concurrent cleanup.
func (m *Manager) BeginUse(ctx context.Context, user, sessionName string) (Use, error) {
	user = strings.TrimSpace(user)
	runtime := m.opts.RuntimeForUser(user)
	session, err := runtime.GetSession(ctx, strings.TrimSpace(sessionName))
	if err != nil {
		return Use{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.byTarget[targetKey(user, session.ID)]
	if entry == nil {
		return Use{}, nil
	}
	if entry.cleanupClaimed {
		return Use{}, ErrCleanupClaimed
	}
	entry.inFlight++
	return Use{LeaseID: entry.lease.LeaseID}, nil
}

// Finish releases an operation guard and renews successful managed activity.
// The renewal is applied in memory before it is persisted and the sweeper
// retries a failed write, so a store failure is logged instead of reported: the
// caller already performed the operation this renewal accounts for.
func (m *Manager) Finish(ctx context.Context, use Use, success bool) error {
	if !use.Managed() {
		return nil
	}
	now := m.now()
	m.mu.Lock()
	entry := m.byLease[use.LeaseID]
	if entry == nil {
		m.mu.Unlock()
		return ErrLeaseNotFound
	}
	if entry.inFlight <= 0 {
		m.mu.Unlock()
		return errors.New("tmux session lifecycle use guard is already finished")
	}
	if !success {
		entry.inFlight--
		m.mu.Unlock()
		return nil
	}
	entry.lease.State = store.TmuxSessionLeaseActive
	entry.lease.LastRenewedAt = now
	entry.lease.ExpiresAt = now.Add(m.opts.IdleTimeout)
	entry.lease.GraceUntil = time.Time{}
	entry.lease.UpdatedAt = now
	entry.dirtyTouch = true
	entry.gen++
	lease := entry.lease
	m.mu.Unlock()

	err := m.persistTouch(ctx, entry, lease)
	m.mu.Lock()
	if current := m.byLease[use.LeaseID]; current == entry && entry.inFlight > 0 {
		entry.inFlight--
	}
	m.mu.Unlock()
	if err != nil {
		slog.Warn("tmux lifecycle renewal persistence deferred", "lease_id", lease.LeaseID, "err", err)
	}
	m.publish(lease)
	return nil
}

// Keep promotes one ephemeral session to persistent by deleting its lease.
func (m *Manager) Keep(ctx context.Context, leaseID, confirmName string) (Snapshot, error) {
	entry, err := m.claim(leaseID, confirmName)
	if err != nil {
		return Snapshot{}, err
	}
	lease := entry.lease
	runtime := m.opts.RuntimeForUser(lease.User)
	current, err := runtime.GetSession(ctx, lease.SessionName)
	if err != nil || current.ID != lease.SessionID {
		m.releaseClaim(entry)
		return Snapshot{}, identityError(err)
	}
	if err := m.store.DeleteTmuxSessionLease(ctx, lease.LeaseID); err != nil {
		m.releaseClaim(entry)
		return Snapshot{}, err
	}
	m.mu.Lock()
	m.removeEntryLocked(entry)
	m.mu.Unlock()
	m.publish(lease)
	return snapshot(lease), nil
}

// Close kills exactly one leased runtime and removes its lifecycle state.
func (m *Manager) Close(ctx context.Context, leaseID, confirmName string) error {
	entry, err := m.claim(leaseID, confirmName)
	if err != nil {
		return err
	}
	lease := entry.lease
	runtime := m.opts.RuntimeForUser(lease.User)
	current, err := runtime.GetSession(ctx, lease.SessionName)
	if err != nil || current.ID != lease.SessionID {
		m.releaseClaim(entry)
		return identityError(err)
	}
	m.opts.DetachSession(lease.User, lease.SessionName)
	current, err = runtime.GetSession(ctx, lease.SessionName)
	if err != nil || current.ID != lease.SessionID {
		m.releaseClaim(entry)
		return identityError(err)
	}
	if err := runtime.KillSessionByID(ctx, lease.SessionID); err != nil {
		m.releaseClaim(entry)
		return err
	}
	if err := m.store.DeleteTmuxSessionRuntimeState(ctx, lease.SessionName); err != nil {
		m.releaseClaim(entry)
		return err
	}
	if err := m.store.DeleteTmuxSessionLease(ctx, lease.LeaseID); err != nil {
		m.releaseClaim(entry)
		return err
	}
	m.mu.Lock()
	m.removeEntryLocked(entry)
	m.mu.Unlock()
	m.publish(lease)
	return nil
}

// Rename records a new observed name for one stable managed runtime.
func (m *Manager) Rename(ctx context.Context, user, sessionID, newName string) error {
	user = strings.TrimSpace(user)
	sessionID = strings.TrimSpace(sessionID)
	newName = strings.TrimSpace(newName)
	m.mu.Lock()
	entry := m.byTarget[targetKey(user, sessionID)]
	if entry == nil {
		m.mu.Unlock()
		return nil
	}
	if entry.cleanupClaimed {
		m.mu.Unlock()
		return ErrCleanupClaimed
	}
	entry.inFlight++
	leaseID := entry.lease.LeaseID
	now := m.now()
	m.mu.Unlock()
	if err := m.store.RenameTmuxSessionLease(ctx, leaseID, newName, now); err != nil {
		m.mu.Lock()
		if current := m.byLease[leaseID]; current == entry && entry.inFlight > 0 {
			entry.inFlight--
		}
		m.mu.Unlock()
		return err
	}
	m.mu.Lock()
	var lease Lease
	if current := m.byLease[leaseID]; current == entry {
		entry.lease.SessionName = newName
		entry.lease.UpdatedAt = now
		entry.gen++
		entry.inFlight--
		lease = entry.lease
	}
	m.mu.Unlock()
	m.publish(lease)
	return nil
}

// Forget removes lifecycle ownership after an external or human kill.
func (m *Manager) Forget(ctx context.Context, user, sessionID string) error {
	m.mu.Lock()
	entry := m.byTarget[targetKey(strings.TrimSpace(user), strings.TrimSpace(sessionID))]
	if entry == nil {
		m.mu.Unlock()
		return nil
	}
	if entry.inFlight > 0 {
		m.mu.Unlock()
		return ErrLeaseInUse
	}
	if entry.cleanupClaimed {
		m.mu.Unlock()
		return ErrCleanupClaimed
	}
	entry.cleanupClaimed = true
	m.mu.Unlock()
	if err := m.store.DeleteTmuxSessionLease(ctx, entry.lease.LeaseID); err != nil {
		m.releaseClaim(entry)
		return err
	}
	m.mu.Lock()
	m.removeEntryLocked(entry)
	m.mu.Unlock()
	m.publish(entry.lease)
	return nil
}

// Snapshot returns every current lifecycle projection.
func (m *Manager) Snapshot() []Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Snapshot, 0, len(m.byLease))
	for _, entry := range m.byLease {
		result = append(result, snapshot(entry.lease))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].LeaseID < result[j].LeaseID })
	return result
}

// SnapshotByID returns a lifecycle projection by opaque lease ID.
func (m *Manager) SnapshotByID(leaseID string) (Snapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.byLease[strings.TrimSpace(leaseID)]
	if entry == nil {
		return Snapshot{}, false
	}
	return snapshot(entry.lease), true
}

// applyLeaseLocked writes a decided lease back into a shared entry only while
// nothing advanced that entry during the unlocked persistence gap, and returns
// the lease the entry holds afterwards. When the write back loses that race the
// store now holds a decision the entry rejected, so a renewed lease is marked
// dirty for the sweeper to re-persist. Callers must hold m.mu.
func (m *Manager) applyLeaseLocked(entry *leaseEntry, lease Lease, gen uint64) Lease {
	if m.byLease[lease.LeaseID] == entry && entry.gen == gen {
		entry.lease = lease
		entry.gen++
		return lease
	}
	if entry.lease.State == store.TmuxSessionLeaseActive {
		entry.dirtyTouch = true
	}
	return entry.lease
}

func (m *Manager) persistTouch(ctx context.Context, entry *leaseEntry, lease Lease) error {
	err := m.store.TouchTmuxSessionLease(ctx, lease.LeaseID, lease.LastRenewedAt, lease.ExpiresAt, lease.UpdatedAt)
	if err != nil {
		return err
	}
	m.mu.Lock()
	if current := m.byLease[lease.LeaseID]; current == entry &&
		current.lease.LastRenewedAt.Equal(lease.LastRenewedAt) {
		current.dirtyTouch = false
	}
	m.mu.Unlock()
	return nil
}

func (m *Manager) abortCreated(
	ctx context.Context,
	runtime Runtime,
	created tmux.Session,
	leaseID string,
) error {
	var result error
	current, err := runtime.GetSession(ctx, created.Name)
	sameRuntime := err == nil && current.ID == created.ID
	missingRuntime := tmux.IsKind(err, tmux.ErrKindSessionNotFound)
	switch {
	case sameRuntime:
		if killErr := runtime.KillSessionByID(ctx, created.ID); killErr != nil {
			return killErr
		}
	case err == nil:
		result = errors.Join(result, ErrIdentityMismatch)
	case missingRuntime:
	default:
		return err
	}
	if sameRuntime || missingRuntime {
		if cleanupErr := m.store.DeleteTmuxSessionRuntimeState(ctx, created.Name); cleanupErr != nil {
			return errors.Join(result, cleanupErr)
		}
	}
	if leaseID != "" {
		if err := m.store.DeleteTmuxSessionLease(ctx, leaseID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.Join(result, err)
		}
		m.mu.Lock()
		if entry := m.byLease[leaseID]; entry != nil {
			m.removeEntryLocked(entry)
		}
		m.mu.Unlock()
	}
	return result
}

func (m *Manager) claim(leaseID, confirmName string) (*leaseEntry, error) {
	leaseID = strings.TrimSpace(leaseID)
	confirmName = strings.TrimSpace(confirmName)
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.byLease[leaseID]
	if entry == nil {
		return nil, ErrLeaseNotFound
	}
	if confirmName != "" && entry.lease.SessionName != confirmName {
		return nil, ErrIdentityMismatch
	}
	if entry.inFlight > 0 {
		return nil, ErrLeaseInUse
	}
	if entry.cleanupClaimed {
		return nil, ErrCleanupClaimed
	}
	entry.cleanupClaimed = true
	return entry, nil
}

func (m *Manager) releaseClaim(entry *leaseEntry) {
	m.mu.Lock()
	if current := m.byLease[entry.lease.LeaseID]; current == entry {
		entry.cleanupClaimed = false
	}
	m.mu.Unlock()
}

func (m *Manager) addEntryLocked(entry *leaseEntry) {
	m.byLease[entry.lease.LeaseID] = entry
	m.byTarget[targetKey(entry.lease.User, entry.lease.SessionID)] = entry
}

func (m *Manager) removeEntryLocked(entry *leaseEntry) {
	delete(m.byLease, entry.lease.LeaseID)
	key := targetKey(entry.lease.User, entry.lease.SessionID)
	if m.byTarget[key] == entry {
		delete(m.byTarget, key)
	}
}

func (m *Manager) publish(lease Lease) {
	m.opts.Publish(events.TypeTmuxSessions, map[string]any{
		"action":  "lifecycle",
		"session": lease.SessionName,
		"user":    lease.User,
		"state":   lease.State,
	})
}

func (m *Manager) now() time.Time { return m.opts.Clock.Now().UTC() }

func targetKey(user, sessionID string) string { return user + "\x00" + sessionID }

func snapshot(lease Lease) Snapshot {
	return Snapshot{
		LeaseID:       lease.LeaseID,
		SessionID:     lease.SessionID,
		SessionName:   lease.SessionName,
		User:          lease.User,
		Source:        lease.Source,
		State:         lease.State,
		CreatedAt:     lease.CreatedAt,
		LastRenewedAt: lease.LastRenewedAt,
		ExpiresAt:     lease.ExpiresAt,
		GraceUntil:    lease.GraceUntil,
	}
}

func newLeaseID() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return "lease_" + hex.EncodeToString(random), nil
}

func identityError(err error) error {
	if err == nil {
		return ErrIdentityMismatch
	}
	return errors.Join(ErrIdentityMismatch, err)
}
