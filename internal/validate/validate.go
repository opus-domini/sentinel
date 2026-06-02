// Package validate validates Sentinel API input.
package validate

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// sessionNameRE requires an alphanumeric/dot/underscore first character so a
// name can never start with '-'. Otherwise tmux parses it as an option flag
// (e.g. rename-session passes the name as a bare positional), letting a crafted
// name like "-d" inject flags into the command.
var sessionNameRE = regexp.MustCompile(`^[A-Za-z0-9._][A-Za-z0-9._-]{0,63}$`)

// SessionName reports whether name is a valid tmux session name.
func SessionName(name string) bool {
	return sessionNameRE.MatchString(name)
}

// windowNameRE allows letters, digits, dots, hyphens, underscores, and spaces,
// but (like sessionNameRE) forbids a leading '-' or ' ' so the name cannot be
// parsed as a tmux option flag.
var windowNameRE = regexp.MustCompile(`^[A-Za-z0-9._][A-Za-z0-9._\- ]{0,63}$`)

// WindowName reports whether name is a valid tmux window name.
func WindowName(name string) bool {
	return windowNameRE.MatchString(name)
}

// paneTitleRE allows printable ASCII and common Unicode up to 128 chars.
var paneTitleRE = regexp.MustCompile(`^.{1,128}$`)

// PaneTitle reports whether title is a valid tmux pane title.
func PaneTitle(title string) bool {
	return paneTitleRE.MatchString(title) && !hasControlOrANSI(title)
}

func hasControlOrANSI(s string) bool {
	for _, r := range s {
		if r == 0x1b || r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

var iconKeyRE = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// IconKey reports whether key is a valid session icon key.
func IconKey(key string) bool {
	return iconKeyRE.MatchString(key)
}

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// CronExpression validates a 5-field cron expression (or @descriptor).
// Returns nil when the expression is valid.
func CronExpression(expr string) error {
	trimmed := strings.TrimSpace(expr)
	upper := strings.ToUpper(trimmed)
	if strings.HasPrefix(trimmed, "@every") || strings.HasPrefix(upper, "TZ=") || strings.HasPrefix(upper, "CRON_TZ=") {
		return fmt.Errorf("invalid cron expression: unsupported schedule prefix")
	}
	_, err := cronParser.Parse(expr)
	if err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}
	return nil
}

// ParseCron parses a cron expression and returns the schedule.
func ParseCron(expr string) (cron.Schedule, error) {
	return cronParser.Parse(expr)
}

// Timezone validates an IANA timezone string.
func Timezone(tz string) error {
	_, err := time.LoadLocation(tz)
	if err != nil {
		return fmt.Errorf("invalid timezone: %w", err)
	}
	return nil
}
