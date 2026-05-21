package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

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

	d := buildTransferDialog(options, 0)

	if d.Title() != "New Transfer" {
		t.Errorf("title = %q, want %q", d.Title(), "New Transfer")
	}

	if !d.IsVisible() {
		t.Error("dialog should be visible after creation")
	}

	fields := d.Fields()
	if len(fields) != 5 {
		t.Fatalf("expected 5 fields, got %d", len(fields))
	}
}

func TestBuildTransferDialog_FieldTypes(t *testing.T) {
	options := []string{"Checking", "Savings"}

	d := buildTransferDialog(options, 0)
	fields := d.Fields()

	expected := []struct {
		label     string
		fieldType FieldType
	}{
		{"From", FieldSelect},
		{"To", FieldSelect},
		{"Amount", FieldText},
		{"Date", FieldDate},
		{"Memo", FieldText},
	}

	for i, exp := range expected {
		if fields[i].Label != exp.label {
			t.Errorf("field[%d] label = %q, want %q", i, fields[i].Label, exp.label)
		}
		if fields[i].Type != exp.fieldType {
			t.Errorf("field[%d] type = %v, want %v", i, fields[i].Type, exp.fieldType)
		}
	}
}

func TestBuildTransferDialog_DefaultFromIndex(t *testing.T) {
	options := []string{"Checking", "Savings", "Credit Card"}

	d := buildTransferDialog(options, 1)
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

	d := buildTransferDialog(options, 0)
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

	d := buildTransferDialog(options, 0)
	fields := d.Fields()

	today := time.Now().Format("01/02/2006")
	if fields[3].Value != today {
		t.Errorf("date default = %q, want %q", fields[3].Value, today)
	}
}

// TestBuildTransferDialog_DateFieldOverwriteSemantics asserts that the Date
// field built by buildTransferDialog uses the FieldDate widget's overwrite
// semantics: typing two digits overwrites the month digits in place and the
// resulting Value is still a canonical 10-char MM/DD/YYYY string.
func TestBuildTransferDialog_DateFieldOverwriteSemantics(t *testing.T) {
	options := []string{"Checking", "Savings"}

	d := buildTransferDialog(options, 0)
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

	d := buildTransferDialog(options, 0)
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
		styles:      NewStyles(),
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		register: &registerData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: accountID},
				Name:      "Checking",
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
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
		menubar:                NewMenuBar(),
		statusbar:              NewStatusBar(),
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *Dialog {
			d := NewDialog("New Transfer")
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *Dialog {
			d := NewDialog("New Transfer")
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *Dialog {
			d := NewDialog("New Transfer")
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
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

func TestApp_SubmitTransferDialog_SameAccount(t *testing.T) {
	accountID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *Dialog {
			d := NewDialog("New Transfer")
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *Dialog {
			d := NewDialog("New Transfer")
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *Dialog {
			d := NewDialog("New Transfer")
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *Dialog {
			d := NewDialog("New Transfer")
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *Dialog {
			d := NewDialog("New Transfer")
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

	// Dialog should be closed
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
		transferDialog: func() *Dialog {
			d := NewDialog("New Transfer")
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
	styles := NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewRegister,
		width:       100,
		height:      30,
		ready:       true,
		styles:      styles,
		sidebar:     NewSidebar(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		keys:        defaultKeyMap(),
		register: &registerData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: types.NewID()},
				Name:      "Checking",
			},
			transactions:  []*transaction.Transaction{},
			balance:       &account.Balance{CurrentBalance: types.ZeroMoney},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
		transferDialog: func() *Dialog {
			d := NewDialog("New Transfer")
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		transferDialog: func() *Dialog {
			d := NewDialog("New Transfer")
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

	d := buildEditTransferDialog("Checking", "Savings", pair)

	if d.Title() != "Edit Transfer" {
		t.Errorf("title = %q, want %q", d.Title(), "Edit Transfer")
	}
	if !strings.Contains(d.Message(), "Checking") || !strings.Contains(d.Message(), "Savings") {
		t.Errorf("message = %q, want to contain From and To names", d.Message())
	}

	fields := d.Fields()
	if len(fields) != 4 {
		t.Fatalf("expected 4 editable fields (Amount, Date, Memo, Status), got %d", len(fields))
	}

	expected := []struct {
		label     string
		fieldType FieldType
	}{
		{"Amount", FieldText},
		{"Date", FieldDate},
		{"Memo", FieldText},
		{"Status", FieldRadio},
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
		t.Errorf("Status SelectedIndex = %d, want 1 (Cleared)", fields[3].SelectedIndex)
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
		styles:      NewStyles(),
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		register: &registerData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: accountID},
				Name:      "Checking",
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
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
	if len(fields) != 4 {
		t.Fatalf("expected 4 fields, got %d", len(fields))
	}
	if fields[0].Value != "300" {
		t.Errorf("Amount = %q, want %q", fields[0].Value, "300")
	}
	if fields[1].Value != "04/01/2024" {
		t.Errorf("Date = %q, want %q", fields[1].Value, "04/01/2024")
	}
}
