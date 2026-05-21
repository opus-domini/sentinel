package cli

import "github.com/spf13/cobra"

func newDaemonCmd(_ *App) *cobra.Command {
	return &cobra.Command{
		Use:   "daemon",
		Short: "Start the Sentinel server",
		Long:  "Start the Sentinel server using the config file and environment defaults.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			// The server logs its own failures via slog; carry the exit
			// code out without printing a second message.
			if code := daemonFn(); code != 0 {
				return exitError{code: code}
			}
			return nil
		},
	}
}
