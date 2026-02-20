//go:build linux

package term

import "testing"

func TestResolveShell(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		requestedShell string
		wantErr        bool
	}{
		{"explicit_sh", "/bin/sh", false},
		{"empty_falls_back", "", false},
		{"nonexistent", "/nonexistent/shell/path", false}, // falls back to other candidates
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path, err := resolveShell(tt.requestedShell)
			if tt.wantErr {
				if err == nil {
					t.Errorf("resolveShell(%q) = nil error, want error", tt.requestedShell)
				}
				return
			}
			if err != nil {
				t.Errorf("resolveShell(%q) error = %v", tt.requestedShell, err)
				return
			}
			if path == "" {
				t.Errorf("resolveShell(%q) returned empty path", tt.requestedShell)
			}
		})
	}
}

func TestEnsureEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    []string
		wantTERM bool
		wantLANG bool
	}{
		{
			name:     "empty_env_adds_defaults",
			input:    []string{},
			wantTERM: true,
			wantLANG: true,
		},
		{
			name:     "term_present_skips_term",
			input:    []string{"TERM=screen-256color"},
			wantTERM: false, // already present, should not add
			wantLANG: true,
		},
		{
			name:     "lang_present_skips_lang",
			input:    []string{"LANG=en_US.UTF-8"},
			wantTERM: true,
			wantLANG: false,
		},
		{
			name:     "both_present_skips_both",
			input:    []string{"TERM=screen", "LANG=en_US.UTF-8"},
			wantTERM: false,
			wantLANG: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := ensureEnv(append([]string{}, tt.input...))

			gotTERM := hasEnvKey(result, "TERM")
			gotLANG := hasEnvKey(result, "LANG")

			if !gotTERM {
				t.Error("result missing TERM")
			}
			if !gotLANG {
				t.Error("result missing LANG")
			}

			// Check that defaults were added when needed.
			origLen := len(tt.input)
			expectedAdded := 0
			if tt.wantTERM {
				expectedAdded++
			}
			if tt.wantLANG {
				expectedAdded++
			}
			if len(result) != origLen+expectedAdded {
				t.Errorf("result len = %d, want %d (orig=%d + added=%d)",
					len(result), origLen+expectedAdded, origLen, expectedAdded)
			}
		})
	}
}

func TestHasEnvKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  []string
		key  string
		want bool
	}{
		{"present", []string{"FOO=bar", "BAZ=qux"}, "FOO", true},
		{"absent", []string{"FOO=bar"}, "BAZ", false},
		{"empty_env", []string{}, "FOO", false},
		{"prefix_match_not_key", []string{"FOOBAR=val"}, "FOO", false},
		{"empty_value", []string{"KEY="}, "KEY", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := hasEnvKey(tt.env, tt.key)
			if got != tt.want {
				t.Errorf("hasEnvKey(%v, %q) = %v, want %v", tt.env, tt.key, got, tt.want)
			}
		})
	}
}

func TestPTYResize_InvalidDimensions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cols int
		rows int
	}{
		{"zero_cols", 0, 24},
		{"zero_rows", 80, 0},
		{"negative_cols", -1, 24},
		{"negative_rows", 80, -1},
		{"both_zero", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pty := &PTY{} // no master fd — only testing validation
			err := pty.Resize(tt.cols, tt.rows)
			if err == nil {
				t.Errorf("Resize(%d, %d) = nil, want error", tt.cols, tt.rows)
			}
		})
	}
}

func TestPTYWait_NilCmd(t *testing.T) {
	t.Parallel()

	pty := &PTY{}
	if err := pty.Wait(); err != nil {
		t.Errorf("Wait() on nil cmd = %v, want nil", err)
	}
}
