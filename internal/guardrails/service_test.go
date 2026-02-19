package guardrails

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/opus-domini/sentinel/internal/store"
)

func newGuardrailTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "sentinel.db")
	st, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestEvaluateCommandBlockRule(t *testing.T) {
	t.Parallel()

	st := newGuardrailTestStore(t)
	svc := New(st)

	decision, err := svc.Evaluate(context.Background(), Input{
		Command: "rm -rf /",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.Mode != store.GuardrailModeBlock {
		t.Fatalf("decision.Mode = %q, want block", decision.Mode)
	}
	if decision.Allowed {
		t.Fatalf("decision.Allowed = true, want false")
	}
}

func TestEvaluateActionConfirmRule(t *testing.T) {
	t.Parallel()

	st := newGuardrailTestStore(t)
	svc := New(st)

	decision, err := svc.Evaluate(context.Background(), Input{
		Action: "session.kill",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.Mode != store.GuardrailModeConfirm {
		t.Fatalf("decision.Mode = %q, want confirm", decision.Mode)
	}
	if !decision.RequireConfirm {
		t.Fatalf("decision.RequireConfirm = false, want true")
	}
	if !decision.Allowed {
		t.Fatalf("decision.Allowed = false, want true (with explicit confirm)")
	}
}

func TestRecordAudit(t *testing.T) {
	t.Parallel()

	st := newGuardrailTestStore(t)
	svc := New(st)

	decision := Decision{
		Mode:          store.GuardrailModeConfirm,
		Allowed:       true,
		MatchedRuleID: "action.session.kill.confirm",
	}
	if err := svc.RecordAudit(context.Background(), Input{
		Action:      "session.kill",
		SessionName: "dev",
		WindowIndex: -1,
	}, decision, true, "confirmed by user"); err != nil {
		t.Fatalf("RecordAudit: %v", err)
	}

	rows, err := svc.ListAudit(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	if rows[0].RuleID != "action.session.kill.confirm" || !rows[0].Override {
		t.Fatalf("unexpected audit row: %+v", rows[0])
	}
}
