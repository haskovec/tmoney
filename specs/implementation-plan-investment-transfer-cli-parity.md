# Implementation Plan: Investment Transfer CLI Parity

This document captures the design and implementation plan for bringing
the unified TUI Transfer dialog's investment-cash-transfer behaviors
to the CLI — closing the [Future-A gap][future-a] left open by the
investment-cash-transfer unification work.

After the unification (Phase 1 / Phase 2 of
`implementation-plan-investment-cash-transfer-unification.md`), the
TUI supports cash transfers in all four `(From.Type, To.Type)`
combinations (bank↔bank, bank↔inv, inv↔bank, inv↔inv) plus edit,
delete, and undo. The CLI has only `tmoney transfer add`, and it
rejects any pair that involves an investment account — directing
users to the TUI. This plan removes that rejection by dispatching
`transfer add` internally, adds CLI `transfer edit` and `transfer
delete` commands with the same dispatch shape, and surfaces
transaction IDs in `transaction list`/`transaction search` so
scripts can refer to legs by ID.

[future-a]: implementation-plan-investment-cash-transfer-unification.md#future-a--cli-parity-for-linked-cash-transfers-deferred-from-q8

Mark items as complete with `[x]` as they are finished.

## Status Legend

- `[ ]` Not started
- `[x]` Complete

---

## Background

### Problem

`tmoney transfer add --from X --to Y --amount Z` was hardened in P1-004
of the unification plan to reject any account whose type satisfies
`account.Type.IsInvestmentType()` (`investment` or `hsa`). The
rejection is correct — the underlying `transaction.Service.CreateTransfer`
would otherwise silently mint a malformed regular-table row in an
investment account's ledger — but it leaves CLI users without any
way to perform a linked cash transfer that touches an investment
account. The TUI has the unified Transfer dialog; the CLI just
errors out.

Additionally, the CLI has no `transfer edit` or `transfer delete`
commands at all (for *any* transfer shape — bank↔bank included).
If a CLI user fat-fingers a `transfer add`, the only recovery is to
drop into the TUI and fix the transfer there. This is symmetrical
to the investment gap above but pre-existing and equally limiting.

### Why now

The cash-transfer unification work just closed; the service surface
for all four dispatch paths is stable
(`transaction.Service.CreateTransfer` + the three `investment.Service`
methods `TransferCash` / `DepositFromAccount` /
`TransferCashBetweenInvestments`), and the TUI's `transfer_dialog.go`
already encodes the dispatch logic as a pure helper
(`chooseTransferDispatch(fromType, toType)`). Lifting that helper
into a shared package and adding three CLI commands on top of it is a
small, mechanically clean piece of work that closes a real user gap.

## Goals

- `tmoney transfer add` accepts any `(--from, --to)` account-type
  combination and dispatches internally to the right service method.
  No type-rejection error for investment accounts at the CLI layer.
- `tmoney transfer edit --txn-id <leg-uuid> [--amount … --date …
  --memo … --status …]` updates a whole-transaction transfer pair.
  Editable fields mirror the TUI's Edit mode; From/To are not
  editable (delete-and-recreate to move accounts). Dispatches by
  account type.
- `tmoney transfer delete --txn-id <leg-uuid>` hard-deletes both
  legs of a whole-transaction transfer. Dispatches by account type.
  Reconciled legs block the cascade (matching existing guards).
- `tmoney transaction list` and `tmoney transaction search` gain a
  `--show-ids` flag that adds a leading UUID column for use with
  `transfer edit`/`transfer delete`.
- `tmoney transfer add`'s confirmation output prints the new
  `transfer-id` and both leg transaction-ids (always, no flag).
- TUI behavior unchanged across all the above.
- Hardening guards on
  `transaction.Service.CreateTransfer`/`UpdateTransfer` remain in
  place (defense-in-depth against future internal callers
  bypassing the dispatcher).

## Non-goals

- **`transfer void`**. The investment subsystem has no concept of
  void — `investment.Transaction` carries no `status` enum that
  includes void, `investment.Service.VoidTransaction` does not
  exist, and the TUI investment register only offers Delete on
  investment rows. Adding void to the investment subsystem is its
  own multi-PR initiative (data-model migration + TUI affordance +
  total-return implications); the CLI cannot ship a feature the
  underlying model doesn't support. Tracked as
  [Future-H](#future-h--investment-side-void).
- **`transfer edit` / `transfer delete` on transfer-line paired
  sides** (the multi-line paycheck case). When `--txn-id` resolves
  to a counter-transaction whose `transfer_id` is owned by a
  *split-item* (paycheck → 401k contribution line), the CLI
  refuses with a "this transaction is part of a multi-line split;
  edit/delete the parent in the TUI" error. Multi-line splits have
  a richer UX surface (mini-split tables, imbalance indicators,
  reverse-cascade to parent split structure) that the CLI doesn't
  yet expose. Tracked as
  [Future-J](#future-j--multi-line-split-edit--delete-from-cli).
- **CLI undo**. The CLI runs as one-shot processes; there is no
  session-spanning undo stack today, and the TUI's undo manager is
  process-local. CLI users get explicit `transfer delete`/`transfer
  edit` for recovery instead.
- **Multi-currency / FX**. Matches the unification plan's
  Future-B. The dispatcher does not check currency.
- **Editing From/To on `transfer edit`**. Matches the TUI's
  Edit-mode read-only convention (P1's D3). Edit-time cross-type
  re-dispatch was deferred in Phase 1; same reasoning applies here.
  Users wanting to change accounts run `transfer delete` then
  `transfer add`.
- **Status = reconciled on `transfer edit`**. The reconcile
  workflow owns that transition. `--status reconciled` errors with
  a redirect to `tmoney reconcile`.
- **General `transaction edit` / `transaction delete`** for
  non-transfer transactions. Real CLI gap of the same shape, but a
  separate planning exercise — bundling it would conflate two
  initiatives.
- **`transferlink` (the link-importer) extension to investment-side
  rows**. The `transferlink` package today joins unlinked rows in
  the `transactions` table; an analogous join across
  `investment_transactions` is a separate initiative. Tracked as
  [Future-I](#future-i--transferlink-investment-side).

---

## Design Decisions

These came out of the grilling session that preceded this plan.
Recorded so future readers understand *why* each choice was made.

### D1 — Extend `tmoney transfer add` (don't add a sibling) [Q1]

`tmoney transfer add` becomes the single user-facing CLI command for
all four `(From.Type, To.Type)` combinations. Inside the command
handler, `transfer add` looks up both accounts via `account.Repository`,
inspects their types, and routes to one of:

| From → To | Service method |
|---|---|
| reg → reg | `transaction.Service.CreateTransfer` |
| reg → inv | `investment.Service.DepositFromAccount` |
| inv → reg | `investment.Service.TransferCash` |
| inv → inv | `investment.Service.TransferCashBetweenInvestments` |

**Why one command instead of `tmoney investment transfer-cash` or
overloading `tmoney investment transfer`?**

- Mirrors the TUI's "one unified dialog" principle (D4 of the
  unification plan): same entry point regardless of account types.
- Scripts that change an account from `checking` to `investment`
  don't need to be rewritten — the same `transfer add` invocation
  keeps working.
- The "transfer = regular ledger only" semantic the rejection
  enforced was a Phase 1 stopgap, not a load-bearing invariant.
- `investment transfer` already means *shares*; overloading it
  with mutually-exclusive `--cash` / `--shares` flag groups makes
  for ugly help text and discoverability.

### D2 — Wide scope: add + edit + delete [Q2]

Three commands ship together (across the four PRs): `transfer add`
(extended), `transfer edit` (new), `transfer delete` (new). The
narrow scope (just `transfer add`) was rejected as "too narrow" —
without edit/delete the user has no recovery from a fat-fingered
`transfer add`.

`transfer edit` and `transfer delete` work for *all* transfer
shapes (bank↔bank too), not just inv-involving ones — they close
the broader CLI edit/delete gap as a side effect. The cost is
small because the dispatcher in `transfer add` is reusable.

### D3 — Identifier: leg transaction-id [Q3]

`transfer edit` and `transfer delete` take `--txn-id <leg-uuid>` —
the UUID of one of the transfer's two transactions (either leg).
The command resolves the pair internally by looking up the leg's
`transfer_id` and fetching its partner.

**Why leg-id and not transfer-id?**

- Discovery: scripts use `transaction list --account X` (with the
  new `--show-ids` flag) to find transactions to operate on. The
  IDs surfaced there are transaction-ids, not transfer-ids.
- Single discovery path serves *every* future CLI mutator
  (`transaction edit`, `transaction delete`, `transaction void` by
  ID) — not just transfers.
- A new `transfer list` command (the alternative) duplicates state
  that `transaction list` already exposes, just under a different
  name.

Composite identification (`--from X --date Y --amount Z`) was
rejected for fragility: brittle to duplicates, ugly disambiguation
errors.

### D4 — `transfer edit`: editable fields mirror the TUI [Q4]

Flags on `transfer edit`:

| Flag | Editable? | Default if omitted |
|---|---|---|
| `--amount` | Yes | Unchanged from existing pair |
| `--date` | Yes | Unchanged |
| `--memo` | Yes | Unchanged |
| `--status` | `cleared` \| `uncleared` only | Unchanged |
| `--from` / `--to` | **No** | n/a |

`--status reconciled` is rejected with a redirect to `tmoney
reconcile` (the reconcile workflow owns that transition).

**Why "only-supplied-flags take effect"?** Matches the established
convention set by `tmoney security edit`. The command loads the
existing pair via `transferRepo.GetByTransferID`, fills in the
unchanged values, and dispatches to the right `Update*` service
method.

**Why From/To read-only?** Matches the TUI's Edit-mode read-only
convention (D3 of the unification plan). Edit-time cross-type
re-dispatch was deferred there; same reasoning applies. CLI users
who want to move accounts: `transfer delete` + `transfer add`.

**At least one editable flag required.** Running `transfer edit
--txn-id <id>` with no other flags is a usage error (`"specify at
least one of --amount, --date, --memo, --status"`).

### D5 — `transfer delete` is hard delete [Q5]

`transfer delete --txn-id <leg-uuid>` removes both legs entirely.
Matches the TUI's `d` key in the register and the user's stated
"delete and try again" use case. Reconciled legs block the cascade
(existing guard).

`transfer void` is not provided in this plan — see Non-goals and
[Future-H](#future-h--investment-side-void).

### D6 — `--show-ids` flag on `transaction list` / `transaction search` [Q6a]

`tmoney transaction list` and `tmoney transaction search` gain a
`--show-ids` boolean flag. When set, the output adds a leading
column with each transaction's full UUID. Default off — existing
output is unchanged for casual users.

**Why not always on?** UUIDs are 36 characters; making them the
default leading column pushes the rest of the row off-screen on
80-col terminals.

**Why not short IDs (8-char prefix)?** Rejected for the prefix-
resolver complexity and brittle failure mode ("error: 4 transactions
match prefix `a3c2`"). `--show-ids` opts in to full UUIDs that
work unambiguously.

The flag also applies (for consistency) to `tmoney transaction
search`. It does *not* apply to other list-shaped commands
(`account list`, `security list`, etc.) — they each have their own
identifier semantics; expanding to them is out of scope.

### D7 — `transfer add` always echoes IDs in the confirmation [Q6b]

`transfer add`'s success output gains three new key:value lines:

```
Transfer created successfully!
  Transfer ID:           <uuid>
  From transaction ID:   <uuid>
  To transaction ID:     <uuid>
  From:                  Checking
  To:                    Brokerage
  Date:                  2026-05-25
  Amount:                $500.00
```

No flag gates this — there's no downside to seeing IDs at create
time, and scripts composing `add → edit/delete` need them.

### D8 — Keep the `CreateTransfer`/`UpdateTransfer` rejection guards [Q7a]

`transaction.Service.CreateTransfer` and `UpdateTransfer` continue
to reject investment-type accounts with `NotRegularAccountError`.
The CLI dispatcher routes inv-involving cases away from these
methods *before* they hit the guard, so the guard's user-facing
surface goes away under normal CLI use — but it stays as
defense-in-depth against any future internal caller that bypasses
the dispatcher.

The guard's error message remains developer-facing
(`"use investment.Service for linked cash transfers"`) — it should
only be seen by code, not users.

### D9 — Dispatch helper lives in `internal/transaction/` [Q7b]

The TUI's existing `chooseTransferDispatch(fromType, toType)` helper
+ `transferDispatchKind` enum (RegToReg / InvToReg / RegToInv /
InvToInv) is lifted from `internal/tui/transfer_dialog.go` to a new
file `internal/transaction/dispatch.go`, exported as:

```go
package transaction

type TransferDispatchKind int
const (
    DispatchRegToReg TransferDispatchKind = iota
    DispatchInvToReg
    DispatchRegToInv
    DispatchInvToInv
)

func ChooseTransferDispatch(from, to account.Type) TransferDispatchKind
```

TUI updates to import `transaction.ChooseTransferDispatch`; CLI
imports the same helper. Single source of truth.

**Why `internal/transaction/` and not `internal/account/` or a new
package?** The `transaction` package already owns the
`TransferPair` shape, `CreateTransfer`, the hardening guard, and is
imported by both TUI and CLI. Adding the dispatch helper here keeps
cross-package imports clean and co-locates with the related code.

### D10 — Whole-transaction transfers only; refuse transfer-line paired sides [Q9]

When `--txn-id` resolves to a transaction whose `transfer_id`
matches a *split-item's* `transfer_id` (i.e., it's the paired
single-line side of a multi-line transfer-line split — a paycheck's
401k contribution counter-transaction), `transfer edit` and
`transfer delete` refuse with:

```
transaction <id> is part of a multi-line split (parent: <parent-id>);
to edit or delete transfer-line splits, use the TUI
```

**Why refuse instead of supporting?** The CLI surface should match
the user's mental model: `transfer edit` / `transfer delete` are
the inverses of `transfer add`. Multi-line splits have richer
semantics (reverse-cascade to parent's split structure, parent may
become imbalanced and block subsequent saves) that the CLI doesn't
have UX for. Quietly supporting them would create surprising
side-effects.

The resolver inspects the leg's `transfer_id`:

1. `splitRepo.GetByTransferID(transferID)` — if this returns a
   non-nil parent split, refuse.
2. Otherwise treat as a whole-transaction transfer and proceed.

### D11 — Four PRs, in dependency order [Q8]

1. **P-A1** — Refactor: lift `chooseTransferDispatch` to
   `internal/transaction/dispatch.go`. Pure refactor; no behavior
   change.
2. **P-A2** — Extend `tmoney transfer add` to dispatch by account
   type. Drop the rejection at the CLI layer (guard at service layer
   stays). Echo IDs in confirmation. Update help text and `cli.md`.
   *Unblocks the user's primary need.*
3. **P-A3** — Add `--show-ids` to `tmoney transaction list` and
   `tmoney transaction search`. Cross-cutting read-path enhancement.
4. **P-A4** — `tmoney transfer edit` and `tmoney transfer delete`
   together (share leg-id resolver and dispatcher). Includes the
   final doc sweep (README, transactions.md if anything changed
   there; cli.md additions for the new commands).

Each PR is independently shippable. P-A4 is the largest but bundles
two natural siblings.

---

## Architecture Summary

### CLI command surface (after Phase A)

```
tmoney transfer add                                          [EXTENDED]
  --from X --to Y --amount Z [--date YYYY-MM-DD] [--memo "..."]
  Dispatches by (from.Type, to.Type) to one of:
    transaction.Service.CreateTransfer            (reg → reg)
    investment.Service.DepositFromAccount         (reg → inv)
    investment.Service.TransferCash               (inv → reg)
    investment.Service.TransferCashBetweenInvestments (inv → inv)
  Confirmation prints: transfer-id, from-txn-id, to-txn-id, ...

tmoney transfer edit                                            [NEW]
  --txn-id <leg-uuid>
  [--amount Z] [--date YYYY-MM-DD] [--memo "..."] [--status cleared|uncleared]
  At least one editable flag required.
  Resolves leg → pair → dispatch by account types to:
    transaction.Service.UpdateTransfer       (reg/reg)
    investment.Service.UpdateTransferCash    (any inv-involving)
  Refuses transfer-line paired sides (D10).
  Refuses --status reconciled.

tmoney transfer delete                                          [NEW]
  --txn-id <leg-uuid>
  Resolves leg → pair → dispatch by account types to:
    transaction.Service.DeleteTransfer       (reg/reg)
    investment.Service.DeleteTransaction     (any inv-involving)
      — cascade to other leg via the existing inv↔inv cascade
        landed in the unification plan's P1-005.
  Refuses transfer-line paired sides (D10).
  Refuses reconciled legs (existing guard).

tmoney transaction list                                    [EXTENDED]
  [--show-ids]    Adds leading UUID column.

tmoney transaction search                                  [EXTENDED]
  [--show-ids]    Adds leading UUID column.
```

### Service surface (no new methods)

```
transaction.Service
├── CreateTransfer            [GUARD STAYS]
├── UpdateTransfer            [GUARD STAYS]
└── (DeleteTransfer existing)

investment.Service
├── TransferCash              [existing, P1]
├── DepositFromAccount        [existing, P1]
├── TransferCashBetweenInvestments [existing, P1]
├── UpdateTransferCash        [existing, P1-008]
└── DeleteTransaction         [existing; cascade reaches inv↔inv per P1-005]

internal/transaction (new file)
└── dispatch.go               [NEW: ChooseTransferDispatch helper + enum]
```

### CLI dispatch flow

```
tmoney transfer add --from X --to Y --amount Z
       │
       ▼
  resolve account names → IDs via account.Repository
       │
       ▼
  lookup both accounts; obtain Type for each
       │
       ▼
  kind := transaction.ChooseTransferDispatch(from.Type, to.Type)
       │
       ▼
  switch kind {
    DispatchRegToReg:   svc.Transaction.CreateTransfer(...)
    DispatchRegToInv:   svc.Investment.DepositFromAccount(...)
    DispatchInvToReg:   svc.Investment.TransferCash(...)
    DispatchInvToInv:   svc.Investment.TransferCashBetweenInvestments(...)
  }
       │
       ▼
  print confirmation with transfer-id + leg IDs

tmoney transfer {edit,delete} --txn-id <leg-uuid>
       │
       ▼
  load leg by ID; assert leg.transfer_id is set
       │
       ▼
  splitRepo.GetByTransferID(leg.transfer_id) → if non-nil parent split, refuse (D10)
       │
       ▼
  resolve partner leg: scan transactions and investment_transactions for
  the other row with the same transfer_id
       │
       ▼
  derive (from, to) account types from the two legs
       │
       ▼
  kind := transaction.ChooseTransferDispatch(...)
       │
       ▼
  dispatch to appropriate Update*/Delete* service method
```

---

## Phase A: Plan

Ordered to minimize integration churn — refactor first (zero
behavior change), then user-visible feature, then read-path
enhancement that the new commands need, then the new commands.

### P-A1: Lift dispatch helper to `internal/transaction/`

- [x] **P-A1-001 — Plan doc**
  - This file. GREEN: file exists at
    `specs/implementation-plan-investment-transfer-cli-parity.md`
    with all sections.

- [x] **P-A1-002 — Export `ChooseTransferDispatch` from `internal/transaction/`**
  - RED: `TestChooseTransferDispatch_AllFourCombinations` in
    `internal/transaction/dispatch_test.go` — exercises every
    `(account.Type, account.Type)` combination and asserts the
    correct `TransferDispatchKind`. HSA accounts route via the
    investment branches (covered by their own table entries).
  - GREEN: create `internal/transaction/dispatch.go` with the
    enum + helper as described in D9. Re-implement the existing
    TUI logic verbatim (1:1 case mapping).
  - Refactor TUI: `internal/tui/transfer_dialog.go` deletes its
    local `chooseTransferDispatch` and `transferDispatchKind`,
    imports the exported types from `transaction`. All existing TUI
    tests in `transfer_dialog_test.go` continue to pass.
  - `go test ./...` and `golangci-lint run` clean.

### P-A2: Extend `tmoney transfer add` to dispatch

- [x] **P-A2-001 — Dispatch in `transfer_add.go` (TDD)**
  - RED: four new tests in
    `internal/cli/transfer_add_test.go`, one per dispatch kind
    (`TestTransferAdd_DispatchRegToReg_CreatesPair`,
    `…_DispatchRegToInv_*`, `…_DispatchInvToReg_*`,
    `…_DispatchInvToInv_*`). Each builds a real DB via
    `createTestServices` / `openServices`, runs `runTransferAdd`,
    and asserts that the right service was hit and both legs land
    in the right tables linked by `transfer_id`.
  - RED: `TestTransferAdd_HSACountsAsInvestment` covers HSA on
    either leg.
  - RED: `TestTransferAdd_PrintsIDsInConfirmation` parses the
    captured stdout and asserts the new ID lines.
  - GREEN: rewrite `runTransferAdd` to:
    1. Resolve both account names → accounts.
    2. Compute `kind := transaction.ChooseTransferDispatch(from.Type, to.Type)`.
    3. Switch on `kind` and call the appropriate service method.
    4. Resolve the returned IDs (regardless of which service)
       into the format-agnostic `transferAddResult` struct used by
       the confirmation print helper.
  - GREEN: update `printTransferAddConfirmation` (or inline the
    print code as today) to emit the three new ID lines first.
  - The `--memo` follow-up call to `UpdateTransfer` (today's code)
    is replaced by passing memo straight into the service method
    on each branch — investment-service methods already accept
    memo at create time, so the follow-up is no longer needed for
    the inv-involving paths. For reg/reg keep using
    `UpdateTransfer` to set memo (matches current behavior).
  - Test that the existing `TestTransferAdd_RejectsInvestmentAccount`
    no longer applies and is replaced by the four happy-path tests.
    (The service-layer guard test in
    `internal/transaction/transaction_service_test.go` stays.)
  - `go fix ./... && go fmt ./... && go test ./... && golangci-lint run`
    all clean.

- [x] **P-A2-002 — Help text + cli.md update**
  - `transfer_add.go` cobra command: rewrite the `Short` and `Long`
    text to drop the "non-investment only" framing; describe the
    dispatch instead. Note the inv-involving paths.
  - `specs/cli.md`: rewrite the `transfer add` section to drop the
    rejection callout and add a brief paragraph on the four-way
    dispatch + example invocations for inv-involving cases.
  - `README.md`: in the existing CLI transfer-add reference,
    remove the "both accounts must be non-investment" parenthetical
    and the "CLI parity is deferred" note. Update the Investment
    section similarly (remove the TUI-only restriction).
  - `specs/transactions.md`: rewrite the paragraph immediately
    after the Investment-Account Transfers dispatch table
    (currently lines 164–167). The current text claims the CLI
    already dispatches (false today) and that "each dispatch path
    integrates with the undo manager" (true only for the TUI). New
    text describes:
    - The shared dispatch helper `transaction.ChooseTransferDispatch`
      (introduced in P-A1) used by both TUI and CLI.
    - The TUI's dispatcher in `transfer_dialog.go` with per-path
      undo integration via `internal/undo/`.
    - The CLI's dispatcher in `tmoney transfer add` (no undo —
      explicitly out of scope; recovery is `transfer delete` /
      `transfer edit` in this plan's P-A4).

### P-A3: `--show-ids` on `transaction list` / `transaction search`

- [x] **P-A3-001 — Add `--show-ids` flag**
  - RED:
    `TestTransactionList_ShowIDs_AddsIDColumn` in
    `internal/cli/transaction_list_test.go` — runs the command with
    `--show-ids`, parses output, asserts each row carries the
    transaction's UUID as the first column.
  - RED: same for `TestTransactionSearch_ShowIDs_AddsIDColumn`.
  - RED: `TestTransactionList_DefaultOmitsIDColumn` — without
    the flag, output is byte-identical to current.
  - GREEN: thread a `showIDs bool` field through
    `transactionListOptions` and `transactionSearchOptions`; pass
    to `printTransactionsTable` / `printSearchResults` in
    `format.go`. The print helpers conditionally render an extra
    leading column.
  - `cli.md`: document `--show-ids` on both commands.

### P-A4: `transfer edit` + `transfer delete` + docs

- [x] **P-A4-001 — Leg-id resolver helper**
  - RED: tests for a new pure helper
    `resolveTransferPair(svc, legID) (TransferPairLike, dispatchKind, error)`
    in `internal/cli/transfer_resolve.go` (or inside a CLI utility
    file). Cases: leg-id not found, leg-id is a non-transfer
    (no `transfer_id`), leg-id is a transfer-line paired side
    (returns "multi-line refuse" error), leg-id is a whole-
    transaction-transfer leg (returns pair + kind for all four
    combinations).
  - GREEN: implement the resolver. For the multi-line-refuse check
    it consults `splitRepo.GetByTransferID(leg.TransferID)` — if
    non-nil, return the refuse error. Otherwise, look the partner
    up by scanning both `transactions` (via
    `txnRepo.ListByTransferID`) and `investment_transactions` (via
    `invRepo.ListByTransferID`) and return both legs' shape
    (account-id, type, amount, date, memo, status).

- [x] **P-A4-002 — `tmoney transfer delete`**
  - RED: `internal/cli/transfer_delete_test.go` with one test per
    dispatch kind. Each creates a transfer via the appropriate
    service (or `transfer add`), then runs `runTransferDelete
    --txn-id <leg-id>`, asserts both legs are gone.
  - RED: `TestTransferDelete_RefusesTransferLineSplit` — sets up a
    paycheck-style parent with a 401k transfer-line split, points
    `--txn-id` at the investment-side row, asserts refusal.
  - RED: `TestTransferDelete_RefusesReconciledLeg` — reconciles
    one leg, asserts refusal.
  - GREEN: implement `runTransferDelete` using the resolver +
    dispatch helper. The `DispatchRegToReg` case calls
    `transaction.Service.DeleteTransfer`; the other three call
    `investment.Service.DeleteTransaction` on the investment-side
    leg (the existing inv↔inv cascade handles the other leg).

- [x] **P-A4-003 — `tmoney transfer edit`**
  - RED: per-dispatch-kind tests in
    `internal/cli/transfer_edit_test.go`. Each edits some subset
    of fields and asserts both legs reflect the change.
  - RED: `TestTransferEdit_NoFieldsProvided_Errors` — running with
    only `--txn-id` errors with the "specify at least one of …"
    message.
  - RED: `TestTransferEdit_StatusReconciled_Errors` — `--status
    reconciled` is rejected with the redirect-to-reconcile message.
  - RED: `TestTransferEdit_RefusesTransferLineSplit` — same shape
    as the delete refuse test.
  - GREEN: implement `runTransferEdit` using the resolver +
    dispatcher. The `DispatchRegToReg` case calls
    `transaction.Service.UpdateTransfer(transferID, date, amount,
    memo, status)` with unchanged values filled in from the loaded
    pair. The inv-involving cases call
    `investment.Service.UpdateTransferCash(...)` with the
    appropriate parameters (matching P1-008's signature).
  - Status mapping in the resolver: the loaded pair returns a
    canonical `transaction.Status`; for investment legs the
    resolver uses `statusFromRegular` / `statusToRegular` (the
    helpers introduced in P1-008/P1-009).

- [x] **P-A4-004 — Docs sweep**
  - `specs/cli.md`: add new sections for `transfer edit` and
    `transfer delete` with flag tables and examples. Mention
    `--show-ids` on the list/search commands in their existing
    sections.
  - `README.md`: in the CLI reference's Transactions section, add
    pointers to `transfer edit` and `transfer delete` (single-line
    examples). The Investment section's "CLI parity is deferred"
    note is gone.
  - `specs/transactions.md`:
    - In the **Whole-Transaction Transfer Rules** list (currently
      lines 144–149), bullet 3 says "Deleting one side prompts to
      delete the other" — "prompt" is TUI-flavored. Reword to
      "Deleting one side removes both sides as a pair (the TUI
      prompts for confirmation; the CLI's `transfer delete`
      deletes both without prompting)."
    - In the **Delete Transaction** section (line 258–265),
      bullet 2 says "Whole-transaction transfer: prompt to delete
      both sides" — same edit: "Whole-transaction transfer: both
      sides are deleted as a pair (TUI prompts; CLI does not)."
    - Optionally extend the **Edit Transaction** section to
      mention CLI `transfer edit` as the non-TUI entry point and
      that it mirrors the TUI's editable-fields set (amount, date,
      memo, status; not from/to).
  - `specs/implementation-plan-investment-cash-transfer-unification.md`:
    update the Future-A section to "completed — see
    `implementation-plan-investment-transfer-cli-parity.md`".

### P-A5: Verification + close-out

- [x] **P-A5-001 — Manual CLI verification** *(2026-05-27)*
  - Verified against a fresh test DB (Checking, Savings, Brokerage,
    Rollover IRA, HSA):
    - `transfer add` every combination (reg→reg, reg→inv, inv→reg,
      inv→inv, reg→hsa) — all create successfully with transfer-id +
      both leg IDs printed.
    - `transaction list --show-ids` — leading UUID column appears and
      IDs are copy-pasteable into edit/delete.
    - `transfer edit` reg→reg (amount+status+memo) and reg→inv
      (amount) — changes land on both legs (verified via the regular
      registers).
    - `transfer delete` all four kinds — both legs gone; Brokerage
      cash returned to $0 after deleting its reg→inv / inv→reg legs,
      confirming investment-side cascade.
    - Refuse cases verified: `transfer edit --status reconciled`,
      no-editable-flags `transfer edit`, unknown `--txn-id`.
  - The transfer-line-split (paycheck → 401k) refuse path requires
    TUI setup that isn't scriptable here; it is covered by the
    integration tests `TestTransfer{Delete,Edit}_RefusesTransferLineSplit`.

- [x] **P-A5-002 — Final test/lint/commit** *(2026-05-27)*
  - `go test ./...` clean — 5516 tests across 28 packages.
  - `golangci-lint run` clean.
  - Implementation committed + pushed (`b98294a`); plan marked complete.

---

## Future Phases

### Future-H — Investment-side void

For `transfer void` parity with the TUI (and a general "void an
investment transaction" affordance), `investment.Service` needs:

- A `Status` field on `investment.Transaction` with an enum that
  includes `void` (currently the model has no status column —
  reconciled and cleared states are *only* on regular
  transactions).
- A schema migration adding the status column and backfilling
  existing rows to a sensible default (`cleared`, probably).
- A `VoidTransaction` method that sets amount=0, memo="**VOID**",
  status=void, and reverses the position/lot side-effects of the
  original transaction (mirroring how `Update*` methods reverse
  side-effects before re-applying).
- Total-return updates to exclude void rows from
  cash/dividends/realized totals.
- TUI affordance: a `v` key in the investment register, or a Void
  menu item, so users can void from the TUI before the CLI ships
  this.
- Then CLI `transfer void --txn-id <leg-uuid>` would dispatch the
  same way as `transfer delete`, calling `VoidTransaction` on each
  leg instead of `Delete*`.

This is its own multi-PR initiative — comparable in size to the
investment-cash-transfer unification.

### Future-I — `transferlink` investment side

`internal/transferlink/transferlink.go` today only scans the
`transactions` table for unlinked cancel-pair candidates. With
investment-side cash flows now produced as
`investment_transactions` rows, an import that brings each side in
separately (regular CSV + investment CSV from a brokerage) might
have two cancellable rows on different tables. Extending
`transferlink` to also scan `investment_transactions` (and pair
across tables) closes that gap.

The transferlink algorithm itself is shape-agnostic — adding the
investment table is mostly query + cross-table join logic.

### Future-J — Multi-line split edit / delete from CLI

The CLI has no entry point today for editing a paycheck's split
structure. The TUI has the multi-line dialog with imbalance
indicator and the paycheck wizard. If/when CLI access to multi-
line edits becomes needed:

- A new command shape like
  `tmoney transaction edit-line --transaction-id <parent> --line-id <split-item>`
  or a JSON / TOML "transaction-as-document" import shape.
- Validation of the parent's signed-sum invariant before save.
- Reverse-cascade semantics when editing a transfer-line's amount
  (already implemented at the service layer; CLI just calls in).

Genuinely a separate UX problem — picking the right CLI shape for
"a paycheck with five lines" is its own design pass.

### Future-K — `transaction edit` / `transaction delete` / `transaction void --by-id`

Same gap as this plan's `transfer edit`/`transfer delete`, but for
single-line non-transfer transactions. Today: `transaction add`
exists but no edit/delete/void-by-id. Adding them with the same
`--txn-id` identifier scheme matches the patterns this plan
introduces. Out of scope here because:

1. The user's framing was "investment transfers"; this would
   double the surface area.
2. The identifier and discovery work (Q3, Q6) done here can be
   reused — they're already cross-cutting.

---

## Test Strategy

### Layer coverage

- **Unit tests** (`internal/transaction/dispatch_test.go`): every
  `(account.Type, account.Type)` combination of
  `ChooseTransferDispatch` returns the right enum value. HSA on
  either side routes via investment branches.
- **CLI integration tests** (`internal/cli/transfer_add_test.go`,
  `transfer_edit_test.go`, `transfer_delete_test.go`): each
  dispatch kind is exercised against a real DB. The CLI command's
  RunE function is invoked directly via the
  `runTransferAdd`/`runTransferEdit`/`runTransferDelete` helpers
  (matching the existing CLI test pattern in
  `transfer_add_test.go`).
- **Refusal-path tests**: transfer-line paired side, reconciled
  leg, `--status reconciled`, no editable flags supplied. Each
  asserts both the error type and the user-facing message text.
- **Display tests** (`internal/cli/transaction_list_test.go`,
  `transaction_search_test.go`): `--show-ids` adds a UUID column;
  default-off output is byte-identical to current.

### Regression discipline

After Phase A lands, the following pre-existing scenarios must
still pass without behavior change:

- `tmoney transfer add` with two bank accounts: same dispatch
  path, same output (modulo the new ID lines).
- TUI's `chooseTransferDispatch` callers (now using the exported
  `transaction.ChooseTransferDispatch`): every TUI dialog test in
  `transfer_dialog_test.go` continues to pass.
- `transaction list` without `--show-ids`: byte-identical to
  current output (`TestTransactionList_DefaultOmitsIDColumn`).
- Service-layer guard tests
  (`TestCreateTransfer_RejectsInvestmentSource` etc.): still pass
  — the guard is unchanged, the CLI just routes around it.

---

## Open Questions Resolved During Grilling

| Q | Decision | Section |
|---|---|---|
| Q1 | Extend `tmoney transfer add` with internal dispatch | D1 |
| Q2 | Wide scope: add + edit + delete | D2 |
| Q3 | Leg transaction-id identifier; `--show-ids` for discovery | D3, D6 |
| Q4 | `transfer edit` mirrors TUI; only-supplied-flags semantic | D4 |
| Q5 | `transfer delete` is hard delete; no `transfer void` (Future-H) | D5 |
| Q6a | Hidden by default, `--show-ids` opts in | D6 |
| Q6b | Always print IDs in `transfer add` confirmation | D7 |
| Q7a | Keep `CreateTransfer`/`UpdateTransfer` rejection guards | D8 |
| Q7b | Dispatch helper in `internal/transaction/` (shared TUI + CLI) | D9 |
| Q8 | Four PRs in dependency order | D11 |
| Q9 | Whole-transaction transfers only; refuse transfer-line paired sides | D10 |
