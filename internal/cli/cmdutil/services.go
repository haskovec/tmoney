package cmdutil

import (
	"errors"
	"fmt"
	"os"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/db"
)

// OpenServices opens the database and creates all services via the shared registry.
// It also does a best-effort update of the recent files in the config.
// Auto-posts due scheduled transactions and prints a summary if any were posted.
func OpenServices(file string) (*db.DB, *app.Services, error) {
	database, err := db.Open(file)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Best-effort update recent files
	if cfg, err := config.Load(); err == nil {
		cfg.AddRecentFile(file)
		_ = cfg.Save()
	}

	svc := app.NewServices(database)

	// Auto-post due scheduled transactions on file open
	if summary, err := svc.Scheduled.AutoPost(); err == nil && summary.PostedCount > 0 {
		fmt.Fprintf(os.Stdout, "Auto-posted %d scheduled transaction(s)\n", summary.PostedCount)
	}

	return database, svc, nil
}

// AutoBackupAfterModification creates an auto-backup after a data-modifying CLI
// command. It CLOSES the database first: a copy of an open DuckDB file misses
// the writes still in its WAL, and on Windows the copy cannot even open the
// file (see db.DB.Close). Call it as the command's last database action; the
// caller's deferred Close is then a harmless no-op.
func AutoBackupAfterModification(database *db.DB) {
	// Best-effort: don't fail the CLI command if the close or backup fails
	if err := database.Close(); err != nil {
		return
	}
	_, _ = backup.CreateAutoBackup(database.Path())
}

// RequireFile returns the standard error when no database file was supplied via
// the persistent --file flag. It folds the identical guard repeated across
// every data-touching command; callers do:
//
//	if err := cmdutil.RequireFile(opts.file); err != nil {
//		return err
//	}
func RequireFile(file string) error {
	if file == "" {
		return errors.New("--file is required to specify a database")
	}
	return nil
}
