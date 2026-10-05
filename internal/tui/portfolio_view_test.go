package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/tui/sidebar"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// renderPortfolio renders the view through its table entry, as the app does,
// so the entry's closure that passes styles and the screen height is under
// test too.
func renderPortfolio(t *testing.T, app *App) string {
	t.Helper()
	e, ok := viewFor(ViewPortfolio)
	if !ok {
		t.Fatal("no view table entry for ViewPortfolio")
	}
	return e.render(app)
}

func testStyles() widget.Styles {
	s := widget.NewStyles()
	s.Resize(120, 40)
	return s
}

func TestPortfolioViewData(t *testing.T) {
	acct := &account.Account{
		BaseModel: types.NewBaseModel(),
		Name:      "Brokerage",
		Type:      account.TypeInvestment,
	}

	data := &portfolioViewData{
		account:       acct,
		securityNames: map[types.ID]string{},
	}

	if data.account.Name != "Brokerage" {
		t.Errorf("account name = %q, want %q", data.account.Name, "Brokerage")
	}
	if data.account.Type != account.TypeInvestment {
		t.Errorf("account type = %q, want %q", data.account.Type, account.TypeInvestment)
	}
}

func TestPortfolioViewData_WithValuation(t *testing.T) {
	acctID := types.NewID()
	secID := types.NewID()

	valuation := &investment.AccountValuation{
		AccountID:      acctID,
		CashBalance:    types.MustNewMoney("5000.00"),
		MarketValue:    types.MustNewMoney("15000.00"),
		TotalValue:     types.MustNewMoney("20000.00"),
		TotalCostBasis: types.MustNewMoney("12000.00"),
		TotalGainLoss:  types.MustNewMoney("3000.00"),
		TotalGainPct:   25.0,
		Holdings: []investment.Holding{
			{
				SecurityID:   secID,
				Shares:       types.MustNewQuantity("100"),
				AvgCost:      types.MustNewMoney("120.00"),
				CurrentPrice: types.MustNewMoney("150.00"),
				PriceDate:    types.NewDate(2024, time.March, 15),
				MarketValue:  types.MustNewMoney("15000.00"),
				CostBasis:    types.MustNewMoney("12000.00"),
				GainLoss:     types.MustNewMoney("3000.00"),
				GainPct:      25.0,
				HasPricing:   true,
			},
		},
	}

	data := &portfolioViewData{
		account: &account.Account{
			BaseModel: types.NewBaseModel(),
			Name:      "Brokerage",
			Type:      account.TypeInvestment,
		},
		valuation:     valuation,
		securityNames: map[types.ID]string{secID: "AAPL"},
	}

	if len(data.valuation.Holdings) != 1 {
		t.Errorf("expected 1 holding, got %d", len(data.valuation.Holdings))
	}
	if data.valuation.CashBalance.String() != "5000" {
		t.Errorf("cash balance = %q, want %q", data.valuation.CashBalance.String(), "5000")
	}
	if data.valuation.TotalGainPct != 25.0 {
		t.Errorf("gain pct = %v, want %v", data.valuation.TotalGainPct, 25.0)
	}
}

func TestFormatHoldingRow(t *testing.T) {
	secID := types.NewID()

	holding := &investment.Holding{
		SecurityID:   secID,
		Shares:       types.MustNewQuantity("100"),
		AvgCost:      types.MustNewMoney("120.00"),
		CurrentPrice: types.MustNewMoney("150.00"),
		PriceDate:    types.NewDate(2024, time.March, 15),
		MarketValue:  types.MustNewMoney("15000.00"),
		CostBasis:    types.MustNewMoney("12000.00"),
		GainLoss:     types.MustNewMoney("3000.00"),
		GainPct:      25.0,
		HasPricing:   true,
	}

	app := &App{
		portfolio: portfolioViewState{data: &portfolioViewData{
			securityNames: map[types.ID]string{secID: "AAPL"},
		}},
	}

	row := app.portfolio.formatHoldingRow(holding)

	if len(row) != 13 {
		t.Fatalf("expected 13 columns, got %d", len(row))
	}

	// Ticker
	if row[0] != "AAPL" {
		t.Errorf("ticker = %q, want %q", row[0], "AAPL")
	}
	// Shares
	if row[1] != "100" {
		t.Errorf("shares = %q, want %q", row[1], "100")
	}
	// Avg cost
	if row[2] != "$120.00" {
		t.Errorf("avg cost = %q, want %q", row[2], "$120.00")
	}
	// Price
	if row[3] != "$150.00" {
		t.Errorf("price = %q, want %q", row[3], "$150.00")
	}
	// Price date
	if row[4] != "03/15/24" {
		t.Errorf("price date = %q, want %q", row[4], "03/15/24")
	}
	// Market value
	if row[5] != "$15000.00" {
		t.Errorf("market value = %q, want %q", row[5], "$15000.00")
	}
	// Cost basis
	if row[6] != "$12000.00" {
		t.Errorf("cost basis = %q, want %q", row[6], "$12000.00")
	}
	// Unrealized gain
	if row[7] != "$3000.00" {
		t.Errorf("unreal = %q, want %q", row[7], "$3000.00")
	}
	// Dividends (default zero)
	if row[8] != "$0.00" {
		t.Errorf("div = %q, want %q", row[8], "$0.00")
	}
	// Realized gain (default zero)
	if row[9] != "$0.00" {
		t.Errorf("real = %q, want %q", row[9], "$0.00")
	}
	// Fees (default zero)
	if row[10] != "$0.00" {
		t.Errorf("fees = %q, want %q", row[10], "$0.00")
	}
	// Total return (default zero — fields not populated on test holding)
	if row[11] != "$0.00" {
		t.Errorf("total ret = %q, want %q", row[11], "$0.00")
	}
	// Return % (nil → placeholder)
	if row[12] != "—" {
		t.Errorf("ret %% = %q, want %q", row[12], "—")
	}
}

func TestFormatHoldingRow_NoPricing(t *testing.T) {
	secID := types.NewID()

	holding := &investment.Holding{
		SecurityID:   secID,
		Shares:       types.MustNewQuantity("50"),
		AvgCost:      types.MustNewMoney("80.00"),
		CurrentPrice: types.MustNewMoney("0"),
		MarketValue:  types.MustNewMoney("4000.00"),
		CostBasis:    types.MustNewMoney("4000.00"),
		GainLoss:     types.MustNewMoney("0"),
		GainPct:      0,
		HasPricing:   false,
	}

	app := &App{
		portfolio: portfolioViewState{data: &portfolioViewData{
			securityNames: map[types.ID]string{secID: "MSFT"},
		}},
	}

	row := app.portfolio.formatHoldingRow(holding)

	// Ticker should have ~ prefix when no pricing
	if row[0] != "~MSFT" {
		t.Errorf("ticker = %q, want %q (~ prefix for no pricing)", row[0], "~MSFT")
	}
	// Price should show N/A
	if row[3] != "N/A" {
		t.Errorf("price = %q, want %q", row[3], "N/A")
	}
	// Price date should be empty
	if row[4] != "" {
		t.Errorf("price date = %q, want empty", row[4])
	}
}

func TestFormatHoldingRow_NegativeGainLoss(t *testing.T) {
	secID := types.NewID()

	holding := &investment.Holding{
		SecurityID:   secID,
		Shares:       types.MustNewQuantity("25"),
		AvgCost:      types.MustNewMoney("200.00"),
		CurrentPrice: types.MustNewMoney("180.00"),
		PriceDate:    types.NewDate(2024, time.June, 1),
		MarketValue:  types.MustNewMoney("4500.00"),
		CostBasis:    types.MustNewMoney("5000.00"),
		GainLoss:     types.MustNewMoney("-500.00"),
		GainPct:      -10.0,
		HasPricing:   true,
	}

	app := &App{
		portfolio: portfolioViewState{data: &portfolioViewData{
			securityNames: map[types.ID]string{secID: "GOOG"},
		}},
	}

	row := app.portfolio.formatHoldingRow(holding)

	if row[7] != "-$500.00" {
		t.Errorf("unreal = %q, want %q", row[7], "-$500.00")
	}
}

func TestFormatHoldingRow_TotalReturnColumns(t *testing.T) {
	secID := types.NewID()
	retPct := 12.5

	holding := &investment.Holding{
		SecurityID:        secID,
		Shares:            types.MustNewQuantity("10"),
		AvgCost:           types.MustNewMoney("100"),
		CurrentPrice:      types.MustNewMoney("95"),
		PriceDate:         types.NewDate(2024, time.July, 1),
		MarketValue:       types.MustNewMoney("950"),
		CostBasis:         types.MustNewMoney("1000"),
		GainLoss:          types.MustNewMoney("-50"),
		GainPct:           -5.0,
		HasPricing:        true,
		DividendsReceived: types.MustNewMoney("75"),
		RealizedGain:      types.MustNewMoney("100"),
		FeesPaid:          types.MustNewMoney("25"),
		TotalReturn:       types.MustNewMoney("100"),
		TotalReturnPct:    &retPct,
	}

	app := &App{
		portfolio: portfolioViewState{data: &portfolioViewData{
			securityNames: map[types.ID]string{secID: "DIV"},
		}},
	}

	row := app.portfolio.formatHoldingRow(holding)

	if row[7] != "-$50.00" {
		t.Errorf("unreal = %q, want %q", row[7], "-$50.00")
	}
	if row[8] != "$75.00" {
		t.Errorf("div = %q, want %q", row[8], "$75.00")
	}
	if row[9] != "$100.00" {
		t.Errorf("real = %q, want %q", row[9], "$100.00")
	}
	// Fees are stored positive; displayed negative.
	if row[10] != "-$25.00" {
		t.Errorf("fees = %q, want %q", row[10], "-$25.00")
	}
	if row[11] != "$100.00" {
		t.Errorf("total ret = %q, want %q", row[11], "$100.00")
	}
	if row[12] != "12.50%" {
		t.Errorf("ret %% = %q, want %q", row[12], "12.50%")
	}
}

func TestFormatHoldingRow_RealizedUnavailable(t *testing.T) {
	secID := types.NewID()

	holding := &investment.Holding{
		SecurityID:              secID,
		Shares:                  types.MustNewQuantity("10"),
		MarketValue:             types.MustNewMoney("1000"),
		CostBasis:               types.MustNewMoney("1000"),
		HasPricing:              true,
		RealizedGainUnavailable: true,
	}

	app := &App{
		portfolio: portfolioViewState{data: &portfolioViewData{
			securityNames: map[types.ID]string{secID: "MRG"},
		}},
	}

	row := app.portfolio.formatHoldingRow(holding)

	if row[9] != "n/a" {
		t.Errorf("real = %q, want %q", row[9], "n/a")
	}
}

func TestFormatLotDetailRow(t *testing.T) {
	lot := &investment.LotDetail{
		LotID:        types.NewID(),
		PurchaseDate: types.NewDate(2024, time.January, 15),
		Shares:       types.MustNewQuantity("50"),
		CostPerShare: types.MustNewMoney("100.00"),
		CostBasis:    types.MustNewMoney("5000.00"),
		CurrentValue: types.MustNewMoney("7500.00"),
		GainLoss:     types.MustNewMoney("2500.00"),
		GainPct:      50.0,
	}

	row := formatLotDetailRow(lot)

	if len(row) != 7 {
		t.Fatalf("expected 7 columns, got %d", len(row))
	}

	// Purchase date
	if row[0] != "01/15/24" {
		t.Errorf("purchase date = %q, want %q", row[0], "01/15/24")
	}
	// Shares
	if row[1] != "50" {
		t.Errorf("shares = %q, want %q", row[1], "50")
	}
	// Cost/share
	if row[2] != "$100.00" {
		t.Errorf("cost/share = %q, want %q", row[2], "$100.00")
	}
	// Cost basis
	if row[3] != "$5000.00" {
		t.Errorf("cost basis = %q, want %q", row[3], "$5000.00")
	}
	// Current value
	if row[4] != "$7500.00" {
		t.Errorf("current value = %q, want %q", row[4], "$7500.00")
	}
	// Gain/loss
	if row[5] != "$2500.00" {
		t.Errorf("gain/loss = %q, want %q", row[5], "$2500.00")
	}
	// Gain/loss %
	if row[6] != "50.00%" {
		t.Errorf("gain/loss %% = %q, want %q", row[6], "50.00%")
	}
}

func TestFormatLotDetailRow_NegativeGain(t *testing.T) {
	lot := &investment.LotDetail{
		LotID:        types.NewID(),
		PurchaseDate: types.NewDate(2024, time.March, 20),
		Shares:       types.MustNewQuantity("30"),
		CostPerShare: types.MustNewMoney("150.00"),
		CostBasis:    types.MustNewMoney("4500.00"),
		CurrentValue: types.MustNewMoney("3600.00"),
		GainLoss:     types.MustNewMoney("-900.00"),
		GainPct:      -20.0,
	}

	row := formatLotDetailRow(lot)

	if row[5] != "-$900.00" {
		t.Errorf("gain/loss = %q, want %q", row[5], "-$900.00")
	}
	if row[6] != "-20.00%" {
		t.Errorf("gain/loss %% = %q, want %q", row[6], "-20.00%")
	}
}

func TestPortfolioSummaryBar(t *testing.T) {
	acctID := types.NewID()
	trPct := 30.0

	app := &App{
		styles: testStyles(),
		portfolio: portfolioViewState{data: &portfolioViewData{
			valuation: &investment.AccountValuation{
				AccountID:         acctID,
				CashBalance:       types.MustNewMoney("5000.00"),
				MarketValue:       types.MustNewMoney("15000.00"),
				TotalValue:        types.MustNewMoney("20000.00"),
				TotalCostBasis:    types.MustNewMoney("12000.00"),
				TotalGainLoss:     types.MustNewMoney("3000.00"),
				TotalGainPct:      25.0,
				RealizedGain:      types.MustNewMoney("200.00"),
				DividendsReceived: types.MustNewMoney("400.00"),
				InterestReceived:  types.MustNewMoney("50.00"),
				FeesPaid:          types.MustNewMoney("50.00"),
				TotalReturn:       types.MustNewMoney("3600.00"),
				TotalReturnPct:    &trPct,
			},
		}},
	}

	summary := app.portfolio.renderSummary(app.styles, 100)

	// Line 1: position snapshot
	for _, label := range []string{"Cash:", "Mkt Value:", "Total:", "Cost Basis:", "Gain/Loss:", "G/L %:"} {
		if !strings.Contains(summary, label) {
			t.Errorf("summary should contain %q label", label)
		}
	}
	if !strings.Contains(summary, "$5000.00") {
		t.Error("summary should contain cash balance value")
	}
	if !strings.Contains(summary, "$20000.00") {
		t.Error("summary should contain total value")
	}
	if !strings.Contains(summary, "25.00%") {
		t.Error("summary should contain G/L %")
	}

	// Line 2: total-return breakdown
	for _, label := range []string{"Realized", "Div", "Int", "Fees", "Total return"} {
		if !strings.Contains(summary, label) {
			t.Errorf("summary should contain %q label", label)
		}
	}
	if !strings.Contains(summary, "$400.00") {
		t.Error("summary should contain dividends value")
	}
	if !strings.Contains(summary, "$3600.00") {
		t.Error("summary should contain total return value")
	}
	if !strings.Contains(summary, "30.00%") {
		t.Error("summary should contain total return %")
	}
	// Fees shown as negative (per spec)
	if !strings.Contains(summary, "-$50.00") {
		t.Error("summary should display fees as negative magnitude")
	}
}

func TestPortfolioSummaryBar_NilTotalReturnPct(t *testing.T) {
	acctID := types.NewID()

	app := &App{
		styles: testStyles(),
		portfolio: portfolioViewState{data: &portfolioViewData{
			valuation: &investment.AccountValuation{
				AccountID:      acctID,
				CashBalance:    types.MustNewMoney("0"),
				MarketValue:    types.MustNewMoney("1000"),
				TotalValue:     types.MustNewMoney("1000"),
				TotalCostBasis: types.MustNewMoney("0"),
				TotalGainLoss:  types.MustNewMoney("0"),
				TotalGainPct:   0,
				TotalReturn:    types.MustNewMoney("0"),
				TotalReturnPct: nil,
			},
		}},
	}

	summary := app.portfolio.renderSummary(app.styles, 100)

	if !strings.Contains(summary, "—") {
		t.Error("nil TotalReturnPct should render as '—' placeholder")
	}
}

func TestPortfolioSummaryBar_PartialRealizedMarker(t *testing.T) {
	acctID := types.NewID()
	trPct := 30.0

	app := &App{
		styles: testStyles(),
		portfolio: portfolioViewState{data: &portfolioViewData{
			valuation: &investment.AccountValuation{
				AccountID:              acctID,
				CashBalance:            types.MustNewMoney("100"),
				MarketValue:            types.MustNewMoney("1000"),
				TotalValue:             types.MustNewMoney("1100"),
				TotalCostBasis:         types.MustNewMoney("800"),
				TotalGainLoss:          types.MustNewMoney("200"),
				TotalGainPct:           25.0,
				RealizedGain:           types.MustNewMoney("-0.29"),
				TotalReturn:            types.MustNewMoney("199.71"),
				TotalReturnPct:         &trPct,
				AnyRealizedUnavailable: true,
			},
		}},
	}

	summary := app.portfolio.renderSummary(app.styles, 100)

	if !strings.Contains(summary, "(partial)") {
		t.Errorf("expected '(partial)' marker when AnyRealizedUnavailable=true; got %q", summary)
	}
}

func TestPortfolioSummaryBar_NoPartialMarkerWhenAllAvailable(t *testing.T) {
	acctID := types.NewID()
	trPct := 30.0

	app := &App{
		styles: testStyles(),
		portfolio: portfolioViewState{data: &portfolioViewData{
			valuation: &investment.AccountValuation{
				AccountID:              acctID,
				CashBalance:            types.MustNewMoney("100"),
				MarketValue:            types.MustNewMoney("1000"),
				TotalValue:             types.MustNewMoney("1100"),
				TotalCostBasis:         types.MustNewMoney("800"),
				TotalGainLoss:          types.MustNewMoney("200"),
				TotalGainPct:           25.0,
				RealizedGain:           types.MustNewMoney("-0.29"),
				TotalReturn:            types.MustNewMoney("199.71"),
				TotalReturnPct:         &trPct,
				AnyRealizedUnavailable: false,
			},
		}},
	}

	summary := app.portfolio.renderSummary(app.styles, 100)

	if strings.Contains(summary, "(partial)") {
		t.Errorf("expected no '(partial)' marker when AnyRealizedUnavailable=false; got %q", summary)
	}
}

func TestPortfolioSummaryBar_NilValuation(t *testing.T) {
	app := &App{
		styles:    testStyles(),
		portfolio: portfolioViewState{data: nil},
	}

	summary := app.portfolio.renderSummary(app.styles, 100)
	if summary != "" {
		t.Errorf("summary should be empty for nil portfolio data, got %q", summary)
	}
}

func TestBuildPortfolioHoldingsTable(t *testing.T) {
	secID := types.NewID()

	app := &App{
		portfolio: portfolioViewState{data: &portfolioViewData{
			valuation: &investment.AccountValuation{
				Holdings: []investment.Holding{
					{
						SecurityID:   secID,
						Shares:       types.MustNewQuantity("100"),
						AvgCost:      types.MustNewMoney("120.00"),
						CurrentPrice: types.MustNewMoney("150.00"),
						PriceDate:    types.NewDate(2024, time.March, 15),
						MarketValue:  types.MustNewMoney("15000.00"),
						CostBasis:    types.MustNewMoney("12000.00"),
						GainLoss:     types.MustNewMoney("3000.00"),
						GainPct:      25.0,
						HasPricing:   true,
					},
				},
			},
			securityNames: map[types.ID]string{secID: "AAPL"},
		}},
	}

	app.portfolio.buildHoldingsTable()

	if app.portfolio.holdingsTable == nil {
		t.Fatal("holdings table should be created")
	}

	rows := app.portfolio.holdingsTable.Rows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	if rows[0][0] != "AAPL" {
		t.Errorf("first row ticker = %q, want %q", rows[0][0], "AAPL")
	}
}

func TestBuildPortfolioHoldingsTable_MultipleHoldings(t *testing.T) {
	sec1 := types.NewID()
	sec2 := types.NewID()

	app := &App{
		portfolio: portfolioViewState{data: &portfolioViewData{
			valuation: &investment.AccountValuation{
				Holdings: []investment.Holding{
					{
						SecurityID:   sec1,
						Shares:       types.MustNewQuantity("100"),
						AvgCost:      types.MustNewMoney("120.00"),
						CurrentPrice: types.MustNewMoney("150.00"),
						MarketValue:  types.MustNewMoney("15000.00"),
						CostBasis:    types.MustNewMoney("12000.00"),
						GainLoss:     types.MustNewMoney("3000.00"),
						GainPct:      25.0,
						HasPricing:   true,
					},
					{
						SecurityID:   sec2,
						Shares:       types.MustNewQuantity("50"),
						AvgCost:      types.MustNewMoney("80.00"),
						CurrentPrice: types.MustNewMoney("90.00"),
						MarketValue:  types.MustNewMoney("4500.00"),
						CostBasis:    types.MustNewMoney("4000.00"),
						GainLoss:     types.MustNewMoney("500.00"),
						GainPct:      12.5,
						HasPricing:   true,
					},
				},
			},
			securityNames: map[types.ID]string{sec1: "AAPL", sec2: "MSFT"},
		}},
	}

	app.portfolio.buildHoldingsTable()

	if app.portfolio.holdingsTable == nil {
		t.Fatal("holdings table should be created")
	}

	rows := app.portfolio.holdingsTable.Rows()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestBuildPortfolioHoldingsTable_NilData(t *testing.T) {
	app := &App{
		portfolio: portfolioViewState{data: nil},
	}

	// Should not panic
	app.portfolio.buildHoldingsTable()

	if app.portfolio.holdingsTable != nil {
		t.Error("holdings table should be nil for nil data")
	}
}

func TestBuildPortfolioLotsTable(t *testing.T) {
	app := &App{
		portfolio: portfolioViewState{data: &portfolioViewData{
			lotDetails: []investment.LotDetail{
				{
					LotID:        types.NewID(),
					PurchaseDate: types.NewDate(2024, time.January, 15),
					Shares:       types.MustNewQuantity("50"),
					CostPerShare: types.MustNewMoney("100.00"),
					CostBasis:    types.MustNewMoney("5000.00"),
					CurrentValue: types.MustNewMoney("7500.00"),
					GainLoss:     types.MustNewMoney("2500.00"),
					GainPct:      50.0,
				},
				{
					LotID:        types.NewID(),
					PurchaseDate: types.NewDate(2024, time.June, 1),
					Shares:       types.MustNewQuantity("30"),
					CostPerShare: types.MustNewMoney("130.00"),
					CostBasis:    types.MustNewMoney("3900.00"),
					CurrentValue: types.MustNewMoney("4500.00"),
					GainLoss:     types.MustNewMoney("600.00"),
					GainPct:      15.38,
				},
			},
		}},
	}

	app.portfolio.buildLotsTable()

	if app.portfolio.lotsTable == nil {
		t.Fatal("lots table should be created")
	}

	rows := app.portfolio.lotsTable.Rows()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if rows[0][0] != "01/15/24" {
		t.Errorf("first lot purchase date = %q, want %q", rows[0][0], "01/15/24")
	}
}

func TestBuildPortfolioLotsTable_NilData(t *testing.T) {
	app := &App{
		portfolio: portfolioViewState{data: nil},
	}

	// Should not panic
	app.portfolio.buildLotsTable()

	if app.portfolio.lotsTable != nil {
		t.Error("lots table should be nil for nil data")
	}
}

func TestPortfolioViewMode(t *testing.T) {
	if portfolioViewHoldings != 0 {
		t.Errorf("portfolioViewHoldings = %d, want 0", portfolioViewHoldings)
	}
	if portfolioViewLots != 1 {
		t.Errorf("portfolioViewLots = %d, want 1", portfolioViewLots)
	}
}

func TestRenderPortfolioView_Loading(t *testing.T) {
	app := &App{
		styles:    testStyles(),
		portfolio: portfolioViewState{data: nil},
	}

	rendered := renderPortfolio(t, app)
	if !strings.Contains(rendered, "Loading portfolio...") {
		t.Error("should show loading message when data is nil")
	}
}

func TestRenderPortfolioView_NoHoldings(t *testing.T) {
	app := &App{
		width:  120,
		height: 40,
		styles: testStyles(),
		portfolio: portfolioViewState{data: &portfolioViewData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
			valuation: &investment.AccountValuation{
				CashBalance:    types.MustNewMoney("5000.00"),
				MarketValue:    types.MustNewMoney("0"),
				TotalValue:     types.MustNewMoney("5000.00"),
				TotalCostBasis: types.MustNewMoney("0"),
				TotalGainLoss:  types.MustNewMoney("0"),
				TotalGainPct:   0,
				Holdings:       []investment.Holding{},
			},
			securityNames: map[types.ID]string{},
		}},
	}

	rendered := renderPortfolio(t, app)
	if !strings.Contains(rendered, "No holdings") {
		t.Error("should show 'No holdings' when there are no holdings")
	}
	if !strings.Contains(rendered, "BROKERAGE PORTFOLIO") {
		t.Error("should show account name in title")
	}
}

func TestRenderPortfolioView_WithHoldings(t *testing.T) {
	secID := types.NewID()

	app := &App{
		width:  120,
		height: 40,
		styles: testStyles(),
		portfolio: portfolioViewState{
			data: &portfolioViewData{
				account: &account.Account{
					BaseModel: types.NewBaseModel(),
					Name:      "Investment",
					Type:      account.TypeInvestment,
				},
				valuation: &investment.AccountValuation{
					CashBalance:    types.MustNewMoney("5000.00"),
					MarketValue:    types.MustNewMoney("15000.00"),
					TotalValue:     types.MustNewMoney("20000.00"),
					TotalCostBasis: types.MustNewMoney("12000.00"),
					TotalGainLoss:  types.MustNewMoney("3000.00"),
					TotalGainPct:   25.0,
					Holdings: []investment.Holding{
						{
							SecurityID:   secID,
							Shares:       types.MustNewQuantity("100"),
							AvgCost:      types.MustNewMoney("120.00"),
							CurrentPrice: types.MustNewMoney("150.00"),
							PriceDate:    types.NewDate(2024, time.March, 15),
							MarketValue:  types.MustNewMoney("15000.00"),
							CostBasis:    types.MustNewMoney("12000.00"),
							GainLoss:     types.MustNewMoney("3000.00"),
							GainPct:      25.0,
							HasPricing:   true,
						},
					},
				},
				securityNames: map[types.ID]string{secID: "AAPL"},
			},
			mode: portfolioViewHoldings,
		},
	}

	// Build the holdings table first
	app.portfolio.buildHoldingsTable()

	rendered := renderPortfolio(t, app)
	if !strings.Contains(rendered, "INVESTMENT PORTFOLIO") {
		t.Error("should show account name with PORTFOLIO suffix")
	}
	// Summary should be present
	if !strings.Contains(rendered, "$5000.00") {
		t.Error("should show cash balance in summary")
	}
}

func TestRenderPortfolioView_LotMode(t *testing.T) {
	secID := types.NewID()

	app := &App{
		width:  120,
		height: 40,
		styles: testStyles(),
		portfolio: portfolioViewState{
			data: &portfolioViewData{
				account: &account.Account{
					BaseModel: types.NewBaseModel(),
					Name:      "Investment",
					Type:      account.TypeInvestment,
				},
				valuation: &investment.AccountValuation{
					CashBalance: types.MustNewMoney("5000.00"),
					Holdings:    []investment.Holding{},
				},
				securityNames: map[types.ID]string{secID: "AAPL"},
				lotDetails: []investment.LotDetail{
					{
						LotID:        types.NewID(),
						PurchaseDate: types.NewDate(2024, time.January, 15),
						Shares:       types.MustNewQuantity("50"),
						CostPerShare: types.MustNewMoney("100.00"),
						CostBasis:    types.MustNewMoney("5000.00"),
						CurrentValue: types.MustNewMoney("7500.00"),
						GainLoss:     types.MustNewMoney("2500.00"),
						GainPct:      50.0,
					},
				},
				lotSecurityID: secID,
			},
			mode: portfolioViewLots,
		},
	}

	// Build the lots table
	app.portfolio.buildLotsTable()

	rendered := renderPortfolio(t, app)
	if !strings.Contains(rendered, "Lots for AAPL") {
		t.Error("should show lot detail header with security ticker")
	}
}

func TestPortfolioViewToggle_RegisterToPortfolio(t *testing.T) {
	sb := sidebar.New()
	sb.SetFocused(false)

	app := &App{
		currentView: ViewInvestmentRegister,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     sb,
		investmentRegister: investmentRegisterViewState{
			data: &investmentRegisterData{
				account: &account.Account{
					BaseModel: types.NewBaseModel(),
					Name:      "Brokerage",
					Type:      account.TypeInvestment,
				},
			},
			table: widget.NewTable(nil),
		},
	}

	// Simulate pressing 'p' to switch to portfolio
	msg := tea.KeyPressMsg{Code: 'p', Text: "p"}
	_, cmd := app.handleInvestmentRegisterKeys(msg)

	if app.currentView != ViewPortfolio {
		t.Errorf("view = %v, want ViewPortfolio", app.currentView)
	}
	if app.portfolio.data != nil {
		t.Error("portfolio data should be nil (cleared for loading)")
	}
	// Command should be non-nil (loading portfolio data)
	if cmd == nil {
		t.Error("should return a command to load portfolio data")
	}
}

func TestPortfolioViewToggle_PortfolioToRegister(t *testing.T) {
	acctID := types.NewID()
	sb := sidebar.New()
	sb.SetFocused(false)

	app := &App{
		currentView: ViewPortfolio,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     sb,
		portfolio: portfolioViewState{
			data: &portfolioViewData{
				account: &account.Account{
					BaseModel: types.BaseModel{ID: acctID},
					Name:      "Brokerage",
					Type:      account.TypeInvestment,
				},
			},
			mode:          portfolioViewHoldings,
			holdingsTable: widget.NewTable(nil),
		},
	}

	// Simulate pressing 'r' to switch to register
	msg := tea.KeyPressMsg{Code: 'r', Text: "r"}
	_, cmd := app.handlePortfolioKeys(msg)

	if app.currentView != ViewInvestmentRegister {
		t.Errorf("view = %v, want ViewInvestmentRegister", app.currentView)
	}
	if cmd == nil {
		t.Error("should return a command to load register data")
	}
}

func TestPortfolioKeys_LotDrillDown(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()
	sb := sidebar.New()
	sb.SetFocused(false)

	app := &App{
		width:       120,
		height:      40,
		currentView: ViewPortfolio,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     sb,
		portfolio: portfolioViewState{
			data: &portfolioViewData{
				account: &account.Account{
					BaseModel: types.BaseModel{ID: acctID},
					Name:      "Brokerage",
					Type:      account.TypeInvestment,
					TrackLots: true,
				},
				valuation: &investment.AccountValuation{
					Holdings: []investment.Holding{
						{
							SecurityID:   secID,
							Shares:       types.MustNewQuantity("100"),
							AvgCost:      types.MustNewMoney("120.00"),
							CurrentPrice: types.MustNewMoney("150.00"),
							MarketValue:  types.MustNewMoney("15000.00"),
							CostBasis:    types.MustNewMoney("12000.00"),
							GainLoss:     types.MustNewMoney("3000.00"),
							GainPct:      25.0,
							HasPricing:   true,
						},
					},
				},
				securityNames: map[types.ID]string{secID: "AAPL"},
			},
			mode: portfolioViewHoldings,
		},
	}

	// Build holdings table and set cursor to first row
	app.portfolio.buildHoldingsTable()
	app.portfolio.holdingsTable.SetFocused(true)

	// Press Enter to drill down
	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	_, cmd := app.handlePortfolioKeys(msg)

	if app.portfolio.mode != portfolioViewLots {
		t.Errorf("mode = %v, want portfolioViewLots", app.portfolio.mode)
	}
	if cmd == nil {
		t.Error("should return a command to load lot detail")
	}
}

func TestPortfolioKeys_LotDrillDown_NonLotTracking(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()
	sb := sidebar.New()
	sb.SetFocused(false)

	app := &App{
		width:       120,
		height:      40,
		currentView: ViewPortfolio,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     sb,
		portfolio: portfolioViewState{
			data: &portfolioViewData{
				account: &account.Account{
					BaseModel: types.BaseModel{ID: acctID},
					Name:      "Brokerage",
					Type:      account.TypeInvestment,
					TrackLots: false,
				},
				valuation: &investment.AccountValuation{
					Holdings: []investment.Holding{
						{
							SecurityID: secID,
							Shares:     types.MustNewQuantity("100"),
							HasPricing: true,
						},
					},
				},
				securityNames: map[types.ID]string{secID: "AAPL"},
			},
			mode: portfolioViewHoldings,
		},
	}

	app.portfolio.buildHoldingsTable()
	app.portfolio.holdingsTable.SetFocused(true)

	// Press Enter - should NOT drill down for non-lot-tracking
	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	_, cmd := app.handlePortfolioKeys(msg)

	if app.portfolio.mode != portfolioViewHoldings {
		t.Errorf("mode should remain portfolioViewHoldings for non-lot-tracking")
	}
	if cmd != nil {
		t.Error("should not return a command for non-lot-tracking accounts")
	}
}

func TestPortfolioKeys_EscapeFromLots(t *testing.T) {
	secID := types.NewID()
	sb := sidebar.New()
	sb.SetFocused(false)

	app := &App{
		currentView: ViewPortfolio,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     sb,
		portfolio: portfolioViewState{
			data: &portfolioViewData{
				account: &account.Account{
					BaseModel: types.NewBaseModel(),
					Name:      "Brokerage",
					Type:      account.TypeInvestment,
				},
				lotDetails:    []investment.LotDetail{},
				lotSecurityID: secID,
			},
			mode:          portfolioViewLots,
			holdingsTable: widget.NewTable(nil),
			lotsTable:     widget.NewTable(nil),
		},
	}

	// Press Escape from lot view - should go back to holdings
	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
	_, _ = app.handlePortfolioKeys(msg)

	if app.portfolio.mode != portfolioViewHoldings {
		t.Errorf("mode = %v, want portfolioViewHoldings after Esc from lots", app.portfolio.mode)
	}
	if app.portfolio.data.lotDetails != nil {
		t.Error("lot details should be cleared")
	}
}

func TestPortfolioKeys_EscapeFromHoldings(t *testing.T) {
	acctID := types.NewID()
	sb := sidebar.New()
	sb.SetFocused(false)

	app := &App{
		currentView: ViewPortfolio,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     sb,
		portfolio: portfolioViewState{
			data: &portfolioViewData{
				account: &account.Account{
					BaseModel: types.BaseModel{ID: acctID},
					Name:      "Brokerage",
					Type:      account.TypeInvestment,
				},
			},
			mode:          portfolioViewHoldings,
			holdingsTable: widget.NewTable(nil),
		},
	}

	// Press Escape from holdings - should go to investment register
	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
	_, cmd := app.handlePortfolioKeys(msg)

	if app.currentView != ViewInvestmentRegister {
		t.Errorf("view = %v, want ViewInvestmentRegister after Esc from holdings", app.currentView)
	}
	if cmd == nil {
		t.Error("should return a command to load investment register data")
	}
}

func TestPortfolioKeys_Navigation(t *testing.T) {
	secID := types.NewID()
	sb := sidebar.New()
	sb.SetFocused(false)

	app := &App{
		width:       120,
		height:      40,
		currentView: ViewPortfolio,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     sb,
		portfolio: portfolioViewState{
			data: &portfolioViewData{
				account: &account.Account{
					BaseModel: types.NewBaseModel(),
					Name:      "Test",
					Type:      account.TypeInvestment,
				},
				valuation: &investment.AccountValuation{
					Holdings: []investment.Holding{
						{SecurityID: secID, Shares: types.MustNewQuantity("10"), HasPricing: true},
						{SecurityID: types.NewID(), Shares: types.MustNewQuantity("20"), HasPricing: true},
					},
				},
				securityNames: map[types.ID]string{},
			},
			mode: portfolioViewHoldings,
		},
	}

	app.portfolio.buildHoldingsTable()
	app.portfolio.holdingsTable.SetFocused(true)

	// Move down
	downMsg := tea.KeyPressMsg{Code: tea.KeyDown}
	app.handlePortfolioKeys(downMsg)
	if app.portfolio.holdingsTable.Cursor() != 1 {
		t.Errorf("cursor = %d after down, want 1", app.portfolio.holdingsTable.Cursor())
	}

	// Move up
	upMsg := tea.KeyPressMsg{Code: tea.KeyUp}
	app.handlePortfolioKeys(upMsg)
	if app.portfolio.holdingsTable.Cursor() != 0 {
		t.Errorf("cursor = %d after up, want 0", app.portfolio.holdingsTable.Cursor())
	}
}

func TestPortfolioLoadedMsg_Handler(t *testing.T) {
	secID := types.NewID()

	app := &App{
		currentView: ViewPortfolio,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		styles:      testStyles(),
	}

	msg := portfolioLoadedMsg{
		data: &portfolioViewData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Test",
				Type:      account.TypeInvestment,
			},
			valuation: &investment.AccountValuation{
				Holdings: []investment.Holding{
					{SecurityID: secID, Shares: types.MustNewQuantity("10"), HasPricing: true},
				},
			},
			securityNames: map[types.ID]string{secID: "AAPL"},
		},
	}

	app.Update(msg)

	if app.portfolio.data == nil {
		t.Fatal("portfolio data should be set after loaded msg")
	}
	if app.portfolio.mode != portfolioViewHoldings {
		t.Errorf("mode = %v, want portfolioViewHoldings", app.portfolio.mode)
	}
	if app.portfolio.holdingsTable == nil {
		t.Error("holdings table should be built after loaded msg")
	}
}

func TestPortfolioLotDetailMsg_Handler(t *testing.T) {
	secID := types.NewID()

	app := &App{
		currentView: ViewPortfolio,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		styles:      testStyles(),
		portfolio: portfolioViewState{
			data: &portfolioViewData{
				account: &account.Account{
					BaseModel: types.NewBaseModel(),
					Name:      "Test",
					Type:      account.TypeInvestment,
				},
				valuation: &investment.AccountValuation{
					Holdings: []investment.Holding{},
				},
				securityNames: map[types.ID]string{secID: "AAPL"},
			},
			mode: portfolioViewLots,
		},
	}

	lots := []investment.LotDetail{
		{
			LotID:        types.NewID(),
			PurchaseDate: types.NewDate(2024, time.January, 15),
			Shares:       types.MustNewQuantity("50"),
			CostPerShare: types.MustNewMoney("100.00"),
			CostBasis:    types.MustNewMoney("5000.00"),
			CurrentValue: types.MustNewMoney("7500.00"),
			GainLoss:     types.MustNewMoney("2500.00"),
			GainPct:      50.0,
		},
	}

	msg := portfolioLotDetailMsg{lots: lots, securityID: secID}
	app.Update(msg)

	if app.portfolio.data.lotDetails == nil {
		t.Fatal("lot details should be set after lot detail msg")
	}
	if len(app.portfolio.data.lotDetails) != 1 {
		t.Errorf("expected 1 lot detail, got %d", len(app.portfolio.data.lotDetails))
	}
	if app.portfolio.data.lotSecurityID != secID {
		t.Error("lot security ID should be set")
	}
	if app.portfolio.lotsTable == nil {
		t.Error("lots table should be built after lot detail msg")
	}
}

func TestSelectedHolding(t *testing.T) {
	secID := types.NewID()

	app := &App{
		portfolio: portfolioViewState{data: &portfolioViewData{
			valuation: &investment.AccountValuation{
				Holdings: []investment.Holding{
					{SecurityID: secID, Shares: types.MustNewQuantity("100"), HasPricing: true},
				},
			},
			securityNames: map[types.ID]string{secID: "AAPL"},
		}},
	}

	app.portfolio.buildHoldingsTable()

	h := app.portfolio.selectedHolding()
	if h == nil {
		t.Fatal("selected holding should not be nil")
	}
	if h.SecurityID != secID {
		t.Errorf("security ID = %v, want %v", h.SecurityID, secID)
	}
}

func TestSelectedHolding_NilData(t *testing.T) {
	app := &App{
		portfolio: portfolioViewState{data: nil},
	}

	h := app.portfolio.selectedHolding()
	if h != nil {
		t.Error("selected holding should be nil when data is nil")
	}
}

func TestPortfolioShortcuts(t *testing.T) {
	s := portfolioShortcuts()
	if s.Title != "Portfolio" {
		t.Errorf("title = %q, want %q", s.Title, "Portfolio")
	}
	if len(s.Entries) == 0 {
		t.Error("should have shortcut entries")
	}
}

func TestViewPortfolioString(t *testing.T) {
	if ViewPortfolio.String() != "Portfolio" {
		t.Errorf("ViewPortfolio.String() = %q, want %q", ViewPortfolio.String(), "Portfolio")
	}
}

func TestActivePortfolioTable_HoldingsMode(t *testing.T) {
	app := &App{
		portfolio: portfolioViewState{
			mode:          portfolioViewHoldings,
			holdingsTable: widget.NewTable(nil),
			lotsTable:     widget.NewTable(nil),
		},
	}

	tbl := app.portfolio.activeTable()
	if tbl != app.portfolio.holdingsTable {
		t.Error("should return holdings table in holdings mode")
	}
}

func TestActivePortfolioTable_LotsMode(t *testing.T) {
	app := &App{
		portfolio: portfolioViewState{
			mode:          portfolioViewLots,
			holdingsTable: widget.NewTable(nil),
			lotsTable:     widget.NewTable(nil),
		},
	}

	tbl := app.portfolio.activeTable()
	if tbl != app.portfolio.lotsTable {
		t.Error("should return lots table in lots mode")
	}
}

func TestActivePortfolioTable_NilTables(t *testing.T) {
	app := &App{
		portfolio: portfolioViewState{mode: portfolioViewHoldings},
	}

	// Should not panic, returns a placeholder
	tbl := app.portfolio.activeTable()
	if tbl == nil {
		t.Error("should return a non-nil placeholder table")
	}
}

// The render closure hands the view the screen height: a portfolio with many
// holdings fits the screen it is given. With the width passed in its place,
// the view would draw 120 lines on a 30-line screen.
func TestPortfolioView_RenderFitsTheScreen(t *testing.T) {
	var holdings []investment.Holding
	names := map[types.ID]string{}
	for i := range 60 {
		id := types.NewID()
		names[id] = fmt.Sprintf("T%02d", i)
		holdings = append(holdings, investment.Holding{
			SecurityID:   id,
			Shares:       types.MustNewQuantity("10"),
			CurrentPrice: types.MustNewMoney(fmt.Sprintf("%d.00", 100+i)),
			MarketValue:  types.MustNewMoney(fmt.Sprintf("%d.00", 1000+10*i)),
			HasPricing:   true,
		})
	}
	app := &App{currentView: ViewPortfolio, width: 120, height: 30, styles: widget.NewStyles()}
	app.styles.Resize(app.width, app.height)
	app.portfolio.data = &portfolioViewData{
		account:       &account.Account{Name: "Brokerage"},
		valuation:     &investment.AccountValuation{Holdings: holdings},
		securityNames: names,
	}
	app.portfolio.buildHoldingsTable()

	lines := strings.Split(renderPortfolio(t, app), "\n")
	if len(lines) > app.height {
		t.Errorf("render is %d lines on a %d-line screen", len(lines), app.height)
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > app.width {
			t.Errorf("line %d is %d cells wide on a %d-cell screen", i, w, app.width)
			break
		}
	}
}
