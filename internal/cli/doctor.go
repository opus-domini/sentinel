package cli

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

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
	cfg, configPath, cfgErr := loadConfigFn()
	tmuxPath, tmuxErr := exec.LookPath("tmux")
	managerLabel := runtimeServiceManagerLabel()
	managerPath, managerErr := exec.LookPath(managerLabel)
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
			rows = append(rows,
				outputRow{Key: fmt.Sprintf("%s unit file", s.Scope), Value: s.ServicePath},
				outputRow{Key: fmt.Sprintf("%s unit exists", s.Scope), Value: fmt.Sprintf("%t", s.UnitFileExists)},
			)
			if s.SystemctlAvailable {
				rows = append(rows,
					outputRow{Key: fmt.Sprintf("%s unit enabled", s.Scope), Value: s.EnabledState},
					outputRow{Key: fmt.Sprintf("%s unit active", s.Scope), Value: s.ActiveState},
				)
				if s.UnitFileExists && s.ActiveState != "active" {
					issues = append(issues, fmt.Sprintf(
						"%s service is installed but not active (state: %s)",
						s.Scope,
						s.ActiveState,
					))
				}
			}
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

func doctorListValue(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}
