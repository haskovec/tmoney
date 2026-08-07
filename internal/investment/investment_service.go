package investment

import (
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

// This file holds the Service type: its repositories, its constructor, the
// transaction plumbing, the four guards every write runs first, and the one
// status setter.
//
// runInTx and healInOwnTx transitively need ALL ten fields, because healInOwnTx
// reaches the rebuild machinery. So any cluster that opens a transaction or heals
// must keep the whole struct — which is what decides who can become their own
// type later. See specs/design-service-decomposition.md section 6.1.

// Service provides business logic for investment transaction operations.
type Service struct {
	repo                *Repository
	accountRepo         *account.Repository
	positionRepo        *PositionRepository
	lotRepo             *LotRepository
	transactionLotRepo  *TransactionLotRepository
	priceRepo           *price.Repository
	corporateActionRepo *CorporateActionRepository
	holdingsRepo        *HoldingsRepository
	db                  *db.DB
	tx                  db.Queryer // nil outside a transaction
}

// NewService creates a new Service.
func NewService(
	repo *Repository,
	accountRepo *account.Repository,
	positionRepo *PositionRepository,
	lotRepo *LotRepository,
	transactionLotRepo *TransactionLotRepository,
	priceRepo *price.Repository,
	corporateActionRepo *CorporateActionRepository,
	database *db.DB,
) *Service {
	return &Service{
		repo:                repo,
		accountRepo:         accountRepo,
		positionRepo:        positionRepo,
		lotRepo:             lotRepo,
		transactionLotRepo:  transactionLotRepo,
		priceRepo:           priceRepo,
		corporateActionRepo: corporateActionRepo,
		holdingsRepo:        NewHoldingsRepository(database),
		db:                  database,
	}
}

// InTx returns a copy of the service bound to tx, with every repository field
// rebound to the same transaction so all writes join one atomic unit. The
// original service is unchanged and remains safe for non-transactional use.
//
// Optional repos (priceRepo, txnRepo, corporateActionRepo, holdingsRepo) are
// nil-guarded to match test fixtures that construct a partial service; the
// production wiring always sets them.
func (s *Service) InTx(tx db.Queryer) *Service {
	c := *s
	c.tx = tx
	c.repo = s.repo.WithTx(tx)
	c.accountRepo = s.accountRepo.WithTx(tx)
	c.positionRepo = s.positionRepo.WithTx(tx)
	c.lotRepo = s.lotRepo.WithTx(tx)
	c.transactionLotRepo = s.transactionLotRepo.WithTx(tx)
	if s.priceRepo != nil {
		c.priceRepo = s.priceRepo.WithTx(tx)
	}
	if s.corporateActionRepo != nil {
		c.corporateActionRepo = s.corporateActionRepo.WithTx(tx)
	}
	if s.holdingsRepo != nil {
		c.holdingsRepo = s.holdingsRepo.WithTx(tx)
	}
	return &c
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

// healInOwnTx runs syncPositionAndLots for (accountID, securityID) in its own
// committed transaction, before a trade's transaction is opened. The heal is
// idempotent repair — committing it is desirable even when the trade then
// fails, and it keeps the trade tx small.
//
// When the service is already tx-bound (a trade invoked from update_edit's
// reverse/re-create flow), the re-heal is skipped: the outermost entry point
// healed before opening the main tx, and the in-tx state is being mutated
// deliberately.
func (s *Service) healInOwnTx(accountID, securityID types.ID) error {
	if s.tx != nil {
		return nil // bound — the outermost entry point already healed
	}
	return s.db.WithTx(func(tx db.Queryer) error {
		return s.InTx(tx).syncPositionAndLots(accountID, securityID)
	})
}

// The four guards below delegate to the package-level forms in guards.go, which
// CounterpartService shares. Each passes THIS service's account repository, so a
// guard called on a tx-bound service reads that transaction's view.

// requireInvestmentAccount verifies that the given account exists, is an
// investment account, and is not closed.
func (s *Service) requireInvestmentAccount(accountID types.ID) error {
	return requireOpenInvestmentAccount(s.accountRepo, accountID)
}

// ensureAccountOpen returns an AccountClosedError when the account is closed.
func (s *Service) ensureAccountOpen(accountID types.ID) error {
	return requireOpenAccount(s.accountRepo, accountID)
}

// getInvestmentAccount retrieves and validates that the account is an investment account.
func (s *Service) getInvestmentAccount(accountID types.ID) (*account.Account, error) {
	return loadInvestmentAccount(s.accountRepo, accountID)
}

// validateTransaction validates an investment transaction and returns any validation errors.
func (s *Service) validateTransaction(txn *Transaction) error {
	return validateInvestmentTransaction(s.accountRepo, txn)
}

// SetClearedStatus marks an investment transaction cleared or pending. It is the
// service-layer chokepoint for the register's cleared toggle, so the
// closed-account freeze gate applies here (a closed account is frozen).
func (s *Service) SetClearedStatus(txnID types.ID, cleared bool) error {
	txn, err := s.repo.GetByID(txnID)
	if err != nil {
		return err
	}
	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}
	if cleared {
		txn.Clear()
	} else {
		txn.MarkPending()
	}
	return s.repo.Update(txn)
}
