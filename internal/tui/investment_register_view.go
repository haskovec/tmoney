package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// investmentRegisterData holds the loaded data for the investment account register view.
type investmentRegisterData struct {
	account       *account.Account
	transactions  []*investment.Transaction
	securityNames map[types.ID]string // SecurityID -> Ticker (or name when tickerless)
	// securityFullNames maps SecurityID -> the security's full name. Kept
	// alongside securityNames so the security filter can match on both the
	// ticker and the name, and so the active-filter line can show
	// "TICKER — Full Name" (degrading to name-only for tickerless holdings).
	securityFullNames map[types.ID]string
	cashBalance       types.Money
	valuation         *investment.AccountValuation
}

// investmentRegisterLoadedMsg is sent when investment register data has been loaded.
type investmentRegisterLoadedMsg struct {
	data *investmentRegisterData
}

// loadInvestmentRegisterData returns a command that loads all data needed for the investment register view.
func (a *App) loadInvestmentRegisterData(accountID types.ID) tea.Cmd {
	return func() tea.Msg {
		data := &investmentRegisterData{
			securityNames:     make(map[types.ID]string),
			securityFullNames: make(map[types.ID]string),
		}

		// Load account
		if a.accountSvc != nil {
			acct, err := a.accountSvc.GetByID(accountID)
			if err != nil {
				return errMsg{err: err}
			}
			data.account = acct
		}

		// Load investment transactions via repository
		if a.investmentRepo != nil {
			txns, err := a.investmentRepo.ListByAccount(accountID, investment.TransactionFilter{})
			if err != nil {
				return errMsg{err: err}
			}
			data.transactions = txns
		}

		// Load cash balance via service
		if a.investmentSvc != nil {
			cash, err := a.investmentSvc.GetCashBalance(accountID)
			if err != nil {
				return errMsg{err: err}
			}
			data.cashBalance = cash

			val, err := a.investmentSvc.GetAccountValuation(accountID, types.Today(), a.valuationOptions())
			if err != nil {
				return errMsg{err: err}
			}
			data.valuation = val
		}

		// Load security names for display
		if a.securitySvc != nil {
			securities, err := a.securitySvc.List(security.Filter{})
			if err == nil {
				for _, sec := range securities {
					data.securityNames[sec.ID] = securityLabel(sec)
					data.securityFullNames[sec.ID] = sec.Name
				}
			}
		}

		return investmentRegisterLoadedMsg{data: data}
	}
}

// investmentRegisterFilterActive reports whether the security filter is
// affecting the register — either the user is typing a query or a security is
// locked. While active, the running-balance column and total-return header are
// suppressed (they are account-wide and can't be meaningfully sliced).
func (a *App) investmentRegisterFilterActive() bool {
	return a.investmentFilterSearching || !a.investmentFilterLockedSec.IsNil()
}

// resetInvestmentRegisterFilter clears all filter state, returning the register
// to its full unfiltered view.
func (a *App) resetInvestmentRegisterFilter() {
	a.investmentFilterSearching = false
	a.investmentFilterQuery = ""
	a.investmentFilterLockedSec = types.NilID
}

// visibleInvestmentTransactions returns the transactions the register should
// display given the current filter. With a security locked, only that
// security's rows show; while typing a non-empty query, rows whose security
// ticker or name contains the query show (rows with no security are excluded);
// otherwise the full ledger is returned.
func (a *App) visibleInvestmentTransactions() []*investment.Transaction {
	if a.investmentRegister == nil {
		return nil
	}
	all := a.investmentRegister.transactions

	if !a.investmentFilterLockedSec.IsNil() {
		out := make([]*investment.Transaction, 0, len(all))
		for _, txn := range all {
			if txn.SecurityID.Valid && txn.SecurityID.ID == a.investmentFilterLockedSec {
				out = append(out, txn)
			}
		}
		return out
	}

	if a.investmentFilterSearching {
		q := strings.ToLower(strings.TrimSpace(a.investmentFilterQuery))
		if q == "" {
			return all
		}
		out := make([]*investment.Transaction, 0, len(all))
		for _, txn := range all {
			if a.txnSecurityMatchesQuery(txn, q) {
				out = append(out, txn)
			}
		}
		return out
	}

	return all
}

// txnSecurityMatchesQuery reports whether a transaction's security matches the
// (already lower-cased) query by ticker/label or full name. Cash rows (no
// security) never match.
func (a *App) txnSecurityMatchesQuery(txn *investment.Transaction, lowerQuery string) bool {
	if !txn.SecurityID.Valid {
		return false
	}
	id := txn.SecurityID.ID
	if strings.Contains(strings.ToLower(a.investmentRegister.securityNames[id]), lowerQuery) {
		return true
	}
	return strings.Contains(strings.ToLower(a.investmentRegister.securityFullNames[id]), lowerQuery)
}

// investmentFilterMatchedSecurities returns the distinct securities among the
// currently visible rows, in first-seen order. Used to decide whether a typed
// query resolves to exactly one security (Enter locks) and to render the
// status line.
func (a *App) investmentFilterMatchedSecurities() []types.ID {
	seen := make(map[types.ID]bool)
	var out []types.ID
	for _, txn := range a.visibleInvestmentTransactions() {
		if txn.SecurityID.Valid && !seen[txn.SecurityID.ID] {
			seen[txn.SecurityID.ID] = true
			out = append(out, txn.SecurityID.ID)
		}
	}
	return out
}

// securityDisplayName renders a security as "TICKER — Full Name", degrading to
// just the name for a tickerless holding (where the label already equals the
// name).
func (a *App) securityDisplayName(id types.ID) string {
	if a.investmentRegister == nil {
		return ""
	}
	label := a.investmentRegister.securityNames[id]
	full := a.investmentRegister.securityFullNames[id]
	switch {
	case label != "" && full != "" && label != full:
		return label + " — " + full
	case full != "":
		return full
	default:
		return label
	}
}

// investmentFilterStatusLine builds the one-line filter indicator shown under
// the register title while a filter is active.
func (a *App) investmentFilterStatusLine() string {
	if a.investmentRegister == nil {
		return ""
	}
	total := len(a.investmentRegister.transactions)
	n := len(a.visibleInvestmentTransactions())

	if !a.investmentFilterLockedSec.IsNil() {
		return fmt.Sprintf("Filter: %s  (%d of %d)", a.securityDisplayName(a.investmentFilterLockedSec), n, total)
	}

	q := strings.TrimSpace(a.investmentFilterQuery)
	if q == "" {
		return "Filter: (type a ticker or name — Enter locks · Esc clears)"
	}
	matched := a.investmentFilterMatchedSecurities()
	switch len(matched) {
	case 0:
		return fmt.Sprintf("Filter: %s  →  no matches", q)
	case 1:
		return fmt.Sprintf("Filter: %s  →  %s (%d rows)", q, a.securityDisplayName(matched[0]), n)
	default:
		return fmt.Sprintf("Filter: %s  →  %d securities, %d rows", q, len(matched), n)
	}
}

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
		cash = runningCash(txns)
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

// selectedInvestmentTransaction returns the currently selected investment transaction based on table cursor.
func (a *App) selectedInvestmentTransaction() *investment.Transaction {
	if a.investmentRegister == nil || a.investmentTable == nil {
		return nil
	}

	txns := a.visibleInvestmentTransactions()
	cursor := a.investmentTable.Cursor()
	if cursor < 0 || cursor >= len(txns) {
		return nil
	}
	return txns[cursor]
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
// (Total return $amount (pct%) · Value $total), where Value is the
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

	pctStr := "—"
	if v.TotalReturnPct != nil {
		pctStr = fmt.Sprintf("%.2f%%", *v.TotalReturnPct)
	}
	total := a.styles.Muted.Render("Total return") + " " + money(v.TotalReturn) + " (" + pctStr + ")"
	if v.AnyRealizedUnavailable {
		total += " " + a.styles.Muted.Render("(partial)")
	}
	// Account value (cash + holdings market value) is appended after the
	// optional (partial) marker: total value is independent of the
	// realized-gain partiality that marker qualifies, so it must sit
	// outside the marker's scope.
	total += " · " + a.styles.Muted.Render("Value") + " " + money(v.TotalValue)

	return breakdown, total
}

// handleInvestmentRegisterKeys handles key presses in the investment register view.
func (a *App) handleInvestmentRegisterKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// While typing a security filter query, every key drives the filter
	// (see handleInvestmentRegisterSearchKey). handleKeyPress routes here
	// with an early guard so global bindings don't steal keystrokes.
	if a.investmentFilterSearching {
		return a.handleInvestmentRegisterSearchKey(msg)
	}

	// Esc clears a locked filter regardless of which pane has focus. This must
	// precede the sidebar delegation below, otherwise a locked filter with the
	// sidebar focused would swallow Esc (handleSidebarKeys has no Esc branch)
	// instead of clearing. Reached via the global Esc exception in handleKeyPress.
	if key.Matches(msg, a.keys.Escape) && a.investmentRegisterFilterActive() {
		a.resetInvestmentRegisterFilter()
		a.buildInvestmentRegisterTable()
		return a, nil
	}

	// Handle Tab to switch focus between sidebar and table
	if key.Matches(msg, a.keys.Tab) || key.Matches(msg, a.keys.ShiftTab) {
		if a.sidebar.IsFocused() {
			a.sidebar.SetFocused(false)
			if a.investmentTable != nil {
				a.investmentTable.SetFocused(true)
			}
		} else {
			a.sidebar.SetFocused(true)
			if a.investmentTable != nil {
				a.investmentTable.SetFocused(false)
			}
		}
		return a, nil
	}

	// If sidebar has focus, delegate to sidebar handling
	if a.sidebar.IsFocused() {
		return a.handleSidebarKeys(msg)
	}

	// widget.Table-focused key handling
	if a.investmentTable == nil || a.investmentRegister == nil {
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Up):
		a.investmentTable.MoveUp()
	case key.Matches(msg, a.keys.Down):
		a.investmentTable.MoveDown()
	case msg.String() == "home" || msg.String() == "g":
		a.investmentTable.MoveToTop()
	case msg.String() == "end" || msg.String() == "G":
		a.investmentTable.MoveToBottom()
	case msg.String() == "pgup":
		tableHeight := max(a.height-6, 1)
		a.investmentTable.PageUp(tableHeight)
	case msg.String() == "pgdown":
		tableHeight := max(a.height-6, 1)
		a.investmentTable.PageDown(tableHeight)
	case key.Matches(msg, a.keys.Search):
		// Enter the security filter. Starting a new query drops any locked
		// security so the user types fresh.
		a.investmentFilterSearching = true
		a.investmentFilterQuery = ""
		a.investmentFilterLockedSec = types.NilID
		a.buildInvestmentRegisterTable()
		if a.investmentTable != nil {
			a.investmentTable.SetCursor(0)
		}
	case a.investmentRegister.account != nil && a.investmentRegister.account.IsClosed() &&
		(msg.String() == "c" || key.Matches(msg, a.keys.New) || key.Matches(msg, a.keys.Enter) || key.Matches(msg, a.keys.Delete)):
		// A closed account is frozen: navigation and `p` (portfolio) still
		// work, but mutating actions are a no-op with an explanatory toast.
		a.statusbar.AddNotification("Account is closed — reopen to make changes", widget.NotificationAlert)
		return a, nil
	case key.Matches(msg, a.keys.New):
		a.openInvestmentTypeSelector(false)
	case key.Matches(msg, a.keys.Enter):
		txn := a.selectedInvestmentTransaction()
		if txn != nil {
			a.openInvestmentTypeSelector(true)
		}
	case msg.String() == "c":
		return a.toggleInvestmentTransactionStatus()
	case msg.String() == "p":
		// Switch to portfolio view
		if a.investmentRegister != nil && a.investmentRegister.account != nil {
			a.portfolioData = nil // Clear old data while loading
			a.switchView(ViewPortfolio)
			return a, a.loadPortfolioData(a.investmentRegister.account.ID)
		}
	case key.Matches(msg, a.keys.Delete):
		txn := a.selectedInvestmentTransaction()
		if txn != nil {
			txnID := txn.ID
			// Transfer-typed rows have a paired counterpart in another
			// account that must be deleted with them — surface that in
			// the confirmation prompt so the user isn't surprised when
			// the savings (or other-investment) side also disappears.
			prompt := fmt.Sprintf("Delete this %s transaction?", txn.Type.DisplayName())
			if txn.TransferID.Valid {
				prompt = fmt.Sprintf("Delete this %s transaction? Both sides will be removed.", txn.Type.DisplayName())
			}
			a.showConfirmDialog(
				"Delete Transaction",
				prompt,
				func() tea.Msg {
					if a.investmentSvc == nil {
						return errMsg{err: fmt.Errorf("investment service not available")}
					}
					if err := a.investmentSvc.DeleteTransaction(txnID); err != nil {
						return errMsg{err: err}
					}
					return investmentTransactionDeletedMsg{}
				},
			)
		}
	}

	return a, nil
}

// handleInvestmentRegisterSearchKey drives the security filter while the user
// is typing a query. Text/Backspace edit the query and re-narrow the list;
// the arrow / page / home / end keys still navigate so the user can
// preview-scroll matches; Enter locks the filter when the query resolves to
// exactly one security; Esc clears the filter entirely.
func (a *App) handleInvestmentRegisterSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.investmentTable == nil {
		return a, nil
	}

	rebuildTop := func() {
		a.buildInvestmentRegisterTable()
		a.investmentTable.SetCursor(0)
	}

	switch {
	case key.Matches(msg, a.keys.Escape):
		a.resetInvestmentRegisterFilter()
		a.buildInvestmentRegisterTable()
	case key.Matches(msg, a.keys.Enter):
		// Lock only when the query resolves to a single security; an ambiguous
		// or empty match keeps the user in typing mode.
		if matched := a.investmentFilterMatchedSecurities(); len(matched) == 1 {
			a.investmentFilterLockedSec = matched[0]
			a.investmentFilterSearching = false
			a.investmentFilterQuery = ""
			rebuildTop()
		}
	// Navigation is matched on the physical arrow keys via msg.Code — NOT via
	// a.keys.Up/Down, whose vim aliases ("k"/"j") would otherwise swallow those
	// letters instead of appending them to the query. Page/Home/End match on
	// msg.String() (no letter alias, so they are safe for typing).
	case msg.Code == tea.KeyUp:
		a.investmentTable.MoveUp()
	case msg.Code == tea.KeyDown:
		a.investmentTable.MoveDown()
	case msg.String() == "pgup":
		a.investmentTable.PageUp(max(a.height-6, 1))
	case msg.String() == "pgdown":
		a.investmentTable.PageDown(max(a.height-6, 1))
	case msg.String() == "home":
		a.investmentTable.MoveToTop()
	case msg.String() == "end":
		a.investmentTable.MoveToBottom()
	case msg.Code == tea.KeyBackspace:
		if r := []rune(a.investmentFilterQuery); len(r) > 0 {
			a.investmentFilterQuery = string(r[:len(r)-1])
			rebuildTop()
		}
	case msg.Text != "":
		a.investmentFilterQuery += msg.Text
		rebuildTop()
	}
	return a, nil
}

// preselectSecurityCombo sets a freshly-built investment dialog's "Security"
// combo to the given security, used so a NEW transaction opened while the
// register is filtered to a security defaults to it. It is a no-op when secID
// is nil or is not among the dialog's options.
func preselectSecurityCombo(d *dialog.Dialog, secIDs []types.ID, secID types.ID) {
	if d == nil || secID.IsNil() {
		return
	}
	for i, id := range secIDs {
		if id == secID {
			if f := d.FieldByLabel("Security"); f != nil {
				f.SelectedIndex = i
			}
			return
		}
	}
}

// toggleInvestmentTransactionStatus toggles the cleared status of the selected investment transaction.
func (a *App) toggleInvestmentTransactionStatus() (tea.Model, tea.Cmd) {
	txn := a.selectedInvestmentTransaction()
	if txn == nil {
		return a, nil
	}

	txnID := txn.ID
	currentStatus := txn.Status

	return a, func() tea.Msg {
		if a.investmentSvc == nil {
			return errMsg{err: fmt.Errorf("investment service not available")}
		}

		var cleared bool
		switch currentStatus {
		case investment.TransactionStatusPending:
			cleared = true
		case investment.TransactionStatusCleared:
			cleared = false
		default:
			return nil
		}

		// Route through the service so the closed-account freeze gate applies.
		if err := a.investmentSvc.SetClearedStatus(txnID, cleared); err != nil {
			return errMsg{err: err}
		}
		return investmentTransactionClearedMsg{}
	}
}

// investmentTransactionDeletedMsg is sent when an investment transaction has been deleted.
type investmentTransactionDeletedMsg struct{}

// investmentTransactionClearedMsg is sent when an investment transaction's status has been toggled.
type investmentTransactionClearedMsg struct{}

// investmentTransactionTypeOptions returns the display names for the investment type selector.
func investmentTransactionTypeOptions() []string {
	return []string{
		investment.TransactionTypeBuy.DisplayName(),
		investment.TransactionTypeSell.DisplayName(),
		investment.TransactionTypeDividend.DisplayName(),
		investment.TransactionTypeReinvestDividend.DisplayName(),
		investment.TransactionTypeDeposit.DisplayName(),
		investment.TransactionTypeWithdrawal.DisplayName(),
		investment.TransactionTypeInterest.DisplayName(),
		investment.TransactionTypeFee.DisplayName(),
		investment.TransactionTypeFeeLiquidation.DisplayName(),
		investment.TransactionTypeTransferCash.DisplayName(),
		investment.TransactionTypeTransferShares.DisplayName(),
	}
}

// investmentTransactionTypeFromIndex maps a selector index back to an InvestmentTransactionType.
func investmentTransactionTypeFromIndex(idx int) investment.TransactionType {
	types := []investment.TransactionType{
		investment.TransactionTypeBuy,
		investment.TransactionTypeSell,
		investment.TransactionTypeDividend,
		investment.TransactionTypeReinvestDividend,
		investment.TransactionTypeDeposit,
		investment.TransactionTypeWithdrawal,
		investment.TransactionTypeInterest,
		investment.TransactionTypeFee,
		investment.TransactionTypeFeeLiquidation,
		investment.TransactionTypeTransferCash,
		investment.TransactionTypeTransferShares,
	}
	if idx >= 0 && idx < len(types) {
		return types[idx]
	}
	return investment.TransactionTypeBuy
}

// investmentTransactionTypeIndex returns the selector index for the given transaction type.
func investmentTransactionTypeIndex(txnType investment.TransactionType) int {
	types := []investment.TransactionType{
		investment.TransactionTypeBuy,
		investment.TransactionTypeSell,
		investment.TransactionTypeDividend,
		investment.TransactionTypeReinvestDividend,
		investment.TransactionTypeDeposit,
		investment.TransactionTypeWithdrawal,
		investment.TransactionTypeInterest,
		investment.TransactionTypeFee,
		investment.TransactionTypeFeeLiquidation,
		investment.TransactionTypeTransferCash,
		investment.TransactionTypeTransferShares,
	}
	for i, t := range types {
		if t == txnType {
			return i
		}
	}
	return 0
}

// openInvestmentTypeSelector opens the transaction type selector dialog.
// If editing is true, the selector is pre-set to the currently selected transaction's type.
func (a *App) openInvestmentTypeSelector(editing bool) {
	options := investmentTransactionTypeOptions()
	selectedIdx := 0

	if editing {
		txn := a.selectedInvestmentTransaction()
		if txn != nil {
			a.investmentEditTxnID = txn.ID
			selectedIdx = investmentTransactionTypeIndex(txn.Type)
		}
		// Editing selects the security from the edited row, not the filter.
		a.investmentNewTxnSecurityID = types.NilID
	} else {
		a.investmentEditTxnID = types.NilID
		// A new transaction opened while the register is locked to a security
		// pre-selects that security in the security-bearing dialogs.
		a.investmentNewTxnSecurityID = a.investmentFilterLockedSec
		// Spin-Off is a corporate action, not a transaction type; offer it as a
		// convenience entry on the New selector (handled by index on submit).
		options = append(options, "Spin-Off…")
	}

	title := "New Transaction"
	if editing {
		title = "Edit Transaction"
	}

	d := dialog.NewDialog(title)
	d.SetWidth(40)
	d.AddSelectField("Type", options, selectedIdx)
	d.SetButtons([]dialog.DialogButton{
		{Label: "OK", Primary: true},
		{Label: "Cancel"},
	})
	d.SetVisible(true)
	a.investmentTypeSelector = d
}

// handleInvestmentTypeSelectorKey handles key presses in the investment type selector dialog.
func (a *App) handleInvestmentTypeSelectorKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	action := a.investmentTypeSelector.HandleKey(msg)

	switch action {
	case dialog.DialogActionSubmit:
		fields := a.investmentTypeSelector.Fields()
		idx := fields[0].SelectedIndex
		a.investmentTypeSelector.SetVisible(false)
		a.investmentTypeSelector = nil

		// "Spin-Off…" is appended after the transaction types on the New
		// selector; it opens the (global) spin-off dialog with the selected
		// holding pre-filled as the parent security.
		if idx >= len(investmentTransactionTypeOptions()) {
			a.spinOffDialogPreSelectedID = nil
			if txn := a.selectedInvestmentTransaction(); txn != nil && txn.SecurityID.Valid {
				secID := txn.SecurityID.ID
				a.spinOffDialogPreSelectedID = &secID
			}
			return a, a.loadSpinOffDialogData()
		}

		selectedType := investmentTransactionTypeFromIndex(idx)

		switch selectedType {
		case investment.TransactionTypeBuy:
			return a, a.loadBuyDialogData()
		case investment.TransactionTypeSell:
			return a, a.loadSellDialogData()
		case investment.TransactionTypeDividend:
			a.dividendDialogReinvest = false
			return a, a.loadDividendDialogData()
		case investment.TransactionTypeReinvestDividend:
			a.dividendDialogReinvest = true
			return a, a.loadDividendDialogData()
		case investment.TransactionTypeFeeLiquidation:
			return a, a.loadFeeLiquidationDialogData()
		case investment.TransactionTypeDeposit,
			investment.TransactionTypeWithdrawal,
			investment.TransactionTypeFee,
			investment.TransactionTypeInterest:
			a.cashOperationType = selectedType
			editTxn, ok := a.loadInvestmentEditTxn()
			if !ok {
				return a, nil
			}
			a.cashOperationDialog = buildCashOperationDialog(selectedType.DisplayName(), editTxn)
			if editTxn == nil {
				a.cashOperationDialog.SeedDateField(a.txnDialogLastSavedDate)
			}
			return a, nil
		case investment.TransactionTypeTransferCash:
			if a.investmentEditTxnID != types.NilID {
				return a, a.loadEditInvestmentTransferDialogData(a.investmentEditTxnID)
			}
			return a, a.loadTransferDialogData()
		case investment.TransactionTypeTransferShares:
			return a, a.loadTransferSharesDialogData()
		}

	case dialog.DialogActionCancel:
		a.investmentTypeSelector.SetVisible(false)
		a.investmentTypeSelector = nil
		return a, nil
	}

	return a, nil
}

// investmentRegisterShortcuts returns the shortcut section for the investment register help overlay.
func investmentRegisterShortcuts() shortcutSection {
	return shortcutSection{
		Title: "Investment Register",
		Entries: []shortcutEntry{
			{Key: "n", Description: "New transaction"},
			{Key: "Enter", Description: "Edit transaction"},
			{Key: "c", Description: "Toggle cleared"},
			{Key: "d", Description: "Delete transaction"},
			{Key: "/", Description: "Filter by security"},
			{Key: "p", Description: "Portfolio view"},
			{Key: "Tab", Description: "Switch sidebar/table"},
			{Key: "Esc", Description: "Go back"},
		},
	}
}
