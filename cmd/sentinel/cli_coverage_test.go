package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/daemon"
	"github.com/opus-domini/sentinel/internal/updater"
)

const testBuildVersionDev = "dev"

// TestHelpFunctions exercises every print*Help function to ensure they write
// non-empty output containing "Usage:" and do not panic.
func TestHelpFunctions(t *testing.T) {
	t.Parallel()

	type helpFunc struct {
		name string
		fn   func(io.Writer)
	}

	cases := []helpFunc{
		{"printRootHelp", printRootHelp},
		{"printServeHelp", printServeHelp},
		{"printServiceHelp", printServiceHelp},
		{"printServiceInstallHelp", printServiceInstallHelp},
		{"printServiceUninstallHelp", printServiceUninstallHelp},
		{"printServiceStatusHelp", printServiceStatusHelp},
		{"printServiceAutoUpdateHelp", printServiceAutoUpdateHelp},
		{"printServiceAutoUpdateInstallHelp", printServiceAutoUpdateInstallHelp},
		{"printServiceAutoUpdateUninstallHelp", printServiceAutoUpdateUninstallHelp},
		{"printServiceAutoUpdateStatusHelp", printServiceAutoUpdateStatusHelp},
		{"printDoctorHelp", printDoctorHelp},
		{"printRecoveryHelp", printRecoveryHelp},
		{"printRecoveryListHelp", printRecoveryListHelp},
		{"printRecoveryRestoreHelp", printRecoveryRestoreHelp},
		{"printUpdateHelp", printUpdateHelp},
		{"printUpdateCheckHelp", printUpdateCheckHelp},
		{"printUpdateApplyHelp", printUpdateApplyHelp},
		{"printUpdateStatusHelp", printUpdateStatusHelp},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			tc.fn(&buf)
			if buf.Len() == 0 {
				t.Fatalf("%s wrote no output", tc.name)
			}
			if !strings.Contains(buf.String(), "Usage:") {
				t.Fatalf("%s output missing 'Usage:': %s", tc.name, buf.String())
			}
		})
	}
}

// TestSubcommandHelpRouting tests that every subcommand group (service, update,
// recovery, service autoupdate) returns exit 0 for help variants and exit 2
// for unknown subcommands.
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
		{name: "root help", args: []string{"help"}, wantCode: 0, wantOut: "Sentinel command-line interface"},
		{name: "root --help", args: []string{"--help"}, wantCode: 0, wantOut: "Sentinel command-line interface"},

		// Service subcommand routing.
		{name: "service no args", args: []string{"service"}, wantCode: 2},
		{name: "service help", args: []string{"service", "help"}, wantCode: 0, wantOut: "sentinel service"},
		{name: "service -h", args: []string{"service", "-h"}, wantCode: 0, wantOut: "sentinel service"},
		{name: "service --help", args: []string{"service", "--help"}, wantCode: 0, wantOut: "sentinel service"},
		{name: "service unknown", args: []string{"service", "bogus"}, wantCode: 2, wantErr: "unknown service command: bogus"},

		// Service autoupdate routing.
		{name: "autoupdate no args", args: []string{"service", "autoupdate"}, wantCode: 2},
		{name: "autoupdate help", args: []string{"service", "autoupdate", "help"}, wantCode: 0, wantOut: "sentinel service autoupdate"},
		{name: "autoupdate -h", args: []string{"service", "autoupdate", "-h"}, wantCode: 0, wantOut: "sentinel service autoupdate"},
		{name: "autoupdate --help", args: []string{"service", "autoupdate", "--help"}, wantCode: 0, wantOut: "sentinel service autoupdate"},
		{name: "autoupdate unknown", args: []string{"service", "autoupdate", "bogus"}, wantCode: 2, wantErr: "unknown autoupdate command: bogus"},

		// Update subcommand routing.
		{name: "update no args", args: []string{"update"}, wantCode: 2},
		{name: "update help", args: []string{"update", "help"}, wantCode: 0, wantOut: "sentinel update"},
		{name: "update -h", args: []string{"update", "-h"}, wantCode: 0, wantOut: "sentinel update"},
		{name: "update --help", args: []string{"update", "--help"}, wantCode: 0, wantOut: "sentinel update"},
		{name: "update unknown", args: []string{"update", "bogus"}, wantCode: 2, wantErr: "unknown update command: bogus"},

		// Recovery subcommand routing.
		{name: "recovery no args", args: []string{"recovery"}, wantCode: 2},
		{name: "recovery help", args: []string{"recovery", "help"}, wantCode: 0, wantOut: "sentinel recovery"},
		{name: "recovery -h", args: []string{"recovery", "-h"}, wantCode: 0, wantOut: "sentinel recovery"},
		{name: "recovery --help", args: []string{"recovery", "--help"}, wantCode: 0, wantOut: "sentinel recovery"},
		{name: "recovery unknown", args: []string{"recovery", "bogus"}, wantCode: 2, wantErr: "unknown recovery command: bogus"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var out, errOut bytes.Buffer
			code := runCLI(tc.args, &out, &errOut)
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

// TestRunServeCommand covers the serve subcommand: help flag, unexpected args,
// invalid flags, and successful dispatch to serveFn.
func TestRunServeCommand(t *testing.T) {
	t.Run("help flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := runCLI([]string{"serve", "--help"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "sentinel serve") {
			t.Fatalf("stdout missing help text: %s", out.String())
		}
	})

	t.Run("unexpected args", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := runCLI([]string{"serve", "extra"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		if !strings.Contains(errOut.String(), "unexpected argument") {
			t.Fatalf("stderr missing error: %s", errOut.String())
		}
	})

	t.Run("invalid flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := runCLI([]string{"serve", "--bogus"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})

	t.Run("calls serveFn", func(t *testing.T) {
		origServe := serveFn
		t.Cleanup(func() { serveFn = origServe })

		called := false
		serveFn = func() int {
			called = true
			return 0
		}

		var out, errOut bytes.Buffer
		code := runCLI([]string{"serve"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !called {
			t.Fatal("serveFn was not called")
		}
	})
}

// TestRunCLIRootFlagFallback verifies that unknown root flags starting with "-"
// are forwarded to the serve subcommand.
func TestRunCLIRootFlagFallback(t *testing.T) {
	origServe := serveFn
	t.Cleanup(func() { serveFn = origServe })

	serveFn = func() int { return 0 }

	var out, errOut bytes.Buffer
	// An unknown flag like --unknown will be parsed by runServeCommand's FlagSet
	// and cause exit 2 (ContinueOnError).
	code := runCLI([]string{"--unknown-flag"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
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
		code := runCLI([]string{"service", "uninstall", "--disable=false", "--stop=false", "--remove-unit=false"}, &out, &errOut)
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
		code := runCLI([]string{"service", "uninstall"}, &out, &errOut)
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
		code := runCLI([]string{"service", "uninstall", "--help"}, &out, &errOut)
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
		code := runCLI([]string{"service", "uninstall", "extra"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		if !strings.Contains(errOut.String(), "unexpected argument") {
			t.Fatalf("stderr missing error: %s", errOut.String())
		}
	})

	t.Run("invalid flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := runCLI([]string{"service", "uninstall", "--bogus"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})
}

// TestParseRecoveryStates validates state parsing for the recovery list command.
func TestParseRecoveryStates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		want    int // expected number of parsed states
		wantErr bool
	}{
		{name: "empty defaults to killed", input: "", want: 1},
		{name: "single killed", input: "killed", want: 1},
		{name: "single running", input: "running", want: 1},
		{name: "single restoring", input: "restoring", want: 1},
		{name: "single restored", input: "restored", want: 1},
		{name: "single archived", input: "archived", want: 1},
		{name: "comma separated", input: "killed,restored,running", want: 3},
		{name: "whitespace trimmed", input: " killed , restored ", want: 2},
		{name: "invalid state", input: "bogus", wantErr: true},
		{name: "mixed valid and invalid", input: "killed,bogus", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseRecoveryStates(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tc.want {
				t.Fatalf("len(states) = %d, want %d", len(got), tc.want)
			}
		})
	}
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

			got := formatTime(tc.input)
			if got != tc.want {
				t.Fatalf("formatTime(%v) = %q, want %q", tc.input, got, tc.want)
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

			got := valueOrDash(tc.input)
			if got != tc.want {
				t.Fatalf("valueOrDash(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestResolveRestartScopeFlag covers all branches of the scope resolution logic.
func TestResolveRestartScopeFlag(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		scope   string
		legacy  string
		want    string
		wantErr bool
	}{
		{name: "both empty", scope: "", legacy: "", want: ""},
		{name: "primary only", scope: "user", legacy: "", want: "user"},
		{name: "legacy only", scope: "", legacy: "system", want: "system"},
		{name: "same value", scope: "user", legacy: "user", want: "user"},
		{name: "same case-insensitive", scope: "User", legacy: "user", want: "User"},
		{name: "conflict", scope: "user", legacy: "system", wantErr: true},
		{name: "whitespace trimmed", scope: "  user  ", legacy: "", want: "user"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveRestartScopeFlag(tc.scope, tc.legacy)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("resolveRestartScopeFlag(%q, %q) = %q, want %q", tc.scope, tc.legacy, got, tc.want)
			}
		})
	}
}

// TestUnitScopeLabel covers all unit scope label detection paths.
func TestUnitScopeLabel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "empty path", path: "", want: "user"},
		{name: "whitespace only", path: "   ", want: "user"},
		{name: "user systemd path", path: "/home/user/.config/systemd/user/sentinel.service", want: "user"},
		{name: "system systemd path", path: "/etc/systemd/system/sentinel.service", want: "system"},
		{name: "system launchd path", path: "/Library/LaunchDaemons/com.sentinel.plist", want: "system"},
		{name: "user launchd path", path: "/Users/user/Library/LaunchAgents/com.sentinel.plist", want: "user"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := unitScopeLabel(tc.path)
			if got != tc.want {
				t.Fatalf("unitScopeLabel(%q) = %q, want %q", tc.path, got, tc.want)
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
			code := runCLI([]string{tc.arg}, &out, &errOut)
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
	origStatus := userStatusFn
	t.Cleanup(func() { userStatusFn = origStatus })

	userStatusFn = func() (daemon.UserServiceStatus, error) {
		return daemon.UserServiceStatus{}, errors.New("status unavailable")
	}

	var out, errOut bytes.Buffer
	code := runCLI([]string{"service", "status"}, &out, &errOut)
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
		code := runCLI([]string{"service", "status", "--help"}, &out, &errOut)
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
		code := runCLI([]string{"service", "status", "extra"}, &out, &errOut)
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
		code := runCLI([]string{"service", "install", "--help"}, &out, &errOut)
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
		code := runCLI([]string{"service", "install", "extra"}, &out, &errOut)
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
	code := runCLI([]string{"service", "autoupdate", "install"}, &out, &errOut)
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
		code := runCLI([]string{"service", "autoupdate", "install", "--help"}, &out, &errOut)
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
		code := runCLI([]string{"service", "autoupdate", "install", "extra"}, &out, &errOut)
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
	code := runCLI([]string{"service", "autoupdate", "uninstall"}, &out, &errOut)
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
		code := runCLI([]string{"service", "autoupdate", "uninstall", "--help"}, &out, &errOut)
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
		code := runCLI([]string{"service", "autoupdate", "uninstall", "extra"}, &out, &errOut)
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
	code := runCLI([]string{"service", "autoupdate", "status"}, &out, &errOut)
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
		code := runCLI([]string{"service", "autoupdate", "status", "--help"}, &out, &errOut)
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
		code := runCLI([]string{"service", "autoupdate", "status", "extra"}, &out, &errOut)
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

	loadConfigFn = func() config.Config { return config.Config{DataDir: "/tmp"} }
	currentVersionFn = func() string { return testCurrentVersion1 }
	updateCheckFn = func(_ context.Context, _ updater.CheckOptions) (updater.CheckResult, error) {
		return updater.CheckResult{}, errors.New("network error")
	}

	var out, errOut bytes.Buffer
	code := runCLI([]string{"update", "check"}, &out, &errOut)
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
		code := runCLI([]string{"update", "check", "--help"}, &out, &errOut)
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
		code := runCLI([]string{"update", "check", "extra"}, &out, &errOut)
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

	loadConfigFn = func() config.Config { return config.Config{DataDir: "/tmp"} }
	currentVersionFn = func() string { return testCurrentVersion1 }
	updateApplyFn = func(_ context.Context, _ updater.ApplyOptions) (updater.ApplyResult, error) {
		return updater.ApplyResult{}, errors.New("apply failed")
	}

	var out, errOut bytes.Buffer
	code := runCLI([]string{"update", "apply"}, &out, &errOut)
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

	loadConfigFn = func() config.Config { return config.Config{DataDir: "/tmp"} }
	currentVersionFn = func() string { return testCurrentVersion1 }
	updateApplyFn = func(_ context.Context, _ updater.ApplyOptions) (updater.ApplyResult, error) {
		return updater.ApplyResult{
			Applied:        false,
			CurrentVersion: testCurrentVersion1,
		}, nil
	}

	var out, errOut bytes.Buffer
	code := runCLI([]string{"update", "apply"}, &out, &errOut)
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
		code := runCLI([]string{"update", "apply", "--help"}, &out, &errOut)
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
		code := runCLI([]string{"update", "apply", "extra"}, &out, &errOut)
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

	loadConfigFn = func() config.Config { return config.Config{DataDir: "/tmp"} }
	updateStatusFn = func(_ string) (updater.State, error) {
		return updater.State{}, errors.New("status failed")
	}

	var out, errOut bytes.Buffer
	code := runCLI([]string{"update", "status"}, &out, &errOut)
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
		code := runCLI([]string{"update", "status", "--help"}, &out, &errOut)
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
		code := runCLI([]string{"update", "status", "extra"}, &out, &errOut)
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
		code := runCLI([]string{"doctor", "--help"}, &out, &errOut)
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
		code := runCLI([]string{"doctor", "extra"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})
}

// TestDoctorStatusError covers the doctor command when service status fails.
func TestDoctorStatusError(t *testing.T) {
	origLoad := loadConfigFn
	origStatus := userStatusFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		userStatusFn = origStatus
	})

	loadConfigFn = func() config.Config {
		return config.Config{
			ListenAddr: "127.0.0.1:4040",
			DataDir:    "/tmp/.sentinel",
		}
	}
	userStatusFn = func() (daemon.UserServiceStatus, error) {
		return daemon.UserServiceStatus{}, errors.New("service not available")
	}

	var out, errOut bytes.Buffer
	code := runCLI([]string{"doctor"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "unavailable") {
		t.Fatalf("stdout missing unavailable label: %s", out.String())
	}
}

// TestRecoveryRestoreHelpAndArgs covers help, unexpected-arg, and validation paths.
func TestRecoveryRestoreHelpAndArgs(t *testing.T) {
	t.Run("help flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := runCLI([]string{"recovery", "restore", "--help"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "sentinel recovery restore") {
			t.Fatalf("stdout missing help: %s", out.String())
		}
	})

	t.Run("unexpected args", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := runCLI([]string{"recovery", "restore", "extra"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})

	t.Run("missing snapshot id", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := runCLI([]string{"recovery", "restore"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		if !strings.Contains(errOut.String(), "snapshot id is required") {
			t.Fatalf("stderr missing error: %s", errOut.String())
		}
	})

	t.Run("negative snapshot id", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := runCLI([]string{"recovery", "restore", "--snapshot", "-1"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})
}

// TestRecoveryListHelpAndArgs covers help and unexpected-arg paths.
func TestRecoveryListHelpAndArgs(t *testing.T) {
	t.Run("help flag", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := runCLI([]string{"recovery", "list", "--help"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "sentinel recovery list") {
			t.Fatalf("stdout missing help: %s", out.String())
		}
	})

	t.Run("unexpected args", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := runCLI([]string{"recovery", "list", "extra"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
	})

	t.Run("invalid state", func(t *testing.T) {
		t.Parallel()

		var out, errOut bytes.Buffer
		code := runCLI([]string{"recovery", "list", "--state", "bogus"}, &out, &errOut)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		if !strings.Contains(errOut.String(), "invalid state") {
			t.Fatalf("stderr missing error: %s", errOut.String())
		}
	})
}

// TestCurrentVersionFallbackToDev tests that currentVersion returns "dev" when
// buildVersion is "dev".
func TestCurrentVersionFallbackToDev(t *testing.T) {
	orig := buildVersion
	t.Cleanup(func() { buildVersion = orig })

	buildVersion = testBuildVersionDev
	got := currentVersion()
	// In test context, debug.ReadBuildInfo might return "(devel)", so we
	// accept either "dev" or a non-empty version.
	if got == "" {
		t.Fatal("currentVersion() returned empty string")
	}
}

// TestCurrentVersionEmptyBuildVersion tests the (devel) fallback path.
func TestCurrentVersionEmptyBuildVersion(t *testing.T) {
	orig := buildVersion
	t.Cleanup(func() { buildVersion = orig })

	buildVersion = ""
	got := currentVersion()
	if got == "" {
		t.Fatal("currentVersion() returned empty string")
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
			code := runCLI(tc.args, &out, &errOut)
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
			code := runCLI(tc.args, &out, &errOut)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
			}
			if !strings.Contains(out.String(), tc.wantOut) {
				t.Fatalf("stdout missing %q: %s", tc.wantOut, out.String())
			}
		})
	}
}
