package terminals

import "context"

// SystemService delegates to the package-level system terminal functions.
type SystemService struct{}

func (SystemService) ListSystem(ctx context.Context) ([]SystemTerminal, error) {
	return ListSystem(ctx)
}

func (SystemService) ListProcesses(ctx context.Context, tty string) ([]TerminalProcess, error) {
	return ListProcesses(ctx, tty)
}
