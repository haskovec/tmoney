package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// holdingsCol names a column of the holdings table.
type holdingsCol int

const (
	holdingsColSecurity holdingsCol = iota
	holdingsColName
	holdingsColShares
	holdingsColPrice
	holdingsColValue
	holdingsColPercent
	holdingsColCost
	holdingsColGain
	holdingsColBar
)

// holdingsColumn is one column of the holdings table on screen.
type holdingsColumn struct {
	col holdingsCol
	widget.Column
}

// The table widths at which a narrower screen drops columns, and the limits
// of the two columns that share the spare width. See
// specs/design-holdings-report.md §7.2.
const (
	holdingsWidthAll  = 109 // below this, no Shares and no Price
	holdingsWidthCost = 85  // below this, also no Cost Basis and no Gain
	holdingsWidthName = 58  // below this, also no Name
	holdingsMinName   = 16
	holdingsMaxBar    = 40
)

// holdingsColumns returns the columns that fit a table of width cells, each
// with a fixed width so the bar width is known when the rows are built.
// Shares and Price go first, then Cost Basis and Gain, then Name; Security,
// Value, % Total and the bar always stay. Name and the bar share the spare
// width: Name stops at the longest name, and the bar at holdingsMaxBar.
func holdingsColumns(width, longestName int) []holdingsColumn {
	all := []holdingsColumn{
		{holdingsColSecurity, widget.Column{Header: "Security", Width: 8}},
		{holdingsColName, widget.Column{Header: "Name"}},
		{holdingsColShares, widget.Column{Header: "Shares", Width: 12, Align: widget.AlignRight}},
		{holdingsColPrice, widget.Column{Header: "Price", Width: 10, Align: widget.AlignRight}},
		{holdingsColValue, widget.Column{Header: "Value", Width: 13, Align: widget.AlignRight}},
		{holdingsColPercent, widget.Column{Header: "% Total", Width: 7, Align: widget.AlignRight}},
		{holdingsColCost, widget.Column{Header: "Cost Basis", Width: 13, Align: widget.AlignRight}},
		{holdingsColGain, widget.Column{Header: "Gain", Width: 12, Align: widget.AlignRight}},
		{holdingsColBar, widget.Column{}},
	}
	dropped := func(c holdingsCol) bool {
		switch c {
		case holdingsColShares, holdingsColPrice:
			return width < holdingsWidthAll
		case holdingsColCost, holdingsColGain:
			return width < holdingsWidthCost
		case holdingsColName:
			return width < holdingsWidthName
		}
		return false
	}

	var cols []holdingsColumn
	spare := width
	for _, c := range all {
		if !dropped(c.col) {
			cols = append(cols, c)
			spare -= c.Width
		}
	}
	spare -= len(cols) - 1 // one separator cell between columns
	for i := range cols {
		if cols[i].col == holdingsColName {
			cols[i].Width = max(holdingsMinName, min(longestName, spare/2))
			spare -= cols[i].Width
		}
	}
	for i := range cols {
		if cols[i].col == holdingsColBar {
			cols[i].Width = max(min(spare, holdingsMaxBar), 1)
		}
	}
	return cols
}

// holdingRef is the report row behind a row of the holdings table.
type holdingRef struct {
	section, row int
}

// holdingsViewState is the holdings report's part of the Reports view: its
// table, the report row behind each table row, and the split of one row when
// that is open.
type holdingsViewState struct {
	table   *widget.Table
	refs    []holdingRef
	columns []holdingsColumn
	width   int // the table width the rows were laid out for
	// pageRows is the number of data rows on screen at the last render, the
	// step of PgUp and PgDn.
	pageRows int
	split    *widget.Table // the open split, or nil
	splitRef holdingRef
	clicks   *widget.ClickTracker
}

// holdingsKey is what a holdings row is across a reload: its security in its
// currency, or the Cash row of its currency.
type holdingsKey struct {
	currency string
	security types.ID
	cash     bool
}

// holdingsDefaultWidth is the width the rows are first laid out for, before
// a render knows the real one.
const holdingsDefaultWidth = holdingsWidthAll

// setData stores loaded report data. A holdings report gets a new table; the
// cursor stays on the row it was on, by security, and an open split closes.
func (s *reportsViewState) setData(data *reportsViewData) {
	keep, hadKeep := s.selectedHoldingKey()
	s.data = data
	width := s.holdings.width
	s.holdings = holdingsViewState{clicks: s.holdings.clicks}
	rpt := s.holdingsReport()
	if rpt == nil {
		return
	}
	for i, sec := range rpt.Sections {
		for j := range sec.Rows {
			s.holdings.refs = append(s.holdings.refs, holdingRef{section: i, row: j})
		}
	}
	s.holdings.table = widget.NewTable(nil)
	if width == 0 {
		width = holdingsDefaultWidth
	}
	s.layoutHoldings(width)
	if hadKeep {
		for i, ref := range s.holdings.refs {
			if holdingKeyOf(rpt, ref) == keep {
				s.holdings.table.SetCursor(i)
				break
			}
		}
	}
}

// holdingsReport is the holdings report on screen, or nil.
func (s *reportsViewState) holdingsReport() *report.Holdings {
	if s.data == nil || s.data.rtype != reportTypeHoldings {
		return nil
	}
	return s.data.holdings
}

func holdingKeyOf(rpt *report.Holdings, ref holdingRef) holdingsKey {
	sec := rpt.Sections[ref.section]
	row := sec.Rows[ref.row]
	return holdingsKey{currency: sec.Currency, security: row.SecurityID, cash: row.Cash}
}

// selectedHoldingKey is the row under the holdings cursor, if any.
func (s *reportsViewState) selectedHoldingKey() (holdingsKey, bool) {
	ref, ok := s.selectedHoldingRef()
	if !ok {
		return holdingsKey{}, false
	}
	return holdingKeyOf(s.data.holdings, ref), true
}

func (s *reportsViewState) selectedHoldingRef() (holdingRef, bool) {
	if s.holdingsReport() == nil || s.holdings.table == nil {
		return holdingRef{}, false
	}
	c := s.holdings.table.Cursor()
	if c < 0 || c >= len(s.holdings.refs) {
		return holdingRef{}, false
	}
	return s.holdings.refs[c], true
}

// layoutHoldings sets the table's columns and rows for a table width.
func (s *reportsViewState) layoutHoldings(width int) {
	rpt := s.data.holdings
	longest := 0
	for _, sec := range rpt.Sections {
		for _, row := range sec.Rows {
			longest = max(longest, utf8.RuneCountInString(row.Name))
		}
	}
	h := &s.holdings
	h.width = width
	h.columns = holdingsColumns(width, longest)
	cols := make([]widget.Column, len(h.columns))
	for i, c := range h.columns {
		cols[i] = c.Column
	}
	rows := make([][]string, len(h.refs))
	for i, ref := range h.refs {
		sec := rpt.Sections[ref.section]
		rows[i] = holdingsCells(h.columns, sec, sec.Rows[ref.row])
	}
	cursor := h.table.Cursor()
	h.table.SetColumns(cols)
	h.table.SetRows(rows)
	h.table.SetCursor(cursor)
}

// holdingsCells is one row of the table, in the order of cols.
func holdingsCells(cols []holdingsColumn, sec report.HoldingsSection, row report.HoldingRow) []string {
	largest := types.ZeroMoney
	if len(sec.Rows) > 0 {
		largest = sec.Rows[0].Value
	}
	money := func(m types.Money) string { return formatDashboardMoneyIn(m, sec.Currency) }
	cells := make([]string, len(cols))
	for i, c := range cols {
		switch c.col {
		case holdingsColSecurity:
			cells[i] = row.Label
			if row.Estimated {
				cells[i] = "~" + row.Label
			}
		case holdingsColName:
			cells[i] = row.Name
		case holdingsColShares:
			if !row.Cash {
				cells[i] = row.Shares.String()
			}
		case holdingsColPrice:
			switch {
			case row.Cash:
			case row.Estimated:
				cells[i] = "N/A"
			default:
				cells[i] = money(row.Price)
			}
		case holdingsColValue:
			cells[i] = money(row.Value)
		case holdingsColPercent:
			cells[i] = fmt.Sprintf("%.1f%%", row.Percent)
		case holdingsColCost:
			cells[i] = money(row.CostBasis)
		case holdingsColGain:
			switch {
			case row.Cash:
			case row.Estimated:
				cells[i] = "N/A"
			default:
				cells[i] = money(row.Gain)
			}
		case holdingsColBar:
			cells[i] = report.Bar(row.Value, largest, c.Width)
		}
	}
	return cells
}

// holdingsTableOnScreen is the table the mouse and the wheel address in the
// holdings report: the split when it is open, else the holdings table. It is
// nil for the other reports.
func (s *reportsViewState) holdingsTableOnScreen() *widget.Table {
	if s.holdingsReport() == nil {
		return nil
	}
	if s.holdings.split != nil {
		return s.holdings.split
	}
	return s.holdings.table
}

// splitOpen reports whether a holdings row's split is on screen.
func (s *reportsViewState) splitOpen() bool {
	return s.holdingsReport() != nil && s.holdings.split != nil
}

// openSplit shows how the row under the cursor splits across the accounts.
func (s *reportsViewState) openSplit() {
	ref, ok := s.selectedHoldingRef()
	if !ok {
		return
	}
	row := s.data.holdings.Sections[ref.section].Rows[ref.row]
	s.holdings.splitRef = ref
	s.holdings.split = widget.NewTable(splitColumns(s.holdings.width, row.Cash))
	rows := make([][]string, len(row.Accounts))
	for i, a := range row.Accounts {
		rows[i] = splitCells(row, a.Name, a.Shares, a.Value, a.Percent, s.data.holdings.Sections[ref.section].Currency)
	}
	s.holdings.split.SetRows(rows)
}

// closeSplit goes back from a split to the holdings table.
func (s *reportsViewState) closeSplit() {
	s.holdings.split = nil
}

// clickHoldingsRow takes a click on a row of the table on screen; the cursor
// is already on it. A second click on the same holdings row opens its split.
func (s *reportsViewState) clickHoldingsRow(row int) {
	if s.holdingsReport() == nil || s.holdings.split != nil {
		return
	}
	if s.holdings.clicks == nil {
		s.holdings.clicks = widget.NewClickTracker(widget.DoubleClickThreshold)
	}
	if s.holdings.clicks.Click(row) {
		s.openSplit()
	}
}

// splitColumns are the columns of a split, all fixed: the Cash row's split
// has no Shares column.
func splitColumns(width int, cash bool) []widget.Column {
	cols := []widget.Column{
		{Header: "Account"},
		{Header: "Shares", Width: 12, Align: widget.AlignRight},
		{Header: "Value", Width: 13, Align: widget.AlignRight},
		{Header: "% of Holding", Width: 12, Align: widget.AlignRight},
	}
	if cash {
		cols = append(cols[:1], cols[2:]...)
	}
	account := width - (len(cols) - 1)
	for _, c := range cols[1:] {
		account -= c.Width
	}
	cols[0].Width = max(account, 10)
	return cols
}

// splitCells is one line of a split, in the order of splitColumns.
func splitCells(row report.HoldingRow, name string, shares types.Quantity, value types.Money, percent float64, currency string) []string {
	cells := []string{name}
	if !row.Cash {
		cells = append(cells, shares.String())
	}
	return append(cells, formatDashboardMoneyIn(value, currency), fmt.Sprintf("%.1f%%", percent))
}

// handleHoldingsKey handles the keys that belong to the holdings report and
// reports whether it took the key. The report switches (n, s, i) are left to
// handleKey.
func (s *reportsViewState) handleHoldingsKey(msg tea.KeyPressMsg, keys keyMap) bool {
	tbl := s.holdingsTableOnScreen()
	if tbl == nil {
		return false
	}
	page := max(s.holdings.pageRows, 1)
	switch {
	case key.Matches(msg, keys.Up):
		tbl.MoveUp()
	case key.Matches(msg, keys.Down):
		tbl.MoveDown()
	case msg.String() == "home" || msg.String() == "g":
		tbl.MoveToTop()
	case msg.String() == "end" || msg.String() == "G":
		tbl.MoveToBottom()
	case msg.String() == "pgup":
		tbl.PageUp(page)
	case msg.String() == "pgdown":
		tbl.PageDown(page)
	case key.Matches(msg, keys.Enter):
		if s.holdings.split == nil {
			s.openSplit()
		}
	case key.Matches(msg, keys.Escape):
		if s.holdings.split == nil {
			return false
		}
		s.closeSplit()
	default:
		return false
	}
	return true
}

// holdingsHints is the status bar's key hints for the holdings report.
func (s *reportsViewState) holdingsHints() string {
	if s.splitOpen() {
		return "↑↓ navigate  esc back to holdings"
	}
	return "↑↓ navigate  enter accounts  n net worth  s spending  esc back"
}

// The app's own rows around the view: its header line and its status bar.
const appChromeRows = 2

// renderHoldings renders the holdings report, or the split of one row when
// it is open. height is the terminal height.
func (s *reportsViewState) renderHoldings(styles widget.Styles, height int) string {
	rpt := s.data.holdings
	if rpt == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("No holdings data available.")
	}
	contentWidth := styles.ContentWidth()
	tableWidth := max(contentWidth-4, 1)
	if s.holdings.width != tableWidth {
		s.layoutHoldings(tableWidth)
		if s.holdings.split != nil {
			s.openSplit()
		}
	}
	if s.holdings.split != nil {
		return s.renderHoldingsSplit(styles, height, tableWidth)
	}

	sections := []string{
		reportTitleRow(styles, "HOLDINGS", "As of: "+rpt.AsOfDate.Time().Format("Jan 2, 2006"), contentWidth),
		styles.Muted.Render(strings.Repeat("═", tableWidth)),
	}
	hint := styles.Muted.Render("  enter accounts  n net worth  s spending  i holdings  esc back")
	if len(rpt.Sections) == 0 && len(rpt.Failed) == 0 {
		sections = append(sections, "",
			styles.Muted.Render("  No investment accounts. Add one to see holdings."),
			"", hint)
		return lipgloss.NewStyle().Padding(1, 2).Render(strings.Join(sections, "\n"))
	}

	var footer []string
	footer = append(footer, styles.Muted.Render(strings.Repeat("─", tableWidth)))
	estimated := false
	for _, sec := range rpt.Sections {
		footer = append(footer, s.holdingsTotalLine(styles, sec, len(rpt.Sections) > 1))
		estimated = estimated || sec.Estimated
	}
	if estimated {
		footer = append(footer, styles.Muted.Render("~ No price on file: the value is the cost basis."))
	}
	for _, f := range rpt.Failed {
		footer = append(footer, styles.Error.Render(fmt.Sprintf("%s: could not be valued (%v)", f.Name, f.Err)))
	}
	if len(rpt.Failed) > 0 {
		footer = append(footer, styles.Muted.Render("Percentages leave out accounts that could not be valued."))
	}
	footer = append(footer, "", hint)

	// Padding (2), title and separator (2), and the footer stay on screen;
	// the table gets the rest, and only its rows scroll.
	avail := max(height-appChromeRows-2-2-len(footer), 3)
	tbl := s.holdings.table
	if rows := tbl.RowCount(); rows > 0 {
		dataRows := avail - 2 // the header and its border
		if rows > dataRows {
			dataRows-- // the scroll info line
		}
		dataRows = max(min(dataRows, rows), 1)
		s.holdings.pageRows = dataRows
		sections = append(sections, tbl.Render(styles, tableWidth, dataRows+2))
		if info := tbl.ScrollInfo(dataRows); info != "" {
			sections = append(sections, styles.Muted.Render(info))
		}
	}
	sections = append(sections, footer...)
	return lipgloss.NewStyle().Padding(1, 2).Render(strings.Join(sections, "\n"))
}

// holdingsTotalLine is a section's TOTAL line, laid out in the table's
// columns. The label spans Security and Name, so "TOTAL USD" fits.
func (s *reportsViewState) holdingsTotalLine(styles widget.Styles, sec report.HoldingsSection, withCurrency bool) string {
	label := "TOTAL"
	if withCurrency {
		label += " " + sec.Currency
	}
	money := func(m types.Money) string { return formatDashboardMoneyIn(m, sec.Currency) }
	value := "not available"
	if sec.Available {
		value = money(sec.Value)
		if sec.Estimated {
			value = "~" + value
		}
	}

	var cells []string
	labelWidth := 0
	for i, c := range s.holdings.columns {
		switch c.col {
		case holdingsColSecurity, holdingsColName:
			if labelWidth > 0 {
				labelWidth++ // the separator the label spans
			}
			labelWidth += c.Width
			if i+1 < len(s.holdings.columns) && s.holdings.columns[i+1].col == holdingsColName {
				continue
			}
			cells = append(cells, styles.Bold.Render(widget.AlignText(label, labelWidth, widget.AlignLeft)))
		case holdingsColValue:
			cells = append(cells, styles.Bold.Render(widget.AlignText(value, c.Width, c.Align)))
		case holdingsColPercent:
			pct := ""
			if sec.Available {
				pct = "100.0%"
			}
			cells = append(cells, widget.AlignText(pct, c.Width, c.Align))
		case holdingsColCost:
			cost := ""
			if sec.Available {
				cost = money(sec.CostBasis)
			}
			cells = append(cells, widget.AlignText(cost, c.Width, c.Align))
		case holdingsColGain:
			gain := ""
			if sec.Available {
				gain = money(sec.Gain)
			}
			cell := widget.AlignText(gain, c.Width, c.Align)
			switch {
			case !sec.Available:
			case sec.Gain.IsNegative():
				cell = styles.Negative.Render(cell)
			case sec.Gain.IsPositive():
				cell = styles.Positive.Render(cell)
			}
			cells = append(cells, cell)
		case holdingsColBar:
		default:
			cells = append(cells, strings.Repeat(" ", c.Width))
		}
	}
	return strings.TrimRight(strings.Join(cells, " "), " ")
}

// renderHoldingsSplit renders how one holdings row splits across the
// accounts.
func (s *reportsViewState) renderHoldingsSplit(styles widget.Styles, height, tableWidth int) string {
	ref := s.holdings.splitRef
	sec := s.data.holdings.Sections[ref.section]
	row := sec.Rows[ref.row]

	title := row.Label
	if row.Name != "" && row.Name != row.Label {
		title += "  " + row.Name
	}
	share := fmt.Sprintf("%s  %.1f%% of total", formatDashboardMoneyIn(row.Value, sec.Currency), row.Percent)
	sections := []string{
		reportTitleRow(styles, title, share, tableWidth+4),
		styles.Muted.Render(strings.Repeat("═", tableWidth)),
	}

	footer := []string{
		styles.Muted.Render(strings.Repeat("─", tableWidth)),
		styles.Bold.Render(strings.TrimRight(alignedLine(s.holdings.split.Columns(),
			splitCells(row, "Total", row.Shares, row.Value, 100, sec.Currency)), " ")),
		"",
		styles.Muted.Render("  esc back to holdings"),
	}
	avail := max(height-appChromeRows-2-2-len(footer), 3)
	tbl := s.holdings.split
	dataRows := avail - 2
	if tbl.RowCount() > dataRows {
		dataRows--
	}
	dataRows = max(min(dataRows, tbl.RowCount()), 1)
	s.holdings.pageRows = dataRows
	sections = append(sections, tbl.Render(styles, tableWidth, dataRows+2))
	if info := tbl.ScrollInfo(dataRows); info != "" {
		sections = append(sections, styles.Muted.Render(info))
	}
	sections = append(sections, footer...)
	return lipgloss.NewStyle().Padding(1, 2).Render(strings.Join(sections, "\n"))
}

// alignedLine lays cells out in fixed-width columns, as a table row is.
func alignedLine(cols []widget.Column, cells []string) string {
	out := make([]string, len(cols))
	for i, c := range cols {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		out[i] = widget.AlignText(cell, c.Width, c.Align)
	}
	return strings.Join(out, " ")
}

// reportTitleRow is a report's title on the left and a muted note on the
// right, in one line of contentWidth cells less the view's padding.
func reportTitleRow(styles widget.Styles, title, note string, contentWidth int) string {
	gap := max(contentWidth-lipgloss.Width(title)-lipgloss.Width(note)-4, 1)
	return styles.Title.Render(title) + strings.Repeat(" ", gap) + styles.Muted.Render(note)
}
