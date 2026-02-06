// Package tui provides the terminal user interface for TMoney.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
	"github.com/haskovec/tmoney/internal/service"
)

// View represents the current view being displayed.
type View int

const (
	// ViewDashboard shows the main dashboard with net worth and scheduled transactions.
	ViewDashboard View = iota
	// ViewRegister shows the account transaction register.
	ViewRegister
	// ViewScheduled shows scheduled transactions.
	ViewScheduled
	// ViewReports shows reports.
	ViewReports
)

// String returns the display name of the view.
func (v View) String() string {
	switch v {
	case ViewDashboard:
		return "Dashboard"
	case ViewRegister:
		return "Register"
	case ViewScheduled:
		return "Scheduled"
	case ViewReports:
		return "Reports"
	default:
		return "Unknown"
	}
}

// App is the main TUI application model.
type App struct {
	// Database connection
	db *db.DB

	// Current view state
	currentView  View
	previousView View

	// Terminal dimensions
	width  int
	height int

	// Application state
	ready    bool
	quitting bool
	err      error

	// Styles
	styles Styles

	// Components
	sidebar *Sidebar
	menubar *MenuBar

	// Services (initialized on start)
	accountSvc      *service.AccountService
	transactionSvc  *service.TransactionService
	categorySvc     *service.CategoryService
	payeeSvc        *service.PayeeService
	scheduledTxnSvc *service.ScheduledTransactionService
	reportSvc       *service.ReportService

	// Key bindings
	keys keyMap
}

// keyMap defines the key bindings for the application.
type keyMap struct {
	Quit       key.Binding
	Help       key.Binding
	Up         key.Binding
	Down       key.Binding
	Left       key.Binding
	Right      key.Binding
	Enter      key.Binding
	Escape     key.Binding
	Tab        key.Binding
	ShiftTab   key.Binding
	New        key.Binding
	Edit       key.Binding
	Delete     key.Binding
	Search     key.Binding
	Dashboard  key.Binding
	Scheduled  key.Binding
	Reports    key.Binding
	Menu       key.Binding
}

// defaultKeyMap returns the default key bindings.
func defaultKeyMap() keyMap {
	return keyMap{
		Quit: key.NewBinding(
			key.WithKeys("ctrl+q", "ctrl+c"),
			key.WithHelp("ctrl+q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "left"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "right"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next"),
		),
		ShiftTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "previous"),
		),
		New: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new"),
		),
		Edit: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "edit"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		Dashboard: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "dashboard"),
		),
		Scheduled: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "scheduled"),
		),
		Reports: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "reports"),
		),
		Menu: key.NewBinding(
			key.WithKeys("f10"),
			key.WithHelp("F10", "menu"),
		),
	}
}

// NewApp creates a new TUI application with the given database.
func NewApp(database *db.DB) *App {
	// Create repositories
	accountRepo := repository.NewAccountRepository(database)
	transactionRepo := repository.NewTransactionRepository(database)
	splitRepo := repository.NewSplitRepository(database)
	transferRepo := repository.NewTransferRepository(database)
	categoryRepo := repository.NewCategoryRepository(database)
	payeeRepo := repository.NewPayeeRepository(database)
	scheduledRepo := repository.NewScheduledTransactionRepository(database)

	// Create services
	accountSvc := service.NewAccountService(accountRepo, database)
	transactionSvc := service.NewTransactionService(transactionRepo, splitRepo, transferRepo, payeeRepo, database)
	categorySvc := service.NewCategoryService(categoryRepo, database)
	payeeSvc := service.NewPayeeService(payeeRepo, database)
	scheduledTxnSvc := service.NewScheduledTransactionService(scheduledRepo, transactionRepo, database)
	reportSvc := service.NewReportService(accountRepo, database)

	return &App{
		db:              database,
		currentView:     ViewDashboard,
		styles:          NewStyles(),
		sidebar:         NewSidebar(),
		menubar:         NewMenuBar(),
		keys:            defaultKeyMap(),
		accountSvc:      accountSvc,
		transactionSvc:  transactionSvc,
		categorySvc:     categorySvc,
		payeeSvc:        payeeSvc,
		scheduledTxnSvc: scheduledTxnSvc,
		reportSvc:       reportSvc,
	}
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		tea.SetWindowTitle("TMoney - Personal Finance Manager"),
		a.loadSidebarData(),
	)
}

// loadSidebarData returns a command that loads accounts and balances for the sidebar.
func (a *App) loadSidebarData() tea.Cmd {
	return func() tea.Msg {
		if a.accountSvc == nil {
			return nil
		}
		accounts, err := a.accountSvc.List(true)
		if err != nil {
			return errMsg{err: err}
		}
		balances, err := a.accountSvc.GetAllBalances()
		if err != nil {
			return errMsg{err: err}
		}
		return sidebarLoadedMsg{accounts: accounts, balances: balances}
	}
}

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.styles.Resize(msg.Width, msg.Height)
		a.ready = true
		return a, nil

	case tea.KeyMsg:
		return a.handleKeyPress(msg)

	case sidebarLoadedMsg:
		a.sidebar.SetAccounts(msg.accounts, msg.balances)
		return a, nil

	case errMsg:
		a.err = msg.err
		return a, nil
	}

	return a, nil
}

// handleKeyPress handles keyboard input.
func (a *App) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// If menu bar is active, route all keys to menu handling
	if a.menubar.IsActive() {
		return a.handleMenuKeys(msg)
	}

	// Global key bindings
	switch {
	case key.Matches(msg, a.keys.Quit):
		a.quitting = true
		return a, tea.Quit

	case key.Matches(msg, a.keys.Menu):
		a.menubar.Activate()
		return a, nil

	case key.Matches(msg, a.keys.Dashboard):
		a.switchView(ViewDashboard)
		return a, nil

	case key.Matches(msg, a.keys.Scheduled):
		a.switchView(ViewScheduled)
		return a, nil

	case key.Matches(msg, a.keys.Reports):
		a.switchView(ViewReports)
		return a, nil

	case key.Matches(msg, a.keys.Escape):
		// Go back to previous view or dashboard
		if a.currentView != ViewDashboard {
			a.switchView(a.previousView)
		}
		return a, nil
	}

	// View-specific key handling
	switch a.currentView {
	case ViewDashboard:
		return a.handleDashboardKeys(msg)
	case ViewRegister:
		return a.handleRegisterKeys(msg)
	case ViewScheduled:
		return a.handleScheduledKeys(msg)
	case ViewReports:
		return a.handleReportsKeys(msg)
	}

	return a, nil
}

// handleDashboardKeys handles key presses in the dashboard view.
func (a *App) handleDashboardKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return a.handleSidebarKeys(msg)
}

// handleRegisterKeys handles key presses in the register view.
func (a *App) handleRegisterKeys(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Placeholder for register-specific key handling
	return a, nil
}

// handleScheduledKeys handles key presses in the scheduled transactions view.
func (a *App) handleScheduledKeys(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Placeholder for scheduled-specific key handling
	return a, nil
}

// handleReportsKeys handles key presses in the reports view.
func (a *App) handleReportsKeys(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Placeholder for reports-specific key handling
	return a, nil
}

// handleSidebarKeys handles keyboard navigation for the sidebar.
func (a *App) handleSidebarKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !a.sidebar.IsFocused() {
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Up):
		a.sidebar.MoveUp()
		return a, nil

	case key.Matches(msg, a.keys.Down):
		a.sidebar.MoveDown()
		return a, nil

	case key.Matches(msg, a.keys.Left):
		a.sidebar.CollapseGroup()
		return a, nil

	case key.Matches(msg, a.keys.Right):
		a.sidebar.ExpandGroup()
		return a, nil

	case key.Matches(msg, a.keys.Enter):
		if a.sidebar.Select() {
			a.switchView(ViewRegister)
		}
		return a, nil
	}

	return a, nil
}

// handleMenuKeys handles keyboard input when the menu bar is active.
func (a *App) handleMenuKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, a.keys.Escape), key.Matches(msg, a.keys.Menu):
		a.menubar.Deactivate()
		return a, nil

	case key.Matches(msg, a.keys.Left):
		a.menubar.MoveLeft()
		return a, nil

	case key.Matches(msg, a.keys.Right):
		a.menubar.MoveRight()
		return a, nil

	case key.Matches(msg, a.keys.Up):
		a.menubar.MoveUp()
		return a, nil

	case key.Matches(msg, a.keys.Down):
		a.menubar.MoveDown()
		return a, nil

	case key.Matches(msg, a.keys.Enter):
		action := a.menubar.Select()
		return a.handleMenuAction(action)

	case key.Matches(msg, a.keys.Quit):
		a.quitting = true
		return a, tea.Quit
	}

	return a, nil
}

// handleMenuAction processes a menu item selection.
func (a *App) handleMenuAction(action MenuAction) (tea.Model, tea.Cmd) {
	switch action {
	case MenuActionExit:
		a.quitting = true
		return a, tea.Quit

	case MenuActionDashboard:
		a.switchView(ViewDashboard)

	case MenuActionNetWorth, MenuActionSpendingByCategory:
		a.switchView(ViewReports)

	// Other actions are placeholders for future implementation
	case MenuActionNone:
		// No action
	}

	return a, nil
}

// switchView changes the current view and stores the previous view.
func (a *App) switchView(v View) {
	if a.currentView != v {
		a.previousView = a.currentView
		a.currentView = v
	}
}

// View implements tea.Model.
func (a *App) View() string {
	if a.quitting {
		return "Goodbye!\n"
	}

	if !a.ready {
		return "Loading..."
	}

	if a.err != nil {
		return a.renderError()
	}

	// Build the main layout
	return a.renderLayout()
}

// renderLayout renders the main application layout.
func (a *App) renderLayout() string {
	// Calculate content area dimensions
	headerHeight := 1
	statusBarHeight := 1
	contentHeight := a.height - headerHeight - statusBarHeight

	contentHeight = max(contentHeight, 1)

	// Render components
	header := a.renderHeader()
	content := a.renderContent(contentHeight)
	statusBar := a.renderStatusBar()

	layout := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		content,
		statusBar,
	)

	// Overlay dropdown if menu is active
	if a.menubar.IsActive() {
		dropdown, offset := a.menubar.RenderDropdown(a.styles)
		if dropdown != "" {
			layout = overlayDropdown(layout, dropdown, offset, 1, a.width)
		}
	}

	return layout
}

// renderHeader renders the application header/menu bar.
func (a *App) renderHeader() string {
	return a.menubar.Render(a.styles, a.width)
}

// renderContent renders the main content area based on current view.
func (a *App) renderContent(height int) string {
	var viewContent string
	switch a.currentView {
	case ViewDashboard:
		viewContent = a.renderDashboard()
	case ViewRegister:
		viewContent = a.renderRegister()
	case ViewScheduled:
		viewContent = a.renderScheduled()
	case ViewReports:
		viewContent = a.renderReports()
	default:
		viewContent = "Unknown view"
	}

	sidebarWidth := a.styles.SidebarWidth()
	if sidebarWidth == 0 {
		// Small layout: no sidebar, full-width content
		return a.styles.Content.
			Width(a.width).
			Height(height).
			Render(viewContent)
	}

	// Two-pane layout: sidebar + content
	sidebar := a.sidebar.Render(a.styles, sidebarWidth, height)
	contentWidth := a.styles.ContentWidth()
	content := a.styles.Content.
		Width(contentWidth).
		Height(height).
		Render(viewContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, content)
}

// renderDashboard renders the dashboard view.
func (a *App) renderDashboard() string {
	// Placeholder - will be implemented in task 041
	return lipgloss.NewStyle().
		Padding(1, 2).
		Render("Dashboard View\n\nPress ? for help, Ctrl+Q to quit")
}

// renderRegister renders the account register view.
func (a *App) renderRegister() string {
	// Placeholder - will be implemented in task 042
	return lipgloss.NewStyle().
		Padding(1, 2).
		Render("Account Register View\n\nPress Esc to go back")
}

// renderScheduled renders the scheduled transactions view.
func (a *App) renderScheduled() string {
	// Placeholder - will be implemented in task 047
	return lipgloss.NewStyle().
		Padding(1, 2).
		Render("Scheduled Transactions View\n\nPress Esc to go back")
}

// renderReports renders the reports view.
func (a *App) renderReports() string {
	// Placeholder - will be implemented in task 049
	return lipgloss.NewStyle().
		Padding(1, 2).
		Render("Reports View\n\nPress Esc to go back")
}

// renderStatusBar renders the status bar at the bottom.
func (a *App) renderStatusBar() string {
	hints := a.getKeyHints()

	return a.styles.StatusBar.Render(hints)
}

// getKeyHints returns key hints for the current view.
func (a *App) getKeyHints() string {
	common := "F10 menu  1 dashboard  2 scheduled  3 reports  ? help  ctrl+q quit"

	switch a.currentView {
	case ViewDashboard:
		return "↑↓ navigate  ←→ collapse/expand  enter select  " + common
	case ViewRegister:
		return "↑↓ navigate  enter edit  n new  d delete  esc back  " + common
	case ViewScheduled:
		return "↑↓ navigate  enter post  s skip  e edit  esc back  " + common
	case ViewReports:
		return "↑↓ navigate  ←→ period  esc back  " + common
	default:
		return common
	}
}

// renderError renders an error message.
func (a *App) renderError() string {
	return a.styles.Error.Render(fmt.Sprintf("Error: %v\n\nPress any key to continue", a.err))
}

// errMsg is a message type for errors.
type errMsg struct {
	err error
}

// sidebarLoadedMsg is sent when sidebar data has been loaded.
type sidebarLoadedMsg struct {
	accounts []*models.Account
	balances map[models.ID]*service.AccountBalance
}

// overlayDropdown places a dropdown string on top of the layout at the given row and column offset.
func overlayDropdown(layout, dropdown string, colOffset, rowOffset, totalWidth int) string {
	layoutLines := strings.Split(layout, "\n")
	dropdownLines := strings.Split(dropdown, "\n")

	for i, dLine := range dropdownLines {
		targetRow := rowOffset + i
		if targetRow >= len(layoutLines) {
			break
		}

		// Build the new line: prefix + dropdown + suffix
		bgLine := layoutLines[targetRow]
		bgRunes := []rune(stripAnsi(bgLine))

		// Build prefix (characters before the dropdown)
		prefix := ""
		if colOffset > 0 {
			if colOffset <= len(bgRunes) {
				prefix = string(bgRunes[:colOffset])
			} else {
				prefix = string(bgRunes) + strings.Repeat(" ", colOffset-len(bgRunes))
			}
		}

		// Build suffix (characters after the dropdown)
		dropdownWidth := lipgloss.Width(dLine)
		endCol := colOffset + dropdownWidth
		suffix := ""
		if endCol < len(bgRunes) {
			suffix = string(bgRunes[endCol:])
		}

		_ = totalWidth
		layoutLines[targetRow] = prefix + dLine + suffix
	}

	return strings.Join(layoutLines, "\n")
}

// stripAnsi removes ANSI escape codes from a string for width calculation.
func stripAnsi(s string) string {
	var result []rune
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
			}
			continue
		}
		result = append(result, r)
	}
	return string(result)
}

// Run starts the TUI application.
func Run(database *db.DB) error {
	app := NewApp(database)
	p := tea.NewProgram(app, tea.WithAltScreen())

	_, err := p.Run()
	return err
}
