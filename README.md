# Terminal Money

A personal finance management application that runs in the terminal. Terminal Money is designed for users who prefer keyboard-driven interfaces and want full control over their financial data.

## Core Philosophy

1. **Local-first**: Your data stays on your computer in a single file
2. **Keyboard-driven**: No mouse required, fast navigation
3. **Dual interface**: TUI for daily use, CLI for scripting and automation
4. **Simple but complete**: Essential features without bloat

## Installation

### Build from Source

Requires Go 1.27 or later, plus a C compiler — the DuckDB bindings use cgo.
The Go 1.27 toolchain does not run on macOS 12 or earlier, so building on a
Mac needs macOS 13 Ventura or later.

```bash
git clone https://github.com/haskovec/tmoney.git
cd tmoney
make install
```

The resulting `tmoney` binary is self-contained with no runtime dependencies.

To build without installing, `make build` leaves a `tmoney` binary in the
checkout.

### Where `make install` puts the binary

| Platform | Destination |
| --- | --- |
| macOS / Linux | `/usr/local/bin/tmoney` |
| Windows | `%GOBIN%\tmoney.exe`, or `%USERPROFILE%\go\bin\tmoney.exe` when `GOBIN` is unset |

On macOS and Linux the destination is controlled by the usual GNU variables:

```bash
make install PREFIX=$HOME/.local   # user-local install, no sudo required
make install BINDIR=/opt/bin       # choose the bin directory outright
make install DESTDIR=/tmp/stage    # staged install, for building packages
sudo make install                  # system-wide, if /usr/local/bin isn't yours
```

`make uninstall` removes the binary again, and honors the same variables — pass
whichever ones you installed with.

Windows has no `/usr/local`, so `make install` there runs `go install`, which
places the binary in the Go bin directory. That avoids a `Program Files`
location needing administrator elevation, and the official Go installer already
adds `%USERPROFILE%\go\bin` to your user `PATH`. If the `tmoney` command isn't
found afterwards, add that directory to your `PATH` and open a new terminal.

### Development toolchain

[`mise`](https://mise.jdx.dev) pins the Go and golangci-lint versions this
project builds and lints with, so a fresh checkout needs no version hunting:

```bash
mise trust     # mise refuses to read an untrusted config, so this is first
mise install   # fetch the pinned Go and golangci-lint
```

`mise install` only downloads the tools; it does not put them on PATH. Activate
mise in your shell profile with `eval "$(mise activate zsh)"`, or the bash/fish
form. Without that, prefix each command with `mise exec --`, for example
`mise exec -- make test`. Run `mise doctor` if a tool still does not appear.

`mise.toml` is the single place those versions live — keep the `go` entry in
step with the `go` directive in `go.mod` when either moves.

mise does not supply the C compiler cgo needs. Install that from the platform:
`xcode-select --install` on macOS, `build-essential` on Debian/Ubuntu, or the
MSYS2 toolchain on Windows.

mise is a convenience, not a dependency. Nothing in the build reads
`mise.toml`, so installing Go and [golangci-lint](https://golangci-lint.run)
by hand works just as well — check `mise.toml` for the versions to match.

## Quick Start

```bash
# Create a new database file
tmoney db create ~/Documents/TMoney/personal.tdb

# Launch the TUI
tmoney ~/Documents/TMoney/personal.tdb

# Or use the CLI to add data
tmoney -f personal.tdb account add --name "My Checking" --type checking --opening-balance 1000
tmoney -f personal.tdb transaction add --account "My Checking" --amount -45.50 --payee "Grocery Store" --category "Food:Groceries"
tmoney -f personal.tdb account balance
```

## Features

### Accounts
- Multiple account types: checking, savings, credit card, investment, HSA, cash, loan, asset
- Per-account currency setting (USD, EUR, GBP)
- Credit limit for credit card accounts
- Interest rate (APR) for checking, savings, credit card, investment, HSA, and loan accounts
- Investment and HSA accounts are lot-tracked by default (override with
  `account add --track-lots=false`, or a "Track lots" checkbox on the New
  Account dialog); existing accounts gain lot tracking via
  `investment enable-lots`, which backfills lots from history
- HSA (Health Savings Account) behaves like an investment account — supports
  cash flows plus securities buy/sell/dividend operations with lot tracking
  on by default, since HSAs typically allow invested funds above a cash
  threshold
- Dynamic account dialog that shows only relevant fields for the selected account type
- Open/close account lifecycle: closing records a (back-datable) close date and
  **freezes** the account — no new transactions, edits, or transfers, and it
  drops out of every account picker — while staying viewable. Closed accounts
  collect in a dimmed section at the bottom of the sidebar; **Reopen** from the
  Accounts menu (or `account reopen`) brings one back.

### Transactions
- Standard transactions with payee, category, and memo
- Split transactions across multiple categories
- Linked transfers between accounts, including between two investment
  accounts (e.g., IRA-to-IRA cash rollovers). A single Transfer dialog
  in the TUI handles every combination — bank↔bank, bank↔investment,
  and investment↔investment — with explicit From / To pickers. A transfer
  can carry an **optional category** — a label for *why* money moved (e.g. a
  credit-card payment tagged `Bills:Credit Card`); it is never required and
  never changes balance math. Investment↔investment transfers cannot carry
  one (neither leg has a place to store it)
- Cleared/pending status tracking
- **Running-balance column** in the register (after Amount), showing the
  account balance after each transaction — Quicken-style — so the newest
  row always matches the account balance in the title bar. Void rows carry
  the balance forward unchanged. Investment-account registers show a running
  **cash** balance after Total (cash-affecting rows only; share-only rows like
  Reinvest Dividend and Transfer Shares carry forward). The column appears when
  the terminal is wide enough and hides automatically when space is tight.
- Full-text search with date, amount, and category filters
- Sticky last-used date across every new-transaction dialog (within a
  session) — batch entry in the regular register and any investment
  register (Buy, Sell, Dividend, Reinvest Dividend, cash ops, transfer
  cash, transfer shares) seeds each open with the date of the last
  saved transaction
- Inline category creation from the Category field — pick
  `[+ Add new category…]` to create a new category (with optional
  new parent) without leaving the transaction, split, scheduled,
  paycheck, or loan flow — and on the Transfer and Edit Transfer
  dialogs, the Scheduled Transfer dialog, the single-line transfer
  post-time preview, and the loan wizard's Principal category field

### Categories
- Two-level hierarchy (parent/subcategory, e.g. "Food:Groceries")
- Income vs expense classification
- Default categories provided on new file creation
- **Value Adjustment** — a system category (like `Transfer`) for asset
  revaluations. It is created automatically on file open, excluded from
  spending reports, and protected from rename/delete/merge. Unlike
  `Transfer`, it is offered in the category picker — but only for
  **asset**-type accounts. Use it to record changes in what an asset is
  worth:
  - **Home value update**: add a transaction in the asset account's
    register for the delta (e.g. `+15,000` when the estimate rises),
    category `Value Adjustment`.
  - **Straight-line depreciation** (e.g. a car): a plain monthly
    scheduled transaction on the asset account for a fixed negative
    amount, category `Value Adjustment`.

  Because it is a system category, these revaluations never distort your
  spending-by-category report.

### Payees
- Auto-creation on first use
- Default category assignment
- Alias/pattern matching for imports

### Scheduled Transactions
- Multiple frequencies: daily, weekly, fortnightly, semimonthly, monthly, quarterly, yearly
- Fixed or indefinite duration
- Variable amount estimation (single-line schedules only)
- **Scheduled transfers** (`t` on the Scheduled view): a recurring transfer
  between two regular accounts — e.g. a monthly credit-card payment from
  Checking, or a savings sweep. Stored as a single-line transfer (From → To);
  posting creates a clean linked transfer pair, identical to an ad-hoc
  transfer. Put an estimate on the schedule and edit the real amount in the
  post-time preview. The recurring transfer can carry an **optional
  category**, which flows onto both legs of every posted pair.
  Investment-account destinations (401k/HSA) use the paycheck/multi-line
  flow instead.
- Post or skip workflow
- **Multi-line templates** for compound events (paychecks, etc.):
  a single scheduled transaction can carry multiple categorized and/or
  transfer lines whose signed amounts sum to the schedule's parent
  amount. Pressing Enter on a due multi-line schedule opens a
  **post-time preview dialog** pre-filled with the template values so
  you can adjust a single occurrence (FICA-penny shifts, holiday
  payday shifts) without changing the underlying template. Date and
  line edits are one-off — the schedule's next occurrence reopens with
  the original template.
- **Paycheck wizard** (Transactions → New Paycheck Schedule…): a
  guided form that creates a multi-line scheduled paycheck, organized
  into five pay-stub-aligned sections — Earnings (multi-line, for
  base salary plus shift differential, imputed income, etc.),
  Pre-tax Deductions, Taxes, Post-tax Deductions, and Net Pay
  Destinations. The wizard is pure UI sugar — the saved record is a
  standard multi-line scheduled transaction. A paycheck-shaped
  schedule can be reopened in the wizard via the **Edit as
  paycheck →** affordance in the Edit Series dialog. See
  [`specs/multiline-splits-and-paycheck.md`](specs/multiline-splits-and-paycheck.md)
  for the full feature spec.

### Loans
- **Loan wizard** (Accounts → New Loan…, or `tmoney loan add`): a guided,
  one-step setup for an amortized loan — it creates the **loan account**
  (a liability, stored as a negative balance), an optional linked **asset
  account** (the house or car), and a **monthly payment schedule**, all in
  one atomic operation. Works both at origination (give the original
  principal + term and let the payment compute) and mid-life (give what you
  owe today and the P&I payment).
- **Recompute-at-post**: a loan payment's interest/principal split is
  **recomputed from the live balance every time it posts** — on manual post,
  the post-time preview, and auto-post — so extra principal payments and APR
  edits automatically reshape every subsequent payment without regenerating
  anything. The split is one categorized **interest** line (default
  `Loan:Interest`), one **principal** transfer line into the loan account
  (moving its negative balance toward zero, labeled `Loan:Principal` by
  default — overridable, or suppressible with an empty value), and zero or
  more fixed **escrow** lines (property tax, insurance, PMI). The principal
  label is preserved on every recompute-at-post. 0% loans book the whole
  payment as principal (and still label the principal line).
- **Amortization view** (`a` on a loan account's register): a live,
  recomputed projection of the remaining payments — balance owed, APR, P&I
  payment, payments left, payoff date, and total interest remaining, over a
  full `# · Date · Payment · Interest · Principal · Escrow · Balance` table.
- **Payoff handling**: when the balance reaches zero the schedule is marked
  completed (and drops out of the due list); a toast suggests closing the
  account. Edit the P&I payment or adopt a hand-built schedule via **Edit as
  loan →** in the Edit Series dialog.
- **CLI**: `tmoney loan add`, `tmoney loan list`, and `tmoney loan show`
  provide full parity (see the CLI Reference below). Full feature spec:
  [`specs/loan-wizard.md`](specs/loan-wizard.md).

### Reports
- Net worth calculation over signed balances (assets + liabilities).
  Liability accounts (credit card, loan) store what you owe as a
  **negative** balance — a $250,000 mortgage sits at −250,000 — and the
  dashboard/report LIABILITIES sections display the raw **signed**
  balance: a debt shows negative (in red), while an overpaid or
  paid-ahead account (positive balance) shows positive — a credit — so
  it no longer reads as a debt. **Upgrade note:** if you previously entered a loan's
  opening balance as a positive number, flip its sign once (edit the
  account's opening balance); credit cards built from purchase
  transactions are already negative and need no change. Net worth for
  files with credit-card debt is now *corrected* — the old
  assets-minus-liabilities math overstated it.
- Spending by category with monthly/yearly aggregation and visual bars. By
  default the spending report **excludes transfers** (an explicit guard).
  Fold categorized transfers in on demand with `report spending
  --include-transfers` on the CLI, or `t` on the TUI Reports view — only the
  outflow leg of a pair counts (no double-count), only expense-typed
  categories appear, and the TUI toggle is session-only

### Prices
- Manual entry, CSV import, and history per security
- Bulk refresh from an online provider (Yahoo Finance by default) stores the latest closed-session price, so reruns on the same day are idempotent; historical closes can also be fetched for a specific date via `price lookup` or `price add --fetch`

### Import / Export
- Import transactions from CSV, QIF, or OFX/QFX files (Quicken / bank
  downloads). Available from File → Import Transactions… in the TUI or
  via `tmoney import` on the CLI. Quicken Mac users export via File →
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
  `tmoney transfer link`): when each side of a transfer is imported as
  a separate transaction (typical when importing one account at a time
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
| `Alt+V` | View menu (Theme switcher, Show closed positions toggle) |
| `Alt+A` | Accounts menu (also has New Loan…) |
| `Alt+T` | Transactions menu (also has Link Transfers… and New Paycheck Schedule…) |
| `Alt+S` | Securities menu (also has Stock Split…, Merger…, Spin-Off…, Corporate Action History…) |
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
| `Left/Right` (`h`/`l`) | Collapse / expand the selected investment account's holdings |

Investment accounts with holdings start collapsed on the dashboard; use
`Left`/`Right` (or `h`/`l`) on the selected account — or click the account's
`▸`/`▾` header with the mouse — to expand or collapse its holdings. Expanding
only the accounts you care about keeps the dashboard short so the totals and
scheduled sections stay in view. Your choices persist for the session.

#### Register (Account Transactions)

| Key | Action |
|-----|--------|
| `n` | New transaction |
| `Enter` | Edit selected transaction |
| `d` | Delete transaction |
| `c` | Toggle cleared status |
| `t` | New transfer |
| `r` | Reconcile account (start a new session) |
| `a` | Amortization schedule (loan accounts only) |
| `/` | Filter by security (investment accounts only) |
| `Tab` | Switch between sidebar and table |

On a **loan** account, `a` drills into a read-only **amortization view**: a
header of live loan stats (balance owed, APR, P&I payment, payments left,
payoff date, total interest remaining) over a full remaining-payment table
(`# · Date · Payment · Interest · Principal · Escrow · Balance`). Every figure
is recomputed from the loan's current balance, the account's APR, and the
loan-shaped schedule's derived P&I payment — nothing is stored — so extra
principal payments and rate edits show up immediately. Very long projections
(a tiny principal that never pays off within 100 years) report payoff date and
interest remaining as `100y+`. If no loan-shaped schedule targets the account,
the view shows the balance and APR it can compute plus a hint to run the loan
wizard or adopt an existing schedule. `Esc` returns to the register.

In an investment account, `n` opens a transaction-type selector (Buy, Sell,
Dividend, …) that also includes **Spin-Off…**. Choosing it launches the
spin-off dialog with the **parent** pre-filled to the security on the selected
register row — a convenience door onto the same engine as the Securities-view
`o` shortcut, which remains available. The dialog takes the **resulting share
count** from your statement (the engine's share ratio is derived from your
parent holding) and offers a **Lookup** button that fetches the spin-off
security's price for the date.

In an investment account, `/` opens a **security filter** for drilling into one
holding — handy for auditing a position or hunting down a data-entry error.
Type a ticker or name substring and the register narrows live (matching on both
ticker and full name, so tickerless holdings like collective trusts work too);
the filter line shows the matched security's ticker and full name, or a
`N securities` count while the query is still ambiguous. Press `Enter` when the
query resolves to a single security to **lock** the filter to it, `Esc` to clear
and return to the full register. While filtered the running-balance column and
the total-return header are hidden, and pressing `n` pre-selects the locked
security in the new-transaction dialog. The filter clears when you leave the
register.

#### Scheduled Transactions

| Key | Action |
|-----|--------|
| `Enter` | Open the post-time preview dialog (edit one occurrence, then save) |
| `s` | Skip occurrence |
| `e` | Edit series — modify the template (affects all future occurrences) |
| `n` | New scheduled transaction |
| `t` | New scheduled transfer (recurring transfer between two accounts) |
| `d` | Delete scheduled transaction |

#### Reports

| Key | Action |
|-----|--------|
| `Left/Right` | Change period |
| `n` | Net worth report |
| `s` | Spending report |
| `t` | Toggle including categorized transfers (spending report; session-only) |
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
| `s` | Stock split on selected security |
| `m` | Merger with selected security as source |
| `o` | Spin-off from selected security |
| `a` | Open Corporate Actions view, pre-filtered to the selected security |

#### Corporate Action History

Reached via the Securities-view `a` shortcut or **Securities → Corporate Action History…** (`Alt+S`).

| Key | Action |
|-----|--------|
| `Enter` | View details for the selected corporate action |
| `d` | Delete / reverse the selected corporate action |
| `/` | Filter by ticker or type |

Splits, reverse splits, and **spin-offs** are reversible: `d` confirms, then
unwinds the action — for a spin-off, it removes the spun-off child lots,
positions, and generated transactions, restores the parent lots' cost basis,
and deletes the seeded child price. If the spun-off shares have already been
sold (or the parent traded on/after the action date), the reversal **refuses**
and names the blocking transaction; nothing is cascade-deleted. Mergers are not
yet reversible.

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

The New/Edit Price dialog has a **Lookup** button next to Save/Cancel: it
fetches the provider's close on or before the dialog's Date from the default
provider and fills the Price field (and snaps the Date to the resolved trading
day). On a failed fetch the value is left for manual entry.

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
| Click a dashboard investment account's `▸`/`▾` header | Collapse/expand its holdings |
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

A few interactive features are deliberately TUI-only: undo/redo (CLI operations are final), the price history chart (`price list` shows the same data), the paycheck wizard, and editing individual split lines (creation via `transaction add --split` is supported).

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

# Rebuild the database indexes (repairs a desynced index)
tmoney -f ~/Documents/TMoney/finances.tdb db reindex
```

`db reindex` drops and recreates every secondary index in the file, rebuilding
each from the table data. Use it when an edit, a void, or posting a scheduled
transaction fails with **`FATAL Error: Invalid Input Error: Failed to delete all
rows from index. Only deleted 0 out of 1 rows`** — a DuckDB storage bug that can
leave an index out of sync with its table on disk. Any UPDATE that rewrites the
affected row (DuckDB turns an UPDATE that touches an indexed or FK-backed column
into an internal DELETE+INSERT) then aborts. Reindexing repairs it; it changes no
financial data. Run `tmoney db backup` first.

**Close the TUI before you run it** — the repair opens the file itself, and
DuckDB's file lock refuses it while tmoney holds the file.

Nothing is written when the error appears: the whole transaction rolls back, so
there is no half-posted entry and no risk of a duplicate when you retry. The
error does invalidate the open database handle, so quit and restart tmoney rather
than continuing in that session.

(Status-only changes — reconcile, clear/unclear — sidestep the bug with a narrow
in-place update, so reconciling never needs a reindex; header/amount/transfer
edits, voids, and the schedule advance that follows a post do.)

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

# Create a lot-tracked investment account (the default for investment/HSA)
tmoney account add --name "Brokerage" --type investment

# Opt a new investment/HSA account out of lot tracking
tmoney account add --name "401k" --type investment --track-lots=false

# Close an account (default today; back-date with --date). Requires a zero
# balance; the date must be within [max(opening, last txn), today].
tmoney account close "Old Savings"
tmoney account close "Old Savings" --date 2024-03-14

# Reopen a closed account (clears the close date)
tmoney account reopen "Old Savings"

# Edit an account (only the supplied flags change; empty string clears a
# nullable field). Opening balance/date are locked while the account is closed.
tmoney account edit --name "Checking" --new-name "Main Checking"
tmoney account edit --name "Checking" --institution "Acme Bank" --notes ""

# Delete an account (dry-run preview by default; --confirm to delete). Only
# works with no transactions and no scheduled references — otherwise close it.
tmoney account delete "Old Savings"
tmoney account delete "Old Savings" --confirm
```

Closing an account **freezes** it: no new transactions, edits, deletes, status
toggles, or transfers (a transfer is refused if either leg is closed), and it no
longer appears in any account picker or scheduled-transaction flow. The account
stays viewable (read-only register/portfolio; `account balance` and
`report net-worth --include-closed` still value it). Zero-balance and
close-date-range violations are hard errors; if scheduled transactions still
reference the account, `account close` prints a warning and proceeds (those
schedules are skipped on auto-post and refused on manual post). `account reopen`
clears the close date and unfreezes the account.

New `investment` and `hsa` accounts are **lot-tracked by default** — each
buy opens a lot and sells are allocated against open lots for exact
cost-basis and realized-gain tracking. Pass `--track-lots=false` to
`account add` to opt a new account out (it then uses the average-cost
path); `--track-lots` (or `--track-lots=true`) is the explicit way to
force it on. The flag is ignored for non-investment account types. To
enable lot tracking on an **existing** investment/HSA account — which
backfills lots from its transaction history — use
[`investment enable-lots`](#investment) rather than `account edit`; the
edit command does not flip this flag.

Account types: `checking`, `savings`, `credit_card`, `investment`, `hsa`, `cash`, `loan`, `asset`

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

# Add a split transaction: repeat --split "Category=amount[:memo]".
# --category is refused with --split; --amount is optional (derived from
# the line sum when omitted, or checked against it when given). A line
# named Transfer:<Account>=amount moves money to another account. Split
# lines are edited in the TUI; list/search mark the parent as [N splits].
tmoney transaction add --account Checking --payee "Costco" \
  --split "Food:Groceries=-80.00:weekly shop" \
  --split "Household=-20.00:paper towels"
tmoney transaction add --account Checking --amount -100.00 \
  --split "Food=-60.00" --split "Transfer:Savings=-40.00"

# Edit a transaction by UUID (find IDs with `transaction list --show-ids`).
# Only supplied flags take effect; "" clears --payee/--category/--memo.
# --status cleared|uncleared is the scriptable register `c` key.
# Transfer legs route to `transfer edit`; splits are edited in the TUI.
tmoney transaction edit --txn-id <id> --amount -45.50 --status cleared
tmoney transaction edit --txn-id <id> --category "Food:Groceries" --memo ""

# Permanently delete a transaction by ID (splits/transfer legs/reconciled
# rows are refused — prefer `void` to keep an auditable record)
tmoney transaction delete <id>

# Void a transaction by ID (transfer counterparts are voided too)
tmoney transaction void <id>

# Create a transfer between accounts. Dispatches internally by the
# (from, to) account types: bank↔bank, bank↔investment, and
# investment↔investment (e.g. IRA-to-IRA cash rollovers) all work.
tmoney transfer add --from Checking --to Savings --amount 500.00
tmoney transfer add --from Checking --to Savings --amount 500.00 \
  --date 2024-03-01 --memo "Monthly savings"
tmoney transfer add --from Checking --to Brokerage --amount 1000.00
tmoney transfer add --from "Old IRA" --to "Rollover IRA" --amount 5000.00

# Optionally label a transfer with an existing (non-system) category.
# Unlike `loan add`, this does NOT create the category.
tmoney transfer add --from Checking --to Visa --amount 500.00 \
  --category "Bills:Credit Card"

# Edit or delete a transfer by either leg's UUID (find IDs with
# `transaction list --show-ids`). Both work for every account-type
# combination. Only supplied flags take effect on edit.
tmoney transfer edit --txn-id <id> --amount 600.00 --status cleared
# Set, change, or clear the category (an explicit "" clears both legs)
tmoney transfer edit --txn-id <id> --category "Bills:Credit Card"
tmoney transfer edit --txn-id <id> --category ""
tmoney transfer delete --txn-id <id>

# Search transactions
tmoney transaction search "grocery"
tmoney transaction search "electric" --from 2024-01-01 --to 2024-12-31
tmoney transaction search "restaurant" --account Visa --min 20 --max 100
tmoney transaction search "transfer" --category "Transfer"
```

### Categories

```bash
# List the category tree (system categories are tagged [system])
tmoney category list
tmoney category list --type income
tmoney category list --show-ids

# Add a top-level category (defaults to expense) or a subcategory
# (a subcategory inherits its parent's type)
tmoney category add --name "Side Gig" --type income
tmoney category add --name Groceries --parent Food

# Rename by name or by id
tmoney category rename --name Groceries --to "Food & Groceries"
tmoney category rename --id <id> --to Utilities

# Delete by id, exact name, or Parent:Child path
tmoney category delete Food:Snacks

# Merge one category into another (reassigns everything, then deletes the source)
tmoney category merge --from Dining --to "Dining Out"
```

Categories form a two-level tree; a reference may be a UUID, an exact name, or a
`Parent:Child` path (an ambiguous bare name is rejected — disambiguate with
`Parent:Child` or `--id`). System categories (`Transfer`, `Value Adjustment`)
are shown in `category list` but cannot be renamed, deleted, or merged.
`category delete` refuses a category that has subcategories or is still
referenced by transactions, split lines, or scheduled transactions; when
references block it, use `category merge` to reassign them onto another category
first.

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

# Scheduled transfer between accounts (mutually exclusive with --payee;
# --category is allowed as an optional non-system label). Posting it
# creates a linked transfer pair. Enter --amount as a positive magnitude.
tmoney scheduled add --account Checking --frequency monthly --amount 250 \
  --transfer-to Savings --category "Savings Goal"

# List all scheduled transactions
tmoney scheduled list
tmoney scheduled list --account Checking

# List only due scheduled transactions
tmoney scheduled list --due

# Show full UUIDs (needed for `scheduled edit`/`scheduled delete`)
tmoney scheduled list --show-ids

# Edit a scheduled transaction by UUID. Only supplied flags take effect;
# "" clears --amount (variable)/--payee/--category/--memo. Multi-line
# (split/paycheck) templates are edited in the TUI.
tmoney scheduled edit --id <id> --amount -1600
tmoney scheduled edit --id <id> --frequency weekly --auto-post
tmoney scheduled edit --id <id> --account Savings --next-date 2024-06-15

# Delete a scheduled transaction template by ID (posted history is kept)
tmoney scheduled delete <id>

# Post a scheduled transaction (create real transaction from it)
tmoney scheduled post <id>
tmoney scheduled post <id> --amount 150.00    # Override amount
tmoney scheduled post <id> --date 2024-03-20  # Override date

# Skip a scheduled transaction (advance to next occurrence)
tmoney scheduled skip <id>
```

Frequencies: `daily`, `weekly`, `fortnightly`, `semimonthly`, `monthly`, `quarterly`, `yearly`. `--day` accepts `1-31` or `-1` for the last day of month. Use `--occurrences <n>` or `--end-date <YYYY-MM-DD>` for fixed-duration schedules; omit both for indefinite. `--lead-days` accepts `0`, `3`, or `7` and requires `--auto-post`.

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
date. For liability accounts (credit card, loan), enter the statement
balance **negated**: servicer statements print a positive amount owed,
but liability balances are stored negative — a card statement showing
$850.00 owed is entered as `--statement-balance -850.00`. `reconcile mark` marks transactions against the active session
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

# Add a security that has NO ticker (e.g. a collective trust held in a 401k).
# The ticker is optional; record an ISIN as a stable identifier if you have one.
tmoney -f personal.tdb security add --name "MFS Mid Cap Value CT" --type other \
  --isin US0378331005

# Reference a tickerless security on any security/price/investment command by
# --isin or exact --name instead of --ticker
tmoney -f personal.tdb security show --isin US0378331005
tmoney -f personal.tdb investment buy --account "Fidelity 401k" \
  --name "MFS Mid Cap Value CT" --shares 12.34 --price-per-share 25.10

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

Security types: `stock`, `etf`, `mutual_fund`, `other`. `--name` and
`--type` are required for `security add`; `--ticker` is **optional** —
a security may have a name but no ticker (e.g. a collective investment
trust held in a 401k that no price provider quotes). Such securities are
priced only from transaction data and are skipped by the online price
refresh. Record an optional `--isin` (ISO 6166, validated) as a stable,
unique identifier. `--currency` defaults to `USD` and `--asset-class`
defaults to `unclassified`. To reference a tickerless security on the
`security show/hide/unhide/delete`, `price add/list/current`, and
`investment` commands, pass `--isin <ISIN>` or `--name "<exact name>"`
in place of the ticker (exactly one selector; an ambiguous name is
rejected). `security edit` is identified by its positional ticker and
gains an `--isin` flag to set or change the ISIN; edit a tickerless
security from the TUI Securities view.
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
tmoney report net-worth
tmoney report net-worth --as-of 2024-06-30
tmoney report net-worth --include-closed

# Spending by category - monthly
tmoney report spending --month 2024-03

# Spending by category - yearly
tmoney report spending --year 2024

# Spending by category - custom date range
tmoney report spending --from 2024-01-01 --to 2024-06-30

# Fold categorized transfers into the spending report (works with
# --month, --year, or --from/--to)
tmoney report spending --month 2026-07 --include-transfers
```

### Prices

```bash
# Manually record a price for a security on a specific date
tmoney -f personal.tdb price add --ticker AAPL --date 2024-01-15 --price 150.00

# Delete the price for a security on a specific date
tmoney -f personal.tdb price delete --ticker AAPL --date 2024-01-15

# List recorded prices for a security
tmoney -f personal.tdb price list AAPL
tmoney -f personal.tdb price list AAPL --from 2024-01-01 --to 2024-06-30

# Show the most recent price for a security
tmoney -f personal.tdb price current AAPL

# Bulk-import prices from a CSV file (Date,Ticker,Price columns)
tmoney -f personal.tdb price import prices.csv
tmoney -f personal.tdb price import prices.csv --overwrite

# Look up the provider's closing price on or before a date (prints, does not store)
tmoney -f personal.tdb price lookup --ticker AAPL --date 2024-01-15
tmoney -f personal.tdb price lookup --ticker AAPL --date 2024-01-15 --provider yahoo

# Record a price by fetching it from the provider instead of passing --price
tmoney -f personal.tdb price add --ticker AAPL --date 2024-01-15 --fetch

# Remove legacy reinvest/fee-liquidation prices (dry-run; add --confirm to apply)
tmoney -f personal.tdb price cleanup
tmoney -f personal.tdb price cleanup --refetch --confirm
```

`price add` requires `--ticker` and `--date`, plus either `--price` or
`--fetch`. With `--price` the value is stored with `source = manual`.
Re-adding a price for a date that already has one returns an error.

`price delete --ticker X --date YYYY-MM-DD` removes the recorded price for
a security on that exact date — the CLI counterpart to deleting a price in
the TUI prices view (both go through the same service). Identify the
security with `--ticker`, `--isin`, or `--name`; it errors if no price
exists on that date.

`price cleanup` removes legacy transaction-sourced prices that are justified
only by a reinvested dividend or fee liquidation (no buy or sell on that
date) — the rows the buy/sell-only auto-price policy no longer creates. It
prints a dry-run plan by default; pass `--confirm` to apply. With `--refetch`,
tickered securities are replaced by the provider's close for that date instead
of deleted (tickerless securities and fetch failures are left in place). Limit
to one security with `--ticker` / `--isin` / `--name`. Run `tmoney db backup`
first.

`price add --fetch` fetches the closing price for `--date` from the
provider (Yahoo by default; override with `--provider`) instead of
requiring `--price`, and stores it with `source = api`. Exactly one of
`--price` or `--fetch` is given. The provider returns the close *on or
before* `--date`, so a weekend or holiday date resolves to the prior
trading day.

`price lookup --ticker X --date YYYY-MM-DD` fetches and prints the
provider's closing price on or before that date — it does **not** store
anything. It prints the resolved value and date so you can copy it into
`price add` or sanity-check it before recording. `--provider` defaults
to `yahoo`.

> **Sanity-check the ticker.** Some symbols are ambiguous on the
> provider. On Yahoo, `BTC` and `ETH` are the Grayscale Bitcoin/Ethereum
> Mini Trust ETFs (closing prices around $5–$6), while spot crypto trades
> under `BTC-USD` / `ETH-USD` (tens of thousands of dollars). Because
> `price lookup` prints the fetched value and date, verify both before
> using the number — a ~$60k print for a `BTC` ETF row means you fetched
> the wrong instrument.

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
# List recorded corporate actions (splits, mergers, spin-offs), newest first
tmoney -f personal.tdb investment actions

# Filter to one security (matched as subject or target) or one action type
tmoney -f personal.tdb investment actions --ticker AAPL
tmoney -f personal.tdb investment actions --type merger --show-ids
```

`investment actions` is a read-only view of the recorded corporate
actions — the CLI counterpart of the TUI's corporate-action history.
It takes no required flags; list everything, or narrow with `--ticker`
/ `--isin` / `--name` (matched as the action's subject *or* target) and
`--type` (`split`, `reverse_split`, `merger`, `spin_off`). Pass
`--show-ids` to print each action's UUID. Record actions with
`investment split`, `investment merge`, and `investment spin-off`.

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

Pass `--catch-up-splits` to repair a **back-dated** buy on a lot-tracked
account: after recording the buy at its real (pre-split) shares and
price, it applies any existing split / reverse-split corporate actions
dated on or after the buy to the *new lot only*, exactly as if the buy
had been entered before those splits. Use it when you enter a purchase
that predates a split you've already recorded — enter the raw historical
shares and let the flag scale the lot. It is a no-op on a non-lot
account (those replay splits automatically) or when no later split
exists.

> **Cash balances may go negative.** Buys, fees, withdrawals, and cash
> transfers never block on the running cash balance. This is by design:
> historical data entry from a brokerage statement frequently lists the
> day's sales after the day's buys, so requiring a positive running
> balance would force the user to reorder same-date transactions. The
> register simply shows a negative cash figure until the offsetting
> deposit/sell is entered.

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
# List the investment register (newest first)
tmoney -f personal.tdb investment list --account Brokerage

# Filter by security, type, or date range; print UUIDs for scripting
tmoney -f personal.tdb investment list --account Brokerage --ticker AAPL \
  --from 2024-01-01 --show-ids
```

`investment list` requires `--account` and prints the account's
investment transactions — date, type, security, shares, price, amount,
and status. Optional filters: `--ticker`, `--type`, `--from`/`--to`, and
`--limit`. Pass `--show-ids` to prefix each row with the transaction's
UUID for use with `investment edit`.

```bash
# Fix a fat-fingered share count, keeping the dollar amount (price re-derives)
tmoney -f personal.tdb investment edit --txn-id <uuid> --shares 1.587

# Move a buy to the right date and annotate it
tmoney -f personal.tdb investment edit --txn-id <uuid> --date 2024-01-16 \
  --memo "fixed date"

# Mark a transaction cleared (the register `c` key, scriptable)
tmoney -f personal.tdb investment edit --txn-id <uuid> --status cleared
```

`investment edit` requires `--txn-id` (find it with `investment list
--show-ids`) plus at least one editable flag: `--date`, `--shares`,
`--amount`, `--price-per-share`, `--commission`, `--memo`, or
`--status`. Only the supplied flags take effect; the account and
security are not editable.
It routes through the same update paths as the TUI's edit dialogs, so
positions and lots are reversed and re-applied — the re-created row gets
a new UUID. Cleared status carries over by default; `--status
cleared|pending` sets it explicitly via a narrow status-only update (a
status-only edit keeps the UUID), and `--status reconciled` is rejected.
Reconciled transactions and transfer legs are refused (use `tmoney
reconcile` / `transfer edit`). For lot-tracked accounts, pass
`--lot <id>` when editing a sell or fee-liquidation.

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

```bash
# Reinvest a dividend into additional shares (no cash leaves the account)
tmoney -f personal.tdb investment reinvest --account Brokerage --ticker AAPL \
  --shares 2 --price-per-share 150

# Reinvest specifying the total dividend amount
tmoney -f personal.tdb investment reinvest --account Brokerage --ticker AAPL \
  --shares 2 --amount 300

# Optional date and memo
tmoney -f personal.tdb investment reinvest --account Brokerage --ticker AAPL \
  --shares 2 --price-per-share 150 --date 2024-04-15 --memo "DRIP Q1"
```

`investment reinvest` requires `--account`, `--ticker`, and `--shares`.
Supply either `--amount` (total dividend), `--price-per-share`, or both.
Shares are added to the position and no cash leaves the account. Date
defaults to today.

```bash
# Record a fee in an investment account (cash debited; share count unchanged)
tmoney -f personal.tdb investment fee --account Brokerage --amount 25 \
  --memo "Annual fee"

# Optional date
tmoney -f personal.tdb investment fee --account Brokerage --amount 25 \
  --date 2024-04-15
```

`investment fee` requires `--account` and `--amount`. Cash is debited
from the account; share counts are unchanged. Date defaults to today.

```bash
# Pay a fee by liquidating shares (no cash effect; share count drops)
tmoney -f personal.tdb investment fee-liquidation --account "Fidelity 401k" \
  --ticker FXAIX --shares 0.123 --amount 5.00

# Specify the price per share instead of the fee total
tmoney -f personal.tdb investment fee-liquidation --account "Fidelity 401k" \
  --ticker FXAIX --shares 0.123 --price-per-share 40.65 \
  --memo "Q2 recordkeeping fee"
```

`investment fee-liquidation` records a fee paid by **selling shares of a
security** rather than debiting cash — the model some retirement plans
(e.g. a Fidelity 401k) use when there's no cash balance to charge. The
share count drops and the dollar amount is booked as a fee, with **no net
cash effect**. It requires `--account`, `--ticker`, and `--shares`, plus
either `--amount` (the fee total) or `--price-per-share` (the third value
is derived). Commission defaults to `0`; date defaults to today. On a
lot-tracked account the shares are drawn FIFO (oldest lot first) by
default; pass `--lot <id>` to allocate against a specific open lot
instead. In the TUI, choose **Fee via Liquidation** from the investment
register's `n` transaction-type selector.

```bash
# Deposit cash into an investment account (cash credited; share count unchanged)
tmoney -f personal.tdb investment deposit --account Brokerage --amount 5000 \
  --memo "Initial funding"

# Optional date
tmoney -f personal.tdb investment deposit --account Brokerage --amount 5000 \
  --date 2024-04-15
```

`investment deposit` requires `--account` and `--amount`. Cash is
credited to the account; share counts are unchanged. Date defaults to
today.

```bash
# Withdraw cash from an investment account (cash debited; share count unchanged)
tmoney -f personal.tdb investment withdraw --account Brokerage --amount 500 \
  --memo "Quarterly draw"

# Optional date
tmoney -f personal.tdb investment withdraw --account Brokerage --amount 500 \
  --date 2024-04-15
```

`investment withdraw` requires `--account` and `--amount`. Cash is
debited from the account; share counts are unchanged. Date defaults to
today.

```bash
# Transfer shares between two investment accounts (no cash changes hands)
tmoney -f personal.tdb investment transfer --from "Source IRA" \
  --to "Dest 401k" --ticker AAPL --shares 5

# Optional date, memo, and (for lot-tracked source accounts) --lot
tmoney -f personal.tdb investment transfer --from Brokerage --to RolloverIRA \
  --ticker VTI --shares 100 --date 2024-04-15 --memo "rollover" --lot <lot-id>
```

`investment transfer` requires `--from`, `--to`, `--ticker`, and
`--shares`. No cash changes hands; the share count moves from the
source account to the destination account. Date defaults to today.
For lot-tracked source accounts, pass `--lot <id>` to allocate against
a specific open lot.

> **Linked cash transfers involving an investment account** — whether
> bank↔investment or investment↔investment (e.g. IRA-to-IRA cash
> rollovers) — go through the unified `tmoney transfer add` command,
> which dispatches by the `(from.Type, to.Type)` combination. See the
> [Transactions](#transactions) section above for examples.

```bash
# Apply a forward stock split (4-for-1)
tmoney -f personal.tdb investment split --ticker AAPL --ratio 4:1

# Apply a reverse split (1-for-10) on a specific date
tmoney -f personal.tdb investment split --ticker AAPL --ratio 1:10 \
  --date 2024-06-15
```

`investment split` requires `--ticker` and `--ratio`. The ratio is
written as `N:D` — `4:1` means four new shares for every one held
(forward split); `1:10` means one new share for every ten held
(reverse split). All open positions and (for lot-tracked accounts)
lots are adjusted by the ratio. Date defaults to today.

```bash
# Repair a single lot that an existing split missed (e.g. a buy entered
# after the split was already applied)
tmoney -f personal.tdb investment split-lot \
  --lot 019e9fea-463f-75bc-9044-cd6f10bb53f0 --ratio 2:1
```

`investment split-lot` applies a split ratio to one lot, identified by
its lot ID (find IDs with `investment portfolio --account NAME
--show-lots`). It is a **repair** for a lot entered *after* a
security-wide `investment split` had already run — so the global split
never scaled it. It scales that lot's shares, original shares, and
per-share cost by the ratio and recomputes the account's position from
its lots, recording **no** corporate-action history and leaving every
other lot untouched. The lot must not have been sold against, and the
security must already have a recorded split (the per-lot scale is only
durable alongside one). For the common case — a back-dated buy you
forgot to enter — prefer `investment buy --catch-up-splits`, which adds
the raw buy and applies any existing later splits to the new lot in one
step.

```bash
# Apply a merger/acquisition: source becomes target at the given ratio
tmoney -f personal.tdb investment merge --source AAPL --target GOOG \
  --exchange-ratio 0.5

# Merger with cash consideration per source share
tmoney -f personal.tdb investment merge --source AAPL --target GOOG \
  --exchange-ratio 0.5 --cash-per-share 10.50

# Effective on a specific date
tmoney -f personal.tdb investment merge --source AAPL --target GOOG \
  --exchange-ratio 0.5 --date 2024-06-15
```

`investment merge` requires `--source`, `--target`, and
`--exchange-ratio`. Every share of the source security is exchanged
for `--exchange-ratio` shares of the target security. Optional
`--cash-per-share` records cash consideration paid per source share in
addition to the exchanged shares. All open positions in the source
security (and, for lot-tracked accounts, the underlying lots) are
converted to the target. Date defaults to today.

```bash
# Apply a corporate spin-off (parent distributes a new child security)
tmoney -f personal.tdb investment spin-off --parent AAPL --spinoff GOOG \
  --share-ratio 0.5 --parent-allocation 80 --spin-off-price 25

# Effective on a specific date
tmoney -f personal.tdb investment spin-off --parent AAPL --spinoff GOOG \
  --share-ratio 0.5 --parent-allocation 80 --spin-off-price 25 \
  --date 2024-06-15
```

`investment spin-off` requires `--parent`, `--spinoff`,
`--share-ratio`, `--parent-allocation`, and `--spin-off-price`. Every
share of the parent security distributes `--share-ratio` shares of the
child (spin-off) security; `--parent-allocation` is the percentage
(0–100) of the parent's cost basis retained by the parent, with the
remainder shifted to the child. `--spin-off-price` is the per-share
price of the child used to record the action. All open positions in
the parent (and, for lot-tracked accounts, the underlying lots) are
adjusted accordingly. Date defaults to today.

```bash
# Recompute positions / lot shares for one or all investment accounts
tmoney -f personal.tdb investment rebuild-positions
tmoney -f personal.tdb investment rebuild-positions --account Brokerage
```

`investment rebuild-positions` recomputes the stored
`investment_positions` rows (and, for lot-tracking accounts, each lot's
`shares` / `closed` fields) from the transaction ledger and lot
junction records. **You normally don't need to run this** — the app
automatically heals desynced positions when it opens a database and
again whenever you save a buy, sell, reinvest, fee-liquidation, or
share transfer. The command is provided as an explicit
recovery/diagnostic tool. Securities that participate in a corporate
action (split, merger, spin-off) are **skipped per-security**, since
those mutate positions and lots outside the ledger and a naive replay
would corrupt that security's cost basis; every other security is still
rebuilt — so a clean holding heals even on a database that contains
corporate-action history.

```bash
# Preview enabling lot tracking on an existing account (no changes made)
tmoney -f personal.tdb investment enable-lots --account "Wealthfront IRA"

# Execute the backfill once the previewed plan looks right
tmoney -f personal.tdb investment enable-lots --account "Wealthfront IRA" --confirm

# Enable lots on every investment/HSA account at once
tmoney -f personal.tdb investment enable-lots --all --confirm

# Choose which lots historical sells consume (default fifo)
tmoney -f personal.tdb investment enable-lots --account "Wealthfront IRA" \
  --method hifo --confirm
```

`investment enable-lots` turns on lot tracking for an **existing**
investment or HSA account and backfills its lots by replaying the full
transaction ledger in chronological order. Each `buy` and
`reinvest_dividend` opens a lot; an inbound `transfer_shares` opens a lot
from the cost basis carried on that row's `total_amount`; and each
`sell`, `fee_liquidation`, and outbound `transfer_shares` consumes open
lots. Cash-only types (`dividend`, `interest`, `fee`, `deposit`,
`withdrawal`, `transfer_cash`) have no share effect and are skipped.
`--method fifo|lifo|hifo` (default `fifo`) controls which open lots the
historical sells draw down. Supply either `--account NAME` or `--all`
(every investment/HSA account). By default the command prints the plan —
the lots to create per security, the sells matched against them, and any
insufficient-share shortfalls — and makes no changes; pass `--confirm` to
execute. **Run `tmoney db backup` first**, since the backfill writes lots
and lot junction records across the account's entire history.

`investment enable-lots` refuses to run when the target account is not an
`investment` or `hsa` account, when the account **already has lots**
(re-running would double-create them), or when the account holds a
security that already has a corporate action (split, merger, or
spin-off). A naive ledger replay cannot reconstruct lots across a
corporate action — those accounts need the future action-aware replay —
so the command stops and names the blocking security rather than
producing an incorrect cost basis.

```bash
# Preview disabling lot tracking on an account (no changes made)
tmoney -f personal.tdb investment disable-lots --account "Fidelity 401k"

# Execute: revert the account to average cost
tmoney -f personal.tdb investment disable-lots --account "Fidelity 401k" --confirm

# Disable lots on every lot-tracked investment/HSA account at once
tmoney -f personal.tdb investment disable-lots --all --confirm
```

`investment disable-lots` is the inverse of `enable-lots`: it turns lot
tracking **off** on a lot-tracked investment/HSA account and reverts it to
average cost. It deletes the account's lots and lot-junction rows, clears
`track_lots`, and recomputes `investment_positions` from the ledger — so
total return, unrealized/realized gain, dividends, and fees all keep
working on the average-cost path (you lose only the per-lot drill-down and
specific-lot realized gain, which carry no value in a tax-deferred
account). By default it prints the plan and makes no changes; pass
`--confirm` to execute, and supply either `--account NAME` or `--all`.
**Run `tmoney db backup` first.** It refuses when the account is not
lot-tracked. A held security with a **stock split** is fine — the split
replays cleanly into average cost — but a held **merger or spin-off** is
**refused**: those holdings exist only as lots on a lot-tracked account
and their average cost cannot be rebuilt from the ledger, so disabling
lots would destroy the holding.

```bash
# Show the portfolio for an investment account (today's valuation)
tmoney -f personal.tdb investment portfolio --account Brokerage

# Value the portfolio as of a specific date
tmoney -f personal.tdb investment portfolio --account Brokerage --as-of 2024-12-31

# Drill into per-lot detail for a lot-tracking account
tmoney -f personal.tdb investment portfolio --account Brokerage --show-lots

# Include fully-sold ("closed") positions in a separate section
tmoney -f personal.tdb investment portfolio --account Brokerage --include-closed
```

`investment portfolio` requires `--account`. The default valuation
date is today; pass `--as-of YYYY-MM-DD` to value holdings at a
different point in time. `--show-lots` expands each holding into its
underlying lots (purchase date, cost/share, cost basis, current value)
and is silently ignored on accounts without lot tracking enabled.

#### Total return

Each per-holding row and the account totals block report **total
return** alongside the legacy unrealized gain. Total return sums four
cash-flow components on top of unrealized gain:

```
total_return = unrealized_gain
             + realized_gain          (from sells and fee_liquidation)
             + dividends_received     (cash dividends AND reinvested dividends)
             + interest_received      (account-level cash sweep)
             − fees_paid              (commissions + fee transactions)
```

A **reinvested dividend** is income the fund paid you that you plowed back
into shares, so it counts toward `dividends_received` (and therefore total
return) just like a cash dividend. The shares it bought carry their own
cost basis, so their appreciation shows up in `unrealized_gain`; the
dividend principal is counted once, here — no double-count, and a later
sale of those shares realizes gain against that basis.

`total_return_pct` divides total return by **total cost deployed**
(`Σ buy.total_amount` — *your* contributions only). Reinvested dividends
are income, not deployed capital, so they are excluded from the
denominator; this makes total return equal `market value − your buys`.
Positions received only via `transfer_shares`, or built only from
reinvested dividends, have no deployed cost and render `—` for the
percent.

The per-holding table gains `UNREAL`, `DIV`, `REAL`, `FEES`, `TOTAL
RETURN`, and `RET %` columns. The account totals block prints
`Cost basis (open)`, `Unrealized gain`, `Realized gain`, `Dividends
received`, `Interest received`, `Fees paid`, `Total return`, and
`Total return %`.

Realized gain in non-lot accounts is replayed from the ledger. Splits
are reconstructed correctly — the replay applies each split's dated
ratio, so realized gain stays available across a split. Mergers and
spin-offs, which transform shares across securities and reallocate cost
basis, still cannot be replayed per-security: for a non-lot security
with a merger or spin-off the realized column renders `unavailable`
(the other components are still computed). Lot-tracked accounts compute
realized gain from lot junctions and are unaffected. Enable lot tracking
before a merger/spin-off to get exact realized numbers.

#### Closed positions

`--include-closed` adds a `Closed positions (fully sold, total-return
only)` block after the open holdings table. Each row shows `TICKER`,
`REALIZED`, `DIV`, `FEES`, `TOTAL RETURN`, and `RET %` — no
shares/price/market value, since the position is gone. Without the
flag, the portfolio command prints a one-line footer hint when closed
positions exist:

```
Hint: --include-closed adds 3 closed-position rows.
```

In the TUI, **View → Show closed positions** (`Alt+V`) toggles the
equivalent state. When on, the investment register and dashboard card
include fully-sold securities. The toggle persists across restarts via
`~/.config/tmoney/config.json`. The investment register header
displays a `TR` row with total return $ and %, and the dashboard
per-account card adds a `TR` line below `Total`.

### Loans

```bash
# One-shot setup: loan account + monthly payment schedule (+ optional asset).
# Mid-life loan: give what you owe today and the P&I payment.
tmoney -f personal.tdb loan add --name "Mortgage" \
  --current-balance 312450.22 --rate 6.5 --payment 2401.86 \
  --next-payment-date 2026-08-01 --from-account "Checking" \
  --escrow "Housing:Property Tax=650" --escrow "Housing:Home Insurance=120" \
  --principal-category "Housing:Principal" \
  --payee "Wells Fargo" --asset-name "123 Main St" --asset-value 450000

# New loan at origination: give the original terms and let the payment compute.
tmoney -f personal.tdb loan add --name "Car Loan" \
  --principal 32000 --rate 5.9 --term-months 60 --open-date 2026-07-01 \
  --next-payment-date 2026-08-01 --from-account "Checking"

# List loans: balance owed, rate, payment, next date, payoff, interest left.
tmoney -f personal.tdb loan list

# Details + amortization projection (--limit N, default 12; --all for everything).
tmoney -f personal.tdb loan show "Mortgage"
tmoney -f personal.tdb loan show "Mortgage" --limit 24
tmoney -f personal.tdb loan show "Mortgage" --all
```

`loan add` creates a **loan account** (a liability, stored as a negative
balance = −current balance), an optional linked **asset account**, and a
**monthly loan-shaped schedule** — all in one atomic operation, reusing the same
shared record assembly as the TUI loan wizard (Accounts → New Loan…). The
schedule's interest/principal split is **recomputed from the live balance every
time it posts** (through the ordinary `scheduled post` / auto-post path), so
extra principal payments and APR edits automatically reshape every subsequent
payment. Provide the monthly **P&I payment** with `--payment` (escrow-exclusive),
or omit it and pass `--principal` and `--term-months` to have it computed from
the amortization formula and printed for comparison against your statement. APR
is required (`--rate`, 0–100); a payment that fails to cover the first month's
interest is refused, and a 0% loan books the whole payment as principal with no
interest line.

`--interest-category` (default `Loan:Interest`), `--principal-category`
(default `Loan:Principal`; pass `""` to leave the principal line unlabeled),
and `--escrow "Category=Amount"` take `Parent` or `Parent:Subcategory` paths
and **create the category if it doesn't exist** (there is no `category add`
on the CLI). The funding account (`--from-account`) must be an active,
non-investment account.

`loan list` shows every loan's balance owed (the liability rendered as a
positive magnitude), APR, P&I payment, next date, payoff date, and interest
remaining — `—` when no loan-shaped schedule targets the account, `100y+` when
the loan never pays off within 100 years. `loan show <name>` adds the
remaining-payment amortization table (`# · Date · Payment · Interest · Principal
· Escrow · Balance`), capped at `--limit` rows (default 12) unless `--all` is
given.

### Import Transactions

```bash
# Dry-run preview (default — shows what would happen without changing anything)
tmoney -f personal.tdb import statements.qif --account "Checking"

# Execute the import
tmoney -f personal.tdb import statements.qif --account "Checking" --confirm

# Force a format if auto-detection from the extension fails
tmoney -f personal.tdb import data.txt --account "Checking" --format qif

# Skip duplicates (matched rows are not imported)
tmoney -f personal.tdb import file.qif --account "Checking" --confirm --skip-duplicates

# Update duplicates (matched rows update existing transactions: cleared status, FITID, etc.)
tmoney -f personal.tdb import file.ofx --account "Checking" --confirm --update-duplicates

# Multi-account CSV (e.g. Quicken Mac's "Register Transactions to CSV"):
# the import refuses to run without --source-account when the file
# contains rows for more than one account.
tmoney -f personal.tdb import register.csv --account "BoA Checking" \
  --source-account "Checking" --confirm
```

Supported formats: CSV, QIF (Quicken), and OFX/QFX (bank downloads). The same flow is also available in the TUI via File → Import Transactions… — when the file contains multiple source accounts, the TUI inserts a picker step between the options and confirm dialogs.

### Export Transactions

```bash
# Export full database as CSV (or QIF with --format qif)
tmoney -f personal.tdb export finances.csv

# Export a single account, date range
tmoney -f personal.tdb export checking_q1.csv --account "Checking" \
  --from 2024-01-01 --to 2024-03-31

# Force a format if the extension is ambiguous (csv or qif; ofx not supported)
tmoney -f personal.tdb export out.txt --format qif
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

The CLI is built on [Cobra](https://github.com/spf13/cobra) and follows a noun-verb structure (`tmoney account add`, `tmoney investment portfolio`, `tmoney report net-worth`). Run `tmoney --help` for the top-level command list, or `tmoney <noun> --help` for the verbs in any group. See [`specs/cli.md`](specs/cli.md) for the full reference.

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
