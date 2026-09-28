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

// investmentRegisterColumns returns the investment register's column set; when
// withBalance is true it appends the trailing running cash-balance column.
// Single source of truth shared by buildInvestmentRegisterTable and the
// resize-time fit check.
func investmentRegisterColumns(withBalance bool) []widget.Column {
	cols := []widget.Column{
		{Header: "Date", Width: 10, Align: widget.AlignLeft},
		{Header: "S", Width: 1, Align: widget.AlignCenter},
		{Header: "Type", Width: 19, Align: widget.AlignLeft},
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

// shouldShowInvestmentBalance reports whether the investment register is wide
// enough to include the running cash-balance column. The investment register
// has seven fixed columns, so Balance only fits on a fairly wide terminal
// (table width ≈ 98). The width must match renderInvestmentRegister's.
func (a *App) shouldShowInvestmentBalance() bool {
	tableWidth := max(a.styles.ContentWidth()-4, 1)
	return columnsFitWidth(investmentRegisterColumns(true), tableWidth, registerFlexMargin)
}

// buildInvestmentRegisterTable creates and populates the table for the investment register view.
func (a *App) buildInvestmentRegisterTable() {
	if a.investmentRegister == nil {
		return
	}

	// The running-balance column is account-wide and can't be sliced per
	// security, so it is suppressed whenever the filter is active.
	showBalance := a.shouldShowInvestmentBalance() && !a.investmentRegisterFilterActive()
	columns := investmentRegisterColumns(showBalance)

	txns := a.visibleInvestmentTransactions()

	var cash []types.Money
	if showBalance {
		opening := types.ZeroMoney
		if a.investmentRegister.account != nil {
			opening = a.investmentRegister.account.OpeningBalance
		}
		cash = runningCash(txns, opening)
	}

	if a.investmentTable == nil {
		a.investmentTable = widget.NewTable(columns)
	} else {
		a.investmentTable.SetColumns(columns)
	}

	rows := make([][]string, len(txns))
	for i, txn := range txns {
		row := a.formatInvestmentRegisterRow(txn)
		if showBalance {
			row = append(row, formatDashboardMoney(cash[i]))
		}
		rows[i] = row
	}
	a.investmentTable.SetRows(rows)

	// After a save, move the cursor onto the just-saved row by matching its
	// transaction ID. Selecting by ID (not position) keeps the cursor on the
	// row even when it sorts into the middle of the list, e.g. a back-dated
	// entry. The pending ID is cleared only once a matching row is found, so a
	// rebuild against a stale ledger (e.g. a resize landing in the async
	// save→reload window) preserves the pending selection for the real reload.
	if !a.pendingInvestmentSelectID.IsNil() {
		for i, txn := range txns {
			if txn.ID == a.pendingInvestmentSelectID {
				a.investmentTable.SetCursor(i)
				a.pendingInvestmentSelectID = types.NilID
				break
			}
		}
	}
}

// formatInvestmentRegisterRow formats an investment transaction into table row strings.
func (a *App) formatInvestmentRegisterRow(txn *investment.Transaction) []string {
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
		if name, ok := a.investmentRegister.securityNames[txn.SecurityID.ID]; ok {
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

// renderInvestmentRegister renders the investment account register view.
func (a *App) renderInvestmentRegister() string {
	if a.investmentRegister == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading investment register...")
	}

	contentWidth := a.styles.ContentWidth()

	var sections []string

	// Title row: account name + cash balance
	acctName := strings.ToUpper(a.investmentRegister.account.Name)
	cashStr := "Cash: " + formatDashboardMoney(a.investmentRegister.cashBalance)

	maxNameWidth := max(contentWidth-lipgloss.Width(cashStr)-6, 10)
	acctName = widget.Truncate(acctName, maxNameWidth)
	padding := max(contentWidth-lipgloss.Width(acctName)-lipgloss.Width(cashStr)-4, 1)

	cashStyle := a.styles.Positive
	if a.investmentRegister.cashBalance.IsNegative() {
		cashStyle = a.styles.Negative
	}
	titleRow := a.styles.Title.Render(acctName) + strings.Repeat(" ", padding) + cashStyle.Render(cashStr)
	sections = append(sections, titleRow)

	// Closed-account banner: a closed account's register is read-only.
	closedBanner := 0
	if a.investmentRegister.account != nil && a.investmentRegister.account.IsClosed() {
		closedBanner = 1
		label := "Closed · read-only"
		if a.investmentRegister.account.ClosedDate.Valid {
			label = "Closed " + a.investmentRegister.account.ClosedDate.Date.String() + " · read-only"
		}
		sections = append(sections, a.styles.Muted.Render(label))
	}

	filterActive := a.investmentRegisterFilterActive()

	// Total-return breakdown (one line of components + one line for total).
	// Suppressed while filtering — it is an account-wide summary and would be
	// misleading next to a single-security row set. Hiding it also frees two
	// rows of vertical space for scanning the filtered list.
	totalReturnLines := 0
	if !filterActive {
		if breakdown, total := a.renderInvestmentTotalReturnLines(); breakdown != "" {
			sections = append(sections, breakdown)
			sections = append(sections, total)
			totalReturnLines = 2
		}
	}

	// Active-filter line (ticker + full security name, and the match count).
	filterLine := 0
	if filterActive {
		filterLine = 1
		sections = append(sections, a.styles.Bold.Render(a.investmentFilterStatusLine()))
	}

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	// widget.Table
	headerHeight := 1
	statusBarHeight := 1
	titleHeight := 2 + totalReturnLines + closedBanner + filterLine // title + separator (+ optional total-return breakdown, filter line, closed banner)
	paddingHeight := 2                                              // top/bottom padding
	scrollInfoHeight := 1                                           // reserve a row for the scroll info line so a long list doesn't overflow the status bar
	tableHeight := max(a.height-headerHeight-statusBarHeight-titleHeight-paddingHeight-scrollInfoHeight, 1)

	visibleCount := len(a.visibleInvestmentTransactions())
	if a.investmentTable != nil && visibleCount > 0 {
		tableWidth := max(contentWidth-4, 1)
		sections = append(sections, a.investmentTable.Render(a.styles, tableWidth, tableHeight))
		if info := a.investmentTable.ScrollInfo(tableHeight - 2); info != "" {
			sections = append(sections, a.styles.Muted.Render("  "+info))
		}
	} else if filterActive {
		// Filtered down to nothing — the status line already names the query.
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No matching transactions"))
	} else {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No investment transactions"))
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  Press 'n' to add a new transaction"))
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// renderInvestmentTotalReturnLines builds the two header lines that show the
// total-return breakdown for the investment account: a components line
// (Unrealized · Realized · Div · Int · Fees) and a summary line
// (Total return $amount (pct%) · IRR pct% · TWR pct% · Value $total), where
// IRR and TWR are the money- and time-weighted returns and Value is the
// account's total worth (cash + holdings market value). Returns ("", "")
// when no valuation is loaded so the register still renders during the
// initial load.
//
// FeesPaid is stored as a positive magnitude on the valuation per the
// total-return spec; the line negates it before formatting so the leading
// minus sign visually reflects the subtraction in the total-return formula.
// A nil TotalReturnPct (no buys ever — denominator is zero) renders as the
// "—" placeholder so the line shape stays stable.
func (a *App) renderInvestmentTotalReturnLines() (string, string) {
	if a.investmentRegister == nil || a.investmentRegister.valuation == nil {
		return "", ""
	}
	v := a.investmentRegister.valuation

	money := func(m types.Money) string {
		s := formatDashboardMoney(m)
		switch {
		case m.IsNegative():
			return a.styles.Negative.Render(s)
		case m.IsZero():
			return a.styles.Bold.Render(s)
		default:
			return a.styles.Positive.Render(s)
		}
	}

	// Fees are displayed as a negative magnitude so the subtraction in the
	// total-return formula is visually obvious. Zero stays zero.
	feeStr := formatDashboardMoney(v.FeesPaid)
	if !v.FeesPaid.IsZero() {
		feeStr = formatDashboardMoney(v.FeesPaid.Neg())
	}
	feeRendered := a.styles.Bold.Render(feeStr)
	if !v.FeesPaid.IsZero() {
		feeRendered = a.styles.Negative.Render(feeStr)
	}

	realizedField := a.styles.Muted.Render("Realized") + " " + money(v.RealizedGain)
	if v.AnyRealizedUnavailable {
		realizedField += " " + a.styles.Muted.Render("(partial)")
	}
	parts := []string{
		a.styles.Muted.Render("Unrealized") + " " + money(v.TotalGainLoss),
		realizedField,
		a.styles.Muted.Render("Div") + " " + money(v.DividendsReceived),
		a.styles.Muted.Render("Int") + " " + money(v.InterestReceived),
		a.styles.Muted.Render("Fees") + " " + feeRendered,
	}
	breakdown := strings.Join(parts, " · ")

	// pct renders a percent, or the "—" placeholder when the figure is
	// undefined, so every line keeps the same shape.
	pct := func(p *float64) string {
		if p == nil {
			return "—"
		}
		return fmt.Sprintf("%.2f%%", *p)
	}
	total := a.styles.Muted.Render("Total return") + " " + money(v.TotalReturn) + " (" + pct(v.TotalReturnPct) + ")"
	if v.AnyRealizedUnavailable {
		total += " " + a.styles.Muted.Render("(partial)")
	}
	// IRR and TWR show the annual figure once the ledger spans a year;
	// before that the holding-period figure is shown and marked "(cum.)".
	perf := func(label string, annual, cumulative *float64) string {
		s := " · " + a.styles.Muted.Render(label) + " "
		if annual != nil {
			return s + pct(annual)
		}
		s += pct(cumulative)
		if cumulative != nil {
			s += " " + a.styles.Muted.Render("(cum.)")
		}
		return s
	}
	total += perf("IRR", v.MoneyWeightedReturnAnnualizedPct, v.MoneyWeightedReturnPct)
	total += perf("TWR", v.TimeWeightedReturnAnnualizedPct, v.TimeWeightedReturnPct)
	// Account value (cash + holdings market value) is appended after the
	// optional (partial) marker: total value is independent of the
	// realized-gain partiality that marker qualifies, so it must sit
	// outside the marker's scope.
	total += " · " + a.styles.Muted.Render("Value") + " " + money(v.TotalValue)

	return breakdown, total
}
