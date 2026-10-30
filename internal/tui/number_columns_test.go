package tui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/loan"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/pricechart"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// wantNumbersFit fails when a fixed, right-aligned column of tbl is narrower
// than one of its cells, or when a cell in want is not in tbl, so that the
// test shows the wide cell was there.
func wantNumbersFit(t *testing.T, tbl *widget.Table, want ...string) {
	t.Helper()
	cells := map[string]bool{}
	cols := tbl.Columns()
	for _, row := range tbl.Rows() {
		for i, c := range cols {
			if i >= len(row) {
				continue
			}
			cells[row[i]] = true
			if n := utf8.RuneCountInString(row[i]); c.Width > 0 && c.Align == widget.AlignRight && n > c.Width {
				t.Errorf("%s column is %d cells, but %q is %d", c.Header, c.Width, row[i], n)
			}
		}
	}
	for _, w := range want {
		if !cells[w] {
			t.Errorf("no cell %q in the table", w)
		}
	}
}

// widthForTable returns the terminal width at which a register's table
// (ContentWidth - 4) is tableWidth cells.
func widthForTable(t *testing.T, tableWidth int) int {
	t.Helper()
	styles := widget.NewStyles()
	for w := 40; w <= 300; w++ {
		styles.Resize(w, 30)
		if styles.ContentWidth()-4 == tableWidth {
			return w
		}
	}
	t.Fatalf("no terminal width gives a %d-cell table", tableWidth)
	return 0
}

func TestFitNumberColumns(t *testing.T) {
	cols := []widget.Column{
		{Header: "Name", Width: 4, Align: widget.AlignLeft},
		{Header: "S", Width: 1, Align: widget.AlignCenter},
		{Header: "Memo", MinWidth: 3, Align: widget.AlignRight},
		{Header: "Amount", Width: 6, Align: widget.AlignRight},
	}
	rows := [][]string{
		{"Groceries", "✓", "a long memo", "$1.00"},
		{"Rent", "R", "", "-$123456.00"},
		{"Short"},
	}
	got := fitNumberColumns(cols, rows)
	want := []int{4, 1, 0, 11}
	for i, w := range want {
		if got[i].Width != w {
			t.Errorf("%s width = %d, want %d", got[i].Header, got[i].Width, w)
		}
	}
	if got[3].MinWidth != 6 {
		t.Errorf("Amount MinWidth = %d, want its usual width 6", got[3].MinWidth)
	}
	if cols[3].Width != 6 {
		t.Errorf("fitNumberColumns changed its input: Amount width = %d, want 6", cols[3].Width)
	}

	// A column is never narrower than its usual width.
	if w := fitNumberColumns(cols, [][]string{{"", "", "", "$1"}})[3].Width; w != 6 {
		t.Errorf("Amount width with narrow cells = %d, want 6", w)
	}
}

func TestBuildRegisterTable_WideAmount(t *testing.T) {
	app := registerAppWide(120)
	app.register.data.transactions[0].Amount = types.MustNewMoney("-123456789")
	app.buildRegisterTable()

	if got := len(app.register.table.Columns()); got != 6 {
		t.Fatalf("columns = %d, want 6 (Balance shown)", got)
	}
	wantNumbersFit(t, app.register.table, "-$123456789.00", "-$123456289.00")
	if view := app.renderRegister(); !contains(view, "-$123456789.00") {
		t.Errorf("rendered register cut the amount; want %q in the view", "-$123456789.00")
	}
}

func TestBuildRegisterTable_WideAmountHidesBalance(t *testing.T) {
	// With the usual widths the Balance column needs a 75-cell table: 5
	// separators, Date 10, S 1, Amount 12, Balance 15, and Payee and
	// Category at their minimum plus registerFlexMargin. A 14-cell amount
	// needs 2 more, so at 76 cells the Balance column must not show.
	width := widthForTable(t, 76)
	app := registerAppWide(width)
	app.buildRegisterTable()
	if got := len(app.register.table.Columns()); got != 6 {
		t.Fatalf("usual amounts: columns = %d, want 6 (Balance shown)", got)
	}

	app = registerAppWide(width)
	app.register.data.transactions[0].Amount = types.MustNewMoney("-123456789")
	app.buildRegisterTable()
	if got := len(app.register.table.Columns()); got != 5 {
		t.Errorf("wide amount: columns = %d, want 5 (Balance hidden)", got)
	}
	if got := len(app.register.table.Rows()[0]); got != 5 {
		t.Errorf("wide amount: row cells = %d, want 5", got)
	}
	wantNumbersFit(t, app.register.table, "-$123456789.00")

	// A resize to the same width keeps the decision.
	app.handleWindowSize(width, 30)
	if got := len(app.register.table.Columns()); got != 5 {
		t.Errorf("after resize: columns = %d, want 5", got)
	}
}

// investmentRegisterApp is an investment register on a terminal of the given
// width, with one Buy of the given total.
func investmentRegisterApp(width int, total string) *App {
	styles := widget.NewStyles()
	styles.Resize(width, 30)
	return &App{
		currentView: ViewInvestmentRegister,
		width:       width,
		height:      30,
		styles:      styles,
		investmentRegister: investmentRegisterViewState{data: &investmentRegisterData{
			account:       &account.Account{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Brokerage", Active: true},
			securityNames: make(map[types.ID]string),
			transactions:  []*investment.Transaction{invTxn(investment.TransactionTypeBuy, total)},
		}},
	}
}

func TestBuildInvestmentRegisterTable_WideTotal(t *testing.T) {
	app := investmentRegisterApp(160, "-123456789")
	app.investmentRegister.buildTable(app.styles)

	if got := len(app.investmentRegister.table.Columns()); got != 8 {
		t.Fatalf("columns = %d, want 8 (Balance shown)", got)
	}
	wantNumbersFit(t, app.investmentRegister.table, "-$123456789.00")
}

func TestBuildInvestmentRegisterTable_WideTotalHidesBalance(t *testing.T) {
	// The usual columns with Balance need a 98-cell table. A 14-cell total
	// needs 2 more, so at 99 cells the Balance column must not show.
	width := widthForTable(t, 99)
	app := investmentRegisterApp(width, "-100")
	app.investmentRegister.buildTable(app.styles)
	if got := len(app.investmentRegister.table.Columns()); got != 8 {
		t.Fatalf("usual total: columns = %d, want 8 (Balance shown)", got)
	}

	app = investmentRegisterApp(width, "-123456789")
	app.investmentRegister.buildTable(app.styles)
	if got := len(app.investmentRegister.table.Columns()); got != 7 {
		t.Errorf("wide total: columns = %d, want 7 (Balance hidden)", got)
	}
	if got := len(app.investmentRegister.table.Rows()[0]); got != 7 {
		t.Errorf("wide total: row cells = %d, want 7", got)
	}
	wantNumbersFit(t, app.investmentRegister.table, "-$123456789.00")

	app.handleWindowSize(width, 30)
	if got := len(app.investmentRegister.table.Columns()); got != 7 {
		t.Errorf("after resize: columns = %d, want 7", got)
	}
}

func TestBuildReconciliationTable_WideAmount(t *testing.T) {
	txn := &transaction.Transaction{
		BaseModel: types.BaseModel{ID: types.NewID()},
		Date:      types.Today(),
		Amount:    types.MustNewMoney("-123456789"),
	}
	app := &App{
		reconciliation: reconciliationViewState{data: &reconciliationViewData{
			candidates:    []*transaction.Transaction{txn},
			checkedIDs:    make(map[types.ID]bool),
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		}},
	}
	app.buildReconciliationTable()
	wantNumbersFit(t, app.reconciliation.table, "-$123456789.00")
}

func TestBuildScheduledTable_WideAmount(t *testing.T) {
	st := &scheduled.Transaction{
		BaseModel: types.BaseModel{ID: types.NewID()},
		AccountID: types.NewID(),
		Frequency: scheduled.FrequencyMonthly,
		NextDate:  types.Today(),
		Amount:    types.NullableMoney{Money: types.MustNewMoney("-123456789"), Valid: true},
	}
	app := &App{
		styles: widget.NewStyles(),
		scheduled: scheduledViewState{data: &scheduledViewData{
			allTxns:       []*scheduled.Transaction{st},
			payeeNames:    make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
		}},
	}
	app.buildScheduledTable()
	wantNumbersFit(t, app.scheduled.table, "-$123456789.00")
}

func TestBuildPortfolioTables_WideAmounts(t *testing.T) {
	secID := types.NewID()
	s := portfolioViewState{data: &portfolioViewData{
		valuation: &investment.AccountValuation{
			Holdings: []investment.Holding{{
				SecurityID:        secID,
				Shares:            types.MustNewQuantity("1234567.891"),
				AvgCost:           types.MustNewMoney("1234567"),
				CurrentPrice:      types.MustNewMoney("1234567"),
				PriceDate:         types.NewDate(2025, time.March, 14),
				MarketValue:       types.MustNewMoney("123456789"),
				CostBasis:         types.MustNewMoney("123456789"),
				GainLoss:          types.MustNewMoney("-1234567"),
				DividendsReceived: types.MustNewMoney("1234567"),
				RealizedGain:      types.MustNewMoney("-1234567"),
				FeesPaid:          types.MustNewMoney("123456"),
				TotalReturn:       types.MustNewMoney("-123456789"),
				HasPricing:        true,
			}},
		},
		securityNames: map[types.ID]string{secID: "FICT"},
		lotDetails: []investment.LotDetail{{
			LotID:        types.NewID(),
			PurchaseDate: types.NewDate(2024, time.June, 1),
			Shares:       types.MustNewQuantity("123456.789012"),
			CostPerShare: types.MustNewMoney("1234567890"),
			CostBasis:    types.MustNewMoney("12345678901"),
			CurrentValue: types.MustNewMoney("12345678901"),
			GainLoss:     types.MustNewMoney("-12345678901"),
			GainPct:      -1234567.25,
		}},
	}}

	s.buildHoldingsTable()
	wantNumbersFit(t, s.holdingsTable, "1234567.891", "$1234567.00", "$123456789.00",
		"-$1234567.00", "-$123456.00", "-$123456789.00")

	s.buildLotsTable()
	wantNumbersFit(t, s.lotsTable, "123456.789012", "$1234567890.00", "$12345678901.00",
		"-$12345678901.00", "-1234567.25%")
}

func TestBuildPriceTables_WidePrice(t *testing.T) {
	secID := types.NewID()
	big := types.MustNewMoney("1234567890123")
	s := priceViewState{data: &priceViewData{
		latestPrices: []*price.LatestPrice{{
			SecurityID: secID,
			Ticker:     "FICT",
			Name:       "Fictional Fund",
			Date:       types.NewDate(2025, time.March, 14),
			Price:      big,
		}},
		prices: []*price.Price{price.NewPrice(secID, types.NewDate(2025, time.March, 14), big, price.SourceManual)},
	}}

	s.buildListTable()
	wantNumbersFit(t, s.listTable, "$1234567890123.00")

	s.buildTable()
	wantNumbersFit(t, s.table, "$1234567890123.00")
}

func TestBuildAmortizationTable_WideAmounts(t *testing.T) {
	s := amortizationViewState{data: &amortizationViewData{
		hasSchedule: true,
		aprValid:    true,
		projection: loan.Projection{Rows: []loan.Row{{
			N:            123456,
			Date:         types.NewDate(2030, time.January, 1),
			TotalDraft:   types.MustNewMoney("1234567890"),
			Interest:     types.MustNewMoney("1234567890"),
			Principal:    types.MustNewMoney("1234567890"),
			Escrow:       types.MustNewMoney("123456789"),
			BalanceAfter: types.MustNewMoney("12345678901"),
		}}},
	}}
	s.buildTable()
	wantNumbersFit(t, s.table, "123456", "$1234567890.00", "$123456789.00", "$12345678901.00")
}

// A wide number must not push the columns on its right out of a narrow
// table. Without Balance the investment register is 82 cells; a 16-cell
// share count makes it 86, so at 84 cells the Type column gives up 2 cells
// and the Shares and Total cells stay whole.
func TestInvestmentRegister_WideSharesNarrowTable(t *testing.T) {
	app := investmentRegisterApp(widthForTable(t, 84), "-100.12")
	txn := app.investmentRegister.data.transactions[0]
	txn.Shares = types.NullableQuantity{Quantity: types.MustNewQuantity("100.123456789012"), Valid: true}
	app.investmentRegister.buildTable(app.styles)
	if got := len(app.investmentRegister.table.Columns()); got != 7 {
		t.Fatalf("columns = %d, want 7 (Balance hidden)", got)
	}

	out := app.investmentRegister.table.Render(app.styles, 84, 4)
	for _, want := range []string{"Buy", "100.123456789012", "-$100.12"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered table cut %q:\n%s", want, out)
		}
	}
}

// When the table has no text column that can give width, a wide amount goes
// back to its usual width: only that cell is cut, and the other amounts stay
// whole. The reconciliation table needs 53 cells, or 55 with a 14-cell
// amount.
func TestReconciliation_WideAmountNarrowTable(t *testing.T) {
	txn := func(amount string) *transaction.Transaction {
		return &transaction.Transaction{
			BaseModel: types.BaseModel{ID: types.NewID()},
			Date:      types.Today(),
			Amount:    types.MustNewMoney(amount),
		}
	}
	app := &App{
		reconciliation: reconciliationViewState{data: &reconciliationViewData{
			candidates:    []*transaction.Transaction{txn("-123456789"), txn("-25")},
			checkedIDs:    make(map[types.ID]bool),
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		}},
	}
	app.buildReconciliationTable()

	out := app.reconciliation.table.Render(widget.NewStyles(), 53, 4)
	if !strings.Contains(out, "-$25.00") {
		t.Errorf("rendered table cut -$25.00:\n%s", out)
	}
}

// The prices list beside the chart is as wide as its columns, so a wide price
// does not cut the name or the date.
func TestPriceList_WidePriceBesideChart(t *testing.T) {
	name := "Fictional Total World Stock Fund" // 32 cells, the Name column's width
	s := priceViewState{data: &priceViewData{
		latestPrices: []*price.LatestPrice{{
			SecurityID: types.NewID(),
			Ticker:     "FICT",
			Name:       name,
			Date:       types.NewDate(2025, time.March, 14),
			Price:      types.MustNewMoney("1000000000000000"),
		}},
	}}
	s.buildListTable()

	out := s.composeListBody(widget.NewStyles(), pricechart.MinContentWidth, 4)
	for _, want := range []string{name, "$1000000000000000.00", "2025-03-14"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered list cut %q:\n%s", want, out)
		}
	}
}
