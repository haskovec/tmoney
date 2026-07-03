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

## Phase 1: Liability Sign Standardization — [x]

The one behavior-change phase; everything else is additive.

- [x] `internal/report/report_service.go`: `netWorthAsOf` — net worth =
      `totalAssets + totalLiabilities` over **signed** balances
      (liabilities ≤ 0). Decision (locked): the report struct carries
      signed values; every presentation layer applies the **negation**
      display rule from the spec.
- [x] TUI dashboard (`internal/tui/dashboard_view.go`,
      `renderAssetLiabilityColumns`): render liability rows and the
      liabilities total **negated** under the LIABILITIES heading
      (credit balances therefore display negative — correct).
- [x] TUI Reports → Net Worth view (`internal/tui/reports_view.go`):
      same negation rule — this view renders the same NetWorth struct
      and is easy to miss. (Verified: it delegates to the shared
      `renderAssetLiabilityColumns`, so the dashboard fix covers it;
      fixture tests added for both views.)
- [x] CLI `report net-worth` (`internal/cli/report/format.go`): same
      negation rendering in the LIABILITIES section; `~` estimated
      markers for investment values unchanged.
- [x] CLI `account balance` (`internal/cli/account/format.go`): math
      already correct (`Add`); leave the flat per-account list signed.
      Reword the net-worth comment to reference the standardized
      convention. (Also fixed the stale `IsLiabilityType` doc comment
      in `internal/account/account.go`, which stated positive = owed.)
- [x] `specs/accounts.md`: fix the sign table (loan, credit card:
      negative = owed, positive = overpayment/credit); note the
      convention is load-bearing for transfers-as-principal-payments.
      (Also updated `specs/reports.md` — formula, classification table,
      calc block, display note — `specs/transactions.md` liability-sign
      paragraph, and `specs/cli.md` reconcile-start liability example.)
- [x] `specs/reconciliation.md`: add a liability example (statement
      shows positive owed → enter negated).
- [x] Tests: net worth with mixed asset/liability accounts (signed
      inputs → correct total; negated display; mixed-sign liabilities
      with one credit-balance card); regression test that a credit card
      with purchases lowers net worth; `--include-closed` still valued.
- [x] README note + release-note blurb: users who entered loan balances
      positive must flip the opening-balance sign once.

## Phase 2: `Value Adjustment` System Category — [x]

- [x] New seed helper (the `EnsurePaycheckCategories` pattern creates
      *non-system* categories and cannot be reused verbatim):
      `Service.EnsureValueAdjustmentCategory()`
      (`internal/category/category_service.go`) creates system category
      `Value Adjustment` (expense-classified, like system `Transfer`)
      **iff no top-level category with that name exists**. Returns a
      `userCollision bool`: if a *user* category with that name exists,
      it is left untouched and the boolean is set so the TUI surfaces a
      one-time notice (log + status-bar toast,
      `internal/tui/value_adjustment_notice.go`) that spending-report
      exclusion will not apply. The notice is gated on a new
      `config.ValueAdjustmentNoticeShown` flag so it fires once across
      sessions. A `GetValueAdjustmentCategory()` accessor mirrors
      `GetTransferCategory`. Name constants (`TransferCategoryName`,
      `ValueAdjustmentCategoryName`) added in `internal/category`.
- [x] Picker plumbing: `buildCategoryOptions` refactored into a wrapper
      over `buildCategoryOptionsFor(cats, includeValueAdjustment)` plus
      `buildCategoryOptionsForAccount(cats, *account.Account)`
      (`internal/tui/transaction_dialog.go`). The exception is
      **name-scoped** to `Value Adjustment` and gated on
      **`Type == TypeAsset` specifically** (*not* `IsAssetType()`), so
      `Transfer` stays hidden everywhere. Wired through the transaction
      dialog (fixed sidebar account), the scheduled dialog (account is
      user-mutable → `refreshSchedCategoryOptionsForAccount` rebuilds
      the combo on account change, preserving selection), and the
      post-time preview dialog (keyed on the template's account, so a
      value-adjustment line survives an edit-at-post rather than
      reverting to `(None)`). Split rows and the paycheck wizard keep
      the default (VA hidden).
- [x] Idempotent on every open; created for existing databases too —
      wired best-effort in `internal/app/registry.go` right after
      `EnsurePaycheckCategories` (runs for both TUI and CLI). The
      collision bool is carried on `Services.ValueAdjustmentUserCollision`.
- [x] Add `Loan` parent + `Interest` child to `DefaultCategories`
      (regular, non-system). **Note:** `SeedDefaultCategories` has *no*
      production caller today (only `EnsurePaycheckCategories` runs on
      open), so this entry does not by itself seed new files — the real
      provisioning is the wizard/CLI get-or-create at save time
      (Phases 7/9). The `DefaultCategories` entry keeps a single source
      of truth and feeds the seed-based tests. (Fixing the dormant
      `SeedDefaultCategories` wiring is out of scope for this phase.)
- [x] Tests: `EnsureValueAdjustmentCategory` seeded on fresh DB +
      idempotent + collision-preserves-user-category
      (`category_service_test.go`); `GetValueAdjustmentCategory` +
      Transfer/VA-not-confused; `DefaultCategories` contains VA (system)
      and `Loan:Interest`; spending report excludes VA
      (`report_service_test.go`); `buildCategoryOptionsFor` /
      `buildCategoryOptionsForAccount` — asset offers VA, checking /
      investment / loan / nil do not, Transfer always hidden; collision
      notice surfaced once via `NewApp` + suppressed when already shown
      (`value_adjustment_notice_test.go`); `accountIsAssetByID`; CLI
      `transaction add --category "Value Adjustment"` resolves the
      on-open-seeded system category (`cli/transaction/add_test.go`).
- [x] Docs: README Categories blurb — asset revaluation / straight-line
      car depreciation recipe (manual entry or plain scheduled
      transaction on the asset account, category Value Adjustment).

## Phase 3: Loan Math Engine (`internal/loan`) — [x]

Pure functions over `types.Money` / `types.Date` (doing the intermediate
arithmetic on `alpacadecimal.Decimal`), the established pattern —
cf. `internal/investment/computation.go`; `Decimal.Round` provides
half-away-from-zero, which equals round-half-up for these non-negative
magnitudes. No DB, no UI. Rounding behavior pinned by tests.

Terminology per spec: `piPayment` is P&I only; escrow never enters the
split math.

- [x] Added `Money.Decimal()` accessor and `NewMoneyFromDecimal`
      constructor to `internal/types` (mirroring `Quantity.Decimal()`) so
      the engine can multiply/divide a Money by a non-integer factor and
      round-trip back; used by the engine and every later phase.
- [x] `MonthlyRate(apr)` → APR/100/12 at the library's division precision.
      Backs the `Payment` closed form; the **authoritative** interest
      split does **not** route through this pre-rounded factor (see below).
- [x] `Payment(principal, apr, termMonths)` → amortization formula rounded
      half-up to cents; `r = 0` branch → `ceil_to_cent(P/n)` (`RoundCeil`);
      integer power via exact exponentiation-by-squaring (`powInt`), not
      the approximate `Decimal.Pow`.
- [x] `SplitPayment(owed, apr, piPayment)` →
      `(interest, principal, final bool, err)`:
      - interest = round_half_up(owed × APR / 1200, 2) — **multiply before
        dividing** so the rate is never pre-rounded, per the spec's
        Interest Convention (a pre-rounded rate scaled by the balance
        understated exact half-cent ties by a cent; caught in review).
      - principal = piPayment − interest
      - clamp: principal > owed → principal = owed, final = true
      - `ErrNegativeAmortization` (sentinel) on principal ≤ 0 with owed > 0
- [x] `Project(owed, apr, piPayment, escrowTotal, nextDate, dayOfMonth)`
      → `Projection{Rows []Row, Truncated bool}`, `Row{N, Date, TotalDraft,
      Interest, Principal, Escrow, BalanceAfter, Final}` until balance = 0,
      capped at **1,200 rows with `Truncated bool`** (the negative-am guard
      admits $1/month principal, so the cap is reachable); replicates the
      scheduled engine's month-end day-of-month clamping (31st → shorter
      months). Callers must pass the schedule's explicit day-of-month (the
      wizard always stores one); the legacy roll-forward for a *missing*
      day-of-month is intentionally not reproduced (documented).
- [x] `RemainingStats(projection)` → payments left, payoff date, total
      interest remaining; when `Truncated`, payoff date and interest
      remaining are left unknown (Truncated propagated; callers render
      `100y+`).
- [x] Table-driven tests: amortization fixtures verified against
      independently computed values (380k @ 6.5% / 360mo → 2401.86, 360
      rows, 484,667.97 interest; 312450.22 @ 6.5%; 32k @ 5.9% / 60mo →
      617.16 needing a 61st clamped payment), 0% loans (ceil prefill; no
      n+1st penny payment), one-payment-left clamp, penny accumulation ends
      at exactly zero, negative-am error, truncation flag + stats, escrow
      pass-through (never double-subtracted), month-end / leap-year date
      clamping, exact half-cent interest ties round up, paid-off / empty
      projections. `MonthlyRate` precision and `powInt` pinned.

## Phase 4: `loan_section` Schema + Model Plumbing — [x]

- [x] Migration `028_loan_section.sql`: adds nullable `loan_section TEXT`
      to **`scheduled_split_items` only** (posted transactions carry no
      tag — the never-populated `transaction_splits.paycheck_section`
      column is a cautionary precedent, not one to copy), with CHECKs
      (`IN ('interest','principal','escrow')`;
      `paycheck_section IS NULL OR loan_section IS NULL`). Uses the
      DuckDB **table-recreate pattern** from
      `020_paycheck_section.sql` / `026_drop_transaction_splits_fk.sql`
      (INSERT…SELECT carries existing `paycheck_section` values through,
      defaults `loan_section` to NULL; index recreated).
      `CurrentSchemaVersion` bumped 27 → 28.
- [x] Model field: `LoanSection types.NullableString` on
      `scheduled.Split`; read/write in
      `internal/scheduled/split_repository.go` (wired through
      `splitColumns`, `Create`, `Update`, `scanSplits` — parent
      `loadSplits` inherits it via the shared scan path).
- [x] Migration tests (`internal/db/migration_test.go` pattern) +
      round-trip repo tests + CHECK violation tests (bad enum value;
      both sections set; `transaction_splits` gains no such column).
      Round-trip repo tests are mutation-based across `Update` (re-tag
      and clear-to-NULL) so a dropped `Update` write is actually
      caught — verified by an adversarial mutation probe.

## Phase 5: Loan-Shaped Schedules in the Scheduled Service — [x]

The recompute engine. All posting paths converge here.

- [x] **Due-list repair (pre-existing zombie bug, ships first in this
      phase)**: `ListDue`
      (`internal/scheduled/scheduled_repository.go`) now excludes
      completed schedules — `occurrences_remaining IS NULL OR > 0` and
      `end_date IS NULL OR next_date <= end_date`, mirroring
      `Transaction.IsCompleted`. Previously a completed schedule with a
      past `next_date` surfaced as *due* forever (TUI due section,
      due-count badge, `scheduled list --due`) while Post/Skip refused
      it with `CompletedError`. Regression tests: naturally-exhausted
      fixed-occurrence schedule and a past-end_date schedule
      (`list_due_test.go`).
- [x] `scheduled.IsLoanShaped(st, AccountLookup)` and `IsLoanAdoptable`
      (`internal/scheduled/loan_shape.go`) per the spec's strict/loose
      detection (monthly, interval 1, no secondary day; every split
      tagged; exactly one principal transfer→active loan account; at
      most one interest; fixed parent). Resolve-failure ⇒ not
      loan-shaped (total function). `LoanSection` constants added.
      Tests: `loan_shape_test.go`.
- [x] As-of balance helpers `account.Repository.BalanceAsOf(id, date)`
      and `Balance(id)` extract the net-worth as-of formula
      (`opening_balance + Σ non-void txns`, signed, parent amounts
      only) as reusable single-account queries. Tests:
      `balance_asof_test.go`.
- [x] `Service.ComputeLoanSplits(st, occurrenceDate)` → `*LoanSplits`
      (parent amount + splits) in `loan_posting.go`. P&I derived as
      parent magnitude − Σ escrow magnitudes; wraps
      `internal/loan.SplitPayment` with APR from the loan account.
      Typed errors `ErrLoanPaidOff`, `ErrLoanNoInterestRate`,
      `ErrLoanMissingInterestLine` (guard skipped when computed interest
      rounds to $0.00), plus propagated `ErrNegativeAmortization`.
      $0.00 interest line omitted. Tests: `loan_posting_test.go`.
- [x] Wired into `buildMultiLineTransaction` via `buildLoanTransaction`
      (covers `Post`, `PostWithDate`, and `AutoPost`): loan-shaped →
      computed splits + computed parent (clamped final shrinks the
      draft; template untouched).
- [x] **Auto-post isolation**: `isLoanComputationError` →
      skip-with-reason (`loanSkipReason`); loan-computation errors never
      abort the batch. Persistence adjusted so a paid-off skip that
      marks the schedule completed is saved.
- [x] Payoff completion on **every** posting path incl. `PostWithEdits`:
      `finalizeLoanPayoff` marks the schedule completed when the loan's
      full balance reaches ≥ 0. The manual paid-off refusal
      (`postMultiLine`) and auto-post skip also complete. Mechanism:
      `Transaction.MarkCompleted` sets `occurrences_remaining = 0`,
      backfills `occurrences = 1`, clears any `end_date` — not the
      `end_date` trick. Tests in `loan_wiring_test.go`.
- [x] **Undo/redo determinism**: added
      `Service.PostReturningSplits`; converted
      `PostScheduledTransactionCommand` to store-and-replay for
      multi-line (replays the captured parent + splits via
      `PostWithEdits` on redo, so a loan schedule never recomputes
      against a since-changed balance); single-line stays re-execute
      (deterministic). `AutoPostCommand` no-op redo left as-is
      (out of scope). Test:
      `TestPostScheduledTransactionCommand_LoanRedoIsDeterministic`.
- [x] Tests: computed splits across months (interest falls / principal
      rises), extra-principal mid-stream lowers next interest,
      multi-overdue auto-post compounds, clamp+complete on final
      payment, PostWithEdits payoff completion, paid-off refusal
      (manual) + skip-with-reason (auto-post batch continues) both
      complete the schedule, zero-rate posting (no interest line),
      nearly-paid loan omits a $0.00 interest line, NULL-APR + missing
      interest line + negative-am typed errors, redo determinism, and
      generic multi-line schedules unaffected.

## Phase 6: Post-Time Preview Integration (TUI) — [x]

- [x] Preview seeding: for loan-shaped schedules, seed the preview's
      lines from `ComputeLoanSplits` (at the schedule's `NextDate`)
      instead of the raw template. Implemented by computing `*LoanSplits`
      in the async loader (`loadSchedulePreviewData`) — gated on a new
      exported `(*scheduled.Service).IsLoanShaped` wrapper — and threading
      it through `schedulePreviewDataMsg` into the (still pure)
      `NewSchedulePreviewDialog`, which seeds the embedded `SplitDialog`
      from `ls.Splits` with `totalAmount = ls.ParentAmount`. The generic
      `transactionSplitsFromScheduled` seam is left unchanged (branch at
      the call site, per the review guidance).
- [x] **Reseed rule** (per spec): editing the preview's Date recomputes
      the seed for the new date (`maybeReseedLoanPreview` →
      `reseedLoanSplits`) *until* any line edit; after that,
      `loanSeedFrozen` sticks and Date edits stop reseeding.
      **Freeze broadened (adversarial-review fix):** the freeze snapshot
      is a per-row *signature* (category / transfer target / amount /
      memo), not amount-only — a category/memo edit also freezes, so a
      later reseed can't silently discard it (the reseed rebuilds the
      whole editor).
- [x] User edits win: a frozen (hand-edited) preview posts the split
      editor's rows verbatim through `PostWithEdits`.
      **Submit made authoritative (adversarial-review fix):** a
      *non-frozen* loan preview recomputes `ComputeLoanSplits` at the
      posting date inside `submitSchedulePreviewDialog` rather than
      trusting the on-screen seed — closing the reseed-refusal desync
      where a Date edit to a paid-off date left stale splits that Save
      would post (and bypassed the paid-off guard). A posting date at
      which the loan is paid off / won't compute is refused with a header
      error, not posted.
- [x] Payoff toast after a completing post (including penny-tweaked
      edits that overshoot): the submit closure re-reads the schedule's
      `IsCompleted()` (gated on loan-shaped) and carries `loanPaidOff` on
      `scheduledPostedMsg`; the synchronous handler shows
      "Loan paid off — close the account from the Accounts menu when
      ready." + `ClearToastCmd`. Also: opening the preview on an
      already-paid-off loan refuses-and-completes (TUI realization of
      "manual post of a paid-off loan") via `schedulePreviewLoanBlockedMsg`
      (checks `Post`'s error so a closed-funding-account failure surfaces
      the real reason, not a false "paid off").
- [x] Tests (`internal/tui/loan_preview_test.go`, 10): seeded values
      match `ComputeLoanSplits` (≠ stale template); date-edit reseeds;
      line-amount edit freezes (direct + via key); memo edit freezes
      (reseed doesn't discard it); clamped final payment posts the
      shrunk parent + completes + payoff toast; paid-off-at-open
      refuses+completes+toast; submit refuses a paid-off posting date;
      submit recomputes at the posting date despite a stale display;
      generic multi-line unaffected (no loanSplits, not loan-shaped).

## Phase 7: Loan Wizard (TUI) + Round-Trip — [~]

The big UI phase; follow the paycheck wizard integration pattern
(`internal/tui/paycheck_wizard.go` — app-level overlay, key routing,
mouse, async data load, submit; its touch points outside the wizard file
are ~10 small edits across menubar/app_menu/app/app_update/app_view/
app_mouse/app_helpers).

**Progress note (2026-07-02):** shipped in two commits — (1) the creation
flow, (2) Edit-as-loan + the demotion guard. Built on the generic
`dialog.Dialog` form widget (like the account dialog) rather than a
paycheck-style custom renderer — far less code, and it reuses the
widget's focus/scroll/mouse/hidden-field machinery. The month-one
snapshot is produced by a new shared, pure `scheduled.BuildLoanSnapshot`
(the creation-time mirror of `ComputeLoanSplits`; Phase 9's CLI reuses
it) rather than calling `ComputeLoanSplits`, which can't run before the
loan account exists. **Only remaining deferral:** escrow uses
progressive-reveal rows (an empty row appears once the prior one has a
category) instead of explicit +/− buttons, and the category pickers are
plain selects — inline category creation is not wired (the default
interest category is still get-or-created at save, so it is always
available). Everything else in this phase is complete and tested.

- [~] `internal/tui/loan_wizard.go`: three-section form per the spec
      (Loan / Payment / Asset). Prefill-only fields (original
      principal, open date, term) optional and never stored;
      payment-amount prefill from `internal/loan.Payment` (recomputed
      while the field is untouched); interest-category field required
      iff APR > 0, defaulting to `Loan:Interest` (get-or-created at
      save time — parent `Loan`, child `Interest`); validation per spec.
      **Done** except: escrow is progressive-reveal rows (not explicit
      add/remove buttons) and pickers have **no** inline category
      creation yet (deferred; plain selects).
- [x] Menu: **Accounts → New Loan…** (`MenuActionNewLoan` in
      `internal/tui/widget/menubar.go`, dispatch in
      `internal/tui/app_menu.go`).
- [x] Save: **one atomic operation + one undo command** — loan
      account (opening balance = −current balance, `interest_rate`,
      opening-date rule per spec) → optional asset account → payee
      get-or-create → indefinite monthly multi-line schedule. The
      month-one snapshot (parent and lines) is `BuildLoanSnapshot`'s
      output for the next payment date — **including the final-payment
      clamp** (a loan created with one payment left stores a clamped
      parent its lines sum to) and the interest-line omission when
      month-one interest is $0.00. Atomicity is at the command layer via
      `undo.CompoundCommand` (DuckDB exposes no cross-service SQL
      transaction), which rolls back already-created records if a later
      step fails — so a failure never strands an orphaned loan account.
- [x] **Edit as loan →** in the Edit Series dialog for loan-shaped
      *and loan-adoptable* schedules (parity with `Edit as paycheck →`).
      The alternate button + action dispatch (`maybeAddEditAsLoanButton`,
      `relaunchScheduledAlternate`, `scheduleWantsLoanEdit`) route to the
      loan wizard vs the paycheck wizard by shape. Edit-mode form
      (`buildEditLoanWizard`) omits the prefill-only fields, the current
      balance, and the schedule's fixed cadence/routing (preserved as-is),
      showing only name/institution/APR + P&I/interest/escrow/auto-post;
      `owed` is the loan's live balance as of the next payment date
      (`account.Service.BalanceAsOf`). Save (`submitEditLoanWizard`)
      rewrites the template snapshot + applies account edits as one
      atomic, single-undo `CompoundCommand` (`EditAccount` +
      `EditScheduledTransaction`); the adoption path re-tags an untagged
      loan-adoptable schedule, promoting it to strictly loan-shaped.
      *Simplification:* a missing interest line (APR raised from 0)
      defaults to `Loan:Interest` via the visible, editable interest
      picker instead of a separate modal prompt.
- [x] **Demotion guard**: saving a loan-shaped schedule through the
      generic split editor — or unchecking Split on it — shows the exact
      spec warning ("…payments will no longer compute interest
      automatically. Continue?") via the confirm dialog before the tags
      are stripped; declining leaves the schedule untouched.
- [x] Tests: form → created records (accounts, schedule, tags, signs),
      atomic rollback on induced failure, single undo restores
      everything, mid-life vs new-loan opening-date rule, prefill math
      + optionality, 0% APR (no interest line; category field hidden),
      validation errors (`loan_wizard_test.go`); the clamped
      one-payment-left snapshot (`scheduled/loan_build_test.go`) and
      `category/loan_interest_category_test.go`; **round-trip
      open/edit/save + undo-restores-both, adoption of a hand-built
      untagged schedule, Edit-as-loan button/dispatch, and the demotion
      warning flow** (`loan_wizard_test.go`).

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
