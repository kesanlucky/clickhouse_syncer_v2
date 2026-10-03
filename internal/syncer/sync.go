package syncer

import (
	"context"
	"fmt"
	"io"
	"time"

	"clickhouse-syncer/internal/clickhouse"
	"clickhouse-syncer/internal/config"
	"clickhouse-syncer/internal/logging"
	"clickhouse-syncer/internal/ui"
)

func (s *Syncer) withRetry(ctx context.Context, opName string, fn func() error) error {
	if !s.config.Sync.Retries.Enabled {
		return fn()
	}

	maxAttempts := s.config.Sync.Retries.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	backoff, err := s.config.GetRetryBackoff()
	if err != nil {
		backoff = 5 * time.Second
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		if !clickhouse.IsRetryableError(err) {
			return err
		}

		lastErr = err
		s.logger.Warn(fmt.Sprintf("%s failed (attempt %d/%d), retrying...", opName, attempt, maxAttempts), "error", err)

		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				// wait
			}
		}
	}
	return fmt.Errorf("%s failed after %d attempts: %w", opName, maxAttempts, lastErr)
}

// syncTable syncs one table for the given date range.
// bar and barIdx are used to update the live progress display; bar may be nil.
func (s *Syncer) syncTable(
	ctx context.Context,
	table config.TableConfig,
	start, end time.Time,
	dateType clickhouse.DateColumnType,
	tableIndex, tableTotal int,
	bar *ui.MultiBar,
	barIdx int,
) TableSyncResult {
	startT := time.Now()
	res := TableSyncResult{Table: table.Name}

	logCtx := s.logger.With(
		"table", table.Name,
		"index", tableIndex,
		"total", tableTotal,
		"op", logging.GenerateOperationID("SYNC", start.Format("2006-01-02")),
	)

	logCtx.Info("Starting table sync")

	// 1. Count source rows
	var sourceCount uint64
	err := s.withRetry(ctx, "count source rows", func() error {
		var e error
		sourceCount, e = s.source.CountRows(ctx, table.Name, table.DateColumn, start, end, dateType)
		return e
	})
	if err != nil {
		res.Error = fmt.Errorf("failed to count source rows: %w", err)
		logCtx.Error("Table sync failed", "error", res.Error)
		if bar != nil {
			bar.Failed(barIdx, res.Error.Error(), 0, time.Since(startT))
		}
		return res
	}
	res.SourceRows = sourceCount

	if sourceCount == 0 {
		logCtx.Info("No rows to sync")
		res.Verified = true
		res.Duration = time.Since(startT)
		if bar != nil {
			bar.Done(barIdx, 0, 0, 0, res.Duration)
		}
		return res
	}

	// Set bar total now that we know it
	if bar != nil {
		bar.SetTotal(barIdx, sourceCount)
	}

	// 2. Count dest before
	var destCountBefore uint64
	err = s.withRetry(ctx, "count dest rows before", func() error {
		var e error
		destCountBefore, e = s.dest.CountRows(ctx, table.Name, table.DateColumn, start, end, dateType)
		return e
	})
	if err != nil {
		res.Error = fmt.Errorf("failed to count dest rows before: %w", err)
		logCtx.Error("Table sync failed", "error", res.Error)
		if bar != nil {
			bar.Failed(barIdx, res.Error.Error(), 0, time.Since(startT))
		}
		return res
	}
	res.DestRowsBefore = destCountBefore

	// 3. Delete dest rows
	if destCountBefore > 0 {
		err = s.withRetry(ctx, "delete dest rows", func() error {
			return s.dest.DeleteDateRange(ctx, table.Name, table.DateColumn, start, end, dateType)
		})
		if err != nil {
			res.Error = fmt.Errorf("failed to delete dest rows: %w", err)
			logCtx.Error("Table sync failed", "error", res.Error)
			if bar != nil {
				bar.Failed(barIdx, res.Error.Error(), 0, time.Since(startT))
			}
			return res
		}

		// 4. Wait for mutations
		err = s.dest.WaitForMutations(ctx, table.Name, 5*time.Minute)
		if err != nil {
			res.Error = fmt.Errorf("failed waiting for mutations: %w", err)
			logCtx.Error("Table sync failed", "error", res.Error)
			if bar != nil {
				bar.Failed(barIdx, res.Error.Error(), 0, time.Since(startT))
			}
			return res
		}
	}

	// 5. Fetch schema for the ColumnSizer
	schema, err := s.source.DescribeTable(ctx, table.Name)
	if err != nil {
		res.Error = fmt.Errorf("failed to describe source table: %w", err)
		logCtx.Error("Table sync failed", "error", res.Error)
		if bar != nil {
			bar.Failed(barIdx, res.Error.Error(), 0, time.Since(startT))
		}
		return res
	}
	sizer := clickhouse.NewColumnSizer(schema)

	// 6. Transfer batches
	progressFn := func(inserted, total, bytesRaw uint64, throughputMBs float64) {
		if bar != nil {
			bar.Update(barIdx, inserted, total, bytesRaw, throughputMBs)
		} else if s.ui {
			// non-TTY plain update
		}
	}

	batches, inserted, bytesRaw, err := s.transferBatches(ctx, table, start, end, dateType, sourceCount, sizer, progressFn, logCtx)
	if err != nil {
		res.Error = fmt.Errorf("transfer failed: %w", err)
		logCtx.Error("Table sync failed", "error", res.Error)
		if bar != nil {
			bar.Failed(barIdx, res.Error.Error(), bytesRaw, time.Since(startT))
		}
		return res
	}
	res.BatchesProcessed = batches
	res.RowsInserted = inserted
	res.BytesTransferred = bytesRaw

	// 7. Count dest after
	var destCountAfter uint64
	err = s.withRetry(ctx, "count dest rows after", func() error {
		var e error
		destCountAfter, e = s.dest.CountRows(ctx, table.Name, table.DateColumn, start, end, dateType)
		return e
	})
	if err != nil {
		res.Error = fmt.Errorf("failed to count dest rows after: %w", err)
		logCtx.Error("Table sync failed", "error", res.Error)
		if bar != nil {
			bar.Failed(barIdx, res.Error.Error(), bytesRaw, time.Since(startT))
		}
		return res
	}
	res.DestRowsAfter = destCountAfter

	// 8. Verify
	res.Verified = destCountAfter == sourceCount
	res.Duration = time.Since(startT)

	avgRow := uint64(0)
	if inserted > 0 {
		avgRow = bytesRaw / inserted
	}

	if !res.Verified {
		logCtx.Warn("Verification failed",
			"source_count", sourceCount,
			"dest_after", destCountAfter,
			"total_bytes_raw", bytesRaw,
		)
	} else {
		logCtx.Info("Sync completed successfully",
			"inserted", inserted,
			"total_bytes_raw", bytesRaw,
			"avg_row_bytes", avgRow,
			"duration_ms", res.Duration.Milliseconds(),
		)
	}

	if bar != nil {
		bar.Done(barIdx, inserted, bytesRaw, avgRow, res.Duration)
	}

	return res
}

// transferBatches reads data from source in batches and inserts into dest.
// It returns (batchCount, rowsInserted, bytesRaw, error).
// progressFn is called after each successful insert; it may be nil.
func (s *Syncer) transferBatches(
	ctx context.Context,
	table config.TableConfig,
	start, end time.Time,
	dateType clickhouse.DateColumnType,
	sourceRowCount uint64,
	sizer *clickhouse.ColumnSizer,
	progressFn func(inserted, total, bytesRaw uint64, throughputMBs float64),
	logCtx *logging.Logger,
) (batches int, inserted uint64, bytesRaw uint64, err error) {
	columns, err := s.source.GetColumns(ctx, table.Name)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("get columns: %w", err)
	}

	batchSize := s.config.Sync.BatchSize
	if batchSize <= 0 {
		batchSize = 50000
	}

	reader, err := s.source.NewBatchReader(ctx, table.Name, columns, table.DateColumn, start, end, dateType, batchSize)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("new batch reader: %w", err)
	}
	defer reader.Close()

	transferStart := time.Now()
	lastLogTime := time.Now()

	for {
		batch, readErr := reader.ReadBatch()
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return batches, inserted, bytesRaw, fmt.Errorf("read batch: %w", readErr)
		}
		if batch == nil || batch.Count == 0 {
			break
		}

		// Measure raw ClickHouse bytes for this batch
		batchBytes := sizer.BatchBytes(batch.Rows)

		insertErr := s.withRetry(ctx, "insert batch", func() error {
			_, e := s.dest.InsertBatch(ctx, table.Name, batch.Columns, batch.Rows)
			return e
		})
		if insertErr != nil {
			return batches, inserted, bytesRaw, fmt.Errorf("insert batch: %w", insertErr)
		}

		batches++
		inserted += uint64(batch.Count)
		bytesRaw += batchBytes

		// Compute throughput
		elapsed := time.Since(transferStart).Seconds()
		throughputMBs := 0.0
		if elapsed > 0 {
			throughputMBs = float64(bytesRaw) / (1024 * 1024) / elapsed
		}

		// Notify progress callback
		if progressFn != nil {
			progressFn(inserted, sourceRowCount, bytesRaw, throughputMBs)
		}

		// Structured log progress (always, independent of --ui)
		now := time.Now()
		if batches%5 == 0 || now.Sub(lastLogTime) > 10*time.Second {
			pct := float64(inserted) / float64(sourceRowCount) * 100
			logCtx.Info("Transfer progress",
				"inserted", inserted,
				"total", sourceRowCount,
				"percent", fmt.Sprintf("%.1f%%", pct),
				"bytes_raw", bytesRaw,
				"batch_bytes_raw", batchBytes,
				"throughput_mbps", fmt.Sprintf("%.2f", throughputMBs),
			)
			lastLogTime = now
		}
	}

	return batches, inserted, bytesRaw, nil
}
