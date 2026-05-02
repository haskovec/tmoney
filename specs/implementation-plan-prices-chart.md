# Implementation Plan: Prices Chart Panel

This document defines the order in which the prices chart panel should be implemented. Each item represents one small session of work following a red-green (test-first) pattern. Mark items as complete with `[x]` as they are finished.

Spec: `specs/prices-chart.md`

## Status Legend

- `[ ]` Not started
- `[x]` Complete

## Priority Rationale

The plan is ordered so that the user-visible chart appears as early as possible (Phase 2), then is hardened against bad data (Phase 3), then made performant (Phases 4–5), then made correct under data changes (Phase 6), then documented (Phase 7). Pure helpers come first only because they are prerequisites for the visible step and are the cheapest, lowest-risk things to TDD.

1. **Phase 1 — Pure helpers**: tiny prerequisite functions (threshold, converter, edge-case strings). Cheap to TDD, no UI risk.
2. **Phase 2 — Visible MVP**: chart appears next to the list at width ≥ 120 with synchronous fetch, no cache, no debounce. This is the headline phase and the riskiest unknown — it proves ntcharts integrates with the layout.
3. **Phase 3 — Edge cases**: 0-price, 1-price, flat-line, empty list, out-of-range cursor. Don't ship a feature that crashes on a brand-new security with one price.
4. **Phase 4 — Memoized cache**: avoid re-fetching the same history on revisit.
5. **Phase 5 — Async fetch with debounce**: 150 ms debounce, async `tea.Cmd`, prior-chart-during-wait. Makes the feature production-quality on rapid cursor movement.
6. **Phase 6 — Cache invalidation wiring**: per-security eviction on CRUD; full clear on bulk refresh.
7. **Phase 7 — Documentation**: README update.

Phases 4–5 could be reversed or combined; the spec's debounce-then-cache design works either way. Cache-first is sequenced earlier here because it's a pure data structure with simple unit tests, while debouncing is a `tea.Cmd` lifecycle change that touches more code.

---

## Phase 1: Pure Helpers

- [ ] **PC-001 — Width threshold function**
  - RED: tests in `internal/tui/price_chart_test.go` for `shouldShowChartPanel(contentWidth int) bool` — `119 → false`, `120 → true`, `200 → true`, `0 → false`.
  - GREEN: implement `shouldShowChartPanel` in a new `internal/tui/price_chart.go` returning `contentWidth >= 120`.

- [ ] **PC-002 — Edge-case placeholder strings**
  - RED: tests for `noPriceHistoryMessage()` returning `"No price history"` and `singlePriceMessage(p *price.Price)` returning `"Only one price on file — chart needs ≥ 2 points\n$185.50 on 2026-04-15"` (or equivalent format).
  - GREEN: implement both functions in `price_chart.go` using existing `formatPriceRow` helpers for consistency.

- [ ] **PC-003 — Price history → time-series converter**
  - RED: tests for `priceHistoryToSeries(prices []*price.Price) (times []time.Time, values []float64)` covering: empty input → empty slices; single element → length-1 slices; multi-element preserves order; reverse-chronological input is rendered chronologically (ntcharts wants ascending time).
  - GREEN: implement converter in `price_chart.go`. Note: `GetPriceHistory` returns newest first, so the converter reverses.

- [ ] **PC-004 — Flat-line Y-range clamp**
  - RED: tests for `clampYRange(values []float64) (min, max float64)` — distinct values pass through; all-equal values produce `[v − 0.5%·v, v + 0.5%·v]` with a non-zero spread; all-zero values produce `[-0.5, 0.5]` (avoid divide-by-zero in the percentage).
  - GREEN: implement `clampYRange` in `price_chart.go`.

## Phase 2: Visible MVP

- [ ] **PC-005 — Add ntcharts dependency**
  - RED: not applicable (dependency-only step).
  - GREEN: `go get github.com/NimbleMarkets/ntcharts`, run `go mod tidy`. Verify build.

- [ ] **PC-006 — Render chart panel beside the list (synchronous fetch)**
  - RED: contract tests in `price_view_test.go` — at content width 120 with a security selected and ≥ 2 prices on file, the rendered list view contains the ticker and name in a bordered panel to the right of the table; at content width 119, no panel appears (output identical to today). Use existing TUI test helpers (`buildTestApp` or whatever pattern `price_view_test.go` already uses).
  - GREEN: in `renderPriceList`, when `shouldShowChartPanel(contentWidth)` is true:
    1. Identify the highlighted security from `priceListTable.Cursor()`.
    2. Synchronously call `priceSvc.GetPriceHistory(secID, nil, nil)` (no cache yet).
    3. Convert via `priceHistoryToSeries`, feed into a `timeserieslinechart`, render to a string.
    4. Wrap in a lipgloss bordered box with embedded title `<ticker> — <name>`.
    5. Compose with the existing table via `lipgloss.JoinHorizontal(lipgloss.Top, table, chartBox)`.
  - Verify visually: launch the TUI on a wide terminal against a real `.tdb` with prices.

## Phase 3: Edge Cases

- [ ] **PC-007 — Render `No price history` for 0-price selection**
  - RED: contract test — at width 120 with a security selected that has 0 prices, the chart panel renders the `No price history` muted message instead of a chart. (Triggerable in tests by adding a security with no prices and selecting it; today's `latestPrices` excludes these so the test must inject directly.)
  - GREEN: branch in the chart-render path: if `len(prices) == 0`, render `noPriceHistoryMessage()` inside the box instead of the ntcharts canvas.

- [ ] **PC-008 — Render single-price message for 1-price selection**
  - RED: contract test — at width 120 with a 1-price security selected, the panel contains the `Only one price on file — chart needs ≥ 2 points` string and the value/date.
  - GREEN: branch: if `len(prices) == 1`, render `singlePriceMessage(prices[0])`.

- [ ] **PC-009 — Flat-line clamp wired into chart**
  - RED: contract test (or unit test on the rendering helper) — passing all-equal prices to the chart-builder does not panic and produces a non-empty render. Pair with PC-004's pure test.
  - GREEN: in the chart-build path, call `clampYRange` and pass the result to `timeserieslinechart`'s Y range setter (whatever its API name is).

- [ ] **PC-010 — Skip panel on empty list / out-of-range cursor**
  - RED: contract test — at width 120 with `latestPrices` empty, no chart panel renders (existing empty-state message is the only output). Same for cursor index ≥ `len(latestPrices)`.
  - GREEN: early-return in the chart-render path before the `JoinHorizontal` if there's no valid selected security.

## Phase 4: Memoized Cache

- [ ] **PC-011 — `historyCache` type with `Get`/`Evict`/`Clear`**
  - RED: unit tests in `price_chart_test.go` — `Get(id, loader)` calls `loader` once on first call and returns the cached slice on subsequent calls; `Evict(id)` causes the next `Get(id, loader)` to call `loader` again; `Clear()` evicts all entries; concurrent calls are safe (sync.Mutex).
  - GREEN: implement `historyCache` in `price_chart.go` as a `map[types.ID][]*price.Price` guarded by a `sync.Mutex`.

- [ ] **PC-012 — Wire chart-render through the cache**
  - RED: integration-style test — render the list view twice with the same selection; assert the underlying price service is called exactly once. Use a counting fake `priceSvc` or wrap the existing one.
  - GREEN: add a `historyCache` field to `priceViewData` (initialized in `loadPriceViewData`); in the chart-render path, fetch via `cache.Get(secID, fetchFn)` instead of calling the service directly.

## Phase 5: Async Fetch with Debounce

- [ ] **PC-013 — Debounced fetch on cursor change**
  - RED: tests covering: (a) a cursor change schedules a debounce timer command; (b) a second cursor change within the window cancels/replaces the first; (c) the timer firing produces a fetch command; (d) fetch results update the chart and trigger no further timer.
  - GREEN: replace the synchronous fetch in PC-006 with a `tea.Cmd` triggered by `tea.Tick(150*time.Millisecond, ...)`. Track the active "pending ticker" on `priceViewData`; ignore tick results that don't match the current cursor (handles the "user moved again before timer fired" case). Continue rendering the previously-loaded history in the meantime — no `Loading…` placeholder.

- [ ] **PC-014 — Cursor-change hook in list-mode key handlers**
  - RED: extend PC-013 tests to cover that all six cursor-moving keys (`Up`, `Down`, `g`/`Home`, `G`/`End`, `pgup`, `pgdown`) trigger the debounce, and that `/` (search) and other non-cursor keys don't.
  - GREEN: in `handlePriceListKeys`, after each cursor-moving call, return the debounce-scheduling command alongside the existing return.

## Phase 6: Cache Invalidation Wiring

- [ ] **PC-015 — Per-security eviction on CRUD messages**
  - RED: tests that asserting after a `priceAddedMsg`, `priceUpdatedMsg`, `priceDeletedMsg`, or `priceImportedMsg` is handled, the cache entry for the affected security is evicted while other entries remain.
  - GREEN: in each of the four message handlers (currently calling `reloadPriceViewKeepingMode`), call `priceView.historyCache.Evict(selectedSecurity.ID)` before reload.

- [ ] **PC-016 — Full cache clear on bulk refresh**
  - RED: test that handling the `u` refresh result message clears every entry in the cache.
  - GREEN: in the bulk-refresh result handler (see `internal/tui/refresh_prices.go`), call `priceView.historyCache.Clear()`.

## Phase 7: Documentation

- [ ] **PC-017 — README update**
  - GREEN: add ~2 lines under the README's `#### Prices` section noting that on terminals where the prices content area is at least 120 columns wide, a chart panel renders to the right of the list showing the highlighted ticker's full price history. No new keyboard shortcut.

---

## Out of scope (per spec non-goals)

The following are explicitly deferred and have no plan items here. Reopen the spec before starting any of them.

- Chart in detail mode.
- Time-window switcher (`1M / 3M / 6M / 1Y / ALL`).
- Sparkline column inside the list table.
- Multi-ticker overlay.
- Up/down conditional line color.
- Candlestick rendering (would also require a schema change).
