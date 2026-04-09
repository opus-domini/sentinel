package store

import (
	"context"
	"testing"
)

func TestSessionUsersLifecycle(t *testing.T) {
	t.Parallel()

	const renamedUser = "anna"

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	users, err := s.ListSessionUsers(ctx)
	if err != nil {
		t.Fatalf("ListSessionUsers() initial error = %v", err)
	}
	if len(users) != 0 {
		t.Fatalf("initial len(users) = %d, want 0", len(users))
	}

	if err := s.SetSessionUser(ctx, "alpha", "alice"); err != nil {
		t.Fatalf("SetSessionUser(alpha, alice) error = %v", err)
	}
	if err := s.SetSessionUser(ctx, "beta", "bob"); err != nil {
		t.Fatalf("SetSessionUser(beta, bob) error = %v", err)
	}
	if err := s.SetSessionUser(ctx, "alpha", renamedUser); err != nil {
		t.Fatalf("SetSessionUser(alpha, anna) update error = %v", err)
	}

	users, err = s.ListSessionUsers(ctx)
	if err != nil {
		t.Fatalf("ListSessionUsers() after set error = %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("len(users) after set = %d, want 2", len(users))
	}
	if users["alpha"] != renamedUser {
		t.Fatalf("users[alpha] = %q, want %s", users["alpha"], renamedUser)
	}
	if users["beta"] != "bob" {
		t.Fatalf("users[beta] = %q, want bob", users["beta"])
	}

	if err := s.RenameSessionUser(ctx, "alpha", "gamma"); err != nil {
		t.Fatalf("RenameSessionUser(alpha, gamma) error = %v", err)
	}

	users, err = s.ListSessionUsers(ctx)
	if err != nil {
		t.Fatalf("ListSessionUsers() after rename error = %v", err)
	}
	if _, ok := users["alpha"]; ok {
		t.Fatalf("users[alpha] still exists after rename")
	}
	if users["gamma"] != renamedUser {
		t.Fatalf("users[gamma] = %q, want %s", users["gamma"], renamedUser)
	}

	if err := s.DeleteSessionUser(ctx, "beta"); err != nil {
		t.Fatalf("DeleteSessionUser(beta) error = %v", err)
	}

	users, err = s.ListSessionUsers(ctx)
	if err != nil {
		t.Fatalf("ListSessionUsers() after delete error = %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("len(users) after delete = %d, want 1", len(users))
	}
	if users["gamma"] != renamedUser {
		t.Fatalf("users[gamma] after delete = %q, want %s", users["gamma"], renamedUser)
	}
}

func TestRenameSessionUserMissingSessionIsNoOp(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	if err := s.SetSessionUser(ctx, "present", "alice"); err != nil {
		t.Fatalf("SetSessionUser(present, alice) error = %v", err)
	}
	if err := s.RenameSessionUser(ctx, "ghost", "renamed"); err != nil {
		t.Fatalf("RenameSessionUser(ghost, renamed) error = %v", err)
	}

	users, err := s.ListSessionUsers(ctx)
	if err != nil {
		t.Fatalf("ListSessionUsers() error = %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("len(users) = %d, want 1", len(users))
	}
	if users["present"] != "alice" {
		t.Fatalf("users[present] = %q, want alice", users["present"])
	}
}

func TestRenameSessionUserOverwritesDestination(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	if err := s.SetSessionUser(ctx, "old", "alice"); err != nil {
		t.Fatalf("SetSessionUser(old, alice) error = %v", err)
	}
	if err := s.SetSessionUser(ctx, "new", "bob"); err != nil {
		t.Fatalf("SetSessionUser(new, bob) error = %v", err)
	}
	if err := s.RenameSessionUser(ctx, "old", "new"); err != nil {
		t.Fatalf("RenameSessionUser(old, new) error = %v", err)
	}

	users, err := s.ListSessionUsers(ctx)
	if err != nil {
		t.Fatalf("ListSessionUsers() error = %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("len(users) = %d, want 1", len(users))
	}
	if users["new"] != "alice" {
		t.Fatalf("users[new] = %q, want alice", users["new"])
	}
}

func TestSessionUsersNilStoreIsNoOp(t *testing.T) {
	t.Parallel()

	var s *Store
	ctx := context.Background()

	if err := s.SetSessionUser(ctx, "alpha", "alice"); err != nil {
		t.Fatalf("SetSessionUser() on nil store error = %v", err)
	}
	if err := s.DeleteSessionUser(ctx, "alpha"); err != nil {
		t.Fatalf("DeleteSessionUser() on nil store error = %v", err)
	}
	if err := s.RenameSessionUser(ctx, "alpha", "beta"); err != nil {
		t.Fatalf("RenameSessionUser() on nil store error = %v", err)
	}

	users, err := s.ListSessionUsers(ctx)
	if err != nil {
		t.Fatalf("ListSessionUsers() on nil store error = %v", err)
	}
	if users != nil {
		t.Fatalf("ListSessionUsers() on nil store = %#v, want nil", users)
	}
}
