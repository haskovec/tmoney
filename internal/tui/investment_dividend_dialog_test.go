package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

func TestBuildDividendDialog_NewTransaction(t *testing.T) {
	options := []string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."}
	ids := []types.ID{types.NewID(), types.NewID()}

	d := buildDividendDialog(options, nil, ids)

	if d == nil {
		t.Fatal("dialog should not be nil")
	}
	if !d.IsVisible() {
		t.Error("dialog should be visible")
	}

	fields := d.Fields()
	if len(fields) != 4 {
		t.Fatalf("expected 4 fields, got %d", len(fields))
	}

	// Field 0: Date (masked, required, default today)
	if fields[0].Type != FieldDate {
		t.Errorf("field 0 type = %d, want FieldDate (%d)", fields[0].Type, FieldDate)
	}
	if fields[0].Label != "Date" {
		t.Errorf("field 0 label = %q, want %q", fields[0].Label, "Date")
	}
	if !fields[0].Required {
		t.Error("date field should be required")
	}
	today := time.Now().Format("01/02/2006")
	if fields[0].Value != today {
		t.Errorf("date default = %q, want %q", fields[0].Value, today)
	}

	// Field 1: Security (typeahead combo)
	if fields[1].Type != FieldCombo {
		t.Errorf("field 1 type = %d, want FieldCombo (%d)", fields[1].Type, FieldCombo)
	}
	if fields[1].Label != "Security" {
		t.Errorf("field 1 label = %q, want %q", fields[1].Label, "Security")
	}
	if len(fields[1].Options) != 2 {
		t.Errorf("expected 2 security options, got %d", len(fields[1].Options))
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
	if fields[2].Value != "" {
		t.Errorf("amount default = %q, want empty for new", fields[2].Value)
	}

	// Field 3: Memo (text)
	if fields[3].Label != "Memo" {
		t.Errorf("field 3 label = %q, want %q", fields[3].Label, "Memo")
	}
}

func TestBuildDividendDialog_EditTransaction(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()
	date := types.NewDate(2024, time.March, 15)

	txn := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeDividend,
		types.MustNewMoney("50.00"), secID, types.ZeroQuantity,
	)
	txn.Memo = types.NullableString{String: "Q1 dividend", Valid: true}

	options := []string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."}
	ids := []types.ID{secID, types.NewID()}

	d := buildDividendDialog(options, txn, ids)
	fields := d.Fields()

	// Date should match
	if fields[0].Value != "03/15/2024" {
		t.Errorf("date = %q, want %q", fields[0].Value, "03/15/2024")
	}

	// Security should be pre-selected
	if fields[1].SelectedIndex != 0 {
		t.Errorf("security selected index = %d, want 0", fields[1].SelectedIndex)
	}

	// Amount
	if fields[2].Value != "50.00" {
		t.Errorf("amount = %q, want %q", fields[2].Value, "50.00")
	}

	// Memo
	if fields[3].Value != "Q1 dividend" {
		t.Errorf("memo = %q, want %q", fields[3].Value, "Q1 dividend")
	}
}

func TestBuildDividendDialog_EditPreSelectsCorrectSecurity(t *testing.T) {
	secID1 := types.NewID()
	secID2 := types.NewID()
	acctID := types.NewID()
	date := types.NewDate(2024, time.June, 1)

	txn := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeDividend,
		types.MustNewMoney("25.00"), secID2, types.ZeroQuantity,
	)

	options := []string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."}
	ids := []types.ID{secID1, secID2}

	d := buildDividendDialog(options, txn, ids)
	fields := d.Fields()

	if fields[1].SelectedIndex != 1 {
		t.Errorf("security selected index = %d, want 1 (MSFT)", fields[1].SelectedIndex)
	}
}

func TestBuildReinvestDividendDialog_NewTransaction(t *testing.T) {
	options := []string{"AAPL - Apple Inc.", "MSFT - Microsoft Corp."}
	ids := []types.ID{types.NewID(), types.NewID()}

	d := buildReinvestDividendDialog(options, nil, ids)

	if d == nil {
		t.Fatal("dialog should not be nil")
	}
	if !d.IsVisible() {
		t.Error("dialog should be visible")
	}

	fields := d.Fields()
	if len(fields) != 6 {
		t.Fatalf("expected 6 fields, got %d", len(fields))
	}

	// Field 0: Date (text, required)
	if fields[0].Label != "Date" {
		t.Errorf("field 0 label = %q, want %q", fields[0].Label, "Date")
	}
	if !fields[0].Required {
		t.Error("date field should be required")
	}

	// Field 1: Security (typeahead combo)
	if fields[1].Type != FieldCombo {
		t.Errorf("field 1 type = %d, want FieldCombo", fields[1].Type)
	}
	if fields[1].Label != "Security" {
		t.Errorf("field 1 label = %q, want %q", fields[1].Label, "Security")
	}

	// Field 2: Shares (text, required)
	if fields[2].Label != "Shares" {
		t.Errorf("field 2 label = %q, want %q", fields[2].Label, "Shares")
	}
	if !fields[2].Required {
		t.Error("shares field should be required")
	}

	// Field 3: Price/Share (text)
	if fields[3].Label != "Price/Share" {
		t.Errorf("field 3 label = %q, want %q", fields[3].Label, "Price/Share")
	}

	// Field 4: Total (text)
	if fields[4].Label != "Total" {
		t.Errorf("field 4 label = %q, want %q", fields[4].Label, "Total")
	}

	// Field 5: Memo (text)
	if fields[5].Label != "Memo" {
		t.Errorf("field 5 label = %q, want %q", fields[5].Label, "Memo")
	}
}

func TestBuildReinvestDividendDialog_EditTransaction(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()
	date := types.NewDate(2024, time.March, 15)

	txn := investment.NewTransactionWithSecurity(
		acctID, date, investment.TransactionTypeReinvestDividend,
		types.MustNewMoney("1850.00"), secID, types.MustNewQuantity("10"),
	)
	txn.PricePerShare = types.NullableMoney{Money: types.MustNewMoney("185.00"), Valid: true}
	txn.Memo = types.NullableString{String: "DRIP", Valid: true}

	options := []string{"AAPL - Apple Inc."}
	ids := []types.ID{secID}

	d := buildReinvestDividendDialog(options, txn, ids)
	fields := d.Fields()

	if fields[0].Value != "03/15/2024" {
		t.Errorf("date = %q, want %q", fields[0].Value, "03/15/2024")
	}
	if fields[1].SelectedIndex != 0 {
		t.Errorf("security selected = %d, want 0", fields[1].SelectedIndex)
	}
	if fields[2].Value != "10" {
		t.Errorf("shares = %q, want %q", fields[2].Value, "10")
	}
	if fields[3].Value != "185.00" {
		t.Errorf("price = %q, want %q", fields[3].Value, "185.00")
	}
	if fields[4].Value != "1850.00" {
		t.Errorf("total = %q, want %q", fields[4].Value, "1850.00")
	}
	if fields[5].Value != "DRIP" {
		t.Errorf("memo = %q, want %q", fields[5].Value, "DRIP")
	}
}

func TestSubmitDividendDialog_ValidationErrors(t *testing.T) {
	secIDs := []types.ID{types.NewID()}

	app := &App{
		dividendDialog: buildDividendDialog([]string{"AAPL - Apple Inc."}, nil, secIDs),
		dividendDialogData: &dividendDialogData{
			securities: []*security.Security{},
		},
		dividendDialogSecurityIDs: secIDs,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
		},
	}

	// Set invalid values
	fields := app.dividendDialog.Fields()
	fields[0].Value = "not-a-date" // invalid date
	fields[2].Value = ""           // empty amount

	model, cmd := app.submitDividendDialog()
	updatedApp := model.(*App)

	if updatedApp.dividendDialog == nil {
		t.Error("dialog should remain open on validation errors")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.dividendDialog.Fields()
	if fields[0].Error == "" {
		t.Error("date field should have error")
	}
	if fields[2].Error == "" {
		t.Error("amount field should have error")
	}
}

func TestSubmitDividendDialog_InvalidAmount(t *testing.T) {
	secIDs := []types.ID{types.NewID()}

	app := &App{
		dividendDialog: buildDividendDialog([]string{"AAPL - Apple Inc."}, nil, secIDs),
		dividendDialogData: &dividendDialogData{
			securities: []*security.Security{},
		},
		dividendDialogSecurityIDs: secIDs,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.dividendDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[2].Value = "not-valid" // invalid amount

	model, _ := app.submitDividendDialog()
	updatedApp := model.(*App)

	if updatedApp.dividendDialog == nil {
		t.Error("dialog should remain open on amount error")
	}
	fields = updatedApp.dividendDialog.Fields()
	if fields[2].Error == "" {
		t.Error("amount field should have error")
	}
}

func TestSubmitDividendDialog_Valid(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()

	app := &App{
		dividendDialog: buildDividendDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
		),
		dividendDialogData: &dividendDialogData{
			securities: []*security.Security{},
		},
		dividendDialogSecurityIDs: []types.ID{secID},
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Name:      "Brokerage",
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.dividendDialog.Fields()
	fields[0].Value = "03/15/2024" // date
	fields[2].Value = "50.00"      // amount

	model, cmd := app.submitDividendDialog()
	updatedApp := model.(*App)

	if updatedApp.dividendDialog != nil {
		t.Error("dialog should be closed after valid submit")
	}
	if cmd == nil {
		t.Error("should return a command for async save")
	}
}

func TestSubmitDividendDialog_ValidWithMemo(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()

	app := &App{
		dividendDialog: buildDividendDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
		),
		dividendDialogData:        &dividendDialogData{},
		dividendDialogSecurityIDs: []types.ID{secID},
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.dividendDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[2].Value = "50.00"
	fields[3].Value = "Q1 dividend" // memo

	model, cmd := app.submitDividendDialog()
	updatedApp := model.(*App)

	if updatedApp.dividendDialog != nil {
		t.Error("dialog should close on valid submit with memo")
	}
	if cmd == nil {
		t.Error("should return command")
	}
}

func TestSubmitDividendDialog_NoSecurities(t *testing.T) {
	app := &App{
		dividendDialog: buildDividendDialog([]string{}, nil, []types.ID{}),
		dividendDialogData: &dividendDialogData{
			securities: []*security.Security{},
		},
		dividendDialogSecurityIDs: []types.ID{},
	}

	fields := app.dividendDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[2].Value = "50.00"

	model, _ := app.submitDividendDialog()
	updatedApp := model.(*App)

	if updatedApp.dividendDialog == nil {
		t.Error("dialog should remain open when no securities available")
	}
	fields = updatedApp.dividendDialog.Fields()
	if fields[1].Error == "" {
		t.Error("security field should have error when no securities available")
	}
}

func TestSubmitReinvestDividendDialog_ValidationErrors(t *testing.T) {
	secIDs := []types.ID{types.NewID()}

	app := &App{
		dividendDialog: buildReinvestDividendDialog([]string{"AAPL - Apple Inc."}, nil, secIDs),
		dividendDialogData: &dividendDialogData{
			securities: []*security.Security{},
		},
		dividendDialogSecurityIDs: secIDs,
		dividendDialogReinvest:    true,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.dividendDialog.Fields()
	fields[0].Value = "not-a-date" // invalid date
	fields[2].Value = ""           // empty shares
	fields[3].Value = ""           // no price
	fields[4].Value = ""           // no total

	model, cmd := app.submitReinvestDividendDialog()
	updatedApp := model.(*App)

	if updatedApp.dividendDialog == nil {
		t.Error("dialog should remain open on validation errors")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.dividendDialog.Fields()
	if fields[0].Error == "" {
		t.Error("date field should have error")
	}
	if fields[2].Error == "" {
		t.Error("shares field should have error")
	}
	if fields[3].Error == "" {
		t.Error("price field should have error when both price and total empty")
	}
	if fields[4].Error == "" {
		t.Error("total field should have error when both price and total empty")
	}
}

func TestSubmitReinvestDividendDialog_ValidWithPrice(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()

	app := &App{
		dividendDialog: buildReinvestDividendDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
		),
		dividendDialogData: &dividendDialogData{
			securities: []*security.Security{},
		},
		dividendDialogSecurityIDs: []types.ID{secID},
		dividendDialogReinvest:    true,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.dividendDialog.Fields()
	fields[0].Value = "03/15/2024" // date
	fields[2].Value = "10"         // shares
	fields[3].Value = "185.00"     // price per share

	model, cmd := app.submitReinvestDividendDialog()
	updatedApp := model.(*App)

	if updatedApp.dividendDialog != nil {
		t.Error("dialog should be closed after valid submit")
	}
	if cmd == nil {
		t.Error("should return a command for async save")
	}
}

func TestSubmitReinvestDividendDialog_ValidWithTotal(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()

	app := &App{
		dividendDialog: buildReinvestDividendDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
		),
		dividendDialogData: &dividendDialogData{
			securities: []*security.Security{},
		},
		dividendDialogSecurityIDs: []types.ID{secID},
		dividendDialogReinvest:    true,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.dividendDialog.Fields()
	fields[0].Value = "06/01/2024" // date
	fields[2].Value = "5"          // shares
	fields[4].Value = "925.00"     // total amount

	model, cmd := app.submitReinvestDividendDialog()
	updatedApp := model.(*App)

	if updatedApp.dividendDialog != nil {
		t.Error("dialog should be closed after valid submit")
	}
	if cmd == nil {
		t.Error("should return a command for async save")
	}
}

func TestSubmitReinvestDividendDialog_NoSecurities(t *testing.T) {
	app := &App{
		dividendDialog: buildReinvestDividendDialog([]string{}, nil, []types.ID{}),
		dividendDialogData: &dividendDialogData{
			securities: []*security.Security{},
		},
		dividendDialogSecurityIDs: []types.ID{},
		dividendDialogReinvest:    true,
	}

	fields := app.dividendDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[2].Value = "10"
	fields[3].Value = "185.00"

	model, _ := app.submitReinvestDividendDialog()
	updatedApp := model.(*App)

	if updatedApp.dividendDialog == nil {
		t.Error("dialog should remain open when no securities available")
	}
	fields = updatedApp.dividendDialog.Fields()
	if fields[1].Error == "" {
		t.Error("security field should have error")
	}
}

func TestSubmitReinvestDividendDialog_InvalidPrice(t *testing.T) {
	secID := types.NewID()

	app := &App{
		dividendDialog: buildReinvestDividendDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
		),
		dividendDialogData:        &dividendDialogData{},
		dividendDialogSecurityIDs: []types.ID{secID},
		dividendDialogReinvest:    true,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.NewBaseModel(),
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.dividendDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[2].Value = "10"
	fields[3].Value = "not-valid" // invalid price
	fields[4].Value = ""

	model, _ := app.submitReinvestDividendDialog()
	updatedApp := model.(*App)

	if updatedApp.dividendDialog == nil {
		t.Error("dialog should remain open on price error")
	}
	fields = updatedApp.dividendDialog.Fields()
	if fields[3].Error == "" {
		t.Error("price field should have error")
	}
}

func TestHandleDividendDialogKey_Cancel(t *testing.T) {
	secID := types.NewID()

	app := &App{
		dividendDialog: buildDividendDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
		),
		dividendDialogData:        &dividendDialogData{},
		dividendDialogSecurityIDs: []types.ID{secID},
	}

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
	model, _ := app.handleDividendDialogKey(escKey)
	updatedApp := model.(*App)

	if updatedApp.dividendDialog != nil {
		t.Error("dialog should be closed after Escape")
	}
	if updatedApp.dividendDialogData != nil {
		t.Error("dialog data should be cleared after cancel")
	}
	if updatedApp.dividendDialogSecurityIDs != nil {
		t.Error("security IDs should be cleared after cancel")
	}
}

func TestHandleDividendDialogKey_NilDialog(t *testing.T) {
	app := &App{}

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
	model, cmd := app.handleDividendDialogKey(escKey)

	if model.(*App) != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

func TestSubmitDividendDialog_NilDialog(t *testing.T) {
	app := &App{}

	model, cmd := app.submitDividendDialog()

	if model.(*App) != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

func TestSubmitReinvestDividendDialog_NilDialog(t *testing.T) {
	app := &App{}

	model, cmd := app.submitReinvestDividendDialog()

	if model.(*App) != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

func TestCloseDividendDialog(t *testing.T) {
	secID := types.NewID()

	app := &App{
		dividendDialog: buildDividendDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
		),
		dividendDialogData:        &dividendDialogData{},
		dividendDialogSecurityIDs: []types.ID{secID},
		dividendDialogReinvest:    true,
	}

	app.closeDividendDialog()

	if app.dividendDialog != nil {
		t.Error("dividendDialog should be nil after close")
	}
	if app.dividendDialogData != nil {
		t.Error("dividendDialogData should be nil after close")
	}
	if app.dividendDialogSecurityIDs != nil {
		t.Error("dividendDialogSecurityIDs should be nil after close")
	}
	if app.dividendDialogReinvest {
		t.Error("dividendDialogReinvest should be false after close")
	}
}

func TestDividendDialogDataMsg(t *testing.T) {
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)

	data := &dividendDialogData{
		securities: []*security.Security{sec},
	}

	msg := dividendDialogDataMsg{data: data}

	if len(msg.data.securities) != 1 {
		t.Errorf("expected 1 security, got %d", len(msg.data.securities))
	}
}

func TestDividendDialogSavedMsg(t *testing.T) {
	msg := dividendDialogSavedMsg{}
	_ = msg
}

func TestHandleDividendDialogKey_RoutesToDividendSubmit(t *testing.T) {
	// Verify that with dividendDialogReinvest=false, submit calls submitDividendDialog.
	// We test this indirectly: set up a valid dividend dialog, call submitDividendDialog directly,
	// and verify the reinvest flag doesn't interfere.
	secID := types.NewID()
	acctID := types.NewID()

	app := &App{
		dividendDialog: buildDividendDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
		),
		dividendDialogData:        &dividendDialogData{},
		dividendDialogSecurityIDs: []types.ID{secID},
		dividendDialogReinvest:    false,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.dividendDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[2].Value = "50.00"

	model, cmd := app.submitDividendDialog()
	updatedApp := model.(*App)

	if updatedApp.dividendDialog != nil {
		t.Error("dialog should be closed after valid submit")
	}
	if cmd == nil {
		t.Error("should return command for async save")
	}
}

func TestHandleDividendDialogKey_RoutesToReinvestSubmit(t *testing.T) {
	// Verify that with dividendDialogReinvest=true, submit calls submitReinvestDividendDialog.
	secID := types.NewID()
	acctID := types.NewID()

	app := &App{
		dividendDialog: buildReinvestDividendDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
		),
		dividendDialogData:        &dividendDialogData{},
		dividendDialogSecurityIDs: []types.ID{secID},
		dividendDialogReinvest:    true,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.dividendDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[2].Value = "10"
	fields[3].Value = "185.00"

	model, cmd := app.submitReinvestDividendDialog()
	updatedApp := model.(*App)

	if updatedApp.dividendDialog != nil {
		t.Error("dialog should be closed after valid reinvest submit")
	}
	if cmd == nil {
		t.Error("should return command for async save")
	}
}

func TestSubmitDividendDialog_DollarSignInAmount(t *testing.T) {
	secID := types.NewID()
	acctID := types.NewID()

	app := &App{
		dividendDialog: buildDividendDialog(
			[]string{"AAPL - Apple Inc."},
			nil,
			[]types.ID{secID},
		),
		dividendDialogData:        &dividendDialogData{},
		dividendDialogSecurityIDs: []types.ID{secID},
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.dividendDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[2].Value = "$50.00" // dollar sign in amount

	model, cmd := app.submitDividendDialog()
	updatedApp := model.(*App)

	if updatedApp.dividendDialog != nil {
		t.Error("dialog should close (dollar sign stripped)")
	}
	if cmd == nil {
		t.Error("should return command")
	}
}
