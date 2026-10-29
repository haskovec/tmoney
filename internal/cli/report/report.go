package report

import (
	"github.com/spf13/cobra"
)

// NewCmd returns the `report` parent command and its verbs: `net-worth`,
// `spending`, `holdings` and `allocation`.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate financial reports",
		Long: "Subcommands for generating reports from TMoney data: " +
			"net worth (assets vs. liabilities), spending by category, " +
			"holdings across the investment accounts, and the asset " +
			"allocation of those holdings.",
		Example: "  tmoney report net-worth\n" +
			"  tmoney report spending --month 2024-03\n" +
			"  tmoney report spending --year 2024\n" +
			"  tmoney report holdings\n" +
			"  tmoney report allocation",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		SilenceUsage: true,
	}
	cmd.AddCommand(newReportNetWorthCmd())
	cmd.AddCommand(newReportSpendingCmd())
	cmd.AddCommand(newReportHoldingsCmd())
	cmd.AddCommand(newReportAllocationCmd())
	return cmd
}
