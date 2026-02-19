package services

import (
	"context"
	"fmt"
	"strings"
)

const (
	defaultLogLines = 100
	maxLogLines     = 1000
)

// ServiceLogs retrieves recent log output for a managed service.
func (m *Manager) Logs(ctx context.Context, name string, lines int) (string, error) {
	serviceName, ok := normalizeServiceName(name)
	if !ok {
		return "", ErrServiceNotFound
	}
	if lines <= 0 {
		lines = defaultLogLines
	}
	if lines > maxLogLines {
		lines = maxLogLines
	}

	services, err := m.ListServices(ctx)
	if err != nil {
		return "", err
	}
	target, ok := findServiceStatus(services, serviceName)
	if !ok {
		return "", ErrServiceNotFound
	}

	switch target.Manager {
	case managerSystemd:
		return m.logsSystemd(ctx, target, lines)
	case managerLaunchd:
		return m.logsLaunchd(ctx, target, lines)
	default:
		return "", fmt.Errorf("unsupported service manager: %s", target.Manager)
	}
}

func (m *Manager) logsSystemd(ctx context.Context, target ServiceStatus, lines int) (string, error) {
	args := make([]string, 0, 8)
	if strings.EqualFold(target.Scope, scopeUser) {
		args = append(args, "--user")
	}
	args = append(args,
		"-u", target.Unit,
		"--no-pager",
		"-n", fmt.Sprintf("%d", lines),
		"--output=short-iso",
	)
	out, err := m.commandRunner(ctx, "journalctl", args...)
	if err != nil {
		return "", fmt.Errorf("journalctl failed: %w", err)
	}
	return out, nil
}

// LogsByUnit retrieves recent log output for a service identified by
// unit/scope/manager directly, without requiring the service to be tracked.
func (m *Manager) LogsByUnit(ctx context.Context, unit, scope, manager string, lines int) (string, error) {
	if lines <= 0 {
		lines = defaultLogLines
	}
	if lines > maxLogLines {
		lines = maxLogLines
	}

	target := ServiceStatus{
		Unit:    unit,
		Scope:   scope,
		Manager: manager,
	}

	switch manager {
	case managerSystemd:
		return m.logsSystemd(ctx, target, lines)
	case managerLaunchd:
		return m.logsLaunchdUnit(ctx, unit, lines)
	default:
		return "", fmt.Errorf("unsupported service manager: %s", manager)
	}
}

func (m *Manager) logsLaunchdUnit(ctx context.Context, label string, lines int) (string, error) {
	out, err := m.commandRunner(ctx, "log", "show",
		"--predicate", fmt.Sprintf(`senderImagePath CONTAINS "%s" OR subsystem == "%s"`, label, label),
		"--style", "compact",
		"--last", fmt.Sprintf("%dm", max(lines/10, 5)),
	)
	if err != nil {
		return "", fmt.Errorf("log show failed: %w", err)
	}
	outputLines := strings.Split(out, "\n")
	if len(outputLines) > lines {
		outputLines = outputLines[len(outputLines)-lines:]
	}
	return strings.Join(outputLines, "\n"), nil
}

func (m *Manager) logsLaunchd(ctx context.Context, target ServiceStatus, lines int) (string, error) {
	label := unitForService(managerLaunchd, target.Name)
	out, err := m.commandRunner(ctx, "log", "show",
		"--predicate", fmt.Sprintf(`senderImagePath CONTAINS "%s" OR subsystem == "%s"`, label, label),
		"--style", "compact",
		"--last", fmt.Sprintf("%dm", max(lines/10, 5)),
	)
	if err != nil {
		return "", fmt.Errorf("log show failed: %w", err)
	}
	// Trim to requested line count.
	outputLines := strings.Split(out, "\n")
	if len(outputLines) > lines {
		outputLines = outputLines[len(outputLines)-lines:]
	}
	return strings.Join(outputLines, "\n"), nil
}
