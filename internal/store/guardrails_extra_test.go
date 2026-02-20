package store

import (
	"context"
	"database/sql"
	"testing"
)

func TestDeleteGuardrailRule(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	// First upsert a custom rule.
	if err := s.UpsertGuardrailRule(ctx, GuardrailRuleWrite{
		ID:       "test.delete.rule",
		Name:     "To Delete",
		Scope:    GuardrailScopeAction,
		Pattern:  "^delete\\.test$",
		Mode:     GuardrailModeWarn,
		Severity: "info",
		Message:  "test",
		Enabled:  true,
		Priority: 10,
	}); err != nil {
		t.Fatalf("UpsertGuardrailRule: %v", err)
	}

	// Delete it.
	if err := s.DeleteGuardrailRule(ctx, "test.delete.rule"); err != nil {
		t.Fatalf("DeleteGuardrailRule: %v", err)
	}

	// Verify it's gone.
	rules, err := s.ListGuardrailRules(ctx)
	if err != nil {
		t.Fatalf("ListGuardrailRules: %v", err)
	}
	for _, r := range rules {
		if r.ID == "test.delete.rule" {
			t.Fatal("rule still exists after delete")
		}
	}
}

func TestDeleteGuardrailRuleEmptyID(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	err := s.DeleteGuardrailRule(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestDeleteGuardrailRuleNonexistent(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	err := s.DeleteGuardrailRule(context.Background(), "nonexistent-rule-id")
	if err != sql.ErrNoRows {
		t.Fatalf("error = %v, want sql.ErrNoRows", err)
	}
}
