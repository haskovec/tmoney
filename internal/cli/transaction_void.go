package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/types"
)

// runVoidTransaction voids a transaction by ID.
func runVoidTransaction(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--void requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Parse the transaction ID
	txnID, err := types.ParseID(opts.voidTxn)
	if err != nil {
		return fmt.Errorf("invalid transaction ID: %w", err)
	}

	// Get the transaction first to show details
	txn, err := svc.TransactionRepo.GetByID(txnID)
	if err != nil {
		return fmt.Errorf("transaction not found: %w", err)
	}

	// Get account info for display
	acct, _ := svc.AccountRepo.GetByID(txn.AccountID)
	accountName := "Unknown"
	currency := "USD"
	if acct != nil {
		accountName = acct.Name
		currency = acct.Currency
	}

	// Remember original amount for confirmation display
	originalAmount := txn.Amount

	// Void the transaction
	if err := svc.Transaction.VoidTransaction(txnID); err != nil {
		return fmt.Errorf("failed to void transaction: %w", err)
	}

	// Print confirmation
	fmt.Fprintln(w, "Transaction voided successfully!")
	fmt.Fprintf(w, "  Account:  %s\n", accountName)
	fmt.Fprintf(w, "  Date:     %s\n", txn.Date.String())
	fmt.Fprintf(w, "  Amount:   %s (was %s)\n", formatMoney(types.ZeroMoney, currency), formatMoney(originalAmount, currency))
	fmt.Fprintf(w, "  Status:   Void\n")
	if txn.IsTransfer() {
		fmt.Fprintln(w, "  Note:     Transfer counterpart was also voided")
	}

	autoBackupAfterModification(opts.file)
	return nil
}
