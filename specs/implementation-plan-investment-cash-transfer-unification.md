# Implementation Plan: Investment Cash Transfer Unification

This document captures the design and implementation plan for two
linked changes:

1. **Phase 1** — enable cash transfers between two investment accounts
   (e.g., E\*Trade Rollover IRA → Wealthfront Roth IRA), unify the
   bank "Transfer" dialog and the investment "Transfer Cash" dialog
   into a single dialog with explicit From/To pickers, and harden
   `transaction.Service.CreateTransfer` against malformed data.

2. **Phase 2** — route transfer-line splits (used by multi-line
   scheduled transactions / paychecks) through the investment service
   when the target is an investment account, so a paycheck schedule
   with a 401k contribution line posts as a proper
   `investment.Transaction` of type `TransferCash` rather than a
   malformed regular `transaction.Transaction` in the investment
   account.

Mark items as complete with `[x]` as they are finished.

## Status Legend

- `[ ]` Not started
- `[x]` Complete

---

## Background

### Problem (Phase 1)

The investment "Transfer Cash" dialog (`investment_transfer_cash_dialog.go`)
filters the "Other account" dropdown to non-investment accounts only.
The underlying service methods (`investment.Service.TransferCash` and
`investment.Service.DepositFromAccount`) reject investment accounts on
the regular side. So IRA→IRA cash transfers — a legitimate real-world
flow when rolling cash between retirement accounts — were never built.

A user attempting the operation today silently cannot pick the other
IRA in the dropdown, with no error or hint that the case is unsupported.

### Problem (Phase 2)

Multi-line scheduled transactions (used by paychecks per
`specs/multiline-splits-and-paycheck.md`) can carry **transfer-line
splits** that mint a paired transaction in a target account. The
posting path goes through `transaction.Service.CreateWithSplits` (and
`scheduled.Service.PostWithEdits`) which uses the regular split-
counterpart creation logic. When a transfer-line split targets an
investment account, this creates a regular `transaction.Transaction`
row in the investment account's ledger — malformed data, since
investment accounts expect `investment.Transaction` rows for cash flow.

This affects realistic user setups: paycheck → 401k contribution,
paycheck → brokerage auto-deposit, etc.

### Why one combined plan

Both problems share the same root: cash-transfer entry points that
involve an investment account need to create `investment.Transaction`
rows on the investment side, not regular `transaction.Transaction`
rows. Phase 1 fixes the direct-dialog path; Phase 2 fixes the
indirect split-counterpart path. They share the new service surface
and the hardening invariant, so it makes sense to plan them together
even though they ship separately.

## Goals

- IRA→IRA cash transfers work end-to-end from the TUI.
- A single Transfer dialog handles all four (From.Type, To.Type)
  combinations: bank↔bank, bank↔investment, investment↔bank,
  investment↔investment.
- All four paths integrate with undo.
- The dialog has consistent UX with the existing bank Transfer dialog
  (From/To pickers, sticky-date, Status-on-Edit).
- The `transaction.Service.CreateTransfer` / `UpdateTransfer` methods
  reject investment accounts at the service layer so the bug cannot
  recur from other entry points.
- Phase 2: scheduled transactions with transfer-line splits to
  investment accounts post correctly.

## Non-goals

- **CLI parity for the new dialog dispatch.** No
  `tmoney investment transfer-cash` command (or equivalent overload of
  `tmoney investment transfer`) is added in this work. The existing
  `tmoney transfer add` will start rejecting investment accounts as
  part of the hardening fix; users wanting linked cash transfers
  involving an investment account use the TUI. Tracked as future work
  in [Future Phases](#future-phases).
- **Multi-currency / FX conversion** on transfers. The existing
  bank↔bank transfer code has no FX support either; we match that
  behavior. A future feature could add FX prompting and store
  per-leg converted amounts.
- **Investment-wide undo coverage**. Only the cash-transfer paths
  gain undo in Phase 1. Buy/Sell/Dividend/Transfer-Shares remain
  un-undoable. Adding investment-wide undo is its own initiative.
- **Transfer Shares dialog refactor**. `Transfer Shares` keeps its
  current implicit-source UX (anchor register + "To Account" picker);
  only `Transfer Cash` adopts From/To. Future consistency pass could
  unify them but is out of scope here.
- **Reconciliation interaction changes.** Each leg reconciles
  independently in its own account's reconciliation session — same as
  bank↔bank today, no special handling required.

---

## Design Decisions

These decisions came out of the design-grilling session that
preceded this plan. They are recorded here so future readers
understand *why* each implementation choice was made.

### D1 — Dedicated service method for inv↔inv (Q1)

A new `investment.Service.TransferCashBetweenInvestments(srcAccountID,
dstAccountID, date, amount, memo)` method, parallel in shape to the
existing `TransferShares`. The method:

- Validates both accounts are investment-type.
- Validates source ≠ destination.
- Validates amount > 0.
- Mints a fresh `transferID`.
- Creates two `investment.Transaction` rows of type `TransferCash`:
  one negative on the source, one positive on the destination,
  linked by the shared `transferID`.
- Cleans up the source row if the destination row create fails
  (mirroring existing `TransferCash` / `TransferShares` patterns).

**Why a new method instead of relaxing the existing `TransferCash` /
`DepositFromAccount`?**

The existing methods use both `s.repo` (investment) and `s.txnRepo`
(regular) to create the two sides. The new method only uses `s.repo`.
Branching internally would be a long if/else; a parallel method
keeps each method's invariants simple. Mirrors the established
`TransferShares` pattern.

### D2 — Dialog uses explicit From/To (Q3)

Replace the existing investment Transfer Cash dialog's Direction +
Other-account fields with explicit From / To pickers, both containing
all accounts (no type filter). From defaults to the currently-open
register's account; To defaults to first non-self account.

**Why drop the Direction toggle?**

- The existing bank Transfer dialog already uses From/To (no
  Direction toggle); the investment Transfer Cash dialog is the
  outlier. Aligning matches the bank-website mental model.
- The Direction toggle was needed only because the dialog had an
  implicit anchor (the current investment register); From/To makes
  the anchor explicit, which is necessary anyway once the dialog is
  unified across bank and investment registers.

### D3 — Edit-mode From→To is read-only (Q3 sub-decision)

In Edit mode, From and To are rendered as a read-only body message
(`From → To`), matching the existing bank Transfer Edit dialog
(`buildEditTransferDialog`). To change the accounts on an existing
transfer, the user deletes and recreates it.

**Why read-only on edit?**

- Matches the existing bank pattern users already know.
- Avoids the implementation complexity of edit-time cross-type
  re-dispatch (e.g., editing a bank↔bank transfer into a bank↔inv
  transfer).
- Less surface area for bugs.

### D4 — Single unified dialog (Q4)

Retire `investment_transfer_cash_dialog.go` entirely. Extend
`transfer_dialog.go` in place to handle all four (From.Type, To.Type)
combinations. `submitTransferDialog` becomes a dispatcher.

**Why unify rather than keep two dialogs?**

- One place to maintain dialog UX and behavior.
- Both the user's "T" entry point in a bank register and the "New
  Transaction → Transfer Cash" path in an investment register now
  open the same dialog seeded with the right default From.
- Aligns with the user's "1 dialog so we don't have to update
  multiple places" principle.

### D5 — Extend the existing bank dialog (Q5)

The bank Transfer dialog (`transfer_dialog.go`) is already structurally
closer to the target shape: From/To pickers, undo integration, account
dropdown already includes all accounts. Most of the work is in the
submit dispatcher and reusing the existing edit-pair display. The
investment Transfer Cash dialog goes away.

### D6 — All paths integrate with undo (Q6)

Three new undo commands in `internal/undo/`:

- `NewCreateInvestmentTransferCashCommand` — wraps `TransferCash`
  (inv→reg withdraw direction).
- `NewCreateInvestmentDepositCommand` — wraps `DepositFromAccount`
  (reg→inv deposit direction).
- `NewCreateInvestmentToInvestmentTransferCommand` — wraps the new
  `TransferCashBetweenInvestments`.

Each `Execute()` creates the transfer; each `Undo()` deletes both
legs. Update counterparts may be added later if Edit needs undo
support; for v1, edit goes through the existing `UpdateTransfer` /
`UpdateTransferCash` delete-and-recreate pattern and is not undoable
(matching today's bank-edit behavior, which also isn't undoable).

**Why all paths and not just bank↔bank (status quo) or none?**

- Don't regress bank behavior (already undoable).
- Don't leave half-finished feature (Q6: full coverage).
- Reasonable cost: ~3 small command structs and tests.

### D7 — Sticky-date everywhere (Q5 add-on)

The unified dialog reads and writes `txnDialogLastSavedDate` on open
and save, regardless of which dispatch path it uses. This means bank
Transfer dialogs now also seed from and write to the session-wide
sticky-date — a small behavior change for bank users, but matches
the investment dialog's existing UX and avoids two different
sticky-date semantics in one dialog.

### D8 — Status field always visible on Edit (Q7)

The Cleared/Uncleared radio that exists on the bank Edit dialog stays
on the unified Edit dialog for all dispatch paths. The investment
`UpdateTransferCash` and new equivalent for inv↔inv take a status
parameter and apply it to both legs after recreation. Today
investment users have to press `c` in the register to toggle cleared;
this exposes the same control inside the dialog for consistency.

### D9 — Defer CLI parity (Q8)

No CLI command is added for the new inv↔inv path or the existing
inv↔reg linked-cash-transfer paths. The CLI gap for cash-transfer
flows is pre-existing (no command today wraps `TransferCash` /
`DepositFromAccount` either). Closing it warrants its own design
pass — see [Future Phases](#future-phases).

### D10 — Match existing FX behavior, i.e. none (Q9)

No currency check or conversion in `TransferCashBetweenInvestments`.
Matches existing bank↔bank transfer behavior, which is also
single-currency-only. A user with a USD IRA and a EUR IRA cannot do
this transfer meaningfully — same as for bank accounts.

### D11 — Harden CreateTransfer / UpdateTransfer (Q10)

Add validation to `transaction.Service.CreateTransfer` and
`UpdateTransfer` that rejects any account whose type satisfies
`Type.IsInvestmentType()`. Error message points callers to the
`investment.Service` family.

**Why harden?**

The pre-existing bank Transfer dialog had no filter; the underlying
`CreateTransfer` had no validation; the result was that picking an
investment account on either side silently created malformed data
(a regular `transaction.Transaction` row in an investment account's
ledger). After unification the dialog dispatcher routes correctly,
but `CreateTransfer` remains a public service method callable from
the CLI (`tmoney transfer add`), the undo system, and any future
internal code. Closing the hole at the service layer prevents
recurrence.

### D12 — Docs (Q11)

- New: `specs/implementation-plan-investment-cash-transfer-unification.md`
  (this file).
- Updated: `specs/transactions.md` — unified Transfer dialog model,
  new `TransferCashBetweenInvestments` service method, hardening
  invariant.
- Updated: `README.md` — Investment section notes inv↔inv TUI
  capability and the deferred-CLI note.
- Updated: `specs/cli.md` — `transfer add` rejection of investment
  accounts; brief pointer in the `investment` section to the
  implementation plan's future-phase CLI section.
- Untouched: `specs/cli-router.md` — historical document (migration
  complete).

### D13 — Two-phase delivery (Q12, Q13)

Phase 1 delivers the user's actual request (IRA→IRA in the TUI) plus
dialog unification and the hardening fix. Phase 2 delivers the
split-path routing so paycheck → 401k contribution lines post as
proper `investment.Transaction` rows.

**Why split rather than ship everything together?**

- Phase 1 alone is already substantial (service + dialog + undo +
  hardening + docs).
- Phase 2 touches a different code path (`CreateWithSplits` and the
  scheduled posting flow) and has its own acceptance criteria.
- Smaller PRs are easier to review and roll back.
- Phase 1 delivers user value the moment it merges; Phase 2 can
  follow without blocking the core feature.

---

## Architecture Summary

### Phase 1 — service-layer surface

```
investment.Service
├── TransferCash(invAcctID, regAcctID, date, amount, memo)             [existing]
├── DepositFromAccount(invAcctID, regAcctID, date, amount, memo)       [existing]
├── TransferCashBetweenInvestments(srcAcctID, dstAcctID, date, amt, m) [NEW]
├── UpdateTransferCash(oldTxnID, ..., direction)                       [extended]
└── (UpdateTransferCashBetweenInvestments may be embedded in           [NEW path]
    UpdateTransferCash via destination-type dispatch)

transaction.Service
├── CreateTransfer(fromAcctID, toAcctID, date, amount)        [HARDENED — rejects inv]
└── UpdateTransfer(transferID, date, amount, memo, status)    [HARDENED — rejects inv]

internal/undo
├── NewCreateInvestmentTransferCashCommand                            [NEW]
├── NewCreateInvestmentDepositCommand                                  [NEW]
└── NewCreateInvestmentToInvestmentTransferCommand                     [NEW]
```

### Phase 1 — UI surface

```
TUI Transfer dialog (transfer_dialog.go — UNIFIED)
├── New mode fields: From, To, Amount, Date, Memo
├── Edit mode fields: (From → To read-only), Amount, Date, Memo, Status
├── Sticky-date seed: read on open, write on save
└── Submit dispatcher → (From.Type, To.Type):
    ├── reg/reg → transaction.CreateTransfer        (via existing undo command)
    ├── inv/reg → investment.TransferCash           (via new undo command)
    ├── reg/inv → investment.DepositFromAccount     (via new undo command)
    └── inv/inv → investment.TransferCashBetween…   (via new undo command)

Entry points (both open the same dialog):
- Bank register: "t" key
- Investment register: "n" → "Transfer Cash" menu item
- Edit: Enter on a transfer row in either register

Deleted:
- investment_transfer_cash_dialog.go (whole file)
- App fields: transferCashDialog, transferCashDialogData, transferCashDialogAccountIDs
- transferCashDialogDataMsg / transferCashDialogSavedMsg routes
- buildTransferCashDialog / submitTransferCashDialog / closeTransferCashDialog /
  handleTransferCashDialogKey
```

### Phase 2 — split-path dispatch

In `transaction.Service.CreateWithSplits` (and the
`scheduled.Service.PostWithEdits` path that calls into it), the
split-counterpart creation logic that today blindly creates a
`transaction.Transaction` in `split.TransferAccountID.ID` will
inspect the target account's type and dispatch:

- target is non-investment → existing path (regular counterpart).
- target is investment → new path: create an `investment.Transaction`
  of type `TransferCash`, sign matches the split direction,
  `transferID` shared with the regular-side counterpart.

Edit/void of the parent transaction must continue to reverse both
regular and investment counterparts coherently.

---

## Phase 1: Plan

Ordered to minimize integration churn — service methods land first
(small, isolated, well-tested), then undo wraps, then the dialog
refactor consumes them, then docs and verification.

### P1-1: Implementation plan doc

- [x] **P1-001 — Draft this document**
  - This file. Captures design decisions, plan, future phases.
  - GREEN: file exists at
    `specs/implementation-plan-investment-cash-transfer-unification.md`
    with all sections from "Background" through "Future Phases".

### P1-2: Service methods (TDD)

- [x] **P1-002 — `investment.Service.TransferCashBetweenInvestments`**
  - RED: `TestTransferCashBetweenInvestments_HappyPath` — given two
    funded investment accounts, transferring $500 produces two linked
    `investment.Transaction` rows of type `TransferCash` (one −$500
    on source, one +$500 on destination) sharing a `transferID`.
  - RED: `TestTransferCashBetweenInvestments_RejectsNonInvestmentSource`
    / `…RejectsNonInvestmentDestination` — passing a non-investment
    account on either side returns a `NotInvestmentError`-shaped error.
  - RED: `TestTransferCashBetweenInvestments_RejectsSameAccount` —
    same source and destination returns the existing same-account
    error.
  - RED: `TestTransferCashBetweenInvestments_RejectsNonPositiveAmount` —
    zero / negative amount returns `InvalidTransferAmountError`.
  - RED: `TestTransferCashBetweenInvestments_NoLeakOnDestinationFailure`
    — a bogus destination account ID errors and leaves no source-side
    row behind. (Covers the spec's CleansUpOnFailure intent via the
    failure path reachable without mocking the repo.)
  - RED: `TestTransferCashBetweenInvestments_AllowsNegativeSourceBalance`
    — matches the wider "cash may go negative" invariant.
  - GREEN: implemented in `internal/investment/investment_service.go`,
    mirroring the `TransferShares` shape but for cash, returning a new
    `InvestmentCashTransferResult`. All seven tests pass; full suite
    and `golangci-lint` clean.

- [x] **P1-003 — Extend `UpdateTransferCash` for inv↔inv**
  - RED: `TestUpdateTransferCash_InvToInv_HappyPath` — given an
    inv↔inv pair created via `TransferCashBetweenInvestments`,
    editing the amount and memo produces a fresh pair on the same
    accounts with the new fields, both legs share a new `transferID`,
    and the original rows are gone (no leftover destination cash).
  - RED: `TestUpdateTransferCash_InvToInv_FlipDirection` — passing
    `direction="in"` with the second account being another investment
    account flips the source/destination orientation so the same
    method can be used to fix a wrong-way IRA→IRA transfer.
  - GREEN: in `internal/investment/update_edit.go`, made
    `UpdateTransferCash` dispatch on the second account's type. When
    that account is also an investment account, the method routes to
    `TransferCashBetweenInvestments` (with the source/destination
    derived from `direction`). The old-counterpart cleanup now
    searches the investment repo (by `transfer_account_id` /
    `transfer_id`) in addition to the regular-transaction repo, so an
    inv↔inv original is fully reaped before the new pair lands.
    `CashTransferResult` gains a `CounterpartInvestmentTransaction`
    field that carries the destination-side investment row on inv↔inv
    edits (nil on inv↔reg edits). Existing inv↔reg behavior is
    unchanged.

### P1-3: Service hardening (TDD)

- [ ] **P1-004 — Reject investment accounts in
  `transaction.Service.CreateTransfer`**
  - RED: `TestCreateTransfer_RejectsInvestmentSource` — `from` is
    an investment account, returns a clear error mentioning
    `investment.Service`.
  - RED: `TestCreateTransfer_RejectsInvestmentDest` — same for `to`.
  - RED: `TestUpdateTransfer_RejectsInvestmentAccounts` — same for
    `UpdateTransfer`.
  - GREEN: add `accountRepo.GetByID` checks at the top of both
    methods; introduce a new `NotRegularAccountError` if not already
    present in the transaction error set.
  - Audit: search the test suite for any fixture that creates a
    `transaction.Transaction` in an investment account via
    `CreateTransfer`; either remove (probably no real cases) or
    convert to the investment service.

### P1-4: Undo commands (TDD)

- [ ] **P1-005 — Three new investment cash-transfer undo commands**
  - RED: per command — `TestCreateInvestmentTransferCashCommand_Roundtrip`
    (Execute then Undo leaves zero rows), `…Deposit…`, `…ToInvestmentTransfer…`.
  - GREEN: add the three commands in `internal/undo/`. Pattern
    matches the existing `NewCreateTransferCommand` shape: store the
    service handle + inputs; `Execute()` creates and stores the
    pair's `transferID`; `Undo()` deletes both rows via the
    service's existing delete paths.

### P1-5: Dialog unification

- [ ] **P1-006 — Dispatch in `submitTransferDialog`**
  - RED: dialog-level unit tests covering all four (From.Type,
    To.Type) combinations: the right undo command is constructed
    with the right service.
  - RED: same-account rejection error path.
  - GREEN: in `submitTransferDialog`, look up From and To account
    types, branch into one of four undo-command constructions, and
    execute via `undoManager.Execute`. Remove the
    `transactionSvc`-only assumption.

- [ ] **P1-007 — Sticky-date on the unified dialog**
  - RED: `TestTransferDialog_SeedsLastSavedDate` — when
    `txnDialogLastSavedDate` is set on the App, opening the dialog
    seeds the Date field with that value.
  - RED: `TestTransferDialog_WritesLastSavedDateOnSave` — after a
    successful submit, `txnDialogLastSavedDate` equals the saved
    date.
  - GREEN: call `dialog.SeedDateField(a.txnDialogLastSavedDate)` in
    the existing dialog-build path; update on save in the submit
    handler.

- [ ] **P1-008 — Status-on-Edit applies to both legs**
  - RED: per dispatch path — open Edit on an existing inv↔reg /
    inv↔inv pair, toggle Status to Cleared, save; verify both legs
    have `cleared` status.
  - GREEN: extend the investment Update methods to accept a status
    parameter and apply it on the freshly-created rows; wire the
    Status radio's selected value through the submit handler.

### P1-6: Investment register integration

- [ ] **P1-009 — Wire investment register Transfer Cash entries**
  - In `investment_register_view.go` and `app_mouse.go`: the "New
    Transaction → Transfer Cash" path and the Enter-on-TransferCash-row
    path call `loadTransferDialogData` (in a mode that defaults From
    to the current investment account; in edit mode, pre-fills with
    the existing transfer pair, which now might be inv↔inv).
  - Update or replace existing tests in
    `investment_transfer_cash_dialog_test.go` (the file goes away)
    against the unified dialog's behavior with an investment-account
    default.

### P1-7: Delete dead code

- [ ] **P1-010 — Remove `investment_transfer_cash_dialog.go` and
  related dead state**
  - Delete `investment_transfer_cash_dialog.go` and
    `investment_transfer_cash_dialog_test.go`.
  - Delete `App.transferCashDialog`, `App.transferCashDialogData`,
    `App.transferCashDialogAccountIDs`, and all references in
    `app.go`, `app_update.go`, `app_view.go`, `app_helpers.go`,
    `app_mouse.go`.
  - Delete `transferCashDialogDataMsg`, `transferCashDialogSavedMsg`,
    and any helper functions that only the old dialog used (e.g.,
    `buildNonInvestmentAccountOptions`).
  - Verify: `go build ./...` passes; `grep -r transferCashDialog`
    returns nothing.

### P1-8: Documentation

- [ ] **P1-011 — Update `transactions.md`, `README.md`, `cli.md`**
  - `specs/transactions.md`: describe the unified Transfer dialog
    model (From/To, dispatch table, Status-on-Edit, sticky-date),
    the new `TransferCashBetweenInvestments` service method, and the
    invariant that `transaction.Service.CreateTransfer` /
    `UpdateTransfer` reject investment accounts.
  - `README.md`: in the Investment section, mention that the TUI now
    supports cash transfers between two investment accounts (e.g.,
    IRA→IRA rollovers). Add a note that CLI parity for linked cash
    transfers involving investment accounts is deferred to a future
    phase.
  - `specs/cli.md`: in the `transfer add` section, document that
    investment accounts are now rejected and direct users to the TUI.
    Optionally add a one-line forward-pointer in the `investment`
    section header.
  - **Leave `specs/cli-router.md` alone** — historical document.

### P1-9: Verification + commit

- [ ] **P1-012 — Manual TUI verification**
  - Build and launch against a test database with at least two
    investment accounts (one lot-tracked, one not), one bank
    checking account, and one savings account. Verify each scenario:
    - New transfer: bank→bank, bank→inv, inv→bank, inv→inv.
    - For each: both legs appear in their respective registers,
      linked via `transferID`, sticky-date seeded on the next open.
    - Undo each: Ctrl+Z removes both legs.
    - Edit each pair (Enter on a leg): From→To displayed as read-only;
      Amount/Date/Memo/Status edits apply to both legs.
    - (R) hardening regression: `tmoney -f test.tdb transfer add
      --from <inv> --to <bank> --amount 100` errors with the new
      message; `--from <bank> --to <inv>` also errors.

- [ ] **P1-013 — `go test ./...` + lint + commit + push**
  - `go test ./...` clean.
  - `golangci-lint run` clean.
  - Stage and commit Phase 1 with a message describing the unified
    Transfer dialog, new inv↔inv service method, hardening fix, and
    referencing this plan.
  - Push.

---

## Phase 2: Plan

Phase 2 is sketched here; details will be filled in once Phase 1
lands and we trace the exact split-counterpart code path.

- [ ] **P2-001 — Trace the split-counterpart creation path**
  - Map every code path that mints a counterpart `transaction.Transaction`
    for a transfer-line split: direct `CreateWithSplits`, scheduled
    `PostWithEdits`, any others (link-transfers, import flows).
  - Identify the single insertion point where a dispatch can branch
    based on `accountRepo.GetByID(split.TransferAccountID).Type`.
  - Append findings as notes to this document.

- [ ] **P2-002 — Investment-target dispatch in split path (TDD)**
  - RED: `TestCreateWithSplits_InvestmentTargetSplit` — given a parent
    transaction in a checking account with a transfer-line split
    whose `TransferAccountID` is an investment account, the resulting
    pair is the parent in checking + an `investment.Transaction` of
    type `TransferCash` (positive) in the investment account, linked
    by a shared `transferID` to the parent's split row.
  - RED: same scenario via the scheduled posting path
    (`scheduled.Service.PostWithEdits`).
  - RED: error cases — investment account is invalid, account
    closed, etc.
  - GREEN: implement dispatch at the identified insertion point.

- [ ] **P2-003 — Void/edit of parent with mixed counterparts**
  - RED: posting a paycheck-style parent with one bank-side transfer
    line + one investment-side transfer line, then voiding the parent,
    reverses both counterparts coherently.
  - RED: editing the line amount of an investment-side transfer
    line propagates correctly.
  - GREEN: extend the existing void / edit-line code paths to call
    the investment service when a counterpart is investment-side.

- [ ] **P2-004 — Spec + manual verification + commit**
  - Update `specs/multiline-splits-and-paycheck.md` to document
    investment-target transfer-line behavior. Note any UI implications
    for the paycheck wizard if the user can pick an investment
    account in a Net Pay Destination row.
  - Manually create a paycheck schedule with a 401k contribution
    transfer line, post it via the post-time preview dialog, and
    verify the resulting `investment.Transaction` in the 401k account
    is type `TransferCash`, signed correctly, and linked via
    `transferID` to the parent's split row.
  - `go test ./...` + lint + commit + push.

---

## Future Phases

### Future-A — CLI parity for linked cash transfers (deferred from Q8)

The existing `tmoney investment deposit` / `withdraw` commands are
**one-sided** — they create a cash flow on the investment account
only, with no linked counterpart in a regular account. There is no
CLI today for the linked cash-transfer flows (`TransferCash`,
`DepositFromAccount`, and now `TransferCashBetweenInvestments`).

Two reasonable designs to evaluate when this work is picked up:

1. **New sibling command** `tmoney investment transfer-cash --from X
   --to Y --amount Z`. Handles all three combinations (inv↔reg,
   reg→inv, inv↔inv) via internal dispatch on account types. Pro:
   unambiguous; matches the unified dialog's mental model. Con: name
   collision potential with the existing `tmoney investment transfer`
   (shares).
2. **Overload `tmoney investment transfer`** with mutually-exclusive
   flag groups: `--shares <n> --ticker <t>` for shares, `--cash
   --amount <a>` for cash. Pro: one command. Con: harder to discover;
   help text gets denser.

Either way, this Future-A also implies updating `tmoney transfer
add` so its error message points users to the new investment
counterpart (currently the message points to the TUI).

### Future-B — Multi-currency / FX on transfers

Not in scope here, but the spot to add it once the data model
supports per-leg `original_amount` vs `converted_amount`. Would
apply to bank↔bank, bank↔investment, and inv↔inv alike.

### Future-C — Investment-wide undo coverage

Phase 1 of this plan adds undo to the cash-transfer paths, but
Buy/Sell/Dividend/Reinvest/Fee/Transfer-Shares still skip undo. A
separate initiative could add undo across the investment subsystem,
at which point the conditional dispatch in this dialog (always undo)
becomes uniformly true for the whole investment surface.

### Future-D — Transfer Shares dialog adoption of From/To

For UX consistency, the Transfer Shares dialog could move from its
current "implicit source = register account, explicit To picker"
shape to the unified From/To shape. Not done in this plan to limit
blast radius. Worth doing as a small UX consistency pass.

---

## Test Strategy

### Layer coverage

- **Service tests** (`internal/investment/*_test.go`,
  `internal/transaction/*_test.go`): every new code path has a
  red-first test. Round-trip tests for the new service method;
  validation tests for hardening; status-preservation tests for the
  Update path.
- **Undo tests** (`internal/undo/*_test.go`): Execute/Undo
  roundtrip for each of the three new commands; verifies both legs
  go away on undo.
- **Dialog tests** (`internal/tui/transfer_dialog_test.go`): all
  four (From.Type, To.Type) combinations dispatch to the right
  service; edit-mode From→To rendered as read-only message;
  Status-on-Edit toggle applies to both legs; sticky-date seeded
  and updated.
- **Integration smoke** (manual TUI verification): the P1-012 step
  exercises the actual user flow end-to-end against a real DuckDB
  file.

### Regression discipline

After P1 lands, the following pre-existing scenarios must still pass
without behavior change:

- Bank↔bank new transfer via the bank register's `t` key.
- Bank↔bank edit via Enter-on-a-transfer-row.
- Ctrl+Z undo on a bank transfer.
- Sticky-date for the existing investment dialogs (Buy/Sell/etc.)
  still seeds from the same App field.

Phase 2 adds:

- Existing paycheck → bank-account transfer line posting unchanged.
- Existing void/edit semantics for all-bank-target multi-line parents
  unchanged.

---

## Open Questions Resolved During Grilling

| Q | Decision | Section |
|---|---|---|
| Q1 | New dedicated `TransferCashBetweenInvestments` service method | D1 |
| Q2 | (superseded by Q3) | – |
| Q3 | From/To pickers; Edit-mode read-only | D2, D3 |
| Q4 | Single unified Transfer dialog | D4 |
| Q5 | Extend `transfer_dialog.go` in place + add sticky-date | D5, D7 |
| Q6 | All paths get undo | D6 |
| Q7 | Status field always shown on Edit | D8 |
| Q8 | CLI deferred (Future-A) | D9, Future-A |
| Q9 | No FX (Future-B) | D10, Future-B |
| Q10 | Harden `CreateTransfer`/`UpdateTransfer` | D11 |
| Q11 | Plan + transactions.md + README + cli.md; not cli-router.md | D12 |
| Q12 | Route split-path correctly via investment service (Phase 2) | Phase 2 |
| Q13 | Two-phase delivery | D13 |
