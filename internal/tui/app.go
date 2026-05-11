// Package tui provides the terminal user interface for TMoney.
package tui

import (
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
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
	// transaction in this process — covers the regular Transaction dialog
	// and every investment dialog (Buy/Sell/Dividend/Reinvest/Cash ops/
	// Transfer Cash/Transfer Shares). Each of those dialogs seeds its Date
	// field from this on the next open in new mode. Cancel does not update
	// it. Process-lifetime only — not persisted across restarts.
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
	// reconDialogLastStatementDate is the statement date used by the most
	// recent Start Reconciliation in this process. The Start Reconciliation
	// dialog seeds its Statement Date field from this on subsequent opens so
	// reconciling consecutive monthly statements is one Enter per month.
	// Cancel does not update it. Process-lifetime only — not persisted.
	reconDialogLastStatementDate types.Date

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

	// Bulk price refresh state. While refreshingPrices is true, the `u`
	// shortcut on the Securities and Prices views is a no-op so the user
	// can't fire a second concurrent refresh. refreshNotifID is the
	// status-bar notification ID for the "Updating prices…" entry,
	// removed when priceRefreshCompleteMsg arrives.
	refreshingPrices bool
	refreshNotifID   int

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
		return a, a.loadDashboardData()

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

// errMsg is a message type for errors.
type errMsg struct {
	err error
}

// mouseOpenAccountMsg is sent to defer a view switch from a mouse click
// to a separate Update cycle, avoiding Bubbletea renderer issues.
type mouseOpenAccountMsg struct {
	accountID types.ID
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
