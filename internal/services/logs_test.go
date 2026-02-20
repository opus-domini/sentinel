package services

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/daemon"
)

const (
	cmdJournalctl = "journalctl"
	cmdLog        = "log"
)

func newLogsTestManager(goos string, runner commandRunner) *Manager {
	return &Manager{
		nowFn:    time.Now,
		hostname: func() (string, error) { return "test-host", nil },
		uidFn:    func() int { return 1000 },
		goos:     goos,
		userStatusFn: func() (daemon.UserServiceStatus, error) {
			return daemon.UserServiceStatus{
				ServicePath:    "/home/dev/.config/systemd/user/sentinel.service",
				UnitFileExists: true,
				EnabledState:   "enabled",
				ActiveState:    "active",
			}, nil
		},
		autoUpdateStatusFn: func(string) (daemon.UserAutoUpdateServiceStatus, error) {
			return daemon.UserAutoUpdateServiceStatus{
				TimerUnitExists:   true,
				TimerEnabledState: "enabled",
				TimerActiveState:  "active",
			}, nil
		},
		commandRunner: runner,
	}
}

func TestLogs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		svcName   string
		lines     int
		goos      string
		runner    commandRunner
		wantErr   error
		wantLines int
	}{
		{
			name:    "unknown service returns ErrServiceNotFound",
			svcName: "",
			lines:   50,
			goos:    "linux",
			wantErr: ErrServiceNotFound,
		},
		{
			name:    "systemd logs success",
			svcName: "sentinel",
			lines:   50,
			goos:    "linux",
			runner: func(_ context.Context, name string, args ...string) (string, error) {
				if name == cmdJournalctl {
					return "line1\nline2\nline3", nil
				}
				return "", nil
			},
		},
		{
			name:    "default lines when zero",
			svcName: "sentinel",
			lines:   0,
			goos:    "linux",
			runner: func(_ context.Context, name string, args ...string) (string, error) {
				if name == cmdJournalctl {
					// Verify -n uses defaultLogLines.
					for i, a := range args {
						if a == "-n" && i+1 < len(args) && args[i+1] == "100" {
							return "ok", nil
						}
					}
					return "", errors.New("expected -n 100")
				}
				return "", nil
			},
		},
		{
			name:    "cap lines at maxLogLines",
			svcName: "sentinel",
			lines:   9999,
			goos:    "linux",
			runner: func(_ context.Context, name string, args ...string) (string, error) {
				if name == cmdJournalctl {
					for i, a := range args {
						if a == "-n" && i+1 < len(args) && args[i+1] == "1000" {
							return "ok", nil
						}
					}
					return "", errors.New("expected -n 1000")
				}
				return "", nil
			},
		},
		{
			name:    "systemd journal error propagates",
			svcName: "sentinel",
			lines:   50,
			goos:    "linux",
			runner: func(_ context.Context, name string, _ ...string) (string, error) {
				if name == cmdJournalctl {
					return "", errors.New("journal unavailable")
				}
				return "", nil
			},
			wantErr: errors.New("journalctl failed"),
		},
		{
			name:    "launchd logs success",
			svcName: "sentinel",
			lines:   20,
			goos:    "darwin",
			runner: func(_ context.Context, name string, _ ...string) (string, error) {
				if name == cmdLog {
					return "2026-02-15 12:00:00 sentinel started\n2026-02-15 12:00:01 ready", nil
				}
				return "", nil
			},
		},
		{
			name:    "launchd log error propagates",
			svcName: "sentinel",
			lines:   20,
			goos:    "darwin",
			runner: func(_ context.Context, name string, _ ...string) (string, error) {
				if name == cmdLog {
					return "", errors.New("log show failed")
				}
				return "", nil
			},
			wantErr: errors.New("log show failed"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := newLogsTestManager(tc.goos, tc.runner)
			out, err := m.Logs(context.Background(), tc.svcName, tc.lines)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !errors.Is(err, tc.wantErr) && !strings.Contains(err.Error(), tc.wantErr.Error()) {
					t.Fatalf("error = %v, want containing %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			_ = out
		})
	}
}

func TestLogsUnsupportedManager(t *testing.T) {
	t.Parallel()

	// Use a custom service with an unknown manager to trigger the default branch.
	fixedNow := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	m := &Manager{
		nowFn:    func() time.Time { return fixedNow },
		hostname: func() (string, error) { return "test", nil },
		uidFn:    func() int { return 1000 },
		goos:     "linux",
		userStatusFn: func() (daemon.UserServiceStatus, error) {
			return daemon.UserServiceStatus{
				ServicePath:    "/home/dev/.config/systemd/user/sentinel.service",
				UnitFileExists: true,
				EnabledState:   "enabled",
				ActiveState:    "active",
			}, nil
		},
		autoUpdateStatusFn: func(string) (daemon.UserAutoUpdateServiceStatus, error) {
			return daemon.UserAutoUpdateServiceStatus{
				TimerUnitExists:   true,
				TimerEnabledState: "enabled",
				TimerActiveState:  "active",
			}, nil
		},
	}

	// We can't directly trigger "unsupported manager" for built-in services
	// since detectManager always returns systemd or launchd based on goos.
	// Instead, test via LogsByUnit with an unsupported manager.
	_, err := m.LogsByUnit(context.Background(), "some.service", "user", "openrc", 50)
	if err == nil {
		t.Fatal("expected error for unsupported manager")
	}
	if !strings.Contains(err.Error(), "unsupported service manager") {
		t.Fatalf("error = %v, want unsupported service manager", err)
	}
}

func TestLogsByUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		unit    string
		scope   string
		manager string
		lines   int
		runner  commandRunner
		wantErr string
	}{
		{
			name:    "systemd logs by unit",
			unit:    "nginx.service",
			scope:   "system",
			manager: "systemd",
			lines:   50,
			runner: func(_ context.Context, name string, args ...string) (string, error) {
				if name == cmdJournalctl {
					// Should NOT have --user for system scope.
					if slices.Contains(args, "--user") {
						return "", errors.New("unexpected --user flag for system scope")
					}
					return "nginx log output", nil
				}
				return "", nil
			},
		},
		{
			name:    "systemd user scope includes --user",
			unit:    "app.service",
			scope:   "user",
			manager: "systemd",
			lines:   50,
			runner: func(_ context.Context, name string, args ...string) (string, error) {
				if name == cmdJournalctl {
					if len(args) == 0 || args[0] != "--user" {
						return "", errors.New("expected --user flag for user scope")
					}
					return "app user log", nil
				}
				return "", nil
			},
		},
		{
			name:    "launchd logs by unit",
			unit:    "com.example.app",
			scope:   "user",
			manager: "launchd",
			lines:   30,
			runner: func(_ context.Context, name string, _ ...string) (string, error) {
				if name == cmdLog {
					return "launchd log output", nil
				}
				return "", nil
			},
		},
		{
			name:    "unsupported manager",
			unit:    "unknown",
			scope:   "user",
			manager: "openrc",
			lines:   50,
			wantErr: "unsupported service manager",
		},
		{
			name:    "default lines applied",
			unit:    "test.service",
			scope:   "user",
			manager: "systemd",
			lines:   0,
			runner: func(_ context.Context, name string, args ...string) (string, error) {
				if name == cmdJournalctl {
					for i, a := range args {
						if a == "-n" && i+1 < len(args) && args[i+1] == "100" {
							return "ok", nil
						}
					}
					return "", errors.New("expected -n 100")
				}
				return "", nil
			},
		},
		{
			name:    "max lines capped",
			unit:    "test.service",
			scope:   "user",
			manager: "systemd",
			lines:   5000,
			runner: func(_ context.Context, name string, args ...string) (string, error) {
				if name == cmdJournalctl {
					for i, a := range args {
						if a == "-n" && i+1 < len(args) && args[i+1] == "1000" {
							return "ok", nil
						}
					}
					return "", errors.New("expected -n 1000")
				}
				return "", nil
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &Manager{
				nowFn:         time.Now,
				uidFn:         func() int { return 1000 },
				goos:          "linux",
				commandRunner: tc.runner,
			}

			_, err := m.LogsByUnit(context.Background(), tc.unit, tc.scope, tc.manager, tc.lines)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestLogsLaunchdTruncatesOutput(t *testing.T) {
	t.Parallel()

	// Generate output with more lines than requested.
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "line"
	}
	bigOutput := strings.Join(lines, "\n")

	m := newLogsTestManager("darwin", func(_ context.Context, name string, _ ...string) (string, error) {
		if name == cmdLog {
			return bigOutput, nil
		}
		return "", nil
	})

	out, err := m.Logs(context.Background(), "sentinel", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	outputLines := strings.Split(out, "\n")
	if len(outputLines) > 10 {
		t.Fatalf("output has %d lines, want <= 10", len(outputLines))
	}
}
