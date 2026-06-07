package investment

import (
	"fmt"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// CorporateActionService provides business logic for corporate action operations.
type CorporateActionService struct {
	caRepo       *CorporateActionRepository
	lotRepo      *LotRepository
	positionRepo *PositionRepository
	priceRepo    *price.Repository
	invRepo      *Repository
	secRepo      *security.Repository
	db           *db.DB
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

// Split applies a stock split (or reverse split) to a security.
// It adjusts all open lots, non-zero positions, and historical prices on or before the split date.
// A corporate action audit record is created.
func (s *CorporateActionService) Split(securityID types.ID, splitDate types.Date, params SplitParams) (*CorporateAction, error) {
	// Validate parameters
	if errs := params.Validate(); errs.HasErrors() {
		return nil, fmt.Errorf("invalid split parameters: %s", errs.Error())
	}

	ratio := alpacadecimal.NewFromFloat(params.Ratio())
	inverseRatio := alpacadecimal.NewFromFloat(1.0 / params.Ratio())

	// Adjust open lots purchased on or before the split date. Shares acquired
	// after the split were already recorded at post-split quantities, so they
	// must NOT be re-split.
	if err := s.adjustLots(securityID, splitDate, ratio, inverseRatio); err != nil {
		return nil, fmt.Errorf("failed to adjust lots: %w", err)
	}

	// Bring positions in line with the shares actually held as of the split date.
	if err := s.adjustPositions(securityID, splitDate, ratio, false); err != nil {
		return nil, fmt.Errorf("failed to adjust positions: %w", err)
	}

	// Adjust price history on or before split date
	if err := s.adjustPrices(securityID, splitDate, inverseRatio); err != nil {
		return nil, fmt.Errorf("failed to adjust prices: %w", err)
	}

	// Create audit record
	actionType := ActionTypeSplit
	if params.Ratio() < 1 {
		actionType = ActionTypeReverseSplit
	}

	paramsJSON, err := params.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize split params: %w", err)
	}

	ca := NewCorporateAction(actionType, securityID, splitDate, paramsJSON)
	if err := s.caRepo.Create(ca); err != nil {
		return nil, fmt.Errorf("failed to create corporate action record: %w", err)
	}

	return ca, nil
}

// adjustLots adjusts open lots that existed as of the split date by the split
// multipliers. Shares are multiplied by shareMul; cost_per_share by costMul.
// Lots whose purchase date is AFTER the split date are left untouched — they
// were acquired post-split and are already at split-adjusted quantities.
// original_shares is NOT modified.
func (s *CorporateActionService) adjustLots(securityID types.ID, splitDate types.Date, shareMul, costMul alpacadecimal.Decimal) error {
	lots, err := s.lotRepo.GetOpenLotsBySecurity(securityID)
	if err != nil {
		return err
	}

	for _, lot := range lots {
		if lot.PurchaseDate.Time().After(splitDate.Time()) {
			continue // acquired after the split — already split-adjusted
		}
		lot.Shares = lot.Shares.Mul(shareMul)
		lot.CostPerShare = lot.CostPerShare.Mul(costMul)
		if err := s.lotRepo.Update(lot); err != nil {
			return fmt.Errorf("failed to update lot %s: %w", lot.ID.String(), err)
		}
	}
	return nil
}

// adjustPositions brings each account's stored position in line with the split,
// scoped to shares held as of the split date. For lot-tracking accounts the
// position is recomputed from the (already date-adjusted) open lots, so it
// stays consistent with them. For non-lot accounts the share count held as of
// the split date is split; shares acquired afterward are left unchanged, and
// total cost basis is preserved (a split never changes total invested). When
// reverse is true the adjustment is inverted (used to undo a split).
func (s *CorporateActionService) adjustPositions(securityID types.ID, splitDate types.Date, ratio alpacadecimal.Decimal, reverse bool) error {
	lots, err := s.lotRepo.GetOpenLotsBySecurity(securityID)
	if err != nil {
		return err
	}

	// Lot-tracking accounts: rebuild the aggregate position from the (already
	// date-adjusted) open lots so it stays consistent with them. This upserts
	// the row even when one didn't exist yet (a lot-tracking account holds its
	// shares in lots; the aggregate position is a derived cache).
	lotsByAccount := make(map[types.ID][]*Lot)
	for _, l := range lots {
		lotsByAccount[l.AccountID] = append(lotsByAccount[l.AccountID], l)
	}
	for accountID, accountLots := range lotsByAccount {
		rebuilt := NewPosition(accountID, securityID)
		for _, l := range accountLots {
			if err := rebuilt.AddShares(l.Shares, l.CostPerShare); err != nil {
				return fmt.Errorf("failed to rebuild position for account %s: %w", accountID.String(), err)
			}
		}
		if err := s.positionRepo.CreateOrUpdate(&rebuilt); err != nil {
			return fmt.Errorf("failed to update position for account %s: %w", accountID.String(), err)
		}
	}

	// Non-lot accounts: split only the shares held as of the split date; shares
	// acquired afterward stay put. Total cost basis is preserved.
	positions, err := s.positionRepo.GetPositionsBySecurity(securityID)
	if err != nil {
		return err
	}
	one := alpacadecimal.NewFromInt(1)
	for _, pos := range positions {
		if _, ok := lotsByAccount[pos.AccountID]; ok {
			continue // lot-tracking account, already rebuilt above
		}
		asOf, err := s.sharesHeldAsOf(pos.AccountID, securityID, splitDate)
		if err != nil {
			return err
		}
		if asOf.IsZero() || asOf.IsNegative() {
			continue // held nothing at the split date — nothing to adjust
		}
		delta := asOf.Mul(ratio.Sub(one)) // asOf × (ratio − 1)
		oldTotalCost := pos.AverageCostPerShare.Mul(pos.Shares.Decimal())
		if reverse {
			pos.Shares = pos.Shares.Sub(delta)
		} else {
			pos.Shares = pos.Shares.Add(delta)
		}
		if pos.Shares.IsZero() || pos.Shares.IsNegative() {
			continue
		}
		pos.AverageCostPerShare = oldTotalCost.Mul(one.Div(pos.Shares.Decimal()))
		if err := s.positionRepo.CreateOrUpdate(pos); err != nil {
			return fmt.Errorf("failed to update position for account %s: %w", pos.AccountID.String(), err)
		}
	}
	return nil
}

// sharesHeldAsOf returns the net share count an account held in a security on
// or before the given date, replayed from the transaction ledger. Used to
// scope a split to the shares that existed when it occurred.
func (s *CorporateActionService) sharesHeldAsOf(accountID, securityID types.ID, asOfDate types.Date) (types.Quantity, error) {
	filter := TransactionFilter{SecurityID: &securityID, ToDate: &asOfDate}
	txns, err := s.invRepo.ListByAccount(accountID, filter)
	if err != nil {
		return types.ZeroQuantity, err
	}
	net := types.ZeroQuantity
	for _, t := range txns {
		if !t.Shares.Valid {
			continue
		}
		switch t.Type {
		case TransactionTypeBuy, TransactionTypeReinvestDividend:
			net = net.Add(t.Shares.Quantity)
		case TransactionTypeSell, TransactionTypeFeeLiquidation:
			net = net.Sub(t.Shares.Quantity)
		case TransactionTypeTransferShares:
			if t.TotalAmount.IsNegative() {
				net = net.Sub(t.Shares.Quantity)
			} else {
				net = net.Add(t.Shares.Quantity)
			}
		}
	}
	return net, nil
}

// adjustPrices adjusts all prices on or before the split date by dividing by the ratio.
func (s *CorporateActionService) adjustPrices(securityID types.ID, splitDate types.Date, inverseRatio alpacadecimal.Decimal) error {
	prices, err := s.priceRepo.GetPriceHistory(securityID, nil, &splitDate)
	if err != nil {
		return err
	}

	for _, p := range prices {
		adjustedPrice := p.Price.Mul(inverseRatio)
		updated := price.NewPrice(p.SecurityID, p.Date, adjustedPrice, p.Source)
		if err := s.priceRepo.CreateOrUpdate(updated); err != nil {
			return fmt.Errorf("failed to update price for date %s: %w", p.Date.String(), err)
		}
	}
	return nil
}

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
	inverseRatio := alpacadecimal.NewFromFloat(1.0 / params.ExchangeRatio)

	// Process lot-tracking accounts
	if err := s.mergerProcessLots(sourceSecurityID, targetSecurityID, mergerDate, exchangeRatio, inverseRatio, params); err != nil {
		return nil, fmt.Errorf("failed to process lots for merger: %w", err)
	}

	// Process non-lot-tracking accounts (positions)
	if err := s.mergerProcessPositions(sourceSecurityID, targetSecurityID, mergerDate, exchangeRatio, inverseRatio, params); err != nil {
		return nil, fmt.Errorf("failed to process positions for merger: %w", err)
	}

	// Hide source security
	if err := s.mergerHideSource(sourceSecurityID); err != nil {
		return nil, fmt.Errorf("failed to hide source security: %w", err)
	}

	// Create audit record
	paramsJSON, err := params.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize merger params: %w", err)
	}

	ca := NewCorporateAction(ActionTypeMerger, sourceSecurityID, mergerDate, paramsJSON)
	ca.SetTargetSecurity(targetSecurityID)
	if err := s.caRepo.Create(ca); err != nil {
		return nil, fmt.Errorf("failed to create corporate action record: %w", err)
	}

	return ca, nil
}

// mergerProcessLots handles the merger for lot-tracking accounts.
// For each open lot of the source security: close the lot, create a new lot for
// the target security with adjusted shares and cost basis, and create an exchange transaction.
func (s *CorporateActionService) mergerProcessLots(sourceSecurityID, targetSecurityID types.ID, mergerDate types.Date, exchangeRatio, inverseRatio alpacadecimal.Decimal, params MergerParams) error {
	lots, err := s.lotRepo.GetOpenLotsBySecurity(sourceSecurityID)
	if err != nil {
		return err
	}

	// Track total old shares per account for cash consideration
	accountOldShares := make(map[types.ID]types.Quantity)

	for _, lot := range lots {
		oldShares := lot.Shares
		newShares := oldShares.Mul(inverseRatio)
		// Cost basis preservation: new_cost_per_share = old × exchange_ratio
		newCostPerShare := lot.CostPerShare.Mul(exchangeRatio)

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
			cashTxn.SetMemo("Merger cash consideration")
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
func (s *CorporateActionService) mergerProcessPositions(sourceSecurityID, targetSecurityID types.ID, mergerDate types.Date, exchangeRatio, inverseRatio alpacadecimal.Decimal, params MergerParams) error {
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
		newShares := oldShares.Mul(inverseRatio)
		newCostPerShare := pos.AverageCostPerShare.Mul(exchangeRatio)

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
			cashTxn.SetMemo("Merger cash consideration")
			if err := s.invRepo.Create(cashTxn); err != nil {
				return fmt.Errorf("failed to create cash consideration transaction: %w", err)
			}
		}
	}

	return nil
}

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

	// Process lot-tracking accounts
	if err := s.spinOffProcessLots(parentSecurityID, spinOffSecurityID, spinOffDate, parentAllocPct, spinOffAllocPct, shareRatio, spinOffPrice); err != nil {
		return nil, fmt.Errorf("failed to process lots for spin-off: %w", err)
	}

	// Process non-lot-tracking accounts (positions)
	if err := s.spinOffProcessPositions(parentSecurityID, spinOffSecurityID, spinOffDate, parentAllocPct, spinOffAllocPct, shareRatio, spinOffPrice); err != nil {
		return nil, fmt.Errorf("failed to process positions for spin-off: %w", err)
	}

	// Create price record for spin-off security
	newPrice := price.NewPrice(spinOffSecurityID, spinOffDate, spinOffPrice, price.SourceTransaction)
	if err := s.priceRepo.CreateOrUpdate(newPrice); err != nil {
		return nil, fmt.Errorf("failed to create spin-off price record: %w", err)
	}

	// Create audit record
	paramsJSON, err := params.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize spin-off params: %w", err)
	}

	ca := NewCorporateAction(ActionTypeSpinOff, parentSecurityID, spinOffDate, paramsJSON)
	ca.SetTargetSecurity(spinOffSecurityID)
	if err := s.caRepo.Create(ca); err != nil {
		return nil, fmt.Errorf("failed to create corporate action record: %w", err)
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

// ListBySecurity retrieves all corporate actions for a security (as source or target).
func (s *CorporateActionService) ListBySecurity(securityID types.ID) ([]*CorporateAction, error) {
	return s.caRepo.ListBySecurity(securityID)
}

// ListAll retrieves every corporate action in the database.
func (s *CorporateActionService) ListAll() ([]*CorporateAction, error) {
	return s.caRepo.ListAll()
}

// DeleteAction reverses a corporate action's effects (on lots, positions,
// and prices) and removes its audit row. Refuses to run if any investment
// transaction on the affected security(ies) has a date on or after the
// action date; the returned *DownstreamEventsError names the earliest
// blocking transaction so the caller can show a precise message.
//
// In v1, only stock-split and reverse-split actions are reversible.
// Merger and spin-off reversals create new transactions, lots, and
// (for mergers) hide the source security; safely undoing all of that
// requires careful per-account choreography and is deferred — those
// types return an *UnsupportedReversalError.
func (s *CorporateActionService) DeleteAction(actionID types.ID) error {
	ca, err := s.caRepo.GetByID(actionID)
	if err != nil {
		return err
	}

	switch ca.ActionType {
	case ActionTypeSplit, ActionTypeReverseSplit:
		if err := s.checkNoDownstreamEvents(ca); err != nil {
			return err
		}
		return s.reverseSplit(ca)
	case ActionTypeSpinOff:
		return s.reverseSpinOff(ca)
	case ActionTypeMerger:
		return &UnsupportedReversalError{ActionType: ca.ActionType}
	}
	return fmt.Errorf("unknown corporate action type: %s", ca.ActionType)
}

// reverseSpinOff undoes a spin-off: it removes the spun-off child lots,
// positions, exchange/cash-in-lieu transactions, and the seeded child price,
// and restores the parent's cost basis. It refuses (with a *DownstreamEventsError
// naming the blocker) when the parent has any transaction on/after the spin date
// or when the child has any transaction other than the spin-off's own same-date
// exchange receipts — i.e. when the spun-off shares have been sold or otherwise
// used, since those consume the lots this reversal would delete.
func (s *CorporateActionService) reverseSpinOff(ca *CorporateAction) error {
	params, err := ParseSpinOffParams(ca.Parameters)
	if err != nil {
		return fmt.Errorf("failed to parse spin-off params: %w", err)
	}
	if !ca.TargetSecurityID.Valid {
		return fmt.Errorf("spin-off action %s has no target security", ca.ID)
	}
	parentID := ca.SecurityID
	childID := ca.TargetSecurityID.ID
	spinDate := ca.ActionDate

	// Guard A: the parent must have no transactions on/after the spin date
	// (the spin-off itself created none on the parent).
	earliest, err := s.invRepo.EarliestSinceDate(parentID, spinDate)
	if err != nil {
		return fmt.Errorf("failed to check parent downstream events: %w", err)
	}
	if earliest != nil {
		return s.downstreamError(parentID, spinDate, earliest.Date, string(earliest.Type))
	}

	// Guard B: the only child transactions may be the spin-off's own exchange
	// receipts dated the spin date. A sale, later buy, or transfer of the child
	// means the spun-off shares were used; refuse and name the blocker.
	childTxns, err := s.invRepo.ListBySecurity(childID)
	if err != nil {
		return fmt.Errorf("failed to list child transactions: %w", err)
	}
	for _, t := range childTxns {
		if t.Type != TransactionTypeExchange || !t.Date.Time().Equal(spinDate.Time()) {
			return s.downstreamError(childID, spinDate, t.Date, string(t.Type))
		}
	}

	// Restore parent cost basis (undo the × parentAllocPct scaling) and collect
	// the touched accounts for cash-in-lieu cleanup.
	parentAllocFrac := alpacadecimal.NewFromFloat(params.ParentAllocationPct).Div(alpacadecimal.NewFromInt(100))
	inverse := alpacadecimal.NewFromInt(1).Div(parentAllocFrac)
	touched := make(map[types.ID]bool)

	parentLots, err := s.lotRepo.GetOpenLotsBySecurity(parentID)
	if err != nil {
		return err
	}
	for _, lot := range parentLots {
		lot.CostPerShare = lot.CostPerShare.Mul(inverse)
		if err := s.lotRepo.Update(lot); err != nil {
			return fmt.Errorf("failed to restore parent lot %s: %w", lot.ID, err)
		}
		touched[lot.AccountID] = true
	}
	parentPositions, err := s.positionRepo.GetPositionsBySecurity(parentID)
	if err != nil {
		return err
	}
	for _, pos := range parentPositions {
		pos.AverageCostPerShare = pos.AverageCostPerShare.Mul(inverse)
		if err := s.positionRepo.CreateOrUpdate(pos); err != nil {
			return fmt.Errorf("failed to restore parent position: %w", err)
		}
		touched[pos.AccountID] = true
	}

	// Delete child lots and the exchange transactions that created them.
	for _, t := range childTxns {
		touched[t.AccountID] = true
		if lot, lerr := s.lotRepo.GetBySourceTransaction(t.ID); lerr == nil && lot != nil {
			if err := s.lotRepo.Delete(lot.ID); err != nil {
				return fmt.Errorf("failed to delete child lot %s: %w", lot.ID, err)
			}
		}
		if err := s.invRepo.Delete(t.ID); err != nil {
			return fmt.Errorf("failed to delete child exchange transaction %s: %w", t.ID, err)
		}
	}

	// Delete child positions (non-lot accounts).
	childPositions, err := s.positionRepo.GetPositionsBySecurity(childID)
	if err != nil {
		return err
	}
	for _, pos := range childPositions {
		if err := s.positionRepo.Delete(pos.AccountID, childID); err != nil {
			if _, ok := err.(*dberrors.NotFoundError); !ok {
				return fmt.Errorf("failed to delete child position: %w", err)
			}
		}
	}

	// Delete cash-in-lieu deposits on the spin date in any touched account.
	for acctID := range touched {
		txns, err := s.invRepo.ListByAccount(acctID, TransactionFilter{})
		if err != nil {
			return fmt.Errorf("failed to list account transactions: %w", err)
		}
		for _, t := range txns {
			if t.Type == TransactionTypeDeposit && t.Date.Time().Equal(spinDate.Time()) &&
				t.Memo.Valid && t.Memo.String == "Spin-off cash-in-lieu for fractional shares" {
				if err := s.invRepo.Delete(t.ID); err != nil {
					return fmt.Errorf("failed to delete cash-in-lieu transaction %s: %w", t.ID, err)
				}
			}
		}
	}

	// Delete the seeded child price record on the spin date (best effort).
	if existing, perr := s.priceRepo.GetBySecurityAndDate(childID, spinDate); perr == nil && existing != nil {
		_ = s.priceRepo.Delete(existing.ID)
	}

	// Delete the audit row.
	if err := s.caRepo.Delete(ca.ID); err != nil {
		return fmt.Errorf("failed to delete corporate action: %w", err)
	}
	return nil
}

// downstreamError builds a *DownstreamEventsError naming a blocking transaction.
func (s *CorporateActionService) downstreamError(secID types.ID, actionDate, blockerDate types.Date, blockerType string) *DownstreamEventsError {
	ticker := ""
	if sec, err := s.secRepo.GetByID(secID); err == nil && sec != nil {
		ticker = sec.Ticker
	}
	return &DownstreamEventsError{
		ActionDate:     actionDate,
		BlockerTicker:  ticker,
		BlockerDate:    blockerDate,
		BlockerTxnType: blockerType,
	}
}

// checkNoDownstreamEvents returns a *DownstreamEventsError naming the
// earliest blocking transaction if any investment transaction on or
// after the action date exists for the action's security (or, for
// two-security actions like mergers and spin-offs, either security).
// Returns nil otherwise.
func (s *CorporateActionService) checkNoDownstreamEvents(ca *CorporateAction) error {
	secIDs := []types.ID{ca.SecurityID}
	if ca.TargetSecurityID.Valid {
		secIDs = append(secIDs, ca.TargetSecurityID.ID)
	}
	for _, secID := range secIDs {
		earliest, err := s.invRepo.EarliestSinceDate(secID, ca.ActionDate)
		if err != nil {
			return fmt.Errorf("failed to check downstream events: %w", err)
		}
		if earliest == nil {
			continue
		}
		ticker := ""
		if sec, err := s.secRepo.GetByID(secID); err == nil && sec != nil {
			ticker = sec.Ticker
		}
		return &DownstreamEventsError{
			ActionDate:     ca.ActionDate,
			BlockerTicker:  ticker,
			BlockerDate:    earliest.Date,
			BlockerTxnType: string(earliest.Type),
		}
	}
	return nil
}

// reverseSplit undoes a Split or ReverseSplit by inverting the share
// and cost-basis multipliers applied at create time, then deleting the
// audit row.
func (s *CorporateActionService) reverseSplit(ca *CorporateAction) error {
	params, err := ParseSplitParams(ca.Parameters)
	if err != nil {
		return fmt.Errorf("failed to parse split params: %w", err)
	}

	// Original Split called adjust* with (ratio, inverseRatio). To undo,
	// swap them: shares *= inverseRatio, cost *= ratio, prices *= ratio.
	origRatio := alpacadecimal.NewFromFloat(params.Ratio())
	origInverse := alpacadecimal.NewFromFloat(1.0 / params.Ratio())

	if err := s.adjustLots(ca.SecurityID, ca.ActionDate, origInverse, origRatio); err != nil {
		return fmt.Errorf("failed to reverse lots: %w", err)
	}
	if err := s.adjustPositions(ca.SecurityID, ca.ActionDate, origRatio, true); err != nil {
		return fmt.Errorf("failed to reverse positions: %w", err)
	}
	if err := s.adjustPrices(ca.SecurityID, ca.ActionDate, origRatio); err != nil {
		return fmt.Errorf("failed to reverse prices: %w", err)
	}

	if err := s.caRepo.Delete(ca.ID); err != nil {
		return fmt.Errorf("failed to delete corporate action: %w", err)
	}
	return nil
}

// DownstreamEventsError is returned when a reversal is blocked by a
// later investment transaction on the affected security.
type DownstreamEventsError struct {
	ActionDate     types.Date
	BlockerTicker  string
	BlockerDate    types.Date
	BlockerTxnType string
}

func (e *DownstreamEventsError) Error() string {
	ticker := e.BlockerTicker
	if ticker == "" {
		ticker = "the affected security"
	}
	return fmt.Sprintf(
		"cannot reverse: %s has a %s transaction on %s (action dated %s). Remove or re-date later transactions first.",
		ticker, e.BlockerTxnType, e.BlockerDate.String(), e.ActionDate.String(),
	)
}

// UnsupportedReversalError is returned when the user tries to reverse
// a corporate-action type for which reversal is not yet implemented.
type UnsupportedReversalError struct {
	ActionType ActionType
}

func (e *UnsupportedReversalError) Error() string {
	return fmt.Sprintf(
		"reversing %s corporate actions is not yet supported — only splits and reverse splits can be undone in this version",
		e.ActionType.DisplayName(),
	)
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
