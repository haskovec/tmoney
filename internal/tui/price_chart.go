package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/ntcharts/v2/linechart"
	"github.com/NimbleMarkets/ntcharts/v2/linechart/timeserieslinechart"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
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
	minY, maxY, step := clampYRange(values)

	chart := timeserieslinechart.New(
		innerW, innerH,
		timeserieslinechart.WithYLabelFormatter(niceYLabelFormatter(step)),
	)
	chart.SetTimeRange(times[0], times[len(times)-1])
	chart.SetViewTimeRange(times[0], times[len(times)-1])
	chart.SetYRange(minY, maxY)
	// SetViewYRange reruns UpdateGraphSizes against the now-correct Y
	// range and our wider formatter, reserving enough left-margin
	// columns so labels like "82.0" don't get clipped.
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

// priceHistoryLoader fetches a security's full price history. The
// historyCache calls this on a miss; errors propagate to the caller and
// are deliberately not cached so the next Get retries the load.
type priceHistoryLoader func() ([]*price.Price, error)

// historyCache memoizes price-history slices per security so repeated
// chart renders for the same ticker don't re-query the price service.
// All operations are safe for concurrent use; entries are evicted only
// via Evict (per-security) or Clear (all).
type historyCache struct {
	mu      sync.Mutex
	entries map[types.ID][]*price.Price
}

// newHistoryCache returns an empty, ready-to-use cache.
func newHistoryCache() *historyCache {
	return &historyCache{entries: make(map[types.ID][]*price.Price)}
}

// Get returns the cached history for id, calling loader exactly once on
// a miss to populate it. Loader errors are returned without caching, so
// the next Get on the same id retries.
func (c *historyCache) Get(id types.ID, loader priceHistoryLoader) ([]*price.Price, error) {
	c.mu.Lock()
	if entry, ok := c.entries[id]; ok {
		c.mu.Unlock()
		return entry, nil
	}
	c.mu.Unlock()

	prices, err := loader()
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.entries[id] = prices
	c.mu.Unlock()
	return prices, nil
}

// Lookup returns the cached history for id without invoking any loader.
// ok is false on a miss. The chart-render path uses Lookup so rendering
// never blocks on the price service; the async debounce path is what
// populates the cache via Put.
func (c *historyCache) Lookup(id types.ID) ([]*price.Price, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.entries[id]
	return p, ok
}

// Put stores prices for id, overwriting any existing entry. Called by
// the debounced fetch path once the price service responds.
func (c *historyCache) Put(id types.ID, prices []*price.Price) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[id] = prices
}

// Evict removes the cached entry for id, if any. Unknown ids are a
// no-op.
func (c *historyCache) Evict(id types.ID) {
	c.mu.Lock()
	delete(c.entries, id)
	c.mu.Unlock()
}

// Clear removes every cached entry.
func (c *historyCache) Clear() {
	c.mu.Lock()
	c.entries = make(map[types.ID][]*price.Price)
	c.mu.Unlock()
}

// clampYRange returns (min, max, step) for the chart's Y axis. For
// distinct values, min and max are snapped outward to multiples of a
// "nice" step (1/2/5 × power of 10) so labels drawn at step boundaries
// fall at evenly-spaced rows — without this, ntcharts' default %.0f
// formatter places labels at whichever rows the integer-rounding
// boundary first appears, which can clump them unevenly when the raw
// data range straddles a half-integer (e.g. 81.49 → 83.40 puts "81" and
// "82" on adjacent rows). Identical values get padded by ±0.5% with
// step 1; all-zero or empty input returns a fixed ±0.5 fallback.
func clampYRange(values []float64) (float64, float64, float64) {
	if len(values) == 0 {
		return -0.5, 0.5, 1
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

	if min == max {
		v := min
		if v == 0 {
			return -0.5, 0.5, 1
		}
		pad := 0.005 * v
		if pad < 0 {
			pad = -pad
		}
		return v - pad, v + pad, 1
	}

	step := niceStep(max-min, 4)
	snappedMin := math.Floor(min/step) * step
	snappedMax := math.Ceil(max/step) * step
	if snappedMin == snappedMax {
		snappedMax = snappedMin + step
	}
	return snappedMin, snappedMax, step
}

// niceStep returns a "nice" axis step (1, 2, or 5 × power of 10) that
// produces roughly targetTicks intervals across rangeSize. Used to snap
// chart Y ranges so that label boundaries fall at evenly-spaced rows.
func niceStep(rangeSize float64, targetTicks int) float64 {
	if rangeSize <= 0 || targetTicks <= 0 {
		return 1
	}
	rough := rangeSize / float64(targetTicks)
	mag := math.Pow(10, math.Floor(math.Log10(rough)))
	switch norm := rough / mag; {
	case norm < 1.5:
		return 1 * mag
	case norm < 3:
		return 2 * mag
	case norm < 7:
		return 5 * mag
	default:
		return 10 * mag
	}
}

// niceYLabelFormatter returns a LabelFormatter that snaps each row's
// value down to the nearest multiple of step and formats with enough
// decimal precision to express step exactly. ntcharts dedupes
// consecutive identical labels, so this produces evenly-spaced labels
// at clean step boundaries (e.g. 81.50, 82.00, 82.50) instead of the
// uneven default placement.
func niceYLabelFormatter(step float64) linechart.LabelFormatter {
	decimals := 0
	for s := step; s > 0 && s < 1 && decimals < 6; s *= 10 {
		decimals++
	}
	format := fmt.Sprintf("%%.%df", decimals)
	return func(_ int, v float64) string {
		return fmt.Sprintf(format, math.Floor(v/step)*step)
	}
}
