package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/recovery"
	"github.com/opus-domini/sentinel/internal/service"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/opus-domini/sentinel/internal/tmux"
	"github.com/opus-domini/sentinel/internal/updater"
)

var (
	serveFn                   = serve
	installUserSvcFn          = service.InstallUser
	uninstallUserSvcFn        = service.UninstallUser
	userStatusFn              = service.UserStatus
	installUserAutoUpdateFn   = service.InstallUserAutoUpdate
	uninstallUserAutoUpdateFn = service.UninstallUserAutoUpdate
	userAutoUpdateStatusFn    = service.UserAutoUpdateStatusForScope
	loadConfigFn              = config.Load
	currentVersionFn          = currentVersion
	updateCheckFn             = updater.Check
	updateApplyFn             = updater.Apply
	updateStatusFn            = updater.Status
)

// buildVersion is injected by release workflows via -ldflags.
var buildVersion = "dev"

const (
	defaultUpdaterRepo = "opus-domini/sentinel"
)

const (
	cmdHelp       = "help"
	cmdStatus     = "status"
	flagHelpShort = "-h"
	flagHelpLong  = "--help"
)

func writef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func writeln(w io.Writer, args ...any) {
	_, _ = fmt.Fprintln(w, args...)
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	ctx := commandContext{stdout: stdout, stderr: stderr}

	if len(args) == 0 {
		return serveFn()
	}

	switch args[0] {
	case "-v", "--version", "version":
		writef(stdout, "sentinel version %s\n", currentVersionFn())
		return 0
	case "serve":
		return runServeCommand(ctx, args[1:])
	case "service":
		return runServiceCommand(ctx, args[1:])
	case "doctor":
		return runDoctorCommand(ctx, args[1:])
	case "recovery":
		return runRecoveryCommand(ctx, args[1:])
	case "update":
		return runUpdateCommand(ctx, args[1:])
	case cmdHelp, flagHelpShort, flagHelpLong:
		printRootHelp(stdout)
		return 0
	default:
		// Preserve backward compatibility for future root flags.
		if strings.HasPrefix(args[0], "-") {
			return runServeCommand(ctx, args)
		}
		writef(stderr, "unknown command: %s\n\n", args[0])
		printRootHelp(stderr)
		return 2
	}
}

func runServeCommand(ctx commandContext, args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	help := fs.Bool("help", false, "show help")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *help {
		printServeHelp(ctx.stdout)
		return 0
	}
	if fs.NArg() > 0 {
		writef(ctx.stderr, "unexpected argument(s): %s\n", strings.Join(fs.Args(), " "))
		printServeHelp(ctx.stderr)
		return 2
	}
	return serveFn()
}

func runServiceCommand(ctx commandContext, args []string) int {
	if len(args) == 0 {
		printServiceHelp(ctx.stderr)
		return 2
	}

	switch args[0] {
	case "install":
		return runServiceInstallCommand(ctx, args[1:])
	case "uninstall":
		return runServiceUninstallCommand(ctx, args[1:])
	case cmdStatus:
		return runServiceStatusCommand(ctx, args[1:])
	case "autoupdate":
		return runServiceAutoUpdateCommand(ctx, args[1:])
	case cmdHelp, flagHelpShort, flagHelpLong:
		printServiceHelp(ctx.stdout)
		return 0
	default:
		writef(ctx.stderr, "unknown service command: %s\n\n", args[0])
		printServiceHelp(ctx.stderr)
		return 2
	}
}

func runServiceInstallCommand(ctx commandContext, args []string) int {
	fs := flag.NewFlagSet("service install", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	execPath := fs.String("exec", "", "path to sentinel binary for ExecStart (defaults to current executable)")
	enable := fs.Bool("enable", true, "enable service at startup")
	start := fs.Bool("start", true, "start service now")
	help := fs.Bool("help", false, "show help")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *help {
		printServiceInstallHelp(ctx.stdout)
		return 0
	}
	if fs.NArg() > 0 {
		writef(ctx.stderr, "unexpected argument(s): %s\n", strings.Join(fs.Args(), " "))
		printServiceInstallHelp(ctx.stderr)
		return 2
	}

	err := installUserSvcFn(service.InstallUserOptions{
		ExecPath: strings.TrimSpace(*execPath),
		Enable:   *enable,
		Start:    *start,
	})
	if err != nil {
		writef(ctx.stderr, "service install failed: %v\n", err)
		return 1
	}

	path, pathErr := service.UserServicePath()
	if pathErr == nil {
		writef(ctx.stdout, "service installed: %s\n", path)
	}
	switch {
	case *enable && *start:
		writeln(ctx.stdout, "service enabled and started")
	case *enable:
		writeln(ctx.stdout, "service enabled")
	case *start:
		writeln(ctx.stdout, "service started")
	default:
		writeln(ctx.stdout, "service installed (not enabled, not started)")
	}
	return 0
}

func runServiceUninstallCommand(ctx commandContext, args []string) int {
	fs := flag.NewFlagSet("service uninstall", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	disable := fs.Bool("disable", true, "disable service from auto-start")
	stop := fs.Bool("stop", true, "stop running service")
	removeUnit := fs.Bool("remove-unit", true, "remove managed unit file")
	help := fs.Bool("help", false, "show help")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *help {
		printServiceUninstallHelp(ctx.stdout)
		return 0
	}
	if fs.NArg() > 0 {
		writef(ctx.stderr, "unexpected argument(s): %s\n", strings.Join(fs.Args(), " "))
		printServiceUninstallHelp(ctx.stderr)
		return 2
	}

	err := uninstallUserSvcFn(service.UninstallUserOptions{
		Disable:    *disable,
		Stop:       *stop,
		RemoveUnit: *removeUnit,
	})
	if err != nil {
		writef(ctx.stderr, "service uninstall failed: %v\n", err)
		return 1
	}
	writeln(ctx.stdout, "service uninstalled")
	return 0
}

func runServiceStatusCommand(ctx commandContext, args []string) int {
	fs := flag.NewFlagSet("service status", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	help := fs.Bool("help", false, "show help")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *help {
		printServiceStatusHelp(ctx.stdout)
		return 0
	}
	if fs.NArg() > 0 {
		writef(ctx.stderr, "unexpected argument(s): %s\n", strings.Join(fs.Args(), " "))
		printServiceStatusHelp(ctx.stderr)
		return 2
	}

	status, err := userStatusFn()
	if err != nil {
		writef(ctx.stderr, "service status failed: %v\n", err)
		return 1
	}
	unitScope := unitScopeLabel(status.ServicePath)
	managerLabel := runtimeServiceManagerLabel()
	rows := []outputRow{
		{Key: fmt.Sprintf("%s unit file", unitScope), Value: status.ServicePath},
		{Key: fmt.Sprintf("%s unit exists", unitScope), Value: fmt.Sprintf("%t", status.UnitFileExists)},
		{Key: fmt.Sprintf("%s available", managerLabel), Value: fmt.Sprintf("%t", status.SystemctlAvailable)},
	}
	if status.SystemctlAvailable {
		rows = append(rows,
			outputRow{Key: fmt.Sprintf("%s unit enabled", unitScope), Value: status.EnabledState},
			outputRow{Key: fmt.Sprintf("%s unit active", unitScope), Value: status.ActiveState},
		)
	}
	printRows(ctx.stdout, rows)
	return 0
}

func runServiceAutoUpdateCommand(ctx commandContext, args []string) int {
	if len(args) == 0 {
		printServiceAutoUpdateHelp(ctx.stderr)
		return 2
	}

	switch args[0] {
	case "install":
		return runServiceAutoUpdateInstallCommand(ctx, args[1:])
	case "uninstall":
		return runServiceAutoUpdateUninstallCommand(ctx, args[1:])
	case cmdStatus:
		return runServiceAutoUpdateStatusCommand(ctx, args[1:])
	case cmdHelp, flagHelpShort, flagHelpLong:
		printServiceAutoUpdateHelp(ctx.stdout)
		return 0
	default:
		writef(ctx.stderr, "unknown autoupdate command: %s\n\n", args[0])
		printServiceAutoUpdateHelp(ctx.stderr)
		return 2
	}
}

func runServiceAutoUpdateInstallCommand(ctx commandContext, args []string) int {
	fs := flag.NewFlagSet("service autoupdate install", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	execPath := fs.String("exec", "", "path to sentinel binary for updater ExecStart (defaults to current executable)")
	enable := fs.Bool("enable", true, "enable autoupdate timer")
	start := fs.Bool("start", true, "start autoupdate timer now")
	serviceUnit := fs.String("service", "sentinel", "service unit/label to restart after update")
	scope := fs.String("scope", defaultAutoUpdateScopeFlag(), "restart manager scope: auto|user|system|launchd")
	onCalendar := fs.String("on-calendar", "daily", "update schedule (daily|hourly|weekly|duration|seconds)")
	randomizedDelay := fs.Duration("randomized-delay", time.Hour, "randomized delay before update (systemd only)")
	help := fs.Bool("help", false, "show help")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *help {
		printServiceAutoUpdateInstallHelp(ctx.stdout)
		return 0
	}
	if fs.NArg() > 0 {
		writef(ctx.stderr, "unexpected argument(s): %s\n", strings.Join(fs.Args(), " "))
		printServiceAutoUpdateInstallHelp(ctx.stderr)
		return 2
	}

	if err := installUserAutoUpdateFn(service.InstallUserAutoUpdateOptions{
		ExecPath:        strings.TrimSpace(*execPath),
		Enable:          *enable,
		Start:           *start,
		ServiceUnit:     strings.TrimSpace(*serviceUnit),
		SystemdScope:    strings.TrimSpace(*scope),
		OnCalendar:      strings.TrimSpace(*onCalendar),
		RandomizedDelay: *randomizedDelay,
	}); err != nil {
		writef(ctx.stderr, "service autoupdate install failed: %v\n", err)
		return 1
	}

	resolvedScope := strings.TrimSpace(*scope)
	timerPath, pathErr := service.UserAutoUpdateTimerPathForScope(resolvedScope)
	if pathErr == nil {
		writef(ctx.stdout, "autoupdate timer installed: %s\n", timerPath)
	}
	switch {
	case *enable && *start:
		writeln(ctx.stdout, "autoupdate timer enabled and started")
	case *enable:
		writeln(ctx.stdout, "autoupdate timer enabled")
	case *start:
		writeln(ctx.stdout, "autoupdate timer started")
	default:
		writeln(ctx.stdout, "autoupdate timer installed (not enabled, not started)")
	}
	return 0
}

func runServiceAutoUpdateUninstallCommand(ctx commandContext, args []string) int {
	fs := flag.NewFlagSet("service autoupdate uninstall", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	disable := fs.Bool("disable", true, "disable autoupdate timer")
	stop := fs.Bool("stop", true, "stop autoupdate timer")
	removeUnit := fs.Bool("remove-unit", true, "remove autoupdate unit files")
	scope := fs.String("scope", defaultAutoUpdateScopeFlag(), "target scope: auto|user|system|launchd")
	help := fs.Bool("help", false, "show help")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *help {
		printServiceAutoUpdateUninstallHelp(ctx.stdout)
		return 0
	}
	if fs.NArg() > 0 {
		writef(ctx.stderr, "unexpected argument(s): %s\n", strings.Join(fs.Args(), " "))
		printServiceAutoUpdateUninstallHelp(ctx.stderr)
		return 2
	}

	if err := uninstallUserAutoUpdateFn(service.UninstallUserAutoUpdateOptions{
		Disable:    *disable,
		Stop:       *stop,
		RemoveUnit: *removeUnit,
		Scope:      strings.TrimSpace(*scope),
	}); err != nil {
		writef(ctx.stderr, "service autoupdate uninstall failed: %v\n", err)
		return 1
	}
	writeln(ctx.stdout, "autoupdate timer uninstalled")
	return 0
}

func runServiceAutoUpdateStatusCommand(ctx commandContext, args []string) int {
	fs := flag.NewFlagSet("service autoupdate status", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	scope := fs.String("scope", defaultAutoUpdateScopeFlag(), "target scope: auto|user|system|launchd")
	help := fs.Bool("help", false, "show help")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *help {
		printServiceAutoUpdateStatusHelp(ctx.stdout)
		return 0
	}
	if fs.NArg() > 0 {
		writef(ctx.stderr, "unexpected argument(s): %s\n", strings.Join(fs.Args(), " "))
		printServiceAutoUpdateStatusHelp(ctx.stderr)
		return 2
	}

	status, err := userAutoUpdateStatusFn(strings.TrimSpace(*scope))
	if err != nil {
		writef(ctx.stderr, "service autoupdate status failed: %v\n", err)
		return 1
	}
	managerLabel := runtimeServiceManagerLabel()
	rows := []outputRow{
		{Key: "service file", Value: status.ServicePath},
		{Key: "timer file", Value: status.TimerPath},
		{Key: "service unit exists", Value: fmt.Sprintf("%t", status.ServiceUnitExists)},
		{Key: "timer unit exists", Value: fmt.Sprintf("%t", status.TimerUnitExists)},
		{Key: fmt.Sprintf("%s available", managerLabel), Value: fmt.Sprintf("%t", status.SystemctlAvailable)},
	}
	if status.SystemctlAvailable {
		rows = append(rows,
			outputRow{Key: "timer enabled", Value: status.TimerEnabledState},
			outputRow{Key: "timer active", Value: status.TimerActiveState},
			outputRow{Key: "last run", Value: status.LastRunState},
		)
	}
	printRows(ctx.stdout, rows)
	return 0
}

func runUpdateCommand(ctx commandContext, args []string) int {
	if len(args) == 0 {
		printUpdateHelp(ctx.stderr)
		return 2
	}

	switch args[0] {
	case "check":
		return runUpdateCheckCommand(ctx, args[1:])
	case "apply":
		return runUpdateApplyCommand(ctx, args[1:])
	case cmdStatus:
		return runUpdateStatusCommand(ctx, args[1:])
	case cmdHelp, flagHelpShort, flagHelpLong:
		printUpdateHelp(ctx.stdout)
		return 0
	default:
		writef(ctx.stderr, "unknown update command: %s\n\n", args[0])
		printUpdateHelp(ctx.stderr)
		return 2
	}
}

func runUpdateCheckCommand(ctx commandContext, args []string) int {
	fs := flag.NewFlagSet("update check", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	repo := fs.String("repo", defaultUpdaterRepo, "GitHub repository in owner/name format")
	apiBase := fs.String("api", "", "GitHub API base URL override")
	targetOS := fs.String("os", runtime.GOOS, "target operating system")
	targetArch := fs.String("arch", runtime.GOARCH, "target CPU architecture")
	help := fs.Bool("help", false, "show help")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *help {
		printUpdateCheckHelp(ctx.stdout)
		return 0
	}
	if fs.NArg() > 0 {
		writef(ctx.stderr, "unexpected argument(s): %s\n", strings.Join(fs.Args(), " "))
		printUpdateCheckHelp(ctx.stderr)
		return 2
	}

	cfg := loadConfigFn()
	result, err := updateCheckFn(context.Background(), updater.CheckOptions{
		CurrentVersion: currentVersionFn(),
		Repo:           strings.TrimSpace(*repo),
		APIBaseURL:     strings.TrimSpace(*apiBase),
		OS:             strings.TrimSpace(*targetOS),
		Arch:           strings.TrimSpace(*targetArch),
		DataDir:        cfg.DataDir,
	})
	if err != nil {
		writef(ctx.stderr, "update check failed: %v\n", err)
		return 1
	}
	printRows(ctx.stdout, []outputRow{
		{Key: "current version", Value: valueOrDash(result.CurrentVersion)},
		{Key: "latest version", Value: valueOrDash(result.LatestVersion)},
		{Key: "up to date", Value: fmt.Sprintf("%t", result.UpToDate)},
		{Key: "release", Value: valueOrDash(result.ReleaseURL)},
		{Key: "asset", Value: valueOrDash(result.AssetName)},
		{Key: "sha256", Value: valueOrDash(result.ExpectedSHA256)},
	})
	return 0
}

func runUpdateApplyCommand(ctx commandContext, args []string) int {
	fs := flag.NewFlagSet("update apply", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	repo := fs.String("repo", defaultUpdaterRepo, "GitHub repository in owner/name format")
	apiBase := fs.String("api", "", "GitHub API base URL override")
	targetOS := fs.String("os", runtime.GOOS, "target operating system")
	targetArch := fs.String("arch", runtime.GOARCH, "target CPU architecture")
	execPath := fs.String("exec", "", "path to sentinel binary to replace (defaults to current executable)")
	allowDowngrade := fs.Bool("allow-downgrade", false, "allow installing an older release")
	allowUnverified := fs.Bool("allow-unverified", false, "allow update when checksum is unavailable")
	restart := fs.Bool("restart", false, "restart managed service after successful update")
	serviceUnit := fs.String("service", "sentinel", "service unit/label name to restart after update")
	scope := fs.String("scope", "", "restart scope: auto|user|system|launchd|none")
	systemdScope := fs.String("systemd-scope", "", "deprecated alias for --scope")
	help := fs.Bool("help", false, "show help")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *help {
		printUpdateApplyHelp(ctx.stdout)
		return 0
	}
	if fs.NArg() > 0 {
		writef(ctx.stderr, "unexpected argument(s): %s\n", strings.Join(fs.Args(), " "))
		printUpdateApplyHelp(ctx.stderr)
		return 2
	}
	resolvedScope, scopeErr := resolveRestartScopeFlag(*scope, *systemdScope)
	if scopeErr != nil {
		writef(ctx.stderr, "invalid scope flags: %v\n", scopeErr)
		return 2
	}

	cfg := loadConfigFn()
	result, err := updateApplyFn(context.Background(), updater.ApplyOptions{
		CurrentVersion:  currentVersionFn(),
		Repo:            strings.TrimSpace(*repo),
		APIBaseURL:      strings.TrimSpace(*apiBase),
		OS:              strings.TrimSpace(*targetOS),
		Arch:            strings.TrimSpace(*targetArch),
		DataDir:         cfg.DataDir,
		ExecPath:        strings.TrimSpace(*execPath),
		AllowDowngrade:  *allowDowngrade,
		AllowUnverified: *allowUnverified,
		Restart:         *restart,
		ServiceUnit:     strings.TrimSpace(*serviceUnit),
		SystemdScope:    resolvedScope,
	})
	if err != nil {
		writef(ctx.stderr, "update apply failed: %v\n", err)
		return 1
	}

	if !result.Applied {
		printRows(ctx.stdout, []outputRow{
			{Key: "already up to date", Value: valueOrDash(result.CurrentVersion)},
		})
		return 0
	}

	printNotice(ctx.stdout, "update applied successfully")
	printRows(ctx.stdout, []outputRow{
		{Key: "updated from", Value: valueOrDash(result.CurrentVersion)},
		{Key: "updated to", Value: valueOrDash(result.LatestVersion)},
		{Key: "binary", Value: valueOrDash(result.BinaryPath)},
		{Key: "backup", Value: valueOrDash(result.BackupPath)},
	})
	return 0
}

func runUpdateStatusCommand(ctx commandContext, args []string) int {
	fs := flag.NewFlagSet("update status", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	help := fs.Bool("help", false, "show help")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *help {
		printUpdateStatusHelp(ctx.stdout)
		return 0
	}
	if fs.NArg() > 0 {
		writef(ctx.stderr, "unexpected argument(s): %s\n", strings.Join(fs.Args(), " "))
		printUpdateStatusHelp(ctx.stderr)
		return 2
	}

	cfg := loadConfigFn()
	state, err := updateStatusFn(cfg.DataDir)
	if err != nil {
		writef(ctx.stderr, "update status failed: %v\n", err)
		return 1
	}
	printRows(ctx.stdout, []outputRow{
		{Key: "current version", Value: valueOrDash(state.CurrentVersion)},
		{Key: "latest version", Value: valueOrDash(state.LatestVersion)},
		{Key: "up to date", Value: fmt.Sprintf("%t", state.UpToDate)},
		{Key: "last checked", Value: formatTime(state.LastCheckedAt)},
		{Key: "last applied", Value: formatTime(state.LastAppliedAt)},
		{Key: "release", Value: valueOrDash(state.LastReleaseURL)},
		{Key: "binary", Value: valueOrDash(state.LastAppliedBinary)},
		{Key: "backup", Value: valueOrDash(state.LastAppliedBackup)},
		{Key: "sha256", Value: valueOrDash(state.LastExpectedSHA256)},
		{Key: "last error", Value: valueOrDash(state.LastError)},
	})
	return 0
}

func runDoctorCommand(ctx commandContext, args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	help := fs.Bool("help", false, "show help")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *help {
		printDoctorHelp(ctx.stdout)
		return 0
	}
	if fs.NArg() > 0 {
		writef(ctx.stderr, "unexpected argument(s): %s\n", strings.Join(fs.Args(), " "))
		printDoctorHelp(ctx.stderr)
		return 2
	}

	cfg := loadConfigFn()
	tmuxPath, tmuxErr := exec.LookPath("tmux")
	managerLabel := runtimeServiceManagerLabel()
	managerPath, managerErr := exec.LookPath(managerLabel)
	status, statusErr := userStatusFn()
	printHeading(ctx.stdout, "Sentinel doctor report")
	if !shouldUsePrettyOutput(ctx.stdout) {
		writeln(ctx.stdout, "---------------------")
	}
	rows := []outputRow{
		{Key: "os", Value: fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)},
		{Key: "supported host", Value: fmt.Sprintf("%t", runtime.GOOS == "linux" || runtime.GOOS == "darwin")},
		{Key: "listen", Value: cfg.ListenAddr},
		{Key: "data dir", Value: cfg.DataDir},
		{Key: "token required", Value: fmt.Sprintf("%t", cfg.Token != "")},
	}
	if tmuxErr == nil {
		rows = append(rows, outputRow{Key: "tmux", Value: tmuxPath})
	} else {
		rows = append(rows, outputRow{Key: "tmux", Value: "not found"})
	}
	if managerErr == nil {
		rows = append(rows, outputRow{Key: managerLabel, Value: managerPath})
	} else {
		rows = append(rows, outputRow{Key: managerLabel, Value: "not found"})
	}
	if statusErr == nil {
		unitScope := unitScopeLabel(status.ServicePath)
		rows = append(rows,
			outputRow{Key: fmt.Sprintf("%s unit file", unitScope), Value: status.ServicePath},
			outputRow{Key: fmt.Sprintf("%s unit exists", unitScope), Value: fmt.Sprintf("%t", status.UnitFileExists)},
		)
		if status.SystemctlAvailable {
			rows = append(rows,
				outputRow{Key: fmt.Sprintf("%s unit enabled", unitScope), Value: status.EnabledState},
				outputRow{Key: fmt.Sprintf("%s unit active", unitScope), Value: status.ActiveState},
			)
		}
	} else {
		rows = append(rows, outputRow{Key: "service status", Value: fmt.Sprintf("unavailable (%v)", statusErr)})
	}
	printRows(ctx.stdout, rows)
	return 0
}

func unitScopeLabel(servicePath string) string {
	path := strings.TrimSpace(servicePath)
	if path == "" {
		return "user"
	}

	normalized := filepath.Clean(path)
	if strings.HasPrefix(normalized, "/etc/systemd/system/") ||
		strings.HasPrefix(normalized, "/Library/LaunchDaemons/") {
		return "system"
	}
	return "user"
}

func runtimeServiceManagerLabel() string {
	if runtime.GOOS == "darwin" {
		return "launchctl"
	}
	return "systemctl"
}

func resolveRestartScopeFlag(scope, legacyScope string) (string, error) {
	primary := strings.TrimSpace(scope)
	legacy := strings.TrimSpace(legacyScope)
	switch {
	case primary == "" && legacy == "":
		return "", nil
	case primary == "":
		return legacy, nil
	case legacy == "":
		return primary, nil
	case strings.EqualFold(primary, legacy):
		return primary, nil
	default:
		return "", fmt.Errorf("--scope=%s conflicts with --systemd-scope=%s", primary, legacy)
	}
}

func runRecoveryCommand(ctx commandContext, args []string) int {
	if len(args) == 0 {
		printRecoveryHelp(ctx.stderr)
		return 2
	}

	switch args[0] {
	case "list":
		return runRecoveryListCommand(ctx, args[1:])
	case "restore":
		return runRecoveryRestoreCommand(ctx, args[1:])
	case cmdHelp, flagHelpShort, flagHelpLong:
		printRecoveryHelp(ctx.stdout)
		return 0
	default:
		writef(ctx.stderr, "unknown recovery command: %s\n\n", args[0])
		printRecoveryHelp(ctx.stderr)
		return 2
	}
}

func runRecoveryListCommand(ctx commandContext, args []string) int {
	fs := flag.NewFlagSet("recovery list", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	state := fs.String("state", "killed", "session states (comma-separated): killed,restoring,restored,running,archived")
	limit := fs.Int("limit", 100, "maximum sessions to print")
	help := fs.Bool("help", false, "show help")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *help {
		printRecoveryListHelp(ctx.stdout)
		return 0
	}
	if fs.NArg() > 0 {
		writef(ctx.stderr, "unexpected argument(s): %s\n", strings.Join(fs.Args(), " "))
		printRecoveryListHelp(ctx.stderr)
		return 2
	}

	states, err := parseRecoveryStates(*state)
	if err != nil {
		writef(ctx.stderr, "invalid state: %v\n", err)
		return 2
	}

	cfg := loadConfigFn()
	st, err := store.New(filepath.Join(cfg.DataDir, "sentinel.db"))
	if err != nil {
		writef(ctx.stderr, "store init failed: %v\n", err)
		return 1
	}
	defer func() { _ = st.Close() }()

	rows, err := st.ListRecoverySessions(context.Background(), states)
	if err != nil {
		writef(ctx.stderr, "failed to list recovery sessions: %v\n", err)
		return 1
	}
	if len(rows) == 0 {
		writeln(ctx.stdout, "no recovery sessions found")
		return 0
	}
	if *limit > 0 && len(rows) > *limit {
		rows = rows[:*limit]
	}

	for _, item := range rows {
		timeLabel := item.SnapshotAt.Format(time.RFC3339)
		if item.SnapshotAt.IsZero() {
			timeLabel = "-"
		}
		writef(ctx.stdout,
			"%s\tstate=%s\tsnapshot=%d\twindows=%d\tpanes=%d\tat=%s\n",
			item.Name, item.State, item.LatestSnapshotID, item.Windows, item.Panes, timeLabel,
		)
	}
	return 0
}

func runRecoveryRestoreCommand(ctx commandContext, args []string) int {
	fs := flag.NewFlagSet("recovery restore", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	snapshotID := fs.Int64("snapshot", 0, "snapshot id to restore")
	mode := fs.String("mode", "confirm", "replay mode: safe|confirm|full")
	conflict := fs.String("conflict", "rename", "name conflict policy: rename|replace|skip")
	target := fs.String("target", "", "target tmux session name")
	wait := fs.Bool("wait", true, "wait for completion and print progress")
	help := fs.Bool("help", false, "show help")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *help {
		printRecoveryRestoreHelp(ctx.stdout)
		return 0
	}
	if fs.NArg() > 0 {
		writef(ctx.stderr, "unexpected argument(s): %s\n", strings.Join(fs.Args(), " "))
		printRecoveryRestoreHelp(ctx.stderr)
		return 2
	}
	if *snapshotID <= 0 {
		writeln(ctx.stderr, "snapshot id is required and must be > 0")
		return 2
	}

	cfg := loadConfigFn()
	st, err := store.New(filepath.Join(cfg.DataDir, "sentinel.db"))
	if err != nil {
		writef(ctx.stderr, "store init failed: %v\n", err)
		return 1
	}
	defer func() { _ = st.Close() }()

	svc := recovery.New(st, tmux.Service{}, recovery.Options{
		SnapshotInterval:    cfg.Recovery.SnapshotInterval,
		CaptureLines:        cfg.Recovery.CaptureLines,
		MaxSnapshotsPerSess: cfg.Recovery.MaxSnapshots,
	})

	job, err := svc.RestoreSnapshotAsync(context.Background(), *snapshotID, recovery.RestoreOptions{
		Mode:           recovery.ReplayMode(strings.ToLower(strings.TrimSpace(*mode))),
		ConflictPolicy: recovery.ConflictPolicy(strings.ToLower(strings.TrimSpace(*conflict))),
		TargetSession:  strings.TrimSpace(*target),
	})
	if err != nil {
		writef(ctx.stderr, "failed to start restore: %v\n", err)
		return 1
	}
	writef(ctx.stdout, "restore job started: %s (session=%s snapshot=%d)\n", job.ID, job.SessionName, job.SnapshotID)
	if !*wait {
		return 0
	}

	deadline := time.Now().Add(5 * time.Minute)
	lastProgress := ""
	for time.Now().Before(deadline) {
		current, err := svc.GetJob(context.Background(), job.ID)
		if err != nil {
			writef(ctx.stderr, "failed to load restore job: %v\n", err)
			return 1
		}
		progress := fmt.Sprintf("%d/%d", current.CompletedSteps, current.TotalSteps)
		line := fmt.Sprintf("status=%s progress=%s step=%s", current.Status, progress, current.CurrentStep)
		if line != lastProgress {
			writeln(ctx.stdout, line)
			lastProgress = line
		}
		switch current.Status {
		case store.RecoveryJobSucceeded:
			writef(ctx.stdout, "restore finished successfully (target=%s)\n", current.TargetSession)
			return 0
		case store.RecoveryJobFailed, store.RecoveryJobPartial:
			writef(ctx.stderr, "restore finished with errors: %s\n", current.Error)
			return 1
		}
		time.Sleep(900 * time.Millisecond)
	}
	writeln(ctx.stderr, "restore job timeout exceeded (5m)")
	return 1
}

func parseRecoveryStates(raw string) ([]store.RecoverySessionState, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []store.RecoverySessionState{store.RecoveryStateKilled}, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]store.RecoverySessionState, 0, len(parts))
	for _, part := range parts {
		value := strings.ToLower(strings.TrimSpace(part))
		switch value {
		case string(store.RecoveryStateRunning),
			string(store.RecoveryStateKilled),
			string(store.RecoveryStateRestoring),
			string(store.RecoveryStateRestored),
			string(store.RecoveryStateArchived):
			out = append(out, store.RecoverySessionState(value))
		default:
			return nil, fmt.Errorf("%q", value)
		}
	}
	return out, nil
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.UTC().Format(time.RFC3339)
}

func valueOrDash(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "-"
	}
	return raw
}

func defaultAutoUpdateScopeFlag() string {
	return "auto"
}

func printRootHelp(w io.Writer) {
	writeln(w, "Sentinel command-line interface")
	writeln(w, "")
	writeln(w, "Usage:")
	writeln(w, "  sentinel [serve]")
	writeln(w, "  sentinel service <install|uninstall|status|autoupdate>")
	writeln(w, "  sentinel doctor")
	writeln(w, "  sentinel recovery <list|restore>")
	writeln(w, "  sentinel update <check|apply|status>")
	writeln(w, "")
	writeln(w, "Commands:")
	writeln(w, "  serve      Start Sentinel HTTP server (default)")
	writeln(w, "  service    Manage local service and autoupdate timer (systemd/launchd)")
	writeln(w, "  doctor     Check local environment and runtime config")
	writeln(w, "  recovery   Inspect and restore persisted tmux snapshots")
	writeln(w, "  update     Check/apply binary updates from GitHub releases")
}

func printServeHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel serve")
	writeln(w, "")
	writeln(w, "Starts the Sentinel server using config file/env defaults.")
}

func printServiceHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel service install [--exec PATH] [--enable=true] [--start=true]")
	writeln(w, "  sentinel service uninstall [--disable=true] [--stop=true] [--remove-unit=true]")
	writeln(w, "  sentinel service status")
	writeln(w, "  sentinel service autoupdate <install|uninstall|status>")
}

func printServiceInstallHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel service install [--exec PATH] [--enable=true] [--start=true]")
}

func printServiceUninstallHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel service uninstall [--disable=true] [--stop=true] [--remove-unit=true]")
}

func printServiceStatusHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel service status")
}

func printServiceAutoUpdateHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel service autoupdate install [--exec PATH] [--enable=true] [--start=true] [--service sentinel] [--scope auto|user|system|launchd] [--on-calendar daily] [--randomized-delay 1h]")
	writeln(w, "  sentinel service autoupdate uninstall [--disable=true] [--stop=true] [--remove-unit=true] [--scope auto|user|system|launchd]")
	writeln(w, "  sentinel service autoupdate status [--scope auto|user|system|launchd]")
}

func printServiceAutoUpdateInstallHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel service autoupdate install [--exec PATH] [--enable=true] [--start=true] [--service sentinel] [--scope auto|user|system|launchd] [--on-calendar daily] [--randomized-delay 1h]")
}

func printServiceAutoUpdateUninstallHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel service autoupdate uninstall [--disable=true] [--stop=true] [--remove-unit=true] [--scope auto|user|system|launchd]")
}

func printServiceAutoUpdateStatusHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel service autoupdate status [--scope auto|user|system|launchd]")
}

func printDoctorHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel doctor")
}

func printRecoveryHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel recovery list [--state killed,restoring,restored] [--limit 100]")
	writeln(w, "  sentinel recovery restore --snapshot ID [--mode confirm] [--conflict rename] [--target NAME] [--wait=true]")
}

func printRecoveryListHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel recovery list [--state killed,restoring,restored] [--limit 100]")
}

func printRecoveryRestoreHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel recovery restore --snapshot ID [--mode confirm] [--conflict rename] [--target NAME] [--wait=true]")
}

func printUpdateHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel update check [--repo owner/name] [--api URL] [--os linux] [--arch amd64]")
	writeln(w, "  sentinel update apply [--repo owner/name] [--api URL] [--exec PATH] [--allow-downgrade=false] [--allow-unverified=false] [--restart=false] [--service sentinel] [--scope auto|user|system|launchd|none]")
	writeln(w, "  sentinel update status")
}

func printUpdateCheckHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel update check [--repo owner/name] [--api URL] [--os linux] [--arch amd64]")
}

func printUpdateApplyHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel update apply [--repo owner/name] [--api URL] [--exec PATH] [--allow-downgrade=false] [--allow-unverified=false] [--restart=false] [--service sentinel] [--scope auto|user|system|launchd|none]")
}

func printUpdateStatusHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel update status")
}

func currentVersion() string {
	if value := strings.TrimSpace(buildVersion); value != "" && value != "dev" && value != "(devel)" {
		return value
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if strings.TrimSpace(bi.Main.Version) != "" && bi.Main.Version != "(devel)" {
			return bi.Main.Version
		}
	}
	return "dev"
}
