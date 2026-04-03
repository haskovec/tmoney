package investment

import (
	"fmt"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

// CorporateActionService provides business logic for corporate action operations.
type CorporateActionService struct {
	caRepo       *CorporateActionRepository
	lotRepo      *LotRepository
	positionRepo *PositionRepository
	priceRepo    *price.Repository
	db           *db.DB
}

// NewCorporateActionService creates a new CorporateActionService.
func NewCorporateActionService(
	caRepo *CorporateActionRepository,
	lotRepo *LotRepository,
	positionRepo *PositionRepository,
	priceRepo *price.Repository,
	database *db.DB,
) *CorporateActionService {
	return &CorporateActionService{
		caRepo:       caRepo,
		lotRepo:      lotRepo,
		positionRepo: positionRepo,
		priceRepo:    priceRepo,
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
