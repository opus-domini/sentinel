package cli

import (
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// buildVersion is injected by release workflows via -ldflags.
var buildVersion = "dev"

// currentVersion resolves the binary version from the ldflags value, falling
// back to the Go build info and finally "dev".
func currentVersion() string {
	if value := strings.TrimSpace(buildVersion); value != "" && value != "dev" && value != "(devel)" {
		return value
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if strings.TrimSpace(bi.Main.Version) != "" && bi.Main.Version != "(devel)" {
			return bi.Main.Version
		}
	}
	return "dev"
}

func newVersionCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			writef(app.Stdout, "sentinel version %s\n", currentVersionFn())
			return nil
		},
	}
}
