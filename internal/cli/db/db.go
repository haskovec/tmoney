package db

import (
	"github.com/spf13/cobra"
)

// NewCmd returns the `db` parent command. Child verbs (`create`,
// `backup`, `restore`, `list-backups`) are registered as they migrate
// from the legacy `--flag` dispatcher.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Manage TMoney database files",
		Long: "Subcommands for creating, backing up, restoring, and " +
			"listing backups of TMoney database files.",
		Example: "  tmoney db create finances.tdb     Create a new database file",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		SilenceUsage: true,
	}
	cmd.AddCommand(newDBCreateCmd())
	cmd.AddCommand(newDBBackupCmd())
	cmd.AddCommand(newDBRestoreCmd())
	cmd.AddCommand(newDBListBackupsCmd())
	return cmd
}
