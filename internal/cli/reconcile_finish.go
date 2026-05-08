package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/reconciliation"
	"github.com/haskovec/tmoney/internal/types"
)

// runFinishReconcile completes the reconciliation for an account.
func runFinishReconcile(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--finish-reconcile requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--finish-reconcile requires --account to specify an account")
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

	// Get active session
	session, err := svc.Reconciliation.GetActiveSession(account.ID)
	if err != nil {
		return fmt.Errorf("failed to get reconciliation session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("no active reconciliation session for %s", account.Name)
	}

	// Get all candidate transactions to mark as reconciled
	candidates, err := svc.Reconciliation.GetCandidateTransactions(account.ID, session.StatementDate)
	if err != nil {
		return fmt.Errorf("failed to get candidate transactions: %w", err)
	}

	// Collect all candidate transaction IDs
	var txnIDs []types.ID
	for _, txn := range candidates {
		txnIDs = append(txnIDs, txn.ID)
	}

	// Finish reconciliation
	err = svc.Reconciliation.FinishReconciliation(account.ID, txnIDs, opts.reconcileForce)
	if err != nil {
		// Check for difference error and provide helpful message
		if diffErr, ok := err.(*reconciliation.DifferenceError); ok {
			return fmt.Errorf("cannot complete reconciliation. Difference: %s\nUse --mark-reconciled to mark additional transactions, or --force to complete anyway",
				formatMoney(diffErr.Difference, account.Currency))
		}
		return fmt.Errorf("failed to finish reconciliation: %w", err)
	}

	fmt.Fprintf(w, "Reconciliation completed for %s\n", account.Name)
	fmt.Fprintf(w, "  Statement date:         %s\n", session.StatementDate.String())
	fmt.Fprintf(w, "  Transactions reconciled: %d\n", len(txnIDs))
	fmt.Fprintf(w, "  Balance:                %s\n", formatMoney(session.StatementBalance, account.Currency))

	autoBackupAfterModification(opts.file)
	return nil
}
