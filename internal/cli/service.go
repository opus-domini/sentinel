package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/opus-domini/sentinel/internal/daemon"
)

func newServiceCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage the local service and autoupdate timer",
		Long:  "Manage the Sentinel background service and its automatic update timer.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newServiceInstallCmd(app),
		newServiceUninstallCmd(app),
		newServiceStatusCmd(app),
		newServiceLogsCmd(app),
		newServiceLifecycleCmd(app, "start", "Start the service", "service started"),
		newServiceLifecycleCmd(app, "stop", "Stop the service", "service stopped"),
		newServiceLifecycleCmd(app, "restart", "Restart the service", "service restarted"),
		newServiceLifecycleCmd(app, "enable", "Enable the service at startup", "service enabled"),
		newServiceLifecycleCmd(app, "disable", "Disable the service from startup", "service disabled"),
		newServiceAutoUpdateCmd(app),
	)
	return cmd
}

// newServiceLifecycleCmd builds a leaf command that runs a single systemd/
// launchd lifecycle action (start, stop, restart, enable, disable).
func newServiceLifecycleCmd(app *App, action, short, doneMsg string) *cobra.Command {
	return &cobra.Command{
		Use:   action,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := controlServiceFn(action); err != nil {
				return failf("service %s failed: %w", action, err)
			}
			writeln(app.Stdout, doneMsg)
			return nil
		},
	}
}

func newServiceInstallCmd(app *App) *cobra.Command {
	var (
		execPath string
		enable   bool
		start    bool
	)
	cmd := &cobra.Command{
		Use:   cmdInstall,
		Short: "Install the service unit and start it",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServiceInstall(app, execPath, enable, start)
		},
	}
	cmd.Flags().StringVar(&execPath, "exec", "", "binary path for the service unit (default: current executable)")
	cmd.Flags().BoolVar(&enable, "enable", true, "enable the service at startup")
	cmd.Flags().BoolVar(&start, "start", true, "start the service immediately")
	return cmd
}

func runServiceInstall(app *App, execPath string, enable, start bool) error {
	if err := installUserSvcFn(daemon.InstallUserOptions{
		ExecPath: strings.TrimSpace(execPath),
		Enable:   enable,
		Start:    start,
	}); err != nil {
		return failf("service install failed: %w", err)
	}

	if path, err := daemon.UserServicePath(); err == nil {
		writef(app.Stdout, "service installed: %s\n", path)
	}
	switch {
	case enable && start:
		writeln(app.Stdout, "service enabled and started")
	case enable:
		writeln(app.Stdout, "service enabled")
	case start:
		writeln(app.Stdout, "service started")
	default:
		writeln(app.Stdout, "service installed (not enabled, not started)")
	}
	return nil
}

func newServiceUninstallCmd(app *App) *cobra.Command {
	var (
		disable    bool
		stop       bool
		removeUnit bool
		purge      bool
	)
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Stop the service and remove its unit",
		Long: "Stop the Sentinel service and remove its unit file.\n\n" +
			"--purge also removes the autoupdate timer, the shell completion and the\n" +
			"sentinel binary. User data in ~/.sentinel is left intact.",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServiceUninstall(app, disable, stop, removeUnit, purge)
		},
	}
	cmd.Flags().BoolVar(&disable, "disable", true, "disable the service from auto-start")
	cmd.Flags().BoolVar(&stop, "stop", true, "stop the running service")
	cmd.Flags().BoolVar(&removeUnit, "remove-unit", true, "remove the managed unit file")
	cmd.Flags().BoolVar(&purge, "purge", false, "also remove the autoupdate timer, shell completion and binary")
	return cmd
}

func runServiceUninstall(app *App, disable, stop, removeUnit, purge bool) error {
	if purge {
		if err := uninstallUserAutoUpdateFn(daemon.UninstallUserAutoUpdateOptions{
			Disable:    true,
			Stop:       true,
			RemoveUnit: true,
		}); err != nil {
			writef(app.Stderr, "autoupdate timer not removed: %v\n", err)
		} else {
			writeln(app.Stdout, "autoupdate timer removed")
		}
	}

	if err := uninstallUserSvcFn(daemon.UninstallUserOptions{
		Disable:    disable,
		Stop:       stop,
		RemoveUnit: removeUnit,
	}); err != nil {
		return failf("service uninstall failed: %w", err)
	}
	writeln(app.Stdout, "service uninstalled")

	if purge {
		for _, path := range removeShellCompletionsFn() {
			writef(app.Stdout, "removed %s\n", path)
		}
		if path, err := removeSentinelBinaryFn(); err != nil {
			writef(app.Stderr, "binary not removed: %v\n", err)
		} else {
			writef(app.Stdout, "removed %s\n", path)
		}
	}
	return nil
}

// removeShellCompletions deletes installed shell completion scripts. It returns
// the paths it removed.
func removeShellCompletions() []string {
	paths := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".local", "share", "bash-completion", "completions", "sentinel"))
		paths = append(paths, filepath.Join(home, ".local", "share", "zsh", "site-functions", "_sentinel"))
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			configHome = filepath.Join(home, ".config")
		}
		paths = append(paths, filepath.Join(configHome, "fish", "completions", "sentinel.fish"))
	}
	paths = append(paths, "/usr/share/bash-completion/completions/sentinel")

	removed := make([]string, 0, len(paths))
	for _, path := range paths {
		if err := os.Remove(path); err == nil {
			removed = append(removed, path)
		}
	}
	return removed
}

// removeSentinelBinary deletes the running sentinel executable. On Linux and
// macOS a process can unlink its own binary; the inode survives until exit.
func removeSentinelBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if err := os.Remove(exe); err != nil {
		return exe, err
	}
	return exe, nil
}

func newServiceStatusCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   cmdStatus,
		Short: "Show whether the service is installed and running",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServiceStatus(app)
		},
	}
}

func runServiceStatus(app *App) error {
	report, err := serviceStatusFn()
	if err != nil {
		return failf("service status failed: %w", err)
	}
	if len(report) == 0 {
		writeln(app.Stdout, "no Sentinel service is installed (checked user and system scopes)")
		return nil
	}
	managerLabel := runtimeServiceManagerLabel()
	for i, s := range report {
		if i > 0 {
			writeln(app.Stdout, "")
		}
		rows := []outputRow{
			{Key: fmt.Sprintf("%s unit file", s.Scope), Value: s.ServicePath},
			{Key: fmt.Sprintf("%s unit exists", s.Scope), Value: fmt.Sprintf("%t", s.UnitFileExists)},
			{Key: fmt.Sprintf("%s available", managerLabel), Value: fmt.Sprintf("%t", s.SystemctlAvailable)},
		}
		if s.SystemctlAvailable {
			rows = append(rows,
				outputRow{Key: fmt.Sprintf("%s unit enabled", s.Scope), Value: s.EnabledState},
				outputRow{Key: fmt.Sprintf("%s unit active", s.Scope), Value: s.ActiveState},
			)
		}
		printRows(app.Stdout, rows)
	}
	return nil
}

func newServiceLogsCmd(app *App) *cobra.Command {
	var (
		follow bool
		lines  int
	)
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Stream the service log",
		Long:  "Stream the Sentinel service log (journalctl on Linux, plist logs on macOS).",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := userLogsFn(daemon.LogsOptions{
				Follow: follow,
				Lines:  lines,
				Stdout: app.Stdout,
				Stderr: app.Stderr,
			}); err != nil {
				return failf("service logs failed: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "stream new log lines as they arrive")
	cmd.Flags().IntVarP(&lines, "lines", "n", 50, "number of past log lines to show")
	return cmd
}

// runtimeServiceManagerLabel names the service manager for the current OS.
func runtimeServiceManagerLabel() string {
	if runtime.GOOS == hostOSDarwin {
		return "launchctl"
	}
	return "systemctl"
}
