package terminals

import (
	"sync"
	"testing"
	"time"
)

func TestNewRegistry(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	list := r.List()
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %d entries", len(list))
	}
}

func TestRegister(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	id, release := r.Register("mysession", "127.0.0.1", 80, 24, nil)
	defer release()

	if id == "" {
		t.Fatal("expected non-empty id")
	}

	list := r.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(list))
	}

	snap := list[0]
	if snap.ID != id {
		t.Fatalf("expected id %q, got %q", id, snap.ID)
	}
	if snap.Session != "mysession" {
		t.Fatalf("expected session %q, got %q", "mysession", snap.Session)
	}
	if snap.RemoteAddr != "127.0.0.1" {
		t.Fatalf("expected remoteAddr %q, got %q", "127.0.0.1", snap.RemoteAddr)
	}
	if snap.Cols != 80 {
		t.Fatalf("expected cols 80, got %d", snap.Cols)
	}
	if snap.Rows != 24 {
		t.Fatalf("expected rows 24, got %d", snap.Rows)
	}
}

func TestRegisterRelease(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	_, release := r.Register("sess", "addr", 80, 24, nil)

	release()

	list := r.List()
	if len(list) != 0 {
		t.Fatalf("expected empty list after release, got %d entries", len(list))
	}

	// Calling release twice must not panic (sync.Once).
	release()
}

func TestRegisterNilReceiver(t *testing.T) {
	t.Parallel()

	var r *Registry
	id, release := r.Register("sess", "addr", 80, 24, nil)

	if id != "" {
		t.Fatalf("expected empty id on nil receiver, got %q", id)
	}

	// release should be a noop and not panic.
	release()
}

func TestUpdateSize(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	id, release := r.Register("sess", "addr", 80, 24, nil)
	defer release()

	r.UpdateSize(id, 120, 40)

	list := r.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(list))
	}
	if list[0].Cols != 120 {
		t.Fatalf("expected cols 120 after update, got %d", list[0].Cols)
	}
	if list[0].Rows != 40 {
		t.Fatalf("expected rows 40 after update, got %d", list[0].Rows)
	}

	// Invalid values should not change anything.
	r.UpdateSize(id, 0, 50)
	list = r.List()
	if list[0].Cols != 120 {
		t.Fatalf("expected cols 120 after invalid update, got %d", list[0].Cols)
	}

	r.UpdateSize(id, 100, -1)
	list = r.List()
	if list[0].Rows != 40 {
		t.Fatalf("expected rows 40 after negative update, got %d", list[0].Rows)
	}

	// Unknown ID is a no-op.
	r.UpdateSize("nonexistent", 200, 50)
}

func TestListSorting(t *testing.T) {
	t.Parallel()

	r := NewRegistry()

	id1, release1 := r.Register("first", "addr", 80, 24, nil)
	defer release1()
	time.Sleep(time.Millisecond)

	id2, release2 := r.Register("second", "addr", 80, 24, nil)
	defer release2()
	time.Sleep(time.Millisecond)

	id3, release3 := r.Register("third", "addr", 80, 24, nil)
	defer release3()

	list := r.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(list))
	}

	// Newest first: id3, id2, id1
	if list[0].ID != id3 {
		t.Fatalf("expected newest entry first (id3=%s), got %s", id3, list[0].ID)
	}
	if list[1].ID != id2 {
		t.Fatalf("expected second newest (id2=%s), got %s", id2, list[1].ID)
	}
	if list[2].ID != id1 {
		t.Fatalf("expected oldest last (id1=%s), got %s", id1, list[2].ID)
	}
}

func TestClose(t *testing.T) {
	t.Parallel()

	r := NewRegistry()

	closed := false
	id, release := r.Register("sess", "addr", 80, 24, func(reason string) {
		closed = true
	})
	defer release()

	ok := r.Close(id, "test shutdown")
	if !ok {
		t.Fatal("expected Close to return true for existing entry")
	}
	if !closed {
		t.Fatal("expected closeFn to be called")
	}

	// Unknown ID returns false.
	if r.Close("nonexistent", "reason") {
		t.Fatal("expected Close to return false for unknown id")
	}

	// Nil registry returns false.
	var nilReg *Registry
	if nilReg.Close("any", "reason") {
		t.Fatal("expected Close to return false on nil registry")
	}
}

func TestRegistryConcurrency(t *testing.T) {
	t.Parallel()

	r := NewRegistry()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			id, release := r.Register("sess", "addr", 80, 24, nil)
			r.List()
			r.UpdateSize(id, 120, 40)
			r.List()
			release()
		}()
	}

	wg.Wait()

	list := r.List()
	if len(list) != 0 {
		t.Fatalf("expected empty list after all releases, got %d", len(list))
	}
}
