package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/loan"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// The views show an account's amounts in the account's currency, and a
// price in its security's currency, not always in dollars.

func TestRegister_AccountCurrency(t *testing.T) {
	app := registerAppWide(120)
	app.register.data.account.Currency = "EUR"
	app.buildRegisterTable()

	row := app.register.table.Rows()[0]
	if row[4] != "-€100.00" || row[5] != "€400.00" {
		t.Errorf("amount, balance = %q, %q; want -€100.00, €400.00", row[4], row[5])
	}
	if view := app.renderRegister(); !strings.Contains(view, "Bal: €400.00") {
		t.Error("the title should show the balance in euros")
	}
}

func TestInvestmentRegister_AccountCurrency(t *testing.T) {
	app := investmentRegisterApp(160, "-1500000")
	app.investmentRegister.data.account.Currency = "JPY"
	app.investmentRegister.buildTable(app.styles)

	wantNumbersFit(t, app.investmentRegister.table, "JPY -1500000.00")
}

func TestReconciliation_AccountCurrency(t *testing.T) {
	txn := &transaction.Transaction{
		BaseModel: types.BaseModel{ID: types.NewID()},
		Date:      types.Today(),
		Amount:    types.MustNewMoney("-25"),
	}
	app := &App{
		reconciliation: reconciliationViewState{data: &reconciliationViewData{
			account:       &account.Account{Name: "Fictional Bank", Currency: "GBP"},
			candidates:    []*transaction.Transaction{txn},
			checkedIDs:    make(map[types.ID]bool),
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		}},
	}
	app.buildReconciliationTable()
	if got := app.reconciliation.table.Rows()[0][5]; got != "-£25.00" {
		t.Errorf("amount = %q, want -£25.00", got)
	}
}

func TestScheduledAndDashboard_AccountCurrency(t *testing.T) {
	acctID := types.NewID()
	st := &scheduled.Transaction{
		BaseModel: types.BaseModel{ID: types.NewID()},
		AccountID: acctID,
		Frequency: scheduled.FrequencyMonthly,
		NextDate:  types.Today(),
		Amount:    types.NullableMoney{Money: types.MustNewMoney("-50"), Valid: true},
	}
	currencies := map[types.ID]string{acctID: "GBP"}

	app := &App{
		styles: widget.NewStyles(),
		scheduled: scheduledViewState{data: &scheduledViewData{
			allTxns:           []*scheduled.Transaction{st},
			payeeNames:        make(map[types.ID]string),
			accountNames:      make(map[types.ID]string),
			categoryNames:     make(map[types.ID]string),
			accountCurrencies: currencies,
		}},
	}
	app.buildScheduledTable()
	wantNumbersFit(t, app.scheduled.table, "-£50.00")

	dash := dashboardViewState{data: &dashboardData{
		payeeNames:        make(map[types.ID]string),
		accountNames:      make(map[types.ID]string),
		accountCurrencies: currencies,
	}}
	if line := dash.formatScheduledItem(widget.NewStyles(), st, true); !strings.Contains(line, "-£50.00") {
		t.Errorf("Dashboard due line = %q, want the amount in pounds", line)
	}
}

func TestPortfolio_AccountCurrency(t *testing.T) {
	secID := types.NewID()
	s := portfolioViewState{data: &portfolioViewData{
		account: &account.Account{Name: "Fictional Broker", Currency: "EUR"},
		valuation: &investment.AccountValuation{
			Holdings: []investment.Holding{{
				SecurityID:   secID,
				Shares:       types.MustNewQuantity("10"),
				CurrentPrice: types.MustNewMoney("42.5"),
				MarketValue:  types.MustNewMoney("425"),
				HasPricing:   true,
			}},
		},
		securityNames: map[types.ID]string{secID: "FICT"},
		lotDetails: []investment.LotDetail{{
			LotID:        types.NewID(),
			PurchaseDate: types.NewDate(2024, time.June, 1),
			Shares:       types.MustNewQuantity("10"),
			CostBasis:    types.MustNewMoney("400"),
		}},
	}}
	s.buildHoldingsTable()
	wantNumbersFit(t, s.holdingsTable, "€42.50", "€425.00")
	s.buildLotsTable()
	wantNumbersFit(t, s.lotsTable, "€400.00")
}

func TestPrices_SecurityCurrency(t *testing.T) {
	sec := security.NewSecurity("FICT", "Fictional Fund", security.TypeMutualFund)
	sec.Currency = "GBP"
	p := price.NewPrice(sec.ID, types.NewDate(2025, time.March, 14), types.MustNewMoney("42.5"), price.SourceManual)
	s := priceViewState{data: &priceViewData{
		latestPrices: []*price.LatestPrice{{
			SecurityID: sec.ID, Ticker: sec.Ticker, Name: sec.Name, Currency: "EUR",
			Date: p.Date, Price: p.Price,
		}},
		selectedSecurity: sec,
		prices:           []*price.Price{p},
	}}
	s.buildListTable()
	wantNumbersFit(t, s.listTable, "€42.50")
	s.buildTable()
	wantNumbersFit(t, s.table, "£42.50")
}

func TestAmortization_AccountCurrency(t *testing.T) {
	s := amortizationViewState{data: &amortizationViewData{
		account:     &account.Account{Name: "Fictional Loan", Currency: "EUR"},
		hasSchedule: true,
		aprValid:    true,
		owed:        types.MustNewMoney("1000"),
		projection: loan.Projection{Rows: []loan.Row{{
			N:            1,
			Date:         types.NewDate(2030, time.January, 1),
			TotalDraft:   types.MustNewMoney("100"),
			BalanceAfter: types.MustNewMoney("900"),
		}}},
	}}
	s.buildTable()
	wantNumbersFit(t, s.table, "€100.00", "€900.00")
	if line := s.statsLine(widget.NewStyles(), 100); !strings.Contains(line, "€1000.00") {
		t.Errorf("stats = %q, want the balance in euros", line)
	}
}

func TestSplitDialog_AccountCurrency(t *testing.T) {
	editor := newSplitEditor(splitOpen{
		amount:          types.MustNewMoney("100"),
		categoryOptions: []string{"(None)"},
		categoryIDs:     []types.ID{types.NilID},
		currency:        "EUR",
	})
	out := editor.Render(widget.NewStyles())
	for _, want := range []string{"Total: €100.00", "Imbalance: €100.00"} {
		if !strings.Contains(out, want) {
			t.Errorf("split dialog should show %q", want)
		}
	}
}

func TestDialogs_AccountCurrency(t *testing.T) {
	err := &account.HasBalanceError{InvestmentCash: true, Balance: types.MustNewMoney("12.5")}
	if msg := closeAccountErrorMessage(err, "EUR"); !strings.Contains(msg, "€12.50") {
		t.Errorf("close message = %q, want the cash in euros", msg)
	}

	lot := &investment.Lot{
		Shares:       types.MustNewQuantity("10"),
		CostPerShare: types.MustNewMoney("150"),
		PurchaseDate: types.NewDate(2023, time.June, 15),
	}
	if label := buildLotLabel(lot, "GBP"); !strings.HasSuffix(label, "@ £150.00") {
		t.Errorf("lot label = %q, want the cost in pounds", label)
	}

	accounts := []*account.Account{{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Fictional Bank", Currency: "EUR", Active: true}}
	w := NewPaycheckWizard([]string{"(None)"}, []types.ID{types.NilID}, accounts)
	if out := w.Render(widget.NewStyles()); !strings.Contains(out, "Net to Fictional Bank: €0.00") {
		t.Error("the paycheck wizard should show the net in the deposit account's currency")
	}
}

// The openers give each dialog its account's currency: the split panel in a
// posting preview, and the seed of an investment dialog (the Sell and
// Transfer Shares lot labels).
func TestOpeners_AccountCurrency(t *testing.T) {
	acct := &account.Account{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Fictional Bank", Currency: "EUR", Active: true}
	catID := types.NewID()
	template := scheduled.NewTransaction(acct.ID, scheduled.FrequencyMonthly, types.Today())
	template.Amount = types.NullableMoney{Money: types.MustNewMoney("100"), Valid: true}
	template.Splits = scheduled.SplitCollection{scheduled.NewCategorizedSplit(template.ID, catID, types.MustNewMoney("100"))}
	p := NewSchedulePreviewDialog(template, []*account.Account{acct}, nil, []string{"(None)", "Salary"}, []types.ID{types.NilID, catID}, nil)
	if got := p.SplitDialog().currency; got != "EUR" {
		t.Errorf("preview split currency = %q, want EUR", got)
	}

	app := &App{investmentRegister: investmentRegisterViewState{data: &investmentRegisterData{account: acct}}}
	if seed, ok := app.takeInvestmentDialogSeed(); !ok || seed.currency != "EUR" {
		t.Errorf("seed currency = %q (ok %v), want EUR", seed.currency, ok)
	}
}
