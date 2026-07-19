package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/opus-domini/sentinel/internal/daemon"
)

// defaultAutoUpdateScope is the default restart-manager scope for the
// autoupdate timer commands.
const defaultAutoUpdateScope = optionAuto

func newServiceAutoUpdateCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "autoupdate",
		Short: "Manage the automatic update timer",
		Long:  "Manage the Sentinel automatic update timer.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newServiceAutoUpdateInstallCmd(app),
		newServiceAutoUpdateUninstallCmd(app),
		newServiceAutoUpdateStatusCmd(app),
	)
	return cmd
}

func newServiceAutoUpdateInstallCmd(app *App) *cobra.Command {
	var (
		execPath        string
		enable          bool
		start           bool
		serviceUnit     string
		scope           string
		onCalendar      string
		randomizedDelay time.Duration
	)
	cmd := &cobra.Command{
		Use:   cmdInstall,
		Short: "Install the autoupdate timer and start it",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			deployment, err := resolveDeploymentFn(scope)
			if err != nil {
				return failf("service autoupdate install failed: %w", err)
			}
			if err := requireScopeAccessFn(deployment.Scope); err != nil {
				return failf("service autoupdate install failed: %w", err)
			}
			canonical, err := daemon.HasCanonicalPaths(deployment)
			if err != nil {
				return failf("service autoupdate install failed: %w", err)
			}
			if !canonical {
				return failf(
					"service autoupdate install failed: the %s deployment uses noncanonical paths; run `sentinel service migrate --scope %s` first",
					deployment.Scope,
					deployment.Scope,
				)
			}
			if requested := strings.TrimSpace(execPath); requested != "" && requested != deployment.BinaryPath {
				return failf("service autoupdate install failed: --exec does not match deployment binary %s", deployment.BinaryPath)
			}
			if err := installUserAutoUpdateFn(daemon.InstallUserAutoUpdateOptions{
				ExecPath:        deployment.BinaryPath,
				ConfigPath:      deployment.ConfigPath,
				DataDir:         deployment.DataDir,
				Enable:          enable,
				Start:           start,
				ServiceUnit:     sentinelServiceUnit,
				SystemdScope:    deployment.Scope,
				OnCalendar:      strings.TrimSpace(onCalendar),
				RandomizedDelay: randomizedDelay,
			}); err != nil {
				return failf("service autoupdate install failed: %w", err)
			}

			if timerPath, err := daemon.UserAutoUpdateTimerPathForScope(deployment.Scope); err == nil {
				writef(app.Stdout, "autoupdate timer installed: %s\n", timerPath)
			}
			switch {
			case enable && start:
				writeln(app.Stdout, "autoupdate timer enabled and started")
			case enable:
				writeln(app.Stdout, "autoupdate timer enabled")
			case start:
				writeln(app.Stdout, "autoupdate timer started")
			default:
				writeln(app.Stdout, "autoupdate timer installed (not enabled, not started)")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&execPath, "exec", "", "binary path for the updater unit (default: current executable)")
	cmd.Flags().BoolVar(&enable, "enable", true, "enable the autoupdate timer at startup")
	cmd.Flags().BoolVar(&start, "start", true, "start the autoupdate timer immediately")
	cmd.Flags().StringVar(&serviceUnit, "service", "sentinel", "service unit/label to restart after an update")
	cmd.Flags().StringVar(&scope, "scope", defaultAutoUpdateScope, "target deployment: auto|user|system")
	cmd.Flags().StringVar(&onCalendar, "on-calendar", "daily", "update schedule: daily|hourly|weekly|duration|seconds")
	cmd.Flags().DurationVar(&randomizedDelay, "randomized-delay", time.Hour, "randomized delay before updating (systemd only)")
	_ = cmd.Flags().MarkHidden("exec")
	_ = cmd.Flags().MarkHidden("service")
	return cmd
}

func newServiceAutoUpdateUninstallCmd(app *App) *cobra.Command {
	var (
		disable    bool
		stop       bool
		removeUnit bool
		scope      string
	)
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Stop the autoupdate timer and remove its units",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			deployment, err := resolveDeploymentFn(scope)
			if err != nil {
				return failf("service autoupdate uninstall failed: %w", err)
			}
			if err := requireScopeAccessFn(deployment.Scope); err != nil {
				return failf("service autoupdate uninstall failed: %w", err)
			}
			if err := uninstallUserAutoUpdateFn(daemon.UninstallUserAutoUpdateOptions{
				Disable:    disable,
				Stop:       stop,
				RemoveUnit: removeUnit,
				Scope:      deployment.Scope,
			}); err != nil {
				return failf("service autoupdate uninstall failed: %w", err)
			}
			writeln(app.Stdout, "autoupdate timer uninstalled")
			return nil
		},
	}
	cmd.Flags().BoolVar(&disable, "disable", true, "disable the autoupdate timer from auto-start")
	cmd.Flags().BoolVar(&stop, "stop", true, "stop the running autoupdate timer")
	cmd.Flags().BoolVar(&removeUnit, "remove-unit", true, "remove the autoupdate unit files")
	cmd.Flags().StringVar(&scope, "scope", defaultAutoUpdateScope, "target deployment: auto|user|system")
	return cmd
}

func newServiceAutoUpdateStatusCmd(app *App) *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   cmdStatus,
		Short: "Show the autoupdate timer status",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			deployment, err := resolveDeploymentFn(scope)
			if err != nil {
				return failf("service autoupdate status failed: %w", err)
			}
			if err := requireScopeAccessFn(deployment.Scope); err != nil {
				return failf("service autoupdate status failed: %w", err)
			}
			status, err := userAutoUpdateStatusFn(deployment.Scope)
			if err != nil {
				return failf("service autoupdate status failed: %w", err)
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
			printRows(app.Stdout, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&scope, "scope", defaultAutoUpdateScope, "target deployment: auto|user|system")
	return cmd
}
