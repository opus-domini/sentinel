package runbook

import (
	"testing"

	"github.com/opus-domini/sentinel/internal/store"
)

func TestShellEscape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple string", input: "hello", want: "'hello'"},
		{name: "empty string", input: "", want: "''"},
		{name: "with spaces", input: "hello world", want: "'hello world'"},
		{name: "with single quote", input: "it's", want: "'it'\\''s'"},
		{name: "multiple single quotes", input: "it's a 'test'", want: "'it'\\''s a '\\''test'\\'''"},
		{name: "with double quotes", input: `say "hello"`, want: `'say "hello"'`},
		{name: "with semicolons", input: "echo; rm -rf /", want: "'echo; rm -rf /'"},
		{name: "with backticks", input: "`whoami`", want: "'`whoami`'"},
		{name: "with dollar sign", input: "$HOME", want: "'$HOME'"},
		{name: "with newline", input: "line1\nline2", want: "'line1\nline2'"},
		{name: "with pipe", input: "echo | cat", want: "'echo | cat'"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ShellEscape(tc.input)
			if got != tc.want {
				t.Errorf("ShellEscape(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidateParams(t *testing.T) {
	t.Parallel()

	defs := []store.RunbookParameter{
		{Name: "host", Label: "Host", Type: "string", Required: true},
		{Name: "port", Label: "Port", Type: "number", Required: false, Default: "8080"},
		{Name: "env", Label: "Environment", Type: "select", Required: true, Options: []string{"dev", "staging", "prod"}},
	}

	tests := []struct {
		name    string
		values  map[string]string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "all required present",
			values:  map[string]string{"host": "example.com", "env": "prod"},
			wantErr: false,
		},
		{
			name:    "all params present including optional",
			values:  map[string]string{"host": "example.com", "port": "9090", "env": "dev"},
			wantErr: false,
		},
		{
			name:    "missing required host",
			values:  map[string]string{"env": "prod"},
			wantErr: true,
			errMsg:  `required parameter "host" is missing`,
		},
		{
			name:    "empty required host",
			values:  map[string]string{"host": "", "env": "prod"},
			wantErr: true,
			errMsg:  `required parameter "host" is missing`,
		},
		{
			name:    "whitespace-only required host",
			values:  map[string]string{"host": "  ", "env": "prod"},
			wantErr: true,
			errMsg:  `required parameter "host" is missing`,
		},
		{
			name:    "missing required env",
			values:  map[string]string{"host": "example.com"},
			wantErr: true,
			errMsg:  `required parameter "env" is missing`,
		},
		{
			name:    "empty values map",
			values:  map[string]string{},
			wantErr: true,
			errMsg:  `required parameter "host" is missing`,
		},
		{
			name:    "nil values map",
			values:  nil,
			wantErr: true,
			errMsg:  `required parameter "host" is missing`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateParams(defs, tc.values)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if err.Error() != tc.errMsg {
					t.Fatalf("error = %q, want %q", err.Error(), tc.errMsg)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateParamsNoDefs(t *testing.T) {
	t.Parallel()

	// No parameter definitions — any values map should pass.
	if err := ValidateParams(nil, map[string]string{"extra": "value"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := ValidateParams([]store.RunbookParameter{}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveParams(t *testing.T) {
	t.Parallel()

	defs := []store.RunbookParameter{
		{Name: "host", Label: "Host", Type: "string", Required: true},
		{Name: "port", Label: "Port", Type: "number", Default: "8080"},
		{Name: "env", Label: "Environment", Type: "select", Required: true},
	}

	tests := []struct {
		name   string
		values map[string]string
		want   map[string]string
	}{
		{
			name:   "all supplied",
			values: map[string]string{"host": "example.com", "port": "9090", "env": "prod"},
			want:   map[string]string{"host": "example.com", "port": "9090", "env": "prod"},
		},
		{
			name:   "default used for port",
			values: map[string]string{"host": "example.com", "env": "dev"},
			want:   map[string]string{"host": "example.com", "port": "8080", "env": "dev"},
		},
		{
			name:   "empty values use defaults or empty",
			values: map[string]string{},
			want:   map[string]string{"host": "", "port": "8080", "env": ""},
		},
		{
			name:   "nil values use defaults or empty",
			values: nil,
			want:   map[string]string{"host": "", "port": "8080", "env": ""},
		},
		{
			name:   "extra values ignored",
			values: map[string]string{"host": "a.com", "env": "staging", "unknown": "val"},
			want:   map[string]string{"host": "a.com", "port": "8080", "env": "staging"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ResolveParams(defs, tc.values)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d; got %v", len(got), len(tc.want), got)
			}
			for k, wantV := range tc.want {
				if got[k] != wantV {
					t.Errorf("resolved[%q] = %q, want %q", k, got[k], wantV)
				}
			}
		})
	}
}

func TestSubstituteParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		params  map[string]string
		want    string
	}{
		{
			name:    "single substitution",
			command: "echo {{NAME}}",
			params:  map[string]string{"NAME": "hello"},
			want:    "echo 'hello'",
		},
		{
			name:    "multiple substitutions",
			command: "ssh {{USER}}@{{HOST}} -p {{PORT}}",
			params:  map[string]string{"USER": "admin", "HOST": "server.com", "PORT": "22"},
			want:    "ssh 'admin'@'server.com' -p '22'",
		},
		{
			name:    "repeated placeholder",
			command: "echo {{X}} {{X}}",
			params:  map[string]string{"X": "val"},
			want:    "echo 'val' 'val'",
		},
		{
			name:    "no placeholders",
			command: "echo hello",
			params:  map[string]string{"NAME": "world"},
			want:    "echo hello",
		},
		{
			name:    "unknown placeholder left unchanged",
			command: "echo {{UNKNOWN}}",
			params:  map[string]string{"NAME": "val"},
			want:    "echo {{UNKNOWN}}",
		},
		{
			name:    "empty params map",
			command: "echo {{NAME}}",
			params:  map[string]string{},
			want:    "echo {{NAME}}",
		},
		{
			name:    "nil params map",
			command: "echo {{NAME}}",
			params:  nil,
			want:    "echo {{NAME}}",
		},
		{
			name:    "value with shell injection",
			command: "echo {{INPUT}}",
			params:  map[string]string{"INPUT": "'; rm -rf / #"},
			want:    "echo ''\\''; rm -rf / #'",
		},
		{
			name:    "value with single quote",
			command: "echo {{MSG}}",
			params:  map[string]string{"MSG": "it's working"},
			want:    "echo 'it'\\''s working'",
		},
		{
			name:    "empty value",
			command: "echo {{EMPTY}}",
			params:  map[string]string{"EMPTY": ""},
			want:    "echo ''",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := SubstituteParams(tc.command, tc.params)
			if got != tc.want {
				t.Errorf("SubstituteParams(%q, %v) = %q, want %q", tc.command, tc.params, got, tc.want)
			}
		})
	}
}

func TestSubstituteParamsBackwardCompatibility(t *testing.T) {
	t.Parallel()

	// Commands without parameters should pass through unchanged.
	cmd := "systemctl restart sentinel"
	got := SubstituteParams(cmd, nil)
	if got != cmd {
		t.Errorf("got %q, want %q (unchanged)", got, cmd)
	}

	got = SubstituteParams(cmd, map[string]string{})
	if got != cmd {
		t.Errorf("got %q, want %q (unchanged)", got, cmd)
	}
}
