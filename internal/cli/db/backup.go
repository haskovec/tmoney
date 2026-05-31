package db

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/spf13/cobra"
)

// dbBackupOptions are the inputs to `tmoney db backup`.
type dbBackupOptions struct {
	file string
}

// newDBBackupCmd registers `tmoney db backup`. The database file is
// taken from the persistent `--file` / `-f` flag inherited from the
// root command.
func newDBBackupCmd() *cobra.Command {
	opts := &dbBackupOptions{}
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Create a manual backup of the database file",
		Long: "Create a manual backup of the TMoney database. Manual " +
			"backups are never auto-deleted by rolling retention.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runDBBackup(opts, cmd.OutOrStdout())
		},
	}
	return cmd
}

// runDBBackup creates a manual backup of the database file.
func runDBBackup(opts *dbBackupOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	backupPath, err := backup.CreateManualBackup(opts.file)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	fmt.Fprintf(w, "Backup created: %s\n", backupPath)
	return nil
}
