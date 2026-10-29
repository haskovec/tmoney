package report

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/haskovec/tmoney/internal/chart"
	reportdom "github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/termimg"
	"github.com/haskovec/tmoney/internal/types"
)

// The CLI pie: its height in rows, the width of the legend beside it when
// no value is wider than allocationValueWidth, the cells between the two,
// and how long the terminal gets to reply. See
// specs/design-asset-allocation-report.md §6.6.
const (
	allocationPieRows        = 16
	allocationValueWidth     = 13
	allocationLegendMinWidth = 39 + allocationValueWidth
	allocationPieGap         = 3
	allocationProbeTimeout   = 300 * time.Millisecond
)

// cliPie is how the report draws its pies: the placement columns, the image
// pixels, the class colors, and the terminal width. A nil *cliPie prints
// text.
type cliPie struct {
	cols, w, h int
	dark       bool
	width      int
}

// probeCLIPie asks the terminal whether it shows the pies, when w is a
// terminal and the chart_images setting allows it. A pipe, a file and an AI
// agent's output get text, and no query.
func probeCLIPie(w io.Writer, mode termimg.Mode) *cliPie {
	f, ok := w.(*os.File)
	if !ok || mode == termimg.ModeOff {
		return nil
	}
	width, ok := termimg.TerminalWidth(f)
	if !ok {
		return nil
	}
	reply, err := termimg.Probe(allocationProbeTimeout)
	if err != nil {
		return nil
	}
	cols, pw, ph := termimg.Placement(allocationPieRows, reply.CellW, reply.CellH)
	// Each section checks its own legend too (printAllocationPie); this is
	// the narrowest one.
	fits := width >= cols+allocationPieGap+allocationLegendMinWidth
	if !termimg.Show(mode, reply.KittyOK, termimg.InTmux(), fits) {
		return nil
	}
	return &cliPie{cols: cols, w: pw, h: ph, dark: reply.Dark(), width: width}
}

// printAllocationPie prints a section's pie with its legend on the right:
// the transmit of the image with a new random ID, then the placeholder
// lines. The pies stay in the scrollback, so the CLI does not delete them.
// It returns false, and prints nothing, when the legend does not fit beside
// the pie, so that no line wraps, or when the image cannot be encoded.
func printAllocationPie(w io.Writer, sec reportdom.AllocationSection, pie *cliPie) bool {
	legend, legendWidth := allocationLegend(sec, pie.dark)
	if pie.cols+allocationPieGap+legendWidth > pie.width {
		return false
	}
	id := termimg.NewID()
	img := chart.Pie(sec.PieSlices(pie.dark), pie.w, pie.h, 2)
	seq, err := termimg.Transmit(img, id, pie.cols, allocationPieRows)
	if err != nil {
		return false
	}
	fmt.Fprint(w, seq)
	gap := strings.Repeat(" ", allocationPieGap)
	for y := range max(allocationPieRows, len(legend)) {
		left := strings.Repeat(" ", pie.cols)
		if y < allocationPieRows {
			left = termimg.PlaceholderLine(id, y, pie.cols)
		}
		right := ""
		if y < len(legend) {
			right = legend[y]
		}
		fmt.Fprintln(w, strings.TrimRight(left+gap+right, " "))
	}
	return true
}

// allocationLegend is the table beside a pie: the header, the class rows
// with their swatches, and the TOTAL, and its width. The Value column is as
// wide as the widest value, at least allocationValueWidth, so that the rows
// stay aligned: a currency code makes a value wider ("JPY 1500000.00"). The
// swatch is the only text in a class color (rule 9).
func allocationLegend(sec reportdom.AllocationSection, dark bool) ([]string, int) {
	money := func(m types.Money) string { return formatTotal(m, sec.Currency, true, false) }
	total := formatTotal(sec.Value, sec.Currency, sec.Available, false)
	valueWidth := max(allocationValueWidth, utf8.RuneCountInString(total))
	for _, c := range sec.Shown() {
		valueWidth = max(valueWidth, utf8.RuneCountInString(money(c.Value)))
	}
	row := func(swatch, class, count, value, pct string) string {
		return fmt.Sprintf("%s %-19s %8s %*s %7s", swatch, class, count, valueWidth, value, pct)
	}
	width := allocationLegendMinWidth - allocationValueWidth + valueWidth
	rule := strings.Repeat("─", width)
	lines := []string{row(" ", "Asset Class", "Holdings", "Value", "% Total"), rule}
	for _, c := range sec.Shown() {
		col := chart.ClassColor(c.Class, dark)
		swatch := fmt.Sprintf("\x1b[38;2;%d;%d;%dm■\x1b[39m", col.R, col.G, col.B)
		lines = append(lines, row(swatch, c.Class.DisplayName(), fmt.Sprint(c.Count()), money(c.Value), fmt.Sprintf("%.1f%%", c.Percent)))
	}
	label := fmt.Sprintf("TOTAL (%s)", sec.Currency)
	if sec.Available {
		lines = append(lines, rule, row(" ", label, fmt.Sprint(sec.Count()), total, "100.0%"))
	} else {
		lines = append(lines, rule, row(" ", label, "", total, ""))
	}
	return lines, width
}
