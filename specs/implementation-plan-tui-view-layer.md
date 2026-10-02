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
- The four pre-switch branches in `handleKeyPress` do not move: the investment-register filter guard, the Corporate Actions filter guard (added by W12 in `specs/work-two-ledgers.md`), the Reconciliation branch, and the global Esc arm with its per-view exceptions.

---

## Phase 0: Corporate Actions Help Section (bug fix, ships alone)

This phase is item W1 in `specs/work-two-ledgers.md`. Ship it from there. When W1 ships, mark VL-001 and VL-002 with its commit.

- [x] **VL-001 — Generic help-overlay test** (W1, PR #46)
  - RED: in `help_overlay_test.go`, add a test that ranges over every `View` constant, renders the help overlay with `currentView` set to that view, and asserts that at least one section is present that is not Global, Navigation, Dialogs, or Mouse (Dialogs and Mouse are always appended, so they prove nothing). It must fail for `ViewCorporateActions` today.
  - Enumerate the constants with `go/ast` over the `View` const block in `app.go`. Fail if the set is empty. This enumerator is reused by VL-101, so put it in a shared test helper.

- [x] **VL-002 — `corporateActionShortcuts()` and the eleventh arm** (W1, PR #46)
  - GREEN: add `corporateActionShortcuts()` in `help_overlay.go` and the `case ViewCorporateActions` arm in `viewShortcutSections`. Keys: `/` filter, Enter details, `d` delete (reverse the action, after a confirm), Esc back, `g`/`G`/PgUp/PgDn move. (Esc cannot clear the filter or close details: `handleKeyPress` claims Esc before the view handler runs.)
  - The section must match the status-bar hint at `app_view.go:174`.
  - Open the PR. This is the only item in the PR.

## Phase 1: The View Table

Only the seven switches collapse. No method moves. Every fallback stays.

- [x] **VL-101 — `views.go` with ids and names; guard 1**
  - Create `internal/tui/views.go` with `viewEntry` (only `id` and `name` for now), package-level `allViews` with eleven entries, `views()`, and `viewFor(v View) (viewEntry, bool)`. Neither accessor takes a receiver.
  - RED then GREEN in a new `views_guard_test.go`: **guard 1** — every `View` constant has exactly one entry, and no entry has a duplicate `id`. Reuse the VL-001 enumerator. Fail on an empty constant set.

- [x] **VL-102 — `View.String()` reads the table**
  - Replace the switch at `app.go:51` with a `viewFor` lookup. Keep the `"Unknown"` fallback; `TestViewString` pins it for `View(999)`.

- [x] **VL-103 — `render` and `fullScreen`; guard 3**
  - Add `render func(*App) string` and `fullScreen bool` to `viewEntry`. Fill `render` with the existing `renderX` method values. Set `fullScreen` on Reconciliation, Securities, Prices, CorporateActions, Amortization.
  - `renderContent` (`app_view.go:94`): the eleven-arm switch becomes `e.render(a)` with the `"Unknown view"` fallback. The five-way `||` becomes `e.fullScreen || a.styles.SidebarWidth() == 0`.
  - `handleMouseContent` (`app_mouse.go:81`): the five-way `||` becomes the same disjunct. Keep the `sidebarWidth == 0` half and the inner Dashboard branch as the design's §Phase 1 shows.
  - RED then GREEN: **guard 3** — no expression outside `views.go` compares `currentView` against three or more `View` constants. Add its self-test over fabricated source, in the `controller_guard_test.go` pattern.

- [x] **VL-104 — `onKey`**
  - Add `onKey func(*App, tea.KeyPressMsg) (tea.Model, tea.Cmd)`. Fill with the existing `handleXKeys` method values.
  - Replace the eleven-arm switch at `app.go:568` with the lookup. The four earlier branches in `handleKeyPress` stay exactly where they are.
  - Done: with `onKey` in the table, the key handlers reach `View.String()`, which reads `allViews`, and Go refuses that initialization cycle. `init` fills `allViews` instead.

- [x] **VL-105 — `hints`**
  - Add `hints func(*App) string`. Replace the switch in `getKeyHints` (`app_view.go:148`) with the lookup. Keep the existing default.

- [x] **VL-106 — `shortcuts`**
  - Add `shortcuts func() shortcutSection`. Replace the switch in `viewShortcutSections` (`help_overlay.go:225`) with the lookup. The VL-001 test must stay green.

- [x] **VL-107 — `table` (a func, because two views pick by mode)**
  - Add `table func(*App) *widget.Table`. Replace the switch in `activeTable` (`app_helpers.go:66`). Prices picks by `priceView.mode`; Portfolio picks by `portfolioMode`. Views without a table return nil.
  - Test: both modes of Prices and both modes of Portfolio return the expected table.

- [x] **VL-108 — `reload`, copied from the W2 arms**
  - Add `reload func(*App) []tea.Cmd`. Replace the switch in `reloadCurrentView` (`app_helpers.go:174`).
  - Correction (2026-09-27): the nil reload for Reconciliation and CorporateActions was the bug that W2 in `specs/work-two-ledgers.md` fixes. W2 ships first (W8 needs it). Copy its two arms verbatim: Reconciliation reloads through `loadReconciliationData` only when a session is on screen, and keeps the check marks that are still candidates; CorporateActions calls `loadCorporateActionViewData`.
  - Test: no entry has a nil `reload`. The W2 reload tests stay green.
  - Done: the test checks every func that a lookup calls without a nil check (`render`, `onKey`, `hints`, `shortcuts`, `reload`, `focus`).

- [x] **VL-109 — `focus`, verbatim per arm; the tables-nil walk**
  - Add `focus func(*App)`. Copy each arm of the focus block in `switchView` (`app_menu.go:296`) into its entry verbatim, every `!= nil` guard included. Do **not** level Amortization (`app_menu.go:366`, sidebar off only) with Corporate Actions (`app_menu.go:353`, sidebar off and table focused when non-nil).
  - Test (§5.2): build `&App{sidebar: NewSidebar(), statusbar: widget.NewStatusBar(), styles: widget.NewStyles()}` with every table nil. For each `View` value, set `currentView` to a different view and call `switchView(v)`. No panic.
  - Done: a script compared each `focus` body with its old arm (11 of 11 the same). The walk test fails when one nil check is removed.

- [x] **VL-110 — Guard 2 with self-test**
  - RED then GREEN: **guard 2** — parse every production file in the package; a `switch` outside `views.go` whose cases name more than four `View` constants fails. State the number four in the guard's message. Add its self-test over fabricated source.
  - This item is last in the phase because it fails until VL-102 to VL-109 are done.

- [x] **VL-111 — Exit check and manual smoke**
  - Confirm: seven switches gone; the five-view list gone from both predicates; `View.String()` returns `"Unknown"` for a miss; the four pre-switch branches untouched; guards 1 to 3 green with self-tests.
  - Manual smoke: visit every view from the View menu, press `?`, click a table row, scroll, and drill from Securities into Corporate Actions and back.
  - Set the design document's phase 1 status to built.
  - Done: the View menu holds only Theme, so the smoke test reached the views by their keys, the sidebar, and the view keys. It ran in a pty with fictional data, and its 40 screens matched the build before this phase. It found one old bug, left alone: the Prices detail hint never shows (see the design's phase 1 status). W13 fixes it in PR #62.

## Phase 3: Split the Two God Files (the 4d motion)

Move functions between files. Rename nothing. Change no signature. Zero test lines. One commit per new file.

- [x] **VL-201 — Rebuild the throwaway `go/ast` tools**
  - In the scratchpad, rebuild the two small `go/ast` tools the 4d work used: a mover that moves top-level declarations by name and reports any comment it would orphan, and a comparer that checks every declaration, doc comment included, is byte-identical to the original and none is missing or duplicated. Run the comparer after every item below.

- [x] **VL-202 — `price_dialog.go`**
  - Move `handlePriceDialogKey`, `priceDialogAction`, `startPriceLookup`, `lookupPriceCmd`, `handlePriceLookupResult`, `submitPriceDialog`, `createPrice`, `updatePrice` and their free helpers. About 200 lines. One file-scope comment: this is a modal surface in `modals()`, not view code.

- [x] **VL-203 — `price_import_dialog.go`**
  - Move `handlePriceImportDialogKey`, `priceImportDialogAction`, `submitImportPriceDialog`, `importPrices`. About 100 lines.

- [x] **VL-204 — Chart methods into the existing `price_chart.go`**
  - Move `schedulePriceChartFetch`, `schedulePriceListChartFetchIfActive`, `fetchPriceChartHistory`, `handlePriceChartDebounceTick`, `applyPriceChartHistory`. About 130 lines. The chart then has one file.

- [x] **VL-205 — `price_view_render.go`**
  - Move `buildPriceListTable`, `buildPriceTable`, `formatPriceRow`, `selectedPrice`, `renderPriceView`, `renderPriceList`, `composePriceListBody`, `buildPriceListChartPanel`, `resolveListPriceSecurity`, `listCursorSecurityID`, `renderPriceDetail`. About 350 lines.

- [x] **VL-206 — `price_view_keys.go`; check the gate**
  - Move `handlePriceViewKeys`, `handlePriceListKeys`, `handlePriceDetailKeys`, `drillIntoSelectedListRow`, `handlePriceSearchKey`. About 200 lines.
  - Check: `price_view.go` ≤ 450 lines and holds only data, load and apply.

- [x] **VL-207 — `investment_type_selector.go`**
  - Move `openInvestmentTypeSelector`, `handleInvestmentTypeSelectorKey`, `investmentTypeSelectorAction`, `dispatchInvestmentTypeSelection` and the type helpers. About 210 lines.

- [x] **VL-208 — `investment_register_filter.go`**
  - Move the security filter and its search-key handler. About 190 lines.

- [x] **VL-209 — `investment_register_render.go`; check the gate**
  - Move the render functions. About 200 lines.
  - Check: `investment_register_view.go` ≤ 500 lines; comparer reports zero problems and zero orphaned comments. Set the design document's phase 3 status to built.
  - Done (branch `refactor/split-price-and-register-views`): `price_view.go` 261 lines, `investment_register_view.go` 398; all 1,120 declarations byte-identical. The chart's debounce delay and two messages also moved to `price_chart.go`. The four table-build helpers (`investmentRegisterColumns`, `shouldShowInvestmentBalance`, `buildInvestmentRegisterTable`, `formatInvestmentRegisterRow`) moved with the render functions, so `investment_register_render.go` is 329 lines; left in the view file, they would make that file about 525 lines, over the gate. The two free section headers that became file comments were removed from `price_view.go` by hand, since the mover refuses to orphan a comment.

## Phase 2: Per-View State Structs

One struct per view, in the view's own file. Move the test literals with the same perl-and-compile motion the services collapse used. No assertion changes. Each item is one view, so each PR is one struct and its literal churn.

Fields that stay on `App` in every item: `currentView`, `previousView`, `pendingRegisterSelectID`, `pendingInvestmentSelectID`, `investmentEditTxnID`, `investmentNewTxnSecurityID`, `refreshingPrices`, `refreshNotifID`, `corporateActionViewFilter`, and the modal fields `price` (a `priceSurface`), `priceImportDialog`, `investmentTypeSelector`, `security`.

- [x] **VL-301 — `priceViewState` (first, smallest with two tables)**
  - Fields: `data`, `table`, `listTable`, `clicks`. `a.priceView` becomes `a.prices.data` and so on. The two dialogs are not in it. Update the VL-107 `table` func.
  - Done: a throwaway `go/ast` tool in the scratchpad (`regroup`) made the move. It rewrites each selector and gathers the keys of each `App{...}` literal into one `prices: priceViewState{...}` element. It refuses a file where a removed key has a comment. No assertion changed; two failure messages now name the new paths. `switchDatabase` and `reloadAfterRestore` still clear only `data` and `table`, not `listTable`, as before.

- [x] **VL-302 — No-service guard for view structs**
  - RED then GREEN in `views_guard_test.go`: walk `App`'s fields for struct types declared in this package whose pointer does **not** implement `Modal`; fail if that set is empty; fail if any holds a pointer in `servicePointerTypes()`. No hand list.
  - Done: `TestGuard_NoViewStateHoldsAService`. The rule also finds `App`'s other non-modal structs (`Sidebar`, `keyMap`, `backupDialogState`, `mergerConfirmSurface`), and none may hold a service either. The walk goes down through pointers, slices, arrays, maps and this package's structs, because after VL-301 the view's data struct is one level below `App`. RED: a `*price.Service` put in `priceViewData` for a moment failed the guard at `priceViewState.data.svc`. The self-test runs the finder and the walk over fabricated types.

- [x] **VL-303 — `dashboardViewState`**
  - Fields: `data`, `expandedAccounts`, `accountRows`. Test (§5.1): a click on a dashboard row without a prior render is a no-op, not a stale account.
  - Done: `a.dashboard` is now the state struct, so `a.dashboard` became `a.dashboard.data`. The test is `TestApp_Dashboard_MouseClickBeforeRenderIsNoOp`; with stale rows put in the map, it fails. A stale row cannot occur in the running program: Bubble Tea renders after every `Update` (`tea.go`, `eventLoop`), so a click always reads the rows of the latest render. `switchDatabase` and `reloadAfterRestore` still clear only `data`, as before.

- [x] **VL-304 — `registerViewState`**
  - Fields: `data`, `table`. `pendingRegisterSelectID` stays on `App`.
  - Done: `a.register` is now the state struct, so `a.register` became `a.register.data` and `a.table` became `a.register.table`. Other structs also have a field named `table` (the view entry, `priceViewState`), so the tool rewrote only the selectors whose receiver is an `App` (`a`, `app`, `updatedApp`, `env.app`), and the compiler checked the rest.

- [x] **VL-305 — `investmentRegisterViewState` and its `leave()` hook**
  - Fields: `data`, `table`, `filterSearching`, `filterQuery`, `filterLockedSec`. `pendingInvestmentSelectID` stays on `App`.
  - Add `leave func(*App)` to `viewEntry`. Move the filter-clearing special case out of `switchView` (`app_menu.go:300`) into this entry's `leave`. Test: leaving the view clears the filter fields.
  - Decision (2026-09-30): `editTxnID` and `newTxnSecurityID` were in this list, but they stay on `App` as `investmentEditTxnID` and `investmentNewTxnSecurityID`. More than one surface writes each, which is the design's rule for what stays (§2.2). The type selector sets both. Every investment dialog reads the edit ID, and `afterInvestmentSave` and `afterTransferSave` clear it. `takeInvestmentDialogSeed` reads and clears the preselect ID as each dialog is built.
  - Done: `a.investmentRegister` is now the state struct, so `a.investmentRegister` became `a.investmentRegister.data`. `leave` is `(*App).resetInvestmentRegisterFilter`; `switchView` calls the leaving view's `leave` when it is not nil. `TestSwitchView_LeavingTheInvestmentRegisterClearsItsFilter` checks all three filter fields, and that a switch to the view on screen keeps them. Without the hook, it fails, as does the older `TestInvestmentFilter_ClearedOnLeavingView`.

- [x] **VL-306 — `portfolioViewState`**
  - Fields: `data`, `holdingsTable`, `lotsTable`, `mode`. Update the VL-107 `table` func.
  - Done: `App` holds it as `portfolio`, so `a.portfolioData` became `a.portfolio.data` and `a.portfolioMode` became `a.portfolio.mode`. The `table` func calls `activePortfolioTable`, which picks by `a.portfolio.mode`; the VL-107 test still checks both modes. Code outside the view clears `data` (a sidebar click, `p` in the investment register, a reversed corporate action, a database switch). That drops a cache; it is not a handoff, so the field moves.

- [x] **VL-307 — `scheduledViewState`**
  - Fields: `data`, `table`.
  - Done: `App` holds it as `scheduled`, so `a.scheduled` became `a.scheduled.data` and `a.scheduledTable` became `a.scheduled.table`. Other structs have a `scheduled` field too (the schedule dialog's data, two deps bags), so the tool rewrote only `App` receivers. The two schedule dialogs read the view's selected row; only the view and the database-switch resets write its state.

- [x] **VL-308 — `reportsViewState`**
  - Field: `data`.
  - Done: `App` holds it as `reports`, so `a.reports` became `a.reports.data`. The field count does not change: one field becomes one field, so every view has the same shape. The tool's receiver filter missed `model.(*App).reports` in three test lines; the compiler found them, and they were fixed by hand.

- [x] **VL-309 — `reconciliationViewState`**
  - Fields: `data`, `table`.
  - Done: `App` holds it as `reconciliation`, so `a.reconciliation` became `a.reconciliation.data` and `a.reconciliationTable` became `a.reconciliation.table`. `reconDialog` (a modal) and `reconDialogLastStatementDate` (the dialog's sticky date) stay on `App`. The Reconciliation branch in `handleKeyPress` reads only `currentView`, so it did not change. The W2 reload tests stay green.

- [x] **VL-310 — `securityViewState`**
  - Fields: `data`, `table`, `pendingSelectID`. `pendingSecuritySelectID` moves here because only this view reads and writes it.
  - Done: `App` holds it as `securities` (beside the `security` surface, as `prices` sits beside `price`), so `a.securityView` became `a.securities.data`, `a.securityTable` became `a.securities.table`, and `a.pendingSecuritySelectID` became `a.securities.pendingSelectID`. Checked before the move: the `securityAddedMsg` arm sets the ID after the security dialog saves, which is the shape of the register IDs that stay on `App`. But only the Securities view opens that dialog (`security_view.go`), and the whole add path is in the view's file, so the ID is the view's own.

- [x] **VL-311 — `corporateActionViewState` and its `leave()` hook**
  - Fields: `data`, `table`, `detail`, `filterEditing`. `corporateActionViewFilter` stays on `App`.
  - Move the departure special case out of `switchView` into this entry's `leave`. It does two things today, and `leave` must do both: drop the detail overlay, and end a filter entry (`filterEditing = false`), or the view comes back with every key captured as filter text. The filter is **not** cleared on leave. Test: a round trip keeps the filter, drops the detail, and ends the filter entry.
  - Done: `App` holds it as `corporateActions` (the view's name, as `investmentRegister` is; the review of #75 renamed it from `corporateActionView`), so the old data field `a.corporateActionView` became `a.corporateActions.data`, `a.corporateActionDetail` became `a.corporateActions.detail`, and so on. `detail` can move because it is a panel inside the view, not a modal (`modal.go`); `isDialogVisible` checks it on its own. The `leave` hook drops the detail and ends a filter entry, so `switchView` has no per-view `if`. `TestCorporateActions_RoundTripKeepsTheFilter` fails with no hook, and with a hook that also clears the filter; the two older tests (`TestCorporateActionDetail_ViewSwitchClearsOverlay`, `TestCorporateActions_LeavingEndsFilterEntry`) fail with no hook. `App` has 71 fields.

- [x] **VL-312 — `amortizationViewState`**
  - Fields: `data`, `table`.
  - Done: `App` holds it as `amortization`, so `a.amortizationData` became `a.amortization.data` and `a.amortizationTable` became `a.amortization.table`. The `focus` func did not change: it turns the sidebar off and does not touch the table, as VL-109 requires. `App` has 70 fields.

- [x] **VL-313 — Exit check**
  - Confirm: `switchView` has no per-view `if`; `App` is at about 70 fields (first written as "under about 60", a miscount: it left out the eleven new struct fields); each view's state is one field; the six recorded decisions applied as written (the sixth is in VL-305); no assertion changed. Set the design document's phase 2 status to built.
  - Done: all confirmed; the design's phase 2 status note has the details. `App` has 70 fields. A throwaway `go/ast` comparer found no changed or removed assertion in 1,596 test functions (4,523 assertions), and it reports one planted to differ. The no-service guard finds all eleven view structs. The `data: nil` literals were kept: all twelve are in tests of the nil or loading state, where the explicit nil names the state under test.

## Phase 4: View Controllers (priced, not committed)

- [x] **VL-401 — Per-view measurement table**
  - For each of the eleven views, grep its files for `*App` methods that name only the view's own state struct, styles and services. Record per view: count that could move under the 4c rule, count pinned, and why. Append the table to `specs/design-tui-view-layer.md` as the 4c notes were appended to the earlier design. No code moves in this item.
  - Done: the table is in the design's phase 4 section. 112 of the 154 view methods could move and 42 are pinned. A grep could not follow calls (a method that calls a pinned one is pinned), so a throwaway `go/ast` tool applied the rule until nothing changed. A read-only value that the view does not own (the screen size, the key bindings, `currentView`, the config, the ticker filter) counts as an input that the caller passes in, as styles are; a write pins. Found on the way: Reports' net-worth render reads the Dashboard's expand state, and the Reports and Amortization key handlers are not pinned by Esc, although the design predicted that they would be.

- [x] **VL-402 — Decide per view**
  - From the VL-401 table, open one item per view that is worth the move. Each is its own future plan item; none is committed here.
  - Done: the rule, the seven views it opens and the four it does not, and the proposed guard shape are in the design's phase 4 section ("Decided (VL-402)"). A view is worth the move when at least two thirds of its methods could move. VL-403 to VL-409 below are the seven items. They are not committed.

### Phase 4 items (opened by VL-402, not committed)

Each item moves the view's movable methods onto its state struct, with the 4c shape: a deps struct for the services, bound by one `App` method, and the inputs passed in at the call. The counts are from VL-401. "Calls" are the call sites that change: production code that stays on `App`, and tests.

Where each moved function goes (the design's VL-403 note): a function that reads or writes the view state, or takes the view's deps, is a method on the state; a pure helper that takes neither is a plain function. Each item also tests the entry closures it adds, as VL-403 does.

- [x] **VL-403 — Pilot: Amortization**
  - Move all 5 methods (`amortizationStatsLine`, `buildAmortizationTable`, `handleAmortizationKeys`, `loadAmortizationData`, `renderAmortizationView`). Deps: services. Inputs: styles, keys, width, height. Calls: 3 production, 5 tests.
  - Settle the guard shape. The proposal: a table for views beside `controllerSurfaces`, with the guard that no method on the view state names `App` and the two deps guards, and without the reach guard (the design says why). Mutation-verify each guard, as the 4c rows were.
  - Record the real cost as the 4c notes did: methods moved, methods added, lines, test churn. It prices the items after this one. If the cost is much higher than the count suggests, stop and re-decide before VL-404.
  - Done: the design's phase 4 section has the note ("Built (VL-403, the pilot)"). `*App` 424 → 420: five methods moved onto `amortizationViewState`, and one arrived (`amortizationDeps`), as counted. The guard shape is as proposed: `viewControllers` beside `controllerSurfaces`, without the reach guard; one new guard, `TestGuard_NoViewStateHoldsItsDeps`; all mutation-verified. Six lines of existing tests changed. Two tests were added for the entry's closures, because a swapped width and height passed every old test. The cost is what the count predicted, so the stop condition does not apply. Each later item should also test its entry's closures.
- [x] **VL-404 — Prices**
  - 23 of 28. Deps: services. Inputs: styles, keys, width, height, `currentView`. Calls: 20 production, 68 tests (the most test churn of the seven). Stays on `App`: the three key handlers (the price dialogs, the bulk refresh), `afterPriceChange`, `applyPriceRefreshResult`.
  - Done: `*App` 420 → 398. 22 methods moved onto `priceViewState`, `formatPriceRow` became a plain function (the VL-403 rule), and `priceDeps` arrived. The 68 test call sites changed; the comparer found no assertion changed, except two conditions in which only the method's path changed. Two adapter tests: the render fits the screen in both modes, and a wheel scroll off the Prices view with price data loaded schedules nothing (the old wheel test had no price data, so it could not catch a wrong `onScreen`).
- [ ] **VL-405 — Portfolio**
  - 12 of 13. Deps: services and `valuationOptions`. Inputs: styles, height. Calls: 22 production, 30 tests. Stays: `handlePortfolioKeys` (sidebar, stock-split dialog).
- [ ] **VL-406 — Dashboard**
  - 11 of 13. Deps: services and `valuationOptions`. Inputs: styles. Calls: 14 production, 33 tests. Stays: `handleDashboardKeys`, `setDashboardAccountExpanded` (both move the sidebar cursor). Reports' two pinned renders call `renderAssetLiabilityColumns`; after the move they call it through `a.dashboard`.
- [ ] **VL-407 — Corporate Actions**
  - 8 of 10. Deps: services. Inputs: styles, width, height, the ticker filter. Calls: 12 production, 11 tests. Stays: `handleCorporateActionViewKeys` (writes the filter), `confirmDeleteCorporateAction` (confirm dialog).
- [ ] **VL-408 — Reports**
  - 6 of 8. Deps: services. Inputs: styles, keys. Calls: 5 production, 19 tests. Stays: `renderReports` and `renderNetWorthReport`, which read Dashboard state (an open question in the design's §8).
- [ ] **VL-409 — Investment register**
  - 14 of 18. Deps: services and `valuationOptions`. Inputs: styles, height. Calls: 26 production, 49 tests. Last, because it is the most coupled view: 113 reads of its state in 19 files, many from the investment dialogs. Stays: the key handler, the search-key handler, the table build (it consumes `pendingInvestmentSelectID`), the status toggle (undo).
