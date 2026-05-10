package tui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestViewString(t *testing.T) {
	tests := []struct {
		view     View
		expected string
	}{
		{ViewDashboard, "Dashboard"},
		{ViewRegister, "Register"},
		{ViewScheduled, "Scheduled"},
		{ViewReports, "Reports"},
		{View(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.view.String(); got != tt.expected {
				t.Errorf("View.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestApp_Init(t *testing.T) {
	// Create a minimal app without database for testing Init
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
	}

	cmd := app.Init()
	if cmd == nil {
		t.Error("Init() should return a command")
	}
}

func TestApp_Update_WindowSize(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
	}

	msg := tea.WindowSizeMsg{Width: 80, Height: 24}
	model, cmd := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.width != 80 {
		t.Errorf("width = %d, want 80", updatedApp.width)
	}
	if updatedApp.height != 24 {
		t.Errorf("height = %d, want 24", updatedApp.height)
	}
	if !updatedApp.ready {
		t.Error("ready should be true after WindowSizeMsg")
	}
	if cmd != nil {
		t.Error("WindowSizeMsg should not return a command")
	}
}

func TestApp_Update_QuitKey(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
	}

	msg := tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl}
	model, cmd := app.Update(msg)

	updatedApp := model.(*App)
	if !updatedApp.quitting {
		t.Error("quitting should be true after Ctrl+Q")
	}
	if cmd == nil {
		t.Error("Quit should return a tea.Quit command")
	}
}

func TestApp_Update_ViewSwitchKeys(t *testing.T) {
	tests := []struct {
		name         string
		key          tea.KeyMsg
		expectedView View
	}{
		{"Dashboard key", tea.KeyPressMsg{Code: '1', Text: "1"}, ViewDashboard},
		{"Scheduled key", tea.KeyPressMsg{Code: '2', Text: "2"}, ViewScheduled},
		{"Reports key", tea.KeyPressMsg{Code: '3', Text: "3"}, ViewReports},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &App{
				currentView: ViewDashboard,
				keys:        defaultKeyMap(),
				menubar:     NewMenuBar(),
				statusbar:   NewStatusBar(),
			}

			model, _ := app.Update(tt.key)
			updatedApp := model.(*App)

			if updatedApp.currentView != tt.expectedView {
				t.Errorf("currentView = %v, want %v", updatedApp.currentView, tt.expectedView)
			}
		})
	}
}

func TestApp_Update_EscapeKey(t *testing.T) {
	app := &App{
		currentView:  ViewRegister,
		previousView: ViewDashboard,
		keys:         defaultKeyMap(),
		menubar:      NewMenuBar(),
		statusbar:    NewStatusBar(),
	}

	msg := tea.KeyPressMsg{Code: tea.KeyEsc}
	model, _ := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.currentView != ViewDashboard {
		t.Errorf("currentView = %v, want %v (should go back to previous)", updatedApp.currentView, ViewDashboard)
	}
}

func TestApp_Update_ScheduledDueCount(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	// Test with 3 due transactions
	msg := scheduledDueCountMsg{count: 3}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	if cmd != nil {
		t.Error("scheduledDueCountMsg should not return a command")
	}
	notifications := updatedApp.statusbar.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if notifications[0].Text != "3 scheduled due" {
		t.Errorf("notification text = %q, want %q", notifications[0].Text, "3 scheduled due")
	}
	if notifications[0].Level != NotificationAlert {
		t.Errorf("notification level = %d, want %d", notifications[0].Level, NotificationAlert)
	}
}

func TestApp_Update_ScheduledDueCount_Single(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	msg := scheduledDueCountMsg{count: 1}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	notifications := updatedApp.statusbar.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if notifications[0].Text != "1 scheduled due" {
		t.Errorf("notification text = %q, want %q", notifications[0].Text, "1 scheduled due")
	}
}

func TestApp_Update_ScheduledDueCount_Zero(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	// Add a notification first, then clear with count 0
	app.statusbar.AddNotification("old", NotificationInfo)

	msg := scheduledDueCountMsg{count: 0}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if len(updatedApp.statusbar.Notifications()) != 0 {
		t.Errorf("expected 0 notifications for count=0, got %d", len(updatedApp.statusbar.Notifications()))
	}
}

// =============================================================================
// Register View Tests
// =============================================================================

func TestApp_RenderRegister_Loading(t *testing.T) {
	styles := NewStyles()
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
	styles := NewStyles()
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
	styles := NewStyles()
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
	styles := NewStyles()
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
	styles := NewStyles()
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

	// Table should start focused, sidebar not
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

func TestApp_HandleRegisterKeys_TabFocus(t *testing.T) {
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	accountID := types.NewID()
	data := &registerData{
		account: &account.Account{
			BaseModel: types.BaseModel{ID: accountID},
			Name:      "Checking",
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
		styles: NewStyles(),
		register: &registerData{
			account: &account.Account{
				BaseModel: types.BaseModel{ID: accountID},
				Name:      "Checking",
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
				styles: NewStyles(),
				register: &registerData{
					account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test"},
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

// =============================================================================
// Scheduled View Tests
// =============================================================================

func TestApp_RenderScheduled_Loading(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewScheduled,
		width:       100,
		height:      30,
		styles:      styles,
		scheduled:   nil,
	}

	view := app.renderScheduled()
	if !contains(view, "Loading") {
		t.Errorf("renderScheduled() should show loading when data is nil, got: %q", view)
	}
}

func TestApp_RenderScheduled_Empty(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewScheduled,
		width:       100,
		height:      30,
		styles:      styles,
		scheduled: &scheduledViewData{
			allTxns:       nil,
			dueCount:      0,
			payeeNames:    make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
		},
	}

	view := app.renderScheduled()
	if !contains(view, "No scheduled transactions") {
		t.Error("renderScheduled() should show 'No scheduled transactions' when empty")
	}
	if !contains(view, "SCHEDULED TRANSACTIONS") {
		t.Error("renderScheduled() should contain title 'SCHEDULED TRANSACTIONS'")
	}
}

func TestApp_RenderScheduled_WithDueAndUpcoming(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 30)

	payeeID1 := types.NewID()
	payeeID2 := types.NewID()
	accountID := types.NewID()

	dueTxn := &scheduled.Transaction{
		BaseModel: types.BaseModel{ID: types.NewID()},
		AccountID: accountID,
		Frequency: scheduled.FrequencyMonthly,
		NextDate:  types.Today(),
		PayeeID:   types.NullableID{ID: payeeID1, Valid: true},
		Amount:    types.NullableMoney{Money: types.MustNewMoney("-1500.00"), Valid: true},
	}

	upcomingTxn := &scheduled.Transaction{
		BaseModel: types.BaseModel{ID: types.NewID()},
		AccountID: accountID,
		Frequency: scheduled.FrequencyWeekly,
		NextDate:  types.Today().AddDays(7),
		PayeeID:   types.NullableID{ID: payeeID2, Valid: true},
		Amount:    types.NullableMoney{Money: types.MustNewMoney("-50.00"), Valid: true},
	}

	app := &App{
		currentView: ViewScheduled,
		width:       120,
		height:      30,
		styles:      styles,
		scheduled: &scheduledViewData{
			dueTxns:       []*scheduled.Transaction{dueTxn},
			upcomingTxns:  []*scheduled.Transaction{upcomingTxn},
			allTxns:       []*scheduled.Transaction{dueTxn, upcomingTxn},
			dueCount:      1,
			payeeNames:    map[types.ID]string{payeeID1: "Landlord", payeeID2: "Netflix"},
			accountNames:  map[types.ID]string{accountID: "Checking"},
			categoryNames: make(map[types.ID]string),
		},
	}

	app.buildScheduledTable()
	view := app.renderScheduled()

	if !contains(view, "SCHEDULED TRANSACTIONS") {
		t.Error("renderScheduled() should contain title")
	}
	if !contains(view, "1 due") {
		t.Error("renderScheduled() should show '1 due' count")
	}
	if !contains(view, "Landlord") {
		t.Error("renderScheduled() should contain payee 'Landlord'")
	}
	if !contains(view, "Netflix") {
		t.Error("renderScheduled() should contain payee 'Netflix'")
	}
	if !contains(view, "$1500.00") {
		t.Error("renderScheduled() should contain amount '$1500.00'")
	}
	if !contains(view, "Monthly") {
		t.Error("renderScheduled() should contain frequency 'Monthly'")
	}
	if !contains(view, "Checking") {
		t.Error("renderScheduled() should contain account 'Checking'")
	}
}

func TestApp_BuildScheduledTable(t *testing.T) {
	payeeID := types.NewID()
	accountID := types.NewID()

	app := &App{
		styles: NewStyles(),
		scheduled: &scheduledViewData{
			allTxns: []*scheduled.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Frequency: scheduled.FrequencyMonthly,
					NextDate:  types.Today(),
					PayeeID:   types.NullableID{ID: payeeID, Valid: true},
					Amount:    types.NullableMoney{Money: types.MustNewMoney("-100.00"), Valid: true},
				},
			},
			dueCount:      1,
			payeeNames:    map[types.ID]string{payeeID: "Electric Co"},
			accountNames:  map[types.ID]string{accountID: "Checking"},
			categoryNames: make(map[types.ID]string),
		},
	}

	app.buildScheduledTable()

	if app.scheduledTable == nil {
		t.Fatal("scheduledTable should be created")
	}
	if app.scheduledTable.RowCount() != 1 {
		t.Errorf("expected 1 row, got %d", app.scheduledTable.RowCount())
	}

	row := app.scheduledTable.SelectedRow()
	if row == nil {
		t.Fatal("selected row should not be nil")
	}

	// Check row content: Status, Date, Payee, Amount, Frequency, Account
	if row[0] != " ●" {
		t.Errorf("status = %q, want %q (due today)", row[0], " ●")
	}
	if row[2] != "Electric Co" {
		t.Errorf("payee = %q, want %q", row[2], "Electric Co")
	}
	if row[3] != "-$100.00" {
		t.Errorf("amount = %q, want %q", row[3], "-$100.00")
	}
	if row[4] != "Monthly" {
		t.Errorf("frequency = %q, want %q", row[4], "Monthly")
	}
	if row[5] != "Checking" {
		t.Errorf("account = %q, want %q", row[5], "Checking")
	}
}

func TestApp_BuildScheduledTable_VariableAmount(t *testing.T) {
	accountID := types.NewID()

	app := &App{
		styles: NewStyles(),
		scheduled: &scheduledViewData{
			allTxns: []*scheduled.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Frequency: scheduled.FrequencyMonthly,
					NextDate:  types.Today(),
					// No amount set - variable
				},
			},
			dueCount:      1,
			payeeNames:    make(map[types.ID]string),
			accountNames:  map[types.ID]string{accountID: "Checking"},
			categoryNames: make(map[types.ID]string),
		},
	}

	app.buildScheduledTable()

	row := app.scheduledTable.SelectedRow()
	if row[3] != "~variable" {
		t.Errorf("amount = %q, want %q for variable amount", row[3], "~variable")
	}
}

func TestApp_BuildScheduledTable_OverdueIndicator(t *testing.T) {
	accountID := types.NewID()
	pastDate := types.Today().AddDays(-3)

	app := &App{
		styles: NewStyles(),
		scheduled: &scheduledViewData{
			allTxns: []*scheduled.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Frequency: scheduled.FrequencyMonthly,
					NextDate:  pastDate,
					Amount:    types.NullableMoney{Money: types.MustNewMoney("-50"), Valid: true},
				},
			},
			dueCount:      1,
			payeeNames:    make(map[types.ID]string),
			accountNames:  map[types.ID]string{accountID: "Checking"},
			categoryNames: make(map[types.ID]string),
		},
	}

	app.buildScheduledTable()

	row := app.scheduledTable.SelectedRow()
	if row[0] != "!●" {
		t.Errorf("status = %q, want %q for overdue", row[0], "!●")
	}
}

func TestApp_BuildScheduledTable_UpcomingIndicator(t *testing.T) {
	accountID := types.NewID()
	futureDate := types.Today().AddDays(7)

	app := &App{
		styles: NewStyles(),
		scheduled: &scheduledViewData{
			allTxns: []*scheduled.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					AccountID: accountID,
					Frequency: scheduled.FrequencyWeekly,
					NextDate:  futureDate,
					Amount:    types.NullableMoney{Money: types.MustNewMoney("-25"), Valid: true},
				},
			},
			dueCount:      0, // not due, so index 0 >= dueCount (0)
			payeeNames:    make(map[types.ID]string),
			accountNames:  map[types.ID]string{accountID: "Checking"},
			categoryNames: make(map[types.ID]string),
		},
	}

	app.buildScheduledTable()

	row := app.scheduledTable.SelectedRow()
	if row[0] != " ○" {
		t.Errorf("status = %q, want %q for upcoming", row[0], " ○")
	}
}

func TestApp_HandleScheduledKeys_TableNavigation(t *testing.T) {
	accountID := types.NewID()

	app := &App{
		currentView: ViewScheduled,
		width:       120,
		height:      30,
		styles:      NewStyles(),
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		scheduled: &scheduledViewData{
			allTxns: []*scheduled.Transaction{
				{BaseModel: types.BaseModel{ID: types.NewID()}, AccountID: accountID, Frequency: scheduled.FrequencyMonthly, NextDate: types.Today()},
				{BaseModel: types.BaseModel{ID: types.NewID()}, AccountID: accountID, Frequency: scheduled.FrequencyWeekly, NextDate: types.Today()},
				{BaseModel: types.BaseModel{ID: types.NewID()}, AccountID: accountID, Frequency: scheduled.FrequencyYearly, NextDate: types.Today()},
			},
			dueCount:      3,
			payeeNames:    make(map[types.ID]string),
			accountNames:  map[types.ID]string{accountID: "Checking"},
			categoryNames: make(map[types.ID]string),
		},
	}
	app.buildScheduledTable()

	// Start with table focused, sidebar not
	app.sidebar.SetFocused(false)
	app.scheduledTable.SetFocused(true)

	// Move down
	downKey := tea.KeyPressMsg{Code: tea.KeyDown}
	app.Update(downKey)
	if app.scheduledTable.Cursor() != 1 {
		t.Errorf("cursor should be 1 after down, got %d", app.scheduledTable.Cursor())
	}

	// Move down again
	app.Update(downKey)
	if app.scheduledTable.Cursor() != 2 {
		t.Errorf("cursor should be 2 after two downs, got %d", app.scheduledTable.Cursor())
	}

	// Move up
	upKey := tea.KeyPressMsg{Code: tea.KeyUp}
	app.Update(upKey)
	if app.scheduledTable.Cursor() != 1 {
		t.Errorf("cursor should be 1 after up, got %d", app.scheduledTable.Cursor())
	}
}

func TestApp_HandleScheduledKeys_TabFocus(t *testing.T) {
	accountID := types.NewID()

	app := &App{
		currentView: ViewScheduled,
		width:       120,
		height:      30,
		styles:      NewStyles(),
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		scheduled: &scheduledViewData{
			allTxns:       []*scheduled.Transaction{},
			dueCount:      0,
			payeeNames:    make(map[types.ID]string),
			accountNames:  map[types.ID]string{accountID: "Checking"},
			categoryNames: make(map[types.ID]string),
		},
	}
	app.buildScheduledTable()

	// Start with table focused
	app.sidebar.SetFocused(false)
	app.scheduledTable.SetFocused(true)

	// Tab should switch focus to sidebar
	tabKey := tea.KeyPressMsg{Code: tea.KeyTab}
	app.Update(tabKey)

	if !app.sidebar.IsFocused() {
		t.Error("sidebar should be focused after Tab")
	}
	if app.scheduledTable.IsFocused() {
		t.Error("scheduled table should not be focused after Tab")
	}

	// Tab again should switch back to table
	app.Update(tabKey)

	if app.sidebar.IsFocused() {
		t.Error("sidebar should not be focused after second Tab")
	}
	if !app.scheduledTable.IsFocused() {
		t.Error("scheduled table should be focused after second Tab")
	}
}

func TestApp_Update_ScheduledViewDataLoaded(t *testing.T) {
	app := &App{
		currentView: ViewScheduled,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
	}

	accountID := types.NewID()
	payeeID := types.NewID()

	data := &scheduledViewData{
		allTxns: []*scheduled.Transaction{
			{
				BaseModel: types.BaseModel{ID: types.NewID()},
				AccountID: accountID,
				Frequency: scheduled.FrequencyMonthly,
				NextDate:  types.Today(),
				PayeeID:   types.NullableID{ID: payeeID, Valid: true},
				Amount:    types.NullableMoney{Money: types.MustNewMoney("-100"), Valid: true},
			},
		},
		dueCount:      1,
		payeeNames:    map[types.ID]string{payeeID: "Landlord"},
		accountNames:  map[types.ID]string{accountID: "Checking"},
		categoryNames: make(map[types.ID]string),
	}

	msg := scheduledViewDataLoadedMsg{data: data}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	if cmd != nil {
		t.Error("scheduledViewDataLoadedMsg should not return a command")
	}
	if updatedApp.scheduled == nil {
		t.Fatal("scheduled data should be set")
	}
	if len(updatedApp.scheduled.allTxns) != 1 {
		t.Errorf("expected 1 scheduled txn, got %d", len(updatedApp.scheduled.allTxns))
	}
	if updatedApp.scheduledTable == nil {
		t.Fatal("scheduled table should be created")
	}
	if updatedApp.scheduledTable.RowCount() != 1 {
		t.Errorf("scheduled table row count = %d, want 1", updatedApp.scheduledTable.RowCount())
	}
}

func TestApp_FormatScheduledRow_AllFrequencies(t *testing.T) {
	accountID := types.NewID()

	frequencies := []struct {
		freq     scheduled.Frequency
		expected string
	}{
		{scheduled.FrequencyDaily, "Daily"},
		{scheduled.FrequencyWeekly, "Weekly"},
		{scheduled.FrequencyBiweekly, "Biweekly"},
		{scheduled.FrequencyMonthly, "Monthly"},
		{scheduled.FrequencyQuarterly, "Quarterly"},
		{scheduled.FrequencyYearly, "Yearly"},
	}

	for _, tt := range frequencies {
		t.Run(string(tt.freq), func(t *testing.T) {
			app := &App{
				styles: NewStyles(),
				scheduled: &scheduledViewData{
					payeeNames:    make(map[types.ID]string),
					accountNames:  map[types.ID]string{accountID: "Checking"},
					categoryNames: make(map[types.ID]string),
				},
			}

			st := &scheduled.Transaction{
				BaseModel: types.BaseModel{ID: types.NewID()},
				AccountID: accountID,
				Frequency: tt.freq,
				NextDate:  types.Today(),
				Amount:    types.NullableMoney{Money: types.MustNewMoney("-25"), Valid: true},
			}

			row := app.formatScheduledRow(st, false)
			if row[4] != tt.expected {
				t.Errorf("frequency = %q, want %q", row[4], tt.expected)
			}
		})
	}
}

// =============================================================================
// Reports View Tests
// =============================================================================

func TestApp_RenderReports_Loading(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewReports,
		width:       100,
		height:      30,
		styles:      styles,
		reports:     nil,
	}

	view := app.renderReports()
	if !contains(view, "Loading") {
		t.Errorf("renderReports() should show loading when data is nil, got: %q", view)
	}
}

func TestApp_RenderNetWorthReport(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 30)

	app := &App{
		currentView: ViewReports,
		width:       120,
		height:      30,
		styles:      styles,
		reports: &reportsViewData{
			rtype: reportTypeNetWorth,
			netWorth: &report.NetWorth{
				AsOfDate: types.Today().Time(),
				Assets: []report.AccountBalance{
					{Name: "Checking", Balance: types.MustNewMoney("5000.00")},
					{Name: "Savings", Balance: types.MustNewMoney("10000.00")},
				},
				Liabilities: []report.AccountBalance{
					{Name: "Visa", Balance: types.MustNewMoney("1500.00")},
				},
				TotalAssets:      types.MustNewMoney("15000.00"),
				TotalLiabilities: types.MustNewMoney("1500.00"),
				NetWorth:         types.MustNewMoney("13500.00"),
			},
		},
	}

	view := app.renderNetWorthReport()

	if !contains(view, "NET WORTH REPORT") {
		t.Error("renderNetWorthReport() should contain 'NET WORTH REPORT'")
	}
	if !contains(view, "$13500.00") {
		t.Error("renderNetWorthReport() should contain net worth '$13500.00'")
	}
	if !contains(view, "ASSETS") {
		t.Error("renderNetWorthReport() should contain 'ASSETS'")
	}
	if !contains(view, "LIABILITIES") {
		t.Error("renderNetWorthReport() should contain 'LIABILITIES'")
	}
	if !contains(view, "Checking") {
		t.Error("renderNetWorthReport() should contain 'Checking'")
	}
	if !contains(view, "Savings") {
		t.Error("renderNetWorthReport() should contain 'Savings'")
	}
	if !contains(view, "Visa") {
		t.Error("renderNetWorthReport() should contain 'Visa'")
	}
}

func TestApp_RenderNetWorthReport_NegativeNetWorth(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewReports,
		width:       100,
		height:      30,
		styles:      styles,
		reports: &reportsViewData{
			rtype: reportTypeNetWorth,
			netWorth: &report.NetWorth{
				AsOfDate:         types.Today().Time(),
				Assets:           nil,
				Liabilities:      []report.AccountBalance{{Name: "Loan", Balance: types.MustNewMoney("5000.00")}},
				TotalAssets:      types.MustNewMoney("0"),
				TotalLiabilities: types.MustNewMoney("5000.00"),
				NetWorth:         types.MustNewMoney("-5000.00"),
			},
		},
	}

	view := app.renderNetWorthReport()
	if !contains(view, "-$5000.00") {
		t.Error("renderNetWorthReport() should show negative net worth")
	}
}

func TestApp_RenderNetWorthReport_NoData(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewReports,
		width:       100,
		height:      30,
		styles:      styles,
		reports: &reportsViewData{
			rtype:    reportTypeNetWorth,
			netWorth: nil,
		},
	}

	view := app.renderNetWorthReport()
	if !contains(view, "No net worth data") {
		t.Error("renderNetWorthReport() should show 'No net worth data' when nil")
	}
}

func TestApp_RenderSpendingReport(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 30)

	app := &App{
		currentView: ViewReports,
		width:       120,
		height:      30,
		styles:      styles,
		reports: &reportsViewData{
			rtype: reportTypeSpending,
			year:  2024,
			month: 1,
			spending: &report.Spending{
				Period:        "January 2024",
				TotalSpending: types.MustNewMoney("3000.00"),
				Categories: []report.CategorySpending{
					{
						Name:       "Housing",
						Amount:     types.MustNewMoney("1500.00"),
						Percentage: 50.0,
						Subcategories: []report.CategorySpending{
							{Name: "Rent", Amount: types.MustNewMoney("1500.00")},
						},
					},
					{
						Name:       "Food",
						Amount:     types.MustNewMoney("1000.00"),
						Percentage: 33.3,
						Subcategories: []report.CategorySpending{
							{Name: "Groceries", Amount: types.MustNewMoney("700.00")},
							{Name: "Restaurants", Amount: types.MustNewMoney("300.00")},
						},
					},
					{
						Name:       "Transportation",
						Amount:     types.MustNewMoney("500.00"),
						Percentage: 16.7,
					},
				},
			},
		},
	}

	view := app.renderSpendingReport()

	if !contains(view, "SPENDING BY CATEGORY") {
		t.Error("renderSpendingReport() should contain 'SPENDING BY CATEGORY'")
	}
	if !contains(view, "January 2024") {
		t.Error("renderSpendingReport() should contain 'January 2024'")
	}
	if !contains(view, "Housing") {
		t.Error("renderSpendingReport() should contain 'Housing'")
	}
	if !contains(view, "Food") {
		t.Error("renderSpendingReport() should contain 'Food'")
	}
	if !contains(view, "Transportation") {
		t.Error("renderSpendingReport() should contain 'Transportation'")
	}
	if !contains(view, "Rent") {
		t.Error("renderSpendingReport() should contain subcategory 'Rent'")
	}
	if !contains(view, "Groceries") {
		t.Error("renderSpendingReport() should contain subcategory 'Groceries'")
	}
	if !contains(view, "TOTAL") {
		t.Error("renderSpendingReport() should contain 'TOTAL'")
	}
	if !contains(view, "$3000.00") {
		t.Error("renderSpendingReport() should contain total '$3000.00'")
	}
}

func TestApp_RenderSpendingReport_Empty(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewReports,
		width:       100,
		height:      30,
		styles:      styles,
		reports: &reportsViewData{
			rtype: reportTypeSpending,
			year:  2024,
			month: 6,
			spending: &report.Spending{
				Period:        "June 2024",
				Categories:    nil,
				TotalSpending: types.ZeroMoney,
			},
		},
	}

	view := app.renderSpendingReport()
	if !contains(view, "No spending data") {
		t.Error("renderSpendingReport() should show 'No spending data' when empty")
	}
}

func TestApp_RenderSpendingReport_NoData(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewReports,
		width:       100,
		height:      30,
		styles:      styles,
		reports: &reportsViewData{
			rtype:    reportTypeSpending,
			spending: nil,
		},
	}

	view := app.renderSpendingReport()
	if !contains(view, "No spending data") {
		t.Error("renderSpendingReport() should show 'No spending data' when nil")
	}
}

func TestRenderSpendingBar(t *testing.T) {
	tests := []struct {
		name     string
		pct      float64
		maxWidth int
		filled   int
		unfilled int
	}{
		{"50% of 20", 50.0, 20, 10, 10},
		{"100% of 10", 100.0, 10, 10, 0},
		{"0% of 10", 0.0, 10, 0, 10},
		{"25% of 20", 25.0, 20, 5, 15},
		{"zero width", 50.0, 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bar := renderSpendingBar(tt.pct, tt.maxWidth)
			if tt.maxWidth == 0 {
				if bar != "" {
					t.Errorf("expected empty string for zero width, got %q", bar)
				}
				return
			}
			expectedLen := tt.maxWidth
			// Count runes since we use Unicode block chars
			runeCount := 0
			for range bar {
				runeCount++
			}
			if runeCount != expectedLen {
				t.Errorf("bar length = %d runes, want %d", runeCount, expectedLen)
			}
		})
	}
}

func TestApp_GetAdjacentPeriods_Monthly(t *testing.T) {
	app := &App{
		reports: &reportsViewData{
			year:  2024,
			month: 3, // March
		},
	}

	prev, next := app.getAdjacentPeriods()
	if !contains(prev, "Feb") || !contains(prev, "2024") {
		t.Errorf("previous period = %q, want Feb 2024", prev)
	}
	if !contains(next, "Apr") || !contains(next, "2024") {
		t.Errorf("next period = %q, want Apr 2024", next)
	}
}

func TestApp_GetAdjacentPeriods_MonthlyYearWrap(t *testing.T) {
	// January wraps to December of previous year
	app := &App{
		reports: &reportsViewData{
			year:  2024,
			month: 1,
		},
	}

	prev, next := app.getAdjacentPeriods()
	if !contains(prev, "Dec") || !contains(prev, "2023") {
		t.Errorf("previous period = %q, want Dec 2023", prev)
	}
	if !contains(next, "Feb") || !contains(next, "2024") {
		t.Errorf("next period = %q, want Feb 2024", next)
	}

	// December wraps to January of next year
	app.reports.month = 12
	prev, next = app.getAdjacentPeriods()
	if !contains(prev, "Nov") || !contains(prev, "2024") {
		t.Errorf("previous period = %q, want Nov 2024", prev)
	}
	if !contains(next, "Jan") || !contains(next, "2025") {
		t.Errorf("next period = %q, want Jan 2025", next)
	}
}

func TestApp_GetAdjacentPeriods_Yearly(t *testing.T) {
	app := &App{
		reports: &reportsViewData{
			year:  2024,
			month: 0, // yearly
		},
	}

	prev, next := app.getAdjacentPeriods()
	if prev != "2023" {
		t.Errorf("previous period = %q, want %q", prev, "2023")
	}
	if next != "2025" {
		t.Errorf("next period = %q, want %q", next, "2025")
	}
}

func TestApp_GetAdjacentPeriods_Nil(t *testing.T) {
	app := &App{
		reports: nil,
	}

	prev, next := app.getAdjacentPeriods()
	if prev != "" || next != "" {
		t.Errorf("expected empty strings for nil reports, got %q, %q", prev, next)
	}
}

func TestApp_HandleReportsKeys_SwitchReportTypes(t *testing.T) {
	app := &App{
		currentView: ViewReports,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		reports: &reportsViewData{
			rtype: reportTypeNetWorth,
			year:  2024,
			month: 6,
		},
	}

	// Press 's' to switch to spending
	sKey := tea.KeyPressMsg{Code: 's', Text: "s"}
	_, cmd := app.Update(sKey)
	if cmd == nil {
		t.Error("pressing 's' should return a command to load spending data")
	}

	// Now set to spending and press 'n' to switch to net worth
	app.reports.rtype = reportTypeSpending
	nKey := tea.KeyPressMsg{Code: 'n', Text: "n"}
	_, cmd = app.Update(nKey)
	if cmd == nil {
		t.Error("pressing 'n' should return a command to load net worth data")
	}
}

func TestApp_HandleReportsKeys_PeriodNavigation(t *testing.T) {
	app := &App{
		currentView: ViewReports,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		reports: &reportsViewData{
			rtype: reportTypeSpending,
			year:  2024,
			month: 6,
		},
	}

	// Press left to go to previous period
	leftKey := tea.KeyPressMsg{Code: tea.KeyLeft}
	_, cmd := app.Update(leftKey)
	if cmd == nil {
		t.Error("pressing left should return a command for previous period")
	}

	// Press right to go to next period
	rightKey := tea.KeyPressMsg{Code: tea.KeyRight}
	_, cmd = app.Update(rightKey)
	if cmd == nil {
		t.Error("pressing right should return a command for next period")
	}
}

func TestApp_HandleReportsKeys_PeriodNav_NetWorthIgnored(t *testing.T) {
	app := &App{
		currentView: ViewReports,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		reports: &reportsViewData{
			rtype: reportTypeNetWorth,
			year:  2024,
			month: 6,
		},
	}

	// Period navigation should be ignored for net worth reports
	leftKey := tea.KeyPressMsg{Code: tea.KeyLeft}
	_, cmd := app.Update(leftKey)
	if cmd != nil {
		t.Error("period navigation should be ignored for net worth reports")
	}
}

func TestApp_HandleReportsKeys_YearlyToggle(t *testing.T) {
	app := &App{
		currentView: ViewReports,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		reports: &reportsViewData{
			rtype: reportTypeSpending,
			year:  2024,
			month: 6,
		},
	}

	// Press 'y' to toggle to yearly view
	yKey := tea.KeyPressMsg{Code: 'y', Text: "y"}
	_, cmd := app.Update(yKey)
	if cmd == nil {
		t.Error("pressing 'y' should return a command to load yearly data")
	}
}

func TestApp_HandleReportsKeys_MonthlyToggle(t *testing.T) {
	app := &App{
		currentView: ViewReports,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		reports: &reportsViewData{
			rtype: reportTypeSpending,
			year:  2024,
			month: 0, // yearly
		},
	}

	// Press 'm' to toggle to monthly view
	mKey := tea.KeyPressMsg{Code: 'm', Text: "m"}
	_, cmd := app.Update(mKey)
	if cmd == nil {
		t.Error("pressing 'm' should return a command to load monthly data")
	}
}

func TestApp_HandleReportsKeys_NilReports(t *testing.T) {
	app := &App{
		currentView: ViewReports,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		reports:     nil,
	}

	// Should not panic
	leftKey := tea.KeyPressMsg{Code: tea.KeyLeft}
	_, cmd := app.Update(leftKey)
	if cmd != nil {
		t.Error("should return nil command when reports is nil")
	}
}

func TestApp_Update_ReportsViewDataLoaded(t *testing.T) {
	app := &App{
		currentView: ViewReports,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	data := &reportsViewData{
		rtype: reportTypeNetWorth,
		netWorth: &report.NetWorth{
			NetWorth: types.MustNewMoney("10000"),
		},
	}

	msg := reportsViewDataLoadedMsg{data: data}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	if cmd != nil {
		t.Error("reportsViewDataLoadedMsg should not return a command")
	}
	if updatedApp.reports == nil {
		t.Fatal("reports data should be set")
	}
	if updatedApp.reports.rtype != reportTypeNetWorth {
		t.Errorf("report type = %v, want net worth", updatedApp.reports.rtype)
	}
}

func TestApp_ReportsPreviousPeriod_Monthly(t *testing.T) {
	app := &App{
		reports: &reportsViewData{
			rtype: reportTypeSpending,
			year:  2024,
			month: 3,
		},
	}

	_, cmd := app.reportsPreviousPeriod()
	if cmd == nil {
		t.Error("reportsPreviousPeriod should return a command")
	}
}

func TestApp_ReportsPreviousPeriod_MonthlyJanuaryWrap(t *testing.T) {
	app := &App{
		reports: &reportsViewData{
			rtype: reportTypeSpending,
			year:  2024,
			month: 1,
		},
	}

	_, cmd := app.reportsPreviousPeriod()
	if cmd == nil {
		t.Error("reportsPreviousPeriod should return a command for January wrap")
	}
}

func TestApp_ReportsNextPeriod_Monthly(t *testing.T) {
	app := &App{
		reports: &reportsViewData{
			rtype: reportTypeSpending,
			year:  2024,
			month: 3,
		},
	}

	_, cmd := app.reportsNextPeriod()
	if cmd == nil {
		t.Error("reportsNextPeriod should return a command")
	}
}

func TestApp_ReportsNextPeriod_MonthlyDecemberWrap(t *testing.T) {
	app := &App{
		reports: &reportsViewData{
			rtype: reportTypeSpending,
			year:  2024,
			month: 12,
		},
	}

	_, cmd := app.reportsNextPeriod()
	if cmd == nil {
		t.Error("reportsNextPeriod should return a command for December wrap")
	}
}

func TestApp_ReportsPeriodNav_Nil(t *testing.T) {
	app := &App{
		reports: nil,
	}

	_, cmd := app.reportsPreviousPeriod()
	if cmd != nil {
		t.Error("reportsPreviousPeriod should return nil for nil reports")
	}

	_, cmd = app.reportsNextPeriod()
	if cmd != nil {
		t.Error("reportsNextPeriod should return nil for nil reports")
	}
}

func TestApp_ReportsPeriodNav_NetWorthIgnored(t *testing.T) {
	app := &App{
		reports: &reportsViewData{
			rtype: reportTypeNetWorth,
		},
	}

	_, cmd := app.reportsPreviousPeriod()
	if cmd != nil {
		t.Error("reportsPreviousPeriod should return nil for net worth")
	}

	_, cmd = app.reportsNextPeriod()
	if cmd != nil {
		t.Error("reportsNextPeriod should return nil for net worth")
	}
}

func TestApp_RenderReports_DispatchesCorrectly(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 30)

	// Test net worth dispatch
	app := &App{
		currentView: ViewReports,
		width:       120,
		height:      30,
		styles:      styles,
		reports: &reportsViewData{
			rtype: reportTypeNetWorth,
			netWorth: &report.NetWorth{
				AsOfDate:         types.Today().Time(),
				TotalAssets:      types.MustNewMoney("1000"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("1000"),
			},
		},
	}

	view := app.renderReports()
	if !contains(view, "NET WORTH REPORT") {
		t.Error("renderReports() should dispatch to net worth report")
	}

	// Test spending dispatch
	app.reports = &reportsViewData{
		rtype: reportTypeSpending,
		year:  2024,
		month: 1,
		spending: &report.Spending{
			Period:        "January 2024",
			Categories:    nil,
			TotalSpending: types.ZeroMoney,
		},
	}

	view = app.renderReports()
	if !contains(view, "SPENDING BY CATEGORY") {
		t.Error("renderReports() should dispatch to spending report")
	}
}

// =============================================================================
// Error Display and Dismissal Tests
// =============================================================================

func TestApp_Update_ErrorDismissedByKeyPress(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		err:         fmt.Errorf("some error"),
	}

	// Any key press should clear the error
	msg := tea.KeyPressMsg{Code: 'a', Text: "a"}
	model, cmd := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.err != nil {
		t.Error("error should be cleared after key press")
	}
	if cmd != nil {
		t.Error("dismissing error should not return a command")
	}
}

func TestApp_Update_ErrorDismissedByEnter(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		err:         fmt.Errorf("some error"),
	}

	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.err != nil {
		t.Error("error should be cleared after Enter key")
	}
}

func TestApp_Update_ErrorDismissedByEscape(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		err:         fmt.Errorf("some error"),
	}

	msg := tea.KeyPressMsg{Code: tea.KeyEsc}
	model, _ := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.err != nil {
		t.Error("error should be cleared after Escape key")
	}
}

func TestApp_Update_ErrorDismissedBySpace(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		err:         fmt.Errorf("some error"),
	}

	msg := tea.KeyPressMsg{Code: tea.KeySpace}
	model, _ := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.err != nil {
		t.Error("error should be cleared after Space key")
	}
}

func TestApp_Update_ErrorDoesNotQuit(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		err:         fmt.Errorf("some error"),
	}

	// Ctrl+Q should dismiss the error, not quit the app
	msg := tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl}
	model, cmd := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.err != nil {
		t.Error("error should be cleared after Ctrl+Q")
	}
	if updatedApp.quitting {
		t.Error("app should not quit when dismissing an error")
	}
	if cmd != nil {
		t.Error("dismissing error should not return a command")
	}
}

func TestApp_Update_ErrMsg(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	msg := errMsg{err: fmt.Errorf("test error")}
	model, cmd := app.Update(msg)

	updatedApp := model.(*App)
	if updatedApp.err == nil {
		t.Error("err should be set after errMsg")
	}
	if updatedApp.err.Error() != "test error" {
		t.Errorf("err = %q, want %q", updatedApp.err.Error(), "test error")
	}
	if cmd != nil {
		t.Error("errMsg should not return a command")
	}
}

func TestApp_Update_ErrorThenNormalOperation(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		err:         fmt.Errorf("some error"),
	}

	// First key press dismisses the error
	msg := tea.KeyPressMsg{Code: 'a', Text: "a"}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.err != nil {
		t.Fatal("error should be cleared after first key press")
	}

	// Second key press should work normally (not get stuck)
	msg = tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl}
	model, cmd := updatedApp.Update(msg)
	updatedApp = model.(*App)

	if !updatedApp.quitting {
		t.Error("app should quit on Ctrl+Q after error is dismissed")
	}
	if cmd == nil {
		t.Error("Ctrl+Q should return tea.Quit command")
	}
}

func TestApp_RenderRegister_LongAccountName(t *testing.T) {
	styles := NewStyles()
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
	styles := NewStyles()
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

func TestApp_RenderNetWorthReport_ImprovedNoData(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewReports,
		width:       100,
		height:      30,
		styles:      styles,
		reports: &reportsViewData{
			rtype:    reportTypeNetWorth,
			netWorth: nil,
		},
	}

	view := app.renderNetWorthReport()
	if !contains(view, "Add accounts to get started") {
		t.Error("renderNetWorthReport() should show helpful message when nil")
	}
}

func TestApp_RenderSpendingReport_ImprovedNoData(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewReports,
		width:       100,
		height:      30,
		styles:      styles,
		reports: &reportsViewData{
			rtype:    reportTypeSpending,
			spending: nil,
		},
	}

	view := app.renderSpendingReport()
	if !contains(view, "Add transactions to see reports") {
		t.Error("renderSpendingReport() should show helpful message when nil")
	}
}

// =============================================================================
// Transaction Status TUI Tests (Task 062)
// =============================================================================

func TestApp_BuildRegisterTable_VoidStatusIndicator(t *testing.T) {
	accountID := types.NewID()

	app := &App{
		styles: NewStyles(),
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test"},
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
		styles: NewStyles(),
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test"},
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

	// Void row (index 1) should have RowStyleVoid
	if style, ok := app.table.rowStyles[1]; !ok || style != RowStyleVoid {
		t.Errorf("void row style = %v (ok=%v), want RowStyleVoid", style, ok)
	}

	// Non-void rows should not have a style override
	if _, ok := app.table.rowStyles[0]; ok {
		t.Error("cleared row should not have a style override")
	}
	if _, ok := app.table.rowStyles[2]; ok {
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
				styles: NewStyles(),
				register: &registerData{
					account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test"},
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
		statusbar:      NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test"},
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
		statusbar:      NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test"},
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
		statusbar:      NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test"},
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
		statusbar:      NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test"},
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
		statusbar:      NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test"},
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
		statusbar:      NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test"},
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
	// The message should mention "transfer"
	errorMsg := app.confirmDialog.ErrorMsg()
	if !contains(errorMsg, "transfer") {
		t.Errorf("dialog message = %q, should mention 'transfer'", errorMsg)
	}
}

func TestApp_VoidKey_InRegisterView(t *testing.T) {
	accountID := types.NewID()

	sidebar := NewSidebar()
	sidebar.SetFocused(false)

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      NewStatusBar(),
		sidebar:        sidebar,
		transactionSvc: &transaction.Service{},
		register: &registerData{
			account: &account.Account{BaseModel: types.BaseModel{ID: accountID}, Name: "Test"},
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
		statusbar:   NewStatusBar(),
	}
	_, cmd := app.showVoidConfirmation()
	if cmd != nil {
		t.Error("showVoidConfirmation() should return nil when table is nil")
	}

	// Nil register
	app.table = NewTable([]Column{{Header: "A", Width: 10}})
	_, cmd = app.showVoidConfirmation()
	if cmd != nil {
		t.Error("showVoidConfirmation() should return nil when register is nil")
	}
}

// =============================================================================
// Dashboard Investment Holdings Tests (SM-175)
// =============================================================================

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestApp_Update_ToastClearMsg covers TH-031's clearing leg: a
// ToastClearMsg (delivered by the tea.Cmd ClearToastCmd produces) must
// drop whatever toast is currently set on the status bar. We pre-set a
// toast on a minimally-constructed App, dispatch the message through
// Update, and assert the status bar is back to no-toast state.
func TestApp_Update_ToastClearMsg(t *testing.T) {
	a := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
		styles:      NewStyles(),
		width:       80,
		height:      24,
	}
	a.statusbar.SetToast("hello", NotificationInfo)
	if a.statusbar.Toast() == nil {
		t.Fatal("precondition: SetToast did not register a toast")
	}

	model, cmd := a.Update(ToastClearMsg{})
	if cmd != nil {
		t.Errorf("Update(ToastClearMsg) cmd = %T, want nil", cmd)
	}
	got := model.(*App)
	if got.statusbar.Toast() != nil {
		t.Errorf("Toast() = %+v after ToastClearMsg, want nil", got.statusbar.Toast())
	}
}

