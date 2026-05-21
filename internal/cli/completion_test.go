package cli

import (
	"bytes"
	"testing"
)

// TestCompletionCommand verifies cobra's generated completion command emits a
// non-empty script for each supported shell.
func TestCompletionCommand(t *testing.T) {
	t.Parallel()

	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			t.Parallel()

			var out, errOut bytes.Buffer
			code := Run([]string{"completion", shell}, &out, &errOut)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
			}
			if out.Len() == 0 {
				t.Fatalf("completion %s produced no script", shell)
			}
		})
	}
}
