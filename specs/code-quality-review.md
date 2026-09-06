# Code quality review: tmoney

**Date:** 2026-07-23  
**Scope:** Whole-project structural review (working tree clean aside from untracked tooling).  
**Bar:** Strict maintainability review — structural simplification, god-files, spaghetti growth, boundary cleanliness. Not a PR diff review.

---

## Summary

The codebase is serious software: clear feature slices, disciplined error types, heavy test coverage. The main problems are **not style**. They are structural:

1. Multi-row money operations are **not atomic** (no domain-level SQL transactions).
2. A **dual-ledger model** (`transactions` vs `investment_transactions`) multiplies transfer paths into every presentation surface.
3. Several **god modules** have long since crossed the 1k-line line and keep absorbing special cases.
4. Cross-cutting rules (closed accounts, transfer labels, open/heal side effects) are **copied rather than centralized**.

Approval bar for “healthy core”: **not met** until the atomicity model and transfer ownership are cleaned up. Presentation decomposition is secondary but real.

---

## 1. No domain SQL transactions — compensate-and-pray is the architecture

**Severity: structural / correctness**

`Begin`/`Commit` appear only for migrations and metadata init. There is **no** `WithTx` (or equivalent) used by services or repositories. Multi-write flows rely on manual cleanup:

```go
// internal/transaction/transfer_repository.go
// Create the to transaction. If this fails, manually unwind the from
// transaction — the two Create calls are not wrapped in a SQL transaction,
// so partial state is otherwise possible.
if err := r.txnRepo.Create(pair.ToTransaction); err != nil {
	_ = r.txnRepo.Delete(pair.FromTransaction.ID)
	return fmt.Errorf("failed to create to transaction: %w", err)
}
```

Same pattern in `CreateWithSplits` / `ReplaceSplits` (best-effort `rollbackTransferLinePairs`), cash transfers (`_ = s.repo.Delete(invTxn.ID)`), and investment edit reverse/reapply.

Worse: **Buy does not even compensate**. If the transaction row lands and lot/position write fails, the function returns an error with an orphan buy still in the DB:

```go
// internal/investment/investment_service.go (Buy)
if err := s.repo.Create(txn); err != nil {
	return nil, fmt.Errorf("failed to create buy transaction: %w", err)
}

// Update position or create lot based on account tracking mode
if acct.TrackLots {
	lot := NewLot(...)
	if err := s.lotRepo.Create(&lot); err != nil {
		return nil, fmt.Errorf("failed to create lot: %w", err) // txn row remains
	}
}
```

`ReplaceSplits` is multi-phase (delete counterparts → delete splits → recreate → mint new counterparts). Preflight reduces risk; it does **not** make the write atomic. A mid-flight failure still leaves half-applied state, and rollbacks **swallow** errors (`_ = delete`).

**Code-judo move:** introduce a single `db.WithTx(ctx, fn)` (or repository methods that take `*sql.Tx` / a unit-of-work). Every multi-row domain mutation should commit once or not at all. That deletes most `rollback*` helpers, cleanup branches in transfer paths, and a large class of “and rollback failed” edit paths in `update_edit.go`.

Until that exists, every new multi-leg feature will grow more spaghetti compensation.

---

## 2. Dual ledgers force a 4-path transfer graph that leaks everywhere

**Severity: structural / missed simplification**

Cash movement lives in two tables and four service entry points:

| Path | Service method |
|------|----------------|
| bank→bank | `transaction.Service.CreateTransfer` |
| inv→bank | `investment.Service.TransferCash` |
| bank→inv | `investment.Service.DepositFromAccount` |
| inv→inv | `investment.Service.TransferCashBetweenInvestments` |

`ChooseTransferDispatch` is the right *classifier*, but it is not an *owner*. The presentation layer still re-implements the full switch:

- TUI: `transfer_dialog.go` (~80 lines of dispatch + undo commands)
- CLI: `dispatchTransferAdd` in `cli/transfer/add.go` (and parallel edit/delete/resolve)
- Undo: separate command types per path
- Split lines: `InvestmentCashCounterpartAdapter` so `transaction` can mint inv rows without importing `investment`

`TransferCash` and `DepositFromAccount` are essentially the same function with sign/direction flipped (~170 lines of near-duplicate validation and create logic).

**Code-judo move:** one **Transfer service** (or one `CreateCashTransfer(from, to, …)` on a façade that both CLI and TUI call) that:

1. Classifies once.
2. Writes both legs.
3. Owns category rules (including inv→inv rejection).
4. Returns a single result shape (`TransferID`, from/to leg IDs, which tables).

Then TUI/CLI become:

```go
res, err := svc.Transfer.Create(from, to, date, amount, memo, categoryID)
```

No 4-way switches in presentation. No four undo factories if undo wraps one service call. The adapter can shrink to an internal detail of that façade rather than a cycle-breaking bolt-on on `transaction.Service`.

As long as “unified transfer UX” is implemented by re-dispatching at every edge, every transfer feature (edit, void, category, scheduled, paycheck lines) will keep adding special cases in multiple packages.

---

## 3. God services: domain logic past healthy file size

**Severity: file-size / modularity (blocker under this skill’s rules)**

| File | Lines (non-test) |
|------|------------------|
| `internal/transaction/transaction_service.go` | **1898** |
| `internal/investment/investment_service.go` | **1839** |
| `internal/investment/corporate_action_service.go` | **1040** |
| `internal/scheduled/scheduled_service.go` | **1029** |
| `internal/investment/update_edit.go` | 746 (same service, second god-file) |

`transaction.Service` owns plain CRUD, split lifecycle, transfer-line pairing, investment adapter routing, transfer pair CRUD, status/void/restore, duplication, and payee defaults. That is several bounded contexts jammed into one type.

`investment.Service` owns cash ops, securities trades, lots/positions, cash transfers (3 variants), share transfers, delete, share queries, plus edit/rebuild/heal in sibling files on the same type.

**Push:** split along **operation boundaries**, not random file chunks:

- `transaction`: `Service` (simple txn) / `SplitService` / `TransferService` (regular pairs + lines)
- `investment`: cash ops / trade ops / transfer ops / edit-reverse-reapply / valuation (valuation is already partly split)

Do not “move methods around” without deleting concepts. Prefer one transfer owner (finding #2) so half of `transaction_service` and a third of `investment_service` collapse into one place.

---

## 4. TUI god-objects: dialog state on `App`, wizards past 1k–1.8k lines

**Severity: file-size / spaghetti growth**

| File | Lines |
|------|-------|
| `paycheck_wizard.go` | **1885** |
| `loan_wizard.go` | **1287** |
| `split_dialog.go` | **1171** |
| `scheduled_preview_dialog.go` | **1149** |
| `price_view.go` | **1116** |
| `scheduled_dialog.go` | **1031** |
| `app_update.go` | **1021** |

`App` is a mega-struct: every dialog’s fields, category-create routing (`createCatSource`, split row, paycheck line, loan field), investment dialog fleets, sticky dates, etc. New features add more fields and more `switch` arms in `Update` / key handlers.

**Push:** extract **view/controller models** that own their dialog state and expose a small surface to `App` (`Open()`, `HandleKey()`, `Save() tea.Cmd`). Paycheck and loan wizards should be packages under `tui/`, not flat 1.8k files in the root package. This is the same discipline already applied to CLI (`cli/<noun>/`) and partially to `tui/widget` + `tui/dialog`.

---

## 5. Boundary leaks and duplicated contracts

**Severity: boundary / type cleanliness**

**Repositories on `app.Services`.** The composition root exposes almost every repo for “CLI/TUI convenience.” That trains callers to bypass services (CLI list/search/edit paths already touch `InvestmentRepo`, `TransactionRepo`, `LotRepo` directly). Prefer service methods for any rule-bearing read; keep repos unexported from `app` except test helpers.

**Duplicate error types:**

- `transaction.InvalidTransferAmountError` vs `investment.InvalidTransferAmountError`
- `transaction.NotRegularAccountError` vs `investment.NotRegularAccountError`
- Closed-account: `account.AccountClosedError` / `account.IsClosedError` / `scheduled.ClosedAccountError`

Callers cannot `errors.As` across package boundaries reliably. One package (`account` for open/closed, a shared transfer errors package or the unified transfer owner) should own these.

**Guards reimplemented per service:** `ensureAccountOpen` is copy-pasted in transaction and investment; scheduled has its own `ensureNoClosedAccounts`. Same rule, three shapes.

---

## 6. `NewServices` mutates the database as a constructor side effect

**Severity: design smell / observability**

```go
// internal/app/registry.go
_ = categorySvc.EnsurePaycheckCategories()
// ...
valueAdjustmentCollision, _ := categorySvc.EnsureValueAdjustmentCategory()
// ...
_, _ = scheduledSvc.HealNextDates()
// ...
_, _ = investmentSvc.HealAllAccounts()
```

Opening a file for a read-only CLI command can still write. Errors are discarded. Heal semantics are invisible to the user.

**Push:** an explicit `OpenAndMigrate` / `HealOnOpen` step at the CLI/TUI entry points, with logging and optional dry-run — not silent work inside a DI factory. Constructors should construct.

### 6a. Four TUI `tea.Cmd` goroutines call `NewServices` mid-session

Filed with the fix that made the import and link-transfers dialogs visible
(they were built and key-routed but never painted), which is what makes these
paths reachable from the menu. Out of scope for that fix; they belong here.

| Site | Call |
|---|---|
| `internal/tui/import_dialog.go:373` | `runImportPreview` |
| `internal/tui/import_dialog.go:425` | `runImportExecute` |
| `internal/tui/link_transfers_dialog.go:31` | `startLinkTransfers` |
| `internal/tui/link_transfers_dialog.go:45` | `runLinkTransfersExecute` |

Each builds a **second** service registry inside a command goroutine, so each
re-runs the write side effects listed above — `EnsurePaycheckCategories`,
`EnsureValueAdjustmentCategory`, `HealAllAccounts`, `HealNextDates`. **Opening
the import preview silently heals scheduled dates and investment accounts.**
The user asked to look at a file, not to mutate their book.

Two further defects at the same four lines:

- Each reads `a.db` at **goroutine time**, not capture time. `switchDatabase`
  re-points `a.db` and closes the previous handle, so an in-flight import racing
  a file-open reads a moving target.
- Each bypasses `newTUIServices`, so none carries the Yahoo price provider.
  Harmless today — neither import nor transfer-linking fetches prices — but it
  means these registries are not the ones the rest of the TUI uses.

The heal-on-open re-run and the `a.db` read are the real defects. Whoever
centralises `HealOnOpen` should fix all four in the same pass, and the `a.db`
read should become a synchronous capture regardless.

---

## 7. Secondary (still real) maintainability debt

These matter, but rank below the above:

- **Investment edit model** (`update_edit.go`): reverse effects → mutate → reapply, with nested “rollback failed” error wrapping. Correctness is hard to prove without a Tx. Prefer immutable event + rebuild-from-history for lots/positions (rebuild/heal paths already exist — lean into them as the single source of truth).
- **Transfer-line + split complexity** in `ReplaceSplits` / `planSplitReplacement`: clever, but it is a state machine buried in a god service. If it stays, give it a dedicated type and file with invariants documented as tests-as-spec (tests exist; the module boundary is still missing).
- **O(n²) transfer linking** in `transferlink` (`FindUnlinked` nested loops over all eligible txns): fine for small books, will hurt on large imports. Group by amount key first.
- **Presentation re-validates domain rules** (inv→inv category rejection in TUI and CLI). Domain should reject; UI should only map errors.

---

## Preferred remediation order

1. **`db.WithTx` + make multi-row ops atomic** (transfers, splits with counterparts, buy/sell+lot, corporate actions). Delete best-effort rollbacks as they become dead.
2. **Unified cash-transfer API** owned by one service; collapse TransferCash/DepositFromAccount; thin CLI/TUI/undo to one call.
3. **Decompose `transaction_service` / `investment_service`** along those new ownership lines (not cosmetic file splits).
4. **Extract TUI wizards/dialogs** out of `App` field sprawl; start with paycheck + loan + transfer.
5. **Stop healing inside `NewServices`**; centralize closed-account / transfer errors.

---

## What is already in good shape

Worth stating so the review is not only negative:

- Vertical-slice layout and `app.NewServices` composition root are sound.
- Value types (`Money`, `Quantity`, `ID`, `Date`) are a real maintainability win.
- Specs under `specs/` and deep tests (including large service tests) are unusually good for a personal-finance codebase.
- CLI noun/verb split and `cmdutil` acyclic hub are cleaner than the TUI layer.
- `ChooseTransferDispatch` is the right *seed* of a unified model — it just stopped one layer too high.

---

## Bottom line

This project does not need polish. It needs **fewer concepts at the money-movement core**: one atomic unit of work, one transfer owner, services that fit in a human’s head, and presentation that only renders and dispatches.

The dual-table investment design may be justified historically, but right now it is paid for every day in adapters, 4-way switches, and compensate-on-failure paths. Either embrace a real unit-of-work and a transfer façade, or accept that every future money feature will make the spaghetti thicker.

---

## Suggested next step

Highest leverage follow-up: a short design sketch for `WithTx` + unified `Transfer.Create` before any large refactor PR.
