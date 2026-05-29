# Multi-Line Splits and Paycheck Wizard Specification

> **Status: v1 implemented** (May 2026). The mixed-sign + transfer-
> line primitive, scheduled multi-line templates, post-time preview
> dialog, paycheck wizard, and the `Edit as paycheck →` round-trip
> affordance are all shipped. See
> [`specs/implementation-plan-multiline-splits-and-paycheck.md`](implementation-plan-multiline-splits-and-paycheck.md)
> for the per-slice landing notes (MS-001 through MS-029 are complete;
> MS-014 / MS-023 / MS-030 are manual visual smoke checks that remain
> open until exercised in a real terminal).
>
> **Paycheck wizard v2 specced** (May 2026, implementation pending).
> The wizard is being reorganized into five pay-stub-aligned sections
> (Earnings / Pre-tax / Taxes / Post-tax / Net Pay Destinations) with
> multi-line earnings support (imputed income, shift differential,
> etc.). Round-trip is preserved via a new nullable
> `paycheck_section` column on `split_items` and
> `scheduled_split_items`. See the "Paycheck Wizard" section below
> for the v2 design.
>
> Deferred out of scope for v1 (carried forward as separate work):
> CLI surface for multi-line scheduled creation and the paycheck
> wizard (TUI-only in v1); tax-aware reports that distinguish
> pre-tax vs. post-tax categories (requires a `tax_treatment` field
> on the category master); migration tool to convert legacy paired-
> single-line transfers into transfer-lines (legacy data remains
> valid as-is); per-line variable amount estimation on multi-line
> schedules; bulk operations on scheduled transactions; year-end tax
> summary report; wizard for non-paycheck complex schedules (e.g.
> mortgage principal/interest/escrow). See `Out of Scope` at the
> bottom of this spec for the canonical list.

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

The existing `split_items` table gains three nullable columns:

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | (existing) |
| `transaction_id` | UUID | (existing) |
| `category_id` | UUID NULL | (existing — now nullable) Line is categorized as this category. |
| `transfer_account_id` | UUID NULL | **(new)** Line transfers cash to this account. |
| `transfer_id` | UUID NULL | **(new)** Shared identifier linking this line to its paired single-line counter-transaction in the target account. |
| `amount` | decimal | (existing) Signed amount (mixed signs now allowed per-row). |
| `memo` | string NULL | (existing) |
| `paycheck_section` | enum NULL | **(v2)** Wizard-layout hint: `earnings`, `pre_tax`, `tax`, `post_tax`, or `net_pay_destination`. NULL for non-paycheck transactions. See [Paycheck Wizard → Section tagging](#section-tagging--paycheck_section). |

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
| `paycheck_section` | enum NULL | **(v2)** Wizard-layout hint: `earnings`, `pre_tax`, `tax`, `post_tax`, or `net_pay_destination`. NULL for non-paycheck schedules. See [Paycheck Wizard → Section tagging](#section-tagging--paycheck_section). |

**Constraint:** exactly one of `category_id` or `transfer_account_id` per row.

A scheduled transaction is **multi-line** when it has one or more `scheduled_split_items` children; otherwise it is single-line and uses the existing scalar `amount` / `category_id` on `scheduled_transactions`. The two shapes cannot coexist on the same record.

### Migration

Strictly additive — no backfill. The new columns on `split_items` and the new `scheduled_split_items` table are introduced via a schema migration; existing rows are unchanged. The legacy patterns (paired-single-line transfers, same-sign splits, single-line schedules) keep working alongside the new primitive.

The v2 `paycheck_section` column is also additive and nullable. Existing rows (including the v1 paycheck-wizard schedules) come back with NULL — they remain valid as multi-line schedules, but the Edit-as-paycheck affordance is hidden until they are re-saved through the v2 wizard, which tags every line.

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

When a transaction is saved with a transfer-line, a paired single-line transaction is automatically created in the target account, linked via the same `transfer_id`. The pair side has no split children — it's a plain single-line transaction (e.g., `+500.00 from Employer`) whose `transfer_id` matches the split-item's `transfer_id`.

**Dispatch by target account type.** The table the paired row lives in depends on the target account's type:

| Target account type | Pair lives in | Row shape |
|---|---|---|
| Non-investment (checking, savings, credit_card, …) | `transactions` | Plain single-line transaction with `transfer_id` set |
| Investment-type (`investment`, `hsa`) | `investment_transactions` | Row of type `transfer_cash`, signed in the destination frame, with `transfer_id` set |

The investment-target case is what makes a paycheck → 401k contribution line work correctly: the 401k account is investment-type, so the counterpart is minted on `investment_transactions` and feeds into investment cash balances, total-return reporting, and the investment register — instead of being a malformed regular row in the investment account's ledger.

**Future-dated posts.** The investment-side `transfer_cash` counterpart is accepted even when the post date is in the future — e.g. posting a paycheck the day before payday, or auto-posting with `post_lead_days`. Investment transactions are restricted to non-future dates only for *position- or price-bearing* types (`buy`, `sell`, `reinvest_dividend`, `fee_liquidation`, `transfer_shares`, `exchange`), because those would mint a future-dated price row or a future share/lot change. Pure cash movements (`deposit`, `withdrawal`, `fee`, `interest`, `transfer_cash`, and `dividend` — a payment linked to a security with no share price or count change) carry no such hazard and share the same future-date latitude as bank transactions. Without this carve-out, a paycheck whose legs fund a 401k or HSA could not be posted ahead of payday.

Lookup of the pair from a transfer-line must therefore consult both tables:

```sql
-- non-investment target
SELECT * FROM transactions
WHERE transfer_id = <split_item.transfer_id>
  AND id != <parent_transaction.id>

-- investment target
SELECT * FROM investment_transactions
WHERE transfer_id = <split_item.transfer_id>
```

The transaction service routes the create / find / delete / amount-update path through an `InvestmentCashCounterpartAdapter` so the cycle between `transaction.Service` and `investment.Service` stays broken; the adapter is wired at app-construction time. Without a wired adapter, transfer-lines targeting investment accounts are rejected at the service layer rather than silently creating a malformed regular row.

### Cascade rules

The cascades below apply uniformly to bank-side and investment-side counterparts; the service picks the right table by inspecting the target account's type.

| Action | Effect |
|--------|--------|
| Save transaction with new transfer-line | Create paired counter-transaction in target account (regular table or `investment_transactions` per dispatch); assign matching `transfer_id` |
| Edit transfer-line amount | Update paired side amount in lock-step (regular row `Update` or investment-row `TotalAmount` update via the adapter) |
| Edit transfer-line target account | Delete old paired side; create new paired side in new target — including cross-table moves (bank → inv, inv → bank, inv → inv) |
| Delete transfer-line from parent | Cascade delete paired side; parent retains other lines (may now be imbalanced — see Hard Validation) |
| Delete entire parent transaction | Cascade delete all paired sides (regular and investment alike) |
| Void entire parent transaction | Same cascade as Delete: paired sides are removed before the parent is set to `**VOID**` and splits are dropped |
| Delete paired side from target register | Reverse cascade: remove the corresponding line from the parent; parent must be rebalanced before saving |
| Edit paired side amount in target register | Update parent's transfer-line amount; parent must remain balanced |

A reconciled paired counterpart (on either table) blocks every cascade with `IsReconciledError`. The user must un-reconcile the counterpart before editing or deleting the parent.

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

A guided UI form (TUI-only) creates a multi-line scheduled paycheck. The wizard is **pure UI sugar**: the saved record is a standard multi-line scheduled transaction. No `kind` field, no paycheck-specific table. The v2 layout below replaces the v1 form.

### Entry point

Menu: `Transactions → New Paycheck Schedule…`

### Form layout

The wizard is organized into five sections that mirror US pay-stub structure (earnings → pre-tax deductions → statutory withholdings → post-tax deductions → net pay destinations):

```
PAYCHECK SCHEDULE                                              [×]

Employer (payee):  [_______________________]
Pay frequency:     [Biweekly ▼]    Next payday: [MM/DD/YYYY]

EARNINGS
  $[__________]   [Income:Salary ▼]
  [+ Add earnings line]

PRE-TAX DEDUCTIONS
  [+ Add pre-tax line]

TAXES
  $[____]   [Tax:Federal ▼]
  $[____]   [Tax:Social Security ▼]
  $[____]   [Tax:Medicare ▼]
  [+ Add tax line]

POST-TAX DEDUCTIONS
  [+ Add post-tax line]

NET PAY DESTINATIONS
  Primary deposit: [Checking ▼]   ($X,XXX.XX — remainder)
  [+ Add transfer]

[Cancel]                                              [Save]
```

### Pre-populated rows

The wizard opens with the following rows visible, ready to fill in:

| Section | Pre-populated rows |
|---|---|
| Earnings | 1 row with `Income:Salary` |
| Pre-tax Deductions | None |
| Taxes | 3 rows: `Tax:Federal`, `Tax:Social Security`, `Tax:Medicare` |
| Post-tax Deductions | None |
| Net Pay Destinations | Primary deposit picker only (defaults to the wizard's selected account) |

Only universally-applicable items are pre-populated. Items that vary by employer — HSA, 401(k), supplemental life, state income tax, health insurance — are added via the `[+ Add line]` button at the bottom of each section.

**Investment-account destinations.** Net Pay Destination rows accept an investment-type account (`investment` or `hsa`) as the target. The wizard does not filter the account picker by type, and the underlying transfer-line dispatch (see [Transfer Lines → Paired counterpart](#paired-counterpart)) routes the paired row to `investment_transactions` as a `transfer_cash` entry. Practically, this is how a paycheck schedule's 401(k) contribution line or HSA contribution line lands as a real investment cash flow rather than as a malformed row in the investment account's regular ledger.

### Multi-line earnings

Real pay stubs itemize earnings: base salary, shift differential, housing allowance, **imputed income** for employer-paid benefits (long-term disability coverage, group-term life over $50k, personal use of company car). The Earnings section supports any number of lines, all of which sum into "gross pay" for the net-pay computation.

Bonus and retro pay are **not** template items — they're irregular and belong in the post-time preview dialog when they actually occur, not on the recurring schedule.

### Imputed income

Imputed-income items (e.g., the value of employer-paid LTD coverage added to taxable wages even though no cash changes hands) are entered as **two independent lines**:

- An earnings line, e.g., `Income:Imputed LTD +44.03`
- An offsetting post-tax line with the same category, e.g., `Income:Imputed LTD −44.03`

The offset is mechanically necessary — without it, the parent transaction's amount would exceed the actual net deposit by the imputed amount, and the checking account balance would drift by that amount every paycheck. Real pay stubs include the same offset, usually listed as a deduction with the benefit's name (e.g., `LONG TERM DIS -44.03`).

The wizard treats the two lines as independent; the user keeps them in sync. If they fall out of sync, the imbalance indicator on the next post-time preview flags it.

The same-category convention nets imputed income to $0 in spending-by-category reports while keeping the earnings line visible for W-2/gross-pay reconciliation.

### $0 row elision on save

Rows the user leaves at $0 are silently dropped on save. Pre-populated rows are starting points, not commitments — leaving `Tax:Medicare` at $0 means "I don't have this," and the row is omitted from the saved schedule. To pause a recurring item for one paycheck (e.g., skip 401(k) for a single pay period), edit it at post-time preview, not in the template.

### Computed remainder

The "Primary deposit" line in Net Pay Destinations is the schedule's parent account. Its amount is computed at save time as the signed sum of all other lines:

```
remainder = sum(earnings) + sum(pre-tax deductions) + sum(taxes) + sum(post-tax deductions) + sum(additional transfers)
```

(All deduction/transfer amounts are negative; remainder is positive.) The computed value is stored as the parent `amount`. There is no runtime plug — on subsequent edits (preview or Edit Series), the parent amount is just another field.

### Section tagging — `paycheck_section`

To support exact round-trip editing, the wizard tags each saved split item with the section it was entered in, via the new nullable `paycheck_section` column on `split_items` and `scheduled_split_items`:

| Section in wizard | `paycheck_section` value |
|---|---|
| Earnings | `earnings` |
| Pre-tax Deductions | `pre_tax` |
| Taxes | `tax` |
| Post-tax Deductions | `post_tax` |
| Net Pay Destinations (additional transfers) | `net_pay_destination` |

The column is nullable so non-paycheck transactions can leave it NULL. The generic multi-line dialog does not expose this field — it's wizard-only — but it preserves the column value on edits to existing lines. Lines added via the generic dialog have a NULL `paycheck_section`.

`paycheck_section` is a wizard-layout hint only. It is **not** a tax classifier — the pre-tax vs post-tax distinction for tax-aware reports belongs on the category master as a separate `tax_treatment` field if/when those reports are added (see Out of Scope).

### Default categories

Seeded into new databases on file initialization (and added to existing databases on next open if missing):

- `Income:Salary`
- `Income:Bonus`
- `Income:Retro Pay`
- `Tax:Federal`
- `Tax:State`
- `Tax:Social Security`
- `Tax:Medicare`
- `Insurance:Health`

`Income:Bonus` and `Income:Retro Pay` are seeded but **not** pre-populated in the wizard; they're available in the dropdown for use in the post-time preview when an irregular bonus or retro-pay event occurs.

### Round-trip — Edit as paycheck →

A scheduled transaction qualifies as "paycheck-shaped" (and gets the `Edit as paycheck →` affordance in the Edit Series dialog) when all of:

1. It has `scheduled_split_items` children (multi-line).
2. **Every** split item has a non-NULL `paycheck_section`.
3. At least one line is tagged `earnings`.

When opened via Edit-as-paycheck, the wizard groups lines by their `paycheck_section` tag — display order within the wizard is by section, not by storage order. Existing pre-tax / tax / post-tax lines appear in their original sections.

If any line lacks a tag (e.g., the schedule was edited in the generic dialog and a new line was added), the affordance is hidden. Re-saving the schedule through the wizard re-tags every line and the affordance returns. The v1 paycheck schedules already in the wild (NULL tags everywhere) take the same path: they open in the generic dialog until re-saved through the v2 wizard.

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
