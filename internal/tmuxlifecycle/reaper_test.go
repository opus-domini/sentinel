package tmuxlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

func TestSweepWaitsForInFlightOperationBeforeCleanup(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	st := newLifecycleStore(t)
	lease := testLease(now, store.TmuxSessionLeaseGrace)
	lease.ExpiresAt = now.Add(-time.Hour)
	lease.GraceUntil = now.Add(time.Minute)
	seedLease(t, st, lease)
	runtime := newFakeRuntime()
	runtime.put(tmux.Session{ID: lease.SessionID, Name: lease.SessionName})
	clock := &fakeClock{now: now}
	manager := newTestManager(st, clock, runtime, Options{})
	if err := manager.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	clock.Advance(2 * time.Minute)
	use, err := manager.BeginUse(ctx, lease.User, lease.SessionName)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if got := runtime.killedIDs(); len(got) != 0 {
		t.Fatalf("in-flight session was killed: %q", got)
	}
	if err := manager.Finish(ctx, use, false); err != nil {
		t.Fatal(err)
	}
	if err := manager.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if got := runtime.killedIDs(); len(got) != 1 || got[0] != lease.SessionID {
		t.Fatalf("killed IDs = %q, want [%s]", got, lease.SessionID)
	}
}

func TestSweepDrainsMCPThenBlocksForHumanAttachment(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	st := newLifecycleStore(t)
	lease := testLease(now, store.TmuxSessionLeaseGrace)
	lease.ExpiresAt = now.Add(-time.Hour)
	lease.GraceUntil = now.Add(time.Minute)
	seedLease(t, st, lease)
	runtime := newFakeRuntime()
	runtime.put(tmux.Session{ID: lease.SessionID, Name: lease.SessionName, Attached: 2})
	clock := &fakeClock{now: now}
	detached := 0
	manager := newTestManager(st, clock, runtime, Options{
		DetachSession: func(user, session string) {
			if user != lease.User || session != lease.SessionName {
				t.Fatalf("DetachSession(%q, %q)", user, session)
			}
			detached++
			runtime.setAttached(session, 1)
		},
	})
	if err := manager.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	clock.Advance(2 * time.Minute)
	if err := manager.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	snapshot, ok := manager.SnapshotByID(lease.LeaseID)
	if !ok || snapshot.State != store.TmuxSessionLeaseCleanupBlocked || detached != 1 {
		t.Fatalf("blocked snapshot = %#v, detached = %d", snapshot, detached)
	}
	if got := runtime.killedIDs(); len(got) != 0 {
		t.Fatalf("human-attached session was killed: %q", got)
	}

	use, err := manager.BeginUse(ctx, lease.User, lease.SessionName)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Finish(ctx, use, true); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = manager.SnapshotByID(lease.LeaseID)
	if snapshot.State != store.TmuxSessionLeaseActive || !snapshot.ExpiresAt.Equal(clock.Now().Add(DefaultIdleTimeout)) {
		t.Fatalf("reactivated snapshot = %#v", snapshot)
	}
}

func TestSweepForgetsReusedNameWithoutKill(t *testing.T) {
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

	if err := manager.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if len(manager.Snapshot()) != 0 {
		t.Fatalf("stale lease remains: %#v", manager.Snapshot())
	}
	if got := runtime.killedIDs(); len(got) != 0 {
		t.Fatalf("ABA reconciliation killed ID: %q", got)
	}
	if !runtime.hasSession(lease.SessionName) {
		t.Fatal("replacement session was removed")
	}
}

func TestReconcileExpiredLeaseGetsFreshRecoveryGrace(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	st := newLifecycleStore(t)
	lease := testLease(now, store.TmuxSessionLeaseActive)
	lease.ExpiresAt = now.Add(-4 * time.Hour)
	seedLease(t, st, lease)
	runtime := newFakeRuntime()
	runtime.put(tmux.Session{ID: lease.SessionID, Name: lease.SessionName})
	manager := newTestManager(st, &fakeClock{now: now}, runtime, Options{})

	if err := manager.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	snapshot, ok := manager.SnapshotByID(lease.LeaseID)
	if !ok || snapshot.State != store.TmuxSessionLeaseGrace ||
		!snapshot.GraceUntil.Equal(now.Add(DefaultGracePeriod)) {
		t.Fatalf("recovery snapshot = %#v", snapshot)
	}
	if got := runtime.killedIDs(); len(got) != 0 {
		t.Fatalf("boot reconciliation killed session: %q", got)
	}
}

func TestReconcileTracksRenameByStableID(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	st := newLifecycleStore(t)
	lease := testLease(now, store.TmuxSessionLeaseActive)
	seedLease(t, st, lease)
	runtime := newFakeRuntime()
	runtime.put(tmux.Session{ID: lease.SessionID, Name: "agent-renamed"})
	manager := newTestManager(st, &fakeClock{now: now}, runtime, Options{})

	if err := manager.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	snapshot, ok := manager.SnapshotByID(lease.LeaseID)
	if !ok || snapshot.SessionName != "agent-renamed" {
		t.Fatalf("renamed snapshot = %#v", snapshot)
	}
	persisted, err := st.GetTmuxSessionLease(ctx, lease.LeaseID)
	if err != nil || persisted.SessionName != "agent-renamed" {
		t.Fatalf("persisted rename = %#v, error = %v", persisted, err)
	}
}

func TestReconcileDefersUnavailableRuntimeWithoutDestroyingLease(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	st := newLifecycleStore(t)
	lease := testLease(now, store.TmuxSessionLeaseActive)
	lease.ExpiresAt = now.Add(-time.Hour)
	seedLease(t, st, lease)
	runtime := newFakeRuntime()
	runtime.getErr = &tmux.Error{Kind: tmux.ErrKindCommandFailed, Msg: "permission denied"}
	manager := newTestManager(st, &fakeClock{now: now}, runtime, Options{})

	if err := manager.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile() unavailable error = %v", err)
	}
	snapshot, ok := manager.SnapshotByID(lease.LeaseID)
	if !ok {
		t.Fatal("unavailable runtime destroyed lease")
	}
	if snapshot.State != store.TmuxSessionLeaseGrace || !snapshot.GraceUntil.Equal(now.Add(DefaultGracePeriod)) {
		t.Fatalf("unavailable runtime recovery snapshot = %#v", snapshot)
	}
	if got := runtime.killedIDs(); len(got) != 0 {
		t.Fatalf("unavailable runtime triggered kill: %q", got)
	}
}
