package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/store"
	"github.com/spf13/cobra"
)

var storeNewFn = store.New

const dbOutputKeyDatabase = "database"

func newDBCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Initialize, inspect and reset Sentinel storage",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newDBInitCmd(app), newDBStatusCmd(app), newDBResetCmd(app))
	return cmd
}

func newDBInitCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create local config, directories and SQLite database",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runDBInit(app)
		},
	}
}

func runDBInit(app *App) error {
	configPath, err := config.Init(false)
	if err != nil {
		if !errors.Is(err, config.ErrConfigExists) {
			return failf(1, "db init failed: %w", err)
		}
		if err := config.ValidateFile(configPath); err != nil {
			return failf(1, "config validation failed: %w", err)
		}
	}
	cfg := config.Load()
	st, dbPath, err := openDBStore(cfg)
	if err != nil {
		return failf(1, "db init failed: %w", err)
	}
	defer func() { _ = st.Close() }()

	reportHeader(app.Stdout, "db", "initialization")
	printRows(app.Stdout, []outputRow{
		{Key: "config", Value: configPath},
		{Key: dbOutputKeyDatabase, Value: dbPath},
		{Key: cmdStatus, Value: "ok"},
	})
	return nil
}

func newDBStatusCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   cmdStatus,
		Short: "Show Sentinel storage status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDBStatus(cmd.Context(), app)
		},
	}
}

func runDBStatus(ctx context.Context, app *App) error {
	cfg, err := loadDBConfig()
	if err != nil {
		return failf(1, "db status failed: %w", err)
	}
	st, dbPath, err := openDBStore(cfg)
	if err != nil {
		return failf(1, "db status failed: %w", err)
	}
	defer func() { _ = st.Close() }()

	stats, err := st.GetStorageStats(ctx)
	if err != nil {
		return failf(1, "db status failed: %w", err)
	}
	rows := []outputRow{
		{Key: dbOutputKeyDatabase, Value: dbPath},
		{Key: "database bytes", Value: fmt.Sprint(stats.DatabaseBytes)},
		{Key: "wal bytes", Value: fmt.Sprint(stats.WALBytes)},
		{Key: "shm bytes", Value: fmt.Sprint(stats.SHMBytes)},
		{Key: "total bytes", Value: fmt.Sprint(stats.TotalBytes)},
	}
	for _, stat := range stats.Resources {
		rows = append(rows, outputRow{
			Key:   stat.Resource,
			Value: fmt.Sprintf("%d rows, %d approx bytes", stat.Rows, stat.ApproxBytes),
		})
	}
	reportHeader(app.Stdout, "db", "status")
	printRows(app.Stdout, rows)
	return nil
}

func newDBResetCmd(app *App) *cobra.Command {
	var yes bool
	var force bool
	var resource string
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Reset Sentinel storage",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !yes {
				return failf(1, "refusing to reset storage without --yes")
			}
			if force && cmd.Flags().Changed("resource") {
				return failf(1, "cannot combine --force with --resource")
			}
			return runDBReset(cmd.Context(), app, resource, force)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm flushing local runtime storage")
	cmd.Flags().BoolVar(&force, "force", false, "delete and recreate the SQLite database")
	cmd.Flags().StringVar(&resource, "resource", store.StorageResourceAll, "resource to flush: timeline, activity-journal, guardrail-audit, ops-activity, ops-alerts, ops-jobs, or all")
	return cmd
}

func runDBReset(ctx context.Context, app *App, resource string, force bool) error {
	if force {
		return runDBResetForce(app)
	}

	resource = store.NormalizeStorageResource(resource)
	if resource == "" {
		resource = store.StorageResourceAll
	}
	if !store.IsStorageResource(resource) {
		return failf(1, "invalid storage resource: %s", resource)
	}
	cfg, err := loadDBConfig()
	if err != nil {
		return failf(1, "db reset failed: %w", err)
	}
	st, dbPath, err := openDBStore(cfg)
	if err != nil {
		return failf(1, "db reset failed: %w", err)
	}
	defer func() { _ = st.Close() }()

	results, err := st.FlushStorageResource(ctx, resource)
	if err != nil {
		return failf(1, "db reset failed: %w", err)
	}
	rows := []outputRow{
		{Key: dbOutputKeyDatabase, Value: dbPath},
		{Key: "mode", Value: "flush"},
		{Key: "resource", Value: resource},
	}
	for _, result := range results {
		rows = append(rows, outputRow{
			Key:   result.Resource,
			Value: fmt.Sprintf("%d rows removed", result.RemovedRows),
		})
	}
	reportHeader(app.Stdout, "db", "reset")
	printRows(app.Stdout, rows)
	return nil
}

func runDBResetForce(app *App) error {
	cfg, err := loadDBConfig()
	if err != nil {
		return failf(1, "db reset failed: %w", err)
	}
	dbPath := filepath.Join(cfg.DataDir, "sentinel.db")
	removed, err := removeDBFiles(dbPath)
	if err != nil {
		return failf(1, "db reset failed: %w", err)
	}
	st, err := storeNewFn(dbPath)
	if err != nil {
		return failf(1, "db reset failed: %w", err)
	}
	defer func() { _ = st.Close() }()

	rows := []outputRow{
		{Key: dbOutputKeyDatabase, Value: dbPath},
		{Key: "mode", Value: "force"},
		{Key: cmdStatus, Value: "recreated"},
		{Key: "removed files", Value: fmt.Sprint(len(removed))},
	}
	for _, path := range removed {
		rows = append(rows, outputRow{Key: "removed", Value: path})
	}
	reportHeader(app.Stdout, "db", "reset")
	printRows(app.Stdout, rows)
	return nil
}

func removeDBFiles(dbPath string) ([]string, error) {
	var removed []string
	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm", dbPath + "-journal"} {
		if err := os.Remove(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("remove %s: %w", path, err)
		}
		removed = append(removed, path)
	}
	return removed, nil
}

func loadDBConfig() (config.Config, error) {
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

func openDBStore(cfg config.Config) (*store.Store, string, error) {
	dbPath := filepath.Join(cfg.DataDir, "sentinel.db")
	st, err := storeNewFn(dbPath)
	if err != nil {
		return nil, dbPath, err
	}
	return st, dbPath, nil
}
