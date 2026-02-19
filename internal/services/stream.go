package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// ErrStreamingUnsupported is returned when log streaming is not available
// for the service manager in use (e.g. launchd).
var ErrStreamingUnsupported = errors.New("log streaming is not supported for this service manager")

// logStreamCloser wraps a journalctl process pipe so callers can read
// streaming log output and cleanly tear down the child process.
type logStreamCloser struct {
	pipe io.ReadCloser
	cmd  *exec.Cmd
}

func (lsc *logStreamCloser) Read(p []byte) (int, error) {
	return lsc.pipe.Read(p)
}

func (lsc *logStreamCloser) Close() error {
	_ = lsc.pipe.Close()
	if lsc.cmd.Process != nil {
		_ = lsc.cmd.Process.Kill()
	}
	_ = lsc.cmd.Wait()
	return nil
}

// StreamLogs starts a streaming log tail for the named managed service.
// Only systemd is supported; launchd returns ErrStreamingUnsupported.
func (m *Manager) StreamLogs(ctx context.Context, name string) (io.ReadCloser, error) {
	serviceName, ok := normalizeServiceName(name)
	if !ok {
		return nil, ErrServiceNotFound
	}

	services, err := m.ListServices(ctx)
	if err != nil {
		return nil, err
	}
	target, ok := findServiceStatus(services, serviceName)
	if !ok {
		return nil, ErrServiceNotFound
	}

	if target.Manager != managerSystemd {
		return nil, ErrStreamingUnsupported
	}
	return streamLogsSystemd(ctx, target)
}

// StreamLogsByUnit starts a streaming log tail for a service identified by
// unit/scope/manager directly, without requiring the service to be tracked.
// Only systemd is supported; launchd returns ErrStreamingUnsupported.
func (m *Manager) StreamLogsByUnit(ctx context.Context, unit, scope, manager string) (io.ReadCloser, error) {
	if manager != managerSystemd {
		return nil, ErrStreamingUnsupported
	}
	target := ServiceStatus{
		Unit:    unit,
		Scope:   scope,
		Manager: manager,
	}
	return streamLogsSystemd(ctx, target)
}

// streamLogsSystemd spawns journalctl --follow for the given service target
// and returns an io.ReadCloser that streams its stdout.
func streamLogsSystemd(ctx context.Context, target ServiceStatus) (io.ReadCloser, error) {
	args := make([]string, 0, 10)
	if strings.EqualFold(target.Scope, scopeUser) {
		args = append(args, "--user")
	}
	args = append(args,
		"-u", target.Unit,
		"--no-pager",
		"-n", "50",
		"--output=short-iso",
		"--follow",
	)

	cmd := exec.CommandContext(ctx, "journalctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("journalctl stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("journalctl start: %w", err)
	}
	return &logStreamCloser{pipe: stdout, cmd: cmd}, nil
}
