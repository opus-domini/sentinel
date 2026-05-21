// Command sentinel is the Sentinel host-operations platform: a single binary
// that serves a web UI and exposes service, update and diagnostics commands.
// All logic lives in internal packages; this entrypoint only wires args and
// the process exit code.
package main

import (
	"os"

	"github.com/opus-domini/sentinel/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
