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

## Phase 3: Migration 029 + Validation Relaxation — [ ]

- [ ] `internal/db/migrations/029_transfer_categories.sql`: backup-drop-
  recreate (026 recipe) for **transaction_splits** — relaxed
  `CHECK (category_id IS NOT NULL OR transfer_account_id IS NOT NULL)`,
  pairing CHECK kept, FK/index decisions copied verbatim from 026:27-55 —
  and for **scheduled_split_items** against the 028 definition (28:33-49;
  keep the section enum CHECKs, the at-most-one-section CHECK, categories
  FK, parent index; no transfer_id column exists there). Recreate the
  `category_spending` view (019:278-288) with `AND t.transfer_id IS NULL`.
- [ ] `internal/db/migration.go:16`: `CurrentSchemaVersion` 28 → 29.
- [ ] `internal/transaction/transaction.go:429-430`: `Split.Validate` —
  remove the both-set error; keep neither-set, pairing, amount, memo
  rules. Update the struct/shape doc comments (:353-358, :414-420).
- [ ] `internal/scheduled/split_item.go:101-107`: same relaxation; update
  the header comment (:11-16).
- [ ] `internal/scheduled/scheduled.go:712-714`: drop the transfer+category
  error; `:228-232`: `SetTransfer` stops clearing `CategoryID`; update doc
  comment (:132-135).
- [ ] `internal/transaction/split_repository.go:39-53` and
  `internal/scheduled/split_repository.go:48-63`: `verifyReferences`
  transfer branch also verifies the category when set (no more
  short-circuit past the category check).
- [ ] New shared guard where transfer categories are assigned: category
  exists and `!IsSystem` (service-level helper in `internal/transaction`,
  reused by scheduled/loan validation).
- [ ] Tests (`internal/db/migration_test.go`, 014/028 subtest pattern):
  accepted shapes now include category+transfer on both tables;
  neither-set still rejected; transfer_account_id-without-transfer_id
  still rejected on transaction_splits; **invert both legacy both-set-
  rejection subtests** — the 014-era transaction_splits one
  (migration_test.go:2718-2771) and the 015-era scheduled_split_items one
  (:3078-3132); existing rows survive reopen; `TestCurrentSchemaVersion`.
  Model tests for each relaxed `Validate` + `SetTransfer` non-clobber +
  system-category rejection. Report-service splits-arm test deferred from
  Phase 1: a categorized transfer split-line is excluded by default and
  included exactly once with the toggle.

## Phase 4: Transfer Service Core + Link Adoption — [ ]

- [ ] `internal/transaction/transaction_service.go:874-905`:
  `CreateTransfer(from, to, date, amount, memo string, categoryID
  types.NullableID)` — memo + category set on both legs at construction
  (via `NewTransferPair` or immediately after). Reject system categories.
- [ ] `internal/transaction/transaction_service.go:924-971`:
  `UpdateTransfer(transferID, date, amount, memo, status, categoryID
  types.NullableID)` — mirror category to both legs (`Valid:false`
  clears both).
- [ ] Retire the create-then-update-memo workaround at its four sites:
  `internal/tui/transfer_dialog.go:538-547`,
  `internal/cli/transfer/add.go:132-138`,
  `internal/scheduled/scheduled_service.go:274-283` (auto-post) and
  `:582-595` (`postSingleLineTransfer`) — all pass memo (and, this phase,
  a not-yet-user-settable empty category) directly.
- [ ] `internal/investment/investment_service.go`: optional category on the
  regular-side leg for `DepositFromAccount` (:1124-1130) and
  `TransferCash` (:1044-1049); thread it through `UpdateTransferCash`
  (`internal/investment/update_edit.go:527-639`, including its delegating
  calls to TransferCash/DepositFromAccount at :623/:625) for edits.
  `TransferCashBetweenInvestments` untouched.
- [ ] `internal/undo/transaction.go`: thread category through
  `CreateTransferCommand` (:215-256) and `EditTransferCommand`
  (:390-445, + `beforeCategory` capture at :424-430);
  `internal/undo/scheduled_transaction.go:223-305`
  (`PostScheduledTransferCommand`) and
  `internal/undo/investment_transfer.go` reg↔inv create commands gain the
  parameter. `DeleteTransferCommand`/`VoidTransferCommand`: no change
  (verified in spec).
- [ ] `internal/transferlink/transferlink.go:189-213`: adoption rule in
  `linkOne` — one-leg → mirror; both-differ → outflow (`c.From`) wins;
  extend the error rollback (:206-210) to restore both legs' original
  categories.
- [ ] Tests: pair create/edit/clear mirroring (both legs equal after every
  write); undo/redo round-trips category (create, edit incl.
  before-category, delete-recreate, void-restore untouched); reg→inv and
  inv→reg carry category on the bank leg only; inv→inv rejects a
  category; link adoption — one-leg, both-differ (outflow wins),
  both-same, rollback restores; legacy divergent pair healed by
  `UpdateTransfer`.

## Phase 5: TUI Transfer Dialogs — [ ]

- [ ] `internal/tui/transfer_dialog.go`: Category combo (+`AddNewLabel`) in
  create mode after Memo — From/To/Amount/Date/Memo/Category (hard-coded
  submit indices :442-503 and `len` guards updated); edit mode
  Amount/Date/Memo/Category/Status, omitted for inv→inv; seed edit from the
  outflow leg; `investmentTransferEdit` payload (:80-88) + the direct
  `UpdateTransferCash` call (:743-760) gain category.
- [ ] Category options loading in `loadTransferDialogData` (:181-198) and
  **both** edit loaders — `loadEditTransferDialogData` (:209-291, the
  bank↔bank path) and `loadEditInvestmentTransferDialogData` (:302-355).
- [ ] Inline-creation plumbing: new `createCategorySource` entry
  (`internal/tui/create_category_dialog.go:20-28`), applier
  (:280-298) + cancel-restore cases, `DialogActionAddNew` handling in
  `handleTransferDialogKey` (transfer_dialog.go:420-427) **and** the
  transfer dialog's mouse path (`internal/tui/app_mouse.go:322-332`).
- [ ] inv→inv submit with a category selected → validation message naming
  the limitation.
- [ ] Tests: dialog-build field lists per combo; submit threads category
  through `CreateTransferCommand`/`EditTransferCommand`/
  `UpdateTransferCash`; inline creation from the transfer dialog; editing
  a `transfer_cash` row from the investment register shows and saves the
  bank-leg category; inv→inv rejection.

## Phase 6: CLI Surface + Output/Export Fixes — [ ]

- [ ] `internal/cli/transfer/add.go`: `--category <path>` — resolves an
  existing non-system category (no auto-create), passed to
  `CreateTransfer`.
- [ ] `internal/cli/transfer/edit.go`: `--category <path>` with
  only-supplied-flags semantics; explicit `--category ""` clears
  (use `Flags().Changed`).
- [ ] `internal/cli/transaction/format.go:52-55,125-128`: transfer override
  keeps `[Transfer]` in the payee column but shows the resolved category
  when present instead of forcing `-`.
- [ ] Export regression tests (no code change expected):
  `internal/imexport` — CSV emits Category + Transfer Account for a
  categorized header transfer and Category for a categorized split
  transfer-line; QIF emits `L[Account]` (transfer wins, category dropped)
  — pins the documented lossiness (qif.go:334-343).
- [ ] Tests: add/edit/clear via CLI against a real file; unknown and
  system categories rejected with clear errors; list/search output shows
  the category; search `--category` matches a categorized transfer.

## Phase 7: Transfer-Line Mirroring + Split-Dialog Carry-Through — [ ]

- [ ] `internal/transaction/transaction_service.go:481-523`
  (`createTransferLineCounterpart`): copy the split's category onto the
  bank-side paired row (investment adapter path unchanged — split line
  holds it alone).
- [ ] `UpdateSplit` (:628-684): category change on a transfer line mirrors
  to the bank-side counterpart (alongside the existing amount cascade);
  `moveTransferLine` (:691-707) carries category onto the re-minted
  counterpart; `ReplaceSplits` (fixed in Phase 2) mirrors category through
  its counterpart create/update reconciliation.
- [ ] `internal/transaction/transaction_service.go:1379-1410`: extend the
  `Duplicate` guard to reject split parents containing transfer lines
  (error mirroring `CannotDuplicateTransferError`). Today the split-copy
  loop drops transfer linkage and fails loudly on `verifyReferences`;
  after Phase 3's relaxation a *categorized* transfer line would duplicate
  silently into a plain categorized split with no counterpart.
- [ ] `internal/tui/split_dialog.go`: `splitRow` stores a seeded transfer
  row's category; `buildSplits` (:452-465) re-emits it instead of
  hardcoding `NilID`; optional compact render marker in the
  `Transfer → <account>` cell (:739-743) when space allows. No picker (v1
  non-goal).
- [ ] `internal/tui/scheduled_dialog.go:946-962`: Edit Series split editor
  re-emits template transfer-line categories (same carry-through).
- [ ] Tests: counterpart carries category at create/edit/move and through
  a `ReplaceSplits` round-trip; delete/void cascades unaffected; a
  categorized transfer line survives a split-dialog round-trip and an
  Edit Series round-trip unchanged; duplicating a split parent with a
  transfer line errors.

## Phase 8: Scheduled Transfers End-to-End — [ ]

- [ ] `internal/tui/scheduled_transfer_dialog.go`: Category combo after
  Amount (renumber the `schedXferField*` constants :20-34); loader
  (:172-187) fetches categories; edit mode seeds from the schedule; submit
  stops calling `st.ClearCategory()` (:306) and sets/clears from the
  field; inline-creation plumbing (source entry + key **and mouse**
  AddNew handling — the scheduled-dialog mouse path currently lacks it,
  `app_mouse.go:334-348`).
- [ ] `internal/scheduled/scheduled_service.go`: `postSingleLineTransfer`
  (:567-603) and the AutoPost inline transfer branch (:267-283) pass
  `st.CategoryID` to `CreateTransfer`.
- [ ] `internal/tui/scheduled_preview_dialog.go`: single-line transfer
  preview header (`buildPreviewHeaderTransfer` :361-377) gains a Category
  combo (renumber `previewXferField*` :45-48) — one-off relabel at post
  time, template untouched; `submitSchedulePreviewTransfer` (:309-356) and
  `PostScheduledTransferCommand` thread it.
- [ ] `internal/scheduled/scheduled_service.go:672-679`
  (`buildMultiLineTransaction`): copy a template transfer line's category
  onto the posted split (counterpart mirroring from Phase 7 takes over).
- [ ] Tests: schedule create/edit round-trips category; all three posting
  paths (manual, auto-post, preview incl. one-off override) produce a
  mirrored pair; multi-line template with a categorized transfer line
  posts through to split + counterpart; preview embedded editor preserves
  it.

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
