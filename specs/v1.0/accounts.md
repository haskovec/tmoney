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
| `active` | boolean | Yes | Whether account is active (default: true) |
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

All properties except `id` and `created_at` can be modified.

Changing `opening_balance` or `opening_date` affects the calculated current balance.

### Close Account

Set `active = false`. Closed accounts:
- Remain visible in account list (can be hidden via filter)
- Cannot receive new transactions
- Maintain historical data

### Delete Account

Only allowed if account has no transactions. Otherwise, close it instead.

## v1.5 Features (Not in v1)

- Currency conversion for display
- Automatic price updates for investments
- Account groups/folders
