// Package tui provides the terminal user interface for TMoney.
package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

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
	sidebar   *Sidebar
	menubar   *MenuBar
	statusbar *StatusBar

	// Services (initialized on start)
	accountSvc      *service.AccountService
	transactionSvc  *service.TransactionService
	categorySvc     *service.CategoryService
	payeeSvc        *service.PayeeService
	scheduledTxnSvc *service.ScheduledTransactionService
	reportSvc       *service.ReportService

	// Dashboard data (loaded asynchronously)
	dashboard *dashboardData

	// Register data (loaded when account is selected)
	register *registerData
	table    *Table

	// Transaction dialog state
	txnDialog            *Dialog
	txnDialogData        *transactionDialogData
	txnDialogCategoryIDs []models.ID

	// Split dialog state
	splitDialog     *SplitDialog
	pendingSplitTxn *pendingSplitTransaction

	// Transfer dialog state
	transferDialog           *Dialog
	transferDialogData       *transferDialogData
	transferDialogAccountIDs []models.ID

	// Key bindings
	keys keyMap
}

// keyMap defines the key bindings for the application.
type keyMap struct {
	Quit             key.Binding
	Help             key.Binding
	Up               key.Binding
	Down             key.Binding
	Left             key.Binding
	Right            key.Binding
	Enter            key.Binding
	Escape           key.Binding
	Tab              key.Binding
	ShiftTab         key.Binding
	New              key.Binding
	Edit             key.Binding
	Delete           key.Binding
	Search           key.Binding
	Dashboard        key.Binding
	Scheduled        key.Binding
	Reports          key.Binding
	Menu             key.Binding
	MenuFile         key.Binding
	MenuAccounts     key.Binding
	MenuTransactions key.Binding
	MenuReports      key.Binding
	MenuHelp         key.Binding
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
		MenuFile: key.NewBinding(
			key.WithKeys("alt+f"),
			key.WithHelp("Alt+F", "file menu"),
		),
		MenuAccounts: key.NewBinding(
			key.WithKeys("alt+a"),
			key.WithHelp("Alt+A", "accounts menu"),
		),
		MenuTransactions: key.NewBinding(
			key.WithKeys("alt+t"),
			key.WithHelp("Alt+T", "transactions menu"),
		),
		MenuReports: key.NewBinding(
			key.WithKeys("alt+r"),
			key.WithHelp("Alt+R", "reports menu"),
		),
		MenuHelp: key.NewBinding(
			key.WithKeys("alt+h"),
			key.WithHelp("Alt+H", "help menu"),
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
		statusbar:       NewStatusBar(),
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
	a.updateStatusBar()
	return tea.Batch(
		tea.EnterAltScreen,
		tea.SetWindowTitle("TMoney - Personal Finance Manager"),
		a.loadSidebarData(),
		a.loadScheduledDueCount(),
		a.loadDashboardData(),
	)
}

// updateStatusBar updates the status bar context and key hints for the current view.
func (a *App) updateStatusBar() {
	a.statusbar.SetContext(a.currentView.String())
	a.statusbar.SetKeyHints(a.getKeyHints())
}

// loadScheduledDueCount returns a command that loads the count of due scheduled transactions.
func (a *App) loadScheduledDueCount() tea.Cmd {
	return func() tea.Msg {
		if a.scheduledTxnSvc == nil {
			return nil
		}
		due, err := a.scheduledTxnSvc.ListDue()
		if err != nil {
			return errMsg{err: err}
		}
		return scheduledDueCountMsg{count: len(due)}
	}
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

// loadDashboardData returns a command that loads all data needed for the dashboard view.
func (a *App) loadDashboardData() tea.Cmd {
	return func() tea.Msg {
		data := &dashboardData{
			payeeNames:   make(map[models.ID]string),
			accountNames: make(map[models.ID]string),
		}

		// Load net worth report
		if a.reportSvc != nil {
			report, err := a.reportSvc.NetWorth()
			if err != nil {
				return errMsg{err: err}
			}
			data.netWorth = report
		}

		// Load due scheduled transactions
		if a.scheduledTxnSvc != nil {
			due, err := a.scheduledTxnSvc.ListDue()
			if err != nil {
				return errMsg{err: err}
			}
			data.dueTxns = due

			upcoming, err := a.scheduledTxnSvc.ListUpcoming(30)
			if err != nil {
				return errMsg{err: err}
			}
			// Filter out items already in due list
			var filteredUpcoming []*models.ScheduledTransaction
			dueIDs := make(map[string]bool)
			for _, d := range due {
				dueIDs[d.ID.String()] = true
			}
			for _, u := range upcoming {
				if !dueIDs[u.ID.String()] {
					filteredUpcoming = append(filteredUpcoming, u)
				}
			}
			data.upcomingTxns = filteredUpcoming
		}

		// Load payee names for scheduled transactions
		if a.payeeSvc != nil {
			payees, err := a.payeeSvc.List()
			if err == nil {
				for _, p := range payees {
					data.payeeNames[p.ID] = p.Name
				}
			}
		}

		// Load account names
		if a.accountSvc != nil {
			accounts, err := a.accountSvc.List(true)
			if err == nil {
				for _, acc := range accounts {
					data.accountNames[acc.ID] = acc.Name
				}
			}
		}

		return dashboardLoadedMsg{data: data}
	}
}

// loadRegisterData returns a command that loads all data needed for the register view.
func (a *App) loadRegisterData(accountID models.ID) tea.Cmd {
	return func() tea.Msg {
		data := &registerData{
			payeeNames:    make(map[models.ID]string),
			categoryNames: make(map[models.ID]string),
			accountNames:  make(map[models.ID]string),
		}

		// Load account
		if a.accountSvc != nil {
			acct, err := a.accountSvc.GetByID(accountID)
			if err != nil {
				return errMsg{err: err}
			}
			data.account = acct

			// Load balance
			bal, err := a.accountSvc.GetBalance(accountID)
			if err != nil {
				return errMsg{err: err}
			}
			data.balance = bal

			// Load account names for transfer display
			accounts, err := a.accountSvc.List(true)
			if err == nil {
				for _, acc := range accounts {
					data.accountNames[acc.ID] = acc.Name
				}
			}
		}

		// Load transactions
		if a.transactionSvc != nil {
			txns, err := a.transactionSvc.ListByAccount(accountID)
			if err != nil {
				return errMsg{err: err}
			}
			data.transactions = txns
		}

		// Load payee names
		if a.payeeSvc != nil {
			payees, err := a.payeeSvc.List()
			if err == nil {
				for _, p := range payees {
					data.payeeNames[p.ID] = p.Name
				}
			}
		}

		// Load category names
		if a.categorySvc != nil {
			categories, err := a.categorySvc.List()
			if err == nil {
				for _, c := range categories {
					data.categoryNames[c.ID] = c.Name
				}
			}
		}

		return registerLoadedMsg{data: data}
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

	case scheduledDueCountMsg:
		a.statusbar.ClearNotifications()
		if msg.count > 0 {
			text := fmt.Sprintf("%d scheduled due", msg.count)
			if msg.count == 1 {
				text = "1 scheduled due"
			}
			a.statusbar.AddNotification(text, NotificationAlert)
		}
		return a, nil

	case dashboardLoadedMsg:
		a.dashboard = msg.data
		return a, nil

	case registerLoadedMsg:
		a.register = msg.data
		a.buildRegisterTable()
		return a, nil

	case transactionDialogDataMsg:
		a.txnDialogData = msg.data
		categoryOptions, categoryIDs := buildCategoryOptions(msg.data.categories)
		a.txnDialogCategoryIDs = categoryIDs
		a.txnDialog = buildTransactionDialog(msg.data, categoryOptions)
		return a, nil

	case transactionDialogSavedMsg:
		accountID := a.sidebar.SelectedAccountID()
		return a, tea.Batch(
			a.loadRegisterData(accountID),
			a.loadSidebarData(),
		)

	case splitDialogSavedMsg:
		accountID := a.sidebar.SelectedAccountID()
		return a, tea.Batch(
			a.loadRegisterData(accountID),
			a.loadSidebarData(),
		)

	case transferDialogDataMsg:
		a.transferDialogData = msg.data
		accountOptions, accountIDs := buildAccountOptions(msg.data.accounts)
		a.transferDialogAccountIDs = accountIDs
		// Pre-select the currently selected sidebar account as "From"
		defaultFromIndex := 0
		selectedID := a.sidebar.SelectedAccountID()
		for i, id := range accountIDs {
			if id == selectedID {
				defaultFromIndex = i
				break
			}
		}
		a.transferDialog = buildTransferDialog(accountOptions, defaultFromIndex)
		return a, nil

	case transferDialogSavedMsg:
		accountID := a.sidebar.SelectedAccountID()
		return a, tea.Batch(
			a.loadRegisterData(accountID),
			a.loadSidebarData(),
		)

	case errMsg:
		a.err = msg.err
		return a, nil
	}

	return a, nil
}

// handleKeyPress handles keyboard input.
func (a *App) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// If split dialog is visible, route all keys to it
	if a.splitDialog != nil && a.splitDialog.IsVisible() {
		return a.handleSplitDialogKey(msg)
	}

	// If transaction dialog is visible, route all keys to it
	if a.txnDialog != nil && a.txnDialog.IsVisible() {
		return a.handleTransactionDialogKey(msg)
	}

	// If transfer dialog is visible, route all keys to it
	if a.transferDialog != nil && a.transferDialog.IsVisible() {
		return a.handleTransferDialogKey(msg)
	}

	// Alt+key menu shortcuts work regardless of menu state
	switch {
	case key.Matches(msg, a.keys.MenuFile):
		a.toggleMenu(0)
		return a, nil
	case key.Matches(msg, a.keys.MenuAccounts):
		a.toggleMenu(1)
		return a, nil
	case key.Matches(msg, a.keys.MenuTransactions):
		a.toggleMenu(2)
		return a, nil
	case key.Matches(msg, a.keys.MenuReports):
		a.toggleMenu(3)
		return a, nil
	case key.Matches(msg, a.keys.MenuHelp):
		a.toggleMenu(4)
		return a, nil
	}

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
func (a *App) handleRegisterKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle Tab to switch focus between sidebar and table
	if key.Matches(msg, a.keys.Tab) || key.Matches(msg, a.keys.ShiftTab) {
		if a.sidebar.IsFocused() {
			a.sidebar.SetFocused(false)
			if a.table != nil {
				a.table.SetFocused(true)
			}
		} else {
			a.sidebar.SetFocused(true)
			if a.table != nil {
				a.table.SetFocused(false)
			}
		}
		return a, nil
	}

	// If sidebar has focus, delegate to sidebar handling
	if a.sidebar.IsFocused() {
		return a.handleSidebarKeys(msg)
	}

	// Table-focused key handling
	if a.table == nil || a.register == nil {
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Up):
		a.table.MoveUp()
	case key.Matches(msg, a.keys.Down):
		a.table.MoveDown()
	case msg.String() == "home" || msg.String() == "g":
		a.table.MoveToTop()
	case msg.String() == "end" || msg.String() == "G":
		a.table.MoveToBottom()
	case msg.String() == "pgup":
		tableHeight := a.height - 6
		if tableHeight < 1 {
			tableHeight = 1
		}
		a.table.PageUp(tableHeight)
	case msg.String() == "pgdown":
		tableHeight := a.height - 6
		if tableHeight < 1 {
			tableHeight = 1
		}
		a.table.PageDown(tableHeight)
	case msg.String() == "c":
		return a.toggleTransactionStatus()
	case key.Matches(msg, a.keys.New):
		return a, a.loadTransactionDialogData()
	case msg.String() == "t":
		return a, a.loadTransferDialogData()
	}

	return a, nil
}

// toggleTransactionStatus toggles the cleared/pending status of the selected transaction.
func (a *App) toggleTransactionStatus() (tea.Model, tea.Cmd) {
	if a.table == nil || a.register == nil || a.transactionSvc == nil {
		return a, nil
	}

	cursor := a.table.Cursor()
	if cursor < 0 || cursor >= len(a.register.transactions) {
		return a, nil
	}

	txn := a.register.transactions[cursor]
	accountID := a.sidebar.SelectedAccountID()

	return a, func() tea.Msg {
		var err error
		if txn.Status == models.TransactionStatusCleared {
			err = a.transactionSvc.MarkTransactionPending(txn.ID)
		} else {
			err = a.transactionSvc.ClearTransaction(txn.ID)
		}
		if err != nil {
			return errMsg{err: err}
		}
		// Reload register data to reflect the change
		return a.loadRegisterData(accountID)()
	}
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
			a.register = nil // Clear old data while loading
			a.switchView(ViewRegister)
			return a, a.loadRegisterData(a.sidebar.SelectedAccountID())
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

// toggleMenu opens the menu at the given index, or closes it if already open at that index.
func (a *App) toggleMenu(index int) {
	if a.menubar.IsActive() && a.menubar.Cursor() == index {
		a.menubar.Deactivate()
	} else {
		a.menubar.ActivateMenu(index)
	}
}

// switchView changes the current view and stores the previous view.
func (a *App) switchView(v View) {
	if a.currentView != v {
		a.previousView = a.currentView
		a.currentView = v
		a.updateStatusBar()

		// Set focus appropriately for the new view
		if a.sidebar != nil {
			switch v {
			case ViewRegister:
				// Start with table focused when entering register
				a.sidebar.SetFocused(false)
				if a.table != nil {
					a.table.SetFocused(true)
				}
			case ViewDashboard:
				// Dashboard uses sidebar navigation
				a.sidebar.SetFocused(true)
				if a.table != nil {
					a.table.SetFocused(false)
				}
			}
		}
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

	// Overlay transaction dialog if visible
	if a.txnDialog != nil && a.txnDialog.IsVisible() {
		overlay := a.txnDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay split dialog if visible
	if a.splitDialog != nil && a.splitDialog.IsVisible() {
		overlay := a.splitDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay transfer dialog if visible
	if a.transferDialog != nil && a.transferDialog.IsVisible() {
		overlay := a.transferDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
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
	if a.dashboard == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading dashboard...")
	}

	var sections []string

	// Title row: DASHBOARD + date
	contentWidth := a.styles.ContentWidth()
	dateStr := time.Now().Format("Jan 2, 2006")
	titleText := "DASHBOARD"
	padding := contentWidth - lipgloss.Width(titleText) - lipgloss.Width(dateStr) - 4
	if padding < 1 {
		padding = 1
	}
	titleRow := a.styles.Title.Render(titleText) + strings.Repeat(" ", padding) + a.styles.Muted.Render(dateStr)
	sections = append(sections, titleRow)

	// Separator
	sepWidth := contentWidth - 4
	if sepWidth < 1 {
		sepWidth = 1
	}
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	// Net worth display
	if a.dashboard.netWorth != nil {
		nw := a.dashboard.netWorth
		nwLabel := "Net Worth:  "
		nwValue := formatDashboardMoney(nw.NetWorth)
		nwStyle := a.styles.Positive
		if nw.NetWorth.IsNegative() {
			nwStyle = a.styles.Negative
		}
		sections = append(sections, "")
		sections = append(sections, a.styles.Bold.Render(nwLabel)+nwStyle.Bold(true).Render(nwValue))
		sections = append(sections, "")

		// Assets and Liabilities columns
		sections = append(sections, a.renderAssetLiabilityColumns(nw, contentWidth))
	}

	// Scheduled transactions section
	sections = append(sections, a.renderDashboardScheduled())

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// renderAssetLiabilityColumns renders the assets and liabilities side by side.
func (a *App) renderAssetLiabilityColumns(report *models.NetWorthReport, totalWidth int) string {
	colWidth := (totalWidth - 6) / 2 // Leave gap between columns
	if colWidth < 20 {
		colWidth = 20
	}

	// Build assets column
	assetsLines := []string{a.styles.SectionHead.Render(padRight("ASSETS", colWidth))}
	if len(report.Assets) == 0 {
		assetsLines = append(assetsLines, a.styles.Muted.Render("  (none)"))
	} else {
		for _, acct := range report.Assets {
			name := truncate(acct.Name, colWidth-14)
			amount := formatDashboardMoney(acct.Balance)
			line := fmt.Sprintf("  %-*s %s", colWidth-len(amount)-4, name, a.styles.Positive.Render(amount))
			assetsLines = append(assetsLines, line)
		}
	}
	assetsLines = append(assetsLines, a.styles.Muted.Render("  "+strings.Repeat("─", colWidth-4)))
	totalLabel := "Total"
	totalAmt := formatDashboardMoney(report.TotalAssets)
	assetsLines = append(assetsLines, fmt.Sprintf("  %-*s %s", colWidth-len(totalAmt)-4, totalLabel, a.styles.Positive.Bold(true).Render(totalAmt)))

	// Build liabilities column
	liabLines := []string{a.styles.SectionHead.Render(padRight("LIABILITIES", colWidth))}
	if len(report.Liabilities) == 0 {
		liabLines = append(liabLines, a.styles.Muted.Render("  (none)"))
	} else {
		for _, acct := range report.Liabilities {
			name := truncate(acct.Name, colWidth-14)
			amount := formatDashboardMoney(acct.Balance)
			line := fmt.Sprintf("  %-*s %s", colWidth-len(amount)-4, name, a.styles.Negative.Render(amount))
			liabLines = append(liabLines, line)
		}
	}
	liabLines = append(liabLines, a.styles.Muted.Render("  "+strings.Repeat("─", colWidth-4)))
	totalLiabAmt := formatDashboardMoney(report.TotalLiabilities)
	liabLines = append(liabLines, fmt.Sprintf("  %-*s %s", colWidth-len(totalLiabAmt)-4, totalLabel, a.styles.Negative.Bold(true).Render(totalLiabAmt)))

	// Ensure both columns have the same height
	for len(assetsLines) < len(liabLines) {
		assetsLines = append(assetsLines, "")
	}
	for len(liabLines) < len(assetsLines) {
		liabLines = append(liabLines, "")
	}

	// Join columns side by side
	var rows []string
	for i := range assetsLines {
		left := padRight(assetsLines[i], colWidth)
		right := liabLines[i]
		rows = append(rows, left+"  "+right)
	}

	return strings.Join(rows, "\n")
}

// renderDashboardScheduled renders the scheduled transactions section of the dashboard.
func (a *App) renderDashboardScheduled() string {
	if a.dashboard == nil {
		return ""
	}

	due := a.dashboard.dueTxns
	upcoming := a.dashboard.upcomingTxns
	total := len(due) + len(upcoming)

	var lines []string
	lines = append(lines, "")

	// Section header with count
	header := "SCHEDULED"
	if total > 0 {
		dueCount := len(due)
		if dueCount > 0 {
			header += fmt.Sprintf(" (%d due)", dueCount)
		}
	}
	lines = append(lines, a.styles.SectionHead.Render(header))

	if total == 0 {
		lines = append(lines, a.styles.Muted.Render("  No scheduled transactions"))
		return strings.Join(lines, "\n")
	}

	// Due items
	for _, st := range due {
		lines = append(lines, a.formatScheduledItem(st, true))
	}

	// Upcoming items (limit to 5)
	limit := 5
	if len(upcoming) < limit {
		limit = len(upcoming)
	}
	for i := 0; i < limit; i++ {
		lines = append(lines, a.formatScheduledItem(upcoming[i], false))
	}
	if len(upcoming) > 5 {
		lines = append(lines, a.styles.Muted.Render(fmt.Sprintf("  ... and %d more", len(upcoming)-5)))
	}

	return strings.Join(lines, "\n")
}

// formatScheduledItem formats a single scheduled transaction line for the dashboard.
func (a *App) formatScheduledItem(st *models.ScheduledTransaction, isDue bool) string {
	// Payee name
	payee := "Unknown"
	if st.HasPayee() {
		if name, ok := a.dashboard.payeeNames[st.PayeeID.ID]; ok {
			payee = name
		}
	}

	// Amount
	var amount string
	if st.HasAmount() {
		amount = formatDashboardMoney(st.Amount.Money)
	} else {
		amount = "~variable"
	}

	// Due indicator
	if isDue {
		today := models.Today()
		if st.NextDate.Equal(today) {
			return fmt.Sprintf("  %s %s - %s %s",
				a.styles.Alert.Render("●"),
				payee,
				amount,
				a.styles.Alert.Render("due today"))
		}
		daysAgo := int(math.Round(time.Since(st.NextDate.Time()).Hours() / 24))
		return fmt.Sprintf("  %s %s - %s %s",
			a.styles.Alert.Render("●"),
			payee,
			amount,
			a.styles.Alert.Render(fmt.Sprintf("overdue %d days", daysAgo)))
	}

	// Upcoming - show days until
	daysUntil := int(math.Round(time.Until(st.NextDate.Time()).Hours() / 24))
	daysText := fmt.Sprintf("in %d days", daysUntil)
	if daysUntil == 1 {
		daysText = "tomorrow"
	}
	return fmt.Sprintf("  %s %s - %s %s",
		a.styles.Muted.Render("○"),
		payee,
		amount,
		a.styles.Muted.Render(daysText))
}

// formatDashboardMoney formats a Money value with $ prefix for dashboard display.
func formatDashboardMoney(m models.Money) string {
	value := fmt.Sprintf("%.2f", m.Float64())
	if m.IsNegative() {
		return fmt.Sprintf("-$%s", strings.TrimPrefix(value, "-"))
	}
	return fmt.Sprintf("$%s", value)
}

// padRight pads a string with spaces to the given width.
func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// truncate truncates a string to maxLen characters, adding "..." if needed.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// buildRegisterTable creates and populates the table for the register view.
func (a *App) buildRegisterTable() {
	if a.register == nil {
		return
	}

	columns := []Column{
		{Header: "Date", Width: 10, Align: AlignLeft},
		{Header: "S", Width: 1, Align: AlignCenter},
		{Header: "Payee", MinWidth: 12, Align: AlignLeft},
		{Header: "Category", MinWidth: 10, Align: AlignLeft},
		{Header: "Amount", Width: 12, Align: AlignRight},
	}

	if a.table == nil {
		a.table = NewTable(columns)
	} else {
		a.table.SetColumns(columns)
	}

	rows := make([][]string, len(a.register.transactions))
	for i, txn := range a.register.transactions {
		rows[i] = a.formatRegisterRow(txn)
	}
	a.table.SetRows(rows)
}

// formatRegisterRow formats a transaction into table row strings.
func (a *App) formatRegisterRow(txn *models.Transaction) []string {
	// Date
	dateStr := txn.Date.Time().Format("01/02/06")

	// Status indicator
	status := " "
	switch txn.Status {
	case models.TransactionStatusCleared:
		status = "✓"
	case models.TransactionStatusReconciled:
		status = "R"
	}

	// Payee
	payee := ""
	if txn.IsTransfer() {
		if name, ok := a.register.accountNames[txn.TransferAccountID.ID]; ok {
			payee = "Transfer: " + name
		} else {
			payee = "Transfer"
		}
	} else if txn.HasPayee() {
		if name, ok := a.register.payeeNames[txn.PayeeID.ID]; ok {
			payee = name
		}
	}

	// Category
	category := ""
	if txn.HasCategory() {
		if name, ok := a.register.categoryNames[txn.CategoryID.ID]; ok {
			category = name
		}
	} else if txn.IsTransfer() {
		category = "[Transfer]"
	}

	// Amount
	amount := formatDashboardMoney(txn.Amount)

	return []string{dateStr, status, payee, category, amount}
}

// renderRegister renders the account register view.
func (a *App) renderRegister() string {
	if a.register == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading register...")
	}

	contentWidth := a.styles.ContentWidth()

	var sections []string

	// Title row: account name + balance
	acctName := strings.ToUpper(a.register.account.Name)
	balStr := ""
	if a.register.balance != nil {
		balStr = "Bal: " + formatDashboardMoney(a.register.balance.CurrentBalance)
	}
	padding := contentWidth - lipgloss.Width(acctName) - lipgloss.Width(balStr) - 4
	if padding < 1 {
		padding = 1
	}

	balStyle := a.styles.Positive
	if a.register.balance != nil && a.register.balance.CurrentBalance.IsNegative() {
		balStyle = a.styles.Negative
	}
	titleRow := a.styles.Title.Render(acctName) + strings.Repeat(" ", padding) + balStyle.Render(balStr)
	sections = append(sections, titleRow)

	// Separator
	sepWidth := contentWidth - 4
	if sepWidth < 1 {
		sepWidth = 1
	}
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	// Table
	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 2   // title + separator
	paddingHeight := 2 // top/bottom padding
	tableHeight := a.height - headerHeight - statusBarHeight - titleHeight - paddingHeight
	if tableHeight < 1 {
		tableHeight = 1
	}

	if a.table != nil {
		tableWidth := contentWidth - 4
		if tableWidth < 1 {
			tableWidth = 1
		}
		sections = append(sections, a.table.Render(a.styles, tableWidth, tableHeight))
	} else if len(a.register.transactions) == 0 {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No transactions"))
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
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
	return a.statusbar.Render(a.styles, a.width)
}

// getKeyHints returns key hints for the current view.
func (a *App) getKeyHints() string {
	common := "Alt+key/F10 menu  1 dashboard  2 scheduled  3 reports  ? help  ctrl+q quit"

	switch a.currentView {
	case ViewDashboard:
		return "↑↓ navigate  ←→ collapse/expand  enter select  " + common
	case ViewRegister:
		return "↑↓ navigate  enter edit  n new  t transfer  d delete  esc back  " + common
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

// scheduledDueCountMsg is sent when the count of due scheduled transactions is loaded.
type scheduledDueCountMsg struct {
	count int
}

// dashboardData holds the loaded data for the dashboard view.
type dashboardData struct {
	netWorth     *models.NetWorthReport
	dueTxns      []*models.ScheduledTransaction
	upcomingTxns []*models.ScheduledTransaction
	payeeNames   map[models.ID]string
	accountNames map[models.ID]string
}

// dashboardLoadedMsg is sent when dashboard data has been loaded.
type dashboardLoadedMsg struct {
	data *dashboardData
}

// registerData holds the loaded data for the account register view.
type registerData struct {
	account       *models.Account
	transactions  []*models.Transaction
	balance       *service.AccountBalance
	payeeNames    map[models.ID]string
	categoryNames map[models.ID]string
	accountNames  map[models.ID]string
}

// registerLoadedMsg is sent when register data has been loaded.
type registerLoadedMsg struct {
	data *registerData
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
