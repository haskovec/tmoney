package scheduled

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	scheduleddom "github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

// printScheduledTransactionsTable prints scheduled transactions in a formatted
// table. When showIDs is true the ID column carries each row's full UUID (for
// use with `scheduled edit`/`scheduled delete`); otherwise it is truncated to
// the first 8 characters.
func printScheduledTransactionsTable(w io.Writer, scheduledTxns []*scheduleddom.Transaction, dueOnly, showIDs bool, accountNames map[types.ID]string, accountCurrencies map[types.ID]string, payeeNames map[types.ID]string, categoryNames map[types.ID]string) {
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
		// Show the full UUID with --show-ids; otherwise truncate for display.
		idStr := st.ID.String()
		if !showIDs && len(idStr) > 8 {
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
			amount = cmdutil.FormatMoney(st.Amount.Money, currency)
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
