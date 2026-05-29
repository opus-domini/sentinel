package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/daemon"
	"github.com/opus-domini/sentinel/internal/humanize"
	"github.com/opus-domini/sentinel/internal/updater"
)

const testBuildVersionDev = "dev"

// TestSubcommandHelpRouting tests that every subcommand group (service, update,
// service autoupdate) returns exit 0 for help variants and for no arguments,
// and exit 2 for unknown subcommands.
func TestSubcommandHelpRouting(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string // fragment expected in stdout
		wantErr  string // fragment expected in stderr
	}{
		// Root help variants.
		{name: "root help", args: []string{"help"}, wantCode: 0, wantOut: "SERVICE COMMANDS"},
		{name: "root --help", args: []string{"--help"}, wantCode: 0, wantOut: "SERVICE COMMANDS"},

		// Service subcommand routing.
		{name: "service no args", args: []string{"service"}, wantCode: 0, wantOut: "sentinel service"},
		{name: "service -h", args: []string{"service", "-h"}, wantCode: 0, wantOut: "sentinel service"},
		{name: "service --help", args: []string{"service", "--help"}, wantCode: 0, wantOut: "sentinel service"},
		{name: "service unknown", args: []string{"service", "bogus"}, wantCode: 2, wantErr: "unknown command"},

		// Service autoupdate routing.
		{name: "autoupdate no args", args: []string{"service", "autoupdate"}, wantCode: 0, wantOut: "sentinel service autoupdate"},
		{name: "autoupdate -h", args: []string{"service", "autoupdate", "-h"}, wantCode: 0, wantOut: "sentinel service autoupdate"},
		{name: "autoupdate --help", args: []string{"service", "autoupdate", "--help"}, wantCode: 0, wantOut: "sentinel service autoupdate"},
		{name: "autoupdate unknown", args: []string{"service", "autoupdate", "bogus"}, wantCode: 2, wantErr: "unknown command"},

		// Update subcommand routing.
		{name: "update no args", args: []string{"update"}, wantCode: 0, wantOut: "sentinel update"},
		{name: "update -h", args: []string{"update", "-h"}, wantCode: 0, wantOut: "sentinel update"},
		{name: "update --help", args: []string{"update", "--help"}, wantCode: 0, wantOut: "sentinel update"},
		{name: "update unknown", args: []string{"update", "bogus"}, wantCode: 2, wantErr: "unknown command"},

		// Config subcommand routing.
		{name: "config no args", args: []string{"config"}, wantCode: 0, wantOut: "sentinel config"},
		{name: "config -h", args: []string{"config", "-h"}, wantCode: 0, wantOut: "sentinel config"},
		{name: "config --help", args: []string{"config", "--help"}, wantCode: 0, wantOut: "sentinel config"},
		{name: "config unknown", args: []string{"config", "bogus"}, wantCode: 2, wantErr: "unknown command"},

		// DB subcommand routing.
		{name: "db no args", args: []string{"db"}, wantCode: 0, wantOut: "sentinel db"},
		{name: "db -h", args: []string{"db", "-h"}, wantCode: 0, wantOut: "sentinel db"},
		{name: "db --help", args: []string{"db", "--help"}, wantCode: 0, wantOut: "sentinel db"},
		{name: "db unknown", args: []string{"db", "bogus"}, wantCode: 2, wantErr: "unknown command"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var out, errOut bytes.Buffer
			code := Run(tc.args, &out, &errOut)
			if code != tc.wantCode {
				t.Fatalf("exit code = %d, want %d (stdout=%q stderr=%q)", code, tc.wantCode, out.String(), errOut.String())
			}
			if tc.wantOut != "" && !strings.Contains(out.String(), tc.wantOut) {
				t.Fatalf("stdout missing %q: %s", tc.wantOut, out.String())
			}
			if tc.wantErr != "" && !strings.Contains(errOut.String(), tc.wantErr) {
				t.Fatalf("stderr missing %q: %s", tc.wantErr, errOut.String())
			}
		})
	}
}

// TestRunDaemonCommand covers the daemon subcommand: help flag, unexpected
// args, invalid flags, and successful dispatch to daemonFn.
func TestRunDaemonCommand(t *testing.T) {
	t.Run("help flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"daemon", "--help"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "sentinel daemon") {
			t.Fatalf("stdout missing help text: %s", out.String())
		}
	})

	t.Run("unexpected args", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"daemon", "extra"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		if !strings.Contains(errOut.String(), "unknown command") {
			t.Fatalf("stderr missing error: %s", errOut.String())
		}
	})

	t.Run("invalid flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"daemon", "--bogus"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})

	t.Run("calls daemonFn", func(t *testing.T) {
		origDaemon := daemonFn
		t.Cleanup(func() { daemonFn = origDaemon })

		called := false
		daemonFn = func() int {
			called = true
			return 0
		}

		var out, errOut bytes.Buffer
		code := Run([]string{"daemon"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !called {
			t.Fatal("daemonFn was not called")
		}
	})
}

// TestRunRejectsUnknownRootFlag verifies that an unknown root flag is treated
// as an unknown command and never starts the daemon.
func TestRunRejectsUnknownRootFlag(t *testing.T) {
	origDaemon := daemonFn
	t.Cleanup(func() { daemonFn = origDaemon })

	called := false
	daemonFn = func() int {
		called = true
		return 0
	}

	var out, errOut bytes.Buffer
	code := Run([]string{"--unknown-flag"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if called {
		t.Fatal("daemonFn must not run for an unknown root flag")
	}
	if !strings.Contains(errOut.String(), "unknown flag") {
		t.Fatalf("stderr missing unknown-flag error: %s", errOut.String())
	}
}

// TestRunServiceUninstallCommand covers success, failure, help and unexpected arg paths.
func TestRunServiceUninstallCommand(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		origUninstall := uninstallUserSvcFn
		t.Cleanup(func() { uninstallUserSvcFn = origUninstall })

		var got daemon.UninstallUserOptions
		uninstallUserSvcFn = func(opts daemon.UninstallUserOptions) error {
			got = opts
			return nil
		}

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "uninstall", "--disable=false", "--stop=false", "--remove-unit=false"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
		}
		if got.Disable {
			t.Fatal("Disable = true, want false")
		}
		if got.Stop {
			t.Fatal("Stop = true, want false")
		}
		if got.RemoveUnit {
			t.Fatal("RemoveUnit = true, want false")
		}
		if !strings.Contains(out.String(), "service uninstalled") {
			t.Fatalf("stdout missing success message: %s", out.String())
		}
	})

	t.Run("failure", func(t *testing.T) {
		origUninstall := uninstallUserSvcFn
		t.Cleanup(func() { uninstallUserSvcFn = origUninstall })

		uninstallUserSvcFn = func(_ daemon.UninstallUserOptions) error {
			return errors.New("uninstall failed")
		}

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "uninstall"}, &out, &errOut)
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if !strings.Contains(errOut.String(), "service uninstall failed") {
			t.Fatalf("stderr missing error: %s", errOut.String())
		}
	})

	t.Run("help flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "uninstall", "--help"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "sentinel service uninstall") {
			t.Fatalf("stdout missing help text: %s", out.String())
		}
	})

	t.Run("unexpected args", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "uninstall", "extra"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		if !strings.Contains(errOut.String(), "unknown command") {
			t.Fatalf("stderr missing error: %s", errOut.String())
		}
	})

	t.Run("invalid flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "uninstall", "--bogus"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})

	t.Run("purge removes autoupdate, completion and binary", func(t *testing.T) {
		origUninstall := uninstallUserSvcFn
		origAutoUpdate := uninstallUserAutoUpdateFn
		origCompletion := removeShellCompletionsFn
		origBinary := removeSentinelBinaryFn
		t.Cleanup(func() {
			uninstallUserSvcFn = origUninstall
			uninstallUserAutoUpdateFn = origAutoUpdate
			removeShellCompletionsFn = origCompletion
			removeSentinelBinaryFn = origBinary
		})

		var autoUpdateCalled, completionCalled, binaryCalled bool
		uninstallUserSvcFn = func(daemon.UninstallUserOptions) error { return nil }
		uninstallUserAutoUpdateFn = func(daemon.UninstallUserAutoUpdateOptions) error {
			autoUpdateCalled = true
			return nil
		}
		removeShellCompletionsFn = func() []string {
			completionCalled = true
			return []string{"/tmp/shell-completion/sentinel"}
		}
		removeSentinelBinaryFn = func() (string, error) {
			binaryCalled = true
			return "/tmp/bin/sentinel", nil
		}

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "uninstall", "--purge"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
		}
		if !autoUpdateCalled || !completionCalled || !binaryCalled {
			t.Fatalf("purge skipped a step: autoupdate=%t completion=%t binary=%t",
				autoUpdateCalled, completionCalled, binaryCalled)
		}
		stdout := out.String()
		if !strings.Contains(stdout, "autoupdate timer removed") ||
			!strings.Contains(stdout, "/tmp/bin/sentinel") {
			t.Fatalf("stdout missing purge output: %s", stdout)
		}
	})

	t.Run("default run does not purge", func(t *testing.T) {
		origUninstall := uninstallUserSvcFn
		origBinary := removeSentinelBinaryFn
		t.Cleanup(func() {
			uninstallUserSvcFn = origUninstall
			removeSentinelBinaryFn = origBinary
		})

		uninstallUserSvcFn = func(daemon.UninstallUserOptions) error { return nil }
		binaryCalled := false
		removeSentinelBinaryFn = func() (string, error) {
			binaryCalled = true
			return "", nil
		}

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "uninstall"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if binaryCalled {
			t.Fatal("plain uninstall removed the binary; --purge must be required")
		}
	})
}

// TestRunServiceLogsCommand covers success, failure, help, and arg-parsing paths.
func TestRunServiceLogsCommand(t *testing.T) {
	t.Run("success forwards parsed flags", func(t *testing.T) {
		origLogs := userLogsFn
		t.Cleanup(func() { userLogsFn = origLogs })

		var got daemon.LogsOptions
		userLogsFn = func(opts daemon.LogsOptions) error {
			got = opts
			return nil
		}

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "logs", "-f", "-n", "120"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
		}
		if !got.Follow {
			t.Fatal("Follow = false, want true")
		}
		if got.Lines != 120 {
			t.Fatalf("Lines = %d, want 120", got.Lines)
		}
		if got.Stdout != &out || got.Stderr != &errOut {
			t.Fatal("LogsOptions writers not wired to command context")
		}
	})

	t.Run("default flags", func(t *testing.T) {
		origLogs := userLogsFn
		t.Cleanup(func() { userLogsFn = origLogs })

		var got daemon.LogsOptions
		userLogsFn = func(opts daemon.LogsOptions) error {
			got = opts
			return nil
		}

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "logs"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if got.Follow {
			t.Fatal("Follow = true, want false")
		}
		if got.Lines != 50 {
			t.Fatalf("Lines = %d, want 50", got.Lines)
		}
	})

	t.Run("failure", func(t *testing.T) {
		origLogs := userLogsFn
		t.Cleanup(func() { userLogsFn = origLogs })

		userLogsFn = func(_ daemon.LogsOptions) error {
			return errors.New("journalctl was not found in PATH")
		}

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "logs"}, &out, &errOut)
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if !strings.Contains(errOut.String(), "service logs failed") {
			t.Fatalf("stderr missing error: %s", errOut.String())
		}
	})

	t.Run("help flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "logs", "--help"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "sentinel service logs") {
			t.Fatalf("stdout missing help text: %s", out.String())
		}
	})

	t.Run("unexpected args", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "logs", "extra"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		if !strings.Contains(errOut.String(), "unknown command") {
			t.Fatalf("stderr missing error: %s", errOut.String())
		}
	})

	t.Run("invalid flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "logs", "--bogus"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})
}

// TestFormatTime tests both zero and non-zero time formatting.
func TestFormatTime(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input time.Time
		want  string
	}{
		{name: "zero time", input: time.Time{}, want: "-"},
		{name: "non-zero", input: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC), want: "2026-01-15T10:30:00Z"},
		{name: "non-UTC converted", input: time.Date(2026, 1, 15, 10, 30, 0, 0, time.FixedZone("EST", -5*3600)), want: "2026-01-15T15:30:00Z"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := humanize.Time(tc.input)
			if got != tc.want {
				t.Fatalf("humanize.Time(%v) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestValueOrDash tests dash replacement for empty values.
func TestValueOrDash(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: "-"},
		{name: "whitespace only", input: "   ", want: "-"},
		{name: "non-empty", input: "hello", want: "hello"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := humanize.ValueOrDash(tc.input)
			if got != tc.want {
				t.Fatalf("humanize.ValueOrDash(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestVersionCommandAliases tests all three version flag variants (-v, --version, version).
func TestVersionCommandAliases(t *testing.T) {
	origVersion := currentVersionFn
	t.Cleanup(func() { currentVersionFn = origVersion })
	currentVersionFn = func() string { return "v2.0.0" }

	cases := []struct {
		name string
		arg  string
	}{
		{name: "-v", arg: "-v"},
		{name: "--version", arg: "--version"},
		{name: "version", arg: "version"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := Run([]string{tc.arg}, &out, &errOut)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0", code)
			}
			if !strings.Contains(out.String(), "sentinel version v2.0.0") {
				t.Fatalf("unexpected output: %q", out.String())
			}
		})
	}
}

// TestServiceStatusFailure covers the error path for service status.
func TestServiceStatusFailure(t *testing.T) {
	origStatus := serviceStatusFn
	t.Cleanup(func() { serviceStatusFn = origStatus })

	serviceStatusFn = func() ([]daemon.ScopedServiceStatus, error) {
		return nil, errors.New("status unavailable")
	}

	var out, errOut bytes.Buffer
	code := Run([]string{"service", "status"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "service status failed") {
		t.Fatalf("stderr missing error: %s", errOut.String())
	}
}

// TestServiceStatusHelpAndArgs covers help and unexpected-arg paths.
func TestServiceStatusHelpAndArgs(t *testing.T) {
	t.Run("help flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "status", "--help"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "sentinel service status") {
			t.Fatalf("stdout missing help: %s", out.String())
		}
	})

	t.Run("unexpected args", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "status", "extra"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})
}

// TestServiceInstallHelpAndArgs covers help and unexpected-arg paths for install.
func TestServiceInstallHelpAndArgs(t *testing.T) {
	t.Run("help flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "install", "--help"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "sentinel service install") {
			t.Fatalf("stdout missing help: %s", out.String())
		}
	})

	t.Run("unexpected args", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "install", "extra"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})
}

// TestServiceAutoUpdateInstallFailure covers the error path.
func TestServiceAutoUpdateInstallFailure(t *testing.T) {
	origInstall := installUserAutoUpdateFn
	t.Cleanup(func() { installUserAutoUpdateFn = origInstall })

	installUserAutoUpdateFn = func(_ daemon.InstallUserAutoUpdateOptions) error {
		return errors.New("install failed")
	}

	var out, errOut bytes.Buffer
	code := Run([]string{"service", "autoupdate", "install"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "service autoupdate install failed") {
		t.Fatalf("stderr missing error: %s", errOut.String())
	}
}

// TestServiceAutoUpdateInstallHelpAndArgs covers help and unexpected-arg paths.
func TestServiceAutoUpdateInstallHelpAndArgs(t *testing.T) {
	t.Run("help flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "autoupdate", "install", "--help"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "sentinel service autoupdate install") {
			t.Fatalf("stdout missing help: %s", out.String())
		}
	})

	t.Run("unexpected args", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "autoupdate", "install", "extra"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})
}

// TestServiceAutoUpdateUninstallFailure covers the error path.
func TestServiceAutoUpdateUninstallFailure(t *testing.T) {
	origUninstall := uninstallUserAutoUpdateFn
	t.Cleanup(func() { uninstallUserAutoUpdateFn = origUninstall })

	uninstallUserAutoUpdateFn = func(_ daemon.UninstallUserAutoUpdateOptions) error {
		return errors.New("uninstall failed")
	}

	var out, errOut bytes.Buffer
	code := Run([]string{"service", "autoupdate", "uninstall"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "service autoupdate uninstall failed") {
		t.Fatalf("stderr missing error: %s", errOut.String())
	}
}

// TestServiceAutoUpdateUninstallHelpAndArgs covers help and unexpected-arg paths.
func TestServiceAutoUpdateUninstallHelpAndArgs(t *testing.T) {
	t.Run("help flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "autoupdate", "uninstall", "--help"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "sentinel service autoupdate uninstall") {
			t.Fatalf("stdout missing help: %s", out.String())
		}
	})

	t.Run("unexpected args", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "autoupdate", "uninstall", "extra"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})
}

// TestServiceAutoUpdateStatusFailure covers the error path.
func TestServiceAutoUpdateStatusFailure(t *testing.T) {
	origStatus := userAutoUpdateStatusFn
	t.Cleanup(func() { userAutoUpdateStatusFn = origStatus })

	userAutoUpdateStatusFn = func(_ string) (daemon.UserAutoUpdateServiceStatus, error) {
		return daemon.UserAutoUpdateServiceStatus{}, errors.New("status failed")
	}

	var out, errOut bytes.Buffer
	code := Run([]string{"service", "autoupdate", "status"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "service autoupdate status failed") {
		t.Fatalf("stderr missing error: %s", errOut.String())
	}
}

// TestServiceAutoUpdateStatusHelpAndArgs covers help and unexpected-arg paths.
func TestServiceAutoUpdateStatusHelpAndArgs(t *testing.T) {
	t.Run("help flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "autoupdate", "status", "--help"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "sentinel service autoupdate status") {
			t.Fatalf("stdout missing help: %s", out.String())
		}
	})

	t.Run("unexpected args", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"service", "autoupdate", "status", "extra"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})
}

// TestUpdateCheckFailure covers the error path for update check.
func TestUpdateCheckFailure(t *testing.T) {
	origLoad := loadConfigFn
	origVersion := currentVersionFn
	origCheck := updateCheckFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		currentVersionFn = origVersion
		updateCheckFn = origCheck
	})

	loadConfigFn = testLoadConfig("/tmp", "")
	currentVersionFn = func() string { return testCurrentVersion1 }
	updateCheckFn = func(_ context.Context, _ updater.CheckOptions) (updater.CheckResult, error) {
		return updater.CheckResult{}, errors.New("network error")
	}

	var out, errOut bytes.Buffer
	code := Run([]string{"update", "check"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "update check failed") {
		t.Fatalf("stderr missing error: %s", errOut.String())
	}
}

// TestUpdateCheckHelpAndArgs covers help and unexpected-arg paths.
func TestUpdateCheckHelpAndArgs(t *testing.T) {
	t.Run("help flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"update", "check", "--help"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "sentinel update check") {
			t.Fatalf("stdout missing help: %s", out.String())
		}
	})

	t.Run("unexpected args", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"update", "check", "extra"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})
}

// TestUpdateApplyFailure covers the error path for update apply.
func TestUpdateApplyFailure(t *testing.T) {
	origLoad := loadConfigFn
	origVersion := currentVersionFn
	origApply := updateApplyFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		currentVersionFn = origVersion
		updateApplyFn = origApply
	})

	loadConfigFn = testLoadConfig("/tmp", "")
	currentVersionFn = func() string { return testCurrentVersion1 }
	updateApplyFn = func(_ context.Context, _ updater.ApplyOptions) (updater.ApplyResult, error) {
		return updater.ApplyResult{}, errors.New("apply failed")
	}

	var out, errOut bytes.Buffer
	code := Run([]string{"update", "apply"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "update apply failed") {
		t.Fatalf("stderr missing error: %s", errOut.String())
	}
}

// TestUpdateApplyAlreadyUpToDate covers the not-applied path.
func TestUpdateApplyAlreadyUpToDate(t *testing.T) {
	origLoad := loadConfigFn
	origVersion := currentVersionFn
	origApply := updateApplyFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		currentVersionFn = origVersion
		updateApplyFn = origApply
	})

	loadConfigFn = testLoadConfig("/tmp", "")
	currentVersionFn = func() string { return testCurrentVersion1 }
	updateApplyFn = func(_ context.Context, _ updater.ApplyOptions) (updater.ApplyResult, error) {
		return updater.ApplyResult{
			Applied:        false,
			CurrentVersion: testCurrentVersion1,
		}, nil
	}

	var out, errOut bytes.Buffer
	code := Run([]string{"update", "apply"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "already up to date") {
		t.Fatalf("stdout missing 'already up to date': %s", out.String())
	}
}

// TestUpdateApplyHelpAndArgs covers help and unexpected-arg paths.
func TestUpdateApplyHelpAndArgs(t *testing.T) {
	t.Run("help flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"update", "apply", "--help"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "sentinel update apply") {
			t.Fatalf("stdout missing help: %s", out.String())
		}
	})

	t.Run("unexpected args", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"update", "apply", "extra"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})
}

// TestUpdateStatusFailure covers the error path for update status.
func TestUpdateStatusFailure(t *testing.T) {
	origLoad := loadConfigFn
	origStatus := updateStatusFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		updateStatusFn = origStatus
	})

	loadConfigFn = testLoadConfig("/tmp", "")
	updateStatusFn = func(_ string) (updater.State, error) {
		return updater.State{}, errors.New("status failed")
	}

	var out, errOut bytes.Buffer
	code := Run([]string{"update", "status"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "update status failed") {
		t.Fatalf("stderr missing error: %s", errOut.String())
	}
}

// TestUpdateStatusHelpAndArgs covers help and unexpected-arg paths.
func TestUpdateStatusHelpAndArgs(t *testing.T) {
	t.Run("help flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"update", "status", "--help"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "sentinel update status") {
			t.Fatalf("stdout missing help: %s", out.String())
		}
	})

	t.Run("unexpected args", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"update", "status", "extra"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})
}

// TestDoctorHelpAndArgs covers help, unexpected-arg, and invalid-flag paths.
func TestDoctorHelpAndArgs(t *testing.T) {
	t.Run("help flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"doctor", "--help"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "sentinel doctor") {
			t.Fatalf("stdout missing help: %s", out.String())
		}
	})

	t.Run("unexpected args", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := Run([]string{"doctor", "extra"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})
}

// TestDoctorStatusError covers the doctor command when service status fails.
func TestDoctorStatusError(t *testing.T) {
	origLoad := loadConfigFn
	origStatus := serviceStatusFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		serviceStatusFn = origStatus
	})

	loadConfigFn = testLoadConfig("/tmp/.sentinel", "")
	serviceStatusFn = func() ([]daemon.ScopedServiceStatus, error) {
		return nil, errors.New("service not available")
	}

	var out, errOut bytes.Buffer
	code := Run([]string{"doctor"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "unavailable") {
		t.Fatalf("stdout missing unavailable label: %s", out.String())
	}
}

// TestVersionFallbackToDev tests that Version returns a non-empty value when
// version is "dev".
func TestVersionFallbackToDev(t *testing.T) {
	orig := version
	t.Cleanup(func() { version = orig })

	version = testBuildVersionDev
	got := Version()
	// In test context, debug.ReadBuildInfo might return "(devel)", so we
	// accept either "dev" or a non-empty version.
	if got == "" {
		t.Fatal("Version() returned empty string")
	}
}

// TestVersionEmptyBuildVersion tests the (devel) fallback path.
func TestVersionEmptyBuildVersion(t *testing.T) {
	orig := version
	t.Cleanup(func() { version = orig })

	version = ""
	got := Version()
	if got == "" {
		t.Fatal("Version() returned empty string")
	}
}

// TestServiceInstallEnableStartCombinations covers the output messages for
// different enable/start flag combinations.
func TestServiceInstallEnableStartCombinations(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantOut string
	}{
		{
			name:    "enable and start",
			args:    []string{"service", "install", "--enable=true", "--start=true"},
			wantOut: "service enabled and started",
		},
		{
			name:    "enable only",
			args:    []string{"service", "install", "--enable=true", "--start=false"},
			wantOut: "service enabled",
		},
		{
			name:    "start only",
			args:    []string{"service", "install", "--enable=false", "--start=true"},
			wantOut: "service started",
		},
		{
			name:    "neither",
			args:    []string{"service", "install", "--enable=false", "--start=false"},
			wantOut: "service installed (not enabled, not started)",
		},
	}

	origInstall := installUserSvcFn
	t.Cleanup(func() { installUserSvcFn = origInstall })
	installUserSvcFn = func(_ daemon.InstallUserOptions) error { return nil }

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := Run(tc.args, &out, &errOut)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
			}
			if !strings.Contains(out.String(), tc.wantOut) {
				t.Fatalf("stdout missing %q: %s", tc.wantOut, out.String())
			}
		})
	}
}

// TestServiceAutoUpdateInstallEnableStartCombinations covers the output messages
// for different enable/start flag combinations on autoupdate install.
func TestServiceAutoUpdateInstallEnableStartCombinations(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantOut string
	}{
		{
			name:    "enable and start",
			args:    []string{"service", "autoupdate", "install", "--enable=true", "--start=true"},
			wantOut: "autoupdate timer enabled and started",
		},
		{
			name:    "enable only",
			args:    []string{"service", "autoupdate", "install", "--enable=true", "--start=false"},
			wantOut: "autoupdate timer enabled",
		},
		{
			name:    "start only",
			args:    []string{"service", "autoupdate", "install", "--enable=false", "--start=true"},
			wantOut: "autoupdate timer started",
		},
		{
			name:    "neither",
			args:    []string{"service", "autoupdate", "install", "--enable=false", "--start=false"},
			wantOut: "autoupdate timer installed (not enabled, not started)",
		},
	}

	origInstall := installUserAutoUpdateFn
	t.Cleanup(func() { installUserAutoUpdateFn = origInstall })
	installUserAutoUpdateFn = func(_ daemon.InstallUserAutoUpdateOptions) error { return nil }

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := Run(tc.args, &out, &errOut)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
			}
			if !strings.Contains(out.String(), tc.wantOut) {
				t.Fatalf("stdout missing %q: %s", tc.wantOut, out.String())
			}
		})
	}
}
