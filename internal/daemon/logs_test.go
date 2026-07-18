package daemon

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestJournalctlLogArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		system bool
		follow bool
		lines  int
		want   []string
	}{
		{
			name:  "user scope paged",
			lines: 50,
			want:  []string{"--user", "-u", userUnitName, "-n", "50", "--no-pager"},
		},
		{
			name:   "user scope follow",
			follow: true,
			lines:  120,
			want:   []string{"--user", "-u", userUnitName, "-n", "120", "-f"},
		},
		{
			name:   "system scope drops --user",
			system: true,
			lines:  10,
			want:   []string{"-u", userUnitName, "-n", "10", "--no-pager"},
		},
		{
			name:  "non-positive lines falls back to default",
			lines: 0,
			want:  []string{"--user", "-u", userUnitName, "-n", "50", "--no-pager"},
		},
		{
			name:  "negative lines falls back to default",
			lines: -7,
			want:  []string{"--user", "-u", userUnitName, "-n", "50", "--no-pager"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := journalctlLogArgs(tc.system, tc.follow, tc.lines)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("journalctlLogArgs(%t, %t, %d) = %v, want %v", tc.system, tc.follow, tc.lines, got, tc.want)
			}
		})
	}
}

func TestTailLogArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		follow bool
		lines  int
		paths  []string
		want   []string
	}{
		{
			name:  "paged with two log files",
			lines: 50,
			paths: []string{"/log/out", "/log/err"},
			want:  []string{"-n", "50", "/log/out", "/log/err"},
		},
		{
			name:   "follow inserts -f before paths",
			follow: true,
			lines:  25,
			paths:  []string{"/log/out", "/log/err"},
			want:   []string{"-n", "25", "-f", "/log/out", "/log/err"},
		},
		{
			name:  "non-positive lines falls back to default",
			lines: 0,
			paths: []string{"/log/out"},
			want:  []string{"-n", "50", "/log/out"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tailLogArgs(tc.follow, tc.lines, tc.paths...)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("tailLogArgs(%t, %d, %v) = %v, want %v", tc.follow, tc.lines, tc.paths, got, tc.want)
			}
		})
	}
}

func TestRunLogCommand(t *testing.T) {
	t.Parallel()

	if err := runLogCommand(exec.Command("sh", "-c", "exit 0")); err != nil {
		t.Fatalf("successful command error = %v", err)
	}
	if err := runLogCommand(exec.Command("sh", "-c", "exit 7")); err != nil {
		t.Fatalf("exit status should be ignored: %v", err)
	}
	cmd := &exec.Cmd{Path: "/definitely/missing/sentinel-log-command"}
	if err := runLogCommand(cmd); err == nil || !strings.Contains(err.Error(), "run sentinel-log-command") {
		t.Fatalf("start failure error = %v", err)
	}
}
