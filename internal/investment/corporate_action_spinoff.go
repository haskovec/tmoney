package investment

import (
	"fmt"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

// Spin-offs: part of a parent holding's cost basis is allocated to a new security.
//
// The parent keeps its share count and loses a percentage of its basis; the
// spun-off security is created with the remainder, at the ratio and price the
// action records.

// SpinOff applies a corporate spin-off to a security.
// It allocates cost basis between the parent and spin-off security, creates new lots/positions
// for the spin-off security, handles fractional shares with cash-in-lieu, creates a price record
// for the spin-off security, and records a corporate action audit entry.
func (s *CorporateActionService) SpinOff(parentSecurityID, spinOffSecurityID types.ID, spinOffDate types.Date, params SpinOffParams, spinOffPrice types.Money) (*CorporateAction, error) {
	if errs := params.Validate(); errs.HasErrors() {
		return nil, fmt.Errorf("invalid spin-off parameters: %s", errs.Error())
	}

	if !spinOffPrice.IsPositive() {
		return nil, fmt.Errorf("spin-off price must be positive")
	}

	parentAllocPct := alpacadecimal.NewFromFloat(params.ParentAllocationPct).Div(alpacadecimal.NewFromInt(100))
	spinOffAllocPct := alpacadecimal.NewFromFloat(params.SpinOffAllocationPct()).Div(alpacadecimal.NewFromInt(100))
	shareRatio := alpacadecimal.NewFromFloat(params.ShareRatio)

	// Create audit record
	paramsJSON, err := params.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize spin-off params: %w", err)
	}

	ca := NewCorporateAction(ActionTypeSpinOff, parentSecurityID, spinOffDate, paramsJSON)
	ca.SetTargetSecurity(spinOffSecurityID)

	// The entire spin-off — reallocated parent lots/positions, minted child
	// lots/positions/exchange rows, cash-in-lieu, the seeded child price, and the
	// audit row — commits atomically.
	if err := s.runInTx(func(b *CorporateActionService) error {
		// Process lot-tracking accounts
		if err := b.spinOffProcessLots(parentSecurityID, spinOffSecurityID, spinOffDate, parentAllocPct, spinOffAllocPct, shareRatio, spinOffPrice); err != nil {
			return fmt.Errorf("failed to process lots for spin-off: %w", err)
		}

		// Process non-lot-tracking accounts (positions)
		if err := b.spinOffProcessPositions(parentSecurityID, spinOffSecurityID, spinOffDate, parentAllocPct, spinOffAllocPct, shareRatio, spinOffPrice); err != nil {
			return fmt.Errorf("failed to process positions for spin-off: %w", err)
		}

		// Create price record for spin-off security
		newPrice := price.NewPrice(spinOffSecurityID, spinOffDate, spinOffPrice, price.SourceTransaction)
		if err := b.priceRepo.CreateOrUpdate(newPrice); err != nil {
			return fmt.Errorf("failed to create spin-off price record: %w", err)
		}

		if err := b.caRepo.Create(ca); err != nil {
			return fmt.Errorf("failed to create corporate action record: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return ca, nil
}

// spinOffProcessLots handles the spin-off for lot-tracking accounts.
// For each open lot of the parent security: reduce cost_per_share by parent allocation %,
// create new lot for the spin-off security with allocated cost basis.
// Fractional shares are rounded down with cash-in-lieu.
func (s *CorporateActionService) spinOffProcessLots(parentSecurityID, spinOffSecurityID types.ID, spinOffDate types.Date, parentAllocPct, spinOffAllocPct, shareRatio alpacadecimal.Decimal, spinOffPrice types.Money) error {
	lots, err := s.lotRepo.GetOpenLotsBySecurity(parentSecurityID)
	if err != nil {
		return err
	}

	for _, lot := range lots {
		oldCostPerShare := lot.CostPerShare

		// Reduce parent lot cost by parent allocation percentage
		lot.CostPerShare = oldCostPerShare.Mul(parentAllocPct)
		if err := s.lotRepo.Update(lot); err != nil {
			return fmt.Errorf("failed to update parent lot %s: %w", lot.ID.String(), err)
		}

		// Calculate spin-off shares (may be fractional)
		rawSpinOffShares := lot.Shares.Mul(shareRatio)
		// Floor to whole shares
		wholeShares := rawSpinOffShares.Floor()
		fractionalPart := rawSpinOffShares.Sub(wholeShares)

		// Calculate spin-off cost per share from allocated cost basis
		spinOffCostBasis := oldCostPerShare.Mul(spinOffAllocPct).Mul(lot.Shares.Decimal())
		var spinOffCostPerShare types.Money
		if wholeShares.IsPositive() {
			spinOffCostPerShare = spinOffCostBasis.Mul(alpacadecimal.NewFromInt(1).Div(wholeShares.Decimal()))
		}

		// Create exchange transaction and lot for whole shares
		if wholeShares.IsPositive() {
			totalAmount := spinOffCostPerShare.Mul(wholeShares.Decimal())
			txn := NewTransactionWithSecurity(lot.AccountID, spinOffDate, TransactionTypeExchange, totalAmount, spinOffSecurityID, wholeShares)
			txn.SetPricePerShare(spinOffCostPerShare)
			txn.SetMemo(fmt.Sprintf("Spin-off: %s shares received", wholeShares.String()))
			if err := s.invRepo.Create(txn); err != nil {
				return fmt.Errorf("failed to create exchange transaction: %w", err)
			}

			newLot := NewLot(lot.AccountID, spinOffSecurityID, wholeShares, spinOffCostPerShare, lot.PurchaseDate, txn.ID)
			if err := s.lotRepo.Create(&newLot); err != nil {
				return fmt.Errorf("failed to create spin-off lot: %w", err)
			}
		}

		// Cash-in-lieu for fractional shares
		if fractionalPart.IsPositive() {
			cashAmount := spinOffPrice.Mul(fractionalPart.Decimal())
			cashTxn := NewTransaction(lot.AccountID, spinOffDate, TransactionTypeDeposit, cashAmount)
			cashTxn.SetMemo("Spin-off cash-in-lieu for fractional shares")
			if err := s.invRepo.Create(cashTxn); err != nil {
				return fmt.Errorf("failed to create cash-in-lieu transaction: %w", err)
			}
		}
	}

	return nil
}

// spinOffProcessPositions handles the spin-off for non-lot-tracking accounts.
// For each position of the parent security: reduce average cost by parent allocation %,
// create a new position for the spin-off security with allocated cost basis.
// Fractional shares are rounded down with cash-in-lieu.
func (s *CorporateActionService) spinOffProcessPositions(parentSecurityID, spinOffSecurityID types.ID, spinOffDate types.Date, parentAllocPct, spinOffAllocPct, shareRatio alpacadecimal.Decimal, spinOffPrice types.Money) error {
	positions, err := s.positionRepo.GetPositionsBySecurity(parentSecurityID)
	if err != nil {
		return err
	}

	for _, pos := range positions {
		// Skip lot-tracked accounts: their holdings ARE the lots, already
		// processed by spinOffProcessLots. Processing the redundant aggregate
		// position too would double-create the spin-off shares.
		if usesLots, err := s.accountUsesLotsFor(pos.AccountID, parentSecurityID); err != nil {
			return err
		} else if usesLots {
			continue
		}

		oldAvgCost := pos.AverageCostPerShare

		// Reduce parent position average cost by parent allocation percentage
		pos.AverageCostPerShare = oldAvgCost.Mul(parentAllocPct)
		if err := s.positionRepo.CreateOrUpdate(pos); err != nil {
			return fmt.Errorf("failed to update parent position: %w", err)
		}

		// Calculate spin-off shares
		rawSpinOffShares := pos.Shares.Mul(shareRatio)
		wholeShares := rawSpinOffShares.Floor()
		fractionalPart := rawSpinOffShares.Sub(wholeShares)

		// Calculate spin-off cost per share
		spinOffCostBasis := oldAvgCost.Mul(spinOffAllocPct).Mul(pos.Shares.Decimal())
		var spinOffCostPerShare types.Money
		if wholeShares.IsPositive() {
			spinOffCostPerShare = spinOffCostBasis.Mul(alpacadecimal.NewFromInt(1).Div(wholeShares.Decimal()))
		}

		// Create spin-off position for whole shares
		if wholeShares.IsPositive() {
			totalAmount := spinOffCostPerShare.Mul(wholeShares.Decimal())
			txn := NewTransactionWithSecurity(pos.AccountID, spinOffDate, TransactionTypeExchange, totalAmount, spinOffSecurityID, wholeShares)
			txn.SetPricePerShare(spinOffCostPerShare)
			txn.SetMemo(fmt.Sprintf("Spin-off: %s shares received", wholeShares.String()))
			if err := s.invRepo.Create(txn); err != nil {
				return fmt.Errorf("failed to create exchange transaction: %w", err)
			}

			spinOffPos, err := s.positionRepo.GetByAccountAndSecurity(pos.AccountID, spinOffSecurityID)
			if err != nil {
				return fmt.Errorf("failed to get spin-off position: %w", err)
			}
			if err := spinOffPos.AddShares(wholeShares, spinOffCostPerShare); err != nil {
				return fmt.Errorf("failed to add shares to spin-off position: %w", err)
			}
			if err := s.positionRepo.CreateOrUpdate(spinOffPos); err != nil {
				return fmt.Errorf("failed to save spin-off position: %w", err)
			}
		}

		// Cash-in-lieu for fractional shares
		if fractionalPart.IsPositive() {
			cashAmount := spinOffPrice.Mul(fractionalPart.Decimal())
			cashTxn := NewTransaction(pos.AccountID, spinOffDate, TransactionTypeDeposit, cashAmount)
			cashTxn.SetMemo("Spin-off cash-in-lieu for fractional shares")
			if err := s.invRepo.Create(cashTxn); err != nil {
				return fmt.Errorf("failed to create cash-in-lieu transaction: %w", err)
			}
		}
	}

	return nil
}
