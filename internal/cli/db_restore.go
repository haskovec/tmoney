package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/backup"
	"github.com/spf13/cobra"
)

// dbRestoreOptions are the inputs to `tmoney db restore <backup-path>`.
type dbRestoreOptions struct {
	file       string
	backupPath string
}

// newDBRestoreCmd registers `tmoney db restore <backup-path>`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command; the single positional argument is
// the path of the backup file to restore from.
func newDBRestoreCmd() *cobra.Command {
	opts := &dbRestoreOptions{}
	cmd := &cobra.Command{
		Use:   "restore <backup-path>",
		Short: "Restore the database from a backup file",
		Long: "Restore the TMoney database from the specified backup " +
			"file. A safety backup of the current state is created " +
			"first so the restore can be undone.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.backupPath = args[0]
			return runDBRestore(opts, cmd.OutOrStdout())
		},
	}
	return cmd
}

// runDBRestore restores the database from a backup file.
func runDBRestore(opts *dbRestoreOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--file is required to specify a database")
	}

	fmt.Fprintln(w, "Creating backup of current state...")

	safetyPath, err := backup.Restore(opts.file, opts.backupPath)
	if safetyPath != "" {
		fmt.Fprintf(w, "Backup created: %s\n", safetyPath)
	}
	if err != nil {
		return fmt.Errorf("failed to restore: %w", err)
	}

	fmt.Fprintf(w, "\nRestoring from: %s\n", opts.backupPath)
	fmt.Fprintln(w, "Restore complete.")

	return nil
}
