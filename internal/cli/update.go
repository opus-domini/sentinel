package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/daemon"
	"github.com/opus-domini/sentinel/internal/humanize"
	"github.com/opus-domini/sentinel/internal/updater"
)

const defaultUpdaterRepo = "opus-domini/sentinel"

func newUpdateCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check and apply binary updates",
		Long:  "Check for and apply Sentinel binary updates.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newUpdateCheckCmd(app),
		newUpdateApplyCmd(app),
		newUpdateStatusCmd(app),
	)
	return cmd
}

func newUpdateCheckCmd(app *App) *cobra.Command {
	var (
		repo       string
		apiBase    string
		targetOS   string
		targetArch string
		scope      string
	)
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check whether a newer release is available",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, err := resolveUpdateContext(scope, false)
			if err != nil {
				return failf("update check failed: %w", err)
			}
			result, err := updateCheckFn(context.Background(), updater.CheckOptions{
				CurrentVersion: currentVersionFn(),
				Repo:           strings.TrimSpace(repo),
				APIBaseURL:     strings.TrimSpace(apiBase),
				OS:             strings.TrimSpace(targetOS),
				Arch:           strings.TrimSpace(targetArch),
				DataDir:        ctx.cfg.DataDir(),
			})
			if err != nil {
				return failf("update check failed: %w", err)
			}
			printRows(app.Stdout, []outputRow{
				{Key: "current version", Value: humanize.ValueOrDash(result.CurrentVersion)},
				{Key: "latest version", Value: humanize.ValueOrDash(result.LatestVersion)},
				{Key: "up to date", Value: fmt.Sprintf("%t", result.UpToDate)},
				{Key: "release", Value: humanize.ValueOrDash(result.ReleaseURL)},
				{Key: "asset", Value: humanize.ValueOrDash(result.AssetName)},
				{Key: "sha256", Value: humanize.ValueOrDash(result.ExpectedSHA256)},
			})
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", defaultUpdaterRepo, "GitHub repository in owner/name format")
	cmd.Flags().StringVar(&apiBase, "api", "", "GitHub API base URL override")
	cmd.Flags().StringVar(&targetOS, "os", runtime.GOOS, "target operating system")
	cmd.Flags().StringVar(&targetArch, "arch", runtime.GOARCH, "target CPU architecture")
	cmd.Flags().StringVar(&scope, "scope", optionAuto, "target deployment: auto|user|system")
	return cmd
}

func newUpdateApplyCmd(app *App) *cobra.Command {
	var (
		repo            string
		apiBase         string
		targetOS        string
		targetArch      string
		execPath        string
		allowDowngrade  bool
		allowUnverified bool
		restart         bool
		serviceUnit     string
		scope           string
	)
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Download and install the latest release",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			restartScope, err := normalizeUpdateApplyScope(scope)
			if err != nil {
				return failf("%w", err)
			}
			updateContext, err := resolveUpdateContext(restartScope, true)
			if err != nil {
				return failf("update apply failed: %w", err)
			}
			if updateContext.deployment != nil {
				if requested := strings.TrimSpace(execPath); requested != "" && !sameBinaryPath(requested, updateContext.deployment.BinaryPath) {
					return failf("update apply failed: --exec=%s does not match the %s deployment binary %s", requested, updateContext.deployment.Scope, updateContext.deployment.BinaryPath)
				}
				if requested := strings.TrimSpace(serviceUnit); requested != "" && requested != sentinelServiceUnit && requested != "io.opusdomini.sentinel" {
					return failf("update apply failed: --service cannot override the managed Sentinel deployment")
				}
				execPath = updateContext.deployment.BinaryPath
				restartScope = updateContext.deployment.Scope
				serviceUnit = sentinelServiceUnit
			} else {
				restartScope = "none"
				restart = false
				if strings.TrimSpace(execPath) == "" {
					execPath, err = os.Executable()
					if err != nil {
						return failf("update apply failed: resolve current executable: %w", err)
					}
				}
				if err := preflightBinaryWrite(execPath); err != nil {
					return failf("update apply failed: %w", err)
				}
			}
			result, err := updateApplyFn(context.Background(), updater.ApplyOptions{
				CurrentVersion:  currentVersionFn(),
				Repo:            strings.TrimSpace(repo),
				APIBaseURL:      strings.TrimSpace(apiBase),
				OS:              strings.TrimSpace(targetOS),
				Arch:            strings.TrimSpace(targetArch),
				DataDir:         updateContext.cfg.DataDir(),
				ConfigPath:      updateContext.configPath,
				ExecPath:        strings.TrimSpace(execPath),
				AllowDowngrade:  allowDowngrade,
				AllowUnverified: allowUnverified,
				SkipRestart:     !restart,
				ServiceUnit:     strings.TrimSpace(serviceUnit),
				SystemdScope:    restartScope,
			})
			if err != nil {
				return failf("update apply failed: %w", err)
			}

			if !result.Applied {
				printRows(app.Stdout, []outputRow{
					{Key: "already up to date", Value: humanize.ValueOrDash(result.CurrentVersion)},
				})
				return nil
			}

			printNotice(app.Stdout, "update applied successfully")
			printRows(app.Stdout, []outputRow{
				{Key: "updated from", Value: humanize.ValueOrDash(result.CurrentVersion)},
				{Key: "updated to", Value: humanize.ValueOrDash(result.LatestVersion)},
				{Key: "binary", Value: humanize.ValueOrDash(result.BinaryPath)},
				{Key: "backup", Value: humanize.ValueOrDash(result.BackupPath)},
			})
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", defaultUpdaterRepo, "GitHub repository in owner/name format")
	cmd.Flags().StringVar(&apiBase, "api", "", "GitHub API base URL override")
	cmd.Flags().StringVar(&targetOS, "os", runtime.GOOS, "target operating system")
	cmd.Flags().StringVar(&targetArch, "arch", runtime.GOARCH, "target CPU architecture")
	cmd.Flags().StringVar(&execPath, "exec", "", "path to the sentinel binary to replace (default: current executable)")
	cmd.Flags().BoolVar(&allowDowngrade, "allow-downgrade", false, "allow installing an older release")
	cmd.Flags().BoolVar(&allowUnverified, "allow-unverified", false, "allow update when the checksum is unavailable")
	cmd.Flags().BoolVar(&restart, "restart", true, "restart the managed service after a successful update")
	cmd.Flags().StringVar(&serviceUnit, "service", sentinelServiceUnit, "service unit/label to restart after the update")
	cmd.Flags().StringVar(&scope, "scope", optionAuto, "installation and service scope: auto, user, or system")
	return cmd
}

func normalizeUpdateApplyScope(raw string) (string, error) {
	scope := strings.ToLower(strings.TrimSpace(raw))
	if scope == "" {
		return optionAuto, nil
	}
	switch scope {
	case optionAuto, optionUser, optionSystem:
		return scope, nil
	default:
		return "", fmt.Errorf("unsupported update apply scope %q (valid: auto, user, system)", raw)
	}
}

func newUpdateStatusCmd(app *App) *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   cmdStatus,
		Short: "Show the last recorded update state",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, err := resolveUpdateContext(scope, false)
			if err != nil {
				return failf("update status failed: %w", err)
			}
			state, err := updateStatusFn(ctx.cfg.DataDir())
			if err != nil {
				return failf("update status failed: %w", err)
			}
			printRows(app.Stdout, []outputRow{
				{Key: "current version", Value: humanize.ValueOrDash(state.CurrentVersion)},
				{Key: "latest version", Value: humanize.ValueOrDash(state.LatestVersion)},
				{Key: "up to date", Value: fmt.Sprintf("%t", state.UpToDate)},
				{Key: "last checked", Value: humanize.Time(state.LastCheckedAt)},
				{Key: "last applied", Value: humanize.Time(state.LastAppliedAt)},
				{Key: "release", Value: humanize.ValueOrDash(state.LastReleaseURL)},
				{Key: "binary", Value: humanize.ValueOrDash(state.LastAppliedBinary)},
				{Key: "backup", Value: humanize.ValueOrDash(state.LastAppliedBackup)},
				{Key: "sha256", Value: humanize.ValueOrDash(state.LastExpectedSHA256)},
				{Key: "last error", Value: humanize.ValueOrDash(state.LastError)},
			})
			return nil
		},
	}
	cmd.Flags().StringVar(&scope, "scope", optionAuto, "target deployment: auto|user|system")
	return cmd
}

type updateContext struct {
	cfg        config.Config
	configPath string
	deployment *daemon.Deployment
}

func resolveUpdateContext(scopeRaw string, requireWritable bool) (updateContext, error) {
	deployment, err := resolveDeploymentFn(scopeRaw)
	if err != nil {
		if !errors.Is(err, daemon.ErrNoServiceInstalled) {
			return updateContext{}, err
		}
		if normalized, normalizeErr := normalizeUpdateApplyScope(scopeRaw); normalizeErr != nil {
			return updateContext{}, normalizeErr
		} else if normalized != optionAuto {
			return updateContext{}, fmt.Errorf("no managed Sentinel deployment exists in %s scope", normalized)
		}
		cfg, configPath, loadErr := loadConfigFn()
		if loadErr != nil {
			return updateContext{}, loadErr
		}
		return updateContext{cfg: cfg, configPath: configPath}, nil
	}
	if err := requireScopeAccessFn(deployment.Scope); err != nil {
		return updateContext{}, err
	}
	canonical, err := daemon.HasCanonicalPaths(deployment)
	if err != nil {
		return updateContext{}, err
	}
	if !canonical {
		return updateContext{}, fmt.Errorf(
			"the %s deployment uses config %s and data %s; run `sentinel service migrate --scope %s` before updating",
			deployment.Scope,
			deployment.ConfigPath,
			deployment.DataDir,
			deployment.Scope,
		)
	}
	if requireWritable {
		if err := preflightBinaryWrite(deployment.BinaryPath); err != nil {
			return updateContext{}, err
		}
	}
	layout, err := daemon.LayoutForScope(deployment.Scope)
	if err != nil {
		return updateContext{}, err
	}
	cfg, configPath, err := loadConfigDeploymentFn(deployment.ConfigPath, deployment.DataDir, layout.LogPath)
	if err != nil {
		return updateContext{}, err
	}
	return updateContext{cfg: cfg, configPath: configPath, deployment: &deployment}, nil
}

func preflightBinaryWrite(binaryPath string) error {
	binaryPath = strings.TrimSpace(binaryPath)
	if binaryPath == "" {
		return errors.New("deployment binary path is empty")
	}
	if info, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("access deployment binary %s: %w", binaryPath, err)
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("deployment binary is not a regular file: %s", binaryPath)
	}
	probe, err := os.CreateTemp(filepath.Dir(binaryPath), ".sentinel-update-preflight-*")
	if err != nil {
		return fmt.Errorf("deployment binary %s is not writable by this user: %w", binaryPath, err)
	}
	probePath := probe.Name()
	if closeErr := probe.Close(); closeErr != nil {
		_ = os.Remove(probePath)
		return fmt.Errorf("close update preflight file: %w", closeErr)
	}
	if err := os.Remove(probePath); err != nil {
		return fmt.Errorf("remove update preflight file: %w", err)
	}
	return nil
}
