# Implementation Plan v1.5

This document defines the order in which v1.5 features should be implemented. Each item represents one session of work. Mark items as complete with `[x]` as they are finished.

## Status Legend

- `[ ]` Not started
- `[x]` Complete

## Priority Rationale

1. **Transaction Status** — foundational schema change; prerequisite for reconciliation, void, and undo
2. **Undo/Redo** — safety net should be in place before users perform destructive operations like void and reconciliation
3. **Backup/Restore** — data protection before import (which creates many transactions at once)
4. **Reconciliation** — depends on transaction status; core financial workflow
5. **Auto-post Scheduled Transactions** — depends on transaction status; straightforward extension
6. **Import/Export** — largest feature; benefits from undo (can undo imports) and backup (safety)
7. **Mouse Support** — independent; can be added last without affecting other features

---

## Phase 1: Transaction Status Expansion

Spec: `specs/v1.5/transaction-status.md`

- [ ] **057 - Database migration for transaction status**
  - Add migration to rename `pending` → `uncleared`
  - Add `void` as valid status value
  - Ensure `reconciled` status is active
  - Update `_metadata` version

- [ ] **058 - Update transaction model and repository for new statuses**
  - Update status enum in models (add `Uncleared`, `Void`)
  - Update repository queries to handle four statuses
  - Update balance calculations to exclude void transactions
  - Cleared balance includes `cleared` + `reconciled`

- [ ] **059 - Implement void transaction logic**
  - Service method to void a transaction (zero amount, set memo to `**VOID**`)
  - Void transfer logic (void both sides atomically)
  - Void split transaction logic
  - Prevent editing void transactions
  - Prevent voiding reconciled transactions (must un-reconcile first)

- [ ] **060 - Implement reconciled transaction locking**
  - Prevent editing reconciled transactions at service layer
  - Un-reconcile method (reconciled → cleared with warning)
  - Update transaction service edit/delete to check reconciled status

- [ ] **061 - Update CLI for new transaction statuses**
  - Update `--status` flag to accept new values
  - Add `--void <txn-id>` command
  - Update transaction list output to show U/C/R/V status codes
  - Update `pending` references throughout CLI code

- [ ] **062 - Update TUI for new transaction statuses**
  - Update register view to show four status indicators
  - Add `v` key to void selected transaction (with confirmation dialog)
  - Dim/strikethrough styling for void transactions
  - Lock indicator when editing reconciled transaction attempted
  - Update `c` key to cycle uncleared ↔ cleared

- [ ] **063 - Tests for transaction status changes**
  - Unit tests for void logic (regular, transfer, split)
  - Unit tests for reconciled locking and un-reconcile
  - Unit tests for balance calculation excluding void
  - Migration test

---

## Phase 2: Undo/Redo

Spec: `specs/v1.5/undo-redo.md`

- [ ] **064 - Undo/redo framework**
  - Define `UndoableCommand` interface
  - Implement `CompoundCommand` for grouped operations
  - Implement `UndoManager` with undo/redo stacks
  - Platform-aware keybinding detection (macOS vs Linux/Windows)

- [ ] **065 - Undoable commands for transactions**
  - CreateTransactionCommand
  - EditTransactionCommand (stores before state)
  - DeleteTransactionCommand (stores full entity)
  - VoidTransactionCommand (stores original amount, memo, status)
  - CreateTransferCommand / DeleteTransferCommand (compound)
  - VoidTransferCommand (compound)

- [ ] **066 - Undoable commands for accounts, categories, payees**
  - Account: create, edit, delete, close, reopen
  - Category: create, edit, delete, merge (compound)
  - Payee: create, edit, delete, merge (compound)

- [ ] **067 - Undoable commands for scheduled transactions**
  - Create, edit, delete scheduled transaction
  - PostScheduledCommand (compound: create transaction + advance schedule)
  - SkipScheduledCommand

- [ ] **068 - TUI undo/redo integration**
  - Wire Cmd+Z / Ctrl+Z and Cmd+Y / Ctrl+Y keybindings
  - Status bar notifications for undo/redo actions
  - Add Edit menu to menu bar with Undo/Redo items
  - Route all TUI data operations through UndoManager
  - Handle "Nothing to undo/redo" feedback

- [ ] **069 - Tests for undo/redo**
  - Unit tests for UndoManager (stack behavior, redo clearing)
  - Unit tests for each command type (execute + undo roundtrip)
  - Unit tests for compound commands (partial failure rollback)
  - TUI integration tests for keybindings

---

## Phase 3: Backup/Restore

Spec: `specs/v1.5/backup-restore.md`

- [ ] **070 - Auto-backup on app close**
  - Implement backup file creation (file copy with timestamp)
  - Rolling retention: keep last 5 auto-backups, delete oldest
  - Naming convention: `<file>.tdb.backup.<timestamp>`
  - Trigger on TUI quit
  - Trigger on CLI command completion (for data-modifying commands)

- [ ] **071 - Manual backup and list backups**
  - CLI `--backup` command for manual backup creation
  - CLI `--list-backups` command showing available backups with date, size, type
  - Manual backup naming: `<file>.tdb.manual-backup.<timestamp>`
  - Manual backups excluded from auto-rotation

- [ ] **072 - Restore functionality**
  - Safe restore process (copy to temp, verify, rename)
  - CLI `--restore <backup-file>` command
  - Safety backup of current state before restoring
  - Database connection reload after restore

- [ ] **073 - TUI backup/restore integration**
  - File menu → Create Backup (with status bar notification)
  - File menu → Restore from Backup... (backup selection dialog)
  - Confirmation dialog before restore
  - Database reload after restore

- [ ] **074 - Tests for backup/restore**
  - Unit tests for backup creation and naming
  - Unit tests for rolling retention logic
  - Unit tests for safe restore process
  - Integration tests for CLI commands

---

## Phase 4: Reconciliation

Spec: `specs/v1.5/reconciliation.md`

- [ ] **075 - Reconciliation session model and repository**
  - Reconciliation session struct (id, account_id, statement_date, statement_balance, status)
  - Database migration for reconciliation_sessions table
  - CRUD operations for sessions
  - Constraint: one active session per account

- [ ] **076 - Reconciliation service**
  - Start reconciliation (create session, validate account)
  - Mark transactions for reconciliation
  - Finish reconciliation (validate $0 difference, update statuses)
  - Cancel reconciliation
  - Get reconciliation status
  - Cleared total calculation (opening_balance + reconciled + checked)

- [ ] **077 - CLI reconciliation commands**
  - `--start-reconcile --account <name> --statement-date <date> --statement-balance <amount>`
  - `--mark-reconciled <txn-id>...`
  - `--finish-reconcile --account <name>` (with `--force` option)
  - `--reconcile-status --account <name>`

- [ ] **078 - TUI reconciliation view**
  - Dedicated full-screen reconciliation view
  - Start reconciliation dialog (statement date + ending balance)
  - Transaction list with checkboxes
  - Sticky footer: statement balance, cleared total, difference, count
  - Keyboard: Space toggle, Enter finish, Esc cancel, a check-all, u uncheck-all

- [ ] **079 - Reconciliation undo integration**
  - ReconcileCommand (compound: update N transaction statuses)
  - Un-reconcile undoes the entire reconciliation session
  - Wire through UndoManager

- [ ] **080 - Tests for reconciliation**
  - Unit tests for reconciliation service (start, mark, finish, cancel)
  - Unit tests for balance calculations
  - Unit tests for edge cases (non-zero difference, force finish, closed account)
  - TUI component tests for reconciliation view

---

## Phase 5: Auto-post Scheduled Transactions

Spec: `specs/v1.5/auto-post.md`

- [ ] **081 - Database migration for auto-post fields**
  - Add `auto_post` (boolean, default false) to scheduled_transactions
  - Add `post_lead_days` (integer, default 0) to scheduled_transactions
  - Migration script

- [ ] **082 - Auto-post service logic**
  - Auto-post check on file open (find due auto-post transactions)
  - Post multiple overdue occurrences
  - Lead time calculation (next_date - post_lead_days <= today)
  - Handle variable amounts (use estimate, skip if unavailable)
  - Return count and details of auto-posted transactions

- [ ] **083 - Update CLI for auto-post**
  - Add `--auto-post` and `--lead-days` flags to `--add-scheduled`
  - Auto-post runs on CLI file open, prints summary
  - Update `--scheduled` output to show auto-post indicator

- [ ] **084 - Update TUI for auto-post**
  - Auto-post runs on file open, status bar notification
  - Update new/edit scheduled transaction dialog with auto-post checkbox and lead time radio
  - Update scheduled transactions view to show auto-post indicators ([Auto], [Auto 3d], [Auto 7d])

- [ ] **085 - Auto-post undo integration**
  - Auto-posted transactions are grouped as compound undo commands
  - Each auto-post session is a single undo step

- [ ] **086 - Tests for auto-post**
  - Unit tests for auto-post detection logic
  - Unit tests for lead time calculation
  - Unit tests for multiple overdue occurrences
  - Unit tests for variable amount handling (estimate, skip)
  - Integration tests for CLI auto-post

---

## Phase 6: Import/Export

Spec: `specs/v1.5/import-export.md`

### 6.1 Format Parsers

- [ ] **087 - CSV parser and writer**
  - Parse CSV files with header row mapping
  - Handle split transactions (multiple rows per parent)
  - Write CSV with standard column layout
  - RFC 4180 compliance

- [ ] **088 - QIF parser and writer**
  - Parse QIF transaction records (D, T, P, L, M, C, N fields)
  - Parse QIF split transactions (S, $, E fields)
  - Parse account type headers
  - Write QIF with proper field codes and record separators

- [ ] **089 - OFX parser (import only)**
  - Parse OFX/QFX XML-like format
  - Extract FITID, date, amount, payee/memo, check number, transaction type
  - Handle various OFX date formats
  - Robust parsing for different bank OFX flavors

### 6.2 Import Engine

- [ ] **090 - Fuzzy matching engine**
  - Amount-exact matching (required first pass)
  - Date closeness scoring (within +/- 7 day window)
  - Payee name similarity scoring (fuzzy string comparison)
  - Combined confidence score (high/low/no match)
  - FITID matching override for OFX imports

- [ ] **091 - Import service**
  - Parse file into intermediate transaction list
  - Run matching against existing account transactions
  - Auto-categorize using payee-to-category mappings
  - Apply import actions (create new, update matched, skip)
  - Bank reference ID storage on transactions (new field)

- [ ] **092 - Database migration for bank reference ID**
  - Add `bank_reference_id` (string, nullable) to transactions table
  - Index for FITID lookup

### 6.3 Export Engine

- [ ] **093 - Export service**
  - Query transactions by account and date range
  - Include related data (payees, categories, splits, transfers)
  - Full database export (accounts, categories, payees, scheduled)
  - Format-agnostic data collection, format-specific writing

### 6.4 CLI Commands

- [ ] **094 - CLI import commands**
  - `--import <file> --account <name>` (dry-run)
  - `--import <file> --account <name> --confirm` (execute)
  - `--import <file> --account <name> --format <fmt>` (override format)
  - `--skip-duplicates` and `--update-duplicates` flags
  - Dry-run summary output
  - Execution summary output

- [ ] **095 - CLI export commands**
  - `--export <file>` (full database)
  - `--export <file> --format csv|qif`
  - `--export <file> --account <name>`
  - `--export <file> --from <date> --to <date>`

### 6.5 TUI Integration

- [ ] **096 - TUI file browser dialog**
  - Directory navigation with arrow keys
  - File extension filtering
  - File size display
  - Path bar and parent directory navigation
  - Type-ahead filename filtering

- [ ] **097 - TUI import workflow**
  - File menu → Import (opens file browser)
  - Account selection dialog
  - Import options dialog (format, duplicate detection)
  - Full-screen import review view
  - Row state cycling (match/new/skip)
  - Inline category editing
  - Confirm/cancel import

- [ ] **098 - TUI export workflow**
  - File menu → Export (opens export dialog)
  - Format selection, account filter, date range
  - File browser for output destination
  - Progress/completion notification

### 6.6 Integration

- [ ] **099 - Import undo integration**
  - Import creates a compound undo command for all created/updated transactions
  - Single undo step reverts the entire import

- [ ] **100 - Tests for import/export**
  - Unit tests for CSV parser and writer
  - Unit tests for QIF parser and writer
  - Unit tests for OFX parser
  - Unit tests for fuzzy matching engine
  - Unit tests for import service (create, update, skip scenarios)
  - Unit tests for export service
  - Integration tests for CLI import/export
  - TUI component tests for import review view

---

## Phase 7: Mouse Support

Spec: `specs/v1.5/mouse-support.md`

- [ ] **101 - Mouse event infrastructure**
  - Enable Bubbletea mouse support (`tea.WithMouseCellMotion()`)
  - Add `--no-mouse` CLI flag
  - Add `mouse_enabled` config setting
  - Priority logic: CLI flag > config > default (enabled)

- [ ] **102 - Mouse click handling for components**
  - Sidebar: click to select account
  - Table/register: click to select row
  - Menu bar: click to open menus
  - Menu dropdown: click to execute action
  - Dialog fields: click to focus
  - Dialog buttons: click to activate
  - Checkboxes and radio buttons: click to toggle

- [ ] **103 - Mouse scroll handling**
  - Scroll wheel in sidebar (account list)
  - Scroll wheel in register (transaction list)
  - Scroll wheel in scheduled transactions view
  - Scroll wheel in reports view
  - Scroll wheel in reconciliation view
  - Scroll wheel in import review
  - Scroll wheel in file browser dialog
  - Scroll speed: 3 lines for content, 1 item for selectable lists

- [ ] **104 - Mouse hit testing and bounds tracking**
  - Components track their screen bounds (x, y, width, height)
  - Hit test translates screen coordinates to component-local coordinates
  - Focus management: click sets focus to target component
  - Clicks outside dialog are ignored when dialog is open

- [ ] **105 - Tests for mouse support**
  - Unit tests for hit testing logic
  - Unit tests for scroll handling
  - Unit tests for config/flag toggle logic
  - Verify keyboard-only mode has no regressions when mouse disabled

---

## Phase 8: Polish & Documentation

- [ ] **106 - Update README for v1.5**
  - Document new features
  - Update keyboard shortcuts reference
  - Add import/export examples
  - Add reconciliation examples

- [ ] **107 - Update existing v1 specs with v1.5 changes**
  - Update `specs/transactions.md` status section
  - Update `specs/scheduled-transactions.md` auto-post section
  - Update `specs/tui.md` mouse and new views
  - Update `specs/cli.md` new commands
  - Remove "v1.5" deferred notes from v1 specs

---

## Notes

### Dependencies

```
Transaction Status (Phase 1)
    ├── Undo/Redo (Phase 2) — needs status for void commands
    ├── Reconciliation (Phase 4) — needs reconciled status
    └── Auto-post (Phase 5) — needs uncleared status for new transactions

Undo/Redo (Phase 2)
    ├── Reconciliation (Phase 4) — reconcile is undoable
    ├── Auto-post (Phase 5) — auto-post is undoable
    └── Import/Export (Phase 6) — import is undoable

Backup/Restore (Phase 3) — independent, but good to have before import

Import/Export (Phase 6) — depends on transaction status, undo, and backup

Mouse Support (Phase 7) — fully independent
```

### Session Guidelines

1. Read the relevant spec file before starting
2. Implement the item completely
3. Run existing tests to verify no regressions
4. Test manually
5. Mark as complete in this file
6. Commit changes

### Adjustments

This plan can be adjusted as we learn more during implementation. If an item is too large, split it. If items can be combined, do so. Update this document to reflect changes.
