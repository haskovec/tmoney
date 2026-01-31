# TMoney

## What is TMoney?

TMoney is a personal finance management application that runs in the terminal. It's designed for users who prefer keyboard-driven interfaces and want full control over their financial data.

## Core Philosophy

1. **Local-first**: Your data stays on your computer in a single file
2. **Keyboard-driven**: No mouse required, fast navigation
3. **Dual interface**: TUI for daily use, CLI for scripting and automation
4. **Simple but complete**: Essential features without bloat

## Target Users

- Developers and power users comfortable with terminal applications
- Users who want privacy (no cloud sync, no accounts)
- People who like tools like lazygit, vim, or other TUI applications
- Anyone migrating from Quicken/Money who wants something simpler

## Key Features (v1)

### Accounts
- Multiple account types (checking, savings, credit cards, investments, loans)
- Per-account currency setting
- Credit card limits and loan interest rates
- Investment lot-level tracking

### Transactions
- Standard transactions with payee, category, memo
- Split transactions across categories
- Linked transfers between accounts
- Clear/pending status tracking
- Full search capabilities

### Categories
- Two-level hierarchy (parent/subcategory)
- Income vs expense classification
- Default categories provided
- User customizable

### Payees
- Auto-creation on first use
- Default category assignment
- Alias/pattern matching for imports

### Scheduled Transactions
- Multiple frequencies (daily to yearly)
- Fixed or indefinite duration
- Variable amount estimation
- Reminder-based workflow

### Reports
- Net worth calculation
- Spending by category (monthly/yearly)

### Interface
- Modern TUI aesthetic (like lazygit)
- Full CLI for scripting
- Arrow key navigation
- Dialog-based data entry

## Not in v1 (Planned for v1.5+)

- Import/export (CSV, QIF, OFX)
- Reconciliation
- Auto-post scheduled transactions
- Mouse support
- Currency conversion
- Budgeting

## Tech Stack

- **Go**: Performance, single binary, cross-platform
- **Bubbletea**: Modern TUI framework
- **DuckDB**: Embedded analytics database
- **alpacadecimal**: Precise decimal arithmetic

## File Format

TMoney uses `.tdb` files (DuckDB databases) stored in `~/Documents/TMoney/` by default. Each file is self-contained and versioned for future migrations.

## Example Usage

```bash
# Launch TUI
tmoney

# Open specific file
tmoney ~/Documents/TMoney/personal.tdb

# CLI operations
tmoney --list-accounts
tmoney --balance
tmoney --transactions Checking --limit 10
tmoney --add-transaction --account Checking --amount -50 --payee "Coffee Shop"
tmoney --scheduled --due
tmoney --report spending --month 2024-01
```

## Design Inspiration

- **lazygit**: Modern TUI aesthetics, keyboard-first
- **Quicken/Microsoft Money**: Feature set and workflows
- **Turbo Vision**: Text-based UI paradigm (nostalgic reference)
- **ledger-cli**: Plain-text accounting philosophy
