package client

import "embed"

// DistFS embeds the compiled frontend assets.
//
//go:embed dist
var DistFS embed.FS

// PublicFS embeds tracked public assets that exist before a frontend build.
//
//go:embed public
var PublicFS embed.FS
