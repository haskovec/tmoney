# CLI Interface Specification

## Overview

TMoney supports two interface modes:
1. **TUI Mode** (default): Interactive terminal user interface
2. **CLI Mode**: Command-line flags for scripting and automation

This document specifies the CLI mode interface.

## Invocation

```bash
# Launch TUI with default or last-used file
tmoney

# Launch TUI with specific file
tmoney ~/Documents/TMoney/personal.tdb

# CLI commands (non-interactive)
tmoney --list-accounts
tmoney --balance
```

## Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--file <path>` | `-f` | Specify database file |
| `--help` | `-h` | Show help message |
| `--version` | `-v` | Show version information |

## Database File Handling

1. If `--file` specified: use that file
2. If positional argument: use as file path
3. If neither: use last opened file (stored in config)
4. If no last file: prompt to create or open

### File Path Examples

```bash
tmoney personal.tdb                    # Relative path
tmoney ~/Documents/TMoney/personal.tdb # Absolute path
tmoney --file /path/to/file.tdb        # Explicit flag
```

## Database Commands

### Create Database

```bash
tmoney --create ~/Documents/TMoney/finances.tdb
```

Creates a new database file with all tables and default categories initialized.

Options:
- If the path does not have a `.tdb` extension, it will be added automatically
- Parent directories will be created if they don't exist
- Returns an error if the file already exists

Output:
```
Created database: /Users/you/Documents/TMoney/finances.tdb
```

## Account Commands

### List Accounts

```bash
tmoney --list-accounts
```

Output:
```
ACCOUNTS
========
Name                Type          Balance      Currency
----                ----          -------      --------
Chase Checking      checking      $5,234.56    USD
Savings             savings       $12,000.00   USD
Visa Card           credit_card   -$1,234.56   USD
Investment          investment    $45,678.90   USD
```

Options:
- `--type <type>`: Filter by account type
- `--active-only`: Show only active accounts (default)
- `--include-closed`: Include closed accounts

### Account Details

```bash
tmoney --account "Chase Checking"
```

Output:
```
ACCOUNT: Chase Checking
=======================
Type:            checking
Currency:        USD
Institution:     Chase Bank
Account Number:  ****1234
Opening Date:    2020-01-15
Opening Balance: $1,000.00
Current Balance: $5,234.56
Cleared Balance: $5,134.56
Status:          Active
```

### Account Balance

```bash
# All accounts
tmoney --balance

# Specific account
tmoney --balance "Chase Checking"
```

Output (all):
```
BALANCES
========
Chase Checking:     $5,234.56
Savings:           $12,000.00
Visa Card:         -$1,234.56
Investment:        $45,678.90
------------------------
Net Worth:         $61,678.90
```

### Add Account

```bash
tmoney --add-account \
  --name "Chase Checking" \
  --type checking \
  --currency USD \
  --opening-balance 1000.00 \
  --opening-date 2024-01-15 \
  --institution "Chase Bank" \
  --account-number "1234567890" \
  --notes "Primary checking account"
```

Required:
- `--name <name>`: Account name (must be unique)
- `--type <type>`: Account type (checking, savings, credit_card, investment, cash, loan, asset)

Optional:
- `--currency <code>`: Currency code (default: USD)
- `--opening-balance <amount>`: Opening balance (default: 0)
- `--opening-date <date>`: Opening date YYYY-MM-DD (default: today)
- `--institution <name>`: Financial institution name
- `--account-number <number>`: Account number
- `--notes <text>`: Account notes

Type-specific:
- `--credit-limit <amount>`: Credit limit (credit_card accounts only)
- `--interest-rate <rate>`: Interest rate percentage (loan accounts only)

Output:
```
Account created successfully!
  Name:            Chase Checking
  Type:            Checking
  Currency:        USD
  Opening Balance: $1,000.00
  Opening Date:    2024-01-15
  Institution:     Chase Bank
  Account Number:  1234567890
  Notes:           Primary checking account
```

## Transaction Commands

### List Transactions

```bash
tmoney --transactions "Chase Checking"
```

Options:
- `--limit <n>`: Show last N transactions (default: 20)
- `--from <date>`: Start date (YYYY-MM-DD)
- `--to <date>`: End date (YYYY-MM-DD)
- `--status <status>`: Filter by pending/cleared/reconciled

Output:
```
TRANSACTIONS: Chase Checking
============================
Date        Payee              Category            Amount      Balance
----        -----              --------            ------      -------
2024-01-15  Kroger             Food:Groceries      -$125.43    $5,234.56
2024-01-14  Employer Inc       Income:Salary      $2,500.00    $5,359.99
2024-01-12  Amazon             Shopping:General    -$45.99     $2,859.99
...
```

### Add Transaction

```bash
tmoney --add-transaction \
  --account "Chase Checking" \
  --date 2024-01-15 \
  --amount -125.43 \
  --payee "Kroger" \
  --category "Food:Groceries" \
  --memo "Weekly groceries"
```

Required:
- `--account <name>`: Account name
- `--date <date>`: Transaction date
- `--amount <amount>`: Transaction amount

Optional:
- `--payee <name>`: Payee name (creates if not exists)
- `--category <name>`: Category (format: "Parent:Child" or "Parent")
- `--memo <text>`: Transaction memo
- `--check <number>`: Check number
- `--status <status>`: pending (default), cleared

Output:
```
Transaction created: ID abc123
  Account:  Chase Checking
  Date:     2024-01-15
  Amount:   -$125.43
  Payee:    Kroger
  Category: Food:Groceries
```

### Add Transfer

```bash
tmoney --transfer \
  --from "Chase Checking" \
  --to "Savings" \
  --date 2024-01-15 \
  --amount 500.00 \
  --memo "Monthly savings"
```

### Search Transactions

```bash
tmoney --search "amazon"
```

Options:
- `--account <name>`: Limit to specific account
- `--from <date>`: Start date
- `--to <date>`: End date
- `--category <name>`: Filter by category
- `--min <amount>`: Minimum amount
- `--max <amount>`: Maximum amount

Output:
```
SEARCH RESULTS: "amazon"
========================
Account          Date        Payee    Category          Amount
-------          ----        -----    --------          ------
Chase Checking   2024-01-12  Amazon   Shopping:General  -$45.99
Chase Checking   2024-01-05  Amazon   Shopping:General  -$23.45
Visa Card        2023-12-28  Amazon   Shopping:General  -$156.78
```

## Scheduled Transaction Commands

### List Scheduled Transactions

```bash
tmoney --scheduled
```

Options:
- `--due`: Show only due/overdue items
- `--account <name>`: Filter by account

Output:
```
SCHEDULED TRANSACTIONS
======================
ID       Next Date   Payee           Account         Amount      Frequency
--       ---------   -----           -------         ------      ---------
abc123   2024-01-15  Landlord        Checking        -$1,500.00  Monthly
def456   2024-01-20  Electric Co     Checking        ~$120.00    Monthly
ghi789   2024-01-31  Auto Finance    Checking        -$450.00    Monthly (48 left)

[DUE] 1 scheduled transaction is due
```

### Post Scheduled Transaction

```bash
# Post with scheduled amount
tmoney --post-scheduled abc123

# Post with different amount
tmoney --post-scheduled def456 --amount 135.67

# Post with different date
tmoney --post-scheduled abc123 --date 2024-01-16
```

Output:
```
Posted scheduled transaction:
  ID:       abc123
  Account:  Checking
  Date:     2024-01-15
  Amount:   -$1,500.00
  Payee:    Landlord
  Category: Housing:Rent

Next occurrence: 2024-02-15
```

### Skip Scheduled Transaction

```bash
tmoney --skip-scheduled abc123
```

Output:
```
Skipped scheduled transaction abc123
Next occurrence: 2024-02-15
```

## Report Commands

### Net Worth

```bash
tmoney --report net-worth
```

Output:
```
NET WORTH REPORT
================
As of: 2024-01-15

ASSETS
------
Chase Checking:     $5,234.56
Savings:           $12,000.00
Investment:        $45,678.90
------------------------
Total Assets:      $62,913.46

LIABILITIES
-----------
Visa Card:          $1,234.56
------------------------
Total Liabilities:  $1,234.56

NET WORTH:         $61,678.90
```

### Spending by Category

```bash
tmoney --report spending --month 2024-01
tmoney --report spending --year 2024
```

Output:
```
SPENDING BY CATEGORY: January 2024
==================================

Category              Amount      % of Total
--------              ------      ----------
Housing               $1,500.00   45.2%
  Rent                $1,500.00
Food                    $523.45   15.8%
  Groceries             $423.45
  Restaurants           $100.00
Transportation          $345.67   10.4%
  Gas                   $245.67
  Maintenance           $100.00
...
------------------------
Total Spending:       $3,320.12
```

## Error Handling

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | File not found |
| 3 | Invalid arguments |
| 4 | Database error |

### Error Output

Errors are written to stderr:
```
Error: Account "Checking" not found
```

## Date Formats

Accepted date formats:
- `YYYY-MM-DD` (preferred): 2024-01-15
- `MM/DD/YYYY`: 01/15/2024
- `today`: Current date
- `yesterday`: Previous date

## Amount Formats

Accepted amount formats:
- Plain number: `123.45`
- With sign: `-123.45` or `+123.45`
- With currency: `$123.45` or `USD 123.45`
- Negative expense: `-50` (money out)
- Positive income: `50` or `+50` (money in)

## Configuration

Config file location (OS-agnostic, uses `os.UserConfigDir()`):
- macOS: `~/Library/Application Support/tmoney/config.json`
- Linux: `~/.config/tmoney/config.json`
- Windows: `%APPDATA%\tmoney\config.json`

Config options:
```json
{
  "default_file": "~/Documents/TMoney/personal.tdb",
  "recent_files": [
    "~/Documents/TMoney/personal.tdb",
    "~/Documents/TMoney/business.tdb"
  ],
  "last_file": "~/Documents/TMoney/personal.tdb"
}
```
