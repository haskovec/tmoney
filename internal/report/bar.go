package report

import (
	"strings"

	"github.com/haskovec/tmoney/internal/types"
)

// barEighths are the partial blocks of a bar, from one eighth of a cell to
// seven eighths.
var barEighths = []string{"▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// Bar draws value as a bar of width cells, where largest fills all of them.
// A part of a cell is drawn to the nearest eighth below, and spaces pad the
// bar to width. The bar is empty when value or largest is zero or less.
func Bar(value, largest types.Money, width int) string {
	if width <= 0 {
		return ""
	}
	eighths := 0
	if value.IsPositive() && largest.IsPositive() {
		eighths = int(value.Float64() / largest.Float64() * float64(width*8))
		eighths = min(eighths, width*8)
	}
	var b strings.Builder
	b.WriteString(strings.Repeat("█", eighths/8))
	cells := eighths / 8
	if part := eighths % 8; part > 0 {
		b.WriteString(barEighths[part-1])
		cells++
	}
	b.WriteString(strings.Repeat(" ", width-cells))
	return b.String()
}
