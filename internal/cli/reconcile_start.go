package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/types"
)

// runStartReconcile starts a reconciliation session for an account.
func runStartReconcile(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--start-reconcile requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--start-reconcile requires --account to specify an account")
	}
	if opts.statementDate == "" {
		return fmt.Errorf("--start-reconcile requires --statement-date")
	}
	if opts.statementBalance == "" {
		return fmt.Errorf("--start-reconcile requires --statement-balance")
	}

	// Parse statement date
	stmtDate, err := types.ParseDate(opts.statementDate)
	if err != nil {
		return fmt.Errorf("invalid --statement-date: %w", err)
	}

	// Parse statement balance
	stmtBalance, err := types.NewMoney(opts.statementBalance)
	if err != nil {
		return fmt.Errorf("invalid --statement-balance: %w", err)
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

	// Start reconciliation
	session, err := svc.Reconciliation.StartReconciliation(account.ID, stmtDate, stmtBalance)
	if err != nil {
		return fmt.Errorf("failed to start reconciliation: %w", err)
	}

	// Get candidate transaction count
	candidates, err := svc.Reconciliation.GetCandidateTransactions(account.ID, stmtDate)
	if err != nil {
		return fmt.Errorf("failed to get candidate transactions: %w", err)
	}

	_ = session // session created successfully
	fmt.Fprintf(w, "Reconciliation started for %s\n", account.Name)
	fmt.Fprintf(w, "  Statement date:    %s\n", stmtDate.String())
	fmt.Fprintf(w, "  Statement balance: %s\n", formatMoney(stmtBalance, account.Currency))
	fmt.Fprintf(w, "  Unreconciled transactions: %d\n", len(candidates))

	autoBackupAfterModification(opts.file)
	return nil
}
