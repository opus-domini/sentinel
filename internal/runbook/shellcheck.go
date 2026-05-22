package runbook

import (
	"errors"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// ShellWarning describes a syntax issue found in a shell command or script.
type ShellWarning struct {
	Step    int    `json:"step"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
}

// ValidateShellSyntax parses a single-line shell command and returns any
// syntax warnings. The step parameter identifies which runbook step the
// command belongs to.
func ValidateShellSyntax(step int, command string) []ShellWarning {
	return validateShell(step, command)
}

// ValidateScriptSyntax parses a multiline shell script and returns any
// syntax warnings. The step parameter identifies which runbook step the
// script belongs to.
func ValidateScriptSyntax(step int, script string) []ShellWarning {
	return validateShell(step, script)
}

func validateShell(step int, src string) []ShellWarning {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil
	}

	parser := syntax.NewParser(syntax.KeepComments(true))
	_, err := parser.Parse(strings.NewReader(src), "")
	if err == nil {
		return nil
	}

	w := ShellWarning{
		Step:    step,
		Line:    1,
		Column:  1,
		Message: err.Error(),
	}

	// Try to extract position from the parser error.
	var pe syntax.ParseError
	if asParseError(err, &pe) {
		w.Line = int(pe.Pos.Line())
		w.Column = int(pe.Pos.Col())
		w.Message = pe.Text
	}

	return []ShellWarning{w}
}

// asParseError attempts to extract a syntax.ParseError from the error.
// The syntax package may return the error directly or wrapped.
func asParseError(err error, target *syntax.ParseError) bool {
	var pe syntax.ParseError
	if errors.As(err, &pe) {
		*target = pe
		return true
	}
	// Also try the error message as a fallback — some versions wrap.
	msg := err.Error()
	if strings.Contains(msg, ":") {
		// Best-effort: the error itself is descriptive enough.
		_ = msg
	}
	return false
}

// ValidateRunbookShellSyntax checks all run/script steps in a runbook and
// returns accumulated warnings. This is intended for use after save to
// provide non-blocking feedback.
func ValidateRunbookShellSyntax(steps []Step) []ShellWarning {
	var warnings []ShellWarning
	for i, s := range steps {
		switch s.Type {
		case stepTypeRun:
			if w := ValidateShellSyntax(i, s.Command); len(w) > 0 {
				warnings = append(warnings, w...)
			}
		case stepTypeScript:
			if w := ValidateScriptSyntax(i, s.Script); len(w) > 0 {
				warnings = append(warnings, w...)
			}
		}
	}
	return warnings
}

// ValidateShellSyntaxFromStrings is a simple helper for validating a list of
// steps given as type/command/script triples.
func ValidateShellSyntaxFromStrings(steps []ShellCheckInput) []ShellWarning {
	var warnings []ShellWarning
	for _, s := range steps {
		switch s.Type {
		case stepTypeRun:
			if w := ValidateShellSyntax(s.Step, s.Source); len(w) > 0 {
				warnings = append(warnings, w...)
			}
		case stepTypeScript:
			if w := ValidateScriptSyntax(s.Step, s.Source); len(w) > 0 {
				warnings = append(warnings, w...)
			}
		}
	}
	return warnings
}

// ShellCheckInput describes a step to validate.
type ShellCheckInput struct {
	Step   int
	Type   string // "run" or "script"
	Source string // command or script body
}

// FormatWarnings produces a human-readable summary of shell warnings.
func FormatWarnings(warnings []ShellWarning) string {
	if len(warnings) == 0 {
		return ""
	}
	var b strings.Builder
	for i, w := range warnings {
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "step %d (line %d, col %d): %s", w.Step, w.Line, w.Column, w.Message)
	}
	return b.String()
}
