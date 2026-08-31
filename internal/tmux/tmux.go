package tmux

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ErrorKind represents error kind data.
type ErrorKind string

const (
	// ErrKindNotFound reports that tmux is not installed or unavailable.
	ErrKindNotFound ErrorKind = "TMUX_NOT_FOUND"
	// ErrKindSessionNotFound reports that a tmux session does not exist.
	ErrKindSessionNotFound ErrorKind = "SESSION_NOT_FOUND"
	// ErrKindSessionExists reports that a tmux session already exists.
	ErrKindSessionExists ErrorKind = "SESSION_ALREADY_EXISTS"
	// ErrKindServerNotRunning reports that the tmux server is not running.
	ErrKindServerNotRunning ErrorKind = "TMUX_SERVER_NOT_RUNNING"
	// ErrKindCommandFailed reports that a tmux command failed.
	ErrKindCommandFailed ErrorKind = "TMUX_COMMAND_FAILED"
	// ErrKindInvalidIdentifier reports that a tmux identifier is invalid.
	ErrKindInvalidIdentifier ErrorKind = "INVALID_IDENTIFIER"
)

// Error represents error data.
type Error struct {
	Kind ErrorKind
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Kind)
}

func (e *Error) Unwrap() error {
	return e.Err
}

// IsKind reports whether kind.
func IsKind(err error, kind ErrorKind) bool {
	var terr *Error
	return errors.As(err, &terr) && terr.Kind == kind
}

// Session represents session data.
type Session struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Windows    int       `json:"windows"`
	Attached   int       `json:"attached"`
	CreatedAt  time.Time `json:"createdAt"`
	ActivityAt time.Time `json:"activityAt"`
}

// PaneSnapshot represents pane snapshot data.
type PaneSnapshot struct {
	Command string
	Panes   int
}

// fieldSep separates the fields of every tmux -F format this package requests.
// tmux does not escape format values, and pane_current_path, pane_start_command
// and pane_current_command can hold any byte a path or command line can hold —
// a tab among them. Splitting such a line on a tab shifts every later field,
// silently corrupting pane geometry. ASCII Unit Separator is not produced by
// tmux itself and does not occur in real paths or command lines.
const fieldSep = "\x1f"

const (
	cmdNewSession                     = "new-session"
	listSessionsFormatWithActivity    = "#{session_id}" + fieldSep + "#{session_name}" + fieldSep + "#{session_windows}" + fieldSep + "#{session_attached}" + fieldSep + "#{session_created}" + fieldSep + "#{session_activity}"
	listSessionsFormatWithoutActivity = "#{session_id}" + fieldSep + "#{session_name}" + fieldSep + "#{session_windows}" + fieldSep + "#{session_attached}" + fieldSep + "#{session_created}"
	createSessionFormat               = "#{session_id}" + fieldSep + "#{session_name}"
	listWindowsFormat                 = "#{session_name}" + fieldSep + "#{window_id}" + fieldSep + "#{window_index}" + fieldSep + "#{window_name}" + fieldSep + "#{window_active}" + fieldSep + "#{window_panes}" + fieldSep + "#{window_layout}"
	listPanesFormat                   = "#{session_name}" + fieldSep + "#{window_index}" + fieldSep + "#{pane_index}" + fieldSep + "#{pane_id}" + fieldSep + "#{pane_title}" + fieldSep + "#{pane_active}" + fieldSep + "#{pane_tty}" + fieldSep + "#{pane_current_path}" + fieldSep + "#{pane_start_command}" + fieldSep + "#{pane_current_command}" + fieldSep + "#{pane_left}" + fieldSep + "#{pane_top}" + fieldSep + "#{pane_width}" + fieldSep + "#{pane_height}"
	activePaneCommandsFormat          = "#{session_name}" + fieldSep + "#{window_active}" + fieldSep + "#{pane_active}" + fieldSep + "#{pane_start_command}" + fieldSep + "#{pane_current_command}"
	newWindowFormat                   = "#{window_id}" + fieldSep + "#{window_index}" + fieldSep + "#{pane_id}"
)

// Window represents window data.
type Window struct {
	Session string `json:"session"`
	ID      string `json:"id"`
	Index   int    `json:"index"`
	Name    string `json:"name"`
	Active  bool   `json:"active"`
	Panes   int    `json:"panes"`
	Layout  string `json:"layout,omitempty"`
}

// Pane represents pane data.
type Pane struct {
	Session        string `json:"session"`
	WindowIndex    int    `json:"windowIndex"`
	PaneIndex      int    `json:"paneIndex"`
	PaneID         string `json:"paneId"`
	Title          string `json:"title"`
	Active         bool   `json:"active"`
	TTY            string `json:"tty"`
	CurrentPath    string `json:"currentPath,omitempty"`
	StartCommand   string `json:"startCommand,omitempty"`
	CurrentCommand string `json:"currentCommand,omitempty"`
	Left           int    `json:"left,omitempty"`
	Top            int    `json:"top,omitempty"`
	Width          int    `json:"width,omitempty"`
	Height         int    `json:"height,omitempty"`
}

// NewWindowResult represents new window result data.
type NewWindowResult struct {
	ID     string
	Index  int
	PaneID string
}

// runners are package runners / prefixes that should be skipped when
// inferring the actual tool from pane_start_command.
var runners = map[string]bool{
	"npx": true, "bunx": true, "pnpm": true, "yarn": true,
	"env": true, "sudo": true, "exec": true,
}

// inferCommand parses a command string and extracts the tool name,
// skipping package runners (npx, bunx), env vars (KEY=val), and flags.
// Returns the basename of the first meaningful token.
//
// Examples:
//
//	"claude --resume"        → "claude"
//	"npx codex --full-auto"  → "codex"
//	"NODE_ENV=prod claude"   → "claude"
//	"/usr/local/bin/claude"  → "claude"
func inferCommand(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		s = s[1 : len(s)-1]
	}
	for _, part := range strings.Fields(s) {
		lower := strings.ToLower(part)
		if strings.Contains(lower, "=") {
			continue
		}
		if strings.HasPrefix(lower, "-") {
			continue
		}
		base := lower
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		if runners[base] {
			continue
		}
		for _, ext := range []string{".js", ".ts", ".mjs", ".cjs", ".py", ".rb", ".pl"} {
			if strings.HasSuffix(base, ext) {
				base = base[:len(base)-len(ext)]
				break
			}
		}
		if base != "" {
			return base
		}
	}
	return ""
}

// SessionHash handles session hash.
func SessionHash(name string, epoch int64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", name, epoch)))
	return fmt.Sprintf("%x", h[:6])
}

const (
	tmuxOn  = "on"
	tmuxOff = "off"

	dirVertical   = "vertical"
	dirHorizontal = "horizontal"
)

const (
	cmdNewWindow   = "new-window"
	cmdSplitWindow = "split-window"
	tableRoot      = "root"
)

const (
	errWindowOrderMismatch = "tmux window order does not match live windows"
	errInvalidSplitDir     = "invalid split direction"
	errPaneIDRequired      = "pane ID is required"
	errSessionRequired     = "tmux session is required"
)

// ensureWebMouseBindings patches a subset of tmux default mouse bindings to
// behave consistently in browser terminals:
//  1. Keep pane context menu open after button release (-O).
//  2. Disable default double/triple-click auto-copy popup behavior.
//  3. Prevent drag-select from exiting copy-mode on mouse release, which
//     would cause the view to jump to the bottom and clear the selection.
//
// It also enables OSC 52 clipboard output so that copy-mode operations
// propagate to the system clipboard via the browser terminal (xterm.js).
//
// The patch is idempotent and only rewrites known default patterns.
func ensureWebMouseBindings(ctx context.Context) error {
	// Enable OSC 52 clipboard output for copy-mode operations.
	// The default "external" (tmux 3.2+) only passes through application
	// OSC 52 but does not emit it for tmux's own copy commands.
	_, _ = run(ctx, "set-option", "-s", "set-clipboard", "on")

	patchers := []struct {
		table string
		key   string
		patch func(string) (string, bool)
	}{
		{table: tableRoot, key: "MouseDown3Pane", patch: patchMouseDown3PaneBinding},
		{table: tableRoot, key: "DoubleClick1Pane", patch: patchDoubleClick1PaneBinding},
		{table: tableRoot, key: "TripleClick1Pane", patch: patchTripleClick1PaneBinding},
		{table: "copy-mode", key: "MouseDragEnd1Pane", patch: patchCopyModeDragEndBinding},
		{table: "copy-mode-vi", key: "MouseDragEnd1Pane", patch: patchCopyModeDragEndBinding},
	}

	var firstErr error
	for _, item := range patchers {
		if err := patchBinding(ctx, item.table, item.key, item.patch); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func patchBinding(
	ctx context.Context,
	table, key string,
	patch func(line string) (string, bool),
) error {
	out, err := run(ctx, "list-keys", "-T", table, key)
	if err != nil {
		// Binding may not exist in this table; nothing to patch.
		return nil
	}

	line := strings.TrimSpace(out)
	if line == "" {
		return nil
	}

	patched, changed := patch(line)
	if !changed || patched == line {
		return nil
	}

	tmpFile, err := os.CreateTemp("", "sentinel-tmux-bind-*.conf")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmpFile.WriteString(patched + "\n"); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	_, err = run(ctx, "source-file", tmpPath)
	return err
}

func patchMouseDown3PaneBinding(line string) (string, bool) {
	if !strings.Contains(line, "bind-key -T root MouseDown3Pane") {
		return line, false
	}
	if strings.Contains(line, "display-menu -O") {
		return line, false
	}
	if !strings.Contains(line, "display-menu ") {
		return line, false
	}
	patched := strings.Replace(line, "display-menu ", "display-menu -O -M ", 1)
	return patched, patched != line
}

func patchDoubleClick1PaneBinding(line string) (string, bool) {
	const before = "{ copy-mode -H ; send-keys -X select-word ; run-shell -d 0.3 ; send-keys -X copy-pipe-and-cancel }"
	const after = "{ send-keys -M }"
	if !strings.Contains(line, "bind-key -T root DoubleClick1Pane") {
		return line, false
	}
	if !strings.Contains(line, before) {
		return line, false
	}
	patched := strings.Replace(line, before, after, 1)
	return patched, patched != line
}

func patchTripleClick1PaneBinding(line string) (string, bool) {
	const before = "{ copy-mode -H ; send-keys -X select-line ; run-shell -d 0.3 ; send-keys -X copy-pipe-and-cancel }"
	const after = "{ send-keys -M }"
	if !strings.Contains(line, "bind-key -T root TripleClick1Pane") {
		return line, false
	}
	if !strings.Contains(line, before) {
		return line, false
	}
	patched := strings.Replace(line, before, after, 1)
	return patched, patched != line
}

// patchCopyModeDragEndBinding replaces -and-cancel variants in
// copy-mode MouseDragEnd1Pane bindings with -no-clear so that
// releasing the mouse after a drag-select keeps copy-mode active
// and the selection visible instead of jumping to the bottom.
func patchCopyModeDragEndBinding(line string) (string, bool) {
	if !strings.Contains(line, "MouseDragEnd1Pane") {
		return line, false
	}
	patched := line
	patched = strings.Replace(patched, "copy-pipe-and-cancel", "copy-pipe-no-clear", 1)
	patched = strings.Replace(patched, "copy-selection-and-cancel", "copy-selection-no-clear", 1)
	return patched, patched != line
}

func valueAt(parts []string, idx int) string {
	if idx < 0 || idx >= len(parts) {
		return ""
	}
	return parts[idx]
}

func parseNewWindowOutput(out string) (NewWindowResult, error) {
	line := strings.TrimSpace(out)
	parts := strings.Split(line, fieldSep)
	if len(parts) != 3 {
		return NewWindowResult{}, &Error{
			Kind: ErrKindCommandFailed,
			Msg:  fmt.Sprintf("tmux new-window returned unexpected output: %q", line),
		}
	}
	windowID := strings.TrimSpace(parts[0])
	if !strings.HasPrefix(windowID, "@") {
		return NewWindowResult{}, &Error{
			Kind: ErrKindCommandFailed,
			Msg:  fmt.Sprintf("tmux new-window returned invalid window id: %q", windowID),
		}
	}
	index, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || index < 0 {
		return NewWindowResult{}, &Error{
			Kind: ErrKindCommandFailed,
			Msg:  fmt.Sprintf("tmux new-window returned invalid index: %q", parts[1]),
			Err:  err,
		}
	}
	paneID := strings.TrimSpace(parts[2])
	if !strings.HasPrefix(paneID, "%") {
		return NewWindowResult{}, &Error{
			Kind: ErrKindCommandFailed,
			Msg:  fmt.Sprintf("tmux new-window returned invalid pane id: %q", paneID),
		}
	}
	return NewWindowResult{
		ID:     windowID,
		Index:  index,
		PaneID: paneID,
	}, nil
}

func parseSplitPaneOutput(out string) (string, error) {
	paneID := strings.TrimSpace(out)
	if !strings.HasPrefix(paneID, "%") {
		return "", &Error{
			Kind: ErrKindCommandFailed,
			Msg:  fmt.Sprintf("tmux split-window returned invalid pane id: %q", paneID),
		}
	}
	return paneID, nil
}

func nextWindowIndexFromListOutput(out string) (int, bool) {
	maxIndex := -1
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		raw := strings.TrimSpace(line)
		if raw == "" {
			continue
		}
		index, err := strconv.Atoi(raw)
		if err != nil || index < 0 {
			continue
		}
		if index > maxIndex {
			maxIndex = index
		}
	}
	if maxIndex < 0 {
		return 0, false
	}
	return maxIndex + 1, true
}

var run = func(ctx context.Context, args ...string) (string, error) { // var enables test injection
	return executeTmuxCommand(ctx, "tmux", args, args)
}

var execCommandContext = exec.CommandContext // var enables test injection

// executeTmuxCommand runs one tmux invocation, either directly or through a
// wrapper (systemd-run for session creation, the user-switch command for
// multi-user targets). tmuxArgs is the tmux argument vector used for error
// messages, which is not the same as commandArgs once a wrapper is involved.
func executeTmuxCommand(ctx context.Context, name string, commandArgs, tmuxArgs []string) (string, error) {
	cmd := execCommandContext(ctx, name, commandArgs...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if name != "tmux" && errors.Is(err, exec.ErrNotFound) {
			return "", &Error{
				Kind: ErrKindCommandFailed,
				Msg:  name + " is required to isolate tmux sessions from Sentinel",
				Err:  err,
			}
		}
		return "", classifyError(err, stderr.String(), tmuxArgs)
	}
	return stdout.String(), nil
}

func classifyError(err error, stderr string, args []string) error {
	// Let context cancellation/timeout propagate idiomatically instead of being
	// flattened into a generic "tmux ... failed" error.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, exec.ErrNotFound) {
		return &Error{
			Kind: ErrKindNotFound,
			Msg:  "tmux binary not found",
			Err:  err,
		}
	}

	msg := strings.ToLower(strings.TrimSpace(stderr))
	switch {
	case strings.Contains(msg, "can't find session"), strings.Contains(msg, "no such session"):
		return &Error{Kind: ErrKindSessionNotFound, Msg: strings.TrimSpace(stderr), Err: err}
	case strings.Contains(msg, "duplicate session"), strings.Contains(msg, "already exists"):
		return &Error{Kind: ErrKindSessionExists, Msg: strings.TrimSpace(stderr), Err: err}
	case isServerNotRunningMessage(msg):
		return &Error{Kind: ErrKindServerNotRunning, Msg: strings.TrimSpace(stderr), Err: err}
	default:
		return &Error{
			Kind: ErrKindCommandFailed,
			Msg:  fmt.Sprintf("tmux %s failed: %s", strings.Join(args, " "), strings.TrimSpace(stderr)),
			Err:  err,
		}
	}
}

func parseSessionListOutput(out string) []Session {
	if strings.TrimSpace(out) == "" {
		return []Session{}
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	sessions := make([]Session, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, fieldSep)
		if len(parts) < 5 {
			continue
		}
		windows, _ := strconv.Atoi(parts[2])
		attached, _ := strconv.Atoi(parts[3])
		createdEpoch, _ := strconv.ParseInt(parts[4], 10, 64)
		activityEpoch := createdEpoch
		if len(parts) >= 6 {
			activityEpoch, _ = strconv.ParseInt(parts[5], 10, 64)
		}
		sessions = append(sessions, Session{
			ID:         parts[0],
			Name:       parts[1],
			Windows:    windows,
			Attached:   attached,
			CreatedAt:  time.Unix(createdEpoch, 0).UTC(),
			ActivityAt: time.Unix(activityEpoch, 0).UTC(),
		})
	}
	return sessions
}

func getSessionVia(ctx context.Context, runner func(context.Context, ...string) (string, error), name string) (Session, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Session{}, &Error{Kind: ErrKindInvalidIdentifier, Msg: errSessionRequired}
	}
	args := []string{"list-sessions", "-F", listSessionsFormatWithActivity}
	out, err := runner(ctx, args...)
	if err != nil && shouldRetryListSessionsWithoutActivity(err) {
		args[len(args)-1] = listSessionsFormatWithoutActivity
		out, err = runner(ctx, args...)
	}
	if err != nil {
		return Session{}, err
	}
	for _, session := range parseSessionListOutput(out) {
		if session.Name == name {
			return session, nil
		}
	}
	return Session{}, &Error{Kind: ErrKindSessionNotFound, Msg: "tmux session not found"}
}

func killSessionByIDVia(ctx context.Context, runner func(context.Context, ...string) (string, error), sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if !validSessionID(sessionID) {
		return &Error{Kind: ErrKindInvalidIdentifier, Msg: "tmux session ID must match $<number>"}
	}
	_, err := runner(ctx, "kill-session", "-t", sessionID)
	return err
}

func validSessionID(sessionID string) bool {
	if len(sessionID) < 2 || sessionID[0] != '$' {
		return false
	}
	_, err := strconv.ParseUint(sessionID[1:], 10, 64)
	return err == nil
}

func createSessionWithIDVia(
	ctx context.Context,
	runner func(context.Context, ...string) (string, error),
	name, cwd string,
) (Session, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Session{}, &Error{Kind: ErrKindInvalidIdentifier, Msg: errSessionRequired}
	}
	args := []string{cmdNewSession, "-d", "-P", "-F", createSessionFormat, "-s", name}
	if strings.TrimSpace(cwd) != "" {
		args = append(args, "-c", cwd)
	}
	out, err := runner(ctx, args...)
	if err != nil {
		return Session{}, err
	}
	parts := strings.Split(strings.TrimSpace(out), fieldSep)
	if len(parts) != 2 || !validSessionID(parts[0]) || strings.TrimSpace(parts[1]) == "" {
		return Session{}, &Error{Kind: ErrKindCommandFailed, Msg: "tmux create returned an invalid session identity"}
	}
	return Session{ID: parts[0], Name: parts[1]}, nil
}

func shouldRetryListSessionsWithoutActivity(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "session_activity") {
		return false
	}
	return strings.Contains(msg, "unknown format") ||
		strings.Contains(msg, "bad format") ||
		strings.Contains(msg, "invalid format")
}

func isServerNotRunningMessage(msg string) bool {
	return strings.Contains(msg, "failed to connect to server") ||
		strings.Contains(msg, "can't connect to server") ||
		strings.Contains(msg, "no server running") ||
		(strings.Contains(msg, "error connecting to") && strings.Contains(msg, "no such file or directory"))
}
