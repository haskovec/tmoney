package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// runTransactions lists transactions for an account.
func runTransactions(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--transactions requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--transactions requires --account to specify an account")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Get account by name
	acct, err := svc.Account.GetByName(opts.accountName)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.accountName)
	}

	// Parse date filters if provided
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

	// Fetch transactions
	var transactions []*transaction.Transaction
	if hasDateFilter {
		// If only one date provided, use it for both bounds
		if opts.fromDate == "" {
			startDate = types.Date{} // Zero date (far past)
		}
		if opts.toDate == "" {
			endDate = types.Today() // Today
		}
		transactions, err = svc.Transaction.ListByAccountAndDateRange(acct.ID, startDate, endDate)
	} else {
		transactions, err = svc.Transaction.ListByAccount(acct.ID)
	}
	if err != nil {
		return fmt.Errorf("failed to list transactions: %w", err)
	}

	// Filter by status if specified
	if opts.txStatus != "" {
		status, err := transaction.ParseStatus(opts.txStatus)
		if err != nil {
			return fmt.Errorf("invalid --status: %w", err)
		}
		var filtered []*transaction.Transaction
		for _, txn := range transactions {
			if txn.Status == status {
				filtered = append(filtered, txn)
			}
		}
		transactions = filtered
	}

	// Apply limit if specified
	if opts.limit > 0 && len(transactions) > opts.limit {
		transactions = transactions[:opts.limit]
	}

	// Build payee and category lookup maps
	payeeNames := make(map[types.ID]string)
	categoryNames := make(map[types.ID]string)

	// Fetch all payees and categories for name lookup
	payees, _ := svc.PayeeRepo.List()
	for _, p := range payees {
		payeeNames[p.ID] = p.Name
	}

	categories, _ := svc.CategoryRepo.List()
	for _, c := range categories {
		categoryNames[c.ID] = c.Name
	}

	// Print transactions table
	printTransactionsTable(w, acct, transactions, payeeNames, categoryNames)

	return nil
}
