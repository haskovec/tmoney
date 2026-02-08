package tui

import (
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
			"Alt+A opens Accounts menu",
			tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}, Alt: true},
			1, "Accounts",
		},
		{
			"Alt+T opens Transactions menu",
			tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}, Alt: true},
			2, "Transactions",
		},
		{
			"Alt+R opens Reports menu",
			tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}, Alt: true},
			3, "Reports",
		},
		{
			"Alt+H opens Help menu",
			tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}, Alt: true},
			4, "Help",
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
	if updatedApp.menubar.Cursor() != 1 {
		t.Errorf("menu cursor = %d, want 1 (Accounts)", updatedApp.menubar.Cursor())
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
					Status:    models.TransactionStatusPending,
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
				{BaseModel: models.BaseModel{ID: models.NewID()}, AccountID: accountID, Date: models.Today(), Amount: models.MustNewMoney("-10"), Status: models.TransactionStatusPending},
				{BaseModel: models.BaseModel{ID: models.NewID()}, AccountID: accountID, Date: models.Today(), Amount: models.MustNewMoney("-20"), Status: models.TransactionStatusPending},
				{BaseModel: models.BaseModel{ID: models.NewID()}, AccountID: accountID, Date: models.Today(), Amount: models.MustNewMoney("-30"), Status: models.TransactionStatusPending},
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
				Status:    models.TransactionStatusPending,
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
		{"pending", models.TransactionStatusPending, " "},
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
