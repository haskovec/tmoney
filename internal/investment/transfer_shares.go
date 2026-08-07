package investment

import (
	"fmt"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// Moving shares between two investment accounts, in kind.
//
// One method, because the operation is one atomic story: remove from the source
// (by lot or by position), add to the destination preserving cost basis, and
// leave both sides consistent or neither.

// ShareTransferResult contains both sides of a share transfer between two investment accounts.
type ShareTransferResult struct {
	SourceTransaction      *Transaction
	DestinationTransaction *Transaction
	TransferID             types.ID
}

// TransferShares transfers shares of a security between two investment accounts.
// The source position is reduced and the destination position is increased with the same cost basis.
// No cash movement occurs in either account.
// Both accounts must be investment accounts and must be different.
// For lot-tracking source accounts, lotAllocations specifies which lots to transfer from.
// For lot-tracking destination accounts, new lots are created preserving original purchase_date and cost_per_share.
func (s *Service) TransferShares(
	sourceAccountID, destAccountID types.ID,
	securityID types.ID,
	date types.Date,
	shares types.Quantity,
	memo string,
	lotAllocations []SellLotAllocation,
) (*ShareTransferResult, error) {
	if !shares.IsPositive() {
		return nil, fmt.Errorf("shares must be positive, got %s", shares)
	}

	if err := s.healInOwnTx(sourceAccountID, securityID); err != nil {
		return nil, err
	}
	if err := s.healInOwnTx(destAccountID, securityID); err != nil {
		return nil, err
	}

	// Validate both accounts are investment accounts
	srcAcct, err := s.getInvestmentAccount(sourceAccountID)
	if err != nil {
		return nil, err
	}

	dstAcct, err := s.getInvestmentAccount(destAccountID)
	if err != nil {
		return nil, err
	}

	// A share transfer is frozen if either leg is a closed account.
	if srcAcct.IsClosed() {
		return nil, &account.AccountClosedError{ID: sourceAccountID.String()}
	}
	if dstAcct.IsClosed() {
		return nil, &account.AccountClosedError{ID: destAccountID.String()}
	}

	// Reject same account
	if sourceAccountID == destAccountID {
		return nil, fmt.Errorf("cannot transfer shares between the same account")
	}

	// Determine cost basis based on source account type
	var totalCostBasis types.Money
	var costPerShare types.Money
	var srcLots []*Lot

	if srcAcct.TrackLots {
		// Lot-tracking source: validate lot allocations before any persistence
		if len(lotAllocations) == 0 {
			return nil, fmt.Errorf("lot allocations required for lot-tracking account")
		}

		totalAllocated := types.ZeroQuantity
		for _, alloc := range lotAllocations {
			totalAllocated = totalAllocated.Add(alloc.Shares)
		}
		if totalAllocated.Cmp(shares) != 0 {
			return nil, &LotAllocationMismatchError{
				Expected: shares,
				Actual:   totalAllocated,
			}
		}

		// Pre-validate all lots
		srcLots = make([]*Lot, len(lotAllocations))
		totalCostBasis = types.ZeroMoney
		for i, alloc := range lotAllocations {
			lot, err := s.lotRepo.GetByID(alloc.LotID)
			if err != nil {
				return nil, &LotNotFoundError{LotID: alloc.LotID.String()}
			}
			if lot.AccountID != sourceAccountID {
				return nil, &LotWrongAccountError{
					LotID:     alloc.LotID.String(),
					AccountID: sourceAccountID.String(),
				}
			}
			if lot.SecurityID != securityID {
				return nil, fmt.Errorf("lot %s is for a different security", alloc.LotID)
			}
			if lot.Closed {
				return nil, fmt.Errorf("lot %s is closed", alloc.LotID)
			}
			if lot.Shares.Cmp(alloc.Shares) < 0 {
				return nil, &LotInsufficientSharesError{
					LotID:     alloc.LotID.String(),
					Available: lot.Shares,
					Requested: alloc.Shares,
				}
			}
			srcLots[i] = lot
			totalCostBasis = totalCostBasis.Add(lot.CostPerShare.Mul(alloc.Shares.Decimal()))
		}
	} else {
		// Non-lot-tracking source: use position average cost
		srcPos, err := s.positionRepo.GetByAccountAndSecurity(sourceAccountID, securityID)
		if err != nil {
			return nil, fmt.Errorf("failed to get source position: %w", err)
		}

		if srcPos.Shares.Cmp(shares) < 0 {
			return nil, &InsufficientSharesError{
				SecurityID: securityID.String(),
				Available:  srcPos.Shares,
				Requested:  shares,
			}
		}

		costPerShare = srcPos.AverageCostPerShare
		totalCostBasis = costPerShare.Mul(shares.Decimal())
	}

	transferID := types.NewID()

	// Create source transaction (negative total — shares leaving)
	negTotal := totalCostBasis.Neg()
	srcTxn := NewTransactionWithSecurity(sourceAccountID, date, TransactionTypeTransferShares, negTotal, securityID, shares)
	srcTxn.SetTransfer(transferID, destAccountID)
	if memo != "" {
		srcTxn.SetMemo(memo)
	}

	if err := s.validateTransaction(srcTxn); err != nil {
		return nil, err
	}

	// Create destination transaction (positive total — shares arriving)
	dstTxn := NewTransactionWithSecurity(destAccountID, date, TransactionTypeTransferShares, totalCostBasis, securityID, shares)
	dstTxn.SetTransfer(transferID, sourceAccountID)
	if memo != "" {
		dstTxn.SetMemo(memo)
	}

	if err := s.validateTransaction(dstTxn); err != nil {
		return nil, err
	}

	// Persist both transactions and the lot/position changes on each side
	// atomically — either the whole transfer lands or none of it does.
	if err := s.runInTx(func(b *Service) error {
		if err := b.repo.Create(srcTxn); err != nil {
			return fmt.Errorf("failed to create source transfer transaction: %w", err)
		}

		if err := b.repo.Create(dstTxn); err != nil {
			return fmt.Errorf("failed to create destination transfer transaction: %w", err)
		}

		// Update source side
		if srcAcct.TrackLots {
			// Reduce source lots and create junction records
			for i, alloc := range lotAllocations {
				lot := srcLots[i]
				if err := lot.Reduce(alloc.Shares); err != nil {
					return fmt.Errorf("failed to reduce lot: %w", err)
				}
				if err := b.lotRepo.Update(lot); err != nil {
					return fmt.Errorf("failed to update lot: %w", err)
				}

				tl := NewTransactionLot(srcTxn.ID, lot.ID, alloc.Shares)
				if err := b.transactionLotRepo.Create(&tl); err != nil {
					return fmt.Errorf("failed to create transaction lot: %w", err)
				}
			}
		} else {
			// Non-lot-tracking source: reduce position
			srcPos, err := b.positionRepo.GetByAccountAndSecurity(sourceAccountID, securityID)
			if err != nil {
				return fmt.Errorf("failed to get source position: %w", err)
			}

			if err := srcPos.RemoveShares(shares); err != nil {
				return fmt.Errorf("failed to reduce source position: %w", err)
			}

			if srcPos.Shares.IsZero() {
				if err := b.positionRepo.Delete(sourceAccountID, securityID); err != nil {
					return fmt.Errorf("failed to delete zero source position: %w", err)
				}
			} else {
				if err := b.positionRepo.CreateOrUpdate(srcPos); err != nil {
					return fmt.Errorf("failed to save source position: %w", err)
				}
			}
		}

		// Update destination side
		if dstAcct.TrackLots {
			// Create new lots in destination preserving original purchase_date and cost_per_share
			for i, alloc := range lotAllocations {
				srcLot := srcLots[i]
				newLot := NewLot(destAccountID, securityID, alloc.Shares, srcLot.CostPerShare, srcLot.PurchaseDate, dstTxn.ID)
				if err := b.lotRepo.Create(&newLot); err != nil {
					return fmt.Errorf("failed to create destination lot: %w", err)
				}
			}
		} else {
			// Non-lot-tracking destination: update position
			dstPos, err := b.positionRepo.GetByAccountAndSecurity(destAccountID, securityID)
			if err != nil {
				return fmt.Errorf("failed to get destination position: %w", err)
			}

			if srcAcct.TrackLots {
				// Cost per share is the weighted average from lot allocations
				reciprocal := alpacadecimal.NewFromInt(1).Div(shares.Decimal())
				costPerShare = totalCostBasis.Mul(reciprocal)
			}

			if err := dstPos.AddShares(shares, costPerShare); err != nil {
				return fmt.Errorf("failed to update destination position: %w", err)
			}

			if err := b.positionRepo.CreateOrUpdate(dstPos); err != nil {
				return fmt.Errorf("failed to save destination position: %w", err)
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return &ShareTransferResult{
		SourceTransaction:      srcTxn,
		DestinationTransaction: dstTxn,
		TransferID:             transferID,
	}, nil
}
