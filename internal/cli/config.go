package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/daemon"
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

type configTarget struct {
	path    string
	dataDir string
	logPath string
}

func newConfigInitCmd(app *App, scope *string) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the Sentinel config file",
		Long:  "Initialize the canonical Sentinel config file.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			target, err := resolveConfigTarget(*scope)
			if err != nil {
				return failf("config init failed: %w", err)
			}
			path, err := config.InitPathForDeployment(target.path, target.dataDir, target.logPath, force)
			if err != nil {
				if errors.Is(err, config.ErrConfigExists) {
					return failf("%v (use --force to overwrite)", err)
				}
				return failf("config init failed: %w", err)
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
	var scope string
	cmd := &cobra.Command{
		Use:   cmdConfig,
		Short: "Initialize, edit, inspect and validate Sentinel config",
		Long:  "Initialize, edit, inspect and validate the canonical Sentinel config file.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.PersistentFlags().StringVar(&scope, "scope", optionAuto, "target deployment: auto|user|system")
	cmd.AddCommand(
		newConfigInitCmd(app, &scope),
		newConfigEditCmd(app, &scope),
		newConfigPathCmd(app, &scope),
		newConfigValidateCmd(app, &scope),
		newConfigShowCmd(app, &scope),
	)
	return cmd
}

func newConfigEditCmd(app *App, scope *string) *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open the Sentinel config file in your editor",
		Long: "Ensures the canonical config.toml exists, then opens it with $EDITOR,\n" +
			"$VISUAL, or xdg-open. Blocking editors are validated after they close.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigEdit(cmd.Context(), app, *scope)
		},
	}
}

func runConfigEdit(ctx context.Context, app *App, scope string) error {
	target, err := resolveConfigTarget(scope)
	if err != nil {
		return failf("config edit failed: %w", err)
	}
	path := target.path
	editor, err := resolveConfigEditor()
	if err != nil {
		return failf("config edit failed: %w", err)
	}
	if _, err := config.InitPathForDeployment(path, target.dataDir, target.logPath, false); err == nil {
		done(app.Stdout, "wrote", path)
	} else if !errors.Is(err, config.ErrConfigExists) {
		return failf("config init failed: %w", err)
	}
	if err := runResolvedConfigEditor(ctx, app, editor, path); err != nil {
		return failf("config edit failed: %w", err)
	}
	if !editor.Waits {
		done(app.Stdout, "opened", path)
		empty(app.Stdout, "Run `sentinel config validate` after saving.")
		return nil
	}
	if err := config.ValidateFile(path); err != nil {
		return failf("config validation failed: %w", err)
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

func newConfigPathCmd(app *App, scope *string) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the Sentinel config path",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			target, err := resolveConfigTarget(*scope)
			if err != nil {
				return failf("config path failed: %w", err)
			}
			writeln(app.Stdout, target.path)
			return nil
		},
	}
}

func newConfigValidateCmd(app *App, scope *string) *cobra.Command {
	var effective bool
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the Sentinel config file",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			target, err := resolveConfigTarget(*scope)
			if err != nil {
				return failf("config validation failed: %w", err)
			}
			path := target.path
			if effective {
				_, path, err = config.LoadPathForDeployment(target.path, target.dataDir, target.logPath)
			} else {
				err = config.ValidateFile(path)
			}
			if err != nil {
				return failf("config validation failed: %w", err)
			}
			done(app.Stdout, "ok:", path+" - config valid")
			return nil
		},
	}
	cmd.Flags().BoolVar(&effective, "effective", false, "validate effective config including environment overrides")
	return cmd
}

func newConfigShowCmd(app *App, scope *string) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show effective Sentinel config",
		Long:  "Show the effective Sentinel config after applying defaults, file values and environment overrides.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			target, err := resolveConfigTarget(*scope)
			if err != nil {
				return failf("config show failed: %w", err)
			}
			cfg, err := loadValidatedConfigTarget(target)
			if err != nil {
				return failf("config show failed: %w", err)
			}
			enc := json.NewEncoder(app.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(newConfigShowOutput(cfg)); err != nil {
				return failf("config show failed: %w", err)
			}
			return nil
		},
	}
}

func loadValidatedConfig() (config.Config, error) {
	target, err := resolveConfigTarget(optionAuto)
	if err != nil {
		return config.Config{}, err
	}
	return loadValidatedConfigTarget(target)
}

func loadValidatedConfigTarget(target configTarget) (config.Config, error) {
	cfg, _, err := config.LoadPathForDeployment(target.path, target.dataDir, target.logPath)
	return cfg, err
}

func resolveConfigTarget(scopeRaw string) (configTarget, error) {
	if strings.TrimSpace(os.Getenv("SENTINEL_CONFIG")) != "" || strings.TrimSpace(os.Getenv("SENTINEL_DATA_DIR")) != "" {
		cfg := config.Default()
		return configTarget{
			path:    config.Path(),
			dataDir: cfg.DataDir(),
			logPath: cfg.Log.Path,
		}, nil
	}
	deployment, err := resolveDeploymentFn(scopeRaw)
	if err == nil {
		layout, layoutErr := daemon.LayoutForScope(deployment.Scope)
		if layoutErr != nil {
			return configTarget{}, layoutErr
		}
		logPath := filepath.Join(deployment.DataDir, "logs", "sentinel.log")
		if canonical, canonicalErr := daemon.HasCanonicalPaths(deployment); canonicalErr != nil {
			return configTarget{}, canonicalErr
		} else if canonical {
			logPath = layout.LogPath
		}
		return configTarget{path: deployment.ConfigPath, dataDir: deployment.DataDir, logPath: logPath}, nil
	}
	if !errors.Is(err, daemon.ErrNoServiceInstalled) {
		return configTarget{}, err
	}
	scope := strings.ToLower(strings.TrimSpace(scopeRaw))
	if scope == "" || scope == optionAuto {
		cfg := config.Default()
		return configTarget{path: config.Path(), dataDir: cfg.DataDir(), logPath: cfg.Log.Path}, nil
	}
	if scope != optionUser && scope != optionSystem {
		return configTarget{}, fmt.Errorf("invalid scope %q (valid: auto, user, system)", scopeRaw)
	}
	if err := requireScopeAccessFn(scope); err != nil {
		return configTarget{}, err
	}
	layout, err := daemon.LayoutForScope(scope)
	if err != nil {
		return configTarget{}, err
	}
	return configTarget{path: layout.ConfigPath, dataDir: layout.DataDir, logPath: layout.LogPath}, nil
}

type configShowOutput struct {
	Version      int                    `json:"version"`
	Server       configShowServer       `json:"server"`
	Storage      config.StorageConfig   `json:"storage"`
	Log          config.LogConfig       `json:"log"`
	HealthReport configShowHealthReport `json:"health_report"`
	Watchtower   configShowWatchtower   `json:"watchtower"`
	MCP          config.MCPConfig       `json:"mcp"`
	Runbooks     config.RunbooksConfig  `json:"runbooks"`
	MultiUser    configShowMultiUser    `json:"multi_user"`
	SystemUsers  []string               `json:"system_users"`
}

type configShowServer struct {
	Host                string   `json:"host"`
	Port                int      `json:"port"`
	Token               string   `json:"token"`
	AllowedOrigins      []string `json:"allowed_origins"`
	TrustedProxies      []string `json:"trusted_proxies"`
	CookieSecure        string   `json:"cookie_secure"`
	AllowInsecureCookie bool     `json:"allow_insecure_cookie"`
	Timezone            string   `json:"timezone"`
	Locale              string   `json:"locale"`
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

// configShowHealthReport mirrors config.HealthReportConfig but redacts the
// webhook URL, whose path embeds the Slack/Discord secret.
type configShowHealthReport struct {
	WebhookURL string `json:"webhook_url"`
	Schedule   string `json:"schedule"`
}

func newConfigShowOutput(cfg config.Config) configShowOutput {
	return configShowOutput{
		Version: cfg.Version,
		Server: configShowServer{
			Host:                cfg.Server.Host,
			Port:                cfg.Server.Port,
			Token:               redactConfigSecret(cfg.Server.Token),
			AllowedOrigins:      nonNilStrings(cfg.Server.AllowedOrigins),
			TrustedProxies:      nonNilStrings(cfg.Server.TrustedProxies),
			CookieSecure:        cfg.Server.CookieSecure,
			AllowInsecureCookie: cfg.Server.AllowInsecureCookie,
			Timezone:            cfg.Server.Timezone,
			Locale:              cfg.Server.Locale,
		},
		Storage: cfg.Storage,
		Log:     cfg.Log,
		HealthReport: configShowHealthReport{
			WebhookURL: redactConfigSecret(cfg.HealthReport.WebhookURL),
			Schedule:   cfg.HealthReport.Schedule,
		},
		Runbooks: cfg.Runbooks,
		MCP:      cfg.MCP,
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
	}
}

func redactConfigSecret(value string) string {
	if value == "" {
		return ""
	}
	return "******"
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
