package guardrails

import (
	"context"
	"errors"
	"testing"

	"github.com/opus-domini/sentinel/internal/store"
)

func TestDefaultDecisionMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode string
		want string
	}{
		{"block", store.GuardrailModeBlock, "operation blocked by guardrail policy"},
		{"confirm", store.GuardrailModeConfirm, "operation requires explicit confirmation"},
		{"warn", store.GuardrailModeWarn, "operation matched warning policy"},
		{"empty", "", ""},
		{"unknown", "unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := defaultDecisionMessage(tt.mode)
			if got != tt.want {
				t.Errorf("defaultDecisionMessage(%q) = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

func TestListRules(t *testing.T) {
	t.Parallel()

	st := newGuardrailTestStore(t)
	svc := New(st)

	rules, err := svc.ListRules(context.Background())
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	// Default rules should exist from schema seeding.
	if len(rules) < 2 {
		t.Fatalf("len(rules) = %d, want >= 2", len(rules))
	}
}

func TestListRulesNilService(t *testing.T) {
	t.Parallel()

	var svc *Service
	rules, err := svc.ListRules(context.Background())
	if err != nil {
		t.Fatalf("ListRules on nil service: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected empty rules on nil service, got %d", len(rules))
	}
}

func TestUpsertAndDeleteRule(t *testing.T) {
	t.Parallel()

	st := newGuardrailTestStore(t)
	svc := New(st)
	ctx := context.Background()

	rule := store.GuardrailRuleWrite{
		ID:       "test.custom.block",
		Name:     "Block rm -rf",
		Scope:    store.GuardrailScopeAction,
		Pattern:  `rm\s+-rf`,
		Mode:     store.GuardrailModeBlock,
		Severity: "critical",
		Message:  "rm -rf is blocked",
		Enabled:  true,
		Priority: 100,
	}
	if err := svc.UpsertRule(ctx, rule); err != nil {
		t.Fatalf("UpsertRule: %v", err)
	}

	rules, err := svc.ListRules(ctx)
	if err != nil {
		t.Fatalf("ListRules after upsert: %v", err)
	}
	found := false
	for _, r := range rules {
		if r.ID == "test.custom.block" {
			found = true
			if r.Mode != store.GuardrailModeBlock {
				t.Fatalf("mode = %q, want block", r.Mode)
			}
		}
	}
	if !found {
		t.Fatal("custom rule not found after upsert")
	}

	if err := svc.DeleteRule(ctx, "test.custom.block"); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}

	rules, err = svc.ListRules(ctx)
	if err != nil {
		t.Fatalf("ListRules after delete: %v", err)
	}
	for _, r := range rules {
		if r.ID == "test.custom.block" {
			t.Fatal("rule still exists after delete")
		}
	}
}

func TestUpsertRuleNilService(t *testing.T) {
	t.Parallel()

	var svc *Service
	if err := svc.UpsertRule(context.Background(), store.GuardrailRuleWrite{}); err != nil {
		t.Fatalf("UpsertRule on nil service: %v", err)
	}
}

func TestDeleteRuleNilService(t *testing.T) {
	t.Parallel()

	var svc *Service
	if err := svc.DeleteRule(context.Background(), "nonexistent"); err != nil {
		t.Fatalf("DeleteRule on nil service: %v", err)
	}
}

func TestEvaluateBlockRule(t *testing.T) {
	t.Parallel()

	st := newGuardrailTestStore(t)
	svc := New(st)
	ctx := context.Background()

	// Add a blocking rule.
	if err := svc.UpsertRule(ctx, store.GuardrailRuleWrite{
		ID:       "test.block.dangerous",
		Name:     "Block dangerous action",
		Scope:    store.GuardrailScopeAction,
		Pattern:  `^dangerous\.action$`,
		Mode:     store.GuardrailModeBlock,
		Severity: "critical",
		Message:  "This action is blocked",
		Enabled:  true,
		Priority: 100,
	}); err != nil {
		t.Fatalf("UpsertRule: %v", err)
	}

	decision, err := svc.Evaluate(ctx, Input{Action: "dangerous.action"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.Mode != store.GuardrailModeBlock {
		t.Fatalf("mode = %q, want block", decision.Mode)
	}
	if decision.Allowed {
		t.Fatal("allowed = true, want false for blocked action")
	}
	if decision.Message != "This action is blocked" {
		t.Fatalf("message = %q, want custom message", decision.Message)
	}
}

func TestEvaluateNoMatchAllowed(t *testing.T) {
	t.Parallel()

	st := newGuardrailTestStore(t)
	svc := New(st)

	decision, err := svc.Evaluate(context.Background(), Input{Action: "nonexistent.action.xyz"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !decision.Allowed {
		t.Fatal("allowed = false, want true for unmatched action")
	}
}

func TestEvaluateEmptyAction(t *testing.T) {
	t.Parallel()

	st := newGuardrailTestStore(t)
	svc := New(st)

	decision, err := svc.Evaluate(context.Background(), Input{Action: ""})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !decision.Allowed {
		t.Fatal("allowed = false, want true for empty action")
	}
}

func TestEvaluateNilService(t *testing.T) {
	t.Parallel()

	var svc *Service
	decision, err := svc.Evaluate(context.Background(), Input{Action: "any"})
	if err != nil {
		t.Fatalf("Evaluate on nil service: %v", err)
	}
	if !decision.Allowed {
		t.Fatal("allowed = false, want true from nil service")
	}
}

type failingRepo struct{}

func (r *failingRepo) ListGuardrailRules(context.Context) ([]store.GuardrailRule, error) {
	return nil, errors.New("db error")
}
func (r *failingRepo) UpsertGuardrailRule(context.Context, store.GuardrailRuleWrite) error {
	return nil
}
func (r *failingRepo) DeleteGuardrailRule(context.Context, string) error { return nil }
func (r *failingRepo) ListGuardrailAudit(context.Context, int) ([]store.GuardrailAudit, error) {
	return nil, nil
}
func (r *failingRepo) InsertGuardrailAudit(context.Context, store.GuardrailAuditWrite) (int64, error) {
	return 0, nil
}

func TestEvaluateRepoError(t *testing.T) {
	t.Parallel()

	svc := New(&failingRepo{})
	_, err := svc.Evaluate(context.Background(), Input{Action: "any"})
	if err == nil {
		t.Fatal("expected error from failing repo")
	}
}

func TestDecisionRank(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mode string
		want int
	}{
		{store.GuardrailModeBlock, 4},
		{store.GuardrailModeConfirm, 3},
		{store.GuardrailModeWarn, 2},
		{"unknown", 1},
		{"", 1},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			t.Parallel()
			got := decisionRank(tt.mode)
			if got != tt.want {
				t.Errorf("decisionRank(%q) = %d, want %d", tt.mode, got, tt.want)
			}
		})
	}
}

func TestEvaluateDefaultMessageFallback(t *testing.T) {
	t.Parallel()

	st := newGuardrailTestStore(t)
	svc := New(st)
	ctx := context.Background()

	// Add a warn rule with an empty message so defaultDecisionMessage is used.
	if err := svc.UpsertRule(ctx, store.GuardrailRuleWrite{
		ID:       "test.empty.msg",
		Name:     "Warn with no message",
		Scope:    store.GuardrailScopeAction,
		Pattern:  `^special\.action$`,
		Mode:     store.GuardrailModeWarn,
		Severity: "info",
		Message:  "",
		Enabled:  true,
		Priority: 50,
	}); err != nil {
		t.Fatalf("UpsertRule: %v", err)
	}

	decision, err := svc.Evaluate(ctx, Input{Action: "special.action"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.Message != "operation matched warning policy" {
		t.Fatalf("message = %q, want default warn message", decision.Message)
	}
}
