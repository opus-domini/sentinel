package cli

import (
	"context"
	"encoding/json"
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
	cmd.AddCommand(newConfigInitCmd(app), newConfigEditCmd(app), newConfigPathCmd(app), newConfigValidateCmd(app), newConfigShowCmd(app))
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

func newConfigShowCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show effective Sentinel config",
		Long:  "Show the effective Sentinel config after applying defaults, file values and environment overrides.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := loadValidatedConfig()
			if err != nil {
				return failf(1, "config show failed: %w", err)
			}
			enc := json.NewEncoder(app.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(newConfigShowOutput(cfg)); err != nil {
				return failf(1, "config show failed: %w", err)
			}
			return nil
		},
	}
}

func loadValidatedConfig() (config.Config, error) {
	configPath := config.Path()
	if _, err := os.Stat(configPath); err == nil {
		if err := config.ValidateFile(configPath); err != nil {
			return config.Config{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return config.Config{}, fmt.Errorf("stat config file: %w", err)
	}
	return config.Load(), nil
}

type configShowOutput struct {
	ListenAddr             string                    `json:"listen_addr"`
	Token                  string                    `json:"token"`
	AllowedOrigins         []string                  `json:"allowed_origins"`
	CookieSecure           string                    `json:"cookie_secure"`
	AllowInsecureCookie    bool                      `json:"allow_insecure_cookie"`
	DataDir                string                    `json:"data_dir"`
	LogLevel               string                    `json:"log_level"`
	Timezone               string                    `json:"timezone"`
	Locale                 string                    `json:"locale"`
	RunbookMaxConcurrent   int                       `json:"runbook_max_concurrent"`
	MultiUser              configShowMultiUser       `json:"multi_user"`
	SystemUsers            []string                  `json:"system_users"`
	Watchtower             configShowWatchtower      `json:"watchtower"`
	AlertThresholds        configShowAlertThresholds `json:"alert_thresholds"`
	AlertWebhookURL        string                    `json:"alert_webhook_url"`
	AlertWebhookEvents     []string                  `json:"alert_webhook_events"`
	HealthReportWebhookURL string                    `json:"health_report_webhook_url"`
	HealthReportSchedule   string                    `json:"health_report_schedule"`
}

type configShowMultiUser struct {
	AllowedUsers     []string `json:"allowed_users"`
	AllowRootTarget  bool     `json:"allow_root_target"`
	UserSwitchMethod string   `json:"user_switch_method"`
}

type configShowWatchtower struct {
	Enabled        bool   `json:"enabled"`
	TickInterval   string `json:"tick_interval"`
	CaptureLines   int    `json:"capture_lines"`
	CaptureTimeout string `json:"capture_timeout"`
	JournalRows    int    `json:"journal_rows"`
}

type configShowAlertThresholds struct {
	CPUPercent  float64 `json:"cpu_percent"`
	MemPercent  float64 `json:"mem_percent"`
	DiskPercent float64 `json:"disk_percent"`
}

func newConfigShowOutput(cfg config.Config) configShowOutput {
	return configShowOutput{
		ListenAddr:           cfg.ListenAddr,
		Token:                cfg.Token,
		AllowedOrigins:       nonNilStrings(cfg.AllowedOrigins),
		CookieSecure:         cfg.CookieSecure,
		AllowInsecureCookie:  cfg.AllowInsecureCookie,
		DataDir:              cfg.DataDir,
		LogLevel:             cfg.LogLevel,
		Timezone:             cfg.Timezone,
		Locale:               cfg.Locale,
		RunbookMaxConcurrent: cfg.RunbookMaxConcurrent,
		MultiUser: configShowMultiUser{
			AllowedUsers:     nonNilStrings(cfg.MultiUser.AllowedUsers),
			AllowRootTarget:  cfg.MultiUser.AllowRootTarget,
			UserSwitchMethod: cfg.MultiUser.UserSwitchMethod,
		},
		SystemUsers: nonNilStrings(cfg.SystemUsers),
		Watchtower: configShowWatchtower{
			Enabled:        cfg.Watchtower.Enabled,
			TickInterval:   cfg.Watchtower.TickInterval.String(),
			CaptureLines:   cfg.Watchtower.CaptureLines,
			CaptureTimeout: cfg.Watchtower.CaptureTimeout.String(),
			JournalRows:    cfg.Watchtower.JournalRows,
		},
		AlertThresholds: configShowAlertThresholds{
			CPUPercent:  cfg.AlertThresholds.CPUPercent,
			MemPercent:  cfg.AlertThresholds.MemPercent,
			DiskPercent: cfg.AlertThresholds.DiskPercent,
		},
		AlertWebhookURL:        cfg.AlertWebhookURL,
		AlertWebhookEvents:     nonNilStrings(cfg.AlertWebhookEvents),
		HealthReportWebhookURL: cfg.HealthReportWebhookURL,
		HealthReportSchedule:   cfg.HealthReportSchedule,
	}
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
