package tui

import (
	"fmt"
	"sort"
	"strings"

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

// priceViewState is everything the Prices view owns. Its zero value is the
// view before its first load.
type priceViewState struct {
	data      *priceViewData
	table     *widget.Table        // detail mode: history for one security
	listTable *widget.Table        // list mode: latest price per ticker
	clicks    *widget.ClickTracker // list-mode double-click; lazy-initialized on first click
}

// priceDeps is what the Prices view needs from outside itself. Every dep is a
// func, because switchDatabase replaces App's services and closes the previous
// *db.DB; and deps are passed to each call, never stored in the view state.
// Both rules are pinned by the guards that run over viewControllers. The name
// is priceDeps, not pricesDeps, because the table guard finds the view state
// from it (priceViewState).
type priceDeps struct {
	securities func() *security.Service
	prices     func() *price.Service
}

// priceDeps binds the Prices view to the services App owns. Every accessor may
// return nil, because an App built by a test has no services, so each caller
// keeps its own nil guard.
func (a *App) priceDeps() priceDeps {
	return priceDeps{
		securities: func() *security.Service { return a.services.Security },
		prices:     func() *price.Service { return a.services.Price },
	}
}

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
	// priceViewData instance — full reload (load) creates
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

// load returns a command that loads the prices landing page: the list of
// latest prices per non-hidden security with any prices. The services are read
// through the deps when the command runs.
func (s *priceViewState) load(d priceDeps) tea.Cmd {
	return func() tea.Msg {
		secSvc, priceSvc := d.securities(), d.prices()
		if secSvc == nil || priceSvc == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		excludeHidden := true
		securities, err := secSvc.List(security.Filter{ExcludeHidden: &excludeHidden})
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load securities: %w", err)}
		}

		latest, err := priceSvc.GetLatestPrices()
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
func (s *priceViewState) evictSelectedSecurityFromHistoryCache() {
	if s.data == nil || s.data.historyCache == nil {
		return
	}
	if s.data.selectedSecurity == nil {
		return
	}
	s.data.historyCache.Evict(s.data.selectedSecurity.ID)
}

// reloadKeepingMode refreshes the prices view in whichever mode it is
// currently showing. Used after a CRUD operation so the user stays in detail
// mode (instead of being kicked back to the landing list) when they
// add/edit/delete a price for a specific ticker.
func (s *priceViewState) reloadKeepingMode(d priceDeps) tea.Cmd {
	if s.data != nil && s.data.mode == pricesViewDetail && s.data.selectedSecurity != nil {
		return s.loadForSecurity(d, s.data.selectedSecurity)
	}
	return s.load(d)
}

// loadForSecurity returns a command that drills into a single security's price
// history (detail mode).
func (s *priceViewState) loadForSecurity(d priceDeps, sec *security.Security) tea.Cmd {
	return func() tea.Msg {
		secSvc, priceSvc := d.securities(), d.prices()
		if secSvc == nil || priceSvc == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		excludeHidden := true
		securities, err := secSvc.List(security.Filter{ExcludeHidden: &excludeHidden})
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load securities: %w", err)}
		}

		var prices []*price.Price
		if sec != nil {
			prices, err = priceSvc.GetPriceHistory(sec.ID, nil, nil)
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

// applyData installs a freshly loaded price view and builds the table its mode
// calls for. In list mode it also kicks off the initial debounced
// chart fetch for the row under the cursor, so the chart panel populates
// without requiring a keystroke; subsequent cursor movement reschedules.
//
// The existing historyCache is carried across the reload. Without that, the
// per-security evictions the price-CRUD handlers perform (PC-015) and the full
// clear bulk refresh performs (PC-016) would be silently undone by the fresh
// empty cache load constructs.
func (s *priceViewState) applyData(data *priceViewData) tea.Cmd {
	if s.data != nil && s.data.historyCache != nil {
		data.historyCache = s.data.historyCache
	}
	s.data = data
	switch data.mode {
	case pricesViewList:
		s.buildListTable()
		if secID := s.listCursorSecurityID(); !secID.IsNil() {
			return s.scheduleChartFetch(secID)
		}
	case pricesViewDetail:
		s.buildTable()
	}
	return nil
}

// afterPriceChange notes the change, drops the affected security's cached
// chart history, and reloads the price view in whichever mode it is in. Every
// price CRUD result ends this way; only the note differs.
func (a *App) afterPriceChange(note string) tea.Cmd {
	a.statusbar.AddNotification(note, widget.NotificationInfo)
	a.prices.evictSelectedSecurityFromHistoryCache()
	return a.prices.reloadKeepingMode(a.priceDeps())
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
		cmds = append(cmds, a.prices.load(a.priceDeps()))
	}
	return tea.Batch(cmds...)
}
