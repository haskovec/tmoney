package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
)

// --- buildInvestmentAccountOptions tests ---

func TestBuildInvestmentAccountOptions(t *testing.T) {
	excludeID := types.NewID()
	otherInvID := types.NewID()
	accounts := []*account.Account{
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Checking", Type: account.TypeChecking},
		{BaseModel: types.BaseModel{ID: excludeID}, Name: "Brokerage", Type: account.TypeInvestment},
		{BaseModel: types.BaseModel{ID: otherInvID}, Name: "IRA", Type: account.TypeInvestment},
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Savings", Type: account.TypeSavings},
	}

	options, ids := buildInvestmentAccountOptions(accounts, excludeID)

	if len(options) != 1 {
		t.Fatalf("expected 1 option (excluding current), got %d", len(options))
	}
	if options[0] != "IRA" {
		t.Errorf("option[0] = %q, want %q", options[0], "IRA")
	}
	if ids[0] != otherInvID {
		t.Error("ids[0] should match IRA account ID")
	}
}

func TestBuildInvestmentAccountOptions_Empty(t *testing.T) {
	options, ids := buildInvestmentAccountOptions(nil, types.NilID)
	if len(options) != 0 {
		t.Errorf("expected 0 options, got %d", len(options))
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 ids, got %d", len(ids))
	}
}

func TestBuildInvestmentAccountOptions_NoOtherInvestment(t *testing.T) {
	selfID := types.NewID()
	accounts := []*account.Account{
		{BaseModel: types.BaseModel{ID: selfID}, Name: "Brokerage", Type: account.TypeInvestment},
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Checking", Type: account.TypeChecking},
	}

	options, ids := buildInvestmentAccountOptions(accounts, selfID)
	if len(options) != 0 {
		t.Errorf("expected 0 options when no other investment accounts, got %d", len(options))
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 ids when no other investment accounts, got %d", len(ids))
	}
}

// --- buildTransferSharesDialog tests ---

func TestBuildTransferSharesDialog_Basic(t *testing.T) {
	acctOptions := []string{"IRA"}
	secOptions := []string{"AAPL - Apple Inc", "MSFT - Microsoft Corp"}
	acctIDs := []types.ID{types.NewID()}
	secIDs := []types.ID{types.NewID(), types.NewID()}

	d := buildTransferSharesDialog(acctOptions, secOptions, nil, acctIDs, secIDs, nil)

	if d == nil {
		t.Fatal("dialog should not be nil")
	}
	if !d.IsVisible() {
		t.Error("dialog should be visible")
	}
	if d.Title() != "Transfer Shares" {
		t.Errorf("title = %q, want %q", d.Title(), "Transfer Shares")
	}

	fields := d.Fields()
	// Without lots: Date(0), Security(1), To Account(2), Shares(3), Memo(4) = 5 fields
	if len(fields) != 5 {
		t.Fatalf("expected 5 fields (no lots), got %d", len(fields))
	}

	// Field 0: Date (masked, required)
	if fields[0].Type != FieldDate {
		t.Errorf("field 0 type = %d, want FieldDate", fields[0].Type)
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

	// Field 2: To Account (select)
	if fields[2].Type != FieldSelect {
		t.Errorf("field 2 type = %d, want FieldSelect (%d)", fields[2].Type, FieldSelect)
	}
	if fields[2].Label != "To Account" {
		t.Errorf("field 2 label = %q, want %q", fields[2].Label, "To Account")
	}

	// Field 3: Shares (text, required)
	if fields[3].Type != FieldText {
		t.Errorf("field 3 type = %d, want FieldText", fields[3].Type)
	}
	if fields[3].Label != "Shares" {
		t.Errorf("field 3 label = %q, want %q", fields[3].Label, "Shares")
	}
	if !fields[3].Required {
		t.Error("shares field should be required")
	}

	// Field 4: Memo (text)
	if fields[4].Label != "Memo" {
		t.Errorf("field 4 label = %q, want %q", fields[4].Label, "Memo")
	}
}

func TestBuildTransferSharesDialog_WithLots(t *testing.T) {
	acctOptions := []string{"IRA"}
	secOptions := []string{"AAPL - Apple Inc"}
	acctIDs := []types.ID{types.NewID()}
	secIDs := []types.ID{types.NewID()}

	lot1 := &investment.Lot{
		BaseModel:    types.BaseModel{ID: types.NewID()},
		PurchaseDate: types.NewDate(2024, time.January, 15),
		Shares:       types.MustNewQuantity("100"),
		CostPerShare: types.MustNewMoney("150.00"),
	}
	lot2 := &investment.Lot{
		BaseModel:    types.BaseModel{ID: types.NewID()},
		PurchaseDate: types.NewDate(2024, time.June, 1),
		Shares:       types.MustNewQuantity("50"),
		CostPerShare: types.MustNewMoney("175.00"),
	}
	lots := []*investment.Lot{lot1, lot2}

	d := buildTransferSharesDialog(acctOptions, secOptions, nil, acctIDs, secIDs, lots)

	fields := d.Fields()
	// With 2 lots: Date(0), Security(1), To Account(2), Shares(3), Lot1(4), Lot2(5), Memo(6) = 7 fields
	if len(fields) != 7 {
		t.Fatalf("expected 7 fields (2 lots), got %d", len(fields))
	}

	// Lot fields at indices 4 and 5
	if fields[4].Type != FieldText {
		t.Errorf("lot field 0 type = %d, want FieldText", fields[4].Type)
	}
	if fields[5].Type != FieldText {
		t.Errorf("lot field 1 type = %d, want FieldText", fields[5].Type)
	}

	// Memo at index 6
	if fields[6].Label != "Memo" {
		t.Errorf("field 6 label = %q, want %q", fields[6].Label, "Memo")
	}
}

func TestBuildTransferSharesDialog_EditTransaction(t *testing.T) {
	acctID := types.NewID()
	destAcctID := types.NewID()
	secID := types.NewID()
	date := types.NewDate(2024, time.March, 15)

	txn := investment.NewTransaction(
		acctID, date, investment.TransactionTypeTransferShares,
		types.MustNewMoney("1500.00"),
	)
	txn.TransferAccountID = types.NullableID{ID: destAcctID, Valid: true}
	txn.SecurityID = types.NullableID{ID: secID, Valid: true}
	txn.Shares = types.NullableQuantity{Quantity: types.MustNewQuantity("10"), Valid: true}
	txn.Memo = types.NullableString{String: "Transfer to IRA", Valid: true}

	acctIDs := []types.ID{types.NewID(), destAcctID}
	secIDs := []types.ID{types.NewID(), secID}
	acctOptions := []string{"401k", "IRA"}
	secOptions := []string{"AAPL - Apple", "MSFT - Microsoft"}

	d := buildTransferSharesDialog(acctOptions, secOptions, txn, acctIDs, secIDs, nil)
	fields := d.Fields()

	// Date (index 0)
	if fields[0].Value != "03/15/2024" {
		t.Errorf("date = %q, want %q", fields[0].Value, "03/15/2024")
	}

	// Security should match secID (index 1)
	if fields[1].SelectedIndex != 1 {
		t.Errorf("security selected = %d, want 1", fields[1].SelectedIndex)
	}

	// Account should match destAcctID (index 1)
	if fields[2].SelectedIndex != 1 {
		t.Errorf("account selected = %d, want 1", fields[2].SelectedIndex)
	}

	// Shares
	if fields[3].Value != "10" {
		t.Errorf("shares = %q, want %q", fields[3].Value, "10")
	}

	// Memo
	if fields[4].Value != "Transfer to IRA" {
		t.Errorf("memo = %q, want %q", fields[4].Value, "Transfer to IRA")
	}
}

// --- Submit validation tests ---

func TestSubmitTransferSharesDialog_ValidationErrors(t *testing.T) {
	acctID := types.NewID()
	destAcctIDs := []types.ID{types.NewID()}
	secIDs := []types.ID{types.NewID()}

	app := &App{
		transferSharesDialog: buildTransferSharesDialog(
			[]string{"IRA"}, []string{"AAPL - Apple"}, nil, destAcctIDs, secIDs, nil,
		),
		transferSharesDialogData: &transferSharesDialogData{
			investmentIDs: destAcctIDs,
		},
		transferSharesDialogAccountIDs:  destAcctIDs,
		transferSharesDialogSecurityIDs: secIDs,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	// Set invalid values
	fields := app.transferSharesDialog.Fields()
	fields[0].Value = "not-a-date" // invalid date
	fields[3].Value = ""           // empty shares

	model, cmd := app.submitTransferSharesDialog()
	updatedApp := model.(*App)

	if updatedApp.transferSharesDialog == nil {
		t.Error("dialog should remain open on validation errors")
	}
	if cmd != nil {
		t.Error("should not return command on validation errors")
	}

	fields = updatedApp.transferSharesDialog.Fields()
	if fields[3].Error == "" {
		t.Error("shares field should have error")
	}
	if fields[0].Error == "" {
		t.Error("date field should have error")
	}
}

func TestSubmitTransferSharesDialog_NoAccounts(t *testing.T) {
	acctID := types.NewID()
	secIDs := []types.ID{types.NewID()}

	app := &App{
		transferSharesDialog: buildTransferSharesDialog(
			[]string{}, []string{"AAPL - Apple"}, nil, nil, secIDs, nil,
		),
		transferSharesDialogData:        &transferSharesDialogData{},
		transferSharesDialogAccountIDs:  nil,
		transferSharesDialogSecurityIDs: secIDs,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.transferSharesDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[3].Value = "10"

	model, cmd := app.submitTransferSharesDialog()
	updatedApp := model.(*App)

	if updatedApp.transferSharesDialog == nil {
		t.Error("dialog should remain open when no accounts")
	}
	if cmd != nil {
		t.Error("should not return command when no accounts")
	}
}

func TestSubmitTransferSharesDialog_NoSecurities(t *testing.T) {
	acctID := types.NewID()
	destAcctIDs := []types.ID{types.NewID()}

	app := &App{
		transferSharesDialog: buildTransferSharesDialog(
			[]string{"IRA"}, []string{}, nil, destAcctIDs, nil, nil,
		),
		transferSharesDialogData: &transferSharesDialogData{
			investmentIDs: destAcctIDs,
		},
		transferSharesDialogAccountIDs:  destAcctIDs,
		transferSharesDialogSecurityIDs: nil,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.transferSharesDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[3].Value = "10"

	model, cmd := app.submitTransferSharesDialog()
	updatedApp := model.(*App)

	if updatedApp.transferSharesDialog == nil {
		t.Error("dialog should remain open when no securities")
	}
	if cmd != nil {
		t.Error("should not return command when no securities")
	}
}

func TestSubmitTransferSharesDialog_ValidTransfer(t *testing.T) {
	acctID := types.NewID()
	destAcctIDs := []types.ID{types.NewID()}
	secIDs := []types.ID{types.NewID()}

	app := &App{
		transferSharesDialog: buildTransferSharesDialog(
			[]string{"IRA"}, []string{"AAPL - Apple"}, nil, destAcctIDs, secIDs, nil,
		),
		transferSharesDialogData: &transferSharesDialogData{
			investmentIDs: destAcctIDs,
		},
		transferSharesDialogAccountIDs:  destAcctIDs,
		transferSharesDialogSecurityIDs: secIDs,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.transferSharesDialog.Fields()
	fields[0].Value = "03/15/2024"
	fields[3].Value = "10"

	model, cmd := app.submitTransferSharesDialog()
	updatedApp := model.(*App)

	if updatedApp.transferSharesDialog != nil {
		t.Error("dialog should be closed after valid submit")
	}
	if cmd == nil {
		t.Error("should return a command for async save")
	}
}

func TestSubmitTransferSharesDialog_ValidWithMemo(t *testing.T) {
	acctID := types.NewID()
	destAcctIDs := []types.ID{types.NewID()}
	secIDs := []types.ID{types.NewID()}

	app := &App{
		transferSharesDialog: buildTransferSharesDialog(
			[]string{"IRA"}, []string{"AAPL - Apple"}, nil, destAcctIDs, secIDs, nil,
		),
		transferSharesDialogData: &transferSharesDialogData{
			investmentIDs: destAcctIDs,
		},
		transferSharesDialogAccountIDs:  destAcctIDs,
		transferSharesDialogSecurityIDs: secIDs,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
			},
		},
	}

	fields := app.transferSharesDialog.Fields()
	fields[0].Value = "06/01/2024"
	fields[3].Value = "25"
	fields[4].Value = "Move shares to IRA"

	model, cmd := app.submitTransferSharesDialog()
	updatedApp := model.(*App)

	if updatedApp.transferSharesDialog != nil {
		t.Error("dialog should close on valid submit with memo")
	}
	if cmd == nil {
		t.Error("should return command")
	}
}

func TestSubmitTransferSharesDialog_WithLotAllocations(t *testing.T) {
	acctID := types.NewID()
	destAcctIDs := []types.ID{types.NewID()}
	secIDs := []types.ID{types.NewID()}

	lot1 := &investment.Lot{
		BaseModel:    types.BaseModel{ID: types.NewID()},
		PurchaseDate: types.NewDate(2024, time.January, 15),
		Shares:       types.MustNewQuantity("100"),
		CostPerShare: types.MustNewMoney("150.00"),
	}
	lot2 := &investment.Lot{
		BaseModel:    types.BaseModel{ID: types.NewID()},
		PurchaseDate: types.NewDate(2024, time.June, 1),
		Shares:       types.MustNewQuantity("50"),
		CostPerShare: types.MustNewMoney("175.00"),
	}
	lots := []*investment.Lot{lot1, lot2}

	app := &App{
		transferSharesDialog: buildTransferSharesDialog(
			[]string{"IRA"}, []string{"AAPL - Apple"}, nil, destAcctIDs, secIDs, lots,
		),
		transferSharesDialogData: &transferSharesDialogData{
			investmentIDs: destAcctIDs,
			lots:          lots,
		},
		transferSharesDialogAccountIDs:  destAcctIDs,
		transferSharesDialogSecurityIDs: secIDs,
		transferSharesDialogLots:        lots,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
				TrackLots: true,
			},
		},
	}

	fields := app.transferSharesDialog.Fields()
	fields[0].Value = "03/15/2024" // date
	fields[3].Value = "30"         // total shares
	fields[4].Value = "20"         // 20 from lot 1
	fields[5].Value = "10"         // 10 from lot 2
	fields[6].Value = "Lot transfer"

	model, cmd := app.submitTransferSharesDialog()
	updatedApp := model.(*App)

	if updatedApp.transferSharesDialog != nil {
		t.Error("dialog should be closed after valid lot allocation submit")
	}
	if cmd == nil {
		t.Error("should return a command for async save")
	}
}

func TestSubmitTransferSharesDialog_LotAllocationMismatch(t *testing.T) {
	acctID := types.NewID()
	destAcctIDs := []types.ID{types.NewID()}
	secIDs := []types.ID{types.NewID()}

	lot1 := &investment.Lot{
		BaseModel:    types.BaseModel{ID: types.NewID()},
		PurchaseDate: types.NewDate(2024, time.January, 15),
		Shares:       types.MustNewQuantity("100"),
		CostPerShare: types.MustNewMoney("150.00"),
	}
	lots := []*investment.Lot{lot1}

	app := &App{
		transferSharesDialog: buildTransferSharesDialog(
			[]string{"IRA"}, []string{"AAPL - Apple"}, nil, destAcctIDs, secIDs, lots,
		),
		transferSharesDialogData: &transferSharesDialogData{
			investmentIDs: destAcctIDs,
			lots:          lots,
		},
		transferSharesDialogAccountIDs:  destAcctIDs,
		transferSharesDialogSecurityIDs: secIDs,
		transferSharesDialogLots:        lots,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
				TrackLots: true,
			},
		},
	}

	fields := app.transferSharesDialog.Fields()
	fields[0].Value = "03/15/2024" // date
	fields[3].Value = "30"         // total shares = 30
	fields[4].Value = "20"         // only 20 allocated from lot (mismatch!)

	model, cmd := app.submitTransferSharesDialog()
	updatedApp := model.(*App)

	if updatedApp.transferSharesDialog == nil {
		t.Error("dialog should remain open on lot allocation mismatch")
	}
	if cmd != nil {
		t.Error("should not return command on lot allocation mismatch")
	}

	fields = updatedApp.transferSharesDialog.Fields()
	if fields[3].Error == "" {
		t.Error("shares field should have allocation mismatch error")
	}
}

func TestSubmitTransferSharesDialog_LotExceedsAvailable(t *testing.T) {
	acctID := types.NewID()
	destAcctIDs := []types.ID{types.NewID()}
	secIDs := []types.ID{types.NewID()}

	lot1 := &investment.Lot{
		BaseModel:    types.BaseModel{ID: types.NewID()},
		PurchaseDate: types.NewDate(2024, time.January, 15),
		Shares:       types.MustNewQuantity("10"),
		CostPerShare: types.MustNewMoney("150.00"),
	}
	lots := []*investment.Lot{lot1}

	app := &App{
		transferSharesDialog: buildTransferSharesDialog(
			[]string{"IRA"}, []string{"AAPL - Apple"}, nil, destAcctIDs, secIDs, lots,
		),
		transferSharesDialogData: &transferSharesDialogData{
			investmentIDs: destAcctIDs,
			lots:          lots,
		},
		transferSharesDialogAccountIDs:  destAcctIDs,
		transferSharesDialogSecurityIDs: secIDs,
		transferSharesDialogLots:        lots,
		investmentRegister: &investmentRegisterData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: acctID},
				Type:      account.TypeInvestment,
				TrackLots: true,
			},
		},
	}

	fields := app.transferSharesDialog.Fields()
	fields[0].Value = "03/15/2024" // date
	fields[3].Value = "20"         // want 20 shares
	fields[4].Value = "20"         // try 20 from lot that only has 10

	model, cmd := app.submitTransferSharesDialog()
	updatedApp := model.(*App)

	if updatedApp.transferSharesDialog == nil {
		t.Error("dialog should remain open when lot exceeds available")
	}
	if cmd != nil {
		t.Error("should not return command when lot exceeds available")
	}

	fields = updatedApp.transferSharesDialog.Fields()
	if fields[4].Error == "" {
		t.Error("lot field should have error about insufficient shares")
	}
}

// --- Handle key tests ---

func TestHandleTransferSharesDialogKey_Cancel(t *testing.T) {
	app := &App{
		transferSharesDialog: buildTransferSharesDialog(
			[]string{"IRA"}, []string{"AAPL"}, nil, nil, nil, nil,
		),
	}

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
	model, _ := app.handleTransferSharesDialogKey(escKey)
	updatedApp := model.(*App)

	if updatedApp.transferSharesDialog != nil {
		t.Error("dialog should be closed after Escape")
	}
}

func TestHandleTransferSharesDialogKey_NilDialog(t *testing.T) {
	app := &App{}

	escKey := tea.KeyPressMsg{Code: tea.KeyEscape}
	model, cmd := app.handleTransferSharesDialogKey(escKey)

	if model.(*App) != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

// --- Submit nil dialog tests ---

func TestSubmitTransferSharesDialog_NilDialog(t *testing.T) {
	app := &App{}

	model, cmd := app.submitTransferSharesDialog()

	if model.(*App) != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

// --- Close dialog tests ---

func TestCloseTransferSharesDialog(t *testing.T) {
	app := &App{
		transferSharesDialog: buildTransferSharesDialog(
			[]string{"IRA"}, []string{"AAPL"}, nil, nil, nil, nil,
		),
		transferSharesDialogData:        &transferSharesDialogData{},
		transferSharesDialogAccountIDs:  []types.ID{types.NewID()},
		transferSharesDialogSecurityIDs: []types.ID{types.NewID()},
		transferSharesDialogLots:        []*investment.Lot{},
	}

	app.closeTransferSharesDialog()

	if app.transferSharesDialog != nil {
		t.Error("transferSharesDialog should be nil after close")
	}
	if app.transferSharesDialogData != nil {
		t.Error("transferSharesDialogData should be nil after close")
	}
	if app.transferSharesDialogAccountIDs != nil {
		t.Error("transferSharesDialogAccountIDs should be nil after close")
	}
	if app.transferSharesDialogSecurityIDs != nil {
		t.Error("transferSharesDialogSecurityIDs should be nil after close")
	}
	if app.transferSharesDialogLots != nil {
		t.Error("transferSharesDialogLots should be nil after close")
	}
}

// --- Message type tests ---

func TestTransferSharesDialogSavedMsg(t *testing.T) {
	msg := transferSharesDialogSavedMsg{}
	_ = msg
}

func TestTransferSharesDialogDataMsg(t *testing.T) {
	msg := transferSharesDialogDataMsg{data: &transferSharesDialogData{}}
	if msg.data == nil {
		t.Error("data should not be nil")
	}
}
