package reconcile

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/reconciliation"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// reconcileFinishOptions are the inputs to `tmoney reconcile finish`.
type reconcileFinishOptions struct {
	file    string
	account string
	force   bool
}

// newReconcileFinishCmd registers `tmoney reconcile finish`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command.
func newReconcileFinishCmd() *cobra.Command {
	opts := &reconcileFinishOptions{}
	cmd := &cobra.Command{
		Use:   "finish",
		Short: "Finish the active reconciliation session for an account",
		Long: "Complete the active reconciliation session: marks every " +
			"candidate transaction reconciled and closes the session. " +
			"Refuses to finish when the cleared total does not match the " +
			"statement balance unless --force is given.",
		Example:      "  tmoney reconcile finish --account Checking --file personal.tdb",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runReconcileFinish(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Account name to finish reconciliation for (required)")
	cmd.Flags().BoolVar(&opts.force, "force", false, "Finish even if the cleared total differs from the statement balance")
	_ = cmd.MarkFlagRequired("account")
	return cmd
}

// runReconcileFinish completes the reconciliation for an account.
func runReconcileFinish(opts *reconcileFinishOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	account, err := svc.Account.GetByName(opts.account)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.account)
	}

	session, err := svc.Reconciliation.GetActiveSession(account.ID)
	if err != nil {
		return fmt.Errorf("failed to get reconciliation session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("no active reconciliation session for %s", account.Name)
	}

	candidates, err := svc.Reconciliation.GetCandidateTransactions(account.ID, session.StatementDate)
	if err != nil {
		return fmt.Errorf("failed to get candidate transactions: %w", err)
	}

	var txnIDs []types.ID
	for _, txn := range candidates {
		txnIDs = append(txnIDs, txn.ID)
	}

	err = svc.Reconciliation.FinishReconciliation(account.ID, txnIDs, opts.force)
	if err != nil {
		if diffErr, ok := err.(*reconciliation.DifferenceError); ok {
			return fmt.Errorf("cannot complete reconciliation. Difference: %s\nUse `tmoney reconcile mark` to mark additional transactions, or --force to complete anyway",
				cmdutil.FormatMoney(diffErr.Difference, account.Currency))
		}
		return fmt.Errorf("failed to finish reconciliation: %w", err)
	}

	fmt.Fprintf(w, "Reconciliation completed for %s\n", account.Name)
	fmt.Fprintf(w, "  Statement date:         %s\n", session.StatementDate.String())
	fmt.Fprintf(w, "  Transactions reconciled: %d\n", len(txnIDs))
	fmt.Fprintf(w, "  Balance:                %s\n", cmdutil.FormatMoney(session.StatementBalance, account.Currency))

	cmdutil.AutoBackupAfterModification(database)
	return nil
}
