package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// SyncTableRow is the data for one table row in the sync summary table.
type SyncTableRow struct {
	Table    string
	OK       bool
	Verified bool
	Rows     uint64
	Bytes    uint64
	Duration time.Duration
	Error    string
}

// DeleteTableRow is the data for one table row in the delete summary table.
type DeleteTableRow struct {
	Table    string
	OK       bool
	Verified bool
	Rows     uint64
	Duration time.Duration
	Error    string
}

// PrintSyncSummary renders a box-drawn sync summary table with a totals line.
func PrintSyncSummary(rows []SyncTableRow, date string, totalRows, totalBytes uint64, totalDuration time.Duration, failed int) {
	tty := IsTTY()
	lw := lineWidth

	// Column widths
	const (
		wTable    = 30
		wStatus   = 10
		wRows     = 12
		wRawSize  = 10
		wAvgRow   = 12
		wDuration = 9
	)

	if tty {
		fmt.Printf("\n  %sSYNC SUMMARY%s\n", AnsiBold, AnsiReset)
		printBoxTop(wTable, wStatus, wRows, wRawSize, wAvgRow, wDuration)
		printBoxHeader("Table", "Status", "Rows", "Raw Size", "Avg Row", "Duration",
			wTable, wStatus, wRows, wRawSize, wAvgRow, wDuration)
		printBoxDivider(wTable, wStatus, wRows, wRawSize, wAvgRow, wDuration)
		for _, r := range rows {
			statusStr, statusColour := syncStatus(r)
			rowStr := "—"
			if r.OK {
				rowStr = FormatNum(r.Rows)
			}
			sizeStr := FormatBytes(r.Bytes)
			avgStr := "—"
			if r.OK && r.Rows > 0 && r.Bytes > 0 {
				avgStr = FormatBytes(r.Bytes/r.Rows) + "/row"
			}
			durStr := r.Duration.Round(time.Millisecond).String()

			fmt.Printf("  │ %s%s│ %s%-*s%s│ %s│ %s│ %s│ %s│\n",
				padRight(truncate(r.Table, wTable-1), wTable-1),
				" ",
				statusColour, wStatus-1, statusStr, AnsiReset+" ",
				padLeft(rowStr, wRows-1)+" ",
				padLeft(sizeStr, wRawSize-1)+" ",
				padLeft(avgStr, wAvgRow-1)+" ",
				padLeft(durStr, wDuration-1)+" ",
			)
		}
		printBoxBottom(wTable, wStatus, wRows, wRawSize, wAvgRow, wDuration)
	} else {
		fmt.Println("\nSYNC SUMMARY")
		fmt.Println(strings.Repeat("-", lw))
		for _, r := range rows {
			status := "✓"
			if !r.OK {
				status = "✗"
			} else if !r.Verified {
				status = "!"
			}
			if r.OK {
				fmt.Printf("  %s  %-30s  %s rows  %s  %s\n",
					status, r.Table, FormatNum(r.Rows), FormatBytes(r.Bytes), r.Duration.Round(time.Millisecond))
			} else {
				fmt.Printf("  %s  %-30s  FAILED: %s\n", status, r.Table, r.Error)
			}
		}
		fmt.Println(strings.Repeat("-", lw))
	}

	// Totals line
	successCount := len(rows) - failed
	totalsLine := fmt.Sprintf("Synced %d / %d tables  •  %s rows  •  %s raw  •  %s total",
		successCount, len(rows),
		FormatNum(totalRows), FormatBytes(totalBytes),
		totalDuration.Round(time.Millisecond))
	if tty {
		colour := AnsiGreen
		if failed > 0 {
			colour = AnsiRed
		}
		fmt.Printf("  %s%s%s\n", colour, totalsLine, AnsiReset)
	} else {
		fmt.Println(totalsLine)
		fmt.Println()
	}

	// Print full error details box for failed tables
	if tty && failed > 0 {
		printErrorBox(func(yield func(table, errMsg string)) {
			for _, r := range rows {
				if !r.OK && r.Error != "" {
					yield(r.Table, r.Error)
				}
			}
		})
	}
}

// PrintDeleteSummary renders a box-drawn delete summary table with a totals line.
func PrintDeleteSummary(rows []DeleteTableRow, beforeDate string, totalRows uint64, totalDuration time.Duration, failed int) {
	tty := IsTTY()
	lw := lineWidth

	const (
		wTable    = 30
		wStatus   = 10
		wRows     = 15
		wDuration = 9
	)

	if tty {
		fmt.Printf("\n  %sDELETE SUMMARY%s\n", AnsiBold, AnsiReset)
		printBoxTopSimple(wTable, wStatus, wRows, wDuration)
		printBoxHeaderSimple("Table", "Status", "Rows Deleted", "Duration",
			wTable, wStatus, wRows, wDuration)
		printBoxDividerSimple(wTable, wStatus, wRows, wDuration)
		for _, r := range rows {
			statusStr, statusColour := deleteStatus(r)
			rowStr := "—"
			if r.OK {
				rowStr = FormatNum(r.Rows)
			}
			durStr := r.Duration.Round(time.Millisecond).String()
			fmt.Printf("  │ %s │ %s%-*s%s│ %s│ %s│\n",
				padRight(truncate(r.Table, wTable-1), wTable-1),
				statusColour, wStatus-1, statusStr, AnsiReset+" ",
				padLeft(rowStr, wRows-1)+" ",
				padLeft(durStr, wDuration-1)+" ",
			)
		}
		printBoxBottomSimple(wTable, wStatus, wRows, wDuration)
	} else {
		fmt.Println("\nDELETE SUMMARY")
		fmt.Println(strings.Repeat("-", lw))
		for _, r := range rows {
			status := "✓"
			if !r.OK {
				status = "✗"
			}
			if r.OK {
				fmt.Printf("  %s  %-30s  %s rows deleted  %s\n",
					status, r.Table, FormatNum(r.Rows), r.Duration.Round(time.Millisecond))
			} else {
				fmt.Printf("  %s  %-30s  FAILED: %s\n", status, r.Table, r.Error)
			}
		}
		fmt.Println(strings.Repeat("-", lw))
	}

	successCount := len(rows) - failed
	totalsLine := fmt.Sprintf("Deleted from %d / %d tables  •  %s rows  •  %s total",
		successCount, len(rows), FormatNum(totalRows), totalDuration.Round(time.Millisecond))
	if tty {
		colour := AnsiGreen
		if failed > 0 {
			colour = AnsiRed
		}
		fmt.Printf("  %s%s%s\n", colour, totalsLine, AnsiReset)
	} else {
		fmt.Println(totalsLine)
		fmt.Println()
	}

	// Print full error details box for failed tables
	if tty && failed > 0 {
		printErrorBox(func(yield func(table, errMsg string)) {
			for _, r := range rows {
				if !r.OK && r.Error != "" {
					yield(r.Table, r.Error)
				}
			}
		})
	}
}

// ─── box drawing helpers ───────────────────────────────────────────────────

func syncStatus(r SyncTableRow) (string, string) {
	if !r.OK {
		return "✗ FAILED", AnsiBold + AnsiRed
	}
	if !r.Verified {
		return "! WARN", AnsiBold + AnsiYellow
	}
	return "✓ OK", AnsiBold + AnsiGreen
}

func deleteStatus(r DeleteTableRow) (string, string) {
	if !r.OK {
		return "✗ FAILED", AnsiBold + AnsiRed
	}
	if !r.Verified {
		return "! WARN", AnsiBold + AnsiYellow
	}
	return "✓ OK", AnsiBold + AnsiGreen
}

func printBoxTop(w ...int) {
	parts := make([]string, len(w))
	for i, c := range w {
		parts[i] = strings.Repeat("─", c+1)
	}
	fmt.Printf("  ┌%s┐\n", strings.Join(parts, "┬"))
}

func printBoxHeader(cols ...interface{}) {
	// cols: name0, name1, ..., w0, w1, ...
	n := len(cols) / 2
	names := cols[:n]
	widths := cols[n:]
	fmt.Print("  │")
	for i := 0; i < n; i++ {
		w := widths[i].(int)
		fmt.Printf(" %-*s│", w, names[i])
	}
	fmt.Println()
}

func printBoxDivider(w ...int) {
	parts := make([]string, len(w))
	for i, c := range w {
		parts[i] = strings.Repeat("─", c+1)
	}
	fmt.Printf("  ├%s┤\n", strings.Join(parts, "┼"))
}

func printBoxBottom(w ...int) {
	parts := make([]string, len(w))
	for i, c := range w {
		parts[i] = strings.Repeat("─", c+1)
	}
	fmt.Printf("  └%s┘\n", strings.Join(parts, "┴"))
}

// Simple 4-column variants for delete summary
func printBoxTopSimple(w ...int)     { printBoxTop(w...) }
func printBoxDividerSimple(w ...int) { printBoxDivider(w...) }
func printBoxBottomSimple(w ...int)  { printBoxBottom(w...) }

func printBoxHeaderSimple(cols ...interface{}) { printBoxHeader(cols...) }

func padLeft(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return strings.Repeat(" ", width-n) + s
}

// printErrorBox renders a titled box containing the full error message for
// each failed table. Long errors are word-wrapped to fit within the box.
func printErrorBox(iter func(yield func(table, errMsg string))) {
	const boxW = 70 // inner text width (excluding border chars)
	border := strings.Repeat("─", boxW+2)

	fmt.Printf("\n  %s%s● ERRORS%s\n", AnsiBold, AnsiRed, AnsiReset)

	iter(func(table, errMsg string) {
		// Top border
		fmt.Printf("  %s┌%s┐%s\n", AnsiRed, border, AnsiReset)
		// Table name header
		header := " ✗ " + table
		fmt.Printf("  %s│%s%-*s%s%s│%s\n",
			AnsiRed, AnsiBold+AnsiRed, boxW+1, header, AnsiReset, AnsiRed, AnsiReset)
		// Divider
		fmt.Printf("  %s├%s┤%s\n", AnsiRed, border, AnsiReset)
		// Word-wrap the error message
		for _, line := range wrapText(errMsg, boxW) {
			fmt.Printf("  %s│%s %-*s %s%s│%s\n",
				AnsiRed, AnsiReset, boxW-1, line, AnsiRed, "", AnsiReset)
		}
		// Bottom border
		fmt.Printf("  %s└%s┘%s\n", AnsiRed, border, AnsiReset)
	})
	fmt.Println()
}

// wrapText splits text into lines of at most maxWidth runes, breaking on
// word boundaries where possible.
func wrapText(text string, maxWidth int) []string {
	var lines []string
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}
	current := ""
	for _, word := range words {
		wl := utf8.RuneCountInString(word)
		cl := utf8.RuneCountInString(current)
		if cl == 0 {
			if wl > maxWidth {
				// Force-break overlong word
				runes := []rune(word)
				for len(runes) > 0 {
					chunk := maxWidth
					if chunk > len(runes) {
						chunk = len(runes)
					}
					lines = append(lines, string(runes[:chunk]))
					runes = runes[chunk:]
				}
				continue
			}
			current = word
		} else if cl+1+wl > maxWidth {
			lines = append(lines, current)
			current = word
		} else {
			current += " " + word
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
