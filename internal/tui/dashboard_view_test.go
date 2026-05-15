package tui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

func TestApp_RenderDashboard_Loading(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		width:       100,
		height:      30,
		styles:      NewStyles(),
		dashboard:   nil, // not loaded yet
	}
	app.styles.Resize(100, 30)

	view := app.renderDashboard()
	if !contains(view, "Loading") {
		t.Errorf("renderDashboard() should show loading when data is nil, got: %q", view)
	}
}

func TestApp_RenderDashboard_WithData(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 30)
	app := &App{
		currentView: ViewDashboard,
		width:       120,
		height:      30,
		styles:      styles,
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets: []report.AccountBalance{
					{Name: "Checking", Balance: types.MustNewMoney("5000.00")},
					{Name: "Savings", Balance: types.MustNewMoney("10000.00")},
				},
				Liabilities: []report.AccountBalance{
					{Name: "Visa", Balance: types.MustNewMoney("1500.00")},
				},
				TotalAssets:      types.MustNewMoney("15000.00"),
				TotalLiabilities: types.MustNewMoney("1500.00"),
				NetWorth:         types.MustNewMoney("13500.00"),
			},
			dueTxns:      nil,
			upcomingTxns: nil,
			payeeNames:   make(map[types.ID]string),
			accountNames: make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	// Check that key elements are present
	if !contains(view, "DASHBOARD") {
		t.Error("renderDashboard() should contain 'DASHBOARD'")
	}
	if !contains(view, "$13500.00") {
		t.Error("renderDashboard() should contain net worth '$13500.00'")
	}
	if !contains(view, "ASSETS") {
		t.Error("renderDashboard() should contain 'ASSETS'")
	}
	if !contains(view, "LIABILITIES") {
		t.Error("renderDashboard() should contain 'LIABILITIES'")
	}
	if !contains(view, "Checking") {
		t.Error("renderDashboard() should contain 'Checking'")
	}
	if !contains(view, "Savings") {
		t.Error("renderDashboard() should contain 'Savings'")
	}
	if !contains(view, "Visa") {
		t.Error("renderDashboard() should contain 'Visa'")
	}
	if !contains(view, "SCHEDULED") {
		t.Error("renderDashboard() should contain 'SCHEDULED'")
	}
}

func TestApp_RenderDashboard_NegativeNetWorth(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewDashboard,
		width:       100,
		height:      30,
		styles:      styles,
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets:           nil,
				Liabilities:      []report.AccountBalance{{Name: "Loan", Balance: types.MustNewMoney("5000.00")}},
				TotalAssets:      types.MustNewMoney("0"),
				TotalLiabilities: types.MustNewMoney("5000.00"),
				NetWorth:         types.MustNewMoney("-5000.00"),
			},
			payeeNames:   make(map[types.ID]string),
			accountNames: make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	if !contains(view, "-$5000.00") {
		t.Error("renderDashboard() should show negative net worth")
	}
	if !contains(view, "(none)") {
		t.Error("renderDashboard() should show '(none)' for empty assets")
	}
}

func TestApp_RenderDashboard_WithScheduled(t *testing.T) {
	payeeID := types.NewID()
	styles := NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewDashboard,
		width:       100,
		height:      30,
		styles:      styles,
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				TotalAssets:      types.MustNewMoney("1000"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("1000"),
			},
			dueTxns: []*scheduled.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					PayeeID:   types.NullableID{ID: payeeID, Valid: true},
					Amount:    types.NullableMoney{Money: types.MustNewMoney("-1500.00"), Valid: true},
					NextDate:  types.Today(),
				},
			},
			upcomingTxns: nil,
			payeeNames:   map[types.ID]string{payeeID: "Landlord"},
			accountNames: make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	if !contains(view, "1 due") {
		t.Error("renderDashboard() should show '1 due' in scheduled header")
	}
	if !contains(view, "Landlord") {
		t.Error("renderDashboard() should show payee name 'Landlord'")
	}
	if !contains(view, "$1500.00") {
		t.Error("renderDashboard() should show amount '$1500.00'")
	}
	if !contains(view, "due today") {
		t.Error("renderDashboard() should show 'due today'")
	}
}

func TestApp_RenderDashboard_EmptyData(t *testing.T) {
	styles := NewStyles()
	styles.Resize(80, 24)
	app := &App{
		currentView: ViewDashboard,
		width:       80,
		height:      24,
		styles:      styles,
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				TotalAssets:      types.ZeroMoney,
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.ZeroMoney,
			},
			payeeNames:   make(map[types.ID]string),
			accountNames: make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	if !contains(view, "$0.00") {
		t.Error("renderDashboard() should show '$0.00' for zero net worth")
	}
	if !contains(view, "No scheduled") {
		t.Error("renderDashboard() should show 'No scheduled' when there are none")
	}
}

func TestApp_Update_DashboardLoaded(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	data := &dashboardData{
		netWorth: &report.NetWorth{
			NetWorth: types.MustNewMoney("5000"),
		},
		payeeNames:   make(map[types.ID]string),
		accountNames: make(map[types.ID]string),
	}

	msg := dashboardLoadedMsg{data: data}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	if cmd != nil {
		t.Error("dashboardLoadedMsg should not return a command")
	}
	if updatedApp.dashboard == nil {
		t.Fatal("dashboard data should be set")
	}
	if !updatedApp.dashboard.netWorth.NetWorth.Equal(types.MustNewMoney("5000")) {
		t.Error("dashboard net worth should be $5000")
	}
}

func TestApp_RenderDashboard_SmallWidth(t *testing.T) {
	styles := NewStyles()
	styles.Resize(60, 20)
	app := &App{
		currentView: ViewDashboard,
		width:       60,
		height:      20,
		styles:      styles,
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets:           []report.AccountBalance{{Name: "Checking", Balance: types.MustNewMoney("100")}},
				TotalAssets:      types.MustNewMoney("100"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("100"),
			},
			payeeNames:   make(map[types.ID]string),
			accountNames: make(map[types.ID]string),
		},
	}

	// Should not panic on small width
	view := app.renderDashboard()
	if view == "" {
		t.Error("renderDashboard() should not return empty string on small width")
	}
}

func TestApp_RenderDashboard_NilNetWorth(t *testing.T) {
	styles := NewStyles()
	styles.Resize(80, 24)
	app := &App{
		currentView: ViewDashboard,
		width:       80,
		height:      24,
		styles:      styles,
		dashboard: &dashboardData{
			netWorth:     nil,
			payeeNames:   make(map[types.ID]string),
			accountNames: make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	if !contains(view, "No account data available") {
		t.Error("renderDashboard() should show 'No account data available' when netWorth is nil")
	}
}

func TestApp_RenderDashboard_InvestmentAccountWithHoldings(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 40)

	investAccountID := types.NewID()
	securityID1 := types.NewID()
	securityID2 := types.NewID()

	app := &App{
		currentView:               ViewDashboard,
		width:                     120,
		height:                    40,
		styles:                    styles,
		dashboardExpandedAccounts: map[types.ID]bool{investAccountID: true},
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets: []report.AccountBalance{
					{AccountID: types.NewID(), Name: "Checking", Type: "checking", Balance: types.MustNewMoney("5000.00")},
					{AccountID: investAccountID, Name: "Brokerage", Type: "investment", Balance: types.MustNewMoney("25000.00")},
				},
				Liabilities:      nil,
				TotalAssets:      types.MustNewMoney("30000.00"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("30000.00"),
			},
			investmentHoldings: map[types.ID]*investment.AccountValuation{
				investAccountID: {
					AccountID:   investAccountID,
					CashBalance: types.MustNewMoney("5000.00"),
					MarketValue: types.MustNewMoney("20000.00"),
					TotalValue:  types.MustNewMoney("25000.00"),
					Holdings: []investment.Holding{
						{SecurityID: securityID1, MarketValue: types.MustNewMoney("12000.00"), HasPricing: true},
						{SecurityID: securityID2, MarketValue: types.MustNewMoney("8000.00"), HasPricing: true},
					},
				},
			},
			securityTickers: map[types.ID]string{
				securityID1: "AAPL",
				securityID2: "GOOG",
			},
			payeeNames:   make(map[types.ID]string),
			accountNames: make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	// Investment account should show total value
	if !contains(view, "Brokerage") {
		t.Error("dashboard should show investment account name 'Brokerage'")
	}
	if !contains(view, "$25000.00") {
		t.Error("dashboard should show investment account total value '$25000.00'")
	}

	// When expanded, top holdings should be visible
	if !contains(view, "AAPL") {
		t.Error("dashboard should show holding ticker 'AAPL' when expanded")
	}
	if !contains(view, "GOOG") {
		t.Error("dashboard should show holding ticker 'GOOG' when expanded")
	}
	if !contains(view, "$12000.00") {
		t.Error("dashboard should show AAPL market value '$12000.00'")
	}
	if !contains(view, "$8000.00") {
		t.Error("dashboard should show GOOG market value '$8000.00'")
	}

	// Regular accounts should still display normally
	if !contains(view, "Checking") {
		t.Error("dashboard should show regular account 'Checking'")
	}
}

func TestApp_RenderDashboard_InvestmentAccountCollapsed(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 40)

	investAccountID := types.NewID()
	securityID := types.NewID()

	app := &App{
		currentView:               ViewDashboard,
		width:                     120,
		height:                    40,
		styles:                    styles,
		dashboardExpandedAccounts: map[types.ID]bool{}, // not expanded
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets: []report.AccountBalance{
					{AccountID: investAccountID, Name: "Brokerage", Type: "investment", Balance: types.MustNewMoney("25000.00")},
				},
				TotalAssets:      types.MustNewMoney("25000.00"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("25000.00"),
			},
			investmentHoldings: map[types.ID]*investment.AccountValuation{
				investAccountID: {
					AccountID:   investAccountID,
					CashBalance: types.MustNewMoney("5000.00"),
					MarketValue: types.MustNewMoney("20000.00"),
					TotalValue:  types.MustNewMoney("25000.00"),
					Holdings: []investment.Holding{
						{SecurityID: securityID, MarketValue: types.MustNewMoney("20000.00"), HasPricing: true},
					},
				},
			},
			securityTickers: map[types.ID]string{securityID: "AAPL"},
			payeeNames:      make(map[types.ID]string),
			accountNames:    make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	// Account total should show
	if !contains(view, "Brokerage") {
		t.Error("dashboard should show investment account name even when collapsed")
	}
	if !contains(view, "$25000.00") {
		t.Error("dashboard should show investment total value even when collapsed")
	}

	// Holdings should NOT show when collapsed
	if contains(view, "AAPL") {
		t.Error("dashboard should NOT show holding tickers when collapsed")
	}
}

func TestApp_RenderDashboard_InvestmentAccountEstimatedValue(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 40)

	investAccountID := types.NewID()
	securityID := types.NewID()

	app := &App{
		currentView:               ViewDashboard,
		width:                     120,
		height:                    40,
		styles:                    styles,
		dashboardExpandedAccounts: map[types.ID]bool{investAccountID: true},
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets: []report.AccountBalance{
					{AccountID: investAccountID, Name: "401k", Type: "investment", Balance: types.MustNewMoney("10000.00"), EstimatedValue: true},
				},
				TotalAssets:      types.MustNewMoney("10000.00"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("10000.00"),
			},
			investmentHoldings: map[types.ID]*investment.AccountValuation{
				investAccountID: {
					AccountID:   investAccountID,
					CashBalance: types.ZeroMoney,
					MarketValue: types.MustNewMoney("10000.00"),
					TotalValue:  types.MustNewMoney("10000.00"),
					Holdings: []investment.Holding{
						{SecurityID: securityID, MarketValue: types.MustNewMoney("10000.00"), HasPricing: false},
					},
				},
			},
			securityTickers: map[types.ID]string{securityID: "VXUS"},
			payeeNames:      make(map[types.ID]string),
			accountNames:    make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	// Estimated value indicator should show
	if !contains(view, "~$10000.00") {
		t.Error("dashboard should show estimated value prefix '~' for accounts with missing pricing")
	}
	// Holding without pricing should show tilde
	if !contains(view, "~$10000.00") {
		t.Error("holding without pricing should show estimated value indicator")
	}
}

func TestApp_RenderDashboard_InvestmentNoHoldings(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 40)

	investAccountID := types.NewID()

	app := &App{
		currentView:               ViewDashboard,
		width:                     120,
		height:                    40,
		styles:                    styles,
		dashboardExpandedAccounts: map[types.ID]bool{investAccountID: true},
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets: []report.AccountBalance{
					{AccountID: investAccountID, Name: "Empty Fund", Type: "investment", Balance: types.MustNewMoney("1000.00")},
				},
				TotalAssets:      types.MustNewMoney("1000.00"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("1000.00"),
			},
			investmentHoldings: map[types.ID]*investment.AccountValuation{
				investAccountID: {
					AccountID:   investAccountID,
					CashBalance: types.MustNewMoney("1000.00"),
					MarketValue: types.ZeroMoney,
					TotalValue:  types.MustNewMoney("1000.00"),
					Holdings:    nil,
				},
			},
			securityTickers: make(map[types.ID]string),
			payeeNames:      make(map[types.ID]string),
			accountNames:    make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	// Should show cash only note when expanded but no holdings
	if !contains(view, "Empty Fund") {
		t.Error("dashboard should show investment account name")
	}
	if !contains(view, "cash only") {
		t.Error("dashboard should show 'cash only' when investment account has no holdings")
	}
}

func TestApp_RenderDashboard_InvestmentTopHoldingsLimit(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 40)

	investAccountID := types.NewID()

	// Create 7 holdings - only top 5 should show
	holdings := make([]investment.Holding, 7)
	tickers := make(map[types.ID]string)
	for i := range 7 {
		secID := types.NewID()
		holdings[i] = investment.Holding{
			SecurityID:  secID,
			MarketValue: types.MustNewMoney(fmt.Sprintf("%d.00", (7-i)*1000)),
			HasPricing:  true,
		}
		tickers[secID] = fmt.Sprintf("STK%d", i+1)
	}

	app := &App{
		currentView:               ViewDashboard,
		width:                     120,
		height:                    40,
		styles:                    styles,
		dashboardExpandedAccounts: map[types.ID]bool{investAccountID: true},
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets: []report.AccountBalance{
					{AccountID: investAccountID, Name: "Big Portfolio", Type: "investment", Balance: types.MustNewMoney("28000.00")},
				},
				TotalAssets:      types.MustNewMoney("28000.00"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("28000.00"),
			},
			investmentHoldings: map[types.ID]*investment.AccountValuation{
				investAccountID: {
					AccountID: investAccountID,
					Holdings:  holdings,
				},
			},
			securityTickers: tickers,
			payeeNames:      make(map[types.ID]string),
			accountNames:    make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	// Top 5 should be visible (STK1 through STK5 have highest values)
	if !contains(view, "STK1") {
		t.Error("dashboard should show top holding STK1")
	}
	if !contains(view, "STK5") {
		t.Error("dashboard should show 5th holding STK5")
	}

	// 6th and 7th should be hidden, replaced by "+2 more" indicator
	if contains(view, "STK6") {
		t.Error("dashboard should NOT show 6th holding STK6 (limit is 5)")
	}
	if contains(view, "STK7") {
		t.Error("dashboard should NOT show 7th holding STK7 (limit is 5)")
	}
	if !contains(view, "+2 more") {
		t.Error("dashboard should show '+2 more' indicator for hidden holdings")
	}
}

func TestApp_RenderDashboard_InvestmentHoldingsNilMap(t *testing.T) {
	// Dashboard should handle nil investmentHoldings gracefully
	styles := NewStyles()
	styles.Resize(120, 40)

	investAccountID := types.NewID()

	app := &App{
		currentView:               ViewDashboard,
		width:                     120,
		height:                    40,
		styles:                    styles,
		dashboardExpandedAccounts: map[types.ID]bool{investAccountID: true},
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets: []report.AccountBalance{
					{AccountID: investAccountID, Name: "Brokerage", Type: "investment", Balance: types.MustNewMoney("10000.00")},
				},
				TotalAssets:      types.MustNewMoney("10000.00"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("10000.00"),
			},
			investmentHoldings: nil, // nil map
			securityTickers:    nil,
			payeeNames:         make(map[types.ID]string),
			accountNames:       make(map[types.ID]string),
		},
	}

	// Should not panic
	view := app.renderDashboard()
	if !contains(view, "Brokerage") {
		t.Error("dashboard should show investment account even with nil holdings map")
	}
}

func TestApp_RenderDashboard_InvestmentAccountTRRow(t *testing.T) {
	// TR-023: an investment account card on the dashboard renders a `TR`
	// line below the account balance row with formatted total return $
	// and %.
	styles := NewStyles()
	styles.Resize(120, 40)

	investAccountID := types.NewID()
	pct := 22.51

	app := &App{
		currentView:               ViewDashboard,
		width:                     120,
		height:                    40,
		styles:                    styles,
		dashboardExpandedAccounts: map[types.ID]bool{investAccountID: true},
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets: []report.AccountBalance{
					{AccountID: investAccountID, Name: "Brokerage", Type: "investment", Balance: types.MustNewMoney("25000.00")},
				},
				TotalAssets:      types.MustNewMoney("25000.00"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("25000.00"),
			},
			investmentHoldings: map[types.ID]*investment.AccountValuation{
				investAccountID: {
					AccountID:      investAccountID,
					TotalValue:     types.MustNewMoney("25000.00"),
					TotalReturn:    types.MustNewMoney("5267.50"),
					TotalReturnPct: &pct,
				},
			},
			securityTickers: map[types.ID]string{},
			payeeNames:      make(map[types.ID]string),
			accountNames:    make(map[types.ID]string),
		},
	}

	view := stripAnsi(app.renderDashboard())

	if !contains(view, "TR") {
		t.Error("dashboard should show 'TR' label for investment accounts")
	}
	if !contains(view, "$5267.50") {
		t.Errorf("dashboard should show TotalReturn value, got:\n%s", view)
	}
	if !contains(view, "22.51%") {
		t.Errorf("dashboard should show TotalReturnPct value, got:\n%s", view)
	}
}

func TestApp_RenderDashboard_InvestmentAccountTRRowNegative(t *testing.T) {
	// TR-023: a negative total return renders with the negative-money
	// format (leading minus sign).
	styles := NewStyles()
	styles.Resize(120, 40)

	investAccountID := types.NewID()
	pct := -8.25

	app := &App{
		currentView: ViewDashboard,
		width:       120,
		height:      40,
		styles:      styles,
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets: []report.AccountBalance{
					{AccountID: investAccountID, Name: "Brokerage", Type: "investment", Balance: types.MustNewMoney("9000.00")},
				},
				TotalAssets:      types.MustNewMoney("9000.00"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("9000.00"),
			},
			investmentHoldings: map[types.ID]*investment.AccountValuation{
				investAccountID: {
					AccountID:      investAccountID,
					TotalValue:     types.MustNewMoney("9000.00"),
					TotalReturn:    types.MustNewMoney("-825.00"),
					TotalReturnPct: &pct,
				},
			},
			securityTickers: map[types.ID]string{},
			payeeNames:      make(map[types.ID]string),
			accountNames:    make(map[types.ID]string),
		},
	}

	view := stripAnsi(app.renderDashboard())

	if !contains(view, "-$825.00") {
		t.Errorf("dashboard should show negative TotalReturn '-$825.00', got:\n%s", view)
	}
	if !contains(view, "-8.25%") {
		t.Errorf("dashboard should show negative TotalReturnPct '-8.25%%', got:\n%s", view)
	}
}

func TestApp_RenderDashboard_InvestmentAccountTRPctNilRendersDash(t *testing.T) {
	// TR-023: a nil TotalReturnPct (no buys ever — denominator is zero)
	// renders the "—" placeholder so the line shape stays stable.
	styles := NewStyles()
	styles.Resize(120, 40)

	investAccountID := types.NewID()

	app := &App{
		currentView: ViewDashboard,
		width:       120,
		height:      40,
		styles:      styles,
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets: []report.AccountBalance{
					{AccountID: investAccountID, Name: "Rollover IRA", Type: "investment", Balance: types.MustNewMoney("10000.00")},
				},
				TotalAssets:      types.MustNewMoney("10000.00"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("10000.00"),
			},
			investmentHoldings: map[types.ID]*investment.AccountValuation{
				investAccountID: {
					AccountID:      investAccountID,
					TotalValue:     types.MustNewMoney("10000.00"),
					TotalReturn:    types.ZeroMoney,
					TotalReturnPct: nil,
				},
			},
			securityTickers: map[types.ID]string{},
			payeeNames:      make(map[types.ID]string),
			accountNames:    make(map[types.ID]string),
		},
	}

	view := stripAnsi(app.renderDashboard())

	if !contains(view, "TR") {
		t.Errorf("dashboard should still show TR row when TotalReturnPct is nil, got:\n%s", view)
	}
	if !contains(view, "—") {
		t.Errorf("dashboard should show '—' placeholder when TotalReturnPct is nil, got:\n%s", view)
	}
}

func TestApp_RenderDashboard_NonInvestmentAccountNoTRRow(t *testing.T) {
	// TR-023: non-investment accounts (checking, savings, etc.) should
	// NOT get a TR row.
	styles := NewStyles()
	styles.Resize(120, 40)

	checkingID := types.NewID()

	app := &App{
		currentView: ViewDashboard,
		width:       120,
		height:      40,
		styles:      styles,
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets: []report.AccountBalance{
					{AccountID: checkingID, Name: "Checking", Type: "checking", Balance: types.MustNewMoney("5000.00")},
				},
				TotalAssets:      types.MustNewMoney("5000.00"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("5000.00"),
			},
			investmentHoldings: map[types.ID]*investment.AccountValuation{},
			securityTickers:    map[types.ID]string{},
			payeeNames:         make(map[types.ID]string),
			accountNames:       make(map[types.ID]string),
		},
	}

	view := stripAnsi(app.renderDashboard())

	// The view does have a column-bottom "Total" row, so we can't search
	// for "TR" alone (it could collide with substrings). Use the
	// account-line indent + "TR" to look for the per-account row that
	// should NOT exist for non-investment accounts.
	if contains(view, "    TR ") {
		t.Errorf("dashboard should NOT show TR row for non-investment accounts, got:\n%s", view)
	}
}

func TestApp_DashboardInvestmentAccountOpensPortfolioView(t *testing.T) {
	// SM-176: Selecting an investment account on the dashboard should open the
	// portfolio view (ViewPortfolio), not the regular register or investment register.
	investAcctID := types.NewID()
	investAcct := &account.Account{
		BaseModel: types.BaseModel{ID: investAcctID},
		Name:      "Brokerage",
		Type:      account.TypeInvestment,
	}

	sidebar := NewSidebar()
	sidebar.SetAccounts([]*account.Account{investAcct}, map[types.ID]*account.Balance{
		investAcctID: {AccountID: investAcctID, CurrentBalance: types.MustNewMoney("10000.00")},
	})
	// Move cursor to the account item (index 0 = group header, index 1 = account)
	sidebar.cursor = 1

	app := &App{
		currentView: ViewDashboard,
		sidebar:     sidebar,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	enterKey := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, cmd := app.Update(enterKey)
	updatedApp := model.(*App)

	if updatedApp.currentView != ViewPortfolio {
		t.Errorf("expected ViewPortfolio (%d), got %v (%d)", ViewPortfolio, updatedApp.currentView, updatedApp.currentView)
	}
	if cmd == nil {
		t.Error("expected a command to load portfolio data")
	}
}

func TestApp_DashboardNonInvestmentAccountOpensRegisterView(t *testing.T) {
	// Verify that non-investment accounts still open the regular register.
	checkAcctID := types.NewID()
	checkAcct := &account.Account{
		BaseModel: types.BaseModel{ID: checkAcctID},
		Name:      "Checking",
		Type:      account.TypeChecking,
	}

	sidebar := NewSidebar()
	sidebar.SetAccounts([]*account.Account{checkAcct}, map[types.ID]*account.Balance{
		checkAcctID: {AccountID: checkAcctID, CurrentBalance: types.MustNewMoney("5000.00")},
	})
	sidebar.cursor = 1

	app := &App{
		currentView: ViewDashboard,
		sidebar:     sidebar,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	enterKey := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, cmd := app.Update(enterKey)
	updatedApp := model.(*App)

	if updatedApp.currentView != ViewRegister {
		t.Errorf("expected ViewRegister (%d), got %v (%d)", ViewRegister, updatedApp.currentView, updatedApp.currentView)
	}
	if cmd == nil {
		t.Error("expected a command to load register data")
	}
}
