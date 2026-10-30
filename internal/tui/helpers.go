package tui

import (
	"slices"
	"unicode/utf8"

	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/tui/widget"
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

// fitWidth is the width of a column that cuts none of its cells: its usual
// width, or its widest cell when that is wider. A money column needs more
// than its usual width for a currency code: "JPY 1500000.00" is 14 cells.
// A cut amount would lose digits.
func fitWidth(usual int, cells ...string) int {
	w := usual
	for _, c := range cells {
		w = max(w, utf8.RuneCountInString(c))
	}
	return w
}

// fitNumberColumns returns cols with each fixed, right-aligned column as wide
// as its widest cell in rows, and never narrower than its usual width. Those
// columns hold amounts, prices, share counts and percentages. A cell wider
// than its column is cut with "…" and loses digits: "-$123456789.00" in 12
// cells shows as "-$123456789…".
func fitNumberColumns(cols []widget.Column, rows [][]string) []widget.Column {
	out := slices.Clone(cols)
	for i, c := range out {
		if c.Width == 0 || c.Align != widget.AlignRight {
			continue
		}
		for _, row := range rows {
			if i < len(row) {
				out[i].Width = fitWidth(out[i].Width, row[i])
			}
		}
	}
	return out
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
