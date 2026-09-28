// Prices view: the list and history tables and their rendering.

package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// buildPriceListTable creates and populates the list-mode summary table
// (one row per security with its latest price).
func (a *App) buildPriceListTable() {
	if a.priceView == nil {
		return
	}

	columns := []widget.Column{
		{Header: "Ticker", Width: 10, Align: widget.AlignLeft},
		{Header: "Name", Width: 32, Align: widget.AlignLeft},
		{Header: "Latest Price", Width: 15, Align: widget.AlignRight},
		{Header: "Date", Width: 12, Align: widget.AlignLeft},
	}

	if a.priceListTable == nil {
		a.priceListTable = widget.NewTable(columns)
	} else {
		a.priceListTable.SetColumns(columns)
	}

	rows := make([][]string, len(a.priceView.latestPrices))
	for i, lp := range a.priceView.latestPrices {
		rows[i] = []string{
			lp.Ticker,
			lp.Name,
			fmt.Sprintf("$%.2f", lp.Price.Float64()),
			lp.Date.Time().Format("2006-01-02"),
		}
	}
	a.priceListTable.SetRows(rows)
	a.priceListTable.SetFocused(true)
}

// buildPriceTable creates and populates the table for the price view.
func (a *App) buildPriceTable() {
	if a.priceView == nil {
		return
	}

	columns := []widget.Column{
		{Header: "Date", Width: 12, Align: widget.AlignLeft},
		{Header: "Price", Width: 15, Align: widget.AlignRight},
		{Header: "Source", Width: 12, Align: widget.AlignLeft},
	}

	if a.priceTable == nil {
		a.priceTable = widget.NewTable(columns)
	} else {
		a.priceTable.SetColumns(columns)
	}

	rows := make([][]string, len(a.priceView.prices))
	for i, p := range a.priceView.prices {
		rows[i] = a.formatPriceRow(p)
	}
	a.priceTable.SetRows(rows)
	a.priceTable.SetFocused(true)
}

// formatPriceRow formats a price into a table row.
func (a *App) formatPriceRow(p *price.Price) []string {
	return []string{
		p.Date.Time().Format("2006-01-02"),
		fmt.Sprintf("$%.2f", p.Price.Float64()),
		p.Source.DisplayName(),
	}
}

// selectedPrice returns the currently selected price based on table cursor.
func (a *App) selectedPrice() *price.Price {
	if a.priceView == nil || a.priceTable == nil {
		return nil
	}

	cursor := a.priceTable.Cursor()
	if cursor < 0 || cursor >= len(a.priceView.prices) {
		return nil
	}
	return a.priceView.prices[cursor]
}

// renderPriceView renders the prices view in either list or detail mode.
func (a *App) renderPriceView() string {
	if a.priceView == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading prices...")
	}

	if a.priceView.mode == pricesViewDetail {
		return a.renderPriceDetail()
	}
	return a.renderPriceList()
}

// priceListNaturalTableWidth is the width the prices list table is
// rendered at when the chart panel is shown beside it. It is the sum of
// the four column widths (10 + 32 + 15 + 12 = 69) plus the three
// inter-column separators (3) plus a small visual gutter (3) so the
// chart border doesn't sit directly against the last column. Below
// chartPanelMinContentWidth the table reverts to filling the full
// content area as before.
const priceListNaturalTableWidth = 75

// renderPriceList renders the landing-page summary table, optionally
// composing the price-history chart panel beside it on wide terminals.
func (a *App) renderPriceList() string {
	// Prices is a full-screen view (see renderView in app.go) — no
	// sidebar is rendered, so use the full terminal width minus the
	// Padding(1, 2) wrapper applied below (2 cols left + 2 cols right).
	// ContentWidth() would over-subtract a sidebar that isn't there,
	// leaving ~30 cols of wasted space on the right at large layouts.
	contentWidth := max(a.width-4, 1)

	var sections []string

	titleText := "PRICES"
	hint := "Enter: view history  ·  u: update prices  ·  /: search"
	if a.priceView.searchQuery != "" {
		hint += "  Search: " + a.priceView.searchQuery
	}
	padding := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(hint)-4, 1)
	headerRow := a.styles.Title.Render(titleText) + strings.Repeat(" ", padding) + a.styles.Muted.Render(hint)
	sections = append(sections, headerRow)

	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	if len(a.priceView.latestPrices) == 0 {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No prices on file. Press 'p' on a security to start."))
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render(strings.Join(sections, "\n"))
	}

	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 2 // title + separator
	footerHeight := 1
	paddingHeight := 2
	tableHeight := max(a.height-headerHeight-statusBarHeight-titleHeight-footerHeight-paddingHeight, 1)

	if a.priceListTable != nil {
		body := a.composePriceListBody(contentWidth, tableHeight)
		sections = append(sections, body)
		if info := a.priceListTable.ScrollInfo(tableHeight - 2); info != "" {
			sections = append(sections, a.styles.Muted.Render("  "+info))
		}
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// composePriceListBody renders the list table on its own at narrow
// content widths, or joined horizontally with the chart panel for the
// highlighted ticker on wide terminals.
func (a *App) composePriceListBody(contentWidth, height int) string {
	if !shouldShowChartPanel(contentWidth) {
		tableWidth := max(contentWidth-4, 1)
		return a.priceListTable.Render(a.styles, tableWidth, height)
	}

	tableWidth := priceListNaturalTableWidth
	if tableWidth >= contentWidth {
		tableWidth = max(contentWidth-4, 1)
		return a.priceListTable.Render(a.styles, tableWidth, height)
	}
	chartWidth := contentWidth - tableWidth
	chartHeight := height

	chartPanel := a.buildPriceListChartPanel(chartWidth, chartHeight)
	tableStr := a.priceListTable.Render(a.styles, tableWidth, height)
	if chartPanel == "" {
		return tableStr
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tableStr, chartPanel)
}

// buildPriceListChartPanel renders the chart panel for the highlighted
// list row. The render path NEVER calls the price service — it only
// reads from priceView.historyCache, which is populated asynchronously
// by the debounce/fetch flow (see schedulePriceChartFetch and the
// priceChartDebounceTickMsg / priceChartHistoryLoadedMsg handlers).
//
// Resolution order for "what ticker to show":
//  1. Highlighted row's security, if its history is cached.
//  2. priceView.chartDisplayedID (the most recently fetched ticker), if
//     its history is still cached. This keeps the panel populated during
//     the 150 ms debounce window after the user moves to a not-yet-fetched
//     ticker — no `Loading…` placeholder needed.
//  3. "" — the panel is omitted entirely until a fetch resolves.
//
// Returns "" if the cursor is out of range or the chart area is too
// small.
func (a *App) buildPriceListChartPanel(width, height int) string {
	if a.priceView == nil || a.priceListTable == nil {
		return ""
	}
	cursor := a.priceListTable.Cursor()
	if cursor < 0 || cursor >= len(a.priceView.latestPrices) {
		return ""
	}
	if a.priceView.historyCache == nil {
		return ""
	}

	highlightedID := a.priceView.latestPrices[cursor].SecurityID

	var (
		displayID types.ID
		prices    []*price.Price
		ok        bool
	)
	if prices, ok = a.priceView.historyCache.Lookup(highlightedID); ok {
		displayID = highlightedID
	} else if !a.priceView.chartDisplayedID.IsNil() {
		if prices, ok = a.priceView.historyCache.Lookup(a.priceView.chartDisplayedID); ok {
			displayID = a.priceView.chartDisplayedID
		}
	}
	if !ok {
		return ""
	}

	sec := a.resolveListPriceSecurity(displayID)
	if sec == nil {
		return ""
	}
	return buildChartPanel(width, height, sec, prices)
}

// resolveListPriceSecurity locates the *security.Security for id from
// priceView.securities, falling back to a synthesized stub built from
// the matching latestPrices row so the chart-panel title can still
// render when the security cache and latestPrices are momentarily out
// of sync.
func (a *App) resolveListPriceSecurity(id types.ID) *security.Security {
	for _, s := range a.priceView.securities {
		if s.ID == id {
			return s
		}
	}
	for _, lp := range a.priceView.latestPrices {
		if lp.SecurityID == id {
			sec := &security.Security{Ticker: lp.Ticker, Name: lp.Name}
			sec.ID = id
			return sec
		}
	}
	return nil
}

// listCursorSecurityID returns the SecurityID of the row currently under
// the price-list table cursor, or types.NilID if no priceView, no table,
// or the cursor is out of range.
func (a *App) listCursorSecurityID() types.ID {
	if a.priceView == nil || a.priceListTable == nil {
		return types.NilID
	}
	cursor := a.priceListTable.Cursor()
	if cursor < 0 || cursor >= len(a.priceView.latestPrices) {
		return types.NilID
	}
	return a.priceView.latestPrices[cursor].SecurityID
}

// renderPriceDetail renders the per-security price history (drill-in).
func (a *App) renderPriceDetail() string {
	// Full-screen view — see comment in renderPriceList above.
	contentWidth := max(a.width-4, 1)

	var sections []string

	titleText := "PRICES"
	var secInfo string
	if sec := a.priceView.selectedSecurity; sec != nil {
		if sec.Ticker != "" {
			secInfo = fmt.Sprintf("%s (%s)", sec.Ticker, sec.Name)
		} else {
			secInfo = sec.Name
		}
	}
	if a.priceView.searchQuery != "" {
		secInfo += "  Search: " + a.priceView.searchQuery
	}
	padding := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(secInfo)-4, 1)
	headerRow := a.styles.Title.Render(titleText) + strings.Repeat(" ", padding) + a.styles.Muted.Render(secInfo)
	sections = append(sections, headerRow)

	sections = append(sections, a.styles.Muted.Render("  Esc: back to list"))

	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 3 // title + back hint + separator
	footerHeight := 1
	paddingHeight := 2
	tableHeight := max(a.height-headerHeight-statusBarHeight-titleHeight-footerHeight-paddingHeight, 1)

	if a.priceTable != nil && len(a.priceView.prices) > 0 {
		tableWidth := max(contentWidth-4, 1)
		sections = append(sections, a.priceTable.Render(a.styles, tableWidth, tableHeight))
		if info := a.priceTable.ScrollInfo(tableHeight - 2); info != "" {
			sections = append(sections, a.styles.Muted.Render("  "+info))
		}
	} else {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No prices found"))
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}
