package client

import "embed"

// DistFS embeds the compiled frontend assets.
//
//go:embed dist
var DistFS embed.FS
