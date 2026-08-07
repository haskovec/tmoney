package investment

import (
	"fmt"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

// Assembling the holdings list an account valuation is built from.
//
// Which source answers depends on the account: a lot-tracked account sums its
// open lots, a position-only account reads its running position, and the
// holdings view answers when it can. Pricing and total-return enrichment are
// layered on top of whichever one ran.

// getHoldings builds holdings for an account based on its tracking mode.
// When the holdings repository is available, it uses the portfolio_holdings
// database view for better performance. Otherwise falls back to manual computation.
//
// When opts.IncludeClosed is true, after the open-position list is built
// the function appends one synthesized Holding per ever-held security that
// is no longer held (Shares == 0, IsClosed = true, total-return components
// enriched from the ledger).
func (s *ValuationService) getHoldings(acct *account.Account, asOf types.Date, opts ValuationOptions) ([]Holding, error) {
	var (
		holdings []Holding
		err      error
	)
	switch {
	case s.holdingsRepo != nil:
		holdings, err = s.getHoldingsFromView(acct, asOf)
	case acct.TrackLots:
		holdings, err = s.getHoldingsFromLots(acct, asOf)
	default:
		holdings, err = s.getHoldingsFromPositions(acct, asOf)
	}
	if err != nil {
		return nil, err
	}
	if !opts.IncludeClosed {
		return holdings, nil
	}
	return s.appendClosedHoldings(acct, holdings)
}

// appendClosedHoldings synthesizes a Holding row for every security the
// account has ever held but no longer holds, and appends those rows to the
// supplied open-position slice. The closed rows have zero shares / cost
// basis / market value but their total-return components are populated by
// the same enrichment helper used for open positions, so they show
// realized gain, dividends received, fees paid, and total cost deployed.
func (s *ValuationService) appendClosedHoldings(acct *account.Account, open []Holding) ([]Holding, error) {
	everHeld, err := s.listEverHeldSecurities(acct.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list ever-held securities: %w", err)
	}
	if len(everHeld) == 0 {
		return open, nil
	}
	openSet := make(map[types.ID]struct{}, len(open))
	for _, h := range open {
		openSet[h.SecurityID] = struct{}{}
	}
	out := open
	for _, secID := range everHeld {
		if _, ok := openSet[secID]; ok {
			continue
		}
		h := Holding{
			SecurityID: secID,
			Shares:     types.ZeroQuantity,
			IsClosed:   true,
		}
		if err := s.enrichHoldingTotalReturn(&h, acct); err != nil {
			return nil, fmt.Errorf("enrich closed holding %s: %w", secID, err)
		}
		out = append(out, h)
	}
	return out, nil
}

// getHoldingsFromView builds holdings using the portfolio_holdings database view.
// The view handles both lot-tracking and non-lot-tracking accounts, returning
// pre-aggregated shares and cost basis. Pricing is enriched on top.
func (s *ValuationService) getHoldingsFromView(acct *account.Account, asOf types.Date) ([]Holding, error) {
	viewHoldings, err := s.holdingsRepo.ListByAccount(acct.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to query holdings view: %w", err)
	}

	holdings := make([]Holding, 0, len(viewHoldings))
	for _, vh := range viewHoldings {
		currentPrice, hasPricing := s.getCurrentPrice(vh.SecurityID, asOf)
		costBasis := vh.TotalCostBasis

		// Compute average cost per share
		reciprocal := alpacadecimal.NewFromInt(1).Div(vh.TotalShares.Decimal())
		avgCost := costBasis.Mul(reciprocal)

		var marketValue types.Money
		var priceDate types.Date
		if hasPricing {
			marketValue = currentPrice.Mul(vh.TotalShares.Decimal())
			priceDate = s.getPriceDate(vh.SecurityID, asOf)
		} else {
			marketValue = costBasis
		}

		gainLoss := marketValue.Sub(costBasis)

		h := Holding{
			SecurityID:   vh.SecurityID,
			Shares:       vh.TotalShares,
			AvgCost:      avgCost,
			CurrentPrice: currentPrice,
			PriceDate:    priceDate,
			MarketValue:  marketValue,
			CostBasis:    costBasis,
			GainLoss:     gainLoss,
			GainPct:      computeGainPct(marketValue, costBasis),
			HasPricing:   hasPricing,
		}
		if err := s.enrichHoldingTotalReturn(&h, acct); err != nil {
			return nil, err
		}
		holdings = append(holdings, h)
	}

	return holdings, nil
}

// getHoldingsFromPositions builds holdings from positions (non-lot-tracking).
func (s *ValuationService) getHoldingsFromPositions(acct *account.Account, asOf types.Date) ([]Holding, error) {
	positions, err := s.positionRepo.ListByAccount(acct.ID, true)
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

		h := Holding{
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
		}
		if err := s.enrichHoldingTotalReturn(&h, acct); err != nil {
			return nil, err
		}
		holdings = append(holdings, h)
	}

	return holdings, nil
}

// getHoldingsFromLots builds holdings aggregated from lots (lot-tracking).
func (s *ValuationService) getHoldingsFromLots(acct *account.Account, asOf types.Date) ([]Holding, error) {
	// Get all open lots for this account, grouped by security
	// We need to find which securities have lots, then aggregate
	// Use the position repo isn't available for lot-tracking, so we query lots directly

	// Get all investment transactions to find which securities are held
	txns, err := s.repo.ListByAccount(acct.ID, TransactionFilter{})
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
		lots, err := s.lotRepo.ListByAccountAndSecurity(acct.ID, secID, false)
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

		h := Holding{
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
		}
		if err := s.enrichHoldingTotalReturn(&h, acct); err != nil {
			return nil, err
		}
		holdings = append(holdings, h)
	}

	return holdings, nil
}

// getCurrentPrice returns the most recent price for a security on or before the given date.
// Returns the price and true if found, or ZeroMoney and false if not found.
func (s *ValuationService) getCurrentPrice(securityID types.ID, asOf types.Date) (types.Money, bool) {
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
func (s *ValuationService) getPriceDate(securityID types.ID, asOf types.Date) types.Date {
	if s.priceRepo == nil {
		return types.Date{}
	}

	p, err := s.priceRepo.GetCurrentPrice(securityID, asOf)
	if err != nil {
		return types.Date{}
	}

	return p.Date
}

// enrichHoldingTotalReturn populates the total-return breakdown on a holding:
// RealizedGain, DividendsReceived, FeesPaid, TotalCostDeployed, TotalReturn,
// TotalReturnPct, and RealizedGainUnavailable. Per-security interest is not
// modeled — interest is account-level only and applied in GetAccountValuation.
//
// TotalReturn = GainLoss (unrealized) + RealizedGain + DividendsReceived − FeesPaid.
// TotalReturnPct = TotalReturn / TotalCostDeployed × 100, or nil when no
// capital has been deployed (e.g., shares received only via transfer_shares).
//
// Realized gain is dispatched between the lot-tracked and non-lot paths via
// acct.TrackLots. When the non-lot path is gated by corporate actions, the
// dispatcher returns (zero, unavailable=true) and the flag is forwarded onto
// the holding so the UI can render `(unavailable)`.
func (s *ValuationService) enrichHoldingTotalReturn(h *Holding, acct *account.Account) error {
	realized, unavailable, err := s.realizedGain(acct.ID, h.SecurityID, acct.TrackLots)
	if err != nil {
		return fmt.Errorf("realized gain for %s: %w", h.SecurityID, err)
	}
	dividends, err := s.sumDividendsForSecurity(acct.ID, h.SecurityID)
	if err != nil {
		return fmt.Errorf("dividends for %s: %w", h.SecurityID, err)
	}
	fees, err := s.sumFeesForSecurity(acct.ID, h.SecurityID)
	if err != nil {
		return fmt.Errorf("fees for %s: %w", h.SecurityID, err)
	}
	deployed, err := s.totalCostDeployedForSecurity(acct.ID, h.SecurityID)
	if err != nil {
		return fmt.Errorf("total cost deployed for %s: %w", h.SecurityID, err)
	}

	h.RealizedGain = realized
	h.RealizedGainUnavailable = unavailable
	h.DividendsReceived = dividends
	h.FeesPaid = fees
	h.TotalCostDeployed = deployed
	h.TotalReturn = h.GainLoss.Add(realized).Add(dividends).Sub(fees)

	if !deployed.IsZero() {
		pct := (h.TotalReturn.Float64() / deployed.Float64()) * 100
		h.TotalReturnPct = &pct
	}
	return nil
}
