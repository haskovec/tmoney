package cli

import (
	"github.com/spf13/cobra"
)

// newTransferCmd returns the `transfer` parent command. Child verbs are
// registered as they migrate from the legacy `--flag` dispatcher.
func newTransferCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transfer",
		Short: "Manage transfers between accounts",
		Long: "Subcommands for creating new transfers and linking " +
			"unlinked transfer pairs across accounts after import.",
		Example: "  tmoney transfer add --from Checking --to Savings --amount 500.00",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		SilenceUsage: true,
	}
	cmd.AddCommand(newTransferAddCmd())
	return cmd
}
