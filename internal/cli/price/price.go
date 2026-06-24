package price

import (
	"github.com/spf13/cobra"
)

// NewCmd returns the `price` parent command. Child verbs are
// registered as they migrate from the legacy `--flag` dispatcher.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "price",
		Short: "Manage security prices",
		Long: "Subcommands for adding, listing, and updating security " +
			"prices in TMoney.",
		Example:      "  tmoney price add --ticker AAPL --date 2024-01-15 --price 150.00",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newPriceAddCmd())
	cmd.AddCommand(newPriceCleanupCmd())
	cmd.AddCommand(newPriceDeleteCmd())
	cmd.AddCommand(newPriceListCmd())
	cmd.AddCommand(newPriceCurrentCmd())
	cmd.AddCommand(newPriceImportCmd())
	cmd.AddCommand(newPriceUpdateCmd())
	cmd.AddCommand(newPriceLookupCmd())
	return cmd
}
