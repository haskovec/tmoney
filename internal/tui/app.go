// Package tui provides the terminal user interface for TMoney.
package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/haskovec/tmoney/internal/db"
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
	)
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

	case errMsg:
		a.err = msg.err
		return a, nil
	}

	return a, nil
}

// handleKeyPress handles keyboard input.
func (a *App) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global key bindings
	switch {
	case key.Matches(msg, a.keys.Quit):
		a.quitting = true
		return a, tea.Quit

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
	// Placeholder for dashboard-specific key handling
	return a, nil
}

// handleRegisterKeys handles key presses in the register view.
func (a *App) handleRegisterKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Placeholder for register-specific key handling
	return a, nil
}

// handleScheduledKeys handles key presses in the scheduled transactions view.
func (a *App) handleScheduledKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Placeholder for scheduled-specific key handling
	return a, nil
}

// handleReportsKeys handles key presses in the reports view.
func (a *App) handleReportsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Placeholder for reports-specific key handling
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

	if contentHeight < 1 {
		contentHeight = 1
	}

	// Render components
	header := a.renderHeader()
	content := a.renderContent(contentHeight)
	statusBar := a.renderStatusBar()

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		content,
		statusBar,
	)
}

// renderHeader renders the application header/menu bar.
func (a *App) renderHeader() string {
	title := "TMoney"
	viewIndicator := fmt.Sprintf(" | %s", a.currentView.String())

	return a.styles.Header.Render(title + viewIndicator)
}

// renderContent renders the main content area based on current view.
func (a *App) renderContent(height int) string {
	contentStyle := a.styles.Content.
		Height(height)

	var content string
	switch a.currentView {
	case ViewDashboard:
		content = a.renderDashboard()
	case ViewRegister:
		content = a.renderRegister()
	case ViewScheduled:
		content = a.renderScheduled()
	case ViewReports:
		content = a.renderReports()
	default:
		content = "Unknown view"
	}

	return contentStyle.Render(content)
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
	common := "1 dashboard  2 scheduled  3 reports  ? help  ctrl+q quit"

	switch a.currentView {
	case ViewDashboard:
		return "↑↓ navigate  enter select  n new  " + common
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

// Run starts the TUI application.
func Run(database *db.DB) error {
	app := NewApp(database)
	p := tea.NewProgram(app, tea.WithAltScreen())

	_, err := p.Run()
	return err
}
