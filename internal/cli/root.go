package cli

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// newRootCmd builds the Sentinel root command and wires every subcommand.
func newRootCmd(app *App) *cobra.Command {
	var configPath string
	root := &cobra.Command{
		Use:   sentinelServiceUnit,
		Short: "Sentinel command-line interface",
		Long: "Sentinel is a host operations platform: a single binary serving a\n" +
			"browser UI for tmux sessions, service monitoring, metrics and runbooks.\n" +
			"This is its CLI — start the server with `sentinel daemon`, manage the\n" +
			"local service, run diagnostics and apply binary updates.",
		Version:       currentVersionFn(),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.PersistentPreRun = func(_ *cobra.Command, _ []string) {
		if path := strings.TrimSpace(configPath); path != "" {
			_ = os.Setenv("SENTINEL_CONFIG", path)
		}
	}
	root.SetVersionTemplate("sentinel version {{.Version}}\n")
	root.InitDefaultVersionFlag()
	if f := root.Flags().Lookup("version"); f != nil {
		f.Shorthand = "v"
	}
	root.PersistentFlags().StringVar(&configPath, "config", "", "config file path")

	applyHelpStyle(root)
	addGrouped(root, groupSetup,
		newConfigCmd(app),
		newDBCmd(app),
		newDoctorCmd(app),
	)
	addGrouped(root, groupService,
		newDaemonCmd(app),
		newServiceCmd(app),
		newUpdateCmd(app),
	)
	addGrouped(root, groupExtra,
		newCompletionCmd(app),
		newVersionCmd(app),
	)
	return root
}
