package runbook

import (
	"strings"
	"testing"

	"github.com/opus-domini/sentinel/internal/store"
)

func TestValidateDefinition(t *testing.T) {
	t.Parallel()
	valid := store.OpsRunbookWrite{
		Name:  "deploy",
		Steps: []store.OpsRunbookStep{{Type: "run", Title: "deploy", Command: "deploy {{ENV}}"}},
		Parameters: []store.RunbookParameter{{
			Name: "ENV", Type: "select", Required: true, Default: "staging", Options: []string{"staging", "production"},
		}},
		WebhookURL: "https://example.test/hook",
	}
	if err := ValidateDefinition(valid); err != nil {
		t.Fatalf("ValidateDefinition(valid) error = %v", err)
	}

	tests := []struct {
		name string
		edit func(*store.OpsRunbookWrite)
		want string
	}{
		{name: "no steps", edit: func(w *store.OpsRunbookWrite) { w.Steps = nil }, want: "at least one step"},
		{name: "run command", edit: func(w *store.OpsRunbookWrite) { w.Steps[0].Command = "" }, want: "command is required"},
		{name: "duplicate parameter", edit: func(w *store.OpsRunbookWrite) { w.Parameters = append(w.Parameters, w.Parameters[0]) }, want: "duplicated"},
		{name: "invalid default", edit: func(w *store.OpsRunbookWrite) { w.Parameters[0].Default = "unknown" }, want: "must be one of"},
		{name: "invalid webhook", edit: func(w *store.OpsRunbookWrite) { w.WebhookURL = "file:///tmp/hook" }, want: "http or https"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := valid
			candidate.Steps = append([]store.OpsRunbookStep(nil), valid.Steps...)
			candidate.Parameters = append([]store.RunbookParameter(nil), valid.Parameters...)
			tt.edit(&candidate)
			if err := ValidateDefinition(candidate); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateDefinition() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateParamsChecksTypedValues(t *testing.T) {
	t.Parallel()
	defs := []store.RunbookParameter{
		{Name: "COUNT", Type: "number", Required: true},
		{Name: "DRY_RUN", Type: "boolean"},
		{Name: "ENV", Type: "select", Options: []string{"staging", "production"}},
	}
	if err := ValidateParams(defs, map[string]string{"COUNT": "2.5", "DRY_RUN": "true", "ENV": "production"}); err != nil {
		t.Fatalf("ValidateParams(valid) error = %v", err)
	}
	if err := ValidateParams(defs, map[string]string{"COUNT": "many"}); err == nil || !strings.Contains(err.Error(), "number") {
		t.Fatalf("ValidateParams(number) error = %v", err)
	}
	if err := ValidateInputParams(defs, map[string]string{"COUNTT": "2"}); err == nil || !strings.Contains(err.Error(), "not defined") {
		t.Fatalf("ValidateInputParams(unknown) error = %v", err)
	}
}
