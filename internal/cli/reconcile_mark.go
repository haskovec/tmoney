package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/types"
)

// runMarkReconciled marks transactions for reconciliation.
func runMarkReconciled(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--mark-reconciled requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Parse transaction IDs
	var txnIDs []types.ID
	for _, idStr := range opts.markReconciled {
		id, err := types.ParseID(idStr)
		if err != nil {
			return fmt.Errorf("invalid transaction ID %q: %w", idStr, err)
		}
		txnIDs = append(txnIDs, id)
	}

	// Get the first transaction to find its account
	firstTxn, err := svc.TransactionRepo.GetByID(txnIDs[0])
	if err != nil {
		return fmt.Errorf("transaction not found: %w", err)
	}

	// Get active session for this account
	session, err := svc.Reconciliation.GetActiveSession(firstTxn.AccountID)
	if err != nil {
		return fmt.Errorf("failed to get reconciliation session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("no active reconciliation session for this account; use --start-reconcile first")
	}

	// Calculate cleared total with these transactions marked
	clearedTotal, err := svc.Reconciliation.CalculateClearedTotal(firstTxn.AccountID, txnIDs)
	if err != nil {
		return fmt.Errorf("failed to calculate cleared total: %w", err)
	}

	difference := session.StatementBalance.Sub(clearedTotal)

	// Get account for currency
	account, _ := svc.AccountRepo.GetByID(firstTxn.AccountID)
	currency := "USD"
	if account != nil {
		currency = account.Currency
	}

	fmt.Fprintf(w, "Marked %d transaction(s) for reconciliation\n", len(txnIDs))
	fmt.Fprintf(w, "  Current difference: %s\n", formatMoney(difference, currency))

	return nil
}
