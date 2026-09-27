package investment

import (
	"fmt"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/types"
)

// Mergers: the source security is exchanged for the target at a fixed ratio.
//
// The ratio is target shares received per source share, the market convention
// and what both the CLI flag and the TUI field promise: a ratio of 2 turns 100
// source shares into 200 target shares. Cost basis carries over, so the cost
// per share is divided by the ratio. Lots and positions move to the target
// security. A fractional target share stays in the account; no cash-in-lieu is
// paid. The source security is hidden rather than deleted, so its history
// stays readable.

// Merger applies a merger/acquisition that converts shares of a source security
// into shares of a target security. Optionally includes cash consideration.
// All open lots and non-zero positions for the source security are exchanged.
// The source security is hidden after all positions are exchanged.
// A corporate action audit record is created.
func (s *CorporateActionService) Merger(sourceSecurityID, targetSecurityID types.ID, mergerDate types.Date, params MergerParams) (*CorporateAction, error) {
	if errs := params.Validate(); errs.HasErrors() {
		return nil, fmt.Errorf("invalid merger parameters: %s", errs.Error())
	}

	exchangeRatio := alpacadecimal.NewFromFloat(params.ExchangeRatio)

	// Create audit record
	paramsJSON, err := params.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize merger params: %w", err)
	}

	ca := NewCorporateAction(ActionTypeMerger, sourceSecurityID, mergerDate, paramsJSON)
	ca.SetTargetSecurity(targetSecurityID)

	// The entire merger — exchanged lots/positions, minted exchange and cash
	// consideration rows, source-security hide, and the audit row — commits
	// atomically.
	if err := s.runInTx(func(b *CorporateActionService) error {
		// Process lot-tracking accounts
		if err := b.mergerProcessLots(sourceSecurityID, targetSecurityID, mergerDate, exchangeRatio, params); err != nil {
			return fmt.Errorf("failed to process lots for merger: %w", err)
		}

		// Process non-lot-tracking accounts (positions)
		if err := b.mergerProcessPositions(sourceSecurityID, targetSecurityID, mergerDate, exchangeRatio, params); err != nil {
			return fmt.Errorf("failed to process positions for merger: %w", err)
		}

		// Hide source security
		if err := b.mergerHideSource(sourceSecurityID); err != nil {
			return fmt.Errorf("failed to hide source security: %w", err)
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

// mergerProcessLots handles the merger for lot-tracking accounts.
// For each open lot of the source security: close the lot, create a new lot for
// the target security with adjusted shares and cost basis, and create an exchange transaction.
func (s *CorporateActionService) mergerProcessLots(sourceSecurityID, targetSecurityID types.ID, mergerDate types.Date, exchangeRatio alpacadecimal.Decimal, params MergerParams) error {
	lots, err := s.lotRepo.GetOpenLotsBySecurity(sourceSecurityID)
	if err != nil {
		return err
	}

	// Track total old shares per account for cash consideration
	accountOldShares := make(map[types.ID]types.Quantity)

	for _, lot := range lots {
		oldShares := lot.Shares
		newShares := oldShares.Mul(exchangeRatio)
		// Cost basis preservation: new_cost_per_share = old ÷ exchange_ratio
		newCostPerShare := mergerCostPerShare(lot.CostPerShare, exchangeRatio)

		// Accumulate old shares per account for cash consideration
		if params.HasCashConsideration() {
			existing, ok := accountOldShares[lot.AccountID]
			if !ok {
				existing = types.ZeroQuantity
			}
			accountOldShares[lot.AccountID] = existing.Add(oldShares)
		}

		// Create exchange transaction for target security
		totalAmount := newCostPerShare.Mul(newShares.Decimal())
		txn := NewTransactionWithSecurity(lot.AccountID, mergerDate, TransactionTypeExchange, totalAmount, targetSecurityID, newShares)
		txn.SetPricePerShare(newCostPerShare)
		txn.SetMemo(fmt.Sprintf("Merger: %s shares exchanged", oldShares.String()))
		if err := s.invRepo.Create(txn); err != nil {
			return fmt.Errorf("failed to create exchange transaction: %w", err)
		}

		// Create new lot for target security
		newLot := NewLot(lot.AccountID, targetSecurityID, newShares, newCostPerShare, lot.PurchaseDate, txn.ID)
		if err := s.lotRepo.Create(&newLot); err != nil {
			return fmt.Errorf("failed to create target lot: %w", err)
		}

		// Close source lot
		if err := lot.Reduce(oldShares); err != nil {
			return fmt.Errorf("failed to close source lot: %w", err)
		}
		if err := s.lotRepo.Update(lot); err != nil {
			return fmt.Errorf("failed to update source lot: %w", err)
		}
	}

	// Add cash consideration for lot-tracking accounts
	if params.HasCashConsideration() {
		cashPerShareMoney := types.NewMoneyFromFloat(params.CashPerShare)
		for accountID, totalShares := range accountOldShares {
			cashAmount := cashPerShareMoney.Mul(totalShares.Decimal())
			cashTxn := NewTransaction(accountID, mergerDate, TransactionTypeDeposit, cashAmount)
			cashTxn.SetMemo(MemoMergerCashConsideration)
			if err := s.invRepo.Create(cashTxn); err != nil {
				return fmt.Errorf("failed to create cash consideration transaction: %w", err)
			}
		}
	}

	return nil
}

// mergerProcessPositions handles the merger for non-lot-tracking accounts.
// For each position of the source security: remove the position, create/update
// a position for the target security with adjusted shares and cost basis.
func (s *CorporateActionService) mergerProcessPositions(sourceSecurityID, targetSecurityID types.ID, mergerDate types.Date, exchangeRatio alpacadecimal.Decimal, params MergerParams) error {
	positions, err := s.positionRepo.GetPositionsBySecurity(sourceSecurityID)
	if err != nil {
		return err
	}

	for _, pos := range positions {
		// Skip lot-tracked accounts: their holdings ARE the lots, already
		// processed by mergerProcessLots. Processing the redundant aggregate
		// position too would double-create the merged shares.
		if usesLots, err := s.accountUsesLotsFor(pos.AccountID, sourceSecurityID); err != nil {
			return err
		} else if usesLots {
			continue
		}

		oldShares := pos.Shares
		newShares := oldShares.Mul(exchangeRatio)
		newCostPerShare := mergerCostPerShare(pos.AverageCostPerShare, exchangeRatio)

		// Create exchange transaction
		totalAmount := newCostPerShare.Mul(newShares.Decimal())
		txn := NewTransactionWithSecurity(pos.AccountID, mergerDate, TransactionTypeExchange, totalAmount, targetSecurityID, newShares)
		txn.SetPricePerShare(newCostPerShare)
		txn.SetMemo(fmt.Sprintf("Merger: %s shares exchanged", oldShares.String()))
		if err := s.invRepo.Create(txn); err != nil {
			return fmt.Errorf("failed to create exchange transaction: %w", err)
		}

		// Get or create target position and add shares
		targetPos, err := s.positionRepo.GetByAccountAndSecurity(pos.AccountID, targetSecurityID)
		if err != nil {
			return fmt.Errorf("failed to get target position: %w", err)
		}
		if err := targetPos.AddShares(newShares, newCostPerShare); err != nil {
			return fmt.Errorf("failed to add shares to target position: %w", err)
		}
		if err := s.positionRepo.CreateOrUpdate(targetPos); err != nil {
			return fmt.Errorf("failed to save target position: %w", err)
		}

		// Zero source position
		pos.Shares = types.ZeroQuantity
		pos.AverageCostPerShare = types.ZeroMoney
		if err := s.positionRepo.CreateOrUpdate(pos); err != nil {
			return fmt.Errorf("failed to zero source position: %w", err)
		}

		// Cash consideration
		if params.HasCashConsideration() {
			cashPerShareMoney := types.NewMoneyFromFloat(params.CashPerShare)
			cashAmount := cashPerShareMoney.Mul(oldShares.Decimal())
			cashTxn := NewTransaction(pos.AccountID, mergerDate, TransactionTypeDeposit, cashAmount)
			cashTxn.SetMemo(MemoMergerCashConsideration)
			if err := s.invRepo.Create(cashTxn); err != nil {
				return fmt.Errorf("failed to create cash consideration transaction: %w", err)
			}
		}
	}

	return nil
}

// costPerSharePlaces is the scale of cost_per_share, average_cost_per_share,
// price_per_share and total_amount (DECIMAL(19,4)).
const costPerSharePlaces = 4

// mergerCostPerShare carries a source cost per share onto the target: the
// same basis spread over ratio-times as many shares.
//
// The quotient is rounded here, once, to the four places the columns store,
// half up as DuckDB rounds on insert. Every caller then uses this one value
// for the new lot or position, the exchange row's price, and its total
// (cost × new shares), so the stored rows describe one cost. Left unrounded,
// the total was computed from 16.6666… and stored as 5000.0000 while the lot
// stored 16.6667 × 300 = 5000.01. When old cost ÷ ratio has more than four
// places the basis moves by that rounding; it cannot stay exact.
func mergerCostPerShare(sourceCost types.Money, exchangeRatio alpacadecimal.Decimal) types.Money {
	return types.NewMoneyFromDecimal(sourceCost.Decimal().Div(exchangeRatio).Round(costPerSharePlaces))
}

// mergerHideSource marks the source security as hidden after all positions are exchanged.
func (s *CorporateActionService) mergerHideSource(securityID types.ID) error {
	sec, err := s.secRepo.GetByID(securityID)
	if err != nil {
		return fmt.Errorf("failed to get source security: %w", err)
	}

	if sec.Hidden {
		return nil
	}

	sec.Hide()
	if err := s.secRepo.Update(sec); err != nil {
		return fmt.Errorf("failed to hide source security: %w", err)
	}

	return nil
}
