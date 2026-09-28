package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/widget"
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

// loadPriceViewData returns a command that loads the prices landing page:
// the list of latest prices per non-hidden security with any prices.
func (a *App) loadPriceViewData() tea.Cmd {
	return func() tea.Msg {
		if a.services.Security == nil || a.services.Price == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		excludeHidden := true
		securities, err := a.services.Security.List(security.Filter{ExcludeHidden: &excludeHidden})
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load securities: %w", err)}
		}

		latest, err := a.services.Price.GetLatestPrices()
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
		if a.services.Security == nil || a.services.Price == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		excludeHidden := true
		securities, err := a.services.Security.List(security.Filter{ExcludeHidden: &excludeHidden})
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load securities: %w", err)}
		}

		var prices []*price.Price
		if sec != nil {
			prices, err = a.services.Price.GetPriceHistory(sec.ID, nil, nil)
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
			d := buildAddPriceDialog(a.priceView.selectedSecurity)
			d.SetVisible(true)
			a.price = priceSurface{modalSurface: modalSurface{dlg: d}, mode: priceDialogModeAdd}
		}
		return a, nil
	case key.Matches(msg, a.keys.Enter):
		p := a.selectedPrice()
		if p != nil && a.priceView.selectedSecurity != nil {
			d := buildEditPriceDialog(a.priceView.selectedSecurity, p)
			d.SetVisible(true)
			a.price = priceSurface{
				modalSurface: modalSurface{dlg: d},
				mode:         priceDialogModeEdit,
				editID:       p.ID,
			}
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
					if a.services.Price == nil {
						return errMsg{err: fmt.Errorf("price service not available")}
					}
					if err := a.services.Price.DeletePrice(priceID); err != nil {
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

// applyPriceViewData installs a freshly loaded price view and builds the table
// its mode calls for. In list mode it also kicks off the initial debounced
// chart fetch for the row under the cursor, so the chart panel populates
// without requiring a keystroke; subsequent cursor movement reschedules.
//
// The existing historyCache is carried across the reload. Without that, the
// per-security evictions the price-CRUD handlers perform (PC-015) and the full
// clear bulk refresh performs (PC-016) would be silently undone by the fresh
// empty cache loadPriceViewData constructs.
func (a *App) applyPriceViewData(data *priceViewData) tea.Cmd {
	if a.priceView != nil && a.priceView.historyCache != nil {
		data.historyCache = a.priceView.historyCache
	}
	a.priceView = data
	switch data.mode {
	case pricesViewList:
		a.buildPriceListTable()
		if secID := a.listCursorSecurityID(); !secID.IsNil() {
			return a.schedulePriceChartFetch(secID)
		}
	case pricesViewDetail:
		a.buildPriceTable()
	}
	return nil
}

// afterPriceChange notes the change, drops the affected security's cached
// chart history, and reloads the price view in whichever mode it is in. Every
// price CRUD result ends this way; only the note differs.
func (a *App) afterPriceChange(note string) tea.Cmd {
	a.statusbar.AddNotification(note, widget.NotificationInfo)
	a.evictSelectedSecurityFromHistoryCache()
	return a.reloadPriceViewKeepingMode()
}

// applyPriceRefreshResult ends a bulk refresh. The in-progress notification
// and the re-entry guard are retired whatever the outcome, so the `u` shortcut
// becomes responsive again.
//
// A bulk refresh can silently change any subset of tickers' prices and the
// result does not enumerate which (PC-016), so every chart-history entry is
// dropped rather than evicted one by one.
func (a *App) applyPriceRefreshResult(msg priceRefreshCompleteMsg) tea.Cmd {
	a.statusbar.RemoveNotification(a.refreshNotifID)
	a.refreshNotifID = 0
	a.refreshingPrices = false
	if msg.err != nil {
		a.err = msg.err
		return nil
	}
	a.statusbar.AddNotification(summarizeRefreshResult(msg.result), widget.NotificationInfo)
	a.invalidatePriceHistoryCache()
	// Re-load any data views that may now be stale.
	var cmds []tea.Cmd
	if a.currentView == ViewSecurities {
		cmds = append(cmds, a.loadSecurityViewData())
	}
	if a.currentView == ViewPrices {
		cmds = append(cmds, a.loadPriceViewData())
	}
	return tea.Batch(cmds...)
}
