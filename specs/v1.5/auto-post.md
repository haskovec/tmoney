# Auto-post Scheduled Transactions Specification (v1.5)

## Overview

Auto-post allows scheduled transactions to be automatically posted (creating real transactions) when TMoney starts up, without requiring user intervention. Each scheduled transaction can be individually configured as auto-post or manual.

## Scheduled Transaction Changes

### New Properties

| Property | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| `auto_post` | boolean | Yes | `false` | Whether to automatically post when due |
| `post_lead_days` | integer | No | `0` | Days before due date to post |

### Lead Time Options

| Value | Description |
|-------|-------------|
| `0` | Post on the due date |
| `3` | Post 3 days before the due date |
| `7` | Post 7 days (one week) before the due date |

Lead time is useful for forecasting — it lets users see upcoming transactions in their register before they actually occur.

## Auto-post Behavior

### Trigger

Auto-posting runs on **app startup** — whenever a database file is opened (both TUI and CLI).

### Algorithm

```
On file open:
  For each scheduled transaction WHERE auto_post = true:
    While next_date - post_lead_days <= today:
      1. Create a real transaction from the scheduled transaction
         - Date: next_date (the actual scheduled date, not today)
         - Amount: scheduled amount (or estimate if variable)
         - Status: uncleared
         - All other fields from the scheduled transaction
      2. Advance next_date based on frequency
      3. Decrement occurrences_remaining (if applicable)
      4. If occurrences_remaining = 0, mark schedule as completed
```

### Multiple Overdue Occurrences

If the app hasn't been opened for a while, multiple occurrences may be due. **All overdue occurrences are posted**, each with their correct scheduled date.

Example: Monthly rent ($1,500) with `post_lead_days = 0`, app not opened for 3 months:
- Posts transaction dated Feb 1 for $1,500
- Posts transaction dated Mar 1 for $1,500
- Posts transaction dated Apr 1 for $1,500
- Advances next_date to May 1

### Variable Amounts

For scheduled transactions with variable amounts (`amount` is null):
- Auto-post uses the **estimated amount** (average of last N transactions with same payee)
- If no estimate is available, the scheduled transaction is **skipped** with a notification
- User can post these manually from the scheduled transactions view

### No Visual Distinction

Auto-posted transactions look identical to manually posted transactions in the register. There is no special indicator — they are regular transactions.

## TUI Integration

### Startup Notification

When auto-posting occurs on file open:
- Status bar shows a brief notification: `"Auto-posted N scheduled transactions"`
- Notification fades after a few seconds or on next action

### Scheduled Transaction Dialogs

The new/edit scheduled transaction dialog adds:

```
┌──────────────────────────────────────────────────────┐
│  EDIT SCHEDULED TRANSACTION                      [×]  │
├──────────────────────────────────────────────────────┤
│                                                        │
│  ... (existing fields) ...                            │
│                                                        │
│  Auto-post:   [✓] Automatically post when due         │
│                                                        │
│  Lead time:   (•) On the day                          │
│               ( ) 3 days early                         │
│               ( ) 1 week early                         │
│                                                        │
├──────────────────────────────────────────────────────┤
│           [Cancel]                    [Save]           │
└──────────────────────────────────────────────────────┘
```

- Lead time radio buttons are only shown/enabled when auto-post is checked
- Default: auto-post off, lead time = on the day

### Scheduled Transactions View

The scheduled transactions list shows an auto-post indicator:

```
DUE NOW
──────────────────────────────────────────────
▸ 01/15  Landlord      Rent    -$1,500  [Auto]

UPCOMING
──────────────────────────────────────────────
  01/20  Electric Co   Utils     ~$120  [Auto 3d]
  01/31  Auto Finance  Car      -$450
  02/01  Landlord      Rent   -$1,500   [Auto]
```

- `[Auto]` = auto-post with 0 lead days
- `[Auto 3d]` = auto-post with 3-day lead
- `[Auto 7d]` = auto-post with 7-day lead
- No indicator = manual post

## CLI Integration

### Auto-post on Startup

Auto-posting also runs when any CLI command opens a database file. A status line is printed:

```
Auto-posted 2 scheduled transactions
```

If nothing was auto-posted, no message is shown.

### New CLI Flags

For `--add-scheduled` and (future) `--edit-scheduled`:

| Flag | Description |
|------|-------------|
| `--auto-post` | Enable auto-posting for this scheduled transaction |
| `--lead-days <0\|3\|7>` | Post N days before due date (requires --auto-post) |

### Examples

```bash
# Create auto-posting rent payment
tmoney --add-scheduled --account "Checking" \
  --payee "Landlord" --category "Housing:Rent" \
  --amount -1500 --frequency monthly --day 1 \
  --auto-post --lead-days 0

# Create auto-posting with 3-day lead
tmoney --add-scheduled --account "Checking" \
  --payee "Car Insurance" --category "Transportation:Insurance" \
  --amount -150 --frequency monthly --day 15 \
  --auto-post --lead-days 3
```

## Database Schema Changes

Add to `scheduled_transactions` table:

```sql
ALTER TABLE scheduled_transactions ADD COLUMN auto_post BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE scheduled_transactions ADD COLUMN post_lead_days INTEGER NOT NULL DEFAULT 0;
```

## Validation Rules

1. `auto_post` defaults to `false`
2. `post_lead_days` must be 0, 3, or 7
3. `post_lead_days` is only meaningful when `auto_post = true`
4. Variable-amount scheduled transactions with no estimate available are skipped during auto-post
5. Auto-post creates transactions with status `uncleared`
