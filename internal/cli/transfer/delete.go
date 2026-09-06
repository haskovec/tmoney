package transfer

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// transferDeleteOptions are the inputs to `tmoney transfer delete`.
type transferDeleteOptions struct {
	file  string
	txnID string
}

// newTransferDeleteCmd registers `tmoney transfer delete`. The database file
// is taken from the persistent `--file` / `-f` flag inherited from the root
// command. `--txn-id` (the UUID of either leg of the transfer) is required.
func newTransferDeleteCmd() *cobra.Command {
	opts := &transferDeleteOptions{}
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete both legs of a transfer",
		Long: "Delete both legs of a whole-transaction transfer, identified by the UUID " +
			"of either leg (`--txn-id`). Use `tmoney transaction list --show-ids` to find " +
			"the ID. Works for every account-type combination (bank↔bank, bank↔investment, " +
			"investment↔investment). Reconciled transfers and multi-line splits are refused.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runTransferDelete(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.txnID, "txn-id", "", "UUID of either leg of the transfer (required)")
	_ = cmd.MarkFlagRequired("txn-id")
	return cmd
}

// runTransferDelete removes both legs of a transfer. The (from, to) account
// types pick which service method performs the cascade (see resolveTransferPair
// and transaction.ChooseTransferDispatch).
func runTransferDelete(opts *transferDeleteOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	legID, err := types.ParseID(opts.txnID)
	if err != nil {
		return fmt.Errorf("invalid --txn-id: %w", err)
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	res, err := resolveTransferPair(svc, legID)
	if err != nil {
		return err
	}

	if res.status == transaction.StatusReconciled {
		return fmt.Errorf("transfer is reconciled and cannot be deleted; unreconcile it first")
	}

	// One call for every shape. dispatchTransferDelete used to send reg↔reg to
	// transaction.DeleteTransfer and every inv-involving kind to
	// investment.DeleteTransaction, relying on that method's transfer_cash
	// cascade to reach the counterpart.
	if _, err := svc.Transfer.Delete(res.transferID); err != nil {
		return fmt.Errorf("failed to delete transfer: %w", err)
	}

	fmt.Fprintln(w, "Transfer deleted successfully!")
	fmt.Fprintf(w, "  Transfer ID:   %s\n", res.transferID)
	fmt.Fprintf(w, "  From:          %s\n", res.fromAccount.Name)
	fmt.Fprintf(w, "  To:            %s\n", res.toAccount.Name)
	fmt.Fprintf(w, "  Date:          %s\n", res.date.String())
	fmt.Fprintf(w, "  Amount:        %s\n", cmdutil.FormatMoney(res.amount, res.fromAccount.Currency))

	cmdutil.AutoBackupAfterModification(database)
	return nil
}
