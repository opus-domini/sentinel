package config

import (
	"bufio"
	"os"
	"os/user"
	"slices"
	"strconv"
	"strings"
)

// readPasswdFile is the function used to open /etc/passwd. It exists as a
// package-level variable so tests can inject sample content.
var readPasswdFile = func() (*os.File, error) { //nolint:gochecknoglobals // var enables test injection
	return os.Open("/etc/passwd")
}

// excludedShells are login shells that indicate non-interactive system accounts.
var excludedShells = map[string]struct{}{
	"/usr/sbin/nologin": {},
	"/usr/bin/nologin":  {},
	"/sbin/nologin":     {},
	"/bin/nologin":      {},
	"/bin/false":        {},
	"/usr/bin/false":    {},
}

// ReadSystemUsers reads the list of non-system human users from the OS.
// On Linux, reads /etc/passwd and filters to UID >= 1000 (normal users)
// plus root (UID 0). Excludes nologin/false shell users.
// The current process user is always included as a fallback.
// Returns a sorted list of usernames; returns an empty slice on error.
func ReadSystemUsers() []string {
	seen := make(map[string]struct{})

	if users := readPasswdUsers(); len(users) > 0 {
		for _, u := range users {
			seen[u] = struct{}{}
		}
	}

	// Always include the current process user as a fallback.
	if current, err := user.Current(); err == nil && current != nil {
		name := strings.TrimSpace(current.Username)
		if name != "" {
			seen[name] = struct{}{}
		}
	}

	if len(seen) == 0 {
		return nil
	}

	users := make([]string, 0, len(seen))
	for u := range seen {
		users = append(users, u)
	}
	slices.Sort(users)
	return users
}

// readPasswdUsers parses /etc/passwd and returns usernames for interactive
// accounts (UID >= 1000 or UID == 0, with a real login shell).
func readPasswdUsers() []string {
	f, err := readPasswdFile()
	if err != nil {
		return nil
	}
	defer f.Close() //nolint:errcheck // best-effort read of /etc/passwd

	return parsePasswd(f)
}

// parsePasswd extracts usernames from an /etc/passwd-formatted reader.
func parsePasswd(f *os.File) []string {
	var users []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.SplitN(line, ":", 8)
		if len(fields) < 7 {
			continue
		}
		username := fields[0]
		uidStr := fields[2]
		shell := fields[6]

		uid, err := strconv.Atoi(uidStr)
		if err != nil {
			continue
		}

		// Only root (UID 0) and normal users (UID >= 1000).
		if uid != 0 && uid < 1000 {
			continue
		}

		// Exclude accounts with non-interactive shells.
		if _, excluded := excludedShells[shell]; excluded {
			continue
		}

		if username != "" {
			users = append(users, username)
		}
	}
	return users
}
