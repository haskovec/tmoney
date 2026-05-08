package cli

import (
	"github.com/spf13/cobra"
)

// newTransactionCmd returns the `transaction` parent command. Child
// verbs are registered as they migrate from the legacy `--flag`
// dispatcher.
func newTransactionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transaction",
		Short: "Manage TMoney transactions",
		Long: "Subcommands for adding, listing, voiding, and " +
			"searching transactions on an account.",
		Example: "  tmoney transaction add --account Checking --amount -50.00 --payee \"Coffee Shop\"",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		SilenceUsage: true,
	}
	cmd.AddCommand(newTransactionAddCmd())
	cmd.AddCommand(newTransactionListCmd())
	cmd.AddCommand(newTransactionVoidCmd())
	cmd.AddCommand(newTransactionSearchCmd())
	return cmd
}
