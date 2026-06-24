package investment

import (
	"fmt"
	"sort"

	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

// IncomeOnlyPrice is a source=transaction security price whose date carries a
// reinvest_dividend or fee_liquidation but no buy or sell — a price the
// buy/sell-only auto-price policy (see TransactionType.CreatesAutoPrice) would
// no longer create. The `price cleanup` command removes or refetches these
// legacy rows; their per-share value is total_amount÷rounded-shares, unreliable
// for tiny income events.
type IncomeOnlyPrice struct {
	// Price is the security_prices row to clean up.
	Price *price.Price
	// Fallback is the latest recorded price strictly before Price.Date — the
	// price that would value the security on Price.Date once Price is deleted.
	// nil means there is no earlier price, so deletion leaves a gap.
	Fallback *price.Price
}

// ListIncomeOnlyTransactionPrices returns, oldest first, the income-only
// transaction-sourced prices for one security: rows with source=transaction on
// a date that has a reinvest_dividend or fee_liquidation but no buy or sell.
// A buy or sell on the same date justifies the price (a real execution), so it
// is kept; corporate-action `exchange` and `transfer_shares` dates are likewise
// left alone — they are not income events. Used by the `price cleanup` command.
func (s *Service) ListIncomeOnlyTransactionPrices(securityID types.ID) ([]IncomeOnlyPrice, error) {
	history, err := s.priceRepo.GetPriceHistory(securityID, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to load price history for %s: %w", securityID, err)
	}
	txns, err := s.repo.ListBySecurity(securityID)
	if err != nil {
		return nil, fmt.Errorf("failed to load transactions for %s: %w", securityID, err)
	}

	hasExecution := make(map[string]bool) // date string -> a buy/sell exists
	hasIncome := make(map[string]bool)    // date string -> a reinvest/fee-liq exists
	for _, t := range txns {
		switch t.Type {
		case TransactionTypeBuy, TransactionTypeSell:
			hasExecution[t.Date.String()] = true
		case TransactionTypeReinvestDividend, TransactionTypeFeeLiquidation:
			hasIncome[t.Date.String()] = true
		}
	}

	sort.SliceStable(history, func(i, j int) bool {
		return history[i].Date.Time().Before(history[j].Date.Time())
	})

	var out []IncomeOnlyPrice
	for i, p := range history {
		if p.Source != price.SourceTransaction {
			continue
		}
		d := p.Date.String()
		if !hasIncome[d] || hasExecution[d] {
			continue
		}
		// Prices have one row per (security, date), so the immediately preceding
		// history entry is the price valuation falls back to after this row goes.
		var fallback *price.Price
		if i > 0 {
			fallback = history[i-1]
		}
		out = append(out, IncomeOnlyPrice{Price: p, Fallback: fallback})
	}
	return out, nil
}
