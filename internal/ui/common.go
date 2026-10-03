package ui

import (
	"fmt"
	"os"
)

// IsTTY reports whether stdout is an interactive terminal.
// When false, ANSI escape codes are suppressed.
func IsTTY() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// FormatBytes formats a byte count as a human-readable string (B / KB / MB / GB).
func FormatBytes(b uint64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// FormatNum formats a row count with K/M suffix for brevity.
func FormatNum(n uint64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		s := fmt.Sprintf("%d", n)
		// Add comma separators
		out := make([]byte, 0, len(s)+len(s)/3)
		for i, c := range s {
			if i > 0 && (len(s)-i)%3 == 0 {
				out = append(out, ',')
			}
			out = append(out, byte(c))
		}
		return string(out)
	}
}

// ANSI colour / style codes
const (
	AnsiReset  = "\033[0m"
	AnsiGreen  = "\033[32m"
	AnsiRed    = "\033[31m"
	AnsiYellow = "\033[33m"
	AnsiBold   = "\033[1m"
	AnsiDim    = "\033[2m"
	AnsiCyan   = "\033[36m"
)
