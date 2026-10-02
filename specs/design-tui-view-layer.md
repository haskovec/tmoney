# Design sketch: TUI view layer — one view table, and the other half of `App`

**Date:** 2026-09-14
**Status:** PHASES 0, 1, 2 AND 3 BUILT; phase 4 is measured and planned (VL-401, VL-402), and its pilot (Amortization, VL-403) is built. Its other items are not committed. Phase 0 shipped as W1 (PR #46). Phase 1 (the view table), phase 2 (the per-view state structs) and phase 3 (the two file splits) are built; see their status notes below.

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
| `App` fields | 91 | ~68 (first written as ~57, a miscount; built: 70 — see the phase 2 status) |
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
| Prices | `priceView`, `priceTable`, `priceListTable`, `priceListClicks` |
| Corporate actions | `corporateActionView`, `corporateActionViewTable`, `corporateActionDetail`, `corporateActionViewFilter`, `corporateActionViewFilterEditing` |
| Amortization | `amortizationData`, `amortizationTable` |
| Shared chrome that looks like view state | `refreshingPrices`, `refreshNotifID` — the bulk price refresh's in-flight guard (`refresh_prices.go`), started by `u` on **both** the Securities and Prices views |
| Navigation | `currentView`, `previousView` |

The shape is the one the earlier design found for dialogs: a `*xData` pointer,
one or two `*widget.Table`s, and a scatter of scalars. The scalars are the
cost. `investmentFilterQuery` and `investmentFilterLockedSec` are only
meaningful while `currentView == ViewInvestmentRegister`, and `switchView`
has a special case to clear them on the way out (`app_menu.go:300`). Nothing
says they belong to that view except their prefix.

Not every prefixed field is private to its view, and the table says so where
it matters. `corporateActionViewFilter` is written by the Securities view (a
ticker drill-in, `security_view.go:371`) and by the menu (`app_menu.go:136`)
before the Corporate Actions load reads it. The two refresh fields are written
from two views. Phase 2 treats both as cross-surface handoffs, not as one
view's state.

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

// allViews is the one list. Package-level, filled with method values exactly
// as modals() fills onKey — (*App).renderDashboard is a func(*App) string —
// so it allocates nothing and needs no App to read. views() exists so the
// guards and the lookup have one accessor to call.
var allViews = []viewEntry{ … }

func views() []viewEntry { return allViews }

// viewFor looks an entry up by id, never by slice index: View(999) must miss
// cleanly, because View.String() is pinned to return "Unknown" for it.
func viewFor(v View) (viewEntry, bool)
```

Then each of the seven switches becomes a lookup and one call, with the
existing fallbacks kept:

```go
func (v View) String() string {
	if e, ok := viewFor(v); ok && e.name != "" {
		return e.name
	}
	return "Unknown" // TestViewString pins this for View(999)
}

func (a *App) renderContent(height int) string {
	viewContent := "Unknown view"
	e, ok := viewFor(a.currentView)
	if ok {
		viewContent = e.render(a)
	}
	if e.fullScreen || a.styles.SidebarWidth() == 0 { … }
	…
}
```

and `handleKeyPress`'s view switch, `getKeyHints`, `viewShortcutSections`,
`activeTable`, `reloadCurrentView` and `switchView`'s focus block read the
same entry. The five feature switches (§1.2) stay as they are; they are not
lists of views, they are lists of *which* views have a feature, and a boolean
per entry for each would be the generic component framework the earlier
design refused.

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
}
```

and `a.priceView` becomes `a.prices.data`, `a.priceTable` becomes
`a.prices.table`, and so on for the eleven views. About 34 fields leave
`App`, and the eleven structs that hold them are fields too, so `App` sheds a
net ~23.
What stays on `App` is what more than one surface writes: `currentView` and
`previousView`; the two register `pending*SelectID` handoffs; the bulk-refresh
flag and its notification id, which two views start; and the corporate-action
ticker filter, which the Securities view and the menu write (the phase 2
notes take each in turn). The two `switchView` departure special cases become
the view's own `leave()` hook on its entry, which is the honest home for "what
to forget on the way out."

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

Add `corporateActionShortcuts()` and the eleventh arm in
`viewShortcutSections`. The section must match what the view binds and what
its status-bar hint already shows (`app_view.go:174`: "↑↓ navigate  / filter
enter details  d delete  esc back"): `/` filter, Enter for details, **`d`
delete (reverse the action, after a confirm — the destructive key is the one
that most needs to be discoverable)**, Esc, and `g`/`G`/PgUp/PgDn for parity
with the amortization section even though Navigation lists them too. About
fifteen lines plus a test that renders the help overlay for every `View`
value and asserts a view-specific section is present — the test that would
have caught this, written generically so it keeps catching it.

**This is a bug fix and ships alone**, ahead of the design work, under the
rule that bug fixes go first.

### Phase 1 — the view table

Declare `viewEntry`, `allViews`, `views()` and `viewFor()` in a new
`views.go`. Move the body of each arm of the seven switches into its entry,
verbatim: `render` is the existing `renderX`, `onKey` the existing
`handleXKeys`, and so on — no method moves, only the switch collapses.
`View.String()` and `renderContent` keep their `"Unknown"` fallbacks (§2.1).

**Only the switches move; the earlier branches in `handleKeyPress` stay
exactly where they are.** They are per-view, but they are not the list, and
each is load-bearing:

- `app.go:447`: while the investment register's filter is being typed, every
  key goes to that view, so digits do not switch views mid-query. (W12 added
  the same guard for the Corporate Actions filter, next to it, and an Esc
  exception for its details panel.)
- `app.go:495`: on the Reconciliation view only Quit and Help are global;
  everything else, including `1`–`5` and Esc, goes to the view, so a
  reconciliation in progress cannot be abandoned by a stray key.
- `app.go:548`: Esc on the Prices detail mode, or with a locked investment
  filter, is claimed by the view instead of `switchView(previousView)`.

`fullScreen` replaces the five-view list in both predicates, and **only that
list**. Both keep their other half. `renderContent` is
`e.fullScreen || a.styles.SidebarWidth() == 0`, as §2.1 shows. `handleMouseContent`
is the same disjunct, and *inside* it the Dashboard branch stays:

```go
if e.fullScreen || sidebarWidth == 0 {
	if a.currentView == ViewDashboard {
		return a.handleMouseDashboard(m, contentY, 0)
	}
	return a.handleMouseTable(msg, contentY)
}
```

Dropping the `sidebarWidth == 0` half would send a narrow-terminal Dashboard
click to the `m.X == sidebarWidth` border check with `sidebarWidth` zero, and
discard it.

Two facts get recorded rather than changed:

- `reload` is nil for Reconciliation and CorporateActions, because
  `reloadCurrentView` has no arm for them today. Whether that is a gap (a
  save from a dialog opened over the reconciliation view does not refresh it)
  is filed as a question for the owner of those views; phase 1 preserves the
  behaviour.
- `focus` differs for the two views whose tables are built when data
  arrives, and the two are **not** the same. Amortization's arm
  (`app_menu.go:366`) only unfocuses the sidebar; `buildAmortizationTable`
  focuses the table on load. Corporate Actions' arm (`app_menu.go:353`)
  unfocuses the sidebar **and** focuses the table when it is non-nil, so
  returning to an already-loaded history lands the cursor on it. Each entry
  copies its own arm verbatim; an implementer who levels the two changes
  back-navigation.

**Guards, in `views_guard_test.go`, in this order:**

1. **Every `View` constant has exactly one entry.** Both sides mechanical: the
   constants from `go/ast` over `app.go`'s const block (reflection cannot
   enumerate constants), the entries by ranging over `views()`, which needs
   no `App`. Fail on an empty constant set, so the parser going stale is
   loud, and fail on a duplicate `id`.
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

**Status: built** (2026-09-28). The seven switches and `View.String()`'s
switch are lookups in `views.go`, and both full-screen predicates read
`fullScreen`. On `main` before this phase, guard 2 fires on all eight
switches and guard 3 on both predicates. Differences from the text above:

- `init` fills `allViews`, not its declaration. The key handlers reach
  `View.String()`, which reads `allViews`, and Go refuses that cycle in a
  package-level initializer. The table is still built once, and `views()`
  and `viewFor()` take no receiver.
- Every entry has a `reload`. W2 (`specs/work-two-ledgers.md`) shipped first
  and gave Reconciliation and Corporate Actions their reload arms, so the
  `reload: nil` text in §2.1, §2.3, this section and §5.4 is out of date.
  A test checks that no entry lacks a func that its lookup calls without a
  nil check (`render`, `onKey`, `hints`, `shortcuts`, `reload`, `focus`).
- `table` is nil for Dashboard and Reports, which have no table.
- `hints` are closures over one constant, `commonKeyHints`, which is also
  the default for a value that is not a view.
- The View menu holds only Theme, so the smoke test reached the views by
  their keys, the sidebar and the view keys. It ran in a pty with fictional
  data: every view, `?` on each, clicks and the wheel, and Securities to
  Corporate Actions and back. Its 40 screens, text and colours, were
  identical to the same run on the build before this phase.

Found and left alone, because this phase changes no behaviour: the Prices
detail hint never shows. `updateStatusBar` runs only on a view switch, a
database switch and start-up, and entering the detail mode is none of
these, so the status bar keeps the list hint. W13 in
`specs/work-two-ledgers.md` fixes it (PR #62): `Update` refreshes the hints
after every message.

### Phase 2 — per-view state structs

One struct per view as §2.2 sketches, eleven of them, in the view's own file.
`App` goes from 91 fields to ~68: ~34 leave, and the eleven new structs are
fields too. (This text first said ~57, which left the eleven out.) This is the
earlier design's phase 3 and
carries the same cost: the 335 test lines that set a view field in an `App`
literal move under the new struct, across roughly thirty test files. The
literal shape changes; no assertion does.

Five decisions to make in the phase, recorded here so they are not made by
accident (VL-305 in the plan added a sixth, the last bullet):

- **The `pending*SelectID` trio.** `pendingRegisterSelectID` and
  `pendingInvestmentSelectID` are written by dialog save paths (`afterTransferSave`
  sets one or the other) and read by the register loaders. They are
  cross-surface handoffs, like the sticky date, and stay on `App`.
  `pendingSecuritySelectID` is written and read only by the securities view
  and moves into its struct.
- **The corporate-action ticker filter is a handoff too.**
  `corporateActionViewFilter` is set by the Securities view's drill-in and
  cleared by the menu before the history loads; `loadCorporateActionViewData`
  documents that callers may pre-populate it. It stays on `App` with the
  pending IDs. `corporateActionViewFilterEditing` is the view's own typing
  mode and moves into its struct. **The filter is not cleared on `leave()`**:
  `switchView` today drops `corporateActionDetail` and ends a filter entry
  (`corporateActionViewFilterEditing = false`) on the way out, and the filter
  itself survives a round trip on purpose. `leave()` must do both of those
  things and nothing more.
- **The bulk-refresh flag stays on `App`.** `refreshingPrices` and
  `refreshNotifID` are the in-flight guard for a refresh that `u` starts from
  the Securities view as well as the Prices view. Nesting them in the Prices
  struct would have one view's key handler writing another view's state.
- **`switchView`'s two departure special cases** (drop the investment filter;
  drop the corporate-action detail and end its filter entry) become a
  `leave func(*App)` on the two
  entries. That puts "what to forget on the way out" beside "what to focus on
  the way in," and removes the last per-view `if` from `switchView`.
- **The modal surfaces declared in view files stay separate fields.**
  `priceSurface`, `priceImportDialog`, `investmentTypeSelector` and
  `security` are in `modals()`; guard 2 of the earlier design walks `App` for
  them by type, and they must stay directly on `App` for it to see them.
- **The investment edit and preselect IDs stay on `App`.**
  `investmentEditTxnID` and `investmentNewTxnSecurityID` are set by the type
  selector and read or cleared by the investment dialogs and their save
  paths, so they are cross-surface handoffs like the pending IDs. Only the
  register's data, table and three filter fields move into its struct.

The phase-3 guards of the earlier design (`TestGuard_NoSurfaceStructHoldsAService`
and the nil-safety guard) are keyed to `Modal`; view structs do not implement
it. Phase 2 adds one guard of its own: **no view struct holds a service
pointer** — the same `switchDatabase` use-after-close argument, reusing
`servicePointerTypes()`. It must find the view structs without a hand list,
which would go stale the first time a twelfth view landed: walk `App`'s
fields for struct types declared in this package whose pointer does **not**
implement `Modal` (the modal guard takes the ones that do), and fail if that
set is empty, as the modal guard does.

**Status: built** (2026-10-02), in PRs #65 to #76 (#71 trimmed two comments).
Each of the eleven views holds its state in one struct, which is one field on
`App`, and `switchView` names no view: the investment register and Corporate
Actions forget their state through `leave` on their entries. The VL-313 checks:

- `App` has 70 fields, not ~57. The ~57 took the ~34 fields that move away
  from 91 but did not add back the eleven structs that hold them:
  91 − 34 + 11 = 68. The sixth decision keeps two more on `App`, so
  91 − 32 + 11 = 70.
- No assertion changed. A throwaway `go/ast` comparer read every test function
  before the phase (`3ca99c4`) and after it, took the condition of each `if`
  that calls `t.Error` or `t.Fatal`, mapped the new field paths back to the old
  names, and compared them function by function: 1,596 functions and 4,523
  assertions, none changed or removed. It reports a condition planted to
  differ. The phase added five test functions: the guard and its self-test,
  a test of the dashboard's render-then-click order, and one for each `leave`.
- The six decisions hold. The fields they keep on `App` are there, and no
  `leave` clears the corporate-action ticker filter.
- `TestGuard_NoViewStateHoldsAService` finds all eleven view structs by rule.

Differences from the text above:

- The sixth decision (in the list above): the investment edit and preselect
  IDs stay on `App`.
- The guard walks down through pointers, containers and this package's
  structs, not only a struct's own fields: after the move, each view's data
  struct is one level below `App`.
- Names. `prices` and `securities` sit beside their modal surfaces `price` and
  `security`; the other nine fields are the view's name, so where the old data
  field had that name, the data is now one level in (`a.dashboard.data`).
- Twelve tests of the nil or loading state still set `data: nil` explicitly,
  as they set the old field to `nil` before. The zero value is the same; the
  explicit nil names the state under test.
- The test churn is larger than §7 said. The 335 lines were the `App`
  literals; the field paths in test code and the indent of grouped literals
  were not counted.

### Phase 3 — split the two god files (the 4d motion)

Independent of phases 1–2 and may run first, under the 4d rule: move
functions between files, rename nothing, change no signature.

`price_view.go` (1,231) → `price_view.go` (data, load, apply; ~250),
`price_view_render.go` (~350), `price_view_keys.go` (~200),
**`price_dialog.go`** (the add/edit dialog and lookup; ~200) and
**`price_import_dialog.go`** (~100). The five `*App` chart methods (~130
lines) go into the **existing** `price_chart.go`, which today holds the
chart's free helpers and history cache (383 lines, no `*App` methods), so the
chart has one file rather than two that the comparer cannot tell apart. The
two dialog files are the point: they are modal surfaces and belong beside the
other dialogs, not inside a view.

`investment_register_view.go` (1,055) → four files, because the register
proper is ~650 lines and would miss the exit gate as three:
`investment_register_view.go` (data, load, keys, status toggle; ~400),
`investment_register_render.go` (table build and render; ~330),
`investment_register_filter.go` (the security filter and its search-key
handler; ~190), and
`investment_type_selector.go` (the dialog and its type helpers; ~210).

Verified the way 4d was: a `go/ast` comparison of every declaration, doc
comment included, against the original, and the splitter's orphan-comment
report. Zero test lines.

**Status: built** (2026-09-28). `price_view.go` is 261 lines and
`investment_register_view.go` 398. Three differences from the plan as first
written: the chart's debounce delay and its two messages moved into
`price_chart.go` with the five methods, so the chart has one file in full;
`price_chart.go` gained one import (bubbletea); and the four table-build
helpers (`investmentRegisterColumns`, `shouldShowInvestmentBalance`,
`buildInvestmentRegisterTable`, `formatInvestmentRegisterRow`) moved into
`investment_register_render.go` with the render functions. The first plan
kept table build in the view file; there the helpers would add about 127
lines and put `investment_register_view.go` over the 500-line gate. The file
list above now shows the split as built. The comparer found all 1,120
declarations byte-identical after every step.

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

#### Measured (VL-401, 2026-10-02): 112 of 154 view methods could move

A throwaway `go/ast` tool applied the 4c rule to every `*App` method in each
view's files. The dialog files beside a view (`price_dialog.go`,
`investment_type_selector.go` and the like) are surfaces, so they are left
out. §1.5 counted 164 methods before phase 3 moved the price dialogs and the
type selector into those files.

A method **could move** onto the view's state struct when both of these hold:

- Every `App` field it names is the view's own state, `styles`, `services`
  (reached through a deps struct, as in the transfer pilot), or a value that it
  only reads and that the caller can pass in: the key bindings, the screen
  size, `currentView`, the config, or a handoff such as the ticker filter.
- Every `App` method it calls could move too and belongs to the same view, or
  is a helper that touches only those things and would become a free function.

A method is **pinned** when it uses a chrome object (the status bar, the
sidebar, the undo manager), names a modal surface or another view's state,
writes a value that the view does not own, or calls a pinned method. The tool
repeats until nothing changes. Five of the movable methods were read by hand
to check that the rule is not too generous.

| View | Methods | Could move | Pinned | What pins them |
|---|---|---|---|---|
| Dashboard | 13 | 11 | 2 | the key handler and the expand toggle move the sidebar cursor |
| Register | 10 | 4 | 6 | the key handler and the edit flow open the transfer dialog; clear, void and delete use the status bar, the sidebar and undo; the table build consumes `pendingRegisterSelectID` |
| Investment register | 18 | 14 | 4 | the key handler (status bar, sidebar, undo, and it clears Portfolio's data); the status toggle (undo); the table build and the search-key handler consume `pendingInvestmentSelectID` |
| Portfolio | 13 | 12 | 1 | the key handler (sidebar, and it opens the stock-split dialog) |
| Scheduled | 12 | 6 | 6 | the key handler (sidebar); skip and delete (undo); the three result handlers (status bar, undo) |
| Reports | 8 | 6 | 2 | both net-worth renders call Dashboard's `renderAssetLiabilityColumns`, which reads Dashboard state |
| Reconciliation | 23 | 14 | 9 | finish and the two after-hooks (status bar, sidebar, undo); the key handler, through finish; the Start Reconciliation dialog's four methods (a surface, and its sticky date); the refusal for investment accounts |
| Securities | 14 | 9 | 5 | the key handler opens four dialogs and writes the corporate-action filter; the security dialog's key, action and submit; the after-change note (status bar) |
| Prices | 28 | 23 | 5 | the key handlers open the two price dialogs and start the bulk refresh; the after-change note and the refresh result (status bar, refresh flag, `a.err`) |
| Corporate Actions | 10 | 8 | 2 | the key handler writes the ticker filter; delete opens the confirm dialog |
| Amortization | 5 | 5 | 0 | — |
| **All** | **154** | **112** | **42** | |

What the table shows:

- **§1.5 said that the view methods are "mostly pinned". They are not.** The
  pins collect in the key handlers and in the after-save and result handlers,
  which touch the chrome. The render, table-build, row-format, selection and
  load methods name only their own view's state.
- **22 of the 112 need a value passed in** that the view does not own. Most
  need the screen size or the key bindings. The three valuation loaders need
  `valuationOptions`, which reads the config. Two Corporate Actions methods
  read the ticker filter, and two methods read `currentView`.
- **Three of the 112 are dialog saves** that live in view files
  (`createSecurity`, `updateSecurity`, `startReconciliation`). They would move
  onto their dialog's surface, not onto the view.
- **The cost of a move is the 4c cost.** The table entries that call a moved
  method become closures that pass the inputs, for example
  `render: func(a *App) string { return a.prices.render(a.styles, a.width, a.height) }`.
  Each view whose movable methods reach a service needs one deps binding on
  `App`, as `transferDeps` is: up to eleven new `*App` methods.
- **The predictions above were wrong in both directions for Reports.** Its key
  handler is not pinned: the global Esc arm in `handleKeyPress` returns to the
  previous view before the view's handler runs, so the handler never calls
  `switchView`. The same is true for Amortization. But Reports' render is
  pinned, which the prediction missed. The Dashboard's render writes
  `accountRows`, which is the Dashboard's own state, so it is not a pin.
- **A coupling the design did not record.** Reports' net-worth render shows the
  Dashboard's expand state: expand an account on the Dashboard, and the
  Reports view shows it expanded too. That may be intended, and no test says
  otherwise, so it is left alone. A move of Reports' render needs a decision
  on it first.
- **Read 112 as the most that could move, not as a forecast.** The earlier
  design's 4c found coupling that a count over one file does not see: the
  transfer pilot moved two methods that its count did not predict, and the
  create-category step moved none of its own. Expect the same here.

So a full phase 4 could take at most about 112 methods off `*App`, which has
424 today, less up to eleven deps bindings: about 424 → 323. That is the
number the "Methods on `*App`" row declined to predict. VL-402 decides, view
by view, whether the move is worth its cost.

#### Decided (VL-402, 2026-10-02): seven views, a pilot first, and no reach guard

**The rule: a view is worth the move when at least two thirds of its methods
could move.** The point of the move is that a view's behaviour sits beside its
state. When a third or more must stay on `App`, the move splits the view
between two homes, which is harder to read than one.

| View | Could move | Decision |
|---|---|---|
| Amortization | 5 of 5 | **VL-403, the pilot** |
| Prices | 23 of 28 | VL-404 |
| Portfolio | 12 of 13 | VL-405 |
| Dashboard | 11 of 13 | VL-406 |
| Corporate Actions | 8 of 10 | VL-407 |
| Reports | 6 of 8 | VL-408 |
| Investment register | 14 of 18 | VL-409, last: the most coupled view |
| Securities | 9 of 14 | not opened: below two thirds, and three of the nine are the security dialog's saves; revisit with that dialog's own 4c |
| Reconciliation | 14 of 23 | not opened: below two thirds |
| Scheduled | 6 of 12 | not opened: below two thirds |
| Register | 4 of 10 | not opened: below two thirds |

The seven items move 79 methods at most, against 7 deps bindings: `*App`
would go from 424 to about 352.

**A pilot first.** The earlier design moved one surface before it priced the
rest, and the pilot found costs that the count did not show. Amortization is
the smallest view, and nothing in it is pinned, so it shows the fixed cost of a
view move with the least else in the way. VL-403 records that cost, and the
plan says to stop and re-decide before VL-404 if it is much higher than the
count suggests.

**The guard shape: a table for views, without the reach guard.** A 4c row
carries three structural guards: no method on the surface names `App`, every
dep is a live func, and no production code reads the surface's state through
`App`. The first two fit a view. The third does not. By this design's own
rules, the message arms in `app_update.go` stay where they are, and the pinned
methods (most of them key handlers) stay on `App`. Both read the view's state:
the reach guard would forbid 275 reads in the seven views, from 7 for
Amortization to 113 for the investment register. Every one would need an accessor method on
the view state, which adds methods and moves none. So the proposal is a table
for views, beside `controllerSurfaces`, with the first two guards only. VL-403
settles it.

**Cost, measured per item.** Every one of the seven needs a deps struct,
because each view's movable methods include a load that calls a service. Three
of them (Portfolio, Dashboard, the investment register) also need
`valuationOptions` in their deps, because it reads the config. The plan items
give each view's inputs and the call sites that change.

#### Built (VL-403, the pilot, 2026-10-02): Amortization, and the guard shape settled

`*App` 424 → **420 methods (−4)**. Five methods left (`loadAmortizationData`,
`buildAmortizationTable`, `amortizationStatsLine`, `renderAmortizationView`,
`handleAmortizationKeys`), and one arrived (`amortizationDeps`). That is the
VL-401 count exactly. **Five methods hang off `*amortizationViewState`**, and
none names `App`: `load`, `buildTable`, `statsLine`, `render`, `handleKey`.
Each takes what it needs as parameters: the deps, the styles, the key
bindings, the screen size. `load` does not use its receiver; it is a method so
that the view's behaviour sits beside its state.

What `App` still does for the view:

| Where | What | Why |
|---|---|---|
| `amortizationDeps` | binds the two services | it reads `App`'s services |
| the view table entry | `render`, `onKey` and `reload` are closures that pass `App`'s values in | the table's funcs take `*App` |
| `app_update.go` | the `amortizationLoadedMsg` arm stores the data and calls `buildTable` | the message arms stay where they are |
| `register_view.go` | the `a` drill-in calls `load` | it is the register's key handler |

**The guard shape is as VL-402 proposed.** `viewControllers` is a table beside
`controllerSurfaces`, with the same row type. The shared guards run over both:
no method on the view state names `App`; every dep is a func and the binding
fills it; the deps follow a database switch; and the tables match `App` (a
`xDeps` struct now needs an `xSurface` or an `xViewState` row). The reach
guard runs over the surfaces only. Even this view, with nothing pinned, is
read by code that stays on `App`: the message arm writes `data`, and the
entry's `table` and `reload` funcs read it. One guard is new:
`TestGuard_NoViewStateHoldsItsDeps`, over every view state. The natural way to
clear a view is to assign its zero value, and a stored deps struct would then
come back with nil funcs.

**Each guard was mutation-verified.** A method on the view state that takes
`*App`, the view's row deleted, a dep the binding leaves nil, a binding that
captures a service pointer, and a deps field in the view state each fail the
guard for that rule. The nil dep fails both deps guards, because the second
one calls it.

**The cost.** Production code +80/−63 (net +17), comments +27/−15 (net +12).
Six lines of existing tests changed: the five call sites and one failure
message. The assertion comparer found no existing assertion changed except the
table guard's own check, which now accepts a view row. Two tests were added,
because the moved code now depends on closures that pass `App`'s values in,
and no old test covered them: `TestAmortizationView_KeysReachTheTable` (no test
had sent a key to this view) and `TestAmortizationView_RenderFitsTheScreen`
(the render tests check text, so a closure with the width and height swapped
passed all of them). The render tests now go through the view table entry, as
the app does.

**What the pilot found that the count did not:**

- **The adapter closures are new code with a new failure mode.** Two `int`
  parameters in the wrong order compile and pass the old tests. Each later item
  should test its entry's closures, as this pilot does.
- **The reach guard cannot apply to a view at all,** not even to one with
  nothing pinned, because the message arm and the entry read the state.
- **A view state could store its deps,** which no guard checked. One does now.

**Verdict for the items after this one:** the cost is what the count
predicted (−4 for five methods moved), plus a fixed cost of about two adapter
tests per view. That is not "much higher than the count suggests", so the
plan's stop condition does not apply. VL-404 can go ahead when it is wanted.

**One rule for where a moved function goes** (from the review of #80, so that
the seven views do not mix two styles):

- A function that reads or writes the view state, or that takes the view's
  deps, is a **method on the view state**. A function that takes the deps is
  one of the view's commands (a load or a fetch), and it belongs beside the
  view's state even when it does not read it. That is why `load` is a method
  although it does not use its receiver.
- A pure helper that takes neither the state nor the deps, such as a row
  formatter, is a **plain function**. `formatAmortizationRow` was one before
  the move and stays one.
- A method that takes the deps takes them **first**, before the message, the
  key bindings or the styles (from the review of #81).

#### Built (VL-404, 2026-10-02): Prices, the largest view

`*App` 420 → **398 methods (−22)**, as counted. 22 methods hang off
`*priceViewState`, and none names `App`. `formatPriceRow` became a plain
function under the rule above: it never used its receiver. `priceDeps` arrived
to bind the security and price services. The name has to be `priceDeps`, not
`pricesDeps`: the table guard finds the view state from the deps name
(`price` + `ViewState`), and it failed the build's tests until the view's row
was in `viewControllers`, which is the guard doing its job.

**What stayed on `App`:** the three key handlers (they open the price dialogs
and start the bulk refresh), `afterPriceChange` and `applyPriceRefreshResult`
(the status bar, the refresh flag, `a.err`). They call the moved methods with
`a.priceDeps()`, `a.keys` and the screen size.

**The adapter tests.** Only parameters of the same type can be swapped
without a compile error, so two tests cover them. The render fits the screen in
both modes (with the width and height swapped, it draws 160 lines on a 30-line
screen). And the chart fetch takes `onScreen`, which replaced a read of
`currentView`: the old wheel test, which scrolls the wheel off the Prices view
and expects no command, now loads a price list. Without it, the fetch returned
nothing whatever the flag said, so the test could not catch a wrong
`onScreen`. The view's data stays loaded after a view switch, so the case is
real. (The review of #81 merged a first, separate test into the old one.)

**The cost.** Production code +194/−177 (net +17), comments +49/−38 (net +11).
68 test call sites changed. The assertion comparer found no assertion changed,
except two conditions in which only the method's path changed
(`a.prices.listCursorSecurityID()`). Each planted mistake (a swapped size, a
constant `onScreen`, a view-state method that takes `*App`, a captured service
pointer) failed its test.

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
Phase 1 adds a test that exercises exactly those guards: build
`&App{sidebar: NewSidebar(), statusbar: widget.NewStatusBar(), styles: widget.NewStyles()}`
with every table nil, and for each `View` value set `currentView` to some
*other* view and call `switchView(v)`. A zero `App` will not do —
`switchView` calls `updateStatusBar`, which dereferences the status bar, and
the whole focus block sits behind `if a.sidebar != nil`, so on a zero `App`
the test would panic before, or skip, the code it exists to check. This is
not a twin of the earlier design's nil-safety guard (which calls `IsVisible`
on a nil pointer); it is a walk over the live focus funcs with the tables
absent, so a lost nil check is a test failure rather than a panic on first
use of a fresh database.

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
as unobservable. `views()` is consulted on every key, paint and mouse event,
and the entries hold nothing but funcs and constants, so §2.1's package-level
`allViews` is the form: filled once with method values, read through
`views()` and `viewFor()`, never indexed by `View` directly (§2.1 says why:
`View(999)` must miss, not panic). If an entry ever needs per-`App` state it
is doing phase 4's job in phase 1's clothes; the guard is that `views()` and
`viewFor()` take no receiver.

---

## 6. Exit criteria

| Phase | Exit criteria |
|---|---|
| 0 | `?` on the Corporate Actions view lists its keys; a test renders the overlay for every `View` value and requires a view-specific section |
| 1 | Seven switches gone, the five-view list gone from both predicates (their `SidebarWidth() == 0` halves and the Dashboard mouse branch kept), `View.String()` reads the table and still returns `"Unknown"` for a miss; the pre-switch `handleKeyPress` branches untouched; guards 1–3 land with self-tests; the tables-nil `switchView` walk (§5.2) passes for every view; **manual smoke: visit every view from the View menu, press `?`, click a table row, scroll, and drill from Securities into Corporate Actions and back** |
| 2 | `App` at ~70 fields (91, less the fields that move, plus the eleven structs; first written as "under ~60", a miscount); each view's state is one field; `switchView` has no per-view `if`; the six recorded decisions applied as written; the no-service guard discovers the view structs without a hand list; the test literals moved and no assertion changed |
| 3 | `price_view.go` ≤ 450 and `investment_register_view.go` ≤ 500, each split by the declaration comparer with zero problems and zero orphaned comments; the two price dialogs in files of their own; the chart's `*App` methods in `price_chart.go` |
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
| 2 | +130 (eleven struct declarations with doc); built: net +81 in 39 files | ~335 literal lines churn, ~30 files; built: +1,833/−1,510 in 42 files | mechanical; the estimate counted only `App` literals, not field paths in test code |
| 3 | 0 net | 0 | file moves, comparer-verified |

The earlier design's §7 recorded that its +100 estimate landed at +432,
because a decomposition is mostly writing down why each boundary is a
boundary. Expect the same here, and expect the comments to be worth it: the
view table is the document a future reader opens to learn what a view is.

---

## 8. Deferred, with a decision

- **View controllers (phase 4).** Measured and planned (VL-401, VL-402):
  seven items, a pilot first, none committed.
- **Reports shows the Dashboard's expand state.** Reports' net-worth render
  calls the Dashboard's `renderAssetLiabilityColumns`, which reads the
  Dashboard's holdings and expanded accounts, so an account expanded on the
  Dashboard shows expanded in Reports too. A product question: should the two
  views share that state? Until it is answered, the two Reports renders stay
  on `App` (VL-408).
- **Reload for Reconciliation and CorporateActions** (§5.4). A product
  question, filed with those views. Phase 1 preserves today's behaviour.
- **`load*` → `*LoadedMsg` → `build*Table` as a registry entry.** Eleven arms
  in `app_update.go` follow one shape (`a.x = msg.data; a.buildXTable()`).
  They could be a `apply func(*App, tea.Msg)` on the entry. Not done here,
  because the earlier design's phase 4 measured those arms as pinned and the
  gain is eleven arms against one more func per entry; revisit if a twelfth
  view is added.
- **Packages under `tui/`.** Same decision as before, same §3.
