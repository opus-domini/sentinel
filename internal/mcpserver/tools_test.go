package mcpserver

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/security"
	"github.com/opus-domini/sentinel/internal/tmux"
	"github.com/opus-domini/sentinel/internal/tmuxlifecycle"
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

func TestToolTimeoutHelpers(t *testing.T) {
	if got := boundedTimeout(0, 3*time.Second); got != 3*time.Second {
		t.Fatalf("boundedTimeout fallback = %s", got)
	}
	if got := boundedTimeout(250, time.Second); got != 250*time.Millisecond {
		t.Fatalf("boundedTimeout explicit = %s", got)
	}
	if got := boundedTimeout(int((maxToolWait+time.Second)/time.Millisecond), time.Second); got != maxToolWait {
		t.Fatalf("boundedTimeout maximum = %s", got)
	}

	nowish := deadlineOf(context.Background())
	if delta := time.Since(nowish); delta < 0 || delta > time.Second {
		t.Fatalf("deadlineOf(background) = %s", nowish)
	}
	want := time.Now().Add(time.Minute)
	ctx, cancel := context.WithDeadline(context.Background(), want)
	defer cancel()
	if got := deadlineOf(ctx); !got.Equal(want) {
		t.Fatalf("deadlineOf(context) = %s, want %s", got, want)
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

	_, created, err := toolset.createSession(context.Background(), nil, createSessionInput{
		Name: "agent", Cwd: "/srv/app", User: "deploy", Lifetime: lifetimePersistent,
	})
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}
	if deploy.createdName != "agent" || deploy.createdCWD != "/srv/app" || created.Session.Windows != 1 ||
		created.Lifecycle.Mode != lifetimePersistent {
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

func TestCreateSessionDefaultsToEphemeralAndRejectsInvalidLifetime(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	service := &fakeTmuxService{
		windows: []tmux.Window{{Session: "agent", ID: "@1", Index: 0}},
		panes:   []tmux.Pane{{Session: "agent", PaneID: "%1"}},
	}
	lifecycle := &fakeSessionLifecycle{
		createSnapshot: tmuxlifecycle.Snapshot{
			LeaseID:     "lease_001",
			SessionID:   "$12",
			SessionName: "agent",
			Source:      "mcp",
			State:       "active",
			ExpiresAt:   now.Add(tmuxlifecycle.DefaultIdleTimeout),
		},
	}
	toolset := &tools{
		guard:          security.New("token", nil, security.CookieSecureAuto),
		lifecycle:      lifecycle,
		serviceForUser: func(string) tmuxService { return service },
	}

	_, created, err := toolset.createSession(context.Background(), nil, createSessionInput{Name: "agent"})
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}
	if lifecycle.createdName != "agent" || service.createdName != "" {
		t.Fatalf("ephemeral create routing: lifecycle=%q direct=%q", lifecycle.createdName, service.createdName)
	}
	if created.Lifecycle.Mode != lifetimeEphemeral || created.Lifecycle.LeaseID != "lease_001" ||
		created.Lifecycle.IdleTimeoutSeconds != 7200 || created.Lifecycle.ExpiresAt != now.Add(2*time.Hour).Format(time.RFC3339) {
		t.Fatalf("ephemeral lifecycle = %#v", created.Lifecycle)
	}
	lifecycle.managedUses = true
	if _, _, err := toolset.listWindows(context.Background(), nil, sessionTargetInput{Session: "agent"}); err != nil {
		t.Fatalf("listWindows() lifecycle error = %v", err)
	}
	if _, _, err := toolset.listPanes(context.Background(), nil, sessionTargetInput{Session: "agent"}); err != nil {
		t.Fatalf("listPanes() lifecycle error = %v", err)
	}
	if len(lifecycle.finishResults) != 2 || !lifecycle.finishResults[0] || !lifecycle.finishResults[1] {
		t.Fatalf("list lifecycle finishes = %v", lifecycle.finishResults)
	}

	_, _, err = toolset.createSession(context.Background(), nil, createSessionInput{Name: "other", Lifetime: "forever"})
	if err == nil || !strings.Contains(err.Error(), "lifetime") {
		t.Fatalf("invalid lifetime error = %v", err)
	}
}

func TestListSessionsOverlaysLifecycleOnlyOnExactRuntimeID(t *testing.T) {
	service := &fakeTmuxService{sessions: []tmux.Session{
		{ID: "$7", Name: "managed"},
		{ID: "$8", Name: "replacement"},
		{ID: "$9", Name: "persistent"},
	}}
	lifecycle := &fakeSessionLifecycle{managedUses: true, snapshots: []tmuxlifecycle.Snapshot{
		{LeaseID: "lease_match", SessionID: "$7", SessionName: "managed", Source: "mcp", State: "active"},
		{LeaseID: "lease_stale", SessionID: "$77", SessionName: "replacement", Source: "mcp", State: "grace"},
	}}
	toolset := &tools{
		guard:          security.New("token", nil, security.CookieSecureAuto),
		lifecycle:      lifecycle,
		serviceForUser: func(string) tmuxService { return service },
	}

	_, output, err := toolset.listSessions(context.Background(), nil, emptyInput{})
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]sessionOutput, len(output.Sessions))
	for _, session := range output.Sessions {
		byName[session.Name] = session
	}
	if byName["managed"].Lifecycle == nil || byName["managed"].Lifecycle.LeaseID != "lease_match" {
		t.Fatalf("managed lifecycle = %#v", byName["managed"].Lifecycle)
	}
	if byName["replacement"].Lifecycle != nil || byName["persistent"].Lifecycle != nil {
		t.Fatalf("unmanaged lifecycle leak: replacement=%#v persistent=%#v", byName["replacement"].Lifecycle, byName["persistent"].Lifecycle)
	}
	if len(lifecycle.beginTargets) != 0 {
		t.Fatalf("tmux_list_sessions renewed lifecycle: %v", lifecycle.beginTargets)
	}
}

func TestKeepAndCloseSessionToolsRequireExactLeaseConfirmation(t *testing.T) {
	lifecycle := &fakeSessionLifecycle{
		keepSnapshot: tmuxlifecycle.Snapshot{LeaseID: "lease_1", SessionName: "agent"},
	}
	toolset := &tools{lifecycle: lifecycle}

	_, kept, err := toolset.keepSession(context.Background(), nil, lifecycleTransitionInput{
		LeaseID: "lease_1", ConfirmName: "agent",
	})
	if err != nil || kept.Session != "agent" || kept.Lifecycle.Mode != lifetimePersistent {
		t.Fatalf("keepSession() = %#v, %v", kept, err)
	}
	_, closed, err := toolset.closeSession(context.Background(), nil, lifecycleTransitionInput{
		LeaseID: "lease_1", ConfirmName: "agent",
	})
	if err != nil || !closed.Closed || lifecycle.closedLease != "lease_1" {
		t.Fatalf("closeSession() = %#v, %v", closed, err)
	}
	if _, _, err := toolset.closeSession(context.Background(), nil, lifecycleTransitionInput{LeaseID: "lease_1"}); err == nil {
		t.Fatal("closeSession() accepted missing confirmName")
	}

	lifecycle.keepErr = tmuxlifecycle.ErrIdentityMismatch
	if _, _, err := toolset.keepSession(context.Background(), nil, lifecycleTransitionInput{
		LeaseID: "lease_1", ConfirmName: "agent",
	}); err == nil {
		t.Fatal("keepSession() accepted lifecycle identity mismatch")
	}

	lifecycle.keepErr = tmuxlifecycle.ErrLeaseNotFound
	lifecycle.closeErr = tmuxlifecycle.ErrLeaseNotFound
	if _, _, err := toolset.keepSession(context.Background(), nil, lifecycleTransitionInput{
		LeaseID: "lease_persistent", ConfirmName: "persistent",
	}); err == nil {
		t.Fatal("keepSession() adopted a persistent session without a lease")
	}
	if _, _, err := toolset.closeSession(context.Background(), nil, lifecycleTransitionInput{
		LeaseID: "lease_persistent", ConfirmName: "persistent",
	}); err == nil {
		t.Fatal("closeSession() adopted a persistent session without a lease")
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
	lifecycle := &fakeSessionLifecycle{managedUses: true}
	toolset := &tools{
		guard:          security.New("token", nil, security.CookieSecureAuto),
		attachments:    manager,
		lifecycle:      lifecycle,
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
	_, timeoutRead, err := toolset.read(context.Background(), nil, readInput{
		AttachmentID: attached.AttachmentID,
		Cursor:       read.Cursor,
		PaneID:       "%1",
		TimeoutMS:    1,
	})
	if err != nil || !timeoutRead.TimedOut || timeoutRead.Closed {
		t.Fatalf("timeout read = %#v, error = %v", timeoutRead, err)
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
	if len(lifecycle.finishResults) != 4 {
		t.Fatalf("targeted lifecycle finishes = %v, want four operations", lifecycle.finishResults)
	}
	for _, success := range lifecycle.finishResults {
		if !success {
			t.Fatalf("targeted lifecycle finish results = %v", lifecycle.finishResults)
		}
	}
}

type fakeSessionLifecycle struct {
	createSnapshot tmuxlifecycle.Snapshot
	keepSnapshot   tmuxlifecycle.Snapshot
	snapshots      []tmuxlifecycle.Snapshot
	createdName    string
	createdCWD     string
	createdUser    string
	managedUses    bool
	beginTargets   []string
	finishResults  []bool
	keepErr        error
	closeErr       error
	closedLease    string
}

func (l *fakeSessionLifecycle) Create(
	ctx context.Context,
	user, name, cwd string,
	prepare func(context.Context, tmux.Session) error,
) (tmuxlifecycle.Snapshot, error) {
	l.createdUser = user
	l.createdName = name
	l.createdCWD = cwd
	snapshot := l.createSnapshot
	if snapshot.SessionName == "" {
		snapshot.SessionName = name
	}
	if err := prepare(ctx, tmux.Session{ID: snapshot.SessionID, Name: snapshot.SessionName}); err != nil {
		return tmuxlifecycle.Snapshot{}, err
	}
	return snapshot, nil
}

func (l *fakeSessionLifecycle) BeginUse(_ context.Context, user, session string) (tmuxlifecycle.Use, error) {
	l.beginTargets = append(l.beginTargets, user+"/"+session)
	if !l.managedUses {
		return tmuxlifecycle.Use{}, nil
	}
	return tmuxlifecycle.Use{LeaseID: "lease_use"}, nil
}

func (l *fakeSessionLifecycle) Finish(_ context.Context, _ tmuxlifecycle.Use, success bool) error {
	l.finishResults = append(l.finishResults, success)
	return nil
}

func (l *fakeSessionLifecycle) Keep(context.Context, string, string) (tmuxlifecycle.Snapshot, error) {
	return l.keepSnapshot, l.keepErr
}

func (l *fakeSessionLifecycle) Close(_ context.Context, leaseID, _ string) error {
	l.closedLease = leaseID
	return l.closeErr
}

func (l *fakeSessionLifecycle) Snapshot() []tmuxlifecycle.Snapshot {
	return append([]tmuxlifecycle.Snapshot(nil), l.snapshots...)
}

var _ sessionLifecycle = (*fakeSessionLifecycle)(nil)

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
