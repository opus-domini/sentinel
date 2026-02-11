package terminals

import (
	"strings"
	"testing"
)

func TestParsePSOutputGroupsByTTY(t *testing.T) {
	t.Parallel()

	out := []byte(`
100 1 pts/2 hugo zsh -zsh
101 100 pts/2 hugo vim vim main.go
200 1 pts/10 hugo bash bash
201 200 pts/10 hugo python python script.py
300 1 ? root systemd /sbin/init
`)

	terminals, err := parsePSOutput(out)
	if err != nil {
		t.Fatalf("parsePSOutput returned error: %v", err)
	}

	if len(terminals) != 2 {
		t.Fatalf("expected 2 terminals, got %d", len(terminals))
	}

	if terminals[0].TTY != "pts/10" {
		t.Fatalf("expected first terminal tty pts/10, got %q", terminals[0].TTY)
	}
	if terminals[0].ProcessCount != 2 {
		t.Fatalf("expected processCount=2 for pts/10, got %d", terminals[0].ProcessCount)
	}
	if terminals[0].Command != "bash" {
		t.Fatalf("expected command bash for pts/10, got %q", terminals[0].Command)
	}

	if terminals[1].TTY != "pts/2" {
		t.Fatalf("expected second terminal tty pts/2, got %q", terminals[1].TTY)
	}
	if terminals[1].ProcessCount != 2 {
		t.Fatalf("expected processCount=2 for pts/2, got %d", terminals[1].ProcessCount)
	}
	if terminals[1].Command != "zsh" {
		t.Fatalf("expected command zsh for pts/2, got %q", terminals[1].Command)
	}
}

func TestParsePSOutputIncludesTTYConsole(t *testing.T) {
	t.Parallel()

	out := []byte(`
500 1 tty2 root bash -bash
`)

	terminals, err := parsePSOutput(out)
	if err != nil {
		t.Fatalf("parsePSOutput returned error: %v", err)
	}

	if len(terminals) != 1 {
		t.Fatalf("expected 1 terminal, got %d", len(terminals))
	}
	if terminals[0].TTY != "tty2" {
		t.Fatalf("expected tty2, got %q", terminals[0].TTY)
	}
}

func TestIsInteractiveTTY(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tty  string
		want bool
	}{
		{"pts/0", "pts/0", true},
		{"pts/10", "pts/10", true},
		{"ttys001 macOS", "ttys001", true},
		{"tty2 Linux console", "tty2", true},
		{"ttyUSB0 matches tty prefix", "ttyUSB0", true},
		{"question mark", "?", false},
		{"dash", "-", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isInteractiveTTY(tt.tty)
			if got != tt.want {
				t.Fatalf("isInteractiveTTY(%q) = %v, want %v", tt.tty, got, tt.want)
			}
		})
	}
}

func TestCommandScore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		args    string
		want    int
	}{
		{"zsh shell", "zsh", "-zsh", 100},
		{"bash shell", "bash", "bash", 100},
		{"fish shell", "fish", "fish", 100},
		{"tmux", "tmux", "tmux attach", 90},
		{"ssh", "ssh", "ssh user@host", 80},
		{"codex in args", "node", "node --codex-flag", 85},
		{"node in args", "node", "node server.js", 70},
		{"python in args", "python3", "python3 script.py", 70},
		{"default score", "vim", "vim main.go", 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := commandScore(tt.command, tt.args)
			if got != tt.want {
				t.Fatalf("commandScore(%q, %q) = %d, want %d", tt.command, tt.args, got, tt.want)
			}
		})
	}
}

func TestParseProcessOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		filterTTY string
		wantCount int
		wantPIDs  []int
	}{
		{
			name: "filter by TTY",
			input: strings.Join([]string{
				"100 1 pts/1 hugo 1.0 0.5 zsh -zsh",
				"101 100 pts/1 hugo 0.0 0.1 vim vim main.go",
				"200 1 pts/2 hugo 0.0 0.2 bash bash",
			}, "\n"),
			filterTTY: "pts/1",
			wantCount: 2,
			wantPIDs:  []int{100, 101},
		},
		{
			name: "multiple processes sorted by PID",
			input: strings.Join([]string{
				"300 1 pts/3 hugo 0.0 0.1 node node app.js",
				"100 1 pts/3 hugo 1.0 0.5 zsh -zsh",
				"200 100 pts/3 hugo 0.0 0.2 vim vim file.go",
			}, "\n"),
			filterTTY: "pts/3",
			wantCount: 3,
			wantPIDs:  []int{100, 200, 300},
		},
		{
			name: "no match returns nil",
			input: strings.Join([]string{
				"100 1 pts/1 hugo 1.0 0.5 zsh -zsh",
			}, "\n"),
			filterTTY: "pts/99",
			wantCount: 0,
			wantPIDs:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			procs, err := parseProcessOutput([]byte(tt.input), tt.filterTTY)
			if err != nil {
				t.Fatalf("parseProcessOutput returned error: %v", err)
			}
			if len(procs) != tt.wantCount {
				t.Fatalf("expected %d processes, got %d", tt.wantCount, len(procs))
			}
			if tt.wantPIDs == nil {
				if procs != nil {
					t.Fatalf("expected nil slice, got %v", procs)
				}
				return
			}
			for i, wantPID := range tt.wantPIDs {
				if procs[i].PID != wantPID {
					t.Fatalf("process[%d].PID = %d, want %d", i, procs[i].PID, wantPID)
				}
			}
		})
	}
}
