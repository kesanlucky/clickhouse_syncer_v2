package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"clickhouse-syncer/internal/cli"
	"clickhouse-syncer/internal/syncer"
)

func main() {
	// Set up signal handling for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cmd := cli.NewRootCommand()
	cmd.SetContext(ctx)

	// Execute root command
	if err := cmd.Execute(); err != nil {
		// Try to unwrap as a specific SyncerError to get exit code
		var syncErr *syncer.SyncerError
		if errors.As(err, &syncErr) {
			if syncErr.Err != nil {
				fmt.Fprintf(os.Stderr, "Error: %s: %v\n", syncErr.Message, syncErr.Err)
			} else {
				fmt.Fprintf(os.Stderr, "Error: %s\n", syncErr.Message)
			}
			os.Exit(int(syncErr.Code))
		}

		// Fallback for general errors
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
