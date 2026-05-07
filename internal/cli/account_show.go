package cli

import (
	"fmt"
	"io"
)

// runAccountDetails shows detailed information for a specific account.
func runAccountDetails(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--account requires --file to specify a database")
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

	// Get balance
	bal, err := svc.Account.GetBalance(acct.ID)
	if err != nil {
		return fmt.Errorf("failed to get balance: %w", err)
	}

	// Print account details
	printAccountDetails(w, acct, bal)

	return nil
}
