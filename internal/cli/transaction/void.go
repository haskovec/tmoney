package transaction

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// transactionVoidOptions are the inputs to `tmoney transaction void <id>`.
type transactionVoidOptions struct {
	file  string
	txnID string
}

// newTransactionVoidCmd registers `tmoney transaction void <id>`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command.
func newTransactionVoidCmd() *cobra.Command {
	opts := &transactionVoidOptions{}
	cmd := &cobra.Command{
		Use:          "void <id>",
		Short:        "Void a transaction by ID",
		Long:         "Void a transaction, zeroing its amount and marking it as void. If the transaction is part of a transfer, the counterpart is voided as well.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.txnID = args[0]
			return runTransactionVoid(opts, cmd.OutOrStdout())
		},
	}
	return cmd
}

// runTransactionVoid voids a transaction by ID.
func runTransactionVoid(opts *transactionVoidOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	txnID, err := types.ParseID(opts.txnID)
	if err != nil {
		return fmt.Errorf("invalid transaction ID: %w", err)
	}

	txn, err := svc.TransactionRepo.GetByID(txnID)
	if err != nil {
		return fmt.Errorf("transaction not found: %w", err)
	}

	acct, _ := svc.AccountRepo.GetByID(txn.AccountID)
	accountName := "Unknown"
	currency := "USD"
	if acct != nil {
		accountName = acct.Name
		currency = acct.Currency
	}

	originalAmount := txn.Amount

	// A whole-transaction transfer leg is voided through the transfer owner, which
	// zeroes BOTH legs wherever they live. transaction.Service.VoidTransaction
	// refuses one outright — it writes a single row, and a transfer's counterpart
	// may be in investment_transactions.
	//
	// Resolve first, so an investment-involving transfer reports
	// *transfer.VoidNotSupportedError by name (the investment ledger has no void
	// status) rather than a generic failure.
	if txn.IsTransfer() {
		resolved, rerr := svc.Transfer.Resolve(txnID)
		if rerr != nil {
			return fmt.Errorf("failed to resolve transfer: %w", rerr)
		}
		if _, verr := svc.Transfer.Void(resolved.TransferID); verr != nil {
			return fmt.Errorf("failed to void transfer: %w", verr)
		}
	} else if err := svc.Transaction.VoidTransaction(txnID); err != nil {
		return fmt.Errorf("failed to void transaction: %w", err)
	}

	fmt.Fprintln(w, "Transaction voided successfully!")
	fmt.Fprintf(w, "  Account:  %s\n", accountName)
	fmt.Fprintf(w, "  Date:     %s\n", txn.Date.String())
	fmt.Fprintf(w, "  Amount:   %s (was %s)\n", cmdutil.FormatMoney(types.ZeroMoney, currency), cmdutil.FormatMoney(originalAmount, currency))
	fmt.Fprintf(w, "  Status:   Void\n")
	if txn.IsTransfer() {
		fmt.Fprintln(w, "  Note:     Transfer counterpart was also voided")
	}

	cmdutil.AutoBackupAfterModification(database)
	return nil
}
