package main

import (
	"io"
	"os"
	"strings"

	isatty "github.com/mattn/go-isatty"
)

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
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
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
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
		writef(w, "%s%-*s%s  %s\n", ansiDim, maxKey, row.Key, ansiReset, colorizeValue(row.Value))
	}
}

func printHeading(w io.Writer, title string) {
	if shouldUsePrettyOutput(w) {
		writef(w, "%s%s%s\n", ansiBold, title, ansiReset)
		return
	}
	writeln(w, title)
}

func printNotice(w io.Writer, message string) {
	if shouldUsePrettyOutput(w) {
		writef(w, "%s%s%s\n", ansiGreen, message, ansiReset)
		return
	}
	writeln(w, message)
}

func colorizeValue(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "true", "active", "enabled", "loaded", "ok", "yes", "up":
		return ansiGreen + value + ansiReset
	case "false", "inactive", "disabled", "not-loaded", "unavailable", "not-found", "failed", "error", "down":
		return ansiRed + value + ansiReset
	case "-", "unknown", "n/a":
		return ansiYellow + value + ansiReset
	default:
		return value
	}
}
