package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

const (
	GuardrailScopeAction = "action"

	GuardrailModeWarn    = "warn"
	GuardrailModeConfirm = "confirm"
	GuardrailModeBlock   = "block"

	GuardrailSeverityInfo  = "info"
	GuardrailSeverityWarn  = "warn"
	GuardrailSeverityError = "error"
)

type GuardrailRule struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Scope     string    `json:"scope"`
	Pattern   string    `json:"pattern"`
	Mode      string    `json:"mode"`
	Severity  string    `json:"severity"`
	Message   string    `json:"message"`
	Enabled   bool      `json:"enabled"`
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type GuardrailRuleWrite struct {
	ID       string
	Name     string
	Scope    string
	Pattern  string
	Mode     string
	Severity string
	Message  string
	Enabled  bool
	Priority int
}

type GuardrailAudit struct {
	ID          int64     `json:"id"`
	RuleID      string    `json:"ruleId"`
	Decision    string    `json:"decision"`
	Action      string    `json:"action"`
	Command     string    `json:"command"`
	SessionName string    `json:"sessionName"`
	WindowIndex int       `json:"windowIndex"`
	PaneID      string    `json:"paneId"`
	Override    bool      `json:"override"`
	Reason      string    `json:"reason"`
	MetadataRaw string    `json:"metadata"`
	CreatedAt   time.Time `json:"createdAt"`
}

type GuardrailAuditWrite struct {
	RuleID      string
	Decision    string
	Action      string
	Command     string
	SessionName string
	WindowIndex int
	PaneID      string
	Override    bool
	Reason      string
	MetadataRaw string
	CreatedAt   time.Time
}

func (s *Store) initGuardrailSchema() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS guardrail_rules (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL DEFAULT '',
			scope      TEXT NOT NULL DEFAULT 'action',
			pattern    TEXT NOT NULL DEFAULT '',
			mode       TEXT NOT NULL DEFAULT 'warn',
			severity   TEXT NOT NULL DEFAULT 'warn',
			message    TEXT NOT NULL DEFAULT '',
			enabled    INTEGER NOT NULL DEFAULT 1,
			priority   INTEGER NOT NULL DEFAULT 100,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS guardrail_audit (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			rule_id      TEXT NOT NULL DEFAULT '',
			decision     TEXT NOT NULL DEFAULT '',
			action       TEXT NOT NULL DEFAULT '',
			command      TEXT NOT NULL DEFAULT '',
			session_name TEXT NOT NULL DEFAULT '',
			window_index INTEGER NOT NULL DEFAULT -1,
			pane_id      TEXT NOT NULL DEFAULT '',
			override     INTEGER NOT NULL DEFAULT 0,
			reason       TEXT NOT NULL DEFAULT '',
			metadata     TEXT NOT NULL DEFAULT '{}',
			created_at   TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_guardrail_rules_priority
			ON guardrail_rules (priority ASC, id ASC)`,
		`CREATE INDEX IF NOT EXISTS idx_guardrail_audit_created
			ON guardrail_audit (created_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_guardrail_audit_action
			ON guardrail_audit (action, created_at DESC, id DESC)`,
		`INSERT OR IGNORE INTO guardrail_rules(
			id, name, scope, pattern, mode, severity, message, enabled, priority
		) VALUES (
			'action.session.kill.confirm',
			'Confirm session kill',
			'action',
			'^session\.kill$',
			'confirm',
			'warn',
			'Session termination requires explicit confirmation.',
			1,
			10
		)`,
		`INSERT OR IGNORE INTO guardrail_rules(
			id, name, scope, pattern, mode, severity, message, enabled, priority
		) VALUES (
			'action.pane.kill.warn',
			'Warn on pane kill',
			'action',
			'^pane\.kill$',
			'warn',
			'warn',
			'Pane termination logged for audit.',
			1,
			20
		)`,
	}

	for _, stmt := range statements {
		if _, err := s.db.ExecContext(context.Background(), stmt); err != nil {
			return err
		}
	}

	// Migrate any legacy command-scope rules to action scope.
	if _, err := s.db.ExecContext(context.Background(),
		`UPDATE guardrail_rules SET scope = 'action' WHERE scope = 'command'`,
	); err != nil {
		return err
	}
	return nil
}

func (s *Store) ListGuardrailRules(ctx context.Context) ([]GuardrailRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, scope, pattern, mode, severity, message, enabled, priority, created_at, updated_at
		   FROM guardrail_rules
		  ORDER BY priority ASC, id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]GuardrailRule, 0, 16)
	for rows.Next() {
		var (
			row                            GuardrailRule
			enabledRaw                     int
			createdAtRaw, updatedAtRawText string
		)
		if err := rows.Scan(
			&row.ID,
			&row.Name,
			&row.Scope,
			&row.Pattern,
			&row.Mode,
			&row.Severity,
			&row.Message,
			&enabledRaw,
			&row.Priority,
			&createdAtRaw,
			&updatedAtRawText,
		); err != nil {
			return nil, err
		}
		row.Enabled = enabledRaw == 1
		row.CreatedAt = parseStoreTime(createdAtRaw)
		row.UpdatedAt = parseStoreTime(updatedAtRawText)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) UpsertGuardrailRule(ctx context.Context, row GuardrailRuleWrite) error {
	id := strings.TrimSpace(row.ID)
	if id == "" {
		return errors.New("rule id is required")
	}
	pattern := strings.TrimSpace(row.Pattern)
	if pattern == "" {
		return errors.New("pattern is required")
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO guardrail_rules(
			id, name, scope, pattern, mode, severity, message, enabled, priority, created_at, updated_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), ?)
		 ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			scope = excluded.scope,
			pattern = excluded.pattern,
			mode = excluded.mode,
			severity = excluded.severity,
			message = excluded.message,
			enabled = excluded.enabled,
			priority = excluded.priority,
			updated_at = excluded.updated_at`,
		id,
		strings.TrimSpace(row.Name),
		normalizeGuardrailScope(row.Scope),
		pattern,
		normalizeGuardrailMode(row.Mode),
		normalizeGuardrailSeverity(row.Severity),
		strings.TrimSpace(row.Message),
		boolToInt(row.Enabled),
		row.Priority,
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (s *Store) DeleteGuardrailRule(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("rule id is required")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM guardrail_rules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) InsertGuardrailAudit(ctx context.Context, row GuardrailAuditWrite) (int64, error) {
	createdAt := row.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	metadata := strings.TrimSpace(row.MetadataRaw)
	if metadata == "" {
		metadata = "{}"
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO guardrail_audit(
			rule_id, decision, action, command, session_name, window_index, pane_id,
			override, reason, metadata, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(row.RuleID),
		normalizeGuardrailMode(row.Decision),
		strings.TrimSpace(row.Action),
		strings.TrimSpace(row.Command),
		strings.TrimSpace(row.SessionName),
		row.WindowIndex,
		strings.TrimSpace(row.PaneID),
		boolToInt(row.Override),
		strings.TrimSpace(row.Reason),
		metadata,
		createdAt.Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) ListGuardrailAudit(ctx context.Context, limit int) ([]GuardrailAudit, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, rule_id, decision, action, command, session_name, window_index, pane_id,
		        override, reason, metadata, created_at
		   FROM guardrail_audit
		  ORDER BY created_at DESC, id DESC
		  LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]GuardrailAudit, 0, limit)
	for rows.Next() {
		var (
			row                    GuardrailAudit
			overrideRaw            int
			createdAtRaw, metadata string
		)
		if err := rows.Scan(
			&row.ID,
			&row.RuleID,
			&row.Decision,
			&row.Action,
			&row.Command,
			&row.SessionName,
			&row.WindowIndex,
			&row.PaneID,
			&overrideRaw,
			&row.Reason,
			&metadata,
			&createdAtRaw,
		); err != nil {
			return nil, err
		}
		row.Override = overrideRaw == 1
		row.MetadataRaw = strings.TrimSpace(metadata)
		if row.MetadataRaw == "" {
			row.MetadataRaw = "{}"
		}
		row.CreatedAt = parseStoreTime(createdAtRaw)
		out = append(out, row)
	}
	return out, rows.Err()
}

func normalizeGuardrailScope(_ string) string {
	return GuardrailScopeAction
}

func normalizeGuardrailMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case GuardrailModeBlock:
		return GuardrailModeBlock
	case GuardrailModeConfirm:
		return GuardrailModeConfirm
	case GuardrailModeWarn:
		return GuardrailModeWarn
	default:
		return GuardrailModeWarn
	}
}

func normalizeGuardrailSeverity(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case GuardrailSeverityError:
		return GuardrailSeverityError
	case GuardrailSeverityInfo:
		return GuardrailSeverityInfo
	default:
		return GuardrailSeverityWarn
	}
}
