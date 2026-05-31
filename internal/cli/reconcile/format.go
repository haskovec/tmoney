package reconcile

import (
	"fmt"
	"io"
	"strings"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/reconciliation"
)

// printReconcileStatus prints the reconciliation status for an account.
func printReconcileStatus(w io.Writer, acct *account.Account, status *reconciliation.Status) {
	fmt.Fprintf(w, "RECONCILIATION STATUS: %s\n", acct.Name)
	fmt.Fprintln(w, strings.Repeat("=", len("RECONCILIATION STATUS: ")+len(acct.Name)))

	if status.LastCompletedSession != nil {
		fmt.Fprintf(w, "Last reconciled:  %s (balance: %s)\n",
			status.LastCompletedSession.StatementDate.String(),
			cmdutil.FormatMoney(status.LastCompletedSession.StatementBalance, acct.Currency))
	} else {
		fmt.Fprintln(w, "Last reconciled:  Never")
	}

	if status.ActiveSession != nil {
		fmt.Fprintln(w, "Current session:  In progress")
		fmt.Fprintf(w, "  Statement date:    %s\n", status.ActiveSession.StatementDate.String())
		fmt.Fprintf(w, "  Statement balance: %s\n", cmdutil.FormatMoney(status.ActiveSession.StatementBalance, acct.Currency))
		fmt.Fprintf(w, "  Unreconciled transactions: %d\n", status.CandidateCount)
	} else {
		fmt.Fprintln(w, "Current session:  None")
	}
}
