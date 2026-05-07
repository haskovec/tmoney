package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/backup"
)

// runRestore restores the database from a backup file.
func runRestore(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--restore requires --file to specify a database")
	}

	fmt.Fprintln(w, "Creating backup of current state...")

	safetyPath, err := backup.Restore(opts.file, opts.restore)
	if safetyPath != "" {
		fmt.Fprintf(w, "Backup created: %s\n", safetyPath)
	}
	if err != nil {
		return fmt.Errorf("failed to restore: %w", err)
	}

	fmt.Fprintf(w, "\nRestoring from: %s\n", opts.restore)
	fmt.Fprintln(w, "Restore complete.")

	return nil
}
