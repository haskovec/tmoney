package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// reconcileStartOptions are the inputs to `tmoney reconcile start`.
type reconcileStartOptions struct {
	file             string
	account          string
	statementDate    string
	statementBalance string
}

// newReconcileStartCmd registers `tmoney reconcile start`. The database
// file is taken from the persistent `--file` / `-f` flag inherited from
// the root command.
func newReconcileStartCmd() *cobra.Command {
	opts := &reconcileStartOptions{}
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a reconciliation session for an account",
		Long: "Start a reconciliation session against a statement: " +
			"records the statement date and balance and reports how many " +
			"unreconciled transactions are eligible to be marked.",
		Example:      "  tmoney reconcile start --account Checking --statement-date 2024-01-31 --statement-balance 850.00 --file personal.tdb",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runReconcileStart(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Account name to reconcile (required)")
	cmd.Flags().StringVar(&opts.statementDate, "statement-date", "", "Statement closing date YYYY-MM-DD (required)")
	cmd.Flags().StringVar(&opts.statementBalance, "statement-balance", "", "Statement closing balance (required)")
	_ = cmd.MarkFlagRequired("account")
	_ = cmd.MarkFlagRequired("statement-date")
	_ = cmd.MarkFlagRequired("statement-balance")
	return cmd
}

// runReconcileStart starts a reconciliation session for an account.
func runReconcileStart(opts *reconcileStartOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--file is required to specify a database")
	}

	stmtDate, err := types.ParseDate(opts.statementDate)
	if err != nil {
		return fmt.Errorf("invalid --statement-date: %w", err)
	}

	stmtBalance, err := types.NewMoney(opts.statementBalance)
	if err != nil {
		return fmt.Errorf("invalid --statement-balance: %w", err)
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	account, err := svc.Account.GetByName(opts.account)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.account)
	}

	session, err := svc.Reconciliation.StartReconciliation(account.ID, stmtDate, stmtBalance)
	if err != nil {
		return fmt.Errorf("failed to start reconciliation: %w", err)
	}

	candidates, err := svc.Reconciliation.GetCandidateTransactions(account.ID, stmtDate)
	if err != nil {
		return fmt.Errorf("failed to get candidate transactions: %w", err)
	}

	_ = session
	fmt.Fprintf(w, "Reconciliation started for %s\n", account.Name)
	fmt.Fprintf(w, "  Statement date:    %s\n", stmtDate.String())
	fmt.Fprintf(w, "  Statement balance: %s\n", formatMoney(stmtBalance, account.Currency))
	fmt.Fprintf(w, "  Unreconciled transactions: %d\n", len(candidates))

	autoBackupAfterModification(opts.file)
	return nil
}
