package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// dashboardViewState is everything the Dashboard view owns. Its zero value is
// the view before its first load.
type dashboardViewState struct {
	data *dashboardData
	// expandedAccounts tracks which investment accounts show their holdings.
	expandedAccounts map[types.ID]bool
	// accountRows maps a content-pane row (0-based, as seen by a mouse click's
	// contentY) to the investment account whose expandable ▸/▾ header renders
	// on that row. Rebuilt every renderDashboard; used by handleMouseDashboard
	// to toggle expand/collapse on click. The render must run before the click
	// is read; Bubble Tea renders after every Update, so it does.
	accountRows map[int]types.ID
}

// dashboardData holds the loaded data for the dashboard view.
type dashboardData struct {
	netWorth           *report.NetWorth
	dueTxns            []*scheduled.Transaction
	upcomingTxns       []*scheduled.Transaction
	payeeNames         map[types.ID]string
	accountNames       map[types.ID]string
	investmentHoldings map[types.ID]*investment.AccountValuation // account ID -> valuation with holdings
	securityTickers    map[types.ID]string                       // security ID -> ticker
}

// dashboardLoadedMsg is sent when dashboard data has been loaded.
type dashboardLoadedMsg struct {
	data *dashboardData
}

// dashboardDeps is what the Dashboard view needs from outside itself. Every
// dep is a func, because switchDatabase replaces App's services and closes the
// previous *db.DB; and deps are passed to each call, never stored in the view
// state. Both rules are pinned by the guards that run over viewControllers.
// config is the user's config, for the valuation options; like the services,
// it is read when the load runs.
type dashboardDeps struct {
	reports    func() *report.Service
	schedules  func() *scheduled.Service
	payees     func() *payee.Service
	accounts   func() *account.Service
	valuations func() *investment.ValuationService
	securities func() *security.Service
	config     func() *config.Config
}

// dashboardDeps binds the Dashboard view to the services App owns. Every
// accessor may return nil, because an App built by a test has no services, so
// each caller keeps its own nil guard.
func (a *App) dashboardDeps() dashboardDeps {
	return dashboardDeps{
		reports:    func() *report.Service { return a.services.Report },
		schedules:  func() *scheduled.Service { return a.services.Scheduled },
		payees:     func() *payee.Service { return a.services.Payee },
		accounts:   func() *account.Service { return a.services.Account },
		valuations: func() *investment.ValuationService { return a.services.InvestmentValuation },
		securities: func() *security.Service { return a.services.Security },
		config:     func() *config.Config { return a.cfg },
	}
}

// load returns a command that loads all data needed for the dashboard view.
func (s *dashboardViewState) load(d dashboardDeps) tea.Cmd {
	return func() tea.Msg {
		data := &dashboardData{
			payeeNames:   make(map[types.ID]string),
			accountNames: make(map[types.ID]string),
		}

		// Load net worth report
		if reports := d.reports(); reports != nil {
			report, err := reports.NetWorthReport()
			if err != nil {
				return errMsg{err: err}
			}
			data.netWorth = report
		}

		// Load due scheduled transactions
		if schedules := d.schedules(); schedules != nil {
			due, err := schedules.ListDue()
			if err != nil {
				return errMsg{err: err}
			}
			data.dueTxns = due

			upcoming, err := schedules.ListUpcoming(30)
			if err != nil {
				return errMsg{err: err}
			}
			// Filter out items already in due list
			var filteredUpcoming []*scheduled.Transaction
			dueIDs := make(map[string]bool)
			for _, dt := range due {
				dueIDs[dt.ID.String()] = true
			}
			for _, u := range upcoming {
				if !dueIDs[u.ID.String()] {
					filteredUpcoming = append(filteredUpcoming, u)
				}
			}
			data.upcomingTxns = filteredUpcoming
		}

		// Load payee names for scheduled transactions
		if payeeSvc := d.payees(); payeeSvc != nil {
			payees, err := payeeSvc.List()
			if err == nil {
				for _, p := range payees {
					data.payeeNames[p.ID] = p.Name
				}
			}
		}

		// Load account names
		if accountSvc := d.accounts(); accountSvc != nil {
			accounts, err := accountSvc.List(true)
			if err == nil {
				for _, acc := range accounts {
					data.accountNames[acc.ID] = acc.Name
				}
			}
		}

		// Load investment account valuations with holdings for dashboard display
		if valuations := d.valuations(); valuations != nil && data.netWorth != nil {
			data.investmentHoldings = make(map[types.ID]*investment.AccountValuation)
			data.securityTickers = make(map[types.ID]string)

			for _, acct := range data.netWorth.Assets {
				if !account.Type(acct.Type).IsInvestmentType() {
					continue
				}
				val, err := valuations.GetAccountValuation(acct.AccountID, types.Today(), valuationOptionsFor(d.config()))
				if err == nil {
					data.investmentHoldings[acct.AccountID] = val
				}
			}

			// Load security tickers for all holdings
			if secSvc := d.securities(); secSvc != nil {
				securityIDs := make(map[types.ID]bool)
				for _, val := range data.investmentHoldings {
					for _, h := range val.Holdings {
						securityIDs[h.SecurityID] = true
					}
				}
				for secID := range securityIDs {
					sec, err := secSvc.GetByID(secID)
					if err == nil {
						data.securityTickers[secID] = sec.Ticker
					}
				}
			}
		}

		return dashboardLoadedMsg{data: data}
	}
}

// handleDashboardKeys handles key presses in the dashboard view. Left/Right
// (h/l) collapse/expand the selected investment account's holdings on the
// dashboard; every other key is delegated to the shared sidebar handler.
func (a *App) handleDashboardKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.sidebar.IsFocused() {
		switch {
		case key.Matches(msg, a.keys.Left):
			a.setDashboardAccountExpanded(false)
			return a, nil
		case key.Matches(msg, a.keys.Right):
			a.setDashboardAccountExpanded(true)
			return a, nil
		}
	}
	return a.handleSidebarKeys(msg)
}

// setDashboardAccountExpanded sets the expand/collapse state of the account
// under the sidebar cursor (the highlighted row — not SelectedAccount, which
// only commits on drill-in). Only investment accounts that render the ▸/▾
// affordance (see renderAssetLiabilityColumns) are expandable, so the toggle
// is a no-op for any other cursor position. Recording an explicit value also
// pins the account against the auto-expand-on-load pass (see the
// dashboardLoadedMsg handler in app_update.go), so an account the user
// collapses stays collapsed across dashboard reloads within the session.
func (a *App) setDashboardAccountExpanded(expanded bool) {
	item := a.sidebar.CursorItem()
	if item == nil || item.kind != sidebarItemAccount || item.account == nil {
		return
	}
	acct := item.account
	if !acct.Type.IsInvestmentType() {
		return
	}
	if a.dashboard.data == nil || a.dashboard.data.investmentHoldings == nil {
		return
	}
	// An investment account is expandable exactly when it renders the ▸/▾
	// affordance, which (matching renderAssetLiabilityColumns) means it has a
	// loaded valuation entry — cash-only accounts included (they expand to a
	// single "cash only" line). An account whose valuation failed to load has
	// no entry and no affordance, so the toggle is a no-op.
	if _, ok := a.dashboard.data.investmentHoldings[acct.ID]; !ok {
		return
	}
	if a.dashboard.expandedAccounts == nil {
		a.dashboard.expandedAccounts = make(map[types.ID]bool)
	}
	a.dashboard.expandedAccounts[acct.ID] = expanded
}

// render renders the dashboard view.
func (s *dashboardViewState) render(styles widget.Styles) string {
	// Discard any hit-test rows recorded on a previous render; they are
	// rebuilt below for the current data/expand state.
	s.accountRows = nil

	if s.data == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading dashboard...")
	}

	var sections []string

	// Title row: DASHBOARD + date
	contentWidth := styles.ContentWidth()
	dateStr := time.Now().Format("Jan 2, 2006")
	titleText := "DASHBOARD"
	padding := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(dateStr)-4, 1)
	titleRow := styles.Title.Render(titleText) + strings.Repeat(" ", padding) + styles.Muted.Render(dateStr)
	sections = append(sections, titleRow)

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, styles.Muted.Render(strings.Repeat("─", sepWidth)))

	// Net worth display
	if s.data.netWorth != nil {
		nw := s.data.netWorth
		sections = append(sections, "")
		sections = append(sections, renderNetWorthSummary(styles, nw)...)
		sections = append(sections, "")

		// Assets and Liabilities columns. renderAssetLiabilityColumns fills
		// expandableRows with block-relative rows for the ▸/▾ account
		// headers; translate those to absolute content-pane rows (the outer
		// Padding(1,2) adds one leading blank line, hence the +1) so a mouse
		// click can map a row back to its account.
		expandableRows := map[int]types.ID{}
		blockStart := dashboardLineCount(sections)
		sections = append(sections, renderAssetLiabilityColumns(styles, nw, contentWidth, s, expandableRows))
		if len(expandableRows) > 0 {
			s.accountRows = make(map[int]types.ID, len(expandableRows))
			for relRow, id := range expandableRows {
				s.accountRows[blockStart+relRow+1] = id
			}
		}
	} else {
		sections = append(sections, "")
		sections = append(sections, styles.Muted.Render("  No account data available"))
	}

	// Scheduled transactions section
	sections = append(sections, s.renderScheduled(styles))

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// maxDashboardHoldings is the maximum number of top holdings to display per investment account.
const maxDashboardHoldings = 5

// dashboardLineCount returns the number of terminal lines the given sections
// occupy once joined with "\n" — the sum of each section's own line count.
// Used to locate the starting row of a section for mouse hit-testing.
func dashboardLineCount(sections []string) int {
	n := 0
	for _, s := range sections {
		n += strings.Count(s, "\n") + 1
	}
	return n
}

// renderAssetLiabilityColumns renders the assets and liabilities side by side.
// dash is the Dashboard's state, for the investment detail it adds to an
// investment account's rows: the ▸/▾ expand affordance, the total-return line
// and, when expanded, the holdings. The Net Worth report passes nil and gets
// plain rows. When expandableRows is non-nil, it is filled with block-relative
// row index → account ID for each account that renders a ▸/▾ affordance, so
// the dashboard can hit-test mouse clicks on those rows; callers that don't
// need hit-testing pass nil.
func renderAssetLiabilityColumns(styles widget.Styles, report *report.NetWorth, totalWidth int, dash *dashboardViewState, expandableRows map[int]types.ID) string {
	colWidth := max(
		// Leave gap between columns
		(totalWidth-6)/2, 20)

	// headerIdxToID maps an assetsLines slice index (built below) to the
	// account whose ▸/▾ header sits there. It is translated to block-relative
	// output rows during the join loop, because a single slice entry can span
	// several terminal lines (the SectionHead cells carry a bottom border).
	// Left nil (never allocated) when the caller doesn't want hit-test rows —
	// e.g. the Net Worth report, which passes expandableRows == nil.
	var headerIdxToID map[int]types.ID

	// Build assets column
	assetsLines := []string{styles.SectionHead.Render(widget.PadRight("ASSETS", colWidth))}
	if len(report.Assets) == 0 {
		assetsLines = append(assetsLines, styles.Muted.Render("  (none)"))
	} else {
		for _, acct := range report.Assets {
			amount := netWorthRowAmount(acct)

			// Investment accounts get an expand/collapse indicator
			prefix := "  "
			expandable := false
			if account.Type(acct.Type).IsInvestmentType() && dash != nil && dash.data != nil && dash.data.investmentHoldings != nil {
				if _, hasHoldings := dash.data.investmentHoldings[acct.AccountID]; hasHoldings {
					expandable = true
					if dash.expandedAccounts[acct.AccountID] {
						prefix = "▾ "
					} else {
						prefix = "▸ "
					}
				}
			}

			// Size the name from the amount's measured width: a currency code
			// and a sign make the amount wider than "$" did, and a row wider
			// than the column wraps and shifts the hit-test rows.
			nameWidth := max(colWidth-lipgloss.Width(amount)-lipgloss.Width(prefix)-2, 1)
			name := widget.Truncate(acct.Name, nameWidth)
			line := fmt.Sprintf("%s%-*s %s", prefix, nameWidth, name, netWorthRowStyle(styles, acct).Render(amount))
			if expandable && expandableRows != nil {
				if headerIdxToID == nil {
					headerIdxToID = map[int]types.ID{}
				}
				headerIdxToID[len(assetsLines)] = acct.AccountID
			}
			assetsLines = append(assetsLines, line)

			// TR (total return) row for investment accounts — always shown
			// regardless of expand state so the headline figure stays
			// visible.
			if account.Type(acct.Type).IsInvestmentType() && dash != nil {
				if tr := dash.renderTRLine(styles, acct.AccountID, acct.Currency, colWidth); tr != "" {
					assetsLines = append(assetsLines, tr)
				}
			}

			// Show top holdings if investment account is expanded
			if account.Type(acct.Type).IsInvestmentType() && dash != nil && dash.expandedAccounts[acct.AccountID] {
				assetsLines = append(assetsLines, dash.renderHoldings(styles, acct.AccountID, acct.Currency, colWidth)...)
			}
		}
	}

	// Build liabilities column. Liability balances are stored signed
	// (negative = owed); under the LIABILITIES heading they render the raw
	// signed balance — a debt shows negative (in red), while a credit /
	// paid-ahead card shows positive (in green), so an overpaid card no
	// longer reads as a debt.
	liabLines := []string{styles.SectionHead.Render(widget.PadRight("LIABILITIES", colWidth))}
	if len(report.Liabilities) == 0 {
		liabLines = append(liabLines, styles.Muted.Render("  (none)"))
	} else {
		for _, acct := range report.Liabilities {
			amount := netWorthRowAmount(acct)
			nameWidth := max(colWidth-lipgloss.Width(amount)-4, 1)
			name := widget.Truncate(acct.Name, nameWidth)
			line := fmt.Sprintf("  %-*s %s", nameWidth, name, netWorthRowStyle(styles, acct).Render(amount))
			liabLines = append(liabLines, line)
		}
	}
	// Make the account sections the same height first, then add the totals,
	// so each currency's assets total and liabilities total share a row.
	// Padding after the totals instead let one extra asset line (a TR row,
	// an expanded holding) pair the EUR assets total with the USD
	// liabilities total.
	for len(assetsLines) < len(liabLines) {
		assetsLines = append(assetsLines, "")
	}
	for len(liabLines) < len(assetsLines) {
		liabLines = append(liabLines, "")
	}
	sep := styles.Muted.Render("  " + strings.Repeat("─", colWidth-4))
	assetsLines = append(assetsLines, sep)
	liabLines = append(liabLines, sep)
	for _, t := range report.Totals {
		// Only valued accounts (assets) can be estimated, so only the assets
		// total carries "~".
		assetsLines = append(assetsLines, renderColumnTotal(styles, colWidth, t.Currency, t.Assets, t.AssetsAvailable, t.Estimated))
		liabLines = append(liabLines, renderColumnTotal(styles, colWidth, t.Currency, t.Liabilities, t.LiabilitiesAvailable, false))
	}

	// Join columns side by side, tracking the cumulative output-line index so
	// expandable-account headers map to the terminal row they actually occupy
	// (a joined row may be multiple lines — the header row is, thanks to the
	// SectionHead bottom border).
	var rows []string
	outLine := 0
	for i := range assetsLines {
		left := widget.PadRight(assetsLines[i], colWidth)
		right := liabLines[i]
		row := left + "  " + right
		if id, ok := headerIdxToID[i]; ok && expandableRows != nil {
			expandableRows[outLine] = id
		}
		rows = append(rows, row)
		outLine += strings.Count(row, "\n") + 1
	}

	return strings.Join(rows, "\n")
}

// amountStyleBySign colors a net-worth amount by its sign, shared by both the
// ASSETS and LIABILITIES columns so signs read consistently: a negative amount
// (a debt, or an overdrawn asset) uses the Negative/red style, and a
// non-negative amount (a healthy asset, or a credit / paid-ahead liability)
// uses the Positive/green style. This keeps an overpaid card from reading as a
// debt and an overdrawn account from reading as healthy.
// renderNetWorthSummary renders one "Net Worth (CUR):" line per currency.
// Money in different currencies is never added. A currency with an account
// that could not be valued shows "not available", not a partial sum.
func renderNetWorthSummary(styles widget.Styles, nw *report.NetWorth) []string {
	lines := make([]string, 0, len(nw.Totals))
	for _, t := range nw.Totals {
		label := styles.Bold.Render("Net Worth (" + t.Currency + "):  ")
		if !t.Available {
			lines = append(lines, label+styles.Negative.Bold(true).Render("not available"))
			continue
		}
		value := formatDashboardMoneyIn(t.NetWorth, t.Currency)
		if t.Estimated {
			value = "~" + value
		}
		lines = append(lines, label+amountStyleBySign(styles, t.NetWorth).Bold(true).Render(value))
	}
	return lines
}

// renderColumnTotal renders a column's total line for one currency, marked
// "~" when estimated. The label is truncated to the width the amount leaves,
// so a long currency amount cannot widen the line past the column.
func renderColumnTotal(styles widget.Styles, colWidth int, currency string, total types.Money, available, estimated bool) string {
	amt, style := "not available", styles.Negative.Bold(true)
	if available {
		amt, style = formatDashboardMoneyIn(total, currency), amountStyleBySign(styles, total).Bold(true)
		if estimated {
			amt = "~" + amt
		}
	}
	labelWidth := max(colWidth-lipgloss.Width(amt)-4, 1)
	label := widget.Truncate("Total ("+currency+")", labelWidth)
	return fmt.Sprintf("  %-*s %s", labelWidth, label, style.Render(amt))
}

// netWorthRowAmount is a report row's amount: "error" when it could not be
// valued, and "~" when it is estimated at cost.
func netWorthRowAmount(acct report.AccountBalance) string {
	if acct.Err != nil {
		return "error"
	}
	amount := formatDashboardMoneyIn(acct.Balance, acct.Currency)
	if acct.EstimatedValue {
		amount = "~" + amount
	}
	return amount
}

// netWorthRowStyle colors a row's amount by sign, and an error as negative.
func netWorthRowStyle(styles widget.Styles, acct report.AccountBalance) lipgloss.Style {
	if acct.Err != nil {
		return styles.Negative
	}
	return amountStyleBySign(styles, acct.Balance)
}

func amountStyleBySign(styles widget.Styles, balance types.Money) lipgloss.Style {
	if balance.IsNegative() {
		return styles.Negative
	}
	return styles.Positive
}

// renderTRLine renders the total-return row for an investment
// account on the dashboard. It sits directly under the account balance
// line and shows the account's TotalReturn value and TotalReturnPct.
// Returns "" when no valuation is available so callers can skip the row
// entirely (e.g., during the initial dashboard load before valuations
// have arrived).
//
// A nil TotalReturnPct (denominator zero — no buys ever) renders as the
// "—" placeholder so the row shape stays stable across accounts.
func (s *dashboardViewState) renderTRLine(styles widget.Styles, accountID types.ID, currency string, colWidth int) string {
	if s.data == nil || s.data.investmentHoldings == nil {
		return ""
	}
	val, ok := s.data.investmentHoldings[accountID]
	if !ok || val == nil {
		return ""
	}

	pctStr := "—"
	if val.TotalReturnPct != nil {
		pctStr = fmt.Sprintf("%.2f%%", *val.TotalReturnPct)
	}

	amount := formatDashboardMoneyIn(val.TotalReturn, currency)
	right := amount + " " + pctStr

	style := styles.Muted
	switch {
	case val.TotalReturn.IsNegative():
		style = styles.Negative
	case !val.TotalReturn.IsZero():
		style = styles.Positive
	}

	pad := max(colWidth-lipgloss.Width(right)-6, 1)
	return fmt.Sprintf("    %-*s %s", pad, "TR", style.Render(right))
}

// renderHoldings renders the top holdings for an investment account on the dashboard.
func (s *dashboardViewState) renderHoldings(styles widget.Styles, accountID types.ID, currency string, colWidth int) []string {
	if s.data == nil || s.data.investmentHoldings == nil {
		return nil
	}

	val, ok := s.data.investmentHoldings[accountID]
	if !ok {
		return nil
	}

	if len(val.Holdings) == 0 {
		return []string{styles.Muted.Render("    cash only")}
	}

	// Sort holdings by market value descending (they may already be sorted, but ensure)
	sorted := make([]investment.Holding, len(val.Holdings))
	copy(sorted, val.Holdings)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].MarketValue.Cmp(sorted[i].MarketValue) > 0 {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	var lines []string
	displayCount := min(len(sorted), maxDashboardHoldings)

	for _, h := range sorted[:displayCount] {
		ticker := "???"
		if s.data.securityTickers != nil {
			if t, ok := s.data.securityTickers[h.SecurityID]; ok {
				ticker = t
			}
		}
		amount := formatDashboardMoneyIn(h.MarketValue, currency)
		ticker = widget.Truncate(ticker, max(colWidth-lipgloss.Width(amount)-6, 1))
		if !h.HasPricing {
			amount = "~" + amount
		}
		line := fmt.Sprintf("    %-*s %s", max(colWidth-lipgloss.Width(amount)-6, 1), ticker, styles.Muted.Render(amount))
		lines = append(lines, line)
	}

	if remaining := len(sorted) - displayCount; remaining > 0 {
		lines = append(lines, styles.Muted.Render(fmt.Sprintf("    +%d more", remaining)))
	}

	return lines
}

// renderScheduled renders the scheduled transactions section of the dashboard.
func (s *dashboardViewState) renderScheduled(styles widget.Styles) string {
	if s.data == nil {
		return ""
	}

	due := s.data.dueTxns
	upcoming := s.data.upcomingTxns
	total := len(due) + len(upcoming)

	var lines []string
	lines = append(lines, "")

	// Section header with count
	header := "SCHEDULED"
	if total > 0 {
		dueCount := len(due)
		if dueCount > 0 {
			header += fmt.Sprintf(" (%d due)", dueCount)
		}
	}
	lines = append(lines, styles.SectionHead.Render(header))

	if total == 0 {
		lines = append(lines, styles.Muted.Render("  No scheduled transactions"))
		return strings.Join(lines, "\n")
	}

	// Due items
	for _, st := range due {
		lines = append(lines, s.formatScheduledItem(styles, st, true))
	}

	// Upcoming items (limit to 5)
	limit := min(len(upcoming), 5)
	for i := range limit {
		lines = append(lines, s.formatScheduledItem(styles, upcoming[i], false))
	}
	if len(upcoming) > 5 {
		lines = append(lines, styles.Muted.Render(fmt.Sprintf("  ... and %d more", len(upcoming)-5)))
	}

	return strings.Join(lines, "\n")
}

// formatScheduledItem formats a single scheduled transaction line for the dashboard.
func (s *dashboardViewState) formatScheduledItem(styles widget.Styles, st *scheduled.Transaction, isDue bool) string {
	// Payee name (cap at 20 chars to prevent overflow)
	payee := "Unknown"
	if st.HasPayee() {
		if name, ok := s.data.payeeNames[st.PayeeID.ID]; ok {
			payee = name
		}
	}
	payee = widget.Truncate(payee, 20)

	// Amount
	var amount string
	if st.HasAmount() {
		amount = formatDashboardMoney(st.Amount.Money)
	} else {
		amount = "~variable"
	}

	// Due indicator
	if isDue {
		today := types.Today()
		if st.NextDate.Equal(today) {
			return fmt.Sprintf("  %s %s - %s %s",
				styles.Alert.Render("●"),
				payee,
				amount,
				styles.Alert.Render("due today"))
		}
		daysAgo := int(math.Round(time.Since(st.NextDate.Time()).Hours() / 24))
		return fmt.Sprintf("  %s %s - %s %s",
			styles.Alert.Render("●"),
			payee,
			amount,
			styles.Alert.Render(fmt.Sprintf("overdue %d days", daysAgo)))
	}

	// Upcoming - show days until
	daysUntil := int(math.Round(time.Until(st.NextDate.Time()).Hours() / 24))
	daysText := fmt.Sprintf("in %d days", daysUntil)
	if daysUntil == 1 {
		daysText = "tomorrow"
	}
	return fmt.Sprintf("  %s %s - %s %s",
		styles.Muted.Render("○"),
		payee,
		amount,
		styles.Muted.Render(daysText))
}
