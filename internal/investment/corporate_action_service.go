package investment

import (
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// This file holds the CorporateActionService type: its repositories, its
// constructor, the join-if-bound transaction triad, the plain list reads, and the
// one predicate that decides whether an account tracks a security by lot.
//
// The four event families live in sibling corporate_action_*.go files. The type
// has no dependency on investment.Service in either direction — they share six
// repositories and observe each other only through the database.

// CorporateActionService provides business logic for corporate action operations.
type CorporateActionService struct {
	caRepo       *CorporateActionRepository
	lotRepo      *LotRepository
	positionRepo *PositionRepository
	priceRepo    *price.Repository
	invRepo      *Repository
	secRepo      *security.Repository
	db           *db.DB
	tx           db.Queryer // nil outside a transaction
}

// NewCorporateActionService creates a new CorporateActionService.
func NewCorporateActionService(
	caRepo *CorporateActionRepository,
	lotRepo *LotRepository,
	positionRepo *PositionRepository,
	priceRepo *price.Repository,
	invRepo *Repository,
	secRepo *security.Repository,
	database *db.DB,
) *CorporateActionService {
	return &CorporateActionService{
		caRepo:       caRepo,
		lotRepo:      lotRepo,
		positionRepo: positionRepo,
		priceRepo:    priceRepo,
		invRepo:      invRepo,
		secRepo:      secRepo,
		db:           database,
	}
}

// InTx returns a copy of the service bound to tx, with every repository field
// rebound to the same transaction so an action's whole write-set joins one
// atomic unit. The original service is unchanged and remains safe for
// non-transactional use.
func (s *CorporateActionService) InTx(tx db.Queryer) *CorporateActionService {
	c := *s
	c.tx = tx
	c.caRepo = s.caRepo.WithTx(tx)
	c.lotRepo = s.lotRepo.WithTx(tx)
	c.positionRepo = s.positionRepo.WithTx(tx)
	c.priceRepo = s.priceRepo.WithTx(tx)
	c.invRepo = s.invRepo.WithTx(tx)
	c.secRepo = s.secRepo.WithTx(tx)
	return &c
}

// runInTx begins a new transaction if the service is unbound, or joins the
// already-bound transaction. A corporate action's full write-set — the biggest
// in the app (loops over lots/positions/prices plus minted rows) — commits once
// or not at all.
func (s *CorporateActionService) runInTx(fn func(b *CorporateActionService) error) error {
	if s.tx != nil {
		return fn(s) // already bound — join the caller's tx
	}
	return s.db.WithTx(func(tx db.Queryer) error {
		return fn(s.InTx(tx))
	})
}

// ListBySecurity retrieves all corporate actions for a security (as source or target).
func (s *CorporateActionService) ListBySecurity(securityID types.ID) ([]*CorporateAction, error) {
	return s.caRepo.ListBySecurity(securityID)
}

// ListAll retrieves every corporate action in the database.
func (s *CorporateActionService) ListAll() ([]*CorporateAction, error) {
	return s.caRepo.ListAll()
}

// accountUsesLotsFor reports whether the account holds (or held) the security
// via lots — i.e. it is lot-tracked for that security, so its lots were already
// processed by the lot path. Share-creating corporate actions (merger, spin-off)
// must skip such accounts in their position path to avoid double-counting,
// since for a lot-tracked account the position is a redundant aggregate of the
// lots.
func (s *CorporateActionService) accountUsesLotsFor(accountID, securityID types.ID) (bool, error) {
	lots, err := s.lotRepo.ListByAccountAndSecurity(accountID, securityID, true)
	if err != nil {
		return false, err
	}
	return len(lots) > 0, nil
}
