# Multi-Line Splits and Paycheck Wizard Specification

## Overview

This feature extends the transaction model to support **multi-line splits with mixed signs and transfer lines**, enabling complex events like paychecks to be modeled as a single transaction with multiple categorized and/or transferred components. It also introduces a **post-time preview/edit dialog** for scheduled transactions (Quicken-style "edit this one occurrence") and a **paycheck wizard** as a UI convenience over the new primitive.

Related specs:

- [`specs/transactions.md`](transactions.md) — base transaction model
- [`specs/scheduled-transactions.md`](scheduled-transactions.md) — scheduled-transaction model
- [`specs/tui.md`](tui.md) — TUI layout and dialogs
- [`specs/implementation-plan-multiline-splits-and-paycheck.md`](implementation-plan-multiline-splits-and-paycheck.md) — phased implementation plan

## Goals

1. Model a paycheck as a single event in the register: gross pay, tax withholdings, retirement transfers, and split deposits to multiple accounts — all as one transaction.
2. Allow per-instance overrides (date, amounts) when posting a scheduled item, without modifying the template.
3. Provide a guided form for creating a paycheck schedule from scratch.

## Non-Goals

- Tax-aware reports (W-2 Box 1 reconciliation, pre-tax vs post-tax separation in spending reports). Lines carry no `pre_tax` flag; if such reports are added later, the metadata belongs on categories.
- Bulk-edit operations on scheduled transactions.
- Migration of existing (legacy) transfers and same-sign splits into the new shape. They remain in their current shape; the new primitive applies to new transactions/schedules only.
- CLI surface for multi-line schedule creation. TUI-only for v1.
- Per-line variable amounts with estimation. Multi-line schedules store fixed line amounts; reality differences are handled at post-time preview.

## Data Model

### Extending `split_items`

The existing `split_items` table gains two nullable columns:

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | (existing) |
| `transaction_id` | UUID | (existing) |
| `category_id` | UUID NULL | (existing — now nullable) Line is categorized as this category. |
| `transfer_account_id` | UUID NULL | **(new)** Line transfers cash to this account. |
| `transfer_id` | UUID NULL | **(new)** Shared identifier linking this line to its paired single-line counter-transaction in the target account. |
| `amount` | decimal | (existing) Signed amount (mixed signs now allowed per-row). |
| `memo` | string NULL | (existing) |

**Constraints:**
1. Exactly one of `category_id` or `transfer_account_id` must be set per row.
2. `transfer_id` is set if and only if `transfer_account_id` is set.

### New `scheduled_split_items`

A new table mirrors `split_items` for scheduled transactions (no `transfer_id`, since templates don't link to real counter-transactions):

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `scheduled_transaction_id` | UUID | FK to `scheduled_transactions` |
| `category_id` | UUID NULL | Category for this line |
| `transfer_account_id` | UUID NULL | Transfer target for this line |
| `amount` | decimal | Signed amount |
| `memo` | string NULL | Optional per-line memo |

**Constraint:** exactly one of `category_id` or `transfer_account_id` per row.

A scheduled transaction is **multi-line** when it has one or more `scheduled_split_items` children; otherwise it is single-line and uses the existing scalar `amount` / `category_id` on `scheduled_transactions`. The two shapes cannot coexist on the same record.

### Migration

Strictly additive — no backfill. The new columns on `split_items` and the new `scheduled_split_items` table are introduced via a schema migration; existing rows are unchanged. The legacy patterns (paired-single-line transfers, same-sign splits, single-line schedules) keep working alongside the new primitive.

## Mixed-Sign Splits

Today's split validation requires all line amounts to share the parent's sign. This is relaxed:

- Split lines may be positive or negative independently.
- The parent transaction's `amount` is the **signed sum** of all line amounts — i.e., the net effect on the parent account.
- Each line contributes its signed amount to its category (or transfer counterpart) in reports.

### Example: paycheck

| Line | Amount | Type |
|------|--------|------|
| Gross salary | +5,000.00 | Category `Income:Salary` |
| Federal tax | −800.00 | Category `Tax:Federal` |
| Social Security | −310.00 | Category `Tax:Social Security` |
| Medicare | −72.50 | Category `Tax:Medicare` |
| 401(k) contribution | −500.00 | Transfer → 401k account |
| Health insurance | −150.00 | Category `Insurance:Health` |
| HSA contribution | −100.00 | Transfer → HSA account |

Parent amount = +3,067.50 (net deposit to checking).

## Transfer Lines

A split line may be a **transfer** instead of a categorized line.

### Storage

`transfer_account_id` is set, `category_id` is NULL, and `transfer_id` carries a fresh UUID shared with the paired counter-transaction. The display label is derived from the linked account's name.

### Paired counterpart

When a transaction is saved with a transfer-line, a paired single-line transaction is automatically created in the target account, linked via the same `transfer_id` on the *transactions* row. The pair side has no split children — it's a plain single-line transaction (e.g., `+500.00 from Employer`) whose `transfer_id` matches the split-item's `transfer_id`.

Lookup of the pair from a transfer-line:

```sql
SELECT * FROM transactions
WHERE transfer_id = <split_item.transfer_id>
  AND id != <parent_transaction.id>
```

### Cascade rules

| Action | Effect |
|--------|--------|
| Save transaction with new transfer-line | Create paired counter-transaction in target account; assign matching `transfer_id` |
| Edit transfer-line amount | Update paired side amount in lock-step |
| Edit transfer-line target account | Delete old paired side; create new paired side in new target |
| Delete transfer-line from parent | Cascade delete paired side; parent retains other lines (may now be imbalanced — see Hard Validation) |
| Delete entire parent transaction | Cascade delete all paired sides |
| Delete paired side from target register | Reverse cascade: remove the corresponding line from the parent; parent must be rebalanced before saving |
| Edit paired side amount in target register | Update parent's transfer-line amount; parent must remain balanced |

### Self-transfer prohibited

A transfer-line's `transfer_account_id` must not equal the parent transaction's `account_id`. Validation rejects self-transfers.

### Display

In a category combo box on a split line, a `Transfer →` sentinel appears as a special option (alongside `[+ Add new category…]`). Selecting it swaps the field for an account picker. On save, the line is stored with `transfer_account_id` set and `category_id` NULL — there is no synthetic "Transfer:<account>" category row in the database for new transfer-lines.

Legacy (pre-feature) transfers continue to use the existing `Transfer:<account>` category convention and paired-row `transfer_id` on the transactions table. Both shapes are valid; new transfer-lines just live on the split-item.

## Hard Validation (No Plug)

A multi-line transaction is **balanced** when:

```
parent.amount == signed_sum(line.amount for line in lines)
```

The UI displays the imbalance live as the user types. Save is **disabled** until the imbalance is zero. There is no automatic plug or balancing line — the user manually adjusts whichever line should absorb the difference.

## Scheduled Multi-Line Templates

A scheduled transaction may store a multi-line template via `scheduled_split_items`. Posting the schedule creates a real transaction whose `split_items` match the template structure (with any transfer-lines auto-creating paired counterparts).

### Variable amounts in multi-line schedules

Multi-line schedules do **not** support per-line variable estimation. Each line has a fixed template amount. Real-world variations (FICA penny shifts, payday holiday shifts) are handled at the post-time preview, where the user edits whatever is different for this one occurrence.

Legacy single-line schedules keep `amount_estimate_count` for backward compatibility.

## Post-Time Preview Dialog

Pressing Enter on a due scheduled transaction in the Scheduled view (or from the dashboard's "Due" panel) opens a **preview dialog** rather than immediately posting.

### Behavior

- The dialog is pre-filled with the schedule's template values (date = `next_date`, amounts = template line amounts).
- The user can edit any field: date, payee, memo, individual line amounts, parent amount.
- For multi-line schedules, lines render as a mini-split table with live imbalance indicator.
- For single-line schedules, the dialog matches the existing single-transaction entry dialog with template values pre-filled.
- Save creates the real transaction and advances the schedule.
- Cancel returns to the scheduled list with no changes.

### Edit semantics

1. **Date edits are one-off.** Editing the date in the preview only changes the posted transaction's date. The schedule's `next_date` advances based on the template's original `next_date`, not the edited date. (Example: schedule fires on the 1st; pay early on Apr 28 by editing date → next occurrence still May 1.)
2. **Amount and line edits are one-off.** Edits do not modify the template. Next instance reverts to template values. To permanently change the template, use Edit Series.
3. **Hard validation applies.** A multi-line preview with an imbalanced split disables Save until balanced.

### Edit Series vs. Edit This Instance

| Action | Key | Effect |
|--------|-----|--------|
| Enter on due item | `Enter` | Open preview, edit and post this one occurrence |
| Edit series | `e` | Open the scheduled-transaction dialog, modify the template (affects all future occurrences) |

This mirrors Quicken's distinction between "Enter Transaction" (post one) and "Edit this Reminder" (edit the series).

### Auto-Post Interaction

Auto-post bypasses the preview entirely and posts using template values exactly. Users who want preview-before-post on a given schedule must leave auto-post off. For paychecks specifically, most users will leave auto-post off because of FICA-penny fluctuations.

## Paycheck Wizard

A guided UI form (TUI-only, v1) creates a multi-line scheduled paycheck. The wizard is **pure UI sugar**: the saved record is a standard multi-line scheduled transaction. No `kind` field, no paycheck-specific table.

### Entry point

Menu: `Transactions → New Paycheck Schedule…`

### Form layout

```
PAYCHECK SCHEDULE                                              [×]

Employer (payee):  [_______________________]
Pay frequency:     [Biweekly ▼]    Next payday: [MM/DD/YYYY]

Gross pay:         $[__________]   → category [Income:Salary ▼]

PRE-TAX DEDUCTIONS
  Federal income tax     $[____]   [Tax:Federal ▼]
  State income tax       $[____]   [Tax:State ▼]
  Social Security        $[____]   [Tax:Social Security ▼]
  Medicare               $[____]   [Tax:Medicare ▼]
  401(k) contribution    $[____]   Transfer → [401k account ▼]
  [+ Add pre-tax line]

POST-TAX DEDUCTIONS
  Health insurance       $[____]   [Insurance:Health ▼]
  HSA contribution       $[____]   Transfer → [HSA account ▼]
  [+ Add post-tax line]

NET PAY DESTINATIONS
  Primary deposit account: [Checking ▼]  ($X,XXX.XX — remainder)
  Additional transfers:
    Savings              $[____]   Transfer → [Savings ▼]
  [+ Add transfer]

[Cancel]                                              [Save]
```

### Computed remainder

The "Primary deposit account" line is computed at wizard-save time as:

```
remainder = gross − sum(pre-tax deductions) − sum(post-tax deductions) − sum(additional transfers)
```

This computed value is stored as a fixed line on the resulting multi-line schedule. There is no runtime plug — on subsequent edits (in the preview or in Edit Series), the line is just another line.

### Pre-tax vs post-tax grouping

The wizard groups deductions into pre-tax and post-tax for visual organization only. The flag is **not stored** on the saved lines. If a future tax-aware report is needed, the metadata belongs on the category master (a `tax_treatment` field on `categories`), not on individual lines.

### Default categories

The wizard pre-populates category dropdowns with the following defaults. These are seeded into the database on file initialization (existing databases gain them on next open if missing):

- `Income:Salary`
- `Tax:Federal`
- `Tax:State`
- `Tax:Social Security`
- `Tax:Medicare`
- `Insurance:Health`

### Round-trip edits

After save, the schedule is just a standard multi-line schedule. Editing it via the regular `e` action opens the generic multi-line scheduled-transaction dialog. An "Edit as paycheck →" button on a paycheck-shaped schedule (heuristic: has gross income line, tax category lines, structured similarly) may relaunch the wizard with current values pre-filled. Best-effort; hidden when the schedule has been edited into a shape the wizard cannot represent.

## Reports Impact

Mixed-sign splits change how a single transaction contributes to category reports:

- Each line independently contributes its signed amount to its category total.
- A paycheck with `+5000` on `Income:Salary` and `−800` on `Tax:Federal` contributes `+5000` to salary income and `−800` to federal tax expense in the spending-by-category report.
- The net-worth report is unaffected: it sees the parent transaction's effect on each account (parent amount = signed sum of lines).

This is the correct accounting behavior — tax withholdings are real expenses, paid by the employer on the user's behalf, and should appear in spending reports even though the money never enters the user's checking account.

## CLI Surface

For v1, multi-line scheduled transactions and the paycheck wizard are TUI-only. The CLI keeps its existing single-line `scheduled add` behavior.

`tmoney scheduled post <id>` works on multi-line schedules: it uses template values verbatim (no per-line overrides) and prints the multi-line breakdown in its output. Users who want per-instance overrides post via the TUI preview dialog.

`tmoney transaction add` and `tmoney transfer add` remain single-line; multi-line transaction creation from the CLI is deferred.

If real automation needs emerge later, a spec-file approach (`tmoney scheduled paycheck create --from paycheck.toml`) is the recommended path — forward-compatible with the TUI-only v1.

## Validation Rules

In addition to existing validation rules:

1. A split line must have exactly one of `category_id` or `transfer_account_id` set.
2. A split line with `transfer_account_id` set must also have a `transfer_id`; one without `transfer_account_id` must have `transfer_id` NULL.
3. For multi-line transactions, `parent.amount == signed_sum(line.amounts)`. The sign of each line is unconstrained.
4. A transfer-line's `transfer_account_id` must not equal the parent transaction's `account_id` (no self-transfers).
5. A scheduled transaction is either single-line (uses scalar `amount` / `category_id`) or multi-line (has `scheduled_split_items` rows). Both shapes cannot coexist on the same record.

## Examples

### Paycheck schedule (created via wizard)

```
scheduled_transactions:
  id: <st-id>
  account_id: <checking-id>
  payee_id: <employer-payee-id>
  amount: +3067.50         -- net deposit
  category_id: NULL        -- multi-line; category lives in split items
  frequency: biweekly
  day_of_week: 5           -- Friday
  start_date: 2026-01-09
  next_date: 2026-01-23

scheduled_split_items (children of <st-id>):
  ────────────────────────────────────────────────────────────────
  category_id=Income:Salary           amount: +5000.00
  category_id=Tax:Federal             amount:  -800.00
  category_id=Tax:Social Security     amount:  -310.00
  category_id=Tax:Medicare            amount:   -72.50
  transfer_account_id=<401k-id>       amount:  -500.00
  category_id=Insurance:Health        amount:  -150.00
  transfer_account_id=<hsa-id>        amount:  -100.00
```

### Posting the paycheck

When Enter is pressed on the due item, the preview opens with the above values pre-filled. The user notices FICA is actually −310.01 this pay period, edits that line to −310.01, and also nudges Federal Tax by +0.01 to keep the parent balanced at +3067.50. Save creates:

- One real `transactions` row in checking, `amount: +3067.50`, with 7 `split_items` rows mirroring the template (with the FICA and Federal edits applied)
- One paired real transaction in the 401k account, `amount: +500.00`, with `transfer_id` matching the corresponding split-item's `transfer_id`
- One paired real transaction in the HSA account, `amount: +100.00`, similarly linked
- The schedule's `next_date` advances from 2026-01-23 to 2026-02-06 (biweekly cadence based on template's original next_date)

The template is unchanged — next paycheck preview opens with FICA back at −310.00.

## Out of Scope

The following are explicitly deferred:

- **CLI surface for multi-line schedules and the paycheck wizard.** Multi-line creation/edit remains TUI-only. If real automation needs emerge, a spec-file approach is the recommended path.
- **Tax-aware reports.** Pre-tax vs post-tax distinction in reports (e.g., W-2 Box 1 reconciliation) requires a `tax_treatment` field on the category master, not on individual lines.
- **Migration tool to convert legacy paired-transfers into transfer-lines.** Legacy data remains valid as-is.
- **Per-line variable amounts with estimation.** Multi-line schedules use fixed template amounts; preview-time edits handle variation.
- **Bulk operations on scheduled transactions** (post-all-due, skip-all, etc.).
- **Year-end tax summary report** based on paycheck data.
- **Wizard for non-paycheck complex schedules** (e.g., mortgage payment with principal/interest/escrow). Such schedules can be created via the generic multi-line scheduled dialog.
