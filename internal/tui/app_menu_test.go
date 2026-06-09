package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

func TestApp_SwitchView(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
	}

	// Switch to Register view
	app.switchView(ViewRegister)
	if app.currentView != ViewRegister {
		t.Errorf("currentView = %v, want %v", app.currentView, ViewRegister)
	}
	if app.previousView != ViewDashboard {
		t.Errorf("previousView = %v, want %v", app.previousView, ViewDashboard)
	}

	// Switch to same view should not change previousView
	app.switchView(ViewRegister)
	if app.previousView != ViewDashboard {
		t.Errorf("previousView should not change when switching to same view")
	}

	// Switch to Scheduled view
	app.switchView(ViewScheduled)
	if app.currentView != ViewScheduled {
		t.Errorf("currentView = %v, want %v", app.currentView, ViewScheduled)
	}
	if app.previousView != ViewRegister {
		t.Errorf("previousView = %v, want %v", app.previousView, ViewRegister)
	}
}

func TestApp_Update_AltKeyMenuShortcuts(t *testing.T) {
	tests := []struct {
		name          string
		key           tea.KeyMsg
		expectedMenu  int
		expectedLabel string
	}{
		{
			"Alt+F opens File menu",
			tea.KeyPressMsg{Code: 'f', Mod: tea.ModAlt},
			0, "File",
		},
		{
			"Alt+E opens Edit menu",
			tea.KeyPressMsg{Code: 'e', Mod: tea.ModAlt},
			1, "Edit",
		},
		{
			"Alt+V opens View menu",
			tea.KeyPressMsg{Code: 'v', Mod: tea.ModAlt},
			2, "View",
		},
		{
			"Alt+A opens Accounts menu",
			tea.KeyPressMsg{Code: 'a', Mod: tea.ModAlt},
			3, "Accounts",
		},
		{
			"Alt+T opens Transactions menu",
			tea.KeyPressMsg{Code: 't', Mod: tea.ModAlt},
			4, "Transactions",
		},
		{
			"Alt+S opens Securities menu",
			tea.KeyPressMsg{Code: 's', Mod: tea.ModAlt},
			5, "Securities",
		},
		{
			"Alt+R opens Reports menu",
			tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt},
			6, "Reports",
		},
		{
			"Alt+H opens Help menu",
			tea.KeyPressMsg{Code: 'h', Mod: tea.ModAlt},
			7, "Help",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &App{
				currentView: ViewDashboard,
				keys:        defaultKeyMap(),
				menubar:     widget.NewMenuBar(),
				statusbar:   widget.NewStatusBar(),
			}

			model, _ := app.Update(tt.key)
			updatedApp := model.(*App)

			if !updatedApp.menubar.IsActive() {
				t.Error("menu bar should be active")
			}
			if updatedApp.menubar.Cursor() != tt.expectedMenu {
				t.Errorf("menu cursor = %d, want %d", updatedApp.menubar.Cursor(), tt.expectedMenu)
			}
		})
	}
}

func TestApp_ToggleMenu_ClosesSameMenu(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
	}

	// Open File menu
	altF := tea.KeyPressMsg{Code: 'f', Mod: tea.ModAlt}
	model, _ := app.Update(altF)
	updatedApp := model.(*App)

	if !updatedApp.menubar.IsActive() {
		t.Fatal("menu should be active after Alt+F")
	}

	// Press Alt+F again to close it
	model, _ = updatedApp.Update(altF)
	updatedApp = model.(*App)

	if updatedApp.menubar.IsActive() {
		t.Error("menu should be deactivated after toggling same menu")
	}
}

func TestApp_ToggleMenu_SwitchesToDifferentMenu(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
	}

	// Open File menu
	altF := tea.KeyPressMsg{Code: 'f', Mod: tea.ModAlt}
	model, _ := app.Update(altF)
	updatedApp := model.(*App)

	if updatedApp.menubar.Cursor() != 0 {
		t.Fatal("should be on File menu")
	}

	// Press Alt+A to switch to Accounts
	altA := tea.KeyPressMsg{Code: 'a', Mod: tea.ModAlt}
	model, _ = updatedApp.Update(altA)
	updatedApp = model.(*App)

	if !updatedApp.menubar.IsActive() {
		t.Error("menu should still be active")
	}
	if updatedApp.menubar.Cursor() != 3 {
		t.Errorf("menu cursor = %d, want 3 (Accounts)", updatedApp.menubar.Cursor())
	}
}

func TestApp_SwitchView_UpdatesStatusBar(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
	}

	app.switchView(ViewScheduled)

	if app.statusbar.Context() != "Scheduled" {
		t.Errorf("statusbar context = %q, want %q", app.statusbar.Context(), "Scheduled")
	}
}

func TestApp_SwitchView_Register_SetsFocus(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		table:       widget.NewTable([]widget.Column{{Header: "Test", Width: 10}}),
	}

	app.sidebar.SetFocused(true)
	app.table.SetFocused(false)

	app.switchView(ViewRegister)

	if app.sidebar.IsFocused() {
		t.Error("sidebar should not be focused in register view")
	}
	if !app.table.IsFocused() {
		t.Error("table should be focused in register view")
	}
}

func TestApp_SwitchView_Dashboard_SetsFocus(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		table:       widget.NewTable([]widget.Column{{Header: "Test", Width: 10}}),
	}

	app.sidebar.SetFocused(false)
	app.table.SetFocused(true)

	app.switchView(ViewDashboard)

	if !app.sidebar.IsFocused() {
		t.Error("sidebar should be focused in dashboard view")
	}
	if app.table.IsFocused() {
		t.Error("table should not be focused in dashboard view")
	}
}

func TestApp_SwitchView_Scheduled_SetsFocus(t *testing.T) {
	app := &App{
		currentView:    ViewDashboard,
		keys:           defaultKeyMap(),
		statusbar:      widget.NewStatusBar(),
		sidebar:        NewSidebar(),
		scheduledTable: widget.NewTable([]widget.Column{{Header: "Test", Width: 10}}),
	}

	app.sidebar.SetFocused(true)
	app.scheduledTable.SetFocused(false)

	app.switchView(ViewScheduled)

	if app.sidebar.IsFocused() {
		t.Error("sidebar should not be focused in scheduled view")
	}
	if !app.scheduledTable.IsFocused() {
		t.Error("scheduled table should be focused in scheduled view")
	}
}

func TestApp_SwitchView_Reports_SetsFocus(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		table:       widget.NewTable([]widget.Column{{Header: "Test", Width: 10}}),
	}

	app.sidebar.SetFocused(true)
	app.table.SetFocused(true)

	app.switchView(ViewReports)

	if app.sidebar.IsFocused() {
		t.Error("sidebar should not be focused in reports view")
	}
	if app.table.IsFocused() {
		t.Error("table should not be focused in reports view")
	}
}

// TestTransactionsMenu_NewPaycheckSchedule_Item covers MS-026: the
// Transactions menu must include a `New Paycheck Schedule…` item that,
// when activated, opens the paycheck wizard.
//
// The test has two parts:
//
//  1. Structural: defaultMenus() returns a Transactions menu (index 4)
//     whose items include the wizard entry alongside the existing
//     New Transaction / New Transfer / etc rows.
//
//  2. Behavioral: dispatching widget.MenuActionNewPaycheckSchedule from the
//     menu action handler returns a tea.Cmd that fetches accounts +
//     categories and emits a paycheckWizardDataMsg. Dispatching that
//     message through Update constructs the wizard on the App.
//
// Save logic and the computed-remainder line land in MS-027 / MS-028.
// MS-026 only needs the menu entry to open the wizard.
func TestTransactionsMenu_NewPaycheckSchedule_Item(t *testing.T) {
	// (1) Structural — the menu item is present.
	menus := widget.DefaultMenus()
	txnMenu := menus[4]
	if txnMenu.Label != "Transactions" {
		t.Fatalf("expected Transactions menu at index 4, got %q", txnMenu.Label)
	}
	foundLabel := false
	foundAction := false
	for _, item := range txnMenu.Items {
		if item.Label == "New Paycheck Schedule..." {
			foundLabel = true
		}
		if item.Action == widget.MenuActionNewPaycheckSchedule {
			foundAction = true
		}
	}
	if !foundLabel {
		t.Errorf("Transactions menu should include `New Paycheck Schedule...` item; got %+v", txnMenu.Items)
	}
	if !foundAction {
		t.Errorf("Transactions menu should include widget.MenuActionNewPaycheckSchedule action")
	}

	// (2) Behavioral — activating the action opens the wizard.
	database := dbtest.New(t)

	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	payeeRepo := payee.NewRepository(database)
	schedRepo := scheduled.NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	splitTxnRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)

	txnSvc := transaction.NewService(txnRepo, splitTxnRepo, transferRepo, payeeRepo, accountRepo, database)
	schedSvc := scheduled.NewService(schedRepo, txnRepo, txnSvc, database, accountRepo)
	accountSvc := account.NewService(accountRepo, database)
	categorySvc := category.NewService(categoryRepo, database)

	// Seed an active account so the wizard's primary-deposit picker has
	// at least one option to land on.
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(acct); err != nil {
		t.Fatalf("Create account: %v", err)
	}

	// Seed the paycheck categories the wizard expects (the file-init
	// path normally does this; for an isolated TUI test we call the
	// service directly).
	if err := categorySvc.EnsurePaycheckCategories(); err != nil {
		t.Fatalf("EnsurePaycheckCategories: %v", err)
	}

	app := &App{
		currentView:     ViewDashboard,
		width:           120,
		height:          30,
		keys:            defaultKeyMap(),
		menubar:         widget.NewMenuBar(),
		statusbar:       widget.NewStatusBar(),
		sidebar:         NewSidebar(),
		styles:          widget.NewStyles(),
		accountSvc:      accountSvc,
		categorySvc:     categorySvc,
		scheduledTxnSvc: schedSvc,
		transactionSvc:  txnSvc,
	}

	// Dispatch the menu action directly (avoids depending on dropdown
	// navigation details — the action enum is the contract).
	_, cmd := app.handleMenuAction(widget.MenuActionNewPaycheckSchedule, "")
	if cmd == nil {
		t.Fatal("widget.MenuActionNewPaycheckSchedule should return a tea.Cmd to load wizard data")
	}

	// Synchronous side-effect: nothing yet. The wizard is constructed
	// when the data message is dispatched through Update.
	if app.paycheckWizard != nil {
		t.Error("paycheck wizard should not be set synchronously — the loader runs as a tea.Cmd")
	}

	msg := cmd()
	if msg == nil {
		t.Fatal("paycheck-wizard loader command should produce a message")
	}
	dataMsg, ok := msg.(paycheckWizardDataMsg)
	if !ok {
		t.Fatalf("expected paycheckWizardDataMsg, got %T", msg)
	}
	if len(dataMsg.accounts) == 0 {
		t.Error("wizard data should include the seeded Checking account")
	}
	if len(dataMsg.categoryOptions) == 0 {
		t.Error("wizard data should include category options seeded by EnsurePaycheckCategories")
	}

	// Dispatch the message — Update constructs the wizard.
	model, _ := app.Update(dataMsg)
	final := model.(*App)

	if final.paycheckWizard == nil {
		t.Fatal("paycheckWizard should be set after paycheckWizardDataMsg")
	}
	if !final.paycheckWizard.IsVisible() {
		t.Error("paycheck wizard should be visible after construction")
	}
}
