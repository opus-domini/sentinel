package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/opus-domini/sentinel/internal/config"
	"github.com/opus-domini/sentinel/internal/humanize"
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
	cfg, configPath, err := config.Ensure()
	if err != nil {
		return failf("db init failed: %w", err)
	}
	st, dbPath, err := openDBStore(cfg)
	if err != nil {
		return failf("db init failed: %w", err)
	}
	defer func() { _ = st.Close() }()

	reportHeader(app.Stdout, "db", "initialization")
	printRows(app.Stdout, []outputRow{
		{Key: cmdConfig, Value: configPath},
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
	cfg, err := loadValidatedConfig()
	if err != nil {
		return failf("db status failed: %w", err)
	}
	st, dbPath, err := openDBStore(cfg)
	if err != nil {
		return failf("db status failed: %w", err)
	}
	defer func() { _ = st.Close() }()

	stats, err := st.GetStorageStats(ctx)
	if err != nil {
		return failf("db status failed: %w", err)
	}
	rows := []outputRow{
		{Key: dbOutputKeyDatabase, Value: dbPath},
		{Key: "database size", Value: humanize.Bytes(stats.DatabaseBytes)},
		{Key: "wal size", Value: humanize.Bytes(stats.WALBytes)},
		{Key: "shm size", Value: humanize.Bytes(stats.SHMBytes)},
		{Key: "total size", Value: humanize.Bytes(stats.TotalBytes)},
	}
	for _, stat := range stats.Resources {
		rows = append(rows, outputRow{
			Key:   stat.Resource,
			Value: fmt.Sprintf("%d %s, %s approx", stat.Rows, humanize.Pluralize(stat.Rows, "row", ""), humanize.Bytes(stat.ApproxBytes)),
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
				return failf("refusing to reset storage without --yes")
			}
			if force && cmd.Flags().Changed("resource") {
				return failf("cannot combine --force with --resource")
			}
			return runDBReset(cmd.Context(), app, resource, force)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm flushing local runtime storage")
	cmd.Flags().BoolVar(&force, "force", false, "delete and recreate the SQLite database")
	cmd.Flags().StringVar(&resource, "resource", store.StorageResourceAll, "resource to flush: activity-journal, ops-jobs, or all")
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
		return failf("invalid storage resource: %s", resource)
	}
	cfg, err := loadValidatedConfig()
	if err != nil {
		return failf("db reset failed: %w", err)
	}
	st, dbPath, err := openDBStore(cfg)
	if err != nil {
		return failf("db reset failed: %w", err)
	}
	defer func() { _ = st.Close() }()

	results, err := st.FlushStorageResource(ctx, resource)
	if err != nil {
		return failf("db reset failed: %w", err)
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
	cfg, err := loadValidatedConfig()
	if err != nil {
		return failf("db reset failed: %w", err)
	}
	dbPath := cfg.Storage.Path
	removed, err := removeDBFiles(dbPath)
	if err != nil {
		return failf("db reset failed: %w", err)
	}
	st, err := storeNewFn(dbPath)
	if err != nil {
		return failf("db reset failed: %w", err)
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

func openDBStore(cfg config.Config) (*store.Store, string, error) {
	dbPath := cfg.Storage.Path
	st, err := storeNewFn(dbPath)
	if err != nil {
		return nil, dbPath, err
	}
	return st, dbPath, nil
}
