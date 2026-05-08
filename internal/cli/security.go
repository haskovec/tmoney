package cli

import (
	"github.com/spf13/cobra"
)

// newSecurityCmd returns the `security` parent command. Child verbs
// are registered as they migrate from the legacy `--flag` dispatcher.
func newSecurityCmd() *cobra.Command {
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
	return cmd
}
