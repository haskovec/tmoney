package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/reconciliation"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// formatMoney formats a Money value with currency symbol.
// Always displays 2 decimal places for currencies.
func formatMoney(m types.Money, currency string) string {
	// Format with 2 decimal places
	value := fmt.Sprintf("%.2f", m.Float64())

	// Determine symbol and formatting
	var symbol string
	var format string
	switch currency {
	case "USD":
		symbol = "$"
		format = "symbol"
	case "EUR":
		symbol = "€"
		format = "symbol"
	case "GBP":
		symbol = "£"
		format = "symbol"
	default:
		return fmt.Sprintf("%s %s", currency, value)
	}

	if format == "symbol" {
		if m.IsNegative() {
			return fmt.Sprintf("-%s%s", symbol, strings.TrimPrefix(value, "-"))
		}
		return fmt.Sprintf("%s%s", symbol, value)
	}

	return value
}

// printAccountsTable prints accounts in a formatted table.
func printAccountsTable(w io.Writer, accounts []*account.Account, balances map[types.ID]*account.Balance) {
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
			balance = formatMoney(b.CurrentBalance, acct.Currency)
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			acct.Name,
			acct.Type.DisplayName(),
			balance,
			acct.Currency,
		)
	}

	tw.Flush()
}

// printAccountDetails prints detailed information for a single account.
func printAccountDetails(w io.Writer, acct *account.Account, bal *account.Balance) {
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
	fmt.Fprintf(w, "Opening Balance: %s\n", formatMoney(acct.OpeningBalance, acct.Currency))
	fmt.Fprintf(w, "Current Balance: %s\n", formatMoney(bal.CurrentBalance, acct.Currency))
	fmt.Fprintf(w, "Cleared Balance: %s\n", formatMoney(bal.ClearedBalance, acct.Currency))

	status := "Active"
	if !acct.Active {
		status = "Closed"
	}
	fmt.Fprintf(w, "Status:          %s\n", status)

	// Type-specific details
	if acct.CreditLimit.Valid {
		fmt.Fprintf(w, "Credit Limit:    %s\n", formatMoney(acct.CreditLimit.Money, acct.Currency))
	}
	if acct.InterestRate.Valid {
		fmt.Fprintf(w, "Interest Rate:   %s%%\n", acct.InterestRate.Money.String())
	}

	if acct.Notes.Valid {
		fmt.Fprintf(w, "Notes:           %s\n", acct.Notes.String)
	}
}

// printBalancesTable prints balances for all accounts with net worth.
func printBalancesTable(w io.Writer, accounts []*account.Account, balances map[types.ID]*account.Balance) {
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

		fmt.Fprintf(w, "%-20s %s\n", acct.Name+":", formatMoney(bal, acct.Currency))

		// Track net worth
		if acct.Type.IsAssetType() {
			totalAssets = totalAssets.Add(bal)
		} else if acct.Type.IsLiabilityType() {
			totalLiabilities = totalLiabilities.Add(bal)
		}
	}

	fmt.Fprintln(w, "------------------------")

	// Net worth = assets - liabilities
	// For liabilities (credit cards, loans), the balance is typically negative
	// (representing what you owe), so we add them to get net worth
	netWorth := totalAssets.Add(totalLiabilities)
	fmt.Fprintf(w, "%-20s %s\n", "Net Worth:", formatMoney(netWorth, "USD"))
}

// printTransactionsTable prints transactions in a formatted table.
func printTransactionsTable(w io.Writer, acct *account.Account, transactions []*transaction.Transaction, payeeNames map[types.ID]string, categoryNames map[types.ID]string) {
	fmt.Fprintf(w, "TRANSACTIONS: %s\n", acct.Name)
	fmt.Fprintln(w, strings.Repeat("=", len("TRANSACTIONS: ")+len(acct.Name)))

	if len(transactions) == 0 {
		fmt.Fprintln(w, "No transactions found.")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Date\tPayee\tCategory\tAmount\tStatus")
	fmt.Fprintln(tw, "----\t-----\t--------\t------\t------")

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

		// For transfers, show the transfer account
		if txn.IsTransfer() {
			py = "[Transfer]"
			cat = "-"
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			txn.Date.String(),
			py,
			cat,
			formatMoney(txn.Amount, acct.Currency),
			txn.Status.Code(),
		)
	}

	tw.Flush()

	fmt.Fprintf(w, "\nShowing %d transaction(s)\n", len(transactions))
}

// printSearchResults prints search results in a formatted table.
func printSearchResults(w io.Writer, searchTerm string, transactions []*transaction.Transaction, accountNames map[types.ID]string, accountCurrencies map[types.ID]string, payeeNames map[types.ID]string, categoryNames map[types.ID]string) {
	fmt.Fprintf(w, "SEARCH RESULTS: %q\n", searchTerm)
	fmt.Fprintln(w, strings.Repeat("=", len("SEARCH RESULTS: ")+len(searchTerm)+2))

	if len(transactions) == 0 {
		fmt.Fprintln(w, "No transactions found.")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Account\tDate\tPayee\tCategory\tAmount")
	fmt.Fprintln(tw, "-------\t----\t-----\t--------\t------")

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

		// For transfers, show transfer indicator
		if txn.IsTransfer() {
			py = "[Transfer]"
			cat = "-"
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			acctName,
			txn.Date.String(),
			py,
			cat,
			formatMoney(txn.Amount, currency),
		)
	}

	tw.Flush()

	fmt.Fprintf(w, "\nFound %d transaction(s)\n", len(transactions))
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

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "tmoney version %s\n", Version)
	fmt.Fprintf(w, "Build time: %s\n", BuildTime)
	fmt.Fprintf(w, "Git commit: %s\n", GitCommit)
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, `TMoney - Personal Finance Manager

Usage:
  tmoney [file.tdb]              Launch TUI with optional file
  tmoney [options]               Run CLI commands

Global Options:
  -f, --file <path>    Specify database file
  -h, --help           Show this help message
  -v, --version        Show version information

Database Commands:
  --create <path>      Create a new database file

Account Commands:
  --list-accounts      List all accounts
    --include-closed   Include closed accounts in listing
  --account <name>     Show details for a specific account
  --balance            Show balances for all accounts with net worth

  --add-account        Create a new account (requires --name, --type)
    --name <name>            Account name
    --type <type>            Account type (checking, savings, credit_card,
                             investment, cash, loan, asset)
    --currency <code>        Currency code (default: USD)
    --opening-balance <amt>  Opening balance (default: 0)
    --opening-date <date>    Opening date (YYYY-MM-DD, default: today)
    --institution <name>     Financial institution name
    --account-number <num>   Account number
    --notes <text>           Account notes
    --credit-limit <amt>     Credit limit (credit_card only)
    --interest-rate <rate>   Interest rate % (loan only)

Transaction Commands:
  --transactions       List transactions (requires --account)
    --account <name>   Account to show transactions for
    --limit <n>        Limit number of transactions shown
    --from <date>      Start date filter (YYYY-MM-DD)
    --to <date>        End date filter (YYYY-MM-DD)
    --status <status>  Filter by status (uncleared, cleared, reconciled, void)

  --void <txn-id>      Void a transaction (sets amount to 0, status to void)

  --add-transaction    Add a new transaction (requires --account, --amount)
    --account <name>   Account for the transaction
    --amount <value>   Transaction amount (negative for expenses)
    --payee <name>     Payee name (auto-created if new)
    --category <name>  Category name
    --date <date>      Transaction date (YYYY-MM-DD, default: today)
    --memo <text>      Transaction memo/note

  --transfer           Create a transfer between accounts
    --from <account>   Source account name
    --to <account>     Destination account name
    --amount <value>   Transfer amount (must be positive)
    --date <date>      Transfer date (YYYY-MM-DD, default: today)
    --memo <text>      Transfer memo/note

  --search <term>      Search transactions by payee name or memo
    --account <name>   Filter by account
    --from <date>      Start date filter (YYYY-MM-DD)
    --to <date>        End date filter (YYYY-MM-DD)
    --category <name>  Filter by category
    --min <amount>     Minimum amount filter
    --max <amount>     Maximum amount filter

Scheduled Transaction Commands:
  --scheduled          List all scheduled transactions
    --due              Show only due scheduled transactions
    --account <name>   Filter by account

  --add-scheduled      Create a new scheduled transaction
    --account <name>         Account for the scheduled transaction
    --frequency <freq>       Frequency (daily, weekly, biweekly, monthly, quarterly, yearly)
    --amount <value>         Transaction amount (omit for variable amount)
    --payee <name>           Payee name
    --category <name>        Category name
    --date <date>            Start date (YYYY-MM-DD, default: today)
    --memo <text>            Memo/note
    --day <1-31|-1>          Day of month (monthly/quarterly; -1 for last day)
    --occurrences <n>        Number of times to repeat
    --end-date <date>        End date for the schedule (YYYY-MM-DD)
    --auto-post              Automatically post when due
    --lead-days <0|3|7>      Days before due date to auto-post (requires --auto-post)

  --post-scheduled <id>  Post a scheduled transaction (create real transaction)
    --amount <value>     Override amount (for variable amount schedules)
    --date <date>        Override date (YYYY-MM-DD, default: scheduled date)

  --skip-scheduled <id>  Skip a scheduled transaction (advance to next date)

Report Commands:
  --report net-worth     Generate net worth report
    --as-of <date>       Report as of specific date (YYYY-MM-DD, default: today)
    --include-closed     Include closed accounts in report

  --report spending      Generate spending by category report
    --month <YYYY-MM>    Report for a specific month
    --year <YYYY>        Report for a specific year
    --from <date>        Start date for custom range (YYYY-MM-DD)
    --to <date>          End date for custom range (YYYY-MM-DD)

Reconciliation Commands:
  --start-reconcile      Start reconciliation (requires --account, --statement-date,
                         --statement-balance)
    --account <name>           Account to reconcile
    --statement-date <date>    Bank statement date (YYYY-MM-DD)
    --statement-balance <amt>  Bank statement ending balance

  --mark-reconciled <id>...  Mark transactions for reconciliation (one or more IDs)

  --finish-reconcile     Complete reconciliation (requires --account)
    --account <name>     Account to finish reconciling
    --force              Complete even with non-zero difference

  --reconcile-status     Show reconciliation status (requires --account)
    --account <name>     Account to check status for

Import Commands:
  --import <file>        Import transactions from a file (dry-run by default)
    --account <name>     Target account for imported transactions
    --format <fmt>       Override format detection (csv, qif, or ofx)
    --confirm            Execute the import (default is dry-run preview)
    --skip-duplicates    Skip matched/duplicate transactions
    --update-duplicates  Update existing matched transactions

Export Commands:
  --export <file>        Export transactions to a file
    --format <fmt>       Override format detection (csv or qif)
    --account <name>     Export only a specific account
    --from <date>        Start date filter (YYYY-MM-DD)
    --to <date>          End date filter (YYYY-MM-DD)

Backup/Restore Commands:
  --backup               Create a manual backup of the database
  --restore <file>       Restore database from a backup file

Security Commands:
  --list-securities      List all securities
    --include-hidden     Include hidden securities in listing
    --type <type>        Filter by type (stock, etf, mutual_fund, other)
    --asset-class <cls>  Filter by asset class
  --security <ticker>    Show details for a specific security

  --add-security         Create a new security (requires --ticker, --name, --type)
    --ticker <ticker>    Ticker symbol (max 20 characters)
    --name <name>        Security name
    --type <type>        Security type (stock, etf, mutual_fund, other)
    --asset-class <cls>  Asset class (default: unclassified)
    --currency <code>    Currency code (default: USD)
    --exchange <name>    Exchange name

  --edit-security <tkr>  Edit a security by ticker
    --ticker <ticker>    New ticker symbol
    --name <name>        New security name
    --type <type>        New security type
    --asset-class <cls>  New asset class
    --currency <code>    New currency code
    --exchange <name>    New exchange name

  --hide-security <tkr>  Hide a security (fails if positions exist)
  --unhide-security <tkr> Unhide a hidden security
  --delete-security <tkr> Delete a security (fails if history exists)

Portfolio Commands:
  --portfolio            Show investment portfolio holdings and summary
    --account <name>     Investment account name
    --as-of <date>       Valuation date (YYYY-MM-DD, default: today)
    --show-lots          Show lot detail for each holding (lot-tracking accounts)

For more information, visit: https://github.com/haskovec/tmoney`)
}

// printSecuritiesTable prints securities in a formatted table.
func printSecuritiesTable(w io.Writer, securities []*security.Security) {
	if len(securities) == 0 {
		fmt.Fprintln(w, "No securities found.")
		return
	}

	fmt.Fprintln(w, "SECURITIES")
	fmt.Fprintln(w, "==========")

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Ticker\tName\tType\tAsset Class\tCurrency")
	fmt.Fprintln(tw, "------\t----\t----\t-----------\t--------")

	for _, sec := range securities {
		hidden := ""
		if sec.Hidden {
			hidden = " [hidden]"
		}

		fmt.Fprintf(tw, "%s%s\t%s\t%s\t%s\t%s\n",
			sec.Ticker,
			hidden,
			sec.Name,
			sec.SecurityType.DisplayName(),
			sec.AssetClass.DisplayName(),
			sec.Currency,
		)
	}

	tw.Flush()

	fmt.Fprintf(w, "\nShowing %d security(ies)\n", len(securities))
}

// printSecurityDetails prints detailed information for a single security.
func printSecurityDetails(w io.Writer, sec *security.Security) {
	fmt.Fprintf(w, "SECURITY: %s\n", sec.Ticker)
	fmt.Fprintln(w, strings.Repeat("=", len("SECURITY: ")+len(sec.Ticker)))

	fmt.Fprintf(w, "Ticker:      %s\n", sec.Ticker)
	fmt.Fprintf(w, "Name:        %s\n", sec.Name)
	fmt.Fprintf(w, "Type:        %s\n", sec.SecurityType.DisplayName())
	fmt.Fprintf(w, "Asset Class: %s\n", sec.AssetClass.DisplayName())
	fmt.Fprintf(w, "Currency:    %s\n", sec.Currency)

	if sec.Exchange.Valid {
		fmt.Fprintf(w, "Exchange:    %s\n", sec.Exchange.String)
	}

	if sec.Hidden {
		fmt.Fprintf(w, "Status:      Hidden\n")
	} else {
		fmt.Fprintf(w, "Status:      Active\n")
	}
}

// printPricesTable prints prices in a formatted table.
func printPricesTable(w io.Writer, ticker string, prices []*price.Price) {
	if len(prices) == 0 {
		fmt.Fprintf(w, "No prices found for %s.\n", ticker)
		return
	}

	fmt.Fprintf(w, "PRICES: %s\n", ticker)
	fmt.Fprintln(w, strings.Repeat("=", len("PRICES: ")+len(ticker)))

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Date\tPrice\tSource")
	fmt.Fprintln(tw, "----\t-----\t------")

	for _, p := range prices {
		fmt.Fprintf(tw, "%s\t%s\t%s\n",
			p.Date.String(),
			fmt.Sprintf("%.2f", p.Price.Float64()),
			p.Source.DisplayName(),
		)
	}

	tw.Flush()

	fmt.Fprintf(w, "\nTotal: %d price(s)\n", len(prices))
}

// formatGainLoss formats a gain/loss value with percentage.
func formatGainLoss(gl types.Money, pct float64, currency string) string {
	s := formatMoney(gl, currency)
	if pct < 0 {
		return fmt.Sprintf("%s (%.1f%%)", s, pct)
	}
	return fmt.Sprintf("%s (+%.1f%%)", s, pct)
}

// printPortfolioSummary prints the investment portfolio summary with holdings.
func printPortfolioSummary(w io.Writer, acct *account.Account, valuation *investment.AccountValuation, securityMap map[types.ID]*security.Security) {
	fmt.Fprintf(w, "PORTFOLIO: %s\n", acct.Name)
	fmt.Fprintln(w, strings.Repeat("=", len("PORTFOLIO: ")+len(acct.Name)))
	fmt.Fprintln(w)

	// Holdings table
	fmt.Fprintln(w, "HOLDINGS")
	fmt.Fprintln(w, "--------")

	if len(valuation.Holdings) == 0 {
		fmt.Fprintln(w, "(No holdings)")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "Ticker\tName\tShares\tAvg Cost\tPrice\tCost Basis\tMarket Value\tGain/Loss")
		fmt.Fprintln(tw, "------\t----\t------\t--------\t-----\t----------\t------------\t---------")

		for _, h := range valuation.Holdings {
			ticker := h.SecurityID.String()[:8]
			name := ""
			if sec, ok := securityMap[h.SecurityID]; ok {
				ticker = sec.Ticker
				name = sec.Name
			}

			priceStr := "N/A"
			if h.HasPricing {
				priceStr = formatMoney(h.CurrentPrice, acct.Currency)
			}

			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				ticker,
				name,
				h.Shares.String(),
				formatMoney(h.AvgCost, acct.Currency),
				priceStr,
				formatMoney(h.CostBasis, acct.Currency),
				formatMoney(h.MarketValue, acct.Currency),
				formatGainLoss(h.GainLoss, h.GainPct, acct.Currency),
			)
		}
		tw.Flush()
	}

	// Summary
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SUMMARY")
	fmt.Fprintln(w, "-------")
	fmt.Fprintf(w, "Cash Balance:     %s\n", formatMoney(valuation.CashBalance, acct.Currency))
	fmt.Fprintf(w, "Market Value:     %s\n", formatMoney(valuation.MarketValue, acct.Currency))
	fmt.Fprintf(w, "Total Value:      %s\n", formatMoney(valuation.TotalValue, acct.Currency))
	fmt.Fprintf(w, "Total Cost Basis: %s\n", formatMoney(valuation.TotalCostBasis, acct.Currency))
	fmt.Fprintf(w, "Total Gain/Loss:  %s\n", formatGainLoss(valuation.TotalGainLoss, valuation.TotalGainPct, acct.Currency))
}

// printPortfolioWithLots prints the portfolio with lot detail for each holding.
func printPortfolioWithLots(w io.Writer, acct *account.Account, valuation *investment.AccountValuation, securityMap map[types.ID]*security.Security, svc *app.Services, asOf types.Date) {
	fmt.Fprintf(w, "PORTFOLIO: %s (with lots)\n", acct.Name)
	fmt.Fprintln(w, strings.Repeat("=", len("PORTFOLIO: ")+len(acct.Name)+len(" (with lots)")))
	fmt.Fprintln(w)

	if len(valuation.Holdings) == 0 {
		fmt.Fprintln(w, "(No holdings)")
	} else {
		for _, h := range valuation.Holdings {
			ticker := h.SecurityID.String()[:8]
			name := ""
			if sec, ok := securityMap[h.SecurityID]; ok {
				ticker = sec.Ticker
				name = sec.Name
			}

			fmt.Fprintf(w, "%s - %s\n", ticker, name)
			fmt.Fprintf(w, "  Shares: %s  Avg Cost: %s  Market Value: %s  Gain/Loss: %s\n",
				h.Shares.String(),
				formatMoney(h.AvgCost, acct.Currency),
				formatMoney(h.MarketValue, acct.Currency),
				formatGainLoss(h.GainLoss, h.GainPct, acct.Currency),
			)

			// Get lot details
			lots, err := svc.Investment.GetLotDetail(acct.ID, h.SecurityID, asOf)
			if err != nil {
				fmt.Fprintf(w, "  (could not retrieve lot details: %v)\n", err)
			} else if len(lots) == 0 {
				fmt.Fprintln(w, "  (no lots)")
			} else {
				tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "  Lot\tPurchase Date\tShares\tCost/Share\tCost Basis\tCurrent Value\tGain/Loss")
				fmt.Fprintln(tw, "  ---\t-------------\t------\t----------\t----------\t-------------\t---------")

				for _, ld := range lots {
					lotIDStr := ld.LotID.String()
					if len(lotIDStr) > 8 {
						lotIDStr = lotIDStr[:8]
					}

					fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s\t%s\n",
						lotIDStr,
						ld.PurchaseDate.String(),
						ld.Shares.String(),
						formatMoney(ld.CostPerShare, acct.Currency),
						formatMoney(ld.CostBasis, acct.Currency),
						formatMoney(ld.CurrentValue, acct.Currency),
						formatGainLoss(ld.GainLoss, ld.GainPct, acct.Currency),
					)
				}
				tw.Flush()
			}
			fmt.Fprintln(w)
		}
	}

	// Summary
	fmt.Fprintln(w, "SUMMARY")
	fmt.Fprintln(w, "-------")
	fmt.Fprintf(w, "Cash Balance:     %s\n", formatMoney(valuation.CashBalance, acct.Currency))
	fmt.Fprintf(w, "Market Value:     %s\n", formatMoney(valuation.MarketValue, acct.Currency))
	fmt.Fprintf(w, "Total Value:      %s\n", formatMoney(valuation.TotalValue, acct.Currency))
	fmt.Fprintf(w, "Total Cost Basis: %s\n", formatMoney(valuation.TotalCostBasis, acct.Currency))
	fmt.Fprintf(w, "Total Gain/Loss:  %s\n", formatGainLoss(valuation.TotalGainLoss, valuation.TotalGainPct, acct.Currency))
}
