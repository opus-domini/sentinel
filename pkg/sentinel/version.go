// Package sentinel exposes the shared build version for the Sentinel binaries.
//
// The version is injected at build time via -ldflags
// (-X github.com/opus-domini/sentinel/pkg/sentinel.version=...) by both the Makefile and GoReleaser,
// so it lives in an exported package rather than internal/cli.
package sentinel

import (
	"runtime/debug"
	"strings"
)

// version is overridden at build time via -ldflags.
var version = "dev"

// Version returns the build version. It prefers the ldflags-injected value,
// then falls back to the module build info, and finally to "dev".
func Version() string {
	if v := strings.TrimSpace(version); v != "" && v != "dev" && v != "(devel)" {
		return v
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := strings.TrimSpace(bi.Main.Version); v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}
