// Prices view: the chart's price-history cache and its debounced fetch. The
// chart panel itself renders in package pricechart.

package tui

import (
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

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

// priceChartDebounceDelay is the time a cursor must dwell on a row
// before the chart panel issues a fetch for the highlighted ticker.
// Declared as a var so tests can shorten it; user-facing default is 150 ms.
var priceChartDebounceDelay = 150 * time.Millisecond

// priceChartDebounceTickMsg is delivered when a scheduled debounce timer
// fires. The handler in app.go's Update verifies (a) gen still matches
// prices.data.chartDebounceGen (i.e. no later schedule has superseded
// this one), and (b) the cursor is still on secID; if both hold, it
// dispatches the actual price-history fetch. Otherwise it drops the
// tick — that's how rapid cursor movement collapses to a single fetch.
type priceChartDebounceTickMsg struct {
	gen   int
	secID types.ID
}

// priceChartHistoryLoadedMsg carries the result of a debounced fetch.
// Its handler stores prices in prices.data.historyCache and sets
// chartDisplayedID = secID so the next render shows the new ticker.
type priceChartHistoryLoadedMsg struct {
	secID  types.ID
	prices []*price.Price
}

// scheduleChartFetch returns a debounced tea.Cmd that, after
// priceChartDebounceDelay elapses, emits a priceChartDebounceTickMsg
// for secID. Each call bumps data.chartDebounceGen so any earlier
// in-flight tick becomes stale (the tick handler drops mismatched gen).
// Returns nil when there is no price data to schedule against.
func (s *priceViewState) scheduleChartFetch(secID types.ID) tea.Cmd {
	if s.data == nil {
		return nil
	}
	s.data.chartDebounceGen++
	gen := s.data.chartDebounceGen
	return tea.Tick(priceChartDebounceDelay, func(_ time.Time) tea.Msg {
		return priceChartDebounceTickMsg{gen: gen, secID: secID}
	})
}

// scheduleListChartFetchIfActive returns a debounced chart-fetch cmd for the
// security under the prices-list cursor, or nil when the prices landing list
// isn't the active surface. onScreen is whether the Prices view is the current
// view. Mouse selection (single click,
// wheel scroll) calls this so the chart panel tracks the cursor exactly
// as keyboard navigation does (handlePriceListKeys). Without it the table
// highlight moves but the chart keeps showing the previously fetched
// ticker via the chartDisplayedID fallback in buildListChartPanel.
func (s *priceViewState) scheduleListChartFetchIfActive(onScreen bool) tea.Cmd {
	if !onScreen || s.data == nil || s.data.mode != pricesViewList {
		return nil
	}
	secID := s.listCursorSecurityID()
	if secID.IsNil() {
		return nil
	}
	return s.scheduleChartFetch(secID)
}

// fetchChartHistory returns a tea.Cmd that synchronously calls the
// price service for secID's full history and emits a
// priceChartHistoryLoadedMsg. On error, it returns no message — the
// chart simply stays in its current state until the next cursor move.
func (s *priceViewState) fetchChartHistory(d priceDeps, secID types.ID) tea.Cmd {
	return func() tea.Msg {
		priceSvc := d.prices()
		if priceSvc == nil {
			return nil
		}
		prices, err := priceSvc.GetPriceHistory(secID, nil, nil)
		if err != nil {
			return nil
		}
		return priceChartHistoryLoadedMsg{secID: secID, prices: prices}
	}
}

// handleChartDebounceTick fetches the chart history for the row the tick
// was scheduled against, unless the tick is stale (a later schedule superseded
// it), the cursor has moved off that row (the move scheduled its own tick), or
// the history is already cached — in which case it only promotes the cached
// series to displayed.
func (s *priceViewState) handleChartDebounceTick(d priceDeps, msg priceChartDebounceTickMsg) tea.Cmd {
	if s.data == nil || msg.gen != s.data.chartDebounceGen || s.listCursorSecurityID() != msg.secID {
		return nil
	}
	if s.data.historyCache != nil {
		if _, ok := s.data.historyCache.Lookup(msg.secID); ok {
			s.data.chartDisplayedID = msg.secID
			return nil
		}
	}
	return s.fetchChartHistory(d, msg.secID)
}

// applyChartHistory caches a fetched series and shows it.
func (s *priceViewState) applyChartHistory(msg priceChartHistoryLoadedMsg) {
	if s.data == nil {
		return
	}
	if s.data.historyCache != nil {
		s.data.historyCache.Put(msg.secID, msg.prices)
	}
	s.data.chartDisplayedID = msg.secID
}
