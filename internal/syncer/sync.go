package syncer

import (
	"context"
	"fmt"
	"io"
	"time"

	"clickhouse-syncer/internal/clickhouse"
	"clickhouse-syncer/internal/config"
	"clickhouse-syncer/internal/logging"
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

func (s *Syncer) syncTable(ctx context.Context, table config.TableConfig, start, end time.Time, dateType clickhouse.DateColumnType, tableIndex, tableTotal int) TableSyncResult {
	startT := time.Now()
	res := TableSyncResult{
		Table: table.Name,
	}

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
		return res
	}
	res.SourceRows = sourceCount

	if sourceCount == 0 {
		logCtx.Info("No rows to sync")
		res.Verified = true
		res.Duration = time.Since(startT)
		return res
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
			return res
		}

		// 4. Wait for mutations
		err = s.dest.WaitForMutations(ctx, table.Name, 5*time.Minute)
		if err != nil {
			res.Error = fmt.Errorf("failed waiting for mutations: %w", err)
			logCtx.Error("Table sync failed", "error", res.Error)
			return res
		}
	}

	// 5. Transfer batches
	batches, inserted, err := s.transferBatches(ctx, table, start, end, dateType, sourceCount)
	if err != nil {
		res.Error = fmt.Errorf("transfer failed: %w", err)
		logCtx.Error("Table sync failed", "error", res.Error)
		return res
	}
	res.BatchesProcessed = batches
	res.RowsInserted = inserted

	// 6. Count dest after
	var destCountAfter uint64
	err = s.withRetry(ctx, "count dest rows after", func() error {
		var e error
		destCountAfter, e = s.dest.CountRows(ctx, table.Name, table.DateColumn, start, end, dateType)
		return e
	})
	if err != nil {
		res.Error = fmt.Errorf("failed to count dest rows after: %w", err)
		logCtx.Error("Table sync failed", "error", res.Error)
		return res
	}
	res.DestRowsAfter = destCountAfter

	// 7. Verify
	res.Verified = destCountAfter == sourceCount
	if !res.Verified {
		logCtx.Warn("Verification failed", "source_count", sourceCount, "dest_after", destCountAfter)
	} else {
		logCtx.Info("Sync completed successfully", "inserted", inserted, "duration", time.Since(startT))
	}

	res.Duration = time.Since(startT)
	return res
}

func (s *Syncer) transferBatches(ctx context.Context, table config.TableConfig, start, end time.Time, dateType clickhouse.DateColumnType, sourceRowCount uint64) (int, uint64, error) {
	columns, err := s.source.GetColumns(ctx, table.Name)
	if err != nil {
		return 0, 0, fmt.Errorf("get columns: %w", err)
	}

	batchSize := s.config.Sync.BatchSize
	if batchSize <= 0 {
		batchSize = 50000
	}

	reader, err := s.source.NewBatchReader(ctx, table.Name, columns, table.DateColumn, start, end, dateType, batchSize)
	if err != nil {
		return 0, 0, fmt.Errorf("new batch reader: %w", err)
	}
	defer reader.Close()

	var totalInserted uint64
	var batchesProcessed int
	lastLogTime := time.Now()

	for {
		batch, err := reader.ReadBatch()
		if err != nil {
			if err == io.EOF {
				break // All data read
			}
			return batchesProcessed, totalInserted, fmt.Errorf("read batch: %w", err)
		}
		if batch == nil || batch.Count == 0 {
			break
		}

		err = s.withRetry(ctx, "insert batch", func() error {
			_, err := s.dest.InsertBatch(ctx, table.Name, batch.Columns, batch.Rows)
			return err
		})
		if err != nil {
			return batchesProcessed, totalInserted, fmt.Errorf("insert batch: %w", err)
		}

		batchesProcessed++
		totalInserted += uint64(batch.Count)

		// Log progress
		now := time.Now()
		if batchesProcessed%5 == 0 || now.Sub(lastLogTime) > 10*time.Second {
			pct := float64(totalInserted) / float64(sourceRowCount) * 100
			s.logger.Info("Transfer progress",
				"table", table.Name,
				"inserted", totalInserted,
				"total", sourceRowCount,
				"percent", fmt.Sprintf("%.1f%%", pct),
			)
			lastLogTime = now
		}
	}

	return batchesProcessed, totalInserted, nil
}
