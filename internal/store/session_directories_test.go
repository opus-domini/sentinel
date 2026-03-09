package store

import (
	"context"
	"testing"
)

func TestRecordSessionDirectory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("insert new directory", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		if err := s.RecordSessionDirectory(ctx, "/home/user/projects"); err != nil {
			t.Fatalf("RecordSessionDirectory() error = %v", err)
		}

		dirs, err := s.ListFrequentDirectories(ctx, 10)
		if err != nil {
			t.Fatalf("ListFrequentDirectories() error = %v", err)
		}
		if len(dirs) != 1 {
			t.Fatalf("got %d dirs, want 1", len(dirs))
		}
		if dirs[0] != "/home/user/projects" {
			t.Errorf("dirs[0] = %q, want /home/user/projects", dirs[0])
		}
	})

	t.Run("upsert increments use_count", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		path := "/home/user/work"
		for i := 0; i < 3; i++ {
			if err := s.RecordSessionDirectory(ctx, path); err != nil {
				t.Fatalf("RecordSessionDirectory() iteration %d error = %v", i, err)
			}
		}

		// Insert a second directory once.
		if err := s.RecordSessionDirectory(ctx, "/tmp"); err != nil {
			t.Fatalf("RecordSessionDirectory(/tmp) error = %v", err)
		}

		dirs, err := s.ListFrequentDirectories(ctx, 10)
		if err != nil {
			t.Fatalf("ListFrequentDirectories() error = %v", err)
		}
		if len(dirs) != 2 {
			t.Fatalf("got %d dirs, want 2", len(dirs))
		}
		// /home/user/work has use_count=3, /tmp has use_count=1.
		if dirs[0] != path {
			t.Errorf("dirs[0] = %q, want %q (highest use_count)", dirs[0], path)
		}
		if dirs[1] != "/tmp" {
			t.Errorf("dirs[1] = %q, want /tmp", dirs[1])
		}
	})
}

func TestListFrequentDirectories(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("empty table returns nil", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		dirs, err := s.ListFrequentDirectories(ctx, 5)
		if err != nil {
			t.Fatalf("ListFrequentDirectories() error = %v", err)
		}
		if dirs != nil {
			t.Fatalf("got %v, want nil", dirs)
		}
	})

	t.Run("respects limit", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		for _, p := range []string{"/a", "/b", "/c", "/d", "/e"} {
			if err := s.RecordSessionDirectory(ctx, p); err != nil {
				t.Fatalf("RecordSessionDirectory(%s) error = %v", p, err)
			}
		}

		dirs, err := s.ListFrequentDirectories(ctx, 3)
		if err != nil {
			t.Fatalf("ListFrequentDirectories() error = %v", err)
		}
		if len(dirs) != 3 {
			t.Fatalf("got %d dirs, want 3", len(dirs))
		}
	})

	t.Run("ordering by use_count descending", func(t *testing.T) {
		t.Parallel()
		s := newTestStore(t)
		defer func() { _ = s.Close() }()

		// Record /rare once, /common three times, /medium twice.
		if err := s.RecordSessionDirectory(ctx, "/rare"); err != nil {
			t.Fatalf("RecordSessionDirectory(/rare) error = %v", err)
		}
		for i := 0; i < 3; i++ {
			if err := s.RecordSessionDirectory(ctx, "/common"); err != nil {
				t.Fatalf("RecordSessionDirectory(/common) error = %v", err)
			}
		}
		for i := 0; i < 2; i++ {
			if err := s.RecordSessionDirectory(ctx, "/medium"); err != nil {
				t.Fatalf("RecordSessionDirectory(/medium) error = %v", err)
			}
		}

		dirs, err := s.ListFrequentDirectories(ctx, 10)
		if err != nil {
			t.Fatalf("ListFrequentDirectories() error = %v", err)
		}
		if len(dirs) != 3 {
			t.Fatalf("got %d dirs, want 3", len(dirs))
		}
		if dirs[0] != "/common" {
			t.Errorf("dirs[0] = %q, want /common", dirs[0])
		}
		if dirs[1] != "/medium" {
			t.Errorf("dirs[1] = %q, want /medium", dirs[1])
		}
		if dirs[2] != "/rare" {
			t.Errorf("dirs[2] = %q, want /rare", dirs[2])
		}
	})
}
