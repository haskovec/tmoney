package tui

import (
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/types"
)

// formatDashboardMoney formats a Money value with $ prefix for dashboard display.
func formatDashboardMoney(m types.Money) string {
	value := fmt.Sprintf("%.2f", m.Float64())
	if m.IsNegative() {
		return fmt.Sprintf("-$%s", strings.TrimPrefix(value, "-"))
	}
	return fmt.Sprintf("$%s", value)
}

// formatDashboardMoneyIn formats m in its currency: "$" for USD (or no
// currency), and the currency code for any other, so a EUR balance never
// reads as dollars.
func formatDashboardMoneyIn(m types.Money, currency string) string {
	if currency == "" || currency == "USD" {
		return formatDashboardMoney(m)
	}
	return fmt.Sprintf("%s %.2f", currency, m.Float64())
}
