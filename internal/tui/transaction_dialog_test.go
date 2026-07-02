package tui

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
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
			expected := types.MustNewMoney(tt.expected)
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
	parentID := types.NewID()
	categories := []*category.Category{
		{
			BaseModel: types.BaseModel{ID: parentID},
			Name:      "Food",
			Type:      category.TypeExpense,
		},
		{
			BaseModel: types.BaseModel{ID: types.NewID()},
			Name:      "Groceries",
			ParentID:  types.NullableID{ID: parentID, Valid: true},
			Type:      category.TypeExpense,
		},
		{
			BaseModel: types.BaseModel{ID: types.NewID()},
			Name:      "Transfer",
			Type:      category.TypeExpense,
			IsSystem:  true,
		},
	}

	options, ids := buildCategoryOptions(categories)

	// First entry should be "(None)"
	if options[0] != "(None)" {
		t.Errorf("first option = %q, want %q", options[0], "(None)")
	}
	if ids[0] != types.NilID {
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

func TestBuildCategoryOptionsFor_ValueAdjustment(t *testing.T) {
	categories := []*category.Category{
		{
			BaseModel: types.BaseModel{ID: types.NewID()},
			Name:      "Food",
			Type:      category.TypeExpense,
		},
		{
			BaseModel: types.BaseModel{ID: types.NewID()},
			Name:      category.TransferCategoryName,
			Type:      category.TypeExpense,
			IsSystem:  true,
		},
		{
			BaseModel: types.BaseModel{ID: types.NewID()},
			Name:      category.ValueAdjustmentCategoryName,
			Type:      category.TypeExpense,
			IsSystem:  true,
		},
	}

	t.Run("excludes Value Adjustment by default", func(t *testing.T) {
		options, _ := buildCategoryOptionsFor(categories, false)
		if slices.Contains(options, category.ValueAdjustmentCategoryName) {
			t.Error("Value Adjustment should be excluded when includeValueAdjustment is false")
		}
		if slices.Contains(options, category.TransferCategoryName) {
			t.Error("Transfer should always be excluded")
		}
	})

	t.Run("includes Value Adjustment when requested but never Transfer", func(t *testing.T) {
		options, ids := buildCategoryOptionsFor(categories, true)
		if !slices.Contains(options, category.ValueAdjustmentCategoryName) {
			t.Errorf("Value Adjustment should be included; got %v", options)
		}
		if slices.Contains(options, category.TransferCategoryName) {
			t.Error("Transfer must stay excluded even when Value Adjustment is surfaced")
		}
		// Options and IDs stay parallel.
		if len(options) != len(ids) {
			t.Fatalf("options/ids length mismatch: %d vs %d", len(options), len(ids))
		}
	})
}

func TestBuildCategoryOptionsForAccount(t *testing.T) {
	categories := []*category.Category{
		{
			BaseModel: types.BaseModel{ID: types.NewID()},
			Name:      category.ValueAdjustmentCategoryName,
			Type:      category.TypeExpense,
			IsSystem:  true,
		},
	}

	tests := []struct {
		name   string
		acct   *account.Account
		wantVA bool
	}{
		{"asset account offers Value Adjustment", &account.Account{Type: account.TypeAsset}, true},
		{"checking account does not", &account.Account{Type: account.TypeChecking}, false},
		{"investment account does not (IsAssetType but not TypeAsset)", &account.Account{Type: account.TypeInvestment}, false},
		{"loan account does not", &account.Account{Type: account.TypeLoan}, false},
		{"nil account does not", nil, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			options, _ := buildCategoryOptionsForAccount(categories, tc.acct)
			got := slices.Contains(options, category.ValueAdjustmentCategoryName)
			if got != tc.wantVA {
				t.Errorf("Value Adjustment present = %v, want %v (options=%v)", got, tc.wantVA, options)
			}
		})
	}
}

func TestAccountIsAssetByID(t *testing.T) {
	asset := &account.Account{BaseModel: types.BaseModel{ID: types.NewID()}, Type: account.TypeAsset}
	checking := &account.Account{BaseModel: types.BaseModel{ID: types.NewID()}, Type: account.TypeChecking}
	investment := &account.Account{BaseModel: types.BaseModel{ID: types.NewID()}, Type: account.TypeInvestment}
	accounts := []*account.Account{asset, checking, investment}

	if !accountIsAssetByID(accounts, asset.ID) {
		t.Error("asset account should be recognized as TypeAsset")
	}
	if accountIsAssetByID(accounts, checking.ID) {
		t.Error("checking account should not be TypeAsset")
	}
	if accountIsAssetByID(accounts, investment.ID) {
		t.Error("investment account should not be TypeAsset (IsAssetType is broader)")
	}
	if accountIsAssetByID(accounts, types.NewID()) {
		t.Error("unknown account ID should return false")
	}
	if accountIsAssetByID(accounts, types.NilID) {
		t.Error("nil account ID should return false")
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
		categories: []*category.Category{},
	}
	options := []string{"(None)"}

	d := buildTransactionDialog(data, options, []types.ID{types.NilID}, types.ZeroDate)

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

func TestBuildTransactionDialog_SeedsFromStickyDate(t *testing.T) {
	data := &transactionDialogData{
		categories: []*category.Category{},
	}
	options := []string{"(None)"}
	seed := types.NewDate(2024, time.January, 15)

	d := buildTransactionDialog(data, options, []types.ID{types.NilID}, seed)

	fields := d.Fields()
	if fields[0].Value != "01/15/2024" {
		t.Errorf("date with seed = %q, want %q", fields[0].Value, "01/15/2024")
	}
}

func TestBuildTransactionDialog_FieldTypes(t *testing.T) {
	data := &transactionDialogData{}
	options := []string{"(None)", "Groceries"}

	d := buildTransactionDialog(data, options, []types.ID{types.NilID, types.NewID()}, types.ZeroDate)
	fields := d.Fields()

	expected := []struct {
		label     string
		fieldType dialog.FieldType
	}{
		{"Date", dialog.FieldDate},
		{"Payee", dialog.FieldText},
		{"Category", dialog.FieldCombo},
		{"Amount", dialog.FieldText},
		{"Memo", dialog.FieldText},
		{"Status", dialog.FieldRadio},
		{"Split transaction", dialog.FieldCheckbox},
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

// TestBuildTransactionDialog_CategoryComboHasAddNewLabel pins that the Category
// combo built by buildTransactionDialog carries the AddNewLabel sentinel so the
// [+ Add new category…] action row appears in the dropdown. CC-001.
func TestBuildTransactionDialog_CategoryComboHasAddNewLabel(t *testing.T) {
	data := &transactionDialogData{}
	options := []string{"(None)", "Groceries"}

	d := buildTransactionDialog(data, options, []types.ID{types.NilID, types.NewID()}, types.ZeroDate)
	got := d.Fields()[2].AddNewLabel
	want := "[+ Add new category…]"
	if got != want {
		t.Errorf("Category AddNewLabel = %q, want %q", got, want)
	}
}

// TestBuildTransactionDialog_DateFieldOverwriteSemantics asserts that the Date
// field built by buildTransactionDialog uses the dialog.FieldDate widget's overwrite
// semantics: typing two digits overwrites the month digits in place and the
// resulting Value is still a canonical 10-char MM/DD/YYYY string.
func TestBuildTransactionDialog_DateFieldOverwriteSemantics(t *testing.T) {
	data := &transactionDialogData{}
	options := []string{"(None)"}
	seed := types.NewDate(2024, time.January, 15)

	d := buildTransactionDialog(data, options, []types.ID{types.NilID}, seed)
	d.SetFocusIndex(0) // Date field

	// Type "0" then "2" — should rewrite the month from "01" to "02".
	d.HandleKey(tea.KeyPressMsg{Code: '0', Text: "0"})
	d.HandleKey(tea.KeyPressMsg{Code: '2', Text: "2"})

	got := d.Fields()[0].Value
	want := "02/15/2024"
	if got != want {
		t.Errorf("after typing 0,2 over month: Value = %q, want %q", got, want)
	}
	if len(got) != 10 {
		t.Errorf("Value len = %d, want 10 (canonical MM/DD/YYYY)", len(got))
	}
}

// TestBuildTransactionDialog_CategoryFieldFiltersAndCommits asserts that the
// Category field built by buildTransactionDialog uses the dialog.FieldCombo widget:
// typing narrows the filtered list, Enter commits the highlighted match, and
// SelectedIndex resolves to that match's index in the full options list.
func TestBuildTransactionDialog_CategoryFieldFiltersAndCommits(t *testing.T) {
	data := &transactionDialogData{}
	options := []string{"(None)", "Auto", "Bills > Electric", "Food > Groceries", "Food > Restaurants"}
	ids := []types.ID{types.NilID, types.NewID(), types.NewID(), types.NewID(), types.NewID()}

	d := buildTransactionDialog(data, options, ids, types.ZeroDate)
	d.SetFocusIndex(2) // Category field

	// Type "g" — only "Food > Groceries" should remain in the filtered list.
	d.HandleKey(tea.KeyPressMsg{Code: 'g', Text: "g"})

	cat := d.Fields()[2]
	if cat.Type != dialog.FieldCombo {
		t.Fatalf("Category field type = %v, want dialog.FieldCombo", cat.Type)
	}
	if cat.Query != "g" {
		t.Errorf("Query after typing 'g' = %q, want %q", cat.Query, "g")
	}
	indices := cat.FilteredIndices()
	if len(indices) != 1 {
		t.Fatalf("filtered indices = %v, want one match", indices)
	}
	if options[indices[0]] != "Food > Groceries" {
		t.Errorf("filtered match = %q, want %q", options[indices[0]], "Food > Groceries")
	}

	// Enter commits the highlighted row and clears the query.
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cat.Query != "" {
		t.Errorf("Query after Enter = %q, want empty", cat.Query)
	}
	wantIdx := indices[0]
	if cat.SelectedIndex != wantIdx {
		t.Errorf("SelectedIndex after commit = %d, want %d (Food > Groceries in full list)",
			cat.SelectedIndex, wantIdx)
	}
}

// =============================================================================
// App Integration Tests (no database)
// =============================================================================

func TestApp_HandleRegisterKeys_NewKey(t *testing.T) {
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

	// Press 'n' for new transaction
	nKey := tea.KeyPressMsg{Code: 'n', Text: "n"}
	_, cmd := app.Update(nKey)

	if cmd == nil {
		t.Error("pressing 'n' in register should return a non-nil cmd")
	}
}

func TestApp_Update_TransactionDialogDataMsg(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	data := &transactionDialogData{
		payees:     []*payee.Payee{},
		categories: []*category.Category{},
		payeeMap:   make(map[string]*payee.Payee),
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		txnDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transaction")
			d.AddDateField("Date", "01/01/2024")
			d.SetVisible(true)
			return d
		}(),
		txnDialogData: &transactionDialogData{
			payeeMap: make(map[string]*payee.Payee),
		},
		txnDialogCategoryIDs: []types.ID{types.NilID},
	}

	// Press Escape to cancel
	escKey := tea.KeyPressMsg{Code: tea.KeyEsc}
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		txnDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transaction")
			d.AddDateField("Date", "01/01/2024")
			d.AddTextField("Payee", "", "Payee name", 0)
			d.SetVisible(true)
			return d
		}(),
		txnDialogData: &transactionDialogData{
			payeeMap: make(map[string]*payee.Payee),
		},
		txnDialogCategoryIDs: []types.ID{types.NilID},
	}

	initialFocus := app.txnDialog.FocusIndex()
	if initialFocus != 0 {
		t.Fatalf("initial focus = %d, want 0", initialFocus)
	}

	// Press Tab to advance focus
	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	model, _ := app.Update(tabKey)
	updatedApp := model.(*App)

	if updatedApp.txnDialog.FocusIndex() != 1 {
		t.Errorf("focus after Tab = %d, want 1", updatedApp.txnDialog.FocusIndex())
	}
}

func TestApp_Update_TransactionDialogSavedMsg(t *testing.T) {
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

	msg := transactionDialogSavedMsg{}
	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Error("transactionDialogSavedMsg should return a reload command")
	}
}

func TestApp_Update_TransactionDialogSavedMsg_StoresStickyDate(t *testing.T) {
	accountID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}
	app.sidebar.SetAccounts([]*account.Account{
		{BaseModel: types.BaseModel{ID: accountID}, Name: "Checking", Active: true, Type: account.TypeChecking},
	}, nil)

	saved := types.NewDate(2024, time.January, 15)
	model, _ := app.Update(transactionDialogSavedMsg{savedDate: saved})
	updatedApp := model.(*App)

	if !updatedApp.txnDialogLastSavedDate.Equal(saved) {
		t.Errorf("txnDialogLastSavedDate = %s, want %s", updatedApp.txnDialogLastSavedDate, saved)
	}
}

func TestApp_Update_TransactionDialogDataMsg_SeedsFromStickyDate(t *testing.T) {
	app := &App{
		currentView:            ViewRegister,
		keys:                   defaultKeyMap(),
		menubar:                widget.NewMenuBar(),
		statusbar:              widget.NewStatusBar(),
		sidebar:                NewSidebar(),
		txnDialogLastSavedDate: types.NewDate(2024, time.January, 15),
	}

	data := &transactionDialogData{
		payees:     []*payee.Payee{},
		categories: []*category.Category{},
		payeeMap:   make(map[string]*payee.Payee),
	}

	model, _ := app.Update(transactionDialogDataMsg{data: data})
	updatedApp := model.(*App)

	if updatedApp.txnDialog == nil {
		t.Fatal("transaction dialog should be created")
	}
	dateValue := updatedApp.txnDialog.Fields()[0].Value
	if dateValue != "01/15/2024" {
		t.Errorf("date field = %q, want %q (seeded from sticky date)", dateValue, "01/15/2024")
	}
}

func TestApp_Update_TransactionDialogDataMsg_DefaultsToTodayWhenNoStickyDate(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	data := &transactionDialogData{
		payees:     []*payee.Payee{},
		categories: []*category.Category{},
		payeeMap:   make(map[string]*payee.Payee),
	}

	model, _ := app.Update(transactionDialogDataMsg{data: data})
	updatedApp := model.(*App)

	today := time.Now().Format("01/02/2006")
	dateValue := updatedApp.txnDialog.Fields()[0].Value
	if dateValue != today {
		t.Errorf("date field = %q, want %q (today)", dateValue, today)
	}
}

func TestApp_TransactionDialogCancel_DoesNotUpdateStickyDate(t *testing.T) {
	initial := types.NewDate(2024, time.January, 15)
	app := &App{
		currentView:            ViewRegister,
		keys:                   defaultKeyMap(),
		menubar:                widget.NewMenuBar(),
		statusbar:              widget.NewStatusBar(),
		sidebar:                NewSidebar(),
		txnDialogLastSavedDate: initial,
		txnDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transaction")
			// User typed a different date but cancels
			d.AddDateField("Date", "02/01/2024")
			d.SetVisible(true)
			return d
		}(),
		txnDialogData: &transactionDialogData{
			payeeMap: make(map[string]*payee.Payee),
		},
		txnDialogCategoryIDs: []types.ID{types.NilID},
	}

	escKey := tea.KeyPressMsg{Code: tea.KeyEsc}
	model, _ := app.Update(escKey)
	updatedApp := model.(*App)

	if !updatedApp.txnDialogLastSavedDate.Equal(initial) {
		t.Errorf("sticky date changed on cancel: got %s, want %s",
			updatedApp.txnDialogLastSavedDate, initial)
	}
}

func TestApp_SubmitTransactionDialog_PassesSavedDateInMessage(t *testing.T) {
	accountID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		txnDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transaction")
			d.AddDateField("Date", "01/15/2024")
			d.AddTextField("Payee", "Coffee Shop", "", 0)
			d.AddComboField("Category", []string{"(None)", "Food"}, 0)
			d.AddTextField("Amount", "-5.00", "", 12)
			d.AddTextField("Memo", "", "", 0)
			d.AddRadioField("Status", []string{"Pending", "Cleared"}, 0)
			d.AddCheckboxField("Split transaction", false)
			d.SetVisible(true)
			return d
		}(),
		txnDialogData: &transactionDialogData{
			payeeMap: make(map[string]*payee.Payee),
		},
		txnDialogCategoryIDs: []types.ID{types.NilID, types.NewID()},
	}
	app.sidebar.SetAccounts([]*account.Account{
		{BaseModel: types.BaseModel{ID: accountID}, Name: "Checking", Active: true, Type: account.TypeChecking},
	}, nil)

	_, cmd := app.submitTransactionDialog()
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	msg := cmd()
	saved, ok := msg.(transactionDialogSavedMsg)
	if !ok {
		t.Fatalf("expected transactionDialogSavedMsg, got %T", msg)
	}

	want := types.NewDate(2024, time.January, 15)
	if !saved.savedDate.Equal(want) {
		t.Errorf("savedDate = %s, want %s", saved.savedDate, want)
	}
}

func TestApp_SubmitThenSaved_UpdatesStickyDate_AcrossOpens(t *testing.T) {
	accountID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}
	app.sidebar.SetAccounts([]*account.Account{
		{BaseModel: types.BaseModel{ID: accountID}, Name: "Checking", Active: true, Type: account.TypeChecking},
	}, nil)

	// First saved date.
	first := types.NewDate(2024, time.January, 15)
	model, _ := app.Update(transactionDialogSavedMsg{savedDate: first})
	app = model.(*App)
	if !app.txnDialogLastSavedDate.Equal(first) {
		t.Fatalf("after first save: got %s, want %s", app.txnDialogLastSavedDate, first)
	}

	// Reopen the dialog: it should seed from `first`.
	data := &transactionDialogData{payeeMap: make(map[string]*payee.Payee)}
	model, _ = app.Update(transactionDialogDataMsg{data: data})
	app = model.(*App)
	if app.txnDialog.Fields()[0].Value != "01/15/2024" {
		t.Errorf("first reopen: date = %q, want %q",
			app.txnDialog.Fields()[0].Value, "01/15/2024")
	}

	// A second save updates the sticky date.
	second := types.NewDate(2024, time.February, 1)
	model, _ = app.Update(transactionDialogSavedMsg{savedDate: second})
	app = model.(*App)
	if !app.txnDialogLastSavedDate.Equal(second) {
		t.Fatalf("after second save: got %s, want %s", app.txnDialogLastSavedDate, second)
	}

	// Reopening reflects the second saved date.
	app.closeTransactionDialog()
	model, _ = app.Update(transactionDialogDataMsg{data: data})
	app = model.(*App)
	if app.txnDialog.Fields()[0].Value != "02/01/2024" {
		t.Errorf("second reopen: date = %q, want %q",
			app.txnDialog.Fields()[0].Value, "02/01/2024")
	}
}

func TestApp_CheckPayeeAutoFill(t *testing.T) {
	categoryID := types.NewID()
	payeeID := types.NewID()

	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		txnDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transaction")
			d.AddDateField("Date", "01/01/2024")
			d.AddTextField("Payee", "kroger", "Payee name", 0)
			d.AddComboField("Category", []string{"(None)", "Groceries"}, 0)
			d.AddTextField("Amount", "", "-50.00", 12)
			d.AddTextField("Memo", "", "", 0)
			d.AddRadioField("Status", []string{"Pending", "Cleared"}, 0)
			d.SetVisible(true)
			return d
		}(),
		txnDialogData: &transactionDialogData{
			payeeMap: map[string]*payee.Payee{
				"kroger": {
					BaseModel:         types.BaseModel{ID: payeeID},
					Name:              "Kroger",
					DefaultCategoryID: types.NullableID{ID: categoryID, Valid: true},
				},
			},
		},
		txnDialogCategoryIDs: []types.ID{types.NilID, categoryID},
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		txnDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transaction")
			d.AddDateField("Date", "01/01/2024")
			d.AddTextField("Payee", "unknown", "Payee name", 0)
			d.AddComboField("Category", []string{"(None)", "Groceries"}, 0)
			d.AddTextField("Amount", "", "", 12)
			d.AddTextField("Memo", "", "", 0)
			d.AddRadioField("Status", []string{"Pending", "Cleared"}, 0)
			d.SetVisible(true)
			return d
		}(),
		txnDialogData: &transactionDialogData{
			payeeMap: make(map[string]*payee.Payee),
		},
		txnDialogCategoryIDs: []types.ID{types.NilID, types.NewID()},
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		txnDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transaction")
			d.AddDateField("Date", "13/45/2024")
			d.AddTextField("Payee", "Test Payee", "", 0)
			d.AddComboField("Category", []string{"(None)"}, 0)
			d.AddTextField("Amount", "-50.00", "", 12)
			d.AddTextField("Memo", "", "", 0)
			d.AddRadioField("Status", []string{"Pending", "Cleared"}, 0)
			d.AddCheckboxField("Split transaction", false)
			d.SetVisible(true)
			return d
		}(),
		txnDialogData:        &transactionDialogData{payeeMap: make(map[string]*payee.Payee)},
		txnDialogCategoryIDs: []types.ID{types.NilID},
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		txnDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transaction")
			d.AddDateField("Date", "01/15/2024")
			d.AddTextField("Payee", "Test Payee", "", 0)
			d.AddComboField("Category", []string{"(None)"}, 0)
			d.AddTextField("Amount", "", "", 12)
			d.AddTextField("Memo", "", "", 0)
			d.AddRadioField("Status", []string{"Pending", "Cleared"}, 0)
			d.AddCheckboxField("Split transaction", false)
			d.SetVisible(true)
			return d
		}(),
		txnDialogData:        &transactionDialogData{payeeMap: make(map[string]*payee.Payee)},
		txnDialogCategoryIDs: []types.ID{types.NilID},
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		txnDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transaction")
			d.AddDateField("Date", "13/45/2024")
			d.AddTextField("Payee", "", "", 0)
			d.AddComboField("Category", []string{"(None)"}, 0)
			d.AddTextField("Amount", "", "", 12)
			d.AddTextField("Memo", "", "", 0)
			d.AddRadioField("Status", []string{"Pending", "Cleared"}, 0)
			d.AddCheckboxField("Split transaction", false)
			d.SetVisible(true)
			return d
		}(),
		txnDialogData:        &transactionDialogData{payeeMap: make(map[string]*payee.Payee)},
		txnDialogCategoryIDs: []types.ID{types.NilID},
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
	accountID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		txnDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transaction")
			d.AddDateField("Date", "01/15/2024")
			d.AddTextField("Payee", "Coffee Shop", "", 0)
			d.AddComboField("Category", []string{"(None)", "Food"}, 0)
			d.AddTextField("Amount", "-5.00", "", 12)
			d.AddTextField("Memo", "Morning coffee", "", 0)
			d.AddRadioField("Status", []string{"Pending", "Cleared"}, 0)
			d.AddCheckboxField("Split transaction", false)
			d.SetVisible(true)
			return d
		}(),
		txnDialogData: &transactionDialogData{
			payeeMap: make(map[string]*payee.Payee),
		},
		txnDialogCategoryIDs: []types.ID{types.NilID, types.NewID()},
	}

	app.sidebar.SetAccounts([]*account.Account{
		{BaseModel: types.BaseModel{ID: accountID}, Name: "Checking", Active: true, Type: account.TypeChecking},
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
		txnDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transaction")
			d.SetVisible(true)
			return d
		}(),
		txnDialogData:        &transactionDialogData{payeeMap: make(map[string]*payee.Payee)},
		txnDialogCategoryIDs: []types.ID{types.NilID},
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
	catID := types.NewID()
	app := &App{
		txnDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transaction")
			d.AddDateField("Date", "01/01/2024")
			d.AddTextField("Payee", "", "", 0)
			d.AddComboField("Category", []string{"(None)", "Food"}, 0)
			d.AddTextField("Amount", "", "", 12)
			d.AddTextField("Memo", "", "", 0)
			d.AddRadioField("Status", []string{"Pending", "Cleared"}, 0)
			d.AddCheckboxField("Split transaction", false)
			d.SetVisible(true)
			return d
		}(),
		txnDialogData: &transactionDialogData{
			payeeMap: map[string]*payee.Payee{
				"coffee shop": {
					BaseModel:         types.BaseModel{ID: types.NewID()},
					Name:              "Coffee Shop",
					DefaultCategoryID: types.NullableID{ID: catID, Valid: true},
				},
			},
		},
		txnDialogCategoryIDs: []types.ID{types.NilID, catID},
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
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
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	model, cmd := app.handleTransactionDialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model != app {
		t.Error("should return same app when dialog is nil")
	}
	if cmd != nil {
		t.Error("should return nil cmd when dialog is nil")
	}
}

func TestBuildCategoryOptions_Sorted(t *testing.T) {
	categories := []*category.Category{
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Utilities", Type: category.TypeExpense},
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Food", Type: category.TypeExpense},
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Housing", Type: category.TypeExpense},
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
	catID := types.NewID()
	categories := []*category.Category{
		{BaseModel: types.BaseModel{ID: catID}, Name: "Food", Type: category.TypeExpense},
	}

	options, ids := buildCategoryOptions(categories)
	if len(options) != 2 || len(ids) != 2 {
		t.Fatalf("Expected 2 options and 2 IDs, got %d and %d", len(options), len(ids))
	}
	if ids[0] != types.NilID {
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
	expected := types.MustNewMoney("-50")
	if !amount.Equal(expected) {
		t.Errorf("parseAmountInput($-50.00) = %s, want %s", amount.String(), expected.String())
	}
}

func TestApp_RenderLayout_WithTransactionDialog(t *testing.T) {
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
		txnDialog: func() *dialog.Dialog {
			d := dialog.NewDialog("New Transaction")
			d.AddDateField("Date", "01/01/2024")
			d.SetVisible(true)
			return d
		}(),
	}

	output := app.renderLayout()
	if !strings.Contains(output, "New Transaction") {
		t.Error("renderLayout() should contain 'New Transaction' when dialog is visible")
	}
}

// =============================================================================
// TD-008 — transaction-dialog → create-category sub-dialog → back
// =============================================================================

// newAppForTxnAddNew builds an *App with a transaction dialog whose Category
// field is loaded with the provided options, focus on Category, and a typed
// query. The categorySvc passed in (may be nil for tests that don't submit)
// is wired so the createCategoryRequestMsg path can persist.
func newAppForTxnAddNew(t *testing.T, query string, categorySvc *category.Service, cats []*category.Category) *App {
	t.Helper()
	options, ids := buildCategoryOptions(cats)
	app := &App{
		currentView:          ViewRegister,
		keys:                 defaultKeyMap(),
		menubar:              widget.NewMenuBar(),
		statusbar:            widget.NewStatusBar(),
		sidebar:              NewSidebar(),
		categorySvc:          categorySvc,
		txnDialogData:        &transactionDialogData{categories: cats, payeeMap: make(map[string]*payee.Payee)},
		txnDialogCategoryIDs: ids,
	}
	d := buildTransactionDialog(app.txnDialogData, options, ids, types.ZeroDate)
	// Capture distinctive state we expect to see preserved across the divert.
	d.Fields()[0].Value = "03/15/2024"
	d.Fields()[1].Value = "Coffee Shop"
	d.Fields()[3].Value = "-9.50"
	d.Fields()[4].Value = "Latte"
	d.SetFocusIndex(2) // Category
	cat := d.Fields()[2]
	cat.Query = query
	// Park the combo highlight on the AddNew action row so Enter triggers
	// the divert. The row sits at len(filteredIndices) — this is what the
	// user reaches by pressing Down past the last filtered match.
	cat.ComboHighlight = len(cat.FilteredIndices())
	app.txnDialog = d
	return app
}

func TestApp_TxnDialog_AddNew_OpensCreateCategoryDialog(t *testing.T) {
	app := newAppForTxnAddNew(t, "Donations", nil, nil)

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handleTransactionDialogKey(enter)
	updated := model.(*App)

	if updated.createCatDialog == nil || !updated.createCatDialog.IsVisible() {
		t.Fatal("createCatDialog should be visible after [+ Add new] is activated")
	}
	if updated.txnDialog == nil {
		t.Fatal("txnDialog should be kept (hidden) so its state survives the divert")
	}
	if updated.txnDialog.IsVisible() {
		t.Error("txnDialog should be hidden while createCatDialog is shown")
	}
	// The pre-fill is wired in TD-009; for TD-008 we assert the dialog opened.
	if updated.createCatDialog.Title() != "New Category" {
		t.Errorf("createCatDialog title = %q, want %q",
			updated.createCatDialog.Title(), "New Category")
	}
}

func TestApp_TxnDialog_AddNew_CancelRestoresState(t *testing.T) {
	app := newAppForTxnAddNew(t, "", nil, nil)
	prevCat := app.txnDialog.Fields()[2].SelectedIndex
	prevFocus := app.txnDialog.FocusIndex()

	// Open create-category dialog.
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handleTransactionDialogKey(enter)
	app = model.(*App)

	// Cancel via Esc.
	esc := tea.KeyPressMsg{Code: tea.KeyEsc}
	model, _ = app.handleCreateCatDialogKey(esc)
	app = model.(*App)

	if app.createCatDialog != nil {
		t.Error("createCatDialog should be cleared after cancel")
	}
	if app.txnDialog == nil || !app.txnDialog.IsVisible() {
		t.Fatal("txnDialog should be restored to visible after cancel")
	}

	// All previous field values preserved.
	fields := app.txnDialog.Fields()
	if fields[0].Value != "03/15/2024" {
		t.Errorf("Date preserved? got %q, want %q", fields[0].Value, "03/15/2024")
	}
	if fields[1].Value != "Coffee Shop" {
		t.Errorf("Payee preserved? got %q, want %q", fields[1].Value, "Coffee Shop")
	}
	if fields[3].Value != "-9.50" {
		t.Errorf("Amount preserved? got %q, want %q", fields[3].Value, "-9.50")
	}
	if fields[4].Value != "Latte" {
		t.Errorf("Memo preserved? got %q, want %q", fields[4].Value, "Latte")
	}
	if fields[2].SelectedIndex != prevCat {
		t.Errorf("Category SelectedIndex changed on cancel: got %d, want %d",
			fields[2].SelectedIndex, prevCat)
	}
	if app.txnDialog.FocusIndex() != prevFocus {
		t.Errorf("FocusIndex changed on cancel: got %d, want %d (Category)",
			app.txnDialog.FocusIndex(), prevFocus)
	}
}

func TestApp_TxnDialog_AddNew_SubmitPersistsAndAdvancesFocus(t *testing.T) {
	database := dbtest.New(t)

	repo := category.NewRepository(database)
	svc := category.NewService(repo, database)
	if err := svc.SeedDefaultCategories(); err != nil {
		t.Fatalf("SeedDefaultCategories: %v", err)
	}
	cats, err := svc.List()
	if err != nil {
		t.Fatalf("svc.List: %v", err)
	}

	app := newAppForTxnAddNew(t, "", svc, cats)
	// Sanity: there should be a "Food" parent already (it's part of the
	// seeded defaults). The new category we add will land under it.
	var hasFood bool
	for _, c := range cats {
		if c.Name == "Food" && c.IsTopLevel() {
			hasFood = true
			break
		}
	}
	if !hasFood {
		t.Fatal("seeded defaults should include 'Food' parent")
	}

	// Open create-category sub-dialog.
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handleTransactionDialogKey(enter)
	app = model.(*App)
	if app.createCatDialog == nil {
		t.Fatal("createCatDialog should be open")
	}

	// Fill out: Name=Sushi, Parent=Food (existing), Type=Expense.
	cFields := app.createCatDialog.Fields()
	cFields[0].Value = "Sushi"
	// Parent: locate "Food" in the Options.
	parentField := cFields[1]
	foodIdx := -1
	for i, opt := range parentField.Options {
		if opt == "Food" {
			foodIdx = i
			break
		}
	}
	if foodIdx <= 0 {
		t.Fatalf("Parent options missing 'Food': %v", parentField.Options)
	}
	parentField.SelectedIndex = foodIdx
	cFields[2].SelectedIndex = 0 // Expense

	// Submit triggers categorySvc.Create + dialog reopen.
	model, cmd := app.submitCreateCatDialog()
	app = model.(*App)
	if cmd == nil {
		t.Fatal("submit should produce a tea.Cmd that emits createCategoryRequestMsg")
	}
	msg := cmd()
	model, _ = app.Update(msg)
	app = model.(*App)

	// New category persisted.
	got, err := svc.List()
	if err != nil {
		t.Fatalf("svc.List after submit: %v", err)
	}
	var found *category.Category
	for _, c := range got {
		if c.Name == "Sushi" {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("new 'Sushi' category should be persisted after submit")
	}
	if !found.ParentID.Valid {
		t.Error("'Sushi' should be a subcategory under 'Food'")
	}

	// Sub-dialog closed; transaction dialog visible again.
	if app.createCatDialog != nil {
		t.Error("createCatDialog should be cleared after submit")
	}
	if app.txnDialog == nil || !app.txnDialog.IsVisible() {
		t.Fatal("txnDialog should be visible again after submit")
	}

	// Other fields preserved.
	fields := app.txnDialog.Fields()
	if fields[0].Value != "03/15/2024" {
		t.Errorf("Date preserved? got %q", fields[0].Value)
	}
	if fields[1].Value != "Coffee Shop" {
		t.Errorf("Payee preserved? got %q", fields[1].Value)
	}
	if fields[3].Value != "-9.50" {
		t.Errorf("Amount preserved? got %q", fields[3].Value)
	}
	if fields[4].Value != "Latte" {
		t.Errorf("Memo preserved? got %q", fields[4].Value)
	}

	// Category field's SelectedIndex points to the new category.
	catField := fields[2]
	wantDisplay := "Food > Sushi"
	if catField.Options[catField.SelectedIndex] != wantDisplay {
		t.Errorf("Category selected = %q, want %q",
			catField.Options[catField.SelectedIndex], wantDisplay)
	}
	// And the parallel ID slice resolves to the new category's ID.
	if app.txnDialogCategoryIDs[catField.SelectedIndex] != found.ID {
		t.Errorf("txnDialogCategoryIDs[%d] = %s, want %s",
			catField.SelectedIndex,
			app.txnDialogCategoryIDs[catField.SelectedIndex],
			found.ID)
	}

	// Focus advances to Amount (index 3).
	if app.txnDialog.FocusIndex() != 3 {
		t.Errorf("FocusIndex after submit = %d, want 3 (Amount)",
			app.txnDialog.FocusIndex())
	}
}

func TestApp_TxnDialog_AddNew_SubmitNewParentCreatesBoth(t *testing.T) {
	database := dbtest.New(t)

	repo := category.NewRepository(database)
	svc := category.NewService(repo, database)
	if err := svc.SeedDefaultCategories(); err != nil {
		t.Fatalf("SeedDefaultCategories: %v", err)
	}
	cats, _ := svc.List()

	app := newAppForTxnAddNew(t, "", svc, cats)

	// Divert into create-category.
	model, _ := app.handleTransactionDialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = model.(*App)

	cFields := app.createCatDialog.Fields()
	cFields[0].Value = "Endowment"
	// Parent typed but not yet committed — Charity does not exist.
	cFields[1].Query = "Charity"
	cFields[2].SelectedIndex = 0 // Expense

	model, cmd := app.submitCreateCatDialog()
	app = model.(*App)
	if cmd == nil {
		t.Fatal("submit should produce a cmd")
	}
	app.Update(cmd())

	// Both Charity (parent) and Endowment (child) persisted.
	got, _ := svc.List()
	var charity, endow *category.Category
	for _, c := range got {
		switch c.Name {
		case "Charity":
			charity = c
		case "Endowment":
			endow = c
		}
	}
	if charity == nil {
		t.Fatal("new top-level 'Charity' should be persisted")
	}
	if charity.ParentID.Valid {
		t.Error("'Charity' should be top-level (no parent)")
	}
	if endow == nil {
		t.Fatal("new 'Endowment' should be persisted")
	}
	if !endow.ParentID.Valid || endow.ParentID.ID != charity.ID {
		t.Errorf("'Endowment' should be a child of 'Charity'")
	}
}

func TestApp_TxnDialog_AddNew_SubmitInvalidLeavesDialogOpen(t *testing.T) {
	app := newAppForTxnAddNew(t, "", nil, nil)
	model, _ := app.handleTransactionDialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = model.(*App)

	// Submit with empty Name — validation should keep the dialog open.
	model, cmd := app.submitCreateCatDialog()
	app = model.(*App)
	if cmd != nil {
		t.Error("invalid input should not produce a cmd")
	}
	if app.createCatDialog == nil || !app.createCatDialog.IsVisible() {
		t.Fatal("createCatDialog should remain open after validation failure")
	}
	if app.createCatDialog.Fields()[0].Error == "" {
		t.Error("Name field should have an inline error")
	}
}

// =============================================================================
// TD-009 — Pre-fill new-category Name from category-field query
// =============================================================================

func TestSplitCategoryQuery(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantParent string
		wantChild  string
	}{
		{"empty", "", "", ""},
		{"plain name", "Donations", "", "Donations"},
		{"parent and child", "Food:Sushi", "Food", "Sushi"},
		{"leading colon", ":Groceries", "", "Groceries"},
		{"trailing colon", "Food:", "Food", ""},
		{"whitespace trimmed", "  Food : Sushi  ", "Food", "Sushi"},
		{"only colon", ":", "", ""},
		{"first colon splits", "Food:Sushi:Spicy", "Food", "Sushi:Spicy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotParent, gotChild := splitCategoryQuery(tt.input)
			if gotParent != tt.wantParent {
				t.Errorf("parent = %q, want %q", gotParent, tt.wantParent)
			}
			if gotChild != tt.wantChild {
				t.Errorf("child = %q, want %q", gotChild, tt.wantChild)
			}
		})
	}
}

func TestApp_TxnDialog_AddNew_PrefillsNameFromQuery(t *testing.T) {
	// Combo query "Donations" → create-category dialog opens with
	// Name=Donations, Parent empty, focus on Parent.
	app := newAppForTxnAddNew(t, "Donations", nil, nil)

	model, _ := app.handleTransactionDialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = model.(*App)

	if app.createCatDialog == nil {
		t.Fatal("createCatDialog should be open")
	}
	fields := app.createCatDialog.Fields()
	if fields[0].Value != "Donations" {
		t.Errorf("Name = %q, want %q", fields[0].Value, "Donations")
	}
	// Parent: no query, no selection beyond top-level sentinel.
	if fields[1].Query != "" {
		t.Errorf("Parent.Query = %q, want empty", fields[1].Query)
	}
	if fields[1].SelectedIndex != 0 {
		t.Errorf("Parent.SelectedIndex = %d, want 0 (top-level)", fields[1].SelectedIndex)
	}
	if app.createCatDialog.FocusIndex() != 1 {
		t.Errorf("FocusIndex = %d, want 1 (Parent)", app.createCatDialog.FocusIndex())
	}
}

func TestApp_TxnDialog_AddNew_PrefillsParentChildFromColonExisting(t *testing.T) {
	// Combo query "Food:Sushi" with "Food" as an existing parent →
	// Name=Sushi, Parent SelectedIndex resolves to Food (no new-parent flag).
	cats := []*category.Category{
		category.NewCategory("Food", category.TypeExpense),
		category.NewCategory("Bills", category.TypeExpense),
	}
	app := newAppForTxnAddNew(t, "Food:Sushi", nil, cats)

	model, _ := app.handleTransactionDialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = model.(*App)

	if app.createCatDialog == nil {
		t.Fatal("createCatDialog should be open")
	}
	fields := app.createCatDialog.Fields()
	if fields[0].Value != "Sushi" {
		t.Errorf("Name = %q, want %q", fields[0].Value, "Sushi")
	}
	// Parent should resolve to "Food" via SelectedIndex, not Query.
	if fields[1].Query != "" {
		t.Errorf("Parent.Query = %q, want empty (existing parent resolves to SelectedIndex)", fields[1].Query)
	}
	wantIdx := -1
	for i, opt := range fields[1].Options {
		if opt == "Food" {
			wantIdx = i
			break
		}
	}
	if wantIdx <= 0 {
		t.Fatalf("Parent options should include 'Food': %v", fields[1].Options)
	}
	if fields[1].SelectedIndex != wantIdx {
		t.Errorf("Parent.SelectedIndex = %d, want %d (Food)", fields[1].SelectedIndex, wantIdx)
	}
}

func TestApp_TxnDialog_AddNew_PrefillsParentChildFromColonNew(t *testing.T) {
	// Combo query "Charity:Endowment" where "Charity" is NOT an existing
	// parent → Name=Endowment, Parent.Query="Charity" (new-parent path).
	cats := []*category.Category{
		category.NewCategory("Food", category.TypeExpense),
	}
	app := newAppForTxnAddNew(t, "Charity:Endowment", nil, cats)

	model, _ := app.handleTransactionDialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = model.(*App)

	if app.createCatDialog == nil {
		t.Fatal("createCatDialog should be open")
	}
	fields := app.createCatDialog.Fields()
	if fields[0].Value != "Endowment" {
		t.Errorf("Name = %q, want %q", fields[0].Value, "Endowment")
	}
	if fields[1].Query != "Charity" {
		t.Errorf("Parent.Query = %q, want %q (new-parent path)", fields[1].Query, "Charity")
	}
}

func TestApp_TxnDialog_AddNew_PrefillsEmptyQuery(t *testing.T) {
	// Combo query empty → all fields empty, focus on Name.
	app := newAppForTxnAddNew(t, "", nil, nil)

	model, _ := app.handleTransactionDialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = model.(*App)

	if app.createCatDialog == nil {
		t.Fatal("createCatDialog should be open")
	}
	fields := app.createCatDialog.Fields()
	if fields[0].Value != "" {
		t.Errorf("Name = %q, want empty", fields[0].Value)
	}
	if fields[1].Query != "" {
		t.Errorf("Parent.Query = %q, want empty", fields[1].Query)
	}
	if fields[1].SelectedIndex != 0 {
		t.Errorf("Parent.SelectedIndex = %d, want 0", fields[1].SelectedIndex)
	}
	if app.createCatDialog.FocusIndex() != 0 {
		t.Errorf("FocusIndex = %d, want 0 (Name)", app.createCatDialog.FocusIndex())
	}
}

func TestApp_TxnDialog_AddNew_PrefillsLeadingColon(t *testing.T) {
	// Combo query ":Groceries" → Name=Groceries, Parent empty (treat as
	// malformed, same as empty parent).
	app := newAppForTxnAddNew(t, ":Groceries", nil, nil)

	model, _ := app.handleTransactionDialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = model.(*App)

	if app.createCatDialog == nil {
		t.Fatal("createCatDialog should be open")
	}
	fields := app.createCatDialog.Fields()
	if fields[0].Value != "Groceries" {
		t.Errorf("Name = %q, want %q", fields[0].Value, "Groceries")
	}
	if fields[1].Query != "" {
		t.Errorf("Parent.Query = %q, want empty", fields[1].Query)
	}
	if fields[1].SelectedIndex != 0 {
		t.Errorf("Parent.SelectedIndex = %d, want 0 (top-level)", fields[1].SelectedIndex)
	}
}

// =============================================================================
// Edit-mode tests (Phase 1: plain transaction edit)
// =============================================================================

// TestBuildTransactionDialog_EditMode_PrefillsFromExisting asserts that when
// transactionDialogData has mode=edit and an existing transaction, the dialog
// is built titled "Edit Transaction" and every field is pre-filled from the
// existing transaction's values: Date, Payee (resolved from data.payees),
// Category (resolved index in categoryIDs), Amount, Memo, Status.
func TestBuildTransactionDialog_EditMode_PrefillsFromExisting(t *testing.T) {
	parentID := types.NewID()
	groceriesID := types.NewID()
	otherID := types.NewID()
	payeeID := types.NewID()
	otherPayeeID := types.NewID()

	cats := []*category.Category{
		{BaseModel: types.BaseModel{ID: parentID}, Name: "Food", Type: category.TypeExpense},
		{BaseModel: types.BaseModel{ID: groceriesID}, Name: "Groceries",
			ParentID: types.NullableID{ID: parentID, Valid: true}, Type: category.TypeExpense},
		{BaseModel: types.BaseModel{ID: otherID}, Name: "Auto", Type: category.TypeExpense},
	}
	options, ids := buildCategoryOptions(cats)

	pys := []*payee.Payee{
		{BaseModel: types.BaseModel{ID: otherPayeeID}, Name: "Shell"},
		{BaseModel: types.BaseModel{ID: payeeID}, Name: "Kroger"},
	}

	existing := transaction.NewTransactionFull(
		types.NewID(),
		types.NewDate(2024, time.March, 15),
		types.MustNewMoney("-125.43"),
		payeeID, groceriesID, "Weekly groceries",
	)
	existing.Status = transaction.StatusCleared

	data := &transactionDialogData{
		payees:     pys,
		categories: cats,
		mode:       transactionDialogModeEdit,
		existing:   existing,
	}

	d := buildTransactionDialog(data, options, ids, types.ZeroDate)

	if d.Title() != "Edit Transaction" {
		t.Errorf("title = %q, want %q", d.Title(), "Edit Transaction")
	}

	fields := d.Fields()
	if len(fields) < 7 {
		t.Fatalf("expected 7 fields, got %d", len(fields))
	}

	// Date pre-filled from existing.Date
	if fields[0].Value != "03/15/2024" {
		t.Errorf("date = %q, want %q", fields[0].Value, "03/15/2024")
	}

	// Payee pre-filled from data.payees[payeeID].Name
	if fields[1].Value != "Kroger" {
		t.Errorf("payee = %q, want %q", fields[1].Value, "Kroger")
	}

	// Category combo SelectedIndex matches "Food > Groceries"
	wantIdx := -1
	for i, id := range ids {
		if id == groceriesID {
			wantIdx = i
			break
		}
	}
	if wantIdx < 0 {
		t.Fatal("groceries category not in ids slice")
	}
	if fields[2].SelectedIndex != wantIdx {
		t.Errorf("category SelectedIndex = %d, want %d (Food > Groceries)",
			fields[2].SelectedIndex, wantIdx)
	}

	// Amount pre-filled
	if fields[3].Value != "-125.43" {
		t.Errorf("amount = %q, want %q", fields[3].Value, "-125.43")
	}

	// Memo pre-filled
	if fields[4].Value != "Weekly groceries" {
		t.Errorf("memo = %q, want %q", fields[4].Value, "Weekly groceries")
	}

	// Status: Cleared = index 1
	if fields[5].SelectedIndex != 1 {
		t.Errorf("status SelectedIndex = %d, want 1 (Cleared)", fields[5].SelectedIndex)
	}

	// Split checkbox: unchecked (Phase 1 — splits handled separately later)
	if fields[6].Checked {
		t.Error("split checkbox should be unchecked in edit mode for plain transactions")
	}
}

// TestBuildTransactionDialog_EditMode_NoPayeeOrCategory asserts edit-mode
// pre-fill leaves payee blank and category at "(None)" when the existing
// transaction has neither set.
func TestBuildTransactionDialog_EditMode_NoPayeeOrCategory(t *testing.T) {
	cats := []*category.Category{
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Auto", Type: category.TypeExpense},
	}
	options, ids := buildCategoryOptions(cats)

	existing := transaction.NewTransaction(
		types.NewID(),
		types.NewDate(2024, time.March, 15),
		types.MustNewMoney("-10.00"),
	)

	data := &transactionDialogData{
		categories: cats,
		mode:       transactionDialogModeEdit,
		existing:   existing,
	}

	d := buildTransactionDialog(data, options, ids, types.ZeroDate)
	fields := d.Fields()

	if fields[1].Value != "" {
		t.Errorf("payee = %q, want empty", fields[1].Value)
	}
	if fields[2].SelectedIndex != 0 {
		t.Errorf("category SelectedIndex = %d, want 0 (None)", fields[2].SelectedIndex)
	}
	if fields[4].Value != "" {
		t.Errorf("memo = %q, want empty", fields[4].Value)
	}
}

// TestApp_HandleRegisterKeys_EnterOpensEditFlow_ForPlainTransaction asserts
// that pressing Enter on a plain (non-transfer, non-void, non-reconciled)
// transaction in the register returns a non-nil cmd — the loader cmd that
// will fetch payees+categories+the transaction and emit a
// transactionDialogDataMsg in edit mode.
func TestApp_HandleRegisterKeys_EnterOpensEditFlow_ForPlainTransaction(t *testing.T) {
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
			transactions: []*transaction.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Date:      types.Today(),
					Amount:    types.MustNewMoney("-25"),
					Status:    transaction.StatusUncleared,
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
		t.Error("Enter on a plain transaction should return a non-nil load cmd")
	}
}

// TestApp_HandleRegisterKeys_EnterOnVoidTransaction_NoOp asserts Enter on a
// void transaction does not open the edit flow (no cmd) and surfaces a
// notification on the status bar.
func TestApp_HandleRegisterKeys_EnterOnVoidTransaction_NoOp(t *testing.T) {
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
			transactions: []*transaction.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Date:      types.Today(),
					Amount:    types.ZeroMoney,
					Status:    transaction.StatusVoid,
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

	if cmd != nil {
		t.Error("Enter on a void transaction should not return a cmd")
	}
	if app.txnDialog != nil {
		t.Error("Enter on a void transaction should not open the txn dialog")
	}
}

// TestApp_HandleRegisterKeys_EnterOnReconciledTransaction_NoOp asserts Enter
// on a reconciled transaction does not open the edit flow.
func TestApp_HandleRegisterKeys_EnterOnReconciledTransaction_NoOp(t *testing.T) {
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
			transactions: []*transaction.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Date:      types.Today(),
					Amount:    types.MustNewMoney("-25"),
					Status:    transaction.StatusReconciled,
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

	if cmd != nil {
		t.Error("Enter on a reconciled transaction should not return a cmd")
	}
	if app.txnDialog != nil {
		t.Error("Enter on a reconciled transaction should not open the txn dialog")
	}
}

// TestApp_Update_TransactionDialogDataMsg_EditMode asserts feeding a
// transactionDialogDataMsg whose data is in edit mode results in the txn
// dialog being constructed in edit mode (title "Edit Transaction") and with
// fields pre-filled from the existing transaction.
func TestApp_Update_TransactionDialogDataMsg_EditMode(t *testing.T) {
	groceriesID := types.NewID()
	parentID := types.NewID()
	payeeID := types.NewID()

	cats := []*category.Category{
		{BaseModel: types.BaseModel{ID: parentID}, Name: "Food", Type: category.TypeExpense},
		{BaseModel: types.BaseModel{ID: groceriesID}, Name: "Groceries",
			ParentID: types.NullableID{ID: parentID, Valid: true}, Type: category.TypeExpense},
	}

	existing := transaction.NewTransactionFull(
		types.NewID(),
		types.NewDate(2024, time.March, 15),
		types.MustNewMoney("-50.00"),
		payeeID, groceriesID, "memo",
	)
	existing.Status = transaction.StatusCleared

	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	data := &transactionDialogData{
		payees:     []*payee.Payee{{BaseModel: types.BaseModel{ID: payeeID}, Name: "Kroger"}},
		categories: cats,
		payeeMap:   map[string]*payee.Payee{"kroger": {BaseModel: types.BaseModel{ID: payeeID}, Name: "Kroger"}},
		mode:       transactionDialogModeEdit,
		existing:   existing,
	}

	model, _ := app.Update(transactionDialogDataMsg{data: data})
	updatedApp := model.(*App)

	if updatedApp.txnDialog == nil {
		t.Fatal("txnDialog should be set after edit-mode data msg")
	}
	if updatedApp.txnDialog.Title() != "Edit Transaction" {
		t.Errorf("title = %q, want %q", updatedApp.txnDialog.Title(), "Edit Transaction")
	}

	fields := updatedApp.txnDialog.Fields()
	if fields[0].Value != "03/15/2024" {
		t.Errorf("date = %q, want %q", fields[0].Value, "03/15/2024")
	}
	if fields[1].Value != "Kroger" {
		t.Errorf("payee = %q, want %q", fields[1].Value, "Kroger")
	}
	if fields[3].Value != "-50" {
		t.Errorf("amount = %q, want %q", fields[3].Value, "-50")
	}
	if fields[4].Value != "memo" {
		t.Errorf("memo = %q, want %q", fields[4].Value, "memo")
	}
	if fields[5].SelectedIndex != 1 {
		t.Errorf("status = %d, want 1 (Cleared)", fields[5].SelectedIndex)
	}
}
