package investment

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// Trades driven by income rather than by an order: a dividend reinvested into new
// shares, and shares liquidated to pay a fee.
//
// Both mirror the trades.go split — a lot-tracked path and a position-only path
// behind one entry point.

// ReinvestDividend creates a reinvest dividend transaction that adds shares without cash movement.
// For non-lot-tracking accounts, it updates the aggregate position.
// For lot-tracking accounts, it creates a new lot.
// At least shares + one of (totalAmount, pricePerShare) must be provided.
func (s *Service) ReinvestDividend(
	accountID types.ID,
	securityID types.ID,
	date types.Date,
	shares types.Quantity,
	totalAmount *types.Money,
	pricePerShare *types.Money,
	memo string,
) (*Transaction, error) {
	acct, err := s.getInvestmentAccount(accountID)
	if err != nil {
		return nil, err
	}
	if acct.IsClosed() {
		return nil, &account.AccountClosedError{ID: accountID.String()}
	}

	// Heal in its own committed tx before the trade tx; a bound service skips it.
	if err := s.healInOwnTx(accountID, securityID); err != nil {
		return nil, err
	}

	// Smart compute missing fields (no commission for reinvest)
	computed, err := SmartCompute(shares, totalAmount, pricePerShare, types.ZeroMoney)
	if err != nil {
		return nil, fmt.Errorf("failed to compute reinvest dividend fields: %w", err)
	}

	// Create transaction — no cash movement, so TotalAmount stored as positive for record-keeping
	txn := NewTransactionWithSecurity(accountID, date, TransactionTypeReinvestDividend, computed.TotalAmount, securityID, shares)
	txn.SetPricePerShare(computed.PricePerShare)
	if memo != "" {
		txn.SetMemo(memo)
	}

	if err := s.validateTransaction(txn); err != nil {
		return nil, err
	}

	// Persist the row and the lot/position atomically.
	if err := s.runInTx(func(b *Service) error {
		if err := b.repo.Create(txn); err != nil {
			return fmt.Errorf("failed to create reinvest dividend transaction: %w", err)
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
		return nil
	}); err != nil {
		return nil, err
	}

	// Reinvested dividends do NOT auto-create a price (see CreatesAutoPrice):
	// their per-share value is total_amount÷rounded-shares, unreliable for tiny
	// income events. The security is priced from buys/sells instead.
	return txn, nil
}

// FeeLiquidation creates a fee-via-liquidation transaction that sells shares to cover a fee.
// There is no net cash effect — the shares are sold and the proceeds pay the fee.
// For non-lot-tracking accounts, it reduces the aggregate position.
// For lot-tracking accounts, lotAllocations specifies which lots to sell from.
// At least shares + one of (totalAmount, pricePerShare) must be provided.
func (s *Service) FeeLiquidation(
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

	// Heal in its own committed tx before the trade tx; a bound service skips it.
	if err := s.healInOwnTx(accountID, securityID); err != nil {
		return nil, err
	}

	// Smart compute missing fields
	computed, err := SmartCompute(shares, totalAmount, pricePerShare, commission)
	if err != nil {
		return nil, fmt.Errorf("failed to compute fee liquidation fields: %w", err)
	}

	// Create transaction — TotalAmount stored as positive for record-keeping (no cash effect)
	txn := NewTransactionWithSecurity(accountID, date, TransactionTypeFeeLiquidation, computed.TotalAmount, securityID, shares)
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

	// On a lot-tracked account with no explicit allocation, auto-allocate FIFO
	// across the security's open lots. This is computed here (not in the caller)
	// so that UpdateFeeLiquidation — which reverses the old transaction's lot
	// effect *before* calling this method — allocates against the restored,
	// post-reverse lot state rather than a stale pre-reverse snapshot. Recurring
	// fee liquidations don't warrant per-lot tax selection, so FIFO (oldest
	// first) is the sensible default; pass explicit allocations to override.
	if acct.TrackLots && len(lotAllocations) == 0 {
		openLots, err := s.lotRepo.ListByAccountAndSecurity(accountID, securityID, false)
		if err != nil {
			return nil, fmt.Errorf("FeeLiquidation: load open lots for FIFO allocation: %w", err)
		}
		lotAllocations, err = fifoLotAllocations(securityID, openLots, shares)
		if err != nil {
			return nil, err
		}
	}

	// Persist the row and the lot/position changes atomically.
	if err := s.runInTx(func(b *Service) error {
		if acct.TrackLots {
			return b.feeLiquidationWithLots(txn, accountID, securityID, shares, lotAllocations)
		}
		return b.feeLiquidationWithPosition(txn, accountID, securityID, shares)
	}); err != nil {
		return nil, err
	}

	// Fee liquidations do NOT auto-create a price (see CreatesAutoPrice): the
	// per-share value is total_amount÷rounded-shares, unreliable for tiny fees.
	// The security is priced from buys/sells instead.
	return txn, nil
}

// feeLiquidationWithPosition handles fee liquidation for non-lot-tracking accounts.
func (s *Service) feeLiquidationWithPosition(txn *Transaction, accountID, securityID types.ID, shares types.Quantity) error {
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
		return fmt.Errorf("failed to create fee liquidation transaction: %w", err)
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

// feeLiquidationWithLots handles fee liquidation for lot-tracking accounts.
func (s *Service) feeLiquidationWithLots(txn *Transaction, accountID, securityID types.ID, shares types.Quantity, lotAllocations []SellLotAllocation) error {
	if len(lotAllocations) == 0 {
		return fmt.Errorf("lot allocations required for lot-tracking account")
	}

	// Validate total shares across allocations equals fee liquidation shares
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
		return fmt.Errorf("failed to create fee liquidation transaction: %w", err)
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
