package ui

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const barWidth = 22

// barState represents the lifecycle state of one table's progress bar.
type barState int

const (
	barPending barState = iota
	barRunning
	barDone
	barFailed
	barSkipped // zero-row tables
)

// tableBar holds the display state for one table.
type tableBar struct {
	name       string
	total      uint64
	inserted   uint64
	bytes      uint64
	throughput float64 // MB/s
	avgRow     uint64
	state      barState
	errMsg     string
	duration   time.Duration
}

// MultiBar renders one live ANSI progress bar per table.
// All public methods are safe for concurrent use.
type MultiBar struct {
	mu    sync.Mutex
	bars  []*tableBar
	tty   bool
	drawn bool // whether the first frame has been printed
}

// NewMultiBar creates a MultiBar for the given table names and prints the
// initial (pending) frame immediately.
func NewMultiBar(names []string) *MultiBar {
	bars := make([]*tableBar, len(names))
	for i, n := range names {
		bars[i] = &tableBar{name: n, state: barPending}
	}
	mb := &MultiBar{bars: bars, tty: IsTTY()}
	mb.mu.Lock()
	mb.render()
	mb.mu.Unlock()
	return mb
}

// SetTotal sets the expected row count and transitions the bar to running.
func (mb *MultiBar) SetTotal(idx int, total uint64) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if idx >= len(mb.bars) {
		return
	}
	mb.bars[idx].total = total
	mb.bars[idx].state = barRunning
	mb.render()
}

// Update refreshes a running bar with current progress.
func (mb *MultiBar) Update(idx int, inserted, total, bytesRaw uint64, throughputMBs float64) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if idx >= len(mb.bars) {
		return
	}
	b := mb.bars[idx]
	b.inserted = inserted
	b.total = total
	b.bytes = bytesRaw
	b.throughput = throughputMBs
	b.state = barRunning
	mb.render()
}

// Done marks a bar as successfully completed.
func (mb *MultiBar) Done(idx int, inserted, bytesRaw, avgRow uint64, duration time.Duration) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if idx >= len(mb.bars) {
		return
	}
	b := mb.bars[idx]
	b.inserted = inserted
	if b.total == 0 {
		b.total = inserted
	}
	b.bytes = bytesRaw
	b.avgRow = avgRow
	b.duration = duration
	if inserted == 0 {
		b.state = barSkipped
	} else {
		b.state = barDone
	}
	mb.render()
}

// Failed marks a bar as failed with an error message.
func (mb *MultiBar) Failed(idx int, errMsg string, bytesRaw uint64, duration time.Duration) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if idx >= len(mb.bars) {
		return
	}
	b := mb.bars[idx]
	b.errMsg = errMsg // full message preserved; renderBar truncates for display
	b.bytes = bytesRaw
	b.duration = duration
	b.state = barFailed
	mb.render()
}

// render redraws all bars. Must be called with mb.mu held.
func (mb *MultiBar) render() {
	if !mb.tty {
		return
	}

	if mb.drawn {
		// Move cursor up to the start of the bars block.
		// Each bar occupies exactly 1 line.
		fmt.Printf("\033[%dA", len(mb.bars))
	}
	mb.drawn = true

	for _, b := range mb.bars {
		// Clear the line, go to column 0, print bar.
		fmt.Printf("\r\033[2K%s\n", mb.renderBar(b))
	}
}

// renderBar returns the single-line string for one table bar.
func (mb *MultiBar) renderBar(b *tableBar) string {
	name := padRight(truncate(b.name, 26), 26)

	switch b.state {
	case barPending:
		bar := AnsiDim + strings.Repeat("░", barWidth) + AnsiReset
		return fmt.Sprintf("  %s  [%s]  %spending%s", name, bar, AnsiDim, AnsiReset)

	case barRunning:
		pct := 0.0
		filled := 0
		if b.total > 0 {
			pct = float64(b.inserted) / float64(b.total) * 100
			filled = int(math.Round(float64(barWidth) * float64(b.inserted) / float64(b.total)))
		}
		if filled > barWidth {
			filled = barWidth
		}
		bar := AnsiGreen + strings.Repeat("█", filled) + AnsiReset + strings.Repeat("░", barWidth-filled)
		tp := ""
		if b.throughput > 0 {
			tp = fmt.Sprintf("  %.1f MB/s", b.throughput)
		}
		return fmt.Sprintf("  %s  [%s]  %5.1f%%   %s / %s rows   %s%s",
			name, bar, pct,
			FormatNum(b.inserted), FormatNum(b.total),
			FormatBytes(b.bytes), tp)

	case barDone:
		bar := AnsiGreen + strings.Repeat("█", barWidth) + AnsiReset
		avgStr := ""
		if b.avgRow > 0 {
			avgStr = fmt.Sprintf("  ~%s/row", FormatBytes(b.avgRow))
		}
		return fmt.Sprintf("  %s  [%s]  %s%s✓%s   %s rows   %s%s   %s",
			name, bar,
			AnsiBold, AnsiGreen, AnsiReset,
			FormatNum(b.inserted), FormatBytes(b.bytes), avgStr,
			b.duration.Round(time.Millisecond))

	case barSkipped:
		bar := AnsiDim + strings.Repeat("─", barWidth) + AnsiReset
		return fmt.Sprintf("  %s  [%s]  %s–%s  no rows", name, bar, AnsiDim, AnsiReset)

	case barFailed:
		// Fill bar proportionally if we have progress data
		filled := 0
		if b.total > 0 && b.inserted > 0 {
			filled = int(math.Round(float64(barWidth) * float64(b.inserted) / float64(b.total)))
		}
		if filled > barWidth {
			filled = barWidth
		}
		bar := AnsiRed + strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled) + AnsiReset
		errPart := ""
		if b.errMsg != "" {
			errPart = fmt.Sprintf("  %s→ %s%s", AnsiRed, b.errMsg, AnsiReset)
		}
		return fmt.Sprintf("  %s  [%s]  %s%sFAILED%s   %s%s",
			name, bar,
			AnsiBold, AnsiRed, AnsiReset,
			FormatBytes(b.bytes), errPart)
	}
	return ""
}

// PrintPlainUpdate prints a plain-text progress update for non-TTY environments.
func (mb *MultiBar) PrintPlainUpdate(idx int, inserted, total, bytesRaw uint64) {
	if mb.tty || idx >= len(mb.bars) {
		return
	}
	pct := 0.0
	if total > 0 {
		pct = float64(inserted) / float64(total) * 100
	}
	fmt.Printf("[%s] %.1f%%  %s / %s rows  %s\n",
		mb.bars[idx].name, pct, FormatNum(inserted), FormatNum(total), FormatBytes(bytesRaw))
}

// helpers

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max-1]) + "…"
}

func padRight(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}
