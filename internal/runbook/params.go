package runbook

import (
	"fmt"
	"strings"

	"github.com/opus-domini/sentinel/internal/store"
)

// ShellEscape wraps a string in single quotes and escapes any embedded
// single quotes. This prevents shell injection when substituting user-
// provided parameter values into commands.
func ShellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// ValidateParams checks that all required parameters have non-empty
// values in the provided map. It returns an error describing the first
// missing required parameter.
func ValidateParams(defs []store.RunbookParameter, values map[string]string) error {
	for _, p := range defs {
		if !p.Required {
			continue
		}
		v, ok := values[p.Name]
		if !ok || strings.TrimSpace(v) == "" {
			return fmt.Errorf("required parameter %q is missing", p.Name)
		}
	}
	return nil
}

// ResolveParams builds a complete parameter map by merging user-supplied
// values with defaults from the parameter definitions. Only parameters
// that are defined in the runbook are included in the result.
func ResolveParams(defs []store.RunbookParameter, values map[string]string) map[string]string {
	resolved := make(map[string]string, len(defs))
	for _, p := range defs {
		if v, ok := values[p.Name]; ok && v != "" {
			resolved[p.Name] = v
		} else if p.Default != "" {
			resolved[p.Name] = p.Default
		} else {
			resolved[p.Name] = ""
		}
	}
	return resolved
}

// SubstituteParams replaces {{PARAM_NAME}} placeholders in the command
// string with shell-escaped parameter values. Unknown placeholders are
// left unchanged.
func SubstituteParams(command string, params map[string]string) string {
	for name, value := range params {
		placeholder := "{{" + name + "}}"
		command = strings.ReplaceAll(command, placeholder, ShellEscape(value))
	}
	return command
}
