package loan

import (
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"

	accountdom "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	loandom "github.com/haskovec/tmoney/internal/loan"
	scheduleddom "github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

// dash is the placeholder for a column that cannot be computed (no loan-shaped
// schedule, missing APR, or an unprojectable loan).
const dash = "—"

// formatAPRPercent renders a stored APR percentage without trailing-zero noise
// (6.5 → "6.5%", 6.375 → "6.375%").
func formatAPRPercent(apr types.Money) string {
	return strconv.FormatFloat(apr.Float64(), 'f', -1, 64) + "%"
}

// loanSummaryFields renders the payment / next-date / payoff / interest-left
// cells shared by `loan list` and the `loan show` header. Truncated projections
// render "100y+" for payoff and interest remaining; an unprojectable loan (no
// schedule, missing APR, negative amortization) renders dashes.
func loanSummaryFields(info *loanInfo) (payment, next, payoff, interestLeft string) {
	if !info.hasSchedule {
		return dash, dash, dash, dash
	}
	currency := info.account.Currency
	payment = cmdutil.FormatMoney(info.piPayment, currency)
	next = info.schedule.NextDate.String()

	switch {
	case !info.aprValid, info.projErr != nil:
		payoff, interestLeft = dash, dash
	case info.stats.Truncated:
		payoff, interestLeft = "100y+", "100y+"
	case info.stats.PaymentsRemaining > 0:
		payoff = info.stats.PayoffDate.String()
		interestLeft = cmdutil.FormatMoney(info.stats.TotalInterestRemaining, currency)
	default:
		// Paid off: no remaining payments.
		payoff, interestLeft = dash, cmdutil.FormatMoney(types.ZeroMoney, currency)
	}
	return payment, next, payoff, interestLeft
}

// printLoanList prints the loan summary table: balance owed (positive magnitude),
// APR, P&I payment, next payment date, payoff date, and interest remaining. A
// loan with no loan-shaped schedule shows dashes for the schedule-derived
// columns.
func printLoanList(w io.Writer, infos []*loanInfo) {
	fmt.Fprintln(w, "LOANS")
	fmt.Fprintln(w, "=====")

	if len(infos) == 0 {
		fmt.Fprintln(w, "No loan accounts found.")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Name\tBalance Owed\tAPR\tP&I\tNext\tPayoff\tInterest Left")
	fmt.Fprintln(tw, "----\t------------\t---\t---\t----\t------\t-------------")

	for _, info := range infos {
		apr := dash
		if info.aprValid {
			apr = formatAPRPercent(info.apr)
		}
		payment, next, payoff, interestLeft := loanSummaryFields(info)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			info.account.Name,
			cmdutil.FormatMoney(info.owed, info.account.Currency),
			apr,
			payment,
			next,
			payoff,
			interestLeft,
		)
	}
	tw.Flush()
	fmt.Fprintf(w, "\nShowing %d loan(s)\n", len(infos))
}

// printLoanShow prints a single loan's details plus its remaining-payment
// amortization projection. limit caps the number of projection rows unless all
// is true; a footer notes how many rows were withheld.
func printLoanShow(w io.Writer, info *loanInfo, limit int, all bool) {
	currency := info.account.Currency

	fmt.Fprintf(w, "LOAN: %s\n", info.account.Name)
	fmt.Fprintln(w, "=====")
	if info.account.Institution.Valid && info.account.Institution.String != "" {
		fmt.Fprintf(w, "  Institution:        %s\n", info.account.Institution.String)
	}
	fmt.Fprintf(w, "  Balance owed:       %s\n", cmdutil.FormatMoney(info.owed, currency))

	aprStr := dash
	if info.aprValid {
		aprStr = formatAPRPercent(info.apr)
	}
	fmt.Fprintf(w, "  APR:                %s\n", aprStr)

	if info.hasSchedule {
		fmt.Fprintf(w, "  P&I payment:        %s\n", cmdutil.FormatMoney(info.piPayment, currency))
		if info.escrowTotal.IsPositive() {
			fmt.Fprintf(w, "  Escrow:             %s\n", cmdutil.FormatMoney(info.escrowTotal, currency))
		}
	}

	// Non-projecting states: explain and stop.
	switch {
	case !info.hasSchedule:
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  No loan payment schedule targets this account.")
		fmt.Fprintln(w, "  Create one with `tmoney loan add`, or adopt an existing monthly transfer")
		fmt.Fprintln(w, "  schedule with Edit as loan on the TUI Scheduled view.")
		return
	case !info.aprValid:
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  This loan account has no interest rate set — set an APR to project payments.")
		return
	case info.projErr != nil:
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  Projection unavailable: %s\n", info.projErr.Error())
		return
	}

	// Projection summary line.
	s := info.stats
	switch {
	case s.Truncated:
		fmt.Fprintf(w, "  Payments left:      %d+\n", s.PaymentsRemaining)
		fmt.Fprintln(w, "  Payoff date:        100y+")
		fmt.Fprintln(w, "  Interest remaining: 100y+")
	case s.PaymentsRemaining > 0:
		fmt.Fprintf(w, "  Payments left:      %d\n", s.PaymentsRemaining)
		fmt.Fprintf(w, "  Payoff date:        %s\n", s.PayoffDate.String())
		fmt.Fprintf(w, "  Interest remaining: %s\n", cmdutil.FormatMoney(s.TotalInterestRemaining, currency))
	default:
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Loan is paid off — no remaining payments.")
		return
	}

	printProjectionTable(w, info.projection.Rows, currency, limit, all)
}

// printProjectionTable prints the remaining-payment amortization rows. When not
// all and there are more than limit rows, only the first limit are shown and a
// footer notes the remainder.
func printProjectionTable(w io.Writer, rows []loandom.Row, currency string, limit int, all bool) {
	shown := rows
	withheld := 0
	if !all && limit > 0 && len(rows) > limit {
		shown = rows[:limit]
		withheld = len(rows) - limit
	}

	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "#\tDate\tPayment\tInterest\tPrincipal\tEscrow\tBalance")
	fmt.Fprintln(tw, "-\t----\t-------\t--------\t---------\t------\t-------")
	for i := range shown {
		r := &shown[i]
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.N,
			r.Date.String(),
			cmdutil.FormatMoney(r.TotalDraft, currency),
			cmdutil.FormatMoney(r.Interest, currency),
			cmdutil.FormatMoney(r.Principal, currency),
			cmdutil.FormatMoney(r.Escrow, currency),
			cmdutil.FormatMoney(r.BalanceAfter, currency),
		)
	}
	tw.Flush()
	if withheld > 0 {
		fmt.Fprintf(w, "\n… %d more payment(s). Use --limit N or --all to see more.\n", withheld)
	}
}

// printLoanCreated prints the confirmation summary after `loan add` succeeds.
func printLoanCreated(w io.Writer, loanAcct, assetAcct *accountdom.Account, schedule *scheduleddom.Transaction, payment types.Money, computedPayment bool) {
	currency := loanAcct.Currency
	owed := loanAcct.OpeningBalance.Neg()

	fmt.Fprintln(w, "Loan created successfully!")
	fmt.Fprintf(w, "  Loan account:   %s\n", loanAcct.Name)
	fmt.Fprintf(w, "  Balance owed:   %s  (stored %s)\n",
		cmdutil.FormatMoney(owed, currency), cmdutil.FormatMoney(loanAcct.OpeningBalance, currency))
	if loanAcct.InterestRate.Valid {
		fmt.Fprintf(w, "  APR:            %s\n", formatAPRPercent(loanAcct.InterestRate.Money))
	}
	paymentLine := cmdutil.FormatMoney(payment, currency)
	if computedPayment {
		paymentLine += "  (computed from principal and term — verify against your statement)"
	}
	fmt.Fprintf(w, "  P&I payment:    %s\n", paymentLine)

	escrowCount := 0
	for _, sp := range schedule.Splits {
		if sp.LoanSection.Valid && sp.LoanSection.String == scheduleddom.LoanSectionEscrow {
			escrowCount++
		}
	}
	if escrowCount > 0 {
		fmt.Fprintf(w, "  Escrow lines:   %d\n", escrowCount)
	}
	if schedule.Amount.Valid {
		fmt.Fprintf(w, "  Monthly draft:  %s\n", cmdutil.FormatMoney(schedule.Amount.Money.Neg(), currency))
	}
	fmt.Fprintf(w, "  Next payment:   %s\n", schedule.NextDate.String())
	if schedule.IsAutoPost() {
		if schedule.PostLeadDays > 0 {
			fmt.Fprintf(w, "  Auto-post:      Yes (%d days early)\n", schedule.PostLeadDays)
		} else {
			fmt.Fprintln(w, "  Auto-post:      Yes")
		}
	}
	if assetAcct != nil {
		fmt.Fprintf(w, "  Asset account:  %s (%s)\n", assetAcct.Name, cmdutil.FormatMoney(assetAcct.OpeningBalance, currency))
	}
}
