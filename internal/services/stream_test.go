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

func installJournalctlCommandRecorder(t *testing.T) {
	t.Helper()

	original := journalctlCommandContext
	t.Cleanup(func() { journalctlCommandContext = original })
	journalctlCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		helperArgs := []string{"-test.run=TestJournalctlCommandRecorder", "--", name}
		helperArgs = append(helperArgs, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], helperArgs...) //nolint:gosec // test helper re-execs the current test binary with recorded args
		cmd.Env = append(os.Environ(), "SENTINEL_JOURNALCTL_COMMAND_RECORDER=1")
		return cmd
	}
}

func TestJournalctlCommandRecorder(t *testing.T) {
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
