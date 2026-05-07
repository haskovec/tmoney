package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/db"
)

// runCreateDB creates a new database file.
func runCreateDB(opts *cliOptions, w io.Writer) error {
	database, err := db.Create(opts.createDB)
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}
	defer database.Close()

	fmt.Fprintf(w, "Created database: %s\n", database.Path())
	return nil
}
