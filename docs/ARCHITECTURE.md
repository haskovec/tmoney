# TMoney Architecture

## Overview

TMoney is a terminal-based personal finance application written in Go. It provides both a TUI (Terminal User Interface) for interactive use and a CLI for scripting and automation. All data is stored locally in a single DuckDB database file (`.tdb`).

## Project Structure

```
tmoney/
├── main.go              # Thin entry point — calls cli.Execute()
├── internal/
│   ├── cli/             # Cobra-based CLI (noun-verb subcommands) + TUI launcher
│   │   ├── cmdutil/     #   shared command hub (FormatMoney, OpenServices, RequireFile)
│   │   ├── clitest/     #   cli-free test fixtures (DB builders, PtrMoney)
│   │   └── <noun>/      #   11 per-noun packages (account, transaction, transfer, investment, …)
│   ├── account/         # Account feature (model, repository, service)
│   ├── category/        # Category feature (model, repository, service)
│   ├── payee/           # Payee feature (model, repository, service)
│   ├── transaction/     # Register ledger (model, repos for txn/split, service)
│   ├── transfer/        # Owner of cash transfers across both ledgers (create, edit, void, delete)
│   ├── transferlink/    # Links two existing rows into a transfer, through transfer/
│   ├── scheduled/       # Scheduled transaction feature (model, repository, service)
│   ├── loan/            # Loan amortization engine (used by scheduled/, the CLI, and the TUI)
│   ├── reconciliation/  # Reconciliation feature, register ledger only (model, repository, service)
│   ├── report/          # Reports: account figures, net worth per currency, spending (no repository)
│   ├── security/        # Security master feature (model, repository, service)
│   ├── price/           # Price feature (model, repository, service, provider)
│   ├── investment/      # Investment ledger (txn/lot/position repos, trades, share transfers,
│   │                    #   corporate actions, valuation)
│   ├── types/           # Shared value types (Money, ID, Date, Quantity, Validator)
│   ├── dberrors/        # Shared repository error types
│   ├── dbutil/          # Shared database helper functions
│   ├── dbtest/          # Test database builders
│   ├── app/             # Composition root (service registry)
│   ├── db/              # Database connection, migrations, transactions (WithTx), error types
│   ├── applog/          # Append-only app log (~/.config/tmoney/log.txt)
│   ├── tui/             # Bubbletea TUI application
│   ├── config/          # User configuration (~/.config/tmoney/)
│   ├── backup/          # Database backup and restore
│   ├── imexport/        # Import/export (CSV, OFX, QIF)
│   └── undo/            # Undo/redo operation manager
├── tests/integration/   # Integration tests with real database
├── specs/               # Feature specifications
└── docs/                # Documentation
```

## Vertical Slice Architecture

The application follows a vertical slice architecture where code is organized by feature. Each feature slice contains its own model, repository, and service in a single package.

```
┌──────────────────────────────────────────────────┐
│          Presentation Layer                       │
│   CLI (internal/cli/) │  TUI (internal/tui/)     │
├──────────────────────────────────────────────────┤
│          Composition Root (internal/app/)         │
├──────────────────────────────────────────────────┤
│          Feature Slices                           │
│  account/ │ transaction/ │ category/ │ payee/ ... │
│  (model + repository + service per slice)        │
├──────────────────────────────────────────────────┤
│          Shared Foundation                        │
│  types/ │ dberrors/ │ dbutil/ │ db/              │
└──────────────────────────────────────────────────┘
```

### Feature Slice Convention

Each feature slice follows a consistent file naming convention:

| File | Contains |
|------|----------|
| `model.go` | Domain model struct(s), enums, constructors, validation |
| `repository.go` | Repository struct, CRUD operations, SQL queries |
| `service.go` | Service struct, business logic, cross-entity orchestration |
| `errors.go` | Feature-specific error types |

Types are named to avoid stutter with the package name:
- `account.Service` (not `account.AccountService`)
- `account.Repository` (not `account.AccountRepository`)
- `account.Type` (not `account.AccountType`)
- `transaction.Status` (not `transaction.TransactionStatus`)

### Feature Slices

| Slice | Contents | Cross-Slice Dependencies |
|-------|----------|------------------------|
| `account/` | Account model, repository, service (close rule via the `InvestmentLedger` port) | — |
| `category/` | Category model, repository, service | — |
| `payee/` | Payee model, repository, service | — |
| `security/` | Security model, repository, service | — |
| `transaction/` | Register ledger: Transaction and Split models + repos + service | `account`, `category`, `payee` |
| `transfer/` | Cash transfers across both ledgers: the one door for create, edit, void, delete | `account`, `category`, `investment`, `transaction` |
| `transferlink/` | Finds rows to link as a transfer; `transfer/` performs the link | `account`, `transaction` |
| `scheduled/` | Scheduled transaction model, repository, service | `account`, `category`, `loan`, `transaction` (transfers post through a port into `transfer/`) |
| `loan/` | Loan amortization engine | — |
| `reconciliation/` | Reconciliation session model, repository, service | `account`, `transaction` |
| `report/` | Report models, service (no repository) | `account` (investment value through the `InvestmentValuer` port) |
| `price/` | Price model, repository, service, provider interface | `security` |
| `investment/` | Investment ledger: transaction, Lot, Position models + repos; trades, share transfers, corporate actions, `ValuationService` | `account`, `price`, `security` |

Where the import direction forbids a call, a port defined in the lower package carries it, and `internal/app` wires the implementation: `account.InvestmentLedger` is implemented by `investment.ValuationService`, `report.InvestmentValuer` by a small adapter over it in `internal/app`, and `scheduled`'s transfer port by `transfer.Service`.

### Shared Foundation Packages

#### Value Types (`internal/types/`)

Custom types with database serialization (`sql.Scanner`/`driver.Valuer`):

| Type | Purpose | Precision |
|------|---------|-----------|
| `ID` | UUID v7 identifiers | — |
| `Money` | Financial amounts via `alpacadecimal` | 4 decimal places |
| `Quantity` | Share counts for investments | 8 decimal places |
| `Date` | Calendar dates without time | Day |
| `Timestamp` | Points in time (UTC) | RFC3339 |

Each value type has a nullable variant (`NullableID`, `NullableMoney`, etc.) for optional database fields.

Also contains: `BaseModel` (common ID/timestamp fields), `Validator` (fluent validation builder), `ValidationErrors`, and `ServiceValidationError`.

#### Repository Errors (`internal/dberrors/`)

Shared error types used by all repositories: `NotFoundError`, `DuplicateError`, `HasDependentsError`.

#### Database Utilities (`internal/dbutil/`)

Exported helper functions for converting nullable types to SQL parameter values (`NullString`, `NullMoney`, `NullID`, `NullDate`, etc.).

### Composition Root (`internal/app/`)

The `Services` struct and `NewServices(db)` factory function wire all repositories and services with proper dependency injection. This is the single initialization point used by both CLI and TUI entry points.

```go
svc := app.NewServices(database)
svc.Account.Create(acct)           // account.Service
svc.Transaction.Create(txn)        // transaction.Service
svc.AccountRepo.GetByID(id)       // account.Repository
```

### Database Layer (`internal/db/`)

DuckDB connection management, schema migrations, and file validation.

- `Open(path)` — Opens an existing `.tdb` file, validates it, and runs pending migrations
- `Create(path)` — Creates a new database with the full schema
- Migrations are embedded SQL files (`migrations/*.sql`) applied sequentially
- The `_metadata` table stores `app_identifier` ("tmoney") and `schema_version`

### Presentation Layer

#### CLI (`internal/cli/`)

The CLI is built on [Cobra](https://github.com/spf13/cobra) with a
noun-verb taxonomy (`tmoney account add`, `tmoney transfer edit`,
`tmoney investment buy`). A thin `main.go` at the repo root calls
`cli.Execute()`.

The package is split into one subpackage per noun plus a shared hub,
mirroring the horizontal extraction in `internal/tui` (`widget/`, `dialog/`,
`theme/`). See [`specs/cli-package-split.md`](../specs/cli-package-split.md)
for the full design and per-PR history.

- **Top-level `internal/cli/`** — `root.go` (`newRootCmd()` wires the
  persistent `--file`/`-f` flag, registers all 11 noun command groups, and
  exports the `ExecuteWith` + `SwapTUILauncher` test seams), `tui.go` (drops
  into the Bubbletea TUI when no subcommand is given), and the single-verb
  commands `version.go`, `import.go`, `export.go` that don't warrant their own
  package.
- **`cmdutil/`** — the shared command hub (the `widget/` analog): a cli-free
  leaf holding `FormatMoney`, `OpenServices`, `AutoBackupAfterModification`,
  and `RequireFile`. Every noun imports it; it imports no other `cli` package,
  keeping the dependency graph acyclic.
- **`clitest/`** — test fixtures only (`CreateInvestmentTestDB`,
  `CreateTestDBWithSecurity`, `PtrMoney`, transfer-account builders). It imports
  only domain packages, **never `cli`**, so both external `_test` packages and
  internal white-box tests can use it without an import cycle.
- **Per-noun packages** — `account/`, `db/`, `transaction/`, `transfer/`,
  `scheduled/`, `reconcile/`, `security/`, `price/`, `investment/`, `report/`,
  `theme/`. Each exposes a single exported `NewCmd()` constructor (e.g.
  `account.NewCmd()`, which `root.go` registers), one file per verb
  (`transfer/add.go`, `transfer/edit.go`, `transfer/delete.go`), and the table
  printers for that noun (the old shared `format.go` god-file dissolved into
  each noun). The 9 nouns whose package name collides with a same-named domain
  package alias the domain import (`accountdom`, `pricedom`, …); `transfer` and
  `reconcile` need no alias.

When invoked with no subcommand the application launches the TUI;
otherwise the named subcommand runs against the database and exits.

#### TUI (`internal/tui/`)

Built on the [Bubbletea](https://github.com/charmbracelet/bubbletea) framework with [Lipgloss](https://github.com/charmbracelet/lipgloss) styling.

**Application Model** — The `App` struct implements `tea.Model` and manages:
- Current view state (Dashboard, Register, Scheduled, Reports, Reconciliation)
- All service references for data access
- Component instances (sidebar, menu bar, status bar, tables)
- Dialog state for modal forms

**Views:**

| View | Purpose |
|------|---------|
| Dashboard | Net worth summary, account list, due scheduled transactions |
| Register | Transaction list for a selected account |
| Scheduled | Scheduled transaction management |
| Reports | Net worth and spending reports |
| Reconciliation | Bank reconciliation workflow |

**Components:**

| Component | Purpose |
|-----------|---------|
| `Sidebar` | Account list navigation |
| `MenuBar` | Top menu (File, Accounts, Transactions, Reports, Help) |
| `StatusBar` | Context-sensitive keyboard shortcut hints |
| `Table` | Scrollable data table with selection |
| `Dialog` | Modal forms with text, select, radio, and checkbox fields |
| `HelpOverlay` | Full keyboard shortcut reference |

**Async Data Loading** — Data is loaded via `tea.Cmd` functions that run in goroutines and return typed messages (e.g., `dashboardLoadedMsg`). The `Update()` method processes these messages to update state without blocking the UI.

**Responsive Layout** — Three layout modes (Small < 80 cols, Medium 80–120, Large > 120) adjust column visibility, sidebar width, and component sizing.

## Data Flow

### Creating a Transaction

```
User input (TUI dialog or CLI flags)
  → transaction.Service.Create(txn)                 (or CreateWithSplits(txn, splits))
    → Validate fields
    → transaction.Repository.Create()          [INSERT]
    → transaction.SplitRepository.Create()     [INSERT per split]
  → TUI: refresh register and sidebar
  → Undo manager: record operation
```

### Transfer Between Accounts

A cash transfer has one owner, `transfer.Service`, for every pair of ledgers:
bank↔bank, bank↔investment, and investment↔investment. Create, edit, void, and
delete all go through it; callers do not reach past it into
`transaction.Service` or `investment.Service` for transfer work.

```
User input (TUI transfer dialog, `tmoney transfer add`)
  → transfer.Service.Create(transfer.Spec{FromAccountID, ToAccountID, Date, Amount, ...})
    → Validate both accounts and the category rules
    → db.DB.WithTx: write both legs in one database transaction
        register account   → transactions row
        investment account → investment_transactions row (transfer_cash)
      each leg carries the shared transfer_id and names the other account
```

Nothing is recalculated or stored: balances are computed on read (see
Balances below). Share transfers are separate and stay in `investment/`
(`Service.TransferShares`, `EditService.UpdateTransferShares`).

### Balances on Two Ledgers

An account keeps its history on one of two ledgers. Checking, savings, credit
card, cash, loan, asset, and HSA cash use the register ledger (`transactions`).
`investment` and `hsa_investment` use the investment ledger
(`investment_transactions`, with `investment_lots` and `investment_positions`).

- **Register balance** — `account_balances` view, read by
  `account.Service.GetBalance`: the current balance is the opening balance
  plus the non-void register rows, and the cleared balance is the opening
  balance plus the rows with status `cleared` or `reconciled`. The view does
  this for every account type. An investment account can have register rows
  (`tmoney transaction add`, an import, or a posted non-transfer schedule
  writes them), and `account.Service.Close` refuses while they total other
  than zero. Either way the sum is not what an investment account holds.
- **Investment value** — `investment.ValuationService.GetAccountValuation`:
  cash on the investment ledger plus the market value of open holdings.
- **What the CLI shows** — `report.Service.AccountFigure` and
  `AccountFigures`: the register balance for a register account, the
  investment value for an investment account. `account list` and
  `account balance` take the figure they print from `AccountFigures`.
  `account show` uses `AccountFigures` for an investment account (cash and
  total value); for a register account it prints the current and cleared
  balance from `account.Service.GetBalance`, since a figure has no cleared
  balance.
- **Net worth** — `report.Service.NetWorthAsOf`: one total per currency; money
  in different currencies is never added.
- **Close** — `account.Service.Close` judges an investment account by its
  investment ledger (cash and shares) through the `account.InvestmentLedger`
  port, not by its register balance.

### Scheduled Transaction Auto-Post

```
Application startup
  → scheduled.Service.AutoPost()
    → Get due transactions (next_date ≤ today)
    → For each: create real transaction, advance schedule
    → Return PostSummary (count, details)
```

## Supporting Packages

### Import/Export (`internal/imexport/`)

Supports three formats for transaction data interchange:

| Format | Extension | Import | Export |
|--------|-----------|--------|--------|
| CSV | `.csv` | Yes | Yes |
| OFX | `.ofx` | Yes | Yes |
| QIF | `.qif` | Yes | No |

The `ImportService` parses files and creates transactions, using a `Matcher` to resolve imported payee names against existing payees and aliases.

### Backup (`internal/backup/`)

Creates timestamped copies of the database file. Supports listing available backups and restoring from a selected backup.

### Undo/Redo (`internal/undo/`)

Session-scoped undo/redo via a `Command` interface with `Execute()`, `Undo()`, and `Description()` methods. Integrated with TUI dialogs for transaction and account operations. Not persisted across sessions.

### Configuration (`internal/config/`)

User preferences stored in `~/.config/tmoney/config.json` (or `$XDG_CONFIG_HOME/tmoney/config.json`):
- Last opened file (auto-reopened on next launch)
- Recent files list (5 most recent)
- Default database location

## Key Design Decisions

**Vertical slices over layered architecture** — Code is organized by feature (account, transaction, category, etc.) rather than by technical layer (models, repository, service). Each slice is self-contained, making it easier to understand and modify a single feature without jumping between packages. Cross-slice dependencies are explicit imports between packages.

**DuckDB over SQLite** — DuckDB's columnar storage is well-suited for the analytical queries used in reports (aggregations, date-range filtering). It also provides precise decimal types natively.

**Raw SQL over ORM** — Direct SQL gives full control over query optimization and avoids ORM abstraction leaks. DuckDB's SQL dialect has some differences from SQLite/PostgreSQL that are easier to handle with raw queries.

**Separate investment transaction table** — Investment transactions have fundamentally different fields (shares, price per share, commission, lot references) than regular transactions, warranting a dedicated table rather than overloading the general transactions table.

**Value types over primitives** — Custom `Money`, `ID`, `Date`, and `Quantity` types prevent mixing incompatible values at compile time and centralize serialization logic.

**Central composition root** — `app.NewServices(db)` wires all dependencies in one place, keeping feature slices unaware of each other's initialization details while providing a single entry point for both CLI and TUI.

## Database

TMoney uses DuckDB as an embedded database. Each `.tdb` file is a self-contained DuckDB database.

### Schema

See [specs/database.md](../specs/database.md) for the complete schema definition including all tables, indexes, views, and migration strategy.

### Key Tables

| Table | Purpose |
|-------|---------|
| `_metadata` | File identification and schema version |
| `accounts` | Financial accounts |
| `transactions` | Register ledger: money movement records |
| `transaction_splits` | Category allocation for splits |
| `investment_transactions` | Investment ledger: trades, cash, income, transfers |
| `investment_positions` | Aggregate share position per account and security |
| `categories` | Income/expense categories (hierarchical) |
| `payees` | Transaction counterparties |
| `payee_aliases` | Pattern matching rules for payees |
| `investment_lots` | Security lot tracking for investment accounts |
| `scheduled_transactions` | Recurring transaction templates |
| `reconciliation_sessions` | Bank reconciliation state |

### Computed Views

| View | Purpose |
|------|---------|
| `account_balances` | Register balance per account: current (opening balance + non-void register rows) and cleared (opening balance + `cleared` and `reconciled` rows). Not an investment account's value; see Balances on Two Ledgers |
| `portfolio_holdings` | Open shares and cost basis per active investment account and security (lots for a lot-tracked account, positions otherwise) |
| `category_spending` | Monthly spending aggregated by category |

## Testing

- **Unit tests** — Per-slice tests using test database fixtures (each slice has its own `*_test.go` files)
- **Integration tests** (`tests/integration/`) — Cross-slice workflows against real DuckDB databases
- **TUI tests** — `App` struct instantiated directly without database for component and key-binding verification
