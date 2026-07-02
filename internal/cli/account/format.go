package account

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	accountdom "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/types"
)

// printAccountsTable prints accounts in a formatted table.
func printAccountsTable(w io.Writer, accounts []*accountdom.Account, balances map[types.ID]*accountdom.Balance) {
	if len(accounts) == 0 {
		fmt.Fprintln(w, "No accounts found.")
		return
	}

	fmt.Fprintln(w, "ACCOUNTS")
	fmt.Fprintln(w, "========")

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Name\tType\tBalance\tCurrency")
	fmt.Fprintln(tw, "----\t----\t-------\t--------")

	for _, acct := range accounts {
		balance := "N/A"
		if b, ok := balances[acct.ID]; ok {
			balance = cmdutil.FormatMoney(b.CurrentBalance, acct.Currency)
		}

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
}

// printAccountDetails prints detailed information for a single account.
func printAccountDetails(w io.Writer, acct *accountdom.Account, bal *accountdom.Balance) {
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
	fmt.Fprintf(w, "Current Balance: %s\n", cmdutil.FormatMoney(bal.CurrentBalance, acct.Currency))
	fmt.Fprintf(w, "Cleared Balance: %s\n", cmdutil.FormatMoney(bal.ClearedBalance, acct.Currency))

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
}

// printBalancesTable prints balances for all accounts with net worth.
func printBalancesTable(w io.Writer, accounts []*accountdom.Account, balances map[types.ID]*accountdom.Balance) {
	if len(accounts) == 0 {
		fmt.Fprintln(w, "No accounts found.")
		return
	}

	fmt.Fprintln(w, "BALANCES")
	fmt.Fprintln(w, "========")

	var totalAssets, totalLiabilities types.Money

	for _, acct := range accounts {
		bal := types.MustNewMoney("0")
		if b, ok := balances[acct.ID]; ok {
			bal = b.CurrentBalance
		}

		fmt.Fprintf(w, "%-20s %s\n", acct.Name+":", cmdutil.FormatMoney(bal, acct.Currency))

		// Track net worth
		if acct.Type.IsAssetType() {
			totalAssets = totalAssets.Add(bal)
		} else if acct.Type.IsLiabilityType() {
			totalLiabilities = totalLiabilities.Add(bal)
		}
	}

	fmt.Fprintln(w, "------------------------")

	// Net worth = assets + liabilities over signed balances: the standardized
	// convention stores liability balances negative when owed (see
	// specs/accounts.md), so adding them yields net worth.
	netWorth := totalAssets.Add(totalLiabilities)
	fmt.Fprintf(w, "%-20s %s\n", "Net Worth:", cmdutil.FormatMoney(netWorth, "USD"))
}
