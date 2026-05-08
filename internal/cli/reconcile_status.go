package cli

import (
	"fmt"
	"io"
)

// runReconcileStatus shows the reconciliation status for an account.
func runReconcileStatus(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--reconcile-status requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--reconcile-status requires --account to specify an account")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Get account by name
	account, err := svc.Account.GetByName(opts.accountName)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.accountName)
	}

	// Get reconciliation status
	status, err := svc.Reconciliation.GetReconciliationStatus(account.ID)
	if err != nil {
		return fmt.Errorf("failed to get reconciliation status: %w", err)
	}

	printReconcileStatus(w, account, status)
	return nil
}
