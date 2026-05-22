package ui

import "embed"

// DistFS embeds the compiled React SPA. It is produced by the frontend build
// into internal/ui/dist before Go builds in Makefile, CI and GoReleaser.
// The all: prefix keeps the package buildable when only .gitkeep exists.
//
//go:embed all:dist
var DistFS embed.FS
