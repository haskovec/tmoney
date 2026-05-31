package reconcile

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/spf13/cobra"
)

// reconcileStatusOptions are the inputs to `tmoney reconcile status`.
type reconcileStatusOptions struct {
	file    string
	account string
}

// newReconcileStatusCmd registers `tmoney reconcile status`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command.
func newReconcileStatusCmd() *cobra.Command {
	opts := &reconcileStatusOptions{}
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show reconciliation status for an account",
		Long: "Display the last completed reconciliation and any active " +
			"session for the named account.",
		Example:      "  tmoney reconcile status --account Checking --file personal.tdb",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runReconcileStatus(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Account name to show status for (required)")
	_ = cmd.MarkFlagRequired("account")
	return cmd
}

// runReconcileStatus shows the reconciliation status for an account.
func runReconcileStatus(opts *reconcileStatusOptions, w io.Writer) error {
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

	status, err := svc.Reconciliation.GetReconciliationStatus(account.ID)
	if err != nil {
		return fmt.Errorf("failed to get reconciliation status: %w", err)
	}

	printReconcileStatus(w, account, status)
	return nil
}
