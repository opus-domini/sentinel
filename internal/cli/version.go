package cli

import (
	"text/template"

	"github.com/spf13/cobra"

	"github.com/opus-domini/sentinel/pkg/sentinel"
)

// Version returns the build version resolved by the shared sentinel package.
func Version() string {
	return sentinel.Version()
}

func newVersionCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Render the root's version template (the same string the
			// --version flag uses) so the `version` subcommand and the flag
			// always produce identical output.
			root := cmd.Root()
			tmpl, err := template.New("version").Parse(root.VersionTemplate())
			if err != nil {
				return failf("parse version template: %w", err)
			}
			return tmpl.Execute(app.Stdout, root)
		},
	}
}
