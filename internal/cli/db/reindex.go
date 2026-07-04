package db

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	tmoneydb "github.com/haskovec/tmoney/internal/db"
	"github.com/spf13/cobra"
)

// dbReindexOptions are the inputs to `tmoney db reindex`.
type dbReindexOptions struct {
	file string
}

// newDBReindexCmd registers `tmoney db reindex`. The database file is taken
// from the persistent `--file` / `-f` flag inherited from the root command.
func newDBReindexCmd() *cobra.Command {
	opts := &dbReindexOptions{}
	cmd := &cobra.Command{
		Use:   "reindex",
		Short: "Rebuild database indexes to repair a desynced index",
		Long: "Drop and recreate every secondary index in the database, " +
			"rebuilding each from the table data. Use this when a reconcile, " +
			"edit, or void fails with \"Failed to delete all rows from index\" — " +
			"a DuckDB storage bug that can leave an index out of sync with the " +
			"table. It changes no financial data. Run `tmoney db backup` first.",
		Example:      "  tmoney db reindex --file personal.tdb",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runDBReindex(opts, cmd.OutOrStdout())
		},
	}
	return cmd
}

// runDBReindex rebuilds every secondary index in the database file.
func runDBReindex(opts *dbReindexOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	database, err := tmoneydb.Open(opts.file)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	n, err := database.Reindex()
	if err != nil {
		return fmt.Errorf("failed to reindex database: %w", err)
	}

	fmt.Fprintf(w, "Rebuilt %d database indexes.\n", n)
	return nil
}
