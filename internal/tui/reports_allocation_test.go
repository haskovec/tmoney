package tui

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/sidebar"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// testHolding is a priced holding of a class in allocationTestReport.
func testHolding(label, name, value string, ofClass, ofTotal float64) report.ClassHolding {
	m := types.MustNewMoney
	return report.ClassHolding{
		HoldingRow: report.HoldingRow{SecurityID: types.NewID(), Label: label, Name: name, Value: m(value),
			Accounts: []report.HoldingAccount{{AccountID: types.NewID(), Name: "Maple Invest Brokerage", Value: m(value), Percent: 100}}},
		PercentOfClass: ofClass, PercentOfTotal: ofTotal,
	}
}

// allocationTestSection is a section of $10000: Large Cap (ACME, INIT, and
// STRK with no price), Mid Cap (MIDX), Cash (the uninvested cash), and
// Unclassified (MYST).
func allocationTestSection(currency string) report.AllocationSection {
	m := types.MustNewMoney
	cash := report.ClassHolding{HoldingRow: report.HoldingRow{SecurityID: types.NilID, Cash: true, Label: "Cash",
		Name: "Uninvested cash", Value: m("1000.00"), AssetClass: security.AssetClassCash,
		Accounts: []report.HoldingAccount{{AccountID: types.NewID(), Name: "Maple Invest Brokerage", Value: m("1000.00"), Percent: 100}}},
		PercentOfClass: 100, PercentOfTotal: 10}
	return report.AllocationSection{
		Currency: currency, Value: m("10000.00"), Available: true,
		Classes: []report.ClassRow{
			{Class: security.AssetClassLargeCapStock, Value: m("6000.00"), Percent: 60,
				Holdings: []report.ClassHolding{
					testHolding("ACME", "Acme Total Market Index", "4500.00", 75, 45),
					testHolding("INIT", "Initech Corp", "1500.00", 25, 15),
				},
				NoPrice: []report.HoldingRow{{SecurityID: types.NewID(), Label: "STRK", Name: "Stark Industries",
					Value: m("500.00"), Estimated: true}}},
			{Class: security.AssetClassMidCapStock, Value: m("2500.00"), Percent: 25,
				Holdings: []report.ClassHolding{testHolding("MIDX", "Maple Mid Cap Index", "2500.00", 100, 25)}},
			{Class: security.AssetClassCash, Value: m("1000.00"), Percent: 10, Holdings: []report.ClassHolding{cash}},
			{Class: security.AssetClassUnclassified, Value: m("500.00"), Percent: 5,
				Holdings: []report.ClassHolding{testHolding("MYST", "Mystery Fund", "500.00", 100, 5)}},
		},
	}
}

func allocationTestReport() *report.Allocation {
	return &report.Allocation{AsOfDate: types.NewDate(2024, 1, 15), AccountCount: 1,
		Sections: []report.AllocationSection{allocationTestSection("USD")}}
}

// newAllocationApp is an App on the Reports view with rpt loaded, at the
// given terminal size.
func newAllocationApp(width, height int, rpt *report.Allocation) *App {
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
	app.reports.setData(&reportsViewData{rtype: reportTypeAllocation, allocation: rpt})
	return app
}

func (a *App) renderedReports() string {
	return widget.StripAnsi(a.reports.render(a.styles, a.height, !a.lightBackground))
}

func classHeaders(cols []allocationColumn) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.Header
	}
	return out
}

// The Holdings column goes below 61 cells; the bar gets the spare width and
// at least 10, so below 52 the table is wider than the screen and is cut.
func TestAllocationColumns_WidthRule(t *testing.T) {
	for _, tc := range []struct {
		width, bar, total int
		count             bool
	}{
		{61, 10, 61, true},
		{60, 18, 60, false},
		{52, 10, 52, false},
		{51, 10, 52, false},
		{120, 40, 91, true},
	} {
		cols := allocationColumns(tc.width)
		headers := strings.Join(classHeaders(cols), "|")
		if strings.Contains(headers, "Holdings") != tc.count {
			t.Errorf("width %d: columns %s, want Holdings %v", tc.width, headers, tc.count)
		}
		total := len(cols) - 1
		for _, c := range cols {
			total += c.Width
		}
		if bar := cols[len(cols)-1].Width; bar != tc.bar || total != tc.total {
			t.Errorf("width %d: bar %d and total %d, want %d and %d", tc.width, bar, total, tc.bar, tc.total)
		}
	}
}

// The class table lists the classes with a priced holding, in the report's
// order, with the TOTAL, the notes, and the hints.
func TestRenderAllocation(t *testing.T) {
	app := newAllocationApp(180, 40, allocationTestReport())
	view := app.renderedReports()
	for _, want := range []string{
		"ASSET ALLOCATION", "As of: Jan 15, 2024",
		"Asset Class", "Holdings", "Value", "% Total",
		"Large Cap Stock            2      $6000.00   60.0% ████",
		"Mid Cap Stock              1      $2500.00   25.0%",
		"Cash                       0      $1000.00   10.0%",
		"TOTAL                      4     $10000.00  100.0%",
		"Not included (no price): STRK (Large Cap Stock). Add a price to include it.",
		"1 holding has no asset class. Press enter on Unclassified, then e to set a class.",
		"enter holdings  n net worth  s spending  i holdings  a allocation  esc back",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	order := []string{"Large Cap Stock", "Mid Cap Stock", "Cash  ", "Unclassified", "TOTAL"}
	for i := 1; i < len(order); i++ {
		if strings.Index(view, order[i-1]) > strings.Index(view, order[i]) {
			t.Errorf("%q is not above %q:\n%s", order[i-1], order[i], view)
		}
	}
	if strings.Contains(view, "currency") || strings.Contains(view, "USD (") {
		t.Errorf("one currency shows the currency keys or count:\n%s", view)
	}
}

// The bars are in the dark class colors until the terminal says that its
// background is light; the text is never in a class color.
func TestRenderAllocation_LightColors(t *testing.T) {
	app := newAllocationApp(180, 40, allocationTestReport())
	fg := func(c color.RGBA) string { return fmt.Sprintf("38;2;%d;%d;%d", c.R, c.G, c.B) }
	dark := fg(color.RGBA{R: 0x36, G: 0x64, B: 0xb2}) // Large Cap, dark
	light := fg(color.RGBA{R: 0x18, G: 0x4f, B: 0x95})

	view := app.reports.render(app.styles, app.height, !app.lightBackground)
	if !strings.Contains(view, dark) || strings.Contains(view, light) {
		t.Errorf("before a reply the bar is not in the dark color")
	}
	app.Update(tea.BackgroundColorMsg{Color: color.White})
	view = app.reports.render(app.styles, app.height, !app.lightBackground)
	if !strings.Contains(view, light) || strings.Contains(view, dark) {
		t.Errorf("after a light reply the bar is not in the light color")
	}
	for line := range strings.Lines(view) {
		if strings.Contains(line, light) && strings.Contains(widget.StripAnsi(line), "Large Cap Stock") {
			name := strings.Index(line, "Large Cap Stock")
			if strings.LastIndex(line[:name], light) > strings.LastIndex(line[:name], "\x1b[m") {
				t.Errorf("the class name is in the class color: %q", line)
			}
		}
	}
}

// 'a' loads the report from another report, and the menu item opens it.
func TestAllocation_KeyAAndMenu(t *testing.T) {
	app := newAllocationApp(180, 40, allocationTestReport())
	app.reports.setData(&reportsViewData{rtype: reportTypeSpending, year: 2023, month: 3})
	_, cmd := app.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if cmd == nil {
		t.Fatal("'a' returned no command")
	}
	msg, ok := cmd().(reportsViewDataLoadedMsg)
	if !ok || msg.data.rtype != reportTypeAllocation || msg.data.year != 2023 || msg.data.month != 3 {
		t.Fatalf("'a' loaded %+v, want the allocation report with the spending period kept", msg)
	}

	app.switchView(ViewDashboard)
	_, cmd = app.handleMenuAction(widget.MenuActionAssetAllocation, "")
	if cmd == nil {
		t.Fatal("the Asset Allocation menu item returned no command")
	}
	if msg, ok := cmd().(reportsViewDataLoadedMsg); !ok || msg.data.rtype != reportTypeAllocation {
		t.Fatalf("the menu loaded %+v, want the allocation report", msg)
	}
	if app.currentView != ViewReports {
		t.Errorf("view = %v, want Reports", app.currentView)
	}
}

// Enter opens the class under the cursor, and Enter there the split of the
// holding. Esc goes back one level at a time, with the cursor on its row,
// and one more Esc leaves the view.
func TestAllocation_DrillDownAndBack(t *testing.T) {
	app := newAllocationApp(180, 40, allocationTestReport())
	e, _ := viewFor(ViewReports)
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !app.reports.drillOpen() || app.reports.allocation.drillClass != security.AssetClassLargeCapStock {
		t.Fatal("Enter did not open Large Cap Stock")
	}
	view := app.renderedReports()
	for _, want := range []string{
		"■ Large Cap Stock", "$6000.00  60.0% of total",
		"Security", "Name", "% of Class",
		"ACME     Acme Total Market Index", "$4500.00", "75.0%", "45.0%",
		"STRK     Stark Industries", "no price",
		"Total", "100.0%",
		"Not included (no price): STRK. Add a price to include it.",
		"enter accounts  e edit security  esc back to allocation",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("drill-down lacks %q:\n%s", want, view)
		}
	}
	if h := e.hints(app); !strings.HasPrefix(h, "↑↓ navigate  enter accounts  e edit security  esc back to allocation") {
		t.Errorf("drill-down hints = %q", h)
	}

	app.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if app.reports.allocation.split == nil {
		t.Fatal("Enter in the drill-down did not open the split")
	}
	view = app.renderedReports()
	for _, want := range []string{"INIT  Initech Corp", "$1500.00  15.0% of total", "% of Holding", "esc back to class"} {
		if !strings.Contains(view, want) {
			t.Errorf("split lacks %q:\n%s", want, view)
		}
	}
	if h := e.hints(app); !strings.HasPrefix(h, "↑↓ navigate  esc back to class") {
		t.Errorf("split hints = %q", h)
	}

	app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.reports.allocation.split != nil || !app.reports.drillOpen() {
		t.Fatal("Esc on the split did not go back to the drill-down")
	}
	if c := app.reports.allocation.drill.Cursor(); c != 1 {
		t.Errorf("drill-down cursor = %d, want 1 (INIT)", c)
	}
	app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.reports.drillOpen() || app.currentView != ViewReports {
		t.Fatal("Esc on the drill-down did not go back to the class table")
	}
	if c := app.reports.allocation.table.Cursor(); c != 0 {
		t.Errorf("class cursor = %d, want 0 (Large Cap Stock)", c)
	}
	app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.currentView == ViewReports {
		t.Error("Esc on the class table did not leave the view")
	}
}

// The split of a holding with no price has the ~ marks and no percentage.
// The Cash row's split has no Shares column.
func TestAllocation_SplitOfNoPriceAndCash(t *testing.T) {
	app := newAllocationApp(180, 40, allocationTestReport())
	app.reports.openDrill()
	app.reports.allocation.drill.SetCursor(2) // STRK
	app.reports.openAllocationSplit()
	view := app.renderedReports()
	for _, want := range []string{"~STRK  Stark Industries", "~$500.00  no price", "~ No price on file"} {
		if !strings.Contains(view, want) {
			t.Errorf("split lacks %q:\n%s", want, view)
		}
	}

	app.reports.closeDrill()
	app.reports.allocation.table.SetCursor(2) // Cash
	app.reports.openDrill()
	app.reports.openAllocationSplit()
	if view := app.renderedReports(); !strings.Contains(view, "Cash  Uninvested cash") || strings.Contains(view, "Shares") {
		t.Errorf("cash split:\n%s", view)
	}
}

// A click selects a row, and a second click on it opens it: a class opens
// its holdings, and a holding its split.
func TestAllocation_ClickAndDoubleClick(t *testing.T) {
	app := newAllocationApp(180, 40, allocationTestReport())
	app.renderedReports()
	app.reports.allocation.table.SetCursor(2)
	now := time.Unix(0, 0)
	app.reports.allocation.clicks = widget.NewClickTracker(400 * time.Millisecond)
	app.reports.allocation.clicks.SetNowFn(func() time.Time { return now })

	// The first row is Y=6, as in the holdings report.
	click := tea.MouseClickMsg{X: app.styles.SidebarWidth() + 10, Y: 6, Button: tea.MouseLeft}
	app.Update(click)
	if c := app.reports.allocation.table.Cursor(); c != 0 || app.reports.drillOpen() {
		t.Fatalf("one click: cursor %d, drill-down open %v; want 0 and closed", c, app.reports.drillOpen())
	}
	now = now.Add(100 * time.Millisecond)
	app.Update(click)
	if !app.reports.drillOpen() || app.reports.allocation.drillClass != security.AssetClassLargeCapStock {
		t.Fatal("a double click did not open Large Cap Stock")
	}

	app.renderedReports()
	now = now.Add(time.Second)
	app.Update(click)
	now = now.Add(100 * time.Millisecond)
	app.Update(click)
	if app.reports.allocation.split == nil || !strings.Contains(app.renderedReports(), "ACME  Acme Total Market Index") {
		t.Error("a double click in the drill-down did not open the split of ACME")
	}
}

// With two currencies, ← and → change the currency on screen, and the title
// and the hints say so. With one currency they do nothing.
func TestAllocation_Currencies(t *testing.T) {
	rpt := allocationTestReport()
	rpt.Sections = []report.AllocationSection{allocationTestSection("EUR"), allocationTestSection("USD")}
	app := newAllocationApp(180, 40, rpt)
	e, _ := viewFor(ViewReports)

	if view := app.renderedReports(); !strings.Contains(view, "ASSET ALLOCATION   EUR (1 of 2)") || !strings.Contains(view, "€10000.00") {
		t.Errorf("first currency:\n%s", view)
	}
	if h := e.hints(app); !strings.Contains(h, "←→ currency") {
		t.Errorf("hints = %q, want the currency keys", h)
	}
	app.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	app.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if view := app.renderedReports(); !strings.Contains(view, "USD (2 of 2)") || !strings.Contains(view, "$10000.00") {
		t.Errorf("→ did not show USD:\n%s", view)
	}
	if c := app.reports.allocation.table.Cursor(); c != 0 {
		t.Errorf("cursor = %d after a currency change, want 0", c)
	}
	app.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if app.reports.allocation.section != 1 {
		t.Error("→ on the last currency moved")
	}
	app.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if app.reports.allocation.section != 0 {
		t.Error("← did not go back to EUR")
	}

	one := newAllocationApp(180, 40, allocationTestReport())
	one.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if one.reports.allocation.section != 0 {
		t.Error("→ with one currency moved")
	}
	if h := e.hints(one); strings.Contains(h, "currency") {
		t.Errorf("one currency hints = %q", h)
	}
}

// A reload keeps the currency, the class and the drill-down (C10): the
// cursor stays on its holding, stays at its position when the holding left
// the class, and the drill-down closes when the class is now empty.
func TestAllocation_ReloadKeepsThePlace(t *testing.T) {
	app := newAllocationApp(180, 40, allocationTestReport())
	app.reports.openDrill()
	app.reports.allocation.drill.SetCursor(1) // INIT
	initech := app.reports.data.allocation.Sections[0].Classes[0].Holdings[1].SecurityID

	t.Run("the holding stays", func(t *testing.T) {
		rpt := allocationTestReport()
		large := &rpt.Sections[0].Classes[0]
		large.Holdings[0].SecurityID = initech // INIT is now first
		app.reports.setData(&reportsViewData{rtype: reportTypeAllocation, allocation: rpt})
		if !app.reports.drillOpen() || app.reports.allocation.drill.Cursor() != 0 {
			t.Fatalf("drill-down open %v, cursor %d; want open on INIT at 0", app.reports.drillOpen(), app.reports.allocation.drill.Cursor())
		}
	})

	t.Run("the holding left the class", func(t *testing.T) {
		app.reports.allocation.drill.SetCursor(1)
		rpt := allocationTestReport()
		app.reports.setData(&reportsViewData{rtype: reportTypeAllocation, allocation: rpt})
		if !app.reports.drillOpen() || app.reports.allocation.drillClass != security.AssetClassLargeCapStock {
			t.Fatal("the drill-down did not stay on Large Cap Stock")
		}
		if c := app.reports.allocation.drill.Cursor(); c != 1 {
			t.Errorf("cursor = %d, want 1: the same position", c)
		}
	})

	t.Run("the class is now empty", func(t *testing.T) {
		app.reports.closeDrill()
		app.reports.allocation.table.SetCursor(1) // Mid Cap
		app.reports.openDrill()
		rpt := allocationTestReport()
		classes := rpt.Sections[0].Classes
		rpt.Sections[0].Classes = append(classes[:1], classes[2:]...)
		app.reports.setData(&reportsViewData{rtype: reportTypeAllocation, allocation: rpt})
		if app.reports.drillOpen() {
			t.Error("the drill-down of an empty class stayed open")
		}
		if c := app.reports.allocation.table.Cursor(); c != 1 {
			t.Errorf("class cursor = %d, want 1: the next class", c)
		}
	})

	t.Run("leaving the view closes it", func(t *testing.T) {
		app.reports.openDrill()
		app.switchView(ViewDashboard)
		if app.reports.allocation.drill != nil {
			t.Error("leaving the view left the drill-down open")
		}
	})
}

// The status bar hints of each report name the allocation report, and the
// table for the mouse is the allocation table.
func TestAllocation_Hints(t *testing.T) {
	app := newAllocationApp(180, 40, allocationTestReport())
	e, _ := viewFor(ViewReports)
	if h := e.hints(app); !strings.HasPrefix(h, "↑↓ navigate  enter holdings  n net worth  s spending  i holdings  esc back") {
		t.Errorf("allocation hints = %q", h)
	}
	if e.table(app) != app.reports.allocation.table {
		t.Error("the mouse does not address the class table")
	}
	app.reports.setData(&reportsViewData{rtype: reportTypeHoldings, holdings: holdingsTestReport()})
	if h := e.hints(app); !strings.Contains(h, "a allocation") {
		t.Errorf("holdings hints = %q", h)
	}
	app.reports.setData(&reportsViewData{rtype: reportTypeNetWorth})
	if h := e.hints(app); !strings.Contains(h, "i holdings  a allocation") {
		t.Errorf("net worth hints = %q", h)
	}
}

// The empty states say what is missing.
func TestRenderAllocation_Empty(t *testing.T) {
	app := newAllocationApp(180, 40, &report.Allocation{})
	if view := app.renderedReports(); !strings.Contains(view, "No investment accounts. Add one to see the allocation.") {
		t.Errorf("no accounts:\n%s", view)
	}
	app = newAllocationApp(180, 40, &report.Allocation{AccountCount: 1})
	if view := app.renderedReports(); !strings.Contains(view, "No holdings in the investment accounts.") {
		t.Errorf("accounts that hold nothing:\n%s", view)
	}
}

// A failed account: the TOTAL is not available, and its error and the note
// show under it.
func TestRenderAllocation_FailedAccount(t *testing.T) {
	rpt := allocationTestReport()
	rpt.Sections[0].Available = false
	rpt.Failed = []report.AccountFailure{{Name: "Birch 401k", Currency: "USD", Err: errString("price lookup failed")}}
	app := newAllocationApp(180, 40, rpt)
	if !errorLineHasNoPadding(app.reports.render(app.styles, app.height, true)) {
		t.Error("the error line has padding")
	}
	view := app.renderedReports()
	for _, want := range []string{
		"TOTAL                        not available",
		"Birch 401k: could not be valued (price lookup failed)",
		"Percentages leave out accounts that could not be valued.",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
}

// 'e' in the drill-down opens the Edit Security dialog on the holding under
// the cursor, and does nothing on the uninvested cash. A save reloads the
// report, and the drill-down stays on its class (C10).
func TestAllocation_EditFromTheDrillDown(t *testing.T) {
	app := NewApp(dbtest.New(t), nil)
	app.width, app.height = 180, 40
	app.styles.Resize(app.width, app.height)
	svc := app.services
	brokerage := account.NewAccount("Maple Invest Brokerage", account.TypeInvestment, "USD", types.MustNewMoney("5000.00"), types.NewDate(2024, 1, 1))
	if err := svc.Account.Create(brokerage); err != nil {
		t.Fatal(err)
	}
	buy := func(ticker, name string, class security.AssetClass, shares, total, last string) *security.Security {
		t.Helper()
		sec := security.NewSecurity(ticker, name, security.TypeETF)
		sec.AssetClass = class
		if err := svc.Security.Create(sec); err != nil {
			t.Fatal(err)
		}
		amount := types.MustNewMoney(total)
		if _, err := svc.Investment.Buy(brokerage.ID, sec.ID, types.NewDate(2024, 1, 3),
			types.MustNewQuantity(shares), &amount, nil, types.ZeroMoney, ""); err != nil {
			t.Fatal(err)
		}
		if err := svc.Price.AddPrice(price.NewPrice(sec.ID, types.NewDate(2024, 2, 1),
			types.MustNewMoney(last), price.SourceManual)); err != nil {
			t.Fatal(err)
		}
		return sec
	}
	buy("ACME", "Acme Total Market Index", security.AssetClassLargeCapStock, "30", "3000.00", "120.00")
	initech := buy("INIT", "Initech Corp", security.AssetClassLargeCapStock, "10", "500.00", "60.00")
	buy("MAPXX", "Maple Money Market", security.AssetClassCash, "100", "100.00", "1.00")

	app.switchView(ViewReports)
	app.Update(app.reports.load(app.reportsDeps(), reportTypeAllocation, 2024, 1, false)())
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // Large Cap Stock
	app.Update(tea.KeyPressMsg{Code: tea.KeyDown})  // INIT
	_, cmd := app.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if cmd == nil {
		t.Fatal("'e' returned no command")
	}
	app.Update(cmd())
	if !app.security.IsVisible() || app.security.editID != initech.ID {
		t.Fatalf("'e' did not open Edit Security on INIT: %+v", app.security)
	}

	// Move INIT to Mid Cap Stock and save.
	field := app.security.dlg.Fields()[4]
	for i, ac := range security.AllAssetClasses() {
		if ac == security.AssetClassMidCapStock {
			field.SelectedIndex = i
		}
	}
	_, cmd = app.submitSecurityDialog()
	updated := cmd()
	if _, ok := updated.(securityUpdatedMsg); !ok {
		t.Fatalf("save returned %T: %v", updated, updated)
	}
	_, cmd = app.Update(updated)
	reloaded := false
	for _, msg := range runBatch(cmd) {
		if loaded, ok := msg.(reportsViewDataLoadedMsg); ok && loaded.data.rtype == reportTypeAllocation {
			app.Update(loaded)
			reloaded = true
		}
	}
	if !reloaded {
		t.Fatal("the save did not reload the allocation report")
	}
	if !app.reports.drillOpen() || app.reports.allocation.drillClass != security.AssetClassLargeCapStock {
		t.Fatal("the drill-down did not stay on Large Cap Stock")
	}
	if view := app.renderedReports(); strings.Contains(view, "INIT") || !strings.Contains(view, "ACME") {
		t.Errorf("the drill-down after the save:\n%s", view)
	}
	mid, ok := app.reports.data.allocation.Sections[0].Class(security.AssetClassMidCapStock)
	if !ok || len(mid.Holdings) != 1 || mid.Holdings[0].Label != "INIT" {
		t.Errorf("INIT is not in Mid Cap Stock: %+v", mid)
	}

	// The uninvested cash is not a security.
	app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	for i, ac := range app.reports.allocation.classes {
		if ac == security.AssetClassCash {
			app.reports.allocation.table.SetCursor(i)
		}
	}
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	for i := range app.reports.allocation.drillRefs {
		if row, _ := app.reports.drillHolding(i); row.Cash {
			app.reports.allocation.drill.SetCursor(i)
		}
	}
	app.security = securitySurface{}
	if _, cmd := app.Update(tea.KeyPressMsg{Code: 'e', Text: "e"}); cmd != nil {
		t.Error("'e' on the uninvested cash returned a command")
	}
}

// A reply to e that comes after its drill-down closed, after a reload, or
// while another dialog is open opens no dialog: the security loads in a
// command, and the keys do not wait for it.
func TestAllocation_StaleEditReply(t *testing.T) {
	app := NewApp(dbtest.New(t), nil)
	app.width, app.height = 180, 40
	app.styles.Resize(app.width, app.height)
	svc := app.services
	brokerage := account.NewAccount("Maple Invest Brokerage", account.TypeInvestment, "USD", types.MustNewMoney("5000.00"), types.NewDate(2024, 1, 1))
	if err := svc.Account.Create(brokerage); err != nil {
		t.Fatal(err)
	}
	sec := security.NewSecurity("ACME", "Acme Total Market Index", security.TypeETF)
	sec.AssetClass = security.AssetClassLargeCapStock
	if err := svc.Security.Create(sec); err != nil {
		t.Fatal(err)
	}
	amount := types.MustNewMoney("3000.00")
	if _, err := svc.Investment.Buy(brokerage.ID, sec.ID, types.NewDate(2024, 1, 3),
		types.MustNewQuantity("30"), &amount, nil, types.ZeroMoney, ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.Price.AddPrice(price.NewPrice(sec.ID, types.NewDate(2024, 2, 1),
		types.MustNewMoney("120.00"), price.SourceManual)); err != nil {
		t.Fatal(err)
	}
	app.switchView(ViewReports)
	load := func() {
		t.Helper()
		app.Update(app.reports.load(app.reportsDeps(), reportTypeAllocation, 2024, 1, false)())
	}
	load()
	// pressE opens the Large Cap Stock drill-down, presses e on ACME and
	// returns the reply, not yet delivered.
	pressE := func() tea.Msg {
		t.Helper()
		if !app.reports.drillOpen() {
			app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		}
		_, cmd := app.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
		if cmd == nil {
			t.Fatal("'e' returned no command")
		}
		return cmd()
	}

	for _, tc := range []struct {
		name   string
		before func()
	}{
		{"esc", func() { app.Update(tea.KeyPressMsg{Code: tea.KeyEscape}) }},
		{"another view", func() { app.switchView(ViewDashboard); app.switchView(ViewReports) }},
		{"reload", load},
		{"esc and back in", func() {
			app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		}},
	} {
		reply := pressE()
		tc.before()
		app.Update(reply)
		if app.security.IsVisible() {
			t.Errorf("%s: the late reply opened Edit Security", tc.name)
			app.security = securitySurface{}
		}
	}

	// Another dialog opened in the meantime stays, and is not replaced.
	reply := pressE()
	app.showHelp = true
	app.Update(reply)
	if app.security.IsVisible() || !app.showHelp {
		t.Error("the late reply opened Edit Security over the help")
	}
	app.showHelp = false

	// A reply opens its dialog once.
	reply = pressE()
	app.Update(reply)
	if !app.security.IsVisible() || app.security.editID != sec.ID {
		t.Fatal("the reply did not open Edit Security")
	}
	app.security = securitySurface{}
	app.Update(reply)
	if app.security.IsVisible() {
		t.Error("the same reply opened Edit Security two times")
	}
}

// Many holdings with no price make a note longer than the screen. The
// footer does not scroll, so the note counts them in place of naming them,
// and the view keeps its hint and a table.
func TestRenderAllocation_LongNoPriceNote(t *testing.T) {
	rpt := allocationTestReport()
	large := &rpt.Sections[0].Classes[0]
	for i := range 40 {
		large.NoPrice = append(large.NoPrice, report.HoldingRow{SecurityID: types.NewID(),
			Label: fmt.Sprintf("NP%02d", i+1), Name: "Unpriced Fund", Estimated: true})
	}
	fits := func(app *App, view string) {
		t.Helper()
		if lines := strings.Count(view, "\n") + 1; lines > app.height-appChromeRows {
			t.Errorf("the view is %d lines, more than the %d of the pane:\n%s", lines, app.height-appChromeRows, view)
		}
	}
	// The notes wrap, so compare with each run of space as one space.
	flat := func(view string) string { return strings.Join(strings.Fields(view), " ") }

	app := newAllocationApp(100, 18, rpt)
	view := app.renderedReports()
	fits(app, view)
	for _, want := range []string{"Not included (no price): 41 holdings. Press i to see them in the holdings report.",
		"esc back", "Large Cap Stock"} {
		if !strings.Contains(flat(view), want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}

	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // Large Cap Stock
	view = app.renderedReports()
	fits(app, view)
	for _, want := range []string{"Not included (no price): 41 holdings. Add a price to include them.",
		"esc back to allocation", "ACME"} {
		if !strings.Contains(flat(view), want) {
			t.Errorf("drill-down lacks %q:\n%s", want, view)
		}
	}

	// With room, the note names each holding.
	app = newAllocationApp(100, 60, rpt)
	if view := app.renderedReports(); !strings.Contains(view, "NP40 (Large Cap Stock)") {
		t.Errorf("the full note is not on a tall screen:\n%s", view)
	}
}

// On a narrow screen the notes wrap to the table width: no line is cut.
func TestRenderAllocation_NotesWrap(t *testing.T) {
	app := newAllocationApp(70, 30, allocationTestReport())
	view := app.renderedReports()
	if strings.Contains(view, "…") {
		t.Errorf("a line is cut:\n%s", view)
	}
	for _, want := range []string{"include it.", "to set a class."} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	for line := range strings.Lines(view) {
		if w := lipgloss.Width(strings.TrimRight(line, " \n")); w > app.styles.ContentWidth() {
			t.Errorf("line is %d cells, wider than the %d of the pane: %q", w, app.styles.ContentWidth(), line)
		}
	}
}
