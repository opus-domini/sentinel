package cli

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

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
	)
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check whether a newer release is available",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg := loadConfigFn()
			result, err := updateCheckFn(context.Background(), updater.CheckOptions{
				CurrentVersion: currentVersionFn(),
				Repo:           strings.TrimSpace(repo),
				APIBaseURL:     strings.TrimSpace(apiBase),
				OS:             strings.TrimSpace(targetOS),
				Arch:           strings.TrimSpace(targetArch),
				DataDir:        cfg.DataDir,
			})
			if err != nil {
				return failf(1, "update check failed: %w", err)
			}
			printRows(app.Stdout, []outputRow{
				{Key: "current version", Value: valueOrDash(result.CurrentVersion)},
				{Key: "latest version", Value: valueOrDash(result.LatestVersion)},
				{Key: "up to date", Value: fmt.Sprintf("%t", result.UpToDate)},
				{Key: "release", Value: valueOrDash(result.ReleaseURL)},
				{Key: "asset", Value: valueOrDash(result.AssetName)},
				{Key: "sha256", Value: valueOrDash(result.ExpectedSHA256)},
			})
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", defaultUpdaterRepo, "GitHub repository in owner/name format")
	cmd.Flags().StringVar(&apiBase, "api", "", "GitHub API base URL override")
	cmd.Flags().StringVar(&targetOS, "os", runtime.GOOS, "target operating system")
	cmd.Flags().StringVar(&targetArch, "arch", runtime.GOARCH, "target CPU architecture")
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
		systemdScope    string
	)
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Download and install the latest release",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			resolvedScope, scopeErr := resolveRestartScopeFlag(scope, systemdScope)
			if scopeErr != nil {
				return failf(2, "invalid scope flags: %w", scopeErr)
			}

			cfg := loadConfigFn()
			result, err := updateApplyFn(context.Background(), updater.ApplyOptions{
				CurrentVersion:  currentVersionFn(),
				Repo:            strings.TrimSpace(repo),
				APIBaseURL:      strings.TrimSpace(apiBase),
				OS:              strings.TrimSpace(targetOS),
				Arch:            strings.TrimSpace(targetArch),
				DataDir:         cfg.DataDir,
				ExecPath:        strings.TrimSpace(execPath),
				AllowDowngrade:  allowDowngrade,
				AllowUnverified: allowUnverified,
				Restart:         restart,
				ServiceUnit:     strings.TrimSpace(serviceUnit),
				SystemdScope:    resolvedScope,
			})
			if err != nil {
				return failf(1, "update apply failed: %w", err)
			}

			if !result.Applied {
				printRows(app.Stdout, []outputRow{
					{Key: "already up to date", Value: valueOrDash(result.CurrentVersion)},
				})
				return nil
			}

			printNotice(app.Stdout, "update applied successfully")
			printRows(app.Stdout, []outputRow{
				{Key: "updated from", Value: valueOrDash(result.CurrentVersion)},
				{Key: "updated to", Value: valueOrDash(result.LatestVersion)},
				{Key: "binary", Value: valueOrDash(result.BinaryPath)},
				{Key: "backup", Value: valueOrDash(result.BackupPath)},
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
	cmd.Flags().BoolVar(&restart, "restart", false, "restart the managed service after a successful update")
	cmd.Flags().StringVar(&serviceUnit, "service", "sentinel", "service unit/label to restart after the update")
	cmd.Flags().StringVar(&scope, "scope", "", "restart scope: auto|user|system|launchd|none")
	cmd.Flags().StringVar(&systemdScope, "systemd-scope", "", "deprecated alias for --scope")
	_ = cmd.Flags().MarkDeprecated("systemd-scope", "use --scope instead")
	return cmd
}

func newUpdateStatusCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   cmdStatus,
		Short: "Show the last recorded update state",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg := loadConfigFn()
			state, err := updateStatusFn(cfg.DataDir)
			if err != nil {
				return failf(1, "update status failed: %w", err)
			}
			printRows(app.Stdout, []outputRow{
				{Key: "current version", Value: valueOrDash(state.CurrentVersion)},
				{Key: "latest version", Value: valueOrDash(state.LatestVersion)},
				{Key: "up to date", Value: fmt.Sprintf("%t", state.UpToDate)},
				{Key: "last checked", Value: formatTime(state.LastCheckedAt)},
				{Key: "last applied", Value: formatTime(state.LastAppliedAt)},
				{Key: "release", Value: valueOrDash(state.LastReleaseURL)},
				{Key: "binary", Value: valueOrDash(state.LastAppliedBinary)},
				{Key: "backup", Value: valueOrDash(state.LastAppliedBackup)},
				{Key: "sha256", Value: valueOrDash(state.LastExpectedSHA256)},
				{Key: "last error", Value: valueOrDash(state.LastError)},
			})
			return nil
		},
	}
}

// resolveRestartScopeFlag reconciles --scope with the deprecated
// --systemd-scope alias, returning an error when they conflict.
func resolveRestartScopeFlag(scope, legacyScope string) (string, error) {
	primary := strings.TrimSpace(scope)
	legacy := strings.TrimSpace(legacyScope)
	switch {
	case primary == "" && legacy == "":
		return "", nil
	case primary == "":
		return legacy, nil
	case legacy == "":
		return primary, nil
	case strings.EqualFold(primary, legacy):
		return primary, nil
	default:
		return "", fmt.Errorf("--scope=%s conflicts with --systemd-scope=%s", primary, legacy)
	}
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.UTC().Format(time.RFC3339)
}

func valueOrDash(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "-"
	}
	return raw
}
