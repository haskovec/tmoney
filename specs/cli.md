# CLI Interface Specification

## Overview

TMoney supports two interface modes:

1. **TUI Mode** (default): Interactive terminal user interface — launched when `tmoney` is run with no subcommand
2. **CLI Mode**: A Cobra-based noun-verb command tree for scripting and automation

This document specifies the CLI mode. The migration from the original flag-style CLI to Cobra noun-verb subcommands is complete; see [`cli-router.md`](cli-router.md) for the router's design and historical migration plan.

## Invocation

```bash
# Launch the TUI with the last-used (or default) file
tmoney

# Launch the TUI with a specific file
tmoney ~/Documents/TMoney/personal.tdb
tmoney --file ~/Documents/TMoney/personal.tdb

# Run a CLI subcommand
tmoney account list --file personal.tdb
tmoney report net-worth --file personal.tdb
```

## Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--file <path>` | `-f` | Database file path (persistent — accepted by every subcommand) |
| `--help` | `-h` | Show help for a command |
| `--version` | (none) | Use `tmoney version` to print version information |

`--file`/`-f` is a persistent flag on the root command, so every CLI subcommand inherits it without re-declaring it. CLI subcommands that need a database (i.e., almost all of them) require either `--file` to be set or a positional file argument; they do not fall back to the last-used file in config.

## Database File Handling

1. If `--file` is supplied, use that file
2. Otherwise, if a positional file argument is supplied at the root, use it (TUI invocation)
3. Otherwise, fall back to the last opened file (stored in `~/.config/tmoney/config.json`)
4. If no last file is recorded, the TUI prompts to create or open one

CLI subcommands always require step 1 — they don't fall back to the last-used file.

```bash
tmoney personal.tdb                    # Relative path (TUI)
tmoney ~/Documents/TMoney/personal.tdb # Absolute path (TUI)
tmoney -f /path/to/file.tdb            # Persistent flag (CLI or TUI)
```

---

## Subcommands

Subcommands are listed alphabetically by noun, then alphabetically by verb. The persistent `--file`/`-f` flag is inherited by every subcommand and is not repeated below.

## `account`

Manage accounts. Account types: `checking`, `savings`, `credit_card`, `investment`, `cash`, `loan`, `asset`.

### `account add`

`Use: account add` · `Args: NoArgs`

Create a new account. `--name` and `--type` are required; other fields take sensible defaults.

**Required flags:** `--name`, `--type`

**Optional flags:**
- `--currency string` — Currency code (default `USD`)
- `--opening-balance string` — Opening balance (default `0`)
- `--opening-date string` — Opening date `YYYY-MM-DD` (default today)
- `--institution string` — Institution name
- `--account-number string` — Account number
- `--notes string` — Free-form notes
- `--credit-limit string` — Credit limit (credit-card accounts only)
- `--interest-rate string` — Interest rate / APR (loan accounts only)
- `--track-lots` — Track individual tax lots (`investment`/`hsa` accounts only; default on for those types). Pass `--track-lots=false` to opt out and use the average-cost path instead. To enable lots on an *existing* account (with a historical backfill), use `investment enable-lots`, not `account edit`.

```bash
tmoney account add --name "Chase Checking" --type checking \
  --currency USD --opening-balance 1000.00 --opening-date 2024-01-15 \
  --institution "Chase Bank" --account-number 1234567890 \
  --notes "Primary checking account"
tmoney account add --name "Wealthfront IRA" --type investment --track-lots=false
```

```
Account created successfully!
  Name:            Chase Checking
  Type:            Checking
  Currency:        USD
  Opening Balance: $1,000.00
  Opening Date:    2024-01-15
  Institution:     Chase Bank
  Account Number:  1234567890
  Notes:           Primary checking account
```

### `account balance`

`Use: account balance` · `Args: NoArgs`

Show balances for every active account along with overall net worth.

```bash
tmoney account balance
```

```
BALANCES
========
Chase Checking:     $5,234.56
Savings:           $12,000.00
Visa Card:         -$1,234.56
Investment:        $45,678.90
------------------------
Net Worth:         $61,678.90
```

### `account list`

`Use: account list` · `Args: NoArgs`

List accounts. By default only active accounts are shown.

**Optional flags:**
- `--include-closed` — Include closed accounts in the listing. Closed rows are
  annotated with their close date (e.g. `Old Savings (closed 2024-03-14)`, or
  `(closed)` when the date is unknown).

```bash
tmoney account list
tmoney account list --include-closed
```

```
ACCOUNTS
========
Name                          Type          Balance      Currency
Chase Checking                checking      $5,234.56    USD
Savings                       savings       $12,000.00   USD
Visa Card                     credit_card   -$1,234.56   USD
Investment                    investment    $45,678.90   USD
Old Savings (closed 2024-03-14)  savings    $0.00        USD
```

### `account show`

`Use: account show <name>` · `Args: ExactArgs(1)`

Show full details and current balance for the named account.

```bash
tmoney account show "Chase Checking"
```

```
ACCOUNT: Chase Checking
=======================
Type:            checking
Currency:        USD
Institution:     Chase Bank
Account Number:  ****1234
Opening Date:    2020-01-15
Opening Balance: $1,000.00
Current Balance: $5,234.56
Cleared Balance: $5,134.56
Status:          Active
```

For a closed account the `Status` line shows the close date, e.g.
`Status:          Closed (2024-03-14)`. An account closed before the close-date
column existed shows `Status:          Closed (date unknown)`.

---

### `account close`

`Use: account close <name>` · `Args: ExactArgs(1)`

Close an account as of a date.

**Flags**

- `--date` — Close date `YYYY-MM-DD` (default today)

```bash
tmoney account close "Old Savings"
tmoney account close "Old Savings" --date 2024-03-14
```

The account must have a **zero balance**, and the close date must fall on or
after the account's opening date and its latest transaction, and not in the
future. Violations are hard errors and abort with a non-zero exit. A closed
account is **frozen** — no new transactions, edits, deletes, status toggles, or
transfers (a transfer is blocked if either leg is closed) — and it no longer
appears in account pickers or in the New Scheduled flows. If any scheduled
transactions still reference the account, the command prints a warning and
proceeds (those schedules are skipped on auto-post and refused on manual post
until redirected or deleted).

### `account reopen`

`Use: account reopen <name>` · `Args: ExactArgs(1)`

Reopen a closed account, clearing its close date and allowing transactions
again.

```bash
tmoney account reopen "Old Savings"
```

---

## `db`

Database file management.

### `db backup`

`Use: db backup` · `Args: NoArgs`

Create a manual backup of the database file. Manual backups are never auto-deleted by rolling retention.

```bash
tmoney -f personal.tdb db backup
```

### `db create`

`Use: db create <path>` · `Args: ExactArgs(1)`

Create a new database file at `<path>` with default categories initialized. The `.tdb` extension is appended automatically if absent. Refuses to overwrite an existing file.

```bash
tmoney db create ~/Documents/TMoney/finances.tdb
```

```
Created database: /Users/you/Documents/TMoney/finances.tdb
```

### `db list-backups`

`Use: db list-backups` · `Args: NoArgs`

List every backup found alongside the database, newest first, with timestamp, size, and type (auto or manual).

```bash
tmoney -f personal.tdb db list-backups
```

### `db restore`

`Use: db restore <backup-path>` · `Args: ExactArgs(1)`

Restore the database from the specified backup file. A safety backup of the current state is created first so the restore can be undone.

```bash
tmoney -f personal.tdb db restore /path/to/backup.tdb
```

---

## `export`

`Use: export <file>` · `Args: ExactArgs(1)`

Export transactions to a CSV or QIF file. Format is auto-detected from the output extension unless `--format` is supplied.

**Optional flags:**
- `--account string` — Limit export to a single account
- `--from string` — Earliest transaction date (`YYYY-MM-DD`)
- `--to string` — Latest transaction date (`YYYY-MM-DD`)
- `--format string` — Override format detection (`csv` or `qif`)

```bash
tmoney -f personal.tdb export finances.csv
tmoney -f personal.tdb export checking_q1.csv --account Checking \
  --from 2024-01-01 --to 2024-03-31
tmoney -f personal.tdb export out.txt --format qif
```

---

## `import`

`Use: import <file>` · `Args: ExactArgs(1)`

Import transactions from a CSV, QIF, or OFX/QFX file into a target account. By default the command runs as a dry-run preview; pass `--confirm` to write changes.

**Required flags:** `--account`

**Optional flags:**
- `--source-account string` — Source account name (required when the file covers multiple accounts, e.g. Quicken Mac's Register Transactions export)
- `--format string` — Override format detection (`csv`, `qif`, or `ofx`)
- `--confirm` — Execute the import (default is dry-run preview)
- `--skip-duplicates` — Skip rows that match existing transactions
- `--update-duplicates` — Update existing transactions when matched

```bash
tmoney -f personal.tdb import statements.qif --account Checking
tmoney -f personal.tdb import bank.csv --account Checking --confirm
tmoney -f personal.tdb import register.csv --account "BoA Checking" \
  --source-account "Checking" --confirm
```

---

## `investment`

Investment-account operations: trades, cash flow, corporate actions, and portfolio reporting.

> **Note**: `investment deposit` and `investment withdraw` are
> one-sided cash flows (no linked counterpart in another account).
> Linked cash transfers involving an investment account — whether
> bank↔investment or investment↔investment (e.g. IRA-to-IRA cash
> rollovers) — go through the unified `tmoney transfer add` command,
> which dispatches by the (from, to) account types. See
> [`transfer add`](#transfer-add) below.
> The existing `investment transfer` (no `-cash` suffix) moves
> **shares** between two investment accounts, not cash.

### `investment buy`

`Use: investment buy` · `Args: NoArgs`

Buy shares of a security in an investment account. Supply either `--amount` (total cost) or `--price-per-share`, or both. Cash is debited from the account; if lot tracking is enabled, a new lot is opened.

**Required flags:** `--account`, `--ticker`, `--shares`

**Optional flags:** `--amount`, `--price-per-share`, `--commission` (default 0), `--date` (default today), `--memo`, `--catch-up-splits`

`--catch-up-splits` repairs a back-dated buy on a lot-tracked account: after recording the buy at its real (pre-split) shares and price, it applies any split / reverse-split dated on or after the buy to the new lot only — as if the buy had preceded those splits. No-op on a non-lot account or when no later split exists.

```bash
tmoney investment buy --account Brokerage --ticker AAPL --shares 10 --amount 1500
tmoney investment buy --account Brokerage --ticker AAPL --shares 10 --price-per-share 150
# Repair a buy entered after a split was already recorded (enter raw pre-split shares):
tmoney investment buy --account Brokerage --ticker AAPL --shares 25 --amount 2247.49 \
  --commission 9.99 --date 2007-08-14 --catch-up-splits
```

### `investment deposit`

`Use: investment deposit` · `Args: NoArgs`

Deposit cash into an investment account. Cash is credited; share counts are unchanged.

**Required flags:** `--account`, `--amount`

**Optional flags:** `--date` (default today), `--memo`

```bash
tmoney investment deposit --account Brokerage --amount 5000 --memo "Initial funding"
```

### `investment dividend`

`Use: investment dividend` · `Args: NoArgs`

Record a cash dividend received for a security. Cash is credited; share count is unchanged.

**Required flags:** `--account`, `--ticker`, `--amount`

**Optional flags:** `--date` (default today), `--memo`

```bash
tmoney investment dividend --account Brokerage --ticker AAPL --amount 125.50
```

### `investment enable-lots`

`Use: investment enable-lots` · `Args: NoArgs`

Enable lot tracking on an existing investment (or HSA) account and backfill its lots from the transaction ledger. Buys, reinvested dividends, and inbound share transfers open lots; sells, liquidating fees, and outbound transfers consume open lots by the chosen method, writing the junction rows that realized-gain math reads. Once enabled, every security in the account routes through the lot path. This is a heavy, near-irreversible operation — run `db backup` first.

By default the command prints the planned summary and makes no changes; pass `--confirm` to execute the backfill.

**Optional flags:**
- `--account string` — Limit to a single investment account by name
- `--all` — Enable lots on every investment/HSA account that isn't already lot-tracked
- `--method string` — Sell-allocation method: `fifo`, `lifo`, or `hifo` (default `fifo`)
- `--confirm` — Execute the backfill (default is a planned-summary preview)

Exactly one of `--account` or `--all` is required.

The command refuses to run when the target account is not an investment/HSA account, or when it already has lots (lots are never double-created). It also refuses, per account, any account that holds a security with recorded corporate actions (splits, mergers, spin-offs), since the naive replay cannot reproduce those — a corporate action on a security held in an *unrelated* account does not block accounts that don't hold it. A sell that open lots can't fully cover is reported as a shortfall in the summary rather than aborting the run.

```bash
tmoney -f personal.tdb investment enable-lots --account "Wealthfront IRA"
tmoney -f personal.tdb investment enable-lots --account "Wealthfront IRA" --confirm
tmoney -f personal.tdb investment enable-lots --account Brokerage --method hifo --confirm
tmoney -f personal.tdb investment enable-lots --all --confirm
```

The summary reports lots created per security, sells matched, any uncovered-share shortfalls, and a reminder that realized gain on past sells is now lot-exact for the chosen method.

### `investment fee`

`Use: investment fee` · `Args: NoArgs`

Record a fee in an investment account. Cash is debited; share counts are unchanged.

**Required flags:** `--account`, `--amount`

**Optional flags:** `--date` (default today), `--memo`

```bash
tmoney investment fee --account Brokerage --amount 25 --memo "Annual fee"
```

### `investment merge`

`Use: investment merge` · `Args: NoArgs`

Apply a merger or acquisition: every share of the source security is exchanged for `--exchange-ratio` shares of the target, optionally with cash consideration.

**Required flags:** `--source`, `--target`, `--exchange-ratio`

**Optional flags:** `--cash-per-share`, `--date` (default today)

```bash
tmoney investment merge --source AAPL --target GOOG --exchange-ratio 0.5
tmoney investment merge --source AAPL --target GOOG --exchange-ratio 0.5 \
  --cash-per-share 10.50
```

### `investment portfolio`

`Use: investment portfolio` · `Args: NoArgs`

Show holdings, market value, cost basis, and gain/loss for an investment account.

**Required flags:** `--account`

**Optional flags:**
- `--as-of string` — Valuation date `YYYY-MM-DD` (default today)
- `--show-lots` — Show per-lot detail (lot-tracking accounts only)

```bash
tmoney investment portfolio --account Brokerage
tmoney investment portfolio --account Brokerage --as-of 2024-12-31
tmoney investment portfolio --account Brokerage --show-lots
```

### `investment rebuild-positions`

`Use: investment rebuild-positions` · `Args: NoArgs`

Recompute the stored `investment_positions` rows (and, for lot-tracking accounts, each lot's `shares` / `closed` fields) from the transaction ledger and lot junction records. Use this to recover from a desynced state after an aborted edit. Corporate actions are handled **per-security**, not all-or-nothing: a security touched only by stock splits is replayed normally (the split's dated ratio is interleaved into the replay), so a clean holding heals even on a database that contains corporate-action history. Only securities with a **merger or spin-off** (cross-security transforms a per-security replay can't reconstruct) are skipped, as are lot-tracked securities with a split (left to their lots so per-lot repairs survive). See [`transactions.md`](transactions.md#investment-transactions).

**Required flags:** none

**Optional flags:**
- `--account string` — Limit rebuild to a single investment account (default: all)

```bash
tmoney -f personal.tdb investment rebuild-positions
tmoney -f personal.tdb investment rebuild-positions --account Brokerage
```

### `investment reinvest`

`Use: investment reinvest` · `Args: NoArgs`

Reinvest a dividend into additional shares: shares are added to the position and no cash leaves the account. Supply either `--amount` (total) or `--price-per-share`, or both.

**Required flags:** `--account`, `--ticker`, `--shares`

**Optional flags:** `--amount`, `--price-per-share`, `--date` (default today), `--memo`

```bash
tmoney investment reinvest --account Brokerage --ticker AAPL \
  --shares 2 --price-per-share 150
tmoney investment reinvest --account Brokerage --ticker AAPL \
  --shares 2 --amount 300
```

### `investment sell`

`Use: investment sell` · `Args: NoArgs`

Sell shares of a security in an investment account. Supply either `--amount` (total proceeds) or `--price-per-share`, or both. For lot-tracked accounts, pass `--lot` to allocate against a specific open lot.

**Required flags:** `--account`, `--ticker`, `--shares`

**Optional flags:** `--amount`, `--price-per-share`, `--commission` (default 0), `--date` (default today), `--memo`, `--lot`

```bash
tmoney investment sell --account Brokerage --ticker AAPL \
  --shares 5 --price-per-share 160
tmoney investment sell --account Brokerage --ticker AAPL \
  --shares 5 --amount 800 --commission 10
```

### `investment spin-off`

`Use: investment spin-off` · `Args: NoArgs`

Apply a corporate spin-off: a parent security distributes a new child security to existing holders, with the parent's cost basis split between the two by the parent-allocation percentage.

**Required flags:** `--parent`, `--spinoff`, `--share-ratio`, `--parent-allocation`, `--spin-off-price`

**Optional flags:** `--date` (default today)

```bash
tmoney investment spin-off --parent AAPL --spinoff GOOG \
  --share-ratio 0.5 --parent-allocation 80 --spin-off-price 25
```

### `investment split`

`Use: investment split` · `Args: NoArgs`

Apply a stock split (`4:1` forward) or reverse split (`1:10`) to a security. All open positions and (if lot tracking is enabled) lots are adjusted.

**Required flags:** `--ticker`, `--ratio`

**Optional flags:** `--date` (default today)

```bash
tmoney investment split --ticker AAPL --ratio 4:1
tmoney investment split --ticker AAPL --ratio 1:10 --date 2025-01-15
```

### `investment split-lot`

`Use: investment split-lot` · `Args: NoArgs`

Apply a split ratio to a **single** lot, identified by its lot ID — a repair for a lot entered after a security-wide `investment split` had already run (so the global split never scaled it). Scales that lot's `shares`, `original_shares`, and `cost_per_share`, recomputes the account's position from its lots, and records no corporate action. The lot must not have been sold against, and the security must already have a recorded split. Find lot IDs with `investment portfolio --account NAME --show-lots`. For the common back-dated-buy case, prefer `investment buy --catch-up-splits`.

**Required flags:** `--lot`, `--ratio`

```bash
tmoney investment split-lot --lot 019e9fea-463f-75bc-9044-cd6f10bb53f0 --ratio 2:1
```

### `investment transfer`

`Use: investment transfer` · `Args: NoArgs`

Transfer shares of a security from one investment account to another. No cash changes hands. For lot-tracked source accounts, pass `--lot` to allocate against a specific open lot.

**Required flags:** `--from`, `--to`, `--ticker`, `--shares`

**Optional flags:** `--date` (default today), `--memo`, `--lot`

```bash
tmoney investment transfer --from "Source IRA" --to "Dest 401k" \
  --ticker AAPL --shares 5
tmoney investment transfer --from Brokerage --to RolloverIRA \
  --ticker VTI --shares 100 --date 2025-04-15
```

### `investment withdraw`

`Use: investment withdraw` · `Args: NoArgs`

Withdraw cash from an investment account. Cash is debited; share counts are unchanged.

**Required flags:** `--account`, `--amount`

**Optional flags:** `--date` (default today), `--memo`

```bash
tmoney investment withdraw --account Brokerage --amount 500 --memo "Quarterly draw"
```

---

## `price`

Manage and refresh security prices.

### `price add`

`Use: price add` · `Args: NoArgs`

Record a price for a security on a specific date. The source is set to `manual`. Pass `--fetch` to look the price up from a provider for the given date instead of supplying it by hand (stored with `source = api`).

**Required flags:** `--ticker`, `--date`, and `--price` (omit `--price` when using `--fetch`)

**Optional flags:**
- `--fetch` — Fetch the closing price for `--date` from a provider instead of passing `--price`
- `--provider string` — Price provider name (default `yahoo`; used with `--fetch`)

```bash
tmoney price add --ticker AAPL --date 2024-01-15 --price 150.00
tmoney price add --ticker AAPL --date 2024-01-15 --fetch
```

### `price cleanup`

`Use: price cleanup` · `Args: NoArgs`

Remove transaction-sourced prices justified only by a reinvested dividend or fee liquidation (no buy or sell on that date) — the rows the buy/sell-only auto-price policy no longer creates. Prints a dry-run plan by default; pass `--confirm` to apply. With `--refetch`, tickered securities are replaced by the provider's close for that date instead of deleted (tickerless securities and fetch failures are left in place). Run `tmoney db backup` first.

**Optional flags:**
- `--confirm` — Apply the cleanup (default: dry-run preview only)
- `--refetch` — Replace tickered prices with the provider's close for that date instead of deleting
- `--provider string` — Price provider for `--refetch` (default `yahoo`)
- `--ticker` / `--isin` / `--name` — Limit to one security (default: all securities)

```bash
tmoney price cleanup
tmoney price cleanup --confirm
tmoney price cleanup --refetch --confirm
```

### `price current`

`Use: price current <ticker>` · `Args: ExactArgs(1)`

Show the most recent price recorded for the security.

```bash
tmoney price current AAPL
```

### `price delete`

`Use: price delete` · `Args: NoArgs`

Remove the recorded price for a security on a specific date — the CLI counterpart to deleting a price in the TUI prices view (both call `price.Service.DeletePrice`). Identify the security with `--ticker`, `--isin`, or `--name`. Errors if no price exists for that date.

**Required flags:** `--date`, plus a security selector (`--ticker`, `--isin`, or `--name`)

```bash
tmoney price delete --ticker AAPL --date 2024-01-15
tmoney price delete --name "MFS Mid Cap Value CT" --date 2024-01-15
```

### `price import`

`Use: price import <file>` · `Args: ExactArgs(1)`

Bulk-import prices from a CSV file. The CSV must have `Date`, `Ticker`, and `Price` columns.

**Optional flags:**
- `--overwrite` — Overwrite existing prices on matching dates

```bash
tmoney price import prices.csv
tmoney price import prices.csv --overwrite
```

### `price list`

`Use: price list <ticker>` · `Args: ExactArgs(1)`

List the price history for a security, optionally filtered by date range.

**Optional flags:**
- `--from string` — Earliest date `YYYY-MM-DD` (inclusive)
- `--to string` — Latest date `YYYY-MM-DD` (inclusive)

```bash
tmoney price list AAPL
tmoney price list AAPL --from 2024-01-01 --to 2024-06-30
```

### `price lookup`

`Use: price lookup` · `Args: NoArgs`

Fetch the closing price for a security on a specific date from a provider and print it, without recording anything. The provider returns the close on or before the requested date, so weekends and holidays resolve to the prior trading day. Use this to sanity-check a value before recording it with `price add --fetch`.

**Required flags:** `--ticker`, `--date`

**Optional flags:**
- `--provider string` — Price provider name (default `yahoo`)

```bash
tmoney price lookup --ticker AAPL --date 2024-01-15
tmoney price lookup --ticker GBTC --date 2024-07-31 --provider yahoo
```

### `price update`

`Use: price update [tickers...]` · `Args: ArbitraryArgs`

Refresh closed-session prices from a provider for every visible security with a ticker, or only the supplied tickers. Re-running the same day is a no-op.

**Optional flags:**
- `--provider string` — Price provider name (default `yahoo`)

```bash
tmoney price update
tmoney price update AAPL MSFT
tmoney price update --provider yahoo
```

---

## `reconcile`

Statement reconciliation against an account.

### `reconcile finish`

`Use: reconcile finish` · `Args: NoArgs`

Complete the active reconciliation session. Refuses to finish when the cleared total does not match the statement balance unless `--force` is supplied.

**Required flags:** `--account`

**Optional flags:**
- `--force` — Finish even if the cleared total differs from the statement balance

```bash
tmoney -f personal.tdb reconcile finish --account Checking
tmoney -f personal.tdb reconcile finish --account Checking --force
```

### `reconcile mark`

`Use: reconcile mark <id>...` · `Args: MinimumNArgs(1)`

Mark one or more transactions as part of the active reconciliation session. Reports the running difference between the cleared total and the statement balance.

```bash
tmoney -f personal.tdb reconcile mark <id1> <id2>
```

### `reconcile start`

`Use: reconcile start` · `Args: NoArgs`

Start a reconciliation session against a statement: records the statement date and balance and reports how many unreconciled transactions are eligible to be marked.

**Required flags:** `--account`, `--statement-date`, `--statement-balance`

```bash
tmoney -f personal.tdb reconcile start --account Checking \
  --statement-date 2024-01-31 --statement-balance 850.00
```

For liability accounts (credit card, loan), enter the statement balance
**negated** — servicer statements print a positive amount owed, but
liability balances are stored negative (see `specs/accounts.md`):

```bash
tmoney -f personal.tdb reconcile start --account Visa \
  --statement-date 2024-01-31 --statement-balance -850.00
```

### `reconcile status`

`Use: reconcile status` · `Args: NoArgs`

Show the last completed reconciliation and any active session for the named account.

**Required flags:** `--account`

```bash
tmoney -f personal.tdb reconcile status --account Checking
```

---

## `report`

Reports.

### `report net-worth`

`Use: report net-worth` · `Args: NoArgs`

Generate a net-worth report summarizing total assets, total liabilities, and net worth.

**Optional flags:**
- `--as-of string` — Valuation date `YYYY-MM-DD` (default today)
- `--include-closed` — Include closed accounts in the report

```bash
tmoney report net-worth
tmoney report net-worth --as-of 2024-06-30
tmoney report net-worth --include-closed
```

```
NET WORTH REPORT
================
As of: 2024-01-15

ASSETS
------
Chase Checking:     $5,234.56
Savings:           $12,000.00
Investment:        $45,678.90
------------------------
Total Assets:      $62,913.46

LIABILITIES
-----------
Visa Card:          $1,234.56
------------------------
Total Liabilities:  $1,234.56

NET WORTH:         $61,678.90
```

### `report spending`

`Use: report spending` · `Args: NoArgs`

Generate a spending-by-category report for a given period. Specify the period with exactly one of `--month`, `--year`, or `--from`/`--to`.

**Optional flags:**
- `--month string` — Period as `YYYY-MM` (e.g. `2024-03`)
- `--year int` — Period as `YYYY` (e.g. `2024`)
- `--from string` — Start of custom date range (`YYYY-MM-DD`)
- `--to string` — End of custom date range (`YYYY-MM-DD`); defaults to today

```bash
tmoney report spending --month 2024-03
tmoney report spending --year 2024
tmoney report spending --from 2024-01-01 --to 2024-06-30
```

```
SPENDING BY CATEGORY: January 2024
==================================

Category              Amount      % of Total
--------              ------      ----------
Housing               $1,500.00   45.2%
  Rent                $1,500.00
Food                    $523.45   15.8%
  Groceries             $423.45
  Restaurants           $100.00
Transportation          $345.67   10.4%
  Gas                   $245.67
  Maintenance           $100.00
------------------------
Total Spending:       $3,320.12
```

---

## `scheduled`

Scheduled (recurring) transactions.

### `scheduled add`

`Use: scheduled add` · `Args: NoArgs`

Create a new scheduled transaction. Frequencies: `daily`, `weekly`, `biweekly`, `monthly`, `quarterly`, `yearly`. Omit `--amount` for a variable-amount schedule.

**Required flags:** `--account`, `--frequency`

**Optional flags:**
- `--amount string` — Scheduled amount; omit for a variable-amount schedule
- `--payee string` — Payee name (auto-created if it doesn't exist)
- `--category string` — Category (`Parent` or `Parent:Subcategory`)
- `--date string` — Start date `YYYY-MM-DD` (default today)
- `--memo string` — Free-form memo
- `--day int` — Day of month (1–31, or `-1` for last day of month)
- `--occurrences int` — Number of occurrences (omit for indefinite)
- `--end-date string` — End date `YYYY-MM-DD` (omit for indefinite)
- `--auto-post` — Post automatically when due
- `--lead-days int` — Auto-post lead days: `0`, `3`, or `7` (requires `--auto-post`)

```bash
tmoney scheduled add --account Checking --frequency monthly \
  --amount -1500 --payee Landlord --memo "Monthly rent" --day 1
tmoney scheduled add --account Checking --frequency monthly --payee "Electric Co"
tmoney scheduled add --account Checking --frequency monthly --amount -150 \
  --payee Insurance --auto-post --lead-days 3
```

### `scheduled list`

`Use: scheduled list` · `Args: NoArgs`

List scheduled transactions on the database.

**Optional flags:**
- `--account string` — Filter by account name
- `--due` — Only show occurrences due today or earlier

```bash
tmoney scheduled list
tmoney scheduled list --account Checking
tmoney scheduled list --due
```

```
SCHEDULED TRANSACTIONS
======================
ID       Next Date   Payee           Account         Amount      Frequency
abc123   2024-01-15  Landlord        Checking        -$1,500.00  Monthly
def456   2024-01-20  Electric Co     Checking        ~$120.00    Monthly
ghi789   2024-01-31  Auto Finance    Checking        -$450.00    Monthly (48 left)

[DUE] 1 scheduled transaction is due
```

### `scheduled post`

`Use: scheduled post <id>` · `Args: ExactArgs(1)`

Post a scheduled transaction (creates a real transaction from it and advances the schedule). `--amount` overrides a variable schedule's posted amount; `--date` overrides the posted date.

**Optional flags:**
- `--amount string` — Override the posted amount (e.g. `-150.00`)
- `--date string` — Override the posted date (`YYYY-MM-DD`)

```bash
tmoney scheduled post <id>
tmoney scheduled post <id> --amount 150.00
tmoney scheduled post <id> --date 2024-03-20
```

### `scheduled skip`

`Use: scheduled skip <id>` · `Args: ExactArgs(1)`

Advance a scheduled transaction to its next occurrence without creating a real transaction.

```bash
tmoney scheduled skip <id>
```

---

## `security`

Manage securities (stocks, ETFs, mutual funds). Security types: `stock`, `etf`, `mutual_fund`, `other`.

### `security add`

`Use: security add` · `Args: NoArgs`

Create a new security. `--ticker`, `--name`, and `--type` are required.

**Required flags:** `--ticker`, `--name`, `--type`

**Optional flags:**
- `--asset-class string` — Asset class (default `unclassified`)
- `--currency string` — Currency code (default `USD`)
- `--exchange string` — Exchange (e.g. `NASDAQ`, `NYSE`)

```bash
tmoney security add --ticker AAPL --name "Apple Inc." --type stock
tmoney security add --ticker VTI --name "Vanguard Total Stock Market" \
  --type etf --asset-class large_cap_stock --exchange NYSE
```

### `security delete`

`Use: security delete <ticker>` · `Args: ExactArgs(1)`

Delete a security. Refuses to delete securities with dependent prices or transactions; use `security hide` instead.

```bash
tmoney security delete AAPL
```

### `security edit`

`Use: security edit <ticker>` · `Args: ExactArgs(1)`

Edit fields on an existing security. Only flags that are supplied take effect. Pass `--ticker` to rename to a new symbol.

**Optional flags:** `--ticker`, `--name`, `--type`, `--asset-class`, `--currency`, `--exchange`

```bash
tmoney security edit AAPL --name "Apple Corporation"
tmoney security edit AAPL --ticker AAPL2
tmoney security edit VTI --asset-class total_market
```

### `security hide`

`Use: security hide <ticker>` · `Args: ExactArgs(1)`

Mark a security hidden so it no longer appears in default listings. Data is preserved; restore with `security unhide`.

```bash
tmoney security hide AAPL
```

### `security list`

`Use: security list` · `Args: NoArgs`

List securities. Hidden securities are excluded by default.

**Optional flags:**
- `--include-hidden` — Include hidden securities
- `--type string` — Filter by type (`stock`, `etf`, `mutual_fund`, `other`)
- `--asset-class string` — Filter by asset class

```bash
tmoney security list
tmoney security list --include-hidden
tmoney security list --type etf --asset-class large_cap_stock
```

### `security show`

`Use: security show <ticker>` · `Args: ExactArgs(1)`

Show full details for the security.

```bash
tmoney security show AAPL
```

### `security unhide`

`Use: security unhide <ticker>` · `Args: ExactArgs(1)`

Restore a hidden security to default listings.

```bash
tmoney security unhide AAPL
```

---

## `theme`

Manage TUI themes.

### `theme generate-from-wal`

`Use: theme generate-from-wal` · `Args: NoArgs`

Generate a theme TOML from the active pywal palette (`~/.cache/wal/colors.json`, XDG_CACHE_HOME aware). Default output is `$XDG_CONFIG_HOME/tmoney/themes/wal.toml`.

**Optional flags:**
- `-o`, `--output string` — Output path (`-` for stdout)

```bash
tmoney theme generate-from-wal
tmoney theme generate-from-wal --output -
tmoney theme generate-from-wal --output /tmp/wal.toml
```

### `theme list`

`Use: theme list` · `Args: NoArgs`

List built-in and user themes. The active theme (per config) is marked with `*`.

```bash
tmoney theme list
```

---

## `transaction`

Per-account transaction management.

### `transaction add`

`Use: transaction add` · `Args: NoArgs`

Create a new transaction. Use a negative amount for an expense, positive for income.

**Required flags:** `--account`, `--amount`

**Optional flags:**
- `--payee string` — Payee name (auto-created if it doesn't exist)
- `--category string` — Category (`Parent` or `Parent:Subcategory`)
- `--date string` — Transaction date `YYYY-MM-DD` (default today)
- `--memo string` — Free-form memo

```bash
tmoney transaction add --account Checking --amount -50.00 --payee "Coffee Shop"
tmoney transaction add --account Checking --amount -120.00 \
  --payee "Electric Co" --category "Bills:Utilities" \
  --date 2024-03-15 --memo "March electric bill"
```

### `transaction list`

`Use: transaction list` · `Args: NoArgs`

List transactions for an account.

**Required flags:** `--account`

**Optional flags:**
- `--limit int` — Maximum number of transactions to display (`0` = no limit)
- `--from string` — Earliest date (`YYYY-MM-DD`)
- `--to string` — Latest date (`YYYY-MM-DD`)
- `--status string` — Filter by status: `uncleared`, `cleared`, `reconciled`, `void`
- `--show-ids` — Prefix each row with the transaction's UUID (off by default). Useful for scripting `transfer edit` / `transfer delete` against discovered transaction IDs.

```bash
tmoney transaction list --account Checking
tmoney transaction list --account Checking --limit 20
tmoney transaction list --account Checking --from 2024-01-01 --to 2024-01-31
tmoney transaction list --account Checking --show-ids
```

```
TRANSACTIONS: Chase Checking
============================
Date        Payee              Category            Amount      Balance
2024-01-15  Kroger             Food:Groceries      -$125.43    $5,234.56
2024-01-14  Employer Inc       Income:Salary      $2,500.00    $5,359.99
2024-01-12  Amazon             Shopping:General    -$45.99     $2,859.99
```

### `transaction search`

`Use: transaction search <term>` · `Args: ExactArgs(1)`

Search transactions whose payee or memo contains `<term>`.

**Optional flags:**
- `--account string` — Limit to this account
- `--category string` — Limit to this category
- `--from string` — Earliest date (`YYYY-MM-DD`)
- `--to string` — Latest date (`YYYY-MM-DD`)
- `--min string` — Minimum amount
- `--max string` — Maximum amount
- `--show-ids` — Prefix each row with the transaction's UUID (off by default). Useful for scripting `transfer edit` / `transfer delete` against discovered transaction IDs.

```bash
tmoney transaction search "grocery"
tmoney transaction search "electric" --from 2024-01-01 --to 2024-12-31
tmoney transaction search "restaurant" --account Visa --min 20 --max 100
tmoney transaction search "amazon" --show-ids
```

```
SEARCH RESULTS: "amazon"
========================
Account          Date        Payee    Category          Amount
Chase Checking   2024-01-12  Amazon   Shopping:General  -$45.99
Chase Checking   2024-01-05  Amazon   Shopping:General  -$23.45
Visa Card        2023-12-28  Amazon   Shopping:General  -$156.78
```

### `transaction void`

`Use: transaction void <id>` · `Args: ExactArgs(1)`

Void a transaction by ID. If the transaction is part of a transfer, the counterpart is voided as well.

```bash
tmoney transaction void <id>
```

---

## `transfer`

Transfers between two accounts.

### `transfer add`

`Use: transfer add` · `Args: NoArgs`

Create a transfer between two accounts. The command dispatches
internally by the `(from.Type, to.Type)` combination so any pairing
works — bank↔bank, bank↔investment, investment↔bank, and
investment↔investment. HSA accounts count as investment on either
leg.

| From → To | Service method |
|---|---|
| reg → reg | `transaction.Service.CreateTransfer` |
| reg → inv | `investment.Service.DepositFromAccount` |
| inv → reg | `investment.Service.TransferCash` |
| inv → inv | `investment.Service.TransferCashBetweenInvestments` |

**Required flags:** `--from`, `--to`, `--amount` (must be positive)

**Optional flags:**
- `--date string` — Transfer date `YYYY-MM-DD` (default today)
- `--memo string` — Free-form memo

```bash
# Bank → bank
tmoney transfer add --from Checking --to Savings --amount 500.00
tmoney transfer add --from Checking --to Savings --amount 500.00 \
  --date 2024-03-01 --memo "Monthly savings"

# Bank → investment (e.g. 401k contribution)
tmoney transfer add --from Checking --to "Brokerage" --amount 1000.00

# Investment → bank (e.g. brokerage withdrawal)
tmoney transfer add --from "Brokerage" --to Checking --amount 250.00

# Investment → investment (e.g. IRA-to-IRA rollover)
tmoney transfer add --from "Old IRA" --to "Rollover IRA" --amount 5000.00
```

The success confirmation always prints the new transfer-id and both
leg transaction-ids so scripts composing follow-up edits/deletes have
the IDs ready to use.

### `transfer edit`

`Use: transfer edit` · `Args: NoArgs`

Edit a whole-transaction transfer identified by the UUID of either leg
(`--txn-id`). Use `tmoney transaction list --show-ids` to find the ID.
Only the supplied flags take effect (matching `security edit`); at
least one editable flag is required. From/To accounts are not editable
— delete and re-add to move a transfer between accounts. Dispatches by
account type the same way `transfer add` does, so every combination
(bank↔bank, bank↔investment, investment↔investment) is supported.

**Required flags:** `--txn-id`

**Editable flags (at least one required):**
- `--amount string` — New transfer amount (must be positive)
- `--date string` — New transfer date `YYYY-MM-DD`
- `--memo string` — New memo
- `--status string` — New status: `cleared` or `uncleared`

`--status reconciled` is rejected — reconciling is owned by `tmoney
reconcile`. Reconciled transfers cannot be edited. Transfer-line
splits (e.g. a paycheck's 401k contribution line) are refused with a
pointer to the TUI.

```bash
tmoney -f personal.tdb transfer edit --txn-id <uuid> --amount 600.00
tmoney -f personal.tdb transfer edit --txn-id <uuid> --date 2024-04-01 --memo "fixed date"
tmoney -f personal.tdb transfer edit --txn-id <uuid> --status cleared
```

### `transfer delete`

`Use: transfer delete` · `Args: NoArgs`

Delete both legs of a whole-transaction transfer identified by the
UUID of either leg (`--txn-id`). Use `tmoney transaction list
--show-ids` to find the ID. Dispatches by account type, so every
combination is supported. Reconciled transfers are refused; so are
transfer-line splits (edit/delete those in the TUI).

**Required flags:** `--txn-id`

```bash
tmoney -f personal.tdb transfer delete --txn-id <uuid>
```

### `transfer link`

`Use: transfer link` · `Args: NoArgs`

Scan for pairs of unlinked transactions across accounts whose amounts cancel and whose dates are within `--max-days`, then join the matched pairs into proper transfers. Default is dry-run preview.

**Optional flags:**
- `--confirm` — Execute the linking (default is dry-run preview)
- `--max-days int` — Maximum days between the two postings of a candidate pair (default 5)

```bash
tmoney -f personal.tdb transfer link
tmoney -f personal.tdb transfer link --max-days 3
tmoney -f personal.tdb transfer link --confirm
```

---

## `version`

`Use: version` · `Args: NoArgs`

Print version, build time, and git commit.

```bash
tmoney version
```

---

## Error Handling

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | File not found |
| 3 | Invalid arguments |
| 4 | Database error |

### Error Output

Errors are written to stderr:

```
Error: Account "Checking" not found
```

Cobra also produces standard error messages for argument-parsing failures (unknown subcommand, missing required flag, wrong number of positional args). Examples:

```
Error: required flag(s) "account" not set
Error: accepts 1 arg(s), received 0
```

## Date Formats

Accepted date formats:

- `YYYY-MM-DD` (preferred): `2024-01-15`
- `MM/DD/YYYY`: `01/15/2024`
- `today`: Current date
- `yesterday`: Previous date

## Amount Formats

Accepted amount formats:

- Plain number: `123.45`
- With sign: `-123.45` or `+123.45`
- With currency: `$123.45` or `USD 123.45`
- Negative expense: `-50` (money out)
- Positive income: `50` or `+50` (money in)

## Configuration

TMoney stores its configuration following the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/latest/):

- macOS / Linux: `$XDG_CONFIG_HOME/tmoney/config.json` (defaults to `~/.config/tmoney/config.json`)
- Windows: `%APPDATA%\tmoney\config.json`

Tracked options:

```json
{
  "default_file": "~/Documents/TMoney/personal.tdb",
  "recent_files": [
    "~/Documents/TMoney/personal.tdb",
    "~/Documents/TMoney/business.tdb"
  ],
  "last_file": "~/Documents/TMoney/personal.tdb",
  "theme": "default"
}
```
