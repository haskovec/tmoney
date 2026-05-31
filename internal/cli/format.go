package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/reconciliation"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

// formatMoney is a package-cli shim delegating to cmdutil.FormatMoney so the
// in-package print* helpers keep compiling unchanged during the split. It is
// removed in the final cleanup phase once every caller references cmdutil.
func formatMoney(m types.Money, currency string) string {
	return cmdutil.FormatMoney(m, currency)
}

// printScheduledTransactionsTable prints scheduled transactions in a formatted table.
func printScheduledTransactionsTable(w io.Writer, scheduledTxns []*scheduled.Transaction, dueOnly bool, accountNames map[types.ID]string, accountCurrencies map[types.ID]string, payeeNames map[types.ID]string, categoryNames map[types.ID]string) {
	if dueOnly {
		fmt.Fprintln(w, "DUE SCHEDULED TRANSACTIONS")
		fmt.Fprintln(w, "==========================")
	} else {
		fmt.Fprintln(w, "SCHEDULED TRANSACTIONS")
		fmt.Fprintln(w, "======================")
	}

	if len(scheduledTxns) == 0 {
		if dueOnly {
			fmt.Fprintln(w, "No scheduled transactions are due.")
		} else {
			fmt.Fprintln(w, "No scheduled transactions found.")
		}
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tAccount\tNext Date\tPayee\tAmount\tFrequency\tAuto")
	fmt.Fprintln(tw, "--------\t-------\t---------\t-----\t------\t---------\t----")

	for _, st := range scheduledTxns {
		// Truncate ID for display
		idStr := st.ID.String()
		if len(idStr) > 8 {
			idStr = idStr[:8]
		}

		acctName := accountNames[st.AccountID]
		if acctName == "" {
			acctName = "-"
		}

		currency := accountCurrencies[st.AccountID]
		if currency == "" {
			currency = "USD"
		}

		py := "-"
		if st.HasPayee() {
			if name, ok := payeeNames[st.PayeeID.ID]; ok {
				py = name
			}
		}

		amount := "~" // Variable amount indicator
		if st.HasAmount() {
			amount = formatMoney(st.Amount.Money, currency)
		}

		freq := st.Frequency.DisplayName()
		if st.OccurrencesRemaining.Valid {
			freq += fmt.Sprintf(" (%d left)", st.OccurrencesRemaining.Int64)
		}

		// Auto-post indicator
		autoIndicator := ""
		if st.IsAutoPost() {
			if st.PostLeadDays > 0 {
				autoIndicator = fmt.Sprintf("[Auto %dd]", st.PostLeadDays)
			} else {
				autoIndicator = "[Auto]"
			}
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			idStr,
			acctName,
			st.NextDate.String(),
			py,
			amount,
			freq,
			autoIndicator,
		)
	}

	tw.Flush()

	fmt.Fprintf(w, "\nShowing %d scheduled transaction(s)\n", len(scheduledTxns))
}

// printNetWorthReport prints the net worth report.
func printNetWorthReport(w io.Writer, rpt *report.NetWorth) {
	fmt.Fprintln(w, "NET WORTH REPORT")
	fmt.Fprintln(w, "================")
	fmt.Fprintf(w, "As of: %s\n\n", rpt.AsOfDate.Format("January 2, 2006"))

	// Assets section
	fmt.Fprintln(w, "ASSETS")
	fmt.Fprintln(w, "------")
	if len(rpt.Assets) == 0 {
		fmt.Fprintln(w, "  (No asset accounts)")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, ab := range rpt.Assets {
			balStr := formatMoney(ab.Balance, "USD")
			if ab.EstimatedValue {
				balStr = "~" + balStr
			}
			fmt.Fprintf(tw, "  %s\t%s\n", ab.Name, balStr)
		}
		tw.Flush()
	}
	fmt.Fprintf(w, "\nTotal Assets:\t\t%s\n\n", formatMoney(rpt.TotalAssets, "USD"))

	// Liabilities section
	fmt.Fprintln(w, "LIABILITIES")
	fmt.Fprintln(w, "-----------")
	if len(rpt.Liabilities) == 0 {
		fmt.Fprintln(w, "  (No liability accounts)")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, ab := range rpt.Liabilities {
			balStr := formatMoney(ab.Balance, "USD")
			if ab.EstimatedValue {
				balStr = "~" + balStr
			}
			fmt.Fprintf(tw, "  %s\t%s\n", ab.Name, balStr)
		}
		tw.Flush()
	}
	fmt.Fprintf(w, "\nTotal Liabilities:\t%s\n\n", formatMoney(rpt.TotalLiabilities, "USD"))

	// Net worth
	fmt.Fprintln(w, "========================")
	fmt.Fprintf(w, "NET WORTH:\t\t%s\n", formatMoney(rpt.NetWorth, "USD"))
}

// printSpendingReport prints the spending by category report.
func printSpendingReport(w io.Writer, rpt *report.Spending) {
	fmt.Fprintln(w, "SPENDING BY CATEGORY")
	fmt.Fprintln(w, "====================")
	fmt.Fprintf(w, "Period: %s\n\n", rpt.Period)

	if len(rpt.Categories) == 0 {
		fmt.Fprintln(w, "No spending found for this period.")
		return
	}

	// Print category spending with visual bars
	maxBarWidth := 30
	maxAmount := types.ZeroMoney
	for _, cs := range rpt.Categories {
		if cs.Amount.Cmp(maxAmount) > 0 {
			maxAmount = cs.Amount
		}
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Category\tAmount\t%\tBar")
	fmt.Fprintln(tw, "--------\t------\t-\t---")

	for _, cs := range rpt.Categories {
		// Calculate bar length
		barLen := 0
		if !maxAmount.IsZero() {
			barLen = int(cs.Amount.Float64() / maxAmount.Float64() * float64(maxBarWidth))
		}
		bar := strings.Repeat("█", barLen)

		fmt.Fprintf(tw, "%s\t%s\t%.1f%%\t%s\n",
			cs.Name,
			formatMoney(cs.Amount, "USD"),
			cs.Percentage,
			bar,
		)

		// Print subcategories with indentation
		for _, sub := range cs.Subcategories {
			subBarLen := 0
			if !maxAmount.IsZero() {
				subBarLen = int(sub.Amount.Float64() / maxAmount.Float64() * float64(maxBarWidth))
			}
			subBar := strings.Repeat("░", subBarLen)

			fmt.Fprintf(tw, "  %s\t%s\t%.1f%%\t%s\n",
				sub.Name,
				formatMoney(sub.Amount, "USD"),
				sub.Percentage,
				subBar,
			)
		}
	}
	tw.Flush()

	fmt.Fprintf(w, "\n------------------------\nTotal Spending:\t%s\n", formatMoney(rpt.TotalSpending, "USD"))
}

// printReconcileStatus prints the reconciliation status for an account.
func printReconcileStatus(w io.Writer, acct *account.Account, status *reconciliation.Status) {
	fmt.Fprintf(w, "RECONCILIATION STATUS: %s\n", acct.Name)
	fmt.Fprintln(w, strings.Repeat("=", len("RECONCILIATION STATUS: ")+len(acct.Name)))

	if status.LastCompletedSession != nil {
		fmt.Fprintf(w, "Last reconciled:  %s (balance: %s)\n",
			status.LastCompletedSession.StatementDate.String(),
			formatMoney(status.LastCompletedSession.StatementBalance, acct.Currency))
	} else {
		fmt.Fprintln(w, "Last reconciled:  Never")
	}

	if status.ActiveSession != nil {
		fmt.Fprintln(w, "Current session:  In progress")
		fmt.Fprintf(w, "  Statement date:    %s\n", status.ActiveSession.StatementDate.String())
		fmt.Fprintf(w, "  Statement balance: %s\n", formatMoney(status.ActiveSession.StatementBalance, acct.Currency))
		fmt.Fprintf(w, "  Unreconciled transactions: %d\n", status.CandidateCount)
	} else {
		fmt.Fprintln(w, "Current session:  None")
	}
}
