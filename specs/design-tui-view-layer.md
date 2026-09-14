# Design sketch: TUI view layer — one view table, and the other half of `App`

**Date:** 2026-09-14
**Status:** PROPOSED — nothing built. Phase 0 is a bug fix and may ship alone.

**Addresses:** `specs/code-quality-review.md` item 4, slice **4b** as
`specs/design-tui-decomposition.md` defined it: the view-layer god files
(`price_view.go`, `investment_register_view.go`) and the 11-arm focus switch
in `switchView`. That design closed 4a (the modal registry), 4c for five
surfaces, and 4d for five files, and deferred this to a second design because
"mixing the two produces one change nobody can review." This is the second
design.

Read §1 for the measurement, then §2 for the shape, then the phases. §3
records the one prescription this design rejects so nobody re-derives it.

---

## Goal

Make the TUI's view layer one concept instead of seven copies of a list.
Concretely, and measured:

1. The set of views is declared **once**. Today it is written out seven times,
   in five files, by hand (§1.2), plus a five-entry "full-screen views" subset
   written twice more.
2. Every view has a help section. One does not today (§1.3).
3. Each view owns its state in **one struct**. Today 41 of `App`'s 91 fields
   are loose view state (§1.1).
4. The two god files are split along their real seams (§1.4). This is the 4d
   motion again and is independent of the rest.
5. The review's prescription to make views "packages under `tui/`" is
   rejected in writing, by reference to the earlier design's §3.

Target numbers at the end of phase 3:

| Measure | Now | Target |
|---|---|---|
| `App` fields | 91 | ~55 |
| Methods on `*App` | 416 | **~416 — unchanged, and stated honestly** — see below |
| Copies of the view list | 7 full + 2 subsets | 1 |
| Views with no help section | 1 | 0 |
| `price_view.go` lines | 1,231 | ≤ 450, in ~5 files |
| `investment_register_view.go` lines | 1,055 | ≤ 500, in ~3 files |
| New packages | — | 0 |

**The method row is flat on purpose.** The earlier design's phases 0–4 also
did not reduce `App`'s methods; its phase 5 and the 4c work after it did, by
moving `open`/`submit`/`close` onto surfaces whose only outside contact was a
service and the create-category divert. Views are not that shape. A view's key
handler opens dialogs (sibling surfaces), moves the sidebar, posts to the
status bar, and calls `switchView`. §1.5 measures it: of the 164 `*App`
methods in the eleven view files, the ones that could leave `App` under the 4c
rule are a minority, and the number is the thing phases 1–3 make cheap to find
out. So this design collapses the lists and the fields, and prices the
controller step (§4, phase 4) without committing to it.

## Non-goals

- **No new packages.** Same decision as the earlier design's §3, same
  measurement: 75 test files reach `App`'s unexported fields; `currentView`
  alone appears in 378 struct literals.
- **No bubbletea sub-models.** A view must not implement `tea.Model`. In
  bubbletea v2 `Update` returns the *root* model, so returning a view from any
  `Update` path replaces the application. Views keep returning `string` for
  `App` to composite with the sidebar and chrome.
- **No change to key bindings, layout, or which views are full-screen.** The
  registry records those facts; it does not revise them.
- **No move of the message arms.** `registerLoadedMsg`, `priceViewDataLoadedMsg`
  and the other nine view-data arms stay in `app_update.go`. The earlier
  design's phase 4 measured why: those arms return `load*` commands, touch the
  status bar, or coordinate two surfaces, and moving them adds `App` methods.
- **No touching the dialogs that live in view files.** `priceSurface`, the
  price import dialog, and the investment type selector are modal surfaces in
  `modals()` already. They stay where 4a put them; phase 3 only moves their
  code into their own files.
- **`currentView` stays a field on `App`.** 378 test literals set it, and it
  is the discriminator the registry is indexed by. Wrapping it would be churn
  with no invariant behind it.

---

## 1. The problem, measured

Eleven views: Dashboard, Register, Scheduled, Reports, Reconciliation,
Securities, Prices, InvestmentRegister, Portfolio, CorporateActions,
Amortization (`app.go:25-46`).

### 1.1 Nearly half of `App`'s remaining fields are view state

After the modal work, `App` has 91 fields. **41 are loose view state**, grouped
here by the view that owns them and nowhere in the code:

| View | Fields on `App` |
|---|---|
| Dashboard | `dashboard`, `dashboardExpandedAccounts`, `dashboardAccountRows` |
| Register | `register`, `table`, `pendingRegisterSelectID` |
| Investment register | `investmentRegister`, `investmentTable`, `investmentEditTxnID`, `investmentFilterSearching`, `investmentFilterQuery`, `investmentFilterLockedSec`, `investmentNewTxnSecurityID`, `pendingInvestmentSelectID` |
| Portfolio | `portfolioData`, `portfolioHoldingsTable`, `portfolioLotsTable`, `portfolioMode` |
| Scheduled | `scheduled`, `scheduledTable` |
| Reports | `reports` |
| Reconciliation | `reconciliation`, `reconciliationTable` |
| Securities | `securityView`, `securityTable`, `pendingSecuritySelectID` |
| Prices | `priceView`, `priceTable`, `priceListTable`, `priceListClicks`, `refreshingPrices`, `refreshNotifID` |
| Corporate actions | `corporateActionView`, `corporateActionViewTable`, `corporateActionDetail` |
| Amortization | `amortizationData`, `amortizationTable` |
| Navigation | `currentView`, `previousView` |

The shape is the one the earlier design found for dialogs: a `*xData` pointer,
one or two `*widget.Table`s, and a scatter of scalars. The scalars are the
cost. `investmentFilterQuery` and `investmentFilterLockedSec` are only
meaningful while `currentView == ViewInvestmentRegister`, and `switchView`
has a special case to clear them on the way out (`app_menu.go:300`). Nothing
says they belong to that view except their prefix.

### 1.2 The same list is written seven times, and a subset twice more

**98 `case ViewX` arms across 12 switches in 6 files.** Seven of the switches
enumerate the views:

| Function | File | Arms | What each arm does |
|---|---|---|---|
| `handleKeyPress` | `app.go:568` | 11 | `return a.handleXKeys(msg)` |
| `renderContent` | `app_view.go:96` | 11 | `viewContent = a.renderX()` |
| `getKeyHints` | `app_view.go:151` | 11 | a string |
| `switchView` focus | `app_menu.go:316` | 11 | unfocus sidebar, focus the view's table |
| `viewShortcutSections` | `help_overlay.go:231` | **10** | append the view's help section |
| `activeTable` | `app_helpers.go:67` | 9 | `return a.xTable` |
| `reloadCurrentView` | `app_helpers.go:176` | 9 | append `a.loadXData(...)` |

The other five are feature switches with two to four arms (which views show a
closed-positions toggle, which have a header row above the table, which are a
register, which refresh after a corporate action, which react to an account
closing). Those are legitimate per-feature logic and stay switches.

Then there is the subset. Five views render full-screen with no sidebar:
Reconciliation, Securities, Prices, CorporateActions, Amortization. That list
is written as a five-way `||` in **two** places — `renderContent`
(`app_view.go:125`) and `handleMouseContent` (`app_mouse.go:90`) — and if the
two ever disagree, clicks land in the wrong pane. Nothing ties them together.

The `View.String()` switch (`app.go:51`) is an eighth list; it becomes the
registry's `name` field.

### 1.3 One view has no help section — a live gap

`viewShortcutSections` has ten arms for eleven views. **`ViewCorporateActions`
is missing** (`help_overlay.go` contains no reference to it at all). That view
binds `/` to search, Enter to open a detail overlay, `g`/`G`/PgUp/PgDn to move,
and Esc to close or clear (`corporate_action_history.go:291-340`). Press `?`
there and the overlay lists only the global and navigation sections; the
view's own keys are undiscoverable.

This is the exact failure mode the earlier design's §1.3 found for the modal
lists, one layer up: the view was added to six of the seven lists. Phase 0 is
the fix; phase 1's guard is what stops it recurring.

### 1.4 The two god files, and what is in them

| File | Lines | `*App` methods | Other funcs |
|---|---|---|---|
| `price_view.go` | 1,231 | 40 | 6 |
| `investment_register_view.go` | 1,055 | 21 | 7 |
| `security_view.go` | 651 | 14 | 4 |
| `register_view.go` | 615 | 10 | 1 |
| `portfolio_view.go` | 588 | 13 | 2 |
| `dashboard_view.go` | 586 | 10 | 1 |
| `reconciliation_view.go` | 568 | 20 | 1 |
| `scheduled_view.go` | 482 | 12 | 0 |
| `corporate_action_history.go` | 422 | 11 | 4 |
| `reports_view.go` | 386 | 8 | 1 |
| `amortization_view.go` | 320 | 5 | 3 |

`price_view.go` is three things in one file. Its 40 `*App` methods cluster as:

| Cluster | Methods | Count |
|---|---|---|
| load / apply | `loadPriceViewData`, `reloadPriceViewKeepingMode`, `loadPriceViewDataForSecurity`, `applyPriceViewData`, `evictSelectedSecurityFromHistoryCache`, `afterPriceChange`, `applyPriceRefreshResult` | 7 |
| tables / render | `buildPriceListTable`, `buildPriceTable`, `formatPriceRow`, `selectedPrice`, `renderPriceView`, `renderPriceList`, `composePriceListBody`, `buildPriceListChartPanel`, `resolveListPriceSecurity`, `listCursorSecurityID`, `renderPriceDetail` | 11 |
| chart fetch | `schedulePriceChartFetch`, `schedulePriceListChartFetchIfActive`, `fetchPriceChartHistory`, `handlePriceChartDebounceTick`, `applyPriceChartHistory` | 5 |
| keys | `handlePriceViewKeys`, `handlePriceListKeys`, `handlePriceDetailKeys`, `drillIntoSelectedListRow`, `handlePriceSearchKey` | 5 |
| **price add/edit dialog** | `handlePriceDialogKey`, `priceDialogAction`, `startPriceLookup`, `lookupPriceCmd`, `handlePriceLookupResult`, `submitPriceDialog`, `createPrice`, `updatePrice` | 8 |
| **price import dialog** | `handlePriceImportDialogKey`, `priceImportDialogAction`, `submitImportPriceDialog`, `importPrices` | 4 |

Twelve of the forty are two **dialogs** — modal surfaces that happen to be
declared in the view's file. The view proper is 28 methods and, with its data
struct and free helpers, about 800 lines. Splitting the dialogs out is the
same compiler-proven file move 4d used, and it does not wait on anything else.

`investment_register_view.go` is two things: the register (load, table,
render, keys, status toggle) and the **type selector** — a small dialog that
picks which investment dialog to open (`openInvestmentTypeSelector`,
`handleInvestmentTypeSelectorKey`, `investmentTypeSelectorAction`,
`dispatchInvestmentTypeSelection`), plus a security filter with its own search
mode. The filter's state is the five `investmentFilter*` /
`investmentNewTxnSecurityID` fields in §1.1.

### 1.5 Why a view is not a controller surface

The earlier design moved `open`/`submit`/`close` off `App` for five dialog
surfaces and got `*App` from 425 to 416. The precondition was that a surface's
only outside contacts were services (reached through closures) and the
create-category divert. Views do not meet it, and the reason is what a view
*is*:

- A key in the register opens the transaction dialog, the transfer dialog, the
  split editor, or a confirm; those are sibling surfaces.
- `handleDashboardKeys` and `handleMouseDashboard` move the sidebar cursor.
- Every load path ends in `a.statusbar` or `a.err`.
- Drilling into a security or an account calls `switchView`, which is the
  chrome's operation.

So the 164 `*App` methods in view files are mostly pinned by what they touch,
in the same way the earlier design's §"Goal" found 53 of 89 message arms
pinned. What *can* move is measurable — the render and table-building methods
read only the view's own data and styles — and phase 4 prices it rather than
guessing. The honest projection for phases 0–3 is that `*App`'s method count
does not move.

---

## 2. The shape

### 2.1 One table

The earlier design's §2 gave the modal layer one interface and one ordered
slice. Views need less: they are never stacked and never nil in the way a
lazily-built dialog is, so there is no interface to declare and no typed-nil
trap to defuse. What they need is **one entry per view** that holds every
per-view fact the seven switches currently hold:

```go
// viewEntry is one view and the glue App supplies for it. Every per-view fact
// the code needs lives here, so adding a view is adding one entry — and the
// guard in views_guard_test.go fails if the entry is missing.
type viewEntry struct {
	id   View
	name string // View.String(); shown in the status bar and test failures

	// fullScreen views paint without the sidebar and take the whole width.
	// Both renderContent and handleMouseContent read this one flag; the two
	// hand-written five-way predicates it replaces had to agree by luck.
	fullScreen bool

	render   func(*App) string
	onKey    func(*App, tea.KeyPressMsg) (tea.Model, tea.Cmd)
	hints    func(*App) string
	shortcuts func() shortcutSection // the view's help section

	// table is the widget the mouse and the wheel address in this view, or
	// nil. A func rather than a field because Prices and Portfolio pick
	// between two tables by mode.
	table func(*App) *widget.Table

	// focus is what switchView does on arrival: which pane takes the cursor.
	// Most entries are "sidebar off, table on"; Dashboard is the reverse.
	focus func(*App)

	// reload returns the commands that refresh this view in place, or nil for
	// a view that reloads through its own path (Reconciliation, Corporate
	// Actions today — see the phase 1 note).
	reload func(*App) []tea.Cmd
}

func (a *App) views() []viewEntry
func (a *App) view(v View) viewEntry
```

Then each of the seven switches becomes one line:

```go
func (a *App) renderContent(height int) string {
	e := a.view(a.currentView)
	viewContent := e.render(a)
	if e.fullScreen || a.styles.SidebarWidth() == 0 { … }
	…
}
```

and `handleKeyPress`, `getKeyHints`, `viewShortcutSections`, `activeTable`,
`reloadCurrentView` and `switchView`'s focus block read the same entry. The
five feature switches (§1.2) stay as they are; they are not lists of views,
they are lists of *which* views have a feature, and a boolean per entry for
each would be the generic component framework the earlier design refused.

**The funcs take `*App`, exactly as `modalEntry.onKey` does.** That is not a
retreat from the controller idea; it is §1.5. Whether a given view's `render`
can later become a method on a view type that does not name `App` is phase
4's question, and the table makes it a one-line change per view when the
answer is yes.

### 2.2 One state struct per view

Phase 2 is the earlier design's phase 3, applied to the 41 fields in §1.1:

```go
// priceViewState is everything the Prices view owns. Its zero value is the
// view before its first load. The price add/edit dialog and the import dialog
// are NOT here: they are modal surfaces, registered in modals(), and a view's
// state must not hold a sibling surface.
type priceViewState struct {
	data      *priceViewData
	table     *widget.Table // detail mode
	listTable *widget.Table // list mode
	clicks    *widget.ClickTracker
	refreshing    bool
	refreshNotifID int
}
```

and `a.priceView` becomes `a.prices.data`, `a.priceTable` becomes
`a.prices.table`, and so on for the eleven views. `App` sheds ~36 fields
(`currentView` and `previousView` stay; the three `pending*SelectID` fields
are discussed in the phase 2 notes). The `switchView` special case for the
investment filter becomes the view's own `leave()` hook on its entry, which
is the honest home for "what to forget on the way out."

### 2.3 What the table cannot do

- It cannot make a view's key handler stop opening dialogs; that is what the
  handler is for. So it does not shrink `*App`.
- It cannot enforce that a view's `render` is pure. `renderDashboard` writes
  `dashboardAccountRows` for the mouse to read (§5.1 of the earlier design,
  and §5.1 here). The table records the coupling; it does not remove it.
- It cannot decide reload semantics for the two views that have none
  (Reconciliation, CorporateActions). Phase 1 records the current behaviour
  as `reload: nil` and files the question, rather than inventing one.

---

## 3. Why not packages, and why not sub-models

Both are rejected for the reasons the earlier design's §3 and "Non-goals"
gave, and the numbers are larger here, not smaller. The view files hold 164
`*App` methods that call into every dialog surface; a `tui/prices` package
would import `tui` for the dialogs and `tui` would import it for the view, so
the split is a cycle before it is a boundary. The test coupling is the same
2,000-odd `app.<unexported>` references, of which 378 are `currentView`
alone. And `tea.Model` sub-models replace the root on return in bubbletea v2.
None of this is re-derived here; it is stated so the next reader does not
start from the review's prescription.

---

## 4. Phases

Ordered so that each phase is one reviewable change and no phase depends on a
later one. Phase 3 depends on nothing and may go first.

### Phase 0 — give the Corporate Actions view a help section

Add `corporateActionShortcuts()` listing `/` search, Enter for details, Esc,
`g`/`G`, PgUp/PgDn, and the eleventh arm in `viewShortcutSections`. Ten
lines plus a test that renders the help overlay for every `View` value and
asserts a view-specific section is present — the test that would have caught
this, written generically so it keeps catching it.

**This is a bug fix and ships alone**, ahead of the design work, under the
rule that bug fixes go first.

### Phase 1 — the view table

Declare `viewEntry` and `views()` in a new `views.go`. Move the body of each
arm of the seven switches into its entry, verbatim: `render` is the existing
`renderX`, `onKey` the existing `handleXKeys`, and so on — no method moves,
only the switch collapses. `fullScreen` replaces both five-way predicates.
`View.String()` reads `name`.

Two facts get recorded rather than changed:

- `reload` is nil for Reconciliation and CorporateActions, because
  `reloadCurrentView` has no arm for them today. Whether that is a gap (a
  save from a dialog opened over the reconciliation view does not refresh it)
  is filed as a question for the owner of those views; phase 1 preserves the
  behaviour.
- `focus` for Amortization and CorporateActions does not focus a table,
  because their tables are built when data arrives and focused then. The
  entries say so in a comment, as `switchView` does now.

**Guards, in `views_guard_test.go`, in this order:**

1. **Every `View` constant has exactly one entry.** Both sides mechanical: the
   constants from `go/ast` over `app.go`'s const block (reflection cannot
   enumerate untyped constants), the entries from calling `views()` on a zero
   `App`. Fail on an empty constant set, so the parser going stale is loud.
2. **No switch outside `views.go` enumerates the views.** Parse every
   production file; a `switch` whose cases name **more than four** `View`
   constants is a second list and fails. Four is the largest feature switch
   today (`refreshAfterCorporateAction`), and the guard states the number so
   raising it is a visible decision.
3. **No expression compares `currentView` against three or more `View`
   constants.** This is the full-screen predicate, and it must not come back
   in a third place.
4. **Self-tests** for 2 and 3 over fabricated source, in the
   `controller_guard_test.go` pattern.

### Phase 2 — per-view state structs

One struct per view as §2.2 sketches, eleven of them, in the view's own file.
`App` goes from 91 fields to ~55. This is the earlier design's phase 3 and
carries the same cost: the 335 test lines that set a view field in an `App`
literal move under the new struct, across roughly thirty test files. The
literal shape changes; no assertion does.

Three decisions to make in the phase, recorded here so they are not made by
accident:

- **The `pending*SelectID` trio.** `pendingRegisterSelectID` and
  `pendingInvestmentSelectID` are written by dialog save paths (`afterTransferSave`
  sets one or the other) and read by the register loaders. They are
  cross-surface handoffs, like the sticky date, and stay on `App`.
  `pendingSecuritySelectID` is written and read only by the securities view
  and moves into its struct.
- **`switchView`'s two departure special cases** (drop the investment filter,
  drop the corporate-action detail) become a `leave func(*App)` on the two
  entries. That puts "what to forget on the way out" beside "what to focus on
  the way in," and removes the last per-view `if` from `switchView`.
- **The modal surfaces declared in view files stay separate fields.**
  `priceSurface`, `priceImportDialog`, `investmentTypeSelector` and
  `security` are in `modals()`; guard 2 of the earlier design walks `App` for
  them by type, and they must stay directly on `App` for it to see them.

The phase-3 guards of the earlier design (`TestGuard_NoSurfaceStructHoldsAService`
and the nil-safety guard) are keyed to `Modal`; view structs do not implement
it. Phase 2 adds one guard of its own: **no view struct holds a service
pointer** — the same `switchDatabase` use-after-close argument, reusing
`servicePointerTypes()`.

### Phase 3 — split the two god files (the 4d motion)

Independent of phases 1–2 and may run first, under the 4d rule: move
functions between files, rename nothing, change no signature.

`price_view.go` (1,231) → `price_view.go` (data, load, apply; ~250),
`price_view_render.go` (~350), `price_view_chart.go` (~150),
`price_view_keys.go` (~200), **`price_dialog.go`** (the add/edit dialog and
lookup; ~200) and **`price_import_dialog.go`** (~100). The two dialog files
are the point: they are modal surfaces and belong beside the other dialogs,
not inside a view.

`investment_register_view.go` (1,055) → `investment_register_view.go`
(~500), `investment_register_filter.go` (the security filter; ~250),
`investment_type_selector.go` (the dialog; ~200).

Verified the way 4d was: a `go/ast` comparison of every declaration, doc
comment included, against the original, and the splitter's orphan-comment
report. Zero test lines.

### Phase 4 — view controllers, priced and not committed

After phases 1–2 every view has one entry and one state struct, and the
question "which of this view's methods name only its own state, styles and
services?" is a grep over one file. The earlier design's 4c motion then
applies per view, with the same deps-closure shape and the same guard table
(`controllerSurfaces` gains a row, or a sibling table for views). The
candidates, by what §1.5 says they touch:

| View | Likely to move | Likely pinned |
|---|---|---|
| Reports | render, load (reads its own filters) | key handler (`switchView` on Esc) |
| Amortization | render, table build | key handler (Esc → previous view) |
| Prices | render, table build, chart fetch | keys (open dialogs), refresh (status bar) |
| Register | render, table build | keys (opens five dialogs), load (sidebar) |
| Dashboard | very little | render writes mouse state; keys move the sidebar |

**This phase is a table of per-view decisions, not a commitment.** Its result
is the number the "Methods on `*App`" row above declines to predict. It is
listed so that phases 1–3 are built in the shape that makes it cheap.

---

## 5. Risks the phases must handle

### 5.1 Rendering writes state that mouse hit-testing reads

`renderDashboard` fills `dashboardAccountRows` (`dashboard_view.go:236`) and
`handleMouseDashboard` reads it (`app_mouse.go:127`). The paycheck wizard has
the same shape (`hitZones`), and the earlier design's §5.1 recorded the rule:
the render must run before the click is interpreted, and the state must be
cleared when the view's data is replaced. Phase 2 moves the map into the
dashboard's struct; phase 1's `render` entry must keep calling the same
function so the write still precedes the read. A test that clicks a dashboard
row without rendering first must get a no-op, not a stale account.

### 5.2 `focus` arms touch tables that may be nil

`switchView` guards each table with `if a.xTable != nil`, because the table
is built when data arrives. The entry's `focus` func must keep every guard.
Phase 1 adds a test that calls `switchView(v)` for every `View` on a zero
`App` — the view-layer twin of the earlier design's guard 1 — so a lost nil
check is a test failure, not a panic on first use of a fresh database.

### 5.3 `activeTable` depends on view *mode*, not only view

Prices returns the list table or the detail table by `priceView.mode`;
Portfolio returns holdings or lots by `portfolioMode`. That is why `table` is
a func on the entry and not a field. The phase-1 move must carry the mode
check into the func, and a test must exercise both modes for both views.

### 5.4 Two views have no reload; do not invent one

Reconciliation and CorporateActions are absent from `reloadCurrentView`.
Phase 1 records `reload: nil` and preserves that. Whether a dialog saved over
those views should refresh them is a product question, filed with the view's
owner; a refactor that quietly starts reloading them changes what the user
sees after a save.

### 5.5 The test literals

378 `currentView:` literals do not change (the field stays). The 335 view-field
literal lines change in phase 2 only, mechanically, by the same perl-and-
compile motion the services collapse used. Phase 1 and phase 3 change zero
test lines; that is part of why they are ordered first.

### 5.6 `views()` must not allocate per key

`modals()` builds a fresh slice per call and the earlier design measured that
as unobservable. `views()` is called on every key, paint and mouse event, and
the entries hold nothing but funcs and constants, so a package-level `var`
indexed by `View` is the natural form. If an entry ever needs per-`App` state
it is doing phase 4's job in phase 1's clothes; the guard is that `views()`
takes no receiver.

---

## 6. Exit criteria

| Phase | Exit criteria |
|---|---|
| 0 | `?` on the Corporate Actions view lists its keys; a test renders the overlay for every `View` value and requires a view-specific section |
| 1 | Seven switches gone, two predicates gone, `View.String()` reads the table; guards 1–3 land with self-tests; the nil-`App` `switchView` walk passes for every view; **manual smoke: visit every view from the View menu, press `?`, click a table row, scroll** |
| 2 | `App` under ~60 fields; each view's state is one field; `switchView` has no per-view `if`; the no-service guard runs over the view structs; the 335 test literals moved and no assertion changed |
| 3 | `price_view.go` ≤ 450 and `investment_register_view.go` ≤ 500, each split by the declaration comparer with zero problems and zero orphaned comments; the two price dialogs in files of their own |
| 4 | Not an exit; a table of per-view decisions with the measured count of what moved and what stayed, appended to this document as the 4c notes were to the earlier one |

**What these criteria do not claim.** No phase reduces `*App`'s method count.
No phase changes what any key does, which views are full-screen, or when a
view reloads. Phase 4, if taken, is where the count moves, and it will be
recorded per view rather than promised here.

---

## 7. Cost

| Phase | Production lines | Test lines | Note |
|---|---|---|---|
| 0 | +15 | +40 | bug fix; ships alone |
| 1 | −120 net (7 switches → 1 table + 11 entries), +100 comments | +250 | guards and self-tests |
| 2 | +130 (eleven struct declarations with doc) | ~335 literal lines churn, ~30 files | mechanical |
| 3 | 0 net | 0 | file moves, comparer-verified |

The earlier design's §7 recorded that its +100 estimate landed at +432,
because a decomposition is mostly writing down why each boundary is a
boundary. Expect the same here, and expect the comments to be worth it: the
view table is the document a future reader opens to learn what a view is.

---

## 8. Deferred, with a decision

- **View controllers (phase 4).** Priced, not committed. Re-open per view
  after phase 2, from the measurement, not from this document.
- **Reload for Reconciliation and CorporateActions** (§5.4). A product
  question, filed with those views. Phase 1 preserves today's behaviour.
- **`load*` → `*LoadedMsg` → `build*Table` as a registry entry.** Eleven arms
  in `app_update.go` follow one shape (`a.x = msg.data; a.buildXTable()`).
  They could be a `apply func(*App, tea.Msg)` on the entry. Not done here,
  because the earlier design's phase 4 measured those arms as pinned and the
  gain is eleven arms against one more func per entry; revisit if a twelfth
  view is added.
- **Packages under `tui/`.** Same decision as before, same §3.
