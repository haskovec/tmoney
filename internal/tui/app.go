// Package tui provides the terminal user interface for TMoney.
package tui

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
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
	// txnDialogLastSavedDate is the date of the last successfully-saved
	// transaction in this process. The new-transaction dialog seeds its
	// Date field from this on subsequent opens. Cancel does not update it.
	// Process-lifetime only — not persisted across restarts.
	txnDialogLastSavedDate types.Date

	// createCatDialog is the inline create-category sub-dialog opened from
	// the transaction dialog's Category combo via the [+ Add new category…]
	// action row. While it is non-nil and visible, txnDialog is hidden but
	// kept alive so its field state survives the divert.
	createCatDialog *Dialog

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
		a.txnDialog = buildTransactionDialog(msg.data, categoryOptions, categoryIDs, a.txnDialogLastSavedDate)
		return a, nil

	case transactionDialogSavedMsg:
		if !msg.savedDate.IsZero() {
			a.txnDialogLastSavedDate = msg.savedDate
		}
		accountID := a.sidebar.SelectedAccountID()
		return a, tea.Batch(
			a.loadRegisterData(accountID),
			a.loadSidebarData(),
		)

	case createCategoryRequestMsg:
		if err := a.applyCreatedCategory(msg.request); err != nil {
			a.err = err
		}
		return a, nil

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

		if msg.data.mode == transferDialogModeEdit && msg.data.existing != nil {
			fromName, toName := transferAccountNames(msg.data)
			a.transferDialog = buildEditTransferDialog(fromName, toName, msg.data.existing)
			return a, nil
		}

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

	// If create-category sub-dialog is visible, route to it. Must come
	// before the transaction dialog check since the sub-dialog overlays it
	// while keeping the txnDialog instance alive (just hidden).
	if a.createCatDialog != nil && a.createCatDialog.IsVisible() {
		return a.handleCreateCatDialogKey(msg)
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

// errMsg is a message type for errors.
type errMsg struct {
	err error
}

// mouseOpenAccountMsg is sent to defer a view switch from a mouse click
// to a separate Update cycle, avoiding Bubbletea renderer issues.
type mouseOpenAccountMsg struct {
	accountID types.ID
}

// scheduledDueCountMsg is sent when the count of due scheduled transactions is loaded.
type scheduledDueCountMsg struct {
	count int
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
