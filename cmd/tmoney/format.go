package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/service"
)

// formatMoney formats a Money value with currency symbol.
// Always displays 2 decimal places for currencies.
func formatMoney(m models.Money, currency string) string {
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
func printAccountsTable(w io.Writer, accounts []*models.Account, balances map[models.ID]*service.AccountBalance) {
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
func printAccountDetails(w io.Writer, account *models.Account, balance *service.AccountBalance) {
	fmt.Fprintf(w, "ACCOUNT: %s\n", account.Name)
	fmt.Fprintln(w, strings.Repeat("=", len("ACCOUNT: ")+len(account.Name)))

	fmt.Fprintf(w, "Type:            %s\n", account.Type.DisplayName())
	fmt.Fprintf(w, "Currency:        %s\n", account.Currency)

	if account.Institution.Valid {
		fmt.Fprintf(w, "Institution:     %s\n", account.Institution.String)
	}

	if account.AccountNumber.Valid {
		// Mask account number for privacy
		num := account.AccountNumber.String
		if len(num) > 4 {
			num = "****" + num[len(num)-4:]
		}
		fmt.Fprintf(w, "Account Number:  %s\n", num)
	}

	fmt.Fprintf(w, "Opening Date:    %s\n", account.OpeningDate.String())
	fmt.Fprintf(w, "Opening Balance: %s\n", formatMoney(account.OpeningBalance, account.Currency))
	fmt.Fprintf(w, "Current Balance: %s\n", formatMoney(balance.CurrentBalance, account.Currency))
	fmt.Fprintf(w, "Cleared Balance: %s\n", formatMoney(balance.ClearedBalance, account.Currency))

	status := "Active"
	if !account.Active {
		status = "Closed"
	}
	fmt.Fprintf(w, "Status:          %s\n", status)

	// Type-specific details
	if account.CreditLimit.Valid {
		fmt.Fprintf(w, "Credit Limit:    %s\n", formatMoney(account.CreditLimit.Money, account.Currency))
	}
	if account.InterestRate.Valid {
		fmt.Fprintf(w, "Interest Rate:   %s%%\n", account.InterestRate.Money.String())
	}

	if account.Notes.Valid {
		fmt.Fprintf(w, "Notes:           %s\n", account.Notes.String)
	}
}

// printBalancesTable prints balances for all accounts with net worth.
func printBalancesTable(w io.Writer, accounts []*models.Account, balances map[models.ID]*service.AccountBalance) {
	if len(accounts) == 0 {
		fmt.Fprintln(w, "No accounts found.")
		return
	}

	fmt.Fprintln(w, "BALANCES")
	fmt.Fprintln(w, "========")

	var totalAssets, totalLiabilities models.Money

	for _, acct := range accounts {
		balance := models.MustNewMoney("0")
		if b, ok := balances[acct.ID]; ok {
			balance = b.CurrentBalance
		}

		fmt.Fprintf(w, "%-20s %s\n", acct.Name+":", formatMoney(balance, acct.Currency))

		// Track net worth
		if acct.Type.IsAssetType() {
			totalAssets = totalAssets.Add(balance)
		} else if acct.Type.IsLiabilityType() {
			totalLiabilities = totalLiabilities.Add(balance)
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
func printTransactionsTable(w io.Writer, account *models.Account, transactions []*models.Transaction, payeeNames map[models.ID]string, categoryNames map[models.ID]string) {
	fmt.Fprintf(w, "TRANSACTIONS: %s\n", account.Name)
	fmt.Fprintln(w, strings.Repeat("=", len("TRANSACTIONS: ")+len(account.Name)))

	if len(transactions) == 0 {
		fmt.Fprintln(w, "No transactions found.")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Date\tPayee\tCategory\tAmount\tStatus")
	fmt.Fprintln(tw, "----\t-----\t--------\t------\t------")

	for _, txn := range transactions {
		payee := "-"
		if txn.PayeeID.Valid {
			if name, ok := payeeNames[txn.PayeeID.ID]; ok {
				payee = name
			}
		}

		category := "-"
		if txn.CategoryID.Valid {
			if name, ok := categoryNames[txn.CategoryID.ID]; ok {
				category = name
			}
		}

		// For transfers, show the transfer account
		if txn.IsTransfer() {
			payee = "[Transfer]"
			category = "-"
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			txn.Date.String(),
			payee,
			category,
			formatMoney(txn.Amount, account.Currency),
			txn.Status.Code(),
		)
	}

	tw.Flush()

	fmt.Fprintf(w, "\nShowing %d transaction(s)\n", len(transactions))
}

// printSearchResults prints search results in a formatted table.
func printSearchResults(w io.Writer, searchTerm string, transactions []*models.Transaction, accountNames map[models.ID]string, accountCurrencies map[models.ID]string, payeeNames map[models.ID]string, categoryNames map[models.ID]string) {
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
		account := accountNames[txn.AccountID]
		currency := accountCurrencies[txn.AccountID]
		if currency == "" {
			currency = "USD"
		}

		payee := "-"
		if txn.PayeeID.Valid {
			if name, ok := payeeNames[txn.PayeeID.ID]; ok {
				payee = name
			}
		}

		category := "-"
		if txn.CategoryID.Valid {
			if name, ok := categoryNames[txn.CategoryID.ID]; ok {
				category = name
			}
		}

		// For transfers, show transfer indicator
		if txn.IsTransfer() {
			payee = "[Transfer]"
			category = "-"
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			account,
			txn.Date.String(),
			payee,
			category,
			formatMoney(txn.Amount, currency),
		)
	}

	tw.Flush()

	fmt.Fprintf(w, "\nFound %d transaction(s)\n", len(transactions))
}

// printScheduledTransactionsTable prints scheduled transactions in a formatted table.
func printScheduledTransactionsTable(w io.Writer, scheduledTxns []*models.ScheduledTransaction, dueOnly bool, accountNames map[models.ID]string, accountCurrencies map[models.ID]string, payeeNames map[models.ID]string, categoryNames map[models.ID]string) {
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
	fmt.Fprintln(tw, "ID\tAccount\tNext Date\tPayee\tAmount\tFrequency")
	fmt.Fprintln(tw, "--------\t-------\t---------\t-----\t------\t---------")

	for _, st := range scheduledTxns {
		// Truncate ID for display
		idStr := st.ID.String()
		if len(idStr) > 8 {
			idStr = idStr[:8]
		}

		account := accountNames[st.AccountID]
		if account == "" {
			account = "-"
		}

		currency := accountCurrencies[st.AccountID]
		if currency == "" {
			currency = "USD"
		}

		payee := "-"
		if st.HasPayee() {
			if name, ok := payeeNames[st.PayeeID.ID]; ok {
				payee = name
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

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			idStr,
			account,
			st.NextDate.String(),
			payee,
			amount,
			freq,
		)
	}

	tw.Flush()

	fmt.Fprintf(w, "\nShowing %d scheduled transaction(s)\n", len(scheduledTxns))
}

// printNetWorthReport prints the net worth report.
func printNetWorthReport(w io.Writer, report *models.NetWorthReport) {
	fmt.Fprintln(w, "NET WORTH REPORT")
	fmt.Fprintln(w, "================")
	fmt.Fprintf(w, "As of: %s\n\n", report.AsOfDate.Format("January 2, 2006"))

	// Assets section
	fmt.Fprintln(w, "ASSETS")
	fmt.Fprintln(w, "------")
	if len(report.Assets) == 0 {
		fmt.Fprintln(w, "  (No asset accounts)")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, acct := range report.Assets {
			fmt.Fprintf(tw, "  %s\t%s\n", acct.Name, formatMoney(acct.Balance, "USD"))
		}
		tw.Flush()
	}
	fmt.Fprintf(w, "\nTotal Assets:\t\t%s\n\n", formatMoney(report.TotalAssets, "USD"))

	// Liabilities section
	fmt.Fprintln(w, "LIABILITIES")
	fmt.Fprintln(w, "-----------")
	if len(report.Liabilities) == 0 {
		fmt.Fprintln(w, "  (No liability accounts)")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, acct := range report.Liabilities {
			fmt.Fprintf(tw, "  %s\t%s\n", acct.Name, formatMoney(acct.Balance, "USD"))
		}
		tw.Flush()
	}
	fmt.Fprintf(w, "\nTotal Liabilities:\t%s\n\n", formatMoney(report.TotalLiabilities, "USD"))

	// Net worth
	fmt.Fprintln(w, "========================")
	fmt.Fprintf(w, "NET WORTH:\t\t%s\n", formatMoney(report.NetWorth, "USD"))
}

// printSpendingReport prints the spending by category report.
func printSpendingReport(w io.Writer, report *models.SpendingReport) {
	fmt.Fprintln(w, "SPENDING BY CATEGORY")
	fmt.Fprintln(w, "====================")
	fmt.Fprintf(w, "Period: %s\n\n", report.Period)

	if len(report.Categories) == 0 {
		fmt.Fprintln(w, "No spending found for this period.")
		return
	}

	// Print category spending with visual bars
	maxBarWidth := 30
	maxAmount := models.ZeroMoney
	for _, cat := range report.Categories {
		if cat.Amount.Cmp(maxAmount) > 0 {
			maxAmount = cat.Amount
		}
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Category\tAmount\t%\tBar")
	fmt.Fprintln(tw, "--------\t------\t-\t---")

	for _, cat := range report.Categories {
		// Calculate bar length
		barLen := 0
		if !maxAmount.IsZero() {
			barLen = int(cat.Amount.Float64() / maxAmount.Float64() * float64(maxBarWidth))
		}
		bar := strings.Repeat("█", barLen)

		fmt.Fprintf(tw, "%s\t%s\t%.1f%%\t%s\n",
			cat.Name,
			formatMoney(cat.Amount, "USD"),
			cat.Percentage,
			bar,
		)

		// Print subcategories with indentation
		for _, sub := range cat.Subcategories {
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

	fmt.Fprintf(w, "\n------------------------\nTotal Spending:\t%s\n", formatMoney(report.TotalSpending, "USD"))
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

For more information, visit: https://github.com/haskovec/tmoney`)
}
