package tui

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/service"
)

// =============================================================================
// Pure Function Tests
// =============================================================================

func TestParseDateInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		year  int
		month time.Month
		day   int
	}{
		{"standard date", "01/15/2024", 2024, time.January, 15},
		{"end of year", "12/31/2025", 2025, time.December, 31},
		{"leap day", "02/29/2024", 2024, time.February, 29},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := parseDateInput(tt.input)
			if err != nil {
				t.Fatalf("parseDateInput(%q) returned error: %v", tt.input, err)
			}
			if d.Year() != tt.year {
				t.Errorf("year = %d, want %d", d.Year(), tt.year)
			}
			if d.Month() != tt.month {
				t.Errorf("month = %v, want %v", d.Month(), tt.month)
			}
			if d.Day() != tt.day {
				t.Errorf("day = %d, want %d", d.Day(), tt.day)
			}
		})
	}
}

func TestParseDateInput_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"ISO format", "2024-01-15"},
		{"invalid month", "13/01/2024"},
		{"random text", "abc"},
		{"empty string", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseDateInput(tt.input)
			if err == nil {
				t.Errorf("parseDateInput(%q) should return error", tt.input)
			}
		})
	}
}

func TestParseAmountInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"positive", "100.50", "100.50"},
		{"negative", "-50.00", "-50.00"},
		{"dollar sign", "$25.00", "25.00"},
		{"negative dollar", "-$10.00", "-10.00"},
		{"whole number", "42", "42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := parseAmountInput(tt.input)
			if err != nil {
				t.Fatalf("parseAmountInput(%q) returned error: %v", tt.input, err)
			}
			expected := models.MustNewMoney(tt.expected)
			if !m.Equal(expected) {
				t.Errorf("parseAmountInput(%q) = %s, want %s", tt.input, m.String(), expected.String())
			}
		})
	}
}

func TestParseAmountInput_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"random text", "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseAmountInput(tt.input)
			if err == nil {
				t.Errorf("parseAmountInput(%q) should return error", tt.input)
			}
		})
	}
}

func TestBuildCategoryOptions(t *testing.T) {
	parentID := models.NewID()
	categories := []*models.Category{
		{
			BaseModel: models.BaseModel{ID: parentID},
			Name:      "Food",
			Type:      models.CategoryTypeExpense,
		},
		{
			BaseModel: models.BaseModel{ID: models.NewID()},
			Name:      "Groceries",
			ParentID:  models.NullableID{ID: parentID, Valid: true},
			Type:      models.CategoryTypeExpense,
		},
		{
			BaseModel: models.BaseModel{ID: models.NewID()},
			Name:      "Transfer",
			Type:      models.CategoryTypeExpense,
			IsSystem:  true,
		},
	}

	options, ids := buildCategoryOptions(categories)

	// First entry should be "(None)"
	if options[0] != "(None)" {
		t.Errorf("first option = %q, want %q", options[0], "(None)")
	}
	if ids[0] != models.NilID {
		t.Errorf("first ID should be NilID")
	}

	// System category should be excluded
	for _, opt := range options {
		if opt == "Transfer" {
			t.Error("system category 'Transfer' should be excluded")
		}
	}

	// Should have "(None)" + "Food" + "Food > Groceries" = 3
	if len(options) != 3 {
		t.Errorf("expected 3 options, got %d: %v", len(options), options)
	}

	// Check "Food > Groceries" format
	found := slices.Contains(options, "Food > Groceries")
	if !found {
		t.Errorf("expected 'Food > Groceries' in options, got: %v", options)
	}
}

func TestBuildCategoryOptions_Empty(t *testing.T) {
	options, ids := buildCategoryOptions(nil)

	if len(options) != 1 {
		t.Errorf("expected 1 option, got %d", len(options))
	}
	if options[0] != "(None)" {
		t.Errorf("option = %q, want %q", options[0], "(None)")
	}
	if len(ids) != 1 {
		t.Errorf("expected 1 ID, got %d", len(ids))
	}
}

func TestBuildTransactionDialog(t *testing.T) {
	data := &transactionDialogData{
		categories: []*models.Category{},
	}
	options := []string{"(None)"}

	d := buildTransactionDialog(data, options)

	if d.Title() != "New Transaction" {
		t.Errorf("title = %q, want %q", d.Title(), "New Transaction")
	}

	fields := d.Fields()
	if len(fields) != 7 {
		t.Fatalf("expected 7 fields, got %d", len(fields))
	}

	// Date field should default to today
	today := time.Now().Format("01/02/2006")
	if fields[0].Value != today {
		t.Errorf("date default = %q, want %q", fields[0].Value, today)
	}

	if !d.IsVisible() {
		t.Error("dialog should be visible after creation")
	}
}

func TestBuildTransactionDialog_FieldTypes(t *testing.T) {
	data := &transactionDialogData{}
	options := []string{"(None)", "Groceries"}

	d := buildTransactionDialog(data, options)
	fields := d.Fields()

	expected := []struct {
		label     string
		fieldType FieldType
	}{
		{"Date", FieldText},
		{"Payee", FieldText},
		{"Category", FieldSelect},
		{"Amount", FieldText},
		{"Memo", FieldText},
		{"Status", FieldRadio},
		{"Split transaction", FieldCheckbox},
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

// =============================================================================
// App Integration Tests (no database)
// =============================================================================

func TestApp_HandleRegisterKeys_NewKey(t *testing.T) {
	accountID := models.NewID()
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
			account: &models.Account{
				BaseModel: models.BaseModel{ID: accountID},
				Name:      "Checking",
			},
			transactions:  []*models.Transaction{},
			balance:       &service.AccountBalance{AccountID: accountID, CurrentBalance: models.ZeroMoney},
			payeeNames:    make(map[models.ID]string),
			categoryNames: make(map[models.ID]string),
			accountNames:  make(map[models.ID]string),
		},
	}
	app.buildRegisterTable()
	app.sidebar.SetFocused(false)
	app.table.SetFocused(true)

	// Press 'n' for new transaction
	nKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}
	_, cmd := app.Update(nKey)

	if cmd == nil {
		t.Error("pressing 'n' in register should return a non-nil cmd")
	}
}

func TestApp_Update_TransactionDialogDataMsg(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	data := &transactionDialogData{
		payees:     []*models.Payee{},
		categories: []*models.Category{},
		payeeMap:   make(map[string]*models.Payee),
	}

	msg := transactionDialogDataMsg{data: data}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.txnDialog == nil {
		t.Fatal("transaction dialog should be created")
	}
	if !updatedApp.txnDialog.IsVisible() {
		t.Error("transaction dialog should be visible")
	}
	if updatedApp.txnDialogData == nil {
		t.Error("transaction dialog data should be set")
	}
	if updatedApp.txnDialogCategoryIDs == nil {
		t.Error("transaction dialog category IDs should be set")
	}
}

func TestApp_HandleTransactionDialogKey_Cancel(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		txnDialog: func() *Dialog {
			d := NewDialog("New Transaction")
			d.AddTextField("Date", "01/01/2024", "", 10)
			d.SetVisible(true)
			return d
		}(),
		txnDialogData: &transactionDialogData{
			payeeMap: make(map[string]*models.Payee),
		},
		txnDialogCategoryIDs: []models.ID{models.NilID},
	}

	// Press Escape to cancel
	escKey := tea.KeyMsg{Type: tea.KeyEsc}
	model, _ := app.Update(escKey)
	updatedApp := model.(*App)

	if updatedApp.txnDialog != nil {
		t.Error("transaction dialog should be nil after cancel")
	}
}

func TestApp_HandleTransactionDialogKey_TabCycles(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		txnDialog: func() *Dialog {
			d := NewDialog("New Transaction")
			d.AddTextField("Date", "01/01/2024", "", 10)
			d.AddTextField("Payee", "", "Payee name", 0)
			d.SetVisible(true)
			return d
		}(),
		txnDialogData: &transactionDialogData{
			payeeMap: make(map[string]*models.Payee),
		},
		txnDialogCategoryIDs: []models.ID{models.NilID},
	}

	initialFocus := app.txnDialog.FocusIndex()
	if initialFocus != 0 {
		t.Fatalf("initial focus = %d, want 0", initialFocus)
	}

	// Press Tab to advance focus
	tabKey := tea.KeyMsg{Type: tea.KeyTab}
	model, _ := app.Update(tabKey)
	updatedApp := model.(*App)

	if updatedApp.txnDialog.FocusIndex() != 1 {
		t.Errorf("focus after Tab = %d, want 1", updatedApp.txnDialog.FocusIndex())
	}
}

func TestApp_Update_TransactionDialogSavedMsg(t *testing.T) {
	accountID := models.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}
	// Set up sidebar with a selected account
	app.sidebar.SetAccounts([]*models.Account{
		{BaseModel: models.BaseModel{ID: accountID}, Name: "Checking", Active: true, Type: models.AccountTypeChecking},
	}, nil)

	msg := transactionDialogSavedMsg{}
	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Error("transactionDialogSavedMsg should return a reload command")
	}
}

func TestApp_CheckPayeeAutoFill(t *testing.T) {
	categoryID := models.NewID()
	payeeID := models.NewID()

	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		txnDialog: func() *Dialog {
			d := NewDialog("New Transaction")
			d.AddTextField("Date", "01/01/2024", "", 10)
			d.AddTextField("Payee", "kroger", "Payee name", 0)
			d.AddSelectField("Category", []string{"(None)", "Groceries"}, 0)
			d.AddTextField("Amount", "", "-50.00", 12)
			d.AddTextField("Memo", "", "", 0)
			d.AddRadioField("Status", []string{"Pending", "Cleared"}, 0)
			d.SetVisible(true)
			return d
		}(),
		txnDialogData: &transactionDialogData{
			payeeMap: map[string]*models.Payee{
				"kroger": {
					BaseModel:         models.BaseModel{ID: payeeID},
					Name:              "Kroger",
					DefaultCategoryID: models.NullableID{ID: categoryID, Valid: true},
				},
			},
		},
		txnDialogCategoryIDs: []models.ID{models.NilID, categoryID},
	}

	app.checkPayeeAutoFill()

	// Category should be auto-selected to index 1 (Groceries)
	fields := app.txnDialog.Fields()
	if fields[2].SelectedIndex != 1 {
		t.Errorf("category selectedIndex = %d, want 1", fields[2].SelectedIndex)
	}
}

func TestApp_CheckPayeeAutoFill_NoMatch(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		txnDialog: func() *Dialog {
			d := NewDialog("New Transaction")
			d.AddTextField("Date", "01/01/2024", "", 10)
			d.AddTextField("Payee", "unknown", "Payee name", 0)
			d.AddSelectField("Category", []string{"(None)", "Groceries"}, 0)
			d.AddTextField("Amount", "", "", 12)
			d.AddTextField("Memo", "", "", 0)
			d.AddRadioField("Status", []string{"Pending", "Cleared"}, 0)
			d.SetVisible(true)
			return d
		}(),
		txnDialogData: &transactionDialogData{
			payeeMap: make(map[string]*models.Payee),
		},
		txnDialogCategoryIDs: []models.ID{models.NilID, models.NewID()},
	}

	app.checkPayeeAutoFill()

	// Category should remain at index 0 (None)
	fields := app.txnDialog.Fields()
	if fields[2].SelectedIndex != 0 {
		t.Errorf("category selectedIndex = %d, want 0", fields[2].SelectedIndex)
	}
}

func TestApp_SubmitTransactionDialog_InvalidDate(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		txnDialog: func() *Dialog {
			d := NewDialog("New Transaction")
			d.AddTextField("Date", "not-a-date", "", 10)
			d.AddTextField("Payee", "Test Payee", "", 0)
			d.AddSelectField("Category", []string{"(None)"}, 0)
			d.AddTextField("Amount", "-50.00", "", 12)
			d.AddTextField("Memo", "", "", 0)
			d.AddRadioField("Status", []string{"Pending", "Cleared"}, 0)
			d.AddCheckboxField("Split transaction", false)
			d.SetVisible(true)
			return d
		}(),
		txnDialogData:        &transactionDialogData{payeeMap: make(map[string]*models.Payee)},
		txnDialogCategoryIDs: []models.ID{models.NilID},
	}

	_, cmd := app.submitTransactionDialog()
	if cmd != nil {
		t.Error("invalid date should not return a cmd")
	}
	if app.txnDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.txnDialog.Fields()[0].Error == "" {
		t.Error("date field should have error")
	}
}

func TestApp_SubmitTransactionDialog_InvalidAmount(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		txnDialog: func() *Dialog {
			d := NewDialog("New Transaction")
			d.AddTextField("Date", "01/15/2024", "", 10)
			d.AddTextField("Payee", "Test Payee", "", 0)
			d.AddSelectField("Category", []string{"(None)"}, 0)
			d.AddTextField("Amount", "", "", 12)
			d.AddTextField("Memo", "", "", 0)
			d.AddRadioField("Status", []string{"Pending", "Cleared"}, 0)
			d.AddCheckboxField("Split transaction", false)
			d.SetVisible(true)
			return d
		}(),
		txnDialogData:        &transactionDialogData{payeeMap: make(map[string]*models.Payee)},
		txnDialogCategoryIDs: []models.ID{models.NilID},
	}

	_, cmd := app.submitTransactionDialog()
	if cmd != nil {
		t.Error("invalid amount should not return a cmd")
	}
	if app.txnDialog == nil {
		t.Fatal("dialog should remain open after validation failure")
	}
	if app.txnDialog.Fields()[3].Error == "" {
		t.Error("amount field should have error")
	}
}

func TestApp_SubmitTransactionDialog_MultipleErrors(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		txnDialog: func() *Dialog {
			d := NewDialog("New Transaction")
			d.AddTextField("Date", "bad-date", "", 10)
			d.AddTextField("Payee", "", "", 0)
			d.AddSelectField("Category", []string{"(None)"}, 0)
			d.AddTextField("Amount", "", "", 12)
			d.AddTextField("Memo", "", "", 0)
			d.AddRadioField("Status", []string{"Pending", "Cleared"}, 0)
			d.AddCheckboxField("Split transaction", false)
			d.SetVisible(true)
			return d
		}(),
		txnDialogData:        &transactionDialogData{payeeMap: make(map[string]*models.Payee)},
		txnDialogCategoryIDs: []models.ID{models.NilID},
	}

	_, cmd := app.submitTransactionDialog()
	if cmd != nil {
		t.Error("should not return a cmd with multiple errors")
	}
	if app.txnDialog == nil {
		t.Fatal("dialog should remain open")
	}

	fields := app.txnDialog.Fields()
	if fields[0].Error == "" {
		t.Error("date field should have error")
	}
	if fields[3].Error == "" {
		t.Error("amount field should have error")
	}
}

func TestApp_SubmitTransactionDialog_ValidNonSplit(t *testing.T) {
	accountID := models.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		txnDialog: func() *Dialog {
			d := NewDialog("New Transaction")
			d.AddTextField("Date", "01/15/2024", "", 10)
			d.AddTextField("Payee", "Coffee Shop", "", 0)
			d.AddSelectField("Category", []string{"(None)", "Food"}, 0)
			d.AddTextField("Amount", "-5.00", "", 12)
			d.AddTextField("Memo", "Morning coffee", "", 0)
			d.AddRadioField("Status", []string{"Pending", "Cleared"}, 0)
			d.AddCheckboxField("Split transaction", false)
			d.SetVisible(true)
			return d
		}(),
		txnDialogData: &transactionDialogData{
			payeeMap: make(map[string]*models.Payee),
		},
		txnDialogCategoryIDs: []models.ID{models.NilID, models.NewID()},
	}

	app.sidebar.SetAccounts([]*models.Account{
		{BaseModel: models.BaseModel{ID: accountID}, Name: "Checking", Active: true, Type: models.AccountTypeChecking},
	}, nil)

	model, cmd := app.submitTransactionDialog()
	updatedApp := model.(*App)

	if cmd == nil {
		t.Error("valid transaction should return a non-nil cmd")
	}
	if updatedApp.txnDialog != nil {
		t.Error("dialog should be closed after valid submit")
	}
	if updatedApp.txnDialogData != nil {
		t.Error("dialog data should be nil after submit")
	}
}

func TestApp_CloseTransactionDialog(t *testing.T) {
	app := &App{
		txnDialog: func() *Dialog {
			d := NewDialog("New Transaction")
			d.SetVisible(true)
			return d
		}(),
		txnDialogData:        &transactionDialogData{payeeMap: make(map[string]*models.Payee)},
		txnDialogCategoryIDs: []models.ID{models.NilID},
	}

	app.closeTransactionDialog()

	if app.txnDialog != nil {
		t.Error("dialog should be nil after close")
	}
	if app.txnDialogData != nil {
		t.Error("dialog data should be nil after close")
	}
	if app.txnDialogCategoryIDs != nil {
		t.Error("category IDs should be nil after close")
	}
}

func TestApp_CheckPayeeAutoFill_NilDialog(t *testing.T) {
	app := &App{}
	// Should not panic
	app.checkPayeeAutoFill()
}

func TestApp_CheckPayeeAutoFill_EmptyPayee(t *testing.T) {
	catID := models.NewID()
	app := &App{
		txnDialog: func() *Dialog {
			d := NewDialog("New Transaction")
			d.AddTextField("Date", "01/01/2024", "", 10)
			d.AddTextField("Payee", "", "", 0)
			d.AddSelectField("Category", []string{"(None)", "Food"}, 0)
			d.AddTextField("Amount", "", "", 12)
			d.AddTextField("Memo", "", "", 0)
			d.AddRadioField("Status", []string{"Pending", "Cleared"}, 0)
			d.AddCheckboxField("Split transaction", false)
			d.SetVisible(true)
			return d
		}(),
		txnDialogData: &transactionDialogData{
			payeeMap: map[string]*models.Payee{
				"coffee shop": {
					BaseModel:         models.BaseModel{ID: models.NewID()},
					Name:              "Coffee Shop",
					DefaultCategoryID: models.NullableID{ID: catID, Valid: true},
				},
			},
		},
		txnDialogCategoryIDs: []models.ID{models.NilID, catID},
	}

	app.checkPayeeAutoFill()

	// Category should remain at (None)
	if app.txnDialog.Fields()[2].SelectedIndex != 0 {
		t.Errorf("category should remain at 0, got %d", app.txnDialog.Fields()[2].SelectedIndex)
	}
}

func TestApp_SubmitTransactionDialog_NilDialog(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	model, cmd := app.submitTransactionDialog()
	if model != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

func TestApp_HandleTransactionDialogKey_NilDialog(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	model, cmd := app.handleTransactionDialogKey(tea.KeyMsg{Type: tea.KeyEnter})
	if model != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

func TestBuildCategoryOptions_Sorted(t *testing.T) {
	categories := []*models.Category{
		{BaseModel: models.BaseModel{ID: models.NewID()}, Name: "Utilities", Type: models.CategoryTypeExpense},
		{BaseModel: models.BaseModel{ID: models.NewID()}, Name: "Food", Type: models.CategoryTypeExpense},
		{BaseModel: models.BaseModel{ID: models.NewID()}, Name: "Housing", Type: models.CategoryTypeExpense},
	}

	options, _ := buildCategoryOptions(categories)
	// Skip first "(None)" entry
	if len(options) != 4 {
		t.Fatalf("Expected 4 options, got %d", len(options))
	}
	if options[1] != "Food" {
		t.Errorf("Expected 'Food' at index 1, got %q", options[1])
	}
	if options[2] != "Housing" {
		t.Errorf("Expected 'Housing' at index 2, got %q", options[2])
	}
	if options[3] != "Utilities" {
		t.Errorf("Expected 'Utilities' at index 3, got %q", options[3])
	}
}

func TestBuildCategoryOptions_IDMapping(t *testing.T) {
	catID := models.NewID()
	categories := []*models.Category{
		{BaseModel: models.BaseModel{ID: catID}, Name: "Food", Type: models.CategoryTypeExpense},
	}

	options, ids := buildCategoryOptions(categories)
	if len(options) != 2 || len(ids) != 2 {
		t.Fatalf("Expected 2 options and 2 IDs, got %d and %d", len(options), len(ids))
	}
	if ids[0] != models.NilID {
		t.Error("First ID should be NilID")
	}
	if ids[1] != catID {
		t.Errorf("Second ID should be %s, got %s", catID.String(), ids[1].String())
	}
}

func TestParseAmountInput_DollarNegative(t *testing.T) {
	// Test $-50.00 format
	amount, err := parseAmountInput("$-50.00")
	if err != nil {
		t.Fatalf("parseAmountInput($-50.00) error = %v", err)
	}
	expected := models.MustNewMoney("-50")
	if !amount.Equal(expected) {
		t.Errorf("parseAmountInput($-50.00) = %s, want %s", amount.String(), expected.String())
	}
}

func TestApp_RenderLayout_WithTransactionDialog(t *testing.T) {
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
			account: &models.Account{
				BaseModel: models.BaseModel{ID: models.NewID()},
				Name:      "Checking",
			},
			transactions:  []*models.Transaction{},
			balance:       &service.AccountBalance{CurrentBalance: models.ZeroMoney},
			payeeNames:    make(map[models.ID]string),
			categoryNames: make(map[models.ID]string),
			accountNames:  make(map[models.ID]string),
		},
		txnDialog: func() *Dialog {
			d := NewDialog("New Transaction")
			d.AddTextField("Date", "01/01/2024", "", 10)
			d.SetVisible(true)
			return d
		}(),
	}

	output := app.renderLayout()
	if !strings.Contains(output, "New Transaction") {
		t.Error("renderLayout() should contain 'New Transaction' when dialog is visible")
	}
}
