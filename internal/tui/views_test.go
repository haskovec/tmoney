package tui

import (
	"testing"

	"github.com/haskovec/tmoney/internal/tui/widget"
)

// Prices and Portfolio each have two tables, and the one the mouse and the
// wheel address depends on the view's mode.
func TestActiveTable_PicksByMode(t *testing.T) {
	list, detail := widget.NewTable(nil), widget.NewTable(nil)
	holdings, lots := widget.NewTable(nil), widget.NewTable(nil)

	cases := []struct {
		name  string
		view  View
		setup func(a *App)
		want  *widget.Table
	}{
		{"prices list", ViewPrices, func(a *App) { a.priceView = &priceViewData{mode: pricesViewList} }, list},
		{"prices detail", ViewPrices, func(a *App) { a.priceView = &priceViewData{mode: pricesViewDetail} }, detail},
		{"prices before load", ViewPrices, func(a *App) {}, detail},
		{"portfolio holdings", ViewPortfolio, func(a *App) {
			a.portfolioData = &portfolioViewData{}
			a.portfolioMode = portfolioViewHoldings
		}, holdings},
		{"portfolio lots", ViewPortfolio, func(a *App) {
			a.portfolioData = &portfolioViewData{}
			a.portfolioMode = portfolioViewLots
		}, lots},
		{"portfolio before load", ViewPortfolio, func(a *App) {}, nil},
		{"dashboard has none", ViewDashboard, func(a *App) {}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &App{
				currentView:            tc.view,
				priceListTable:         list,
				priceTable:             detail,
				portfolioHoldingsTable: holdings,
				portfolioLotsTable:     lots,
			}
			tc.setup(a)
			if got := a.activeTable(); got != tc.want {
				t.Errorf("activeTable() = %p, want %p", got, tc.want)
			}
		})
	}
}
