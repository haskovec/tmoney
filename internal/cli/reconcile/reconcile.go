package reconcile

import (
	"github.com/spf13/cobra"
)

// NewCmd returns the `reconcile` parent command. Child verbs
// are registered as they migrate from the legacy `--flag` dispatcher.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Reconcile an account against a statement",
		Long: "Subcommands for starting, marking, finishing, and inspecting " +
			"reconciliation sessions for an account.",
		Example: "  tmoney reconcile start --account Checking --statement-date 2024-01-31 --statement-balance 850.00",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		SilenceUsage: true,
	}
	cmd.AddCommand(newReconcileStartCmd())
	cmd.AddCommand(newReconcileMarkCmd())
	cmd.AddCommand(newReconcileFinishCmd())
	cmd.AddCommand(newReconcileStatusCmd())
	return cmd
}
