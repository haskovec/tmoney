package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/backup"
)

// runBackup creates a manual backup of the database file.
func runBackup(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--backup requires --file to specify a database")
	}

	backupPath, err := backup.CreateManualBackup(opts.file)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	fmt.Fprintf(w, "Backup created: %s\n", backupPath)
	return nil
}
