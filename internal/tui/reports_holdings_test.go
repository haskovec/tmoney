package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/tui/sidebar"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// holdingsTestReport is a one-currency report: ACME in two accounts, the
// Cash row, and STRK with no price.
func holdingsTestReport() *report.Holdings {
	m := types.MustNewMoney
	q := types.MustNewQuantity
	return &report.Holdings{
		AsOfDate: types.NewDate(2024, 1, 15),
		Sections: []report.HoldingsSection{{
			Currency: "USD", Available: true, Estimated: true,
			Value: m("6000.00"), CostBasis: m("5300.00"), Gain: m("800.00"),
			Rows: []report.HoldingRow{
				{SecurityID: types.NewID(), Label: "ACME", Name: "Acme Total Market Index",
					Shares: q("40"), Price: m("120.00"), Value: m("4800.00"), CostBasis: m("4000.00"),
					Gain: m("800.00"), Percent: 80,
					Accounts: []report.HoldingAccount{
						{AccountID: types.NewID(), Name: "Maple Invest Brokerage", Shares: q("30"), Value: m("3600.00"), Percent: 75},
						{AccountID: types.NewID(), Name: "Cedar HSA Investment", Shares: q("10"), Value: m("1200.00"), Percent: 25},
					}},
				{SecurityID: types.NilID, Cash: true, Label: "Cash", Name: "Uninvested cash",
					Value: m("700.00"), CostBasis: m("700.00"), Percent: 11.7,
					Accounts: []report.HoldingAccount{
						{AccountID: types.NewID(), Name: "Maple Invest Brokerage", Value: m("700.00"), Percent: 100},
					}},
				{SecurityID: types.NewID(), Label: "STRK", Name: "Stark Industries",
					Shares: q("5"), Value: m("500.00"), CostBasis: m("500.00"), Percent: 8.3, Estimated: true,
					Accounts: []report.HoldingAccount{
						{AccountID: types.NewID(), Name: "Maple Invest Roth IRA", Shares: q("5"), Value: m("500.00"), Percent: 100},
					}},
			},
		}},
	}
}

// newHoldingsApp is an App on the Reports view with the holdings report
// loaded, at the given terminal size.
func newHoldingsApp(width, height int) *App {
	app := &App{
		currentView:  ViewReports,
		previousView: ViewDashboard,
		keys:         defaultKeyMap(),
		menubar:      widget.NewMenuBar(),
		statusbar:    widget.NewStatusBar(),
		sidebar:      sidebar.New(),
		styles:       widget.NewStyles(),
		width:        width,
		height:       height,
	}
	app.styles.Resize(width, height)
	app.sidebar.SetFocused(false)
	app.reports.setData(&reportsViewData{rtype: reportTypeHoldings, holdings: holdingsTestReport()})
	return app
}

func holdingsHeaders(cols []holdingsColumn) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.Header
	}
	return out
}

// The columns drop in the agreed order as the table narrows, and Name and the
// bar share what is left.
func TestHoldingsColumns_DropOrder(t *testing.T) {
	tests := []struct {
		width int
		want  string
	}{
		{109, "Security Name Shares Price Value % Total Cost Basis Gain "},
		{108, "Security Name Value % Total Cost Basis Gain "},
		{85, "Security Name Value % Total Cost Basis Gain "},
		{84, "Security Name Value % Total "},
		{58, "Security Name Value % Total "},
		{57, "Security Value % Total "},
	}
	for _, tt := range tests {
		cols := holdingsColumns(tt.width, 26)
		if got := strings.Join(holdingsHeaders(cols), " "); got != tt.want {
			t.Errorf("width %d: columns %q, want %q", tt.width, got, tt.want)
		}
		total := len(cols) - 1
		for _, c := range cols {
			total += c.Width
		}
		if total > tt.width {
			t.Errorf("width %d: the columns take %d cells", tt.width, total)
		}
	}
}

func TestHoldingsColumns_NameAndBarShareTheSpare(t *testing.T) {
	width := func(cols []holdingsColumn, col holdingsCol) int {
		for _, c := range cols {
			if c.col == col {
				return c.Width
			}
		}
		return -1
	}
	// At each drop threshold the two get their minimums.
	for _, w := range []int{109, 85, 58} {
		cols := holdingsColumns(w, 26)
		if n, b := width(cols, holdingsColName), width(cols, holdingsColBar); n != 16 || b != 10 {
			t.Errorf("width %d: name %d, bar %d; want 16 and 10", w, n, b)
		}
	}
	// With room, Name stops at the longest name and the bar at 40.
	cols := holdingsColumns(200, 26)
	if n, b := width(cols, holdingsColName), width(cols, holdingsColBar); n != 26 || b != 40 {
		t.Errorf("width 200: name %d, bar %d; want 26 and 40", n, b)
	}
	// Short names never go below the minimum.
	if n := width(holdingsColumns(150, 4), holdingsColName); n != 16 {
		t.Errorf("short names: name %d, want 16", n)
	}
	// With no Name column, the bar takes the spare.
	if b := width(holdingsColumns(50, 26), holdingsColBar); b != 50-3-8-13-7 {
		t.Errorf("width 50: bar %d, want %d", b, 50-3-8-13-7)
	}
}

func TestRenderHoldings(t *testing.T) {
	app := newHoldingsApp(180, 40)
	view := app.reports.render(app.styles, app.height)
	for _, want := range []string{
		"HOLDINGS", "As of: Jan 15, 2024",
		"Security", "% Total", "Cost Basis",
		"ACME", "Acme Total Market Index", "$120.00", "$4800.00", "80.0%",
		"Cash", "Uninvested cash",
		"~STRK", "N/A",
		"TOTAL", "~$6000.00", "100.0%", "$800.00",
		"~ No price on file: the value is the cost basis.",
		"i holdings",
		strings.Repeat("█", 10),
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	for line := range strings.Lines(view) {
		if w := lipgloss.Width(strings.TrimRight(line, "\n")); w > app.styles.ContentWidth() {
			t.Errorf("line is %d cells, wider than the %d of the content: %q", w, app.styles.ContentWidth(), line)
		}
	}
	if lines := strings.Count(view, "\n") + 1; lines > app.height-appChromeRows {
		t.Errorf("view is %d lines, more than the %d the screen has", lines, app.height-appChromeRows)
	}
}

// On a short screen only the rows scroll: the title, the TOTAL and the notes
// stay, and the view still fits.
func TestRenderHoldings_ShortScreenScrolls(t *testing.T) {
	app := newHoldingsApp(180, 15)
	view := app.reports.render(app.styles, app.height)
	for _, want := range []string{"HOLDINGS", "TOTAL", "~ No price on file", "of 3"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if lines := strings.Count(view, "\n") + 1; lines > app.height-appChromeRows {
		t.Errorf("view is %d lines, more than the %d the screen has:\n%s", lines, app.height-appChromeRows, view)
	}
}

func TestRenderHoldings_FailedAccountAndEmpty(t *testing.T) {
	app := newHoldingsApp(180, 40)
	rpt := holdingsTestReport()
	rpt.Sections[0].Available = false
	rpt.Failed = []report.AccountFailure{{Name: "Birch 401k", Currency: "USD", Err: errString("price lookup failed")}}
	app.reports.setData(&reportsViewData{rtype: reportTypeHoldings, holdings: rpt})
	view := app.reports.render(app.styles, app.height)
	for _, want := range []string{
		"not available",
		"Birch 401k: could not be valued (price lookup failed)",
		"Percentages leave out accounts that could not be valued.",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}

	app.reports.setData(&reportsViewData{rtype: reportTypeHoldings, holdings: &report.Holdings{}})
	if view := app.reports.render(app.styles, app.height); !strings.Contains(view, "No investment accounts. Add one to see holdings.") {
		t.Errorf("empty view:\n%s", view)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestHoldings_KeyIFromAnotherReport(t *testing.T) {
	app := newHoldingsApp(180, 40)
	app.reports.setData(&reportsViewData{rtype: reportTypeNetWorth, year: 2024, month: 6})
	_, cmd := app.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	if cmd == nil {
		t.Fatal("pressing 'i' should return a command to load the holdings report")
	}
	msg, ok := cmd().(reportsViewDataLoadedMsg)
	if !ok || msg.data.rtype != reportTypeHoldings {
		t.Fatalf("'i' loaded %+v, want the holdings report", msg)
	}
}

func TestHoldings_MenuOpensTheReport(t *testing.T) {
	app := newHoldingsApp(180, 40)
	_, cmd := app.handleMenuAction(widget.MenuActionHoldings, "")
	if cmd == nil {
		t.Fatal("the Holdings menu item should return a load command")
	}
	if msg, ok := cmd().(reportsViewDataLoadedMsg); !ok || msg.data.rtype != reportTypeHoldings {
		t.Fatalf("the menu loaded %+v, want the holdings report", msg)
	}
	if app.currentView != ViewReports {
		t.Errorf("view = %v, want Reports", app.currentView)
	}
}

// Enter opens the split of the row under the cursor; Esc goes back to the
// holdings table with the cursor where it was; a second Esc leaves the view.
func TestHoldings_SplitOpensAndCloses(t *testing.T) {
	app := newHoldingsApp(180, 40)
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !app.reports.splitOpen() {
		t.Fatal("Enter did not open the split")
	}
	view := app.reports.render(app.styles, app.height)
	for _, want := range []string{
		"ACME  Acme Total Market Index", "$4800.00  80.0% of total",
		"Account", "% of Holding",
		"Maple Invest Brokerage", "$3600.00", "75.0%",
		"Cedar HSA Investment", "$1200.00", "25.0%",
		"Total", "100.0%",
		"esc back to holdings",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("split lacks %q:\n%s", want, view)
		}
	}
	if strings.Index(view, "Maple Invest Brokerage") > strings.Index(view, "Cedar HSA Investment") {
		t.Errorf("the larger account is not first:\n%s", view)
	}
	if hints := app.reports.holdingsHints(); !strings.Contains(hints, "esc back to holdings") {
		t.Errorf("split hints = %q", hints)
	}

	app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.reports.splitOpen() {
		t.Fatal("Esc did not close the split")
	}
	if app.currentView != ViewReports {
		t.Fatalf("Esc on the split left the view for %v", app.currentView)
	}
	if c := app.reports.holdings.table.Cursor(); c != 0 {
		t.Errorf("cursor = %d after the split, want 0", c)
	}

	app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.currentView == ViewReports {
		t.Error("Esc on the holdings table did not leave the view")
	}
}

// The Cash row's split lists the cash of each account, with no Shares column.
func TestHoldings_CashSplit(t *testing.T) {
	app := newHoldingsApp(180, 40)
	app.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	view := app.reports.render(app.styles, app.height)
	if !strings.Contains(view, "Cash  Uninvested cash") || !strings.Contains(view, "$700.00") {
		t.Errorf("cash split:\n%s", view)
	}
	if strings.Contains(view, "Shares") {
		t.Errorf("cash split has a Shares column:\n%s", view)
	}
}

// A click on the first row selects it, and a second click on it opens the
// split. The click lands on the row the table draws there, which pins the
// rows above the table (tableContentRowOffset).
func TestHoldings_ClickAndDoubleClick(t *testing.T) {
	app := newHoldingsApp(180, 40)
	app.reports.render(app.styles, app.height)
	app.reports.holdings.table.SetCursor(2)

	now := time.Unix(0, 0)
	app.reports.holdings.clicks = widget.NewClickTracker(400 * time.Millisecond)
	app.reports.holdings.clicks.SetNowFn(func() time.Time { return now })

	// The app header is line 0; the view's padding, title and separator take
	// three more, then the table header and its border: the first row is Y=6.
	click := tea.MouseClickMsg{X: app.styles.SidebarWidth() + 10, Y: 6, Button: tea.MouseLeft}
	app.Update(click)
	if c := app.reports.holdings.table.Cursor(); c != 0 {
		t.Fatalf("cursor = %d after a click on the first row, want 0", c)
	}
	if app.reports.splitOpen() {
		t.Fatal("one click opened the split")
	}
	now = now.Add(100 * time.Millisecond)
	app.Update(click)
	if !app.reports.splitOpen() {
		t.Fatal("a double click did not open the split")
	}
	if !strings.Contains(app.reports.render(app.styles, app.height), "ACME  Acme Total Market Index") {
		t.Error("the double click opened another row's split")
	}
}

// A reload keeps the cursor on the same security, even when it moves, and
// closes an open split. Leaving the view closes the split too.
func TestHoldings_ReloadKeepsTheRow(t *testing.T) {
	app := newHoldingsApp(180, 40)
	app.reports.holdings.table.SetCursor(2) // STRK
	app.reports.openSplit()

	rpt := holdingsTestReport()
	rpt.Sections[0].Rows[2].SecurityID = app.reports.data.holdings.Sections[0].Rows[2].SecurityID
	rows := rpt.Sections[0].Rows
	rows[0], rows[2] = rows[2], rows[0] // STRK is now first
	app.reports.setData(&reportsViewData{rtype: reportTypeHoldings, holdings: rpt})

	if app.reports.splitOpen() {
		t.Error("a reload left the split open")
	}
	if c := app.reports.holdings.table.Cursor(); c != 0 {
		t.Errorf("cursor = %d after the reload, want 0 (STRK)", c)
	}

	app.reports.openSplit()
	app.switchView(ViewDashboard)
	if app.reports.holdings.split != nil {
		t.Error("leaving the view left the split open")
	}
}

func TestHoldings_Hints(t *testing.T) {
	app := newHoldingsApp(180, 40)
	e, _ := viewFor(ViewReports)
	if h := e.hints(app); !strings.Contains(h, "enter accounts") || strings.Contains(h, "period") {
		t.Errorf("holdings hints = %q", h)
	}
	app.reports.setData(&reportsViewData{rtype: reportTypeNetWorth})
	if h := e.hints(app); !strings.Contains(h, "i holdings") || !strings.Contains(h, "period") {
		t.Errorf("net worth hints = %q", h)
	}
	if e.table(app) != nil {
		t.Error("the net worth report has a table for the mouse")
	}
}
