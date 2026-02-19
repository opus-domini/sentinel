package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type OpsCustomService struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Manager     string `json:"manager"`
	Unit        string `json:"unit"`
	Scope       string `json:"scope"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type OpsCustomServiceWrite struct {
	Name        string
	DisplayName string
	Manager     string
	Unit        string
	Scope       string
}

func (s *Store) initCustomServicesSchema() error {
	_, err := s.db.ExecContext(context.Background(), `CREATE TABLE IF NOT EXISTS ops_custom_services (
		name         TEXT PRIMARY KEY,
		display_name TEXT NOT NULL,
		manager      TEXT NOT NULL DEFAULT 'systemd',
		unit         TEXT NOT NULL,
		scope        TEXT NOT NULL DEFAULT 'user',
		enabled      INTEGER NOT NULL DEFAULT 1,
		created_at   TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at   TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	return err
}

func (s *Store) InsertOpsCustomService(ctx context.Context, w OpsCustomServiceWrite) (OpsCustomService, error) {
	name := strings.TrimSpace(w.Name)
	if name == "" {
		return OpsCustomService{}, fmt.Errorf("service name is required")
	}
	displayName := strings.TrimSpace(w.DisplayName)
	if displayName == "" {
		displayName = name
	}
	manager := strings.ToLower(strings.TrimSpace(w.Manager))
	if manager == "" {
		manager = "systemd"
	}
	unit := strings.TrimSpace(w.Unit)
	if unit == "" {
		return OpsCustomService{}, fmt.Errorf("service unit is required")
	}
	scope := strings.ToLower(strings.TrimSpace(w.Scope))
	if scope == "" {
		scope = "user"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO ops_custom_services (
		name, display_name, manager, unit, scope, enabled, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
		name, displayName, manager, unit, scope, now, now,
	); err != nil {
		return OpsCustomService{}, err
	}
	return OpsCustomService{
		Name:        name,
		DisplayName: displayName,
		Manager:     manager,
		Unit:        unit,
		Scope:       scope,
		Enabled:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (s *Store) ListOpsCustomServices(ctx context.Context) ([]OpsCustomService, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT
		name, display_name, manager, unit, scope, enabled, created_at, updated_at
	FROM ops_custom_services
	WHERE enabled = 1
	ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]OpsCustomService, 0, 8)
	for rows.Next() {
		var item OpsCustomService
		var enabled int
		if err := rows.Scan(
			&item.Name, &item.DisplayName, &item.Manager,
			&item.Unit, &item.Scope, &enabled,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Enabled = enabled == 1
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) DeleteOpsCustomService(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return sql.ErrNoRows
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM ops_custom_services WHERE name = ?", name)
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
