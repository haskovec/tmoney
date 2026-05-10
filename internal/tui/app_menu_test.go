package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

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
	if updatedApp.menubar.Cursor() != 3 {
		t.Errorf("menu cursor = %d, want 3 (Accounts)", updatedApp.menubar.Cursor())
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
