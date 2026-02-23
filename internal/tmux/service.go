package tmux

import "context"

// Service delegates to the package-level tmux functions.
type Service struct{}

func (Service) ListSessions(ctx context.Context) ([]Session, error) {
	return ListSessions(ctx)
}

func (Service) ListActivePaneCommands(ctx context.Context) (map[string]PaneSnapshot, error) {
	return ListActivePaneCommands(ctx)
}

func (Service) CapturePane(ctx context.Context, session string) (string, error) {
	return CapturePane(ctx, session)
}

func (Service) CreateSession(ctx context.Context, name, cwd string) error {
	return CreateSession(ctx, name, cwd)
}

func (Service) RenameSession(ctx context.Context, session, newName string) error {
	return RenameSession(ctx, session, newName)
}

func (Service) RenameWindow(ctx context.Context, session string, index int, name string) error {
	return RenameWindow(ctx, session, index, name)
}

func (Service) RenamePane(ctx context.Context, paneID, title string) error {
	return RenamePane(ctx, paneID, title)
}

func (Service) KillSession(ctx context.Context, session string) error {
	return KillSession(ctx, session)
}

func (Service) ListWindows(ctx context.Context, session string) ([]Window, error) {
	return ListWindows(ctx, session)
}

func (Service) ListPanes(ctx context.Context, session string) ([]Pane, error) {
	return ListPanes(ctx, session)
}

func (Service) SelectWindow(ctx context.Context, session string, index int) error {
	return SelectWindow(ctx, session, index)
}

func (Service) SelectPane(ctx context.Context, paneID string) error {
	return SelectPane(ctx, paneID)
}

func (Service) NewWindow(ctx context.Context, session string) (NewWindowResult, error) {
	return NewWindow(ctx, session)
}

func (Service) NewWindowAt(ctx context.Context, session string, index int, name, cwd string) error {
	return NewWindowAt(ctx, session, index, name, cwd)
}

func (Service) KillWindow(ctx context.Context, session string, index int) error {
	return KillWindow(ctx, session, index)
}

func (Service) KillPane(ctx context.Context, paneID string) error {
	return KillPane(ctx, paneID)
}

func (Service) SplitPane(ctx context.Context, paneID, direction string) (string, error) {
	return SplitPane(ctx, paneID, direction)
}

func (Service) SessionExists(ctx context.Context, session string) (bool, error) {
	return SessionExists(ctx, session)
}

func (Service) SplitPaneIn(ctx context.Context, paneID, direction, cwd string) (string, error) {
	return SplitPaneIn(ctx, paneID, direction, cwd)
}

func (Service) SelectLayout(ctx context.Context, session string, index int, layout string) error {
	return SelectLayout(ctx, session, index, layout)
}

func (Service) SendKeys(ctx context.Context, paneID, keys string, enter bool) error {
	return SendKeys(ctx, paneID, keys, enter)
}

func (Service) CapturePaneLines(ctx context.Context, target string, lines int) (string, error) {
	return CapturePaneLines(ctx, target, lines)
}
