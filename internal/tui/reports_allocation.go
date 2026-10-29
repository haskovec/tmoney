package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haskovec/tmoney/internal/chart"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// allocationCol names a column of the allocation table.
type allocationCol int

const (
	allocationColClass allocationCol = iota
	allocationColCount
	allocationColValue
	allocationColPercent
	allocationColBar
)

// allocationColumn is one column of the allocation table on screen.
type allocationColumn struct {
	col allocationCol
	widget.Column
}

// The table width below which the Holdings column goes, and the limits of
// the bar. See specs/design-asset-allocation-report.md §5.5.2.
const (
	allocationWidthAll = 61
	allocationMinBar   = 10
	allocationMaxBar   = 40
)

// allocationColumns returns the columns for a table of width cells. The bar
// gets the spare width, from allocationMinBar to allocationMaxBar; below
// allocationWidthAll the Holdings column goes. A table still too wide is cut
// on the right, as other tables are.
func allocationColumns(width int) []allocationColumn {
	all := []allocationColumn{
		{allocationColClass, widget.Column{Header: "Asset Class", Width: 19}},
		{allocationColCount, widget.Column{Header: "Holdings", Width: 8, Align: widget.AlignRight}},
		{allocationColValue, widget.Column{Header: "Value", Width: 13, Align: widget.AlignRight}},
		{allocationColPercent, widget.Column{Header: "% Total", Width: 7, Align: widget.AlignRight}},
		{allocationColBar, widget.Column{}},
	}
	var cols []allocationColumn
	spare := width
	for _, c := range all {
		if c.col == allocationColCount && width < allocationWidthAll {
			continue
		}
		cols = append(cols, c)
		spare -= c.Width
	}
	spare -= len(cols) - 1 // one separator cell between columns
	cols[len(cols)-1].Width = max(min(spare, allocationMaxBar), allocationMinBar)
	return cols
}

// drillRef is the holding behind a row of the class drill-down: one of the
// class's priced Holdings, or one of its NoPrice rows.
type drillRef struct {
	noPrice bool
	index   int
}

// allocationViewState is the allocation report's part of the Reports view:
// the class table of one currency, the holdings of one class when that is
// open, and the account split of one of those holdings when that is open.
type allocationViewState struct {
	section int // the index of the currency on screen
	table   *widget.Table
	classes []security.AssetClass // the class behind each row of table
	columns []allocationColumn
	width   int  // the table width the rows were laid out for
	dark    bool // the class colors the rows were laid out with
	// pageRows is the number of data rows on screen at the last render, the
	// step of PgUp and PgDn.
	pageRows   int
	drill      *widget.Table // the holdings of drillClass, or nil
	drillClass security.AssetClass
	drillRefs  []drillRef
	split      *widget.Table // the open split, or nil
	splitRef   drillRef
	clicks     *widget.ClickTracker
	// editRequest numbers the e press that waits for its security. A close
	// of the drill-down and a reload change it, so a late reply opens no
	// dialog.
	editRequest int
}

// allocationPlace is where the allocation view is, by what is on screen
// and not by row index, so that a reload keeps it.
type allocationPlace struct {
	currency   string
	class      security.AssetClass // the class under the cursor
	classRow   int
	drill      bool
	drillClass security.AssetClass
	drillRow   int
	drillKey   holdingsKey
}

// allocationReport is the allocation report on screen, or nil.
func (s *reportsViewState) allocationReport() *report.Allocation {
	if s.data == nil || s.data.rtype != reportTypeAllocation {
		return nil
	}
	return s.data.allocation
}

// allocationSection is the section of the currency on screen.
func (s *reportsViewState) allocationSection() (report.AllocationSection, bool) {
	rpt := s.allocationReport()
	if rpt == nil || s.allocation.section >= len(rpt.Sections) {
		return report.AllocationSection{}, false
	}
	return rpt.Sections[s.allocation.section], true
}

// allocationPlace returns where the allocation view is, if it is on screen.
func (s *reportsViewState) allocationPlace() (allocationPlace, bool) {
	sec, ok := s.allocationSection()
	a := &s.allocation
	if !ok || a.table == nil {
		return allocationPlace{}, false
	}
	p := allocationPlace{currency: sec.Currency, classRow: a.table.Cursor()}
	if c := a.table.Cursor(); c >= 0 && c < len(a.classes) {
		p.class = a.classes[c]
	}
	if a.drill != nil {
		p.drill, p.drillClass, p.drillRow = true, a.drillClass, a.drill.Cursor()
		if row, ok := s.drillHolding(p.drillRow); ok {
			p.drillKey = holdingsKey{currency: sec.Currency, security: row.SecurityID, cash: row.Cash}
		}
	}
	return p, true
}

// setAllocation lays out newly loaded data. The view stays on the currency,
// the class and the drill-down it was on (C10): the drill-down cursor stays
// on its holding, or at its position when the holding left the class, and
// the drill-down closes when the class is now empty. An open split closes.
func (s *reportsViewState) setAllocation(p allocationPlace, kept bool) {
	width, dark := s.allocation.width, s.allocation.dark
	s.allocation = allocationViewState{clicks: s.allocation.clicks, dark: dark,
		editRequest: s.allocation.editRequest + 1}
	rpt := s.allocationReport()
	if rpt == nil {
		return
	}
	a := &s.allocation
	if kept {
		for i, sec := range rpt.Sections {
			if sec.Currency == p.currency {
				a.section = i
			}
		}
	}
	a.table = widget.NewTable(nil)
	if width == 0 {
		width = allocationWidthAll
	}
	s.layoutAllocation(width, dark)
	if !kept {
		return
	}
	a.table.SetCursor(p.classRow)
	for i, ac := range a.classes {
		if ac == p.class {
			a.table.SetCursor(i)
		}
	}
	if !p.drill {
		return
	}
	for i, ac := range a.classes {
		if ac != p.drillClass {
			continue
		}
		a.table.SetCursor(i)
		s.openDrill()
		a.drill.SetCursor(p.drillRow)
		for j := range a.drillRefs {
			if row, ok := s.drillHolding(j); ok && row.SecurityID == p.drillKey.security && row.Cash == p.drillKey.cash {
				a.drill.SetCursor(j)
			}
		}
	}
}

// layoutAllocation sets the class table's columns and rows for a table
// width, with the bars in the light or the dark class colors.
func (s *reportsViewState) layoutAllocation(width int, dark bool) {
	a := &s.allocation
	a.width, a.dark = width, dark
	a.columns = allocationColumns(width)
	cols := make([]widget.Column, len(a.columns))
	for i, c := range a.columns {
		cols[i] = c.Column
	}
	sec, _ := s.allocationSection()
	shown := sec.Shown()
	largest := types.ZeroMoney
	for _, c := range shown {
		if c.Value.Cmp(largest) > 0 {
			largest = c.Value
		}
	}
	a.classes = make([]security.AssetClass, len(shown))
	rows := make([][]string, len(shown))
	for i, c := range shown {
		a.classes[i] = c.Class
		rows[i] = allocationCells(a.columns, c, sec.Currency, largest)
	}
	cursor := a.table.Cursor()
	a.table.SetColumns(cols)
	a.table.SetRows(rows)
	a.table.SetCursor(cursor)
	bar := len(a.columns) - 1
	for i, ac := range a.classes {
		a.table.SetCellColor(i, bar, chart.ClassColor(ac, dark))
	}
}

// allocationCells is one class row, in the order of cols.
func allocationCells(cols []allocationColumn, c report.ClassRow, currency string, largest types.Money) []string {
	cells := make([]string, len(cols))
	for i, col := range cols {
		switch col.col {
		case allocationColClass:
			cells[i] = c.Class.DisplayName()
		case allocationColCount:
			cells[i] = strconv.Itoa(c.Count())
		case allocationColValue:
			cells[i] = formatDashboardMoneyIn(c.Value, currency)
		case allocationColPercent:
			cells[i] = fmt.Sprintf("%.1f%%", c.Percent)
		case allocationColBar:
			cells[i] = report.Bar(c.Value, largest, col.Width)
		}
	}
	return cells
}

// changeCurrency shows the next (+1) or the previous (-1) currency, if
// there is one.
func (s *reportsViewState) changeCurrency(step int) {
	rpt := s.allocationReport()
	a := &s.allocation
	next := a.section + step
	if rpt == nil || next < 0 || next >= len(rpt.Sections) {
		return
	}
	a.section = next
	a.table.SetCursor(0)
	s.layoutAllocation(a.width, a.dark)
}

// drillColumns are the columns of the class drill-down. Name gets the
// width that the fixed columns leave.
func drillColumns(width int) []widget.Column {
	cols := []widget.Column{
		{Header: "Security", Width: 8},
		{Header: "Name"},
		{Header: "Value", Width: 13, Align: widget.AlignRight},
		{Header: "% of Class", Width: 10, Align: widget.AlignRight},
		{Header: "% Total", Width: 7, Align: widget.AlignRight},
	}
	name := width - (len(cols) - 1)
	for _, c := range cols {
		name -= c.Width
	}
	cols[1].Width = max(name, 10)
	return cols
}

// openDrill opens the holdings of the class under the cursor.
func (s *reportsViewState) openDrill() {
	a := &s.allocation
	c := a.table.Cursor()
	if c < 0 || c >= len(a.classes) {
		return
	}
	sec, _ := s.allocationSection()
	class, ok := sec.Class(a.classes[c])
	if !ok {
		return
	}
	pct := func(p float64) string { return fmt.Sprintf("%.1f%%", p) }
	a.drillClass = class.Class
	a.drillRefs = nil
	var rows [][]string
	for i, h := range class.Holdings {
		a.drillRefs = append(a.drillRefs, drillRef{index: i})
		rows = append(rows, []string{h.Label, h.Name, formatDashboardMoneyIn(h.Value, sec.Currency),
			pct(h.PercentOfClass), pct(h.PercentOfTotal)})
	}
	for i, row := range class.NoPrice {
		a.drillRefs = append(a.drillRefs, drillRef{noPrice: true, index: i})
		rows = append(rows, []string{row.Label, row.Name, "no price", "", ""})
	}
	a.drill = widget.NewTable(drillColumns(a.width))
	a.drill.SetRows(rows)
}

// closeDrill goes back from the drill-down, or its split, to the class
// table. The class table's cursor is still on the class.
func (s *reportsViewState) closeDrill() {
	s.allocation.drill = nil
	s.allocation.split = nil
	s.allocation.editRequest++
}

// drillHolding is the holding behind row i of the drill-down.
func (s *reportsViewState) drillHolding(i int) (report.HoldingRow, bool) {
	a := &s.allocation
	if a.drill == nil || i < 0 || i >= len(a.drillRefs) {
		return report.HoldingRow{}, false
	}
	return s.drillRefHolding(a.drillRefs[i])
}

func (s *reportsViewState) drillRefHolding(ref drillRef) (report.HoldingRow, bool) {
	sec, _ := s.allocationSection()
	class, ok := sec.Class(s.allocation.drillClass)
	if !ok {
		return report.HoldingRow{}, false
	}
	if ref.noPrice {
		return class.NoPrice[ref.index], true
	}
	return class.Holdings[ref.index].HoldingRow, true
}

// openAllocationSplit shows how the holding under the drill-down cursor
// splits across the accounts.
func (s *reportsViewState) openAllocationSplit() {
	a := &s.allocation
	c := a.drill.Cursor()
	row, ok := s.drillHolding(c)
	if !ok {
		return
	}
	sec, _ := s.allocationSection()
	a.splitRef = a.drillRefs[c]
	a.split = widget.NewTable(splitColumns(a.width, row.Cash))
	rows := make([][]string, len(row.Accounts))
	for i, acct := range row.Accounts {
		rows[i] = splitCells(row, acct.Name, acct.Shares, acct.Value, acct.Percent, sec.Currency)
	}
	a.split.SetRows(rows)
}

// allocationTableOnScreen is the table the mouse and the wheel address in
// the allocation report: the split, the drill-down, or the class table. It
// is nil for the other reports.
func (s *reportsViewState) allocationTableOnScreen() *widget.Table {
	if s.allocationReport() == nil {
		return nil
	}
	a := &s.allocation
	switch {
	case a.split != nil:
		return a.split
	case a.drill != nil:
		return a.drill
	}
	return a.table
}

// drillOpen reports whether a class drill-down, or its split, is on screen.
func (s *reportsViewState) drillOpen() bool {
	return s.allocationReport() != nil && s.allocation.drill != nil
}

// clickAllocationRow takes a click on a row of the table on screen; the
// cursor is already on it. A second click on the same row opens it: a class
// opens its holdings, and a holding its split.
func (s *reportsViewState) clickAllocationRow(row int) {
	a := &s.allocation
	if s.allocationReport() == nil || a.split != nil {
		return
	}
	if a.clicks == nil {
		a.clicks = widget.NewClickTracker(widget.DoubleClickThreshold)
	}
	if !a.clicks.Click(row) {
		return
	}
	if a.drill != nil {
		s.openAllocationSplit()
	} else {
		s.openDrill()
	}
}

// reportsEditSecurityMsg carries a security that `e` in the class
// drill-down loaded for the Edit Security dialog.
type reportsEditSecurityMsg struct {
	sec     *security.Security
	request int // the editRequest of the e press
	fileOf
}

// editDrillSecurity loads the security under the drill-down cursor for the
// Edit Security dialog. The uninvested cash is not a security.
func (s *reportsViewState) editDrillSecurity(d reportsDeps) tea.Cmd {
	row, ok := s.drillHolding(s.allocation.drill.Cursor())
	if !ok || row.Cash {
		return nil
	}
	id := row.SecurityID
	file := d.file()
	s.allocation.editRequest++
	request := s.allocation.editRequest
	return func() tea.Msg {
		securities := d.securities()
		if securities == nil {
			return nil
		}
		sec, err := securities.GetByID(id)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to load security: %w", err), fileOf: file}
		}
		return reportsEditSecurityMsg{sec: sec, request: request, fileOf: file}
	}
}

// takeEditRequest reports whether request is the e press that waits for its
// security, with its drill-down still open, and ends it.
func (s *reportsViewState) takeEditRequest(request int) bool {
	if !s.drillOpen() || request != s.allocation.editRequest {
		return false
	}
	s.allocation.editRequest++
	return true
}

// handleAllocationKey handles the keys that belong to the allocation report
// and reports whether it took the key. The report switches (n, s, i, a) are
// left to handleKey.
func (s *reportsViewState) handleAllocationKey(d reportsDeps, msg tea.KeyPressMsg, keys keyMap) (bool, tea.Cmd) {
	tbl := s.allocationTableOnScreen()
	if tbl == nil {
		return false, nil
	}
	a := &s.allocation
	page := max(a.pageRows, 1)
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
	case key.Matches(msg, keys.Left):
		if a.drill == nil {
			s.changeCurrency(-1)
		}
	case key.Matches(msg, keys.Right):
		if a.drill == nil {
			s.changeCurrency(1)
		}
	case key.Matches(msg, keys.Enter):
		switch {
		case a.split != nil:
		case a.drill != nil:
			s.openAllocationSplit()
		default:
			s.openDrill()
		}
	case key.Matches(msg, keys.Edit):
		if a.drill == nil || a.split != nil {
			return false, nil
		}
		return true, s.editDrillSecurity(d)
	case key.Matches(msg, keys.Escape):
		switch {
		case a.split != nil:
			a.split = nil
		case a.drill != nil:
			s.closeDrill()
		default:
			return false, nil
		}
	default:
		return false, nil
	}
	return true, nil
}

// allocationHints is the status bar's key hints for the allocation report.
func (s *reportsViewState) allocationHints() string {
	a := &s.allocation
	switch {
	case a.split != nil:
		return "↑↓ navigate  esc back to class"
	case a.drill != nil:
		return "↑↓ navigate  enter accounts  e edit security  esc back to allocation"
	}
	currency := ""
	if rpt := s.allocationReport(); rpt != nil && len(rpt.Sections) > 1 {
		currency = "  ←→ currency"
	}
	return "↑↓ navigate  enter holdings" + currency + "  n net worth  s spending  i holdings  esc back"
}

// renderAllocation renders the allocation report: the class table of one
// currency, or the drill-down or the split when one is open. height is the
// terminal height; dark selects the class colors.
func (s *reportsViewState) renderAllocation(styles widget.Styles, height int, dark bool) string {
	rpt := s.data.allocation
	if rpt == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("No allocation data available.")
	}
	a := &s.allocation
	tableWidth := max(styles.ContentWidth()-4, 1)
	if a.width != tableWidth || a.dark != dark {
		s.layoutAllocation(tableWidth, dark)
		// Only the widths change, so the drill-down and the split keep their
		// cursors.
		if a.drill != nil {
			a.drill.SetColumns(drillColumns(tableWidth))
		}
		if a.split != nil {
			if row, ok := s.drillRefHolding(a.splitRef); ok {
				a.split.SetColumns(splitColumns(tableWidth, row.Cash))
			}
		}
	}
	sec, ok := s.allocationSection()
	switch {
	case ok && a.split != nil:
		return s.renderAllocationSplit(styles, height, tableWidth, sec)
	case ok && a.drill != nil:
		return s.renderAllocationDrill(styles, height, tableWidth, sec, dark)
	}

	title := "ASSET ALLOCATION"
	if len(rpt.Sections) > 1 {
		title += fmt.Sprintf("   %s (%d of %d)", sec.Currency, a.section+1, len(rpt.Sections))
	}
	sections := []string{
		reportTitleRow(styles, title, "As of: "+rpt.AsOfDate.Time().Format("Jan 2, 2006"), tableWidth),
		styles.Muted.Render(strings.Repeat("═", tableWidth)),
	}
	// The hint leaves out whole hints to fit, the current view's first;
	// "esc back" and "enter holdings" stay longest.
	hints := []string{"enter holdings", "n net worth", "s spending", "i holdings", "a allocation", "esc back"}
	drop := []int{4, 3, 2, 1, 0}
	if len(rpt.Sections) > 1 {
		hints = append(hints[:1], append([]string{"←→ currency"}, hints[1:]...)...)
		drop = []int{5, 4, 3, 2, 1, 0}
	}
	hint := styles.Muted.Render("  " + widget.FitParts(hints, "  ", tableWidth-2, drop))
	if !ok {
		empty := "  No holdings in the investment accounts."
		if rpt.AccountCount == 0 {
			empty = "  No investment accounts. Add one to see the allocation."
		}
		sections = append(sections, "", styles.Muted.Render(empty), "", hint)
		return lipgloss.NewStyle().Padding(1, 2).Render(strings.Join(sections, "\n"))
	}

	footer := []string{
		styles.Muted.Render(strings.Repeat("─", tableWidth)),
		s.allocationTotalLine(styles, sec),
	}
	for _, f := range rpt.Failed {
		if f.Currency == sec.Currency {
			footer = append(footer, styles.Error.UnsetPadding().Render(fmt.Sprintf("%s: could not be valued (%v)", f.Name, f.Err)))
		}
	}
	if !sec.Available {
		footer = append(footer, styles.Muted.Render("Percentages leave out accounts that could not be valued."))
	}
	var after []string
	if c, ok := sec.Class(security.AssetClassUnclassified); ok {
		if note := report.UnclassifiedNote(c.Count()); note != "" {
			after = append(after, mutedNote(styles, note+" Press enter on Unclassified, then e to set a class.", tableWidth)...)
		}
	}
	after = append(after, "", hint)
	// A class whose holdings all have no price has no row, so the short
	// note sends the owner to the holdings report for the names.
	names := sec.NoPriceNames()
	footer = append(footer, fitNoPriceNote(styles, names,
		shortNoPriceNote(len(names), "Press i to see %s in the holdings report."),
		tableWidth, footerRoom(height)-len(footer)-len(after))...)
	footer = append(footer, after...)
	sections = append(sections, s.renderAllocationTable(styles, height, tableWidth, a.table, len(footer))...)
	sections = append(sections, footer...)
	return lipgloss.NewStyle().Padding(1, 2).Render(strings.Join(sections, "\n"))
}

// allocationTableMin is the fewest lines a table gets: the header, its
// border, one row and the scroll line.
const allocationTableMin = 4

// footerRoom is the number of footer lines that leave the table on screen
// its allocationTableMin lines.
func footerRoom(height int) int {
	return height - appChromeRows - 2 - 2 - allocationTableMin
}

// fitNoPriceNote is the note that names the holdings with no price, wrapped
// to width, or short when the names take more than room lines. The footer
// does not scroll, and the screen cuts the last lines of a footer that is
// too tall.
func fitNoPriceNote(styles widget.Styles, names []string, short string, width, room int) []string {
	lines := mutedNote(styles, report.NoPriceNote(names), width)
	if len(lines) > max(room, 1) {
		return mutedNote(styles, short, width)
	}
	return lines
}

// shortNoPriceNote counts the holdings with no price in place of naming
// them; then, with "it" or "them" for its %s, tells where the names are.
func shortNoPriceNote(n int, then string) string {
	if n == 1 {
		return "Not included (no price): 1 holding. " + fmt.Sprintf(then, "it")
	}
	return fmt.Sprintf("Not included (no price): %d holdings. ", n) + fmt.Sprintf(then, "them")
}

// mutedNote is a note wrapped to width, one muted line per element, so
// that the footer counts each line; none for an empty note. A note can name
// many holdings, and a cut line would lose them.
func mutedNote(styles widget.Styles, note string, width int) []string {
	if note == "" {
		return nil
	}
	var out []string
	for line := range strings.SplitSeq(ansi.Wrap(note, width, ""), "\n") {
		out = append(out, styles.Muted.Render(line))
	}
	return out
}

// renderAllocationTable renders tbl in the rows that the title, the
// separator and footer lines leave, with its scroll line when the rows do
// not fit. Only the rows scroll.
func (s *reportsViewState) renderAllocationTable(styles widget.Styles, height, tableWidth int, tbl *widget.Table, footer int) []string {
	// Padding (2), title and separator (2), and the footer stay on screen;
	// the table gets the rest.
	avail := max(height-appChromeRows-2-2-footer, allocationTableMin)
	if tbl.RowCount() == 0 {
		return nil
	}
	dataRows := avail - 2 // the header and its border
	if tbl.RowCount() > dataRows {
		dataRows-- // the scroll info line
	}
	dataRows = max(min(dataRows, tbl.RowCount()), 1)
	s.allocation.pageRows = dataRows
	out := []string{tbl.Render(styles, tableWidth, dataRows+2)}
	if info := tbl.ScrollInfo(dataRows); info != "" {
		out = append(out, styles.Muted.Render(info))
	}
	return out
}

// allocationTotalLine is the section's TOTAL line, laid out in the table's
// columns.
func (s *reportsViewState) allocationTotalLine(styles widget.Styles, sec report.AllocationSection) string {
	var cells []string
	for _, c := range s.allocation.columns {
		switch c.col {
		case allocationColClass:
			cells = append(cells, styles.Bold.Render(widget.AlignText("TOTAL", c.Width, c.Align)))
		case allocationColCount:
			count := ""
			if sec.Available {
				count = strconv.Itoa(sec.Count())
			}
			cells = append(cells, widget.AlignText(count, c.Width, c.Align))
		case allocationColValue:
			value := "not available"
			if sec.Available {
				value = formatDashboardMoneyIn(sec.Value, sec.Currency)
			}
			cells = append(cells, styles.Bold.Render(widget.AlignText(value, c.Width, c.Align)))
		case allocationColPercent:
			// The classes are 0% of a total that is zero or less (negative
			// cash), so the total is not 100% of them either.
			pct := ""
			if sec.Available && sec.Value.IsPositive() {
				pct = "100.0%"
			}
			cells = append(cells, widget.AlignText(pct, c.Width, c.Align))
		}
	}
	return strings.TrimRight(strings.Join(cells, " "), " ")
}

// renderAllocationDrill renders the holdings of one class.
func (s *reportsViewState) renderAllocationDrill(styles widget.Styles, height, tableWidth int, sec report.AllocationSection, dark bool) string {
	a := &s.allocation
	class, _ := sec.Class(a.drillClass)
	money := func(m types.Money) string { return formatDashboardMoneyIn(m, sec.Currency) }
	swatch := lipgloss.NewStyle().Foreground(chart.ClassColor(class.Class, dark)).Render("■")
	share := fmt.Sprintf("%s  %.1f%% of total", money(class.Value), class.Percent)
	sections := []string{
		swatch + " " + reportTitleRow(styles, class.Class.DisplayName(), share, tableWidth-2),
		styles.Muted.Render(strings.Repeat("═", tableWidth)),
	}

	ofClass := ""
	if class.Value.IsPositive() {
		ofClass = "100.0%"
	}
	footer := []string{
		styles.Muted.Render(strings.Repeat("─", tableWidth)),
		styles.Bold.Render(strings.TrimRight(alignedLine(a.drill.Columns(),
			[]string{"Total", "", money(class.Value), ofClass, fmt.Sprintf("%.1f%%", class.Percent)}), " ")),
	}
	if !sec.Available {
		footer = append(footer, styles.Muted.Render("Percentages leave out accounts that could not be valued."))
	}
	labels := make([]string, len(class.NoPrice))
	for i, row := range class.NoPrice {
		labels[i] = row.Label
	}
	after := []string{"", styles.Muted.Render("  " + widget.FitParts(
		[]string{"enter accounts", "e edit security", "esc back to allocation"}, "  ", tableWidth-2, []int{0, 1}))}
	// The holdings with no price are rows of this table, so the short note
	// needs no names.
	footer = append(footer, fitNoPriceNote(styles, labels,
		shortNoPriceNote(len(labels), "Add a price to include %s."),
		tableWidth, footerRoom(height)-len(footer)-len(after))...)
	footer = append(footer, after...)
	sections = append(sections, s.renderAllocationTable(styles, height, tableWidth, a.drill, len(footer))...)
	sections = append(sections, footer...)
	return lipgloss.NewStyle().Padding(1, 2).Render(strings.Join(sections, "\n"))
}

// renderAllocationSplit renders how one holding of the drill-down splits
// across the accounts.
func (s *reportsViewState) renderAllocationSplit(styles widget.Styles, height, tableWidth int, sec report.AllocationSection) string {
	a := &s.allocation
	row, _ := s.drillRefHolding(a.splitRef)
	value := formatDashboardMoneyIn(row.Value, sec.Currency)
	// A holding with no price is in no percentage; its value is its cost
	// basis, with the ~ of the holdings report.
	share := "~" + value + "  no price"
	if !a.splitRef.noPrice {
		class, _ := sec.Class(a.drillClass)
		share = fmt.Sprintf("%s  %.1f%% of total", value, class.Holdings[a.splitRef.index].PercentOfTotal)
	}
	out, pageRows := renderSplit(styles, height, tableWidth, a.split, row, sec.Currency, share, sec.Available, "esc back to class")
	a.pageRows = pageRows
	return out
}
