package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"clickhouse-syncer/internal/clickhouse"
	"clickhouse-syncer/internal/config"
	"clickhouse-syncer/internal/logging"
	"clickhouse-syncer/internal/syncer"
)

// NewRootCommand creates and configures the root cobra command for the CLI.
func NewRootCommand() *cobra.Command {
	var configPath string

	rootCmd := &cobra.Command{
		Use:           "clickhouse-syncer",
		Short:         "ClickHouse Data Syncer - Synchronize tables between ClickHouse instances",
		Long:          "A tool to incrementally synchronize data between two ClickHouse clusters or instances.",
		Version:       "1.0.0",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVar(&configPath, "config", "config.yaml", "Path to configuration file")

	// Validate command
	validateCmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the configuration and schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, logger, src, dst, s, err := setupDependencies(cmd.Context(), configPath)
			if err != nil {
				return err
			}
			defer cleanup(logger, src, dst, s)
			return s.RunValidate(cmd.Context())
		},
	}

	// Sync command
	var syncDate, syncTable string
	var syncAll bool
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Synchronize data for a specific date",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, logger, src, dst, s, err := setupDependencies(cmd.Context(), configPath)
			if err != nil {
				return err
			}
			defer cleanup(logger, src, dst, s)

			tables, err := resolveSelectedTables(cfg, syncTable, syncAll)
			if err != nil {
				return err
			}

			_, err = s.RunSync(cmd.Context(), syncDate, tables)
			return err
		},
	}
	syncCmd.Flags().StringVar(&syncDate, "date", "", "Date to sync (e.g. 2023-01-01)")
	syncCmd.MarkFlagRequired("date")
	syncCmd.Flags().StringVar(&syncTable, "table", "", "Specific table to sync")
	syncCmd.Flags().BoolVar(&syncAll, "all", false, "Sync all tables")

	// Delete command
	var deleteBefore, deleteTable string
	var deleteAll, deleteForce bool
	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete data before a specific date",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, logger, src, dst, s, err := setupDependencies(cmd.Context(), configPath)
			if err != nil {
				return err
			}
			defer cleanup(logger, src, dst, s)

			tables, err := resolveSelectedTables(cfg, deleteTable, deleteAll)
			if err != nil {
				return err
			}

			_, err = s.RunDelete(cmd.Context(), deleteBefore, tables, deleteForce)
			return err
		},
	}
	deleteCmd.Flags().StringVar(&deleteBefore, "before", "", "Delete data before this date (e.g. 2023-01-01)")
	deleteCmd.MarkFlagRequired("before")
	deleteCmd.Flags().StringVar(&deleteTable, "table", "", "Specific table to delete from")
	deleteCmd.Flags().BoolVar(&deleteAll, "all", false, "Delete from all tables")
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Force delete without prompt")

	// Dry-run command
	dryRunCmd := &cobra.Command{
		Use:   "dry-run",
		Short: "Preview operations without modifying data",
	}

	var drySyncDate, drySyncTable string
	var drySyncAll bool
	dryRunSyncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Preview sync operation",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, logger, src, dst, s, err := setupDependencies(cmd.Context(), configPath)
			if err != nil {
				return err
			}
			defer cleanup(logger, src, dst, s)

			tables, err := resolveSelectedTables(cfg, drySyncTable, drySyncAll)
			if err != nil {
				return err
			}

			return s.RunDryRun(cmd.Context(), syncer.OpSync, drySyncDate, "", tables)
		},
	}
	dryRunSyncCmd.Flags().StringVar(&drySyncDate, "date", "", "Date to sync (e.g. 2023-01-01)")
	dryRunSyncCmd.MarkFlagRequired("date")
	dryRunSyncCmd.Flags().StringVar(&drySyncTable, "table", "", "Specific table to sync")
	dryRunSyncCmd.Flags().BoolVar(&drySyncAll, "all", false, "Sync all tables")

	var dryDeleteBefore, dryDeleteTable string
	var dryDeleteAll bool
	dryRunDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Preview delete operation",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, logger, src, dst, s, err := setupDependencies(cmd.Context(), configPath)
			if err != nil {
				return err
			}
			defer cleanup(logger, src, dst, s)

			tables, err := resolveSelectedTables(cfg, dryDeleteTable, dryDeleteAll)
			if err != nil {
				return err
			}

			return s.RunDryRun(cmd.Context(), syncer.OpDelete, "", dryDeleteBefore, tables)
		},
	}
	dryRunDeleteCmd.Flags().StringVar(&dryDeleteBefore, "before", "", "Delete data before this date")
	dryRunDeleteCmd.MarkFlagRequired("before")
	dryRunDeleteCmd.Flags().StringVar(&dryDeleteTable, "table", "", "Specific table to delete from")
	dryRunDeleteCmd.Flags().BoolVar(&dryDeleteAll, "all", false, "Delete from all tables")

	dryRunCmd.AddCommand(dryRunSyncCmd, dryRunDeleteCmd)

	// Standby-sync command
	standbySyncCmd := &cobra.Command{
		Use:   "standby-sync",
		Short: "Run the syncer in standby mode for continuous synchronization",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, logger, src, dst, s, err := setupDependencies(cmd.Context(), configPath)
			if err != nil {
				return err
			}
			defer cleanup(logger, src, dst, s)

			return s.RunStandby(cmd.Context())
		},
	}

	rootCmd.AddCommand(validateCmd, syncCmd, deleteCmd, dryRunCmd, standbySyncCmd)
	return rootCmd
}

// setupDependencies loads configuration, sets up clients, and initializes the syncer.
func setupDependencies(ctx context.Context, configPath string) (*config.Config, *logging.Logger, *clickhouse.Client, *clickhouse.Client, *syncer.Syncer, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, nil, nil, nil, nil, &syncer.SyncerError{Code: syncer.ExitConfigError, Message: "failed to load config", Err: err}
	}
	if err := cfg.Validate(); err != nil {
		return nil, nil, nil, nil, nil, &syncer.SyncerError{Code: syncer.ExitValidationError, Message: "invalid config", Err: err}
	}

	logConfig := logging.LogConfig{
		Level:     cfg.Logging.Level,
		Directory: cfg.Logging.Directory,
		Console:   cfg.Logging.Console,
	}
	logger, err := logging.NewLogger(logConfig)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	srcClient, err := clickhouse.NewClient(ctx, cfg.Source, "source")
	if err != nil {
		return nil, nil, nil, nil, nil, &syncer.SyncerError{Code: syncer.ExitConnectionError, Message: "failed to connect to source", Err: err}
	}

	dstClient, err := clickhouse.NewClient(ctx, cfg.Destination, "destination")
	if err != nil {
		srcClient.Close()
		return nil, nil, nil, nil, nil, &syncer.SyncerError{Code: syncer.ExitConnectionError, Message: "failed to connect to destination", Err: err}
	}

	s, err := syncer.New(cfg, srcClient, dstClient, logger)
	if err != nil {
		srcClient.Close()
		dstClient.Close()
		return nil, nil, nil, nil, nil, &syncer.SyncerError{Code: syncer.ExitGeneralFailure, Message: "failed to create syncer", Err: err}
	}

	return cfg, logger, srcClient, dstClient, s, nil
}

// cleanup gracefully closes all resources.
func cleanup(logger *logging.Logger, src, dst *clickhouse.Client, s *syncer.Syncer) {
	if s != nil {
		_ = s.Close()
	}
	if src != nil {
		_ = src.Close()
	}
	if dst != nil {
		_ = dst.Close()
	}
	if logger != nil {
		_ = logger.Sync()
	}
}

// resolveSelectedTables validates table selection arguments and returns the list of tables to process.
func resolveSelectedTables(cfg *config.Config, tableName string, allFlag bool) ([]config.TableConfig, error) {
	if tableName != "" && allFlag {
		return nil, &syncer.SyncerError{
			Code:    syncer.ExitGeneralFailure,
			Message: "invalid arguments",
			Err:     errors.New("--table and --all are mutually exclusive"),
		}
	}
	if tableName != "" {
		table, found := cfg.FindTable(tableName)
		if !found {
			return nil, &syncer.SyncerError{
				Code:    syncer.ExitValidationError,
				Message: "table not found",
				Err:     fmt.Errorf("table %q not found in config", tableName),
			}
		}
		return []config.TableConfig{*table}, nil
	}
	// Default to all configured tables
	return cfg.Tables, nil
}
