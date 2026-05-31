package transaction

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	transactiondom "github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// transactionListOptions are the inputs to `tmoney transaction list`.
type transactionListOptions struct {
	file     string
	account  string
	limit    int
	fromDate string
	toDate   string
	status   string
	showIDs  bool
}

// newTransactionListCmd registers `tmoney transaction list`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. `--account` is required.
func newTransactionListCmd() *cobra.Command {
	opts := &transactionListOptions{}
	cmd := &cobra.Command{
		Use:          "list",
		Short:        "List transactions for an account",
		Long:         "List transactions on an account, optionally filtered by date range, cleared status, or row limit.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			if opts.limit < 0 {
				return fmt.Errorf("--limit must be a non-negative number")
			}
			return runTransactionList(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Account name (required)")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "Maximum number of transactions to display (0 = no limit)")
	cmd.Flags().StringVar(&opts.fromDate, "from", "", "Earliest date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&opts.toDate, "to", "", "Latest date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&opts.status, "status", "", "Filter by status: uncleared, cleared, reconciled, void")
	cmd.Flags().BoolVar(&opts.showIDs, "show-ids", false, "Prefix each row with the transaction's UUID")
	_ = cmd.MarkFlagRequired("account")
	return cmd
}

// runTransactionList lists transactions for an account.
func runTransactionList(opts *transactionListOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	acct, err := svc.Account.GetByName(opts.account)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.account)
	}

	var startDate, endDate types.Date
	hasDateFilter := false

	if opts.fromDate != "" {
		startDate, err = types.ParseDate(opts.fromDate)
		if err != nil {
			return fmt.Errorf("invalid --from date: %w", err)
		}
		hasDateFilter = true
	}

	if opts.toDate != "" {
		endDate, err = types.ParseDate(opts.toDate)
		if err != nil {
			return fmt.Errorf("invalid --to date: %w", err)
		}
		hasDateFilter = true
	}

	var transactions []*transactiondom.Transaction
	if hasDateFilter {
		if opts.fromDate == "" {
			startDate = types.Date{}
		}
		if opts.toDate == "" {
			endDate = types.Today()
		}
		transactions, err = svc.Transaction.ListByAccountAndDateRange(acct.ID, startDate, endDate)
	} else {
		transactions, err = svc.Transaction.ListByAccount(acct.ID)
	}
	if err != nil {
		return fmt.Errorf("failed to list transactions: %w", err)
	}

	if opts.status != "" {
		status, perr := transactiondom.ParseStatus(opts.status)
		if perr != nil {
			return fmt.Errorf("invalid --status: %w", perr)
		}
		var filtered []*transactiondom.Transaction
		for _, txn := range transactions {
			if txn.Status == status {
				filtered = append(filtered, txn)
			}
		}
		transactions = filtered
	}

	if opts.limit > 0 && len(transactions) > opts.limit {
		transactions = transactions[:opts.limit]
	}

	payeeNames := make(map[types.ID]string)
	categoryNames := make(map[types.ID]string)

	payees, _ := svc.PayeeRepo.List()
	for _, p := range payees {
		payeeNames[p.ID] = p.Name
	}

	categories, _ := svc.CategoryRepo.List()
	for _, c := range categories {
		categoryNames[c.ID] = c.Name
	}

	printTransactionsTable(w, acct, transactions, payeeNames, categoryNames, opts.showIDs)

	return nil
}
