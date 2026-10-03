package ui

import (
	"fmt"
	"strings"
	"time"
)

const lineWidth = 62

// PrintHeader renders the operation header to stdout.
// It is a no-op when stdout is not a TTY.
func PrintHeader(op, target string, tableCount int) {
	if !IsTTY() {
		return
	}
	fmt.Println(AnsiCyan + strings.Repeat("═", lineWidth) + AnsiReset)
	fmt.Printf("  %sClickHouse Data Syncer%s\n", AnsiBold, AnsiReset)
	fmt.Printf("  Operation   : %s%s%s\n", AnsiBold, op, AnsiReset)
	fmt.Printf("  Target      : %s\n", target)
	fmt.Printf("  Tables      : %d\n", tableCount)
	fmt.Printf("  Started     : %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(AnsiCyan + strings.Repeat("═", lineWidth) + AnsiReset)
}
