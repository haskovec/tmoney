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
	"github.com/haskovec/tmoney/internal/transfer"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/theme"
	"github.com/haskovec/tmoney/internal/tui/widget"
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
	// ViewCorporateActions shows the global corporate-action register.
	ViewCorporateActions
	// ViewAmortization shows a loan account's live amortization projection.
	ViewAmortization
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
	case ViewCorporateActions:
		return "Corporate Actions"
	case ViewAmortization:
		return "Amortization"
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

	// widget.Styles
	styles widget.Styles

	// Components
	sidebar   *Sidebar
	menubar   *widget.MenuBar
	statusbar *widget.StatusBar

	// Services (initialized on start)
	accountSvc     *account.Service
	transactionSvc *transaction.Service
	// transferSvc owns whole-transaction cash transfers across both ledgers.
	// Reads and writes of transfers go through it rather than branching between
	// transactionSvc and investmentSvc on account type.
	transferSvc       *transfer.Service
	categorySvc       *category.Service
	payeeSvc          *payee.Service
	scheduledTxnSvc   *scheduled.Service
	reportSvc         *report.Service
	reconciliationSvc *reconciliation.Service

	// Dashboard data (loaded asynchronously)
	dashboard                 *dashboardData
	dashboardExpandedAccounts map[types.ID]bool // tracks expanded investment accounts on dashboard
	// dashboardAccountRows maps a content-pane row (0-based, as seen by a
	// mouse click's contentY) to the investment account whose expandable
	// ▸/▾ header renders on that row. Rebuilt every renderDashboard; used by
	// handleMouseDashboard to toggle expand/collapse on click.
	dashboardAccountRows map[int]types.ID

	// Register data (loaded when account is selected)
	register *registerData
	table    *widget.Table

	// Amortization view data (loan-account drill-in via 'a')
	amortizationData  *amortizationViewData
	amortizationTable *widget.Table

	// Transaction dialog state
	txnDialog            *dialog.Dialog
	txnDialogData        *transactionDialogData
	txnDialogCategoryIDs []types.ID
	// txnDialogLastSavedDate is the date of the last successfully-saved
	// operation in this process — covers the regular Transaction dialog,
	// every investment dialog (Buy/Sell/Dividend/Reinvest/Cash ops/
	// Transfer Cash/Transfer Shares), and the corporate-action dialogs
	// (Stock Split/Merger/Spin-Off). Each of those dialogs seeds its Date
	// field from this on the next open in new mode. Cancel does not update
	// it. Process-lifetime only — not persisted across restarts.
	txnDialogLastSavedDate types.Date

	// createCatDialog is the inline create-category sub-dialog opened from
	// any of the transaction-entry surfaces via the [+ Add new category…]
	// action row. While it is non-nil and visible, the originating dialog
	// is hidden but kept alive so its field state survives the divert. The
	// createCatSource field below records which surface opened it so the
	// post-create router can dispatch back to the right applier.
	createCatDialog *dialog.Dialog
	createCatSource createCategorySource
	// createCatSplitRow is the row index in splitDialog whose Category combo
	// activated [+ Add new category…]. Used by applyCreatedCategoryToSplit to
	// point the originating row at the freshly-created category after the
	// sub-dialog returns. -1 when no split-sourced sub-dialog is in flight.
	createCatSplitRow int
	// createCatPaycheckLine is the paycheck-wizard line whose select field
	// activated [+ Add new category…]. Used by applyCreatedCategoryToPaycheck
	// to point the originating line at the freshly-created category after the
	// sub-dialog returns. nil when no paycheck-sourced sub-dialog is in flight.
	createCatPaycheckLine *PaycheckLine
	// createCatLoanField is the loan-wizard dialog field index (interest or an
	// escrow category combo) that activated [+ Add new category…]. Used by
	// applyCreatedCategoryToLoan to point that field at the freshly-created
	// category after the sub-dialog returns. -1 when no loan-sourced sub-dialog
	// is in flight.
	createCatLoanField int

	// Split dialog state
	splitDialog           *SplitDialog
	pendingSplitTxn       *pendingSplitTransaction
	pendingSplitScheduled *pendingSplitScheduled

	// Transfer dialog state
	transferDialog            *dialog.Dialog
	transferDialogData        *transferDialogData
	transferDialogAccountIDs  []types.ID
	transferDialogCategoryIDs []types.ID // parallel to the Category combo options

	// Account dialog state
	acctDialog     *dialog.Dialog
	acctDialogData *accountDialogData

	// Scheduled dialog state
	schedDialog                *dialog.Dialog
	schedDialogData            *scheduledDialogData
	schedDialogAccountIDs      []types.ID
	schedDialogCategoryIDs     []types.ID
	schedDialogCategoryOptions []string

	// Scheduled preview dialog state. Opens when the user presses Enter
	// on a due scheduled item (replaces the legacy immediate-post path
	// per MS-019). MS-020 will land the save handler; for now the
	// dialog is open-and-cancel only.
	schedPreviewDialog *SchedulePreviewDialog

	// Paycheck wizard state. Opens from Transactions →
	// New Paycheck Schedule… (MS-026). MS-026 only lands the open path;
	// the save handler that builds a multi-line scheduled transaction
	// from the wizard's fields lands in MS-027/MS-028.
	paycheckWizard *PaycheckWizard

	// Loan wizard state. Opens from Accounts → New Loan…. Built on the
	// generic dialog.Dialog form widget (see loan_wizard.go); creates a loan
	// account, an optional asset account, and a monthly loan-shaped schedule
	// as one atomic, single-undo operation.
	loan *loanSurface

	// Scheduled view state
	scheduled      *scheduledViewData
	scheduledTable *widget.Table

	// Reports view state
	reports *reportsViewData

	// Reconciliation view state
	reconciliation      *reconciliationViewData
	reconciliationTable *widget.Table
	reconDialog         *dialog.Dialog
	// reconDialogLastStatementDate is the statement date used by the most
	// recent Start Reconciliation in this process. The Start Reconciliation
	// dialog seeds its Statement Date field from this on subsequent opens so
	// reconciling consecutive monthly statements is one Enter per month.
	// Cancel does not update it. Process-lifetime only — not persisted.
	reconDialogLastStatementDate types.Date

	// Close-account dialog state.
	closeAcct *closeAcctSurface

	// Security view state
	securityView  *securityViewData
	securityTable *widget.Table
	security      *securitySurface
	securitySvc   *security.Service

	// After adding a security, the table build step moves the cursor onto the
	// row whose security ID matches, so a freshly added ticker scrolls into
	// view rather than leaving the cursor wherever it was. Selecting by ID
	// (not position) lands on the row even though the list is sorted by ticker.
	// NilID means "no pending selection"; the build step clears it after use.
	pendingSecuritySelectID types.ID

	// Price view state
	priceView         *priceViewData
	priceTable        *widget.Table // detail-mode: history for one security
	priceListTable    *widget.Table // list-mode: latest price per ticker
	price             *priceSurface
	priceImportDialog *dialog.Dialog
	priceSvc          *price.Service

	// Bulk price refresh state. While refreshingPrices is true, the `u`
	// shortcut on the Securities and Prices views is a no-op so the user
	// can't fire a second concurrent refresh. refreshNotifID is the
	// status-bar notification ID for the "Updating prices…" entry,
	// removed when priceRefreshCompleteMsg arrives.
	refreshingPrices bool
	refreshNotifID   int

	// Investment register state
	investmentRegister *investmentRegisterData
	investmentTable    *widget.Table
	investmentSvc      *investment.Service
	// investmentValuationSvc is the read-only half: holdings, valuation and
	// total return. Views take it instead of the full service.
	investmentValuationSvc *investment.ValuationService
	// investmentEditSvc owns the ten edit entry points.
	investmentEditSvc      *investment.EditService
	investmentRepo         *investment.Repository
	investmentTypeSelector *dialog.Dialog
	investmentEditTxnID    types.ID // set when editing an existing transaction

	// Investment register security filter (the `/` key). While searching is
	// true the user is typing a substring query that live-narrows the register
	// by security ticker/name; pressing Enter on a query matching exactly one
	// security locks the filter (searching=false, query cleared) to
	// investmentFilterLockedSec. NilID means no security is locked; the filter
	// is active when either searching or a security is locked. Cleared when the
	// user leaves the register (see switchView) or presses Esc.
	investmentFilterSearching bool
	investmentFilterQuery     string
	investmentFilterLockedSec types.ID
	// One-shot: the security to pre-select in the next NEW security-bearing
	// investment dialog, seeded from the locked filter when `n` is pressed and
	// consumed (then reset) as each dialog is built. NilID means no preselect.
	investmentNewTxnSecurityID types.ID

	// After a save+reload, the register/investment-register build step moves
	// the cursor onto the row whose transaction ID matches. Selecting by ID
	// (rather than position) keeps the cursor on the saved row even when it
	// sorts into the middle of the list (e.g. a back-dated entry). NilID means
	// "no pending selection"; the build step clears the field after applying.
	pendingRegisterSelectID   types.ID
	pendingInvestmentSelectID types.ID

	// Buy dialog state
	buyDialog            *dialog.Dialog
	buyDialogData        *buyDialogData
	buyDialogSecurityIDs []types.ID

	// Sell dialog state
	sellDialog            *dialog.Dialog
	sellDialogData        *sellDialogData
	sellDialogSecurityIDs []types.ID
	sellDialogLots        []*investment.Lot

	// Fee via Liquidation dialog state
	feeLiquidationDialog            *dialog.Dialog
	feeLiquidationDialogData        *feeLiquidationDialogData
	feeLiquidationDialogSecurityIDs []types.ID

	// Dividend dialog state
	dividendDialog            *dialog.Dialog
	dividendDialogData        *dividendDialogData
	dividendDialogSecurityIDs []types.ID
	dividendDialogReinvest    bool // true when dialog is for reinvest dividend

	// Cash operation dialog state (deposit, withdrawal, fee, interest)
	cashOperationDialog *dialog.Dialog
	cashOperationType   investment.TransactionType

	// Transfer shares dialog state (between investment accounts)
	transferSharesDialog            *dialog.Dialog
	transferSharesDialogData        *transferSharesDialogData
	transferSharesDialogAccountIDs  []types.ID
	transferSharesDialogSecurityIDs []types.ID
	transferSharesDialogLots        []*investment.Lot

	// Portfolio view state
	portfolioData          *portfolioViewData
	portfolioHoldingsTable *widget.Table
	portfolioLotsTable     *widget.Table
	portfolioMode          portfolioViewMode

	// Corporate action service and stock split dialog state
	corporateActionSvc            *investment.CorporateActionService
	stockSplitDialog              *dialog.Dialog
	stockSplitDialogData          *stockSplitDialogData
	stockSplitDialogSecurityIDs   []types.ID
	stockSplitDialogPreSelectedID *types.ID

	// Merger dialog state
	mergerDialog              *dialog.Dialog
	mergerDialogData          *mergerDialogData
	mergerDialogSecurityIDs   []types.ID
	mergerDialogPreSelectedID *types.ID

	// Merger confirmation overlay state
	mergerConfirmData   *mergerConfirmData
	mergerConfirmParams *mergerConfirmParams

	// Spin-off dialog state
	spinOffDialog              *dialog.Dialog
	spinOffDialogData          *spinOffDialogData
	spinOffDialogSecurityIDs   []types.ID
	spinOffDialogPreSelectedID *types.ID

	// Corporate-action register state
	corporateActionView              *corporateActionViewData
	corporateActionViewTable         *widget.Table
	corporateActionViewFilter        string
	corporateActionViewFilterEditing bool
	corporateActionDetail            *investment.CorporateAction

	// Repositories for investment dialogs
	lotRepo      *investment.LotRepository
	positionRepo *investment.PositionRepository

	// File dialog state
	fileDialog     *dialog.Dialog
	fileDialogMode fileDialogMode
	browseDir      string

	// Import dialog state (transaction import via File → Import)
	importer *importSurface

	// Link Transfers dialog state (Transactions → Link Transfers)
	linkTransfers *linkTransfersSurface

	// Confirmation dialog state
	confirmDialog *dialog.Dialog
	confirmAction func() tea.Msg

	// About dialog (Help → About)
	aboutDialog *dialog.Dialog

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
	sidebarClicks      *widget.ClickTracker
	priceListClicks    *widget.ClickTracker
	browseDialogClicks *widget.ClickTracker
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
		styles:                    widget.NewStyles(),
		sidebar:                   NewSidebar(),
		menubar:                   widget.NewMenuBar(),
		statusbar:                 widget.NewStatusBar(),
		undoManager:               undo.NewManager(),
		keys:                      defaultKeyMap(),
		accountSvc:                svc.Account,
		transactionSvc:            svc.Transaction,
		transferSvc:               svc.Transfer,
		categorySvc:               svc.Category,
		payeeSvc:                  svc.Payee,
		scheduledTxnSvc:           svc.Scheduled,
		reportSvc:                 svc.Report,
		reconciliationSvc:         svc.Reconciliation,
		securitySvc:               svc.Security,
		priceSvc:                  svc.Price,
		investmentSvc:             svc.Investment,
		investmentValuationSvc:    svc.InvestmentValuation,
		investmentEditSvc:         svc.InvestmentEdit,
		investmentRepo:            svc.InvestmentRepo,
		dashboardExpandedAccounts: make(map[types.ID]bool),
		lotRepo:                   svc.LotRepo,
		positionRepo:              svc.PositionRepo,
		corporateActionSvc:        svc.CorporateAction,
		createCatSplitRow:         -1,
		createCatLoanField:        -1,
	}

	a.menubar.SetMenuItemsBuilder(widget.ViewMenuIndex, func() []widget.MenuItem {
		var active string
		var showClosed bool
		if a.cfg != nil {
			active = a.cfg.Theme
			showClosed = a.cfg.ShowClosedPositions
		}
		return widget.BuildViewMenuItems(active, showClosed)
	})

	// Apply the persisted theme (TH-029). On a clean load the styles
	// adopt the new palette and we're done. On parse issues the styles
	// adopt the partially-recovered theme and TH-032 surfaces the issue
	// list to the log file plus a toast on the status bar (the toast's
	// widget.ClearToastCmd is added to the Init batch below). On an outright
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
			a.styles.ApplyTheme(t)
			a.surfaceThemeIssues(cfg.Theme, issues)
		default:
			a.styles.ApplyTheme(t)
		}
	}

	// If a pre-existing user category blocked seeding the system Value
	// Adjustment category, warn the user once (log + toast). The toast's
	// auto-clear is scheduled in Init alongside the theme toast's.
	if svc.ValueAdjustmentUserCollision {
		a.surfaceValueAdjustmentCollision()
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
	// disappears after widget.ToastDuration like any other toast.
	if a.statusbar != nil && a.statusbar.Toast() != nil {
		cmds = append(cmds, widget.ClearToastCmd())
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

	// Route every key to the frontmost visible modal surface. The registry is
	// in priority order, so this replaces a 31-arm cascade that spelled that
	// order out by hand. See modal.go.
	if e := a.frontmostModal(); e != nil {
		return e.onKey(a, msg)
	}

	// While typing the investment register's security filter, capture every
	// key so global bindings (view-switch digits, Esc, Alt+menu) don't steal
	// keystrokes from the query.
	if a.currentView == ViewInvestmentRegister && a.investmentFilterSearching {
		return a.handleInvestmentRegisterKeys(msg)
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
			return a, a.loadReportsViewData(reportTypeNetWorth, now.Year(), int(now.Month()), false)
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
		// With the investment register filter active, Esc clears the filter
		// rather than navigating away; let the view handler claim it. (A
		// still-typing filter is already captured by the early guard above.)
		if a.currentView == ViewInvestmentRegister && a.investmentRegisterFilterActive() {
			return a.handleInvestmentRegisterKeys(msg)
		}
		// Go back to previous view or dashboard, and refresh that view's
		// data so changes made in the view we're leaving (e.g. a new
		// investment transaction) are reflected on arrival.
		if a.currentView != ViewDashboard {
			a.switchView(a.previousView)
			return a, a.reloadCurrentView()
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
	case ViewCorporateActions:
		return a.handleCorporateActionViewKeys(msg)
	case ViewAmortization:
		return a.handleAmortizationKeys(msg)
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

	// Auto-backup on quit (best-effort). Back up the database that is open NOW,
	// which File → Open may have swapped in. This closes app.db.
	createAutoBackupOnQuit(app.db)

	return err
}
