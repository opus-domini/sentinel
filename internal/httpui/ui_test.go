package httpui

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type fakePingConn struct {
	writes atomic.Int32
	err    error
}

func (f *fakePingConn) WritePing(_ []byte) error {
	f.writes.Add(1)
	return f.err
}

func TestRunPingLoopStopsOnContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time, 2)
	conn := &fakePingConn{}
	errCh := make(chan error, 1)
	done := make(chan struct{})

	go func() {
		runPingLoop(ctx, conn, ticks, func(err error) {
			select {
			case errCh <- err:
			default:
			}
		})
		close(done)
	}()

	ticks <- time.Now()

	deadline := time.Now().Add(500 * time.Millisecond)
	for conn.writes.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if conn.writes.Load() != 1 {
		t.Fatalf("writes = %d, want 1", conn.writes.Load())
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runPingLoop did not stop after context cancellation")
	}

	select {
	case err := <-errCh:
		t.Fatalf("unexpected error reported: %v", err)
	default:
	}
}

func TestRunPingLoopReportsPingError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ticks := make(chan time.Time, 1)
	pingErr := errors.New("ping failed")
	conn := &fakePingConn{err: pingErr}
	gotErr := make(chan error, 1)
	done := make(chan struct{})

	go func() {
		runPingLoop(ctx, conn, ticks, func(err error) {
			select {
			case gotErr <- err:
			default:
			}
		})
		close(done)
	}()

	ticks <- time.Now()

	select {
	case err := <-gotErr:
		if !errors.Is(err, pingErr) {
			t.Fatalf("reported error = %v, want %v", err, pingErr)
		}
	case <-time.After(time.Second):
		t.Fatal("expected ping error to be reported")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runPingLoop did not stop after ping error")
	}
}
