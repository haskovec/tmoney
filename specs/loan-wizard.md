# Loan Wizard Specification

**Status**: Draft v2 — revised after adversarial review
**Depends on**: multi-line scheduled transactions (`specs/multiline-splits-and-paycheck.md`), scheduled transactions (`specs/scheduled-transactions.md`), accounts (`specs/accounts.md`)

A guided flow that sets up everything tmoney needs to track an amortized
loan: the loan account, an optional linked asset account (house, car), and
a monthly loan-payment schedule whose interest/principal split is
**recomputed from the live loan balance every time it posts** — so extra
principal payments, mid-life adoption, and rate changes stay correct
without regenerating anything.

This spec also standardizes the liability sign convention (resolving a
pre-existing contradiction between the two net-worth code paths) and adds
a system **Value Adjustment** category for asset revaluations.

## Overview

A monthly loan payment decomposes into:

- **Interest** — an expense, categorized (default `Loan:Interest`,
  auto-created), paid from the funding account.
- **Principal** — not an expense: a transfer from the funding account into
  the loan account, moving its negative balance toward zero. The
  principal transfer line is labeled **`Loan:Principal`** by default —
  overridable via `loan add --principal-category` or the wizard's
  *Principal category* field, and suppressible to none. The category is
  purely a cash-flow label: the line stays a real transfer that reduces
  the loan balance and never double-counts against net worth (the
  category and the transfer target now coexist on one split — relaxed
  CHECK, migration 029). Recompute-at-post preserves the label on every
  post. See [`specs/transfer-categories.md`](transfer-categories.md) for
  the categorized-transfer mechanics.
- **Escrow / other** (optional) — fixed categorized lines (property tax,
  insurance, PMI) so the schedule total matches the real bank draft.

This maps exactly onto an existing primitive: a **multi-line scheduled
transaction** on the funding account with one categorized interest line,
one transfer line targeting the loan account, and zero or more categorized
escrow lines. The model, posting logic, and TUI pickers already support
transfer lines into loan accounts end to end. What is new:

1. A **loan math engine** (amortization formulas, projection).
2. **Recompute-at-post**: line amounts for loan-shaped schedules are
   computed from the loan's current balance at posting time on every
   posting path, instead of copied verbatim from the template.
3. A **wizard** (TUI: Accounts → New Loan…; CLI: `tmoney loan add`) that
   creates the accounts and the schedule in one atomic pass.
4. An **amortization view** (TUI drill-in + `tmoney loan show`).

## Terminology

Used consistently throughout this spec:

- **owed** — the positive magnitude of the loan's (negative) balance.
- **P&I payment** — the fixed principal-and-interest portion of the
  monthly payment. This is what the wizard's *Payment amount* field and
  the CLI `--payment` flag mean. Escrow is **not** included.
- **total draft** — P&I payment + Σ escrow lines; the amount that leaves
  the funding account. The schedule's parent amount is −(total draft).

## Goals

- Set up a loan (new at origination **or** already in progress) in one
  guided flow: loan account, optional asset account, payment schedule.
- Post-time interest/principal splits that track reality: computed from
  the actual loan balance, so extra principal payments and APR edits
  automatically reshape every subsequent payment.
- Correct net worth: asset value plus (negative) loan balance, with
  liabilities rendered as positive magnitudes where a section heading
  already carries the sign.
- Full CLI parity: `loan add`, `loan list`, `loan show`.
- Amortization projection: remaining payments, payoff date, total
  interest remaining — in the TUI and CLI.

## Non-Goals (v1)

- Payment frequencies other than **monthly**.
- Historical backfill of past payments for mid-life loans.
- Negative amortization (P&I payment ≤ month's interest is an error).
- Automatic ARM rate schedules (edit the account's APR when the rate
  changes; future posts pick it up).
- Escrow analysis / escrow rebalancing (edit the escrow lines via
  *Edit as loan* when the servicer changes them).
- Automatic depreciation curves for assets (a plain scheduled transaction
  on the asset account already covers straight-line depreciation).
- A persistent loan↔asset link column (net worth needs none; revisit if an
  equity report is ever wanted).
- Daily/exact-day interest accrual and mid-cycle proration (see
  *Interest convention*).
- Deferral/forbearance modeling (see *Skipping an occurrence*).

## Sign Convention Standardization

Today the codebase contradicts itself:

- `specs/accounts.md` declares loan **positive = owed**, and
  `report net-worth` computes `assets − liabilities`
  (`internal/report/report_service.go:151`).
- The `account balance` CLI computes `assets + liabilities`, assuming
  liabilities are stored **negative** (`internal/cli/account/format.go:138`)
  — which is how credit cards de facto behave, since purchases are
  negative transactions.

Only one convention lets a principal payment be an ordinary linked
transfer (from-leg negative, to-leg positive, legs cancel —
`internal/transaction/transaction.go:494`):

**Standard: liability balances are stored negative.** A $250,000 mortgage
sits at −250,000. A principal payment's +500 leg moves it toward zero.

Changes:

- `report net-worth` computes `NetWorth = TotalAssets + TotalLiabilities`
  over signed balances (liabilities ≤ 0). The report struct keeps
  **signed** values; presentation layers transform.
- **Display rule (negation, not abs)**: wherever liabilities appear under
  an explicit LIABILITIES heading (TUI dashboard columns, TUI Reports →
  Net Worth view, CLI `report net-worth`), render the **negated**
  balance: a −$249,500 mortgage displays as `249,500.00`; an overpaid
  loan or credit-balance card (positive balance) displays negative,
  which correctly reads as a credit. Section totals are the negated sum.
  Registers and the flat `account balance` list keep true signed values
  (double-entry stays visible where it matters).
- Update the sign table in `specs/accounts.md` (loan and credit card:
  negative = owed; positive = overpayment/credit).
- **Reconciliation note**: servicer statements show a positive balance
  owed. When reconciling a loan or credit-card account, enter the
  statement balance **negated**, exactly as credit-card reconciliation
  works today. Document this in the reconciliation spec's examples.
- **Behavior note**: users who entered credit-card debt see net worth
  *corrected* (the old subtract path overstated it). Users who entered
  loan balances positive per the old spec must flip the sign of the
  opening balance (one-time manual edit; called out in release notes).

## Interest Convention

- Monthly periodic rate: `r = APR / 100 / 12`, kept at full
  `alpacadecimal` precision — **never** pre-rounded. Only the final
  interest figure is rounded.
- `interest = round_half_up(owed × r, 2)`.
- The balance used is the loan account's balance **as of the occurrence
  date being posted** (opening balance + all non-void transactions dated
  on or before that date), before the payment being posted is written.
  Posting several overdue occurrences in sequence therefore compounds
  correctly — each post sees the previous one.
- **Known approximation**: a curtailment (extra principal) dated
  mid-cycle reduces the *upcoming* payment's interest by a full month's
  worth of `r × amount`; a real servicer accrues on the cycle-start
  balance, so tmoney can understate one month's interest by up to that
  amount. The post-time preview exists to reconcile pennies (and, for
  large curtailments, dollars) against the statement.
- This is the standard US mortgage/car-loan convention and matches
  servicer statements to within rounding.

## Payment Math

Standard amortization P&I payment (wizard prefill, always editable):

```
M = P · r / (1 − (1 + r)^−n)        r > 0    (round half-up to cents)
M = ceil_to_cent(P / n)              r = 0    (0% promotional loans)
```

`P` = principal, `n` = term in months. The 0% branch rounds **up** so
`M × n ≥ P` — the clamped final payment is then slightly smaller,
instead of a stray n+1st one-cent payment.

Per-occurrence split, given the fixed **P&I payment** (escrow lines are
fixed pass-throughs and take no part in this formula):

```
interest  = round_half_up(owed × r, 2)
principal = pi_payment − interest
```

Guards:

- **Clamp (final payment)**: if `principal > owed`, set
  `principal = owed`; the posted total draft shrinks to
  `interest + principal + Σ escrow`. The preview shows the smaller
  final amount.
- **Negative amortization**: if `principal ≤ 0` while `owed > 0`
  (i.e. `pi_payment ≤ interest`), refuse to post with a clear error.
  The wizard validates `pi_payment > month-one interest` at save time.
- **Missing APR**: a loan account whose `interest_rate` is NULL (e.g.
  cleared later via the account-edit dialog) is a **typed error** at
  post/projection time ("loan account has no interest rate set") — never
  silently treated as 0%.
- **Zero computed lines**: a computed line of exactly $0.00 is omitted
  from the **posted transaction** (posted-transaction validation rejects
  zero-amount splits — `internal/transaction/transaction.go:440`). The
  same rule applies to the **template**: scheduled-split validation also
  rejects zero amounts (`internal/scheduled/split_item.go:100`), which
  is why 0% loans have no interest line at all (next section).

### 0% APR loans

A 0% loan's template **omits the interest line entirely** (a $0.00 line
would be rejected by template validation). Loan-shape detection
therefore requires *at most* one interest line, not exactly one. While
the APR is 0, every post books the full P&I payment as principal. The
principal transfer line is still labeled `Loan:Principal` — 0% loans
omit only the interest line.

The missing-interest-line guard fires only when **computed interest is
greater than $0.00** and the schedule has no interest line — the typed
error "loan schedule has no interest line — open Edit as loan to add
one" (*Edit as loan* adds the line, prompting for the interest category,
which the wizard did not collect for a 0% loan). While computed interest
rounds to $0.00 — a 0% APR loan, or a nearly-paid loan with a tiny
remaining balance — posting simply proceeds with no interest line; no
repair is needed or possible (a $0.00 line could not be stored anyway).

## Data Model

### `loan_section` split tag (scheduled templates only)

Mirroring the `paycheck_section` pattern
(`internal/db/migrations/020_paycheck_section.sql`), add a nullable
`loan_section TEXT` column to **`scheduled_split_items` only**:

- CHECK `loan_section IN ('interest', 'principal', 'escrow')`.
- CHECK `paycheck_section IS NULL OR loan_section IS NULL` (a split
  belongs to at most one wizard family).
- DuckDB table-recreate migration pattern (as in 020/026), with
  migration tests.

**Not** on `transaction_splits`: posted transactions need no tag (the
interest line is identifiable by its category, the principal line by its
transfer target's account type), and the `paycheck_section` column on
`transaction_splits` is in fact never populated by any posting path
today — there is no copy-through precedent to follow.

No schedule-level columns are needed:

- The **loan account** is identified by the principal split's
  `transfer_account_id`.
- The **APR** lives where it already lives: `accounts.interest_rate`
  (currently display-only; this feature is its first computation).
- The **payment day** and cadence are the schedule's existing fields.
- The **P&I payment** is derived: parent amount magnitude − Σ escrow
  line magnitudes. `ComputeLoanSplits` derives it exactly this way;
  nothing stores it separately (so editing escrow via *Edit as loan*
  keeps everything consistent by construction).

### Loan-shaped schedule (strict detection)

A schedule is **loan-shaped** — and gets recompute-at-post, clamp,
payoff completion, and the loan affordances — when all of the following
hold (`IsLoanShaped`, analogous to `looksLikePaycheck`,
`internal/tui/paycheck_wizard.go:1731`):

- Multi-line with a fixed parent amount, **monthly frequency with
  interval 1 and no semi-monthly secondary day** (anything else would
  book a full month's interest at the wrong cadence).
- Every split has a non-NULL `loan_section`.
- Exactly one `principal` split, and it is a transfer line whose target
  is an **active loan-type account**.
- **At most one** `interest` split (categorized); absent only for 0%
  loans per above.
- Zero or more `escrow` splits (categorized).

Anything else is a generic multi-line schedule: no recompute, no loan
affordances, template values post verbatim.

### Loose shape (adoption / re-promotion)

A schedule is **loan-adoptable** when it is multi-line, monthly
(interval 1, no secondary day), and has exactly one transfer line
targeting an active loan-type account — regardless of tags. The Edit Series dialog offers **Edit as loan →** for
loan-adoptable schedules too; saving through the wizard (re)writes the
tags, promoting the schedule to strictly loan-shaped. This is both the
recovery path after an accidental demotion and the adoption path for
hand-built loan schedules that predate the wizard.

### Demotion guard

Saving a **loan-shaped** schedule through the *generic* Edit Series
split editor strips the tags (the generic editor round-trips only
category/target, amount, and memo) and silently demotes the schedule —
after which posts would copy the stale snapshot verbatim. Because that
failure mode is behavioral (wrong interest booked monthly), not merely
cosmetic like a paycheck demotion:

- The Edit Series dialog for a loan-shaped schedule presents
  **Edit as loan →** as the primary affordance (same placement as
  *Edit as paycheck*).
- Proceeding through the generic editor anyway shows a one-line warning
  on save: *"This converts the loan schedule to a generic schedule —
  payments will no longer compute interest automatically. Continue?"*
- Recovery is always available via the loose-shape adoption path above.

### Template contents

The wizard stores a **snapshot of month one** in the template (so the
generic multi-line validation `Σ lines = parent amount`,
`internal/scheduled/scheduled_service.go:928`, keeps holding, and
non-loan-aware UI renders something sensible). Template amounts are
never trusted at post time for loan-shaped schedules — they are
recomputed. The schedule is **indefinite** (no occurrences / end date):
its end is driven by payoff, so extra principal ends it early and
skipped months extend it, both for free.

## Posting Behavior

One computation function serves every path:
`ComputeLoanSplits(schedule, occurrenceDate)` in the scheduled service —
looks up the loan account's APR and as-of balance, derives the P&I
payment (parent magnitude − Σ escrow), applies *Payment Math*, and
returns the adjusted parent amount + splits (tags carried on the
template side only; zero computed lines omitted; signs: parent and all
lines negative on the funding account, transfer counterpart positive
into the loan).

| Path | Behavior |
|------|----------|
| TUI post-time preview | Seeds the preview's lines with computed values instead of template values. **Reseed rule**: editing the preview's Date recomputes the seed for the new date, *until* the user edits any line amount — after that, user values win and date changes no longer reseed. Saves via `PostWithEdits` (no recompute after user edits). |
| `scheduled post` (CLI) / `Post`/`PostWithDate` | `buildMultiLineTransaction` uses computed splits for loan-shaped schedules. `--amount` override remains rejected for multi-line, unchanged. |
| Auto-post | Same computed splits (never stale). The wizard **defaults auto-post off**. Any loan-computation error (paid off, negative-am, missing APR, missing interest line) **skips that schedule with a reason** — the existing skip-with-reason mechanism used for closed accounts — and must never abort the rest of the auto-post batch. |
| Manual post of a paid-off loan | If `owed ≤ 0` at post time, refuse with a typed "loan is paid off" error **and mark the schedule completed on the spot** (see below). |

### Payoff completion

Applies on **every** posting path, including `PostWithEdits` (the
flagship manual-preview flow): after the post is written, if the loan
balance is now **≥ 0** (zero or overshot by a penny-tweaked edit), the
service marks the schedule **completed**.

Completion also applies when a post is **refused** because the loan is
already paid off — `owed ≤ 0` is a terminal state, so both the manual
"loan is paid off" refusal and the auto-post skip mark the schedule
completed. An ad-hoc payoff transfer therefore cannot strand a
never-postable schedule (there is no other user-facing "complete this
schedule" affordance).

Mechanism — explicitly *not* the `end_date` trick, which has two traps
(set before `AdvanceSchedule` it strands `NextDate == EndDate`, which
`IsCompleted` does not treat as complete; and a first-occurrence payoff
would violate `end_date > start_date` validation, making the schedule
uneditable). Instead: set `occurrences_remaining = 0` (backfilling
`occurrences = 1` on an indefinite schedule — validation requires a
positive value ≥ remaining), which is the same state a naturally
exhausted fixed-duration schedule reaches.
Implementation must verify field nullability and add a validation
carve-out if setting a terminal occurrence state on an
indefinite schedule trips existing checks.

**Due-list repair (pre-existing bug, fixed here):** completed schedules
are currently filtered only from the *upcoming* list; `ListDue` filters
on `next_date` alone, so a completed schedule whose `next_date` is in
the past shows as *due* forever in the TUI due section, the due-count
badge, and `scheduled list --due` — and both Post and Skip refuse with
`CompletedError`, so the user can never clear it. Fix `ListDue` (or its
consumers) to exclude completed schedules. This also repairs the same
zombie for naturally exhausted fixed-duration schedules today.

After a completing post, the TUI shows a toast: *"Loan paid off — close
the account from the Accounts menu when ready."* (`account close`
requires the zero balance it now has.) Nothing is deleted automatically.

## Loan Wizard (TUI)

Menu: **Accounts → New Loan…** (the wizard's primary product is
accounts; the schedule is attached output). App-level overlay following
the paycheck wizard's integration pattern
(`internal/tui/paycheck_wizard.go`, state field + key routing + render
overlay + mouse routing).

### Fields

**Loan** section:

| Field | Notes |
|-------|-------|
| Name | Loan account name (e.g. "Mortgage — 123 Main St") |
| Institution | Optional |
| Current balance | Required; what you owe today (entered positive; stored negated) |
| APR | Required (0 allowed); stored on the account's `interest_rate` |
| Original principal | *Optional* — used only to prefill the payment |
| Open date | *Optional* — origination date; recorded as the account's opening date only for a new loan (see Save behavior) |
| Term (months) | *Optional* — used only to prefill the payment |

Original principal, open date, and term exist **only** to compute the
payment prefill; they are not stored anywhere. Mid-life users who know
their payment can skip all three.

**Payment** section:

| Field | Notes |
|-------|-------|
| Payment amount (P&I) | Prefilled from the amortization formula when principal/APR/term are all present and the field is untouched; always editable (servicers round differently). Required. Excludes escrow. |
| Next payment date | First unposted payment; seeds the schedule's start/next date and day-of-month |
| From account | Picker: active non-investment accounts |
| Payee | Optional (e.g. the servicer); auto-created |
| Interest category | Required when APR > 0; hidden when APR = 0. Defaults to `Loan:Interest`, which is **get-or-created at save time** (parent `Loan`, child `Interest`) — the default is always available even on files where it was deleted. Picker with inline category creation for choosing something else. |
| Principal category | Optional; defaults to `Loan:Principal` (get-or-created at save time). Clearable to none; picker with inline category creation. Labels the principal transfer line for cash-flow reporting and never affects balance math. |
| Escrow lines | Repeatable rows: category + fixed amount; add/remove |
| Auto-post | Checkbox, default **off** |

**Asset** section (optional):

| Field | Notes |
|-------|-------|
| Track an asset | Checkbox; reveals the fields below |
| Asset name | e.g. "123 Main St", "2022 Outback" |
| Current value | Purchase price at origination, or today's market value |

### Save behavior

Wizard save is **one atomic, single-undo operation**: all record
creation happens in one DB transaction wrapped in one undo/redo
command; any failure rolls back everything (no orphaned loan account
silently swinging net worth).

1. Create the **loan account**: type `loan`, opening balance =
   **−(current balance)**, `interest_rate` = APR, institution. Opening
   date = open date when it was provided *and* current balance equals
   the original principal (new loan at origination), otherwise today
   (mid-life: the balance is a today snapshot; no history exists behind
   it — the origination date is intentionally not recorded).
2. Optionally create the **asset account**: type `asset`, opening
   balance = current value, opening date as above. No transactions are
   generated (opening-balances-only model; a down payment is whatever
   was already recorded in the funding account).
3. Create the **schedule**: monthly, indefinite, on the funding account.
   The month-one snapshot (parent and lines) is exactly the
   `ComputeLoanSplits` output for the next payment date — **including
   the final-payment clamp**, so a loan created with one payment left
   stores a clamped parent that its lines sum to; the interest line is
   omitted when month-one interest is $0.00. Lines are tagged with
   `loan_section`; auto-post per checkbox.
4. Validation: current balance > 0; APR in [0, 100); P&I payment >
   month-one interest (no negative amortization); interest category
   present when APR > 0; escrow categories required on non-empty rows.

### Edit as loan (round-trip)

The Edit Series dialog shows **Edit as loan →** for loan-shaped *and*
loan-adoptable schedules (paycheck parity,
`internal/tui/scheduled_dialog.go:335`). Edit mode reopens the wizard
prefilled from the schedule + loan account, with the creation-only
fields (original principal, open date, term) **omitted** — they are not
stored and are not needed: the P&I payment is the source of truth, and
the projection derives everything else from the live balance.

Edit mode shows: loan-account fields (name, institution, APR — editing
these edits the account), payment fields (P&I amount, interest
category, principal category, escrow lines, auto-post), and — when the
schedule lacks an interest line and APR > 0 — prompts for the interest
category to add one. The principal category prefills from the existing
principal line, so **Edit as loan →** round-trips the `Loan:Principal`
label instead of dropping it. Saving rewrites the template snapshot (rebalanced month-one
values, fresh tags) and applies account edits, as **one atomic,
single-undo operation** (an undo must never leave a new-APR/old-payment
hybrid).

Changing the **payment amount — including adding a recurring extra
principal contribution — is done here**, not through the generic
editor (see *Demotion guard*).

## Amortization View (TUI)

Drill-in view from a **loan account's register**, key **`a`** (verified
free in the register keymap; mirrors the prices list → history
drill-in; Esc returns). Computed live from the current balance, the
account's APR, and the loan-shaped schedule's derived P&I payment —
never stored.

Header: loan name · balance (displayed negated) · APR · P&I payment ·
payments left · payoff date · total interest remaining.

Table: `# | DATE | PAYMENT | INTEREST | PRINCIPAL | ESCROW | BALANCE`
— one row per remaining payment, final row clamped.

**Projection cap**: the projection stops at 1,200 rows (100 years). If
the balance is still positive at the cap, the projection is flagged
**truncated**: the view (and `loan show` / `loan list`) display payoff
date and total-interest-remaining as `100y+` rather than reporting the
cap row as if it were payoff. (Reachable: the negative-am guard only
requires principal > 0, so a $1/month principal is legal.)

If no loan-shaped schedule targets the account, the view shows the
stats it can compute (balance, APR) and a hint to run the wizard or
adopt an existing schedule via *Edit as loan*.

## CLI

New `loan` command group (noun-verb, `specs/cli.md` conventions):

```bash
# One-shot setup: loan account + schedule (+ optional asset account)
tmoney -f personal.tdb loan add --name "Mortgage" \
  --current-balance 312450.22 --rate 6.5 \
  --payment 2401.86 \
  --next-payment-date 2026-08-01 --from-account "Checking" \
  --escrow "Housing:Property Tax=650" --escrow "Housing:Home Insurance=120" \
  --payee "Wells Fargo" \
  --asset-name "123 Main St" --asset-value 450000

# New loan at origination: give original terms, let the payment be computed
tmoney -f personal.tdb loan add --name "Car Loan" \
  --principal 32000 --rate 5.9 --term-months 60 --open-date 2026-07-01 \
  --next-payment-date 2026-08-01 --from-account "Checking"

# List loans: balance, rate, payment, next date, payoff, interest left
tmoney -f personal.tdb loan list

# Details + amortization projection (--limit N, default 12; --all for everything)
tmoney -f personal.tdb loan show "Mortgage"
tmoney -f personal.tdb loan show "Mortgage" --limit 24
tmoney -f personal.tdb loan show "Mortgage" --all
```

- Required: `--name`, `--rate`, `--from-account`, `--next-payment-date`,
  and a balance (`--current-balance`, defaulting to `--principal` when
  only that is given).
- `--payment` (P&I, escrow-exclusive) is required unless `--principal`
  and `--term-months` are both present, in which case it is computed
  and printed for comparison against the statement.
- `--open-date`, `--principal`, `--term-months`: prefill-only, same
  semantics and non-storage as the wizard fields.
- `--interest-category`: optional; defaults to `Loan:Interest`,
  get-or-created at save time exactly like the wizard. Pass the flag to
  book interest elsewhere.
- `--principal-category`: optional; defaults to `Loan:Principal`,
  get-or-created at save time; an explicit empty string
  (`--principal-category ""`) disables the label.
- `--escrow` repeatable, `Category=Amount` (parent:sub category paths as
  elsewhere in the CLI).
- `--auto-post` opt-in, default off; `--lead-days` as on `scheduled add`.
- Creation is atomic, mirroring the wizard.
- Posting needs no new commands: `scheduled list --due` / `scheduled post`
  compute loan splits through the shared service path.
- `loan list`/`loan show` find loans by account type; the schedule is
  located via its principal split's transfer target (`—` columns when
  no loan-shaped schedule exists).

## Value Adjustment Category & Asset Revaluation

Seed a **system** category `Value Adjustment` so revaluations are
excluded from spending reports (which already exclude system
categories — `internal/report/report_service.go:193`) and protected
from rename/delete/merge like the system `Transfer` category.

Two gaps in the existing machinery must be handled explicitly:

- **Picker visibility**: system categories are excluded from every TUI
  category picker (`buildCategoryOptions`,
  `internal/tui/transaction_dialog.go:152`), which would make the
  category unusable in the TUI. Extend the picker plumbing so that when
  the dialog's account is of type **`asset` specifically** (`Type ==
  TypeAsset` — *not* the broader `IsAssetType()` helper, which includes
  checking/savings/cash/investment), the picker additionally offers the
  `Value Adjustment` system category. The system `Transfer` category
  stays engine-assigned and hidden everywhere. Wire through the
  transaction dialog and scheduled dialog. The CLI resolves categories
  by name without a system filter and needs no change.
- **Seeding**: the `EnsurePaycheckCategories` on-open pattern creates
  *non-system* categories and silently skips existing names, so it
  cannot be reused verbatim. New helper: create the category **with**
  the system flag when the name is free; if a *user* category named
  "Value Adjustment" already exists, leave it untouched and surface a
  one-time notice (log + toast) that spending-report exclusion will not
  apply to it.

Recommended flows (documented; no new engine):

- **Home value update**: transaction in the asset register for the
  delta, category Value Adjustment.
- **Car depreciation**: a plain monthly scheduled transaction on the
  asset account (fixed amount, category Value Adjustment) — the
  scheduled engine already allows asset-account targets.

A dedicated "Update value…" affordance (enter new total, tmoney writes
the delta) is a possible v2 on top of the same category.

## Edge Cases

- **Extra principal (one-off)**: an ad-hoc transfer funding→loan any
  time; every later computed split and the projection adapt
  automatically.
- **Extra principal (recurring)**: raise the P&I payment via
  **Edit as loan** (never the generic editor — see *Demotion guard*);
  recompute books the surplus to principal automatically.
- **Skipping an occurrence**: skip means that month's interest accrual
  is **never charged** — the register ends a full month of interest
  (~$1,700 on a $312k @ 6.5% loan) richer than the servicer's records,
  and the projection turns optimistic. For a payment made late, post
  with an edited date instead of skipping. True deferral/forbearance
  (interest capitalization) is out of scope; the honest workaround is a
  manual interest-only transaction matching the servicer statement.
- **Rate change (ARM/refi-lite)**: edit the loan account's APR (TUI
  account edit); future posts use it. The projection view reflects it
  immediately. (No CLI account-edit exists today; noted as a known gap,
  out of scope here.)
- **0% APR**: payment prefill = ceil(P/n); no interest line (see
  *0% APR loans*); raising the rate later triggers the typed
  add-an-interest-line error path.
- **Servicer rounding drift**: reconcile pennies in the post-time
  preview; principal is the plug line, so adjusting interest by a cent
  and principal by the opposite cent keeps the total.
- **Undo/redo determinism**: redo of a posted loan payment must
  re-create the stored rows verbatim, never recompute from the
  (possibly changed) live balance. Where this binds: the preview path's
  command already stores and replays its transaction+splits; the plain
  post command currently re-executes `svc.Post` on redo and must be
  converted to store-and-replay before loan schedules use it. (The
  auto-post undo command's redo is already a no-op — a pre-existing
  quirk, out of scope.)
- **Same-date extra principal + payment**: both dated the occurrence
  date; computation includes any transaction dated ≤ occurrence date
  that is already saved. Order of same-day entry can shift pennies —
  preview tweaks cover it.
- **Currency**: loan and asset accounts inherit the funding account's
  currency; mixed-currency loans are out of scope (consistent with
  transfers generally).
- **Closed accounts**: existing rules apply unchanged (schedules
  referencing closed accounts refuse to post and are skipped by
  auto-post; the payoff toast suggests closing the loan only after it
  reaches zero).

## Out of Scope Summary

Biweekly/other frequencies · historical backfill · negative
amortization support · automatic ARM schedules · escrow analysis ·
deferral/forbearance · depreciation curves · loan↔asset link column ·
daily interest accrual / mid-cycle proration · CLI account editing
(pre-existing gap) · balloon-payment modeling · "Update value…"
affordance (v2).

**Forward compatibility — now delivered**: transfer lines may carry an
*optional* category, and the loan wizard labels the principal transfer
line `Loan:Principal` by default (see the *Principal* bullet above). As
promised, loan-shape detection was unaffected — it keys on the principal
split's transfer target, not on the absence of a category — and existing
schedules adopt the label via **Edit as loan →** rather than a data
backfill. The delivered feature made its own decisions (one shared
category mirrored across legs, opt-in `--include-transfers` spending,
the relaxed split CHECK in migration 029, QIF lossiness): see
[`specs/transfer-categories.md`](transfer-categories.md).
