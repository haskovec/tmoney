package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/haskovec/tmoney/internal/backup"
	"github.com/spf13/cobra"
)

// dbListBackupsOptions are the inputs to `tmoney db list-backups`.
type dbListBackupsOptions struct {
	file string
}

// newDBListBackupsCmd registers `tmoney db list-backups`. The database
// file is taken from the persistent `--file` / `-f` flag inherited
// from the root command.
func newDBListBackupsCmd() *cobra.Command {
	opts := &dbListBackupsOptions{}
	cmd := &cobra.Command{
		Use:   "list-backups",
		Short: "List available backups for the database file",
		Long: "List every backup found alongside the TMoney database, " +
			"newest first, with timestamp, size, and type (auto or manual).",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runDBListBackups(opts, cmd.OutOrStdout())
		},
	}
	return cmd
}

// runDBListBackups lists available backups for the database file.
func runDBListBackups(opts *dbListBackupsOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--file is required to specify a database")
	}

	backups, err := backup.ListBackups(opts.file)
	if err != nil {
		return fmt.Errorf("failed to list backups: %w", err)
	}

	dbBase := filepath.Base(opts.file)
	fmt.Fprintf(w, "BACKUPS: %s\n", dbBase)
	fmt.Fprintln(w, strings.Repeat("=", len("BACKUPS: ")+len(dbBase)))

	if len(backups) == 0 {
		fmt.Fprintln(w, "No backups found.")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 4, ' ', 0)
	fmt.Fprintln(tw, "Date\tSize\tType")
	fmt.Fprintln(tw, "----\t----\t----")

	for _, b := range backups {
		fmt.Fprintf(tw, "%s\t%s\t%s\n",
			b.Timestamp.Format("2006-01-02 15:04:05"),
			backup.FormatSize(b.Size),
			b.Type,
		)
	}

	tw.Flush()
	fmt.Fprintf(w, "\n%d backup(s) found\n", len(backups))

	return nil
}
