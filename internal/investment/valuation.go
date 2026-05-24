package investment

import (
	"github.com/haskovec/tmoney/internal/types"
)

// ValuationOptions controls optional behavior of valuation entry points.
type ValuationOptions struct {
	// IncludeClosed, when true, causes Holdings to include rows for
	// securities the account no longer holds (Shares == 0) with
	// total-return components populated. Open-position behavior is
	// unchanged.
	IncludeClosed bool
}

// AccountValuation represents the total valuation of an investment account.
type AccountValuation struct {
	AccountID      types.ID    `json:"account_id"`
	CashBalance    types.Money `json:"cash_balance"`
	MarketValue    types.Money `json:"market_value"`
	TotalValue     types.Money `json:"total_value"`
	TotalCostBasis types.Money `json:"total_cost_basis"`
	TotalGainLoss  types.Money `json:"total_gain_loss"`
	TotalGainPct   float64     `json:"total_gain_pct"`
	Holdings       []Holding   `json:"holdings"`

	// Total-return breakdown (see specs/investment-total-return.md).
	// TotalGainLoss / TotalGainPct retain their unrealized-only meaning.
	RealizedGain        types.Money `json:"realized_gain"`
	DividendsReceived   types.Money `json:"dividends_received"`
	InterestReceived    types.Money `json:"interest_received"`
	FeesPaid            types.Money `json:"fees_paid"`
	TotalCostDeployed   types.Money `json:"total_cost_deployed"`
	TotalReturn         types.Money `json:"total_return"`
	TotalReturnPct      *float64    `json:"total_return_pct,omitempty"`
	HasClosedPositions  bool        `json:"has_closed_positions"`
	ClosedPositionCount int         `json:"closed_position_count"`

	// AnyRealizedUnavailable is true when at least one of the
	// contributing holdings has RealizedGainUnavailable=true (a non-lot
	// security with a corporate action on file, where the chronological
	// replay can't produce a reliable number). When set, RealizedGain
	// (and TotalReturn, which sums it) is a partial figure — the UI
	// should mark it accordingly so the user knows the value is
	// incomplete rather than a true zero.
	AnyRealizedUnavailable bool `json:"any_realized_unavailable,omitempty"`
}

// Holding represents a rolled-up holding of a single security in an account.
type Holding struct {
	SecurityID   types.ID       `json:"security_id"`
	Shares       types.Quantity `json:"shares"`
	AvgCost      types.Money    `json:"avg_cost"`
	CurrentPrice types.Money    `json:"current_price"`
	PriceDate    types.Date     `json:"price_date"`
	MarketValue  types.Money    `json:"market_value"`
	CostBasis    types.Money    `json:"cost_basis"`
	GainLoss     types.Money    `json:"gain_loss"`
	GainPct      float64        `json:"gain_pct"`
	HasPricing   bool           `json:"has_pricing"`

	// Total-return breakdown for this (account, security). GainLoss /
	// GainPct above retain their unrealized-only meaning.
	RealizedGain            types.Money `json:"realized_gain"`
	DividendsReceived       types.Money `json:"dividends_received"`
	FeesPaid                types.Money `json:"fees_paid"`
	TotalCostDeployed       types.Money `json:"total_cost_deployed"`
	TotalReturn             types.Money `json:"total_return"`
	TotalReturnPct          *float64    `json:"total_return_pct,omitempty"`
	IsClosed                bool        `json:"is_closed"`
	RealizedGainUnavailable bool        `json:"realized_gain_unavailable"`
}

// LotDetail represents a single lot's valuation detail for lot-tracking accounts.
type LotDetail struct {
	LotID        types.ID       `json:"lot_id"`
	PurchaseDate types.Date     `json:"purchase_date"`
	Shares       types.Quantity `json:"shares"`
	CostPerShare types.Money    `json:"cost_per_share"`
	CostBasis    types.Money    `json:"cost_basis"`
	CurrentValue types.Money    `json:"current_value"`
	GainLoss     types.Money    `json:"gain_loss"`
	GainPct      float64        `json:"gain_pct"`
}

// computeGainPct calculates gain/loss percentage from market value and cost basis.
// Returns 0 if cost basis is zero to avoid division by zero.
func computeGainPct(marketValue, costBasis types.Money) float64 {
	if costBasis.IsZero() {
		return 0
	}
	gainLoss := marketValue.Sub(costBasis)
	return (gainLoss.Float64() / costBasis.Float64()) * 100
}
