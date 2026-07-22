package category

import (
	"fmt"
	"io"
	"text/tabwriter"

	categorydom "github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/spf13/cobra"
)

// categoryListOptions are the inputs to `tmoney category list`.
type categoryListOptions struct {
	file        string
	categoryTyp string
	showIDs     bool
}

// newCategoryListCmd registers `tmoney category list`. Unlike the TUI
// picker this is a management listing: it includes system categories,
// marking them with a `[system]` tag.
func newCategoryListCmd() *cobra.Command {
	opts := &categoryListOptions{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List categories as an indented tree",
		Long: "List categories as an indented tree: top-level categories " +
			"alphabetically, each followed by its subcategories indented two " +
			"spaces. System categories are included and tagged `[system]`. " +
			"Pass --type to restrict to income or expense, and --show-ids to " +
			"prefix each row with its UUID.",
		Example: "  tmoney category list --file personal.tdb\n" +
			"  tmoney category list --file personal.tdb --type income\n" +
			"  tmoney category list --file personal.tdb --show-ids",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runCategoryList(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.categoryTyp, "type", "", "Filter by type: income or expense")
	cmd.Flags().BoolVar(&opts.showIDs, "show-ids", false, "Prefix each row with the category's full UUID")
	return cmd
}

// runCategoryList prints the category tree.
func runCategoryList(opts *categoryListOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	var filterType categorydom.Type
	if opts.categoryTyp != "" {
		t, err := categorydom.ParseType(opts.categoryTyp)
		if err != nil {
			return fmt.Errorf("invalid --type %q: valid types are income, expense", opts.categoryTyp)
		}
		filterType = t
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	topLevel, err := svc.Category.ListTopLevel()
	if err != nil {
		return fmt.Errorf("failed to list categories: %w", err)
	}

	fmt.Fprintln(w, "CATEGORIES")
	fmt.Fprintln(w, "==========")

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if opts.showIDs {
		fmt.Fprintln(tw, "ID\tName\tType")
		fmt.Fprintln(tw, "--\t----\t----")
	} else {
		fmt.Fprintln(tw, "Name\tType")
		fmt.Fprintln(tw, "----\t----")
	}

	rows := 0
	for _, parent := range topLevel {
		if filterType != "" && parent.Type != filterType {
			continue
		}
		printCategoryRow(tw, parent, "", opts.showIDs)
		rows++

		children, err := svc.Category.ListChildren(parent.ID)
		if err != nil {
			return fmt.Errorf("failed to list subcategories of %q: %w", parent.Name, err)
		}
		for _, child := range children {
			printCategoryRow(tw, child, "  ", opts.showIDs)
			rows++
		}
	}
	tw.Flush()

	if rows == 0 {
		fmt.Fprintln(w, "No categories found.")
		return nil
	}
	fmt.Fprintf(w, "\nShowing %d categor%s\n", rows, plural(rows))
	return nil
}

// printCategoryRow writes one tree row. indent is prepended to the name
// column (two spaces for subcategories); system categories are tagged.
func printCategoryRow(tw io.Writer, cat *categorydom.Category, indent string, showIDs bool) {
	name := indent + cat.Name
	if cat.IsSystem {
		name += " [system]"
	}
	if showIDs {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", cat.ID.String(), name, cat.Type.DisplayName())
	} else {
		fmt.Fprintf(tw, "%s\t%s\n", name, cat.Type.DisplayName())
	}
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
