# TMoney

A personal finance management application that runs in the terminal. TMoney is designed for users who prefer keyboard-driven interfaces and want full control over their financial data.

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
tmoney --create ~/Documents/TMoney/personal.tdb

# Launch the TUI
tmoney ~/Documents/TMoney/personal.tdb

# Or use the CLI to add data
tmoney -f personal.tdb --add-account --name "My Checking" --type checking --opening-balance 1000
tmoney -f personal.tdb --add-transaction --account "My Checking" --amount -45.50 --payee "Grocery Store" --category "Food:Groceries"
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

## TUI Interface

Launch the TUI by running `tmoney` with a database file (or no arguments for the default file):

```bash
tmoney                                    # Default file
tmoney ~/Documents/TMoney/personal.tdb    # Specific file
```

The TUI has four main views accessible via number keys or the menu bar:

| Key | View | Description |
|-----|------|-------------|
| `1` | Dashboard | Net worth, account balances, due scheduled transactions |
| `2` | Scheduled | Due and upcoming scheduled transactions |
| `3` | Reports | Net worth and spending by category reports |
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
| `F10` | Activate menu bar |
| `Alt+F` | File menu |
| `Alt+A` | Accounts menu |
| `Alt+T` | Transactions menu |
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

TMoney supports mouse interaction for common operations:

| Action | Effect |
|--------|--------|
| Click menu label | Open/close menu dropdown |
| Click dropdown item | Execute menu action |
| Click account in sidebar | Open account register |
| Click group header in sidebar | Select group heading |
| Click transaction row | Select transaction |
| Click sidebar/table area | Switch focus between panes |
| Scroll wheel | Navigate lists and tables |

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
tmoney --create ~/Documents/TMoney/finances.tdb
```

### Accounts

```bash
# List all accounts
tmoney --list-accounts
tmoney --list-accounts --include-closed

# Show account details
tmoney --account Checking

# Show all balances with net worth
tmoney --balance

# Create a new account
tmoney --add-account --name "Checking" --type checking
tmoney --add-account --name "Visa" --type credit_card --credit-limit 5000
tmoney --add-account --name "Mortgage" --type loan --interest-rate 6.5
tmoney --add-account --name "Savings" --type savings \
  --opening-balance 10000 --opening-date 2024-01-15 \
  --institution "First Bank" --currency USD
```

Account types: `checking`, `savings`, `credit_card`, `investment`, `cash`, `loan`, `asset`

### Transactions

```bash
# List transactions for an account
tmoney --transactions --account Checking
tmoney --transactions --account Checking --limit 20
tmoney --transactions --account Checking --from 2024-01-01 --to 2024-01-31

# Add a transaction (negative amounts for expenses)
tmoney --add-transaction --account Checking --amount -50.00 --payee "Coffee Shop"
tmoney --add-transaction --account Checking --amount -120.00 \
  --payee "Electric Co" --category "Bills:Utilities" \
  --date 2024-03-15 --memo "March electric bill"

# Add income
tmoney --add-transaction --account Checking --amount 3500.00 \
  --payee "Employer Inc" --category "Income:Salary"

# Create a transfer between accounts
tmoney --transfer --from Checking --to Savings --amount 500.00
tmoney --transfer --from Checking --to Savings --amount 500.00 \
  --date 2024-03-01 --memo "Monthly savings"

# Search transactions
tmoney --search "grocery"
tmoney --search "electric" --from 2024-01-01 --to 2024-12-31
tmoney --search "restaurant" --account Visa --min 20 --max 100
tmoney --search "transfer" --category "Transfer"
```

### Scheduled Transactions

```bash
# List all scheduled transactions
tmoney --scheduled
tmoney --scheduled --account Checking

# List only due scheduled transactions
tmoney --scheduled --due

# Post a scheduled transaction (create real transaction from it)
tmoney --post-scheduled <id>
tmoney --post-scheduled <id> --amount 150.00    # Override amount
tmoney --post-scheduled <id> --date 2024-03-20  # Override date

# Skip a scheduled transaction (advance to next occurrence)
tmoney --skip-scheduled <id>
```

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

## File Format

TMoney uses `.tdb` files (DuckDB databases) stored in `~/Documents/TMoney/` by default. Each file is self-contained and versioned with automatic schema migrations.

## Configuration

TMoney stores its configuration at `~/.config/tmoney/config.json`, following the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/latest/). If `$XDG_CONFIG_HOME` is set, it uses `$XDG_CONFIG_HOME/tmoney/` instead.

The config file tracks:
- **Last opened file** — automatically reopened when you launch `tmoney` without specifying a file
- **Recent files** — the 5 most recently opened database files (available via File > Open Recent in the TUI)

This means you can simply run `tmoney` after your first session and it will reopen the last file you were working with. Specifying `-f <path>` or a positional argument always takes priority over the saved default.

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
