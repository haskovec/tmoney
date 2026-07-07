package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/tui/widget"
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
	styles := widget.NewStyles()
	styles.Resize(80, 24)
	app := &App{
		currentView: ViewDashboard,
		width:       80,
		height:      24,
		ready:       true,
		styles:      styles,
		sidebar:     NewSidebar(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
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
	styles := widget.NewStyles()
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
		statusbar:   widget.NewStatusBar(),
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
				menubar:     widget.NewMenuBar(),
				sidebar:     NewSidebar(),
				statusbar:   widget.NewStatusBar(),
				width:       termWidth,
				height:      24,
				ready:       true,
			}
			app.styles = widget.NewStyles()
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
				menubar:     widget.NewMenuBar(),
				sidebar:     NewSidebar(),
				statusbar:   widget.NewStatusBar(),
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
			app.styles = widget.NewStyles()
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
				t.Logf("Line content: %q", widget.StripAnsi(viewLines[widestLine]))
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
		menubar:     widget.NewMenuBar(),
		sidebar:     NewSidebar(),
		statusbar:   widget.NewStatusBar(),
		width:       120,
		height:      24,
		ready:       true,
	}
	app.styles = widget.NewStyles()
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
		t.Errorf("First line should contain menu bar, got visual content: %q", widget.StripAnsi(regLines[0]))
	}

	// Log the first few lines for debugging
	for i := 0; i < min(5, len(regLines)); i++ {
		t.Logf("Line %d (width=%d): %q", i, lipgloss.Width(regLines[i]), widget.StripAnsi(regLines[i]))
	}
}

// TestApp_View_TallDashboardKeepsStatusBar pins the fix for the dashboard
// (landing) view overflowing the content area: with many investment
// accounts expanded the composed dashboard is far taller than the screen,
// and lipgloss Height() only pads short content — it does not clip tall
// content. Before the RenderViewContent MaxHeight clamp, the overflow
// pushed the status bar off the bottom of the altscreen. The full layout
// must always be exactly a.height lines, and the last line must be the
// status bar.
func TestApp_View_TallDashboardKeepsStatusBar(t *testing.T) {
	const termWidth, termHeight = 120, 24

	// Build a net-worth report with enough expanded investment accounts
	// (each contributing a name row, a TR row, and up to 5 holding rows)
	// to blow well past 24 lines of content.
	var assets []report.AccountBalance
	holdings := map[types.ID]*investment.AccountValuation{}
	tickers := map[types.ID]string{}
	expanded := map[types.ID]bool{}
	for i := range 8 {
		acctID := types.NewID()
		assets = append(assets, report.AccountBalance{
			AccountID: acctID,
			Name:      fmt.Sprintf("Investment %d", i),
			Type:      "investment",
			Balance:   types.MustNewMoney("25000.00"),
		})
		var hs []investment.Holding
		for j := range 6 {
			secID := types.NewID()
			hs = append(hs, investment.Holding{
				SecurityID:  secID,
				MarketValue: types.MustNewMoney(fmt.Sprintf("%d.00", (6-j)*1000)),
				HasPricing:  true,
			})
			tickers[secID] = fmt.Sprintf("A%dS%d", i, j)
		}
		holdings[acctID] = &investment.AccountValuation{AccountID: acctID, Holdings: hs}
		expanded[acctID] = true
	}

	styles := widget.NewStyles()
	styles.Resize(termWidth, termHeight)
	app := &App{
		currentView:               ViewDashboard,
		keys:                      defaultKeyMap(),
		menubar:                   widget.NewMenuBar(),
		sidebar:                   NewSidebar(),
		statusbar:                 widget.NewStatusBar(),
		width:                     termWidth,
		height:                    termHeight,
		ready:                     true,
		styles:                    styles,
		dashboardExpandedAccounts: expanded,
		dashboard: &dashboardData{
			netWorth: &report.NetWorth{
				Assets:           assets,
				TotalAssets:      types.MustNewMoney("200000.00"),
				TotalLiabilities: types.ZeroMoney,
				NetWorth:         types.MustNewMoney("200000.00"),
			},
			investmentHoldings: holdings,
			securityTickers:    tickers,
			payeeNames:         make(map[types.ID]string),
			accountNames:       make(map[types.ID]string),
		},
	}
	app.statusbar.SetContext("Dashboard")
	app.statusbar.SetKeyHints(app.getKeyHints())

	// Sanity: the raw dashboard content really is taller than the screen,
	// otherwise this test isn't exercising the clamp.
	if h := lipgloss.Height(app.renderDashboard()); h <= termHeight {
		t.Fatalf("test precondition failed: dashboard content is only %d lines, expected > %d", h, termHeight)
	}

	view := app.View()
	lines := strings.Split(view.Content, "\n")
	if len(lines) != termHeight {
		t.Errorf("View() has %d lines, want exactly %d (status bar pushed off screen?)", len(lines), termHeight)
	}

	// The last visible line must be the status bar, not clipped dashboard
	// content.
	lastLine := strings.TrimRight(widget.StripAnsi(lines[len(lines)-1]), " ")
	statusBar := strings.TrimRight(widget.StripAnsi(app.renderStatusBar()), " ")
	if lastLine != statusBar {
		t.Errorf("last layout line = %q, want status bar %q", lastLine, statusBar)
	}
	if !strings.Contains(lastLine, "Dashboard") {
		t.Errorf("status bar should still show the 'Dashboard' context, got %q", lastLine)
	}
}
