package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

func TestApp_RenderRegister_Loading(t *testing.T) {
	styles := widget.NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewRegister,
		width:       100,
		height:      30,
		styles:      styles,
		register:    nil,
	}

	view := app.renderRegister()
	if !contains(view, "Loading") {
		t.Errorf("renderRegister() should show loading when data is nil, got: %q", view)
	}
}

func TestApp_RenderRegister_WithData(t *testing.T) {
	styles := widget.NewStyles()
	styles.Resize(120, 30)

	accountID := types.NewID()
	payeeID := types.NewID()
	categoryID := types.NewID()

	app := &App{
		currentView: ViewRegister,
		width:       120,
		height:      30,
		styles:      styles,
		register: &registerData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: accountID},
				Name:      "Checking",
				Active:    true,
			},
			transactions: []*transaction.Transaction{
				{
					BaseModel:  types.BaseModel{ID: types.NewID()},
					AccountID:  accountID,
					Date:       types.Today(),
					Amount:     types.MustNewMoney("-125.43"),
					Status:     transaction.StatusCleared,
					PayeeID:    types.NullableID{ID: payeeID, Valid: true},
					CategoryID: types.NullableID{ID: categoryID, Valid: true},
				},
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Date:      types.Today(),
					Amount:    types.MustNewMoney("2500.00"),
					Status:    transaction.StatusUncleared,
					PayeeID:   types.NullableID{ID: payeeID, Valid: true},
				},
			},
			balance: &account.Balance{
				AccountID:      accountID,
				CurrentBalance: types.MustNewMoney("5234.57"),
				ClearedBalance: types.MustNewMoney("5000.00"),
			},
			payeeNames:    map[types.ID]string{payeeID: "Kroger"},
			categoryNames: map[types.ID]string{categoryID: "Groceries"},
			accountNames:  make(map[types.ID]string),
		},
		table: nil,
	}

	app.buildRegisterTable()
	view := app.renderRegister()

	if !contains(view, "CHECKING") {
		t.Error("renderRegister() should contain account name 'CHECKING'")
	}
	if !contains(view, "$5234.57") {
		t.Error("renderRegister() should contain balance '$5234.57'")
	}
	if !contains(view, "Kroger") {
		t.Error("renderRegister() should contain payee 'Kroger'")
	}
	if !contains(view, "Groceries") {
		t.Error("renderRegister() should contain category 'Groceries'")
	}
	if !contains(view, "$125.43") {
		t.Error("renderRegister() should contain amount '$125.43'")
	}
	if !contains(view, "$2500.00") {
		t.Error("renderRegister() should contain amount '$2500.00'")
	}
}

func TestApp_RenderRegister_EmptyTransactions(t *testing.T) {
	styles := widget.NewStyles()
	styles.Resize(100, 30)

	accountID := types.NewID()

	app := &App{
		currentView: ViewRegister,
		width:       100,
		height:      30,
		styles:      styles,
		register: &registerData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: accountID},
				Name:      "Savings",
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
	view := app.renderRegister()

	if !contains(view, "SAVINGS") {
		t.Error("renderRegister() should show account name 'SAVINGS'")
	}
	if !contains(view, "$0.00") {
		t.Error("renderRegister() should show zero balance")
	}
}

func TestApp_RenderRegister_NegativeBalance(t *testing.T) {
	styles := widget.NewStyles()
	styles.Resize(100, 30)

	accountID := types.NewID()

	app := &App{
		currentView: ViewRegister,
		width:       100,
		height:      30,
		styles:      styles,
		register: &registerData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: accountID},
				Name:      "Credit Card",
				Active:    true,
			},
			transactions:  []*transaction.Transaction{},
			balance:       &account.Balance{AccountID: accountID, CurrentBalance: types.MustNewMoney("-1500.00")},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}

	app.buildRegisterTable()
	view := app.renderRegister()

	if !contains(view, "-$1500.00") {
		t.Error("renderRegister() should show negative balance")
	}
}

func TestApp_RenderRegister_TransferDisplay(t *testing.T) {
	styles := widget.NewStyles()
	styles.Resize(120, 30)

	accountID := types.NewID()
	otherAccountID := types.NewID()
	transferID := types.NewID()

	app := &App{
		currentView: ViewRegister,
		width:       120,
		height:      30,
		styles:      styles,
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
					Amount:            types.MustNewMoney("-500.00"),
					Status:            transaction.StatusCleared,
					TransferID:        types.NullableID{ID: transferID, Valid: true},
					TransferAccountID: types.NullableID{ID: otherAccountID, Valid: true},
				},
			},
			balance:       &account.Balance{AccountID: accountID, CurrentBalance: types.MustNewMoney("4500.00")},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  map[types.ID]string{otherAccountID: "Savings"},
		},
	}

	app.buildRegisterTable()
	view := app.renderRegister()

	if !contains(view, "Transfer: Savings") {
		t.Error("renderRegister() should show 'Transfer: Savings' for transfer payee")
	}
	if !contains(view, "[Transfer]") {
		t.Error("renderRegister() should show '[Transfer]' in category for transfers")
	}
}

func TestApp_HandleRegisterKeys_TableNavigation(t *testing.T) {
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
				{BaseModel: types.BaseModel{ID: types.NewID()}, AccountID: accountID, Date: types.Today(), Amount: types.MustNewMoney("-10"), Status: transaction.StatusUncleared},
				{BaseModel: types.BaseModel{ID: types.NewID()}, AccountID: accountID, Date: types.Today(), Amount: types.MustNewMoney("-20"), Status: transaction.StatusUncleared},
				{BaseModel: types.BaseModel{ID: types.NewID()}, AccountID: accountID, Date: types.Today(), Amount: types.MustNewMoney("-30"), Status: transaction.StatusUncleared},
			},
			balance:       &account.Balance{AccountID: accountID, CurrentBalance: types.MustNewMoney("100")},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}
	app.buildRegisterTable()

	// widget.Table should start focused, sidebar not
	app.sidebar.SetFocused(false)
	app.table.SetFocused(true)

	// Move down
	downKey := tea.KeyPressMsg{Code: tea.KeyDown}
	app.Update(downKey)
	if app.table.Cursor() != 1 {
		t.Errorf("cursor should be 1 after down, got %d", app.table.Cursor())
	}

	// Move down again
	app.Update(downKey)
	if app.table.Cursor() != 2 {
		t.Errorf("cursor should be 2 after two downs, got %d", app.table.Cursor())
	}

	// Move up
	upKey := tea.KeyPressMsg{Code: tea.KeyUp}
	app.Update(upKey)
	if app.table.Cursor() != 1 {
		t.Errorf("cursor should be 1 after up, got %d", app.table.Cursor())
	}
}

func TestApp_HandleRegisterKeys_RKeyOpensReconciliation(t *testing.T) {
	// In the register view, "r" should open the start-reconciliation dialog
	// for the currently selected account — the same flow as
	// Accounts → Reconcile Account, but reachable without opening the menu.
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
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}
	app.buildRegisterTable()
	app.sidebar.SetFocused(false)
	app.table.SetFocused(true)

	model, _ := app.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	updatedApp := model.(*App)

	if updatedApp.reconDialog == nil {
		t.Fatal("pressing r in the register should open the reconciliation start dialog")
	}
	if !updatedApp.reconDialog.IsVisible() {
		t.Error("reconciliation dialog should be visible after r")
	}
}

func TestApp_HandleRegisterKeys_TabFocus(t *testing.T) {
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

	// Start with table focused
	app.sidebar.SetFocused(false)
	app.table.SetFocused(true)

	// Tab should switch focus to sidebar
	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	app.Update(tabKey)

	if !app.sidebar.IsFocused() {
		t.Error("sidebar should be focused after Tab")
	}
	if app.table.IsFocused() {
		t.Error("table should not be focused after Tab")
	}

	// Tab again should switch back to table
	app.Update(tabKey)

	if app.sidebar.IsFocused() {
		t.Error("sidebar should not be focused after second Tab")
	}
	if !app.table.IsFocused() {
		t.Error("table should be focused after second Tab")
	}
}

func TestApp_Update_RegisterLoaded(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	accountID := types.NewID()
	data := &registerData{
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
				Amount:    types.MustNewMoney("-50"),
				Status:    transaction.StatusUncleared,
			},
		},
		balance:       &account.Balance{AccountID: accountID, CurrentBalance: types.MustNewMoney("950")},
		payeeNames:    make(map[types.ID]string),
		categoryNames: make(map[types.ID]string),
		accountNames:  make(map[types.ID]string),
	}

	msg := registerLoadedMsg{data: data}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	if cmd != nil {
		t.Error("registerLoadedMsg should not return a command")
	}
	if updatedApp.register == nil {
		t.Fatal("register data should be set")
	}
	if updatedApp.register.account.Name != "Checking" {
		t.Errorf("register account name = %q, want %q", updatedApp.register.account.Name, "Checking")
	}
	if updatedApp.table == nil {
		t.Fatal("table should be created")
	}
	if updatedApp.table.RowCount() != 1 {
		t.Errorf("table row count = %d, want 1", updatedApp.table.RowCount())
	}
}

func TestApp_BuildRegisterTable_RowContent(t *testing.T) {
	accountID := types.NewID()
	payeeID := types.NewID()
	categoryID := types.NewID()

	app := &App{
		styles: widget.NewStyles(),
		register: &registerData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: accountID},
				Name:      "Checking",
				Active:    true,
			},
			transactions: []*transaction.Transaction{
				{
					BaseModel:  types.BaseModel{ID: types.NewID()},
					AccountID:  accountID,
					Date:       types.Today(),
					Amount:     types.MustNewMoney("-42.50"),
					Status:     transaction.StatusCleared,
					PayeeID:    types.NullableID{ID: payeeID, Valid: true},
					CategoryID: types.NullableID{ID: categoryID, Valid: true},
				},
			},
			balance:       &account.Balance{AccountID: accountID, CurrentBalance: types.MustNewMoney("100")},
			payeeNames:    map[types.ID]string{payeeID: "Shell"},
			categoryNames: map[types.ID]string{categoryID: "Gas"},
			accountNames:  make(map[types.ID]string),
		},
	}

	app.buildRegisterTable()

	if app.table.RowCount() != 1 {
		t.Fatalf("expected 1 row, got %d", app.table.RowCount())
	}

	row := app.table.SelectedRow()
	if row == nil {
		t.Fatal("selected row should not be nil")
	}

	// Check row contents: Date, Status, Payee, Category, Amount
	if row[1] != "✓" {
		t.Errorf("status = %q, want %q", row[1], "✓")
	}
	if row[2] != "Shell" {
		t.Errorf("payee = %q, want %q", row[2], "Shell")
	}
	if row[3] != "Gas" {
		t.Errorf("category = %q, want %q", row[3], "Gas")
	}
	if row[4] != "-$42.50" {
		t.Errorf("amount = %q, want %q", row[4], "-$42.50")
	}
}

// TestApp_BuildRegisterTable_SelectsPendingByID verifies the cursor lands on
// the just-saved transaction's row, matched by ID — even when that row sorts
// into the middle of the list (e.g. a back-dated entry), not at the top.
func TestApp_BuildRegisterTable_SelectsPendingByID(t *testing.T) {
	accountID := types.NewID()

	// Rows in display order (date DESC): a newer txn, the back-dated new txn,
	// then an older one. The new entry is at index 1, not the top.
	newID := types.NewID()
	txns := []*transaction.Transaction{
		{BaseModel: types.BaseModel{ID: types.NewID()}, AccountID: accountID, Date: types.NewDate(2024, 3, 10), Amount: types.MustNewMoney("-10")},
		{BaseModel: types.BaseModel{ID: newID}, AccountID: accountID, Date: types.NewDate(2024, 2, 5), Amount: types.MustNewMoney("-20")},
		{BaseModel: types.BaseModel{ID: types.NewID()}, AccountID: accountID, Date: types.NewDate(2024, 1, 1), Amount: types.MustNewMoney("-30")},
	}

	app := &App{
		styles: widget.NewStyles(),
		register: &registerData{
			account:       &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Checking", Active: true},
			transactions:  txns,
			balance:       &account.Balance{AccountID: accountID, CurrentBalance: types.MustNewMoney("0")},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
		pendingRegisterSelectID: newID,
	}

	app.buildRegisterTable()

	if app.table.Cursor() != 1 {
		t.Errorf("cursor = %d, want 1 (the back-dated new transaction)", app.table.Cursor())
	}
	if !app.pendingRegisterSelectID.IsNil() {
		t.Error("pendingRegisterSelectID should be cleared after selection")
	}
}

// TestApp_BuildRegisterTable_NoPendingLeavesCursor verifies that with no
// pending selection the cursor stays where it was (clamped), so unrelated
// reloads (delete, toggle-clear) don't jump the cursor.
func TestApp_BuildRegisterTable_NoPendingLeavesCursor(t *testing.T) {
	accountID := types.NewID()
	txns := []*transaction.Transaction{
		{BaseModel: types.BaseModel{ID: types.NewID()}, AccountID: accountID, Date: types.NewDate(2024, 3, 10), Amount: types.MustNewMoney("-10")},
		{BaseModel: types.BaseModel{ID: types.NewID()}, AccountID: accountID, Date: types.NewDate(2024, 2, 5), Amount: types.MustNewMoney("-20")},
		{BaseModel: types.BaseModel{ID: types.NewID()}, AccountID: accountID, Date: types.NewDate(2024, 1, 1), Amount: types.MustNewMoney("-30")},
	}

	app := &App{
		styles: widget.NewStyles(),
		register: &registerData{
			account:       &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Checking", Active: true},
			transactions:  txns,
			balance:       &account.Balance{AccountID: accountID, CurrentBalance: types.MustNewMoney("0")},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}

	app.buildRegisterTable()
	app.table.SetCursor(2)
	// Rebuild (simulating a reload with no pending selection).
	app.buildRegisterTable()

	if app.table.Cursor() != 2 {
		t.Errorf("cursor = %d, want 2 (unchanged)", app.table.Cursor())
	}
}

// TestApp_Update_TransactionDialogSaved_SetsPendingSelectID verifies the saved
// message's ID is stashed so the next register build can re-select the row.
func TestApp_Update_TransactionDialogSaved_SetsPendingSelectID(t *testing.T) {
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

	savedID := types.NewID()
	model, _ := app.Update(transactionDialogSavedMsg{savedID: savedID})
	updatedApp := model.(*App)

	if updatedApp.pendingRegisterSelectID != savedID {
		t.Errorf("pendingRegisterSelectID = %v, want %v", updatedApp.pendingRegisterSelectID, savedID)
	}
}

func TestApp_BuildRegisterTable_StatusIndicators(t *testing.T) {
	accountID := types.NewID()

	tests := []struct {
		name     string
		status   transaction.Status
		expected string
	}{
		{"uncleared", transaction.StatusUncleared, " "},
		{"cleared", transaction.StatusCleared, "✓"},
		{"reconciled", transaction.StatusReconciled, "R"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &App{
				styles: widget.NewStyles(),
				register: &registerData{
					account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test", Active: true},
					transactions: []*transaction.Transaction{
						{
							BaseModel: types.BaseModel{ID: types.NewID()},
							AccountID: accountID,
							Date:      types.Today(),
							Amount:    types.MustNewMoney("-10"),
							Status:    tt.status,
						},
					},
					balance:       &account.Balance{AccountID: accountID, CurrentBalance: types.ZeroMoney},
					payeeNames:    make(map[types.ID]string),
					categoryNames: make(map[types.ID]string),
					accountNames:  make(map[types.ID]string),
				},
			}

			app.buildRegisterTable()
			row := app.table.SelectedRow()
			if row[1] != tt.expected {
				t.Errorf("status indicator = %q, want %q", row[1], tt.expected)
			}
		})
	}
}

func TestApp_RenderRegister_LongAccountName(t *testing.T) {
	styles := widget.NewStyles()
	styles.Resize(60, 30) // narrow width to force truncation

	accountID := types.NewID()

	app := &App{
		currentView: ViewRegister,
		width:       60,
		height:      30,
		styles:      styles,
		register: &registerData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: accountID},
				Name:      "My Super Duper Extremely Long Savings Account Name That Overflows",
				Active:    true,
			},
			transactions:  []*transaction.Transaction{},
			balance:       &account.Balance{AccountID: accountID, CurrentBalance: types.MustNewMoney("100.00")},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}

	app.buildRegisterTable()
	view := app.renderRegister()

	// The full name should NOT appear (it's too long)
	fullName := "MY SUPER DUPER EXTREMELY LONG SAVINGS ACCOUNT NAME THAT OVERFLOWS"
	if contains(view, fullName) {
		t.Error("renderRegister() should truncate long account names")
	}
	// Truncation indicator should appear
	if !contains(view, "...") {
		t.Error("renderRegister() should show '...' for truncated account names")
	}
	// Balance should still be visible
	if !contains(view, "$100.00") {
		t.Error("renderRegister() should still show balance after truncation")
	}
}

func TestApp_RenderRegister_EmptyShowsHint(t *testing.T) {
	styles := widget.NewStyles()
	styles.Resize(100, 30)

	accountID := types.NewID()

	app := &App{
		currentView: ViewRegister,
		width:       100,
		height:      30,
		styles:      styles,
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

	view := app.renderRegister()

	if !contains(view, "No transactions") {
		t.Error("renderRegister() should show 'No transactions' when empty")
	}
	if !contains(view, "Press 'n' to add a new transaction") {
		t.Error("renderRegister() should show action hint when empty")
	}
}

func TestApp_BuildRegisterTable_VoidStatusIndicator(t *testing.T) {
	accountID := types.NewID()

	app := &App{
		styles: widget.NewStyles(),
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test", Active: true},
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
	row := app.table.SelectedRow()
	if row[1] != "V" {
		t.Errorf("void status indicator = %q, want %q", row[1], "V")
	}
}

func TestApp_BuildRegisterTable_VoidRowStyling(t *testing.T) {
	accountID := types.NewID()

	app := &App{
		styles: widget.NewStyles(),
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test", Active: true},
			transactions: []*transaction.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Date:      types.Today(),
					Amount:    types.MustNewMoney("-50"),
					Status:    transaction.StatusCleared,
				},
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Date:      types.Today(),
					Amount:    types.ZeroMoney,
					Status:    transaction.StatusVoid,
				},
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

	// Void row (index 1) should have widget.RowStyleVoid
	if style, ok := app.table.RowStyles()[1]; !ok || style != widget.RowStyleVoid {
		t.Errorf("void row style = %v (ok=%v), want widget.RowStyleVoid", style, ok)
	}

	// Non-void rows should not have a style override
	if _, ok := app.table.RowStyles()[0]; ok {
		t.Error("cleared row should not have a style override")
	}
	if _, ok := app.table.RowStyles()[2]; ok {
		t.Error("uncleared row should not have a style override")
	}
}

func TestApp_BuildRegisterTable_AllFourStatusIndicators(t *testing.T) {
	accountID := types.NewID()

	tests := []struct {
		name     string
		status   transaction.Status
		expected string
	}{
		{"uncleared", transaction.StatusUncleared, " "},
		{"cleared", transaction.StatusCleared, "✓"},
		{"reconciled", transaction.StatusReconciled, "R"},
		{"void", transaction.StatusVoid, "V"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &App{
				styles: widget.NewStyles(),
				register: &registerData{
					account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test", Active: true},
					transactions: []*transaction.Transaction{
						{
							BaseModel: types.BaseModel{ID: types.NewID()},
							AccountID: accountID,
							Date:      types.Today(),
							Amount:    types.MustNewMoney("-10"),
							Status:    tt.status,
						},
					},
					balance:       &account.Balance{AccountID: accountID, CurrentBalance: types.ZeroMoney},
					payeeNames:    make(map[types.ID]string),
					categoryNames: make(map[types.ID]string),
					accountNames:  make(map[types.ID]string),
				},
			}

			app.buildRegisterTable()
			row := app.table.SelectedRow()
			if row[1] != tt.expected {
				t.Errorf("status indicator = %q, want %q", row[1], tt.expected)
			}
		})
	}
}

func TestApp_ToggleTransactionStatus_VoidBlocked(t *testing.T) {
	accountID := types.NewID()

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      widget.NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test", Active: true},
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
	_, cmd := app.toggleTransactionStatus()

	// Should return nil cmd (no service call made)
	if cmd != nil {
		t.Error("toggleTransactionStatus() should return nil cmd for void transaction")
	}

	// Should have an alert notification
	notifications := app.statusbar.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if !contains(notifications[0].Text, "void") {
		t.Errorf("notification = %q, should mention void", notifications[0].Text)
	}
}

func TestApp_ToggleTransactionStatus_ReconciledBlocked(t *testing.T) {
	accountID := types.NewID()

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      widget.NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test", Active: true},
			transactions: []*transaction.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Date:      types.Today(),
					Amount:    types.MustNewMoney("-10"),
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
	_, cmd := app.toggleTransactionStatus()

	if cmd != nil {
		t.Error("toggleTransactionStatus() should return nil cmd for reconciled transaction")
	}

	notifications := app.statusbar.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if !contains(notifications[0].Text, "reconciled") {
		t.Errorf("notification = %q, should mention reconciled", notifications[0].Text)
	}
}

func TestApp_ShowVoidConfirmation_AlreadyVoid(t *testing.T) {
	accountID := types.NewID()

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      widget.NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test", Active: true},
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
	_, cmd := app.showVoidConfirmation()

	if cmd != nil {
		t.Error("showVoidConfirmation() should return nil cmd for already-void transaction")
	}

	// Should show notification
	notifications := app.statusbar.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if !contains(notifications[0].Text, "already void") {
		t.Errorf("notification = %q, should mention 'already void'", notifications[0].Text)
	}

	// No confirm dialog should be shown
	if app.confirmDialog != nil {
		t.Error("confirmDialog should be nil for already-void transaction")
	}
}

func TestApp_ShowVoidConfirmation_ReconciledBlocked(t *testing.T) {
	accountID := types.NewID()

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      widget.NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test", Active: true},
			transactions: []*transaction.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Date:      types.Today(),
					Amount:    types.MustNewMoney("-50"),
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
	_, cmd := app.showVoidConfirmation()

	if cmd != nil {
		t.Error("showVoidConfirmation() should return nil cmd for reconciled transaction")
	}

	notifications := app.statusbar.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if !contains(notifications[0].Text, "reconciled") {
		t.Errorf("notification = %q, should mention 'reconciled'", notifications[0].Text)
	}
}

func TestApp_ShowVoidConfirmation_ShowsDialog(t *testing.T) {
	accountID := types.NewID()

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      widget.NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test", Active: true},
			transactions: []*transaction.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Date:      types.Today(),
					Amount:    types.MustNewMoney("-50"),
					Status:    transaction.StatusCleared,
				},
			},
			balance:       &account.Balance{AccountID: accountID, CurrentBalance: types.ZeroMoney},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}

	app.buildRegisterTable()
	_, _ = app.showVoidConfirmation()

	if app.confirmDialog == nil {
		t.Fatal("confirmDialog should be set after showVoidConfirmation()")
	}
	if !app.confirmDialog.IsVisible() {
		t.Error("confirmDialog should be visible")
	}
	if app.confirmDialog.Title() != "Void Transaction" {
		t.Errorf("dialog title = %q, want %q", app.confirmDialog.Title(), "Void Transaction")
	}
	if app.confirmAction == nil {
		t.Error("confirmAction should be set")
	}
}

func TestApp_ShowVoidConfirmation_TransferMessage(t *testing.T) {
	accountID := types.NewID()
	transferAccountID := types.NewID()
	transferPairID := types.NewID()

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      widget.NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test", Active: true},
			transactions: []*transaction.Transaction{
				{
					BaseModel:         types.BaseModel{ID: types.NewID()},
					AccountID:         accountID,
					TransferID:        types.NullableID{ID: transferPairID, Valid: true},
					TransferAccountID: types.NullableID{ID: transferAccountID, Valid: true},
					Date:              types.Today(),
					Amount:            types.MustNewMoney("-50"),
					Status:            transaction.StatusCleared,
				},
			},
			balance:       &account.Balance{AccountID: accountID, CurrentBalance: types.ZeroMoney},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  map[types.ID]string{transferAccountID: "Savings"},
		},
	}

	app.buildRegisterTable()
	_, _ = app.showVoidConfirmation()

	if app.confirmDialog == nil {
		t.Fatal("confirmDialog should be set")
	}
	// The message should mention "transfer" (rendered in the wrapped body).
	msg := strings.Join(strings.Fields(app.confirmDialog.Message()), " ")
	if !contains(msg, "transfer") {
		t.Errorf("dialog message = %q, should mention 'transfer'", msg)
	}
}

func TestApp_VoidKey_InRegisterView(t *testing.T) {
	accountID := types.NewID()

	sidebar := NewSidebar()
	sidebar.SetFocused(false)

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      widget.NewStatusBar(),
		sidebar:        sidebar,
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test", Active: true},
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

	// Press 'v' key
	msg := tea.KeyPressMsg{Code: 'v', Text: "v"}
	_, _ = app.handleRegisterKeys(msg)

	// Should show confirmation dialog
	if app.confirmDialog == nil {
		t.Fatal("pressing 'v' should show confirmation dialog")
	}
	if !app.confirmDialog.IsVisible() {
		t.Error("confirmation dialog should be visible")
	}
}

func TestApp_RegisterFrozenOnClosedAccount(t *testing.T) {
	accountID := types.NewID()
	app := &App{
		currentView:    ViewRegister,
		styles:         widget.NewStyles(),
		keys:           defaultKeyMap(),
		statusbar:      widget.NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{
				BaseModel:  types.BaseModel{ID: accountID},
				Name:       "Old Checking",
				Active:     false,
				ClosedDate: types.NullableDate{Date: types.MustParseDate("2024-03-14"), Valid: true},
			},
			transactions: []*transaction.Transaction{
				{BaseModel: types.BaseModel{ID: types.NewID()}, AccountID: accountID, Date: types.Today(), Amount: types.MustNewMoney("-25"), Status: transaction.StatusUncleared},
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

	// 'r' must not open the reconciliation dialog.
	app.handleRegisterKeys(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if app.reconDialog != nil {
		t.Error("'r' should be a no-op on a closed account")
	}
	// 'v' / 'd' must not open a confirmation dialog.
	app.handleRegisterKeys(tea.KeyPressMsg{Code: 'v', Text: "v"})
	app.handleRegisterKeys(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if app.confirmDialog != nil {
		t.Error("'v'/'d' should be no-ops on a closed account")
	}
	// 'n' (new) must not return a load command.
	if _, cmd := app.handleRegisterKeys(tea.KeyPressMsg{Code: 'n', Text: "n"}); cmd != nil {
		t.Error("'n' should be a no-op on a closed account")
	}

	// The register shows a read-only banner with the close date.
	out := app.renderRegister()
	if !strings.Contains(out, "Closed 2024-03-14") || !strings.Contains(out, "read-only") {
		t.Errorf("expected a closed read-only banner with the date, got:\n%s", out)
	}
}

func TestApp_HelpOverlay_RegisterIncludesVoid(t *testing.T) {
	section := registerShortcuts()
	found := false
	for _, entry := range section.Entries {
		if entry.Key == "v" && contains(entry.Description, "Void") {
			found = true
			break
		}
	}
	if !found {
		t.Error("register shortcuts should include 'v' for voiding transactions")
	}
}

func TestApp_ShowVoidConfirmation_NilGuards(t *testing.T) {
	// Nil table
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
	}
	_, cmd := app.showVoidConfirmation()
	if cmd != nil {
		t.Error("showVoidConfirmation() should return nil when table is nil")
	}

	// Nil register
	app.table = widget.NewTable([]widget.Column{{Header: "A", Width: 10}})
	_, cmd = app.showVoidConfirmation()
	if cmd != nil {
		t.Error("showVoidConfirmation() should return nil when register is nil")
	}
}

func TestApp_ShowDeleteConfirmation_AlreadyVoid(t *testing.T) {
	accountID := types.NewID()

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      widget.NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test", Active: true},
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
	_, cmd := app.showDeleteConfirmation()

	if cmd != nil {
		t.Error("showDeleteConfirmation() should return nil cmd for void transaction")
	}

	notifications := app.statusbar.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if !contains(notifications[0].Text, "void") {
		t.Errorf("notification = %q, should mention void", notifications[0].Text)
	}
	if app.confirmDialog != nil {
		t.Error("confirmDialog should be nil for void transaction")
	}
}

func TestApp_ShowDeleteConfirmation_ReconciledBlocked(t *testing.T) {
	accountID := types.NewID()

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      widget.NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test", Active: true},
			transactions: []*transaction.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Date:      types.Today(),
					Amount:    types.MustNewMoney("-50"),
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
	_, cmd := app.showDeleteConfirmation()

	if cmd != nil {
		t.Error("showDeleteConfirmation() should return nil cmd for reconciled transaction")
	}

	notifications := app.statusbar.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if !contains(notifications[0].Text, "reconciled") {
		t.Errorf("notification = %q, should mention reconciled", notifications[0].Text)
	}
	if app.confirmDialog != nil {
		t.Error("confirmDialog should be nil for reconciled transaction")
	}
}

func TestApp_ShowDeleteConfirmation_ShowsDialog(t *testing.T) {
	accountID := types.NewID()

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      widget.NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test", Active: true},
			transactions: []*transaction.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Date:      types.Today(),
					Amount:    types.MustNewMoney("-50"),
					Status:    transaction.StatusCleared,
				},
			},
			balance:       &account.Balance{AccountID: accountID, CurrentBalance: types.ZeroMoney},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
		},
	}

	app.buildRegisterTable()
	_, _ = app.showDeleteConfirmation()

	if app.confirmDialog == nil {
		t.Fatal("confirmDialog should be set after showDeleteConfirmation()")
	}
	if !app.confirmDialog.IsVisible() {
		t.Error("confirmDialog should be visible")
	}
	if app.confirmDialog.Title() != "Delete Transaction" {
		t.Errorf("dialog title = %q, want %q", app.confirmDialog.Title(), "Delete Transaction")
	}
	if app.confirmAction == nil {
		t.Error("confirmAction should be set")
	}
}

func TestApp_ShowDeleteConfirmation_TransferMessage(t *testing.T) {
	accountID := types.NewID()
	transferAccountID := types.NewID()
	transferPairID := types.NewID()

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      widget.NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test", Active: true},
			transactions: []*transaction.Transaction{
				{
					BaseModel:         types.BaseModel{ID: types.NewID()},
					AccountID:         accountID,
					TransferID:        types.NullableID{ID: transferPairID, Valid: true},
					TransferAccountID: types.NullableID{ID: transferAccountID, Valid: true},
					Date:              types.Today(),
					Amount:            types.MustNewMoney("-50"),
					Status:            transaction.StatusCleared,
				},
			},
			balance:       &account.Balance{AccountID: accountID, CurrentBalance: types.ZeroMoney},
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  map[types.ID]string{transferAccountID: "Savings"},
		},
	}

	app.buildRegisterTable()
	_, _ = app.showDeleteConfirmation()

	if app.confirmDialog == nil {
		t.Fatal("confirmDialog should be set")
	}
	// The prompt is rendered in the (wrapped) message body; normalize
	// whitespace so word-wrap line breaks don't split the phrases.
	msg := strings.Join(strings.Fields(app.confirmDialog.Message()), " ")
	if !contains(msg, "transfer") {
		t.Errorf("dialog message = %q, should mention 'transfer'", msg)
	}
	if !contains(msg, "Both sides") {
		t.Errorf("dialog message = %q, should mention 'Both sides'", msg)
	}
}

func TestApp_DeleteKey_InRegisterView(t *testing.T) {
	accountID := types.NewID()

	sidebar := NewSidebar()
	sidebar.SetFocused(false)

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      widget.NewStatusBar(),
		sidebar:        sidebar,
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test", Active: true},
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

	msg := tea.KeyPressMsg{Code: 'd', Text: "d"}
	_, _ = app.handleRegisterKeys(msg)

	if app.confirmDialog == nil {
		t.Fatal("pressing 'd' should show confirmation dialog")
	}
	if !app.confirmDialog.IsVisible() {
		t.Error("confirmation dialog should be visible")
	}
	if app.confirmDialog.Title() != "Delete Transaction" {
		t.Errorf("dialog title = %q, want %q", app.confirmDialog.Title(), "Delete Transaction")
	}
}

func TestApp_ShowDeleteConfirmation_NilGuards(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
	}
	_, cmd := app.showDeleteConfirmation()
	if cmd != nil {
		t.Error("showDeleteConfirmation() should return nil when table is nil")
	}

	app.table = widget.NewTable([]widget.Column{{Header: "A", Width: 10}})
	_, cmd = app.showDeleteConfirmation()
	if cmd != nil {
		t.Error("showDeleteConfirmation() should return nil when register is nil")
	}
}
