package investment

import (
	"fmt"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

// GetAccountValuation returns the total valuation of an investment account.
// It computes cash balance + market value of all holdings.
// Securities with no price as of the given date use cost basis as the estimated value.
func (s *Service) GetAccountValuation(accountID types.ID, asOf types.Date) (*AccountValuation, error) {
	acct, err := s.getInvestmentAccount(accountID)
	if err != nil {
		return nil, err
	}

	cashBalance, err := s.GetCashBalance(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get cash balance: %w", err)
	}

	holdings, err := s.getHoldings(acct, asOf)
	if err != nil {
		return nil, fmt.Errorf("failed to get holdings: %w", err)
	}

	marketValue := types.ZeroMoney
	totalCostBasis := types.ZeroMoney
	for _, h := range holdings {
		marketValue = marketValue.Add(h.MarketValue)
		totalCostBasis = totalCostBasis.Add(h.CostBasis)
	}

	totalValue := cashBalance.Add(marketValue)
	totalGainLoss := marketValue.Sub(totalCostBasis)
	totalGainPct := computeGainPct(marketValue, totalCostBasis)

	return &AccountValuation{
		AccountID:      accountID,
		CashBalance:    cashBalance,
		MarketValue:    marketValue,
		TotalValue:     totalValue,
		TotalCostBasis: totalCostBasis,
		TotalGainLoss:  totalGainLoss,
		TotalGainPct:   totalGainPct,
		Holdings:       holdings,
	}, nil
}

// GetHoldings returns a list of holdings for an investment account, rolled up by security.
// For lot-tracking accounts, lots are aggregated into a single holding per security.
func (s *Service) GetHoldings(accountID types.ID, asOf types.Date) ([]Holding, error) {
	acct, err := s.getInvestmentAccount(accountID)
	if err != nil {
		return nil, err
	}

	return s.getHoldings(acct, asOf)
}

// GetLotDetail returns lot-level detail for a specific security in a lot-tracking account.
func (s *Service) GetLotDetail(accountID types.ID, securityID types.ID, asOf types.Date) ([]LotDetail, error) {
	acct, err := s.getInvestmentAccount(accountID)
	if err != nil {
		return nil, err
	}

	if !acct.TrackLots {
		return nil, fmt.Errorf("account %s does not use lot tracking", accountID)
	}

	lots, err := s.lotRepo.ListByAccountAndSecurity(accountID, securityID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list lots: %w", err)
	}

	currentPrice, hasPricing := s.getCurrentPrice(securityID, asOf)

	details := make([]LotDetail, 0, len(lots))
	for _, lot := range lots {
		costBasis := lot.CostBasis()

		var currentValue types.Money
		if hasPricing {
			currentValue = currentPrice.Mul(lot.Shares.Decimal())
		} else {
			currentValue = costBasis
		}

		gainLoss := currentValue.Sub(costBasis)

		details = append(details, LotDetail{
			LotID:        lot.ID,
			PurchaseDate: lot.PurchaseDate,
			Shares:       lot.Shares,
			CostPerShare: lot.CostPerShare,
			CostBasis:    costBasis,
			CurrentValue: currentValue,
			GainLoss:     gainLoss,
			GainPct:      computeGainPct(currentValue, costBasis),
		})
	}

	return details, nil
}

// getHoldings builds holdings for an account based on its tracking mode.
func (s *Service) getHoldings(acct *account.Account, asOf types.Date) ([]Holding, error) {
	if acct.TrackLots {
		return s.getHoldingsFromLots(acct.ID, asOf)
	}
	return s.getHoldingsFromPositions(acct.ID, asOf)
}

// getHoldingsFromPositions builds holdings from positions (non-lot-tracking).
func (s *Service) getHoldingsFromPositions(accountID types.ID, asOf types.Date) ([]Holding, error) {
	positions, err := s.positionRepo.ListByAccount(accountID, true)
	if err != nil {
		return nil, fmt.Errorf("failed to list positions: %w", err)
	}

	holdings := make([]Holding, 0, len(positions))
	for _, pos := range positions {
		currentPrice, hasPricing := s.getCurrentPrice(pos.SecurityID, asOf)
		costBasis := pos.CostBasis()

		var marketValue types.Money
		var priceDate types.Date
		if hasPricing {
			marketValue = pos.MarketValue(currentPrice)
			priceDate = s.getPriceDate(pos.SecurityID, asOf)
		} else {
			marketValue = costBasis
		}

		gainLoss := marketValue.Sub(costBasis)

		holdings = append(holdings, Holding{
			SecurityID:   pos.SecurityID,
			Shares:       pos.Shares,
			AvgCost:      pos.AverageCostPerShare,
			CurrentPrice: currentPrice,
			PriceDate:    priceDate,
			MarketValue:  marketValue,
			CostBasis:    costBasis,
			GainLoss:     gainLoss,
			GainPct:      computeGainPct(marketValue, costBasis),
			HasPricing:   hasPricing,
		})
	}

	return holdings, nil
}

// getHoldingsFromLots builds holdings aggregated from lots (lot-tracking).
func (s *Service) getHoldingsFromLots(accountID types.ID, asOf types.Date) ([]Holding, error) {
	// Get all open lots for this account, grouped by security
	// We need to find which securities have lots, then aggregate
	// Use the position repo isn't available for lot-tracking, so we query lots directly

	// Get all investment transactions to find which securities are held
	txns, err := s.repo.ListByAccount(accountID, TransactionFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list transactions: %w", err)
	}

	// Collect unique security IDs from transactions
	securityIDs := make(map[types.ID]bool)
	for _, txn := range txns {
		if txn.SecurityID.Valid {
			securityIDs[txn.SecurityID.ID] = true
		}
	}

	holdings := make([]Holding, 0)
	for secID := range securityIDs {
		lots, err := s.lotRepo.ListByAccountAndSecurity(accountID, secID, false)
		if err != nil {
			return nil, fmt.Errorf("failed to list lots for security %s: %w", secID, err)
		}

		if len(lots) == 0 {
			continue
		}

		// Aggregate lots
		totalShares := types.ZeroQuantity
		totalCostBasis := types.ZeroMoney
		for _, lot := range lots {
			totalShares = totalShares.Add(lot.Shares)
			totalCostBasis = totalCostBasis.Add(lot.CostBasis())
		}

		if totalShares.IsZero() {
			continue
		}

		// Compute average cost
		reciprocal := alpacadecimal.NewFromInt(1).Div(totalShares.Decimal())
		avgCost := totalCostBasis.Mul(reciprocal)

		currentPrice, hasPricing := s.getCurrentPrice(secID, asOf)

		var marketValue types.Money
		var priceDate types.Date
		if hasPricing {
			marketValue = currentPrice.Mul(totalShares.Decimal())
			priceDate = s.getPriceDate(secID, asOf)
		} else {
			marketValue = totalCostBasis
		}

		gainLoss := marketValue.Sub(totalCostBasis)

		holdings = append(holdings, Holding{
			SecurityID:   secID,
			Shares:       totalShares,
			AvgCost:      avgCost,
			CurrentPrice: currentPrice,
			PriceDate:    priceDate,
			MarketValue:  marketValue,
			CostBasis:    totalCostBasis,
			GainLoss:     gainLoss,
			GainPct:      computeGainPct(marketValue, totalCostBasis),
			HasPricing:   hasPricing,
		})
	}

	return holdings, nil
}

// getCurrentPrice returns the most recent price for a security on or before the given date.
// Returns the price and true if found, or ZeroMoney and false if not found.
func (s *Service) getCurrentPrice(securityID types.ID, asOf types.Date) (types.Money, bool) {
	if s.priceRepo == nil {
		return types.ZeroMoney, false
	}

	p, err := s.priceRepo.GetCurrentPrice(securityID, asOf)
	if err != nil {
		if _, ok := err.(*dberrors.NotFoundError); ok {
			return types.ZeroMoney, false
		}
		return types.ZeroMoney, false
	}

	return p.Price, true
}

// getPriceDate returns the date of the most recent price for a security on or before asOf.
func (s *Service) getPriceDate(securityID types.ID, asOf types.Date) types.Date {
	if s.priceRepo == nil {
		return types.Date{}
	}

	p, err := s.priceRepo.GetCurrentPrice(securityID, asOf)
	if err != nil {
		return types.Date{}
	}

	return p.Date
}
