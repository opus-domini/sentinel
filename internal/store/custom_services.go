package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// CustomService represents a user-registered service tracked by Sentinel.
type CustomService struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Manager     string `json:"manager"`
	Unit        string `json:"unit"`
	Scope       string `json:"scope"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// CustomServiceWrite contains the fields needed to register a custom service.
type CustomServiceWrite struct {
	Name        string
	DisplayName string
	Manager     string
	Unit        string
	Scope       string
}

func (s *Store) InsertCustomService(ctx context.Context, w CustomServiceWrite) (CustomService, error) {
	name := strings.TrimSpace(w.Name)
	if name == "" {
		return CustomService{}, fmt.Errorf("service name is required")
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
		return CustomService{}, fmt.Errorf("service unit is required")
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
		return CustomService{}, err
	}
	return CustomService{
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

func (s *Store) ListCustomServices(ctx context.Context) ([]CustomService, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT
		name, display_name, manager, unit, scope, enabled, created_at, updated_at
	FROM ops_custom_services
	WHERE enabled = 1
	ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]CustomService, 0, 8)
	for rows.Next() {
		var item CustomService
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

func (s *Store) DeleteCustomService(ctx context.Context, name string) error {
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
