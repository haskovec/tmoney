# Design sketch: service decomposition — god files, god types, and one deleted engine

**Date:** 2026-08-07
**Status:** COMPLETE, 2026-08-07. Every phase shipped; phase 5 was a recorded
non-goal and stayed one. §11 records what the design got right and wrong,
measured against what actually landed, and §10 now carries a decision for every
item it deferred rather than a list of maybes.

Reading order for anyone picking this up cold: §11 first for the outcome, then
§10 for what was deliberately not done and why, then the numbered sections for
the reasoning behind each phase.

| Phase | Shipped | What it did |
|---|---|---|
| 0 | `828e0b5` | 11 dead symbols out of `internal/transaction` (−522/+14) |
| 1a | `70babfd` | 4 dead methods + 3 orphaned error types (−137/+8); all 33 test sites reviewed individually (§1.5) |
| 1b | `e1e47ed` | `transaction` + `corporate_action`: 2 files → 12, byte-identical motion |
| 2 | `d430172` | `investment` + `scheduled`: 2 files → 14, byte-identical motion |
| 3 | `071c27d`, `d6978cf` | the second posting engine deleted; 417 → 381 lines of code |
| 4 | `3963b16`, `ba24049`, `8462935`, `259cf69` | `CounterpartService`, `ValuationService`, `EditService`; `Service` sheds 36 methods |

Doing the work surfaced seven live bugs that predate it, fixed separately in
`d728c45`, `3667a28`, `f8d616d` and `d7a64ef` — see §5.5. None was caused by the
refactor; all were found by taking the two posting engines' differences
seriously enough to enumerate them.

**Addresses:** `specs/code-quality-review.md` item 3 (god services: domain logic
past healthy file size)

---

## Goal

Make the four remaining god modules navigable, and delete the one duplicated
engine hiding inside them. Concretely:

1. No non-test file over ~450 lines in `internal/transaction`,
   `internal/investment`, `internal/scheduled`.
2. `scheduled.Service` has **one** posting engine, not two. Today `AutoPost`
   re-implements all three posting shapes inline (124 lines duplicating ~180
   lines of `post*` bodies, §5.1).
3. `investment.Service` sheds the clusters that are genuinely separable — the
   counterpart port, the valuation/total-return read side, and the edit family
   — leaving one type that owns trades, heal, rebuild and delete.
4. The review's proposed `transaction.Service` → `Service`/`SplitService` cut is
   **rejected in writing** (§3), so the next reader does not re-litigate it from
   the old review.

The ordering principle, learned from items 1–2: a file split is a code move the
compiler proves; a type split re-opens the transaction-binding question, whose
failure mode is silent. Those are not the same kind of work and this design does
not mix them into one phase.

## Non-goals

- **No type split of `transaction.Service`.** §3 records why, with the coupling
  measured. This is the one place the original review's prescription is wrong for
  the current tree.
- **No generic unit-of-work framework.** Every `InTx` stays a hand-written
  per-field rebind. If the boilerplate hurts after the first two extractions in
  phase 4, a small shared `bindRepos` helper can be considered *then* — it is
  explicitly not phase-1 work. Inventing a UoW abstraction before the pain is
  demonstrated is how this design would grow a fifth god type.
- **No new packages.** Every extracted type stays in its current package
  (`package investment`, `package scheduled`). A new package would break the 41
  tests that call unexported methods (§6.3) and risks the
  `TestArch_InvestmentDoesNotImportTransaction` guard
  (`internal/transfer/arch_test.go:124`).
- **No repository-boundary work** (review item 5) and **no removal of the
  heal-on-open side effects** (item 6). Both are touched by phase 4's blast
  radius but neither is in scope; `app.Services` keeps exposing repositories.
- **No behavior change in phases 1b–2.** Those phases are code motion only.
  (Phase 1a deletes dead methods, so it changes the type's surface — but no
  reachable behavior, since nothing in production calls them.)

---

## 1. The problem, measured

### 1.1 Item 3 is two problems with a 10× cost difference

Go allows methods on one type to live in many files of a package. So "god file"
and "god type" are independent, and they cost differently:

| | Fix | Cost | Verified by |
|---|---|---|---|
| God **file** | move methods to sibling files, same type | hours; zero call-site churn | compiler |
| God **type** | new type + own tx plumbing + rebind + callers | days; wide call-site churn | compiler for the moves, **hand-written fault injection for the correctness** |

`investment.Service` is the exhibit for why the distinction matters: its
1,674-line file holds only 33 of the type's **86** methods. The other 53 are
already spread across `update_edit.go`, `total_return.go`, `valuation_service.go`,
`rebuild.go`, `backfill.go`, `split_replay.go`, `price_cleanup.go`. Splitting the
file again changes nothing structural. Conversely `corporate_action_service.go`
holds all 27 of its type's methods in one file and has no service→service edge in
either direction — its whole problem *is* the file.

### 1.2 What items 1–2 collapsed, and what grew

Item 3 is not self-solving via items 1–2. Two of the four god files **grew** after
the transfer refactor:

| File | Review (2026-07-23) | Now | Methods on the type |
|---|---|---|---|
| `transaction/transaction_service.go` | 1898 | **1551** | 46, all in this file |
| `investment/investment_service.go` | 1839 | **1674** | 33 here, **86 across 8 files** |
| `investment/corporate_action_service.go` | 1040 | **1110** ▲ | 27, all in this file |
| `scheduled/scheduled_service.go` | 1029 | **1174** ▲ | 38 here, +8 in `loan_*.go` |

`transaction` and `investment` shrank because `internal/transfer` took the
whole-transfer surface. `corporate_action` and `scheduled` grew because posting
now routes through the transfer port. So the file-size half of item 3 needs doing
on its own merits even if every type split is deferred.

### 1.3 The dead surface — phase 0, shipped

Removed 2026-08-07. All verified by counting production call sites separately
from test call sites, at both the service and repository layer:

| Symbol | Why dead |
|---|---|
| `Service.rejectInvestmentAccount` | zero callers anywhere, incl. tests — leftover of the pre-`transfer` whole-transfer surface |
| `Service.List`, `.ListByDateRange` | zero production callers; `transferlink.go:102` uses `Repository.List` directly |
| `Service.Search`, `.SearchByPayee`, `.SearchByMemo`, `.SearchByCategory` | zero production callers; the live CLI search (`cli/transaction/search.go:122`) calls `TransactionRepo.Search` directly |
| `Repository.ListByDateRange` | orphaned once the service wrapper went |
| `Repository.SearchByPayee`, `.SearchByMemo`, `.SearchByCategory` | 3-line wrappers over `Repository.Search`; zero production callers |

Kept: `Service.ListByAccount` and `.ListByAccountAndDateRange` (TUI register view,
CLI list, and two `imexport` consumer interfaces), and `Repository.Search`
(the live CLI search path).

**Coverage was preserved, not dropped.** The three deleted repository wrappers
were the only thing exercising the case-insensitive partial-match SQL that the
live CLI search depends on, so their 11 integration-test call sites were rewired
onto `txnRepo.Search(SearchCriteria{PayeeName: …})` rather than deleted. Only
`TestTransactionListByDateRange` was removed outright, because nothing reaches
that SQL any more.

### 1.4 A second dead tier — resolved, and its real cost

Seven more exported `transaction.Service` methods have **zero** production call
sites outside the service file (~321 lines plus tests). They were left in place
because deleting them is a design decision, not a free deletion, and they split
into two different cases:

| Method | Lines | Why uncalled |
|---|---|---|
| `ReconcileTransaction` | 21 | `reconciliation.Service` deliberately uses `txnRepo.UpdateStatus` instead, for a documented DuckDB index reason (`reconciliation_service.go:275-282`) |
| `UnReconcileTransaction` | 26 | same |
| `AddSplit` | 63 | every caller goes through `ReplaceSplits`/`UpdateWithSplits` |
| `UpdateSplit` | 81 | same |
| `DeleteSplit` | 59 | same |
| `Duplicate` | 62 | no surface ever wired it up |
| `ValidateSplitTotals` | 9 | test-only assertion helper |

The first two are genuinely orphaned — the bypass is intentional and documented,
so there is no "restore the boundary" story to tell. The middle three are
different: they are where the *per-split* transfer-line cascade lives, and 23
tests reach `findPairedByTransferID` through them. Deleting those three means
deleting cascade coverage that `ReplaceSplits` only exercises in bulk.

**Decided 2026-08-07:** delete `ReconcileTransaction`, `UnReconcileTransaction`,
`Duplicate`, `ValidateSplitTotals` (118 lines). Hold
`AddSplit`/`UpdateSplit`/`DeleteSplit` until the file split has made the cascade
tests' true subject visible.

**Cost correction (measured after the decision).** An earlier draft of this
section implied the deletion was "118 lines plus their own tests." It is not.
The four methods have **33 test call sites**, and the majority are *fixtures for
behavior that stays live*, not tests of the doomed methods:

| Method | Sites | Fixture (rewire) | Subject (delete/trim) |
|---|---|---|---|
| `ReconcileTransaction` | 18 | 17 | 1 |
| `UnReconcileTransaction` | 9 | 4 | 5 |
| `Duplicate` | 4 | 0 | 4 |
| `ValidateSplitTotals` | 2 | 0 | 2 |

The fixture sites use `svc.ReconcileTransaction(id)` as a convenient way to *put a
row into reconciled state*, then assert something that is genuinely live — that a
reconciled transaction refuses edits (`TestStatusLifecycle_ReconciledIsLocked`,
`service_status_test.go:372`), or that it counts toward the cleared balance
(`TestBalanceCalculation_ReconciledInClearedBalance`, `:184`;
`TestBalanceCalculation_UnReconciledRemainsInClearedBalance`, `:222`). Those tests
must survive; only their setup line changes to
`svc.txnRepo.UpdateStatus(id, StatusReconciled)`.

**This makes the tests more faithful, not less.** Production has no path that
calls `ReconcileTransaction` — `reconciliation.Service` writes reconciled state
via `txnRepo.UpdateStatus` (`reconciliation_service.go:282`) and restores prior
status the same way on reopen (`:391`). So the guards inside
`ReconcileTransaction` (void check, closed-account check) are **already
unenforced in production today**; deleting the method removes the illusion of a
guard rather than a guard. Rewiring the fixtures onto `txnRepo.UpdateStatus` makes
them set up state the way production actually does.

Subject-side work is trimming, not wholesale deletion, because two of the
lifecycle tests cover live and dead methods in one sequence:
`TestStatusLifecycle_FullCycle` (`:273`) walks uncleared → cleared → reconciled →
un-reconcile → cleared → void, and `TestStatusLifecycle_VoidIsTerminal` (`:330`)
asserts every status op fails on a void row. Both keep their `ClearTransaction` /
`MarkTransactionUncleared` / `VoidTransaction` legs and lose only the reconcile
legs.

One site is cross-package: `internal/undo/reconciliation_test.go:280` uses
`env.txnSvc.ReconcileTransaction` and cannot reach the private `txnRepo`, so
`createReconTestEnv` (`:25-32`, which already constructs one) gains a `txnRepo`
field.

### 1.5 What the site-by-site review changed — phase 1a, shipped

Reviewing all 33 sites individually (§8's requirement) found a **second**
cross-package site the tally above missed, and corrected the fixture/subject split
in three places. Shipped shape, versus §1.4's plan:

| Plan said | Shipped | Why |
|---|---|---|
| 21 fixture rewires | 12 | Four "fixture" sites were subject legs inside surviving tests, so they were trimmed, not rewired |
| `tests/integration/split_test.go` untracked | **rewired, not deleted** | Its `ValidateSplitTotals` call is the only test that validates split totals *after* `CreateWithSplits`' transactional write; `split_repository_test.go` persists splits directly and cannot reach that path. Re-pointed onto `splitRepo.ValidateSplitsAgainstTransaction` and renamed, following phase 0's own precedent (§1.3) |
| three error types unmentioned | **deleted** | `NotReconciledError`, `CannotDuplicateTransferError`, `CannotDuplicateSplitTransferError` had zero references left. Go does not fail a build on an unreferenced exported type, so nothing would have surfaced them |
| `TestStatusLifecycle_FullCycle` keeps its name | **renamed** | After losing the reconcile legs it walks three of six states; `_UnclearedToClearedToVoid` is what it now does |
| `TestTransactionService_UnReconcileTransaction` trims to one subtest | **deleted, one subtest rehomed** | The surviving subtest's setup was inert after rewiring — it asserted only that `Update` works on a cleared row. Rebuilt as `Update_VoidGuard/"allows editing once a reconciled transaction returns to cleared"`, which writes reconciled, asserts `Update` is refused, writes cleared, then asserts it succeeds. Now neither write can be dropped without a failure |

Two coverage facts worth recording, because a later reader will otherwise
re-derive them:

- **`splitRepo.ValidateSplitsAgainstTransaction` is not dead.** It looks like an
  orphan once the service passthrough goes, but `AddSplit`
  (`transaction_service.go:621`) is a live second caller feeding
  `SplitTotalMismatchError`. Keep it.
- **One composite path is no longer exercised:** reconciled → un-reconciled →
  void, which only `TestStatusLifecycle_FullCycle` walked. This is acceptable
  because `VoidTransaction` inspects only the row's *current* status
  (`:1307`), so prior status cannot influence it. No guard that production can
  reach lost coverage.

Test-side outcome: 6 test functions deleted whole, 3 trimmed in place, 7 subtests
deleted, 12 sites rewired onto `UpdateStatus`, 1 test-env struct given a
`txnRepo` field, 1 dead fixture field removed. Full suite green, unmodified
elsewhere.

---

## 2. The constraint that prices every type split

### 2.1 There is no generic unit of work

`db.WithTx` takes a closure and **must not be nested** — DuckDB has no
savepoints, and `WithTx` serializes on `db.txMu`, so a nested call deadlocks
(`internal/db/tx.go:5-15`). Composition is instead done by the join-if-bound
triad, hand-written on each of the seven services:

```go
func (s *Service) runInTx(fn func(b *Service) error) error {
	if s.tx != nil { return fn(s) }              // join the caller's tx
	return s.db.WithTx(func(tx db.Queryer) error { return fn(s.InTx(tx)) })
}
```

`runInTx`'s parameter is concretely typed on its own receiver, and `InTx`
enumerates its own fields. There is no generic form in the tree, so **every new
type pays ~20–37 lines of its own plumbing**, and two sibling types share a
transaction only via one of three existing mechanisms:

1. **A holds B and rebinds it** — `c.b = s.b.InTx(tx)`, as
   `scheduled_service.go:79` does for `txnSvc`. Requires an acyclic A→B.
2. **A consumer-declared port taking `db.Queryer` per call** — used twice
   (`transaction.InvestmentCounterpartPort`, `scheduled.TransferPort`).
3. **A new type owning both sides' repositories** — `transfer.Service`.

### 2.2 join-if-bound, and the one way it breaks

The failure mode is not a compile error and, since `SetMaxOpenConns(1)` was
removed (`d05b784`), not a deadlock either. A pool access inside an open tx now
**silently misses in-tx writes**. Only `txMu` remains, so the loud tripwire is
gone.

Precisely: `runInTx` on a **bound** receiver joins and is safe. `runInTx` on an
**unbound** receiver, called while a tx is open, reaches `db.WithTx` a second
time and deadlocks on the mutex. So the rule is about the *receiver*, not the
method:

> Everything reachable inside a `runInTx` closure must be invoked on the bound
> copy `b`. A method called on the captured outer `s` is a bug — silently
> non-atomic if it only reads a pooled connection, a hang if it opens a tx.

This is the whole reason type extraction is money-risk rather than refactor-risk,
and it is why every phase-3/4 exit criterion below includes fault injection.

### 2.3 The rule this design adds: participants and entry points

To make the above mistake structurally impossible rather than merely avoidable,
each extracted cluster is written as two layers:

- **Participant** — takes the bound `b` (or is a method on a bound receiver),
  performs row writes and in-memory mutation, **opens no transaction**, and
  never persists a sibling aggregate's row.
- **Entry point** — opens the transaction exactly once via `runInTx`, calls
  participants on `b`, and owns the commit boundary.

A participant cannot deadlock because it never calls `WithTx`. An entry point
cannot double-open because it is the only layer allowed to. This is the rule
phase 3 turns on `scheduled`, and phase 4 reuses.

---

## 3. `transaction.Service` — file split only. Type split is a non-goal.

The review proposed `Service` (simple txn) / `SplitService` / `TransferService`.
`TransferService` already exists as `internal/transfer`. The remaining
`Service`/`SplitService` cut is **rejected**, on three measured grounds.

**(a) Plain CRUD and the split lifecycle are mutually entangled.** Not layered —
mutual:

| Direction | Evidence |
|---|---|
| CRUD → splits | `Update` reads `splitRepo.GetByTransferID` twice (`:212` transfer probe, `:236` reverse-cascade probe) and writes a split via `cascadeAmountToParentSplit` (`:241`) |
| CRUD → counterparts | `Delete` reads `splitRepo` (`:300`) then calls `deletePairedSideOfMultiLine` (`:306`) or `deleteTransferLinePairs` (`:325`) |
| splits → CRUD | `UpdateWithSplits` calls `b.Update` then `b.ReplaceSplits` (`:1005`-ish) inside one `runInTx` |
| status → both | `RestoreVoidedTransactionWithSplits` calls `b.RestoreVoidedTransaction` then `b.ReplaceSplits`; `VoidTransaction` calls `deleteTransferLinePairs` |

Mechanism 1 (field rebind) cannot express a cycle: A holding B and B holding A is
a construction-order cycle whose `InTx` would recurse. Mechanisms 2 and 3 both
mean inventing a *third* owner for logic that already has one.

**(b) Both clusters need both repositories.** `txnRepo` and `splitRepo` are each
used by CRUD, splits, transfer-line helpers, status and duplicate. Neither repo
can be assigned to one side, so a split duplicates both dependencies and gains
nothing in isolation.

**(c) The blast radius buys nothing.** 48 production call sites across 14 files,
24 type-declaration sites across 8 files, 15 sites where the whole `*Service` is
passed to a constructor (TUI/CLI), 165 test call sites across 17 files, plus 23
tests calling unexported `findPairedByTransferID` and 9 poking private fields. And
no concept dies — the same logic, in two types, with a new port between them.

**What does happen:** a pure file split along the banners already in the file,
plus the transfer-line counterpart machinery lifted into its own file. Line counts
measured from the current tree:

| New file | Contents | Lines |
|---|---|---|
| `transaction_service.go` | struct, `InvestmentCounterpartPort`, `NewService`, `InTx`/`q`/`runInTx` | ~145 |
| `service_crud.go` | `Create`, `GetByID`, `Update`, `cascadeAmountToParentSplit`, `Delete`, `ListByAccount`, `ListByAccountAndDateRange` | ~208 |
| `service_splits.go` | `CreateWithSplits`, `GetSplits`, `AddSplit`, `UpdateSplit`, `DeleteSplit`, `ReplaceSplits`, `UpdateWithSplits` | ~413 |
| `service_split_plan.go` | `splitReplacementPlan`, `planSplitReplacement`, `preflightSplitReplacement` | ~110 |
| `service_transfer_line.go` | the 12 counterpart/transfer-line helpers incl. the 3 delete cascades | ~305 |
| `service_status.go` | clear/reconcile/uncleared/unreconcile, void/restore, `BalanceImpact`, `Duplicate` | ~274 |
| `service_validation.go` | `validateTransaction`, `validateSplit`, `validateSplits`, `applyPayeeDefaultCategory`, `ensureAccountOpen` | ~107 |

`service_transfer_line.go` is the file worth having: it collects the one cluster
with a private dependency (`investmentCounterpart`, used by exactly 6 methods and
nothing else) and makes the split↔counterpart contract readable in one place.
That is the honest ceiling for this package — a legible module boundary without a
type boundary.

---

## 4. `corporate_action_service.go` — pure file split, the cheapest win

`CorporateActionService` has **no** dependency on `investment.Service` in either
direction: no `*Service` field, no method call either way. They share six
repositories and are wired from the same instances (`registry.go:115`, `:133`), so
they observe each other only through the database. One shared package-level
function (`netSharesHeldAsOf`, `investment_service.go:1610`) is called by both.

So there is nothing to untangle — only a 1,110-line file with four independent
event families in it:

| New file | Contents | Lines |
|---|---|---|
| `corporate_action_service.go` | struct, ctor, `InTx`/`runInTx`, `ListBySecurity`, `ListAll`, `accountUsesLotsFor` | ~108 |
| `corporate_action_split.go` | `Split`, `SplitLot`, `adjustLots/Positions/Prices`, `CatchUpSplitsFor*`, `securityHasSplitAction`, `rebuildPositionFromLots`, `sharesHeldAsOf` | ~338 |
| `corporate_action_merger.go` | `Merger`, `mergerProcessLots/Positions`, `mergerHideSource` | ~202 |
| `corporate_action_spinoff.go` | `SpinOff`, `spinOffProcessLots/Positions` | ~197 |
| `corporate_action_reversal.go` | `DeleteAction`, `reverseSplit`, `reverseSpinOff`, `checkNoDownstreamEvents`, `downstreamError` | ~240 |

Errors (`DownstreamEventsError`, `UnsupportedReversalError`, 25 lines) move to
`corporate_action.go` beside the other domain types.

While here, note two duplicated concepts for a later pass — **not** this phase:
`CorporateActionService.rebuildPositionFromLots` parallels
`Service.syncPositionAndLots` (`rebuild.go:217`), and `securityHasSplitAction`
parallels `securityHasNonSplitAction` (`split_replay.go:55`). Collapsing them
means deciding which type owns position rebuilding, which is phase 4's question.

---

## 5. `scheduled.Service` — file split, then delete the second posting engine

### 5.1 The duplication, measured

`AutoPost` (`:314`, 183 lines) does not call the posting helpers. Its `runInTx`
closure (`:356`–`:479`, **124 lines**) inlines all three shapes:

| Shape | AutoPost inlines | Duplicating |
|---|---|---|
| single-line transfer | `b.transferPort.CreateTransfer(b.q(), …)` `:373` | `postSingleLineTransfer` (`:683`, 66 lines) |
| multi-line | `b.buildMultiLineTransaction` + `b.txnSvc.CreateWithSplits` `:99`-rel | `postMultiLine` (`:749`, 50 lines) |
| single-line | `transaction.NewTransaction` + `b.txnRepo.Create` | `postSingleLine` (`:619`, 64 lines) |

So ~124 lines of `AutoPost` shadow ~180 lines of `post*`. Two engines for one
concept — exactly the "delete a concept" work item 3 asks for, and the highest
non-cosmetic payoff in this design.

### 5.2 Why naive delegation is wrong — and why it is *not* a nesting problem

A first reading suggests `AutoPost` cannot call `postSingleLine` because each
`post*` opens its own `runInTx` (`:661`, `:721`, `:771`), and nesting `WithTx`
deadlocks. **That reading is wrong, and the design must say so** or an
implementer will hunt the wrong bug. `runInTx` joins when already bound
(`:102`-`:109`): inside `AutoPost`'s closure the receiver `b` has `b.tx != nil`,
so `b.postSingleLine(...)` would return `fn(b)` inline. No nesting, no deadlock.
(A deadlock needs the *unbound* `s` — see §2.2.)

The real blockers are semantic, and there are three:

1. **The estimate reads from different places.** `postSingleLine:634` calls
   `s.EstimateAmount(st.ID)`, which re-reads the row (`EstimateAmount:954` →
   `s.repo.GetByID(id)`). `AutoPost:423` calls `b.estimateAmountForSchedule(st)`
   against the **in-memory `st`** that prior loop iterations have already
   advanced. In a multi-occurrence catch-up these are different values.
2. **Skip is not an error.** `AutoPost` breaks its loop and returns `nil` so the
   transaction still commits the completion mark (`:341-342`, `:473-478`);
   `post*` signal failure by returning an error. Delegating naively turns a skip
   into a rollback.
3. **Advance-and-persist sits at a different level.** Each `post*` does
   `st.AdvanceSchedule()` + `b.repo.Update(st)` **per call** (`:661`-rel).
   `AutoPost` advances per occurrence but persists **once after the loop**, and
   only conditionally (`:473`: `len(result.Transactions) > 0 || (result.Skipped
   && st.IsCompleted())`). A per-occurrence persist would rewrite the schedule
   row N times inside one tx.

### 5.3 The target shape

Apply §2.3. Extract the *bodies* as participants that neither open a transaction
nor persist the schedule:

```go
// participant: writes the posted rows for ONE occurrence, mutates st in memory,
// persists nothing about the schedule, opens no transaction.
func (b *Service) postOccurrence(st *Transaction, date types.Date, amount *types.Money)
    (*transaction.Transaction, []*transaction.Split, error)
```

with the three shapes behind it as `postOccurrenceTransfer` /
`postOccurrenceMultiLine` / `postOccurrenceSingle`. Then:

- `Post`, `PostReturningSplits`, `PostWithDate`, `PostWithEdits`, `Skip` become
  entry points: one `runInTx`, one `postOccurrence` on `b`, then advance +
  persist.
- `AutoPost` becomes an entry point with the per-candidate tx it already has, a
  loop calling `postOccurrence` on `b`, and its existing skip-reason and
  conditional-persist logic preserved verbatim.

Divergence (1) is resolved by giving `postOccurrence` the amount as a parameter —
the entry point decides whether it came from the template, the caller, or an
estimate, so the "re-read vs in-memory" question stops existing. Divergences (2)
and (3) are resolved by construction: participants do not persist and do not
classify skips, so both semantics live only in entry points.

### 5.4 Two divergences §5.2 did not name — found by the differential

Building the differential inventory before writing the collapse turned up two
more shape-level divergences. Both are now resolved in the shipped code:

1. **Dispatch order was inverted.** The manual path tested `len(st.Splits) > 0`
   first; `AutoPost` tested `st.IsTransfer()` first. Unreachable, and provably
   so: `Transaction.Validate` (`scheduled.go:693`) refuses a transfer schedule
   that also carries split lines, and `Repository.Create`/`Update` are called
   only from the two service methods that run it. The participant dispatches
   transfer-first and says why.
2. **`finalizeLoanPayoff` ran on some paths and not others.** `postMultiLine`,
   `PostWithEdits` and `AutoPost` called it; `postSingleLine` and
   `postSingleLineTransfer` did not. Calling it uniformly from the entry point is
   a provable no-op for the two paths that lacked it, because `IsLoanShaped`
   short-circuits on `len(st.Splits) == 0` (`loan_shape.go:98`) before touching
   the database. That is what lets one envelope serve all three shapes.

Two deliberate behavior changes, neither pinned by any test:

- **The transfer leg's `GetByID` error is now propagated on both paths.**
  `AutoPost` used to discard it (`gerr == nil` guard), which left the occurrence
  unrecorded, which made the persist gate false, which committed the transfer
  legs while leaving `next_date` unadvanced — a silent duplicate-post on the next
  open. Propagating rolls the candidate back instead.
- **Error wrap text is unified** on the manual path's wording. `errors.Is`/`As`
  are unaffected; no test asserted the strings. Note that the *blast radius*
  behind those strings still differs by design — a manual post fails alone, an
  auto-post failure aborts the batch.

### 5.5 Three live bugs found while collapsing — FIXED 2026-08-07, separately

The differential surfaced these. Each predates this work and none is caused by
the collapse, so each was kept out of the behavior-preserving refactor and fixed
afterwards, in its own commit, with a failing test written first.

**The root cause behind all three is one idea, stated two ways.** Bugs 2 and 3
are the same mistake seen from either side: *a posted occurrence that produces no
regular-ledger row is indistinguishable from no occurrence at all*. Bug 1 is its
sibling in the date domain: *a schedule that has run out is indistinguishable
from one that is merely due*, because the terminal state the type describes was
unreachable. Both are cases of an inferred signal standing in for a real one.

The fixes replace the inference with the signal in each case: `AutoPost` counts
occurrences instead of counting posted rows, and `AdvanceSchedule` reports
"that was the last one" instead of leaving callers to deduce it.

1. **`AutoPost` hangs on an end-date-bounded schedule.** When the next occurrence
   would fall past `end_date`, `AdvanceSchedule` returns false **without
   assigning `st.NextDate`** (`scheduled.go:428-432`; the assignment is at
   `:434`, after the early return). `IsCompleted` then tests the *unadvanced*
   `NextDate` against `end_date` and returns false (`:378-386`), and
   `isAutoPostDue` still returns true — so `AutoPost`'s loop condition can never
   go false. It spins inside one open transaction, inserting rows that never
   commit, until memory or disk runs out. The user sees a hang on file open.
   Schedules bounded by `occurrences` instead are fine: `OccurrencesRemaining`
   reaching 0 makes `IsCompleted` true. Fixing it means assigning `NextDate`
   before the end-date check, or having `IsCompleted` consult
   `CalculateNextDate`.
2. **An investment↔investment transfer schedule re-posts on every open.** Such an
   occurrence has no regular-ledger leg, so `result.Transactions` stays empty, so
   the persist gate `len(result.Transactions) > 0 || (result.Skipped &&
   st.IsCompleted())` is false, so the advance made in memory is never written.
   `PostedCount` is still incremented, so the TUI reports success and pushes an
   undo entry that iterates an empty `Results` slice, restores nothing, and
   returns nil. Fixing it means gating the persist and the `summary.Results`
   admission on an occurrence count rather than on the posted-row count.
3. **`tmoney scheduled post <id>` panics on an inv↔inv transfer schedule.**
   `cli/scheduled/post.go` dereferences `txn.Amount` and `txn.Date` after
   checking only `err`, and the manual path legitimately returns a nil
   transaction for an occurrence with no regular-ledger leg.

**What the fixes were.**

1. `AdvanceSchedule` now assigns `next_date` unconditionally, *including* past
   `end_date`, and returns whether the schedule can still run. That makes the
   terminal state `IsCompleted` already tested for actually reachable. Two other
   pieces of the codebase turn out to have been written expecting exactly this
   and were dead alongside it: `IsCompleted`'s end-date branch
   (`scheduled.go:382`) and `ListDue`'s `next_date <= end_date` SQL predicate
   (`scheduled_repository.go:258`). Validation permits the new state — the only
   `end_date` rule is that it follow `start_date`. `AutoPost` additionally
   **honours the return value**, which no caller previously did, so its loop now
   has two independent reasons to stop; if `IsCompleted` and `AdvanceSchedule`
   ever disagree again the loop terminates instead of spinning.
2. `AutoPost` counts **occurrences**, and that count gates both the schedule
   persist and the `summary.Results` admission. `len(result.Transactions)` was
   never the right question.
3. The CLI falls back to the schedule's own amount and the posted date when
   there is no regular-ledger row, and says so in its output.
   `PostScheduledTransactionCommand.Undo` gained the matching nil guard.

**An adversarial review of those three fixes found four more, and one of them
showed the first attempt was incomplete.** All are now fixed:

4. **The same bug survived at the manual door.**
   `PostScheduledTransferCommand` learned the posted transfer's id by reading it
   *off* the regular-ledger leg — so for an investment↔investment occurrence it
   got nothing, `Undo` skipped its delete, and the schedule was rewound anyway.
   The undo reported success while leaving both legs in the ledger, so the next
   post duplicated the transfer. The command's own doc comment claimed "Undo
   addresses the transfer, not a leg, so it works for every shape"; the code took
   it from a leg. Fixed by surfacing `transferID` out of the posting path
   (`PostWithDateReturningTransfer`), which is where `postOccurrence` already had
   it. This also stops the preview's one-off memo and status being silently
   dropped for that shape.
5. **Fixing bug 1 moved a phantom rather than removing it.** With `next_date`
   now landing past `end_date`, a completed schedule left `ListDue` — which
   already excluded it — and appeared in `ListUpcoming`, which carried no
   completion predicate at all. Its `next_date` is frozen while every live
   schedule's keeps moving, so it migrated to the front of an ascending sort and
   held a slot in the dashboard's upcoming panel permanently. `ListAutoPostDue`
   had the same gap. Both now carry `ListDue`'s two clauses, so all three
   predicates agree with `IsCompleted`.
6. **`AdvanceSchedule`'s *other* terminal route had the same shape.** The
   occurrences branch also reported terminality by withholding the advance, so
   re-arming a finished schedule through the TUI's Occurrences field re-posted the
   occurrence it had already posted. Both routes now advance and merely *report*
   terminality.
7. **The CLI printed a transfer's amount from the wrong frame.** The returned row
   is whichever leg landed on the regular ledger — the *destination* when the
   source is an investment account — so its sign belonged to a different account
   than the one named above it. Transfers now display the schedule's own signed
   amount, and the summary states the destination instead of leaving direction to
   be inferred from a sign.

**The three items left open above are now closed, 2026-08-07.**

8. **`PostScheduledTransactionCommand` deleted.** It had no production caller and
   was superseded rather than forgotten: posting from the TUI always goes through
   the preview dialog, which uses `PostScheduledTransferCommand` for a transfer
   and `PostScheduledTransactionWithEditsCommand` for every other shape. That
   distinction decided it — a command with no caller can mean forgotten wiring,
   in which case the fix is to wire it up. Leaving it was not neutral: its `Undo`
   deleted only the regular-ledger row, so the nil guard added in fix 4 was
   standing in for a delete that should have happened.
9. **An inv↔inv transfer schedule can no longer carry a category.** Refused at
   creation and edit by calling `transfer.Kind.StoresCategory()` (the shape
   `cli/transfer/add.go` already uses, so the rule keeps one implementation);
   existing rows cleared by `HealTransferCategories` on file open, beside
   `HealAllAccounts` and `HealNextDates`; and auto-post now skips such a schedule
   with a reason instead of aborting the whole batch. The third is not redundant:
   `account edit --type investment` can create a fresh one *after* the heal has
   run. `internal/scheduled` cannot import `internal/transfer`, so the rule is
   reached through `TransferPort`, which gains a `StoresCategory` method.
10. **The TUI transfer schedule dialogs now offer every account.** The exclusion
    of investment accounts made sense only while scheduled transfers were
    regular↔regular; posting has routed through the transfer owner since phase 4
    of the transfer design. It was also actively corrupting data: a CLI-created
    inv↔inv schedule found neither endpoint in the picker, `indexOfID` returned 0
    for both, and saving silently re-pointed the transfer to the first bank
    account. `indexOfID` now reports −1, the account pickers treat that as a
    refusal and the category combo as "(None)", and the save path refuses a
    category on the one pair that cannot store it.

File split for the package (phase 2, before the collapse):

| New file | Contents | Lines |
|---|---|---|
| `scheduled_service.go` | struct, wiring, closed-account guards, CRUD, `IsDue`/`IsCompleted`/`GetNextDate`/`CalculateNextDate` | ~322 |
| `posting.go` | `Post*`, `postSingleLine*`, `postMultiLine`, `buildMultiLineTransaction`, `Skip` | ~410 |
| `auto_post.go` | `AutoPostResult`, `AutoPostSummary`, `AutoPost`, `isAutoPostDue`, `estimateAmountForSchedule` | ~254 |
| `estimate.go` | `EstimateAmount`, `getRecentTransactionsByPayee` | ~93 |
| `validation.go` | the three validators | ~95 |

Loan extraction is **not** in the file-split phase. The eight `loan_*.go` methods
on `Service` depend on only `accountRepo` plus `ListReferencing`, and
`loan_shape.go:22` already shows the seam (`AccountLookup` func type) — but it is
a type-shaped change and belongs after phase 3, if at all.

---

## 6. `investment.Service` — the one real type extraction

### 6.1 What can leave, and what cannot

The load-bearing measurement: `runInTx` and `healInOwnTx` transitively require
**all 10 struct fields** (`healInOwnTx` → `syncPositionAndLots` → the rebuild
machinery). So any cluster that opens a transaction or heals must keep the whole
struct. That draws the line:

| Cluster | Fields used | Verdict |
|---|---|---|
| counterpart port (4 methods, 94 ln) | `repo`, `accountRepo`, `InTx` | **extract** — already behind an interface, 1 consumer |
| valuation + total_return (22 methods) | read-side repos only | **extract** — no `runInTx` anywhere |
| edit family (10 `Update*`, ~338 ln) | needs a bound core service | **extract** — acyclic edit→core |
| cash ops (5 methods, 148 ln) | `repo` + 2 guards; **no tx at all** | extract *optional* — low value alone |
| trades, `TransferShares`, `DeleteTransaction`, rebuild, heal, backfill | all 10 fields via `runInTx`/`healInOwnTx` | **stays one type** |

### 6.2 The edit family's exact seam

Every `Update*` reapplies by calling the create method on the tx-bound `b`
(`update_edit.go:258` `b.Buy`, `:292` `b.Sell`, `:399` `b.Dividend`, …), and
nothing in the create path calls an `Update*`. So edit→core is acyclic and
mechanism 1 (field rebind) applies: `EditService{core *Service}` with
`InTx` doing `c.core = s.core.InTx(tx)`.

One correction to make before splitting: `DeleteTransaction`
(`investment_service.go:1447`, `:1464`) calls `reverseTxnEffects`, which lives in
`update_edit.go`. The reverse family is therefore **not** exclusive to editing. So
`update_edit.go` splits by owner, not by file:

- `reverse.go` (~222 ln) — `reverseTxnEffects`, `reverseShareAddition`,
  `reverseShareRemoval`, `reverseTransferShares`, `loadAndReverseForEdit`,
  `guardEditByOldID` → **stays on `Service`**, because delete needs them.
- `edit.go` (~338 ln) — the 10 `Update*` entry points → **moves to
  `EditService`**.

### 6.3 Blast radius

~104 production call sites across 38 files (33 in `cli/investment`, 35 in `tui`),
plus `dispatchUpdate` (`cli/investment/edit.go:338`) which takes
`*investment.Service` and makes 9 `Update*` calls — that signature becomes
`*investment.EditService`. On the test side: `createFullTestService` is used at
251 sites across 13 files, and 41 tests call unexported methods (38 of them in
`valuation_total_return_test.go` against `total_return.go`). Staying in
`package investment` keeps all 41 compiling with a receiver change only; a new
package would require exporting them.

Also of note: the counterpart extraction changes nothing at the consumer.
`transaction.Service` depends on the `InvestmentCounterpartPort` interface it
declares itself (`transaction_service.go:43`), so a new type satisfying the same
four methods is a one-line change in `registry.go:125`.

### 6.4 File split first (phase 2), independent of any of the above

| New file | Contents | Lines |
|---|---|---|
| `investment_service.go` | struct, ctor, `InTx`/`runInTx`/`healInOwnTx`, the 4 guards, `SetClearedStatus` | ~197 |
| `cash_ops.go` | `Deposit`, `Withdrawal`, `Interest`, `Fee`, `Dividend` | ~148 |
| `trades.go` | `Buy`, `Sell`, `sellWith*`, `fifoLotAllocations` | ~309 |
| `trades_income.go` | `ReinvestDividend`, `FeeLiquidation`, `feeLiquidationWith*` | ~269 |
| `counterpart.go` | the 4 port methods | ~94 |
| `transfer_shares.go` | `TransferShares` (one 246-line method) | ~246 |
| `delete.go` | `DeleteTransaction` | ~104 |
| `queries.go` | `GetCashBalance`, `TotalSharesForSecurity`, `SharesBySecurity*`, `netSharesHeldAsOf` | ~202 |
| `auto_price.go` | `autoCreatePrice`, `cleanupAutoPrice` | ~81 |

The two error types (24 ln) move to the existing `investment_errors.go`.
`TransferShares` at 246 lines in one function is flagged here as the next-largest
single unit of complexity in the package; splitting *it* is not in scope.

---

## 7. Rollout plan

Each phase compiles, passes the full suite, and is a separate commit to main.
Phase 1a is deletion, 1b–2 are code motion with no behavior change; 3–4 change
structure and carry fault-injection gates.

**Phase 0 — free deletions (risk: none) — SHIPPED 2026-08-07.**
§1.3. −522/+14 lines, full suite green.
*Exit criteria (met):* `go build ./... && go vet ./... && go test ./...` green;
`grep -rn 'rejectInvestmentAccount\|SearchByPayee\|SearchByMemo\|SearchByCategory'`
returns nothing outside history.

**Phase 1a — tier-2 deletions and their test rewiring (risk: low-medium).**
§1.4, decided. Delete the four methods; rewire the ~21 fixture sites onto
`txnRepo.UpdateStatus`; trim the reconcile legs from the two lifecycle tests; add
`txnRepo` to `createReconTestEnv`. **Kept separate from 1b on purpose:** 33 test
edits inside a "pure motion" commit destroys the one property that makes the file
split trivially reviewable.
*Exit criteria:* build + full suite green. Every surviving test that previously
reconciled through the service still asserts the same thing about the same state
— reviewed site by site, since a rewire that quietly changes the fixture's status
turns a real guard test into a tautology. `grep -rn 'ReconcileTransaction\|UnReconcileTransaction\|\.Duplicate(\|ValidateSplitTotals' internal/ tests/`
returns nothing.

**Phase 1b — `transaction` + `corporate_action` file splits (risk: low).**
SHIPPED 2026-08-07. §3 and §4. Pure `git mv`-style motion within each package.
*Exit criteria:* build + full suite green with **zero test-file edits**. No
**service** file in either package over 450 non-test lines. `git log --stat`
shows only moves — any behavioral diff in this phase is a bug.

*Scope correction, made when the criterion was checked.* An earlier draft said
"no file in either package over 450 non-test lines". §3 and §4's tables cannot
deliver that, and never could: they cover only `transaction_service.go` and
`corporate_action_service.go`. What stays over 450 after this phase is
`transaction_repository.go` (627), `transaction.go` (519),
`investment/update_edit.go` (574), `backfill.go` (562), `valuation_service.go`
(530) and `investment.go` (503) — repositories and domain models, none of them
a god service, and repository-boundary work is an explicit non-goal (§10).
`investment_service.go` (1,673) is phase 2's job. The criterion is therefore
about service files, which all now land between 102 and 381 lines.

*How pure motion was proved,* rather than asserted: every top-level declaration
was sliced from the original source **bytes** — doc comment included — instead of
being re-printed, then the before and after sets were dumped, sorted by
declaration name and diffed. Both packages came back byte-identical across all
80 declarations. The only text that did not survive is five section-banner
comment blocks in `transaction_service.go`, each of which is now the file-level
doc comment of the file named after it.

**Phase 2 — `investment` + `scheduled` file splits (risk: low).**
SHIPPED 2026-08-07. §5.3 table and §6.4. Same discipline, same byte-identity
proof. Explicitly **not** loan extraction, **not** the AutoPost collapse.
*Exit criteria:* as phase 1b. `investment_service.go` under 200 lines;
`scheduled_service.go` under 350.

*Result:* `scheduled_service.go` 315 ✓. `investment_service.go` **203**, three
over. The 200 came from §6.4's ~197 content estimate, which budgeted no
file-level doc comment; the file carries an eight-line one recording why
`runInTx`/`healInOwnTx` need all ten fields, since that is the measurement
phase 4 turns on. Keeping the comment was judged worth three lines. Every other
file landed well inside: `investment` 94–317 across nine files,
`scheduled` 100–427 across five. `scheduled/posting.go` at 427 is the largest
and is phase 3's subject anyway.

**Phase 3 — collapse the second posting engine (risk: medium).**
SHIPPED 2026-08-07. §5.3. Extract `postOccurrence*` participants; re-point
`Post*`/`Skip`/`AutoPost` onto them as entry points; delete the 124-line inlined
engine. Measured, comments excluded: 417 → 381 lines, and one engine where there
were two. See §5.4 for the two divergences §5.2 did not name, and §5.5 for three
live bugs the differential turned up that are **not** fixed here.
*Exit criteria:* build + full suite green **unmodified** — the existing
`scheduled_service_test.go` (1,616 ln) and `scheduled_transfer_*_test.go` are the
differential test, since the observable behavior of both engines must be
identical. Plus: `TestPost_DoublePostRegression`
(`scheduled_service_tx_test.go:41`) still green; a new fault-injection test
failing the Nth write inside a multi-occurrence `AutoPost` candidate and asserting
the whole candidate rolled back while earlier candidates stayed committed; a new
test asserting a variable-amount schedule in a multi-occurrence catch-up gets the
same amounts before and after the collapse (this is divergence (1)); and
`grep -n 'transferPort.CreateTransfer\|txnSvc.CreateWithSplits' auto_post.go`
returns nothing.

**Phase 4 — investment type extractions, in ascending risk (risk: medium→high).**
SHIPPED 2026-08-07, all three. Three separate commits, in this order, each
independently revertable:

*What the three turned out to have in common,* which §6 did not predict: each new
type is defined by something it **cannot** do, and in each case the guarantee is
enforced by what the struct omits rather than by a convention.
`CounterpartService` holds two repositories and no `*db.DB`, so it cannot open a
transaction — only join the one it is handed. `ValuationService` holds eight and
no `*db.DB`, so it cannot write at all. `EditService` holds exactly one field,
the core, so "InTx rebinds every field" is checkable in three lines.

*A correction to §6.1.* It described the valuation cluster as using "read-side
repos only", which reads like a small dependency set. It is eight of the ten
struct fields — everything except `db` and `tx`. The extraction is still right,
but for a different reason than the field count: the cluster is a **leaf**, with
no inbound call from any write path in the package, and every production caller
is a view, a CLI command or a report. That is what makes it safe to give a type
with no transaction plumbing at all.

*No guard was duplicated to get any of this.* Seven helpers became package-level
functions taking the repository the caller is bound to —
`requireOpenInvestmentAccount`, `requireOpenAccount`, `loadInvestmentAccount`,
`validateInvestmentTransaction` (guards.go), plus `cashBalanceOf`,
`splitEventsFor` and `securityHasNonSplitActionIn`. `Service`'s methods delegate
to them and keep their existing callers.

*`Service` sheds 36 methods and keeps all ten fields.* That is the honest
outcome: `runInTx`/`healInOwnTx` still need the whole struct (§6.1), so the win
is ownership and testability, not a smaller core.
1. **Counterpart** (`CounterpartService`, 4 methods) — proves the pattern against
   the smallest surface; one-line consumer change.
2. **Valuation** (`ValuationService`, valuation + total_return) — read-only, so
   no tx question; largest test-receiver churn (38 sites).
3. **Edit family** (`EditService`, holding a rebindable core) — the only one that
   must share a transaction with the core; `reverse.go` stays on `Service` (§6.2).
*Exit criteria, per commit:* build + full suite green; the new type's `InTx`
rebinds **every** field it holds (reviewed line by line against the struct); a
fault-injection test per extracted write path asserting a mid-write failure
leaves zero partial state; and a test that the extracted type invoked from inside
the core's open transaction joins it rather than opening a second one. Stop after
any commit if the plumbing cost exceeds the legibility gain — the three are
independent by construction.

**Phase 5 — none. Recorded non-goal.** §3. `transaction.Service` stays one type.

---

## 8. Testing strategy

- **Phases 1b–2 need no new tests, and that is the point.** A file split whose
  test suite needs editing was not a file split.
- **Phase 1a needs no new tests either, but 33 existing ones must be reviewed
  individually** (§1.4). It writes nothing new; it re-points fixtures at
  `txnRepo.UpdateStatus` and trims two lifecycle tests. The risk is a silently
  weakened assertion, not a missing one, so the review question per site is "does
  this test still set up the same state and assert the same thing?" — a diff that
  merely compiles and passes proves nothing here.
- **Phase 3's regression net is the existing suite, unmodified.** Both posting
  engines are supposed to be observationally identical, so 1,616 lines of
  existing scheduled tests plus the transfer/loan posting tests *are* the
  differential. New tests cover only what the existing suite cannot see: the
  multi-occurrence estimate path (divergence 1) and the rollback boundary.
- **Fault injection per extracted write path** (phases 3–4), following
  `design-withtx.md` §8: a `failingQueryer` wrapper erroring on the Nth `Exec`,
  wired as `database.WithTx(tx => svc.InTx(failWrap(tx)).Method(...))`, asserting
  the flow errors *and* a fresh read shows zero partial state. Existing examples
  to copy: `scheduled_service_tx_test.go:20-39`, `transaction_service_tx_test.go`,
  `investment_service_tx_test.go`.
- **A binding test per new type** (phase 4), which is the gap the current suite
  has: call the extracted type from inside the core's open transaction and assert
  it joined (one commit, no deadlock) rather than opening a second transaction.
  This is the only automated check that §2.2's silent failure did not happen.
- **No new arch test is proposed.** `internal/transfer/arch_test.go` is a text
  scan with no notion of file size or type ownership; extending it to police
  decomposition would encode this design's file names as build-breaking law. Two
  of its existing guards do apply and must stay green: no `internal/transaction`
  import in `internal/investment` non-test files (`:124`), and no method named any
  of the 16 banned transfer identifiers (`:78-82`) — so no extracted type may
  expose a `CreateTransfer`.

---

## 9. Estimated line delta

Production, non-test. Function extents measured from the tree; ±10%.

> **These estimates were wrong.** The actual figure for phases 1a–4 is +432
> production lines, not +100, and §11.2 explains why: a third of everything added
> is comments, which this table did not cost. Kept unedited as the record of what
> was predicted.

| Phase | Deleted | Added | Net |
|---|---|---|---|
| 0 (shipped) | 522 (incl. 356 test) | 14 | **−508** |
| 1a — 4 tier-2 deletions | 118 | 0 (test-only churn: ~33 sites) | **−118** |
| 1b — file splits | 0 | ~90 (7 file headers + imports) | +90 |
| 2 — file splits | 0 | ~120 (14 file headers + imports) | +120 |
| 3 — posting collapse | ~180 (`post*` bodies) + 124 (inlined engine) = 304 | ~200 (participants + entry points) | **−104** |
| 4 — three extractions | ~40 (dedup'd guards) | ~150 (3× plumbing + 3 ctors + registry) | +110 |

Net across phases 1–4: roughly **+100 production lines** for one deleted engine,
three smaller types, and 21 files where there were 4. The file-header cost is
real and worth naming: decomposition is not a line-count win, it is a
navigation and ownership win. The only genuine deletion is phase 3.

---

## 10. What this deliberately does not do

- **Does not split `transaction.Service` by type** (§3) — measured as mutual
  coupling with no concept deleted.
- **Does not build a shared unit-of-work.** DECIDED after phase 4: not warranted,
  and the reason is better than "it didn't hurt enough". The boilerplate never
  materialised because each extracted type was defined by what it *cannot* do, so
  none of them needed the full triad. `CounterpartService` holds two fields and
  has no `runInTx` at all — it takes the caller's `db.Queryer` per call.
  `ValuationService` has no `InTx` either, because it never writes.
  `EditService` holds exactly one field. Total plumbing across all three: one
  three-line `InTx`. A shared unit-of-work would have abstracted a cost that
  never arrived, and §2.1's warning stands — inventing it early is how this
  design would have grown a fifth god type.
- **Does not extract the loan methods** from `scheduled.Service`. DECIDED after
  phase 3, and the measurement reverses the original "the seam is real". It is
  not: the dependency is **mutual**, which is the §3 pattern. The core calls the
  loan cluster (`postManually` and `AutoPost` both call `finalizeLoanPayoff`, and
  `buildMultiLineTransaction` calls `buildLoanTransaction`), and the loan cluster
  calls the core (`FindLoanSchedule` → `ListReferencing` →
  `repo.List`). Breaking the cycle means giving a `LoanService` its own `repo`
  and its own copy of the referencing walk — duplicating a concept to extract a
  type, which is the trade this design refuses everywhere else. The file boundary
  already delivers the navigability: `loan_build.go`, `loan_posting.go`,
  `loan_projection.go` and `loan_shape.go` are 8 methods across four named files,
  and `scheduled_service.go` is 315 lines.
- **Does not touch the trade/heal/rebuild core of `investment.Service`.**
  `runInTx`/`healInOwnTx` need all 10 fields; splitting them would mean sharing
  the whole struct through a port, which is the §3 mistake in a different
  package.
- **Does not split `TransferShares`** (238 lines, one function) or
  `reverseSpinOff` (124) — still flagged, still not scheduled, and now with a
  reason rather than a shrug. Each owns its own file at 261 and 258 lines, so
  the navigability problem this design set out to solve is gone; what is left is
  one long function each, and no defect found during any phase traced to either.
  Splitting a 238-line function that nothing has gone wrong in is churn with a
  risk budget and no return. Revisit if a bug lands in one.
- **Does not collapse the position-rebuild concepts** between
  `CorporateActionService.rebuildPositionFromLots` and
  `Service.syncPositionAndLots` (§4) — because on inspection **they are not
  duplicates**, and §4's "parallels" was wrong. `rebuildPositionFromLots` is 13
  lines that sum the existing lots into a position.
  `syncPositionAndLots` replays the *ledger* for an (account, security), gating
  on corporate-action involvement and applying split events as it goes. They
  answer different questions from different sources; the only thing they share is
  writing a position at the end. No owner decision is needed, because there is
  nothing to own jointly. This item is retired, not deferred.
- **Did collapse one real duplication §4 flagged.**
  `securityHasSplitAction` and `securityHasNonSplitAction` were the same query
  and the same loop with opposite predicates — one question asked twice. Both now
  delegate to `securityHasAction(repo, id, match)`, with `isSplitAction` naming
  the predicate they disagreed about.
- **Does not address review items 5 or 6** — repositories on `app.Services`, or
  the heal-on-open side effects in `NewServices`. Phase 4 touches `registry.go`
  and will be tempted; resist.
- **Does not add CI or size linting.** `.golangci.yml` enables no size linters,
  the Makefile has no `lint` target, and `.github/` holds only `dependabot.yml`.
  Nothing mechanically enforces the 450-line ceiling this design proposes; it is
  a review convention, and stating that honestly is better than pretending a
  guard exists.

---

## 11. Outcome — what the design got right, and what it got wrong

Written after every phase shipped, measured rather than remembered.

### 11.1 The goals

| Goal | Result |
|---|---|
| No **service** file over ~450 lines in the three packages | Met, with one exception named below |
| `scheduled.Service` has one posting engine | Met — the inlined engine is gone |
| `investment.Service` sheds the separable clusters | Met — three types, 36 methods |
| The `transaction.Service` type split stays rejected in writing | Met — §3 stands, unamended |

The exception is `investment/backfill.go` at 562 lines. It was never in §3's or
§6.4's tables and was 562 lines before this work started; it is a service file
over the ceiling, and saying so is better than quietly redefining the ceiling.

Non-test file counts: `transaction` 6 → 12, `investment` 23 → 38, `scheduled`
11 → 16.

### 11.2 The line-count estimate was wrong, and instructively so

§9 estimated **+100 production lines** across phases 1a–4. The actual figure is
**+432**. The gap is almost entirely prose: of the 7,240 production lines added
across those phases, **2,382 are comments or blanks** — a third. §9 costed file
headers and forgot that a file split is mostly an opportunity to write down why
each cluster is a cluster, and that a type extraction is mostly an opportunity to
write down what the new type cannot do.

That is not overrun to apologise for; it is the deliverable. But the honest
lesson is that §9's framing — "decomposition is not a line-count win, it is a
navigation and ownership win" — should have carried a line-count estimate that
reflected its own claim.

The one genuine deletion the design predicted did arrive: phase 3 took the
posting engines from 417 to 381 lines of code, comments excluded, and left one
engine where there were two.

### 11.3 What the design did not predict

- **Every extracted type turned out to be defined by what it cannot do.** §2.3
  framed this as a discipline to apply (participants vs entry points); it turned
  out to be a property to *discover*. `CounterpartService` cannot open a
  transaction, `ValuationService` cannot write, `EditService` holds one field.
  Each guarantee is enforced by what the struct omits, not by a convention, and
  each made its test trivial to state. That is the reusable finding.
- **Two of the three phase-4 exit criteria did not apply to `ValuationService`,**
  because a type with no `InTx` and no writes has no field-rebinding to review
  and no write path to fault-inject. The substitute — a mechanical proof that it
  writes nothing, with an anti-vacuity check that the wrapper saw reads — is a
  better test than either criterion would have produced.
- **The differential inventory was worth more than the refactor.** Phase 3
  required enumerating every way the two posting engines differed. That
  enumeration found seven live bugs (§5.5), including one that hung the
  application on file open and two that silently duplicated money. None was
  caused by the refactor; all had been latent for as long as the duplication had.
  The lesson generalises: *a duplicated concept is a place where two answers
  disagree, and enumerating the disagreements finds bugs whether or not you then
  delete one of them.*
- **§4's "duplicated position-rebuild concepts" was a false positive**, and one
  of its two claimed duplications was real (§10). Flagging suspected duplication
  by name is cheap and useful; asserting it without reading both bodies is how a
  design acquires a permanent, wrong TODO.

### 11.4 What is left

Nothing in this design. §10 now carries a decision for every item, and the three
that were conditional on a phase completing — the unit of work, the loan
extraction, the position-rebuild collapse — are decided against, with the
measurement that decided them. Review items 5 and 6 remain untouched and remain
somebody else's design.
