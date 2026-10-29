package report

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/haskovec/tmoney/internal/chart"
	reportdom "github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/termimg"
	"github.com/haskovec/tmoney/internal/types"
)

// The CLI pie: its height in rows, the width of the legend beside it, the
// cells between the two, and how long the terminal gets to reply. See
// specs/design-asset-allocation-report.md §6.6.
const (
	allocationPieRows      = 16
	allocationLegendWidth  = 52
	allocationPieGap       = 3
	allocationProbeTimeout = 300 * time.Millisecond
)

// cliPie is how the report draws its pies: the placement columns, the image
// pixels, and the class colors. A nil *cliPie prints text.
type cliPie struct {
	cols, w, h int
	dark       bool
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
	fits := width >= cols+allocationPieGap+allocationLegendWidth
	if !termimg.Show(mode, reply.KittyOK, termimg.InTmux(), fits) {
		return nil
	}
	return &cliPie{cols: cols, w: pw, h: ph, dark: reply.Dark()}
}

// printAllocationPie prints a section's pie with its legend on the right:
// the transmit of the image with a new random ID, then the placeholder
// lines. The pies stay in the scrollback, so the CLI does not delete them.
// It returns false, and prints nothing, when the image cannot be encoded.
func printAllocationPie(w io.Writer, sec reportdom.AllocationSection, pie *cliPie) bool {
	id := termimg.NewID()
	img := chart.Pie(sec.PieSlices(pie.dark), pie.w, pie.h, 2)
	seq, err := termimg.Transmit(img, id, pie.cols, allocationPieRows)
	if err != nil {
		return false
	}
	legend := allocationLegend(sec, pie.dark)
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
// with their swatches, and the TOTAL, allocationLegendWidth cells wide. The
// swatch is the only text in a class color (rule 9).
func allocationLegend(sec reportdom.AllocationSection, dark bool) []string {
	money := func(m types.Money) string { return formatTotal(m, sec.Currency, true, false) }
	row := func(swatch, class, count, value, pct string) string {
		return fmt.Sprintf("%s %-19s %8s %13s %7s", swatch, class, count, value, pct)
	}
	rule := strings.Repeat("─", allocationLegendWidth)
	lines := []string{row(" ", "Asset Class", "Holdings", "Value", "% Total"), rule}
	for _, c := range sec.Shown() {
		col := chart.ClassColor(c.Class, dark)
		swatch := fmt.Sprintf("\x1b[38;2;%d;%d;%dm■\x1b[39m", col.R, col.G, col.B)
		lines = append(lines, row(swatch, c.Class.DisplayName(), fmt.Sprint(c.Count()), money(c.Value), fmt.Sprintf("%.1f%%", c.Percent)))
	}
	total := fmt.Sprintf("TOTAL (%s)", sec.Currency)
	if sec.Available {
		lines = append(lines, rule, row(" ", total, fmt.Sprint(sec.Count()), money(sec.Value), "100.0%"))
	} else {
		lines = append(lines, rule, row(" ", total, "", formatTotal(sec.Value, sec.Currency, false, false), ""))
	}
	return lines
}
