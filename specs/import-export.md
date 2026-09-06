# Import/Export Specification (v1.5)

## Overview

Import/Export enables users to bring in transactions from bank downloads and other finance tools, and export their financial data for use elsewhere. Three formats are supported: CSV, QIF, and OFX.

## Supported Formats

| Format | Import | Export | Description |
|--------|--------|--------|-------------|
| CSV | Yes | Yes | Universal comma-separated values |
| QIF | Yes | Yes | Quicken Interchange Format |
| OFX/QFX | Yes | No | Open Financial Exchange (bank downloads) |

Format is auto-detected from file extension (`.csv`, `.qif`, `.ofx`, `.qfx`) but can be overridden.

---

## Import

### Import Workflow (TUI)

1. **File selection**: user navigates a **file browser dialog** to select the import file
2. **Account selection**: user picks the target account from a dropdown
3. **Options dialog**:
   - Format (auto-detected, can override)
   - Enable/disable duplicate detection (user chooses per import)
4. **Parsing**: TMoney reads and parses the file
5. **Import review view**: full-screen table showing all parsed transactions

### Import Review View (TUI)

A **dedicated full-screen view** showing all imported transactions.

```
┌─────────────────────────────────────────────────────────────────────┐
│ IMPORT REVIEW: checking_jan.ofx → Checking          47 transactions │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  Status   Date     Payee              Amount     Category    Match  │
│  ──────────────────────────────────────────────────────────────────  │
│  ✓ Match  01/03    LANDLORD PMT       -1,500.00  Rent        01/03 Landlord │
│  ✓ Match  01/05    STARBUCKS 1234        -5.75   Coffee      01/05 Coffee Shop │
│  + New    01/07    NETFLIX              -15.99   (none)      --    │
│▸ ? Review 01/08    SHELL OIL            -42.50   Gas         01/10 Shell │
│  + New    01/09    AMAZON MKTPL         -23.45   Shopping    --    │
│  ─ Skip   01/10    DUPLICATE CHK        -42.50   --          01/10 Shell │
│  ...                                                                │
│                                                                     │
├─────────────────────────────────────────────────────────────────────┤
│ Total: 47  New: 12  Matched: 33  Skipped: 2  │  Enter confirm  Esc cancel │
└─────────────────────────────────────────────────────────────────────┘
```

### Row States

| State | Icon | Description |
|-------|------|-------------|
| Match | `✓` | Auto-matched to an existing transaction; will update existing |
| New | `+` | No match found; will create a new transaction |
| Review | `?` | Low-confidence match; user should verify |
| Skip | `─` | User chose to skip (not import) |

### Keyboard Shortcuts (Import Review)

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate rows |
| `Enter` | Confirm import (commit all non-skipped rows) |
| `Esc` | Cancel import (no changes) |
| `m` | Toggle match/unmatch on selected row |
| `s` | Toggle skip on selected row |
| `e` | Edit selected row (open inline editor for category, payee) |
| `Space` | Cycle state: Match → New → Skip (or New → Skip for unmatched) |
| `c` | Edit category for selected row |
| `a` | Accept all matches |

### Auto-categorization

When importing, TMoney pre-fills categories using existing payee-to-category mappings:

1. Match imported payee name against existing payees (exact match first, then alias/pattern match)
2. If a payee match is found and that payee has a default category, pre-fill it
3. User can override any auto-filled category in the review view

### Fuzzy Matching Algorithm

When duplicate detection is enabled, TMoney matches imported transactions against existing transactions in the target account using a weighted scoring system:

1. **Amount** (highest weight — required): exact amount match is required for a transaction to be considered a potential match
2. **Date closeness** (medium weight): transactions within +/- 7 days score higher the closer they are. Exact date match gets full score.
3. **Payee name similarity** (lower weight): fuzzy string comparison between imported payee and existing payee names

Scoring:
- **High confidence** (auto-match): amount matches exactly, date within 3 days, payee similarity > 70%
- **Low confidence** (review): amount matches, but date > 3 days apart or payee similarity < 70%
- **No match**: amount doesn't match any existing transaction

### Match Actions

When a match is confirmed:
- The **existing transaction is updated**:
  - Status set to `cleared` (if currently `uncleared`)
  - Bank reference ID added (if from OFX and FITID available)
  - Date updated to bank date (if different and user confirms)
- The imported row is **not** created as a new transaction

When a match is unmatched (user rejects the match):
- The imported row is treated as a **new transaction**

### OFX-specific Matching

When importing OFX files, the FITID (Financial Transaction ID) is available:
- If a FITID matches an existing transaction's bank reference ID → automatic high-confidence match
- FITID-based matching **overrides** fuzzy matching

### Import Workflow (CLI)

```bash
# Dry-run (default) — shows what would happen
tmoney --import transactions.csv --account "Checking"

# With format override
tmoney --import bank.txt --account "Checking" --format csv

# Execute the import
tmoney --import transactions.csv --account "Checking" --confirm

# With duplicate detection — skip matches
tmoney --import statement.ofx --account "Checking" --confirm --skip-duplicates

# With duplicate detection — update matches
tmoney --import statement.ofx --account "Checking" --confirm --update-duplicates
```

#### Dry-run Output

```
IMPORT PREVIEW: transactions.csv → Checking
============================================
Parsed: 47 transactions
  New:      12 transactions (will be created)
  Matched:  33 transactions (will be updated)
  Skipped:   2 transactions (duplicates)

Date range: 2024-01-01 to 2024-01-31
Total amount: -$3,456.78

Run with --confirm to execute the import.
```

#### Execution Output

```
IMPORT COMPLETE: transactions.csv → Checking
=============================================
Created:  12 new transactions
Updated:  33 existing transactions
Skipped:   2 duplicates
```

---

## Export

### Export Scope

Full database export includes:
- Accounts (with balances)
- Transactions (with splits and transfer linkages)
- Categories (with hierarchy)
- Payees (with aliases)
- Scheduled transactions

Does **not** include internal metadata (undo history, backup metadata, reconciliation sessions).

### Export Workflow (TUI)

1. File menu → Export
2. **Export dialog**:
   - Format: CSV or QIF (radio buttons)
   - Scope: All accounts / specific account (dropdown)
   - Date range: optional from/to fields
   - Output file: file browser for destination

### Export Workflow (CLI)

```bash
# Export full database as CSV
tmoney --export finances.csv

# Export as QIF
tmoney --export finances.qif --format qif

# Export specific account
tmoney --export checking.csv --account "Checking"

# Export date range
tmoney --export q1.csv --from 2024-01-01 --to 2024-03-31

# Export specific account with date range
tmoney --export checking_jan.csv --account "Checking" --from 2024-01-01 --to 2024-01-31
```

---

## CSV Format

### Standard

- RFC 4180 compliant
- UTF-8 encoding
- Header row with column names
- One row per transaction (or per split for split transactions)

### Columns

| Column | Description | Example |
|--------|-------------|---------|
| `Date` | Transaction date (YYYY-MM-DD) | `2024-01-15` |
| `Account` | Account name | `Checking` |
| `Payee` | Payee name | `Kroger` |
| `Category` | Category path | `Food:Groceries` |
| `Amount` | Transaction amount (negative = expense) | `-125.43` |
| `Memo` | Transaction memo | `Weekly groceries` |
| `Check Number` | Check number if applicable | `1234` |
| `Status` | Transaction status | `C` |
| `Transfer Account` | Counterpart account for transfers | `Savings` |

### Categorized Transfers

A transfer can carry an optional category (see
[`specs/transfer-categories.md`](transfer-categories.md)). The `Category` and
`Transfer Account` columns are independent, so CSV export round-trips both for a
categorized transfer. Import is unchanged: no import path creates transfers, so a
`Transfer Account` value on an imported row is parsed but never used to build a
transfer.

### Split Transactions in CSV

Split transactions are exported as multiple rows:
- First row has the parent transaction fields with category blank
- Subsequent rows have the split category, split amount, and split memo
- Parent fields (date, payee, account) are repeated for each split row

On import, a run of rows is folded back into one split transaction only when it
has the exact shape the exporter writes: a parent row with a blank category,
followed by **two or more** rows with the same date, account and payee that each
carry a category, whose amounts sum to the parent's amount. A run that fails any
of those checks is imported as separate transactions — two uncategorized ATM
withdrawals on one day, or two same-payee purchases where the first lacks a
category, must not be merged into one.

Known limits of this shape-based detection: a one-line split exports as two rows
and imports as two transactions, and an uncategorized purchase followed by two
categorized same-payee purchases that happen to sum to it is folded. Both are
rare; a dedicated split marker column would remove the guesswork.

### Import CSV Parsing

When importing CSV:
- Header row is required (used for column mapping)
- Columns are matched by name (case-insensitive)
- Missing optional columns are allowed
- Unknown columns are ignored

---

## QIF Format

### Structure

```
!Type:Bank
D01/15/2024
T-125.43
PKroger
LFood:Groceries
MWeekly groceries
C*
N1234
^
```

### Field Codes

| Code | Description |
|------|-------------|
| `!Type:` | Account type header |
| `D` | Date |
| `T` | Amount |
| `P` | Payee |
| `L` | Category (or transfer: `[Account Name]`) |
| `M` | Memo |
| `C` | Cleared status, Quicken convention (`*` or `c` = cleared, `X` or `R` = reconciled) |
| `N` | Check number |
| `^` | End of record |

Because `L` is a single field, a **categorized transfer** is lossy in QIF: `L`
holds *either* the category *or* the `[Account]` transfer marker, and the
transfer marker wins — the category is dropped on export. This is expected, not a
bug; use CSV to preserve both.

### Split Transactions in QIF

```
D01/15/2024
T-150.00
PKroger
SFood:Groceries
$-120.00
EFood items
SHousehold:Cleaning
$-30.00
ECleaning supplies
^
```

| Code | Description |
|------|-------------|
| `S` | Split category |
| `$` | Split amount |
| `E` | Split memo |

### QIF Account Types

| TMoney Type | QIF Type |
|-------------|----------|
| `checking` | `Bank` |
| `savings` | `Bank` |
| `credit_card` | `CCard` |
| `investment` | `Invst` |
| `cash` | `Cash` |
| `loan` | `Oth L` |
| `asset` | `Oth A` |

---

## OFX Format (Import Only)

### Parsing

OFX files contain XML-like markup with financial transaction data. TMoney parses:

| OFX Element | Maps To |
|-------------|---------|
| `<DTPOSTED>` | Transaction date |
| `<TRNAMT>` | Amount |
| `<NAME>` or `<MEMO>` | Payee name |
| `<FITID>` | Bank reference ID (used for matching) |
| `<CHECKNUM>` | Check number |
| `<TRNTYPE>` | Transaction type (DEBIT, CREDIT, etc.) |

### FITID Handling

- The FITID (Financial Institution Transaction ID) is stored on matched/created transactions
- Used for reliable duplicate detection on subsequent imports
- Stored as `bank_reference_id` on the transaction record

### Database Schema Addition

Add to transactions table:

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `bank_reference_id` | string | No | External reference ID (e.g., OFX FITID) |

---

## File Browser Dialog (TUI)

A navigable file browser for selecting import files and export destinations.

### Layout

```
┌──────────────────────────────────────────────────┐
│  SELECT FILE                                 [×]  │
├──────────────────────────────────────────────────┤
│                                                    │
│  Path: ~/Downloads/                               │
│                                                    │
│  📁 ..                                             │
│  📁 Documents                                      │
│  📁 Desktop                                        │
│▸ 📄 checking_jan.ofx                    12.4 KB    │
│  📄 savings_feb.csv                      3.2 KB    │
│  📄 statement.qif                        8.1 KB    │
│                                                    │
│  Filter: *.csv, *.qif, *.ofx, *.qfx              │
│                                                    │
├──────────────────────────────────────────────────┤
│           [Cancel]                    [Open]       │
└──────────────────────────────────────────────────┘
```

### Features

- Navigate directories with arrow keys and Enter
- Filter by file extension (based on context — import vs export)
- Show file sizes
- `..` entry to go to parent directory
- Path bar showing current directory
- Type-ahead filtering by filename

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate files |
| `Enter` | Open directory / select file |
| `Backspace` | Go to parent directory |
| `Esc` | Cancel |
| `/` | Type filename filter |

---

## Validation Rules

### Import

1. File must exist and be readable
2. File format must be parseable (valid CSV/QIF/OFX)
3. Target account must exist and be active
4. Transactions must have at minimum: date and amount
5. Dates must be valid
6. Amounts must be valid numbers

### Export

1. Output path must be writable
2. At least one transaction must exist for the given filters
3. Format must be `csv` or `qif`

## Error Handling

- Parse errors: show line number and description of the issue
- Partial imports: if some rows fail, show which ones and why; successfully parsed rows still appear in review
- File access errors: clear message about permissions or missing file
