package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func writef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func writeln(w io.Writer, args ...any) {
	_, _ = fmt.Fprintln(w, args...)
}

type textStyle int

const (
	stylePlain textStyle = iota
	styleBold
	styleMuted
	styleSuccess
	styleWarning
	styleDanger
)

type outputRow struct {
	Key   string
	Value string
}

func shouldUsePrettyOutput(w io.Writer) bool {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TERM")), "dumb") {
		return false
	}
	fd, ok := fileDescriptor(w)
	if !ok {
		return false
	}
	return isTerminal(fd)
}

func renderStyle(w io.Writer, kind textStyle, s string) string {
	if kind == stylePlain || s == "" || !shouldUsePrettyOutput(w) {
		return s
	}
	style := lipgloss.NewRenderer(w).NewStyle()
	switch kind {
	case stylePlain:
		return s
	case styleBold:
		style = style.Bold(true)
	case styleMuted:
		style = style.Faint(true)
	case styleSuccess:
		style = style.Foreground(lipgloss.Color("42"))
	case styleWarning:
		style = style.Foreground(lipgloss.Color("214"))
	case styleDanger:
		style = style.Foreground(lipgloss.Color("203"))
	}
	return style.Render(s)
}

func fileDescriptor(w io.Writer) (uintptr, bool) {
	type fdWriter interface {
		Fd() uintptr
	}
	f, ok := w.(fdWriter)
	if !ok {
		return 0, false
	}
	return f.Fd(), true
}

func printRows(w io.Writer, rows []outputRow) {
	if !shouldUsePrettyOutput(w) {
		for _, row := range rows {
			writef(w, "%s: %s\n", row.Key, row.Value)
		}
		return
	}

	maxKey := 0
	for _, row := range rows {
		if len(row.Key) > maxKey {
			maxKey = len(row.Key)
		}
	}
	for _, row := range rows {
		key := fmt.Sprintf("%-*s", maxKey, row.Key)
		writef(w, "%s  %s\n", renderStyle(w, styleMuted, key), renderStyle(w, valueStyle(row.Value), row.Value))
	}
}

func printHeading(w io.Writer, title string) {
	writeln(w, renderStyle(w, styleBold, title))
}

func printNotice(w io.Writer, message string) {
	writeln(w, renderStyle(w, styleSuccess, message))
}

func reportHeader(w io.Writer, title, detail string) {
	if detail == "" {
		writeln(w, renderStyle(w, styleBold, title))
		writeln(w)
		return
	}
	writef(w, "%s  %s\n\n", renderStyle(w, styleBold, title), renderStyle(w, styleMuted, detail))
}

func done(w io.Writer, verb, target string) {
	writeln(w, renderStyle(w, styleSuccess, verb), target)
}

func empty(w io.Writer, msg string) {
	writeln(w, renderStyle(w, styleMuted, msg))
}

func valueStyle(value string) textStyle {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "true", "active", "enabled", "loaded", "ok", "yes", "up":
		return styleSuccess
	case "false", "inactive", "disabled", "not-loaded", "unavailable", "not-found", "failed", "error", "down":
		return styleDanger
	case "-", "unknown", "n/a":
		return styleWarning
	default:
		return stylePlain
	}
}
