package tui

import (
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
