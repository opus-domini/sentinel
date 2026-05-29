package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newCompletionCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish]",
		Short: "Generate shell completion scripts",
		Long:  "Generate shell completion scripts for bash, zsh or fish.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(app.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(app.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(app.Stdout, true)
			default:
				return failf("unsupported shell %q", args[0])
			}
		},
	}

	install := &cobra.Command{
		Use:   "install",
		Short: "Install shell completion scripts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			shell, _ := cmd.Flags().GetString("shell")
			return installCompletions(cmd.Root(), app, shell)
		},
	}
	install.Flags().String("shell", "auto", "shell to install completion for (auto|bash|zsh|fish|all)")
	cmd.AddCommand(install)

	return cmd
}

func installCompletions(root *cobra.Command, app *App, shell string) error {
	shells, err := completionShells(shell)
	if err != nil {
		return err
	}
	for _, sh := range shells {
		path, err := completionPath(sh)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return failf("create completion directory: %w", err)
		}
		file, err := os.Create(path)
		if err != nil {
			return failf("create completion script: %w", err)
		}
		var genErr error
		switch sh {
		case "bash":
			genErr = root.GenBashCompletion(file)
		case "zsh":
			genErr = root.GenZshCompletion(file)
		case "fish":
			genErr = root.GenFishCompletion(file, true)
		}
		closeErr := file.Close()
		if genErr != nil {
			return failf("generate %s completion: %w", sh, genErr)
		}
		if closeErr != nil {
			return failf("write completion script: %w", closeErr)
		}
		writef(app.Stdout, "%s completion installed to %s\n", sh, path)
	}
	return nil
}

func completionShells(shell string) ([]string, error) {
	normalized := strings.ToLower(strings.TrimSpace(shell))
	switch normalized {
	case "auto", "":
		return []string{detectShell()}, nil
	case "bash", "zsh", "fish":
		return []string{normalized}, nil
	case "all":
		return []string{"bash", "zsh", "fish"}, nil
	default:
		return nil, failf("unsupported shell %q", shell)
	}
}

func detectShell() string {
	if sh := shellName(os.Getenv("SHELL")); sh != "" {
		return sh
	}
	if comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", os.Getppid())); err == nil {
		if sh := shellName(string(comm)); sh != "" {
			return sh
		}
	}
	return "bash"
}

func shellName(value string) string {
	name := strings.TrimPrefix(filepath.Base(strings.TrimSpace(value)), "-")
	switch name {
	case "bash", "zsh", "fish":
		return name
	default:
		return ""
	}
}

func completionPath(shell string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", failf("resolve home directory: %w", err)
	}
	switch shell {
	case "bash":
		return filepath.Join(home, ".local", "share", "bash-completion", "completions", "sentinel"), nil
	case "zsh":
		return filepath.Join(home, ".local", "share", "zsh", "site-functions", "_sentinel"), nil
	case "fish":
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			configHome = filepath.Join(home, ".config")
		}
		return filepath.Join(configHome, "fish", "completions", "sentinel.fish"), nil
	default:
		return "", failf("unsupported shell %q", shell)
	}
}
