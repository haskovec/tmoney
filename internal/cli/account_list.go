package cli

import (
	"fmt"
	"io"
)

// runListAccounts lists accounts from the database.
func runListAccounts(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--list-accounts requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// List accounts (activeOnly = !includeClosed)
	accounts, err := svc.Account.List(!opts.includeClosed)
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}

	// Get all balances
	balances, err := svc.Account.GetAllBalances()
	if err != nil {
		return fmt.Errorf("failed to get balances: %w", err)
	}

	// Print accounts table
	printAccountsTable(w, accounts, balances)

	return nil
}
