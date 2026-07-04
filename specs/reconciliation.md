# Reconciliation Specification (v1.5)

## Overview

Reconciliation allows users to match their account register against a bank statement, verifying that all transactions are accounted for. Once reconciled, transactions are locked from editing to preserve the verified state.

## Workflow

### Starting Reconciliation

1. User selects an account to reconcile. From the register view, press
   `r` to open the start-reconciliation dialog; equivalently, use
   **Accounts → Reconcile Account** from the menu.
2. User enters:
   - **Statement date**: the ending date on the bank statement
   - **Statement ending balance**: the final balance shown on the statement
3. TMoney opens the dedicated reconciliation view

**Liability accounts (credit card, loan): enter the statement balance
negated.** Servicer statements show what you owe as a positive number,
but liability balances are stored negative (see `specs/accounts.md`).
A credit-card statement showing a $1,234.56 balance owed is entered as
`-1234.56`; a loan statement showing $249,500.00 outstanding is entered
as `-249500.00`. The difference math (`statement balance − cleared
total`) then works unchanged.

Example: reconciling a credit card against a statement with a New
Balance of $850.00 —

```
tmoney -f personal.tdb reconcile start --account "Visa" \
  --statement-date 2024-01-31 --statement-balance -850.00
```

### Reconciliation View (TUI)

A **dedicated full-screen view** with a single-column transaction list and sticky footer.

```
┌─────────────────────────────────────────────────────────────────────┐
│ File  Accounts  Transactions  Reports  Help                         │
├─────────────────────────────────────────────────────────────────────┤
│  RECONCILE: Checking               Statement Date: 2024-01-31      │
│  ─────────────────────────────────────────────────────────────────  │
│                                                                     │
│  [ ] 01/03  ✓  Landlord        Rent              -$1,500.00        │
│  [✓] 01/05     Coffee Shop     Coffee                -$5.75        │
│  [✓] 01/08  ✓  Transfer        Savings             -$500.00        │
│  [ ] 01/10  ✓  Shell           Gas                   -$42.50       │
│  [✓] 01/12     Amazon          Shopping              -$45.99       │
│  [✓] 01/14     Employer        Salary             $2,500.00        │
│▸ [ ] 01/15  ✓  Kroger          Groceries            -$125.43       │
│                                                                     │
│                                                                     │
├─────────────────────────────────────────────────────────────────────┤
│ Statement: $5,234.56  Cleared: $1,948.26  Difference: $3,286.30    │
│ Checked: 4 of 7    Space toggle  Enter finish  Esc cancel          │
└─────────────────────────────────────────────────────────────────────┘
```

### Layout

- **Header**: account name and statement date
- **Transaction list**: all unreconciled transactions (status `uncleared` or `cleared`) dated on or before the statement date
  - Each row has a checkbox, date, cleared indicator, payee, category, and amount
  - Selected row highlighted with `▸`
- **Sticky footer** showing:
  - **Statement balance**: the target balance entered by the user
  - **Cleared total**: sum of opening balance + all reconciled transactions + currently checked transactions
  - **Difference**: statement balance - cleared total (goal: $0.00)
  - **Count**: checked transactions out of total

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Space` | Toggle checkbox on selected transaction |
| `↑` / `k` | Move selection up |
| `↓` / `j` | Move selection down |
| `Home` / `g` | First transaction |
| `End` / `G` | Last transaction |
| `PgUp` | Page up |
| `PgDn` | Page down |
| `Enter` | Finish reconciliation |
| `Esc` | Cancel reconciliation (no changes saved) |
| `a` | Check all |
| `u` | Uncheck all |

### Completing Reconciliation

When the user presses Enter to finish:

1. If **difference is $0.00**:
   - All checked transactions are set to status `reconciled`
   - Success message shown
   - Returns to the account register view
2. If **difference is not $0.00**:
   - **Warning only**: "Statement balance does not match. Difference: $X.XX. Reconciliation cannot be completed until the difference is $0.00."
   - User remains in the reconciliation view to continue checking or correcting

**Persistence.** Marking a transaction `reconciled` and completing the session are
**status-only, in-place updates** (`transaction.Repository.UpdateStatus` /
`reconciliation.Repository.UpdateStatus`) — they set only `status` (plus
`updated_at`/`completed_at`), never rewriting the row. This deliberately avoids
DuckDB's UPDATE→DELETE+INSERT rewrite (migration 030 drops the `status` indexes so
these columns are unindexed), which can abort on a desynced ART index with
`Failed to delete all rows from index. Only deleted 0 out of 1 rows`. If a
**full-row** edit or void of a transaction ever fails with that error, run
[`tmoney db reindex`](cli.md#db-reindex) to rebuild the indexes; reconcile itself
never needs it.

### Cancelling Reconciliation

Pressing Esc cancels without saving. No transactions are changed.

## Reconciliation Sessions

### Database Schema

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | UUID | Yes | Unique identifier |
| `account_id` | UUID | Yes | Account being reconciled |
| `statement_date` | date | Yes | Bank statement date |
| `statement_balance` | decimal | Yes | Bank statement ending balance |
| `status` | enum | Yes | `in_progress` or `completed` |
| `completed_at` | timestamp | No | When reconciliation was finished |
| `created_at` | timestamp | Yes | When session was started |

### Session Lifecycle

1. `--start-reconcile` creates a session with status `in_progress`
2. Only one active (in_progress) session per account at a time
3. `--finish-reconcile` sets status to `completed` and marks transactions
4. Starting a new reconciliation while one is in_progress replaces it

## Discrepancy Handling

- TMoney shows the discrepancy amount but **does not create adjustment transactions**
- User must fix manually by:
  - Adding missing transactions
  - Correcting incorrect amounts
  - Checking/unchecking transactions
- Reconciliation can only be completed when difference equals $0.00

## CLI Commands

### Start Reconciliation

```bash
tmoney --start-reconcile --account "Checking" \
  --statement-date 2024-01-31 \
  --statement-balance 5234.56
```

Output:
```
Reconciliation started for Checking
Statement date: 2024-01-31
Statement balance: $5,234.56
Unreconciled transactions: 7
```

### Mark Transactions as Reconciled

```bash
tmoney --mark-reconciled <txn-id-1> <txn-id-2> <txn-id-3>
```

Output:
```
Marked 3 transactions for reconciliation
Current difference: $3,286.30
```

### Finish Reconciliation

```bash
tmoney --finish-reconcile --account "Checking"
```

Output (success):
```
Reconciliation completed for Checking
  Statement date: 2024-01-31
  Transactions reconciled: 7
  Balance: $5,234.56
```

Output (failure):
```
Error: Cannot complete reconciliation. Difference: $3,286.30
Use --mark-reconciled to mark additional transactions, or --force to complete anyway.
```

The `--force` flag allows completing with a non-zero difference (for edge cases).

### Reconciliation Status

```bash
tmoney --reconcile-status --account "Checking"
```

Output:
```
RECONCILIATION STATUS: Checking
================================
Last reconciled: 2024-01-31 (balance: $5,234.56)
Current session: In progress
  Statement date: 2024-02-28
  Statement balance: $6,123.45
  Checked: 5 transactions
  Difference: $234.56
```

## Balance Display Changes

With reconciliation, account details show additional balance:

```
ACCOUNT: Checking
=================
Current Balance:     $5,234.56
Cleared Balance:     $5,134.56
Reconciled Balance:  $4,500.00
Last Reconciled:     2024-01-31
```

## Validation Rules

1. Statement date is required and must not be in the future
2. Statement balance is required
3. Only one active reconciliation session per account
4. Cannot reconcile a closed account
5. Reconciliation can only complete when difference is $0.00 (unless `--force`)
6. Transactions dated after statement date are not included
