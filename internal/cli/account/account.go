// Package account holds the `tmoney account` command group (add, list,
// show, balance). It is one of the per-noun CLI subpackages carved out of
// internal/cli; it depends only on the shared cmdutil hub, the account
// domain package (imported as accountdom to avoid the name collision), and
// types — never on a sibling noun package or on internal/cli itself.
package account

import (
	"github.com/spf13/cobra"
)

// NewCmd returns the `account` parent command. Child verbs
// (`add`, `list`, `show`, `balance`) are registered as they migrate
// from the legacy `--flag` dispatcher.
func NewCmd() *cobra.Command {
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
	cmd.AddCommand(newAccountListCmd())
	cmd.AddCommand(newAccountShowCmd())
	cmd.AddCommand(newAccountBalanceCmd())
	return cmd
}
