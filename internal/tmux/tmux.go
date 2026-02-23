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

type ErrorKind string

const (
	ErrKindNotFound          ErrorKind = "TMUX_NOT_FOUND"
	ErrKindSessionNotFound   ErrorKind = "SESSION_NOT_FOUND"
	ErrKindSessionExists     ErrorKind = "SESSION_ALREADY_EXISTS"
	ErrKindServerNotRunning  ErrorKind = "TMUX_SERVER_NOT_RUNNING"
	ErrKindCommandFailed     ErrorKind = "TMUX_COMMAND_FAILED"
	ErrKindInvalidIdentifier ErrorKind = "INVALID_IDENTIFIER"
)

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

func IsKind(err error, kind ErrorKind) bool {
	var terr *Error
	return errors.As(err, &terr) && terr.Kind == kind
}

type Session struct {
	Name       string    `json:"name"`
	Windows    int       `json:"windows"`
	Attached   int       `json:"attached"`
	CreatedAt  time.Time `json:"createdAt"`
	ActivityAt time.Time `json:"activityAt"`
}

type PaneSnapshot struct {
	Command string
	Panes   int
}

const (
	listSessionsFormatWithActivity    = "#{session_name}\t#{session_windows}\t#{session_attached}\t#{session_created}\t#{session_activity}"
	listSessionsFormatWithoutActivity = "#{session_name}\t#{session_windows}\t#{session_attached}\t#{session_created}"
)

type Window struct {
	Session string `json:"session"`
	Index   int    `json:"index"`
	Name    string `json:"name"`
	Active  bool   `json:"active"`
	Panes   int    `json:"panes"`
	Layout  string `json:"layout,omitempty"`
}

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

type NewWindowResult struct {
	Index  int
	PaneID string
}

func ListSessions(ctx context.Context) ([]Session, error) {
	out, err := run(ctx, "list-sessions", "-F", listSessionsFormatWithActivity)
	if err != nil {
		if IsKind(err, ErrKindServerNotRunning) {
			return []Session{}, nil
		}
		if !shouldRetryListSessionsWithoutActivity(err) {
			return nil, err
		}

		out, err = run(ctx, "list-sessions", "-F", listSessionsFormatWithoutActivity)
		if err != nil {
			if IsKind(err, ErrKindServerNotRunning) {
				return []Session{}, nil
			}
			return nil, err
		}
	}
	return parseSessionListOutput(out), nil
}

// runners are package runners / prefixes that should be skipped when
// inferring the actual tool from pane_start_command.
var runners = map[string]bool{
	"npx": true, "bunx": true, "pnpm": true, "yarn": true,
	"env": true, "sudo": true, "exec": true,
}

func ListActivePaneCommands(ctx context.Context) (map[string]PaneSnapshot, error) {
	out, err := run(ctx, "list-panes", "-a", "-F", "#{session_name}\t#{window_active}\t#{pane_active}\t#{pane_start_command}\t#{pane_current_command}")
	if err != nil {
		if IsKind(err, ErrKindServerNotRunning) {
			return map[string]PaneSnapshot{}, nil
		}
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return map[string]PaneSnapshot{}, nil
	}

	result := make(map[string]PaneSnapshot)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) != 5 {
			continue
		}
		name := parts[0]
		snap := result[name]
		snap.Panes++
		if parts[1] == "1" && parts[2] == "1" {
			cmd := inferCommand(parts[3])
			if cmd == "" {
				cmd = inferCommand(parts[4])
			}
			if cmd == "" {
				cmd = parts[4]
			}
			snap.Command = cmd
		}
		result[name] = snap
	}
	return result, nil
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

func CapturePane(ctx context.Context, session string) (string, error) {
	out, err := run(ctx, "capture-pane", "-t", session+":", "-p", "-S", "-3")
	if err != nil {
		return "", nil
	}
	lines := strings.Split(out, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "" {
			return trimmed, nil
		}
	}
	return "", nil
}

func SessionHash(name string, epoch int64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", name, epoch)))
	return fmt.Sprintf("%x", h[:6])
}

func CreateSession(ctx context.Context, name, cwd string) error {
	args := []string{"new-session", "-d", "-s", name}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	_, err := run(ctx, args...)
	return err
}

// SetSessionMouse toggles tmux mouse support for a target session.
// When enabled, wheel gestures are handled by tmux copy-mode instead of
// being interpreted as terminal cursor keys by applications.
func SetSessionMouse(ctx context.Context, session string, enabled bool) error {
	value := "off"
	if enabled {
		value = "on"
	}
	_, err := run(ctx, "set-option", "-t", session, "mouse", value)
	return err
}

// EnsureWebMouseBindings patches a subset of tmux default mouse bindings to
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
func EnsureWebMouseBindings(ctx context.Context) error {
	// Enable OSC 52 clipboard output for copy-mode operations.
	// The default "external" (tmux 3.2+) only passes through application
	// OSC 52 but does not emit it for tmux's own copy commands.
	_, _ = run(ctx, "set-option", "-s", "set-clipboard", "on")

	patchers := []struct {
		table string
		key   string
		patch func(string) (string, bool)
	}{
		{table: "root", key: "MouseDown3Pane", patch: patchMouseDown3PaneBinding},
		{table: "root", key: "DoubleClick1Pane", patch: patchDoubleClick1PaneBinding},
		{table: "root", key: "TripleClick1Pane", patch: patchTripleClick1PaneBinding},
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

func RenameSession(ctx context.Context, session, newName string) error {
	_, err := run(ctx, "rename-session", "-t", session, newName)
	return err
}

func RenameWindow(ctx context.Context, session string, index int, name string) error {
	target := fmt.Sprintf("%s:%d", session, index)
	_, err := run(ctx, "rename-window", "-t", target, name)
	return err
}

func RenamePane(ctx context.Context, paneID, title string) error {
	_, err := run(ctx, "select-pane", "-t", paneID, "-T", title)
	return err
}

func KillSession(ctx context.Context, session string) error {
	_, err := run(ctx, "kill-session", "-t", session)
	return err
}

func SelectWindow(ctx context.Context, session string, index int) error {
	target := fmt.Sprintf("%s:%d", session, index)
	_, err := run(ctx, "select-window", "-t", target)
	return err
}

func SelectPane(ctx context.Context, paneID string) error {
	_, err := run(ctx, "select-pane", "-t", paneID)
	return err
}

func NewWindow(ctx context.Context, session string) (NewWindowResult, error) {
	target := fmt.Sprintf("%s:", session)
	if indexesOut, listErr := run(ctx, "list-windows", "-t", session, "-F", "#{window_index}"); listErr == nil {
		if nextIndex, ok := nextWindowIndexFromListOutput(indexesOut); ok {
			target = fmt.Sprintf("%s:%d", session, nextIndex)
		}
	}
	args := []string{"new-window", "-P", "-F", "#{window_index}\t#{pane_id}", "-t", target}
	if pathOut, pathErr := run(ctx, "display-message", "-t", session, "-p", "#{session_path}"); pathErr == nil {
		if sp := strings.TrimSpace(pathOut); sp != "" {
			args = append(args, "-c", sp)
		}
	}
	out, err := run(ctx, args...)
	if err != nil {
		return NewWindowResult{}, err
	}
	result, parseErr := parseNewWindowOutput(out)
	if parseErr != nil {
		return NewWindowResult{}, parseErr
	}
	return result, nil
}

func NewWindowAt(ctx context.Context, session string, index int, name, cwd string) error {
	target := fmt.Sprintf("%s:%d", session, index)
	args := []string{"new-window", "-d", "-t", target}
	if strings.TrimSpace(name) != "" {
		args = append(args, "-n", name)
	}
	if strings.TrimSpace(cwd) != "" {
		args = append(args, "-c", cwd)
	}
	_, err := run(ctx, args...)
	return err
}

func KillWindow(ctx context.Context, session string, index int) error {
	target := fmt.Sprintf("%s:%d", session, index)
	_, err := run(ctx, "kill-window", "-t", target)
	return err
}

func KillPane(ctx context.Context, paneID string) error {
	_, err := run(ctx, "kill-pane", "-t", paneID)
	return err
}

func SplitPane(ctx context.Context, paneID, direction string) (string, error) {
	args := []string{"split-window", "-t", paneID}
	switch direction {
	case "vertical":
		args = append(args, "-h")
	case "horizontal":
		args = append(args, "-v")
	default:
		return "", &Error{Kind: ErrKindInvalidIdentifier, Msg: "invalid split direction"}
	}
	args = append(args, "-P", "-F", "#{pane_id}")
	out, err := run(ctx, args...)
	if err != nil {
		return "", err
	}
	createdPaneID, parseErr := parseSplitPaneOutput(out)
	if parseErr != nil {
		return "", parseErr
	}
	return createdPaneID, nil
}

func SplitPaneIn(ctx context.Context, paneID, direction, cwd string) (string, error) {
	args := []string{"split-window", "-d", "-P", "-F", "#{pane_id}", "-t", paneID}
	switch direction {
	case "vertical":
		args = append(args, "-h")
	case "horizontal":
		args = append(args, "-v")
	default:
		return "", &Error{Kind: ErrKindInvalidIdentifier, Msg: "invalid split direction"}
	}
	if strings.TrimSpace(cwd) != "" {
		args = append(args, "-c", cwd)
	}
	out, err := run(ctx, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func SelectLayout(ctx context.Context, session string, index int, layout string) error {
	target := fmt.Sprintf("%s:%d", session, index)
	_, err := run(ctx, "select-layout", "-t", target, layout)
	return err
}

func SendKeys(ctx context.Context, paneID, keys string, enter bool) error {
	keys = strings.TrimSpace(keys)
	if keys != "" {
		if _, err := run(ctx, "send-keys", "-t", paneID, "-l", keys); err != nil {
			return err
		}
	}
	if enter {
		if _, err := run(ctx, "send-keys", "-t", paneID, "C-m"); err != nil {
			return err
		}
	}
	return nil
}

func ListWindows(ctx context.Context, session string) ([]Window, error) {
	out, err := run(ctx, "list-windows", "-t", session, "-F", "#{session_name}\t#{window_index}\t#{window_name}\t#{window_active}\t#{window_panes}\t#{window_layout}")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return []Window{}, nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	windows := make([]Window, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) < 5 {
			continue
		}
		idx, _ := strconv.Atoi(parts[1])
		panes, _ := strconv.Atoi(parts[4])
		layout := ""
		if len(parts) > 5 {
			layout = parts[5]
		}
		windows = append(windows, Window{
			Session: parts[0],
			Index:   idx,
			Name:    parts[2],
			Active:  parts[3] == "1",
			Panes:   panes,
			Layout:  layout,
		})
	}
	return windows, nil
}

func ListPanes(ctx context.Context, session string) ([]Pane, error) {
	out, err := run(ctx, "list-panes", "-a", "-F", "#{session_name}\t#{window_index}\t#{pane_index}\t#{pane_id}\t#{pane_title}\t#{pane_active}\t#{pane_tty}\t#{pane_current_path}\t#{pane_start_command}\t#{pane_current_command}\t#{pane_left}\t#{pane_top}\t#{pane_width}\t#{pane_height}")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return []Pane{}, nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	panes := make([]Pane, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) < 7 {
			continue
		}
		if parts[0] != session {
			continue
		}
		windowIndex, _ := strconv.Atoi(parts[1])
		paneIndex, _ := strconv.Atoi(parts[2])
		left, _ := strconv.Atoi(valueAt(parts, 10))
		top, _ := strconv.Atoi(valueAt(parts, 11))
		width, _ := strconv.Atoi(valueAt(parts, 12))
		height, _ := strconv.Atoi(valueAt(parts, 13))
		panes = append(panes, Pane{
			Session:        parts[0],
			WindowIndex:    windowIndex,
			PaneIndex:      paneIndex,
			PaneID:         parts[3],
			Title:          parts[4],
			Active:         parts[5] == "1",
			TTY:            parts[6],
			CurrentPath:    valueAt(parts, 7),
			StartCommand:   valueAt(parts, 8),
			CurrentCommand: valueAt(parts, 9),
			Left:           left,
			Top:            top,
			Width:          width,
			Height:         height,
		})
	}
	return panes, nil
}

func CapturePaneLines(ctx context.Context, target string, lines int) (string, error) {
	if strings.TrimSpace(target) == "" {
		return "", &Error{Kind: ErrKindInvalidIdentifier, Msg: "target is required"}
	}
	if lines <= 0 {
		lines = 80
	}
	start := fmt.Sprintf("-%d", lines)
	out, err := run(ctx, "capture-pane", "-t", target, "-p", "-S", start)
	if err != nil {
		return "", err
	}
	return out, nil
}

func SessionExists(ctx context.Context, session string) (bool, error) {
	_, err := run(ctx, "has-session", "-t", session)
	if err != nil {
		if IsKind(err, ErrKindSessionNotFound) || IsKind(err, ErrKindServerNotRunning) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func valueAt(parts []string, idx int) string {
	if idx < 0 || idx >= len(parts) {
		return ""
	}
	return parts[idx]
}

func parseNewWindowOutput(out string) (NewWindowResult, error) {
	line := strings.TrimSpace(out)
	parts := strings.Split(line, "\t")
	if len(parts) != 2 {
		return NewWindowResult{}, &Error{
			Kind: ErrKindCommandFailed,
			Msg:  fmt.Sprintf("tmux new-window returned unexpected output: %q", line),
		}
	}
	index, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || index < 0 {
		return NewWindowResult{}, &Error{
			Kind: ErrKindCommandFailed,
			Msg:  fmt.Sprintf("tmux new-window returned invalid index: %q", parts[0]),
			Err:  err,
		}
	}
	paneID := strings.TrimSpace(parts[1])
	if !strings.HasPrefix(paneID, "%") {
		return NewWindowResult{}, &Error{
			Kind: ErrKindCommandFailed,
			Msg:  fmt.Sprintf("tmux new-window returned invalid pane id: %q", paneID),
		}
	}
	return NewWindowResult{
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

var run = func(ctx context.Context, args ...string) (string, error) { //nolint:gochecknoglobals // var enables test injection
	cmd := exec.CommandContext(ctx, "tmux", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", classifyError(err, stderr.String(), args)
	}
	return stdout.String(), nil
}

func classifyError(err error, stderr string, args []string) error {
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
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			continue
		}
		windows, _ := strconv.Atoi(parts[1])
		attached, _ := strconv.Atoi(parts[2])
		createdEpoch, _ := strconv.ParseInt(parts[3], 10, 64)
		activityEpoch := createdEpoch
		if len(parts) >= 5 {
			activityEpoch, _ = strconv.ParseInt(parts[4], 10, 64)
		}
		sessions = append(sessions, Session{
			Name:       parts[0],
			Windows:    windows,
			Attached:   attached,
			CreatedAt:  time.Unix(createdEpoch, 0).UTC(),
			ActivityAt: time.Unix(activityEpoch, 0).UTC(),
		})
	}
	return sessions
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
