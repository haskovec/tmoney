# Transactions Specification

## Overview

Transactions represent the movement of money into, out of, or between accounts. They are the core data of the application and drive all balance calculations and reports.

## Transaction Properties

### Core Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | UUID | Yes | Unique identifier |
| `account_id` | UUID | Yes | Account this transaction belongs to |
| `date` | date | Yes | Transaction date |
| `amount` | decimal | Yes | Transaction amount (see sign convention) |
| `payee_id` | UUID | No | Reference to payee |
| `memo` | string | No | User notes/description |
| `check_number` | string | No | Check number if applicable |
| `status` | enum | Yes | uncleared, cleared, reconciled, void |
| `created_at` | timestamp | Yes | When record was created |
| `updated_at` | timestamp | Yes | When record was last updated |

### Transfer Properties

| Property | Type | Description |
|----------|------|-------------|
| `transfer_id` | UUID | Links two transactions as a transfer pair |
| `transfer_account_id` | UUID | The other account in the transfer |

## Amount Sign Convention

| Transaction Type | Amount Sign | Example |
|------------------|-------------|---------|
| Income/Deposit | Positive | +1000.00 (paycheck) |
| Expense/Withdrawal | Negative | -50.00 (grocery purchase) |
| Transfer Out | Negative | -500.00 (to savings) |
| Transfer In | Positive | +500.00 (from checking) |

Credit cards and loans (liability accounts) use the same storage signs:
a charge is negative, a payment (transfer in) is positive, so the
account balance is negative while money is owed (see the sign table in
`specs/accounts.md`). Registers show the stored signed amounts; only
the net-worth views negate liability balances for display under their
LIABILITIES headings.

## Transaction Status

| Status | Description |
|--------|-------------|
| `uncleared` | Entered but not yet cleared at bank |
| `cleared` | Confirmed cleared at bank |
| `reconciled` | Matched during reconciliation (v1.5) |
| `void` | Zeroed but retained for the audit trail |

(The `investment_transactions` table uses its own three-value enum —
`pending`, `cleared`, `reconciled` — with no `void`. `pending` there means the
same thing `uncleared` means here; `transfer.StatusToRegular` /
`StatusFromRegular` are the single mapping between them, and the absence of
`void` on the investment side is why an investment-involving transfer cannot be
voided.)

## Split Transactions

A transaction can be split across multiple lines. When split:

- The parent transaction has the total amount
- Split items define how the amount is allocated
- Each line is **categorized**, a **transfer** to another account, or both (a **categorized transfer** — e.g. a loan payment's principal line)
- Signed sum of split amounts equals the parent amount (mixed signs allowed)

### Split Item Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | UUID | Yes | Unique identifier |
| `transaction_id` | UUID | Yes | Parent transaction |
| `category_id` | UUID | No | Category for this line — set when the line is categorized |
| `transfer_account_id` | UUID | No | Target account — set when the line is a transfer |
| `transfer_id` | UUID | No | Shared identifier linking the line to its paired single-line counter-transaction in the target account |
| `amount` | decimal | Yes | Signed amount for this line |
| `memo` | string | No | Note for this line |

Validation: at least one of `category_id` or `transfer_account_id` is set per row — a transfer line may **also** carry an optional category (a *categorized transfer*; see [`specs/transfer-categories.md`](transfer-categories.md)); `transfer_id` is set iff `transfer_account_id` is set.

### Split Transaction Example (same-sign, categorized)

Grocery store purchase of $150:
- Total: -$150.00 (Payee: Kroger)
  - Split 1: -$120.00 (Category: Food → Groceries)
  - Split 2: -$30.00 (Category: Household → Cleaning)

### Mixed-Sign Split with Transfer Lines

Split lines may carry mixed signs and may be transfer lines (a line that moves cash to another account, auto-creating a paired single-line transaction in the target). This is the primitive used to model paychecks and similar compound events. See [`specs/multiline-splits-and-paycheck.md`](multiline-splits-and-paycheck.md) for the full spec.

Example — paycheck deposited to checking:

- Total: +$3,067.50 (Payee: Employer)
  - +$5,000.00 (Category: Income → Salary)
  - -$800.00 (Category: Tax → Federal)
  - -$310.00 (Category: Tax → Social Security)
  - -$72.50 (Category: Tax → Medicare)
  - -$500.00 (Transfer → 401k account)
  - -$150.00 (Category: Insurance → Health)
  - -$100.00 (Transfer → HSA account)

The parent amount (+$3,067.50) is the signed sum of all lines, which is what the bank actually deposits to checking. Each transfer-line creates its own paired single-line transaction in the target account, linked by `transfer_id` (the same mechanism as today's whole-transaction transfers).

Legacy whole-transaction transfers (one pair of single-line transactions linked by `transfer_id` at the transactions level, with no split children) remain valid alongside new transfer-lines.

The split dialog renders a live `Imbalance: $X.XX` indicator below the line list that recomputes the signed-sum delta on every keystroke; Save is disabled until the delta is zero. The dialog also accepts the `Transfer →` sentinel in the category combo of any line, which swaps that line's category cell for an account picker (the parent's own account is excluded from the picker to prevent self-transfers).

## Transfers

There are two shapes of transfer in TMoney, and they coexist:

1. **Whole-transaction transfers** — two single-line transactions, one
   per account, linked by a shared `transfer_id`. This is the legacy
   shape that `tmoney transfer add` and the TUI's `t` (new transfer)
   action emit, and the only shape produced by `transfer link` import
   matching. When both accounts are non-investment, both linked rows
   live in the `transactions` table. When one or both legs are an
   investment account, the investment-side row lives in the
   `investment_transactions` table as a `transfer_cash` row — see
   [Investment-Account Transfers](#investment-account-transfers) for
   the dispatch rules.
2. **Transfer-lines** — one line of a multi-line split on a parent
   transaction whose `transfer_account_id` is set; the service mints a
   single-line *paired counter-transaction* in the target account
   carrying the matching `transfer_id`. The parent itself is not a
   transfer (its scalar `transfer_account_id` and `transfer_id` stay
   null); only the split-line carries the linkage on the parent's
   side.

Both shapes use the same `transfer_id` field to link the two sides, so
balance calculations and the existing transfer detection logic treat
them uniformly. Transfer status is encoded by the `transfer_id` /
`transfer_account_id` pair, not by a "Transfer" category; a transfer
may **optionally** carry a shared non-system category, mirrored across
both legs (on the regular-side leg only when one leg is an investment
account; inv↔inv transfers have nowhere to store one). The category is
a tracking label that never affects balance math or transfer linkage —
see [`specs/transfer-categories.md`](transfer-categories.md). The
register renders the other account's name from `transfer_account_id`.

### Whole-Transaction Transfer Example: $500 from Checking to Savings

**Transaction 1 (in Checking account):**
- amount: -500.00
- category_id: NULL
- transfer_id: <shared-uuid>
- transfer_account_id: <savings-account-id>

**Transaction 2 (in Savings account):**
- amount: +500.00
- category_id: NULL
- transfer_id: <shared-uuid>
- transfer_account_id: <checking-account-id>

Both legs show `category_id: NULL` because this transfer is unlabeled; a
labeled transfer stores the same shared category on both legs (see
[`specs/transfer-categories.md`](transfer-categories.md)).

### Whole-Transaction Transfer Rules

1. Both transactions share the same `transfer_id`
2. Amounts are equal but opposite signs
3. Deleting one side removes both sides as a pair (the TUI prompts for confirmation; the CLI's `transfer delete` deletes both without prompting)
4. Editing amount on one side updates the other

### One transfer owner

Every whole-transaction cash transfer — bank↔bank, bank↔investment and
investment↔investment alike — goes through `transfer.Service`
(`internal/transfer`), reached as `Services.Transfer`:

```go
res, err := svc.Transfer.Create(transfer.Spec{
    FromAccountID: from.ID, ToAccountID: to.ID,
    Date: date, Amount: amount, Memo: memo, CategoryID: categoryID,
})
```

There is no dispatch. `planLegs` signs the From leg negative and the To leg
positive, and each leg's table comes from its own account type, so 2 signs ×
2 ledgers reproduces all four shapes as a property of the data:

| From → To | Where the legs land |
|---|---|
| reg → reg | both rows in `transactions` |
| reg → inv | regular row in `transactions`, `transfer_cash` row in `investment_transactions` |
| inv → reg | mirror of the above |
| inv → inv | both rows in `investment_transactions`, type `transfer_cash`, signed opposite |

The verbs are `Create`, `Get`, `Resolve` (from any leg's row ID), `Update`,
`Reverse`, `SetStatus`, `SetLegStatus`, `Void`, `Restore`, `Delete`, `Recreate`
and `LinkExisting`. `Update` edits both legs IN PLACE, so a transfer keeps its
`transfer_id` and row identities across an edit.

Accounts are immutable on `Update`: delete and recreate to re-account a
transfer. Reads (`Resolve`) succeed for a transfer LINE inside a multi-line
split so callers can explain the refusal; every verb refuses it, along with
share transfers and reconciled legs.

This replaced four service methods, two edit methods, five hand-written 4-arm
switches in the TUI and CLI, three cross-table leg resolvers, three result
shapes and seven undo commands. See
[`specs/design-unified-transfer.md`](design-unified-transfer.md) for the design
and its phase history; `transaction.ChooseTransferDispatch` is now
`transfer.ClassifyKind`, and is a LABEL for errors and the category rule rather
than a routing decision.

**Category rule**: a transfer may carry one optional non-system category,
mirrored onto every leg that can store one. An investment↔investment transfer
cannot carry one at all — `investment_transactions` has no `category_id` column —
and `transfer.Kind.StoresCategory()` is the single predicate for that, called by
the domain guard and by the front ends for inline field errors.

**Transfer LINES inside splits** stay in `transaction.Service`: a split line's
counterpart must commit with the split row that owns it. When the line's target
is an investment account, the counterpart is minted through
`transaction.InvestmentCounterpartPort`, satisfied by `investment.Service`.

**Unified TUI dialog**: a single Transfer dialog handles all four
combinations. Field order is `From`, `To`, `Amount`, `Date`, `Memo`,
`Category` (the category is optional, offers inline creation, and is
omitted for investment↔investment transfers — see
[`specs/transfer-categories.md`](transfer-categories.md)).
On Edit, `From → To` renders as a read-only body message (the
accounts cannot be changed on an existing pair; delete and recreate
to move a transfer); editable fields are `Amount`, `Date`, `Memo`,
`Category` (present for combos with a regular leg; omitted for
inv↔inv), and `Status` (Cleared/Uncleared, applied to both legs). The dialog
reads and writes `txnDialogLastSavedDate` (sticky-date) on open and
save for all dispatch paths.

See
[`specs/implementation-plan-investment-cash-transfer-unification.md`](implementation-plan-investment-cash-transfer-unification.md)
for the design rationale and full implementation plan.

### Transfer-Line Rules

1. The split-line and its paired counter-transaction share the same
   `transfer_id`; the parent transaction's own scalar fields are
   *not* a transfer.
2. The paired counter-transaction's amount is the negation of the
   split-line's amount.
3. Editing the split-line's amount cascades to the paired side; the
   parent's other lines are unchanged.
4. Editing the split-line's target account deletes the old paired
   side and creates a new one in the new target account (a fresh
   `transfer_id` is minted). Cross-table moves (bank→inv, inv→bank)
   route through the right repository for each direction.
5. Deleting the split-line deletes the paired side; the parent
   retains its other lines and may be left imbalanced (the next Save
   on the parent is blocked until rebalanced).
6. Deleting *or voiding* the parent transaction cascades to every
   paired side before the parent's splits are removed. Bank-side
   counterparts are deleted from `transactions`; investment-side
   counterparts are deleted from `investment_transactions` via the
   investment service. (A reconciled paired side blocks the
   cascade.)
7. Deleting the paired side from its own register reverse-cascades:
   the parent's transfer-line is removed (same imbalance-blocking
   rule applies).
8. A transfer-line's `transfer_account_id` must not equal the
   parent's `account_id` (no self-transfers).
9. If either side is reconciled, the cascade is refused — the user
   must unreconcile first.

**Investment-target dispatch.** When a transfer-line's
`transfer_account_id` points at an investment-type account
(`investment` or `hsa`), the paired counter-transaction is created
on `investment_transactions` as a `transfer_cash` row, not on
`transactions`. The dispatch lives in `transaction.Service` and
calls into `investment.Service` via an
`InvestmentCashCounterpartAdapter` wired at app construction time;
without that adapter, transfer-lines targeting investment accounts
are rejected at the service layer. Every cascade above
(create / amount edit / target move / parent delete / parent void /
single-line delete) honors the dispatch — see
[`specs/multiline-splits-and-paycheck.md`](multiline-splits-and-paycheck.md#paired-counterpart)
for the full table.

## Validation Rules

1. `date` is required
2. `amount` cannot be zero
3. `account_id` must reference an active account
4. For splits: signed sum of split line amounts must equal parent transaction amount
5. For transfers (whole-transaction or transfer-line): both sides must exist and balance
6. A transfer-line's `transfer_account_id` must not equal the parent's `account_id` (no self-transfers)
7. `date` must not be before the account's opening date (the opening date
   itself is allowed). A transfer checks both accounts; investment buy/sell/
   dividend/reinvest/cash/transfer operations check the investment account
   (and the regular leg of a cash transfer checks that account too). This is
   enforced at the service layer — so it also applies to imports and the CLI —
   via `account.Account.ValidateTransactionDate`, which returns a
   `DateBeforeOpeningError`. Corporate-action `exchange` rows are exempt (they
   carry the action date and are written directly by the corporate-action
   engine). The rule turns a mistyped year (e.g. `0018` for `2018`) into an
   immediate rejection instead of a transaction — and auto-created price —
   thousands of years in the past.

## Operations

### Create Transaction

Required: account_id, date, amount
Optional: payee_id, category_id, memo, check_number, status

When payee is selected, auto-populate category from payee's default.

### Edit Transaction

All properties except `id` and `created_at` can be modified.

For transfers, editing amount updates both sides. On the CLI, `tmoney
transfer edit --txn-id <leg-uuid>` is the non-TUI entry point; it
mirrors the TUI's editable-field set (amount, date, memo, category,
status — not from/to). Only supplied flags take effect; `--category ""`
clears the category on both legs, and inv↔inv edits reject `--category`.

### Delete Transaction

- Regular transaction: delete immediately
- Whole-transaction transfer: both sides are deleted as a pair (TUI prompts; CLI's `transfer delete` does not)
- Multi-line transaction with transfer-lines: cascade delete every paired counter-transaction in target accounts
- Single transfer-line removed from a parent split: cascade delete its paired counter-transaction; parent retains other lines (may be imbalanced — hard validation blocks the next Save until rebalanced)
- Paired side deleted from its own (target) register: reverse cascade — remove the corresponding line from the parent split
- Reconciled transaction: warn user (v1.5)

### Duplicate Transaction

Create a copy with today's date, status = uncleared.

## Search

Transactions are searchable by:
- Payee name (partial match)
- Memo (partial match)
- Category name (partial match)
- Date range
- Amount (exact or range - TBD)

## Investment Transactions

For investment accounts, additional transaction types:

| Type | Description |
|------|-------------|
| `buy` | Purchase securities (creates lot) |
| `sell` | Sell securities (reduces lot) |
| `dividend` | Dividend income |
| `transfer_shares` | Securities moved between investment accounts (a linked pair) |
| `transfer_cash` | Cash moved into or out of an investment account (one leg of a transfer) |

The full set is in `investment.AllTransactionTypes()`; `transfer_in` and
`transfer_out` were never in the DB CHECK constraint and are not real types.

### Buy Transaction Additional Fields

| Property | Type | Description |
|----------|------|-------------|
| `symbol` | string | Security symbol |
| `quantity` | decimal | Number of shares |
| `price` | decimal | Price per share |
| `commission` | decimal | Trading commission |

### Investment Cash and Edit Semantics

Investment-account cash balances are computed from the transaction
ledger and are allowed to go negative. `buy`, `sell`, `withdrawal`,
`fee`, and cash-transfer operations never reject an entry on the basis
of the running balance — brokerage statements commonly emit the day's
sales after the day's buys, and a naive ordering check would force the
user to reorder same-date entries.

Editing an investment transaction goes through dedicated
`Update*` service methods (`UpdateBuy`, `UpdateSell`,
`UpdateReinvestDividend`, `UpdateDividend`, `UpdateFee`,
`UpdateDeposit`, `UpdateWithdrawal`, `UpdateInterest`,
`UpdateTransferCash`, `UpdateTransferShares`). Each Update reverses
the original transaction's effect on positions and lots **before**
applying the new transaction. The naive "delete then create" pattern
is incorrect for share-bearing types (Buy, Sell, ReinvestDividend,
FeeLiquidation, TransferShares) because the original transaction's
position/lot side-effects survive the deletion, producing
false-positive `InsufficientSharesError` on the re-create and leaving
the database in a desynced state.

Positions and lot shares auto-heal: every share-bearing service
operation (Buy, Sell, ReinvestDividend, FeeLiquidation,
TransferShares) calls `syncPositionAndLots` for the affected
(account, security) before reading the stored row, and
`Service.HealAllAccounts` runs once whenever `app.NewServices` builds
a registry (i.e. on every CLI command and TUI launch). The
corporate-action guard is **per-security**, and splits are replayable:
the position replay (`replayPosition`) and realized-gain replay
(`replayRealizedGain`) interleave each split's dated ratio, so a non-lot
account heals and reports realized gain across splits. Securities with a
**merger or spin-off** (cross-security transforms / cost-basis
reallocation that a per-security replay can't reconstruct) are skipped.
Lot-tracked split securities are also left untouched by heal — a
deliberate choice, not a data limitation: a split now scales each lot's
`original_shares` in lock-step with `shares` (so `remaining = original −
consumed` holds and the buy/reinvest edit guard no longer mis-fires on a
split lot), and skipping these securities during heal is exactly what lets
a manual per-lot split repair (`investment split-lot`, `investment buy
--catch-up-splits`) survive the next heal instead of being reverted to the
ledger. Lot-tracked realized gain is computed from lot junctions and is
unaffected. Every security untouched
by an un-replayable action heals normally even on a database that holds
corporate-action history. The `tmoney investment rebuild-positions`
command follows the same rules.

## v1.5 Features (Not in v1)

- Reconciliation status and workflow
- Attachments (receipts, documents)
- Recurring transaction link
- Tags (in addition to categories)
