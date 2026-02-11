package tmux

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
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

type Window struct {
	Session string `json:"session"`
	Index   int    `json:"index"`
	Name    string `json:"name"`
	Active  bool   `json:"active"`
	Panes   int    `json:"panes"`
}

type Pane struct {
	Session     string `json:"session"`
	WindowIndex int    `json:"windowIndex"`
	PaneIndex   int    `json:"paneIndex"`
	PaneID      string `json:"paneId"`
	Title       string `json:"title"`
	Active      bool   `json:"active"`
	TTY         string `json:"tty"`
}

func ListSessions(ctx context.Context) ([]Session, error) {
	out, err := run(ctx, "list-sessions", "-F", "#{session_name}\t#{session_windows}\t#{session_attached}\t#{session_created}\t#{session_activity}")
	if err != nil {
		if IsKind(err, ErrKindServerNotRunning) {
			return []Session{}, nil
		}
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return []Session{}, nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	sessions := make([]Session, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) != 5 {
			continue
		}
		windows, _ := strconv.Atoi(parts[1])
		attached, _ := strconv.Atoi(parts[2])
		createdEpoch, _ := strconv.ParseInt(parts[3], 10, 64)
		activityEpoch, _ := strconv.ParseInt(parts[4], 10, 64)
		sessions = append(sessions, Session{
			Name:       parts[0],
			Windows:    windows,
			Attached:   attached,
			CreatedAt:  time.Unix(createdEpoch, 0).UTC(),
			ActivityAt: time.Unix(activityEpoch, 0).UTC(),
		})
	}
	return sessions, nil
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

func NewWindow(ctx context.Context, session string) error {
	target := fmt.Sprintf("%s:", session)
	_, err := run(ctx, "new-window", "-t", target)
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

func SplitPane(ctx context.Context, paneID, direction string) error {
	args := []string{"split-window", "-t", paneID}
	switch direction {
	case "vertical":
		args = append(args, "-h")
	case "horizontal":
		args = append(args, "-v")
	default:
		return &Error{Kind: ErrKindInvalidIdentifier, Msg: "invalid split direction"}
	}
	_, err := run(ctx, args...)
	return err
}

func ListWindows(ctx context.Context, session string) ([]Window, error) {
	out, err := run(ctx, "list-windows", "-t", session, "-F", "#{session_name}\t#{window_index}\t#{window_name}\t#{window_active}\t#{window_panes}")
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
		if len(parts) != 5 {
			continue
		}
		idx, _ := strconv.Atoi(parts[1])
		panes, _ := strconv.Atoi(parts[4])
		windows = append(windows, Window{
			Session: parts[0],
			Index:   idx,
			Name:    parts[2],
			Active:  parts[3] == "1",
			Panes:   panes,
		})
	}
	return windows, nil
}

func ListPanes(ctx context.Context, session string) ([]Pane, error) {
	out, err := run(ctx, "list-panes", "-a", "-F", "#{session_name}\t#{window_index}\t#{pane_index}\t#{pane_id}\t#{pane_title}\t#{pane_active}\t#{pane_tty}")
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
		if len(parts) != 7 {
			continue
		}
		if parts[0] != session {
			continue
		}
		windowIndex, _ := strconv.Atoi(parts[1])
		paneIndex, _ := strconv.Atoi(parts[2])
		panes = append(panes, Pane{
			Session:     parts[0],
			WindowIndex: windowIndex,
			PaneIndex:   paneIndex,
			PaneID:      parts[3],
			Title:       parts[4],
			Active:      parts[5] == "1",
			TTY:         parts[6],
		})
	}
	return panes, nil
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

func run(ctx context.Context, args ...string) (string, error) {
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
	case strings.Contains(msg, "failed to connect to server"):
		return &Error{Kind: ErrKindServerNotRunning, Msg: strings.TrimSpace(stderr), Err: err}
	default:
		return &Error{
			Kind: ErrKindCommandFailed,
			Msg:  fmt.Sprintf("tmux %s failed: %s", strings.Join(args, " "), strings.TrimSpace(stderr)),
			Err:  err,
		}
	}
}
