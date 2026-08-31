package tmux

import (
	"context"
	"testing"

	"github.com/opus-domini/sentinel/internal/userswitch"
)

// serviceMethodCall exercises one Service method, discarding typed results so
// every method fits a single table.
type serviceMethodCall struct {
	name string
	call func(context.Context, Service) error
}

// isolatedRunnerMethods are the methods that can start the tmux server. They
// must route through createSessionRun so the server lands outside Sentinel's
// cgroup and survives a sentinel.service restart.
var isolatedRunnerMethods = map[string]bool{
	"CreateSession":       true,
	"CreateSessionWithID": true,
}

func serviceMethodCalls() []serviceMethodCall {
	return []serviceMethodCall{
		{"ListSessions", func(ctx context.Context, s Service) error { _, e := s.ListSessions(ctx); return e }},
		{"GetSession", func(ctx context.Context, s Service) error { _, e := s.GetSession(ctx, "dev"); return e }},
		{"ListActivePaneCommands", func(ctx context.Context, s Service) error { _, e := s.ListActivePaneCommands(ctx); return e }},
		{"CapturePane", func(ctx context.Context, s Service) error { _, e := s.CapturePane(ctx, "dev"); return e }},
		{"CreateSession", func(ctx context.Context, s Service) error { return s.CreateSession(ctx, "dev", "") }},
		{"CreateSessionWithID", func(ctx context.Context, s Service) error { _, e := s.CreateSessionWithID(ctx, "dev", ""); return e }},
		{"RenameSession", func(ctx context.Context, s Service) error { return s.RenameSession(ctx, "dev", "prod") }},
		{"RenameWindow", func(ctx context.Context, s Service) error { return s.RenameWindow(ctx, "dev", 1, "logs") }},
		{"RenamePane", func(ctx context.Context, s Service) error { return s.RenamePane(ctx, "%1", "title") }},
		{"KillSession", func(ctx context.Context, s Service) error { return s.KillSession(ctx, "dev") }},
		{"KillSessionByID", func(ctx context.Context, s Service) error { return s.KillSessionByID(ctx, "$1") }},
		{"ListWindows", func(ctx context.Context, s Service) error { _, e := s.ListWindows(ctx, "dev"); return e }},
		{"ListPanes", func(ctx context.Context, s Service) error { _, e := s.ListPanes(ctx, "dev"); return e }},
		{"ReorderWindows", func(ctx context.Context, s Service) error { return s.ReorderWindows(ctx, "dev", []string{"@1"}) }},
		{"SelectWindow", func(ctx context.Context, s Service) error { return s.SelectWindow(ctx, "dev", 0) }},
		{"SelectPane", func(ctx context.Context, s Service) error { return s.SelectPane(ctx, "%1") }},
		{"NewWindow", func(ctx context.Context, s Service) error { _, e := s.NewWindow(ctx, "dev"); return e }},
		{"NewWindowWithOptions", func(ctx context.Context, s Service) error {
			_, e := s.NewWindowWithOptions(ctx, "dev", "w", "/tmp")
			return e
		}},
		{"NewWindowAt", func(ctx context.Context, s Service) error { return s.NewWindowAt(ctx, "dev", 2, "w", "/tmp") }},
		{"KillWindow", func(ctx context.Context, s Service) error { return s.KillWindow(ctx, "dev", 1) }},
		{"KillPane", func(ctx context.Context, s Service) error { return s.KillPane(ctx, "%1") }},
		{"SplitPane", func(ctx context.Context, s Service) error { _, e := s.SplitPane(ctx, "%1", dirVertical); return e }},
		{"SplitPaneIn", func(ctx context.Context, s Service) error {
			_, e := s.SplitPaneIn(ctx, "%1", dirVertical, "/tmp")
			return e
		}},
		{"SelectLayout", func(ctx context.Context, s Service) error { return s.SelectLayout(ctx, "dev", 0, "tiled") }},
		{"SendKeys", func(ctx context.Context, s Service) error { return s.SendKeys(ctx, "%1", "ls", true) }},
		{"CapturePaneLines", func(ctx context.Context, s Service) error { _, e := s.CapturePaneLines(ctx, "dev", 10); return e }},
		{"SetSessionMouse", func(ctx context.Context, s Service) error { return s.SetSessionMouse(ctx, "dev", true) }},
		{"SetSessionStatus", func(ctx context.Context, s Service) error { return s.SetSessionStatus(ctx, "dev", false) }},
		{"EnsureWebMouseBindings", func(ctx context.Context, s Service) error { return s.EnsureWebMouseBindings(ctx) }},
		{"HasSession", func(ctx context.Context, s Service) error {
			_ = s.HasSession(ctx, "dev")
			return nil
		}},
	}
}

// TestServiceMethodsDelegateToPackageLevel covers the s.User == "" branch of
// every Service method: it must route to the package-level tmux function,
// which uses the injectable run variable.
func TestServiceMethodsDelegateToPackageLevel(t *testing.T) {
	// Not parallel: mutates the package-level run variable.

	originalRun, originalCreate := run, createSessionRun
	t.Cleanup(func() { run, createSessionRun = originalRun, originalCreate })

	var calledRun, calledCreate bool
	run = func(_ context.Context, _ ...string) (string, error) {
		calledRun = true
		return "", nil
	}
	createSessionRun = func(_ context.Context, _ ...string) (string, error) {
		calledCreate = true
		return "", nil
	}

	for _, m := range serviceMethodCalls() {
		t.Run(m.name, func(t *testing.T) {
			calledRun, calledCreate = false, false
			// Result/error is tmux-output dependent; we only assert which
			// package-level runner the method routed through.
			_ = m.call(context.Background(), Service{})
			if isolatedRunnerMethods[m.name] {
				if !calledCreate || calledRun {
					t.Fatalf("%s: createSessionRun = %v, run = %v; want the isolated runner only", m.name, calledCreate, calledRun)
				}
				return
			}
			if !calledRun {
				t.Fatalf("%s did not delegate to the package-level run", m.name)
			}
		})
	}
}

// TestServiceMethodsWithUser covers the s.User != "" branch of every Service
// method: it must route through runAsUser and the user switch wrapper.
func TestServiceMethodsWithUser(t *testing.T) {
	// Not parallel: mutates execCommandContext, UserSwitchMethod and SystemUsers.

	originalUsers := SystemUsers
	t.Cleanup(func() { SystemUsers = originalUsers })
	SystemUsers = []string{"testuser"}

	originalMethod := UserSwitchMethod
	t.Cleanup(func() { UserSwitchMethod = originalMethod })
	UserSwitchMethod = userswitch.MethodSudo

	installExecCommandRecorder(t)

	svc := Service{User: "testuser"}
	for _, m := range serviceMethodCalls() {
		t.Run(m.name, func(_ *testing.T) {
			// The recorder echoes argv rather than tmux output, so parse
			// errors are expected; the test asserts the user-scoped path
			// runs without panicking.
			_ = m.call(context.Background(), svc)
		})
	}
}

// TestServiceMethodsRejectUnknownUser ensures an unverified user is refused
// before any command runs.
func TestServiceMethodsRejectUnknownUser(t *testing.T) {
	// Not parallel: mutates SystemUsers.

	originalUsers := SystemUsers
	t.Cleanup(func() { SystemUsers = originalUsers })
	SystemUsers = []string{"knownuser"}

	svc := Service{User: "intruder"}
	if _, err := svc.ListSessions(context.Background()); err == nil {
		t.Fatal("ListSessions with unknown user error = nil, want rejection")
	}
}
