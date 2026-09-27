package syncer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Syncer) runStandbyLoop(ctx context.Context) error {
	interval, err := s.config.GetStandbyInterval()
	if err != nil {
		return &SyncerError{Code: ExitConfigError, Message: "invalid standby interval", Err: err}
	}

	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("ClickHouse Data Syncer - STANDBY MODE")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Interval: %v\n", interval)
	fmt.Printf("Tables: %d\n", len(s.config.Tables))
	fmt.Printf("Timezone: %s\n", s.timezone.String())
	fmt.Println(strings.Repeat("=", 60))

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

		fmt.Printf("\nCycle %d completed in %v. Successful: %d, Failed: %d\n", cycleNum, time.Since(start).Round(time.Millisecond), successful, failed)

		timer := time.NewTimer(interval)
		fmt.Printf("Waiting %v for next cycle (CTRL+C to exit)...\n", interval)

		select {
		case <-ctx.Done():
			timer.Stop()
			fmt.Println("\nShutdown signal received. Exiting standby mode.")
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
