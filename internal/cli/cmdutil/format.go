// Package cmdutil holds the shared command infrastructure used across the
// per-noun CLI packages: money formatting, service construction, and the
// common --file guard. It is the hub of the CLI star topology — every noun
// package depends on it, and it depends only on domain packages, never on a
// noun package or on internal/cli itself (which would form a cycle).
package cmdutil

import (
	"github.com/haskovec/tmoney/internal/types"
)

// FormatMoney formats a Money value for display in currency. It is
// types.Money.Format, the one formatter the CLI and the TUI share.
func FormatMoney(m types.Money, currency string) string {
	return m.Format(currency)
}
