package tmux

import (
	"context"
	"os"
	"runtime"
)

var createSessionRun = func(ctx context.Context, args ...string) (string, error) {
	name, commandArgs := buildCreateSessionCommand(runtime.GOOS, os.Geteuid(), args)
	return executeTmuxCommand(ctx, name, commandArgs, args)
}

// buildCreateSessionCommand isolates the command that may create the tmux
// server from Sentinel's systemd unit. tmux 3.7 links pane scopes to the unit
// containing its server process, so starting the server directly from
// sentinel.service would make every pane stop with Sentinel.
func buildCreateSessionCommand(goos string, euid int, tmuxArgs []string) (string, []string) {
	if goos != "linux" {
		return "tmux", append([]string(nil), tmuxArgs...)
	}

	args := make([]string, 0, len(tmuxArgs)+7)
	if euid != 0 {
		args = append(args, "--user")
	}
	args = append(args, "--scope", "--collect", "--quiet", "--", "tmux")
	args = append(args, tmuxArgs...)
	return "systemd-run", args
}
