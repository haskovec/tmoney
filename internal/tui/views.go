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
}

// allViews is the one list of views. It holds only constants and method
// values, so reading it allocates nothing and needs no App. Read it through
// views() and viewFor(), never by indexing with a View: View(999) must miss,
// not panic.
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
		},
		{
			id:         ViewPrices,
			name:       "Prices",
			fullScreen: true,
			render:     (*App).renderPriceView,
			onKey:      (*App).handlePriceViewKeys,
			hints: func(a *App) string {
				if a.priceView != nil && a.priceView.mode == pricesViewDetail {
					return "↑↓ navigate  enter edit  n new  d delete  i import  / search  esc back  " + commonKeyHints
				}
				return "↑↓ navigate  enter view history  / search  esc back  " + commonKeyHints
			},
			shortcuts: pricesShortcuts,
			table: func(a *App) *widget.Table {
				if a.priceView != nil && a.priceView.mode == pricesViewList {
					return a.priceListTable
				}
				return a.priceTable
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
