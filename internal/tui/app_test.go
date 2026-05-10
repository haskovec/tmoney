package tui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
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

// =============================================================================
// Register View Tests
// =============================================================================

// =============================================================================
// Scheduled View Tests
// =============================================================================

// =============================================================================
// Reports View Tests
// =============================================================================

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

// =============================================================================
// Transaction Status TUI Tests (Task 062)
// =============================================================================

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

