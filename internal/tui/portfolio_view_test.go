package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

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
		portfolioData: &portfolioViewData{
			securityNames: map[types.ID]string{secID: "AAPL"},
		},
	}

	row := app.formatHoldingRow(holding)

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
		portfolioData: &portfolioViewData{
			securityNames: map[types.ID]string{secID: "MSFT"},
		},
	}

	row := app.formatHoldingRow(holding)

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
		portfolioData: &portfolioViewData{
			securityNames: map[types.ID]string{secID: "GOOG"},
		},
	}

	row := app.formatHoldingRow(holding)

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
		portfolioData: &portfolioViewData{
			securityNames: map[types.ID]string{secID: "DIV"},
		},
	}

	row := app.formatHoldingRow(holding)

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
		portfolioData: &portfolioViewData{
			securityNames: map[types.ID]string{secID: "MRG"},
		},
	}

	row := app.formatHoldingRow(holding)

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
		portfolioData: &portfolioViewData{
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
		},
	}

	summary := app.renderPortfolioSummary(100)

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
		portfolioData: &portfolioViewData{
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
		},
	}

	summary := app.renderPortfolioSummary(100)

	if !strings.Contains(summary, "—") {
		t.Error("nil TotalReturnPct should render as '—' placeholder")
	}
}

func TestPortfolioSummaryBar_PartialRealizedMarker(t *testing.T) {
	acctID := types.NewID()
	trPct := 30.0

	app := &App{
		styles: testStyles(),
		portfolioData: &portfolioViewData{
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
		},
	}

	summary := app.renderPortfolioSummary(100)

	if !strings.Contains(summary, "(partial)") {
		t.Errorf("expected '(partial)' marker when AnyRealizedUnavailable=true; got %q", summary)
	}
}

func TestPortfolioSummaryBar_NoPartialMarkerWhenAllAvailable(t *testing.T) {
	acctID := types.NewID()
	trPct := 30.0

	app := &App{
		styles: testStyles(),
		portfolioData: &portfolioViewData{
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
		},
	}

	summary := app.renderPortfolioSummary(100)

	if strings.Contains(summary, "(partial)") {
		t.Errorf("expected no '(partial)' marker when AnyRealizedUnavailable=false; got %q", summary)
	}
}

func TestPortfolioSummaryBar_NilValuation(t *testing.T) {
	app := &App{
		styles:        testStyles(),
		portfolioData: nil,
	}

	summary := app.renderPortfolioSummary(100)
	if summary != "" {
		t.Errorf("summary should be empty for nil portfolio data, got %q", summary)
	}
}

func TestBuildPortfolioHoldingsTable(t *testing.T) {
	secID := types.NewID()

	app := &App{
		portfolioData: &portfolioViewData{
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
		},
	}

	app.buildPortfolioHoldingsTable()

	if app.portfolioHoldingsTable == nil {
		t.Fatal("holdings table should be created")
	}

	rows := app.portfolioHoldingsTable.Rows()
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
		portfolioData: &portfolioViewData{
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
		},
	}

	app.buildPortfolioHoldingsTable()

	if app.portfolioHoldingsTable == nil {
		t.Fatal("holdings table should be created")
	}

	rows := app.portfolioHoldingsTable.Rows()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestBuildPortfolioHoldingsTable_NilData(t *testing.T) {
	app := &App{
		portfolioData: nil,
	}

	// Should not panic
	app.buildPortfolioHoldingsTable()

	if app.portfolioHoldingsTable != nil {
		t.Error("holdings table should be nil for nil data")
	}
}

func TestBuildPortfolioLotsTable(t *testing.T) {
	app := &App{
		portfolioData: &portfolioViewData{
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
		},
	}

	app.buildPortfolioLotsTable()

	if app.portfolioLotsTable == nil {
		t.Fatal("lots table should be created")
	}

	rows := app.portfolioLotsTable.Rows()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if rows[0][0] != "01/15/24" {
		t.Errorf("first lot purchase date = %q, want %q", rows[0][0], "01/15/24")
	}
}

func TestBuildPortfolioLotsTable_NilData(t *testing.T) {
	app := &App{
		portfolioData: nil,
	}

	// Should not panic
	app.buildPortfolioLotsTable()

	if app.portfolioLotsTable != nil {
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
		styles:        testStyles(),
		portfolioData: nil,
	}

	rendered := app.renderPortfolioView()
	if !strings.Contains(rendered, "Loading portfolio...") {
		t.Error("should show loading message when data is nil")
	}
}

func TestRenderPortfolioView_NoHoldings(t *testing.T) {
	app := &App{
		width:  120,
		height: 40,
		styles: testStyles(),
		portfolioData: &portfolioViewData{
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
		},
	}

	rendered := app.renderPortfolioView()
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
		portfolioData: &portfolioViewData{
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
		portfolioMode: portfolioViewHoldings,
	}

	// Build the holdings table first
	app.buildPortfolioHoldingsTable()

	rendered := app.renderPortfolioView()
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
		portfolioData: &portfolioViewData{
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
		portfolioMode: portfolioViewLots,
	}

	// Build the lots table
	app.buildPortfolioLotsTable()

	rendered := app.renderPortfolioView()
	if !strings.Contains(rendered, "Lots for AAPL") {
		t.Error("should show lot detail header with security ticker")
	}
}

func TestPortfolioViewToggle_RegisterToPortfolio(t *testing.T) {
	sidebar := NewSidebar()
	sidebar.SetFocused(false)

	app := &App{
		currentView: ViewInvestmentRegister,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     sidebar,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
		},
		investmentTable: widget.NewTable(nil),
	}

	// Simulate pressing 'p' to switch to portfolio
	msg := tea.KeyPressMsg{Code: 'p', Text: "p"}
	_, cmd := app.handleInvestmentRegisterKeys(msg)

	if app.currentView != ViewPortfolio {
		t.Errorf("view = %v, want ViewPortfolio", app.currentView)
	}
	if app.portfolioData != nil {
		t.Error("portfolio data should be nil (cleared for loading)")
	}
	// Command should be non-nil (loading portfolio data)
	if cmd == nil {
		t.Error("should return a command to load portfolio data")
	}
}

func TestPortfolioViewToggle_PortfolioToRegister(t *testing.T) {
	acctID := types.NewID()
	sidebar := NewSidebar()
	sidebar.SetFocused(false)

	app := &App{
		currentView: ViewPortfolio,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     sidebar,
		portfolioData: &portfolioViewData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
		},
		portfolioMode:          portfolioViewHoldings,
		portfolioHoldingsTable: widget.NewTable(nil),
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
	sidebar := NewSidebar()
	sidebar.SetFocused(false)

	app := &App{
		width:       120,
		height:      40,
		currentView: ViewPortfolio,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     sidebar,
		portfolioData: &portfolioViewData{
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
		portfolioMode: portfolioViewHoldings,
	}

	// Build holdings table and set cursor to first row
	app.buildPortfolioHoldingsTable()
	app.portfolioHoldingsTable.SetFocused(true)

	// Press Enter to drill down
	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	_, cmd := app.handlePortfolioKeys(msg)

	if app.portfolioMode != portfolioViewLots {
		t.Errorf("mode = %v, want portfolioViewLots", app.portfolioMode)
	}
	if cmd == nil {
		t.Error("should return a command to load lot detail")
	}
}

func TestPortfolioKeys_LotDrillDown_NonLotTracking(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()
	sidebar := NewSidebar()
	sidebar.SetFocused(false)

	app := &App{
		width:       120,
		height:      40,
		currentView: ViewPortfolio,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     sidebar,
		portfolioData: &portfolioViewData{
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
		portfolioMode: portfolioViewHoldings,
	}

	app.buildPortfolioHoldingsTable()
	app.portfolioHoldingsTable.SetFocused(true)

	// Press Enter - should NOT drill down for non-lot-tracking
	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	_, cmd := app.handlePortfolioKeys(msg)

	if app.portfolioMode != portfolioViewHoldings {
		t.Errorf("mode should remain portfolioViewHoldings for non-lot-tracking")
	}
	if cmd != nil {
		t.Error("should not return a command for non-lot-tracking accounts")
	}
}

func TestPortfolioKeys_EscapeFromLots(t *testing.T) {
	secID := types.NewID()
	sidebar := NewSidebar()
	sidebar.SetFocused(false)

	app := &App{
		currentView: ViewPortfolio,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     sidebar,
		portfolioData: &portfolioViewData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
			lotDetails:    []investment.LotDetail{},
			lotSecurityID: secID,
		},
		portfolioMode:          portfolioViewLots,
		portfolioHoldingsTable: widget.NewTable(nil),
		portfolioLotsTable:     widget.NewTable(nil),
	}

	// Press Escape from lot view - should go back to holdings
	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
	_, _ = app.handlePortfolioKeys(msg)

	if app.portfolioMode != portfolioViewHoldings {
		t.Errorf("mode = %v, want portfolioViewHoldings after Esc from lots", app.portfolioMode)
	}
	if app.portfolioData.lotDetails != nil {
		t.Error("lot details should be cleared")
	}
}

func TestPortfolioKeys_EscapeFromHoldings(t *testing.T) {
	acctID := types.NewID()
	sidebar := NewSidebar()
	sidebar.SetFocused(false)

	app := &App{
		currentView: ViewPortfolio,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     sidebar,
		portfolioData: &portfolioViewData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
		},
		portfolioMode:          portfolioViewHoldings,
		portfolioHoldingsTable: widget.NewTable(nil),
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
	sidebar := NewSidebar()
	sidebar.SetFocused(false)

	app := &App{
		width:       120,
		height:      40,
		currentView: ViewPortfolio,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     sidebar,
		portfolioData: &portfolioViewData{
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
		portfolioMode: portfolioViewHoldings,
	}

	app.buildPortfolioHoldingsTable()
	app.portfolioHoldingsTable.SetFocused(true)

	// Move down
	downMsg := tea.KeyPressMsg{Code: tea.KeyDown}
	app.handlePortfolioKeys(downMsg)
	if app.portfolioHoldingsTable.Cursor() != 1 {
		t.Errorf("cursor = %d after down, want 1", app.portfolioHoldingsTable.Cursor())
	}

	// Move up
	upMsg := tea.KeyPressMsg{Code: tea.KeyUp}
	app.handlePortfolioKeys(upMsg)
	if app.portfolioHoldingsTable.Cursor() != 0 {
		t.Errorf("cursor = %d after up, want 0", app.portfolioHoldingsTable.Cursor())
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

	if app.portfolioData == nil {
		t.Fatal("portfolio data should be set after loaded msg")
	}
	if app.portfolioMode != portfolioViewHoldings {
		t.Errorf("mode = %v, want portfolioViewHoldings", app.portfolioMode)
	}
	if app.portfolioHoldingsTable == nil {
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
		portfolioData: &portfolioViewData{
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
		portfolioMode: portfolioViewLots,
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

	if app.portfolioData.lotDetails == nil {
		t.Fatal("lot details should be set after lot detail msg")
	}
	if len(app.portfolioData.lotDetails) != 1 {
		t.Errorf("expected 1 lot detail, got %d", len(app.portfolioData.lotDetails))
	}
	if app.portfolioData.lotSecurityID != secID {
		t.Error("lot security ID should be set")
	}
	if app.portfolioLotsTable == nil {
		t.Error("lots table should be built after lot detail msg")
	}
}

func TestSelectedHolding(t *testing.T) {
	secID := types.NewID()

	app := &App{
		portfolioData: &portfolioViewData{
			valuation: &investment.AccountValuation{
				Holdings: []investment.Holding{
					{SecurityID: secID, Shares: types.MustNewQuantity("100"), HasPricing: true},
				},
			},
			securityNames: map[types.ID]string{secID: "AAPL"},
		},
	}

	app.buildPortfolioHoldingsTable()

	h := app.selectedHolding()
	if h == nil {
		t.Fatal("selected holding should not be nil")
	}
	if h.SecurityID != secID {
		t.Errorf("security ID = %v, want %v", h.SecurityID, secID)
	}
}

func TestSelectedHolding_NilData(t *testing.T) {
	app := &App{
		portfolioData: nil,
	}

	h := app.selectedHolding()
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
		portfolioMode:          portfolioViewHoldings,
		portfolioHoldingsTable: widget.NewTable(nil),
		portfolioLotsTable:     widget.NewTable(nil),
	}

	tbl := app.activePortfolioTable()
	if tbl != app.portfolioHoldingsTable {
		t.Error("should return holdings table in holdings mode")
	}
}

func TestActivePortfolioTable_LotsMode(t *testing.T) {
	app := &App{
		portfolioMode:          portfolioViewLots,
		portfolioHoldingsTable: widget.NewTable(nil),
		portfolioLotsTable:     widget.NewTable(nil),
	}

	tbl := app.activePortfolioTable()
	if tbl != app.portfolioLotsTable {
		t.Error("should return lots table in lots mode")
	}
}

func TestActivePortfolioTable_NilTables(t *testing.T) {
	app := &App{
		portfolioMode: portfolioViewHoldings,
	}

	// Should not panic, returns a placeholder
	tbl := app.activePortfolioTable()
	if tbl == nil {
		t.Error("should return a non-nil placeholder table")
	}
}
