package tui

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

func TestRenderInvestmentTotalReturnLines_PartialMarker(t *testing.T) {
	trPct := 30.0
	app := &App{
		styles: testStyles(),
		investmentRegister: &investmentRegisterData{
			valuation: &investment.AccountValuation{
				AccountID:              types.NewID(),
				TotalGainLoss:          types.MustNewMoney("200"),
				RealizedGain:           types.MustNewMoney("-0.29"),
				DividendsReceived:      types.MustNewMoney("50"),
				InterestReceived:       types.MustNewMoney("5"),
				FeesPaid:               types.MustNewMoney("0"),
				TotalReturn:            types.MustNewMoney("254.71"),
				TotalReturnPct:         &trPct,
				AnyRealizedUnavailable: true,
			},
		},
	}

	breakdown, total := app.renderInvestmentTotalReturnLines()
	if !strings.Contains(breakdown, "(partial)") {
		t.Errorf("expected '(partial)' on breakdown line; got %q", breakdown)
	}
	if !strings.Contains(total, "(partial)") {
		t.Errorf("expected '(partial)' on total-return line; got %q", total)
	}
}

func TestRenderInvestmentTotalReturnLines_NoPartialMarker(t *testing.T) {
	trPct := 30.0
	app := &App{
		styles: testStyles(),
		investmentRegister: &investmentRegisterData{
			valuation: &investment.AccountValuation{
				AccountID:              types.NewID(),
				TotalGainLoss:          types.MustNewMoney("200"),
				RealizedGain:           types.MustNewMoney("-0.29"),
				DividendsReceived:      types.MustNewMoney("50"),
				InterestReceived:       types.MustNewMoney("5"),
				FeesPaid:               types.MustNewMoney("0"),
				TotalReturn:            types.MustNewMoney("254.71"),
				TotalReturnPct:         &trPct,
				AnyRealizedUnavailable: false,
			},
		},
	}

	breakdown, total := app.renderInvestmentTotalReturnLines()
	if strings.Contains(breakdown, "(partial)") {
		t.Errorf("unexpected '(partial)' on breakdown line; got %q", breakdown)
	}
	if strings.Contains(total, "(partial)") {
		t.Errorf("unexpected '(partial)' on total-return line; got %q", total)
	}
}

func TestInvestmentRegisterData(t *testing.T) {
	acct := &account.Account{
		BaseModel: types.NewBaseModel(),
		Name:      "Brokerage",
		Type:      account.TypeInvestment,
	}

	data := &investmentRegisterData{
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

func TestInvestmentRegisterData_WithTransactions(t *testing.T) {
	acctID := types.NewID()
	secID := types.NewID()
	date := types.NewDate(2024, time.March, 15)
	amount := types.MustNewMoney("1850.00")

	txn := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeBuy, amount, secID, types.MustNewQuantity("10"),
	)

	data := &investmentRegisterData{
		account: &account.Account{
			BaseModel: types.NewBaseModel(),
			Name:      "Brokerage",
			Type:      account.TypeInvestment,
		},
		transactions:  []*investment.Transaction{txn},
		securityNames: map[types.ID]string{secID: "AAPL"},
	}

	if len(data.transactions) != 1 {
		t.Errorf("expected 1 transaction, got %d", len(data.transactions))
	}
	if data.securityNames[secID] != "AAPL" {
		t.Errorf("security name = %q, want %q", data.securityNames[secID], "AAPL")
	}
}

func TestFormatInvestmentRegisterRow(t *testing.T) {
	acctID := types.NewID()
	secID := types.NewID()
	date := types.NewDate(2024, time.March, 15)
	amount := types.MustNewMoney("1850.00")

	txn := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeBuy, amount, secID, types.MustNewQuantity("10"),
	)
	txn.PricePerShare = types.NullableMoney{Money: types.MustNewMoney("185.00"), Valid: true}
	txn.Status = investment.TransactionStatusCleared

	app := &App{
		investmentRegister: &investmentRegisterData{
			securityNames: map[types.ID]string{secID: "AAPL"},
		},
	}

	row := app.formatInvestmentRegisterRow(txn)

	if len(row) != 7 {
		t.Fatalf("expected 7 columns, got %d", len(row))
	}

	// Date
	if row[0] != "03/15/2024" {
		t.Errorf("date = %q, want %q", row[0], "03/15/2024")
	}
	// Status
	if row[1] != "✓" {
		t.Errorf("status = %q, want %q", row[1], "✓")
	}
	// Type
	if row[2] != "Buy" {
		t.Errorf("type = %q, want %q", row[2], "Buy")
	}
	// Security
	if row[3] != "AAPL" {
		t.Errorf("security = %q, want %q", row[3], "AAPL")
	}
	// Shares
	if row[4] != "10" {
		t.Errorf("shares = %q, want %q", row[4], "10")
	}
	// Price
	if row[5] != "$185.00" {
		t.Errorf("price = %q, want %q", row[5], "$185.00")
	}
	// Total
	if row[6] != "$1850.00" {
		t.Errorf("total = %q, want %q", row[6], "$1850.00")
	}
}

func TestFormatInvestmentRegisterRow_PendingStatus(t *testing.T) {
	acctID := types.NewID()
	date := types.NewDate(2024, time.March, 15)
	amount := types.MustNewMoney("500.00")

	txn := investment.NewTransaction(acctID, date, investment.TransactionTypeDeposit, amount)
	txn.Status = investment.TransactionStatusPending

	app := &App{
		investmentRegister: &investmentRegisterData{
			securityNames: map[types.ID]string{},
		},
	}

	row := app.formatInvestmentRegisterRow(txn)

	if row[1] != " " {
		t.Errorf("status = %q, want %q for pending", row[1], " ")
	}
	if row[2] != "Deposit" {
		t.Errorf("type = %q, want %q", row[2], "Deposit")
	}
	// No security for deposit
	if row[3] != "" {
		t.Errorf("security = %q, want empty for deposit", row[3])
	}
	// No shares for deposit
	if row[4] != "" {
		t.Errorf("shares = %q, want empty for deposit", row[4])
	}
	// No price for deposit
	if row[5] != "" {
		t.Errorf("price = %q, want empty for deposit", row[5])
	}
}

func TestFormatInvestmentRegisterRow_ReconciledStatus(t *testing.T) {
	acctID := types.NewID()
	secID := types.NewID()
	date := types.NewDate(2024, time.June, 1)
	amount := types.MustNewMoney("50.00")

	txn := investment.NewTransaction(acctID, date, investment.TransactionTypeDividend, amount)
	txn.SecurityID = types.NullableID{ID: secID, Valid: true}
	txn.Status = investment.TransactionStatusReconciled

	app := &App{
		investmentRegister: &investmentRegisterData{
			securityNames: map[types.ID]string{secID: "MSFT"},
		},
	}

	row := app.formatInvestmentRegisterRow(txn)

	if row[1] != "R" {
		t.Errorf("status = %q, want %q for reconciled", row[1], "R")
	}
	if row[2] != "Dividend" {
		t.Errorf("type = %q, want %q", row[2], "Dividend")
	}
	if row[3] != "MSFT" {
		t.Errorf("security = %q, want %q", row[3], "MSFT")
	}
}

func TestFormatInvestmentRegisterRow_SellTransaction(t *testing.T) {
	acctID := types.NewID()
	secID := types.NewID()
	date := types.NewDate(2024, time.April, 10)
	amount := types.MustNewMoney("2000.00")

	txn := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeSell, amount, secID, types.MustNewQuantity("5"),
	)
	txn.PricePerShare = types.NullableMoney{Money: types.MustNewMoney("400.00"), Valid: true}

	app := &App{
		investmentRegister: &investmentRegisterData{
			securityNames: map[types.ID]string{secID: "GOOG"},
		},
	}

	row := app.formatInvestmentRegisterRow(txn)

	if row[2] != "Sell" {
		t.Errorf("type = %q, want %q", row[2], "Sell")
	}
	if row[3] != "GOOG" {
		t.Errorf("security = %q, want %q", row[3], "GOOG")
	}
	if row[4] != "5" {
		t.Errorf("shares = %q, want %q", row[4], "5")
	}
	if row[5] != "$400.00" {
		t.Errorf("price = %q, want %q", row[5], "$400.00")
	}
}

func TestFormatInvestmentRegisterRow_NoSecurity(t *testing.T) {
	acctID := types.NewID()
	date := types.NewDate(2024, time.May, 1)
	amount := types.MustNewMoney("25.00")

	txn := investment.NewTransaction(acctID, date, investment.TransactionTypeFee, amount)

	app := &App{
		investmentRegister: &investmentRegisterData{
			securityNames: map[types.ID]string{},
		},
	}

	row := app.formatInvestmentRegisterRow(txn)

	if row[2] != "Fee" {
		t.Errorf("type = %q, want %q", row[2], "Fee")
	}
	if row[3] != "" {
		t.Errorf("security = %q, want empty for fee", row[3])
	}
	if row[4] != "" {
		t.Errorf("shares = %q, want empty for fee", row[4])
	}
}

func TestBuildInvestmentRegisterTable(t *testing.T) {
	acctID := types.NewID()
	secID := types.NewID()
	date := types.NewDate(2024, time.March, 15)

	txn1 := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeBuy, types.MustNewMoney("1850.00"), secID, types.MustNewQuantity("10"),
	)
	txn2 := investment.NewTransaction(acctID, date, investment.TransactionTypeDeposit, types.MustNewMoney("5000.00"))

	app := &App{
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
			transactions:  []*investment.Transaction{txn1, txn2},
			securityNames: map[types.ID]string{secID: "AAPL"},
		},
	}

	app.buildInvestmentRegisterTable()

	if app.investmentTable == nil {
		t.Fatal("investmentTable should not be nil after build")
	}
	if app.investmentTable.RowCount() != 2 {
		t.Errorf("expected 2 rows, got %d", app.investmentTable.RowCount())
	}
}

func TestBuildInvestmentRegisterTable_Empty(t *testing.T) {
	app := &App{
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
			transactions:  []*investment.Transaction{},
			securityNames: map[types.ID]string{},
		},
	}

	app.buildInvestmentRegisterTable()

	if app.investmentTable == nil {
		t.Fatal("investmentTable should not be nil after build even with no transactions")
	}
	if app.investmentTable.RowCount() != 0 {
		t.Errorf("expected 0 rows, got %d", app.investmentTable.RowCount())
	}
}

func TestBuildInvestmentRegisterTable_NilData(t *testing.T) {
	app := &App{}
	app.buildInvestmentRegisterTable() // should not panic
	if app.investmentTable != nil {
		t.Error("investmentTable should be nil when investmentRegister is nil")
	}
}

func TestRenderInvestmentRegister_Loading(t *testing.T) {
	app := &App{
		styles: widget.NewStyles(),
	}
	app.styles.Resize(80, 24)

	output := app.renderInvestmentRegister()
	if !strings.Contains(output, "Loading investment register") {
		t.Error("should show loading message when investment register data is nil")
	}
}

func TestRenderInvestmentRegister_NoTransactions(t *testing.T) {
	app := &App{
		width:  80,
		height: 24,
		styles: widget.NewStyles(),
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
			transactions:  []*investment.Transaction{},
			securityNames: map[types.ID]string{},
			cashBalance:   types.ZeroMoney,
		},
	}
	app.styles.Resize(80, 24)
	app.buildInvestmentRegisterTable()

	output := app.renderInvestmentRegister()
	if !strings.Contains(output, "No investment transactions") {
		t.Error("should show 'No investment transactions' message")
	}
}

func TestRenderInvestmentRegister_WithData(t *testing.T) {
	acctID := types.NewID()
	secID := types.NewID()
	date := types.NewDate(2024, time.March, 15)

	txn := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeBuy, types.MustNewMoney("1850.00"), secID, types.MustNewQuantity("10"),
	)

	app := &App{
		width:  100,
		height: 30,
		styles: widget.NewStyles(),
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
			transactions:  []*investment.Transaction{txn},
			securityNames: map[types.ID]string{secID: "AAPL"},
			cashBalance:   types.MustNewMoney("3150.00"),
		},
	}
	app.styles.Resize(100, 30)
	app.buildInvestmentRegisterTable()

	output := app.renderInvestmentRegister()
	if !strings.Contains(output, "BROKERAGE") {
		t.Error("should contain account name in uppercase")
	}
	if !strings.Contains(output, "Cash:") {
		t.Error("should contain 'Cash:' label")
	}
}

func TestRenderInvestmentRegister_ShowsCashBalance(t *testing.T) {
	app := &App{
		width:  100,
		height: 30,
		styles: widget.NewStyles(),
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Investment Account",
				Type:      account.TypeInvestment,
			},
			transactions:  []*investment.Transaction{},
			securityNames: map[types.ID]string{},
			cashBalance:   types.MustNewMoney("5000.00"),
		},
	}
	app.styles.Resize(100, 30)
	app.buildInvestmentRegisterTable()

	output := app.renderInvestmentRegister()
	if !strings.Contains(output, "$5000.00") {
		t.Error("should show cash balance amount")
	}
}

func TestInvestmentRegisterLoadedMsg(t *testing.T) {
	acctID := types.NewID()
	data := &investmentRegisterData{
		account: &account.Account{
			BaseModel: types.BaseModel{ID: acctID},
			Name:      "Brokerage",
			Type:      account.TypeInvestment,
		},
		transactions:  []*investment.Transaction{},
		securityNames: map[types.ID]string{},
		cashBalance:   types.ZeroMoney,
	}

	app := &App{
		currentView: ViewInvestmentRegister,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
	}

	msg := investmentRegisterLoadedMsg{data: data}
	model, _ := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.investmentRegister == nil {
		t.Fatal("investment register data should be set")
	}
	if updatedApp.investmentTable == nil {
		t.Error("investment table should be built")
	}
}

func TestViewInvestmentRegisterString(t *testing.T) {
	v := ViewInvestmentRegister
	if v.String() != "Investment Register" {
		t.Errorf("ViewInvestmentRegister.String() = %q, want %q", v.String(), "Investment Register")
	}
}

func TestHandleInvestmentRegisterKeys_Navigation(t *testing.T) {
	acctID := types.NewID()
	secID := types.NewID()
	date := types.NewDate(2024, time.March, 15)

	txn1 := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeBuy, types.MustNewMoney("1000.00"), secID, types.MustNewQuantity("5"),
	)
	txn2 := investment.NewTransaction(acctID, date, investment.TransactionTypeDeposit, types.MustNewMoney("5000.00"))

	sidebar := NewSidebar()
	sidebar.SetFocused(false)

	app := &App{
		width:   80,
		height:  24,
		keys:    defaultKeyMap(),
		sidebar: sidebar,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
			transactions:  []*investment.Transaction{txn1, txn2},
			securityNames: map[types.ID]string{secID: "AAPL"},
		},
	}
	app.buildInvestmentRegisterTable()

	// Move down
	downKey := tea.KeyPressMsg{Code: tea.KeyDown}
	app.handleInvestmentRegisterKeys(downKey)

	if app.investmentTable.Cursor() != 1 {
		t.Errorf("cursor = %d, want 1 after down", app.investmentTable.Cursor())
	}

	// Move up
	upKey := tea.KeyPressMsg{Code: tea.KeyUp}
	app.handleInvestmentRegisterKeys(upKey)

	if app.investmentTable.Cursor() != 0 {
		t.Errorf("cursor = %d, want 0 after up", app.investmentTable.Cursor())
	}
}

func TestHandleInvestmentRegisterKeys_ToggleClear(t *testing.T) {
	acctID := types.NewID()
	secID := types.NewID()
	date := types.NewDate(2024, time.March, 15)

	txn := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeBuy, types.MustNewMoney("1000.00"), secID, types.MustNewQuantity("5"),
	)
	txn.Status = investment.TransactionStatusPending

	sidebar := NewSidebar()
	sidebar.SetFocused(false)

	app := &App{
		width:   80,
		height:  24,
		keys:    defaultKeyMap(),
		sidebar: sidebar,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
			transactions:  []*investment.Transaction{txn},
			securityNames: map[types.ID]string{secID: "AAPL"},
		},
	}
	app.buildInvestmentRegisterTable()

	// Press 'c' to toggle cleared - should return a command since it requires service call
	cKey := tea.KeyPressMsg{Code: 'c', Text: "c"}
	_, cmd := app.handleInvestmentRegisterKeys(cKey)

	// Without an investmentSvc, the command may be nil, but the key should be handled
	// The important thing is it doesn't panic
	_ = cmd
}

func TestSelectedInvestmentTransaction(t *testing.T) {
	acctID := types.NewID()
	secID := types.NewID()
	date := types.NewDate(2024, time.March, 15)

	txn1 := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeBuy, types.MustNewMoney("1000.00"), secID, types.MustNewQuantity("5"),
	)
	txn2 := investment.NewTransaction(acctID, date, investment.TransactionTypeDeposit, types.MustNewMoney("5000.00"))

	app := &App{
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
			transactions:  []*investment.Transaction{txn1, txn2},
			securityNames: map[types.ID]string{secID: "AAPL"},
		},
	}
	app.buildInvestmentRegisterTable()

	selected := app.selectedInvestmentTransaction()
	if selected == nil {
		t.Fatal("selectedInvestmentTransaction() returned nil")
	}
	if selected.Type != investment.TransactionTypeBuy {
		t.Errorf("selected type = %q, want %q", selected.Type, investment.TransactionTypeBuy)
	}

	// Move to second
	app.investmentTable.MoveDown()
	selected = app.selectedInvestmentTransaction()
	if selected == nil {
		t.Fatal("selectedInvestmentTransaction() returned nil after MoveDown")
	}
	if selected.Type != investment.TransactionTypeDeposit {
		t.Errorf("selected type = %q, want %q", selected.Type, investment.TransactionTypeDeposit)
	}
}

func TestSelectedInvestmentTransaction_NilData(t *testing.T) {
	app := &App{}
	selected := app.selectedInvestmentTransaction()
	if selected != nil {
		t.Error("selectedInvestmentTransaction() should return nil when no data")
	}
}

func TestInvestmentRegisterColumns(t *testing.T) {
	app := &App{
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
			transactions:  []*investment.Transaction{},
			securityNames: map[types.ID]string{},
		},
	}
	app.buildInvestmentRegisterTable()

	if app.investmentTable == nil {
		t.Fatal("investmentTable should not be nil")
	}

	cols := app.investmentTable.Columns()
	expectedHeaders := []string{"Date", "S", "Type", "Security", "Shares", "Price", "Total"}
	if len(cols) != len(expectedHeaders) {
		t.Fatalf("expected %d columns, got %d", len(expectedHeaders), len(cols))
	}
	for i, h := range expectedHeaders {
		if cols[i].Header != h {
			t.Errorf("column[%d].Header = %q, want %q", i, cols[i].Header, h)
		}
	}
}

func TestInvestmentRegisterView_FullScreenRender(t *testing.T) {
	acctID := types.NewID()
	secID := types.NewID()
	date := types.NewDate(2024, time.March, 15)

	txn := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeBuy, types.MustNewMoney("1850.00"), secID, types.MustNewQuantity("10"),
	)

	styles := widget.NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewInvestmentRegister,
		width:       100,
		height:      30,
		ready:       true,
		styles:      styles,
		sidebar:     NewSidebar(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		keys:        defaultKeyMap(),
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
			transactions:  []*investment.Transaction{txn},
			securityNames: map[types.ID]string{secID: "AAPL"},
			cashBalance:   types.MustNewMoney("3150.00"),
		},
	}
	app.buildInvestmentRegisterTable()

	content := app.renderContent(28)
	if !strings.Contains(content, "BROKERAGE") {
		t.Error("renderContent should contain investment register view content")
	}
}

func TestHandleInvestmentRegisterKeys_NewOpensTypeSelector(t *testing.T) {
	acctID := types.NewID()
	secID := types.NewID()
	date := types.NewDate(2024, time.March, 15)

	txn := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeBuy, types.MustNewMoney("1000.00"), secID, types.MustNewQuantity("5"),
	)

	sidebar := NewSidebar()
	sidebar.SetFocused(false)

	app := &App{
		width:   80,
		height:  24,
		keys:    defaultKeyMap(),
		sidebar: sidebar,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
			transactions:  []*investment.Transaction{txn},
			securityNames: map[types.ID]string{secID: "AAPL"},
		},
	}
	app.buildInvestmentRegisterTable()

	// Press 'n' to open transaction type selector
	nKey := tea.KeyPressMsg{Code: 'n', Text: "n"}
	app.handleInvestmentRegisterKeys(nKey)

	// Should open the investment type selector dialog
	if app.investmentTypeSelector == nil {
		t.Fatal("pressing 'n' should open the investment type selector dialog")
	}
	if !app.investmentTypeSelector.IsVisible() {
		t.Error("investment type selector dialog should be visible")
	}
	// Verify it has options for transaction types
	fields := app.investmentTypeSelector.Fields()
	if len(fields) != 1 {
		t.Fatalf("expected 1 field (type selector), got %d", len(fields))
	}
	if fields[0].Type != dialog.FieldSelect {
		t.Errorf("field type = %d, want dialog.FieldSelect (%d)", fields[0].Type, dialog.FieldSelect)
	}
	if len(fields[0].Options) == 0 {
		t.Error("type selector should have options")
	}
}

func TestHandleInvestmentRegisterKeys_EnterEditsTransaction(t *testing.T) {
	acctID := types.NewID()
	secID := types.NewID()
	date := types.NewDate(2024, time.March, 15)

	txn := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeBuy, types.MustNewMoney("1000.00"), secID, types.MustNewQuantity("5"),
	)

	sidebar := NewSidebar()
	sidebar.SetFocused(false)

	app := &App{
		width:   80,
		height:  24,
		keys:    defaultKeyMap(),
		sidebar: sidebar,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
			transactions:  []*investment.Transaction{txn},
			securityNames: map[types.ID]string{secID: "AAPL"},
		},
	}
	app.buildInvestmentRegisterTable()

	// Press Enter to edit the selected transaction
	enterKey := tea.KeyPressMsg{Code: tea.KeyEnter}
	app.handleInvestmentRegisterKeys(enterKey)

	// Should open the investment type selector in edit mode with the transaction's type pre-selected
	if app.investmentTypeSelector == nil {
		t.Fatal("pressing Enter should open the investment type selector dialog in edit mode")
	}
	if !app.investmentTypeSelector.IsVisible() {
		t.Error("investment type selector dialog should be visible")
	}
	// In edit mode, the type should be pre-selected to the transaction's type (Buy)
	fields := app.investmentTypeSelector.Fields()
	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}
	// Find the Buy option index
	buyIdx := -1
	for i, opt := range fields[0].Options {
		if opt == "Buy" {
			buyIdx = i
			break
		}
	}
	if buyIdx < 0 {
		t.Fatal("Buy should be in the type selector options")
	}
	if fields[0].SelectedIndex != buyIdx {
		t.Errorf("selected index = %d, want %d (Buy)", fields[0].SelectedIndex, buyIdx)
	}
}

func TestHandleInvestmentRegisterKeys_EnterNoOpsWithNoTransaction(t *testing.T) {
	sidebar := NewSidebar()
	sidebar.SetFocused(false)

	app := &App{
		width:   80,
		height:  24,
		keys:    defaultKeyMap(),
		sidebar: sidebar,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
			transactions:  []*investment.Transaction{},
			securityNames: map[types.ID]string{},
		},
	}
	app.buildInvestmentRegisterTable()

	// Press Enter with no transactions
	enterKey := tea.KeyPressMsg{Code: tea.KeyEnter}
	app.handleInvestmentRegisterKeys(enterKey)

	// Should not open any dialog
	if app.investmentTypeSelector != nil {
		t.Error("pressing Enter with no transactions should not open the type selector")
	}
}

func TestHandleInvestmentRegisterKeys_DeleteExistingTransaction(t *testing.T) {
	acctID := types.NewID()
	secID := types.NewID()
	date := types.NewDate(2024, time.March, 15)

	txn := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeBuy, types.MustNewMoney("1000.00"), secID, types.MustNewQuantity("5"),
	)

	sidebar := NewSidebar()
	sidebar.SetFocused(false)

	app := &App{
		width:   80,
		height:  24,
		keys:    defaultKeyMap(),
		sidebar: sidebar,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
			transactions:  []*investment.Transaction{txn},
			securityNames: map[types.ID]string{secID: "AAPL"},
		},
	}
	app.buildInvestmentRegisterTable()

	// Press 'd' to delete
	dKey := tea.KeyPressMsg{Code: 'd', Text: "d"}
	app.handleInvestmentRegisterKeys(dKey)

	// Should show confirmation dialog
	if app.confirmDialog == nil {
		t.Fatal("pressing 'd' should show confirmation dialog")
	}
	if !app.confirmDialog.IsVisible() {
		t.Error("confirmation dialog should be visible")
	}
}

func TestInvestmentTypeSelectorOptions(t *testing.T) {
	// Verify all expected transaction types are available in the selector
	options := investmentTransactionTypeOptions()
	expectedTypes := []string{
		"Buy", "Sell", "Dividend", "Reinvest Dividend",
		"Deposit", "Withdrawal", "Interest", "Fee",
	}
	for _, exp := range expectedTypes {
		found := slices.Contains(options, exp)
		if !found {
			t.Errorf("expected option %q not found in type selector", exp)
		}
	}
}

func TestInvestmentRegisterKeyHints(t *testing.T) {
	app := &App{
		currentView: ViewInvestmentRegister,
	}

	hints := app.getKeyHints()
	if !strings.Contains(hints, "navigate") {
		t.Errorf("hints should contain 'navigate', got: %s", hints)
	}
	if !strings.Contains(hints, "c clear") {
		t.Errorf("hints should contain 'c clear', got: %s", hints)
	}
	if !strings.Contains(hints, "n new") {
		t.Errorf("hints should contain 'n new', got: %s", hints)
	}
	if !strings.Contains(hints, "enter edit") {
		t.Errorf("hints should contain 'enter edit', got: %s", hints)
	}
}

func TestInvestmentRegisterHelpOverlay(t *testing.T) {
	sections := viewShortcutSections(ViewInvestmentRegister)

	found := false
	for _, s := range sections {
		if s.Title == "Investment Register" {
			found = true
			hasClear := false
			hasDelete := false
			hasNew := false
			hasEdit := false
			for _, e := range s.Entries {
				switch e.Key {
				case "c":
					hasClear = true
				case "d":
					hasDelete = true
				case "n":
					hasNew = true
				case "Enter":
					hasEdit = true
				}
			}
			if !hasClear {
				t.Error("investment register shortcuts should include 'c' for toggle cleared")
			}
			if !hasDelete {
				t.Error("investment register shortcuts should include 'd' for delete")
			}
			if !hasNew {
				t.Error("investment register shortcuts should include 'n' for new transaction")
			}
			if !hasEdit {
				t.Error("investment register shortcuts should include 'Enter' for edit")
			}
		}
	}
	if !found {
		t.Error("investment register shortcuts section not found in help overlay")
	}
}

func TestInvestmentRegister_UsesInvestmentTransactionsNotRegular(t *testing.T) {
	acctID := types.NewID()
	secID := types.NewID()
	date := types.NewDate(2024, time.March, 15)

	txn := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeBuy, types.MustNewMoney("1850.00"), secID, types.MustNewQuantity("10"),
	)

	data := &investmentRegisterData{
		account: &account.Account{
			BaseModel: types.NewBaseModel(),
			Name:      "Brokerage",
			Type:      account.TypeInvestment,
		},
		transactions:  []*investment.Transaction{txn},
		securityNames: map[types.ID]string{secID: "AAPL"},
	}

	// The data struct uses investment.Transaction, not transaction.Transaction
	if data.transactions[0].Type != investment.TransactionTypeBuy {
		t.Error("should use investment transaction types")
	}
	if data.transactions[0].SecurityID.Valid != true {
		t.Error("should have valid security ID for buy transaction")
	}
}

// Test that all 12 transaction types format correctly
func TestFormatInvestmentRegisterRow_AllTypes(t *testing.T) {
	acctID := types.NewID()
	secID := types.NewID()
	date := types.NewDate(2024, time.March, 15)

	app := &App{
		investmentRegister: &investmentRegisterData{
			securityNames: map[types.ID]string{secID: "AAPL"},
		},
	}

	tests := []struct {
		txnType     investment.TransactionType
		wantDisplay string
	}{
		{investment.TransactionTypeBuy, "Buy"},
		{investment.TransactionTypeSell, "Sell"},
		{investment.TransactionTypeDividend, "Dividend"},
		{investment.TransactionTypeReinvestDividend, "Reinvest Dividend"},
		{investment.TransactionTypeFee, "Fee"},
		{investment.TransactionTypeFeeLiquidation, "Fee via Liquidation"},
		{investment.TransactionTypeDeposit, "Deposit"},
		{investment.TransactionTypeWithdrawal, "Withdrawal"},
		{investment.TransactionTypeInterest, "Interest"},
		{investment.TransactionTypeTransferShares, "Transfer Shares"},
		{investment.TransactionTypeTransferCash, "Transfer Cash"},
		{investment.TransactionTypeExchange, "Exchange"},
	}

	for _, tt := range tests {
		t.Run(string(tt.txnType), func(t *testing.T) {
			var txn *investment.Transaction
			if tt.txnType.RequiresSecurity() {
				txn = investment.NewTransactionWithSecurity(
					acctID, date, tt.txnType, types.MustNewMoney("100.00"), secID, types.MustNewQuantity("1"),
				)
			} else {
				txn = investment.NewTransaction(acctID, date, tt.txnType, types.MustNewMoney("100.00"))
			}

			row := app.formatInvestmentRegisterRow(txn)
			if row[2] != tt.wantDisplay {
				t.Errorf("type display = %q, want %q", row[2], tt.wantDisplay)
			}
		})
	}
}

func TestInvestmentRegisterView_SwitchView(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	app.switchView(ViewInvestmentRegister)

	if app.currentView != ViewInvestmentRegister {
		t.Errorf("currentView = %v, want ViewInvestmentRegister", app.currentView)
	}
	if app.previousView != ViewDashboard {
		t.Errorf("previousView = %v, want ViewDashboard", app.previousView)
	}
}

func TestRenderInvestmentRegister_TotalReturnHeader(t *testing.T) {
	pct := 22.51
	val := &investment.AccountValuation{
		CashBalance:       types.MustNewMoney("1200.00"),
		MarketValue:       types.MustNewMoney("27000.00"),
		TotalValue:        types.MustNewMoney("28200.00"),
		TotalCostBasis:    types.MustNewMoney("22500.00"),
		TotalGainLoss:     types.MustNewMoney("4500.00"),
		RealizedGain:      types.MustNewMoney("200.00"),
		DividendsReceived: types.MustNewMoney("570.00"),
		InterestReceived:  types.MustNewMoney("13.00"),
		FeesPaid:          types.MustNewMoney("15.00"),
		TotalReturn:       types.MustNewMoney("5267.50"),
		TotalReturnPct:    &pct,
	}

	app := &App{
		width:  120,
		height: 30,
		styles: widget.NewStyles(),
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
			transactions:  []*investment.Transaction{},
			securityNames: map[types.ID]string{},
			cashBalance:   types.MustNewMoney("1200.00"),
			valuation:     val,
		},
	}
	app.styles.Resize(120, 30)
	app.buildInvestmentRegisterTable()

	output := widget.StripAnsi(app.renderInvestmentRegister())

	wants := []string{
		"Unrealized", "$4500.00",
		"Realized", "$200.00",
		"Div", "$570.00",
		"Int", "$13.00",
		"Fees", "-$15.00",
		"Total return", "$5267.50",
		"22.51%",
	}
	for _, want := range wants {
		if !strings.Contains(output, want) {
			t.Errorf("output should contain %q\nfull output:\n%s", want, output)
		}
	}
}

func TestRenderInvestmentRegister_TotalReturnPctNilRendersDash(t *testing.T) {
	val := &investment.AccountValuation{
		CashBalance:       types.MustNewMoney("100.00"),
		TotalGainLoss:     types.ZeroMoney,
		RealizedGain:      types.ZeroMoney,
		DividendsReceived: types.ZeroMoney,
		InterestReceived:  types.ZeroMoney,
		FeesPaid:          types.ZeroMoney,
		TotalReturn:       types.ZeroMoney,
		TotalReturnPct:    nil,
	}

	app := &App{
		width:  120,
		height: 30,
		styles: widget.NewStyles(),
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
			transactions:  []*investment.Transaction{},
			securityNames: map[types.ID]string{},
			cashBalance:   types.MustNewMoney("100.00"),
			valuation:     val,
		},
	}
	app.styles.Resize(120, 30)
	app.buildInvestmentRegisterTable()

	output := widget.StripAnsi(app.renderInvestmentRegister())
	if !strings.Contains(output, "Total return") {
		t.Fatalf("output should contain 'Total return' line; got:\n%s", output)
	}
	if !strings.Contains(output, "—") {
		t.Errorf("output should contain '—' placeholder when TotalReturnPct is nil; got:\n%s", output)
	}
}

func TestRenderInvestmentRegister_NilValuationOmitsTotalReturn(t *testing.T) {
	app := &App{
		width:  120,
		height: 30,
		styles: widget.NewStyles(),
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
			transactions:  []*investment.Transaction{},
			securityNames: map[types.ID]string{},
			cashBalance:   types.MustNewMoney("100.00"),
			valuation:     nil,
		},
	}
	app.styles.Resize(120, 30)
	app.buildInvestmentRegisterTable()

	output := widget.StripAnsi(app.renderInvestmentRegister())
	if strings.Contains(output, "Total return") {
		t.Errorf("output should NOT contain 'Total return' when valuation is nil; got:\n%s", output)
	}
}

func TestInvestmentRegister_SecurityNameLookup(t *testing.T) {
	secID1 := types.NewID()
	secID2 := types.NewID()

	_ = security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	data := &investmentRegisterData{
		securityNames: map[types.ID]string{
			secID1: "AAPL",
			secID2: "MSFT",
		},
	}

	if data.securityNames[secID1] != "AAPL" {
		t.Errorf("security name lookup failed for secID1")
	}
	if data.securityNames[secID2] != "MSFT" {
		t.Errorf("security name lookup failed for secID2")
	}
}
