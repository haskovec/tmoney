package investment

import (
	"github.com/haskovec/tmoney/internal/types"
)

// AccountValuation represents the total valuation of an investment account.
type AccountValuation struct {
	AccountID     types.ID    `json:"account_id"`
	CashBalance   types.Money `json:"cash_balance"`
	MarketValue   types.Money `json:"market_value"`
	TotalValue    types.Money `json:"total_value"`
	TotalCostBasis types.Money `json:"total_cost_basis"`
	TotalGainLoss types.Money `json:"total_gain_loss"`
	TotalGainPct  float64     `json:"total_gain_pct"`
	Holdings      []Holding   `json:"holdings"`
}

// Holding represents a rolled-up holding of a single security in an account.
type Holding struct {
	SecurityID  types.ID       `json:"security_id"`
	Shares      types.Quantity `json:"shares"`
	AvgCost     types.Money    `json:"avg_cost"`
	CurrentPrice types.Money   `json:"current_price"`
	PriceDate   types.Date     `json:"price_date"`
	MarketValue types.Money    `json:"market_value"`
	CostBasis   types.Money    `json:"cost_basis"`
	GainLoss    types.Money    `json:"gain_loss"`
	GainPct     float64        `json:"gain_pct"`
	HasPricing  bool           `json:"has_pricing"`
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
