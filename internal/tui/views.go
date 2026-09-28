package tui

import tea "charm.land/bubbletea/v2"

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
			id:     ViewDashboard,
			name:   "Dashboard",
			render: (*App).renderDashboard,
			onKey:  (*App).handleDashboardKeys,
		},
		{
			id:     ViewRegister,
			name:   "Register",
			render: (*App).renderRegister,
			onKey:  (*App).handleRegisterKeys,
		},
		{
			id:     ViewScheduled,
			name:   "Scheduled",
			render: (*App).renderScheduled,
			onKey:  (*App).handleScheduledKeys,
		},
		{
			id:     ViewReports,
			name:   "Reports",
			render: (*App).renderReports,
			onKey:  (*App).handleReportsKeys,
		},
		{
			id:         ViewReconciliation,
			name:       "Reconciliation",
			fullScreen: true,
			render:     (*App).renderReconciliation,
			onKey:      (*App).handleReconciliationKeys,
		},
		{
			id:         ViewSecurities,
			name:       "Securities",
			fullScreen: true,
			render:     (*App).renderSecurityView,
			onKey:      (*App).handleSecurityViewKeys,
		},
		{
			id:         ViewPrices,
			name:       "Prices",
			fullScreen: true,
			render:     (*App).renderPriceView,
			onKey:      (*App).handlePriceViewKeys,
		},
		{
			id:     ViewInvestmentRegister,
			name:   "Investment Register",
			render: (*App).renderInvestmentRegister,
			onKey:  (*App).handleInvestmentRegisterKeys,
		},
		{
			id:     ViewPortfolio,
			name:   "Portfolio",
			render: (*App).renderPortfolioView,
			onKey:  (*App).handlePortfolioKeys,
		},
		{
			id:         ViewCorporateActions,
			name:       "Corporate Actions",
			fullScreen: true,
			render:     (*App).renderCorporateActionView,
			onKey:      (*App).handleCorporateActionViewKeys,
		},
		{
			id:         ViewAmortization,
			name:       "Amortization",
			fullScreen: true,
			render:     (*App).renderAmortizationView,
			onKey:      (*App).handleAmortizationKeys,
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
