package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestTmuxSessionLeaseCRUDPersistsAcrossReopen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sentinel.db")
	st, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	lease := TmuxSessionLease{
		LeaseID:       "lease_001",
		SessionID:     "$7",
		SessionName:   "agent",
		User:          "deploy",
		Source:        TmuxSessionLeaseSourceMCP,
		State:         TmuxSessionLeaseActive,
		CreatedAt:     now,
		LastRenewedAt: now,
		ExpiresAt:     now.Add(2 * time.Hour),
		UpdatedAt:     now,
	}
	if err := st.CreateTmuxSessionLease(ctx, lease); err != nil {
		t.Fatalf("CreateTmuxSessionLease() error = %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	got, err := st.GetTmuxSessionLease(ctx, lease.LeaseID)
	if err != nil {
		t.Fatalf("GetTmuxSessionLease() error = %v", err)
	}
	if got.SessionID != "$7" || got.SessionName != "agent" || got.User != "deploy" ||
		!got.ExpiresAt.Equal(lease.ExpiresAt) {
		t.Fatalf("GetTmuxSessionLease() = %#v", got)
	}

	renewedAt := now.Add(time.Hour)
	if err := st.TouchTmuxSessionLease(ctx, lease.LeaseID, renewedAt, renewedAt.Add(2*time.Hour), renewedAt); err != nil {
		t.Fatalf("TouchTmuxSessionLease() error = %v", err)
	}
	graceUntil := renewedAt.Add(2*time.Hour + 10*time.Minute)
	if err := st.UpdateTmuxSessionLeaseState(
		ctx,
		lease.LeaseID,
		TmuxSessionLeaseGrace,
		renewedAt.Add(2*time.Hour),
		graceUntil,
		graceUntil.Add(-10*time.Minute),
	); err != nil {
		t.Fatalf("UpdateTmuxSessionLeaseState() error = %v", err)
	}
	if err := st.RenameTmuxSessionLease(ctx, lease.LeaseID, "agent-renamed", graceUntil); err != nil {
		t.Fatalf("RenameTmuxSessionLease() error = %v", err)
	}
	listed, err := st.ListTmuxSessionLeases(ctx)
	if err != nil {
		t.Fatalf("ListTmuxSessionLeases() error = %v", err)
	}
	if len(listed) != 1 || listed[0].State != TmuxSessionLeaseGrace ||
		listed[0].SessionName != "agent-renamed" || !listed[0].GraceUntil.Equal(graceUntil) {
		t.Fatalf("ListTmuxSessionLeases() = %#v", listed)
	}
	if err := st.DeleteTmuxSessionLease(ctx, lease.LeaseID); err != nil {
		t.Fatalf("DeleteTmuxSessionLease() error = %v", err)
	}
	if _, err := st.GetTmuxSessionLease(ctx, lease.LeaseID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetTmuxSessionLease() after delete error = %v", err)
	}
}

func TestTmuxSessionLeaseConstraints(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := newTestStore(t)
	t.Cleanup(func() { _ = st.Close() })
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	base := TmuxSessionLease{
		LeaseID:       "lease_base",
		SessionID:     "$1",
		SessionName:   "agent",
		Source:        TmuxSessionLeaseSourceMCP,
		State:         TmuxSessionLeaseActive,
		CreatedAt:     now,
		LastRenewedAt: now,
		ExpiresAt:     now.Add(2 * time.Hour),
		UpdatedAt:     now,
	}
	if err := st.CreateTmuxSessionLease(ctx, base); err != nil {
		t.Fatal(err)
	}
	duplicateID := base
	duplicateID.LeaseID = "lease_duplicate_id"
	duplicateID.SessionName = "other"
	if err := st.CreateTmuxSessionLease(ctx, duplicateID); err == nil {
		t.Fatal("duplicate user/session_id succeeded")
	}
	duplicateName := base
	duplicateName.LeaseID = "lease_duplicate_name"
	duplicateName.SessionID = "$2"
	if err := st.CreateTmuxSessionLease(ctx, duplicateName); err == nil {
		t.Fatal("duplicate user/session_name succeeded")
	}
	invalidSource := base
	invalidSource.LeaseID = "lease_source"
	invalidSource.SessionID = "$3"
	invalidSource.SessionName = "source"
	invalidSource.Source = "human"
	if err := st.CreateTmuxSessionLease(ctx, invalidSource); err == nil {
		t.Fatal("invalid source succeeded")
	}
	if err := st.UpdateTmuxSessionLeaseState(ctx, base.LeaseID, "expired", base.ExpiresAt, time.Time{}, now); err == nil {
		t.Fatal("invalid state succeeded")
	}
}
