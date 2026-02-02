# Implementation Order

This document defines the order in which features should be implemented. Each item represents one session of work. Mark items as complete with `[x]` as they are finished.

## Status Legend

- `[ ]` Not started
- `[x]` Complete

---

## Phase 1: Project Foundation

### 1.1 Project Structure
- [x] **001 - Project scaffold**
  - Create directory structure (cmd/, internal/, etc.)
  - Initialize go.mod with dependencies
  - Create main.go entry point
  - Verify build works

### 1.2 Database Foundation
- [x] **002 - Database connection**
  - DuckDB connection wrapper
  - Open/close file handling
  - Connection pooling (if needed)
  - Error handling

- [x] **003 - Migration system**
  - Migration runner
  - Version tracking in _metadata table
  - Initial schema migration (001_initial.sql)
  - File validation (is this a tmoney file?)

### 1.3 Core Models
- [x] **004 - Base model types**
  - UUID handling
  - Decimal type with alpacadecimal
  - Timestamp handling
  - Common validation helpers

---

## Phase 2: Accounts

- [x] **005 - Account model**
  - Account struct with all properties
  - Account type enum
  - Validation methods

- [x] **006 - Account repository**
  - Create account
  - Read account (by ID, by name)
  - Update account
  - Delete account
  - List accounts (with filters)

- [x] **007 - Account service**
  - Business logic layer
  - Balance calculation
  - Account validation
  - Close account logic

---

## Phase 3: Categories

- [x] **008 - Category model**
  - Category struct
  - Category type enum (income/expense)
  - Parent/child relationship

- [x] **009 - Category repository**
  - CRUD operations
  - Get with parent
  - List by type
  - List top-level
  - List children of parent

- [x] **010 - Default categories**
  - Seed default categories
  - System category flag (Transfer)
  - Run on new file creation

- [x] **011 - Category service**
  - Create with parent validation
  - Delete with transaction check
  - Merge categories

---

## Phase 4: Payees

- [x] **012 - Payee model**
  - Payee struct
  - Alias struct
  - Match type enum

- [x] **013 - Payee repository**
  - Payee CRUD
  - Alias CRUD
  - Find by pattern match

- [x] **014 - Payee service**
  - Auto-create on new name
  - Alias matching logic
  - Default category assignment
  - Merge payees

---

## Phase 5: Transactions

- [x] **015 - Transaction model**
  - Transaction struct
  - Split struct
  - Status enum
  - Transfer fields

- [x] **016 - Transaction repository**
  - Create transaction
  - Read transaction
  - Update transaction
  - Delete transaction
  - List by account
  - List by date range

- [x] **017 - Split transaction repository**
  - Create splits
  - Update splits
  - Delete splits
  - Validate split totals

- [x] **018 - Transfer handling**
  - Create linked transfers
  - Update both sides
  - Delete both sides
  - Transfer category assignment

- [x] **019 - Transaction service**
  - Full transaction lifecycle
  - Split validation
  - Transfer orchestration
  - Balance impact calculation

- [x] **020 - Transaction search**
  - Search by payee
  - Search by memo
  - Search by category
  - Search by date range
  - Combined filters

---

## Phase 6: CLI Foundation

- [x] **021 - CLI scaffold**
  - Root command (manual arg parsing)
  - Global flags (--file, --help, --version)
  - File opening logic
  - Error output handling

- [x] **022 - Account CLI commands**
  - `--list-accounts`
  - `--account <name>`
  - `--balance`
  - Table formatting

- [x] **023 - Transaction CLI commands**
  - `--transactions <account>`
  - `--limit` flag
  - `--from` and `--to` date flags
  - Transaction table formatting

- [ ] **024 - Add transaction CLI**
  - `--add-transaction`
  - Required and optional flags
  - Payee auto-creation
  - Validation and feedback

- [ ] **025 - Transfer CLI**
  - `--transfer`
  - `--from` and `--to` accounts
  - Amount and date

- [ ] **026 - Search CLI**
  - `--search <term>`
  - Filter flags
  - Result formatting

---

## Phase 7: Scheduled Transactions

- [ ] **027 - Scheduled transaction model**
  - Scheduled transaction struct
  - Frequency enum
  - Next date calculation

- [ ] **028 - Scheduled transaction repository**
  - CRUD operations
  - Find due transactions
  - Update next date

- [ ] **029 - Scheduled transaction service**
  - Create scheduled
  - Calculate next occurrence
  - Post scheduled (create real transaction)
  - Skip scheduled
  - Variable amount estimation

- [ ] **030 - Scheduled CLI commands**
  - `--scheduled`
  - `--scheduled --due`
  - `--post-scheduled <id>`
  - `--skip-scheduled <id>`

---

## Phase 8: Reports

- [ ] **031 - Net worth report**
  - Asset/liability calculation
  - Report data structure
  - CLI output formatting

- [ ] **032 - Spending by category report**
  - Monthly aggregation
  - Yearly aggregation
  - Percentage calculation
  - CLI output with bars

- [ ] **033 - Report CLI commands**
  - `--report net-worth`
  - `--report spending --month`
  - `--report spending --year`

---

## Phase 9: TUI Foundation

- [ ] **034 - TUI app scaffold**
  - Bubbletea program setup
  - Main model structure
  - View routing
  - Quit handling

- [ ] **035 - TUI styles**
  - Color palette
  - Lipgloss style definitions
  - Responsive width handling

- [ ] **036 - Sidebar component**
  - Account list
  - Collapsible groups
  - Selection state
  - Keyboard navigation

- [ ] **037 - Menu bar component**
  - Menu items
  - Dropdown menus
  - Keyboard activation

- [ ] **038 - Status bar component**
  - Context display
  - Key hints
  - Notifications

- [ ] **039 - Table component**
  - Generic table rendering
  - Column sizing
  - Row selection
  - Scrolling

- [ ] **040 - Dialog component**
  - Modal overlay
  - Form fields
  - Button row
  - Focus management

---

## Phase 10: TUI Views

- [ ] **041 - Dashboard view**
  - Net worth display
  - Account balances
  - Scheduled transactions due
  - Layout composition

- [ ] **042 - Account register view**
  - Transaction table
  - Balance display
  - Status indicators
  - Row selection

- [ ] **043 - Transaction entry dialog**
  - Date input
  - Payee autocomplete
  - Category selector
  - Amount input
  - Memo input
  - Save/cancel

- [ ] **044 - Split transaction dialog**
  - Split list
  - Add/remove splits
  - Amount validation
  - Category per split

- [ ] **045 - Transfer dialog**
  - From/to account selection
  - Amount input
  - Date input

- [ ] **046 - Account dialogs**
  - New account dialog
  - Edit account dialog
  - Account type selection

- [ ] **047 - Scheduled transactions view**
  - Due section
  - Upcoming section
  - Post/skip actions
  - Navigation

- [ ] **048 - Scheduled transaction dialogs**
  - New scheduled dialog
  - Edit scheduled dialog
  - Frequency selection
  - Duration options

- [ ] **049 - Reports view**
  - Report selection sidebar
  - Net worth display
  - Spending chart
  - Period navigation

---

## Phase 11: Integration & Polish

- [ ] **050 - TUI and CLI integration**
  - Shared services
  - Consistent behavior
  - Error handling

- [ ] **051 - File management**
  - New file creation
  - Recent files tracking
  - Default file location

- [ ] **052 - Input validation**
  - Field-level validation
  - Form-level validation
  - Error messages

- [ ] **053 - Keyboard shortcuts**
  - Global shortcuts
  - View-specific shortcuts
  - Help overlay

- [ ] **054 - Edge cases**
  - Empty states
  - Large data sets
  - Long text handling

- [ ] **055 - Testing**
  - Unit tests for services
  - Repository integration tests
  - TUI component tests

- [ ] **056 - Documentation**
  - README update
  - Usage examples
  - Keyboard reference

---

## Notes

### Dependencies

Each item may depend on previous items:
- Repository items need models
- Services need repositories
- CLI commands need services
- TUI views need services and components

### Session Guidelines

1. Read the relevant spec file before starting
2. Implement the item completely
3. Test manually
4. Mark as complete in this file
5. Commit changes

### Adjustments

This plan can be adjusted as we learn more during implementation. If an item is too large, split it. If items can be combined, do so. Update this document to reflect changes.
