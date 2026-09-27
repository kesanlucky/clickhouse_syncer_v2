package syncer

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"clickhouse-syncer/internal/clickhouse"
	"clickhouse-syncer/internal/config"
	"clickhouse-syncer/internal/logging"
)

// Operation represents a sync operation type.
type Operation int

const (
	OpSync Operation = iota
	OpDelete
	OpDryRun
	OpValidate
	OpStandbySync
)

func (o Operation) String() string {
	switch o {
	case OpSync:
		return "SYNC"
	case OpDelete:
		return "DELETE"
	case OpDryRun:
		return "DRY RUN"
	case OpValidate:
		return "VALIDATE"
	case OpStandbySync:
		return "STANDBY SYNC"
	default:
		return "UNKNOWN"
	}
}

// ExitCode represents application exit codes.
type ExitCode int

const (
	ExitSuccess         ExitCode = 0
	ExitGeneralFailure  ExitCode = 1
	ExitConfigError     ExitCode = 2
	ExitValidationError ExitCode = 3
	ExitConnectionError ExitCode = 4
	ExitSchemaMismatch  ExitCode = 5
	ExitSyncFailure     ExitCode = 6
	ExitVerifyFailure   ExitCode = 7
	ExitDeleteFailure   ExitCode = 8
)

// SyncerError wraps errors with an exit code.
type SyncerError struct {
	Code    ExitCode
	Message string
	Err     error
}

func (e *SyncerError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *SyncerError) Unwrap() error {
	return e.Err
}

// Syncer orchestrates all sync operations.
type Syncer struct {
	config   *config.Config
	source   *clickhouse.Client
	dest     *clickhouse.Client
	logger   *logging.Logger
	timezone *time.Location
}

// TableSyncResult holds the result of syncing one table.
type TableSyncResult struct {
	Table            string
	SourceRows       uint64
	DestRowsBefore   uint64
	DestRowsAfter    uint64
	BatchesProcessed int
	RowsInserted     uint64
	Duration         time.Duration
	Verified         bool
	Error            error
}

// TableDeleteResult holds the result of deleting data from one table.
type TableDeleteResult struct {
	Table            string
	RowsBeforeCutoff uint64
	RowsAfterDelete  uint64
	Duration         time.Duration
	Verified         bool
	Error            error
}

// SyncSummary holds the overall operation summary.
type SyncSummary struct {
	Operation         Operation
	Date              string
	TablesSuccessful  int
	TablesFailed      int
	TotalRowsInserted uint64
	TotalDuration     time.Duration
	TableResults      []TableSyncResult
}

// DeleteSummary holds the overall delete summary.
type DeleteSummary struct {
	BeforeDate       string
	TablesSuccessful int
	TablesFailed     int
	TotalRowsDeleted uint64
	TotalDuration    time.Duration
	TableResults     []TableDeleteResult
}

// New creates a new Syncer instance.
func New(cfg *config.Config, source, dest *clickhouse.Client, logger *logging.Logger) (*Syncer, error) {
	tz, err := cfg.GetTimezone()
	if err != nil {
		return nil, &SyncerError{Code: ExitConfigError, Message: "invalid timezone", Err: err}
	}
	return &Syncer{
		config:   cfg,
		source:   source,
		dest:     dest,
		logger:   logger,
		timezone: tz,
	}, nil
}

// resolveTables returns the selected tables or all tables from config if empty.
func (s *Syncer) resolveTables(selectedTables []config.TableConfig) []config.TableConfig {
	if len(selectedTables) > 0 {
		return selectedTables
	}
	return s.config.Tables
}

// parseDate parses a YYYY-MM-DD date string using the configured timezone.
func (s *Syncer) parseDate(dateStr string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", dateStr, s.timezone)
}

// printHeader prints a formatted operation header.
func (s *Syncer) printHeader(op Operation, dateStr string, tables int) {
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("ClickHouse Data Syncer\n")
	fmt.Printf("Operation: %s\n", op)
	fmt.Printf("Date/Target: %s\n", dateStr)
	fmt.Printf("Tables: %d\n", tables)
	fmt.Println(strings.Repeat("=", 60))
}

// formatNumber formats a number with comma separators.
func formatNumber(n uint64) string {
	s := fmt.Sprintf("%d", n)
	var b []byte
	l := len(s)
	for i, v := range s {
		b = append(b, byte(v))
		if (l-i-1)%3 == 0 && i != l-1 {
			b = append(b, ',')
		}
	}
	return string(b)
}

// printSyncSummary prints a formatted sync summary.
func (s *Syncer) printSyncSummary(summary *SyncSummary) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("SYNC SUMMARY")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Date: %s\n", summary.Date)
	fmt.Printf("Total Duration: %s\n", summary.TotalDuration.Round(time.Millisecond))
	fmt.Printf("Tables Sync'd: %d\n", summary.TablesSuccessful)
	fmt.Printf("Tables Failed: %d\n", summary.TablesFailed)
	fmt.Printf("Total Rows Inserted: %s\n", formatNumber(summary.TotalRowsInserted))
	fmt.Println(strings.Repeat("-", 60))
	for _, tr := range summary.TableResults {
		if tr.Error != nil {
			fmt.Printf("✗ %s: Failed (%v)\n", tr.Table, tr.Error)
		} else if tr.Verified {
			fmt.Printf("✓ %s: %s rows in %s\n", tr.Table, formatNumber(tr.RowsInserted), tr.Duration.Round(time.Millisecond))
		} else {
			fmt.Printf("! %s: %s rows in %s (Verification Failed)\n", tr.Table, formatNumber(tr.RowsInserted), tr.Duration.Round(time.Millisecond))
		}
	}
	fmt.Println(strings.Repeat("=", 60))
}

// printDeleteSummary prints a formatted delete summary.
func (s *Syncer) printDeleteSummary(summary *DeleteSummary) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("DELETE SUMMARY")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Before Date: %s\n", summary.BeforeDate)
	fmt.Printf("Total Duration: %s\n", summary.TotalDuration.Round(time.Millisecond))
	fmt.Printf("Tables Processed: %d\n", summary.TablesSuccessful)
	fmt.Printf("Tables Failed: %d\n", summary.TablesFailed)
	fmt.Printf("Total Rows Deleted: %s\n", formatNumber(summary.TotalRowsDeleted))
	fmt.Println(strings.Repeat("-", 60))
	for _, tr := range summary.TableResults {
		if tr.Error != nil {
			fmt.Printf("✗ %s: Failed (%v)\n", tr.Table, tr.Error)
		} else if tr.Verified {
			fmt.Printf("✓ %s: %s rows deleted in %s\n", tr.Table, formatNumber(tr.RowsBeforeCutoff), tr.Duration.Round(time.Millisecond))
		} else {
			fmt.Printf("! %s: %s rows deleted in %s (Verification Failed)\n", tr.Table, formatNumber(tr.RowsBeforeCutoff), tr.Duration.Round(time.Millisecond))
		}
	}
	fmt.Println(strings.Repeat("=", 60))
}

// RunSync orchestrates the sync operation.
func (s *Syncer) RunSync(ctx context.Context, dateStr string, selectedTables []config.TableConfig) (*SyncSummary, error) {
	startTime := time.Now()
	tables := s.resolveTables(selectedTables)

	s.printHeader(OpSync, dateStr, len(tables))

	targetDate, err := s.parseDate(dateStr)
	if err != nil {
		return nil, &SyncerError{Code: ExitGeneralFailure, Message: "failed to parse date", Err: err}
	}
	endDate := targetDate.AddDate(0, 0, 1)

	validRes, err := s.ValidateAll(ctx, tables)
	if err != nil {
		return nil, err // Returns SyncerError
	}
	if !validRes.Passed {
		return nil, &SyncerError{Code: ExitValidationError, Message: "validation failed"}
	}

	summary := &SyncSummary{
		Operation: OpSync,
		Date:      dateStr,
	}

	sem := make(chan struct{}, s.config.Sync.MaxConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, table := range tables {
		wg.Add(1)
		sem <- struct{}{}

		go func(t config.TableConfig, index int) {
			defer wg.Done()
			defer func() { <-sem }()

			// Get date type from source
			dateType, err := s.source.GetDateColumnType(ctx, t.Name, t.DateColumn)
			if err != nil {
				res := TableSyncResult{Table: t.Name, Error: fmt.Errorf("failed to get date column type: %w", err)}
				mu.Lock()
				summary.TableResults = append(summary.TableResults, res)
				summary.TablesFailed++
				mu.Unlock()
				return
			}

			res := s.syncTable(ctx, t, targetDate, endDate, dateType, index+1, len(tables))

			mu.Lock()
			summary.TableResults = append(summary.TableResults, res)
			if res.Error != nil {
				summary.TablesFailed++
			} else {
				summary.TablesSuccessful++
				summary.TotalRowsInserted += res.RowsInserted
			}
			mu.Unlock()
		}(table, i)
	}

	wg.Wait()
	summary.TotalDuration = time.Since(startTime)
	s.printSyncSummary(summary)

	if summary.TablesFailed > 0 {
		return summary, &SyncerError{Code: ExitSyncFailure, Message: fmt.Sprintf("%d tables failed to sync", summary.TablesFailed)}
	}

	return summary, nil
}

// RunDelete orchestrates the delete operation.
func (s *Syncer) RunDelete(ctx context.Context, beforeStr string, selectedTables []config.TableConfig, force bool) (*DeleteSummary, error) {
	startTime := time.Now()
	tables := s.resolveTables(selectedTables)

	s.printHeader(OpDelete, "Before "+beforeStr, len(tables))

	cutoffDate, err := s.parseDate(beforeStr)
	if err != nil {
		return nil, &SyncerError{Code: ExitGeneralFailure, Message: "failed to parse before date", Err: err}
	}

	validRes, err := s.ValidateForDelete(ctx, tables)
	if err != nil {
		return nil, err
	}
	if !validRes.Passed {
		return nil, &SyncerError{Code: ExitValidationError, Message: "validation failed"}
	}

	if !force {
		confirmed, err := s.confirmDelete(tables, beforeStr)
		if err != nil {
			return nil, &SyncerError{Code: ExitGeneralFailure, Message: "confirmation failed", Err: err}
		}
		if !confirmed {
			fmt.Println("Operation cancelled by user.")
			return nil, nil
		}
	}

	summary := &DeleteSummary{
		BeforeDate: beforeStr,
	}

	for i, t := range tables {
		dateType, err := s.dest.GetDateColumnType(ctx, t.Name, t.DateColumn)
		if err != nil {
			res := TableDeleteResult{Table: t.Name, Error: fmt.Errorf("failed to get date column type: %w", err)}
			summary.TableResults = append(summary.TableResults, res)
			summary.TablesFailed++
			continue
		}

		res := s.deleteTable(ctx, t, cutoffDate, dateType, i+1, len(tables))
		summary.TableResults = append(summary.TableResults, res)
		if res.Error != nil {
			summary.TablesFailed++
		} else {
			summary.TablesSuccessful++
			summary.TotalRowsDeleted += res.RowsBeforeCutoff
		}
	}

	summary.TotalDuration = time.Since(startTime)
	s.printDeleteSummary(summary)

	if summary.TablesFailed > 0 {
		return summary, &SyncerError{Code: ExitDeleteFailure, Message: fmt.Sprintf("%d tables failed to delete", summary.TablesFailed)}
	}

	return summary, nil
}

// RunDryRun performs a dry run without modifying data.
func (s *Syncer) RunDryRun(ctx context.Context, op Operation, dateStr, beforeStr string, selectedTables []config.TableConfig) error {
	tables := s.resolveTables(selectedTables)

	var targetDate time.Time
	var err error

	if op == OpSync {
		s.printHeader(OpDryRun, "SYNC "+dateStr, len(tables))
		targetDate, err = s.parseDate(dateStr)
	} else if op == OpDelete {
		s.printHeader(OpDryRun, "DELETE Before "+beforeStr, len(tables))
		targetDate, err = s.parseDate(beforeStr)
	}

	if err != nil {
		return &SyncerError{Code: ExitGeneralFailure, Message: "failed to parse date", Err: err}
	}

	fmt.Println("\nRunning Validation...")
	var validRes *ValidationResult
	if op == OpSync {
		validRes, err = s.ValidateAll(ctx, tables)
	} else {
		validRes, err = s.ValidateForDelete(ctx, tables)
	}

	if err != nil {
		return err
	}
	if !validRes.Passed {
		return &SyncerError{Code: ExitValidationError, Message: "validation failed"}
	}

	fmt.Printf("\nDRY RUN\n")
	fmt.Println(strings.Repeat("-", 60))
	if op == OpSync {
		fmt.Printf("Operation       : SYNC\n")
		fmt.Printf("Date            : %s\n", dateStr)
	} else {
		fmt.Printf("Operation       : DELETE\n")
		fmt.Printf("Before          : %s\n", beforeStr)
		fmt.Printf("Target          : DESTINATION\n")
	}
	fmt.Printf("Timezone        : %s\n", s.timezone.String())
	fmt.Printf("Batch Size      : %s\n", formatNumber(uint64(s.config.Sync.BatchSize)))
	fmt.Printf("Concurrency     : %d\n", s.config.Sync.MaxConcurrency)
	fmt.Println(strings.Repeat("-", 60))

	for _, t := range tables {
		if op == OpSync {
			endDate := targetDate.AddDate(0, 0, 1)
			dateType, err := s.source.GetDateColumnType(ctx, t.Name, t.DateColumn)
			if err != nil {
				fmt.Printf("\n! %s: Error getting date type (%v)\n", t.Name, err)
				continue
			}

			sourceCount, err := s.source.CountRows(ctx, t.Name, t.DateColumn, targetDate, endDate, dateType)
			if err != nil {
				fmt.Printf("\n! %s: Error counting source rows (%v)\n", t.Name, err)
				continue
			}

			destCount, err := s.dest.CountRows(ctx, t.Name, t.DateColumn, targetDate, endDate, dateType)
			if err != nil {
				fmt.Printf("\n! %s: Error counting dest rows (%v)\n", t.Name, err)
				continue
			}

			batchSize := s.config.Sync.BatchSize
			estimatedBatches := (int(sourceCount) + batchSize - 1) / batchSize
			if sourceCount == 0 {
				estimatedBatches = 0
			}

			fmt.Printf("\nTable: %s\n", t.Name)
			fmt.Printf("  Schema            : VALID\n")
			fmt.Printf("  Source rows        : %s\n", formatNumber(sourceCount))
			fmt.Printf("  Destination rows   : %s\n", formatNumber(destCount))
			fmt.Printf("  Batch size         : %s\n", formatNumber(uint64(batchSize)))
			fmt.Printf("  Estimated batches  : %d\n", estimatedBatches)
			fmt.Printf("  Action             : REPLACE DATE DATA\n")

		} else if op == OpDelete {
			dateType, err := s.dest.GetDateColumnType(ctx, t.Name, t.DateColumn)
			if err != nil {
				fmt.Printf("\n! %s: Error getting date type (%v)\n", t.Name, err)
				continue
			}

			destCount, err := s.dest.CountRowsBefore(ctx, t.Name, t.DateColumn, targetDate, dateType)
			if err != nil {
				fmt.Printf("\n! %s: Error counting dest rows (%v)\n", t.Name, err)
				continue
			}

			fmt.Printf("\nTable: %s\n", t.Name)
			fmt.Printf("  Rows to delete     : %s\n", formatNumber(destCount))
			fmt.Printf("  Action             : DELETE BEFORE CUTOFF\n")
		}
	}
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("\nNo data was modified.")
	return nil
}

// RunStandby orchestrates the standby sync operation.
func (s *Syncer) RunStandby(ctx context.Context) error {
	return s.runStandbyLoop(ctx)
}

// RunValidate runs validation only.
func (s *Syncer) RunValidate(ctx context.Context) error {
	s.printHeader(OpValidate, "ALL TABLES", len(s.config.Tables))

	res, err := s.ValidateAll(ctx, s.config.Tables)
	if err != nil {
		return err
	}

	if !res.Passed {
		return &SyncerError{Code: ExitValidationError, Message: "validation failed"}
	}

	fmt.Println("\nAll validation steps passed successfully!")
	return nil
}

// Close closes the source and destination clients.
func (s *Syncer) Close() error {
	var errs []string
	if err := s.source.Close(); err != nil {
		errs = append(errs, fmt.Sprintf("source: %v", err))
	}
	if err := s.dest.Close(); err != nil {
		errs = append(errs, fmt.Sprintf("dest: %v", err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %s", strings.Join(errs, ", "))
	}
	return nil
}
