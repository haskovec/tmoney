package category

import (
	"errors"
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/spf13/cobra"
)

// categoryDeleteOptions are the inputs to `tmoney category delete <id-or-name>`.
type categoryDeleteOptions struct {
	file string
	ref  string
}

// newCategoryDeleteCmd registers `tmoney category delete <id-or-name>`. The
// single positional argument is resolved as a UUID, an exact name, or a
// Parent:Child path.
func newCategoryDeleteCmd() *cobra.Command {
	opts := &categoryDeleteOptions{}
	cmd := &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Delete a category by id or name",
		Long: "Delete a category identified by its UUID, exact name, or " +
			"Parent:Child path. The deletion is refused when the category is a " +
			"system category, has subcategories, or is still referenced by " +
			"transactions, split lines, or scheduled transactions. When " +
			"references block the delete, use `category merge` to reassign them " +
			"first.",
		Example: "  tmoney category delete --file personal.tdb Groceries\n" +
			"  tmoney category delete --file personal.tdb Food:Groceries\n" +
			"  tmoney category delete --file personal.tdb 0d9f7c2a-…",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.ref = args[0]
			return runCategoryDelete(opts, cmd.OutOrStdout())
		},
	}
	return cmd
}

// runCategoryDelete deletes a category via the domain service, surfacing its
// guard errors. A dependents refusal (transactions/splits/scheduled) is
// annotated with a hint to use `category merge`.
func runCategoryDelete(opts *categoryDeleteOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	resolver := categoryResolver{svc: svc.Category}
	cat, err := resolver.resolve(opts.ref)
	if err != nil {
		return err
	}

	name := cat.Name
	if err := svc.Category.Delete(cat.ID); err != nil {
		var dep *dberrors.HasDependentsError
		if errors.As(err, &dep) && dep.Dependents != "subcategories" {
			return fmt.Errorf("%w; merge it into another category first with `category merge --from %q --to <target>`", err, name)
		}
		return fmt.Errorf("failed to delete category: %w", err)
	}

	fmt.Fprintf(w, "Deleted category %q\n", name)

	cmdutil.AutoBackupAfterModification(database)
	return nil
}
