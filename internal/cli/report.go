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
			"  tmoney report net-worth --as-of 2024-06-30\n" +
			"  tmoney report net-worth --include-closed",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		SilenceUsage: true,
	}
	cmd.AddCommand(newReportNetWorthCmd())
	return cmd
}
