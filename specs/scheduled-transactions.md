# Scheduled Transactions Specification

## Overview

Scheduled transactions represent recurring or future transactions that repeat on a defined schedule. They generate reminders for the user to enter the actual transaction.

A scheduled transaction may be **single-line** (one account, one payee, one category or transfer, one amount — described throughout this spec) or **multi-line** (a template with multiple categorized and/or transfer lines, used for paychecks and other compound events). The multi-line model, post-time preview dialog, and paycheck wizard are specified in [`specs/multiline-splits-and-paycheck.md`](multiline-splits-and-paycheck.md); the relevant cross-references are summarized in the **Multi-Line Schedules** and **Post-Time Preview Dialog** sections below. Transfers (single-line schedules and multi-line transfer lines) may also carry an optional category — see [`specs/transfer-categories.md`](transfer-categories.md).

## Scheduled Transaction Properties

### Core Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | UUID | Yes | Unique identifier |
| `account_id` | UUID | Yes | Account for the transaction (the "From" account on a transfer schedule) |
| `payee_id` | UUID | No | Payee reference |
| `category_id` | UUID | No | Default category (NULL on multi-line schedules; optional on single-line transfer schedules, where it is mirrored onto both posted legs) |
| `transfer_account_id` | UUID | No | Destination ("To") account when this is a single-line transfer schedule. Mutually exclusive with split children; may coexist with an optional `category_id` (a categorized transfer). |
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
| `secondary_day_of_month` | integer | No | Second pay day for semimonthly (1-31, or -1 for last day) |
| `next_date` | date | Yes | Next occurrence date |
| `occurrences_remaining` | integer | No | Countdown for fixed occurrences |

## Frequency Types

| Frequency | Description | Uses |
|-----------|-------------|------|
| `daily` | Every N days | interval |
| `weekly` | Every N weeks | interval |
| `fortnightly` | Every N fortnights | interval |
| `semimonthly` | Twice a month on two pay days | day_of_month, secondary_day_of_month |
| `monthly` | Every N months | interval, day_of_month |
| `quarterly` | Every N quarters | interval, day_of_month |
| `yearly` | Every N years | interval, day_of_month |

### Special Day Handling

| day_of_month | Meaning |
|--------------|---------|
| 1-28 | That day of month |
| 29-31 | That day, or last day if month is shorter |
| -1 | Last day of month |
| (unset) | The start date's day, with the same month-end clamping |

A month-based cadence (monthly, quarterly, yearly) with no `day_of_month`
anchors to the start date's day, so a clamped month is a one-off rather than a
permanent shift: a schedule started on Jan 31 runs Jan 31, Feb 28/29, Mar 31.

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

For **single-line** scheduled transactions with variable amounts (utility bills, etc.):

1. `amount` is set to null or stores an estimate
2. `amount_estimate_count` (optional, single-line only): Use average of last N posted transactions
3. When posting, user enters actual amount

### Estimate Calculation

If `amount_estimate_count` is set (e.g., 3):
1. Find last 3 posted transactions with same payee
2. Calculate average
3. Pre-fill amount field with estimate
4. User can accept or modify

Multi-line schedules do **not** support per-line variable estimation, and the parent `amount_estimate_count` field is not consulted on multi-line schedules either — it is exclusively a single-line affordance. Each line on a multi-line template stores a fixed expected amount; real-world variation (FICA penny shifts, payday holiday shifts) is handled at the post-time preview where the user edits the one occurrence. See **Post-Time Preview Dialog** below.

## Posting Mode

| Mode | Description |
|------|-------------|
| `remind` | Show reminder, user manually posts via the preview dialog |
| `auto_post` | Service-layer auto-poster fires on file open and posts due occurrences using template values exactly (no preview) |

Auto-post is configured per-schedule via the `auto_post` flag and the `post_lead_days` field (accepted values: `0`, `3`, `7` — days before the due date to fire). When `auto_post` is off, the schedule uses `remind` mode and Enter on a due item opens the preview dialog. Both modes coexist in the same database; the user picks per-schedule.

## Reminder Behavior

When `next_date` is reached (or passed):

1. Show in "Scheduled Transactions Due" list
2. User can:
   - Enter / Post: Opens the **post-time preview dialog** (see below), pre-filled with template values, where the user can adjust this one occurrence before saving
   - Skip: Advances schedule without creating transaction
   - Edit Series (`e`): Modify the template itself, affecting all future occurrences. On a paycheck-shaped multi-line template (a positive categorized income line plus at least one negative `Tax > ...` line), the edit dialog exposes an extra **Edit as paycheck →** button that closes the generic dialog and reopens the schedule in the paycheck wizard with values pre-filled — a convenience for round-tripping schedules originally created via the wizard. See `specs/multiline-splits-and-paycheck.md` for the heuristic.

### Posting a Scheduled Transaction

1. Open the preview dialog pre-filled with template values
2. User edits this one occurrence as needed (date, amounts, lines)
3. On Save: create transaction(s) per the template structure (single-line or multi-line; transfer-lines auto-create paired counterparts)
4. Advance `next_date` based on the template's original `next_date` and frequency — **not** based on the user's edited posting date
5. Decrement `occurrences_remaining` (if applicable); mark schedule completed when it hits 0

### Auto-Post Interaction

Auto-post bypasses the preview entirely and creates the transaction(s) using template values exactly. Users who want preview-before-post on a given schedule leave auto-post off. For paychecks, most users leave auto-post off precisely because of FICA-penny fluctuations.

## Single-Line Transfers

A scheduled transaction may be a **single-line transfer**: `transfer_account_id`
is set (the destination), `account_id` is the source, `category_id` is optional
(see below), and there are no split children. This models a recurring transfer between two
**regular** accounts — a monthly credit-card payment from Checking, a savings
sweep, a loan payment.

- Created in the TUI via `t` on the Scheduled view (mirrors the register's
  `t`), which opens a From / To / Amount / Memo dialog plus the recurrence
  fields. The From/To pickers exclude investment-type accounts.
- The stored `amount` is the signed effect on the source account (negative).
  The dialog shows a positive magnitude; the list shows the negative amount.
- **Amount is required** (an estimate). There is no variable/null transfer
  amount — the post-time preview pre-fills the estimate and the user edits the
  real figure for that one occurrence (one-off; the template keeps the estimate).
- Posting (preview Enter, or auto-post) creates a **clean linked transfer pair**
  via the transaction service's transfer path — identical to an ad-hoc
  transfer, not a one-line split. For bank↔bank both legs are regular rows.
- **Optional category.** A transfer schedule may carry an optional category
  (`category_id` on the `scheduled_transactions` row); on posting it flows to
  both posted legs. It never affects balances, linkage, or shape detection. See
  [`specs/transfer-categories.md`](transfer-categories.md).
- A schedule is exactly one of three shapes: categorized single-line, transfer
  single-line, or multi-line — enforced by validation.
- **Investment-account destinations** (401k/HSA) are out of scope here; fund
  those on a schedule via the paycheck wizard / a multi-line scheduled
  transaction (whose transfer lines route to `investment_transactions`). See
  [`specs/multiline-splits-and-paycheck.md`](multiline-splits-and-paycheck.md).

The CLI creates transfer schedules with `scheduled add --transfer-to <account>`
(mutually exclusive with `--payee`; `--category` is allowed as the optional
non-system label). `--amount` is entered as a positive magnitude and stored as
the negative signed effect on the source account, matching the TUI dialog. See
[`specs/cli.md`](cli.md).

## Multi-Line Schedules

A scheduled transaction may store a multi-line template — multiple categorized and/or transfer lines that together represent a single compound event (e.g., a paycheck). Posting a multi-line schedule creates a real transaction whose split items mirror the template structure; any transfer-line in the template creates a paired single-line counterpart in the target account.

The data model adds a `scheduled_split_items` table (one row per line, requiring at least one of `category_id` / `transfer_account_id` — a transfer line may carry both, and its category flows onto the posted split); the parent `scheduled_transactions` row keeps its scalar `amount` (representing the net effect on the parent account) and has `category_id` NULL when multi-line. A scheduled transaction is either single-line or multi-line — both shapes cannot coexist on one record.

See [`specs/multiline-splits-and-paycheck.md`](multiline-splits-and-paycheck.md) for the full data model, cascade rules, and validation. A guided **paycheck wizard** sits on top of this primitive as UI sugar — see the same spec.

A **loan payment schedule** is another consumer of this primitive: one categorized interest line, one transfer line into the loan account (principal), and zero or more fixed escrow lines, tagged with a `loan_section` column. Unlike every other schedule, a **loan-shaped** schedule is **recomputed at post time** — its interest/principal split is derived from the loan's live balance on each posting path (manual, preview, and auto-post) rather than posted verbatim — so extra principal payments and APR changes reshape subsequent payments automatically. It is created by the **loan wizard** (TUI Accounts → New Loan…, or `tmoney loan add`). See [`specs/loan-wizard.md`](loan-wizard.md) for the loan math, shape detection, and recompute-at-post behavior.

## Post-Time Preview Dialog

Pressing Enter on a due scheduled transaction (in the Scheduled view or from the dashboard's "Due" panel) opens a preview dialog rather than immediately posting. The dialog is pre-filled with the schedule's template values and lets the user adjust **this one occurrence** before saving.

Key semantics (full detail in [`specs/multiline-splits-and-paycheck.md`](multiline-splits-and-paycheck.md)):

- **Date edits are one-off.** Editing the date only changes the posted transaction's date; the schedule's `next_date` advances per the template, not the edit.
- **Amount and line edits are one-off.** Edits do not modify the template. Next occurrence reverts to template values. To permanently change the template, use **Edit Series** (`e`).
- **Hard validation applies to multi-line.** A multi-line preview with an imbalanced split disables Save until balanced.
- **Auto-post bypasses the preview** entirely and posts using template values.

## Validation Rules

1. `start_date` is required
2. `end_date` must be after `start_date` (if set)
3. `occurrences` must be positive (if set)
4. `day_of_month` must be 1-31 or -1
5. Cannot have both `end_date` and `occurrences`

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
fortnightly: next_date + (interval * 14 days)
semimonthly: next of the two pay days (day_of_month / secondary_day_of_month)
monthly:   next_date + (interval months), adjusted for day_of_month
quarterly: next_date + (interval * 3 months), adjusted for day_of_month
yearly:    next_date + (interval * 12 months), adjusted for day_of_month
```

### Month-End Handling

If day_of_month (or, when unset, the start date's day) > days in target month:
- Use last day of month
- Example: 31st monthly → Jan 31, Feb 28/29, Mar 31, Apr 30...
- Example: yearly from Feb 29 → Feb 28, Feb 28, Feb 28, Feb 29 (never Mar 1)

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

### Fortnightly Paycheck

```
frequency: fortnightly
start_date: 2024-01-05  # a Friday; the cadence recurs on next_date's weekday
amount: 2500.00
payee: "Employer"
category: "Income:Salary"
```

## CLI Commands

The CLI uses Cobra noun-verb subcommands. Multi-line schedule creation
and the paycheck wizard are TUI-only in v1 (see `specs/multiline-
splits-and-paycheck.md`); single-line schedule creation (including
transfers), editing, deletion, listing, posting, and skipping are
supported on the CLI. Editing a multi-line template is TUI-only —
`scheduled edit` refuses it with a pointer to the TUI.

```bash
# Add a scheduled transaction (fixed or variable amount; see README for flags)
tmoney scheduled add --account Checking --frequency monthly \
    --amount -1500 --payee Landlord --day 1

# Add a scheduled transfer (see the transfer note above)
tmoney scheduled add --account Checking --frequency monthly \
    --amount 250 --transfer-to Savings --category "Savings Goal"

# List scheduled transactions (--due for only those past next_date;
# --show-ids for the full UUIDs needed by edit/delete)
tmoney scheduled list
tmoney scheduled list --due
tmoney scheduled list --account Checking
tmoney scheduled list --show-ids

# Edit a single-line schedule (only supplied flags take effect; "" clears
# --amount/--payee/--category/--memo). Multi-line templates are TUI-only.
tmoney scheduled edit --id <id> --amount -1600 --frequency weekly

# Delete a schedule template by ID (posted history is kept)
tmoney scheduled delete <id>

# Post a due occurrence (works on multi-line schedules too: lines are
# posted verbatim from the template — no per-line overrides via the
# CLI; use the TUI preview for per-instance edits)
tmoney scheduled post <id> [--amount <n>] [--date YYYY-MM-DD]

# Skip an occurrence (advance next_date without creating a transaction)
tmoney scheduled skip <id>
```

## v1.5 Features (Not in v1)

- Email/notification reminders
- Linked to actual posted transactions
- Bulk operations (post all due)
- Per-line variable amount estimation on multi-line schedules
- CLI surface for creating multi-line schedules and paychecks
