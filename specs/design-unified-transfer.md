# Design sketch: `internal/transfer` — one ledger-agnostic transfer owner

**Date:** 2026-08-02
**Status:** IN PROGRESS — phase 1 shipped. v3 folds in what implementing it
taught: `Get` must decide Shape BEFORE asserting leg arity, because a split
transfer-line is one ledger row plus a `transaction_splits` row, not two ledger
rows (§2.6); `transaction/dispatch.go` cannot be moved in phase 1 without leaving
the tree red, so it dies in phase 3 with its last caller; `Transfer` carries the
loaded `FromAccount`/`ToAccount` since reads must load them anyway and phase 2's
guards need them. Phase 1's actual test churn was much smaller than predicted —
see its Test churn note.

**Status history:** PROPOSED (v2 — revised after an independent adversarial review that
checked every load-bearing claim against the tree). Three defects were found and
fixed here: (1) v1 homed the exported status mapping in
`internal/investment/status.go`, but those functions are typed on
`transaction.Status`, so that choice silently preserved the very import edge
phase 5 exists to sever — the mapping now lives in `internal/transfer` (§5.2,
phase 1); (2) v1 never re-pointed the register's cleared toggle, which reaches
`transaction.Service.Update` for transfer legs too, so phase 5's `IsTransferError`
refusal would have broken Space-to-clear with no test catching it (phase 3);
(3) v1 deleted the undo transfer commands in phase 3 but their tests in phase 5,
leaving phase 3 red (phase 3). Also corrected: the `created_at` tie-break for
voided pairs is not deterministic and no longer claims to be (§2.3); the
`UpdateTransferShares` type-check cited in §4 does not exist (it checks
`TransferID.Valid` only); "byte-identical rows" in phase 2 is now stated modulo
freshly-minted ids and timestamps; call-site and query-site counts re-derived
(22, not 26; 34 across 7 packages, not 35 across 8).
**Addresses:** `specs/code-quality-review.md` item 2 (dual ledgers force a
4-path transfer graph that leaks everywhere)
**Companion to:** `specs/design-withtx.md` (item 1, IMPLEMENTED). That design
said, in its own non-goals: *"No unified Transfer service (review item 2). This
design is the prerequisite; the façade comes later and will sit on top of
`WithTx`."* This is that later. Everything here is built on `db.WithTx`,
`Service.InTx(db.Queryer)` and the join-if-bound `runInTx` contract; none of it
would be safe without them.

---

## Goal

One owner — `transfer.Service` — for every whole-transaction cash transfer,
whatever pair of account types it connects. It classifies once, guards once,
and writes both legs directly through `transaction.Repository` and
`investment.Repository` inside a single `db.WithTx`. The four create entry
points, the two edit entry points and the two delete entry points collapse into
one implementation whose write path contains **no dispatch switch at all** — the
sign comes from which side of the transfer a leg is on, the table comes from the
account's type, and 2 signs × 2 ledgers reproduces the four shapes as an
emergent property rather than four hand-written branches.

Presentation becomes:

```go
res, err := svc.Transfer.Create(transfer.Spec{
    FromAccountID: from.ID, ToAccountID: to.ID,
    Date: date, Amount: amount, Memo: memo, CategoryID: categoryID,
})
```

The five hand-written 4-arm switches in the TUI and CLI, the two hand-rolled
cross-table leg resolvers, the three copies of the investment↔regular status
mapping, and the five presentation-layer copies of the inv↔inv category refusal
all die.

## Non-goals

- **No single-ledger merge.** `transactions` and `investment_transactions` stay
  separate tables. The verdict and the evidence behind it are in §12.1 — this is
  the single most important scope decision in the document and it is argued, not
  assumed.
- **No share-transfer ownership.** `TransferShares` / `UpdateTransferShares` stay
  in `investment.Service` (§4). They are classified by the new vocabulary so the
  cash verbs refuse them, but they are not moved.
- **No split-lifecycle move.** Transfer *lines* inside multi-line splits stay in
  `transaction.Service` (§3). The counterpart port survives — reduced from five
  methods to four, with the naming hack and the post-construction setter deleted.
- **No god-file decomposition** (review item 3) beyond what falls out of the
  deletions. `transaction_service.go` loses ~460 lines and `investment_service.go`
  ~336; both remain over 1,400.
- **No `context.Context` threading.** Same reasoning as `design-withtx.md` §
  Non-goals.

---

## 1. The problem, measured

### 1.1 Four services for one concept

| Path | Owner | Lines |
|---|---|---|
| bank→bank | `transaction.Service.CreateTransfer` (`transaction_service.go:1244`) | 47 |
| inv→bank | `investment.Service.TransferCash` (`investment_service.go:1078`) | 89 |
| bank→inv | `investment.Service.DepositFromAccount` (`investment_service.go:1170`) | 87 |
| inv→inv | `investment.Service.TransferCashBetweenInvestments` (`investment_service.go:1271`) | 65 |

`TransferCash` and `DepositFromAccount` are not merely similar. Extracting their
two bodies (`sed -n '1082,1166p'` vs `sed -n '1174,1256p'`) and diffing yields
**19 diff lines over 85** — six changed lines and a moved comment. Everything
else — the `txnRepo == nil` check, `amount.IsPositive`, `requireInvestmentAccount`,
the regular-account load, the `NotRegularAccountError` block, `IsClosed`, the
same-account check, `ValidateTransactionDate`, the `transferID := types.NewID()`
mint, the "category labels the regular-side leg only" comment block, the
`runInTx` body and the returned `CashTransferResult` literal — is byte-identical.
The only real difference is which leg gets `amount.Neg()`.

### 1.2 The classifier is not an owner

`ChooseTransferDispatch` (`dispatch.go:33`) makes the right decision and then
hands it to five separate hand-written switch bodies:

| Site | Lines |
|---|---|
| `tui/transfer_dialog.go:713-795` (create) | 88 |
| `tui/transfer_dialog.go:954-999` (`dispatchInvestmentEditTransfer`) | 46 |
| `cli/transfer/add.go:150-209` (`dispatchTransferAdd`) | 60 |
| `cli/transfer/edit.go:199-231` (`dispatchTransferEdit`) | 33 |
| `cli/transfer/delete.go:90-95` (`dispatchTransferDelete`) | 6 |

Each arm re-maps the caller's `(from, to)` ordering into a different service's
argument order. `DepositFromAccount` takes `(investmentAccountID,
regularAccountID)` — the reverse of the user's `--from/--to` — so
`add.go:164-176` flips the arguments and then flips the result fields back. No
test asserts that correspondence directly.

### 1.3 Reading a transfer is re-implemented per front end

There is no domain type for "a transfer, whichever tables its legs are in".
`transaction.TransferRepository.GetByTransferID` (`transfer_repository.go:66`)
hard-asserts **exactly two rows on `transactions`** at `:115-117`, so it is
structurally incapable of seeing an inv↔reg pair, an inv↔inv pair, or a split
transfer-line. That single assertion is why `tmoney transaction void <regular leg
of an inv↔reg transfer>` fails today with `expected 2 transactions for transfer,
found 1`.

So each front end built its own resolver:

| Site | Lines |
|---|---|
| `cli/transfer/resolve.go` (`resolvedTransfer`, `resolveTransferPair`, `resolveFromRegularLeg`, `resolveFromInvestmentLeg`, `findRegularLeg`, `findInvestmentLeg`, `refuseIfMultiLineSplit`, `investmentStatusToRegular`) | 268 |
| `tui/transfer_dialog.go:298-391` (`loadEditTransferDialogData`) | 94 |
| `tui/transfer_dialog.go:402-482` (`loadEditInvestmentTransferDialogData`) | 81 |

`internal/tui/transfer_dialog.go` is 999 lines; roughly 43% of it is
cross-table plumbing.

### 1.4 Rules live in presentation because the domain has no place for them

- **"inv↔inv cannot carry a category"** is enforced at `cli/transfer/add.go:112`,
  `add.go:194`, `cli/transfer/edit.go:107`, `tui/transfer_dialog.go:718` and
  `tui/transfer_dialog.go:212-228`. There is **no domain guard at all** —
  `TransferCashBetweenInvestments` simply has no `categoryID` parameter, and
  `UpdateTransferCash` silently drops one on the inv↔inv branch
  (`update_edit.go:608-633`).
- **"a reconciled transfer cannot be edited or deleted"** is enforced in the
  domain only for bank↔bank (`checkTransferEditable`, `:1525`). Grep for
  `IsReconciled` in `investment_service.go` returns exactly one hit — `:1389`,
  inside `FindTransferCashCounterpart`. `UpdateTransferCash` and
  `DeleteTransaction` have no reconciled guard, so the rule exists only at
  `cli/transfer/edit.go:99` and `cli/transfer/delete.go:68`. **The TUI has
  neither**, so it can silently edit and delete reconciled investment transfers.
- **"a transfer's category must exist and be non-system"** is enforced only for
  bank↔bank. `internal/investment` contains zero category validation and its
  comments at `investment_service.go:1135-1138` explicitly delegate it to the
  caller.
- **"a transfer_id owned by a multi-line split is not a whole transfer"**
  (plan D10) exists only at `cli/transfer/resolve.go:237`. The TUI has no
  equivalent: its bank-side path errors with "expected 2 transactions", and its
  investment-side path sails into `UpdateTransferCash`, which deletes both legs
  and re-creates them under a **brand-new transfer_id** — permanently orphaning
  the split line *and* minting a second regular-side leg in the bank account.

### 1.5 Duplicated concepts, counted

| Concept | Copies | Where |
|---|---|---|
| 4-way create dispatch | 2 (+2 edit, +1 delete) | TUI, CLI |
| Cross-table leg resolution | 3 | CLI ×1, TUI ×2 |
| investment↔regular status mapping | 3 | `update_edit.go:19`, `tui:487`, `cli/resolve.go:253` |
| "negative leg is From" orientation | 5 | `cli/resolve.go:104`, `:166`, `tui:353`, `:434`, `transfer_repository.go:73` |
| Reconciled-transfer refusal | 6 | `checkTransferEditable`, `UpdateTransfer` inline, `voidTransfer` inline, `Delete` inline, CLI ×2 |
| inv↔inv category refusal | 5 | CLI ×3, TUI ×2 |
| Dual-table counterpart lookup ladder | 5 | `transaction_service.go:394`, `:792`, `:814`, `:852`, `:1183` |
| `InvalidTransferAmountError` type | 2 | `transaction_errors.go:48`, `investment_service.go:1937` |
| `NotRegularAccountError` type | 2 | `transaction_errors.go:125`, `investment_service.go:1946` |
| Result struct for a two-leg write | 3 | `TransferPair`, `CashTransferResult`, `InvestmentCashTransferResult` |
| Transfer view-model | 2 | `cli.resolvedTransfer`, `tui.investmentTransferEdit` |
| Undo command per create path | 4 | `undo/transaction.go:207`, `undo/investment_transfer.go` ×3 |

---

## 2. The design: `internal/transfer`

### 2.1 Where it sits

`internal/transfer` imports both ledger packages and is imported by nothing
inside them. Four packages — `internal/app`, `internal/cli/transfer`,
`internal/tui`, `internal/undo` — already import both `transaction` and
`investment` and compile today, so this placement is proven, not speculative.

```
transfer   → transaction, investment, account, category, db, dberrors, types
transaction → account, category, payee, db, dberrors, types
investment  → account, price, security, db, dberrors, types
```

After §5, `investment` no longer imports `transaction` at all and the two ledger
packages are siblings. `internal/cli/transfer` is also `package transfer`, so it
imports the domain package as `xfer` (the precedent is `transactiondom` in
`internal/cli/transaction/edit.go`).

### 2.2 Vocabulary: three independent axes

`Kind` alone is insufficient, and that insufficiency is a live defect: a
`transfer_shares` pair is inv↔inv by account type but must never be touched by a
cash verb. Today nothing enforces that — `UpdateTransferCash` never checks
`old.Type`, so handing it a buy's ID deletes the buy at `update_edit.go:604` via
`repo.Delete` **without** calling `reverseTxnEffects`, silently corrupting lots.

```go
// internal/transfer/kind.go
package transfer

// Ledger names the physical table a leg's row lives in.
type Ledger uint8

const (
	LedgerRegular    Ledger = iota // transactions
	LedgerInvestment               // investment_transactions
)

// LedgerFor maps an account type to its ledger. HSA counts as investment
// (account.Type.IsInvestmentType covers TypeInvestment and TypeHSA).
func LedgerFor(t account.Type) Ledger {
	if t.IsInvestmentType() {
		return LedgerInvestment
	}
	return LedgerRegular
}

// Kind is the account-type axis. It replaces
// transaction.TransferDispatchKind, and internal/transaction/dispatch.go is
// deleted: after this change the transaction package has no business
// classifying transfers, and every existing caller of ChooseTransferDispatch
// (tui/transfer_dialog.go:713, cli/transfer/add.go:151, edit.go:200,
// delete.go:91, resolve.go:120, :181) becomes a transfer caller anyway.
// dispatch_test.go moves here verbatim, including the case at :113 pinning
// that an unknown/zero account.Type falls through to KindRegToReg.
type Kind uint8

const (
	KindRegToReg Kind = iota
	KindInvToReg
	KindRegToInv
	KindInvToInv
)

func KindOf(from, to account.Type) Kind
func (k Kind) String() string

// SupportsCategory reports whether a transfer of this kind has a leg that can
// store a category. False only for KindInvToInv, because
// investment_transactions has no category_id column (verified: the last full
// recreate is migrations/019_account_type_hsa.sql:178-200, 15 columns, and
// `grep -rn category_id internal/investment/*.go` returns nothing).
// PHASE 7 DELETES THIS PREDICATE: migration 032 adds the column and the rule
// becomes universal. See §9 phase 7 and §12.2.
func (k Kind) SupportsCategory() bool { return k != KindInvToInv }

// SupportsVoid reports whether a transfer of this kind can be voided. True
// only for KindRegToReg: investment_transactions.status has no 'void' value
// (migrations/019:193 — the enum is pending/cleared/reconciled).
func (k Kind) SupportsVoid() bool { return k == KindRegToReg }

// Movement is the second axis, orthogonal to Kind: what actually moves.
// A transfer_shares pair moves cost basis and lots, not cash, and must never
// reach a cash verb.
type Movement uint8

const (
	MovementCash Movement = iota
	MovementShares
)

// Shape is the third axis: whether the transfer_id belongs to a whole
// transaction or to a transfer LINE inside a multi-line split (a paycheck's
// 401k contribution). Promoting plan decision D10 from a CLI-local check
// (cli/transfer/resolve.go:237) into the read model is what finally gives the
// TUI the guard it lacks.
type Shape uint8

const (
	ShapeWholeTransaction Shape = iota
	ShapeSplitLine
)
```

### 2.3 The aggregate

```go
// internal/transfer/transfer.go

// Leg is one side of a transfer, normalized across both ledgers.
//
// Amount is SIGNED as stored (negative = money leaving this account).
// Status is normalized to transaction.Status: an investment leg's "pending"
// reads back as StatusUncleared via transfer.StatusToRegular (§5.2 — the
// mapping lives here, not in internal/investment, because it names both
// ledgers' status types and internal/investment must not import transaction).
type Leg struct {
	Ledger            Ledger
	RowID             types.ID
	AccountID         types.ID
	AccountName       string
	AccountType       account.Type
	Date              types.Date
	Amount            types.Money
	Memo              types.NullableString
	Status            transaction.Status
	CategoryID        types.NullableID // always unset on an investment leg (pre-phase-7)
	TransferID        types.ID
	TransferAccountID types.ID

	// InvestmentType is "" on a regular leg; otherwise the raw
	// investment_transactions.transaction_type. It is what determines Movement.
	InvestmentType string
}

func (l Leg) IsOutflow() bool     { return l.Amount.IsNegative() }
func (l Leg) IsReconciled() bool  { return l.Status == transaction.StatusReconciled }
func (l Leg) IsVoid() bool        { return l.Status == transaction.StatusVoid }

// Transfer is the ledger-agnostic aggregate every caller reads and edits. It
// replaces transaction.TransferPair, investment.CashTransferResult,
// investment.InvestmentCashTransferResult, cli/transfer.resolvedTransfer and
// tui.investmentTransferEdit.
type Transfer struct {
	ID       types.ID // the shared transfer_id
	Kind     Kind
	Movement Movement
	Shape    Shape

	From Leg // the negative leg
	To   Leg // the positive leg

	Date       types.Date
	Amount     types.Money // positive magnitude
	Memo       string
	CategoryID types.NullableID

	// ParentTransactionID / SplitID are set only when Shape == ShapeSplitLine.
	ParentTransactionID types.ID
	SplitID             types.ID
}

// Status collapses the two legs deterministically. It does NOT depend on which
// leg the caller named — cli/transfer/resolve.go reads it from whichever leg
// was addressed (:127 for the regular leg, :188 for the investment leg), which
// is why `transfer edit --amount` can today silently rewrite the unnamed leg's
// status. Precedence: Reconciled, then Void, then Cleared-only-if-both, else
// Uncleared.
func (t *Transfer) Status() transaction.Status

// StatusDiverged reports whether the two legs disagree. This is a normal,
// reachable state: reconciliation reconciles one leg at a time
// (reconciliation_service.go:283) and the register's cleared toggle writes one
// leg (register_view.go:252-262 → undo.EditTransactionCommand →
// transaction.Service.Update, which does no mirroring).
func (t *Transfer) StatusDiverged() bool

func (t *Transfer) IsSplitLine() bool
func (t *Transfer) LegByRowID(rowID types.ID) (Leg, bool)
func (t *Transfer) LegForAccount(acctID types.ID) (Leg, bool)

// InvestmentLegID returns the investment-table leg, if any. For KindInvToInv it
// returns the From (source) leg. Replaces resolvedTransfer.investmentTxnID,
// tui.transferLegForAccount, tui.cashTransferLegForAccount and the third inline
// leg matcher at tui/transfer_dialog.go:787-794.
func (t *Transfer) InvestmentLegID() (types.ID, bool)
```

**Orientation is derived from the leg PAIR, never from one leg's sign, and it is
never used to address a leg for a write.** Four call sites today each infer
"the negative leg is From" from whichever single leg they loaded. After
`voidTransfer` (`transaction_service.go:1732`, `:1736`) both amounts are
`types.ZeroMoney`, so `GetByTransferID`'s `ORDER BY amount ASC`
(`transfer_repository.go:73`, assignment at `:119-125`) has an unspecified
tie-break, and `RestoreVoidedTransfer` (`:1803-1809`) applies `fromAmount` to
whichever row happened to sort first. A void→undo round trip can silently
reverse a transfer's direction, and `pair.Validate()` still passes.

Proposal 3 proposed to fix this by "falling back to `TransferAccountID`
cross-references when both amounts are zero." **That cannot work.**
`TransferPair.Validate` (`transaction.go:548-553`) asserts
`From.TransferAccountID.ID == To.AccountID` **and**
`To.TransferAccountID.ID == From.AccountID` — a symmetric mutual pointer.
Swap the two legs and both conditions still hold. Cross-references cannot
orient *any* pair, zeroed or not.

The actual fix is to stop addressing legs by orientation on the write side.
Every snapshot in this design is **RowID-addressed** (§2.8, §2.9), so orientation
is a presentation concern only.

For *display* of a zero-amount (voided) pair there is no reliable tie-break, and
the design does not pretend otherwise. `created_at` is stamped Go-side by
`types.NewBaseModel()` (`types.go:418`), the two legs are minted nanoseconds
apart, and DuckDB `TIMESTAMP` truncates to microseconds — so equal stored values
are likely, and insertion order does not order equal keys. IDs are UUIDv7
(millisecond precision plus random bits) and do not save it either. The
consequence is bounded: a voided pair may render From/To swapped. The write side
is unaffected because it never reads orientation.

### 2.4 The service

```go
// internal/transfer/service.go

type Service struct {
	txnRepo     *transaction.Repository
	invRepo     *investment.Repository
	splitRepo   *transaction.SplitRepository // read-only: Shape detection
	accountRepo *account.Repository
	categoryRepo *category.Repository
	db          *db.DB
	tx          db.Queryer // nil outside a transaction
}

func NewService(
	txnRepo *transaction.Repository,
	invRepo *investment.Repository,
	splitRepo *transaction.SplitRepository,
	accountRepo *account.Repository,
	categoryRepo *category.Repository,
	database *db.DB,
) *Service

// InTx / q / runInTx follow design-withtx.md §3 exactly. A bound service joins
// the caller's transaction and must never reach db.WithTx — DuckDB has no
// savepoints and db.WithTx holds a mutex (internal/db/tx.go:89-91), so nesting
// deadlocks.
func (s *Service) InTx(tx db.Queryer) *Service
func (s *Service) q() db.Queryer
func (s *Service) runInTx(fn func(b *Service) error) error
```

Having `InTx` is not optional. `internal/scheduled` composes transfer creation
into the same transaction that advances `next_date` — `scheduled_service.go:355`
(AutoPost) and `:679` (`postSingleLineTransfer`), both inside the schedule's own
`runInTx`, with the comment at `:676` explaining that this is what closes the
double-post window. Without `InTx`, `internal/scheduled` would have to keep
calling `transaction.Service.CreateTransfer` directly and the "one door" claim
would be false by construction.

**All guards run inside `runInTx`.** The reconciled/void checks this design newly
adds for investment-involving transfers have no backstop underneath them
(`UpdateTransferCash` has none), so a read-then-write outside the transaction
would be a genuine TOCTOU on a rule nothing else enforces.

### 2.5 The write path — where the four-way graph dies

```go
// internal/transfer/rows.go

// legPlan is a table-agnostic description of one row to write.
type legPlan struct {
	accountID  types.ID
	other      types.ID    // the counterpart account (transfer_account_id)
	ledger     Ledger
	amount     types.Money // already signed
	memo       string
	categoryID types.NullableID // dropped when ledger == LedgerInvestment (pre-phase-7)
	status     transaction.Status
	date       types.Date
}

// planLegs is where the four-way graph used to be. There is no switch:
// the sign comes from which side you are on, the table comes from the account
// type. 2 signs × 2 ledgers = the four shapes, emergent.
func planLegs(from, to *account.Account, spec Spec, transferID types.ID) [2]legPlan {
	return [2]legPlan{
		{accountID: from.ID, other: to.ID, ledger: LedgerFor(from.Type),
			amount: spec.Amount.Neg(), memo: spec.Memo,
			categoryID: spec.CategoryID, status: spec.Status, date: spec.Date},
		{accountID: to.ID, other: from.ID, ledger: LedgerFor(to.Type),
			amount: spec.Amount.Abs(), memo: spec.Memo,
			categoryID: spec.CategoryID, status: spec.Status, date: spec.Date},
	}
}

// insertLeg / updateLeg / deleteLeg are the ONLY places a whole-transaction
// transfer row is written. Each is a two-arm switch on p.ledger, and each runs
// the ledger's own row validation before persisting — investment rows through
// investment.Transaction.Validate (investment.go:454-497), regular rows through
// transaction.Transaction.Validate (transaction.go:~300).
//
// The investment arm ALWAYS sets investment.TransactionTypeTransferCash. This
// is load-bearing and easy to get silently wrong: GetCashBalance
// (investment_service.go:519-533) sums TotalAmount only over rows whose
// Type.AffectsCash(), so a leg written with the wrong type stops counting as
// money with no error anywhere. The test matrix asserts it explicitly (§10).
func (s *Service) insertLeg(transferID types.ID, p legPlan) (LegRow, error)
func (s *Service) updateLeg(row LegRow, p legPlan) error
func (s *Service) deleteLeg(row LegRow) error

// validatePair asserts, for any two legs on any tables: both carry the same
// transfer_id; each transfer_account_id points at the OTHER leg's account;
// amounts are equal and opposite; the accounts differ. It replaces
// transaction.TransferPair.Validate (transaction.go:515) and, critically, is
// also run on the undo/Recreate path — RecreateTransferPair
// (transaction_service.go:1299) routes through Service.Create per leg today and
// therefore skips pair validation entirely.
func validatePair(a, b LegRow) error
```

### 2.6 Reading: one cross-table query

```go
// internal/transfer/read.go

// legSelect is the unified projection over both ledgers. Both tables index
// transfer_id (idx_transactions_transfer, idx_inv_tx_transfer).
//
// This is deliberately a Go query string and NOT a database VIEW: a view would
// have to be dropped and recreated by every future migration that rebuilds
// either table — the recipe migration 019 already follows for
// portfolio_holdings / account_balances / category_spending — and it buys
// nothing a string does not.
//
// PRE-PHASE-7: investment_transactions has no category_id column, so the
// investment arm projects a typed NULL. Phase 7 replaces the literal with the
// real column and nothing else changes.
const legSelect = `
SELECT 'regular' AS ledger, CAST(NULL AS VARCHAR) AS inv_type,
       t.id, t.account_id, t.date, t.amount, t.memo, t.status,
       t.category_id, t.transfer_id, t.transfer_account_id, t.created_at
FROM transactions t
WHERE t.transfer_id IS NOT NULL
UNION ALL
SELECT 'investment' AS ledger, CAST(i.transaction_type AS VARCHAR) AS inv_type,
       i.id, i.account_id, i.date, i.total_amount, i.memo, i.status,
       CAST(NULL AS UUID) AS category_id, i.transfer_id, i.transfer_account_id, i.created_at
FROM investment_transactions i
WHERE i.transfer_id IS NOT NULL
`

// Get loads a transfer by its transfer_id, reading both ledgers. Unlike
// TransferRepository.GetByTransferID it does not assert two rows on the regular
// table; it asserts the arity appropriate to the SHAPE, and returns
// *MalformedPairError otherwise (naming the per-table counts).
//
// Shape must be decided BEFORE the arity check, because the two shapes have
// different arities:
//
//   - ShapeWhole is two ledger rows, one per leg.
//   - ShapeSplitLine is ONE ledger row — the counterpart minted in the target
//     account — plus the transaction_splits row that IS the other side. A split
//     line has no second transaction row, so asserting two ledger rows first
//     reports every split line as malformed.
//
// (v3 correction: v2 specified a flat two-rows-across-both-ledgers assertion
// alongside "a split line resolves successfully". Those are incompatible;
// implementing phase 1 surfaced it as a hard test failure. The split row's side
// is surfaced as a Leg with Ledger == LedgerSplit — see kind.go — so the read
// model can describe it without inventing a fake transaction row.)
func (s *Service) Get(transferID types.ID) (*Transfer, error)

// Resolve loads the whole transfer from ANY leg's row ID, probing the regular
// table then the investment table.
//
// Replaces cli/transfer/resolve.go in its entirety and the cross-table halves
// of both TUI edit loaders.
//
// A split-line transfer RESOLVES SUCCESSFULLY, with Shape == ShapeSplitLine —
// reads must work so callers can explain the refusal. Only the verbs refuse.
//
// Errors: dberrors.NotFoundError (no such row in either table);
// *transaction.IsNotTransferError (the row carries no transfer_id — this typed
// error exists at transaction_errors.go:107 and finally gets a production
// producer, replacing two bare fmt.Errorf sites at transfer_repository.go:137
// and :289).
func (s *Service) Resolve(legRowID types.ID) (*Transfer, error)
```

`Resolve` is also a bug fix for the TUI. Today `loadEditTransferDialogData`
decides which service to call by scanning `a.accounts`, which is populated by
`accountSvc.List(true)` — **active only**. `accountTypeByID`
(`tui/transfer_dialog.go:30-37`) returns `""` for a missing account, which reads
as non-investment, so an inv↔reg transfer whose investment counterpart is closed
is misrouted to `GetTransferPair` and errors. `Resolve` reads the account rows
directly and is unaffected by what the dialog happens to have loaded.

### 2.7 Create

```go
// internal/transfer/create.go

// Spec is the complete input. Amount is always a POSITIVE magnitude; the signs
// are applied per leg by planLegs.
type Spec struct {
	FromAccountID types.ID
	ToAccountID   types.ID
	Date          types.Date
	Amount        types.Money
	Memo          string
	CategoryID    types.NullableID // zero value = no category
	Status        transaction.Status // StatusUncleared for a plain create
}

// LegRef identifies a written leg. Presentation uses it for post-save cursor
// restoration.
type LegRef struct {
	Ledger    Ledger
	RowID     types.ID
	AccountID types.ID
}

// Result is the ONE result shape.
type Result struct {
	TransferID types.ID
	Kind       Kind
	From       LegRef
	To         LegRef
	Before     *Transfer // nil for Create; the pre-edit state for Update
}

func (r *Result) LegForAccount(acctID types.ID) (LegRef, bool)

// Create writes both legs — each to the ledger its account belongs to — inside
// one transaction. This ONE method replaces transaction.CreateTransfer,
// investment.TransferCash, investment.DepositFromAccount and
// investment.TransferCashBetweenInvestments.
//
// Guards, in this FIXED order, applied uniformly to both legs. The order is
// part of the contract: today the error a user sees for e.g. a same-account
// closed investment transfer depends on which of four paths they took
// (transaction raises a ServiceValidationError wrapped in "failed to create
// transfer:", the three investment paths raise "cannot transfer between the
// same account").
//
//  1. amount strictly positive     -> *InvalidAmountError
//  2. from != to                   -> *SameAccountError
//  3. both accounts exist          -> dberrors.NotFoundError
//  4. neither account closed       -> account.AccountClosedError
//  5. date >= both opening dates   -> account.ValidateTransactionDate's error
//  6. category exists + non-system -> dberrors.NotFoundError /
//                                     *transaction.SystemCategoryTransferError
//  7. category storable for Kind   -> *CategoryNotSupportedError  [deleted in phase 7]
//  8. status valid                 -> *InvalidStatusError
//
// Steps 6 and 7 are NEW ENFORCEMENT for the three investment paths: internal/
// investment performs no category validation whatsoever and its comments at
// investment_service.go:1135-1138 delegate it to the caller. Inputs the
// investment paths accept today will be rejected. That is the point.
func (s *Service) Create(spec Spec) (*Result, error)
```

### 2.8 Update, Reverse, status

```go
// internal/transfer/write.go

// Edit carries the mutable fields. Accounts are IMMUTABLE — re-accounting a
// transfer is Delete + Create; flipping direction is Reverse.
type Edit struct {
	Date       types.Date
	Amount     types.Money // positive magnitude
	Memo       string      // "" clears
	Status     transaction.Status
	CategoryID types.NullableID // unset clears
}

// Update rewrites both legs IN PLACE for every Kind. Row IDs and the
// transfer_id are preserved.
//
// This is the largest behavioural change in the design and it is a fix.
// investment.Service.UpdateTransferCash implements "edit" as
// delete-both-legs-then-recreate (update_edit.go:604-648), minting a new
// transfer_id and new row IDs. In-place editing is SAFE for cash transfers
// because reverseTxnEffects is a documented no-op for
// TransactionTypeTransferCash (update_edit.go:41-47) — a cash row carries no
// lot or position state to unwind. It removes four defects at once:
//
//   - the TUI's stale cursor: transfer_dialog.go:969-978 comments "the
//     investment leg's ID is unchanged by the edit" and stashes the OLD id —
//     of a row update_edit.go:604 just deleted;
//   - the category wipe that forces the CLI's defensive pre-read at
//     cli/transfer/resolve.go:192-203;
//   - the orphaning of any split line sharing the transfer_id;
//   - un-undoable investment edits.
//
// Re-accounting was never reachable from a front end: `transfer edit` exposes
// no --from/--to (cli/transfer/edit.go:37-70) and the TUI renders From→To as a
// read-only banner (transfer_dialog.go:180). Both derive UpdateTransferCash's
// `direction` argument from the stored orientation (cli/transfer/edit.go:207-217,
// tui/transfer_dialog.go:955-965), so its flip and re-account branches are dead
// from the UI. The direction-flip CAPABILITY is preserved as Reverse.
//
// Guards: checkMutable, then the Create guard list on the new values, then
// Movement/Shape refusals. When only Status changed, the narrow status path
// (below) is used instead of a full-row rewrite.
func (s *Service) Update(transferID types.ID, edit Edit) (*Result, error)

// Reverse swaps which account is From and which is To, in place, by negating
// both legs' amounts and swapping their transfer_account_id cross-references.
// This is what UpdateTransferCash's `direction string` parameter existed for
// (pinned by TestUpdateTransferCash_InvToInv_FlipDirection,
// investment_service_test.go:3686 — that test is RETARGETED at Reverse, not
// retired, so the capability keeps its coverage).
func (s *Service) Reverse(transferID types.ID) (*Result, error)

// SetStatus writes one status to BOTH legs. SetLegStatus writes ONE leg — the
// register's cleared toggle, which today routes through
// transaction.Service.Update (register_view.go:259) and silently desyncs the
// pair because Update rewrites one leg with no mirroring.
//
// Both use the NARROW status-only UPDATE:
// transaction.Repository.UpdateStatus (transaction_repository.go:376) and a new
// investment.Repository.UpdateStatus. DuckDB rewrites an UPDATE touching an
// indexed column as an internal DELETE+INSERT that aborts on a desynced ART
// index — the reconcile-finish bug behind migration 030. Both tables index
// transfer_id, so a full-row rewrite for a status change is exactly the hazard
// the codebase engineered around everywhere except UpdateTransfer.
func (s *Service) SetStatus(transferID types.ID, status transaction.Status) error
func (s *Service) SetLegStatus(legRowID types.ID, status transaction.Status) error

// checkMutable is THE precondition gate. It replaces
// transaction.checkTransferEditable (:1525), UpdateTransfer's inlined subset
// (:1363-1376), voidTransfer's different inlined subset (:1716-1729), Delete's
// third inlined subset (:313-322), and the presentation copies at
// cli/transfer/edit.go:99, cli/transfer/delete.go:68,
// tui/register_view.go:194-202, :283-292, :335-352.
//
//   either leg reconciled            -> *transaction.IsReconciledError
//   either leg void (unless allowVoid) -> *transaction.IsVoidError
//   either leg's account closed      -> account.AccountClosedError
//
// NEW COVERAGE: the reconciled and void checks now apply to investment-involving
// transfers in the DOMAIN, where today they exist only in the CLI.
func (s *Service) checkMutable(t *Transfer, allowVoid bool) error
```

### 2.9 Void, delete, and their undo tokens

```go
// LegSnapshot captures one leg's mutable state ADDRESSED BY ROW ID, never by
// orientation. This is what makes the void→undo direction-reversal bug
// (§2.3) structurally impossible rather than merely documented.
type LegSnapshot struct {
	RowID  types.ID
	Ledger Ledger
	Amount types.Money
	Memo   types.NullableString
	Status transaction.Status
}

type VoidSnapshot struct {
	TransferID types.ID
	Legs       [2]LegSnapshot
}

// Void zeroes both amounts, stamps memo "**VOID**" and sets status void on both
// legs. Returns *VoidNotSupportedError when !t.Kind.SupportsVoid(): the
// investment status enum has no 'void' value and adding one requires reversing
// position/lot effects and excluding the row from total return (Future-H). That
// typed refusal replaces today's failure mode, which is
// TransferRepository.GetByTransferID:115-117 erroring with "expected 2
// transactions for transfer, found 1" — reachable right now via
// `tmoney transaction void <bank leg of an inv↔reg transfer>`, because
// cli/transaction/void.go has no transfer-kind guard (unlike edit.go and
// delete.go's guardTransactionEditable) and void_test.go has no transfer case.
func (s *Service) Void(legRowID types.ID) (*VoidSnapshot, error)

// Restore reverses a Void, applying each snapshot to its own RowID. Requires
// every leg to still be void; otherwise *NotVoidError.
func (s *Service) Restore(snap *VoidSnapshot) error

// LegRow is a whole persisted leg. Exactly one pointer is non-nil.
type LegRow struct {
	Ledger     Ledger
	Regular    *transaction.Transaction
	Investment *investment.Transaction
}

type DeleteSnapshot struct {
	TransferID types.ID
	Kind       Kind
	Rows       [2]LegRow
}

// Delete removes both legs after checkMutable, in one transaction, from
// whichever tables hold them. It does NOT delegate to
// investment.Service.DeleteTransaction's cascade — that method has no
// reconciled guard, so delegating would let the guards be bypassed.
func (s *Service) Delete(transferID types.ID) (*DeleteSnapshot, error)

// Recreate re-inserts both captured rows verbatim — original row IDs,
// transfer_id, dates and statuses — after validatePair. Both repositories'
// Create persist a supplied ID (transaction_repository.go:43+,
// investment_repository.go:78), so identity survives a delete/undo round trip
// for EVERY kind. Today only bank↔bank does; investment deletes are not
// undoable at all.
func (s *Service) Recreate(snap *DeleteSnapshot) error
```

### 2.10 LinkExisting — transferlink, with today's semantics preserved

```go
// LinkExisting stamps a shared transfer_id and cross-references onto two
// existing, unlinked REGULAR rows. It is the write half of
// internal/transferlink, which today reaches past every service guard straight
// into s.transferRepo.WithTx(tx).Update(pair) (transferlink.go:230-232) inside
// its own db.WithTx.
//
// DELIBERATELY RELAXED GUARDS. transferlink's isEligible (transferlink.go:249)
// filters IsTransfer, IsVoid, zero amount, inactive account, investment type and
// split-bearing rows — it does NOT filter reconciled, and linkOne does no date
// validation at all. Applying Update's full guard set here would be a capability
// REDUCTION on exactly the legacy-import cleanup this feature exists for.
// LinkExisting therefore runs: both rows exist, both regular, neither already a
// transfer, different accounts, amounts net to zero, neither void. No reconciled
// check, no opening-date check. Documented, tested, and intentional.
//
// categoryID is transferlink's already-normalized "outflow leg wins" choice
// (transferlink.go:217-224) — that merge rule stays in transferlink, it is
// transferlink's rule and exists nowhere else.
func (s *Service) LinkExisting(fromRowID, toRowID types.ID, categoryID types.NullableID) (*Transfer, error)
```

### 2.11 The refusal table

| Situation | Verb | Result |
|---|---|---|
| `Movement == MovementShares` | any cash verb | `*ShareTransferError` |
| `Shape == ShapeSplitLine` | Update / Delete / Void / Reverse | `*SplitLineTransferError` |
| `Shape == ShapeSplitLine` | Resolve / Get | succeeds, `Shape` set |
| `!Kind.SupportsVoid()` | Void | `*VoidNotSupportedError` |
| `!Kind.SupportsCategory()` + category set | Create / Update | `*CategoryNotSupportedError` (deleted in phase 7) |
| either leg reconciled | Update / Delete / Void / Reverse / SetStatus | `*transaction.IsReconciledError` |
| either leg void | Update / Delete | `*transaction.IsVoidError` |
| either leg's account closed | Create / Update / Delete / Void | `account.AccountClosedError` |
| new accounts requested | Update | not expressible — `Edit` has no account fields |

---

## 3. Transfer lines inside splits: stay in `transaction`

A transfer *line* is a row in `transaction_splits` carrying a
`transfer_account_id` + `transfer_id`, whose counterpart is a whole row in
either ledger. It shares the `transfer_id` column with whole-transaction
transfers and nothing else: the parent transaction is **not** itself flagged as
a transfer, and the linkage lives on the split row.

**They do not move, and the review's suggestion that the adapter "can shrink to
an internal detail of that façade" is wrong.** The counterpart write must commit
in the same transaction as the split row write, and split rows belong to
`transaction.Service` (`CreateWithSplits:478`, `UpdateSplit:688`,
`moveTransferLine:769`, `DeleteSplit:895`, `ReplaceSplits:954`,
`deleteTransferLinePairs:372`, `VoidTransaction:1663`). Moving the counterpart
means moving the whole split lifecycle — a strictly larger change on a different
axis, and the most heavily pinned code in the area (12 tests across
`split_investment_test.go`, `replace_splits_transfer_test.go`,
`transfer_line_category_test.go`, `split_counterpart_test.go`).

What *does* change inside `transaction.Service`:

**The five-copy lookup ladder collapses to one resolver.** Today
`findPairedByTransferID` (`:792`), `deletePairedCounterTransaction` (`:394`),
`mirrorToPairedCounterpart` (`:814`), `ensureRetainedCounterpartMutable`
(`:852`) and `ensureCounterpartNotReconciled` (`:1183`) each re-implement "look
on `transactions` by transfer_id; else ask the port".

```go
// internal/transaction/counterpart.go

// counterpart is the single paired row for a transfer-LINE's transfer_id, on
// whichever ledger holds it. Exactly one of the two is populated when Found().
type counterpart struct {
	regular       *Transaction
	investmentID  types.ID
	investmentRec bool
	onInvestment  bool
}

func (c counterpart) Found() bool
func (c counterpart) OnInvestment() bool   // the side MUST stay visible — see below
func (c counterpart) IsReconciled() bool

func (s *Service) resolveCounterpart(transferID types.ID) (counterpart, error)
```

**`OnInvestment()` is not decoration.** The reconciled rule is asymmetric and the
asymmetry is pinned. A reconciled REGULAR counterpart always blocks, because a
regular counterpart mirrors both amount and category. A reconciled INVESTMENT
counterpart blocks **only when the amount changed**, because
`investment_transactions` has no category column so a category-only edit never
writes it. Verified at `transaction_service.go:828-831` and `:863-872`; pinned by
`transfer_line_category_test.go:427` and `:453`. A uniform `IsReconciled()`
accessor with no side information would silently break both tests. The collapse
keeps one *lookup* and two distinct *rules*.

Two gaps are closed while in the neighbourhood:

- **Split transfer lines finally get category validation.**
  `validateTransferCategory` is called only from `CreateTransfer` (`:1261`) and
  `UpdateTransfer` (`:1358`); `scheduled`'s copy is gated on `st.IsTransfer()`
  so it covers single-line schedules only. Nothing stops a caller labelling a
  transfer line with the system `Transfer` category — the only defence is that
  the TUI hides them from the picker. The new free function (§7) is called from
  `CreateWithSplits`, `UpdateSplit` and `ReplaceSplits`.
- **The bank-side counterpart gets the opening-date guard.** An investment-side
  counterpart gets it today via `CreateTransferCashCounterpart` →
  `validateTransaction` (`investment_service.go:1365`); the bank-side one is
  written by a bare `txnRepo.Create` (`:573`), which only does FK-existence
  checks. A back-dated paycheck can plant a bank counterpart before the target
  account opened.

And one invariant that must NOT be lost: `rejectInvestmentAccount`
(`:1219`) and `NotRegularAccountError` (`transaction_errors.go:125`) **stay**.
Their only callers today are `CreateTransfer` and `UpdateTransfer`, both of
which are deleted, so the naive move is to delete the guard as an emergent
property of `planLegs`. It is not: `transaction.Service.Create` will still write
any `account_id` handed to it. The guard is re-pointed at
`createTransferLineCounterpart`'s regular branch, where the routing already
guarantees it passes — a live backstop with zero behavioural change. The
duplicate in `internal/investment` is deleted (§7).

---

## 4. Share transfers: out of scope, and why

`investment.Service.TransferShares` (232 lines), `UpdateTransferShares` (85) and
`reverseTransferShares` (65) stay exactly where they are, along with
`DeleteTransaction`'s `transfer_shares` cascade.

**Reason:** both legs live in `investment_transactions`. There is no cross-ledger
pairing to own, which is the entire problem this design exists to solve. What the
operation actually contains is lot allocation, cost-basis preservation, position
reduction and reversal — moving it would drag `lotRepo`, `positionRepo`,
`transactionLotRepo`, `reverseShareRemoval`/`reverseShareAddition` and
`syncPositionAndLots`/`healInOwnTx` into `internal/transfer`, making the transfer
package larger than everything it replaces and importing the investment package's
internals rather than its ledger.

**But they are classified.** `Movement` exists in the read model so that every
cash verb refuses `MovementShares` with `*ShareTransferError`, and
`investment.Service.UpdateTransferCash`'s replacement path asserts the row type.
That closes a live corruption hole: `UpdateTransferCash` never checks `old.Type`,
so handing it a buy's ID today deletes the buy via `repo.Delete` at `:604`
**without** calling `reverseTxnEffects`. Nothing else covers this —
`UpdateTransferShares` checks only `srcOld.TransferID.Valid`
(`update_edit.go:707-709`), which a `transfer_cash` row passes, so the two cash
and share edit verbs can each be handed the other's row. Only `Movement`
classification closes it.

**Consequence, stated plainly:** the "one owner" rule has one documented
exception, and share transfers remain non-undoable in the TUI.

---

## 5. The import cycle and the counterpart port

### 5.1 Why the port exists today

`internal/investment` imports `internal/transaction`
(`investment_service.go:12`), so the reverse edge is forbidden by the compiler.
`InvestmentCashCounterpartAdapter` (`transaction_service.go:27-65`) inverts the
dependency for split-line counterparts: the interface is declared by the
*consumer*, satisfied by `*investment.Service`, wired post-construction at
`registry.go:113`, with `db.Queryer` as the only shared vocabulary — which is
exactly why `Queryer` lives in `internal/db`. The tx-binding method is spelled
`CounterpartInTx`, not `InTx`, purely because `investment.Service` already has
`InTx(tx) *Service` and Go forbids two methods sharing a name with different
signatures (`investment_service.go:61-69`).

### 5.2 The edge is severed, not flipped

Grep for `transaction.` in `internal/investment`'s non-test files returns
exactly twelve production uses, and **every one of them is cash-transfer or
adapter code this design deletes**:

| Use | Site | Fate |
|---|---|---|
| `txnRepo *transaction.Repository` field | `investment_service.go:24` | deleted with the cash paths |
| `CounterpartInTx` return type | `:67` | port reshaped (below) |
| `txnRepo` constructor param | `:110` | deleted |
| `CashTransferResult.RegularTransaction` | `:1069` | struct deleted |
| `transaction.NewTransaction` ×2 | `:1139`, `:1229` | `TransferCash`/`DepositFromAccount` deleted |
| `statusFromRegular` | `update_edit.go:19-23` | moved to `internal/transfer/status.go`, **not** to an `investment` file — see the note below |
| `UpdateTransferCash` status param | `update_edit.go:537` | method deleted |
| `applyInvestmentStatus`/`applyRegularStatus` | `update_edit.go:658`, `:675` | deleted |

**The status mapping must not stay in `internal/investment`.** This is the one
place where the sever is easy to lose by accident. `statusFromRegular` is typed
on `transaction.Status` (`update_edit.go:19`), so *any* home for it inside
`internal/investment` — including a new `status.go` — keeps the import edge alive
and fails phase 5's own exit criterion. It belongs in `internal/transfer`, which
imports both ledgers and is, after the deletions, its only consumer
(`Leg.Status` normalization and `updateLeg`). `internal/investment` keeps only
its own `TransactionStatus` constants, which name no foreign type.

So the port only has to lose one thing — its self-returning tx-binder — and
`internal/investment` stops importing `internal/transaction` **entirely**:

```go
// internal/transaction/counterpart_port.go

// InvestmentCounterpartPort is how transaction.Service mints, finds, deletes
// and amends the investment_transactions row that is the counterpart of a
// transfer LINE inside a multi-line split (e.g. a paycheck → 401k line).
//
// It replaces InvestmentCashCounterpartAdapter. Two changes:
//
//   1. The tx is an explicit db.Queryer PARAMETER, not a bound-copy return.
//      The old CounterpartInTx had to name transaction.InvestmentCash-
//      CounterpartAdapter as its return type, which is what forced
//      internal/investment to import internal/transaction. Passing the Queryer
//      per call removes the last cross-package type reference, the
//      CounterpartInTx naming hack, and a whole interface method.
//   2. It is injected at construction, not set afterwards. Once
//      investment.NewService no longer takes a *transaction.Repository, the
//      construction order inverts freely: build investmentSvc, then txnSvc with
//      the port. SetInvestmentCounterpart is deleted.
//
// A nil port means transfer LINES targeting an investment account are refused
// (ensureTransferTargetRoutable) rather than written as a malformed regular
// row. That refusal now returns a typed *InvestmentTargetUnroutableError; today
// it is a bare fmt.Errorf at :609-612 despite the doc comment at :102-103
// claiming NotRegularAccountError, so nothing can errors.As on it.
type InvestmentCounterpartPort interface {
	CreateCounterpart(q db.Queryer, invAcctID, otherAcctID types.ID,
		date types.Date, amount types.Money, memo string, transferID types.ID) (types.ID, error)
	FindCounterpart(q db.Queryer, transferID types.ID) (rowID types.ID, reconciled, found bool, err error)
	DeleteCounterpart(q db.Queryer, rowID types.ID) error
	UpdateCounterpartAmount(q db.Queryer, rowID types.ID, newAmount types.Money) error
}
```

`transaction.Service` passes `s.q()` — the bound tx when inside one, the live
connection otherwise. `investment.Service` implements each method as
`s.InTx(q).<existing body>`, so the four existing implementations
(`investment_service.go:1348`, `:1380`, `:1395`, `:1411`) keep their bodies and
their guards, including `requireInvestmentAccount` and `validateTransaction`.
That last point matters: a naive "just use `*investment.Repository` directly"
would silently drop the opening-date guard on the investment-side counterpart
while advertising the opposite.

### 5.3 Why not flip the edge and delete the port entirely

It was considered and rejected on two grounds.

**Semantics.** After the deletions, the *only* reason for an edge in either
direction is split transfer-lines. Making `transaction` import `investment` buys
four repository operations at the cost of pulling the whole investment package —
positions, lots, corporate actions, valuation, and transitively `price` and
`security` — into `transferlink`, `scheduled`, `reconciliation`, `imexport` and
`cli/transaction`. A four-method interface that names exactly what is needed is
better engineering than importing 5,000 lines to use four of them.

**Tests.** All 33 files in `internal/investment` are `package investment` (zero
external test packages), and four of them import `internal/transaction`. Flipping
the edge makes those an `import cycle not allowed in test` compile failure. Three
of the four resolve themselves — `cash_position_test.go` is deleted with
`cash_position.go`; `investment_service_test.go`'s 22 references are the two
`createTestService` helper wirings at `:28`/`:76` plus twenty `UpdateTransferCash`
test lines at `:3472-3934`, all of which go away with the deletions;
`transfer_category_test.go` moves into the transfer suite. But
`split_counterpart_test.go` (528 lines) genuinely tests the port from inside the
package and would have to be relocated to `package investment_test`, exporting or
duplicating `createTestDB`. Severing rather than flipping means it never comes up.

### 5.4 Final wiring

```go
// internal/app/registry.go — construction order inverts; the setter is gone.
investmentSvc := investment.NewService(investmentRepo, accountRepo, positionRepo,
	lotRepo, transactionLotRepo, priceRepo, corporateActionRepo, database)
txnSvc := transaction.NewService(txnRepo, splitRepo, payeeRepo, accountRepo,
	investmentSvc /* InvestmentCounterpartPort */, database)
transferSvc := transfer.NewService(txnRepo, investmentRepo, splitRepo,
	accountRepo, categoryRepo, database)
// Services gains: Transfer *transfer.Service
// transferRepo is no longer constructed — transfer_repository.go is deleted.
```

---

## 6. Invariant preservation

Every invariant from the exploration map. "Same" means the code is not touched.

| # | Invariant | Today | After |
|---|---|---|---|
| 1 | Sign: From = `amount.Neg()`, To = `amount.Abs()`; caller supplies a positive magnitude | `transaction.go:501-505`, `transaction_service.go:1382`, `transfer_repository.go:206`, `investment_service.go:1124/1214/1297` | `transfer/rows.go planLegs` (one site) |
| 2 | Amount strictly positive | `transaction_service.go:1246`, `:1434`; `investment_service.go:1083/1175/1277`; NOT in `UpdateTransfer` | `transfer.Service.guard` step 1, all verbs incl. Update |
| 3 | From ≠ To | `transaction.go:555`; `investment_service.go:1109/1201/1281` (three different points in three different orders) | `transfer.Service.guard` step 2, `*SameAccountError` |
| 4 | Shared transfer_id, cross-referenced transfer_account_id | `transaction.go:498-506` | `transfer/rows.go` mint + `validatePair` |
| 5 | transfer_id ⟺ transfer_account_id on a row | `transaction.go:339-341`, `:269-271`; DB CHECK `029:54` (splits) | same |
| 6 | Amounts equal and opposite; accounts differ | `TransferPair.Validate`, `transaction.go:537-558`; **bypassed by `RecreateTransferPair:1299`** | `transfer.validatePair`, **also on Recreate** |
| 7 | Category optional, must exist, must be non-system | `transfer_category.go:31` + `transaction_service.go:1412`; `scheduled_service.go:1050` (copy); never for inv paths or split lines | `transaction.ValidateTransferCategoryByID` (free fn), called by transfer, scheduled, and the split paths |
| 8 | Category mirrored on both legs (reg↔reg) / regular leg only (inv↔reg) / impossible (inv↔inv) | `transaction_service.go:1272`, `:1391`; `investment_service.go:1135-1146` | emergent from `planLegs`: only a `LedgerRegular` leg has the column. Phase 7 makes it uniform |
| 9 | inv↔inv cannot carry a category | 5 presentation sites, **no domain guard** | `Kind.SupportsCategory()` + `*CategoryNotSupportedError`; deleted by phase 7 |
| 10 | Investment account never owns a `transactions` row | `rejectInvestmentAccount:1219` via `CreateTransfer`/`UpdateTransfer` | structural in `LedgerFor`; guard retained and re-pointed at `createTransferLineCounterpart` (§3) |
| 11 | Reconciled leg blocks edit/void/delete | `checkTransferEditable:1531` + 3 inline + 6 presentation; **absent from the investment domain** | `transfer.checkMutable` (one site), now covering all kinds |
| 12 | Void leg blocks edit/delete | `:1537`, `:1371`, `:206`, `:277` | `transfer.checkMutable` |
| 13 | Closed account freezes the transfer (either leg) | `guardTransferDate:1475`, `checkTransferEditable:1544`, `ensureAccountOpen:1490`; `investment_service.go:1104/1196/1701`, `update_edit.go:547/717` | `transfer.Service.guard` step 4 + `checkMutable` |
| 14 | Date ≥ both accounts' opening dates | `guardTransferDate:1478`; `investment_service.go:1115/1207`, `:644-652` | `transfer.Service.guard` step 5, per leg, no nil-repo hole |
| 15 | Void semantics: amount→0, memo `**VOID**`, both legs | `transaction_service.go:1732-1738`; zero-amount exemption `transaction.go:321` | `transfer.Service.Void` (reg↔reg only, typed refusal otherwise) |
| 16 | Both legs atomic | `runInTx` at `:1283`/`:1400`/`:1518`; `investment_service.go:1149/1239/1318` | `transfer.Service.runInTx`, one site per verb |
| 17 | Bound service joins, never nests `db.WithTx` | `design-withtx.md` §3 | same contract, `transfer.Service.InTx` |
| 18 | Narrow status-only UPDATE (DuckDB ART hazard) | `transaction_repository.go:376`; used by `transfer_repository.go:245` and `reconciliation`; **`UpdateTransfer` takes the wide path anyway** | `SetStatus`/`SetLegStatus`, both ledgers (new `investment.Repository.UpdateStatus`); `Update` uses it when only status changed |
| 19 | Half-reconciled transfers are reachable | `reconciliation_service.go:283` writes one leg at a time | same — `reconciliation` still writes through the repository; only the service door closes |
| 20 | D10: a split-line transfer_id is not a whole transfer | `cli/transfer/resolve.go:44/237` (CLI only) | `Transfer.Shape` + `*SplitLineTransferError` (domain; TUI gains it) |
| 21 | Counterpart amount = negated split-line amount | `transaction_service.go:548`, `:754`, `:1008` | same |
| 22 | Category on a transfer line mirrors to a REGULAR counterpart only | `:535-538`, `:570-572`, `:824` | same |
| 23 | Category-only edit must not touch/be blocked by an investment counterpart | `:828-831`, `:863` (`amountChanged` gate) | same — `counterpart.OnInvestment()` preserves the asymmetry (§3) |
| 24 | Reconciled counterpart blocks split ops | `:400`, `:418`, `:820`, `:858`, `:1189` | same, via `resolveCounterpart` |
| 25 | Reconciled parent blocks reverse cascades | `:257`, `:356` | same |
| 26 | One split row per transfer_id | `split_repository.go:166` | same |
| 27 | A multi-line parent is never itself a transfer | invariant by construction, `:788-791` | same |
| 28 | Retained transfer_id survives ReplaceSplits | `planSplitReplacement:1121-1153` | same — untouched |
| 29 | Investment-target transfer line requires a wired port | `ensureTransferTargetRoutable:600-613` (bare `fmt.Errorf`) | same rule, now `*InvestmentTargetUnroutableError` |
| 30 | Self-transfer forbidden on a split line | `:715-720`, `:1971-1977` | same |
| 31 | Transfers cannot have splits; cannot be duplicated | `:646`, `:1873`, `:1887` | same |
| 32 | Transfers excluded from spending reports | `report_service.go:207-208`; view guard `029:125` | same — verified `internal/report` never reads `investment_transactions` |
| 33 | Cash is never stored; = Σ `AffectsCash()` rows | `investment_service.go:519-533` | same — and `insertLeg` asserts `TransactionTypeTransferCash` (§2.5) |
| 34 | Investment cash may go negative | `investment_service.go:1119`, `:1292` | same |
| 35 | Reverse position/lot effects before delete | `update_edit.go:30-32`, `investment_repository.go:326` | same — cash legs are a documented no-op (`update_edit.go:41-47`) |
| 36 | Cost-basis preservation, lot allocations | `investment_service.go:1487-1551` | same (out of scope, §4) |
| 37 | Exchange rows undeletable | `investment_service.go:1695` | same |
| 38 | transferlink normalizes to one category, outflow wins | `transferlink.go:215-224` | same — stays in transferlink |
| 39 | Scheduled templates never own a transfer_id | `split_item.go:16-18`; no column (`029:79-95`) | same |
| 40 | Scheduled: no closed account as source/destination/line target | `scheduled_service.go:120-134` | same |
| 41 | Posting a transfer + advancing next_date commit together | `scheduled_service.go:674-688`, `:338-445` | same shape, via `transferSvc.InTx(tx).Create` |
| 42 | transferlink candidate eligibility | `transferlink.go:249-268` | same — `LinkExisting` deliberately does not add guards (§2.10) |

---

## 7. Error-type unification

### 7.1 Deleted duplicates

| Type | Sites today | Fate |
|---|---|---|
| `InvalidTransferAmountError` | `transaction_errors.go:48`, `investment_service.go:1937` (field-for-field twins, identical message) | both deleted → `transfer.InvalidAmountError`, same message text |
| `NotRegularAccountError` | `transaction_errors.go:125`, `investment_service.go:1946` | the investment copy is deleted; the transaction one **survives** with a real call site (§3). They cannot merge — the messages point in opposite directions ("use investment.Service" vs "use transfer between investment accounts instead") |
| `investment.InsufficientCashError` | `investment_errors.go:9-19` | deleted with `cash_position.go` — dead in production, referenced only from `cash_position_test.go`, and its non-negative-balance rule is the **opposite** of the live invariant (#34) |
| `account.IsClosedError` | `account_errors.go:62-69` | deleted; single producer is `reconciliation_service.go:94`, which switches to `account.AccountClosedError` |

**`scheduled.ClosedAccountError` is deliberately NOT merged.** `AutoPost`'s
skip-versus-abort branch (`scheduled_service.go:318-325`) matches that exact type
with `errors.As`. Folding it into `account.AccountClosedError` would silently
convert every closed-account skip into a batch abort.

### 7.2 Owned by `internal/transfer`

```go
type InvalidAmountError struct{ Amount types.Money }        // "transfer amount must be positive, got %s"
type SameAccountError struct{ AccountID string }
type InvalidStatusError struct{ Status string }
type CategoryNotSupportedError struct{ Kind Kind }          // deleted in phase 7
type VoidNotSupportedError struct{ Kind Kind }
type NotVoidError struct{ RowID string }
type ShareTransferError struct{ RowID string }
type SplitLineTransferError struct{ LegID, TransferID, ParentID string }
type MalformedPairError struct{ TransferID string; Regular, Investment int }
```

`SplitLineTransferError.Error()` **preserves the literal substring
`"multi-line split"`** from `cli/transfer/resolve.go:44-55`. Three tests assert
on it (`resolve_test.go`, `delete_test.go`, `edit_test.go`).

### 7.3 Re-used, not re-declared

`*transaction.IsReconciledError`, `*transaction.IsVoidError`,
`*transaction.IsNotTransferError`, `*transaction.IsTransferError`,
`*transaction.SystemCategoryTransferError`, `*transaction.NotRegularAccountError`,
`*transaction.SelfTransferError`, `account.AccountClosedError`,
`*dberrors.NotFoundError`. Every existing `errors.As` on these keeps working.

`*transaction.IsNotTransferError` (`transaction_errors.go:107`) exists today with
**zero production producers** — `transfer_repository.go:137` and `:289` return
bare `fmt.Errorf` instead, so callers cannot match on it. `Resolve` becomes its
first producer.

### 7.4 `errors.As` migration

| Consumer | Change |
|---|---|
| `cli/transfer/*.go` | `investment.InvalidTransferAmountError` → `xfer.InvalidAmountError`; the two inline reconciled string errors and the three inv↔inv category strings are deleted (the domain now raises typed errors) |
| `cli/transfer/resolve.go` | `errTransferLineSplit` → `*xfer.SplitLineTransferError` |
| `cli/investment/edit.go:220` | unchanged (`guardEditable` refuses transfer rows by type, not by error) |
| `tui/transfer_dialog.go` | inline pre-checks for self-transfer / positive amount / opening date are **kept** — they produce per-field dialog errors the service cannot express, and `transfer_dialog_test.go:851/895` assert their exact wording |
| `reconciliation_service.go:94` | `account.IsClosedError` → `account.AccountClosedError` |
| `scheduled_service.go:318` | unchanged — still matches `scheduled.ClosedAccountError` |

---

## 8. Undo

Seven command types collapse to four, one per verb, all kind-agnostic.

```go
// internal/undo/transfer.go
func NewCreateTransferCommand(svc *transfer.Service, spec transfer.Spec) *CreateTransferCommand
func NewEditTransferCommand(svc *transfer.Service, transferID types.ID, edit transfer.Edit) *EditTransferCommand
func NewDeleteTransferCommand(svc *transfer.Service, transferID types.ID) *DeleteTransferCommand
func NewVoidTransferCommand(svc *transfer.Service, legRowID types.ID) *VoidTransferCommand
```

Deleted: `internal/undo/investment_transfer.go` in its entirety
(`CreateInvestmentTransferCashCommand`, `CreateInvestmentDepositCommand`,
`CreateInvestmentToInvestmentTransferCommand`) and the four transfer commands in
`internal/undo/transaction.go:207-443`.

Notes:

- **`Description()` strings are preserved verbatim** — `"Create transfer"`,
  `"Edit transfer"`, `"Delete transfer"`, `"Void transfer"`.
  `investment_transfer_test.go:127`, `:172` and `:225` assert the first literally.
- **Investment transfer edit and delete become undoable for the first time.**
  Today `tui/transfer_dialog.go:984` calls `UpdateTransferCash` directly and
  `tui/investment_register_view.go:640` calls `DeleteTransaction` directly, both
  bypassing `a.undoManager` — so Ctrl+Z after either silently undoes whatever
  came *before*.
- **Every command nil-guards its captured state.** Today
  `CreateTransferCommand.Undo` (`transaction.go:242`) and
  `DeleteTransferCommand.Undo` (`:289`) panic if `Undo` precedes `Execute`.
- **No undo-history compatibility concern.** `internal/undo/manager.go:11-12`:
  *"The stacks are not persisted — they are cleared when the application exits."*
  `tui/app.go` builds a fresh `undo.NewManager()` per run, so no pre-change
  command object can ever be replayed by post-change code.

Two undo paths delete a transfer leg through the **generic** transaction delete
and must be re-pointed, or phase 5's `*IsTransferError` refusal breaks them:

| Site | Call | Fix |
|---|---|---|
| `undo/scheduled_transaction.go:293` | `c.txnSvc.Delete(c.createdTxn.ID)` — `createdTxn` is documented at `:306` as the From leg, and `Execute` tests `txn.TransferID.Valid` at `:283` | route to `transferSvc.Delete` when `TransferID.Valid` |
| `undo/auto_post.go:53` | `c.txnSvc.Delete(result.Transactions[j].ID)` over every auto-posted row; for a transfer schedule that is the From leg written by `scheduled_service.go:355` | same |

Also `undo/scheduled_transaction.go:284` calls `txnSvc.UpdateTransfer` as a
second, **non-atomic** write after `PostWithDate` — the pair is created inside
the schedule's tx, then the preview's memo/status/category are applied in a
separate call. Phase 4 repoints it at `transferSvc.Update`; making it atomic with
the post is left open (§13).

---

## 9. Rollout plan

Each phase compiles, passes the full suite, and is a separate commit to main.
Phases 1–4 add the new path without deleting the old one, so a revert is a
single-file change per caller. Nothing is deleted until phase 5.

### Phase 1 — read model and `Resolve` (risk: low)

Create `internal/transfer` with `kind.go`, `transfer.go`, `errors.go`,
`service.go`, `read.go` and the read half of `rows.go`. Re-home the classifier as
`transfer.ClassifyKind` with `dispatch_test.go`'s truth table (preserving the
unknown-type → `KindRegToReg` case, and HSA-as-investment).
`internal/transaction/dispatch.go` itself is **not deleted here** — its five
presentation callers still switch on `TransferDispatchKind` until phase 3, and
phases 1–4 are additive by construction. It dies in phase 3 with the last
switch. (v3 correction: v2's phase 1 said "move", which contradicts the same
section's "nothing is deleted until phase 5" and would have left phase 1 red.)
Move the status mapping out
of `internal/investment` into a new `internal/transfer/status.go` as
`StatusToRegular` / `StatusFromRegular` — **not** into an `internal/investment`
file: the functions are typed on `transaction.Status`, so any home inside
`investment` preserves the very import edge phase 5 exists to sever (§5.2). Make
`StatusFromRegular` return `*UnrepresentableStatusError` for
`transaction.StatusVoid` instead of silently coercing to `Pending` (today
`update_edit.go:19-27` maps void→pending, so a void regular leg round-tripped
through an edit comes back Uncleared). Wire
`Services.Transfer`. Re-point `cli/transfer/resolve.go` and both TUI edit loaders
at `Resolve`; delete `tui.statusToRegular` and `cli.investmentStatusToRegular`.

Two behaviour improvements land in the TUI loader here rather than waiting for
phase 3, because both replace an outcome that is already wrong and neither can
regress anything: a split transfer-line and a share transfer are refused with a
typed message instead of (respectively) erroring with "expected 2 transactions"
on the bank side and silently corrupting on the investment side.

**Exit criteria.** `go build ./... && go test ./...` green.
The existing `cli/transfer` suite passing unmodified against the new resolver IS
the differential test — `resolveTransferPair` becomes a thin projection of
`Resolve`, so its 184-line white-box suite exercises both. New coverage in
`internal/transfer`: inv↔reg from either leg, reg↔inv from either leg, inv↔inv
from either leg, a `transfer_shares` pair (`Movement == MovementShares`), a split
transfer-line (`Shape == ShapeSplitLine`, with the parent named), a non-transfer
row (`*transaction.IsNotTransferError`), a missing row
(`*dberrors.NotFoundError`), a deliberately orphaned single leg
(`*MalformedPairError` with per-table counts), a voided reg↔reg pair resolving
with STABLE From/To across repeated reads, cross-ledger status normalization, and
the category being found from either leg of an inv↔reg pair.

**Test churn, actual (measured after implementing).** Far less than v2
predicted, because `resolvedTransfer` was kept as a projection rather than
deleted: `cli/transfer/resolve_test.go` passes unmodified. In the TUI only
`TestStatusToRegular_Mapping` is deleted (its coverage moved to
`transfer.TestStatusRoundTrip`, which additionally pins the reverse direction and
the void case the TUI copy got wrong), and `newTransferCategoryTestApp` gains a
`transferSvc` field. `investmentTransferEdit`, `transferLegForAccount` and
`cashTransferLegForAccount` are untouched until phase 3.

### Phase 2 — the signed write path (risk: medium)

Add the write half of `rows.go` (`planLegs`, `insertLeg`/`updateLeg`/`deleteLeg`,
`validatePair`), `guards.go` (`resolveAccounts`, the ordered guard preamble,
`checkMutable`), `create.go` and `write.go`. Add
`investment.Repository.UpdateStatus` (narrow) and
`transaction.ValidateTransferCategoryByID`. Nothing calls any of it yet.

**Exit criteria.** The matrix suite: 4 kinds × {Create, Update, Reverse,
SetStatus, SetLegStatus, Void, Restore, Delete, Recreate, LinkExisting}, plus the
guard matrix (non-positive amount, same account, closed account either side,
opening date either side, missing category, system category, category on inv↔inv,
reconciled leg, void leg, share transfer, split line). Fault-injection rollback
tests per kind, in the style of `transaction_service_tx_test.go:44`. Explicit
assertions that (a) every investment leg is written with
`TransactionTypeTransferCash` and (b) `GetCashBalance` moves by the expected
amount on both sides. Every behaviour pinned by
`investment/transfer_category_test.go`, `transaction/transfer_category_service_test.go`,
`transaction/closed_account_guard_test.go:94`,
`investment/closed_account_guard_test.go:84` and
`tests/integration/transfer_test.go` has an equivalent assertion against
`transfer.Service`.

**Two live implementations.** From here until phase 5, `transfer.Service.Create`
and `transaction.Service.CreateTransfer` both write reg↔reg pairs. They produce
identical rows **modulo `id`, `transfer_id`, `created_at` and `updated_at`**,
which are freshly minted per call and can never match across two invocations; the
differential test compares every other column plus the sign convention and the
pair relationship. This is the price of not having an unsplittable mega-commit.

### Phase 3 — front ends and undo (risk: high)

Collapse undo to the four commands. Rewrite `cli/transfer/{add,edit,delete}.go`
against `Spec`/`Edit`/`Resolve`, preserving the D7 confirmation-block label
widths (25/12/17 chars — `add_test.go:324` and
`specs/implementation-plan-investment-transfer-cli-parity.md:266-286` pin them),
the raw-trimmed `--category` echo, and the `cmd.Flags().Changed` tri-state that
distinguishes `--category ""` (clear) from `--category` unset (preserve). Rewrite
`tui/submitTransferDialog`, `submitEditTransferDialog`, both loaders and the
delete/void confirmations. Branch the investment register's delete: transfers to
`transferSvc.Delete`, everything else stays on `investmentSvc.DeleteTransaction`.
Add the missing transfer guard to `cli/transaction/void.go`. Delete the phase-1
differential test.

**Re-point the register's cleared toggle — do not skip this.** `register_view.go`
toggles cleared on *any* row, transfer legs included, by loading the row and
calling `undo.NewEditTransactionCommand` → `transaction.Service.Update`
(`register_view.go:246-262`, `undo/transaction.go:84`). Phase 5 adds an
`*IsTransferError` refusal to `Update`, so unless this moves to `SetLegStatus`
here, Space-on-a-transfer-leg starts erroring the moment phase 5 lands — a
user-facing regression that no phase-5 exit criterion would catch. Moving it in
phase 3 is also the only ordering that keeps each phase green on its own. (The
alternative — exempting status-only updates from the refusal — is rejected: it
reintroduces a second write path to a transfer leg, which is the thing this
design exists to remove.)

**Undo test churn lands here, with the commands it tests.** Deleting
`undo/investment_transfer.go` and the four transfer commands at
`undo/transaction.go:207-443` orphans `undo/investment_transfer_test.go` (deleted
here, not in phase 5), and requires rewriting the transfer-command references in
`undo/transaction_test.go` and `undo/transfer_category_test.go`. Left to phase 5
these leave phase 3's tree red.

**Exit criteria.** Behavioural changes, each with a new test:

- Editing a reconciled investment-involving transfer is refused (today: silently
  succeeds — there is no reconciled guard anywhere in `internal/investment`).
- Editing a split transfer-line is refused from the TUI (today: the bank side
  errors with "expected 2 transactions", the investment side orphans the split
  line and double-counts).
- After an investment-involving edit the cursor lands on the leg that still
  exists (today the TUI stashes an ID `update_edit.go:604` just deleted).
- Ctrl+Z undoes an investment transfer edit and delete.
- `tmoney transaction void <bank leg of an inv↔reg transfer>` returns
  `*VoidNotSupportedError`, not `"expected 2 transactions for transfer, found 1"`.
- Space on a transfer leg in the register still toggles cleared, now via
  `SetLegStatus`, and is still a single undo step. Assert it for a bank↔bank leg
  **and** the bank leg of an inv↔reg transfer, so the phase-5 `Update` refusal
  cannot silently break it.
- `TestUpdateTransferCash_InvToInv_FlipDirection`
  (`investment_service_test.go:3686`) is **retargeted at `Reverse`**, not retired.

Existing suites that must pass: `cli/transfer/add_test.go` (20),
`edit_test.go` (7), `delete_test.go` (7), `category_cmd_test.go` (13),
`tui/transfer_dialog_test.go` (49), `transfer_dialog_category_test.go` (11),
`undo/transfer_category_test.go` (2), `tests/integration/transfer_test.go`.

Expect edits to ~15 assertions where guard unification changed the error source:
the same-account and non-positive-amount messages become uniform across all four
kinds, and inv↔reg system-category input is newly rejected.

### Phase 4 — domain composers (risk: medium)

The `InTx` phase, split out of phase 3 so the blast radius is bounded to
non-user-facing call sites. Point `scheduled.postSingleLineTransfer` (`:679`) and
`AutoPost`'s transfer branch (`:355`) at `transferSvc.InTx(tx).Create`. Re-point
`undo/scheduled_transaction.go:284`/`:293` and `undo/auto_post.go:53`. Point
`transferlink.linkOne` at `LinkExisting`. Delete `scheduled`'s
`validateTransferCategory` copy (`:1050`).

**One return-shape adjustment.** `postSingleLineTransfer` returns
`pair.FromTransaction` — a `*transaction.Transaction` that feeds
`PostResult.Transactions` and the undo command (`scheduled_service.go:690`).
`transfer.Result` carries `LegRef`s, not rows, so the posting path re-reads the
From leg via `txnRepo.GetByID(res.From.RowID)` inside the same tx. Cheap and
already tx-bound, but it is a real call-site change, not a drop-in swap.

**Exit criteria.** A scheduled single-line transfer whose destination is an
investment account now **posts** instead of failing. Today it can be created
(`scheduled_repository.go` only checks the account exists) but never posts:
`CreateTransfer` → `rejectInvestmentAccount` → `NotRegularAccountError`, which in
`AutoPost` is neither a `ClosedAccountError` nor a loan error and therefore
**aborts the entire batch** at `scheduled_service.go:446-448`. New test.
`scheduled/scheduled_transfer_test.go`, `closed_account_guard_test.go`,
`transferlink/transferlink_category_test.go` (5, including the rollback-leaves-
candidates-unmutated case at `:189`) all pass.

### Phase 5 — delete the old surface; sever the import edge (risk: medium)

Delete from `transaction`: `CreateTransfer`, `UpdateTransfer`,
`UpdateTransferAmount`, `UpdateTransferDate`, `UpdateTransferStatus`,
`DeleteTransfer`, `voidTransfer`, `RestoreVoidedTransfer`, `RecreateTransferPair`,
`GetTransferPair`, `GetTransferCounterpart`, `checkTransferEditable`,
`guardTransferDate`, `Service.IsTransfer`, `GetBalanceImpact`, the
`TransferPair` family, `transfer_repository.go`, and `Delete`'s legacy two-leg
branch. Add `*IsTransferError` refusals to `Update`, `Delete` and
`VoidTransaction` for whole-transaction transfer legs, **gated on the existing
`splitRepo.GetByTransferID` probe** so transfer-LINE counterparts keep their
reverse-cascade path (`:299-307`, `:233-243`). Introduce `resolveCounterpart` and
rewrite the four ladder call sites on top of it, preserving the `amountChanged`
asymmetry.

Delete from `investment`: the three create paths, `UpdateTransferCash`,
`applyInvestmentStatus`/`applyRegularStatus`, both result structs, the `txnRepo`
field and constructor param, `DeleteTransaction`'s `transfer_cash` cascade
(replaced by a typed refusal), `cash_position.go`. Reshape the four port methods
to take `db.Queryer`; delete `CounterpartInTx` and
`transaction.SetInvestmentCounterpart`; move port injection into
`transaction.NewService` and invert the construction order in `registry.go`.
Remove the `internal/transaction` import from `investment_service.go` and
`update_edit.go`.

**Exit criteria.** `go list -deps ./internal/investment | grep internal/transaction`
returns nothing. `grep -rn 'InvestmentCashCounterpartAdapter\|CounterpartInTx\|SetInvestmentCounterpart'`
returns nothing. All 33 `internal/investment` test files remain `package
investment` and still compile (verified: `investment_service_test.go`'s 22
`transaction.` references are the two helper wirings at `:28`/`:76` plus twenty
`UpdateTransferCash` lines at `:3472-3934`, all deleted here;
`cash_position_test.go` goes with its subject; `transfer_category_test.go` moves
to the transfer suite; `split_counterpart_test.go` is unaffected because the edge
is severed, not flipped). The fake in
`transaction/split_investment_test.go:18-129` is reshaped to the new port
signature. Paycheck→401k create/edit/target-move/parent-delete/parent-void round
trips unchanged (12 tests).

`app.Services.TransferRepo` (`registry.go:40`) is removed with
`transfer_repository.go`. Its only consumer outside `internal/transaction` is
`transferlink`, repointed at `LinkExisting` in phase 4 — so by the time this
phase runs the field has no callers.

**Test deletions:** `transfer_repository_test.go` (997),
`cash_position_test.go` (410), and the
~15 blocks pinning the zero-production-caller surface
(`transaction_service_test.go:1470/1502/1714/2514/2584/2615`,
`transfer_repository_test.go:461/491/621/651/697/765/842`,
`tests/integration/transfer_test.go:184/229/249/268`,
`closed_account_guard_test.go:122`).

### Phase 6 — error unification and sweep (risk: low)

Delete `account.IsClosedError` (repoint `reconciliation_service.go:94`),
`investment.InvalidTransferAmountError`, `investment.NotRegularAccountError`,
`investment.InsufficientCashError`. Add `internal/transfer/arch_test.go`: a grep
guard over non-test `.go` files outside `internal/transfer` for
`CreateTransfer(|UpdateTransfer(|DeleteTransfer(|TransferCash(|DepositFromAccount(|TransferCashBetweenInvestments(|UpdateTransferCash(`,
with **no allowlist**. Verified against the tree: 22 non-test call sites today —
19 outside the defining files (`cli/transfer` 7, `undo` 9, `scheduled` 2,
`tui/transfer_dialog.go` 1) plus 3 internal `UpdateTransferCash` self-calls in
`update_edit.go` that go with it. All are migrated by phase 4 or deleted in
phase 5, so the guard starts life with nothing to exempt.
Update `specs/database.md` and `specs/transactions.md`, both already
documented as stale — the latter still lists `transfer_in`/`transfer_out`
transaction types that do not exist in the DB CHECK, and still says `pending` for
the regular status enum.

**Exit criteria.** The arch test passes, and fails when a direct call is
reintroduced. `go vet` clean; no unused symbols in `internal/transaction` or
`internal/tui`.

### Phase 7 — migration 032: `investment_transactions.category_id` (risk: medium) — **STANDALONE, OPTIONAL, SEPARATE GO/NO-GO**

This is the only schema change in the plan and it is deliberately last, alone,
and separately decided.

```sql
-- Migration 032: category label on investment transactions.
--
-- investment_transactions has no category column. That single absence is the
-- last SCHEMA-level asymmetry in transfer handling and it costs: an inv↔inv
-- transfer must reject a category outright (a rule with five presentation
-- guards and, before this design, zero domain backstop); an inv↔reg transfer's
-- category can live only on the regular leg; and both front ends must re-read
-- that leg before an edit or the label is silently wiped.
--
-- A plain nullable UUID with NO foreign key and NO index. This matches the
-- established treatment of transfer_account_id / transfer_id on
-- transaction_splits (migration 026) and scheduled_transactions (022): an FK or
-- index here would require the full backup-drop-recreate dance, because
-- investment_transaction_lots holds an FK to investment_transactions(id)
-- (migrations 008:81, 010:95) and would have to be rebuilt too. Referential
-- integrity for this column is a service-layer responsibility, exactly as it is
-- for the transfer columns.
--
-- ALTER TABLE ADD COLUMN is legal in DuckDB for a column with no constraint.
ALTER TABLE investment_transactions ADD COLUMN category_id UUID;
```

Plus `CurrentSchemaVersion` 31 → 32 (`internal/db/migration.go:16`).

Then: add `CategoryID types.NullableID` to `investment.Transaction` and plumb it
through the repository SQL with `dbutil.NullID` / `dbutil.NullUUIDCast`; replace
the `CAST(NULL AS UUID)` literal in `legSelect` with the real column; make
`planLegs` carry the category onto **both** legs for every kind; delete
`Kind.SupportsCategory`, `*CategoryNotSupportedError`, guard step 7, and the two
category-preservation hacks (`cli/transfer/resolve.go:192-203`,
`tui/transfer_dialog.go:450-467`); delete the `editTransferIncludesCategory`
variable-field-layout logic in the TUI (three independent copies of the same
layout decision: `buildEditTransferDialog:190-199`, `submitEditTransferDialog:851-859`,
`transferCategoryFieldIndex:251-259`).

**Required follow-on, in the same commit:**
`category_repository.go`'s dependent count before a delete
(verified at `:355-380`) counts only `transactions` and `transaction_splits`. It
gains a third `COUNT(*) FROM investment_transactions WHERE category_id = ?`, or a
category can be deleted out from under investment rows leaving dangling UUIDs.

**Reporting is unaffected.** Verified: `grep -rn investment_transactions
internal/report/ internal/account/ internal/reconciliation/ internal/imexport/`
returns nothing, and `category_spending` (migration 019:287) joins only
`transactions`. Investment rows are not in any spending report, so mirroring the
category onto them cannot leak into one.

**This is a one-way door and that is why it is isolated.** `db.Migrate()` runs
unprompted inside `db.Open` (`connection.go:61`) with no pre-migration backup,
and a downgraded binary hits `VersionMismatchError` (`db_errors.go:71-78`) and
cannot open the file at all. The data is safe — an additive nullable column
loses nothing — but the file becomes version-locked. Ship phases 1–6 first,
confirm them in real use, then decide.

**Exit criteria.** New `internal/db/migration_test.go` case asserting the column
exists and is nullable; `TestCurrentSchemaVersion` and
`TestMigrationFileIntegrity` updated. `TestMigration008InvestmentTables` needs
**no** change — it inserts with an explicit column list
(`migration_test.go:1257-1266`), so a new nullable column is invisible to it.
`internal/db/reindex_test.go` unaffected (no index added). New round-trip test:
an IRA→IRA rollover carries a category.

---

## 10. Testing strategy

- **The existing corpus is the regression net.** 364 top-level `Test*Transfer*`
  functions exist repo-wide. Phases 1–4 must pass them essentially unmodified;
  the deliberate exceptions are enumerated per phase above.
- **Differential tests bridge the two live implementations** (phase 1 for reads,
  phase 2 for writes) and are deleted when the old path goes.
- **Fault injection per verb per kind**, using the `Queryer` wrapper from
  `design-withtx.md` §8: `database.WithTx(tx => transferSvc.InTx(failWrap(tx)).Create(...))`,
  asserting both that the call errors and that a fresh read shows zero partial
  state. What this verifies is that **no write escaped the transaction** —
  nothing in the new package reaches `db.Conn()` directly.
- **Three assertions that would otherwise be silently wrong:**
  1. Every investment leg is written with `TransactionTypeTransferCash`
     (`GetCashBalance` sums only `AffectsCash()` rows — a wrong type makes money
     vanish from a balance with no error).
  2. `insertLeg` runs the ledger's own row validation
     (`investment.Transaction.Validate` includes the future-date rule for
     position-bearing types at `investment.go:460-465`; a bare `repo.Create`
     would skip it).
  3. A void→undo round trip on a reg↔reg transfer preserves direction (the
     RowID-addressed snapshot; §2.3).
- **Schema test for the UNION.** A case in `internal/db/migration_test.go`
  executes `legSelect` against a freshly migrated DB and asserts the column set,
  so the projection cannot drift from either table.

---

## 11. Estimated line delta

Production, non-test. Function extents measured from the tree; ±10%.

**Deleted**

| | Lines |
|---|---|
| `transaction/transfer_repository.go` (whole file) | 364 |
| `transaction/dispatch.go` (whole file; logic moves) | 46 |
| `transaction_service.go` whole-transaction transfer region (`:1244`–`:1330`, `:1341`–`:1560`, `:1709`–`:1860`, `Delete`'s legacy branch, `VoidTransaction`'s delegate) | 350 |
| `transaction.go` `TransferPair` + `NewTransferPair` + `Validate` + `IsValid` | 88 |
| `transaction_service.go` counterpart ladder 5→1 | 110 |
| `investment_service.go` transfer + result-struct + error region | 336 |
| `investment/update_edit.go` `UpdateTransferCash` + status helpers | 160 |
| `investment/cash_position.go` + `InsufficientCashError` | 67 |
| `undo/investment_transfer.go` (whole file) | 185 |
| `undo/transaction.go` four transfer commands | 236 |
| `cli/transfer/resolve.go` (268 → 40) | 228 |
| `cli/transfer/add.go` dispatch + result struct + 2 guards | 54 |
| `cli/transfer/edit.go` dispatch + 2 guards + status mapper | 105 |
| `cli/transfer/delete.go` dispatch + guard | 25 |
| `tui/transfer_dialog.go` (999 → 570) | 429 |
| `scheduled_service.go` `validateTransferCategory` copy | 17 |
| `transferlink.go` `linkOne` body | 22 |
| `app/registry.go` setter + `transferRepo` wiring | 6 |
| **Total deleted** | **2,828** |

**Added**

| | Lines |
|---|---|
| `internal/transfer/` — `kind.go` 70, `transfer.go` 150, `errors.go` 110, `service.go` 90, `guards.go` 130, `rows.go` 190, `read.go` 160, `create.go` 100, `write.go` 240 | 1,240 |
| `internal/undo/transfer.go` (4 commands) | 170 |
| `transfer/status.go` (mapping moved out of `investment` + `UnrepresentableStatusError`) | 45 |
| `investment/investment_repository.go` narrow `UpdateStatus` | 22 |
| `investment` counterpart port reshape (4 methods) | 20 |
| `transaction.ValidateTransferCategoryByID` free function | 25 |
| `transaction` refusal + re-pointed backstop guards | 30 |
| `investment` refusal guards (`DeleteTransaction`, `SetClearedStatus`) | 18 |
| `app/registry.go` transfer wiring + reorder | 14 |
| `cli/transfer` re-wiring | 60 |
| `tui` re-wiring | 90 |
| Phase 7: migration 032 + version bump | 35 |
| Phase 7: `investment` `category_id` plumbing | 25 |
| Phase 7: `category_repository` third dependent count | 12 |
| **Total added** | **1,806** |

**Net production: 1,806 − 2,828 = −1,022 lines.**

Tests, rougher. Deleted: `transfer_repository_test.go` 997,
`cash_position_test.go` 410, `undo/investment_transfer_test.go` 230,
`cli/transfer/resolve_test.go` 184, the fake adapter in
`split_investment_test.go` ~130, ~15 dead-surface blocks ~400, trimmed TUI dialog
tests ~250 ⇒ **~2,600**. Added: the transfer matrix, guard and fault-injection
suites ⇒ **~1,500**. **Net tests ≈ −1,100.**

**Net total ≈ −2,100 lines.**

The number that matters more than the line count:

| Concept | Sites before | Sites after |
|---|---|---|
| 4-way create dispatch | 5 hand-written switches + 3 near-clone implementations | 1 `planLegs`, no switch |
| Cross-table leg resolution | 3 | 1 |
| Reconciled-transfer refusal | 6 | 1 |
| inv↔inv category refusal | 5 | 1 (0 after phase 7) |
| Orientation from amount sign | 5 | 1 |
| investment↔regular status mapping | 3 | 1 |
| `direction` string derivation | 2 | 0 (`Reverse`) |
| Dual-table counterpart lookup | 5 | 1 |
| Duplicate error types | 4 | 0 |
| Two-leg result structs | 3 | 1 |
| Transfer view models | 2 | 1 |
| Undo command types | 7 | 4 |

---

## 12. What this deliberately does not do

### 12.1 The single-ledger question — verdict: NO

The tempting move is to merge `investment_transactions` into `transactions`. It
would genuinely kill the root cause rather than contain it: `TransferPair` would
work for all four combinations, `GetByTransferID` would stop being structurally
blind, the two status enums would unify, the category asymmetry would evaporate,
and the counterpart port would die outright, because a split transfer-line's
counterpart into a 401k is a pure cash row with no security data that
`transaction.Repository` could write directly.

**Rejected, on cost and blast radius.**

- **34 `FROM transactions` query sites across 7 packages** (`transaction` 20,
  `account` 4, `reconciliation` 3, `payee` 3, `report` 2, `scheduled` 1,
  `category` 1) silently change meaning the moment investment rows land in that
  table — plus `imexport`, which issues no raw SQL of its own but inherits the
  change through the transaction repository.
  `account.Repository.Balance`/`BalanceAsOf` (`:175-221`) and the
  `account_balances` view would start summing brokerage buys into a bank-style
  balance. `report.netWorthAsOf` — which already falls back to the SQL balance
  when the investment valuer errors (`report_service.go:128-134`) — would change
  what that fallback means. `reconciliation.GetCandidateTransactions` would start
  returning investment rows. `imexport` would start exporting buys as plain
  transactions. Each needs a new predicate, and a missed one is a **wrong number
  on a real personal-finance database with no obvious symptom**.
- `investment_transaction_lots` holds an FK to `investment_transactions(id)`
  (migrations 008:81, 010:95) and would need a backup-drop-recreate repoint.
- `transactions` would grow `transaction_type`, `security_id`, `shares`,
  `price_per_share`, `commission` and become single-table inheritance with
  type-conditional domain validation.
- `transaction.Transaction.Validate`'s "amount cannot be zero unless void"
  (`transaction.go:321-323`) would have to be relaxed, because a zero-cost-basis
  share transfer is legal.
- The status enum needs a data conversion (`uncleared` vs `pending`, and
  `investment_transactions` has no `void`).

That is weeks of work with a real chance of silent numeric corruption, to solve a
problem a package boundary solves. **A dual ledger is a fact about the data model;
a four-path write graph is a fact about the code. This design fixes the second and
leaves the first.** Recorded here so the question is not relitigated from scratch.

### 12.2 Everything else out of scope

- **Split transfer-LINES stay in `transaction.Service`** (§3). Two places still
  write an `investment_transactions` transfer row: `transfer.Service` (whole
  transfers) and `transaction.Service.createTransferLineCounterpart` (split
  lines). Ownership is singular for whole transfers, not for the `transfer_id`
  column.
- **Counterpart date drift is not fixed.** `mirrorToPairedCounterpart` syncs
  amount and category but never the date, so under `UpdateWithSplits` an ADDED
  line's counterpart gets the new date while RETAINED lines keep the old one —
  one save, two counterparts on different dates. Counterpart memo is still always
  empty (`createTransferLineCounterpart:561` passes `memo=""`) and counterpart
  status still never cascades.
- **`planSplitReplacement`'s greedy target-account fallback match** is preserved
  exactly. Two transfer lines to the same target can have their counterparts
  swapped by reordering, and because the TUI split dialog always omits
  `transfer_id` (`split_dialog.go:462-475`) the fallback is the normal path, not
  the exception. Deliberate: 12 tests pin it and it is the most dangerous code in
  the area.
- **Deleting an investment counterpart from the investment register still
  orphans the split line.** `investment.Service` has no `splitRepo`, so the
  parent→counterpart cascade works and counterpart→parent does not. This design
  turns the whole-transfer case into a typed refusal; the split-line case is
  unchanged.
- **Share transfers** (§4), including the TUI bug where a NEW share transfer out
  of a lot-tracked account always submits with `lotAllocations == nil` and is
  rejected (`fifoLotAllocations` exists at `investment_service.go:1038` but is
  wired only into `FeeLiquidation`).
- **No unified void.** Investment-involving transfers still cannot be voided.
  Closing that (Future-H) needs a migration adding `void` to the
  `investment_transactions` status CHECK, plus position/lot reversal and
  total-return exclusion.
- **Reconciliation stays transfer-unaware.** It reconciles one leg at a time and
  never reads `investment_transactions`, so half-reconciled transfers remain
  reachable and still block editing the whole pair. Investment legs remain
  unreconcilable.
- **`transferlink` matching is untouched.** Still O(n²) in memory
  (`transferlink.go:119-145`) with one `splitRepo.CountByTransaction` round trip
  per transaction (`:260`), still commits per candidate rather than per run,
  still excludes investment accounts (Future-I). `LinkExisting` changes who
  performs the write, not what is matched.
- **Import/export still cannot round-trip a transfer.** `ParseCSV` (`csv.go:163`)
  and `ParseQIF` (`qif.go:206-211`) populate `ImportRecord.TransferAccount` and
  `ImportService.buildTransaction` (`:393-450`) never reads it. Export emits a
  bank↔bank transfer as two independent rows (double-counted on re-import), an
  inv↔reg transfer as one, an inv↔inv transfer as none. QIF export still destroys
  a categorized transfer's category by overwriting the L field with the bracket
  form (`qif.go:334-338`).
- **The five divergent CLI category resolvers** stay divergent:
  `--category "Groceries"` (a subcategory) still works on `transaction edit`
  (bare-name scan, `transaction/edit.go:250-258`) and still fails on
  `transfer edit` (exact `Parent[:Child]` path, `transfer/category.go:31-51`).
  Only the transfer-side *validation* is unified.
- **God files** (review item 3). `transaction_service.go` 2,009 → ~1,550;
  `investment_service.go` 1,954 → ~1,620. Both still too big.
- **TUI presentation bugs** orthogonal to this: the mouse-cancel state leaks
  (`app_mouse.go:373-374` nils only `a.transferDialog`, leaving three companion
  fields populated; `:559-560` the same for the shares dialog), the three
  divergent post-create-category focus behaviours, and the investment register's
  missing `t` key.

---

## 13. Open questions

1. **Should `undo.PostScheduledTransferCommand` become atomic?**
   `Execute` posts via `PostWithDate` (creating the pair inside the schedule's
   tx) and then makes a **separate** `UpdateTransfer` call at
   `scheduled_transaction.go:284` to apply the preview's memo/status/category. If
   the second call fails, the pair exists with template values and the schedule
   has already advanced. Making it atomic means threading the preview's overrides
   into `PostWithDate` so one `runInTx` covers both.
   **Recommendation:** fix it, but as a separate change after phase 4. It is a
   pre-existing `scheduled` defect, not a transfer-ownership one, and folding it
   into an already-high-risk phase buys nothing.

2. **Should `transfer.Service.Update` be allowed to change accounts?**
   No front end can request it today and `Edit` deliberately cannot express it.
   But `UpdateTransferCash`'s signature accepts new account IDs, so the
   *capability* exists in the service layer even though it is UI-unreachable.
   **Recommendation:** keep accounts immutable. Delete + Create is correct,
   observable, and undoable; a silent re-account would have to reason about
   opening dates and closed accounts on four accounts at once, which is exactly
   the complexity `UpdateTransferShares` already carries and gets wrong
   (`update_edit.go:733` dereferences `srcOld.TransferAccountID.ID` having only
   checked `TransferID.Valid`). If a real need appears, add an explicit
   `Reaccount(transferID, from, to)` verb rather than widening `Edit`.

3. **Ship phase 7 at all?** It is the highest concept-per-line item in the plan —
   it deletes a predicate, an error type, a guard step, two front-end
   category-preservation hacks and three copies of a variable-field-layout
   decision — but it is also the only irreversible step, applied automatically on
   first open with no backup and no downgrade path.
   **Recommendation:** ship phases 1–6, live on them for a release, then decide.
   The design is complete and correct without phase 7; phase 7 only makes the
   category rule uniform instead of kind-dependent.
