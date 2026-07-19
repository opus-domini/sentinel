package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/opus-domini/sentinel/internal/daemon"
)

func newServiceResolveInstallScopeCmd(app *App) *cobra.Command {
	var (
		scope       string
		interactive bool
	)
	cmd := &cobra.Command{
		Use:    "resolve-install-scope",
		Short:  "Resolve installation scope for the supported installers",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			resolved, err := resolveInstallerScope(app, scope, interactive)
			if err != nil {
				return failf("resolve install scope failed: %w", err)
			}
			writef(app.Stdout, "%s\n", resolved)
			return nil
		},
	}
	cmd.Flags().StringVar(&scope, "scope", optionAuto, "installation scope: auto|user|system")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "ask for scope when no installation exists")
	return cmd
}

func resolveInstallerScope(app *App, scope string, interactive bool) (string, error) {
	resolved, err := resolveInstallScopeFn(scope)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, daemon.ErrInstallScopeRequired) {
		return "", err
	}
	if !interactive {
		return "", fmt.Errorf("%w; set INSTALL_SCOPE=user or INSTALL_SCOPE=system", err)
	}
	if app.Stdin == nil {
		return "", errors.New("interactive installation requires a terminal input")
	}
	return promptInstallScope(app.Stdin, app.Stderr)
}

func promptInstallScope(in io.Reader, out io.Writer) (string, error) {
	printHeading(out, "Choose how Sentinel should be installed")
	writeln(out)
	writef(out, "  %s %s\n", renderStyle(out, styleBold, "1) User installation"), renderStyle(out, styleSuccess, "recommended for personal workstations"))
	writeln(out, "     Binary: ~/.local/bin/sentinel")
	writeln(out, "     Config, data and service belong to the current user; no sudo required.")
	writeln(out)
	writef(out, "  %s\n", renderStyle(out, styleBold, "2) System installation"))
	writeln(out, "     Binary: /usr/local/bin/sentinel")
	writeln(out, "     Machine-wide service with system config, data and logs; requires sudo.")
	writeln(out)

	scanner := bufio.NewScanner(in)
	for {
		writef(out, "%s", renderStyle(out, styleBold, "Select [1/2]: "))
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", fmt.Errorf("read installation scope: %w", err)
			}
			return "", errors.New("installation scope selection was canceled")
		}
		switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
		case "1", optionUser:
			return daemon.ScopeUser, nil
		case "2", optionSystem:
			return daemon.ScopeSystem, nil
		default:
			writeln(out, renderStyle(out, styleWarning, "Enter 1 for user or 2 for system."))
		}
	}
}
