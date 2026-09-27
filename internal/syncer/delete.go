package syncer

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"clickhouse-syncer/internal/clickhouse"
	"clickhouse-syncer/internal/config"
	"clickhouse-syncer/internal/logging"
)

func (s *Syncer) deleteTable(ctx context.Context, table config.TableConfig, cutoff time.Time, dateType clickhouse.DateColumnType, tableIndex, tableTotal int) TableDeleteResult {
	startT := time.Now()
	res := TableDeleteResult{
		Table: table.Name,
	}

	logCtx := s.logger.With(
		"table", table.Name,
		"index", tableIndex,
		"total", tableTotal,
		"op", logging.GenerateOperationID("DELETE", cutoff.Format("2006-01-02")),
	)

	logCtx.Info("Starting table delete")

	// 1. Count dest before cutoff
	var destCountBefore uint64
	err := s.withRetry(ctx, "count dest rows before delete", func() error {
		var e error
		destCountBefore, e = s.dest.CountRowsBefore(ctx, table.Name, table.DateColumn, cutoff, dateType)
		return e
	})
	if err != nil {
		res.Error = fmt.Errorf("failed to count dest rows: %w", err)
		logCtx.Error("Table delete failed", "error", res.Error)
		return res
	}
	res.RowsBeforeCutoff = destCountBefore

	if destCountBefore == 0 {
		logCtx.Info("No rows to delete")
		res.Verified = true
		res.Duration = time.Since(startT)
		return res
	}

	// 2. Delete dest rows
	err = s.withRetry(ctx, "delete dest rows", func() error {
		return s.dest.DeleteBefore(ctx, table.Name, table.DateColumn, cutoff, dateType)
	})
	if err != nil {
		res.Error = fmt.Errorf("failed to delete dest rows: %w", err)
		logCtx.Error("Table delete failed", "error", res.Error)
		return res
	}

	// 3. Wait for mutations
	err = s.dest.WaitForMutations(ctx, table.Name, 5*time.Minute)
	if err != nil {
		res.Error = fmt.Errorf("failed waiting for mutations: %w", err)
		logCtx.Error("Table delete failed", "error", res.Error)
		return res
	}

	// 4. Count dest after cutoff
	var destCountAfter uint64
	err = s.withRetry(ctx, "count dest rows after delete", func() error {
		var e error
		destCountAfter, e = s.dest.CountRowsBefore(ctx, table.Name, table.DateColumn, cutoff, dateType)
		return e
	})
	if err != nil {
		res.Error = fmt.Errorf("failed to count dest rows after: %w", err)
		logCtx.Error("Table delete failed", "error", res.Error)
		return res
	}
	res.RowsAfterDelete = destCountAfter

	// 5. Verify
	res.Verified = destCountAfter == 0
	if !res.Verified {
		logCtx.Warn("Verification failed", "rows_remaining", destCountAfter)
	} else {
		logCtx.Info("Delete completed successfully", "deleted", destCountBefore, "duration", time.Since(startT))
	}

	res.Duration = time.Since(startT)
	return res
}

func (s *Syncer) confirmDelete(tables []config.TableConfig, beforeDate string) (bool, error) {
	// Simple check if stdin is a terminal, if not, we fail safe.
	fileInfo, _ := os.Stdin.Stat()
	if (fileInfo.Mode() & os.ModeCharDevice) == 0 {
		fmt.Println("Warning: Non-interactive terminal detected, use --force to confirm deletion.")
		return false, nil
	}

	fmt.Println("\n" + strings.Repeat("!", 60))
	fmt.Println("WARNING: DATA DELETION")
	fmt.Println(strings.Repeat("!", 60))
	fmt.Printf("You are about to PERMANENTLY DELETE data BEFORE %s.\n", beforeDate)
	fmt.Printf("This will affect %d tables on the destination database.\n", len(tables))
	fmt.Println("This operation CANNOT BE UNDONE.")
	fmt.Println(strings.Repeat("!", 60))

	fmt.Print("\nType 'DELETE' to confirm: ")

	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	text = strings.TrimSpace(text)
	return text == "DELETE", nil
}
