package cli

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/daemon"
	"github.com/spf13/cobra"
)

var doctorLookPath = exec.LookPath

func newDoctorCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the local environment and runtime config",
		Long:  "Check the local environment and runtime configuration.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runDoctor(app)
		},
	}
}

func runDoctor(app *App) error {
	deployments, deploymentsErr := installedDeploymentsFn()
	var (
		cfg                 config.Config
		configPath          string
		cfgErr              error
		deploymentAccessErr error
	)
	if deploymentsErr != nil {
		cfg = config.Default()
		configPath = "unavailable"
	} else {
		switch len(deployments) {
		case 0:
			cfg, configPath, cfgErr = loadConfigFn()
		case 1:
			deployment := deployments[0]
			configPath = deployment.ConfigPath
			deploymentAccessErr = requireScopeAccessFn(deployment.Scope)
			if deploymentAccessErr == nil {
				cfg, configPath, cfgErr = loadConfigPathFn(deployment.ConfigPath, deployment.DataDir)
			} else {
				cfg = config.DefaultForDataDir(deployment.DataDir)
			}
		default:
			cfg = config.Default()
			configPath = "ambiguous: select one deployment"
			deploymentAccessErr = daemon.ErrAmbiguousDeployment
		}
	}
	tmuxPath, tmuxErr := doctorLookPath("tmux")
	managerLabel := runtimeServiceManagerLabel()
	managerPath, managerErr := doctorLookPath(managerLabel)
	report, statusErr := serviceStatusFn()

	printHeading(app.Stdout, "Sentinel doctor report")
	if !shouldUsePrettyOutput(app.Stdout) {
		writeln(app.Stdout, "---------------------")
	}
	rows := []outputRow{
		{Key: "os", Value: fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)},
		{Key: "supported host", Value: fmt.Sprintf("%t", runtime.GOOS == "linux" || runtime.GOOS == hostOSDarwin)},
		{Key: "config path", Value: configPath},
		{Key: "listen", Value: cfg.Address()},
		{Key: "data dir", Value: cfg.DataDir()},
		{Key: "token required", Value: fmt.Sprintf("%t", cfg.Server.Token != "")},
		{Key: "allowed origins", Value: doctorListValue(cfg.Server.AllowedOrigins)},
		{Key: "trusted proxies", Value: doctorListValue(cfg.Server.TrustedProxies)},
	}
	var issues []string
	if deploymentsErr != nil {
		issues = append(issues, fmt.Sprintf("deployment discovery failed: %v", deploymentsErr))
	}
	if deploymentAccessErr != nil {
		if errors.Is(deploymentAccessErr, daemon.ErrAmbiguousDeployment) {
			issues = append(issues, "Sentinel is installed in both user and system scope; remove one deployment or inspect each with an explicit scope")
		} else {
			issues = append(issues, deploymentAccessErr.Error()+"; run `sudo sentinel doctor` to inspect the system config")
		}
	}
	if runtime.GOOS != "linux" && runtime.GOOS != hostOSDarwin {
		issues = append(issues, fmt.Sprintf("host operating system %q is not supported", runtime.GOOS))
	}
	if cfgErr != nil {
		rows = append(rows, outputRow{Key: cmdConfig, Value: fmt.Sprintf("invalid: %v", cfgErr)})
		issues = append(issues, fmt.Sprintf("configuration is invalid: %v", cfgErr))
	} else {
		rows = append(rows, outputRow{Key: cmdConfig, Value: "valid"})
	}
	if tmuxErr == nil {
		rows = append(rows, outputRow{Key: "tmux", Value: tmuxPath})
	} else {
		rows = append(rows, outputRow{Key: "tmux", Value: "not found"})
		issues = append(issues, "tmux binary was not found in PATH")
	}
	if managerErr == nil {
		rows = append(rows, outputRow{Key: managerLabel, Value: managerPath})
	} else {
		rows = append(rows, outputRow{Key: managerLabel, Value: "not found"})
		issues = append(issues, managerLabel+" was not found in PATH")
	}
	switch {
	case statusErr != nil:
		rows = append(rows, outputRow{Key: "service status", Value: fmt.Sprintf("unavailable (%v)", statusErr)})
		issues = append(issues, fmt.Sprintf("service status is unavailable: %v", statusErr))
	case len(report) == 0:
		rows = append(rows, outputRow{Key: "service", Value: "not installed"})
	default:
		for _, s := range report {
			canonical, canonicalErr := daemon.HasCanonicalPaths(s.Deployment)
			if canonicalErr != nil {
				issues = append(issues, fmt.Sprintf("inspect %s deployment layout: %v", s.Scope, canonicalErr))
			}
			rows = append(rows,
				outputRow{Key: fmt.Sprintf("%s unit file", s.Scope), Value: s.ServicePath},
				outputRow{Key: fmt.Sprintf("%s unit exists", s.Scope), Value: fmt.Sprintf("%t", s.UnitFileExists)},
				outputRow{Key: fmt.Sprintf("%s binary", s.Scope), Value: s.BinaryPath},
				outputRow{Key: fmt.Sprintf("%s config", s.Scope), Value: s.ConfigPath},
				outputRow{Key: fmt.Sprintf("%s data dir", s.Scope), Value: s.DataDir},
				outputRow{Key: fmt.Sprintf("%s canonical layout", s.Scope), Value: fmt.Sprintf("%t", canonical)},
			)
			if canonicalErr == nil && !canonical {
				layout, layoutErr := daemon.LayoutForScope(s.Scope)
				if layoutErr != nil {
					issues = append(issues, fmt.Sprintf("resolve %s canonical layout: %v", s.Scope, layoutErr))
				} else {
					issues = append(issues, fmt.Sprintf(
						"%s deployment uses noncanonical paths; run `sentinel service migrate --scope %s` (expected config %s, data %s, log %s)",
						s.Scope,
						s.Scope,
						layout.ConfigPath,
						layout.DataDir,
						layout.LogPath,
					))
				}
			}
			if s.SystemctlAvailable {
				rows = append(rows,
					outputRow{Key: fmt.Sprintf("%s unit enabled", s.Scope), Value: s.EnabledState},
					outputRow{Key: fmt.Sprintf("%s unit active", s.Scope), Value: s.ActiveState},
				)
				if s.UnitFileExists && s.ActiveState != stateActive {
					issues = append(issues, fmt.Sprintf(
						"%s service is installed but not active (state: %s)",
						s.Scope,
						s.ActiveState,
					))
				}
			}
			autoUpdateRows, autoUpdateIssues := inspectDoctorAutoUpdate(s.Scope)
			rows = append(rows, autoUpdateRows...)
			issues = append(issues, autoUpdateIssues...)
		}
	}
	printRows(app.Stdout, rows)
	if len(issues) == 0 {
		done(app.Stdout, "ok:", "no problems found")
		return nil
	}

	printHeading(app.Stdout, "Problems found")
	problemRows := make([]outputRow, 0, len(issues))
	for index, issue := range issues {
		problemRows = append(problemRows, outputRow{
			Key:   fmt.Sprintf("problem %d", index+1),
			Value: issue,
		})
	}
	printRows(app.Stdout, problemRows)
	return failf("doctor found %d problem(s)", len(issues))
}

func inspectDoctorAutoUpdate(scope string) ([]outputRow, []string) {
	status, err := userAutoUpdateStatusFn(scope)
	if err != nil {
		return nil, []string{fmt.Sprintf("inspect %s autoupdate: %v", scope, err)}
	}
	if !status.ServiceUnitExists && !status.TimerUnitExists {
		return nil, nil
	}

	rows := []outputRow{
		{Key: fmt.Sprintf("%s autoupdate service exists", scope), Value: fmt.Sprintf("%t", status.ServiceUnitExists)},
		{Key: fmt.Sprintf("%s autoupdate timer exists", scope), Value: fmt.Sprintf("%t", status.TimerUnitExists)},
	}
	var issues []string
	if status.ServiceUnitExists != status.TimerUnitExists {
		issues = append(issues, fmt.Sprintf("%s autoupdate installation is incomplete", scope))
	}
	if !status.SystemctlAvailable {
		return rows, issues
	}

	rows = append(rows,
		outputRow{Key: fmt.Sprintf("%s autoupdate timer enabled", scope), Value: status.TimerEnabledState},
		outputRow{Key: fmt.Sprintf("%s autoupdate timer active", scope), Value: status.TimerActiveState},
		outputRow{Key: fmt.Sprintf("%s autoupdate last run", scope), Value: status.LastRunState},
	)
	if !strings.EqualFold(status.LastRunState, "failed") {
		return rows, issues
	}

	command := fmt.Sprintf("sentinel service install --scope %s", scope)
	if scope == daemon.ScopeSystem {
		command = "sudo " + command
	}
	issues = append(issues, fmt.Sprintf(
		"%s autoupdate last run failed; refresh it with `%s`",
		scope,
		command,
	))
	return rows, issues
}

func doctorListValue(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}
