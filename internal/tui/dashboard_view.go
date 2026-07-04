package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

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

// loadDashboardData returns a command that loads all data needed for the dashboard view.
func (a *App) loadDashboardData() tea.Cmd {
	return func() tea.Msg {
		data := &dashboardData{
			payeeNames:   make(map[types.ID]string),
			accountNames: make(map[types.ID]string),
		}

		// Load net worth report
		if a.reportSvc != nil {
			report, err := a.reportSvc.NetWorthReport()
			if err != nil {
				return errMsg{err: err}
			}
			data.netWorth = report
		}

		// Load due scheduled transactions
		if a.scheduledTxnSvc != nil {
			due, err := a.scheduledTxnSvc.ListDue()
			if err != nil {
				return errMsg{err: err}
			}
			data.dueTxns = due

			upcoming, err := a.scheduledTxnSvc.ListUpcoming(30)
			if err != nil {
				return errMsg{err: err}
			}
			// Filter out items already in due list
			var filteredUpcoming []*scheduled.Transaction
			dueIDs := make(map[string]bool)
			for _, d := range due {
				dueIDs[d.ID.String()] = true
			}
			for _, u := range upcoming {
				if !dueIDs[u.ID.String()] {
					filteredUpcoming = append(filteredUpcoming, u)
				}
			}
			data.upcomingTxns = filteredUpcoming
		}

		// Load payee names for scheduled transactions
		if a.payeeSvc != nil {
			payees, err := a.payeeSvc.List()
			if err == nil {
				for _, p := range payees {
					data.payeeNames[p.ID] = p.Name
				}
			}
		}

		// Load account names
		if a.accountSvc != nil {
			accounts, err := a.accountSvc.List(true)
			if err == nil {
				for _, acc := range accounts {
					data.accountNames[acc.ID] = acc.Name
				}
			}
		}

		// Load investment account valuations with holdings for dashboard display
		if a.investmentSvc != nil && data.netWorth != nil {
			data.investmentHoldings = make(map[types.ID]*investment.AccountValuation)
			data.securityTickers = make(map[types.ID]string)

			for _, acct := range data.netWorth.Assets {
				if !account.Type(acct.Type).IsInvestmentType() {
					continue
				}
				val, err := a.investmentSvc.GetAccountValuation(acct.AccountID, types.Today(), a.valuationOptions())
				if err == nil {
					data.investmentHoldings[acct.AccountID] = val
				}
			}

			// Load security tickers for all holdings
			if a.securitySvc != nil {
				securityIDs := make(map[types.ID]bool)
				for _, val := range data.investmentHoldings {
					for _, h := range val.Holdings {
						securityIDs[h.SecurityID] = true
					}
				}
				for secID := range securityIDs {
					sec, err := a.securitySvc.GetByID(secID)
					if err == nil {
						data.securityTickers[secID] = sec.Ticker
					}
				}
			}
		}

		return dashboardLoadedMsg{data: data}
	}
}

// handleDashboardKeys handles key presses in the dashboard view.
func (a *App) handleDashboardKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return a.handleSidebarKeys(msg)
}

// renderDashboard renders the dashboard view.
func (a *App) renderDashboard() string {
	if a.dashboard == nil {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Render("Loading dashboard...")
	}

	var sections []string

	// Title row: DASHBOARD + date
	contentWidth := a.styles.ContentWidth()
	dateStr := time.Now().Format("Jan 2, 2006")
	titleText := "DASHBOARD"
	padding := max(contentWidth-lipgloss.Width(titleText)-lipgloss.Width(dateStr)-4, 1)
	titleRow := a.styles.Title.Render(titleText) + strings.Repeat(" ", padding) + a.styles.Muted.Render(dateStr)
	sections = append(sections, titleRow)

	// Separator
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	// Net worth display
	if a.dashboard.netWorth != nil {
		nw := a.dashboard.netWorth
		nwLabel := "Net Worth:  "
		nwValue := formatDashboardMoney(nw.NetWorth)
		nwStyle := a.styles.Positive
		if nw.NetWorth.IsNegative() {
			nwStyle = a.styles.Negative
		}
		sections = append(sections, "")
		sections = append(sections, a.styles.Bold.Render(nwLabel)+nwStyle.Bold(true).Render(nwValue))
		sections = append(sections, "")

		// Assets and Liabilities columns
		sections = append(sections, a.renderAssetLiabilityColumns(nw, contentWidth))
	} else {
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No account data available"))
	}

	// Scheduled transactions section
	sections = append(sections, a.renderDashboardScheduled())

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(strings.Join(sections, "\n"))
}

// maxDashboardHoldings is the maximum number of top holdings to display per investment account.
const maxDashboardHoldings = 5

// renderAssetLiabilityColumns renders the assets and liabilities side by side.
func (a *App) renderAssetLiabilityColumns(report *report.NetWorth, totalWidth int) string {
	colWidth := max(
		// Leave gap between columns
		(totalWidth-6)/2, 20)

	// Build assets column
	assetsLines := []string{a.styles.SectionHead.Render(widget.PadRight("ASSETS", colWidth))}
	if len(report.Assets) == 0 {
		assetsLines = append(assetsLines, a.styles.Muted.Render("  (none)"))
	} else {
		for _, acct := range report.Assets {
			name := widget.Truncate(acct.Name, colWidth-14)
			amount := formatDashboardMoney(acct.Balance)
			if acct.EstimatedValue {
				amount = "~" + amount
			}

			// Investment accounts get an expand/collapse indicator
			prefix := "  "
			if account.Type(acct.Type).IsInvestmentType() && a.dashboard != nil && a.dashboard.investmentHoldings != nil {
				if _, hasHoldings := a.dashboard.investmentHoldings[acct.AccountID]; hasHoldings {
					if a.dashboardExpandedAccounts[acct.AccountID] {
						prefix = "▾ "
					} else {
						prefix = "▸ "
					}
				}
			}

			line := fmt.Sprintf("%s%-*s %s", prefix, colWidth-len(amount)-lipgloss.Width(prefix)-2, name, a.styles.Positive.Render(amount))
			assetsLines = append(assetsLines, line)

			// TR (total return) row for investment accounts — always shown
			// regardless of expand state so the headline figure stays
			// visible.
			if account.Type(acct.Type).IsInvestmentType() {
				if tr := a.renderDashboardTRLine(acct.AccountID, colWidth); tr != "" {
					assetsLines = append(assetsLines, tr)
				}
			}

			// Show top holdings if investment account is expanded
			if account.Type(acct.Type).IsInvestmentType() && a.dashboardExpandedAccounts[acct.AccountID] {
				assetsLines = append(assetsLines, a.renderDashboardHoldings(acct.AccountID, colWidth)...)
			}
		}
	}
	assetsLines = append(assetsLines, a.styles.Muted.Render("  "+strings.Repeat("─", colWidth-4)))
	totalLabel := "Total"
	totalAmt := formatDashboardMoney(report.TotalAssets)
	assetsLines = append(assetsLines, fmt.Sprintf("  %-*s %s", colWidth-len(totalAmt)-4, totalLabel, a.styles.Positive.Bold(true).Render(totalAmt)))

	// Build liabilities column. Liability balances are stored signed
	// (negative = owed); under the LIABILITIES heading they render the raw
	// signed balance — a debt shows negative (in red), while a credit /
	// paid-ahead card shows positive (in green), so an overpaid card no
	// longer reads as a debt.
	liabLines := []string{a.styles.SectionHead.Render(widget.PadRight("LIABILITIES", colWidth))}
	if len(report.Liabilities) == 0 {
		liabLines = append(liabLines, a.styles.Muted.Render("  (none)"))
	} else {
		for _, acct := range report.Liabilities {
			name := widget.Truncate(acct.Name, colWidth-14)
			amount := formatDashboardMoney(acct.Balance)
			if acct.EstimatedValue {
				amount = "~" + amount
			}
			line := fmt.Sprintf("  %-*s %s", colWidth-len(amount)-4, name, a.liabilityAmountStyle(acct.Balance).Render(amount))
			liabLines = append(liabLines, line)
		}
	}
	liabLines = append(liabLines, a.styles.Muted.Render("  "+strings.Repeat("─", colWidth-4)))
	totalLiabAmt := formatDashboardMoney(report.TotalLiabilities)
	liabLines = append(liabLines, fmt.Sprintf("  %-*s %s", colWidth-len(totalLiabAmt)-4, totalLabel, a.liabilityAmountStyle(report.TotalLiabilities).Bold(true).Render(totalLiabAmt)))

	// Ensure both columns have the same height
	for len(assetsLines) < len(liabLines) {
		assetsLines = append(assetsLines, "")
	}
	for len(liabLines) < len(assetsLines) {
		liabLines = append(liabLines, "")
	}

	// Join columns side by side
	var rows []string
	for i := range assetsLines {
		left := widget.PadRight(assetsLines[i], colWidth)
		right := liabLines[i]
		rows = append(rows, left+"  "+right)
	}

	return strings.Join(rows, "\n")
}

// liabilityAmountStyle colors a liability amount by sign, now that the
// LIABILITIES section shows the raw signed balance: a negative balance is a
// debt (Negative/red) and a non-negative balance is a credit or paid-ahead
// account (Positive/green), so an overpaid card does not read as a debt.
func (a *App) liabilityAmountStyle(balance types.Money) lipgloss.Style {
	if balance.IsNegative() {
		return a.styles.Negative
	}
	return a.styles.Positive
}

// renderDashboardTRLine renders the total-return row for an investment
// account on the dashboard. It sits directly under the account balance
// line and shows the account's TotalReturn value and TotalReturnPct.
// Returns "" when no valuation is available so callers can skip the row
// entirely (e.g., during the initial dashboard load before valuations
// have arrived).
//
// A nil TotalReturnPct (denominator zero — no buys ever) renders as the
// "—" placeholder so the row shape stays stable across accounts.
func (a *App) renderDashboardTRLine(accountID types.ID, colWidth int) string {
	if a.dashboard == nil || a.dashboard.investmentHoldings == nil {
		return ""
	}
	val, ok := a.dashboard.investmentHoldings[accountID]
	if !ok || val == nil {
		return ""
	}

	pctStr := "—"
	if val.TotalReturnPct != nil {
		pctStr = fmt.Sprintf("%.2f%%", *val.TotalReturnPct)
	}

	amount := formatDashboardMoney(val.TotalReturn)
	right := amount + " " + pctStr

	style := a.styles.Muted
	switch {
	case val.TotalReturn.IsNegative():
		style = a.styles.Negative
	case !val.TotalReturn.IsZero():
		style = a.styles.Positive
	}

	pad := max(colWidth-lipgloss.Width(right)-6, 1)
	return fmt.Sprintf("    %-*s %s", pad, "TR", style.Render(right))
}

// renderDashboardHoldings renders the top holdings for an investment account on the dashboard.
func (a *App) renderDashboardHoldings(accountID types.ID, colWidth int) []string {
	if a.dashboard == nil || a.dashboard.investmentHoldings == nil {
		return nil
	}

	val, ok := a.dashboard.investmentHoldings[accountID]
	if !ok {
		return nil
	}

	if len(val.Holdings) == 0 {
		return []string{a.styles.Muted.Render("    cash only")}
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
		if a.dashboard.securityTickers != nil {
			if t, ok := a.dashboard.securityTickers[h.SecurityID]; ok {
				ticker = t
			}
		}
		ticker = widget.Truncate(ticker, colWidth-20)
		amount := formatDashboardMoney(h.MarketValue)
		if !h.HasPricing {
			amount = "~" + amount
		}
		line := fmt.Sprintf("    %-*s %s", colWidth-len(amount)-6, ticker, a.styles.Muted.Render(amount))
		lines = append(lines, line)
	}

	if remaining := len(sorted) - displayCount; remaining > 0 {
		lines = append(lines, a.styles.Muted.Render(fmt.Sprintf("    +%d more", remaining)))
	}

	return lines
}

// renderDashboardScheduled renders the scheduled transactions section of the dashboard.
func (a *App) renderDashboardScheduled() string {
	if a.dashboard == nil {
		return ""
	}

	due := a.dashboard.dueTxns
	upcoming := a.dashboard.upcomingTxns
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
	lines = append(lines, a.styles.SectionHead.Render(header))

	if total == 0 {
		lines = append(lines, a.styles.Muted.Render("  No scheduled transactions"))
		return strings.Join(lines, "\n")
	}

	// Due items
	for _, st := range due {
		lines = append(lines, a.formatScheduledItem(st, true))
	}

	// Upcoming items (limit to 5)
	limit := min(len(upcoming), 5)
	for i := range limit {
		lines = append(lines, a.formatScheduledItem(upcoming[i], false))
	}
	if len(upcoming) > 5 {
		lines = append(lines, a.styles.Muted.Render(fmt.Sprintf("  ... and %d more", len(upcoming)-5)))
	}

	return strings.Join(lines, "\n")
}

// formatScheduledItem formats a single scheduled transaction line for the dashboard.
func (a *App) formatScheduledItem(st *scheduled.Transaction, isDue bool) string {
	// Payee name (cap at 20 chars to prevent overflow)
	payee := "Unknown"
	if st.HasPayee() {
		if name, ok := a.dashboard.payeeNames[st.PayeeID.ID]; ok {
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
				a.styles.Alert.Render("●"),
				payee,
				amount,
				a.styles.Alert.Render("due today"))
		}
		daysAgo := int(math.Round(time.Since(st.NextDate.Time()).Hours() / 24))
		return fmt.Sprintf("  %s %s - %s %s",
			a.styles.Alert.Render("●"),
			payee,
			amount,
			a.styles.Alert.Render(fmt.Sprintf("overdue %d days", daysAgo)))
	}

	// Upcoming - show days until
	daysUntil := int(math.Round(time.Until(st.NextDate.Time()).Hours() / 24))
	daysText := fmt.Sprintf("in %d days", daysUntil)
	if daysUntil == 1 {
		daysText = "tomorrow"
	}
	return fmt.Sprintf("  %s %s - %s %s",
		a.styles.Muted.Render("○"),
		payee,
		amount,
		a.styles.Muted.Render(daysText))
}
