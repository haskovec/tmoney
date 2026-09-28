package tui

// viewEntry is one view and the glue App supplies for it. Every per-view fact
// the code needs lives here, so adding a view is adding one entry, and the
// guard in views_guard_test.go fails if the entry is missing.
type viewEntry struct {
	id   View
	name string // View.String(); shown in the status bar and test failures
}

// allViews is the one list of views. It holds only constants, so reading it
// allocates nothing and needs no App. Read it through views() and viewFor(),
// never by indexing with a View: View(999) must miss, not panic.
var allViews = []viewEntry{
	{id: ViewDashboard, name: "Dashboard"},
	{id: ViewRegister, name: "Register"},
	{id: ViewScheduled, name: "Scheduled"},
	{id: ViewReports, name: "Reports"},
	{id: ViewReconciliation, name: "Reconciliation"},
	{id: ViewSecurities, name: "Securities"},
	{id: ViewPrices, name: "Prices"},
	{id: ViewInvestmentRegister, name: "Investment Register"},
	{id: ViewPortfolio, name: "Portfolio"},
	{id: ViewCorporateActions, name: "Corporate Actions"},
	{id: ViewAmortization, name: "Amortization"},
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
