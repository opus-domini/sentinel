package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/opus-domini/sentinel/internal/config"
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
		newServiceResolveInstallScopeCmd(app),
		newServiceInstallCmd(app),
		newServiceMigrateCmd(app),
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
	var scope string
	cmd := &cobra.Command{
		Use:   action,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := controlScopedServiceFn(action, scope); err != nil {
				return failf("service %s failed: %w", action, err)
			}
			writeln(app.Stdout, doneMsg)
			return nil
		},
	}
	cmd.Flags().StringVar(&scope, "scope", optionAuto, "target deployment: auto|user|system")
	return cmd
}

func newServiceInstallCmd(app *App) *cobra.Command {
	var (
		execPath string
		enable   bool
		start    bool
		scope    string
		check    bool
	)
	cmd := &cobra.Command{
		Use:   cmdInstall,
		Short: "Install the service unit and start it",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServiceInstall(app, execPath, scope, enable, start, check)
		},
	}
	cmd.Flags().StringVar(&execPath, "exec", "", "binary path for the service unit (default: current executable)")
	cmd.Flags().StringVar(&scope, "scope", optionAuto, "installation scope: auto|user|system")
	cmd.Flags().BoolVar(&enable, "enable", true, "enable the service at startup")
	cmd.Flags().BoolVar(&start, "start", true, "start the service immediately")
	cmd.Flags().BoolVar(&check, "check", false, "validate scope, config and binary destination without installing")
	return cmd
}

func runServiceInstall(app *App, execPath, scopeRaw string, enable, start, check bool) error {
	scope, err := resolveInstallScopeFn(scopeRaw)
	if err != nil {
		return failf("service install failed: %w", err)
	}
	if err := requireScopeAccessFn(scope); err != nil {
		return failf("service install failed: %w", err)
	}
	resolvedExecPath, err := validateServiceInstallBinary(scope, execPath)
	if err != nil {
		return failf("service install failed: %w", err)
	}
	configPath, dataDir, err := prepareServiceConfig(scope, !check)
	if err != nil {
		return failf("service install failed: %w", err)
	}
	if check {
		if err := preflightInstallDestination(resolvedExecPath); err != nil {
			return failf("service install check failed: %w", err)
		}
		writef(app.Stdout, "service install check passed: scope=%s config=%s data=%s\n", scope, configPath, dataDir)
		return nil
	}
	if err := installUserSvcFn(daemon.InstallUserOptions{
		ExecPath:   resolvedExecPath,
		Scope:      scope,
		ConfigPath: configPath,
		DataDir:    dataDir,
		Enable:     enable,
		Start:      start,
	}); err != nil {
		return failf("service install failed: %w", err)
	}
	reconciledAutoUpdate, err := reconcileAutoUpdateFn(daemon.InstallUserAutoUpdateOptions{
		ExecPath:     resolvedExecPath,
		ConfigPath:   configPath,
		DataDir:      dataDir,
		ServiceUnit:  sentinelServiceUnit,
		SystemdScope: scope,
	})
	if err != nil {
		return failf("service install failed: refresh existing autoupdate: %w", err)
	}

	if path, err := daemon.UserServicePathForScope(scope); err == nil {
		writef(app.Stdout, "service installed: %s\n", path)
	}
	if reconciledAutoUpdate {
		writeln(app.Stdout, "existing autoupdate installation refreshed")
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

func validateServiceInstallBinary(scope, requestedPath string) (string, error) {
	requestedPath = strings.TrimSpace(requestedPath)
	if requestedPath == "" {
		var err error
		requestedPath, err = os.Executable()
		if err != nil {
			return "", fmt.Errorf("resolve current executable: %w", err)
		}
	}
	deployments, err := installedDeploymentsFn()
	if err != nil {
		return "", err
	}
	for _, deployment := range deployments {
		if deployment.Scope != scope {
			continue
		}
		if !sameBinaryPath(deployment.BinaryPath, requestedPath) {
			return "", fmt.Errorf(
				"the %s deployment uses %s; reinstall with --exec %s or uninstall it before changing the binary path",
				scope,
				deployment.BinaryPath,
				deployment.BinaryPath,
			)
		}
		return deployment.BinaryPath, nil
	}
	return requestedPath, nil
}

func preflightInstallDestination(execPath string) error {
	execPath = strings.TrimSpace(execPath)
	if execPath == "" {
		var err error
		execPath, err = os.Executable()
		if err != nil {
			return err
		}
	}
	dir := filepath.Dir(execPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create binary directory %s: %w", dir, err)
	}
	probe, err := os.CreateTemp(dir, ".sentinel-install-preflight-*")
	if err != nil {
		return fmt.Errorf("binary destination %s is not writable: %w", execPath, err)
	}
	probePath := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(probePath)
		return err
	}
	return os.Remove(probePath)
}

func newServiceUninstallCmd(app *App) *cobra.Command {
	var (
		disable    bool
		stop       bool
		removeUnit bool
		purge      bool
		scope      string
	)
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Stop the service and remove its unit",
		Long: "Stop the Sentinel service and remove its unit file.\n\n" +
			"--purge also removes the autoupdate timer, the shell completion and the\n" +
			"deployment binary. Configuration and runtime data are left intact.",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServiceUninstall(app, scope, disable, stop, removeUnit, purge)
		},
	}
	cmd.Flags().BoolVar(&disable, "disable", true, "disable the service from auto-start")
	cmd.Flags().BoolVar(&stop, "stop", true, "stop the running service")
	cmd.Flags().BoolVar(&removeUnit, "remove-unit", true, "remove the managed unit file")
	cmd.Flags().BoolVar(&purge, "purge", false, "also remove the autoupdate timer, shell completion and binary")
	cmd.Flags().StringVar(&scope, "scope", optionAuto, "target deployment: auto|user|system")
	return cmd
}

func runServiceUninstall(app *App, scopeRaw string, disable, stop, removeUnit, purge bool) error {
	deployment, err := resolveDeploymentFn(scopeRaw)
	if err != nil {
		return failf("service uninstall failed: %w", err)
	}
	if err := requireScopeAccessFn(deployment.Scope); err != nil {
		return failf("service uninstall failed: %w", err)
	}
	removeBinary := purge
	if purge {
		deployments, discoveryErr := installedDeploymentsFn()
		if discoveryErr != nil {
			removeBinary = false
			writef(app.Stderr, "binary not removed: cannot verify whether another deployment uses it: %v\n", discoveryErr)
		} else {
			for _, other := range deployments {
				if other.Scope != deployment.Scope && sameBinaryPath(other.BinaryPath, deployment.BinaryPath) {
					removeBinary = false
					writef(app.Stderr, "binary not removed: the %s deployment also uses %s\n", other.Scope, deployment.BinaryPath)
					break
				}
			}
		}
	}
	if purge {
		if err := uninstallUserAutoUpdateFn(daemon.UninstallUserAutoUpdateOptions{
			Disable:    true,
			Stop:       true,
			RemoveUnit: true,
			Scope:      deployment.Scope,
		}); err != nil {
			writef(app.Stderr, "autoupdate timer not removed: %v\n", err)
		} else {
			writeln(app.Stdout, "autoupdate timer removed")
		}
	}

	if err := uninstallUserSvcFn(daemon.UninstallUserOptions{
		Scope:      deployment.Scope,
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
		if removeBinary {
			if path, err := removeSentinelBinaryAtFn(deployment.BinaryPath); err != nil {
				writef(app.Stderr, "binary not removed: %v\n", err)
			} else {
				writef(app.Stdout, "removed %s\n", path)
			}
		}
	}
	return nil
}

func sameBinaryPath(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	if filepath.Clean(left) == filepath.Clean(right) {
		return true
	}
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	return leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo)
}

// removeShellCompletions deletes installed shell completion scripts. It returns
// the paths it removed.
func removeShellCompletions() []string {
	home, _ := os.UserHomeDir()
	return removeShellCompletionsFrom(home, os.Getenv("XDG_CONFIG_HOME"), "/usr/share/bash-completion/completions/sentinel")
}

func removeShellCompletionsFrom(home, configHome, systemPath string) []string {
	paths := make([]string, 0, 4)
	if home != "" {
		paths = append(paths, filepath.Join(home, ".local", "share", "bash-completion", "completions", "sentinel"))
		paths = append(paths, filepath.Join(home, ".local", "share", "zsh", "site-functions", "_sentinel"))
		if configHome == "" {
			configHome = filepath.Join(home, ".config")
		}
		paths = append(paths, filepath.Join(configHome, "fish", "completions", "sentinel.fish"))
	}
	if systemPath != "" {
		paths = append(paths, systemPath)
	}

	removed := make([]string, 0, len(paths))
	for _, path := range paths {
		if err := os.Remove(path); err == nil {
			removed = append(removed, path)
		}
	}
	return removed
}

func removeSentinelBinaryAt(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("deployment binary path is empty")
	}
	if err := os.Remove(path); err != nil {
		return path, err
	}
	return path, nil
}

func prepareServiceConfig(scope string, create bool) (string, string, error) {
	layout, err := daemon.LayoutForScope(scope)
	if err != nil {
		return "", "", err
	}
	if explicit := strings.TrimSpace(os.Getenv("SENTINEL_CONFIG")); explicit != "" &&
		filepath.Clean(config.Path()) != filepath.Clean(layout.ConfigPath) {
		return "", "", fmt.Errorf(
			"managed %s deployments use %s; remove --config or set it to the canonical path",
			scope,
			layout.ConfigPath,
		)
	}
	deployments, listErr := installedDeploymentsFn()
	if listErr != nil {
		return "", "", listErr
	}
	for _, deployment := range deployments {
		if deployment.Scope != scope {
			continue
		}
		canonical, canonicalErr := daemon.HasCanonicalPaths(deployment)
		if canonicalErr != nil {
			return "", "", canonicalErr
		}
		if !canonical {
			return "", "", fmt.Errorf(
				"the %s deployment uses config %s and data %s; run `sentinel service migrate --scope %s` before reinstalling",
				scope,
				deployment.ConfigPath,
				deployment.DataDir,
				scope,
			)
		}
		break
	}

	if _, statErr := os.Stat(layout.ConfigPath); statErr != nil {
		if !os.IsNotExist(statErr) {
			return "", "", fmt.Errorf("stat config %s: %w", layout.ConfigPath, statErr)
		}
		if create {
			if _, initErr := config.InitPathForDeployment(layout.ConfigPath, layout.DataDir, layout.LogPath, false); initErr != nil {
				return "", "", initErr
			}
		}
	}
	cfg, resolved, err := config.LoadPathForDeployment(layout.ConfigPath, layout.DataDir, layout.LogPath)
	if err != nil {
		return "", "", err
	}
	if create {
		if err := config.EnsureDirs(cfg); err != nil {
			return "", "", err
		}
	}
	return resolved, layout.DataDir, nil
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
		canonical, canonicalErr := daemon.HasCanonicalPaths(s.Deployment)
		layout, layoutErr := daemon.LayoutForScope(s.Scope)
		if canonicalErr != nil {
			return failf("service status failed: %w", canonicalErr)
		}
		if layoutErr != nil {
			return failf("service status failed: %w", layoutErr)
		}
		rows := []outputRow{
			{Key: fmt.Sprintf("%s unit file", s.Scope), Value: s.ServicePath},
			{Key: fmt.Sprintf("%s unit exists", s.Scope), Value: fmt.Sprintf("%t", s.UnitFileExists)},
			{Key: fmt.Sprintf("%s available", managerLabel), Value: fmt.Sprintf("%t", s.SystemctlAvailable)},
			{Key: fmt.Sprintf("%s binary", s.Scope), Value: s.BinaryPath},
			{Key: fmt.Sprintf("%s config", s.Scope), Value: s.ConfigPath},
			{Key: fmt.Sprintf("%s data dir", s.Scope), Value: s.DataDir},
			{Key: fmt.Sprintf("%s canonical layout", s.Scope), Value: fmt.Sprintf("%t", canonical)},
			{Key: fmt.Sprintf("%s expected config", s.Scope), Value: layout.ConfigPath},
			{Key: fmt.Sprintf("%s expected data dir", s.Scope), Value: layout.DataDir},
			{Key: fmt.Sprintf("%s expected log", s.Scope), Value: layout.LogPath},
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
		scope  string
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
				Scope:  scope,
			}); err != nil {
				return failf("service logs failed: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "stream new log lines as they arrive")
	cmd.Flags().IntVarP(&lines, "lines", "n", 50, "number of past log lines to show")
	cmd.Flags().StringVar(&scope, "scope", optionAuto, "target deployment: auto|user|system")
	return cmd
}

// runtimeServiceManagerLabel names the service manager for the current OS.
func runtimeServiceManagerLabel() string {
	if runtime.GOOS == hostOSDarwin {
		return "launchctl"
	}
	return "systemctl"
}
