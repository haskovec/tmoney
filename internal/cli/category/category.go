// Package category implements the `tmoney category` CLI noun: management
// subcommands (add, list, rename, delete, merge) over the category domain
// service. Unlike the TUI picker, this surface is a management view — it
// includes system categories in listings and surfaces the service's mutation
// guards (system-category refusal, dependents-before-delete) as CLI errors.
package category

import (
	"github.com/spf13/cobra"
)

// NewCmd returns the `category` parent command with its management verbs.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "category",
		Short: "Manage TMoney categories",
		Long: "Subcommands for adding, listing, renaming, deleting, and " +
			"merging categories. System categories (Transfer, Value " +
			"Adjustment) are shown in listings but cannot be renamed, " +
			"deleted, or merged.",
		Example: "  tmoney category add --name Groceries --parent Food\n" +
			"  tmoney category list --file personal.tdb\n" +
			"  tmoney category merge --from Dining --to \"Dining Out\"",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newCategoryAddCmd())
	cmd.AddCommand(newCategoryDeleteCmd())
	cmd.AddCommand(newCategoryListCmd())
	cmd.AddCommand(newCategoryMergeCmd())
	cmd.AddCommand(newCategoryRenameCmd())
	return cmd
}
