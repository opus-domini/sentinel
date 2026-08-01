package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestDeleteTmuxSessionRuntimeStateIsTargetedAndPreservesDurableData(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := newTestStore(t)
	t.Cleanup(func() { _ = st.Close() })

	for _, name := range []string{"agent", "keep"} {
		if err := st.UpsertSession(ctx, name, "hash", "content"); err != nil {
			t.Fatal(err)
		}
		if err := seedTmuxRuntimeState(ctx, st, name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.CreateSessionPreset(ctx, SessionPresetWrite{Name: "agent", Cwd: "/tmp", Icon: "terminal"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSessionLauncher(ctx, SessionLauncherWrite{Name: "agent-launcher", Cwd: "/tmp", Icon: "terminal"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateTmuxLauncher(ctx, TmuxLauncherWrite{
		Name: "window-launcher", Icon: "terminal", Command: "true", CwdMode: "session", UserMode: "session",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx,
		`INSERT INTO wt_journal (global_rev, entity_type, session_name) VALUES (1, 'session', 'agent')`,
	); err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteTmuxSessionRuntimeState(ctx, "agent"); err != nil {
		t.Fatalf("DeleteTmuxSessionRuntimeState() error = %v", err)
	}
	for _, item := range []struct {
		table  string
		column string
	}{
		{table: "sessions", column: "name"},
		{table: "wt_sessions", column: "session_name"},
		{table: "wt_windows", column: "session_name"},
		{table: "wt_panes", column: "session_name"},
		{table: "wt_presence", column: "session_name"},
		{table: "managed_tmux_windows", column: "session_name"},
		{table: "session_users", column: "session_name"},
	} {
		if countStoreRows(t, st, item.table, item.column, "agent") != 0 {
			t.Fatalf("%s still contains agent runtime state", item.table)
		}
		if countStoreRows(t, st, item.table, item.column, "keep") != 1 {
			t.Fatalf("%s lost unrelated runtime state", item.table)
		}
	}
	for _, item := range []struct {
		table  string
		column string
		value  string
	}{
		{table: "session_presets", column: "name", value: "agent"},
		{table: "session_launchers", column: "name", value: "agent-launcher"},
		{table: "tmux_launchers", column: "name", value: "window-launcher"},
		{table: "wt_journal", column: "session_name", value: "agent"},
	} {
		if countStoreRows(t, st, item.table, item.column, item.value) != 1 {
			t.Fatalf("%s durable row was removed", item.table)
		}
	}
}

func seedTmuxRuntimeState(ctx context.Context, st *Store, name string) error {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	statements := []struct {
		query string
		args  []any
	}{
		{query: `INSERT INTO wt_sessions (session_name, activity_at) VALUES (?, ?)`, args: []any{name, now}},
		{query: `INSERT INTO wt_windows (session_name, window_index) VALUES (?, 0)`, args: []any{name}},
		{query: `INSERT INTO wt_panes (pane_id, session_name, window_index, pane_index) VALUES (?, ?, 0, 0)`, args: []any{"%" + name, name}},
		{query: `INSERT INTO wt_presence (terminal_id, session_name) VALUES (?, ?)`, args: []any{"terminal-" + name, name}},
		{query: `INSERT INTO managed_tmux_windows (id, session_name) VALUES (?, ?)`, args: []any{"managed-" + name, name}},
		{query: `INSERT INTO session_users (session_name, user) VALUES (?, ?)`, args: []any{name, "deploy"}},
	}
	for _, statement := range statements {
		if _, err := st.db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return err
		}
	}
	return nil
}

func countStoreRows(t *testing.T, st *Store, table, column, value string) int {
	t.Helper()
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?", table, column)
	var count int
	if err := st.db.QueryRowContext(context.Background(), query, value).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
