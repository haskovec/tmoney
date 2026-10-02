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

// amortizationViewState is everything the Amortization view owns. Its zero
// value is the view before its first load.
type amortizationViewState struct {
	data  *amortizationViewData
	table *widget.Table
}

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

// amortizationDeps is what the Amortization view needs from outside itself.
// Every dep is a func, because switchDatabase replaces App's services and
// closes the previous *db.DB; and deps are passed to each call, never stored
// in the view state. Both rules are pinned by the guards that run over
// viewControllers.
type amortizationDeps struct {
	accounts  func() *account.Service
	scheduled func() *scheduled.Service
}

// amortizationDeps binds the Amortization view to the services App owns. Every
// accessor may return nil, because an App built by a test has no services, so
// each caller keeps its own nil guard.
func (a *App) amortizationDeps() amortizationDeps {
	return amortizationDeps{
		accounts:  func() *account.Service { return a.services.Account },
		scheduled: func() *scheduled.Service { return a.services.Scheduled },
	}
}

// load computes the live amortization projection for a loan account and
// delivers it as an amortizationLoadedMsg. It locates the loan's payment
// schedule by its principal transfer target (FindLoanSchedule), derives the
// projection inputs, and runs internal/loan.Project. Missing schedule, missing
// APR, and negative-amortization all resolve to a graceful partial state rather
// than an error.
//
// The services are read through the deps when the command runs, where the
// a.services reads were.
func (s *amortizationViewState) load(d amortizationDeps, accountID types.ID) tea.Cmd {
	return func() tea.Msg {
		accounts := d.accounts()
		if accounts == nil {
			return errMsg{err: fmt.Errorf("account service not available")}
		}
		acct, err := accounts.GetByID(accountID)
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
		if schedules := d.scheduled(); schedules != nil {
			sched, err = schedules.FindLoanSchedule(accountID)
			if err != nil {
				return errMsg{err: err}
			}
		}

		if sched == nil {
			// No schedule: show the current balance owed and APR only.
			bal, gerr := accounts.GetBalance(accountID)
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
		signedBal, berr := accounts.BalanceAsOf(accountID, sched.NextDate)
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

// buildTable (re)builds the projection table. It is cleared when there is no
// projection to show (no schedule, missing APR, or a projection error) so the
// render path falls through to its hint states.
func (s *amortizationViewState) buildTable() {
	d := s.data
	if d == nil || !d.hasSchedule || !d.aprValid || d.projErr != nil || len(d.projection.Rows) == 0 {
		s.table = nil
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
	if s.table == nil {
		s.table = widget.NewTable(columns)
	} else {
		s.table.SetColumns(columns)
	}

	rows := d.projection.Rows
	tableRows := make([][]string, len(rows))
	for i := range rows {
		tableRows[i] = formatAmortizationRow(&rows[i])
	}
	s.table.SetRows(tableRows)
	s.table.SetFocused(true)
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

// statsLine builds the header stats block. It is one line
// (Balance / APR / P&I / Escrow) in the partial states and two lines with the
// projection summary (Payments left / Payoff / Interest remaining) when a full
// projection exists. Truncated projections render Payoff and Interest remaining
// as "100y+" per the spec — never the cap row as if it were payoff.
func (s *amortizationViewState) statsLine(styles widget.Styles) string {
	d := s.data
	pair := func(l, v string) string { return styles.Muted.Render(l+":") + " " + styles.Bold.Render(v) }

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

	st := d.stats
	paymentsLeft := strconv.Itoa(st.PaymentsRemaining)
	payoff := "—"
	interestRem := "—"
	switch {
	case st.Truncated:
		paymentsLeft += "+"
		payoff = "100y+"
		interestRem = "100y+"
	case st.PaymentsRemaining > 0:
		payoff = st.PayoffDate.String()
		interestRem = formatDashboardMoney(st.TotalInterestRemaining)
	}
	line2 := strings.Join([]string{
		pair("Payments left", paymentsLeft),
		pair("Payoff", payoff),
		pair("Interest remaining", interestRem),
	}, "  ")

	return line1 + "\n" + line2
}

// render renders the loan amortization drill-in. width and height are the
// screen size.
func (s *amortizationViewState) render(styles widget.Styles, width, height int) string {
	if s.data == nil {
		return lipgloss.NewStyle().Padding(1, 2).Render("Loading amortization…")
	}
	d := s.data
	contentWidth := max(width-4, 1)

	var sections []string

	// Title row: account name + AMORTIZATION
	acctName := strings.ToUpper(d.account.Name)
	titleSuffix := "  AMORTIZATION"
	maxNameWidth := max(contentWidth-lipgloss.Width(titleSuffix)-4, 10)
	acctName = widget.Truncate(acctName, maxNameWidth)
	sections = append(sections, styles.Title.Render(acctName+titleSuffix))

	// Stats block (1 or 2 lines).
	statsBlock := s.statsLine(styles)
	sections = append(sections, statsBlock)

	// Back hint + separator.
	sections = append(sections, styles.Muted.Render("  Esc: back to register"))
	sepWidth := max(contentWidth-4, 1)
	sections = append(sections, styles.Muted.Render(strings.Repeat("─", sepWidth)))

	switch {
	case !d.hasSchedule:
		sections = append(sections, "")
		sections = append(sections, styles.Muted.Render("  No loan payment schedule targets this account."))
		sections = append(sections, styles.Muted.Render("  Create one via Accounts → New Loan…, or adopt an existing monthly"))
		sections = append(sections, styles.Muted.Render("  transfer schedule with Edit as loan on the Scheduled view."))
	case !d.aprValid:
		sections = append(sections, "")
		sections = append(sections, styles.Muted.Render("  This loan account has no interest rate set — set an APR to project payments."))
	case d.projErr != nil:
		sections = append(sections, "")
		sections = append(sections, styles.Negative.Render("  Projection unavailable: "+d.projErr.Error()))
	case len(d.projection.Rows) == 0:
		sections = append(sections, "")
		sections = append(sections, styles.Muted.Render("  Loan is paid off — no remaining payments."))
	default:
		statsHeight := strings.Count(statsBlock, "\n") + 1
		headerHeight := 1
		statusBarHeight := 1
		titleHeight := 1 + statsHeight + 1 + 1 // title + stats + back hint + separator
		footerHeight := 1
		paddingHeight := 2
		tableHeight := max(height-headerHeight-statusBarHeight-titleHeight-footerHeight-paddingHeight, 1)

		if s.table != nil {
			tableWidth := max(contentWidth-4, 1)
			sections = append(sections, s.table.Render(styles, tableWidth, tableHeight))
			if info := s.table.ScrollInfo(tableHeight - 2); info != "" {
				sections = append(sections, styles.Muted.Render("  "+info))
			}
		}
	}

	return lipgloss.NewStyle().Padding(1, 2).Render(strings.Join(sections, "\n"))
}

// handleKey handles navigation in the amortization view. height is the screen
// height, for the page size. Esc is claimed by the global handler (returns to
// the register and reloads it), so it is intentionally not handled here.
func (s *amortizationViewState) handleKey(msg tea.KeyPressMsg, keys keyMap, height int) {
	if s.table == nil {
		return
	}
	switch {
	case key.Matches(msg, keys.Up):
		s.table.MoveUp()
	case key.Matches(msg, keys.Down):
		s.table.MoveDown()
	case msg.String() == "home" || msg.String() == "g":
		s.table.MoveToTop()
	case msg.String() == "end" || msg.String() == "G":
		s.table.MoveToBottom()
	case msg.String() == "pgup":
		s.table.PageUp(max(height-10, 1))
	case msg.String() == "pgdown":
		s.table.PageDown(max(height-10, 1))
	}
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
