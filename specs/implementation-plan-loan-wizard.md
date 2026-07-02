# Implementation Plan: Loan Wizard

Feature spec: [`specs/loan-wizard.md`](loan-wizard.md)

Each phase is independently shippable, TDD'd (tests written with the
phase), gated on `go build ./... && go test ./...` + `gofmt`, and
committed directly to `main` before the next phase starts.

## Status Legend

- [ ] Not started
- [~] In progress
- [x] Complete

## Priority Rationale

The sign-convention fix ships first: it is a real, user-visible bug
today (net worth overstated for anyone with credit-card debt), and every
later phase's arithmetic assumes the standardized convention. The pure
math engine lands before any schema or UI so its behavior is pinned by
unit tests early. Schema (`loan_section`) precedes the service work that
reads it; the wizard precedes the views that assume wizard-created
schedules exist; CLI comes last since it reuses everything.

## Phase 1: Liability Sign Standardization — [ ]

The one behavior-change phase; everything else is additive.

- [ ] `internal/report/report_service.go`: `netWorthAsOf` — net worth =
      `totalAssets + totalLiabilities` over **signed** balances
      (liabilities ≤ 0). Decision (locked): the report struct carries
      signed values; every presentation layer applies the **negation**
      display rule from the spec.
- [ ] TUI dashboard (`internal/tui/dashboard_view.go`,
      `renderAssetLiabilityColumns`): render liability rows and the
      liabilities total **negated** under the LIABILITIES heading
      (credit balances therefore display negative — correct).
- [ ] TUI Reports → Net Worth view (`internal/tui/reports_view.go`):
      same negation rule — this view renders the same NetWorth struct
      and is easy to miss.
- [ ] CLI `report net-worth` (`internal/cli/report/format.go`): same
      negation rendering in the LIABILITIES section; `~` estimated
      markers for investment values unchanged.
- [ ] CLI `account balance` (`internal/cli/account/format.go`): math
      already correct (`Add`); leave the flat per-account list signed.
      Reword the net-worth comment to reference the standardized
      convention.
- [ ] `specs/accounts.md`: fix the sign table (loan, credit card:
      negative = owed, positive = overpayment/credit); note the
      convention is load-bearing for transfers-as-principal-payments.
- [ ] `specs/reconciliation.md`: add a liability example (statement
      shows positive owed → enter negated).
- [ ] Tests: net worth with mixed asset/liability accounts (signed
      inputs → correct total; negated display; mixed-sign liabilities
      with one credit-balance card); regression test that a credit card
      with purchases lowers net worth; `--include-closed` still valued.
- [ ] README note + release-note blurb: users who entered loan balances
      positive must flip the opening-balance sign once.

## Phase 2: `Value Adjustment` System Category — [ ]

- [ ] New seed helper (the `EnsurePaycheckCategories` pattern creates
      *non-system* categories and cannot be reused verbatim): on open,
      create system category `Value Adjustment` (expense-classified,
      like system `Transfer`) **iff no category with that name exists**.
      If a user category with that name exists, leave it untouched and
      surface a one-time notice (log + status-bar toast) that
      spending-report exclusion will not apply.
- [ ] Picker plumbing: extend `buildCategoryOptions`
      (`internal/tui/transaction_dialog.go:152`) so dialogs whose
      account is of type **`asset` specifically** (`Type == TypeAsset`,
      *not* `IsAssetType()`, which includes checking/savings/cash)
      additionally offer the `Value Adjustment` system category — the
      system `Transfer` category stays hidden everywhere. Wire through
      the transaction dialog and scheduled dialog loaders. Without this
      the category is unusable in the TUI (system categories are
      excluded from every picker today).
- [ ] Idempotent on every open; created for existing databases too.
- [ ] Add `Loan` parent + `Interest` child to `DefaultCategories`
      (regular, non-system) so **new** files carry the loan-interest
      default from day one; existing files get it via the wizard/CLI
      get-or-create at save time (Phases 7/9) — no on-open seed needed
      for it.
- [ ] Tests: seeded on new + existing files; collision leaves user
      category untouched + notices; spending report excludes it; asset
      register picker offers it, checking register picker does not;
      CLI `transaction add --category "Value Adjustment"` works.
- [ ] Docs: README blurb — asset revaluation / straight-line car
      depreciation recipe (manual entry or plain scheduled transaction
      on the asset account, category Value Adjustment).

## Phase 3: Loan Math Engine (`internal/loan`) — [ ]

Pure functions on `alpacadecimal.Decimal` (the established pattern —
cf. `internal/investment/computation.go`; `Decimal.Round` provides
half-away-from-zero, which equals round-half-up for these non-negative
magnitudes). No DB, no UI. Pin the rounding behavior with tests.

Terminology per spec: `piPayment` is P&I only; escrow never enters the
split math.

- [ ] `MonthlyRate(apr)` → APR/100/12 at full precision (never
      pre-rounded; only final interest figures are rounded).
- [ ] `Payment(principal, apr, termMonths)` → amortization formula
      rounded half-up to cents; `r = 0` branch → `ceil_to_cent(P/n)`.
- [ ] `SplitPayment(owed, apr, piPayment)` →
      `(interest, principal, final bool, err)`:
      - interest = round_half_up(owed × r, 2)
      - principal = piPayment − interest
      - clamp: principal > owed → principal = owed, final = true
      - err on principal ≤ 0 with owed > 0 (negative amortization)
- [ ] `Projection(owed, apr, piPayment, escrowTotal, nextDate, dayOfMonth)`
      → rows `{n, date, totalDraft, interest, principal, escrow, balanceAfter}`
      until balance = 0, capped at **1,200 rows with an explicit
      `Truncated bool`** (the negative-am guard admits $1/month
      principal, so the cap is reachable); reuse the scheduled engine's
      month-end day-of-month semantics (31st → shorter months).
- [ ] `RemainingStats(projection)` → payments left, payoff date, total
      interest remaining; when `Truncated`, payoff date and interest
      remaining are flagged unknown (rendered `100y+` by callers).
- [ ] Table-driven tests: known amortization fixtures (verify 380k @
      6.5% / 360mo against an independently computed value), 0% loans
      (ceil prefill; no n+1st penny payment), one-payment-left clamp,
      penny rounding accumulation across a full projection ends at
      exactly zero, negative-am error, truncation flag, escrow
      pass-through (never double-subtracted).

## Phase 4: `loan_section` Schema + Model Plumbing — [ ]

- [ ] Migration `0NN_loan_section.sql`: add nullable `loan_section TEXT`
      to **`scheduled_split_items` only** (posted transactions carry no
      tag — the never-populated `transaction_splits.paycheck_section`
      column is a cautionary precedent, not one to copy), with CHECKs
      (`IN ('interest','principal','escrow')`;
      `paycheck_section IS NULL OR loan_section IS NULL`). Use the
      DuckDB **table-recreate pattern** from
      `020_paycheck_section.sql` / `026_drop_transaction_splits_fk.sql`.
- [ ] Model field: `LoanSection types.NullableString` on
      `scheduled.Split`; read/write in
      `internal/scheduled/split_repository.go`.
- [ ] Migration tests (`internal/db/migration_test.go` pattern) +
      round-trip repo tests + CHECK violation tests (bad enum value;
      both sections set).

## Phase 5: Loan-Shaped Schedules in the Scheduled Service — [ ]

The recompute engine. All posting paths converge here.

- [ ] **Due-list repair (pre-existing zombie bug, ships first in this
      phase)**: `ListDue` filters only on `next_date`
      (`internal/scheduled/scheduled_repository.go:226`), so completed
      schedules surface as *due* forever in the TUI due section,
      due-count badge, and `scheduled list --due`, while Post and Skip
      refuse with `CompletedError`. Exclude completed schedules from
      the due queries/consumers. Regression test with a naturally
      exhausted fixed-occurrence schedule.
- [ ] `scheduled.IsLoanShaped(st, accountLookup)` per the spec's strict
      detection (monthly with **interval 1 and no semi-monthly
      secondary day**; every split tagged; exactly one principal
      transfer→active-loan; **at most one** interest; parent fixed) and
      `IsLoanAdoptable` (loose shape, same cadence rule) for the TUI
      affordance.
- [ ] As-of balance helper: loan balance as of a date
      (`opening_balance + Σ non-void txns ≤ date`) — reuse/extract the
      report service's as-of query rather than duplicating SQL.
- [ ] `Service.ComputeLoanSplits(st, occurrenceDate)` → adjusted parent
      amount + splits. P&I payment **derived** as parent magnitude −
      Σ escrow-tagged lines (never stored separately). Wraps
      `internal/loan.SplitPayment` with APR lookup from the loan
      account. Typed errors: paid off (`owed ≤ 0`), negative-am,
      **NULL APR** ("no interest rate set" — never silent 0%),
      **missing interest line while computed interest > $0.00** ("open
      Edit as loan to add one" — the guard must NOT fire when computed
      interest rounds to zero, or a nearly-paid loan becomes
      unpostable). $0.00 computed lines omitted from output.
- [ ] Wire into `buildMultiLineTransaction` (used by `Post`,
      `PostWithDate`, **and** `AutoPost`): loan-shaped → computed splits
      and computed parent amount (clamped final payment shrinks the
      posted total; the template is untouched).
- [ ] **Auto-post isolation**: loan-computation errors skip that
      schedule with a reason (the existing closed-account
      skip-with-reason mechanism) — they must never abort the rest of
      the auto-post batch.
- [ ] Payoff completion — on **every** posting path including
      `PostWithEdits`: after writing the post, if loan balance **≥ 0**,
      mark the schedule completed. The paid-off **refusal** (manual)
      and **skip** (auto-post) also mark it completed — `owed ≤ 0` is
      terminal, and there is no other user-facing complete-schedule
      affordance after an ad-hoc payoff transfer. Mechanism: set
      `occurrences_remaining = 0`, backfilling `occurrences = 1` on an
      indefinite schedule (validation requires a positive value ≥
      remaining) — explicitly **not** the `end_date` trick
      (AdvanceSchedule-ordering trap; first-occurrence payoff violates
      `end_date > start_date` validation and bricks editing).
- [ ] **Undo/redo determinism**: redo re-creates stored rows verbatim,
      never recomputes from the live balance. Binding sites: the
      preview-path command (`PostScheduledTransactionWithEditsCommand`)
      already stores and replays; convert the plain
      `PostScheduledTransactionCommand` (whose redo currently
      re-executes `svc.Post`) to store-and-replay. `AutoPostCommand`'s
      redo is already a no-op — pre-existing quirk, out of scope.
- [ ] Tests: computed splits across several months (balance drops ⇒
      interest drops), extra-principal transfer mid-stream changes next
      split, multi-overdue auto-post posts sequentially compounding
      correctly, clamp+complete on final payment, completion via
      PostWithEdits with penny-overshoot (balance slightly positive),
      paid-off refusal (manual) and skip-with-reason (auto-post batch
      continues) both mark the schedule completed, 0%-loan posting (no
      interest line), nearly-paid loan whose computed interest rounds
      to $0.00 posts successfully without an interest line, APR raised
      on a 0% schedule → typed error, NULL-APR typed error, negative-am
      refusal, redo determinism, generic multi-line schedules
      completely unaffected.

## Phase 6: Post-Time Preview Integration (TUI) — [ ]

- [ ] Preview seeding: for loan-shaped schedules, seed the preview's
      lines from `ComputeLoanSplits` (at the schedule's `NextDate`)
      instead of the raw template
      (`internal/tui/scheduled_preview_dialog.go` /
      `transactionSplitsFromScheduled` seam,
      `internal/tui/scheduled_dialog.go:45`).
- [ ] **Reseed rule** (per spec): editing the preview's Date recomputes
      the seed for the new date *until* any line amount is edited;
      after that, user values win and date changes stop reseeding.
- [ ] User edits win: submission continues through `PostWithEdits`
      unchanged — no recompute after the user touches lines.
- [ ] Payoff toast after a completing post (including penny-tweaked
      edits that overshoot): "Loan paid off — close the account from
      the Accounts menu when ready."
- [ ] Tests: seeded values match engine output; date-edit reseeds; line
      edit freezes reseeding; edited values post verbatim; toast on
      payoff via the preview path.

## Phase 7: Loan Wizard (TUI) + Round-Trip — [ ]

The big UI phase; follow the paycheck wizard integration pattern
(`internal/tui/paycheck_wizard.go` — app-level overlay, key routing,
mouse, async data load, submit; its touch points outside the wizard file
are ~10 small edits across menubar/app_menu/app/app_update/app_view/
app_mouse/app_helpers).

- [ ] `internal/tui/loan_wizard.go`: three-section form per the spec
      (Loan / Payment / Asset). Prefill-only fields (original
      principal, open date, term) optional and never stored;
      payment-amount prefill from `internal/loan.Payment` (recomputed
      while the field is untouched); interest-category field required
      iff APR > 0, defaulting to `Loan:Interest` (get-or-created at
      save time — parent `Loan`, child `Interest`); escrow add/remove
      rows; pickers with inline category creation; validation per spec.
- [ ] Menu: **Accounts → New Loan…** (`MenuActionNewLoan` in
      `internal/tui/widget/menubar.go`, dispatch in
      `internal/tui/app_menu.go`).
- [ ] Save: **one atomic DB transaction + one undo command** — loan
      account (opening balance = −current balance, `interest_rate`,
      opening-date rule per spec) → optional asset account → payee
      get-or-create → indefinite monthly multi-line schedule. The
      month-one snapshot (parent and lines) is the `ComputeLoanSplits`
      output for the next payment date — **including the final-payment
      clamp** (a loan created with one payment left stores a clamped
      parent its lines sum to) and the interest-line omission when
      month-one interest is $0.00. Failure anywhere rolls back
      everything.
- [ ] **Edit as loan →** in the Edit Series dialog for loan-shaped
      *and loan-adoptable* schedules (parity with `Edit as paycheck →`,
      `internal/tui/scheduled_dialog.go:335`): edit-mode form omits the
      prefill-only fields; prompts for interest category when adding a
      missing interest line (APR raised from 0); save rewrites the
      template snapshot + account edits as one atomic, single-undo
      operation; adoption path (re)writes tags on untagged schedules.
- [ ] **Demotion guard**: saving a loan-shaped schedule through the
      generic split editor warns ("converts to a generic schedule —
      payments will no longer compute interest automatically.
      Continue?") before stripping tags.
- [ ] Tests: form → created records (accounts, schedule, tags, signs),
      atomic rollback on induced failure, single undo restores
      everything, mid-life vs new-loan opening-date rule, prefill math
      + optionality, 0% APR (no interest line; category field hidden),
      creation with one payment left (clamped snapshot validates),
      validation errors, round-trip open/edit/save, adoption of a
      hand-built untagged loan schedule, demotion warning flow, undo of
      Edit-as-loan restores both account and template together.

## Phase 8: Amortization View (TUI) — [ ]

- [ ] `internal/tui/amortization_view.go`: drill-in from a loan
      account's register on **`a`** (verified free; do *not* fall back
      to `v`, which is taken by void), Esc returns. Header stats +
      remaining-payments table per spec, computed live via
      `internal/loan.Projection`.
- [ ] Truncated projections render `100y+` for payoff date / interest
      remaining (never the cap row as payoff).
- [ ] Graceful no-schedule state (stats it can compute + wizard/adopt
      hint).
- [ ] Help overlay + README keybinding table entries.
- [ ] Tests: render snapshot with fixture loan; clamp row; truncated
      state; no-schedule state.

## Phase 9: CLI `loan` Group — [ ]

- [ ] `internal/cli/loan/`: `add.go`, `list.go`, `show.go` following
      the Cobra noun-verb layout (`specs/cli-package-split.md`).
- [ ] `loan add`: flags/requiredness per spec (`--payment` required
      unless `--principal` + `--term-months` given, then computed and
      printed; `--interest-category` optional, defaulting to
      `Loan:Interest` get-or-created like the wizard);
      `--escrow Category=Amount` repeatable; atomic creation through
      the same shared service code as the TUI wizard (no logic in the
      CLI layer).
- [ ] `loan list`: loans with balance (negated display), APR, P&I
      payment, next date, payoff date, interest remaining (`—` when no
      loan-shaped schedule exists; `100y+` when truncated).
- [ ] `loan show <name>`: account details + projection table
      (`--limit` default 12, `--all`).
- [ ] Tests: golden-output tests per existing CLI test conventions;
      `scheduled post` on a wizard-created schedule computes splits
      (integration); 0% loan add; error cases (missing payment inputs,
      missing interest category).
- [ ] `specs/cli.md` + README CLI reference sections.

## Phase 10: Docs + End-to-End Verification — [ ]

- [ ] README: Features section (Loans), TUI keybindings, menu tables,
      CLI examples.
- [ ] `specs/multiline-splits-and-paycheck.md`: update **all three**
      invalidated claims — the "mortgage wizard out of scope" note (it
      now exists), "multi-line scheduled transactions are TUI-only"
      (`loan add` creates one from the CLI), and "`scheduled post`
      uses template values verbatim" (loan-shaped schedules recompute).
- [ ] Cross-link `specs/loan-wizard.md` from `specs/accounts.md` and
      `specs/scheduled-transactions.md`.
- [ ] End-to-end smoke on a scratch `.tdb`: wizard-create a mid-life
      mortgage with escrow + asset → `loan show` → post two months via
      preview (tweak a penny) → extra-principal transfer → verify next
      split shrinks interest → `report net-worth` sanity (asset − loan,
      negated display) → drive a small fixture loan to payoff → clamp +
      completion + due-list clean + close account → 0% car loan
      round-trip.
- [ ] Purge any stray `zz_`/`probe_` files before the final commit
      (`git status` sweep — never `git add -A`).

## Out of Scope (tracked for later)

- TUI "Update value…" affordance on asset registers (v2 of revaluation).
- `loan set-rate` / CLI `account edit` (pre-existing CLI gap).
- Biweekly frequency; escrow analysis; ARM schedules; backfill;
  deferral/forbearance.
- Equity report (asset + linked loan) — would motivate a loan↔asset link.
