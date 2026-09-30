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
			render:    (*App).renderDashboard,
			onKey:     (*App).handleDashboardKeys,
			hints:     func(*App) string { return "↑↓ navigate  ←→ collapse/expand  enter select  " + commonKeyHints },
			shortcuts: dashboardShortcuts,
			reload: func(a *App) []tea.Cmd {
				return []tea.Cmd{a.loadDashboardData(), a.loadScheduledDueCount()}
			},
			focus: func(a *App) {
				// Dashboard uses sidebar navigation
				a.sidebar.SetFocused(true)
				if a.table != nil {
					a.table.SetFocused(false)
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
			table:     func(a *App) *widget.Table { return a.table },
			reload: func(a *App) []tea.Cmd {
				accountID := a.sidebar.SelectedAccountID()
				return []tea.Cmd{a.loadRegisterData(accountID)}
			},
			focus: func(a *App) {
				// Start with table focused when entering register
				a.sidebar.SetFocused(false)
				if a.table != nil {
					a.table.SetFocused(true)
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
			table:     func(a *App) *widget.Table { return a.scheduledTable },
			reload: func(a *App) []tea.Cmd {
				return []tea.Cmd{a.loadScheduledViewData(), a.loadScheduledDueCount()}
			},
			focus: func(a *App) {
				// Start with scheduled table focused
				a.sidebar.SetFocused(false)
				if a.scheduledTable != nil {
					a.scheduledTable.SetFocused(true)
				}
			},
		},
		{
			id:     ViewReports,
			name:   "Reports",
			render: (*App).renderReports,
			onKey:  (*App).handleReportsKeys,
			hints: func(*App) string {
				return "←→ period  n net worth  s spending  y year  m month  esc back  " + commonKeyHints
			},
			shortcuts: reportsShortcuts,
			reload: func(a *App) []tea.Cmd {
				if a.reports != nil {
					return []tea.Cmd{a.loadReportsViewData(
						a.reports.rtype, a.reports.year, a.reports.month, a.reports.includeTransfers,
					)}
				}
				return nil
			},
			focus: func(a *App) {
				// Reports view doesn't use sidebar focus
				a.sidebar.SetFocused(false)
				if a.table != nil {
					a.table.SetFocused(false)
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
			table:      func(a *App) *widget.Table { return a.reconciliationTable },
			reload: func(a *App) []tea.Cmd {
				// No session on screen yet means its first load is still in flight;
				// that load fills the table. Never start a session from here.
				if r := a.reconciliation; r != nil && r.session != nil && r.account != nil {
					return []tea.Cmd{a.reloadReconciliationData(r)}
				}
				return nil
			},
			focus: func(a *App) {
				// Reconciliation is full-screen, no sidebar
				a.sidebar.SetFocused(false)
				if a.reconciliationTable != nil {
					a.reconciliationTable.SetFocused(true)
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
			table:     func(a *App) *widget.Table { return a.securityTable },
			reload:    func(a *App) []tea.Cmd { return []tea.Cmd{a.loadSecurityViewData()} },
			focus: func(a *App) {
				// Securities is full-screen, no sidebar
				a.sidebar.SetFocused(false)
				if a.securityTable != nil {
					a.securityTable.SetFocused(true)
				}
			},
		},
		{
			id:         ViewPrices,
			name:       "Prices",
			fullScreen: true,
			render:     (*App).renderPriceView,
			onKey:      (*App).handlePriceViewKeys,
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
			reload: func(a *App) []tea.Cmd { return []tea.Cmd{a.loadPriceViewData()} },
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
			render: (*App).renderInvestmentRegister,
			onKey:  (*App).handleInvestmentRegisterKeys,
			hints: func(*App) string {
				return "↑↓ navigate  enter edit  n new  c clear  d delete  p portfolio  esc back  " + commonKeyHints
			},
			shortcuts: investmentRegisterShortcuts,
			table:     func(a *App) *widget.Table { return a.investmentTable },
			reload: func(a *App) []tea.Cmd {
				if a.investmentRegister != nil && a.investmentRegister.account != nil {
					return []tea.Cmd{a.loadInvestmentRegisterData(a.investmentRegister.account.ID)}
				}
				return nil
			},
			focus: func(a *App) {
				// Start with investment table focused
				a.sidebar.SetFocused(false)
				if a.investmentTable != nil {
					a.investmentTable.SetFocused(true)
				}
			},
		},
		{
			id:        ViewPortfolio,
			name:      "Portfolio",
			render:    (*App).renderPortfolioView,
			onKey:     (*App).handlePortfolioKeys,
			hints:     func(*App) string { return "↑↓ navigate  enter lot detail  r register  esc back  " + commonKeyHints },
			shortcuts: portfolioShortcuts,
			table: func(a *App) *widget.Table {
				if a.portfolioData != nil {
					return a.activePortfolioTable()
				}
				return nil
			},
			reload: func(a *App) []tea.Cmd {
				if a.portfolioData != nil && a.portfolioData.account != nil {
					return []tea.Cmd{a.loadPortfolioData(a.portfolioData.account.ID)}
				}
				return nil
			},
			focus: func(a *App) {
				// Start with portfolio table focused
				a.sidebar.SetFocused(false)
				a.setPortfolioTableFocused(true)
			},
		},
		{
			id:         ViewCorporateActions,
			name:       "Corporate Actions",
			fullScreen: true,
			render:     (*App).renderCorporateActionView,
			onKey:      (*App).handleCorporateActionViewKeys,
			hints: func(*App) string {
				return "↑↓ navigate  / filter  enter details  d delete  esc back  " + commonKeyHints
			},
			shortcuts: corporateActionShortcuts,
			table:     func(a *App) *widget.Table { return a.corporateActionViewTable },
			reload: func(a *App) []tea.Cmd {
				return []tea.Cmd{a.loadCorporateActionViewData()}
			},
			focus: func(a *App) {
				// Corporate Actions is full-screen, no sidebar (the table is
				// (re)built and focused by buildCorporateActionTable).
				a.sidebar.SetFocused(false)
				if a.corporateActionViewTable != nil {
					a.corporateActionViewTable.SetFocused(true)
				}
			},
		},
		{
			id:         ViewAmortization,
			name:       "Amortization",
			fullScreen: true,
			render:     (*App).renderAmortizationView,
			onKey:      (*App).handleAmortizationKeys,
			hints:      func(*App) string { return "↑↓ navigate  g/G first/last  esc back  " + commonKeyHints },
			shortcuts:  amortizationShortcuts,
			table:      func(a *App) *widget.Table { return a.amortizationTable },
			reload: func(a *App) []tea.Cmd {
				if a.amortizationData != nil && a.amortizationData.account != nil {
					return []tea.Cmd{a.loadAmortizationData(a.amortizationData.account.ID)}
				}
				return nil
			},
			focus: func(a *App) {
				// Amortization is full-screen, no sidebar. The table is built
				// once its data loads (buildAmortizationTable focuses it then).
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
