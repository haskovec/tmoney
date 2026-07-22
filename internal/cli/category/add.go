package category

import (
	"fmt"
	"io"
	"strings"

	categorydom "github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/spf13/cobra"
)

// categoryAddOptions are the inputs to `tmoney category add`.
type categoryAddOptions struct {
	file        string
	name        string
	parent      string
	categoryTyp string
}

// newCategoryAddCmd registers `tmoney category add`. The database file is
// taken from the persistent `--file` / `-f` flag inherited from the root
// command. `--name` is required.
func newCategoryAddCmd() *cobra.Command {
	opts := &categoryAddOptions{}
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a new category",
		Long: "Create a new category. `--name` is required. Pass `--parent` to " +
			"create a subcategory under an existing top-level category; the " +
			"subcategory inherits its parent's type unless `--type` is given. " +
			"Without `--parent`, `--type` defaults to expense.",
		Example: "  tmoney category add --file personal.tdb --name \"Side Gig\" --type income\n" +
			"  tmoney category add --file personal.tdb --name Groceries --parent Food",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runCategoryAdd(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.name, "name", "", "Category name (required)")
	cmd.Flags().StringVar(&opts.parent, "parent", "", "Parent category name, id, or Parent:Child path (creates a subcategory)")
	cmd.Flags().StringVar(&opts.categoryTyp, "type", "", "Category type: income or expense (default expense; inherited from --parent when omitted)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

// runCategoryAdd creates a category or subcategory.
func runCategoryAdd(opts *categoryAddOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}
	if strings.TrimSpace(opts.name) == "" {
		return fmt.Errorf("--name must not be empty")
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	resolver := categoryResolver{svc: svc.Category}

	var cat *categorydom.Category
	var parent *categorydom.Category

	if opts.parent != "" {
		parent, err = resolver.resolve(opts.parent)
		if err != nil {
			return err
		}
		if parent.IsSystem {
			return fmt.Errorf("cannot add a subcategory under system category %q", parent.Name)
		}
		if !parent.IsTopLevel() {
			return fmt.Errorf("cannot nest under %q: subcategories cannot have their own children", parent.Name)
		}

		// Subcategory type: inherit the parent's unless --type overrides it.
		catType := parent.Type
		if opts.categoryTyp != "" {
			catType, err = categorydom.ParseType(opts.categoryTyp)
			if err != nil {
				return fmt.Errorf("invalid --type %q: valid types are income, expense", opts.categoryTyp)
			}
			if catType != parent.Type {
				return fmt.Errorf("subcategory type %s must match parent %q type %s", catType, parent.Name, parent.Type)
			}
		}
		cat = categorydom.NewSubcategory(opts.name, parent.ID, catType)
	} else {
		catType := categorydom.TypeExpense
		if opts.categoryTyp != "" {
			catType, err = categorydom.ParseType(opts.categoryTyp)
			if err != nil {
				return fmt.Errorf("invalid --type %q: valid types are income, expense", opts.categoryTyp)
			}
		}
		cat = categorydom.NewCategory(opts.name, catType)
	}

	if err := svc.Category.Create(cat); err != nil {
		return fmt.Errorf("failed to create category: %w", err)
	}

	fmt.Fprintln(w, "Category created successfully!")
	fmt.Fprintf(w, "  Name: %s\n", cat.Name)
	fmt.Fprintf(w, "  Type: %s\n", cat.Type.DisplayName())
	if parent != nil {
		fmt.Fprintf(w, "  Parent: %s\n", parent.Name)
	}

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
