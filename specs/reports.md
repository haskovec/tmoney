# Reports Specification

## Overview

Reports provide insights into financial data through summaries, charts, and analysis. Version 1 includes two core reports: Net Worth and Spending by Category.

## Net Worth Report

### Purpose

Shows total assets plus (negative) total liabilities to calculate net worth.

### Data Structure

```
Net Worth = Total Assets + Total Liabilities   (signed balances)

Assets:
  - checking accounts (positive balance)
  - savings accounts (positive balance)
  - investment accounts (current value)
  - cash accounts
  - asset accounts

Liabilities (stored negative when owed — see specs/accounts.md):
  - credit card accounts (negative balance = owed)
  - loan accounts (negative balance = owed)
```

### Account Type Classification

| Account Type | Classification | Balance Interpretation |
|--------------|----------------|------------------------|
| checking | Asset | Positive = asset |
| savings | Asset | Positive = asset |
| investment | Asset | Always asset (value) |
| cash | Asset | Positive = asset |
| asset | Asset | Always asset |
| credit_card | Liability | Negative = owed |
| loan | Liability | Negative = owed |

### Display Format (TUI)

```
NET WORTH REPORT
================
As of: January 15, 2024

ASSETS
──────────────────────────────────────
  Chase Checking              $5,234.56
  Savings Account            $12,000.00
  Investment Account         $45,678.90
  ────────────────────────────────────
  Total Assets               $62,913.46

LIABILITIES
──────────────────────────────────────
  Visa Credit Card           -$1,234.56
  ────────────────────────────────────
  Total Liabilities          -$1,234.56

──────────────────────────────────────
NET WORTH                    $61,678.90
```

Amounts under the LIABILITIES heading are the **raw signed** stored
balances: the Visa's stored balance is −$1,234.56 and displays as
−$1,234.56 (a debt, in red). An overpaid account (positive stored
balance) displays positive, correctly reading as a credit.

### Display Format (CLI)

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
Visa Card:         -$1,234.56
------------------------
Total Liabilities: -$1,234.56

NET WORTH:         $61,678.90
```

### Calculations

```
For each account:
  current_balance = opening_balance + sum(transactions)

For investment accounts:
  current_value = sum(lots.quantity * current_price)
  Note: In v1, current_price = purchase_price (no price updates)

Total Assets = sum of asset account balances
               (checking, savings, investment, cash, asset)

Total Liabilities = sum of signed liability balances, ≤ 0 when owed
                    (credit_card, loan)

Net Worth = Total Assets + Total Liabilities

Display: liability rows and the liabilities total render their raw
signed balance under the LIABILITIES heading — a debt shows negative
(in red), an overpaid/credit-balance account shows positive (a credit,
in green). The report struct carries signed values; presentation layers
show them directly.
```

### Options

| Option | Description |
|--------|-------------|
| Include closed | Include closed accounts (default: no) |
| As of date | Calculate as of specific date (default: today) |

## Spending by Category Report

### Purpose

Shows spending breakdown by category for a given time period (month or year).

### Data Structure

```
Period: January 2024

Categories (expense type only):
  Housing:           $1,500.00  (45.2%)
    Rent:            $1,500.00
  Food:                $523.45  (15.8%)
    Groceries:         $423.45
    Restaurants:       $100.00
  Transportation:      $345.67  (10.4%)
    ...
  ─────────────────────────────
  Total Spending:    $3,320.12
```

### What Counts as Spending

1. Transactions with categories of type `expense`
2. Negative amounts (money going out)
3. Refunds/credits reduce the category total (not added as income)
4. Transfers are excluded by default — explicit guards
   (`t.transfer_id IS NULL` on transactions, `ts.transfer_account_id IS NULL`
   on splits) keep them out; the opt-in `--include-transfers` folds
   categorized transfers in (see *Including transfers (opt-in)*)

### Display Format (TUI)

```
SPENDING BY CATEGORY                        January 2024
═══════════════════════════════════════════════════════════

Category              Amount        % of Total    Bar
────────────────────────────────────────────────────────────
Housing               $1,500.00     45.2%    ████████████████
  Rent                $1,500.00
Food                    $523.45     15.8%    █████░░░░░░░░░░░
  Groceries             $423.45
  Restaurants           $100.00
Transportation          $345.67     10.4%    ████░░░░░░░░░░░░
  Gas                   $245.67
  Maintenance           $100.00
Shopping                $245.99      7.4%    ███░░░░░░░░░░░░░
  Electronics            $45.99
  Clothing              $200.00
Utilities               $156.78      4.7%    ██░░░░░░░░░░░░░░
  Electric              $100.00
  Internet               $56.78
Entertainment            $89.50      2.7%    █░░░░░░░░░░░░░░░
Other                    $45.00      1.4%    █░░░░░░░░░░░░░░░
────────────────────────────────────────────────────────────
TOTAL                 $2,906.39    100.0%

◀ Dec 2023              Jan 2024              Feb 2024 ▶
```

### Display Format (CLI)

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
Shopping                $245.99    7.4%
Utilities               $156.78    4.7%
Entertainment            $89.50    2.7%
Other                    $45.00    1.4%
------------------------
Total Spending:       $2,906.39
```

### Time Periods

| Period | Date Range |
|--------|------------|
| This Month | 1st of current month to today |
| Last Month | Full previous month |
| This Year | Jan 1 of current year to today |
| Last Year | Full previous year |
| Custom Month | Any month (YYYY-MM) |
| Custom Year | Any year (YYYY) |
| Custom Range | From date to date |

### Calculations

```sql
-- For a given month
SELECT
    c.id,
    c.name,
    COALESCE(parent.name, c.name) AS parent_name,
    c.parent_id,
    ABS(SUM(t.amount)) AS spending
FROM transactions t
JOIN categories c ON t.category_id = c.id
LEFT JOIN categories parent ON c.parent_id = parent.id
WHERE c.type = 'expense'
  AND c.system_category = FALSE  -- Exclude Transfer
  AND t.transfer_id IS NULL  -- Exclude transfers (dropped by --include-transfers)
  AND t.date >= '2024-01-01'
  AND t.date < '2024-02-01'
  AND t.amount < 0  -- Only outflows
GROUP BY c.id, c.name, parent.name, c.parent_id
ORDER BY spending DESC;
```

### Handling Split Transactions

For split transactions, each split's amount is attributed to its category:

```sql
-- Include splits
SELECT
    c.id,
    c.name,
    ABS(SUM(ts.amount)) AS spending
FROM transaction_splits ts
JOIN categories c ON ts.category_id = c.id
JOIN transactions t ON ts.transaction_id = t.id
WHERE c.type = 'expense'
  AND ts.transfer_account_id IS NULL  -- Exclude transfer lines (dropped by --include-transfers)
  AND t.date >= '2024-01-01'
  AND t.date < '2024-02-01'
  AND ts.amount < 0
GROUP BY c.id, c.name;
```

### Including transfers (opt-in)

By default the report excludes every transfer, categorized or not, via the
explicit guards above (`t.transfer_id IS NULL` / `ts.transfer_account_id IS
NULL`). These replaced an earlier implicit NULL-join that let categorized
transfer pairs — e.g. ones produced by `transfer link` — leak in as
spending. The opt-in toggle drops both guards and folds **categorized**
transfers into the totals:

- **CLI**: `tmoney --report spending --month 2024-01 --include-transfers`
  (also works with `--year` and `--from/--to`).
- **TUI**: `t` on the Reports view toggles it for the current spending
  report. The toggle is session-only and is not persisted.

Even with the toggle on, only the **outflow** leg of a transfer counts (the
`amount < 0` filter prevents double-counting within a mirrored pair), and
only **expense-typed** categories appear. Uncategorized transfers stay
invisible regardless of the toggle — the category join still drops them. The
Net Worth report is category-blind and is unaffected. See
[`specs/transfer-categories.md`](transfer-categories.md) for the full feature.

### Options

| Option | Description |
|--------|-------------|
| Period | Month or year |
| Show subcategories | Include subcategory breakdown (default: yes) |
| Accounts | All or specific accounts |
| Minimum | Hide categories below threshold |

### Drill-Down (TUI)

In the TUI, selecting a category shows:
1. List of transactions in that category
2. Subcategory breakdown (if parent)
3. Trend over time

## Report Data Models

### NetWorthReport

```go
type NetWorthReport struct {
    AsOfDate     time.Time
    Assets       []AccountBalance
    Liabilities  []AccountBalance
    TotalAssets  decimal.Decimal
    TotalLiabilities decimal.Decimal
    NetWorth     decimal.Decimal
}

type AccountBalance struct {
    AccountID   uuid.UUID
    Name        string
    Type        string
    Balance     decimal.Decimal
}
```

### SpendingReport

```go
type SpendingReport struct {
    Period       string  // "January 2024" or "2024"
    StartDate    time.Time
    EndDate      time.Time
    Categories   []CategorySpending
    TotalSpending decimal.Decimal
}

type CategorySpending struct {
    CategoryID   uuid.UUID
    Name         string
    ParentID     *uuid.UUID
    ParentName   string
    Amount       decimal.Decimal
    Percentage   float64
    Subcategories []CategorySpending
}
```

## CLI Commands

```bash
# Net worth report
tmoney --report net-worth
tmoney --report net-worth --as-of 2024-01-01

# Spending report
tmoney --report spending --month 2024-01
tmoney --report spending --year 2024
tmoney --report spending --from 2024-01-01 --to 2024-06-30

# Fold categorized transfers into the spending report (default: excluded)
tmoney --report spending --month 2024-01 --include-transfers
```

## v1.5 Features (Not in v1)

- Income report (parallel to spending)
- Cash flow report (income vs spending over time)
- Category trends over time
- Year-over-year comparison
- Export reports to PDF/CSV
- Custom report date ranges
- Report scheduling/automation
- Charts and visualizations
- Budget vs actual (requires budget feature)
