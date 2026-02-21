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
	if version != 1 || name != "init" {
		t.Fatalf("latest migration = (%d, %q), want (1, %q)", version, name, "init")
	}

	// Spot-check that a few tables exist.
	for _, table := range []string{"sessions", "wt_sessions", "guardrail_rules", "ops_runbooks", "ops_schedules"} {
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
	if count != 1 {
		t.Fatalf("schema_migrations rows = %d, want 1", count)
	}
}

func TestRunMigrationsSeedData(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	if err := runMigrations(ctx, db); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	// Guardrail default rules.
	var guardrailCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM guardrail_rules").Scan(&guardrailCount); err != nil {
		t.Fatalf("count guardrail_rules: %v", err)
	}
	if guardrailCount != 2 {
		t.Fatalf("guardrail_rules count = %d, want 2", guardrailCount)
	}

	// All guardrail rules have scope='action'.
	var legacyCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM guardrail_rules WHERE scope != 'action'").Scan(&legacyCount); err != nil {
		t.Fatalf("count legacy scope: %v", err)
	}
	if legacyCount != 0 {
		t.Fatalf("legacy scope rules = %d, want 0", legacyCount)
	}

	// Default runbooks.
	var runbookCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ops_runbooks").Scan(&runbookCount); err != nil {
		t.Fatalf("count ops_runbooks: %v", err)
	}
	if runbookCount != 3 {
		t.Fatalf("ops_runbooks count = %d, want 3", runbookCount)
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
