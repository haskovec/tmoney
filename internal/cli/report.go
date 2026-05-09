package cli

import (
	"github.com/spf13/cobra"
)

// newReportCmd returns the `report` parent command. Child verbs
// (`net-worth`, `spending`) are registered as they migrate from the
// legacy `--report --report-type` dispatcher.
func newReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate financial reports",
		Long: "Subcommands for generating reports from TMoney data: " +
			"net worth (assets vs. liabilities) and spending by category.",
		Example: "  tmoney report net-worth\n" +
			"  tmoney report spending --month 2024-03\n" +
			"  tmoney report spending --year 2024",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		SilenceUsage: true,
	}
	cmd.AddCommand(newReportNetWorthCmd())
	cmd.AddCommand(newReportSpendingCmd())
	return cmd
}
