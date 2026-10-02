package tui

import (
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
)

// formatDashboardMoney formats a Money value in dollars for dashboard display.
func formatDashboardMoney(m types.Money) string {
	return m.Format("USD")
}

// formatDashboardMoneyIn formats m in its own currency, with the same rules
// as the CLI (types.Money.Format), so a EUR balance never reads as dollars.
func formatDashboardMoneyIn(m types.Money, currency string) string {
	return m.Format(currency)
}

// valuationOptions returns the ValuationOptions struct that callers
// should pass when requesting an investment account valuation. The
// IncludeClosed flag is sourced from cfg.ShowClosedPositions so the
// View → Show closed positions toggle plumbs through to every
// valuation-bearing view (dashboard cards, register header, portfolio
// holdings list). A nil cfg falls back to IncludeClosed=false so the
// helper is safe to call from tests that don't construct a config.
func (a *App) valuationOptions() investment.ValuationOptions {
	return valuationOptionsFor(a.cfg)
}

// valuationOptionsFor is valuationOptions for a config that a view's deps hand
// it, so a view state can build the options without App.
func valuationOptionsFor(cfg *config.Config) investment.ValuationOptions {
	if cfg == nil {
		return investment.ValuationOptions{}
	}
	return investment.ValuationOptions{IncludeClosed: cfg.ShowClosedPositions}
}
