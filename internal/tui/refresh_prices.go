package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haskovec/tmoney/internal/price"
)

// defaultRefreshProviderName is the provider used by the TUI's "u"
// shortcut on the securities view. Tests register a fake provider under
// this name to exercise the refresh flow without hitting the network.
const defaultRefreshProviderName = "yahoo"

// priceRefreshCompleteMsg is dispatched after RefreshPrices returns so
// the main Update loop can update the status bar.
type priceRefreshCompleteMsg struct {
	result *price.RefreshResult
	err    error
}

// refreshPricesCmd returns a tea.Cmd that calls
// priceSvc.RefreshPrices for all visible securities using the default
// provider. The result is delivered as a priceRefreshCompleteMsg.
func (a *App) refreshPricesCmd() tea.Cmd {
	if a.priceSvc == nil {
		return func() tea.Msg {
			return priceRefreshCompleteMsg{err: fmt.Errorf("price service not available")}
		}
	}
	return func() tea.Msg {
		result, err := a.priceSvc.RefreshPrices(defaultRefreshProviderName, nil)
		return priceRefreshCompleteMsg{result: result, err: err}
	}
}

// summarizeRefreshResult builds a one-line status-bar summary of a
// completed RefreshPrices run, including up to the first three failure
// tickers when present.
func summarizeRefreshResult(result *price.RefreshResult) string {
	counts := result.CountByOutcome()
	skipped := counts[price.OutcomeSkippedHidden] +
		counts[price.OutcomeSkippedNoTicker] +
		counts[price.OutcomeSkippedCurrency]

	summary := fmt.Sprintf(
		"Prices: %d updated, %d up-to-date, %d skipped, %d failed",
		counts[price.OutcomeUpdated],
		counts[price.OutcomeUpToDate],
		skipped,
		counts[price.OutcomeFailed],
	)

	if counts[price.OutcomeFailed] > 0 {
		var failures []string
		for _, e := range result.Entries {
			if e.Outcome != price.OutcomeFailed {
				continue
			}
			failures = append(failures, e.Ticker)
			if len(failures) == 3 {
				break
			}
		}
		summary += " (" + strings.Join(failures, ", ")
		if counts[price.OutcomeFailed] > len(failures) {
			summary += ", ..."
		}
		summary += ")"
	}
	return summary
}
