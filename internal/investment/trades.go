package investment

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// Buying and selling shares.
//
// Sell has two shapes behind one entry point: a lot-tracked account allocates the
// sale across tax lots FIFO, a position-only account adjusts a single running
// position. Which one runs is a property of the account, not of the caller.

// Buy creates a buy transaction that purchases shares of a security.
// For non-lot-tracking accounts, it updates the aggregate position.
// For lot-tracking accounts, it creates a new lot.
// At least shares + one of (totalAmount, pricePerShare) must be provided.
func (s *Service) Buy(
	accountID types.ID,
	securityID types.ID,
	date types.Date,
	shares types.Quantity,
	totalAmount *types.Money,
	pricePerShare *types.Money,
	commission types.Money,
	memo string,
) (*Transaction, error) {
	acct, err := s.getInvestmentAccount(accountID)
	if err != nil {
		return nil, err
	}
	if acct.IsClosed() {
		return nil, &account.AccountClosedError{ID: accountID.String()}
	}

	// Heal any stale stored position/lot state for this (account, security)
	// before we read it (no-op when corporate actions are present). The heal
	// commits in its own tx before the trade tx; a bound service skips it.
	if err := s.healInOwnTx(accountID, securityID); err != nil {
		return nil, err
	}

	// Smart compute missing fields
	computed, err := SmartCompute(shares, totalAmount, pricePerShare, commission)
	if err != nil {
		return nil, fmt.Errorf("failed to compute buy fields: %w", err)
	}

	// Cash balance is allowed to go negative — bank statements often list
	// the day's sales after the day's buys, and we shouldn't require the
	// user to reorder same-date entries to get past a transient shortfall.

	// Create transaction with negative total (buy deducts cash)
	negTotal := computed.TotalAmount.Neg()
	txn := NewTransactionWithSecurity(accountID, date, TransactionTypeBuy, negTotal, securityID, shares)
	txn.SetPricePerShare(computed.PricePerShare)
	if !commission.IsZero() {
		txn.SetCommission(commission)
	}
	if memo != "" {
		txn.SetMemo(memo)
	}

	if err := s.validateTransaction(txn); err != nil {
		return nil, err
	}

	// Persist the row, the lot/position, and the auto-price atomically: a buy
	// either fully lands or not at all (no orphan transaction on a lot failure).
	if err := s.runInTx(func(b *Service) error {
		if err := b.repo.Create(txn); err != nil {
			return fmt.Errorf("failed to create buy transaction: %w", err)
		}

		// Update position or create lot based on account tracking mode
		if acct.TrackLots {
			lot := NewLot(accountID, securityID, shares, computed.PricePerShare, date, txn.ID)
			if err := b.lotRepo.Create(&lot); err != nil {
				return fmt.Errorf("failed to create lot: %w", err)
			}
		} else {
			pos, err := b.positionRepo.GetByAccountAndSecurity(accountID, securityID)
			if err != nil {
				return fmt.Errorf("failed to get position: %w", err)
			}
			if err := pos.AddShares(shares, computed.PricePerShare); err != nil {
				return fmt.Errorf("failed to update position: %w", err)
			}
			if err := b.positionRepo.CreateOrUpdate(pos); err != nil {
				return fmt.Errorf("failed to save position: %w", err)
			}
		}

		// Auto-create price record from transaction
		b.autoCreatePrice(securityID, date, computed.PricePerShare)
		return nil
	}); err != nil {
		return nil, err
	}

	return txn, nil
}

// SellLotAllocation specifies how many shares to sell from a specific lot.
type SellLotAllocation struct {
	LotID  types.ID
	Shares types.Quantity
}

// Sell creates a sell transaction that sells shares of a security.
// For non-lot-tracking accounts, it reduces the aggregate position.
// For lot-tracking accounts, lotAllocations specifies which lots to sell from.
// At least shares + one of (totalAmount, pricePerShare) must be provided.
func (s *Service) Sell(
	accountID types.ID,
	securityID types.ID,
	date types.Date,
	shares types.Quantity,
	totalAmount *types.Money,
	pricePerShare *types.Money,
	commission types.Money,
	memo string,
	lotAllocations []SellLotAllocation,
) (*Transaction, error) {
	acct, err := s.getInvestmentAccount(accountID)
	if err != nil {
		return nil, err
	}
	if acct.IsClosed() {
		return nil, &account.AccountClosedError{ID: accountID.String()}
	}

	// Heal any stale stored position/lot state before validating. The heal
	// commits in its own tx before the trade tx; a bound service skips it.
	if err := s.healInOwnTx(accountID, securityID); err != nil {
		return nil, err
	}

	// Smart compute missing fields
	computed, err := SmartCompute(shares, totalAmount, pricePerShare, commission)
	if err != nil {
		return nil, fmt.Errorf("failed to compute sell fields: %w", err)
	}

	// Create transaction with positive total (sell adds cash)
	txn := NewTransactionWithSecurity(accountID, date, TransactionTypeSell, computed.TotalAmount, securityID, shares)
	txn.SetPricePerShare(computed.PricePerShare)
	if !commission.IsZero() {
		txn.SetCommission(commission)
	}
	if memo != "" {
		txn.SetMemo(memo)
	}

	if err := s.validateTransaction(txn); err != nil {
		return nil, err
	}

	// Persist the row, the lot/position changes, and the auto-price atomically.
	if err := s.runInTx(func(b *Service) error {
		if acct.TrackLots {
			if err := b.sellWithLots(txn, accountID, securityID, shares, lotAllocations); err != nil {
				return err
			}
		} else {
			if err := b.sellWithPosition(txn, accountID, securityID, shares); err != nil {
				return err
			}
		}

		// Auto-create price record from transaction
		b.autoCreatePrice(securityID, date, computed.PricePerShare)
		return nil
	}); err != nil {
		return nil, err
	}

	return txn, nil
}

// sellWithPosition handles sell for non-lot-tracking accounts.
func (s *Service) sellWithPosition(txn *Transaction, accountID, securityID types.ID, shares types.Quantity) error {
	pos, err := s.positionRepo.GetByAccountAndSecurity(accountID, securityID)
	if err != nil {
		return fmt.Errorf("failed to get position: %w", err)
	}

	if pos.Shares.Cmp(shares) < 0 {
		return &InsufficientSharesError{
			SecurityID: securityID.String(),
			Available:  pos.Shares,
			Requested:  shares,
		}
	}

	if err := s.repo.Create(txn); err != nil {
		return fmt.Errorf("failed to create sell transaction: %w", err)
	}

	if err := pos.RemoveShares(shares); err != nil {
		return fmt.Errorf("failed to update position: %w", err)
	}

	if pos.Shares.IsZero() {
		if err := s.positionRepo.Delete(accountID, securityID); err != nil {
			return fmt.Errorf("failed to delete zero position: %w", err)
		}
	} else {
		if err := s.positionRepo.CreateOrUpdate(pos); err != nil {
			return fmt.Errorf("failed to save position: %w", err)
		}
	}

	return nil
}

// sellWithLots handles sell for lot-tracking accounts.
func (s *Service) sellWithLots(txn *Transaction, accountID, securityID types.ID, shares types.Quantity, lotAllocations []SellLotAllocation) error {
	if len(lotAllocations) == 0 {
		return fmt.Errorf("lot allocations required for lot-tracking account")
	}

	// Validate total shares across allocations equals sell shares
	totalAllocated := types.ZeroQuantity
	for _, alloc := range lotAllocations {
		totalAllocated = totalAllocated.Add(alloc.Shares)
	}
	if totalAllocated.Cmp(shares) != 0 {
		return &LotAllocationMismatchError{
			Expected: shares,
			Actual:   totalAllocated,
		}
	}

	// Validate each lot allocation before making any changes
	lots := make([]*Lot, len(lotAllocations))
	for i, alloc := range lotAllocations {
		lot, err := s.lotRepo.GetByID(alloc.LotID)
		if err != nil {
			return &LotNotFoundError{LotID: alloc.LotID.String()}
		}
		if lot.AccountID != accountID {
			return &LotWrongAccountError{
				LotID:     alloc.LotID.String(),
				AccountID: accountID.String(),
			}
		}
		if lot.SecurityID != securityID {
			return fmt.Errorf("lot %s is for a different security", alloc.LotID)
		}
		if lot.Closed {
			return fmt.Errorf("lot %s is closed", alloc.LotID)
		}
		if lot.Shares.Cmp(alloc.Shares) < 0 {
			return &LotInsufficientSharesError{
				LotID:     alloc.LotID.String(),
				Available: lot.Shares,
				Requested: alloc.Shares,
			}
		}
		lots[i] = lot
	}

	// All validations passed — persist the transaction
	if err := s.repo.Create(txn); err != nil {
		return fmt.Errorf("failed to create sell transaction: %w", err)
	}

	// Reduce lots and create junction records
	for i, alloc := range lotAllocations {
		lot := lots[i]
		if err := lot.Reduce(alloc.Shares); err != nil {
			return fmt.Errorf("failed to reduce lot: %w", err)
		}
		if err := s.lotRepo.Update(lot); err != nil {
			return fmt.Errorf("failed to update lot: %w", err)
		}

		tl := NewTransactionLot(txn.ID, lot.ID, alloc.Shares)
		if err := s.transactionLotRepo.Create(&tl); err != nil {
			return fmt.Errorf("failed to create transaction lot: %w", err)
		}
	}

	return nil
}

// fifoLotAllocations consumes `shares` across the given open lots oldest-first
// (the caller passes them in purchase-date order, which is FIFO) and returns
// the per-lot allocations. It is used to auto-allocate a fee liquidation on a
// lot-tracked account when the caller supplies no explicit allocation. It
// returns an *InsufficientSharesError when the open lots don't cover `shares`.
func fifoLotAllocations(securityID types.ID, openLots []*Lot, shares types.Quantity) ([]SellLotAllocation, error) {
	var allocations []SellLotAllocation
	remaining := shares
	for _, lot := range openLots {
		if !remaining.IsPositive() {
			break
		}
		take := lot.Shares
		if take.Cmp(remaining) > 0 {
			take = remaining
		}
		allocations = append(allocations, SellLotAllocation{LotID: lot.ID, Shares: take})
		remaining = remaining.Sub(take)
	}
	if remaining.IsPositive() {
		return nil, &InsufficientSharesError{
			SecurityID: securityID.String(),
			Available:  shares.Sub(remaining),
			Requested:  shares,
		}
	}
	return allocations, nil
}
