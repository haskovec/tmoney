package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haskovec/tmoney/internal/models"
)

// =============================================================================
// Pure Function Tests
// =============================================================================

func TestBuildAccountTypeOptions(t *testing.T) {
	options := buildAccountTypeOptions()

	allTypes := models.AllAccountTypes()
	if len(options) != len(allTypes) {
		t.Fatalf("expected %d options, got %d", len(allTypes), len(options))
	}

	for i, at := range allTypes {
		if options[i] != at.DisplayName() {
			t.Errorf("options[%d] = %q, want %q", i, options[i], at.DisplayName())
		}
	}
}

func TestAccountTypeFromIndex(t *testing.T) {
	tests := []struct {
		index    int
		expected models.AccountType
	}{
		{0, models.AccountTypeChecking},
		{1, models.AccountTypeSavings},
		{2, models.AccountTypeCreditCard},
		{3, models.AccountTypeInvestment},
		{4, models.AccountTypeCash},
		{5, models.AccountTypeLoan},
		{6, models.AccountTypeAsset},
		{-1, models.AccountTypeChecking},  // out of range defaults to checking
		{100, models.AccountTypeChecking}, // out of range defaults to checking
	}

	for _, tc := range tests {
		got := accountTypeFromIndex(tc.index)
		if got != tc.expected {
			t.Errorf("accountTypeFromIndex(%d) = %q, want %q", tc.index, got, tc.expected)
		}
	}
}

func TestAccountTypeToIndex(t *testing.T) {
	tests := []struct {
		accountType models.AccountType
		expected    int
	}{
		{models.AccountTypeChecking, 0},
		{models.AccountTypeSavings, 1},
		{models.AccountTypeCreditCard, 2},
		{models.AccountTypeInvestment, 3},
		{models.AccountTypeCash, 4},
		{models.AccountTypeLoan, 5},
		{models.AccountTypeAsset, 6},
		{models.AccountType("unknown"), 0}, // unknown defaults to 0
	}

	for _, tc := range tests {
		got := accountTypeToIndex(tc.accountType)
		if got != tc.expected {
			t.Errorf("accountTypeToIndex(%q) = %d, want %d", tc.accountType, got, tc.expected)
		}
	}
}

func TestAccountTypeRoundTrip(t *testing.T) {
	for i, at := range models.AllAccountTypes() {
		idx := accountTypeToIndex(at)
		if idx != i {
			t.Errorf("accountTypeToIndex(%q) = %d, want %d", at, idx, i)
		}
		back := accountTypeFromIndex(idx)
		if back != at {
			t.Errorf("accountTypeFromIndex(%d) = %q, want %q", idx, back, at)
		}
	}
}

func TestBuildNewAccountDialog(t *testing.T) {
	d := buildNewAccountDialog()

	if d.Title() != "New Account" {
		t.Errorf("title = %q, want %q", d.Title(), "New Account")
	}

	if !d.IsVisible() {
		t.Error("dialog should be visible after creation")
	}

	fields := d.Fields()
	if len(fields) != 10 {
		t.Fatalf("expected 10 fields, got %d", len(fields))
	}
}

func TestBuildNewAccountDialog_FieldTypes(t *testing.T) {
	d := buildNewAccountDialog()
	fields := d.Fields()

	expected := []struct {
		label     string
		fieldType FieldType
	}{
		{"Name", FieldText},
		{"Type", FieldSelect},
		{"Currency", FieldText},
		{"Opening Balance", FieldText},
		{"Opening Date", FieldText},
		{"Institution", FieldText},
		{"Account #", FieldText},
		{"Notes", FieldText},
		{"Credit Limit", FieldText},
		{"Interest Rate", FieldText},
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

func TestBuildNewAccountDialog_Defaults(t *testing.T) {
	d := buildNewAccountDialog()
	fields := d.Fields()

	// Currency should default to USD
	if fields[acctFieldCurrency].Value != "USD" {
		t.Errorf("currency default = %q, want %q", fields[acctFieldCurrency].Value, "USD")
	}

	// Opening balance should default to 0.00
	if fields[acctFieldOpeningBalance].Value != "0.00" {
		t.Errorf("opening balance default = %q, want %q", fields[acctFieldOpeningBalance].Value, "0.00")
	}

	// Opening date should default to today
	today := time.Now().Format("01/02/2006")
	if fields[acctFieldOpeningDate].Value != today {
		t.Errorf("opening date default = %q, want %q", fields[acctFieldOpeningDate].Value, today)
	}

	// Account type should default to Checking (index 0)
	if fields[acctFieldType].SelectedIndex != 0 {
		t.Errorf("type selectedIndex = %d, want 0", fields[acctFieldType].SelectedIndex)
	}
}

func TestBuildEditAccountDialog(t *testing.T) {
	account := models.NewAccount(
		"My Checking",
		models.AccountTypeChecking,
		"USD",
		models.ZeroMoney,
		models.Today(),
	)
	account.SetInstitution("First Bank")
	account.SetAccountNumber("1234")
	account.SetNotes("Main account")

	d := buildEditAccountDialog(account)

	if d.Title() != "Edit Account" {
		t.Errorf("title = %q, want %q", d.Title(), "Edit Account")
	}

	if !d.IsVisible() {
		t.Error("dialog should be visible after creation")
	}

	fields := d.Fields()
	if len(fields) != 10 {
		t.Fatalf("expected 10 fields, got %d", len(fields))
	}

	// Name should be pre-filled
	if fields[acctFieldName].Value != "My Checking" {
		t.Errorf("name = %q, want %q", fields[acctFieldName].Value, "My Checking")
	}

	// Type should be Checking (index 0)
	if fields[acctFieldType].SelectedIndex != 0 {
		t.Errorf("type selectedIndex = %d, want 0", fields[acctFieldType].SelectedIndex)
	}

	// Currency
	if fields[acctFieldCurrency].Value != "USD" {
		t.Errorf("currency = %q, want %q", fields[acctFieldCurrency].Value, "USD")
	}

	// Institution
	if fields[acctFieldInstitution].Value != "First Bank" {
		t.Errorf("institution = %q, want %q", fields[acctFieldInstitution].Value, "First Bank")
	}

	// Account number
	if fields[acctFieldAccountNumber].Value != "1234" {
		t.Errorf("account number = %q, want %q", fields[acctFieldAccountNumber].Value, "1234")
	}

	// Notes
	if fields[acctFieldNotes].Value != "Main account" {
		t.Errorf("notes = %q, want %q", fields[acctFieldNotes].Value, "Main account")
	}
}

func TestBuildEditAccountDialog_CreditCard(t *testing.T) {
	creditLimit, _ := models.NewMoney("5000.00")
	account := models.NewAccount(
		"Visa",
		models.AccountTypeCreditCard,
		"USD",
		models.ZeroMoney,
		models.Today(),
	)
	account.SetCreditLimit(creditLimit)

	d := buildEditAccountDialog(account)
	fields := d.Fields()

	// Type should be Credit Card (index 2)
	if fields[acctFieldType].SelectedIndex != 2 {
		t.Errorf("type selectedIndex = %d, want 2", fields[acctFieldType].SelectedIndex)
	}

	// Credit limit should be populated
	if fields[acctFieldCreditLimit].Value != "5000.00" {
		t.Errorf("credit limit = %q, want %q", fields[acctFieldCreditLimit].Value, "5000.00")
	}
}

func TestBuildEditAccountDialog_Loan(t *testing.T) {
	rate, _ := models.NewMoney("4.50")
	account := models.NewAccount(
		"Mortgage",
		models.AccountTypeLoan,
		"USD",
		models.ZeroMoney,
		models.Today(),
	)
	account.SetInterestRate(rate)

	d := buildEditAccountDialog(account)
	fields := d.Fields()

	// Type should be Loan (index 5)
	if fields[acctFieldType].SelectedIndex != 5 {
		t.Errorf("type selectedIndex = %d, want 5", fields[acctFieldType].SelectedIndex)
	}

	// Interest rate should be populated
	if fields[acctFieldInterestRate].Value != "4.50" {
		t.Errorf("interest rate = %q, want %q", fields[acctFieldInterestRate].Value, "4.50")
	}
}

// =============================================================================
// App Integration Tests (no database)
// =============================================================================

func TestApp_Update_AccountDialogDataMsg_New(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	msg := accountDialogDataMsg{
		data: &accountDialogData{mode: accountDialogModeNew},
	}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.acctDialog == nil {
		t.Fatal("account dialog should be created")
	}
	if !updatedApp.acctDialog.IsVisible() {
		t.Error("account dialog should be visible")
	}
	if updatedApp.acctDialog.Title() != "New Account" {
		t.Errorf("title = %q, want %q", updatedApp.acctDialog.Title(), "New Account")
	}
	if updatedApp.acctDialogData == nil {
		t.Error("account dialog data should be set")
	}
}

func TestApp_Update_AccountDialogDataMsg_Edit(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	account := models.NewAccount("Savings", models.AccountTypeSavings, "EUR", models.ZeroMoney, models.Today())

	msg := accountDialogDataMsg{
		data: &accountDialogData{
			mode:    accountDialogModeEdit,
			account: account,
		},
	}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.acctDialog == nil {
		t.Fatal("account dialog should be created")
	}
	if updatedApp.acctDialog.Title() != "Edit Account" {
		t.Errorf("title = %q, want %q", updatedApp.acctDialog.Title(), "Edit Account")
	}

	// Verify the name is pre-filled
	fields := updatedApp.acctDialog.Fields()
	if fields[acctFieldName].Value != "Savings" {
		t.Errorf("name = %q, want %q", fields[acctFieldName].Value, "Savings")
	}
}

func TestApp_HandleAccountDialogKey_Cancel(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *Dialog {
			d := buildNewAccountDialog()
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	escKey := tea.KeyMsg{Type: tea.KeyEsc}
	model, _ := app.Update(escKey)
	updatedApp := model.(*App)

	if updatedApp.acctDialog != nil {
		t.Error("account dialog should be nil after cancel")
	}
	if updatedApp.acctDialogData != nil {
		t.Error("account dialog data should be nil after cancel")
	}
}

func TestApp_HandleAccountDialogKey_TabCycles(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *Dialog {
			d := buildNewAccountDialog()
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	initialFocus := app.acctDialog.FocusIndex()
	if initialFocus != 0 {
		t.Fatalf("initial focus = %d, want 0", initialFocus)
	}

	tabKey := tea.KeyMsg{Type: tea.KeyTab}
	model, _ := app.Update(tabKey)
	updatedApp := model.(*App)

	if updatedApp.acctDialog.FocusIndex() != 1 {
		t.Errorf("focus after Tab = %d, want 1", updatedApp.acctDialog.FocusIndex())
	}
}

func TestApp_SubmitAccountDialog_EmptyName(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *Dialog {
			d := buildNewAccountDialog()
			// Clear the name field
			d.Fields()[acctFieldName].Value = ""
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	model, cmd := app.submitAccountDialog()
	updatedApp := model.(*App)

	if cmd != nil {
		t.Error("empty name should not return a cmd")
	}
	if updatedApp.err == nil {
		t.Error("empty name should set an error")
	}
	if updatedApp.err != nil && !strings.Contains(updatedApp.err.Error(), "name") {
		t.Errorf("error = %q, should mention name", updatedApp.err.Error())
	}
}

func TestApp_SubmitAccountDialog_EmptyCurrency(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *Dialog {
			d := buildNewAccountDialog()
			d.Fields()[acctFieldName].Value = "Test Account"
			d.Fields()[acctFieldCurrency].Value = ""
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	model, cmd := app.submitAccountDialog()
	updatedApp := model.(*App)

	if cmd != nil {
		t.Error("empty currency should not return a cmd")
	}
	if updatedApp.err == nil {
		t.Error("empty currency should set an error")
	}
	if updatedApp.err != nil && !strings.Contains(updatedApp.err.Error(), "currency") {
		t.Errorf("error = %q, should mention currency", updatedApp.err.Error())
	}
}

func TestApp_SubmitAccountDialog_InvalidOpeningBalance(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *Dialog {
			d := buildNewAccountDialog()
			d.Fields()[acctFieldName].Value = "Test Account"
			d.Fields()[acctFieldOpeningBalance].Value = "not-a-number"
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	model, cmd := app.submitAccountDialog()
	updatedApp := model.(*App)

	if cmd != nil {
		t.Error("invalid balance should not return a cmd")
	}
	if updatedApp.err == nil {
		t.Error("invalid balance should set an error")
	}
	if updatedApp.err != nil && !strings.Contains(updatedApp.err.Error(), "opening balance") {
		t.Errorf("error = %q, should mention opening balance", updatedApp.err.Error())
	}
}

func TestApp_SubmitAccountDialog_InvalidDate(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *Dialog {
			d := buildNewAccountDialog()
			d.Fields()[acctFieldName].Value = "Test Account"
			d.Fields()[acctFieldOpeningDate].Value = "not-a-date"
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	model, cmd := app.submitAccountDialog()
	updatedApp := model.(*App)

	if cmd != nil {
		t.Error("invalid date should not return a cmd")
	}
	if updatedApp.err == nil {
		t.Error("invalid date should set an error")
	}
	if updatedApp.err != nil && !strings.Contains(updatedApp.err.Error(), "opening date") {
		t.Errorf("error = %q, should mention opening date", updatedApp.err.Error())
	}
}

func TestApp_SubmitAccountDialog_InvalidCreditLimit(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *Dialog {
			d := buildNewAccountDialog()
			d.Fields()[acctFieldName].Value = "Test Card"
			d.Fields()[acctFieldCreditLimit].Value = "abc"
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	model, cmd := app.submitAccountDialog()
	updatedApp := model.(*App)

	if cmd != nil {
		t.Error("invalid credit limit should not return a cmd")
	}
	if updatedApp.err == nil {
		t.Error("invalid credit limit should set an error")
	}
	if updatedApp.err != nil && !strings.Contains(updatedApp.err.Error(), "credit limit") {
		t.Errorf("error = %q, should mention credit limit", updatedApp.err.Error())
	}
}

func TestApp_SubmitAccountDialog_InvalidInterestRate(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *Dialog {
			d := buildNewAccountDialog()
			d.Fields()[acctFieldName].Value = "Test Loan"
			d.Fields()[acctFieldInterestRate].Value = "xyz"
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	model, cmd := app.submitAccountDialog()
	updatedApp := model.(*App)

	if cmd != nil {
		t.Error("invalid interest rate should not return a cmd")
	}
	if updatedApp.err == nil {
		t.Error("invalid interest rate should set an error")
	}
	if updatedApp.err != nil && !strings.Contains(updatedApp.err.Error(), "interest rate") {
		t.Errorf("error = %q, should mention interest rate", updatedApp.err.Error())
	}
}

func TestApp_SubmitAccountDialog_ValidNew(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *Dialog {
			d := buildNewAccountDialog()
			d.Fields()[acctFieldName].Value = "My Checking"
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	model, cmd := app.submitAccountDialog()
	updatedApp := model.(*App)

	if cmd == nil {
		t.Error("valid new account should return a non-nil cmd")
	}
	if updatedApp.acctDialog != nil {
		t.Error("dialog should be closed after submit")
	}
	if updatedApp.acctDialogData != nil {
		t.Error("dialog data should be nil after submit")
	}
	if updatedApp.err != nil {
		t.Errorf("unexpected error: %v", updatedApp.err)
	}
}

func TestApp_SubmitAccountDialog_ValidEdit(t *testing.T) {
	existing := models.NewAccount("Old Name", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())

	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *Dialog {
			d := buildEditAccountDialog(existing)
			d.Fields()[acctFieldName].Value = "New Name"
			return d
		}(),
		acctDialogData: &accountDialogData{
			mode:    accountDialogModeEdit,
			account: existing,
		},
	}

	model, cmd := app.submitAccountDialog()
	updatedApp := model.(*App)

	if cmd == nil {
		t.Error("valid edit should return a non-nil cmd")
	}
	if updatedApp.acctDialog != nil {
		t.Error("dialog should be closed after submit")
	}
	if updatedApp.err != nil {
		t.Errorf("unexpected error: %v", updatedApp.err)
	}
}

func TestApp_CloseAccountDialog(t *testing.T) {
	app := &App{
		acctDialog: func() *Dialog {
			d := NewDialog("New Account")
			d.SetVisible(true)
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	app.closeAccountDialog()

	if app.acctDialog != nil {
		t.Error("dialog should be nil after close")
	}
	if app.acctDialogData != nil {
		t.Error("dialog data should be nil after close")
	}
}

func TestApp_Update_AccountDialogSavedMsg(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	msg := accountDialogSavedMsg{}
	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Error("accountDialogSavedMsg should return a reload command")
	}
}

func TestApp_Update_AccountDeletedMsg(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	msg := accountDeletedMsg{}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	if cmd == nil {
		t.Error("accountDeletedMsg should return a reload command")
	}
	if updatedApp.currentView != ViewDashboard {
		t.Errorf("view after delete = %v, want %v", updatedApp.currentView, ViewDashboard)
	}
}

func TestApp_Update_AccountClosedMsg(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	msg := accountClosedMsg{}
	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Error("accountClosedMsg should return a reload command")
	}
}

func TestApp_RenderLayout_WithAccountDialog(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewDashboard,
		width:       100,
		height:      30,
		ready:       true,
		styles:      styles,
		sidebar:     NewSidebar(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		keys:        defaultKeyMap(),
		acctDialog: func() *Dialog {
			d := buildNewAccountDialog()
			return d
		}(),
	}

	output := app.renderLayout()
	if !strings.Contains(output, "New Account") {
		t.Error("renderLayout() should contain 'New Account' when account dialog is visible")
	}
}

func TestApp_HandleMenuAction_NewAccount(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	_, cmd := app.handleMenuAction(MenuActionNewAccount)

	if cmd == nil {
		t.Error("MenuActionNewAccount should return a non-nil cmd")
	}
}

func TestApp_HandleMenuAction_EditAccount_NoSelection(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	_, cmd := app.handleMenuAction(MenuActionEditAccount)

	// No account selected, should not return a cmd
	if cmd != nil {
		t.Error("MenuActionEditAccount with no selection should return nil cmd")
	}
}

func TestApp_HandleMenuAction_EditAccount_WithSelection(t *testing.T) {
	accountID := models.NewID()
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	app.sidebar.SetAccounts([]*models.Account{
		{BaseModel: models.BaseModel{ID: accountID}, Name: "Checking", Active: true, Type: models.AccountTypeChecking},
	}, nil)
	app.sidebar.MoveDown() // move to account
	app.sidebar.Select()

	_, cmd := app.handleMenuAction(MenuActionEditAccount)

	if cmd == nil {
		t.Error("MenuActionEditAccount with selection should return a non-nil cmd")
	}
}
