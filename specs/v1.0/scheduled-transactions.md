# Scheduled Transactions Specification

## Overview

Scheduled transactions represent recurring or future transactions that repeat on a defined schedule. They generate reminders for the user to enter the actual transaction.

## Scheduled Transaction Properties

### Core Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | UUID | Yes | Unique identifier |
| `account_id` | UUID | Yes | Account for the transaction |
| `payee_id` | UUID | No | Payee reference |
| `category_id` | UUID | No | Default category |
| `amount` | decimal | No | Scheduled amount (null if variable) |
| `memo` | string | No | Transaction memo |
| `created_at` | timestamp | Yes | When record was created |
| `updated_at` | timestamp | Yes | When record was last updated |

### Schedule Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `frequency` | enum | Yes | How often it repeats |
| `interval` | integer | Yes | Every N periods (default: 1) |
| `start_date` | date | Yes | When schedule begins |
| `end_date` | date | No | When schedule ends (null = indefinite) |
| `occurrences` | integer | No | Number of times to repeat (null = indefinite) |
| `day_of_month` | integer | No | Specific day (1-31, or -1 for last day) |
| `day_of_week` | integer | No | Day of week (0=Sunday, 6=Saturday) |
| `next_date` | date | Yes | Next occurrence date |
| `occurrences_remaining` | integer | No | Countdown for fixed occurrences |

## Frequency Types

| Frequency | Description | Uses |
|-----------|-------------|------|
| `daily` | Every N days | interval |
| `weekly` | Every N weeks | interval, day_of_week |
| `biweekly` | Every 2 weeks | day_of_week |
| `monthly` | Every N months | interval, day_of_month |
| `quarterly` | Every 3 months | day_of_month |
| `yearly` | Every N years | interval, day_of_month, month |

### Special Day Handling

| day_of_month | Meaning |
|--------------|---------|
| 1-28 | That day of month |
| 29-31 | That day, or last day if month is shorter |
| -1 | Last day of month |

## Duration Options

A scheduled transaction ends when:

1. **Indefinite**: `end_date = null` AND `occurrences = null`
   - Runs forever until manually deleted
   - Example: Monthly utility bill

2. **End Date**: `end_date` is set
   - Stops after that date
   - Example: Lease payment ending Dec 2025

3. **Fixed Occurrences**: `occurrences` is set
   - Stops after N occurrences
   - `occurrences_remaining` counts down
   - Example: 60-month car loan

## Variable Amounts

For transactions with variable amounts (utility bills, etc.):

1. `amount` is set to null or stores an estimate
2. `amount_estimate_count` (optional): Use average of last N transactions
3. When posting, user enters actual amount

### Estimate Calculation

If `amount_estimate_count` is set (e.g., 3):
1. Find last 3 posted transactions with same payee
2. Calculate average
3. Pre-fill amount field with estimate
4. User can accept or modify

## Transaction Mode

| Mode | Description | v1 Support |
|------|-------------|------------|
| `remind` | Show reminder, user manually posts | Yes |
| `auto_post` | Automatically create transaction | v1.5 |

For v1, all scheduled transactions use `remind` mode.

## Reminder Behavior

When `next_date` is reached (or passed):

1. Show in "Scheduled Transactions Due" list
2. User can:
   - Post: Creates actual transaction, advances schedule
   - Skip: Advances schedule without creating transaction
   - Edit & Post: Modify amount/date/category then post
   - Postpone: Move next_date forward

### Posting a Scheduled Transaction

1. Create new transaction with:
   - Date: next_date (or user-specified)
   - Amount: scheduled amount or user-entered
   - Other fields from scheduled transaction
2. Advance next_date based on frequency
3. Decrement occurrences_remaining (if applicable)
4. If occurrences_remaining = 0, mark schedule as completed

## Validation Rules

1. `start_date` is required
2. `end_date` must be after `start_date` (if set)
3. `occurrences` must be positive (if set)
4. `day_of_month` must be 1-31 or -1
5. `day_of_week` must be 0-6
6. Cannot have both `end_date` and `occurrences`

## Operations

### Create Scheduled Transaction

Required: account_id, frequency, start_date
Optional: payee_id, category_id, amount, memo, end_date, occurrences, etc.

### Edit Scheduled Transaction

All properties can be modified. Changes affect future occurrences only.

### Delete Scheduled Transaction

Removes the schedule. Posted transactions are unaffected.

### Skip Occurrence

Advances to next occurrence without creating a transaction.

### Post Occurrence

Creates a real transaction and advances the schedule.

## Schedule Calculation

### Next Date Calculation

```
Given: current next_date, frequency, interval

daily:     next_date + (interval days)
weekly:    next_date + (interval * 7 days)
biweekly:  next_date + 14 days
monthly:   next_date + (interval months), adjusted for day_of_month
quarterly: next_date + 3 months, adjusted for day_of_month
yearly:    next_date + (interval years)
```

### Month-End Handling

If day_of_month > days in target month:
- Use last day of month
- Example: 31st monthly → Jan 31, Feb 28/29, Mar 31, Apr 30...

## Examples

### Monthly Rent (Indefinite)

```
frequency: monthly
interval: 1
day_of_month: 1
start_date: 2024-01-01
end_date: null
occurrences: null
amount: -1500.00
payee: "Landlord"
category: "Housing:Rent"
```

### Car Loan (60 Payments)

```
frequency: monthly
interval: 1
day_of_month: 15
start_date: 2024-01-15
occurrences: 60
amount: -450.00
payee: "Auto Finance Co"
category: "Transportation:Auto Loan"
```

### Variable Utility Bill

```
frequency: monthly
interval: 1
day_of_month: -1  # Last day of month
start_date: 2024-01-31
amount: null
amount_estimate_count: 3  # Average of last 3
payee: "Electric Company"
category: "Housing:Utilities"
```

### Biweekly Paycheck

```
frequency: biweekly
day_of_week: 5  # Friday
start_date: 2024-01-05
amount: 2500.00
payee: "Employer"
category: "Income:Salary"
```

## CLI Commands

```bash
# List scheduled transactions
tmoney --scheduled

# List due scheduled transactions
tmoney --scheduled --due

# Post a scheduled transaction
tmoney --post-scheduled <id> [--amount <amount>] [--date <date>]

# Skip a scheduled transaction
tmoney --skip-scheduled <id>
```

## v1.5 Features (Not in v1)

- Auto-post mode
- Email/notification reminders
- Linked to actual posted transactions
- Bulk operations (post all due)
