package cli

import (
	"github.com/spf13/cobra"
)

// newScheduledCmd returns the `scheduled` parent command. Child verbs
// are registered as they migrate from the legacy `--flag` dispatcher.
func newScheduledCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scheduled",
		Short: "Manage TMoney scheduled transactions",
		Long: "Subcommands for adding, listing, posting, and skipping " +
			"scheduled transactions on an account.",
		Example: "  tmoney scheduled add --account Checking --frequency monthly --amount -1500 --payee Landlord",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		SilenceUsage: true,
	}
	cmd.AddCommand(newScheduledAddCmd())
	cmd.AddCommand(newScheduledListCmd())
	cmd.AddCommand(newScheduledPostCmd())
	cmd.AddCommand(newScheduledSkipCmd())
	return cmd
}
