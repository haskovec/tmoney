package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// Dispatcher Tests (P1-006)
//
// The dispatcher itself was lifted to internal/transaction/dispatch.go in
// P-A1-002 and is exhaustively tested there
// (TestChooseTransferDispatch_AllFourCombinations). The TUI-side tests below
// keep a thin smoke check that the TUI still pins the right dispatch result
// for each of the four kinds — the assertions also document which
// transaction.Dispatch* constant maps to which input shape.
// =============================================================================

func TestChooseTransferDispatch_RegToReg(t *testing.T) {
	got := transaction.ChooseTransferDispatch(account.TypeChecking, account.TypeSavings)
	if got != transaction.DispatchRegToReg {
		t.Errorf("checking→savings dispatch = %v, want regToReg", got)
	}
}

func TestChooseTransferDispatch_InvToReg(t *testing.T) {
	got := transaction.ChooseTransferDispatch(account.TypeInvestment, account.TypeChecking)
	if got != transaction.DispatchInvToReg {
		t.Errorf("investment→checking dispatch = %v, want invToReg", got)
	}
}

func TestChooseTransferDispatch_RegToInv(t *testing.T) {
	got := transaction.ChooseTransferDispatch(account.TypeChecking, account.TypeInvestment)
	if got != transaction.DispatchRegToInv {
		t.Errorf("checking→investment dispatch = %v, want regToInv", got)
	}
}

func TestChooseTransferDispatch_InvToInv(t *testing.T) {
	got := transaction.ChooseTransferDispatch(account.TypeInvestment, account.TypeInvestment)
	if got != transaction.DispatchInvToInv {
		t.Errorf("investment→investment dispatch = %v, want invToInv", got)
	}
}

// HSA counts as an investment-type account (per account.Type.IsInvestmentType),
// so it must take the investment-side dispatch paths on both ends — otherwise
// an HSA → checking sweep would create a malformed regular transaction in the
// HSA register.
func TestChooseTransferDispatch_HSATreatedAsInvestment(t *testing.T) {
	if got := transaction.ChooseTransferDispatch(account.TypeHSA, account.TypeChecking); got != transaction.DispatchInvToReg {
		t.Errorf("HSA→checking dispatch = %v, want invToReg", got)
	}
	if got := transaction.ChooseTransferDispatch(account.TypeChecking, account.TypeHSA); got != transaction.DispatchRegToInv {
		t.Errorf("checking→HSA dispatch = %v, want regToInv", got)
	}
	if got := transaction.ChooseTransferDispatch(account.TypeHSA, account.TypeInvestment); got != transaction.DispatchInvToInv {
		t.Errorf("HSA→investment dispatch = %v, want invToInv", got)
	}
}

func TestAccountTypeByID_Found(t *testing.T) {
	id := types.NewID()
	accts := []*account.Account{
		{BaseModel: types.BaseModel{ID: types.NewID()}, Type: account.TypeChecking},
		{BaseModel: types.BaseModel{ID: id}, Type: account.TypeInvestment},
	}
	if got := accountTypeByID(accts, id); got != account.TypeInvestment {
		t.Errorf("accountTypeByID = %v, want investment", got)
	}
}

func TestAccountTypeByID_NotFound(t *testing.T) {
	accts := []*account.Account{
		{BaseModel: types.BaseModel{ID: types.NewID()}, Type: account.TypeChecking},
	}
	got := accountTypeByID(accts, types.NewID())
	if got != "" {
		t.Errorf("accountTypeByID for missing ID = %q, want empty (so dispatcher falls through to reg/reg)", got)
	}
	// Empty type must dispatch to reg/reg so callers don't accidentally route
	// to an investment path with an unknown account.
	if transaction.ChooseTransferDispatch(got, account.TypeChecking) != transaction.DispatchRegToReg {
		t.Error("unknown account type should dispatch to regToReg")
	}
}

// TestApp_SubmitTransferDialog_DispatchesInvToInv exercises submitTransferDialog
// with two investment accounts in the dialog data and asserts the dialog
// closes and a cmd is produced. The actual undo command construction lives
// inside the returned closure, so we only verify the synchronous path here;
// end-to-end correctness of the investment-side service calls is covered by
// the service- and undo-package tests.
func TestApp_SubmitTransferDialog_DispatchesInvToInv(t *testing.T) {
	fromID := types.NewID()
	toID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transfer")
			d.AddSelectField("From", []string{"IRA A", "IRA B"}, 0)
			d.AddSelectField("To", []string{"IRA A", "IRA B"}, 1)
			d.AddTextField("Amount", "1000.00", "", 12)
			d.AddDateField("Date", "01/15/2024")
			d.AddTextField("Memo", "rollover", "", 0)
			d.SetVisible(true)
			return d
		}(),
		transferDialogData: &transferDialogData{
			accounts: []*account.Account{
				{BaseModel: types.BaseModel{ID: fromID}, Name: "IRA A", Type: account.TypeInvestment},
				{BaseModel: types.BaseModel{ID: toID}, Name: "IRA B", Type: account.TypeInvestment},
			},
		},
		transferDialogAccountIDs: []types.ID{fromID, toID},
	}

	model, cmd := app.submitTransferDialog()
	updatedApp := model.(*App)

	if cmd == nil {
		t.Fatal("inv↔inv transfer should return a non-nil cmd")
	}
	if updatedApp.transferDialog != nil {
		t.Error("transfer dialog should be closed after a valid submit")
	}
}

func TestApp_SubmitTransferDialog_DispatchesInvToReg(t *testing.T) {
	fromID := types.NewID()
	toID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transfer")
			d.AddSelectField("From", []string{"Brokerage", "Checking"}, 0)
			d.AddSelectField("To", []string{"Brokerage", "Checking"}, 1)
			d.AddTextField("Amount", "250.00", "", 12)
			d.AddDateField("Date", "01/15/2024")
			d.AddTextField("Memo", "", "", 0)
			d.SetVisible(true)
			return d
		}(),
		transferDialogData: &transferDialogData{
			accounts: []*account.Account{
				{BaseModel: types.BaseModel{ID: fromID}, Name: "Brokerage", Type: account.TypeInvestment},
				{BaseModel: types.BaseModel{ID: toID}, Name: "Checking", Type: account.TypeChecking},
			},
		},
		transferDialogAccountIDs: []types.ID{fromID, toID},
	}

	model, cmd := app.submitTransferDialog()
	updatedApp := model.(*App)

	if cmd == nil {
		t.Fatal("inv→reg transfer should return a non-nil cmd")
	}
	if updatedApp.transferDialog != nil {
		t.Error("transfer dialog should be closed after a valid submit")
	}
}

func TestApp_SubmitTransferDialog_DispatchesRegToInv(t *testing.T) {
	fromID := types.NewID()
	toID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transfer")
			d.AddSelectField("From", []string{"Checking", "Brokerage"}, 0)
			d.AddSelectField("To", []string{"Checking", "Brokerage"}, 1)
			d.AddTextField("Amount", "500.00", "", 12)
			d.AddDateField("Date", "01/15/2024")
			d.AddTextField("Memo", "", "", 0)
			d.SetVisible(true)
			return d
		}(),
		transferDialogData: &transferDialogData{
			accounts: []*account.Account{
				{BaseModel: types.BaseModel{ID: fromID}, Name: "Checking", Type: account.TypeChecking},
				{BaseModel: types.BaseModel{ID: toID}, Name: "Brokerage", Type: account.TypeInvestment},
			},
		},
		transferDialogAccountIDs: []types.ID{fromID, toID},
	}

	model, cmd := app.submitTransferDialog()
	updatedApp := model.(*App)

	if cmd == nil {
		t.Fatal("reg→inv transfer should return a non-nil cmd")
	}
	if updatedApp.transferDialog != nil {
		t.Error("transfer dialog should be closed after a valid submit")
	}
}

// =============================================================================
// Pure Function Tests
// =============================================================================

func TestBuildAccountOptions(t *testing.T) {
	accounts := []*account.Account{
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Checking"},
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Savings"},
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Credit Card"},
	}

	options, ids := buildAccountOptions(accounts)

	if len(options) != 3 {
		t.Fatalf("expected 3 options, got %d", len(options))
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}

	if options[0] != "Checking" {
		t.Errorf("options[0] = %q, want %q", options[0], "Checking")
	}
	if options[1] != "Savings" {
		t.Errorf("options[1] = %q, want %q", options[1], "Savings")
	}
	if options[2] != "Credit Card" {
		t.Errorf("options[2] = %q, want %q", options[2], "Credit Card")
	}

	for i, acct := range accounts {
		if ids[i] != acct.ID {
			t.Errorf("ids[%d] = %v, want %v", i, ids[i], acct.ID)
		}
	}
}

func TestBuildAccountOptions_Empty(t *testing.T) {
	options, ids := buildAccountOptions(nil)

	if len(options) != 0 {
		t.Errorf("expected 0 options, got %d", len(options))
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 IDs, got %d", len(ids))
	}
}

func TestBuildTransferDialog(t *testing.T) {
	options := []string{"Checking", "Savings", "Credit Card"}
	catOptions := []string{"(None)", "Food", "Bills"}

	d := buildTransferDialog(options, catOptions, 0)

	if d.Title() != "New Transfer" {
		t.Errorf("title = %q, want %q", d.Title(), "New Transfer")
	}

	if !d.IsVisible() {
		t.Error("dialog should be visible after creation")
	}

	fields := d.Fields()
	if len(fields) != 6 {
		t.Fatalf("expected 6 fields, got %d", len(fields))
	}
}

func TestBuildTransferDialog_FieldTypes(t *testing.T) {
	options := []string{"Checking", "Savings"}
	catOptions := []string{"(None)", "Food"}

	d := buildTransferDialog(options, catOptions, 0)
	fields := d.Fields()

	expected := []struct {
		label     string
		fieldType dialog.FieldType
	}{
		{"From", dialog.FieldSelect},
		{"To", dialog.FieldSelect},
		{"Amount", dialog.FieldText},
		{"Date", dialog.FieldDate},
		{"Memo", dialog.FieldText},
		{"Category", dialog.FieldCombo},
	}

	if len(fields) != len(expected) {
		t.Fatalf("expected %d fields, got %d", len(expected), len(fields))
	}

	for i, exp := range expected {
		if fields[i].Label != exp.label {
			t.Errorf("field[%d] label = %q, want %q", i, fields[i].Label, exp.label)
		}
		if fields[i].Type != exp.fieldType {
			t.Errorf("field[%d] type = %v, want %v", i, fields[i].Type, exp.fieldType)
		}
	}

	// The Category combo exposes the inline create-category action row.
	if fields[5].AddNewLabel == "" {
		t.Error("Category combo should set AddNewLabel for inline creation")
	}
}

func TestBuildTransferDialog_DefaultFromIndex(t *testing.T) {
	options := []string{"Checking", "Savings", "Credit Card"}

	d := buildTransferDialog(options, nil, 1)
	fields := d.Fields()

	// From should default to index 1 (Savings)
	if fields[0].SelectedIndex != 1 {
		t.Errorf("From selectedIndex = %d, want 1", fields[0].SelectedIndex)
	}

	// To should default to 0 since From is 1
	if fields[1].SelectedIndex != 0 {
		t.Errorf("To selectedIndex = %d, want 0", fields[1].SelectedIndex)
	}
}

func TestBuildTransferDialog_DefaultFromZero(t *testing.T) {
	options := []string{"Checking", "Savings"}

	d := buildTransferDialog(options, nil, 0)
	fields := d.Fields()

	// From should default to index 0 (Checking)
	if fields[0].SelectedIndex != 0 {
		t.Errorf("From selectedIndex = %d, want 0", fields[0].SelectedIndex)
	}

	// To should default to 1 (Savings) to avoid same account
	if fields[1].SelectedIndex != 1 {
		t.Errorf("To selectedIndex = %d, want 1", fields[1].SelectedIndex)
	}
}

func TestBuildTransferDialog_DateDefault(t *testing.T) {
	options := []string{"Checking", "Savings"}

	d := buildTransferDialog(options, nil, 0)
	fields := d.Fields()

	today := time.Now().Format("01/02/2006")
	if fields[3].Value != today {
		t.Errorf("date default = %q, want %q", fields[3].Value, today)
	}
}

// TestBuildTransferDialog_DateFieldOverwriteSemantics asserts that the Date
// field built by buildTransferDialog uses the dialog.FieldDate widget's overwrite
// semantics: typing two digits overwrites the month digits in place and the
// resulting Value is still a canonical 10-char MM/DD/YYYY string.
func TestBuildTransferDialog_DateFieldOverwriteSemantics(t *testing.T) {
	options := []string{"Checking", "Savings"}

	d := buildTransferDialog(options, nil, 0)
	d.SetFocusIndex(3) // Date field

	// Pre-load a known value so the assertion is deterministic.
	d.Fields()[3].Value = "01/15/2024"

	// Type "0" then "2" — should rewrite the month from "01" to "02".
	d.HandleKey(tea.KeyPressMsg{Code: '0', Text: "0"})
	d.HandleKey(tea.KeyPressMsg{Code: '2', Text: "2"})

	got := d.Fields()[3].Value
	want := "02/15/2024"
	if got != want {
		t.Errorf("after typing 0,2 over month: Value = %q, want %q", got, want)
	}
	if len(got) != 10 {
		t.Errorf("Value len = %d, want 10 (canonical MM/DD/YYYY)", len(got))
	}
}

func TestBuildTransferDialog_SingleAccount(t *testing.T) {
	options := []string{"Checking"}

	d := buildTransferDialog(options, nil, 0)
	fields := d.Fields()

	// From should be 0
	if fields[0].SelectedIndex != 0 {
		t.Errorf("From selectedIndex = %d, want 0", fields[0].SelectedIndex)
	}

	// To should also be 0 (only one account)
	if fields[1].SelectedIndex != 0 {
		t.Errorf("To selectedIndex = %d, want 0", fields[1].SelectedIndex)
	}
}

// =============================================================================
// App Integration Tests (no database)
// =============================================================================

func TestApp_HandleRegisterKeys_TransferKey(t *testing.T) {
	accountID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		width:       120,
		height:      30,
		styles:      widget.NewStyles(),
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		register: &registerData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: accountID},
				Name:      "Checking",
				Active:    true,
			},
			transactions:  []*transaction.Transaction{},
			balance:       &account.Balance{AccountID: accountID, CurrentBalance: types.ZeroMoney},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}
	app.buildRegisterTable()
	app.sidebar.SetFocused(false)
	app.table.SetFocused(true)

	// Press 't' for transfer
	tKey := tea.KeyPressMsg{Code: 't', Text: "t"}
	_, cmd := app.Update(tKey)

	if cmd == nil {
		t.Error("pressing 't' in register should return a non-nil cmd")
	}
}

func TestApp_Update_TransferDialogDataMsg(t *testing.T) {
	checkingID := types.NewID()
	savingsID := types.NewID()

	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	// Set up sidebar with a selected account
	app.sidebar.SetAccounts([]*account.Account{
		{BaseModel: types.BaseModel{ID: checkingID}, Name: "Checking", Active: true, Type: account.TypeChecking},
		{BaseModel: types.BaseModel{ID: savingsID}, Name: "Savings", Active: true, Type: account.TypeSavings},
	}, nil)

	data := &transferDialogData{
		accounts: []*account.Account{
			{BaseModel: types.BaseModel{ID: checkingID}, Name: "Checking"},
			{BaseModel: types.BaseModel{ID: savingsID}, Name: "Savings"},
		},
		accountIDs: []types.ID{checkingID, savingsID},
	}

	msg := transferDialogDataMsg{data: data}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.transferDialog == nil {
		t.Fatal("transfer dialog should be created")
	}
	if !updatedApp.transferDialog.IsVisible() {
		t.Error("transfer dialog should be visible")
	}
	if updatedApp.transferDialogData == nil {
		t.Error("transfer dialog data should be set")
	}
	if updatedApp.transferDialogAccountIDs == nil {
		t.Error("transfer dialog account IDs should be set")
	}
	if len(updatedApp.transferDialogAccountIDs) != 2 {
		t.Errorf("expected 2 account IDs, got %d", len(updatedApp.transferDialogAccountIDs))
	}
}

func TestApp_Update_TransferDialogDataMsg_SeedsFromStickyDate(t *testing.T) {
	checkingID := types.NewID()
	savingsID := types.NewID()

	app := &App{
		currentView:            ViewRegister,
		keys:                   defaultKeyMap(),
		menubar:                widget.NewMenuBar(),
		statusbar:              widget.NewStatusBar(),
		sidebar:                NewSidebar(),
		txnDialogLastSavedDate: types.NewDate(2024, time.January, 15),
	}
	app.sidebar.SetAccounts([]*account.Account{
		{BaseModel: types.BaseModel{ID: checkingID}, Name: "Checking", Active: true, Type: account.TypeChecking},
		{BaseModel: types.BaseModel{ID: savingsID}, Name: "Savings", Active: true, Type: account.TypeSavings},
	}, nil)

	data := &transferDialogData{
		accounts: []*account.Account{
			{BaseModel: types.BaseModel{ID: checkingID}, Name: "Checking"},
			{BaseModel: types.BaseModel{ID: savingsID}, Name: "Savings"},
		},
		accountIDs: []types.ID{checkingID, savingsID},
	}

	model, _ := app.Update(transferDialogDataMsg{data: data})
	updatedApp := model.(*App)

	if updatedApp.transferDialog == nil {
		t.Fatal("transfer dialog should be created")
	}
	// New-transfer field order: From(0), To(1), Amount(2), Date(3), Memo(4).
	dateValue := updatedApp.transferDialog.Fields()[3].Value
	if dateValue != "01/15/2024" {
		t.Errorf("date field = %q, want %q (seeded from sticky date)", dateValue, "01/15/2024")
	}
}

func TestApp_SubmitTransferDialog_PassesSavedDateInMessage(t *testing.T) {
	fromID := types.NewID()
	toID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transfer")
			d.AddSelectField("From", []string{"Checking", "Savings"}, 0)
			d.AddSelectField("To", []string{"Checking", "Savings"}, 1)
			d.AddTextField("Amount", "500.00", "", 12)
			d.AddDateField("Date", "01/15/2024")
			d.AddTextField("Memo", "", "", 0)
			d.SetVisible(true)
			return d
		}(),
		transferDialogData: &transferDialogData{
			accounts: []*account.Account{},
		},
		transferDialogAccountIDs: []types.ID{fromID, toID},
	}

	_, cmd := app.submitTransferDialog()
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	// The cmd performs a real DB write through undoManager, so we can't
	// safely execute it here. Instead, assert the sticky-date wiring by
	// feeding a saved message directly and checking the app updates.
	saved := transferDialogSavedMsg{savedDate: types.NewDate(2024, time.January, 15)}
	model, _ := app.Update(saved)
	updatedApp := model.(*App)
	want := types.NewDate(2024, time.January, 15)
	if !updatedApp.txnDialogLastSavedDate.Equal(want) {
		t.Errorf("sticky date = %s, want %s", updatedApp.txnDialogLastSavedDate, want)
	}
}

func TestApp_HandleTransferDialogKey_Cancel(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transfer")
			d.AddSelectField("From", []string{"Checking", "Savings"}, 0)
			d.AddSelectField("To", []string{"Checking", "Savings"}, 1)
			d.AddTextField("Amount", "", "100.00", 12)
			d.AddDateField("Date", "01/01/2024")
			d.AddTextField("Memo", "", "", 0)
			d.SetVisible(true)
			return d
		}(),
		transferDialogData: &transferDialogData{
			accounts: []*account.Account{},
		},
		transferDialogAccountIDs: []types.ID{types.NewID(), types.NewID()},
	}

	// Press Escape to cancel
	escKey := tea.KeyPressMsg{Code: tea.KeyEsc}
	model, _ := app.Update(escKey)
	updatedApp := model.(*App)

	if updatedApp.transferDialog != nil {
		t.Error("transfer dialog should be nil after cancel")
	}
	if updatedApp.transferDialogData != nil {
		t.Error("transfer dialog data should be nil after cancel")
	}
	if updatedApp.transferDialogAccountIDs != nil {
		t.Error("transfer dialog account IDs should be nil after cancel")
	}
}

func TestApp_HandleTransferDialogKey_TabCycles(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transfer")
			d.AddSelectField("From", []string{"Checking", "Savings"}, 0)
			d.AddSelectField("To", []string{"Checking", "Savings"}, 1)
			d.AddTextField("Amount", "", "100.00", 12)
			d.AddDateField("Date", "01/01/2024")
			d.AddTextField("Memo", "", "", 0)
			d.SetVisible(true)
			return d
		}(),
		transferDialogData: &transferDialogData{
			accounts: []*account.Account{},
		},
		transferDialogAccountIDs: []types.ID{types.NewID(), types.NewID()},
	}

	initialFocus := app.transferDialog.FocusIndex()
	if initialFocus != 0 {
		t.Fatalf("initial focus = %d, want 0", initialFocus)
	}

	// Press Tab to advance focus
	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	model, _ := app.Update(tabKey)
	updatedApp := model.(*App)

	if updatedApp.transferDialog.FocusIndex() != 1 {
		t.Errorf("focus after Tab = %d, want 1", updatedApp.transferDialog.FocusIndex())
	}
}

func TestApp_Update_TransferDialogSavedMsg(t *testing.T) {
	accountID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}
	// Set up sidebar with a selected account
	app.sidebar.SetAccounts([]*account.Account{
		{BaseModel: types.BaseModel{ID: accountID}, Name: "Checking", Active: true, Type: account.TypeChecking},
	}, nil)

	msg := transferDialogSavedMsg{}
	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Error("transferDialogSavedMsg should return a reload command")
	}
}

// TestApp_Update_TransferDialogSavedMsg_SelectsRegularLeg verifies the saved
// regular leg is queued for selection in the regular register.
func TestApp_Update_TransferDialogSavedMsg_SelectsRegularLeg(t *testing.T) {
	legID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	msg := transferDialogSavedMsg{savedID: legID, savedIsInvestment: false}
	model, _ := app.Update(msg)
	updated := model.(*App)

	if updated.pendingRegisterSelectID != legID {
		t.Errorf("pendingRegisterSelectID = %v, want %v", updated.pendingRegisterSelectID, legID)
	}
	if !updated.pendingInvestmentSelectID.IsNil() {
		t.Error("pendingInvestmentSelectID should stay unset for a regular leg")
	}
}

// TestApp_Update_TransferDialogSavedMsg_SelectsInvestmentLeg verifies an
// investment-side leg routes to the investment register's pending selection.
func TestApp_Update_TransferDialogSavedMsg_SelectsInvestmentLeg(t *testing.T) {
	legID := types.NewID()
	app := &App{
		currentView: ViewInvestmentRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	msg := transferDialogSavedMsg{savedID: legID, savedIsInvestment: true}
	model, _ := app.Update(msg)
	updated := model.(*App)

	if updated.pendingInvestmentSelectID != legID {
		t.Errorf("pendingInvestmentSelectID = %v, want %v", updated.pendingInvestmentSelectID, legID)
	}
	if !updated.pendingRegisterSelectID.IsNil() {
		t.Error("pendingRegisterSelectID should stay unset for an investment leg")
	}
}

// TestApp_Update_TransferDialogSavedMsg_NilLegSelectsNothing verifies that a
// transfer whose legs don't belong to the current register (NilID savedID)
// leaves both pending selections untouched.
func TestApp_Update_TransferDialogSavedMsg_NilLegSelectsNothing(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	model, _ := app.Update(transferDialogSavedMsg{savedID: types.NilID})
	updated := model.(*App)

	if !updated.pendingRegisterSelectID.IsNil() || !updated.pendingInvestmentSelectID.IsNil() {
		t.Error("a NilID savedID should leave both pending selections unset")
	}
}

func TestTransferLegForAccount(t *testing.T) {
	acctA, acctB := types.NewID(), types.NewID()
	fromID, toID := types.NewID(), types.NewID()
	from := &transaction.Transaction{BaseModel: types.BaseModel{ID: fromID}, AccountID: acctA}
	to := &transaction.Transaction{BaseModel: types.BaseModel{ID: toID}, AccountID: acctB}

	if got := transferLegForAccount(acctA, from, to); got != fromID {
		t.Errorf("acctA leg = %v, want from leg %v", got, fromID)
	}
	if got := transferLegForAccount(acctB, from, to); got != toID {
		t.Errorf("acctB leg = %v, want to leg %v", got, toID)
	}
	if got := transferLegForAccount(types.NewID(), from, to); !got.IsNil() {
		t.Errorf("unrelated account leg = %v, want NilID", got)
	}
	// nil legs must not panic.
	if got := transferLegForAccount(acctA, nil, nil); !got.IsNil() {
		t.Errorf("nil legs = %v, want NilID", got)
	}
}

func TestCashTransferLegForAccount(t *testing.T) {
	regAcct, invAcct := types.NewID(), types.NewID()
	regID, invID := types.NewID(), types.NewID()
	res := &investment.CashTransferResult{
		RegularTransaction:    &transaction.Transaction{BaseModel: types.BaseModel{ID: regID}, AccountID: regAcct},
		InvestmentTransaction: &investment.Transaction{BaseModel: types.BaseModel{ID: invID}, AccountID: invAcct},
	}

	if id, isInv := cashTransferLegForAccount(regAcct, res); id != regID || isInv {
		t.Errorf("regular account leg = (%v, %v), want (%v, false)", id, isInv, regID)
	}
	if id, isInv := cashTransferLegForAccount(invAcct, res); id != invID || !isInv {
		t.Errorf("investment account leg = (%v, %v), want (%v, true)", id, isInv, invID)
	}
	if id, isInv := cashTransferLegForAccount(types.NewID(), res); !id.IsNil() || isInv {
		t.Errorf("unrelated account = (%v, %v), want (NilID, false)", id, isInv)
	}
	if id, isInv := cashTransferLegForAccount(regAcct, nil); !id.IsNil() || isInv {
		t.Errorf("nil result = (%v, %v), want (NilID, false)", id, isInv)
	}
}

// TestCurrentRegisterAccountID verifies the helper reports the on-screen
// register's account and NilID elsewhere.
func TestCurrentRegisterAccountID(t *testing.T) {
	regAcct := &account.Account{BaseModel: types.BaseModel{ID: types.NewID()}, Type: account.TypeChecking}
	invAcct := &account.Account{BaseModel: types.BaseModel{ID: types.NewID()}, Type: account.TypeInvestment}

	app := &App{
		currentView:        ViewRegister,
		register:           &registerData{account: regAcct},
		investmentRegister: &investmentRegisterData{account: invAcct},
	}
	if got := app.currentRegisterAccountID(); got != regAcct.ID {
		t.Errorf("ViewRegister account = %v, want %v", got, regAcct.ID)
	}

	app.currentView = ViewInvestmentRegister
	if got := app.currentRegisterAccountID(); got != invAcct.ID {
		t.Errorf("ViewInvestmentRegister account = %v, want %v", got, invAcct.ID)
	}

	app.currentView = ViewDashboard
	if got := app.currentRegisterAccountID(); !got.IsNil() {
		t.Errorf("non-register view = %v, want NilID", got)
	}
}

func TestApp_SubmitTransferDialog_SameAccount(t *testing.T) {
	accountID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transfer")
			d.AddSelectField("From", []string{"Checking"}, 0)
			d.AddSelectField("To", []string{"Checking"}, 0) // same account
			d.AddTextField("Amount", "100.00", "", 12)
			d.AddDateField("Date", "01/01/2024")
			d.AddTextField("Memo", "", "", 0)
			d.SetVisible(true)
			return d
		}(),
		transferDialogData: &transferDialogData{
			accounts: []*account.Account{},
		},
		transferDialogAccountIDs: []types.ID{accountID},
	}

	_, cmd := app.submitTransferDialog()

	if cmd != nil {
		t.Error("same-account transfer should not return a cmd")
	}
	if app.transferDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.transferDialog.ErrorMsg() == "" {
		t.Error("same-account transfer should set dialog-level error")
	}
	if !strings.Contains(app.transferDialog.ErrorMsg(), "different") {
		t.Errorf("error = %q, should mention accounts must be different", app.transferDialog.ErrorMsg())
	}
}

func TestApp_SubmitTransferDialog_NegativeAmount(t *testing.T) {
	fromID := types.NewID()
	toID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transfer")
			d.AddSelectField("From", []string{"Checking", "Savings"}, 0)
			d.AddSelectField("To", []string{"Checking", "Savings"}, 1)
			d.AddTextField("Amount", "-50.00", "", 12)
			d.AddDateField("Date", "01/01/2024")
			d.AddTextField("Memo", "", "", 0)
			d.SetVisible(true)
			return d
		}(),
		transferDialogData: &transferDialogData{
			accounts: []*account.Account{},
		},
		transferDialogAccountIDs: []types.ID{fromID, toID},
	}

	_, cmd := app.submitTransferDialog()

	if cmd != nil {
		t.Error("negative amount transfer should not return a cmd")
	}
	if app.transferDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.transferDialog.Fields()[2].Error == "" {
		t.Error("negative amount should set field-level error on amount")
	}
	if !strings.Contains(app.transferDialog.Fields()[2].Error, "positive") {
		t.Errorf("error = %q, should mention amount must be positive", app.transferDialog.Fields()[2].Error)
	}
}

func TestApp_SubmitTransferDialog_InvalidDate(t *testing.T) {
	fromID := types.NewID()
	toID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transfer")
			d.AddSelectField("From", []string{"Checking", "Savings"}, 0)
			d.AddSelectField("To", []string{"Checking", "Savings"}, 1)
			d.AddTextField("Amount", "100.00", "", 12)
			d.AddDateField("Date", "13/45/2024")
			d.AddTextField("Memo", "", "", 0)
			d.SetVisible(true)
			return d
		}(),
		transferDialogData: &transferDialogData{
			accounts: []*account.Account{},
		},
		transferDialogAccountIDs: []types.ID{fromID, toID},
	}

	_, cmd := app.submitTransferDialog()

	if cmd != nil {
		t.Error("invalid date transfer should not return a cmd")
	}
	if app.transferDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.transferDialog.Fields()[3].Error == "" {
		t.Error("invalid date should set field-level error")
	}
}

func TestApp_SubmitTransferDialog_EmptyAmount(t *testing.T) {
	fromID := types.NewID()
	toID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transfer")
			d.AddSelectField("From", []string{"Checking", "Savings"}, 0)
			d.AddSelectField("To", []string{"Checking", "Savings"}, 1)
			d.AddTextField("Amount", "", "", 12)
			d.AddDateField("Date", "01/01/2024")
			d.AddTextField("Memo", "", "", 0)
			d.SetVisible(true)
			return d
		}(),
		transferDialogData: &transferDialogData{
			accounts: []*account.Account{},
		},
		transferDialogAccountIDs: []types.ID{fromID, toID},
	}

	_, cmd := app.submitTransferDialog()

	if cmd != nil {
		t.Error("empty amount transfer should not return a cmd")
	}
	if app.transferDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.transferDialog.Fields()[2].Error == "" {
		t.Error("empty amount should set field-level error")
	}
}

func TestApp_SubmitTransferDialog_ValidTransfer(t *testing.T) {
	fromID := types.NewID()
	toID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transfer")
			d.AddSelectField("From", []string{"Checking", "Savings"}, 0)
			d.AddSelectField("To", []string{"Checking", "Savings"}, 1)
			d.AddTextField("Amount", "500.00", "", 12)
			d.AddDateField("Date", "01/15/2024")
			d.AddTextField("Memo", "Monthly savings", "", 0)
			d.SetVisible(true)
			return d
		}(),
		transferDialogData: &transferDialogData{
			accounts: []*account.Account{},
		},
		transferDialogAccountIDs: []types.ID{fromID, toID},
	}

	model, cmd := app.submitTransferDialog()
	updatedApp := model.(*App)

	// Should return a cmd (async save)
	if cmd == nil {
		t.Error("valid transfer should return a non-nil cmd")
	}

	// dialog.Dialog should be closed
	if updatedApp.transferDialog != nil {
		t.Error("transfer dialog should be nil after submit")
	}
	if updatedApp.transferDialogData != nil {
		t.Error("transfer dialog data should be nil after submit")
	}

	// No immediate error
	if updatedApp.err != nil {
		t.Errorf("unexpected error: %v", updatedApp.err)
	}
}

func TestApp_CloseTransferDialog(t *testing.T) {
	app := &App{
		transferDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transfer")
			d.SetVisible(true)
			return d
		}(),
		transferDialogData:       &transferDialogData{},
		transferDialogAccountIDs: []types.ID{types.NewID()},
	}

	app.closeTransferDialog()

	if app.transferDialog != nil {
		t.Error("transfer dialog should be nil after close")
	}
	if app.transferDialogData != nil {
		t.Error("transfer dialog data should be nil after close")
	}
	if app.transferDialogAccountIDs != nil {
		t.Error("transfer dialog account IDs should be nil after close")
	}
}

func TestApp_RenderLayout_WithTransferDialog(t *testing.T) {
	styles := widget.NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewRegister,
		width:       100,
		height:      30,
		ready:       true,
		styles:      styles,
		sidebar:     NewSidebar(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		keys:        defaultKeyMap(),
		register: &registerData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: types.NewID()},
				Name:      "Checking",
				Active:    true,
			},
			transactions:  []*transaction.Transaction{},
			balance:       &account.Balance{CurrentBalance: types.ZeroMoney},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
		transferDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transfer")
			d.AddSelectField("From", []string{"Checking", "Savings"}, 0)
			d.SetVisible(true)
			return d
		}(),
	}

	output := app.renderLayout()
	if !strings.Contains(output, "New Transfer") {
		t.Error("renderLayout() should contain 'New Transfer' when transfer dialog is visible")
	}
}

func TestApp_TransferDialogDataMsg_PreSelectsFromAccount(t *testing.T) {
	checkingID := types.NewID()
	savingsID := types.NewID()

	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	// Set up sidebar with Savings selected
	app.sidebar.SetAccounts([]*account.Account{
		{BaseModel: types.BaseModel{ID: checkingID}, Name: "Checking", Active: true, Type: account.TypeChecking},
		{BaseModel: types.BaseModel{ID: savingsID}, Name: "Savings", Active: true, Type: account.TypeSavings},
	}, nil)
	// Select Savings account (second item - navigate down past the group header)
	app.sidebar.MoveDown() // to Checking
	app.sidebar.MoveDown() // to Savings
	app.sidebar.Select()

	data := &transferDialogData{
		accounts: []*account.Account{
			{BaseModel: types.BaseModel{ID: checkingID}, Name: "Checking"},
			{BaseModel: types.BaseModel{ID: savingsID}, Name: "Savings"},
		},
		accountIDs: []types.ID{checkingID, savingsID},
	}

	msg := transferDialogDataMsg{data: data}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.transferDialog == nil {
		t.Fatal("transfer dialog should be created")
	}

	fields := updatedApp.transferDialog.Fields()

	// The From account should be pre-selected to the sidebar's selected account
	selectedID := app.sidebar.SelectedAccountID()
	fromIdx := fields[0].SelectedIndex
	if fromIdx >= 0 && fromIdx < len(updatedApp.transferDialogAccountIDs) {
		if updatedApp.transferDialogAccountIDs[fromIdx] != selectedID {
			t.Errorf("From account should be pre-selected to sidebar account %v, got index %d", selectedID, fromIdx)
		}
	}
}

func TestApp_SubmitTransferDialog_ZeroAmount(t *testing.T) {
	fromID := types.NewID()
	toID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transfer")
			d.AddSelectField("From", []string{"Checking", "Savings"}, 0)
			d.AddSelectField("To", []string{"Checking", "Savings"}, 1)
			d.AddTextField("Amount", "0.00", "", 12)
			d.AddDateField("Date", "01/01/2024")
			d.AddTextField("Memo", "", "", 0)
			d.SetVisible(true)
			return d
		}(),
		transferDialogData: &transferDialogData{
			accounts: []*account.Account{},
		},
		transferDialogAccountIDs: []types.ID{fromID, toID},
	}

	_, cmd := app.submitTransferDialog()

	if cmd != nil {
		t.Error("zero amount transfer should not return a cmd")
	}
	if app.transferDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.transferDialog.Fields()[2].Error == "" {
		t.Error("zero amount should set field-level error")
	}
	if !strings.Contains(app.transferDialog.Fields()[2].Error, "positive") {
		t.Errorf("error = %q, should mention amount must be positive", app.transferDialog.Fields()[2].Error)
	}
}

// =============================================================================
// Edit-mode tests (Phase 2: transfer edit)
// =============================================================================

// TestBuildEditTransferDialog_Prefill asserts that the edit-mode transfer
// dialog is titled "Edit Transfer", carries a read-only "From → To" body
// message, and pre-fills Amount (positive), Date, Memo, and Status from the
// existing pair. From and To are NOT among the editable fields.
func TestBuildEditTransferDialog_Prefill(t *testing.T) {
	fromAcctID := types.NewID()
	toAcctID := types.NewID()
	transferID := types.NewID()

	pair := &transaction.TransferPair{
		FromTransaction: &transaction.Transaction{
			BaseModel:         types.BaseModel{ID: types.NewID()},
			AccountID:         fromAcctID,
			Date:              types.NewDate(2024, 3, 15),
			Amount:            types.MustNewMoney("-250.00"),
			Status:            transaction.StatusCleared,
			Memo:              types.NullableString{String: "rent split", Valid: true},
			TransferID:        types.NullableID{ID: transferID, Valid: true},
			TransferAccountID: types.NullableID{ID: toAcctID, Valid: true},
		},
		ToTransaction: &transaction.Transaction{
			BaseModel:         types.BaseModel{ID: types.NewID()},
			AccountID:         toAcctID,
			Date:              types.NewDate(2024, 3, 15),
			Amount:            types.MustNewMoney("250.00"),
			Status:            transaction.StatusCleared,
			Memo:              types.NullableString{String: "rent split", Valid: true},
			TransferID:        types.NullableID{ID: transferID, Valid: true},
			TransferAccountID: types.NullableID{ID: fromAcctID, Valid: true},
		},
	}

	amount := pair.ToTransaction.Amount
	date := pair.FromTransaction.Date
	memo := pair.FromTransaction.Memo.String
	status := pair.FromTransaction.Status
	catOptions := []string{"(None)", "Bills"}
	d := buildEditTransferDialog("Checking", "Savings", amount, date, memo, status, true, catOptions, 1)

	if d.Title() != "Edit Transfer" {
		t.Errorf("title = %q, want %q", d.Title(), "Edit Transfer")
	}
	if !strings.Contains(d.Message(), "Checking") || !strings.Contains(d.Message(), "Savings") {
		t.Errorf("message = %q, want to contain From and To names", d.Message())
	}

	fields := d.Fields()
	if len(fields) != 5 {
		t.Fatalf("expected 5 editable fields (Amount, Date, Memo, Category, Status), got %d", len(fields))
	}

	expected := []struct {
		label     string
		fieldType dialog.FieldType
	}{
		{"Amount", dialog.FieldText},
		{"Date", dialog.FieldDate},
		{"Memo", dialog.FieldText},
		{"Category", dialog.FieldCombo},
		{"Status", dialog.FieldRadio},
	}
	for i, exp := range expected {
		if fields[i].Label != exp.label {
			t.Errorf("field[%d] label = %q, want %q", i, fields[i].Label, exp.label)
		}
		if fields[i].Type != exp.fieldType {
			t.Errorf("field[%d] type = %v, want %v", i, fields[i].Type, exp.fieldType)
		}
	}

	if fields[0].Value != "250" {
		t.Errorf("Amount = %q, want %q (positive side)", fields[0].Value, "250")
	}
	if fields[1].Value != "03/15/2024" {
		t.Errorf("Date = %q, want %q", fields[1].Value, "03/15/2024")
	}
	if fields[2].Value != "rent split" {
		t.Errorf("Memo = %q, want %q", fields[2].Value, "rent split")
	}
	if fields[3].SelectedIndex != 1 {
		t.Errorf("Category SelectedIndex = %d, want 1 (seeded)", fields[3].SelectedIndex)
	}
	if fields[4].SelectedIndex != 1 {
		t.Errorf("Status SelectedIndex = %d, want 1 (Cleared)", fields[4].SelectedIndex)
	}
}

// TestBuildEditTransferDialog_InvToInvOmitsCategory pins that the inv↔inv edit
// layout keeps the pre-category shape (Amount, Date, Memo, Status) since
// neither leg can store a category.
func TestBuildEditTransferDialog_InvToInvOmitsCategory(t *testing.T) {
	d := buildEditTransferDialog("IRA A", "IRA B", types.MustNewMoney("1000"), types.NewDate(2024, 3, 15), "rollover", transaction.StatusUncleared, false, nil, 0)

	fields := d.Fields()
	if len(fields) != 4 {
		t.Fatalf("expected 4 fields (no Category), got %d", len(fields))
	}
	for _, f := range fields {
		if f.Label == "Category" {
			t.Error("inv↔inv edit dialog must not include a Category field")
		}
	}
	if fields[3].Label != "Status" {
		t.Errorf("field[3] label = %q, want Status", fields[3].Label)
	}
}

// TestApp_HandleRegisterKeys_EnterOnTransfer_OpensTransferEdit asserts Enter
// on a transfer row in the register returns a non-nil cmd — the loader for
// the edit-transfer flow.
func TestApp_HandleRegisterKeys_EnterOnTransfer_OpensTransferEdit(t *testing.T) {
	accountID := types.NewID()
	otherID := types.NewID()
	transferID := types.NewID()

	app := &App{
		currentView: ViewRegister,
		width:       120,
		height:      30,
		styles:      widget.NewStyles(),
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		register: &registerData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: accountID},
				Name:      "Checking",
				Active:    true,
			},
			transactions: []*transaction.Transaction{
				{
					BaseModel:         types.BaseModel{ID: types.NewID()},
					AccountID:         accountID,
					Date:              types.Today(),
					Amount:            types.MustNewMoney("-200"),
					Status:            transaction.StatusUncleared,
					TransferID:        types.NullableID{ID: transferID, Valid: true},
					TransferAccountID: types.NullableID{ID: otherID, Valid: true},
				},
			},
			balance:       &account.Balance{AccountID: accountID, CurrentBalance: types.ZeroMoney},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}
	app.buildRegisterTable()
	app.sidebar.SetFocused(false)
	app.table.SetFocused(true)

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	_, cmd := app.handleRegisterKeys(enter)

	if cmd == nil {
		t.Error("Enter on a transfer row should return a non-nil load cmd")
	}
}

// TestApp_Update_TransferDialogDataMsg_EditMode asserts that feeding a
// transferDialogDataMsg whose data is in edit mode results in the transfer
// dialog being constructed in edit mode (title "Edit Transfer", body shows
// account names, fields pre-filled).
func TestApp_Update_TransferDialogDataMsg_EditMode(t *testing.T) {
	checkingID := types.NewID()
	savingsID := types.NewID()
	transferID := types.NewID()

	pair := &transaction.TransferPair{
		FromTransaction: &transaction.Transaction{
			BaseModel:         types.BaseModel{ID: types.NewID()},
			AccountID:         checkingID,
			Date:              types.NewDate(2024, 4, 1),
			Amount:            types.MustNewMoney("-300"),
			Status:            transaction.StatusUncleared,
			TransferID:        types.NullableID{ID: transferID, Valid: true},
			TransferAccountID: types.NullableID{ID: savingsID, Valid: true},
		},
		ToTransaction: &transaction.Transaction{
			BaseModel:         types.BaseModel{ID: types.NewID()},
			AccountID:         savingsID,
			Date:              types.NewDate(2024, 4, 1),
			Amount:            types.MustNewMoney("300"),
			Status:            transaction.StatusUncleared,
			TransferID:        types.NullableID{ID: transferID, Valid: true},
			TransferAccountID: types.NullableID{ID: checkingID, Valid: true},
		},
	}

	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	data := &transferDialogData{
		accounts: []*account.Account{
			{BaseModel: types.BaseModel{ID: checkingID}, Name: "Checking"},
			{BaseModel: types.BaseModel{ID: savingsID}, Name: "Savings"},
		},
		accountIDs: []types.ID{checkingID, savingsID},
		mode:       transferDialogModeEdit,
		existing:   pair,
	}

	model, _ := app.Update(transferDialogDataMsg{data: data})
	updatedApp := model.(*App)

	if updatedApp.transferDialog == nil {
		t.Fatal("transferDialog should be set after edit-mode data msg")
	}
	if updatedApp.transferDialog.Title() != "Edit Transfer" {
		t.Errorf("title = %q, want %q", updatedApp.transferDialog.Title(), "Edit Transfer")
	}
	body := updatedApp.transferDialog.Message()
	if !strings.Contains(body, "Checking") || !strings.Contains(body, "Savings") {
		t.Errorf("message = %q, want to contain Checking and Savings", body)
	}

	fields := updatedApp.transferDialog.Fields()
	// bank↔bank edit carries a Category combo: Amount, Date, Memo, Category, Status.
	if len(fields) != 5 {
		t.Fatalf("expected 5 fields, got %d", len(fields))
	}
	if fields[0].Value != "300" {
		t.Errorf("Amount = %q, want %q", fields[0].Value, "300")
	}
	if fields[1].Value != "04/01/2024" {
		t.Errorf("Date = %q, want %q", fields[1].Value, "04/01/2024")
	}
	if fields[3].Label != "Category" {
		t.Errorf("field[3] label = %q, want Category", fields[3].Label)
	}
}

// =============================================================================
// P1-009: Investment-edit dispatch through the unified Transfer dialog
// =============================================================================

func TestStatusToRegular_Mapping(t *testing.T) {
	cases := []struct {
		in   investment.TransactionStatus
		want transaction.Status
	}{
		{investment.TransactionStatusPending, transaction.StatusUncleared},
		{investment.TransactionStatusCleared, transaction.StatusCleared},
		{investment.TransactionStatusReconciled, transaction.StatusReconciled},
	}
	for _, c := range cases {
		got := statusToRegular(c.in)
		if got != c.want {
			t.Errorf("statusToRegular(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestTransferAccountNames_InvestmentEdit verifies the read-only "From → To"
// banner shows the right account names when the edit payload is the
// investment shape (P1-009 dispatch path).
func TestTransferAccountNames_InvestmentEdit(t *testing.T) {
	iraA := types.NewID()
	iraB := types.NewID()
	data := &transferDialogData{
		accounts: []*account.Account{
			{BaseModel: types.BaseModel{ID: iraA}, Name: "IRA A", Type: account.TypeInvestment},
			{BaseModel: types.BaseModel{ID: iraB}, Name: "IRA B", Type: account.TypeInvestment},
		},
		existingInvestment: &investmentTransferEdit{
			fromAccountID: iraA,
			toAccountID:   iraB,
		},
	}
	from, to := transferAccountNames(data)
	if from != "IRA A" {
		t.Errorf("from = %q, want %q", from, "IRA A")
	}
	if to != "IRA B" {
		t.Errorf("to = %q, want %q", to, "IRA B")
	}
}

// TestApp_Update_TransferDialogDataMsg_InvestmentEdit verifies the edit-mode
// data message with an investmentTransferEdit payload opens the dialog with
// the right title, "From → To" banner, and pre-filled fields.
func TestApp_Update_TransferDialogDataMsg_InvestmentEdit(t *testing.T) {
	iraA := types.NewID()
	iraB := types.NewID()

	app := &App{
		currentView: ViewInvestmentRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	data := &transferDialogData{
		accounts: []*account.Account{
			{BaseModel: types.BaseModel{ID: iraA}, Name: "IRA A", Type: account.TypeInvestment},
			{BaseModel: types.BaseModel{ID: iraB}, Name: "IRA B", Type: account.TypeInvestment},
		},
		accountIDs: []types.ID{iraA, iraB},
		mode:       transferDialogModeEdit,
		existingInvestment: &investmentTransferEdit{
			fromAccountID:   iraA,
			toAccountID:     iraB,
			amount:          types.MustNewMoney("400.00"),
			date:            types.NewDate(2024, time.March, 15),
			memo:            "rollover",
			status:          transaction.StatusCleared,
			investmentTxnID: types.NewID(),
		},
	}

	model, _ := app.Update(transferDialogDataMsg{data: data})
	updated := model.(*App)

	if updated.transferDialog == nil {
		t.Fatal("dialog should be created")
	}
	if updated.transferDialog.Title() != "Edit Transfer" {
		t.Errorf("title = %q, want %q", updated.transferDialog.Title(), "Edit Transfer")
	}
	body := updated.transferDialog.Message()
	if !strings.Contains(body, "IRA A") || !strings.Contains(body, "IRA B") {
		t.Errorf("message = %q, want From → To with both IRA names", body)
	}
	fields := updated.transferDialog.Fields()
	if len(fields) != 4 {
		t.Fatalf("expected 4 fields, got %d", len(fields))
	}
	if fields[0].Value != "400" {
		t.Errorf("Amount = %q, want 400", fields[0].Value)
	}
	if fields[1].Value != "03/15/2024" {
		t.Errorf("Date = %q, want 03/15/2024", fields[1].Value)
	}
	if fields[2].Value != "rollover" {
		t.Errorf("Memo = %q, want rollover", fields[2].Value)
	}
	if fields[3].SelectedIndex != 1 {
		t.Errorf("Status selectedIndex = %d, want 1 (Cleared)", fields[3].SelectedIndex)
	}
}

// TestApp_SubmitEditTransferDialog_InvestmentEdit_Dispatches verifies the
// edit-mode submit path returns a non-nil command when the dialog data
// carries an investmentTransferEdit payload (the path through
// dispatchInvestmentEditTransfer). The actual UpdateTransferCash invocation
// fires inside the returned tea.Cmd; we only assert the dispatch closes the
// dialog and produces a cmd.
func TestApp_SubmitEditTransferDialog_InvestmentEdit_Dispatches(t *testing.T) {
	iraA := types.NewID()
	iraB := types.NewID()
	invTxnID := types.NewID()

	app := &App{
		currentView: ViewInvestmentRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("Edit Transfer")
			d.AddTextField("Amount", "400.00", "", 12)
			d.AddDateField("Date", "03/15/2024")
			d.AddTextField("Memo", "rollover", "", 0)
			d.AddRadioField("Status", []string{"Uncleared", "Cleared"}, 1)
			d.SetVisible(true)
			return d
		}(),
		transferDialogData: &transferDialogData{
			accounts: []*account.Account{
				{BaseModel: types.BaseModel{ID: iraA}, Name: "IRA A", Type: account.TypeInvestment},
				{BaseModel: types.BaseModel{ID: iraB}, Name: "IRA B", Type: account.TypeInvestment},
			},
			accountIDs: []types.ID{iraA, iraB},
			mode:       transferDialogModeEdit,
			existingInvestment: &investmentTransferEdit{
				fromAccountID:   iraA,
				toAccountID:     iraB,
				amount:          types.MustNewMoney("400.00"),
				date:            types.NewDate(2024, time.March, 15),
				memo:            "rollover",
				status:          transaction.StatusCleared,
				investmentTxnID: invTxnID,
			},
		},
		transferDialogAccountIDs: []types.ID{iraA, iraB},
	}

	model, cmd := app.submitTransferDialog()
	updated := model.(*App)

	if cmd == nil {
		t.Fatal("investment edit submit should return a non-nil cmd")
	}
	if updated.transferDialog != nil {
		t.Error("dialog should be closed after submit")
	}
}
