package term

// Keep tmux web clients isolated so opening the same session on desktop and
// mobile does not couple active pane selection or force the smallest client
// size onto every other attached browser.
const tmuxAttachClientFlags = "active-pane,ignore-size"

func tmuxAttachArgs(session string) []string {
	return []string{"attach", "-f", tmuxAttachClientFlags, "-t", session}
}
