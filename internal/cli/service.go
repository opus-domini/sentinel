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
		newServiceAutoUpdateCmd(app),
	)
	return cmd
}

func newServiceInstallCmd(app *App) *cobra.Command {
	var (
		execPath string
		enable   bool
		start    bool
	)
	cmd := &cobra.Command{
		Use:   "install",
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
		return failf(1, "service install failed: %w", err)
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
		return failf(1, "service uninstall failed: %w", err)
	}
	writeln(app.Stdout, "service uninstalled")

	if purge {
		for _, path := range removeBashCompletionFn() {
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

// removeBashCompletion deletes the installed bash completion script from the
// user and system completion directories. It returns the paths it removed.
func removeBashCompletion() []string {
	paths := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".local", "share", "bash-completion", "completions", "sentinel"))
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
	status, err := userStatusFn()
	if err != nil {
		return failf(1, "service status failed: %w", err)
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
	printRows(app.Stdout, rows)
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
				return failf(1, "service logs failed: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "stream new log lines as they arrive")
	cmd.Flags().IntVarP(&lines, "lines", "n", 50, "number of past log lines to show")
	return cmd
}

// unitScopeLabel reports whether a unit path is a system- or user-scoped unit.
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

// runtimeServiceManagerLabel names the service manager for the current OS.
func runtimeServiceManagerLabel() string {
	if runtime.GOOS == "darwin" {
		return "launchctl"
	}
	return "systemctl"
}
