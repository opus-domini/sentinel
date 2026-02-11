package validate

import (
	"strings"
	"testing"
)

func TestSessionName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"alphanumeric", "myapp", true},
		{"single_char", "a", true},
		{"max_length_64", strings.Repeat("x", 64), true},
		{"all_numeric", "12345", true},
		{"with_dots", "my.app", true},
		{"with_underscores", "my_app", true},
		{"with_hyphens", "my-app", true},
		{"mixed_valid", "My.App_v2-test", true},
		{"uppercase", "ALLCAPS", true},

		{"empty", "", false},
		{"too_long_65", strings.Repeat("x", 65), false},
		{"with_space", "has space", false},
		{"with_slash", "has/slash", false},
		{"with_semicolon", "has;semicolon", false},
		{"with_unicode", "café", false},
		{"with_tab", "tab\there", false},
		{"with_colon", "has:colon", false},
		{"with_newline", "has\nnewline", false},
		{"with_at", "has@sign", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := SessionName(tt.input)
			if got != tt.want {
				t.Errorf("SessionName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
