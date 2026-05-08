package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// transactionSearchOptions are the inputs to `tmoney transaction search <term>`.
type transactionSearchOptions struct {
	file      string
	term      string
	account   string
	category  string
	fromDate  string
	toDate    string
	minAmount string
	maxAmount string
}

// newTransactionSearchCmd registers `tmoney transaction search <term>`.
// The database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command.
func newTransactionSearchCmd() *cobra.Command {
	opts := &transactionSearchOptions{}
	cmd := &cobra.Command{
		Use:          "search <term>",
		Short:        "Search transactions by payee or memo",
		Long:         "Search transactions whose payee name or memo contains the term, optionally filtered by account, category, date range, or amount range.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.term = args[0]
			return runTransactionSearch(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Limit to transactions on this account")
	cmd.Flags().StringVar(&opts.category, "category", "", "Limit to transactions in this category")
	cmd.Flags().StringVar(&opts.fromDate, "from", "", "Earliest date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&opts.toDate, "to", "", "Latest date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&opts.minAmount, "min", "", "Minimum amount (most negative for expenses)")
	cmd.Flags().StringVar(&opts.maxAmount, "max", "", "Maximum amount")
	return cmd
}

// runTransactionSearch searches for transactions matching the search
// term and filters.
func runTransactionSearch(opts *transactionSearchOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--file is required to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	criteria := transaction.SearchCriteria{
		PayeeName: opts.term,
		Memo:      opts.term,
	}

	if opts.fromDate != "" {
		startDate, err := types.ParseDate(opts.fromDate)
		if err != nil {
			return fmt.Errorf("invalid --from date: %w", err)
		}
		criteria.StartDate = &startDate
	}

	if opts.toDate != "" {
		endDate, err := types.ParseDate(opts.toDate)
		if err != nil {
			return fmt.Errorf("invalid --to date: %w", err)
		}
		criteria.EndDate = &endDate
	}

	if opts.account != "" {
		acct, err := svc.Account.GetByName(opts.account)
		if err != nil {
			return fmt.Errorf("account %q not found", opts.account)
		}
		criteria.AccountID = &acct.ID
	}

	if opts.category != "" {
		criteria.CategoryName = opts.category
	}

	if opts.minAmount != "" {
		minAmt, err := types.NewMoney(opts.minAmount)
		if err != nil {
			return fmt.Errorf("invalid --min amount: %w", err)
		}
		criteria.MinAmount = &minAmt
	}

	if opts.maxAmount != "" {
		maxAmt, err := types.NewMoney(opts.maxAmount)
		if err != nil {
			return fmt.Errorf("invalid --max amount: %w", err)
		}
		criteria.MaxAmount = &maxAmt
	}

	// The repository's Search uses AND logic across PayeeName and Memo,
	// but we want OR semantics: match if either field contains the term.
	// Run two queries and merge the results.
	var transactions []*transaction.Transaction

	payeeCriteria := criteria
	payeeCriteria.Memo = ""
	payeeResults, err := svc.TransactionRepo.Search(payeeCriteria)
	if err != nil {
		return fmt.Errorf("failed to search transactions: %w", err)
	}

	memoCriteria := criteria
	memoCriteria.PayeeName = ""
	memoResults, err := svc.TransactionRepo.Search(memoCriteria)
	if err != nil {
		return fmt.Errorf("failed to search transactions: %w", err)
	}

	seen := make(map[string]bool)
	for _, txn := range payeeResults {
		if !seen[txn.ID.String()] {
			seen[txn.ID.String()] = true
			transactions = append(transactions, txn)
		}
	}
	for _, txn := range memoResults {
		if !seen[txn.ID.String()] {
			seen[txn.ID.String()] = true
			transactions = append(transactions, txn)
		}
	}

	payeeNames := make(map[types.ID]string)
	categoryNames := make(map[types.ID]string)
	accountNames := make(map[types.ID]string)
	accountCurrencies := make(map[types.ID]string)

	payees, _ := svc.PayeeRepo.List()
	for _, p := range payees {
		payeeNames[p.ID] = p.Name
	}

	categories, _ := svc.CategoryRepo.List()
	for _, c := range categories {
		categoryNames[c.ID] = c.Name
	}

	accounts, _ := svc.AccountRepo.List(false)
	for _, a := range accounts {
		accountNames[a.ID] = a.Name
		accountCurrencies[a.ID] = a.Currency
	}

	printSearchResults(w, opts.term, transactions, accountNames, accountCurrencies, payeeNames, categoryNames)

	return nil
}
