# Design sketch: `db.WithTx` — atomic multi-row money operations

**Date:** 2026-07-24 (v2 — revised after design review; v1 missed the
scheduled-service flows, both merges, void/undo, and cross-service nesting)
**Status:** Draft for review (no code yet)
**Addresses:** `specs/code-quality-review.md` item 1 (no domain SQL transactions — compensate-and-pray)

---

## Goal

One primitive — `db.WithTx(fn)` — so every multi-row domain mutation commits
once or not at all. Compensation code (`rollbackCreateWithSplits`,
`rollbackTransferLinePairs`, `_ = s.repo.Delete(...)` branches, the nested
"rollback failed" wrapping in `update_edit.go`, the scheduled-service
compensation at `scheduled_service.go:134`) becomes dead and is deleted.
Buy — which today leaves an orphan transaction row if the lot/position write
fails — becomes atomic. Scheduled posting — which today can post a
transaction and crash before advancing `next_date` (the "partial success"
comment at `scheduled_service.go:562`) — becomes atomic, killing the
double-post window.

## Non-goals

- No unified Transfer service (review item 2). This design is the
  prerequisite; the façade comes later and will sit *on top of* `WithTx`.
- No context.Context threading. The codebase has no ctx anywhere; adding it
  everywhere is orthogonal churn. `WithTx` takes a closure, not a ctx.
- No repository interface extraction, no service decomposition (item 3).
- No change to read paths' behavior (reads inside a tx go through the tx for
  consistency, but no isolation-level tuning).

---

## 1. The primitive (`internal/db`)

### `Queryer`

`*sql.DB` and `*sql.Tx` already share the three method signatures every repo
uses. The interface just names that:

```go
// internal/db/queryer.go
package db

import "database/sql"

// Queryer is the query/exec surface shared by *sql.DB and *sql.Tx.
// Repositories run all SQL through a Queryer so the same method works
// inside and outside a transaction.
type Queryer interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}
```

### `WithTx`

```go
// internal/db/tx.go

// WithTx runs fn inside a single SQL transaction. If fn returns an error
// or panics, the transaction is rolled back; otherwise it is committed.
// The transaction is passed to fn as a Queryer — hand it to repositories
// and services via their WithTx/InTx derivations.
//
// WithTx serializes: a mutex enforces single-writer discipline, so a
// concurrent call (e.g. from a TUI tea.Cmd goroutine) queues briefly
// instead of racing on a second pooled connection. WithTx must not be
// nested — DuckDB has no savepoints. The join-if-bound pattern (§3)
// makes nesting structurally impossible; a bug that bypasses it
// deadlocks on the mutex, which any test exercising the path catches
// immediately.
func (db *DB) WithTx(fn func(tx Queryer) error) error {
	db.txMu.Lock()
	defer db.txMu.Unlock()

	tx, err := db.conn.Begin() // reads the live conn — safe across reconnect()
	if err != nil {
		return &DatabaseError{Op: "begin transaction", Err: err}
	}

	done := false
	defer func() {
		if !done {
			_ = tx.Rollback() // panic path; re-panic proceeds after rollback
		}
	}()

	if err := fn(tx); err != nil {
		done = true
		if rbErr := tx.Rollback(); rbErr != nil {
			return &DatabaseError{Op: "rollback", Err: errors.Join(err, rbErr)}
		}
		return err
	}

	done = true
	if err := tx.Commit(); err != nil {
		return &DatabaseError{Op: "commit transaction", Err: err}
	}
	return nil
}
```

Notes:

- Method on `*db.DB`, and it dereferences `db.conn` at call time — this
  composes with `reconnect()` (connection.go), which swaps `db.conn` after
  migrations/reindex. Migrations/reindex only run at open time, never while a
  domain tx is in flight, so no race in practice.
- A failed `Rollback` is joined onto the domain error, not swallowed — the
  review's "rollbacks swallow errors" complaint dies here too.
- `Open`/`Create` additionally call `conn.SetMaxOpenConns(1)`: single
  process, single writer, no reason to let the pool interleave connections.
  Belt and suspenders with the mutex.

---

## 2. Repository pattern: `q()` + `WithTx` derivation

Repos keep their exact constructors and fields (`registry.go` untouched).
Each gains one nil-able field, one accessor, and one derivation method:

```go
// internal/transaction/transaction_repository.go
type Repository struct {
	db *db.DB
	tx db.Queryer // nil outside a transaction
}

// q returns the active Queryer: the bound transaction if any, else the
// live connection. All SQL in this repo goes through q().
func (r *Repository) q() db.Queryer {
	if r.tx != nil {
		return r.tx
	}
	return r.db.Conn()
}

// WithTx returns a copy of the repository bound to tx. The original is
// unchanged and remains safe for non-transactional use.
func (r *Repository) WithTx(tx db.Queryer) *Repository {
	c := *r
	c.tx = tx
	return &c
}
```

Then the mechanical sweep: every `r.db.Conn().Exec/Query/QueryRow` becomes
`r.q().Exec/Query/QueryRow` (~180–200 sites across 17 repos; see
`reference_scripted_refactor_gotchas.md` before scripting it). Services with
ad-hoc `s.db.Conn()` queries (e.g. `transaction_service.go:1346`, category
merge's raw UPDATEs) get the same treatment via the service-level binding
(§3).

Key properties:

- **Zero constructor/wiring changes.** Repos still take `*db.DB`;
  `app.NewServices` is untouched. CLI/TUI code that reaches into repos
  directly keeps working (non-tx path is the default).
- **Derived repos are shallow copies** — cheap, no locks, no shared mutable
  state. The tx-bound copy dies with the closure.
- **Composite repos rebind children.** `TransferRepository` holds a
  `*Repository`; its derivation must rebind both:

```go
func (r *TransferRepository) WithTx(tx db.Queryer) *TransferRepository {
	return &TransferRepository{db: r.db, tx: tx, txnRepo: r.txnRepo.WithTx(tx)}
}
```

- **Reads inside a flow use the tx too** (e.g. `GetByTransferID` before a
  pair update) — they come along for free because the whole derived repo is
  bound. This also means a flow sees its own uncommitted writes.

---

## 3. Composition: join-if-bound services

This is the load-bearing rule, and it replaces v1's "entry point vs
participant" convention, which could not handle the real call graph.
Exploration found these cross-flow edges (all verified, file:line in §4):

- scheduled posting → `transaction.Service.CreateTransfer` /
  `CreateWithSplits` / `Create` / `txnRepo.Create`, then `repo.Update(st)` —
  cross-package, both sides must share one tx for posting to be atomic
- `update_edit.go` `UpdateBuy/Sell/FeeLiquidation/ReinvestDividend` →
  `Buy`/`Sell`/`FeeLiquidation`/`ReinvestDividend`
- every trade → `syncPositionAndLots` (a multi-write heal, §5)
- `transferlink.linkOne` → `TransferRepository.Update`
- `CreateWithSplits`/`ReplaceSplits`/`VoidTransaction` → counterpart adapter
  + `txnRepo`

The pattern: **services, like repos, are derivable.** `svc.InTx(tx)` returns
a shallow copy with its repos (and collaborating services/adapters) rebound
to the tx. Every transactional service method runs its write body through a
small helper:

```go
// on each service that owns transactional flows
type Service struct {
	// ...existing fields...
	tx db.Queryer // nil outside a transaction
}

// InTx returns a copy of the service bound to tx: repos rebound,
// collaborating services/adapters rebound recursively.
func (s *Service) InTx(tx db.Queryer) *Service {
	c := *s
	c.tx = tx
	c.txnRepo = s.txnRepo.WithTx(tx)
	c.splitRepo = s.splitRepo.WithTx(tx)
	// ...every repo the write paths touch...
	return &c
}

// runInTx begins a new transaction if the service is unbound, or joins
// the already-bound transaction. This is what makes service methods
// composable without savepoints: an outer flow binds once, inner calls
// join. A bound service must never reach db.WithTx (asserted here).
func (s *Service) runInTx(fn func(b *Service) error) error {
	if s.tx != nil {
		return fn(s) // already bound — join the caller's tx
	}
	return s.db.WithTx(func(tx db.Queryer) error {
		return fn(s.InTx(tx))
	})
}
```

A public method then reads:

```go
func (s *Service) CreateTransfer(args...) (*TransferPair, error) {
	// validation, account lookups — outside the tx
	return s.runInTx(func(b *Service) error {
		return b.transferRepo.Create(pair) // participant body, two inserts
	})
}
```

Called standalone → opens and commits its own tx. Called from scheduled
posting via `s.txnSvc.InTx(tx).CreateTransfer(...)` → joins the posting tx.
Public APIs are unchanged; no `*Tx` method-variant explosion.

**Consequences:**

- Repo methods are always pure participants: they never call `WithTx`. v1's
  sketch of `TransferRepository.Create` opening its own tx is dropped — the
  service owns the tx; the repo body is two inserts on whatever it's bound
  to. `transferlink.linkOne` wraps its `transferRepo.Update` call in
  `db.WithTx` itself (it is an entry point).
- The adapter seam (§4.4) is just this pattern across the import-cycle
  boundary: `InvestmentCashCounterpartAdapter` gains
  `InTx(tx db.Queryer) InvestmentCashCounterpartAdapter`, implemented by
  `investment.Service` as the same shallow-copy derivation.
- Nesting cannot occur by construction: bound services join, only unbound
  ones Begin. The `WithTx` mutex (§1) turns any convention-violating nested
  Begin into an immediate, test-visible deadlock rather than a silent
  second-connection race.

---

## 4. Worked conversions

### 4.1 Transfer create (deletes the compensation)

`transaction.Service.CreateTransfer` wraps via `runInTx`;
`TransferRepository.Create` becomes a pure two-insert participant:

```go
// transfer_repository.go — participant: no WithTx here
func (r *TransferRepository) Create(pair *TransferPair) error {
	if errs := pair.Validate(); errs.HasErrors() {
		return fmt.Errorf("invalid transfer pair: %v", errs)
	}
	if err := r.txnRepo.Create(pair.FromTransaction); err != nil {
		return fmt.Errorf("failed to create from transaction: %w", err)
	}
	if err := r.txnRepo.Create(pair.ToTransaction); err != nil {
		return fmt.Errorf("failed to create to transaction: %w", err)
	}
	return nil
}
```

The manual unwind (`_ = r.txnRepo.Delete(pair.FromTransaction.ID)`) and its
comment are deleted. Same shape for `Update`, `UpdateStatus` (two narrow
updates), and `Delete` (two deletes) — service-level `runInTx`, repo-level
participant bodies.

### 4.2 Investment Buy (adds the atomicity it never had)

```go
// investment_service.go
func (s *Service) Buy(args...) (*Transaction, error) {
	// validation + account lookup outside the tx
	// syncPositionAndLots: own committed tx BEFORE the trade tx when
	// unbound; skipped when bound (see §5)
	err := s.runInTx(func(b *Service) error {
		if err := b.repo.Create(txn); err != nil {
			return fmt.Errorf("failed to create buy transaction: %w", err)
		}
		if acct.TrackLots {
			lot := NewLot(...)
			if err := b.lotRepo.Create(&lot); err != nil {
				return fmt.Errorf("failed to create lot: %w", err) // txn row rolls back too
			}
		} else if err := b.positionRepo.CreateOrUpdate(...); err != nil {
			return err
		}
		return b.autoPrice(...) // priceRepo joins the same tx
	})
	...
}
```

A buy either fully lands (row + lot/position + price) or not at all.
Validation and account lookups stay *outside* the tx; only writes (and reads
the writes depend on) go inside — keep transactions short.

### 4.3 Cash transfers (investment ↔ transaction, same pattern)

`TransferCash` / `DepositFromAccount` already hold both repos; `runInTx`
binds both, the `_ = s.repo.Delete(invTxn.ID)` compensation branch is
deleted.

### 4.4 The adapter seam (`CreateWithSplits` / `ReplaceSplits` / `VoidTransaction`)

`transaction.Service` mints investment rows through
`InvestmentCashCounterpartAdapter` (cycle-breaker, wired in registry.go).
The adapter gains one method so counterpart writes join the caller's tx:

```go
// transaction_service.go
type InvestmentCashCounterpartAdapter interface {
	// ...existing four methods unchanged...

	// InTx returns a copy of the adapter whose writes run on tx.
	// db.Queryer is the only shared vocabulary between the packages,
	// which is exactly why Queryer lives in internal/db.
	InTx(tx db.Queryer) InvestmentCashCounterpartAdapter
}
```

`investment.Service.InTx` (§3) satisfies it directly. The test fake
`fakeInvCounterpart` (`split_investment_test.go:17`) gains a trivial
`InTx` returning itself.

`CreateWithSplits` becomes: preflight/validate outside; one `runInTx`
wrapping parent create + N split creates + N counterpart mints via the
bound adapter. `rollbackCreateWithSplits` and `rollbackTransferLinePairs`
are deleted. `ReplaceSplits` keeps its planning phase
(`planSplitReplacement`) outside the tx and executes the whole
delete-counterparts → delete-splits → recreate → mint plan inside one tx.
`VoidTransaction` (`transaction_service.go:~1590`: `deleteTransferLinePairs`
+ `splitRepo.DeleteByTransaction` + `txnRepo.Update`) and
`RestoreVoidedTransaction` get the same wrap.

### 4.5 Scheduled service (missed by v1 — the worst offender)

`internal/scheduled/scheduled_service.go`:

- **`Create`** (repo.Create + N split creates, compensation at :134) and
  **`Update`** (delete-splits → update → recreate, :171–179): plain
  `runInTx` wraps; compensation deleted.
- **Posting flows** — `postSingleLine` (:514), `postSingleLineTransfer`
  (:577), `postMultiLine` (:619), `PostWithEdits` (:727), `AutoPost`
  (:243) — each posts via the transaction service *and* advances the
  schedule. This is the flagship join-if-bound case:

```go
return s.db.WithTx(func(tx db.Queryer) error {
	if _, err := s.txnSvc.InTx(tx).CreateWithSplits(...); err != nil {
		return err
	}
	return s.repo.WithTx(tx).Update(st) // advance next_date atomically
})
```

  The "partial success" double-post window (:562–564) is closed.
  `AutoPost` scopes one tx per candidate (one bad schedule doesn't roll
  back the others' posts).

### 4.6 Category merge and payee merge (missed by v1)

- **`category.Service.MergeCategories`** (:522): 4 raw UPDATEs + child
  reassignment loop + delete, today "best-effort" behind a comment wrongly
  claiming DuckDB doesn't support transactions. One `runInTx`; delete the
  false comment.
- **`payee.Service.MergePayees`** (:257): multi-write, built on
  `CREATE TEMPORARY TABLE` / `DROP`. **Spike result (phase 7):** temp-table
  DDL + DML + DROP all work inside an explicit tx on the pinned duckdb-go
  version, and rollback after temp-table DDL is clean — so the temp-table
  pattern is wrapped as-is, no rewrite.
- **Implementation finding (phase 7):** DuckDB refuses to DELETE an
  FK-referenced key in the same transaction that earlier moved the
  references off it (`Violates foreign key constraint ... still
  referenced`), for every update form tried (bulk UPDATE, DELETE+INSERT,
  temp-table reinsert). So both merges wrap the **full reassignment
  write-set** in one tx (the real integrity hazard) and run the source
  DELETE as a separate trailing autocommit statement. If that trailing
  delete fails, the source survives as a zero-reference orphan and a re-run
  of the merge is idempotent. Full single-tx merge would require the
  drop-child-FKs schema path — out of scope.

### 4.7 Undo commands (missed by v1)

Most undo commands delegate to a single service call and become atomic for
free. Three do their own multi-writes (`internal/undo/transaction.go`):

- `DeleteTransferCommand.Undo` (:294) recreates a pair with two `svc.Create`
  calls + compensation — change it to call the transfer-create service
  method (one atomic call), delete the compensation.
- `VoidTransactionCommand.Undo` (:191) and the edit-with-splits
  Execute/Undo (:506–522) each chain two service calls
  (`RestoreVoidedTransaction`+`ReplaceSplits`, `Update`+`ReplaceSplits`).
  Give `transaction.Service` small composed methods (one `runInTx` wrapping
  both bound calls) and point the undo commands at those.

### 4.8 Edit reverse/reapply (`update_edit.go`) and corporate actions

`UpdateBuy/UpdateSell/UpdateFeeLiquidation/UpdateReinvestDividend` each call
`loadAndReverseForEdit` (multi-write) then the trade itself (:335, :366,
:401, :428), with reapply-on-failure compensation. Conversion: the entry
point runs the heal (§5) in its own tx, then one `runInTx` wraps
reverse → mutate → reapply; the inner `Buy`/`Sell` call joins via the bound
service, so "rollback failed, DB now inconsistent" error paths become
impossible states and their wrapping code is deleted.

Corporate actions (`Split`/`Merger`/`SpinOff`, plus `DeleteAction` /
`reverseSpinOff` reversals) write directly via repos and never call
Buy/Sell (verified) — each action is a self-contained top-level tx. These
are the largest write sets — convert last, after the pattern is proven.

---

## 5. Heals and rebuilds

Decisions:

- **`syncPositionAndLots`** (called at the top of every trade —
  `investment_service.go:195, 283, 699, 773` — and by cash transfers):
  runs in its **own committed tx before** the trade tx. Heal is idempotent
  repair — committing it is desirable even when the trade then fails, and
  it keeps trade txs small. When the service is already tx-bound (trade
  called from `update_edit`), the bound copy **skips the re-heal**: the
  outermost entry point healed (own tx) before opening the main tx, and
  the in-tx state is being mutated deliberately.
- **`RebuildPositions` / `HealAllAccounts`** (`rebuild.go:30, 164`):
  **one tx per account.** Each account's rebuild is all-or-nothing;
  `HealAllAccounts` loops committed per-account txs so one bad account
  doesn't roll back the others' repairs. Bounded tx size.
- `scheduled.HealNextDates` → repo method: wrap only if it is a
  multi-statement loop (check `scheduled_repository.go:435` during
  conversion).

---

## 6. DuckDB constraints this design respects

| Constraint (already documented in-repo) | Consequence for WithTx |
|---|---|
| `CREATE INDEX` aborts inside an explicit tx (`internal/db/reindex.go`) | Money ops are DML only. Reindex/migrations stay outside `WithTx`. **Payee merge's temp-table DDL must be rewritten (or verified) before wrapping — §4.6.** |
| UPDATE on indexed/FK columns = internal DELETE+INSERT; can abort on ART index desync (reindex.go, migration 026 notes) | Unchanged risk, better outcome: mid-tx abort now rolls back cleanly instead of leaving half a flow. Keep the narrow-UPDATE discipline (`UpdateStatus`-style) inside transactions. |
| `reconnect()` swaps `db.conn` after migrations (connection.go) | `WithTx` reads `db.conn` at call time; reconnect only happens at open, never mid-flow. |
| No savepoints | No nested transactions. Join-if-bound (§3) makes nesting structurally impossible; the `WithTx` mutex turns violations into immediate deadlocks caught by tests. |
| database/sql pool may hold >1 conn; a tx pins one | `SetMaxOpenConns(1)` at open + the `WithTx` mutex: single-writer discipline, concurrent TUI-goroutine calls queue briefly instead of racing. |

---

## 7. Rollout plan

Each phase compiles, passes the full suite, and is a separate commit to main.

1. **Primitive.** `db.Queryer`, `DB.WithTx` (mutex), `SetMaxOpenConns(1)`,
   unit tests: commit persists, error rolls back, panic rolls back and
   re-panics, rollback-failure joins errors, concurrent calls serialize.
2. **Mechanical sweep, no behavior change.** Add `tx` field / `q()` /
   `WithTx` to all 17 repos; rewrite `r.db.Conn().X` → `r.q().X`. Pure
   refactor — nothing calls repo `WithTx` yet. (Scriptable; mind the
   zsh/BSD-sed gotchas in the refactor-gotchas memory.)
3. **Transfers.** `runInTx`/`InTx` on `transaction.Service`;
   `TransferRepository` methods become participants; delete the unwind in
   `Create`. `transferlink.linkOne` wraps its Update call.
4. **Investment trades.** `InTx` on `investment.Service`;
   Buy/Sell/ReinvestDividend/FeeLiquidation (+ heal-before-trade per §5);
   then the three cash-transfer variants; delete compensation branches.
5. **Splits + adapter + void.** Adapter `InTx` (+ fake); `CreateWithSplits`
   / `ReplaceSplits` / `VoidTransaction` / `RestoreVoidedTransaction`
   atomic; delete `rollbackCreateWithSplits` / `rollbackTransferLinePairs`.
6. **Scheduled service.** Create/Update wrapped; all five posting flows
   join-if-bound with the transaction service; per-candidate tx in
   `AutoPost`; delete compensation and the "partial success" caveat.
7. **Merges + undo.** Category merge wrapped (delete false comment); payee
   merge temp-table rewrite then wrap; the three multi-write undo commands
   repointed at composed service methods.
8. **Edit + corporate actions + rebuild scoping.** `update_edit.go`
   reverse/mutate/reapply in one tx (delete nested rollback wrapping);
   corporate-action flows incl. reversals; per-account tx in
   `RebuildPositions`/`HealAllAccounts`.

Each of phases 3–8 ends with `grep -rn "rollback\|unwind\|compensat\|partial" internal/`
to confirm the dead helpers are actually removed, not stranded.

## 8. Testing strategy

- **Primitive tests** (phase 1) against a scratch `.tdb` via `internal/dbtest`
  template copies. All existing service tests already run real repos on a
  real template-copied DuckDB file — no repo-mock layer exists to fight.
- **Fault injection per converted flow:** a test `Queryer` wrapper
  delegating to a real tx but erroring on the Nth `Exec`. Wiring:
  `database.WithTx(tx => svc.InTx(failWrap(tx)).Buy(...))` (or
  `repo.WithTx(failWrap(tx))` for repo bodies). Assert the flow errors
  *and* a fresh read shows zero partial state. Because `WithTx` itself is
  proven in phase 1, what this actually verifies per flow is that **no
  write escaped the transaction** (nothing still goes through
  `r.db.Conn()` directly) — the regression that would silently reintroduce
  partial state.
- **Existing suite as regression net** — the deep service tests already cover
  the happy paths; they must pass unmodified at every phase.
- **DuckDB-quirk regressions:** (a) an indexed-column UPDATE inside `WithTx`
  whose abort rolls back the whole flow; (b) before phase 7, a spike test
  for `CREATE TEMPORARY TABLE` inside an explicit tx on the pinned
  duckdb-go version (decides whether payee merge needs the UPDATE rewrite
  or just the wrap).
- **Double-post regression** (phase 6): force a failure in the
  schedule-advance write and assert the posted transaction rolled back too.

## 9. What this deliberately leaves alone

- The four transfer entry points and their 4-way presentation dispatch
  (review item 2) — each just becomes internally atomic. The future unified
  Transfer service will be a client of `WithTx` and `InTx`, not a rewrite.
- Repos exposed on `app.Services` (item 5) — direct repo reads keep working
  because the non-tx default path is unchanged.
- `NewServices` heal-on-open side effects (item 6) — the heals gain
  per-account atomicity in phase 8, but moving them out of the constructor
  is a separate change.
- `account.Delete` — verified single-mutation (two guard reads + one
  DELETE, `account_repository.go:317`); nothing to wrap.
