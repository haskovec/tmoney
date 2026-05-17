package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// Pure Function Tests - SplitDialog
// =============================================================================

func TestNewSplitDialog(t *testing.T) {
	amount := types.MustNewMoney("-150.00")
	catOptions := []string{"(None)", "Food", "Household"}
	catIDs := []types.ID{types.NilID, types.NewID(), types.NewID()}

	sd := NewSplitDialog(amount, catOptions, catIDs)

	if !sd.IsVisible() {
		t.Error("should be visible after creation")
	}
	if len(sd.Rows()) != 1 {
		t.Errorf("expected 1 initial row, got %d", len(sd.Rows()))
	}
	if sd.Focus() != splitFocusRows {
		t.Errorf("focus = %d, want splitFocusRows", sd.Focus())
	}
	if sd.RowIndex() != 0 {
		t.Errorf("rowIndex = %d, want 0", sd.RowIndex())
	}
	if sd.FieldFocus() != splitFieldCategory {
		t.Errorf("fieldFocus = %d, want splitFieldCategory", sd.FieldFocus())
	}
}

func TestSplitDialog_Remaining_NoAmounts(t *testing.T) {
	amount := types.MustNewMoney("-100.00")
	sd := NewSplitDialog(amount, []string{"(None)"}, []types.ID{types.NilID})

	rem := sd.remaining()
	if !rem.Equal(amount) {
		t.Errorf("remaining = %s, want %s", rem.String(), amount.String())
	}
}

func TestSplitDialog_Remaining_Partial(t *testing.T) {
	amount := types.MustNewMoney("-100.00")
	sd := NewSplitDialog(amount, []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})

	sd.rows[0].amountField.Value = "-60.00"

	rem := sd.remaining()
	expected := types.MustNewMoney("-40.00")
	if !rem.Equal(expected) {
		t.Errorf("remaining = %s, want %s", rem.String(), expected.String())
	}
}

func TestSplitDialog_Remaining_Exact(t *testing.T) {
	amount := types.MustNewMoney("-100.00")
	sd := NewSplitDialog(amount, []string{"(None)", "Food", "Household"}, []types.ID{types.NilID, types.NewID(), types.NewID()})

	sd.addRow()
	sd.rows[0].amountField.Value = "-60.00"
	sd.rows[1].amountField.Value = "-40.00"

	rem := sd.remaining()
	if !rem.IsZero() {
		t.Errorf("remaining = %s, want 0", rem.String())
	}
}

func TestSplitDialog_Remaining_OverAllocated(t *testing.T) {
	amount := types.MustNewMoney("-100.00")
	sd := NewSplitDialog(amount, []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})

	sd.addRow()
	sd.rows[0].amountField.Value = "-80.00"
	sd.rows[1].amountField.Value = "-40.00"

	rem := sd.remaining()
	expected := types.MustNewMoney("20.00")
	if !rem.Equal(expected) {
		t.Errorf("remaining = %s, want %s", rem.String(), expected.String())
	}
}

func TestSplitDialog_AddRow(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})

	if len(sd.rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(sd.rows))
	}

	sd.addRow()
	if len(sd.rows) != 2 {
		t.Errorf("expected 2 rows after addRow, got %d", len(sd.rows))
	}
}

func TestSplitDialog_RemoveRow(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})
	sd.addRow()
	sd.addRow()

	if len(sd.rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(sd.rows))
	}

	sd.removeRow(1)
	if len(sd.rows) != 2 {
		t.Errorf("expected 2 rows after remove, got %d", len(sd.rows))
	}
}

func TestSplitDialog_RemoveRow_MinimumOne(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})

	sd.removeRow(0)
	if len(sd.rows) != 1 {
		t.Errorf("should not remove last row, got %d rows", len(sd.rows))
	}
}

func TestSplitDialog_RemoveRow_AdjustsFocus(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})
	sd.addRow()
	sd.rowIndex = 1

	sd.removeRow(1)
	if sd.rowIndex != 0 {
		t.Errorf("rowIndex = %d, want 0 after removing focused row", sd.rowIndex)
	}
}

func TestSplitDialog_Validate_Valid(t *testing.T) {
	catIDs := []types.ID{types.NilID, types.NewID(), types.NewID()}
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food", "Household"}, catIDs)
	sd.addRow()

	sd.rows[0].categoryIndex = 1
	sd.rows[0].amountField.Value = "-60.00"
	sd.rows[1].categoryIndex = 2
	sd.rows[1].amountField.Value = "-40.00"

	err := sd.validate()
	if err != nil {
		t.Errorf("expected valid, got error: %v", err)
	}
}

func TestSplitDialog_Validate_MissingCategory(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})

	sd.rows[0].categoryIndex = 0 // (None)
	sd.rows[0].amountField.Value = "-100.00"

	err := sd.validate()
	if err == nil {
		t.Error("expected error for missing category")
	}
	if !strings.Contains(err.Error(), "category is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSplitDialog_Validate_MissingAmount(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})

	sd.rows[0].categoryIndex = 1
	sd.rows[0].amountField.Value = ""

	err := sd.validate()
	if err == nil {
		t.Error("expected error for missing amount")
	}
	if !strings.Contains(err.Error(), "amount is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSplitDialog_Validate_SumMismatch(t *testing.T) {
	catIDs := []types.ID{types.NilID, types.NewID()}
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, catIDs)

	sd.rows[0].categoryIndex = 1
	sd.rows[0].amountField.Value = "-50.00"

	err := sd.validate()
	if err == nil {
		t.Error("expected error for sum mismatch")
	}
	if !strings.Contains(err.Error(), "must sum to") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSplitDialog_BuildSplits(t *testing.T) {
	catID1 := types.NewID()
	catID2 := types.NewID()
	catIDs := []types.ID{types.NilID, catID1, catID2}
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food", "Household"}, catIDs)
	sd.addRow()

	sd.rows[0].categoryIndex = 1
	sd.rows[0].amountField.Value = "-60.00"
	sd.rows[0].memoField.Value = "Groceries"
	sd.rows[1].categoryIndex = 2
	sd.rows[1].amountField.Value = "-40.00"

	splits, err := sd.buildSplits()
	if err != nil {
		t.Fatalf("buildSplits error: %v", err)
	}

	if len(splits) != 2 {
		t.Fatalf("expected 2 splits, got %d", len(splits))
	}

	if splits[0].CategoryID != catID1 {
		t.Errorf("split[0] category = %s, want %s", splits[0].CategoryID.String(), catID1.String())
	}
	if !splits[0].Amount.Equal(types.MustNewMoney("-60.00")) {
		t.Errorf("split[0] amount = %s, want -60.00", splits[0].Amount.String())
	}
	if !splits[0].Memo.Valid || splits[0].Memo.String != "Groceries" {
		t.Errorf("split[0] memo = %v, want 'Groceries'", splits[0].Memo)
	}

	if splits[1].CategoryID != catID2 {
		t.Errorf("split[1] category = %s, want %s", splits[1].CategoryID.String(), catID2.String())
	}
	if !splits[1].Amount.Equal(types.MustNewMoney("-40.00")) {
		t.Errorf("split[1] amount = %s, want -40.00", splits[1].Amount.String())
	}
	if splits[1].Memo.Valid {
		t.Errorf("split[1] memo should be empty, got %v", splits[1].Memo)
	}
}

// =============================================================================
// HandleKey Tests
// =============================================================================

func TestSplitDialog_HandleKey_EscCancels(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})

	action := sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if action != DialogActionCancel {
		t.Errorf("Esc should return DialogActionCancel, got %d", action)
	}
}

func TestSplitDialog_HandleKey_TabCycles(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})

	// Start: rows, row 0, category
	if sd.focus != splitFocusRows || sd.fieldFocus != splitFieldCategory {
		t.Fatal("unexpected initial focus")
	}

	// Tab -> amount
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if sd.focus != splitFocusRows || sd.fieldFocus != splitFieldAmount {
		t.Errorf("after tab 1: focus=%d, field=%d", sd.focus, sd.fieldFocus)
	}

	// Tab -> memo
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if sd.focus != splitFocusRows || sd.fieldFocus != splitFieldMemo {
		t.Errorf("after tab 2: focus=%d, field=%d", sd.focus, sd.fieldFocus)
	}

	// Tab -> add button
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if sd.focus != splitFocusAddBtn {
		t.Errorf("after tab 3: focus=%d, want splitFocusAddBtn", sd.focus)
	}

	// Tab -> cancel button
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if sd.focus != splitFocusCancelBtn {
		t.Errorf("after tab 4: focus=%d, want splitFocusCancelBtn", sd.focus)
	}

	// Tab -> save button
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if sd.focus != splitFocusSaveBtn {
		t.Errorf("after tab 5: focus=%d, want splitFocusSaveBtn", sd.focus)
	}

	// Tab -> wrap to first row
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if sd.focus != splitFocusRows || sd.rowIndex != 0 || sd.fieldFocus != splitFieldCategory {
		t.Errorf("after tab 6: focus=%d, row=%d, field=%d", sd.focus, sd.rowIndex, sd.fieldFocus)
	}
}

func TestSplitDialog_HandleKey_ShiftTabCycles(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})

	// Start at rows, row 0, category
	// Shift-Tab should wrap to save button
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if sd.focus != splitFocusSaveBtn {
		t.Errorf("shift-tab from start: focus=%d, want splitFocusSaveBtn", sd.focus)
	}

	// Shift-Tab -> cancel
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if sd.focus != splitFocusCancelBtn {
		t.Errorf("shift-tab: focus=%d, want splitFocusCancelBtn", sd.focus)
	}

	// Shift-Tab -> add
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if sd.focus != splitFocusAddBtn {
		t.Errorf("shift-tab: focus=%d, want splitFocusAddBtn", sd.focus)
	}

	// Shift-Tab -> back to rows, last row, last field (memo)
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if sd.focus != splitFocusRows || sd.fieldFocus != splitFieldMemo {
		t.Errorf("shift-tab: focus=%d, field=%d", sd.focus, sd.fieldFocus)
	}
}

func TestSplitDialog_HandleKey_EnterOnSave_Valid(t *testing.T) {
	catIDs := []types.ID{types.NilID, types.NewID()}
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, catIDs)
	sd.rows[0].categoryIndex = 1
	sd.rows[0].amountField.Value = "-100.00"

	sd.focus = splitFocusSaveBtn
	action := sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != DialogActionSubmit {
		t.Errorf("Enter on Save with valid data should return DialogActionSubmit, got %d", action)
	}
}

func TestSplitDialog_HandleKey_EnterOnSave_Invalid(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})

	sd.focus = splitFocusSaveBtn
	action := sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != DialogActionNone {
		t.Errorf("Enter on Save with invalid data should return DialogActionNone, got %d", action)
	}
	if sd.errorMsg == "" {
		t.Error("expected error message to be set")
	}
}

func TestSplitDialog_HandleKey_EnterOnCancel(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})

	sd.focus = splitFocusCancelBtn
	action := sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != DialogActionCancel {
		t.Errorf("Enter on Cancel should return DialogActionCancel, got %d", action)
	}
}

func TestSplitDialog_HandleKey_EnterOnAdd(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})

	sd.focus = splitFocusAddBtn
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	if len(sd.rows) != 2 {
		t.Errorf("expected 2 rows after Enter on Add, got %d", len(sd.rows))
	}
	if sd.focus != splitFocusRows || sd.rowIndex != 1 {
		t.Errorf("focus should be on new row: focus=%d, row=%d", sd.focus, sd.rowIndex)
	}
}

func TestSplitDialog_HandleKey_CategoryUpDown(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food", "Household"}, []types.ID{types.NilID, types.NewID(), types.NewID()})

	if sd.rows[0].categoryIndex != 0 {
		t.Fatal("expected initial categoryIndex 0")
	}

	// Down -> Food
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if sd.rows[0].categoryIndex != 1 {
		t.Errorf("categoryIndex after down = %d, want 1", sd.rows[0].categoryIndex)
	}

	// Down -> Household
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if sd.rows[0].categoryIndex != 2 {
		t.Errorf("categoryIndex after down = %d, want 2", sd.rows[0].categoryIndex)
	}

	// Down -> Transfer → sentinel (appended after real categories)
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if sd.rows[0].categoryIndex != 3 {
		t.Errorf("categoryIndex after down = %d, want 3 (Transfer sentinel)", sd.rows[0].categoryIndex)
	}

	// Down at max (should stay at sentinel)
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if sd.rows[0].categoryIndex != 3 {
		t.Errorf("categoryIndex should stay at 3, got %d", sd.rows[0].categoryIndex)
	}

	// Up -> Household
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if sd.rows[0].categoryIndex != 2 {
		t.Errorf("categoryIndex after up = %d, want 2", sd.rows[0].categoryIndex)
	}
}

// TestSplitDialog_TransferSentinel_PresentInCategoryCombo asserts that
// the category combo for a split row exposes a `Transfer →` option as
// the trailing entry, reachable via Down navigation and rendered when
// selected. (MS-011 wires up the account-picker swap.)
func TestSplitDialog_TransferSentinel_PresentInCategoryCombo(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)
	sd := NewSplitDialog(types.MustNewMoney("-100.00"),
		[]string{"(None)", "Food"},
		[]types.ID{types.NilID, types.NewID()})

	// Navigate Down past every real category — should land on the
	// Transfer sentinel at index len(categoryOptions).
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown}) // -> Food
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown}) // -> Transfer →

	if sd.rows[0].categoryIndex != 2 {
		t.Fatalf("after two Down presses, categoryIndex = %d, want 2 (Transfer sentinel)", sd.rows[0].categoryIndex)
	}
	if !sd.isTransferSentinel(sd.rows[0].categoryIndex) {
		t.Errorf("expected categoryIndex %d to be the Transfer sentinel", sd.rows[0].categoryIndex)
	}

	out := sd.Render(styles)
	if !strings.Contains(out, "Transfer →") {
		t.Errorf("rendered dialog should display 'Transfer →' when the sentinel is selected; got:\n%s", out)
	}
}

func TestSplitDialog_HandleKey_TextEditing(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()})

	// Move to amount field
	sd.fieldFocus = splitFieldAmount

	// Type some characters
	sd.HandleKey(tea.KeyPressMsg{Code: '5', Text: "5"})
	sd.HandleKey(tea.KeyPressMsg{Code: '0', Text: "0"})

	if sd.rows[0].amountField.Value != "50" {
		t.Errorf("amount value = %q, want '50'", sd.rows[0].amountField.Value)
	}

	// Backspace
	sd.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if sd.rows[0].amountField.Value != "5" {
		t.Errorf("amount value after backspace = %q, want '5'", sd.rows[0].amountField.Value)
	}
}

func TestSplitDialog_HandleKey_CtrlD_RemovesRow(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})
	sd.addRow()

	if len(sd.rows) != 2 {
		t.Fatal("expected 2 rows")
	}

	sd.HandleKey(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if len(sd.rows) != 1 {
		t.Errorf("expected 1 row after Ctrl+D, got %d", len(sd.rows))
	}
}

func TestSplitDialog_HandleKey_CtrlD_NoRemoveLastRow(t *testing.T) {
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID})

	sd.HandleKey(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if len(sd.rows) != 1 {
		t.Errorf("should not remove last row, got %d", len(sd.rows))
	}
}

// =============================================================================
// Render Tests
// =============================================================================

func TestSplitDialog_Render(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)
	sd := NewSplitDialog(types.MustNewMoney("-150.00"), []string{"(None)", "Food", "Household"}, []types.ID{types.NilID, types.NewID(), types.NewID()})

	output := sd.Render(styles)

	checks := []string{
		"SPLIT TRANSACTION",
		"$150.00",
		"Remaining",
		"Category",
		"Amount",
		"Memo",
		"Add split",
		"Cancel",
		"Save",
	}

	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("render output should contain %q", check)
		}
	}
}

// =============================================================================
// App Integration Tests
// =============================================================================

func TestApp_SubmitTransactionDialog_SplitChecked(t *testing.T) {
	accountID := types.NewID()
	catID := types.NewID()

	app := &App{
		currentView: ViewRegister,
		width:       120,
		height:      30,
		styles:      NewStyles(),
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		txnDialog: func() *Dialog {
			d := NewDialog("New Transaction")
			d.AddTextField("Date", "01/15/2024", "", 10)
			d.AddTextField("Payee", "Grocery Store", "", 0)
			d.AddSelectField("Category", []string{"(None)", "Food"}, 1)
			d.AddTextField("Amount", "-150.00", "", 12)
			d.AddTextField("Memo", "", "", 0)
			d.AddRadioField("Status", []string{"Pending", "Cleared"}, 0)
			d.AddCheckboxField("Split transaction", true) // checked!
			d.SetVisible(true)
			return d
		}(),
		txnDialogData: &transactionDialogData{
			payeeMap: make(map[string]*payee.Payee),
			categories: []*category.Category{
				{
					BaseModel: types.BaseModel{ID: catID},
					Name:      "Food",
					Type:      category.TypeExpense,
				},
			},
		},
		txnDialogCategoryIDs: []types.ID{types.NilID, catID},
	}

	// Set up sidebar with a selected account
	app.sidebar.SetAccounts([]*account.Account{
		{BaseModel: types.BaseModel{ID: accountID}, Name: "Checking", Active: true, Type: account.TypeChecking},
	}, nil)

	// Submit the dialog (focus on Save button)
	app.txnDialog.SetFocusIndex(len(app.txnDialog.Fields()))
	app.txnDialog.FocusNext() // move to Save button

	model, cmd := app.submitTransactionDialog()
	updatedApp := model.(*App)

	// Transaction dialog should be closed
	if updatedApp.txnDialog != nil {
		t.Error("txnDialog should be nil after split submit")
	}

	// Split dialog should be open
	if updatedApp.splitDialog == nil {
		t.Fatal("splitDialog should be created when split is checked")
	}
	if !updatedApp.splitDialog.IsVisible() {
		t.Error("splitDialog should be visible")
	}

	// Pending transaction should be stored
	if updatedApp.pendingSplitTxn == nil {
		t.Fatal("pendingSplitTxn should be set")
	}
	if updatedApp.pendingSplitTxn.payeeName != "Grocery Store" {
		t.Errorf("pendingSplitTxn.payeeName = %q, want 'Grocery Store'", updatedApp.pendingSplitTxn.payeeName)
	}
	if !updatedApp.pendingSplitTxn.amount.Equal(types.MustNewMoney("-150.00")) {
		t.Errorf("pendingSplitTxn.amount = %s, want -150.00", updatedApp.pendingSplitTxn.amount.String())
	}

	// Should not return an async command (dialog opens synchronously)
	if cmd != nil {
		t.Error("split dialog opening should not return a cmd")
	}
}

func TestApp_HandleSplitDialogKey_Cancel(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		splitDialog: NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)"}, []types.ID{types.NilID}),
		pendingSplitTxn: &pendingSplitTransaction{
			amount: types.MustNewMoney("-100.00"),
		},
	}

	// Press Escape to cancel
	escKey := tea.KeyPressMsg{Code: tea.KeyEsc}
	model, _ := app.Update(escKey)
	updatedApp := model.(*App)

	if updatedApp.splitDialog != nil {
		t.Error("splitDialog should be nil after cancel")
	}
	if updatedApp.pendingSplitTxn != nil {
		t.Error("pendingSplitTxn should be nil after cancel")
	}
}

func TestApp_Update_SplitDialogSavedMsg(t *testing.T) {
	accountID := types.NewID()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}
	app.sidebar.SetAccounts([]*account.Account{
		{BaseModel: types.BaseModel{ID: accountID}, Name: "Checking", Active: true, Type: account.TypeChecking},
	}, nil)

	msg := splitDialogSavedMsg{}
	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Error("splitDialogSavedMsg should return a reload command")
	}
}

func TestApp_RenderLayout_WithSplitDialog(t *testing.T) {
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
		splitDialog: NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()}),
	}

	output := app.renderLayout()
	if !strings.Contains(output, "SPLIT TRANSACTION") {
		t.Error("renderLayout() should contain 'SPLIT TRANSACTION' when split dialog is visible")
	}
}

func TestApp_SplitDialogKeyRouting(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		width:       120,
		height:      30,
		ready:       true,
		styles:      NewStyles(),
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		splitDialog: NewSplitDialog(types.MustNewMoney("-100.00"), []string{"(None)", "Food"}, []types.ID{types.NilID, types.NewID()}),
		pendingSplitTxn: &pendingSplitTransaction{
			amount: types.MustNewMoney("-100.00"),
		},
	}

	// Tab key should be routed to split dialog, not to register view
	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	app.Update(tabKey)

	// After tab, split dialog should have advanced focus
	if app.splitDialog.fieldFocus != splitFieldAmount {
		t.Errorf("split dialog field focus should advance on Tab, got %d", app.splitDialog.fieldFocus)
	}
}

// =============================================================================
// Edit-mode tests (Phase 3: split transaction edit)
// =============================================================================

// TestNewSplitDialogFromExisting asserts that constructing a split dialog
// from an existing slice of splits seeds one row per split, with the
// matching category index pre-selected and the amount and memo
// pre-populated.
func TestNewSplitDialogFromExisting(t *testing.T) {
	amount := types.MustNewMoney("-100.00")
	cat1 := types.NewID()
	cat2 := types.NewID()
	cat3 := types.NewID()
	options := []string{"(None)", "Food", "Drink", "Snack"}
	ids := []types.ID{types.NilID, cat1, cat2, cat3}

	existing := []*transaction.Split{
		{
			BaseModel:  types.BaseModel{ID: types.NewID()},
			CategoryID: cat1,
			Amount:     types.MustNewMoney("-60.00"),
			Memo:       types.NullableString{String: "lunch", Valid: true},
		},
		{
			BaseModel:  types.BaseModel{ID: types.NewID()},
			CategoryID: cat3,
			Amount:     types.MustNewMoney("-40.00"),
			Memo:       types.NullableString{String: "tip", Valid: true},
		},
	}

	sd := NewSplitDialogFromExisting(amount, options, ids, existing)

	if !sd.IsVisible() {
		t.Error("dialog should be visible after creation")
	}
	rows := sd.Rows()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].categoryIndex != 1 { // Food at idx 1
		t.Errorf("row[0] categoryIndex = %d, want 1 (Food)", rows[0].categoryIndex)
	}
	if rows[0].amountField.Value != "-60" {
		t.Errorf("row[0] amount = %q, want %q", rows[0].amountField.Value, "-60")
	}
	if rows[0].memoField.Value != "lunch" {
		t.Errorf("row[0] memo = %q, want %q", rows[0].memoField.Value, "lunch")
	}
	if rows[1].categoryIndex != 3 { // Snack at idx 3
		t.Errorf("row[1] categoryIndex = %d, want 3 (Snack)", rows[1].categoryIndex)
	}
	if rows[1].amountField.Value != "-40" {
		t.Errorf("row[1] amount = %q, want %q", rows[1].amountField.Value, "-40")
	}
	// remaining() should be zero since the two splits sum to amount.
	if !sd.remaining().IsZero() {
		t.Errorf("remaining = %s, want 0 for fully allocated splits", sd.remaining().String())
	}
}

// TestApp_HandleRegisterKeys_EnterOnSplitTransaction_OpensEditFlow asserts
// Enter on a split (non-transfer) row in the register returns a non-nil
// load cmd. The transaction-load path inspects splits as part of the data
// fetch.
func TestApp_HandleRegisterKeys_EnterOnSplitTransaction_OpensEditFlow(t *testing.T) {
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
			transactions: []*transaction.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Date:      types.Today(),
					Amount:    types.MustNewMoney("-100"),
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
		t.Error("Enter on a (potentially split) plain row should return a load cmd")
	}
}

// TestApp_BuildTransactionDialog_EditMode_ChecksSplitBoxIfSplitsExist
// asserts that in edit mode, when the existing transaction has at least
// one split, the Split-transaction checkbox is pre-checked.
func TestApp_BuildTransactionDialog_EditMode_ChecksSplitBoxIfSplitsExist(t *testing.T) {
	cats := []*category.Category{
		{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Auto", Type: category.TypeExpense},
	}
	options, ids := buildCategoryOptions(cats)

	existing := transaction.NewTransaction(
		types.NewID(),
		types.NewDate(2024, 3, 15),
		types.MustNewMoney("-100.00"),
	)
	splits := []*transaction.Split{
		{BaseModel: types.BaseModel{ID: types.NewID()}, CategoryID: cats[0].ID, Amount: types.MustNewMoney("-100.00")},
	}

	data := &transactionDialogData{
		categories:     cats,
		mode:           transactionDialogModeEdit,
		existing:       existing,
		existingSplits: splits,
	}

	d := buildTransactionDialog(data, options, ids, types.ZeroDate)

	fields := d.Fields()
	if !fields[6].Checked {
		t.Error("split checkbox should be checked when editing a transaction with splits")
	}
}
