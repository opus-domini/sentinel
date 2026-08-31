package tmux

import (
	"context"
	"fmt"
	"regexp"
	"runtime"
	"strings"

	"github.com/opus-domini/sentinel/internal/userswitch"
)

// Service runs tmux commands for one target user. When User is non-empty,
// every command is wrapped according to UserSwitchMethod; when it is empty,
// commands go through the package-level run variable. The two methods that can
// start a tmux server keep an explicit branch so that the local case still
// routes through the systemd-run isolation in createSessionRun.
type Service struct {
	User string
}

func (s Service) run(ctx context.Context, args ...string) (string, error) {
	return runAsUser(ctx, s.User, args...)
}

// validUserRe restricts user names to safe characters (POSIX portable).
var validUserRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// SystemUsers holds the list of valid system users loaded at startup.
// Set from main.go after config.ReadSystemUsers().
var SystemUsers []string // set once at startup from main

// UserSwitchMethod controls how multi-user tmux commands are launched.
// Set from main.go after config.Load().
var UserSwitchMethod = userswitch.DefaultMethod(runtime.GOOS) // set once at startup from config

// BuildControlCommand returns the executable and arguments for a persistent
// tmux control-mode client. Multi-user targets use the same validated switch
// path as one-shot tmux commands.
func BuildControlCommand(user, session string) (string, []string, error) {
	user = strings.TrimSpace(user)
	session = strings.TrimSpace(session)
	if session == "" {
		return "", nil, &Error{Kind: ErrKindInvalidIdentifier, Msg: "session is required"}
	}
	if user != "" {
		if err := verifySystemUser(user); err != nil {
			return "", nil, &Error{Kind: ErrKindCommandFailed, Msg: err.Error()}
		}
	}
	args := []string{"-C", "attach-session", "-f", "active-pane,ignore-size", "-t", session}
	name, commandArgs, err := userswitch.BuildTmuxCommand(UserSwitchMethod, user, args, true)
	if err != nil {
		return "", nil, &Error{Kind: ErrKindCommandFailed, Msg: err.Error()}
	}
	return name, commandArgs, nil
}

// verifySystemUser checks that the username matches the safe character set
// and exists in the in-memory system users list.
func verifySystemUser(name string) error {
	if !validUserRe.MatchString(name) {
		return fmt.Errorf("invalid username %q", name)
	}
	for _, u := range SystemUsers {
		if u == name {
			return nil
		}
	}
	return fmt.Errorf("unknown system user %q", name)
}

// runAsUser executes a tmux command through the configured user switch method
// when user is non-empty. For the default (no user) case the package-level run
// variable is used so that tests can inject fakes.
// The user is validated against the system user database before execution
// to prevent command injection even when the allowlist is empty.
func runAsUser(ctx context.Context, user string, args ...string) (string, error) {
	if user == "" {
		return run(ctx, args...)
	}
	if err := verifySystemUser(user); err != nil {
		return "", &Error{Kind: ErrKindCommandFailed, Msg: err.Error()}
	}
	name, commandArgs, err := userswitch.BuildTmuxCommand(UserSwitchMethod, user, args, false)
	if err != nil {
		return "", &Error{Kind: ErrKindCommandFailed, Msg: err.Error()}
	}
	return executeTmuxCommand(ctx, name, commandArgs, args)
}

// ListSessions lists sessions.
func (s Service) ListSessions(ctx context.Context) ([]Session, error) {
	return listSessionsVia(ctx, s.run)
}

// GetSession returns one exact tmux session.
func (s Service) GetSession(ctx context.Context, name string) (Session, error) {
	return getSessionVia(ctx, s.run, name)
}

// ListActivePaneCommands lists active pane commands.
func (s Service) ListActivePaneCommands(ctx context.Context) (map[string]PaneSnapshot, error) {
	return listActivePaneCommandsVia(ctx, s.run)
}

// CapturePane captures pane.
func (s Service) CapturePane(ctx context.Context, session string) (string, error) {
	return capturePane(ctx, s.run, session)
}

// HasSession reports whether session.
func (s Service) HasSession(ctx context.Context, session string) bool {
	_, err := s.run(ctx, "has-session", "-t", session)
	return err == nil
}

// CreateSession creates session. The local case must go through the isolated
// runner so a newly started tmux server does not inherit Sentinel's cgroup.
func (s Service) CreateSession(ctx context.Context, name, cwd string) error {
	if s.User == "" {
		return CreateSession(ctx, name, cwd)
	}
	args := []string{cmdNewSession, "-d", "-s", name}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	_, err := s.run(ctx, args...)
	return err
}

// CreateSessionWithID creates a detached session and returns its stable runtime
// identity. Like CreateSession, the local case uses the isolated runner.
func (s Service) CreateSessionWithID(ctx context.Context, name, cwd string) (Session, error) {
	if s.User == "" {
		return CreateSessionWithID(ctx, name, cwd)
	}
	return createSessionWithIDVia(ctx, s.run, name, cwd)
}

// RenameSession renames session.
func (s Service) RenameSession(ctx context.Context, session, newName string) error {
	_, err := s.run(ctx, "rename-session", "-t", session, newName)
	return err
}

// RenameWindow renames window.
func (s Service) RenameWindow(ctx context.Context, session string, index int, name string) error {
	return renameWindowVia(ctx, s.run, session, index, name)
}

// RenamePane renames pane.
func (s Service) RenamePane(ctx context.Context, paneID, title string) error {
	_, err := s.run(ctx, "select-pane", "-t", paneID, "-T", title)
	return err
}

// KillSession handles kill session.
func (s Service) KillSession(ctx context.Context, session string) error {
	_, err := s.run(ctx, "kill-session", "-t", session)
	return err
}

// KillSessionByID kills one exact tmux runtime session.
func (s Service) KillSessionByID(ctx context.Context, sessionID string) error {
	return killSessionByIDVia(ctx, s.run, sessionID)
}

// ListWindows lists windows.
func (s Service) ListWindows(ctx context.Context, session string) ([]Window, error) {
	return listWindowsVia(ctx, s.run, session)
}

// ListPanes lists panes.
func (s Service) ListPanes(ctx context.Context, session string) ([]Pane, error) {
	return listPanesVia(ctx, s.run, session)
}

// ReorderWindows reorders windows.
func (s Service) ReorderWindows(ctx context.Context, session string, orderedWindowIDs []string) error {
	return reorderWindowsVia(ctx, s.run, session, orderedWindowIDs)
}

// SelectWindow selects window.
func (s Service) SelectWindow(ctx context.Context, session string, index int) error {
	return selectWindowVia(ctx, s.run, session, index)
}

// SelectPane selects pane.
func (s Service) SelectPane(ctx context.Context, paneID string) error {
	_, err := s.run(ctx, "select-pane", "-t", paneID)
	return err
}

// NewWindow creates window.
func (s Service) NewWindow(ctx context.Context, session string) (NewWindowResult, error) {
	return s.NewWindowWithOptions(ctx, session, "", "")
}

// NewWindowWithOptions creates window with options.
func (s Service) NewWindowWithOptions(ctx context.Context, session, name, cwd string) (NewWindowResult, error) {
	return newWindowWithOptionsVia(ctx, s.run, session, name, cwd)
}

// KillWindow handles kill window.
func (s Service) KillWindow(ctx context.Context, session string, index int) error {
	return killWindowVia(ctx, s.run, session, index)
}

// KillPane handles kill pane.
func (s Service) KillPane(ctx context.Context, paneID string) error {
	_, err := s.run(ctx, "kill-pane", "-t", paneID)
	return err
}

// SplitPane splits pane.
func (s Service) SplitPane(ctx context.Context, paneID, direction string) (string, error) {
	return splitPaneVia(ctx, s.run, paneID, direction)
}

// SessionExists handles session exists.
func (s Service) SessionExists(ctx context.Context, session string) (bool, error) {
	_, err := s.run(ctx, "has-session", "-t", session)
	if err != nil {
		if IsKind(err, ErrKindSessionNotFound) || IsKind(err, ErrKindServerNotRunning) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// SendKeys sends keys.
func (s Service) SendKeys(ctx context.Context, paneID, keys string, enter bool) error {
	return sendKeysVia(ctx, s.run, paneID, keys, enter)
}

// SendText sends text literally, preserving whitespace.
func (s Service) SendText(ctx context.Context, paneID, text string) error {
	return sendTextVia(ctx, s.run, paneID, text)
}

// SendKey sends one named tmux key such as Enter or C-c.
func (s Service) SendKey(ctx context.Context, paneID, key string) error {
	return sendKeyVia(ctx, s.run, paneID, key)
}

// CapturePaneScreen captures the visible pane as plain text.
func (s Service) CapturePaneScreen(ctx context.Context, paneID string) (string, error) {
	return capturePaneScreenVia(ctx, s.run, paneID)
}

// CapturePaneLines captures pane lines.
func (s Service) CapturePaneLines(ctx context.Context, target string, lines int) (string, error) {
	return capturePaneLinesVia(ctx, s.run, target, lines)
}

// SetSessionMouse sets session mouse.
func (s Service) SetSessionMouse(ctx context.Context, session string, enabled bool) error {
	return setSessionOptionVia(ctx, s.run, session, "mouse", enabled)
}

// SetSessionStatus sets session status.
func (s Service) SetSessionStatus(ctx context.Context, session string, enabled bool) error {
	return setSessionOptionVia(ctx, s.run, session, "status", enabled)
}

// EnsureWebMouseBindings ensures web mouse bindings.
func (s Service) EnsureWebMouseBindings(ctx context.Context) error {
	if s.User == "" {
		return EnsureWebMouseBindings(ctx)
	}
	// Best-effort for multi-user: apply global bindings via the user's server.
	_, _ = s.run(ctx, "set-option", "-s", "set-clipboard", "on")
	return nil
}
