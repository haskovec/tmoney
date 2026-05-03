# Implementation Plan: Prices Chart Panel

This document defines the order in which the prices chart panel should be implemented. Each item represents one small session of work following a red-green (test-first) pattern. Mark items as complete with `[x]` as they are finished.

Spec: `specs/prices-chart.md`

## Status Legend

- `[ ]` Not started
- `[x]` Complete

## Priority Rationale

The plan is ordered so that the user-visible chart appears as early as possible (Phase 3), then is hardened against bad data (Phase 4), then made performant (Phases 5–6), then made correct under data changes (Phase 7), then documented (Phase 8). Pure helpers come first only because they are prerequisites for the visible step and are the cheapest, lowest-risk things to TDD.

A new Phase 2 was inserted *before* the visible MVP after we discovered that ntcharts v2 (the supported line) requires `charm.land/bubbletea/v2` and `charm.land/lipgloss/v2`. Pinning ntcharts v0.x is a dead end — the upstream chart library has moved on. We migrate the whole TUI stack to v2 first, then write the chart against ntcharts v2 with no risk of being stranded on a stale dependency.

1. **Phase 1 — Pure helpers**: tiny prerequisite functions (threshold, converter, edge-case strings). Cheap to TDD, no UI risk.
2. **Phase 2 — Charm v2 stack upgrade**: migrate `bubbletea`, `lipgloss`, and `bubbles` to their v2 lines (`charm.land/...`). Required before pulling in `ntcharts/v2`. Broken into small, individually-mergeable steps so the tree compiles between each.
3. **Phase 3 — Visible MVP**: chart appears next to the list at width ≥ 120 with synchronous fetch, no cache, no debounce. This is the headline phase and the riskiest unknown — it proves ntcharts integrates with the layout.
4. **Phase 4 — Edge cases**: 0-price, 1-price, flat-line, empty list, out-of-range cursor. Don't ship a feature that crashes on a brand-new security with one price.
5. **Phase 5 — Memoized cache**: avoid re-fetching the same history on revisit.
6. **Phase 6 — Async fetch with debounce**: 150 ms debounce, async `tea.Cmd`, prior-chart-during-wait. Makes the feature production-quality on rapid cursor movement.
7. **Phase 7 — Cache invalidation wiring**: per-security eviction on CRUD; full clear on bulk refresh.
8. **Phase 8 — Documentation**: README update.

Phases 5–6 could be reversed or combined; the spec's debounce-then-cache design works either way. Cache-first is sequenced earlier here because it's a pure data structure with simple unit tests, while debouncing is a `tea.Cmd` lifecycle change that touches more code.

---

## Phase 1: Pure Helpers

- [x] **PC-001 — Width threshold function**
  - RED: tests in `internal/tui/price_chart_test.go` for `shouldShowChartPanel(contentWidth int) bool` — `119 → false`, `120 → true`, `200 → true`, `0 → false`.
  - GREEN: implement `shouldShowChartPanel` in a new `internal/tui/price_chart.go` returning `contentWidth >= 120`.

- [x] **PC-002 — Edge-case placeholder strings**
  - RED: tests for `noPriceHistoryMessage()` returning `"No price history"` and `singlePriceMessage(p *price.Price)` returning `"Only one price on file — chart needs ≥ 2 points\n$185.50 on 2026-04-15"` (or equivalent format).
  - GREEN: implement both functions in `price_chart.go` using existing `formatPriceRow` helpers for consistency.

- [x] **PC-003 — Price history → time-series converter**
  - RED: tests for `priceHistoryToSeries(prices []*price.Price) (times []time.Time, values []float64)` covering: empty input → empty slices; single element → length-1 slices; multi-element preserves order; reverse-chronological input is rendered chronologically (ntcharts wants ascending time).
  - GREEN: implement converter in `price_chart.go`. Note: `GetPriceHistory` returns newest first, so the converter reverses.

- [x] **PC-004 — Flat-line Y-range clamp**
  - RED: tests for `clampYRange(values []float64) (min, max float64)` — distinct values pass through; all-equal values produce `[v − 0.5%·v, v + 0.5%·v]` with a non-zero spread; all-zero values produce `[-0.5, 0.5]` (avoid divide-by-zero in the percentage).
  - GREEN: implement `clampYRange` in `price_chart.go`.

## Phase 2: Charm v2 Stack Upgrade

This phase migrates the TUI from `github.com/charmbracelet/{bubbletea,lipgloss,bubbles}` v1 to the v2 lines hosted at `charm.land/...`. ntcharts v2 (the actively-maintained line) only works on top of these. Without this phase, every later step would either pin us to abandoned ntcharts v0.x or pull in two parallel charm stacks.

References:
- Bubble Tea v2 upgrade guide: `https://github.com/charmbracelet/bubbletea/blob/v2.0.0/UPGRADE_GUIDE_V2.md`
- Lip Gloss v2 upgrade guide: `https://github.com/charmbracelet/lipgloss/blob/v2.0.3/UPGRADE_GUIDE_V2.md`
- Bubbles v2: `https://github.com/charmbracelet/bubbles/releases/tag/v2.0.0`

Scope of v1 surface in this codebase (informs sequencing):
- ~61 files import `bubbletea`/`lipgloss`/`bubbles`.
- 53 files reference `tea.KeyMsg`.
- Mouse: `Dialog.HandleMouse(tea.MouseMsg, …)` is the one production handler; ~25 test sites construct `tea.MouseMsg{X:…, Y:…, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}`.
- Imperative program options live only in `internal/tui/app.go:525-526` (`tea.EnterAltScreen`, `tea.SetWindowTitle(…)`) and `app.go:4457` (`tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())`).
- Single `View() string` on `App` at `app.go:3128`. The `renderXxxView() string` helpers are internal and can keep returning strings — only the top-level `App.View()` has to return `tea.View`.
- Lipgloss usage is contained: 13 `lipgloss.Color("…")` literals (all numeric ANSI strings, no `AdaptiveColor`/`CompleteColor`/`Renderer`/`HasDarkBackground`), 77 `lipgloss.NewStyle()` calls. `compat` package is not needed.

Strategy: do the upgrade in a single working branch but as small, individually-reviewable commits. Between each commit, the tree must `go build ./...` and `go test ./...` clean. The order below is chosen so the compiler tells you exactly what is left to fix at each step.

- [x] **PC-U01 — Snapshot baseline**
  - RED: not applicable.
  - GREEN: confirm `go build ./...`, `go test ./...`, and `golangci-lint run` are all green on `main` before starting. Capture current code coverage from `go test -cover ./...` so we have a regression baseline.
  - Baseline captured 2026-05-02 (commit `b8f2a24`): `go build ./...` clean; `go test ./...` 4734 pass / 0 fail across 23 packages; `golangci-lint run` 0 issues. Per-package coverage:
    - `cmd/tmoney` 82.7% · `internal/account` 86.1% · `internal/app` 75.7% · `internal/backup` 76.3% · `internal/category` 78.6% · `internal/config` 75.0% · `internal/db` 68.9% · `internal/dberrors` 0.0% (no test file) · `internal/dbutil` 0.0% (no test file) · `internal/imexport` 87.7% · `internal/investment` 81.1% · `internal/payee` 83.8% · `internal/price` 87.2% · `internal/reconciliation` 72.5% · `internal/report` 83.0% · `internal/scheduled` 76.9% · `internal/security` 89.6% · `internal/transaction` 77.1% · `internal/transferlink` 0.0% (no test file) · `internal/tui` 68.7% · `internal/types` 64.6% · `internal/undo` 88.0% · `tests/integration` no statements.

- [x] **PC-U02 — Bubble Tea + Lip Gloss + Bubbles module bumps**
  - RED: not applicable (dependency-only step; build will be red after this until PC-U03+ land).
  - GREEN: in `go.mod`, replace
    - `github.com/charmbracelet/bubbletea v1.3.10` → `charm.land/bubbletea/v2 vX.Y.Z` (latest stable; v2.0.6 at time of writing)
    - `github.com/charmbracelet/lipgloss v1.1.0` → `charm.land/lipgloss/v2 vX.Y.Z`
    - `github.com/charmbracelet/bubbles v1.0.0` → `charm.land/bubbles/v2 vX.Y.Z`
  - Run `go get charm.land/bubbletea/v2@latest charm.land/lipgloss/v2@latest charm.land/bubbles/v2@latest`, then `go mod tidy`. Expect `go build` to fail — that's the to-do list for PC-U03+.

- [x] **PC-U03 — Mechanical import-path rewrite**
  - RED: not applicable (mechanical).
  - GREEN: in every `.go` file, replace
    - `github.com/charmbracelet/bubbletea` → `charm.land/bubbletea/v2`
    - `github.com/charmbracelet/lipgloss` → `charm.land/lipgloss/v2`
    - `github.com/charmbracelet/lipgloss/table` → `charm.land/lipgloss/v2/table`
    - `github.com/charmbracelet/bubbles/...` → `charm.land/bubbles/v2/...`
  - Run `go fix ./...` then `go build ./...`. Build will still be red on the API changes covered by PC-U04 onward, but every import error should be gone.

- [x] **PC-U04 — `App.View()` returns `tea.View`**
  - RED: extend the existing app render tests so they call `App.View()` and assert on `.Content` (or call `String()` on the returned `tea.View`). Existing assertions on the rendered string must continue to pass.
  - GREEN: in `internal/tui/app.go`:
    - Change signature `func (a *App) View() string` → `func (a *App) View() tea.View`, build the existing string into `tea.NewView(...)`.
    - Set `v.AltScreen = true` and `v.MouseMode = tea.MouseModeCellMotion` on the returned view (replaces `WithAltScreen()` / `WithMouseCellMotion()` from `tea.NewProgram` and the `EnterAltScreen` startup command).
    - Replace `tea.SetWindowTitle("…")` (Init-time command) with `v.WindowTitle = "TMoney - Personal Finance Manager"`.
    - Strip `tea.WithAltScreen()`, `tea.WithMouseCellMotion()` from the `tea.NewProgram(...)` call site.
    - Strip `tea.EnterAltScreen` and `tea.SetWindowTitle(...)` from the `Init()`/startup `tea.Batch`.

- [x] **PC-U05 — Production key-handling migration**
  - RED: keep existing key handler tests passing. Where a test today builds `tea.KeyMsg{Type: tea.KeyEnter}` to drive a handler, do not change those tests yet — they live behind PC-U07.
  - GREEN: in every non-test handler that does `case tea.KeyMsg:` or accepts `tea.KeyMsg` as a parameter, switch the type to `tea.KeyPressMsg` (interface narrowing). Handlers continue to call `msg.String()` for matching, which keeps working — that is the safe migration path. Verify the app builds after this step.

- [x] **PC-U06 — Production mouse-handling migration**
  - RED: as above, leave production-driving mouse tests for PC-U07.
  - GREEN: rewrite `Dialog.HandleMouse(msg tea.MouseMsg, ...)` to take the v2 `tea.MouseMsg` interface, extract `mouse := msg.Mouse()` for `X`/`Y`, and switch matching from `msg.Action == tea.MouseActionPress` + `msg.Button` to type-switch on `tea.MouseClickMsg` / `tea.MouseReleaseMsg` / `tea.MouseWheelMsg`. Update every caller. Replace `tea.MouseButtonLeft` with `tea.MouseLeft`, etc., per the v2 button-name table.

- [x] **PC-U07 — Test-fixture migration for keys and mice**
  - RED: tests will fail to compile after PC-U05/U06 land — this step is the green wave that brings them back.
  - GREEN: rewrite test fixtures:
    - `tea.KeyMsg{Type: tea.KeyEnter}` → `tea.KeyPressMsg{Code: tea.KeyEnter}`
    - `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}` → `tea.KeyPressMsg{Code: 'n', Text: "n"}`
    - `tea.KeyMsg{Type: tea.KeyCtrlD}` → `tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}` (or rely on `String() == "ctrl+d"`)
    - `tea.MouseMsg{X:…, Y:…, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}` → `tea.MouseClickMsg{Mouse: tea.Mouse{X:…, Y:…, Button: tea.MouseLeft}}`
    - `tea.MouseMsg{… Button: tea.MouseButtonWheelDown}` → `tea.MouseWheelMsg{Mouse: tea.Mouse{Button: tea.MouseWheelDown}}`
  - This is mechanical but voluminous (≥80 sites). Consider a single `gofmt`-friendly sed pass per pattern, then hand-fix outliers.

- [x] **PC-U08 — Lipgloss color-type adjustment**
  - RED: not applicable.
  - GREEN: `lipgloss.Color("214")` in v2 returns `image/color.Color`, not the v1 named-string type. The 13 package-level color constants in `internal/tui/styles.go` keep the same call shape, but their declared types may need to change (`lipgloss.Color` → `color.Color` from `image/color`) — `go build` will tell you. Add `import "image/color"` where needed.

- [x] **PC-U09 — Renames and removed APIs cleanup**
  - RED: not applicable.
  - GREEN: search-and-replace:
    - `tea.Sequentially(` → `tea.Sequence(` (none expected here, but verify)
    - `tea.WindowSize()` (a `Cmd`) → `tea.RequestWindowSize` (a `Msg`); update any caller (none expected here).
    - `case " ":` (literal space match) → `case "space":` if the surrounding code matches on `msg.String()`. Audit every `case " ":` to see whether it was matching the rune or the `String()` form.
  - Re-run `go fix ./...`, `go build ./...`, `go test ./...`, `golangci-lint run`. Tree must be fully green.

- [x] **PC-U10 — Coverage and behavioral regression check**
  - RED: not applicable.
  - GREEN: re-run `go test -cover ./...`, compare to PC-U01 baseline; investigate any new regressions. Manually launch the TUI against a real `.tdb`, exercise alt-screen entry, mouse clicks, key handling for the major views (dashboard, register, scheduled, prices). Commit, push, and merge before moving on to Phase 3.
  - Completed 2026-05-03: charm.land v2 migration landed in a single bundled commit (PC-U02 through PC-U10) because PC-U02 alone could not stand green between commits. Stack: `charm.land/bubbletea/v2 v2.0.6`, `charm.land/lipgloss/v2 v2.0.3`, `charm.land/bubbles/v2 v2.1.0`. Result: `go build ./...` clean; `go test ./...` 4734 pass / 0 fail across 23 packages (zero regression vs baseline); `golangci-lint run` 0 issues. Per-package coverage held at the PC-U01 baseline values exactly: `cmd/tmoney` 82.7% · `internal/account` 86.1% · `internal/app` 75.7% · `internal/backup` 76.3% · `internal/category` 78.6% · `internal/config` 75.0% · `internal/db` 68.9% · `internal/dberrors` 0.0% · `internal/dbutil` 0.0% · `internal/imexport` 87.7% · `internal/investment` 81.1% · `internal/payee` 83.8% · `internal/price` 87.2% · `internal/reconciliation` 72.5% · `internal/report` 83.0% · `internal/scheduled` 76.9% · `internal/security` 89.6% · `internal/transaction` 77.1% · `internal/transferlink` 0.0% · `internal/tui` 68.7% · `internal/types` 64.6% · `internal/undo` 88.0% · `tests/integration` no statements. Two test fixes were required: `reconciliation_view.go` matched `msg.String() == " "` for space, which in v2 returns `"space"` instead — fixed under PC-U09; `TestTable_Render_VoidRowStyle` asserted on the literal substring `"Void"` in rendered output, but lipgloss v2 wraps each strikethrough rune in its own ANSI escape, so the test was updated to `stripAnsi` first.

## Phase 3: Visible MVP

- [x] **PC-005 — Add ntcharts v2 dependency**
  - RED: not applicable (dependency-only step).
  - GREEN: `go get github.com/NimbleMarkets/ntcharts/v2`, run `go mod tidy`. Verify `go build ./...`. (Now safe because the surrounding `bubbletea`/`lipgloss` are v2.)
  - Completed 2026-05-03 bundled with PC-006: a standalone `go get` + `go mod tidy` produces a no-op commit because tidy strips an unused module — the dep only persists once a `.go` file imports it. Bundled with PC-006 (which adds the first real import) under a single commit.

- [x] **PC-006 — Render chart panel beside the list (synchronous fetch)**
  - RED: contract tests in `price_view_test.go` — at content width 120 with a security selected and ≥ 2 prices on file, the rendered list view contains the ticker and name in a bordered panel to the right of the table; at content width 119, no panel appears (output identical to today). Use existing TUI test helpers (`buildTestApp` or whatever pattern `price_view_test.go` already uses).
  - GREEN: in `renderPriceList`, when `shouldShowChartPanel(contentWidth)` is true:
    1. Identify the highlighted security from `priceListTable.Cursor()`.
    2. Synchronously call `priceSvc.GetPriceHistory(secID, nil, nil)` (no cache yet).
    3. Convert via `priceHistoryToSeries`, feed into a `timeserieslinechart`, render to a string.
    4. Wrap in a lipgloss bordered box with embedded title `<ticker> — <name>`.
    5. Compose with the existing table via `lipgloss.JoinHorizontal(lipgloss.Top, table, chartBox)`.
  - Verify visually: launch the TUI on a wide terminal against a real `.tdb` with prices.
  - Completed 2026-05-03. Stack pinned `github.com/NimbleMarkets/ntcharts/v2 v2.0.3`. New file split: `internal/tui/price_chart.go` got `buildChartPanel`, `renderTimeSeriesChart`, `composeChartBox`, and the unicode-aware `truncateToDisplayWidth` / `padOrTruncateToDisplayWidth` helpers. `internal/tui/price_view.go` factored the body of `renderPriceList` into `composePriceListBody` (chooses between full-width table and table+chart layout) and `buildPriceListChartPanel` (resolves the cursor's security, synchronously fetches `priceSvc.GetPriceHistory`, feeds `buildChartPanel`). Layout split: when `ContentWidth ≥ 120` the table is pinned at 75 columns (10+32+15+12 cols + 3 separators + 3 gutter) and the chart fills the remainder; below threshold the table fills the full content area exactly as before. Edge-case handling baked in from the start (0/1-price placeholders, flat-line clamp via `clampYRange`, nil-sec / too-small-area guards) so PC-007/008/009 are essentially already covered behaviorally — the explicit contract tests for those still need to land. Tests: 9 new tests added (`buildChartPanel` direct unit tests + 2 integration tests through `renderPriceView` driven by a real DB-backed `priceSvc` via `setupRefreshTUITest`). `go build ./...`, `go test ./...` (4743 pass / 0 fail across 23 packages, +9 net), `golangci-lint run` all clean.

## Phase 4: Edge Cases

- [x] **PC-007 — Render `No price history` for 0-price selection**
  - RED: contract test — at width 120 with a security selected that has 0 prices, the chart panel renders the `No price history` muted message instead of a chart. (Triggerable in tests by adding a security with no prices and selecting it; today's `latestPrices` excludes these so the test must inject directly.)
  - GREEN: branch in the chart-render path: if `len(prices) == 0`, render `noPriceHistoryMessage()` inside the box instead of the ntcharts canvas.
  - Completed 2026-05-03. The rendering branch already shipped under PC-006 (`buildChartPanel`'s `case 0`). PC-007 added the missing contract test `TestRenderPriceView_ListMode_ZeroPriceSecurityShowsPlaceholder` in `internal/tui/price_view_test.go`: drives `renderPriceView` through `composePriceListBody` → `buildPriceListChartPanel` → `priceSvc.GetPriceHistory` (returns empty) → `buildChartPanel` 0-price branch → `noPriceHistoryMessage()`, and asserts the rendered output contains both `No price history` and the bordered title `AAPL — AAPL Inc.` so the panel still renders (it just shows the placeholder instead of a line chart). Test injects a synthetic `LatestPrice` row pointing at a security with zero prices in the DB, since today's `loadPriceViewData` would otherwise filter it out. Tests: `go build ./...`, `go test ./...` 4744 pass / 0 fail across 23 packages (+1 net), `golangci-lint run` 0 issues. Coverage held at the PC-006 baseline; `internal/tui` ticked 68.7% → 68.8%.

- [x] **PC-008 — Render single-price message for 1-price selection**
  - RED: contract test — at width 120 with a 1-price security selected, the panel contains the `Only one price on file — chart needs ≥ 2 points` string and the value/date.
  - GREEN: branch: if `len(prices) == 1`, render `singlePriceMessage(prices[0])`.
  - Completed 2026-05-03. The 1-price routing branch already shipped under PC-006 (`buildChartPanel`'s `case 1` calls `singlePriceMessage(prices[0])`). PC-008 added the missing end-to-end contract test `TestRenderPriceView_ListMode_OnePriceSecurityShowsPlaceholder` in `internal/tui/price_view_test.go`: drives `renderPriceView` → `composePriceListBody` → `buildPriceListChartPanel` → `priceSvc.GetPriceHistory` (returns the single AddPrice'd row) → `buildChartPanel` 1-price branch → `singlePriceMessage()`, asserting the rendered output contains all four required strings — `Only one price on file`, `≥ 2 points`, `$185.50`, and the formatted date `2026-04-22` — plus the bordered title `AAPL — AAPL Inc.` to confirm the panel still renders. Tests: `go build ./...`, `go test ./...` 4745 pass / 0 fail across 23 packages (+1 net), `golangci-lint run` 0 issues. Coverage held at the PC-007 baseline exactly: `cmd/tmoney` 82.7% · `internal/account` 86.1% · `internal/app` 75.7% · `internal/backup` 76.3% · `internal/category` 78.6% · `internal/config` 75.0% · `internal/db` 68.9% · `internal/dberrors` 0.0% · `internal/dbutil` 0.0% · `internal/imexport` 87.7% · `internal/investment` 81.1% · `internal/payee` 83.8% · `internal/price` 87.2% · `internal/reconciliation` 72.5% · `internal/report` 83.0% · `internal/scheduled` 76.9% · `internal/security` 89.6% · `internal/transaction` 77.1% · `internal/transferlink` 0.0% · `internal/tui` 68.8% · `internal/types` 64.6% · `internal/undo` 88.0% · `tests/integration` no statements.

- [x] **PC-009 — Flat-line clamp wired into chart**
  - RED: contract test (or unit test on the rendering helper) — passing all-equal prices to the chart-builder does not panic and produces a non-empty render. Pair with PC-004's pure test.
  - GREEN: in the chart-build path, call `clampYRange` and pass the result to `timeserieslinechart`'s Y range setter (whatever its API name is).
  - Completed 2026-05-03. The wiring shipped under PC-006: `renderTimeSeriesChart` already calls `clampYRange(values)` and passes the result into `chart.SetYRange(minY, maxY)` / `chart.SetViewYRange(minY, maxY)` (see `internal/tui/price_chart.go:127-133`). The unit-level no-panic guard at `TestBuildChartPanel_FlatLineDoesNotPanic` was already in place. PC-009 added the missing end-to-end contract test `TestRenderPriceView_ListMode_FlatLinePriceHistoryRendersChart` in `internal/tui/price_view_test.go`: drives `renderPriceView` → `composePriceListBody` → `buildPriceListChartPanel` → `priceSvc.GetPriceHistory` (returns 3 prices all at $100.00) → `buildChartPanel` `default` branch → `renderTimeSeriesChart` → `clampYRange` (all-equal padding, ±0.5%). Asserts: no panic, title `AAPL — AAPL Inc.` is present, neither the 0-price nor 1-price placeholders fire, and box-drawing border is rendered. Tests: `go build ./...`, `go test ./...` 4746 pass / 0 fail across 23 packages (+1 net), `golangci-lint run` 0 issues. Coverage held at the PC-008 baseline.

- [x] **PC-010 — Skip panel on empty list / out-of-range cursor**
  - RED: contract test — at width 120 with `latestPrices` empty, no chart panel renders (existing empty-state message is the only output). Same for cursor index ≥ `len(latestPrices)`.
  - GREEN: early-return in the chart-render path before the `JoinHorizontal` if there's no valid selected security.
  - Completed 2026-05-03. Both early-returns shipped behaviorally under PC-006: `renderPriceList` short-circuits to the "No prices on file" hint when `len(latestPrices) == 0` (`internal/tui/price_view.go:287-293`), and `buildPriceListChartPanel` returns "" when `cursor < 0 || cursor >= len(latestPrices)` (`price_view.go:350-352`), with `composePriceListBody` falling back to the bare table on empty chart-panel string (`price_view.go:334-336`). PC-010 added the missing contract pins in `internal/tui/price_view_test.go`: `TestRenderPriceView_ListMode_WideEmptyOmitsChartPanel` drives width=200 / empty `latestPrices` through `renderPriceView` and asserts the empty hint appears with no `AAPL — AAPL Inc.` title decoration and no box-drawing border (`│`); `TestRenderPriceView_ListMode_OutOfRangeCursorOmitsChartPanel` constructs the transient out-of-range case (build table with two rows, `MoveDown` to cursor=1, then truncate `latestPrices` to one row without rebuilding the table), drives `renderPriceView`, and asserts the chart-panel title decoration is absent and no placeholder fires either. Tests: `go build ./...`, `go test ./...` 4748 pass / 0 fail across 23 packages (+2 net), `golangci-lint run` 0 issues. Coverage held at the PC-009 baseline exactly (`internal/tui` 68.8%; all other packages unchanged).

## Phase 5: Memoized Cache

- [x] **PC-011 — `historyCache` type with `Get`/`Evict`/`Clear`**
  - RED: unit tests in `price_chart_test.go` — `Get(id, loader)` calls `loader` once on first call and returns the cached slice on subsequent calls; `Evict(id)` causes the next `Get(id, loader)` to call `loader` again; `Clear()` evicts all entries; concurrent calls are safe (sync.Mutex).
  - GREEN: implement `historyCache` in `price_chart.go` as a `map[types.ID][]*price.Price` guarded by a `sync.Mutex`.
  - Completed 2026-05-03. Added `priceHistoryLoader` (a `func() ([]*price.Price, error)` alias) plus `historyCache` with `newHistoryCache()`, `Get(id, loader)`, `Evict(id)`, and `Clear()` in `internal/tui/price_chart.go`. Implementation guards a `map[types.ID][]*price.Price` with a `sync.Mutex`; the loader runs *outside* the lock to avoid serializing concurrent fetches across distinct ids, and the success result is then re-locked in. Loader errors are intentionally not cached so the next `Get` retries — this lets a transient DB/service failure not get pinned for the lifetime of the cache. 7 new tests in `price_chart_test.go` cover the contract: loader-once-per-id, distinct-ids-load-independently, post-`Evict` reload, unknown-`Evict` no-op, post-`Clear` reload-of-everything, error-not-cached (with recovery semantics), and a `-race`-driven concurrent stress test (32 goroutines × 50 iterations interleaving `Get`/`Evict`/`Clear` across 4 ids). Tests: `go fmt ./...` clean; `go build ./...` clean; `go test -race ./internal/tui/ -run 'TestHistoryCache'` 7 pass; `go test ./...` 4755 pass / 0 fail across 23 packages (+7 net vs PC-010 baseline of 4748); `golangci-lint run` 0 issues.

- [x] **PC-012 — Wire chart-render through the cache**
  - RED: integration-style test — render the list view twice with the same selection; assert the underlying price service is called exactly once. Use a counting fake `priceSvc` or wrap the existing one.
  - GREEN: add a `historyCache` field to `priceViewData` (initialized in `loadPriceViewData`); in the chart-render path, fetch via `cache.Get(secID, fetchFn)` instead of calling the service directly.
  - Completed 2026-05-03. Added `historyCache *historyCache` field to `priceViewData` with `newHistoryCache()` initialization in both `loadPriceViewData` (list mode) and `loadPriceViewDataForSecurity` (detail mode, for symmetry — PC-013/PC-015 may key off it later). `buildPriceListChartPanel` now wraps the `priceSvc.GetPriceHistory(targetID, nil, nil)` call in a `priceHistoryLoader` closure and routes through `a.priceView.historyCache.Get(targetID, loader)` when the cache is non-nil; falls back to a direct loader call when nil so existing test fixtures that hand-construct `priceViewData` without a cache (the rest of the PC-006/PC-007/PC-008/PC-009/PC-010 tests) keep passing untouched. Note: did *not* use `*price.Service` as a counting fake — the field is a concrete-typed pointer and an interface was rejected as scope creep. Instead, the new contract test `TestRenderPriceView_ListMode_ChartUsesHistoryCache` proves cache wiring behaviorally by mutating the underlying price store between renders: render once with 2 prices on file (full chart), `priceSvc.DeletePrice` the older row, render again — if the chart re-fetched it would route to the 1-price `singlePriceMessage` placeholder; cache hit means the placeholder is absent. Then call `priceView.historyCache.Clear()` and render a third time — must now reflect the post-delete state ("Only one price on file" placeholder appears), proving Clear actually drops the wired-in entry. This single test covers both `Get` (memoize) and `Clear` (invalidate) wiring in one round-trip. Tests: `go fix ./...`, `go fmt ./...` clean; `go build ./...` clean; `go test ./internal/tui/ -run 'TestRenderPriceView_ListMode_ChartUsesHistoryCache|TestHistoryCache'` 8 pass; `go test ./...` 4756 pass / 0 fail across 23 packages (+1 net vs PC-011 baseline of 4755); `golangci-lint run` 0 issues.

## Phase 6: Async Fetch with Debounce

- [x] **PC-013 — Debounced fetch on cursor change**
  - RED: tests covering: (a) a cursor change schedules a debounce timer command; (b) a second cursor change within the window cancels/replaces the first; (c) the timer firing produces a fetch command; (d) fetch results update the chart and trigger no further timer.
  - GREEN: replace the synchronous fetch in PC-006 with a `tea.Cmd` triggered by `tea.Tick(150*time.Millisecond, ...)`. Track the active "pending ticker" on `priceViewData`; ignore tick results that don't match the current cursor (handles the "user moved again before timer fired" case). Continue rendering the previously-loaded history in the meantime — no `Loading…` placeholder.
  - Completed 2026-05-03. Mechanism in place: `priceChartDebounceDelay = 150ms` (a `var`, so tests can shorten it), `priceChartDebounceTickMsg{gen, secID}` and `priceChartHistoryLoadedMsg{secID, prices}` carry the lifecycle. Each call to `App.schedulePriceChartFetch(secID)` bumps `priceView.chartDebounceGen` and emits a `tea.Tick` cmd capturing the new gen; the `priceChartDebounceTickMsg` handler in `app.go`'s `Update` drops ticks whose gen has been superseded *or* whose secID no longer matches `listCursorSecurityID()`, then either short-circuits on a cache hit (just promotes `chartDisplayedID = secID`) or returns `App.fetchPriceChartHistory(secID)`. The fetch cmd does the actual `priceSvc.GetPriceHistory` call off the render thread; the `priceChartHistoryLoadedMsg` handler stores the result via `historyCache.Put` and sets `chartDisplayedID`. The render path (`buildPriceListChartPanel`) is now strictly cache-only — it calls `historyCache.Lookup(highlightedID)`, falls back to `historyCache.Lookup(chartDisplayedID)` so the panel stays populated during the 150 ms debounce window after a cursor move ("Continue rendering the previously-loaded history" — no `Loading…` flash), and returns `""` when both miss. Two new methods on `historyCache`: `Lookup(id)` (read-only) and `Put(id, prices)` (write, called only by the async fetch path). Wiring: `handlePriceListKeys` returns the schedule cmd after every cursor-moving call (`Up`, `Down`, `g`/`Home`, `G`/`End`, `pgup`, `pgdown`) — though PC-014 is what *tests* the full key set; PC-013 hooked all six up front to avoid leaving Up/Down asymmetric in shipped code. Initial chart population is also handled: the `priceViewDataLoadedMsg` handler schedules a debounce on the row under the cursor when `mode == pricesViewList` so the chart panel populates without requiring a keystroke. Six new tests in `price_view_test.go` cover the contract: (a) `TestHandlePriceListKeys_DownSchedulesDebounceTick` — Down keypress returns a non-nil cmd that produces a `priceChartDebounceTickMsg` targeting the new cursor row at the current gen; (b) `TestPriceChartDebounceTick_StaleGenIsDropped` — two consecutive schedules; the older gen is dropped by `Update`, the newer is dispatched; (c) `TestPriceChartDebounceTick_DispatchesFetchOnMatch` — a current-gen + matching-cursor tick returns a fetch cmd whose execution yields a `priceChartHistoryLoadedMsg` carrying the priceSvc-fetched history; (d) `TestPriceChartHistoryLoadedMsg_UpdatesStateAndStops` — loaded msg writes to cache, sets `chartDisplayedID`, returns no further cmd; plus `TestPriceChartDebounceTick_CacheHitSkipsFetch` (cache-hit fast path) and `TestPriceChartDebounceTick_CursorMismatchIsDropped` (cursor moved off in the debounce window). The five existing PC-006/007/008/009/012 contract tests in `price_view_test.go` were updated to pre-populate `historyCache.Put(secID, prices)` since the chart-render path no longer queries `priceSvc`; the PC-012 test was rewritten — it still proves "render uses the cache" by mutating `priceSvc` between renders, but its post-`Clear` expectation flipped from "render shows 1-price placeholder" to "render omits the panel entirely" because the new model has no synchronous re-fetch fallback. Tests: `go fix ./...`, `go fmt ./...` clean; `go build ./...` clean; `go test ./...` 4762 pass / 0 fail across 23 packages (+6 net vs PC-012 baseline of 4756); `golangci-lint run` 0 issues. Coverage: `internal/tui` 68.8% → 68.9% (+0.1%); every other package held at the PC-012 baseline exactly: `cmd/tmoney` 82.7% · `internal/account` 86.1% · `internal/app` 75.7% · `internal/backup` 76.3% · `internal/category` 78.6% · `internal/config` 75.0% · `internal/db` 68.9% · `internal/dberrors` 0.0% · `internal/dbutil` 0.0% · `internal/imexport` 87.7% · `internal/investment` 81.1% · `internal/payee` 83.8% · `internal/price` 87.2% · `internal/reconciliation` 72.5% · `internal/report` 83.0% · `internal/scheduled` 76.9% · `internal/security` 89.6% · `internal/transaction` 77.1% · `internal/transferlink` 0.0% · `internal/types` 64.6% · `internal/undo` 88.0% · `tests/integration` no statements.

- [ ] **PC-014 — Cursor-change hook in list-mode key handlers**
  - RED: extend PC-013 tests to cover that all six cursor-moving keys (`Up`, `Down`, `g`/`Home`, `G`/`End`, `pgup`, `pgdown`) trigger the debounce, and that `/` (search) and other non-cursor keys don't.
  - GREEN: in `handlePriceListKeys`, after each cursor-moving call, return the debounce-scheduling command alongside the existing return.

## Phase 7: Cache Invalidation Wiring

- [ ] **PC-015 — Per-security eviction on CRUD messages**
  - RED: tests that asserting after a `priceAddedMsg`, `priceUpdatedMsg`, `priceDeletedMsg`, or `priceImportedMsg` is handled, the cache entry for the affected security is evicted while other entries remain.
  - GREEN: in each of the four message handlers (currently calling `reloadPriceViewKeepingMode`), call `priceView.historyCache.Evict(selectedSecurity.ID)` before reload.

- [ ] **PC-016 — Full cache clear on bulk refresh**
  - RED: test that handling the `u` refresh result message clears every entry in the cache.
  - GREEN: in the bulk-refresh result handler (see `internal/tui/refresh_prices.go`), call `priceView.historyCache.Clear()`.

## Phase 8: Documentation

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
