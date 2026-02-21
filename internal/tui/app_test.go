package tui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/service"
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

	msg := tea.KeyMsg{Type: tea.KeyCtrlQ}
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
		{"Dashboard key", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}}, ViewDashboard},
		{"Scheduled key", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}, ViewScheduled},
		{"Reports key", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}}, ViewReports},
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

	msg := tea.KeyMsg{Type: tea.KeyEsc}
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
	if view != "Loading..." {
		t.Errorf("View() = %q, want %q", view, "Loading...")
	}
}

func TestApp_View_Quitting(t *testing.T) {
	app := &App{
		ready:    true,
		quitting: true,
	}

	view := app.View()
	if view != "Goodbye!\n" {
		t.Errorf("View() = %q, want %q", view, "Goodbye!\n")
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
			tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}, Alt: true},
			0, "File",
		},
		{
			"Alt+E opens Edit menu",
			tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}, Alt: true},
			1, "Edit",
		},
		{
			"Alt+A opens Accounts menu",
			tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}, Alt: true},
			2, "Accounts",
		},
		{
			"Alt+T opens Transactions menu",
			tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}, Alt: true},
			3, "Transactions",
		},
		{
			"Alt+R opens Reports menu",
			tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}, Alt: true},
			4, "Reports",
		},
		{
			"Alt+H opens Help menu",
			tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}, Alt: true},
			5, "Help",
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
	altF := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}, Alt: true}
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
	altF := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}, Alt: true}
	model, _ := app.Update(altF)
	updatedApp := model.(*App)

	if updatedApp.menubar.Cursor() != 0 {
		t.Fatal("should be on File menu")
	}

	// Press Alt+A to switch to Accounts
	altA := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}, Alt: true}
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
		money    models.Money
		expected string
	}{
		{"positive", models.MustNewMoney("1234.56"), "$1234.56"},
		{"negative", models.MustNewMoney("-50.00"), "-$50.00"},
		{"zero", models.MustNewMoney("0"), "$0.00"},
		{"large", models.MustNewMoney("99999.99"), "$99999.99"},
		{"small negative", models.MustNewMoney("-0.50"), "-$0.50"},
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
			netWorth: &models.NetWorthReport{
				Assets: []models.ReportAccountBalance{
					{Name: "Checking", Balance: models.MustNewMoney("5000.00")},
					{Name: "Savings", Balance: models.MustNewMoney("10000.00")},
				},
				Liabilities: []models.ReportAccountBalance{
					{Name: "Visa", Balance: models.MustNewMoney("1500.00")},
				},
				TotalAssets:      models.MustNewMoney("15000.00"),
				TotalLiabilities: models.MustNewMoney("1500.00"),
				NetWorth:         models.MustNewMoney("13500.00"),
			},
			dueTxns:      nil,
			upcomingTxns: nil,
			payeeNames:   make(map[models.ID]string),
			accountNames: make(map[models.ID]string),
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
			netWorth: &models.NetWorthReport{
				Assets:           nil,
				Liabilities:      []models.ReportAccountBalance{{Name: "Loan", Balance: models.MustNewMoney("5000.00")}},
				TotalAssets:      models.MustNewMoney("0"),
				TotalLiabilities: models.MustNewMoney("5000.00"),
				NetWorth:         models.MustNewMoney("-5000.00"),
			},
			payeeNames:   make(map[models.ID]string),
			accountNames: make(map[models.ID]string),
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
	payeeID := models.NewID()
	styles := NewStyles()
	styles.Resize(100, 30)

	app := &App{
		currentView: ViewDashboard,
		width:       100,
		height:      30,
		styles:      styles,
		dashboard: &dashboardData{
			netWorth: &models.NetWorthReport{
				TotalAssets:      models.MustNewMoney("1000"),
				TotalLiabilities: models.ZeroMoney,
				NetWorth:         models.MustNewMoney("1000"),
			},
			dueTxns: []*models.ScheduledTransaction{
				{
					BaseModel: models.BaseModel{ID: models.NewID()},
					PayeeID:   models.NullableID{ID: payeeID, Valid: true},
					Amount:    models.NullableMoney{Money: models.MustNewMoney("-1500.00"), Valid: true},
					NextDate:  models.Today(),
				},
			},
			upcomingTxns: nil,
			payeeNames:   map[models.ID]string{payeeID: "Landlord"},
			accountNames: make(map[models.ID]string),
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
			netWorth: &models.NetWorthReport{
				TotalAssets:      models.ZeroMoney,
				TotalLiabilities: models.ZeroMoney,
				NetWorth:         models.ZeroMoney,
			},
			payeeNames:   make(map[models.ID]string),
			accountNames: make(map[models.ID]string),
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
		netWorth: &models.NetWorthReport{
			NetWorth: models.MustNewMoney("5000"),
		},
		payeeNames:   make(map[models.ID]string),
		accountNames: make(map[models.ID]string),
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
	if !updatedApp.dashboard.netWorth.NetWorth.Equal(models.MustNewMoney("5000")) {
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
			netWorth: &models.NetWorthReport{
				Assets:           []models.ReportAccountBalance{{Name: "Checking", Balance: models.MustNewMoney("100")}},
				TotalAssets:      models.MustNewMoney("100"),
				TotalLiabilities: models.ZeroMoney,
				NetWorth:         models.MustNewMoney("100"),
			},
			payeeNames:   make(map[models.ID]string),
			accountNames: make(map[models.ID]string),
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

	accountID := models.NewID()
	payeeID := models.NewID()
	categoryID := models.NewID()

	app := &App{
		currentView: ViewRegister,
		width:       120,
		height:      30,
		styles:      styles,
		register: &registerData{
			account: &models.Account{
				BaseModel: models.BaseModel{ID: accountID},
				Name:      "Checking",
			},
			transactions: []*models.Transaction{
				{
					BaseModel:  models.BaseModel{ID: models.NewID()},
					AccountID:  accountID,
					Date:       models.Today(),
					Amount:     models.MustNewMoney("-125.43"),
					Status:     models.TransactionStatusCleared,
					PayeeID:    models.NullableID{ID: payeeID, Valid: true},
					CategoryID: models.NullableID{ID: categoryID, Valid: true},
				},
				{
					BaseModel: models.BaseModel{ID: models.NewID()},
					AccountID: accountID,
					Date:      models.Today(),
					Amount:    models.MustNewMoney("2500.00"),
					Status:    models.TransactionStatusUncleared,
					PayeeID:   models.NullableID{ID: payeeID, Valid: true},
				},
			},
			balance: &service.AccountBalance{
				AccountID:      accountID,
				CurrentBalance: models.MustNewMoney("5234.57"),
				ClearedBalance: models.MustNewMoney("5000.00"),
			},
			payeeNames:    map[models.ID]string{payeeID: "Kroger"},
			categoryNames: map[models.ID]string{categoryID: "Groceries"},
			accountNames:  make(map[models.ID]string),
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

	accountID := models.NewID()

	app := &App{
		currentView: ViewRegister,
		width:       100,
		height:      30,
		styles:      styles,
		register: &registerData{
			account: &models.Account{
				BaseModel: models.BaseModel{ID: accountID},
				Name:      "Savings",
			},
			transactions:  []*models.Transaction{},
			balance:       &service.AccountBalance{AccountID: accountID, CurrentBalance: models.ZeroMoney},
			payeeNames:    make(map[models.ID]string),
			categoryNames: make(map[models.ID]string),
			accountNames:  make(map[models.ID]string),
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

	accountID := models.NewID()

	app := &App{
		currentView: ViewRegister,
		width:       100,
		height:      30,
		styles:      styles,
		register: &registerData{
			account: &models.Account{
				BaseModel: models.BaseModel{ID: accountID},
				Name:      "Credit Card",
			},
			transactions:  []*models.Transaction{},
			balance:       &service.AccountBalance{AccountID: accountID, CurrentBalance: models.MustNewMoney("-1500.00")},
			payeeNames:    make(map[models.ID]string),
			categoryNames: make(map[models.ID]string),
			accountNames:  make(map[models.ID]string),
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

	accountID := models.NewID()
	otherAccountID := models.NewID()
	transferID := models.NewID()

	app := &App{
		currentView: ViewRegister,
		width:       120,
		height:      30,
		styles:      styles,
		register: &registerData{
			account: &models.Account{
				BaseModel: models.BaseModel{ID: accountID},
				Name:      "Checking",
			},
			transactions: []*models.Transaction{
				{
					BaseModel:         models.BaseModel{ID: models.NewID()},
					AccountID:         accountID,
					Date:              models.Today(),
					Amount:            models.MustNewMoney("-500.00"),
					Status:            models.TransactionStatusCleared,
					TransferID:        models.NullableID{ID: transferID, Valid: true},
					TransferAccountID: models.NullableID{ID: otherAccountID, Valid: true},
				},
			},
			balance:       &service.AccountBalance{AccountID: accountID, CurrentBalance: models.MustNewMoney("4500.00")},
			payeeNames:    make(map[models.ID]string),
			categoryNames: make(map[models.ID]string),
			accountNames:  map[models.ID]string{otherAccountID: "Savings"},
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
			transactions: []*models.Transaction{
				{BaseModel: models.BaseModel{ID: models.NewID()}, AccountID: accountID, Date: models.Today(), Amount: models.MustNewMoney("-10"), Status: models.TransactionStatusUncleared},
				{BaseModel: models.BaseModel{ID: models.NewID()}, AccountID: accountID, Date: models.Today(), Amount: models.MustNewMoney("-20"), Status: models.TransactionStatusUncleared},
				{BaseModel: models.BaseModel{ID: models.NewID()}, AccountID: accountID, Date: models.Today(), Amount: models.MustNewMoney("-30"), Status: models.TransactionStatusUncleared},
			},
			balance:       &service.AccountBalance{AccountID: accountID, CurrentBalance: models.MustNewMoney("100")},
			payeeNames:    make(map[models.ID]string),
			categoryNames: make(map[models.ID]string),
			accountNames:  make(map[models.ID]string),
		},
	}
	app.buildRegisterTable()

	// Table should start focused, sidebar not
	app.sidebar.SetFocused(false)
	app.table.SetFocused(true)

	// Move down
	downKey := tea.KeyMsg{Type: tea.KeyDown}
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
	upKey := tea.KeyMsg{Type: tea.KeyUp}
	app.Update(upKey)
	if app.table.Cursor() != 1 {
		t.Errorf("cursor should be 1 after up, got %d", app.table.Cursor())
	}
}

func TestApp_HandleRegisterKeys_TabFocus(t *testing.T) {
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

	// Start with table focused
	app.sidebar.SetFocused(false)
	app.table.SetFocused(true)

	// Tab should switch focus to sidebar
	tabKey := tea.KeyMsg{Type: tea.KeyTab}
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

	accountID := models.NewID()
	data := &registerData{
		account: &models.Account{
			BaseModel: models.BaseModel{ID: accountID},
			Name:      "Checking",
		},
		transactions: []*models.Transaction{
			{
				BaseModel: models.BaseModel{ID: models.NewID()},
				AccountID: accountID,
				Date:      models.Today(),
				Amount:    models.MustNewMoney("-50"),
				Status:    models.TransactionStatusUncleared,
			},
		},
		balance:       &service.AccountBalance{AccountID: accountID, CurrentBalance: models.MustNewMoney("950")},
		payeeNames:    make(map[models.ID]string),
		categoryNames: make(map[models.ID]string),
		accountNames:  make(map[models.ID]string),
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
	accountID := models.NewID()
	payeeID := models.NewID()
	categoryID := models.NewID()

	app := &App{
		styles: NewStyles(),
		register: &registerData{
			account: &models.Account{
				BaseModel: models.BaseModel{ID: accountID},
				Name:      "Checking",
			},
			transactions: []*models.Transaction{
				{
					BaseModel:  models.BaseModel{ID: models.NewID()},
					AccountID:  accountID,
					Date:       models.Today(),
					Amount:     models.MustNewMoney("-42.50"),
					Status:     models.TransactionStatusCleared,
					PayeeID:    models.NullableID{ID: payeeID, Valid: true},
					CategoryID: models.NullableID{ID: categoryID, Valid: true},
				},
			},
			balance:       &service.AccountBalance{AccountID: accountID, CurrentBalance: models.MustNewMoney("100")},
			payeeNames:    map[models.ID]string{payeeID: "Shell"},
			categoryNames: map[models.ID]string{categoryID: "Gas"},
			accountNames:  make(map[models.ID]string),
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
	accountID := models.NewID()

	tests := []struct {
		name     string
		status   models.TransactionStatus
		expected string
	}{
		{"uncleared", models.TransactionStatusUncleared, " "},
		{"cleared", models.TransactionStatusCleared, "✓"},
		{"reconciled", models.TransactionStatusReconciled, "R"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &App{
				styles: NewStyles(),
				register: &registerData{
					account: &models.Account{BaseModel: models.BaseModel{ID: accountID}, Name: "Test"},
					transactions: []*models.Transaction{
						{
							BaseModel: models.BaseModel{ID: models.NewID()},
							AccountID: accountID,
							Date:      models.Today(),
							Amount:    models.MustNewMoney("-10"),
							Status:    tt.status,
						},
					},
					balance:       &service.AccountBalance{AccountID: accountID, CurrentBalance: models.ZeroMoney},
					payeeNames:    make(map[models.ID]string),
					categoryNames: make(map[models.ID]string),
					accountNames:  make(map[models.ID]string),
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
			payeeNames:    make(map[models.ID]string),
			accountNames:  make(map[models.ID]string),
			categoryNames: make(map[models.ID]string),
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

	payeeID1 := models.NewID()
	payeeID2 := models.NewID()
	accountID := models.NewID()

	dueTxn := &models.ScheduledTransaction{
		BaseModel: models.BaseModel{ID: models.NewID()},
		AccountID: accountID,
		Frequency: models.FrequencyMonthly,
		NextDate:  models.Today(),
		PayeeID:   models.NullableID{ID: payeeID1, Valid: true},
		Amount:    models.NullableMoney{Money: models.MustNewMoney("-1500.00"), Valid: true},
	}

	upcomingTxn := &models.ScheduledTransaction{
		BaseModel: models.BaseModel{ID: models.NewID()},
		AccountID: accountID,
		Frequency: models.FrequencyWeekly,
		NextDate:  models.Today().AddDays(7),
		PayeeID:   models.NullableID{ID: payeeID2, Valid: true},
		Amount:    models.NullableMoney{Money: models.MustNewMoney("-50.00"), Valid: true},
	}

	app := &App{
		currentView: ViewScheduled,
		width:       120,
		height:      30,
		styles:      styles,
		scheduled: &scheduledViewData{
			dueTxns:      []*models.ScheduledTransaction{dueTxn},
			upcomingTxns: []*models.ScheduledTransaction{upcomingTxn},
			allTxns:      []*models.ScheduledTransaction{dueTxn, upcomingTxn},
			dueCount:     1,
			payeeNames:   map[models.ID]string{payeeID1: "Landlord", payeeID2: "Netflix"},
			accountNames:  map[models.ID]string{accountID: "Checking"},
			categoryNames: make(map[models.ID]string),
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
	payeeID := models.NewID()
	accountID := models.NewID()

	app := &App{
		styles: NewStyles(),
		scheduled: &scheduledViewData{
			allTxns: []*models.ScheduledTransaction{
				{
					BaseModel: models.BaseModel{ID: models.NewID()},
					AccountID: accountID,
					Frequency: models.FrequencyMonthly,
					NextDate:  models.Today(),
					PayeeID:   models.NullableID{ID: payeeID, Valid: true},
					Amount:    models.NullableMoney{Money: models.MustNewMoney("-100.00"), Valid: true},
				},
			},
			dueCount:      1,
			payeeNames:    map[models.ID]string{payeeID: "Electric Co"},
			accountNames:  map[models.ID]string{accountID: "Checking"},
			categoryNames: make(map[models.ID]string),
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
	accountID := models.NewID()

	app := &App{
		styles: NewStyles(),
		scheduled: &scheduledViewData{
			allTxns: []*models.ScheduledTransaction{
				{
					BaseModel: models.BaseModel{ID: models.NewID()},
					AccountID: accountID,
					Frequency: models.FrequencyMonthly,
					NextDate:  models.Today(),
					// No amount set - variable
				},
			},
			dueCount:      1,
			payeeNames:    make(map[models.ID]string),
			accountNames:  map[models.ID]string{accountID: "Checking"},
			categoryNames: make(map[models.ID]string),
		},
	}

	app.buildScheduledTable()

	row := app.scheduledTable.SelectedRow()
	if row[3] != "~variable" {
		t.Errorf("amount = %q, want %q for variable amount", row[3], "~variable")
	}
}

func TestApp_BuildScheduledTable_OverdueIndicator(t *testing.T) {
	accountID := models.NewID()
	pastDate := models.Today().AddDays(-3)

	app := &App{
		styles: NewStyles(),
		scheduled: &scheduledViewData{
			allTxns: []*models.ScheduledTransaction{
				{
					BaseModel: models.BaseModel{ID: models.NewID()},
					AccountID: accountID,
					Frequency: models.FrequencyMonthly,
					NextDate:  pastDate,
					Amount:    models.NullableMoney{Money: models.MustNewMoney("-50"), Valid: true},
				},
			},
			dueCount:      1,
			payeeNames:    make(map[models.ID]string),
			accountNames:  map[models.ID]string{accountID: "Checking"},
			categoryNames: make(map[models.ID]string),
		},
	}

	app.buildScheduledTable()

	row := app.scheduledTable.SelectedRow()
	if row[0] != "!●" {
		t.Errorf("status = %q, want %q for overdue", row[0], "!●")
	}
}

func TestApp_BuildScheduledTable_UpcomingIndicator(t *testing.T) {
	accountID := models.NewID()
	futureDate := models.Today().AddDays(7)

	app := &App{
		styles: NewStyles(),
		scheduled: &scheduledViewData{
			allTxns: []*models.ScheduledTransaction{
				{
					BaseModel: models.BaseModel{ID: models.NewID()},
					AccountID: accountID,
					Frequency: models.FrequencyWeekly,
					NextDate:  futureDate,
					Amount:    models.NullableMoney{Money: models.MustNewMoney("-25"), Valid: true},
				},
			},
			dueCount:      0, // not due, so index 0 >= dueCount (0)
			payeeNames:    make(map[models.ID]string),
			accountNames:  map[models.ID]string{accountID: "Checking"},
			categoryNames: make(map[models.ID]string),
		},
	}

	app.buildScheduledTable()

	row := app.scheduledTable.SelectedRow()
	if row[0] != " ○" {
		t.Errorf("status = %q, want %q for upcoming", row[0], " ○")
	}
}

func TestApp_HandleScheduledKeys_TableNavigation(t *testing.T) {
	accountID := models.NewID()

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
			allTxns: []*models.ScheduledTransaction{
				{BaseModel: models.BaseModel{ID: models.NewID()}, AccountID: accountID, Frequency: models.FrequencyMonthly, NextDate: models.Today()},
				{BaseModel: models.BaseModel{ID: models.NewID()}, AccountID: accountID, Frequency: models.FrequencyWeekly, NextDate: models.Today()},
				{BaseModel: models.BaseModel{ID: models.NewID()}, AccountID: accountID, Frequency: models.FrequencyYearly, NextDate: models.Today()},
			},
			dueCount:      3,
			payeeNames:    make(map[models.ID]string),
			accountNames:  map[models.ID]string{accountID: "Checking"},
			categoryNames: make(map[models.ID]string),
		},
	}
	app.buildScheduledTable()

	// Start with table focused, sidebar not
	app.sidebar.SetFocused(false)
	app.scheduledTable.SetFocused(true)

	// Move down
	downKey := tea.KeyMsg{Type: tea.KeyDown}
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
	upKey := tea.KeyMsg{Type: tea.KeyUp}
	app.Update(upKey)
	if app.scheduledTable.Cursor() != 1 {
		t.Errorf("cursor should be 1 after up, got %d", app.scheduledTable.Cursor())
	}
}

func TestApp_HandleScheduledKeys_TabFocus(t *testing.T) {
	accountID := models.NewID()

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
			allTxns:       []*models.ScheduledTransaction{},
			dueCount:      0,
			payeeNames:    make(map[models.ID]string),
			accountNames:  map[models.ID]string{accountID: "Checking"},
			categoryNames: make(map[models.ID]string),
		},
	}
	app.buildScheduledTable()

	// Start with table focused
	app.sidebar.SetFocused(false)
	app.scheduledTable.SetFocused(true)

	// Tab should switch focus to sidebar
	tabKey := tea.KeyMsg{Type: tea.KeyTab}
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

	accountID := models.NewID()
	payeeID := models.NewID()

	data := &scheduledViewData{
		allTxns: []*models.ScheduledTransaction{
			{
				BaseModel: models.BaseModel{ID: models.NewID()},
				AccountID: accountID,
				Frequency: models.FrequencyMonthly,
				NextDate:  models.Today(),
				PayeeID:   models.NullableID{ID: payeeID, Valid: true},
				Amount:    models.NullableMoney{Money: models.MustNewMoney("-100"), Valid: true},
			},
		},
		dueCount:      1,
		payeeNames:    map[models.ID]string{payeeID: "Landlord"},
		accountNames:  map[models.ID]string{accountID: "Checking"},
		categoryNames: make(map[models.ID]string),
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
	accountID := models.NewID()

	frequencies := []struct {
		freq     models.Frequency
		expected string
	}{
		{models.FrequencyDaily, "Daily"},
		{models.FrequencyWeekly, "Weekly"},
		{models.FrequencyBiweekly, "Biweekly"},
		{models.FrequencyMonthly, "Monthly"},
		{models.FrequencyQuarterly, "Quarterly"},
		{models.FrequencyYearly, "Yearly"},
	}

	for _, tt := range frequencies {
		t.Run(string(tt.freq), func(t *testing.T) {
			app := &App{
				styles: NewStyles(),
				scheduled: &scheduledViewData{
					payeeNames:    make(map[models.ID]string),
					accountNames:  map[models.ID]string{accountID: "Checking"},
					categoryNames: make(map[models.ID]string),
				},
			}

			st := &models.ScheduledTransaction{
				BaseModel: models.BaseModel{ID: models.NewID()},
				AccountID: accountID,
				Frequency: tt.freq,
				NextDate:  models.Today(),
				Amount:    models.NullableMoney{Money: models.MustNewMoney("-25"), Valid: true},
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
			netWorth: &models.NetWorthReport{
				AsOfDate: models.Today().Time(),
				Assets: []models.ReportAccountBalance{
					{Name: "Checking", Balance: models.MustNewMoney("5000.00")},
					{Name: "Savings", Balance: models.MustNewMoney("10000.00")},
				},
				Liabilities: []models.ReportAccountBalance{
					{Name: "Visa", Balance: models.MustNewMoney("1500.00")},
				},
				TotalAssets:      models.MustNewMoney("15000.00"),
				TotalLiabilities: models.MustNewMoney("1500.00"),
				NetWorth:         models.MustNewMoney("13500.00"),
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
			netWorth: &models.NetWorthReport{
				AsOfDate:         models.Today().Time(),
				Assets:           nil,
				Liabilities:      []models.ReportAccountBalance{{Name: "Loan", Balance: models.MustNewMoney("5000.00")}},
				TotalAssets:      models.MustNewMoney("0"),
				TotalLiabilities: models.MustNewMoney("5000.00"),
				NetWorth:         models.MustNewMoney("-5000.00"),
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
			spending: &models.SpendingReport{
				Period:    "January 2024",
				TotalSpending: models.MustNewMoney("3000.00"),
				Categories: []models.CategorySpending{
					{
						Name:       "Housing",
						Amount:     models.MustNewMoney("1500.00"),
						Percentage: 50.0,
						Subcategories: []models.CategorySpending{
							{Name: "Rent", Amount: models.MustNewMoney("1500.00")},
						},
					},
					{
						Name:       "Food",
						Amount:     models.MustNewMoney("1000.00"),
						Percentage: 33.3,
						Subcategories: []models.CategorySpending{
							{Name: "Groceries", Amount: models.MustNewMoney("700.00")},
							{Name: "Restaurants", Amount: models.MustNewMoney("300.00")},
						},
					},
					{
						Name:       "Transportation",
						Amount:     models.MustNewMoney("500.00"),
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
			spending: &models.SpendingReport{
				Period:        "June 2024",
				Categories:    nil,
				TotalSpending: models.ZeroMoney,
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
		name      string
		pct       float64
		maxWidth  int
		filled    int
		unfilled  int
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
	sKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}
	_, cmd := app.Update(sKey)
	if cmd == nil {
		t.Error("pressing 's' should return a command to load spending data")
	}

	// Now set to spending and press 'n' to switch to net worth
	app.reports.rtype = reportTypeSpending
	nKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}
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
	leftKey := tea.KeyMsg{Type: tea.KeyLeft}
	_, cmd := app.Update(leftKey)
	if cmd == nil {
		t.Error("pressing left should return a command for previous period")
	}

	// Press right to go to next period
	rightKey := tea.KeyMsg{Type: tea.KeyRight}
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
	leftKey := tea.KeyMsg{Type: tea.KeyLeft}
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
	yKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}
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
	mKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}}
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
	leftKey := tea.KeyMsg{Type: tea.KeyLeft}
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
		netWorth: &models.NetWorthReport{
			NetWorth: models.MustNewMoney("10000"),
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
			netWorth: &models.NetWorthReport{
				AsOfDate:         models.Today().Time(),
				TotalAssets:      models.MustNewMoney("1000"),
				TotalLiabilities: models.ZeroMoney,
				NetWorth:         models.MustNewMoney("1000"),
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
		spending: &models.SpendingReport{
			Period:        "January 2024",
			Categories:    nil,
			TotalSpending: models.ZeroMoney,
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
	if !contains(view, "Error") {
		t.Error("View() should contain 'Error' when err is set")
	}
	if !contains(view, "failed to open database") {
		t.Error("View() should contain the error message")
	}
	if !contains(view, "Press any key to continue") {
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
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
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

	msg := tea.KeyMsg{Type: tea.KeyEnter}
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

	msg := tea.KeyMsg{Type: tea.KeyEsc}
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

	msg := tea.KeyMsg{Type: tea.KeySpace}
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
	msg := tea.KeyMsg{Type: tea.KeyCtrlQ}
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
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	model, _ := app.Update(msg)
	updatedApp := model.(*App)

	if updatedApp.err != nil {
		t.Fatal("error should be cleared after first key press")
	}

	// Second key press should work normally (not get stuck)
	msg = tea.KeyMsg{Type: tea.KeyCtrlQ}
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

	accountID := models.NewID()

	app := &App{
		currentView: ViewRegister,
		width:       60,
		height:      30,
		styles:      styles,
		register: &registerData{
			account: &models.Account{
				BaseModel: models.BaseModel{ID: accountID},
				Name:      "My Super Duper Extremely Long Savings Account Name That Overflows",
			},
			transactions:  []*models.Transaction{},
			balance:       &service.AccountBalance{AccountID: accountID, CurrentBalance: models.MustNewMoney("100.00")},
			payeeNames:    make(map[models.ID]string),
			categoryNames: make(map[models.ID]string),
			accountNames:  make(map[models.ID]string),
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

	accountID := models.NewID()

	app := &App{
		currentView: ViewRegister,
		width:       100,
		height:      30,
		styles:      styles,
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
			payeeNames:   make(map[models.ID]string),
			accountNames: make(map[models.ID]string),
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
	accountID := models.NewID()

	app := &App{
		styles: NewStyles(),
		register: &registerData{
			account: &models.Account{BaseModel: models.BaseModel{ID: accountID}, Name: "Test"},
			transactions: []*models.Transaction{
				{
					BaseModel: models.BaseModel{ID: models.NewID()},
					AccountID: accountID,
					Date:      models.Today(),
					Amount:    models.ZeroMoney,
					Status:    models.TransactionStatusVoid,
				},
			},
			balance:       &service.AccountBalance{AccountID: accountID, CurrentBalance: models.ZeroMoney},
			payeeNames:    make(map[models.ID]string),
			categoryNames: make(map[models.ID]string),
			accountNames:  make(map[models.ID]string),
		},
	}

	app.buildRegisterTable()
	row := app.table.SelectedRow()
	if row[1] != "V" {
		t.Errorf("void status indicator = %q, want %q", row[1], "V")
	}
}

func TestApp_BuildRegisterTable_VoidRowStyling(t *testing.T) {
	accountID := models.NewID()

	app := &App{
		styles: NewStyles(),
		register: &registerData{
			account: &models.Account{BaseModel: models.BaseModel{ID: accountID}, Name: "Test"},
			transactions: []*models.Transaction{
				{
					BaseModel: models.BaseModel{ID: models.NewID()},
					AccountID: accountID,
					Date:      models.Today(),
					Amount:    models.MustNewMoney("-50"),
					Status:    models.TransactionStatusCleared,
				},
				{
					BaseModel: models.BaseModel{ID: models.NewID()},
					AccountID: accountID,
					Date:      models.Today(),
					Amount:    models.ZeroMoney,
					Status:    models.TransactionStatusVoid,
				},
				{
					BaseModel: models.BaseModel{ID: models.NewID()},
					AccountID: accountID,
					Date:      models.Today(),
					Amount:    models.MustNewMoney("-25"),
					Status:    models.TransactionStatusUncleared,
				},
			},
			balance:       &service.AccountBalance{AccountID: accountID, CurrentBalance: models.ZeroMoney},
			payeeNames:    make(map[models.ID]string),
			categoryNames: make(map[models.ID]string),
			accountNames:  make(map[models.ID]string),
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
	accountID := models.NewID()

	tests := []struct {
		name     string
		status   models.TransactionStatus
		expected string
	}{
		{"uncleared", models.TransactionStatusUncleared, " "},
		{"cleared", models.TransactionStatusCleared, "✓"},
		{"reconciled", models.TransactionStatusReconciled, "R"},
		{"void", models.TransactionStatusVoid, "V"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &App{
				styles: NewStyles(),
				register: &registerData{
					account: &models.Account{BaseModel: models.BaseModel{ID: accountID}, Name: "Test"},
					transactions: []*models.Transaction{
						{
							BaseModel: models.BaseModel{ID: models.NewID()},
							AccountID: accountID,
							Date:      models.Today(),
							Amount:    models.MustNewMoney("-10"),
							Status:    tt.status,
						},
					},
					balance:       &service.AccountBalance{AccountID: accountID, CurrentBalance: models.ZeroMoney},
					payeeNames:    make(map[models.ID]string),
					categoryNames: make(map[models.ID]string),
					accountNames:  make(map[models.ID]string),
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
	accountID := models.NewID()

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &service.TransactionService{},
		register: &registerData{
			account: &models.Account{BaseModel: models.BaseModel{ID: accountID}, Name: "Test"},
			transactions: []*models.Transaction{
				{
					BaseModel: models.BaseModel{ID: models.NewID()},
					AccountID: accountID,
					Date:      models.Today(),
					Amount:    models.ZeroMoney,
					Status:    models.TransactionStatusVoid,
				},
			},
			balance:       &service.AccountBalance{AccountID: accountID, CurrentBalance: models.ZeroMoney},
			payeeNames:    make(map[models.ID]string),
			categoryNames: make(map[models.ID]string),
			accountNames:  make(map[models.ID]string),
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
	accountID := models.NewID()

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &service.TransactionService{},
		register: &registerData{
			account: &models.Account{BaseModel: models.BaseModel{ID: accountID}, Name: "Test"},
			transactions: []*models.Transaction{
				{
					BaseModel: models.BaseModel{ID: models.NewID()},
					AccountID: accountID,
					Date:      models.Today(),
					Amount:    models.MustNewMoney("-10"),
					Status:    models.TransactionStatusReconciled,
				},
			},
			balance:       &service.AccountBalance{AccountID: accountID, CurrentBalance: models.ZeroMoney},
			payeeNames:    make(map[models.ID]string),
			categoryNames: make(map[models.ID]string),
			accountNames:  make(map[models.ID]string),
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
	accountID := models.NewID()

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &service.TransactionService{},
		register: &registerData{
			account: &models.Account{BaseModel: models.BaseModel{ID: accountID}, Name: "Test"},
			transactions: []*models.Transaction{
				{
					BaseModel: models.BaseModel{ID: models.NewID()},
					AccountID: accountID,
					Date:      models.Today(),
					Amount:    models.ZeroMoney,
					Status:    models.TransactionStatusVoid,
				},
			},
			balance:       &service.AccountBalance{AccountID: accountID, CurrentBalance: models.ZeroMoney},
			payeeNames:    make(map[models.ID]string),
			categoryNames: make(map[models.ID]string),
			accountNames:  make(map[models.ID]string),
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
	accountID := models.NewID()

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &service.TransactionService{},
		register: &registerData{
			account: &models.Account{BaseModel: models.BaseModel{ID: accountID}, Name: "Test"},
			transactions: []*models.Transaction{
				{
					BaseModel: models.BaseModel{ID: models.NewID()},
					AccountID: accountID,
					Date:      models.Today(),
					Amount:    models.MustNewMoney("-50"),
					Status:    models.TransactionStatusReconciled,
				},
			},
			balance:       &service.AccountBalance{AccountID: accountID, CurrentBalance: models.ZeroMoney},
			payeeNames:    make(map[models.ID]string),
			categoryNames: make(map[models.ID]string),
			accountNames:  make(map[models.ID]string),
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
	accountID := models.NewID()

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &service.TransactionService{},
		register: &registerData{
			account: &models.Account{BaseModel: models.BaseModel{ID: accountID}, Name: "Test"},
			transactions: []*models.Transaction{
				{
					BaseModel: models.BaseModel{ID: models.NewID()},
					AccountID: accountID,
					Date:      models.Today(),
					Amount:    models.MustNewMoney("-50"),
					Status:    models.TransactionStatusCleared,
				},
			},
			balance:       &service.AccountBalance{AccountID: accountID, CurrentBalance: models.ZeroMoney},
			payeeNames:    make(map[models.ID]string),
			categoryNames: make(map[models.ID]string),
			accountNames:  make(map[models.ID]string),
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
	accountID := models.NewID()
	transferAccountID := models.NewID()
	transferPairID := models.NewID()

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      NewStatusBar(),
		sidebar:        NewSidebar(),
		transactionSvc: &service.TransactionService{},
		register: &registerData{
			account: &models.Account{BaseModel: models.BaseModel{ID: accountID}, Name: "Test"},
			transactions: []*models.Transaction{
				{
					BaseModel:         models.BaseModel{ID: models.NewID()},
					AccountID:         accountID,
					TransferID:        models.NullableID{ID: transferPairID, Valid: true},
					TransferAccountID: models.NullableID{ID: transferAccountID, Valid: true},
					Date:              models.Today(),
					Amount:            models.MustNewMoney("-50"),
					Status:            models.TransactionStatusCleared,
				},
			},
			balance:       &service.AccountBalance{AccountID: accountID, CurrentBalance: models.ZeroMoney},
			payeeNames:    make(map[models.ID]string),
			categoryNames: make(map[models.ID]string),
			accountNames:  map[models.ID]string{transferAccountID: "Savings"},
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
	_, _ = app.handleConfirmDialogKey(tea.KeyMsg{Type: tea.KeyEsc})

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
	_, cmd := app.handleConfirmDialogKey(tea.KeyMsg{Type: tea.KeyEnter})

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
	accountID := models.NewID()

	sidebar := NewSidebar()
	sidebar.SetFocused(false)

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		statusbar:      NewStatusBar(),
		sidebar:        sidebar,
		transactionSvc: &service.TransactionService{},
		register: &registerData{
			account: &models.Account{BaseModel: models.BaseModel{ID: accountID}, Name: "Test"},
			transactions: []*models.Transaction{
				{
					BaseModel: models.BaseModel{ID: models.NewID()},
					AccountID: accountID,
					Date:      models.Today(),
					Amount:    models.MustNewMoney("-25"),
					Status:    models.TransactionStatusUncleared,
				},
			},
			balance:       &service.AccountBalance{AccountID: accountID, CurrentBalance: models.ZeroMoney},
			payeeNames:    make(map[models.ID]string),
			categoryNames: make(map[models.ID]string),
			accountNames:  make(map[models.ID]string),
		},
	}

	app.buildRegisterTable()

	// Press 'v' key
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}}
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
