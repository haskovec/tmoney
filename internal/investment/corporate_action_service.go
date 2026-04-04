package investment

import (
	"fmt"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/db"
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

	// Adjust all open lots
	if err := s.adjustLots(securityID, ratio, inverseRatio); err != nil {
		return nil, fmt.Errorf("failed to adjust lots: %w", err)
	}

	// Adjust all non-zero positions
	if err := s.adjustPositions(securityID, ratio, inverseRatio); err != nil {
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

// adjustLots adjusts all open lots for a security by the split ratio.
// Shares are multiplied by ratio; cost_per_share is divided by ratio (multiplied by inverse).
// original_shares is NOT modified.
func (s *CorporateActionService) adjustLots(securityID types.ID, ratio, inverseRatio alpacadecimal.Decimal) error {
	lots, err := s.lotRepo.GetOpenLotsBySecurity(securityID)
	if err != nil {
		return err
	}

	for _, lot := range lots {
		lot.Shares = lot.Shares.Mul(ratio)
		lot.CostPerShare = lot.CostPerShare.Mul(inverseRatio)
		if err := s.lotRepo.Update(lot); err != nil {
			return fmt.Errorf("failed to update lot %s: %w", lot.ID.String(), err)
		}
	}
	return nil
}

// adjustPositions adjusts all non-zero positions for a security by the split ratio.
// Shares are multiplied by ratio; average_cost_per_share is divided by ratio.
func (s *CorporateActionService) adjustPositions(securityID types.ID, ratio, inverseRatio alpacadecimal.Decimal) error {
	positions, err := s.positionRepo.GetPositionsBySecurity(securityID)
	if err != nil {
		return err
	}

	for _, pos := range positions {
		pos.Shares = pos.Shares.Mul(ratio)
		pos.AverageCostPerShare = pos.AverageCostPerShare.Mul(inverseRatio)
		if err := s.positionRepo.CreateOrUpdate(pos); err != nil {
			return fmt.Errorf("failed to update position for account %s: %w", pos.AccountID.String(), err)
		}
	}
	return nil
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

// ListBySecurity retrieves all corporate actions for a security (as source or target).
func (s *CorporateActionService) ListBySecurity(securityID types.ID) ([]*CorporateAction, error) {
	return s.caRepo.ListBySecurity(securityID)
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
