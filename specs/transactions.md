# Transactions Specification

## Overview

Transactions represent the movement of money into, out of, or between accounts. They are the core data of the application and drive all balance calculations and reports.

## Transaction Properties

### Core Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | UUID | Yes | Unique identifier |
| `account_id` | UUID | Yes | Account this transaction belongs to |
| `date` | date | Yes | Transaction date |
| `amount` | decimal | Yes | Transaction amount (see sign convention) |
| `payee_id` | UUID | No | Reference to payee |
| `memo` | string | No | User notes/description |
| `check_number` | string | No | Check number if applicable |
| `status` | enum | Yes | cleared, pending, reconciled |
| `created_at` | timestamp | Yes | When record was created |
| `updated_at` | timestamp | Yes | When record was last updated |

### Transfer Properties

| Property | Type | Description |
|----------|------|-------------|
| `transfer_id` | UUID | Links two transactions as a transfer pair |
| `transfer_account_id` | UUID | The other account in the transfer |

## Amount Sign Convention

| Transaction Type | Amount Sign | Example |
|------------------|-------------|---------|
| Income/Deposit | Positive | +1000.00 (paycheck) |
| Expense/Withdrawal | Negative | -50.00 (grocery purchase) |
| Transfer Out | Negative | -500.00 (to savings) |
| Transfer In | Positive | +500.00 (from checking) |

For credit cards and loans (liability accounts), the signs are inverted for display but stored consistently.

## Transaction Status

| Status | Description |
|--------|-------------|
| `pending` | Entered but not yet cleared at bank |
| `cleared` | Confirmed cleared at bank |
| `reconciled` | Matched during reconciliation (v1.5) |

## Split Transactions

A transaction can be split across multiple categories. When split:

- The parent transaction has the total amount
- Split items define how the amount is categorized
- Sum of split amounts must equal parent amount

### Split Item Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | UUID | Yes | Unique identifier |
| `transaction_id` | UUID | Yes | Parent transaction |
| `category_id` | UUID | Yes | Category for this portion |
| `amount` | decimal | Yes | Amount for this split |
| `memo` | string | No | Note for this split item |

### Split Transaction Example

Grocery store purchase of $150:
- Total: -$150.00 (Payee: Kroger)
  - Split 1: -$120.00 (Category: Food → Groceries)
  - Split 2: -$30.00 (Category: Household → Cleaning)

## Transfers

Transfers are represented as two linked transactions:

### Transfer Example: $500 from Checking to Savings

**Transaction 1 (in Checking account):**
- amount: -500.00
- category: Transfer → Savings Account
- transfer_id: <shared-uuid>
- transfer_account_id: <savings-account-id>

**Transaction 2 (in Savings account):**
- amount: +500.00
- category: Transfer → Checking Account
- transfer_id: <shared-uuid>
- transfer_account_id: <checking-account-id>

### Transfer Rules

1. Both transactions share the same `transfer_id`
2. Amounts are equal but opposite signs
3. Deleting one side prompts to delete the other
4. Editing amount on one side updates the other
5. Category is always "Transfer" with subcategory being the other account name

## Validation Rules

1. `date` is required
2. `amount` cannot be zero
3. `account_id` must reference an active account
4. For splits: sum of split amounts must equal transaction amount
5. For transfers: both sides must exist and balance

## Operations

### Create Transaction

Required: account_id, date, amount
Optional: payee_id, category_id, memo, check_number, status

When payee is selected, auto-populate category from payee's default.

### Edit Transaction

All properties except `id` and `created_at` can be modified.

For transfers, editing amount updates both sides.

### Delete Transaction

- Regular transaction: delete immediately
- Transfer: prompt to delete both sides
- Reconciled transaction: warn user (v1.5)

### Duplicate Transaction

Create a copy with today's date, status = pending.

## Search

Transactions are searchable by:
- Payee name (partial match)
- Memo (partial match)
- Category name (partial match)
- Date range
- Amount (exact or range - TBD)

## Investment Transactions

For investment accounts, additional transaction types:

| Type | Description |
|------|-------------|
| `buy` | Purchase securities (creates lot) |
| `sell` | Sell securities (reduces lot) |
| `dividend` | Dividend income |
| `transfer_in` | Securities transferred in |
| `transfer_out` | Securities transferred out |

### Buy Transaction Additional Fields

| Property | Type | Description |
|----------|------|-------------|
| `symbol` | string | Security symbol |
| `quantity` | decimal | Number of shares |
| `price` | decimal | Price per share |
| `commission` | decimal | Trading commission |

## v1.5 Features (Not in v1)

- Reconciliation status and workflow
- Attachments (receipts, documents)
- Recurring transaction link
- Tags (in addition to categories)
