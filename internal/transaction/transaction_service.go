package transaction

import (
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/types"
)

// This file holds the Service type itself: its dependencies, its constructor, and
// the join-if-bound transaction triad (InTx / q / runInTx) every other file in the
// package relies on. The behavior is spread across sibling service_*.go files;
// they are all methods on this one type, which the package deliberately does not
// split (see specs/design-service-decomposition.md section 3 for why).

// InvestmentCounterpartPort is how transaction.Service mints, finds, deletes and
// amends the investment_transactions row that is the counterpart of a transfer
// LINE inside a multi-line split (e.g. a paycheck → 401k contribution line, an
// auto-deposit to a brokerage).
//
// Whole-transaction transfers do NOT come through here — internal/transfer owns
// those and writes both ledgers directly. This port exists only because the
// split lifecycle stays in this package, and a split's counterpart must commit
// in the same transaction as the split row itself.
//
// It replaces InvestmentCashCounterpartAdapter. Two changes:
//
//  1. The transaction is an explicit db.Queryer PARAMETER, not a bound-copy
//     return. The old CounterpartInTx had to name
//     transaction.InvestmentCashCounterpartAdapter as its return type, and that
//     single reference is what forced internal/investment to import
//     internal/transaction. Passing the Queryer per call removes the last
//     cross-package type reference — and with it the CounterpartInTx naming
//     hack, which existed only because investment.Service already had an
//     InTx(tx) *Service and Go forbids two methods sharing a name.
//  2. It is injected at construction, not set afterwards. Once
//     investment.NewService no longer takes a *transaction.Repository, the
//     construction order inverts freely: build investmentSvc, then txnSvc with
//     the port. SetInvestmentCounterpart is gone.
//
// db.Queryer remains the only vocabulary shared across the boundary, which is
// exactly why it lives in internal/db.
//
// A nil port means transfer LINES targeting an investment account are refused
// (ensureTransferTargetRoutable) rather than written as a malformed regular row.
type InvestmentCounterpartPort interface {
	// CreateCounterpart mints the investment-side row on q. amount carries the
	// sign in the destination frame (positive = cash arriving, negative = cash
	// leaving); the caller provides the shared transferID. Returns the new row's
	// ID.
	CreateCounterpart(
		q db.Queryer,
		invAcctID, otherAcctID types.ID,
		date types.Date,
		amount types.Money,
		memo string,
		transferID types.ID,
	) (types.ID, error)

	// FindCounterpart returns the investment row linked to transferID.
	// found=false means no investment-side row exists for it (the counterpart
	// may be on the regular table, or none was ever minted).
	FindCounterpart(q db.Queryer, transferID types.ID) (rowID types.ID, reconciled bool, found bool, err error)

	// DeleteCounterpart removes the investment row by ID.
	DeleteCounterpart(q db.Queryer, rowID types.ID) error

	// UpdateCounterpartAmount mirrors a transfer-line amount edit onto the
	// investment row. newAmount is in the destination frame.
	UpdateCounterpartAmount(q db.Queryer, rowID types.ID, newAmount types.Money) error
}

// Service provides business logic for transaction operations.
type Service struct {
	txnRepo               *Repository
	splitRepo             *SplitRepository
	payeeRepo             *payee.Repository
	accountRepo           *account.Repository
	investmentCounterpart InvestmentCounterpartPort
	db                    *db.DB
	tx                    db.Queryer // nil outside a transaction
}

// NewService creates a new Service.
func NewService(
	txnRepo *Repository,
	splitRepo *SplitRepository,
	payeeRepo *payee.Repository,
	accountRepo *account.Repository,
	investmentCounterpart InvestmentCounterpartPort,
	database *db.DB,
) *Service {
	return &Service{
		txnRepo:               txnRepo,
		splitRepo:             splitRepo,
		payeeRepo:             payeeRepo,
		accountRepo:           accountRepo,
		investmentCounterpart: investmentCounterpart,
		db:                    database,
	}
}

// InTx returns a copy of the service bound to tx, with every repository field
// and the investment-counterpart adapter rebound to the same transaction so all
// writes join one atomic unit. The original service is unchanged and remains
// safe for non-transactional use.
func (s *Service) InTx(tx db.Queryer) *Service {
	c := *s
	c.tx = tx
	c.txnRepo = s.txnRepo.WithTx(tx)
	c.splitRepo = s.splitRepo.WithTx(tx)
	if s.payeeRepo != nil {
		c.payeeRepo = s.payeeRepo.WithTx(tx)
	}
	if s.accountRepo != nil {
		c.accountRepo = s.accountRepo.WithTx(tx)
	}
	return &c
}

// q returns the active Queryer for ad-hoc service-level SQL: the bound
// transaction if any, else the live connection. A bound service must not
// query the pool directly — a pool read would miss the transaction's own
// uncommitted writes, and a pool write would escape its atomicity.
func (s *Service) q() db.Queryer {
	if s.tx != nil {
		return s.tx
	}
	return s.db.Conn()
}

// runInTx begins a new transaction if the service is unbound, or joins the
// already-bound transaction. This is what makes service methods composable
// without savepoints: an outer flow binds once, inner calls join. A bound
// service must never reach db.WithTx (nesting would deadlock the mutex).
func (s *Service) runInTx(fn func(b *Service) error) error {
	if s.tx != nil {
		return fn(s) // already bound — join the caller's tx
	}
	return s.db.WithTx(func(tx db.Queryer) error {
		return fn(s.InTx(tx))
	})
}
