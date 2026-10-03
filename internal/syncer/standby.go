package syncer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"clickhouse-syncer/internal/ui"
)

func (s *Syncer) runStandbyLoop(ctx context.Context) error {
	interval, err := s.config.GetStandbyInterval()
	if err != nil {
		return &SyncerError{Code: ExitConfigError, Message: "invalid standby interval", Err: err}
	}

	if s.ui {
		ui.PrintHeader("STANDBY SYNC", fmt.Sprintf("every %v", interval), len(s.config.Tables))
	}

	cycleNum := 1

	for {
		start := time.Now()

		successful, failed, cycleErr := s.runStandbyCycle(ctx, cycleNum)
		if cycleErr != nil {
			if s.isFatalError(cycleErr) {
				return cycleErr
			}
			s.logger.Error("Standby cycle failed with transient error", "error", cycleErr)
		}

		elapsed := time.Since(start).Round(time.Millisecond)
		s.logger.Info("Standby cycle completed",
			"cycle", cycleNum,
			"duration", elapsed,
			"successful", successful,
			"failed", failed,
		)
		if s.ui {
			statusColour := ui.AnsiGreen
			if failed > 0 {
				statusColour = ui.AnsiRed
			}
			fmt.Printf("\n  Cycle %d  done in %v  —  %s%d ok  %d failed%s\n",
				cycleNum, elapsed, statusColour, successful, failed, ui.AnsiReset)
			fmt.Printf("  Next cycle in %v  (Ctrl+C to exit)...\n", interval)
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			if s.ui {
				fmt.Println("\nShutdown signal received. Exiting standby mode.")
			}
			return nil
		case <-timer.C:
			// next cycle
		}
		cycleNum++
	}
}

func (s *Syncer) runStandbyCycle(ctx context.Context, cycleNum int) (int, int, error) {
	now := time.Now().In(s.timezone)
	dateStr := now.Format("2006-01-02")

	s.logger.Info("Starting standby cycle", "cycle", cycleNum, "target_date", dateStr)

	summary, err := s.RunSync(ctx, dateStr, nil)
	if err != nil {
		return 0, 0, err
	}

	return summary.TablesSuccessful, summary.TablesFailed, nil
}

func (s *Syncer) isFatalError(err error) bool {
	var syncerErr *SyncerError
	if errors.As(err, &syncerErr) {
		switch syncerErr.Code {
		case ExitConfigError, ExitValidationError, ExitSchemaMismatch:
			return true
		}
	}
	return false
}
