package tui

import (
	"fmt"
	"sort"
	"time"

	"github.com/haskovec/tmoney/internal/price"
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
