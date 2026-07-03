// Package loan implements the `tmoney loan` CLI command group: a guided,
// one-shot setup for an amortized loan (loan add), plus read-only listing and
// amortization projection (loan list / loan show). All loan math and record
// assembly is shared with the TUI loan wizard through internal/loan and
// internal/scheduled — this package only parses flags, resolves names to IDs,
// and renders output.
package loan

import (
	"github.com/spf13/cobra"
)

// NewCmd returns the `loan` parent command with its add/list/show verbs.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "loan",
		Short: "Set up and inspect amortized loans",
		Long: "Subcommands for setting up an amortized loan (loan account, " +
			"optional linked asset account, and a monthly payment schedule whose " +
			"interest/principal split recomputes from the live balance at post " +
			"time), and for listing loans and projecting their amortization.",
		Example: "  tmoney loan add --name Mortgage --current-balance 312450.22 --rate 6.5 \\\n" +
			"    --payment 2401.86 --next-payment-date 2026-08-01 --from-account Checking\n" +
			"  tmoney loan list\n" +
			"  tmoney loan show Mortgage",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		SilenceUsage: true,
	}
	cmd.AddCommand(newLoanAddCmd())
	cmd.AddCommand(newLoanListCmd())
	cmd.AddCommand(newLoanShowCmd())
	return cmd
}
