// Package investment holds the `tmoney investment` command group (buy, sell,
// dividend, reinvest, fee, deposit, withdraw, transfer, split, merge, spin-off,
// portfolio, rebuild-positions). It is one of the per-noun CLI subpackages
// carved out of internal/cli; it depends only on the shared cmdutil hub, the
// investment domain package (imported as investmentdom to avoid the name
// collision) plus the account/app/security/types domain packages — never on a
// sibling noun package or on internal/cli itself.
package investment

import (
	"github.com/spf13/cobra"
)

// NewCmd returns the `investment` parent command with every investment verb
// registered as a subcommand.
func NewCmd() *cobra.Command {
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
	cmd.AddCommand(newInvestmentEditCmd())
	cmd.AddCommand(newInvestmentListCmd())
	cmd.AddCommand(newInvestmentActionsCmd())
	cmd.AddCommand(newInvestmentDividendCmd())
	cmd.AddCommand(newInvestmentReinvestCmd())
	cmd.AddCommand(newInvestmentFeeCmd())
	cmd.AddCommand(newInvestmentFeeLiquidationCmd())
	cmd.AddCommand(newInvestmentDepositCmd())
	cmd.AddCommand(newInvestmentWithdrawCmd())
	cmd.AddCommand(newInvestmentTransferCmd())
	cmd.AddCommand(newInvestmentSplitCmd())
	cmd.AddCommand(newInvestmentSplitLotCmd())
	cmd.AddCommand(newInvestmentMergeCmd())
	cmd.AddCommand(newInvestmentSpinOffCmd())
	cmd.AddCommand(newInvestmentPortfolioCmd())
	cmd.AddCommand(newInvestmentRebuildPositionsCmd())
	cmd.AddCommand(newInvestmentEnableLotsCmd())
	cmd.AddCommand(newInvestmentDisableLotsCmd())
	return cmd
}
