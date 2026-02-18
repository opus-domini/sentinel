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

func TestIconKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"lowercase", "terminal", true},
		{"with hyphens", "arrow-left", true},
		{"with numbers", "h2", true},
		{"all numbers", "123", true},
		{"single char", "a", true},
		{"max length 32", strings.Repeat("a", 32), true},
		{"mixed hyphens numbers", "icon-v2-alt", true},

		{"empty", "", false},
		{"too long 33", strings.Repeat("a", 33), false},
		{"uppercase", "Terminal", false},
		{"all caps", "ICON", false},
		{"with spaces", "arrow left", false},
		{"with underscore", "arrow_left", false},
		{"with dot", "icon.name", false},
		{"with at sign", "icon@name", false},
		{"with slash", "icon/name", false},
		{"with unicode", "café", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := IconKey(tt.input)
			if got != tt.want {
				t.Errorf("IconKey(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
