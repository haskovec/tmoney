package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// The Prices key hints depend on the view's mode, and the mode changes
// without a view switch: Enter on the list opens a security's history, Esc
// goes back, and the drill from Securities loads straight into the history.
// The status bar must follow each change.
func TestPriceView_StatusBarHintsFollowTheMode(t *testing.T) {
	const listHint, detailHint = "enter view history", "i import"
	sec := security.NewSecurity("FABR", "Fabrikam Inc.", security.TypeStock)
	app := &App{
		currentView: ViewPrices,
		width:       120,
		height:      40,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
	}
	app.updateStatusBar()
	wantHint := func(step, hint string) {
		t.Helper()
		if got := app.statusbar.KeyHints(); !strings.Contains(got, hint) {
			t.Errorf("%s: status-bar hints = %q, want them to contain %q", step, got, hint)
		}
	}

	app.Update(priceViewDataLoadedMsg{data: &priceViewData{
		mode:         pricesViewList,
		securities:   []*security.Security{sec},
		latestPrices: []*price.LatestPrice{{SecurityID: sec.ID, Ticker: "FABR", Name: "Fabrikam Inc."}},
	}})
	wantHint("list loaded", listHint)

	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if app.priceView.mode != pricesViewDetail {
		t.Fatalf("Enter on the list did not open the history (mode %v)", app.priceView.mode)
	}
	wantHint("Enter on the list", detailHint)

	app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.priceView.mode != pricesViewList {
		t.Fatalf("Esc in the history did not go back to the list (mode %v)", app.priceView.mode)
	}
	wantHint("Esc in the history", listHint)

	app.Update(priceViewDataLoadedMsg{data: &priceViewData{
		mode:             pricesViewDetail,
		selectedSecurity: sec,
		securities:       []*security.Security{sec},
		prices:           []*price.Price{},
	}})
	wantHint("history loaded (the drill from Securities)", detailHint)
}
