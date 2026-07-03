# Transfer Categories Specification

**Status**: Draft v1 — design settled in a grill-me review (2026-07-03)
**Depends on**: `specs/transactions.md`, `specs/multiline-splits-and-paycheck.md`, `specs/scheduled-transactions.md`, `specs/loan-wizard.md`, `specs/categories.md`
**Implementation plan**: [`specs/implementation-plan-transfer-categories.md`](implementation-plan-transfer-categories.md)

Transfers gain an **optional category**. A monthly credit-card payment
scheduled as a transfer can be labeled `Bills:Credit Card`; a mortgage
payment's principal transfer line can be labeled `Loan:Principal` alongside
its already-categorized `Loan:Interest` line. The category is purely a label
for tracking *why* money moved — it never changes balance math, transfer
linkage, or loan-shape detection — and it is **never required**. Reporting
is opt-in: the spending-by-category report keeps today's transfer-free
semantics by default and folds categorized transfers in only when asked
(`report spending --include-transfers` on the CLI, `t` on the TUI Reports
view).

This spec also fixes a latent leak it would otherwise widen: transfer rows
have always been excluded from the spending report *implicitly* — their
`category_id` is NULL, so the category join drops them — and
`transfer link` never clears categories off the imported rows it joins
(`internal/transferlink/transferlink.go:200-205`,
`Transaction.SetTransfer` at `internal/transaction/transaction.go:274-278`).
Linking two categorized imported rows therefore already produces a
categorized transfer pair whose outflow leg counts as spending today. The
new explicit transfer guards in the default report close that hole.

## Overview

What exists today:

- A **whole-transaction transfer** is two `transactions` rows sharing a
  `transfer_id`. The rows physically have a `category_id` column (no DB
  CHECK ties it to the transfer columns — `019_account_type_hsa.sql:81-98`);
  the "transfers carry no category" invariant is app-level convention only.
- A **transfer-line** is one row of a multi-line split
  (`transaction_splits`) with `transfer_account_id` + `transfer_id` set; a
  paired single-line counter-transaction is minted in the target account.
  Here the exclusivity **is** DB-enforced:
  `CHECK ((category_id IS NULL) <> (transfer_account_id IS NULL))`
  (`026_drop_transaction_splits_fk.sql:39`).
- **Scheduled transfers** (single-line, `scheduled_transactions.transfer_account_id`)
  and **scheduled multi-line template lines** (`scheduled_split_items`)
  mirror the same shapes; `scheduled_split_items` carries the same XOR
  CHECK (`028_loan_section.sql:47`) and the parent's exclusivity is
  service-level (`internal/scheduled/scheduled.go:709-726`).

What is new:

1. `category_id` may coexist with the transfer fields on every regular-side
   storage row: `transactions` (validation change only),
   `transaction_splits` and `scheduled_split_items` (CHECK relaxation,
   migration 029), and `scheduled_transactions` (validation change only).
2. One **shared category per transfer**, mirrored onto both legs wherever a
   leg can store it, cascading through edits exactly like amount/date/memo.
3. The default spending report gains **explicit transfer guards**; a new
   opt-in toggle (CLI flag + TUI key) folds categorized transfers in.
4. The loan wizard labels the principal transfer line `Loan:Principal` by
   default (overridable, suppressible), and recompute-at-post preserves it.
5. Category fields (combo with inline `[+ Add new category…]` creation) on
   the Transfer dialog, Edit Transfer dialog, Scheduled Transfer dialog, and
   the single-line transfer post-time preview; `--category` flags on
   `transfer add` / `transfer edit`; `--principal-category` on `loan add`.

## Goals

- Label transfers with any non-system category, at every creation/edit
  surface for whole-transaction and scheduled transfers.
- Keep the default spending report's semantics exactly as today —
  deterministically, via explicit guards rather than the NULL-join accident.
- Give the mortgage use case an end-to-end path: wizard-labeled principal,
  preserved through recompute-at-post, visible in the loan register and in
  the opt-in spending view.
- Zero behavior change for uncategorized transfers.

## Non-Goals (v1)

- **Per-leg (divergent) categories** — one shared category per transfer.
  Legacy divergent pairs created by `transfer link` are tolerated on read
  and healed on the next pair edit (see *Mirroring*).
- **inv→inv transfers** — neither leg lives in the `transactions` table
  (`investment_transactions` has no category column,
  `008_investment_tables.sql:10-32`), so there is nowhere to store one.
- **A category editor for transfer lines inside the generic split dialog.**
  The split dialog *preserves* an existing category on a transfer line
  (carry-through), but offers no in-dialog picker for it in v1 — the
  one-line-per-row layout has no free cell
  (`internal/tui/split_dialog.go:722-763`). The surfaces that actually set
  transfer-line categories (loan wizard) don't need it. See *Split dialog*.
- **Paycheck wizard changes** — net-pay destination lines stay categoryless
  when built by the wizard; the underlying records support a category.
  Caveat: **Edit as paycheck →** rebuilds lines and would drop a category
  added to a destination line by other means (`internal/tui/paycheck_wizard.go:1839-1852`)
  — documented, accepted.
- **Backfilling existing loan schedules** with `Loan:Principal` — new loans
  only; existing schedules pick it up via **Edit as loan →**.
- **An income-by-category report** or any new report view — only the
  existing spending report gains the toggle.
- **Persisting the TUI report toggle** — session-only, matching the reports
  view's period state; the CLI flag is per-invocation.
- **Category as a `transfer link` matching tiebreaker** — linking still
  matches on amount/date/accounts only.

## Data Model

### `transaction_splits` — CHECK relaxation (migration 029)

Replace the XOR CHECK with an at-least-one CHECK; keep the pairing CHECK:

```sql
-- was: CHECK ((category_id IS NULL) <> (transfer_account_id IS NULL))
CHECK (category_id IS NOT NULL OR transfer_account_id IS NOT NULL),
CHECK ((transfer_account_id IS NULL) = (transfer_id IS NULL))
```

Valid row shapes become: categorized, transfer, **categorized transfer**.
A row with neither remains invalid. Both legacy shapes satisfy the new
CHECK by construction, so the data copy is safe.

DuckDB cannot drop or alter an anonymous CHECK constraint
(`019_account_type_hsa.sql:4-8`), so migration 029 uses the established
backup-drop-recreate recipe (`026_drop_transaction_splits_fk.sql:24-55`):
temp backup table → DROP →
CREATE with the relaxed CHECK → `INSERT … SELECT` with explicit column
lists → drop backup → recreate `idx_splits_transaction` and
`idx_splits_transfer`. FK decisions carry over unchanged: no inbound FK on
`transaction_id`, outbound `category_id REFERENCES categories(id)`
preserved, `transfer_account_id`/`transfer_id` plain UUIDs with no FK and
no index on `transfer_account_id` (the UPDATE-as-DELETE+INSERT trap —
`026:4-14`).

### `scheduled_split_items` — same relaxation (migration 029)

Same recipe against the current definition (`028_loan_section.sql:33-49`),
preserving the `paycheck_section`/`loan_section` enum CHECKs, the
at-most-one-section CHECK
(`CHECK (paycheck_section IS NULL OR loan_section IS NULL)` — both NULL is
the normal untagged state), the `categories` FK, the absent
`scheduled_transaction_id` FK, and `idx_scheduled_split_items_parent`.
This table has no `transfer_id` column (pairs are minted at post time), so
only the XOR CHECK changes.

### Header tables — no schema change

`transactions` and `scheduled_transactions` already hold independent
nullable `category_id` and transfer columns with no relating CHECK
(`019:81-98`, `022_scheduled_transfer_account.sql:22`). Coexistence is
enabled purely by relaxing app-level validation (see *Validation*).

### `category_spending` view

The DB view `category_spending` (`019_account_type_hsa.sql:278-288`) joins
`transactions.category_id` with no transfer guard, so categorized transfer
rows would silently enter its totals for anyone querying the `.tdb`
directly. No production Go code reads it — the only Go reference is the
view-exists smoke test at `internal/db/migration_test.go:335-338` (which
still passes after a recreate), and the same-named `category_spending` in
`report_service.go:197` is a query-local CTE, not the view. Migration 029
recreates the view with the same `t.transfer_id IS NULL` guard the report
gains.

### Migration bookkeeping

`029_transfer_categories.sql`; bump `CurrentSchemaVersion` 28 → 29
(`internal/db/migration.go:16`). The migration runs in one transaction like
every other (`migration.go:127-151`).

## Category Semantics

### One shared category, mirrored

A transfer has **at most one category**, stored on every leg that can hold
it and kept identical across legs by the service layer (the same contract
amount/date/memo/status already have — `UpdateTransfer` mirrors to both
legs, `internal/transaction/transaction_service.go:957-968`). Not
DB-enforced; `TransferPair.Validate` does **not** gain a category-equality
rule, because legacy `transfer link` pairs may hold divergent categories —
those are tolerated on read (the outflow leg's category is canonical for
display) and overwritten to a single value on the next pair edit.

### Where the category lives, by transfer shape

| Shape | Storage |
|---|---|
| Whole-transaction reg↔reg | `category_id` on **both** `transactions` legs |
| Whole-transaction reg→inv / inv→reg | `category_id` on the **regular-side** leg only (`investment_transactions` has no column) |
| Whole-transaction inv→inv | none — category unsupported (v1) |
| Transfer-line, regular target | `category_id` on the split line **and** the bank-side paired counterpart row |
| Transfer-line, investment target | `category_id` on the split line only (adapter counterpart can't store it) |
| Scheduled single-line transfer | `category_id` on the `scheduled_transactions` row; flows to both posted legs |
| Scheduled multi-line template transfer line | `category_id` on the `scheduled_split_items` row; flows to the posted split line + counterpart |

### Allowed categories

Any **non-system** category, income- or expense-typed. The system
categories (`Transfer`, `Value Adjustment`) are rejected: pickers already
exclude them, and the service/CLI paths that assign a transfer category
validate `!IsSystem`. Category existence is verified wherever a transfer
category is written, including the previously short-circuited transfer
branch of split `verifyReferences`
(`internal/transaction/split_repository.go:39-53`,
`internal/scheduled/split_repository.go:48-63`).

## Validation Changes

All of these currently enforce the old exclusivity and are relaxed to
"category optional on transfers; at least one of category/transfer
required on split rows":

1. `transaction.Split.Validate` — drop the both-set error
   (`internal/transaction/transaction.go:429-430`); keep neither-set,
   transfer-id pairing, non-zero amount, memo length.
2. `scheduled.Split.Validate` — same
   (`internal/scheduled/split_item.go:101-107`).
3. `scheduled.Transaction.Validate` — drop "a transfer schedule cannot
   also set a category" (`internal/scheduled/scheduled.go:712-714`); a
   transfer schedule still requires an amount and forbids splits/self.
4. `scheduled.Transaction.SetTransfer` — stop clobbering `CategoryID`
   (`internal/scheduled/scheduled.go:228-232`).
5. New rule (all shapes): an assigned transfer category must exist and be
   non-system.

Unchanged: no self-transfers, signed-sum balance, positive pair amounts,
`TransferPair` linkage rules, `transactions`-level `Validate` (already
tolerant, `internal/transaction/transaction.go:313-344`).

## Reporting

### Default: explicit transfer guards (behavior fix)

`spendingByCategory` (`internal/report/report_service.go:196-248`) gains:

- transactions arm: `AND t.transfer_id IS NULL`
- splits arm: `AND ts.transfer_account_id IS NULL`

This is a deliberate, user-visible fix: databases containing categorized
transfer pairs created by `transfer link` had those outflow legs counted as
spending; they no longer are, by default.

### Opt-in: `--include-transfers`

With the toggle on, both guards are dropped — categorized transfers flow
through the existing query mechanics with **no double-count inside a
pair**: only negative-amount rows are summed (`t.amount < 0` /
`ts.amount < 0`), so exactly the outflow leg of a mirrored pair counts.
Only expense-typed categories appear (existing `c.type = 'expense'`
filter); an income-typed category on a transfer never enters the spending
report. Uncategorized transfers stay invisible regardless of the toggle
(the category join still drops them).

The cross-pair double-count is the user's informed choice: turning the
toggle on while categorizing credit-card payment transfers *and*
categorizing the card's purchases counts those dollars twice. That
trade-off is the whole reason the toggle defaults off.

Surfaces:

- **Service**: `SpendingByCategoryMonth/Year/DateRange` and the unexported
  `spendingByCategory` gain an `includeTransfers bool` parameter
  (`report_service.go:166,175,184,190`).
- **CLI**: `tmoney report spending --include-transfers` (all three period
  forms; `internal/cli/report/spending.go`).
- **TUI**: `t` toggles it on the Reports view's spending report (`t` is
  unbound there — `handleReportsKeys`,
  `internal/tui/reports_view.go:77-119`). Session-only state in
  `reportsViewData`; the spending header shows an `(incl. transfers)`
  suffix while on; footer hint and help overlay
  (`internal/tui/help_overlay.go:127-139`) list the key.

Net-worth math is category-blind (`report_service.go:68-163`) and cannot be
affected. No other report reads `category_id`.

## Whole-Transaction Transfers

### Create / edit

`CreateTransfer` gains `memo string` and `categoryID types.NullableID`
parameters (flat accretion, matching `UpdateTransfer`'s existing shape) and
sets both on both legs at construction — this also retires the
"create-then-`UpdateTransfer`-for-memo" workaround duplicated at
`internal/tui/transfer_dialog.go:538-547`, `internal/cli/transfer/add.go:136`,
and `internal/scheduled/scheduled_service.go:278-282,587-595`.
`UpdateTransfer` gains `categoryID types.NullableID`, mirrored to both legs
(clearable — `Valid: false` clears both). Repository SQL needs no change:
INSERT/UPDATE/SELECT already carry `category_id`
(`internal/transaction/transaction_repository.go:86,98,311,320`;
`transfer_repository.go:52,75`).

Investment-involved combos: `DepositFromAccount` (reg→inv) and
`TransferCash` (inv→reg) gain an optional category applied to the
regular-side leg (`internal/investment/investment_service.go:1124-1130,
1044-1049`); `UpdateTransferCash` threads it for edits.
`TransferCashBetweenInvestments` is untouched.

Undo commands thread the new parameters: `CreateTransferCommand`,
`EditTransferCommand` (+ before-category capture),
`PostScheduledTransferCommand`, and the two investment create commands for
reg↔inv. `DeleteTransferCommand` and `VoidTransferCommand` need no changes
— delete-undo recreates full captured legs (category round-trips through
the INSERT) and void never touches category (`transaction_service.go:1265-1271`).

### Void / delete / duplicate / reconcile

Void zeroes amount and stamps `**VOID**` on both legs, leaving the
category in place (void rows are excluded from reports by status). Delete
removes both legs. Reconciliation never reads categories.

`Duplicate` already refuses whole-transaction transfers
(`CannotDuplicateTransferError`, `transaction_service.go:1380-1382`); the
guard is **extended to split parents containing transfer lines**. Its
split-copy loop (:1396-1410) drops transfer linkage — a loud
`verifyReferences` failure today, but after the CHECK relaxation a
*categorized* transfer line would silently degrade into a plain
categorized split with no counterpart (and count as spending). No UI or
CLI invokes `Duplicate` today; the extended guard closes the service-API
hazard before it becomes reachable.

### Transfer link adoption

`transfer link` keeps its matching inputs (amount/date/accounts) but
normalizes categories when joining a pair, in `linkOne`
(`internal/transferlink/transferlink.go:189-213`):

- exactly one leg categorized → mirror it to the other leg;
- both categorized and different → the **outflow** leg's category wins,
  mirrored to both;
- both same or both empty → unchanged.

The error rollback restores the original per-leg categories alongside
`ClearTransfer` (today's rollback restores transfer fields only —
`transferlink.go:206-210`).

## Transfer-Lines (splits)

### Counterpart mirroring and cascades

`createTransferLineCounterpart`
(`internal/transaction/transaction_service.go:481-523`) copies the split
line's category onto the bank-side paired row (investment-side counterparts
can't store one; the split line keeps it alone). Cascade table, extending
`specs/multiline-splits-and-paycheck.md#cascade-rules`:

| Action | Category effect |
|---|---|
| Create transfer-line with category | Counterpart (bank target) gets the same category |
| Edit split-line category | Mirrored to the bank-side counterpart |
| Edit split-line amount | Category untouched on both sides |
| Move target account (`moveTransferLine`, `:691-707`) | Category carries onto the re-minted counterpart |
| Delete line / parent delete / parent void | Existing cascades; category needs nothing |
| Paired-side edits from its own register | Category not editable from the paired side (v1); the split line is canonical |

### Split dialog: carry-through, not an editor

The split dialog's transfer rows currently hardcode
`CategoryID: types.NilID` on save
(`internal/tui/split_dialog.go:452-462`), which would silently strip a
categorized transfer line (e.g. a posted loan payment's principal line)
the next time its parent is edited, and the scheduled split editor does the
same to templates (`internal/tui/scheduled_dialog.go:946-962`). v1 fixes
this as **carry-through**: `splitRow` stores the category of a seeded
transfer row and `buildSplits` re-emits it unchanged. The dialog gains no
picker for it (see Non-Goals); rendering may append a compact marker to
the `Transfer → <account>` cell when a category is present, truncation
permitting.

`ReplaceSplits` (`transaction_service.go:800-840`), the path behind every
TUI split edit (`submitSplitDialog` → `EditTransactionWithSplitsCommand`)
and `VoidTransactionCommand.Undo`, is **transfer-unaware — a confirmed
pre-existing corruption bug** (runtime-verified 2026-07-03): editing a
split transaction containing a transfer line fails the pairing CHECK
mid-flight and non-atomically (the parent is left with a partial split set
and the counterpart orphaned — the clear step alone orphans it), and with
a preserved transfer_id an amount edit silently desyncs the two legs while
a dropped line silently orphans its counterpart. The implementation plan
fixes this as its own early phase — diff old-vs-new transfer lines: mint
transfer_ids for added lines, delete counterparts for removed ones, update
retained ones — and category carry-through then rides the fixed path.

## Scheduled Transfers

### Single-line schedules

The scheduled transfer dialog (TUI `t` on the Scheduled view) gains an
optional Category combo (with inline creation); the schedule stores it on
`scheduled_transactions.category_id` alongside `transfer_account_id`. All
three posting paths converge on `CreateTransfer` and pick the category up
from the new parameter: manual post (`postSingleLineTransfer`,
`internal/scheduled/scheduled_service.go:567-603`), auto-post (inline at
`:267-283`), and the post-time preview
(`PostScheduledTransferCommand`, `internal/undo/scheduled_transaction.go:257-284`).
The single-line transfer preview header gains a Category combo
(`buildPreviewHeaderTransfer`,
`internal/tui/scheduled_preview_dialog.go:361-377`) so one occurrence can
be relabeled at post time without touching the template — consistent with
the estimate-then-edit-at-post workflow.

### Multi-line templates

`buildMultiLineTransaction` copies a template transfer line's category onto
the posted split (today it deliberately leaves it zero —
`scheduled_service.go:672-679`), from where counterpart mirroring takes
over. The preview's embedded split editor and the Edit Series split editor
get the same carry-through as the register split dialog
(`transactionSplitsFromScheduled` at `internal/tui/scheduled_dialog.go:46-68`
already copies both fields independently and needs no change).

## Loan Integration

### `Loan:Principal` default

`BuildLoanSnapshot` gains `PrincipalCatID` on `LoanSnapshotInput`
(`internal/scheduled/loan_build.go:24-31`) and sets it on the principal
transfer line (`loan_build.go:76-78`). Default resolution mirrors
`Loan:Interest`: a `GetOrCreateLoanPrincipalCategory` helper beside
`GetOrCreateLoanInterestCategory`
(`internal/category/category_service.go:308-331`), creating
`Loan` → `Principal` (expense-typed) when missing.

- **CLI**: `loan add --principal-category` — omitted → `Loan:Principal`
  (auto-created); explicit path → that category (created if missing, via
  the existing `getOrCreateCategoryPath`, `internal/cli/loan/add.go:322-342`);
  explicit empty string → no category.
- **TUI wizard**: a *Principal category* field prefilled `Loan:Principal`,
  clearable to none, with inline creation — beside the existing Interest
  category field. The edit wizard prefills from the existing principal
  line's category, so **Edit as loan →** round-trips it (today's rebuild
  would drop it — `internal/tui/loan_wizard.go:1104-1126`).

### Recompute-at-post preserves the label

`ComputeLoanSplits` regenerates lines from the live balance each post; the
principal line construction (`internal/scheduled/loan_posting.go:124-128`)
copies the template principal line's category, exactly as the interest
line already copies its template category (`:122`).

### Detection, projection, payoff: unaffected

`IsLoanShaped` inspects the principal line's transfer target and sections,
never its category (`internal/scheduled/loan_shape.go:118-121`) — the
forward-compatibility promise in `specs/loan-wizard.md` holds, and a
categorized principal line neither breaks detection nor trips the demotion
guard. `IsLoanAdoptable`, `FindLoanSchedule`, `LoanScheduleInputs`,
amortization view, payoff finalization, and `loan list/show` read no
category from the principal line (verified per-site). Regression tests pin
this.

## TUI

### Transfer dialog (create)

Field order becomes From, To, Amount, Date, Memo, **Category** — an
optional combo with `[+ Add new category…]`, wired through the standard
inline-creation plumbing (`createCategorySource` entry, `DialogActionAddNew`
key + mouse cases, applier/cancel restore —
`internal/tui/create_category_dialog.go:20-28,280-298`). The loader starts
fetching categories (`loadTransferDialogData`,
`internal/tui/transfer_dialog.go:181-198`). Category applies to reg→reg,
reg→inv, inv→reg; submitting inv→inv with a category selected is a
validation error naming the limitation. Submit-index guards update (fields
are read by hard-coded index, `transfer_dialog.go:442-503`).

### Edit Transfer dialog

Fields become Amount, Date, Memo, **Category**, Status for combos with a
regular leg; inv→inv omits the field. Seeds from the outflow leg's
category; saving mirrors through `UpdateTransfer` (bank↔bank, undo-managed)
or `UpdateTransferCash` (investment-involved, direct) — the
`investmentTransferEdit` payload gains the category
(`transfer_dialog.go:80-88`). Editing from the investment register flows
through this same dialog automatically
(`internal/tui/investment_register_view.go:650-652`).

### Scheduled transfer dialog

Gains **Category** after Amount (the `schedXferField*` constant block
renumbers — `internal/tui/scheduled_transfer_dialog.go:20-34`); edit mode
seeds from the schedule; save stops calling `ClearCategory()` (`:306`).

### Registers and views

- Register and reconciliation rows: no changes needed — `HasCategory()`
  already renders before the `[Transfer]` placeholder
  (`internal/tui/register_view.go:500-506`,
  `reconciliation_view.go:307-315`); payee slot keeps `Transfer: <account>`.
- Scheduled view and dashboard due-list have no category column — untouched.

## CLI

```bash
# Categorize a transfer at creation
tmoney transfer add --from Checking --to Visa --amount 500 \
  --category "Bills:Credit Card"

# Add, change, or clear the category on an existing pair
tmoney transfer edit --txn-id <id> --category "Bills:Credit Card"
tmoney transfer edit --txn-id <id> --category ""

# Label the loan principal line (default Loan:Principal; "" disables)
tmoney loan add --name "Mortgage" ... --principal-category "Housing:Principal"

# Fold categorized transfers into the spending report
tmoney report spending --month 2026-07 --include-transfers
```

- `transfer add --category <path>`: optional; `Parent` or
  `Parent:Subcategory`; must resolve to an existing non-system category
  (unlike `loan add`, it does **not** create categories — transfers are
  labeled with categories you already report on).
- `transfer edit --category <path>`: only-supplied-flags semantics as
  today; explicit `--category ""` clears both legs.
- `loan add --principal-category`: creates the path if missing (same
  helper as `--interest-category`/`--escrow`).
- `report spending --include-transfers`: works with `--month`, `--year`,
  and `--from/--to`.
- `transaction list` / `transaction search` output: the transfer override
  currently discards the resolved category
  (`internal/cli/transaction/format.go:52-55,125-128`); it now shows the
  category when present, keeping `[Transfer]` in the payee column.

## Import / Export

- **CSV export**: automatic — the `Category` and `Transfer Account` columns
  are independent, and the export code paths already emit a category the
  moment `HasCategory()`/`split.CategoryID` is non-nil
  (`internal/imexport/export_service.go:270-276,253-257`). Regression
  tests pin both (header transfer, split transfer-line).
- **QIF export**: structurally lossy and stays that way — QIF's single `L`
  field holds *either* a category *or* the `[Account]` transfer marker, and
  the transfer wins (`internal/imexport/qif.go:334-343`). Documented, not
  an error.
- **Imports**: unchanged. No import path creates transfers
  (`ImportRecord.TransferAccount` is parsed but never consumed); imported
  categorized rows join into categorized pairs only via `transfer link`
  (see adoption rules). Duplicate matching never consults categories
  (`internal/imexport/matcher.go`).

## Edge Cases

- **Legacy divergent pairs** (pre-feature `transfer link` output): read
  paths display the outflow leg's category; the first pair edit rewrites
  both legs to the dialog's single value. No migration rewrites data.
- **Category delete / merge**: transfer rows and lines are covered by the
  same `category_id` columns merge already updates
  (`internal/category/category_service.go:527-544`) and the same in-use
  checks delete already runs. Pre-existing gaps (merge skips
  `scheduled_split_items`; delete's in-use check reads only
  `transactions.category_id`) are unchanged by this feature and tracked in
  the implementation plan's out-of-scope list.
- **Void then restore**: category survives — void/restore touch
  amount/memo/status only.
- **Search**: a categorized transfer matches `--category` filters with no
  query change (`Search` joins `t.category_id`,
  `internal/transaction/transaction_repository.go:535-536`). Split-line
  categories remain invisible to search (pre-existing: search never joins
  `transaction_splits`).
- **Income-typed category on a transfer**: legal; never appears in the
  spending report (expense-type filter), with or without the toggle.
- **0% loans**: no interest line, principal line still labeled.
- **DuckDB UPDATE safety**: no new risk — the transfer edit path already
  runs the full-column `UPDATE transactions SET … category_id = ? …`
  through the rewrite-safe post-026 schema
  (`internal/transaction/transaction_repository.go:299-329`), and transfer
  legs can never have child split rows (`TransferCannotHaveSplitsError`).

## Out of Scope Summary

Per-leg categories · inv→inv categories · split-dialog category picker for
transfer lines · paycheck wizard field · loan-schedule backfill · income
report · toggle persistence · link tiebreaking · fixing pre-existing
category merge/delete coverage gaps.

**Forward compatibility note**: if per-leg categories ever become a
requirement (e.g. asymmetric labeling of reimbursement flows), the storage
already permits divergence — the constraint is purely the service-layer
mirroring contract and the single Category field per dialog. A future
feature would replace the mirror-on-edit rule with per-leg fields and would
need its own reporting story for the inflow leg.
