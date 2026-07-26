// Package testenv isolates test binaries from the developer's real Sentinel
// installation and host service managers.
package testenv

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	blockedCommandExitCode = 125
	blockedNetworkProxy    = "http://127.0.0.1:1"
)

var blockedCommands = []string{
	"journalctl",
	"launchctl",
	"log",
	"pkexec",
	"sudo",
	"systemctl",
	"systemd-run",
	"tmux",
	"top",
	"vm_stat",
}

var hostHTTPTransport = http.DefaultTransport

type loopbackOnlyTransport struct {
	base http.RoundTripper
}

func (t loopbackOnlyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	host := strings.TrimSuffix(strings.ToLower(req.URL.Hostname()), ".")
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, fmt.Errorf("outbound HTTP request blocked during tests: %s://%s", req.URL.Scheme, req.URL.Host)
	}
	return t.base.RoundTrip(req)
}

// Option customizes a package test sandbox.
type Option func(*options)

type options struct {
	emptyPath bool
}

// WithEmptyPath removes every host executable from PATH. Tests that need a
// command must prepend a fake executable created inside their own temporary
// directory.
func WithEmptyPath() Option {
	return func(opts *options) {
		opts.emptyPath = true
	}
}

// Run executes a package test binary inside an isolated home, data directory,
// runtime directory, command path, and outbound-network environment.
//
// A package fails even when a test ignores the error from a blocked host
// command. Tests that exercise command integration must put their own fake
// executable earlier in PATH.
func Run(m *testing.M, optionFns ...Option) {
	opts := options{}
	for _, optionFn := range optionFns {
		optionFn(&opts)
	}

	root, err := os.MkdirTemp("", "sentinel-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create test sandbox: %v\n", err)
		os.Exit(2)
	}

	commandLog := filepath.Join(root, "blocked-host-commands.log")
	if err := configure(root, commandLog, opts); err != nil {
		fmt.Fprintf(os.Stderr, "configure test sandbox: %v\n", err)
		_ = os.RemoveAll(root)
		os.Exit(2)
	}

	code := m.Run()
	// #nosec G304 -- commandLog is constructed below the freshly-created sandbox.
	if attempts, err := os.ReadFile(commandLog); err == nil && len(strings.TrimSpace(string(attempts))) > 0 {
		fmt.Fprintf(os.Stderr, "\nblocked host command attempted during tests:\n%s", attempts)
		code = 1
	} else if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "read blocked command log: %v\n", err)
		code = 1
	}

	if err := os.RemoveAll(root); err != nil {
		fmt.Fprintf(os.Stderr, "remove test sandbox: %v\n", err)
		code = 1
	}
	os.Exit(code)
}

func configure(root, commandLog string, opts options) error {
	recorderEnv := map[string]string{}
	for _, key := range []string{
		"SENTINEL_EXEC_COMMAND_RECORDER",
		"SENTINEL_JOURNALCTL_COMMAND_RECORDER",
	} {
		if value := os.Getenv(key); value != "" {
			recorderEnv[key] = value
		}
	}
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if ok && strings.HasPrefix(key, "SENTINEL_") {
			if err := os.Unsetenv(key); err != nil {
				return fmt.Errorf("unset %s: %w", key, err)
			}
		}
	}
	for key, value := range recorderEnv {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("restore %s: %w", key, err)
		}
	}

	dirs := map[string]string{
		"HOME":              filepath.Join(root, "home"),
		"TMPDIR":            filepath.Join(root, "tmp"),
		"XDG_CACHE_HOME":    filepath.Join(root, "xdg", "cache"),
		"XDG_CONFIG_HOME":   filepath.Join(root, "xdg", "config"),
		"XDG_DATA_HOME":     filepath.Join(root, "xdg", "data"),
		"XDG_RUNTIME_DIR":   filepath.Join(root, "xdg", "runtime"),
		"XDG_STATE_HOME":    filepath.Join(root, "xdg", "state"),
		"SENTINEL_DATA_DIR": filepath.Join(root, "sentinel"),
		"TMUX_TMPDIR":       filepath.Join(root, "tmux"),
	}
	for key, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", key, err)
		}
		if err := os.Setenv(key, dir); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}

	for _, key := range []string{
		"DBUS_SESSION_BUS_ADDRESS",
		"EDITOR",
		"GIT_CONFIG_GLOBAL",
		"SSH_AUTH_SOCK",
		"SUDO_USER",
		"TMUX",
		"TMUX_PANE",
		"VISUAL",
	} {
		if err := os.Unsetenv(key); err != nil {
			return fmt.Errorf("unset %s: %w", key, err)
		}
	}

	// Preserve loopback for httptest servers while ensuring an accidental
	// external HTTP request cannot leave the machine.
	for key, value := range map[string]string{
		"ALL_PROXY":   blockedNetworkProxy,
		"HTTP_PROXY":  blockedNetworkProxy,
		"HTTPS_PROXY": blockedNetworkProxy,
		"NO_PROXY":    "127.0.0.1,localhost,::1",
		"all_proxy":   blockedNetworkProxy,
		"http_proxy":  blockedNetworkProxy,
		"https_proxy": blockedNetworkProxy,
		"no_proxy":    "127.0.0.1,localhost,::1",
	} {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}
	http.DefaultTransport = loopbackOnlyTransport{base: hostHTTPTransport}

	commandDir := filepath.Join(root, "blocked-commands")
	if err := os.MkdirAll(commandDir, 0o700); err != nil {
		return fmt.Errorf("create blocked command directory: %w", err)
	}
	for _, name := range blockedCommands {
		path := filepath.Join(commandDir, name)
		script := "#!/bin/sh\n" +
			"printf '%s %s\\n' \"$0\" \"$*\" >>\"$SENTINEL_TEST_HOST_COMMAND_LOG\"\n" +
			fmt.Sprintf("exit %d\n", blockedCommandExitCode)
		if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
			return fmt.Errorf("write %s blocker: %w", name, err)
		}
		// #nosec G302 -- blockers must be executable and remain owner-only.
		if err := os.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("make %s blocker executable: %w", name, err)
		}
	}
	if err := os.Setenv("SENTINEL_TEST_HOST_COMMAND_LOG", commandLog); err != nil {
		return fmt.Errorf("set blocked command log: %w", err)
	}
	// Host binaries are never inherited. Known dangerous commands resolve to
	// blockers above so even an ignored command error fails the package; tests
	// that need a command must prepend a fake executable from their sandbox.
	pathValue := commandDir
	if opts.emptyPath {
		pathValue = filepath.Join(root, "empty-path")
		if err := os.MkdirAll(pathValue, 0o700); err != nil {
			return fmt.Errorf("create empty PATH: %w", err)
		}
	}
	if err := os.Setenv("PATH", pathValue); err != nil {
		return fmt.Errorf("set isolated PATH: %w", err)
	}

	return nil
}
