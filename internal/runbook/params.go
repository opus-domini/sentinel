package runbook

import (
	"fmt"
	"strings"

	"github.com/opus-domini/sentinel/internal/store"
)

// ValidateInputParams rejects misspelled or otherwise undeclared parameter
// names before values are resolved with defaults.
func ValidateInputParams(defs []store.RunbookParameter, values map[string]string) error {
	defined := make(map[string]struct{}, len(defs))
	for _, parameter := range defs {
		defined[parameter.Name] = struct{}{}
	}
	for name := range values {
		if _, ok := defined[name]; !ok {
			return fmt.Errorf("parameter %q is not defined by this runbook", name)
		}
	}
	return nil
}

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
		value, ok := values[p.Name]
		if p.Required && (!ok || strings.TrimSpace(value) == "") {
			return fmt.Errorf("required parameter %q is missing", p.Name)
		}
		if !ok || value == "" {
			continue
		}
		if err := validateParameterValue(p, value); err != nil {
			return err
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
