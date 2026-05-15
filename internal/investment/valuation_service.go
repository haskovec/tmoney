package investment

import (
	"fmt"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

// GetAccountValuation returns the total valuation of an investment account.
// It computes cash balance + market value of all holdings. Securities with
// no price as of the given date use cost basis as the estimated value.
//
// The returned struct also carries the total-return breakdown
// (RealizedGain, DividendsReceived, InterestReceived, FeesPaid,
// TotalCostDeployed, TotalReturn, TotalReturnPct). RealizedGain and
// DividendsReceived are summed across the per-holding values produced by
// enrichHoldingTotalReturn; InterestReceived, FeesPaid (per-security
// commissions + account-level fee transactions), and TotalCostDeployed are
// pulled from authoritative account-level helpers so closed positions
// contribute even before TR-015 wires them into the holdings list. The
// legacy TotalGainLoss / TotalGainPct fields retain their unrealized-only
// meaning.
//
// When opts.IncludeClosed is true, Holdings additionally contains
// synthesized rows for securities the account has ever held but no longer
// holds (Shares == 0, MarketValue / CostBasis zero, IsClosed = true) with
// total-return components populated from the ledger. Closed-row totals are
// already reflected in the account-level RealizedGain / DividendsReceived
// because those account-level numbers come from authoritative ledger
// helpers (not from summing per-holding values), so adding closed rows to
// the Holdings slice does not change the account-level totals.
//
// HasClosedPositions is set whenever the account has at least one
// fully-sold security, regardless of opts.IncludeClosed — it advises the
// caller that there are closed positions to display.
func (s *Service) GetAccountValuation(accountID types.ID, asOf types.Date, opts ValuationOptions) (*AccountValuation, error) {
	acct, err := s.getInvestmentAccount(accountID)
	if err != nil {
		return nil, err
	}

	cashBalance, err := s.GetCashBalance(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get cash balance: %w", err)
	}

	holdings, err := s.getHoldings(acct, asOf, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get holdings: %w", err)
	}

	marketValue := types.ZeroMoney
	totalCostBasis := types.ZeroMoney
	realizedGain := types.ZeroMoney
	dividendsReceived := types.ZeroMoney
	for _, h := range holdings {
		marketValue = marketValue.Add(h.MarketValue)
		totalCostBasis = totalCostBasis.Add(h.CostBasis)
		realizedGain = realizedGain.Add(h.RealizedGain)
		dividendsReceived = dividendsReceived.Add(h.DividendsReceived)
	}

	totalValue := cashBalance.Add(marketValue)
	totalGainLoss := marketValue.Sub(totalCostBasis)
	totalGainPct := computeGainPct(marketValue, totalCostBasis)

	interestReceived, err := s.sumInterestForAccount(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to sum interest for account: %w", err)
	}
	feesPaid, err := s.sumFeesForAccount(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to sum fees for account: %w", err)
	}
	totalCostDeployed, err := s.totalCostDeployedForAccount(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to sum total cost deployed for account: %w", err)
	}

	// HasClosedPositions advises callers (CLI footer, TUI affordance) that
	// the account has at least one fully-sold security. It is set
	// independently of opts.IncludeClosed: it describes the account's
	// history, not the shape of the returned holdings slice. We count the
	// distinct securities ever held in the ledger and compare against the
	// number of open positions in `holdings` (those without IsClosed).
	openCount := 0
	for _, h := range holdings {
		if !h.IsClosed {
			openCount++
		}
	}
	everHeld, err := s.listEverHeldSecurities(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to list ever-held securities: %w", err)
	}
	closedPositionCount := max(len(everHeld)-openCount, 0)
	hasClosedPositions := closedPositionCount > 0

	totalReturn := totalGainLoss.
		Add(realizedGain).
		Add(dividendsReceived).
		Add(interestReceived).
		Sub(feesPaid)
	var totalReturnPct *float64
	if !totalCostDeployed.IsZero() {
		pct := (totalReturn.Float64() / totalCostDeployed.Float64()) * 100
		totalReturnPct = &pct
	}

	return &AccountValuation{
		AccountID:           accountID,
		CashBalance:         cashBalance,
		MarketValue:         marketValue,
		TotalValue:          totalValue,
		TotalCostBasis:      totalCostBasis,
		TotalGainLoss:       totalGainLoss,
		TotalGainPct:        totalGainPct,
		Holdings:            holdings,
		RealizedGain:        realizedGain,
		DividendsReceived:   dividendsReceived,
		InterestReceived:    interestReceived,
		FeesPaid:            feesPaid,
		TotalCostDeployed:   totalCostDeployed,
		TotalReturn:         totalReturn,
		TotalReturnPct:      totalReturnPct,
		HasClosedPositions:  hasClosedPositions,
		ClosedPositionCount: closedPositionCount,
	}, nil
}

// GetHoldings returns a list of holdings for an investment account, rolled up by security.
// For lot-tracking accounts, lots are aggregated into a single holding per security.
//
// When opts.IncludeClosed is true, the returned slice also contains
// synthesized rows (Shares == 0, IsClosed = true) for securities the
// account has ever held but no longer holds, with total-return components
// populated from the ledger. Otherwise only open positions are returned.
func (s *Service) GetHoldings(accountID types.ID, asOf types.Date, opts ValuationOptions) ([]Holding, error) {
	acct, err := s.getInvestmentAccount(accountID)
	if err != nil {
		return nil, err
	}

	return s.getHoldings(acct, asOf, opts)
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
// When the holdings repository is available, it uses the portfolio_holdings
// database view for better performance. Otherwise falls back to manual computation.
//
// When opts.IncludeClosed is true, after the open-position list is built
// the function appends one synthesized Holding per ever-held security that
// is no longer held (Shares == 0, IsClosed = true, total-return components
// enriched from the ledger).
func (s *Service) getHoldings(acct *account.Account, asOf types.Date, opts ValuationOptions) ([]Holding, error) {
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
func (s *Service) appendClosedHoldings(acct *account.Account, open []Holding) ([]Holding, error) {
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
func (s *Service) getHoldingsFromView(acct *account.Account, asOf types.Date) ([]Holding, error) {
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
func (s *Service) getHoldingsFromPositions(acct *account.Account, asOf types.Date) ([]Holding, error) {
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
func (s *Service) getHoldingsFromLots(acct *account.Account, asOf types.Date) ([]Holding, error) {
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
func (s *Service) enrichHoldingTotalReturn(h *Holding, acct *account.Account) error {
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
