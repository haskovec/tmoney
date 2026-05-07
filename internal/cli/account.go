package cli

import (
	"github.com/spf13/cobra"
)

// newAccountCmd returns the `account` parent command. Child verbs
// (`add`, `list`, `show`, `balance`) are registered as they migrate
// from the legacy `--flag` dispatcher.
func newAccountCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "account",
		Short: "Manage TMoney accounts",
		Long: "Subcommands for creating, listing, showing, and " +
			"reporting balances on accounts.",
		Example: "  tmoney account add --name \"Checking\" --type checking",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		SilenceUsage: true,
	}
	cmd.AddCommand(newAccountAddCmd())
	return cmd
}
