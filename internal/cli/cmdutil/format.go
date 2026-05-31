// Package cmdutil holds the shared command infrastructure used across the
// per-noun CLI packages: money formatting, service construction, and the
// common --file guard. It is the hub of the CLI star topology — every noun
// package depends on it, and it depends only on domain packages, never on a
// noun package or on internal/cli itself (which would form a cycle).
package cmdutil

import (
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/types"
)

// FormatMoney formats a Money value with currency symbol.
// Always displays 2 decimal places for currencies.
func FormatMoney(m types.Money, currency string) string {
	// Format with 2 decimal places
	value := fmt.Sprintf("%.2f", m.Float64())

	// Determine symbol and formatting
	var symbol string
	var format string
	switch currency {
	case "USD":
		symbol = "$"
		format = "symbol"
	case "EUR":
		symbol = "€"
		format = "symbol"
	case "GBP":
		symbol = "£"
		format = "symbol"
	default:
		return fmt.Sprintf("%s %s", currency, value)
	}

	if format == "symbol" {
		if m.IsNegative() {
			return fmt.Sprintf("-%s%s", symbol, strings.TrimPrefix(value, "-"))
		}
		return fmt.Sprintf("%s%s", symbol, value)
	}

	return value
}
