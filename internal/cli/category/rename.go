package category

import (
	"fmt"
	"io"
	"strings"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// categoryRenameOptions are the inputs to `tmoney category rename`.
type categoryRenameOptions struct {
	file string
	id   string
	name string
	to   string
}

// newCategoryRenameCmd registers `tmoney category rename`. Exactly one of
// `--id` / `--name` identifies the category; `--to` supplies the new name.
func newCategoryRenameCmd() *cobra.Command {
	opts := &categoryRenameOptions{}
	cmd := &cobra.Command{
		Use:   "rename",
		Short: "Rename a category",
		Long: "Rename a category identified by exactly one of `--id` or " +
			"`--name`, setting its name to `--to`. System categories cannot be " +
			"renamed.",
		Example: "  tmoney category rename --file personal.tdb --name Groceries --to \"Food & Groceries\"\n" +
			"  tmoney category rename --file personal.tdb --id 0d9f7c2a-… --to Utilities",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runCategoryRename(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.id, "id", "", "Category UUID (mutually exclusive with --name)")
	cmd.Flags().StringVar(&opts.name, "name", "", "Category name or Parent:Child path (mutually exclusive with --id)")
	cmd.Flags().StringVar(&opts.to, "to", "", "New name (required)")
	return cmd
}

// runCategoryRename renames a category via the domain service (which refuses
// system categories).
func runCategoryRename(opts *categoryRenameOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	if (opts.id == "") == (opts.name == "") {
		return fmt.Errorf("exactly one of --id or --name is required")
	}
	if strings.TrimSpace(opts.to) == "" {
		return fmt.Errorf("--to (new name) is required")
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	resolver := categoryResolver{svc: svc.Category}

	var ref string
	if opts.id != "" {
		id, err := types.ParseID(opts.id)
		if err != nil {
			return fmt.Errorf("invalid --id: %w", err)
		}
		ref = id.String()
	} else {
		ref = opts.name
	}

	cat, err := resolver.resolve(ref)
	if err != nil {
		return err
	}

	oldName := cat.Name
	cat.Name = strings.TrimSpace(opts.to)
	if err := svc.Category.Update(cat); err != nil {
		return fmt.Errorf("failed to rename category: %w", err)
	}

	fmt.Fprintf(w, "Renamed category %q to %q\n", oldName, cat.Name)

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
