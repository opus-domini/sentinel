package tmuxlifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

func TestCreateCompensatesPrepareFailureByExactID(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	st := newLifecycleStore(t)
	runtime := newFakeRuntime()
	runtime.nextID = "$31"
	manager := newTestManager(st, &fakeClock{now: now}, runtime, Options{})

	_, err := manager.Create(ctx, "", "agent", "/srv/app", func(context.Context, tmux.Session) error {
		return errors.New("inspect panes")
	})
	if err == nil {
		t.Fatal("Create() succeeded despite prepare failure")
	}
	if got := runtime.killedIDs(); len(got) != 1 || got[0] != "$31" {
		t.Fatalf("killed IDs = %q, want [$31]", got)
	}
	if runtime.hasSession("agent") {
		t.Fatal("failed create left the tmux session running")
	}
	leases, listErr := st.ListTmuxSessionLeases(ctx)
	if listErr != nil || len(leases) != 0 || len(manager.Snapshot()) != 0 {
		t.Fatalf("leases after abort = %#v, list error = %v", leases, listErr)
	}
}

func TestFinishReportsSuccessAndRetriesDeferredRenewalPersistence(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	base := newLifecycleStore(t)
	lease := testLease(now, store.TmuxSessionLeaseActive)
	lease.ExpiresAt = now.Add(30 * time.Minute)
	seedLease(t, base, lease)
	runtime := newFakeRuntime()
	runtime.put(tmux.Session{ID: lease.SessionID, Name: lease.SessionName})
	failing := &failTouchStore{Store: base, remaining: 1}
	clock := &fakeClock{now: now}
	manager := newTestManager(failing, clock, runtime, Options{})
	if err := manager.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}

	use, err := manager.BeginUse(ctx, lease.User, lease.SessionName)
	if err != nil || !use.Managed() {
		t.Fatalf("BeginUse() = %#v, %v", use, err)
	}
	clock.Advance(time.Minute)
	if err := manager.Finish(ctx, use, true); err != nil {
		t.Fatalf("Finish() reported a deferred persistence failure: %v", err)
	}
	snapshot, ok := manager.SnapshotByID(lease.LeaseID)
	if !ok || !snapshot.ExpiresAt.Equal(clock.Now().Add(DefaultIdleTimeout)) {
		t.Fatalf("conservative snapshot = %#v", snapshot)
	}

	clock.Advance(30 * time.Minute)
	if err := manager.Sweep(ctx); err != nil {
		t.Fatalf("Sweep() retry error = %v", err)
	}
	if got := runtime.killedIDs(); len(got) != 0 {
		t.Fatalf("renewal persistence retry killed session: %q", got)
	}
	persisted, err := base.GetTmuxSessionLease(ctx, lease.LeaseID)
	if err != nil || !persisted.ExpiresAt.Equal(snapshot.ExpiresAt) {
		t.Fatalf("persisted lease = %#v, error = %v", persisted, err)
	}
}

func TestKeepAndCloseRespectClaimsAndStableIdentity(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	st := newLifecycleStore(t)
	lease := testLease(now, store.TmuxSessionLeaseActive)
	seedLease(t, st, lease)
	runtime := newFakeRuntime()
	runtime.put(tmux.Session{ID: lease.SessionID, Name: lease.SessionName})
	manager := newTestManager(st, &fakeClock{now: now}, runtime, Options{})
	if err := manager.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}

	use, err := manager.BeginUse(ctx, lease.User, lease.SessionName)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Close(ctx, lease.LeaseID, lease.SessionName); !errors.Is(err, ErrLeaseInUse) {
		t.Fatalf("Close() during use error = %v", err)
	}
	if err := manager.Finish(ctx, use, false); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Keep(ctx, lease.LeaseID, "wrong-name"); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("Keep() wrong confirmation error = %v", err)
	}
	if _, err := manager.Keep(ctx, lease.LeaseID, lease.SessionName); err != nil {
		t.Fatalf("Keep() error = %v", err)
	}
	if got := runtime.killedIDs(); len(got) != 0 {
		t.Fatalf("Keep() killed runtime IDs: %q", got)
	}
}

func TestKeepAndCloseRejectReusedNameWithoutKill(t *testing.T) {
	for _, operation := range []string{"keep", "close"} {
		t.Run(operation, func(t *testing.T) {
			ctx := context.Background()
			now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
			st := newLifecycleStore(t)
			lease := testLease(now, store.TmuxSessionLeaseActive)
			seedLease(t, st, lease)
			runtime := newFakeRuntime()
			runtime.put(tmux.Session{ID: "$99", Name: lease.SessionName})
			manager := newTestManager(st, &fakeClock{now: now}, runtime, Options{})
			entry := &leaseEntry{lease: lease}
			manager.mu.Lock()
			manager.addEntryLocked(entry)
			manager.mu.Unlock()

			var err error
			if operation == "keep" {
				_, err = manager.Keep(ctx, lease.LeaseID, lease.SessionName)
			} else {
				err = manager.Close(ctx, lease.LeaseID, lease.SessionName)
			}
			if !errors.Is(err, ErrIdentityMismatch) {
				t.Fatalf("%s reused-name error = %v", operation, err)
			}
			if got := runtime.killedIDs(); len(got) != 0 {
				t.Fatalf("%s killed reused runtime ID: %q", operation, got)
			}
			if !runtime.hasSession(lease.SessionName) {
				t.Fatalf("%s removed reused session", operation)
			}
		})
	}
}

func TestManagerStartAndStopWaitsForSweeper(t *testing.T) {
	st := newLifecycleStore(t)
	runtime := newFakeRuntime()
	manager := newTestManager(st, &fakeClock{now: time.Now().UTC()}, runtime, Options{
		SweepInterval: 5 * time.Millisecond,
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := manager.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

// TestStartBoundsBootReconciliationOnWedgedRuntime pins the boot budget: the
// HTTP listener only comes up after Start returns, so an unresponsive tmux
// server must defer its leases to the next sweep instead of holding the
// process before it can serve anything.
func TestStartBoundsBootReconciliationOnWedgedRuntime(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	st := newLifecycleStore(t)
	lease := testLease(now, store.TmuxSessionLeaseActive)
	seedLease(t, st, lease)
	runtime := newFakeRuntime()
	runtime.getBlocks = true
	manager := newTestManager(st, &fakeClock{now: now}, runtime, Options{
		SweepInterval: 50 * time.Millisecond,
	})

	started := make(chan error, 1)
	go func() { started <- manager.Start(context.Background()) }()
	select {
	case err := <-started:
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start() blocked on an unresponsive tmux runtime")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := manager.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if _, ok := manager.SnapshotByID(lease.LeaseID); !ok {
		t.Fatal("deferred boot reconciliation dropped the lease")
	}
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(duration time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(duration)
	c.mu.Unlock()
}

type fakeRuntime struct {
	mu       sync.Mutex
	sessions map[string]tmux.Session
	nextID   string
	killed   []string
	getErr   error
	listErr  error
	killErr  error
	// getBlocks makes GetSession answer only when its context ends, standing in
	// for a wedged tmux server (a hung user switch, an unresponsive socket).
	getBlocks bool
}

func newFakeRuntime() *fakeRuntime {
	return &fakeRuntime{sessions: make(map[string]tmux.Session), nextID: "$1"}
}

func (r *fakeRuntime) CreateSessionWithID(_ context.Context, name, _ string) (tmux.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	session := tmux.Session{ID: r.nextID, Name: name}
	r.sessions[name] = session
	return session, nil
}

func (r *fakeRuntime) GetSession(ctx context.Context, name string) (tmux.Session, error) {
	r.mu.Lock()
	blocks, getErr := r.getBlocks, r.getErr
	session, ok := r.sessions[name]
	r.mu.Unlock()
	if blocks {
		<-ctx.Done()
		return tmux.Session{}, ctx.Err()
	}
	if getErr != nil {
		return tmux.Session{}, getErr
	}
	if !ok {
		return tmux.Session{}, &tmux.Error{Kind: tmux.ErrKindSessionNotFound, Msg: "missing"}
	}
	return session, nil
}

func (r *fakeRuntime) ListSessions(context.Context) ([]tmux.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listErr != nil {
		return nil, r.listErr
	}
	result := make([]tmux.Session, 0, len(r.sessions))
	for _, session := range r.sessions {
		result = append(result, session)
	}
	return result, nil
}

func (r *fakeRuntime) KillSessionByID(_ context.Context, sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.killErr != nil {
		return r.killErr
	}
	r.killed = append(r.killed, sessionID)
	for name, session := range r.sessions {
		if session.ID == sessionID {
			delete(r.sessions, name)
		}
	}
	return nil
}

func (r *fakeRuntime) put(session tmux.Session) {
	r.mu.Lock()
	r.sessions[session.Name] = session
	r.mu.Unlock()
}

func (r *fakeRuntime) setAttached(name string, attached int) {
	r.mu.Lock()
	session := r.sessions[name]
	session.Attached = attached
	r.sessions[name] = session
	r.mu.Unlock()
}

func (r *fakeRuntime) hasSession(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.sessions[name]
	return ok
}

func (r *fakeRuntime) killedIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.killed...)
}

type failTouchStore struct {
	*store.Store
	mu        sync.Mutex
	remaining int
}

func (s *failTouchStore) TouchTmuxSessionLease(
	ctx context.Context,
	leaseID string,
	lastRenewedAt, expiresAt, updatedAt time.Time,
) error {
	s.mu.Lock()
	if s.remaining > 0 {
		s.remaining--
		s.mu.Unlock()
		return errors.New("database unavailable")
	}
	s.mu.Unlock()
	return s.Store.TouchTmuxSessionLease(ctx, leaseID, lastRenewedAt, expiresAt, updatedAt)
}

func newLifecycleStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "sentinel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func newTestManager(leaseStore LeaseStore, clock Clock, runtime Runtime, overrides Options) *Manager {
	overrides.Clock = clock
	overrides.RuntimeForUser = func(string) Runtime { return runtime }
	return New(leaseStore, overrides)
}

func testLease(now time.Time, state string) Lease {
	return Lease{
		LeaseID:       "lease_test",
		SessionID:     "$7",
		SessionName:   "agent",
		User:          "deploy",
		Source:        store.TmuxSessionLeaseSourceMCP,
		State:         state,
		CreatedAt:     now.Add(-time.Hour),
		LastRenewedAt: now.Add(-time.Hour),
		ExpiresAt:     now.Add(time.Hour),
		UpdatedAt:     now.Add(-time.Hour),
	}
}

func seedLease(t *testing.T, st *store.Store, lease Lease) {
	t.Helper()
	if err := st.CreateTmuxSessionLease(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
}
