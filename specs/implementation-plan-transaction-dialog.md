# Implementation Plan: Transaction Dialog Improvements

This document defines the order in which three UX improvements to the new-transaction dialog are landed: a sticky last-used date, a masked overwrite-style date control, and a typeahead category selector with inline category creation. Each item is one PR following a red-green (test-first) pattern. Mark items as complete with `[x]` as they are finished.

Specs:
- `specs/tui.md` — TUI interface reference; Phase 6 updates the New Transaction dialog and date-field sections.
- `specs/categories.md` — category model (parent/child hierarchy, income/expense classification); the create-category sub-dialog is constrained to this model.
- `README.md` — keyboard reference and Transactions section; Phase 6 updates.

## Status Legend

- `[ ]` Not started
- `[x]` Complete

## Goal

Three independent improvements to the new-transaction dialog (`internal/tui/transaction_dialog.go`):

1. **Sticky last-used date** — within a session, the next dialog open seeds the date field with the date from the last *saved* transaction (not Cancel). On launch, defaults to today. Process-lifetime state only — no config-file persistence.

2. **Masked overwrite-style date control** — replace the free-text `MM/DD/YYYY` input with a fixed-width masked widget. Cursor only lands on digit positions; slashes are skipped. Typing a digit overwrites the digit at the cursor and advances. Backspace replaces with `0` and steps back. The current digit renders with a block highlight. Once the widget exists, swap it into every other date field in the TUI for consistency (transfer, scheduled, investment, corporate-action, account, reconciliation, price).

3. **Category combo-box with inline create** — replace the arrow-only category dropdown with a type-to-filter combo box that expands a scrollable list while focused. Filtered results rank prefix matches first, then substring, alphabetical within each group. The last row of the filtered list is `[+ Add new category…]`, which opens a small create-category sub-dialog (Name + Parent + Income/Expense). Parent is itself a combo box that accepts an existing parent or a new top-level name. The new category is saved immediately, auto-selected back in the transaction dialog, and focus advances to Amount. If the user typed a query in the search box, that query pre-fills the Name field; if it contains `:`, it is split into `Parent:Child`.

## Priority Rationale

The plan is ordered to:

1. Land sticky-date first (Phase 1) — single-PR, no new widgets, smallest blast radius. Delivers the most-immediately-felt productivity win for batch entry.
2. Build the masked-date widget and ship it in the new-transaction dialog only (Phase 2) before retrofitting elsewhere. This validates the widget against the dialog the user actually grilled, and isolates regressions to one call site if anything breaks.
3. Build the category combo-box widget and apply it to the new-transaction dialog (Phase 3) before adding the create-category branch. The combo widget is independently useful even without the `[+ Add new]` row, and tests for filtering/ranking are simpler in isolation.
4. Add the inline create-category sub-dialog (Phase 4) on top of the combo widget. This is the most complex phase — it spans two dialogs, persists DB state, and coordinates focus.
5. Roll out the masked-date widget to the remaining 9 dialogs (Phase 5). Pure mechanical migration once the widget is proven; ordered roughly by dialog complexity. `price_view.go` uses `YYYY-MM-DD` rather than `MM/DD/YYYY`, so it lands last and motivates a small extension to the widget.
6. Phase 6 updates `specs/tui.md` and `README.md` once, against the final shape.

## Per-item Shape

Every item follows the same four-step pattern:

1. **RED** — write tests first. For new widget types, tests live in `internal/tui/dialog_test.go` and exercise the field directly (construct a `Field`, drive it through `HandleKey`/method calls, assert on `Value`, `cursorPos`, `SelectedIndex`, etc.). For dialog-level changes, tests live alongside the dialog (`transaction_dialog_test.go`, etc.) and exercise behavior end-to-end (build dialog, simulate keypresses, assert on emitted messages or persisted state). Existing tests must not be broken; replacement tests for swapped-out behavior land in the same PR.
2. **GREEN** — implement the smallest change that makes the new tests pass. Cite the file and line numbers under the item.
3. **CLEANUP** — delete code obviated by the change. For migration items in Phase 5, this is the legacy `AddTextField` call site. For widget-introduction items, this may be nothing. Build and full test suite green.
4. **DOCS** — note doc impact in the item (most items defer to Phase 6, which writes the docs in a single sweep).

Phase-prep items (no RED) cover non-behavior-changing scaffolding: introducing struct fields, helper functions, or empty file stubs that subsequent items fill in. Tests are not required for prep items; build green is.

---

## Phase 1: Sticky last-used date

The new-transaction dialog currently re-seeds the date to today on every open (`transaction_dialog.go:122` — `time.Now().Format("01/02/2006")`). After this phase, the dialog seeds with the date of the last successfully-saved transaction in the current process; on the very first open of the session (and on Cancel), it falls back to today.

- [x] **TD-001 — Last-used date persists across new-transaction dialog opens**
  - RED: tests in `internal/tui/transaction_dialog_test.go` covering:
    1. First open: date field shows today.
    2. Submit a transaction with date `01/15/2024`; reopen the dialog; date field shows `01/15/2024`.
    3. Open the dialog, change the date in-place to `02/01/2024`, press Esc (cancel); reopen; date field still shows `01/15/2024` (the last *saved* date), not `02/01/2024`.
    4. Submit two transactions; the second seed reflects the second saved date.
  - GREEN: add `txnDialogLastSavedDate types.Date` (or `time.Time`) field on `App` (`internal/tui/app.go`, near `txnDialog` at app.go:131). In `submitTransactionDialog` (`transaction_dialog.go:246`) write the parsed `date` value to that field on the success path *only after* the save succeeds (set inside the `func() tea.Msg` closure on the path that returns `transactionDialogSavedMsg{}`). In `buildTransactionDialog` (`transaction_dialog.go:118`) seed the date from `App.txnDialogLastSavedDate` when non-zero, else from `time.Now()`. The simplest plumbing is to pass a `seedDate time.Time` argument into `buildTransactionDialog` and let `App` decide what to send.
  - CLEANUP: none — the `time.Now()` fallback stays inline.
  - DOCS: deferred to Phase 6 (one-line note in README under "Transactions" → CLI/TUI flow; spec note in `specs/tui.md` under "Transaction Entry/Edit Dialog").

---

## Phase 2: Masked date widget — primitive + new-transaction dialog

Introduces a new `FieldDate` field type. The widget renders a fixed-width 10-character `MM/DD/YYYY` string; the cursor only lands on the eight digit positions (skips slashes); typing a digit overwrites in place and auto-advances; Backspace replaces with `0` and steps back; the current digit renders with a block highlight.

- [x] **TD-002 — Add `FieldDate` widget primitive (`MM/DD/YYYY`)**
  - RED: tests in `internal/tui/dialog_test.go` exercising the new type:
    1. Constructed `Field{Type: FieldDate, Value: "01/15/2024", cursorPos: 0}`: typing `0`, `2` produces `Value == "02/15/2024"`, cursor advances over the `/` to digit position 3 (string index 3, which is the first digit of DD).
    2. Cursor never lands on a slash position: stepping right from string index 1 skips to index 3; stepping right from index 4 skips to index 6.
    3. Backspace at digit position string index 3 sets that digit to `0` and moves cursor back to string index 1 (skipping the `/` at index 2).
    4. Home jumps to the first digit (string index 0); End jumps to the last digit (string index 9).
    5. Non-digit input (letters, punctuation other than handled keys) is ignored.
    6. Initial cursor on focus is the first digit (string index 0).
    7. `Value` is always exactly 10 characters in the canonical mask shape `??/??/????` while the field is in the focused/editing state — no partial states.
  - GREEN: extend `internal/tui/dialog.go`:
    - Add `FieldDate FieldType` constant near `FieldList` (around dialog.go:24).
    - Add a guard in `InsertChar`/`DeleteBack`/`DeleteForward` so they no-op for `FieldDate` (its key handling is special and lives in the central `HandleKey` dispatch).
    - Add masked-cursor methods on `Field`: `dateCursorRight`, `dateCursorLeft`, `dateCursorHome`, `dateCursorEnd`, `dateOverwriteDigit(rune)`, `dateBackspace`. These manipulate `cursorPos` with awareness of slash positions (string indices 2 and 5) and only operate on `Value` if the rune is `'0'..'9'`.
    - Add `AddDateField(label, initialValue, _placeholder, _width)` helper near `AddTextField` (dialog.go: ~330). Width is fixed at 10; placeholder unused; initial value defaults to a valid date string (callers that previously passed `today := time.Now().Format("01/02/2006")` keep working). If `initialValue == ""`, default to `"  /  /    "` *only* for the optional-end-date case, and require callers to opt in via a separate `AddOptionalDateField` helper that allows the all-blank state. (Decision: scheduled-dialog's End Date is the only known optional date; defer this branch to Phase 5 item TD-011 and keep `AddDateField` strict-format for now.)
    - Extend `Dialog.HandleKey` (dialog.go:499) to dispatch `FieldDate` to a new `handleDateKey(field, msg)` that wires the masked-cursor methods to Left/Right/Home/End/Backspace and to digit keypresses. Tab/Shift+Tab/Enter/Esc continue to be handled by the dialog-level dispatch.
    - Extend `Dialog.Render` (dialog.go:617) to render `FieldDate` with block-highlight styling on the digit at `cursorPos` when the field is focused. Slashes render in normal text style.
  - CLEANUP: none.
  - DOCS: deferred to Phase 6.

- [x] **TD-003 — Migrate transaction-dialog Date field to `AddDateField`**
  - RED: update tests in `transaction_dialog_test.go` that construct dialogs with `d.AddTextField("Date", …)` (around lines 325, 354, 414, 453, 486, 521, 556, 597) to use `d.AddDateField(…)` instead. Add at least one new test asserting that typing into the focused date field uses overwrite semantics and produces a valid `MM/DD/YYYY` `Value` after a few keypresses. The "invalid date" test at line 486 (`"not-a-date"`) becomes "out-of-range date" using a syntactically valid but semantically invalid string like `"13/45/2024"` — `parseDateInput` already returns an error for that and the test still asserts on the rendered error message.
  - GREEN: change `transaction_dialog.go:123` from `d.AddTextField("Date", today, "MM/DD/YYYY", 10)` to `d.AddDateField("Date", today)`. The `Required = true` on the returned field stays.
  - CLEANUP: none.
  - DOCS: deferred to Phase 6.

---

## Phase 3: Category combo-box widget

Introduces a new `FieldCombo` field type: an autocompleting select where typing filters the list, the list expands inline below the field while focused, and arrow keys navigate the filtered subset. After this phase, the new-transaction dialog's category dropdown is a typeahead combo box; the `[+ Add new]` row arrives in Phase 4.

- [x] **TD-004 — Add `FieldCombo` widget primitive (typeahead + filtered list)**
  - RED: tests in `internal/tui/dialog_test.go`:
    1. Construct `FieldCombo` with options `["(None)", "Auto", "Bills > Electric", "Food > Groceries", "Food > Restaurants"]`. Typing `f` produces a filtered list of `["Food > Groceries", "Food > Restaurants"]`; typing `g` produces `["Food > Groceries"]`; clearing the query returns to the full list.
    2. Ranking: typing `r` matches `["Food > Restaurants"]` (prefix on the visible name segment) ahead of `["Bills > Electric"]` (substring match). With option list `["Restaurant Co", "Auto Repair", "Restaurant Bar"]`, typing `r` returns `["Restaurant Bar", "Restaurant Co", "Auto Repair"]` — prefix matches first (alphabetical within prefix group), then substring matches (alphabetical).
    3. Match is substring, case-insensitive: `Gr` matches `Food > Groceries`.
    4. Up/Down navigate only the filtered subset; pressing Down past the last filtered row stays put (does not wrap).
    5. Enter on a highlighted row sets `SelectedIndex` to the index in the *full* options list, clears the query, and signals to the dialog that focus may advance.
    6. Tab without selecting (highlight on first match) accepts the highlighted row.
    7. Esc clears the query and restores the previously-selected row without changing it.
    8. Empty query shows all options, including the current selection highlighted.
  - GREEN: extend `internal/tui/dialog.go`:
    - Add `FieldCombo FieldType` constant.
    - Extend `Field` with `Query string` (the typed-but-not-yet-committed search), and a derived `FilteredIndices []int` recomputed when the query changes (or compute on the fly in handlers — pick whichever keeps the test surface small).
    - Add ranker: `rankComboMatches(options []string, query string) []int` returning indices in (prefix-first-then-substring, alphabetical-within-group) order. Pure function, easy to unit-test.
    - Add `AddComboField(label, options, selected)` helper near `AddSelectField` (dialog.go:334). Returns `*Field`.
    - Extend `Dialog.HandleKey` to dispatch `FieldCombo` to a new `handleComboKey(field, msg)` covering: digit/letter/space → append to `Query`; Backspace → trim `Query`; Up/Down → move within filtered list; Enter → commit highlighted; Esc → clear `Query` (without leaving the field); Tab/Shift+Tab → commit highlighted then leave field.
    - Extend `Dialog.Render` to render `FieldCombo` as: single line showing the typed query (or current selection if query is empty) with a chevron, and — when focused — a multi-line panel below listing filtered matches. Reuse the styling of `FieldList` where possible (FieldList already has scrolling; can borrow its scroll-window math).
  - CLEANUP: none.
  - DOCS: deferred to Phase 6.

- [x] **TD-005 — Migrate transaction-dialog Category field to `AddComboField`**
  - RED: update tests in `transaction_dialog_test.go` that construct dialogs with `d.AddSelectField("Category", …)` (lines 416, 455, 488, 523, 558, 599, 668) to `d.AddComboField(…)` and add a typing-to-filter assertion in at least one. Add a test that types into Category, confirms the filtered list narrows, presses Enter, and asserts `SelectedIndex` matches the chosen entry's index in the full options list. The submit-path tests still pass because they set `SelectedIndex` directly without typing.
  - GREEN: change `transaction_dialog.go:130` from `d.AddSelectField("Category", categoryOptions, 0)` to `d.AddComboField("Category", categoryOptions, 0)`. The `submitTransactionDialog` lookup at `transaction_dialog.go:270` (`fields[2].SelectedIndex`) is unchanged.
  - CLEANUP: none.
  - DOCS: deferred to Phase 6.

---

## Phase 4: Inline create-category sub-dialog

Introduces a small create-category dialog reachable from the category combo's filtered list via a sentinel `[+ Add new category…]` row. On confirm, the new category is persisted immediately via `category.Service.Create` (`internal/category/category_service.go:240`), the parent transaction dialog reopens with the new category selected, and focus advances to Amount.

- [x] **TD-006 — `[+ Add new category…]` action row in `FieldCombo`**
  - RED: tests in `dialog_test.go`:
    1. Combo with `AddNewLabel: "[+ Add new category…]"` set: filtered list always includes the action row at the bottom, regardless of query.
    2. Selecting the action row emits a distinct dialog action (e.g. `DialogActionAddNew` constant) rather than committing a normal selection. The dialog parent (App) intercepts that action.
    3. The action row is identifiable by index/sentinel so the parent can read the typed query at the moment of trigger.
  - GREEN: extend `Field` with `AddNewLabel string` (empty means no action row) and `AddNewTriggered bool`. Extend `handleComboKey` so Enter on the action-row index sets `AddNewTriggered = true` and returns `DialogActionAddNew` from `Dialog.HandleKey`. Add the new `DialogAction` constant. Render the action row with a distinct style (dimmed or accented; reuse theme `text.accent` slot).
  - CLEANUP: none.
  - DOCS: deferred to Phase 6.

- [x] **TD-007 — Build `createCategoryDialog` (Name + Parent + Income/Expense)**
  - RED: tests in a new `internal/tui/create_category_dialog_test.go`:
    1. Construct the dialog with `existingParents = ["Food", "Bills", "Auto"]`. Tab through fields: Name (text), Parent (combo), Income/Expense (radio).
    2. Submit with Name=`Groceries`, Parent=`Food`, Income/Expense=Expense → emits a `createCategoryRequestMsg` (or analogous message) with parent name `"Food"` (existing) and child name `"Groceries"`.
    3. Submit with Name=`Donations`, Parent=`Charity` (typed but not in existing list), Income/Expense=Expense → emits a request that flags the parent as "create new top-level".
    4. Validation: empty Name → inline error.
    5. Esc cancels; no message emitted.
  - GREEN: create `internal/tui/create_category_dialog.go` containing `buildCreateCategoryDialog(query string, existingParents []string)` analogous to `buildTransactionDialog`. Use `AddTextField` for Name, `AddComboField` for Parent (with `AddNewLabel = ""` — typing a non-matching name and submitting the dialog is the new-parent path; flag in submission), `AddRadioField` for Income/Expense. The submission handler is a method on `App` (`submitCreateCategoryDialog`) that resolves parent (existing → use ID; new → call `categorySvc.Create` to make the parent first), then creates the child via `categorySvc.Create`. Both creates fire on the success path; failure surfaces an inline error and leaves the dialog open.
  - CLEANUP: none.
  - DOCS: deferred to Phase 6.

- [x] **TD-008 — Wire transaction-dialog → create-category sub-dialog → back**
  - RED: tests in `transaction_dialog_test.go`:
    1. Open the transaction dialog; select `[+ Add new category…]` from the category combo. Verify `App.txnDialog` is hidden and `App.createCategoryDialog` is visible (or however the App-level state holders are named — e.g. `App.createCatDialog`).
    2. Submit the create-category dialog with valid fields; verify the new category is persisted (assert via the in-memory test `categorySvc.List()` returning the new entry); the transaction dialog reopens with the date/payee/amount/memo fields preserved exactly as they were before the diversion; the category field's `SelectedIndex` points to the new category; focus advances to Amount (FocusIndex==3).
    3. Cancel (Esc) the create-category dialog; transaction dialog reopens with all previous fields preserved; category field's `SelectedIndex` is unchanged from before; focus stays on the category field.
  - GREEN:
    - Add `App.createCatDialog *Dialog` and `App.savedTxnDialogState *<snapshot>` fields (the snapshot captures all current `Field.Value`/`SelectedIndex`/`Checked`/`cursorPos`/`Query` values). 
    - In `handleTransactionDialogKey` (`transaction_dialog.go:223`), when `Dialog.HandleKey` returns `DialogActionAddNew` for the category field (focus index 2), snapshot the dialog state, hide the transaction dialog, and open the create-category dialog seeded with the typed `Query` — pre-fill is wired in TD-009.
    - On `createCategoryRequestMsg` success (the `tea.Msg` returned by the create-category submit), persist via `categorySvc.Create`, then rebuild the transaction dialog from the snapshot — with the new category appended to `categoryOptions`/`txnDialogCategoryIDs` and `SelectedIndex` set to that entry. Set `FocusIndex` to the Amount field.
    - On create-category cancel, restore the transaction dialog from the snapshot unchanged.
  - CLEANUP: none.
  - DOCS: deferred to Phase 6.

- [x] **TD-009 — Pre-fill new-category Name from category-field query (with `Parent:Child` split)**
  - RED: tests in `create_category_dialog_test.go` and `transaction_dialog_test.go`:
    1. Combo query `Donations` → `[+ Add new]` → create-category dialog opens with Name=`Donations`, Parent empty, focus on Parent.
    2. Combo query `Food:Sushi` (contains `:`) → create-category dialog opens with Parent=`Food`, Name=`Sushi`. If `Food` is an existing parent, the Parent combo's `SelectedIndex` resolves to it (no new-parent flag); if not, it's typed-but-not-matched (the new-parent path).
    3. Combo query empty → create-category dialog opens with all fields empty, focus on Name.
    4. Combo query `:Groceries` (leading colon) → Name=`Groceries`, Parent empty (treat as malformed, same as empty parent).
  - GREEN: in TD-008's wiring code, parse the captured query: split on the first `:`; trim each side; route to `buildCreateCategoryDialog(name, parent, existingParents)`. Add a small helper `splitCategoryQuery(q string) (parent, name string)` near `buildCategoryOptions` in `transaction_dialog.go` so it can be unit-tested directly.
  - CLEANUP: none.
  - DOCS: deferred to Phase 6.

---

## Phase 5: Roll out masked-date widget to remaining date fields

Mechanical migration of the remaining 9 dialogs to `AddDateField`. Each item is a single-PR swap of `AddTextField("…Date", today, "MM/DD/YYYY", 10)` → `AddDateField("…Date", today)`. Existing dialog tests typically construct fields with `AddTextField("Date", "01/15/2024", "", 10)` — those are updated to `AddDateField("Date", "01/15/2024")` in the same PR.

- [ ] **TD-010 — Migrate transfer dialog**
  - RED: update `transfer_dialog_test.go` AddTextField date constructions (lines 270, 309, 370, 411, 452, 490, 528, 687) to `AddDateField`. Existing assertions on parsed dates remain valid.
  - GREEN: `internal/tui/transfer_dialog.go:63` — swap to `AddDateField`.
  - CLEANUP: none.
  - DOCS: deferred to Phase 6.

- [ ] **TD-011 — Migrate scheduled dialog (Start Date + optional End Date)**
  - RED: tests in `scheduled_dialog_test.go` (and any End Date round-trip cases) covering: Start Date filled by default; End Date empty by default and accepted as empty on submit; End Date typed value parses correctly. The optional-blank case requires a small extension: `AddOptionalDateField(label, initialValue)` that allows an all-blank value (`"  /  /    "`) and only validates when at least one digit has been typed.
  - GREEN: `internal/tui/dialog.go` — add `AddOptionalDateField` that returns a `FieldDate` field with `OptionalBlank: true`; teach `handleDateKey` and `Render` about the all-blank state. `internal/tui/scheduled_dialog.go:156` and `:236` use `AddDateField`; `:163` and `:253` use `AddOptionalDateField`. `submitScheduledDialog` already tolerates an empty End Date; tighten its parser to accept the all-blank canonical form.
  - CLEANUP: none.
  - DOCS: deferred to Phase 6.

- [ ] **TD-012 — Migrate investment dialogs (buy, sell, dividend × 2, cash, transfer-cash, transfer-shares)**
  - RED: any existing tests touching these dialogs' date fields stay green; new minimal coverage as needed. (The investment dialogs are not as densely tested as transaction/transfer; do not bulk up tests beyond what's needed to assert masked behavior on at least one of them.)
  - GREEN: swap to `AddDateField`:
    - `internal/tui/investment_buy_dialog.go:87`
    - `internal/tui/investment_sell_dialog.go:60`
    - `internal/tui/investment_dividend_dialog.go:49` and `:97`
    - `internal/tui/investment_cash_dialog.go:27`
    - `internal/tui/investment_transfer_cash_dialog.go:86`
    - `internal/tui/investment_transfer_shares_dialog.go:105`
  - CLEANUP: none.
  - DOCS: deferred to Phase 6.

- [ ] **TD-013 — Migrate corporate-action dialogs (split, merger, spin-off)**
  - RED: tests in any existing corporate-action dialog tests stay green.
  - GREEN: swap to `AddDateField`:
    - `internal/tui/corporate_action_split_dialog.go:46`
    - `internal/tui/corporate_action_merger_dialog.go:48`
    - `internal/tui/corporate_action_spinoff_dialog.go:50`
  - CLEANUP: none.
  - DOCS: deferred to Phase 6.

- [ ] **TD-014 — Migrate account opening-date and reconciliation statement-date**
  - RED: existing tests stay green; add at least one assertion that the masked widget is in use (e.g., assert field type).
  - GREEN: swap to `AddDateField`:
    - `internal/tui/account_dialog.go:150` and `:194`
    - `internal/tui/reconciliation_view.go:57`
  - CLEANUP: none.
  - DOCS: deferred to Phase 6.

- [ ] **TD-015 — Extend `AddDateField` for `YYYY-MM-DD` and migrate price-view date fields**
  - RED: tests in `dialog_test.go` for the new format variant: `AddDateField(label, initialValue, …)` accepts a format option (e.g. `WithFormat("YYYY-MM-DD")`); cursor skips dashes at string indices 4 and 7; total mask width is 10. Existing tests on the default `MM/DD/YYYY` format stay green.
  - GREEN: extend `AddDateField` with a format-string option (or split into `AddDateFieldUS` / `AddDateFieldISO`). Update `handleDateKey` and `Render` to read the field's separator positions from a format descriptor rather than hard-coded `2` and `5`. Migrate `internal/tui/price_view.go:794` and `:813` from `AddTextField("Date", today, "YYYY-MM-DD", 12)` to the ISO form of `AddDateField`.
  - CLEANUP: none.
  - DOCS: deferred to Phase 6.

---

## Phase 6: Documentation

- [ ] **TD-016 — Update `specs/tui.md` and `README.md`**
  - GREEN:
    - `specs/tui.md` — under "Transaction Entry/Edit Dialog", note the sticky-date behavior (within a session, seeds from last saved transaction; reset on app restart). Under "Dialogs" or a new "Date Fields" subsection, document the masked-input widget: cursor only on digits, slashes/dashes are skipped, typing overwrites, Backspace replaces with `0`. Under "Dialogs" or "Field Validation", document the category combo: type to filter, expanded list, prefix-then-substring ranking, `[+ Add new category…]` row that opens a sub-dialog (Name + Parent + Income/Expense, immediate save, auto-select on return).
    - `README.md` — under the "Transactions" feature bullet list, mention sticky-date and inline category creation as part of the new-transaction flow. No changes needed to the keyboard reference table — no new top-level shortcuts are added.
  - CLEANUP: none.
  - DOCS: this *is* the docs item.

---

## Out of Scope

Explicitly deferred — not in this implementation plan:

- **Sticky last-used date for other dialogs.** Only the new-transaction dialog is sticky. Transfer, scheduled, investment, etc. keep `today` as their default. Revisit if batch entry into one of those dialogs becomes a common workflow.
- **Cross-launch persistence of the sticky date.** No `config.json` field, no per-account state. Process-lifetime only. Revisit if users complain about losing context across restarts.
- **Combo-box upgrade for the Payee field.** Payee already supports free-text typing and auto-creation on first use, so its UX is functionally fine. Upgrading to `FieldCombo` for visual consistency is a follow-up if the new combo widget proves it earns its keep.
- **Combo-box upgrade for the split-transaction and scheduled-transaction category fields.** Both currently use `FieldSelect`. Migrate opportunistically when next touched; not load-bearing for this plan's user-visible win.
- **Filtering categories by income vs expense based on amount sign.** The dialog field order is Date → Payee → Category → Amount, so the amount sign is unknown at category-pick time. Reordering or dynamic filtering is a separate UX experiment.
- **Auto-validation of impossible date digits as you type** (rejecting `13` as a month, etc.). The current per-field-on-Tab/Enter validation flow is preserved; the masked widget does not add per-keystroke rejection.
- **Mouse support for the combo-box typeahead and `[+ Add new]` row.** Mouse-click selection of a list row is already covered by the existing `FieldList` mouse support; the action row picks it up for free if rendered as a list row. Verifying mouse-click on the action row triggers the same `DialogActionAddNew` is left as a manual-test step in TD-006 rather than a dedicated automated test.
- **Themable styling for the masked-date block highlight and combo-box action row.** Both reuse existing theme slots (`table.selected.bg`, `text.accent`); no new slot is introduced. Add slots only if the visual treatment turns out to need it.
