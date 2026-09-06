package category

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/spf13/cobra"
)

// categoryMergeOptions are the inputs to `tmoney category merge`.
type categoryMergeOptions struct {
	file string
	from string
	to   string
}

// newCategoryMergeCmd registers `tmoney category merge`. Both `--from` and
// `--to` are required and each is resolved as an id-or-name.
func newCategoryMergeCmd() *cobra.Command {
	opts := &categoryMergeOptions{}
	cmd := &cobra.Command{
		Use:   "merge",
		Short: "Merge one category into another",
		Long: "Reassign every transaction, split line, payee default, and " +
			"scheduled transaction from the `--from` category to the `--to` " +
			"category, move any subcategories of `--from` under `--to`, then " +
			"delete `--from`. Both categories must be the same type " +
			"(income/expense); system categories cannot be merged.",
		Example: "  tmoney category merge --file personal.tdb --from Dining --to \"Dining Out\"\n" +
			"  tmoney category merge --file personal.tdb --from Food:Snacks --to Food:Groceries",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runCategoryMerge(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.from, "from", "", "Source category id, name, or Parent:Child path (required)")
	cmd.Flags().StringVar(&opts.to, "to", "", "Target category id, name, or Parent:Child path (required)")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

// runCategoryMerge merges the source category into the target via the domain
// service.
func runCategoryMerge(opts *categoryMergeOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	resolver := categoryResolver{svc: svc.Category}

	source, err := resolver.resolve(opts.from)
	if err != nil {
		return fmt.Errorf("--from: %w", err)
	}
	target, err := resolver.resolve(opts.to)
	if err != nil {
		return fmt.Errorf("--to: %w", err)
	}

	if err := svc.Category.MergeCategories(source.ID, target.ID); err != nil {
		return fmt.Errorf("failed to merge categories: %w", err)
	}

	fmt.Fprintf(w, "Merged category %q into %q\n", source.Name, target.Name)

	cmdutil.AutoBackupAfterModification(database)
	return nil
}
