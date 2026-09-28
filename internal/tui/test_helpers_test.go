package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/tui/theme"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// restoreDefaultTheme reapplies the embedded default theme. Use as a
// t.Cleanup so a test that calls ApplyTheme doesn't leak palette state
// into the widget package-level Color* vars and confuse later tests.
func restoreDefaultTheme(t *testing.T) {
	t.Helper()
	def, _, err := theme.LoadBuiltin("default")
	if err != nil {
		t.Fatalf("restoreDefaultTheme: load default: %v", err)
	}
	s := widget.NewStyles()
	s.ApplyTheme(def)
}

// runCmd runs cmd and feeds each message it yields back into the app, a few
// levels deep, the way the Bubble Tea loop would. An errMsg fails the test.
func runCmd(t *testing.T, a *App, cmd tea.Cmd, depth int) {
	t.Helper()
	if cmd == nil || depth == 0 {
		return
	}
	switch msg := cmd().(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			runCmd(t, a, c, depth)
		}
	case errMsg:
		t.Fatalf("command failed: %v", msg.err)
	default:
		_, next := a.Update(msg)
		runCmd(t, a, next, depth-1)
	}
}

// paramDec parses a test constant into an investment.ParamDecimal. It panics
// on a typo, the way types.MustNewMoney does.
func paramDec(s string) investment.ParamDecimal {
	d, err := investment.ParseParamDecimal(s)
	if err != nil {
		panic(err)
	}
	return d
}

// usdTotals is a one-currency net-worth total, every part available, for
// tests that build a report.NetWorth by hand.
func usdTotals(assets, liabilities, netWorth types.Money) []report.CurrencyTotal {
	return []report.CurrencyTotal{{
		Currency: "USD", Assets: assets, Liabilities: liabilities, NetWorth: netWorth,
		Available: true, AssetsAvailable: true, LiabilitiesAvailable: true,
	}}
}
