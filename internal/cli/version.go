package cli

import (
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// version is overridden at release build time via -ldflags.
var version = "dev"

// Version resolves the binary version from the ldflags value, falling back to
// the Go build info and finally "dev".
func Version() string {
	if value := strings.TrimSpace(version); value != "" && value != "dev" && value != "(devel)" {
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
