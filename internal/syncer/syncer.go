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
	"clickhouse-syncer/internal/ui"
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
	ui       bool // whether to render rich terminal output
}

// TableSyncResult holds the result of syncing one table.
type TableSyncResult struct {
	Table            string
	SourceRows       uint64
	DestRowsBefore   uint64
	DestRowsAfter    uint64
	BatchesProcessed int
	RowsInserted     uint64
	BytesTransferred uint64 // uncompressed ClickHouse row bytes
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
	Operation             Operation
	Date                  string
	TablesSuccessful      int
	TablesFailed          int
	TotalRowsInserted     uint64
	TotalBytesTransferred uint64
	TotalDuration         time.Duration
	TableResults          []TableSyncResult
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
func New(cfg *config.Config, source, dest *clickhouse.Client, logger *logging.Logger, enableUI bool) (*Syncer, error) {
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
		ui:       enableUI,
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

// printHeader prints the operation header to stdout (only when --ui is set).
func (s *Syncer) printHeader(op Operation, target string, tables int) {
	if !s.ui {
		return
	}
	ui.PrintHeader(op.String(), target, tables)
}

// printValidationResult prints the validation result (only when --ui is set).
func (s *Syncer) printValidationResult(res *ValidationResult) {
	if !s.ui {
		return
	}
	steps := make([]ui.ValidationStep, len(res.Steps))
	for i, step := range res.Steps {
		steps[i] = ui.ValidationStep{
			Name:    step.Name,
			Passed:  step.Passed,
			Message: step.Message,
		}
	}
	ui.PrintValidationResult(steps, res.Passed)
}

// printSyncSummary prints the sync summary (only when --ui is set).
func (s *Syncer) printSyncSummary(summary *SyncSummary) {
	if !s.ui {
		return
	}
	rows := make([]ui.SyncTableRow, len(summary.TableResults))
	for i, r := range summary.TableResults {
		errStr := ""
		if r.Error != nil {
			errStr = r.Error.Error()
		}
		rows[i] = ui.SyncTableRow{
			Table:    r.Table,
			OK:       r.Error == nil,
			Verified: r.Verified,
			Rows:     r.RowsInserted,
			Bytes:    r.BytesTransferred,
			Duration: r.Duration,
			Error:    errStr,
		}
	}
	ui.PrintSyncSummary(rows, summary.Date,
		summary.TotalRowsInserted, summary.TotalBytesTransferred,
		summary.TotalDuration, summary.TablesFailed)
}

// printDeleteSummary prints the delete summary (only when --ui is set).
func (s *Syncer) printDeleteSummary(summary *DeleteSummary) {
	if !s.ui {
		return
	}
	rows := make([]ui.DeleteTableRow, len(summary.TableResults))
	for i, r := range summary.TableResults {
		errStr := ""
		if r.Error != nil {
			errStr = r.Error.Error()
		}
		rows[i] = ui.DeleteTableRow{
			Table:    r.Table,
			OK:       r.Error == nil,
			Verified: r.Verified,
			Rows:     r.RowsBeforeCutoff,
			Duration: r.Duration,
			Error:    errStr,
		}
	}
	ui.PrintDeleteSummary(rows, summary.BeforeDate,
		summary.TotalRowsDeleted, summary.TotalDuration, summary.TablesFailed)
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
		return nil, err
	}
	if !validRes.Passed {
		return nil, &SyncerError{Code: ExitValidationError, Message: "validation failed"}
	}

	summary := &SyncSummary{
		Operation: OpSync,
		Date:      dateStr,
	}

	// Build progress bars if UI is enabled
	var bar *ui.MultiBar
	if s.ui {
		names := make([]string, len(tables))
		for i, t := range tables {
			names[i] = t.Name
		}
		fmt.Println() // spacing after validation output — must come BEFORE NewMultiBar
		bar = ui.NewMultiBar(names)
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
				if bar != nil {
					bar.Failed(index, err.Error(), 0, 0)
				}
				mu.Lock()
				summary.TableResults = append(summary.TableResults, res)
				summary.TablesFailed++
				mu.Unlock()
				return
			}

			res := s.syncTable(ctx, t, targetDate, endDate, dateType, index+1, len(tables), bar, index)

			mu.Lock()
			summary.TableResults = append(summary.TableResults, res)
			if res.Error != nil {
				summary.TablesFailed++
			} else {
				summary.TablesSuccessful++
				summary.TotalRowsInserted += res.RowsInserted
				summary.TotalBytesTransferred += res.BytesTransferred
			}
			mu.Unlock()
		}(table, i)
	}

	wg.Wait()
	if s.ui {
		fmt.Println() // blank line after bars
	}
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
			if s.ui {
				fmt.Println("Operation cancelled by user.")
			}
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

	if s.ui {
		fmt.Println("\nRunning Validation...")
	}
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

	if !s.ui {
		return nil
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

	if s.ui {
		fmt.Printf("\n  %s%sAll validation steps passed successfully!%s\n\n",
			ui.AnsiBold, ui.AnsiGreen, ui.AnsiReset)
	}
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
