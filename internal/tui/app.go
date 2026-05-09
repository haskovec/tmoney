// Package tui provides the terminal user interface for TMoney.
package tui

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/applog"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/reconciliation"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transferlink"
	"github.com/haskovec/tmoney/internal/tui/theme"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
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
	// ViewReconciliation shows the reconciliation view.
	ViewReconciliation
	// ViewSecurities shows the security management view.
	ViewSecurities
	// ViewPrices shows the price management view.
	ViewPrices
	// ViewInvestmentRegister shows the investment account transaction register.
	ViewInvestmentRegister
	// ViewPortfolio shows the investment account portfolio (holdings) view.
	ViewPortfolio
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
	case ViewReconciliation:
		return "Reconciliation"
	case ViewSecurities:
		return "Securities"
	case ViewPrices:
		return "Prices"
	case ViewInvestmentRegister:
		return "Investment Register"
	case ViewPortfolio:
		return "Portfolio"
	default:
		return "Unknown"
	}
}

// App is the main TUI application model.
type App struct {
	// Database connection
	db     *db.DB
	prevDB *db.DB // deferred close: kept alive until next switch so in-flight goroutines don't hit a nil conn

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
	accountSvc        *account.Service
	transactionSvc    *transaction.Service
	categorySvc       *category.Service
	payeeSvc          *payee.Service
	scheduledTxnSvc   *scheduled.Service
	reportSvc         *report.Service
	reconciliationSvc *reconciliation.Service

	// Dashboard data (loaded asynchronously)
	dashboard                 *dashboardData
	dashboardExpandedAccounts map[types.ID]bool // tracks expanded investment accounts on dashboard

	// Register data (loaded when account is selected)
	register *registerData
	table    *Table

	// Transaction dialog state
	txnDialog            *Dialog
	txnDialogData        *transactionDialogData
	txnDialogCategoryIDs []types.ID

	// Split dialog state
	splitDialog     *SplitDialog
	pendingSplitTxn *pendingSplitTransaction

	// Transfer dialog state
	transferDialog           *Dialog
	transferDialogData       *transferDialogData
	transferDialogAccountIDs []types.ID

	// Account dialog state
	acctDialog     *Dialog
	acctDialogData *accountDialogData

	// Scheduled dialog state
	schedDialog            *Dialog
	schedDialogData        *scheduledDialogData
	schedDialogAccountIDs  []types.ID
	schedDialogCategoryIDs []types.ID

	// Scheduled view state
	scheduled      *scheduledViewData
	scheduledTable *Table

	// Reports view state
	reports *reportsViewData

	// Reconciliation view state
	reconciliation      *reconciliationViewData
	reconciliationTable *Table
	reconDialog         *Dialog

	// Security view state
	securityView         *securityViewData
	securityTable        *Table
	securityDialog       *Dialog
	securityDialogMode   securityDialogMode
	securityDialogEditID types.ID
	securitySvc          *security.Service

	// Price view state
	priceView         *priceViewData
	priceTable        *Table // detail-mode: history for one security
	priceListTable    *Table // list-mode: latest price per ticker
	priceDialog       *Dialog
	priceDialogMode   priceDialogMode
	priceDialogEditID types.ID
	priceImportDialog *Dialog
	priceSvc          *price.Service

	// Investment register state
	investmentRegister     *investmentRegisterData
	investmentTable        *Table
	investmentSvc          *investment.Service
	investmentRepo         *investment.Repository
	investmentTypeSelector *Dialog
	investmentEditTxnID    types.ID // set when editing an existing transaction

	// Buy dialog state
	buyDialog            *Dialog
	buyDialogData        *buyDialogData
	buyDialogSecurityIDs []types.ID

	// Sell dialog state
	sellDialog            *Dialog
	sellDialogData        *sellDialogData
	sellDialogSecurityIDs []types.ID
	sellDialogLots        []*investment.Lot

	// Dividend dialog state
	dividendDialog            *Dialog
	dividendDialogData        *dividendDialogData
	dividendDialogSecurityIDs []types.ID
	dividendDialogReinvest    bool // true when dialog is for reinvest dividend

	// Cash operation dialog state (deposit, withdrawal, fee, interest)
	cashOperationDialog *Dialog
	cashOperationType   investment.TransactionType

	// Transfer cash dialog state (between investment and regular accounts)
	transferCashDialog           *Dialog
	transferCashDialogData       *transferCashDialogData
	transferCashDialogAccountIDs []types.ID
	transferCashDirection        string // "deposit" or "withdraw"

	// Transfer shares dialog state (between investment accounts)
	transferSharesDialog            *Dialog
	transferSharesDialogData        *transferSharesDialogData
	transferSharesDialogAccountIDs  []types.ID
	transferSharesDialogSecurityIDs []types.ID
	transferSharesDialogLots        []*investment.Lot

	// Portfolio view state
	portfolioData          *portfolioViewData
	portfolioHoldingsTable *Table
	portfolioLotsTable     *Table
	portfolioMode          portfolioViewMode

	// Corporate action service and stock split dialog state
	corporateActionSvc            *investment.CorporateActionService
	stockSplitDialog              *Dialog
	stockSplitDialogData          *stockSplitDialogData
	stockSplitDialogSecurityIDs   []types.ID
	stockSplitDialogPreSelectedID *types.ID

	// Merger dialog state
	mergerDialog              *Dialog
	mergerDialogData          *mergerDialogData
	mergerDialogSecurityIDs   []types.ID
	mergerDialogPreSelectedID *types.ID

	// Merger confirmation overlay state
	mergerConfirmData   *mergerConfirmData
	mergerConfirmParams *mergerConfirmParams

	// Spin-off dialog state
	spinOffDialog              *Dialog
	spinOffDialogData          *spinOffDialogData
	spinOffDialogSecurityIDs   []types.ID
	spinOffDialogPreSelectedID *types.ID

	// Corporate action history overlay state
	corporateActionHistory      *corporateActionHistoryData
	corporateActionHistoryTable *Table

	// Repositories for investment dialogs
	lotRepo      *investment.LotRepository
	positionRepo *investment.PositionRepository

	// File dialog state
	fileDialog     *Dialog
	fileDialogMode fileDialogMode
	browseDir      string

	// Import dialog state (transaction import via File → Import)
	importDialog      *Dialog
	importDialogState *importDialogState

	// Link Transfers dialog state (Transactions → Link Transfers)
	linkTransfersDialog *Dialog
	linkTransfersResult *transferlink.Result

	// Confirmation dialog state
	confirmDialog *Dialog
	confirmAction func() tea.Msg

	// About dialog (Help → About)
	aboutDialog *Dialog

	// Backup dialog state (for restore selection)
	backupDialog *backupDialogState

	// Undo/redo manager (session-based, not persisted)
	undoManager *undo.Manager

	// Configuration
	cfg *config.Config

	// Help overlay state
	showHelp bool

	// Key bindings
	keys keyMap

	// Mouse double-click trackers (lazy-initialized on first click).
	sidebarClicks      *ClickTracker
	priceListClicks    *ClickTracker
	browseDialogClicks *ClickTracker
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
	Securities       key.Binding
	Prices           key.Binding
	Menu             key.Binding
	MenuFile         key.Binding
	MenuAccounts     key.Binding
	MenuTransactions key.Binding
	MenuSecurities   key.Binding
	MenuEdit         key.Binding
	MenuView         key.Binding
	MenuReports      key.Binding
	MenuHelp         key.Binding
	Undo             key.Binding
	Redo             key.Binding
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
		Securities: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", "securities"),
		),
		Prices: key.NewBinding(
			key.WithKeys("5"),
			key.WithHelp("5", "prices"),
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
		MenuSecurities: key.NewBinding(
			key.WithKeys("alt+s"),
			key.WithHelp("Alt+S", "securities menu"),
		),
		MenuReports: key.NewBinding(
			key.WithKeys("alt+r"),
			key.WithHelp("Alt+R", "reports menu"),
		),
		MenuEdit: key.NewBinding(
			key.WithKeys("alt+e"),
			key.WithHelp("Alt+E", "edit menu"),
		),
		MenuView: key.NewBinding(
			key.WithKeys("alt+v"),
			key.WithHelp("Alt+V", "view menu"),
		),
		MenuHelp: key.NewBinding(
			key.WithKeys("alt+h"),
			key.WithHelp("Alt+H", "help menu"),
		),
		Undo: undoKeyBinding(),
		Redo: redoKeyBinding(),
	}
}

// undoKeyBinding returns the platform-aware undo key binding.
// On macOS, Cmd+Z is the primary binding; Ctrl+Z is also accepted
// because some terminals send Ctrl+Z for Cmd+Z.
func undoKeyBinding() key.Binding {
	if runtime.GOOS == "darwin" {
		return key.NewBinding(
			key.WithKeys("ctrl+z"),
			key.WithHelp("Cmd+Z", "undo"),
		)
	}
	return key.NewBinding(
		key.WithKeys("ctrl+z"),
		key.WithHelp("Ctrl+Z", "undo"),
	)
}

// redoKeyBinding returns the platform-aware redo key binding.
func redoKeyBinding() key.Binding {
	if runtime.GOOS == "darwin" {
		return key.NewBinding(
			key.WithKeys("ctrl+y"),
			key.WithHelp("Cmd+Y", "redo"),
		)
	}
	return key.NewBinding(
		key.WithKeys("ctrl+y"),
		key.WithHelp("Ctrl+Y", "redo"),
	)
}

// newTUIServices constructs an *app.Services for use inside the TUI and
// registers the price providers the TUI exposes (currently yahoo, used by
// the securities view's "u" shortcut). All TUI code paths that swap the
// underlying database must go through this helper so the provider
// registry stays in sync with the freshly-built price service.
func newTUIServices(database *db.DB) *app.Services {
	svc := app.NewServices(database)
	svc.Price.ProviderRegistry().Register(price.NewYahooProvider())
	return svc
}

// NewApp creates a new TUI application with the given database and optional config.
func NewApp(database *db.DB, cfg *config.Config) *App {
	svc := newTUIServices(database)

	a := &App{
		db:                        database,
		cfg:                       cfg,
		currentView:               ViewDashboard,
		styles:                    NewStyles(),
		sidebar:                   NewSidebar(),
		menubar:                   NewMenuBar(),
		statusbar:                 NewStatusBar(),
		undoManager:               undo.NewManager(),
		keys:                      defaultKeyMap(),
		accountSvc:                svc.Account,
		transactionSvc:            svc.Transaction,
		categorySvc:               svc.Category,
		payeeSvc:                  svc.Payee,
		scheduledTxnSvc:           svc.Scheduled,
		reportSvc:                 svc.Report,
		reconciliationSvc:         svc.Reconciliation,
		securitySvc:               svc.Security,
		priceSvc:                  svc.Price,
		investmentSvc:             svc.Investment,
		investmentRepo:            svc.InvestmentRepo,
		dashboardExpandedAccounts: make(map[types.ID]bool),
		lotRepo:                   svc.LotRepo,
		positionRepo:              svc.PositionRepo,
		corporateActionSvc:        svc.CorporateAction,
	}

	a.menubar.SetMenuItemsBuilder(viewMenuIndex, func() []menuItem {
		var active string
		if a.cfg != nil {
			active = a.cfg.Theme
		}
		return buildThemeMenuItems(active)
	})

	// Apply the persisted theme (TH-029). On a clean load the styles
	// adopt the new palette and we're done. On parse issues the styles
	// adopt the partially-recovered theme and TH-032 surfaces the issue
	// list to the log file plus a toast on the status bar (the toast's
	// ClearToastCmd is added to the Init batch below). On an outright
	// load failure (unknown ID, unreadable file) the styles stay on the
	// embedded default and the user sees the same toast/log pair.
	// LoadTheme (TH-026) lets a user-dir file shadow the embedded
	// built-in of the same ID, so overrides are picked up here too.
	if cfg != nil && cfg.Theme != "" {
		t, issues, err := theme.LoadTheme(cfg.Theme)
		switch {
		case err != nil:
			a.surfaceThemeFailure(cfg.Theme, err)
		case len(issues) > 0:
			a.styles.applyTheme(t)
			a.surfaceThemeIssues(cfg.Theme, issues)
		default:
			a.styles.applyTheme(t)
		}
	}

	return a
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	a.updateStatusBar()
	cmds := []tea.Cmd{
		a.autoPostOnFileOpen(),
		a.loadSidebarData(),
		a.loadScheduledDueCount(),
		a.loadDashboardData(),
	}
	// If NewApp surfaced a startup theme issue/failure, the toast is
	// already on the status bar — schedule its auto-clear here so it
	// disappears after ToastDuration like any other toast.
	if a.statusbar != nil && a.statusbar.Toast() != nil {
		cmds = append(cmds, ClearToastCmd())
	}
	return tea.Batch(cmds...)
}

// autoPostOnFileOpen returns a command that runs auto-posting on startup.
func (a *App) autoPostOnFileOpen() tea.Cmd {
	if a.scheduledTxnSvc == nil {
		return nil
	}
	return func() tea.Msg {
		summary, err := a.scheduledTxnSvc.AutoPost()
		if err != nil {
			return errMsg{err: err}
		}
		return autoPostCompletedMsg{summary: summary}
	}
}

// updateStatusBar updates the status bar context and key hints for the current view.
func (a *App) updateStatusBar() {
	context := a.currentView.String()
	if a.db != nil {
		context += " - " + filepath.Base(a.db.Path())
	}
	a.statusbar.SetContext(context)
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
			payeeNames:   make(map[types.ID]string),
			accountNames: make(map[types.ID]string),
		}

		// Load net worth report
		if a.reportSvc != nil {
			report, err := a.reportSvc.NetWorthReport()
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
			var filteredUpcoming []*scheduled.Transaction
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

		// Load investment account valuations with holdings for dashboard display
		if a.investmentSvc != nil && data.netWorth != nil {
			data.investmentHoldings = make(map[types.ID]*investment.AccountValuation)
			data.securityTickers = make(map[types.ID]string)

			for _, acct := range data.netWorth.Assets {
				if acct.Type != string(account.TypeInvestment) {
					continue
				}
				val, err := a.investmentSvc.GetAccountValuation(acct.AccountID, types.Today())
				if err == nil {
					data.investmentHoldings[acct.AccountID] = val
				}
			}

			// Load security tickers for all holdings
			if a.securitySvc != nil {
				securityIDs := make(map[types.ID]bool)
				for _, val := range data.investmentHoldings {
					for _, h := range val.Holdings {
						securityIDs[h.SecurityID] = true
					}
				}
				for secID := range securityIDs {
					sec, err := a.securitySvc.GetByID(secID)
					if err == nil {
						data.securityTickers[secID] = sec.Ticker
					}
				}
			}
		}

		return dashboardLoadedMsg{data: data}
	}
}

// loadScheduledViewData returns a command that loads all data needed for the scheduled transactions view.
func (a *App) loadScheduledViewData() tea.Cmd {
	return func() tea.Msg {
		data := &scheduledViewData{
			payeeNames:    make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
		}

		// Load due scheduled transactions
		if a.scheduledTxnSvc != nil {
			due, err := a.scheduledTxnSvc.ListDue()
			if err != nil {
				return errMsg{err: err}
			}
			data.dueTxns = due
			data.dueCount = len(due)

			upcoming, err := a.scheduledTxnSvc.ListUpcoming(30)
			if err != nil {
				return errMsg{err: err}
			}
			// Filter out items already in due list
			dueIDs := make(map[string]bool)
			for _, d := range due {
				dueIDs[d.ID.String()] = true
			}
			var filteredUpcoming []*scheduled.Transaction
			for _, u := range upcoming {
				if !dueIDs[u.ID.String()] {
					filteredUpcoming = append(filteredUpcoming, u)
				}
			}
			data.upcomingTxns = filteredUpcoming

			// Build combined list: due first, then upcoming
			data.allTxns = make([]*scheduled.Transaction, 0, len(due)+len(filteredUpcoming))
			data.allTxns = append(data.allTxns, due...)
			data.allTxns = append(data.allTxns, filteredUpcoming...)
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

		// Load account names
		if a.accountSvc != nil {
			accounts, err := a.accountSvc.List(true)
			if err == nil {
				for _, acc := range accounts {
					data.accountNames[acc.ID] = acc.Name
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

		return scheduledViewDataLoadedMsg{data: data}
	}
}

// loadReportsViewData returns a command that loads report data for the reports view.
func (a *App) loadReportsViewData(rt reportType, year, month int) tea.Cmd {
	return func() tea.Msg {
		data := &reportsViewData{
			rtype: rt,
			year:  year,
			month: month,
		}

		switch rt {
		case reportTypeNetWorth:
			if a.reportSvc != nil {
				report, err := a.reportSvc.NetWorthReport()
				if err != nil {
					return errMsg{err: err}
				}
				data.netWorth = report
			}
		case reportTypeSpending:
			if a.reportSvc != nil {
				var report *report.Spending
				var err error
				if month > 0 {
					report, err = a.reportSvc.SpendingByCategoryMonth(year, month)
				} else {
					report, err = a.reportSvc.SpendingByCategoryYear(year)
				}
				if err != nil {
					return errMsg{err: err}
				}
				data.spending = report
			}
		}

		return reportsViewDataLoadedMsg{data: data}
	}
}

// loadRegisterData returns a command that loads all data needed for the register view.
func (a *App) loadRegisterData(accountID types.ID) tea.Cmd {
	return func() tea.Msg {
		data := &registerData{
			payeeNames:    make(map[types.ID]string),
			categoryNames: make(map[types.ID]string),
			accountNames:  make(map[types.ID]string),
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

	case tea.KeyPressMsg:
		return a.handleKeyPress(msg)

	case tea.MouseMsg:
		return a.handleMouseEvent(msg)

	case mouseOpenAccountMsg:
		acct := a.sidebar.SelectedAccount()
		if acct != nil && acct.Type == account.TypeInvestment {
			a.portfolioData = nil
			a.switchView(ViewPortfolio)
			return a, a.loadPortfolioData(msg.accountID)
		}
		a.register = nil
		a.switchView(ViewRegister)
		return a, a.loadRegisterData(msg.accountID)

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

	case ToastClearMsg:
		// Auto-clear after ToastDuration. If a newer toast was set in
		// the meantime, it carries its own clear cmd, so dropping the
		// current toast here is safe — the next clear will fire when
		// that one's timer expires.
		a.statusbar.ClearToast()
		return a, nil

	case dashboardLoadedMsg:
		a.dashboard = msg.data
		// Auto-expand investment accounts that have holdings
		if a.dashboardExpandedAccounts == nil {
			a.dashboardExpandedAccounts = make(map[types.ID]bool)
		}
		if msg.data != nil && msg.data.investmentHoldings != nil {
			for accountID, val := range msg.data.investmentHoldings {
				if len(val.Holdings) > 0 {
					// Only set if not already explicitly toggled by user
					if _, exists := a.dashboardExpandedAccounts[accountID]; !exists {
						a.dashboardExpandedAccounts[accountID] = true
					}
				}
			}
		}
		return a, nil

	case registerLoadedMsg:
		a.register = msg.data
		a.buildRegisterTable()
		return a, nil

	case investmentRegisterLoadedMsg:
		a.investmentRegister = msg.data
		a.buildInvestmentRegisterTable()
		return a, nil

	case portfolioLoadedMsg:
		a.portfolioData = msg.data
		a.portfolioMode = portfolioViewHoldings
		a.buildPortfolioHoldingsTable()
		return a, nil

	case portfolioLotDetailMsg:
		if a.portfolioData != nil {
			a.portfolioData.lotDetails = msg.lots
			a.portfolioData.lotSecurityID = msg.securityID
			a.buildPortfolioLotsTable()
			if a.portfolioLotsTable != nil {
				a.portfolioLotsTable.SetFocused(true)
			}
			if a.portfolioHoldingsTable != nil {
				a.portfolioHoldingsTable.SetFocused(false)
			}
		}
		return a, nil

	case investmentTransactionDeletedMsg:
		a.statusbar.AddNotification("Transaction deleted", NotificationInfo)
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			return a, a.loadInvestmentRegisterData(a.investmentRegister.account.ID)
		}
		return a, nil

	case investmentTransactionClearedMsg:
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			return a, a.loadInvestmentRegisterData(a.investmentRegister.account.ID)
		}
		return a, nil

	case buyDialogDataMsg:
		a.buyDialogData = msg.data
		secOptions, secIDs := buildSecurityOptions(msg.data.securities)
		a.buyDialogSecurityIDs = secIDs
		var editTxn *investment.Transaction
		if a.investmentEditTxnID != types.NilID && a.investmentRepo != nil {
			editTxn, _ = a.investmentRepo.GetByID(a.investmentEditTxnID)
		}
		a.buyDialog = buildBuyDialog(secOptions, editTxn, secIDs)
		return a, nil

	case buyDialogSavedMsg:
		a.statusbar.AddNotification("Buy transaction saved", NotificationInfo)
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			return a, a.loadInvestmentRegisterData(a.investmentRegister.account.ID)
		}
		return a, nil

	case sellDialogDataMsg:
		a.sellDialogData = msg.data
		secOptions, secIDs := buildSecurityOptions(msg.data.securities)
		a.sellDialogSecurityIDs = secIDs
		a.sellDialogLots = msg.data.lots
		var editTxn *investment.Transaction
		if a.investmentEditTxnID != types.NilID && a.investmentRepo != nil {
			editTxn, _ = a.investmentRepo.GetByID(a.investmentEditTxnID)
		}
		a.sellDialog = buildSellDialog(secOptions, editTxn, secIDs, msg.data.lots)
		return a, nil

	case sellDialogSavedMsg:
		a.statusbar.AddNotification("Sell transaction saved", NotificationInfo)
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			return a, a.loadInvestmentRegisterData(a.investmentRegister.account.ID)
		}
		return a, nil

	case dividendDialogDataMsg:
		a.dividendDialogData = msg.data
		secOptions, secIDs := buildSecurityOptions(msg.data.securities)
		a.dividendDialogSecurityIDs = secIDs
		var editTxn *investment.Transaction
		if a.investmentEditTxnID != types.NilID && a.investmentRepo != nil {
			editTxn, _ = a.investmentRepo.GetByID(a.investmentEditTxnID)
		}
		if a.dividendDialogReinvest {
			a.dividendDialog = buildReinvestDividendDialog(secOptions, editTxn, secIDs)
		} else {
			a.dividendDialog = buildDividendDialog(secOptions, editTxn, secIDs)
		}
		return a, nil

	case dividendDialogSavedMsg:
		label := "Dividend"
		if a.dividendDialogReinvest {
			label = "Reinvest dividend"
		}
		a.statusbar.AddNotification(label+" transaction saved", NotificationInfo)
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			return a, a.loadInvestmentRegisterData(a.investmentRegister.account.ID)
		}
		return a, nil

	case cashOperationDialogSavedMsg:
		label := string(a.cashOperationType)
		if label == "" {
			label = "Cash operation"
		} else {
			label = a.cashOperationType.DisplayName()
		}
		a.statusbar.AddNotification(label+" transaction saved", NotificationInfo)
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			return a, a.loadInvestmentRegisterData(a.investmentRegister.account.ID)
		}
		return a, nil

	case transferCashDialogDataMsg:
		a.transferCashDialogData = msg.data
		acctOptions, acctIDs := buildNonInvestmentAccountOptions(msg.data.accounts)
		a.transferCashDialogAccountIDs = acctIDs
		var editTxn *investment.Transaction
		if a.investmentEditTxnID != types.NilID && a.investmentRepo != nil {
			editTxn, _ = a.investmentRepo.GetByID(a.investmentEditTxnID)
		}
		a.transferCashDialog = buildTransferCashDialog(a.transferCashDirection, acctOptions, editTxn, acctIDs)
		return a, nil

	case transferCashDialogSavedMsg:
		a.statusbar.AddNotification("Cash transfer saved", NotificationInfo)
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			return a, a.loadInvestmentRegisterData(a.investmentRegister.account.ID)
		}
		return a, nil

	case transferSharesDialogDataMsg:
		a.transferSharesDialogData = msg.data
		secOptions, secIDs := buildSecurityOptions(msg.data.securities)
		a.transferSharesDialogSecurityIDs = secIDs
		excludeID := types.NilID
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			excludeID = a.investmentRegister.account.ID
		}
		acctOptions, acctIDs := buildInvestmentAccountOptions(msg.data.investmentAccounts, excludeID)
		a.transferSharesDialogAccountIDs = acctIDs
		a.transferSharesDialogLots = msg.data.lots
		var editTxn *investment.Transaction
		if a.investmentEditTxnID != types.NilID && a.investmentRepo != nil {
			editTxn, _ = a.investmentRepo.GetByID(a.investmentEditTxnID)
		}
		a.transferSharesDialog = buildTransferSharesDialog(acctOptions, secOptions, editTxn, acctIDs, secIDs, msg.data.lots)
		return a, nil

	case transferSharesDialogSavedMsg:
		a.statusbar.AddNotification("Share transfer saved", NotificationInfo)
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			return a, a.loadInvestmentRegisterData(a.investmentRegister.account.ID)
		}
		return a, nil

	case stockSplitDialogDataMsg:
		a.stockSplitDialogData = msg.data
		secOptions, secIDs := buildSecurityOptions(msg.data.securities)
		a.stockSplitDialogSecurityIDs = secIDs
		a.stockSplitDialog = buildStockSplitDialog(secOptions, secIDs, a.stockSplitDialogPreSelectedID)
		a.stockSplitDialogPreSelectedID = nil
		return a, nil

	case stockSplitDialogSavedMsg:
		a.statusbar.AddNotification("Stock split executed", NotificationInfo)
		return a, a.loadSecurityViewData()

	case mergerDialogDataMsg:
		a.mergerDialogData = msg.data
		secOptions, secIDs := buildSecurityOptions(msg.data.securities)
		a.mergerDialogSecurityIDs = secIDs
		a.mergerDialog = buildMergerDialog(secOptions, secIDs, a.mergerDialogPreSelectedID)
		a.mergerDialogPreSelectedID = nil
		return a, nil

	case mergerConfirmDataMsg:
		a.mergerConfirmData = msg.data
		return a, nil

	case mergerDialogSavedMsg:
		a.statusbar.AddNotification("Merger executed", NotificationInfo)
		return a, a.loadSecurityViewData()

	case spinOffDialogDataMsg:
		a.spinOffDialogData = msg.data
		secOptions, secIDs := buildSecurityOptions(msg.data.securities)
		a.spinOffDialogSecurityIDs = secIDs
		a.spinOffDialog = buildSpinOffDialog(secOptions, secIDs, a.spinOffDialogPreSelectedID)
		a.spinOffDialogPreSelectedID = nil
		return a, nil

	case spinOffDialogSavedMsg:
		a.statusbar.AddNotification("Spin-off executed", NotificationInfo)
		return a, a.loadSecurityViewData()

	case corporateActionHistoryDataLoadedMsg:
		a.corporateActionHistory = msg.data
		a.buildCorporateActionHistoryTable()
		return a, nil

	case scheduledViewDataLoadedMsg:
		a.scheduled = msg.data
		a.buildScheduledTable()
		return a, nil

	case reportsViewDataLoadedMsg:
		a.reports = msg.data
		return a, nil

	case scheduledPostedMsg:
		return a, tea.Batch(
			a.loadScheduledViewData(),
			a.loadSidebarData(),
			a.loadScheduledDueCount(),
		)

	case scheduledSkippedMsg:
		return a, tea.Batch(
			a.loadScheduledViewData(),
			a.loadScheduledDueCount(),
		)

	case scheduledDeletedMsg:
		return a, tea.Batch(
			a.loadScheduledViewData(),
			a.loadScheduledDueCount(),
		)

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

	case scheduledDialogDataMsg:
		a.schedDialogData = msg.data
		accountOptions, accountIDs := buildAccountOptions(msg.data.accounts)
		a.schedDialogAccountIDs = accountIDs

		var categories []*category.Category
		if a.categorySvc != nil {
			cats, err := a.categorySvc.List()
			if err == nil {
				categories = cats
			}
		}
		categoryOptions, categoryIDs := buildCategoryOptions(categories)
		a.schedDialogCategoryIDs = categoryIDs

		if msg.data.mode == scheduledDialogModeEdit && msg.data.scheduled != nil {
			// Build payee name map for edit dialog
			payeeNames := make(map[types.ID]string)
			for _, p := range msg.data.payees {
				payeeNames[p.ID] = p.Name
			}
			a.schedDialog = buildEditScheduledDialog(msg.data.scheduled, accountOptions, accountIDs, categoryOptions, categoryIDs, payeeNames)
		} else {
			a.schedDialog = buildNewScheduledDialog(accountOptions, categoryOptions)
		}
		return a, nil

	case scheduledDialogSavedMsg:
		return a, tea.Batch(
			a.loadScheduledViewData(),
			a.loadScheduledDueCount(),
			a.loadSidebarData(),
		)

	case autoPostCompletedMsg:
		if msg.summary != nil && msg.summary.PostedCount > 0 {
			text := fmt.Sprintf("Auto-posted %d scheduled transaction(s)", msg.summary.PostedCount)
			a.statusbar.AddNotification(text, NotificationInfo)
			// Register auto-post as a single undo step
			if a.undoManager != nil && a.transactionSvc != nil && a.scheduledTxnSvc != nil {
				cmd := undo.NewAutoPostCommand(a.transactionSvc, a.scheduledTxnSvc, msg.summary)
				a.undoManager.Push(cmd)
			}
			// Reload data since auto-posting created transactions
			return a, tea.Batch(
				a.loadSidebarData(),
				a.loadScheduledDueCount(),
				a.loadDashboardData(),
			)
		}
		return a, nil

	case accountDialogDataMsg:
		a.acctDialogData = msg.data
		if msg.data.mode == accountDialogModeEdit && msg.data.account != nil {
			a.acctDialog = buildEditAccountDialog(msg.data.account)
		} else {
			a.acctDialog = buildNewAccountDialog()
		}
		return a, nil

	case accountDialogSavedMsg:
		cmds := []tea.Cmd{a.loadSidebarData(), a.loadDashboardData()}
		if a.currentView == ViewRegister {
			accountID := a.sidebar.SelectedAccountID()
			cmds = append(cmds, a.loadRegisterData(accountID))
		}
		return a, tea.Batch(cmds...)

	case fileDialogSavedMsg:
		return a.switchDatabase(msg.db)

	case backupCreatedMsg:
		a.statusbar.AddNotification(
			fmt.Sprintf("Backup created: %s", backupFilename(msg.path)),
			NotificationInfo,
		)
		return a, nil

	case restoreConfirmedMsg:
		// Switch to the reopened database
		model, cmd := a.switchDatabase(msg.db)
		a.statusbar.AddNotification(
			fmt.Sprintf("Restored from backup (safety backup: %s)", backupFilename(msg.safetyBackupPath)),
			NotificationInfo,
		)
		return model, cmd

	case reconciliationStartedMsg:
		// Session started, switch to reconciliation view and load data
		a.switchView(ViewReconciliation)
		return a, a.loadReconciliationData(msg.session, msg.account)

	case reconciliationLoadedMsg:
		a.reconciliation = msg.data
		a.buildReconciliationTable()
		return a, nil

	case reconciliationClearedTotalMsg:
		if a.reconciliation != nil {
			a.reconciliation.clearedTotal = msg.clearedTotal
		}
		return a, nil

	case reconciliationFinishedMsg:
		acctName := ""
		if a.reconciliation != nil {
			acctName = a.reconciliation.account.Name
		}
		a.reconciliation = nil
		a.reconciliationTable = nil
		a.switchView(ViewRegister)
		a.statusbar.AddNotification(
			fmt.Sprintf("Reconciliation completed for %s", acctName),
			NotificationInfo,
		)
		accountID := a.sidebar.SelectedAccountID()
		return a, tea.Batch(
			a.loadRegisterData(accountID),
			a.loadSidebarData(),
		)

	case reconciliationCancelledMsg:
		a.reconciliation = nil
		a.reconciliationTable = nil
		a.switchView(ViewRegister)
		a.statusbar.AddNotification("Reconciliation cancelled", NotificationInfo)
		return a, nil

	case accountDeletedMsg:
		a.switchView(ViewDashboard)
		return a, tea.Batch(a.loadSidebarData(), a.loadDashboardData())

	case accountClosedMsg:
		cmds := []tea.Cmd{a.loadSidebarData(), a.loadDashboardData()}
		if a.currentView == ViewRegister {
			accountID := a.sidebar.SelectedAccountID()
			cmds = append(cmds, a.loadRegisterData(accountID))
		}
		if a.currentView == ViewInvestmentRegister {
			accountID := a.sidebar.SelectedAccountID()
			cmds = append(cmds, a.loadInvestmentRegisterData(accountID))
		}
		if a.currentView == ViewPortfolio && a.portfolioData != nil && a.portfolioData.account != nil {
			cmds = append(cmds, a.loadPortfolioData(a.portfolioData.account.ID))
		}
		return a, tea.Batch(cmds...)

	case undoResultMsg:
		if errors.Is(msg.err, undo.ErrNothingToUndo) {
			a.statusbar.AddNotification("Nothing to undo", NotificationInfo)
			return a, nil
		}
		if errors.Is(msg.err, undo.ErrNothingToRedo) {
			a.statusbar.AddNotification("Nothing to redo", NotificationInfo)
			return a, nil
		}
		if msg.err != nil {
			a.err = msg.err
			return a, nil
		}
		a.statusbar.AddNotification(
			fmt.Sprintf("%s: %s", msg.action, msg.description),
			NotificationInfo,
		)
		// Reload current view data after undo/redo
		return a, a.reloadCurrentView()

	case securityViewDataLoadedMsg:
		a.securityView = msg.data
		a.buildSecurityTable()
		return a, nil

	case securityAddedMsg:
		a.statusbar.AddNotification("Security added", NotificationInfo)
		return a, a.loadSecurityViewData()

	case securityUpdatedMsg:
		a.statusbar.AddNotification("Security updated", NotificationInfo)
		return a, a.loadSecurityViewData()

	case securityDeletedMsg:
		a.statusbar.AddNotification("Security deleted", NotificationInfo)
		return a, a.loadSecurityViewData()

	case securityHiddenMsg:
		if msg.hidden {
			a.statusbar.AddNotification("Security hidden", NotificationInfo)
		} else {
			a.statusbar.AddNotification("Security unhidden", NotificationInfo)
		}
		return a, a.loadSecurityViewData()

	case priceViewDataLoadedMsg:
		// Preserve the existing historyCache across reload so that the
		// per-security evictions performed by the price-CRUD handlers
		// (PC-015) and the full clear performed by bulk refresh (PC-016)
		// are not silently undone by the fresh empty cache that
		// loadPriceViewData/loadPriceViewDataForSecurity construct.
		if a.priceView != nil && a.priceView.historyCache != nil {
			msg.data.historyCache = a.priceView.historyCache
		}
		a.priceView = msg.data
		var cmd tea.Cmd
		switch msg.data.mode {
		case pricesViewList:
			a.buildPriceListTable()
			// Kick off the initial debounced fetch for the row under the
			// cursor so the chart panel populates without requiring a
			// keystroke. Subsequent cursor movement reschedules.
			if secID := a.listCursorSecurityID(); !secID.IsNil() {
				cmd = a.schedulePriceChartFetch(secID)
			}
		case pricesViewDetail:
			a.buildPriceTable()
		}
		return a, cmd

	case priceChartDebounceTickMsg:
		if a.priceView == nil {
			return a, nil
		}
		// Stale: a later schedule has superseded this tick.
		if msg.gen != a.priceView.chartDebounceGen {
			return a, nil
		}
		// Cursor moved off the row this tick was scheduled for. Drop
		// silently — the move that triggered the change scheduled its
		// own fresh tick.
		if a.listCursorSecurityID() != msg.secID {
			return a, nil
		}
		// Already cached (e.g. user scrolled away and back, or a CRUD
		// invalidation re-fired before the tick). No fetch needed; just
		// promote to displayed.
		if a.priceView.historyCache != nil {
			if _, ok := a.priceView.historyCache.Lookup(msg.secID); ok {
				a.priceView.chartDisplayedID = msg.secID
				return a, nil
			}
		}
		return a, a.fetchPriceChartHistory(msg.secID)

	case priceChartHistoryLoadedMsg:
		if a.priceView == nil {
			return a, nil
		}
		if a.priceView.historyCache != nil {
			a.priceView.historyCache.Put(msg.secID, msg.prices)
		}
		a.priceView.chartDisplayedID = msg.secID
		return a, nil

	case priceAddedMsg:
		a.statusbar.AddNotification("Price added", NotificationInfo)
		a.evictSelectedSecurityFromHistoryCache()
		return a, a.reloadPriceViewKeepingMode()

	case priceUpdatedMsg:
		a.statusbar.AddNotification("Price updated", NotificationInfo)
		a.evictSelectedSecurityFromHistoryCache()
		return a, a.reloadPriceViewKeepingMode()

	case priceDeletedMsg:
		a.statusbar.AddNotification("Price deleted", NotificationInfo)
		a.evictSelectedSecurityFromHistoryCache()
		return a, a.reloadPriceViewKeepingMode()

	case priceImportedMsg:
		a.statusbar.AddNotification(
			fmt.Sprintf("Imported %d prices (%d skipped)", msg.imported, msg.skipped),
			NotificationInfo,
		)
		a.evictSelectedSecurityFromHistoryCache()
		return a, a.reloadPriceViewKeepingMode()

	case importDialogOpenMsg:
		d, ids := buildImportOptionsDialog(msg.accounts, msg.defaultAccountID)
		a.importDialog = d
		a.importDialogState = &importDialogState{
			step:       importStepOptions,
			accountIDs: ids,
		}
		return a, nil

	case importPreviewedMsg:
		state := msg.state
		state.preview = msg.result
		state.step = importStepConfirm
		a.importDialog = buildImportConfirmDialog(state)
		a.importDialogState = state
		return a, nil

	case importNeedsSourceMsg:
		state := msg.state
		state.step = importStepSourcePicker
		state.sourceOptions = msg.sources
		a.importDialog = buildImportSourcePickerDialog(msg.sources, state.accountName)
		a.importDialogState = state
		return a, nil

	case importCompletedMsg:
		a.statusbar.AddNotification(
			fmt.Sprintf("Imported: %d created, %d updated, %d skipped", msg.created, msg.updated, msg.skipped),
			NotificationInfo,
		)
		// Reload data so the new transactions appear in the dashboard /
		// register without the user having to navigate away and back.
		var cmds []tea.Cmd
		cmds = append(cmds, a.loadSidebarData(), a.loadDashboardData())
		if a.currentView == ViewRegister && a.register != nil {
			cmds = append(cmds, a.loadRegisterData(a.register.account.ID))
		}
		if len(msg.errors) > 0 {
			a.err = fmt.Errorf("import completed with %d errors:\n%s",
				len(msg.errors), strings.Join(msg.errors, "\n"))
		}
		return a, tea.Batch(cmds...)

	case linkTransfersPreviewedMsg:
		a.linkTransfersResult = msg.result
		a.linkTransfersDialog = buildLinkTransfersDialog(msg.result)
		return a, nil

	case linkTransfersCompletedMsg:
		summary := fmt.Sprintf("Linked %d transfer pairs", msg.linked)
		if msg.ambiguous > 0 {
			summary += fmt.Sprintf(" (%d ambiguous left for review)", msg.ambiguous)
		}
		a.statusbar.AddNotification(summary, NotificationInfo)
		var cmds []tea.Cmd
		cmds = append(cmds, a.loadSidebarData(), a.loadDashboardData())
		if a.currentView == ViewRegister && a.register != nil {
			cmds = append(cmds, a.loadRegisterData(a.register.account.ID))
		}
		if len(msg.errors) > 0 {
			parts := make([]string, len(msg.errors))
			for i, e := range msg.errors {
				parts[i] = e.Error()
			}
			a.err = fmt.Errorf("link transfers had %d errors:\n%s",
				len(msg.errors), strings.Join(parts, "\n"))
		}
		return a, tea.Batch(cmds...)

	case priceRefreshCompleteMsg:
		if msg.err != nil {
			a.err = msg.err
			return a, nil
		}
		a.statusbar.AddNotification(summarizeRefreshResult(msg.result), NotificationInfo)
		// PC-016: bulk refresh can silently change any subset of
		// tickers' prices, and the result doesn't enumerate which.
		// Drop every chart-history cache entry so the next render
		// re-fetches from the price service.
		if a.priceView != nil && a.priceView.historyCache != nil {
			a.priceView.historyCache.Clear()
		}
		// Re-load any data views that may now be stale.
		var cmds []tea.Cmd
		if a.currentView == ViewSecurities {
			cmds = append(cmds, a.loadSecurityViewData())
		}
		if a.currentView == ViewPrices {
			cmds = append(cmds, a.loadPriceViewData())
		}
		return a, tea.Batch(cmds...)

	case errMsg:
		a.err = msg.err
		return a, nil
	}

	return a, nil
}

// handleKeyPress handles keyboard input.
func (a *App) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// If an error is displayed, any key press dismisses it
	if a.err != nil {
		a.err = nil
		return a, nil
	}

	// If help overlay is visible, close it on ? or Esc
	if a.showHelp {
		if key.Matches(msg, a.keys.Help) || key.Matches(msg, a.keys.Escape) {
			a.showHelp = false
		}
		return a, nil
	}

	// If confirmation dialog is visible, route all keys to it
	if a.confirmDialog != nil && a.confirmDialog.IsVisible() {
		return a.handleConfirmDialogKey(msg)
	}

	// If About dialog is visible, route all keys to it
	if a.aboutDialog != nil && a.aboutDialog.IsVisible() {
		return a.handleAboutDialogKey(msg)
	}

	// If backup dialog is visible, route all keys to it
	if a.backupDialog != nil && a.backupDialog.dialog.IsVisible() {
		return a.handleBackupDialogKey(msg)
	}

	// If file dialog is visible, route all keys to it
	if a.fileDialog != nil && a.fileDialog.IsVisible() {
		return a.handleFileDialogKey(msg)
	}

	// If transaction import dialog is visible, route all keys to it
	if a.importDialog != nil && a.importDialog.IsVisible() {
		return a.handleImportDialogKey(msg)
	}

	// If link-transfers dialog is visible, route all keys to it
	if a.linkTransfersDialog != nil && a.linkTransfersDialog.IsVisible() {
		return a.handleLinkTransfersDialogKey(msg)
	}

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

	// If scheduled dialog is visible, route all keys to it
	if a.schedDialog != nil && a.schedDialog.IsVisible() {
		return a.handleScheduledDialogKey(msg)
	}

	// If account dialog is visible, route all keys to it
	if a.acctDialog != nil && a.acctDialog.IsVisible() {
		return a.handleAccountDialogKey(msg)
	}

	// If reconciliation start dialog is visible, route all keys to it
	if a.reconDialog != nil && a.reconDialog.IsVisible() {
		return a.handleReconDialogKey(msg)
	}

	// If security dialog is visible, route all keys to it
	if a.securityDialog != nil && a.securityDialog.IsVisible() {
		return a.handleSecurityDialogKey(msg)
	}

	// If price dialog is visible, route all keys to it
	if a.priceDialog != nil && a.priceDialog.IsVisible() {
		return a.handlePriceDialogKey(msg)
	}

	// If price import dialog is visible, route all keys to it
	if a.priceImportDialog != nil && a.priceImportDialog.IsVisible() {
		return a.handlePriceImportDialogKey(msg)
	}

	// If buy dialog is visible, route all keys to it
	if a.buyDialog != nil && a.buyDialog.IsVisible() {
		return a.handleBuyDialogKey(msg)
	}

	// If sell dialog is visible, route all keys to it
	if a.sellDialog != nil && a.sellDialog.IsVisible() {
		return a.handleSellDialogKey(msg)
	}

	// If dividend dialog is visible, route all keys to it
	if a.dividendDialog != nil && a.dividendDialog.IsVisible() {
		return a.handleDividendDialogKey(msg)
	}

	// If transfer cash dialog is visible, route all keys to it
	if a.transferCashDialog != nil && a.transferCashDialog.IsVisible() {
		return a.handleTransferCashDialogKey(msg)
	}

	// If transfer shares dialog is visible, route all keys to it
	if a.transferSharesDialog != nil && a.transferSharesDialog.IsVisible() {
		return a.handleTransferSharesDialogKey(msg)
	}

	// If stock split dialog is visible, route all keys to it
	if a.stockSplitDialog != nil && a.stockSplitDialog.IsVisible() {
		return a.handleStockSplitDialogKey(msg)
	}

	// If merger confirmation overlay is visible, route all keys to it
	if a.mergerConfirmData != nil {
		return a.handleMergerConfirmKey(msg)
	}

	// If merger dialog is visible, route all keys to it
	if a.mergerDialog != nil && a.mergerDialog.IsVisible() {
		return a.handleMergerDialogKey(msg)
	}

	// If spin-off dialog is visible, route all keys to it
	if a.spinOffDialog != nil && a.spinOffDialog.IsVisible() {
		return a.handleSpinOffDialogKey(msg)
	}

	// If corporate action history overlay is visible, route all keys to it
	if a.corporateActionHistory != nil {
		return a.handleCorporateActionHistoryKeys(msg)
	}

	// If cash operation dialog is visible, route all keys to it
	if a.cashOperationDialog != nil && a.cashOperationDialog.IsVisible() {
		return a.handleCashOperationDialogKey(msg)
	}

	// If investment type selector is visible, route all keys to it
	if a.investmentTypeSelector != nil && a.investmentTypeSelector.IsVisible() {
		return a.handleInvestmentTypeSelectorKey(msg)
	}

	// Undo/redo key bindings (handled before menus since they should
	// work from any non-dialog context)
	switch {
	case key.Matches(msg, a.keys.Undo):
		return a, a.performUndo()
	case key.Matches(msg, a.keys.Redo):
		return a, a.performRedo()
	}

	// Alt+key menu shortcuts work regardless of menu state
	switch {
	case key.Matches(msg, a.keys.MenuFile):
		a.toggleMenu(0)
		return a, nil
	case key.Matches(msg, a.keys.MenuEdit):
		a.toggleMenu(1)
		return a, nil
	case key.Matches(msg, a.keys.MenuView):
		a.toggleMenu(2)
		return a, nil
	case key.Matches(msg, a.keys.MenuAccounts):
		a.toggleMenu(3)
		return a, nil
	case key.Matches(msg, a.keys.MenuTransactions):
		a.toggleMenu(4)
		return a, nil
	case key.Matches(msg, a.keys.MenuSecurities):
		a.toggleMenu(5)
		return a, nil
	case key.Matches(msg, a.keys.MenuReports):
		a.toggleMenu(6)
		return a, nil
	case key.Matches(msg, a.keys.MenuHelp):
		a.toggleMenu(7)
		return a, nil
	}

	// If menu bar is active, route all keys to menu handling
	if a.menubar.IsActive() {
		return a.handleMenuKeys(msg)
	}

	// In reconciliation view, only allow quit, help, and view-specific keys
	// Don't allow view switching or generic escape (handled by reconciliation keys)
	if a.currentView == ViewReconciliation {
		switch {
		case key.Matches(msg, a.keys.Quit):
			a.quitting = true
			return a, tea.Quit
		case key.Matches(msg, a.keys.Help):
			a.showHelp = true
			return a, nil
		}
		return a.handleReconciliationKeys(msg)
	}

	// Global key bindings
	switch {
	case key.Matches(msg, a.keys.Quit):
		a.quitting = true
		return a, tea.Quit

	case key.Matches(msg, a.keys.Help):
		a.showHelp = true
		return a, nil

	case key.Matches(msg, a.keys.Menu):
		a.menubar.Activate()
		return a, nil

	case key.Matches(msg, a.keys.Dashboard):
		a.switchView(ViewDashboard)
		return a, nil

	case key.Matches(msg, a.keys.Scheduled):
		a.switchView(ViewScheduled)
		return a, a.loadScheduledViewData()

	case key.Matches(msg, a.keys.Reports):
		a.switchView(ViewReports)
		if a.reports == nil {
			now := time.Now()
			return a, a.loadReportsViewData(reportTypeNetWorth, now.Year(), int(now.Month()))
		}
		return a, nil

	case key.Matches(msg, a.keys.Securities):
		a.switchView(ViewSecurities)
		return a, a.loadSecurityViewData()

	case key.Matches(msg, a.keys.Prices):
		a.switchView(ViewPrices)
		return a, a.loadPriceViewData()

	case key.Matches(msg, a.keys.Escape):
		// In prices detail mode, Esc returns to the prices list within the
		// view; let the view-specific handler claim the key.
		if a.currentView == ViewPrices && a.priceView != nil && a.priceView.mode == pricesViewDetail {
			return a.handlePriceViewKeys(msg)
		}
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
	case ViewReconciliation:
		return a.handleReconciliationKeys(msg)
	case ViewSecurities:
		return a.handleSecurityViewKeys(msg)
	case ViewPrices:
		return a.handlePriceViewKeys(msg)
	case ViewInvestmentRegister:
		return a.handleInvestmentRegisterKeys(msg)
	case ViewPortfolio:
		return a.handlePortfolioKeys(msg)
	}

	return a, nil
}

// handleDashboardKeys handles key presses in the dashboard view.
func (a *App) handleDashboardKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return a.handleSidebarKeys(msg)
}

// handleRegisterKeys handles key presses in the register view.
func (a *App) handleRegisterKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		tableHeight := max(a.height-6, 1)
		a.table.PageUp(tableHeight)
	case msg.String() == "pgdown":
		tableHeight := max(a.height-6, 1)
		a.table.PageDown(tableHeight)
	case msg.String() == "c":
		return a.toggleTransactionStatus()
	case msg.String() == "v":
		return a.showVoidConfirmation()
	case key.Matches(msg, a.keys.New):
		return a, a.loadTransactionDialogData()
	case msg.String() == "t":
		return a, a.loadTransferDialogData()
	}

	return a, nil
}

// toggleTransactionStatus toggles the cleared/uncleared status of the selected transaction.
func (a *App) toggleTransactionStatus() (tea.Model, tea.Cmd) {
	if a.table == nil || a.register == nil || a.transactionSvc == nil {
		return a, nil
	}

	cursor := a.table.Cursor()
	if cursor < 0 || cursor >= len(a.register.transactions) {
		return a, nil
	}

	txn := a.register.transactions[cursor]

	// Cannot toggle void transactions
	if txn.IsVoid() {
		a.statusbar.AddNotification("Cannot change status of void transaction", NotificationAlert)
		return a, nil
	}

	// Cannot toggle reconciled transactions
	if txn.IsReconciled() {
		a.statusbar.AddNotification("Cannot change status of reconciled transaction (un-reconcile first)", NotificationAlert)
		return a, nil
	}

	accountID := a.sidebar.SelectedAccountID()
	txnID := txn.ID
	currentStatus := txn.Status

	return a, func() tea.Msg {
		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}

		// Get current state from DB for the edit command
		current, err := a.transactionSvc.GetByID(txnID)
		if err != nil {
			return errMsg{err: err}
		}

		// Build updated copy with toggled status
		updated := *current
		if currentStatus == transaction.StatusCleared {
			updated.MarkUncleared()
		} else {
			updated.Clear()
		}

		cmd := undo.NewEditTransactionCommand(a.transactionSvc, &updated)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: err}
		}

		// Reload register data to reflect the change
		return a.loadRegisterData(accountID)()
	}
}

// showVoidConfirmation shows a confirmation dialog before voiding the selected transaction.
func (a *App) showVoidConfirmation() (tea.Model, tea.Cmd) {
	if a.table == nil || a.register == nil || a.transactionSvc == nil {
		return a, nil
	}

	cursor := a.table.Cursor()
	if cursor < 0 || cursor >= len(a.register.transactions) {
		return a, nil
	}

	txn := a.register.transactions[cursor]

	// Cannot void already-void transactions
	if txn.IsVoid() {
		a.statusbar.AddNotification("Transaction is already void", NotificationAlert)
		return a, nil
	}

	// Cannot void reconciled transactions
	if txn.IsReconciled() {
		a.statusbar.AddNotification("Cannot void reconciled transaction (un-reconcile first)", NotificationAlert)
		return a, nil
	}

	// Build confirmation message
	msg := "Void this transaction? Amount will be set to $0.00 and memo replaced with **VOID**."
	if txn.IsTransfer() {
		msg = "Void this transfer? Both sides will be voided. Amount will be set to $0.00."
	}

	accountID := a.sidebar.SelectedAccountID()
	txnID := txn.ID

	isTransfer := txn.IsTransfer()

	a.showConfirmDialog("Void Transaction", msg, func() tea.Msg {
		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}

		var cmd undo.Command
		if isTransfer {
			cmd = undo.NewVoidTransferCommand(a.transactionSvc, txnID)
		} else {
			cmd = undo.NewVoidTransactionCommand(a.transactionSvc, txnID)
		}
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: err}
		}
		return a.loadRegisterData(accountID)()
	})

	return a, nil
}

// showConfirmDialog displays a confirmation dialog with the given title and message.
// The action function is called when the user confirms.
func (a *App) showConfirmDialog(title, message string, action func() tea.Msg) {
	d := NewDialog(title)
	d.SetWidth(50)
	d.SetButtons([]DialogButton{
		{Label: "No"},
		{Label: "Yes", Primary: true},
	})
	// Use a text field with the message as a label (read-only visual)
	// We'll render the message as the dialog error message area (repurposed for display)
	d.SetErrorMsg(message)
	d.SetFocusIndex(len(d.Fields())) // Focus on first button (No)
	d.SetVisible(true)
	a.confirmDialog = d
	a.confirmAction = action
}

// handleConfirmDialogKey handles key input for the confirmation dialog.
func (a *App) handleConfirmDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	action := a.confirmDialog.HandleKey(msg)

	switch action {
	case DialogActionSubmit:
		a.confirmDialog.SetVisible(false)
		fn := a.confirmAction
		a.confirmDialog = nil
		a.confirmAction = nil
		return a, func() tea.Msg {
			return fn()
		}
	case DialogActionCancel:
		a.confirmDialog.SetVisible(false)
		a.confirmDialog = nil
		a.confirmAction = nil
		return a, nil
	}

	return a, nil
}

// showAboutDialog displays the Help → About dialog.
func (a *App) showAboutDialog() {
	d := NewDialog("Terminal Money")
	d.SetWidth(44)
	d.SetMessage("Author: Jeffrey Haskovec\nCopyright 2026")
	d.SetButtons([]DialogButton{
		{Label: "OK", Primary: true},
	})
	d.SetFocusIndex(len(d.Fields())) // focus the OK button
	d.SetVisible(true)
	a.aboutDialog = d
}

// handleAboutDialogKey handles key input for the About dialog.
func (a *App) handleAboutDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	action := a.aboutDialog.HandleKey(msg)
	switch action {
	case DialogActionSubmit, DialogActionCancel:
		a.aboutDialog.SetVisible(false)
		a.aboutDialog = nil
	}
	return a, nil
}

// handleScheduledKeys handles key presses in the scheduled transactions view.
func (a *App) handleScheduledKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Handle Tab to switch focus between sidebar and table
	if key.Matches(msg, a.keys.Tab) || key.Matches(msg, a.keys.ShiftTab) {
		if a.sidebar.IsFocused() {
			a.sidebar.SetFocused(false)
			if a.scheduledTable != nil {
				a.scheduledTable.SetFocused(true)
			}
		} else {
			a.sidebar.SetFocused(true)
			if a.scheduledTable != nil {
				a.scheduledTable.SetFocused(false)
			}
		}
		return a, nil
	}

	// If sidebar has focus, delegate to sidebar handling
	if a.sidebar.IsFocused() {
		return a.handleSidebarKeys(msg)
	}

	// Table-focused key handling
	if a.scheduledTable == nil || a.scheduled == nil {
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Up):
		a.scheduledTable.MoveUp()
	case key.Matches(msg, a.keys.Down):
		a.scheduledTable.MoveDown()
	case msg.String() == "home" || msg.String() == "g":
		a.scheduledTable.MoveToTop()
	case msg.String() == "end" || msg.String() == "G":
		a.scheduledTable.MoveToBottom()
	case msg.String() == "pgup":
		tableHeight := max(a.height-6, 1)
		a.scheduledTable.PageUp(tableHeight)
	case msg.String() == "pgdown":
		tableHeight := max(a.height-6, 1)
		a.scheduledTable.PageDown(tableHeight)
	case key.Matches(msg, a.keys.Enter):
		return a.postSelectedScheduled()
	case msg.String() == "s":
		return a.skipSelectedScheduled()
	case key.Matches(msg, a.keys.Delete):
		return a.deleteSelectedScheduled()
	case key.Matches(msg, a.keys.New):
		return a, a.loadNewScheduledDialogData()
	case key.Matches(msg, a.keys.Edit):
		return a, a.loadEditScheduledDialogData()
	}

	return a, nil
}

// postSelectedScheduled posts the currently selected scheduled transaction.
func (a *App) postSelectedScheduled() (tea.Model, tea.Cmd) {
	if a.scheduled == nil || a.scheduledTable == nil || a.scheduledTxnSvc == nil {
		return a, nil
	}

	cursor := a.scheduledTable.Cursor()
	if cursor < 0 || cursor >= len(a.scheduled.allTxns) {
		return a, nil
	}

	st := a.scheduled.allTxns[cursor]
	return a, func() tea.Msg {
		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}
		cmd := undo.NewPostScheduledTransactionCommand(a.scheduledTxnSvc, a.transactionSvc, st.ID, nil)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: err}
		}
		return scheduledPostedMsg{}
	}
}

// skipSelectedScheduled skips the currently selected scheduled transaction.
func (a *App) skipSelectedScheduled() (tea.Model, tea.Cmd) {
	if a.scheduled == nil || a.scheduledTable == nil || a.scheduledTxnSvc == nil {
		return a, nil
	}

	cursor := a.scheduledTable.Cursor()
	if cursor < 0 || cursor >= len(a.scheduled.allTxns) {
		return a, nil
	}

	st := a.scheduled.allTxns[cursor]
	return a, func() tea.Msg {
		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}
		cmd := undo.NewSkipScheduledTransactionCommand(a.scheduledTxnSvc, st.ID)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: err}
		}
		return scheduledSkippedMsg{}
	}
}

// deleteSelectedScheduled deletes the currently selected scheduled transaction.
func (a *App) deleteSelectedScheduled() (tea.Model, tea.Cmd) {
	if a.scheduled == nil || a.scheduledTable == nil || a.scheduledTxnSvc == nil {
		return a, nil
	}

	cursor := a.scheduledTable.Cursor()
	if cursor < 0 || cursor >= len(a.scheduled.allTxns) {
		return a, nil
	}

	st := a.scheduled.allTxns[cursor]
	return a, func() tea.Msg {
		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}
		cmd := undo.NewDeleteScheduledTransactionCommand(a.scheduledTxnSvc, st.ID)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: err}
		}
		return scheduledDeletedMsg{}
	}
}

// handleReportsKeys handles key presses in the reports view.
func (a *App) handleReportsKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.reports == nil {
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Left):
		// Navigate to previous period
		return a.reportsPreviousPeriod()

	case key.Matches(msg, a.keys.Right):
		// Navigate to next period
		return a.reportsNextPeriod()

	case msg.String() == "n":
		// Switch to net worth report
		now := time.Now()
		return a, a.loadReportsViewData(reportTypeNetWorth, now.Year(), int(now.Month()))

	case msg.String() == "s":
		// Switch to spending report
		year := a.reports.year
		month := a.reports.month
		if month == 0 {
			month = int(time.Now().Month())
		}
		return a, a.loadReportsViewData(reportTypeSpending, year, month)

	case msg.String() == "y":
		// Toggle to yearly spending view (only for spending)
		if a.reports.rtype == reportTypeSpending {
			return a, a.loadReportsViewData(reportTypeSpending, a.reports.year, 0)
		}

	case msg.String() == "m":
		// Toggle to monthly spending view (only for spending)
		if a.reports.rtype == reportTypeSpending && a.reports.month == 0 {
			return a, a.loadReportsViewData(reportTypeSpending, a.reports.year, int(time.Now().Month()))
		}
	}

	return a, nil
}

// reportsPreviousPeriod navigates to the previous time period for reports.
func (a *App) reportsPreviousPeriod() (tea.Model, tea.Cmd) {
	if a.reports == nil || a.reports.rtype != reportTypeSpending {
		return a, nil
	}

	year := a.reports.year
	month := a.reports.month

	if month > 0 {
		// Monthly: go to previous month
		month--
		if month < 1 {
			month = 12
			year--
		}
	} else {
		// Yearly: go to previous year
		year--
	}

	return a, a.loadReportsViewData(reportTypeSpending, year, month)
}

// reportsNextPeriod navigates to the next time period for reports.
func (a *App) reportsNextPeriod() (tea.Model, tea.Cmd) {
	if a.reports == nil || a.reports.rtype != reportTypeSpending {
		return a, nil
	}

	year := a.reports.year
	month := a.reports.month

	if month > 0 {
		// Monthly: go to next month
		month++
		if month > 12 {
			month = 1
			year++
		}
	} else {
		// Yearly: go to next year
		year++
	}

	return a, a.loadReportsViewData(reportTypeSpending, year, month)
}

// handleSidebarKeys handles keyboard navigation for the sidebar.
func (a *App) handleSidebarKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

	case key.Matches(msg, a.keys.Enter):
		if a.sidebar.Select() {
			accountID := a.sidebar.SelectedAccountID()
			acct := a.sidebar.SelectedAccount()
			if acct != nil && acct.Type == account.TypeInvestment {
				a.portfolioData = nil // Clear old data while loading
				a.switchView(ViewPortfolio)
				return a, a.loadPortfolioData(accountID)
			}
			a.register = nil // Clear old data while loading
			a.switchView(ViewRegister)
			return a, a.loadRegisterData(accountID)
		}
		return a, nil

	case key.Matches(msg, a.keys.New):
		return a, a.loadNewAccountDialogData()
	}

	return a, nil
}

// handleMouseEvent handles mouse events (clicks, wheel scrolling).
func (a *App) handleMouseEvent(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.MouseWheelMsg:
		return a.handleMouseWheel(msg)
	case tea.MouseClickMsg:
		if msg.Mouse().Button != tea.MouseLeft {
			return a, nil
		}
	default:
		return a, nil
	}

	m := msg.Mouse()

	// Route mouse clicks to dialog when a modal dialog is visible
	if a.isDialogVisible() {
		return a.handleDialogMouse(msg)
	}

	// Menu bar (row 0)
	if m.Y == 0 {
		return a.handleMouseMenuBar(msg)
	}

	// If menu dropdown is open, check if click is on dropdown
	if a.menubar.IsActive() {
		colOffset, dropdownWidth, itemCount := a.menubar.DropdownBounds()
		if m.Y >= 1 && m.Y <= itemCount &&
			m.X >= colOffset && m.X < colOffset+dropdownWidth {
			itemIdx := a.menubar.HitTestDropdown(m.Y - 1)
			if itemIdx >= 0 {
				a.menubar.SetItemCursor(itemIdx)
				action, data := a.menubar.Select()
				return a.handleMenuAction(action, data)
			}
		}
		// Click outside dropdown closes it
		a.menubar.Deactivate()
		// Fall through to handle the click in the content area
	}

	// Status bar (last row) - ignore
	if m.Y >= a.height-1 {
		return a, nil
	}

	// Content area
	if m.Y >= 1 && m.Y < a.height-1 {
		return a.handleMouseContent(msg)
	}

	return a, nil
}

// handleMouseMenuBar handles mouse clicks on the menu bar (row 0).
func (a *App) handleMouseMenuBar(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	idx := a.menubar.HitTestBar(msg.Mouse().X)
	if idx >= 0 {
		if a.menubar.IsActive() && a.menubar.Cursor() == idx {
			a.menubar.Deactivate()
		} else {
			a.menubar.ActivateMenu(idx)
		}
	} else if a.menubar.IsActive() {
		a.menubar.Deactivate()
	}
	return a, nil
}

// handleMouseContent handles mouse clicks in the content area.
func (a *App) handleMouseContent(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := msg.Mouse()
	contentY := m.Y - 1 // Offset for header row

	sidebarWidth := a.styles.SidebarWidth()

	// Full-screen views (no sidebar)
	if sidebarWidth == 0 || a.currentView == ViewReconciliation ||
		a.currentView == ViewSecurities || a.currentView == ViewPrices {
		return a.handleMouseTable(msg, contentY)
	}

	// Sidebar zone
	if m.X < sidebarWidth {
		return a.handleMouseSidebar(msg, contentY)
	}

	// Border column - ignore
	if m.X == sidebarWidth {
		return a, nil
	}

	// Content zone (right of sidebar)
	a.focusContent()
	return a.handleMouseTable(msg, contentY)
}

// handleMouseSidebar handles mouse clicks in the sidebar area.
// Single click on a group header just moves the cursor.
// Single click on an account moves the cursor; a double click on the same
// account opens the register/portfolio.
func (a *App) handleMouseSidebar(_ tea.MouseMsg, contentY int) (tea.Model, tea.Cmd) {
	idx := a.sidebar.HitTest(contentY)
	if idx < 0 {
		return a, nil
	}

	a.focusSidebar()
	a.sidebar.SetCursor(idx)

	item := a.sidebar.CursorItem()
	if item == nil {
		return a, nil
	}

	// Group headers: just move cursor
	if item.kind == sidebarItemGroup {
		return a, nil
	}

	// Account item - require a double click to drill in.
	if a.sidebarClicks == nil {
		a.sidebarClicks = NewClickTracker(doubleClickThreshold)
	}
	if !a.sidebarClicks.Click(idx) {
		return a, nil
	}

	// Defer the view switch to the next Update cycle.
	// This avoids a Bubbletea renderer issue where switching views directly
	// inside a mouse event handler causes the menu bar to disappear.
	if a.sidebar.Select() {
		accountID := a.sidebar.SelectedAccountID()
		return a, func() tea.Msg {
			return mouseOpenAccountMsg{accountID: accountID}
		}
	}

	return a, nil
}

// handleMouseTable handles mouse clicks in the table/content area.
// Single click moves the cursor; on tables that support drill-in
// (currently the prices list), a second click on the same row within
// the double-click threshold opens the row.
func (a *App) handleMouseTable(_ tea.MouseMsg, contentY int) (tea.Model, tea.Cmd) {
	// Table offset within content: 1 (top padding) + 1 (title) + 1 (separator) = 3
	const tableContentOffset = 3
	tableY := contentY - tableContentOffset

	tbl := a.activeTable()
	if tbl == nil {
		return a, nil
	}

	rowIdx := tbl.HitTest(tableY)
	if rowIdx < 0 {
		return a, nil
	}
	tbl.SetCursor(rowIdx)

	// Prices landing list: double-click drills into a ticker's history.
	if a.currentView == ViewPrices && a.priceView != nil && a.priceView.mode == pricesViewList {
		if a.priceListClicks == nil {
			a.priceListClicks = NewClickTracker(doubleClickThreshold)
		}
		if a.priceListClicks.Click(rowIdx) {
			return a, a.drillIntoSelectedListRow()
		}
	}

	return a, nil
}

// handleMouseWheel handles mouse wheel scrolling.
func (a *App) handleMouseWheel(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if a.isDialogVisible() {
		return a.handleDialogMouse(msg)
	}

	wheelUp := msg.Mouse().Button == tea.MouseWheelUp

	if a.sidebar.IsFocused() {
		if wheelUp {
			a.sidebar.MoveUp()
		} else {
			a.sidebar.MoveDown()
		}
		return a, nil
	}

	tbl := a.activeTable()
	if tbl != nil {
		if wheelUp {
			tbl.MoveUp()
		} else {
			tbl.MoveDown()
		}
	}

	return a, nil
}

// handleDialogMouse routes mouse events to the currently visible dialog.
// For non-Dialog overlays (SplitDialog, help, mergerConfirm, corporateActionHistory),
// mouse events are blocked (returns no-op).
func (a *App) handleDialogMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Non-Dialog overlays: block mouse events
	if a.showHelp || a.mergerConfirmData != nil || a.corporateActionHistory != nil {
		return a, nil
	}
	if a.splitDialog != nil && a.splitDialog.IsVisible() {
		return a, nil
	}

	// Dialog cascade (same order as handleKeyPress)
	if a.confirmDialog != nil && a.confirmDialog.IsVisible() {
		action := a.confirmDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			a.confirmDialog.SetVisible(false)
			fn := a.confirmAction
			a.confirmDialog = nil
			a.confirmAction = nil
			return a, func() tea.Msg { return fn() }
		case DialogActionCancel:
			a.confirmDialog.SetVisible(false)
			a.confirmDialog = nil
			a.confirmAction = nil
		}
		return a, nil
	}

	if a.aboutDialog != nil && a.aboutDialog.IsVisible() {
		action := a.aboutDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit, DialogActionCancel:
			a.aboutDialog.SetVisible(false)
			a.aboutDialog = nil
		}
		return a, nil
	}

	if a.backupDialog != nil && a.backupDialog.dialog.IsVisible() {
		action := a.backupDialog.dialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitBackupDialog()
		case DialogActionCancel:
			a.backupDialog.dialog.SetVisible(false)
			a.backupDialog = nil
		}
		return a, nil
	}

	if a.fileDialog != nil && a.fileDialog.IsVisible() {
		// In browse mode, a double-click on a list row activates that entry
		// (navigate into a directory or open a .tdb file) without requiring
		// a separate Open button press.
		listItemRow := -1
		if click, ok := msg.(tea.MouseClickMsg); ok &&
			a.fileDialogMode == fileDialogModeBrowse &&
			click.Button == tea.MouseLeft {
			listItemRow = a.browseDialogListHit(msg)
		}

		action := a.fileDialog.HandleMouse(msg, a.width, a.height)

		if listItemRow >= 0 {
			if a.browseDialogClicks == nil {
				a.browseDialogClicks = NewClickTracker(doubleClickThreshold)
			}
			if a.browseDialogClicks.Click(listItemRow) {
				return a.submitFileDialog()
			}
		}

		switch action {
		case DialogActionSubmit:
			return a.submitFileDialog()
		case DialogActionCancel:
			a.closeFileDialog()
		}
		return a, nil
	}

	if a.importDialog != nil && a.importDialog.IsVisible() {
		action := a.importDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitImportDialog()
		case DialogActionCancel:
			a.closeImportDialog()
		}
		return a, nil
	}

	if a.linkTransfersDialog != nil && a.linkTransfersDialog.IsVisible() {
		action := a.linkTransfersDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitLinkTransfersDialog()
		case DialogActionCancel:
			a.closeLinkTransfersDialog()
		}
		return a, nil
	}

	if a.txnDialog != nil && a.txnDialog.IsVisible() {
		action := a.txnDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitTransactionDialog()
		case DialogActionCancel:
			a.txnDialog.SetVisible(false)
			a.txnDialog = nil
		}
		return a, nil
	}

	if a.transferDialog != nil && a.transferDialog.IsVisible() {
		action := a.transferDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitTransferDialog()
		case DialogActionCancel:
			a.transferDialog.SetVisible(false)
			a.transferDialog = nil
		}
		return a, nil
	}

	if a.schedDialog != nil && a.schedDialog.IsVisible() {
		action := a.schedDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitScheduledDialog()
		case DialogActionCancel:
			a.schedDialog.SetVisible(false)
			a.schedDialog = nil
		}
		return a, nil
	}

	if a.acctDialog != nil && a.acctDialog.IsVisible() {
		action := a.acctDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitAccountDialog()
		case DialogActionCancel:
			a.acctDialog.SetVisible(false)
			a.acctDialog = nil
		}
		return a, nil
	}

	if a.reconDialog != nil && a.reconDialog.IsVisible() {
		action := a.reconDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitStartReconciliation()
		case DialogActionCancel:
			a.reconDialog.SetVisible(false)
			a.reconDialog = nil
		}
		return a, nil
	}

	if a.securityDialog != nil && a.securityDialog.IsVisible() {
		action := a.securityDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitSecurityDialog()
		case DialogActionCancel:
			a.securityDialog.SetVisible(false)
			a.securityDialog = nil
		}
		return a, nil
	}

	if a.priceDialog != nil && a.priceDialog.IsVisible() {
		action := a.priceDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitPriceDialog()
		case DialogActionCancel:
			a.priceDialog.SetVisible(false)
			a.priceDialog = nil
		}
		return a, nil
	}

	if a.priceImportDialog != nil && a.priceImportDialog.IsVisible() {
		action := a.priceImportDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitImportPriceDialog()
		case DialogActionCancel:
			a.priceImportDialog.SetVisible(false)
			a.priceImportDialog = nil
		}
		return a, nil
	}

	if a.buyDialog != nil && a.buyDialog.IsVisible() {
		action := a.buyDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitBuyDialog()
		case DialogActionCancel:
			a.buyDialog.SetVisible(false)
			a.buyDialog = nil
		}
		return a, nil
	}

	if a.sellDialog != nil && a.sellDialog.IsVisible() {
		action := a.sellDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitSellDialog()
		case DialogActionCancel:
			a.sellDialog.SetVisible(false)
			a.sellDialog = nil
		}
		return a, nil
	}

	if a.dividendDialog != nil && a.dividendDialog.IsVisible() {
		action := a.dividendDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitDividendDialog()
		case DialogActionCancel:
			a.dividendDialog.SetVisible(false)
			a.dividendDialog = nil
		}
		return a, nil
	}

	if a.transferCashDialog != nil && a.transferCashDialog.IsVisible() {
		action := a.transferCashDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitTransferCashDialog()
		case DialogActionCancel:
			a.transferCashDialog.SetVisible(false)
			a.transferCashDialog = nil
		}
		return a, nil
	}

	if a.transferSharesDialog != nil && a.transferSharesDialog.IsVisible() {
		action := a.transferSharesDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitTransferSharesDialog()
		case DialogActionCancel:
			a.transferSharesDialog.SetVisible(false)
			a.transferSharesDialog = nil
		}
		return a, nil
	}

	if a.stockSplitDialog != nil && a.stockSplitDialog.IsVisible() {
		action := a.stockSplitDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitStockSplitDialog()
		case DialogActionCancel:
			a.stockSplitDialog.SetVisible(false)
			a.stockSplitDialog = nil
		}
		return a, nil
	}

	if a.mergerDialog != nil && a.mergerDialog.IsVisible() {
		action := a.mergerDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitMergerDialog()
		case DialogActionCancel:
			a.mergerDialog.SetVisible(false)
			a.mergerDialog = nil
		}
		return a, nil
	}

	if a.spinOffDialog != nil && a.spinOffDialog.IsVisible() {
		action := a.spinOffDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitSpinOffDialog()
		case DialogActionCancel:
			a.spinOffDialog.SetVisible(false)
			a.spinOffDialog = nil
		}
		return a, nil
	}

	if a.cashOperationDialog != nil && a.cashOperationDialog.IsVisible() {
		action := a.cashOperationDialog.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			return a.submitCashOperationDialog()
		case DialogActionCancel:
			a.cashOperationDialog.SetVisible(false)
			a.cashOperationDialog = nil
		}
		return a, nil
	}

	if a.investmentTypeSelector != nil && a.investmentTypeSelector.IsVisible() {
		action := a.investmentTypeSelector.HandleMouse(msg, a.width, a.height)
		switch action {
		case DialogActionSubmit:
			fields := a.investmentTypeSelector.Fields()
			selectedType := investmentTransactionTypeFromIndex(fields[0].SelectedIndex)
			a.investmentTypeSelector.SetVisible(false)
			a.investmentTypeSelector = nil
			switch selectedType {
			case investment.TransactionTypeBuy:
				return a, a.loadBuyDialogData()
			case investment.TransactionTypeSell:
				return a, a.loadSellDialogData()
			case investment.TransactionTypeDividend:
				a.dividendDialogReinvest = false
				return a, a.loadDividendDialogData()
			case investment.TransactionTypeReinvestDividend:
				a.dividendDialogReinvest = true
				return a, a.loadDividendDialogData()
			case investment.TransactionTypeDeposit,
				investment.TransactionTypeWithdrawal,
				investment.TransactionTypeFee,
				investment.TransactionTypeInterest:
				a.cashOperationType = selectedType
				var editTxn *investment.Transaction
				if a.investmentEditTxnID != types.NilID && a.investmentRepo != nil {
					editTxn, _ = a.investmentRepo.GetByID(a.investmentEditTxnID)
				}
				a.cashOperationDialog = buildCashOperationDialog(selectedType.DisplayName(), editTxn)
				return a, nil
			case investment.TransactionTypeTransferCash:
				a.transferCashDirection = "deposit"
				return a, a.loadTransferCashDialogData()
			case investment.TransactionTypeTransferShares:
				return a, a.loadTransferSharesDialogData()
			}
		case DialogActionCancel:
			a.investmentTypeSelector.SetVisible(false)
			a.investmentTypeSelector = nil
		}
		return a, nil
	}

	return a, nil
}

// activeTable returns the currently active table for the current view, or nil.
func (a *App) activeTable() *Table {
	switch a.currentView {
	case ViewRegister:
		return a.table
	case ViewScheduled:
		return a.scheduledTable
	case ViewInvestmentRegister:
		return a.investmentTable
	case ViewPortfolio:
		if a.portfolioData != nil {
			return a.activePortfolioTable()
		}
	case ViewSecurities:
		return a.securityTable
	case ViewPrices:
		if a.priceView != nil && a.priceView.mode == pricesViewList {
			return a.priceListTable
		}
		return a.priceTable
	}
	return nil
}

// focusSidebar switches focus to the sidebar and unfocuses the active table.
func (a *App) focusSidebar() {
	a.sidebar.SetFocused(true)
	if tbl := a.activeTable(); tbl != nil {
		tbl.SetFocused(false)
	}
}

// focusContent switches focus to the content table and unfocuses the sidebar.
func (a *App) focusContent() {
	a.sidebar.SetFocused(false)
	if tbl := a.activeTable(); tbl != nil {
		tbl.SetFocused(true)
	}
}

// isDialogVisible returns true if any modal dialog is currently visible.
func (a *App) isDialogVisible() bool {
	return (a.confirmDialog != nil && a.confirmDialog.IsVisible()) ||
		(a.aboutDialog != nil && a.aboutDialog.IsVisible()) ||
		(a.backupDialog != nil && a.backupDialog.dialog.IsVisible()) ||
		(a.fileDialog != nil && a.fileDialog.IsVisible()) ||
		(a.splitDialog != nil && a.splitDialog.IsVisible()) ||
		(a.txnDialog != nil && a.txnDialog.IsVisible()) ||
		(a.transferDialog != nil && a.transferDialog.IsVisible()) ||
		(a.schedDialog != nil && a.schedDialog.IsVisible()) ||
		(a.acctDialog != nil && a.acctDialog.IsVisible()) ||
		(a.reconDialog != nil && a.reconDialog.IsVisible()) ||
		(a.securityDialog != nil && a.securityDialog.IsVisible()) ||
		(a.priceDialog != nil && a.priceDialog.IsVisible()) ||
		(a.priceImportDialog != nil && a.priceImportDialog.IsVisible()) ||
		(a.buyDialog != nil && a.buyDialog.IsVisible()) ||
		(a.sellDialog != nil && a.sellDialog.IsVisible()) ||
		(a.dividendDialog != nil && a.dividendDialog.IsVisible()) ||
		(a.transferCashDialog != nil && a.transferCashDialog.IsVisible()) ||
		(a.transferSharesDialog != nil && a.transferSharesDialog.IsVisible()) ||
		(a.stockSplitDialog != nil && a.stockSplitDialog.IsVisible()) ||
		(a.mergerDialog != nil && a.mergerDialog.IsVisible()) ||
		(a.spinOffDialog != nil && a.spinOffDialog.IsVisible()) ||
		(a.cashOperationDialog != nil && a.cashOperationDialog.IsVisible()) ||
		(a.investmentTypeSelector != nil && a.investmentTypeSelector.IsVisible()) ||
		a.mergerConfirmData != nil ||
		a.corporateActionHistory != nil ||
		a.showHelp
}

// handleMenuKeys handles keyboard input when the menu bar is active.
func (a *App) handleMenuKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		action, data := a.menubar.Select()
		return a.handleMenuAction(action, data)

	case key.Matches(msg, a.keys.Quit):
		a.quitting = true
		return a, tea.Quit
	}

	return a, nil
}

// handleMenuAction processes a menu item selection. The data string
// carries action-specific context populated from menuItem.data — for
// example, MenuActionLoadTheme uses it to carry the theme ID. It is
// the empty string for actions that don't need a payload.
func (a *App) handleMenuAction(action MenuAction, data string) (tea.Model, tea.Cmd) {
	switch action {
	case MenuActionNewFile:
		a.menubar.Deactivate()
		a.fileDialogMode = fileDialogModeNew
		a.fileDialog = buildNewFileDialog()
		return a, nil

	case MenuActionOpenFile:
		a.menubar.Deactivate()
		a.openBrowseDialog(db.DefaultDirectory())
		return a, nil

	case MenuActionOpenRecent:
		a.menubar.Deactivate()
		a.fileDialogMode = fileDialogModeOpenRecent
		var recent []string
		if a.cfg != nil {
			recent = a.cfg.RecentFiles
		}
		a.fileDialog = buildOpenRecentDialog(recent)
		return a, nil

	case MenuActionImportTransactions:
		a.menubar.Deactivate()
		return a, a.startImport()

	case MenuActionCreateBackup:
		a.menubar.Deactivate()
		return a, a.createManualBackupCmd()

	case MenuActionRestoreBackup:
		a.menubar.Deactivate()
		d, backups, err := buildRestoreBackupDialog(a.db.Path())
		if err != nil {
			a.err = err
			return a, nil
		}
		a.backupDialog = &backupDialogState{dialog: d, backups: backups}
		return a, nil

	case MenuActionCloseFile:
		a.quitting = true
		return a, tea.Quit

	case MenuActionExit:
		a.quitting = true
		return a, tea.Quit

	case MenuActionDashboard:
		a.switchView(ViewDashboard)

	case MenuActionNetWorth:
		a.switchView(ViewReports)
		now := time.Now()
		return a, a.loadReportsViewData(reportTypeNetWorth, now.Year(), int(now.Month()))

	case MenuActionSpendingByCategory:
		a.switchView(ViewReports)
		now := time.Now()
		return a, a.loadReportsViewData(reportTypeSpending, now.Year(), int(now.Month()))

	case MenuActionSecurities:
		a.switchView(ViewSecurities)
		return a, a.loadSecurityViewData()

	case MenuActionPrices:
		a.switchView(ViewPrices)
		return a, a.loadPriceViewData()

	case MenuActionNewAccount:
		return a, a.loadNewAccountDialogData()

	case MenuActionEditAccount:
		if a.sidebar.SelectedAccountID() != types.NilID {
			return a, a.loadEditAccountDialogData()
		}

	case MenuActionCloseAccount:
		if a.sidebar.SelectedAccountID() != types.NilID {
			return a, a.closeSelectedAccount()
		}

	case MenuActionDeleteAccount:
		if a.sidebar.SelectedAccountID() != types.NilID {
			return a, a.deleteSelectedAccount()
		}

	case MenuActionReconcileAccount:
		a.menubar.Deactivate()
		if a.sidebar.SelectedAccountID() != types.NilID {
			a.showStartReconciliationDialog()
		}
		return a, nil

	case MenuActionNewTransaction:
		if a.currentView == ViewRegister {
			return a, a.loadTransactionDialogData()
		}

	case MenuActionNewTransfer:
		if a.currentView == ViewRegister {
			return a, a.loadTransferDialogData()
		}

	case MenuActionLinkTransfers:
		a.menubar.Deactivate()
		return a, a.startLinkTransfers()

	case MenuActionUndo:
		a.menubar.Deactivate()
		return a, a.performUndo()

	case MenuActionRedo:
		a.menubar.Deactivate()
		return a, a.performRedo()

	case MenuActionKeyboardShortcuts:
		a.menubar.Deactivate()
		a.showHelp = true
		return a, nil

	case MenuActionAbout:
		a.menubar.Deactivate()
		a.showAboutDialog()
		return a, nil

	case MenuActionLoadTheme:
		a.menubar.Deactivate()
		if data == "" {
			return a, nil
		}
		return a, a.reloadTheme(data)

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
			case ViewScheduled:
				// Start with scheduled table focused
				a.sidebar.SetFocused(false)
				if a.scheduledTable != nil {
					a.scheduledTable.SetFocused(true)
				}
			case ViewDashboard:
				// Dashboard uses sidebar navigation
				a.sidebar.SetFocused(true)
				if a.table != nil {
					a.table.SetFocused(false)
				}
			case ViewReports:
				// Reports view doesn't use sidebar focus
				a.sidebar.SetFocused(false)
				if a.table != nil {
					a.table.SetFocused(false)
				}
			case ViewReconciliation:
				// Reconciliation is full-screen, no sidebar
				a.sidebar.SetFocused(false)
				if a.reconciliationTable != nil {
					a.reconciliationTable.SetFocused(true)
				}
			case ViewSecurities:
				// Securities is full-screen, no sidebar
				a.sidebar.SetFocused(false)
				if a.securityTable != nil {
					a.securityTable.SetFocused(true)
				}
			case ViewPrices:
				// Prices is full-screen, no sidebar
				a.sidebar.SetFocused(false)
				if a.priceTable != nil {
					a.priceTable.SetFocused(true)
				}
			case ViewInvestmentRegister:
				// Start with investment table focused
				a.sidebar.SetFocused(false)
				if a.investmentTable != nil {
					a.investmentTable.SetFocused(true)
				}
			case ViewPortfolio:
				// Start with portfolio table focused
				a.sidebar.SetFocused(false)
				a.setPortfolioTableFocused(true)
			}
		}
	}
}

// View implements tea.Model.
func (a *App) View() tea.View {
	v := tea.NewView(a.viewContent())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "TMoney - Personal Finance Manager"
	return v
}

func (a *App) viewContent() string {
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

	// Overlay scheduled dialog if visible
	if a.schedDialog != nil && a.schedDialog.IsVisible() {
		overlay := a.schedDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay account dialog if visible
	if a.acctDialog != nil && a.acctDialog.IsVisible() {
		overlay := a.acctDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay backup dialog if visible
	if a.backupDialog != nil && a.backupDialog.dialog.IsVisible() {
		overlay := a.backupDialog.dialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay file dialog if visible
	if a.fileDialog != nil && a.fileDialog.IsVisible() {
		overlay := a.fileDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay reconciliation start dialog if visible
	if a.reconDialog != nil && a.reconDialog.IsVisible() {
		overlay := a.reconDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay security dialog if visible
	if a.securityDialog != nil && a.securityDialog.IsVisible() {
		overlay := a.securityDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay price dialog if visible
	if a.priceDialog != nil && a.priceDialog.IsVisible() {
		overlay := a.priceDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay price import dialog if visible
	if a.priceImportDialog != nil && a.priceImportDialog.IsVisible() {
		overlay := a.priceImportDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay investment type selector dialog if visible
	if a.investmentTypeSelector != nil && a.investmentTypeSelector.IsVisible() {
		overlay := a.investmentTypeSelector.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay buy dialog if visible
	if a.buyDialog != nil && a.buyDialog.IsVisible() {
		overlay := a.buyDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay sell dialog if visible
	if a.sellDialog != nil && a.sellDialog.IsVisible() {
		overlay := a.sellDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay dividend dialog if visible
	if a.dividendDialog != nil && a.dividendDialog.IsVisible() {
		overlay := a.dividendDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay cash operation dialog if visible
	if a.cashOperationDialog != nil && a.cashOperationDialog.IsVisible() {
		overlay := a.cashOperationDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay transfer cash dialog if visible
	if a.transferCashDialog != nil && a.transferCashDialog.IsVisible() {
		overlay := a.transferCashDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay transfer shares dialog if visible
	if a.transferSharesDialog != nil && a.transferSharesDialog.IsVisible() {
		overlay := a.transferSharesDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay stock split dialog if visible
	if a.stockSplitDialog != nil && a.stockSplitDialog.IsVisible() {
		overlay := a.stockSplitDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay merger dialog if visible
	if a.mergerDialog != nil && a.mergerDialog.IsVisible() {
		overlay := a.mergerDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay merger confirmation if visible
	if a.mergerConfirmData != nil {
		overlay := a.renderMergerConfirmation()
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay spin-off dialog if visible
	if a.spinOffDialog != nil && a.spinOffDialog.IsVisible() {
		overlay := a.spinOffDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay corporate action history if visible
	if a.corporateActionHistory != nil {
		overlay := a.renderCorporateActionHistory()
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay confirmation dialog if visible
	if a.confirmDialog != nil && a.confirmDialog.IsVisible() {
		overlay := a.confirmDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay About dialog if visible
	if a.aboutDialog != nil && a.aboutDialog.IsVisible() {
		overlay := a.aboutDialog.Render(a.styles)
		layout = OverlayCenter(layout, overlay, a.width, a.height)
	}

	// Overlay help if visible
	if a.showHelp {
		overlay := renderHelpOverlay(a.styles, a.currentView, a.width, a.height)
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
	case ViewReconciliation:
		viewContent = a.renderReconciliation()
	case ViewSecurities:
		viewContent = a.renderSecurityView()
	case ViewPrices:
		viewContent = a.renderPriceView()
	case ViewInvestmentRegister:
		viewContent = a.renderInvestmentRegister()
	case ViewPortfolio:
		viewContent = a.renderPortfolioView()
	default:
		viewContent = "Unknown view"
	}

	// Reconciliation, Securities, and Prices views are full-screen (no sidebar)
	if a.currentView == ViewReconciliation || a.currentView == ViewSecurities || a.currentView == ViewPrices {
		return a.styles.RenderViewContent(viewContent, a.width, height)
	}

	sidebarWidth := a.styles.SidebarWidth()
	if sidebarWidth == 0 {
		// Small layout: no sidebar, full-width content
		return a.styles.RenderViewContent(viewContent, a.width, height)
	}

	// Two-pane layout: sidebar + content
	sidebar := a.sidebar.Render(a.styles, sidebarWidth, height)
	content := a.styles.RenderViewContent(viewContent, a.styles.ContentWidth(), height)

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
	padding := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(dateStr)-4, 1)
	titleRow := a.styles.Title.Render(titleText) + strings.Repeat(" ", padding) + a.styles.Muted.Render(dateStr)
	sections = append(sections, titleRow)

	// Separator
	sepWidth := max(contentWidth-4, 1)
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
	} else {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No account data available"))
	}

	// Scheduled transactions section
	sections = append(sections, a.renderDashboardScheduled())

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// maxDashboardHoldings is the maximum number of top holdings to display per investment account.
const maxDashboardHoldings = 5

// renderAssetLiabilityColumns renders the assets and liabilities side by side.
func (a *App) renderAssetLiabilityColumns(report *report.NetWorth, totalWidth int) string {
	colWidth := max(
		// Leave gap between columns
		(totalWidth-6)/2, 20)

	// Build assets column
	assetsLines := []string{a.styles.SectionHead.Render(padRight("ASSETS", colWidth))}
	if len(report.Assets) == 0 {
		assetsLines = append(assetsLines, a.styles.Muted.Render("  (none)"))
	} else {
		for _, acct := range report.Assets {
			name := truncate(acct.Name, colWidth-14)
			amount := formatDashboardMoney(acct.Balance)
			if acct.EstimatedValue {
				amount = "~" + amount
			}

			// Investment accounts get an expand/collapse indicator
			prefix := "  "
			if acct.Type == string(account.TypeInvestment) && a.dashboard != nil && a.dashboard.investmentHoldings != nil {
				if _, hasHoldings := a.dashboard.investmentHoldings[acct.AccountID]; hasHoldings {
					if a.dashboardExpandedAccounts[acct.AccountID] {
						prefix = "▾ "
					} else {
						prefix = "▸ "
					}
				}
			}

			line := fmt.Sprintf("%s%-*s %s", prefix, colWidth-len(amount)-lipgloss.Width(prefix)-2, name, a.styles.Positive.Render(amount))
			assetsLines = append(assetsLines, line)

			// Show top holdings if investment account is expanded
			if acct.Type == string(account.TypeInvestment) && a.dashboardExpandedAccounts[acct.AccountID] {
				assetsLines = append(assetsLines, a.renderDashboardHoldings(acct.AccountID, colWidth)...)
			}
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
			if acct.EstimatedValue {
				amount = "~" + amount
			}
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

// renderDashboardHoldings renders the top holdings for an investment account on the dashboard.
func (a *App) renderDashboardHoldings(accountID types.ID, colWidth int) []string {
	if a.dashboard == nil || a.dashboard.investmentHoldings == nil {
		return nil
	}

	val, ok := a.dashboard.investmentHoldings[accountID]
	if !ok {
		return nil
	}

	if len(val.Holdings) == 0 {
		return []string{a.styles.Muted.Render("    cash only")}
	}

	// Sort holdings by market value descending (they may already be sorted, but ensure)
	sorted := make([]investment.Holding, len(val.Holdings))
	copy(sorted, val.Holdings)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].MarketValue.Cmp(sorted[i].MarketValue) > 0 {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	var lines []string
	displayCount := min(len(sorted), maxDashboardHoldings)

	for _, h := range sorted[:displayCount] {
		ticker := "???"
		if a.dashboard.securityTickers != nil {
			if t, ok := a.dashboard.securityTickers[h.SecurityID]; ok {
				ticker = t
			}
		}
		ticker = truncate(ticker, colWidth-20)
		amount := formatDashboardMoney(h.MarketValue)
		if !h.HasPricing {
			amount = "~" + amount
		}
		line := fmt.Sprintf("    %-*s %s", colWidth-len(amount)-6, ticker, a.styles.Muted.Render(amount))
		lines = append(lines, line)
	}

	if remaining := len(sorted) - displayCount; remaining > 0 {
		lines = append(lines, a.styles.Muted.Render(fmt.Sprintf("    +%d more", remaining)))
	}

	return lines
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
	limit := min(len(upcoming), 5)
	for i := range limit {
		lines = append(lines, a.formatScheduledItem(upcoming[i], false))
	}
	if len(upcoming) > 5 {
		lines = append(lines, a.styles.Muted.Render(fmt.Sprintf("  ... and %d more", len(upcoming)-5)))
	}

	return strings.Join(lines, "\n")
}

// formatScheduledItem formats a single scheduled transaction line for the dashboard.
func (a *App) formatScheduledItem(st *scheduled.Transaction, isDue bool) string {
	// Payee name (cap at 20 chars to prevent overflow)
	payee := "Unknown"
	if st.HasPayee() {
		if name, ok := a.dashboard.payeeNames[st.PayeeID.ID]; ok {
			payee = name
		}
	}
	payee = truncate(payee, 20)

	// Amount
	var amount string
	if st.HasAmount() {
		amount = formatDashboardMoney(st.Amount.Money)
	} else {
		amount = "~variable"
	}

	// Due indicator
	if isDue {
		today := types.Today()
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
func formatDashboardMoney(m types.Money) string {
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

// truncateRunes truncates a string to maxLen runes, adding "..." if needed.
func truncateRunes(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
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

	// Apply void row styling
	for i, txn := range a.register.transactions {
		if txn.IsVoid() {
			a.table.SetRowStyle(i, RowStyleVoid)
		}
	}
}

// formatRegisterRow formats a transaction into table row strings.
func (a *App) formatRegisterRow(txn *transaction.Transaction) []string {
	// Date
	dateStr := txn.Date.Time().Format("01/02/06")

	// Status indicator
	status := " "
	switch txn.Status {
	case transaction.StatusCleared:
		status = "✓"
	case transaction.StatusReconciled:
		status = "R"
	case transaction.StatusVoid:
		status = "V"
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
	// Truncate account name if it would overflow available space
	maxNameWidth := max(
		// 4 padding + 2 gap
		contentWidth-lipgloss.Width(balStr)-6, 10)
	acctName = truncate(acctName, maxNameWidth)
	padding := max(contentWidth-lipgloss.Width(acctName)-lipgloss.Width(balStr)-4, 1)

	balStyle := a.styles.Positive
	if a.register.balance != nil && a.register.balance.CurrentBalance.IsNegative() {
		balStyle = a.styles.Negative
	}
	titleRow := a.styles.Title.Render(acctName) + strings.Repeat(" ", padding) + balStyle.Render(balStr)
	sections = append(sections, titleRow)

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	// Table
	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 2   // title + separator
	paddingHeight := 2 // top/bottom padding
	tableHeight := max(a.height-headerHeight-statusBarHeight-titleHeight-paddingHeight, 1)

	if a.table != nil {
		tableWidth := max(contentWidth-4, 1)
		sections = append(sections, a.table.Render(a.styles, tableWidth, tableHeight))
		if info := a.table.ScrollInfo(tableHeight - 2); info != "" {
			sections = append(sections, a.styles.Muted.Render("  "+info))
		}
	} else if len(a.register.transactions) == 0 {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No transactions"))
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  Press 'n' to add a new transaction"))
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// renderScheduled renders the scheduled transactions view.
func (a *App) renderScheduled() string {
	if a.scheduled == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading scheduled transactions...")
	}

	contentWidth := a.styles.ContentWidth()

	var sections []string

	// Title row: SCHEDULED + counts
	titleText := "SCHEDULED TRANSACTIONS"
	countText := ""
	if a.scheduled.dueCount > 0 {
		countText = fmt.Sprintf("%d due", a.scheduled.dueCount)
	}
	padding := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(countText)-4, 1)
	titleRow := a.styles.Title.Render(titleText)
	if countText != "" {
		titleRow += strings.Repeat(" ", padding) + a.styles.Alert.Render(countText)
	}
	sections = append(sections, titleRow)

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	if len(a.scheduled.allTxns) == 0 {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No scheduled transactions"))
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  Press 'n' to create a new scheduled transaction"))
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render(strings.Join(sections, "\n"))
	}

	// Table
	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 2   // title + separator
	paddingHeight := 2 // top/bottom padding
	tableHeight := max(a.height-headerHeight-statusBarHeight-titleHeight-paddingHeight, 1)

	if a.scheduledTable != nil {
		tableWidth := max(contentWidth-4, 1)
		sections = append(sections, a.scheduledTable.Render(a.styles, tableWidth, tableHeight))
		if info := a.scheduledTable.ScrollInfo(tableHeight - 2); info != "" {
			sections = append(sections, a.styles.Muted.Render("  "+info))
		}
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// buildScheduledTable creates and populates the table for the scheduled view.
func (a *App) buildScheduledTable() {
	if a.scheduled == nil {
		return
	}

	columns := []Column{
		{Header: " ", Width: 3, Align: AlignCenter},
		{Header: "Next Date", Width: 10, Align: AlignLeft},
		{Header: "Payee", MinWidth: 12, Align: AlignLeft},
		{Header: "Amount", Width: 12, Align: AlignRight},
		{Header: "Frequency", Width: 10, Align: AlignLeft},
		{Header: "Account", MinWidth: 10, Align: AlignLeft},
		{Header: "Auto", Width: 10, Align: AlignLeft},
	}

	if a.scheduledTable == nil {
		a.scheduledTable = NewTable(columns)
	} else {
		a.scheduledTable.SetColumns(columns)
	}

	rows := make([][]string, len(a.scheduled.allTxns))
	for i, st := range a.scheduled.allTxns {
		rows[i] = a.formatScheduledRow(st, i < a.scheduled.dueCount)
	}
	a.scheduledTable.SetRows(rows)
}

// formatScheduledRow formats a scheduled transaction into table row strings.
func (a *App) formatScheduledRow(st *scheduled.Transaction, isDue bool) []string {
	// Status indicator
	status := " ○"
	if isDue {
		today := types.Today()
		if st.NextDate.Equal(today) {
			status = " ●"
		} else {
			status = "!●"
		}
	}

	// Next date
	dateStr := st.NextDate.Time().Format("01/02/06")

	// Payee
	payee := ""
	if st.HasPayee() {
		if name, ok := a.scheduled.payeeNames[st.PayeeID.ID]; ok {
			payee = name
		}
	}

	// Amount
	amount := "~variable"
	if st.HasAmount() {
		amount = formatDashboardMoney(st.Amount.Money)
	}

	// Frequency
	freq := st.Frequency.DisplayName()

	// Account
	account := ""
	if name, ok := a.scheduled.accountNames[st.AccountID]; ok {
		account = name
	}

	// Auto-post indicator
	autoIndicator := ""
	if st.IsAutoPost() {
		switch st.PostLeadDays {
		case 3:
			autoIndicator = "[Auto 3d]"
		case 7:
			autoIndicator = "[Auto 7d]"
		default:
			autoIndicator = "[Auto]"
		}
	}

	return []string{status, dateStr, payee, amount, freq, account, autoIndicator}
}

// renderReports renders the reports view.
func (a *App) renderReports() string {
	if a.reports == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading reports...")
	}

	switch a.reports.rtype {
	case reportTypeNetWorth:
		return a.renderNetWorthReport()
	case reportTypeSpending:
		return a.renderSpendingReport()
	default:
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Unknown report type")
	}
}

// renderNetWorthReport renders the net worth report.
func (a *App) renderNetWorthReport() string {
	if a.reports.netWorth == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("No net worth data available. Add accounts to get started.")
	}

	contentWidth := a.styles.ContentWidth()
	nw := a.reports.netWorth

	var sections []string

	// Title row: NET WORTH REPORT + date
	dateStr := nw.AsOfDate.Format("Jan 2, 2006")
	titleText := "NET WORTH REPORT"
	padding := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(dateStr)-4, 1)
	titleRow := a.styles.Title.Render(titleText) + strings.Repeat(" ", padding) + a.styles.Muted.Render("As of: "+dateStr)
	sections = append(sections, titleRow)

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("═", sepWidth)))

	// Net worth summary
	nwLabel := "Net Worth:  "
	nwValue := formatDashboardMoney(nw.NetWorth)
	nwStyle := a.styles.Positive
	if nw.NetWorth.IsNegative() {
		nwStyle = a.styles.Negative
	}
	sections = append(sections, "")
	sections = append(sections, a.styles.Bold.Render(nwLabel)+nwStyle.Bold(true).Render(nwValue))
	sections = append(sections, "")

	// Assets and liabilities columns
	sections = append(sections, a.renderAssetLiabilityColumns(nw, contentWidth))

	// Navigation hints
	sections = append(sections, "")
	sections = append(sections, a.styles.Muted.Render("  n net worth  s spending  esc back"))

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// renderSpendingReport renders the spending by category report.
func (a *App) renderSpendingReport() string {
	if a.reports.spending == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("No spending data available. Add transactions to see reports.")
	}

	contentWidth := a.styles.ContentWidth()
	sr := a.reports.spending

	var sections []string

	// Title row
	titleText := "SPENDING BY CATEGORY"
	periodText := sr.Period
	padding := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(periodText)-4, 1)
	titleRow := a.styles.Title.Render(titleText) + strings.Repeat(" ", padding) + a.styles.Bold.Render(periodText)
	sections = append(sections, titleRow)

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("═", sepWidth)))

	if len(sr.Categories) == 0 {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No spending data for this period"))
	} else {
		// Column header
		tableWidth := max(contentWidth-4, 1)
		barWidth := max(
			// Reserve space for name(20), amount(12), percent(8), gaps(2)
			tableWidth-42, 4)

		headerLine := fmt.Sprintf("  %-20s %12s %7s  %s", "Category", "Amount", "% Total", "")
		sections = append(sections, a.styles.TableHeader.Render(headerLine))

		// Category rows
		for _, cat := range sr.Categories {
			// Parent category row with bar
			name := truncate(cat.Name, 20)
			amount := formatDashboardMoney(cat.Amount)
			pct := fmt.Sprintf("%.1f%%", cat.Percentage)
			bar := renderSpendingBar(cat.Percentage, barWidth)

			line := fmt.Sprintf("  %-20s %12s %7s  %s",
				a.styles.Bold.Render(name),
				a.styles.Negative.Render(amount),
				pct,
				a.styles.Negative.Render(bar))
			sections = append(sections, line)

			// Subcategory rows
			for _, sub := range cat.Subcategories {
				subName := "  " + truncate(sub.Name, 18)
				subAmount := formatDashboardMoney(sub.Amount)
				subLine := fmt.Sprintf("  %-20s %12s",
					a.styles.Muted.Render(subName),
					a.styles.Muted.Render(subAmount))
				sections = append(sections, subLine)
			}
		}

		// Total row
		sections = append(sections, a.styles.Muted.Render("  "+strings.Repeat("─", tableWidth-2)))
		totalAmount := formatDashboardMoney(sr.TotalSpending)
		totalLine := fmt.Sprintf("  %-20s %12s %7s",
			a.styles.Bold.Render("TOTAL"),
			a.styles.Negative.Bold(true).Render(totalAmount),
			"100.0%")
		sections = append(sections, totalLine)
	}

	// Period navigation
	sections = append(sections, "")
	prevPeriod, nextPeriod := a.getAdjacentPeriods()
	navLine := fmt.Sprintf("  %s  %s  %s",
		a.styles.Muted.Render(fmt.Sprintf("< %s", prevPeriod)),
		a.styles.Bold.Render(periodText),
		a.styles.Muted.Render(fmt.Sprintf("%s >", nextPeriod)))
	sections = append(sections, navLine)

	// Navigation hints
	modeHint := "m monthly"
	if a.reports.month > 0 {
		modeHint = "y yearly"
	}
	sections = append(sections, a.styles.Muted.Render(fmt.Sprintf("  <-> period  %s  n net worth  s spending  esc back", modeHint)))

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// getAdjacentPeriods returns display strings for the previous and next periods.
func (a *App) getAdjacentPeriods() (string, string) {
	if a.reports == nil {
		return "", ""
	}

	year := a.reports.year
	month := a.reports.month

	if month > 0 {
		// Monthly
		prevMonth := month - 1
		prevYear := year
		if prevMonth < 1 {
			prevMonth = 12
			prevYear--
		}
		nextMonth := month + 1
		nextYear := year
		if nextMonth > 12 {
			nextMonth = 1
			nextYear++
		}
		prev := time.Date(prevYear, time.Month(prevMonth), 1, 0, 0, 0, 0, time.UTC).Format("Jan 2006")
		next := time.Date(nextYear, time.Month(nextMonth), 1, 0, 0, 0, 0, time.UTC).Format("Jan 2006")
		return prev, next
	}

	// Yearly
	return fmt.Sprintf("%d", year-1), fmt.Sprintf("%d", year+1)
}

// renderSpendingBar renders a horizontal bar chart segment for spending percentage.
func renderSpendingBar(percentage float64, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	filled := max(min(int(math.Round(percentage/100.0*float64(maxWidth))), maxWidth), 0)
	return strings.Repeat("█", filled) + strings.Repeat("░", maxWidth-filled)
}

// renderStatusBar renders the status bar at the bottom.
func (a *App) renderStatusBar() string {
	return a.statusbar.Render(a.styles, a.width)
}

// getKeyHints returns key hints for the current view.
func (a *App) getKeyHints() string {
	common := "Alt+key/F10 menu  1 dashboard  2 scheduled  3 reports  4 securities  5 prices  ? help  ctrl+q quit"

	switch a.currentView {
	case ViewDashboard:
		return "↑↓ navigate  ←→ collapse/expand  enter select  " + common
	case ViewRegister:
		return "↑↓ navigate  enter edit  n new  t transfer  c clear  v void  d delete  esc back  " + common
	case ViewScheduled:
		return "↑↓ navigate  enter post  s skip  n new  e edit  d delete  esc back  " + common
	case ViewReports:
		return "←→ period  n net worth  s spending  y year  m month  esc back  " + common
	case ViewReconciliation:
		return "space toggle  enter finish  esc cancel  a check all  u uncheck all  ? help"
	case ViewSecurities:
		return "↑↓ navigate  n new  enter edit  h hide/unhide  d delete  f filter hidden  u update prices  a actions  / search  esc back  " + common
	case ViewPrices:
		if a.priceView != nil && a.priceView.mode == pricesViewDetail {
			return "↑↓ navigate  enter edit  n new  d delete  i import  / search  esc back  " + common
		}
		return "↑↓ navigate  enter view history  / search  esc back  " + common
	case ViewInvestmentRegister:
		return "↑↓ navigate  enter edit  n new  c clear  d delete  p portfolio  esc back  " + common
	case ViewPortfolio:
		return "↑↓ navigate  enter lot detail  r register  esc back  " + common
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

// mouseOpenAccountMsg is sent to defer a view switch from a mouse click
// to a separate Update cycle, avoiding Bubbletea renderer issues.
type mouseOpenAccountMsg struct {
	accountID types.ID
}

// sidebarLoadedMsg is sent when sidebar data has been loaded.
type sidebarLoadedMsg struct {
	accounts []*account.Account
	balances map[types.ID]*account.Balance
}

// scheduledDueCountMsg is sent when the count of due scheduled transactions is loaded.
type scheduledDueCountMsg struct {
	count int
}

// dashboardData holds the loaded data for the dashboard view.
type dashboardData struct {
	netWorth           *report.NetWorth
	dueTxns            []*scheduled.Transaction
	upcomingTxns       []*scheduled.Transaction
	payeeNames         map[types.ID]string
	accountNames       map[types.ID]string
	investmentHoldings map[types.ID]*investment.AccountValuation // account ID -> valuation with holdings
	securityTickers    map[types.ID]string                       // security ID -> ticker
}

// dashboardLoadedMsg is sent when dashboard data has been loaded.
type dashboardLoadedMsg struct {
	data *dashboardData
}

// registerData holds the loaded data for the account register view.
type registerData struct {
	account       *account.Account
	transactions  []*transaction.Transaction
	balance       *account.Balance
	payeeNames    map[types.ID]string
	categoryNames map[types.ID]string
	accountNames  map[types.ID]string
}

// registerLoadedMsg is sent when register data has been loaded.
type registerLoadedMsg struct {
	data *registerData
}

// scheduledViewData holds the loaded data for the scheduled transactions view.
type scheduledViewData struct {
	dueTxns       []*scheduled.Transaction
	upcomingTxns  []*scheduled.Transaction
	allTxns       []*scheduled.Transaction // combined: due first, then upcoming
	dueCount      int                      // number of due items (index boundary)
	payeeNames    map[types.ID]string
	accountNames  map[types.ID]string
	categoryNames map[types.ID]string
}

// scheduledViewDataLoadedMsg is sent when scheduled view data has been loaded.
type scheduledViewDataLoadedMsg struct {
	data *scheduledViewData
}

// scheduledPostedMsg is sent when a scheduled transaction has been posted.
type scheduledPostedMsg struct{}

// scheduledSkippedMsg is sent when a scheduled transaction has been skipped.
type scheduledSkippedMsg struct{}

// scheduledDeletedMsg is sent when a scheduled transaction has been deleted.
type scheduledDeletedMsg struct{}

// autoPostCompletedMsg is sent when auto-posting on file open completes.
type autoPostCompletedMsg struct {
	summary *scheduled.AutoPostSummary
}

// reportType represents which report is being displayed.
type reportType int

const (
	reportTypeNetWorth reportType = iota
	reportTypeSpending
)

// reportsViewData holds the loaded data for the reports view.
type reportsViewData struct {
	rtype    reportType
	netWorth *report.NetWorth
	spending *report.Spending
	year     int
	month    int // 1-12 for monthly, 0 for yearly
}

// reportsViewDataLoadedMsg is sent when reports data has been loaded.
type reportsViewDataLoadedMsg struct {
	data *reportsViewData
}

// overlayDropdown places a dropdown string on top of the layout at the given row and column offset.
func overlayDropdown(layout, dropdown string, colOffset, rowOffset, totalWidth int) string {
	_ = totalWidth // reserved for future right-edge clipping; kept to avoid touching call sites.
	layoutLines := strings.Split(layout, "\n")
	dropdownLines := strings.Split(dropdown, "\n")

	for i, dLine := range dropdownLines {
		targetRow := rowOffset + i
		if targetRow >= len(layoutLines) {
			break
		}
		layoutLines[targetRow] = spliceLine(layoutLines[targetRow], colOffset, dLine)
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

// reloadCurrentView returns a tea.Cmd that reloads data for the active view
// and the sidebar. Used after undo/redo to reflect changes.
func (a *App) reloadCurrentView() tea.Cmd {
	cmds := []tea.Cmd{a.loadSidebarData()}
	switch a.currentView {
	case ViewDashboard:
		cmds = append(cmds, a.loadDashboardData(), a.loadScheduledDueCount())
	case ViewRegister:
		accountID := a.sidebar.SelectedAccountID()
		cmds = append(cmds, a.loadRegisterData(accountID))
	case ViewScheduled:
		cmds = append(cmds, a.loadScheduledViewData(), a.loadScheduledDueCount())
	case ViewReports:
		if a.reports != nil {
			cmds = append(cmds, a.loadReportsViewData(
				a.reports.rtype, a.reports.year, a.reports.month,
			))
		}
	case ViewSecurities:
		cmds = append(cmds, a.loadSecurityViewData())
	}
	return tea.Batch(cmds...)
}

// undoResultMsg carries the result of an undo or redo operation.
type undoResultMsg struct {
	action      string // "Undo" or "Redo"
	description string
	err         error
}

// performUndo returns a tea.Cmd that undoes the last operation.
func (a *App) performUndo() tea.Cmd {
	if a.undoManager == nil {
		return nil
	}
	return func() tea.Msg {
		desc, err := a.undoManager.Undo()
		return undoResultMsg{action: "Undo", description: desc, err: err}
	}
}

// performRedo returns a tea.Cmd that redoes the last undone operation.
func (a *App) performRedo() tea.Cmd {
	if a.undoManager == nil {
		return nil
	}
	return func() tea.Msg {
		desc, err := a.undoManager.Redo()
		return undoResultMsg{action: "Redo", description: desc, err: err}
	}
}

// themeReloadFailedMsg is sent when reloadTheme cannot load or apply
// the requested theme — the active palette and cfg are left untouched.
// reloadTheme also surfaces the failure synchronously (toast + log) so
// downstream consumers can ignore this message; it stays for tests and
// any future handler that wants to react explicitly.
type themeReloadFailedMsg struct {
	id  string
	err error
}

// reloadTheme loads the theme with the given ID, applies it to
// a.styles, persists the ID into a.cfg, and returns a tea.Cmd that
// emits a tea.WindowSizeMsg matching the App's current dimensions so
// the next render reflects the new palette.
//
// LoadTheme (TH-026) consults the user theme directory first and
// falls back to the embedded built-ins, so user overrides take effect
// without any extra wiring here. On failure (unknown ID, parse error)
// the styles, palette, and config are left unchanged and the returned
// cmd emits a themeReloadFailedMsg.
//
// TH-032: parse issues encountered during a successful load and any
// failure-path error are appended to the applog and surfaced as a
// status-bar toast describing the issue count. The returned cmd is
// batched with a ClearToastCmd so the toast clears after ToastDuration.
// Successful loads with zero issues set no toast and return the bare
// WindowSizeMsg cmd.
func (a *App) reloadTheme(id string) tea.Cmd {
	t, issues, err := theme.LoadTheme(id)
	if err != nil {
		a.surfaceThemeFailure(id, err)
		return tea.Batch(
			func() tea.Msg { return themeReloadFailedMsg{id: id, err: err} },
			ClearToastCmd(),
		)
	}

	a.styles.applyTheme(t)
	a.styles.Resize(a.width, a.height)

	if a.cfg != nil {
		a.cfg.Theme = id
		// Save is best-effort: under `go test` it's a no-op, and
		// in production a write failure is non-fatal — the theme is
		// already live in memory.
		_ = a.cfg.Save()
	}

	width, height := a.width, a.height
	sizeCmd := func() tea.Msg {
		return tea.WindowSizeMsg{Width: width, Height: height}
	}

	if len(issues) == 0 {
		return sizeCmd
	}
	a.surfaceThemeIssues(id, issues)
	return tea.Batch(sizeCmd, ClearToastCmd())
}

// surfaceThemeIssues appends each parse issue to the applog file and
// sets a status-bar toast summarizing the count. Format mirrors the
// spec: "Theme '<id>': <N> issues, see <log path>".
func (a *App) surfaceThemeIssues(id string, issues []theme.Issue) {
	for _, iss := range issues {
		_ = applog.Append("theme", formatThemeIssue(id, iss))
	}
	if a.statusbar != nil {
		a.statusbar.SetToast(formatThemeToast(id, len(issues)), NotificationAlert)
	}
}

// surfaceThemeFailure logs an unparseable / missing-theme failure and
// sets a toast pointing the user at the log file.
func (a *App) surfaceThemeFailure(id string, err error) {
	_ = applog.Append("theme", fmt.Sprintf("%s: failed to load: %v", id, err))
	if a.statusbar != nil {
		text := fmt.Sprintf("Theme %q: failed to load", id)
		if path, perr := applog.LogPath(); perr == nil {
			text = fmt.Sprintf("%s, see %s", text, path)
		}
		a.statusbar.SetToast(text, NotificationAlert)
	}
}

// formatThemeIssue renders one parse issue as a single log line.
// Includes the theme ID, slot kind, key, the offending raw value (if
// any), and the parser's reason text.
func formatThemeIssue(id string, iss theme.Issue) string {
	if iss.Value != "" {
		return fmt.Sprintf("%s: %s %s=%q (%s)", id, iss.Kind, iss.Key, iss.Value, iss.Reason)
	}
	return fmt.Sprintf("%s: %s %s (%s)", id, iss.Kind, iss.Key, iss.Reason)
}

// formatThemeToast renders the user-facing toast text for a theme load
// that produced N parse issues. Includes the log path so users know
// where to look for details.
func formatThemeToast(id string, n int) string {
	noun := "issues"
	if n == 1 {
		noun = "issue"
	}
	base := fmt.Sprintf("Theme %q: %d %s", id, n, noun)
	if path, err := applog.LogPath(); err == nil {
		return fmt.Sprintf("%s, see %s", base, path)
	}
	return base
}

// Run starts the TUI application.
func Run(database *db.DB, cfg *config.Config) error {
	app := NewApp(database, cfg)
	p := tea.NewProgram(app)

	_, err := p.Run()

	// Close any database deferred during a database switch
	if app.prevDB != nil {
		_ = app.prevDB.Close()
	}

	// Auto-backup on quit (best-effort)
	createAutoBackupOnQuit(database.Path())

	return err
}
