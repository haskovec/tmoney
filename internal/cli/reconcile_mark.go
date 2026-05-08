package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// reconcileMarkOptions are the inputs to `tmoney reconcile mark`.
type reconcileMarkOptions struct {
	file string
	ids  []string
}

// newReconcileMarkCmd registers `tmoney reconcile mark <id>...`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command.
func newReconcileMarkCmd() *cobra.Command {
	opts := &reconcileMarkOptions{}
	cmd := &cobra.Command{
		Use:   "mark <id>...",
		Short: "Mark transactions for reconciliation",
		Long: "Mark one or more transactions as part of the active " +
			"reconciliation session for their account. Reports the " +
			"running difference between the cleared total and the " +
			"statement balance.",
		Example:      "  tmoney reconcile mark <id1> <id2> --file personal.tdb",
		Args:         cobra.MinimumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.ids = args
			return runReconcileMark(opts, cmd.OutOrStdout())
		},
	}
	return cmd
}

// runReconcileMark marks transactions for reconciliation.
func runReconcileMark(opts *reconcileMarkOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--file is required to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	var txnIDs []types.ID
	for _, idStr := range opts.ids {
		id, err := types.ParseID(idStr)
		if err != nil {
			return fmt.Errorf("invalid transaction ID %q: %w", idStr, err)
		}
		txnIDs = append(txnIDs, id)
	}

	firstTxn, err := svc.TransactionRepo.GetByID(txnIDs[0])
	if err != nil {
		return fmt.Errorf("transaction not found: %w", err)
	}

	session, err := svc.Reconciliation.GetActiveSession(firstTxn.AccountID)
	if err != nil {
		return fmt.Errorf("failed to get reconciliation session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("no active reconciliation session for this account; use `tmoney reconcile start` first")
	}

	clearedTotal, err := svc.Reconciliation.CalculateClearedTotal(firstTxn.AccountID, txnIDs)
	if err != nil {
		return fmt.Errorf("failed to calculate cleared total: %w", err)
	}

	difference := session.StatementBalance.Sub(clearedTotal)

	account, _ := svc.AccountRepo.GetByID(firstTxn.AccountID)
	currency := "USD"
	if account != nil {
		currency = account.Currency
	}

	fmt.Fprintf(w, "Marked %d transaction(s) for reconciliation\n", len(txnIDs))
	fmt.Fprintf(w, "  Current difference: %s\n", formatMoney(difference, currency))

	return nil
}
