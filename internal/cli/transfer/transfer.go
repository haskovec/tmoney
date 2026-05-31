package transfer

import (
	"github.com/spf13/cobra"
)

// NewCmd returns the `transfer` parent command. Child verbs are
// registered as they migrate from the legacy `--flag` dispatcher.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transfer",
		Short: "Manage transfers between accounts",
		Long: "Subcommands for creating, editing, deleting, and linking " +
			"transfers between accounts.",
		Example: "  tmoney transfer add --from Checking --to Savings --amount 500.00\n" +
			"  tmoney transfer edit --txn-id <uuid> --amount 600.00\n" +
			"  tmoney transfer delete --txn-id <uuid>\n" +
			"  tmoney transfer link --confirm",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		SilenceUsage: true,
	}
	cmd.AddCommand(newTransferAddCmd())
	cmd.AddCommand(newTransferEditCmd())
	cmd.AddCommand(newTransferDeleteCmd())
	cmd.AddCommand(newTransferLinkCmd())
	return cmd
}
