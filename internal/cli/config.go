package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/spf13/cobra"
)

type configEditor struct {
	Label string
	Cmd   string
	Args  []string
	Waits bool
}

var (
	lookupExec  = exec.LookPath
	execCommand = exec.CommandContext
)

func newConfigInitCmd(app *App) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the Sentinel config file",
		Long:  "Initialize the canonical Sentinel config file.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			path, err := config.Init(force)
			if err != nil {
				if errors.Is(err, config.ErrConfigExists) {
					return failf(1, "%v (use --force to overwrite)", err)
				}
				return failf(1, "config init failed: %w", err)
			}
			reportHeader(app.Stdout, "config", "initialization")
			if force {
				done(app.Stdout, "overwrote", path)
			} else {
				done(app.Stdout, "wrote", path)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing config file")
	return cmd
}

func newConfigCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Initialize, edit, inspect and validate Sentinel config",
		Long:  "Initialize, edit, inspect and validate the canonical Sentinel config file.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newConfigInitCmd(app), newConfigEditCmd(app), newConfigPathCmd(app), newConfigValidateCmd(app))
	return cmd
}

func newConfigEditCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open the Sentinel config file in your editor",
		Long: "Ensures the canonical config.toml exists, then opens it with $EDITOR,\n" +
			"$VISUAL, or xdg-open. Blocking editors are validated after they close.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigEdit(cmd.Context(), app)
		},
	}
}

func runConfigEdit(ctx context.Context, app *App) error {
	path := config.Path()
	editor, err := resolveConfigEditor()
	if err != nil {
		return failf(1, "config edit failed: %w", err)
	}
	if _, err := config.Init(false); err == nil {
		done(app.Stdout, "wrote", path)
	} else if !errors.Is(err, config.ErrConfigExists) {
		return failf(1, "config init failed: %w", err)
	}
	if err := runResolvedConfigEditor(ctx, app, editor, path); err != nil {
		return failf(1, "config edit failed: %w", err)
	}
	if !editor.Waits {
		done(app.Stdout, "opened", path)
		empty(app.Stdout, "Run `sentinel config validate` after saving.")
		return nil
	}
	if err := config.ValidateFile(path); err != nil {
		return failf(1, "config validation failed: %w", err)
	}
	done(app.Stdout, "ok:", path+" - config valid")
	return nil
}

func resolveConfigEditor() (configEditor, error) {
	if editor := strings.TrimSpace(os.Getenv("EDITOR")); editor != "" {
		return configEditor{
			Label: editor,
			Cmd:   "sh",
			Args:  []string{"-c", editor + ` "$1"`, "sentinel-config-edit"},
			Waits: true,
		}, nil
	}
	if editor := strings.TrimSpace(os.Getenv("VISUAL")); editor != "" {
		return configEditor{
			Label: editor,
			Cmd:   "sh",
			Args:  []string{"-c", editor + ` "$1"`, "sentinel-config-edit"},
			Waits: true,
		}, nil
	}
	if _, err := lookupExec("xdg-open"); err == nil {
		return configEditor{
			Label: "xdg-open",
			Cmd:   "xdg-open",
			Waits: false,
		}, nil
	}
	return configEditor{}, fmt.Errorf("no editor found: set $EDITOR or $VISUAL, or install xdg-open")
}

func runResolvedConfigEditor(ctx context.Context, app *App, editor configEditor, path string) error {
	args := append([]string(nil), editor.Args...)
	args = append(args, path)
	cmd := execCommand(ctx, editor.Cmd, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = app.Stdout
	cmd.Stderr = app.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("open %s with %s: %w", path, editor.Label, err)
	}
	return nil
}

func newConfigPathCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the Sentinel config path",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			writeln(app.Stdout, config.Path())
			return nil
		},
	}
}

func newConfigValidateCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate the Sentinel config file",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			path := config.Path()
			if err := config.ValidateFile(path); err != nil {
				return failf(1, "config validation failed: %w", err)
			}
			done(app.Stdout, "ok:", path+" - config valid")
			return nil
		},
	}
}
