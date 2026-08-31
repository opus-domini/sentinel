// Package tmux wraps tmux command execution.
package tmux

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// runnerFunc is the function signature for executing tmux commands.
type runnerFunc func(ctx context.Context, args ...string) (string, error)

func listSessionsVia(ctx context.Context, runFn runnerFunc) ([]Session, error) {
	out, err := runFn(ctx, "list-sessions", "-F", listSessionsFormatWithActivity)
	if err != nil {
		if IsKind(err, ErrKindServerNotRunning) {
			return []Session{}, nil
		}
		if !shouldRetryListSessionsWithoutActivity(err) {
			return nil, err
		}
		out, err = runFn(ctx, "list-sessions", "-F", listSessionsFormatWithoutActivity)
		if err != nil {
			if IsKind(err, ErrKindServerNotRunning) {
				return []Session{}, nil
			}
			return nil, err
		}
	}
	return parseSessionListOutput(out), nil
}

func listActivePaneCommandsVia(ctx context.Context, runFn runnerFunc) (map[string]PaneSnapshot, error) {
	out, err := runFn(ctx, "list-panes", "-a", "-F", activePaneCommandsFormat)
	if err != nil {
		if IsKind(err, ErrKindServerNotRunning) {
			return map[string]PaneSnapshot{}, nil
		}
		return nil, err
	}
	return parseActivePaneCommandsOutput(out), nil
}

// parseActivePaneCommandsOutput parses list-panes output into a
// session -> PaneSnapshot map.
func parseActivePaneCommandsOutput(out string) map[string]PaneSnapshot {
	if strings.TrimSpace(out) == "" {
		return map[string]PaneSnapshot{}
	}
	result := make(map[string]PaneSnapshot)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		parts := strings.Split(line, fieldSep)
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
	return result
}

func capturePane(ctx context.Context, runFn runnerFunc, session string) (string, error) {
	out, err := runFn(ctx, "capture-pane", "-t", session+":", "-p", "-S", "-3")
	if err != nil {
		// A missing session legitimately yields an empty preview, but context
		// cancellation/timeout must propagate so callers can stop work.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
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

func renameWindowVia(ctx context.Context, runFn runnerFunc, session string, index int, name string) error {
	target := fmt.Sprintf("%s:%d", session, index)
	_, err := runFn(ctx, "rename-window", "-t", target, name)
	return err
}

func listWindowsVia(ctx context.Context, runFn runnerFunc, session string) ([]Window, error) {
	out, err := runFn(ctx, "list-windows", "-t", session, "-F", listWindowsFormat)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return []Window{}, nil
	}
	return parseWindowListOutput(out), nil
}

func listPanesVia(ctx context.Context, runFn runnerFunc, session string) ([]Pane, error) {
	out, err := runFn(ctx, "list-panes", "-a", "-F", listPanesFormat)
	if err != nil {
		return nil, err
	}
	return parsePaneListOutput(out, session), nil
}

func selectWindowVia(ctx context.Context, runFn runnerFunc, session string, index int) error {
	target := fmt.Sprintf("%s:%d", session, index)
	_, err := runFn(ctx, "select-window", "-t", target)
	return err
}

func newWindowWithOptionsVia(ctx context.Context, runFn runnerFunc, session, name, cwd string) (NewWindowResult, error) {
	target := fmt.Sprintf("%s:", session)
	if indexesOut, listErr := runFn(ctx, "list-windows", "-t", session, "-F", "#{window_index}"); listErr == nil {
		if nextIndex, ok := nextWindowIndexFromListOutput(indexesOut); ok {
			target = fmt.Sprintf("%s:%d", session, nextIndex)
		}
	}
	args := []string{cmdNewWindow, "-P", "-F", newWindowFormat, "-t", target}
	if strings.TrimSpace(name) != "" {
		args = append(args, "-n", strings.TrimSpace(name))
	}
	resolvedCWD := strings.TrimSpace(cwd)
	if resolvedCWD == "" {
		if pathOut, pathErr := runFn(ctx, "display-message", "-t", session, "-p", "#{session_path}"); pathErr == nil {
			resolvedCWD = strings.TrimSpace(pathOut)
		}
	}
	if resolvedCWD != "" {
		args = append(args, "-c", resolvedCWD)
	}
	out, err := runFn(ctx, args...)
	if err != nil {
		return NewWindowResult{}, err
	}
	return parseNewWindowOutput(out)
}

func killWindowVia(ctx context.Context, runFn runnerFunc, session string, index int) error {
	target := fmt.Sprintf("%s:%d", session, index)
	_, err := runFn(ctx, "kill-window", "-t", target)
	return err
}

func reorderWindowsVia(ctx context.Context, runFn runnerFunc, session string, orderedWindowIDs []string) error {
	session = strings.TrimSpace(session)
	if session == "" {
		return &Error{Kind: ErrKindInvalidIdentifier, Msg: errSessionRequired}
	}
	if len(orderedWindowIDs) == 0 {
		return &Error{Kind: ErrKindInvalidIdentifier, Msg: "tmux window order is required"}
	}

	normalized := make([]string, 0, len(orderedWindowIDs))
	seen := make(map[string]struct{}, len(orderedWindowIDs))
	for _, item := range orderedWindowIDs {
		windowID := strings.TrimSpace(item)
		if windowID == "" {
			return &Error{Kind: ErrKindInvalidIdentifier, Msg: "tmux window id is required"}
		}
		if _, exists := seen[windowID]; exists {
			return &Error{Kind: ErrKindInvalidIdentifier, Msg: "tmux window ids must be unique"}
		}
		seen[windowID] = struct{}{}
		normalized = append(normalized, windowID)
	}

	liveWindows, err := listWindowsVia(ctx, runFn, session)
	if err != nil {
		return err
	}
	if len(liveWindows) != len(normalized) {
		return &Error{Kind: ErrKindInvalidIdentifier, Msg: errWindowOrderMismatch}
	}

	current := make([]string, 0, len(liveWindows))
	positions := make(map[string]int, len(liveWindows))
	for index, window := range liveWindows {
		windowID := strings.TrimSpace(window.ID)
		if windowID == "" {
			return &Error{Kind: ErrKindInvalidIdentifier, Msg: "tmux live window id is required"}
		}
		current = append(current, windowID)
		positions[windowID] = index
	}
	for _, windowID := range normalized {
		if _, ok := positions[windowID]; !ok {
			return &Error{Kind: ErrKindInvalidIdentifier, Msg: errWindowOrderMismatch}
		}
	}

	for index, wantID := range normalized {
		currentID := current[index]
		if currentID == wantID {
			continue
		}
		swapIndex := positions[wantID]
		if _, err := runFn(ctx, "swap-window", "-d", "-s", currentID, "-t", wantID); err != nil {
			return err
		}
		current[index], current[swapIndex] = current[swapIndex], current[index]
		positions[currentID] = swapIndex
		positions[wantID] = index
	}
	return nil
}

func splitPaneVia(ctx context.Context, runFn runnerFunc, paneID, direction string) (string, error) {
	args := []string{cmdSplitWindow, "-t", paneID}
	switch direction {
	case dirVertical:
		args = append(args, "-h")
	case dirHorizontal:
		args = append(args, "-v")
	default:
		return "", &Error{Kind: ErrKindInvalidIdentifier, Msg: errInvalidSplitDir}
	}
	args = append(args, "-P", "-F", "#{pane_id}")
	out, err := runFn(ctx, args...)
	if err != nil {
		return "", err
	}
	return parseSplitPaneOutput(out)
}

func sendKeysVia(ctx context.Context, runFn runnerFunc, paneID, keys string, enter bool) error {
	keys = strings.TrimSpace(keys)
	if keys != "" {
		if _, err := runFn(ctx, "send-keys", "-t", paneID, "-l", keys); err != nil {
			return err
		}
	}
	if enter {
		if _, err := runFn(ctx, "send-keys", "-t", paneID, "C-m"); err != nil {
			return err
		}
	}
	return nil
}

func sendTextVia(ctx context.Context, runFn runnerFunc, paneID, text string) error {
	if strings.TrimSpace(paneID) == "" {
		return &Error{Kind: ErrKindInvalidIdentifier, Msg: errPaneIDRequired}
	}
	if text == "" {
		return nil
	}
	_, err := runFn(ctx, "send-keys", "-t", paneID, "-l", "--", text)
	return err
}

func sendKeyVia(ctx context.Context, runFn runnerFunc, paneID, key string) error {
	if strings.TrimSpace(paneID) == "" {
		return &Error{Kind: ErrKindInvalidIdentifier, Msg: errPaneIDRequired}
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return &Error{Kind: ErrKindInvalidIdentifier, Msg: "key is required"}
	}
	if len(key) > 64 || strings.ContainsAny(key, "\r\n\x00") {
		return &Error{Kind: ErrKindInvalidIdentifier, Msg: "invalid key"}
	}
	_, err := runFn(ctx, "send-keys", "-t", paneID, "--", key)
	return err
}

func capturePaneScreenVia(ctx context.Context, runFn runnerFunc, paneID string) (string, error) {
	if strings.TrimSpace(paneID) == "" {
		return "", &Error{Kind: ErrKindInvalidIdentifier, Msg: errPaneIDRequired}
	}
	return runFn(ctx, "capture-pane", "-p", "-t", paneID)
}

func capturePaneLinesVia(ctx context.Context, runFn runnerFunc, target string, lines int) (string, error) {
	if strings.TrimSpace(target) == "" {
		return "", &Error{Kind: ErrKindInvalidIdentifier, Msg: "target is required"}
	}
	if lines <= 0 {
		lines = 80
	}
	start := fmt.Sprintf("-%d", lines)
	out, err := runFn(ctx, "capture-pane", "-t", target, "-p", "-S", start)
	if err != nil {
		return "", err
	}
	return out, nil
}

func setSessionOptionVia(ctx context.Context, runFn runnerFunc, session, option string, enabled bool) error {
	value := tmuxOff
	if enabled {
		value = tmuxOn
	}
	_, err := runFn(ctx, "set-option", "-t", session, option, value)
	return err
}

// parseWindowListOutput parses list-windows output into []Window.
func parseWindowListOutput(out string) []Window {
	if strings.TrimSpace(out) == "" {
		return []Window{}
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	windows := make([]Window, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, fieldSep)
		if len(parts) < 6 {
			continue
		}
		windowID := strings.TrimSpace(parts[1])
		idx, _ := strconv.Atoi(parts[2])
		panes, _ := strconv.Atoi(parts[5])
		layout := ""
		if len(parts) > 6 {
			layout = parts[6]
		}
		windows = append(windows, Window{
			Session: parts[0],
			ID:      windowID,
			Index:   idx,
			Name:    parts[3],
			Active:  parts[4] == "1",
			Panes:   panes,
			Layout:  layout,
		})
	}
	return windows
}

// parsePaneListOutput parses list-panes output filtered by session.
func parsePaneListOutput(out string, session string) []Pane {
	if strings.TrimSpace(out) == "" {
		return []Pane{}
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	panes := make([]Pane, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, fieldSep)
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
	return panes
}
