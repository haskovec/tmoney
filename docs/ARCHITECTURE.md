# TMoney Architecture

## Overview

TMoney is a terminal-based personal finance application written in Go. It provides both a TUI (Terminal User Interface) for interactive use and a CLI for scripting and automation. All data is stored locally in a single DuckDB database file (`.tdb`).

## Project Structure

```
tmoney/
├── cmd/tmoney/          # Application entry point, CLI commands, argument parsing
├── internal/
│   ├── account/         # Account feature (model, repository, service)
│   ├── category/        # Category feature (model, repository, service)
│   ├── payee/           # Payee feature (model, repository, service)
│   ├── transaction/     # Transaction feature (model, repos for txn/split/transfer, service)
│   ├── scheduled/       # Scheduled transaction feature (model, repository, service)
│   ├── reconciliation/  # Reconciliation feature (model, repository, service)
│   ├── report/          # Report feature (model, service)
│   ├── security/        # Security master feature (model, repository, service)
│   ├── price/           # Price feature (model, repository, service, provider)
│   ├── investment/      # Investment feature (models, repos for txn/lot/position, service)
│   ├── types/           # Shared value types (Money, ID, Date, Quantity, Validator)
│   ├── dberrors/        # Shared repository error types
│   ├── dbutil/          # Shared database helper functions
│   ├── app/             # Composition root (service registry)
│   ├── db/              # Database connection, migrations, error types
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
│    CLI (cmd/tmoney/)  │  TUI (internal/tui/)     │
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
| `account/` | Account model, repository, service | — |
| `category/` | Category model, repository, service | — |
| `payee/` | Payee model, repository, service | — |
| `security/` | Security model, repository, service | — |
| `transaction/` | Transaction, Split, Transfer models + repos + service | `payee` (for auto-category) |
| `scheduled/` | Scheduled transaction model, repository, service | `transaction` (for posting) |
| `reconciliation/` | Reconciliation session model, repository, service | `transaction`, `account` |
| `report/` | Report models, service (no repository) | `account` |
| `price/` | Price model, repository, service, provider interface | `security` |
| `investment/` | Investment transaction, Lot, Position, TransactionLot models + repos + service | `account` |

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
svc.Transaction.Create(txn, nil)   // transaction.Service
svc.AccountRepo.GetByID(id)       // account.Repository
```

### Database Layer (`internal/db/`)

DuckDB connection management, schema migrations, and file validation.

- `Open(path)` — Opens an existing `.tdb` file, validates it, and runs pending migrations
- `Create(path)` — Creates a new database with the full schema
- Migrations are embedded SQL files (`migrations/*.sql`) applied sequentially
- The `_metadata` table stores `app_identifier` ("tmoney") and `schema_version`

### Presentation Layer

#### CLI (`cmd/tmoney/`)

- `main.go` — Entry point, routes to TUI or CLI command handlers
- `args.go` — Argument parsing into a `cliOptions` struct
- `commands.go` — Command handler functions (one per CLI operation)
- `format.go` — Output formatting (`formatMoney`, table printing)

When no command flags are provided, the application launches the TUI. Otherwise, the specified command runs against the database and exits.

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
  → transaction.Service.Create(txn, splits)
    → Validate fields
    → transaction.Repository.Create()          [INSERT]
    → transaction.SplitRepository.Create()     [INSERT per split]
  → TUI: refresh register, update sidebar balances
  → Undo manager: record operation
```

### Transfer Between Accounts

```
User input
  → transaction.Service.CreateTransfer(from, to, amount)
    → Create TransferPair (two linked transactions)
    → transaction.TransferRepository.Create()
      → transaction.Repository.Create() × 2
    → Recalculate balances for both accounts
```

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

Session-scoped undo/redo via an `Operation` interface with `Do()` and `Undo()` methods. Integrated with TUI dialogs for transaction and account operations. Not persisted across sessions.

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
| `transactions` | Money movement records |
| `transaction_splits` | Category allocation for splits |
| `categories` | Income/expense categories (hierarchical) |
| `payees` | Transaction counterparties |
| `payee_aliases` | Pattern matching rules for payees |
| `investment_lots` | Security lot tracking for investment accounts |
| `scheduled_transactions` | Recurring transaction templates |
| `reconciliation_sessions` | Bank reconciliation state |

### Computed Views

| View | Purpose |
|------|---------|
| `account_balances` | Current and cleared balance per account |
| `category_spending` | Monthly spending aggregated by category |

## Testing

- **Unit tests** — Per-slice tests using test database fixtures (each slice has its own `*_test.go` files)
- **Integration tests** (`tests/integration/`) — Cross-slice workflows against real DuckDB databases
- **TUI tests** — `App` struct instantiated directly without database for component and key-binding verification
