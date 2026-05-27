package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/app"
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
	if opts.file == "" {
		return fmt.Errorf("--file is required to specify a database")
	}

	legID, err := types.ParseID(opts.txnID)
	if err != nil {
		return fmt.Errorf("invalid --txn-id: %w", err)
	}

	database, svc, err := openServices(opts.file)
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

	if err := dispatchTransferDelete(svc, res); err != nil {
		return fmt.Errorf("failed to delete transfer: %w", err)
	}

	fmt.Fprintln(w, "Transfer deleted successfully!")
	fmt.Fprintf(w, "  Transfer ID:   %s\n", res.transferID)
	fmt.Fprintf(w, "  From:          %s\n", res.fromAccount.Name)
	fmt.Fprintf(w, "  To:            %s\n", res.toAccount.Name)
	fmt.Fprintf(w, "  Date:          %s\n", res.date.String())
	fmt.Fprintf(w, "  Amount:        %s\n", formatMoney(res.amount, res.fromAccount.Currency))

	autoBackupAfterModification(opts.file)
	return nil
}

// dispatchTransferDelete deletes both legs of the resolved transfer. reg↔reg
// goes through transaction.Service.DeleteTransfer; every inv-involving kind
// deletes the investment-side leg, whose service cascades to the counterpart.
func dispatchTransferDelete(svc *app.Services, res *resolvedTransfer) error {
	if res.kind == transaction.DispatchRegToReg {
		return svc.Transaction.DeleteTransfer(res.transferID)
	}
	return svc.Investment.DeleteTransaction(res.investmentTxnID)
}
