package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
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

func TestDefaultKeyMap(t *testing.T) {
	km := defaultKeyMap()

	// Verify key bindings are set
	if len(km.Quit.Keys()) == 0 {
		t.Error("Quit key binding not set")
	}
	if len(km.Help.Keys()) == 0 {
		t.Error("Help key binding not set")
	}
	if len(km.Up.Keys()) == 0 {
		t.Error("Up key binding not set")
	}
	if len(km.Down.Keys()) == 0 {
		t.Error("Down key binding not set")
	}
	if len(km.Enter.Keys()) == 0 {
		t.Error("Enter key binding not set")
	}
	if len(km.Escape.Keys()) == 0 {
		t.Error("Escape key binding not set")
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

func TestApp_SwitchView(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
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

func TestApp_View_NotReady(t *testing.T) {
	app := &App{
		ready: false,
	}

	view := app.View()
	if view.Content != "Loading..." {
		t.Errorf("View().Content = %q, want %q", view.Content, "Loading...")
	}
}

func TestApp_View_Quitting(t *testing.T) {
	app := &App{
		ready:    true,
		quitting: true,
	}

	view := app.View()
	if view.Content != "Goodbye!\n" {
		t.Errorf("View().Content = %q, want %q", view.Content, "Goodbye!\n")
	}
}

func TestApp_GetKeyHints(t *testing.T) {
	tests := []struct {
		view     View
		contains string
	}{
		{ViewDashboard, "dashboard"},
		{ViewRegister, "esc back"},
		{ViewScheduled, "post"},
		{ViewReports, "period"},
	}

	for _, tt := range tests {
		t.Run(tt.view.String(), func(t *testing.T) {
			app := &App{
				currentView: tt.view,
			}

			hints := app.getKeyHints()
			if hints == "" {
				t.Error("getKeyHints() should not return empty string")
			}
			// All views should have common hint
			if !contains(hints, "ctrl+q quit") {
				t.Errorf("getKeyHints() should contain 'ctrl+q quit', got: %s", hints)
			}
		})
	}
}

func TestApp_RenderLayout(t *testing.T) {
	styles := NewStyles()
	styles.Resize(80, 24)
	app := &App{
		currentView: ViewDashboard,
		width:       80,
		height:      24,
		ready:       true,
		styles:      styles,
		sidebar:     NewSidebar(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		keys:        defaultKeyMap(),
	}

	layout := app.renderLayout()
	if layout == "" {
		t.Error("renderLayout() should not return empty string")
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
			"Alt+A opens Accounts menu",
			tea.KeyPressMsg{Code: 'a', Mod: tea.ModAlt},
			2, "Accounts",
		},
		{
			"Alt+T opens Transactions menu",
			tea.KeyPressMsg{Code: 't', Mod: tea.ModAlt},
			3, "Transactions",
		},
		{
			"Alt+S opens Securities menu",
			tea.KeyPressMsg{Code: 's', Mod: tea.ModAlt},
			4, "Securities",
		},
		{
			"Alt+R opens Reports menu",
			tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt},
			5, "Reports",
		},
		{
			"Alt+H opens Help menu",
			tea.KeyPressMsg{Code: 'h', Mod: tea.ModAlt},
			6, "Help",
		},
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
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
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
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
	if updatedApp.menubar.Cursor() != 2 {
		t.Errorf("menu cursor = %d, want 2 (Accounts)", updatedApp.menubar.Cursor())
	}
}

func TestDefaultKeyMap_MenuShortcuts(t *testing.T) {
	km := defaultKeyMap()

	if len(km.MenuFile.Keys()) == 0 {
		t.Error("MenuFile key binding not set")
	}
	if len(km.MenuAccounts.Keys()) == 0 {
		t.Error("MenuAccounts key binding not set")
	}
	if len(km.MenuTransactions.Keys()) == 0 {
		t.Error("MenuTransactions key binding not set")
	}
	if len(km.MenuReports.Keys()) == 0 {
		t.Error("MenuReports key binding not set")
	}
	if len(km.MenuHelp.Keys()) == 0 {
		t.Error("MenuHelp key binding not set")
	}
}

func TestApp_GetKeyHints_IncludesAltKey(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
	}

	hints := app.getKeyHints()
	if !contains(hints, "Alt+key") {
		t.Errorf("getKeyHints() should contain 'Alt+key', got: %s", hints)
	}
}

func TestApp_SwitchView_UpdatesStatusBar(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
	}

	app.switchView(ViewScheduled)

	if app.statusbar.Context() != "Scheduled" {
		t.Errorf("statusbar context = %q, want %q", app.statusbar.Context(), "Scheduled")
	}
}

func TestFormatDashboardMoney(t *testing.T) {
	tests := []struct {
		name     string
		money    types.Money
		expected string
	}{
		{"positive", types.MustNewMoney("1234.56"), "$1234.56"},
		{"negative", types.MustNewMoney("-50.00"), "-$50.00"},
		{"zero", types.MustNewMoney("0"), "$0.00"},
		{"large", types.MustNewMoney("99999.99"), "$99999.99"},
		{"small negative", types.MustNewMoney("-0.50"), "-$0.50"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDashboardMoney(tt.money)
			if got != tt.expected {
				t.Errorf("formatDashboardMoney(%v) = %q, want %q", tt.money, got, tt.expected)
			}
		})
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		input    string
		width    int
		expected string
	}{
		{"abc", 6, "abc   "},
		{"abc", 3, "abc"},
		{"abc", 1, "abc"}, // already wider, no padding
		{"", 4, "    "},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := padRight(tt.input, tt.width)
			if got != tt.expected {
				t.Errorf("padRight(%q, %d) = %q, want %q", tt.input, tt.width, got, tt.expected)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is too long", 10, "this is..."},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"}, // maxLen <= 3, no ellipsis
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.expected {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.expected)
			}
		})
	}
}

func TestApp_RenderDashboard_Loading(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		width:       100,
		height:      30,
		styles:      NewStyles(),
		dashboard:   nil, // not loaded yet
	}
	app.styles.Resize(100, 30)

	view := app.renderDashboard()
	if !contains(view, "Loading") {
		t.Errorf("renderDashboard() should show loading when data is nil, got: %q", view)
	}
}

func TestApp_RenderDashboard_WithData(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 30)
	app := &App{
		currentView: ViewDashboard,
		width:       120,
		height:      30,
		styles:      styles,
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
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
			dueTxns:      nil,
			upcomingTxns: nil,
			payeeNames:   make(map[types.ID]string),
			accountNames: make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	// Check that key elements are present
	if !contains(view, "DASHBOARD") {
		t.Error("renderDashboard() should contain 'DASHBOARD'")
	}
	if !contains(view, "$13500.00") {
		t.Error("renderDashboard() should contain net worth '$13500.00'")
	}
	if !contains(view, "ASSETS") {
		t.Error("renderDashboard() should contain 'ASSETS'")
	}
	if !contains(view, "LIABILITIES") {
		t.Error("renderDashboard() should contain 'LIABILITIES'")
	}
	if !contains(view, "Checking") {
		t.Error("renderDashboard() should contain 'Checking'")
	}
	if !contains(view, "Savings") {
		t.Error("renderDashboard() should contain 'Savings'")
	}
	if !contains(view, "Visa") {
		t.Error("renderDashboard() should contain 'Visa'")
	}
	if !contains(view, "SCHEDULED") {
		t.Error("renderDashboard() should contain 'SCHEDULED'")
	}
}

func TestApp_RenderDashboard_NegativeNetWorth(t *testing.T) {
	styles := NewStyles()
	styles.Resize(100, 30)
	app := &App{
		currentView: ViewDashboard,
		width:       100,
		height:      30,
		styles:      styles,
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets:           nil,
				Liabilities:      []report.AccountBalance{{Name: "Loan", Balance: types.MustNewMoney("5000.00")}},
				TotalAssets:      types.MustNewMoney("0"),
				TotalLiabilities: types.MustNewMoney("5000.00"),
				NetWorth:         types.MustNewMoney("-5000.00"),
			},
			payeeNames:   make(map[types.ID]string),
			accountNames: make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	if !contains(view, "-$5000.00") {
		t.Error("renderDashboard() should show negative net worth")
	}
	if !contains(view, "(none)") {
		t.Error("renderDashboard() should show '(none)' for empty assets")
	}
}

func TestApp_RenderDashboard_WithScheduled(t *testing.T) {
	payeeID := types.NewID()
	styles := NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewDashboard,
		width:       100,
		height:      30,
		styles:      styles,
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				TotalAssets:      types.MustNewMoney("1000"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("1000"),
			},
			dueTxns: []*scheduled.Transaction{
				{
					BaseModel: types.BaseModel{ID: types.NewID()},
					PayeeID:   types.NullableID{ID: payeeID, Valid: true},
					Amount:    types.NullableMoney{Money: types.MustNewMoney("-1500.00"), Valid: true},
					NextDate:  types.Today(),
				},
			},
			upcomingTxns: nil,
			payeeNames:   map[types.ID]string{payeeID: "Landlord"},
			accountNames: make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	if !contains(view, "1 due") {
		t.Error("renderDashboard() should show '1 due' in scheduled header")
	}
	if !contains(view, "Landlord") {
		t.Error("renderDashboard() should show payee name 'Landlord'")
	}
	if !contains(view, "$1500.00") {
		t.Error("renderDashboard() should show amount '$1500.00'")
	}
	if !contains(view, "due today") {
		t.Error("renderDashboard() should show 'due today'")
	}
}

func TestApp_RenderDashboard_EmptyData(t *testing.T) {
	styles := NewStyles()
	styles.Resize(80, 24)
	app := &App{
		currentView: ViewDashboard,
		width:       80,
		height:      24,
		styles:      styles,
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				TotalAssets:      types.ZeroMoney,
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.ZeroMoney,
			},
			payeeNames:   make(map[types.ID]string),
			accountNames: make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	if !contains(view, "$0.00") {
		t.Error("renderDashboard() should show '$0.00' for zero net worth")
	}
	if !contains(view, "No scheduled") {
		t.Error("renderDashboard() should show 'No scheduled' when there are none")
	}
}

func TestApp_Update_DashboardLoaded(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	data := &dashboardData{
		netWorth: &report.NetWorth{
			NetWorth: types.MustNewMoney("5000"),
		},
		payeeNames:   make(map[types.ID]string),
		accountNames: make(map[types.ID]string),
	}

	msg := dashboardLoadedMsg{data: data}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	if cmd != nil {
		t.Error("dashboardLoadedMsg should not return a command")
	}
	if updatedApp.dashboard == nil {
		t.Fatal("dashboard data should be set")
	}
	if !updatedApp.dashboard.netWorth.NetWorth.Equal(types.MustNewMoney("5000")) {
		t.Error("dashboard net worth should be $5000")
	}
}

func TestApp_RenderDashboard_SmallWidth(t *testing.T) {
	styles := NewStyles()
	styles.Resize(60, 20)
	app := &App{
		currentView: ViewDashboard,
		width:       60,
		height:      20,
		styles:      styles,
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets:           []report.AccountBalance{{Name: "Checking", Balance: types.MustNewMoney("100")}},
				TotalAssets:      types.MustNewMoney("100"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("100"),
			},
			payeeNames:   make(map[types.ID]string),
			accountNames: make(map[types.ID]string),
		},
	}

	// Should not panic on small width
	view := app.renderDashboard()
	if view == "" {
		t.Error("renderDashboard() should not return empty string on small width")
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

func TestApp_SwitchView_Register_SetsFocus(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		table:       NewTable([]Column{{Header: "Test", Width: 10}}),
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
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		table:       NewTable([]Column{{Header: "Test", Width: 10}}),
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

func TestApp_SwitchView_Scheduled_SetsFocus(t *testing.T) {
	app := &App{
		currentView:    ViewDashboard,
		keys:           defaultKeyMap(),
		statusbar:      NewStatusBar(),
		sidebar:        NewSidebar(),
		scheduledTable: NewTable([]Column{{Header: "Test", Width: 10}}),
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

func TestApp_SwitchView_Reports_SetsFocus(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
		sidebar:     NewSidebar(),
		table:       NewTable([]Column{{Header: "Test", Width: 10}}),
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

func TestApp_GetKeyHints_Reports(t *testing.T) {
	app := &App{
		currentView: ViewReports,
	}

	hints := app.getKeyHints()
	if !contains(hints, "net worth") {
		t.Error("reports key hints should mention 'net worth'")
	}
	if !contains(hints, "spending") {
		t.Error("reports key hints should mention 'spending'")
	}
	if !contains(hints, "period") {
		t.Error("reports key hints should mention 'period'")
	}
}

// =============================================================================
// Error Display and Dismissal Tests
// =============================================================================

func TestApp_View_Error(t *testing.T) {
	styles := NewStyles()
	styles.Resize(80, 24)
	app := &App{
		ready:  true,
		styles: styles,
		err:    fmt.Errorf("failed to open database: not a valid file"),
	}

	view := app.View()
	if !contains(view.Content, "Error") {
		t.Error("View() should contain 'Error' when err is set")
	}
	if !contains(view.Content, "failed to open database") {
		t.Error("View() should contain the error message")
	}
	if !contains(view.Content, "Press any key to continue") {
		t.Error("View() should contain 'Press any key to continue'")
	}
}

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

func TestApp_RenderDashboard_NilNetWorth(t *testing.T) {
	styles := NewStyles()
	styles.Resize(80, 24)
	app := &App{
		currentView: ViewDashboard,
		width:       80,
		height:      24,
		styles:      styles,
		dashboard: &dashboardData{
			netWorth:     nil,
			payeeNames:   make(map[types.ID]string),
			accountNames: make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	if !contains(view, "No account data available") {
		t.Error("renderDashboard() should show 'No account data available' when netWorth is nil")
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

func TestApp_HandleConfirmDialogKey_Cancel(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
	}

	d := NewDialog("Confirm")
	d.SetButtons([]DialogButton{
		{Label: "No"},
		{Label: "Yes", Primary: true},
	})
	d.SetVisible(true)
	app.confirmDialog = d
	app.confirmAction = func() tea.Msg { return nil }

	// Press Escape to cancel
	_, _ = app.handleConfirmDialogKey(tea.KeyPressMsg{Code: tea.KeyEsc})

	if app.confirmDialog != nil {
		t.Error("confirmDialog should be nil after cancel")
	}
	if app.confirmAction != nil {
		t.Error("confirmAction should be nil after cancel")
	}
}

func TestApp_HandleConfirmDialogKey_Confirm(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
	}

	called := false
	d := NewDialog("Confirm")
	d.SetButtons([]DialogButton{
		{Label: "No"},
		{Label: "Yes", Primary: true},
	})
	d.SetVisible(true)
	// Focus on the Yes button (fields count = 0, so button index 1 = focus index 1)
	d.SetFocusIndex(1)
	app.confirmDialog = d
	app.confirmAction = func() tea.Msg {
		called = true
		return nil
	}

	// Press Enter on Yes button
	_, cmd := app.handleConfirmDialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	if app.confirmDialog != nil {
		t.Error("confirmDialog should be nil after confirm")
	}
	if cmd == nil {
		t.Error("should return a cmd after confirm")
	}

	// Execute the cmd to verify action was captured
	if cmd != nil {
		cmd()
		if !called {
			t.Error("confirm action should have been called")
		}
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

func TestApp_KeyHints_RegisterIncludesVoid(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
	}

	hints := app.getKeyHints()
	if !contains(hints, "v void") {
		t.Errorf("key hints = %q, should include 'v void'", hints)
	}
	if !contains(hints, "c clear") {
		t.Errorf("key hints = %q, should include 'c clear'", hints)
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
// Undo/Redo TUI Integration Tests
// =============================================================================

func TestApp_UndoKeyBinding(t *testing.T) {
	mgr := undo.NewManager()
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		undoManager: mgr,
	}

	// Press Ctrl+Z with nothing to undo
	msg := tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl}
	_, cmd := app.handleKeyPress(msg)

	if cmd == nil {
		t.Fatal("Ctrl+Z should return a command")
	}

	// Execute the command to get the result message
	result := cmd()
	undoResult, ok := result.(undoResultMsg)
	if !ok {
		t.Fatalf("expected undoResultMsg, got %T", result)
	}
	if undoResult.action != "Undo" {
		t.Errorf("action = %q, want %q", undoResult.action, "Undo")
	}
	if !errors.Is(undoResult.err, undo.ErrNothingToUndo) {
		t.Errorf("err = %v, want ErrNothingToUndo", undoResult.err)
	}
}

func TestApp_RedoKeyBinding(t *testing.T) {
	mgr := undo.NewManager()
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		undoManager: mgr,
	}

	// Press Ctrl+Y with nothing to redo
	msg := tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl}
	_, cmd := app.handleKeyPress(msg)

	if cmd == nil {
		t.Fatal("Ctrl+Y should return a command")
	}

	result := cmd()
	undoResult, ok := result.(undoResultMsg)
	if !ok {
		t.Fatalf("expected undoResultMsg, got %T", result)
	}
	if undoResult.action != "Redo" {
		t.Errorf("action = %q, want %q", undoResult.action, "Redo")
	}
	if !errors.Is(undoResult.err, undo.ErrNothingToRedo) {
		t.Errorf("err = %v, want ErrNothingToRedo", undoResult.err)
	}
}

func TestApp_UndoResultMsg_NothingToUndo(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	msg := undoResultMsg{action: "Undo", err: undo.ErrNothingToUndo}
	_, cmd := app.Update(msg)

	if cmd != nil {
		t.Error("nothing-to-undo should not trigger a reload command")
	}

	notifications := app.statusbar.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if notifications[0].Text != "Nothing to undo" {
		t.Errorf("notification = %q, want %q", notifications[0].Text, "Nothing to undo")
	}
}

func TestApp_UndoResultMsg_NothingToRedo(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	msg := undoResultMsg{action: "Redo", err: undo.ErrNothingToRedo}
	_, cmd := app.Update(msg)

	if cmd != nil {
		t.Error("nothing-to-redo should not trigger a reload command")
	}

	notifications := app.statusbar.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if notifications[0].Text != "Nothing to redo" {
		t.Errorf("notification = %q, want %q", notifications[0].Text, "Nothing to redo")
	}
}

func TestApp_UndoResultMsg_Success(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	msg := undoResultMsg{action: "Undo", description: "Create transaction"}
	_, cmd := app.Update(msg)

	// Should trigger a reload
	if cmd == nil {
		t.Error("successful undo should trigger a reload command")
	}

	notifications := app.statusbar.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if notifications[0].Text != "Undo: Create transaction" {
		t.Errorf("notification = %q, want %q", notifications[0].Text, "Undo: Create transaction")
	}
}

func TestApp_RedoResultMsg_Success(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	msg := undoResultMsg{action: "Redo", description: "Delete account"}
	_, cmd := app.Update(msg)

	if cmd == nil {
		t.Error("successful redo should trigger a reload command")
	}

	notifications := app.statusbar.Notifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if notifications[0].Text != "Redo: Delete account" {
		t.Errorf("notification = %q, want %q", notifications[0].Text, "Redo: Delete account")
	}
}

func TestApp_UndoResultMsg_Error(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	msg := undoResultMsg{action: "Undo", err: fmt.Errorf("database error")}
	_, _ = app.Update(msg)

	// Error should be set on the app for display
	if app.err == nil {
		t.Error("error should be set on the app")
	}
	if app.err.Error() != "database error" {
		t.Errorf("err = %q, want %q", app.err.Error(), "database error")
	}
}

func TestApp_PerformUndo_NilManager(t *testing.T) {
	app := &App{
		undoManager: nil,
	}

	cmd := app.performUndo()
	if cmd != nil {
		t.Error("performUndo with nil manager should return nil")
	}
}

func TestApp_PerformRedo_NilManager(t *testing.T) {
	app := &App{
		undoManager: nil,
	}

	cmd := app.performRedo()
	if cmd != nil {
		t.Error("performRedo with nil manager should return nil")
	}
}

func TestApp_MenuUndo(t *testing.T) {
	mgr := undo.NewManager()
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		undoManager: mgr,
	}

	// Simulate menu action
	_, cmd := app.handleMenuAction(MenuActionUndo)
	if cmd == nil {
		t.Fatal("MenuActionUndo should return a command")
	}

	// Menu should be deactivated
	if app.menubar.IsActive() {
		t.Error("menu should be deactivated after undo")
	}
}

func TestApp_MenuRedo(t *testing.T) {
	mgr := undo.NewManager()
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		undoManager: mgr,
	}

	_, cmd := app.handleMenuAction(MenuActionRedo)
	if cmd == nil {
		t.Fatal("MenuActionRedo should return a command")
	}

	if app.menubar.IsActive() {
		t.Error("menu should be deactivated after redo")
	}
}

func TestApp_UndoKeyBindingNotActiveInDialogs(t *testing.T) {
	mgr := undo.NewManager()
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
		undoManager: mgr,
	}

	// Open a transaction dialog
	app.txnDialog = NewDialog("Test")
	app.txnDialog.AddTextField("Name", "", "", 0)
	app.txnDialog.SetVisible(true)
	app.txnDialogData = &transactionDialogData{}
	app.txnDialogCategoryIDs = []types.ID{}

	// Press Ctrl+Z - should be routed to dialog, not undo
	msg := tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl}
	_, cmd := app.handleKeyPress(msg)

	// The dialog should handle it (Ctrl+Z is not a dialog action, so it may just be consumed)
	// The key point is: it should NOT route to performUndo
	if cmd != nil {
		result := cmd()
		if _, ok := result.(undoResultMsg); ok {
			t.Error("Ctrl+Z should not trigger undo when dialog is open")
		}
	}
}

// =============================================================================
// Dashboard Investment Holdings Tests (SM-175)
// =============================================================================

func TestApp_RenderDashboard_InvestmentAccountWithHoldings(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 40)

	investAccountID := types.NewID()
	securityID1 := types.NewID()
	securityID2 := types.NewID()

	app := &App{
		currentView:               ViewDashboard,
		width:                     120,
		height:                    40,
		styles:                    styles,
		dashboardExpandedAccounts: map[types.ID]bool{investAccountID: true},
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets: []report.AccountBalance{
					{AccountID: types.NewID(), Name: "Checking", Type: "checking", Balance: types.MustNewMoney("5000.00")},
					{AccountID: investAccountID, Name: "Brokerage", Type: "investment", Balance: types.MustNewMoney("25000.00")},
				},
				Liabilities:      nil,
				TotalAssets:      types.MustNewMoney("30000.00"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("30000.00"),
			},
			investmentHoldings: map[types.ID]*investment.AccountValuation{
				investAccountID: {
					AccountID:   investAccountID,
					CashBalance: types.MustNewMoney("5000.00"),
					MarketValue: types.MustNewMoney("20000.00"),
					TotalValue:  types.MustNewMoney("25000.00"),
					Holdings: []investment.Holding{
						{SecurityID: securityID1, MarketValue: types.MustNewMoney("12000.00"), HasPricing: true},
						{SecurityID: securityID2, MarketValue: types.MustNewMoney("8000.00"), HasPricing: true},
					},
				},
			},
			securityTickers: map[types.ID]string{
				securityID1: "AAPL",
				securityID2: "GOOG",
			},
			payeeNames:   make(map[types.ID]string),
			accountNames: make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	// Investment account should show total value
	if !contains(view, "Brokerage") {
		t.Error("dashboard should show investment account name 'Brokerage'")
	}
	if !contains(view, "$25000.00") {
		t.Error("dashboard should show investment account total value '$25000.00'")
	}

	// When expanded, top holdings should be visible
	if !contains(view, "AAPL") {
		t.Error("dashboard should show holding ticker 'AAPL' when expanded")
	}
	if !contains(view, "GOOG") {
		t.Error("dashboard should show holding ticker 'GOOG' when expanded")
	}
	if !contains(view, "$12000.00") {
		t.Error("dashboard should show AAPL market value '$12000.00'")
	}
	if !contains(view, "$8000.00") {
		t.Error("dashboard should show GOOG market value '$8000.00'")
	}

	// Regular accounts should still display normally
	if !contains(view, "Checking") {
		t.Error("dashboard should show regular account 'Checking'")
	}
}

func TestApp_RenderDashboard_InvestmentAccountCollapsed(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 40)

	investAccountID := types.NewID()
	securityID := types.NewID()

	app := &App{
		currentView:               ViewDashboard,
		width:                     120,
		height:                    40,
		styles:                    styles,
		dashboardExpandedAccounts: map[types.ID]bool{}, // not expanded
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets: []report.AccountBalance{
					{AccountID: investAccountID, Name: "Brokerage", Type: "investment", Balance: types.MustNewMoney("25000.00")},
				},
				TotalAssets:      types.MustNewMoney("25000.00"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("25000.00"),
			},
			investmentHoldings: map[types.ID]*investment.AccountValuation{
				investAccountID: {
					AccountID:   investAccountID,
					CashBalance: types.MustNewMoney("5000.00"),
					MarketValue: types.MustNewMoney("20000.00"),
					TotalValue:  types.MustNewMoney("25000.00"),
					Holdings: []investment.Holding{
						{SecurityID: securityID, MarketValue: types.MustNewMoney("20000.00"), HasPricing: true},
					},
				},
			},
			securityTickers: map[types.ID]string{securityID: "AAPL"},
			payeeNames:      make(map[types.ID]string),
			accountNames:    make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	// Account total should show
	if !contains(view, "Brokerage") {
		t.Error("dashboard should show investment account name even when collapsed")
	}
	if !contains(view, "$25000.00") {
		t.Error("dashboard should show investment total value even when collapsed")
	}

	// Holdings should NOT show when collapsed
	if contains(view, "AAPL") {
		t.Error("dashboard should NOT show holding tickers when collapsed")
	}
}

func TestApp_RenderDashboard_InvestmentAccountEstimatedValue(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 40)

	investAccountID := types.NewID()
	securityID := types.NewID()

	app := &App{
		currentView:               ViewDashboard,
		width:                     120,
		height:                    40,
		styles:                    styles,
		dashboardExpandedAccounts: map[types.ID]bool{investAccountID: true},
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets: []report.AccountBalance{
					{AccountID: investAccountID, Name: "401k", Type: "investment", Balance: types.MustNewMoney("10000.00"), EstimatedValue: true},
				},
				TotalAssets:      types.MustNewMoney("10000.00"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("10000.00"),
			},
			investmentHoldings: map[types.ID]*investment.AccountValuation{
				investAccountID: {
					AccountID:   investAccountID,
					CashBalance: types.ZeroMoney,
					MarketValue: types.MustNewMoney("10000.00"),
					TotalValue:  types.MustNewMoney("10000.00"),
					Holdings: []investment.Holding{
						{SecurityID: securityID, MarketValue: types.MustNewMoney("10000.00"), HasPricing: false},
					},
				},
			},
			securityTickers: map[types.ID]string{securityID: "VXUS"},
			payeeNames:      make(map[types.ID]string),
			accountNames:    make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	// Estimated value indicator should show
	if !contains(view, "~$10000.00") {
		t.Error("dashboard should show estimated value prefix '~' for accounts with missing pricing")
	}
	// Holding without pricing should show tilde
	if !contains(view, "~$10000.00") {
		t.Error("holding without pricing should show estimated value indicator")
	}
}

func TestApp_RenderDashboard_InvestmentNoHoldings(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 40)

	investAccountID := types.NewID()

	app := &App{
		currentView:               ViewDashboard,
		width:                     120,
		height:                    40,
		styles:                    styles,
		dashboardExpandedAccounts: map[types.ID]bool{investAccountID: true},
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets: []report.AccountBalance{
					{AccountID: investAccountID, Name: "Empty Fund", Type: "investment", Balance: types.MustNewMoney("1000.00")},
				},
				TotalAssets:      types.MustNewMoney("1000.00"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("1000.00"),
			},
			investmentHoldings: map[types.ID]*investment.AccountValuation{
				investAccountID: {
					AccountID:   investAccountID,
					CashBalance: types.MustNewMoney("1000.00"),
					MarketValue: types.ZeroMoney,
					TotalValue:  types.MustNewMoney("1000.00"),
					Holdings:    nil,
				},
			},
			securityTickers: make(map[types.ID]string),
			payeeNames:      make(map[types.ID]string),
			accountNames:    make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	// Should show cash only note when expanded but no holdings
	if !contains(view, "Empty Fund") {
		t.Error("dashboard should show investment account name")
	}
	if !contains(view, "cash only") {
		t.Error("dashboard should show 'cash only' when investment account has no holdings")
	}
}

func TestApp_RenderDashboard_InvestmentTopHoldingsLimit(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 40)

	investAccountID := types.NewID()

	// Create 7 holdings - only top 5 should show
	holdings := make([]investment.Holding, 7)
	tickers := make(map[types.ID]string)
	for i := range 7 {
		secID := types.NewID()
		holdings[i] = investment.Holding{
			SecurityID:  secID,
			MarketValue: types.MustNewMoney(fmt.Sprintf("%d.00", (7-i)*1000)),
			HasPricing:  true,
		}
		tickers[secID] = fmt.Sprintf("STK%d", i+1)
	}

	app := &App{
		currentView:               ViewDashboard,
		width:                     120,
		height:                    40,
		styles:                    styles,
		dashboardExpandedAccounts: map[types.ID]bool{investAccountID: true},
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets: []report.AccountBalance{
					{AccountID: investAccountID, Name: "Big Portfolio", Type: "investment", Balance: types.MustNewMoney("28000.00")},
				},
				TotalAssets:      types.MustNewMoney("28000.00"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("28000.00"),
			},
			investmentHoldings: map[types.ID]*investment.AccountValuation{
				investAccountID: {
					AccountID: investAccountID,
					Holdings:  holdings,
				},
			},
			securityTickers: tickers,
			payeeNames:      make(map[types.ID]string),
			accountNames:    make(map[types.ID]string),
		},
	}

	view := app.renderDashboard()

	// Top 5 should be visible (STK1 through STK5 have highest values)
	if !contains(view, "STK1") {
		t.Error("dashboard should show top holding STK1")
	}
	if !contains(view, "STK5") {
		t.Error("dashboard should show 5th holding STK5")
	}

	// 6th and 7th should be hidden, replaced by "+2 more" indicator
	if contains(view, "STK6") {
		t.Error("dashboard should NOT show 6th holding STK6 (limit is 5)")
	}
	if contains(view, "STK7") {
		t.Error("dashboard should NOT show 7th holding STK7 (limit is 5)")
	}
	if !contains(view, "+2 more") {
		t.Error("dashboard should show '+2 more' indicator for hidden holdings")
	}
}

func TestApp_RenderDashboard_InvestmentHoldingsNilMap(t *testing.T) {
	// Dashboard should handle nil investmentHoldings gracefully
	styles := NewStyles()
	styles.Resize(120, 40)

	investAccountID := types.NewID()

	app := &App{
		currentView:               ViewDashboard,
		width:                     120,
		height:                    40,
		styles:                    styles,
		dashboardExpandedAccounts: map[types.ID]bool{investAccountID: true},
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets: []report.AccountBalance{
					{AccountID: investAccountID, Name: "Brokerage", Type: "investment", Balance: types.MustNewMoney("10000.00")},
				},
				TotalAssets:      types.MustNewMoney("10000.00"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("10000.00"),
			},
			investmentHoldings: nil, // nil map
			securityTickers:    nil,
			payeeNames:         make(map[types.ID]string),
			accountNames:       make(map[types.ID]string),
		},
	}

	// Should not panic
	view := app.renderDashboard()
	if !contains(view, "Brokerage") {
		t.Error("dashboard should show investment account even with nil holdings map")
	}
}

func TestApp_DashboardInvestmentAccountOpensPortfolioView(t *testing.T) {
	// SM-176: Selecting an investment account on the dashboard should open the
	// portfolio view (ViewPortfolio), not the regular register or investment register.
	investAcctID := types.NewID()
	investAcct := &account.Account{
		BaseModel: types.BaseModel{ID: investAcctID},
		Name:      "Brokerage",
		Type:      account.TypeInvestment,
	}

	sidebar := NewSidebar()
	sidebar.SetAccounts([]*account.Account{investAcct}, map[types.ID]*account.Balance{
		investAcctID: {AccountID: investAcctID, CurrentBalance: types.MustNewMoney("10000.00")},
	})
	// Move cursor to the account item (index 0 = group header, index 1 = account)
	sidebar.cursor = 1

	app := &App{
		currentView: ViewDashboard,
		sidebar:     sidebar,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	enterKey := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, cmd := app.Update(enterKey)
	updatedApp := model.(*App)

	if updatedApp.currentView != ViewPortfolio {
		t.Errorf("expected ViewPortfolio (%d), got %v (%d)", ViewPortfolio, updatedApp.currentView, updatedApp.currentView)
	}
	if cmd == nil {
		t.Error("expected a command to load portfolio data")
	}
}

func TestApp_DashboardNonInvestmentAccountOpensRegisterView(t *testing.T) {
	// Verify that non-investment accounts still open the regular register.
	checkAcctID := types.NewID()
	checkAcct := &account.Account{
		BaseModel: types.BaseModel{ID: checkAcctID},
		Name:      "Checking",
		Type:      account.TypeChecking,
	}

	sidebar := NewSidebar()
	sidebar.SetAccounts([]*account.Account{checkAcct}, map[types.ID]*account.Balance{
		checkAcctID: {AccountID: checkAcctID, CurrentBalance: types.MustNewMoney("5000.00")},
	})
	sidebar.cursor = 1

	app := &App{
		currentView: ViewDashboard,
		sidebar:     sidebar,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		statusbar:   NewStatusBar(),
	}

	enterKey := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, cmd := app.Update(enterKey)
	updatedApp := model.(*App)

	if updatedApp.currentView != ViewRegister {
		t.Errorf("expected ViewRegister (%d), got %v (%d)", ViewRegister, updatedApp.currentView, updatedApp.currentView)
	}
	if cmd == nil {
		t.Error("expected a command to load register data")
	}
}

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

func TestApp_HandleSidebarKeys_NewAccountShortcut(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
	}
	// Sidebar is focused by default in NewSidebar()

	msg := tea.KeyPressMsg{Code: 'n', Text: "n"}
	_, cmd := app.Update(msg)

	// The 'n' key in dashboard view (sidebar focused) should return a command
	// to load the new account dialog data
	if cmd == nil {
		t.Error("pressing 'n' in dashboard with sidebar focused should return a command")
	}
}

func TestApp_MouseClick_MenuBar_OpensDropdown(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		width:       80,
		height:      24,
	}
	app.styles.Resize(80, 24)

	// Click on "File" label (x=2, y=0)
	msg := tea.MouseClickMsg{X: 2, Y: 0, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if !updatedApp.menubar.IsActive() {
		t.Error("menu bar should be active after clicking File label")
	}
	if updatedApp.menubar.Cursor() != 0 {
		t.Errorf("cursor = %d, want 0 (File)", updatedApp.menubar.Cursor())
	}
}

func TestApp_MouseClick_MenuBar_ToggleDropdown(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		width:       80,
		height:      24,
	}
	app.styles.Resize(80, 24)

	// Click File to open
	msg := tea.MouseClickMsg{X: 2, Y: 0, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if !updatedApp.menubar.IsActive() {
		t.Fatal("menu should be active after first click")
	}

	// Click File again to close
	model, _ = updatedApp.Update(msg)
	updatedApp = model.(*App)

	if updatedApp.menubar.IsActive() {
		t.Error("menu should be deactivated after clicking same label again")
	}
}

func TestApp_MouseClick_MenuBar_SwitchMenu(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		width:       80,
		height:      24,
	}
	app.styles.Resize(80, 24)

	// Click File to open
	msg := tea.MouseClickMsg{X: 2, Y: 0, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	// Click Edit (x=8, offset for " File " = 6)
	msg = tea.MouseClickMsg{X: 8, Y: 0, Button: tea.MouseLeft}
	model, _ = updatedApp.Update(msg)
	updatedApp = model.(*App)

	if !updatedApp.menubar.IsActive() {
		t.Error("menu should still be active")
	}
	if updatedApp.menubar.Cursor() != 1 {
		t.Errorf("cursor = %d, want 1 (Edit)", updatedApp.menubar.Cursor())
	}
}

func TestApp_MouseClick_Dropdown_SelectsItem(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		width:       80,
		height:      24,
	}
	app.styles.Resize(80, 24)

	// Open Edit menu
	app.menubar.ActivateMenu(1)

	// Click first item in Edit dropdown (Undo) at y=1 (first dropdown row)
	// Edit dropdown offset = 6 (width of " File ")
	msg := tea.MouseClickMsg{X: 8, Y: 1, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	// Menu should be deactivated after selection
	if updatedApp.menubar.IsActive() {
		t.Error("menu should be deactivated after selecting a dropdown item")
	}
}

func TestApp_MouseClick_OutsideMenu_ClosesDropdown(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		width:       80,
		height:      24,
	}
	app.styles.Resize(80, 24)

	// Open File menu
	app.menubar.Activate()

	// Click in content area (far from menu)
	msg := tea.MouseClickMsg{X: 50, Y: 10, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.menubar.IsActive() {
		t.Error("menu should close when clicking outside dropdown")
	}
}

func TestApp_MouseClick_Sidebar_SingleClick_OnlySelects(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)

	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
	}
	app.sidebar.SetAccounts(accounts, nil)
	// items: [Bank Accounts, Checking]

	// Click on the account row (y=2 = content row 1 = Checking item)
	msg := tea.MouseClickMsg{X: 5, Y: 2, Button: tea.MouseLeft}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.sidebar.cursor != 1 {
		t.Errorf("sidebar cursor = %d, want 1", updatedApp.sidebar.cursor)
	}
	// Single click selects only — no open command, view does not switch.
	if cmd != nil {
		t.Error("single click should not return an open command")
	}
	if updatedApp.currentView != ViewDashboard {
		t.Errorf("view should still be Dashboard, got %v", updatedApp.currentView)
	}
}

func TestApp_MouseClick_Sidebar_DoubleClick_OpensAccount(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)

	now := time.Unix(0, 0)
	app.sidebarClicks = NewClickTracker(400 * time.Millisecond)
	app.sidebarClicks.SetNowFn(func() time.Time { return now })

	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
	}
	app.sidebar.SetAccounts(accounts, nil)

	click := tea.MouseClickMsg{X: 5, Y: 2, Button: tea.MouseLeft}

	// First click — selects only.
	_, cmd := app.Update(click)
	if cmd != nil {
		t.Fatal("first click should not return an open command")
	}

	// Second click within threshold on same row — drills in.
	now = now.Add(100 * time.Millisecond)
	model, cmd := app.Update(click)
	updatedApp := model.(*App)

	if cmd == nil {
		t.Fatal("double click should return an open command")
	}
	openMsg, ok := cmd().(mouseOpenAccountMsg)
	if !ok {
		t.Fatalf("expected mouseOpenAccountMsg, got %T", cmd())
	}
	if openMsg.accountID != accounts[0].ID {
		t.Errorf("opened account = %v, want %v", openMsg.accountID, accounts[0].ID)
	}
	// View switch is still deferred — currentView only changes on the message.
	if updatedApp.currentView != ViewDashboard {
		t.Errorf("view should still be Dashboard before message processed, got %v", updatedApp.currentView)
	}
}

func TestApp_MouseOpenAccountMsg_SwitchesView(t *testing.T) {
	checking := testAccount("Checking", account.TypeChecking)
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)
	app.sidebar.SetAccounts([]*account.Account{checking}, nil)
	app.sidebar.MoveDown()
	app.sidebar.Select()

	// Simulate the deferred message
	msg := mouseOpenAccountMsg{accountID: checking.ID}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.currentView != ViewRegister {
		t.Errorf("currentView = %v, want ViewRegister", updatedApp.currentView)
	}
	if cmd == nil {
		t.Error("should return a command to load register data")
	}
}

func TestApp_MouseClick_Sidebar_GroupHeader_JustMovesCursor(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)

	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
		testAccount("Savings", account.TypeSavings),
	}
	app.sidebar.SetAccounts(accounts, nil)
	// items: [Bank Accounts, Checking, Savings] = 3 items

	// Click on group header (y=1 = content row 0 = Bank Accounts)
	msg := tea.MouseClickMsg{X: 5, Y: 1, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	// Items should remain unchanged (no collapse ever)
	if updatedApp.sidebar.ItemCount() != 3 {
		t.Errorf("ItemCount = %d, want 3", updatedApp.sidebar.ItemCount())
	}
	if updatedApp.sidebar.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (group header)", updatedApp.sidebar.cursor)
	}
}

func TestApp_MouseClick_Table_SelectsRow(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		table:       NewTable([]Column{{Header: "A", Width: 10}}),
		register:    &registerData{},
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)
	app.sidebar.SetFocused(false)
	app.table.SetRows([][]string{{"row1"}, {"row2"}, {"row3"}})

	sidebarWidth := app.styles.SidebarWidth()

	// Click on the second data row in the table
	// Table starts at content-relative row 3 (1 padding + 1 title + 1 separator)
	// So screen row = 1 (header) + 3 (offset) + 1 (header row of table) + 1 (second data row) = row 5+1=6
	// Actually: screen Y = 1 (menu bar) + 3 (content offset) + 0 (table header) + 2 (second data row)
	// contentY = Y - 1 = 4, tableY = contentY - 3 = 1, which is the header. For data row 1, we need tableY=2
	// So Y = 1 + 3 + 2 = 6
	msg := tea.MouseClickMsg{
		X:      sidebarWidth + 5,
		Y:      6,
		Button: tea.MouseLeft,
	}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.table.Cursor() != 1 {
		t.Errorf("table cursor = %d, want 1", updatedApp.table.Cursor())
	}
}

func TestApp_MouseClick_FocusSwitchToTable(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		table:       NewTable([]Column{{Header: "A", Width: 10}}),
		register:    &registerData{},
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)
	// Start with sidebar focused
	app.sidebar.SetFocused(true)
	app.table.SetFocused(false)

	sidebarWidth := app.styles.SidebarWidth()

	// Click in content area (right of sidebar)
	msg := tea.MouseClickMsg{
		X:      sidebarWidth + 5,
		Y:      5,
		Button: tea.MouseLeft,
	}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.sidebar.IsFocused() {
		t.Error("sidebar should not be focused after clicking content area")
	}
	if !updatedApp.table.IsFocused() {
		t.Error("table should be focused after clicking content area")
	}
}

func TestApp_MouseClick_FocusSwitchToSidebar(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		table:       NewTable([]Column{{Header: "A", Width: 10}}),
		register:    &registerData{},
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)

	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
	}
	app.sidebar.SetAccounts(accounts, nil)

	// Start with table focused
	app.sidebar.SetFocused(false)
	app.table.SetFocused(true)

	// Click in sidebar area
	msg := tea.MouseClickMsg{X: 5, Y: 2, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if !updatedApp.sidebar.IsFocused() {
		t.Error("sidebar should be focused after clicking in sidebar area")
	}
	if updatedApp.table.IsFocused() {
		t.Error("table should not be focused after clicking in sidebar area")
	}
}

func TestApp_MouseWheel_ScrollsTable(t *testing.T) {
	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		table:       NewTable([]Column{{Header: "A", Width: 10}}),
		register:    &registerData{},
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)
	app.sidebar.SetFocused(false)
	app.table.SetFocused(true)
	app.table.SetRows([][]string{{"a"}, {"b"}, {"c"}, {"d"}, {"e"}})

	// Scroll down
	msg := tea.MouseWheelMsg{X: 50, Y: 10, Button: tea.MouseWheelDown}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.table.Cursor() != 1 {
		t.Errorf("after wheel down, cursor = %d, want 1", updatedApp.table.Cursor())
	}

	// Scroll up
	msg = tea.MouseWheelMsg{X: 50, Y: 10, Button: tea.MouseWheelUp}
	model, _ = updatedApp.Update(msg)
	updatedApp = model.(*App)

	if updatedApp.table.Cursor() != 0 {
		t.Errorf("after wheel up, cursor = %d, want 0", updatedApp.table.Cursor())
	}
}

func TestApp_MouseWheel_ScrollsSidebar(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		width:       100,
		height:      24,
	}
	app.styles.Resize(100, 24)

	accounts := []*account.Account{
		testAccount("Checking", account.TypeChecking),
		testAccount("Savings", account.TypeSavings),
		testAccount("Visa", account.TypeCreditCard),
	}
	app.sidebar.SetAccounts(accounts, nil)
	// Sidebar focused by default

	// Scroll down
	msg := tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelDown}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.sidebar.cursor != 1 {
		t.Errorf("after wheel down, sidebar cursor = %d, want 1", updatedApp.sidebar.cursor)
	}
}

func TestApp_MouseClick_IgnoredDuringHelpOverlay(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		showHelp:    true,
		width:       80,
		height:      24,
	}
	app.styles.Resize(80, 24)

	// Click on menu bar while help overlay is visible
	msg := tea.MouseClickMsg{X: 2, Y: 0, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	// Menu should not open
	if updatedApp.menubar.IsActive() {
		t.Error("menu should not open while help overlay is visible")
	}
}

func TestApp_MouseClick_Dialog_CloseButton(t *testing.T) {
	dlg := NewDialog("Confirm")
	dlg.SetButtons([]DialogButton{{Label: "Cancel"}, {Label: "OK", Primary: true}})
	dlg.SetVisible(true)

	app := &App{
		currentView:   ViewDashboard,
		keys:          defaultKeyMap(),
		menubar:       NewMenuBar(),
		sidebar:       NewSidebar(),
		statusbar:     NewStatusBar(),
		confirmDialog: dlg,
		confirmAction: func() tea.Msg { return nil },
		width:         80,
		height:        24,
	}
	app.styles.Resize(80, 24)

	// Click the [x] close button
	contentWidth := dlg.Width() - dialogHorizontalOverhead
	startCol, startRow, _, _ := dlg.DialogBounds(80, 24)
	clickX := startCol + 3 + contentWidth - 2
	clickY := startRow + 2

	msg := tea.MouseClickMsg{X: clickX, Y: clickY, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.confirmDialog != nil {
		t.Error("confirm dialog should be closed after clicking [x]")
	}
}

func TestApp_MouseClick_Dialog_SubmitButton(t *testing.T) {
	dlg := NewDialog("Confirm")
	dlg.SetButtons([]DialogButton{{Label: "Cancel"}, {Label: "OK", Primary: true}})
	dlg.SetVisible(true)

	submitted := false
	app := &App{
		currentView:   ViewDashboard,
		keys:          defaultKeyMap(),
		menubar:       NewMenuBar(),
		sidebar:       NewSidebar(),
		statusbar:     NewStatusBar(),
		confirmDialog: dlg,
		confirmAction: func() tea.Msg { submitted = true; return nil },
		width:         80,
		height:        24,
	}
	app.styles.Resize(80, 24)

	contentWidth := dlg.Width() - dialogHorizontalOverhead
	buttonRow := dlg.ContentHeight() - 1
	startCol, startRow, _, _ := dlg.DialogBounds(80, 24)

	// Find OK button position
	var okX int
	for x := range contentWidth {
		hit := dlg.HitTestContent(x, buttonRow, contentWidth)
		if hit.Zone == DialogHitButton && hit.ButtonIndex == 1 {
			okX = x
			break
		}
	}

	msg := tea.MouseClickMsg{
		X:      startCol + 3 + okX,
		Y:      startRow + 2 + buttonRow,
		Button: tea.MouseLeft,
	}
	model, cmd := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.confirmDialog != nil {
		t.Error("confirm dialog should be closed after clicking OK")
	}
	// Execute the command to trigger the confirm action
	if cmd != nil {
		cmd()
	}
	if !submitted {
		t.Error("confirm action should have been triggered")
	}
}

func TestApp_MouseClick_Dialog_CancelButton(t *testing.T) {
	dlg := NewDialog("Confirm")
	dlg.SetButtons([]DialogButton{{Label: "Cancel"}, {Label: "OK", Primary: true}})
	dlg.SetVisible(true)

	app := &App{
		currentView:   ViewDashboard,
		keys:          defaultKeyMap(),
		menubar:       NewMenuBar(),
		sidebar:       NewSidebar(),
		statusbar:     NewStatusBar(),
		confirmDialog: dlg,
		confirmAction: func() tea.Msg { return nil },
		width:         80,
		height:        24,
	}
	app.styles.Resize(80, 24)

	contentWidth := dlg.Width() - dialogHorizontalOverhead
	buttonRow := dlg.ContentHeight() - 1
	startCol, startRow, _, _ := dlg.DialogBounds(80, 24)

	// Find Cancel button position
	var cancelX int
	for x := range contentWidth {
		hit := dlg.HitTestContent(x, buttonRow, contentWidth)
		if hit.Zone == DialogHitButton && hit.ButtonIndex == 0 {
			cancelX = x
			break
		}
	}

	msg := tea.MouseClickMsg{
		X:      startCol + 3 + cancelX,
		Y:      startRow + 2 + buttonRow,
		Button: tea.MouseLeft,
	}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.confirmDialog != nil {
		t.Error("confirm dialog should be closed after clicking Cancel")
	}
}

func TestApp_MouseClick_Dialog_OutsideNoAction(t *testing.T) {
	dlg := NewDialog("Confirm")
	dlg.SetVisible(true)

	app := &App{
		currentView:   ViewDashboard,
		keys:          defaultKeyMap(),
		menubar:       NewMenuBar(),
		sidebar:       NewSidebar(),
		statusbar:     NewStatusBar(),
		confirmDialog: dlg,
		confirmAction: func() tea.Msg { return nil },
		width:         80,
		height:        24,
	}
	app.styles.Resize(80, 24)

	// Click outside the dialog (top-left corner)
	msg := tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.confirmDialog == nil || !updatedApp.confirmDialog.IsVisible() {
		t.Error("dialog should remain open when clicking outside")
	}
}

func TestApp_MouseClick_HelpOverlay_StillBlocked(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		showHelp:    true,
		width:       80,
		height:      24,
	}
	app.styles.Resize(80, 24)

	// Click on menu bar while help overlay is visible
	msg := tea.MouseClickMsg{X: 2, Y: 0, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	// Menu should not open
	if updatedApp.menubar.IsActive() {
		t.Error("menu should not open while help overlay is visible")
	}
}

func TestApp_MouseWheel_Dialog_ListField(t *testing.T) {
	dlg := NewDialog("Browse")
	dlg.AddListField("File", []string{"../", "docs/", "main.go", "go.mod", "go.sum"}, 0, 3)
	dlg.SetFocusIndex(0) // Focus on list field
	dlg.SetVisible(true)

	app := &App{
		currentView:   ViewDashboard,
		keys:          defaultKeyMap(),
		menubar:       NewMenuBar(),
		sidebar:       NewSidebar(),
		statusbar:     NewStatusBar(),
		confirmDialog: dlg,
		confirmAction: func() tea.Msg { return nil },
		width:         80,
		height:        24,
	}
	app.styles.Resize(80, 24)

	startCol, startRow, _, _ := dlg.DialogBounds(80, 24)

	// Wheel down within dialog bounds
	msg := tea.MouseWheelMsg{
		X:      startCol + 10,
		Y:      startRow + 5,
		Button: tea.MouseWheelDown,
	}
	app.Update(msg)

	if dlg.Fields()[0].SelectedIndex != 1 {
		t.Errorf("SelectedIndex after wheel down = %d, want 1", dlg.Fields()[0].SelectedIndex)
	}
}

func TestApp_MouseRelease_Ignored(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		width:       80,
		height:      24,
	}
	app.styles.Resize(80, 24)

	// Mouse release should be ignored
	msg := tea.MouseReleaseMsg{X: 2, Y: 0, Button: tea.MouseLeft}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.menubar.IsActive() {
		t.Error("mouse release should not activate menu")
	}
}

func TestApp_HandleSidebarKeys_NewAccountNotWhenUnfocused(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
	}
	app.sidebar.SetFocused(false)

	msg := tea.KeyPressMsg{Code: 'n', Text: "n"}
	_, cmd := app.Update(msg)

	// When sidebar is not focused, 'n' should not trigger anything
	if cmd != nil {
		t.Error("pressing 'n' with sidebar unfocused should not return a command")
	}
}

func TestApp_View_ComponentWidths(t *testing.T) {
	checking := testAccount("Discover Checking", account.TypeChecking)

	for _, termWidth := range []int{100, 120, 160, 200} {
		t.Run(fmt.Sprintf("width=%d", termWidth), func(t *testing.T) {
			app := &App{
				currentView: ViewDashboard,
				keys:        defaultKeyMap(),
				menubar:     NewMenuBar(),
				sidebar:     NewSidebar(),
				statusbar:   NewStatusBar(),
				width:       termWidth,
				height:      24,
				ready:       true,
			}
			app.styles = NewStyles()
			app.styles.Resize(termWidth, 24)
			app.sidebar.SetAccounts([]*account.Account{checking}, nil)

			header := app.renderHeader()
			headerWidth := lipgloss.Width(header)
			headerLines := len(strings.Split(header, "\n"))

			contentHeight := app.height - 2
			content := app.renderContent(contentHeight)
			contentWidth := lipgloss.Width(content)
			contentLines := len(strings.Split(content, "\n"))

			statusBar := app.renderStatusBar()
			statusWidth := lipgloss.Width(statusBar)
			statusLines := len(strings.Split(statusBar, "\n"))

			t.Logf("Terminal width: %d", termWidth)
			t.Logf("Header: %d cols x %d lines", headerWidth, headerLines)
			t.Logf("Content: %d cols x %d lines", contentWidth, contentLines)
			t.Logf("StatusBar: %d cols x %d lines", statusWidth, statusLines)

			view := app.View()
			viewLines := strings.Split(view.Content, "\n")
			maxLineWidth := 0
			for _, line := range viewLines {
				w := lipgloss.Width(line)
				if w > maxLineWidth {
					maxLineWidth = w
				}
			}
			t.Logf("View: %d lines, max line width: %d", len(viewLines), maxLineWidth)

			if maxLineWidth > termWidth {
				t.Errorf("View lines (%d cols) wider than terminal (%d cols)", maxLineWidth, termWidth)
			}
			if len(viewLines) != 24 {
				t.Errorf("View has %d lines, want 24", len(viewLines))
			}
		})
	}
}

func TestApp_View_RegisterLoadedWidths(t *testing.T) {
	checking := testAccount("Discover Checking", account.TypeChecking)

	for _, termWidth := range []int{100, 120, 160, 200} {
		t.Run(fmt.Sprintf("width=%d", termWidth), func(t *testing.T) {
			app := &App{
				currentView: ViewRegister,
				keys:        defaultKeyMap(),
				menubar:     NewMenuBar(),
				sidebar:     NewSidebar(),
				statusbar:   NewStatusBar(),
				width:       termWidth,
				height:      24,
				ready:       true,
				register: &registerData{
					account:       checking,
					balance:       &account.Balance{CurrentBalance: types.MustNewMoney("0.00")},
					transactions:  nil,
					payeeNames:    map[types.ID]string{},
					categoryNames: map[types.ID]string{},
					accountNames:  map[types.ID]string{},
				},
			}
			app.styles = NewStyles()
			app.styles.Resize(termWidth, 24)
			app.sidebar.SetAccounts([]*account.Account{checking}, nil)
			app.sidebar.SetFocused(false)
			// Build the register table
			app.buildRegisterTable()

			view := app.View()
			viewLines := strings.Split(view.Content, "\n")
			maxLineWidth := 0
			widestLine := 0
			for i, line := range viewLines {
				w := lipgloss.Width(line)
				if w > maxLineWidth {
					maxLineWidth = w
					widestLine = i
				}
			}
			t.Logf("View: %d lines, max width: %d (line %d), terminal: %d",
				len(viewLines), maxLineWidth, widestLine, termWidth)

			if maxLineWidth > termWidth {
				t.Errorf("Line %d is %d cols, exceeds terminal width %d", widestLine, maxLineWidth, termWidth)
				t.Logf("Line content: %q", stripAnsi(viewLines[widestLine]))
			}
			if len(viewLines) != 24 {
				t.Errorf("View has %d lines, want 24", len(viewLines))
			}

			// Check component line counts
			header := app.renderHeader()
			hLines := len(strings.Split(header, "\n"))
			contentHeight := app.height - 2
			content := app.renderContent(contentHeight)
			cLines := len(strings.Split(content, "\n"))
			statusBar := app.renderStatusBar()
			sLines := len(strings.Split(statusBar, "\n"))
			t.Logf("Header: %d lines, Content: %d lines (want %d), StatusBar: %d lines",
				hLines, cLines, contentHeight, sLines)
		})
	}
}

func TestApp_View_LineCount_AfterMouseAccountClick(t *testing.T) {
	checking := testAccount("Discover Checking", account.TypeChecking)
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   NewStatusBar(),
		width:       120,
		height:      24,
		ready:       true,
	}
	app.styles = NewStyles()
	app.styles.Resize(120, 24)
	app.sidebar.SetAccounts([]*account.Account{checking}, nil)

	// Step 1: Render dashboard - should have exactly 24 lines
	dashView := app.View()
	dashLines := strings.Split(dashView.Content, "\n")
	t.Logf("Dashboard view: %d lines", len(dashLines))
	if len(dashLines) != 24 {
		t.Errorf("Dashboard View() has %d lines, want 24", len(dashLines))
	}

	// Step 2: Simulate mouse click on account
	clickMsg := tea.MouseClickMsg{X: 5, Y: 2, Button: tea.MouseLeft}
	model, _ := app.Update(clickMsg)
	app = model.(*App)

	// Step 3: Simulate the deferred open account message
	openMsg := mouseOpenAccountMsg{accountID: checking.ID}
	model, _ = app.Update(openMsg)
	app = model.(*App)

	// Step 4: Render register view - should have exactly 24 lines
	regView := app.View()
	regLines := strings.Split(regView.Content, "\n")
	t.Logf("Register view: %d lines", len(regLines))
	if len(regLines) != 24 {
		t.Errorf("Register View() has %d lines, want 24", len(regLines))
	}

	// Check first line contains menu bar content
	if !strings.Contains(regLines[0], "File") && !strings.Contains(regLines[0], "\033") {
		t.Logf("First line (raw bytes): %q", regLines[0])
		t.Logf("First line visual width: %d", lipgloss.Width(regLines[0]))
		t.Errorf("First line should contain menu bar, got visual content: %q", stripAnsi(regLines[0]))
	}

	// Log the first few lines for debugging
	for i := 0; i < min(5, len(regLines)); i++ {
		t.Logf("Line %d (width=%d): %q", i, lipgloss.Width(regLines[i]), stripAnsi(regLines[i]))
	}
}
