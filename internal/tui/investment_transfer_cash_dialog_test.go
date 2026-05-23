package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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

func TestBuildTransferCashDialog_NewDefaultsToDepositInto(t *testing.T) {
	options := []string{"Checking", "Savings"}

	d := buildTransferCashDialog("Brokerage", options, nil, nil)

	if d == nil {
		t.Fatal("dialog should not be nil")
	}
	if !d.IsVisible() {
		t.Error("dialog should be visible")
	}
	if d.Title() != "Transfer Cash" {
		t.Errorf("title = %q, want %q", d.Title(), "Transfer Cash")
	}

	fields := d.Fields()
	if len(fields) != 5 {
		t.Fatalf("expected 5 fields, got %d", len(fields))
	}

	// Field 0: Direction (select)
	if fields[0].Type != FieldSelect {
		t.Errorf("field 0 type = %d, want FieldSelect", fields[0].Type)
	}
	if fields[0].Label != "Direction" {
		t.Errorf("field 0 label = %q, want %q", fields[0].Label, "Direction")
	}
	if fields[0].SelectedIndex != 0 {
		t.Errorf("direction default = %d, want 0 (deposit)", fields[0].SelectedIndex)
	}
	if len(fields[0].Options) != 2 {
		t.Fatalf("expected 2 direction options, got %d", len(fields[0].Options))
	}
	if !strings.Contains(fields[0].Options[0], "Brokerage") {
		t.Errorf("deposit option = %q, expected to mention investment account name", fields[0].Options[0])
	}
	if !strings.Contains(fields[0].Options[1], "Brokerage") {
		t.Errorf("withdraw option = %q, expected to mention investment account name", fields[0].Options[1])
	}

	// Field 1: Other account (select)
	if fields[1].Type != FieldSelect {
		t.Errorf("field 1 type = %d, want FieldSelect", fields[1].Type)
	}

	// Field 2: Amount (text, required)
	if fields[2].Type != FieldText {
		t.Errorf("field 2 type = %d, want FieldText", fields[2].Type)
	}
	if fields[2].Label != "Amount" {
		t.Errorf("field 2 label = %q, want %q", fields[2].Label, "Amount")
	}
	if !fields[2].Required {
		t.Error("amount field should be required")
	}

	// Field 3: Date (masked, required)
	if fields[3].Type != FieldDate {
		t.Errorf("field 3 type = %d, want FieldDate", fields[3].Type)
	}
	if !fields[3].Required {
		t.Error("date field should be required")
	}
	today := time.Now().Format("01/02/2006")
	if fields[3].Value != today {
		t.Errorf("date default = %q, want %q", fields[3].Value, today)
	}

	// Field 4: Memo (text)
	if fields[4].Label != "Memo" {
		t.Errorf("field 4 label = %q, want %q", fields[4].Label, "Memo")
	}
}

func TestBuildTransferCashDialog_EditPositiveAmount_DefaultsToDeposit(t *testing.T) {
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

	d := buildTransferCashDialog("Brokerage", options, txn, accountIDs)
	fields := d.Fields()

	if fields[0].SelectedIndex != 0 {
		t.Errorf("direction = %d, want 0 (deposit) for positive amount", fields[0].SelectedIndex)
	}

	if fields[1].SelectedIndex != 1 {
		t.Errorf("account selected = %d, want 1", fields[1].SelectedIndex)
	}
	if fields[2].Value != "500.00" {
		t.Errorf("amount = %q, want %q", fields[2].Value, "500.00")
	}
	if fields[3].Value != "03/15/2024" {
		t.Errorf("date = %q, want %q", fields[3].Value, "03/15/2024")
	}
	if fields[4].Value != "Fund transfer" {
		t.Errorf("memo = %q, want %q", fields[4].Value, "Fund transfer")
	}
}

func TestBuildTransferCashDialog_EditNegativeAmount_DefaultsToWithdraw(t *testing.T) {
	acctID := types.NewID()
	date := types.NewDate(2024, time.June, 1)

	txn := investment.NewTransaction(
		acctID, date, investment.TransactionTypeTransferCash,
		types.MustNewMoney("-250.00"),
	)

	d := buildTransferCashDialog("Brokerage", []string{"Checking"}, txn, nil)
	fields := d.Fields()

	if fields[0].SelectedIndex != 1 {
		t.Errorf("direction = %d, want 1 (withdraw) for negative amount", fields[0].SelectedIndex)
	}

	// Amount field displays the absolute value (sign is conveyed by Direction).
	if fields[2].Value != "250.00" {
		t.Errorf("amount = %q, want %q (absolute value)", fields[2].Value, "250.00")
	}
}

// --- Submit validation tests ---

func TestSubmitTransferCashDialog_ValidationErrors(t *testing.T) {
	acctID := types.NewID()
	linkedAcctIDs := []types.ID{types.NewID()}

	app := &App{
		transferCashDialog: buildTransferCashDialog("Brokerage", []string{"Checking"}, nil, linkedAcctIDs),
		transferCashDialogData: &transferCashDialogData{
			accountIDs: linkedAcctIDs,
		},
		transferCashDialogAccountIDs: linkedAcctIDs,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	// Set invalid values
	fields := app.transferCashDialog.Fields()
	fields[2].Value = ""           // empty amount
	fields[3].Value = "not-a-date" // invalid date

	model, cmd := app.submitTransferCashDialog()
	updatedApp := model.(*App)

	if updatedApp.transferCashDialog == nil {
		t.Error("dialog should remain open on validation errors")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.transferCashDialog.Fields()
	if fields[2].Error == "" {
		t.Error("amount field should have error")
	}
	if fields[3].Error == "" {
		t.Error("date field should have error")
	}
}

func TestSubmitTransferCashDialog_InvalidAmount(t *testing.T) {
	acctID := types.NewID()
	linkedAcctIDs := []types.ID{types.NewID()}

	app := &App{
		transferCashDialog: buildTransferCashDialog("Brokerage", []string{"Checking"}, nil, linkedAcctIDs),
		transferCashDialogData: &transferCashDialogData{
			accountIDs: linkedAcctIDs,
		},
		transferCashDialogAccountIDs: linkedAcctIDs,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.transferCashDialog.Fields()
	fields[2].Value = "not-valid"  // invalid amount
	fields[3].Value = "03/15/2024" // valid date

	model, _ := app.submitTransferCashDialog()
	updatedApp := model.(*App)

	if updatedApp.transferCashDialog == nil {
		t.Error("dialog should remain open on amount error")
	}
	fields = updatedApp.transferCashDialog.Fields()
	if fields[2].Error == "" {
		t.Error("amount field should have error")
	}
}

func TestSubmitTransferCashDialog_NoAccounts(t *testing.T) {
	acctID := types.NewID()

	app := &App{
		transferCashDialog: buildTransferCashDialog("Brokerage", []string{}, nil, nil),
		transferCashDialogData: &transferCashDialogData{
			accountIDs: nil,
		},
		transferCashDialogAccountIDs: nil,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.transferCashDialog.Fields()
	fields[2].Value = "500.00"
	fields[3].Value = "03/15/2024"

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
		transferCashDialog: buildTransferCashDialog("Brokerage", []string{"Checking"}, nil, linkedAcctIDs),
		transferCashDialogData: &transferCashDialogData{
			accountIDs: linkedAcctIDs,
		},
		transferCashDialogAccountIDs: linkedAcctIDs,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.transferCashDialog.Fields()
	// Direction defaults to deposit (index 0)
	fields[2].Value = "500.00"
	fields[3].Value = "03/15/2024"

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
		transferCashDialog: buildTransferCashDialog("Brokerage", []string{"Checking"}, nil, linkedAcctIDs),
		transferCashDialogData: &transferCashDialogData{
			accountIDs: linkedAcctIDs,
		},
		transferCashDialogAccountIDs: linkedAcctIDs,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.transferCashDialog.Fields()
	fields[0].SelectedIndex = 1 // flip to withdraw
	fields[2].Value = "250.00"
	fields[3].Value = "06/01/2024"
	fields[4].Value = "Monthly withdrawal"

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
		transferCashDialog: buildTransferCashDialog("Brokerage", []string{"Checking"}, nil, linkedAcctIDs),
		transferCashDialogData: &transferCashDialogData{
			accountIDs: linkedAcctIDs,
		},
		transferCashDialogAccountIDs: linkedAcctIDs,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.transferCashDialog.Fields()
	fields[2].Value = "$500.00" // dollar sign in amount
	fields[3].Value = "03/15/2024"

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
		transferCashDialog: buildTransferCashDialog("Brokerage", []string{"Checking"}, nil, nil),
	}

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
	model, _ := app.handleTransferCashDialogKey(escKey)
	updatedApp := model.(*App)

	if updatedApp.transferCashDialog != nil {
		t.Error("dialog should be closed after Escape")
	}
}

func TestHandleTransferCashDialogKey_NilDialog(t *testing.T) {
	app := &App{}

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
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
		transferCashDialog:           buildTransferCashDialog("Brokerage", []string{"Checking"}, nil, nil),
		transferCashDialogData:       &transferCashDialogData{},
		transferCashDialogAccountIDs: []types.ID{types.NewID()},
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
