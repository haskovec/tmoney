package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// runTransfer creates a transfer between two accounts.
func runTransfer(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--transfer requires --file to specify a database")
	}
	if opts.fromAccount == "" {
		return fmt.Errorf("--transfer requires --from to specify the source account")
	}
	if opts.toAccount == "" {
		return fmt.Errorf("--transfer requires --to to specify the destination account")
	}
	if opts.txAmount == "" {
		return fmt.Errorf("--transfer requires --amount to specify the transfer amount")
	}

	// Parse amount
	amount, err := types.NewMoney(opts.txAmount)
	if err != nil {
		return fmt.Errorf("invalid --amount: %w", err)
	}

	// Amount must be positive for transfers
	if !amount.IsPositive() {
		return fmt.Errorf("--amount must be positive for transfers")
	}

	// Parse date (default to today)
	var date types.Date
	if opts.txDate != "" {
		date, err = types.ParseDate(opts.txDate)
		if err != nil {
			return fmt.Errorf("invalid --date: %w", err)
		}
	} else {
		date = types.Today()
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Get source account by name
	fromAcct, err := svc.Account.GetByName(opts.fromAccount)
	if err != nil {
		return fmt.Errorf("source account %q not found", opts.fromAccount)
	}

	// Get destination account by name
	toAcct, err := svc.Account.GetByName(opts.toAccount)
	if err != nil {
		return fmt.Errorf("destination account %q not found", opts.toAccount)
	}

	// Create the transfer
	pair, err := svc.Transaction.CreateTransfer(fromAcct.ID, toAcct.ID, date, amount)
	if err != nil {
		return fmt.Errorf("failed to create transfer: %w", err)
	}

	// Set memo if provided
	if opts.txMemo != "" {
		err = svc.Transaction.UpdateTransfer(pair.FromTransaction.TransferID.ID, date, amount, opts.txMemo, transaction.StatusUncleared)
		if err != nil {
			return fmt.Errorf("failed to set memo on transfer: %w", err)
		}
	}

	// Print confirmation
	fmt.Fprintln(w, "Transfer created successfully!")
	fmt.Fprintf(w, "  From:   %s\n", fromAcct.Name)
	fmt.Fprintf(w, "  To:     %s\n", toAcct.Name)
	fmt.Fprintf(w, "  Date:   %s\n", date.String())
	fmt.Fprintf(w, "  Amount: %s\n", formatMoney(amount, fromAcct.Currency))
	if opts.txMemo != "" {
		fmt.Fprintf(w, "  Memo:   %s\n", opts.txMemo)
	}

	autoBackupAfterModification(opts.file)
	return nil
}
