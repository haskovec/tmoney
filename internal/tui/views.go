package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/haskovec/tmoney/internal/tui/widget"
)

// viewEntry is one view and the glue App supplies for it. Every per-view fact
// the code needs lives here, so adding a view is adding one entry, and the
// guard in views_guard_test.go fails if the entry is missing.
type viewEntry struct {
	id   View
	name string // View.String(); shown in the status bar and test failures

	// fullScreen views paint without the sidebar and take the whole width.
	// Both renderContent and handleMouseContent read this one flag.
	fullScreen bool

	render func(*App) string
	onKey  func(*App, tea.KeyPressMsg) (tea.Model, tea.Cmd)
	hints  func(*App) string // the status-bar key hints

	shortcuts func() shortcutSection // the view's section in the help overlay

	// table is the widget the mouse and the wheel address in this view. A
	// func, not a field, because Prices and Portfolio pick between two tables
	// by mode. It is nil for a view with no table.
	table func(*App) *widget.Table

	// reload returns the commands that refresh this view in place, after
	// reloadCurrentView's sidebar reload. It returns none while the view has
	// nothing on screen to refresh yet. Every view has one.
	reload func(*App) []tea.Cmd

	// focus is what switchView does on arrival, when there is a sidebar:
	// which pane takes the cursor. Each guards the tables that are built only
	// when data arrives.
	focus func(*App)

	// leave is what switchView does on departure: the view state to forget
	// on the way out. It is nil for a view that keeps everything, and
	// switchView checks for that.
	leave func(*App)
}

// allViews is the one list of views. It holds only constants, method values,
// and funcs that capture nothing, so reading it allocates nothing and needs
// no App. Read it through views() and viewFor(), never by indexing with a
// View: View(999) must miss, not panic.
//
// init fills it, not the declaration: the funcs reach View.String(), which
// reads allViews, and Go refuses that cycle in a package-level initializer.
var allViews []viewEntry

func init() {
	allViews = []viewEntry{
		{
			id:        ViewDashboard,
			name:      "Dashboard",
			render:    func(a *App) string { return a.dashboard.render(a.styles) },
			onKey:     (*App).handleDashboardKeys,
			hints:     func(*App) string { return "↑↓ navigate  ←→ collapse/expand  enter select  " + commonKeyHints },
			shortcuts: dashboardShortcuts,
			reload: func(a *App) []tea.Cmd {
				return []tea.Cmd{a.dashboard.load(a.dashboardDeps()), a.loadScheduledDueCount()}
			},
			focus: func(a *App) {
				// Dashboard uses sidebar navigation
				a.sidebar.SetFocused(true)
				if a.register.table != nil {
					a.register.table.SetFocused(false)
				}
			},
		},
		{
			id:     ViewRegister,
			name:   "Register",
			render: (*App).renderRegister,
			onKey:  (*App).handleRegisterKeys,
			hints: func(*App) string {
				return "↑↓ navigate  enter edit  n new  t transfer  c clear  v void  r reconcile  d delete  esc back  " + commonKeyHints
			},
			shortcuts: registerShortcuts,
			table:     func(a *App) *widget.Table { return a.register.table },
			reload: func(a *App) []tea.Cmd {
				accountID := a.sidebar.SelectedAccountID()
				return []tea.Cmd{a.loadRegisterData(accountID)}
			},
			focus: func(a *App) {
				// Start with table focused when entering register
				a.sidebar.SetFocused(false)
				if a.register.table != nil {
					a.register.table.SetFocused(true)
				}
			},
		},
		{
			id:     ViewScheduled,
			name:   "Scheduled",
			render: (*App).renderScheduled,
			onKey:  (*App).handleScheduledKeys,
			hints: func(*App) string {
				return "↑↓ navigate  enter post  s skip  n new  t transfer  e edit  d delete  esc back  " + commonKeyHints
			},
			shortcuts: scheduledShortcuts,
			table:     func(a *App) *widget.Table { return a.scheduled.table },
			reload: func(a *App) []tea.Cmd {
				return []tea.Cmd{a.loadScheduledViewData(), a.loadScheduledDueCount()}
			},
			focus: func(a *App) {
				// Start with scheduled table focused
				a.sidebar.SetFocused(false)
				if a.scheduled.table != nil {
					a.scheduled.table.SetFocused(true)
				}
			},
		},
		{
			id:     ViewReports,
			name:   "Reports",
			render: func(a *App) string { return a.reports.render(a.styles) },
			onKey: func(a *App, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
				return a, a.reports.handleKey(a.reportsDeps(), msg, a.keys)
			},
			hints: func(*App) string {
				return "←→ period  n net worth  s spending  y year  m month  esc back  " + commonKeyHints
			},
			shortcuts: reportsShortcuts,
			reload: func(a *App) []tea.Cmd {
				if a.reports.data != nil {
					return []tea.Cmd{a.reports.load(a.reportsDeps(),
						a.reports.data.rtype, a.reports.data.year, a.reports.data.month, a.reports.data.includeTransfers,
					)}
				}
				return nil
			},
			focus: func(a *App) {
				// Reports view doesn't use sidebar focus
				a.sidebar.SetFocused(false)
				if a.register.table != nil {
					a.register.table.SetFocused(false)
				}
			},
		},
		{
			id:         ViewReconciliation,
			name:       "Reconciliation",
			fullScreen: true,
			render:     (*App).renderReconciliation,
			onKey:      (*App).handleReconciliationKeys,
			hints:      func(*App) string { return "space toggle  enter finish  esc cancel  a check all  u uncheck all  ? help" },
			shortcuts:  reconciliationShortcuts,
			table:      func(a *App) *widget.Table { return a.reconciliation.table },
			reload: func(a *App) []tea.Cmd {
				// No session on screen yet means its first load is still in flight;
				// that load fills the table. Never start a session from here.
				if r := a.reconciliation.data; r != nil && r.session != nil && r.account != nil {
					return []tea.Cmd{a.reloadReconciliationData(r)}
				}
				return nil
			},
			focus: func(a *App) {
				// Reconciliation is full-screen, no sidebar
				a.sidebar.SetFocused(false)
				if a.reconciliation.table != nil {
					a.reconciliation.table.SetFocused(true)
				}
			},
		},
		{
			id:         ViewSecurities,
			name:       "Securities",
			fullScreen: true,
			render:     (*App).renderSecurityView,
			onKey:      (*App).handleSecurityViewKeys,
			hints: func(*App) string {
				return "↑↓ navigate  n new  enter edit  h hide/unhide  d delete  f filter hidden  u update prices  a actions  / search  esc back  " + commonKeyHints
			},
			shortcuts: securitiesShortcuts,
			table:     func(a *App) *widget.Table { return a.securities.table },
			reload:    func(a *App) []tea.Cmd { return []tea.Cmd{a.loadSecurityViewData()} },
			focus: func(a *App) {
				// Securities is full-screen, no sidebar
				a.sidebar.SetFocused(false)
				if a.securities.table != nil {
					a.securities.table.SetFocused(true)
				}
			},
		},
		{
			id:         ViewPrices,
			name:       "Prices",
			fullScreen: true,
			render: func(a *App) string {
				return a.prices.render(a.styles, a.width, a.height)
			},
			onKey: (*App).handlePriceViewKeys,
			hints: func(a *App) string {
				if a.prices.data != nil && a.prices.data.mode == pricesViewDetail {
					return "↑↓ navigate  enter edit  n new  d delete  i import  / search  esc back  " + commonKeyHints
				}
				return "↑↓ navigate  enter view history  / search  esc back  " + commonKeyHints
			},
			shortcuts: pricesShortcuts,
			table: func(a *App) *widget.Table {
				if a.prices.data != nil && a.prices.data.mode == pricesViewList {
					return a.prices.listTable
				}
				return a.prices.table
			},
			reload: func(a *App) []tea.Cmd { return []tea.Cmd{a.prices.load(a.priceDeps())} },
			focus: func(a *App) {
				// Prices is full-screen, no sidebar
				a.sidebar.SetFocused(false)
				if a.prices.table != nil {
					a.prices.table.SetFocused(true)
				}
			},
		},
		{
			id:     ViewInvestmentRegister,
			name:   "Investment Register",
			render: func(a *App) string { return a.investmentRegister.render(a.styles, a.height) },
			onKey:  (*App).handleInvestmentRegisterKeys,
			hints: func(*App) string {
				return "↑↓ navigate  enter edit  n new  c clear  d delete  p portfolio  esc back  " + commonKeyHints
			},
			shortcuts: investmentRegisterShortcuts,
			table:     func(a *App) *widget.Table { return a.investmentRegister.table },
			reload: func(a *App) []tea.Cmd {
				if a.investmentRegister.data != nil && a.investmentRegister.data.account != nil {
					return []tea.Cmd{a.investmentRegister.load(a.investmentRegisterDeps(), a.investmentRegister.data.account.ID)}
				}
				return nil
			},
			focus: func(a *App) {
				// Start with investment table focused
				a.sidebar.SetFocused(false)
				if a.investmentRegister.table != nil {
					a.investmentRegister.table.SetFocused(true)
				}
			},
			// Leaving the investment register drops its (transient) security
			// filter, so reopening the account later shows the full register.
			leave: func(a *App) { a.investmentRegister.resetFilter() },
		},
		{
			id:   ViewPortfolio,
			name: "Portfolio",
			render: func(a *App) string {
				return a.portfolio.render(a.styles, a.height)
			},
			onKey:     (*App).handlePortfolioKeys,
			hints:     func(*App) string { return "↑↓ navigate  enter lot detail  r register  esc back  " + commonKeyHints },
			shortcuts: portfolioShortcuts,
			table: func(a *App) *widget.Table {
				if a.portfolio.data != nil {
					return a.portfolio.activeTable()
				}
				return nil
			},
			reload: func(a *App) []tea.Cmd {
				if a.portfolio.data != nil && a.portfolio.data.account != nil {
					return []tea.Cmd{a.portfolio.load(a.portfolioDeps(), a.portfolio.data.account.ID)}
				}
				return nil
			},
			focus: func(a *App) {
				// Start with portfolio table focused
				a.sidebar.SetFocused(false)
				a.portfolio.setTableFocused(true)
			},
		},
		{
			id:         ViewCorporateActions,
			name:       "Corporate Actions",
			fullScreen: true,
			render: func(a *App) string {
				return a.corporateActions.render(a.styles, a.height)
			},
			onKey: (*App).handleCorporateActionViewKeys,
			hints: func(*App) string {
				return "↑↓ navigate  / filter  enter details  d delete  esc back  " + commonKeyHints
			},
			shortcuts: corporateActionShortcuts,
			table:     func(a *App) *widget.Table { return a.corporateActions.table },
			reload: func(a *App) []tea.Cmd {
				return []tea.Cmd{a.corporateActions.load(a.corporateActionDeps())}
			},
			focus: func(a *App) {
				// Corporate Actions is full-screen, no sidebar (the table is
				// (re)built and focused by buildCorporateActionTable).
				a.sidebar.SetFocused(false)
				if a.corporateActions.table != nil {
					a.corporateActions.table.SetFocused(true)
				}
			},
			leave: func(a *App) {
				// The details panel goes. Left set, isDialogVisible stays
				// true on the next view, which routes every click into the
				// dialog cascade — a dead mouse with no modal on screen.
				a.corporateActions.detail = nil
				// A filter entry ends with the view, or the view would
				// return still capturing every key as filter text. The
				// filter itself is kept: a drill-in from Securities set it.
				a.corporateActions.filterEditing = false
			},
		},
		{
			id:         ViewAmortization,
			name:       "Amortization",
			fullScreen: true,
			render: func(a *App) string {
				return a.amortization.render(a.styles, a.width, a.height)
			},
			onKey: func(a *App, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
				a.amortization.handleKey(msg, a.keys, a.height)
				return a, nil
			},
			hints:     func(*App) string { return "↑↓ navigate  g/G first/last  esc back  " + commonKeyHints },
			shortcuts: amortizationShortcuts,
			table:     func(a *App) *widget.Table { return a.amortization.table },
			reload: func(a *App) []tea.Cmd {
				if a.amortization.data != nil && a.amortization.data.account != nil {
					return []tea.Cmd{a.amortization.load(a.amortizationDeps(), a.amortization.data.account.ID)}
				}
				return nil
			},
			focus: func(a *App) {
				// Amortization is full-screen, no sidebar. The table is built
				// once its data loads (buildTable focuses it then).
				a.sidebar.SetFocused(false)
			},
		},
	}
}

// views returns every view entry, in View order.
func views() []viewEntry { return allViews }

// viewFor returns the entry for v, or false when v is not a view.
func viewFor(v View) (viewEntry, bool) {
	for _, e := range allViews {
		if e.id == v {
			return e, true
		}
	}
	return viewEntry{}, false
}
