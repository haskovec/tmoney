package transaction

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// transactionDeleteOptions are the inputs to `tmoney transaction delete <id>`.
type transactionDeleteOptions struct {
	file  string
	txnID string
}

// newTransactionDeleteCmd registers `tmoney transaction delete <id>`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. The ID is positional, matching
// `transaction void <id>`.
func newTransactionDeleteCmd() *cobra.Command {
	opts := &transactionDeleteOptions{}
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a transaction by ID",
		Long: "Permanently delete a transaction identified by its UUID (find it " +
			"with `tmoney transaction list --show-ids`). Transfer legs are " +
			"deleted with `tmoney transfer delete`; split transactions are " +
			"deleted in the TUI; void and reconciled transactions are refused. " +
			"To keep an auditable record instead, use `tmoney transaction void`.",
		Example:      "  tmoney transaction delete 0d9f7c2a-…",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.txnID = args[0]
			return runTransactionDelete(opts, cmd.OutOrStdout())
		},
	}
	return cmd
}

// runTransactionDelete deletes a transaction by ID via
// transaction.Service.Delete — the same path the TUI delete action uses.
func runTransactionDelete(opts *transactionDeleteOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	txnID, err := types.ParseID(opts.txnID)
	if err != nil {
		return fmt.Errorf("invalid transaction ID: %w", err)
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	txn, err := svc.Transaction.GetByID(txnID)
	if err != nil {
		return fmt.Errorf("transaction %s not found", opts.txnID)
	}

	if err := guardTransactionEditable(svc, txn, "deleted"); err != nil {
		return err
	}

	if err := svc.Transaction.Delete(txnID); err != nil {
		return fmt.Errorf("failed to delete transaction: %w", err)
	}

	printTransactionSummary(w, svc, "Transaction deleted successfully!", txn)

	cmdutil.AutoBackupAfterModification(database)
	return nil
}
