package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// runSearch searches for transactions matching the search term and filters.
func runSearch(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--search requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Build search criteria
	criteria := transaction.SearchCriteria{
		PayeeName: opts.searchTerm,
		Memo:      opts.searchTerm,
	}

	// Parse date filters if provided
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

	// Parse account filter if provided
	if opts.accountName != "" {
		acct, err := svc.Account.GetByName(opts.accountName)
		if err != nil {
			return fmt.Errorf("account %q not found", opts.accountName)
		}
		criteria.AccountID = &acct.ID
	}

	// Parse category filter if provided
	if opts.txCategory != "" {
		criteria.CategoryName = opts.txCategory
	}

	// Parse min/max amount filters if provided
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

	// Search for transactions - we need to search by payee OR memo
	// Since the Search method uses AND logic, we'll do two searches and merge
	var transactions []*transaction.Transaction

	// Search by payee name
	payeeCriteria := criteria
	payeeCriteria.Memo = ""
	payeeResults, err := svc.TransactionRepo.Search(payeeCriteria)
	if err != nil {
		return fmt.Errorf("failed to search transactions: %w", err)
	}

	// Search by memo
	memoCriteria := criteria
	memoCriteria.PayeeName = ""
	memoResults, err := svc.TransactionRepo.Search(memoCriteria)
	if err != nil {
		return fmt.Errorf("failed to search transactions: %w", err)
	}

	// Merge results, avoiding duplicates
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

	// Build lookup maps for display
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

	// Print search results
	printSearchResults(w, opts.searchTerm, transactions, accountNames, accountCurrencies, payeeNames, categoryNames)

	return nil
}
