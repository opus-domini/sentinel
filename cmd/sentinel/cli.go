package main

import (
	"flag"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/service"
)

var (
	serveFn            = serve
	installUserSvcFn   = service.InstallUser
	uninstallUserSvcFn = service.UninstallUser
	userStatusFn       = service.UserStatus
	loadConfigFn       = config.Load
	currentVersionFn   = currentVersion
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
	case "help", "-h", "--help":
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
	case "status":
		return runServiceStatusCommand(ctx, args[1:])
	case "help", "-h", "--help":
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
	enable := fs.Bool("enable", true, "enable service at login")
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
	removeUnit := fs.Bool("remove-unit", true, "remove user unit file")
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
	writef(ctx.stdout, "service file: %s\n", status.ServicePath)
	writef(ctx.stdout, "unit exists: %t\n", status.UnitFileExists)
	writef(ctx.stdout, "systemctl: %t\n", status.SystemctlAvailable)
	if status.SystemctlAvailable {
		writef(ctx.stdout, "enabled: %s\n", status.EnabledState)
		writef(ctx.stdout, "active: %s\n", status.ActiveState)
	}
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
	systemctlPath, systemctlErr := exec.LookPath("systemctl")
	status, statusErr := userStatusFn()

	writeln(ctx.stdout, "Sentinel doctor report")
	writeln(ctx.stdout, "---------------------")
	writef(ctx.stdout, "os: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	writef(ctx.stdout, "supported host: %t\n", runtime.GOOS == "linux" || runtime.GOOS == "darwin")
	if tmuxErr == nil {
		writef(ctx.stdout, "tmux: %s\n", tmuxPath)
	} else {
		writeln(ctx.stdout, "tmux: not found")
	}
	writef(ctx.stdout, "listen: %s\n", cfg.ListenAddr)
	writef(ctx.stdout, "data dir: %s\n", cfg.DataDir)
	writef(ctx.stdout, "token required: %t\n", cfg.Token != "")
	if systemctlErr == nil {
		writef(ctx.stdout, "systemctl: %s\n", systemctlPath)
	} else {
		writeln(ctx.stdout, "systemctl: not found")
	}
	if statusErr == nil {
		writef(ctx.stdout, "user unit file: %s\n", status.ServicePath)
		writef(ctx.stdout, "user unit exists: %t\n", status.UnitFileExists)
		if status.SystemctlAvailable {
			writef(ctx.stdout, "user unit enabled: %s\n", status.EnabledState)
			writef(ctx.stdout, "user unit active: %s\n", status.ActiveState)
		}
	} else {
		writef(ctx.stdout, "service status: unavailable (%v)\n", statusErr)
	}
	return 0
}

func printRootHelp(w io.Writer) {
	writeln(w, "Sentinel command-line interface")
	writeln(w, "")
	writeln(w, "Usage:")
	writeln(w, "  sentinel [serve]")
	writeln(w, "  sentinel service <install|uninstall|status>")
	writeln(w, "  sentinel doctor")
	writeln(w, "")
	writeln(w, "Commands:")
	writeln(w, "  serve      Start Sentinel HTTP server (default)")
	writeln(w, "  service    Manage systemd user service (Linux)")
	writeln(w, "  doctor     Check local environment and runtime config")
}

func printServeHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel serve")
	writeln(w, "")
	writeln(w, "Starts the Sentinel server using config file/env defaults.")
}

func printServiceHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel service install [-exec PATH] [-enable=true] [-start=true]")
	writeln(w, "  sentinel service uninstall [-disable=true] [-stop=true] [-remove-unit=true]")
	writeln(w, "  sentinel service status")
}

func printServiceInstallHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel service install [-exec PATH] [-enable=true] [-start=true]")
}

func printServiceUninstallHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel service uninstall [-disable=true] [-stop=true] [-remove-unit=true]")
}

func printServiceStatusHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel service status")
}

func printDoctorHelp(w io.Writer) {
	writeln(w, "Usage:")
	writeln(w, "  sentinel doctor")
}

func currentVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		if strings.TrimSpace(bi.Main.Version) != "" && bi.Main.Version != "(devel)" {
			return bi.Main.Version
		}
	}
	return "dev"
}
