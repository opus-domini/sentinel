package mcpserver

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/tmux"
)

func TestNormalizeWaitDefaultsAndBounds(t *testing.T) {
	wait, err := normalizeWait(waitInput{QuietMS: int((maxToolWait + time.Second) / time.Millisecond), TimeoutMS: int((maxToolWait + time.Second) / time.Millisecond)})
	if err != nil {
		t.Fatalf("normalizeWait() error = %v", err)
	}
	if wait.Mode != "idle" || wait.QuietMS != 20000 || wait.TimeoutMS != 20000 {
		t.Fatalf("normalizeWait() = %#v", wait)
	}
}

func TestNormalizeWaitRejectsInvalidModeBeforeInteraction(t *testing.T) {
	_, err := normalizeWait(waitInput{Mode: "command-complete"})
	if err == nil || !strings.Contains(err.Error(), "unsupported wait mode") {
		t.Fatalf("normalizeWait() error = %v", err)
	}
}

func TestNormalizeWaitValidatesTextPattern(t *testing.T) {
	_, err := normalizeWait(waitInput{Mode: "text", Pattern: "[", Regex: true})
	if err == nil || !strings.Contains(err.Error(), "invalid wait pattern") {
		t.Fatalf("normalizeWait() error = %v", err)
	}
}

func TestValidateInputActionsRejectsWholeInvalidSequence(t *testing.T) {
	err := validateInputActions([]inputAction{
		{Type: "text", Value: "echo safe"},
		{Type: "key", Value: "Enter\nC-c"},
	})
	if err == nil || !strings.Contains(err.Error(), "input[1]") {
		t.Fatalf("validateInputActions() error = %v", err)
	}
}

func TestTmuxDiscoveryAndCreationTools(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	local := &fakeTmuxService{sessions: []tmux.Session{{Name: "local", Windows: 1, CreatedAt: now}}}
	deploy := &fakeTmuxService{
		sessions: []tmux.Session{{Name: "remote", Windows: 2, Attached: 1}},
		windows:  []tmux.Window{{Session: "remote", ID: "@2", Index: 0, Active: true}},
		panes:    []tmux.Pane{{Session: "remote", WindowIndex: 0, PaneID: "%2", Active: true}},
	}
	registered := ""
	toolset := &tools{
		guard: security.NewWithMultiUser("token", nil, security.CookieSecureAuto, security.MultiUserConfig{
			AllowedUsers: []string{"deploy"},
			SystemUsers:  []string{"deploy"},
		}),
		serviceForUser: func(user string) tmuxService {
			if user == "deploy" {
				return deploy
			}
			return local
		},
		knownSessionUsers: func() []string { return []string{"deploy", "deploy"} },
		sessionUser:       func(session string) string { return map[string]string{"remote": "deploy"}[session] },
		registerSessionUser: func(session, user string) {
			registered = session + ":" + user
		},
	}

	_, listed, err := toolset.listSessions(context.Background(), nil, emptyInput{})
	if err != nil {
		t.Fatalf("listSessions() error = %v", err)
	}
	if len(listed.Sessions) != 2 || listed.Sessions[0].Name != "local" || listed.Sessions[1].User != "deploy" {
		t.Fatalf("listSessions() = %#v", listed.Sessions)
	}
	if listed.Sessions[0].CreatedAt != now.Format(time.RFC3339) || registered != "remote:deploy" {
		t.Fatalf("createdAt = %q, registered = %q", listed.Sessions[0].CreatedAt, registered)
	}

	_, created, err := toolset.createSession(context.Background(), nil, createSessionInput{Name: "agent", Cwd: "/srv/app", User: "deploy"})
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}
	if deploy.createdName != "agent" || deploy.createdCWD != "/srv/app" || created.Session.Windows != 1 {
		t.Fatalf("createSession() output = %#v, service = %#v", created, deploy)
	}

	_, windows, err := toolset.listWindows(context.Background(), nil, sessionTargetInput{Session: "remote"})
	if err != nil || windows.User != "deploy" || len(windows.Windows) != 1 {
		t.Fatalf("listWindows() = %#v, error = %v", windows, err)
	}
	_, panes, err := toolset.listPanes(context.Background(), nil, sessionTargetInput{Session: "remote"})
	if err != nil || panes.User != "deploy" || len(panes.Panes) != 1 {
		t.Fatalf("listPanes() = %#v, error = %v", panes, err)
	}
}

func TestTmuxAttachmentInteractionLifecycle(t *testing.T) {
	service := &fakeTmuxService{
		hasSession: true,
		windows:    []tmux.Window{{Session: "dev", ID: "@1", Index: 0, Active: true}},
		panes:      []tmux.Pane{{Session: "dev", WindowIndex: 0, PaneID: "%1", Active: true}},
		screen:     "prompt$ ",
	}
	stream := newTestControlStream()
	stream.key = "\x00dev"
	stream.session = "dev"
	stream.done = make(chan struct{})
	close(stream.done)
	stream.cancel = func() {}
	stream.stdin = nopWriteCloser{}
	manager := &AttachmentManager{
		attachments: make(map[string]*attachmentLease),
		streams:     map[string]*controlStream{stream.key: stream},
		ttl:         time.Hour,
	}
	toolset := &tools{
		guard:          security.New("token", nil, security.CookieSecureAuto),
		attachments:    manager,
		serviceForUser: func(string) tmuxService { return service },
	}

	_, attached, err := toolset.attach(context.Background(), nil, sessionTargetInput{Session: "dev"})
	if err != nil {
		t.Fatalf("attach() error = %v", err)
	}
	if attached.AttachmentID == "" || attached.PaneID != "%1" || attached.Screen != "prompt$ " {
		t.Fatalf("attach() = %#v", attached)
	}

	_, interacted, err := toolset.interact(context.Background(), nil, interactInput{
		AttachmentID: attached.AttachmentID,
		PaneID:       "%1",
		Input: []inputAction{
			{Type: inputTypeText, Value: "echo ready"},
			{Type: inputTypeKey, Value: "Enter"},
		},
		Wait: waitInput{Mode: waitModeNone},
	})
	if err != nil {
		t.Fatalf("interact() error = %v", err)
	}
	if interacted.Settled || service.sentText != "echo ready" || service.sentKey != "Enter" {
		t.Fatalf("interact() = %#v, service = %#v", interacted, service)
	}

	stream.mu.Lock()
	stream.appendLocked(ControlEvent{Type: "output", PaneID: "%1", Data: "ready\r\n"})
	stream.mu.Unlock()
	_, read, err := toolset.read(context.Background(), nil, readInput{
		AttachmentID:  attached.AttachmentID,
		Cursor:        attached.Cursor,
		PaneID:        "%1",
		IncludeScreen: true,
	})
	if err != nil {
		t.Fatalf("read() error = %v", err)
	}
	if len(read.Events) != 1 || read.Events[0].Data != "ready\r\n" || read.Screen != "prompt$ " {
		t.Fatalf("read() = %#v", read)
	}

	wait, err := normalizeWait(waitInput{Mode: waitModeIdle, QuietMS: 1, TimeoutMS: 20})
	if err != nil {
		t.Fatal(err)
	}
	_, settled, _, timedOut, err := toolset.waitAfterInput(context.Background(), service, attached.AttachmentID, "%1", read.Cursor, wait)
	if err != nil || !settled || timedOut {
		t.Fatalf("idle wait = settled:%t timedOut:%t error:%v", settled, timedOut, err)
	}

	service.screen = "READY"
	wait, err = normalizeWait(waitInput{Mode: waitModeText, Pattern: "READY", TimeoutMS: 20})
	if err != nil {
		t.Fatal(err)
	}
	_, _, matched, timedOut, err := toolset.waitAfterInput(context.Background(), service, attached.AttachmentID, "%1", read.Cursor, wait)
	if err != nil || !matched || timedOut {
		t.Fatalf("text wait = matched:%t timedOut:%t error:%v", matched, timedOut, err)
	}

	_, detached, err := toolset.detach(context.Background(), nil, detachInput{AttachmentID: attached.AttachmentID})
	if err != nil || !detached.Detached || detached.Session != "dev" {
		t.Fatalf("detach() = %#v, error = %v", detached, err)
	}
}

type fakeTmuxService struct {
	sessions    []tmux.Session
	windows     []tmux.Window
	panes       []tmux.Pane
	hasSession  bool
	screen      string
	createdName string
	createdCWD  string
	sentText    string
	sentKey     string
}

func (s *fakeTmuxService) ListSessions(context.Context) ([]tmux.Session, error) {
	return s.sessions, nil
}

func (s *fakeTmuxService) CreateSession(_ context.Context, name, cwd string) error {
	s.createdName = name
	s.createdCWD = cwd
	return nil
}

func (s *fakeTmuxService) ListWindows(context.Context, string) ([]tmux.Window, error) {
	return s.windows, nil
}

func (s *fakeTmuxService) ListPanes(context.Context, string) ([]tmux.Pane, error) {
	return s.panes, nil
}

func (s *fakeTmuxService) HasSession(context.Context, string) bool { return s.hasSession }

func (s *fakeTmuxService) SendText(_ context.Context, _ string, text string) error {
	s.sentText = text
	return nil
}

func (s *fakeTmuxService) SendKey(_ context.Context, _ string, key string) error {
	s.sentKey = key
	return nil
}

func (s *fakeTmuxService) CapturePaneScreen(context.Context, string) (string, error) {
	return s.screen, nil
}

type nopWriteCloser struct{}

func (nopWriteCloser) Write(value []byte) (int, error) { return len(value), nil }

func (nopWriteCloser) Close() error { return nil }

var _ io.WriteCloser = nopWriteCloser{}
