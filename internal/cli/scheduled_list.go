package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

// runScheduled lists scheduled transactions.
func runScheduled(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--scheduled requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Get scheduled transactions
	var scheduledTxns []*scheduled.Transaction
	if opts.scheduledDue {
		scheduledTxns, err = svc.Scheduled.ListDue()
	} else {
		scheduledTxns, err = svc.Scheduled.List()
	}
	if err != nil {
		return fmt.Errorf("failed to list scheduled transactions: %w", err)
	}

	// Filter by account if specified
	if opts.accountName != "" {
		acct, err := svc.Account.GetByName(opts.accountName)
		if err != nil {
			return fmt.Errorf("account %q not found", opts.accountName)
		}

		var filtered []*scheduled.Transaction
		for _, st := range scheduledTxns {
			if st.AccountID == acct.ID {
				filtered = append(filtered, st)
			}
		}
		scheduledTxns = filtered
	}

	// Build lookup maps
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

	// Print scheduled transactions table
	printScheduledTransactionsTable(w, scheduledTxns, opts.scheduledDue, accountNames, accountCurrencies, payeeNames, categoryNames)

	return nil
}
