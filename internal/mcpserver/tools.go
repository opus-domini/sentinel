package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/opus-domini/sentinel/internal/runbook"
	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/tmux"
	"github.com/opus-domini/sentinel/internal/validate"
)

const (
	maxToolWait   = 20 * time.Second
	inputTypeKey  = "key"
	inputTypeText = "text"
	waitModeNone  = "none"
	waitModeIdle  = "idle"
	waitModeText  = "text"
)

type tools struct {
	guard               *security.Guard
	attachments         *AttachmentManager
	serviceForUser      func(string) tmuxService
	sessionUser         func(string) string
	knownSessionUsers   func() []string
	registerSessionUser func(string, string)
	runbooks            *runbook.Manager
}

type tmuxService interface {
	ListSessions(context.Context) ([]tmux.Session, error)
	CreateSession(context.Context, string, string) error
	ListWindows(context.Context, string) ([]tmux.Window, error)
	ListPanes(context.Context, string) ([]tmux.Pane, error)
	HasSession(context.Context, string) bool
	SendText(context.Context, string, string) error
	SendKey(context.Context, string, string) error
	CapturePaneScreen(context.Context, string) (string, error)
}

type emptyInput struct{}

type sessionOutput struct {
	Name       string `json:"name"`
	User       string `json:"user,omitempty"`
	Windows    int    `json:"windows"`
	Attached   int    `json:"attached"`
	CreatedAt  string `json:"createdAt,omitempty"`
	ActivityAt string `json:"activityAt,omitempty"`
}

type listSessionsOutput struct {
	Sessions []sessionOutput `json:"sessions"`
}

type sessionTargetInput struct {
	Session string `json:"session" jsonschema:"tmux session name"`
	User    string `json:"user,omitempty" jsonschema:"optional OS user that owns the tmux session"`
}

type createSessionInput struct {
	Name string `json:"name" jsonschema:"new tmux session name"`
	Cwd  string `json:"cwd,omitempty" jsonschema:"optional absolute working directory"`
	User string `json:"user,omitempty" jsonschema:"optional allowed OS user"`
}

type createSessionOutput struct {
	Session sessionOutput `json:"session"`
	Windows []tmux.Window `json:"windows"`
	Panes   []tmux.Pane   `json:"panes"`
}

type listWindowsOutput struct {
	Session string        `json:"session"`
	User    string        `json:"user,omitempty"`
	Windows []tmux.Window `json:"windows"`
}

type listPanesOutput struct {
	Session string      `json:"session"`
	User    string      `json:"user,omitempty"`
	Panes   []tmux.Pane `json:"panes"`
}

type attachOutput struct {
	AttachmentID string        `json:"attachmentId"`
	Session      string        `json:"session"`
	User         string        `json:"user,omitempty"`
	PaneID       string        `json:"paneId"`
	Cursor       int64         `json:"cursor"`
	Screen       string        `json:"screen"`
	Windows      []tmux.Window `json:"windows"`
	Panes        []tmux.Pane   `json:"panes"`
}

type inputAction struct {
	Type  string `json:"type" jsonschema:"input type: text or key"`
	Value string `json:"value" jsonschema:"literal text or one tmux key name"`
}

type waitInput struct {
	Mode      string `json:"mode,omitempty" jsonschema:"wait mode: none, idle, or text"`
	QuietMS   int    `json:"quietMs,omitempty" jsonschema:"idle duration in milliseconds"`
	TimeoutMS int    `json:"timeoutMs,omitempty" jsonschema:"maximum wait in milliseconds, capped at 20000"`
	Pattern   string `json:"pattern,omitempty" jsonschema:"text or regular expression to wait for"`
	Regex     bool   `json:"regex,omitempty" jsonschema:"interpret pattern as a regular expression"`
}

type interactInput struct {
	AttachmentID string        `json:"attachmentId" jsonschema:"attachment returned by tmux_attach"`
	PaneID       string        `json:"paneId" jsonschema:"stable tmux pane ID such as %12"`
	Input        []inputAction `json:"input" jsonschema:"ordered text and key actions"`
	Wait         waitInput     `json:"wait,omitempty" jsonschema:"condition evaluated after sending input"`
}

type interactOutput struct {
	AttachmentID string         `json:"attachmentId"`
	Session      string         `json:"session"`
	PaneID       string         `json:"paneId"`
	Cursor       int64          `json:"cursor"`
	Events       []ControlEvent `json:"events"`
	Screen       string         `json:"screen"`
	Settled      bool           `json:"settled"`
	Matched      bool           `json:"matched"`
	TimedOut     bool           `json:"timedOut"`
	Dropped      bool           `json:"droppedEvents"`
}

type readInput struct {
	AttachmentID  string `json:"attachmentId" jsonschema:"attachment returned by tmux_attach"`
	Cursor        int64  `json:"cursor" jsonschema:"last event cursor seen by the caller"`
	PaneID        string `json:"paneId,omitempty" jsonschema:"optional pane filter; active pane is used when omitted"`
	TimeoutMS     int    `json:"timeoutMs,omitempty" jsonschema:"long-poll timeout in milliseconds, capped at 20000"`
	IncludeScreen bool   `json:"includeScreen,omitempty" jsonschema:"capture the current visible pane after reading"`
}

type readOutput struct {
	AttachmentID string         `json:"attachmentId"`
	Session      string         `json:"session"`
	PaneID       string         `json:"paneId"`
	Cursor       int64          `json:"cursor"`
	Events       []ControlEvent `json:"events"`
	Screen       string         `json:"screen,omitempty"`
	TimedOut     bool           `json:"timedOut"`
	Dropped      bool           `json:"droppedEvents"`
	Closed       bool           `json:"closed"`
}

type detachInput struct {
	AttachmentID string `json:"attachmentId" jsonschema:"attachment returned by tmux_attach"`
}

type detachOutput struct {
	AttachmentID string `json:"attachmentId"`
	Session      string `json:"session"`
	Detached     bool   `json:"detached"`
}

func (t *tools) register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tmux_list_sessions",
		Description: "List live tmux sessions visible to Sentinel, including their OS user when applicable.",
		Annotations: closedWorldAnnotations(true, false, true),
	}, t.listSessions)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tmux_create_session",
		Description: "Create one detached tmux session and return its initial windows and panes.",
		Annotations: closedWorldAnnotations(false, false, false),
	}, t.createSession)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tmux_list_windows",
		Description: "List windows in one tmux session with stable tmux window IDs.",
		Annotations: closedWorldAnnotations(true, false, true),
	}, t.listWindows)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tmux_list_panes",
		Description: "List panes in one tmux session with stable pane IDs, commands, paths and geometry.",
		Annotations: closedWorldAnnotations(true, false, true),
	}, t.listPanes)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tmux_attach",
		Description: "Attach a native tmux control-mode client and return the active pane screen plus an attachment ID.",
		Annotations: closedWorldAnnotations(false, false, false),
	}, t.attach)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tmux_interact",
		Description: "Send ordered literal text and named keys to an attached pane, wait for idle or text, and return events plus the current screen.",
		Annotations: closedWorldAnnotations(false, true, false),
	}, t.interact)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tmux_read",
		Description: "Long-poll incremental output and tmux events after a cursor, optionally returning the current pane screen.",
		Annotations: closedWorldAnnotations(true, false, false),
	}, t.read)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tmux_detach",
		Description: "Detach an MCP control client without stopping or killing the tmux session.",
		Annotations: closedWorldAnnotations(false, false, false),
	}, t.detach)
	if t.runbooks != nil {
		t.registerRunbookTools(server)
	}
}

func (t *tools) listSessions(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, listSessionsOutput, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	users := []string{""}
	if t.knownSessionUsers != nil {
		users = append(users, t.knownSessionUsers()...)
	}
	users = uniqueStrings(users)
	output := listSessionsOutput{Sessions: []sessionOutput{}}
	for _, user := range users {
		sessions, err := t.service(user).ListSessions(ctx)
		if err != nil {
			return nil, listSessionsOutput{}, toolError("list tmux sessions", err)
		}
		for _, session := range sessions {
			output.Sessions = append(output.Sessions, sessionResult(session, user))
			if user != "" && t.registerSessionUser != nil {
				t.registerSessionUser(session.Name, user)
			}
		}
	}
	slices.SortFunc(output.Sessions, func(a, b sessionOutput) int {
		if a.User != b.User {
			return strings.Compare(a.User, b.User)
		}
		return strings.Compare(a.Name, b.Name)
	})
	return nil, output, nil
}

func (t *tools) createSession(ctx context.Context, _ *mcp.CallToolRequest, input createSessionInput) (*mcp.CallToolResult, createSessionOutput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Cwd = strings.TrimSpace(input.Cwd)
	input.User = strings.TrimSpace(input.User)
	if !validate.SessionName(input.Name) {
		return nil, createSessionOutput{}, errors.New("name must match ^[A-Za-z0-9._][A-Za-z0-9._-]{0,63}$")
	}
	if input.Cwd != "" && !filepath.IsAbs(input.Cwd) {
		return nil, createSessionOutput{}, errors.New("cwd must be an absolute path")
	}
	if err := t.guard.ValidateTargetUser(input.User); err != nil {
		return nil, createSessionOutput{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	service := t.service(input.User)
	if err := service.CreateSession(ctx, input.Name, input.Cwd); err != nil {
		return nil, createSessionOutput{}, toolError("create tmux session", err)
	}
	if input.User != "" && t.registerSessionUser != nil {
		t.registerSessionUser(input.Name, input.User)
	}
	windows, err := service.ListWindows(ctx, input.Name)
	if err != nil {
		return nil, createSessionOutput{}, toolError("list created session windows", err)
	}
	panes, err := service.ListPanes(ctx, input.Name)
	if err != nil {
		return nil, createSessionOutput{}, toolError("list created session panes", err)
	}
	return nil, createSessionOutput{
		Session: sessionOutput{Name: input.Name, User: input.User, Windows: len(windows)},
		Windows: windows,
		Panes:   panes,
	}, nil
}

func (t *tools) listWindows(ctx context.Context, _ *mcp.CallToolRequest, input sessionTargetInput) (*mcp.CallToolResult, listWindowsOutput, error) {
	service, user, session, err := t.serviceForTarget(input)
	if err != nil {
		return nil, listWindowsOutput{}, err
	}
	windows, err := service.ListWindows(ctx, session)
	if err != nil {
		return nil, listWindowsOutput{}, toolError("list tmux windows", err)
	}
	return nil, listWindowsOutput{Session: session, User: user, Windows: windows}, nil
}

func (t *tools) listPanes(ctx context.Context, _ *mcp.CallToolRequest, input sessionTargetInput) (*mcp.CallToolResult, listPanesOutput, error) {
	service, user, session, err := t.serviceForTarget(input)
	if err != nil {
		return nil, listPanesOutput{}, err
	}
	panes, err := service.ListPanes(ctx, session)
	if err != nil {
		return nil, listPanesOutput{}, toolError("list tmux panes", err)
	}
	return nil, listPanesOutput{Session: session, User: user, Panes: panes}, nil
}

func (t *tools) attach(ctx context.Context, _ *mcp.CallToolRequest, input sessionTargetInput) (*mcp.CallToolResult, attachOutput, error) {
	service, user, session, err := t.serviceForTarget(input)
	if err != nil {
		return nil, attachOutput{}, err
	}
	if !service.HasSession(ctx, session) {
		return nil, attachOutput{}, errors.New("tmux session not found")
	}
	attachment, err := t.attachments.Open(user, session)
	if err != nil {
		return nil, attachOutput{}, toolError("attach tmux control client", err)
	}
	windows, panes, paneID, screen, err := inspectTarget(ctx, service, session, "")
	if err != nil {
		_ = t.attachments.Detach(attachment.ID)
		return nil, attachOutput{}, err
	}
	return nil, attachOutput{
		AttachmentID: attachment.ID,
		Session:      session,
		User:         user,
		PaneID:       paneID,
		Cursor:       attachment.Cursor,
		Screen:       screen,
		Windows:      windows,
		Panes:        panes,
	}, nil
}

func (t *tools) interact(ctx context.Context, _ *mcp.CallToolRequest, input interactInput) (*mcp.CallToolResult, interactOutput, error) {
	input.AttachmentID = strings.TrimSpace(input.AttachmentID)
	input.PaneID = strings.TrimSpace(input.PaneID)
	if input.AttachmentID == "" || input.PaneID == "" {
		return nil, interactOutput{}, errors.New("attachmentId and paneId are required")
	}
	if len(input.Input) == 0 {
		return nil, interactOutput{}, errors.New("input must contain at least one text or key action")
	}
	wait, err := normalizeWait(input.Wait)
	if err != nil {
		return nil, interactOutput{}, err
	}
	if err := validateInputActions(input.Input); err != nil {
		return nil, interactOutput{}, err
	}
	attachment, err := t.attachments.Lookup(input.AttachmentID)
	if err != nil {
		return nil, interactOutput{}, err
	}
	service := t.service(attachment.User)
	if err := ensurePane(ctx, service, attachment.Session, input.PaneID); err != nil {
		return nil, interactOutput{}, err
	}
	unlock, err := t.attachments.LockPane(input.AttachmentID, input.PaneID)
	if err != nil {
		return nil, interactOutput{}, err
	}
	defer unlock()

	startCursor := attachment.Cursor
	for _, action := range input.Input {
		switch strings.ToLower(strings.TrimSpace(action.Type)) {
		case inputTypeText:
			if err := service.SendText(ctx, input.PaneID, action.Value); err != nil {
				return nil, interactOutput{}, toolError("send text to tmux pane", err)
			}
		case inputTypeKey:
			if err := service.SendKey(ctx, input.PaneID, action.Value); err != nil {
				return nil, interactOutput{}, toolError("send key to tmux pane", err)
			}
		default:
			return nil, interactOutput{}, fmt.Errorf("unsupported input type %q", action.Type)
		}
	}

	batch, settled, matched, timedOut, err := t.waitAfterInput(ctx, service, input.AttachmentID, input.PaneID, startCursor, wait)
	if err != nil {
		return nil, interactOutput{}, err
	}
	screen, err := service.CapturePaneScreen(ctx, input.PaneID)
	if err != nil {
		return nil, interactOutput{}, toolError("capture tmux pane", err)
	}
	return nil, interactOutput{
		AttachmentID: input.AttachmentID,
		Session:      attachment.Session,
		PaneID:       input.PaneID,
		Cursor:       batch.Cursor,
		Events:       nonNilEvents(batch.Events),
		Screen:       screen,
		Settled:      settled,
		Matched:      matched,
		TimedOut:     timedOut,
		Dropped:      batch.Dropped,
	}, nil
}

func (t *tools) read(ctx context.Context, _ *mcp.CallToolRequest, input readInput) (*mcp.CallToolResult, readOutput, error) {
	input.AttachmentID = strings.TrimSpace(input.AttachmentID)
	if input.AttachmentID == "" {
		return nil, readOutput{}, errors.New("attachmentId is required")
	}
	attachment, err := t.attachments.Lookup(input.AttachmentID)
	if err != nil {
		return nil, readOutput{}, err
	}
	service := t.service(attachment.User)
	paneID := strings.TrimSpace(input.PaneID)
	if paneID == "" {
		_, _, paneID, _, err = inspectTarget(ctx, service, attachment.Session, "")
		if err != nil {
			return nil, readOutput{}, err
		}
	} else if err := ensurePane(ctx, service, attachment.Session, paneID); err != nil {
		return nil, readOutput{}, err
	}
	timeout := boundedTimeout(input.TimeoutMS, 0)
	batch, err := t.attachments.Read(ctx, input.AttachmentID, input.Cursor, paneID, timeout)
	if err != nil {
		return nil, readOutput{}, err
	}
	output := readOutput{
		AttachmentID: input.AttachmentID,
		Session:      attachment.Session,
		PaneID:       paneID,
		Cursor:       batch.Cursor,
		Events:       nonNilEvents(batch.Events),
		TimedOut:     batch.TimedOut,
		Dropped:      batch.Dropped,
		Closed:       batch.Closed,
	}
	if input.IncludeScreen {
		output.Screen, err = service.CapturePaneScreen(ctx, paneID)
		if err != nil {
			return nil, readOutput{}, toolError("capture tmux pane", err)
		}
	}
	return nil, output, nil
}

func (t *tools) detach(_ context.Context, _ *mcp.CallToolRequest, input detachInput) (*mcp.CallToolResult, detachOutput, error) {
	attachment, err := t.attachments.Lookup(input.AttachmentID)
	if err != nil {
		return nil, detachOutput{}, err
	}
	if err := t.attachments.Detach(input.AttachmentID); err != nil {
		return nil, detachOutput{}, err
	}
	return nil, detachOutput{AttachmentID: attachment.ID, Session: attachment.Session, Detached: true}, nil
}

func (t *tools) serviceForTarget(input sessionTargetInput) (tmuxService, string, string, error) {
	session := strings.TrimSpace(input.Session)
	if !validate.SessionName(session) {
		return nil, "", "", errors.New("invalid tmux session name")
	}
	user := strings.TrimSpace(input.User)
	if user == "" && t.sessionUser != nil {
		user = strings.TrimSpace(t.sessionUser(session))
	}
	if err := t.guard.ValidateTargetUser(user); err != nil {
		return nil, "", "", err
	}
	return t.service(user), user, session, nil
}

func (t *tools) service(user string) tmuxService {
	if t.serviceForUser != nil {
		return t.serviceForUser(user)
	}
	return tmux.Service{User: user}
}

func (t *tools) waitAfterInput(ctx context.Context, service tmuxService, attachmentID, paneID string, cursor int64, wait waitInput) (EventBatch, bool, bool, bool, error) {
	if wait.Mode == waitModeNone {
		batch, err := t.attachments.Read(ctx, attachmentID, cursor, paneID, 0)
		return batch, false, false, false, err
	}
	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(wait.TimeoutMS)*time.Millisecond)
	defer cancel()
	batch := EventBatch{Events: []ControlEvent{}, Cursor: cursor}
	if wait.Mode == waitModeText {
		matcher, err := textMatcher(wait.Pattern, wait.Regex)
		if err != nil {
			return batch, false, false, false, err
		}
		for {
			screen, err := service.CapturePaneScreen(waitCtx, paneID)
			if err != nil {
				return batch, false, false, false, toolError("capture tmux pane while waiting", err)
			}
			if matcher(screen) {
				next, err := t.attachments.Read(waitCtx, attachmentID, batch.Cursor, paneID, 0)
				if err != nil {
					return batch, false, false, false, err
				}
				mergeBatch(&batch, next)
				return batch, false, true, false, nil
			}
			next, err := t.attachments.Read(waitCtx, attachmentID, batch.Cursor, paneID, time.Until(deadlineOf(waitCtx)))
			if err != nil {
				return batch, false, false, false, err
			}
			mergeBatch(&batch, next)
			if waitCtx.Err() != nil {
				return batch, false, false, true, nil
			}
			if next.TimedOut {
				return batch, false, false, true, nil
			}
		}
	}

	quiet := time.Duration(wait.QuietMS) * time.Millisecond
	for {
		next, err := t.attachments.Read(waitCtx, attachmentID, batch.Cursor, paneID, quiet)
		if err != nil {
			return batch, false, false, false, err
		}
		mergeBatch(&batch, next)
		if waitCtx.Err() != nil {
			return batch, false, false, true, nil
		}
		if next.TimedOut {
			return batch, true, false, false, nil
		}
	}
}

func normalizeWait(wait waitInput) (waitInput, error) {
	wait.Mode = strings.ToLower(strings.TrimSpace(wait.Mode))
	if wait.Mode == "" {
		wait.Mode = waitModeIdle
	}
	if wait.Mode != waitModeNone && wait.Mode != waitModeIdle && wait.Mode != waitModeText {
		return waitInput{}, fmt.Errorf("unsupported wait mode %q", wait.Mode)
	}
	if wait.QuietMS <= 0 {
		wait.QuietMS = 400
	}
	if wait.QuietMS > int(maxToolWait/time.Millisecond) {
		wait.QuietMS = int(maxToolWait / time.Millisecond)
	}
	if wait.TimeoutMS <= 0 {
		wait.TimeoutMS = 5000
	}
	if wait.TimeoutMS > int(maxToolWait/time.Millisecond) {
		wait.TimeoutMS = int(maxToolWait / time.Millisecond)
	}
	if wait.Mode == waitModeText {
		if _, err := textMatcher(wait.Pattern, wait.Regex); err != nil {
			return waitInput{}, err
		}
	}
	return wait, nil
}

func validateInputActions(actions []inputAction) error {
	for index, action := range actions {
		switch strings.ToLower(strings.TrimSpace(action.Type)) {
		case inputTypeText:
		case inputTypeKey:
			key := strings.TrimSpace(action.Value)
			if key == "" || len(key) > 64 || strings.ContainsAny(key, "\r\n\x00") {
				return fmt.Errorf("input[%d] has an invalid key", index)
			}
		default:
			return fmt.Errorf("input[%d] has unsupported type %q", index, action.Type)
		}
	}
	return nil
}

func boundedTimeout(milliseconds int, fallback time.Duration) time.Duration {
	if milliseconds <= 0 {
		return fallback
	}
	timeout := time.Duration(milliseconds) * time.Millisecond
	if timeout > maxToolWait {
		return maxToolWait
	}
	return timeout
}

func deadlineOf(ctx context.Context) time.Time {
	deadline, ok := ctx.Deadline()
	if !ok {
		return time.Now()
	}
	return deadline
}

func textMatcher(pattern string, useRegex bool) (func(string) bool, error) {
	if pattern == "" {
		return nil, errors.New("wait.pattern is required for text mode")
	}
	if !useRegex {
		return func(value string) bool { return strings.Contains(value, pattern) }, nil
	}
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid wait pattern: %w", err)
	}
	return expression.MatchString, nil
}

func inspectTarget(ctx context.Context, service tmuxService, session, requestedPane string) ([]tmux.Window, []tmux.Pane, string, string, error) {
	windows, err := service.ListWindows(ctx, session)
	if err != nil {
		return nil, nil, "", "", toolError("list tmux windows", err)
	}
	panes, err := service.ListPanes(ctx, session)
	if err != nil {
		return nil, nil, "", "", toolError("list tmux panes", err)
	}
	paneID := strings.TrimSpace(requestedPane)
	if paneID == "" {
		paneID = activePaneID(windows, panes)
	}
	if paneID == "" {
		return nil, nil, "", "", errors.New("tmux session has no panes")
	}
	screen, err := service.CapturePaneScreen(ctx, paneID)
	if err != nil {
		return nil, nil, "", "", toolError("capture tmux pane", err)
	}
	return windows, panes, paneID, screen, nil
}

func activePaneID(windows []tmux.Window, panes []tmux.Pane) string {
	activeWindow := -1
	for _, window := range windows {
		if window.Active {
			activeWindow = window.Index
			break
		}
	}
	for _, pane := range panes {
		if pane.WindowIndex == activeWindow && pane.Active {
			return pane.PaneID
		}
	}
	if len(panes) > 0 {
		return panes[0].PaneID
	}
	return ""
}

func ensurePane(ctx context.Context, service tmuxService, session, paneID string) error {
	panes, err := service.ListPanes(ctx, session)
	if err != nil {
		return toolError("list tmux panes", err)
	}
	for _, pane := range panes {
		if pane.PaneID == paneID {
			return nil
		}
	}
	return errors.New("pane does not belong to the attached tmux session")
}

func sessionResult(session tmux.Session, user string) sessionOutput {
	return sessionOutput{
		Name:       session.Name,
		User:       user,
		Windows:    session.Windows,
		Attached:   session.Attached,
		CreatedAt:  formatTime(session.CreatedAt),
		ActivityAt: formatTime(session.ActivityAt),
	}
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func nonNilEvents(events []ControlEvent) []ControlEvent {
	if events == nil {
		return []ControlEvent{}
	}
	return events
}

func mergeBatch(target *EventBatch, next EventBatch) {
	target.Events = append(target.Events, next.Events...)
	target.Cursor = next.Cursor
	target.Dropped = target.Dropped || next.Dropped
	target.Closed = target.Closed || next.Closed
}
