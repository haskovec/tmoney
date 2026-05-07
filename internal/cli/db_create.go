package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/spf13/cobra"
)

// dbCreateOptions are the inputs to `tmoney db create <path>`.
type dbCreateOptions struct {
	path string
}

// newDBCreateCmd registers `tmoney db create <path>`. The single
// positional argument is the destination file path; if it lacks a
// `.tdb` extension, db.Create will add one.
func newDBCreateCmd() *cobra.Command {
	opts := &dbCreateOptions{}
	cmd := &cobra.Command{
		Use:   "create <path>",
		Short: "Create a new database file",
		Long: "Create a new TMoney database at the given path. The " +
			"`.tdb` extension is appended automatically if absent. " +
			"Refuses to overwrite an existing file.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.path = args[0]
			return runDBCreate(opts, cmd.OutOrStdout())
		},
	}
	return cmd
}

// runDBCreate creates a new database file at opts.path.
func runDBCreate(opts *dbCreateOptions, w io.Writer) error {
	database, err := db.Create(opts.path)
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}
	defer database.Close()

	fmt.Fprintf(w, "Created database: %s\n", database.Path())
	return nil
}
