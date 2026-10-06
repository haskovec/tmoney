# Design sketch: TUI sub-packages — the chart and the Sidebar, and why not the rest

**Date:** 2026-10-05
**Status:** BUILT. SP-1 (the chart) and SP-2 (the Sidebar) are built; the rest stays in `package tui` (§1.2).

**Re-opens:** the "Why not packages" sections of
`specs/design-tui-decomposition.md` (§3) and `specs/design-tui-view-layer.md`
(§3). Both said the same thing: packages are the result of decomposition, not
the method, so re-open the question after phase 4, with a surface's own
numbers. Phase 4 of the view layer is built (VL-409). This file holds those
numbers, the decisions made from them, and the two items that follow.

---

## Goal

**Boundaries that the compiler enforces.** A sub-package of `tui` cannot
import `tui`, because `tui` imports it. So the code in it cannot reach `App`,
and `tui` cannot reach its unexported names. That is the gain, and the only
one this design counts. Fewer files in `internal/tui/` is a side effect, not a
goal.

So a candidate is worth a package only when the package hides something:
unexported state or helpers that `tui` then cannot touch.

## Non-goals

- **No new shared package for `errMsg`, `keyMap` or the format helpers.**
  §1.2 says why each one blocks the views and the wizards, and that is a
  reason to leave those where they are, not to start there.
- **No `package tui_test`.** The tests of a moved package move into it as
  internal tests. The `tui` tests stay in `package tui`.

---

## 1. The measurement (2026-10-05, main at `2c4a638`)

Package `tui` had 85 production files, 28,254 lines; 7,331 of those lines are
in 26 files that declare no `App` method. An `App`-free file is not a
candidate by that fact alone. Two counts decide:

- **Needs:** the `tui` names the group uses. Each must move down with the
  group, or move to a lower package first.
- **Inbound:** the uses, from code outside the group, of the names the group
  declares (its package-level names, and the fields and methods of its
  types). Each unexported one must be exported, or the code that uses it must
  change.

The inbound counts come from the type checker, over the production and the
test files of `package tui` together. The needs come from a scan of the
names in the group's declarations, checked by hand (a local closure in the
wizard has the name of a `tui` function). For a view, "outside" means code
that is not a method of the view's state, because the view's files still
hold `App` methods that stay.

### 1.1 The two candidates

| Group | Needs | Inbound, production | Inbound, tests | Unexported crossings |
|---|---|---|---|---|
| Chart rendering (part of `price_chart.go`) | 0 | 2 calls in `price_view_render.go` | 20 uses of the threshold in `price_view_test.go` | 3 names, all made exported: `Panel`, `ShouldShow`, `MinContentWidth` |
| Sidebar (`sidebar.go`) | 0 | 84 uses in 18 files | 612 | production: the cursor item and its kind and account, 2 sites; tests: the cursor field, 12 lines; the item's ID, 1 line |

### 1.2 The rest

| Group | Needs | Inbound, production | Inbound, tests |
|---|---|---|---|
| `keyMap` (`app_keymap.go`) | 0 | 117 in 17 files | 373 |
| Paycheck wizard, its `App`-free files | 4 | 67 in 5 files | 536 |
| Split dialog, its `App`-free files | 5 | 147 in 8 files | 742 |
| The seven phase-4 view states | 7 to 13 each | 12 to 119 per view | 12 to 180 per view |

- **`keyMap`** needs nothing and could move. But its fields are exported
  already (`Enter`, `Escape`, `Up` and the rest). Only the type and
  `defaultKeyMap` are not, and `tui` must call `defaultKeyMap`. A package
  would hide only the two helpers that build the undo and redo bindings, and
  nothing else calls them now. The compiler would enforce almost nothing
  new.
- **The paycheck wizard and the split dialog.** Their `App`-free files need
  four and five `tui` names. Three are shared helpers that many other files
  use (`formatDashboardMoney`, `parseAmountInput`, `prefillField`). The rest
  tie them to other surfaces: the split dialog needs the wizard's
  `PaycheckSection` and the scheduled dialog's `pendingSplitScheduled`. Their
  `App` files (`*_app.go`) stay and drive them. The rest of `tui` uses their
  names 67 and 147 times in production, and the tests 536 and 742 times. A
  package would hide state that the `App` files must still read.
- **The view states.** Every one of the seven needs `errMsg`, four need
  `keyMap`, and most need `formatDashboardMoney` or `valuationOptionsFor`.
  The rest of `tui` reads their state from 12 (Amortization) to 119 (the
  investment register) places in production: the view table entries, the
  message arms, and the methods that stayed on `App`.
- **`errMsg`** is the block under the views. It is the only error channel
  out of a `tea.Cmd`, with 229 production uses in 44 files (the decomposition
  design counted 246 in 38 files, before the 4c work). A package that loads
  data must construct it, and it cannot import it from `tui`. To move it down
  first is 229 edits before any view gains a boundary.

**Recorded decision:** these stay in `package tui`. Re-open one only for a
new reason (a new user of the code, or a change that removes a block), with
its own numbers, as both earlier §3 sections said.

---

## 2. Decisions (the interview, 2026-10-05)

1. **The goal is enforced boundaries**, not fewer files (above).
2. **The chart first, as a pilot, then the Sidebar.** The chart is the
   cheapest candidate and settles the shape: the names, the test placement,
   and the stop rule.
3. **The history cache stays with Prices.** Its only user is the Prices view
   state, and the chart package does not fetch or cache. The package stays
   pure rendering.
4. **`pricechart` exports `Panel`, `ShouldShow` and `MinContentWidth`**, and
   nothing else.
5. **A moved package's tests move into it as internal tests**, so they keep
   their reach into its unexported helpers.
6. **The stop rule, for each item.** If the item needs an export that this
   file does not list, or a production file other than the ones listed must
   change, stop and decide again. A surprise in the API is the signal that
   the boundary is in the wrong place.
7. **The Sidebar is `package sidebar`**, with `sidebar.Sidebar` and
   `sidebar.New()`. The 21 local variables named `sidebar` that hold a
   Sidebar, in 6 test files, become `sb`, so that no local hides the package
   name. (The interview first counted 25; that grep also matched two log
   messages, an `App` field, and a rendered string in `app_view.go`. That
   file will not import the package, so its local keeps its name.)
8. **The Sidebar's accessors.** `CursorAccount() *account.Account` replaces
   `CursorItem()`, and `Cursor() int` reads the cursor for the tests. The
   item type, its kind and its constants stay unexported.
9. **A test helper that both packages need is copied, not exported.**
   `testAccount` (now two lines around `account.NewAccount`) and `mkPrice`
   each exist once in each package. Exporting them would put test-only code
   in production.
10. **The rest stays** (§1.2).

---

## 3. Items

### SP-1 — the chart panel into `internal/tui/pricechart` (BUILT, 2026-10-05)

- [x] Move the chart rendering out of `price_chart.go` into
  `internal/tui/pricechart/pricechart.go`. Rename the three names that cross
  the boundary: `buildChartPanel` → `Panel`, `shouldShowChartPanel` →
  `ShouldShow`, `chartPanelMinContentWidth` → `MinContentWidth`.
- [x] Keep the history cache and the debounced fetch in `price_chart.go`.
- [x] Move the 23 chart tests into `package pricechart`; keep the 7 cache
  tests in `tui`.

**As built.** `price_chart.go` went from 490 to 194 lines. The new file is
313 lines: the 290 lines that moved, the package comment and the imports. The only other
production file that changed is `price_view_render.go`: two calls and a
comment, and one import. No fourth export was needed, so the stop rule did
not fire.

**Verification.** The moved code and the moved tests diff clean against main
after the three renames, and after the test names that follow them
(`TestShouldShowChartPanel` → `TestShouldShow`, `TestBuildChartPanel_*` →
`TestPanel_*`). The cache tests and the code that stayed diff clean with no
rename. One line was dropped on purpose: `var _ = time.Time{}`, which kept
an otherwise unused import alive in the old test file.

Two planted mistakes:

- **A threshold of 121** failed `TestShouldShow`. No Prices view test failed:
  they render at widths far from the threshold, as they did before the move.
  The unit test is what pins the exact value, and it moved with the code.
- **`Panel` returning ""** failed five `Panel` tests in `pricechart` and five
  Prices render tests in `tui`, so the call from `tui` is under test.

**What the boundary enforces.** Nothing in `tui` called the chart's helpers
(`clampYRange`, `composeChartBox` and the rest) before the move. Now the
compiler says so, and it will refuse the first call.

### SP-2 — the Sidebar into `internal/tui/sidebar` (BUILT, 2026-10-05)

- [x] Move `sidebar.go` to `internal/tui/sidebar/sidebar.go`. Rename
  `NewSidebar` → `New`. Add `CursorAccount()` and `Cursor()`, and delete
  `CursorItem()`. `CursorAccount()` returns the account under the cursor, or
  nil on a group header or an empty list. Account items always hold their
  account, because the Sidebar builds them from the account list, so nil
  covers both of today's checks.
- [x] Move `sidebar_test.go` (32 tests, none of them uses `App`) into the
  package. Its 30 `NewSidebar()` calls become `New()`.
- [x] In `tui`, production: `app.go` (the field's type and its constructor),
  `app_sidebar.go` (the mouse handler's group-header check) and
  `dashboard_view.go` (the expand toggle's account check). No other
  production file may change (the stop rule).
- [x] In `tui`, tests: the 271 `NewSidebar()` calls in 40 files become
  `sidebar.New()`; the 21 locals named `sidebar` become `sb`; the 8 reads of
  the cursor use `Cursor()` and the 4 writes use the existing `SetCursor`;
  the one `CursorItem().accountID` becomes `CursorAccount().ID`; and
  `testAccount` (12 calls in 3 files) gets its `tui` copy in
  `app_sidebar_test.go`.

**What the boundary enforces.** The cursor, the item list, the scroll offset
and the grouping are the Sidebar's own. Before SP-2, the tests set the cursor
field directly in four places, and two production sites read the item's kind.
Now the only way in is the Sidebar's methods.

**As built.** `CursorItem` became the unexported `cursorItem`, because
`Select` and the rebuild still use it inside the package. The three
production files changed by 7 lines added and 12 removed; no other
production file changed, and no export beyond `New`, `CursorAccount` and
`Cursor` was needed, so the stop rule did not fire. A type-checked rename
changed the 21 locals (68 identifiers) and left the `sidebar:` keys of the
`App` literals alone. The helper `testAccount` now calls
`account.NewAccount`, which sets the same fields.

One count was short. The plan named one `CursorItem()` use in the tests;
there were two more, in `TestApp_Dashboard_MouseClickBeforeRenderIsNoOp`,
which compared the item pointer before and after a click. It now compares
`Cursor()`. That is the test's intent (the click must not move the cursor),
and `Cursor` was already planned, so this did not change the API.

**Verification.** The moved code and tests diff clean against main after the
renames, apart from the planned additions: the package comment, the two
accessors, and one new test, `TestSidebar_CursorAccount`. The 32 tests that
left `tui` are the 32 in the new package, under the same names except two
(`TestNewSidebar` → `TestNew`, `TestSidebar_CursorItem` →
`TestSidebar_cursorItem`). In `tui`, six assertions changed, and only their
path: four reads of the cursor and two reads of the item under it.
Four planted mistakes:

- **`CursorAccount` always nil** failed five Dashboard expand tests, the
  sidebar double-click test, and `TestSidebar_CursorAccount`: both production
  call sites are under test.
- **`Cursor` always 0** failed two mouse tests in `tui` and
  `TestSidebar_CursorAccount`.
- **The expand toggle reading `SelectedAccount`** (the committed account, not
  the one under the cursor) failed four Dashboard tests.
- **The mouse handler with no group-header check** failed nothing. The check
  repeats one in `Select`, which returns false on a group header, so a
  double-click on a header opens nothing either way. The two checks it
  replaced were redundant in the same way before the move. The move kept it,
  because a move does not change behaviour. All it still did was keep header
  clicks out of the double-click tracker, so account, header, account opened
  the account. A follow-up (2026-10-05) removed it: every click now goes to
  the tracker, and two tests pin the header's double click and the broken
  one.
