package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/ntcharts/v2/linechart/timeserieslinechart"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
)

// chartPanelMinContentWidth is the minimum content-area width (after
// sidebar) at which the prices chart panel is shown alongside the list.
// Below this, the prices view stays exactly as it was before the chart
// feature.
const chartPanelMinContentWidth = 120

// shouldShowChartPanel reports whether the chart panel should render
// for a given prices content-area width. The threshold is a hard lower
// bound: at exactly chartPanelMinContentWidth the panel appears, below
// it does not.
func shouldShowChartPanel(contentWidth int) bool {
	return contentWidth >= chartPanelMinContentWidth
}

// noPriceHistoryMessage returns the muted placeholder shown in the
// chart panel when the highlighted security has zero prices on file.
func noPriceHistoryMessage() string {
	return "No price history"
}

// singlePriceMessage returns the muted placeholder shown in the chart
// panel when the highlighted security has exactly one price on file —
// a chart needs at least two points.
func singlePriceMessage(p *price.Price) string {
	if p == nil {
		return "Only one price on file — chart needs ≥ 2 points"
	}
	return fmt.Sprintf(
		"Only one price on file — chart needs ≥ 2 points\n$%.2f on %s",
		p.Price.Float64(),
		p.Date.Time().Format("2006-01-02"),
	)
}

// priceHistoryToSeries converts a slice of prices into parallel time
// and value slices suitable for ntcharts' timeserieslinechart, sorted
// chronologically (oldest first). The price service returns history
// newest-first, so this function reverses by sorting on time.
func priceHistoryToSeries(prices []*price.Price) ([]time.Time, []float64) {
	if len(prices) == 0 {
		return nil, nil
	}

	sorted := make([]*price.Price, len(prices))
	copy(sorted, prices)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Date.Time().Before(sorted[j].Date.Time())
	})

	times := make([]time.Time, len(sorted))
	values := make([]float64, len(sorted))
	for i, p := range sorted {
		times[i] = p.Date.Time()
		values[i] = p.Price.Float64()
	}
	return times, values
}

// chartPanelMinWidth is the minimum overall width (including borders)
// at which buildChartPanel produces a usable rendering.
const chartPanelMinWidth = 12

// chartPanelMinHeight is the minimum overall height (including
// top/bottom borders + at least one inner line) for buildChartPanel.
const chartPanelMinHeight = 3

// buildChartPanel renders the prices chart panel: a bordered box with
// the security's ticker/name embedded in the top border and either a
// line chart, an edge-case placeholder, or nothing inside. Returns ""
// when the security is nil or the requested area is too small to draw a
// meaningful box.
//
// The returned string is exactly width columns wide and height rows
// tall (suitable for lipgloss.JoinHorizontal alongside a same-height
// table).
func buildChartPanel(width, height int, sec *security.Security, prices []*price.Price) string {
	if sec == nil || width < chartPanelMinWidth || height < chartPanelMinHeight {
		return ""
	}

	innerW := width - 2
	innerH := height - 2
	if innerW < 1 || innerH < 1 {
		return ""
	}

	title := sec.Ticker
	if sec.Name != "" {
		title = fmt.Sprintf("%s — %s", sec.Ticker, sec.Name)
	}

	var inner string
	switch len(prices) {
	case 0:
		inner = noPriceHistoryMessage()
	case 1:
		inner = singlePriceMessage(prices[0])
	default:
		inner = renderTimeSeriesChart(innerW, innerH, prices)
	}

	return composeChartBox(title, inner, innerW, innerH)
}

// renderTimeSeriesChart builds an ntcharts timeserieslinechart over the
// supplied prices and returns its rendered View. innerW/innerH are the
// drawable area, excluding the surrounding border.
func renderTimeSeriesChart(innerW, innerH int, prices []*price.Price) string {
	times, values := priceHistoryToSeries(prices)
	if len(times) < 2 {
		return ""
	}
	minY, maxY := clampYRange(values)

	chart := timeserieslinechart.New(innerW, innerH)
	chart.SetTimeRange(times[0], times[len(times)-1])
	chart.SetViewTimeRange(times[0], times[len(times)-1])
	chart.SetYRange(minY, maxY)
	chart.SetViewYRange(minY, maxY)
	for i, t := range times {
		chart.Push(timeserieslinechart.TimePoint{Time: t, Value: values[i]})
	}
	chart.Draw()
	return chart.View()
}

// composeChartBox wraps inner content in a normal-bordered box whose
// top edge has the title embedded between two dashes
// (e.g. `┌─ AAPL — Apple Inc. ─...─┐`). The returned string is sized to
// (innerW + 2) × (innerH + 2) — i.e. inner content plus a single-cell
// border on all four sides.
func composeChartBox(title, inner string, innerW, innerH int) string {
	const (
		topLeft     = "┌"
		topRight    = "┐"
		bottomLeft  = "└"
		bottomRight = "┘"
		hBar        = "─"
		vBar        = "│"
	)

	// Build the title segment: `─ <title> `. Truncate on display width
	// if it would consume the entire top edge — leave at least one
	// trailing dash before the corner.
	segment := fmt.Sprintf("%s %s ", hBar, title)
	maxSegW := max(innerW-1, 0)
	if lipgloss.Width(segment) > maxSegW {
		segment = truncateToDisplayWidth(segment, maxSegW)
	}
	fillW := max(innerW-lipgloss.Width(segment), 0)
	top := topLeft + segment + strings.Repeat(hBar, fillW) + topRight

	bottom := bottomLeft + strings.Repeat(hBar, innerW) + bottomRight

	innerLines := strings.Split(inner, "\n")
	rows := make([]string, innerH)
	for i := range innerH {
		var raw string
		if i < len(innerLines) {
			raw = innerLines[i]
		}
		rows[i] = vBar + padOrTruncateToDisplayWidth(raw, innerW) + vBar
	}

	return top + "\n" + strings.Join(rows, "\n") + "\n" + bottom
}

// truncateToDisplayWidth truncates s rune-by-rune until its display
// width is <= maxW. ANSI escape sequences are preserved when possible
// because lipgloss.Width is escape-aware.
func truncateToDisplayWidth(s string, maxW int) string {
	if lipgloss.Width(s) <= maxW {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		candidate := b.String() + string(r)
		if lipgloss.Width(candidate) > maxW {
			return b.String()
		}
		b.WriteRune(r)
	}
	return b.String()
}

// padOrTruncateToDisplayWidth pads s with spaces or truncates it (by
// display width) so the returned string has display width exactly w.
func padOrTruncateToDisplayWidth(s string, w int) string {
	cur := lipgloss.Width(s)
	if cur == w {
		return s
	}
	if cur < w {
		return s + strings.Repeat(" ", w-cur)
	}
	t := truncateToDisplayWidth(s, w)
	if pad := w - lipgloss.Width(t); pad > 0 {
		t += strings.Repeat(" ", pad)
	}
	return t
}

// clampYRange returns a (min, max) pair suitable for the chart's Y
// axis. Distinct values pass through. Identical values get padded by
// ±0.5% so the chart renders a horizontal line instead of dividing by
// zero. All-zero (or empty) input returns a fixed ±0.5 fallback.
func clampYRange(values []float64) (float64, float64) {
	if len(values) == 0 {
		return -0.5, 0.5
	}

	min, max := values[0], values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	if min != max {
		return min, max
	}

	// All equal: pad by 0.5% of the value, falling back to ±0.5 when
	// the value itself is zero (so we never produce a zero spread).
	v := min
	if v == 0 {
		return -0.5, 0.5
	}
	pad := 0.005 * v
	if pad < 0 {
		pad = -pad
	}
	return v - pad, v + pad
}
