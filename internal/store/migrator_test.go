package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRunMigrationsFreshDB(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	if err := runMigrations(ctx, db); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	// Verify schema_migrations was populated.
	var version int
	var name string
	if err := db.QueryRowContext(ctx,
		"SELECT version, name FROM schema_migrations ORDER BY version DESC LIMIT 1",
	).Scan(&version, &name); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if version != 22 || name != "drop-custom-service-enabled" {
		t.Fatalf("latest migration = (%d, %q), want (22, %q)", version, name, "drop-custom-service-enabled")
	}

	// Spot-check that a few tables exist.
	for _, table := range []string{"sessions", "session_presets", "session_launchers", "tmux_launchers", "managed_tmux_windows", "tmux_session_leases", "wt_sessions", "ops_runbooks", "ops_schedules"} {
		var n int
		if err := db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?",
			table,
		).Scan(&n); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if n != 1 {
			t.Fatalf("table %s not found", table)
		}
	}
	var targetIndex int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type = 'index' AND name = 'idx_ops_runbook_runs_target_latest'
	`).Scan(&targetIndex); err != nil {
		t.Fatalf("check target latest index: %v", err)
	}
	if targetIndex != 1 {
		t.Fatalf("target latest index count = %d, want 1", targetIndex)
	}
	var leaseIndex int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type = 'index' AND name = 'idx_tmux_session_leases_deadlines'
	`).Scan(&leaseIndex); err != nil {
		t.Fatalf("check lease deadline index: %v", err)
	}
	if leaseIndex != 1 {
		t.Fatalf("lease deadline index count = %d, want 1", leaseIndex)
	}
}

// Custom services are registered and deregistered by INSERT/DELETE; nothing
// ever wrote enabled = 0, so the column and its filter were a soft-delete stub.
func TestCustomServicesHaveNoEnabledColumn(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	if err := runMigrations(ctx, db); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	rows, err := db.QueryContext(ctx, "SELECT name FROM pragma_table_info('ops_custom_services')")
	if err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		if column == "enabled" {
			t.Fatal("ops_custom_services still has an enabled column")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate columns: %v", err)
	}
}

func TestRunMigrationsIdempotent(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	if err := runMigrations(ctx, db); err != nil {
		t.Fatalf("first runMigrations: %v", err)
	}
	if err := runMigrations(ctx, db); err != nil {
		t.Fatalf("second runMigrations: %v", err)
	}

	// Only one row in schema_migrations.
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count != 19 {
		t.Fatalf("schema_migrations rows = %d, want 19", count)
	}
}

func TestBuiltinServicesMigrationRemovesOnlyReservedCollisions(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}

	all, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	var builtinMigration migration
	for _, item := range all {
		if item.version < 17 {
			if err := applyMigration(ctx, db, item); err != nil {
				t.Fatalf("apply migration %d: %v", item.version, err)
			}
			continue
		}
		if item.version == 17 {
			builtinMigration = item
			break
		}
	}

	rows := []struct {
		name string
		unit string
	}{
		{name: "sentinel", unit: "other.service"},
		{name: "updater-copy", unit: "sentinel-updater.timer"},
		{name: "launchd-copy", unit: "io.opusdomini.sentinel"},
		{name: "nginx", unit: "nginx.service"},
	}
	for _, row := range rows {
		if _, err := db.ExecContext(
			ctx,
			`INSERT INTO ops_custom_services (name, display_name, manager, unit, scope)
			 VALUES (?, ?, 'systemd', ?, 'user')`,
			row.name,
			row.name,
			row.unit,
		); err != nil {
			t.Fatalf("insert %s: %v", row.name, err)
		}
	}

	if err := applyMigration(ctx, db, builtinMigration); err != nil {
		t.Fatalf("apply builtin migration: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ops_custom_services").Scan(&count); err != nil {
		t.Fatalf("count custom services: %v", err)
	}
	if count != 1 {
		t.Fatalf("custom services count = %d, want 1", count)
	}
	var name string
	if err := db.QueryRowContext(ctx, "SELECT name FROM ops_custom_services").Scan(&name); err != nil {
		t.Fatalf("read surviving custom service: %v", err)
	}
	if name != "nginx" {
		t.Fatalf("surviving custom service = %q, want nginx", name)
	}
}

func TestRunMigrationsFreshDatabaseHasNoDefaultRunbooks(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	if err := runMigrations(ctx, db); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	var runbookCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ops_runbooks").Scan(&runbookCount); err != nil {
		t.Fatalf("count ops_runbooks: %v", err)
	}
	if runbookCount != 0 {
		t.Fatalf("ops_runbooks count = %d, want 0", runbookCount)
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO ops_runbooks (
		id, name, description, steps_json, enabled, webhook_url, parameters,
		target_service, created_at, updated_at
	) VALUES ('first.target', 'First', '', '[]', 1, '', '[]',
		'sentinel', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("first target_service insert: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO ops_runbooks (
		id, name, description, steps_json, enabled, webhook_url, parameters,
		target_service, created_at, updated_at
	) VALUES ('duplicate.target', 'Duplicate', '', '[]', 1, '', '[]',
		'sentinel', datetime('now'), datetime('now'))`); err == nil {
		t.Fatal("duplicate target_service insert succeeded")
	}

	// Runbooks have webhook_url column.
	var webhookURL string
	if err := db.QueryRowContext(ctx, "SELECT webhook_url FROM ops_runbooks LIMIT 1").Scan(&webhookURL); err != nil {
		t.Fatalf("select webhook_url: %v", err)
	}

	// Watchtower global revision seed.
	var globalRev string
	if err := db.QueryRowContext(ctx, "SELECT value FROM wt_runtime WHERE key='global_rev'").Scan(&globalRev); err != nil {
		t.Fatalf("select wt_runtime global_rev: %v", err)
	}
	if globalRev != "0" {
		t.Fatalf("global_rev = %q, want %q", globalRev, "0")
	}
}

func TestRunbookExecutionReceiptMigrationPreservesEditedDataAndClosesLegacyActiveRuns(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	all, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	var receiptMigration migration
	for _, item := range all {
		if item.version < 19 {
			if err := applyMigration(ctx, db, item); err != nil {
				t.Fatalf("apply migration %d: %v", item.version, err)
			}
			continue
		}
		receiptMigration = item
		break
	}

	if _, err := db.ExecContext(ctx, `UPDATE ops_runbooks
		SET description = 'User edited recovery', updated_at = '2099-01-01T00:00:00Z'
		WHERE id = 'ops.service.recover'`); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		id        string
		runbookID string
		status    string
		target    string
	}{
		{id: "terminal", runbookID: "ops.update.apply", status: "succeeded"},
		{id: "active", runbookID: "ops.service.recover", status: "waiting_approval", target: "sentinel"},
	} {
		if _, err := db.ExecContext(ctx, `INSERT INTO ops_runbook_runs (
			id, runbook_id, runbook_name, status, total_steps, completed_steps,
			current_step, error, step_results, parameters_used, source,
			target_kind, target_name, created_at, started_at, finished_at
		) VALUES (?, ?, 'Legacy', ?, 1, 0, '', '', '[]', '{}', 'runbooks',
			CASE WHEN ? = '' THEN '' ELSE 'service' END, ?, datetime('now'), '', '')`,
			row.id, row.runbookID, row.status, row.target, row.target,
		); err != nil {
			t.Fatalf("insert %s: %v", row.id, err)
		}
	}
	for _, schedule := range []struct {
		id        string
		runbookID string
	}{
		{id: "edited-schedule", runbookID: "ops.service.recover"},
		{id: "intact-schedule", runbookID: "ops.autoupdate.verify"},
	} {
		if _, err := db.ExecContext(ctx, `INSERT INTO ops_schedules (
			id, runbook_id, name, schedule_type
		) VALUES (?, ?, ?, 'once')`, schedule.id, schedule.runbookID, schedule.id); err != nil {
			t.Fatal(err)
		}
	}

	if err := applyMigration(ctx, db, receiptMigration); err != nil {
		t.Fatalf("apply receipt migration: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ops_runbooks`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("runbooks after migration = %d, want edited seed only", count)
	}
	var editedDescription string
	if err := db.QueryRowContext(ctx, `SELECT description FROM ops_runbooks
		WHERE id = 'ops.service.recover'`).Scan(&editedDescription); err != nil {
		t.Fatal(err)
	}
	if editedDescription != "User edited recovery" {
		t.Fatalf("edited description = %q", editedDescription)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ops_schedules`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("schedules after migration = %d, want edited seed schedule only", count)
	}
	var terminalStatus, terminalDefinition string
	if err := db.QueryRowContext(ctx, `SELECT status, definition_snapshot
		FROM ops_runbook_runs WHERE id = 'terminal'`).Scan(&terminalStatus, &terminalDefinition); err != nil {
		t.Fatal(err)
	}
	if terminalStatus != "succeeded" || terminalDefinition != "" {
		t.Fatalf("terminal legacy run = (%q, %q)", terminalStatus, terminalDefinition)
	}
	var activeStatus, activeError, finishedAt string
	if err := db.QueryRowContext(ctx, `SELECT status, error, finished_at
		FROM ops_runbook_runs WHERE id = 'active'`).Scan(&activeStatus, &activeError, &finishedAt); err != nil {
		t.Fatal(err)
	}
	if activeStatus != "failed" || activeError != "execution predates immutable receipt" || finishedAt == "" {
		t.Fatalf("active legacy run = (%q, %q, %q)", activeStatus, activeError, finishedAt)
	}
}

func TestRunbookServiceContextMigrationPreservesHistoricalRunAsUnknown(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	all, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	var contextMigration migration
	for _, item := range all {
		if item.version < 18 {
			if err := applyMigration(ctx, db, item); err != nil {
				t.Fatalf("apply migration %d: %v", item.version, err)
			}
			continue
		}
		contextMigration = item
		break
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO ops_runbook_runs (
		id, runbook_id, runbook_name, status, total_steps, completed_steps,
		current_step, error, step_results, parameters_used, created_at,
		started_at, finished_at
	) VALUES ('historical', 'ops.update.apply', 'Apply Update', 'succeeded',
		2, 2, 'done', '', '[]', '{}', datetime('now'), '', datetime('now'))`); err != nil {
		t.Fatalf("insert historical run: %v", err)
	}
	if err := applyMigration(ctx, db, contextMigration); err != nil {
		t.Fatalf("apply runbook context migration: %v", err)
	}
	var source, targetKind, targetName string
	if err := db.QueryRowContext(ctx, `SELECT source, target_kind, target_name
		FROM ops_runbook_runs WHERE id = 'historical'`).Scan(
		&source, &targetKind, &targetName,
	); err != nil {
		t.Fatalf("read historical context: %v", err)
	}
	if source != "" || targetKind != "" || targetName != "" {
		t.Fatalf("historical context = (%q, %q, %q), want empty", source, targetKind, targetName)
	}
}

func TestRunMigrationsExistingDB(t *testing.T) {
	t.Parallel()

	// Simulate a pre-migration DB: create the sessions table manually,
	// then run migrations. The IF NOT EXISTS DDL should be a no-op and
	// the session data should survive.
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `CREATE TABLE sessions (
		name TEXT PRIMARY KEY,
		hash TEXT NOT NULL,
		last_content TEXT DEFAULT '',
		icon TEXT DEFAULT '',
		next_window_seq INTEGER NOT NULL DEFAULT 1,
		updated_at TEXT DEFAULT (datetime('now'))
	)`)
	if err != nil {
		t.Fatalf("create legacy sessions: %v", err)
	}
	_, err = db.ExecContext(ctx,
		"INSERT INTO sessions (name, hash, last_content) VALUES ('dev', 'h1', 'preview')")
	if err != nil {
		t.Fatalf("insert legacy session: %v", err)
	}

	if err := runMigrations(ctx, db); err != nil {
		t.Fatalf("runMigrations on existing DB: %v", err)
	}

	// Session data survived.
	var hash string
	if err := db.QueryRowContext(ctx, "SELECT hash FROM sessions WHERE name='dev'").Scan(&hash); err != nil {
		t.Fatalf("read session after migration: %v", err)
	}
	if hash != "h1" {
		t.Fatalf("hash = %q, want %q", hash, "h1")
	}

	// Backfill copied session to wt_sessions.
	var preview string
	if err := db.QueryRowContext(ctx, "SELECT last_preview FROM wt_sessions WHERE session_name='dev'").Scan(&preview); err != nil {
		t.Fatalf("read wt_sessions backfill: %v", err)
	}
	if preview != "preview" {
		t.Fatalf("last_preview = %q, want %q", preview, "preview")
	}
}

func TestLoadMigrationsOrdering(t *testing.T) {
	t.Parallel()

	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("no migrations found")
	}

	for i := 1; i < len(migrations); i++ {
		if migrations[i].version <= migrations[i-1].version {
			t.Fatalf("migrations not sorted: version %d <= %d",
				migrations[i].version, migrations[i-1].version)
		}
	}
}

func TestParseMigrationFilename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input       string
		wantVersion int
		wantName    string
		wantErr     bool
	}{
		{"000001_init.sql", 1, "init", false},
		{"000042_add_column.sql", 42, "add_column", false},
		{"bad.sql", 0, "", true},
		{"abc_name.sql", 0, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			version, name, err := parseMigrationFilename(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseMigrationFilename(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err == nil {
				if version != tt.wantVersion || name != tt.wantName {
					t.Fatalf("parseMigrationFilename(%q) = (%d, %q), want (%d, %q)",
						tt.input, version, name, tt.wantVersion, tt.wantName)
				}
			}
		})
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}
