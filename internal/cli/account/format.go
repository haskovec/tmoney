package account

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	accountdom "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	reportdom "github.com/haskovec/tmoney/internal/report"
)

// printAccountsTable prints accounts in a formatted table. figs holds one
// figure per account, in the same order.
func printAccountsTable(w io.Writer, accounts []*accountdom.Account, figs []reportdom.AccountFigure) {
	if len(accounts) == 0 {
		fmt.Fprintln(w, "No accounts found.")
		return
	}

	fmt.Fprintln(w, "ACCOUNTS")
	fmt.Fprintln(w, "========")

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Name\tType\tBalance\tCurrency")
	fmt.Fprintln(tw, "----\t----\t-------\t--------")

	for i, acct := range accounts {
		balance := formatFigure(figs[i])

		// Annotate closed rows (only shown with --include-closed) with the
		// close date, tolerating a NULL date on a pre-existing closed account.
		name := acct.Name
		if !acct.Active {
			if acct.ClosedDate.Valid {
				name += " (closed " + acct.ClosedDate.Date.String() + ")"
			} else {
				name += " (closed)"
			}
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			name,
			acct.Type.DisplayName(),
			balance,
			acct.Currency,
		)
	}

	tw.Flush()
	printEstimateNote(w, figs)
}

// printAccountDetails prints detailed information for a single account. A
// register account shows its current and cleared register balance (bal). An
// investment account shows its cash and total value from fig instead: its
// register balance is not what it holds, and the investment ledger has no
// cleared state.
func printAccountDetails(w io.Writer, acct *accountdom.Account, bal *accountdom.Balance, fig reportdom.AccountFigure) {
	fmt.Fprintf(w, "ACCOUNT: %s\n", acct.Name)
	fmt.Fprintln(w, strings.Repeat("=", len("ACCOUNT: ")+len(acct.Name)))

	fmt.Fprintf(w, "Type:            %s\n", acct.Type.DisplayName())
	fmt.Fprintf(w, "Currency:        %s\n", acct.Currency)

	if acct.Institution.Valid {
		fmt.Fprintf(w, "Institution:     %s\n", acct.Institution.String)
	}

	if acct.AccountNumber.Valid {
		// Mask account number for privacy
		num := acct.AccountNumber.String
		if len(num) > 4 {
			num = "****" + num[len(num)-4:]
		}
		fmt.Fprintf(w, "Account Number:  %s\n", num)
	}

	fmt.Fprintf(w, "Opening Date:    %s\n", acct.OpeningDate.String())
	fmt.Fprintf(w, "Opening Balance: %s\n", cmdutil.FormatMoney(acct.OpeningBalance, acct.Currency))
	if acct.Type.IsInvestmentType() {
		cash := "error"
		if fig.Err == nil {
			cash = cmdutil.FormatMoney(fig.Cash, acct.Currency)
		}
		fmt.Fprintf(w, "Cash:            %s\n", cash)
		fmt.Fprintf(w, "Total Value:     %s\n", formatFigure(fig))
	} else {
		fmt.Fprintf(w, "Current Balance: %s\n", cmdutil.FormatMoney(bal.CurrentBalance, acct.Currency))
		fmt.Fprintf(w, "Cleared Balance: %s\n", cmdutil.FormatMoney(bal.ClearedBalance, acct.Currency))
	}

	status := "Active"
	if !acct.Active {
		if acct.ClosedDate.Valid {
			status = "Closed (" + acct.ClosedDate.Date.String() + ")"
		} else {
			status = "Closed (date unknown)"
		}
	}
	fmt.Fprintf(w, "Status:          %s\n", status)

	// Type-specific details
	if acct.CreditLimit.Valid {
		fmt.Fprintf(w, "Credit Limit:    %s\n", cmdutil.FormatMoney(acct.CreditLimit.Money, acct.Currency))
	}
	if acct.InterestRate.Valid {
		fmt.Fprintf(w, "Interest Rate:   %s%%\n", acct.InterestRate.Money.String())
	}

	if acct.Notes.Valid {
		fmt.Fprintf(w, "Notes:           %s\n", acct.Notes.String)
	}
	printEstimateNote(w, []reportdom.AccountFigure{fig})
}

// printBalancesTable prints each account's figure and the net worth of each
// currency. Money in different currencies is never added. A currency with an
// account that could not be valued has no total: a sum that leaves the
// account out would look like the real one.
func printBalancesTable(w io.Writer, accounts []*accountdom.Account, figs []reportdom.AccountFigure) {
	if len(accounts) == 0 {
		fmt.Fprintln(w, "No accounts found.")
		return
	}

	fmt.Fprintln(w, "BALANCES")
	fmt.Fprintln(w, "========")

	for i, acct := range accounts {
		fmt.Fprintf(w, "%-20s %s\n", acct.Name+":", formatFigure(figs[i]))
	}

	fmt.Fprintln(w, "------------------------")

	// Net worth = assets + liabilities over signed balances: the standardized
	// convention stores liability balances negative when owed (see
	// specs/accounts.md), so adding them yields net worth.
	for _, t := range reportdom.TotalsByCurrency(figs) {
		label := "Net Worth (" + t.Currency + "):"
		total := "not available"
		if t.Available {
			total = cmdutil.FormatMoney(t.NetWorth, t.Currency)
			if t.Estimated {
				total = "~" + total
			}
		}
		fmt.Fprintf(w, "%-20s %s\n", label, total)
	}
	printEstimateNote(w, figs)
}

// formatFigure renders an account's figure for a balance column: "error"
// when it could not be valued, and a "~" prefix when it is estimated, as the
// dashboard marks it.
func formatFigure(fig reportdom.AccountFigure) string {
	if fig.Err != nil {
		return "error"
	}
	s := cmdutil.FormatMoney(fig.Displayed, fig.Currency)
	if fig.Estimated {
		s = "~" + s
	}
	return s
}

// printEstimateNote explains the "~" mark when any figure carries it.
func printEstimateNote(w io.Writer, figs []reportdom.AccountFigure) {
	for _, fig := range figs {
		if fig.Err == nil && fig.Estimated {
			fmt.Fprintln(w, "\n~ estimated: a holding has no price, so it is valued at cost.")
			return
		}
	}
}

// figureErrors returns one error naming every account whose figure failed, or
// nil. Commands print all rows first and return it, so the command exits
// non-zero and the reasons reach stderr.
func figureErrors(accounts []*accountdom.Account, figs []reportdom.AccountFigure) error {
	var failed []string
	for i, fig := range figs {
		if fig.Err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", accounts[i].Name, fig.Err))
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return fmt.Errorf("could not value %d account(s): %s", len(failed), strings.Join(failed, "; "))
}
