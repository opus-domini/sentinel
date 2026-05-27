package cli

import "github.com/spf13/cobra"

// newRootCmd builds the Sentinel root command and wires every subcommand.
func newRootCmd(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:   "sentinel",
		Short: "Sentinel command-line interface",
		Long: "Sentinel is a host operations platform: a single binary serving a\n" +
			"browser UI for tmux sessions, service monitoring, alerts and recovery.\n" +
			"This is its CLI — start the server with `sentinel daemon`, manage the\n" +
			"local service, run diagnostics and apply binary updates.",
		Version:       currentVersionFn(),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.SetVersionTemplate("sentinel version {{.Version}}\n")
	root.InitDefaultVersionFlag()
	if f := root.Flags().Lookup("version"); f != nil {
		f.Shorthand = "v"
	}

	applyHelpStyle(root)
	addGrouped(root, groupSetup,
		newConfigCmd(app),
		newDBCmd(app),
		newDoctorCmd(app),
	)
	addGrouped(root, groupCore,
		newDaemonCmd(app),
		newServiceCmd(app),
		newUpdateCmd(app),
	)
	addGrouped(root, groupExtra,
		newVersionCmd(app),
	)
	return root
}
