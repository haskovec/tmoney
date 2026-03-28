# TMoney Architecture

## Overview

TMoney is a terminal-based personal finance application written in Go. It provides both a TUI (Terminal User Interface) for interactive use and a CLI for scripting and automation. All data is stored locally in a single DuckDB database file (`.tdb`).

## Project Structure

```
tmoney/
├── cmd/tmoney/          # Application entry point, CLI commands, argument parsing
├── internal/
│   ├── models/          # Domain models and value types
│   ├── repository/      # Data access layer (SQL queries)
│   ├── service/         # Business logic and orchestration
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

## Layered Architecture

The application follows a strict layered architecture where each layer only depends on the layers below it.

```
┌──────────────────────────────────────────────────┐
│          Presentation Layer                       │
│    CLI (cmd/tmoney/)  │  TUI (internal/tui/)     │
├──────────────────────────────────────────────────┤
│          Service Layer (internal/service/)        │
├──────────────────────────────────────────────────┤
│          Repository Layer (internal/repository/) │
├──────────────────────────────────────────────────┤
│          Database Layer (internal/db/)            │
├──────────────────────────────────────────────────┤
│          Domain Models (internal/models/)         │
└──────────────────────────────────────────────────┘
```

### Models Layer (`internal/models/`)

Domain models and value types that are used across all layers. No dependencies on other internal packages.

**Value Types** — Custom types with database serialization (`sql.Scanner`/`driver.Valuer`):

| Type | Purpose | Precision |
|------|---------|-----------|
| `ID` | UUID v7 identifiers | — |
| `Money` | Financial amounts via `alpacadecimal` | 4 decimal places |
| `Quantity` | Share counts for investments | 8 decimal places |
| `Date` | Calendar dates without time | Day |
| `Timestamp` | Points in time (UTC) | RFC3339 |

Each value type has a nullable variant (`NullableID`, `NullableMoney`, etc.) for optional database fields.

**Entity Models** — Structs representing domain objects. All embed `BaseModel` which provides `ID`, `CreatedAt`, and `UpdatedAt` fields:

- `Account` — Financial accounts with type-specific properties (credit limit, interest rate)
- `Transaction` — Money movement with status tracking and transfer linking
- `Split` / `SplitCollection` — Category allocation for split transactions
- `Category` — Two-level hierarchy (parent/subcategory), income or expense
- `Payee` — Transaction counterparties with alias pattern matching
- `ScheduledTransaction` — Recurring transaction templates with frequency rules
- `ReconciliationSession` — Bank statement reconciliation state

**Validation** — The `Validator` struct collects field-level errors and returns `ValidationErrors`.

### Repository Layer (`internal/repository/`)

Data access using raw SQL with parameterized queries against DuckDB. No ORM. Each repository is responsible for a single entity type.

| Repository | Entity | Key Operations |
|------------|--------|----------------|
| `AccountRepository` | Account | CRUD, GetBalance, List |
| `TransactionRepository` | Transaction | CRUD, ListByAccount, Search |
| `SplitRepository` | Split | CRUD, ListByTransaction |
| `TransferRepository` | Transfer pairs | Create linked pairs, GetLinked |
| `CategoryRepository` | Category | CRUD, GetHierarchy |
| `PayeeRepository` | Payee | CRUD, alias matching |
| `ScheduledTransactionRepository` | Scheduled | CRUD, GetDue, GetByAccount |
| `ReconciliationRepository` | Reconciliation | Session lifecycle |

Each repository receives `*db.DB` via constructor injection. Custom error types (`NotFoundError`, `DuplicateError`) provide structured error handling.

### Service Layer (`internal/service/`)

Business logic, validation, and cross-entity orchestration. Services compose one or more repositories and enforce invariants.

| Service | Responsibilities |
|---------|-----------------|
| `AccountService` | Account lifecycle, balance calculation |
| `TransactionService` | Transaction creation with splits and transfers, voiding |
| `CategoryService` | Category hierarchy management |
| `PayeeService` | Payee management, alias resolution |
| `ScheduledTransactionService` | Schedule management, auto-posting, skip/post workflow |
| `ReportService` | Cash flow, category breakdown, net worth |
| `ReconciliationService` | Reconciliation session workflow |

The `Services` struct in `registry.go` is the single initialization point — `NewServices(db)` creates all repositories and services with proper dependency wiring.

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
  → Service.Transaction.Create(txn, splits)
    → Validate fields
    → Repository.Transaction.Create()     [INSERT]
    → Repository.Split.Create() per split [INSERT]
  → TUI: refresh register, update sidebar balances
  → Undo manager: record operation
```

### Transfer Between Accounts

```
User input
  → Service.Transaction.CreateTransfer(from, to, amount)
    → Create TransferPair (two linked transactions)
    → Repository.Transfer.Create()
      → Repository.Transaction.Create() × 2
    → Recalculate balances for both accounts
```

### Scheduled Transaction Auto-Post

```
Application startup
  → Service.ScheduledTxn.AutoPost()
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

User preferences stored in `~/.config/tmoney/config.json`:
- Recent files list
- Default database location

## Key Design Decisions

**DuckDB over SQLite** — DuckDB's columnar storage is well-suited for the analytical queries used in reports (aggregations, date-range filtering). It also provides precise decimal types natively.

**Raw SQL over ORM** — Direct SQL gives full control over query optimization and avoids ORM abstraction leaks. DuckDB's SQL dialect has some differences from SQLite/PostgreSQL that are easier to handle with raw queries.

**Separate investment transaction table** — Investment transactions have fundamentally different fields (shares, price per share, commission, lot references) than regular transactions, warranting a dedicated table rather than overloading the general transactions table.

**Value types over primitives** — Custom `Money`, `ID`, `Date`, and `Quantity` types prevent mixing incompatible values at compile time and centralize serialization logic.

**Service registry pattern** — `NewServices(db)` wires all dependencies in one place, making it easy to initialize the full service layer for both CLI and TUI entry points.

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

- **Unit tests** — Per-package tests using test database fixtures
- **Integration tests** (`tests/integration/`) — Full service-layer workflows against real DuckDB databases
- **TUI tests** — `App` struct instantiated directly without database for component and key-binding verification
