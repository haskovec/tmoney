# Implementation Plan: Multi-Line Splits and Paycheck Wizard

This document defines the order in which the multi-line splits + paycheck wizard feature (`specs/multiline-splits-and-paycheck.md`) should be implemented. Each item is one small session of work following a red-green (test-first) pattern. Mark items as complete with `[x]` as they are finished.

Spec:

- `specs/multiline-splits-and-paycheck.md` — feature spec

## Status Legend

- `[ ]` Not started
- `[x]` Complete

## Priority Rationale

The plan is ordered to deliver the smallest shippable user value first, then layer scheduling and convenience on top:

1. **Primitive first** (Phases 1–3). Mixed-sign splits and transfer-lines on real transactions, with the TUI affordances to use them. After Phase 3, users can manually model a paycheck-shaped transaction in the register, even without scheduling.
2. **Scheduled multi-line storage** (Phase 4). Recurring paychecks become possible. Posting uses template values; preview comes next.
3. **Post-time preview dialog** (Phase 5). The user's per-instance-override pain point — edits to this occurrence without sticky-changes to the template.
4. **Paycheck wizard** (Phase 6). UI sugar that makes the common case effortless. Sits on top of everything else.
5. **Cross-cutting documentation** (Phase 7). Final review of existing specs and the README to ensure they describe the as-shipped behavior.

The primitive can ship without scheduling at all (a meaningful intermediate release). Scheduled multi-line + preview can ship without the wizard. Each phase is a coherent shippable unit.

Inside each phase, the data-model and repository items come before service items, which come before TUI items — this is the same red-green ordering used by the investment-total-return plan.

---

## Phase 1: Primitive — Data Model and Repository

Add schema and repository support with no behavior change yet.

- [x] **MS-001 — Add `transfer_account_id` and `transfer_id` columns to `split_items`**
  - RED: test asserts the new migration (next number, e.g. `014_split_items_transfer.sql`) adds both columns nullable, with check constraints: `(category_id IS NULL) <> (transfer_account_id IS NULL)` AND `(transfer_account_id IS NULL) = (transfer_id IS NULL)`. Existing rows continue to satisfy.
  - GREEN: add the migration in `internal/db/migrations/`. Update `internal/transaction/split_item.go` (or equivalent) so the struct exposes the two new fields.
  - Confirm: `go build ./... && go test ./internal/db/... ./internal/transaction/...` green.
  - Done: migration `014_split_items_transfer.sql` recreates `transaction_splits` with nullable `category_id`, new `transfer_account_id` / `transfer_id`, both CHECK constraints, and an `idx_splits_transfer` index. `Split` struct exposes `TransferAccountID` and `TransferID` as `NullableID`. Existing categorized rows preserved.

- [x] **MS-002 — Add `scheduled_split_items` table**
  - RED: test asserts the new migration creates the table with columns per the spec (`id`, `scheduled_transaction_id`, `category_id NULL`, `transfer_account_id NULL`, `amount`, `memo NULL`) and the `(category_id IS NULL) <> (transfer_account_id IS NULL)` check constraint.
  - GREEN: add the migration. Create `internal/scheduled/split_item.go` with the struct, basic constructors, and `Validate()`.
  - Confirm: tests green.
  - Done: migration `015_scheduled_split_items.sql` creates the table with the CHECK constraint and `idx_scheduled_split_items_parent` index; `scheduled.Split` struct (in `internal/scheduled/split_item.go`) exposes `CategoryID`/`TransferAccountID` as `NullableID`, with `NewCategorizedSplit` / `NewTransferSplit` constructors and `Validate()` enforcing the same exclusive-shape rule. `CurrentSchemaVersion` bumped to 15.

- [x] **MS-003 — Split-item repository reads/writes the new columns**
  - RED: test `TestSplitItemRepo_TransferLine_RoundTrip` — create a transaction with one categorized split-item and one transfer-typed split-item (with `transfer_account_id` and `transfer_id` set); reload via the repo; both rows come back with all fields intact.
  - GREEN: extend the split-item read/write paths to include the new columns. Existing category-only splits unaffected.
  - Confirm: tests green.
  - Done: `SplitRepository.Create`/`Update` now write the new columns (with NULL `category_id` for transfer-lines) and validate the appropriate FK target (account for transfer-lines, category otherwise); `GetByID`/`ListByTransaction`/`scanSplits` read them. Tests `TestSplitItemRepo_TransferLine_RoundTrip` and `TestSplitItemRepo_TransferLine_Update` cover both create and update flows; full suite (5169 tests) and lint stay green.

- [ ] **MS-004 — Scheduled-split-item repository CRUD**
  - RED: test `TestScheduledSplitItemRepo_RoundTrip` — create, read, update, delete rows including both category-typed and transfer-typed. Test `TestScheduledRepo_LoadsChildren` — loading a scheduled transaction also loads its `scheduled_split_items`.
  - GREEN: extend `internal/scheduled/scheduled_repository.go` (or add a new file) with the CRUD and the join load.
  - Confirm: tests green.

## Phase 2: Primitive — Service Layer

Make the new primitive usable through services. Still no UI change.

- [ ] **MS-005 — Relax split validation to allow mixed signs**
  - RED: test `TestTransactionService_MixedSignSplit_Allowed` — create a transaction with parent amount +100 and lines `+200, -100`. Expect success. Test `TestTransactionService_LegacySameSignSplit_StillWorks` — existing same-sign splits keep validating.
  - GREEN: in `internal/transaction/transaction_service.go`, update split validation to enforce `parent.amount == signed_sum(line.amounts)` instead of same-sign + absolute sum.
  - Confirm: existing transaction tests pass; new mixed-sign test passes.

- [ ] **MS-006 — Service: transfer-line auto-creates paired counterpart**
  - RED: test `TestTransactionService_TransferLine_CreatesPair` — create a transaction in account A with one categorized line and one transfer-line targeting account B. Expect a second single-line transaction in account B with the inverse amount and matching `transfer_id`. Test `TestTransactionService_SelfTransfer_Rejected` — transfer-line targeting the parent's account is rejected by validation.
  - GREEN: in the create path, detect transfer-typed split lines. For each, mint a new `transfer_id`, store it on the split-item, and create a single-line paired transaction in the target account carrying the same `transfer_id`.
  - Confirm: tests pass.

- [ ] **MS-007 — Service: editing a transfer-line updates the paired side**
  - RED: test `TestTransactionService_EditTransferLineAmount_UpdatesPair` — edit a transfer-line's amount; assert the paired transaction's amount updates in lock-step. Test `TestTransactionService_EditTransferLineTarget_MovesPair` — edit the target account; assert the old paired side is deleted and a new one is created in the new target with a fresh `transfer_id`.
  - GREEN: extend the transaction-update path with the cascade logic.
  - Confirm: tests pass.

- [ ] **MS-008 — Service: deleting a transfer-line cascades**
  - RED: test `TestTransactionService_DeleteTransferLine_DeletesPair` — delete a single transfer-line; paired side is gone; parent retains other lines (and may now be imbalanced — the delete itself succeeds but a subsequent Save with imbalance would fail).
  - Test `TestTransactionService_DeleteParent_DeletesAllPairs` — delete the parent transaction; all paired sides in target accounts are also deleted.
  - GREEN: extend the line-delete and transaction-delete paths.
  - Confirm: tests pass.

- [ ] **MS-009 — Service: reverse cascade from paired side**
  - RED: test `TestTransactionService_DeletePairedSide_RemovesParentLine` — delete the paired side from the target account's register; the corresponding split-item on the parent is removed.
  - Test `TestTransactionService_EditPairedSideAmount_UpdatesParentLine` — edit the paired side's amount; the parent's transfer-line amount updates in lock-step.
  - GREEN: extend the transaction-update/delete paths to detect when the affected transaction is a paired side of a multi-line split (lookup via its `transfer_id` against `split_items.transfer_id`) and propagate back to the parent.
  - Confirm: tests pass.

## Phase 3: Primitive — TUI for Real Transactions

Make the new primitive reachable from the TUI on regular (non-scheduled) transactions. After Phase 3, users can model a paycheck-shaped real transaction in the register without any scheduling involved.

- [ ] **MS-010 — Split dialog adds `Transfer →` category sentinel**
  - RED: test `TestSplitDialog_TransferSentinel_PresentInCategoryCombo` — the category combo box for a split line includes a `Transfer →` row alongside `[+ Add new category…]`.
  - GREEN: extend the category combo widget to render the sentinel option when used in a split-line context (not on the top-level transaction category field on single-line dialogs).
  - Confirm: tests pass.

- [ ] **MS-011 — Selecting `Transfer →` swaps to account picker**
  - RED: test `TestSplitDialog_SelectTransfer_OpensAccountPicker` — choosing `Transfer →` replaces the category field with an account-select widget that excludes the parent transaction's account.
  - GREEN: implement the swap logic. On Save, the line is persisted with `category_id=NULL`, `transfer_account_id=<picked>`, and a fresh `transfer_id`.
  - Confirm: tests pass.

- [ ] **MS-012 — Split dialog supports mixed-sign amount entry**
  - RED: test `TestSplitDialog_MixedSignAmounts_Accepted` — enter lines with mixed signs; the dialog accepts them and persists them as entered.
  - GREEN: relax any sign-matching enforcement in the split dialog widget. Line-amount inputs accept negative values (leading `-`).
  - Confirm: tests pass.

- [ ] **MS-013 — Live imbalance indicator and hard validation**
  - RED: test `TestSplitDialog_ImbalanceIndicator_VisibleAndLive` — typing into a line updates a visible "Imbalance: $X.XX" indicator that recomputes per keystroke. Test `TestSplitDialog_SaveDisabledOnImbalance` — Save button is disabled when imbalance ≠ 0; enabled when balanced.
  - GREEN: extend the split dialog with a signed-sum recompute on every keystroke and a Save-enablement guard. Indicator appears between the line list and the action buttons.
  - Confirm: tests pass.

- [ ] **MS-014 — Visual smoke check (Phase 3 milestone)**
  - Manual: launch `tmoney`, open a register, create a new transaction with a split that includes a transfer-line to another account. Verify the paired transaction appears in that account. Edit the transfer-line amount; verify the paired side updates. Delete the transfer-line; verify the paired side disappears. Delete the parent; verify all paired sides go away.

## Phase 4: Scheduled Multi-Line — Service and TUI

After Phase 3, users can manually create paycheck-shaped real transactions. Phase 4 makes them recurring.

- [ ] **MS-015 — Scheduled service supports multi-line templates**
  - RED: test `TestScheduledService_CreateMultiLine_RoundTrip` — create a scheduled transaction with multi-line split children; reload and verify both parent and children persist. Test `TestScheduledService_ValidateMultiLine_RejectsBothShapes` — a record with both a scalar `category_id` AND `scheduled_split_items` children is rejected.
  - GREEN: extend the scheduled service create/update paths to accept and persist `scheduled_split_items` rows. A scheduled transaction is multi-line when children are present; legacy single-line continues to work.
  - Confirm: tests pass.

- [ ] **MS-016 — Posting a multi-line scheduled transaction**
  - RED: test `TestScheduledService_PostMultiLine_CreatesTransactionWithSplits` — posting a multi-line schedule creates a real transaction whose `split_items` mirror the template; transfer-line templates create paired counterparts. Test `TestScheduledService_PostMultiLine_AdvancesSchedule` — `next_date` advances per the template's cadence.
  - GREEN: extend `Post()` / `PostWithDate()` to branch on multi-line vs single-line and delegate to the transaction service's multi-line create path (which already handles paired counterparts per MS-006).
  - Confirm: tests pass; auto-post path also handles multi-line.

- [ ] **MS-017 — Scheduled dialog adds Split toggle**
  - RED: test `TestScheduledDialog_SplitToggle_OpensMultiLineEditor` — toggling Split on the scheduled-transaction dialog reveals the same multi-line editor used for regular transactions. Test `TestScheduledDialog_MultiLineSave_PersistsChildren` — saving with split lines creates `scheduled_split_items` rows.
  - GREEN: extend the scheduled dialog (`internal/tui/scheduled_dialog.go`) to share the split-editor widget with the regular transaction dialog. Toggle hides the scalar `amount` / `category_id` fields when active.
  - Confirm: tests pass; manual smoke check: create a multi-line schedule via the dialog.

## Phase 5: Post-Time Preview Dialog

After Phase 4, multi-line schedules exist but post immediately. Phase 5 introduces the preview-and-edit dialog for per-instance overrides.

- [ ] **MS-018 — Preview dialog scaffolding**
  - RED: test `TestSchedulePreviewDialog_OpensWithTemplateValues` — invoking the preview action on a due item opens a dialog pre-filled with template values (date = `next_date`, amounts, lines).
  - GREEN: add `internal/tui/scheduled_preview_dialog.go`. For multi-line schedules, render the multi-line split editor; for single-line, render the regular transaction edit dialog shape.
  - Confirm: tests pass.

- [ ] **MS-019 — Enter on due item opens preview (replaces immediate post)**
  - RED: test `TestScheduledView_EnterOnDueItem_OpensPreview` — pressing Enter on a due scheduled item in the scheduled view opens the preview dialog instead of immediately calling `postSelectedScheduled`.
  - GREEN: update `handleScheduledKeys` and the existing post path to open the preview dialog instead of calling `Post()` directly.
  - Confirm: tests pass; manual smoke: Enter opens preview.

- [ ] **MS-020 — Preview-save creates real transaction and advances schedule**
  - RED: test `TestSchedulePreview_SaveCreatesTransactionAndAdvances` — save on the preview dialog creates the real transaction with any user edits, advances the schedule's `next_date` per the template's original cadence (not the edited posted-date), and closes the dialog.
  - Test `TestSchedulePreview_Cancel_NoChanges` — cancel leaves both the transaction store and the schedule untouched.
  - GREEN: implement save/cancel handlers. Schedule advancement uses existing `CalculateNextDate` against the *template's* `next_date`, not the edited posting date.
  - Confirm: tests pass.

- [ ] **MS-021 — Preview supports one-off date and line edits**
  - RED: test `TestSchedulePreview_EditDate_OneOff` — editing the date does not shift the schedule's `next_date`. Test `TestSchedulePreview_EditLineAmount_OneOff` — editing a line amount does not modify the template (next preview reverts).
  - Test `TestSchedulePreview_ImbalancedSaveDisabled` — imbalanced multi-line preview disables Save (hard validation per MS-013).
  - GREEN: ensure edits write only to the in-flight transaction; template stays untouched on save. Re-use MS-013's imbalance indicator.
  - Confirm: tests pass; manual smoke: edit FICA by a penny, balance, save, verify next instance opens with original template values.

- [ ] **MS-022 — Auto-post bypasses preview**
  - RED: test `TestAutoPost_MultiLineSchedule_BypassesPreview` — a multi-line schedule with auto-post enabled fires automatically on the scheduled date, using template values exactly, without opening any dialog.
  - GREEN: confirm existing auto-post path delegates to `Post()` (which by MS-016 handles multi-line). No UI hooks fire.
  - Confirm: tests pass.

- [ ] **MS-023 — Visual smoke check (Phase 5 milestone)**
  - Manual: create a multi-line scheduled transaction (paycheck-shaped) via the regular scheduled dialog with the Split toggle. Wait for it to be due. Press Enter. Edit FICA by 1¢, rebalance, save. Verify the register shows the parent + paired counterparts. Verify the next occurrence in the scheduled view shows the original template amounts (not the edited values).

## Phase 6: Paycheck Wizard

After Phase 5, multi-line scheduling works end-to-end. Phase 6 adds the convenience wizard.

- [ ] **MS-024 — Default paycheck categories on file initialization**
  - RED: test `TestFileInit_PaycheckCategoriesExist` — opening or creating a database ensures the categories `Income:Salary`, `Tax:Federal`, `Tax:State`, `Tax:Social Security`, `Tax:Medicare`, `Insurance:Health` exist.
  - GREEN: extend the file-init / category-seed logic to seed these categories if missing. Existing databases gain them on next open.
  - Confirm: tests pass.

- [ ] **MS-025 — Paycheck wizard form scaffolding**
  - RED: test `TestPaycheckWizard_OpensWithEmptyForm` — invoking the wizard opens a dialog with the layout from the spec: employer, frequency, gross, pre-tax deductions, post-tax deductions, net pay destinations.
  - GREEN: implement `internal/tui/paycheck_wizard.go` with the static form layout (no save logic yet).
  - Confirm: tests pass.

- [ ] **MS-026 — Wizard menu entry**
  - RED: test `TestTransactionsMenu_NewPaycheckSchedule_Item` — the `Transactions` menu includes a `New Paycheck Schedule…` item that, when activated, opens the wizard.
  - GREEN: extend the menu definition and key handlers.
  - Confirm: tests pass.

- [ ] **MS-027 — Wizard computes remainder at save**
  - RED: test `TestPaycheckWizard_Save_ComputesRemainder` — fill out gross 5000, deductions totaling 1432.50, additional transfers 500. The "Primary deposit account" line is stored as +3067.50 on the resulting schedule.
  - GREEN: in the wizard's save handler, compute `remainder = gross − sum(deductions) − sum(additional transfers)` and write it as the primary-account line. Assemble the rest of the lines from the form.
  - Confirm: tests pass.

- [ ] **MS-028 — Wizard save creates standard multi-line schedule**
  - RED: test `TestPaycheckWizard_Save_CreatesMultiLineSchedule` — after save, the database contains a `scheduled_transactions` row with the expected scalar fields and `scheduled_split_items` children matching the wizard inputs (gross + deduction lines + transfer lines + computed remainder line).
  - GREEN: wire the wizard's save handler through the scheduled service's multi-line create path.
  - Confirm: tests pass; manual smoke: fill out the wizard, save, verify the schedule appears in the scheduled view with the right structure.

- [ ] **MS-029 — Optional "Edit as paycheck" re-open**
  - RED: test `TestScheduledDialog_EditAsPaycheck_RelaunchesWizard` — a scheduled transaction that matches the paycheck heuristic (has gross income line, ≥ 1 tax-categorized deduction line, structured similarly) shows an "Edit as paycheck →" affordance; activating it relaunches the wizard with current values pre-filled.
  - Test `TestScheduledDialog_NonPaycheckShape_HidesEditAsPaycheck` — a generic multi-line schedule that doesn't match the heuristic does not show the affordance.
  - GREEN: implement a `looksLikePaycheck(st)` heuristic and the relaunch path.
  - Confirm: tests pass.

- [ ] **MS-030 — Visual smoke check (Phase 6 milestone)**
  - Manual: invoke `Transactions → New Paycheck Schedule…`, fill out the full form per the spec example, save, verify the resulting schedule, post once via the preview dialog. Verify the paired counterparts in 401k / HSA / savings accounts. Re-open the schedule via Edit as paycheck and verify pre-fill works.

## Phase 7: Cross-Cutting Documentation

Update existing specs and the README to reflect the as-shipped behavior.

- [ ] **MS-031 — Update `specs/transactions.md`**
  - GREEN: confirm the **Split Transactions** section reflects mixed-sign semantics and the new transfer-line type. Cross-reference `specs/multiline-splits-and-paycheck.md`.
  - Document that legacy paired-single-line transfers (whole-transaction transfers) remain valid alongside new transfer-lines.

- [ ] **MS-032 — Update `specs/scheduled-transactions.md`**
  - GREEN: confirm the **Multi-Line Schedules** and **Post-Time Preview** sections describe the as-shipped behavior. Document that `amount_estimate_count` applies only to single-line schedules.

- [ ] **MS-033 — Update `specs/tui.md`**
  - GREEN: confirm the **Scheduled Preview Dialog** and **Paycheck Wizard** mockups match the implemented UI. Confirm the `Transfer →` category sentinel description in the **Category Combo Box** section. Update **Scheduled Transaction Keys** so Enter on a due item is documented as opening the preview.

- [ ] **MS-034 — Update `README.md`**
  - GREEN: add a short paragraph under the existing **Scheduled Transactions** section describing multi-line schedules, the preview dialog, and the paycheck wizard. Document the new menu entry. No new top-level section; this slots into the existing block.

- [ ] **MS-035 — Mark feature spec as implemented**
  - GREEN: add a status note at the top of `specs/multiline-splits-and-paycheck.md` indicating the v1 feature is implemented, listing any out-of-scope follow-ups.

---

## Out of Scope

The following are explicitly deferred — not in this implementation plan:

- **CLI surface for multi-line schedules and the paycheck wizard.** Multi-line creation/edit remains TUI-only. If real automation needs emerge, a spec-file approach (`tmoney scheduled paycheck create --from paycheck.toml`) is the recommended path.
- **Tax-aware reports.** Pre-tax vs post-tax distinction in reports (e.g., W-2 Box 1 reconciliation) requires a `tax_treatment` field on the category master, not on individual lines. Separate spec.
- **Migration tool to convert legacy paired-transfers into transfer-lines.** Legacy data remains valid as-is.
- **Per-line variable amounts with estimation.** Multi-line schedules use fixed template amounts; preview-time edits handle variation.
- **Bulk operations on scheduled transactions** (post-all-due, skip-all, etc.).
- **Year-end tax summary report** based on paycheck data.
- **Wizard for non-paycheck complex schedules** (e.g., mortgage payment with principal/interest/escrow). Such schedules can be created via the generic multi-line scheduled dialog.
