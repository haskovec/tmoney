# Phase 1 Implementation Plan: Inline Category Creation Across All Five Surfaces

## Background

The README and `specs/tui.md` both describe a `[+ Add new category…]` row at
the bottom of the Category combo box that opens a sub-dialog to create a
new category from inside any transaction-entry flow. The scaffolding
exists in code (sub-dialog, persistence helper, `AddNewLabel` field on
`Field`, comprehensive tests) but **no production combo field sets
`AddNewLabel`**, and the scheduled dialog uses a plain `AddSelectField`
with no typeahead at all. Result: the documented feature is invisible to
users.

This plan fills the gap across every category-input surface in the TUI.

### Affected surfaces

| # | Surface | File | Current state |
|---|---|---|---|
| 1 | New/Edit Transaction | `internal/tui/transaction_dialog.go:229` | Standard combo, missing `AddNewLabel` |
| 2 | Scheduled (new + edit) | `internal/tui/scheduled_dialog.go:197,273` | Plain `AddSelectField` — needs Select→Combo conversion + `AddNewLabel` |
| 3 | Scheduled Preview (post-time edit) | `internal/tui/scheduled_preview_dialog.go:189` | Standard combo, missing `AddNewLabel` |
| 4 | Split Dialog | `internal/tui/split_dialog.go` | Custom widget — needs its own action-row + create-flow integration |
| 5 | Paycheck Wizard | `internal/tui/paycheck_wizard.go` | Custom widget — multiple section-specific pickers |

### Out of scope (Phase 2)

A Categories management view for rename / delete / reparent / type-change
on existing categories is **not** part of this plan. It is deferred to a
separate design pass after Phase 1 ships.

## Decisions made during design

1. **Scope: all five surfaces in one phase.** Inconsistent UX (some
   dialogs offer Add-New, others don't) is worse than the current
   uniform brokenness.
2. **Type radio default is context-aware where free, else Expense.**
   - Paycheck Wizard Earnings / Net Pay Destination sections → Income
   - Paycheck Wizard Taxes / Pre-tax / Post-tax sections → Expense
   - Transaction / Split / Scheduled / Scheduled Preview with typed
     amount: positive → Income, negative → Expense
   - Empty / unparseable / ambiguous → Expense (current behavior)
3. **Workflow is plan-first, TDD, then implement.** Each task writes
   tests first, then production code.
4. **Spec maintenance is part of Phase 1.** `specs/tui.md` and `README.md`
   are updated alongside the code so the docs stop lying.

## Architecture: enum dispatch on `App`

The codebase consistently uses **scratch state on `App`** for
cross-dialog coordination. The precedents:

- `internal/tui/app.go:274,280,290` — `stockSplitDialogPreSelectedID`,
  `mergerDialogPreSelectedID`, `spinOffDialogPreSelectedID` for the
  Portfolio → corporate-action diverts
- `internal/tui/app.go:146-147` — `pendingSplitTxn` /
  `pendingSplitScheduled` with the nil-pointer discriminator at
  `internal/tui/split_dialog.go:776-780`:
  ```
  if a.pendingSplitScheduled != nil {
      return a.submitScheduledSplitDialog()
  }
  return a.submitSplitDialog()
  ```

We adopt the same shape for the create-category sub-dialog, with an
**enum** rather than nil-pointer discrimination because the transaction
and scheduled-preview surfaces already populate
`txnDialogData`/`schedPreviewDialog` for other reasons, so nil-pointer
checks are ambiguous.

### Additions to `App`

```
// internal/tui/app.go — append near line 142 (alongside createCatDialog)
createCatSource       createCategorySource
createCatPaycheckLine *PaycheckLine // non-nil only when source == paycheck wizard
createCatSplitRow     int           // -1 when source != split dialog
```

### The router

`applyCreatedCategory` becomes a 10-line dispatcher:

```
applyCreatedCategory(req):
  newCat, err := a.persistCategory(req)
  if err != nil { return err }
  cats, _ := a.categorySvc.List()
  switch a.createCatSource:
    case createCatSourceTxnDialog:      applyCreatedCategoryToTxn(newCat, cats)
    case createCatSourceSchedDialog:    applyCreatedCategoryToSched(newCat, cats)
    case createCatSourceSchedPreview:   applyCreatedCategoryToSchedPreview(newCat, cats)
    case createCatSourceSplitDialog:    applyCreatedCategoryToSplit(newCat, cats)
    case createCatSourcePaycheckWizard: applyCreatedCategoryToPaycheck(newCat, cats)
  a.createCatSource = createCatSourceNone
  a.createCatPaycheckLine = nil
  a.createCatSplitRow = -1
  a.createCatDialog = nil
  return nil
```

Shared persistence stays in `persistCategory(req) (*category.Category, error)`.

### Alternatives considered

- **Closure callback on `App`** (`createCatOnSuccess func(...)`):
  rejected. Closure-on-model state is absent from the codebase and
  escapes Bubbletea's model-passes-by-value semantics — the captured
  `*App` may diverge from the current model after rebuilds.
- **Per-dialog copies** of the open/apply pair: rejected. Five copies
  of ~60 lines each; future changes (e.g., spec adjustments) would have
  to land five times.

## Status Legend

- `[ ]` Not started
- `[x]` Complete

## Task Checklist

Each item is one small session of work following a red-green
(test-first) pattern. Mark items as complete with `[x]` as they are
finished. The detailed design notes for each task live in the
`## Task N — …` sections below — refer to them when starting a task.

Task ordering — task 0 is the keystone; tasks 1 & 2 lock in the router
on the simplest surfaces; task 3 is the riskiest mechanical change
(`AddSelectField` → `AddComboField`); tasks 4 & 5 are custom-widget
integrations; task 6 is the cross-cutting UX value-add; task 7 brings
the docs in line.

- [x] **CC-000 — Task 0: Refactor `applyCreatedCategory` into shared `persistCategory` core + router** (foundational)
  - RED: unit tests against `persistCategory(req)` for top-level,
    existing-parent (child inherits parent Type), new-parent
    (parent+child both created), and nil-service guard. Router test
    pinning the unknown-source safety branch.
  - GREEN: extract persistence half out of
    `internal/tui/transaction_dialog.go`; add `createCategorySource`
    enum + `createCatSource` scratch field on `App`; the router
    dispatcher lives in `create_category_dialog.go`. Existing
    transaction-dialog Add-New tests pass unmodified.
  - Confirm: `go build ./... && go test ./internal/tui/... && golangci-lint run ./internal/tui/...` green.
  - Done: shipped on `main` as commit `refactor(tui): split applyCreatedCategory into persistCategory + router`. `persistCategory` is pure persistence; `applyCreatedCategory` is a 10-line router that dispatches on `createCatSource` into per-surface appliers, with `applyCreatedCategoryToTxn` as the only wired surface. 5 unit tests added (`TestPersistCategory_TopLevel/_ExistingParent/_NewParent/_NilService`, `TestApplyCreatedCategory_UnknownSourceClearsDialog`). Full suite (5296 tests) and lint green.

- [x] **CC-001 — Task 1: New/Edit Transaction Dialog sets `AddNewLabel` + wires router** (low risk)
  - RED: `TestBuildTransactionDialog_CategoryComboHasAddNewLabel` reads
    `d.Fields()[2].AddNewLabel` after `buildTransactionDialog(...)` and
    asserts `"[+ Add new category…]"`. Remove the manual `AddNewLabel`
    injection in `transaction_dialog_test.go:1138` and verify the
    existing TD-008 flow tests still pass.
  - GREEN: capture the `*Field` returned by `d.AddComboField("Category", ...)`
    at `internal/tui/transaction_dialog.go:229` and set
    `f.AddNewLabel = "[+ Add new category…]"`. Opener already sets
    `createCatSource = createCatSourceTxnDialog` (landed in CC-000).
  - Confirm: existing `TestApp_TxnDialog_AddNew_*` tests pass without
    manual `AddNewLabel` injection.
  - Done: production code now sets `AddNewLabel = "[+ Add new category…]"`
    on the Category combo for both new and edit Transaction dialogs.
    New `TestBuildTransactionDialog_CategoryComboHasAddNewLabel` pins
    the behaviour; the manual `cat.AddNewLabel = …` injection in
    `newAppForTxnAddNew` was removed and all `TestApp_TxnDialog_AddNew_*`
    flow tests still pass. Full suite (5297 tests) and `golangci-lint`
    green. The feature is now user-visible on the simplest surface and
    the Task 0 router (`createCatSource = createCatSourceTxnDialog` →
    `applyCreatedCategoryToTxn`) handles dispatch end-to-end.

- [x] **CC-002 — Task 2: Scheduled Preview Dialog (single-line) wires `AddNewLabel`** (low risk)
  - RED: `TestBuildPreviewHeaderSingle_CategoryComboHasAddNewLabel`;
    `TestApp_SchedPreview_AddNew_{OpensCreateCategoryDialog, CancelRestoresState, SubmitPersistsAndAdvancesFocus}`.
  - GREEN: capture the `*Field` at
    `internal/tui/scheduled_preview_dialog.go:189`; extend
    `handleSchedulePreviewDialogKey` to handle `DialogActionAddNew`;
    add `openCreateCategorySubDialogFromSchedPreview()` (sets
    `createCatSource = createCatSourceSchedPreview`) and
    `applyCreatedCategoryToSchedPreview(newCat, cats)` as the per-
    surface applier.
  - Confirm: multi-line preview unaffected (it routes through the
    embedded split editor — covered by Task 4).
  - Done: single-line preview's Category combo now sets
    `AddNewLabel = "[+ Add new category…]"` so the action row is visible.
    `handleSchedulePreviewDialogKey` handles `DialogActionAddNew` by
    diverting into `openCreateCategorySubDialogFromSchedPreview`, which
    sets `createCatSource = createCatSourceSchedPreview` and hides the
    header dialog. The Task 0 router dispatches to the new
    `applyCreatedCategoryToSchedPreview`, which rebuilds the dialog's
    `categoryIDs` + Category combo options, selects the new category,
    and advances focus to Amount. `cancelCreateCatDialog` now restores
    the originating surface based on `createCatSource` (txn dialog vs.
    sched preview). `topLevelParentNames` was refactored to take
    `[]*category.Category` directly and a shared
    `App.parentsForCreateCatDialog()` helper picks the parents source
    based on `createCatSource` (cached `txnDialogData` for txn, live
    `categorySvc.List()` for sched preview). 4 new tests added; full
    `./internal/tui/` suite (1696 tests) and `golangci-lint` green.

- [x] **CC-003 — Task 3: Scheduled new + edit Dialog — `AddSelectField` → `AddComboField` + `AddNewLabel`** (highest risk — input-handling conversion)
  - RED: `TestBuildNewScheduledDialog_CategoryIsCombo`;
    `TestBuildNewScheduledDialog_CategoryComboHasAddNewLabel`;
    `TestBuildEditScheduledDialog_CategoryIsCombo`;
    `TestApp_SchedDialog_AddNew_*` trio; integration regression-pin
    `TestSubmitScheduledDialog_CategoryRoundTrip` (pick, save, reload,
    assert preselection); behaviour pin
    `TestScheduledDialog_CategoryCombo_TabAwayPreservesPreviousSelection`.
  - GREEN: convert `AddSelectField` → `AddComboField` at
    `internal/tui/scheduled_dialog.go:197` and `:273`; capture the
    `*Field`; set `AddNewLabel`; extend
    `handleScheduledDialogKey` (lines 428-445) to handle
    `DialogActionAddNew`; add per-surface opener + applier as in CC-002.
  - Confirm: all existing `scheduled_dialog_test.go` tests pass; the
    "Edit as paycheck →" alternate button is untouched.
  - Done: New + Edit Scheduled dialogs now use `FieldCombo` for Category
    with `AddNewLabel = "[+ Add new category…]"` set. The pre-existing
    `TestBuildNewScheduledDialog_FieldTypes` expectation was flipped
    from `FieldSelect` to `FieldCombo` (the only test that pinned the
    type). `handleScheduledDialogKey` routes `DialogActionAddNew` into
    a new `openCreateCategorySubDialogFromSched`, which sets
    `createCatSource = createCatSourceSchedDialog` and hides the
    scheduled dialog. The Task 0 router dispatches to the new
    `applyCreatedCategoryToSched`, which rebuilds
    `schedDialogCategoryIDs` + `schedDialogCategoryOptions` and the
    Category combo's options/SelectedIndex, then advances focus to
    Amount. `cancelCreateCatDialog` learned a third branch to restore
    the scheduled dialog. 8 new tests added (Category-is-Combo for new
    + edit, AddNewLabel, Tab-away preserves prior SelectedIndex,
    open/cancel/submit trio, and the submit→reload category
    round-trip). Full suite (5309 tests across 26 packages) and
    `golangci-lint run ./...` green; "Edit as paycheck →" alternate
    button untouched.

- [x] **CC-004 — Task 4: Split Dialog appends `[+ Add new category…]` sentinel past Transfer** (medium risk — custom widget)
  - RED: `TestSplitDialog_CategoryCount_IncludesAddNewSentinel`;
    `TestSplitDialog_DownPastTransfer_LandsOnAddNew`;
    `TestSplitDialog_EnterOnAddNew_ReturnsDialogActionAddNew`;
    `TestApp_SplitDialog_AddNew_{OpensCreateCategoryDialog, AppliesToCurrentRow}`.
  - GREEN: add `addNewSentinelLabel` constant; bump
    `categoryOptionCount()` `+1` → `+2`; add `isAddNewSentinel(idx)`
    helper; extend `categoryOptionLabel`; update Down/Up handling at
    `internal/tui/split_dialog.go:476-491` so the new sentinel sits
    after `Transfer →`; app-level handler at lines 769-787 switches on
    `DialogActionAddNew`; new `openCreateCategorySubDialogFromSplit()`
    sets `createCatSource = createCatSourceSplitDialog` and
    `createCatSplitRow = sd.rowIndex`; new
    `applyCreatedCategoryToSplit(newCat, cats)` rebuilds
    `sd.categoryOptions` / `sd.categoryIDs` and points the originating
    row at the new index.
  - Confirm: existing split-dialog tests pass; Down past `Transfer →`
    reveals `[+ Add new category…]`; Up steps back to `Transfer →`.
  - Done: The Split dialog's per-row Category picker now exposes
    `[+ Add new category…]` as a sentinel past `Transfer →` (index
    layout: real cats `[0..N-1]`, Transfer at `N`, AddNew at `N+1`).
    `categoryOptionCount` returns `N+2`; `isAddNewSentinel` /
    `categoryOptionLabel` were extended; `validate()` rejects landing
    on the AddNew sentinel without activating it. Down at the last
    transfer-mode account now exits transferMode to the AddNew
    sentinel so AddNew is reachable even when transfer targets are
    configured (the existing `Saturate at last account` assertion in
    `TestSplitDialog_SelectTransfer_OpensAccountPicker` was updated to
    pin the new behavior). Enter on the AddNew sentinel returns
    `DialogActionAddNew`; the App-level `handleSplitDialogKey` switch
    diverts into a new `openCreateCategorySubDialogFromSplit`, which
    seeds `createCatSource = createCatSourceSplitDialog`,
    `createCatSplitRow = sd.rowIndex`, hides the split dialog via a
    newly-added `SetVisible`, and opens the create-category sub-dialog
    with empty Name/Parent (the split picker has no typed query to
    harvest). `cancelCreateCatDialog` learned a `createCatSourceSplitDialog`
    branch to re-show the split dialog and reset `createCatSplitRow`.
    The Task 0 router dispatches to a new
    `applyCreatedCategoryToSplit`, which rebuilds
    `sd.categoryOptions` / `sd.categoryIDs` from
    `buildCategoryOptions`, re-maps non-originating rows to their
    preserved category by ID (so a row that pointed at "Food" before
    the rebuild still points at "Food" after, even if its index
    shifted), and points the originating row at the new category. A
    new `App.createCatSplitRow` scratch field tracks which row owns
    the sub-dialog (-1 when no split-sourced sub-dialog is in flight).
    7 new tests added (`TestSplitDialog_CategoryCount_IncludesAddNewSentinel`,
    `TestSplitDialog_DownPastTransfer_LandsOnAddNew`,
    `TestSplitDialog_EnterOnAddNew_ReturnsDialogActionAddNew`,
    `TestSplitDialog_EnterOnRealCategory_AdvancesFocus`,
    `TestSplitDialog_Validate_RejectsAddNewSentinel`,
    `TestApp_SplitDialog_AddNew_OpensCreateCategoryDialog`,
    `TestApp_SplitDialog_AddNew_CancelRestoresState`,
    `TestApp_SplitDialog_AddNew_AppliesToCurrentRow`). Full suite
    (5317 tests across 26 packages) and `golangci-lint run ./...`
    green.

- [x] **CC-005 — Task 5: Paycheck Wizard appends sentinel + section-aware Type default; shifts transfer indices on rebuild** (medium-high risk — index-shift footgun)
  - RED: `TestPaycheckWizard_CombinedOptionsIncludesAddNewSentinel`;
    `TestPaycheckWizard_IsAddNew_TrueForLastIndex`;
    `TestPaycheckWizard_EnterOnAddNew_ReturnsDialogActionAddNew`;
    `TestApp_PaycheckWizard_AddNew_OpensCreateCategoryDialog_TaxLine_DefaultsExpense`;
    `TestApp_PaycheckWizard_AddNew_OpensCreateCategoryDialog_EarningsLine_DefaultsIncome`;
    `TestApp_PaycheckWizard_AddNew_AppliesToOriginatingLine`;
    footgun-pinning
    `TestPaycheckWizard_AddNew_PreservesTransferLineSelections`.
  - GREEN: append AddNew sentinel to `combinedOptions`
    (`internal/tui/paycheck_wizard.go:338-342`); add `IsAddNew()` next
    to `IsTransfer()`; detect Enter-on-AddNew in `dispatchSelectFieldKey`
    (lines 1257-1264); extend `handlePaycheckWizardKey` to handle
    `DialogActionAddNew`; new opener seeds
    `createCatPaycheckLine = target.line` and the section-aware Type
    default; new `applyCreatedCategoryToPaycheck(newCat, cats)`
    rebuilds `combinedOptions`, updates each line's
    `selectField.Options` explicitly (lines share the backing slice),
    shifts every transfer-mode line's `SelectedIndex` and
    `line.categoryCount` by `(newCategoryCount - oldCategoryCount)`,
    and points the originating line at the new category's index.
  - Confirm: each line's select includes `[+ Add new category…]`;
    sub-dialog Type radio pre-set per originating section; after
    submit, other lines (incl. transfers) retain their effective
    selections.
  - Done: The Paycheck Wizard's per-line select picker now exposes
    `[+ Add new category…]` as a sentinel appended after the transfer
    block (layout: categories `[0..N-1]`, transfers `[N..N+|A|-1]`,
    AddNew at `N+|A|`). `paycheckAddNewSentinelLabel` is a package
    constant; `NewPaycheckWizard` and `applyCreatedCategoryToPaycheck`
    both append it to `combinedOptions`. New `PaycheckLine.IsAddNew()`
    accessor reports the sentinel state; `IsTransfer()` now excludes
    the trailing sentinel (`< len(Options)-1`); `SetAccountIndex`
    upper bound was tightened by 1 so it can't accidentally land on
    the sentinel. `handleEnter` diverts via the new
    `PaycheckWizard.lineForSelectField` helper: when the focused field
    is a line's select parked on AddNew, it returns
    `DialogActionAddNew`; otherwise the existing advance-focus
    behavior is preserved. `handlePaycheckWizardKey` switches on
    `DialogActionAddNew` into the new
    `openCreateCategorySubDialogFromPaycheck`, which seeds
    `createCatSource = createCatSourcePaycheckWizard`,
    `createCatPaycheckLine = line`, hides the wizard via a newly-added
    `PaycheckWizard.SetVisible`, and opens the create-category sub-
    dialog with empty Name/Parent. `cancelCreateCatDialog` learned a
    `createCatSourcePaycheckWizard` branch to re-show the wizard and
    reset `createCatPaycheckLine`. The Task 0 router dispatches to the
    new `applyCreatedCategoryToPaycheck`, which: rebuilds
    `w.combinedOptions` with the AddNew sentinel; updates each line's
    `selectField.Options` and `categoryCount` explicitly (lines share
    the backing slice); re-maps category-mode lines by ID (the new
    category may insert alphabetically into the middle, shifting
    subsequent indices); shifts transfer-mode lines' `SelectedIndex`
    by `delta = newCatCount - oldCatCount`; and points the originating
    line at the new category's index. A new `App.createCatPaycheckLine`
    scratch field tracks which line owns the sub-dialog (nil when no
    paycheck-sourced sub-dialog is in flight). The context-aware Type
    default (per-section) is deferred to CC-006 — the sub-dialog
    currently opens with the existing default (Expense). 8 new tests
    added (`TestPaycheckWizard_CombinedOptionsIncludesAddNewSentinel`,
    `TestPaycheckWizard_IsAddNew_TrueForLastIndex`,
    `TestPaycheckWizard_EnterOnAddNew_ReturnsDialogActionAddNew`,
    `TestPaycheckWizard_EnterOnRealCategory_AdvancesFocus`,
    `TestApp_PaycheckWizard_AddNew_OpensCreateCategoryDialog`,
    `TestApp_PaycheckWizard_AddNew_CancelRestoresState`,
    `TestApp_PaycheckWizard_AddNew_AppliesToOriginatingLine`,
    `TestPaycheckWizard_AddNew_PreservesTransferLineSelections`). Full
    suite (5325 tests across 26 packages) and `golangci-lint run ./...`
    green.

- [ ] **CC-006 — Task 6: Context-aware Type radio default** (low risk)
  - RED: table-driven test for `inferCategoryTypeFromAmount` covering
    `""`, `"50.00"`, `"-50.00"`, `"$50"`, `"$-50"`, `"-$50"`, `"abc"`.
    One integration test per surface asserting the create-category
    dialog opens with the expected Type SelectedIndex (0 = Expense,
    1 = Income per `create_category_dialog.go:68`).
  - GREEN: extend `buildCreateCategoryDialog` at
    `internal/tui/create_category_dialog.go:44` to accept
    `defaultType category.Type`; add shared
    `inferCategoryTypeFromAmount(s string) category.Type`; each of the
    five openers passes the right default (Amount-derived on
    Transaction/Scheduled/Scheduled Preview/Split rows; section-based
    on Paycheck Wizard).
  - Confirm: all existing create-category tests pass with explicit
    `category.TypeExpense` at call sites; new helper used uniformly
    across the four amount-bearing surfaces.
  - Done: _pending._

- [ ] **CC-007 — Task 7: Spec + README updates** (doc only)
  - GREEN: update `specs/tui.md` "Category Combo Box" section (line
    466) to enumerate the surfaces (typeahead combos vs index-navigated
    custom widgets); add the context-aware Type default paragraph to
    "Activating [+ Add new category…]" (line 489); add the
    `[+ Add new category…]` action-row mention to "Split Transaction
    Dialog" (line 136), "Scheduled Transaction Preview Dialog" (line
    167), and "Paycheck Schedule Wizard" (line 209). Update
    `README.md:65-67` so "transaction flow" enumerates transaction,
    split, scheduled, and paycheck.
  - Confirm: spec accurately enumerates the surfaces; README matches
    reality.
  - Done: _pending._

## Task 0 — Refactor `applyCreatedCategory` into shared core + router ✅ done

**Files touched:**
- `internal/tui/transaction_dialog.go:524-585` — extract persistence half
- `internal/tui/app.go:142` — add `createCatSource` and supporting scratch fields
- The router dispatcher lives in `create_category_dialog.go` (next to
  the message it consumes) for discoverability

**Test-first.** Add unit tests against `persistCategory(req)`:

- Creates parent + child when `NewParent=true`
- Looks up existing parent and inherits its `Type` when `NewParent=false`
- Creates a top-level when `ParentName=""`

Reuse the in-memory `category.Service` pattern from
`transaction_dialog_test.go:1217-1232` (temp DuckDB + `SeedDefaultCategories`).

**Acceptance criteria:**
- `persistCategory(req)` returns the new `*category.Category` and an error
- `applyCreatedCategory(req)` is a 10-line dispatcher
- Existing transaction-dialog Add-New tests
  (`transaction_dialog_test.go:1148-1408`) pass unmodified
- New `TestApplyCreatedCategory_PersistOnly` covers persistence in isolation

## Task 1 — New/Edit Transaction Dialog

**Files touched:**
- `internal/tui/transaction_dialog.go:229` — capture the returned
  `*Field` and set `AddNewLabel = "[+ Add new category…]"`
- `internal/tui/transaction_dialog.go:433-454`
  (`openCreateCategorySubDialog`) — set
  `a.createCatSource = createCatSourceTxnDialog` before opening
- `internal/tui/transaction_dialog.go:496-501`
  (`cancelCreateCatDialog`) — clear `a.createCatSource` and friends
- `internal/tui/transaction_dialog.go:524-585` — split into
  `applyCreatedCategoryToTxn` (per-surface) + Task 0 move

**Test-first.** A test today (`transaction_dialog_test.go:1138`)
manually sets `cat.AddNewLabel = "..."` to fake what production should
do. New test:
`TestBuildTransactionDialog_CategoryComboHasAddNewLabel` reads
`d.Fields()[2].AddNewLabel` after `buildTransactionDialog(...)` and
asserts it equals `"[+ Add new category…]"`. Remove the manual
injection on line 1138; verify the existing flow tests still pass.

**Acceptance criteria:**
- Production code sets `AddNewLabel` on the Category combo in both new
  and edit modes
- `TestApp_TxnDialog_AddNew_OpensCreateCategoryDialog` (line 1148)
  passes without manual `AddNewLabel` injection
- Focus advances to Amount (field index 3) after submit

**Visible behavior:** Press Tab to focus Category, press Down past the
last filtered match, see `[+ Add new category…]` dimmed at the bottom
of the dropdown panel; Enter opens the sub-dialog.

**Dependencies:** Task 0.

## Task 2 — Scheduled Preview Dialog (single-line shape)

Identical in shape to Task 1. The multi-line preview routes through the
embedded split editor, which Task 4 covers.

**Files touched:**
- `internal/tui/scheduled_preview_dialog.go:189`
  (`buildPreviewHeaderSingle`) — capture the `*Field` and set
  `AddNewLabel`
- `internal/tui/scheduled_preview_dialog.go:347-365`
  (`handleSchedulePreviewDialogKey`) — extend the switch to handle
  `DialogActionAddNew`
- New helpers: `openCreateCategorySubDialogFromSchedPreview()` and
  `applyCreatedCategoryToSchedPreview(newCat, cats)`

**Test-first.** Mirror Task 1's tests in
`scheduled_preview_dialog_test.go`:
- `TestBuildPreviewHeaderSingle_CategoryComboHasAddNewLabel`
- `TestApp_SchedPreview_AddNew_OpensCreateCategoryDialog`
- `TestApp_SchedPreview_AddNew_CancelRestoresState`
- `TestApp_SchedPreview_AddNew_SubmitPersistsAndAdvancesFocus`
  (checks `headerDialog.FocusIndex() == previewSingleFieldAmount`)

**Acceptance criteria:**
- Single-line preview shows `[+ Add new category…]` in its Category combo
- Sub-dialog cancel keeps the preview's edited fields intact
- Multi-line preview unaffected

**Dependencies:** Tasks 0, 1.

**Note:** Don't generalize the cancel-and-restore helper across surfaces
prematurely — the 4-line cancel path differs only in which dialog's
visibility to flip. A per-surface `restore<Surface>Dialog()` helper is
clearer than a configurable shared one.

## Task 3 — Scheduled Dialog: Select → Combo + AddNew (highest risk)

**Files touched:**
- `internal/tui/scheduled_dialog.go:197` — `AddSelectField` →
  `AddComboField`, capture return, set `AddNewLabel`
- `internal/tui/scheduled_dialog.go:273` — same change in edit dialog
- `internal/tui/scheduled_dialog.go:428-445` — extend
  `handleScheduledDialogKey` to handle `DialogActionAddNew`
- New helpers: `openCreateCategorySubDialogFromSched()` and
  `applyCreatedCategoryToSched`

### What input-handling code paths break

`Dialog.HandleKey` routes `FieldCombo` input internally (`dialog.go:1058-1082`)
— letter input filters, Enter/Tab commits, Esc clears query, Up/Down
moves within the filtered subset. **Nothing in `scheduledDialog`
depends on `FieldSelect`-specific behavior, except:**

- `submitScheduledDialog` reads `fields[schedFieldCategory].SelectedIndex`
  at line 476. Combo also writes through to `SelectedIndex` on commit
  — works unchanged.
- `buildEditScheduledDialog` lines 264-273 computes `catIdx` and passes
  it to `AddSelectField`. Same `catIdx` works as `AddComboField`'s
  `selected` argument.

**One subtle behavior to pin in a test:** typing into the combo to
filter, then Tabbing away without selecting, preserves the previous
`SelectedIndex` (the typed query is discarded). This is documented
combo behavior. Add:
- `TestScheduledDialog_CategoryCombo_TabAwayPreservesPreviousSelection`

### Existing test impact

`internal/tui/scheduled_dialog_test.go` — no test pins
`fields[schedFieldCategory].Type == FieldSelect`. Tests use
`SelectedIndex` directly (lines 300-302), which continues to work.
The standalone scaffolding at lines 1542-1547 doesn't need updating.

### New tests

- `TestBuildNewScheduledDialog_CategoryIsCombo`
- `TestBuildNewScheduledDialog_CategoryComboHasAddNewLabel`
- `TestBuildEditScheduledDialog_CategoryIsCombo`
- `TestApp_SchedDialog_AddNew_*` trio (open / cancel / submit)
- **Integration regression-pin:**
  `TestSubmitScheduledDialog_CategoryRoundTrip` — pick category, save,
  reload edit, assert preselection. Catches any combo-vs-select
  divergence in `submitScheduledDialog`.

**Acceptance criteria:**
- Scheduled new + edit dialogs use `FieldCombo` for Category
- Action row appears in the dropdown
- All existing scheduled_dialog_test.go tests pass
- The "Edit as paycheck →" alternate button at lines 333-339 is untouched

**Dependencies:** Tasks 0, 1.

## Task 4 — Split Dialog (custom widget)

The Split Dialog is hand-written (`split_dialog.go:75-95`), not a
standard `*Dialog`. Its per-row category picker is a `categoryIndex int`
cycled with Up/Down through `categoryOptions` plus the trailing
`Transfer →` sentinel (`split_dialog.go:188-196`). **No typeahead, no
filter, no dropdown panel** — purely navigational.

### Design

Append a second sentinel to the same index space:

- `categoryOptions[0..N-1]` — real categories
- `categoryOptions[N]` — `Transfer →` (existing)
- `categoryOptions[N+1]` — `[+ Add new category…]` (new)

**Files touched:**
- `internal/tui/split_dialog.go` — add `addNewSentinelLabel` constant
- `categoryOptionCount()` (line 188) — bump `+1` → `+2`
- Add `isAddNewSentinel(idx) bool` (mirrors `isTransferSentinel`)
- `categoryOptionLabel(idx)` (line 200) — return the new label for the
  AddNew sentinel
- `handleRowFieldKey` lines 476-491 — when Down lands on the AddNew
  sentinel, **don't** auto-switch to transfer mode; keep
  `categoryIndex` parked there
- App-level handler at lines 769-787 — switch on `DialogActionAddNew`
  and call new `openCreateCategorySubDialogFromSplit()`
- New `openCreateCategorySubDialogFromSplit()`:
  - Sets `a.createCatSource = createCatSourceSplitDialog`
  - Sets `a.createCatSplitRow = sd.rowIndex`
  - Hides split dialog (`splitDialog.SetVisible(false)`)
  - Builds create-category sub-dialog with **empty** typed query (the
    index-based picker has no query to harvest)
  - Type default: context from row's amount field (Task 6)
- New `applyCreatedCategoryToSplit(newCat, cats)`:
  - Rebuilds `sd.categoryOptions` + `sd.categoryIDs` from
    `buildCategoryOptions(cats)`
  - Looks up the new category's index in the rebuilt slice
  - Sets `sd.rows[a.createCatSplitRow].categoryIndex` to that index
  - Re-shows the split dialog

The renderer at lines 630-636 already calls
`categoryOptionLabel(row.categoryIndex)`, so the new sentinel label
renders for free.

### Tests

- `TestSplitDialog_CategoryCount_IncludesAddNewSentinel`
- `TestSplitDialog_DownPastTransfer_LandsOnAddNew`
- `TestSplitDialog_EnterOnAddNew_ReturnsDialogActionAddNew`
- `TestApp_SplitDialog_AddNew_OpensCreateCategoryDialog`
- `TestApp_SplitDialog_AddNew_AppliesToCurrentRow` — submit, originating
  row carries the new category, other rows untouched

**Acceptance criteria:**
- Down past Transfer reveals `[+ Add new category…]`
- Enter on it opens the sub-dialog
- After submit, originating row updates; others untouched
- Existing split-dialog tests pass

**Dependencies:** Tasks 0, 1.

**Subtlety:** Up from `[+ Add new category…]` steps back to
`Transfer →` then to the last real category — same skeleton as the
existing Transfer step-up at lines 458-470.

## Task 5 — Paycheck Wizard (custom widget, hardest)

The wizard uses a flat `FieldSelect` per line (`paycheck_wizard.go:442-444`),
navigated by raw Up/Down (`paycheck_wizard.go:1257-1264`).
`combinedOptions` (`paycheck_wizard.go:338-342`) merges category options
with `→ <Account>` transfer entries.

### Design

Append `[+ Add new category…]` as a synthetic last entry of
`combinedOptions`. Today's `IsTransfer()` check (line 200) is
`SelectedIndex >= categoryCount`; with the new sentinel past the
transfer entries it becomes:

- `IsTransfer()`: `SelectedIndex >= categoryCount && SelectedIndex < len(combinedOptions) - 1`
- Add `IsAddNew()`: `SelectedIndex == len(combinedOptions) - 1`

**Files touched:**
- `internal/tui/paycheck_wizard.go:338-342` — extend `combinedOptions`
  to append AddNew sentinel
- Lines 196-201 — add `IsAddNew` next to `IsTransfer`
- `dispatchSelectFieldKey` (lines 1257-1264) — detect Enter-on-AddNew
  and return `DialogActionAddNew`
- `handlePaycheckWizardKey` (line 1438) — extend switch to handle
  `DialogActionAddNew`; walk sections to find which line owns the
  focused field (the focusable list at lines 683-707 doesn't carry
  `*PaycheckLine` for `selectField` targets); store
  `a.createCatPaycheckLine = target.line`
- New `openCreateCategorySubDialogFromPaycheck(line)`:
  - Sets source, paycheck line scratch
  - Reads `line.Section` and seeds Type radio per section (Task 6)
- New `applyCreatedCategoryToPaycheck(newCat, cats)`:
  - Rebuild `combinedOptions` (capture old `categoryCount` first)
  - Update each line's `selectField.Options` explicitly (line 444
    shows lines share the same backing slice; reassigning
    `w.combinedOptions` doesn't propagate)
  - Shift every transfer-mode line's `SelectedIndex += (newCategoryCount - oldCategoryCount)`
  - Same shift for `line.categoryCount`
  - Set originating line's `selectField.SelectedIndex` to the new
    category's index

### Section-specific Type default

Earnings, Net Pay Destinations → Income. Taxes, Pre-tax, Post-tax →
Expense. Requires extending `buildCreateCategoryDialog`
(`create_category_dialog.go:44`) to accept a default `category.Type`
parameter (Task 6).

### Tests

- `TestPaycheckWizard_CombinedOptionsIncludesAddNewSentinel`
- `TestPaycheckWizard_IsAddNew_TrueForLastIndex`
- `TestPaycheckWizard_EnterOnAddNew_ReturnsDialogActionAddNew`
- `TestApp_PaycheckWizard_AddNew_OpensCreateCategoryDialog_TaxLine_DefaultsExpense`
- `TestApp_PaycheckWizard_AddNew_OpensCreateCategoryDialog_EarningsLine_DefaultsIncome`
- `TestApp_PaycheckWizard_AddNew_AppliesToOriginatingLine`
- **Footgun-pinning test:**
  `TestPaycheckWizard_AddNew_PreservesTransferLineSelections` —
  transfer-mode lines still point at the same account after the
  category insert

**Acceptance criteria:**
- Each line's select includes `[+ Add new category…]` as last entry
- Sub-dialog Type radio is pre-set per the originating section
- After submit, originating line points to new category; all other
  lines (including transfers) retain their effective selections

**Dependencies:** Tasks 0, 1, 6.

## Task 6 — Context-aware Type radio default

**Files touched:**
- `internal/tui/create_category_dialog.go:44` — extend
  `buildCreateCategoryDialog` to accept `defaultType category.Type`
  parameter (existing call sites pass `category.TypeExpense`)
- Each of the five openers passes the right default:
  - **Transaction:** read `fields[3].Value` (Amount), parse, dispatch
  - **Scheduled new/edit:** same on `fields[schedFieldAmount].Value`
  - **Scheduled Preview single-line:** same on
    `fields[previewSingleFieldAmount].Value`
  - **Split row:** same on `sd.rows[sd.rowIndex].amountField.Value`
  - **Paycheck Earnings / Net Pay Destination line:** Income
  - **Paycheck Taxes / Pre-tax / Post-tax line:** Expense

**Shared helper:**

```
func inferCategoryTypeFromAmount(s string) category.Type
```

**Test-first** with a table-driven test:

| Input | Expected |
|---|---|
| `""` | Expense |
| `"50.00"` | Income |
| `"-50.00"` | Expense |
| `"$50"` | Income |
| `"$-50"` | Expense |
| `"-$50"` | Expense |
| `"abc"` | Expense |

Plus one integration test per surface asserting the create-category
dialog opens with the expected Type SelectedIndex (0 = Expense,
1 = Income per `create_category_dialog.go:68`).

**Acceptance criteria:**
- `inferCategoryTypeFromAmount` is used uniformly across the four
  amount-bearing surfaces
- Paycheck section-based default works via pure section switch

**Dependencies:** Tasks 1-5 (openers must exist).

## Task 7 — Spec + README updates

### `specs/tui.md`

#### Section "Category Combo Box" (line 466)

Current text (line 468) implies the feature exists only on the New
Transaction dialog. Rewrite the opening paragraph:

```
The Category field is a typeahead combo box on:

- The New Transaction and Edit Transaction dialogs
- The New Scheduled Transaction and Edit Scheduled Transaction dialogs
- The Scheduled Transaction Preview dialog (single-line schedules)

The Split Transaction dialog and the Paycheck Schedule Wizard use a
simpler index-navigated picker (no typeahead) but expose the same
[+ Add new category…] action at the bottom of the option list.
```

#### Section "Activating [+ Add new category…]" (line 489)

Insert a paragraph documenting the context-aware Type default:

```
The Type radio defaults are context-aware:

- Earnings and Net Pay Destination rows of the Paycheck Wizard
  default to Income.
- Tax, Pre-tax, and Post-tax rows of the Paycheck Wizard default to
  Expense.
- The Transaction, Split, Scheduled, and Scheduled Preview dialogs
  infer the default from the typed amount: a positive number
  defaults to Income, a negative or unparseable value defaults to
  Expense.
```

#### Section "Split Transaction Dialog" (line 136)

Add: "The Category / Target combo also exposes a
`[+ Add new category…]` action row below the Transfer sentinel."

#### Sections "Scheduled Transaction Preview Dialog" (line 167) and "Paycheck Schedule Wizard" (line 209)

Add one sentence to each: "Each Category picker exposes a
`[+ Add new category…]` action row to create a new category inline."

### `README.md` (line 65-67)

Change "transaction flow" to enumerate the surfaces:

```
- Inline category creation from the Category field — pick
  [+ Add new category…] to create a new category (with optional new
  parent) without leaving the transaction, split, scheduled, or
  paycheck flow
```

**Acceptance criteria:**
- Spec accurately enumerates the surfaces
- Context-aware Type default is documented
- README matches reality

## Test strategy

### What exists today

| Layer | File | Status |
|---|---|---|
| Field combo + AddNew action row mechanics | `dialog_test.go:2748-2950+` | Comprehensive, never triggered by production |
| `buildCreateCategoryDialog` + `submitCreateCategoryDialog` | `create_category_dialog_test.go:1-296` | Comprehensive |
| App-level open / cancel / submit on Transaction surface | `transaction_dialog_test.go:1109-1408+` | Comprehensive, but manually injects `AddNewLabel` |

### New scaffolding helpers

Mirror `newAppForTxnAddNew` (`transaction_dialog_test.go:1117-1146`) as:

- `newAppForSchedAddNew`
- `newAppForSchedPreviewAddNew`
- `newAppForSplitAddNew`
- `newAppForPaycheckAddNew`

Each constructs the relevant dialog/widget with a known set of
categories and a typeahead query parked on the AddNew row.

## Risks and unknowns

1. **Multi-line preview wraps a split widget.** When a Paycheck-line or
   Split-line opens a sub-dialog, the split widget hides — but for a
   multi-line preview, the split widget is embedded inside a
   `SchedulePreviewDialog`. On restore, both layers must come back.
   Decision: when the split editor is the preview's embedded one, the
   router uses `createCatSource = createCatSourceSchedPreview`,
   distinguishing "preview" from "split" by which `App` slot holds the
   split dialog (preview's `schedPreviewDialog.splitDialog` vs. App's
   `a.splitDialog`).

2. **Paycheck wizard's `combinedOptions` is rebuilt only by
   `NewPaycheckWizard`.** Lines hold a reference to the same backing
   slice (`paycheck_wizard.go:444`); reassigning `w.combinedOptions`
   does not propagate. The applier must update each line's
   `selectField.Options` explicitly.

3. **Sticky parent ID across opens** — none of this design persists
   "user previously picked Food as parent" across consecutive Add-New
   invocations. Document as non-goal in the spec to head off scope creep.

4. **Mouse support** — wizard hit zones (`paycheck_wizard.go:88-92`)
   don't open a dropdown UI; clicks just focus the field. Mouse
   navigation to the AddNew row through Down arrows works; mouse-click
   to jump to it doesn't exist (and isn't part of any baseline). No
   new mouse work needed in Phase 1.

5. **Edit-as-paycheck round-trip** — Phase 1 does not touch the
   Edit-as-paycheck wizard reopener. Existing tagging behavior unchanged.

6. **Two-tab navigation from Paycheck Wizard's `selectField` past the
   AddNew row** — Enter on the AddNew sentinel triggers the divert;
   Tab still just moves to the next field. Same semantic the standard
   `*Dialog` already has for combo + AddNew. Pin in a test.

## Critical files for implementation

- `internal/tui/transaction_dialog.go` — Task 0 refactor target; Task 1
  surface (line 229; lines 524-585 split into shared + per-surface)
- `internal/tui/scheduled_dialog.go` — Task 3 conversion at lines 197
  and 273; new key-handler branch at lines 428-445
- `internal/tui/split_dialog.go` — Task 4 option-index space at lines
  188-205; Up/Down handling at lines 476-491
- `internal/tui/paycheck_wizard.go` — Task 5 `combinedOptions` at
  lines 338-342; `IsAddNew` next to `IsTransfer` at lines 196-201;
  Enter-on-AddNew detection at lines 1257-1264
- `internal/tui/create_category_dialog.go` — Task 6 `defaultType`
  parameter (line 44); home for the `createCategorySource` enum and
  the router
- `internal/tui/app.go` — line 142, new `createCatSource` and scratch
  fields
- `specs/tui.md` — Task 7 doc updates at lines 136, 167, 209, 466, 489
- `README.md` — Task 7 line 65-67
