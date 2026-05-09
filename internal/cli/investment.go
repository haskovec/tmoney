package cli

import (
	"github.com/spf13/cobra"
)

// newInvestmentCmd returns the `investment` parent command. Child verbs
// are registered as they migrate from the legacy `--flag` dispatcher.
func newInvestmentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "investment",
		Short: "Manage investment transactions",
		Long: "Subcommands for buying, selling, and managing investment " +
			"transactions in TMoney.",
		Example:      "  tmoney investment buy --account Brokerage --ticker AAPL --shares 10 --amount 1500",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newInvestmentBuyCmd())
	cmd.AddCommand(newInvestmentSellCmd())
	cmd.AddCommand(newInvestmentDividendCmd())
	cmd.AddCommand(newInvestmentReinvestCmd())
	cmd.AddCommand(newInvestmentFeeCmd())
	cmd.AddCommand(newInvestmentDepositCmd())
	cmd.AddCommand(newInvestmentWithdrawCmd())
	cmd.AddCommand(newInvestmentTransferCmd())
	cmd.AddCommand(newInvestmentSplitCmd())
	cmd.AddCommand(newInvestmentMergeCmd())
	return cmd
}
