// Prices view: the list and history tables and their rendering.

package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/pricechart"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// buildListTable creates and populates the list-mode summary table (one row
// per security with its latest price).
func (s *priceViewState) buildListTable() {
	if s.data == nil {
		return
	}

	columns := []widget.Column{
		{Header: "Ticker", Width: 10, Align: widget.AlignLeft},
		{Header: "Name", Width: 32, Align: widget.AlignLeft},
		{Header: "Latest Price", Width: 15, Align: widget.AlignRight},
		{Header: "Date", Width: 12, Align: widget.AlignLeft},
	}

	if s.listTable == nil {
		s.listTable = widget.NewTable(columns)
	} else {
		s.listTable.SetColumns(columns)
	}

	rows := make([][]string, len(s.data.latestPrices))
	for i, lp := range s.data.latestPrices {
		rows[i] = []string{
			lp.Ticker,
			lp.Name,
			fmt.Sprintf("$%.2f", lp.Price.Float64()),
			lp.Date.Time().Format("2006-01-02"),
		}
	}
	s.listTable.SetRows(rows)
	s.listTable.SetFocused(true)
}

// buildTable creates and populates the detail-mode history table.
func (s *priceViewState) buildTable() {
	if s.data == nil {
		return
	}

	columns := []widget.Column{
		{Header: "Date", Width: 12, Align: widget.AlignLeft},
		{Header: "Price", Width: 15, Align: widget.AlignRight},
		{Header: "Source", Width: 12, Align: widget.AlignLeft},
	}

	if s.table == nil {
		s.table = widget.NewTable(columns)
	} else {
		s.table.SetColumns(columns)
	}

	rows := make([][]string, len(s.data.prices))
	for i, p := range s.data.prices {
		rows[i] = formatPriceRow(p)
	}
	s.table.SetRows(rows)
	s.table.SetFocused(true)
}

// formatPriceRow formats a price into a table row.
func formatPriceRow(p *price.Price) []string {
	return []string{
		p.Date.Time().Format("2006-01-02"),
		fmt.Sprintf("$%.2f", p.Price.Float64()),
		p.Source.DisplayName(),
	}
}

// selectedPrice returns the currently selected price based on table cursor.
func (s *priceViewState) selectedPrice() *price.Price {
	if s.data == nil || s.table == nil {
		return nil
	}

	cursor := s.table.Cursor()
	if cursor < 0 || cursor >= len(s.data.prices) {
		return nil
	}
	return s.data.prices[cursor]
}

// render renders the prices view in either list or detail mode. width and
// height are the screen size.
func (s *priceViewState) render(styles widget.Styles, width, height int) string {
	if s.data == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading prices...")
	}

	if s.data.mode == pricesViewDetail {
		return s.renderDetail(styles, width, height)
	}
	return s.renderList(styles, width, height)
}

// priceListNaturalTableWidth is the width the prices list table is
// rendered at when the chart panel is shown beside it. It is the sum of
// the four column widths (10 + 32 + 15 + 12 = 69) plus the three
// inter-column separators (3) plus a small visual gutter (3) so the
// chart border doesn't sit directly against the last column. Below
// pricechart.MinContentWidth the table reverts to filling the full
// content area as before.
const priceListNaturalTableWidth = 75

// renderList renders the landing-page summary table, optionally composing the
// price-history chart panel beside it on wide terminals.
func (s *priceViewState) renderList(styles widget.Styles, width, height int) string {
	// Prices is a full-screen view (see renderView in app.go) — no
	// sidebar is rendered, so use the full terminal width minus the
	// Padding(1, 2) wrapper applied below (2 cols left + 2 cols right).
	// ContentWidth() would over-subtract a sidebar that isn't there,
	// leaving ~30 cols of wasted space on the right at large layouts.
	contentWidth := max(width-4, 1)

	var sections []string

	titleText := "PRICES"
	hint := "Enter: view history  ·  u: update prices  ·  /: search"
	if s.data.searchQuery != "" {
		hint += "  Search: " + s.data.searchQuery
	}
	padding := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(hint)-4, 1)
	headerRow := styles.Title.Render(titleText) + strings.Repeat(" ", padding) + styles.Muted.Render(hint)
	sections = append(sections, headerRow)

	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, styles.Muted.Render(strings.Repeat("─", sepWidth)))

	if len(s.data.latestPrices) == 0 {
		sections = append(sections, "")
		sections = append(sections, styles.Muted.Render("  No prices on file. Press 'p' on a security to start."))
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render(strings.Join(sections, "\n"))
	}

	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 2 // title + separator
	footerHeight := 1
	paddingHeight := 2
	tableHeight := max(height-headerHeight-statusBarHeight-titleHeight-footerHeight-paddingHeight, 1)

	if s.listTable != nil {
		body := s.composeListBody(styles, contentWidth, tableHeight)
		sections = append(sections, body)
		if info := s.listTable.ScrollInfo(tableHeight - 2); info != "" {
			sections = append(sections, styles.Muted.Render("  "+info))
		}
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// composeListBody renders the list table on its own at narrow content widths,
// or joined horizontally with the chart panel for the highlighted ticker on
// wide terminals.
func (s *priceViewState) composeListBody(styles widget.Styles, contentWidth, height int) string {
	if !pricechart.ShouldShow(contentWidth) {
		tableWidth := max(contentWidth-4, 1)
		return s.listTable.Render(styles, tableWidth, height)
	}

	tableWidth := priceListNaturalTableWidth
	if tableWidth >= contentWidth {
		tableWidth = max(contentWidth-4, 1)
		return s.listTable.Render(styles, tableWidth, height)
	}
	chartWidth := contentWidth - tableWidth
	chartHeight := height

	chartPanel := s.buildListChartPanel(chartWidth, chartHeight)
	tableStr := s.listTable.Render(styles, tableWidth, height)
	if chartPanel == "" {
		return tableStr
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tableStr, chartPanel)
}

// buildListChartPanel renders the chart panel for the highlighted list row. The render path NEVER calls the price service — it only
// reads from prices.data.historyCache, which is populated asynchronously
// by the debounce/fetch flow (see scheduleChartFetch and the
// priceChartDebounceTickMsg / priceChartHistoryLoadedMsg handlers).
//
// Resolution order for "what ticker to show":
//  1. Highlighted row's security, if its history is cached.
//  2. prices.data.chartDisplayedID (the most recently fetched ticker), if
//     its history is still cached. This keeps the panel populated during
//     the 150 ms debounce window after the user moves to a not-yet-fetched
//     ticker — no `Loading…` placeholder needed.
//  3. "" — the panel is omitted entirely until a fetch resolves.
//
// Returns "" if the cursor is out of range or the chart area is too
// small.
func (s *priceViewState) buildListChartPanel(width, height int) string {
	if s.data == nil || s.listTable == nil {
		return ""
	}
	cursor := s.listTable.Cursor()
	if cursor < 0 || cursor >= len(s.data.latestPrices) {
		return ""
	}
	if s.data.historyCache == nil {
		return ""
	}

	highlightedID := s.data.latestPrices[cursor].SecurityID

	var (
		displayID types.ID
		prices    []*price.Price
		ok        bool
	)
	if prices, ok = s.data.historyCache.Lookup(highlightedID); ok {
		displayID = highlightedID
	} else if !s.data.chartDisplayedID.IsNil() {
		if prices, ok = s.data.historyCache.Lookup(s.data.chartDisplayedID); ok {
			displayID = s.data.chartDisplayedID
		}
	}
	if !ok {
		return ""
	}

	sec := s.resolveListSecurity(displayID)
	if sec == nil {
		return ""
	}
	return pricechart.Panel(width, height, sec, prices)
}

// resolveListSecurity locates the *security.Security for id from
// prices.data.securities, falling back to a synthesized stub built from
// the matching latestPrices row so the chart-panel title can still
// render when the security cache and latestPrices are momentarily out
// of sync.
func (s *priceViewState) resolveListSecurity(id types.ID) *security.Security {
	for _, sec := range s.data.securities {
		if sec.ID == id {
			return sec
		}
	}
	for _, lp := range s.data.latestPrices {
		if lp.SecurityID == id {
			sec := &security.Security{Ticker: lp.Ticker, Name: lp.Name}
			sec.ID = id
			return sec
		}
	}
	return nil
}

// listCursorSecurityID returns the SecurityID of the row currently under
// the price-list table cursor, or types.NilID if no price data, no table,
// or the cursor is out of range.
func (s *priceViewState) listCursorSecurityID() types.ID {
	if s.data == nil || s.listTable == nil {
		return types.NilID
	}
	cursor := s.listTable.Cursor()
	if cursor < 0 || cursor >= len(s.data.latestPrices) {
		return types.NilID
	}
	return s.data.latestPrices[cursor].SecurityID
}

// renderDetail renders the per-security price history (drill-in).
func (s *priceViewState) renderDetail(styles widget.Styles, width, height int) string {
	// Full-screen view — see comment in renderList above.
	contentWidth := max(width-4, 1)

	var sections []string

	titleText := "PRICES"
	var secInfo string
	if sec := s.data.selectedSecurity; sec != nil {
		if sec.Ticker != "" {
			secInfo = fmt.Sprintf("%s (%s)", sec.Ticker, sec.Name)
		} else {
			secInfo = sec.Name
		}
	}
	if s.data.searchQuery != "" {
		secInfo += "  Search: " + s.data.searchQuery
	}
	padding := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(secInfo)-4, 1)
	headerRow := styles.Title.Render(titleText) + strings.Repeat(" ", padding) + styles.Muted.Render(secInfo)
	sections = append(sections, headerRow)

	sections = append(sections, styles.Muted.Render("  Esc: back to list"))

	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, styles.Muted.Render(strings.Repeat("─", sepWidth)))

	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 3 // title + back hint + separator
	footerHeight := 1
	paddingHeight := 2
	tableHeight := max(height-headerHeight-statusBarHeight-titleHeight-footerHeight-paddingHeight, 1)

	if s.table != nil && len(s.data.prices) > 0 {
		tableWidth := max(contentWidth-4, 1)
		sections = append(sections, s.table.Render(styles, tableWidth, tableHeight))
		if info := s.table.ScrollInfo(tableHeight - 2); info != "" {
			sections = append(sections, styles.Muted.Render("  "+info))
		}
	} else {
		sections = append(sections, "")
		sections = append(sections, styles.Muted.Render("  No prices found"))
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}
