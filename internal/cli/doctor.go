package cli

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

func newDoctorCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the local environment and runtime config",
		Long:  "Check the local environment and runtime configuration.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			runDoctor(app)
			return nil
		},
	}
}

func runDoctor(app *App) {
	cfg := loadConfigFn()
	tmuxPath, tmuxErr := exec.LookPath("tmux")
	managerLabel := runtimeServiceManagerLabel()
	managerPath, managerErr := exec.LookPath(managerLabel)
	status, statusErr := userStatusFn()

	printHeading(app.Stdout, "Sentinel doctor report")
	if !shouldUsePrettyOutput(app.Stdout) {
		writeln(app.Stdout, "---------------------")
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
	printRows(app.Stdout, rows)
}
