package services

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestStreamLogsByUnitRejectsUnsupportedManager(t *testing.T) {
	t.Parallel()

	m := NewManager(time.Time{}, nil)
	_, err := m.StreamLogsByUnit(context.Background(), "sentinel.service", scopeUser, managerLaunchd)
	if !errors.Is(err, ErrStreamingUnsupported) {
		t.Fatalf("StreamLogsByUnit() error = %v, want ErrStreamingUnsupported", err)
	}
}

func TestStreamLogsByUnitBuildsJournalctlCommand(t *testing.T) {
	// Not parallel: mutates package-level journalctlCommandContext.

	installJournalctlCommandRecorder(t)

	m := NewManager(time.Time{}, nil)
	reader, err := m.StreamLogsByUnit(context.Background(), "sentinel.service", scopeUser, managerSystemd)
	if err != nil {
		t.Fatalf("StreamLogsByUnit() error = %v", err)
	}
	defer func() { _ = reader.Close() }()

	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(out)), "\n")
	want := []string{
		"journalctl",
		"--user",
		"-u",
		"sentinel.service",
		"--no-pager",
		"-n",
		"50",
		"--output=short-iso",
		"--follow",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("journalctl command = %#v, want %#v", got, want)
	}
}

func TestStreamLogsRejectsInvalidAndUnavailableServices(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		service string
		repo    *stubCustomServicesRepo
		goos    string
		runner  commandRunner
		wantErr error
	}{
		{
			name:    "invalid name",
			service: "   ",
			wantErr: ErrServiceNotFound,
		},
		{
			name:    "repository failure",
			service: ServiceNameSentinel,
			repo:    &stubCustomServicesRepo{err: errors.New("store unavailable")},
			wantErr: errors.New("store unavailable"),
		},
		{
			name:    "service not found",
			service: ServiceNameSentinel,
			repo:    &stubCustomServicesRepo{},
			goos:    "linux",
			wantErr: ErrServiceNotFound,
		},
		{
			name:    "unsupported manager",
			service: ServiceNameSentinel,
			repo:    &stubCustomServicesRepo{},
			goos:    "darwin",
			runner: func(context.Context, string, ...string) (string, error) {
				return "state = running\nlast exit code = 0", nil
			},
			wantErr: ErrStreamingUnsupported,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			manager := &Manager{
				nowFn:         time.Now,
				uidFn:         func() int { return 1000 },
				goos:          tc.goos,
				commandRunner: tc.runner,
			}
			if tc.repo != nil {
				manager.customServices = tc.repo
			}
			_, err := manager.StreamLogs(context.Background(), tc.service)
			if !errors.Is(err, tc.wantErr) && (err == nil || err.Error() != tc.wantErr.Error()) {
				t.Fatalf("StreamLogs() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestStreamLogsBuildsJournalctlCommand(t *testing.T) {
	// Not parallel: mutates package-level journalctlCommandContext.
	installJournalctlCommandRecorder(t)

	manager := &Manager{
		nowFn:          time.Now,
		uidFn:          func() int { return 1000 },
		goos:           "linux",
		customServices: &stubCustomServicesRepo{},
		commandRunner: withSystemdBuiltinStates(map[string]string{
			sentinelSystemdUnit: probeActiveResponse,
		}, func(context.Context, string, ...string) (string, error) {
			return probeActiveResponse, nil
		}),
	}

	reader, err := manager.StreamLogs(context.Background(), ServiceNameSentinel)
	if err != nil {
		t.Fatalf("StreamLogs() error = %v", err)
	}
	defer func() { _ = reader.Close() }()

	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !strings.Contains(string(out), sentinelSystemdUnit) {
		t.Fatalf("journalctl command = %q, want unit %q", out, sentinelSystemdUnit)
	}
}

func installJournalctlCommandRecorder(t *testing.T) {
	t.Helper()

	original := journalctlCommandContext
	t.Cleanup(func() { journalctlCommandContext = original })
	journalctlCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		helperArgs := []string{"-test.run=TestJournalctlCommandRecorder", "--", name}
		helperArgs = append(helperArgs, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], helperArgs...)
		cmd.Env = append(os.Environ(), "SENTINEL_JOURNALCTL_COMMAND_RECORDER=1")
		return cmd
	}
}

func TestJournalctlCommandRecorder(_ *testing.T) {
	if os.Getenv("SENTINEL_JOURNALCTL_COMMAND_RECORDER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			for _, item := range os.Args[i+1:] {
				_, _ = os.Stdout.WriteString(item + "\n")
			}
			os.Exit(0)
		}
	}
	os.Exit(2)
}
