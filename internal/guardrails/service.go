package guardrails

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/store"
)

type Input struct {
	Action      string `json:"action"`
	Command     string `json:"command"`
	SessionName string `json:"sessionName"`
	WindowIndex int    `json:"windowIndex"`
	PaneID      string `json:"paneId"`
}

type Decision struct {
	Mode           string                `json:"mode"`
	Allowed        bool                  `json:"allowed"`
	RequireConfirm bool                  `json:"requireConfirm"`
	Message        string                `json:"message"`
	MatchedRuleID  string                `json:"matchedRuleId"`
	MatchedRules   []store.GuardrailRule `json:"matchedRules"`
}

type Service struct {
	store *store.Store
}

type evaluatedSelection struct {
	mode    string
	ruleID  string
	message string
	matched []store.GuardrailRule
}

func New(st *store.Store) *Service {
	return &Service{store: st}
}

func (s *Service) Evaluate(ctx context.Context, input Input) (Decision, error) {
	defaultDecision := Decision{
		Mode:    store.GuardrailModeAllow,
		Allowed: true,
	}
	if s == nil || s.store == nil {
		return defaultDecision, nil
	}

	rules, err := s.store.ListGuardrailRules(ctx)
	if err != nil {
		return Decision{}, err
	}

	compiled := compileRulePatterns(rules)
	selection := evaluateSelection(rules, input, compiled)
	message := strings.TrimSpace(selection.message)
	if message == "" {
		message = defaultDecisionMessage(selection.mode)
	}

	decision := Decision{
		Mode:           selection.mode,
		Allowed:        selection.mode != store.GuardrailModeBlock,
		RequireConfirm: selection.mode == store.GuardrailModeConfirm,
		Message:        message,
		MatchedRuleID:  selection.ruleID,
		MatchedRules:   selection.matched,
	}
	return decision, nil
}

// compileRulePatterns pre-compiles regex patterns for all rules.
// Rules with invalid patterns are skipped with a warning log.
func compileRulePatterns(rules []store.GuardrailRule) map[string]*regexp.Regexp {
	compiled := make(map[string]*regexp.Regexp, len(rules))
	for _, rule := range rules {
		pattern := strings.TrimSpace(rule.Pattern)
		if pattern == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			slog.Warn("guardrail regex pre-compile failed, skipping rule", "rule", rule.ID, "pattern", pattern, "err", err)
			continue
		}
		compiled[rule.ID] = re
	}
	return compiled
}

func evaluateSelection(rules []store.GuardrailRule, input Input, compiled map[string]*regexp.Regexp) evaluatedSelection {
	selection := evaluatedSelection{
		mode:    store.GuardrailModeAllow,
		matched: make([]store.GuardrailRule, 0, 4),
	}
	winningRank := decisionRank(store.GuardrailModeAllow)
	action := strings.TrimSpace(input.Action)
	command := strings.TrimSpace(input.Command)

	for _, rule := range rules {
		mode, ok := evaluateRuleMatch(rule, action, command, compiled)
		if !ok {
			continue
		}
		selection.matched = append(selection.matched, rule)
		rank := decisionRank(mode)
		if rank <= winningRank {
			continue
		}
		winningRank = rank
		selection.mode = mode
		selection.ruleID = rule.ID
		selection.message = strings.TrimSpace(rule.Message)
	}
	return selection
}

func evaluateRuleMatch(rule store.GuardrailRule, action, command string, compiled map[string]*regexp.Regexp) (string, bool) {
	if !rule.Enabled {
		return "", false
	}

	target := command
	if strings.EqualFold(rule.Scope, store.GuardrailScopeAction) {
		target = action
	}
	if strings.TrimSpace(target) == "" {
		return "", false
	}

	re, ok := compiled[rule.ID]
	if !ok {
		return "", false
	}
	if !re.MatchString(target) {
		return "", false
	}

	mode := store.GuardrailModeAllow
	if strings.TrimSpace(rule.Mode) != "" {
		mode = rule.Mode
	}
	return mode, true
}

func defaultDecisionMessage(mode string) string {
	switch mode {
	case store.GuardrailModeBlock:
		return "operation blocked by guardrail policy"
	case store.GuardrailModeConfirm:
		return "operation requires explicit confirmation"
	case store.GuardrailModeWarn:
		return "operation matched warning policy"
	default:
		return ""
	}
}

func (s *Service) ListRules(ctx context.Context) ([]store.GuardrailRule, error) {
	if s == nil || s.store == nil {
		return []store.GuardrailRule{}, nil
	}
	return s.store.ListGuardrailRules(ctx)
}

func (s *Service) UpsertRule(ctx context.Context, rule store.GuardrailRuleWrite) error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.UpsertGuardrailRule(ctx, rule)
}

func (s *Service) ListAudit(ctx context.Context, limit int) ([]store.GuardrailAudit, error) {
	if s == nil || s.store == nil {
		return []store.GuardrailAudit{}, nil
	}
	return s.store.ListGuardrailAudit(ctx, limit)
}

func (s *Service) RecordAudit(ctx context.Context, input Input, decision Decision, override bool, reason string) error {
	if s == nil || s.store == nil {
		return nil
	}
	metadata := map[string]any{
		"matchedRules": len(decision.MatchedRules),
		"mode":         decision.Mode,
	}
	metadataRaw, _ := json.Marshal(metadata)
	_, err := s.store.InsertGuardrailAudit(ctx, store.GuardrailAuditWrite{
		RuleID:      strings.TrimSpace(decision.MatchedRuleID),
		Decision:    decision.Mode,
		Action:      strings.TrimSpace(input.Action),
		Command:     strings.TrimSpace(input.Command),
		SessionName: strings.TrimSpace(input.SessionName),
		WindowIndex: input.WindowIndex,
		PaneID:      strings.TrimSpace(input.PaneID),
		Override:    override,
		Reason:      strings.TrimSpace(reason),
		MetadataRaw: string(metadataRaw),
		CreatedAt:   time.Now().UTC(),
	})
	return err
}

func decisionRank(mode string) int {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case store.GuardrailModeBlock:
		return 4
	case store.GuardrailModeConfirm:
		return 3
	case store.GuardrailModeWarn:
		return 2
	default:
		return 1
	}
}
