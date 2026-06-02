package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// Pure Function Tests
// =============================================================================

func TestBuildAccountTypeOptions(t *testing.T) {
	options := buildAccountTypeOptions()

	allTypes := account.AllTypes()
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
		expected account.Type
	}{
		{0, account.TypeChecking},
		{1, account.TypeSavings},
		{2, account.TypeCreditCard},
		{3, account.TypeInvestment},
		{4, account.TypeHSA},
		{5, account.TypeCash},
		{6, account.TypeLoan},
		{7, account.TypeAsset},
		{-1, account.TypeChecking},  // out of range defaults to checking
		{100, account.TypeChecking}, // out of range defaults to checking
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
		accountType account.Type
		expected    int
	}{
		{account.TypeChecking, 0},
		{account.TypeSavings, 1},
		{account.TypeCreditCard, 2},
		{account.TypeInvestment, 3},
		{account.TypeHSA, 4},
		{account.TypeCash, 5},
		{account.TypeLoan, 6},
		{account.TypeAsset, 7},
		{account.Type("unknown"), 0}, // unknown defaults to 0
	}

	for _, tc := range tests {
		got := accountTypeToIndex(tc.accountType)
		if got != tc.expected {
			t.Errorf("accountTypeToIndex(%q) = %d, want %d", tc.accountType, got, tc.expected)
		}
	}
}

func TestAccountTypeRoundTrip(t *testing.T) {
	for i, at := range account.AllTypes() {
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
	if len(fields) != 11 {
		t.Fatalf("expected 11 fields, got %d", len(fields))
	}

	// Default type is Checking: credit limit hidden, interest rate visible
	if !fields[acctFieldCreditLimit].Hidden {
		t.Error("credit limit should be hidden for Checking")
	}
	if fields[acctFieldInterestRate].Hidden {
		t.Error("interest rate should be visible for Checking")
	}
}

func TestBuildNewAccountDialog_FieldTypes(t *testing.T) {
	d := buildNewAccountDialog()
	fields := d.Fields()

	expected := []struct {
		label     string
		fieldType dialog.FieldType
	}{
		{"Name", dialog.FieldText},
		{"Type", dialog.FieldSelect},
		{"Currency", dialog.FieldText},
		{"Opening Balance", dialog.FieldText},
		{"Opening Date", dialog.FieldDate},
		{"Institution", dialog.FieldText},
		{"Account #", dialog.FieldText},
		{"Notes", dialog.FieldText},
		{"Credit Limit", dialog.FieldText},
		{"Interest Rate", dialog.FieldText},
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

// TestBuildNewAccountDialog_OpeningDateMaskedOverwrite verifies the new
// account dialog's Opening Date field uses overwrite-style masked input —
// typing a digit replaces the digit at the cursor and auto-advances over
// the slash.
func TestBuildNewAccountDialog_OpeningDateMaskedOverwrite(t *testing.T) {
	d := buildNewAccountDialog()
	d.SetFocusIndex(acctFieldOpeningDate)

	// Seed Value to a known date so the overwrite is deterministic.
	d.Fields()[acctFieldOpeningDate].Value = "03/15/2024"

	// Type "0" then "5" — overwrites "03" with "05", cursor advances skipping the slash.
	d.HandleKey(tea.KeyPressMsg{Code: '0', Text: "0"})
	d.HandleKey(tea.KeyPressMsg{Code: '5', Text: "5"})

	if got := d.Fields()[acctFieldOpeningDate].Value; got != "05/15/2024" {
		t.Errorf("Value = %q, want %q (overwrite + skip slash)", got, "05/15/2024")
	}
}

func TestBuildEditAccountDialog(t *testing.T) {
	acct := account.NewAccount(
		"My Checking",
		account.TypeChecking,
		"USD",
		types.ZeroMoney,
		types.Today(),
	)
	acct.SetInstitution("First Bank")
	acct.SetAccountNumber("1234")
	acct.SetNotes("Main account")

	d := buildEditAccountDialog(acct)

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

func TestBuildEditAccountDialog_AlphanumericAccountNumber(t *testing.T) {
	acct := account.NewAccount(
		"Brokerage",
		account.TypeInvestment,
		"USD",
		types.ZeroMoney,
		types.Today(),
	)
	acct.SetInstitution("Fidelity")
	acct.SetAccountNumber("Z12-345ABC")

	d := buildEditAccountDialog(acct)
	fields := d.Fields()

	if fields[acctFieldAccountNumber].Value != "Z12-345ABC" {
		t.Errorf("account number = %q, want %q", fields[acctFieldAccountNumber].Value, "Z12-345ABC")
	}
}

func TestBuildEditAccountDialog_CreditCard(t *testing.T) {
	creditLimit, _ := types.NewMoney("5000.00")
	acct := account.NewAccount(
		"Visa",
		account.TypeCreditCard,
		"USD",
		types.ZeroMoney,
		types.Today(),
	)
	acct.SetCreditLimit(creditLimit)

	d := buildEditAccountDialog(acct)
	fields := d.Fields()

	// Type should be Credit Card (index 2)
	if fields[acctFieldType].SelectedIndex != 2 {
		t.Errorf("type selectedIndex = %d, want 2", fields[acctFieldType].SelectedIndex)
	}

	// Credit limit should be populated and visible
	if fields[acctFieldCreditLimit].Value != "5000.00" {
		t.Errorf("credit limit = %q, want %q", fields[acctFieldCreditLimit].Value, "5000.00")
	}
	if fields[acctFieldCreditLimit].Hidden {
		t.Error("credit limit should be visible for Credit Card")
	}

	// Interest rate should also be visible for credit cards
	if fields[acctFieldInterestRate].Hidden {
		t.Error("interest rate should be visible for Credit Card")
	}
}

func TestBuildEditAccountDialog_Loan(t *testing.T) {
	rate, _ := types.NewMoney("4.50")
	acct := account.NewAccount(
		"Mortgage",
		account.TypeLoan,
		"USD",
		types.ZeroMoney,
		types.Today(),
	)
	acct.SetInterestRate(rate)

	d := buildEditAccountDialog(acct)
	fields := d.Fields()

	// Type should be Loan (index 6)
	if fields[acctFieldType].SelectedIndex != 6 {
		t.Errorf("type selectedIndex = %d, want 6", fields[acctFieldType].SelectedIndex)
	}

	// Interest rate should be populated and visible
	if fields[acctFieldInterestRate].Value != "4.50" {
		t.Errorf("interest rate = %q, want %q", fields[acctFieldInterestRate].Value, "4.50")
	}
	if fields[acctFieldInterestRate].Hidden {
		t.Error("interest rate should be visible for Loan")
	}

	// Credit limit should be hidden for loans
	if !fields[acctFieldCreditLimit].Hidden {
		t.Error("credit limit should be hidden for Loan")
	}
}

// =============================================================================
// App Integration Tests (no database)
// =============================================================================

func TestApp_Update_AccountDialogDataMsg_New(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	acct := account.NewAccount("Savings", account.TypeSavings, "EUR", types.ZeroMoney, types.Today())

	msg := accountDialogDataMsg{
		data: &accountDialogData{
			mode:    accountDialogModeEdit,
			account: acct,
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *dialog.Dialog {
			d := buildNewAccountDialog()
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	escKey := tea.KeyPressMsg{Code: tea.KeyEsc}
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *dialog.Dialog {
			d := buildNewAccountDialog()
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	initialFocus := app.acctDialog.FocusIndex()
	if initialFocus != 0 {
		t.Fatalf("initial focus = %d, want 0", initialFocus)
	}

	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *dialog.Dialog {
			d := buildNewAccountDialog()
			// Clear the name field
			d.Fields()[acctFieldName].Value = ""
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	_, cmd := app.submitAccountDialog()

	if cmd != nil {
		t.Error("empty name should not return a cmd")
	}
	if app.acctDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.acctDialog.Fields()[acctFieldName].Error == "" {
		t.Error("empty name should set field-level error")
	}
}

func TestApp_SubmitAccountDialog_EmptyCurrency(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *dialog.Dialog {
			d := buildNewAccountDialog()
			d.Fields()[acctFieldName].Value = "Test Account"
			d.Fields()[acctFieldCurrency].Value = ""
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	_, cmd := app.submitAccountDialog()

	if cmd != nil {
		t.Error("empty currency should not return a cmd")
	}
	if app.acctDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.acctDialog.Fields()[acctFieldCurrency].Error == "" {
		t.Error("empty currency should set field-level error")
	}
}

func TestApp_SubmitAccountDialog_InvalidOpeningBalance(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *dialog.Dialog {
			d := buildNewAccountDialog()
			d.Fields()[acctFieldName].Value = "Test Account"
			d.Fields()[acctFieldOpeningBalance].Value = "not-a-number"
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	_, cmd := app.submitAccountDialog()

	if cmd != nil {
		t.Error("invalid balance should not return a cmd")
	}
	if app.acctDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.acctDialog.Fields()[acctFieldOpeningBalance].Error == "" {
		t.Error("invalid balance should set field-level error")
	}
}

func TestApp_SubmitAccountDialog_InvalidDate(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *dialog.Dialog {
			d := buildNewAccountDialog()
			d.Fields()[acctFieldName].Value = "Test Account"
			d.Fields()[acctFieldOpeningDate].Value = "not-a-date"
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	_, cmd := app.submitAccountDialog()

	if cmd != nil {
		t.Error("invalid date should not return a cmd")
	}
	if app.acctDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.acctDialog.Fields()[acctFieldOpeningDate].Error == "" {
		t.Error("invalid date should set field-level error")
	}
}

func TestApp_SubmitAccountDialog_InvalidCreditLimit(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *dialog.Dialog {
			d := buildNewAccountDialog()
			d.Fields()[acctFieldName].Value = "Test Card"
			// Set type to Credit Card so credit limit field is visible
			d.Fields()[acctFieldType].SelectedIndex = accountTypeToIndex(account.TypeCreditCard)
			updateAccountFieldVisibility(d)
			d.Fields()[acctFieldCreditLimit].Value = "abc"
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	_, cmd := app.submitAccountDialog()

	if cmd != nil {
		t.Error("invalid credit limit should not return a cmd")
	}
	if app.acctDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.acctDialog.Fields()[acctFieldCreditLimit].Error == "" {
		t.Error("invalid credit limit should set field-level error")
	}
}

func TestApp_SubmitAccountDialog_InvalidInterestRate(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *dialog.Dialog {
			d := buildNewAccountDialog()
			d.Fields()[acctFieldName].Value = "Test Loan"
			// Default type is Checking which shows interest rate
			d.Fields()[acctFieldInterestRate].Value = "xyz"
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	_, cmd := app.submitAccountDialog()

	if cmd != nil {
		t.Error("invalid interest rate should not return a cmd")
	}
	if app.acctDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.acctDialog.Fields()[acctFieldInterestRate].Error == "" {
		t.Error("invalid interest rate should set field-level error")
	}
}

func TestApp_SubmitAccountDialog_MultipleErrors(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *dialog.Dialog {
			d := buildNewAccountDialog()
			d.Fields()[acctFieldName].Value = ""
			d.Fields()[acctFieldCurrency].Value = ""
			d.Fields()[acctFieldOpeningBalance].Value = "bad"
			d.Fields()[acctFieldOpeningDate].Value = "bad"
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	_, cmd := app.submitAccountDialog()

	if cmd != nil {
		t.Error("should not return a cmd with multiple errors")
	}
	if app.acctDialog == nil {
		t.Fatal("dialog should remain open")
	}

	fields := app.acctDialog.Fields()
	if fields[acctFieldName].Error == "" {
		t.Error("name field should have error")
	}
	if fields[acctFieldCurrency].Error == "" {
		t.Error("currency field should have error")
	}
	if fields[acctFieldOpeningBalance].Error == "" {
		t.Error("opening balance field should have error")
	}
	if fields[acctFieldOpeningDate].Error == "" {
		t.Error("opening date field should have error")
	}
}

func TestBuildNewAccountDialog_RequiredFields(t *testing.T) {
	d := buildNewAccountDialog()
	fields := d.Fields()

	if !fields[acctFieldName].Required {
		t.Error("Name field should be required")
	}
	if !fields[acctFieldCurrency].Required {
		t.Error("Currency field should be required")
	}
	if !fields[acctFieldOpeningBalance].Required {
		t.Error("Opening Balance field should be required")
	}
	if !fields[acctFieldOpeningDate].Required {
		t.Error("Opening Date field should be required")
	}
	if fields[acctFieldInstitution].Required {
		t.Error("Institution field should not be required")
	}
}

func TestApp_SubmitAccountDialog_ValidNew(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *dialog.Dialog {
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
	existing := account.NewAccount("Old Name", account.TypeChecking, "USD", types.ZeroMoney, types.Today())

	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *dialog.Dialog {
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
		acctDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Account")
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	msg := accountClosedMsg{}
	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Error("accountClosedMsg should return a reload command")
	}
}

func TestApp_RenderLayout_WithAccountDialog(t *testing.T) {
	styles := widget.NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewDashboard,
		width:       100,
		height:      30,
		ready:       true,
		styles:      styles,
		sidebar:     NewSidebar(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		keys:        defaultKeyMap(),
		acctDialog: func() *dialog.Dialog {
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	_, cmd := app.handleMenuAction(widget.MenuActionNewAccount, "")

	if cmd == nil {
		t.Error("widget.MenuActionNewAccount should return a non-nil cmd")
	}
}

func TestApp_HandleMenuAction_EditAccount_NoSelection(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	_, cmd := app.handleMenuAction(widget.MenuActionEditAccount, "")

	// No account selected, should not return a cmd
	if cmd != nil {
		t.Error("widget.MenuActionEditAccount with no selection should return nil cmd")
	}
}

func TestApp_HandleMenuAction_EditAccount_WithSelection(t *testing.T) {
	accountID := types.NewID()
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	app.sidebar.SetAccounts([]*account.Account{
		{BaseModel: types.BaseModel{ID: accountID}, Name: "Checking", Active: true, Type: account.TypeChecking},
	}, nil)
	app.sidebar.MoveDown() // move to account
	app.sidebar.Select()

	_, cmd := app.handleMenuAction(widget.MenuActionEditAccount, "")

	if cmd == nil {
		t.Error("widget.MenuActionEditAccount with selection should return a non-nil cmd")
	}
}

// =============================================================================
// Dynamic dialog.Field Visibility Tests
// =============================================================================

func TestAccountTypeShowsCreditLimit(t *testing.T) {
	tests := []struct {
		accountType account.Type
		expected    bool
	}{
		{account.TypeChecking, false},
		{account.TypeSavings, false},
		{account.TypeCreditCard, true},
		{account.TypeInvestment, false},
		{account.TypeCash, false},
		{account.TypeLoan, false},
		{account.TypeAsset, false},
	}

	for _, tc := range tests {
		got := accountTypeShowsCreditLimit(tc.accountType)
		if got != tc.expected {
			t.Errorf("accountTypeShowsCreditLimit(%q) = %v, want %v", tc.accountType, got, tc.expected)
		}
	}
}

func TestAccountTypeShowsInterestRate(t *testing.T) {
	tests := []struct {
		accountType account.Type
		expected    bool
	}{
		{account.TypeChecking, true},
		{account.TypeSavings, true},
		{account.TypeCreditCard, true},
		{account.TypeInvestment, true},
		{account.TypeCash, false},
		{account.TypeLoan, true},
		{account.TypeAsset, false},
	}

	for _, tc := range tests {
		got := accountTypeShowsInterestRate(tc.accountType)
		if got != tc.expected {
			t.Errorf("accountTypeShowsInterestRate(%q) = %v, want %v", tc.accountType, got, tc.expected)
		}
	}
}

func TestNewAccountDialog_FieldVisibility_Checking(t *testing.T) {
	d := buildNewAccountDialog()
	// Default type is Checking (index 0)
	fields := d.Fields()

	if !fields[acctFieldCreditLimit].Hidden {
		t.Error("credit limit should be hidden for Checking")
	}
	if fields[acctFieldInterestRate].Hidden {
		t.Error("interest rate should be visible for Checking")
	}
}

func TestNewAccountDialog_FieldVisibility_CreditCard(t *testing.T) {
	d := buildNewAccountDialog()
	fields := d.Fields()

	// Change type to Credit Card
	fields[acctFieldType].SelectedIndex = accountTypeToIndex(account.TypeCreditCard)
	updateAccountFieldVisibility(d)

	if fields[acctFieldCreditLimit].Hidden {
		t.Error("credit limit should be visible for Credit Card")
	}
	if fields[acctFieldInterestRate].Hidden {
		t.Error("interest rate should be visible for Credit Card")
	}
}

func TestNewAccountDialog_FieldVisibility_Cash(t *testing.T) {
	d := buildNewAccountDialog()
	fields := d.Fields()

	// Change type to Cash
	fields[acctFieldType].SelectedIndex = accountTypeToIndex(account.TypeCash)
	updateAccountFieldVisibility(d)

	if !fields[acctFieldCreditLimit].Hidden {
		t.Error("credit limit should be hidden for Cash")
	}
	if !fields[acctFieldInterestRate].Hidden {
		t.Error("interest rate should be hidden for Cash")
	}
}

func TestNewAccountDialog_FieldVisibility_Asset(t *testing.T) {
	d := buildNewAccountDialog()
	fields := d.Fields()

	// Change type to Asset
	fields[acctFieldType].SelectedIndex = accountTypeToIndex(account.TypeAsset)
	updateAccountFieldVisibility(d)

	if !fields[acctFieldCreditLimit].Hidden {
		t.Error("credit limit should be hidden for Asset")
	}
	if !fields[acctFieldInterestRate].Hidden {
		t.Error("interest rate should be hidden for Asset")
	}
}

func TestNewAccountDialog_FieldVisibility_Savings(t *testing.T) {
	d := buildNewAccountDialog()
	fields := d.Fields()

	// Change type to Savings
	fields[acctFieldType].SelectedIndex = accountTypeToIndex(account.TypeSavings)
	updateAccountFieldVisibility(d)

	if !fields[acctFieldCreditLimit].Hidden {
		t.Error("credit limit should be hidden for Savings")
	}
	if fields[acctFieldInterestRate].Hidden {
		t.Error("interest rate should be visible for Savings")
	}
}

func TestNewAccountDialog_FieldVisibility_Investment(t *testing.T) {
	d := buildNewAccountDialog()
	fields := d.Fields()

	// Change type to Investment
	fields[acctFieldType].SelectedIndex = accountTypeToIndex(account.TypeInvestment)
	updateAccountFieldVisibility(d)

	if !fields[acctFieldCreditLimit].Hidden {
		t.Error("credit limit should be hidden for Investment")
	}
	if fields[acctFieldInterestRate].Hidden {
		t.Error("interest rate should be visible for Investment")
	}
}

func TestNewAccountDialog_FieldVisibility_Loan(t *testing.T) {
	d := buildNewAccountDialog()
	fields := d.Fields()

	// Change type to Loan
	fields[acctFieldType].SelectedIndex = accountTypeToIndex(account.TypeLoan)
	updateAccountFieldVisibility(d)

	if !fields[acctFieldCreditLimit].Hidden {
		t.Error("credit limit should be hidden for Loan")
	}
	if fields[acctFieldInterestRate].Hidden {
		t.Error("interest rate should be visible for Loan")
	}
}

func TestUpdateAccountFieldVisibility_ClearsHiddenValues(t *testing.T) {
	d := buildNewAccountDialog()
	fields := d.Fields()

	// Set type to Credit Card and fill in credit limit
	fields[acctFieldType].SelectedIndex = accountTypeToIndex(account.TypeCreditCard)
	updateAccountFieldVisibility(d)
	fields[acctFieldCreditLimit].Value = "5000.00"

	// Now switch to Cash - credit limit and interest rate should be cleared
	fields[acctFieldType].SelectedIndex = accountTypeToIndex(account.TypeCash)
	updateAccountFieldVisibility(d)

	if fields[acctFieldCreditLimit].Value != "" {
		t.Errorf("credit limit should be cleared when hidden, got %q", fields[acctFieldCreditLimit].Value)
	}
	if fields[acctFieldInterestRate].Value != "" {
		t.Errorf("interest rate should be cleared when hidden, got %q", fields[acctFieldInterestRate].Value)
	}
}

func TestUpdateAccountFieldVisibility_ClearsHiddenErrors(t *testing.T) {
	d := buildNewAccountDialog()
	fields := d.Fields()

	// Set type to Credit Card with an error on credit limit
	fields[acctFieldType].SelectedIndex = accountTypeToIndex(account.TypeCreditCard)
	updateAccountFieldVisibility(d)
	fields[acctFieldCreditLimit].Error = "Invalid amount"

	// Switch to Checking - error should be cleared
	fields[acctFieldType].SelectedIndex = accountTypeToIndex(account.TypeChecking)
	updateAccountFieldVisibility(d)

	if fields[acctFieldCreditLimit].Error != "" {
		t.Errorf("credit limit error should be cleared when hidden, got %q", fields[acctFieldCreditLimit].Error)
	}
}

func TestApp_HandleAccountDialogKey_TypeChangeUpdatesVisibility(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *dialog.Dialog {
			d := buildNewAccountDialog()
			// Focus the Type field
			d.SetFocusIndex(acctFieldType)
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	fields := app.acctDialog.Fields()

	// Default is Checking (index 0) - credit limit hidden
	if !fields[acctFieldCreditLimit].Hidden {
		t.Error("credit limit should start hidden for Checking")
	}

	// Press down to change type to Savings (index 1)
	downKey := tea.KeyPressMsg{Code: tea.KeyDown}
	app.Update(downKey)

	// Still Savings - credit limit should still be hidden
	if !fields[acctFieldCreditLimit].Hidden {
		t.Error("credit limit should be hidden for Savings")
	}

	// Press down again to Credit Card (index 2)
	app.Update(downKey)

	// Now credit limit should be visible
	if fields[acctFieldCreditLimit].Hidden {
		t.Error("credit limit should be visible for Credit Card")
	}
}

func TestApp_SubmitAccountDialog_HiddenCreditLimitIgnored(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *dialog.Dialog {
			d := buildNewAccountDialog()
			d.Fields()[acctFieldName].Value = "My Checking"
			// Credit limit has invalid value but field is hidden (default Checking type)
			d.Fields()[acctFieldCreditLimit].Value = "abc"
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	// Since credit limit is hidden for Checking, invalid value should be ignored
	_, cmd := app.submitAccountDialog()

	if cmd == nil {
		t.Error("submit should succeed - hidden credit limit field should be ignored")
	}
}

func TestApp_SubmitAccountDialog_HiddenInterestRateIgnored(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		acctDialog: func() *dialog.Dialog {
			d := buildNewAccountDialog()
			d.Fields()[acctFieldName].Value = "My Cash"
			// Change type to Cash
			d.Fields()[acctFieldType].SelectedIndex = accountTypeToIndex(account.TypeCash)
			updateAccountFieldVisibility(d)
			// Interest rate has invalid value but field is hidden
			d.Fields()[acctFieldInterestRate].Value = "xyz"
			d.Fields()[acctFieldInterestRate].Hidden = true
			return d
		}(),
		acctDialogData: &accountDialogData{mode: accountDialogModeNew},
	}

	_, cmd := app.submitAccountDialog()

	if cmd == nil {
		t.Error("submit should succeed - hidden interest rate field should be ignored")
	}
}

func TestEditAccountDialog_FieldVisibility_Asset(t *testing.T) {
	acct := account.NewAccount(
		"House",
		account.TypeAsset,
		"USD",
		types.ZeroMoney,
		types.Today(),
	)

	d := buildEditAccountDialog(acct)
	fields := d.Fields()

	if !fields[acctFieldCreditLimit].Hidden {
		t.Error("credit limit should be hidden for Asset")
	}
	if !fields[acctFieldInterestRate].Hidden {
		t.Error("interest rate should be hidden for Asset")
	}
}

func TestDialog_FocusNext_SkipsHiddenFields(t *testing.T) {
	d := buildNewAccountDialog()
	// Default type is Checking: credit limit (index 8) is hidden

	// Focus on Notes (index 7, just before credit limit)
	d.SetFocusIndex(acctFieldNotes)

	// FocusNext should skip hidden credit limit (index 8) and land on interest rate (index 9)
	d.FocusNext()

	if d.FocusIndex() != acctFieldInterestRate {
		t.Errorf("FocusNext from Notes should skip hidden credit limit and land on interest rate, got focus index %d", d.FocusIndex())
	}
}

func TestDialog_FocusPrev_SkipsHiddenFields(t *testing.T) {
	d := buildNewAccountDialog()
	// Default type is Checking: credit limit (index 8) is hidden

	// Focus on interest rate (index 9)
	d.SetFocusIndex(acctFieldInterestRate)

	// FocusPrev should skip hidden credit limit (index 8) and land on notes (index 7)
	d.FocusPrev()

	if d.FocusIndex() != acctFieldNotes {
		t.Errorf("FocusPrev from interest rate should skip hidden credit limit and land on notes, got focus index %d", d.FocusIndex())
	}
}

func TestDialog_Render_SkipsHiddenFields(t *testing.T) {
	d := buildNewAccountDialog()
	styles := widget.NewStyles()
	styles.Resize(80, 30)

	output := d.Render(styles)

	// Credit limit is hidden by default (Checking type)
	if strings.Contains(output, "Credit Limit") {
		t.Error("hidden Credit Limit field should not appear in rendered output")
	}

	// Interest rate should be visible for Checking
	if !strings.Contains(output, "Interest Rate") {
		t.Error("visible Interest Rate field should appear in rendered output")
	}
}

func TestDialog_Render_ShowsCreditLimitForCreditCard(t *testing.T) {
	d := buildNewAccountDialog()
	fields := d.Fields()
	fields[acctFieldType].SelectedIndex = accountTypeToIndex(account.TypeCreditCard)
	updateAccountFieldVisibility(d)

	styles := widget.NewStyles()
	styles.Resize(80, 30)

	output := d.Render(styles)

	if !strings.Contains(output, "Credit Limit") {
		t.Error("Credit Limit field should appear in rendered output for Credit Card")
	}
	if !strings.Contains(output, "Interest Rate") {
		t.Error("Interest Rate field should appear in rendered output for Credit Card")
	}
}

func TestDialog_Render_HidesBothForCash(t *testing.T) {
	d := buildNewAccountDialog()
	fields := d.Fields()
	fields[acctFieldType].SelectedIndex = accountTypeToIndex(account.TypeCash)
	updateAccountFieldVisibility(d)

	styles := widget.NewStyles()
	styles.Resize(80, 30)

	output := d.Render(styles)

	if strings.Contains(output, "Credit Limit") {
		t.Error("Credit Limit field should not appear for Cash")
	}
	if strings.Contains(output, "Interest Rate") {
		t.Error("Interest Rate field should not appear for Cash")
	}
}

func TestBuildNewAccountDialog_HasTrackLotsCheckbox(t *testing.T) {
	d := buildNewAccountDialog()
	fields := d.Fields()
	if len(fields) <= acctFieldTrackLots {
		t.Fatalf("expected a Track Lots field at index %d, only %d fields", acctFieldTrackLots, len(fields))
	}
	f := fields[acctFieldTrackLots]
	if f.Type != dialog.FieldCheckbox {
		t.Errorf("Track Lots field type = %v, want FieldCheckbox", f.Type)
	}
	if !f.Checked {
		t.Error("Track Lots should default to checked")
	}
}

func TestUpdateAccountFieldVisibility_TrackLots(t *testing.T) {
	d := buildNewAccountDialog()
	fields := d.Fields()

	fields[acctFieldType].SelectedIndex = accountTypeToIndex(account.TypeInvestment)
	updateAccountFieldVisibility(d)
	if fields[acctFieldTrackLots].Hidden {
		t.Error("Track Lots should be visible for investment accounts")
	}

	fields[acctFieldType].SelectedIndex = accountTypeToIndex(account.TypeHSA)
	updateAccountFieldVisibility(d)
	if fields[acctFieldTrackLots].Hidden {
		t.Error("Track Lots should be visible for HSA accounts")
	}

	fields[acctFieldType].SelectedIndex = accountTypeToIndex(account.TypeChecking)
	updateAccountFieldVisibility(d)
	if !fields[acctFieldTrackLots].Hidden {
		t.Error("Track Lots should be hidden for non-investment accounts")
	}
}

func TestBuildEditAccountDialog_NoTrackLotsCheckbox(t *testing.T) {
	acct := account.NewAccount("X", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	d := buildEditAccountDialog(acct)
	if len(d.Fields()) > acctFieldTrackLots {
		t.Errorf("edit dialog must not expose a Track Lots field (got %d fields)", len(d.Fields()))
	}
}
