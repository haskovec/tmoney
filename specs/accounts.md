# Accounts Specification

## Overview

Accounts represent financial accounts that hold money or track balances. Each account belongs to a specific type and maintains a running balance based on its transactions.

## Account Types

| Type | Description | Special Properties |
|------|-------------|-------------------|
| `checking` | Bank checking account | - |
| `savings` | Bank savings account | - |
| `credit_card` | Credit card account | Credit limit |
| `investment` | Investment/brokerage account | Holdings (lot-level) |
| `cash` | Physical cash tracking | - |
| `loan` | Loans and mortgages | Interest rate |
| `asset` | Other assets (property, vehicles) | - |

## Account Properties

### Core Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | UUID | Yes | Unique identifier |
| `name` | string | Yes | Display name (e.g., "Chase Checking") |
| `type` | enum | Yes | One of the account types above |
| `currency` | string | Yes | ISO 4217 currency code (e.g., "USD") |
| `opening_balance` | decimal | Yes | Starting balance when account was created |
| `opening_date` | date | Yes | Date the account was opened/added |
| `active` | boolean | Yes | Whether account is active (default: true). Source of truth for whether an account is closed |
| `closed_date` | date | No | Date the account was closed. Set in lockstep with `active` (populated on close, cleared on reopen). NULL on a closed account means the close date is unknown (e.g. an account closed before this column existed) |
| `created_at` | timestamp | Yes | When record was created |
| `updated_at` | timestamp | Yes | When record was last updated |

### Optional Properties

| Property | Type | Applies To | Description |
|----------|------|------------|-------------|
| `institution` | string | All | Bank/brokerage name |
| `account_number` | string | All | Full account number (alphanumeric, max 50 chars) |
| `notes` | string | All | User notes |
| `credit_limit` | decimal | credit_card | Maximum credit limit |
| `interest_rate` | decimal | loan | Annual interest rate (percentage) |

## Investment Holdings

Investment accounts track securities at the lot level. Each purchase of a security creates a new lot.

### Lot Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | UUID | Yes | Unique identifier |
| `account_id` | UUID | Yes | Parent investment account |
| `symbol` | string | Yes | Security symbol (e.g., "AAPL") |
| `name` | string | Yes | Security name (e.g., "Apple Inc.") |
| `quantity` | decimal | Yes | Number of shares/units |
| `purchase_price` | decimal | Yes | Price per share at purchase |
| `purchase_date` | date | Yes | Date of purchase |
| `created_at` | timestamp | Yes | When record was created |

### Lot Behavior

- Each buy transaction creates a new lot
- Sell transactions reduce lots (FIFO, LIFO, or specific lot - TBD for v1)
- Current value = quantity × current price (price updates are v1.5+)

## Calculated Fields

These are computed, not stored:

| Field | Calculation |
|-------|-------------|
| `current_balance` | opening_balance + sum(transactions) |
| `cleared_balance` | opening_balance + sum(cleared transactions) |

## Account Balance Calculation

For different account types, positive/negative balances mean:

| Type | Positive Balance | Negative Balance |
|------|------------------|------------------|
| checking, savings, cash, investment, asset | Money you have | Overdrawn |
| credit_card | You owe money | Credit/overpayment |
| loan | You owe money | Overpayment |

## Validation Rules

1. `name` must be unique within the file
2. `name` cannot be empty
3. `currency` must be a valid ISO 4217 code
4. `credit_limit` must be positive (if provided)
5. `interest_rate` must be between 0 and 100 (if provided)
6. `opening_date` cannot be in the future

## Operations

### Create Account

Required: name, type, currency, opening_balance, opening_date

### Edit Account

All properties except `id` and `created_at` can be modified **while the account
is active**. When an account is **closed**, only metadata (`name`,
`institution`, `account_number`, `notes`) may be edited; `opening_balance` and
`opening_date` are locked, since changing them would silently move the balance
of a frozen account off zero. Reopen the account to change those.

Changing `opening_balance` or `opening_date` affects the calculated current balance.

### Close Account

Closing sets `active = false` and records `closed_date`. The close date defaults
to today and may be back-dated; it must satisfy
`max(opening_date, latest transaction date) ≤ closed_date ≤ today`, and the
account must have a **zero balance**. `closed_date` is cleared on reopen, in
lockstep with `active`.

Closed accounts:
- Remain visible in the account list (`account list --include-closed`) and in a
  dimmed **Closed Accounts** section at the bottom of the TUI sidebar, annotated
  with the close date where known.
- Are **frozen**: no new transactions, edits, deletes, cleared-status toggles,
  reconciliation, or transfers. A transfer is blocked if **either** leg is
  closed. The freeze is enforced at the service layer, so it holds across the
  TUI, the CLI, scheduled posting, and imports. Reopen to make changes.
- No longer appear in any account picker (transfer, scheduled transfer,
  investment share transfer, import, paycheck), and a schedule cannot be created
  against — or post into — a closed account.
- Maintain historical data, and remain **viewable** (read-only register /
  portfolio valuation, net-worth `--include-closed`).

Read and maintenance paths (valuation, position rebuild, lot backfill, the
startup self-heal) continue to operate on closed accounts unchanged.

**Scheduled transactions referencing a closed account.** New schedules can't
target a closed account. An existing schedule whose source, transfer
destination, or transfer-line split target becomes closed is handled as: a soft
warning at close time (listing the affected schedules; the close still
proceeds), a hard refusal on manual post (the schedule stays due), and a skip on
auto-post (the batch is not aborted).

### Reopen Account

Set `active = true` and clear `closed_date`. Reopening is instant and undoable;
undo re-closes to the exact prior close date (including an unknown/NULL date).
Available via the Accounts menu (**Reopen Account**) in the TUI and
`tmoney account reopen <name>` on the CLI.

### Delete Account

Only allowed if account has no transactions. Otherwise, close it instead.

## v1.5 Features (Not in v1)

- Currency conversion for display
- Automatic price updates for investments
- Account groups/folders
