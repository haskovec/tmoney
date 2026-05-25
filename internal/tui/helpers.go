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
