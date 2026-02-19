package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

type WatchtowerSession struct {
	SessionName       string    `json:"sessionName"`
	Attached          int       `json:"attached"`
	Windows           int       `json:"windows"`
	Panes             int       `json:"panes"`
	ActivityAt        time.Time `json:"activityAt"`
	LastPreview       string    `json:"lastPreview"`
	LastPreviewAt     time.Time `json:"lastPreviewAt"`
	LastPreviewPaneID string    `json:"lastPreviewPaneId"`
	UnreadWindows     int       `json:"unreadWindows"`
	UnreadPanes       int       `json:"unreadPanes"`
	Rev               int64     `json:"rev"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type WatchtowerSessionWrite struct {
	SessionName       string
	Attached          int
	Windows           int
	Panes             int
	ActivityAt        time.Time
	LastPreview       string
	LastPreviewAt     time.Time
	LastPreviewPaneID string
	UnreadWindows     int
	UnreadPanes       int
	Rev               int64
	UpdatedAt         time.Time
}

// BuildWatchtowerSessionActivityPatch returns the compact projection pushed to
// clients for session-list reconciliation without requiring full list reloads.
func BuildWatchtowerSessionActivityPatch(row WatchtowerSession) map[string]any {
	activityAt := ""
	if !row.ActivityAt.IsZero() {
		activityAt = row.ActivityAt.UTC().Format(time.RFC3339)
	}
	return map[string]any{
		"name":          row.SessionName,
		"attached":      row.Attached,
		"windows":       row.Windows,
		"panes":         row.Panes,
		"activityAt":    activityAt,
		"lastContent":   row.LastPreview,
		"unreadWindows": row.UnreadWindows,
		"unreadPanes":   row.UnreadPanes,
		"rev":           row.Rev,
	}
}

// BuildWatchtowerWindowPatches returns projection rows suitable for
// client-side window strip reconciliation without additional API reads.
func BuildWatchtowerWindowPatches(windows []WatchtowerWindow, panes []WatchtowerPane) []map[string]any {
	paneCounts := make(map[int]int, len(windows))
	for _, pane := range panes {
		paneCounts[pane.WindowIndex]++
	}

	patches := make([]map[string]any, 0, len(windows))
	for _, row := range windows {
		activityAt := ""
		if !row.WindowActivityAt.IsZero() {
			activityAt = row.WindowActivityAt.UTC().Format(time.RFC3339)
		}
		patches = append(patches, map[string]any{
			"session":     row.SessionName,
			"index":       row.WindowIndex,
			"name":        row.Name,
			"active":      row.Active,
			"panes":       paneCounts[row.WindowIndex],
			"layout":      row.Layout,
			"unreadPanes": row.UnreadPanes,
			"hasUnread":   row.HasUnread,
			"rev":         row.Rev,
			"activityAt":  activityAt,
		})
	}
	return patches
}

// BuildWatchtowerPanePatches returns projection rows suitable for
// client-side pane strip reconciliation without additional API reads.
func BuildWatchtowerPanePatches(panes []WatchtowerPane) []map[string]any {
	patches := make([]map[string]any, 0, len(panes))
	for _, row := range panes {
		changedAt := ""
		if !row.ChangedAt.IsZero() {
			changedAt = row.ChangedAt.UTC().Format(time.RFC3339)
		}
		patches = append(patches, map[string]any{
			"session":        row.SessionName,
			"windowIndex":    row.WindowIndex,
			"paneIndex":      row.PaneIndex,
			"paneId":         row.PaneID,
			"title":          row.Title,
			"active":         row.Active,
			"tty":            row.TTY,
			"currentPath":    row.CurrentPath,
			"startCommand":   row.StartCommand,
			"currentCommand": row.CurrentCommand,
			"tailPreview":    row.TailPreview,
			"revision":       row.Revision,
			"seenRevision":   row.SeenRevision,
			"hasUnread":      row.Revision > row.SeenRevision,
			"changedAt":      changedAt,
		})
	}
	return patches
}

// BuildWatchtowerInspectorPatch returns a full window/pane projection patch for
// one session, used by ws activity/seen events to avoid inspector polling.
func BuildWatchtowerInspectorPatch(sessionName string, windows []WatchtowerWindow, panes []WatchtowerPane) map[string]any {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		if len(windows) > 0 {
			sessionName = strings.TrimSpace(windows[0].SessionName)
		} else if len(panes) > 0 {
			sessionName = strings.TrimSpace(panes[0].SessionName)
		}
	}
	return map[string]any{
		"session": sessionName,
		"windows": BuildWatchtowerWindowPatches(windows, panes),
		"panes":   BuildWatchtowerPanePatches(panes),
	}
}

type WatchtowerWindow struct {
	SessionName      string    `json:"sessionName"`
	WindowIndex      int       `json:"windowIndex"`
	Name             string    `json:"name"`
	Active           bool      `json:"active"`
	Layout           string    `json:"layout"`
	WindowActivityAt time.Time `json:"windowActivityAt"`
	UnreadPanes      int       `json:"unreadPanes"`
	HasUnread        bool      `json:"hasUnread"`
	Rev              int64     `json:"rev"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type WatchtowerWindowWrite struct {
	SessionName      string
	WindowIndex      int
	Name             string
	Active           bool
	Layout           string
	WindowActivityAt time.Time
	UnreadPanes      int
	HasUnread        bool
	Rev              int64
	UpdatedAt        time.Time
}

type WatchtowerPane struct {
	PaneID         string    `json:"paneId"`
	SessionName    string    `json:"sessionName"`
	WindowIndex    int       `json:"windowIndex"`
	PaneIndex      int       `json:"paneIndex"`
	Title          string    `json:"title"`
	Active         bool      `json:"active"`
	TTY            string    `json:"tty"`
	CurrentPath    string    `json:"currentPath"`
	StartCommand   string    `json:"startCommand"`
	CurrentCommand string    `json:"currentCommand"`
	TailHash       string    `json:"tailHash"`
	TailPreview    string    `json:"tailPreview"`
	TailCapturedAt time.Time `json:"tailCapturedAt"`
	Revision       int64     `json:"revision"`
	SeenRevision   int64     `json:"seenRevision"`
	ChangedAt      time.Time `json:"changedAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type WatchtowerPaneWrite struct {
	PaneID         string
	SessionName    string
	WindowIndex    int
	PaneIndex      int
	Title          string
	Active         bool
	TTY            string
	CurrentPath    string
	StartCommand   string
	CurrentCommand string
	TailHash       string
	TailPreview    string
	TailCapturedAt time.Time
	Revision       int64
	SeenRevision   int64
	ChangedAt      time.Time
	UpdatedAt      time.Time
}

type WatchtowerPresence struct {
	TerminalID  string    `json:"terminalId"`
	SessionName string    `json:"sessionName"`
	WindowIndex int       `json:"windowIndex"`
	PaneID      string    `json:"paneId"`
	Visible     bool      `json:"visible"`
	Focused     bool      `json:"focused"`
	UpdatedAt   time.Time `json:"updatedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

type WatchtowerPresenceWrite struct {
	TerminalID  string
	SessionName string
	WindowIndex int
	PaneID      string
	Visible     bool
	Focused     bool
	UpdatedAt   time.Time
	ExpiresAt   time.Time
}

type WatchtowerJournal struct {
	ID         int64     `json:"id"`
	GlobalRev  int64     `json:"globalRev"`
	EntityType string    `json:"entityType"`
	Session    string    `json:"session"`
	WindowIdx  int       `json:"windowIndex"`
	PaneID     string    `json:"paneId"`
	ChangeKind string    `json:"changeKind"`
	ChangedAt  time.Time `json:"changedAt"`
}

type WatchtowerJournalWrite struct {
	GlobalRev  int64
	EntityType string
	Session    string
	WindowIdx  int
	PaneID     string
	ChangeKind string
	ChangedAt  time.Time
}

func (s *Store) initWatchtowerSchema() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS wt_sessions (
			session_name         TEXT PRIMARY KEY,
			attached             INTEGER NOT NULL DEFAULT 0,
			windows              INTEGER NOT NULL DEFAULT 0,
			panes                INTEGER NOT NULL DEFAULT 0,
			activity_at          TEXT NOT NULL DEFAULT '',
			last_preview         TEXT NOT NULL DEFAULT '',
			last_preview_at      TEXT NOT NULL DEFAULT '',
			last_preview_pane_id TEXT NOT NULL DEFAULT '',
			unread_windows       INTEGER NOT NULL DEFAULT 0,
			unread_panes         INTEGER NOT NULL DEFAULT 0,
			rev                  INTEGER NOT NULL DEFAULT 0,
			updated_at           TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS wt_windows (
			session_name       TEXT NOT NULL,
			window_index       INTEGER NOT NULL,
			name               TEXT NOT NULL DEFAULT '',
			active             INTEGER NOT NULL DEFAULT 0,
			layout             TEXT NOT NULL DEFAULT '',
			window_activity_at TEXT NOT NULL DEFAULT '',
			unread_panes       INTEGER NOT NULL DEFAULT 0,
			has_unread         INTEGER NOT NULL DEFAULT 0,
			rev                INTEGER NOT NULL DEFAULT 0,
			updated_at         TEXT NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (session_name, window_index)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_wt_windows_session_activity
			ON wt_windows (session_name, window_activity_at DESC)`,
		`CREATE TABLE IF NOT EXISTS wt_panes (
			pane_id          TEXT PRIMARY KEY,
			session_name     TEXT NOT NULL,
			window_index     INTEGER NOT NULL,
			pane_index       INTEGER NOT NULL,
			title            TEXT NOT NULL DEFAULT '',
			active           INTEGER NOT NULL DEFAULT 0,
			tty              TEXT NOT NULL DEFAULT '',
			current_path     TEXT NOT NULL DEFAULT '',
			start_command    TEXT NOT NULL DEFAULT '',
			current_command  TEXT NOT NULL DEFAULT '',
			tail_hash        TEXT NOT NULL DEFAULT '',
			tail_preview     TEXT NOT NULL DEFAULT '',
			tail_captured_at TEXT NOT NULL DEFAULT '',
			revision         INTEGER NOT NULL DEFAULT 0,
			seen_revision    INTEGER NOT NULL DEFAULT 0,
			changed_at       TEXT NOT NULL DEFAULT '',
			updated_at       TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_wt_panes_session_window
			ON wt_panes (session_name, window_index, pane_index)`,
		`CREATE INDEX IF NOT EXISTS idx_wt_panes_unread
			ON wt_panes (session_name, revision, seen_revision)`,
		`CREATE TABLE IF NOT EXISTS wt_presence (
			terminal_id   TEXT PRIMARY KEY,
			session_name  TEXT NOT NULL DEFAULT '',
			window_index  INTEGER NOT NULL DEFAULT -1,
			pane_id       TEXT NOT NULL DEFAULT '',
			visible       INTEGER NOT NULL DEFAULT 0,
			focused       INTEGER NOT NULL DEFAULT 0,
			updated_at    TEXT NOT NULL DEFAULT (datetime('now')),
			expires_at    TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_wt_presence_expires_at
			ON wt_presence (expires_at)`,
		`CREATE TABLE IF NOT EXISTS wt_journal (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			global_rev   INTEGER NOT NULL,
			entity_type  TEXT NOT NULL,
			session_name TEXT NOT NULL DEFAULT '',
			window_index INTEGER NOT NULL DEFAULT -1,
			pane_id      TEXT NOT NULL DEFAULT '',
			change_kind  TEXT NOT NULL DEFAULT '',
			changed_at   TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_wt_journal_global_rev
			ON wt_journal (global_rev ASC)`,
		`CREATE TABLE IF NOT EXISTS wt_runtime (
			key        TEXT PRIMARY KEY,
			value      TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS wt_timeline_events (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			session_name  TEXT NOT NULL DEFAULT '',
			window_index  INTEGER NOT NULL DEFAULT -1,
			pane_id       TEXT NOT NULL DEFAULT '',
			event_type    TEXT NOT NULL DEFAULT '',
			severity      TEXT NOT NULL DEFAULT '',
			command       TEXT NOT NULL DEFAULT '',
			cwd           TEXT NOT NULL DEFAULT '',
			duration_ms   INTEGER NOT NULL DEFAULT 0,
			summary       TEXT NOT NULL DEFAULT '',
			details       TEXT NOT NULL DEFAULT '',
			marker        TEXT NOT NULL DEFAULT '',
			metadata_json TEXT NOT NULL DEFAULT '{}',
			created_at    TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_wt_timeline_created
			ON wt_timeline_events (created_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_wt_timeline_session
			ON wt_timeline_events (session_name, created_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_wt_timeline_scope
			ON wt_timeline_events (session_name, window_index, pane_id, created_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_wt_timeline_event_type
			ON wt_timeline_events (event_type, created_at DESC, id DESC)`,
		`CREATE TABLE IF NOT EXISTS wt_pane_runtime (
			pane_id          TEXT PRIMARY KEY,
			session_name     TEXT NOT NULL DEFAULT '',
			window_index     INTEGER NOT NULL DEFAULT -1,
			current_command  TEXT NOT NULL DEFAULT '',
			started_at       TEXT NOT NULL DEFAULT '',
			updated_at       TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_wt_pane_runtime_session
			ON wt_pane_runtime (session_name, pane_id)`,
		`INSERT OR IGNORE INTO wt_runtime(key, value)
		 VALUES ('global_rev', '0')`,
		`INSERT OR IGNORE INTO wt_sessions(session_name, last_preview, updated_at)
		 SELECT name, COALESCE(last_content, ''), datetime('now')
		 FROM sessions`,
	}

	for _, stmt := range statements {
		if _, err := s.db.ExecContext(context.Background(), stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertWatchtowerSession(ctx context.Context, row WatchtowerSessionWrite) error {
	name := strings.TrimSpace(row.SessionName)
	if name == "" {
		return errors.New("session name is required")
	}
	updatedAt := row.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wt_sessions (
			session_name, attached, windows, panes, activity_at,
			last_preview, last_preview_at, last_preview_pane_id,
			unread_windows, unread_panes, rev, updated_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(session_name) DO UPDATE SET
			attached = excluded.attached,
			windows = excluded.windows,
			panes = excluded.panes,
			activity_at = excluded.activity_at,
			last_preview = excluded.last_preview,
			last_preview_at = excluded.last_preview_at,
			last_preview_pane_id = excluded.last_preview_pane_id,
			unread_windows = excluded.unread_windows,
			unread_panes = excluded.unread_panes,
			rev = excluded.rev,
			updated_at = excluded.updated_at`,
		name,
		row.Attached,
		row.Windows,
		row.Panes,
		formatStoreValueTime(row.ActivityAt),
		strings.TrimSpace(row.LastPreview),
		formatStoreValueTime(row.LastPreviewAt),
		strings.TrimSpace(row.LastPreviewPaneID),
		row.UnreadWindows,
		row.UnreadPanes,
		row.Rev,
		updatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *Store) GetWatchtowerSession(ctx context.Context, sessionName string) (WatchtowerSession, error) {
	var (
		row                                     WatchtowerSession
		activityAtRaw, previewAtRaw, updatedRaw string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT session_name, attached, windows, panes, activity_at,
		        last_preview, last_preview_at, last_preview_pane_id,
		        unread_windows, unread_panes, rev, updated_at
		   FROM wt_sessions
		  WHERE session_name = ?`,
		strings.TrimSpace(sessionName),
	).Scan(
		&row.SessionName,
		&row.Attached,
		&row.Windows,
		&row.Panes,
		&activityAtRaw,
		&row.LastPreview,
		&previewAtRaw,
		&row.LastPreviewPaneID,
		&row.UnreadWindows,
		&row.UnreadPanes,
		&row.Rev,
		&updatedRaw,
	)
	if err != nil {
		return WatchtowerSession{}, err
	}
	row.ActivityAt = parseStoreTime(activityAtRaw)
	row.LastPreviewAt = parseStoreTime(previewAtRaw)
	row.UpdatedAt = parseStoreTime(updatedRaw)
	return row, nil
}

func (s *Store) GetWatchtowerSessionActivityPatch(ctx context.Context, sessionName string) (map[string]any, error) {
	row, err := s.GetWatchtowerSession(ctx, sessionName)
	if err != nil {
		return nil, err
	}
	return BuildWatchtowerSessionActivityPatch(row), nil
}

func (s *Store) GetWatchtowerInspectorPatch(ctx context.Context, sessionName string) (map[string]any, error) {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return nil, errors.New("session name is required")
	}
	windows, err := s.ListWatchtowerWindows(ctx, sessionName)
	if err != nil {
		return nil, err
	}
	panes, err := s.ListWatchtowerPanes(ctx, sessionName)
	if err != nil {
		return nil, err
	}
	return BuildWatchtowerInspectorPatch(sessionName, windows, panes), nil
}

func (s *Store) ListWatchtowerSessions(ctx context.Context) ([]WatchtowerSession, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_name, attached, windows, panes, activity_at,
		        last_preview, last_preview_at, last_preview_pane_id,
		        unread_windows, unread_panes, rev, updated_at
		   FROM wt_sessions
		  ORDER BY session_name ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]WatchtowerSession, 0, 16)
	for rows.Next() {
		var (
			row                                     WatchtowerSession
			activityAtRaw, previewAtRaw, updatedRaw string
		)
		if err := rows.Scan(
			&row.SessionName,
			&row.Attached,
			&row.Windows,
			&row.Panes,
			&activityAtRaw,
			&row.LastPreview,
			&previewAtRaw,
			&row.LastPreviewPaneID,
			&row.UnreadWindows,
			&row.UnreadPanes,
			&row.Rev,
			&updatedRaw,
		); err != nil {
			return nil, err
		}
		row.ActivityAt = parseStoreTime(activityAtRaw)
		row.LastPreviewAt = parseStoreTime(previewAtRaw)
		row.UpdatedAt = parseStoreTime(updatedRaw)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) UpsertWatchtowerWindow(ctx context.Context, row WatchtowerWindowWrite) error {
	name := strings.TrimSpace(row.SessionName)
	if name == "" {
		return errors.New("session name is required")
	}
	updatedAt := row.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wt_windows (
			session_name, window_index, name, active, layout,
			window_activity_at, unread_panes, has_unread, rev, updated_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(session_name, window_index) DO UPDATE SET
			name = excluded.name,
			active = excluded.active,
			layout = excluded.layout,
			window_activity_at = excluded.window_activity_at,
			unread_panes = excluded.unread_panes,
			has_unread = excluded.has_unread,
			rev = excluded.rev,
			updated_at = excluded.updated_at`,
		name,
		row.WindowIndex,
		strings.TrimSpace(row.Name),
		boolToInt(row.Active),
		strings.TrimSpace(row.Layout),
		formatStoreValueTime(row.WindowActivityAt),
		row.UnreadPanes,
		boolToInt(row.HasUnread),
		row.Rev,
		updatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *Store) ListWatchtowerWindows(ctx context.Context, sessionName string) ([]WatchtowerWindow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_name, window_index, name, active, layout,
		        window_activity_at, unread_panes, has_unread, rev, updated_at
		   FROM wt_windows
		  WHERE session_name = ?
		  ORDER BY window_index ASC`,
		strings.TrimSpace(sessionName),
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]WatchtowerWindow, 0, 8)
	for rows.Next() {
		var (
			row                            WatchtowerWindow
			activeRaw, unreadRaw           int
			activityAtRaw, updatedAtRawRaw string
		)
		if err := rows.Scan(
			&row.SessionName,
			&row.WindowIndex,
			&row.Name,
			&activeRaw,
			&row.Layout,
			&activityAtRaw,
			&row.UnreadPanes,
			&unreadRaw,
			&row.Rev,
			&updatedAtRawRaw,
		); err != nil {
			return nil, err
		}
		row.Active = activeRaw == 1
		row.HasUnread = unreadRaw == 1
		row.WindowActivityAt = parseStoreTime(activityAtRaw)
		row.UpdatedAt = parseStoreTime(updatedAtRawRaw)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) UpsertWatchtowerPane(ctx context.Context, row WatchtowerPaneWrite) error {
	paneID := strings.TrimSpace(row.PaneID)
	if paneID == "" {
		return errors.New("pane id is required")
	}
	name := strings.TrimSpace(row.SessionName)
	if name == "" {
		return errors.New("session name is required")
	}
	updatedAt := row.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wt_panes (
			pane_id, session_name, window_index, pane_index, title,
			active, tty, current_path, start_command, current_command,
			tail_hash, tail_preview, tail_captured_at,
			revision, seen_revision, changed_at, updated_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(pane_id) DO UPDATE SET
			session_name = excluded.session_name,
			window_index = excluded.window_index,
			pane_index = excluded.pane_index,
			title = excluded.title,
			active = excluded.active,
			tty = excluded.tty,
			current_path = excluded.current_path,
			start_command = excluded.start_command,
			current_command = excluded.current_command,
			tail_hash = excluded.tail_hash,
			tail_preview = excluded.tail_preview,
			tail_captured_at = excluded.tail_captured_at,
			revision = excluded.revision,
			seen_revision = excluded.seen_revision,
			changed_at = excluded.changed_at,
			updated_at = excluded.updated_at`,
		paneID,
		name,
		row.WindowIndex,
		row.PaneIndex,
		strings.TrimSpace(row.Title),
		boolToInt(row.Active),
		strings.TrimSpace(row.TTY),
		strings.TrimSpace(row.CurrentPath),
		strings.TrimSpace(row.StartCommand),
		strings.TrimSpace(row.CurrentCommand),
		strings.TrimSpace(row.TailHash),
		strings.TrimSpace(row.TailPreview),
		formatStoreValueTime(row.TailCapturedAt),
		row.Revision,
		row.SeenRevision,
		formatStoreValueTime(row.ChangedAt),
		updatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *Store) ListWatchtowerPanes(ctx context.Context, sessionName string) ([]WatchtowerPane, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT pane_id, session_name, window_index, pane_index, title,
		        active, tty, current_path, start_command, current_command,
		        tail_hash, tail_preview, tail_captured_at,
		        revision, seen_revision, changed_at, updated_at
		   FROM wt_panes
		  WHERE session_name = ?
		  ORDER BY window_index ASC, pane_index ASC`,
		strings.TrimSpace(sessionName),
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]WatchtowerPane, 0, 16)
	for rows.Next() {
		var (
			row                                   WatchtowerPane
			activeRaw                             int
			tailCapturedRaw, changedAt, updatedAt string
		)
		if err := rows.Scan(
			&row.PaneID,
			&row.SessionName,
			&row.WindowIndex,
			&row.PaneIndex,
			&row.Title,
			&activeRaw,
			&row.TTY,
			&row.CurrentPath,
			&row.StartCommand,
			&row.CurrentCommand,
			&row.TailHash,
			&row.TailPreview,
			&tailCapturedRaw,
			&row.Revision,
			&row.SeenRevision,
			&changedAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		row.Active = activeRaw == 1
		row.TailCapturedAt = parseStoreTime(tailCapturedRaw)
		row.ChangedAt = parseStoreTime(changedAt)
		row.UpdatedAt = parseStoreTime(updatedAt)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) UpsertWatchtowerPresence(ctx context.Context, row WatchtowerPresenceWrite) error {
	terminalID := strings.TrimSpace(row.TerminalID)
	if terminalID == "" {
		return errors.New("terminal id is required")
	}
	updatedAt := row.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	expiresAt := row.ExpiresAt.UTC()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wt_presence (
			terminal_id, session_name, window_index, pane_id,
			visible, focused, updated_at, expires_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(terminal_id) DO UPDATE SET
			session_name = excluded.session_name,
			window_index = excluded.window_index,
			pane_id = excluded.pane_id,
			visible = excluded.visible,
			focused = excluded.focused,
			updated_at = excluded.updated_at,
			expires_at = excluded.expires_at`,
		terminalID,
		strings.TrimSpace(row.SessionName),
		row.WindowIndex,
		strings.TrimSpace(row.PaneID),
		boolToInt(row.Visible),
		boolToInt(row.Focused),
		updatedAt.Format(time.RFC3339),
		formatStoreValueTime(expiresAt),
	)
	return err
}

func (s *Store) ListWatchtowerPresence(ctx context.Context) ([]WatchtowerPresence, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT terminal_id, session_name, window_index, pane_id,
		        visible, focused, updated_at, expires_at
		   FROM wt_presence
		  ORDER BY terminal_id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]WatchtowerPresence, 0, 8)
	for rows.Next() {
		var (
			row                        WatchtowerPresence
			visibleRaw, focusedRaw     int
			updatedAtRaw, expiresAtRaw string
		)
		if err := rows.Scan(
			&row.TerminalID,
			&row.SessionName,
			&row.WindowIndex,
			&row.PaneID,
			&visibleRaw,
			&focusedRaw,
			&updatedAtRaw,
			&expiresAtRaw,
		); err != nil {
			return nil, err
		}
		row.Visible = visibleRaw == 1
		row.Focused = focusedRaw == 1
		row.UpdatedAt = parseStoreTime(updatedAtRaw)
		row.ExpiresAt = parseStoreTime(expiresAtRaw)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) ListWatchtowerPresenceBySession(ctx context.Context, sessionName string) ([]WatchtowerPresence, error) {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return []WatchtowerPresence{}, nil
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT terminal_id, session_name, window_index, pane_id,
		        visible, focused, updated_at, expires_at
		   FROM wt_presence
		  WHERE session_name = ?
		  ORDER BY terminal_id ASC`,
		sessionName,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]WatchtowerPresence, 0, 8)
	for rows.Next() {
		var (
			row                        WatchtowerPresence
			visibleRaw, focusedRaw     int
			updatedAtRaw, expiresAtRaw string
		)
		if err := rows.Scan(
			&row.TerminalID,
			&row.SessionName,
			&row.WindowIndex,
			&row.PaneID,
			&visibleRaw,
			&focusedRaw,
			&updatedAtRaw,
			&expiresAtRaw,
		); err != nil {
			return nil, err
		}
		row.Visible = visibleRaw == 1
		row.Focused = focusedRaw == 1
		row.UpdatedAt = parseStoreTime(updatedAtRaw)
		row.ExpiresAt = parseStoreTime(expiresAtRaw)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) PruneWatchtowerPresence(ctx context.Context, now time.Time) (int64, error) {
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM wt_presence
		  WHERE expires_at != ''
		    AND expires_at < ?`,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) MarkWatchtowerPaneSeen(ctx context.Context, sessionName, paneID string) (bool, error) {
	sessionName = strings.TrimSpace(sessionName)
	paneID = strings.TrimSpace(paneID)
	if sessionName == "" {
		return false, errors.New("session name is required")
	}
	if paneID == "" {
		return false, errors.New("pane id is required")
	}
	return s.markWatchtowerSeen(ctx,
		sessionName,
		`UPDATE wt_panes
		    SET seen_revision = revision,
		        updated_at = datetime('now')
		  WHERE session_name = ?
		    AND pane_id = ?
		    AND revision > seen_revision`,
		sessionName,
		paneID,
	)
}

func (s *Store) MarkWatchtowerWindowSeen(ctx context.Context, sessionName string, windowIndex int) (bool, error) {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return false, errors.New("session name is required")
	}
	if windowIndex < 0 {
		return false, errors.New("window index must be >= 0")
	}
	return s.markWatchtowerSeen(ctx,
		sessionName,
		`UPDATE wt_panes
		    SET seen_revision = revision,
		        updated_at = datetime('now')
		  WHERE session_name = ?
		    AND window_index = ?
		    AND revision > seen_revision`,
		sessionName,
		windowIndex,
	)
}

func (s *Store) MarkWatchtowerSessionSeen(ctx context.Context, sessionName string) (bool, error) {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return false, errors.New("session name is required")
	}
	return s.markWatchtowerSeen(ctx,
		sessionName,
		`UPDATE wt_panes
		    SET seen_revision = revision,
		        updated_at = datetime('now')
		  WHERE session_name = ?
		    AND revision > seen_revision`,
		sessionName,
	)
}

func (s *Store) markWatchtowerSeen(ctx context.Context, sessionName, updateQuery string, args ...any) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, updateQuery, args...)
	if err != nil {
		return false, err
	}
	if err := recomputeWatchtowerUnreadTx(ctx, tx, sessionName); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (s *Store) SetWatchtowerRuntimeValue(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wt_runtime (key, value, updated_at)
		 VALUES (?, ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at`,
		strings.TrimSpace(key),
		value,
	)
	return err
}

func (s *Store) GetWatchtowerRuntimeValue(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM wt_runtime WHERE key = ?`,
		strings.TrimSpace(key),
	).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

// WatchtowerGlobalRevision returns the current global revision counter,
// or 0 if the value is absent or unparseable.
func (s *Store) WatchtowerGlobalRevision(ctx context.Context) (int64, error) {
	raw, err := s.GetWatchtowerRuntimeValue(ctx, "global_rev")
	if err != nil {
		return 0, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	return strconv.ParseInt(raw, 10, 64)
}

func (s *Store) InsertWatchtowerJournal(ctx context.Context, row WatchtowerJournalWrite) (int64, error) {
	entityType := strings.TrimSpace(row.EntityType)
	if entityType == "" {
		return 0, errors.New("entity type is required")
	}
	changedAt := row.ChangedAt.UTC()
	if changedAt.IsZero() {
		changedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO wt_journal (
			global_rev, entity_type, session_name, window_index,
			pane_id, change_kind, changed_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		row.GlobalRev,
		entityType,
		strings.TrimSpace(row.Session),
		row.WindowIdx,
		strings.TrimSpace(row.PaneID),
		strings.TrimSpace(row.ChangeKind),
		changedAt.Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) ListWatchtowerJournalSince(ctx context.Context, sinceRev int64, limit int) ([]WatchtowerJournal, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, global_rev, entity_type, session_name, window_index,
		        pane_id, change_kind, changed_at
		   FROM wt_journal
		  WHERE global_rev > ?
		  ORDER BY global_rev ASC, id ASC
		  LIMIT ?`,
		sinceRev,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]WatchtowerJournal, 0, limit)
	for rows.Next() {
		var (
			row          WatchtowerJournal
			changedAtRaw string
		)
		if err := rows.Scan(
			&row.ID,
			&row.GlobalRev,
			&row.EntityType,
			&row.Session,
			&row.WindowIdx,
			&row.PaneID,
			&row.ChangeKind,
			&changedAtRaw,
		); err != nil {
			return nil, err
		}
		row.ChangedAt = parseStoreTime(changedAtRaw)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) PruneWatchtowerJournalRows(ctx context.Context, maxRows int) (int64, error) {
	if maxRows <= 0 {
		return 0, nil
	}

	result, err := s.db.ExecContext(ctx,
		`DELETE FROM wt_journal
		  WHERE id IN (
			SELECT id
			  FROM wt_journal
			 ORDER BY global_rev DESC, id DESC
			 LIMIT -1 OFFSET ?
		  )`,
		maxRows,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func recomputeWatchtowerUnreadTx(ctx context.Context, tx *sql.Tx, sessionName string) error {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return nil
	}

	windowUnread := make(map[int]int)
	rows, err := tx.QueryContext(ctx,
		`SELECT window_index, COUNT(*)
		   FROM wt_panes
		  WHERE session_name = ?
		    AND revision > seen_revision
		  GROUP BY window_index`,
		sessionName,
	)
	if err != nil {
		return err
	}
	for rows.Next() {
		var idx, count int
		if err := rows.Scan(&idx, &count); err != nil {
			_ = rows.Close()
			return err
		}
		windowUnread[idx] = count
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()

	wRows, err := tx.QueryContext(ctx,
		`SELECT window_index, unread_panes, has_unread
		   FROM wt_windows
		  WHERE session_name = ?`,
		sessionName,
	)
	if err != nil {
		return err
	}
	defer func() { _ = wRows.Close() }()

	unreadWindows := 0
	unreadPanes := 0
	for wRows.Next() {
		var idx, prevUnread, prevHasUnread int
		if err := wRows.Scan(&idx, &prevUnread, &prevHasUnread); err != nil {
			return err
		}
		nextUnread := windowUnread[idx]
		nextHasUnread := 0
		if nextUnread > 0 {
			nextHasUnread = 1
			unreadWindows++
			unreadPanes += nextUnread
		}

		if nextUnread != prevUnread || nextHasUnread != prevHasUnread {
			if _, err := tx.ExecContext(ctx,
				`UPDATE wt_windows
				    SET unread_panes = ?,
				        has_unread = ?,
				        rev = rev + 1,
				        updated_at = datetime('now')
				  WHERE session_name = ?
				    AND window_index = ?`,
				nextUnread,
				nextHasUnread,
				sessionName,
				idx,
			); err != nil {
				return err
			}
		}
	}
	if err := wRows.Err(); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE wt_sessions
		    SET unread_panes = ?,
		        unread_windows = ?,
		        rev = CASE
		              WHEN unread_panes <> ? OR unread_windows <> ? THEN rev + 1
		              ELSE rev
		            END,
		        updated_at = datetime('now')
		  WHERE session_name = ?`,
		unreadPanes,
		unreadWindows,
		unreadPanes,
		unreadWindows,
		sessionName,
	); err != nil {
		return err
	}
	return nil
}

func (s *Store) PurgeWatchtowerSessions(ctx context.Context, activeSessions []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if len(activeSessions) == 0 {
		for _, stmt := range []string{
			"DELETE FROM wt_panes",
			"DELETE FROM wt_windows",
			"DELETE FROM wt_sessions",
		} {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return err
			}
		}
		return tx.Commit()
	}

	placeholders := sqlPlaceholders(len(activeSessions))
	args := stringsToAny(activeSessions)
	for _, item := range []struct {
		table  string
		column string
	}{
		{table: "wt_panes", column: "session_name"},
		{table: "wt_windows", column: "session_name"},
		{table: "wt_sessions", column: "session_name"},
	} {
		query := "DELETE FROM " + item.table + " WHERE " + item.column + " NOT IN (" + placeholders + ")" //nolint:gosec // table/column values are fixed literals
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) PurgeWatchtowerWindows(ctx context.Context, sessionName string, activeWindowIndices []int) error {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return errors.New("session name is required")
	}

	if len(activeWindowIndices) == 0 {
		_, err := s.db.ExecContext(ctx,
			"DELETE FROM wt_windows WHERE session_name = ?",
			sessionName,
		)
		return err
	}

	placeholders := sqlPlaceholders(len(activeWindowIndices))
	args := make([]any, 0, len(activeWindowIndices)+1)
	args = append(args, sessionName)
	for _, value := range activeWindowIndices {
		args = append(args, value)
	}
	query := "DELETE FROM wt_windows WHERE session_name = ? AND window_index NOT IN (" + placeholders + ")" //nolint:gosec // placeholders are generated literals
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Store) PurgeWatchtowerPanes(ctx context.Context, sessionName string, activePaneIDs []string) error {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return errors.New("session name is required")
	}

	if len(activePaneIDs) == 0 {
		_, err := s.db.ExecContext(ctx,
			"DELETE FROM wt_panes WHERE session_name = ?",
			sessionName,
		)
		return err
	}

	placeholders := sqlPlaceholders(len(activePaneIDs))
	args := make([]any, 0, len(activePaneIDs)+1)
	args = append(args, sessionName)
	args = append(args, stringsToAny(activePaneIDs)...)
	query := "DELETE FROM wt_panes WHERE session_name = ? AND pane_id NOT IN (" + placeholders + ")" //nolint:gosec // placeholders are generated literals
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func formatStoreValueTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func sqlPlaceholders(count int) string {
	if count <= 0 {
		return ""
	}
	parts := make([]string, count)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ", ")
}

func stringsToAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, item := range values {
		out = append(out, item)
	}
	return out
}
