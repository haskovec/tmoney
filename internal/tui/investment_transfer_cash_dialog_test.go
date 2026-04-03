package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
)

// --- buildNonInvestmentAccountOptions tests ---

func TestBuildNonInvestmentAccountOptions(t *testing.T) {
	accounts := []*account.Account{
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Checking", Type: account.TypeChecking},
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Brokerage", Type: account.TypeInvestment},
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Savings", Type: account.TypeSavings},
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "IRA", Type: account.TypeInvestment},
	}

	options, ids := buildNonInvestmentAccountOptions(accounts)

	if len(options) != 2 {
		t.Fatalf("expected 2 options, got %d", len(options))
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}
	if options[0] != "Checking" {
		t.Errorf("option[0] = %q, want %q", options[0], "Checking")
	}
	if options[1] != "Savings" {
		t.Errorf("option[1] = %q, want %q", options[1], "Savings")
	}
	if ids[0] != accounts[0].ID {
		t.Errorf("ids[0] should match Checking account ID")
	}
	if ids[1] != accounts[2].ID {
		t.Errorf("ids[1] should match Savings account ID")
	}
}

func TestBuildNonInvestmentAccountOptions_Empty(t *testing.T) {
	options, ids := buildNonInvestmentAccountOptions(nil)
	if len(options) != 0 {
		t.Errorf("expected 0 options, got %d", len(options))
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 ids, got %d", len(ids))
	}
}

func TestBuildNonInvestmentAccountOptions_AllInvestment(t *testing.T) {
	accounts := []*account.Account{
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Brokerage", Type: account.TypeInvestment},
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "IRA", Type: account.TypeInvestment},
	}

	options, ids := buildNonInvestmentAccountOptions(accounts)
	if len(options) != 0 {
		t.Errorf("expected 0 options when all investment, got %d", len(options))
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 ids when all investment, got %d", len(ids))
	}
}

// --- buildTransferCashDialog tests ---

func TestBuildTransferCashDialog_Deposit(t *testing.T) {
	options := []string{"Checking", "Savings"}

	d := buildTransferCashDialog("deposit", options, nil, nil)

	if d == nil {
		t.Fatal("dialog should not be nil")
	}
	if !d.IsVisible() {
		t.Error("dialog should be visible")
	}
	if d.Title() != "Transfer Cash In" {
		t.Errorf("title = %q, want %q", d.Title(), "Transfer Cash In")
	}

	fields := d.Fields()
	if len(fields) != 4 {
		t.Fatalf("expected 4 fields, got %d", len(fields))
	}

	// Field 0: Account (select)
	if fields[0].Type != FieldSelect {
		t.Errorf("field 0 type = %d, want FieldSelect (%d)", fields[0].Type, FieldSelect)
	}
	if fields[0].Label != "Account" {
		t.Errorf("field 0 label = %q, want %q", fields[0].Label, "Account")
	}

	// Field 1: Amount (text, required)
	if fields[1].Type != FieldText {
		t.Errorf("field 1 type = %d, want FieldText", fields[1].Type)
	}
	if fields[1].Label != "Amount" {
		t.Errorf("field 1 label = %q, want %q", fields[1].Label, "Amount")
	}
	if !fields[1].Required {
		t.Error("amount field should be required")
	}

	// Field 2: Date (text, required)
	if fields[2].Type != FieldText {
		t.Errorf("field 2 type = %d, want FieldText", fields[2].Type)
	}
	if fields[2].Label != "Date" {
		t.Errorf("field 2 label = %q, want %q", fields[2].Label, "Date")
	}
	if !fields[2].Required {
		t.Error("date field should be required")
	}
	today := time.Now().Format("01/02/2006")
	if fields[2].Value != today {
		t.Errorf("date default = %q, want %q", fields[2].Value, today)
	}

	// Field 3: Memo (text)
	if fields[3].Label != "Memo" {
		t.Errorf("field 3 label = %q, want %q", fields[3].Label, "Memo")
	}
}

func TestBuildTransferCashDialog_Withdraw(t *testing.T) {
	d := buildTransferCashDialog("withdraw", []string{"Checking"}, nil, nil)

	if d.Title() != "Transfer Cash Out" {
		t.Errorf("title = %q, want %q", d.Title(), "Transfer Cash Out")
	}
}

func TestBuildTransferCashDialog_EditTransaction(t *testing.T) {
	acctID := types.NewID()
	linkedAcctID := types.NewID()
	date := types.NewDate(2024, time.March, 15)

	txn := investment.NewTransaction(
		acctID, date, investment.TransactionTypeTransferCash,
		types.MustNewMoney("500.00"),
	)
	txn.TransferAccountID = types.NullableID{ID: linkedAcctID, Valid: true}
	txn.Memo = types.NullableString{String: "Fund transfer", Valid: true}

	accountIDs := []types.ID{types.NewID(), linkedAcctID}
	options := []string{"Checking", "Savings"}

	d := buildTransferCashDialog("deposit", options, txn, accountIDs)
	fields := d.Fields()

	// Account selector should pre-select the matching account
	if fields[0].SelectedIndex != 1 {
		t.Errorf("account selected = %d, want 1", fields[0].SelectedIndex)
	}

	// Amount should be pre-filled
	if fields[1].Value != "500.00" {
		t.Errorf("amount = %q, want %q", fields[1].Value, "500.00")
	}

	// Date should match
	if fields[2].Value != "03/15/2024" {
		t.Errorf("date = %q, want %q", fields[2].Value, "03/15/2024")
	}

	// Memo should be pre-filled
	if fields[3].Value != "Fund transfer" {
		t.Errorf("memo = %q, want %q", fields[3].Value, "Fund transfer")
	}
}

func TestBuildTransferCashDialog_EditNegativeAmount(t *testing.T) {
	acctID := types.NewID()
	date := types.NewDate(2024, time.June, 1)

	txn := investment.NewTransaction(
		acctID, date, investment.TransactionTypeTransferCash,
		types.MustNewMoney("-250.00"),
	)

	d := buildTransferCashDialog("withdraw", []string{"Checking"}, txn, nil)
	fields := d.Fields()

	// Negative amount should be displayed as positive
	if fields[1].Value != "250.00" {
		t.Errorf("amount = %q, want %q (negated)", fields[1].Value, "250.00")
	}
}

// --- Submit validation tests ---

func TestSubmitTransferCashDialog_ValidationErrors(t *testing.T) {
	acctID := types.NewID()
	linkedAcctIDs := []types.ID{types.NewID()}

	app := &App{
		transferCashDialog: buildTransferCashDialog("deposit", []string{"Checking"}, nil, linkedAcctIDs),
		transferCashDialogData: &transferCashDialogData{
			accountIDs: linkedAcctIDs,
		},
		transferCashDialogAccountIDs: linkedAcctIDs,
		transferCashDirection:        "deposit",
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	// Set invalid values
	fields := app.transferCashDialog.Fields()
	fields[1].Value = ""           // empty amount
	fields[2].Value = "not-a-date" // invalid date

	model, cmd := app.submitTransferCashDialog()
	updatedApp := model.(*App)

	if updatedApp.transferCashDialog == nil {
		t.Error("dialog should remain open on validation errors")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.transferCashDialog.Fields()
	if fields[1].Error == "" {
		t.Error("amount field should have error")
	}
	if fields[2].Error == "" {
		t.Error("date field should have error")
	}
}

func TestSubmitTransferCashDialog_InvalidAmount(t *testing.T) {
	acctID := types.NewID()
	linkedAcctIDs := []types.ID{types.NewID()}

	app := &App{
		transferCashDialog: buildTransferCashDialog("deposit", []string{"Checking"}, nil, linkedAcctIDs),
		transferCashDialogData: &transferCashDialogData{
			accountIDs: linkedAcctIDs,
		},
		transferCashDialogAccountIDs: linkedAcctIDs,
		transferCashDirection:        "deposit",
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.transferCashDialog.Fields()
	fields[1].Value = "not-valid"  // invalid amount
	fields[2].Value = "03/15/2024" // valid date

	model, _ := app.submitTransferCashDialog()
	updatedApp := model.(*App)

	if updatedApp.transferCashDialog == nil {
		t.Error("dialog should remain open on amount error")
	}
	fields = updatedApp.transferCashDialog.Fields()
	if fields[1].Error == "" {
		t.Error("amount field should have error")
	}
}

func TestSubmitTransferCashDialog_NoAccounts(t *testing.T) {
	acctID := types.NewID()

	app := &App{
		transferCashDialog: buildTransferCashDialog("deposit", []string{}, nil, nil),
		transferCashDialogData: &transferCashDialogData{
			accountIDs: nil,
		},
		transferCashDialogAccountIDs: nil,
		transferCashDirection:        "deposit",
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.transferCashDialog.Fields()
	fields[1].Value = "500.00"
	fields[2].Value = "03/15/2024"

	model, cmd := app.submitTransferCashDialog()
	updatedApp := model.(*App)

	if updatedApp.transferCashDialog == nil {
		t.Error("dialog should remain open when no accounts")
	}
	if cmd != nil {
		t.Error("should not return command when no accounts")
	}
}

func TestSubmitTransferCashDialog_ValidDeposit(t *testing.T) {
	acctID := types.NewID()
	linkedAcctIDs := []types.ID{types.NewID()}

	app := &App{
		transferCashDialog: buildTransferCashDialog("deposit", []string{"Checking"}, nil, linkedAcctIDs),
		transferCashDialogData: &transferCashDialogData{
			accountIDs: linkedAcctIDs,
		},
		transferCashDialogAccountIDs: linkedAcctIDs,
		transferCashDirection:        "deposit",
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.transferCashDialog.Fields()
	fields[1].Value = "500.00"
	fields[2].Value = "03/15/2024"

	model, cmd := app.submitTransferCashDialog()
	updatedApp := model.(*App)

	if updatedApp.transferCashDialog != nil {
		t.Error("dialog should be closed after valid submit")
	}
	if cmd == nil {
		t.Error("should return a command for async save")
	}
}

func TestSubmitTransferCashDialog_ValidWithdraw(t *testing.T) {
	acctID := types.NewID()
	linkedAcctIDs := []types.ID{types.NewID()}

	app := &App{
		transferCashDialog: buildTransferCashDialog("withdraw", []string{"Checking"}, nil, linkedAcctIDs),
		transferCashDialogData: &transferCashDialogData{
			accountIDs: linkedAcctIDs,
		},
		transferCashDialogAccountIDs: linkedAcctIDs,
		transferCashDirection:        "withdraw",
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.transferCashDialog.Fields()
	fields[1].Value = "250.00"
	fields[2].Value = "06/01/2024"
	fields[3].Value = "Monthly withdrawal"

	model, cmd := app.submitTransferCashDialog()
	updatedApp := model.(*App)

	if updatedApp.transferCashDialog != nil {
		t.Error("dialog should close on valid submit")
	}
	if cmd == nil {
		t.Error("should return command")
	}
}

func TestSubmitTransferCashDialog_DollarSignInAmount(t *testing.T) {
	acctID := types.NewID()
	linkedAcctIDs := []types.ID{types.NewID()}

	app := &App{
		transferCashDialog: buildTransferCashDialog("deposit", []string{"Checking"}, nil, linkedAcctIDs),
		transferCashDialogData: &transferCashDialogData{
			accountIDs: linkedAcctIDs,
		},
		transferCashDialogAccountIDs: linkedAcctIDs,
		transferCashDirection:        "deposit",
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.transferCashDialog.Fields()
	fields[1].Value = "$500.00" // dollar sign in amount
	fields[2].Value = "03/15/2024"

	model, cmd := app.submitTransferCashDialog()
	updatedApp := model.(*App)

	if updatedApp.transferCashDialog != nil {
		t.Error("dialog should close (dollar sign stripped)")
	}
	if cmd == nil {
		t.Error("should return command")
	}
}

// --- Handle key tests ---

func TestHandleTransferCashDialogKey_Cancel(t *testing.T) {
	app := &App{
		transferCashDialog:    buildTransferCashDialog("deposit", []string{"Checking"}, nil, nil),
		transferCashDirection: "deposit",
	}

	escKey := tea.KeyMsg{Type: tea.KeyEscape}
	model, _ := app.handleTransferCashDialogKey(escKey)
	updatedApp := model.(*App)

	if updatedApp.transferCashDialog != nil {
		t.Error("dialog should be closed after Escape")
	}
	if updatedApp.transferCashDirection != "" {
		t.Error("direction should be cleared after cancel")
	}
}

func TestHandleTransferCashDialogKey_NilDialog(t *testing.T) {
	app := &App{}

	escKey := tea.KeyMsg{Type: tea.KeyEscape}
	model, cmd := app.handleTransferCashDialogKey(escKey)

	if model.(*App) != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

// --- Submit nil dialog tests ---

func TestSubmitTransferCashDialog_NilDialog(t *testing.T) {
	app := &App{}

	model, cmd := app.submitTransferCashDialog()

	if model.(*App) != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

// --- Close dialog tests ---

func TestCloseTransferCashDialog(t *testing.T) {
	app := &App{
		transferCashDialog:           buildTransferCashDialog("deposit", []string{"Checking"}, nil, nil),
		transferCashDialogData:       &transferCashDialogData{},
		transferCashDialogAccountIDs: []types.ID{types.NewID()},
		transferCashDirection:        "deposit",
	}

	app.closeTransferCashDialog()

	if app.transferCashDialog != nil {
		t.Error("transferCashDialog should be nil after close")
	}
	if app.transferCashDialogData != nil {
		t.Error("transferCashDialogData should be nil after close")
	}
	if app.transferCashDialogAccountIDs != nil {
		t.Error("transferCashDialogAccountIDs should be nil after close")
	}
	if app.transferCashDirection != "" {
		t.Error("transferCashDirection should be empty after close")
	}
}

// --- Message type tests ---

func TestTransferCashDialogSavedMsg(t *testing.T) {
	msg := transferCashDialogSavedMsg{}
	_ = msg
}

func TestTransferCashDialogDataMsg(t *testing.T) {
	msg := transferCashDialogDataMsg{data: &transferCashDialogData{}}
	if msg.data == nil {
		t.Error("data should not be nil")
	}
}
