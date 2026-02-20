package store

import (
	"context"
	"database/sql"
	"testing"
)

func TestGetOpsRunbook(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	// Seeded runbooks should exist.
	runbooks, err := s.ListOpsRunbooks(ctx)
	if err != nil {
		t.Fatalf("ListOpsRunbooks: %v", err)
	}
	if len(runbooks) == 0 {
		t.Fatal("expected at least one seeded runbook")
	}

	// Fetch by ID.
	got, err := s.GetOpsRunbook(ctx, runbooks[0].ID)
	if err != nil {
		t.Fatalf("GetOpsRunbook: %v", err)
	}
	if got.ID != runbooks[0].ID {
		t.Fatalf("id = %q, want %q", got.ID, runbooks[0].ID)
	}
	if got.Name == "" {
		t.Fatal("name is empty")
	}
}

func TestGetOpsRunbookEmptyID(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	_, err := s.GetOpsRunbook(context.Background(), "")
	if err != sql.ErrNoRows {
		t.Fatalf("error = %v, want sql.ErrNoRows", err)
	}
}

func TestGetOpsRunbookNonexistent(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	_, err := s.GetOpsRunbook(context.Background(), "nonexistent-runbook-id")
	if err != sql.ErrNoRows {
		t.Fatalf("error = %v, want sql.ErrNoRows", err)
	}
}
