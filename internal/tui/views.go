package tui

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
}

// allViews is the one list of views. It holds only constants, so reading it
// allocates nothing and needs no App. Read it through views() and viewFor(),
// never by indexing with a View: View(999) must miss, not panic.
var allViews = []viewEntry{
	{
		id:     ViewDashboard,
		name:   "Dashboard",
		render: (*App).renderDashboard,
	},
	{
		id:     ViewRegister,
		name:   "Register",
		render: (*App).renderRegister,
	},
	{
		id:     ViewScheduled,
		name:   "Scheduled",
		render: (*App).renderScheduled,
	},
	{
		id:     ViewReports,
		name:   "Reports",
		render: (*App).renderReports,
	},
	{
		id:         ViewReconciliation,
		name:       "Reconciliation",
		fullScreen: true,
		render:     (*App).renderReconciliation,
	},
	{
		id:         ViewSecurities,
		name:       "Securities",
		fullScreen: true,
		render:     (*App).renderSecurityView,
	},
	{
		id:         ViewPrices,
		name:       "Prices",
		fullScreen: true,
		render:     (*App).renderPriceView,
	},
	{
		id:     ViewInvestmentRegister,
		name:   "Investment Register",
		render: (*App).renderInvestmentRegister,
	},
	{
		id:     ViewPortfolio,
		name:   "Portfolio",
		render: (*App).renderPortfolioView,
	},
	{
		id:         ViewCorporateActions,
		name:       "Corporate Actions",
		fullScreen: true,
		render:     (*App).renderCorporateActionView,
	},
	{
		id:         ViewAmortization,
		name:       "Amortization",
		fullScreen: true,
		render:     (*App).renderAmortizationView,
	},
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
