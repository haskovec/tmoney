package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/loan"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// amortizationViewData holds the live amortization projection for a loan
// account's drill-in view (register → 'a'). Everything is derived from the
// loan's balance, its APR, and its loan-shaped schedule's derived P&I payment;
// nothing here is stored.
type amortizationViewData struct {
	account *account.Account

	// hasSchedule is true when a loan-shaped schedule targets the account. When
	// false the view shows the stats it can compute (balance, APR) plus a hint.
	hasSchedule bool

	owed        types.Money // positive magnitude of what is owed
	aprValid    bool
	apr         types.Money // percentage (e.g. 6.5)
	piPayment   types.Money // fixed principal-and-interest payment (escrow-exclusive)
	escrowTotal types.Money // fixed escrow pass-through per period (positive magnitude)

	projection loan.Projection
	stats      loan.Stats

	// projErr is set when a schedule + APR exist but the projection could not be
	// computed (e.g. negative amortization). The header surfaces the reason and
	// the table is omitted.
	projErr error
}

// amortizationLoadedMsg carries loaded amortization data into the update loop.
type amortizationLoadedMsg struct {
	data *amortizationViewData
}

// loadAmortizationData computes the live amortization projection for a loan
// account and delivers it as an amortizationLoadedMsg. It locates the loan's
// payment schedule by its principal transfer target (FindLoanSchedule), derives
// the projection inputs, and runs internal/loan.Project. Missing schedule,
// missing APR, and negative-amortization all resolve to a graceful partial
// state rather than an error.
func (a *App) loadAmortizationData(accountID types.ID) tea.Cmd {
	return func() tea.Msg {
		if a.accountSvc == nil {
			return errMsg{err: fmt.Errorf("account service not available")}
		}
		acct, err := a.accountSvc.GetByID(accountID)
		if err != nil {
			return errMsg{err: err}
		}
		data := &amortizationViewData{account: acct}
		if acct.InterestRate.Valid {
			data.aprValid = true
			data.apr = acct.InterestRate.Money
		}

		// The loan-shaped schedule's principal transfer targets this loan
		// account; its own AccountID is the funding account, so it can only be
		// found by transfer target.
		var sched *scheduled.Transaction
		if a.scheduledTxnSvc != nil {
			sched, err = a.scheduledTxnSvc.FindLoanSchedule(accountID)
			if err != nil {
				return errMsg{err: err}
			}
		}

		if sched == nil {
			// No schedule: show the current balance owed and APR only.
			bal, gerr := a.accountSvc.GetBalance(accountID)
			if gerr != nil {
				return errMsg{err: gerr}
			}
			data.owed = bal.CurrentBalance.Neg()
			return amortizationLoadedMsg{data: data}
		}

		data.hasSchedule = true
		piPayment, escrowTotal, dayOfMonth := scheduled.LoanScheduleInputs(sched)
		data.piPayment = piPayment
		data.escrowTotal = escrowTotal

		// owed is the loan balance as of the next payment date — the same as-of
		// balance the next post will compute against.
		signedBal, berr := a.accountSvc.BalanceAsOf(accountID, sched.NextDate)
		if berr != nil {
			return errMsg{err: berr}
		}
		data.owed = signedBal.Neg()

		if data.aprValid {
			proj, perr := loan.Project(data.owed, data.apr, piPayment, escrowTotal, sched.NextDate, dayOfMonth)
			if perr != nil {
				data.projErr = perr
			} else {
				data.projection = proj
				data.stats = loan.RemainingStats(proj)
			}
		}
		return amortizationLoadedMsg{data: data}
	}
}

// buildAmortizationTable (re)builds the projection table. It is cleared when
// there is no projection to show (no schedule, missing APR, or a projection
// error) so the render path falls through to its hint states.
func (a *App) buildAmortizationTable() {
	d := a.amortizationData
	if d == nil || !d.hasSchedule || !d.aprValid || d.projErr != nil || len(d.projection.Rows) == 0 {
		a.amortizationTable = nil
		return
	}

	columns := []widget.Column{
		{Header: "#", Width: 5, Align: widget.AlignRight},
		{Header: "DATE", Width: 12, Align: widget.AlignLeft},
		{Header: "PAYMENT", Width: 13, Align: widget.AlignRight},
		{Header: "INTEREST", Width: 13, Align: widget.AlignRight},
		{Header: "PRINCIPAL", Width: 13, Align: widget.AlignRight},
		{Header: "ESCROW", Width: 12, Align: widget.AlignRight},
		{Header: "BALANCE", Width: 14, Align: widget.AlignRight},
	}
	if a.amortizationTable == nil {
		a.amortizationTable = widget.NewTable(columns)
	} else {
		a.amortizationTable.SetColumns(columns)
	}

	rows := d.projection.Rows
	tableRows := make([][]string, len(rows))
	for i := range rows {
		tableRows[i] = formatAmortizationRow(&rows[i])
	}
	a.amortizationTable.SetRows(tableRows)
	a.amortizationTable.SetFocused(true)
}

// formatAmortizationRow formats one projection row into table cells.
func formatAmortizationRow(r *loan.Row) []string {
	return []string{
		strconv.Itoa(r.N),
		r.Date.String(),
		formatDashboardMoney(r.TotalDraft),
		formatDashboardMoney(r.Interest),
		formatDashboardMoney(r.Principal),
		formatDashboardMoney(r.Escrow),
		formatDashboardMoney(r.BalanceAfter),
	}
}

// formatAPR renders a stored APR percentage without trailing-zero noise
// (6.5 → "6.5%", 6.375 → "6.375%").
func formatAPR(apr types.Money) string {
	return strconv.FormatFloat(apr.Float64(), 'f', -1, 64) + "%"
}

// amortizationStatsLine builds the header stats block. It is one line
// (Balance / APR / P&I / Escrow) in the partial states and two lines with the
// projection summary (Payments left / Payoff / Interest remaining) when a full
// projection exists. Truncated projections render Payoff and Interest remaining
// as "100y+" per the spec — never the cap row as if it were payoff.
func (a *App) amortizationStatsLine() string {
	d := a.amortizationData
	pair := func(l, v string) string { return a.styles.Muted.Render(l+":") + " " + a.styles.Bold.Render(v) }

	aprStr := "—"
	if d.aprValid {
		aprStr = formatAPR(d.apr)
	}

	line1Parts := []string{
		pair("Balance", formatDashboardMoney(d.owed)),
		pair("APR", aprStr),
	}
	if d.hasSchedule {
		line1Parts = append(line1Parts, pair("P&I", formatDashboardMoney(d.piPayment)))
		if !d.escrowTotal.IsZero() {
			line1Parts = append(line1Parts, pair("Escrow", formatDashboardMoney(d.escrowTotal)))
		}
	}
	line1 := strings.Join(line1Parts, "  ")

	if !d.hasSchedule || !d.aprValid || d.projErr != nil {
		return line1
	}

	s := d.stats
	paymentsLeft := strconv.Itoa(s.PaymentsRemaining)
	payoff := "—"
	interestRem := "—"
	switch {
	case s.Truncated:
		paymentsLeft += "+"
		payoff = "100y+"
		interestRem = "100y+"
	case s.PaymentsRemaining > 0:
		payoff = s.PayoffDate.String()
		interestRem = formatDashboardMoney(s.TotalInterestRemaining)
	}
	line2 := strings.Join([]string{
		pair("Payments left", paymentsLeft),
		pair("Payoff", payoff),
		pair("Interest remaining", interestRem),
	}, "  ")

	return line1 + "\n" + line2
}

// renderAmortizationView renders the loan amortization drill-in.
func (a *App) renderAmortizationView() string {
	if a.amortizationData == nil {
		return lipgloss.NewStyle().Padding(1, 2).Render("Loading amortization…")
	}
	d := a.amortizationData
	contentWidth := max(a.width-4, 1)

	var sections []string

	// Title row: account name + AMORTIZATION
	acctName := strings.ToUpper(d.account.Name)
	titleSuffix := "  AMORTIZATION"
	maxNameWidth := max(contentWidth-lipgloss.Width(titleSuffix)-4, 10)
	acctName = widget.Truncate(acctName, maxNameWidth)
	sections = append(sections, a.styles.Title.Render(acctName+titleSuffix))

	// Stats block (1 or 2 lines).
	statsBlock := a.amortizationStatsLine()
	sections = append(sections, statsBlock)

	// Back hint + separator.
	sections = append(sections, a.styles.Muted.Render("  Esc: back to register"))
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, a.styles.Muted.Render(strings.Repeat("─", sepWidth)))

	switch {
	case !d.hasSchedule:
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  No loan payment schedule targets this account."))
		sections = append(sections, a.styles.Muted.Render("  Create one via Accounts → New Loan…, or adopt an existing monthly"))
		sections = append(sections, a.styles.Muted.Render("  transfer schedule with Edit as loan on the Scheduled view."))
	case !d.aprValid:
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  This loan account has no interest rate set — set an APR to project payments."))
	case d.projErr != nil:
		sections = append(sections, "")
		sections = append(sections, a.styles.Negative.Render("  Projection unavailable: "+d.projErr.Error()))
	case len(d.projection.Rows) == 0:
		sections = append(sections, "")
		sections = append(sections, a.styles.Muted.Render("  Loan is paid off — no remaining payments."))
	default:
		statsHeight := strings.Count(statsBlock, "\n") + 1
		headerHeight := 1
		statusBarHeight := 1
		titleHeight := 1 + statsHeight + 1 + 1 // title + stats + back hint + separator
		footerHeight := 1
		paddingHeight := 2
		tableHeight := max(a.height-headerHeight-statusBarHeight-titleHeight-footerHeight-paddingHeight, 1)

		if a.amortizationTable != nil {
			tableWidth := max(contentWidth-4, 1)
			sections = append(sections, a.amortizationTable.Render(a.styles, tableWidth, tableHeight))
			if info := a.amortizationTable.ScrollInfo(tableHeight - 2); info != "" {
				sections = append(sections, a.styles.Muted.Render("  "+info))
			}
		}
	}

	return lipgloss.NewStyle().Padding(1, 2).Render(strings.Join(sections, "\n"))
}

// handleAmortizationKeys handles navigation in the amortization view. Esc is
// claimed by the global handler (returns to the register and reloads it), so it
// is intentionally not handled here.
func (a *App) handleAmortizationKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.amortizationTable == nil {
		return a, nil
	}
	switch {
	case key.Matches(msg, a.keys.Up):
		a.amortizationTable.MoveUp()
	case key.Matches(msg, a.keys.Down):
		a.amortizationTable.MoveDown()
	case msg.String() == "home" || msg.String() == "g":
		a.amortizationTable.MoveToTop()
	case msg.String() == "end" || msg.String() == "G":
		a.amortizationTable.MoveToBottom()
	case msg.String() == "pgup":
		a.amortizationTable.PageUp(max(a.height-10, 1))
	case msg.String() == "pgdown":
		a.amortizationTable.PageDown(max(a.height-10, 1))
	}
	return a, nil
}

// amortizationShortcuts returns the shortcut section for the amortization help
// overlay.
func amortizationShortcuts() shortcutSection {
	return shortcutSection{
		Title: "Amortization",
		Entries: []shortcutEntry{
			{"↑↓ / j k", "Navigate payments"},
			{"g / G", "First / last payment"},
			{"PgUp/PgDn", "Page through payments"},
			{"Esc", "Back to register"},
		},
	}
}
