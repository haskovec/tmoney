package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

func TestShortcutSections(t *testing.T) {
	t.Run("global shortcuts are non-empty", func(t *testing.T) {
		s := globalShortcuts()
		if s.Title != "Global" {
			t.Errorf("Title = %q, want %q", s.Title, "Global")
		}
		if len(s.Entries) == 0 {
			t.Error("global shortcuts should have entries")
		}
	})

	t.Run("navigation shortcuts are non-empty", func(t *testing.T) {
		s := navigationShortcuts()
		if s.Title != "Navigation" {
			t.Errorf("Title = %q, want %q", s.Title, "Navigation")
		}
		if len(s.Entries) == 0 {
			t.Error("navigation shortcuts should have entries")
		}
	})

	t.Run("dashboard shortcuts are non-empty", func(t *testing.T) {
		s := dashboardShortcuts()
		if s.Title != "Dashboard" {
			t.Errorf("Title = %q, want %q", s.Title, "Dashboard")
		}
		if len(s.Entries) == 0 {
			t.Error("dashboard shortcuts should have entries")
		}
	})

	t.Run("register shortcuts are non-empty", func(t *testing.T) {
		s := registerShortcuts()
		if s.Title != "Register" {
			t.Errorf("Title = %q, want %q", s.Title, "Register")
		}
		if len(s.Entries) == 0 {
			t.Error("register shortcuts should have entries")
		}
	})

	t.Run("scheduled shortcuts are non-empty", func(t *testing.T) {
		s := scheduledShortcuts()
		if s.Title != "Scheduled Transactions" {
			t.Errorf("Title = %q, want %q", s.Title, "Scheduled Transactions")
		}
		if len(s.Entries) == 0 {
			t.Error("scheduled shortcuts should have entries")
		}
	})

	t.Run("reports shortcuts are non-empty", func(t *testing.T) {
		s := reportsShortcuts()
		if s.Title != "Reports" {
			t.Errorf("Title = %q, want %q", s.Title, "Reports")
		}
		if len(s.Entries) == 0 {
			t.Error("reports shortcuts should have entries")
		}
	})

	t.Run("dialog shortcuts are non-empty", func(t *testing.T) {
		s := dialogShortcuts()
		if s.Title != "Dialogs" {
			t.Errorf("Title = %q, want %q", s.Title, "Dialogs")
		}
		if len(s.Entries) == 0 {
			t.Error("dialog shortcuts should have entries")
		}
	})
}

func TestAllShortcutSections(t *testing.T) {
	sections := allShortcutSections()
	if len(sections) != 9 {
		t.Errorf("expected 9 sections, got %d", len(sections))
	}

	// Verify ordering
	expectedTitles := []string{
		"Global", "Navigation", "Dashboard", "Register",
		"Scheduled Transactions", "Reports", "Securities", "Reconciliation", "Dialogs",
	}
	for i, s := range sections {
		if s.Title != expectedTitles[i] {
			t.Errorf("section[%d].Title = %q, want %q", i, s.Title, expectedTitles[i])
		}
	}
}

func TestViewShortcutSections(t *testing.T) {
	tests := []struct {
		view         View
		wantViewName string
	}{
		{ViewDashboard, "Dashboard"},
		{ViewRegister, "Register"},
		{ViewScheduled, "Scheduled Transactions"},
		{ViewReports, "Reports"},
		{ViewReconciliation, "Reconciliation"},
		{ViewSecurities, "Securities"},
	}

	for _, tt := range tests {
		t.Run(tt.view.String(), func(t *testing.T) {
			sections := viewShortcutSections(tt.view)
			// Should always have Global, Navigation, view-specific, Dialog, and Mouse sections
			if len(sections) != 5 {
				t.Errorf("expected 5 sections for %v, got %d", tt.view, len(sections))
			}

			// First two should be Global and Navigation
			if sections[0].Title != "Global" {
				t.Errorf("first section = %q, want Global", sections[0].Title)
			}
			if sections[1].Title != "Navigation" {
				t.Errorf("second section = %q, want Navigation", sections[1].Title)
			}

			// Third should be view-specific
			if sections[2].Title != tt.wantViewName {
				t.Errorf("view section = %q, want %q", sections[2].Title, tt.wantViewName)
			}

			// Fourth should be Dialogs
			if sections[3].Title != "Dialogs" {
				t.Errorf("fourth section = %q, want Dialogs", sections[3].Title)
			}

			// Last should be Mouse
			if sections[4].Title != "Mouse" {
				t.Errorf("last section = %q, want Mouse", sections[4].Title)
			}
		})
	}
}

func TestRenderHelpOverlay(t *testing.T) {
	styles := NewStyles()
	styles.Resize(120, 50)

	t.Run("renders for dashboard view", func(t *testing.T) {
		result := renderHelpOverlay(styles, ViewDashboard, 120, 50)
		if result == "" {
			t.Error("renderHelpOverlay returned empty string")
		}
		// Check it contains the title
		if !strings.Contains(result, "KEYBOARD SHORTCUTS") {
			t.Error("overlay should contain 'KEYBOARD SHORTCUTS' title")
		}
		// Check it contains section headers
		if !strings.Contains(result, "Global") {
			t.Error("overlay should contain 'Global' section")
		}
		if !strings.Contains(result, "Dashboard") {
			t.Error("overlay should contain 'Dashboard' section for dashboard view")
		}
	})

	t.Run("renders for register view", func(t *testing.T) {
		result := renderHelpOverlay(styles, ViewRegister, 120, 50)
		if !strings.Contains(result, "Register") {
			t.Error("overlay should contain 'Register' section for register view")
		}
	})

	t.Run("renders for scheduled view", func(t *testing.T) {
		result := renderHelpOverlay(styles, ViewScheduled, 120, 50)
		if !strings.Contains(result, "Scheduled Transactions") {
			t.Error("overlay should contain 'Scheduled Transactions' section for scheduled view")
		}
	})

	t.Run("renders for reports view", func(t *testing.T) {
		result := renderHelpOverlay(styles, ViewReports, 120, 50)
		if !strings.Contains(result, "Reports") {
			t.Error("overlay should contain 'Reports' section for reports view")
		}
	})

	t.Run("contains close hint", func(t *testing.T) {
		result := renderHelpOverlay(styles, ViewDashboard, 120, 50)
		// The close hint text goes through lipgloss styling, so check for key parts
		stripped := stripAnsi(result)
		if !strings.Contains(stripped, "Esc to close") {
			t.Errorf("overlay should contain close hint, got stripped: %s", stripped)
		}
	})

	t.Run("adapts to narrow screen", func(t *testing.T) {
		result := renderHelpOverlay(styles, ViewDashboard, 40, 50)
		if result == "" {
			t.Error("renderHelpOverlay should work on narrow screens")
		}
	})

	t.Run("limits height to screen", func(t *testing.T) {
		result := renderHelpOverlay(styles, ViewDashboard, 120, 10)
		lines := strings.Split(result, "\n")
		if len(lines) > 10 {
			t.Errorf("overlay has %d lines but screen is 10 tall", len(lines))
		}
	})
}

func TestShortcutEntries_HaveKeyAndDescription(t *testing.T) {
	sections := allShortcutSections()
	for _, section := range sections {
		for _, entry := range section.Entries {
			if entry.Key == "" {
				t.Errorf("section %q has entry with empty Key", section.Title)
			}
			if entry.Description == "" {
				t.Errorf("section %q has entry %q with empty Description", section.Title, entry.Key)
			}
		}
	}
}

func TestApp_HelpOverlayToggle(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		styles:      NewStyles(),
		width:       120,
		height:      40,
		ready:       true,
	}

	// Initially help is not shown
	if app.showHelp {
		t.Error("showHelp should be false initially")
	}

	// Press ? to show help
	helpKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}
	app.Update(helpKey)
	if !app.showHelp {
		t.Error("showHelp should be true after pressing ?")
	}

	// Press ? again to close help
	app.Update(helpKey)
	if app.showHelp {
		t.Error("showHelp should be false after pressing ? again")
	}

	// Show help and close with Esc
	app.Update(helpKey)
	if !app.showHelp {
		t.Error("showHelp should be true after pressing ?")
	}
	escKey := tea.KeyMsg{Type: tea.KeyEsc}
	app.Update(escKey)
	if app.showHelp {
		t.Error("showHelp should be false after pressing Esc")
	}
}

func TestApp_HelpOverlayBlocksOtherKeys(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		styles:      NewStyles(),
		width:       120,
		height:      40,
		ready:       true,
	}

	// Show help
	helpKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}
	app.Update(helpKey)
	if !app.showHelp {
		t.Fatal("showHelp should be true")
	}

	// Press '1' - should not switch view while help is shown
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	if !app.showHelp {
		t.Error("help should still be visible (non-dismiss keys should not close it)")
	}

	// Other keys should not change state while help is shown
	startView := app.currentView
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if app.currentView != startView {
		t.Error("view should not change while help is shown")
	}
}

func TestApp_HelpOverlayRendered(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		styles:      NewStyles(),
		width:       120,
		height:      40,
		ready:       true,
	}

	// Without help
	viewWithout := app.View()

	// With help
	app.showHelp = true
	viewWith := app.View()

	if viewWithout == viewWith {
		t.Error("view output should differ when help overlay is visible")
	}

	if !strings.Contains(viewWith, "KEYBOARD SHORTCUTS") {
		t.Error("view with help should contain 'KEYBOARD SHORTCUTS'")
	}
}

func TestApp_MenuActionKeyboardShortcuts(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		styles:      NewStyles(),
		width:       120,
		height:      40,
		ready:       true,
	}

	// Simulate selecting Keyboard Shortcuts from Help menu
	app.menubar.Activate()
	_, _ = app.handleMenuAction(MenuActionKeyboardShortcuts)

	if !app.showHelp {
		t.Error("showHelp should be true after MenuActionKeyboardShortcuts")
	}
	if app.menubar.IsActive() {
		t.Error("menu bar should be deactivated after selecting keyboard shortcuts")
	}
}

func TestApp_HelpKeyBinding(t *testing.T) {
	km := defaultKeyMap()

	// Verify the ? key binding exists
	if !key.Matches(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}, km.Help) {
		t.Error("? should match the Help key binding")
	}
}
