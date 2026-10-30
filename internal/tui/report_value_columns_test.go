package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// A money column is as wide as its widest amount, and the widths at which
// columns drop grow by the cells it adds.
func TestHoldingsColumns_MoneyWidths(t *testing.T) {
	money := map[holdingsCol]int{holdingsColPrice: 12, holdingsColValue: 15, holdingsColCost: 15, holdingsColGain: 14}
	widths := func(cols []holdingsColumn) map[holdingsCol]int {
		out := make(map[holdingsCol]int)
		for _, c := range cols {
			out[c.col] = c.Width
		}
		return out
	}
	got := widths(holdingsColumns(200, 26, money))
	for col, w := range money {
		if got[col] != w {
			t.Errorf("column %d: width %d, want %d", col, got[col], w)
		}
	}
	// 2 more cells for Price, Value and Cost Basis, 2 for Gain.
	for _, tc := range []struct {
		width int
		want  string
	}{
		{117, "Security Name Shares Price Value % Total Cost Basis Gain "},
		{116, "Security Name Value % Total Cost Basis Gain "},
		{91, "Security Name Value % Total Cost Basis Gain "},
		{90, "Security Name Value % Total "},
		{60, "Security Name Value % Total "},
		{59, "Security Value % Total "},
	} {
		if got := strings.Join(holdingsHeaders(holdingsColumns(tc.width, 26, money)), " "); got != tc.want {
			t.Errorf("width %d: columns %q, want %q", tc.width, got, tc.want)
		}
	}
	// A narrower amount does not make a column narrower.
	if got := widths(holdingsColumns(200, 26, map[holdingsCol]int{holdingsColValue: 5})); got[holdingsColValue] != 13 {
		t.Errorf("Value width %d, want 13", got[holdingsColValue])
	}
}

// yenHoldingsReport is holdingsTestReport in yen, with amounts wider than
// the usual columns.
func yenHoldingsReport() *report.Holdings {
	m := types.MustNewMoney
	q := types.MustNewQuantity
	return &report.Holdings{
		AsOfDate: types.NewDate(2024, 1, 15),
		Sections: []report.HoldingsSection{{
			Currency: "JPY", Available: true,
			Value: m("12345678.00"), CostBasis: m("10000000.00"), Gain: m("-2345678.00"),
			Rows: []report.HoldingRow{
				{SecurityID: types.NewID(), Label: "ACME", Name: "Acme Total Market Index",
					Shares: q("400"), Price: m("30864.20"), Value: m("12345678.00"), CostBasis: m("10000000.00"),
					Gain: m("-2345678.00"), Percent: 100,
					Accounts: []report.HoldingAccount{
						{AccountID: types.NewID(), Name: "Maple Invest Brokerage", Shares: q("300"), Value: m("9259258.50"), Percent: 75},
						{AccountID: types.NewID(), Name: "Cedar HSA Investment", Shares: q("100"), Value: m("3086419.50"), Percent: 25},
					}},
			},
		}},
	}
}

// noCutAmount fails when the view cuts an amount: every amount is whole.
func noCutAmount(t *testing.T, name, view string, amounts ...string) {
	t.Helper()
	for _, a := range amounts {
		if !strings.Contains(view, a) {
			t.Errorf("%s lacks the whole amount %q:\n%s", name, a, view)
		}
	}
	if strings.Contains(view, "JPY") && strings.Contains(view, ".…") {
		t.Errorf("%s has a cut amount:\n%s", name, view)
	}
}

// The holdings table and its split show yen amounts whole.
func TestRenderHoldings_WideAmounts(t *testing.T) {
	app := newHoldingsApp(200, 30)
	app.reports.setData(&reportsViewData{rtype: reportTypeHoldings, holdings: yenHoldingsReport()})
	view := widget.StripAnsi(app.reports.render(app.styles, app.height, reportsLook{dark: true}))
	noCutAmount(t, "holdings", view,
		"JPY 30864.20", "JPY 12345678.00", "JPY 10000000.00", "JPY -2345678.00")
	// The TOTAL line's amounts end where the row's do.
	var row, total string
	for line := range strings.Lines(view) {
		switch {
		case strings.Contains(line, "ACME"):
			row = line
		case strings.Contains(line, "TOTAL"):
			total = line
		}
	}
	if end := strings.Index(row, "JPY 12345678.00") + len("JPY 12345678.00"); strings.Index(total, "JPY 12345678.00")+len("JPY 12345678.00") != end {
		t.Errorf("the TOTAL value does not line up with the row:\n%s%s", row, total)
	}

	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	view = widget.StripAnsi(app.reports.render(app.styles, app.height, reportsLook{dark: true}))
	noCutAmount(t, "split", view, "JPY 9259258.50", "JPY 3086419.50", "JPY 12345678.00")
}

// yenAllocationReport is one yen section with amounts wider than the usual
// Value column.
func yenAllocationReport() *report.Allocation {
	m := types.MustNewMoney
	h := testHolding("ACME", "Acme Total Market Index", "12345678.00", 100, 98.4)
	h.Accounts = []report.HoldingAccount{{AccountID: types.NewID(), Name: "Maple Invest Brokerage", Value: m("12345678.00"), Percent: 100}}
	return &report.Allocation{AsOfDate: types.NewDate(2024, 1, 15), AccountCount: 1, Sections: []report.AllocationSection{{
		Currency: "JPY", Value: m("12545678.00"), Available: true,
		Classes: []report.ClassRow{
			{Class: security.AssetClassLargeCapStock, Value: m("12345678.00"), Percent: 98.4, Holdings: []report.ClassHolding{h}},
			{Class: security.AssetClassCash, Value: m("200000.00"), Percent: 1.6,
				Holdings: []report.ClassHolding{testHolding("Cash", "Uninvested cash", "200000.00", 100, 1.6)}},
		},
	}}}
}

// The class table, the pie's legend, the class view and the split show yen
// amounts whole, and the legend is as wide as its widest amount.
func TestRenderAllocation_WideAmounts(t *testing.T) {
	app := newAllocationApp(200, 40, yenAllocationReport())
	noCutAmount(t, "class table", app.renderedReports(), "JPY 12345678.00", "JPY 12545678.00", "JPY 200000.00")

	app = newPieApp(200, 40, yenAllocationReport())
	app.step(t, tea.WindowSizeMsg{Width: 200, Height: 40})
	if _, ok := app.wantedPie(); !ok {
		t.Fatal("no pie")
	}
	view := app.renderedReports()
	noCutAmount(t, "legend", view, "JPY 12345678.00", "JPY 12545678.00")
	if w := columnsWidth(app.reports.allocation.columns); w != 52+2 {
		t.Errorf("legend width %d, want 54", w)
	}

	app.step(t, tea.KeyPressMsg{Code: tea.KeyEnter})
	noCutAmount(t, "class view", app.renderedReports(), "JPY 12345678.00")
	app.step(t, tea.KeyPressMsg{Code: tea.KeyEnter})
	noCutAmount(t, "split", app.renderedReports(), "JPY 12345678.00")
}

// The pie needs room for the wider legend. At 96 columns the table is 72
// cells; with square cells the pie's columns are its rows, so the 54-cell
// legend leaves 72 - 3 - 54 = 15 rows. The usual 52-cell legend would leave
// 17.
func TestAllocationPie_WideLegendFit(t *testing.T) {
	app := newPieApp(96, 40, yenAllocationReport())
	app.pie.cellW, app.pie.cellH = 20, 20
	if w := allocationTableWidth(app.styles); w != 72 {
		t.Fatalf("table width %d, want 72", w)
	}
	sec, _ := app.reports.allocationSection()
	if w := columnsWidth(allocationLegendColumns(allocationValueWidth(sec))); w != 54 {
		t.Fatalf("legend width %d, want 54", w)
	}
	if key, ok := app.wantedPie(); !ok || key.rows != 15 {
		t.Errorf("pie rows %d (ok %v), want 15", key.rows, ok)
	}
}

// The Holdings column goes at the usual width plus the cells a wider Value
// column adds.
func TestAllocationColumns_WideValue(t *testing.T) {
	has := func(cols []allocationColumn) bool {
		return strings.Contains(strings.Join(classHeaders(cols), "|"), "Holdings")
	}
	if !has(allocationColumns(63, 15)) || has(allocationColumns(62, 15)) {
		t.Error("with a Value of 15 cells, Holdings must show from 63 cells")
	}
}

// The spending report's amounts end under the end of the Amount header, in
// color too: the color codes are not cells. The Amount column is as wide as
// its widest amount, and no line is wider than the content.
func TestRenderSpending_ColumnsAlign(t *testing.T) {
	styles := widget.NewStyles()
	styles.Resize(120, 30)
	s := reportsViewState{data: &reportsViewData{rtype: reportTypeSpending, year: 2024, month: 1, spending: &report.Spending{
		Period:        "January 2024",
		TotalSpending: types.MustNewMoney("123456789"),
		Categories: []report.CategorySpending{
			{Name: "Housing", Amount: types.MustNewMoney("1500"), Percentage: 0.1,
				Subcategories: []report.CategorySpending{{Name: "Rent", Amount: types.MustNewMoney("1500")}}},
			{Name: "Transportation", Amount: types.MustNewMoney("123455289"), Percentage: 99.9},
		},
	}}}
	lines := strings.Split(ansi.Strip(s.renderSpending(styles)), "\n")

	end := -1
	for _, l := range lines {
		if strings.Contains(l, "Category") {
			end = strings.Index(l, "Amount") + len("Amount")
		}
	}
	if end < 0 {
		t.Fatal("no header line")
	}
	found := 0
	for _, l := range lines {
		for _, amount := range []string{"$1500.00", "$123455289.00", "$123456789.00"} {
			if i := strings.Index(l, amount); i >= 0 {
				found++
				if i+len(amount) != end {
					t.Errorf("%q ends at %d, want %d: %q", amount, i+len(amount), end, l)
				}
			}
		}
		if w := lipgloss.Width(l); w > styles.ContentWidth() {
			t.Errorf("line is %d cells, wider than the content (%d): %q", w, styles.ContentWidth(), l)
		}
	}
	if found != 4 {
		t.Errorf("found %d amounts, want 4", found)
	}
}
