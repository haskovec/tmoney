# Terminal Money

A personal finance management application that runs in the terminal. Terminal Money is designed for users who prefer keyboard-driven interfaces and want full control over their financial data.

## Core Philosophy

1. **Local-first**: Your data stays on your computer in a single file
2. **Keyboard-driven**: No mouse required, fast navigation
3. **Dual interface**: TUI for daily use, CLI for scripting and automation
4. **Simple but complete**: Essential features without bloat

## Installation

### Build from Source

Requires Go 1.26 or later.

```bash
git clone https://github.com/haskovec/tmoney.git
cd tmoney
go build -o tmoney ./cmd/tmoney
```

The resulting `tmoney` binary is self-contained with no runtime dependencies.

## Quick Start

```bash
# Create a new database file
tmoney db create ~/Documents/TMoney/personal.tdb

# Launch the TUI
tmoney ~/Documents/TMoney/personal.tdb

# Or use the CLI to add data
tmoney -f personal.tdb --add-account --name "My Checking" --type checking --opening-balance 1000
tmoney -f personal.tdb transaction add --account "My Checking" --amount -45.50 --payee "Grocery Store" --category "Food:Groceries"
tmoney -f personal.tdb --balance
```

## Features

### Accounts
- Multiple account types: checking, savings, credit card, investment, cash, loan, asset
- Per-account currency setting (USD, EUR, GBP)
- Credit limit for credit card accounts
- Interest rate (APR) for checking, savings, credit card, investment, and loan accounts
- Dynamic account dialog that shows only relevant fields for the selected account type
- Open/close account lifecycle

### Transactions
- Standard transactions with payee, category, and memo
- Split transactions across multiple categories
- Linked transfers between accounts
- Cleared/pending status tracking
- Full-text search with date, amount, and category filters

### Categories
- Two-level hierarchy (parent/subcategory, e.g. "Food:Groceries")
- Income vs expense classification
- Default categories provided on new file creation

### Payees
- Auto-creation on first use
- Default category assignment
- Alias/pattern matching for imports

### Scheduled Transactions
- Multiple frequencies: daily, weekly, biweekly, monthly, quarterly, yearly
- Fixed or indefinite duration
- Variable amount estimation
- Post or skip workflow

### Reports
- Net worth calculation (assets vs liabilities)
- Spending by category with monthly/yearly aggregation and visual bars

### Prices
- Manual entry, CSV import, and history per security
- Bulk refresh from an online provider (Yahoo Finance by default) — only stores the latest closed-session price, so reruns on the same day are idempotent

### Import / Export
- Import transactions from CSV, QIF, or OFX/QFX files (Quicken / bank
  downloads). Available from File → Import Transactions… in the TUI or
  via `--import` on the CLI. Quicken Mac users export via File →
  Export → Register Transactions to CSV File…
- Multi-account CSVs (one file containing every account, as Quicken Mac
  emits) are detected automatically: the importer asks you to pick
  which source account to import this pass and which tmoney account it
  maps to. Re-run once per source account.
- Optional duplicate detection: skip or update matched rows
- Preview-then-confirm flow shows what will be created/updated/skipped
  before any changes are written
- Export to CSV or QIF via `--export` on the CLI
- **Link Transfers** (Transactions → Link Transfers… or
  `--link-transfers`): when each side of a transfer is imported as a
  separate transaction (typical when importing one account at a time
  from Quicken), this scans for unlinked transactions across accounts
  whose amounts cancel and whose dates are within a few days, and joins
  the matched pairs into proper transfers. Pairs with multiple possible
  partners are flagged as ambiguous and left untouched for manual
  review.

## TUI Interface

Launch the TUI by running `tmoney` with a database file (or no arguments for the default file):

```bash
tmoney                                    # Default file
tmoney ~/Documents/TMoney/personal.tdb    # Specific file
```

The TUI has several views accessible via number keys or the menu bar:

| Key | View | Description |
|-----|------|-------------|
| `1` | Dashboard | Net worth, account balances, due scheduled transactions |
| `2` | Scheduled | Due and upcoming scheduled transactions |
| `3` | Reports | Net worth and spending by category reports |
| `4` | Securities | Security master list with add/edit/hide/delete and `u` to refresh prices |
| `5` | Prices | Latest price per security; Enter or double-click drills into one ticker's full history; `u` to refresh prices |
| - | Register | Transaction list for a selected account (open from Dashboard) |

### Keyboard Shortcuts

Press `?` at any time to show the help overlay.

#### Global

| Key | Action |
|-----|--------|
| `?` | Show/hide keyboard shortcuts help |
| `Ctrl+Q` | Quit application |
| `Esc` | Close dialog / go back |
| `Tab` | Next pane / next field |
| `Shift+Tab` | Previous pane / previous field |
| `/` | Search |
| `1` | Dashboard view |
| `2` | Scheduled view |
| `3` | Reports view |
| `4` | Securities view |
| `5` | Prices view |
| `F10` | Activate menu bar |
| `Alt+F` | File menu (also has Import Transactions…) |
| `Alt+E` | Edit menu |
| `Alt+V` | View menu (Theme switcher) |
| `Alt+A` | Accounts menu |
| `Alt+T` | Transactions menu (also has Link Transfers…) |
| `Alt+S` | Securities menu |
| `Alt+R` | Reports menu |
| `Alt+H` | Help menu |

#### Navigation

| Key | Action |
|-----|--------|
| `Up` / `k` | Move up |
| `Down` / `j` | Move down |
| `Left` / `h` | Previous |
| `Right` / `l` | Next |
| `Home` / `g` | Go to first item |
| `End` / `G` | Go to last item |
| `PgUp` | Page up |
| `PgDn` | Page down |

#### Dashboard

| Key | Action |
|-----|--------|
| `Enter` | Open selected account register |
| `n` | New account |
| `Up/Down` | Navigate accounts |

#### Register (Account Transactions)

| Key | Action |
|-----|--------|
| `n` | New transaction |
| `Enter` | Edit selected transaction |
| `d` | Delete transaction |
| `c` | Toggle cleared status |
| `t` | New transfer |
| `Tab` | Switch between sidebar and table |

#### Scheduled Transactions

| Key | Action |
|-----|--------|
| `Enter` | Post scheduled transaction |
| `s` | Skip occurrence |
| `e` | Edit scheduled transaction |
| `n` | New scheduled transaction |
| `d` | Delete scheduled transaction |

#### Reports

| Key | Action |
|-----|--------|
| `Left/Right` | Change period |
| `n` | Net worth report |
| `s` | Spending report |
| `y` | Yearly view |
| `m` | Monthly view |

#### Securities

| Key | Action |
|-----|--------|
| `n` | New security |
| `Enter` | Edit security |
| `h` | Toggle hidden status |
| `d` | Delete security |
| `f` | Toggle show hidden |
| `/` | Search |
| `p` | View prices for selected security |
| `u` | Update prices for all visible securities from the default provider |

#### Prices

The Prices view (`5`) opens on a summary list — one row per security
with prices, showing ticker, name, latest price, and the date of that
latest price. Drill into a ticker to see its full price history; press
Esc to return to the list.

When the prices content area is at least 120 columns wide, a chart
panel renders to the right of the list showing the highlighted
ticker's full price history. No keyboard shortcut — it tracks the
cursor automatically.

| Key | Action |
|-----|--------|
| `Enter` | (List) View history for selected ticker · (History) Edit selected price |
| `Esc` | Return to the prices list |
| `n` | New price (history view) |
| `d` | Delete selected price (history view) |
| `i` | Import prices from CSV (history view) |
| `u` | Update prices for all visible securities from the default provider |
| `/` | Search |

#### Dialogs

| Key | Action |
|-----|--------|
| `Tab` | Next field |
| `Shift+Tab` | Previous field |
| `Enter` | Submit / confirm |
| `Esc` | Cancel / close |
| `Up/Down` | Navigate options |
| `Space` | Type space in text fields / toggle checkbox |

### Mouse Support

TMoney supports mouse interaction for common operations. The general
rule is *single click selects, double click opens* — single-clicking a
list or table row only moves the cursor; a double-click on the same
row drills in (open account register, open ticker price history,
etc.). Buttons, menu items, and other affordances activate on a single
click as you'd expect.

| Action | Effect |
|--------|--------|
| Click menu label | Open/close menu dropdown |
| Click dropdown item | Execute menu action |
| Click account in sidebar | Select account |
| Double-click account in sidebar | Open account register |
| Click group header in sidebar | Select group heading |
| Click row in prices list | Select ticker |
| Double-click row in prices list | View that ticker's price history |
| Click transaction row | Select transaction |
| Click sidebar/table area | Switch focus between panes |
| Scroll wheel | Navigate lists and tables |
| Click dialog field | Focus field (text fields position cursor) |
| Click dialog checkbox | Toggle checkbox |
| Click dialog list item | Select item |
| Click dialog button | Activate button (OK, Cancel, etc.) |
| Click dialog `[x]` | Close dialog |
| Scroll wheel in dialog | Scroll list fields |

## CLI Reference

The CLI provides full access to all features for scripting and automation.

### Global Options

```
-f, --file <path>    Specify database file
-h, --help           Show help message
-v, --version        Show version information
```

### Database

```bash
# Create a new database file
tmoney db create ~/Documents/TMoney/finances.tdb

# Create a manual backup of the database
tmoney -f ~/Documents/TMoney/finances.tdb db backup

# Restore the database from a backup file (a safety backup of the
# current state is created automatically before the swap)
tmoney -f ~/Documents/TMoney/finances.tdb db restore /path/to/backup.tdb

# List backups for the database file
tmoney -f ~/Documents/TMoney/finances.tdb db list-backups
```

### Accounts

```bash
# List all accounts
tmoney account list
tmoney account list --include-closed

# Show account details
tmoney account show Checking

# Show all balances with net worth
tmoney account balance

# Create a new account
tmoney account add --name "Checking" --type checking
tmoney account add --name "Visa" --type credit_card --credit-limit 5000
tmoney account add --name "Mortgage" --type loan --interest-rate 6.5
tmoney account add --name "Savings" --type savings \
  --opening-balance 10000 --opening-date 2024-01-15 \
  --institution "First Bank" --currency USD
```

Account types: `checking`, `savings`, `credit_card`, `investment`, `cash`, `loan`, `asset`

### Transactions

```bash
# List transactions for an account
tmoney transaction list --account Checking
tmoney transaction list --account Checking --limit 20
tmoney transaction list --account Checking --from 2024-01-01 --to 2024-01-31

# Add a transaction (negative amounts for expenses)
tmoney transaction add --account Checking --amount -50.00 --payee "Coffee Shop"
tmoney transaction add --account Checking --amount -120.00 \
  --payee "Electric Co" --category "Bills:Utilities" \
  --date 2024-03-15 --memo "March electric bill"

# Add income
tmoney transaction add --account Checking --amount 3500.00 \
  --payee "Employer Inc" --category "Income:Salary"

# Void a transaction by ID (transfer counterparts are voided too)
tmoney transaction void <id>

# Create a transfer between accounts
tmoney transfer add --from Checking --to Savings --amount 500.00
tmoney transfer add --from Checking --to Savings --amount 500.00 \
  --date 2024-03-01 --memo "Monthly savings"

# Search transactions
tmoney transaction search "grocery"
tmoney transaction search "electric" --from 2024-01-01 --to 2024-12-31
tmoney transaction search "restaurant" --account Visa --min 20 --max 100
tmoney transaction search "transfer" --category "Transfer"
```

### Scheduled Transactions

```bash
# Add a scheduled transaction (fixed amount)
tmoney scheduled add --account Checking --frequency monthly \
  --amount -1500 --payee Landlord --memo "Monthly rent" --day 1

# Variable-amount schedule (estimate at post time)
tmoney scheduled add --account Checking --frequency monthly --payee "Electric Co"

# Auto-post a few days early
tmoney scheduled add --account Checking --frequency monthly --amount -150 \
  --payee Insurance --auto-post --lead-days 3

# List all scheduled transactions
tmoney scheduled list
tmoney scheduled list --account Checking

# List only due scheduled transactions
tmoney scheduled list --due

# Post a scheduled transaction (create real transaction from it)
tmoney scheduled post <id>
tmoney scheduled post <id> --amount 150.00    # Override amount
tmoney scheduled post <id> --date 2024-03-20  # Override date

# Skip a scheduled transaction (advance to next occurrence)
tmoney scheduled skip <id>
```

Frequencies: `daily`, `weekly`, `biweekly`, `monthly`, `quarterly`, `yearly`. `--day` accepts `1-31` or `-1` for the last day of month. Use `--occurrences <n>` or `--end-date <YYYY-MM-DD>` for fixed-duration schedules; omit both for indefinite. `--lead-days` accepts `0`, `3`, or `7` and requires `--auto-post`.

### Reconciliation

```bash
# Start a reconciliation session against a statement
tmoney -f personal.tdb reconcile start --account "Checking" \
  --statement-date 2024-01-31 --statement-balance 850.00

# Mark one or more transactions against the active session
tmoney -f personal.tdb reconcile mark <id> [<id> ...]

# Finish the active session (refuses to close with a non-zero diff
# unless --force is given)
tmoney -f personal.tdb reconcile finish --account "Checking"
tmoney -f personal.tdb reconcile finish --account "Checking" --force

# Show reconciliation status for an account
tmoney -f personal.tdb reconcile status --account "Checking"
```

`reconcile start` records the statement date and balance and reports
the count of unreconciled transactions on or before the statement
date. `reconcile mark` marks transactions against the active session
and reports the running difference between the cleared total and the
statement balance. `reconcile finish` marks every candidate
transaction reconciled and closes the session — it refuses to finish
if the cleared total still differs from the statement balance, unless
`--force` is supplied. `reconcile status` shows the last completed
reconciliation and any active session for the account.

### Securities

```bash
# Add a new security
tmoney -f personal.tdb security add --ticker AAPL --name "Apple Inc." --type stock
tmoney -f personal.tdb security add --ticker VTI --name "Vanguard Total Stock Market" \
  --type etf --asset-class large_cap_stock --exchange NYSE

# List securities
tmoney -f personal.tdb security list
tmoney -f personal.tdb security list --include-hidden
tmoney -f personal.tdb security list --type etf
tmoney -f personal.tdb security list --asset-class large_cap_stock

# Show details for one security
tmoney -f personal.tdb security show AAPL

# Edit fields on an existing security (only supplied flags take effect)
tmoney -f personal.tdb security edit AAPL --name "Apple Corporation"
tmoney -f personal.tdb security edit AAPL --ticker AAPL2          # rename
tmoney -f personal.tdb security edit VTI --asset-class total_market

# Hide a security from default listings (data is preserved)
tmoney -f personal.tdb security hide AAPL

# Restore a hidden security to default listings
tmoney -f personal.tdb security unhide AAPL

# Permanently delete a security (refuses if prices or transactions exist)
tmoney -f personal.tdb security delete AAPL
```

Security types: `stock`, `etf`, `mutual_fund`, `other`. `--ticker`,
`--name`, and `--type` are required for `security add`. `--currency`
defaults to `USD` and `--asset-class` defaults to `unclassified`.
`security list` excludes hidden securities by default; pass
`--include-hidden` to show them, and filter the list with `--type` or
`--asset-class`. `security show <ticker>` prints the full record for a
single security (errors if the ticker is unknown). `security edit
<ticker>` updates only the fields whose flag is supplied; pass
`--ticker` to rename the security to a new symbol. `security hide
<ticker>` marks the security as hidden so it no longer appears in
default listings (use `--include-hidden` on `security list` to see it,
or `security unhide <ticker>` to restore visibility). `security delete
<ticker>` permanently removes a security; it refuses to run if any
prices or transactions still reference it and suggests `security hide`
in that case.

### Reports

```bash
# Net worth report
tmoney --report net-worth
tmoney --report net-worth --as-of 2024-06-30
tmoney --report net-worth --include-closed

# Spending by category - monthly
tmoney --report spending --month 2024-03

# Spending by category - yearly
tmoney --report spending --year 2024

# Spending by category - custom date range
tmoney --report spending --from 2024-01-01 --to 2024-06-30
```

### Prices

```bash
# Manually record a price for a security on a specific date
tmoney -f personal.tdb price add --ticker AAPL --date 2024-01-15 --price 150.00

# List recorded prices for a security
tmoney -f personal.tdb price list AAPL
tmoney -f personal.tdb price list AAPL --from 2024-01-01 --to 2024-06-30

# Show the most recent price for a security
tmoney -f personal.tdb price current AAPL

# Bulk-import prices from a CSV file (Date,Ticker,Price columns)
tmoney -f personal.tdb price import prices.csv
tmoney -f personal.tdb price import prices.csv --overwrite
```

`price add` requires `--ticker`, `--date`, and `--price`. The price is
stored with `source = manual`. Re-adding a price for a date that
already has one returns an error.

`price list <ticker>` prints the price history for a security.
Optional `--from` and `--to` flags limit the date range
(YYYY-MM-DD, both inclusive).

`price current <ticker>` prints the most recent price on or before
today for a security, including the date and source.

`price import <file>` bulk-loads prices from a CSV file containing
`Date`, `Ticker`, and `Price` columns. Imported rows are stored with
`source = import`. Existing prices on matching dates are skipped
unless `--overwrite` is passed.

### Update Prices

```bash
# Refresh prices for all visible securities with a ticker (default: yahoo)
tmoney -f personal.tdb price update

# Refresh only specific tickers
tmoney -f personal.tdb price update AAPL MSFT

# Choose a different provider (must be registered)
tmoney -f personal.tdb price update --provider yahoo
```

Each run fetches the latest *closed-session* price from the provider and upserts it as `source = api`. Hidden securities, securities without a ticker, and securities whose currency does not match the provider's are skipped silently or with a note. If the date the provider returned is already on file, the row is left alone (so re-running the same day is a no-op).

The command prints a per-ticker table followed by a summary like `4 updated, 1 up-to-date, 0 skipped, 0 failed` and exits non-zero if any ticker failed.

### Investment

```bash
# Buy shares with a total amount (cash debited from the account)
tmoney -f personal.tdb investment buy --account Brokerage --ticker AAPL \
  --shares 10 --amount 1500

# Buy shares specifying the price per share
tmoney -f personal.tdb investment buy --account Brokerage --ticker AAPL \
  --shares 10 --price-per-share 150

# Optional commission, custom date, and memo
tmoney -f personal.tdb investment buy --account Brokerage --ticker AAPL \
  --shares 10 --amount 1510 --commission 10 --date 2024-06-15 --memo "AAPL dip"
```

`investment buy` requires `--account`, `--ticker`, and `--shares`. Supply
either `--amount` (total cost), `--price-per-share`, or both. Commission
defaults to `0`; date defaults to today. If lot tracking is enabled on
the account, a new lot is opened.

```bash
# Sell shares at a price per share (cash credited to the account)
tmoney -f personal.tdb investment sell --account Brokerage --ticker AAPL \
  --shares 5 --price-per-share 160

# Sell with total proceeds and a commission
tmoney -f personal.tdb investment sell --account Brokerage --ticker AAPL \
  --shares 5 --amount 800 --commission 10

# For lot-tracked accounts, allocate the sell against a specific open lot
tmoney -f personal.tdb investment sell --account Brokerage --ticker AAPL \
  --shares 5 --price-per-share 160 --lot <lot-id>
```

`investment sell` requires `--account`, `--ticker`, and `--shares`.
Supply either `--amount` (total proceeds), `--price-per-share`, or both.
Commission defaults to `0`; date defaults to today. For lot-tracked
accounts, pass `--lot <id>` to allocate the sell against a specific
open lot.

```bash
# Record a cash dividend (cash credited to the account, share count unchanged)
tmoney -f personal.tdb investment dividend --account Brokerage --ticker AAPL \
  --amount 125.50

# Optional date and memo
tmoney -f personal.tdb investment dividend --account Brokerage --ticker AAPL \
  --amount 125.50 --date 2024-04-15 --memo "Q1 dividend"
```

`investment dividend` requires `--account`, `--ticker`, and `--amount`. The
share count is unchanged; cash is credited to the account. Date defaults
to today.

### Import Transactions

```bash
# Dry-run preview (default — shows what would happen without changing anything)
tmoney -f personal.tdb --import statements.qif --account "Checking"

# Execute the import
tmoney -f personal.tdb --import statements.qif --account "Checking" --confirm

# Force a format if auto-detection from the extension fails
tmoney -f personal.tdb --import data.txt --account "Checking" --format qif

# Skip duplicates (matched rows are not imported)
tmoney -f personal.tdb --import file.qif --account "Checking" --confirm --skip-duplicates

# Update duplicates (matched rows update existing transactions: cleared status, FITID, etc.)
tmoney -f personal.tdb --import file.ofx --account "Checking" --confirm --update-duplicates

# Multi-account CSV (e.g. Quicken Mac's "Register Transactions to CSV"):
# the import refuses to run without --source-account when the file
# contains rows for more than one account.
tmoney -f personal.tdb --import register.csv --account "BoA Checking" \
  --source-account "Checking" --confirm
```

Supported formats: CSV, QIF (Quicken), and OFX/QFX (bank downloads). The same flow is also available in the TUI via File → Import Transactions… — when the file contains multiple source accounts, the TUI inserts a picker step between the options and confirm dialogs.

### Export Transactions

```bash
# Export full database as CSV (or QIF with --format qif)
tmoney -f personal.tdb --export finances.csv

# Export a single account, date range
tmoney -f personal.tdb --export checking_q1.csv --account "Checking" \
  --from 2024-01-01 --to 2024-03-31
```

### Link Transfers

```bash
# Dry-run preview (default)
tmoney -f personal.tdb transfer link

# Widen or narrow the date-tolerance window (default: 5 days)
tmoney -f personal.tdb transfer link --max-days 3

# Execute the linking
tmoney -f personal.tdb transfer link --confirm
```

Useful after importing each account's QIF/OFX separately: it scans for
pairs of unlinked transactions across accounts whose amounts cancel and
whose dates are within `--max-days`, then joins the matched pairs into
real transfers. Pairs with multiple possible counterparts are listed as
"ambiguous" and left alone — you can review and link those by hand. The
preview lists every candidate; `--confirm` only links the unambiguous
ones.

## File Format

TMoney uses `.tdb` files (DuckDB databases) stored in `~/Documents/TMoney/` by default. Each file is self-contained and versioned with automatic schema migrations.

## Configuration

TMoney stores its configuration at `~/.config/tmoney/config.json`, following the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/latest/). If `$XDG_CONFIG_HOME` is set, it uses `$XDG_CONFIG_HOME/tmoney/` instead.

The config file tracks:
- **Last opened file** — automatically reopened when you launch `tmoney` without specifying a file
- **Recent files** — the 5 most recently opened database files (available via File > Open Recent in the TUI)

This means you can simply run `tmoney` after your first session and it will reopen the last file you were working with. Specifying `-f <path>` or a positional argument always takes priority over the saved default.

## Themes

TMoney's TUI is skinnable via a theme system. Three themes ship built in, and users can author their own theme files or generate one from a pywal palette.

### Built-in themes

| ID | Description |
|----|-------------|
| `default` | The original TMoney palette. Transparent backgrounds (terminal shows through), single borders. The fallback used when a user theme is missing slots. |
| `turbo-vision` | Faithful to the Borland Turbo Vision aesthetic — yellow titles on blue, red shortcut letters, double borders, black-on-cyan selected rows. |
| `light` | Designed for light-background terminals; dark text on near-white panels, single borders. |

### Switching themes

In the TUI, open **View → Theme** (or `Alt+V`) and select a theme. The change applies immediately — no restart — and the active theme is persisted to `~/.config/tmoney/config.json` so it's restored on the next launch. The active theme is marked with a `✓` in the submenu.

From the CLI:

```bash
tmoney theme list                # list built-ins and user themes; * marks active
tmoney theme generate-from-wal   # create ~/.config/tmoney/themes/wal.toml from pywal
```

### Authoring a custom theme

Drop a TOML file into `~/.config/tmoney/themes/` (or `$XDG_CONFIG_HOME/tmoney/themes/` if that env var is set). The filename stem becomes the theme ID — e.g., `mine.toml` → ID `mine`. User themes with the same ID as a built-in override it.

```toml
# ~/.config/tmoney/themes/mine.toml
name = "Mine"
description = "My personal palette"
border_style = "rounded"   # one of: single, double, rounded, thick

window.title.fg     = "#ffcc66"
table.selected.bg   = "#264f78"
text.negative       = "#ff6b6b"
# ... any slots you don't set fall back to the default theme
```

Theme files are hand-editable. Missing slots inherit from `default`, so a custom theme can be as small as a single override. The full slot list is documented in [`specs/theming.md`](specs/theming.md).

### Pywal integration

Users running [pywal](https://github.com/dylanaraps/pywal) (Omarchy, etc.) can generate a theme from their current system palette:

```bash
tmoney theme generate-from-wal              # writes ~/.config/tmoney/themes/wal.toml
tmoney theme generate-from-wal --output -   # write to stdout
```

To regenerate automatically when pywal updates, add a one-liner to `~/.config/wal/postrun.sh` (make it executable):

```bash
#!/bin/sh
tmoney theme generate-from-wal
```

The TUI does not auto-pick-up the regenerated file — re-select `wal` in **View → Theme** to apply the new colors.

### Misconfigured themes

If a theme file has malformed values (e.g., `text.negative = "not-a-color"`), the offending slot falls back to its `default` value rather than rejecting the whole theme. A status-bar toast surfaces the issue count, and details are appended to `~/.config/tmoney/log.txt` with the slot name and reason. If the file is unparseable TOML entirely, the app falls back to `default` and shows the error in the toast.

## CLI

The CLI is being migrated from flat `--flag` verbs to a Cobra-based noun-verb structure (`tmoney theme list`). The migration is opportunistic — `tmoney version`, `tmoney theme list`, `tmoney theme generate-from-wal`, the entire `tmoney db` subtree (`create`, `backup`, `restore`, `list-backups`), `tmoney account add`/`list`/`show`/`balance`, `tmoney transaction add`/`list`/`void`/`search`, and `tmoney transfer add`/`link`, `tmoney scheduled add`/`list`/`post`/`skip`, `tmoney reconcile start`/`mark`/`finish`/`status`, and `tmoney security add`/`list`/`show`/`edit`/`hide`/`unhide`/`delete`, and `tmoney price add`/`list`/`current`/`import`/`update`, and `tmoney investment buy`/`sell`/`dividend` are Cobra-native today; the legacy `--flag` forms documented under [CLI Reference](#cli-reference) continue to work for everything else.

## Tech Stack

- **Go**: Performance, single binary, cross-platform
- **Bubbletea**: Modern TUI framework
- **DuckDB**: Embedded analytics database
- **alpacadecimal**: Precise decimal arithmetic

## Documentation

- **[Architecture](docs/ARCHITECTURE.md)** — Layered architecture, data flow, design decisions, and package overview
- **[Feature Specifications](specs/)** — Detailed specs for all features (accounts, transactions, categories, reports, security master, and more)

## Design Inspiration

- **lazygit**: Modern TUI aesthetics, keyboard-first
- **Quicken/Microsoft Money**: Feature set and workflows
- **Turbo Vision**: Text-based UI paradigm
- **ledger-cli**: Plain-text accounting philosophy
