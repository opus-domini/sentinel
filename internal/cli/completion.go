package cli

import (
	"embed"
	"flag"
	"io"
	"strings"
)

//go:embed completions
var completionScripts embed.FS

func runCompletionCommand(ctx commandContext, args []string) int {
	fs := flag.NewFlagSet("completion", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	help := fs.Bool("help", false, "show help")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *help {
		printCompletionHelp(ctx.stdout)
		return 0
	}
	if fs.NArg() != 1 {
		writef(ctx.stderr, "completion requires exactly one shell argument\n\n")
		printCompletionHelp(ctx.stderr)
		return 2
	}

	shell := strings.ToLower(strings.TrimSpace(fs.Arg(0)))
	var path string
	switch shell {
	case "bash":
		path = "completions/sentinel.bash"
	case "zsh":
		path = "completions/sentinel.zsh"
	case "fish":
		path = "completions/sentinel.fish"
	default:
		writef(ctx.stderr, "unsupported shell: %s\n\n", fs.Arg(0))
		printCompletionHelp(ctx.stderr)
		return 2
	}

	script, err := completionScripts.ReadFile(path)
	if err != nil {
		writef(ctx.stderr, "completion script unavailable: %v\n", err)
		return 1
	}
	if _, err := ctx.stdout.Write(script); err != nil {
		writef(ctx.stderr, "failed to write completion script: %v\n", err)
		return 1
	}
	return 0
}

func printCompletionHelp(w io.Writer) {
	printHelp(w, helpDoc{
		summary: "Print a shell completion script to stdout.",
		usage:   []string{"sentinel completion <bash|zsh|fish>"},
		notes: []string{
			"To install it:\n" +
				"  bash:  sentinel completion bash > ~/.local/share/bash-completion/completions/sentinel\n" +
				"  zsh:   sentinel completion zsh  > \"${fpath[1]}/_sentinel\"\n" +
				"  fish:  sentinel completion fish > ~/.config/fish/completions/sentinel.fish",
		},
	})
}
