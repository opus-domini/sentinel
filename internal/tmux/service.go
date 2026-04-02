package tmux

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
)

// Service delegates to the package-level tmux functions. When User is
// non-empty, commands are wrapped with sudo -n -u <user>.
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
var SystemUsers []string //nolint:gochecknoglobals // set once at startup from main

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

// runAsUser executes a tmux command, optionally wrapping it with sudo -n -u
// when user is non-empty. For the default (no user) case the package-level
// run variable is used so that tests can inject fakes.
// The user is validated against the system user database before execution
// to prevent command injection even when the allowlist is empty.
func runAsUser(ctx context.Context, user string, args ...string) (string, error) {
	if user == "" {
		return run(ctx, args...)
	}
	if err := verifySystemUser(user); err != nil {
		return "", &Error{Kind: ErrKindCommandFailed, Msg: err.Error()}
	}
	sudoArgs := []string{"-n", "-u", user, "tmux"}
	sudoArgs = append(sudoArgs, args...)
	cmd := exec.CommandContext(ctx, "sudo", sudoArgs...) //nolint:gosec // user validated by verifySystemUser above
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", classifyError(err, stderr.String(), args)
	}
	return stdout.String(), nil
}

func (s Service) ListSessions(ctx context.Context) ([]Session, error) {
	if s.User == "" {
		return ListSessions(ctx)
	}
	out, err := s.run(ctx, "list-sessions", "-F", listSessionsFormatWithActivity)
	if err != nil {
		if IsKind(err, ErrKindServerNotRunning) {
			return []Session{}, nil
		}
		if !shouldRetryListSessionsWithoutActivity(err) {
			return nil, err
		}
		out, err = s.run(ctx, "list-sessions", "-F", listSessionsFormatWithoutActivity)
		if err != nil {
			if IsKind(err, ErrKindServerNotRunning) {
				return []Session{}, nil
			}
			return nil, err
		}
	}
	return parseSessionListOutput(out), nil
}

func (s Service) ListActivePaneCommands(ctx context.Context) (map[string]PaneSnapshot, error) {
	if s.User == "" {
		return ListActivePaneCommands(ctx)
	}
	out, err := s.run(ctx, "list-panes", "-a", "-F", "#{session_name}\t#{window_active}\t#{pane_active}\t#{pane_start_command}\t#{pane_current_command}")
	if err != nil {
		if IsKind(err, ErrKindServerNotRunning) {
			return map[string]PaneSnapshot{}, nil
		}
		return nil, err
	}
	return parseActivePaneCommandsOutput(out), nil
}

func (s Service) CapturePane(ctx context.Context, session string) (string, error) {
	if s.User == "" {
		return CapturePane(ctx, session)
	}
	return capturePane(ctx, s.run, session)
}

func (s Service) HasSession(ctx context.Context, session string) bool {
	if s.User == "" {
		return HasSession(ctx, session)
	}
	_, err := s.run(ctx, "has-session", "-t", session)
	return err == nil
}

func (s Service) CreateSession(ctx context.Context, name, cwd string) error {
	if s.User == "" {
		return CreateSession(ctx, name, cwd)
	}
	args := []string{"new-session", "-d", "-s", name}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	_, err := s.run(ctx, args...)
	return err
}

func (s Service) RenameSession(ctx context.Context, session, newName string) error {
	if s.User == "" {
		return RenameSession(ctx, session, newName)
	}
	_, err := s.run(ctx, "rename-session", "-t", session, newName)
	return err
}

func (s Service) RenameWindow(ctx context.Context, session string, index int, name string) error {
	if s.User == "" {
		return RenameWindow(ctx, session, index, name)
	}
	return renameWindowVia(ctx, s.run, session, index, name)
}

func (s Service) RenamePane(ctx context.Context, paneID, title string) error {
	if s.User == "" {
		return RenamePane(ctx, paneID, title)
	}
	_, err := s.run(ctx, "select-pane", "-t", paneID, "-T", title)
	return err
}

func (s Service) KillSession(ctx context.Context, session string) error {
	if s.User == "" {
		return KillSession(ctx, session)
	}
	_, err := s.run(ctx, "kill-session", "-t", session)
	return err
}

func (s Service) ListWindows(ctx context.Context, session string) ([]Window, error) {
	if s.User == "" {
		return ListWindows(ctx, session)
	}
	return listWindowsVia(ctx, s.run, session)
}

func (s Service) ListPanes(ctx context.Context, session string) ([]Pane, error) {
	if s.User == "" {
		return ListPanes(ctx, session)
	}
	return listPanesVia(ctx, s.run, session)
}

func (s Service) ReorderWindows(ctx context.Context, session string, orderedWindowIDs []string) error {
	if s.User == "" {
		return ReorderWindows(ctx, session, orderedWindowIDs)
	}
	return reorderWindowsVia(ctx, s.run, session, orderedWindowIDs)
}

func (s Service) SelectWindow(ctx context.Context, session string, index int) error {
	if s.User == "" {
		return SelectWindow(ctx, session, index)
	}
	return selectWindowVia(ctx, s.run, session, index)
}

func (s Service) SelectPane(ctx context.Context, paneID string) error {
	if s.User == "" {
		return SelectPane(ctx, paneID)
	}
	_, err := s.run(ctx, "select-pane", "-t", paneID)
	return err
}

func (s Service) NewWindow(ctx context.Context, session string) (NewWindowResult, error) {
	return s.NewWindowWithOptions(ctx, session, "", "")
}

func (s Service) NewWindowWithOptions(ctx context.Context, session, name, cwd string) (NewWindowResult, error) {
	if s.User == "" {
		return NewWindowWithOptions(ctx, session, name, cwd)
	}
	return newWindowWithOptionsVia(ctx, s.run, session, name, cwd)
}

func (s Service) NewWindowAt(ctx context.Context, session string, index int, name, cwd string) error {
	if s.User == "" {
		return NewWindowAt(ctx, session, index, name, cwd)
	}
	return newWindowAtVia(ctx, s.run, session, index, name, cwd)
}

func (s Service) KillWindow(ctx context.Context, session string, index int) error {
	if s.User == "" {
		return KillWindow(ctx, session, index)
	}
	return killWindowVia(ctx, s.run, session, index)
}

func (s Service) KillPane(ctx context.Context, paneID string) error {
	if s.User == "" {
		return KillPane(ctx, paneID)
	}
	_, err := s.run(ctx, "kill-pane", "-t", paneID)
	return err
}

func (s Service) SplitPane(ctx context.Context, paneID, direction string) (string, error) {
	if s.User == "" {
		return SplitPane(ctx, paneID, direction)
	}
	return splitPaneVia(ctx, s.run, paneID, direction)
}

func (s Service) SessionExists(ctx context.Context, session string) (bool, error) {
	if s.User == "" {
		return SessionExists(ctx, session)
	}
	_, err := s.run(ctx, "has-session", "-t", session)
	if err != nil {
		if IsKind(err, ErrKindSessionNotFound) || IsKind(err, ErrKindServerNotRunning) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s Service) SplitPaneIn(ctx context.Context, paneID, direction, cwd string) (string, error) {
	if s.User == "" {
		return SplitPaneIn(ctx, paneID, direction, cwd)
	}
	return splitPaneInVia(ctx, s.run, paneID, direction, cwd)
}

func (s Service) SelectLayout(ctx context.Context, session string, index int, layout string) error {
	if s.User == "" {
		return SelectLayout(ctx, session, index, layout)
	}
	return selectLayoutVia(ctx, s.run, session, index, layout)
}

func (s Service) SendKeys(ctx context.Context, paneID, keys string, enter bool) error {
	if s.User == "" {
		return SendKeys(ctx, paneID, keys, enter)
	}
	return sendKeysVia(ctx, s.run, paneID, keys, enter)
}

func (s Service) CapturePaneLines(ctx context.Context, target string, lines int) (string, error) {
	if s.User == "" {
		return CapturePaneLines(ctx, target, lines)
	}
	return capturePaneLinesVia(ctx, s.run, target, lines)
}

func (s Service) SetSessionMouse(ctx context.Context, session string, enabled bool) error {
	if s.User == "" {
		return SetSessionMouse(ctx, session, enabled)
	}
	return setSessionOptionVia(ctx, s.run, session, "mouse", enabled)
}

func (s Service) SetSessionStatus(ctx context.Context, session string, enabled bool) error {
	if s.User == "" {
		return SetSessionStatus(ctx, session, enabled)
	}
	return setSessionOptionVia(ctx, s.run, session, "status", enabled)
}

func (s Service) EnsureWebMouseBindings(ctx context.Context) error {
	if s.User == "" {
		return EnsureWebMouseBindings(ctx)
	}
	// Best-effort for multi-user: apply global bindings via the user's server.
	_, _ = s.run(ctx, "set-option", "-s", "set-clipboard", "on")
	return nil
}
