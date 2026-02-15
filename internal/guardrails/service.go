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

	winningRank := decisionRank(store.GuardrailModeAllow)
	winningRuleID := ""
	winningMessage := ""
	winningMode := store.GuardrailModeAllow
	matched := make([]store.GuardrailRule, 0, 4)

	action := strings.TrimSpace(input.Action)
	command := strings.TrimSpace(input.Command)

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		target := command
		if strings.EqualFold(rule.Scope, store.GuardrailScopeAction) {
			target = action
		}
		if strings.TrimSpace(target) == "" {
			continue
		}
		matchedRule, compileErr := ruleMatches(rule, target)
		if compileErr != nil {
			slog.Warn("guardrail regex compile failed", "rule", rule.ID, "pattern", rule.Pattern, "err", compileErr)
			continue
		}
		if !matchedRule {
			continue
		}

		matched = append(matched, rule)
		mode := store.GuardrailModeAllow
		if strings.TrimSpace(rule.Mode) != "" {
			mode = rule.Mode
		}
		rank := decisionRank(mode)
		if rank > winningRank {
			winningRank = rank
			winningMode = mode
			winningRuleID = rule.ID
			winningMessage = strings.TrimSpace(rule.Message)
		}
	}

	if strings.TrimSpace(winningMessage) == "" {
		switch winningMode {
		case store.GuardrailModeBlock:
			winningMessage = "operation blocked by guardrail policy"
		case store.GuardrailModeConfirm:
			winningMessage = "operation requires explicit confirmation"
		case store.GuardrailModeWarn:
			winningMessage = "operation matched warning policy"
		default:
			winningMessage = ""
		}
	}

	decision := Decision{
		Mode:           winningMode,
		Allowed:        winningMode != store.GuardrailModeBlock,
		RequireConfirm: winningMode == store.GuardrailModeConfirm,
		Message:        winningMessage,
		MatchedRuleID:  winningRuleID,
		MatchedRules:   matched,
	}
	return decision, nil
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

func ruleMatches(rule store.GuardrailRule, target string) (bool, error) {
	pattern := strings.TrimSpace(rule.Pattern)
	if pattern == "" {
		return false, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(target), nil
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
