# Design: split the HSA account type into `hsa` and `hsa_investment`

**Date:** 2026-09-21
**Status:** IMPLEMENTED. Two deviations from the proposal, both stronger than
what was specified:

1. §5.1 proposed an `arch_test` as the guard against a stray cross-ledger
   type change. The implementation puts the guard in the domain instead:
   `account.Service.Update` refuses a cross-ledger change while the departing
   ledger has rows (`account.LedgerChangeError`). Every caller is covered, not
   only the two the test would have listed.
2. §5.5/§5.6 said a backup is written before the move. The CLI does that
   (close, auto backup, reopen, move). The TUI cannot copy an open DuckDB
   file mid-session, so it relies on the auto backup written on exit and
   says so in the confirm dialog.

The runbook in §7 is unchanged.

## 1. Problem

A Health Savings Account has two forms at most custodians:

1. A **cash account** with a debit card. Contributions arrive from payroll.
   Medical bills are paid from it. It behaves like a checking account whose
   card only works at medical merchants.
2. An **investment account**. Once the cash balance passes a custodian
   threshold (for example $2,000), the owner can move the excess into a
   brokerage-like account and buy securities.

Many owners hold both, at two custodians. The app today has one type, `hsa`,
and it models only the second form. `account.Type.IsInvestmentType()` returns
`true` for `hsa`, so:

- `transfer.LedgerFor(hsa)` returns `LedgerInvestment`. Every HSA row is
  written to `investment_transactions`.
- `investment_transactions` has no `category_id` column. A medical bill paid
  from the cash HSA can only be recorded as a `withdrawal` with no category.
  It never appears in the spending report.
- The TUI opens the Portfolio view for an `hsa` account
  (`app_sidebar.go:57`, `app_mouse.go:284`). The Register view, and with it
  the normal transaction dialog, is not reachable.
- Transfer linking (`transferlink.isEligible`) and the loan wizard exclude
  the account.

### 1.1 Worked example

Every name and amount in this document is hypothetical. The example file
`finances.tdb` holds two HSAs at two made-up custodians:

| Account | Type | Rows in `investment_transactions` | Content |
|---|---|---|---|
| Cedar Bank HSA | `hsa` | 13 | 9 `transfer_cash` in from paycheck split lines ($250.00 each), 2 `transfer_cash` out to Maple Invest ($600.00, $200.00), 1 `withdrawal` ($180.25, a medical bill, no category), 1 `interest` ($0.12). Cash $1,269.87, no holdings. |
| Maple Invest HSA | `hsa` | 23 | 2 `transfer_cash` in from Cedar Bank, `buy` rows for several ETFs, a few `dividend` rows, 1 `fee`. Holdings and a small cash balance. |

Cedar Bank is form 1. Maple Invest is form 2. The app treats both as form 2.

### 1.2 A second defect this design fixes

`account edit --type` and the TUI Edit Account dialog change `accounts.type`
with no check on existing rows. `account.Service.Update` validates fields
only. Tested on a throwaway copy: changing Cedar Bank to `checking` left its
13 rows in `investment_transactions`. The register showed nothing, `account
show` reported $0.00, and net worth fell by the account's cash balance. The
rows were not lost, but
no read path could see them. A type change that crosses ledgers must either
move the rows or be refused.

## 2. Decisions

These were settled in review with the owner on 2026-09-21.

| # | Decision | Chosen |
|---|---|---|
| D1 | Model | Two types. `hsa` becomes the cash account (regular ledger). New `hsa_investment` takes today's investment behavior. |
| D2 | Cedar Bank's rows | A cross-ledger type change is a guarded service operation that moves cash-only rows. It is refused when security rows exist. |
| D3 | Migration | Migration 034 rewrites **every** existing `hsa` account to `hsa_investment`. It does not guess which HSAs are cash. |
| D4 | Row mapping | Move rows with no category and no payee. Memo is copied. `pending` → `uncleared`. Transfer ids are kept. The owner categorizes by hand afterward. |
| D5 | Reverse direction | Regular → investment type is refused when the account has any rows. |
| D6 | Confirmation | CLI prints a plan and does nothing until `--confirm`. TUI shows a confirm dialog with the same counts. |
| D7 | Naming | Ids `hsa` / `hsa_investment`. Display names `HSA` / `HSA Investment`. Both in the `Health Savings` sidebar group, cash first. Both support an interest rate. Only `hsa_investment` supports lot tracking, on by default. |
| D8 | Process | This spec ships first as a design PR. Code follows in a second PR. |

### 2.1 Why `hsa` is the cash type and not the investment type

The alternative keeps `hsa` as the investment type and adds `hsa_cash`. It
avoids the rename in migration 034. It was rejected because the plain word
"HSA" names the account the card is attached to. A user who picks `HSA` in
the account dialog expects a register. The investment form is the special
case and carries the qualifier.

### 2.2 Why the migration does not convert cash-only HSAs automatically

A new investment HSA is also cash-only until its first buy. Maple Invest had
this shape for the two weeks between its opening and its first buy. A
migration that converted every
HSA with no security rows would have misfiled it. The migration therefore
preserves today's behavior for every existing account and leaves the one
manual edit to the owner (§7).

## 3. Type model

`internal/account/account.go`:

```go
const (
    ...
    // TypeHSA is the cash side of a Health Savings Account: payroll
    // contributions in, medical expenses out, a debit card. It lives in the
    // regular ledger and behaves like checking. Excess cash is transferred
    // to a TypeHSAInvestment account.
    TypeHSA Type = "hsa"
    // TypeHSAInvestment is the invested side of a Health Savings Account.
    // It shares the investment account's lot-tracking and
    // buy/sell/dividend semantics and lives in investment_transactions.
    TypeHSAInvestment Type = "hsa_investment"
)
```

| Method | `hsa` | `hsa_investment` |
|---|---|---|
| `IsValid` | true | true |
| `IsAssetType` | true | true |
| `IsInvestmentType` | **false** | true |
| `IsLiabilityType` | false | false |
| `DisplayName` | `HSA` | `HSA Investment` |
| `AllTypes` order | after `investment` | after `hsa` |

`AllTypes` order drives the TUI select field (`accountTypeFromIndex`), so the
new entry is appended after `hsa` and nothing else moves.

Interest rate support (`accountTypeShowsInterestRate` in the TUI,
`accountTypeSupportsInterestRate` in `cli/account/edit.go`) adds
`TypeHSAInvestment` to its list. `hsa` is already there.

Lot tracking: `Validate` already gates `TrackLots` on `IsInvestmentType`, so
`hsa` loses lot tracking and `hsa_investment` gains it with no extra rule.
`account add` and the New Account dialog default `TrackLots` on for
investment types; `hsa_investment` inherits that.

### 3.1 Every site that switches on the HSA type

Every `IsInvestmentType()` caller is correct as written once the method
returns `false` for `hsa`. They are listed here so the implementation PR can
confirm each one by test, not so they can be edited:

`transfer/kind.go` (LedgerFor, ClassifyKind), `transaction/service_transfer_line.go:81`,
`investment/guards.go:30`, `investment/queries.go:47`, `investment/rebuild.go:191`,
`transferlink/transferlink.go:269`, `report/report_service.go:138`,
`tui/app_sidebar.go:57`, `tui/app_mouse.go:284`, `tui/dashboard_view.go` (5 sites),
`tui/account_dialog.go:132,453`, `tui/investment_transfer_shares_dialog.go:46`,
`tui/loan_wizard.go:140`, `cli/loan/add.go:229`, `cli/account/{add,edit}.go`,
`cli/investment/{enable_lots,disable_lots,rebuild_positions}.go`.

Sites that name `TypeHSA` directly and **do** need an edit:

| File | Change |
|---|---|
| `tui/sidebar.go:19,31` | Add `TypeHSAInvestment` to `accountGroupOrder` after `TypeHSA`; label `"Health Savings"`. |
| `tui/account_dialog.go:96` | Add `TypeHSAInvestment` to the interest-rate list. |
| `cli/account/edit.go:270` | Same. |
| `cli/clitest/transferfixtures.go:61` | The fixture named `HSA` becomes `TypeHSAInvestment` so the inv↔inv transfer tests keep their shape. Add a `TypeHSA` fixture for the reg↔inv case. |
| help strings in `cli/account/{add,edit}.go` | Type lists and the "interest rate is only valid for…" text. |

## 4. Migration 034

DuckDB cannot alter an anonymous `CHECK`, so `accounts` is rebuilt with the
backup-drop-recreate recipe of migration 019, which is the last migration to
touch this constraint. Every child table with an FK to `accounts` is backed
up, dropped, and recreated from its **latest** definition (019 for most,
026/029 for `transaction_splits`, 028/029 for `scheduled_split_items`, 032
for the `uuidv7()` defaults). The implementation PR must diff each recreated
`CREATE TABLE` against the latest migration that defined it; a rebuild that
silently reverts a column default or a relaxed CHECK is the known failure
mode of this recipe.

Steps:

1. Drop views `portfolio_holdings`, `account_balances`, `category_spending`.
2. Back up and drop every child of `accounts`, then `accounts`.
3. Recreate `accounts` with
   `CHECK (type IN ('checking','savings','credit_card','investment','hsa','hsa_investment','cash','loan','asset'))`.
4. Restore rows: `INSERT INTO accounts SELECT * REPLACE (CASE WHEN type = 'hsa' THEN 'hsa_investment' ELSE type END AS type) FROM accounts_backup;`
5. Recreate the children and restore their rows unchanged.
6. Recreate the views. `portfolio_holdings` gates on
   `a.type IN ('investment', 'hsa_investment')`. `account_balances` and
   `category_spending` are recreated verbatim from 029.

The rewrite in step 4 is the whole semantic content of the migration. It is
correct for every account in every file, because before 034 every `hsa`
account's rows are in `investment_transactions`, and after 034 that is what
`hsa_investment` means.

## 5. Ledger move: `hsa_investment` → `hsa` and `investment` → any regular type

### 5.1 Where it lives

`account.Service` cannot do this. It must read `investment_transactions` and
write `transactions`, and both `internal/investment` and `internal/transaction`
import `internal/account`. `internal/transfer` already sits above both
ledgers, imports both repositories, and owns the "which table does this leg
live in" rule (`LedgerFor`). The move is a ledger concern, so it lives
there:

```go
// internal/transfer/ledger_move.go
package transfer

// LedgerMovePlan describes what ChangeAccountType would do. It is returned
// by PlanAccountTypeChange and printed by the CLI / shown by the TUI before
// anything is written.
type LedgerMovePlan struct {
    AccountID   types.ID
    From, To    account.Type
    FromLedger  Ledger
    ToLedger    Ledger
    RowsByType  map[investment.TransactionType]int // empty when no move
    Total       int
}

// PlanAccountTypeChange validates the type change and returns the plan.
// Errors: *LedgerMoveRefusedError (security rows present, or reg→inv with
// rows), account not found.
func (s *Service) PlanAccountTypeChange(acctID types.ID, to account.Type) (*LedgerMovePlan, error)

// ChangeAccountType applies the plan in one db transaction: writes the new
// type, moves the rows, and deletes the source rows. Positions and lots are
// not touched because a movable account has none (see 5.2).
func (s *Service) ChangeAccountType(plan *LedgerMovePlan) error
```

`PlanAccountTypeChange` is called by both `account edit` and the TUI dialog
**before** they touch any other field, and only when the type actually
changes ledger. A type change inside one ledger (`checking` → `savings`,
`investment` → `hsa_investment`) has an empty plan and goes through
`account.Service.Update` as today. The account service gains nothing; the
guard is that the two presentation paths route every type change through
`PlanAccountTypeChange` first. An `arch_test` in `internal/transfer`
confirms no other production code writes `accounts.type` across a ledger
boundary.

### 5.2 Rules

Let `from = LedgerFor(old type)`, `to = LedgerFor(new type)`.

| from → to | Rule |
|---|---|
| same | No move. Empty plan. |
| investment → regular | Allowed iff every row's type is in {`deposit`, `withdrawal`, `interest`, `fee`, `transfer_cash`}. Any `buy`, `sell`, `dividend`, `reinvest_dividend`, `fee_liquidation`, `transfer_shares`, or `exchange` row refuses the whole change with `LedgerMoveRefusedError{Reason: "security rows"}` naming the count. |
| regular → investment | Allowed iff `transactions` has no row for the account (`CountByAccount == 0`) **and** no `scheduled_transactions` row targets it. Otherwise refused with `Reason: "rows exist"`. |

An account whose rows are all cash-kind has no `investment_positions` and no
open `investment_lots` rows (lots are created by `buy`, `reinvest_dividend`,
`transfer_shares` in, and `exchange`). The plan asserts this and refuses if
it is false, because it means the ledger is inconsistent and needs
`rebuild-positions` first.

### 5.3 Row mapping, investment → regular

For each `investment.Transaction` `r` in date order:

| Regular field | Source |
|---|---|
| `id` | `r.ID` (kept, so anything holding the id keeps working) |
| `account_id` | `r.AccountID` |
| `date` | `r.Date` |
| `amount` | `r.TotalAmount` (already signed: withdrawals and outbound transfers are negative) |
| `status` | `pending` → `uncleared`; `cleared`, `reconciled` unchanged |
| `payee_id` | NULL |
| `category_id` | NULL |
| `memo` | `r.Memo` |
| `check_number` | NULL |
| `transfer_id` | `r.TransferID` |
| `transfer_account_id` | `r.TransferAccountID` |
| `bank_reference_id` | NULL |
| `created_at`, `updated_at` | `r.CreatedAt`, now |

`commission` is zero on every cash-kind row by validation, so nothing is
dropped.

The source row is deleted after the insert, inside the same transaction.

### 5.4 Transfer pairs after the move

A moved `transfer_cash` row keeps its `transfer_id` and `transfer_account_id`,
so its partner leg needs no change:

- **Paycheck split line** (Cedar Bank's 9 contributions). The partner is a
  `transaction_splits` row on the paycheck with the same `transfer_id`. After
  the move, `transaction.Service.findPairedByTransferID` finds the moved row
  in `transactions` by that id, which is what it does for every regular
  target today. A later edit of the paycheck mirrors amount and category onto
  it (`mirrorToPairedCounterpart`). Future posts of the scheduled paycheck
  call `ensureTransferTargetRoutable`, see a non-investment target, and
  create a regular counterpart. No change to `internal/scheduled`.
- **Whole transfer to another investment account** (Cedar Bank → Maple
  Invest). The Maple Invest leg stays in `investment_transactions`. `transfer.Service.Get`
  derives each leg's table from its own account type, so the pair reads as
  `KindRegToInv` afterward. `StoresCategory` becomes true for it; the moved
  leg has NULL category, which is the same as an uncategorized bank→investment
  transfer today.
- **Whole transfer to a regular account** (none in the data, but possible).
  Reads as `KindRegToReg`.

`transferlink` becomes eligible for the account after the move. That is the
desired behavior for a cash HSA.

### 5.5 Undo

The TUI edit path runs through `undo.NewEditAccountCommand`. That command
restores account fields only. It cannot restore moved rows, and the reverse
move is refused by D5, so an undo after a ledger move would leave the type
and the ledger disagreeing. The TUI therefore:

1. Calls `PlanAccountTypeChange`. If the plan is empty, proceeds as today.
2. Otherwise closes the edit dialog and opens `showConfirmDialog` with the
   plan: account, `HSA Investment → HSA`, row counts by kind, and the line
   "This cannot be undone. A backup is written first."
3. On confirm, calls `ChangeAccountType` directly (not through the undo
   manager), then `undoManager.Clear()`, as `backup_dialog.go:164` and
   `file_dialog.go:276` do after an irreversible file operation. Other field
   edits from the same dialog are applied in the same call after the move.

### 5.6 CLI

`tmoney account edit --name X --type hsa` with a non-empty plan prints:

```
Cedar Bank HSA: HSA Investment -> HSA
This moves 13 rows from the investment ledger to the register:
  transfer_cash  11
  withdrawal      1
  interest        1
Moved rows keep their transfer links. They get no payee and no category.
This cannot be undone. Re-run with --confirm to apply (a backup is written first).
```

and exits 0 with no change. With `--confirm` it runs `db backup`-equivalent
auto backup **before** the write (today `AutoBackupAfterModification` runs
after; the ledger move calls the backup helper first, then again after as
usual), applies, and prints `Moved 13 rows. Account updated.` A refused plan
prints the reason and exits 1. `--confirm` on an empty plan is accepted and
ignored, so scripts need not special-case it.

## 6. Reads and reports

No change is needed; listed to be verified by test in the implementation PR.

- **Register** (`transactions`): a cash `hsa` account now opens here. Add,
  edit, categorize, split, reconcile, import, and scheduled posts all work
  as for checking. Nothing in the tree gates on `TypeChecking` except the
  sidebar grouping and the interest-rate list.
- **Portfolio**: `hsa_investment` opens here, as `hsa` does today.
- **Net worth** (`report_service.go:138`): `hsa` sums its regular rows;
  `hsa_investment` uses `investmentValue`. The `account_balances` view
  reports the cash HSA correctly for the first time.
- **Spending**: medical bills paid from the cash HSA appear once they carry a
  category.
- **Dashboard**: `hsa_investment` shows the expandable holdings row; `hsa`
  shows a plain balance.

## 7. Runbook for an existing file

Run after the implementation PR ships. Nothing here is automatic. The
example continues §1.1.

1. `tmoney -f finances.tdb db backup`
2. Open the file once (TUI or any CLI command). Migration 034 runs. Both
   HSAs are now `HSA Investment`. Maple Invest is finished.
3. `tmoney -f finances.tdb account edit --name "Cedar Bank HSA" --type hsa`
   Read the plan. Expect 13 rows: 11 `transfer_cash`, 1 `withdrawal`,
   1 `interest`.
4. Re-run with `--confirm`.
5. In the Register for Cedar Bank HSA, set the category on the two
   non-transfer rows: the withdrawal (−$180.25) → `Health`; the interest
   ($0.12) → `Interest`. Set a payee on the withdrawal if wanted.
6. Check: `report net-worth` shows Cedar Bank at the same cash balance as
   before the move ($1,269.87 in the example) and Maple Invest unchanged.

## 8. Out of scope

- A `category_id` column on `investment_transactions`. It would let form-2
  accounts categorize fees and would remove `Kind.StoresCategory`; it is a
  different design.
- Reverse move with representable rows (D5 alternative).
- An `hsa_investment` threshold or contribution-limit report.
- Automatic category assignment during the move (D4 alternative).

## 9. Test plan for the implementation PR

- `account`: `IsInvestmentType` table covers both new values; `Validate`
  refuses `TrackLots` on `hsa`.
- `db`: migration 034 on a fixture with one `hsa` account holding a `buy` and
  a `transfer_cash`: type becomes `hsa_investment`, every child row count is
  unchanged, `portfolio_holdings` still lists the holding, and every
  recreated table's `DESCRIBE` matches the pre-migration one apart from the
  CHECK.
- `transfer`: plan/apply on (a) cash-only inv account → counts match, rows
  land in `transactions` with mapped fields, source rows gone, paycheck split
  partner still resolves, inv→inv pair now reads `KindRegToInv`; (b) account
  with one `buy` → refused, nothing written; (c) regular account with rows →
  inv refused; (d) same-ledger change → empty plan; (e) apply is atomic: a
  forced failure on the last delete leaves the source ledger intact.
- `cli/account`: `edit --type` prints the plan without `--confirm`, applies
  with it, exits 1 on refusal.
- `tui`: edit dialog with a cross-ledger change opens the confirm dialog;
  cancel writes nothing; confirm moves rows and clears undo.
- `transfer/arch_test`: no package other than `transfer` and the two
  presentation entry points assigns `accounts.type` across ledgers.
