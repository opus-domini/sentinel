package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestRunCompletionCommand covers each supported shell plus the error paths.
func TestRunCompletionCommand(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string // fragment expected in stdout
		wantErr  string // fragment expected in stderr
	}{
		{
			name:     "bash",
			args:     []string{"completion", "bash"},
			wantCode: 0,
			wantOut:  "complete -F _sentinel sentinel",
		},
		{
			name:     "zsh",
			args:     []string{"completion", "zsh"},
			wantCode: 0,
			wantOut:  "#compdef sentinel",
		},
		{
			name:     "fish",
			args:     []string{"completion", "fish"},
			wantCode: 0,
			wantOut:  "complete -c sentinel",
		},
		{
			name:     "shell name is case-insensitive",
			args:     []string{"completion", "BASH"},
			wantCode: 0,
			wantOut:  "_sentinel",
		},
		{
			name:     "missing shell argument",
			args:     []string{"completion"},
			wantCode: 2,
			wantErr:  "exactly one shell argument",
		},
		{
			name:     "too many arguments",
			args:     []string{"completion", "bash", "zsh"},
			wantCode: 2,
			wantErr:  "exactly one shell argument",
		},
		{
			name:     "unsupported shell",
			args:     []string{"completion", "powershell"},
			wantCode: 2,
			wantErr:  "unsupported shell: powershell",
		},
		{
			name:     "help flag",
			args:     []string{"completion", "--help"},
			wantCode: 0,
			wantOut:  "sentinel completion <bash|zsh|fish>",
		},
		{
			name:     "invalid flag",
			args:     []string{"completion", "--bogus"},
			wantCode: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var out, errOut bytes.Buffer
			code := Run(tc.args, &out, &errOut)
			if code != tc.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, tc.wantCode, errOut.String())
			}
			if tc.wantOut != "" && !strings.Contains(out.String(), tc.wantOut) {
				t.Fatalf("stdout missing %q:\n%s", tc.wantOut, out.String())
			}
			if tc.wantErr != "" && !strings.Contains(errOut.String(), tc.wantErr) {
				t.Fatalf("stderr missing %q:\n%s", tc.wantErr, errOut.String())
			}
		})
	}
}

// TestCompletionScriptsEmbedded verifies every advertised shell script is
// embedded and non-empty.
func TestCompletionScriptsEmbedded(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"completions/sentinel.bash",
		"completions/sentinel.zsh",
		"completions/sentinel.fish",
	} {
		data, err := completionScripts.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", path, err)
		}
		if len(data) == 0 {
			t.Fatalf("%q is empty", path)
		}
	}
}
