// Package security holds the `tmoney security` command group (add, list,
// show, edit, hide, unhide, delete). It is one of the per-noun CLI subpackages
// carved out of internal/cli; it depends only on the shared cmdutil hub and
// the security domain package (imported as securitydom to avoid the name
// collision) — never on a sibling noun package or on internal/cli itself.
package security

import (
	"github.com/spf13/cobra"
)

// NewCmd returns the `security` parent command with every security verb
// registered as a subcommand.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "security",
		Short: "Manage securities (stocks, ETFs, mutual funds)",
		Long: "Subcommands for creating, listing, editing, hiding, and " +
			"deleting securities tracked by TMoney.",
		Example:      "  tmoney security add --ticker AAPL --name \"Apple Inc.\" --type stock",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newSecurityAddCmd())
	cmd.AddCommand(newSecurityListCmd())
	cmd.AddCommand(newSecurityShowCmd())
	cmd.AddCommand(newSecurityEditCmd())
	cmd.AddCommand(newSecurityHideCmd())
	cmd.AddCommand(newSecurityUnhideCmd())
	cmd.AddCommand(newSecurityDeleteCmd())
	return cmd
}
