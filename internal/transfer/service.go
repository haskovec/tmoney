package transfer

import (
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
)

// Service owns whole-transaction cash transfers across both ledgers. It writes
// both legs directly through the two ledger repositories inside a single
// db.WithTx, so there is exactly one place a transfer is created, edited or
// deleted regardless of which pair of account types it connects.
type Service struct {
	txnRepo      *transaction.Repository
	invRepo      *investment.Repository
	splitRepo    *transaction.SplitRepository // read-only: Shape detection
	accountRepo  *account.Repository
	categoryRepo *category.Repository
	db           *db.DB

	tx db.Queryer // nil outside a transaction
}

// NewService wires the transfer owner. It takes repositories rather than the
// two ledger services: a transfer is two row writes plus guards, and going
// through the services would reintroduce the very dispatch this package exists
// to delete.
func NewService(
	txnRepo *transaction.Repository,
	invRepo *investment.Repository,
	splitRepo *transaction.SplitRepository,
	accountRepo *account.Repository,
	categoryRepo *category.Repository,
	database *db.DB,
) *Service {
	return &Service{
		txnRepo:      txnRepo,
		invRepo:      invRepo,
		splitRepo:    splitRepo,
		accountRepo:  accountRepo,
		categoryRepo: categoryRepo,
		db:           database,
	}
}

// InTx returns a copy of the service bound to tx, with every repository
// rebound. Follows specs/design-withtx.md §3 exactly.
//
// internal/scheduled needs this: it composes transfer creation into the same
// transaction that advances a schedule's next_date, which is what closes the
// double-post window.
func (s *Service) InTx(tx db.Queryer) *Service {
	c := *s
	c.tx = tx
	c.txnRepo = s.txnRepo.WithTx(tx)
	c.invRepo = s.invRepo.WithTx(tx)
	c.splitRepo = s.splitRepo.WithTx(tx)
	c.accountRepo = s.accountRepo.WithTx(tx)
	c.categoryRepo = s.categoryRepo.WithTx(tx)
	return &c
}

// q returns the active Queryer: the bound transaction if any, else the live
// connection. All ad-hoc SQL in this package goes through q().
func (s *Service) q() db.Queryer {
	if s.tx != nil {
		return s.tx
	}
	return s.db.Conn()
}

// runInTx begins a new transaction if the service is unbound, or joins the
// already-bound one. A bound service must never reach db.WithTx — DuckDB has
// no savepoints and db.WithTx holds a mutex, so nesting deadlocks.
func (s *Service) runInTx(fn func(b *Service) error) error {
	if s.tx != nil {
		return fn(s) // already bound — join the caller's tx
	}
	return s.db.WithTx(func(tx db.Queryer) error {
		return fn(s.InTx(tx))
	})
}
