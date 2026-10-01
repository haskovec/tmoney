package tui

import (
	"testing"

	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
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
		{"prices list", ViewPrices, func(a *App) { a.prices.data = &priceViewData{mode: pricesViewList} }, list},
		{"prices detail", ViewPrices, func(a *App) { a.prices.data = &priceViewData{mode: pricesViewDetail} }, detail},
		{"prices before load", ViewPrices, func(a *App) {}, detail},
		{"portfolio holdings", ViewPortfolio, func(a *App) {
			a.portfolio.data = &portfolioViewData{}
			a.portfolio.mode = portfolioViewHoldings
		}, holdings},
		{"portfolio lots", ViewPortfolio, func(a *App) {
			a.portfolio.data = &portfolioViewData{}
			a.portfolio.mode = portfolioViewLots
		}, lots},
		{"portfolio before load", ViewPortfolio, func(a *App) {}, nil},
		{"dashboard has none", ViewDashboard, func(a *App) {}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &App{
				currentView: tc.view,
				prices: priceViewState{
					listTable: list,
					table:     detail,
				},
				portfolio: portfolioViewState{
					holdingsTable: holdings,
					lotsTable:     lots,
				},
			}
			tc.setup(a)
			if got := a.activeTable(); got != tc.want {
				t.Errorf("activeTable() = %p, want %p", got, tc.want)
			}
		})
	}
}

// Each view's lookup calls these funcs without a nil check, so a missing one
// is a panic on first use. reload matters most: W2 gave Reconciliation and
// Corporate Actions the reload they lacked, and a nil one would undo that.
// table is not here, because a view with no table leaves it nil, and nor is
// leave, because a view that keeps all its state on departure has none.
func TestViews_EveryEntryHasItsFuncs(t *testing.T) {
	for _, e := range views() {
		for field, missing := range map[string]bool{
			"render":    e.render == nil,
			"onKey":     e.onKey == nil,
			"hints":     e.hints == nil,
			"shortcuts": e.shortcuts == nil,
			"reload":    e.reload == nil,
			"focus":     e.focus == nil,
		} {
			if missing {
				t.Errorf("%s has no %s", e.name, field)
			}
		}
	}
}

// A view's tables are built when its data arrives, so switchView can reach a
// view whose tables are all nil, as on a fresh database. Each focus func must
// keep its nil checks. The App has a sidebar, a status bar, and styles,
// because without a sidebar switchView skips focus, and without a status bar
// it panics before it gets there.
func TestSwitchView_AllTablesNil(t *testing.T) {
	consts := viewConstants(t)
	for i, vc := range consts {
		t.Run(vc.Name, func(t *testing.T) {
			a := &App{sidebar: NewSidebar(), statusbar: widget.NewStatusBar(), styles: widget.NewStyles()}
			a.currentView = consts[(i+1)%len(consts)].Value
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("switchView(%s) with every table nil panicked: %v", vc.Name, r)
				}
			}()
			a.switchView(vc.Value)
			if a.currentView != vc.Value {
				t.Errorf("currentView = %s, want %s", a.currentView, vc.Value)
			}
		})
	}
}

// The investment register's leave hook resets all three filter fields when
// switchView moves to another view. A switchView to the view already on
// screen is not a departure and keeps them.
func TestSwitchView_LeavingTheInvestmentRegisterClearsItsFilter(t *testing.T) {
	locked := types.NewID()
	filtered := func() *App {
		a := &App{
			sidebar:     NewSidebar(),
			statusbar:   widget.NewStatusBar(),
			styles:      widget.NewStyles(),
			currentView: ViewInvestmentRegister,
		}
		a.investmentRegister.filterSearching = true
		a.investmentRegister.filterQuery = "fx"
		a.investmentRegister.filterLockedSec = locked
		return a
	}

	a := filtered()
	a.switchView(ViewInvestmentRegister)
	if got := a.investmentRegister; !got.filterSearching || got.filterQuery != "fx" || got.filterLockedSec != locked {
		t.Errorf("switchView to the view on screen changed the filter: searching=%v query=%q locked=%v",
			got.filterSearching, got.filterQuery, got.filterLockedSec)
	}

	a = filtered()
	a.switchView(ViewDashboard)
	if got := a.investmentRegister; got.filterSearching || got.filterQuery != "" || got.filterLockedSec != types.NilID {
		t.Errorf("leaving the investment register kept its filter: searching=%v query=%q locked=%v",
			got.filterSearching, got.filterQuery, got.filterLockedSec)
	}
}
