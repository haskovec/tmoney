package transaction

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	transactiondom "github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// buildSplitCounts returns, for each transaction that is a split parent,
// the number of split lines it has. Non-split transactions are absent from
// the map. One query per transaction is fine for a CLI listing.
func buildSplitCounts(svc *app.Services, transactions []*transactiondom.Transaction) map[types.ID]int {
	counts := make(map[types.ID]int)
	for _, txn := range transactions {
		splits, err := svc.Transaction.GetSplits(txn.ID)
		if err != nil {
			continue
		}
		if len(splits) > 0 {
			counts[txn.ID] = len(splits)
		}
	}
	return counts
}

// printTransactionsTable prints transactions in a formatted table.
// When showIDs is true, each row (including the header) is prefixed with the
// transaction's UUID for use with `transfer edit` / `transfer delete`.
func printTransactionsTable(w io.Writer, acct *account.Account, transactions []*transactiondom.Transaction, payeeNames map[types.ID]string, categoryNames map[types.ID]string, splitCounts map[types.ID]int, showIDs bool) {
	fmt.Fprintf(w, "TRANSACTIONS: %s\n", acct.Name)
	fmt.Fprintln(w, strings.Repeat("=", len("TRANSACTIONS: ")+len(acct.Name)))

	if len(transactions) == 0 {
		fmt.Fprintln(w, "No transactions found.")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if showIDs {
		fmt.Fprintln(tw, "ID\tDate\tPayee\tCategory\tAmount\tStatus")
		fmt.Fprintln(tw, "--\t----\t-----\t--------\t------\t------")
	} else {
		fmt.Fprintln(tw, "Date\tPayee\tCategory\tAmount\tStatus")
		fmt.Fprintln(tw, "----\t-----\t--------\t------\t------")
	}

	for _, txn := range transactions {
		py := "-"
		if txn.PayeeID.Valid {
			if name, ok := payeeNames[txn.PayeeID.ID]; ok {
				py = name
			}
		}

		cat := "-"
		if txn.CategoryID.Valid {
			if name, ok := categoryNames[txn.CategoryID.ID]; ok {
				cat = name
			}
		}

		// For transfers, mark the payee column; the category column keeps any
		// label resolved above (a categorized transfer shows its category).
		if txn.IsTransfer() {
			py = "[Transfer]"
		}

		// Split parents carry their categories on the split lines, so mark
		// the category column instead of showing a single label.
		if n := splitCounts[txn.ID]; n > 0 {
			cat = fmt.Sprintf("[%d splits]", n)
		}

		if showIDs {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				txn.ID.String(),
				txn.Date.String(),
				py,
				cat,
				cmdutil.FormatMoney(txn.Amount, acct.Currency),
				txn.Status.Code(),
			)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				txn.Date.String(),
				py,
				cat,
				cmdutil.FormatMoney(txn.Amount, acct.Currency),
				txn.Status.Code(),
			)
		}
	}

	tw.Flush()

	fmt.Fprintf(w, "\nShowing %d transaction(s)\n", len(transactions))
}

// printSearchResults prints search results in a formatted table.
// When showIDs is true, each row (including the header) is prefixed with the
// transaction's UUID for use with `transfer edit` / `transfer delete`.
func printSearchResults(w io.Writer, searchTerm string, transactions []*transactiondom.Transaction, accountNames map[types.ID]string, accountCurrencies map[types.ID]string, payeeNames map[types.ID]string, categoryNames map[types.ID]string, splitCounts map[types.ID]int, showIDs bool) {
	fmt.Fprintf(w, "SEARCH RESULTS: %q\n", searchTerm)
	fmt.Fprintln(w, strings.Repeat("=", len("SEARCH RESULTS: ")+len(searchTerm)+2))

	if len(transactions) == 0 {
		fmt.Fprintln(w, "No transactions found.")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if showIDs {
		fmt.Fprintln(tw, "ID\tAccount\tDate\tPayee\tCategory\tAmount")
		fmt.Fprintln(tw, "--\t-------\t----\t-----\t--------\t------")
	} else {
		fmt.Fprintln(tw, "Account\tDate\tPayee\tCategory\tAmount")
		fmt.Fprintln(tw, "-------\t----\t-----\t--------\t------")
	}

	for _, txn := range transactions {
		acctName := accountNames[txn.AccountID]
		currency := accountCurrencies[txn.AccountID]
		if currency == "" {
			currency = "USD"
		}

		py := "-"
		if txn.PayeeID.Valid {
			if name, ok := payeeNames[txn.PayeeID.ID]; ok {
				py = name
			}
		}

		cat := "-"
		if txn.CategoryID.Valid {
			if name, ok := categoryNames[txn.CategoryID.ID]; ok {
				cat = name
			}
		}

		// For transfers, mark the payee column; the category column keeps any
		// label resolved above (a categorized transfer shows its category).
		if txn.IsTransfer() {
			py = "[Transfer]"
		}

		// Split parents carry their categories on the split lines, so mark
		// the category column instead of showing a single label.
		if n := splitCounts[txn.ID]; n > 0 {
			cat = fmt.Sprintf("[%d splits]", n)
		}

		if showIDs {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				txn.ID.String(),
				acctName,
				txn.Date.String(),
				py,
				cat,
				cmdutil.FormatMoney(txn.Amount, currency),
			)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				acctName,
				txn.Date.String(),
				py,
				cat,
				cmdutil.FormatMoney(txn.Amount, currency),
			)
		}
	}

	tw.Flush()

	fmt.Fprintf(w, "\nFound %d transaction(s)\n", len(transactions))
}
