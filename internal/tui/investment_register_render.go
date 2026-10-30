// Investment register: the table and the rendering, the total-return lines
// included.

package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// investmentRegisterColumns returns the investment register's column set at
// its usual widths; when withBalance is true it appends the trailing running
// cash-balance column. buildTable fits the number columns to the rows. Type
// can give up width to a wide number on a narrow table; at 10 cells it
// still shows all but the four longest type names.
func investmentRegisterColumns(withBalance bool) []widget.Column {
	cols := []widget.Column{
		{Header: "Date", Width: 10, Align: widget.AlignLeft},
		{Header: "S", Width: 1, Align: widget.AlignCenter},
		{Header: "Type", Width: 19, MinWidth: 10, Align: widget.AlignLeft},
		{Header: "Security", Width: 10, Align: widget.AlignLeft},
		{Header: "Shares", Width: 12, Align: widget.AlignRight},
		{Header: "Price", Width: 12, Align: widget.AlignRight},
		{Header: "Total", Width: 12, Align: widget.AlignRight},
	}
	if withBalance {
		cols = append(cols, widget.Column{Header: "Balance", Width: balanceColWidth, Align: widget.AlignRight})
	}
	return cols
}

// showsBalance reports whether the investment register shows the running
// cash-balance column: the security filter is off, and the register is wide
// enough for the column set with Balance, its number columns fitted to the
// rows (withBalance). The investment register has seven fixed columns, so
// Balance only fits on a fairly wide terminal (table width ≈ 98). The width
// must match the view's render.
func (s *investmentRegisterViewState) showsBalance(styles widget.Styles) bool {
	tableWidth := max(styles.ContentWidth()-4, 1)
	return !s.filterActive() && columnsFitWidth(s.withBalance, tableWidth, registerFlexMargin)
}

// buildTable creates and populates the table for the investment register view.
func (s *investmentRegisterViewState) buildTable(styles widget.Styles) {
	if s.data == nil {
		return
	}

	txns := s.visibleTransactions()
	rows := make([][]string, len(txns))
	for i, txn := range txns {
		rows[i] = s.formatRow(txn)
	}

	// The running-balance column is account-wide and can't be sliced per
	// security, so it is suppressed whenever the filter is active. The
	// Shares, Price, Total and Balance columns are as wide as their widest
	// cell.
	s.withBalance = nil
	if !s.filterActive() {
		opening := types.ZeroMoney
		if s.data.account != nil {
			opening = s.data.account.OpeningBalance
		}
		for i, cash := range runningCash(txns, opening) {
			rows[i] = append(rows[i], formatDashboardMoney(cash))
		}
		s.withBalance = fitNumberColumns(investmentRegisterColumns(true), rows)
	}
	columns := s.withBalance
	if !s.showsBalance(styles) {
		columns = fitNumberColumns(investmentRegisterColumns(false), rows)
		for i := range rows {
			rows[i] = rows[i][:len(columns)]
		}
	}

	if s.table == nil {
		s.table = widget.NewTable(columns)
	} else {
		s.table.SetColumns(columns)
	}
	s.table.SetRows(rows)

	// After a save, move the cursor onto the just-saved row by matching its
	// transaction ID. Selecting by ID (not position) keeps the cursor on the
	// row even when it sorts into the middle of the list, e.g. a back-dated
	// entry. The pending ID is cleared only once a matching row is found, so a
	// rebuild against a stale ledger (e.g. a resize landing in the async
	// save→reload window) preserves the pending selection for the real reload.
	if !s.pendingSelectID.IsNil() {
		for i, txn := range txns {
			if txn.ID == s.pendingSelectID {
				s.table.SetCursor(i)
				s.pendingSelectID = types.NilID
				break
			}
		}
	}
}

// formatRow formats an investment transaction into table row strings.
func (s *investmentRegisterViewState) formatRow(txn *investment.Transaction) []string {
	// Date — 4-digit year so impossibly-old typos like 0018 vs 2018 are
	// visually distinguishable rather than both rendering as "18".
	dateStr := txn.Date.Time().Format("01/02/2006")

	// Status indicator
	status := " "
	switch txn.Status {
	case investment.TransactionStatusCleared:
		status = "✓"
	case investment.TransactionStatusReconciled:
		status = "R"
	}

	// Type
	txnType := txn.Type.DisplayName()

	// Security (ticker from lookup map)
	sec := ""
	if txn.SecurityID.Valid {
		if name, ok := s.data.securityNames[txn.SecurityID.ID]; ok {
			sec = name
		}
	}

	// Shares
	shares := ""
	if txn.Shares.Valid && !txn.Shares.Quantity.IsZero() {
		shares = txn.Shares.Quantity.String()
	}

	// Price per share
	price := ""
	if txn.PricePerShare.Valid {
		price = formatDashboardMoney(txn.PricePerShare.Money)
	}

	// Total amount
	total := formatDashboardMoney(txn.TotalAmount)

	return []string{dateStr, status, txnType, sec, shares, price, total}
}

// render renders the investment account register view.
func (s *investmentRegisterViewState) render(styles widget.Styles, height int) string {
	if s.data == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading investment register...")
	}

	contentWidth := styles.ContentWidth()

	var sections []string

	// Title row: account name + cash balance
	acctName := strings.ToUpper(s.data.account.Name)
	cashStr := "Cash: " + formatDashboardMoney(s.data.cashBalance)

	maxNameWidth := max(contentWidth-lipgloss.Width(cashStr)-6, 10)
	acctName = widget.Truncate(acctName, maxNameWidth)
	padding := max(contentWidth-lipgloss.Width(acctName)-lipgloss.Width(cashStr)-4, 1)

	cashStyle := styles.Positive
	if s.data.cashBalance.IsNegative() {
		cashStyle = styles.Negative
	}
	titleRow := styles.Title.Render(acctName) + strings.Repeat(" ", padding) + cashStyle.Render(cashStr)
	sections = append(sections, titleRow)

	// Closed-account banner: a closed account's register is read-only.
	if s.showsClosedBanner() {
		label := "Closed · read-only"
		if s.data.account.ClosedDate.Valid {
			label = "Closed " + s.data.account.ClosedDate.Date.String() + " · read-only"
		}
		sections = append(sections, styles.Muted.Render(label))
	}

	filterActive := s.filterActive()

	// Total-return breakdown (one line of components + one line for total).
	// Suppressed while filtering — it is an account-wide summary and would be
	// misleading next to a single-security row set. Hiding it also frees two
	// rows of vertical space for scanning the filtered list.
	if s.showsTotalReturn() {
		breakdown, total := s.renderTotalReturnLines(styles, max(contentWidth-4, 1))
		sections = append(sections, breakdown, total)
	}

	// Active-filter line (ticker + full security name, and the match count).
	if filterActive {
		sections = append(sections, styles.Bold.Render(s.fittedFilterStatusLine(max(contentWidth-4, 1))))
	}

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, styles.Muted.Render(strings.Repeat("─", sepWidth)))

	// widget.Table
	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 2 + s.headerExtraRows() // title + separator (+ closed banner, total-return breakdown or filter line)
	paddingHeight := 2                     // top/bottom padding
	scrollInfoHeight := 1                  // reserve a row for the scroll info line so a long list doesn't overflow the status bar
	tableHeight := max(height-headerHeight-statusBarHeight-titleHeight-paddingHeight-scrollInfoHeight, 1)

	visibleCount := len(s.visibleTransactions())
	if s.table != nil && visibleCount > 0 {
		tableWidth := max(contentWidth-4, 1)
		sections = append(sections, s.table.Render(styles, tableWidth, tableHeight))
		if info := s.table.ScrollInfo(tableHeight - 2); info != "" {
			sections = append(sections, styles.Muted.Render("  "+info))
		}
	} else if filterActive {
		// Filtered down to nothing — the status line already names the query.
		sections = append(sections, "")
		sections = append(sections, styles.Muted.Render("  No matching transactions"))
	} else {
		sections = append(sections, "")
		sections = append(sections, styles.Muted.Render("  No investment transactions"))
		sections = append(sections, "")
		sections = append(sections, styles.Muted.Render("  Press 'n' to add a new transaction"))
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// showsClosedBanner reports whether the register draws the read-only banner
// row under its title.
func (s *investmentRegisterViewState) showsClosedBanner() bool {
	return s.data != nil && s.data.account != nil && s.data.account.IsClosed()
}

// showsTotalReturn reports whether the register draws the two total-return
// lines. They are hidden while a filter is on, and before the valuation
// loads.
func (s *investmentRegisterViewState) showsTotalReturn() bool {
	return !s.filterActive() && s.data != nil && s.data.valuation != nil
}

// headerExtraRows returns the rows the register draws between its title and
// its separator: the closed banner, then the two total-return lines or the
// one filter line. render and tableContentRowOffset both use it, so the mouse
// math follows the render.
func (s *investmentRegisterViewState) headerExtraRows() int {
	n := 0
	if s.showsClosedBanner() {
		n++
	}
	if s.filterActive() {
		n++
	} else if s.showsTotalReturn() {
		n += 2
	}
	return n
}

// renderTotalReturnLines builds the two header lines that show the
// total-return breakdown for the investment account: a components line
// (Unrealized · Realized · Div · Int · Fees) and a summary line
// (Total return $amount (pct%) · IRR pct% · TWR pct% · Value $total), where
// IRR and TWR are the money- and time-weighted returns and Value is the
// account's total worth (cash + holdings market value). Returns ("", "")
// when no valuation is loaded so the register still renders during the
// initial load.
//
// Each line fits in width cells: the render and tableContentRowOffset count
// exactly two rows, so a wrap would put a click on the wrong transaction. A
// line that is too wide leaves out whole figures from the right; Unrealized,
// Total return and Value stay longest.
//
// FeesPaid is stored as a positive magnitude on the valuation per the
// total-return spec; the line negates it before formatting so the leading
// minus sign visually reflects the subtraction in the total-return formula.
// A nil TotalReturnPct (no buys ever — denominator is zero) renders as the
// "—" placeholder so the line shape stays stable.
func (s *investmentRegisterViewState) renderTotalReturnLines(styles widget.Styles, width int) (string, string) {
	if s.data == nil || s.data.valuation == nil {
		return "", ""
	}
	v := s.data.valuation

	money := func(m types.Money) string {
		str := formatDashboardMoney(m)
		switch {
		case m.IsNegative():
			return styles.Negative.Render(str)
		case m.IsZero():
			return styles.Bold.Render(str)
		default:
			return styles.Positive.Render(str)
		}
	}

	// Fees are displayed as a negative magnitude so the subtraction in the
	// total-return formula is visually obvious. Zero stays zero.
	feeStr := formatDashboardMoney(v.FeesPaid)
	if !v.FeesPaid.IsZero() {
		feeStr = formatDashboardMoney(v.FeesPaid.Neg())
	}
	feeRendered := styles.Bold.Render(feeStr)
	if !v.FeesPaid.IsZero() {
		feeRendered = styles.Negative.Render(feeStr)
	}

	realizedField := styles.Muted.Render("Realized") + " " + money(v.RealizedGain)
	if v.AnyRealizedUnavailable {
		realizedField += " " + styles.Muted.Render("(partial)")
	}
	parts := []string{
		styles.Muted.Render("Unrealized") + " " + money(v.TotalGainLoss),
		realizedField,
		styles.Muted.Render("Div") + " " + money(v.DividendsReceived),
		styles.Muted.Render("Int") + " " + money(v.InterestReceived),
		styles.Muted.Render("Fees") + " " + feeRendered,
	}
	breakdown := widget.FitParts(parts, " · ", width, []int{4, 3, 2, 1})

	// pct renders a percent, or the "—" placeholder when the figure is
	// undefined, so every line keeps the same shape.
	pct := func(p *float64) string {
		if p == nil {
			return "—"
		}
		return fmt.Sprintf("%.2f%%", *p)
	}
	total := styles.Muted.Render("Total return") + " " + money(v.TotalReturn) + " (" + pct(v.TotalReturnPct) + ")"
	if v.AnyRealizedUnavailable {
		total += " " + styles.Muted.Render("(partial)")
	}
	// IRR and TWR show the annual figure once the ledger spans a year;
	// before that the holding-period figure is shown and marked "(cum.)".
	perf := func(label string, annual, cumulative *float64) string {
		str := styles.Muted.Render(label) + " "
		if annual != nil {
			return str + pct(annual)
		}
		str += pct(cumulative)
		if cumulative != nil {
			str += " " + styles.Muted.Render("(cum.)")
		}
		return str
	}
	// Account value (cash + holdings market value) comes after the optional
	// (partial) marker: total value is independent of the realized-gain
	// partiality that marker qualifies, so it must sit outside the marker's
	// scope.
	segments := []string{
		total,
		perf("IRR", v.MoneyWeightedReturnAnnualizedPct, v.MoneyWeightedReturnPct),
		perf("TWR", v.TimeWeightedReturnAnnualizedPct, v.TimeWeightedReturnPct),
		styles.Muted.Render("Value") + " " + money(v.TotalValue),
	}

	// TWR goes first, then IRR, then Value; the total return always stays.
	return breakdown, widget.FitParts(segments, " · ", width, []int{2, 1, 3})
}
