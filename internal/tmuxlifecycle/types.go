// Package tmuxlifecycle owns ephemeral tmux session leases independently of
// the MCP transport that created them.
package tmuxlifecycle

import (
	"context"
	"errors"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
)

const (
	// DefaultIdleTimeout is the sliding idle lifetime of an ephemeral session.
	DefaultIdleTimeout = 2 * time.Hour
	// DefaultGracePeriod protects an expired session before cleanup.
	DefaultGracePeriod = 10 * time.Minute
	// DefaultSweepInterval controls lifecycle reconciliation frequency.
	DefaultSweepInterval = time.Minute
)

var (
	// ErrLeaseNotFound means no managed lifecycle lease matched the request.
	ErrLeaseNotFound = errors.New("tmux session lifecycle lease not found")
	// ErrLeaseInUse means a destructive transition lost to an active operation.
	ErrLeaseInUse = errors.New("tmux session lifecycle lease is in use")
	// ErrCleanupClaimed means cleanup already owns the lease transition.
	ErrCleanupClaimed = errors.New("tmux session lifecycle cleanup is in progress")
	// ErrIdentityMismatch means the observed tmux runtime no longer matches the lease.
	ErrIdentityMismatch = errors.New("tmux session lifecycle identity mismatch")
)

// Lease is the persisted lifecycle identity and deadline state.
type Lease = store.TmuxSessionLease

// Snapshot is an immutable lifecycle projection safe to return to callers.
type Snapshot struct {
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
}

// Use guards one targeted operation against concurrent cleanup.
type Use struct {
	LeaseID string
}

// Managed reports whether this operation targets an ephemeral session.
func (u Use) Managed() bool { return u.LeaseID != "" }

// Clock supplies deterministic server time.
type Clock interface {
	Now() time.Time
}

// Runtime is the minimum tmux adapter surface owned by lifecycle.
type Runtime interface {
	CreateSessionWithID(context.Context, string, string) (tmux.Session, error)
	GetSession(context.Context, string) (tmux.Session, error)
	ListSessions(context.Context) ([]tmux.Session, error)
	KillSessionByID(context.Context, string) error
}

// LeaseStore is the persistence surface owned by lifecycle.
type LeaseStore interface {
	CreateTmuxSessionLease(context.Context, store.TmuxSessionLease) error
	GetTmuxSessionLease(context.Context, string) (store.TmuxSessionLease, error)
	ListTmuxSessionLeases(context.Context) ([]store.TmuxSessionLease, error)
	UpdateTmuxSessionLeaseState(context.Context, string, string, time.Time, time.Time, time.Time) error
	TouchTmuxSessionLease(context.Context, string, time.Time, time.Time, time.Time) error
	RenameTmuxSessionLease(context.Context, string, string, time.Time) error
	DeleteTmuxSessionLease(context.Context, string) error
	DeleteTmuxSessionRuntimeState(context.Context, string) error
}

// Options configures the shared lifecycle manager.
type Options struct {
	Clock          Clock
	IdleTimeout    time.Duration
	GracePeriod    time.Duration
	SweepInterval  time.Duration
	RuntimeForUser func(string) Runtime
	DetachSession  func(user, session string)
	Publish        func(eventType string, payload map[string]any)
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }
