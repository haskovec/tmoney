package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

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

func TestApp_GetKeyHints_IncludesAltKey(t *testing.T) {
	app := &App{
		currentView: ViewDashboard,
	}

	hints := app.getKeyHints()
	if !contains(hints, "Alt+key") {
		t.Errorf("getKeyHints() should contain 'Alt+key', got: %s", hints)
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
			minLineWidth := termWidth + 1
			for _, line := range viewLines {
				w := lipgloss.Width(line)
				if w > maxLineWidth {
					maxLineWidth = w
				}
				if w < minLineWidth {
					minLineWidth = w
				}
			}
			t.Logf("View: %d lines, line width range: [%d, %d]", len(viewLines), minLineWidth, maxLineWidth)

			if maxLineWidth > termWidth {
				t.Errorf("View lines (%d cols) wider than terminal (%d cols)", maxLineWidth, termWidth)
			}
			// Every line must fill the terminal width; a shorter line leaves a
			// stripe of bare terminal background on the right (regression
			// guard for the ContentWidth off-by-one).
			if minLineWidth != termWidth {
				t.Errorf("Shortest view line is %d cols, want %d (gap of %d on the right)",
					minLineWidth, termWidth, termWidth-minLineWidth)
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
