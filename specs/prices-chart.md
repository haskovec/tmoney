# Prices Chart Panel Specification

## Overview

The prices list view (`5`) currently shows a table of every non-hidden security with at least one price on file: ticker, name, latest price, latest date. This spec adds a **chart panel** that renders the price history of the currently highlighted ticker as a line chart, sitting to the right of the list table on sufficiently wide terminals.

The chart is a passive read-only visual aid for browsing — it has no focus, no cursor, and no interactive controls. Moving the cursor on the list re-renders the chart for the new ticker.

## Goals

- Give the user a visual sense of a security's price trajectory while scanning the prices list.
- Make multi-security comparison fast: arrow up/down through the list and the chart updates underneath.
- Stay within the project's restrained TUI aesthetic (lazygit / Turbo Vision, not Bloomberg).

## Non-goals (v1)

The following are deliberately deferred. They are not part of v1 and should not be added without revisiting this spec.

- **Chart in detail mode.** Detail mode is for editing prices (`n`/`d`/`i`/`Enter`); a chart there competes with the table for horizontal space without adding new information. Reconsider after v1 is in use.
- **Time-window switcher** (`1M / 3M / 6M / 1Y / ALL`). Most users have weeks-to-months of history at this point; window controls would be ceremony for nothing. Add when at least one user has multi-year daily data and the always-ALL view is visibly noisy.
- **Sparkline column inside the list table.** Possible future enhancement; not required to ship the side panel.
- **Multi-ticker overlay / comparison chart.**
- **Up-vs-down conditional line color.** Encodes a judgment based on a window the user didn't choose; misleading on short or freshly-tracked histories.
- **Candlesticks.** Would require schema changes to store OHLC; the price model today is `(date, price, source)`.

## Visibility and Layout

### Width threshold

The chart panel is visible only when the prices content area is at least **120 columns** wide. Below the threshold, the prices list looks exactly as it does today — the chart is *not* stacked below the table.

The threshold is on `a.styles.ContentWidth()`, not on raw terminal width, so it correctly accounts for any sidebar that may be present.

At exactly 120 columns of content width, with the existing table at ~75 columns, the chart panel gets ~45 columns. After 2 columns of border and ~6 columns for ntcharts' Y-axis labels, the actual plot area is ~37 columns — tight but legible. The threshold is intentionally low to make the chart available on more terminals.

### Layout structure

```
┌─────────── PRICES list table ──────────────┐  ┌─ AAPL — Apple Inc. ─────────┐
│ Ticker  Name           Latest Price  Date  │  │                             │
│ AAPL    Apple Inc.       $185.50  2026-04 │  │      ╭─╮                    │
│ MSFT    Microsoft        $410.20  2026-04 │  │   ╭─╯ ╰─╮                   │
│ ...                                        │  │ ──╯     ╰─                  │
│                                            │  │                             │
└────────────────────────────────────────────┘  └─────────────────────────────┘
```

- **Left column**: existing list table, unchanged widths.
- **Right column**: bordered, titled box containing the ntcharts canvas. Title is the ticker plus name of the highlighted row, embedded in the top border (e.g., `┌─ AAPL — Apple Inc. ─┐`).
- **Vertical sizing**: chart box matches the table's height exactly. Plot area ≈ box height − 2 (top/bottom border).
- **No gap separator** beyond the boxes' borders themselves.

The chart title row inside the border is redundant with the highlighted table row, *except* that the table scrolls and the box title doesn't — the user always knows which security is being charted.

## Chart Rendering

### Library

This feature adds the `github.com/NimbleMarkets/ntcharts` dependency. The component used is `timeserieslinechart`, which:

- Plots `(time.Time, float64)` pairs natively.
- Handles uneven time spacing correctly (Mon → Tue → Friday is not squashed to three equal slots).
- Draws a time-aware X axis with date labels.
- Is bubbletea-native (returns rendered strings via a `View()` method).

### Time window

The chart shows **all available history** for the selected security. There are no window-switching controls in v1.

### Styling

- Single solid line color, no fill, no conditional coloring.
- A new `Chart` lipgloss style in `styles.go` (or reuse `Title`) defines the line color.
- Axis labels and gridlines (if shown) use the existing `Muted` style.
- Box border uses the same border style as other lipgloss-bordered TUI panels.

## Data Flow

### Source

Price history per security is loaded via the existing `priceSvc.GetPriceHistory(securityID, nil, nil)` call (no date filtering — `all history`).

### Update behavior

- Cursor movement on the list **schedules a 150 ms debounced fetch** of the highlighted security's history. If the user keeps moving, the timer resets.
- During the debounce window, the chart **continues to display the previously rendered ticker's data** rather than a "Loading…" placeholder. This feels more responsive and the brief mismatch is benign.
- Once the timer fires, the fetch runs as a `tea.Cmd`. When the result arrives, the chart re-renders with the new dataset.

### In-memory cache

Histories already fetched are memoized for the lifetime of the prices view:

```go
// On priceViewData
historyCache map[types.ID][]*price.Price
```

- **Cache hit**: skip the DB call entirely; the debounce timer still applies (so rapid cursor movement is still throttled), but the actual chart update uses the cached slice.
- **Cache miss**: fetch and store before rendering.

#### Eviction rules

| Event | Effect on cache |
|-------|-----------------|
| Add price (`priceAddedMsg`) | Evict that security's entry |
| Edit price (`priceUpdatedMsg`) | Evict that security's entry |
| Delete price (`priceDeletedMsg`) | Evict that security's entry |
| CSV import (`priceImportedMsg`) | Evict that security's entry |
| Bulk refresh `u` | Clear entire cache |
| Leave and re-enter prices view | **Keep** cache (it's still valid) |
| Switch database file | Cache is rebuilt automatically with the new `priceView` |

The CRUD invalidations hook into the existing message handlers that already trigger `reloadPriceViewKeepingMode()` — they call `evict(id)` before the reload. The bulk-refresh path calls `clear()` because the provider response doesn't tell us per-ticker which rows changed.

> **Note**: this cache assumes prices change only via TUI-mediated actions (CRUD, import, bulk refresh). If background price refresh is ever introduced (e.g., a daemon), this cache becomes silently stale — invalidate or remove it at that point.

## Edge Cases

| Condition | Panel content |
|-----------|---------------|
| Selected security has 0 prices | Muted text: `No price history` |
| Selected security has exactly 1 price | Muted text: `Only one price on file — chart needs ≥ 2 points` followed by `<value> on <date>` |
| All prices equal (flat line) | Y axis clamped to a non-zero range (e.g., ±0.5% of value) so the chart renders a horizontal line rather than dividing by zero |
| List is empty (no securities have prices yet) | Panel does not render; the existing empty-state message in the list area is the only content |
| Cursor out of range (e.g., search filtered to 0 rows) | Panel does not render |
| Terminal width below threshold | Panel does not render; list looks exactly like today |

## Code Organization

A new file `internal/tui/price_chart.go` contains:

- The `historyCache` type and its `evict` / `clear` helpers.
- The `[]*price.Price → []time.Time, []float64` converter used to feed `timeserieslinechart`.
- The threshold check (`shouldShowChartPanel(contentWidth int) bool`).
- The placeholder-text logic for edge cases.

These are pure functions and are unit-tested directly without spinning up an `App`.

Wiring lives in `price_view.go`:

- `renderPriceList` composes the list table and the chart panel side by side using `lipgloss.JoinHorizontal` when `shouldShowChartPanel` returns true.
- A cursor-change hook (in `handlePriceListKeys` after `MoveUp`/`MoveDown`/`MoveToTop`/`MoveToBottom`/`PageUp`/`PageDown`) schedules the debounced fetch.
- The four CRUD message handlers (`priceAddedMsg`, `priceUpdatedMsg`, `priceDeletedMsg`, `priceImportedMsg`) call `historyCache.evict(secID)` before reload.
- The `u` refresh path calls `historyCache.clear()` after the bulk-refresh result arrives.

## Testing

Tests assert behavior, not rendered glyphs. The chart's exact pixel output is owned by ntcharts and may change across versions without changing correctness.

### Required tests

- **Threshold**:
  - `contentWidth = 119` → chart panel is not present in the rendered output.
  - `contentWidth = 120` → chart panel is present, with the highlighted ticker in the box title.
- **Edge case messages**:
  - 0-price security renders the `No price history` string.
  - 1-price security renders the `Only one price on file…` string plus the value/date.
- **Cache behavior** (unit tests on the cache type):
  - Two consecutive loads for the same ID hit the DB once.
  - After `evict(id)`, the next load for that ID hits the DB.
  - After `clear()`, the next load for any ID hits the DB.
- **Eviction wiring**:
  - `priceAddedMsg`/`priceUpdatedMsg`/`priceDeletedMsg`/`priceImportedMsg` evict only the affected security's entry.
  - The `u` refresh path clears the entire cache.

Snapshot tests of the chart's rendered output are **not** used.

## Documentation

The README's "Prices" TUI section gets a brief addition noting that on terminals at least 120 columns wide, a chart panel renders to the right of the list showing the highlighted ticker's full price history.

No new keyboard shortcut is documented — the chart auto-renders based on the existing cursor position and width; there is no toggle.
