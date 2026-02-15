package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sub", "sentinel.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = s.Close() }()

	// Verify the subdirectory was created by New().
	if _, err := New(dbPath); err != nil {
		t.Fatalf("second New() on same path error = %v", err)
	}
}

func TestGetAllEmpty(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	got, err := s.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("GetAll() returned %d entries, want 0", len(got))
	}
}

func TestUpsertAndGetAll(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	// Insert two sessions.
	if err := s.UpsertSession(ctx, "foo", "h1", "c1"); err != nil {
		t.Fatalf("UpsertSession(foo) error = %v", err)
	}
	if err := s.UpsertSession(ctx, "bar", "h2", "c2"); err != nil {
		t.Fatalf("UpsertSession(bar) error = %v", err)
	}

	got, err := s.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetAll() returned %d entries, want 2", len(got))
	}
	if got["foo"].Hash != "h1" || got["foo"].LastContent != "c1" {
		t.Errorf("foo = %+v, want Hash=h1 LastContent=c1", got["foo"])
	}
	if got["bar"].Hash != "h2" || got["bar"].LastContent != "c2" {
		t.Errorf("bar = %+v, want Hash=h2 LastContent=c2", got["bar"])
	}

	// Upsert foo again (conflict update).
	if err := s.UpsertSession(ctx, "foo", "h3", "c3"); err != nil {
		t.Fatalf("UpsertSession(foo update) error = %v", err)
	}

	got, err = s.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll() after update error = %v", err)
	}
	if got["foo"].Hash != "h3" || got["foo"].LastContent != "c3" {
		t.Errorf("foo after update = %+v, want Hash=h3 LastContent=c3", got["foo"])
	}
	if got["bar"].Hash != "h2" || got["bar"].LastContent != "c2" {
		t.Errorf("bar unchanged = %+v, want Hash=h2 LastContent=c2", got["bar"])
	}
}

func TestPurgeRemovesInactive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	seedSessionsForPurgeTest(t, s, ctx, []string{"a", "b", "c"})
	if err := s.Purge(ctx, []string{"a", "c"}); err != nil {
		t.Fatalf("Purge([a,c]) error = %v", err)
	}

	got := mustGetAllSessions(t, s, ctx)
	if len(got) != 2 {
		t.Fatalf("after purge got %d entries, want 2", len(got))
	}
	if _, ok := got["b"]; ok {
		t.Errorf("session 'b' should have been purged")
	}
}

func TestPurgeWithEmptyActiveRemovesAll(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	seedSessionsForPurgeTest(t, s, ctx, []string{"a", "b", "c"})
	if err := s.Purge(ctx, []string{}); err != nil {
		t.Fatalf("Purge([]) error = %v", err)
	}

	got := mustGetAllSessions(t, s, ctx)
	if len(got) != 0 {
		t.Fatalf("after purge-all got %d entries, want 0", len(got))
	}
}

func TestPurgeKeepsAllActive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	seedSessionsForPurgeTest(t, s, ctx, []string{"x", "y"})
	if err := s.Purge(ctx, []string{"x", "y"}); err != nil {
		t.Fatalf("Purge([x,y]) error = %v", err)
	}

	got := mustGetAllSessions(t, s, ctx)
	if len(got) != 2 {
		t.Fatalf("after purge-none got %d entries, want 2", len(got))
	}
}

func seedSessionsForPurgeTest(t *testing.T, s *Store, ctx context.Context, names []string) {
	t.Helper()
	for _, name := range names {
		if err := s.UpsertSession(ctx, name, "h", "c"); err != nil {
			t.Fatalf("UpsertSession(%s) error = %v", name, err)
		}
	}
}

func mustGetAllSessions(t *testing.T, s *Store, ctx context.Context) map[string]SessionMeta {
	t.Helper()
	got, err := s.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}
	return got
}

func TestRename(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("rename existing", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		if err := s.UpsertSession(ctx, "old", "h1", "c1"); err != nil {
			t.Fatalf("UpsertSession(old) error = %v", err)
		}
		if err := s.Rename(ctx, "old", "new"); err != nil {
			t.Fatalf("Rename(old→new) error = %v", err)
		}

		got, err := s.GetAll(ctx)
		if err != nil {
			t.Fatalf("GetAll() error = %v", err)
		}
		if _, ok := got["old"]; ok {
			t.Errorf("'old' should not exist after rename")
		}
		if got["new"].Hash != "h1" || got["new"].LastContent != "c1" {
			t.Errorf("'new' = %+v, want Hash=h1 LastContent=c1", got["new"])
		}
	})

	t.Run("rename nonexistent", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		if err := s.Rename(ctx, "ghost", "phantom"); err != nil {
			t.Fatalf("Rename(ghost→phantom) error = %v, want nil", err)
		}
	})
}

func TestSetIcon(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("set and get icon", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		if err := s.UpsertSession(ctx, "dev", "h1", "c1"); err != nil {
			t.Fatalf("UpsertSession(dev) error = %v", err)
		}
		if err := s.SetIcon(ctx, "dev", "bot"); err != nil {
			t.Fatalf("SetIcon(dev, bot) error = %v", err)
		}

		got, err := s.GetAll(ctx)
		if err != nil {
			t.Fatalf("GetAll() error = %v", err)
		}
		if got["dev"].Icon != "bot" {
			t.Errorf("dev.Icon = %q, want bot", got["dev"].Icon)
		}
	})

	t.Run("upsert does not overwrite icon", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		if err := s.UpsertSession(ctx, "dev", "h1", "c1"); err != nil {
			t.Fatalf("UpsertSession(dev) error = %v", err)
		}
		if err := s.SetIcon(ctx, "dev", "code"); err != nil {
			t.Fatalf("SetIcon(dev, code) error = %v", err)
		}
		// Upsert again — icon should be preserved.
		if err := s.UpsertSession(ctx, "dev", "h2", "c2"); err != nil {
			t.Fatalf("UpsertSession(dev update) error = %v", err)
		}

		got, err := s.GetAll(ctx)
		if err != nil {
			t.Fatalf("GetAll() error = %v", err)
		}
		if got["dev"].Icon != "code" {
			t.Errorf("dev.Icon = %q, want code (should survive upsert)", got["dev"].Icon)
		}
		if got["dev"].Hash != "h2" {
			t.Errorf("dev.Hash = %q, want h2", got["dev"].Hash)
		}
	})
}

func TestAllocateNextWindowSequence(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("allocates monotonically for same session", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		first, err := s.AllocateNextWindowSequence(ctx, "dev", 1)
		if err != nil {
			t.Fatalf("AllocateNextWindowSequence(first) error = %v", err)
		}
		second, err := s.AllocateNextWindowSequence(ctx, "dev", 1)
		if err != nil {
			t.Fatalf("AllocateNextWindowSequence(second) error = %v", err)
		}

		if first != 1 || second != 2 {
			t.Fatalf("allocated (%d, %d), want (1, 2)", first, second)
		}
	})

	t.Run("respects minimum floor for legacy sessions", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		seq, err := s.AllocateNextWindowSequence(ctx, "dev", 5)
		if err != nil {
			t.Fatalf("AllocateNextWindowSequence(min=5) error = %v", err)
		}
		next, err := s.AllocateNextWindowSequence(ctx, "dev", 1)
		if err != nil {
			t.Fatalf("AllocateNextWindowSequence(next) error = %v", err)
		}

		if seq != 5 || next != 6 {
			t.Fatalf("allocated (%d, %d), want (5, 6)", seq, next)
		}
	})

	t.Run("keeps sequence after rename", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		if _, err := s.AllocateNextWindowSequence(ctx, "old", 1); err != nil {
			t.Fatalf("AllocateNextWindowSequence(old) error = %v", err)
		}
		if err := s.Rename(ctx, "old", "new"); err != nil {
			t.Fatalf("Rename(old->new) error = %v", err)
		}

		seq, err := s.AllocateNextWindowSequence(ctx, "new", 1)
		if err != nil {
			t.Fatalf("AllocateNextWindowSequence(new) error = %v", err)
		}
		if seq != 2 {
			t.Fatalf("allocated seq = %d, want 2", seq)
		}
	})
}

func TestClose(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	ctx := context.Background()
	_, err := s.GetAll(ctx)
	if err == nil {
		t.Fatal("GetAll() after Close() should return error")
	}
}

// newTestStore creates a Store backed by a temporary SQLite database.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "sentinel.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return s
}
