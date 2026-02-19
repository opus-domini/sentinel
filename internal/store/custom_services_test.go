package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

const testManager = "systemd"

func TestInsertOpsCustomService(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	t.Run("happy path", func(t *testing.T) {
		svc, err := s.InsertOpsCustomService(ctx, OpsCustomServiceWrite{
			Name:        "nginx",
			DisplayName: "Nginx Web Server",
			Manager:     testManager,
			Unit:        "nginx.service",
			Scope:       "system",
		})
		if err != nil {
			t.Fatalf("InsertOpsCustomService: %v", err)
		}
		if svc.Name != "nginx" {
			t.Fatalf("name = %q, want nginx", svc.Name)
		}
		if svc.DisplayName != "Nginx Web Server" {
			t.Fatalf("displayName = %q, want Nginx Web Server", svc.DisplayName)
		}
		if svc.Manager != testManager {
			t.Fatalf("manager = %q, want systemd", svc.Manager)
		}
		if svc.Unit != "nginx.service" {
			t.Fatalf("unit = %q, want nginx.service", svc.Unit)
		}
		if svc.Scope != "system" {
			t.Fatalf("scope = %q, want system", svc.Scope)
		}
		if !svc.Enabled {
			t.Fatalf("enabled = false, want true")
		}
		if svc.CreatedAt == "" || svc.UpdatedAt == "" {
			t.Fatalf("timestamps should be set")
		}
	})

	t.Run("defaults applied", func(t *testing.T) {
		svc, err := s.InsertOpsCustomService(ctx, OpsCustomServiceWrite{
			Name: "redis",
			Unit: "redis.service",
		})
		if err != nil {
			t.Fatalf("InsertOpsCustomService: %v", err)
		}
		if svc.DisplayName != "redis" {
			t.Fatalf("displayName should default to name, got %q", svc.DisplayName)
		}
		if svc.Manager != testManager {
			t.Fatalf("manager should default to systemd, got %q", svc.Manager)
		}
		if svc.Scope != "user" {
			t.Fatalf("scope should default to user, got %q", svc.Scope)
		}
	})

	t.Run("empty name errors", func(t *testing.T) {
		_, err := s.InsertOpsCustomService(ctx, OpsCustomServiceWrite{
			Name: "",
			Unit: "foo.service",
		})
		if err == nil {
			t.Fatalf("expected error for empty name")
		}
	})

	t.Run("empty unit errors", func(t *testing.T) {
		_, err := s.InsertOpsCustomService(ctx, OpsCustomServiceWrite{
			Name: "foo",
			Unit: "",
		})
		if err == nil {
			t.Fatalf("expected error for empty unit")
		}
	})

	t.Run("duplicate name errors", func(t *testing.T) {
		_, err := s.InsertOpsCustomService(ctx, OpsCustomServiceWrite{
			Name: "nginx",
			Unit: "nginx2.service",
		})
		if err == nil {
			t.Fatalf("expected error for duplicate name")
		}
	})
}

func TestListOpsCustomServices(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	// Empty list.
	list, err := s.ListOpsCustomServices(ctx)
	if err != nil {
		t.Fatalf("ListOpsCustomServices(empty): %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("len = %d, want 0", len(list))
	}

	// Insert two services.
	for _, w := range []OpsCustomServiceWrite{
		{Name: "beta", Unit: "beta.service"},
		{Name: "alpha", Unit: "alpha.service"},
	} {
		if _, err := s.InsertOpsCustomService(ctx, w); err != nil {
			t.Fatalf("InsertOpsCustomService(%s): %v", w.Name, err)
		}
	}

	list, err = s.ListOpsCustomServices(ctx)
	if err != nil {
		t.Fatalf("ListOpsCustomServices: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	// Sorted by name ASC.
	if list[0].Name != "alpha" || list[1].Name != "beta" {
		t.Fatalf("services not sorted: [%s, %s]", list[0].Name, list[1].Name)
	}
}

func TestDeleteOpsCustomService(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	ctx := context.Background()

	t.Run("delete existing", func(t *testing.T) {
		if _, err := s.InsertOpsCustomService(ctx, OpsCustomServiceWrite{
			Name: "to-delete",
			Unit: "to-delete.service",
		}); err != nil {
			t.Fatalf("InsertOpsCustomService: %v", err)
		}

		if err := s.DeleteOpsCustomService(ctx, "to-delete"); err != nil {
			t.Fatalf("DeleteOpsCustomService: %v", err)
		}

		list, err := s.ListOpsCustomServices(ctx)
		if err != nil {
			t.Fatalf("ListOpsCustomServices: %v", err)
		}
		if len(list) != 0 {
			t.Fatalf("len = %d, want 0 after delete", len(list))
		}
	})

	t.Run("delete nonexistent returns ErrNoRows", func(t *testing.T) {
		err := s.DeleteOpsCustomService(ctx, "ghost")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})

	t.Run("delete empty name returns ErrNoRows", func(t *testing.T) {
		err := s.DeleteOpsCustomService(ctx, "")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("error = %v, want sql.ErrNoRows", err)
		}
	})
}
