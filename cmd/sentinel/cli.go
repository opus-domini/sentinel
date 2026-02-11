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

func runCLI(args []string, stdout, stderr io.Writer) int {
	ctx := commandContext{stdout: stdout, stderr: stderr}

	if len(args) == 0 {
		return serveFn()
	}

	switch args[0] {
	case "-v", "--version", "version":
		fmt.Fprintf(stdout, "sentinel version %s\n", currentVersionFn())
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
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
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
		fmt.Fprintf(ctx.stderr, "unexpected argument(s): %s\n", strings.Join(fs.Args(), " "))
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
		fmt.Fprintf(ctx.stderr, "unknown service command: %s\n\n", args[0])
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
		fmt.Fprintf(ctx.stderr, "unexpected argument(s): %s\n", strings.Join(fs.Args(), " "))
		printServiceInstallHelp(ctx.stderr)
		return 2
	}

	err := installUserSvcFn(service.InstallUserOptions{
		ExecPath: strings.TrimSpace(*execPath),
		Enable:   *enable,
		Start:    *start,
	})
	if err != nil {
		fmt.Fprintf(ctx.stderr, "service install failed: %v\n", err)
		return 1
	}

	path, pathErr := service.UserServicePath()
	if pathErr == nil {
		fmt.Fprintf(ctx.stdout, "service installed: %s\n", path)
	}
	if *enable && *start {
		fmt.Fprintln(ctx.stdout, "service enabled and started")
	} else if *enable {
		fmt.Fprintln(ctx.stdout, "service enabled")
	} else if *start {
		fmt.Fprintln(ctx.stdout, "service started")
	} else {
		fmt.Fprintln(ctx.stdout, "service installed (not enabled, not started)")
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
		fmt.Fprintf(ctx.stderr, "unexpected argument(s): %s\n", strings.Join(fs.Args(), " "))
		printServiceUninstallHelp(ctx.stderr)
		return 2
	}

	err := uninstallUserSvcFn(service.UninstallUserOptions{
		Disable:    *disable,
		Stop:       *stop,
		RemoveUnit: *removeUnit,
	})
	if err != nil {
		fmt.Fprintf(ctx.stderr, "service uninstall failed: %v\n", err)
		return 1
	}
	fmt.Fprintln(ctx.stdout, "service uninstalled")
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
		fmt.Fprintf(ctx.stderr, "unexpected argument(s): %s\n", strings.Join(fs.Args(), " "))
		printServiceStatusHelp(ctx.stderr)
		return 2
	}

	status, err := userStatusFn()
	if err != nil {
		fmt.Fprintf(ctx.stderr, "service status failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(ctx.stdout, "service file: %s\n", status.ServicePath)
	fmt.Fprintf(ctx.stdout, "unit exists: %t\n", status.UnitFileExists)
	fmt.Fprintf(ctx.stdout, "systemctl: %t\n", status.SystemctlAvailable)
	if status.SystemctlAvailable {
		fmt.Fprintf(ctx.stdout, "enabled: %s\n", status.EnabledState)
		fmt.Fprintf(ctx.stdout, "active: %s\n", status.ActiveState)
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
		fmt.Fprintf(ctx.stderr, "unexpected argument(s): %s\n", strings.Join(fs.Args(), " "))
		printDoctorHelp(ctx.stderr)
		return 2
	}

	cfg := loadConfigFn()
	tmuxPath, tmuxErr := exec.LookPath("tmux")
	systemctlPath, systemctlErr := exec.LookPath("systemctl")
	status, statusErr := userStatusFn()

	fmt.Fprintln(ctx.stdout, "Sentinel doctor report")
	fmt.Fprintln(ctx.stdout, "---------------------")
	fmt.Fprintf(ctx.stdout, "os: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(ctx.stdout, "supported host: %t\n", runtime.GOOS == "linux" || runtime.GOOS == "darwin")
	if tmuxErr == nil {
		fmt.Fprintf(ctx.stdout, "tmux: %s\n", tmuxPath)
	} else {
		fmt.Fprintln(ctx.stdout, "tmux: not found")
	}
	fmt.Fprintf(ctx.stdout, "listen: %s\n", cfg.ListenAddr)
	fmt.Fprintf(ctx.stdout, "data dir: %s\n", cfg.DataDir)
	fmt.Fprintf(ctx.stdout, "token required: %t\n", cfg.Token != "")
	if systemctlErr == nil {
		fmt.Fprintf(ctx.stdout, "systemctl: %s\n", systemctlPath)
	} else {
		fmt.Fprintln(ctx.stdout, "systemctl: not found")
	}
	if statusErr == nil {
		fmt.Fprintf(ctx.stdout, "user unit file: %s\n", status.ServicePath)
		fmt.Fprintf(ctx.stdout, "user unit exists: %t\n", status.UnitFileExists)
		if status.SystemctlAvailable {
			fmt.Fprintf(ctx.stdout, "user unit enabled: %s\n", status.EnabledState)
			fmt.Fprintf(ctx.stdout, "user unit active: %s\n", status.ActiveState)
		}
	} else {
		fmt.Fprintf(ctx.stdout, "service status: unavailable (%v)\n", statusErr)
	}
	return 0
}

func printRootHelp(w io.Writer) {
	fmt.Fprintln(w, "Sentinel command-line interface")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  sentinel [serve]")
	fmt.Fprintln(w, "  sentinel service <install|uninstall|status>")
	fmt.Fprintln(w, "  sentinel doctor")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  serve      Start Sentinel HTTP server (default)")
	fmt.Fprintln(w, "  service    Manage systemd user service (Linux)")
	fmt.Fprintln(w, "  doctor     Check local environment and runtime config")
}

func printServeHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  sentinel serve")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Starts the Sentinel server using config file/env defaults.")
}

func printServiceHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  sentinel service install [-exec PATH] [-enable=true] [-start=true]")
	fmt.Fprintln(w, "  sentinel service uninstall [-disable=true] [-stop=true] [-remove-unit=true]")
	fmt.Fprintln(w, "  sentinel service status")
}

func printServiceInstallHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  sentinel service install [-exec PATH] [-enable=true] [-start=true]")
}

func printServiceUninstallHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  sentinel service uninstall [-disable=true] [-stop=true] [-remove-unit=true]")
}

func printServiceStatusHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  sentinel service status")
}

func printDoctorHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  sentinel doctor")
}

func currentVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		if strings.TrimSpace(bi.Main.Version) != "" && bi.Main.Version != "(devel)" {
			return bi.Main.Version
		}
	}
	return "dev"
}
