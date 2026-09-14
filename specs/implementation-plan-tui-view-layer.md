# Implementation Plan: TUI View Layer (slice 4b)

This document defines the order in which the view-layer design is built. Each item is one small session of work that leaves the tree compiling and the tests green. Mark items as complete with `[x]` as they are finished.

Design: `specs/design-tui-view-layer.md`
Earlier design (4a, 4c, 4d): `specs/design-tui-decomposition.md`

## Status Legend

- `[ ]` Not started
- `[x]` Complete

## Priority Rationale

The plan is ordered by the value of each phase and by the cost of doing it later.

1. **Phase 0 — Corporate Actions help section.** A live bug: `?` on that view shows no view-specific keys. Bug fixes go before refactors. Ships alone.
2. **Phase 1 — The view table.** The core of the design. It collapses seven hand-written view lists into one, and its guards stop the phase 0 bug from coming back. It changes zero test lines, so it is cheap to review.
3. **Phase 3 — Split the two god files.** Independent of the other phases. Zero test lines and zero logic change. It runs *before* phase 2 so that phase 2's field churn lands in small files, and so that a later split does not have to re-touch phase 2's diff.
4. **Phase 2 — Per-view state structs.** The largest change (about 335 test literal lines in about 30 files). It is mechanical, but it is the most churn, so it goes after the two phases that need no test edits.
5. **Phase 4 — View controllers.** Priced, not committed. Produces a table of per-view decisions, not code.

Rules that hold for every item:

- No new package. No `tea.Model` sub-model. No change to key bindings, layout, or which views are full-screen.
- The message arms in `app_update.go` do not move.
- `currentView` stays a field on `App`.
- The three pre-switch branches in `handleKeyPress` (`app.go:447`, `app.go:495`, `app.go:548`) do not move.

---

## Phase 0: Corporate Actions Help Section (bug fix, ships alone)

- [ ] **VL-001 — Generic help-overlay test**
  - RED: in `help_overlay_test.go`, add a test that ranges over every `View` constant, renders the help overlay with `currentView` set to that view, and asserts that at least one section beyond the global and navigation sections is present. It must fail for `ViewCorporateActions` today.
  - Enumerate the constants with `go/ast` over the `View` const block in `app.go`. Fail if the set is empty. This enumerator is reused by VL-101, so put it in a shared test helper.

- [ ] **VL-002 — `corporateActionShortcuts()` and the eleventh arm**
  - GREEN: add `corporateActionShortcuts()` in `help_overlay.go` and the `case ViewCorporateActions` arm in `viewShortcutSections`. Keys: `/` filter, Enter details, `d` delete (reverse the action, after a confirm), Esc back or clear, `g`/`G`/PgUp/PgDn move.
  - The section must match the status-bar hint at `app_view.go:174`.
  - Open the PR. This is the only item in the PR.

## Phase 1: The View Table

Only the seven switches collapse. No method moves. Every fallback stays.

- [ ] **VL-101 — `views.go` with ids and names; guard 1**
  - Create `internal/tui/views.go` with `viewEntry` (only `id` and `name` for now), package-level `allViews` with eleven entries, `views()`, and `viewFor(v View) (viewEntry, bool)`. Neither accessor takes a receiver.
  - RED then GREEN in a new `views_guard_test.go`: **guard 1** — every `View` constant has exactly one entry, and no entry has a duplicate `id`. Reuse the VL-001 enumerator. Fail on an empty constant set.

- [ ] **VL-102 — `View.String()` reads the table**
  - Replace the switch at `app.go:51` with a `viewFor` lookup. Keep the `"Unknown"` fallback; `TestViewString` pins it for `View(999)`.

- [ ] **VL-103 — `render` and `fullScreen`; guard 3**
  - Add `render func(*App) string` and `fullScreen bool` to `viewEntry`. Fill `render` with the existing `renderX` method values. Set `fullScreen` on Reconciliation, Securities, Prices, CorporateActions, Amortization.
  - `renderContent` (`app_view.go:94`): the eleven-arm switch becomes `e.render(a)` with the `"Unknown view"` fallback. The five-way `||` becomes `e.fullScreen || a.styles.SidebarWidth() == 0`.
  - `handleMouseContent` (`app_mouse.go:81`): the five-way `||` becomes the same disjunct. Keep the `sidebarWidth == 0` half and the inner Dashboard branch as the design's §Phase 1 shows.
  - RED then GREEN: **guard 3** — no expression outside `views.go` compares `currentView` against three or more `View` constants. Add its self-test over fabricated source, in the `controller_guard_test.go` pattern.

- [ ] **VL-104 — `onKey`**
  - Add `onKey func(*App, tea.KeyPressMsg) (tea.Model, tea.Cmd)`. Fill with the existing `handleXKeys` method values.
  - Replace the eleven-arm switch at `app.go:568` with the lookup. The three earlier branches in `handleKeyPress` stay exactly where they are.

- [ ] **VL-105 — `hints`**
  - Add `hints func(*App) string`. Replace the switch in `getKeyHints` (`app_view.go:148`) with the lookup. Keep the existing default.

- [ ] **VL-106 — `shortcuts`**
  - Add `shortcuts func() shortcutSection`. Replace the switch in `viewShortcutSections` (`help_overlay.go:225`) with the lookup. The VL-001 test must stay green.

- [ ] **VL-107 — `table` (a func, because two views pick by mode)**
  - Add `table func(*App) *widget.Table`. Replace the switch in `activeTable` (`app_helpers.go:66`). Prices picks by `priceView.mode`; Portfolio picks by `portfolioMode`. Views without a table return nil.
  - Test: both modes of Prices and both modes of Portfolio return the expected table.

- [ ] **VL-108 — `reload`, nil for two views**
  - Add `reload func(*App) []tea.Cmd`. Replace the switch in `reloadCurrentView` (`app_helpers.go:174`). Reconciliation and CorporateActions get `reload: nil`, as today.
  - Test: exactly those two entries have a nil `reload`. This records the behaviour; it does not invent a reload. File the product question with the owner of those two views.

- [ ] **VL-109 — `focus`, verbatim per arm; the tables-nil walk**
  - Add `focus func(*App)`. Copy each arm of the focus block in `switchView` (`app_menu.go:296`) into its entry verbatim, every `!= nil` guard included. Do **not** level Amortization (`app_menu.go:366`, sidebar off only) with Corporate Actions (`app_menu.go:353`, sidebar off and table focused when non-nil).
  - Test (§5.2): build `&App{sidebar: NewSidebar(), statusbar: widget.NewStatusBar(), styles: widget.NewStyles()}` with every table nil. For each `View` value, set `currentView` to a different view and call `switchView(v)`. No panic.

- [ ] **VL-110 — Guard 2 with self-test**
  - RED then GREEN: **guard 2** — parse every production file in the package; a `switch` outside `views.go` whose cases name more than four `View` constants fails. State the number four in the guard's message. Add its self-test over fabricated source.
  - This item is last in the phase because it fails until VL-102 to VL-109 are done.

- [ ] **VL-111 — Exit check and manual smoke**
  - Confirm: seven switches gone; the five-view list gone from both predicates; `View.String()` returns `"Unknown"` for a miss; the three pre-switch branches untouched; guards 1 to 3 green with self-tests.
  - Manual smoke: visit every view from the View menu, press `?`, click a table row, scroll, and drill from Securities into Corporate Actions and back.
  - Set the design document's phase 1 status to built.

## Phase 3: Split the Two God Files (the 4d motion)

Move functions between files. Rename nothing. Change no signature. Zero test lines. One commit per new file.

- [ ] **VL-201 — Rebuild the throwaway `go/ast` tools**
  - In the scratchpad, rebuild the two small `go/ast` tools the 4d work used: a mover that moves top-level declarations by name and reports any comment it would orphan, and a comparer that checks every declaration, doc comment included, is byte-identical to the original and none is missing or duplicated. Run the comparer after every item below.

- [ ] **VL-202 — `price_dialog.go`**
  - Move `handlePriceDialogKey`, `priceDialogAction`, `startPriceLookup`, `lookupPriceCmd`, `handlePriceLookupResult`, `submitPriceDialog`, `createPrice`, `updatePrice` and their free helpers. About 200 lines. One file-scope comment: this is a modal surface in `modals()`, not view code.

- [ ] **VL-203 — `price_import_dialog.go`**
  - Move `handlePriceImportDialogKey`, `priceImportDialogAction`, `submitImportPriceDialog`, `importPrices`. About 100 lines.

- [ ] **VL-204 — Chart methods into the existing `price_chart.go`**
  - Move `schedulePriceChartFetch`, `schedulePriceListChartFetchIfActive`, `fetchPriceChartHistory`, `handlePriceChartDebounceTick`, `applyPriceChartHistory`. About 130 lines. The chart then has one file.

- [ ] **VL-205 — `price_view_render.go`**
  - Move `buildPriceListTable`, `buildPriceTable`, `formatPriceRow`, `selectedPrice`, `renderPriceView`, `renderPriceList`, `composePriceListBody`, `buildPriceListChartPanel`, `resolveListPriceSecurity`, `listCursorSecurityID`, `renderPriceDetail`. About 350 lines.

- [ ] **VL-206 — `price_view_keys.go`; check the gate**
  - Move `handlePriceViewKeys`, `handlePriceListKeys`, `handlePriceDetailKeys`, `drillIntoSelectedListRow`, `handlePriceSearchKey`. About 200 lines.
  - Check: `price_view.go` ≤ 450 lines and holds only data, load and apply.

- [ ] **VL-207 — `investment_type_selector.go`**
  - Move `openInvestmentTypeSelector`, `handleInvestmentTypeSelectorKey`, `investmentTypeSelectorAction`, `dispatchInvestmentTypeSelection` and the type helpers. About 210 lines.

- [ ] **VL-208 — `investment_register_filter.go`**
  - Move the security filter and its search-key handler. About 190 lines.

- [ ] **VL-209 — `investment_register_render.go`; check the gate**
  - Move the render functions. About 200 lines.
  - Check: `investment_register_view.go` ≤ 500 lines; comparer reports zero problems and zero orphaned comments. Set the design document's phase 3 status to built.

## Phase 2: Per-View State Structs

One struct per view, in the view's own file. Move the test literals with the same perl-and-compile motion the services collapse used. No assertion changes. Each item is one view, so each PR is one struct and its literal churn.

Fields that stay on `App` in every item: `currentView`, `previousView`, `pendingRegisterSelectID`, `pendingInvestmentSelectID`, `refreshingPrices`, `refreshNotifID`, `corporateActionViewFilter`, and the modal fields `priceSurface`, `priceImportDialog`, `investmentTypeSelector`, `security`.

- [ ] **VL-301 — `priceViewState` (first, smallest with two tables)**
  - Fields: `data`, `table`, `listTable`, `clicks`. `a.priceView` becomes `a.prices.data` and so on. The two dialogs are not in it. Update the VL-107 `table` func.

- [ ] **VL-302 — No-service guard for view structs**
  - RED then GREEN in `views_guard_test.go`: walk `App`'s fields for struct types declared in this package whose pointer does **not** implement `Modal`; fail if that set is empty; fail if any holds a pointer in `servicePointerTypes()`. No hand list.

- [ ] **VL-303 — `dashboardViewState`**
  - Fields: `data`, `expandedAccounts`, `accountRows`. Test (§5.1): a click on a dashboard row without a prior render is a no-op, not a stale account.

- [ ] **VL-304 — `registerViewState`**
  - Fields: `data`, `table`. `pendingRegisterSelectID` stays on `App`.

- [ ] **VL-305 — `investmentRegisterViewState` and its `leave()` hook**
  - Fields: `data`, `table`, `editTxnID`, `filterSearching`, `filterQuery`, `filterLockedSec`, `newTxnSecurityID`. `pendingInvestmentSelectID` stays on `App`.
  - Add `leave func(*App)` to `viewEntry`. Move the filter-clearing special case out of `switchView` (`app_menu.go:300`) into this entry's `leave`. Test: leaving the view clears the filter fields.

- [ ] **VL-306 — `portfolioViewState`**
  - Fields: `data`, `holdingsTable`, `lotsTable`, `mode`. Update the VL-107 `table` func.

- [ ] **VL-307 — `scheduledViewState`**
  - Fields: `data`, `table`.

- [ ] **VL-308 — `reportsViewState`**
  - Field: `data`.

- [ ] **VL-309 — `reconciliationViewState`**
  - Fields: `data`, `table`.

- [ ] **VL-310 — `securityViewState`**
  - Fields: `data`, `table`, `pendingSelectID`. `pendingSecuritySelectID` moves here because only this view reads and writes it.

- [ ] **VL-311 — `corporateActionViewState` and its `leave()` hook**
  - Fields: `data`, `table`, `detail`, `filterEditing`. `corporateActionViewFilter` stays on `App`.
  - Move the detail-dropping special case out of `switchView` into this entry's `leave`. The filter is **not** cleared on leave. Test: a round trip keeps the filter and drops the detail.

- [ ] **VL-312 — `amortizationViewState`**
  - Fields: `data`, `table`.

- [ ] **VL-313 — Exit check**
  - Confirm: `switchView` has no per-view `if`; `App` is under about 60 fields; each view's state is one field; the five recorded decisions applied as written; no assertion changed. Set the design document's phase 2 status to built.

## Phase 4: View Controllers (priced, not committed)

- [ ] **VL-401 — Per-view measurement table**
  - For each of the eleven views, grep its files for `*App` methods that name only the view's own state struct, styles and services. Record per view: count that could move under the 4c rule, count pinned, and why. Append the table to `specs/design-tui-view-layer.md` as the 4c notes were appended to the earlier design. No code moves in this item.

- [ ] **VL-402 — Decide per view**
  - From the VL-401 table, open one item per view that is worth the move. Each is its own future plan item; none is committed here.
