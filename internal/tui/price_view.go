package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/imexport"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// pricesViewMode is the prices view's top-level mode: a summary list of
// the latest price per security, or the price history of a single
// drilled-in security.
type pricesViewMode int

const (
	pricesViewList   pricesViewMode = iota // landing page: latest price per ticker
	pricesViewDetail                       // price history for one security
)

// priceViewData holds the loaded data for the price management view.
// Both list and detail modes share this struct; mode tells which slice
// is the source of truth.
type priceViewData struct {
	mode pricesViewMode

	// List mode: the most recent price per non-hidden security with
	// any prices, sorted by ticker. selectedSecurity is nil here.
	latestPrices []*price.LatestPrice

	// Detail mode: full history for selectedSecurity (newest first).
	selectedSecurity *security.Security
	prices           []*price.Price

	// Shared
	securities  []*security.Security
	searchQuery string
	searching   bool

	// historyCache memoizes per-security price history slices so the
	// list-mode chart panel doesn't re-query the price service when the
	// cursor lands on the same row twice. Lifecycle is tied to the
	// priceViewData instance — full reload (loadPriceViewData) creates
	// a new cache; per-security CRUD invalidations call Evict; bulk
	// refresh calls Clear (see PC-015 / PC-016).
	historyCache *historyCache

	// chartDebounceGen increments each time a new debounced chart fetch
	// is scheduled. The tick message carries the gen it was scheduled
	// under; the tick handler ignores ticks whose gen has been
	// superseded by a later schedule (rapid cursor movement).
	chartDebounceGen int

	// chartDisplayedID is the security ID of the chart currently shown
	// to the user. It's updated only when an async fetch resolves
	// successfully. While a debounced fetch for a freshly-highlighted
	// ticker is in flight, the chart panel falls back to rendering this
	// id's cached history so the user doesn't see a blank panel during
	// the debounce window.
	chartDisplayedID types.ID
}

// filteredSecurities returns securities filtered by search query (hidden excluded).
func (d *priceViewData) filteredSecurities() []*security.Security {
	var result []*security.Security
	query := strings.ToLower(strings.TrimSpace(d.searchQuery))

	for _, sec := range d.securities {
		if sec.Hidden {
			continue
		}
		if query != "" {
			tickerMatch := strings.Contains(strings.ToLower(sec.Ticker), query)
			nameMatch := strings.Contains(strings.ToLower(sec.Name), query)
			if !tickerMatch && !nameMatch {
				continue
			}
		}
		result = append(result, sec)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Ticker < result[j].Ticker
	})

	return result
}

// Message types

type priceViewDataLoadedMsg struct {
	data *priceViewData
}

type priceAddedMsg struct{}
type priceUpdatedMsg struct{}
type priceDeletedMsg struct{}

type priceImportedMsg struct {
	total    int
	imported int
	skipped  int
}

// priceChartDebounceDelay is the time a cursor must dwell on a row
// before the chart panel issues a fetch for the highlighted ticker.
// Declared as a var so tests can shorten it; user-facing default is 150 ms.
var priceChartDebounceDelay = 150 * time.Millisecond

// priceChartDebounceTickMsg is delivered when a scheduled debounce timer
// fires. The handler in app.go's Update verifies (a) gen still matches
// priceView.chartDebounceGen (i.e. no later schedule has superseded
// this one), and (b) the cursor is still on secID; if both hold, it
// dispatches the actual price-history fetch. Otherwise it drops the
// tick — that's how rapid cursor movement collapses to a single fetch.
type priceChartDebounceTickMsg struct {
	gen   int
	secID types.ID
}

// priceChartHistoryLoadedMsg carries the result of a debounced fetch.
// Its handler stores prices in priceView.historyCache and sets
// chartDisplayedID = secID so the next render shows the new ticker.
type priceChartHistoryLoadedMsg struct {
	secID  types.ID
	prices []*price.Price
}

// loadPriceViewData returns a command that loads the prices landing page:
// the list of latest prices per non-hidden security with any prices.
func (a *App) loadPriceViewData() tea.Cmd {
	return func() tea.Msg {
		if a.securitySvc == nil || a.priceSvc == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		excludeHidden := true
		securities, err := a.securitySvc.List(security.Filter{ExcludeHidden: &excludeHidden})
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load securities: %w", err)}
		}

		latest, err := a.priceSvc.GetLatestPrices()
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load latest prices: %w", err)}
		}

		data := &priceViewData{
			mode:         pricesViewList,
			securities:   securities,
			latestPrices: latest,
			historyCache: newHistoryCache(),
		}

		return priceViewDataLoadedMsg{data: data}
	}
}

// evictSelectedSecurityFromHistoryCache invalidates the chart-history
// cache entry for the security currently being mutated by a CRUD action
// (PC-015). The CRUD handlers all run in detail mode where
// selectedSecurity is set, but this guards anyway so a stray dispatch
// doesn't panic. Other entries are intentionally left alone — only the
// modified ticker needs to re-fetch.
func (a *App) evictSelectedSecurityFromHistoryCache() {
	if a.priceView == nil || a.priceView.historyCache == nil {
		return
	}
	if a.priceView.selectedSecurity == nil {
		return
	}
	a.priceView.historyCache.Evict(a.priceView.selectedSecurity.ID)
}

// reloadPriceViewKeepingMode refreshes the prices view in whichever mode
// it is currently showing. Used after a CRUD operation so the user stays
// in detail mode (instead of being kicked back to the landing list) when
// they add/edit/delete a price for a specific ticker.
func (a *App) reloadPriceViewKeepingMode() tea.Cmd {
	if a.priceView != nil && a.priceView.mode == pricesViewDetail && a.priceView.selectedSecurity != nil {
		return a.loadPriceViewDataForSecurity(a.priceView.selectedSecurity)
	}
	return a.loadPriceViewData()
}

// loadPriceViewDataForSecurity returns a command that drills into a single
// security's price history (detail mode).
func (a *App) loadPriceViewDataForSecurity(sec *security.Security) tea.Cmd {
	return func() tea.Msg {
		if a.securitySvc == nil || a.priceSvc == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		excludeHidden := true
		securities, err := a.securitySvc.List(security.Filter{ExcludeHidden: &excludeHidden})
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load securities: %w", err)}
		}

		var prices []*price.Price
		if sec != nil {
			prices, err = a.priceSvc.GetPriceHistory(sec.ID, nil, nil)
			if err != nil {
				return errMsg{err: fmt.Errorf("failed to load prices: %w", err)}
			}
		}

		data := &priceViewData{
			mode:             pricesViewDetail,
			securities:       securities,
			selectedSecurity: sec,
			prices:           prices,
			historyCache:     newHistoryCache(),
		}

		return priceViewDataLoadedMsg{data: data}
	}
}

// buildPriceListTable creates and populates the list-mode summary table
// (one row per security with its latest price).
func (a *App) buildPriceListTable() {
	if a.priceView == nil {
		return
	}

	columns := []Column{
		{Header: "Ticker", Width: 10, Align: AlignLeft},
		{Header: "Name", Width: 32, Align: AlignLeft},
		{Header: "Latest Price", Width: 15, Align: AlignRight},
		{Header: "Date", Width: 12, Align: AlignLeft},
	}

	if a.priceListTable == nil {
		a.priceListTable = NewTable(columns)
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

	columns := []Column{
		{Header: "Date", Width: 12, Align: AlignLeft},
		{Header: "Price", Width: 15, Align: AlignRight},
		{Header: "Source", Width: 12, Align: AlignLeft},
	}

	if a.priceTable == nil {
		a.priceTable = NewTable(columns)
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

// schedulePriceChartFetch returns a debounced tea.Cmd that, after
// priceChartDebounceDelay elapses, emits a priceChartDebounceTickMsg
// for secID. Each call bumps priceView.chartDebounceGen so any earlier
// in-flight tick becomes stale (the tick handler drops mismatched gen).
// Returns nil when there is no priceView to schedule against.
func (a *App) schedulePriceChartFetch(secID types.ID) tea.Cmd {
	if a.priceView == nil {
		return nil
	}
	a.priceView.chartDebounceGen++
	gen := a.priceView.chartDebounceGen
	return tea.Tick(priceChartDebounceDelay, func(_ time.Time) tea.Msg {
		return priceChartDebounceTickMsg{gen: gen, secID: secID}
	})
}

// fetchPriceChartHistory returns a tea.Cmd that synchronously calls the
// price service for secID's full history and emits a
// priceChartHistoryLoadedMsg. On error, it returns no message — the
// chart simply stays in its current state until the next cursor move.
func (a *App) fetchPriceChartHistory(secID types.ID) tea.Cmd {
	return func() tea.Msg {
		if a.priceSvc == nil {
			return nil
		}
		prices, err := a.priceSvc.GetPriceHistory(secID, nil, nil)
		if err != nil {
			return nil
		}
		return priceChartHistoryLoadedMsg{secID: secID, prices: prices}
	}
}

// renderPriceDetail renders the per-security price history (drill-in).
func (a *App) renderPriceDetail() string {
	// Full-screen view — see comment in renderPriceList above.
	contentWidth := max(a.width-4, 1)

	var sections []string

	titleText := "PRICES"
	var secInfo string
	if a.priceView.selectedSecurity != nil {
		secInfo = fmt.Sprintf("%s (%s)", a.priceView.selectedSecurity.Ticker, a.priceView.selectedSecurity.Name)
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

// handlePriceViewKeys dispatches key presses to the list- or detail-mode
// handler.
func (a *App) handlePriceViewKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.priceView == nil {
		return a, nil
	}
	if a.priceView.searching {
		return a.handlePriceSearchKey(msg)
	}
	if a.priceView.mode == pricesViewDetail {
		return a.handlePriceDetailKeys(msg)
	}
	return a.handlePriceListKeys(msg)
}

// handlePriceListKeys handles keys on the prices landing page.
func (a *App) handlePriceListKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	tbl := a.priceListTable
	cursorMoved := false
	switch {
	case key.Matches(msg, a.keys.Up):
		if tbl != nil {
			tbl.MoveUp()
			cursorMoved = true
		}
	case key.Matches(msg, a.keys.Down):
		if tbl != nil {
			tbl.MoveDown()
			cursorMoved = true
		}
	case msg.String() == "home" || msg.String() == "g":
		if tbl != nil {
			tbl.MoveToTop()
			cursorMoved = true
		}
	case msg.String() == "end" || msg.String() == "G":
		if tbl != nil {
			tbl.MoveToBottom()
			cursorMoved = true
		}
	case msg.String() == "pgup":
		if tbl != nil {
			tbl.PageUp(max(a.height-10, 1))
			cursorMoved = true
		}
	case msg.String() == "pgdown":
		if tbl != nil {
			tbl.PageDown(max(a.height-10, 1))
			cursorMoved = true
		}
	case key.Matches(msg, a.keys.Search):
		a.priceView.searching = true
		a.priceView.searchQuery = ""
	case key.Matches(msg, a.keys.Enter):
		return a, a.drillIntoSelectedListRow()
	case msg.String() == "u":
		return a, a.startPriceRefresh()
	}
	if cursorMoved {
		return a, a.schedulePriceChartFetch(a.listCursorSecurityID())
	}
	return a, nil
}

// handlePriceDetailKeys handles keys on a single security's price history.
func (a *App) handlePriceDetailKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Up):
		if a.priceTable != nil {
			a.priceTable.MoveUp()
		}
	case key.Matches(msg, a.keys.Down):
		if a.priceTable != nil {
			a.priceTable.MoveDown()
		}
	case msg.String() == "home" || msg.String() == "g":
		if a.priceTable != nil {
			a.priceTable.MoveToTop()
		}
	case msg.String() == "end" || msg.String() == "G":
		if a.priceTable != nil {
			a.priceTable.MoveToBottom()
		}
	case msg.String() == "pgup":
		if a.priceTable != nil {
			a.priceTable.PageUp(max(a.height-10, 1))
		}
	case msg.String() == "pgdown":
		if a.priceTable != nil {
			a.priceTable.PageDown(max(a.height-10, 1))
		}
	case key.Matches(msg, a.keys.Escape):
		// Flip back to list mode synchronously so the next render is the
		// landing page; loadPriceViewData refreshes the data behind it.
		a.priceView.mode = pricesViewList
		a.priceView.selectedSecurity = nil
		a.priceView.prices = nil
		return a, a.loadPriceViewData()
	case key.Matches(msg, a.keys.Search):
		a.priceView.searching = true
		a.priceView.searchQuery = ""
	case key.Matches(msg, a.keys.New):
		if a.priceView.selectedSecurity != nil {
			a.priceDialog = buildAddPriceDialog(a.priceView.selectedSecurity)
			a.priceDialogMode = priceDialogModeAdd
			a.priceDialog.SetVisible(true)
		}
		return a, nil
	case key.Matches(msg, a.keys.Enter):
		p := a.selectedPrice()
		if p != nil && a.priceView.selectedSecurity != nil {
			a.priceDialog = buildEditPriceDialog(a.priceView.selectedSecurity, p)
			a.priceDialogMode = priceDialogModeEdit
			a.priceDialogEditID = p.ID
			a.priceDialog.SetVisible(true)
		}
		return a, nil
	case key.Matches(msg, a.keys.Delete):
		p := a.selectedPrice()
		if p != nil {
			priceID := p.ID
			dateStr := p.Date.Time().Format("2006-01-02")
			a.showConfirmDialog(
				"Delete Price",
				fmt.Sprintf("Delete price for %s?", dateStr),
				func() tea.Msg {
					if a.priceSvc == nil {
						return errMsg{err: fmt.Errorf("price service not available")}
					}
					if err := a.priceSvc.DeletePrice(priceID); err != nil {
						return errMsg{err: err}
					}
					return priceDeletedMsg{}
				},
			)
		}
	case msg.String() == "i":
		a.priceImportDialog = buildImportPriceDialog()
		a.priceImportDialog.SetVisible(true)
		return a, nil
	case msg.String() == "u":
		return a, a.startPriceRefresh()
	}
	return a, nil
}

// drillIntoSelectedListRow loads detail mode for the security at the
// list-table cursor.
func (a *App) drillIntoSelectedListRow() tea.Cmd {
	if a.priceView == nil || a.priceListTable == nil {
		return nil
	}
	cursor := a.priceListTable.Cursor()
	if cursor < 0 || cursor >= len(a.priceView.latestPrices) {
		return nil
	}
	targetID := a.priceView.latestPrices[cursor].SecurityID

	// Resolve to a *security.Security from the cached list so the loader
	// can populate ticker/name without an extra round-trip.
	var sec *security.Security
	for _, s := range a.priceView.securities {
		if s.ID == targetID {
			sec = s
			break
		}
	}
	if sec == nil {
		// Fall back: synthesize from the LatestPrice row.
		sec = &security.Security{Ticker: a.priceView.latestPrices[cursor].Ticker, Name: a.priceView.latestPrices[cursor].Name}
		sec.ID = targetID
	}
	a.priceView.mode = pricesViewDetail
	a.priceView.selectedSecurity = sec
	return a.loadPriceViewDataForSecurity(sec)
}

// handlePriceSearchKey handles key presses while in search mode.
func (a *App) handlePriceSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Escape):
		a.priceView.searching = false
		a.priceView.searchQuery = ""
	case key.Matches(msg, a.keys.Enter):
		a.priceView.searching = false
		// Select the first filtered security
		filtered := a.priceView.filteredSecurities()
		if len(filtered) > 0 {
			a.priceView.selectedSecurity = filtered[0]
			a.priceView.searchQuery = ""
			return a, a.loadPriceViewDataForSecurity(filtered[0])
		}
		a.priceView.searchQuery = ""
	case msg.String() == "backspace":
		if len(a.priceView.searchQuery) > 0 {
			a.priceView.searchQuery = a.priceView.searchQuery[:len(a.priceView.searchQuery)-1]
		}
	case msg.Text != "":
		a.priceView.searchQuery += msg.Text
	}
	return a, nil
}

// Price dialog types and builders

type priceDialogMode int

const (
	priceDialogModeAdd priceDialogMode = iota
	priceDialogModeEdit
)

// buildAddPriceDialog builds the dialog for adding a new price.
func buildAddPriceDialog(sec *security.Security) *Dialog {
	d := NewDialog(fmt.Sprintf("Add Price — %s", sec.Ticker))

	today := time.Now().Format("2006-01-02")
	f := d.AddDateFieldISO("Date", today)
	f.Required = true

	f = d.AddTextField("Price", "", "e.g. 185.50", 15)
	f.Required = true

	d.SetButtons([]DialogButton{
		{Label: "Cancel"},
		{Label: "Save", Primary: true},
	})

	return d
}

// buildEditPriceDialog builds the dialog for editing an existing price.
func buildEditPriceDialog(sec *security.Security, p *price.Price) *Dialog {
	d := NewDialog(fmt.Sprintf("Edit Price — %s", sec.Ticker))

	dateStr := p.Date.Time().Format("2006-01-02")
	f := d.AddDateFieldISO("Date", dateStr)
	f.Required = true

	priceStr := fmt.Sprintf("%.2f", p.Price.Float64())
	f = d.AddTextField("Price", priceStr, "e.g. 185.50", 15)
	f.Required = true

	d.SetButtons([]DialogButton{
		{Label: "Cancel"},
		{Label: "Save", Primary: true},
	})

	return d
}

// handlePriceDialogKey handles key presses in the price add/edit dialog.
func (a *App) handlePriceDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	action := a.priceDialog.HandleKey(msg)
	switch action {
	case DialogActionCancel:
		a.priceDialog.SetVisible(false)
		a.priceDialog = nil
		return a, nil
	case DialogActionSubmit:
		return a.submitPriceDialog()
	}
	return a, nil
}

// submitPriceDialog processes the price dialog submission.
func (a *App) submitPriceDialog() (tea.Model, tea.Cmd) {
	fields := a.priceDialog.Fields()

	dateStr := strings.TrimSpace(fields[0].Value)
	priceStr := strings.TrimSpace(fields[1].Value)

	if dateStr == "" {
		a.priceDialog.SetErrorMsg("Date is required.")
		return a, nil
	}
	if priceStr == "" {
		a.priceDialog.SetErrorMsg("Price is required.")
		return a, nil
	}

	date, err := types.ParseDate(dateStr)
	if err != nil {
		a.priceDialog.SetErrorMsg("Invalid date format. Use YYYY-MM-DD.")
		return a, nil
	}

	amount, err := types.NewMoney(priceStr)
	if err != nil {
		a.priceDialog.SetErrorMsg("Invalid price value.")
		return a, nil
	}

	if amount.IsZero() || !amount.IsPositive() {
		a.priceDialog.SetErrorMsg("Price must be positive.")
		return a, nil
	}

	secID := a.priceView.selectedSecurity.ID
	mode := a.priceDialogMode
	editID := a.priceDialogEditID

	a.priceDialog.SetVisible(false)
	a.priceDialog = nil

	if mode == priceDialogModeAdd {
		return a, a.createPrice(secID, date, amount)
	}
	return a, a.updatePrice(editID, secID, date, amount)
}

// createPrice creates a new price via the service.
func (a *App) createPrice(securityID types.ID, date types.Date, amount types.Money) tea.Cmd {
	return func() tea.Msg {
		if a.priceSvc == nil {
			return errMsg{err: fmt.Errorf("price service not available")}
		}

		p := price.NewPrice(securityID, date, amount, price.SourceManual)
		if err := a.priceSvc.AddPrice(p); err != nil {
			return errMsg{err: err}
		}
		return priceAddedMsg{}
	}
}

// updatePrice updates an existing price via the service.
func (a *App) updatePrice(id, securityID types.ID, date types.Date, amount types.Money) tea.Cmd {
	return func() tea.Msg {
		if a.priceSvc == nil {
			return errMsg{err: fmt.Errorf("price service not available")}
		}

		p := price.NewPrice(securityID, date, amount, price.SourceManual)
		p.ID = id
		if err := a.priceSvc.UpdatePrice(p); err != nil {
			return errMsg{err: err}
		}
		return priceUpdatedMsg{}
	}
}

// Bulk import dialog

// buildImportPriceDialog builds the dialog for importing prices from CSV.
func buildImportPriceDialog() *Dialog {
	d := NewDialog("Import Prices")

	f := d.AddTextField("CSV File", "", "Path to CSV file", 0)
	f.Required = true

	d.AddCheckboxField("Overwrite existing", false)

	d.SetButtons([]DialogButton{
		{Label: "Cancel"},
		{Label: "Import", Primary: true},
	})

	return d
}

// handlePriceImportDialogKey handles key presses in the import dialog.
func (a *App) handlePriceImportDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	action := a.priceImportDialog.HandleKey(msg)
	switch action {
	case DialogActionCancel:
		a.priceImportDialog.SetVisible(false)
		a.priceImportDialog = nil
		return a, nil
	case DialogActionSubmit:
		return a.submitImportPriceDialog()
	}
	return a, nil
}

// submitImportPriceDialog processes the import dialog submission.
func (a *App) submitImportPriceDialog() (tea.Model, tea.Cmd) {
	fields := a.priceImportDialog.Fields()

	filePath := strings.TrimSpace(fields[0].Value)
	if filePath == "" {
		a.priceImportDialog.SetErrorMsg("CSV file path is required.")
		return a, nil
	}

	overwrite := fields[1].Checked

	a.priceImportDialog.SetVisible(false)
	a.priceImportDialog = nil

	return a, a.importPrices(filePath, overwrite)
}

// importPrices imports prices from a CSV file.
func (a *App) importPrices(filePath string, overwrite bool) tea.Cmd {
	return func() tea.Msg {
		if a.priceSvc == nil || a.securitySvc == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		f, err := os.Open(filePath)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to open file: %w", err)}
		}
		defer f.Close()

		result, err := imexport.ParsePriceCSV(f)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to parse CSV: %w", err)}
		}

		if result.HasErrors() {
			var msgs []string
			for _, e := range result.Errors {
				msgs = append(msgs, fmt.Sprintf("line %d: %s", e.Line, e.Message))
			}
			return errMsg{err: fmt.Errorf("CSV errors:\n%s", strings.Join(msgs, "\n"))}
		}

		// Resolve tickers to security IDs
		var prices []*price.Price
		for _, rec := range result.Records {
			sec, lookupErr := a.securitySvc.GetByTicker(rec.Ticker, "USD")
			if lookupErr != nil {
				return errMsg{err: fmt.Errorf("unknown ticker %q (line %d): %w", rec.Ticker, rec.SourceLine, lookupErr)}
			}
			prices = append(prices, price.NewPrice(sec.ID, rec.Date, rec.Price, price.SourceImport))
		}

		importResult, err := a.priceSvc.BulkImport(prices, overwrite)
		if err != nil {
			return errMsg{err: fmt.Errorf("import failed: %w", err)}
		}

		return priceImportedMsg{
			total:    importResult.Total,
			imported: importResult.Imported,
			skipped:  importResult.Skipped,
		}
	}
}
