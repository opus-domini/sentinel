package store

import (
	"context"
	"testing"
	"time"
)

func TestGuardrailSchemaSeedsDefaultRules(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()

	rules, err := s.ListGuardrailRules(context.Background())
	if err != nil {
		t.Fatalf("ListGuardrailRules: %v", err)
	}
	if len(rules) < 2 {
		t.Fatalf("rules len = %d, want >= 2 defaults", len(rules))
	}

	foundSessionKill := false
	foundDangerousRM := false
	for _, rule := range rules {
		switch rule.ID {
		case "action.session.kill.confirm":
			foundSessionKill = true
			if rule.Mode != GuardrailModeConfirm {
				t.Fatalf("session kill mode = %q, want confirm", rule.Mode)
			}
		case "command.rm.root.block":
			foundDangerousRM = true
			if rule.Mode != GuardrailModeBlock {
				t.Fatalf("rm block mode = %q, want block", rule.Mode)
			}
		}
	}
	if !foundSessionKill || !foundDangerousRM {
		t.Fatalf("missing default rules, sessionKill=%v rmBlock=%v", foundSessionKill, foundDangerousRM)
	}
}

func TestGuardrailRuleUpsert(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	if err := s.UpsertGuardrailRule(ctx, GuardrailRuleWrite{
		ID:       "action.window.kill.warn",
		Name:     "Warn on window kill",
		Scope:    GuardrailScopeAction,
		Pattern:  "^window\\.kill$",
		Mode:     GuardrailModeWarn,
		Severity: "info",
		Message:  "Window kill warning",
		Enabled:  true,
		Priority: 30,
	}); err != nil {
		t.Fatalf("UpsertGuardrailRule(create): %v", err)
	}

	if err := s.UpsertGuardrailRule(ctx, GuardrailRuleWrite{
		ID:       "action.window.kill.warn",
		Name:     "Confirm window kill",
		Scope:    GuardrailScopeAction,
		Pattern:  "^window\\.kill$",
		Mode:     GuardrailModeConfirm,
		Severity: "warn",
		Message:  "Window kill confirm",
		Enabled:  true,
		Priority: 12,
	}); err != nil {
		t.Fatalf("UpsertGuardrailRule(update): %v", err)
	}

	rules, err := s.ListGuardrailRules(ctx)
	if err != nil {
		t.Fatalf("ListGuardrailRules: %v", err)
	}
	var found *GuardrailRule
	for i := range rules {
		if rules[i].ID == "action.window.kill.warn" {
			found = &rules[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected rule action.window.kill.warn in %+v", rules)
		return
	}
	if found.Mode != GuardrailModeConfirm {
		t.Fatalf("rule mode = %q, want confirm", found.Mode)
	}
	if found.Priority != 12 {
		t.Fatalf("rule priority = %d, want 12", found.Priority)
	}
}

func TestGuardrailAuditInsertAndList(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := s.InsertGuardrailAudit(ctx, GuardrailAuditWrite{
		RuleID:      "action.session.kill.confirm",
		Decision:    GuardrailModeConfirm,
		Action:      "session.kill",
		SessionName: "dev",
		WindowIndex: -1,
		PaneID:      "",
		Override:    false,
		Reason:      "confirm required",
		MetadataRaw: `{"source":"api"}`,
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("InsertGuardrailAudit: %v", err)
	}

	rows, err := s.ListGuardrailAudit(ctx, 10)
	if err != nil {
		t.Fatalf("ListGuardrailAudit: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	if rows[0].Action != "session.kill" || rows[0].Decision != GuardrailModeConfirm {
		t.Fatalf("unexpected audit row: %+v", rows[0])
	}
}
