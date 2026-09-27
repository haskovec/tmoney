package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
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

// sharedSectionTitles are the sections viewShortcutSections gives every view,
// the unknown fallback included. A section with any other title is the view's
// own.
var sharedSectionTitles = map[string]bool{
	"Global":     true,
	"Navigation": true,
	"Dialogs":    true,
	"Mouse":      true,
}

// TestViewShortcutSections ranges over every View constant in app.go, so a
// view added without a help arm fails here instead of shipping with a `?`
// overlay that lists none of its keys (the Corporate Actions bug).
func TestViewShortcutSections(t *testing.T) {
	for _, vc := range viewConstants(t) {
		t.Run(vc.Name, func(t *testing.T) {
			sections := viewShortcutSections(vc.Value)
			var own []string
			for _, s := range sections {
				if !sharedSectionTitles[s.Title] {
					own = append(own, s.Title)
				}
			}
			if len(own) == 0 {
				t.Fatalf("%s has no help section of its own; add an arm to viewShortcutSections", vc.Name)
			}
			if len(sections) != 5 {
				t.Fatalf("got %d sections, want Global, Navigation, %s, Dialogs, Mouse", len(sections), own[0])
			}
			if sections[0].Title != "Global" || sections[1].Title != "Navigation" {
				t.Errorf("first sections = %q, %q; want Global, Navigation", sections[0].Title, sections[1].Title)
			}
			if sections[3].Title != "Dialogs" || sections[4].Title != "Mouse" {
				t.Errorf("last sections = %q, %q; want Dialogs, Mouse", sections[3].Title, sections[4].Title)
			}
		})
	}

	t.Run("unknown view gets only the shared sections", func(t *testing.T) {
		for _, s := range viewShortcutSections(View(999)) {
			if !sharedSectionTitles[s.Title] {
				t.Errorf("View(999) got section %q", s.Title)
			}
		}
	})
}

// TestCorporateActionShortcuts pins the section to the view's status-bar hint
// and to the keys handleCorporateActionViewKeys binds.
func TestCorporateActionShortcuts(t *testing.T) {
	s := corporateActionShortcuts()
	if s.Title != "Corporate Actions" {
		t.Errorf("Title = %q, want Corporate Actions", s.Title)
	}
	keys := make(map[string]bool)
	for _, e := range s.Entries {
		keys[e.Key] = true
	}
	for _, want := range []string{"/", "Enter", "d", "Esc", "g / G", "PgUp/PgDn"} {
		if !keys[want] {
			t.Errorf("section has no %q entry", want)
		}
	}
}

func TestRenderHelpOverlay(t *testing.T) {
	styles := widget.NewStyles()
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

	t.Run("renders for corporate actions view", func(t *testing.T) {
		stripped := widget.StripAnsi(renderHelpOverlay(styles, ViewCorporateActions, 120, 50))
		for _, want := range []string{"Corporate Actions", "Filter actions", "Show details", "Reverse and delete action"} {
			if !strings.Contains(stripped, want) {
				t.Errorf("overlay should contain %q for the corporate actions view", want)
			}
		}
	})

	t.Run("contains close hint", func(t *testing.T) {
		result := renderHelpOverlay(styles, ViewDashboard, 120, 50)
		// The close hint text goes through lipgloss styling, so check for key parts
		stripped := widget.StripAnsi(result)
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
	for _, vc := range viewConstants(t) {
		for _, section := range viewShortcutSections(vc.Value) {
			for _, entry := range section.Entries {
				if entry.Key == "" {
					t.Errorf("%s: section %q has entry with empty Key", vc.Name, section.Title)
				}
				if entry.Description == "" {
					t.Errorf("%s: section %q has entry %q with empty Description", vc.Name, section.Title, entry.Key)
				}
			}
		}
	}
}

func TestApp_HelpOverlayToggle(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		menubar:     widget.NewMenuBar(),
		sidebar:     NewSidebar(),
		styles:      widget.NewStyles(),
		width:       120,
		height:      40,
		ready:       true,
	}

	// Initially help is not shown
	if app.showHelp {
		t.Error("showHelp should be false initially")
	}

	// Press ? to show help
	helpKey := tea.KeyPressMsg{Code: '?', Text: "?"}
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
	escKey := tea.KeyPressMsg{Code: tea.KeyEsc}
	app.Update(escKey)
	if app.showHelp {
		t.Error("showHelp should be false after pressing Esc")
	}
}

func TestApp_HelpOverlayBlocksOtherKeys(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		menubar:     widget.NewMenuBar(),
		sidebar:     NewSidebar(),
		styles:      widget.NewStyles(),
		width:       120,
		height:      40,
		ready:       true,
	}

	// Show help
	helpKey := tea.KeyPressMsg{Code: '?', Text: "?"}
	app.Update(helpKey)
	if !app.showHelp {
		t.Fatal("showHelp should be true")
	}

	// Press '1' - should not switch view while help is shown
	app.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	if !app.showHelp {
		t.Error("help should still be visible (non-dismiss keys should not close it)")
	}

	// Other keys should not change state while help is shown
	startView := app.currentView
	app.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	if app.currentView != startView {
		t.Error("view should not change while help is shown")
	}
}

func TestApp_HelpOverlayRendered(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		menubar:     widget.NewMenuBar(),
		sidebar:     NewSidebar(),
		styles:      widget.NewStyles(),
		width:       120,
		height:      40,
		ready:       true,
	}

	// Without help
	viewWithout := app.View()

	// With help
	app.showHelp = true
	viewWith := app.View()

	if viewWithout.Content == viewWith.Content {
		t.Error("view output should differ when help overlay is visible")
	}

	if !strings.Contains(viewWith.Content, "KEYBOARD SHORTCUTS") {
		t.Error("view with help should contain 'KEYBOARD SHORTCUTS'")
	}
}

func TestApp_MenuActionKeyboardShortcuts(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		menubar:     widget.NewMenuBar(),
		sidebar:     NewSidebar(),
		styles:      widget.NewStyles(),
		width:       120,
		height:      40,
		ready:       true,
	}

	// Simulate selecting Keyboard Shortcuts from Help menu
	app.menubar.Activate()
	_, _ = app.handleMenuAction(widget.MenuActionKeyboardShortcuts, "")

	if !app.showHelp {
		t.Error("showHelp should be true after widget.MenuActionKeyboardShortcuts")
	}
	if app.menubar.IsActive() {
		t.Error("menu bar should be deactivated after selecting keyboard shortcuts")
	}
}

func TestApp_HelpOverlayCloseHit(t *testing.T) {
	styles := widget.NewStyles()
	view := ViewDashboard
	screenW, screenH := 120, 40

	rendered := renderHelpOverlay(styles, view, screenW, screenH)
	lines := strings.Split(rendered, "\n")
	overlayH := len(lines)
	overlayW := 0
	for _, ln := range lines {
		if w := lipgloss.Width(ln); w > overlayW {
			overlayW = w
		}
	}
	startCol := (screenW - overlayW) / 2
	startRow := (screenH - overlayH) / 2
	contentWidth := overlayW - dialog.DialogHorizontalOverhead
	xCenter := startCol + 3 + contentWidth - 2 // middle of "[x]"
	titleY := startRow + 2

	if !helpOverlayCloseHit(styles, view, screenW, screenH, xCenter, titleY) {
		t.Errorf("click at (%d,%d) should hit [x]", xCenter, titleY)
	}
	// One column outside [x] should not hit.
	if helpOverlayCloseHit(styles, view, screenW, screenH, startCol+3+contentWidth-4, titleY) {
		t.Error("click just left of [x] should not hit")
	}
	// Different row should not hit.
	if helpOverlayCloseHit(styles, view, screenW, screenH, xCenter, titleY+1) {
		t.Error("click below the title row should not hit [x]")
	}
}

func TestApp_HelpOverlayClickClosesOnX(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		menubar:     widget.NewMenuBar(),
		sidebar:     NewSidebar(),
		styles:      widget.NewStyles(),
		width:       120,
		height:      40,
		ready:       true,
	}
	app.showHelp = true

	// Compute [x] coordinates the same way the handler does.
	rendered := renderHelpOverlay(app.styles, app.currentView, app.width, app.height)
	lines := strings.Split(rendered, "\n")
	overlayW := 0
	for _, ln := range lines {
		if w := lipgloss.Width(ln); w > overlayW {
			overlayW = w
		}
	}
	startCol := (app.width - overlayW) / 2
	startRow := (app.height - len(lines)) / 2
	contentWidth := overlayW - dialog.DialogHorizontalOverhead
	x := startCol + 3 + contentWidth - 2
	y := startRow + 2

	app.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if app.showHelp {
		t.Error("help overlay should close after clicking [x]")
	}
}

func TestApp_HelpOverlayClickElsewhereDoesNotClose(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		menubar:     widget.NewMenuBar(),
		sidebar:     NewSidebar(),
		styles:      widget.NewStyles(),
		width:       120,
		height:      40,
		ready:       true,
	}
	app.showHelp = true

	// Click near the centre of the overlay (not on [x]).
	app.Update(tea.MouseClickMsg{X: app.width / 2, Y: app.height / 2, Button: tea.MouseLeft})
	if !app.showHelp {
		t.Error("help overlay should remain open when clicking outside [x]")
	}
}

func TestApp_HelpKeyBinding(t *testing.T) {
	km := defaultKeyMap()

	// Verify the ? key binding exists
	if !key.Matches(tea.KeyPressMsg{Code: '?', Text: "?"}, km.Help) {
		t.Error("? should match the Help key binding")
	}
}
