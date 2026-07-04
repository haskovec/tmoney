# Implementation Plan: Transfer Categories

Feature spec: [`specs/transfer-categories.md`](transfer-categories.md)

Each phase is independently shippable, TDD'd (tests written with the
phase), gated on `go build ./... && go test ./...` + `gofmt`, and committed
directly to `main` before the next phase starts.

## Status Legend

- `[ ]` not started
- `[~]` in progress
- `[x]` done

## Priority Rationale

The report guards come first: they fix the latent `transfer link`
category-leak today, implement the one user-confirmed decision (opt-in
toggle), touch no schema, and ship alone. The **confirmed `ReplaceSplits`
corruption fix** lands second — it is a live pre-existing bug (raw CHECK
error plus orphaned counterparts on every TUI split edit that contains a
transfer line), fixable against today's schema, and Phase 7's
carry-through rides that path. Schema + validation relaxation next: the
split-line and scheduled phases (7–9) write rows the current
CHECKs/validation reject, and Phase 4 consumes Phase 3's shared
system-category guard. The transaction-service core precedes its three
consumers — TUI dialogs, CLI flags, scheduled posting — and split-line
mirroring precedes the scheduled multi-line and loan phases that post
through it. The loan wizard composes everything and lands second-to-last.
Docs + e2e close, per house convention.

## Phase 1: Spending-Report Guards + Opt-In Toggle — [x]

Fixes the pre-existing leak (categorized `transfer link` pairs count as
spending today) and lands the toggle end-to-end.

- [x] `internal/report/report_service.go`: add `includeTransfers bool` to
  `SpendingByCategoryMonth` (:166), `SpendingByCategoryYear` (:175),
  `SpendingByCategoryDateRange` (:184), and `spendingByCategory` (:190).
  When false, append `AND t.transfer_id IS NULL` to the transactions arm
  (after :213) and `AND ts.transfer_account_id IS NULL` to the splits arm
  (after :237); when true, keep today's query text.
- [x] `internal/cli/report/spending.go`: `--include-transfers` bool flag
  (:47-50 block), threaded to the three service calls (:76, :81, :101).
- [x] `internal/tui/reports_view.go`: `includeTransfers` field on
  `reportsViewData` (:25-31, session-only); `t` key in `handleReportsKeys`
  (:77-119) flips it and reloads spending via `loadReportsViewData`;
  spending title gains an `(incl. transfers)` suffix and the spending
  footer (:319-323) gains the `t` hint.
- [x] `internal/tui/help_overlay.go`: add `t` to `reportsShortcuts()`
  (:127-139).
- [x] Tests: report-service tests seeding a categorized whole-transfer
  pair (built via `SetTransfer`+`SetCategory` to simulate legacy linked
  data) — excluded with the flag false (leak regression), included exactly
  once (outflow leg only) with it true; income-typed transfer category
  never appears; CLI flag test through `runReportSpending`; TUI toggle
  test flipping state and asserting the reload + title suffix; TUI
  session-state test pinning includeTransfers carry-forward across
  s/y/n/left/right and reset-to-false on a fresh menu entry
  (mutation-verified). (The splits-arm inclusion test needs a categorized
  transfer split-line, a row shape the current CHECK rejects — it lands
  with Phase 3's migration; the splits-arm guard is a provable no-op until
  then, since `transaction_splits`' XOR CHECK forbids a both-set row.)

## Phase 2: `ReplaceSplits` Transfer-Awareness (pre-existing corruption fix) — [x]

Confirmed by runtime probe (2026-07-03): `ReplaceSplits`
(`internal/transaction/transaction_service.go:800-840`) — the path behind
every TUI split edit (`submitSplitDialog` →
`undo.EditTransactionWithSplitsCommand`,
`internal/undo/transaction.go:474-498`) and `VoidTransactionCommand.Undo`
(:198-199) — deletes all splits and recreates the caller's rows with none
of the transfer behaviors its siblings have. Observed: a TUI-shaped
replace of a split set containing a transfer line fails the pairing CHECK
**mid-flight and non-atomically** (parent left with a partial split set;
counterpart orphaned — the clear step alone orphans it), and with a
preserved transfer_id an amount edit silently desyncs the legs while a
dropped line silently orphans its counterpart. Independent of the feature;
fixable against today's schema.

- [x] `internal/transaction/transaction_service.go`: `ReplaceSplits` now
  diffs old-vs-new transfer lines (`planSplitReplacement`) — mints
  `TransferID` for added rows, deletes counterparts for removed lines
  (`deletePairedCounterTransaction`), creates counterparts for added lines
  (`createTransferLineCounterpart`), and mirrors amount edits onto retained
  lines' counterparts (`updatePairedAmount`). A `preflightSplitReplacement`
  pass runs every fallible counterpart op (reconciled check via new
  `ensureCounterpartNotReconciled`; routability via extracted
  `ensureTransferTargetRoutable`) **before** any mutation, so a reconciled
  or unroutable counterpart fails cleanly with no partial write. Rollback of
  added counterparts shares the new `rollbackTransferLinePairs` helper.
- [x] Handle the TUI calling convention: `buildSplits` emits transfer rows
  with `TransferID` unset even for retained lines — matched to existing
  lines first by `transfer_id` (void-undo replay), then by target account
  (TUI edit); unmatched new → added (mint + create), unmatched old →
  removed (delete). `buildSplits` itself needs no change.
- [x] **Collapsed the now-vestigial clear-first dance** in
  `EditTransactionWithSplitsCommand` (`internal/undo/transaction.go`,
  Execute + Undo). The "clear splits before updating the parent" step existed
  only for a pre-026 DuckDB FK-on-rewrite error; migration 026 dropped
  `transaction_splits`' inbound FK (runtime-verified 2026-07-03), so the
  parent updates fine with children present. Removing the intermediate
  `ReplaceSplits(id, nil)` is what lets `ReplaceSplits` see the old rows and
  **preserve** each retained transfer line's counterpart identity (and its
  cleared/reconciled status) instead of churning it — and makes reconciled
  blocking fire only when the transfer line is actually mutated, not on any
  edit of the parent.
- [x] Tests: probe scenarios pinned as regressions —
  `internal/transaction/replace_splits_transfer_test.go` (TUI-shaped replace
  no longer errors/orphans; amount edit cascades; dropped deletes
  counterpart; added mints one; target-change moves it; investment-target
  variants via the adapter; reconciled bank/investment counterpart blocks,
  and an unrelated edit is *allowed* when the transfer line is untouched) and
  `internal/undo/transaction_replace_splits_test.go` (edit + undo preserves
  counterpart identity; void + undo restores a transfer-line split set
  intact).

## Phase 3: Migration 029 + Validation Relaxation — [x]

- [x] `internal/db/migrations/029_transfer_categories.sql`: backup-drop-
  recreate (026 recipe) for **transaction_splits** — relaxed
  `CHECK (category_id IS NOT NULL OR transfer_account_id IS NOT NULL)`,
  pairing CHECK kept, FK/index decisions copied verbatim from 026:27-55 —
  and for **scheduled_split_items** against the 028 definition (28:33-49;
  keep the section enum CHECKs, the at-most-one-section CHECK, categories
  FK, parent index; no transfer_id column exists there — loan_section is
  copied through, not NULLed). Recreate the `category_spending` view
  (019:278-288) with `AND t.transfer_id IS NULL` in the LEFT JOIN.
- [x] `internal/db/migration.go:16`: `CurrentSchemaVersion` 28 → 29.
- [x] `internal/transaction/transaction.go`: `Split.Validate` —
  removed the both-set error; kept neither-set, pairing, amount, memo
  rules. Updated the struct/shape doc comments.
- [x] `internal/scheduled/split_item.go`: same relaxation; updated
  the header comment.
- [x] `internal/scheduled/scheduled.go`: dropped the transfer+category
  error in `Transaction.Validate`; `SetTransfer` no longer clears
  `CategoryID`; updated the field/method doc comments.
- [x] `internal/transaction/split_repository.go` and
  `internal/scheduled/split_repository.go`: `verifyReferences`
  transfer branch now falls through to verify the category when set (no
  more short-circuit past the category check).
- [x] New shared guard where transfer categories are assigned:
  `ValidateTransferCategory` + `SystemCategoryTransferError` in
  `internal/transaction/transfer_category.go` (non-system rule on a
  resolved `*category.Category`; existence enforced by the FK +
  `verifyReferences`). Reusable by scheduled/loan/CLI (all can import
  `internal/transaction`); consumed in Phase 4+.
- [x] Tests (`internal/db/migration_test.go`, 014/028 subtest pattern):
  accepted shapes now include category+transfer on both tables (inverted
  the 014-era transaction_splits and 015-era scheduled_split_items
  both-set-rejection subtests to acceptance); neither-set still rejected;
  transfer_account_id-without-transfer_id still rejected;
  `TestCurrentSchemaVersion` passes at 29; new `TestMigration029Transfer
  Categories` pins the `category_spending` view transfer guard. Model
  tests for each relaxed `Validate` + `SetTransfer` non-clobber +
  `ValidateTransferCategory` system-category rejection. Report-service
  splits-arm test (deferred from Phase 1): a categorized transfer
  split-line is excluded by default and included exactly once with the
  toggle.

## Phase 4: Transfer Service Core + Link Adoption — [x]

- [x] `internal/transaction/transaction_service.go`:
  `CreateTransfer(from, to, date, amount, memo string, categoryID
  types.NullableID)` — memo + category set on both legs after
  construction; rejects system + nonexistent categories via a new
  `validateTransferCategory` helper (raw `system_category` lookup on
  `s.db`, delegating the non-system rule to `ValidateTransferCategory`).
- [x] `internal/transaction/transaction_service.go`:
  `UpdateTransfer(transferID, date, amount, memo, status, categoryID
  types.NullableID)` — mirrors category to both legs (`Valid:false`
  clears both); doubles as the legacy-divergent-pair healing path.
- [x] Retired the create-then-update-memo workaround at its four sites:
  `internal/tui/transfer_dialog.go` (create path),
  `internal/cli/transfer/add.go` (`dispatchTransferAdd` reg↔reg),
  `internal/scheduled/scheduled_service.go` (auto-post inline transfer)
  and `postSingleLineTransfer` — all pass memo (and, this phase, an empty
  category) directly to `CreateTransfer`.
- [x] `internal/investment/investment_service.go`: optional category on the
  regular-side leg for `DepositFromAccount` and `TransferCash`; threaded
  through `UpdateTransferCash` (`internal/investment/update_edit.go`, its
  delegating TransferCash/DepositFromAccount calls) for the inv↔reg
  branch. `TransferCashBetweenInvestments` untouched.
- [x] `internal/undo/transaction.go`: threaded category through
  `CreateTransferCommand` (+ memo) and `EditTransferCommand`
  (+ `beforeCategory` capture from the outflow leg);
  `internal/undo/scheduled_transaction.go` (`PostScheduledTransferCommand`)
  and `internal/undo/investment_transfer.go` reg↔inv create commands gained
  the parameter. `DeleteTransferCommand`/`VoidTransferCommand`: no change.
- [x] `internal/transferlink/transferlink.go`: adoption rule in
  `linkOne` — one-leg → mirror; both-differ → outflow (`c.From`) wins;
  both-same/empty → unchanged; the error rollback restores both legs'
  original categories alongside `ClearTransfer`.
- [x] CLI/TUI edit paths preserve an existing category (they gain no
  category picker until later phases): `resolvedTransfer.categoryID` is
  read from the loaded leg and threaded back through `dispatchTransferEdit`;
  the TUI edit passes `regularPair.FromTransaction.CategoryID`.
- [x] Tests: pair create/edit/clear mirroring (transaction);
  system + nonexistent category rejection; legacy divergent pair healed by
  `UpdateTransfer`; undo round-trips category (create + delete-undo, edit +
  before-category restore-on-undo); reg→inv and inv→reg carry the category
  on the bank leg only (investment); link adoption — one-leg (both
  directions), both-differ (outflow wins), both-same, both-empty, rollback
  restores. (inv→inv rejection has no Phase 4 service code path —
  `TransferCashBetweenInvestments` takes no category param; it is a Phase 5
  dialog-validation concern.)

## Phase 5: TUI Transfer Dialogs — [x]

- [x] `internal/tui/transfer_dialog.go`: Category combo (+`AddNewLabel`) in
  create mode after Memo — From/To/Amount/Date/Memo/Category (create submit
  reads category at index 5, guarded so a legacy 5-field dialog degrades to
  no category); edit mode Amount/Date/Memo/Category/Status via a shared
  `editTransferIncludesCategory` predicate (index-aware submit), **omitted for
  inv→inv**; seed edit from the outflow/regular-side leg; `investmentTransferEdit`
  gains `categoryID`, and the direct `UpdateTransferCash` call threads it.
  New helpers: `editTransferIncludesCategory`, `categoryComboIndex`,
  `transferCategoryFieldIndex`.
- [x] Category options loading in `loadTransferDialogData` and **both** edit
  loaders — `loadEditTransferDialogData` (bank↔bank + the counterpart-is-
  investment branch, seeding `categoryID` from the bank leg `txn`) and
  `loadEditInvestmentTransferDialogData` (inv↔reg seeds the regular leg's
  category via the new `transaction.Service.ListByTransferID`; inv↔inv leaves
  it zero). `app_update.go` `transferDialogDataMsg` handler computes the
  combo options + parallel `transferDialogCategoryIDs` and resolves the edit
  seed index.
- [x] Inline-creation plumbing: new `createCatSourceTransferDialog` enum entry,
  router case in `applyCreatedCategory` → `applyCreatedCategoryToTransfer`,
  cancel-restore case in `cancelCreateCatDialog`, `parentsForCreateCatDialog`
  case, and `DialogActionAddNew` handling in `handleTransferDialogKey`
  **and** the transfer dialog's mouse path (`app_mouse.go`). The
  create-category sub-dialog defaults to Expense (transfers carry no
  income/expense signal in their always-positive amount).
- [x] inv→inv submit with a category selected → validation message naming
  the limitation (dialog stays open, inline error on the Category field).
- [x] Tests (`transfer_dialog_category_test.go` + updated
  `transfer_dialog_test.go`): dialog-build field lists per combo (create 6
  fields; bank↔bank/inv↔reg edit 5; inv↔inv edit 4); `editTransferIncludesCategory`
  / `categoryComboIndex` / `transferCategoryFieldIndex` unit tests; DB-backed
  end-to-end create threads category to both legs; DB-backed edit sets then
  clears the category on both legs (mirror + seed round-trip); inv→inv
  rejection (and the no-category variant still submits); inline creation
  open/cancel/apply from the transfer dialog.

## Phase 6: CLI Surface + Output/Export Fixes — [x]

- [x] `internal/cli/transfer/category.go` (new): shared `resolveTransferCategory`
  helper — parses a `Parent`/`Parent:Subcategory` path, looks up an **existing**
  category only (no auto-create, unlike loan add's `getOrCreateCategoryPath`),
  and rejects system categories via `transaction.ValidateTransferCategory`; an
  empty/whitespace path resolves to a cleared `NullableID`.
- [x] `internal/cli/transfer/add.go`: `--category <path>` threaded through
  `dispatchTransferAdd` to all three category-bearing legs (reg↔reg
  `CreateTransfer`, reg→inv `DepositFromAccount`, inv→reg `TransferCash`);
  inv→inv is rejected up front in `runTransferAdd` (and defensively in
  dispatch). Prints a `Category:` line on success.
- [x] `internal/cli/transfer/edit.go`: `--category <path>` with
  only-supplied-flags semantics (`Flags().Changed`); explicit `--category ""`
  clears both legs; unset preserves the existing category. **inv→inv edit now
  rejects `--category`** (a review-found gap: the old path silently dropped it
  in `UpdateTransferCash`'s inv↔inv branch and falsely printed success).
- [x] `internal/cli/transfer/resolve.go`: **review-found HIGH fix** —
  `resolveFromInvestmentLeg` now reads the regular-side counterpart's category
  (new `findRegularLeg`) so editing an inv↔reg transfer via its *investment*-leg
  id no longer silently wipes the category on an unrelated edit.
- [x] `internal/cli/transaction/format.go:51-55,124-128`: the transfer override
  keeps `[Transfer]` in the payee column but no longer forces the category cell
  to `-` — a categorized transfer shows its category in `transaction list` /
  `transaction search`.
- [x] Export regression tests (no code change needed):
  `internal/imexport/export_service_test.go` — CSV emits Category + Transfer
  Account for a categorized header transfer and Category for a categorized split
  transfer-line; QIF emits `L[Account]` (transfer wins, category dropped) —
  pins the documented lossiness (qif.go:334-343).
- [x] Tests: white-box `resolveTransferCategory` unit tests (empty/parent/
  subcategory/unknown/unknown-subcategory/system-rejected); end-to-end CLI
  add/edit/clear against a real file (reg↔reg **and** reg→inv/inv→reg bank-leg
  threading); unknown + system categories rejected; inv→inv add & edit
  rejection; category preserved on an investment-leg edit; `format` output
  shows a categorized transfer's category (list + search) and hides it when
  absent; `transaction search --category` matches a categorized transfer.

## Phase 7: Transfer-Line Mirroring + Split-Dialog Carry-Through — [x]

- [x] `internal/transaction/transaction_service.go`
  (`createTransferLineCounterpart`): copies the split's category onto the
  bank-side paired row (`paired.SetCategory` when `!split.CategoryID.IsNil()`);
  investment adapter path unchanged — the split line holds it alone.
- [x] `UpdateSplit` retained same-target branch now mirrors an amount **or**
  category change onto the counterpart via the generalized
  `mirrorToPairedCounterpart` (renamed from `updatePairedAmount`; sets amount
  always + category on a regular-side counterpart, reconciled blocks).
  `moveTransferLine` carries the category onto the re-minted counterpart for
  free through `createTransferLineCounterpart`. `ReplaceSplits`: the diff
  (`planSplitReplacement`) now records `retainedChanged` on amount **or**
  category change (was `retainedAmountChanged`); `preflightSplitReplacement`
  and the re-sync loop use `mirrorToPairedCounterpart`; added lines mint a
  categorized counterpart. New `splitCategoryNullable` helper converts a
  split's plain `CategoryID` to a `NullableID`.
- [x] `Duplicate` refuses a split parent containing any transfer line up front
  (splits loaded before creating the duplicate) with a new
  `CannotDuplicateSplitTransferError`; plain split parents still duplicate.
- [x] `internal/tui/split_dialog.go`: `splitRow.seedTransferCategoryID` stores a
  seeded transfer row's category; `NewSplitDialogFromExisting` seeds it and
  `buildSplits` re-emits it instead of hardcoding `NilID`; a fresh transfer
  row carries no category. Render marker deferred (v1 non-goal; the cell is
  width-constrained with mouse hit-testing and there is no picker).
- [x] `internal/tui/scheduled_dialog.go`: extracted
  `scheduledSplitsFromTransaction` (inverse of `transactionSplitsFromScheduled`)
  which carries a template transfer line's category through the Edit Series
  round-trip; `submitScheduledSplitDialog` now calls it.
- [x] Tests: `internal/transaction/transfer_line_category_test.go` (counterpart
  carries category at create/edit/move, ReplaceSplits retained/category-only/
  added round-trips, reconciled blocks a category-only change, delete cascade,
  investment-target line holds category with no regular counterpart, Duplicate
  refusal + plain-split still-works); `internal/undo/transaction_replace_splits_test.go`
  (void+undo preserves the categorized transfer line and its counterpart);
  `internal/tui/split_dialog_transfer_category_test.go` (split-dialog carry-through,
  fresh row no category, `scheduledSplitsFromTransaction` round-trip).

## Phase 8: Scheduled Transfers End-to-End — [x]

- [x] `internal/tui/scheduled_transfer_dialog.go`: Category combo after
  Amount (renumbered the `schedXferField*` constants — Category=3, Memo→4 …
  Count→13); edit mode seeds from `st.CategoryID`; `submitScheduledTransferDialog`
  drops the unconditional `st.ClearCategory()` and instead sets/clears from
  the field on both create and edit paths (index 0 "(None)" = clear). Category
  options are loaded (non-system only, via `buildCategoryOptions`) in the
  `scheduledDialogDataMsg` transfer branch of `app_update.go` — matching how the
  regular scheduled dialog loads them — and stashed on
  `schedDialogCategoryIDs`/`schedDialogCategoryOptions`. Inline-creation plumbing:
  new `createCatSourceSchedTransferDialog` source, `openCreateCategorySubDialogFromSchedTransfer`
  / `applyCreatedCategoryToSchedTransfer` (Expense default — a transfer's
  positive magnitude carries no income/expense signal), cancel-restore case
  grouped with `createCatSourceSchedDialog`, router dispatch, and **key + mouse**
  AddNew handling (added the previously-absent `DialogActionAddNew` case to the
  `app_mouse.go` scheduled-dialog branch, gated on `isTransfer`).
- [x] `internal/scheduled/scheduled_service.go`: `postSingleLineTransfer`
  and the AutoPost inline transfer branch pass `st.CategoryID` to
  `CreateTransfer`, so both posted legs inherit the schedule's label.
- [x] `internal/tui/scheduled_preview_dialog.go`: single-line transfer
  preview header (`buildPreviewHeaderTransfer`) gains a Category combo
  (renumbered `previewXferField*` — Category=2, Memo→3, Status→4), seeded
  from the template's category; `submitSchedulePreviewTransfer` threads the
  one-off category into `PostScheduledTransferCommand` (which applies it via
  `UpdateTransfer` after `PostWithDate`, so the template is untouched and
  "(None)" clears both legs). New `categoryFieldIndex` helper makes the
  preview's inline-creation divert (`openCreateCategorySubDialogFromSchedPreview`
  / `applyCreatedCategoryToSchedPreview`) shape-aware (transfer vs single-line);
  `loadSchedulePreviewData` suppresses Value Adjustment for transfer previews.
- [x] `internal/scheduled/scheduled_service.go` (`buildMultiLineTransaction`):
  copies a template transfer line's category onto the posted transfer-line
  split; counterpart mirroring from Phase 7 (`createTransferLineCounterpart`)
  carries it to the bank-side paired row.
- [x] Tests: `internal/scheduled/scheduled_transfer_category_test.go`
  (manual-post + auto-post mirror the category onto both legs; uncategorized
  transfer stays category-free; `buildMultiLineTransaction` unit + full-post
  end-to-end carries a categorized transfer line to the split **and** the
  minted counterpart) and `internal/tui/scheduled_transfer_dialog_category_test.go`
  (data-msg builds the Category combo; create threads the category; edit clears
  it; inline-creation divert open/apply; preview seeds from template; preview
  one-off relabel posts the new category while the template keeps its original).

## Phase 9: Loan Wizard Integration — [ ]

- [ ] `internal/category/category.go:106-112` +
  `category_service.go:308-331`: `LoanPrincipalChildName = "Principal"` and
  `GetOrCreateLoanPrincipalCategory()` beside the Interest twin.
- [ ] `internal/scheduled/loan_build.go`: `PrincipalCatID` on
  `LoanSnapshotInput` (:24-31), set on the principal line (:76-78).
- [ ] `internal/scheduled/loan_posting.go:124-128`: posted principal line
  copies the template principal line's category (as :122 does for
  interest).
- [ ] `internal/cli/loan/add.go`: `--principal-category` — omitted →
  `GetOrCreateLoanPrincipalCategory`; explicit path →
  `getOrCreateCategoryPath` (:322-342); explicit `""` → none (detect via
  `Flags().Changed`); resolution block beside :235-249.
- [ ] `internal/tui/loan_wizard.go`: *Principal category* field (prefilled
  `Loan:Principal`, clearable, inline creation) on create; edit wizard
  prefills from the existing principal line (`prefillLoanPaymentFields`
  :307-340 currently skips the transfer line's category) so **Edit as
  loan →** round-trips it through `submitEditLoanWizard`'s wholesale
  rebuild (:1104-1126).
- [ ] Regression tests: categorized principal line keeps `IsLoanShaped` /
  `IsLoanAdoptable` / `FindLoanSchedule` / demotion guard behavior
  identical (loan_shape.go:118-121 never reads it); amortization view and
  `loan list/show` unaffected; recompute-at-post preserves the label
  across an extra-principal payment and an APR edit; 0% loan books
  principal with label and no interest line; `--principal-category ""`
  and cleared wizard field produce a bare transfer line (old shape).

## Phase 10: Docs + End-to-End Verification — [ ]

- [ ] `README.md`: Transactions transfer bullet (:66-69) + inline-creation
  surface list (:84-88); Scheduled transfers bullet (:118-124); Loans
  split description (:159-162, `Loan:Principal`); Reports (:187, toggle);
  CLI Reference — `transfer add/edit` (:550-563), `report spending`
  (:714-721), `loan add` (:1279-1296).
- [ ] Cross-edit invalidated claims: `specs/transactions.md:76,129-133,
  139,145`; `specs/multiline-splits-and-paycheck.md:85-87,103,141,
  196-198,409-410`; `specs/scheduled-transactions.md:18-19,126-128,
  143-144`; `specs/loan-wizard.md:24-29` + forward-compat note :620-627
  (now points here); `specs/categories.md` Transfer-category section;
  `specs/reports.md:154,236,249-261`; `specs/import-export.md:271` (QIF L
  lossiness note); `specs/tui.md:557`; `specs/cli.md` (`transfer add`
  :1269-1273, `report spending` :903-907, `loan` :576-582). Note
  `specs/database.md:192-199` is already stale (pre-014) — annotate or
  refresh the split-table DDL while touching it.
- [ ] Cross-link this spec from `specs/transactions.md` and
  `specs/scheduled-transactions.md` "related specs" lists.
- [ ] Scripted CLI smoke on a scratch `.tdb`: create accounts → categorized
  `transfer add` → `transaction list` shows the label → `report spending`
  excludes → `--include-transfers` includes once → `transfer edit
  --category ""` clears both legs → `loan add` (default principal
  category) → post one payment → loan register shows `Loan:Principal` →
  amortization view unaffected → CSV export carries the columns → QIF
  emits `L[Account]`.
- [ ] `git status` sweep before staging; purge any `zz_`/`probe_` files;
  never `git add -A`.

## Out of Scope (tracked for later)

- Split-dialog category **picker** for transfer lines. Recorded UI options
  from review: a 4th column (feasible; costs width from the 19/14/23
  layout, touches render math, mouse mapping, and the 3-cell focus enum)
  vs. an after-picker sub-dialog divert (precedent: the create-category
  divert, `split_dialog.go:994-1081`). Revisit if hand-built categorized
  transfer lines become a real workflow.
- Paycheck wizard per-line category slot (and its Edit-as-paycheck
  round-trip, which would currently drop one).
- Per-leg categories; inv→inv categories; TUI toggle persistence.
- CLI creation of scheduled transfers (`scheduled add` is single-line
  categorized only — unrelated gap this feature doesn't widen).
- Pre-existing category-maintenance gaps: merge skips
  `scheduled_split_items.category_id`
  (`category_service.go:527-564`) and delete's in-use check reads only
  `transactions.category_id` (`category_repository.go:318-351`) — equally
  reachable today via paycheck/loan template lines.
- Search over split-line categories (search never joins
  `transaction_splits`).
